package en16931

import (
	"strings"
	"testing"
)

// The cases of the business rules of the EXTENDED-CTC-FR schematron: the
// rules it adds and the CEN rules whose test it changes. Each rule has an
// invoice that breaks it; the base invoice and the cases that want no
// finding are the valid ones. As for the syntax rules, the expected rules
// are the verdicts of the FNFE XSLT EXTENDED-CTC-FR-CII.xslt.

// Snippets of the EXTENDED-CTC-FR cases.
const (
	tLineSettlementEnd = "</ram:SpecifiedLineTradeSettlement>"
	tHeaderLineTotal   = "<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n"
	tTaxTotal          = `<ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>`
	tGrandTotal        = "<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>"
	tDue               = "<ram:DuePayableAmount>119.00</ram:DuePayableAmount>"
	tTaxBasisTotal     = "<ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>"
	tSellerName        = "<ram:Name>Seller</ram:Name>"
	tBuyerName         = "<ram:Name>Buyer</ram:Name>"
	tHeaderDelivery    = "<ram:ApplicableHeaderTradeDelivery></ram:ApplicableHeaderTradeDelivery>"
	tBilledQuantity    = `<ram:SpecifiedLineTradeDelivery><ram:BilledQuantity unitCode="C62">10</ram:BilledQuantity></ram:SpecifiedLineTradeDelivery>`
	tNetPriceAgreement = "<ram:SpecifiedLineTradeAgreement><ram:NetPriceProductTradePrice><ram:ChargeAmount>10.00</ram:ChargeAmount></ram:NetPriceProductTradePrice></ram:SpecifiedLineTradeAgreement>"
	tLineSummation     = "<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>100.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>"
	tBuyerVAT          = `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR12345678901</ram:ID></ram:SpecifiedTaxRegistration>`
	tDelivery          = `<ram:ApplicableHeaderTradeDelivery><ram:ShipToTradeParty><ram:Name>Site</ram:Name><ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress></ram:ShipToTradeParty><ram:ActualDeliverySupplyChainEvent><ram:OccurrenceDateTime><udt:DateTimeString format="102">20240110</udt:DateTimeString></ram:OccurrenceDateTime></ram:ActualDeliverySupplyChainEvent></ram:ApplicableHeaderTradeDelivery>`
)

// subLine returns an invoice line of VAT category S at 19 % with the net
// amount amount, the parent line parent and the subtype status ("" for
// none).
func subLine(id, parent, status, amount string) string {
	adl := "<ram:LineID>" + id + "</ram:LineID>"
	if parent != "" {
		adl += "<ram:ParentLineID>" + parent + "</ram:ParentLineID>"
	}
	if status != "" {
		adl += "<ram:LineStatusReasonCode>" + status + "</ram:LineStatusReasonCode>"
	}
	return "<ram:IncludedSupplyChainTradeLineItem>\n<ram:AssociatedDocumentLineDocument>" + adl + "</ram:AssociatedDocumentLineDocument>\n" +
		"<ram:SpecifiedTradeProduct><ram:Name>Widget</ram:Name></ram:SpecifiedTradeProduct>\n" + tNetPriceAgreement + "\n" + tBilledQuantity + "\n" +
		tLineSettlement + lineTax + "\n<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>" + amount +
		"</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>\n" + tLineSettlementEnd + "\n</ram:IncludedSupplyChainTradeLineItem>\n"
}

// vatCat describes a VAT category for the cases: its code, the identifier
// of its rules, a rate ("" for none), the tax amount of a taxable amount of
// 100.00 and whether its breakdown needs an exemption reason.
type vatCat struct {
	code, rule, rate, tax string
	reason                bool
}

var ctcfrVATCats = []vatCat{
	{"Z", "Z", "0", "0.00", false},
	{"E", "E", "0", "0.00", true},
	{"AE", "AE", "0", "0.00", true},
	{"K", "IC", "0", "0.00", true},
	{"G", "G", "0", "0.00", true},
	{"O", "O", "", "0.00", true},
	{"L", "AF", "7", "7.00", false},
	{"M", "AG", "4", "4.00", false},
}

// taxElem returns the tax element name of category c, with the exemption
// reason of its breakdown if reason is set.
func (c vatCat) taxElem(name string, reason bool) string {
	r, rate := "", ""
	if reason && c.reason {
		r = "<ram:ExemptionReason>Exempt</ram:ExemptionReason>"
	}
	if c.rate != "" {
		rate = "<ram:RateApplicablePercent>" + c.rate + "</ram:RateApplicablePercent>"
	}
	return "<ram:" + name + "><ram:TypeCode>VAT</ram:TypeCode>" + r + "<ram:CategoryCode>" + c.code + "</ram:CategoryCode>" + rate + "</ram:" + name + ">"
}

