// Package ubl implements the validation of UBL 2.1 invoices and credit notes
// against the EN 16931 validation artefacts of CEN TC 434 for UBL (release
// Release).
//
// A UBL document is validated in two parts:
//
//   - View builds a CII view of the document, on which the rule sets written
//     for CII run: the business rules of package en16931 (whose identifiers
//     and tests are the same in both syntaxes, with the differences of the
//     UBL artefacts handled there) and the Chorus Pro rules. Their findings
//     are located in the UBL document. The French, German and Peppol rules,
//     whose UBL schematrons have their own identifiers and contexts, run on
//     the UBL document that the view carries (rules.Doc.UBL);
//   - Check runs, on the UBL document itself, the rules whose context is UBL
//     syntax: the UBL syntax rules UBL-CR (elements and attributes that
//     EN 16931 does not use), UBL-SR (cardinalities) and UBL-DT (data
//     types), the code lists BR-CL (the UBL artefacts check other elements,
//     and have their own lists for the document type and VAT point date
//     codes) and the two business rules whose context differs, BR-CO-03 and
//     BR-53.
//
// An invoice that declares the Dutch G-account extension in its
// specification identifier (see rules.Doc.GAccount) is checked against the
// CEN syntax rules as the artefacts of the extension modify them: without
// UBL-CR-411, UBL-CR-453 and UBL-CR-459.
//
// An EXTENDED-CTC-FR invoice is checked against the rules of the FNFE-MPE
// schematron EXTENDED-CTC-FR-UBL.sch instead of the CEN ones (see ctcfr.go
// and CTCFRCatalog).
//
// Severities are the official ones (fatal: Error, warning: Warning), with the
// levels of the KoSIT validator configuration for XRechnung invoices (see
// en16931.Level).
package ubl

