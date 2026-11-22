package facturx

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// This file holds the rules of the MINIMUM and BASIC WL schematrons. Each
// function names the contexts of the official rules it implements; the rule
// table gives the profiles each rule belongs to, and fail drops the findings
// of the rules that do not belong to the profile checked.

var (
	one       = invoice.DecimalFromInt(1)
	hundredth = invoice.NewDecimal(1, 2)
)

// vatPrefixes lists the country prefixes accepted by BR-CO-09 in the
// official schematron: ISO 3166-1 alpha-2 codes plus EL (Greece), XI
// (Northern Ireland) and 1A (Kosovo).
const vatPrefixes = " 1A AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH EL ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS XI YE YT ZA ZM ZW "

// Code lists of the Factur-X code database used by the MINIMUM and BASIC
// WL schematrons.
var (
	// invoiceTypeCodes is the UNTDID 1001 subset of the invoice type code
	// (BT-3), code list 2.
	invoiceTypeCodes = sync.OnceValue(func() map[string]bool {
		return codeSet("71 80 81 82 83 84 102 130 202 203 204 211 218 219 261 262 295 296 308 325 326 331 380 381 382 383 384 385 386 387 388 389 390 393 394 395 396 420 456 457 458 471 472 473 500 501 502 503 527 532 553 575 623 633 751 780 817 870 875 876 877 935")
	})
	// vatCategoryCodes is the UNTDID 5305 subset of the VAT category codes.
	vatCategoryCodes = sync.OnceValue(func() map[string]bool { return codeSet("AE E G K L M O S Z") })
	// vatDateCodes is the UNTDID 2005 subset of the VAT point date code (BT-8).
	vatDateCodes = codeSet("5 29 72")
	// allowanceReasonCodes is the UNTDID 5189 subset of the document level
	// allowance reason code (BT-98).
	allowanceReasonCodes = sync.OnceValue(func() map[string]bool {
		return codeSet("41 42 60 62 63 64 65 66 67 68 70 71 88 95 100 102 103 104 105")
	})
)

func codeSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, c := range strings.Fields(s) {
		m[c] = true
	}
	return m
}

// walkState collects what the rules need from the whole document.
type walkState struct {
	nonOTax    bool // an ram:ApplicableTradeTax has a category other than O
	nonOCatTax bool // an ram:CategoryTradeTax has a category other than O
}

// light runs the MINIMUM and BASIC WL rules.
func (k *checker) light() {
	d := k.d
	k.seller = d.Agreement.Elem("SellerTradeParty")
	k.buyer = d.Agreement.Elem("BuyerTradeParty")
	k.taxRep = d.Agreement.Elem("SellerTaxRepresentativeTradeParty")
	k.payee = d.Settlement.Elem("PayeeTradeParty")
	k.shipTo = d.Delivery.Elem("ShipToTradeParty")
	k.icc = d.Settlement.Elem("InvoiceCurrencyCode")
	k.tcc = d.Settlement.Elem("TaxCurrencyCode")

	var w walkState
	k.walk(d.Root, &w)
	k.document()
	k.parties()
	k.totals()
	k.breakdown(&w)
	k.vatTotals(false)
	k.period()
	k.allowances()
	if k.p == mMin {
		k.lines()
	}
	k.payment()
	k.structure()
	k.unused()
}

// walk visits the whole document once: BR-CO-09 (every VAT identifier) and
// FX-DATE (every date), and the facts needed by BR-O-11 to BR-O-14, whose
// contexts use the descendant axis.
func (k *checker) walk(n *xmltree.Node, w *walkState) {
	switch n.Name {
	case "ApplicableTradeTax", "CategoryTradeTax":
		for _, ch := range n.Children {
			if ch.Name == "CategoryCode" && ch.Text != "O" {
				if n.Name == "ApplicableTradeTax" {
					w.nonOTax = true
				} else {
					w.nonOCatTax = true
				}
			}
		}
	case "DateTimeString":
		if n.Attr("format") == "102" && !validDate102(n.Text) {
			k.fail("FX-DATE", n, "%s: %q is not a valid date in format 102 (CCYYMMDD)", display(n.Parent), strings.TrimSpace(n.Text))
		}
	case "ID":
		if n.Parent != nil && n.Parent.Name == "SpecifiedTaxRegistration" {
			if s, ok := n.LookupAttr("schemeID"); ok && s == "VA" && !strings.Contains(vatPrefixes, " "+firstRunes(n.Text, 2)+" ") {
				k.fail("BR-CO-09", n, "the VAT identifier %q does not start with an ISO 3166-1 alpha-2 country code (or EL, XI)", n.Text)
			}
		}
	}
	for _, ch := range n.Children {
		k.walk(ch, w)
	}
}

// validDate102 reports whether s, trimmed, is a calendar day written
// CCYYMMDD.
func validDate102(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	_, err := invoice.ParseDate(s, "102")
	return err == nil
}

// firstRunes returns the first n characters of s, as XPath substring(s, 1, n).
func firstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// document implements the rules of context /rsm:CrossIndustryInvoice (BR-01
// to BR-11, BR-62, BR-63) and //ram:SellerTradeParty (BR-CO-26).
func (k *checker) document() {
	d := k.d
	if !filled(d.Context.Elem("GuidelineSpecifiedDocumentContextParameter").Elem("ID")) {
		k.fail("BR-01", or(d.Context, d.Root), "the specification identifier (BT-24) is missing")
	}
	if !filled(d.Header.Elem("ID")) {
		k.fail("BR-02", or(d.Header, d.Root), "the invoice number (BT-1) is missing")
	}
	issue := false
	for _, ch := range kids(d.Header.Elem("IssueDateTime")) {
		if ch.Name == "DateTimeString" && ch.Attr("format") == "102" && ch.Text != "" {
			issue = true
		}
	}
	if !issue {
		k.fail("BR-03", or(d.Header, d.Root), "the invoice issue date (BT-2) is missing or not in format 102")
	}
	if !filled(d.Header.Elem("TypeCode")) {
		k.fail("BR-04", or(d.Header, d.Root), "the invoice type code (BT-3) is missing")
	}
	if !filled(k.icc) {
		k.fail("BR-05", or(d.Settlement, d.Root), "the invoice currency code (BT-5) is missing")
	}
	if !filled(k.seller.Elem("Name")) {
		k.fail("BR-06", or(k.seller, d.Agreement), "the seller name (BT-27) is missing")
	}
	if !filled(k.buyer.Elem("Name")) {
		k.fail("BR-07", or(k.buyer, d.Agreement), "the buyer name (BT-44) is missing")
	}
	sellerAddr := k.seller.Elem("PostalTradeAddress")
	if sellerAddr == nil {
		k.fail("BR-08", or(k.seller, d.Agreement), "the seller postal address (BG-5) is missing")
	}
	if !filled(sellerAddr.Elem("CountryID")) {
		k.fail("BR-09", or(sellerAddr, k.seller), "the seller country code (BT-40) is missing")
	}
	buyerAddr := k.buyer.Elem("PostalTradeAddress")
	if buyerAddr == nil {
		k.fail("BR-10", or(k.buyer, d.Agreement), "the buyer postal address (BG-8) is missing")
	}
	if !filled(buyerAddr.Elem("CountryID")) {
		k.fail("BR-11", or(buyerAddr, k.buyer), "the buyer country code (BT-55) is missing")
	}
	if uc := k.seller.Elem("URIUniversalCommunication"); uc != nil && strings.TrimSpace(uc.Elem("URIID").Attr("schemeID")) == "" {
		k.fail("BR-62", uc, "the seller electronic address (BT-34) has no scheme identifier")
	}
	if uc := k.buyer.Elem("URIUniversalCommunication"); uc != nil && strings.TrimSpace(uc.Elem("URIID").Attr("schemeID")) == "" {
		k.fail("BR-63", uc, "the buyer electronic address (BT-49) has no scheme identifier")
	}
	if s := k.seller; s != nil && s.Elem("ID") == nil && s.Elem("GlobalID") == nil &&
		s.Elem("SpecifiedLegalOrganization").Elem("ID") == nil && !hasTaxID(s, "VA", "") {
		k.fail("BR-CO-26", s, "the seller has no identifier (BT-29), legal registration identifier (BT-30) nor VAT identifier (BT-31)")
	}
}

