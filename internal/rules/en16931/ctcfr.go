package en16931

import (
	"math"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// This file holds the rules of the EXTENDED-CTC-FR schematron of the
// FNFE-MPE (EXTENDED-CTC-FR-CII.sch, France_RFE CTCFRRelease) that differ
// from the CEN ones. The schematron keeps most CEN rules as they are; it
// removes others (BR-27, BR-28, BR-CO-17, BR-IP and BR-IG, most of the rules
// 01 and 08 to 10 of the VAT categories...), renames a few (ctcfrRenamed),
// changes the test of BR-CO-09, BR-CO-14, BR-DEC-13 and BR-DEC-15, replaces
// the code lists by its own (ctcfrLists) and adds its own rules:
//
//   - lines: only plain and DETAIL lines (subtype BT-X-8) need a quantity, a
//     unit, a net amount, a net price and a VAT category (BR-FREXT-BR-22 to
//     BR-FREXT-BR-26, BR-FREXT-CO-04); BR-FREXT-05 to BR-FREXT-12 check the
//     subtypes and the groups of sub-lines;
//   - totals: only plain and DETAIL lines are summed, the logistics service
//     charges (BG-X-42) count as charges, and the sums get a tolerance of 0.01
//     per amount summed (BR-FREXT-CO-10 to BR-FREXT-CO-15, BR-FXEXT-*-08,
//     BR-FXEXT-S-09);
//   - VAT categories: a category is in the VAT breakdown if and only if a
//     plain or DETAIL line, or an allowance or charge, has it
//     (BR-FREXT-*-01); the rules of categories L and M are BR-AF and BR-AG;
//   - item attributes: a name or a code, and a value or a measure with a unit
//     (BR-FREXT-54-1, BR-FREXT-54-2), the code from UNTDED 6313 and the
//     Factur-X extension (BR-FXEXT-04, a warning);
//   - BR-CO-25, which the CEN artefacts only have for UBL.
//
// As elsewhere in the package, the rules follow the XPath of the schematron.
// Some consequences are worth knowing:
//
//   - BR-CO-14, BR-DEC-13, BR-DEC-15 and BR-FREXT-08 compute with untyped
//     values, which XPath converts to xs:double: they are computed in floating
//     point here too.
//   - The rules whose test starts with "for $x in xs:decimal(...)" fail when
//     an element is missing, as the expression then yields an empty sequence.
//     BR-FREXT-CO-15 thus requires a total VAT amount (BT-110) in the invoice
//     currency; when there are several, the totals without and with VAT must
//     be equal.
//   - The schematron declares the rules of the VAT breakdowns of type VAT and
//     category AE, L, M, E, G, K, O, S and Z before the rule of the other
//     breakdowns, in the same pattern: BR-45 to BR-48, BR-CO-03, BR-DEC-19 and
//     BR-DEC-20 only apply to the other breakdowns.

// ctcfrRenamed maps the CEN rules that the schematron keeps under another
// identifier, with the same context and test, to that identifier.
var ctcfrRenamed = map[string]string{
	"BR-38":    "BR-FREXT-BR-38",
	"BR-44":    "BR-FREXT-BR-44",
	"BR-CL-10": "BR-FREXT-CL-10",
	"BR-CL-24": "BR-FREXT-CL-24",
}

// ctcfrCodeListText gives the subject and the list name of the code list
// rules of the schematron that the CEN artefacts do not have, for messages.
var ctcfrCodeListText = map[string][2]string{
	"BR-FREXT-CL-10":        {"identifier scheme", "ISO 6523 ICD, BY and SE"},
	"BR-FREXT-CL-19-GP":     {"gross price allowance reason code", "UNTDID 5189"},
	"BR-FREXT-CL-20":        {"charge reason code", "UNTDID 7161"},
	"BR-FREXT-CL-20-TAX":    {"tax reason code", "UNTDID 5153 without VAT"},
	"BR-FREXT-CL-20-LISTID": {"charge reason code list identifier", "7161 and 5153"},
	"BR-FREXT-CL-24":        {"attached document MIME code (BT-125)", "the MIME codes of the EXTENDED-CTC-FR schematron"},
	"BR-FREXT-CL-27":        {"delivery terms code", "the Incoterms of the EXTENDED-CTC-FR schematron"},
	"BR-FXEXT-04":           {"item attribute code (BT-X-11)", "UNTDED 6313 and the Factur-X extension"},
}

// vatCategoriesCTCFR are the category rules of the schematron: those of the
// CEN, with BR-AF for category L and BR-AG for category M.
var vatCategoriesCTCFR = func() [nCat]vatCategory {
	c := vatCategories
	c[catL].prefix = "BR-AF-"
	c[catM].prefix = "BR-AG-"
	return c
}()

// vatCategoriesUBLCTCFR are the category rules of the EXTENDED-CTC-FR
// schematron for UBL: those of the CEN UBL artefacts (vatCategoriesUBL),
// whose rules of category L (BR-IG there) and M (BR-IP) it numbers BR-AF
// and BR-AG.
var vatCategoriesUBLCTCFR = func() [nCat]vatCategory {
	c := vatCategoriesUBL
	c[catL].prefix = "BR-AF-"
	c[catM].prefix = "BR-AG-"
	return c
}()

// ctcfrCategory describes the rules BR-FREXT-*-01 and BR-FXEXT-*-08 of a VAT
// category.
type ctcfrCategory struct {
	cat              int    // index in catCodes
	one, b, ini, rev string // rule identifiers
	rate             bool   // the rules group the amounts by rate (all but O)
}

// ctcfrCategories lists the categories in the order of the rules of the VAT
// breakdowns in the schematron: the first rule whose context matches a
// breakdown wins.
var ctcfrCategories = [...]ctcfrCategory{
	{catAE, "BR-FREXT-AE-01", "BR-FXEXT-AE-08", "BR-FXEXT-AE-08ini", "BR-FXEXT-AE-08rev", true},
	{catL, "BR-FREXT-AF-01", "BR-FXEXT-AF-08", "BR-FXEXT-AF-08ini", "BR-FXEXT-AF-08rev", true},
	{catM, "BR-FREXT-AG-01", "BR-FXEXT-AG-08", "BR-FXEXT-AG-08ini", "BR-FXEXT-AG-08rev", true},
	{catE, "BR-FREXT-E-01", "BR-FXEXT-E-08", "BR-FXEXT-E-08ini", "BR-FXEXT-E-08rev", true},
	{catG, "BR-FREXT-G-01", "BR-FXEXT-G-08", "BR-FXEXT-G-08ini", "BR-FXEXT-G-08rev", true},
	{catK, "BR-FREXT-IC-01", "BR-FXEXT-IC-08", "BR-FXEXT-IC-08ini", "BR-FXEXT-IC-08rev", true},
	{catO, "BR-FREXT-O-01", "BR-FXEXT-O-08", "BR-FXEXT-O-08ini", "BR-FXEXT-O-08rev", false},
	{catS, "BR-FREXT-S-01", "BR-FXEXT-S-08", "BR-FXEXT-S-08ini", "BR-FXEXT-S-08rev", true},
	{catZ, "BR-FREXT-Z-01", "BR-FXEXT-Z-08", "BR-FXEXT-Z-08ini", "BR-FXEXT-Z-08rev", true},
}

var (
	hundred   = invoice.DecimalFromInt(100)
	hundredth = invoice.NewDecimal(1, 2)
)

// tolerance returns 0.01 × n.
func tolerance(n int) invoice.Decimal { return invoice.DecimalFromInt(int64(n)).Mul(hundredth) }

// near reports whether |a − b| <= tol.
func near(a, b, tol invoice.Decimal) bool { return a.Sub(b).Abs().Cmp(tol) <= 0 }

// detail reports whether the ram:AssociatedDocumentLineDocument children of
// a line satisfy not(ram:LineStatusReasonCode) or ram:LineStatusReasonCode =
// 'DETAIL': the line is a plain or DETAIL line, summed in the totals.
func detail(line *xmltree.Node) bool {
	status := false
	for it, adl := elems(line, "AssociatedDocumentLineDocument"); adl != nil; adl = it.next() {
		for it, s := elems(adl, "LineStatusReasonCode"); s != nil; s = it.next() {
			if s.Text == "DETAIL" {
				return true
			}
			status = true
		}
	}
	return !status
}

// lineText reports whether f returns true for the text of a
// ram:AssociatedDocumentLineDocument/name element of line, as an XPath
// general comparison of ram:AssociatedDocumentLineDocument/name does.
func lineText(line *xmltree.Node, name string, f func(string) bool) bool {
	for it, adl := elems(line, "AssociatedDocumentLineDocument"); adl != nil; adl = it.next() {
		for it, c := elems(adl, name); c != nil; c = it.next() {
			if f(c.Text) {
				return true
			}
		}
	}
	return false
}

// lineIs reports whether a ram:AssociatedDocumentLineDocument/name element
// of line has the text v.
func lineIs(line *xmltree.Node, name, v string) bool {
	return lineText(line, name, func(s string) bool { return s == v })
}

// double returns the value of s converted to xs:double, as XPath converts an
// untyped value in arithmetic and comparisons with a number: NaN when s is
// not a number.
func double(s string) float64 {
	t := trim(s)
	switch t {
	case "INF", "+INF":
		return math.Inf(1)
	case "-INF":
		return math.Inf(-1)
	}
	digits := false
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case '0' <= c && c <= '9':
			digits = true
		case c == '.' || c == '+' || c == '-' || c == 'e' || c == 'E':
		default:
			return math.NaN()
		}
	}
	if !digits {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

// roundDouble returns round(v) for an xs:double: halves towards positive
// infinity.
func roundDouble(v float64) float64 { return math.Floor(v + 0.5) }

// twoDecimalsDouble reports whether . = round(. * 100) div 100 for the
// untyped value of t, the test of BR-DEC-13 and BR-DEC-15 in the schematron.
func twoDecimalsDouble(t *xmltree.Node) bool {
	v := double(t.Text)
	return v == roundDouble(v*100)/100
}

// ctcfrVATTotal checks BR-CO-14 as the schematron writes it: the invoice
// total VAT amount (BT-110) in the invoice currency equals
// round(sum(BT-117) * 10 * 10) div 100, in floating point.
func (k *checker) ctcfrVATTotal(m *xmltree.Node) {
	st := k.d.Settlement
	sum := 0.0
	for it, t := elems(st, "ApplicableTradeTax"); t != nil; t = it.next() {
		for it, c := elems(t, "CalculatedAmount"); c != nil; c = it.next() {
			sum += double(c.Text)
		}
	}
	want := roundDouble(sum*10*10) / 100
	for it, t := elems(m, "TaxTotalAmount"); t != nil; t = it.next() {
		if v, ok := t.LookupAttr("currencyID"); !ok || !textIn(v, st, "InvoiceCurrencyCode") {
			continue
		}
		if double(t.Text) != want {
			k.fail("BR-CO-14", t, "invoice total VAT amount (BT-110) %q is not the sum of the VAT category tax amounts (BT-117) %s",
				quote(t.Text), strconv.FormatFloat(want, 'f', -1, 64))
		}
	}
}

// ctcfrDocument checks the rules of context /rsm:CrossIndustryInvoice that
// the schematron adds: BR-CO-25 and BR-FREXT-*-01.
func (k *checker) ctcfrDocument() {
	// BR-CO-25: xs:decimal(//ram:DuePayableAmount) > 0 requires a payment due
	// date or payment terms.
	if v, ok := dec(find(k.d.Summation, "DuePayableAmount")); ok && v.Sign() > 0 {
		terms := false
		for it, pt := elems(k.d.Settlement, "SpecifiedTradePaymentTerms"); pt != nil; pt = it.next() {
			if pt.Elem("DueDateDateTime") != nil || pt.Elem("Description") != nil {
				terms = true
			}
		}
		if !terms {
			k.fail("BR-CO-25", k.d.Root, "the amount due for payment (BT-115) %s is positive, but the invoice has neither a payment due date (BT-9) nor payment terms (BT-20)", v.String())
		}
	}
	for i := range ctcfrCategories {
		c := &ctcfrCategories[i]
		hdr, line, tax := k.catHdr[c.cat], k.catDetail[c.cat], k.catTax[c.cat]
		if (hdr == 0 && line == 0 && tax == 0) || (hdr >= 1 && (line > 0 || tax > 0)) {
			continue
		}
		code := catCodes[c.cat]
		if hdr == 0 {
			k.fail(c.one, k.d.Root, "a plain or DETAIL line, an allowance or a charge has VAT category %s, but the VAT breakdown (BG-23) has no entry of that category", code)
		} else {
			k.fail(c.one, k.d.Root, "the VAT breakdown (BG-23) has category %s, but no plain or DETAIL line, allowance or charge has it", code)
		}
	}
}

// ctcfrLine checks the line rules of the schematron: BR-FREXT-BR-22 to
// BR-FREXT-BR-26 and BR-FREXT-CO-04 (context
// //ram:IncludedSupplyChainTradeLineItem/ram:AssociatedDocumentLineDocument[not(ram:LineStatusReasonCode)
// or ram:LineStatusReasonCode = 'DETAIL']) and the rules of the line subtypes
// BR-FREXT-05 to BR-FREXT-12 (context //ram:IncludedSupplyChainTradeLineItem).
func (k *checker) ctcfrLine(l *xmltree.Node) {
	id := lineID(l)
	for it, adl := elems(l, "AssociatedDocumentLineDocument"); adl != nil; adl = it.next() {
		status := false
		plain := false
		for it, s := elems(adl, "LineStatusReasonCode"); s != nil; s = it.next() {
			status = true
			plain = plain || s.Text == "DETAIL"
		}
		if status && !plain {
			continue
		}
		if find(l, "SpecifiedLineTradeDelivery", "BilledQuantity") == nil {
			k.fail("BR-FREXT-BR-22", adl, "plain or DETAIL line %q has no invoiced quantity (BT-129)", id)
		}
		unit := false
		for it, del := elems(l, "SpecifiedLineTradeDelivery"); del != nil; del = it.next() {
			for it, q := elems(del, "BilledQuantity"); q != nil; q = it.next() {
				if _, ok := q.LookupAttr("unitCode"); ok {
					unit = true
				}
			}
		}
		if !unit {
			k.fail("BR-FREXT-BR-23", adl, "plain or DETAIL line %q has no invoiced quantity unit of measure code (BT-130)", id)
		}
		if find(l, "SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount") == nil {
			k.fail("BR-FREXT-BR-24", adl, "plain or DETAIL line %q has no line net amount (BT-131)", id)
		}
		if find(l, "SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "ChargeAmount") == nil {
			k.fail("BR-FREXT-BR-26", adl, "plain or DETAIL line %q has no item net price (BT-146)", id)
		}
		cat := false
		for it, s := elems(l, "SpecifiedLineTradeSettlement"); s != nil; s = it.next() {
			for it, t := elems(s, "ApplicableTradeTax"); t != nil; t = it.next() {
				if isVAT(t) && t.Elem("CategoryCode") != nil {
					cat = true
				}
			}
		}
		if !cat {
			k.fail("BR-FREXT-CO-04", adl, "plain or DETAIL line %q has no invoiced item VAT category code (BT-151)", id)
		}
	}
	k.subLines(l, id)
}

// lineTotalPath leads from a line to its net amounts (BT-131).
var lineTotalPath = []string{"SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount"}

// subLines checks BR-FREXT-05, BR-FREXT-06, BR-FREXT-08, BR-FREXT-11 and
// BR-FREXT-12 on line l, whose variables are $lineID (the line identifiers),
// $subLineCode (the subtypes) and $parentID (the normalized parent line
// identifier).
func (k *checker) subLines(l *xmltree.Node, id string) {
	var status *xmltree.Node
	var parent string
	for it, adl := elems(l, "AssociatedDocumentLineDocument"); adl != nil; adl = it.next() {
		if status == nil {
			status = adl.Elem("LineStatusReasonCode")
		}
		if parent == "" {
			parent = adl.Elem("ParentLineID").Normalized()
		}
	}
	if s := status; s != nil && (strings.Contains(s.Text, " ") || !strings.Contains(ctcfrLists["BR-FREXT-05"], " "+s.Text+" ")) {
		k.fail("BR-FREXT-05", l, "line %q: the subtype (BT-X-8) %q is not DETAIL, GROUP or INFORMATION", id, quote(s.Text))
	}
	if parent != "" {
		if !lineText(l, "LineStatusReasonCode", func(s string) bool { return s != "" }) {
			k.fail("BR-FREXT-06", l, "line %q has a parent line (BT-X-304) %q but no subtype (BT-X-8)", id, quote(parent))
		}
		// A line whose identifier is $parentID and differs from one of
		// $lineID.
		found := false
		for _, m := range k.d.Lines {
			if lineIs(m, "LineID", parent) && lineText(m, "LineID", func(x string) bool {
				return lineText(l, "LineID", func(y string) bool { return x != y })
			}) {
				found = true
				break
			}
		}
		if !found {
			k.fail("BR-FREXT-11", l, "the parent line %q (BT-X-304) of line %q is not the identifier (BT-126) of another line", quote(parent), id)
		}
	}
	total := find(l, lineTotalPath...)
	if total == nil || !lineIs(l, "LineStatusReasonCode", "GROUP") {
		return
	}
	// The sub-lines: ram:ParentLineID = $lineID.
	sum := 0.0
	groups, amounts := 0, 0
	for _, m := range k.d.Lines {
		if !lineText(m, "ParentLineID", func(p string) bool { return lineIs(l, "LineID", p) }) {
			continue
		}
		group := lineIs(m, "LineStatusReasonCode", "GROUP")
		if !group && !lineIs(m, "LineStatusReasonCode", "DETAIL") {
			continue
		}
		n := 0
		forEach(m, lineTotalPath, func(t *xmltree.Node) {
			sum += double(t.Text)
			n++
		})
		if group {
			groups++
			amounts += n
		}
	}
	want := roundDouble(sum*100) / 100
	ok := false
	forEach(l, lineTotalPath, func(t *xmltree.Node) { ok = ok || double(t.Text) == want })
	if !ok {
		k.fail("BR-FREXT-08", total, "the net amount (BT-131) %q of group line %q is not %s, the sum of the net amounts of its DETAIL and GROUP sub-lines",
			quote(total.Text), id, strconv.FormatFloat(want, 'f', -1, 64))
	}
	if groups != amounts {
		k.fail("BR-FREXT-12", total, "group line %q has a net amount (BT-131), but %d of its %d GROUP sub-lines have none", id, groups-amounts, groups)
	}
}

// forEach calls f on each descendant of n matching the path of local names.
func forEach(n *xmltree.Node, path []string, f func(*xmltree.Node)) {
	if n == nil {
		return
	}
	if len(path) == 0 {
		f(n)
		return
	}
	for _, c := range n.Children {
		if c.Name == path[0] {
			forEach(c, path[1:], f)
		}
	}
}

// ctcfrReasonCode checks the reason codes of the allowances and charges
// against the lists of the schematron. Its rules, in that order, have the
// contexts ram:SpecifiedTradeAllowanceCharge[allowance]/ram:ReasonCode,
// ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge[allowance]/ram:ReasonCode
// and, for a charge, ram:ReasonCode[@listID='7161' or not(@listID)],
// [@listID='5153'] and a non-empty @listID: the first that matches applies.
func (k *checker) ctcfrReasonCode(n *xmltree.Node) {
	p := n.Parent
	switch {
	case p.Name == "SpecifiedTradeAllowanceCharge":
		allowance, charge := chargeIndicator(p)
		if allowance {
			k.codeList("BR-CL-19", n, n.Text)
			return
		}
		if !charge {
			return
		}
		switch list, ok := n.LookupAttr("listID"); {
		case !ok || list == "7161":
			k.codeList("BR-FREXT-CL-20", n, n.Text)
		case list == "5153":
			k.codeList("BR-FREXT-CL-20-TAX", n, n.Text)
		case list != "":
			k.codeList("BR-FREXT-CL-20-LISTID", n, list)
		}
	case p.Name == "AppliedTradeAllowanceCharge" && p.Parent != nil && p.Parent.Name == "GrossPriceProductTradePrice":
		if allowance, _ := chargeIndicator(p); allowance {
			k.codeList("BR-FREXT-CL-19-GP", n, n.Text)
		}
	}
}

// ctcfrCharacteristic checks BR-FREXT-54-1 and BR-FREXT-54-2 on an item
// attribute (BG-32).
func (k *checker) ctcfrCharacteristic(c *xmltree.Node) {
	var desc, typeCode, value, measure, measureUnit bool
	for _, ch := range c.Children {
		switch ch.Name {
		case "Description":
			desc = true
		case "TypeCode":
			typeCode = true
		case "Value":
			value = true
		case "ValueMeasure":
			measure = true
			if u, ok := ch.LookupAttr("unitCode"); ok && u != "" {
				measureUnit = true
			}
		}
	}
	if !desc && !typeCode {
		k.fail("BR-FREXT-54-1", c, "an item attribute (BG-32) has neither a name (BT-160) nor a code (BT-X-11)")
	}
	if !(value && !measure || !value && measureUnit) {
		k.fail("BR-FREXT-54-2", c, "an item attribute (BG-32) must have either a value (BT-161) or a measure with a unit (BT-X-12), not both")
	}
}

// ctcfrTotals checks the rules of context
// //ram:SpecifiedTradeSettlementHeaderMonetarySummation that the schematron
// adds: BR-FREXT-CO-10 to BR-FREXT-CO-13 and BR-FREXT-CO-15.
func (k *checker) ctcfrTotals(m *xmltree.Node) {
	var allLines, nDetail int
	lines := zero
	if tr := m.Parent.Parent; tr != nil {
		for it, l := elems(tr, "IncludedSupplyChainTradeLineItem"); l != nil; l = it.next() {
			allLines++
			if !detail(l) {
				continue
			}
			nDetail++
			forEach(l, lineTotalPath, func(t *xmltree.Node) {
				if v, ok := dec(t); ok {
					lines = lines.Add(v)
				}
			})
		}
	}
	lines = round2(lines)
	// The sums read the header settlement, the counts the parent of the
	// summation.
	allowances, charges, logistics := zero, zero, zero
	for _, ac := range children(k.d.Settlement) {
		switch {
		case ac.Name == "SpecifiedTradeAllowanceCharge" && childText(ac.Elem("ChargeIndicator"), "Indicator", "false"):
			allowances = allowances.Add(sumOf(ac, "ActualAmount"))
		case ac.Name == "SpecifiedTradeAllowanceCharge" && childText(ac.Elem("ChargeIndicator"), "Indicator", "true"):
			charges = charges.Add(sumOf(ac, "ActualAmount"))
		case ac.Name == "SpecifiedLogisticsServiceCharge":
			logistics = logistics.Add(sumOf(ac, "AppliedAmount"))
		}
	}
	allowances = round2(allowances)
	charges = round2(charges).Add(round2(logistics))
	nAllowances, nCharges, nLogistics := 0, 0, 0
	for _, ac := range children(m.Parent) {
		switch {
		case ac.Name == "SpecifiedTradeAllowanceCharge" && childText(ac.Elem("ChargeIndicator"), "Indicator", "false"):
			nAllowances++
		case ac.Name == "SpecifiedTradeAllowanceCharge" && childText(ac.Elem("ChargeIndicator"), "Indicator", "true"):
			nCharges++
		case ac.Name == "SpecifiedLogisticsServiceCharge":
			nLogistics++
		}
	}
	hasAllowance, hasCharge := false, false
	for _, ac := range children(k.d.Settlement) {
		if ac.Name == "SpecifiedTradeAllowanceCharge" {
			hasAllowance = hasAllowance || childText(ac.Elem("ChargeIndicator"), "Indicator", "false")
			hasCharge = hasCharge || childText(ac.Elem("ChargeIndicator"), "Indicator", "true")
		}
	}

	// BR-FREXT-CO-10 counts every line in the tolerance.
	lt := m.Elem("LineTotalAmount")
	if v, ok := dec(lt); !ok || !near(v, lines, tolerance(allLines)) {
		k.fail("BR-FREXT-CO-10", at(lt, m), "sum of invoice line net amounts (BT-106) %q is not %s, the sum of the net amounts of the plain and DETAIL lines, within %s",
			quote(text(lt)), lines.String(), tolerance(allLines).String())
	}
	if at11 := m.Elem("AllowanceTotalAmount"); hasAllowance || at11 != nil {
		if v, ok := dec(at11); !ok || !near(v, allowances, tolerance(nAllowances)) {
			k.fail("BR-FREXT-CO-11", at(at11, m), "sum of allowances on document level (BT-107) %q is not %s within %s", quote(text(at11)), allowances.String(), tolerance(nAllowances).String())
		}
	}
	nChargeItems := nCharges + nLogistics
	if ct := m.Elem("ChargeTotalAmount"); hasCharge || ct != nil {
		if v, ok := dec(ct); !ok || !near(v, charges, tolerance(nChargeItems)) {
			k.fail("BR-FREXT-CO-12", at(ct, m), "sum of charges on document level (BT-108) %q is not %s (charges and logistics service charges) within %s", quote(text(ct)), charges.String(), tolerance(nChargeItems).String())
		}
	}
	n := nDetail + nAllowances + nChargeItems
	tb := m.Elem("TaxBasisTotalAmount")
	basis := lines.Sub(allowances).Add(charges)
	if v, ok := dec(tb); !ok || !near(v, basis, tolerance(n)) {
		k.fail("BR-FREXT-CO-13", at(tb, m), "invoice total amount without VAT (BT-109) %q is not %s (lines %s - allowances %s + charges %s) within %s",
			quote(text(tb)), basis.String(), lines.String(), allowances.String(), charges.String(), tolerance(n).String())
	}

	// BR-FREXT-CO-15, see the file comment.
	icc := m.Parent.Elem("InvoiceCurrencyCode")
	gt := m.Elem("GrandTotalAmount")
	if icc == nil {
		k.fail("BR-FREXT-CO-15", at(gt, m), "without invoice currency code (BT-5), the invoice total amount with VAT (BT-112) cannot be checked")
		return
	}
	var tax *xmltree.Node
	nTax := 0
	for it, t := elems(m, "TaxTotalAmount"); t != nil; t = it.next() {
		if cur, ok := t.LookupAttr("currencyID"); ok && cur == icc.Text {
			if tax == nil {
				tax = t
			}
			nTax++
		}
	}
	b, okB := dec(tb)
	g, okG := dec(gt)
	v, okV := dec(tax)
	switch {
	case !okB || !okG || !okV:
		k.fail("BR-FREXT-CO-15", at(gt, m), "the invoice total amount without VAT (BT-109) %q, the invoice total VAT amount (BT-110) in the invoice currency %q or the invoice total amount with VAT (BT-112) %q is missing or not a number",
			quote(text(tb)), quote(text(tax)), quote(text(gt)))
	case nTax == 1 && !near(g, b.Add(v), tolerance(n)):
		k.fail("BR-FREXT-CO-15", at(gt, m), "the invoice total amount with VAT (BT-112) %q is not BT-109 %q + BT-110 %q within %s", quote(text(gt)), quote(text(tb)), quote(text(tax)), tolerance(n).String())
	case nTax > 1 && !b.Equal(g):
		k.fail("BR-FREXT-CO-15", at(gt, m), "the invoice has %d invoice total VAT amounts (BT-110) in the invoice currency: the invoice total amounts without VAT (BT-109) %q and with VAT (BT-112) %q must then be equal", nTax, quote(text(tb)), quote(text(gt)))
	}
}

// sumOf returns the sum of the decimal values of the children of n named
// name; the other values are ignored.
func sumOf(n *xmltree.Node, name string) invoice.Decimal {
	s := zero
	for it, c := elems(n, name); c != nil; c = it.next() {
		if v, ok := dec(c); ok {
			s = s.Add(v)
		}
	}
	return s
}

// ctcfrBreakdowns checks the VAT breakdowns (BG-23) of a header settlement
// with the rules of the schematron.
func (k *checker) ctcfrBreakdowns(s *xmltree.Node) {
	for _, t := range s.Children {
		if t.Name != "ApplicableTradeTax" {
			continue
		}
		var cat *ctcfrCategory
		if isVAT(t) {
			for i := range ctcfrCategories {
				if childText(t, "CategoryCode", catCodes[ctcfrCategories[i].cat]) {
					cat = &ctcfrCategories[i]
					break
				}
			}
		}
		if cat == nil {
			k.breakdown(t)
		} else {
			k.categoryBasis08(t, cat)
			if cat.cat == catS {
				k.standardAmount(t)
			}
		}
		k.categoryCodeRules(s, t)
	}
}

// taxSelection is the predicate of the variables of the BR-FXEXT-*-08 and
// BR-FXEXT-S-09 rules on the tax elements of a line, an allowance or charge,
// or a logistics service charge.
type taxSelection struct {
	cat        string
	rate       invoice.Decimal
	byRate     bool // the rate must be rate
	noRate     bool // the breakdown has no numeric rate: no rate matches
	byReason   bool // the exemption reason and its code must be reasonText and reasonCode
	reasonText string
	reasonCode string
}

// matches reports whether the children of n named name satisfy s. As in
// XPath, the conditions are existential: one tax element has the category
// and one has the rate; the exemption reason and code are those of the
// first tax element having them, normalized ("" when absent).
func (s *taxSelection) matches(n *xmltree.Node, name string) bool {
	cat, rate := false, !s.byRate
	var reason, code *xmltree.Node
	for it, t := elems(n, name); t != nil; t = it.next() {
		cat = cat || childText(t, "CategoryCode", s.cat)
		if !rate && !s.noRate {
			r, ok := dec(t.Elem("RateApplicablePercent"))
			rate = ok && r.Equal(s.rate)
		}
		if reason == nil {
			reason = t.Elem("ExemptionReason")
		}
		if code == nil {
			code = t.Elem("ExemptionReasonCode")
		}
	}
	return cat && rate && (!s.byReason || reason.Normalized() == s.reasonText && code.Normalized() == s.reasonCode)
}

// categorySum returns the sum, each part rounded to 2 decimals, of the net
// amounts of the plain and DETAIL lines, minus the allowances, plus the
// charges and the logistics service charges that s selects, and the number
// of lines, allowances or charges, and logistics service charges selected.
func (k *checker) categorySum(s *taxSelection) (invoice.Decimal, int) {
	n := 0
	lines := zero
	for _, l := range k.d.Lines {
		if !detail(l) {
			continue
		}
		counted := false
		for it, lts := elems(l, "SpecifiedLineTradeSettlement"); lts != nil; lts = it.next() {
			if !s.matches(lts, "ApplicableTradeTax") {
				continue
			}
			if !counted {
				n++
				counted = true
			}
			for it, ms := elems(lts, "SpecifiedTradeSettlementLineMonetarySummation"); ms != nil; ms = it.next() {
				if v, ok := dec(ms.Elem("LineTotalAmount")); ok {
					lines = lines.Add(v)
				}
			}
		}
	}
	allowances, charges, logistics := zero, zero, zero
	for _, ac := range children(k.d.Settlement) {
		switch ac.Name {
		case "SpecifiedTradeAllowanceCharge":
			if !s.matches(ac, "CategoryTradeTax") {
				continue
			}
			n++
			allowance, charge := chargeIndicator(ac)
			v, ok := dec(ac.Elem("ActualAmount"))
			switch {
			case !ok:
			case charge:
				charges = charges.Add(v)
			case allowance:
				allowances = allowances.Add(v)
			}
		case "SpecifiedLogisticsServiceCharge":
			if !s.matches(ac, "AppliedTradeTax") {
				continue
			}
			n++
			if v, ok := dec(ac.Elem("AppliedAmount")); ok {
				logistics = logistics.Add(v)
			}
		}
	}
	return round2(lines).Sub(round2(allowances)).Add(round2(charges)).Add(round2(logistics)), n
}

// selection returns the selection of the lines, allowances and charges of
// the VAT breakdown tt of category c.
func selection(tt *xmltree.Node, c *ctcfrCategory) taxSelection {
	s := taxSelection{cat: catCodes[c.cat], byRate: c.rate,
		reasonText: tt.Elem("ExemptionReason").Normalized(), reasonCode: tt.Elem("ExemptionReasonCode").Normalized()}
	if c.rate {
		var ok bool
		s.rate, ok = dec(tt.Elem("RateApplicablePercent"))
		s.noRate = !ok
	}
	return s
}

// categoryBasis08 checks BR-FXEXT-*-08, -08ini and -08rev on a VAT breakdown
// tt of category c: the taxable amount (BT-116) is the sum of the plain and
// DETAIL lines, charges and logistics service charges minus the allowances
// of the category and rate (ini) or of the category, rate and exemption
// reason (rev), within 0.01 per amount summed; the rule 08 requires one of
// them.
func (k *checker) categoryBasis08(tt *xmltree.Node, c *ctcfrCategory) {
	basisNode := tt.Elem("BasisAmount")
	basis, okB := dec(basisNode)
	what := func() string {
		if !c.rate {
			return "category " + catCodes[c.cat]
		}
		return "category " + catCodes[c.cat] + " at " + quote(text(tt.Elem("RateApplicablePercent"))) + "%"
	}
	loc := at(basisNode, tt)
	if !okB {
		for _, id := range [...]string{c.ini, c.rev, c.b} {
			k.fail(id, loc, "VAT breakdown of %s: the taxable amount (BT-116) %q is missing or not a number", what(), quote(text(basisNode)))
		}
		return
	}
	s := selection(tt, c)
	ini, nIni := k.categorySum(&s)
	s.byReason = true
	rev, nRev := k.categorySum(&s)
	iniOK, revOK := near(basis, ini, tolerance(nIni)), near(basis, rev, tolerance(nRev))
	if !iniOK {
		k.fail(c.ini, loc, "VAT breakdown of %s: the taxable amount (BT-116) %q is not %s (plain and DETAIL lines, charges and logistics service charges minus allowances of that category and rate) within %s",
			what(), quote(basisNode.Text), ini.String(), tolerance(nIni).String())
	}
	if !revOK {
		k.fail(c.rev, loc, "VAT breakdown of %s: the taxable amount (BT-116) %q is not %s (plain and DETAIL lines, charges and logistics service charges minus allowances of that category, rate and exemption reason) within %s",
			what(), quote(basisNode.Text), rev.String(), tolerance(nRev).String())
	}
	if !iniOK && !revOK {
		k.fail(c.b, loc, "VAT breakdown of %s: the taxable amount (BT-116) %q is neither %s (same rate) within %s nor %s (same rate and exemption reason) within %s",
			what(), quote(basisNode.Text), ini.String(), tolerance(nIni).String(), rev.String(), tolerance(nRev).String())
	}
}

// standardAmount checks BR-FXEXT-S-09: the tax amount (BT-117) of a standard
// rated breakdown is its taxable amount × rate rounded to 2 decimals, within
// 0.01 per amount of the category and rate (with or without the exemption
// reason).
func (k *checker) standardAmount(tt *xmltree.Node) {
	rate, okR := dec(tt.Elem("RateApplicablePercent"))
	basis, okB := dec(tt.Elem("BasisAmount"))
	calcNode := tt.Elem("CalculatedAmount")
	calc, okC := dec(calcNode)
	if !okR || !okB || !okC {
		k.fail("BR-FXEXT-S-09", at(calcNode, tt), "standard rated VAT breakdown: the tax amount (BT-117), the taxable amount (BT-116) or the rate (BT-119) is missing or not a number")
		return
	}
	want := basis.Mul(rate)
	if want.Scale()+2 <= 18 {
		want = invoice.NewDecimal(want.Coef(), want.Scale()+2) // ÷ 100, exactly
	} else {
		want = want.Div(hundred, want.Scale()+2)
	}
	want = want.RoundHalfCeil(2)
	s := selection(tt, &ctcfrCategories[7])
	_, nIni := k.categorySum(&s)
	s.byReason = true
	_, nRev := k.categorySum(&s)
	if near(calc, want, tolerance(nRev)) || near(calc, want, tolerance(nIni)) {
		return
	}
	k.fail("BR-FXEXT-S-09", calcNode, "standard rated VAT breakdown at %s%%: the tax amount (BT-117) %q is not %s (taxable amount %s × rate) within %s",
		quote(text(tt.Elem("RateApplicablePercent"))), quote(calcNode.Text), want.String(), basis.String(), tolerance(nIni).String())
}
