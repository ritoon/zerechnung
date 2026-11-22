package ubl

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The rules of the FNFE-MPE schematron EXTENDED-CTC-FR-UBL.sch (France_RFE
// en16931.CTCFRRelease), which apply to an EXTENDED-CTC-FR invoice in UBL
// instead of the CEN ones (see CTCFRCatalog).
//
// The schematron keeps most CEN rules: package en16931 reports those of the
// model on the CII view (its table ctcfrUBLRules), this package the others.
// It removes some (fail drops the rules that CTCFRCatalog does not list:
// BR-22 to BR-28, BR-CO-04, BR-CO-10 to BR-CO-17, the rules 01 and 08 of the
// VAT categories, a few UBL-CR and UBL-SR rules...), renames two code lists
// (ctcfrRenamed), changes the lists of the code lists (ctcfrLists), the test
// of UBL-SR-25 (at most one deliver to party name, without requiring one)
// and of BR-CO-09 (the prefix of a VAT identifier is looked up as a
// substring of the list), and restricts UBL-SR-06, UBL-SR-07 and BR-55 to the
// preceding invoice references of the document, a cac:BillingReference of a
// line being a line subtype. It adds its own rules:
//
//   - the line subtypes (EXT-FR-FE-162, EXT-FR-FE-163): a line whose
//     cac:BillingReference refers to the invoice itself
//     (cac:InvoiceDocumentReference/cbc:ID = /*/cbc:ID) is a sub-line, of the
//     subtype of its cbc:DocumentStatusCode (DETAIL, GROUP or INFORMATION),
//     whose parent line is cac:BillingReferenceLine/cbc:ID (BR-FREXT-05 to
//     BR-FREXT-12). Only the plain and DETAIL lines ($lineInCalul, see
//     ctcfrInit) are summed in the totals and the VAT breakdowns, and must
//     have a quantity, a net amount, a price and a VAT category (BR-FREXT-22
//     to BR-FREXT-26, BR-FREXT-CO-04, UBL-FREXT-SR-48);
//   - the VAT breakdowns per category, rate and exemption reason
//     (BR-FREXT-*-08: with or without the exemption reason, *-08ini: without,
//     *-08rev: with; BR-FREXT-S-09) and the presence of the breakdown of each
//     category used (BR-FREXT-*-01), with a tolerance of 0.01 per summed
//     amount;
//   - the document totals with the same tolerance (BR-FREXT-CO-10 to
//     BR-FREXT-CO-15) and BR-CO-25 (payment terms or due date of an invoice
//     with an amount due);
//   - the item attributes (BR-FREXT-54-1, BR-FREXT-54-2, BR-FREXT-04), the
//     delivery terms (UBL-FREXT-CR-410, UBL-FREXT-CR-645, BR-FREXT-CL-27),
//     the tax exchange rate (CII-FREXT-CR-490) and the reason codes of the
//     charges by list (BR-FREXT-CL-20, BR-FREXT-CL-20-TAX,
//     BR-FREXT-CL-20-LISTID).
//
// The arithmetic is that of the schematron, see xnum.

// ctcfrSeverity maps the rules of CTCFRCatalog to their severity.
var ctcfrSeverity = sync.OnceValue(func() map[string]rules.Severity {
	m := make(map[string]rules.Severity, len(CTCFRCatalog))
	for _, r := range CTCFRCatalog {
		m[r.ID] = r.Severity
	}
	return m
})

// ctcfrDescription maps the rules of CTCFRCatalog to their message.
var ctcfrDescription = sync.OnceValue(func() map[string]string {
	m := make(map[string]string, len(CTCFRCatalog))
	for _, r := range CTCFRCatalog {
		m[r.ID] = r.Description
	}
	return m
})

// ctcfrRenamed maps the CEN code list rules that the schematron keeps, with
// its own list, under another identifier.
var ctcfrRenamed = sync.OnceValue(func() map[string]string {
	return map[string]string{
		"BR-CL-10": "BR-FREXT-CL-10",
		"BR-CL-24": "BR-FREXT-CL-24",
	}
})

// ctcfrCodeListText gives the subject and the list name of the code list
// rules of the schematron that the CEN artefacts do not have, for messages.
var ctcfrCodeListText = sync.OnceValue(func() map[string][2]string {
	return map[string][2]string{
		"BR-FREXT-CL-10":        {"identifier scheme", "ISO 6523 ICD, BY and SE"},
		"BR-FREXT-CL-20":        {"charge reason code", "UNTDID 7161"},
		"BR-FREXT-CL-20-TAX":    {"charge reason code of list 5153", "UNTDID 5153 without VAT"},
		"BR-FREXT-CL-20-LISTID": {"charge reason code list identifier", "the lists 7161 and 5153"},
		"BR-FREXT-CL-24":        {"attached document MIME code (BT-125)", "the MIME codes of the EXTENDED-CTC-FR schematron"},
		"BR-FREXT-CL-27":        {"delivery terms code", "the Incoterms of the EXTENDED-CTC-FR schematron"},
		"BR-FREXT-04":           {"item attribute code", "UNTDID 6313 and the Factur-X extension"},
	}
})

// ctcfrCheck checks value against the list of the schematron for rule, or
// for the rule that replaces it: normalize-space(value) must contain no space
// and be in the list. BR-CL-22 looks the code up in upper case and
// BR-FREXT-CL-24 compares the MIME code exactly.
func (k *checker) ctcfrCheck(rule string, n *xmltree.Node, value string) {
	if r, ok := ctcfrRenamed()[rule]; ok {
		rule = r
	}
	list, ok := ctcfrLists()[rule]
	if !ok {
		return
	}
	v := normalize(value)
	switch rule {
	case "BR-FREXT-CL-24":
		ok = !strings.Contains(value, " ") && strings.Contains(list, " "+value+" ")
	case "BR-CL-22":
		ok = !strings.Contains(v, " ") && strings.Contains(list, " "+normalize(strings.ToUpper(value))+" ")
	default:
		ok = !strings.Contains(v, " ") && strings.Contains(list, " "+v+" ")
	}
	if ok {
		return
	}
	what, name := codeListRules()[rule].what, codeListRules()[rule].list
	if t, ok := ctcfrCodeListText()[rule]; ok {
		what, name = t[0], t[1]
	}
	k.fail(rule, n, "%s %q is not in %s", what, strings.TrimSpace(value), name)
}

// ctcfrReasonCode checks the reason code n of a charge. The rules of the
// schematron, in that order, have the contexts
// cac:AllowanceCharge[charge]/cbc:AllowanceChargeReasonCode[@listID='7161'
// or not(@listID)] (BR-FREXT-CL-20), [@listID='5153'] (BR-FREXT-CL-20-TAX)
// and [@listID != ”] (BR-FREXT-CL-20-LISTID): the first one that matches
// wins, and a code with an empty list identifier is not checked.
func (k *checker) ctcfrReasonCode(n *xmltree.Node) {
	switch listID, ok := attr(n, "listID"); {
	case !ok || listID == "7161":
		k.ctcfrCheck("BR-FREXT-CL-20", n, n.Text)
	case listID == "5153":
		k.ctcfrCheck("BR-FREXT-CL-20-TAX", n, n.Text)
	case listID != "":
		k.ctcfrCheck("BR-FREXT-CL-20-LISTID", n, listID)
	}
}

// ctcfrInit computes $invoiceID and, for each line, $lineInCalul: whether
// the line is a plain or DETAIL line, summed in the totals and the VAT
// breakdowns (not(cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID =
// $invoiceID) or
// cac:BillingReference[cac:InvoiceDocumentReference/cbc:ID =
// $invoiceID]/cac:InvoiceDocumentReference/cbc:DocumentStatusCode =
// 'DETAIL').
func (k *checker) ctcfrInit() {
	for _, id := range elems(k.root, "cbc", "ID") {
		k.invoiceIDs = append(k.invoiceIDs, id.Text)
	}
	k.inCalc = map[*xmltree.Node]bool{}
	for _, l := range k.lines() {
		refs := k.selfRefs(l)
		k.inCalc[l] = len(refs) == 0 || statusIs(refs, "DETAIL")
	}
}

// isInvoiceID reports whether s is the text of an invoice number ($invoiceID).
func (k *checker) isInvoiceID(s string) bool {
	for _, v := range k.invoiceIDs {
		if s == v {
			return true
		}
	}
	return false
}

// selfRefs returns the cac:BillingReference children of line that refer to
// the invoice itself: cac:BillingReference[cac:InvoiceDocumentReference/cbc:ID
// = $invoiceID].
func (k *checker) selfRefs(line *xmltree.Node) []*xmltree.Node {
	var out []*xmltree.Node
	for _, ref := range elems(line, "cac", "BillingReference") {
		for _, id := range find(ref, "cac:InvoiceDocumentReference/cbc:ID") {
			if k.isInvoiceID(id.Text) {
				out = append(out, ref)
				break
			}
		}
	}
	return out
}

// statusIs reports whether a cac:InvoiceDocumentReference/cbc:DocumentStatusCode
// of the billing references refs is v.
func statusIs(refs []*xmltree.Node, v string) bool {
	for _, ref := range refs {
		for _, s := range find(ref, "cac:InvoiceDocumentReference/cbc:DocumentStatusCode") {
			if s.Text == v {
				return true
			}
		}
	}
	return false
}

// selfStatus reports whether a cac:InvoiceDocumentReference of line that
// refers to the invoice has the status v
// (cac:BillingReference/cac:InvoiceDocumentReference[cbc:ID =
// $invoiceID]/cbc:DocumentStatusCode = v).
func (k *checker) selfStatus(line *xmltree.Node, v string) bool {
	for _, idr := range find(line, "cac:BillingReference/cac:InvoiceDocumentReference") {
		self := false
		for _, id := range elems(idr, "cbc", "ID") {
			self = self || k.isInvoiceID(id.Text)
		}
		if !self {
			continue
		}
		for _, s := range elems(idr, "cbc", "DocumentStatusCode") {
			if s.Text == v {
				return true
			}
		}
	}
	return false
}

// lineInCalc returns $lineInCalul for line (see ctcfrInit).
func (k *checker) lineInCalc(line *xmltree.Node) bool {
	if v, ok := k.inCalc[line]; ok {
		return v
	}
	refs := k.selfRefs(line)
	return len(refs) == 0 || statusIs(refs, "DETAIL")
}

// idIs reports whether normalize-space(cbc:ID) = code for n (the first
// identifier: the schematron raises an error for several).
func idIs(n *xmltree.Node, code string) bool {
	return normalize(text(first(n, "cbc:ID"))) == code
}

// vatScheme reports whether n has a cac:TaxScheme whose identifier is VAT
// (cac:TaxScheme/normalize-space(upper-case(cbc:ID)) = 'VAT').
func vatScheme(n *xmltree.Node) bool {
	for _, s := range elems(n, "cac", "TaxScheme") {
		if isVATScheme(s) {
			return true
		}
	}
	return false
}

// ctcfrCategory describes the rules of a VAT category in the schematron.
type ctcfrCategory struct {
	code string // VAT category code (BT-118)
	id   string // the rules are BR-FREXT-<id>-01, BR-FREXT-<id>-08...
	rate bool   // the breakdowns are per rate (cbc:Percent)
}

// ctcfrPresence lists the categories in the order of the rules
// BR-FREXT-*-01 in the schematron.
var ctcfrPresence = []ctcfrCategory{
	{"S", "S", true}, {"Z", "Z", true}, {"E", "E", true}, {"AE", "AE", true}, {"K", "IC", true},
	{"G", "G", true}, {"O", "O", false}, {"L", "AF", true}, {"M", "AG", true},
}

// ctcfrBreakdowns lists the categories in the order of the rules of the VAT
// breakdowns in the schematron: the first rule whose context matches a
// breakdown wins.
var ctcfrBreakdowns = []ctcfrCategory{
	{"AE", "AE", true}, {"E", "E", true}, {"G", "G", true}, {"K", "IC", true}, {"L", "AF", true},
	{"M", "AG", true}, {"O", "O", false}, {"S", "S", true}, {"Z", "Z", true},
}

// ctcfrDocument checks the rules of the schematron whose context is the
// document element: BR-FREXT-*-01 and BR-CO-25. It first computes the line
// subtypes (ctcfrInit), on which the other rules depend.
func (k *checker) ctcfrDocument() {
	k.ctcfrInit()
	r := k.root
	for _, c := range ctcfrPresence {
		allowances, lines, breakdowns := 0, 0, 0
		for _, t := range find(r, "cac:AllowanceCharge/cac:TaxCategory") {
			if idIs(t, c.code) {
				allowances++
			}
		}
		for _, l := range k.lines() {
			if !k.lineInCalc(l) {
				continue
			}
			for _, t := range find(l, "cac:Item/cac:ClassifiedTaxCategory") {
				if idIs(t, c.code) {
					lines++
				}
			}
		}
		for _, t := range find(r, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory") {
			if idIs(t, c.code) {
				breakdowns++
			}
		}
		if allowances+lines+breakdowns == 0 || breakdowns >= 1 && (allowances > 0 || lines > 0) {
			continue
		}
		if breakdowns == 0 {
			k.fail("BR-FREXT-"+c.id+"-01", nil, "VAT category %s is used by %d plain or DETAIL lines and %d document level allowances or charges but the VAT breakdown (BG-23) has none", c.code, lines, allowances)
		} else {
			k.fail("BR-FREXT-"+c.id+"-01", nil, "the VAT breakdown (BG-23) has VAT category %s but no plain or DETAIL line and no document level allowance or charge uses it", c.code)
		}
	}

	// BR-CO-25, of context /ubl:Invoice/cac:LegalMonetaryTotal/cbc:PayableAmount:
	// ((. > 0) and (exists(//cbc:DueDate) or exists(//cac:PaymentTerms/cbc:Note))) or (. <= 0).
	if r.Name != "Invoice" || r.Space != NSInvoice {
		return
	}
	due := false
	r.Walk(func(n *xmltree.Node) bool {
		due = due || is(n, "cbc", "DueDate") || is(n, "cbc", "Note") && parentIs(n, "cac", "PaymentTerms")
		return !due
	})
	for _, p := range find(r, "cac:LegalMonetaryTotal/cbc:PayableAmount") {
		v := untypedDouble(p.Text)
		if v > 0 && due || v <= 0 {
			continue
		}
		k.fail("BR-CO-25", p, "the amount due for payment (BT-115) %q is positive but the invoice has neither a payment due date (BT-9) nor payment terms (BT-20)", strings.TrimSpace(p.Text))
	}
}

// ctcfrNode checks the rules of the schematron whose context is the element
// n.
func (k *checker) ctcfrNode(n *xmltree.Node) {
	p := n.Parent
	switch n.Space {
	case NSCac:
		switch n.Name {
		case "LegalMonetaryTotal":
			k.ctcfrTotals(n)
		case "TaxCategory":
			if is(p, "cac", "TaxSubtotal") && parentIs(p, "cac", "TaxTotal") && p.Parent.Parent == k.root {
				k.ctcfrBreakdown(n)
			}
		case "InvoiceLine", "CreditNoteLine":
			k.ctcfrLine(n)
			k.ctcfrSubLine(n)
		case "AdditionalItemProperty":
			k.ctcfrItemProperty(n)
		case "PartyTaxScheme":
			if vatScheme(n) {
				k.ctcfrVATPrefix(n)
			}
		}
	case NSCbc:
		switch {
		case n.Name == "NameCode" && is(p, "cac", "AdditionalItemProperty") && parentIs(p, "cac", "Item") && isLine(p.Parent.Parent):
			k.ctcfrCheck("BR-FREXT-04", n, n.Text)
		case n.Name == "ID" && is(p, "cac", "DeliveryTerms"):
			k.ctcfrCheck("BR-FREXT-CL-27", n, n.Text)
		}
	}
}

// ctcfrVATPrefix checks BR-CO-09 on a VAT party tax scheme:
// contains(list, substring(cbc:CompanyID, 1, 2)). The schematron looks the
// first two characters up as a substring of the list, so that a single
// character or a space after one also passes, and a scheme without
// identifier too.
func (k *checker) ctcfrVATPrefix(s *xmltree.Node) {
	id := first(s, "cbc:CompanyID")
	prefix := []rune(text(id))
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	if strings.Contains(ctcfrLists()["BR-CO-09"], string(prefix)) {
		return
	}
	k.fail("BR-CO-09", id, "VAT identifier %q does not start with an ISO 3166-1 alpha-2 country prefix", strings.TrimSpace(text(id)))
}

// ctcfrLine checks BR-FREXT-22 to BR-FREXT-26 and BR-FREXT-CO-04 on a line:
// a plain or DETAIL line must have a quantity, its unit, a net amount, a
// price and a VAT category code.
func (k *checker) ctcfrLine(l *xmltree.Node) {
	if !k.lineInCalc(l) {
		return
	}
	qty := append(elems(l, "cbc", "InvoicedQuantity"), elems(l, "cbc", "CreditedQuantity")...)
	if len(qty) == 0 {
		k.fail("BR-FREXT-22", l, "a plain or DETAIL invoice line has no invoiced quantity (BT-129)")
	}
	unit := false
	for _, q := range qty {
		unit = unit || hasAttr(q, "unitCode")
	}
	if !unit {
		k.fail("BR-FREXT-23", l, "a plain or DETAIL invoice line has no invoiced quantity unit of measure code (BT-130)")
	}
	if first(l, "cbc:LineExtensionAmount") == nil {
		k.fail("BR-FREXT-24", l, "a plain or DETAIL invoice line has no invoice line net amount (BT-131)")
	}
	if first(l, "cac:Price/cbc:PriceAmount") == nil {
		k.fail("BR-FREXT-26", l, "a plain or DETAIL invoice line has no item net price (BT-146)")
	}
	code := false
	for _, t := range find(l, "cac:Item/cac:ClassifiedTaxCategory") {
		code = code || vatScheme(t) && first(t, "cbc:ID") != nil
	}
	if !code {
		k.fail("BR-FREXT-CO-04", l, "a plain or DETAIL invoice line has no invoiced item VAT category code (BT-151)")
	}
}

// ctcfrTaxCategories checks UBL-FREXT-SR-48 on a line: one VAT category
// for a plain or DETAIL line, at most one for the others. Here a line is
// plain when it has no preceding invoice reference at all
// (not(cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID)).
func (k *checker) ctcfrTaxCategories(l *xmltree.Node) {
	plain := len(find(l, "cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID")) == 0 || statusIs(k.selfRefs(l), "DETAIL")
	c := find(l, "cac:Item/cac:ClassifiedTaxCategory")
	if plain && len(c) != 1 || !plain && len(c) > 1 {
		k.fail("UBL-FREXT-SR-48", l, "%s (%d found)", k.desc("UBL-FREXT-SR-48"), len(c))
	}
}

// ctcfrSubLine checks the rules of the line subtypes on a line:
// BR-FREXT-05, BR-FREXT-06, BR-FREXT-08, BR-FREXT-11 and BR-FREXT-12.
func (k *checker) ctcfrSubLine(l *xmltree.Node) {
	refs := k.selfRefs(l)
	var parents []string
	isCode := false
	for _, ref := range refs {
		isCode = isCode || len(find(ref, "cac:InvoiceDocumentReference/cbc:DocumentStatusCode")) > 0
		for _, id := range find(ref, "cac:BillingReferenceLine/cbc:ID") {
			parents = append(parents, id.Text)
		}
	}
	var ids []string
	for _, id := range elems(l, "cbc", "ID") {
		ids = append(ids, id.Text)
	}

	// BR-FREXT-05: normalize-space(cac:BillingReference/cac:InvoiceDocumentReference/cbc:DocumentStatusCode)
	// in the list, the first status of all the references.
	if isCode {
		s := first(l, "cac:BillingReference/cac:InvoiceDocumentReference/cbc:DocumentStatusCode")
		if v := normalize(text(s)); strings.Contains(v, " ") || !strings.Contains(ctcfrLists()["BR-FREXT-05"], " "+v+" ") {
			k.fail("BR-FREXT-05", s, "invoice line subtype (EXT-FR-FE-163) %q is not DETAIL, GROUP or INFORMATION", strings.TrimSpace(text(s)))
		}
	}
	if len(parents) > 0 && !isCode {
		k.fail("BR-FREXT-06", l, "an invoice line has a parent line identifier (EXT-FR-FE-162) but no line subtype (EXT-FR-FE-163)")
	}
	// BR-FREXT-11: exists(lines[cbc:ID = $parentID][cbc:ID != $lineID]).
	if len(parents) > 0 {
		found := false
		for _, o := range k.lines() {
			var oids []string
			for _, id := range elems(o, "cbc", "ID") {
				oids = append(oids, id.Text)
			}
			if anyEqual(oids, parents) && anyDiffers(oids, ids) {
				found = true
				break
			}
		}
		if !found {
			k.fail("BR-FREXT-11", l, "the parent line identifier (EXT-FR-FE-162) %q is not the identifier of another invoice line", parents[0])
		}
	}
	// BR-FREXT-08 and BR-FREXT-12 on a GROUP line with a net amount.
	if !k.selfStatus(l, "GROUP") || first(l, "cbc:LineExtensionAmount") == nil {
		return
	}
	var sums []*xmltree.Node
	groups, groupAmounts := 0, 0
	for _, o := range k.lines() {
		orefs := k.selfRefs(o)
		var oparents []string
		for _, ref := range orefs {
			for _, id := range find(ref, "cac:BillingReferenceLine/cbc:ID") {
				oparents = append(oparents, id.Text)
			}
		}
		if !anyEqual(oparents, ids) {
			continue
		}
		group := statusIs(orefs, "GROUP")
		if group || statusIs(orefs, "DETAIL") {
			sums = append(sums, elems(o, "cbc", "LineExtensionAmount")...)
		}
		if group {
			groups++
			groupAmounts += len(elems(o, "cbc", "LineExtensionAmount"))
		}
	}
	sum := sumDouble(sums).hundredths()
	ok := false
	for _, a := range elems(l, "cbc", "LineExtensionAmount") {
		ok = ok || untypedDouble(a.Text) == sum.float()
	}
	if !ok {
		k.fail("BR-FREXT-08", first(l, "cbc:LineExtensionAmount"), "the net amount (BT-131) %q of a GROUP line is not %s, the sum of the net amounts of its DETAIL and GROUP sub-lines",
			strings.TrimSpace(text(first(l, "cbc:LineExtensionAmount"))), sum)
	}
	if groups != groupAmounts {
		k.fail("BR-FREXT-12", l, "a GROUP line with a net amount (BT-131) has %d GROUP sub-lines of which only %d have a net amount", groups, groupAmounts)
	}
}

// anyEqual reports whether a value of a equals a value of b (XPath a = b).
func anyEqual(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// anyDiffers reports whether a value of a differs from a value of b (XPath
// a != b).
func anyDiffers(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x != y {
				return true
			}
		}
	}
	return false
}

