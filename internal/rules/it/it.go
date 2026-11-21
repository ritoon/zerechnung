// Package it implements the Italian rules of CIUS-IT, the CIUS of EN 16931
// for invoices addressed to Italian public administrations through the
// Sistema di Interscambio (SDI), as the Italian technical rules define them
// (Regole tecniche relative alla gestione delle fatture di cui all'art. 3,
// comma 1, d.lgs. 148/2018, version 2.7 of 20/11/2026, applicable from
// 11/01/2027): the rules of appendix 2, which have not changed since
// version 2.5 of 22/11/2024, version 2.6 in force today included. The SDI
// rejects an invoice that breaks one of them with the code 00703.
//
// The technical rules publish no schematron: the rules are written from
// the table of appendix 2, with its identifiers (BR-IT-*). Their messages
// follow the English version of the technical rules, corrected where the
// Italian text, which prevails, differs (see Catalog). The rules that
// make the SDI reject the invoice are errors. Three rules only decide how
// the SDI maps a value (BR-IT-DC-150, BR-IT-220, BR-IT-250: a country
// subdivision that is not an Italian province is attached to the
// translated invoice instead of being mapped): they are warnings.
//
// # Conditions
//
// The rules marked EU in the table apply to every CIUS-IT invoice; those
// marked IT (BR-IT-DC-*, the domestic CIUS) only when the seller is in
// Italy (BT-40 is IT), as section 4.1.3.1 of the technical rules says.
// Check applies both to any invoice it runs on: package validate runs it
// for an invoice whose specification identifier (BT-24) is a CIUS-IT one,
// or on option.
//
// # Rules not implemented
//
// Two rules of appendix 2 depend on facts that the invoice does not
// carry:
//
//   - BR-IT-DC-291: the reason of a document level charge (BT-104) must
//     give the type of pension fund when the charge is a pension fund
//     contribution (Rivalsa Cassa Previdenziale), which nothing in the
//     charge says.
//   - BR-IT-DC-480: an invoice subject to stamp duty above 77,47 euros
//     describes it with a document level charge of reason code SAE; whether
//     the invoice is subject to stamp duty is not in the invoice, and the
//     mapping rules of appendix 4 accept other amounts than the zero the
//     rule gives.
//
// BR-IT-DC-510 is checked in part: a VAT category code B (split payment)
// requires the category B everywhere and a payment terms text (BT-20) that
// mentions split payment; the exact wording the rule quotes is not
// required.
//
// The rules of the Italian extensions (appendix 3, BR-IT-DE-*) are not
// implemented: they check the content of UBL extensions (EXT-IT), which
// packages ubl and validate do not read.
//
// # Identifiers
//
// The technical rules do not check the Italian identifiers beyond their
// length. Check also verifies, as warnings, the format and the check
// character of the Partita IVA (IT-PIVA-01, IT-PIVA-02) and of the Codice
// Fiscale (IT-CF-01, IT-CF-02) wherever the invoice declares them, and that
// the specification identifier matches the countries of the parties
// (IT-PROFILE-01). These identifiers are specific to this package.
//
// For a UBL invoice, the rules run on the CII view of the document, and
// the findings are located in the UBL document.
package it

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Specification identifiers (BT-24) of a CIUS-IT invoice.
const (
	urnCrossBorder = invoice.URNCIUSIT
	urnDomestic    = invoice.URNCIUSIT + invoice.ItalianExtension
)

// Schemes of the Italian identifiers.
const (
	schemeIPA = "0201" // EAS: code of the Indice delle Pubbliche Amministrazioni
	schemeCF  = "0210" // ICD and EAS: Codice Fiscale
	schemePIV = "0211" // ICD and EAS: Partita IVA
)

// VAT category codes BR-IT-350 accepts.
const vatCategories = " AE E S G K Z B "

// checker runs the rules on one document.
type checker struct {
	c        *rules.Checker
	d        *rules.Doc
	domestic bool // the seller is in Italy (BT-40 is IT)
}

