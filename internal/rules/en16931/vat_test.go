package en16931

import (
	"strings"
	"testing"
)

// The VAT category cases mutate an invoice of a single VAT category (catDoc):
// one line of 100.00, one document level allowance of 10.00 and one document
// level charge of 5.00 of that category, its VAT breakdown on a taxable
// amount of 95.00, and the parties the category requires. Unlike zeroRated,
// the documents follow the element order of the CII schema, so that the
// parity job compares them with the KoSIT validator on the business rules.

// catSpec is what a VAT category puts in catDoc.
type catSpec struct {
	rate      string // VAT rate (BT-152, BT-96, BT-103, BT-119), "" for none
	tax       string // VAT category tax amount (BT-117) on 95.00
	total     string // invoice total amount with VAT (BT-112)
	exemption string // VAT exemption reason text (BT-120) of the breakdown
}

var catSpecs = map[string]catSpec{
	"S":  {rate: "19", tax: "18.05", total: "113.05"},
	"Z":  {rate: "0", tax: "0.00", total: "95.00"},
	"E":  {rate: "0", tax: "0.00", total: "95.00", exemption: "Exempt"},
	"AE": {rate: "0", tax: "0.00", total: "95.00", exemption: "Reverse charge"},
	"K":  {rate: "0", tax: "0.00", total: "95.00", exemption: "Intra-community supply"},
	"G":  {rate: "0", tax: "0.00", total: "95.00", exemption: "Export outside the EU"},
	"O":  {tax: "0.00", total: "95.00", exemption: "Not subject to VAT"},
	"L":  {rate: "7", tax: "6.65", total: "101.65"},
	"M":  {rate: "10", tax: "9.50", total: "104.50"},
}

// Parties of catDoc.
const (
	// sellerLegal gives the seller a legal registration identifier (BT-30),
	// which satisfies BR-CO-26 without a VAT identifier.
	sellerLegal = `<ram:Name>Seller</ram:Name><ram:SpecifiedLegalOrganization><ram:ID>HRB 12345</ram:ID></ram:SpecifiedLegalOrganization>`
	sellerEnd   = `</ram:PostalTradeAddress></ram:SellerTradeParty>`
	buyerFR     = `<ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>`
	buyerVAT    = `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR12345678901</ram:ID></ram:SpecifiedTaxRegistration>`
	noDelivery  = `<ram:ApplicableHeaderTradeDelivery></ram:ApplicableHeaderTradeDelivery>`
	// deliveryFR gives the deliver to country (BT-80) and the actual
	// delivery date (BT-72) that category K requires (BR-IC-11, BR-IC-12).
	deliveryFR = `<ram:ApplicableHeaderTradeDelivery><ram:ShipToTradeParty><ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress></ram:ShipToTradeParty><ram:ActualDeliverySupplyChainEvent><ram:OccurrenceDateTime><udt:DateTimeString format="102">20240110</udt:DateTimeString></ram:OccurrenceDateTime></ram:ActualDeliverySupplyChainEvent></ram:ApplicableHeaderTradeDelivery>`
)

// catTax returns the VAT elements of a line, an allowance or a charge of
// category cat at rate rate ("" for none).
func catTax(cat, rate string) string {
	s := `<ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>` + cat + `</ram:CategoryCode>`
	if rate != "" {
		s += `<ram:RateApplicablePercent>` + rate + `</ram:RateApplicablePercent>`
	}
	return s
}

// catLine, catAllowance and catCharge return the VAT category of the line,
// the document level allowance and the document level charge of catDoc,
// each with what sets it apart in the document.
func catLine(cat, rate string) string {
	return `<ram:ApplicableTradeTax>` + catTax(cat, rate) + `</ram:ApplicableTradeTax>`
}

func catAllowance(cat, rate string) string {
	return `<ram:Reason>Discount</ram:Reason><ram:CategoryTradeTax>` + catTax(cat, rate) + `</ram:CategoryTradeTax>`
}

func catCharge(cat, rate string) string {
	return `<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax>` + catTax(cat, rate) + `</ram:CategoryTradeTax>`
}

