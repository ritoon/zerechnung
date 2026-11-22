package facturx

import (
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// extendedBase returns EN16931_Einfach declared as an EXTENDED invoice.
func extendedBase(t testing.TB) string {
	en := readTestdata(t, "EN16931_Einfach.cii.xml")
	return strings.Replace(en, "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>", "<ram:ID>"+invoice.URNExtended+"</ram:ID>", 1)
}

// Fragments of EN16931_Einfach used by the EXTENDED cases.
const (
	extLine1     = "<ram:LineID>1</ram:LineID>"
	extLine2     = "<ram:LineID>2</ram:LineID>"
	extFirstLine = "<rsm:SupplyChainTradeTransaction>\n"
	extTerms     = "<ram:SpecifiedTradePaymentTerms>"
	extSumEnd    = "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>"
	extProduct   = "<ram:Name>Trennblätter A4</ram:Name>"
	extDelivery  = `<udt:DateTimeString format="102">20180305</udt:DateTimeString>
        </ram:OccurrenceDateTime>`
	extBreakdown19 = "<ram:BasisAmount>198.00</ram:BasisAmount>\n        <ram:CategoryCode>S</ram:CategoryCode>"
	extLineTax19   = "<ram:CategoryCode>S</ram:CategoryCode>\n          <ram:RateApplicablePercent>19.00</ram:RateApplicablePercent>"
	extLineTax7    = "<ram:CategoryCode>S</ram:CategoryCode>\n          <ram:RateApplicablePercent>7.00</ram:RateApplicablePercent>"
	extGroup       = `<ram:IncludedSupplyChainTradeLineItem><ram:AssociatedDocumentLineDocument><ram:LineID>G</ram:LineID><ram:LineStatusReasonCode>GROUP</ram:LineStatusReasonCode></ram:AssociatedDocumentLineDocument><ram:SpecifiedTradeProduct><ram:Name>Group</ram:Name></ram:SpecifiedTradeProduct><ram:SpecifiedLineTradeAgreement/><ram:SpecifiedLineTradeDelivery/><ram:SpecifiedLineTradeSettlement><ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>473.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation></ram:SpecifiedLineTradeSettlement></ram:IncludedSupplyChainTradeLineItem>
`
	extDetail    = "<ram:ParentLineID>G</ram:ParentLineID><ram:LineStatusReasonCode>DETAIL</ram:LineStatusReasonCode>"
	extLogistics = `<ram:SpecifiedLogisticsServiceCharge><ram:Description>Freight</ram:Description><ram:AppliedAmount>10.00</ram:AppliedAmount><ram:AppliedTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19.00</ram:RateApplicablePercent></ram:AppliedTradeTax></ram:SpecifiedLogisticsServiceCharge>`
)

type extCase struct {
	name  string
	edits []string // old, new pairs, each replaced once
	want  string   // sorted distinct rules
}

// grouped returns the edits that put both lines of EN16931_Einfach in a
// group G whose net amount is 473.00.
func grouped(more ...string) []string {
	return append([]string{extFirstLine, extFirstLine + extGroup, extLine1, extLine1 + extDetail, extLine2, extLine2 + extDetail}, more...)
}

var extCases = []extCase{
	{"valid", nil, ""},
	// Notes.
	{"BR-FXEXT-01 note without content", []string{"<ram:Content>Rechnung gemäß Bestellung vom 01.03.2018.</ram:Content>", "<ram:SubjectCode>AAI</ram:SubjectCode>"}, "BR-FXEXT-01"},
	{"BR-FXEXT-02 line note without content", []string{extLine1, extLine1 + "<ram:IncludedNote><ram:SubjectCode>AAI</ram:SubjectCode></ram:IncludedNote>"}, "BR-FXEXT-02"},
	{"line note with content code", []string{extLine1, extLine1 + "<ram:IncludedNote><ram:ContentCode>X</ram:ContentCode><ram:SubjectCode>AAI</ram:SubjectCode></ram:IncludedNote>"}, ""},
	// Parties, attributes, dates.
	{"BR-FXEXT-03 buyer tax number", []string{"</ram:PostalTradeAddress>\n      </ram:BuyerTradeParty>", `</ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">123/456</ram:ID></ram:SpecifiedTaxRegistration></ram:BuyerTradeParty>`}, "BR-FXEXT-03 FX-SCH-A-000703"},
	{"buyer VAT identifier", []string{"</ram:PostalTradeAddress>\n      </ram:BuyerTradeParty>", `</ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE987654321</ram:ID></ram:SpecifiedTaxRegistration></ram:BuyerTradeParty>`}, ""},
	{"item attribute", []string{extProduct, extProduct + "<ram:ApplicableProductCharacteristic><ram:Description>Colour</ram:Description><ram:Value>red</ram:Value></ram:ApplicableProductCharacteristic>"}, ""},
	{"item attribute with code and measure", []string{extProduct, extProduct + `<ram:ApplicableProductCharacteristic><ram:TypeCode>AAA</ram:TypeCode><ram:ValueMeasure unitCode="KGM">2</ram:ValueMeasure></ram:ApplicableProductCharacteristic>`}, ""},
	{"BR-FXEXT-BR-54-1 attribute without name", []string{extProduct, extProduct + "<ram:ApplicableProductCharacteristic><ram:Value>red</ram:Value></ram:ApplicableProductCharacteristic>"}, "BR-FXEXT-BR-54-1"},
	{"BR-FXEXT-BR-54-2 and BR-FXEXT-04", []string{extProduct, extProduct + `<ram:ApplicableProductCharacteristic><ram:TypeCode>XYZ</ram:TypeCode><ram:Value>2</ram:Value><ram:ValueMeasure unitCode="KGM">2</ram:ValueMeasure></ram:ApplicableProductCharacteristic>`}, "BR-FXEXT-04 BR-FXEXT-BR-54-2"},
	{"BR-FXEXT-BR-54-2 measure without unit", []string{extProduct, extProduct + `<ram:ApplicableProductCharacteristic><ram:Description>Weight</ram:Description><ram:ValueMeasure>2</ram:ValueMeasure></ram:ApplicableProductCharacteristic>`}, "BR-FXEXT-BR-54-2"},
	// The code list of the delivery date (BT-72) only holds format 102
	// (FX-SCH-A-000786): the XSLT checks format 205 anyway.
	{"format 205", []string{extDelivery, `<udt:DateTimeString format="205">201803051230</udt:DateTimeString></ram:OccurrenceDateTime>`}, "FX-SCH-A-000786"},
	// The official pattern has no seconds, whatever its message says.
	{"BR-FXEXT-CII-DT-097a seconds", []string{extDelivery, `<udt:DateTimeString format="205">20180305123000</udt:DateTimeString></ram:OccurrenceDateTime>`}, "BR-FXEXT-CII-DT-097a FX-SCH-A-000786"},
	// Lines.
	{"BR-FXEXT-BR-23 unit", []string{`<ram:BilledQuantity unitCode="H87">20.0000</ram:BilledQuantity>`, "<ram:BilledQuantity>20.0000</ram:BilledQuantity>"}, "BR-FXEXT-BR-23"},
	{"BR-FXEXT-BR-22 quantity", []string{`<ram:BilledQuantity unitCode="H87">20.0000</ram:BilledQuantity>`, ""}, "BR-FXEXT-BR-22 BR-FXEXT-BR-23"},
	{"BR-FXEXT-BR-26 net price", []string{"<ram:NetPriceProductTradePrice>\n          <ram:ChargeAmount>9.9000</ram:ChargeAmount>\n        </ram:NetPriceProductTradePrice>", ""}, "BR-FXEXT-BR-26"},
	{"BR-FXEXT-CO-04 line category", []string{extLineTax19, "<ram:RateApplicablePercent>19.00</ram:RateApplicablePercent>"}, "BR-FXEXT-CO-04 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b"},
	{"BR-FXEXT-BR-24 line net amount", []string{"<ram:LineTotalAmount>198.00</ram:LineTotalAmount>", ""}, "BR-FXEXT-BR-24 BR-FXEXT-CO-10 BR-FXEXT-CO-13 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b"},
	{"information line not checked", []string{extLine1, extLine1 + "<ram:LineStatusReasonCode>INFORMATION</ram:LineStatusReasonCode>", `<ram:BilledQuantity unitCode="H87">20.0000</ram:BilledQuantity>`, "",
		"<ram:LineTotalAmount>473.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>275.00</ram:LineTotalAmount>", "<ram:TaxBasisTotalAmount>473.00", "<ram:TaxBasisTotalAmount>275.00"},
		"BR-FXEXT-CO-15 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b"},
	{"BR-FXEXT-BR-44 line charge reason", []string{"<ram:SpecifiedLineTradeSettlement>", "<ram:SpecifiedLineTradeSettlement><ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount></ram:SpecifiedTradeAllowanceCharge>"}, "BR-FXEXT-BR-44"},
	// Groups.
	{"group", grouped(), ""},
	{"BR-FXEXT-08 group amount", grouped("<ram:LineTotalAmount>473.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation></ram:SpecifiedLineTradeSettlement></ram:IncludedSupplyChainTradeLineItem>",
		"<ram:LineTotalAmount>474.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation></ram:SpecifiedLineTradeSettlement></ram:IncludedSupplyChainTradeLineItem>"), "BR-FXEXT-08"},
	{"BR-FXEXT-11 unknown parent", grouped(extLine2+"<ram:ParentLineID>G</ram:ParentLineID>", extLine2+"<ram:ParentLineID>H</ram:ParentLineID>"), "BR-FXEXT-08 BR-FXEXT-11"},
	{"BR-FXEXT-06 parent without subtype", grouped(extLine2+"<ram:ParentLineID>G</ram:ParentLineID><ram:LineStatusReasonCode>DETAIL</ram:LineStatusReasonCode>", extLine2+"<ram:ParentLineID>G</ram:ParentLineID>"), "BR-FXEXT-06 BR-FXEXT-08"},
	{"BR-FXEXT-12 sub-group without amount", grouped(extLine2+"<ram:ParentLineID>G</ram:ParentLineID><ram:LineStatusReasonCode>DETAIL</ram:LineStatusReasonCode>", extLine2+"<ram:ParentLineID>G</ram:ParentLineID><ram:LineStatusReasonCode>GROUP</ram:LineStatusReasonCode>",
		"<ram:LineTotalAmount>275.00</ram:LineTotalAmount>", ""),
		"BR-FXEXT-08 BR-FXEXT-12 BR-FXEXT-CO-10 BR-FXEXT-CO-13 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b"},
	// Document level allowances and charges.
	{"BR-FXEXT-BR-38 charge reason", []string{extTerms, `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19.00</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>` + extTerms}, "BR-FXEXT-BR-38"},
	{"logistics service charge not in totals", []string{extTerms, extLogistics + extTerms}, "BR-FXEXT-CO-12 BR-FXEXT-CO-13 BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b"},
	{"logistics service charge", []string{extTerms, extLogistics + extTerms,
		"<ram:CalculatedAmount>37.62</ram:CalculatedAmount>", "<ram:CalculatedAmount>39.52</ram:CalculatedAmount>",
		extBreakdown19, "<ram:BasisAmount>208.00</ram:BasisAmount>\n        <ram:CategoryCode>S</ram:CategoryCode>",
		"<ram:ChargeTotalAmount>0.00", "<ram:ChargeTotalAmount>10.00", "<ram:TaxBasisTotalAmount>473.00", "<ram:TaxBasisTotalAmount>483.00",
		`currencyID="EUR">56.87`, `currencyID="EUR">58.77`, "<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>541.77",
		"<ram:DuePayableAmount>529.87", "<ram:DuePayableAmount>541.77"}, ""},
	// Totals.
	{"BR-FXEXT-CO-10 within tolerance", []string{"<ram:LineTotalAmount>473.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>473.02</ram:LineTotalAmount>"}, ""},
	{"BR-FXEXT-CO-10", []string{"<ram:LineTotalAmount>473.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>473.03</ram:LineTotalAmount>"}, "BR-FXEXT-CO-10"},
	{"BR-FXEXT-CO-11", []string{"<ram:AllowanceTotalAmount>0.00", "<ram:AllowanceTotalAmount>0.01"}, "BR-FXEXT-CO-11"},
	{"BR-FXEXT-CO-15 no VAT total", []string{`<ram:TaxTotalAmount currencyID="EUR">56.87</ram:TaxTotalAmount>`, ""}, "BR-FXEXT-CO-15"},
	{"BR-FXEXT-CO-15 within tolerance, BR-FXEXT-CO-16 exact", []string{"<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>529.88"}, "BR-FXEXT-CO-16"},
	{"BR-FXEXT-CO-16 third party charge", []string{extSumEnd, extSumEnd + "<ram:SpecifiedFinancialAdjustment><ram:Reason>Stamp duty</ram:Reason><ram:ActualAmount>10.00</ram:ActualAmount></ram:SpecifiedFinancialAdjustment>"}, "BR-FXEXT-CO-16"},
	{"third party charge", []string{extSumEnd, extSumEnd + "<ram:SpecifiedFinancialAdjustment><ram:Reason>Stamp duty</ram:Reason><ram:ActualAmount>10.00</ram:ActualAmount></ram:SpecifiedFinancialAdjustment>",
		"<ram:DuePayableAmount>529.87", "<ram:DuePayableAmount>539.87"}, ""},
	// VAT breakdown.
	{"BR-FXEXT-Z-01 unused breakdown", []string{extTerms, "<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>0.00</ram:BasisAmount><ram:CategoryCode>Z</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>" + extTerms}, "BR-FXEXT-Z-01"},
	{"BR-FXEXT-Z-01 missing breakdown", []string{extLineTax7, "<ram:CategoryCode>Z</ram:CategoryCode>\n          <ram:RateApplicablePercent>7.00</ram:RateApplicablePercent>"}, "BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S08b BR-FXEXT-Z-01"},
	// A VAT breakdown that no line, allowance or charge uses: its taxable
	// amount must be 0 (BR-FXEXT-*-08, nothing summed, no tolerance).
	{"BR-FXEXT-Z-08 unused breakdown with an amount", extUnused("Z", "0", "1.00"), "BR-FXEXT-Z-01 BR-FXEXT-Z-08b BR-FXEXT-Z-08ini BR-FXEXT-Z-08rev"},
	{"BR-FXEXT-E-01 BR-FXEXT-E-08 unused breakdown", extUnused("E", "0", "1.00"), "BR-FXEXT-E-01 BR-FXEXT-E-08b BR-FXEXT-E-08ini BR-FXEXT-E-08rev"},
	{"BR-FXEXT-AE-01 BR-FXEXT-AE-08 unused breakdown", extUnused("AE", "0", "1.00"), "BR-FXEXT-AE-01 BR-FXEXT-AE-08b BR-FXEXT-AE-08ini BR-FXEXT-AE-08rev"},
	{"BR-FXEXT-IC-01 unused breakdown", extUnused("K", "0", "0.00"), "BR-FXEXT-IC-01"},
	{"BR-FXEXT-G-01 unused breakdown", extUnused("G", "0", "0.00"), "BR-FXEXT-G-01"},
	{"BR-FXEXT-O-01 BR-FXEXT-O-08 unused breakdown", extUnused("O", "", "1.00"), "BR-FXEXT-O-01 BR-FXEXT-O-08b BR-FXEXT-O-08ini BR-FXEXT-O-08rev"},
	{"BR-FXEXT-AF-01 BR-FXEXT-AF-08 unused IGIC breakdown", extUnused("L", "7.00", "1.00"), "BR-FXEXT-AF-01 BR-FXEXT-AF-08b BR-FXEXT-AF-08ini BR-FXEXT-AF-08rev"},
	{"BR-FXEXT-AG-01 BR-FXEXT-AG-08 unused IPSI breakdown", extUnused("M", "10.00", "1.00"), "BR-FXEXT-AG-01 BR-FXEXT-AG-08b BR-FXEXT-AG-08ini BR-FXEXT-AG-08rev"},
	{"BR-FXEXT-S-01 lines without breakdown", []string{extBreakdowns, "", `currencyID="EUR">56.87`, `currencyID="EUR">0.00`,
		"<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>473.00", "<ram:DuePayableAmount>529.87", "<ram:DuePayableAmount>473.00"}, "BR-FXEXT-S-01"},
	{"BR-FXEXT-S-09b within tolerance", []string{"<ram:CalculatedAmount>37.62</ram:CalculatedAmount>", "<ram:CalculatedAmount>37.63</ram:CalculatedAmount>",
		`currencyID="EUR">56.87`, `currencyID="EUR">56.88`, "<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>529.88", "<ram:DuePayableAmount>529.87", "<ram:DuePayableAmount>529.88"}, ""},
	{"BR-FXEXT-S-09b", []string{"<ram:CalculatedAmount>37.62</ram:CalculatedAmount>", "<ram:CalculatedAmount>37.64</ram:CalculatedAmount>",
		`currencyID="EUR">56.87`, `currencyID="EUR">56.89`, "<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>529.89", "<ram:DuePayableAmount>529.87", "<ram:DuePayableAmount>529.89"}, "BR-FXEXT-S-09b"},
	// Only the warning grouping by exemption reason fails: BR-FXEXT-S08b
	// accepts either grouping.
	{"BR-FXEXT-S-08rev exemption reason", []string{extBreakdown19, extBreakdown19 + "<ram:ExemptionReason>x</ram:ExemptionReason>"}, "BR-FXEXT-S-08rev"},
	{"BR-FXEXT-S08b missing rate", []string{extBreakdown19 + "\n        <ram:RateApplicablePercent>19.00</ram:RateApplicablePercent>", extBreakdown19}, "BR-FXEXT-S-08ini BR-FXEXT-S-08rev BR-FXEXT-S-09b BR-FXEXT-S08b"},
}

// extBreakdowns are the two VAT breakdowns of EN16931_Einfach.
const extBreakdowns = `      <ram:ApplicableTradeTax>
        <ram:CalculatedAmount>19.25</ram:CalculatedAmount>
        <ram:TypeCode>VAT</ram:TypeCode>
        <ram:BasisAmount>275.00</ram:BasisAmount>
        <ram:CategoryCode>S</ram:CategoryCode>
        <ram:RateApplicablePercent>7.00</ram:RateApplicablePercent>
      </ram:ApplicableTradeTax>
      <ram:ApplicableTradeTax>
        <ram:CalculatedAmount>37.62</ram:CalculatedAmount>
        <ram:TypeCode>VAT</ram:TypeCode>
        <ram:BasisAmount>198.00</ram:BasisAmount>
        <ram:CategoryCode>S</ram:CategoryCode>
        <ram:RateApplicablePercent>19.00</ram:RateApplicablePercent>
      </ram:ApplicableTradeTax>
`

// extUnused returns the edits that add to EN16931_Einfach a VAT breakdown of
// category cat at rate ("" for none) with taxable amount basis and no tax,
// which no line, allowance or charge uses.
func extUnused(cat, rate, basis string) []string {
	r := ""
	if rate != "" {
		r = "<ram:RateApplicablePercent>" + rate + "</ram:RateApplicablePercent>"
	}
	return []string{extTerms, "<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>" + basis +
		"</ram:BasisAmount><ram:CategoryCode>" + cat + "</ram:CategoryCode>" + r + "</ram:ApplicableTradeTax>" + extTerms}
}

// applyEdits replaces each old string of edits once in s.
func applyEdits(t testing.TB, s string, edits []string) string {
	t.Helper()
	for i := 0; i+1 < len(edits); i += 2 {
		if !strings.Contains(s, edits[i]) {
			t.Fatalf("%q not found", edits[i])
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	return s
}

func TestExtended(t *testing.T) {
	base := extendedBase(t)
	sev := map[string]rules.Severity{}
	for _, ri := range Catalog {
		sev[ri.ID] = ri.Severity
	}
	for _, tc := range extCases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, applyEdits(t, base, tc.edits), invoice.ProfileUnknown)
			if got := ruleSet(r); got != tc.want {
				t.Errorf("rules = %q, want %q\n%s", got, tc.want, r)
			}
			for _, f := range r.Findings {
				if f.Path == "" || f.Line == 0 || f.Message == "" {
					t.Errorf("incomplete finding %+v", f)
				}
				if s, ok := sev[f.Rule]; !ok || s != f.Severity {
					t.Errorf("finding %s (%s) does not match Catalog", f.Rule, f.Severity)
				}
			}
		})
	}
}