// parties implements the rules of the contexts //ram:PayeeTradeParty
// (BR-17), //ram:SellerTaxRepresentativeTradeParty (BR-18 to BR-20, BR-56),
// ram:ApplicableHeaderTradeDelivery (BR-57) and
// ram:ApplicableHeaderTradeAgreement (CII-SR-465, CII-SR-466).
func (k *checker) parties() {
	// BR-17 also compares the payee with "../ram:SellerTradeParty", which
	// does not exist next to ram:PayeeTradeParty: only the name is checked.
	if k.payee != nil && k.payee.Elem("Name") == nil {
		k.fail("BR-17", k.payee, "the payee name (BT-59) is missing")
	}
	if t := k.taxRep; t != nil {
		if blank(t.Elem("Name")) {
			k.fail("BR-18", t, "the seller tax representative name (BT-62) is missing or blank")
		}
		addr := t.Elem("PostalTradeAddress")
		if addr == nil {
			k.fail("BR-19", t, "the seller tax representative postal address (BG-12) is missing")
		}
		if blank(addr.Elem("CountryID")) {
			k.fail("BR-20", or(addr, t), "the seller tax representative country code (BT-69) is missing or blank")
		}
		vat := false
		for _, r := range t.Children {
			if r.Name != "SpecifiedTaxRegistration" {
				continue
			}
			for _, id := range r.Children {
				if id.Name == "ID" && id.Attr("schemeID") == "VA" && id.Text != "" {
					vat = true
				}
			}
		}
		if !vat {
			k.fail("BR-56", t, "the seller tax representative VAT identifier (BT-63) is missing")
		}
	}
	if addr := k.shipTo.Elem("PostalTradeAddress"); addr != nil && !filled(addr.Elem("CountryID")) {
		k.fail("BR-57", addr, "the deliver to country code (BT-80) is missing")
	}
	for _, c := range [...]struct {
		id, who string
		party   *xmltree.Node
	}{{"CII-SR-465", "seller", k.seller}, {"CII-SR-466", "buyer", k.buyer}} {
		if hasGrandChild(c.party, "DefinedTradeContact", "PersonName") && hasGrandChild(c.party, "DefinedTradeContact", "DepartmentName") {
			k.fail(c.id, c.party.Elem("DefinedTradeContact"), "the %s contact has both a person name and a department name", c.who)
		}
	}
}

