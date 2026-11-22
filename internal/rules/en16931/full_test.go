package en16931

import "testing"

// tFull is a valid invoice with the optional elements that the cases of
// fullCases mutate: a line of category S with an allowance and a charge
// (BG-27, BG-28), a line of category E, document level allowance and charge
// with base amounts and reason codes, a VAT accounting currency (BT-6, BT-111),
// rounding and paid amounts, a payment means, an item identifier and country
// of origin, a deliver to location identifier and a VAT exemption reason
// code. It follows the element order of the CII schema.
const tFull = tHeader + `<ram:IncludedSupplyChainTradeLineItem>
<ram:AssociatedDocumentLineDocument><ram:LineID>1</ram:LineID></ram:AssociatedDocumentLineDocument>
<ram:SpecifiedTradeProduct><ram:GlobalID schemeID="0160">4012345000016</ram:GlobalID><ram:Name>Widget</ram:Name><ram:OriginTradeCountry><ram:ID>DE</ram:ID></ram:OriginTradeCountry></ram:SpecifiedTradeProduct>
<ram:SpecifiedLineTradeAgreement><ram:NetPriceProductTradePrice><ram:ChargeAmount>10.00</ram:ChargeAmount></ram:NetPriceProductTradePrice></ram:SpecifiedLineTradeAgreement>
<ram:SpecifiedLineTradeDelivery><ram:BilledQuantity unitCode="C62">10</ram:BilledQuantity></ram:SpecifiedLineTradeDelivery>
<ram:SpecifiedLineTradeSettlement>
<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>
<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:CalculationPercent>5</ram:CalculationPercent><ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>95</ram:ReasonCode><ram:Reason>Discount</ram:Reason></ram:SpecifiedTradeAllowanceCharge>
<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:CalculationPercent>5</ram:CalculationPercent><ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>ABL</ram:ReasonCode><ram:Reason>Packaging</ram:Reason></ram:SpecifiedTradeAllowanceCharge>
<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>100.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>
</ram:SpecifiedLineTradeSettlement>
</ram:IncludedSupplyChainTradeLineItem>
<ram:IncludedSupplyChainTradeLineItem>
<ram:AssociatedDocumentLineDocument><ram:LineID>2</ram:LineID></ram:AssociatedDocumentLineDocument>
<ram:SpecifiedTradeProduct><ram:Name>Training</ram:Name></ram:SpecifiedTradeProduct>
<ram:SpecifiedLineTradeAgreement><ram:NetPriceProductTradePrice><ram:ChargeAmount>50.00</ram:ChargeAmount></ram:NetPriceProductTradePrice></ram:SpecifiedLineTradeAgreement>
<ram:SpecifiedLineTradeDelivery><ram:BilledQuantity unitCode="C62">1</ram:BilledQuantity></ram:SpecifiedLineTradeDelivery>
<ram:SpecifiedLineTradeSettlement>
<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>E</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>
<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>50.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>
</ram:SpecifiedLineTradeSettlement>
</ram:IncludedSupplyChainTradeLineItem>
<ram:ApplicableHeaderTradeAgreement>
<ram:SellerTradeParty><ram:Name>Seller</ram:Name><ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTradeParty>
<ram:BuyerTradeParty><ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress></ram:BuyerTradeParty>
</ram:ApplicableHeaderTradeAgreement>
<ram:ApplicableHeaderTradeDelivery><ram:ShipToTradeParty><ram:GlobalID schemeID="0088">4000001000005</ram:GlobalID></ram:ShipToTradeParty></ram:ApplicableHeaderTradeDelivery>
<ram:ApplicableHeaderTradeSettlement>
<ram:TaxCurrencyCode>GBP</ram:TaxCurrencyCode>
<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
<ram:SpecifiedTradeSettlementPaymentMeans><ram:TypeCode>1</ram:TypeCode></ram:SpecifiedTradeSettlementPaymentMeans>
<ram:ApplicableTradeTax><ram:CalculatedAmount>18.05</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>95.00</ram:BasisAmount><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>
<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>50.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode><ram:ExemptionReasonCode>VATEX-EU-132</ram:ExemptionReasonCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>
<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:CalculationPercent>10</ram:CalculationPercent><ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>10.00</ram:ActualAmount><ram:ReasonCode>95</ram:ReasonCode><ram:Reason>Discount</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>
<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:CalculationPercent>5</ram:CalculationPercent><ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>FC</ram:ReasonCode><ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>
<ram:SpecifiedTradeSettlementHeaderMonetarySummation>
<ram:LineTotalAmount>150.00</ram:LineTotalAmount>
<ram:ChargeTotalAmount>5.00</ram:ChargeTotalAmount>
<ram:AllowanceTotalAmount>10.00</ram:AllowanceTotalAmount>
<ram:TaxBasisTotalAmount>145.00</ram:TaxBasisTotalAmount>
<ram:TaxTotalAmount currencyID="EUR">18.05</ram:TaxTotalAmount>
<ram:TaxTotalAmount currencyID="GBP">15.50</ram:TaxTotalAmount>
<ram:RoundingAmount>0.00</ram:RoundingAmount>
<ram:GrandTotalAmount>163.05</ram:GrandTotalAmount>
<ram:TotalPrepaidAmount>10.00</ram:TotalPrepaidAmount>
<ram:DuePayableAmount>153.05</ram:DuePayableAmount>
</ram:SpecifiedTradeSettlementHeaderMonetarySummation>
</ram:ApplicableHeaderTradeSettlement>
` + tFooter