// breakdown returns a VAT breakdown of category c with the taxable amount
// basis and the tax amount tax.
func (c vatCat) breakdown(basis, tax string) string {
	reason := ""
	if c.reason {
		reason = "<ram:ExemptionReason>Exempt</ram:ExemptionReason>"
	}
	rate := ""
	if c.rate != "" {
		rate = "<ram:RateApplicablePercent>" + c.rate + "</ram:RateApplicablePercent>"
	}
	return "<ram:ApplicableTradeTax><ram:CalculatedAmount>" + tax + "</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode>" + reason +
		"<ram:BasisAmount>" + basis + "</ram:BasisAmount><ram:CategoryCode>" + c.code + "</ram:CategoryCode>" + rate + "</ram:ApplicableTradeTax>"
}

// parties returns the edits that give the base invoice the parties and the
// delivery that category c needs: a buyer VAT identifier for AE and K, a
// deliver to country and a delivery date for K.
func (c vatCat) parties() []string {
	switch c.code {
	case "AE":
		return []string{buyerAddr, buyerAddr + tBuyerVAT}
	case "K":
		return []string{buyerAddr, buyerAddr + tBuyerVAT, tHeaderDelivery, tDelivery}
	}
	return nil
}

// invoice returns the edits that turn the line and the breakdown of the
// base invoice into category c, with the breakdown taxable amount basis. The
// line has the exemption reason of the breakdown, as BR-FXEXT-*-08rev
// selects the lines by category, rate and exemption reason.
func (c vatCat) invoice(basis string) []string {
	grand := "10" + c.tax // 100.00 + tax, below 10.00
	return append(c.parties(),
		lineTax, c.taxElem("ApplicableTradeTax", true),
		breakdownS, c.breakdown(basis, c.tax),
		tTaxTotal, `<ram:TaxTotalAmount currencyID="EUR">`+c.tax+`</ram:TaxTotalAmount>`,
		tGrandTotal, "<ram:GrandTotalAmount>"+grand+"</ram:GrandTotalAmount>",
		tDue, "<ram:DuePayableAmount>"+grand+"</ram:DuePayableAmount>")
}

// headerCharge returns a document level charge of 0.00 of VAT category S
// with the fields extra after the amount, and the edit giving the summation
// a charge total.
func headerCharge(extra string) []string {
	return []string{
		tPaymentTerms, `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount>` + extra +
			`<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>` + tPaymentTerms,
		tHeaderLineTotal, tHeaderLineTotal + "<ram:ChargeTotalAmount>0.00</ram:ChargeTotalAmount>\n",
	}
}

// reasonCode returns the edits giving the line and the breakdown of an
// invoice of category E the exemption reason code code.
func reasonCode(code string) []string {
	c := "<ram:ExemptionReasonCode>" + code + "</ram:ExemptionReasonCode>"
	return []string{
		"<ram:ExemptionReason>Exempt</ram:ExemptionReason><ram:CategoryCode>E</ram:CategoryCode>", "<ram:ExemptionReason>Exempt</ram:ExemptionReason><ram:CategoryCode>E</ram:CategoryCode>" + c,
		"<ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode>", "<ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode>" + c,
	}
}

