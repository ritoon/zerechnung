package ubl

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// crScope is the scope of a UBL-CR assertion of crRules.
type crScope uint8

// Scopes of the UBL-CR assertions.
const (
	crRoot crScope = iota + 1 // not(path), path relative to the document element
	crLine                    // not((cac:InvoiceLine|cac:CreditNoteLine)/path)
	crAny                     // not(//path): anywhere in the document
)

// crRule is a UBL-CR assertion: the element or attribute at path must not
// exist.
type crRule struct {
	id    string
	scope crScope
	path  string
}

// gaccountDropped lists the UBL-CR assertions that the artefacts of the Dutch
// G-account extension leave out of the CEN syntax rules
// (si-ubl-2.0-ext-gaccount/EN16931-syntax-modified.sch), since the extension
// uses these elements: the payment means identifier (NL-GA-04), the payment
// means reference (NL-GA-02) and the amount (NL-GA-03) of the payment terms.
// They are not checked on an invoice that declares the extension (see
// rules.Doc.GAccount).
var gaccountDropped = map[string]bool{
	"UBL-CR-411": true, // cac:PaymentMeans/cbc:ID
	"UBL-CR-453": true, // cac:PaymentTerms/cbc:PaymentMeansID
	"UBL-CR-459": true, // cac:PaymentTerms/cbc:Amount
}

// selectPath returns the nodes at path relative to the nodes from: a path of
// prefixed element names ("cac:PartyName/cbc:Name"), optionally ending with
// an attribute step ("@schemeID"), which keeps the elements that carry the
// attribute.
func selectPath(from []*xmltree.Node, path string) []*xmltree.Node {
	nodes := from
	for _, seg := range strings.Split(path, "/") {
		var next []*xmltree.Node
		if attr, ok := strings.CutPrefix(seg, "@"); ok {
			for _, n := range nodes {
				if hasAttr(n, attr) {
					next = append(next, n)
				}
			}
		} else {
			prefix, name, _ := strings.Cut(seg, ":")
			for _, n := range nodes {
				next = append(next, elems(n, prefix, name)...)
			}
		}
		nodes = next
		if len(nodes) == 0 {
			break
		}
	}
	return nodes
}

// hasAttr reports whether n has the attribute name in no namespace.
func hasAttr(n *xmltree.Node, name string) bool {
	for _, a := range n.Attrs {
		if a.Name == name && a.Prefix == "" {
			return true
		}
	}
	return false
}

// attr returns the attribute name in no namespace of n.
func attr(n *xmltree.Node, name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name && a.Prefix == "" {
			return a.Value, true
		}
	}
	return "", false
}

// lines returns the invoice or credit note lines of the document.
func (k *checker) lines() []*xmltree.Node {
	var out []*xmltree.Node
	for _, c := range kids(k.root) {
		if is(c, "cac", "InvoiceLine") || is(c, "cac", "CreditNoteLine") {
			out = append(out, c)
		}
	}
	return out
}