// totals implements the rules of the contexts
// //ram:SpecifiedTradeSettlementHeaderMonetarySummation and its
// ram:TaxTotalAmount in the invoice currency (BR-CO-14).
func (k *checker) totals() {
	sum := k.d.Summation
	if sum == nil {
		return
	}
	lt := sum.Elem("LineTotalAmount")
	ct := sum.Elem("ChargeTotalAmount")
	at := sum.Elem("AllowanceTotalAmount")
	tb := sum.Elem("TaxBasisTotalAmount")
	gt := sum.Elem("GrandTotalAmount")
	tp := sum.Elem("TotalPrepaidAmount")
	ra := sum.Elem("RoundingAmount")
	dp := sum.Elem("DuePayableAmount")
	for _, c := range [...]struct {
		id, bt string
		n      *xmltree.Node
	}{{"BR-12", "the sum of invoice line net amounts (BT-106)", lt}, {"BR-13", "the invoice total amount without VAT (BT-109)", tb},
		{"BR-14", "the invoice total amount with VAT (BT-112)", gt}, {"BR-15", "the amount due for payment (BT-115)", dp}} {
		if c.n == nil && k.applies(c.id) {
			k.fail(c.id, sum, "%s is missing", c.bt)
		}
	}
	for _, c := range [...]struct {
		id string
		n  *xmltree.Node
	}{{"BR-DEC-09", lt}, {"BR-DEC-10", at}, {"BR-DEC-11", ct}, {"BR-DEC-12", tb}, {"BR-DEC-14", gt},
		{"BR-DEC-16", tp}, {"BR-DEC-17", ra}, {"BR-DEC-18", dp}} {
		k.decimals(c.id, c.n)
	}

	// BR-DEC-13 and BR-DEC-15 hold when one ram:TaxTotalAmount matches their
	// predicate, as in the official XPath.
	icc, tcc := text(k.icc), text(k.tcc)
	hasTax, ok13, ok15 := false, false, k.tcc == nil
	var bad13, bad15 *xmltree.Node
	for _, t := range sum.Children {
		if t.Name != "TaxTotalAmount" {
			continue
		}
		hasTax = true
		cur, has := t.LookupAttr("currencyID")
		inv := has && k.icc != nil && cur == icc
		if !inv || twoDecimals(t) {
			ok13 = true
		} else if bad13 == nil {
			bad13 = t
		}
		if k.tcc != nil && has && cur == tcc {
			if twoDecimals(t) {
				ok15 = true
			} else if bad15 == nil {
				bad15 = t
			}
		}
	}
	if hasTax && !ok13 {
		k.fail("BR-DEC-13", or(bad13, sum), "the invoice total VAT amount (BT-110) %q has more than 2 decimals", strings.TrimSpace(text(bad13)))
	}
	if hasTax && !ok15 {
		k.fail("BR-DEC-15", or(bad15, sum), "the invoice total VAT amount in accounting currency (BT-111) %q has more than 2 decimals", strings.TrimSpace(text(bad15)))
	}

	if k.p == mMin {
		k.grandTotal("FX-MIN-CO-15", sum, tb, gt)
		return
	}

	// BR-CO-11 and BR-CO-12: document level allowances and charges.
	allowances, charges := 0, 0
	var allowSum, chargeSum invoice.Decimal
	for _, ac := range kids(k.d.Settlement) {
		if ac.Name != "SpecifiedTradeAllowanceCharge" {
			continue
		}
		switch text(ac.Elem("ChargeIndicator").Elem("Indicator")) {
		case "false":
			allowances++
			allowSum = allowSum.Add(sumDec(ac, "ActualAmount"))
		case "true":
			charges++
			chargeSum = chargeSum.Add(sumDec(ac, "ActualAmount"))
		}
	}
	if allowances > 0 || at != nil {
		want := allowSum.RoundHalfCeil(2)
		if v, ok := rules.Dec(at); !ok || !v.Equal(want) {
			k.fail("BR-CO-11", or(at, sum), "the sum of allowances on document level (BT-107) is %s, expected %s (sum of the document level allowances)", valueOf(at), want)
		}
	}
	if charges > 0 || ct != nil {
		logistics := rules.SumDec(k.d.Settlement.FindAll("SpecifiedLogisticsServiceCharge/AppliedAmount"))
		want := chargeSum.RoundHalfCeil(2).Add(logistics.RoundHalfCeil(2)).RoundHalfCeil(2)
		if v, ok := rules.Dec(ct); !ok || !v.RoundHalfCeil(2).Equal(want) {
			k.fail("BR-CO-12", or(ct, sum), "the sum of charges on document level (BT-108) is %s, expected %s (sum of the document level charges)", valueOf(ct), want)
		}
	}

	// BR-CO-13: absent allowance and charge totals count as zero.
	if v, ok := rules.Dec(tb); ok {
		l, okL := rules.Dec(lt)
		a, okA := optDec(at)
		c, okC := optDec(ct)
		if want := l.Sub(a).Add(c).RoundHalfCeil(2); !okL || !okA || !okC || !v.Equal(want) {
			k.fail("BR-CO-13", tb, "the invoice total amount without VAT (BT-109) is %s, expected %s (BT-106 %s - BT-107 %s + BT-108 %s)", valueOf(tb), want, valueOf(lt), valueOrZero(at), valueOrZero(ct))
		}
	} else if tb == nil {
		k.fail("BR-CO-13", sum, "the invoice total amount without VAT (BT-109) is missing")
	}

	k.grandTotal("BR-CO-15", sum, tb, gt)

	// BR-CO-16: absent paid and rounding amounts count as zero.
	g, okG := rules.Dec(gt)
	p, okP := optDec(tp)
	r, okR := optDec(ra)
	due := g.Sub(p).Add(r)
	if v, ok := rules.Dec(dp); !ok || !okG || !okP || !okR || !v.Equal(due) {
		k.fail("BR-CO-16", or(dp, sum), "the amount due for payment (BT-115) is %s, expected %s (BT-112 %s - BT-113 %s + BT-114 %s)", valueOf(dp), due, valueOf(gt), valueOrZero(tp), valueOrZero(ra))
	}

	// BR-53: VAT accounting currency.
	if k.tcc != nil {
		found := false
		for _, t := range sum.Children {
			if t.Name == "TaxTotalAmount" {
				if cur, ok := t.LookupAttr("currencyID"); ok && cur == tcc {
					found = true
				}
			}
		}
		if !found || (k.icc != nil && tcc == icc) {
			k.fail("BR-53", k.tcc, "with the VAT accounting currency %q (BT-6), the total VAT amount in that currency (BT-111) must be given and BT-6 must differ from the invoice currency (BT-5)", tcc)
		}
	}

	// BR-CO-14: total VAT in invoice currency.
	var vatSum invoice.Decimal
	for _, tt := range kids(k.d.Settlement) {
		if tt.Name == "ApplicableTradeTax" {
			vatSum = vatSum.Add(sumDec(tt, "CalculatedAmount"))
		}
	}
	want := vatSum.RoundHalfCeil(2)
	for _, t := range sum.Children {
		if t.Name != "TaxTotalAmount" || k.icc == nil {
			continue
		}
		if cur, ok := t.LookupAttr("currencyID"); ok && cur == icc {
			if v, ok := rules.Dec(t); !ok || !v.Equal(want) {
				k.fail("BR-CO-14", t, "the invoice total VAT amount (BT-110) is %s, expected %s (sum of the VAT category tax amounts BT-117)", valueOf(t), want)
			}
		}
	}
}

// grandTotal implements BR-CO-15: for the invoice currency, a single total
// VAT amount and BT-112 = BT-109 + BT-110, or BT-112 = BT-109.
func (k *checker) grandTotal(id string, sum, tb, gt *xmltree.Node) {
	if k.icc == nil {
		return
	}
	icc := text(k.icc)
	var tax *xmltree.Node
	n := 0
	for _, t := range sum.Children {
		if t.Name == "TaxTotalAmount" {
			if cur, ok := t.LookupAttr("currencyID"); ok && cur == icc {
				tax = t
				n++
			}
		}
	}
	g, okG := rules.Dec(gt)
	b, okB := rules.Dec(tb)
	if okG && okB {
		if g.Equal(b) {
			return
		}
		if v, ok := rules.Dec(tax); ok && n == 1 && g.Equal(b.Add(v).RoundHalfCeil(2)) {
			return
		}
	}
	k.fail(id, or(gt, sum), "the invoice total amount with VAT (BT-112) is %s, expected BT-109 %s + BT-110 %s in %s", valueOf(gt), valueOf(tb), valueOrZero(tax), icc)
}