// Check runs the CIUS-IT rules and the identifier checks on c.Doc and
// records the findings in c.Report.
func Check(c *rules.Checker) {
	if c == nil || c.Doc == nil || c.Doc.Root == nil {
		return
	}
	d := c.Doc
	k := &checker{c: c, d: d}
	k.domestic = d.Agreement.Value("SellerTradeParty/PostalTradeAddress/CountryID") == "IT"
	k.header()
	k.seller()
	k.buyer()
	k.taxRepresentative()
	k.delivery()
	k.payment()
	k.totals()
	k.vat()
	k.documents()
	for _, l := range d.Lines {
		k.line(l)
	}
	k.splitPayment()
	k.profile()
}

// fail records a finding of rule id at n, with the description of the rule
// and the details.
func (k *checker) fail(id string, n *xmltree.Node, details string, args ...any) {
	if n == nil {
		n = k.d.Root
	}
	msg := "[" + id + "] " + descriptions[id]
	if details != "" {
		msg += " " + fmt.Sprintf(details, args...)
	}
	k.c.Fail(id, severities[id], n, "%s", msg)
}

// maxLen checks that the trimmed text of each node has at most max
// characters.
func (k *checker) maxLen(id string, max int, nodes ...*xmltree.Node) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if v := n.Trim(); length(v) > max {
			k.fail(id, n, "Length: %d, value: %q.", length(v), v)
		}
	}
}

// required checks that each path under parent has a non blank value.
func (k *checker) required(id string, parent *xmltree.Node, paths ...string) {
	var missing []string
	for _, p := range paths {
		if parent.Value(p) == "" {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		k.fail(id, parent, "Missing: ram:%s.", strings.Join(missing, ", ram:"))
	}
}

// province checks that the country subdivision of address is an Italian
// province code.
func (k *checker) province(id string, address *xmltree.Node) {
	if n := address.Elem("CountrySubDivisionName"); n != nil {
		if v := n.Trim(); v != "" && !provinces[v] {
			k.fail(id, n, "Value: %q.", v)
		}
	}
}

// amounts checks that each amount, written with 2 decimals, has at most 15
// characters (BR-IT-290 to 340, 390, 420).
func (k *checker) amounts(id string, nodes ...*xmltree.Node) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if v := n.Trim(); !fits(v, 15, 2, 2) {
			k.fail(id, n, "Value: %q.", v)
		}
	}
}

// prices checks that each quantity or price has at most 8 decimals and,
// written with at least 2, at most 21 characters (BR-IT-380, 430).
func (k *checker) prices(id string, nodes ...*xmltree.Node) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if v := n.Trim(); !fits(v, 21, 2, 8) {
			k.fail(id, n, "Value: %q.", v)
		}
	}
}

// vatID checks a VAT identifier (BT-31, BT-48, BT-63): BR-IT-120, 180 or 230,
// and the Partita IVA of an Italian one.
func (k *checker) vatID(id string, party *xmltree.Node, name string) {
	for _, n := range party.Elems("SpecifiedTaxRegistration") {
		idn := n.Elem("ID")
		if idn == nil || strings.TrimSpace(idn.Attr("schemeID")) != "VA" {
			continue
		}
		v := idn.Trim()
		if length(v) > 30 || len(v) < 2 || !letter(v[0]) || !letter(v[1]) {
			k.fail(id, idn, "Value: %q.", v)
		}
		if rest, ok := strings.CutPrefix(v, "IT"); ok {
			k.piva(idn, rest, name)
		}
	}
}

// name checks the name of a party (BT-27, BT-44, BT-62): BR-IT-091, 171 or
// 222.
func (k *checker) name(id string, party *xmltree.Node) {
	n := party.Elem("Name")
	if n == nil {
		return
	}
	v := n.Trim()
	rest, person := strings.CutPrefix(v, "Nome#Cognome:")
	if !person {
		rest, person = strings.CutPrefix(v, "Nome&Cognome:")
	}
	if !person {
		if length(v) > 80 {
			k.fail(id, n, "Length: %d, value: %q.", length(v), v)
		}
		return
	}
	first, last, _ := strings.Cut(rest, "#")
	if length(first) > 60 || length(last) > 60 {
		k.fail(id, n, "First name: %d characters, surname: %d, value: %q.", length(first), length(last), v)
	}
}