// rootRules checks the assertions whose context is the document element: the
// UBL-CR rules and the UBL-SR rules on the document.
func (k *checker) rootRules() {
	r := k.root
	root := []*xmltree.Node{r}
	lines := k.lines()
	var all []*xmltree.Node
	r.Walk(func(n *xmltree.Node) bool {
		all = append(all, n)
		return true
	})

	k.crSpecial(r, all)
	if k.ctcfr {
		k.ctcfrSyntax(lines)
	}
	gaccount := k.d.GAccount()
	crs := crRules
	if k.ctcfr {
		crs = ctcfrCRRules
	}
	for _, cr := range crs {
		if gaccount && gaccountDropped[cr.id] {
			continue
		}
		var found []*xmltree.Node
		switch cr.scope {
		case crRoot:
			found = selectPath(root, cr.path)
		case crLine:
			found = selectPath(lines, cr.path)
		case crAny:
			if strings.HasPrefix(cr.path, "@") {
				found = selectPath(all, cr.path) // //@a: any element with the attribute
			} else {
				for _, n := range all {
					if found = selectPath([]*xmltree.Node{n}, cr.path); len(found) > 0 {
						break
					}
				}
			}
		}
		if len(found) > 0 {
			k.fail(cr.id, found[0], "%s (%s)", k.desc(cr.id), cr.path)
		}
	}

	// UBL-SR rules of the document.
	atMost := func(id string, n int, nodes []*xmltree.Node) {
		if len(nodes) > n {
			k.fail(id, nodes[n], "%s (%d found)", k.desc(id), len(nodes))
		}
	}
	sel := func(path string) []*xmltree.Node { return selectPath(root, path) }
	atMost("UBL-SR-01", 1, sel("cac:ContractDocumentReference/cbc:ID"))
	atMost("UBL-SR-02", 1, sel("cac:ReceiptDocumentReference/cbc:ID"))
	atMost("UBL-SR-03", 1, sel("cac:DespatchDocumentReference/cbc:ID"))
	var invoiced []*xmltree.Node
	for _, ref := range sel("cac:AdditionalDocumentReference") {
		if docTypeCode130(ref) {
			invoiced = append(invoiced, elems(ref, "cbc", "ID")...)
		}
	}
	atMost("UBL-SR-04", 1, invoiced)
	atMost("UBL-SR-05", 1, sel("cac:PaymentTerms/cbc:Note"))
	atMost("UBL-SR-08", 1, sel("cac:InvoicePeriod"))
	atMost("UBL-SR-09", 1, sel("cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName"))
	atMost("UBL-SR-10", 1, sel("cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name"))
	atMost("UBL-SR-11", 1, sel("cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID"))
	vat, other := taxSchemeCompanyIDs(sel("cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme"))
	atMost("UBL-SR-12", 1, vat)
	atMost("UBL-SR-13", 1, other)
	atMost("UBL-SR-14", 1, sel("cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm"))
	atMost("UBL-SR-15", 1, sel("cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName"))
	atMost("UBL-SR-16", 1, sel("cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification/cbc:ID"))
	atMost("UBL-SR-17", 1, sel("cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID"))
	vat, _ = taxSchemeCompanyIDs(sel("cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme"))
	atMost("UBL-SR-18", 1, vat)
	atMost("UBL-SR-24", 1, sel("cac:Delivery"))
	var sepa []*xmltree.Node
	for _, n := range all {
		if is(n, "cbc", "ID") && parentIs(n, "cac", "PartyIdentification") {
			if v, ok := attr(n, "schemeID"); ok && strings.ToUpper(v) == "SEPA" {
				sepa = append(sepa, n)
			}
		}
	}
	atMost("UBL-SR-29", 1, sepa)
	atMost("UBL-SR-39", 1, sel("cac:ProjectReference/cbc:ID"))
	atMost("UBL-SR-40", 1, sel("cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name"))
	var paymentIDs, meansCodes []*xmltree.Node
	for _, n := range all {
		switch {
		case is(n, "cbc", "PaymentID"):
			paymentIDs = append(paymentIDs, n)
		case is(n, "cbc", "PaymentMeansCode"):
			meansCodes = append(meansCodes, n)
		}
	}
	atMost("UBL-SR-44", 1, distinct(paymentIDs))
	atMost("UBL-SR-45", 1, sel("cac:PaymentMeans/cbc:PaymentDueDate"))
	atMost("UBL-SR-46", 1, selectPath(sel("cac:PaymentMeans/cbc:PaymentMeansCode"), "@name"))
	atMost("UBL-SR-47", 1, distinct(meansCodes))
	atMost("UBL-SR-49", 1, sel("cac:InvoicePeriod/cbc:DescriptionCode"))
	atMost("UBL-SR-54", 1, sel("cac:PaymentMeans/cac:CardAccount"))
	atMost("UBL-SR-55", 1, sel("cac:PaymentMeans/cac:PaymentMandate"))
	atMost("UBL-SR-56", 1, sel("cac:OriginatorDocumentReference/cbc:ID"))
	for _, path := range []string{
		"cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity",
		"cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity",
		"cac:PayeeParty/cac:PartyLegalEntity",
		"cac:TaxRepresentativeParty/cac:PartyLegalEntity",
	} {
		if nodes := sel(path); len(nodes) > 1 {
			k.fail("UBL-SR-57", nodes[1], "%s (%d %s)", k.desc("UBL-SR-57"), len(nodes), path)
			break
		}
	}
}

