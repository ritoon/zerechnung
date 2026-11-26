package peppol

import (
	"math/big"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rules of the first patterns of the UBL schematron: empty elements
// (PEPPOL-EN16931-R008), the credit note pattern (R080) and the main pattern
// (PEPPOL-EN16931-R*, PEPPOL-COMMON-R*).

// empty checks PEPPOL-EN16931-R008, whose context is
// //*[not(*) and not(normalize-space())]: an element without child element
// whose text is empty or white space.
func (k *ublChecker) empty(n *xmltree.Node) {
	if len(n.Children) == 0 && norm(n.Text) == "" {
		k.fail("PEPPOL-EN16931-R008", n, "")
	}
}

// creditNoteRules checks the pattern whose context is a credit note:
// PEPPOL-EN16931-R080.
func (k *ublChecker) creditNoteRules(n *xmltree.Node) {
	if isElem(n, nsCreditNote, "CreditNote") {
		c := countDocType(n, "50")
		k.check(c <= 1, "PEPPOL-EN16931-R080", n, "%d project references", c)
	}
}

// countDocType returns
// count(cac:AdditionalDocumentReference[cbc:DocumentTypeCode = code]).
func countDocType(n *xmltree.Node, code string) int {
	c := 0
	for _, d := range ulist(n, "cac:AdditionalDocumentReference") {
		if uanyEq(d, "cbc:DocumentTypeCode", code) {
			c++
		}
	}
	return c
}

// element checks n against the rules of the main pattern. Each element is
// checked by the first rule whose context it matches.
func (k *ublChecker) element(n *xmltree.Node) {
	switch {
	case isDoc(n):
		k.document(n)
	case uis(n, "cbc:TaxCurrencyCode"):
		v := norm(text(n))
		k.check(v != norm(text(ufirst(n.Parent, "cbc:DocumentCurrencyCode"))), "PEPPOL-EN16931-R005", n, "value: %q", v)
	case uis(n, "cac:Party") && uparent(n, "cac:AccountingCustomerParty"):
		k.check(uexists(n, "cbc:EndpointID"), "PEPPOL-EN16931-R010", n, "")
	case uis(n, "cac:Party") && uparent(n, "cac:AccountingSupplierParty"):
		k.check(uexists(n, "cbc:EndpointID"), "PEPPOL-EN16931-R020", n, "")
	case uis(n, "cac:AllowanceCharge") && documentOrLine(n.Parent):
		k.allowanceCharge(n)
	case uis(n, "cac:PaymentMeans") && in(nsv(ufirst(n, "cbc:PaymentMeansCode")), "49", "59"):
		k.check(uexists(n, "cac:PaymentMandate/cbc:ID"), "PEPPOL-EN16931-R061", n, "")
	case currencyChecked(n):
		c, _ := n.LookupAttr("currencyID")
		k.check(currencyIn(n, k.currency), "PEPPOL-EN16931-R051", n, "currency %q, invoice currency %q", c, firstValue(k.currency))
	case linePeriod(n, "cbc:StartDate"):
		k.linePeriodDate(n, "PEPPOL-EN16931-R110", func(line, invoice int64) bool { return line >= invoice })
	case linePeriod(n, "cbc:EndDate"):
		k.linePeriodDate(n, "PEPPOL-EN16931-R111", func(line, invoice int64) bool { return line <= invoice })
	case uis(n, "cac:InvoiceLine") || uis(n, "cac:CreditNoteLine"):
		k.line(n)
	case uis(n, "cac:AllowanceCharge") && uparent(n, "cac:Price"):
		k.priceAllowance(n)
	case uis(n, "cbc:BaseQuantity") && uparent(n, "cac:Price") && hasAttr(n, "unitCode"):
		k.baseQuantityUnit(n)
	default:
		k.identifier(n)
	}
}

// hasAttr reports whether n has the attribute name.
func hasAttr(n *xmltree.Node, name string) bool {
	_, ok := n.LookupAttr(name)
	return ok
}

// firstValue returns the string value of the first of nodes, "" if none.
func firstValue(nodes []*xmltree.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	return sv(nodes[0])
}

// orNode returns n, or def when n is nil.
func orNode(n, def *xmltree.Node) *xmltree.Node {
	if n == nil {
		return def
	}
	return n
}

// document checks the rule whose context is the invoice or credit note:
// business process and specification identifiers, notes, buyer reference,
// VAT totals.
func (k *ublChecker) document(n *xmltree.Node) {
	pid := ufirst(n, "cbc:ProfileID")
	k.check(pid != nil, "PEPPOL-EN16931-R001", n, "")
	k.check(k.profile != "Unknown", "PEPPOL-EN16931-R007", orNode(pid, n), "value: %q", norm(sv(ufirst(k.root, "cbc:ProfileID"))))
	notes := ucount(n, "cbc:Note")
	k.check(notes <= 1 || k.supplierDE && k.customerDE, "PEPPOL-EN16931-R002", n, "%d notes", notes)
	k.check(uexists(n, "cbc:BuyerReference") || uexists(n, "cac:OrderReference/cbc:ID"), "PEPPOL-EN16931-R003", n, "")
	cid := ufirst(n, "cbc:CustomizationID")
	spec := norm(text(cid))
	specID := SpecificationID
	if k.sb {
		specID = SelfBillingSpecificationID
	}
	k.check(strings.HasPrefix(spec, specID) && !strings.Contains(spec, "::"), "PEPPOL-EN16931-R004", orNode(cid, n), "value: %q", spec)

	with, without := 0, 0
	for _, t := range ulist(n, "cac:TaxTotal") {
		if uexists(t, "cac:TaxSubtotal") {
			with++
		} else {
			without++
		}
	}
	k.check(with == 1, "PEPPOL-EN16931-R053", n, "%d tax totals with tax subtotals", with)
	tcs := ulist(n, "cbc:TaxCurrencyCode")
	want := 0
	if len(tcs) > 0 {
		want = 1
	}
	k.check(without == want, "PEPPOL-EN16931-R054", n, "%d tax totals without tax subtotals", without)
	if len(tcs) == 0 {
		return
	}
	// R055: the VAT totals in the tax currency and in the invoice currency
	// have the same sign.
	tc, dc := nsv(tcs[0]), nsv(ufirst(n, "cbc:DocumentCurrencyCode"))
	var inTax, inDoc []*xmltree.Node
	for a := range uall(n, "cac:TaxTotal/cbc:TaxAmount") {
		c, ok := a.LookupAttr("currencyID")
		if !ok {
			continue
		}
		if c == tc {
			inTax = append(inTax, a)
		}
		if c == dc {
			inDoc = append(inDoc, a)
		}
	}
	neg := anyNumber(inTax, func(f float64) bool { return f <= 0 }) && anyNumber(inDoc, func(f float64) bool { return f <= 0 })
	pos := anyNumber(inTax, func(f float64) bool { return f >= 0 }) && anyNumber(inDoc, func(f float64) bool { return f >= 0 })
	k.check(neg || pos, "PEPPOL-EN16931-R055", n, "")
}

// documentOrLine reports whether an allowance or charge whose parent is p is
// at document or line level: ubl-invoice:Invoice/cac:AllowanceCharge,
// ubl-invoice:Invoice/cac:InvoiceLine/cac:AllowanceCharge and their credit
// note equivalents.
func documentOrLine(p *xmltree.Node) bool {
	switch {
	case isDoc(p):
		return true
	case uis(p, "cac:InvoiceLine"):
		return isElem(p.Parent, nsInvoice, "Invoice")
	case uis(p, "cac:CreditNoteLine"):
		return isElem(p.Parent, nsCreditNote, "CreditNote")
	}
	return false
}

// allowanceCharge checks a document or line allowance or charge: percentage
// and base amount go together (R041, R042); when both or none are given,
// the amount is base × percentage / 100 (R040) and the indicator is true or
// false (R043).
func (k *ublChecker) allowanceCharge(n *xmltree.Node) {
	pct, base := ufirst(n, "cbc:MultiplierFactorNumeric"), ufirst(n, "cbc:BaseAmount")
	switch {
	case pct != nil && base == nil:
		k.fail("PEPPOL-EN16931-R041", n, "")
		return
	case pct == nil && base != nil:
		k.fail("PEPPOL-EN16931-R042", n, "")
		return
	}
	if pct != nil && base != nil {
		b, ok1 := dec(base)
		p, ok2 := dec(pct)
		amount, ok3 := new(big.Rat), true
		a := ufirst(n, "cbc:Amount")
		if a != nil {
			amount, ok3 = dec(a)
		}
		if ok1 && ok2 && ok3 {
			want := new(big.Rat).Mul(b, p)
			want.Quo(want, big.NewRat(100, 1))
			k.check(within(amount, want, big.NewRat(2, 100)), "PEPPOL-EN16931-R040", n,
				"amount %s, base amount %s × percentage %s / 100 = %s", sv(a), sv(base), sv(pct), want.FloatString(4))
		}
	}
	v := norm(text(ufirst(n, "cbc:ChargeIndicator")))
	k.check(v == "true" || v == "false", "PEPPOL-EN16931-R043", n, "value: %q", v)
}

// currencyChecked reports whether n is an amount whose currency R051
// checks: every amount except the invoice total VAT amount in accounting
// currency (a cac:TaxTotal without cac:TaxSubtotal).
func currencyChecked(n *xmltree.Node) bool {
	if n.Space != nsCbc {
		return false
	}
	switch n.Name {
	case "Amount", "BaseAmount", "PriceAmount", "TaxableAmount", "LineExtensionAmount", "TaxExclusiveAmount",
		"TaxInclusiveAmount", "AllowanceTotalAmount", "ChargeTotalAmount", "PrepaidAmount", "PayableRoundingAmount", "PayableAmount":
		return true
	case "TaxAmount":
		p := n.Parent
		return uis(p, "cac:TaxSubtotal") || uis(p, "cac:TaxTotal") && uexists(p, "cac:TaxSubtotal")
	}
	return false
}

// linePeriod reports whether n is the bound name (cbc:StartDate or
// cbc:EndDate) of a line period, in a document that has an invoice period
// with that bound: ubl-invoice:Invoice[cac:InvoicePeriod/cbc:StartDate]/
// cac:InvoiceLine/cac:InvoicePeriod/cbc:StartDate and the like.
func linePeriod(n *xmltree.Node, name string) bool {
	if !uis(n, name) || !uparent(n, "cac:InvoicePeriod") {
		return false
	}
	line := n.Parent.Parent
	if line == nil {
		return false
	}
	doc := line.Parent
	ok := uis(line, "cac:InvoiceLine") && isElem(doc, nsInvoice, "Invoice") ||
		uis(line, "cac:CreditNoteLine") && isElem(doc, nsCreditNote, "CreditNote")
	return ok && uexists(doc, "cac:InvoicePeriod/"+name)
}

// linePeriodDate checks R110 or R111 on the bound n of a line period: the
// dates are compared as xs:date. A date that is not valid skips the check
// (the reference stops with an error), but a bound without text fails it
// (xs:date of an empty sequence is empty, and the comparison false).
func (k *ublChecker) linePeriodDate(n *xmltree.Node, id string, ok func(line, invoice int64) bool) {
	bound := ufirst(n.Parent.Parent.Parent, "cac:InvoicePeriod/cbc:"+n.Name)
	if n.Text == "" {
		k.fail(id, n, "line date %q, invoice date %q", n.Text, sv(bound))
		return
	}
	line, ok1 := xsDate(n.Text)
	invoice, ok2 := xsDate(sv(bound))
	if ok1 && ok2 {
		k.check(ok(line, invoice), id, n, "line date %q, invoice date %q", n.Text, sv(bound))
	}
}

// line checks an invoice or credit note line: net amount (R120), base
// quantity (R121) and line object references (R100, R101).
func (k *ublChecker) line(n *xmltree.Node) {
	k.lineAmount(n)
	if q := ufirst(n, "cac:Price/cbc:BaseQuantity"); q != nil {
		if v, ok := dec(q); ok {
			k.check(v.Sign() > 0, "PEPPOL-EN16931-R121", q, "value: %q", sv(q))
		}
	}
	refs := ucount(n, "cac:DocumentReference")
	k.check(refs <= 1, "PEPPOL-EN16931-R100", n, "%d document references", refs)
	if refs > 0 {
		k.check(uanyEq(n, "cac:DocumentReference/cbc:DocumentTypeCode", "130"), "PEPPOL-EN16931-R101", n,
			"type code %q", sv(ufirst(n, "cac:DocumentReference/cbc:DocumentTypeCode")))
	}
}

// lineAmount checks R120: line net amount = quantity × (net price / base
// quantity) + charges - allowances, with a tolerance of 0.02. The quantity
// is the invoiced quantity in an invoice, the credited quantity otherwise.
func (k *ublChecker) lineAmount(n *xmltree.Node) {
	value := func(path string, def int64) (*big.Rat, bool) {
		e := ufirst(n, path)
		if e == nil {
			return big.NewRat(def, 1), true
		}
		return dec(e)
	}
	qtyPath := "cbc:CreditedQuantity"
	if k.invoice {
		qtyPath = "cbc:InvoicedQuantity"
	}
	total, ok1 := value("cbc:LineExtensionAmount", 0)
	qty, ok2 := value(qtyPath, 1)
	price, ok3 := value("cac:Price/cbc:PriceAmount", 0)
	base, ok4 := value("cac:Price/cbc:BaseQuantity", 1)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	if base.Sign() == 0 {
		base = big.NewRat(1, 1)
	}
	allowances, ok5 := lineAllowancesUBL(n, "false")
	charges, ok6 := lineAllowancesUBL(n, "true")
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

// lineAllowancesUBL returns the sum of the amounts of the allowances
// (indicator "false") or charges ("true") of a line, rounded to two
// decimals, halves towards positive infinity.
func lineAllowancesUBL(n *xmltree.Node, indicator string) (*big.Rat, bool) {
	sum := new(big.Rat)
	for _, ac := range ulist(n, "cac:AllowanceCharge") {
		if nsv(ufirst(ac, "cbc:ChargeIndicator")) != indicator {
			continue
		}
		for a := range uall(ac, "cbc:Amount") {
			v, ok := dec(a)
			if !ok {
				return nil, false
			}
			sum.Add(sum, v)
		}
	}
	return roundHalfCeil2(sum), true
}

// priceAllowance checks an allowance on the price of a line: it is an
// allowance (R044), and the net price is the gross price minus the
// allowance (R046). A missing net price or allowance amount fails R046
// (xs:decimal of an empty sequence is empty, and the comparison false).
func (k *ublChecker) priceAllowance(n *xmltree.Node) {
	v := nsv(ufirst(n, "cbc:ChargeIndicator"))
	k.check(v == "false", "PEPPOL-EN16931-R044", n, "value: %q", v)
	base := ufirst(n, "cbc:BaseAmount")
	if base == nil {
		return
	}
	price, amount := ufirst(n.Parent, "cbc:PriceAmount"), ufirst(n, "cbc:Amount")
	vals := map[*xmltree.Node]*big.Rat{}
	for _, e := range []*xmltree.Node{price, base, amount} {
		if e == nil {
			continue
		}
		d, ok := dec(e)
		if !ok {
			return
		}
		vals[e] = d
	}
	if price == nil || amount == nil {
		k.fail("PEPPOL-EN16931-R046", n, "net price %q, gross price %q, allowance %q", sv(price), sv(base), sv(amount))
		return
	}
	want := new(big.Rat).Sub(vals[base], vals[amount])
	k.check(vals[price].Cmp(want) == 0, "PEPPOL-EN16931-R046", n, "net price %q, gross price %q, allowance %q", sv(price), sv(base), sv(amount))
}

// baseQuantityUnit checks R130 on the base quantity of a price: when the
// line has a quantity, the units are the same. In an invoice the unit is the
// one of the invoiced quantity, in a credit note of the credited quantity.
func (k *ublChecker) baseQuantityUnit(n *xmltree.Node) {
	line := n.Parent.Parent
	if !uexists(line, "cbc:InvoicedQuantity") && !uexists(line, "cbc:CreditedQuantity") {
		return
	}
	qtyPath := "cbc:CreditedQuantity"
	if k.invoice {
		qtyPath = "cbc:InvoicedQuantity"
	}
	u := n.Attr("unitCode")
	k.check(uanyAttrIn(line, qtyPath, "unitCode", u), "PEPPOL-EN16931-R130", n,
		"unit code %q, quantity in %q", u, attr(ufirst(line, qtyPath), "unitCode"))
}

// identifier checks a cbc:EndpointID, cac:PartyIdentification/cbc:ID or
// cbc:CompanyID against the format of its ICD scheme (PEPPOL-COMMON-R*), and
// a Dutch VAT identifier (PEPPOL-COMMON-R056-2). The rules are tried in the
// order of the schematron: the scheme 9907 is only checked on an electronic
// address, and a Dutch VAT identifier with the scheme 0217 is checked by
// R056-2 instead of R057.
func (k *ublChecker) identifier(n *xmltree.Node) {
	endpoint := uis(n, "cbc:EndpointID")
	company := uis(n, "cbc:CompanyID")
	if !endpoint && !company && !(uis(n, "cbc:ID") && uparent(n, "cac:PartyIdentification")) {
		return
	}
	scheme, ok := n.LookupAttr("schemeID")
	v := nsv(n)
	if ok && scheme != "0217" && (scheme != "9907" || endpoint) {
		if id, valid := icdRule(scheme, v, sv(n)); id != "" {
			k.check(valid, id, n, "scheme %s, value: %q", scheme, v)
			return
		}
	}
	if company && uparent(n, "cac:PartyTaxScheme") && taxSchemeID(n.Parent) == "VAT" && strings.HasPrefix(v, "NL") {
		k.check(dutchVAT(v), "PEPPOL-COMMON-R056-2", n, "value: %q", v)
		return
	}
	if ok && scheme == "0217" {
		id, valid := icdRule(scheme, v, sv(n))
		k.check(valid, id, n, "scheme %s, value: %q", scheme, v)
	}
}
