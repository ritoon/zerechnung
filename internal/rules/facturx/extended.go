package facturx

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// This file holds the BR-FXEXT rules of the EXTENDED validation XSLT of
// Factur-X 1.09.2 (ZUGFeRD 2.5.2). In EXTENDED they replace EN 16931 rules
// that package en16931 reports as warnings for this profile (BR-CO-10 to
// BR-CO-16, BR-*-01, BR-*-08, BR-S-09, BR-22 to BR-26, BR-38, BR-44,
// BR-54): only plain and DETAIL lines (subtype BT-X-8) are summed, the
// logistics service charges (BG-X-42) count as charges, the charges
// collected on behalf of a third party (BG-34) are added to the amount due,
// and the sums get a tolerance of 0.01 per amount summed. The MINIMUM and
// BASIC WL schematrons of Factur-X 1.09 hold some of the BR-FXEXT-*-08
// rules too (see vatTotals).
//
// As elsewhere in the package, the rules follow the XPath of the XSLT. Some
// consequences are worth knowing:
//
//   - In the BR-FXEXT-*-01 rules, the predicate on the line subtype applies
//     to ram:SpecifiedLineTradeSettlement, which has no
//     ram:AssociatedDocumentLineDocument child: it is always true, so GROUP
//     and INFORMATION lines count as well.
//   - The rules whose test starts with "for $x in xs:decimal(...)" fail
//     when the element is missing, as the expression then yields an empty
//     sequence. BR-FXEXT-CO-15 thus requires a total VAT amount (BT-110) in
//     the invoice currency, although its message describes a case without
//     one.
//   - BR-FXEXT-CII-DT-097a accepts 12 digits (CCYYMMDDHHMM), although its
//     message mentions the seconds.
//
// BR-FXEXT-08, whose context is every line but whose test covers every
// group, is reported once per failing group line rather than on every line.

// fxextCategory describes the VAT category rules BR-FXEXT-*-01 and
// BR-FXEXT-*-08.
type fxextCategory struct {
	code             string // VAT category code (BT-118)
	one, b, ini, rev string // rule identifiers
	rate             bool   // the rules group the amounts by rate (all but O)
}

var fxextCategories = [...]fxextCategory{
	{"S", "BR-FXEXT-S-01", "BR-FXEXT-S08b", "BR-FXEXT-S-08ini", "BR-FXEXT-S-08rev", true},
	{"Z", "BR-FXEXT-Z-01", "BR-FXEXT-Z-08b", "BR-FXEXT-Z-08ini", "BR-FXEXT-Z-08rev", true},
	{"E", "BR-FXEXT-E-01", "BR-FXEXT-E-08b", "BR-FXEXT-E-08ini", "BR-FXEXT-E-08rev", true},
	{"AE", "BR-FXEXT-AE-01", "BR-FXEXT-AE-08b", "BR-FXEXT-AE-08ini", "BR-FXEXT-AE-08rev", true},
	{"K", "BR-FXEXT-IC-01", "BR-FXEXT-IC-08b", "BR-FXEXT-IC-08ini", "BR-FXEXT-IC-08rev", true},
	{"G", "BR-FXEXT-G-01", "BR-FXEXT-G-08b", "BR-FXEXT-G-08ini", "BR-FXEXT-G-08rev", true},
	{"O", "BR-FXEXT-O-01", "BR-FXEXT-O-08b", "BR-FXEXT-O-08ini", "BR-FXEXT-O-08rev", false},
	{"L", "BR-FXEXT-AF-01", "BR-FXEXT-AF-08b", "BR-FXEXT-AF-08ini", "BR-FXEXT-AF-08rev", true},
	{"M", "BR-FXEXT-AG-01", "BR-FXEXT-AG-08b", "BR-FXEXT-AG-08ini", "BR-FXEXT-AG-08rev", true},
}