// breakdown implements the rules of the contexts
// ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax (BR-45 to BR-48, BR-CO-03, BR-CO-17, BR-DEC-19, BR-DEC-20, the category L,
// M and O rules) and their ram:CategoryCode (the category S, Z, E, AE, G
// and K rules).
func (k *checker) breakdown(w *walkState) {
	st := k.d.Settlement
	if st == nil {
		return
	}
	for _, tt := range st.Children {
		if tt.Name != "ApplicableTradeTax" {
			continue
		}
		basis := tt.Elem("BasisAmount")
		calc := tt.Elem("CalculatedAmount")
		rate := tt.Elem("RateApplicablePercent")
		cat := tt.Elem("CategoryCode")
		if basis == nil {
			k.fail("BR-45", tt, "the VAT category taxable amount (BT-116) is missing")
		}
		if calc == nil {
			k.fail("BR-46", tt, "the VAT category tax amount (BT-117) is missing")
		}
		if cat == nil {
			k.fail("BR-47", tt, "the VAT category code (BT-118) is missing")
		}
		if rate == nil && !hasCategory(tt, "O") {
			k.fail("BR-48", tt, "the VAT category rate (BT-119) is missing")
		}
		if tt.Elem("TaxPointDate") != nil && tt.Elem("DueDateTypeCode") != nil {
			k.fail("BR-CO-03", tt, "the VAT point date (BT-7) and the VAT point date code (BT-8) are both given")
		}
		k.brCO17(tt, basis, calc, rate)
		k.decimals("BR-DEC-19", basis)
		k.decimals("BR-DEC-20", calc)
		if n := count(tt, "CalculatedAmount"); n != 1 {
			k.fail("FX-SCH-A-000176", tt, "ram:CalculatedAmount must occur exactly once in ram:ApplicableTradeTax (found %d)", n)
		}
		if n := count(tt, "BasisAmount"); n != 1 {
			k.fail("FX-SCH-A-000177", tt, "ram:BasisAmount must occur exactly once in ram:ApplicableTradeTax (found %d)", n)
		}
		k.taxCodes(tt)

		for _, cc := range tt.Children {
			if cc.Name != "CategoryCode" {
				continue
			}
			switch cc.Text {
			case "S":
				k.brS09(tt, basis, calc, rate)
				k.noExemption("BR-S-10", tt, "S")
			case "Z":
				k.zeroTax("BR-Z-09", tt, calc, "Z")
				k.noExemption("BR-Z-10", tt, "Z")
			case "E":
				k.zeroTax("BR-E-09", tt, calc, "E")
				k.exemption("BR-E-10", tt, "E")
			case "AE":
				k.zeroTax("BR-AE-09", tt, calc, "AE")
				k.exemption("BR-AE-10", tt, "AE")
			case "G":
				if k.p == mMin {
					k.categoryBasis("BR-G-08", tt, basis, "G", true)
				}
				k.zeroTax("BR-G-09", tt, calc, "G")
				k.exemption("BR-G-10", tt, "G")
			case "K":
				if k.p == mMin {
					k.categoryBasis("BR-IC-08", tt, basis, "K", true)
				}
				k.zeroTax("BR-IC-09", tt, calc, "K")
				k.exemption("BR-IC-10", tt, "K")
				d := k.d
				if d.Delivery.Find("ActualDeliverySupplyChainEvent/OccurrenceDateTime/DateTimeString") == nil &&
					st.Find("BillingSpecifiedPeriod/StartDateTime") == nil && st.Find("BillingSpecifiedPeriod/EndDateTime") == nil {
					k.fail("BR-IC-11", tt, "an intra-community supply (category K) requires the actual delivery date (BT-72) or the invoicing period (BG-14)")
				}
				if d.Delivery.Find("ShipToTradeParty/PostalTradeAddress/CountryID") == nil {
					k.fail("BR-IC-12", tt, "an intra-community supply (category K) requires the deliver to country code (BT-80)")
				}
			}
		}
		if hasCategory(tt, "L") {
			k.noExemption("BR-AF-10", tt, "L")
		}
		if hasCategory(tt, "M") {
			k.noExemption("BR-AG-10", tt, "M")
		}
		if hasCategory(tt, "O") {
			if k.p == mMin {
				k.categoryBasis("BR-O-08", tt, basis, "O", false)
			}
			k.zeroTax("BR-O-09", tt, calc, "O")
			k.exemption("BR-O-10", tt, "O")
			if w.nonOTax {
				k.fail("BR-O-11", tt, "an invoice with a VAT breakdown of category O (not subject to VAT) cannot contain other VAT categories")
				k.fail("BR-O-12", tt, "an invoice with a VAT breakdown of category O (not subject to VAT) cannot contain lines of other VAT categories")
			}
			if w.nonOCatTax {
				k.fail("BR-O-13", tt, "an invoice with a VAT breakdown of category O (not subject to VAT) cannot contain allowances of other VAT categories")
				k.fail("BR-O-14", tt, "an invoice with a VAT breakdown of category O (not subject to VAT) cannot contain charges of other VAT categories")
			}
		}
	}
}

// brCO17 implements BR-CO-17 as written in the schematron: the rate is only
// used when the tax type is VAT, and a rate that rounds to 0 requires a tax
// amount that rounds to 0.
func (k *checker) brCO17(tt, basis, calc, rate *xmltree.Node) {
	c, okC := rules.Dec(calc)
	// upper-case(normalize-space(.)) = 'VAT': "VAT" has no inner space.
	isVAT := strings.EqualFold(strings.Trim(text(tt.Elem("TypeCode")), xmlSpace), "VAT")
	ok := false
	var want invoice.Decimal
	if okC {
		r, okR := rules.Dec(rate)
		switch {
		case !isVAT || rate == nil:
			ok = c.RoundHalfCeil(0).IsZero()
		case !okR:
		case r.RoundHalfCeil(0).IsZero():
			ok = c.RoundHalfCeil(0).IsZero()
		default:
			if b, okB := rules.Dec(basis); okB {
				want = b.Abs().Mul(r).Mul(hundredth).RoundHalfCeil(2)
				a := c.Abs()
				ok = a.Sub(one).Cmp(want) <= 0 && a.Add(one).Cmp(want) >= 0
			}
		}
	}
	if !ok {
		k.fail("BR-CO-17", or(calc, tt), "the VAT category tax amount (BT-117) %s does not match the taxable amount (BT-116) %s at rate (BT-119) %s%%", valueOf(calc), valueOf(basis), valueOf(rate))
	}
}

// brS09 implements BR-S-09: |BT-117| - 1 < round(|BT-116| × BT-119) / 100 < |BT-117| + 1.
func (k *checker) brS09(tt, basis, calc, rate *xmltree.Node) {
	b, okB := rules.Dec(basis)
	c, okC := rules.Dec(calc)
	r, okR := rules.Dec(rate)
	if okB && okC && okR {
		v := b.Abs().Mul(r).RoundHalfCeil(0).Mul(hundredth)
		a := c.Abs()
		if a.Sub(one).Cmp(v) < 0 && a.Add(one).Cmp(v) > 0 {
			return
		}
	}
	k.fail("BR-S-09", or(calc, tt), "standard rated VAT breakdown: the tax amount (BT-117) %s is not the taxable amount (BT-116) %s × rate (BT-119) %s%%", valueOf(calc), valueOf(basis), valueOf(rate))
}

// categoryBasis implements BR-G-08, BR-IC-08 (tolerance: BT-116 - 1 <= sum
// < BT-116 + 1) and BR-O-08 (exact): the taxable amount of the category is
// the sum of its line net amounts plus charges minus allowances, each sum
// rounded to 2 decimals. Only the first amount of an allowance or charge
// counts (ram:ActualAmount[1]).
func (k *checker) categoryBasis(id string, tt, basis *xmltree.Node, cat string, tolerance bool) {
	var lines invoice.Decimal
	for _, l := range k.d.Lines {
		lts := l.Elem("SpecifiedLineTradeSettlement")
		for _, t := range kids(lts) {
			if t.Name == "ApplicableTradeTax" && hasCategory(t, cat) {
				lines = lines.Add(rules.SumDec(lts.FindAll("SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount")))
				break
			}
		}
	}
	var allowances, charges invoice.Decimal
	for _, ac := range kids(k.d.Settlement) {
		if ac.Name != "SpecifiedTradeAllowanceCharge" || !hasCategory(ac.Elem("CategoryTradeTax"), cat) {
			continue
		}
		charge, ok := xsBoolean(text(ac.Elem("ChargeIndicator").Elem("Indicator")))
		amount, okA := rules.Dec(ac.Elem("ActualAmount"))
		switch {
		case !ok || !okA:
		case charge:
			charges = charges.Add(amount)
		default:
			allowances = allowances.Add(amount)
		}
	}
	want := lines.RoundHalfCeil(2).Add(charges.RoundHalfCeil(2)).Sub(allowances.RoundHalfCeil(2))
	b, ok := rules.Dec(basis)
	if ok && (tolerance && b.Sub(one).Cmp(want) <= 0 && b.Add(one).Cmp(want) > 0 || !tolerance && b.Equal(want)) {
		return
	}
	k.fail(id, or(basis, tt), "the taxable amount (BT-116) of category %s is %s, expected %s (lines plus charges minus allowances of the category)", cat, valueOf(basis), want)
}