// ctcfrItemProperty checks BR-FREXT-54-1 and BR-FREXT-54-2 on an item
// attribute (BG-32): a name or a name code, and either a value or a value
// quantity with its unit.
func (k *checker) ctcfrItemProperty(a *xmltree.Node) {
	if first(a, "cbc:Name") == nil && first(a, "cbc:NameCode") == nil {
		k.fail("BR-FREXT-54-1", a, "an item attribute (BG-32) has neither a name (BT-160) nor a name code")
	}
	value := first(a, "cbc:Value") != nil
	quantities := elems(a, "cbc", "ValueQuantity")
	unit := false
	for _, q := range quantities {
		if v, ok := attr(q, "unitCode"); ok && v != "" {
			unit = true
		}
	}
	if value && len(quantities) == 0 || !value && unit {
		return
	}
	k.fail("BR-FREXT-54-2", a, "an item attribute (BG-32) shall have either a value (BT-161) or a value quantity with a unit code, not both")
}

// ctcfrSyntax checks the syntax rules of the schematron whose context is the
// document element and that the CEN artefacts do not have:
// UBL-FREXT-CR-410 and UBL-FREXT-CR-645 (the delivery terms of the document
// and of the lines hold only an Incoterms code and a delivery location
// name) and CII-FREXT-CR-490 (the tax exchange rate holds only the
// currencies, the rate and its date).
func (k *checker) ctcfrSyntax(lines []*xmltree.Node) {
	if n := deliveryTermsExtra(elems(k.root, "cac", "DeliveryTerms")); n != nil {
		k.fail("UBL-FREXT-CR-410", n, "%s (%s)", k.desc("UBL-FREXT-CR-410"), n.Name)
	}
	var terms []*xmltree.Node
	for _, l := range lines {
		terms = append(terms, elems(l, "cac", "DeliveryTerms")...)
	}
	if n := deliveryTermsExtra(terms); n != nil {
		k.fail("UBL-FREXT-CR-645", n, "%s (%s)", k.desc("UBL-FREXT-CR-645"), n.Name)
	}
	for _, r := range elems(k.root, "cac", "TaxExchangeRate") {
		for _, c := range r.Children {
			switch c.Name {
			case "SourceCurrencyCode", "TargetCurrencyCode", "CalculationRate", "Date":
				continue
			}
			k.fail("CII-FREXT-CR-490", c, "%s (%s)", k.desc("CII-FREXT-CR-490"), c.Name)
			return
		}
	}
}