// characteristicCodes is the code list of BR-FXEXT-04: UNTDED 6313 and the
// Factur-X extension, for the item attribute code (BT-X-11).
var characteristicCodes = sync.OnceValue(func() map[string]bool {
	return codeSet("A AAA AAB AAC AAD AAF AAG AAH AAI AAJ AAK AAM AAN AAO AAP AAQ AAR AAS AAT AAU AAV AAW AAX AAY AAZ ABA ABB ABC ABD ABE ABF ABG ABH ABI ABJ ABK ABL ABM ABN ABO ABP ABS ABT ABX ABY ABZ ACA ACE ACG ACN ACP ACS ACV ACW ACX ADR ADS ADT ADU ADV ADW ADX ADY ADZ AEA AEB AEC AED AEE AEF AEG AEH AEI AEJ AEK AEL AEM AEN AEO AEP AEQ AER AES AET AEU AEV AEW AEX AEY AEZ AF AFA AFB AFC AFD AFE AFF AFG AFH AFI AFJ AFK AFL AFM AFN AFO AFP AFQ AFR AFS AFT AFU AFV AFW AFX B BL BMY BMZ BNA BNB BNC BND BNE BNF BNG BNH BNI BNJ BNK BNL BNM BNN BNO BNP BNQ BNR BNS BNT BNU BNV BNW BNX BNY BNZ BR BRA BRB BRC BRD BRE BRF BRG BRH BRI BRJ BRK BRL BRM BRN BRO BRP BRQ BRR BRS BRT BRU BRV BS BSW BSX BSY BSZ BTA BTB BTC BTD BTE BTF BTG BTH BTI BTJ BTK BTL BTM BW CHN CHO CM CT CV CZ D DI DL DN DP DR DS DW E EA F FI FL FN FV GG GW HF HM HT IB ID L LM LN LND M MO MW N OD PRS PTN RA RF RJ RMW RP RUN RY SQ T TC TH TN TT VGM VH VW WA WD WM WU XH XQ XZ YS ZAL ZAS ZB ZBI ZC ZCA ZCB ZCE ZCL ZCO ZCR ZCU ZFE ZFS ZGE ZH ZK ZMG ZMN ZMO ZN ZNA ZNB ZNI ZO ZP ZPB ZS ZSB ZSE ZSI ZSL ZSN ZTA ZTE ZTI ZV ZW ZWA ZZN ZZR ZZZ BEST_BEFORE_DATE COLOR_TEXT COMMISSION DEPOSIT_SYSTEM DEPOSIT_TYPE ENERGY_CLASS EXPIRATION_DATE FEE KIND_OF_ARTICLE MATERIAL METER_LOCATION METER_NUMBER ORGANIC_CONTROL_BODY PACKAGING_MATERIAL PACKAGING_TYPE PROMOTIONAL_VARIANT SEAL_NUMBER SIZE_CODE SIZE_TEXT TRADING_UNIT WASTE_CODE WASTE_FRACTION WEEE_NUMBER")
})

// dateTime205 is the pattern of BR-FXEXT-CII-DT-097a. \d of XPath matches
// any decimal digit, \s the XML white space.
var dateTime205 = regexp.MustCompile(`^[ \t\n\r]*\p{Nd}{4}(1[0-2]|0[1-9])(3[01]|[12][0-9]|0[1-9])([01][0-9]|2[0-3])[0-5][0-9][ \t\n\r]*$`)

var hundred = invoice.DecimalFromInt(100)

// extState collects what the BR-FXEXT-*-01 rules need from the whole
// document: the VAT categories of the line taxes, of the allowances and
// charges (ram:CategoryTradeTax) and of the logistics service charges
// (ram:AppliedTradeTax), as bits indexed like fxextCategories, and the first
// element having each.
type extState struct {
	used   uint16
	usedAt [len(fxextCategories)]*xmltree.Node
}

// use records the categories of the tax element t.
func (s *extState) use(t *xmltree.Node) {
	for i := range fxextCategories {
		if s.used&(1<<i) == 0 && hasCategory(t, fxextCategories[i].code) {
			s.used |= 1 << i
			s.usedAt[i] = t
		}
	}
}

// extended runs the BR-FXEXT rules of the EXTENDED profile.
func (k *checker) extended() {
	d := k.d
	var s extState
	k.extWalk(d.Root, false, &s)
	k.categoriesUsed(&s)
	for _, note := range kids(d.Header) {
		if note.Name == "IncludedNote" {
			k.noteContent("BR-FXEXT-01", note, "invoice note (BG-1)")
		}
	}
	for _, l := range d.Lines {
		k.extLine(l)
	}
	for _, ac := range kids(d.Settlement) {
		if ac.Name == "SpecifiedTradeAllowanceCharge" {
			k.chargeReason("BR-FXEXT-BR-38", ac, "document level charge (BG-21)", "BT-104", "BT-105")
		}
	}
	k.extTotals()
	k.vatTotals(true)
}

