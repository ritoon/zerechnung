package de

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rules of the Peppol patterns that the XRechnung schematrons include, before
// their own patterns. They are copies of Peppol BIS Billing 3.0 rules, which
// KoSIT adapted: the identifiers are those of Peppol, but the contexts, the
// tests and the flags are those of the XRechnung schematrons, which differ
// from the Peppol schematrons (see the package documentation).
//
// CII (XRechnung-CII-validation.sch):
//
//   - peppol-cii-pattern-1: R001, R005, R053, R054, R055, R010, R020, R041,
//     R042, R040, R043-1, R043-2, R061, R110, R111, R120, R101, R121, R130;
//   - peppol-cii-pattern-2: R008 (empty elements);
//   - peppol-cii-pattern-3: R044 and R046 (gross price).
//
// UBL (XRechnung-UBL-validation.sch):
//
//   - peppol-ubl-pattern-1: R008;
//   - peppol-ubl-pattern-2: R001, R053, R054, R055, R005, R010, R020, R041,
//     R042, R040, R043, R061, R110, R111, R120, R121, R101, R044, R046, R130.
//
// Within a pattern, an element is checked by the first rule whose context it
// matches.

// Rule identifiers.
const (
	rP001  = "PEPPOL-EN16931-R001"
	rP005  = "PEPPOL-EN16931-R005"
	rP008  = "PEPPOL-EN16931-R008"
	rP010  = "PEPPOL-EN16931-R010"
	rP020  = "PEPPOL-EN16931-R020"
	rP040  = "PEPPOL-EN16931-R040"
	rP041  = "PEPPOL-EN16931-R041"
	rP042  = "PEPPOL-EN16931-R042"
	rP043  = "PEPPOL-EN16931-R043"
	rP0431 = "PEPPOL-EN16931-R043-1"
	rP0432 = "PEPPOL-EN16931-R043-2"
	rP044  = "PEPPOL-EN16931-R044"
	rP046  = "PEPPOL-EN16931-R046"
	rP053  = "PEPPOL-EN16931-R053"
	rP054  = "PEPPOL-EN16931-R054"
	rP055  = "PEPPOL-EN16931-R055"
	rP061  = "PEPPOL-EN16931-R061"
	rP101  = "PEPPOL-EN16931-R101"
	rP110  = "PEPPOL-EN16931-R110"
	rP111  = "PEPPOL-EN16931-R111"
	rP120  = "PEPPOL-EN16931-R120"
	rP121  = "PEPPOL-EN16931-R121"
	rP130  = "PEPPOL-EN16931-R130"
)

// Messages of the XRechnung schematrons, without the identifier.
const (
	mP001 = "Business process MUST be provided."
	mP005 = "VAT accounting currency code MUST be different from invoice currency code when provided."
	mP008 = "Document MUST not contain empty elements."
	mP010 = "Buyer electronic address MUST be provided"
	mP020 = "Seller electronic address MUST be provided"
	mP040 = "Allowance/charge amount must equal base amount * percentage/100 if base amount and percentage exists"
	mP041 = "Allowance/charge base amount MUST be provided when allowance/charge percentage is provided."
	mP042 = "Allowance/charge percentage MUST be provided when allowance/charge base amount is provided."
	mP043 = "Allowance/charge ChargeIndicator value MUST equal 'true' or 'false'"
	mP044 = "Charge on price level is NOT allowed. Only value 'false' allowed."
	mP046 = "Item net price MUST equal (Gross price - Allowance amount) when gross price is provided."
	mP055 = "Invoice total VAT amount and Invoice total VAT amount in accounting currency MUST have the same operational sign"
	mP061 = "Mandate reference MUST be provided for direct debit."
	mP110 = "Start date of line period MUST be within invoice period."
	mP111 = "End date of line period MUST be within invoice period."
	mP120 = "Invoice line net amount MUST equal (Invoiced quantity * (Item net price/item price base quantity) + Sum of invoice line charge amount - sum of invoice line allowance amount"
	mP121 = "Base quantity MUST be a positive number above zero."
	mP130 = "Unit code of price base quantity MUST be same as invoiced quantity."
)

// pfail records the failure of a Peppol assertion: the message of the
// schematron, then the values at fault (format, args).
func (k *checker) pfail(rule string, sev rules.Severity, n *xmltree.Node, msg, format string, args ...any) {
	id := rule
	if rule == rP0431 || rule == rP0432 {
		id = rP043 // the message of both assertions
	}
	text := "[" + id + "]-" + strings.ReplaceAll(msg, "%", "%%")
	if format != "" {
		text += "; " + format
	}
	k.fail(rule, sev, n, text, args...)
}

// slackFor returns $slackValue: 0.5 when a value of the invoice currency
// code is 'HUF', 0.02 otherwise.
func slackFor(currencies []*xmltree.Node) *big.Rat {
	for _, c := range currencies {
		if sv(c) == "HUF" {
			return big.NewRat(1, 2)
		}
	}
	return big.NewRat(2, 100)
}