// deliveryTermsExtra returns the first element of the delivery terms that
// the schematron does not accept: a child other than cbc:ID and
// cac:DeliveryLocation, or a child of the delivery location other than
// cbc:Name (not(cac:DeliveryTerms/*[not(self::cbc:ID or
// self::cac:DeliveryLocation)]) and
// not(cac:DeliveryTerms/cac:DeliveryLocation/*[not(self::cbc:Name)])).
func deliveryTermsExtra(terms []*xmltree.Node) *xmltree.Node {
	for _, t := range terms {
		for _, c := range t.Children {
			if !is(c, "cbc", "ID") && !is(c, "cac", "DeliveryLocation") {
				return c
			}
		}
	}
	for _, t := range terms {
		for _, l := range elems(t, "cac", "DeliveryLocation") {
			for _, c := range l.Children {
				if !is(c, "cbc", "Name") {
					return c
				}
			}
		}
	}
	return nil
}

// ctcfrTotals checks the rules of context cac:LegalMonetaryTotal that the
// schematron adds: BR-FREXT-CO-10 to BR-FREXT-CO-13 and BR-FREXT-CO-15. The
// line net amounts are those of the plain and DETAIL lines; the tolerance is
// 0.01 per summed amount.
func (k *checker) ctcfrTotals(m *xmltree.Node) {
	var amounts []*xmltree.Node
	for _, l := range k.lines() {
		if k.lineInCalc(l) {
			amounts = append(amounts, elems(l, "cbc", "LineExtensionAmount")...)
		}
	}
	lines := sumDouble(amounts).hundredths()
	nLines := len(amounts)
	var allowances, charges []*xmltree.Node
	nAllowances, nCharges := 0, 0
	for _, ac := range elems(m.Parent, "cac", "AllowanceCharge") {
		if chargeIs(ac, false) {
			nAllowances++
			allowances = append(allowances, elems(ac, "cbc", "Amount")...)
		}
		if chargeIs(ac, true) {
			nCharges++
			charges = append(charges, elems(ac, "cbc", "Amount")...)
		}
	}
	allowed := toDecimal(sumDouble(allowances).hundredths())
	charged := toDecimal(sumDouble(charges).hundredths())

	bt106 := first(m, "cbc:LineExtensionAmount")
	if tol := tolerance(nLines); !decimalOf(bt106).sub(lines).abs().hundredths().le(tol) {
		k.fail("BR-FREXT-CO-10", at(bt106, m), "sum of invoice line net amounts (BT-106) %q is not %s, the sum of the net amounts of the plain and DETAIL lines, within %s",
			strings.TrimSpace(text(bt106)), lines, tol)
	}
	bt107 := first(m, "cbc:AllowanceTotalAmount")
	if tol := tolerance(nAllowances); !decimalOf(bt107).sub(allowed).abs().le(tol) && (bt107 != nil || nAllowances > 0) {
		k.fail("BR-FREXT-CO-11", at(bt107, m), "sum of allowances on document level (BT-107) %q is not %s within %s", strings.TrimSpace(text(bt107)), allowed, tol)
	}
	bt108 := first(m, "cbc:ChargeTotalAmount")
	if tol := tolerance(nCharges); !decimalOf(bt108).sub(charged).abs().le(tol) && (bt108 != nil || nCharges > 0) {
		k.fail("BR-FREXT-CO-12", at(bt108, m), "sum of charges on document level (BT-108) %q is not %s within %s", strings.TrimSpace(text(bt108)), charged, tol)
	}
	n := nLines + nAllowances + nCharges
	bt109 := first(m, "cbc:TaxExclusiveAmount")
	if tol := tolerance(n); !decimalOf(bt109).sub(lines).add(allowed).sub(charged).abs().hundredths().le(tol) {
		k.fail("BR-FREXT-CO-13", at(bt109, m), "invoice total amount without VAT (BT-109) %q is not %s (lines) - %s (allowances) + %s (charges) within %s",
			strings.TrimSpace(text(bt109)), lines, allowed, charged, tol)
	}
	// BR-FREXT-CO-15, when the invoice has a total VAT amount (BT-110) in the
	// invoice currency.
	var tax *xmltree.Node
	currencies := elems(k.root, "cbc", "DocumentCurrencyCode")
	for _, a := range find(m.Parent, "cac:TaxTotal/cbc:TaxAmount") {
		if v, ok := attr(a, "currencyID"); ok && tax == nil {
			for _, c := range currencies {
				if c.Text == v {
					tax = a
				}
			}
		}
	}
	if tax == nil {
		return
	}
	bt112 := first(m, "cbc:TaxInclusiveAmount")
	if tol := tolerance(n); !decimalOf(bt112).sub(decimalOf(tax)).sub(decimalOf(bt109)).abs().le(tol) {
		k.fail("BR-FREXT-CO-15", at(bt112, m), "invoice total amount with VAT (BT-112) %q is not BT-109 %q + BT-110 %q within %s",
			strings.TrimSpace(text(bt112)), strings.TrimSpace(text(bt109)), strings.TrimSpace(text(tax)), tol)
	}
}