// catBreakdown returns the VAT breakdown of catDoc.
func catBreakdown(cat string) string {
	c := catSpecs[cat]
	s := `<ram:ApplicableTradeTax><ram:CalculatedAmount>` + c.tax + `</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode>`
	if c.exemption != "" {
		s += `<ram:ExemptionReason>` + c.exemption + `</ram:ExemptionReason>`
	}
	s += `<ram:BasisAmount>95.00</ram:BasisAmount><ram:CategoryCode>` + cat + `</ram:CategoryCode>`
	if c.rate != "" {
		s += `<ram:RateApplicablePercent>` + c.rate + `</ram:RateApplicablePercent>`
	}
	return s + `</ram:ApplicableTradeTax>`
}

// catDoc returns the valid invoice of VAT category cat.
func catDoc(t *testing.T, cat string) string {
	t.Helper()
	c := catSpecs[cat]
	d := edit(t, tBase,
		lineTax, catLine(cat, c.rate),
		breakdownS, catBreakdown(cat)+"\n"+
			`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount>`+catAllowance(cat, c.rate)+`</ram:SpecifiedTradeAllowanceCharge>`+"\n"+
			`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>5.00</ram:ActualAmount>`+catCharge(cat, c.rate)+`</ram:SpecifiedTradeAllowanceCharge>`,
		"<ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">19.00</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>\n<ram:DuePayableAmount>119.00</ram:DuePayableAmount>",
		"<ram:ChargeTotalAmount>5.00</ram:ChargeTotalAmount>\n<ram:AllowanceTotalAmount>10.00</ram:AllowanceTotalAmount>\n<ram:TaxBasisTotalAmount>95.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">"+c.tax+"</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>"+c.total+"</ram:GrandTotalAmount>\n<ram:DuePayableAmount>"+c.total+"</ram:DuePayableAmount>")
	switch cat {
	case "AE":
		d = edit(t, d, buyerAddr, buyerFR+buyerVAT)
	case "K":
		d = edit(t, d, buyerAddr, buyerFR+buyerVAT, noDelivery, deliveryFR)
	case "O":
		d = edit(t, d, tSellerVAT, "", "<ram:Name>Seller</ram:Name>", sellerLegal)
	}
	return d
}

// splitPayment returns the base invoice between Italian parties, with VAT
// category B (split payment) instead of S.
func splitPayment(t *testing.T) string {
	t.Helper()
	d := strings.ReplaceAll(tBase, "<ram:CountryID>DE</ram:CountryID>", "<ram:CountryID>IT</ram:CountryID>")
	return edit(t, d, ">DE123456789<", ">IT12345678901<",
		lineTax, catLine("B", "19"),
		breakdownS, strings.Replace(breakdownS, ">S<", ">B<", 1))
}

// vatCases are the cases of the VAT category rules, appended to ruleCases:
// for each category, the valid invoice of catDoc, then the invoices that
// break the presence of the breakdown (BR-x-01), the VAT identifiers of the
// parties (BR-x-02 to BR-x-04), the rates (BR-x-05 to BR-x-07), the taxable
// amount (BR-x-08), the tax amount (BR-x-09) and the exemption reason
// (BR-x-10) of the breakdown, as far as the category has these rules. The
// CII artefacts name the rules of category L (IGIC) BR-IP and those of
// category M (IPSI) BR-IG; the case names say IGIC and IPSI, which
// conformance/parity/known.txt matches for the KoSIT validator, whose CEN
// 1.3.16 names them BR-AF and BR-AG.
func vatCases() []ruleCase {
	// doc returns catDoc(cat) with the replacements of pairs.
	doc := func(cat string, pairs ...string) func(t *testing.T) string {
		return func(t *testing.T) string { return edit(t, catDoc(t, cat), pairs...) }
	}
	// duplicate repeats the VAT breakdown of cat.
	duplicate := func(cat string) func(t *testing.T) string {
		return doc(cat, catBreakdown(cat), catBreakdown(cat)+"\n"+catBreakdown(cat))
	}
	// withoutBreakdown removes the VAT breakdown of cat.
	withoutBreakdown := func(cat string) func(t *testing.T) string {
		return doc(cat, catBreakdown(cat)+"\n", "")
	}
	// sellerWithoutVAT replaces the seller VAT identifier with a legal
	// registration identifier.
	sellerWithoutVAT := func(cat string) func(t *testing.T) string {
		return doc(cat, tSellerVAT, "", "<ram:Name>Seller</ram:Name>", sellerLegal)
	}
	// rates sets rate on the line, the allowance and the charge of cat.
	rates := func(cat, rate string) func(t *testing.T) string {
		r := catSpecs[cat].rate
		return doc(cat,
			catLine(cat, r), catLine(cat, rate),
			catAllowance(cat, r), catAllowance(cat, rate),
			catCharge(cat, r), catCharge(cat, rate))
	}
	// basis lowers the taxable amount of the breakdown beyond the tolerance.
	basis := func(cat string) func(t *testing.T) string {
		return doc(cat, "<ram:BasisAmount>95.00</ram:BasisAmount>", "<ram:BasisAmount>90.00</ram:BasisAmount>")
	}
	// tax gives the breakdown of a category without VAT a tax amount of
	// 0.40, which BR-CO-17 accepts (rounded to 0), and updates the totals.
	tax := func(cat string) func(t *testing.T) string {
		return doc(cat,
			"<ram:CalculatedAmount>0.00</ram:CalculatedAmount>", "<ram:CalculatedAmount>0.40</ram:CalculatedAmount>",
			`<ram:TaxTotalAmount currencyID="EUR">0.00</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="EUR">0.40</ram:TaxTotalAmount>`,
			"<ram:GrandTotalAmount>95.00</ram:GrandTotalAmount>", "<ram:GrandTotalAmount>95.40</ram:GrandTotalAmount>",
			"<ram:DuePayableAmount>95.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>95.40</ram:DuePayableAmount>")
	}
	// withoutExemption removes the exemption reason of the breakdown.
	withoutExemption := func(cat string) func(t *testing.T) string {
		return doc(cat, "<ram:ExemptionReason>"+catSpecs[cat].exemption+"</ram:ExemptionReason>", "")
	}
	valid := func(cat string) func(t *testing.T) string {
		return func(t *testing.T) string { return catDoc(t, cat) }
	}
	return []ruleCase{
		{name: "VAT category S with allowance and charge", doc: valid("S"), want: ""},
		{name: "BR-S-02 to BR-S-04 seller without VAT identifier", doc: sellerWithoutVAT("S"), want: "BR-S-02 BR-S-03 BR-S-04"},
		{name: "VAT category E with allowance and charge", doc: valid("E"), want: ""},
		{name: "BR-E-02 to BR-E-04 seller without VAT identifier", doc: sellerWithoutVAT("E"), want: "BR-E-02 BR-E-03 BR-E-04"},

		{name: "VAT category Z with allowance and charge", doc: valid("Z"), want: ""},
		{name: "BR-Z-01 two breakdowns", doc: duplicate("Z"), want: "BR-Z-01"},
		{name: "BR-Z-02 to BR-Z-04 seller without VAT identifier", doc: sellerWithoutVAT("Z"), want: "BR-Z-02 BR-Z-03 BR-Z-04"},
		{name: "BR-Z-05 to BR-Z-07 rates", doc: rates("Z", "10"), want: "BR-Z-05 BR-Z-06 BR-Z-07"},
		{name: "BR-Z-08 taxable amount", doc: basis("Z"), want: "BR-Z-08"},
		{name: "BR-Z-09 tax amount", doc: tax("Z"), want: "BR-Z-09"},

		{name: "VAT category AE with allowance and charge", doc: valid("AE"), want: ""},
		{name: "BR-AE-01 two breakdowns", doc: duplicate("AE"), want: "BR-AE-01"},
		{name: "BR-AE-02 to BR-AE-04 buyer without VAT identifier", doc: doc("AE", buyerVAT, ""), want: "BR-AE-02 BR-AE-03 BR-AE-04"},
		{name: "BR-AE-05 to BR-AE-07 rates", doc: rates("AE", "19"), want: "BR-AE-05 BR-AE-06 BR-AE-07"},
		{name: "BR-AE-08 taxable amount", doc: basis("AE"), want: "BR-AE-08"},
		{name: "BR-AE-09 tax amount", doc: tax("AE"), want: "BR-AE-09"},
		{name: "BR-AE-10 no exemption reason", doc: withoutExemption("AE"), want: "BR-AE-10"},

		{name: "VAT category K with allowance and charge", doc: valid("K"), want: ""},
		{name: "BR-IC-01 two breakdowns", doc: duplicate("K"), want: "BR-IC-01"},
		{name: "BR-IC-02 to BR-IC-04 buyer without VAT identifier", doc: doc("K", buyerVAT, ""), want: "BR-IC-02 BR-IC-03 BR-IC-04"},
		{name: "BR-IC-05 to BR-IC-07 rates", doc: rates("K", "19"), want: "BR-IC-05 BR-IC-06 BR-IC-07"},
		{name: "BR-IC-09 tax amount", doc: tax("K"), want: "BR-IC-09"},

		{name: "VAT category G with allowance and charge", doc: valid("G"), want: ""},
		{name: "BR-G-01 two breakdowns", doc: duplicate("G"), want: "BR-G-01"},
		{name: "BR-G-02 to BR-G-04 seller without VAT identifier", doc: sellerWithoutVAT("G"), want: "BR-G-02 BR-G-03 BR-G-04"},
		{name: "BR-G-05 to BR-G-07 rates", doc: rates("G", "19"), want: "BR-G-05 BR-G-06 BR-G-07"},

		{name: "VAT category O with allowance and charge", doc: valid("O"), want: ""},
		{name: "BR-O-02 to BR-O-04 seller VAT identifier", doc: doc("O", sellerEnd, "</ram:PostalTradeAddress>"+tSellerVAT+"</ram:SellerTradeParty>"), want: "BR-O-02 BR-O-03 BR-O-04"},
		{name: "BR-O-05 to BR-O-07 rates", doc: rates("O", "0"), want: "BR-O-05 BR-O-06 BR-O-07"},
		{name: "BR-O-08 taxable amount", doc: basis("O"), want: "BR-O-08"},
		{name: "BR-O-09 tax amount", doc: tax("O"), want: "BR-O-09"},
		{name: "BR-O-10 no exemption reason", doc: withoutExemption("O"), want: "BR-O-10"},

		{name: "VAT category L IGIC with allowance and charge", doc: valid("L"), want: ""},
		{name: "BR-IP-01 IGIC without breakdown", doc: withoutBreakdown("L"), want: "BR-CO-14 BR-CO-18 BR-IP-01"},
		{name: "BR-IP-02 to BR-IP-04 IGIC seller without VAT identifier", doc: sellerWithoutVAT("L"), want: "BR-IP-02 BR-IP-03 BR-IP-04"},
		{name: "BR-IP-05 to BR-IP-07 IGIC zero rates", doc: rates("L", "0"), want: "BR-IP-05 BR-IP-06 BR-IP-07"},
		{name: "BR-IP-10 IGIC exemption reason", doc: doc("L", "<ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>", "<ram:TypeCode>VAT</ram:TypeCode><ram:ExemptionReason>Exempt</ram:ExemptionReason><ram:BasisAmount>"), want: "BR-IP-10"},

		{name: "VAT category M IPSI with allowance and charge", doc: valid("M"), want: ""},
		{name: "BR-IG-01 IPSI without breakdown", doc: withoutBreakdown("M"), want: "BR-CO-14 BR-CO-18 BR-IG-01"},
		{name: "BR-IG-02 to BR-IG-04 IPSI seller without VAT identifier", doc: sellerWithoutVAT("M"), want: "BR-IG-02 BR-IG-03 BR-IG-04"},
		{name: "BR-IG-05 to BR-IG-07 IPSI negative rates", doc: rates("M", "-1"), want: "BR-IG-05 BR-IG-06 BR-IG-07"},

		{name: "VAT category B between Italian parties", doc: splitPayment, want: ""},
		{name: "BR-B-02 split payment and standard rate", doc: func(t *testing.T) string {
			return edit(t, splitPayment(t),
				"</ram:IncludedSupplyChainTradeLineItem>\n", "</ram:IncludedSupplyChainTradeLineItem>\n"+strings.Replace(tLine, "<ram:LineID>1</ram:LineID>", "<ram:LineID>2</ram:LineID>", 1),
				"</ram:ApplicableTradeTax>\n"+tPaymentTerms, "</ram:ApplicableTradeTax>\n"+breakdownS+"\n"+tPaymentTerms,
				"<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">19.00</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>\n<ram:DuePayableAmount>119.00</ram:DuePayableAmount>",
				"<ram:LineTotalAmount>200.00</ram:LineTotalAmount>\n<ram:TaxBasisTotalAmount>200.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">38.00</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>238.00</ram:GrandTotalAmount>\n<ram:DuePayableAmount>238.00</ram:DuePayableAmount>")
		}, want: "BR-B-02"},
	}
}