// extWalk visits the whole document once: BR-FXEXT-03 (tax registrations
// of the parties other than the seller), BR-FXEXT-04 and BR-FXEXT-BR-54
// (item attributes), BR-FXEXT-CII-DT-097a (dates in format 205) and the
// categories needed by the BR-FXEXT-*-01 rules, whose contexts use the
// descendant axis.
func (k *checker) extWalk(n *xmltree.Node, inSeller bool, s *extState) {
	switch n.Name {
	case "SellerTradeParty":
		inSeller = true
	case "ApplicableTradeTax":
		if n.Parent != nil && n.Parent.Name == "SpecifiedLineTradeSettlement" {
			s.use(n)
		}
	case "CategoryTradeTax", "AppliedTradeTax":
		s.use(n)
	case "ApplicableProductCharacteristic":
		k.characteristic(n)
	case "ID":
		if !inSeller && n.Parent != nil && n.Parent.Name == "SpecifiedTaxRegistration" {
			if scheme, ok := n.LookupAttr("schemeID"); !ok || scheme != "VA" {
				k.fail("BR-FXEXT-03", n, "the tax registration %q of %s has scheme %q: only the seller may have other tax registrations than a VAT identifier (schemeID VA)",
					strings.TrimSpace(n.Text), display(n.Parent.Parent), scheme)
			}
		}
	case "DateTimeString":
		if n.Attr("format") == "205" && !dateTime205.MatchString(n.Text) {
			k.fail("BR-FXEXT-CII-DT-097a", n, "%s: %q does not match format 205 as the official rule checks it (CCYYMMDDHHMM)", display(n.Parent), strings.TrimSpace(n.Text))
		}
	}
	for _, ch := range n.Children {
		k.extWalk(ch, inSeller, s)
	}
}

// categoriesUsed implements BR-FXEXT-S-01 to BR-FXEXT-AG-01: a VAT category
// is in the VAT breakdown (BG-23) if and only if a line, an allowance or
// charge or a logistics service charge has it.
func (k *checker) categoriesUsed(s *extState) {
	var header uint16
	var headerAt [len(fxextCategories)]*xmltree.Node
	for _, tt := range kids(k.d.Settlement) {
		if tt.Name != "ApplicableTradeTax" {
			continue
		}
		for i := range fxextCategories {
			if header&(1<<i) == 0 && hasCategory(tt, fxextCategories[i].code) {
				header |= 1 << i
				headerAt[i] = tt
			}
		}
	}
	for i := range fxextCategories {
		c := &fxextCategories[i]
		h, u := header&(1<<i) != 0, s.used&(1<<i) != 0
		switch {
		case h && !u:
			k.fail(c.one, headerAt[i], "the VAT breakdown (BG-23) has category %s, but no invoice line, allowance, charge or logistics service charge has it", c.code)
		case u && !h:
			k.fail(c.one, s.usedAt[i], "%s has VAT category %s, but the VAT breakdown (BG-23) has no entry of that category", display(s.usedAt[i].Parent), c.code)
		}
	}
}

// noteContent implements BR-FXEXT-01 and BR-FXEXT-02: a note with a subject
// code has a content or a content code.
func (k *checker) noteContent(id string, note *xmltree.Node, what string) {
	if note.Elem("SubjectCode") != nil && note.Elem("Content") == nil && note.Elem("ContentCode") == nil {
		k.fail(id, note, "the %s has a subject code %q but neither a content nor a content code", what, strings.TrimSpace(text(note.Elem("SubjectCode"))))
	}
}

// chargeReason implements BR-FXEXT-BR-38 and BR-FXEXT-BR-44: a charge
// (ram:ChargeIndicator/udt:Indicator = 'true') has a reason or a reason
// code.
func (k *checker) chargeReason(id string, ac *xmltree.Node, what, reason, code string) {
	for _, ci := range ac.Children {
		if ci.Name != "ChargeIndicator" || !hasText(ci, "Indicator", "true") {
			continue
		}
		if ac.Elem("Reason") == nil && ac.Elem("ReasonCode") == nil {
			k.fail(id, ci, "the %s has neither a reason (%s) nor a reason code (%s)", what, reason, code)
		}
	}
}