// at returns n, or def when n is nil.
func at(n, def *xmltree.Node) *xmltree.Node {
	if n == nil {
		return def
	}
	return n
}

// taxSelection selects the VAT categories of the lines and of the document
// level allowances and charges that a VAT breakdown sums: same category code,
// same rate (cbc:Percent, compared as decimals) for a category with rates,
// and, with the exemption reasons, the same normalized exemption reason code
// and text.
type taxSelection struct {
	code       string
	byRate     bool
	rate       xnum
	reasonCode string
	reasonText string
}

// matches reports whether the VAT category t is selected, with or without
// the exemption reasons.
func (s *taxSelection) matches(t *xmltree.Node, reasons bool) bool {
	if !idIs(t, s.code) {
		return false
	}
	if s.byRate && !decimalOf(first(t, "cbc:Percent")).eq(s.rate) {
		return false
	}
	return !reasons || normalize(text(first(t, "cbc:TaxExemptionReasonCode"))) == s.reasonCode &&
		normalize(text(first(t, "cbc:TaxExemptionReason"))) == s.reasonText
}

// categorySum holds the amounts that a VAT breakdown sums.
type categorySum struct {
	lines, allowances, charges xnum
	n                          int // summed line net amounts and allowance or charge amounts
}

// sum returns the amounts of the plain and DETAIL lines and of the document
// level allowances and charges selected by s.
func (k *checker) sum(s *taxSelection, reasons bool) categorySum {
	var amounts, allowances, charges []*xmltree.Node
	n := 0
	for _, l := range k.lines() {
		if !k.lineInCalc(l) {
			continue
		}
		for _, t := range find(l, "cac:Item/cac:ClassifiedTaxCategory") {
			if s.matches(t, reasons) {
				amounts = append(amounts, elems(l, "cbc", "LineExtensionAmount")...)
				break
			}
		}
	}
	n += len(amounts)
	for _, ac := range elems(k.root, "cac", "AllowanceCharge") {
		match := false
		for _, t := range elems(ac, "cac", "TaxCategory") {
			match = match || s.matches(t, reasons)
		}
		if !match {
			continue
		}
		a := elems(ac, "cbc", "Amount")
		n += len(a)
		if chargeIs(ac, false) {
			allowances = append(allowances, a...)
		}
		if chargeIs(ac, true) {
			charges = append(charges, a...)
		}
	}
	return categorySum{
		lines:      sumDouble(amounts).hundredths(),
		allowances: toDecimal(sumDouble(allowances).hundredths()),
		charges:    toDecimal(sumDouble(charges).hundredths()),
		n:          n,
	}
}