// header checks the document level references and notes.
func (k *checker) header() {
	d, agr := k.d, k.d.Agreement
	if n := d.Header.Elem("ID"); n != nil {
		if v := n.Trim(); v != "" && (length(v) > 20 || !strings.ContainsAny(v, "0123456789")) {
			k.fail("BR-IT-010", n, "Value: %q.", v)
		}
	}
	k.maxLen("BR-IT-020", 15, agr.FindAll("SpecifiedProcuringProject/ID")...)
	k.maxLen("BR-IT-030", 20, agr.FindAll("ContractReferencedDocument/IssuerAssignedID")...)
	k.maxLen("BR-IT-040", 20, agr.FindAll("BuyerOrderReferencedDocument/IssuerAssignedID")...)
	k.maxLen("BR-IT-050", 20, d.Delivery.FindAll("ReceivingAdviceReferencedDocument/IssuerAssignedID")...)
	k.maxLen("BR-IT-060", 20, d.Delivery.FindAll("DespatchAdviceReferencedDocument/IssuerAssignedID")...)
	for _, r := range agr.Elems("AdditionalReferencedDocument") {
		switch r.Value("TypeCode") {
		case "50":
			k.maxLen("BR-IT-070", 15, r.Elem("IssuerAssignedID"))
		case "130":
			k.maxLen("BR-IT-071", 20, r.Elem("IssuerAssignedID"))
		}
	}
	k.maxLen("BR-IT-080", 20, d.Settlement.FindAll("ReceivableSpecifiedTradeAccountingAccount/ID")...)
	k.maxLen("BR-IT-081", 200, d.Header.FindAll("IncludedNote/Content")...)
	k.maxLen("BR-IT-090", 20, d.Settlement.FindAll("InvoiceReferencedDocument/IssuerAssignedID")...)
}

// seller checks the seller party (BG-4).
func (k *checker) seller() {
	s := k.d.Agreement.Elem("SellerTradeParty")
	if s == nil {
		return
	}
	k.name("BR-IT-091", s)
	ids := append(s.Elems("ID"), s.Elems("GlobalID")...)
	if k.domestic {
		for _, n := range ids {
			k.sellerIdentifier(n)
		}
		if n := s.Find("SpecifiedLegalOrganization/ID"); n != nil {
			v := n.Trim()
			switch {
			case strings.HasPrefix(v, "CF:"):
				if l := length(v); l < 14 || l > 19 {
					k.fail("BR-IT-DC-110", n, "Length: %d, value: %q.", l, v)
				}
			case strings.TrimSpace(n.Attr("schemeID")) == schemeCF:
				if l := length(v); l < 11 || l > 16 {
					k.fail("BR-IT-DC-110", n, "Length: %d, value: %q.", l, v)
				}
			}
		}
	}
	k.vatID("BR-IT-120", s, "Seller VAT identifier (BT-31)")
	k.maxLen("BR-IT-122", 21, s.Elem("Description"))
	if k.domestic {
		addr := s.Elem("PostalTradeAddress")
		k.required("BR-IT-DC-140", or(addr, s), "LineOne", "CityName", "PostcodeCode")
		if line2 := addr.Value("LineTwo"); !houseNumber(line2) {
			k.fail("BR-IT-DC-141", or(addr.Elem("LineTwo"), or(addr, s)), "Value: %q.", line2)
		}
		k.province("BR-IT-DC-150", addr)
		for _, n := range s.FindAll("DefinedTradeContact/TelephoneUniversalCommunication/CompleteNumber") {
			if v := n.Trim(); length(v) < 5 || length(v) > 12 {
				k.fail("BR-IT-DC-161", n, "Length: %d, value: %q.", length(v), v)
			}
		}
	}
	k.identifiers(s, ids, "Seller", "BT-29", "BT-30", "BT-34")
	if k.domestic {
		for _, r := range s.Elems("SpecifiedTaxRegistration") {
			if n := r.Elem("ID"); n != nil && strings.TrimSpace(n.Attr("schemeID")) == "FC" {
				k.cf(n, n.Trim(), "Seller tax registration identifier (BT-32)")
			}
		}
	}
}