// fullCases are the cases that mutate tFull, appended to ruleCases: the
// reasons of the line allowances and charges, which need the reason text and
// the reason code removed together (BR-42, BR-44, BR-CO-23, BR-CO-24), the
// number of decimals of the amounts that the base invoice lacks (BR-DEC), a
// third decimal of value 0 or, for BR-DEC-15, which compares values, of
// another value, and the code lists of its optional elements (BR-CL).
func fullCases() []ruleCase {
	e := func(pairs ...string) func(t *testing.T) string {
		return func(t *testing.T) string { return edit(t, tFull, pairs...) }
	}
	return []ruleCase{
		{name: "invoice with optional elements", doc: e(), want: ""},
		{name: "BR-42 BR-CO-23 line allowance reason", doc: e("<ram:ReasonCode>95</ram:ReasonCode><ram:Reason>Discount</ram:Reason></ram:SpecifiedTradeAllowanceCharge>", "</ram:SpecifiedTradeAllowanceCharge>"), want: "BR-42 BR-CO-23"},
		{name: "BR-44 BR-CO-24 line charge reason", doc: e("<ram:ReasonCode>ABL</ram:ReasonCode><ram:Reason>Packaging</ram:Reason>", ""), want: "BR-44 BR-CO-24"},
		{name: "BR-DEC-02 BR-DEC-05 BR-DEC-06 document level allowance and charge", doc: e(
			"<ram:CalculationPercent>10</ram:CalculationPercent><ram:BasisAmount>100.00</ram:BasisAmount>", "<ram:CalculationPercent>10</ram:CalculationPercent><ram:BasisAmount>100.000</ram:BasisAmount>",
			"<ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>FC</ram:ReasonCode>", "<ram:BasisAmount>100.000</ram:BasisAmount><ram:ActualAmount>5.000</ram:ActualAmount><ram:ReasonCode>FC</ram:ReasonCode>"),
			want: "BR-DEC-02 BR-DEC-05 BR-DEC-06"},
		{name: "BR-DEC-10 to BR-DEC-17 document totals", doc: e(
			"<ram:ChargeTotalAmount>5.00</ram:ChargeTotalAmount>", "<ram:ChargeTotalAmount>5.000</ram:ChargeTotalAmount>",
			"<ram:AllowanceTotalAmount>10.00</ram:AllowanceTotalAmount>", "<ram:AllowanceTotalAmount>10.000</ram:AllowanceTotalAmount>",
			`<ram:TaxTotalAmount currencyID="GBP">15.50</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="GBP">15.505</ram:TaxTotalAmount>`,
			"<ram:RoundingAmount>0.00</ram:RoundingAmount>", "<ram:RoundingAmount>0.000</ram:RoundingAmount>",
			"<ram:GrandTotalAmount>163.05</ram:GrandTotalAmount>", "<ram:GrandTotalAmount>163.050</ram:GrandTotalAmount>",
			"<ram:TotalPrepaidAmount>10.00</ram:TotalPrepaidAmount>", "<ram:TotalPrepaidAmount>10.000</ram:TotalPrepaidAmount>"),
			want: "BR-DEC-10 BR-DEC-11 BR-DEC-14 BR-DEC-15 BR-DEC-16 BR-DEC-17"},
		{name: "BR-DEC-19 BR-DEC-20 VAT breakdown", doc: e("<ram:CalculatedAmount>18.05</ram:CalculatedAmount>", "<ram:CalculatedAmount>18.050</ram:CalculatedAmount>", "<ram:BasisAmount>95.00</ram:BasisAmount>", "<ram:BasisAmount>95.000</ram:BasisAmount>"), want: "BR-DEC-19 BR-DEC-20"},
		{name: "BR-DEC-23 to BR-DEC-28 invoice line", doc: e(
			"<ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>95</ram:ReasonCode>", "<ram:BasisAmount>100.000</ram:BasisAmount><ram:ActualAmount>5.000</ram:ActualAmount><ram:ReasonCode>95</ram:ReasonCode>",
			"<ram:BasisAmount>100.00</ram:BasisAmount><ram:ActualAmount>5.00</ram:ActualAmount><ram:ReasonCode>ABL</ram:ReasonCode>", "<ram:BasisAmount>100.000</ram:BasisAmount><ram:ActualAmount>5.000</ram:ActualAmount><ram:ReasonCode>ABL</ram:ReasonCode>",
			"<ram:LineTotalAmount>100.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>100.000</ram:LineTotalAmount>"),
			want: "BR-DEC-23 BR-DEC-24 BR-DEC-25 BR-DEC-27 BR-DEC-28"},
		{name: "BR-CL-03 BR-CL-05 VAT accounting currency", doc: e("<ram:TaxCurrencyCode>GBP</ram:TaxCurrencyCode>", "<ram:TaxCurrencyCode>QQQ</ram:TaxCurrencyCode>", `currencyID="GBP"`, `currencyID="QQQ"`), want: "BR-CL-03 BR-CL-05"},
		{name: "BR-CL-15 country of origin", doc: e("<ram:OriginTradeCountry><ram:ID>DE</ram:ID>", "<ram:OriginTradeCountry><ram:ID>ZZ</ram:ID>"), want: "BR-CL-15"},
		{name: "BR-CL-16 payment means", doc: e("<ram:TypeCode>1</ram:TypeCode>", "<ram:TypeCode>99</ram:TypeCode>"), want: "BR-CL-16"},
		{name: "BR-CL-17 allowance VAT category", doc: e("<ram:Reason>Discount</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode>", "<ram:Reason>Discount</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>Q</ram:CategoryCode>"), want: "BR-CL-17 BR-S-08"},
		{name: "BR-CL-18 line VAT category", doc: e("<ram:CategoryCode>E</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>\n", "<ram:CategoryCode>Q</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>\n"), want: "BR-CL-18 BR-E-01 BR-E-08"},
		{name: "BR-CL-19 allowance reason code", doc: e("<ram:ReasonCode>95</ram:ReasonCode><ram:Reason>Discount</ram:Reason></ram:SpecifiedTradeAllowanceCharge>", "<ram:ReasonCode>1</ram:ReasonCode><ram:Reason>Discount</ram:Reason></ram:SpecifiedTradeAllowanceCharge>"), want: "BR-CL-19"},
		{name: "BR-CL-20 charge reason code", doc: e("<ram:ReasonCode>ABL</ram:ReasonCode>", "<ram:ReasonCode>XYZ</ram:ReasonCode>"), want: "BR-CL-20"},
		{name: "BR-CL-21 item identifier scheme", doc: e(`<ram:GlobalID schemeID="0160">`, `<ram:GlobalID schemeID="9999">`), want: "BR-CL-21"},
		{name: "BR-CL-22 exemption reason code", doc: e("VATEX-EU-132", "VATEX-EU-999"), want: "BR-CL-22"},
		{name: "BR-CL-26 deliver to location identifier scheme", doc: e(`<ram:GlobalID schemeID="0088">`, `<ram:GlobalID schemeID="9999">`), want: "BR-CL-26"},
	}
}