// basisOK reports whether |basis − lines + allowances − charges| <= 0.01 × n.
func (c categorySum) basisOK(basis xnum) bool {
	return basis.sub(c.lines).add(c.allowances).sub(c.charges).abs().le(tolerance(c.n))
}

// ctcfrBreakdown checks the rules of a VAT breakdown (BG-23) of the
// document: BR-FREXT-*-08, *-08ini and *-08rev of its category, and
// BR-FREXT-S-09.
func (k *checker) ctcfrBreakdown(t *xmltree.Node) {
	if !vatScheme(t) {
		return
	}
	var c *ctcfrCategory
	for i := range ctcfrBreakdowns {
		if idIs(t, ctcfrBreakdowns[i].code) {
			c = &ctcfrBreakdowns[i]
			break
		}
	}
	if c == nil {
		return
	}
	s := &taxSelection{
		code:       c.code,
		byRate:     c.rate,
		reasonCode: normalize(text(first(t, "cbc:TaxExemptionReasonCode"))),
		reasonText: normalize(text(first(t, "cbc:TaxExemptionReason"))),
	}
	if c.rate {
		s.rate = decimalOf(first(t, "cbc:Percent"))
	}
	taxable := first(t.Parent, "cbc:TaxableAmount")
	basis := decimalOf(taxable)
	rev, ini := k.sum(s, true), k.sum(s, false)
	okRev, okIni := rev.basisOK(basis), ini.basisOK(basis)
	what := "VAT category " + c.code
	if c.rate {
		what += " rate " + strings.TrimSpace(text(first(t, "cbc:Percent")))
	}
	if !okRev && !okIni {
		k.fail("BR-FREXT-"+c.id+"-08", at(taxable, t), "%s: the VAT category taxable amount (BT-116) %q is neither %s (with the exemption reason) nor %s (without it): lines - allowances + charges, within 0.01 per amount",
			what, strings.TrimSpace(text(taxable)), rev.basis(), ini.basis())
	}
	if !okIni {
		k.fail("BR-FREXT-"+c.id+"-08ini", at(taxable, t), "%s: the VAT category taxable amount (BT-116) %q is not %s (lines %s - allowances %s + charges %s, whatever the exemption reason) within %s",
			what, strings.TrimSpace(text(taxable)), ini.basis(), ini.lines, ini.allowances, ini.charges, tolerance(ini.n))
	}
	if !okRev {
		k.fail("BR-FREXT-"+c.id+"-08rev", at(taxable, t), "%s: the VAT category taxable amount (BT-116) %q is not %s (lines %s - allowances %s + charges %s, of the same exemption reason) within %s",
			what, strings.TrimSpace(text(taxable)), rev.basis(), rev.lines, rev.allowances, rev.charges, tolerance(rev.n))
	}
	if c.code != "S" {
		return
	}
	// BR-FREXT-S-09: abs(BT-117 − round(BT-116 × rate div 100 × 100) div 100)
	// <= 0.01 × n, in decimals.
	tax := first(t.Parent, "cbc:TaxAmount")
	want := basis.mul(s.rate).hundredthsOfPercent()
	if tol := tolerance(rev.n); !decimalOf(tax).sub(want).abs().le(tol) {
		k.fail("BR-FREXT-S-09", at(tax, t), "%s: the VAT category tax amount (BT-117) %q is not %s (BT-116 %q × rate / 100) within %s",
			what, strings.TrimSpace(text(tax)), want, strings.TrimSpace(text(taxable)), tol)
	}
}