// crSpecial checks the UBL-CR assertions that are not a plain not(path).
func (k *checker) crSpecial(r *xmltree.Node, all []*xmltree.Node) {
	// UBL-CR-002: not(cbc:UBLVersionID) or cbc:UBLVersionID = '2.1'
	if v := elems(r, "cbc", "UBLVersionID"); len(v) > 0 {
		ok := false
		for _, n := range v {
			ok = ok || n.Text == "2.1"
		}
		if !ok {
			k.fail("UBL-CR-002", v[0], "%s (%q)", k.desc("UBL-CR-002"), v[0].Text)
		}
	}
	// UBL-CR-412: not(cac:PaymentMeans/cbc:PaymentDueDate) or ../cn:CreditNote
	if due := selectPath([]*xmltree.Node{r}, "cac:PaymentMeans/cbc:PaymentDueDate"); len(due) > 0 && !k.credit {
		k.failDesc("UBL-CR-412", due[0])
	}
	var c665, c666, c673 *xmltree.Node
	for _, n := range all {
		if !is(n, "cac", "AdditionalDocumentReference") {
			continue
		}
		codes := elems(n, "cbc", "DocumentTypeCode")
		other := len(codes) == 0 // cbc:DocumentTypeCode != '130' or not(cbc:DocumentTypeCode)
		for _, c := range codes {
			other = other || c.Text != "130"
		}
		if other && c665 == nil {
			if ids := selectPath([]*xmltree.Node{n}, "cbc:ID/@schemeID"); len(ids) > 0 {
				c665 = ids[0]
			}
		}
		if docTypeCode130(n) {
			if a := elems(n, "cac", "Attachment"); len(a) > 0 && c666 == nil {
				c666 = a[0]
			}
			if dd := elems(n, "cbc", "DocumentDescription"); len(dd) > 0 && c673 == nil {
				c673 = dd[0]
			}
		}
	}
	if c665 != nil {
		k.failDesc("UBL-CR-665", c665)
	}
	if c666 != nil {
		k.failDesc("UBL-CR-666", c666)
	}
	if c673 != nil {
		k.failDesc("UBL-CR-673", c673)
	}
}

// taxSchemeCompanyIDs returns the company identifiers of the party tax
// schemes whose tax scheme is VAT (cac:TaxScheme/upper-case(cbc:ID) = 'VAT')
// and those of the schemes whose tax scheme is another one (!= 'VAT').
func taxSchemeCompanyIDs(schemes []*xmltree.Node) (vat, other []*xmltree.Node) {
	for _, s := range schemes {
		isVAT, isOther := false, false
		for _, id := range selectPath([]*xmltree.Node{s}, "cac:TaxScheme/cbc:ID") {
			if strings.ToUpper(id.Text) == "VAT" {
				isVAT = true
			} else {
				isOther = true
			}
		}
		ids := elems(s, "cbc", "CompanyID")
		if isVAT {
			vat = append(vat, ids...)
		}
		if isOther {
			other = append(other, ids...)
		}
	}
	return vat, other
}

// distinct returns the first node of each distinct string value of nodes.
func distinct(nodes []*xmltree.Node) []*xmltree.Node {
	seen := map[string]bool{}
	var out []*xmltree.Node
	for _, n := range nodes {
		if !seen[n.Text] {
			seen[n.Text] = true
			out = append(out, n)
		}
	}
	return out
}