import (
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/en16931"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

//go:generate go run ./gen -o catalog_gen.go
//go:generate go run ./gen -ctcfr -o ctcfr_gen.go

// Check runs the UBL rules on c.Doc.UBL and records the findings in c.Report.
// It does nothing for a CII document, nor for a profile that is not an
// EN 16931 invoice.
func Check(c *rules.Checker) {
	d := c.Doc
	if d == nil || d.UBL == nil {
		return
	}
	if p := d.Profile; p != invoice.ProfileUnknown && !p.EN16931() {
		return
	}
	k := &checker{c: c, d: d, root: d.UBL, credit: d.UBL.Name == "CreditNote", ctcfr: d.Profile == invoice.ProfileExtendedCTCFR}
	k.document()
	k.business()
	if k.ctcfr {
		k.ctcfrDocument()
	}
	k.root.Walk(func(n *xmltree.Node) bool {
		k.businessRules(n)
		k.codeLists(n)
		k.contextRules(n)
		k.dataType(n)
		if k.ctcfr {
			k.ctcfrNode(n)
		}
		return true
	})
	k.rootRules()
	k.attributes()
}

// checker holds the state of one run.
type checker struct {
	c      *rules.Checker
	d      *rules.Doc
	root   *xmltree.Node
	credit bool
	ctcfr  bool // EXTENDED-CTC-FR: the rules of the FNFE-MPE schematron (ctcfr.go)

	// EXTENDED-CTC-FR, computed once by ctcfrInit.
	invoiceIDs []string               // /*/cbc:ID ($invoiceID)
	inCalc     map[*xmltree.Node]bool // lines summed in the totals ($lineInCalul)
}

// severity maps the rule identifiers of Catalog to their severity.
var severity = sync.OnceValue(func() map[string]rules.Severity {
	m := make(map[string]rules.Severity, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Severity
	}
	return m
})

// description maps the rule identifiers of Catalog to their description.
var description = sync.OnceValue(func() map[string]string {
	m := make(map[string]string, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Description
	}
	return m
})

// fail records a finding for rule id located at n (the root when nil). For an
// EXTENDED-CTC-FR invoice, the rules that the schematron removes are not
// reported, and the others have its identifiers and severities.
func (k *checker) fail(id string, n *xmltree.Node, format string, args ...any) {
	if k.ctcfr {
		if r, ok := ctcfrRenamed()[id]; ok {
			id = r
		}
		sev, ok := ctcfrSeverity()[id]
		if !ok {
			return
		}
		if n == nil {
			n = k.root
		}
		k.c.Fail(id, sev, n, format, args...)
		return
	}
	sev, ok := severity()[id]
	if !ok {
		sev = rules.Error // BR-CL, BR-CO-03, BR-53: fatal
	}
	if n == nil {
		n = k.root
	}
	k.c.Fail(id, en16931.Level(k.d, id, sev), n, format, args...)
}

// desc returns the official description of rule id: that of the
// EXTENDED-CTC-FR schematron for an EXTENDED-CTC-FR invoice.
func (k *checker) desc(id string) string {
	if k.ctcfr {
		return ctcfrDescription()[id]
	}
	return description()[id]
}

// failDesc records a finding whose message is the official description of
// the rule.
func (k *checker) failDesc(id string, n *xmltree.Node) {
	k.fail(id, n, "%s", k.desc(id))
}

// is reports whether n is the UBL element space:name, space being "cac",
// "cbc" or "ext".
func is(n *xmltree.Node, prefix, name string) bool {
	if n == nil || n.Name != name {
		return false
	}
	switch prefix {
	case "cac":
		return n.Space == NSCac
	case "cbc":
		return n.Space == NSCbc
	case "ext":
		return n.Space == NSExt
	}
	return false
}

// parentIs reports whether the parent of n is the UBL element prefix:name.
func parentIs(n *xmltree.Node, prefix, name string) bool {
	return n != nil && is(n.Parent, prefix, name)
}

// elems returns the children of n that are the UBL element prefix:name.
func elems(n *xmltree.Node, prefix, name string) []*xmltree.Node {
	var out []*xmltree.Node
	for _, c := range kids(n) {
		if is(c, prefix, name) {
			out = append(out, c)
		}
	}
	return out
}

// find returns the elements at a path of prefixed names relative to n
// ("cac:PartyName/cbc:Name").
func find(n *xmltree.Node, path string) []*xmltree.Node {
	nodes := []*xmltree.Node{n}
	for _, seg := range strings.Split(path, "/") {
		prefix, name, _ := strings.Cut(seg, ":")
		var next []*xmltree.Node
		for _, m := range nodes {
			next = append(next, elems(m, prefix, name)...)
		}
		nodes = next
	}
	return nodes
}

// boolValue returns the value of an xs:boolean and whether s is one.
func boolValue(s string) (value, ok bool) {
	switch strings.TrimSpace(s) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

// chargeIs reports whether an allowance or charge has a cbc:ChargeIndicator
// equal to want (cbc:ChargeIndicator = true() or false()).
func chargeIs(ac *xmltree.Node, want bool) bool {
	for _, c := range elems(ac, "cbc", "ChargeIndicator") {
		if v, ok := boolValue(c.Text); ok && v == want {
			return true
		}
	}
	return false
}

// document checks BR-CO-03 and BR-53, whose context is the document in the
// UBL artefacts.
func (k *checker) document() {
	r := k.root
	if tp := elems(r, "cbc", "TaxPointDate"); len(tp) > 0 && len(find(r, "cac:InvoicePeriod/cbc:DescriptionCode")) > 0 {
		k.fail("BR-CO-03", tp[0], "the invoice has both a value added tax point date (BT-7) and a value added tax point date code (BT-8)")
	}
	var amounts []*xmltree.Node
	r.Walk(func(n *xmltree.Node) bool {
		if is(n, "cbc", "TaxAmount") && parentIs(n, "cac", "TaxTotal") {
			amounts = append(amounts, n)
		}
		return true
	})
	for _, tc := range elems(r, "cbc", "TaxCurrencyCode") {
		ok := false
		for _, a := range amounts {
			if v, has := a.LookupAttr("currencyID"); has && v == tc.Text {
				ok = true
				break
			}
		}
		if !ok {
			k.fail("BR-53", tc, "VAT accounting currency code (BT-6) %q is given but there is no invoice total VAT amount (cac:TaxTotal/cbc:TaxAmount) in that currency (BT-111)", strings.TrimSpace(tc.Text))
			return // one assertion on the document
		}
	}
}

// codeList describes the subject and the list of a BR-CL rule, for messages.
type codeList struct {
	ok   func(string) bool
	what string
	list string
}

var codeListRules = sync.OnceValue(func() map[string]codeList {
	return map[string]codeList{
		"BR-CL-03": {codes.Currency, "currencyID", "ISO 4217 alpha-3"},
		"BR-CL-04": {codes.Currency, "invoice currency code (BT-5)", "ISO 4217 alpha-3"},
		"BR-CL-05": {codes.Currency, "VAT accounting currency code (BT-6)", "ISO 4217 alpha-3"},
		"BR-CL-06": {func(s string) bool { return inList(taxPointDateCodes, s) }, "value added tax point date code (BT-8)", "UNTDID 2005 restricted to 3, 35 and 432"},
		"BR-CL-07": {codes.ReferenceType, "object identifier scheme (BT-18)", "UNTDID 1153"},
		"BR-CL-10": {codes.ICD, "identifier scheme", "ISO 6523 ICD"},
		"BR-CL-11": {codes.ICD, "identifier scheme", "ISO 6523 ICD"},
		"BR-CL-13": {codes.ItemClassification, "item classification scheme (BT-158)", "UNTDID 7143"},
		"BR-CL-14": {codes.Country, "country code", "ISO 3166-1 alpha-2"},
		"BR-CL-15": {codes.Country, "item country of origin (BT-159)", "ISO 3166-1 alpha-2"},
		"BR-CL-16": {codes.PaymentMeans, "payment means type code (BT-81)", "UNTDID 4461"},
		"BR-CL-17": {codes.VATCategory, "allowance or charge VAT category code", "UNTDID 5305 (EN 16931 subset)"},
		"BR-CL-18": {codes.VATCategory, "VAT category code", "UNTDID 5305 (EN 16931 subset)"},
		"BR-CL-19": {codes.AllowanceReason, "allowance reason code", "UNTDID 5189"},
		"BR-CL-20": {codes.ChargeReason, "charge reason code", "UNTDID 7161"},
		"BR-CL-21": {codes.ICD, "item standard identifier scheme (BT-157)", "ISO 6523 ICD"},
		"BR-CL-22": {codes.VATEX, "VAT exemption reason code (BT-121)", "CEF VATEX"},
		"BR-CL-23": {codes.Unit, "unit code", "UN/ECE Recommendations 20 and 21"},
		"BR-CL-24": {codes.MIME, "attached document MIME code (BT-125)", "the accepted MIME codes"},
		"BR-CL-25": {codes.EAS, "electronic address scheme (BT-34, BT-49)", "CEF EAS"},
		"BR-CL-26": {codes.ICD, "deliver to location identifier scheme (BT-71)", "ISO 6523 ICD"},
	}
})

// inList reports whether s is one of the codes of list (" a b c ").
func inList(list, s string) bool { return strings.Contains(list, " "+s+" ") }

// code returns normalize-space(s) and whether it contains no space: the code
// list rules accept a value when, once trimmed, it is a single code.
func code(s string) (string, bool) {
	v := normalize(s)
	return v, !strings.Contains(v, " ")
}

// check checks value against the code list of rule.
func (k *checker) check(rule string, n *xmltree.Node, value string) {
	if k.ctcfr {
		k.ctcfrCheck(rule, n, value)
		return
	}
	r := codeListRules()[rule]
	v, ok := value, true
	if rule != "BR-CL-24" {
		v, ok = code(value)
	}
	if ok && r.ok(v) {
		return
	}
	k.fail(rule, n, "%s %q is not in %s", r.what, strings.TrimSpace(value), r.list)
}

// attrCheck checks the attribute of n against the code list of rule, when n
// has the attribute.
func (k *checker) attrCheck(rule string, n *xmltree.Node, attr string) {
	if v, ok := n.LookupAttr(attr); ok {
		k.check(rule, n, v)
	}
}

// docTypeCode130 reports whether a document reference has a document type
// code 130 (cbc:DocumentTypeCode = '130').
func docTypeCode130(ref *xmltree.Node) bool {
	for _, c := range elems(ref, "cbc", "DocumentTypeCode") {
		if c.Text == "130" {
			return true
		}
	}
	return false
}

// hasAncestor reports whether an ancestor of n is the UBL element
// prefix:name.
func hasAncestor(n *xmltree.Node, prefix, name string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if is(p, prefix, name) {
			return true
		}
	}
	return false
}

// codeLists checks the code list rules whose context is n.
func (k *checker) codeLists(n *xmltree.Node) {
	if n.Space != NSCbc {
		return
	}
	switch n.Name {
	case "InvoiceTypeCode", "CreditNoteTypeCode":
		list, what := invoiceTypeCodes, "a UBL Invoice"
		if n.Name == "CreditNoteTypeCode" {
			list, what = creditNoteTypeCodes, "a UBL CreditNote"
		}
		if k.ctcfr {
			list = ctcfrLists()["BR-CL-01"]
			if n.Name == "CreditNoteTypeCode" {
				list = ctcfrLists()["BR-CL-01/credit"]
			}
		}
		if v, ok := code(n.Text); !ok || !inList(list, v) {
			k.fail("BR-CL-01", n, "invoice type code (BT-3) %q is not in UNTDID 1001 (the codes of %s)", strings.TrimSpace(n.Text), what)
		}
	case "Amount", "BaseAmount", "PriceAmount", "TaxAmount", "TaxableAmount", "LineExtensionAmount", "TaxExclusiveAmount",
		"TaxInclusiveAmount", "AllowanceTotalAmount", "ChargeTotalAmount", "PrepaidAmount", "PayableRoundingAmount", "PayableAmount":
		v, _ := n.LookupAttr("currencyID")
		k.check("BR-CL-03", n, v)
	case "DocumentCurrencyCode":
		k.check("BR-CL-04", n, n.Text)
	case "TaxCurrencyCode":
		k.check("BR-CL-05", n, n.Text)
	case "DescriptionCode":
		if parentIs(n, "cac", "InvoicePeriod") {
			k.check("BR-CL-06", n, n.Text)
		}
	case "Note":
		if n.Parent == k.root {
			k.noteSubject(n)
		}
	case "ID":
		p := n.Parent
		switch {
		case (is(p, "cac", "AdditionalDocumentReference") || is(p, "cac", "DocumentReference")) && docTypeCode130(p):
			k.attrCheck("BR-CL-07", n, "schemeID")
		case is(p, "cac", "PartyIdentification"):
			if v, ok := n.LookupAttr("schemeID"); ok {
				c, single := code(v)
				sepa := single && c == "SEPA" && (hasAncestor(n, "cac", "AccountingSupplierParty") || hasAncestor(n, "cac", "PayeeParty"))
				if !sepa {
					k.check("BR-CL-10", n, v)
				}
			}
		case is(p, "cac", "StandardItemIdentification"):
			k.attrCheck("BR-CL-21", n, "schemeID")
		case is(p, "cac", "DeliveryLocation"):
			k.attrCheck("BR-CL-26", n, "schemeID")
		case is(p, "cac", "TaxCategory"):
			k.check("BR-CL-17", n, n.Text)
		case is(p, "cac", "ClassifiedTaxCategory"):
			k.check("BR-CL-18", n, n.Text)
		}
	case "CompanyID":
		if parentIs(n, "cac", "PartyLegalEntity") {
			k.attrCheck("BR-CL-11", n, "schemeID")
		}
	case "ItemClassificationCode":
		if parentIs(n, "cac", "CommodityClassification") {
			k.attrCheck("BR-CL-13", n, "listID")
		}
	case "IdentificationCode":
		switch {
		case parentIs(n, "cac", "Country"):
			k.check("BR-CL-14", n, n.Text)
		case parentIs(n, "cac", "OriginCountry"):
			k.check("BR-CL-15", n, n.Text)
		}
	case "PaymentMeansCode":
		if parentIs(n, "cac", "PaymentMeans") {
			k.check("BR-CL-16", n, n.Text)
		}
	case "AllowanceChargeReasonCode":
		if p := n.Parent; is(p, "cac", "AllowanceCharge") {
			if chargeIs(p, false) {
				k.check("BR-CL-19", n, n.Text)
			} else if chargeIs(p, true) {
				if k.ctcfr {
					k.ctcfrReasonCode(n)
				} else {
					k.check("BR-CL-20", n, n.Text)
				}
			}
		}
	case "TaxExemptionReasonCode":
		k.check("BR-CL-22", n, n.Text)
	case "InvoicedQuantity", "BaseQuantity", "CreditedQuantity":
		k.attrCheck("BR-CL-23", n, "unitCode")
	case "EmbeddedDocumentBinaryObject":
		k.attrCheck("BR-CL-24", n, "mimeCode")
	case "EndpointID":
		k.attrCheck("BR-CL-25", n, "schemeID")
	}
}

// noteSubject checks BR-CL-08 on a note: when it contains "#", the text
// between the first two "#", if it has three characters, must be found in
// the list of subject codes (the official test looks for a substring of the
// list).
func (k *checker) noteSubject(n *xmltree.Node) {
	_, after, found := strings.Cut(n.Text, "#")
	if !found {
		return
	}
	subject, _, found := strings.Cut(after, "#")
	if !found {
		return // substring-before returns "" without a second "#"
	}
	list := noteSubjectCodes
	if k.ctcfr {
		list = ctcfrLists()["BR-CL-08"]
	}
	if len([]rune(subject)) != 3 || strings.Contains(list, subject) {
		return
	}
	k.fail("BR-CL-08", n, "invoice note subject code (BT-21) %q is not in UNTDID 4451", subject)
}