// basis returns lines − allowances + charges, for messages.
func (c categorySum) basis() xnum {
	return c.lines.sub(c.allowances).add(c.charges)
}

// xnum is an XPath number as the schematron computes it: an xs:decimal
// (exact, d) or an xs:double (f). XPath converts the untyped amounts that
// sum() adds to xs:double, while xs:decimal() casts to xs:decimal; an
// operation with a double is done in doubles. The zero value is the empty
// sequence (a missing amount), which propagates through arithmetic and fails
// every comparison.
type xnum struct {
	kind uint8
	d    *big.Rat
	f    float64
}

// Kinds of xnum.
const (
	xEmpty uint8 = iota
	xDecimal
	xDouble
)

func decimalNum(r *big.Rat) xnum { return xnum{kind: xDecimal, d: r} }

func doubleNum(f float64) xnum { return xnum{kind: xDouble, f: f} }

// decimalOf returns xs:decimal(n): empty for no element or a value that is
// not a decimal (for which the schematron raises an error).
func decimalOf(n *xmltree.Node) xnum {
	if n == nil {
		return xnum{}
	}
	s := strings.TrimSpace(n.Text)
	t := strings.TrimLeft(s, "+-")
	if len(s)-len(t) > 1 || t == "" || t == "." {
		return xnum{}
	}
	dot := false
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case c == '.' && !dot:
			dot = true
		case c < '0' || c > '9':
			return xnum{}
		}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return xnum{}
	}
	return decimalNum(r)
}