// TestExtendedProfiles checks that the BR-FXEXT rules apply to EXTENDED
// only: EXTENDED-CTC-FR gets the rules of the FNFE schematron, which package
// en16931 checks.
func TestExtendedProfiles(t *testing.T) {
	bad := applyEdits(t, extendedBase(t), []string{"<ram:GrandTotalAmount>529.87", "<ram:GrandTotalAmount>529.88"})
	for _, tc := range []struct {
		p    invoice.Profile
		want string
	}{
		{invoice.ProfileUnknown, "BR-FXEXT-CO-16"},
		{invoice.ProfileExtended, "BR-FXEXT-CO-16"},
		{invoice.ProfileExtendedCTCFR, "FX-SCH-A-000026"},
		{invoice.ProfileEN16931, "FX-SCH-A-000026"},
	} {
		if got := ruleSet(run(t, bad, tc.p)); got != tc.want {
			t.Errorf("%s: rules = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestDateTime205(t *testing.T) {
	for s, ok := range map[string]bool{"202401151230": true, " 202401152359\n": true, "202401152400": false, "20240115123000": false, "202413151230": false, "2024011512": false} {
		if dateTime205.MatchString(s) != ok {
			t.Errorf("dateTime205(%q) = %t", s, !ok)
		}
	}
}
