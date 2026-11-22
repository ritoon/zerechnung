package en16931

import (
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// VAT category rules: BR-S, BR-Z, BR-E, BR-AE, BR-IC (K), BR-G, BR-O, BR-IP
// (IGIC, L), BR-IG (IPSI, M) (the reverse in UBL, see vatCategoriesUBL), and
// the VAT breakdown rules BR-45 to BR-48, BR-CO-03, BR-CO-17, BR-DEC-19 and
// BR-DEC-20.

// rateCond is the condition a category puts on the VAT rate of a line, an
// allowance or a charge.
type rateCond uint8

const (
	rateZero        rateCond = iota // ram:RateApplicablePercent = 0
	ratePositive                    // ram:RateApplicablePercent > 0
	rateNonNegative                 // ram:RateApplicablePercent >= 0
	rateAbsent                      // not(ram:RateApplicablePercent)
)

// partyCond is the condition a category puts on the VAT identifiers of the
// parties.
type partyCond uint8

const (
	partySellerVAFC partyCond = iota // seller VA or FC identifier, or tax representative VA identifier
	partySellerVA                    // seller or tax representative VA identifier
	partyAE                          // partySellerVAFC, and buyer VA or legal identifier
	partyIC                          // partySellerVA, and buyer VA identifier
	partyNone                        // no seller, tax representative or buyer VA identifier
)

// vatCategory describes the rules of one VAT category.
type vatCategory struct {
	name      string    // for messages
	prefix    string    // rule identifier prefix
	seller    partyCond // condition on the VAT identifiers of the parties
	sellerMsg string    // message when the party condition fails
	rate      rateCond  // condition on the rate of lines, allowances and charges
}

const (
	sellerAny  = "the seller has no VAT identifier (BT-31) or tax registration identifier (BT-32) and there is no seller tax representative VAT identifier (BT-63)"
	sellerVAT  = "the seller has no VAT identifier (BT-31) and there is no seller tax representative VAT identifier (BT-63)"
	sellerAE   = sellerAny + ", or the buyer has neither a VAT identifier (BT-48) nor a legal registration identifier (BT-47)"
	sellerIC   = sellerVAT + ", or the buyer has no VAT identifier (BT-48)"
	sellerNone = "the seller VAT identifier (BT-31), the seller tax representative VAT identifier (BT-63) and the buyer VAT identifier (BT-48) must be absent"
)

// partiesOK reports whether the parties satisfy cond.
func (k *checker) partiesOK(cond partyCond) bool {
	vafc, va := k.sellerVAFC || k.taxRepVA, k.sellerVA || k.taxRepVA
	switch cond {
	case partySellerVAFC:
		return vafc
	case partySellerVA:
		return va
	case partyAE:
		return vafc && (k.buyerVA || k.buyerLegalID)
	case partyIC:
		return va && k.buyerVA
	}
	return !k.sellerVA && !k.taxRepVA && !k.buyerVA
}

var vatCategories = [nCat]vatCategory{
	catS:  {name: "S (standard rated)", prefix: "BR-S-", seller: partySellerVAFC, sellerMsg: sellerAny, rate: ratePositive},
	catZ:  {name: "Z (zero rated)", prefix: "BR-Z-", seller: partySellerVAFC, sellerMsg: sellerAny, rate: rateZero},
	catE:  {name: "E (exempt from VAT)", prefix: "BR-E-", seller: partySellerVAFC, sellerMsg: sellerAny, rate: rateZero},
	catAE: {name: "AE (reverse charge)", prefix: "BR-AE-", seller: partyAE, sellerMsg: sellerAE, rate: rateZero},
	catK:  {name: "K (intra-community supply)", prefix: "BR-IC-", seller: partyIC, sellerMsg: sellerIC, rate: rateZero},
	catG:  {name: "G (export outside the EU)", prefix: "BR-G-", seller: partySellerVA, sellerMsg: sellerVAT, rate: rateZero},
	catO:  {name: "O (not subject to VAT)", prefix: "BR-O-", seller: partyNone, sellerMsg: sellerNone, rate: rateAbsent},
	catL:  {name: "L (IGIC, Canary Islands)", prefix: "BR-IP-", seller: partySellerVAFC, sellerMsg: sellerAny, rate: ratePositive},
	catM:  {name: "M (IPSI, Ceuta and Melilla)", prefix: "BR-IG-", seller: partySellerVAFC, sellerMsg: sellerAny, rate: rateNonNegative},
}

// vatCategoriesUBL are the category rules of the UBL artefacts. They differ
// from the CII ones for L and M: the UBL artefacts number the rules of
// category L (IGIC) BR-IG and those of category M (IPSI) BR-IP, the reverse
// of the CII artefacts, and accept a zero rate for category L.
var vatCategoriesUBL = func() [nCat]vatCategory {
	c := vatCategories
	c[catL].prefix, c[catL].rate = "BR-IG-", rateNonNegative
	c[catM].prefix = "BR-IP-"
	return c
}()

// vatOrder is the order of the category rules in the official pattern: when
// an element has several category codes, the first rule wins.
var vatOrder = [...]int{catAE, catL, catM, catE, catG, catK, catO, catS, catZ}

// rateOK reports whether the RateApplicablePercent children of t satisfy cond
// (general comparison: one value is enough).
func rateOK(t *xmltree.Node, cond rateCond) bool {
	for _, c := range t.Children {
		if c.Name != "RateApplicablePercent" {
			continue
		}
		if cond == rateAbsent {
			return false
		}
		v, ok := dec(c)
		if !ok {
			continue
		}
		switch s := v.Sign(); cond {
		case rateZero:
			if s == 0 {
				return true
			}
		case ratePositive:
			if s > 0 {
				return true
			}
		case rateNonNegative:
			if s >= 0 {
				return true
			}
		}
	}
	return cond == rateAbsent
}

// rateFail reports a failed rate condition.
func (k *checker) rateFail(id string, t *xmltree.Node, subject string, cond rateCond) {
	r := t.Elem("RateApplicablePercent")
	var want string
	switch cond {
	case rateZero:
		want = "must be 0"
	case ratePositive:
		want = "must be greater than 0"
	case rateNonNegative:
		want = "must be 0 or greater"
	case rateAbsent:
		k.fail(id, t, "%s: the VAT rate %q must be absent", subject, quote(r.Text))
		return
	}
	if r == nil {
		k.fail(id, t, "%s: the VAT rate is missing, it %s", subject, want)
		return
	}
	k.fail(id, t, "%s: the VAT rate %q %s", subject, quote(r.Text), want)
}

// categories returns the categories of the CategoryCode children of n, and
// whether one of them is not O.
func categories(n *xmltree.Node) (present [nCat]bool, notO, any bool) {
	for _, c := range n.Children {
		if c.Name != "CategoryCode" {
			continue
		}
		any = true
		if c.Text != "O" {
			notO = true
		}
		if i := catIndex(c.Text); i >= 0 {
			present[i] = true
		}
	}
	return present, notO, any
}

// categoryTradeTax counts the allowance and charge VAT categories and checks
// BR-x-03, BR-x-04, BR-x-06 and BR-x-07 (contexts
// //ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator=false()|true()]/ram:CategoryTradeTax[ram:CategoryCode = 'x'][upper-case(ram:TypeCode) = 'VAT']).
func (k *checker) categoryTradeTax(n *xmltree.Node) {
	present, notO, _ := categories(n)
	if notO {
		k.categoryNotO = true
	}
	vat := isVAT(n)
	for i, p := range present {
		if p {
			k.catTax[i]++
			if vat {
				k.catTaxVAT[i]++
			}
		}
	}
	if n.Parent.Name != "SpecifiedTradeAllowanceCharge" || !vat {
		return
	}
	allowance, charge := chargeIndicator(n.Parent)
	if !allowance && !charge {
		return
	}
	for _, i := range vatOrder {
		if !present[i] {
			continue
		}
		v := &k.cats[i]
		seller, rate, what := "04", "07", "charge"
		if allowance {
			seller, rate, what = "03", "06", "allowance"
		}
		if !k.partiesOK(v.seller) {
			k.fail(v.prefix+seller, n, "%s with VAT category %s: %s", what, v.name, v.sellerMsg)
		}
		if !rateOK(n, v.rate) {
			k.rateFail(v.prefix+rate, n, what+" with VAT category "+v.name, v.rate)
		}
		return
	}
}

// applicableTradeTax counts the line and breakdown VAT categories and checks
// BR-x-02 and BR-x-05 (contexts
// //rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax[ram:CategoryCode = 'x'][upper-case(ram:TypeCode) = 'VAT']).
func (k *checker) applicableTradeTax(n *xmltree.Node) {
	present, notO, _ := categories(n)
	if notO {
		k.applicableNotO = true
	}
	vat := isVAT(n)
	switch n.Parent.Name {
	case "ApplicableHeaderTradeSettlement":
		for i, p := range present {
			if p {
				k.catHdr[i]++
				if vat {
					k.catHdrVAT[i]++
				}
			}
		}
	case "SpecifiedLineTradeSettlement":
		plain := k.mode == modeExtendedCTCFR && n.Parent.Parent != nil && detail(n.Parent.Parent)
		for i, p := range present {
			if p {
				k.catLine[i]++
				if vat {
					k.catLineVAT[i]++
				}
				if plain {
					k.catDetail[i]++
				}
			}
		}
		if !isVAT(n) || !k.parentIs("SpecifiedLineTradeSettlement", "IncludedSupplyChainTradeLineItem", "SupplyChainTradeTransaction") {
			return
		}
		for _, i := range vatOrder {
			if !present[i] {
				continue
			}
			v := &k.cats[i]
			if !k.partiesOK(v.seller) || !rateOK(n, v.rate) {
				subject := "line " + lineID(n.Parent.Parent) + " with VAT category " + v.name
				if !k.partiesOK(v.seller) {
					k.fail(v.prefix+"02", n, "%s: %s", subject, v.sellerMsg)
				}
				if !rateOK(n, v.rate) {
					k.rateFail(v.prefix+"05", n, subject, v.rate)
				}
			}
			return
		}
	}
}

// categoriesPresent checks BR-x-01 (context /rsm:CrossIndustryInvoice).
func (k *checker) categoriesPresent() {
	for _, i := range vatOrder {
		hdr, line, tax := k.catHdr[i], k.catLine[i], k.catTax[i]
		v := &k.cats[i]
		if k.ubl {
			if !k.categoryPresentUBL(i) {
				k.fail(v.prefix+"01", k.d.Root, ubl01Message(i), v.name, hdr, line, tax)
			}
			continue
		}
		var ok bool
		switch i {
		case catS, catL, catM:
			ok = (line+hdr >= 2 || line == 0) && (tax+hdr >= 2 || tax == 0)
		case catO:
			ok = hdr == 0 || (hdr == 1 && (line > 0 || tax > 0))
		default:
			ok = (hdr == 0 && line == 0 && tax == 0) || (hdr == 1 && (line > 0 || tax > 0))
		}
		if ok {
			continue
		}
		var msg string
		switch {
		case i == catS || i == catL || i == catM:
			msg = "an invoice with lines, allowances or charges of VAT category %s must have at least one VAT breakdown (BG-23) of that category (%d breakdown(s), %d line(s), %d allowance(s) or charge(s))"
		case hdr > 1:
			msg = "an invoice must have exactly one VAT breakdown (BG-23) of VAT category %s (%d breakdowns, %d line(s), %d allowance(s) or charge(s))"
		default:
			msg = "an invoice with lines, allowances or charges of VAT category %s must have exactly one VAT breakdown (BG-23) of that category, and such a breakdown needs lines, allowances or charges of that category (%d breakdown(s), %d line(s), %d allowance(s) or charge(s))"
		}
		k.fail(v.prefix+"01", k.d.Root, msg, v.name, hdr, line, tax)
	}
}

// categoryPresentUBL reports whether BR-x-01 holds for category i, as the
// UBL artefacts test it (context /ubl:Invoice | /cn:CreditNote). Their tests
// differ from the CII ones:
//
//   - S (BR-S-01): a VAT breakdown of the category exists if and only if a
//     line, allowance or charge has it, the tax type not being tested;
//   - L (BR-IG-01) and M (BR-IP-01): the same, for the categories of tax type
//     VAT, except that the first clause of BR-IG-01 counts the breakdowns of
//     category L of any tax type;
//   - the other categories: when the category appears anywhere (cac:TaxCategory,
//     breakdowns included, or cac:ClassifiedTaxCategory, of tax type VAT),
//     exactly one VAT breakdown has it.
//
// The UBL tests count the cac:TaxCategory of every cac:AllowanceCharge,
// including those of the lines, which EN 16931 does not define and the view
// does not carry.
func (k *checker) categoryPresentUBL(i int) bool {
	lines := k.catLine[i] + k.catTax[i]
	linesVAT := k.catLineVAT[i] + k.catTaxVAT[i]
	switch i {
	case catS:
		return (lines > 0) == (k.catHdr[i] > 0)
	case catL:
		return linesVAT > 0 && k.catHdr[i] > 0 || linesVAT == 0 && k.catHdrVAT[i] == 0
	case catM:
		return (linesVAT > 0) == (k.catHdrVAT[i] > 0)
	}
	return linesVAT+k.catHdrVAT[i] == 0 || k.catHdrVAT[i] == 1
}

// ubl01Message is the message of a failed BR-x-01 in UBL (see
// categoryPresentUBL).
func ubl01Message(i int) string {
	switch i {
	case catS, catL, catM:
		return "an invoice must have a VAT breakdown (BG-23) of VAT category %s if and only if it has lines, allowances or charges of that category (%d breakdown(s), %d line(s), %d allowance(s) or charge(s))"
	}
	return "an invoice where VAT category %s appears must have exactly one VAT breakdown (BG-23) of that category (%d breakdown(s), %d line(s), %d allowance(s) or charge(s))"
}

// hasExemption reports whether a VAT breakdown has an exemption reason text
// or code.
func hasExemption(t *xmltree.Node) bool {
	return t.Elem("ExemptionReason") != nil || t.Elem("ExemptionReasonCode") != nil
}

// calculatedZero reports whether ram:CalculatedAmount = 0.
func calculatedZero(t *xmltree.Node) bool {
	for _, c := range t.Children {
		if c.Name == "CalculatedAmount" {
			if v, ok := dec(c); ok && v.IsZero() {
				return true
			}
		}
	}
	return false
}

// taxMatches reports whether one of the children of n named name has the
// category code cat and, when rate is not nil, whether one of them has that
// rate.
func taxMatches(n *xmltree.Node, name, cat string, rate *invoice.Decimal) bool {
	hasCat, hasRate := false, rate == nil
	for _, t := range n.Children {
		if t.Name != name {
			continue
		}
		for _, c := range t.Children {
			switch c.Name {
			case "CategoryCode":
				if c.Text == cat {
					hasCat = true
				}
			case "RateApplicablePercent":
				if rate != nil {
					if v, ok := dec(c); ok && v.Equal(*rate) {
						hasRate = true
					}
				}
			}
		}
	}
	return hasCat && hasRate
}

// categoryBasis returns the expected taxable amount of a VAT category (and
// rate): the rounded sum of the line net amounts plus the rounded sum of the
// document level charges minus the rounded sum of the document level
// allowances of that category.
func (k *checker) categoryBasis(cat string, rate *invoice.Decimal) invoice.Decimal {
	lines, charges, allowances, _ := k.categorySums(cat, rate)
	return round2(lines).Add(round2(charges)).Sub(round2(allowances))
}

// categoryTotal returns the sum of the line net amounts plus the sum of the
// document level charges minus the sum of the document level allowances of
// a VAT category (and rate), unrounded as in the UBL artefacts, and the
// number of lines, allowances and charges of that category.
func (k *checker) categoryTotal(cat string, rate *invoice.Decimal) (invoice.Decimal, int) {
	lines, charges, allowances, n := k.categorySums(cat, rate)
	return lines.Add(charges).Sub(allowances), n
}

// categorySums returns the sums of the line net amounts, document level
// charges and document level allowances of a VAT category (and rate), and
// the number of lines, allowances and charges of that category.
func (k *checker) categorySums(cat string, rate *invoice.Decimal) (lines, charges, allowances invoice.Decimal, n int) {
	lines, charges, allowances = zero, zero, zero
	for _, l := range children(k.d.Transaction) {
		switch l.Name {
		case "IncludedSupplyChainTradeLineItem":
			for _, s := range l.Children {
				if s.Name != "SpecifiedLineTradeSettlement" || !taxMatches(s, "ApplicableTradeTax", cat, rate) {
					continue
				}
				n++
				for _, m := range s.Children {
					if m.Name != "SpecifiedTradeSettlementLineMonetarySummation" {
						continue
					}
					if v, ok := dec(m.Elem("LineTotalAmount")); ok {
						lines = lines.Add(v)
					}
				}
			}
		case "ApplicableHeaderTradeSettlement":
			for _, ac := range l.Children {
				if ac.Name != "SpecifiedTradeAllowanceCharge" || !taxMatches(ac, "CategoryTradeTax", cat, rate) {
					continue
				}
				n++
				v, ok := dec(ac.Elem("ActualAmount"))
				if !ok {
					continue
				}
				allowance, charge := chargeIndicator(ac)
				if allowance {
					allowances = allowances.Add(v)
				}
				if charge {
					charges = charges.Add(v)
				}
			}
		}
	}
	return lines, charges, allowances, n
}

// breakdowns checks the VAT breakdowns (BG-23) of a header settlement.
func (k *checker) breakdowns(s *xmltree.Node) {
	underTransaction := s.Parent != nil && s.Parent.Name == "SupplyChainTradeTransaction"
	for _, t := range s.Children {
		if t.Name != "ApplicableTradeTax" {
			continue
		}
		vat := isVAT(t)
		special := vat && underTransaction
		switch {
		case special && childText(t, "CategoryCode", "L"):
			k.canaryBreakdown(t, catL)
		case special && childText(t, "CategoryCode", "M"):
			k.canaryBreakdown(t, catM)
		case special && childText(t, "CategoryCode", "O"):
			k.notSubjectBreakdown(t)
		default:
			k.breakdown(t)
		}
		k.categoryCodeRules(s, t)
	}
}

// categoryCodeRules checks the rules of contexts on the ram:CategoryCode of
// a VAT breakdown t of header settlement s: BR-AE-08 to BR-AE-10, BR-E-08 to
// BR-E-10, BR-G-08 to BR-G-10 and BR-IC-08 to BR-IC-12 for the breakdowns of
// type VAT of the transaction, BR-Z-08 to BR-Z-10 and BR-S-08 to BR-S-10 for
// all.
func (k *checker) categoryCodeRules(s, t *xmltree.Node) {
	special := isVAT(t) && s.Parent != nil && s.Parent.Name == "SupplyChainTradeTransaction"
	for _, c := range t.Children {
		if c.Name != "CategoryCode" {
			continue
		}
		switch c.Text {
		case "AE":
			if special {
				k.zeroBreakdown(s, t, catAE, true)
			}
		case "E":
			if special {
				k.zeroBreakdown(s, t, catE, true)
			}
		case "G":
			if special {
				k.zeroBreakdown(s, t, catG, true)
			}
		case "K":
			if special {
				k.zeroBreakdown(s, t, catK, true)
			}
		case "Z":
			k.zeroBreakdown(s, t, catZ, false)
		case "S":
			k.standardBreakdown(t)
		}
	}
}

// canaryBreakdown checks BR-IP-08 to BR-IP-10 and BR-IG-08 to BR-IG-10 on a
// VAT breakdown of category L or M. In the CII artefacts the first two
// assertions are always true; the UBL artefacts check the taxable amount and
// the tax amount, within one unit.
func (k *checker) canaryBreakdown(t *xmltree.Node, cat int) {
	v := &k.cats[cat]
	if k.ubl {
		basis, calc := t.Elem("BasisAmount"), t.Elem("CalculatedAmount")
		b, bok := dec(basis)
		for _, r := range t.Children {
			if r.Name != "RateApplicablePercent" {
				continue
			}
			rate, ok := dec(r)
			if !ok {
				continue
			}
			want, _ := k.categoryTotal(catCodes[cat], &rate)
			if !bok || !within(b, want, false) {
				k.fail(v.prefix+"08", t, "VAT category %s taxable amount (BT-116) %q at rate %q is not the sum of the line net amounts minus allowances plus charges of that category and rate (%s, tolerance 1)", v.name, quote(text(basis)), quote(r.Text), want.String())
			}
		}
		ok := false
		var want invoice.Decimal
		if c, cok := dec(calc); cok && bok {
			if rate, rok := dec(t.Elem("RateApplicablePercent")); rok {
				want = percentOf(b, rate)
				ok = within(c.Abs(), want, false)
			}
		}
		if !ok {
			k.fail(v.prefix+"09", t, "VAT category %s tax amount (BT-117) %q is not VAT category taxable amount (BT-116) %q × rate (BT-119) %q / 100 = %s (tolerance 1)",
				v.name, quote(text(calc)), quote(text(basis)), quote(text(t.Elem("RateApplicablePercent"))), want.String())
		}
	}
	if hasExemption(t) {
		k.fail(v.prefix+"10", t, "a VAT breakdown of category %s must not have a VAT exemption reason (BT-120, BT-121)", v.name)
	}
}

// breakdown checks BR-45 to BR-48, BR-CO-03, BR-CO-17, BR-DEC-19 and
// BR-DEC-20 (context //ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax).
func (k *checker) breakdown(t *xmltree.Node) {
	basis, calc := t.Elem("BasisAmount"), t.Elem("CalculatedAmount")
	if basis == nil {
		k.fail("BR-45", t, "a VAT breakdown (BG-23) has no VAT category taxable amount (BT-116)")
	}
	if calc == nil {
		k.fail("BR-46", t, "a VAT breakdown (BG-23) has no VAT category tax amount (BT-117)")
	}
	vat := isVAT(t)
	if !vat || t.Elem("CategoryCode") == nil {
		k.fail("BR-47", t, "a VAT breakdown (BG-23) has no VAT category code (BT-118) with tax type VAT")
	}
	if !vat || (t.Elem("RateApplicablePercent") == nil && !childText(t, "CategoryCode", "O")) {
		k.fail("BR-48", t, "a VAT breakdown (BG-23) of category %q has no VAT category rate (BT-119)", quote(text(t.Elem("CategoryCode"))))
	}
	if k.taxPointDate && k.dueDateTypeCode {
		k.fail("BR-CO-03", t, "the invoice has both a value added tax point date (BT-7) and a value added tax point date code (BT-8)")
	}
	k.vatAmount(t, basis, calc)
	if tooManyDecimals(basis) {
		k.fail("BR-DEC-19", basis, "VAT category taxable amount (BT-116) %q has more than 2 decimals", quote(basis.Text))
	}
	if tooManyDecimals(calc) {
		k.fail("BR-DEC-20", calc, "VAT category tax amount (BT-117) %q has more than 2 decimals", quote(calc.Text))
	}
}

// vatAmount checks BR-CO-17: the VAT category tax amount is the taxable
// amount multiplied by the rate, within one unit.
func (k *checker) vatAmount(t, basis, calc *xmltree.Node) {
	c, cok := dec(calc)
	var rate invoice.Decimal
	rok := false
	if tc := t.Elem("TypeCode"); tc != nil && upperIs(trim(tc.Text), "VAT") {
		rate, rok = dec(t.Elem("RateApplicablePercent"))
	}
	var expected invoice.Decimal
	ok := false
	switch {
	case !cok:
	case rok && rate.RoundHalfCeil(0).IsZero():
		ok = c.RoundHalfCeil(0).IsZero()
	case rok:
		if b, bok := dec(basis); bok {
			expected = percentOf(b, rate)
			ok = within(c.Abs(), expected, !k.ubl) // UBL: strict comparisons
		}
	default:
		ok = c.RoundHalfCeil(0).IsZero()
	}
	if ok {
		return
	}
	if rok && expected.IsSet() {
		k.fail("BR-CO-17", t, "VAT category tax amount (BT-117) %q is not VAT category taxable amount (BT-116) %q × rate (BT-119) %q / 100 = %s (tolerance 1)",
			quote(text(calc)), quote(text(basis)), quote(text(t.Elem("RateApplicablePercent"))), expected.String())
		return
	}
	k.fail("BR-CO-17", t, "VAT category tax amount (BT-117) %q is not consistent with VAT category taxable amount (BT-116) %q and rate (BT-119) %q",
		quote(text(calc)), quote(text(basis)), quote(text(t.Elem("RateApplicablePercent"))))
}

// notSubjectBreakdown checks BR-O-08 to BR-O-14 (context
// //rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax[ram:CategoryCode = 'O'][upper-case(ram:TypeCode) = 'VAT']).
func (k *checker) notSubjectBreakdown(t *xmltree.Node) {
	basis := t.Elem("BasisAmount")
	want := k.categoryBasis("O", nil)
	if b, ok := dec(basis); !ok || !b.Equal(want) {
		k.fail("BR-O-08", t, "VAT category O taxable amount (BT-116) %q is not the sum of the line net amounts minus allowances plus charges of category O (%s)", quote(text(basis)), want.String())
	}
	if !calculatedZero(t) {
		k.fail("BR-O-09", t, "VAT category O tax amount (BT-117) %q must be 0", quote(text(t.Elem("CalculatedAmount"))))
	}
	if !hasExemption(t) {
		k.fail("BR-O-10", t, "a VAT breakdown of category O (not subject to VAT) has no VAT exemption reason (BT-120) or code (BT-121)")
	}
	if k.applicableNotO {
		k.fail("BR-O-11", t, "an invoice with a VAT breakdown of category O (not subject to VAT) must not contain VAT breakdowns of other categories")
		k.fail("BR-O-12", t, "an invoice with a VAT breakdown of category O (not subject to VAT) must not contain lines of other VAT categories")
	}
	if k.categoryNotO {
		k.fail("BR-O-13", t, "an invoice with a VAT breakdown of category O (not subject to VAT) must not contain document level allowances of other VAT categories")
		k.fail("BR-O-14", t, "an invoice with a VAT breakdown of category O (not subject to VAT) must not contain document level charges of other VAT categories")
	}
}

// zeroBreakdown checks BR-x-08 to BR-x-10 for the categories with a zero
// tax amount, plus BR-IC-11 and BR-IC-12 (contexts
// .../ram:ApplicableTradeTax/ram:CategoryCode[. = 'x']). exempt tells whether
// an exemption reason is required (AE, E, G, K) or forbidden (Z).
func (k *checker) zeroBreakdown(s, t *xmltree.Node, cat int, exempt bool) {
	v := &k.cats[cat]
	basis := t.Elem("BasisAmount")
	if k.ubl {
		// The UBL artefacts compare the exact sums, without tolerance.
		want, _ := k.categoryTotal(catCodes[cat], nil)
		if b, ok := dec(basis); !ok || !b.Equal(want) {
			k.fail(v.prefix+"08", t, "VAT category %s taxable amount (BT-116) %q is not the sum of the line net amounts minus allowances plus charges of that category (%s)", v.name, quote(text(basis)), want.String())
		}
	} else {
		want := k.categoryBasis(catCodes[cat], nil)
		if b, ok := dec(basis); !ok || !within(b, want, false) {
			k.fail(v.prefix+"08", t, "VAT category %s taxable amount (BT-116) %q is not the sum of the line net amounts minus allowances plus charges of that category (%s, tolerance 1)", v.name, quote(text(basis)), want.String())
		}
	}
	if !calculatedZero(t) {
		k.fail(v.prefix+"09", t, "VAT category %s tax amount (BT-117) %q must be 0", v.name, quote(text(t.Elem("CalculatedAmount"))))
	}
	switch {
	case exempt && !hasExemption(t):
		k.fail(v.prefix+"10", t, "a VAT breakdown of category %s has no VAT exemption reason (BT-120) or code (BT-121)", v.name)
	case !exempt && hasExemption(t):
		k.fail(v.prefix+"10", t, "a VAT breakdown of category %s must not have a VAT exemption reason (BT-120, BT-121)", v.name)
	}
	if cat != catK {
		return
	}
	if !exists(k.d.Delivery, "ActualDeliverySupplyChainEvent", "OccurrenceDateTime", "DateTimeString") &&
		!exists(s, "BillingSpecifiedPeriod", "StartDateTime") && !exists(s, "BillingSpecifiedPeriod", "EndDateTime") {
		k.fail("BR-IC-11", t, "an intra-community supply (VAT category K) needs an actual delivery date (BT-72) or an invoicing period (BG-14)")
	}
	if !exists(k.d.Delivery, "ShipToTradeParty", "PostalTradeAddress", "CountryID") {
		k.fail("BR-IC-12", t, "an intra-community supply (VAT category K) needs a deliver to country code (BT-80)")
	}
}

// standardBreakdown checks BR-S-08 to BR-S-10 (context
// //ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax/ram:CategoryCode[.='S']).
func (k *checker) standardBreakdown(t *xmltree.Node) {
	basis, calc := t.Elem("BasisAmount"), t.Elem("CalculatedAmount")
	b, bok := dec(basis)
	for _, r := range t.Children {
		if r.Name != "RateApplicablePercent" {
			continue
		}
		rate, ok := dec(r)
		if !ok {
			continue
		}
		if k.ubl {
			// The UBL artefacts compare the exact sums within one unit, and
			// require a line, an allowance or a charge at that rate.
			want, n := k.categoryTotal("S", &rate)
			if n == 0 || !bok || !within(b, want, false) {
				k.fail("BR-S-08", t, "VAT category S taxable amount (BT-116) %q at rate %q is not the sum of the line net amounts minus allowances plus charges of category S at that rate (%s, tolerance 1)", quote(text(basis)), quote(r.Text), want.String())
			}
			continue
		}
		want := k.categoryBasis("S", &rate)
		if !bok || !b.Equal(want) {
			k.fail("BR-S-08", t, "VAT category S taxable amount (BT-116) %q at rate %q is not the sum of the line net amounts minus allowances plus charges of category S at that rate (%s)", quote(text(basis)), quote(r.Text), want.String())
		}
	}
	ok := false
	var want invoice.Decimal
	rate, rok := dec(t.Elem("RateApplicablePercent"))
	if c, cok := dec(calc); cok && bok && rok {
		want = percentOf(b, rate)
		ok = within(c.Abs(), want, false)
	}
	if !ok {
		k.fail("BR-S-09", t, "VAT category S tax amount (BT-117) %q is not VAT category taxable amount (BT-116) %q × rate (BT-119) %q / 100 = %s (tolerance 1)",
			quote(text(calc)), quote(text(basis)), quote(text(t.Elem("RateApplicablePercent"))), want.String())
	}
	if hasExemption(t) {
		k.fail("BR-S-10", t, "a VAT breakdown of category S (standard rated) must not have a VAT exemption reason (BT-120, BT-121)")
	}
}