// zeroTax requires a VAT category tax amount of 0.
func (k *checker) zeroTax(id string, tt, calc *xmltree.Node, cat string) {
	if v, ok := rules.Dec(calc); !ok || !v.IsZero() {
		k.fail(id, or(calc, tt), "the VAT category tax amount (BT-117) of category %s is %s, expected 0", cat, valueOf(calc))
	}
}

// exemption requires an exemption reason or reason code.
func (k *checker) exemption(id string, tt *xmltree.Node, cat string) {
	if tt.Elem("ExemptionReason") == nil && tt.Elem("ExemptionReasonCode") == nil {
		k.fail(id, tt, "the VAT breakdown of category %s requires an exemption reason (BT-120) or reason code (BT-121)", cat)
	}
}

// noExemption forbids an exemption reason or reason code.
func (k *checker) noExemption(id string, tt *xmltree.Node, cat string) {
	if tt.Elem("ExemptionReason") != nil || tt.Elem("ExemptionReasonCode") != nil {
		k.fail(id, tt, "the VAT breakdown of category %s cannot have an exemption reason (BT-120) or reason code (BT-121)", cat)
	}
}

// taxCodes checks the code values of a ram:ApplicableTradeTax or
// ram:CategoryTradeTax (FX-SCH-A-000023, 000179, 000180).
func (k *checker) taxCodes(tt *xmltree.Node) {
	for _, ch := range tt.Children {
		switch ch.Name {
		case "TypeCode":
			if ch.Text != "" && ch.Text != "VAT" {
				k.fail("FX-SCH-A-000023", ch, "the tax type code %q is not allowed: it must be VAT", ch.Text)
			}
		case "CategoryCode":
			if ch.Text != "" && !vatCategoryCodes()[ch.Text] {
				k.fail("FX-SCH-A-000179", ch, "the VAT category code %q is not allowed (expected AE, E, G, K, L, M, O, S or Z)", ch.Text)
			}
		case "DueDateTypeCode":
			if ch.Text != "" && !vatDateCodes[ch.Text] {
				k.fail("FX-SCH-A-000180", ch, "the VAT point date code (BT-8) %q is not allowed (expected 5, 29 or 72)", ch.Text)
			}
		}
	}
}

// period implements the rules of context
// ram:ApplicableHeaderTradeSettlement/ram:BillingSpecifiedPeriod (BR-29,
// BR-CO-19).
func (k *checker) period() {
	for _, p := range kids(k.d.Settlement) {
		if p.Name != "BillingSpecifiedPeriod" {
			continue
		}
		start, end := p.Elem("StartDateTime"), p.Elem("EndDateTime")
		if start == nil && end == nil {
			k.fail("BR-CO-19", p, "the invoicing period (BG-14) has neither a start date (BT-73) nor an end date (BT-74)")
		}
		if start == nil || end == nil {
			continue
		}
		// The schematron compares the format 102 strings.
		ok := false
		for _, e := range end.Children {
			for _, s := range start.Children {
				if e.Name == "DateTimeString" && s.Name == "DateTimeString" && e.Attr("format") == "102" && s.Attr("format") == "102" && e.Text >= s.Text {
					ok = true
				}
			}
		}
		if !ok {
			k.fail("BR-29", end, "the invoicing period end date (BT-74) %q is before its start date (BT-73) %q", strings.TrimSpace(text(end.Elem("DateTimeString"))), strings.TrimSpace(text(start.Elem("DateTimeString"))))
		}
	}
}

// vatCategory describes the VAT category rules of document level allowances
// and charges.
type vatCategory struct {
	code   string
	ids    [2][2]string // [allowance, charge][identifiers rule, rate rule]
	vatIDs idsRule
	need   string // the identifiers required, for messages
	rate   func(r invoice.Decimal, present, ok bool) bool
	want   string // the rate required, for messages
}

var vatCategories = [...]vatCategory{
	{"S", [2][2]string{{"BR-S-03", "BR-S-06"}, {"BR-S-04", "BR-S-07"}}, idsSellerTax, sellerTaxIDs, ratePositive, "greater than 0"},
	{"Z", [2][2]string{{"BR-Z-03", "BR-Z-06"}, {"BR-Z-04", "BR-Z-07"}}, idsSellerTax, sellerTaxIDs, rateZero, "0"},
	{"E", [2][2]string{{"BR-E-03", "BR-E-06"}, {"BR-E-04", "BR-E-07"}}, idsSellerTax, sellerTaxIDs, rateZero, "0"},
	{"AE", [2][2]string{{"BR-AE-03", "BR-AE-06"}, {"BR-AE-04", "BR-AE-07"}}, idsReverseCharge, "a seller VAT or tax identifier (BT-31, BT-32, BT-63) and the buyer VAT identifier (BT-48) or legal registration identifier (BT-47)", rateZero, "0"},
	{"G", [2][2]string{{"BR-G-03", "BR-G-06"}, {"BR-G-04", "BR-G-07"}}, idsSellerVAT, "the seller VAT identifier (BT-31) or tax representative VAT identifier (BT-63)", rateZero, "0"},
	{"K", [2][2]string{{"BR-IC-03", "BR-IC-06"}, {"BR-IC-04", "BR-IC-07"}}, idsIntraCommunity, "the seller (BT-31) or tax representative (BT-63) VAT identifier and the buyer VAT identifier (BT-48)", rateZero, "0"},
	{"L", [2][2]string{{"BR-AF-03", "BR-AF-06"}, {"BR-AF-04", "BR-AF-07"}}, idsSellerTax, sellerTaxIDs, ratePositive, "greater than 0"},
	{"M", [2][2]string{{"BR-AG-03", "BR-AG-06"}, {"BR-AG-04", "BR-AG-07"}}, idsSellerTax, sellerTaxIDs, rateNonNegative, "0 or greater"},
	{"O", [2][2]string{{"BR-O-03", "BR-O-06"}, {"BR-O-04", "BR-O-07"}}, idsNone, "no seller, tax representative or buyer VAT identifier (BT-31, BT-63, BT-48)", rateAbsent, "absent"},
}