// sellerIdentifier checks a seller identifier (BT-29) of a domestic invoice:
// BR-IT-DC-100.
func (k *checker) sellerIdentifier(n *xmltree.Node) {
	v := n.Trim()
	l := length(v)
	switch {
	case strings.HasPrefix(v, "EORI:"):
		if l < 18 || l > 22 {
			k.fail("BR-IT-DC-100", n, "Length: %d, value: %q.", l, v)
		}
	case strings.HasPrefix(v, "ALBO:"):
		if l > 137 {
			k.fail("BR-IT-DC-100", n, "Length: %d, value: %q.", l, v)
		}
	case strings.HasPrefix(v, "REA:"):
		office, number, ok := strings.Cut(v[len("REA:"):], "#")
		if l < 8 || l > 27 || !ok || office == "" || number == "" {
			k.fail("BR-IT-DC-100", n, "Length: %d, value: %q.", l, v)
		}
	}
}

// buyer checks the buyer party (BG-7).
func (k *checker) buyer() {
	b := k.d.Agreement.Elem("BuyerTradeParty")
	if b == nil {
		return
	}
	k.name("BR-IT-171", b)
	k.vatID("BR-IT-180", b, "Buyer VAT identifier (BT-48)")

	uri := b.Find("URIUniversalCommunication/URIID")
	switch scheme := strings.TrimSpace(uri.Attr("schemeID")); {
	case uri == nil || uri.Trim() == "":
		k.fail("BR-IT-190", or(uri, b), "The buyer has no electronic address.")
	case scheme != schemeIPA:
		k.fail("BR-IT-190", uri, "Scheme: %q.", scheme)
	case !ipaCode(uri.Trim()):
		k.fail("BR-IT-200", uri, "Value: %q.", uri.Trim())
	}

	addr := b.Elem("PostalTradeAddress")
	k.required("BR-IT-210", or(addr, b), "LineOne", "CityName", "PostcodeCode")
	k.province("BR-IT-220", addr)

	legal := b.Find("SpecifiedLegalOrganization/ID")
	hasVAT := false
	for _, r := range b.Elems("SpecifiedTaxRegistration") {
		if n := r.Elem("ID"); n != nil && strings.TrimSpace(n.Attr("schemeID")) == "VA" && n.Trim() != "" {
			hasVAT = true
		}
	}
	switch {
	case legal.Trim() == "" && !hasVAT:
		k.fail("BR-IT-520", b, "The buyer has neither a VAT identifier nor a legal registration identifier.")
	case legal.Trim() != "":
		v, l := legal.Trim(), length(legal.Trim())
		cf := strings.HasPrefix(v, "CF:") && l >= 14 && l <= 19
		scheme := strings.TrimSpace(legal.Attr("schemeID")) == schemeCF && l >= 11 && l <= 16
		if !cf && !scheme {
			k.fail("BR-IT-520", legal, "Scheme: %q, value: %q.", legal.Attr("schemeID"), v)
		}
	}

	ids := append(b.Elems("ID"), b.Elems("GlobalID")...)
	for _, n := range ids {
		v := n.Trim()
		code, ok := strings.CutPrefix(v, "EORI:")
		if l := length(code); !ok || l < 13 || l > 17 || !alnum(code) {
			k.fail("BR-IT-521", n, "Value: %q.", v)
		}
	}
	k.identifiers(b, ids, "Buyer", "BT-46", "BT-47", "BT-49")
}

// taxRepresentative checks the seller tax representative party (BG-11).
func (k *checker) taxRepresentative() {
	p := k.d.Agreement.Elem("SellerTaxRepresentativeTradeParty")
	if p == nil {
		return
	}
	k.name("BR-IT-222", p)
	k.vatID("BR-IT-230", p, "Seller tax representative VAT identifier (BT-63)")
}

// delivery checks the deliver to address (BG-15).
func (k *checker) delivery() {
	for _, addr := range k.d.Delivery.FindAll("ShipToTradeParty/PostalTradeAddress") {
		if addr.Value("CountryID") != "IT" {
			continue
		}
		k.required("BR-IT-240", addr, "LineOne", "CityName", "PostcodeCode")
		k.province("BR-IT-250", addr)
	}
}