// --- CII ---

// peppolCII runs the three Peppol patterns of the CII schematron.
func (k *checker) peppolCII() {
	settlement := pTransaction + "/ApplicableHeaderTradeSettlement/"
	p := &ciiPeppol{checker: k}
	for n := range all(k.root, settlement+"InvoiceCurrencyCode") {
		p.currency = append(p.currency, n)
	}
	for n := range all(k.root, settlement+"TaxCurrencyCode") {
		p.taxCurrency = append(p.taxCurrency, n)
	}
	p.slack = slackFor(p.currency)
	p.walk(k.root)
}

// ciiPeppol holds the parameters of the CII schematron.
type ciiPeppol struct {
	*checker
	currency    []*xmltree.Node // $documentCurrencyCode
	taxCurrency []*xmltree.Node // $taxCurrencyCode
	slack       *big.Rat        // $slackValue
}

func (k *ciiPeppol) walk(n *xmltree.Node) {
	k.element(n)
	k.empty(n)
	k.grossPrice(n)
	for _, c := range n.Children {
		k.walk(c)
	}
}

func isRam(n *xmltree.Node, name string) bool {
	return n != nil && n.Name == name && n.Space == rules.NSRam
}

// element checks n against the rules of peppol-cii-pattern-1.
func (k *ciiPeppol) element(n *xmltree.Node) {
	switch {
	case n.Name == "ExchangedDocumentContext" && n.Space == rules.NSRsm:
		if !exists(n, "BusinessProcessSpecifiedDocumentContextParameter/ID") {
			k.pfail(rP001, rules.Error, n, mP001, "")
		}
	case isRam(n, "ApplicableHeaderTradeSettlement"):
		k.settlement(n)
	case isRam(n, "BuyerTradeParty"):
		if !exists(n, "URIUniversalCommunication/URIID") {
			k.pfail(rP010, rules.Error, n, mP010, "")
		}
	case isRam(n, "SellerTradeParty"):
		if !exists(n, "URIUniversalCommunication/URIID") {
			k.pfail(rP020, rules.Error, n, mP020, "")
		}
	case isRam(n, "SpecifiedTradeAllowanceCharge"):
		k.allowanceCharge(n)
	case isRam(n, "AppliedTradeAllowanceCharge"):
		k.indicator(n, rP0432)
	case isRam(n, "SpecifiedTradeSettlementPaymentMeans") && in(nsv(first(n, "TypeCode")), "49", "59"):
		if !exists(n.Parent, "SpecifiedTradePaymentTerms/DirectDebitMandateID") {
			k.pfail(rP061, rules.Error, n, mP061, "")
		}
	case isRam(n, "StartDateTime"):
		k.linePeriod(n, rP110, mP110, func(line, invoice string) bool { return line >= invoice })
	case isRam(n, "EndDateTime"):
		k.linePeriod(n, rP111, mP111, func(line, invoice string) bool { return line <= invoice })
	case isRam(n, "IncludedSupplyChainTradeLineItem"):
		k.lineAmount(n)
		if exists(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument") &&
			!anyEq(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument/TypeCode", "130") {
			k.pfail(rP101, rules.Error, n, "Element Additional referenced document can only be used for Invoice line object.", "type code %q",
				sv(first(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument/TypeCode")))
		}
	case isRam(n, "NetPriceProductTradePrice") || isRam(n, "GrossPriceProductTradePrice"):
		if q := first(n, "BasisQuantity"); q != nil {
			if v, ok := xsRat(q); !ok || v.Sign() <= 0 {
				k.pfail(rP121, rules.Error, q, mP121, "value: %q", sv(q))
			}
		}
	case hasUnit(n) && isPrice(n.Parent):
		var line *xmltree.Node
		if n.Parent.Parent != nil {
			line = n.Parent.Parent.Parent
		}
		u := n.Attr("unitCode")
		if !anyAttrEq(line, "SpecifiedLineTradeDelivery/BilledQuantity", "unitCode", u) {
			k.pfail(rP130, rules.Error, n, mP130, "unit code %q, invoiced quantity in %q", u, attr(first(line, "SpecifiedLineTradeDelivery/BilledQuantity"), "unitCode"))
		}
	}
}

// isPrice reports whether n is a ram:NetPriceProductTradePrice or a
// ram:GrossPriceProductTradePrice.
func isPrice(n *xmltree.Node) bool {
	return isRam(n, "NetPriceProductTradePrice") || isRam(n, "GrossPriceProductTradePrice")
}

// hasUnit reports whether n is a ram:BasisQuantity with a unitCode attribute.
func hasUnit(n *xmltree.Node) bool {
	return isRam(n, "BasisQuantity") && hasUnitAttr(n)
}

// settlement checks the rule of ram:ApplicableHeaderTradeSettlement: VAT
// accounting currency (R005) and VAT totals (R053 to R055).
func (k *ciiPeppol) settlement(n *xmltree.Node) {
	if tc := first(n, "TaxCurrencyCode"); tc != nil {
		var ic string
		if c := first(n, "InvoiceCurrencyCode"); c != nil {
			ic = c.Text
		}
		if norm(tc.Text) == norm(ic) {
			k.pfail(rP005, rules.Error, tc, mP005, "value: %q", norm(tc.Text))
		}
	}
	var inDoc, other int
	var docTotals, taxTotals []*xmltree.Node
	for t := range all(n, "SpecifiedTradeSettlementHeaderMonetarySummation/TaxTotalAmount") {
		if currencyIn(t, k.currency) {
			inDoc++
			docTotals = append(docTotals, t)
		}
		if currencyIn(t, k.taxCurrency) {
			taxTotals = append(taxTotals, t)
		}
		if currencyNotIn(t, k.currency) {
			other++
		}
	}
	if inDoc > 1 {
		k.pfail(rP053, rules.Error, n, "No more than one tax total amount must be provided where currency id equals document currency code.", "%d tax totals in the invoice currency", inDoc)
	}
	want := 0
	if exists(n, "TaxCurrencyCode") {
		want = 1
	}
	if other != want {
		k.pfail(rP054, rules.Error, n, "Only one tax total amount must be provided where currency id equals tax currency code, if tax currency code (BT-6) is provided.", "%d tax totals in another currency", other)
	}
	if len(k.taxCurrency) == 0 || inDoc == 0 {
		return
	}
	neg, ok1 := anyDouble(taxTotals, func(f float64) bool { return f < 0 })
	negDoc, ok2 := anyDouble(docTotals, func(f float64) bool { return f < 0 })
	pos, ok3 := anyDouble(taxTotals, func(f float64) bool { return f >= 0 })
	posDoc, ok4 := anyDouble(docTotals, func(f float64) bool { return f >= 0 })
	if !(ok1 && ok2 && ok3 && ok4) || !(neg && negDoc || pos && posDoc) {
		k.pfail(rP055, rules.Error, n, mP055, "")
	}
}

// currencyIn reports whether @currencyID of n equals the value of one of set
// (@currencyID = $set).
func currencyIn(n *xmltree.Node, set []*xmltree.Node) bool {
	c, ok := n.LookupAttr("currencyID")
	if !ok {
		return false
	}
	for _, s := range set {
		if sv(s) == c {
			return true
		}
	}
	return false
}

// currencyNotIn reports whether @currencyID of n differs from the value of
// one of set (@currencyID != $set).
func currencyNotIn(n *xmltree.Node, set []*xmltree.Node) bool {
	c, ok := n.LookupAttr("currencyID")
	if !ok {
		return false
	}
	for _, s := range set {
		if sv(s) != c {
			return true
		}
	}
	return false
}

// allowanceCharge checks a ram:SpecifiedTradeAllowanceCharge, of the
// document or of a line: percentage and base amount go together (R041,
// R042); otherwise the amount is base amount × percentage / 100 when both
// are given (R040), and the indicator is true or false (R043-1).
func (k *ciiPeppol) allowanceCharge(n *xmltree.Node) {
	pct, basis := first(n, "CalculationPercent"), first(n, "BasisAmount")
	switch {
	case pct != nil && basis == nil:
		k.pfail(rP041, rules.Error, n, mP041, "")
		return
	case pct == nil && basis != nil:
		k.pfail(rP042, rules.Error, n, mP042, "")
		return
	}
	if pct != nil {
		k.percentAmount(n, first(n, "ActualAmount"), basis, pct, k.slack)
	}
	k.indicator(n, rP0431)
}

// percentAmount checks R040: amount (0 when absent) = base × percentage /
// 100, within slack. A value that is not a decimal fails it.
func (k *checker) percentAmount(n, amount, basis, pct *xmltree.Node, slack *big.Rat) {
	a, ok1 := big.NewRat(0, 1), true
	if amount != nil {
		a, ok1 = xsRat(amount)
	}
	b, ok2 := xsRat(basis)
	p, ok3 := xsRat(pct)
	if ok1 && ok2 && ok3 {
		want := new(big.Rat).Mul(b, p)
		want.Quo(want, big.NewRat(100, 1))
		if within(a, want, slack) {
			return
		}
	}
	k.pfail(rP040, rules.Error, n, mP040, "amount %q, base amount %q, percentage %q", sv(amount), sv(basis), sv(pct))
}

// indicator checks R043-1 or R043-2: normalize-space of the text of the
// charge indicator is 'true' or 'false'.
func (k *ciiPeppol) indicator(n *xmltree.Node, rule string) {
	var v string
	if ind := first(n, "ChargeIndicator/Indicator"); ind != nil {
		v = norm(ind.Text)
	}
	if v != "true" && v != "false" {
		k.pfail(rule, rules.Error, n, mP043, "value: %q", v)
	}
}

// linePeriod checks R110 or R111 on the ram:StartDateTime or
// ram:EndDateTime n of a line period, when the invoice has a period with the
// same bound: the udt:DateTimeString values are compared as strings, and
// one pair that satisfies ok is enough.
func (k *ciiPeppol) linePeriod(n *xmltree.Node, rule, msg string, ok func(line, invoice string) bool) {
	period := n.Parent
	if !isRam(period, "BillingSpecifiedPeriod") || !isRam(period.Parent, "SpecifiedLineTradeSettlement") {
		return
	}
	line := period.Parent.Parent
	if !isRam(line, "IncludedSupplyChainTradeLineItem") || line.Parent == nil ||
		line.Parent.Name != "SupplyChainTradeTransaction" || line.Parent.Space != rules.NSRsm {
		return
	}
	var invoice []string
	found := false
	for b := range all(line.Parent, "ApplicableHeaderTradeSettlement/BillingSpecifiedPeriod/"+n.Name) {
		found = true
		invoice = append(invoice, dateStrings(b)...)
	}
	if !found {
		return
	}
	lines := dateStrings(n)
	for _, l := range lines {
		for _, i := range invoice {
			if ok(l, i) {
				return
			}
		}
	}
	k.pfail(rule, rules.Error, n, msg, "line date %q, invoice date %q", firstOf(lines), firstOf(invoice))
}

// dateStrings returns the values of the udt:DateTimeString children of n.
func dateStrings(n *xmltree.Node) []string {
	var out []string
	for _, c := range n.Children {
		if c.Name == "DateTimeString" && c.Space == rules.NSUdt {
			out = append(out, sv(c))
		}
	}
	return out
}

func firstOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// lineAmount checks R120 on a line: line net amount = quantity × (net
// price / base quantity) + charges - allowances, within $slackValue. The base
// quantity is the one of the net price, else of the gross price, else 1, a
// zero not counting; the sums of the allowances and of the charges are
// rounded to 2 decimals. A value that is not a decimal fails it.
func (k *ciiPeppol) lineAmount(n *xmltree.Node) {
	value := func(path string, def int64) (*big.Rat, bool) {
		e := first(n, path)
		if e == nil {
			return big.NewRat(def, 1), true
		}
		return xsRat(e)
	}
	total, ok1 := value("SpecifiedLineTradeSettlement/SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount", 0)
	qty, ok2 := value("SpecifiedLineTradeDelivery/BilledQuantity", 1)
	price, ok3 := value("SpecifiedLineTradeAgreement/NetPriceProductTradePrice/ChargeAmount", 0)
	base, ok4 := big.NewRat(1, 1), true
	for _, path := range []string{"SpecifiedLineTradeAgreement/NetPriceProductTradePrice/BasisQuantity", "SpecifiedLineTradeAgreement/GrossPriceProductTradePrice/BasisQuantity"} {
		if e := first(n, path); e != nil {
			v, ok := xsRat(e)
			if !ok {
				ok4 = false
				break
			}
			if v.Sign() != 0 {
				base = v
				break
			}
		}
	}
	allowances, ok5 := k.lineAllowances(n, "false")
	charges, ok6 := k.lineAllowances(n, "true")
	if ok1 && ok2 && ok3 && ok4 && ok5 && ok6 {
		want := new(big.Rat).Quo(price, base)
		want.Mul(want, qty)
		want.Add(want, charges)
		want.Sub(want, allowances)
		if within(total, want, k.slack) {
			return
		}
		k.pfail(rP120, rules.Warning, n, mP120, "line net amount %s, expected %s", total.FloatString(2), want.FloatString(4))
		return
	}
	k.pfail(rP120, rules.Warning, n, mP120, "an amount or a quantity is not a decimal")
}

// lineAllowances returns the sum of the amounts of the allowances
// (indicator "false") or charges ("true") of a CII line, rounded to two
// decimals, halves towards positive infinity.
func (k *ciiPeppol) lineAllowances(n *xmltree.Node, indicator string) (*big.Rat, bool) {
	sum := new(big.Rat)
	for ac := range all(n, "SpecifiedLineTradeSettlement/SpecifiedTradeAllowanceCharge") {
		if nsv(first(ac, "ChargeIndicator/Indicator")) != indicator {
			continue
		}
		for a := range all(ac, "ActualAmount") {
			v, ok := xsRat(a)
			if !ok {
				return nil, false
			}
			sum.Add(sum, v)
		}
	}
	return roundHalfCeil2(sum), true
}

// empty checks R008 (peppol-cii-pattern-2), whose context is
// //*[not(name() = 'ram:ApplicableHeaderTradeDelivery') and not(*) and
// not(normalize-space())]: an element without child element whose text is
// empty or white space, but an empty delivery written with the prefix ram.
func (k *ciiPeppol) empty(n *xmltree.Node) {
	if len(n.Children) == 0 && norm(n.Text) == "" && qname(n) != "ram:ApplicableHeaderTradeDelivery" {
		k.pfail(rP008, rules.Error, n, mP008, "element %s", qname(n))
	}
}

// qname returns name(): the name of n with the prefix of the document.
func qname(n *xmltree.Node) string {
	if n.Prefix == "" {
		return n.Name
	}
	return n.Prefix + ":" + n.Name
}

// grossPrice checks the rule of peppol-cii-pattern-3, whose context is
// rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/
// ram:SpecifiedLineTradeAgreement/ram:GrossPriceProductTradePrice: an
// allowance on the price is not a charge (R044), and the net price is the
// gross price minus the first amount of the allowance (R046).
func (k *ciiPeppol) grossPrice(n *xmltree.Node) {
	agreement := n.Parent
	if !isRam(n, "GrossPriceProductTradePrice") || !isRam(agreement, "SpecifiedLineTradeAgreement") ||
		!isRam(agreement.Parent, "IncludedSupplyChainTradeLineItem") {
		return
	}
	if t := agreement.Parent.Parent; t == nil || t.Name != "SupplyChainTradeTransaction" || t.Space != rules.NSRsm {
		return
	}
	if exists(n, "AppliedTradeAllowanceCharge/ActualAmount") && !anyEq(n, "AppliedTradeAllowanceCharge/ChargeIndicator/Indicator", "false") {
		k.pfail(rP044, rules.Error, n, mP044, "value: %q", sv(first(n, "AppliedTradeAllowanceCharge/ChargeIndicator/Indicator")))
	}
	gross := first(n, "ChargeAmount")
	if gross == nil {
		return
	}
	net := first(agreement, "NetPriceProductTradePrice/ChargeAmount")
	// u:decimalOrZero(ram:AppliedTradeAllowanceCharge/ram:ActualAmount[1]):
	// the first amount of each allowance, the first of them when there are
	// several.
	var allowance *xmltree.Node
	for ac := range all(n, "AppliedTradeAllowanceCharge") {
		if allowance = first(ac, "ActualAmount"); allowance != nil {
			break
		}
	}
	g, ok1 := xsRat(gross)
	a, ok2 := big.NewRat(0, 1), true
	if allowance != nil {
		a, ok2 = xsRat(allowance)
	}
	if net != nil && ok1 && ok2 {
		if v, ok := xsRat(net); ok && v.Cmp(new(big.Rat).Sub(g, a)) == 0 {
			return
		}
	}
	k.pfail(rP046, rules.Error, n, mP046, "net price %q, gross price %q, allowance %q", sv(net), sv(gross), sv(allowance))
}

// --- UBL ---

// ublPeppol runs the two Peppol patterns of the UBL schematron.
func (k *ublChecker) ublPeppol() {
	k.slack = slackFor(k.root.Elems("DocumentCurrencyCode"))
	k.ublPeppolWalk(k.root)
}

func (k *ublChecker) ublPeppolWalk(n *xmltree.Node) {
	// peppol-ubl-pattern-1: R008, an element without child element whose
	// text is empty or white space, but cbc:ID of cac:OrderReference.
	if len(n.Children) == 0 && norm(n.Text) == "" && !(isCbc(n, "ID") && isCac(n.Parent, "OrderReference")) {
		k.pfail(rP008, rules.Error, n, mP008, "element %s", qname(n))
	}
	k.ublPeppolElement(n)
	for _, c := range n.Children {
		k.ublPeppolWalk(c)
	}
}

// ublPeppolElement checks n against the rules of peppol-ubl-pattern-2.
func (k *ublChecker) ublPeppolElement(n *xmltree.Node) {
	switch {
	case n == k.root:
		k.ublPeppolDocument()
	case isCbc(n, "TaxCurrencyCode"):
		var dc string
		if c := first(n.Parent, "DocumentCurrencyCode"); c != nil {
			dc = c.Text
		}
		if norm(n.Text) == norm(dc) {
			k.pfail(rP005, rules.Error, n, mP005, "value: %q", norm(n.Text))
		}
	case isCac(n, "Party") && isCac(n.Parent, "AccountingCustomerParty"):
		if !exists(n, "EndpointID") {
			k.pfail(rP010, rules.Error, n, mP010, "")
		}
	case isCac(n, "Party") && isCac(n.Parent, "AccountingSupplierParty"):
		if !exists(n, "EndpointID") {
			k.pfail(rP020, rules.Error, n, mP020, "")
		}
	case isCac(n, "AllowanceCharge") && (n.Parent == k.root || isCac(n.Parent, k.line) && n.Parent.Parent == k.root):
		k.ublAllowanceCharge(n)
	case isCac(n, "PaymentMeans") && in(nsv(first(n, "PaymentMeansCode")), "49", "59"):
		if !exists(n, "PaymentMandate/ID") {
			k.pfail(rP061, rules.Error, n, mP061, "")
		}
	case k.ublLinePeriod(n, "StartDate"):
		k.ublLineDate(n, rP110, mP110, func(line, invoice int64) bool { return line >= invoice })
	case k.ublLinePeriod(n, "EndDate"):
		k.ublLineDate(n, rP111, mP111, func(line, invoice int64) bool { return line <= invoice })
	case isCac(n, "InvoiceLine") || isCac(n, "CreditNoteLine"):
		k.ublLine(n)
	case isCac(n, "AllowanceCharge") && isCac(n.Parent, "Price"):
		k.ublPriceAllowance(n)
	case isCbc(n, "BaseQuantity") && isCac(n.Parent, "Price") && hasUnitAttr(n):
		line := n.Parent.Parent
		if !exists(line, "InvoicedQuantity") && !exists(line, "CreditedQuantity") {
			return
		}
		qty := k.quantityName()
		u := n.Attr("unitCode")
		if !anyAttrEq(line, qty, "unitCode", u) {
			k.pfail(rP130, rules.Error, n, mP130, "unit code %q, quantity in %q", u, attr(first(line, qty), "unitCode"))
		}
	}
}

func hasUnitAttr(n *xmltree.Node) bool {
	_, ok := n.LookupAttr("unitCode")
	return ok
}

// quantityName returns the name of the quantity of the lines: the invoiced
// quantity in an invoice, the credited quantity in a credit note.
func (k *ublChecker) quantityName() string {
	if k.root.Name == "Invoice" {
		return "InvoicedQuantity"
	}
	return "CreditedQuantity"
}

// ublPeppolDocument checks the rule of the invoice or credit note: business
// process (R001) and VAT totals (R053 to R055).
func (k *ublChecker) ublPeppolDocument() {
	n := k.root
	if !exists(n, "ProfileID") {
		k.pfail(rP001, rules.Error, n, mP001, "")
	}
	with, without := 0, 0
	for t := range all(n, "TaxTotal") {
		if exists(t, "TaxSubtotal") {
			with++
		} else {
			without++
		}
	}
	if with != 1 {
		k.pfail(rP053, rules.Error, n, "Only one tax total with tax subtotals MUST be provided.", "%d tax totals with tax subtotals", with)
	}
	tc := first(n, "TaxCurrencyCode")
	want := 0
	if tc != nil {
		want = 1
	}
	if without != want {
		k.pfail(rP054, rules.Error, n, "Only one tax total without tax subtotals MUST be provided when tax currency code is provided.", "%d tax totals without tax subtotals", without)
	}
	if tc == nil {
		return
	}
	// The amounts whose currency is the tax currency, and those whose
	// currency is the invoice currency, have the same sign (zero counting
	// as both).
	tcode, dcode := nsv(tc), nsv(first(n, "DocumentCurrencyCode"))
	var inTax, inDoc []*xmltree.Node
	for a := range all(n, "TaxTotal/TaxAmount") {
		c, ok := a.LookupAttr("currencyID")
		if !ok {
			continue
		}
		if c == tcode {
			inTax = append(inTax, a)
		}
		if c == dcode {
			inDoc = append(inDoc, a)
		}
	}
	neg, ok1 := anyDouble(inTax, func(f float64) bool { return f <= 0 })
	negDoc, ok2 := anyDouble(inDoc, func(f float64) bool { return f <= 0 })
	pos, ok3 := anyDouble(inTax, func(f float64) bool { return f >= 0 })
	posDoc, ok4 := anyDouble(inDoc, func(f float64) bool { return f >= 0 })
	if !(ok1 && ok2 && ok3 && ok4) || !(neg && negDoc || pos && posDoc) {
		k.pfail(rP055, rules.Error, n, mP055, "")
	}
}

// ublAllowanceCharge checks an allowance or charge of the document or of a
// line: percentage and base amount go together (R041, R042); otherwise the
// amount is base amount × percentage / 100 when both are given (R040), and
// the indicator is true or false (R043).
func (k *ublChecker) ublAllowanceCharge(n *xmltree.Node) {
	pct, basis := first(n, "MultiplierFactorNumeric"), first(n, "BaseAmount")
	switch {
	case pct != nil && basis == nil:
		k.pfail(rP041, rules.Error, n, mP041, "")
		return
	case pct == nil && basis != nil:
		k.pfail(rP042, rules.Error, n, mP042, "")
		return
	}
	if pct != nil {
		k.percentAmount(n, first(n, "Amount"), basis, pct, k.slack)
	}
	var v string
	if ind := first(n, "ChargeIndicator"); ind != nil {
		v = norm(ind.Text)
	}
	if v != "true" && v != "false" {
		k.pfail(rP043, rules.Error, n, mP043, "value: %q", v)
	}
}

// ublLinePeriod reports whether n is the bound name (cbc:StartDate or
// cbc:EndDate) of the period of a line of the document, in a document that
// has an invoice period with that bound.
func (k *ublChecker) ublLinePeriod(n *xmltree.Node, name string) bool {
	if !isCbc(n, name) || !isCac(n.Parent, "InvoicePeriod") {
		return false
	}
	line := n.Parent.Parent
	return isCac(line, k.line) && line.Parent == k.root && exists(k.root, "InvoicePeriod/"+name)
}

// ublLineDate checks R110 or R111 on the bound n of a line period: the
// dates are compared as xs:date. A date that is not valid, or a line bound
// without text, fails it.
func (k *ublChecker) ublLineDate(n *xmltree.Node, rule, msg string, ok func(line, invoice int64) bool) {
	bound := first(k.root, "InvoicePeriod/"+n.Name)
	line, ok1 := xsDate(n.Text)
	invoice, ok2 := xsDate(sv(bound))
	if ok1 && ok2 && ok(line, invoice) {
		return
	}
	k.pfail(rule, rules.Error, n, msg, "line date %q, invoice date %q", n.Text, sv(bound))
}

// ublLine checks a line: net amount (R120), base quantity (R121) and line
// object reference (R101).
func (k *ublChecker) ublLine(n *xmltree.Node) {
	k.ublLineAmount(n)
	if q := first(n, "Price/BaseQuantity"); q != nil {
		if v, ok := xsRat(q); !ok || v.Sign() <= 0 {
			k.pfail(rP121, rules.Error, q, mP121, "value: %q", sv(q))
		}
	}
	if exists(n, "DocumentReference") && !anyEq(n, "DocumentReference/DocumentTypeCode", "130") {
		k.pfail(rP101, rules.Error, n, "Element Document reference can only be used for Invoice line object", "type code %q", sv(first(n, "DocumentReference/DocumentTypeCode")))
	}
}

// ublLineAmount checks R120: line net amount = quantity × (price / base
// quantity) + charges - allowances, within $slackValue. The quantity is the
// invoiced quantity in an invoice, the credited quantity in a credit note; a
// zero base quantity counts as 1. A value that is not a decimal fails it.
func (k *ublChecker) ublLineAmount(n *xmltree.Node) {
	value := func(path string, def int64) (*big.Rat, bool) {
		e := first(n, path)
		if e == nil {
			return big.NewRat(def, 1), true
		}
		return xsRat(e)
	}
	total, ok1 := value("LineExtensionAmount", 0)
	qty, ok2 := value(k.quantityName(), 1)
	price, ok3 := value("Price/PriceAmount", 0)
	base, ok4 := value("Price/BaseQuantity", 1)
	if ok4 && base.Sign() == 0 {
		base = big.NewRat(1, 1)
	}
	allowances, ok5 := ublLineAllowances(n, "false")
	charges, ok6 := ublLineAllowances(n, "true")
	if ok1 && ok2 && ok3 && ok4 && ok5 && ok6 {
		want := new(big.Rat).Quo(price, base)
		want.Mul(want, qty)
		want.Add(want, charges)
		want.Sub(want, allowances)
		if within(total, want, k.slack) {
			return
		}
		k.pfail(rP120, rules.Warning, n, mP120, "line net amount %s, expected %s", total.FloatString(2), want.FloatString(4))
		return
	}
	k.pfail(rP120, rules.Warning, n, mP120, "an amount or a quantity is not a decimal")
}

// ublLineAllowances returns the sum of the amounts of the allowances
// (indicator "false") or charges ("true") of a UBL line, rounded to two
// decimals, halves towards positive infinity.
func ublLineAllowances(n *xmltree.Node, indicator string) (*big.Rat, bool) {
	sum := new(big.Rat)
	for ac := range all(n, "AllowanceCharge") {
		if nsv(first(ac, "ChargeIndicator")) != indicator {
			continue
		}
		for a := range all(ac, "Amount") {
			v, ok := xsRat(a)
			if !ok {
				return nil, false
			}
			sum.Add(sum, v)
		}
	}
	return roundHalfCeil2(sum), true
}

// ublPriceAllowance checks an allowance on the price of a line: it is not a
// charge (R044), and with a base amount, the net price is the base amount
// minus the allowance amount (R046). A missing net price or allowance
// amount fails R046 (xs:decimal of an empty sequence is empty, and the
// comparison false).
func (k *ublChecker) ublPriceAllowance(n *xmltree.Node) {
	if v := nsv(first(n, "ChargeIndicator")); v != "false" {
		k.pfail(rP044, rules.Error, n, mP044, "value: %q", v)
	}
	base := first(n, "BaseAmount")
	if base == nil {
		return
	}
	price, amount := first(n.Parent, "PriceAmount"), first(n, "Amount")
	p, ok1 := xsRat(price)
	b, ok2 := xsRat(base)
	a, ok3 := xsRat(amount)
	if ok1 && ok2 && ok3 && p.Cmp(new(big.Rat).Sub(b, a)) == 0 {
		return
	}
	k.pfail(rP046, rules.Error, n, mP046, "net price %q, gross price %q, allowance %q", sv(price), sv(base), sv(amount))
}

// --- Values ---

// xsRat implements xs:decimal(n) as an exact rational, without limit of
// precision, as Saxon does: the official XRechnung test suite has amounts
// with 18 decimals. It fails when n is nil or its value is not in the
// lexical space of xs:decimal (an optional sign, then digits with an
// optional decimal point, one digit at least, around white space).
func xsRat(n *xmltree.Node) (*big.Rat, bool) {
	if n == nil {
		return nil, false
	}
	s := strings.Trim(sv(n), " \t\n\r")
	m := s
	if m != "" && (m[0] == '+' || m[0] == '-') {
		m = m[1:]
	}
	ip, fp, _ := strings.Cut(m, ".")
	if ip+fp == "" || !allDigits(ip) || !allDigits(fp) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}

// within implements u:slack(a, b, slack): |a - b| <= slack.
func within(a, b, slack *big.Rat) bool {
	d := new(big.Rat).Sub(a, b)
	return d.Abs(d).Cmp(slack) <= 0
}

// roundHalfCeil2 implements round(v * 10 * 10) div 100: v rounded to two
// decimals, halves towards positive infinity.
func roundHalfCeil2(v *big.Rat) *big.Rat {
	x := new(big.Rat).Mul(v, big.NewRat(100, 1))
	x.Add(x, big.NewRat(1, 2))
	q := new(big.Int).Div(x.Num(), x.Denom()) // Euclidean division: the floor, for a positive denominator
	return new(big.Rat).SetFrac(q, big.NewInt(100))
}

// anyDouble reports whether the value of one of nodes, cast to xs:double,
// satisfies f (the general comparison nodes < 0). ok is false when a value
// is not a double: the reference stops with an error.
func anyDouble(nodes []*xmltree.Node, f func(float64) bool) (found, ok bool) {
	for _, n := range nodes {
		v, valid := xsDouble(sv(n))
		if !valid {
			return false, false
		}
		if !math.IsNaN(v) && f(v) {
			found = true
		}
	}
	return found, true
}

// xsDouble implements the cast of an untyped value to xs:double.
func xsDouble(s string) (float64, bool) {
	s = strings.Trim(s, " \t\n\r")
	switch s {
	case "INF", "+INF":
		return math.Inf(1), true
	case "-INF":
		return math.Inf(-1), true
	case "NaN":
		return math.NaN(), true
	}
	m, exp, hasExp := strings.Cut(strings.ReplaceAll(s, "E", "e"), "e")
	if m != "" && (m[0] == '+' || m[0] == '-') {
		m = m[1:]
	}
	ip, fp, _ := strings.Cut(m, ".")
	if ip == "" && fp == "" || !allDigits(ip) || !allDigits(fp) {
		return 0, false
	}
	if hasExp {
		if exp != "" && (exp[0] == '+' || exp[0] == '-') {
			exp = exp[1:]
		}
		if exp == "" || !allDigits(exp) {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	return f, true
}

// allDigits reports whether s holds ASCII digits only ("" included).
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// xsDate implements the cast to xs:date: YYYY-MM-DD with an optional time
// zone, as minutes since the epoch at the start of the day, in UTC.
func xsDate(s string) (int64, bool) {
	s = strings.Trim(s, " \t\n\r")
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	i := strings.IndexByte(s, '-')
	if i < 4 || i > 4 && s[0] == '0' || !allDigits(s[:i]) || len(s) < i+6 || s[i+3] != '-' {
		return 0, false
	}
	year, err := strconv.ParseInt(s[:i], 10, 32)
	if err != nil {
		return 0, false
	}
	if neg {
		year = -year
	}
	mm, dd, tz := s[i+1:i+3], s[i+4:i+6], s[i+6:]
	if !allDigits(mm) || !allDigits(dd) {
		return 0, false
	}
	month, _ := strconv.Atoi(mm)
	day, _ := strconv.Atoi(dd)
	offset := 0
	switch {
	case tz == "" || tz == "Z":
	case len(tz) == 6 && (tz[0] == '+' || tz[0] == '-') && tz[3] == ':' && allDigits(tz[1:3]) && allDigits(tz[4:]):
		h, _ := strconv.Atoi(tz[1:3])
		m, _ := strconv.Atoi(tz[4:])
		if m > 59 || h > 14 || h == 14 && m > 0 {
			return 0, false
		}
		offset = h*60 + m
		if tz[0] == '-' {
			offset = -offset
		}
	default:
		return 0, false
	}
	if month < 1 || month > 12 || day < 1 {
		return 0, false
	}
	t := time.Date(int(year), time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day {
		return 0, false // past the end of the month
	}
	return t.Unix()/60 - int64(offset), true
}