// untypedDouble returns the untyped value s converted to xs:double: NaN when
// s is not a number (the schematron then raises an error).
func untypedDouble(s string) float64 {
	t := strings.TrimSpace(s)
	switch t {
	case "INF", "+INF":
		return math.Inf(1)
	case "-INF":
		return math.Inf(-1)
	case "NaN":
		return math.NaN()
	}
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case '0' <= c && c <= '9', c == '.', c == '+', c == '-', c == 'e', c == 'E':
		default:
			return math.NaN()
		}
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

// sumDouble returns sum(nodes) of untyped values: an xs:double, or the
// integer 0 for no node.
func sumDouble(nodes []*xmltree.Node) xnum {
	if len(nodes) == 0 {
		return decimalNum(new(big.Rat))
	}
	s := 0.0
	for i, n := range nodes {
		if i == 0 {
			s = untypedDouble(n.Text)
		} else {
			s += untypedDouble(n.Text)
		}
	}
	return doubleNum(s)
}

// tolerance returns 0.01 × n.
func tolerance(n int) xnum { return decimalNum(big.NewRat(int64(n), 100)) }

// toDecimal returns xs:decimal(x): the exact value of a double (empty for NaN
// and infinities, which the cast rejects).
func toDecimal(x xnum) xnum {
	if x.kind != xDouble {
		return x
	}
	if math.IsNaN(x.f) || math.IsInf(x.f, 0) {
		return xnum{}
	}
	return decimalNum(new(big.Rat).SetFloat64(x.f))
}