// payment checks the payment instructions (BG-16) and terms (BT-20).
func (k *checker) payment() {
	set := k.d.Settlement
	if set == nil {
		return
	}
	means := set.Elems("SpecifiedTradeSettlementPaymentMeans")
	if len(means) == 0 {
		k.fail("BR-IT-260", set, "")
	}
	terms := false
	for _, n := range set.FindAll("SpecifiedTradePaymentTerms/Description") {
		if n.Trim() != "" {
			terms = true
		}
	}
	if !terms {
		k.fail("BR-IT-261", or(set.Elem("SpecifiedTradePaymentTerms"), set), "")
	}
	if k.domestic {
		k.maxLen("BR-IT-DC-262", 60, set.Elems("PaymentReference")...)
		k.maxLen("BR-IT-DC-221", 200, set.FindAll("PayeeTradeParty/Name")...)
	}
	for _, m := range means {
		for _, acc := range m.Elems("PayeePartyCreditorFinancialAccount") {
			if n := acc.Elem("IBANID"); n != nil {
				if v := n.Trim(); !validIBAN(v) {
					k.fail("BR-IT-270", n, "Value: %q.", v)
				}
			} else if n := acc.Elem("ProprietaryID"); n != nil {
				k.fail("BR-IT-270", n, "The account is identified by a proprietary identifier: %q.", n.Trim())
			}
		}
		for _, n := range m.FindAll("PayeeSpecifiedCreditorFinancialInstitution/BICID") {
			if l := length(n.Trim()); l < 8 || l > 11 {
				k.fail("BR-IT-280", n, "Length: %d, value: %q.", l, n.Trim())
			}
		}
	}
}

// totals checks the document level amounts.
func (k *checker) totals() {
	set, sum := k.d.Settlement, k.d.Summation
	k.amounts("BR-IT-290", set.FindAll("SpecifiedTradeAllowanceCharge/ActualAmount")...)
	k.amounts("BR-IT-300", sum.Elems("GrandTotalAmount")...)
	k.amounts("BR-IT-310", sum.Elems("RoundingAmount")...)
	k.amounts("BR-IT-320", sum.Elems("DuePayableAmount")...)
	k.amounts("BR-IT-330", set.FindAll("ApplicableTradeTax/BasisAmount")...)
	k.amounts("BR-IT-340", set.FindAll("ApplicableTradeTax/CalculatedAmount")...)
}

// categoryCodes returns the VAT category codes of the invoice: BT-118,
// BT-95 and BT-102, BT-151.
func (k *checker) categoryCodes() []*xmltree.Node {
	set := k.d.Settlement
	codes := set.FindAll("ApplicableTradeTax/CategoryCode")
	codes = append(codes, set.FindAll("SpecifiedTradeAllowanceCharge/CategoryTradeTax/CategoryCode")...)
	for _, l := range k.d.Lines {
		codes = append(codes, l.FindAll("SpecifiedLineTradeSettlement/ApplicableTradeTax/CategoryCode")...)
	}
	return codes
}

// vat checks the VAT categories and exemption reasons.
func (k *checker) vat() {
	for _, n := range k.categoryCodes() {
		if v := n.Trim(); v != "" && !strings.Contains(vatCategories, " "+v+" ") {
			k.fail("BR-IT-350", n, "Value: %q.", v)
		}
	}
	if k.domestic {
		for _, n := range k.d.Settlement.FindAll("ApplicableTradeTax/ExemptionReason") {
			if v := n.Trim(); length(v) > 105 || !latin(v) {
				k.fail("BR-IT-DC-351", n, "Length: %d, value: %q.", length(v), v)
			}
		}
	}
}

// documents checks the supporting documents (BG-24).
func (k *checker) documents() {
	for _, r := range k.d.Agreement.Elems("AdditionalReferencedDocument") {
		if r.Value("TypeCode") != "916" || r.Value("IssuerAssignedID") == "" {
			continue
		}
		if r.Value("URIID") == "" && r.Elem("AttachmentBinaryObject") == nil {
			k.fail("BR-IT-360", r, "Supporting document: %q.", r.Value("IssuerAssignedID"))
		}
	}
}