// idsRule names the identifiers a VAT category requires. It is an enum
// rather than a method value so that the checker does not escape to the
// heap.
type idsRule uint8

const (
	idsSellerTax idsRule = iota
	idsSellerVAT
	idsReverseCharge
	idsIntraCommunity
	idsNone
)

// hasIDs reports whether the invoice has the identifiers rule requires.
func (k *checker) hasIDs(rule idsRule) bool {
	switch rule {
	case idsSellerTax:
		return k.sellerTaxID()
	case idsSellerVAT:
		return k.sellerVAT()
	case idsReverseCharge:
		return k.reverseChargeIDs()
	case idsIntraCommunity:
		return k.intraCommunityIDs()
	}
	return k.noVATIDs()
}

const sellerTaxIDs = "the seller VAT identifier (BT-31), tax registration identifier (BT-32) or tax representative VAT identifier (BT-63)"

func ratePositive(r invoice.Decimal, present, ok bool) bool    { return ok && r.Sign() > 0 }
func rateZero(r invoice.Decimal, present, ok bool) bool        { return ok && r.IsZero() }
func rateNonNegative(r invoice.Decimal, present, ok bool) bool { return ok && r.Sign() >= 0 }
func rateAbsent(r invoice.Decimal, present, ok bool) bool      { return !present }

func (k *checker) sellerTaxID() bool {
	return hasTaxID(k.seller, "VA", "FC") || hasTaxID(k.taxRep, "VA", "")
}

func (k *checker) sellerVAT() bool {
	return hasTaxID(k.seller, "VA", "") || hasTaxID(k.taxRep, "VA", "")
}

func (k *checker) reverseChargeIDs() bool {
	return k.sellerTaxID() && (hasTaxID(k.buyer, "VA", "") || k.buyer.Elem("SpecifiedLegalOrganization").Elem("ID") != nil)
}

func (k *checker) intraCommunityIDs() bool {
	return k.sellerVAT() && hasTaxID(k.buyer, "VA", "")
}

func (k *checker) noVATIDs() bool {
	return !hasTaxID(k.seller, "VA", "") && !hasTaxID(k.taxRep, "VA", "") && !hasTaxID(k.buyer, "VA", "")
}

// allowances implements the rules of the contexts
// //ram:SpecifiedTradeAllowanceCharge (CII-SR-463), its
// ram:ChargeIndicator (BR-31 to BR-38, BR-CO-21, BR-CO-22, BR-DEC-01 to
// BR-DEC-06) and its ram:CategoryTradeTax (the VAT category rules).
func (k *checker) allowances() {
	for _, ac := range kids(k.d.Settlement) {
		if ac.Name != "SpecifiedTradeAllowanceCharge" {
			continue
		}
		ci := ac.Elem("ChargeIndicator")
		if ci == nil {
			k.fail("CII-SR-463", ac, "the document level allowance or charge has no charge indicator")
			continue
		}
		kind := 0 // allowance
		ids := [...]string{"BR-31", "BR-32", "BR-33", "BR-CO-21", "BR-DEC-01", "BR-DEC-02"}
		what, amount, cat, reason := "allowance", "BT-92", "BT-95", "BT-97, BT-98"
		switch text(ci.Elem("Indicator")) {
		case "false":
		case "true":
			kind = 1
			ids = [...]string{"BR-36", "BR-37", "BR-38", "BR-CO-22", "BR-DEC-05", "BR-DEC-06"}
			what, amount, cat, reason = "charge", "BT-99", "BT-102", "BT-104, BT-105"
		default:
			continue // reported as not used
		}
		if ac.Elem("ActualAmount") == nil {
			k.fail(ids[0], ac, "the document level %s has no amount (%s)", what, amount)
		}
		vatCat := false
		for _, t := range ac.Children {
			if t.Name == "CategoryTradeTax" && strings.ToUpper(text(t.Elem("TypeCode"))) == "VAT" && t.Elem("CategoryCode") != nil {
				vatCat = true
			}
		}
		if !vatCat {
			k.fail(ids[1], ac, "the document level %s has no VAT category code (%s)", what, cat)
		}
		if ac.Elem("Reason") == nil && ac.Elem("ReasonCode") == nil {
			k.fail(ids[2], ac, "the document level %s has neither a reason nor a reason code (%s)", what, reason)
			k.fail(ids[3], ac, "the document level %s has neither a reason nor a reason code (%s)", what, reason)
		}
		k.decimals(ids[4], ac.Elem("ActualAmount"))
		k.decimals(ids[5], ac.Elem("BasisAmount"))
		for _, rc := range ac.Children {
			if rc.Name != "ReasonCode" || rc.Text == "" {
				continue
			}
			if kind == 0 && !allowanceReasonCodes()[rc.Text] {
				k.fail("FX-SCH-A-000186", rc, "the allowance reason code (BT-98) %q is not in the UNTDID 5189 subset of EN 16931", rc.Text)
			}
			if _, list := rc.LookupAttr("listID"); kind == 1 && !list && !codes.ChargeReason(rc.Text) {
				k.fail("FX-SCH-A-000376", rc, "the charge reason code (BT-105) %q is not in UNTDID 7161", rc.Text)
			}
		}
		for _, t := range ac.Children {
			if t.Name != "CategoryTradeTax" {
				continue
			}
			k.taxCodes(t)
			for i := range vatCategories {
				c := &vatCategories[i]
				if !hasCategory(t, c.code) {
					continue
				}
				if !k.hasIDs(c.vatIDs) {
					k.fail(c.ids[kind][0], t, "a document level %s with VAT category %s requires %s", what, c.code, c.need)
				}
				rn := t.Elem("RateApplicablePercent")
				r, ok := rules.Dec(rn)
				if !c.rate(r, rn != nil, ok) {
					k.fail(c.ids[kind][1], or(rn, t), "the VAT rate of a document level %s with VAT category %s must be %s (found %s)", what, c.code, c.want, valueOf(rn))
				}
			}
		}
	}
}

// lineCategories lists the VAT categories of the line rules of the MINIMUM
// schematron: the identifiers required (…-02) and a rate of 0 (…-05).
var lineCategories = [...]struct {
	code, ids, rate string
	vatIDs          idsRule
	need            string
}{
	{"E", "BR-E-02", "BR-E-05", idsSellerTax, sellerTaxIDs},
	{"G", "BR-G-02", "BR-G-05", idsSellerVAT, "the seller VAT identifier (BT-31) or tax representative VAT identifier (BT-63)"},
	{"K", "BR-IC-02", "BR-IC-05", idsIntraCommunity, "the seller (or tax representative) and the buyer VAT identifiers"},
}

