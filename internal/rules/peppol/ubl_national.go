package peppol

import (
	"math"
	"math/big"
	"regexp"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// National rules of the UBL schematron: Slovakia, Norway, Denmark, Italy,
// Sweden, Greece, Iceland, the Netherlands and Germany. Each method checks
// the rules of one pattern whose context is n.

// supplierParty reports whether p is cac:AccountingSupplierParty/cac:Party.
func supplierParty(p *xmltree.Node) bool {
	return uis(p, "cac:Party") && uparent(p, "cac:AccountingSupplierParty")
}

// customerParty reports whether p is cac:AccountingCustomerParty/cac:Party.
func customerParty(p *xmltree.Node) bool {
	return uis(p, "cac:Party") && uparent(p, "cac:AccountingCustomerParty")
}

// parentOf returns the parent of n, nil when n is nil.
func parentOf(n *xmltree.Node) *xmltree.Node {
	if n == nil {
		return nil
	}
	return n.Parent
}

// slovakiaAddress checks the pattern of the Slovak addresses: on a domestic
// invoice, where both the seller and the buyer have their postal address in
// Slovakia, the addresses give a street, a city and a postal code.
func (k *ublChecker) slovakiaAddress(n *xmltree.Node) {
	if !k.supplierPostalSK || !k.customerPostalSK || !uis(n, "cac:PostalAddress") {
		return
	}
	switch p := n.Parent; {
	case supplierParty(p):
		k.check(uexists(n, "cbc:StreetName"), "SK-R-002", n, "")
		k.check(uexists(n, "cbc:CityName"), "SK-R-003", n, "")
		k.check(uexists(n, "cbc:PostalZone"), "SK-R-004", n, "")
	case customerParty(p):
		k.check(uexists(n, "cbc:StreetName"), "SK-R-005", n, "")
		k.check(uexists(n, "cbc:CityName"), "SK-R-006", n, "")
		k.check(uexists(n, "cbc:PostalZone"), "SK-R-007", n, "")
	}
}

// slovakia checks the pattern of SK-R-001 on an invoice whose seller and
// buyer are in Slovakia ($supplierCountry and $customerCountry): a corrected
// invoice (384) refers to the invoice it corrects.
func (k *ublChecker) slovakia(n *xmltree.Node) {
	if k.supplierCountry != "SK" || k.customerCountry != "SK" || !isElem(n, nsInvoice, "Invoice") {
		return
	}
	code := nsv(ufirst(n, "cbc:InvoiceTypeCode"))
	k.check(code != "384" || uexists(n, "cac:BillingReference"), "SK-R-001", n, "type code %q", code)
}

// norway checks the Norwegian pattern on a seller, when $supplierCountry
// is NO.
func (k *ublChecker) norway(n *xmltree.Node) {
	if k.supplierCountry != "NO" || !supplierParty(n) {
		return
	}
	var registered *xmltree.Node    // first CompanyID of a tax scheme 'TAX'
	var vat, vatIDs []*xmltree.Node // tax schemes 'VAT' and their CompanyID
	for _, s := range ulist(n, "cac:PartyTaxScheme") {
		switch taxSchemeID(s) {
		case "TAX":
			if registered == nil {
				registered = ufirst(s, "cbc:CompanyID")
			}
		case "VAT":
			vat = append(vat, s)
			vatIDs = append(vatIDs, ulist(s, "cbc:CompanyID")...)
		}
	}
	k.check(nsv(registered) == "Foretaksregisteret", "NO-R-002", orNode(registered, n), "value: %q", nsv(registered))
	norwegian := false
	for _, s := range vat {
		if substring(sv(ufirst(s, "cbc:CompanyID")), 1, 2) == "NO" {
			norwegian = true
		}
	}
	if norwegian {
		v := sv(ufirst(vat[0], "cbc:CompanyID"))
		k.check(reNorwegianVAT().MatchString(substringFrom(v, 3)) && mod11(substring(firstValue(vatIDs), 3, 9)), "NO-R-001",
			orNode(ufirst(vat[0], "cbc:CompanyID"), n), "value: %q", v)
	}
}

// denmark checks the Danish pattern: document rules for a seller in
// Denmark, the other rules when the buyer is in Denmark too. The countries
// are the raw codes of the addresses.
func (k *ublChecker) denmark(n *xmltree.Node) {
	if k.supplierCode != "DK" {
		return
	}
	if isDoc(n) {
		k.danishDocument(n)
		return
	}
	if k.customerCode != "DK" {
		return
	}
	p := parentOf(n)
	switch {
	case uis(n, "cac:PartyIdentification") && (supplierParty(p) || customerParty(p)) && isDoc(p.Parent.Parent):
		k.check(!uexists(n, "cbc:ID") || ufirstAttr(n, "cbc:ID", "schemeID") != "", "DK-R-013", n, "")
	case uis(n, "cac:PaymentMeans") && isElem(p, nsInvoice, "Invoice"):
		k.danishPaymentMeans(n)
	case customerParty(n) && isDoc(p.Parent):
		const legal = "cac:PartyLegalEntity/cbc:CompanyID"
		k.check(!uexists(n, legal) || ufirstAttr(n, legal, "schemeID") == "0184", "DK-R-017", n, "scheme %q", ufirstAttr(n, legal, "schemeID"))
	case uis(n, "cac:InvoiceLine") && isElem(p, nsInvoice, "Invoice") || uis(n, "cac:CreditNoteLine") && isElem(p, nsCreditNote, "CreditNote"):
		const class = "cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode"
		if uanyAttrIn(n, class, "listID", "TST") {
			k.check(uanyAttrIn(n, class, "listVersionID", "19.05.01", "19.0501", "26.08.01", "26.0801"), "DK-R-003", n,
				"version %q", attr(ufirst(n, class), "listVersionID"))
		}
	case uis(n, "cac:AllowanceCharge"):
		if uanyEq(n, "cbc:AllowanceChargeReasonCode", "ZZZ") {
			reasons := ulist(n, "cbc:AllowanceChargeReason")
			reason, reasonText := firstValue(reasons), ""
			if len(reasons) > 0 {
				reasonText = text(reasons[0])
			}
			taxCategory := length(norm(reasonText)) == 4 && number(reason) >= 0 && number(reason) <= 9999
			hashed := len(reasons) > 0 && strings.Contains(reason, "#") && !strings.HasPrefix(reason, "#") && !strings.HasSuffix(reason, "#")
			k.check(taxCategory || hashed, "DK-R-004", n, "reason %q", reason)
		}
	}
}

// danishDocument checks the document rules of a Danish seller: legal entity
// (DK-R-002, DK-R-014) and the total of a domestic credit note (DK-R-016).
func (k *ublChecker) danishDocument(n *xmltree.Node) {
	const legal = uSupplier + "/cac:PartyLegalEntity/cbc:CompanyID"
	id := ufirst(n, legal)
	k.check(norm(text(id)) != "", "DK-R-002", n, "")
	k.check(id == nil || ufirstAttr(n, legal, "schemeID") == "0184", "DK-R-014", n, "scheme %q", ufirstAttr(n, legal, "schemeID"))
	if k.creditNote && k.customerCode == "DK" {
		pay := ufirst(n, "cac:LegalMonetaryTotal/cbc:PayableAmount")
		k.check(!(number(text(pay)) < 0), "DK-R-016", orNode(pay, n), "value: %q", text(pay))
	}
}

var reDanishGiro = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[0-9]{7,8}$`) })

// danishPaymentMeans checks DK-R-005 to DK-R-011 on a payment means of a
// domestic Danish invoice.
func (k *ublChecker) danishPaymentMeans(pm *xmltree.Node) {
	code := sv(ufirst(pm, "cbc:PaymentMeansCode"))
	k.check(strings.Contains(" 1 10 31 42 48 49 50 58 59 93 97 ", " "+code+" "), "DK-R-005", pm, "value: %q", code)

	is := func(c string) bool { return uanyEq(pm, "cbc:PaymentMeansCode", c) }
	account := ufirst(pm, "cac:PayeeFinancialAccount/cbc:ID")
	id := sv(ufirst(pm, "cbc:PaymentID"))
	prefix := substring(id, 1, 3)
	if is("31") || is("42") {
		k.check(norm(text(account)) != "" && norm(text(ufirst(pm, "cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID"))) != "",
			"DK-R-006", pm, "")
	}
	if is("49") {
		k.check(norm(text(ufirst(pm, "cac:PaymentMandate/cbc:ID"))) != "" && norm(text(ufirst(pm, "cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID"))) != "",
			"DK-R-007", pm, "")
	}
	if is("50") {
		k.check(in(prefix, "01#", "04#", "15#") && reDanishGiro().MatchString(sv(account)), "DK-R-008", pm, "payment ID %q, account %q", id, sv(account))
		if in(prefix, "04#", "15#") {
			k.check(length(id) == 19, "DK-R-009", pm, "payment ID %q", id)
		}
	}
	if is("93") {
		k.check(in(prefix, "71#", "73#", "75#") && length(text(account)) == 8, "DK-R-010", pm, "payment ID %q, account %q", id, sv(account))
		if in(prefix, "71#", "75#") {
			k.check(length(id) == 18 || length(id) == 19, "DK-R-011", pm, "payment ID %q", id)
		}
	}
}

// italy checks the Italian pattern on a seller, when $supplierCountry is IT.
func (k *ublChecker) italy(n *xmltree.Node) {
	if k.supplierCountry != "IT" {
		return
	}
	switch {
	case uis(n, "cac:PartyTaxScheme") && supplierParty(n.Parent) && taxSchemeID(n) != "VAT":
		v := nsv(ufirst(n, "cbc:CompanyID"))
		k.check(reItalianFC().MatchString(v), "IT-R-001", n, "value: %q", v)
	case supplierParty(n):
		k.check(uexists(n, "cac:PostalAddress/cbc:StreetName"), "IT-R-002", n, "")
		k.check(uexists(n, "cac:PostalAddress/cbc:CityName"), "IT-R-003", n, "")
		k.check(uexists(n, "cac:PostalAddress/cbc:PostalZone"), "IT-R-004", n, "")
	}
}

// sweden checks the Swedish pattern, for sellers whose address is in Sweden.
func (k *ublChecker) sweden(n *xmltree.Node) {
	p := parentOf(n)
	swedish := func(party *xmltree.Node) bool { return uanyEq(party, uCountry, "SE") }
	switch {
	case supplierParty(n) && swedish(n) && swedishVATParty(n):
		var id *xmltree.Node // cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID
		for _, s := range ulist(n, "cac:PartyTaxScheme") {
			if isVATScheme(s) {
				if id = ufirst(s, "cbc:CompanyID"); id != nil {
					break
				}
			}
		}
		v := sv(id)
		k.check(length(norm(v)) == 14, "SE-R-001", id, "value: %q", v)
		k.check(isNumber(substring(v, 3, 12)), "SE-R-002", id, "value: %q", v)
	case uis(n, "cac:PartyLegalEntity") && supplierParty(p) && swedish(p) && uexists(n, "cbc:CompanyID"):
		id := ufirst(n, "cbc:CompanyID")
		v := nsv(id)
		k.check(isNumber(sv(id)), "SE-R-003", id, "value: %q", v)
		k.check(length(v) == 10, "SE-R-004", id, "value: %q", v)
		k.check(swedishOrgNumber(v), "SE-R-013", id, "value: %q", v)
	case uis(n, "cbc:CompanyID") && uis(p, "cac:PartyTaxScheme") && supplierParty(p.Parent) && swedish(p.Parent) &&
		uexists(p.Parent, "cac:PartyLegalEntity/cbc:CompanyID") && norm(strings.ToUpper(sv(ufirst(p, "cac:TaxScheme/cbc:ID")))) != "VAT":
		v := norm(strings.ToUpper(sv(n)))
		k.check(v == "GODKÄND FÖR F-SKATT", "SE-R-005", n, "value: %q", sv(n))
	case (uis(n, "cac:TaxCategory") || uis(n, "cac:ClassifiedTaxCategory")) && k.seSupplierVAT && k.seCustomer && uanyEq(n, "cbc:ID", "S"):
		rate := sv(ufirst(n, "cbc:Percent"))
		f := number(rate)
		k.check(f == 25 || f == 12 || f == 6, "SE-R-006", n, "rate %q", rate)
	case uis(n, "cbc:ID") && uis(p, "cac:PayeeFinancialAccount") && uis(p.Parent, "cac:PaymentMeans") && k.seSupplier:
		pm := p.Parent
		code := nsv(ufirst(pm, "cbc:PaymentMeansCode"))
		branch := nsv(ufirst(pm, "cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID"))
		v := nsv(n)
		switch {
		case code == "30" && branch == "SE:PLUSGIRO":
			k.check(isNumber(v), "SE-R-007", n, "value: %q", v)
			k.check(length(v) >= 2 && length(v) <= 8, "SE-R-010", n, "value: %q", v)
		case code == "30" && branch == "SE:BANKGIRO":
			k.check(isNumber(v), "SE-R-008", n, "value: %q", v)
			k.check(length(v) == 7 || length(v) == 8, "SE-R-009", n, "value: %q", v)
		}
	case uis(n, "cac:PaymentMeans") && k.seSupplier && uanyIn(n, "cbc:PaymentMeansCode", "50", "56"):
		k.fail("SE-R-011", n, "payment means code %q", sv(ufirst(n, "cbc:PaymentMeansCode")))
	case uis(n, "cac:PaymentMeans") && k.seSupplier && k.seCustomer && uanyEq(n, "cbc:PaymentMeansCode", "31"):
		k.fail("SE-R-012", n, "")
	}
}

var (
	// reGreekDate is $dateRegExp: a day, a month and a year, optionally
	// separated by -, \, / or a space. In the XPath string, \\ is an escaped
	// backslash of the regular expression, as in Go.
	reGreekDate = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^(0?[1-9]|[12][0-9]|3[01])[-\\/ ]?(0?[1-9]|1[0-2])[-\\/ ]?(19|20)[0-9]{2}`)
	})
	reMARK = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[1-9]([0-9]*)`) })
)

// greece checks the pattern of the Greek sender rules.
func (k *ublChecker) greece(n *xmltree.Node) {
	p := parentOf(n)
	switch {
	case k.greekSender && uis(n, "cbc:ID") && k.atRoot(n):
		k.greekInvoiceID(n)
	case k.greekSender && supplierParty(n):
		k.check(length(sv(ufirst(n, "cac:PartyName/cbc:Name"))) > 0, "GR-R-002", n, "")
		k.check(greekVAT(n), "GR-S-011", n, "")
	case k.greekSender && uis(n, "cbc:CompanyID") && uis(p, "cac:PartyTaxScheme") && taxSchemeID(p) == "VAT" && supplierParty(p.Parent):
		v := sv(n)
		k.check(substring(v, 1, 2) == "EL" && tin(substringFrom(v, 3)), "GR-R-003", n, "value: %q", v)
	case k.greekSender && n == k.root && (k.invoice || k.creditNote) && k.greekAddress():
		mark, url := countDescription(n, "##M.AR.K##"), countDescription(n, "##INVOICE|URL##")
		k.check(mark == 1, "GR-R-004-1", n, "%d MARK numbers", mark)
		k.check(url == 1, "GR-S-008-1", n, "%d invoice URLs", url)
		k.check(url <= 1, "GR-R-008-2", n, "%d invoice URLs", url)
	case k.greekSender && uis(n, "cbc:ID") && uis(p, "cac:AdditionalDocumentReference") && k.greekAddress() &&
		uanyEq(p, "cbc:DocumentDescription", "##M.AR.K##"):
		k.check(reMARK().MatchString(sv(n)), "GR-R-004-2", n, "value: %q", sv(n))
	case k.greekSender && uis(n, "cac:AdditionalDocumentReference") && uanyEq(n, "cbc:DocumentDescription", "##INVOICE|URL##"):
		k.check(length(nsv(ufirst(n, "cac:Attachment/cac:ExternalReference/cbc:URI"))) > 0, "GR-R-008-3", n, "")
	case k.greekSender && customerParty(n):
		k.check(length(sv(ufirst(n, "cac:PartyName/cbc:Name"))) > 0, "GR-R-005", n, "")
	case (k.accountingSupplierCountry == "GR" || k.accountingSupplierCountry == "EL") && uis(n, "cbc:EndpointID") && supplierParty(p):
		k.check(n.Attr("schemeID") == "9933" && tin(sv(n)), "GR-R-009", n, "scheme %q, value: %q", n.Attr("schemeID"), sv(n))
	}
}

// greeceDomestic checks the pattern of the rules for a Greek sender and a
// Greek receiver.
func (k *ublChecker) greeceDomestic(n *xmltree.Node) {
	if !k.greekSender || !k.greekReceiver {
		return
	}
	switch {
	case customerParty(n):
		k.check(greekVAT(n), "GR-R-006", n, "")
	case uis(n, "cbc:EndpointID") && customerParty(n.Parent):
		k.check(n.Attr("schemeID") == "9933" && tin(sv(n)), "GR-R-010", n, "scheme %q, value: %q", n.Attr("schemeID"), sv(n))
	}
}

// greekAddress reports whether the address of the seller is in Greece
// (/*/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode = 'GR').
func (k *ublChecker) greekAddress() bool { return uanyEq(k.root, uSupplierCountry, "GR") }

// countDescription returns
// count(cac:AdditionalDocumentReference[cbc:DocumentDescription = desc]).
func countDescription(n *xmltree.Node, desc string) int {
	c := 0
	for _, d := range ulist(n, "cac:AdditionalDocumentReference") {
		if uanyEq(d, "cbc:DocumentDescription", desc) {
			c++
		}
	}
	return c
}

// greekVAT reports whether a party has exactly one VAT identifier, made of
// EL and a valid TIN (GR-S-011, GR-R-006).
func greekVAT(party *xmltree.Node) bool {
	var ids []*xmltree.Node
	for _, s := range ulist(party, "cac:PartyTaxScheme") {
		if taxSchemeID(s) == "VAT" {
			ids = append(ids, ulist(s, "cbc:CompanyID")...)
		}
	}
	return len(ids) == 1 && substring(sv(ids[0]), 1, 2) == "EL" && tin(substringFrom(sv(ids[0]), 3))
}

// tin implements u:TinVerification: the ninth character of a Greek tax
// identification number is the check digit of the first eight (weights 256
// to 2, modulo 11, modulo 10). The characters are converted with number().
func tin(s string) bool {
	r := []rune(s)
	d := func(i int) float64 {
		if i >= len(r) {
			return math.NaN()
		}
		return number(string(r[i]))
	}
	sum := 0.0
	for i := 0; i < 8; i++ {
		sum += d(i) * float64(int(256)>>i)
	}
	return math.Mod(math.Mod(sum, 11), 10) == d(8)
}

// tokenize implements tokenize(s, separator) for a literal separator: the
// empty string gives no token.
func tokenize(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

// greekInvoiceID checks the six segments of the invoice number of a Greek
// seller (GR-R-001-*).
func (k *ublChecker) greekInvoiceID(n *xmltree.Node) {
	segs := tokenize(sv(n), "|")
	seg := func(i int) (string, bool) {
		if i > len(segs) {
			return "", false
		}
		return segs[i-1], true
	}
	k.check(len(segs) == 6, "GR-R-001-1", n, "%d segments", len(segs))

	s1, _ := seg(1)
	var tins []string // the TINs of the VAT identifiers of the seller and of the tax representative
	for _, path := range []string{uSupplier + "/cac:PartyTaxScheme", uTaxRep + "/cac:PartyTaxScheme"} {
		for _, s := range ulist(k.root, path) {
			if isVATScheme(s) {
				tins = append(tins, substring(sv(ufirst(s, "cbc:CompanyID")), 3, 9))
			}
		}
	}
	k.check(length(norm(s1)) == 9 && tin(s1) && in(s1, tins...), "GR-R-001-2", n, "value: %q", s1)

	s2, ok := seg(2)
	date := tokenize(s2, "/")
	issue := tokenize(sv(ufirst(k.root, "cbc:IssueDate")), "-")
	same := func(i, j int) bool { return i <= len(date) && j <= len(issue) && date[i-1] == issue[j-1] }
	k.check(ok && length(norm(s2)) > 0 && reGreekDate().MatchString(s2) && same(1, 3) && same(2, 2) && same(3, 1), "GR-R-001-3", n,
		"value: %q, issue date %q", s2, sv(ufirst(k.root, "cbc:IssueDate")))

	s3, _ := seg(3)
	if length(norm(s3)) > 0 && isNumber(s3) {
		if v, ok := xsInteger(s3); ok { // xs:integer() fails in the reference otherwise
			k.check(v.Sign() >= 0, "GR-R-001-4", n, "value: %q", s3)
		}
	} else {
		k.fail("GR-R-001-4", n, "value: %q", s3)
	}

	s5, _ := seg(5)
	k.check(length(s5) > 0, "GR-R-001-6", n, "")
	s6, _ := seg(6)
	k.check(length(s6) > 0, "GR-R-001-7", n, "")
}

// xsInteger implements xs:integer(s): an optional sign and ASCII digits,
// surrounding white space ignored.
func xsInteger(s string) (*big.Int, bool) {
	if !castableInteger(s) {
		return nil, false
	}
	v, ok := new(big.Int).SetString(strings.TrimPrefix(trim(s), "+"), 10)
	return v, ok
}

// iceland checks the Icelandic pattern, for a seller whose address is in
// Iceland (raw country code).
func (k *ublChecker) iceland(n *xmltree.Node) {
	if k.supplierCode != "IS" {
		return
	}
	switch {
	case isDoc(n):
		k.icelandDocument(n)
	case uis(n, "cac:AccountingCustomerParty") && isDoc(n.Parent) && k.customerCode == "IS":
		const legal = "cac:Party/cac:PartyLegalEntity/cbc:CompanyID"
		k.check(uexists(n, legal) && uanyAttrIn(n, legal, "schemeID", "0196"), "IS-R-004", n, "")
		k.check(uexists(n, "cac:Party/cac:PostalAddress/cbc:StreetName") && uexists(n, "cac:Party/cac:PostalAddress/cbc:PostalZone"), "IS-R-005", n, "")
	}
}

// icelandDocument checks the document rules of an Icelandic seller.
func (k *ublChecker) icelandDocument(n *xmltree.Node) {
	typeCode := func(path string) bool {
		v := nsv(ufirst(n, path))
		return !strings.Contains(v, " ") && strings.Contains(" 380 381 ", " "+v+" ")
	}
	k.check(typeCode("cbc:InvoiceTypeCode") || typeCode("cbc:CreditNoteTypeCode"), "IS-R-001", n, "")
	const legal = uSupplier + "/cac:PartyLegalEntity/cbc:CompanyID"
	k.check(uexists(n, legal) && uanyAttrIn(n, legal, "schemeID", "0196"), "IS-R-002", n, "")
	k.check(uexists(n, uSupplier+"/cac:PostalAddress/cbc:StreetName") && uexists(n, uSupplier+"/cac:PostalAddress/cbc:PostalZone"), "IS-R-003", n, "")
	for _, c := range []struct{ code, id string }{{"9", "IS-R-006"}, {"42", "IS-R-007"}} {
		means := false
		var ids []*xmltree.Node
		for _, pm := range ulist(n, "cac:PaymentMeans") {
			if uanyEq(pm, "cbc:PaymentMeansCode", c.code) {
				means = true
				ids = append(ids, ulist(pm, "cac:PayeeFinancialAccount/cbc:ID")...)
			}
		}
		k.check(len(ids) > 0 && length(nsv(ids[0])) == 12 || !means, c.id, n, "account %q", firstValue(ids))
	}
	var eindagi []*xmltree.Node // cac:AdditionalDocumentReference[cbc:DocumentDescription = 'EINDAGI']/cbc:ID
	found := false
	for _, d := range ulist(n, "cac:AdditionalDocumentReference") {
		if uanyEq(d, "cbc:DocumentDescription", "EINDAGI") {
			found = true
			eindagi = append(eindagi, ulist(d, "cbc:ID")...)
		}
	}
	if !found {
		return
	}
	v := firstValue(eindagi)
	_, date := xsDate(v)
	k.check(length(v) == 10 && date, "IS-R-008", n, "value: %q", v)
	due := ulist(n, "cbc:DueDate")
	k.check(len(due) > 0, "IS-R-009", n, "")
	later := false
	for _, d := range due {
		for _, e := range eindagi {
			if sv(d) <= sv(e) {
				later = true
			}
		}
	}
	k.check(later, "IS-R-010", n, "due date %q, EINDAGI %q", firstValue(due), v)
}

// netherlands checks the Dutch pattern, for a seller whose address is in
// the Netherlands.
func (k *ublChecker) netherlands(n *xmltree.Node) {
	if !k.supplierNL {
		return
	}
	p := parentOf(n)
	address := func(id string) {
		k.check(uexists(n, "cbc:StreetName") && uexists(n, "cbc:CityName") && uexists(n, "cbc:PostalZone"), id, n, "")
	}
	legal := func(id string) {
		s := " " + n.Attr("schemeID") + " "
		k.check((strings.Contains(s, " 0106 ") || strings.Contains(s, " 0190 ")) && nsv(n) != "", id, n, "scheme %q", n.Attr("schemeID"))
	}
	switch {
	case uis(n, "cbc:CreditNoteTypeCode"):
		k.check(uexists(k.root, "cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID"), "NL-R-001", n, "")
	case uis(n, "cac:PostalAddress") && supplierParty(p):
		address("NL-R-002")
	case uis(n, "cbc:CompanyID") && uis(p, "cac:PartyLegalEntity") && supplierParty(p.Parent):
		legal("NL-R-003")
	case k.customerNL && uis(n, "cac:PostalAddress") && customerParty(p):
		address("NL-R-004")
	case k.customerNL && uis(n, "cbc:CompanyID") && uis(p, "cac:PartyLegalEntity") && customerParty(p.Parent):
		legal("NL-R-005")
	case k.taxRepNL && uis(n, "cac:PostalAddress") && uis(p, "cac:TaxRepresentativeParty"):
		address("NL-R-006")
	case uis(n, "cac:LegalMonetaryTotal"):
		paid := false
		if pay := ufirst(n, "cbc:PayableAmount"); pay != nil && (k.invoice || k.creditNote) {
			v, ok := dec(pay)
			if !ok {
				return // xs:decimal() fails in the reference
			}
			paid = k.invoice && v.Sign() <= 0 || k.creditNote && v.Sign() >= 0
		}
		k.check(paid || k.hasPaymentMeans(), "NL-R-007", n, "payable amount %q", sv(ufirst(n, "cbc:PayableAmount")))
	case k.customerNL && uis(n, "cac:PaymentMeans"):
		v := nsv(ufirst(n, "cbc:PaymentMeansCode"))
		k.check(in(v, "30", "48", "49", "57", "58", "59"), "NL-R-008", n, "value: %q", v)
	case uis(n, "cbc:LineID") && uis(p, "cac:OrderLineReference"):
		k.check(uexists(k.root, "cac:OrderReference/cbc:ID"), "NL-R-009", n, "")
	}
}

// hasPaymentMeans reports whether the document has a cac:PaymentMeans
// anywhere (//cac:PaymentMeans).
func (k *ublChecker) hasPaymentMeans() bool {
	found := false
	k.root.Walk(func(n *xmltree.Node) bool {
		if uis(n, "cac:PaymentMeans") {
			found = true
		}
		return !found
	})
	return found
}

// Regular expressions of the German pattern. XPath's \s is the XML white
// space, without the form feed of Go's \s.
var (
	reSkonto = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(^|\r?\n)#(SKONTO)#TAGE=([0-9]+#PROZENT=[0-9]+\.[0-9]{2})(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$`)
	})
	reXREmail    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[^@ \t\n\r]+@([^@. \t\n\r]+\.)+[^@. \t\n\r]+$`) })
	reXRPhone    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`.*([0-9].*){3,}.*`) })
	reXRURL      = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^([a-zA-Z])([a-zA-Z0-9+.-])+:.*`) })
	reNewline    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\r?\n`) })
	reSkontoItem = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`#.+#`) })
	reLeadingNL  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[ \t\n\r]*\n`) })
	reIBAN       = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[a-zA-Z0-9]{0,30}$`) })
	reSpaces     = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[ \n\r\t]`) })
)

// germany checks the German pattern, when both the seller and the buyer
// have their address in Germany. Its contexts are absolute paths from the
// invoice or credit note.
func (k *ublChecker) germany(n *xmltree.Node) {
	if !k.supplierDE || !k.customerDE || !(k.invoice || k.creditNote) {
		return
	}
	p := parentOf(n)
	switch {
	case n == k.root:
		k.germanDocument(n)
	case uis(n, "cac:ExternalReference") && uis(p, "cac:Attachment") && uis(p.Parent, "cac:AdditionalDocumentReference") && k.atRoot(p.Parent):
		uri := nsv(ufirst(n, "cbc:URI"))
		k.check(reXRURL().MatchString(uri), "DE-R-T02", n, "value: %q", uri)
	case uis(n, "cac:AccountingSupplierParty") && k.atRoot(n):
		k.check(uexists(n, "cac:Party/cac:Contact"), "DE-R-002", n, "")
	case uis(n, "cac:PostalAddress") && supplierParty(p) && k.atRoot(p.Parent):
		k.check(nonBlank(n, "cbc:CityName"), "DE-R-003", n, "")
		k.check(nonBlank(n, "cbc:PostalZone"), "DE-R-004", n, "")
	case uis(n, "cac:Contact") && supplierParty(p) && k.atRoot(p.Parent):
		k.check(nonBlank(n, "cbc:Name"), "DE-R-005", n, "")
		k.check(nonBlank(n, "cbc:Telephone"), "DE-R-006", n, "")
		k.check(nonBlank(n, "cbc:ElectronicMail"), "DE-R-007", n, "")
		phone, mail := nsv(ufirst(n, "cbc:Telephone")), nsv(ufirst(n, "cbc:ElectronicMail"))
		k.check(reXRPhone().MatchString(phone), "DE-R-027", n, "value: %q", phone)
		k.check(reXREmail().MatchString(mail), "DE-R-028", n, "value: %q", mail)
	case uis(n, "cac:PostalAddress") && customerParty(p) && k.atRoot(p.Parent):
		k.check(nonBlank(n, "cbc:CityName"), "DE-R-008", n, "")
		k.check(nonBlank(n, "cbc:PostalZone"), "DE-R-009", n, "")
	case uis(n, "cac:Address") && uis(p, "cac:DeliveryLocation") && uis(p.Parent, "cac:Delivery") && k.atRoot(p.Parent):
		k.check(nonBlank(n, "cbc:CityName"), "DE-R-010", n, "")
		k.check(nonBlank(n, "cbc:PostalZone"), "DE-R-011", n, "")
	case uis(n, "cac:PaymentMeans") && k.atRoot(n):
		k.germanPaymentMeans(n)
	case uis(n, "cac:TaxSubtotal") && uis(p, "cac:TaxTotal") && k.atRoot(p):
		k.check(nonBlank(n, "cac:TaxCategory/cbc:Percent"), "DE-R-014", n, "")
	}
}

// germanDocument checks the document rules of the German pattern.
func (k *ublChecker) germanDocument(n *xmltree.Node) {
	k.check(uexists(n, "cac:PaymentMeans"), "DE-R-001", n, "")
	k.check(nonBlank(n, "cbc:BuyerReference"), "DE-R-015", n, "")

	// DE-R-016: a VAT category code of an allowance, a charge or a line
	// among S, Z, E, AE, K, G, L and M requires a VAT identifier of the
	// seller or a tax representative.
	vatCodes := []string{"S", "Z", "E", "AE", "K", "G", "L", "M"}
	used := false
	for _, ac := range ulist(n, "cac:AllowanceCharge") {
		if (uanyEq(ac, "cbc:ChargeIndicator", "false") || uanyEq(ac, "cbc:ChargeIndicator", "true")) &&
			uanyIn(ac, "cac:TaxCategory/cbc:ID", vatCodes...) {
			used = true
		}
	}
	for _, path := range []string{"cac:InvoiceLine", "cac:CreditNoteLine"} {
		for _, l := range ulist(n, path) {
			if uanyIn(l, "cac:Item/cac:ClassifiedTaxCategory/cbc:ID", vatCodes...) {
				used = true
			}
		}
	}
	k.check(!used || uexists(n, uTaxRep) || nonBlank(n, uSupplier+"/cac:PartyTaxScheme/cbc:CompanyID"), "DE-R-016", n, "")

	types := []string{"326", "380", "384", "389", "381", "875", "876", "877"}
	invoiceType, creditType := nsv(ufirst(n, "cbc:InvoiceTypeCode")), nsv(ufirst(n, "cbc:CreditNoteTypeCode"))
	k.check(in(invoiceType, types...) || in(creditType, types...), "DE-R-017", n, "value: %q", invoiceType+creditType)
	k.check(skonto(n), "DE-R-018", n, "")

	// DE-R-022: no attachment has the file name of an attachment of a
	// preceding document reference.
	seen := map[string]bool{}
	dup := ""
	for _, d := range ulist(n, "cac:AdditionalDocumentReference") {
		var names []string
		for b := range uall(d, "cac:Attachment/cbc:EmbeddedDocumentBinaryObject") {
			if f, ok := b.LookupAttr("filename"); ok {
				if seen[f] && dup == "" {
					dup = f
				}
				names = append(names, f)
			}
		}
		for _, f := range names {
			seen[f] = true
		}
	}
	k.check(dup == "", "DE-R-022", n, "file name %q", dup)

	k.check(invoiceType != "384" && creditType != "384" || uexists(n, "cac:BillingReference/cac:InvoiceDocumentReference"), "DE-R-026", n, "")
	mandate := uexists(n, "cac:PaymentMeans/cac:PaymentMandate")
	k.check(!mandate || uanyAttrIn(n, uSupplier+"/cac:PartyIdentification/cbc:ID", "schemeID", "SEPA") ||
		uanyAttrIn(n, "cac:PayeeParty/cac:PartyIdentification/cbc:ID", "schemeID", "SEPA"), "DE-R-030", n, "")
	k.check(!mandate || uexists(n, "cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID"), "DE-R-031", n, "")
}

// tokenizeRe implements tokenize(s, re): the empty string gives no token.
func tokenizeRe(s string, re *regexp.Regexp) []string {
	if s == "" {
		return nil
	}
	return re.Split(s, -1)
}

// skonto checks DE-R-018: every line of the first note of the payment terms
// that starts with # is a cash discount statement, and the last statement
// is followed by a line break.
func skonto(n *xmltree.Node) bool {
	var notes []*xmltree.Node // cac:PaymentTerms/cbc:Note[1]
	for _, t := range ulist(n, "cac:PaymentTerms") {
		if note := ufirst(t, "cbc:Note"); note != nil {
			notes = append(notes, note)
		}
	}
	var lines []string
	var last []string // tokenize(., '#.+#')[last()] of each note
	for _, note := range notes {
		for _, l := range tokenizeRe(sv(note), reNewline()) {
			if strings.HasPrefix(norm(l), "#") {
				lines = append(lines, l)
			}
		}
		if t := tokenizeRe(sv(note), reSkontoItem()); len(t) > 0 {
			last = append(last, t[len(t)-1])
		}
	}
	for _, l := range lines {
		if !reSkonto().MatchString(norm(l)) || !reLeadingNL().MatchString(firstOr(last)) {
			return false
		}
	}
	return true
}

// germanPaymentMeans checks the German rules on a payment means: credit
// transfer (30, 58), payment card (48, 54, 55) or direct debit (59).
func (k *ublChecker) germanPaymentMeans(pm *xmltree.Node) {
	transfer := uexists(pm, "cac:PayeeFinancialAccount")
	card := uexists(pm, "cac:CardAccount")
	mandate := uexists(pm, "cac:PaymentMandate")
	switch code := nsv(ufirst(pm, "cbc:PaymentMeansCode")); code {
	case "30", "58":
		if code == "58" {
			id := sv(ufirst(pm, "cac:PayeeFinancialAccount/cbc:ID"))
			k.check(validIBAN(id), "DE-R-019", pm, "value: %q", id)
		}
		k.check(transfer, "DE-R-023-1", pm, "")
		k.check(!card && !mandate, "DE-R-023-2", pm, "")
	case "48", "54", "55":
		k.check(card, "DE-R-024-1", pm, "")
		k.check(!transfer && !mandate, "DE-R-024-2", pm, "")
	case "59":
		id := sv(ufirst(pm, "cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID"))
		k.check(validIBAN(id), "DE-R-020", pm, "value: %q", id)
		k.check(mandate, "DE-R-025-1", pm, "")
		k.check(!transfer && !card, "DE-R-025-2", pm, "")
	}
}

// validIBAN checks an IBAN as DE-R-019 and DE-R-020 do: white space removed,
// two capital letters, two digits and up to 30 letters or digits, and the
// ISO 7064 check (letters as numbers from 10, lower-case letters too).
func validIBAN(s string) bool {
	s = norm(reSpaces().ReplaceAllString(s, ""))
	if !reIBAN().MatchString(s) {
		return false
	}
	var b strings.Builder
	for _, c := range s[4:] + strings.ToUpper(s[:2]) + s[2:4] {
		if c > 64 {
			b.WriteString(big.NewInt(int64(c - 55)).String())
		} else {
			b.WriteString(big.NewInt(int64(c - 48)).String())
		}
	}
	v, ok := new(big.Int).SetString(b.String(), 10)
	return ok && new(big.Int).Mod(v, big.NewInt(97)).Int64() == 1
}