// characteristic implements BR-FXEXT-BR-54-1, BR-FXEXT-BR-54-2 and
// BR-FXEXT-04 on an item attribute (BG-32).
func (k *checker) characteristic(c *xmltree.Node) {
	var desc, typeCode, value, measure, measureUnit bool
	for _, ch := range c.Children {
		switch ch.Name {
		case "Description":
			desc = true
		case "TypeCode":
			typeCode = true
			if v := ch.Normalized(); !characteristicCodes()[v] {
				k.fail("BR-FXEXT-04", ch, "the item attribute code (BT-X-11) %q is not in UNTDED 6313 or the Factur-X extension", v)
			}
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
		k.fail("BR-FXEXT-BR-54-1", c, "the item attribute (BG-32) has neither a name (BT-160) nor a code (BT-X-11)")
	}
	if !(value && !measure || !value && measureUnit) {
		k.fail("BR-FXEXT-BR-54-2", c, "the item attribute (BG-32) must have either a value (BT-161) or a value with a unit of measure (BT-X-12), not both")
	}
}

// extLine implements the line rules: BR-FXEXT-02, BR-FXEXT-06, BR-FXEXT-08,
// BR-FXEXT-11, BR-FXEXT-12, BR-FXEXT-BR-22 to BR-FXEXT-BR-26,
// BR-FXEXT-BR-44 and BR-FXEXT-CO-04.
func (k *checker) extLine(l *xmltree.Node) {
	adl := l.Elem("AssociatedDocumentLineDocument")
	lineID := adl.Elem("LineID").Normalized()
	for _, note := range kids(adl) {
		if note.Name == "IncludedNote" {
			k.noteContent("BR-FXEXT-02", note, "note of line "+lineID)
		}
	}
	if parent := adl.Elem("ParentLineID"); parent.Normalized() != "" {
		if !hasStatus(adl, func(s string) bool { return s != "" }) {
			k.fail("BR-FXEXT-06", parent, "line %s has a parent line (BT-X-304) but no subtype (BT-X-8)", lineID)
		}
		if !k.lineExists(parent.Normalized()) {
			k.fail("BR-FXEXT-11", parent, "the parent line %q (BT-X-304) of line %s is not the identifier (BT-126) of a line", parent.Normalized(), lineID)
		}
	}
	lts := l.Elem("SpecifiedLineTradeSettlement")
	if adl != nil && summed(adl) {
		qty := l.Find("SpecifiedLineTradeDelivery/BilledQuantity")
		if qty == nil {
			k.fail("BR-FXEXT-BR-22", adl, "line %s has no invoiced quantity (BT-129)", lineID)
		}
		unit := false
		for _, q := range l.FindAll("SpecifiedLineTradeDelivery/BilledQuantity") {
			if _, ok := q.LookupAttr("unitCode"); ok {
				unit = true
			}
		}
		if !unit {
			k.fail("BR-FXEXT-BR-23", or(qty, adl), "line %s has no unit of measure code (BT-130) for its invoiced quantity", lineID)
		}
		if lts.Find("SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount") == nil {
			k.fail("BR-FXEXT-BR-24", adl, "line %s has no net amount (BT-131)", lineID)
		}
		if l.Find("SpecifiedLineTradeAgreement/NetPriceProductTradePrice/ChargeAmount") == nil {
			k.fail("BR-FXEXT-BR-26", adl, "line %s has no item net price (BT-146)", lineID)
		}
		vat, cat := false, false
		for _, t := range kids(lts) {
			if t.Name != "ApplicableTradeTax" {
				continue
			}
			vat = vat || hasText(t, "TypeCode", "VAT")
			cat = cat || strings.ToUpper(text(t.Elem("TypeCode"))) == "VAT" && t.Elem("CategoryCode") != nil
		}
		if !vat || !cat {
			k.fail("BR-FXEXT-CO-04", adl, "line %s has no VAT category code (BT-151) in a tax of type VAT", lineID)
		}
	}
	for _, ac := range kids(lts) {
		if ac.Name == "SpecifiedTradeAllowanceCharge" {
			k.chargeReason("BR-FXEXT-BR-44", ac, "charge (BG-28) of line "+lineID, "BT-144", "BT-145")
		}
	}
	if hasStatus(adl, func(s string) bool { return s == "GROUP" }) {
		k.group(l, adl, lineID)
	}
}

// lineTotalPath leads from a line to its net amount (BT-131).
const lineTotalPath = "SpecifiedLineTradeSettlement/SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount"

// group implements BR-FXEXT-08 and BR-FXEXT-12 on a GROUP line: when it has
// a net amount, the amount is the sum of the net amounts of its DETAIL and
// GROUP sub-lines (rounded to 2 decimals), all of which have one.
func (k *checker) group(l, adl *xmltree.Node, lineID string) {
	lt := l.Find(lineTotalPath)
	if lt == nil {
		return
	}
	if v := lt.Normalized(); v != "" {
		sum := invoice.NewDecimal(0, 0)
		var missing *xmltree.Node
		for _, c := range k.d.Lines {
			cadl := c.Elem("AssociatedDocumentLineDocument")
			if cadl.Elem("ParentLineID").Normalized() != lineID || !hasStatus(cadl, func(s string) bool { return s == "DETAIL" || s == "GROUP" }) {
				continue
			}
			cv, err := invoice.ParseDecimal(c.Find(lineTotalPath).Normalized())
			if err != nil {
				if missing == nil {
					missing = cadl
				}
				continue
			}
			sum = sum.Add(cv)
		}
		gv, err := invoice.ParseDecimal(v)
		switch want := sum.RoundHalfCeil(2); {
		case missing != nil:
			k.fail("BR-FXEXT-08", lt, "group line %s has a net amount (BT-131), but its sub-line %s has no numeric net amount", lineID, missing.Elem("LineID").Normalized())
		case err != nil || !gv.Equal(want):
			k.fail("BR-FXEXT-08", lt, "the net amount (BT-131) %s of group line %s is not %s, the sum of the net amounts of its DETAIL and GROUP sub-lines", v, lineID, want)
		}
	}
	ids := 0
	for _, id := range adl.Children {
		if id.Name != "LineID" {
			continue
		}
		ids++
		groups, amounts := 0, 0
		for _, c := range k.d.Lines {
			cadl := c.Elem("AssociatedDocumentLineDocument")
			if hasText(cadl, "ParentLineID", id.Text) && hasStatus(cadl, func(s string) bool { return s == "GROUP" }) {
				groups++
				amounts += c.Count(lineTotalPath)
			}
		}
		if groups != amounts {
			k.fail("BR-FXEXT-12", lt, "group line %s has a net amount (BT-131), but not all its GROUP sub-lines have one", lineID)
		}
	}
	if ids == 0 {
		k.fail("BR-FXEXT-12", lt, "the group line has a net amount (BT-131) but no line identifier (BT-126)")
	}
}

// lineExists reports whether a line has the identifier id (BT-126,
// normalized).
func (k *checker) lineExists(id string) bool {
	for _, l := range k.d.Lines {
		for _, adl := range l.Children {
			if adl.Name != "AssociatedDocumentLineDocument" {
				continue
			}
			for _, lid := range adl.Children {
				if lid.Name == "LineID" && lid.Normalized() == id {
					return true
				}
			}
		}
	}
	return false
}

// summed reports whether the line whose ram:AssociatedDocumentLineDocument
// is adl is summed in the totals: it has no subtype (BT-X-8) or the subtype
// DETAIL.
func summed(adl *xmltree.Node) bool {
	status := false
	for _, ch := range kids(adl) {
		if ch.Name == "LineStatusReasonCode" {
			if ch.Text == "DETAIL" {
				return true
			}
			status = true
		}
	}
	return !status
}

// hasStatus reports whether a line subtype (ram:LineStatusReasonCode) of adl
// satisfies ok.
func hasStatus(adl *xmltree.Node, ok func(string) bool) bool {
	for _, ch := range kids(adl) {
		if ch.Name == "LineStatusReasonCode" && ok(ch.Text) {
			return true
		}
	}
	return false
}

// hasText reports whether n has a child named name whose string value is s.
func hasText(n *xmltree.Node, name, s string) bool {
	for _, ch := range kids(n) {
		if ch.Name == name && ch.Text == s {
			return true
		}
	}
	return false
}

// tolerance returns 0.01 × n.
func tolerance(n int) invoice.Decimal { return invoice.DecimalFromInt(int64(n)).Mul(hundredth) }

// within reports whether |a − b| <= tol.
func within(a, b, tol invoice.Decimal) bool { return a.Sub(b).Abs().Cmp(tol) <= 0 }

// extTotals implements the rules of context
// //ram:SpecifiedTradeSettlementHeaderMonetarySummation: BR-FXEXT-CO-10 to
// BR-FXEXT-CO-13, BR-FXEXT-CO-15 and BR-FXEXT-CO-16.
func (k *checker) extTotals() {
	sum := k.d.Summation
	if sum == nil {
		return
	}
	st := k.d.Settlement

	lines, nSummed := invoice.NewDecimal(0, 0), 0
	for _, l := range k.d.Lines {
		if summed(l.Elem("AssociatedDocumentLineDocument")) {
			nSummed++
			lines = lines.Add(rules.SumDec(l.FindAll(lineTotalPath)))
		}
	}
	lines = lines.RoundHalfCeil(2)
	allowances, charges, logistics := invoice.NewDecimal(0, 0), invoice.NewDecimal(0, 0), invoice.NewDecimal(0, 0)
	nAllowances, nCharges, nLogistics := 0, 0, 0
	for _, ac := range kids(st) {
		switch {
		case ac.Name == "SpecifiedTradeAllowanceCharge" && hasText(ac.Elem("ChargeIndicator"), "Indicator", "false"):
			nAllowances++
			allowances = allowances.Add(sumDec(ac, "ActualAmount"))
		case ac.Name == "SpecifiedTradeAllowanceCharge" && hasText(ac.Elem("ChargeIndicator"), "Indicator", "true"):
			nCharges++
			charges = charges.Add(sumDec(ac, "ActualAmount"))
		case ac.Name == "SpecifiedLogisticsServiceCharge":
			nLogistics++
			logistics = logistics.Add(sumDec(ac, "AppliedAmount"))
		}
	}
	allowances = allowances.RoundHalfCeil(2)
	charges = charges.RoundHalfCeil(2).Add(logistics.RoundHalfCeil(2))
	nChargeItems := nCharges + nLogistics

	// BR-FXEXT-CO-10 counts every line in the tolerance.
	lt := sum.Elem("LineTotalAmount")
	if v, ok := rules.Dec(lt); !ok || !within(v, lines, tolerance(len(k.d.Lines))) {
		k.fail("BR-FXEXT-CO-10", or(lt, sum), "the sum of invoice line net amounts (BT-106) is %s, expected %s (net amounts of the plain and DETAIL lines) within %s",
			valueOf(lt), lines, tolerance(len(k.d.Lines)))
	}
	if at := sum.Elem("AllowanceTotalAmount"); nAllowances > 0 || at != nil {
		if v, ok := rules.Dec(at); !ok || !within(v, allowances, tolerance(nAllowances)) {
			k.fail("BR-FXEXT-CO-11", or(at, sum), "the sum of allowances on document level (BT-107) is %s, expected %s within %s", valueOf(at), allowances, tolerance(nAllowances))
		}
	}
	if ct := sum.Elem("ChargeTotalAmount"); nCharges > 0 || ct != nil {
		if v, ok := rules.Dec(ct); !ok || !within(v, charges, tolerance(nChargeItems)) {
			k.fail("BR-FXEXT-CO-12", or(ct, sum), "the sum of charges on document level (BT-108) is %s, expected %s (charges and logistics service charges) within %s", valueOf(ct), charges, tolerance(nChargeItems))
		}
	}
	n := nSummed + nAllowances + nChargeItems
	tb := sum.Elem("TaxBasisTotalAmount")
	basis := lines.Sub(allowances).Add(charges)
	if v, ok := rules.Dec(tb); !ok || !within(v, basis, tolerance(n)) {
		k.fail("BR-FXEXT-CO-13", or(tb, sum), "the invoice total amount without VAT (BT-109) is %s, expected %s (lines %s - allowances %s + charges %s) within %s",
			valueOf(tb), basis, lines, allowances, charges, tolerance(n))
	}

	// BR-FXEXT-CO-15: exactly one BT-110 in the invoice currency, see the
	// file comment.
	gt := sum.Elem("GrandTotalAmount")
	if icc := st.Elem("InvoiceCurrencyCode"); icc == nil {
		k.fail("BR-FXEXT-CO-15", or(gt, sum), "without invoice currency code (BT-5), the invoice total amount with VAT (BT-112) cannot be checked")
	} else {
		var tax *xmltree.Node
		nTax := 0
		for _, t := range sum.Children {
			if t.Name == "TaxTotalAmount" {
				if cur, ok := t.LookupAttr("currencyID"); ok && cur == icc.Text {
					tax = t
					nTax++
				}
			}
		}
		b, okB := rules.Dec(tb)
		g, okG := rules.Dec(gt)
		v, okV := rules.Dec(tax)
		switch {
		case nTax != 1:
			k.fail("BR-FXEXT-CO-15", or(gt, sum), "the invoice has %d invoice total VAT amounts (BT-110) in the invoice currency %q: the official rule requires exactly one", nTax, icc.Text)
		case !okB || !okG || !okV || !within(g, b.Add(v), tolerance(n)):
			k.fail("BR-FXEXT-CO-15", or(gt, sum), "the invoice total amount with VAT (BT-112) is %s, expected BT-109 %s + BT-110 %s within %s", valueOf(gt), valueOf(tb), valueOf(tax), tolerance(n))
		}
	}

	// BR-FXEXT-CO-16: exact, with the charges collected on behalf of a third
	// party (BG-34).
	adjustments := invoice.NewDecimal(0, 0)
	for _, fa := range kids(st) {
		if fa.Name == "SpecifiedFinancialAdjustment" {
			adjustments = adjustments.Add(sumDec(fa, "ActualAmount"))
		}
	}
	due := sumDec(sum, "GrandTotalAmount").Sub(sumDec(sum, "TotalPrepaidAmount")).Add(sumDec(sum, "RoundingAmount")).Add(adjustments)
	dp := sum.Elem("DuePayableAmount")
	if v, ok := rules.Dec(dp); !ok || !v.Equal(due) {
		k.fail("BR-FXEXT-CO-16", or(dp, sum), "the amount due for payment (BT-115) is %s, expected %s (BT-112 - BT-113 + BT-114 + charges on behalf of a third party BT-179 %s)", valueOf(dp), due, adjustments)
	}
}

// taxSelection is the predicate of the variables of the BR-FXEXT-*-08 and
// BR-FXEXT-S-09b rules on the tax elements of a line, an allowance or
// charge, or a logistics service charge.
type taxSelection struct {
	cat        string
	rate       invoice.Decimal
	byRate     bool // the rate must be rate
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
	for _, t := range kids(n) {
		if t.Name != name {
			continue
		}
		cat = cat || hasCategory(t, s.cat)
		if !rate {
			r, ok := rules.Dec(t.Elem("RateApplicablePercent"))
			rate = ok && r.Equal(s.rate)
		}
		reason, code = or(reason, t.Elem("ExemptionReason")), or(code, t.Elem("ExemptionReasonCode"))
	}
	return cat && rate && (!s.byReason || reason.Normalized() == s.reasonText && code.Normalized() == s.reasonCode)
}

// reasonDesc describes the exemption reason of s for messages.
func (s *taxSelection) reasonDesc() string {
	if s.reasonText == "" && s.reasonCode == "" {
		return "without exemption reason"
	}
	return "with exemption reason " + strconv.Quote(s.reasonText) + " and code " + strconv.Quote(s.reasonCode)
}

// categorySums returns the sum, each part rounded to 2 decimals, of the net
// amounts of the lines, minus the allowances, plus the charges and the
// logistics service charges that s selects, and the number of amounts
// summed. With detailOnly, only plain and DETAIL lines count.
func (k *checker) categorySums(s *taxSelection, detailOnly bool) (invoice.Decimal, int) {
	n := 0
	lines := invoice.NewDecimal(0, 0)
	for _, l := range k.d.Lines {
		if detailOnly && !summed(l.Elem("AssociatedDocumentLineDocument")) {
			continue
		}
		lts := l.Elem("SpecifiedLineTradeSettlement")
		if !s.matches(lts, "ApplicableTradeTax") {
			continue
		}
		n++
		for _, ms := range lts.Children {
			if ms.Name == "SpecifiedTradeSettlementLineMonetarySummation" {
				if v, ok := rules.Dec(ms.Elem("LineTotalAmount")); ok {
					lines = lines.Add(v)
				}
			}
		}
	}
	allowances, charges, logistics := invoice.NewDecimal(0, 0), invoice.NewDecimal(0, 0), invoice.NewDecimal(0, 0)
	for _, ac := range kids(k.d.Settlement) {
		switch ac.Name {
		case "SpecifiedTradeAllowanceCharge":
			if !s.matches(ac, "CategoryTradeTax") {
				continue
			}
			n++
			charge, ok := xsBoolean(text(ac.Elem("ChargeIndicator").Elem("Indicator")))
			v, okV := rules.Dec(ac.Elem("ActualAmount"))
			switch {
			case !ok || !okV:
			case charge:
				charges = charges.Add(v)
			default:
				allowances = allowances.Add(v)
			}
		case "SpecifiedLogisticsServiceCharge":
			if !s.matches(ac, "AppliedTradeTax") {
				continue
			}
			n++
			if v, ok := rules.Dec(ac.Elem("AppliedAmount")); ok {
				logistics = logistics.Add(v)
			}
		}
	}
	return lines.RoundHalfCeil(2).Sub(allowances.RoundHalfCeil(2)).Add(charges.RoundHalfCeil(2)).Add(logistics.RoundHalfCeil(2)), n
}

// vatTotals implements the rules of context
// //ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax: the
// BR-FXEXT-*-08 rules (on the breakdowns of type VAT), BR-FXEXT-S-09b and,
// for MINIMUM, BR-FXEXT-G-08. detailOnly is set for EXTENDED, whose rules
// only sum plain and DETAIL lines.
func (k *checker) vatTotals(detailOnly bool) {
	for _, tt := range kids(k.d.Settlement) {
		if tt.Name != "ApplicableTradeTax" {
			continue
		}
		vat := strings.ToUpper(text(tt.Elem("TypeCode"))) == "VAT"
		for i := range fxextCategories {
			c := &fxextCategories[i]
			if vat && hasCategory(tt, c.code) && (k.applies(c.b) || k.applies(c.ini) || k.applies(c.rev)) {
				k.categoryBasis08(tt, c, detailOnly)
			}
		}
		if hasCategory(tt, "S") && k.applies("BR-FXEXT-S-09b") {
			k.brS09b(tt)
		}
		if k.applies("BR-FXEXT-G-08") {
			for _, cc := range tt.Children {
				if cc.Name == "CategoryCode" && cc.Text == "G" {
					k.brG08(tt, cc)
				}
			}
		}
	}
}

// categoryBasis08 implements BR-FXEXT-*-08b, -08ini and -08rev on a VAT
// breakdown tt of category c: the taxable amount (BT-116) is the sum of the
// lines, charges and logistics service charges minus the allowances of the
// category and rate (ini) or of the category, rate and exemption reason
// (rev), within 0.01 per amount summed; b requires one of them.
func (k *checker) categoryBasis08(tt *xmltree.Node, c *fxextCategory, detailOnly bool) {
	basisNode := tt.Elem("BasisAmount")
	basis, okB := rules.Dec(basisNode)
	s := taxSelection{cat: c.code, byRate: c.rate,
		reasonText: tt.Elem("ExemptionReason").Normalized(), reasonCode: tt.Elem("ExemptionReasonCode").Normalized()}
	okR, what := true, "category "+c.code
	if c.rate {
		rate := tt.Elem("RateApplicablePercent")
		s.rate, okR = rules.Dec(rate)
		what += " at " + valueOf(rate) + "%"
	}
	loc := or(basisNode, tt)
	if !okB || !okR {
		for _, id := range [...]string{c.b, c.ini, c.rev} {
			k.fail(id, loc, "VAT breakdown of %s: the taxable amount (BT-116) or the rate (BT-119) is missing or not a number", what)
		}
		return
	}
	ini, nIni := k.categorySums(&s, detailOnly)
	s.byReason = true
	rev, nRev := k.categorySums(&s, detailOnly)
	iniOK, revOK := within(basis, ini, tolerance(nIni)), within(basis, rev, tolerance(nRev))
	if !iniOK {
		k.fail(c.ini, loc, "VAT breakdown of %s: the taxable amount (BT-116) %s is not %s (lines, charges and logistics service charges minus allowances of that category and rate) within %s",
			what, valueOf(basisNode), ini, tolerance(nIni))
	}
	if !revOK {
		k.fail(c.rev, loc, "VAT breakdown of %s: the taxable amount (BT-116) %s is not %s (lines, charges and logistics service charges minus allowances of that category and rate, %s) within %s",
			what, valueOf(basisNode), rev, s.reasonDesc(), tolerance(nRev))
	}
	if !iniOK && !revOK {
		k.fail(c.b, loc, "VAT breakdown of %s: the taxable amount (BT-116) %s is neither %s (same rate) within %s nor %s (same rate and exemption reason) within %s",
			what, valueOf(basisNode), ini, tolerance(nIni), rev, tolerance(nRev))
	}
}

// brS09b implements BR-FXEXT-S-09b: the tax amount (BT-117) of a standard
// rated breakdown is its taxable amount × rate rounded to 2 decimals,
// within 0.01 per amount of the category and rate (with or without the
// exemption reason).
func (k *checker) brS09b(tt *xmltree.Node) {
	rate, okR := rules.Dec(tt.Elem("RateApplicablePercent"))
	basis, okB := rules.Dec(tt.Elem("BasisAmount"))
	calcNode := tt.Elem("CalculatedAmount")
	calc, okC := rules.Dec(calcNode)
	if !okR || !okB || !okC {
		k.fail("BR-FXEXT-S-09b", or(calcNode, tt), "standard rated VAT breakdown: the tax amount (BT-117), taxable amount (BT-116) or rate (BT-119) is missing or not a number")
		return
	}
	want := basis.Mul(rate).Div(hundred, basis.Scale()+rate.Scale()+2).RoundHalfCeil(2)
	s := taxSelection{cat: "S", rate: rate, byRate: true,
		reasonText: tt.Elem("ExemptionReason").Normalized(), reasonCode: tt.Elem("ExemptionReasonCode").Normalized()}
	_, nIni := k.categorySums(&s, true)
	s.byReason = true
	_, nRev := k.categorySums(&s, true)
	if within(calc, want, tolerance(nRev)) || within(calc, want, tolerance(nIni)) {
		return
	}
	k.fail("BR-FXEXT-S-09b", calcNode, "standard rated VAT breakdown at %s%%: the tax amount (BT-117) %s is not %s (taxable amount %s × rate) within %s",
		valueOf(tt.Elem("RateApplicablePercent")), valueOf(calcNode), want, valueOf(tt.Elem("BasisAmount")), tolerance(nIni))
}

// brG08 implements BR-FXEXT-G-08 of the MINIMUM schematron
// (FX-SCH-A-000310): the taxable amount of an export breakdown is the sum of
// its plain and DETAIL lines, charges and logistics service charges minus
// allowances, within 0.01 per amount summed.
func (k *checker) brG08(tt, cc *xmltree.Node) {
	basisNode := tt.Elem("BasisAmount")
	basis, ok := rules.Dec(basisNode)
	want, n := k.categorySums(&taxSelection{cat: "G"}, true)
	if !ok || !within(basis, want, tolerance(n)) {
		k.fail("BR-FXEXT-G-08", or(basisNode, cc), "VAT breakdown of category G: the taxable amount (BT-116) %s is not %s (lines, charges and logistics service charges minus allowances of the category) within %s",
			valueOf(basisNode), want, tolerance(n))
	}
}