// contextRules checks the UBL-SR rules whose context is the element n.
func (k *checker) contextRules(n *xmltree.Node) {
	if n.Space != NSCac {
		return
	}
	one := []*xmltree.Node{n}
	atMost := func(id string, max int, path string) {
		if nodes := selectPath(one, path); len(nodes) > max {
			k.fail(id, nodes[max], "%s (%d found)", k.desc(id), len(nodes))
		}
	}
	switch n.Name {
	case "PostalAddress", "Address":
		atMost("UBL-SR-51", 1, "cac:AddressLine")
	case "Party":
		if parentIs(n, "cac", "AccountingSupplierParty") {
			atMost("UBL-SR-42", 2, "cac:PartyTaxScheme")
		}
	case "AdditionalDocumentReference":
		atMost("UBL-SR-33", 1, "cbc:DocumentDescription")
		codes := elems(n, "cbc", "DocumentTypeCode")
		ok := docTypeCode130(n) || len(codes) == 0 && len(selectPath(one, "cbc:ID/@schemeID")) == 0
		if !ok && k.credit {
			for _, c := range codes {
				ok = ok || c.Text == "50"
			}
		}
		if !ok {
			k.failDesc("UBL-SR-43", n)
		}
	case "Delivery":
		if k.ctcfr {
			// count(cac:DeliveryParty/cac:PartyName/cbc:Name) <= 1
			atMost("UBL-SR-25", 1, "cac:DeliveryParty/cac:PartyName/cbc:Name")
		} else if len(elems(n, "cac", "DeliveryParty")) > 0 {
			if names := selectPath(one, "cac:DeliveryParty/cac:PartyName/cbc:Name"); len(names) != 1 {
				k.fail("UBL-SR-25", n, "%s (%d found)", k.desc("UBL-SR-25"), len(names))
			}
		}
	case "AllowanceCharge":
		switch {
		case chargeIs(n, false):
			atMost("UBL-SR-30", 1, "cbc:AllowanceChargeReason")
		case chargeIs(n, true):
			atMost("UBL-SR-31", 1, "cbc:AllowanceChargeReason")
		}
	case "PartyTaxScheme":
		if len(selectPath(one, "cac:TaxScheme/cbc:ID")) == 0 || len(elems(n, "cbc", "CompanyID")) == 0 {
			k.failDesc("UBL-SR-53", n)
		}
	case "InvoiceLine", "CreditNoteLine":
		atMost("UBL-SR-34", 1, "cbc:Note")
		atMost("UBL-SR-35", 1, "cac:OrderLineReference/cbc:LineID")
		atMost("UBL-SR-36", 1, "cac:InvoicePeriod")
		atMost("UBL-SR-37", 1, "cac:Price/cac:AllowanceCharge/cbc:Amount")
		if k.ctcfr {
			k.ctcfrTaxCategories(n)
		} else if c := selectPath(one, "cac:Item/cac:ClassifiedTaxCategory"); len(c) != 1 {
			k.fail("UBL-SR-48", n, "%s (%d found)", k.desc("UBL-SR-48"), len(c))
		}
		atMost("UBL-SR-50", 1, "cac:Item/cbc:Description")
		atMost("UBL-SR-52", 1, "cac:DocumentReference")
	case "PayeeParty":
		// (count(...) <= 1) and ((cac:PartyName/cbc:Name) !=
		// (../cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName)):
		// the general comparison is false when either side is empty.
		names := selectPath(one, "cac:PartyName/cbc:Name")
		seller := selectPath([]*xmltree.Node{n.Parent}, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName")
		differs := false
		for _, a := range names {
			for _, b := range seller {
				differs = differs || a.Text != b.Text
			}
		}
		var ids []*xmltree.Node
		for _, id := range selectPath(one, "cac:PartyIdentification/cbc:ID") {
			if v, ok := attr(id, "schemeID"); ok && strings.ToUpper(v) != "SEPA" {
				ids = append(ids, id)
			}
		}
		counts := map[string]int{
			"UBL-SR-19": len(names),
			"UBL-SR-20": len(ids),
			"UBL-SR-21": len(selectPath(one, "cac:PartyLegalEntity/cbc:CompanyID")),
		}
		for _, id := range []string{"UBL-SR-19", "UBL-SR-20", "UBL-SR-21"} {
			if counts[id] > 1 || !differs {
				k.failDesc(id, n)
			}
		}
	case "PaymentMeans":
		atMost("UBL-SR-26", 1, "cbc:PaymentID")
		atMost("UBL-SR-27", 1, "cbc:PaymentMeansCode")
		atMost("UBL-SR-28", 1, "cac:PaymentMandate/cbc:ID")
		if len(selectPath(one, "cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch")) > 0 &&
			len(selectPath(one, "cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID")) != 1 {
			k.failDesc("UBL-SR-58", n)
		}
		if len(elems(n, "cac", "PayeeFinancialAccount")) > 0 && len(selectPath(one, "cac:PayeeFinancialAccount/cbc:ID")) != 1 {
			k.failDesc("UBL-SR-59", n)
		}
	case "BillingReference":
		if k.ctcfr && n.Parent != k.root {
			return // (/ubl:Invoice|/cn:CreditNote)/cac:BillingReference: not the line subtypes
		}
		atMost("UBL-SR-06", 1, "cac:InvoiceDocumentReference")
		if len(selectPath(one, "cac:InvoiceDocumentReference/cbc:ID")) == 0 {
			k.failDesc("UBL-SR-07", n)
		}
	case "TaxRepresentativeParty":
		atMost("UBL-SR-22", 1, "cac:PartyName/cbc:Name")
		atMost("UBL-SR-23", 1, "cac:PartyTaxScheme/cbc:CompanyID")
	case "TaxSubtotal":
		atMost("UBL-SR-32", 1, "cac:TaxCategory/cbc:TaxExemptionReason")
	}
}

// dataType checks the UBL-DT rules whose context is the element n.
func (k *checker) dataType(n *xmltree.Node) {
	switch {
	case strings.HasSuffix(n.Name, "Amount") && !strings.HasSuffix(n.Name, "PriceAmount") && !inPriceWithAllowance(n):
		// string-length(substring-after(., '.')) <= 2
		if _, frac, ok := strings.Cut(n.Text, "."); ok && len([]rune(frac)) > 2 {
			k.fail("UBL-DT-01", n, "%s (%q)", k.desc("UBL-DT-01"), n.Text)
		}
	case strings.HasSuffix(n.Name, "BinaryObject"):
		if !hasAttr(n, "mimeCode") {
			k.failDesc("UBL-DT-06", n)
		}
		if !hasAttr(n, "filename") {
			k.failDesc("UBL-DT-07", n)
		}
	}
}

// inPriceWithAllowance reports whether n has an ancestor cac:Price with a
// cac:AllowanceCharge (ancestor::cac:Price/cac:AllowanceCharge).
func inPriceWithAllowance(n *xmltree.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if is(p, "cac", "Price") && len(elems(p, "cac", "AllowanceCharge")) > 0 {
			return true
		}
	}
	return false
}