// lines implements the line rules of the MINIMUM schematron (BR-E-02,
// BR-E-05, BR-G-02, BR-G-05, BR-IC-02, BR-IC-05), whose context cannot occur
// in a MINIMUM invoice that matches its schema.
func (k *checker) lines() {
	for _, l := range k.d.Lines {
		for _, t := range kids(l.Elem("SpecifiedLineTradeSettlement")) {
			if t.Name != "ApplicableTradeTax" {
				continue
			}
			for i := range lineCategories {
				c := &lineCategories[i]
				if !hasCategory(t, c.code) {
					continue
				}
				if !k.hasIDs(c.vatIDs) {
					k.fail(c.ids, t, "an invoice line with VAT category %s requires %s", c.code, c.need)
				}
				if r, ok := rules.Dec(t.Elem("RateApplicablePercent")); !ok || !r.IsZero() {
					k.fail(c.rate, t, "the VAT rate of an invoice line with VAT category %s must be 0", c.code)
				}
			}
		}
	}
}

// payment implements the rules of the contexts
// //ram:SpecifiedTradeSettlementPaymentMeans (BR-49, BR-50, BR-61,
// BR-CO-27), //ram:ApplicableTradeSettlementFinancialCard (BR-51) and
// ram:InvoiceReferencedDocument (BR-55).
func (k *checker) payment() {
	for _, pm := range kids(k.d.Settlement) {
		switch pm.Name {
		case "InvoiceReferencedDocument":
			if !filled(pm.Elem("IssuerAssignedID")) {
				k.fail("BR-55", pm, "the preceding invoice reference (BG-3) has no invoice number (BT-25)")
			}
			continue
		case "SpecifiedTradeSettlementPaymentMeans":
		default:
			continue
		}
		tc := pm.Elem("TypeCode")
		if tc == nil {
			k.fail("BR-49", pm, "the payment instruction (BG-16) has no payment means type code (BT-81)")
		}
		iban, prop := false, false
		for _, acc := range pm.Children {
			switch acc.Name {
			case "ApplicableTradeSettlementFinancialCard":
				if id := acc.Elem("ID"); normalizedLen(text(id)) > 10 {
					k.fail("BR-51", id, "the payment card primary account number (BT-87) has %d characters: at most 10 may be shown (the first 6 and the last 4 digits)", normalizedLen(text(id)))
				}
				continue
			case "PayeePartyCreditorFinancialAccount":
			default:
				continue
			}
			i, p := acc.Elem("IBANID") != nil, acc.Elem("ProprietaryID") != nil
			iban, prop = iban || i, prop || p
			if text(tc) != "30" && text(tc) != "58" {
				continue
			}
			if blank(acc.Elem("IBANID")) && blank(acc.Elem("ProprietaryID")) {
				k.fail("BR-50", acc, "a credit transfer (payment means %s) requires a payment account identifier (BT-84) that is not blank", text(tc))
			}
			if !i && !p {
				k.fail("BR-61", acc, "a credit transfer (payment means %s) requires the payment account identifier (BT-84)", text(tc))
			}
		}
		if code := strings.TrimSpace(text(tc)); (code == "30" || code == "58") && (iban == prop) {
			k.fail("BR-CO-27", pm, "a credit transfer (payment means %s) requires either an IBAN or a proprietary account identifier (BT-84), not both and not none", code)
		}
	}
}

// structure implements the structural assertions of the schematrons that
// the schemas of the profiles do not express.
func (k *checker) structure() {
	d := k.d
	if slo := k.buyer.Elem("SpecifiedLegalOrganization"); slo != nil {
		if n := count(slo, "ID"); n != 1 {
			k.fail("FX-SCH-A-000019", slo, "ram:ID must occur exactly once in the buyer ram:SpecifiedLegalOrganization (found %d)", n)
		}
	}
	type party struct {
		name string
		n    *xmltree.Node
	}
	parties := [...]party{{"seller", k.seller}, {"buyer", k.buyer}, {"seller tax representative", k.taxRep}, {"payee", k.payee}, {"deliver to party", k.shipTo}}
	for i, p := range parties {
		if p.n == nil {
			continue
		}
		if i < 4 {
			if n := count(p.n, "Name"); n != 1 {
				k.fail("FX-SCH-A-000030", p.n, "ram:Name must occur exactly once in the %s (found %d)", p.name, n)
			}
		}
		if i == 0 || (i < 3 && k.p == mBWL) {
			if n := count(p.n, "PostalTradeAddress"); n != 1 {
				k.fail("FX-SCH-A-000032", p.n, "ram:PostalTradeAddress must occur exactly once in the %s (found %d)", p.name, n)
			}
		}
		if i == 1 || i == 3 || i == 4 {
			if n := count(p.n, "ID"); n > 1 {
				k.fail("FX-SCH-A-000163", p.n, "ram:ID may occur at most once in the %s (found %d)", p.name, n)
			}
			if n := count(p.n, "GlobalID"); n > 1 {
				k.fail("FX-SCH-A-000164", p.n, "ram:GlobalID may occur at most once in the %s (found %d)", p.name, n)
			}
		}
		if i != 2 {
			for _, g := range p.n.Children {
				if g.Name == "GlobalID" {
					k.schemeID(g, p.name, "global identifier")
				}
			}
		}
		if i <= 1 {
			if uc := p.n.Elem("URIUniversalCommunication"); uc != nil {
				for _, id := range uc.Children {
					if id.Name == "URIID" {
						k.schemeID(id, p.name, "electronic address")
					}
				}
			}
		}
		if i == 1 || i == 2 {
			for _, r := range p.n.Children {
				if r.Name == "SpecifiedTaxRegistration" {
					for _, id := range r.Children {
						if id.Name == "ID" {
							k.schemeID(id, p.name, "tax registration identifier")
						}
					}
				}
			}
		}
	}
	if k.seller != nil {
		va, fc := 0, 0
		for _, r := range k.seller.Children {
			if r.Name != "SpecifiedTaxRegistration" {
				continue
			}
			switch schemeOf(r) {
			case "VA":
				va++
			case "FC":
				fc++
			}
		}
		if va > 1 {
			k.fail("FX-SCH-A-000033", k.seller, "the seller may have at most one VAT identifier (scheme VA), found %d", va)
		}
		if fc > 1 {
			k.fail("FX-SCH-A-000034", k.seller, "the seller may have at most one tax registration identifier (scheme FC), found %d", fc)
		}
	}
	if n := count(k.buyer, "SpecifiedTaxRegistration"); n > 1 {
		k.fail("FX-SCH-A-000166", k.buyer, "ram:SpecifiedTaxRegistration may occur at most once in the buyer (found %d)", n)
	}
	if k.taxRep != nil {
		if n := count(k.taxRep, "SpecifiedTaxRegistration"); n != 1 {
			k.fail("FX-SCH-A-000169", k.taxRep, "ram:SpecifiedTaxRegistration must occur exactly once in the seller tax representative (found %d)", n)
		}
	}
	if sum := d.Summation; sum != nil {
		inv, tax := 0, 0
		for _, t := range sum.Children {
			if t.Name != "TaxTotalAmount" {
				continue
			}
			cur, ok := t.LookupAttr("currencyID")
			if ok && k.icc != nil && cur == k.icc.Text {
				inv++
			}
			if ok && k.tcc != nil && cur == k.tcc.Text {
				tax++
			}
		}
		if inv > 1 {
			k.fail("FX-SCH-A-000042", sum, "the total VAT amount in the invoice currency (BT-110) may occur only once (found %d)", inv)
		}
		if tax > 1 {
			k.fail("FX-SCH-A-000192", sum, "the total VAT amount in the VAT accounting currency (BT-111) may occur only once (found %d)", tax)
		}
	}

	// Code values.
	if tc := d.Header.Elem("TypeCode"); tc != nil && tc.Text != "" && !invoiceTypeCodes()[tc.Text] {
		k.fail("FX-SCH-A-000023", tc, "the invoice type code (BT-3) %q is not allowed in profile %s", tc.Text, k.profileName())
	}
	k.dateFormat(d.Header.Elem("IssueDateTime"))
	if k.p == mBWL {
		k.dateFormat(d.Delivery.Find("ActualDeliverySupplyChainEvent/OccurrenceDateTime"))
		for _, ch := range kids(d.Settlement) {
			switch ch.Name {
			case "BillingSpecifiedPeriod":
				k.dateFormat(ch.Elem("StartDateTime"))
				k.dateFormat(ch.Elem("EndDateTime"))
			case "SpecifiedTradePaymentTerms":
				k.dateFormat(ch.Elem("DueDateDateTime"))
			case "InvoiceReferencedDocument":
				k.dateFormat(ch.Elem("FormattedIssueDateTime"))
			}
		}
	}
}

