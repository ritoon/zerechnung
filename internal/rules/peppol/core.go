package peppol

import (
	"math"
	"math/big"
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rules of the main pattern of the schematron: PEPPOL-EN16931-R* and
// PEPPOL-COMMON-R*.

// element checks n against the rules of the main pattern. Each element is
// checked by the first rule whose context it matches.
func (k *checker) element(n *xmltree.Node) {
	switch n.Name {
	case "ExchangedDocumentContext":
		if n.Space == rules.NSRsm {
			k.documentContext(n)
		}
	case "ApplicableHeaderTradeAgreement":
		k.agreement(n)
	case "ApplicableHeaderTradeSettlement":
		k.settlement(n)
	case "ExchangedDocument":
		if n.Space == rules.NSRsm {
			k.check(count(n, "IncludedNote") <= 1 && !exists(n, "IncludedNote/SubjectCode"), "PEPPOL-EN16931-R002", n,
				"%d notes", count(n, "IncludedNote"))
		}
	case "BuyerTradeParty":
		k.check(exists(n, "URIUniversalCommunication/URIID"), "PEPPOL-EN16931-R010", n, "")
	case "SellerTradeParty":
		k.check(exists(n, "URIUniversalCommunication/URIID"), "PEPPOL-EN16931-R020", n, "")
	case "SpecifiedTradeAllowanceCharge":
		k.allowanceCharge(n)
	case "AppliedTradeAllowanceCharge":
		k.chargeIndicator(n)
	case "SpecifiedTradeSettlementPaymentMeans":
		if in(nsv(first(n, "TypeCode")), "49", "59") {
			k.check(exists(n.Parent, "SpecifiedTradePaymentTerms/DirectDebitMandateID"), "PEPPOL-EN16931-R061", n, "")
		}
	case "StartDateTime":
		k.linePeriod(n, "PEPPOL-EN16931-R110", func(line, invoice string) bool { return line >= invoice })
	case "EndDateTime":
		k.linePeriod(n, "PEPPOL-EN16931-R111", func(line, invoice string) bool { return line <= invoice })
	case "IncludedSupplyChainTradeLineItem":
		k.line(n)
	case "NetPriceProductTradePrice", "GrossPriceProductTradePrice":
		if q := first(n, "BasisQuantity"); q != nil {
			if v, ok := dec(q); ok {
				k.check(v.Sign() > 0, "PEPPOL-EN16931-R121", q, "value: %q", sv(q))
			}
		}
	case "BasisQuantity":
		if u, ok := n.LookupAttr("unitCode"); ok && (parentIs(n, "NetPriceProductTradePrice") || parentIs(n, "GrossPriceProductTradePrice")) {
			var line *xmltree.Node
			if p := n.Parent.Parent; p != nil {
				line = p.Parent
			}
			k.check(anyAttrIn(line, "SpecifiedLineTradeDelivery/BilledQuantity", "unitCode", u), "PEPPOL-EN16931-R130", n,
				"unit code %q, invoiced quantity in %q", u, attr(first(line, "SpecifiedLineTradeDelivery/BilledQuantity"), "unitCode"))
		}
	case "URIID", "ID", "GlobalID":
		k.identifier(n)
	case "SpecifiedTaxRegistration":
		k.vatRegistration(n)
	}
}

// vatRegistration checks a ram:SpecifiedTaxRegistration whose ram:ID has
// the scheme VA against the format of a Belgian, Dutch or Luxembourgish VAT
// identifier, by the first rule that applies, in the order of the
// schematron: PEPPOL-COMMON-R062 looks at the upper-cased ram:ID,
// PEPPOL-COMMON-R056-2 and R058 at the string value of the registration.
func (k *checker) vatRegistration(n *xmltree.Node) {
	if !anyAttrIn(n, "ID", "schemeID", "VA") {
		return
	}
	id := nsv(first(n, "ID"))
	v := nsv(n)
	switch {
	case strings.HasPrefix(strings.ToUpper(id), "BE"):
		k.check(belgianVAT(strings.ToUpper(id)), "PEPPOL-COMMON-R062", n, "value: %q", id)
	case strings.HasPrefix(v, "NL"):
		k.check(dutchVAT(v), "PEPPOL-COMMON-R056-2", n, "value: %q", v)
	case strings.HasPrefix(strings.ToUpper(v), "LU"):
		k.check(luxembourgVAT(strings.ToUpper(v)), "PEPPOL-COMMON-R058", n, "value: %q", v)
	}
}

// documentContext checks rsm:ExchangedDocumentContext: business process
// (BT-23) and specification identifier (BT-24).
func (k *checker) documentContext(n *xmltree.Node) {
	bp := first(n, "BusinessProcessSpecifiedDocumentContextParameter/ID")
	k.check(bp != nil, "PEPPOL-EN16931-R001", n, "")
	if bp == nil {
		bp = n
	}
	k.check(k.profile != "Unknown", "PEPPOL-EN16931-R007", bp, "value: %q", sv(k.absFirst(pBusinessProcess)))
	id := first(n, "GuidelineSpecifiedDocumentContextParameter/ID")
	spec := norm(text(id))
	at := id
	if at == nil {
		at = n
	}
	k.check(strings.HasPrefix(spec, SpecificationID) && !strings.Contains(spec, "::"), "PEPPOL-EN16931-R004", at, "value: %q", spec)
}

// agreement checks ram:ApplicableHeaderTradeAgreement.
func (k *checker) agreement(n *xmltree.Node) {
	k.check(exists(n, "BuyerReference") || exists(n, "BuyerOrderReferencedDocument/IssuerAssignedID"), "PEPPOL-EN16931-R003", n, "")
	k.check(countTyped(n, "AdditionalReferencedDocument", "130") <= 1, "PEPPOL-EN16931-R006", n, "%d invoiced objects", countTyped(n, "AdditionalReferencedDocument", "130"))
	k.check(countTyped(n, "AdditionalReferencedDocument", "50") <= 1, "PEPPOL-EN16931-R080", n, "%d project references", countTyped(n, "AdditionalReferencedDocument", "50"))
}

// countTyped returns count(path[ram:TypeCode = code]).
func countTyped(n *xmltree.Node, path, code string) int {
	c := 0
	for d := range all(n, path) {
		if anyEq(d, "TypeCode", code) {
			c++
		}
	}
	return c
}

// settlement checks ram:ApplicableHeaderTradeSettlement: VAT accounting
// currency and VAT totals.
func (k *checker) settlement(n *xmltree.Node) {
	if tc := first(n, "TaxCurrencyCode"); tc != nil {
		k.check(norm(tc.Text) != norm(text(first(n, "InvoiceCurrencyCode"))), "PEPPOL-EN16931-R005", tc, "value: %q", norm(tc.Text))
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
	k.check(inDoc == 1, "PEPPOL-EN16931-R053", n, "%d tax totals in the invoice currency", inDoc)
	want := 0
	if exists(n, "TaxCurrencyCode") {
		want = 1
	}
	k.check(other == want, "PEPPOL-EN16931-R054", n, "%d tax totals in another currency", other)
	if len(k.taxCurrency) > 0 {
		neg := anyNumber(taxTotals, func(f float64) bool { return f < 0 }) && anyNumber(docTotals, func(f float64) bool { return f < 0 })
		pos := anyNumber(taxTotals, func(f float64) bool { return f >= 0 }) && anyNumber(docTotals, func(f float64) bool { return f >= 0 })
		k.check(neg || pos, "PEPPOL-EN16931-R055", n, "")
	}
}

// currencyIn reports whether @currencyID of n equals one of the values of set
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

// currencyNotIn reports whether @currencyID of n differs from one of the
// values of set (@currencyID != $set).
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

// anyNumber reports whether the value of one of nodes, cast to xs:double,
// satisfies f (the general comparison nodes < 0, nodes >= 0).
func anyNumber(nodes []*xmltree.Node, f func(float64) bool) bool {
	for _, n := range nodes {
		if v, ok := double(sv(n)); ok && !math.IsNaN(v) && f(v) {
			return true
		}
	}
	return false
}

// allowanceCharge checks a ram:SpecifiedTradeAllowanceCharge, at document
// or line level: percentage and base amount go together (R041, R042); when
// both or none are given, the amount is base × percentage / 100 (R040) and
// the indicator is true or false (R043).
func (k *checker) allowanceCharge(n *xmltree.Node) {
	pct, basis := first(n, "CalculationPercent"), first(n, "BasisAmount")
	switch {
	case pct != nil && basis == nil:
		k.fail("PEPPOL-EN16931-R041", n, "")
		return
	case pct == nil && basis != nil:
		k.fail("PEPPOL-EN16931-R042", n, "")
		return
	}
	if pct != nil && basis != nil {
		b, ok1 := dec(basis)
		p, ok2 := dec(pct)
		amount, ok3 := new(big.Rat), true
		if a := first(n, "ActualAmount"); a != nil {
			amount, ok3 = dec(a)
		}
		if ok1 && ok2 && ok3 {
			want := new(big.Rat).Mul(b, p)
			want.Quo(want, big.NewRat(100, 1))
			k.check(within(amount, want, big.NewRat(2, 100)), "PEPPOL-EN16931-R040", n,
				"amount %s, base amount %s × percentage %s / 100 = %s", sv(first(n, "ActualAmount")), sv(basis), sv(pct), want.FloatString(4))
		}
	}
	k.chargeIndicator(n)
}

// chargeIndicator checks R043 on an allowance or charge.
func (k *checker) chargeIndicator(n *xmltree.Node) {
	v := norm(text(first(n, "ChargeIndicator/Indicator")))
	k.check(v == "true" || v == "false", "PEPPOL-EN16931-R043", n, "value: %q", v)
}

// linePeriod checks R110 or R111 on the ram:StartDateTime or
// ram:EndDateTime n of a line period, when the invoice has a period with the
// same bound: the dates are compared as strings.
func (k *checker) linePeriod(n *xmltree.Node, id string, ok func(line, invoice string) bool) {
	period := n.Parent
	if period == nil || period.Name != "BillingSpecifiedPeriod" || !parentIs(period, "SpecifiedLineTradeSettlement") {
		return
	}
	line := period.Parent.Parent
	if line == nil || line.Name != "IncludedSupplyChainTradeLineItem" || !parentIs(line, "SupplyChainTradeTransaction") ||
		line.Parent.Space != rules.NSRsm {
		return
	}
	bounds := line.Parent.Elems("ApplicableHeaderTradeSettlement")
	var invoice []string
	found := false
	for _, s := range bounds {
		for b := range all(s, "BillingSpecifiedPeriod/"+n.Name) {
			found = true
			invoice = append(invoice, dateStrings(b)...)
		}
	}
	if !found {
		return
	}
	for _, l := range dateStrings(n) {
		for _, i := range invoice {
			if ok(l, i) {
				return
			}
		}
	}
	k.fail(id, n, "line date %q, invoice date %q", firstOr(dateStrings(n)), firstOr(invoice))
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

func firstOr(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// line checks ram:IncludedSupplyChainTradeLineItem: net amount (R120) and
// line object references (R100, R101).
func (k *checker) line(n *xmltree.Node) {
	k.lineAmount(n)
	k.check(countTyped(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument", "130") <= 1, "PEPPOL-EN16931-R100", n, "")
	if exists(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument") {
		k.check(anyEq(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument/TypeCode", "130"), "PEPPOL-EN16931-R101", n,
			"type code %q", sv(first(n, "SpecifiedLineTradeSettlement/AdditionalReferencedDocument/TypeCode")))
	}
}

// lineAmount checks R120: line net amount = quantity × (net price / base
// quantity) + charges - allowances, with a tolerance of 0.02. The sums of
// the line allowances and charges are rounded to two decimals.
func (k *checker) lineAmount(n *xmltree.Node) {
	value := func(path string, def int64) (*big.Rat, bool) {
		e := first(n, path)
		if e == nil {
			return big.NewRat(def, 1), true
		}
		return dec(e)
	}
	total, ok1 := value("SpecifiedLineTradeSettlement/SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount", 0)
	qty, ok2 := value("SpecifiedLineTradeDelivery/BilledQuantity", 1)
	price, ok3 := value("SpecifiedLineTradeAgreement/NetPriceProductTradePrice/ChargeAmount", 0)
	base, ok4 := value("SpecifiedLineTradeAgreement/NetPriceProductTradePrice/BasisQuantity", 1)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	if base.Sign() == 0 {
		base = big.NewRat(1, 1)
	}
	allowances, ok5 := k.lineAllowances(n, "false")
	charges, ok6 := k.lineAllowances(n, "true")
	if !ok5 || !ok6 {
		return
	}
	want := new(big.Rat).Quo(price, base)
	want.Mul(want, qty)
	want.Add(want, charges)
	want.Sub(want, allowances)
	k.check(within(total, want, big.NewRat(2, 100)), "PEPPOL-EN16931-R120", n,
		"line net amount %s, expected %s", total.FloatString(2), want.FloatString(4))
}

// lineAllowances returns the sum of the amounts of the line allowances
// (indicator "false") or charges ("true"), rounded to two decimals, halves
// towards positive infinity.
func (k *checker) lineAllowances(n *xmltree.Node, indicator string) (*big.Rat, bool) {
	sum := new(big.Rat)
	for ac := range all(n, "SpecifiedLineTradeSettlement/SpecifiedTradeAllowanceCharge") {
		if nsv(first(ac, "ChargeIndicator/Indicator")) != indicator {
			continue
		}
		for a := range all(ac, "ActualAmount") {
			v, ok := dec(a)
			if !ok {
				return nil, false
			}
			sum.Add(sum, v)
		}
	}
	return roundHalfCeil2(sum), true
}

// roundHalfCeil2 implements round(v * 100) div 100: v rounded to two
// decimals, halves towards positive infinity.
func roundHalfCeil2(v *big.Rat) *big.Rat {
	x := new(big.Rat).Mul(v, big.NewRat(100, 1))
	x.Add(x, big.NewRat(1, 2))
	// floor(x)
	q := new(big.Int).Div(x.Num(), x.Denom()) // Euclidean division: floor for a positive denominator
	return new(big.Rat).SetFrac(q, big.NewInt(100))
}