// float returns x as an xs:double.
func (x xnum) float() float64 {
	switch x.kind {
	case xDouble:
		return x.f
	case xDecimal:
		f, _ := x.d.Float64()
		return f
	}
	return math.NaN()
}

func (x xnum) add(y xnum) xnum {
	switch {
	case x.kind == xEmpty || y.kind == xEmpty:
		return xnum{}
	case x.kind == xDouble || y.kind == xDouble:
		return doubleNum(x.float() + y.float())
	}
	return decimalNum(new(big.Rat).Add(x.d, y.d))
}

func (x xnum) sub(y xnum) xnum {
	switch {
	case x.kind == xEmpty || y.kind == xEmpty:
		return xnum{}
	case x.kind == xDouble || y.kind == xDouble:
		return doubleNum(x.float() - y.float())
	}
	return decimalNum(new(big.Rat).Sub(x.d, y.d))
}

func (x xnum) mul(y xnum) xnum {
	switch {
	case x.kind == xEmpty || y.kind == xEmpty:
		return xnum{}
	case x.kind == xDouble || y.kind == xDouble:
		return doubleNum(x.float() * y.float())
	}
	return decimalNum(new(big.Rat).Mul(x.d, y.d))
}

func (x xnum) abs() xnum {
	switch x.kind {
	case xDouble:
		return doubleNum(math.Abs(x.f))
	case xDecimal:
		return decimalNum(new(big.Rat).Abs(x.d))
	}
	return x
}

// le reports x le y: false when either is empty or NaN.
func (x xnum) le(y xnum) bool {
	switch {
	case x.kind == xEmpty || y.kind == xEmpty:
		return false
	case x.kind == xDouble || y.kind == xDouble:
		return x.float() <= y.float()
	}
	return x.d.Cmp(y.d) <= 0
}

// eq reports x = y: false when either is empty.
func (x xnum) eq(y xnum) bool {
	switch {
	case x.kind == xEmpty || y.kind == xEmpty:
		return false
	case x.kind == xDouble || y.kind == xDouble:
		return x.float() == y.float()
	}
	return x.d.Cmp(y.d) == 0
}

// hundredths returns round(x * 100) div 100, round going to the nearest
// integer, halves towards positive infinity.
func (x xnum) hundredths() xnum {
	switch x.kind {
	case xDouble:
		v := x.f * 100
		r := math.Floor(v)
		if v-r >= 0.5 {
			r++
		}
		return doubleNum(r / 100)
	case xDecimal:
		return decimalNum(roundRat(new(big.Rat).Mul(x.d, big.NewRat(100, 1)), 100))
	}
	return x
}

// hundredthsOfPercent returns round(x div 100 * 100) div 100 for a decimal
// x: x div 100 rounded to two decimals, halves towards positive infinity
// (empty for an empty x).
func (x xnum) hundredthsOfPercent() xnum {
	if x.kind != xDecimal {
		return xnum{}
	}
	return decimalNum(roundRat(new(big.Rat).Set(x.d), 100))
}

// roundRat returns floor(r + 1/2) / div.
func roundRat(r *big.Rat, div int64) *big.Rat {
	r.Add(r, big.NewRat(1, 2))
	q := new(big.Int).Div(r.Num(), r.Denom()) // Euclidean: the floor for a positive denominator
	return new(big.Rat).SetFrac(q, big.NewInt(div))
}

// String formats x for messages.
func (x xnum) String() string {
	switch x.kind {
	case xDouble:
		return strconv.FormatFloat(x.f, 'f', -1, 64)
	case xDecimal:
		if x.d.IsInt() {
			return x.d.Num().String()
		}
		if s, exact := x.d.FloatPrec(); exact {
			return x.d.FloatString(s)
		}
		return x.d.FloatString(20)
	}
	return "(none)"
}