// line checks an invoice line (BG-25).
func (k *checker) line(l *xmltree.Node) {
	if n := l.Find("AssociatedDocumentLineDocument/LineID"); n != nil {
		if v := n.Trim(); !digits(v) || len(strings.TrimLeft(v, "0")) > 4 {
			k.fail("BR-IT-361", n, "Value: %q.", v)
		}
	}
	agr := l.Elem("SpecifiedLineTradeAgreement")
	set := l.Elem("SpecifiedLineTradeSettlement")
	for _, r := range set.Elems("AdditionalReferencedDocument") {
		if r.Value("TypeCode") == "130" {
			k.maxLen("BR-IT-370", 100, r.Elem("IssuerAssignedID"))
		}
	}
	k.prices("BR-IT-380", l.FindAll("SpecifiedLineTradeDelivery/BilledQuantity")...)
	k.amounts("BR-IT-390", set.FindAll("SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount")...)
	k.maxLen("BR-IT-400", 20, agr.FindAll("BuyerOrderReferencedDocument/LineID")...)
	k.maxLen("BR-IT-410", 20, set.FindAll("ReceivableSpecifiedTradeAccountingAccount/ID")...)
	k.amounts("BR-IT-420", set.FindAll("SpecifiedTradeAllowanceCharge/ActualAmount")...)
	k.prices("BR-IT-430", agr.FindAll("NetPriceProductTradePrice/ChargeAmount")...)
	product := l.Elem("SpecifiedTradeProduct")
	k.maxLen("BR-IT-440", 35, product.Elems("SellerAssignedID")...)
	k.maxLen("BR-IT-470", 35, product.FindAll("DesignatedProductClassification/ClassCode")...)
	if product != nil {
		name, desc := product.Value("Name"), product.Value("Description")
		if l := length(name) + length(desc); l > 1000 || !latin(name) || !latin(desc) {
			k.fail("BR-IT-530", or(product.Elem("Name"), product), "Length: %d, value: %q.", l, strings.TrimSpace(name+" "+desc))
		}
	}
}

// splitPayment checks BR-IT-DC-510 on an invoice that uses the VAT category
// B (split payment).
func (k *checker) splitPayment() {
	if !k.domestic {
		return
	}
	codes := k.categoryCodes()
	split := false
	for _, n := range codes {
		if n.Trim() == "B" {
			split = true
		}
	}
	if !split {
		return
	}
	for _, n := range codes {
		if v := n.Trim(); v != "B" {
			k.fail("BR-IT-DC-510", n, "The invoice uses the VAT category B (split payment) and the category %q.", v)
		}
	}
	mentioned := false
	for _, n := range k.d.Settlement.FindAll("SpecifiedTradePaymentTerms/Description") {
		if strings.Contains(strings.ToLower(n.Normalized()), "split payment") {
			mentioned = true
		}
	}
	if !mentioned {
		k.fail("BR-IT-DC-510", or(k.d.Settlement.Elem("SpecifiedTradePaymentTerms"), k.d.Settlement),
			"The payment terms (BT-20) do not mention split payment.")
	}
}

// profile checks IT-PROFILE-01 on an invoice that declares CIUS-IT.
func (k *checker) profile() {
	n := k.d.Context.Find("GuidelineSpecifiedDocumentContextParameter/ID")
	spec := n.Trim()
	if invoice.ProfileFromURN(spec) != invoice.ProfileCIUSIT {
		return
	}
	ext := strings.HasSuffix(spec, invoice.ItalianExtension)
	buyerIT := k.d.Agreement.Value("BuyerTradeParty/PostalTradeAddress/CountryID") == "IT"
	switch {
	case k.domestic && !ext:
		k.fail(RuleProfile, n, "The seller is in Italy: the invoice is domestic, its identifier should end with %q; value: %q.", invoice.ItalianExtension, spec)
	case !k.domestic && ext:
		k.fail(RuleProfile, n, "The seller is not in Italy: the Italian extension applies to domestic invoices only; value: %q.", spec)
	case !k.domestic && !buyerIT:
		k.fail(RuleProfile, n, "Neither the seller nor the buyer is in Italy: the CIUS-IT applies to invoices sent to Italian public administrations.")
	}
}