func ctcfrModelCases() []ruleCase {
	e := func(pairs ...string) func(t *testing.T) string {
		return func(t *testing.T) string { return ctcfr(t, pairs...) }
	}
	cat := func(c vatCat, pairs ...string) func(t *testing.T) string {
		return e(append(c.invoice("100.00"), pairs...)...)
	}
	lines := func(ls ...string) []string { return []string{tLine, strings.Join(ls, "")} }
	agr := func(s string) []string { return []string{tCTCFRAgreementEnd, s + tCTCFRAgreementEnd} }
	characteristic := func(s string) []string {
		return []string{tItemName, tItemName + "<ram:ApplicableProductCharacteristic>" + s + "</ram:ApplicableProductCharacteristic>"}
	}
	full := subLine("1", "", "", "100.00")
	cases := []ruleCase{
		// Lines: only plain and DETAIL lines need a quantity, a unit, a net
		// amount, a net price and a VAT category.
		{name: "BR-FREXT-BR-22 quantity", doc: e(tBilledQuantity, "<ram:SpecifiedLineTradeDelivery></ram:SpecifiedLineTradeDelivery>"), want: "BR-FREXT-BR-22 BR-FREXT-BR-23"},
		{name: "BR-FREXT-BR-23 unit", doc: e(` unitCode="C62">10<`, `>10<`), want: "BR-FREXT-BR-23"},
		{name: "BR-FREXT-BR-24 net amount", doc: e(tLineSummation, ""), want: "BR-FREXT-BR-24 BR-FREXT-CO-10 BR-FREXT-CO-13 BR-FXEXT-S-08 BR-FXEXT-S-08ini BR-FXEXT-S-08rev"},
		{name: "BR-FREXT-BR-26 net price", doc: e(tNetPriceAgreement, "<ram:SpecifiedLineTradeAgreement></ram:SpecifiedLineTradeAgreement>"), want: "BR-FREXT-BR-26"},
		{name: "BR-FREXT-CO-04 line VAT category", doc: e(lineTax, "<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>"),
			want: "BR-FREXT-CO-04 BR-FREXT-S-01 BR-FXEXT-S-08 BR-FXEXT-S-08ini BR-FXEXT-S-08rev"},
		{name: "DETAIL line", doc: e(lines(subLine("1", "", "DETAIL", "100.00"))...), want: ""},
		{name: "group and information lines", doc: e(lines(
			subLine("1", "", "GROUP", "100.00"),
			subLine("1.1", "1", "DETAIL", "60.00"),
			subLine("1.2", "1", "DETAIL", "40.00"),
			strings.Replace(subLine("2", "", "INFORMATION", "0.00"), lineTax, "<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>Z</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>", 1),
			strings.Replace(strings.Replace(subLine("3", "", "INFORMATION", "0.00"), tBilledQuantity, "<ram:SpecifiedLineTradeDelivery></ram:SpecifiedLineTradeDelivery>", 1), tNetPriceAgreement, "<ram:SpecifiedLineTradeAgreement></ram:SpecifiedLineTradeAgreement>", 1))...), want: ""},
		{name: "BR-FREXT-05 line subtype", doc: e(lines(full, subLine("2", "", "SUBTOTAL", "0.00"))...), want: "BR-FREXT-05"},
		{name: "BR-FREXT-06 parent without subtype", doc: e(lines(full, subLine("2", "1", "", "0.00"))...), want: "BR-FREXT-06"},
		{name: "BR-FREXT-08 group amount", doc: e(lines(
			subLine("1", "", "GROUP", "90.00"),
			subLine("1.1", "1", "DETAIL", "60.00"),
			subLine("1.2", "1", "DETAIL", "40.00"))...), want: "BR-FREXT-08"},
		{name: "BR-FREXT-11 unknown parent", doc: e(lines(full, subLine("2", "9", "INFORMATION", "0.00"))...), want: "BR-FREXT-11"},
		{name: "BR-FREXT-11 own parent", doc: e(lines(full, subLine("2", "2", "INFORMATION", "0.00"))...), want: "BR-FREXT-11"},
		{name: "BR-FREXT-12 group sub-line amounts", doc: e(lines(
			subLine("1", "", "GROUP", "100.00"),
			strings.Replace(subLine("1.1", "1", "GROUP", ""), "<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount></ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>\n", "", 1),
			subLine("1.2", "1", "DETAIL", "100.00"))...), want: "BR-FREXT-12"},

		// Totals: plain and DETAIL lines only, logistics service charges
		// as charges, a tolerance of 0.01 per amount summed.
		{name: "BR-FREXT-CO-10 within tolerance", doc: e(tHeaderLineTotal, "<ram:LineTotalAmount>100.01</ram:LineTotalAmount>\n"), want: ""},
		{name: "BR-FREXT-CO-10 line total", doc: e(tHeaderLineTotal, "<ram:LineTotalAmount>100.02</ram:LineTotalAmount>\n"), want: "BR-FREXT-CO-10"},
		{name: "BR-FREXT-CO-11 allowance total", doc: e(tHeaderLineTotal, tHeaderLineTotal+"<ram:AllowanceTotalAmount>1.00</ram:AllowanceTotalAmount>\n"), want: "BR-FREXT-CO-11"},
		{name: "BR-FREXT-CO-12 charge total", doc: e(tHeaderLineTotal, tHeaderLineTotal+"<ram:ChargeTotalAmount>1.00</ram:ChargeTotalAmount>\n"), want: "BR-FREXT-CO-12"},
		{name: "BR-FREXT-CO-12 missing charge total", doc: e(headerCharge("<ram:Reason>Fee</ram:Reason>")[:2]...), want: "BR-FREXT-CO-12"},
		{name: "logistics service charge", doc: e(
			tPaymentTerms, `<ram:SpecifiedLogisticsServiceCharge><ram:Description>Transport</ram:Description><ram:AppliedAmount>5.00</ram:AppliedAmount><ram:AppliedTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:AppliedTradeTax></ram:SpecifiedLogisticsServiceCharge>`+tPaymentTerms,
			tHeaderLineTotal, tHeaderLineTotal+"<ram:ChargeTotalAmount>5.00</ram:ChargeTotalAmount>\n",
			tTaxBasisTotal, "<ram:TaxBasisTotalAmount>105.00</ram:TaxBasisTotalAmount>",
			tTaxTotal, `<ram:TaxTotalAmount currencyID="EUR">19.95</ram:TaxTotalAmount>`,
			tGrandTotal, "<ram:GrandTotalAmount>124.95</ram:GrandTotalAmount>",
			tDue, "<ram:DuePayableAmount>124.95</ram:DuePayableAmount>",
			breakdownS, strings.NewReplacer("19.00", "19.95", "100.00", "105.00").Replace(breakdownS)), want: "CII-SR-396"},
		{name: "BR-FREXT-CO-13 total without VAT", doc: e(tTaxBasisTotal, "<ram:TaxBasisTotalAmount>100.02</ram:TaxBasisTotalAmount>", tGrandTotal, "<ram:GrandTotalAmount>119.02</ram:GrandTotalAmount>", tDue, "<ram:DuePayableAmount>119.02</ram:DuePayableAmount>"), want: "BR-FREXT-CO-13"},
		{name: "BR-FREXT-CO-15 total with VAT", doc: e(tGrandTotal, "<ram:GrandTotalAmount>119.02</ram:GrandTotalAmount>", tDue, "<ram:DuePayableAmount>119.02</ram:DuePayableAmount>"), want: "BR-FREXT-CO-15"},
		{name: "BR-FREXT-CO-15 no VAT total in the invoice currency", doc: e(tTaxTotal, `<ram:TaxTotalAmount currencyID="USD">19.00</ram:TaxTotalAmount>`), want: "BR-FREXT-CO-15"},
		{name: "BR-CO-25 payment terms", doc: e(tPaymentTerms, ""), want: "BR-CO-25"},
		{name: "BR-CO-25 payment terms description", doc: e(tPaymentTerms, "<ram:SpecifiedTradePaymentTerms><ram:Description>30 days</ram:Description></ram:SpecifiedTradePaymentTerms>"), want: ""},

		// Changed tests.
		{name: "BR-CO-14 VAT total", doc: e(tTaxTotal, `<ram:TaxTotalAmount currencyID="EUR">19.01</ram:TaxTotalAmount>`), want: "BR-CO-14"},
		{name: "BR-DEC-13 VAT total decimals", doc: e(tTaxTotal, `<ram:TaxTotalAmount currencyID="EUR">19.001</ram:TaxTotalAmount>`), want: "BR-CO-14 BR-DEC-13"},
		{name: "BR-DEC-15 VAT total in accounting currency decimals", doc: e(settlStart, settlStart+"<ram:TaxCurrencyCode>USD</ram:TaxCurrencyCode>\n", tTaxTotal, tTaxTotal+`<ram:TaxTotalAmount currencyID="USD">20.001</ram:TaxTotalAmount>`), want: "BR-DEC-15"},
		{name: "BR-CO-09 VAT identifier prefix", doc: e(">DE123456789<", ">XX123456789<"), want: "BR-CO-09"},

		// Allowances and charges.
		{name: "BR-FREXT-BR-38 charge reason", doc: e(headerCharge("")...), want: "BR-FREXT-BR-38"},
		{name: "BR-FREXT-BR-44 line charge reason", doc: e(tLineSettlement, tLineSettlement+`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount></ram:SpecifiedTradeAllowanceCharge>`), want: "BR-FREXT-BR-44"},

		// Item attributes.
		{name: "item attribute measure", doc: e(characteristic(`<ram:Description>Length</ram:Description><ram:ValueMeasure unitCode="MTR">2</ram:ValueMeasure>`)...), want: ""},
		{name: "BR-FREXT-54-1 item attribute name", doc: e(characteristic("<ram:Value>Red</ram:Value>")...), want: "BR-FREXT-54-1"},
		{name: "BR-FREXT-54-2 item attribute value and measure", doc: e(characteristic(`<ram:Description>Colour</ram:Description><ram:ValueMeasure unitCode="MTR">1</ram:ValueMeasure><ram:Value>Red</ram:Value>`)...), want: "BR-FREXT-54-2"},
		{name: "BR-FREXT-54-2 item attribute without value", doc: e(characteristic(`<ram:Description>Colour</ram:Description>`)...), want: "BR-FREXT-54-2"},
		{name: "BR-FREXT-54-2 measure without unit", doc: e(characteristic(`<ram:Description>Length</ram:Description><ram:ValueMeasure>2</ram:ValueMeasure>`)...), want: "BR-FREXT-54-2"},
		{name: "BR-FXEXT-04 item attribute code", doc: e(characteristic(`<ram:TypeCode>XYZ</ram:TypeCode><ram:Value>Red</ram:Value>`)...), want: "BR-FXEXT-04"},

		// Code lists.
		{name: "BR-FREXT-CL-10 scheme BY", doc: e(tSellerName, `<ram:GlobalID schemeID="BY">123</ram:GlobalID>`+tSellerName), want: ""},
		{name: "BR-FREXT-CL-10 identifier scheme", doc: e(tSellerName, `<ram:GlobalID schemeID="XX">123</ram:GlobalID>`+tSellerName), want: "BR-FREXT-CL-10"},
		{name: "BR-FREXT-CL-19-GP gross price allowance reason", doc: e("<ram:SpecifiedLineTradeAgreement>", "<ram:SpecifiedLineTradeAgreement><ram:GrossPriceProductTradePrice><ram:ChargeAmount>10.00</ram:ChargeAmount><ram:AppliedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount><ram:ReasonCode>1</ram:ReasonCode></ram:AppliedTradeAllowanceCharge></ram:GrossPriceProductTradePrice>"), want: "BR-FREXT-CL-19-GP"},
		{name: "BR-FREXT-CL-20 charge reason", doc: e(headerCharge("<ram:ReasonCode>XYZ</ram:ReasonCode>")...), want: "BR-FREXT-CL-20"},
		{name: "BR-FREXT-CL-20-TAX tax reason", doc: e(headerCharge(`<ram:ReasonCode listID="5153">VAT</ram:ReasonCode>`)...), want: "BR-FREXT-CL-20-TAX"},
		{name: "tax reason", doc: e(headerCharge(`<ram:ReasonCode listID="5153">AAA</ram:ReasonCode>`)...), want: ""},
		{name: "BR-FREXT-CL-20-LISTID reason list", doc: e(headerCharge(`<ram:ReasonCode listID="1234">AAA</ram:ReasonCode>`)...), want: "BR-FREXT-CL-20-LISTID"},
		{name: "BR-FREXT-CL-24 MIME code xml", doc: e(agr(`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject mimeCode="text/xml" filename="a.xml">QUJD</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`)...), want: ""},
		{name: "BR-FREXT-CL-24 MIME code", doc: e(agr(`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject mimeCode="application/zip" filename="a.zip">QUJD</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`)...), want: "BR-FREXT-CL-24"},
		{name: "BR-FREXT-CL-27 delivery terms", doc: e(agr("<ram:ApplicableTradeDeliveryTerms><ram:DeliveryTypeCode>XXX</ram:DeliveryTypeCode></ram:ApplicableTradeDeliveryTerms>")...), want: "BR-FREXT-CL-27"},
		{name: "BR-CL-22 exemption reason code in lower case", doc: cat(ctcfrVATCats[1], reasonCode("vatex-eu-132")...), want: ""},
		{name: "BR-CL-22 exemption reason code", doc: cat(ctcfrVATCats[1], reasonCode("VATEX-XX")...), want: "BR-CL-22"},

		// VAT breakdown: a category is in the breakdown if and only if a
		// plain or DETAIL line, an allowance or a charge has it.
		{name: "BR-FREXT-S-01 standard rated breakdown", doc: cat(ctcfrVATCats[0], ctcfrVATCats[0].breakdown("100.00", "0.00"), ctcfrVATCats[0].breakdown("100.00", "0.00")+strings.NewReplacer("19.00", "0.00", "100.00", "0.00").Replace(breakdownS)), want: "BR-FREXT-S-01"},
		{name: "BR-FXEXT-S-08 standard rated taxable amount", doc: e(breakdownS, strings.Replace(breakdownS, "100.00", "90.00", 1)), want: "BR-FXEXT-S-08 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S-09"},
		{name: "BR-FXEXT-S-08rev exemption reason", doc: e(breakdownS, strings.Replace(breakdownS, "<ram:BasisAmount>", "<ram:ExemptionReason>x</ram:ExemptionReason><ram:BasisAmount>", 1)), want: "BR-FXEXT-S-08rev"},
		{name: "BR-FXEXT-S-09 standard rated tax amount", doc: e(breakdownS, strings.Replace(breakdownS, "19.00", "19.02", 1), tTaxTotal, `<ram:TaxTotalAmount currencyID="EUR">19.02</ram:TaxTotalAmount>`, tGrandTotal, "<ram:GrandTotalAmount>119.02</ram:GrandTotalAmount>", tDue, "<ram:DuePayableAmount>119.02</ram:DuePayableAmount>"), want: "BR-FXEXT-S-09"},
		{name: "information line category", doc: e(lines(full, strings.Replace(subLine("2", "", "INFORMATION", "0.00"), lineTax, ctcfrVATCats[0].taxElem("ApplicableTradeTax", false), 1))...), want: ""},
	}
	for _, c := range ctcfrVATCats {
		cases = append(cases,
			ruleCase{name: "category " + c.code, doc: cat(c), want: ""},
			ruleCase{name: "BR-FXEXT-" + c.rule + "-08 taxable amount", doc: e(c.invoice("90.00")...),
				want: "BR-FXEXT-" + c.rule + "-08 BR-FXEXT-" + c.rule + "-08ini BR-FXEXT-" + c.rule + "-08rev"},
			ruleCase{name: "BR-FREXT-" + c.rule + "-01 breakdown without line", doc: e(append(c.parties(), breakdownS, breakdownS+c.breakdown("0.00", "0.00"))...),
				want: "BR-FREXT-" + c.rule + "-01"},
		)
	}
	// Categories L and M: the rules BR-AF and BR-AG.
	for _, c := range ctcfrVATCats[6:] {
		bad := "-1"
		want := "BR-" + c.rule + "-05"
		if c.code == "L" {
			bad = "0"
		}
		cases = append(cases,
			ruleCase{name: "BR-" + c.rule + "-02 seller VAT identifier", doc: cat(c, tSellerVAT, ""), want: "BR-CO-26 BR-" + c.rule + "-02"},
			ruleCase{name: want + " line rate", doc: cat(c, c.taxElem("ApplicableTradeTax", false), strings.Replace(c.taxElem("ApplicableTradeTax", false), ">"+c.rate+"<", ">"+bad+"<", 1)),
				want: want + " BR-FXEXT-" + c.rule + "-08 BR-FXEXT-" + c.rule + "-08ini BR-FXEXT-" + c.rule + "-08rev"},
			ruleCase{name: "BR-" + c.rule + "-03 BR-" + c.rule + "-06 allowance", doc: cat(c, tSellerVAT, "", tPaymentTerms,
				`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount><ram:Reason>Discount</ram:Reason>`+
					strings.Replace(c.taxElem("CategoryTradeTax", false), ">"+c.rate+"<", ">"+bad+"<", 1)+`</ram:SpecifiedTradeAllowanceCharge>`+tPaymentTerms,
				tHeaderLineTotal, tHeaderLineTotal+"<ram:AllowanceTotalAmount>0.00</ram:AllowanceTotalAmount>\n"),
				want: "BR-CO-26 BR-" + c.rule + "-02 BR-" + c.rule + "-03 BR-" + c.rule + "-06"},
			ruleCase{name: "BR-" + c.rule + "-04 BR-" + c.rule + "-07 charge", doc: cat(c, tSellerVAT, "", tPaymentTerms,
				`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount><ram:Reason>Fee</ram:Reason>`+
					strings.Replace(c.taxElem("CategoryTradeTax", false), ">"+c.rate+"<", ">"+bad+"<", 1)+`</ram:SpecifiedTradeAllowanceCharge>`+tPaymentTerms,
				tHeaderLineTotal, tHeaderLineTotal+"<ram:ChargeTotalAmount>0.00</ram:ChargeTotalAmount>\n"),
				want: "BR-CO-26 BR-" + c.rule + "-02 BR-" + c.rule + "-04 BR-" + c.rule + "-07"},
		)
	}
	return cases
}