// forbiddenAttrs maps the UBL-DT rules not(//@name) to the attribute.
var forbiddenAttrs = []struct{ id, attr string }{
	{"UBL-DT-08", "schemeName"},
	{"UBL-DT-09", "schemeAgencyName"},
	{"UBL-DT-10", "schemeDataURI"},
	{"UBL-DT-11", "schemeURI"},
	{"UBL-DT-12", "format"},
	{"UBL-DT-13", "unitCodeListIdentifier"},
	{"UBL-DT-14", "unitCodeListAgencyIdentifier"},
	{"UBL-DT-15", "unitCodeListAgencyName"},
	{"UBL-DT-16", "listAgencyName"},
	{"UBL-DT-17", "listName"},
	{"UBL-DT-19", "languageID"},
	{"UBL-DT-20", "listURI"},
	{"UBL-DT-21", "listSchemeURI"},
	{"UBL-DT-22", "languageLocaleID"},
	{"UBL-DT-23", "uri"},
	{"UBL-DT-24", "currencyCodeListVersionID"},
	{"UBL-DT-25", "characterSetCode"},
	{"UBL-DT-26", "encodingCode"},
	{"UBL-DT-27", "schemeAgencyID"},
	{"UBL-DT-28", "listAgencyID"},
}

// attributes checks the UBL-DT rules on attributes, whose context is the
// document: the attributes of forbiddenAttrs must not be used, and @name only
// on cbc:PaymentMeansCode (UBL-DT-18).
func (k *checker) attributes() {
	first := map[string]*xmltree.Node{}
	var name *xmltree.Node
	k.root.Walk(func(n *xmltree.Node) bool {
		for _, a := range n.Attrs {
			if a.Prefix != "" {
				continue
			}
			if first[a.Name] == nil {
				first[a.Name] = n
			}
			if a.Name == "name" && name == nil && !is(n, "cbc", "PaymentMeansCode") {
				name = n
			}
		}
		return true
	})
	for _, f := range forbiddenAttrs {
		if n := first[f.attr]; n != nil {
			k.fail(f.id, n, "%s (@%s)", k.desc(f.id), f.attr)
		}
		if f.id == "UBL-DT-17" && name != nil {
			k.fail("UBL-DT-18", name, "%s (@name)", k.desc("UBL-DT-18"))
		}
	}
}
