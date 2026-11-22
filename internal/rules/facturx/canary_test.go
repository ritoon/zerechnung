package facturx

import (
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// canaryBase returns the edits that turn basicWL into a valid invoice of the
// IGIC (category L) or IPSI (category M) at rate: the allowance of 100.00
// takes that category, a charge of 10.00 of that category is added, and the
// second VAT breakdown becomes that category on 410.00 (500.00 of lines,
// minus the allowance, plus the charge) with tax amount tax; the totals
// follow.
func canaryBase(cat, rate, tax, taxTotal, grandTotal, due string) []string {
	return []string{
		"<ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>20.00</ram:RateApplicablePercent></ram:CategoryTradeTax>",
		"<ram:CategoryCode>" + cat + "</ram:CategoryCode><ram:RateApplicablePercent>" + rate + "</ram:RateApplicablePercent></ram:CategoryTradeTax>",
		"</ram:SpecifiedTradeAllowanceCharge>",
		"</ram:SpecifiedTradeAllowanceCharge>\n<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount><ram:Reason>Freight</ram:Reason>" +
			"<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>" + cat + "</ram:CategoryCode><ram:RateApplicablePercent>" + rate + "</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>",
		secondTax,
		"<ram:ApplicableTradeTax>\n<ram:CalculatedAmount>" + tax + "</ram:CalculatedAmount>\n<ram:TypeCode>VAT</ram:TypeCode>\n<ram:BasisAmount>410.00</ram:BasisAmount>\n<ram:CategoryCode>" + cat + "</ram:CategoryCode>\n<ram:RateApplicablePercent>" + rate + "</ram:RateApplicablePercent>\n</ram:ApplicableTradeTax>",
		"<ram:LineTotalAmount>1000.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>1000.00</ram:LineTotalAmount>\n<ram:ChargeTotalAmount>10.00</ram:ChargeTotalAmount>",
		"<ram:TaxBasisTotalAmount>900.00", "<ram:TaxBasisTotalAmount>910.00",
		`<ram:TaxTotalAmount currencyID="EUR">122.00`, `<ram:TaxTotalAmount currencyID="EUR">` + taxTotal,
		"<ram:GrandTotalAmount>1022.00", "<ram:GrandTotalAmount>" + grandTotal,
		"<ram:DuePayableAmount>1000.00", "<ram:DuePayableAmount>" + due,
	}
}

// Seller VAT identifier of basicWL, which the IGIC and IPSI allowances and
// charges require, or a tax registration identifier (BR-AF-03, BR-AF-04,
// BR-AG-03, BR-AG-04).
const bwlSellerVAT = `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration>
</ram:SellerTradeParty>`

// canaryCases are the cases of the rules of the IGIC (BR-AF) and IPSI
// (BR-AG) of the BASIC WL schematron, edits of basicWL: the identifiers of
// the seller and the rates of the allowances and charges of these
// categories, and the exemption reason of their VAT breakdowns.
func canaryCases() []extCase {
	igic := canaryBase("L", "7.00", "28.70", "128.70", "1038.70", "1016.70")
	ipsi := canaryBase("M", "10.00", "41.00", "141.00", "1051.00", "1029.00")
	with := func(base []string, edits ...string) []string {
		return append(append([]string(nil), base...), edits...)
	}
	return []extCase{
		{"IGIC allowance and charge", igic, ""},
		{"BR-AF-03 BR-AF-04 IGIC seller without tax identifier", with(igic, bwlSellerVAT, "</ram:SellerTradeParty>"), "BR-AF-03 BR-AF-04"},
		{"BR-AF-06 BR-AF-07 IGIC zero rates", with(igic,
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>L</ram:CategoryCode><ram:RateApplicablePercent>7.00",
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>L</ram:CategoryCode><ram:RateApplicablePercent>0",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>L</ram:CategoryCode><ram:RateApplicablePercent>7.00",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>L</ram:CategoryCode><ram:RateApplicablePercent>0"),
			"BR-AF-06 BR-AF-07"},
		{"BR-AF-10 IGIC exemption reason", with(igic, "<ram:BasisAmount>410.00</ram:BasisAmount>", "<ram:ExemptionReason>Exempt</ram:ExemptionReason>\n<ram:BasisAmount>410.00</ram:BasisAmount>"), "BR-AF-10"},
		{"IPSI allowance and charge", ipsi, ""},
		{"BR-AG-03 BR-AG-04 IPSI seller without tax identifier", with(ipsi, bwlSellerVAT, "</ram:SellerTradeParty>"), "BR-AG-03 BR-AG-04"},
		{"IPSI zero rates", with(ipsi,
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>10.00",
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>0",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>10.00",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>0"),
			""},
		{"BR-AG-06 BR-AG-07 IPSI negative rates", with(ipsi,
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>10.00",
			"<ram:Reason>Discount</ram:Reason>\n<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>-1",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>10.00",
			"<ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>M</ram:CategoryCode><ram:RateApplicablePercent>-1"),
			"BR-AG-06 BR-AG-07"},
		{"BR-AG-10 IPSI exemption reason", with(ipsi, "<ram:BasisAmount>410.00</ram:BasisAmount>", "<ram:ExemptionReason>Exempt</ram:ExemptionReason>\n<ram:BasisAmount>410.00</ram:BasisAmount>"), "BR-AG-10"},
	}
}

// TestCanaryIslands runs canaryCases.
func TestCanaryIslands(t *testing.T) {
	for _, tc := range canaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, applyEdits(t, basicWL, tc.edits), invoice.ProfileUnknown)
			if got := ruleSet(r); got != tc.want {
				t.Errorf("rules = %q, want %q\n%s", got, tc.want, r)
			}
		})
	}
}