// identifiers checks the Partita IVA and the Codice Fiscale in the
// identifiers of a party: its identifiers ids (BT-29, BT-46) of scheme 0210
// or 0211, its legal registration identifier (BT-30, BT-47) beginning with
// "CF:" or of scheme 0210 or 0211, and its electronic address (BT-34,
// BT-49) of scheme 0210 or 0211. who, idBT, legalBT and uriBT name them in
// the messages.
func (k *checker) identifiers(party *xmltree.Node, ids []*xmltree.Node, who, idBT, legalBT, uriBT string) {
	check := func(n *xmltree.Node, v, scheme, name string) {
		switch scheme {
		case schemeCF:
			k.cf(n, v, name+" of scheme "+schemeCF)
		case schemePIV:
			k.piva(n, v, name+" of scheme "+schemePIV)
		}
	}
	for _, n := range ids {
		check(n, n.Trim(), strings.TrimSpace(n.Attr("schemeID")), who+" identifier ("+idBT+")")
	}
	if n := party.Find("SpecifiedLegalOrganization/ID"); n != nil {
		name := who + " legal registration identifier (" + legalBT + ")"
		if rest, ok := strings.CutPrefix(n.Trim(), "CF:"); ok {
			k.cf(n, rest, name+" after the prefix CF:")
		} else {
			check(n, n.Trim(), strings.TrimSpace(n.Attr("schemeID")), name)
		}
	}
	for _, n := range party.FindAll("URIUniversalCommunication/URIID") {
		check(n, n.Trim(), strings.TrimSpace(n.Attr("schemeID")), who+" electronic address ("+uriBT+")")
	}
}

// piva checks that v, the value of n named name in the messages, is a
// Partita IVA.
func (k *checker) piva(n *xmltree.Node, v, name string) {
	if v == "" {
		return
	}
	if why := pivaFormat(v); why != "" {
		k.c.Fail(RulePIVAFormat, rules.Warning, n, "[%s] The %s should be a Partita IVA: %s; value: %q.", RulePIVAFormat, name, why, v)
		return
	}
	if want := pivaCheckDigit(v[:10]); v[10] != want {
		k.c.Fail(RulePIVAKey, rules.Warning, n, "[%s] The check digit of the Partita IVA in the %s should be %c, not %c; value: %q.", RulePIVAKey, name, want, v[10], v)
	}
}

// cf checks that v, the value of n named name in the messages, is a Codice
// Fiscale.
func (k *checker) cf(n *xmltree.Node, v, name string) {
	if v == "" {
		return
	}
	if why := cfFormat(v); why != "" {
		k.c.Fail(RuleCFFormat, rules.Warning, n, "[%s] The %s should be a Codice Fiscale: %s; value: %q.", RuleCFFormat, name, why, v)
		return
	}
	if want := cfCheck(v); v[len(v)-1] != want {
		k.c.Fail(RuleCFKey, rules.Warning, n, "[%s] The check character of the Codice Fiscale in the %s should be %c, not %c; value: %q.", RuleCFKey, name, want, v[len(v)-1], v)
	}
}

// fits reports whether the decimal text v, written with at least minFrac
// decimals, has at most size characters, its sign included, and at most
// maxFrac decimals: the size of the FatturaPA types the SDI maps the value
// to (Amount2DecimalType, Amount8DecimalType, QuantitaType). Leading zeros
// of the integer part do not count. A text that is not a decimal fits: the
// EN 16931 rules report it.
func fits(v string, size, minFrac, maxFrac int) bool {
	s := strings.TrimPrefix(strings.TrimPrefix(v, "+"), "-")
	sign := len(v) - len(s)
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" && frac == "" || intPart != "" && !digits(intPart) || frac != "" && !digits(frac) {
		return true
	}
	if len(frac) > maxFrac {
		return false
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	return sign+len(intPart)+1+max(len(frac), minFrac) <= size
}

// houseNumber reports whether v, BT-36 of an Italian seller, gives a house
// number (it holds a digit) or the acronym s.n.c. (senza numero civico),
// written with or without dots, in any case.
func houseNumber(v string) bool {
	if strings.ContainsAny(v, "0123456789") {
		return true
	}
	return strings.EqualFold(strings.NewReplacer(".", "", " ", "").Replace(v), "snc")
}

// ipaCode reports whether v is an IPA code: 6 upper-case letters and digits.
func ipaCode(v string) bool {
	if len(v) != 6 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if c := v[i]; !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// latin reports whether every character of s is in the Unicode blocks Basic
// Latin and Latin-1 Supplement, the characters of the FatturaPA strings.
func latin(s string) bool {
	for _, r := range s {
		if r > 0xFF {
			return false
		}
	}
	return true
}

func alnum(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func letter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// length returns the number of characters of s.
func length(s string) int { return utf8.RuneCountInString(s) }

// or returns n, or alt when n is nil.
func or(n, alt *xmltree.Node) *xmltree.Node {
	if n != nil {
		return n
	}
	return alt
}