// schemeID implements FX-SCH-A-000037: the identifier has a schemeID.
func (k *checker) schemeID(id *xmltree.Node, party, what string) {
	if _, ok := id.LookupAttr("schemeID"); !ok {
		k.fail("FX-SCH-A-000037", id, "the %s %s %q has no schemeID attribute", party, what, strings.TrimSpace(id.Text))
	}
}

// dateFormat implements FX-SCH-A-000022 on the date strings of n.
func (k *checker) dateFormat(n *xmltree.Node) {
	if n == nil {
		return
	}
	for _, ch := range n.Children {
		if ch.Name == "DateTimeString" {
			if f := ch.Attr("format"); f != "" && f != "102" {
				k.fail("FX-SCH-A-000022", ch, "the date format code %q is not allowed: Factur-X dates use format 102 (CCYYMMDD)", f)
			}
		}
	}
}

// decimals implements the BR-DEC rules: string-length(substring-after(n,
// '.')) <= 2, on the untrimmed value as in the schematron.
func (k *checker) decimals(id string, n *xmltree.Node) {
	s := text(n)
	if i := strings.IndexByte(s, '.'); i >= 0 && utf8.RuneCountInString(s[i+1:]) > 2 {
		k.fail(id, n, "%s %q has more than 2 decimals", display(n), s)
	}
}

// twoDecimals reports whether the amount n has at most 2 significant
// decimals (. = round(. * 100) div 100).
func twoDecimals(n *xmltree.Node) bool {
	v, ok := rules.Dec(n)
	return ok && v.Equal(v.RoundHalfCeil(2))
}

// optDec parses an optional amount: absent counts as zero.
func optDec(n *xmltree.Node) (invoice.Decimal, bool) {
	if n == nil {
		return invoice.NewDecimal(0, 0), true
	}
	return rules.Dec(n)
}

// valueOf returns the trimmed value of n for messages, or "(missing)".
func valueOf(n *xmltree.Node) string {
	if n == nil {
		return "(missing)"
	}
	return strings.TrimSpace(n.Text)
}

// valueOrZero returns the trimmed value of n for messages, or "0".
func valueOrZero(n *xmltree.Node) string {
	if n == nil {
		return "0"
	}
	return strings.TrimSpace(n.Text)
}

// xsBoolean casts s to xs:boolean as XPath 2.0 does.
func xsBoolean(s string) (value, ok bool) {
	switch strings.TrimSpace(s) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

// hasCategory reports whether the tax element t has a ram:CategoryCode equal
// to code, as the predicate [ram:CategoryCode = 'code'].
func hasCategory(t *xmltree.Node, code string) bool {
	if t == nil {
		return false
	}
	for _, ch := range t.Children {
		if ch.Name == "CategoryCode" && ch.Text == code {
			return true
		}
	}
	return false
}

// hasTaxID reports whether party has a ram:SpecifiedTaxRegistration/ram:ID
// whose schemeID is s1 or, when not empty, s2.
func hasTaxID(party *xmltree.Node, s1, s2 string) bool {
	if party == nil {
		return false
	}
	for _, r := range party.Children {
		if r.Name != "SpecifiedTaxRegistration" {
			continue
		}
		for _, id := range r.Children {
			if id.Name != "ID" {
				continue
			}
			if s, ok := id.LookupAttr("schemeID"); ok && (s == s1 || s2 != "" && s == s2) {
				return true
			}
		}
	}
	return false
}

// schemeOf returns the schemeID of the ram:ID of a tax registration.
func schemeOf(r *xmltree.Node) string {
	for _, id := range r.Children {
		if id.Name == "ID" {
			if s, ok := id.LookupAttr("schemeID"); ok {
				return s
			}
		}
	}
	return ""
}

// hasGrandChild reports whether n has a child a with a child b.
func hasGrandChild(n *xmltree.Node, a, b string) bool {
	if n == nil {
		return false
	}
	for _, ch := range n.Children {
		if ch.Name == a && ch.Elem(b) != nil {
			return true
		}
	}
	return false
}

// kids returns the child elements of n, nil when n is nil.
func kids(n *xmltree.Node) []*xmltree.Node {
	if n == nil {
		return nil
	}
	return n.Children
}

// or returns a when it is not nil, b otherwise.
func or(a, b *xmltree.Node) *xmltree.Node {
	if a != nil {
		return a
	}
	return b
}

// xmlSpace holds the white space characters of XML, which normalize-space
// removes.
const xmlSpace = " \t\r\n"

// blank reports whether normalize-space(n) is the empty string: n is absent
// or holds only XML white space.
func blank(n *xmltree.Node) bool { return strings.Trim(text(n), xmlSpace) == "" }

// normalizedLen returns string-length(normalize-space(s)): the number of
// characters of s once XML white space is trimmed and each inner run of it
// is collapsed to one space.
func normalizedLen(s string) int {
	n, space := 0, false
	for _, r := range strings.Trim(s, xmlSpace) {
		if strings.ContainsRune(xmlSpace, r) {
			space = true
			continue
		}
		if space {
			n++
			space = false
		}
		n++
	}
	return n
}

// sumDec returns the sum of the decimal values of the children of n named
// name, as sum() in XPath, skipping the values that are not decimals.
func sumDec(n *xmltree.Node, name string) invoice.Decimal {
	s := invoice.NewDecimal(0, 0)
	for _, ch := range kids(n) {
		if ch.Name == name {
			if v, ok := rules.Dec(ch); ok {
				s = s.Add(v)
			}
		}
	}
	return s
}
