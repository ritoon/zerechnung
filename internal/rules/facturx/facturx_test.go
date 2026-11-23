package facturx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const header = `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
`

// minimum is a valid MINIMUM invoice.
const minimum = header + `<rsm:ExchangedDocumentContext>
<ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>urn:factur-x.eu:1p0:minimum</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter>
</rsm:ExchangedDocumentContext>
<rsm:ExchangedDocument>
<ram:ID>F-2024-001</ram:ID>
<ram:TypeCode>380</ram:TypeCode>
<ram:IssueDateTime><udt:DateTimeString format="102">20240115</udt:DateTimeString></ram:IssueDateTime>
</rsm:ExchangedDocument>
<rsm:SupplyChainTradeTransaction>
<ram:ApplicableHeaderTradeAgreement>
<ram:BuyerReference>REF-42</ram:BuyerReference>
<ram:SellerTradeParty>
<ram:Name>Seller SARL</ram:Name>
<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration>
</ram:SellerTradeParty>
<ram:BuyerTradeParty>
<ram:Name>Buyer SAS</ram:Name>
</ram:BuyerTradeParty>
</ram:ApplicableHeaderTradeAgreement>
<ram:ApplicableHeaderTradeDelivery/>
<ram:ApplicableHeaderTradeSettlement>
<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
<ram:SpecifiedTradeSettlementHeaderMonetarySummation>
<ram:TaxBasisTotalAmount>500.00</ram:TaxBasisTotalAmount>
<ram:TaxTotalAmount currencyID="EUR">100.00</ram:TaxTotalAmount>
<ram:GrandTotalAmount>600.00</ram:GrandTotalAmount>
<ram:DuePayableAmount>450.00</ram:DuePayableAmount>
</ram:SpecifiedTradeSettlementHeaderMonetarySummation>
</ram:ApplicableHeaderTradeSettlement>
</rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>`

// basicWL is a valid BASIC WL invoice: lines 1000.00, allowance 100.00 at
// 20 %, breakdowns 500.00 at 20 % and 400.00 at 5.5 %, paid 22.00.
const basicWL = header + `<rsm:ExchangedDocumentContext>
<ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>urn:factur-x.eu:1p0:basicwl</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter>
</rsm:ExchangedDocumentContext>
<rsm:ExchangedDocument>
<ram:ID>F-2024-002</ram:ID>
<ram:TypeCode>380</ram:TypeCode>
<ram:IssueDateTime><udt:DateTimeString format="102">20240115</udt:DateTimeString></ram:IssueDateTime>
<ram:IncludedNote><ram:Content>Thank you</ram:Content></ram:IncludedNote>
</rsm:ExchangedDocument>
<rsm:SupplyChainTradeTransaction>
<ram:ApplicableHeaderTradeAgreement>
<ram:SellerTradeParty>
<ram:Name>Seller SARL</ram:Name>
<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:PostcodeCode>75001</ram:PostcodeCode><ram:CityName>Paris</ram:CityName><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration>
</ram:SellerTradeParty>
<ram:BuyerTradeParty>
<ram:Name>Buyer SAS</ram:Name>
<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR45987654321</ram:ID></ram:SpecifiedTaxRegistration>
</ram:BuyerTradeParty>
</ram:ApplicableHeaderTradeAgreement>
<ram:ApplicableHeaderTradeDelivery/>
<ram:ApplicableHeaderTradeSettlement>
<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
<ram:SpecifiedTradeSettlementPaymentMeans>
<ram:TypeCode>30</ram:TypeCode>
<ram:PayeePartyCreditorFinancialAccount><ram:IBANID>FR7630006000011234567890189</ram:IBANID></ram:PayeePartyCreditorFinancialAccount>
</ram:SpecifiedTradeSettlementPaymentMeans>
<ram:ApplicableTradeTax>
<ram:CalculatedAmount>100.00</ram:CalculatedAmount>
<ram:TypeCode>VAT</ram:TypeCode>
<ram:BasisAmount>500.00</ram:BasisAmount>
<ram:CategoryCode>S</ram:CategoryCode>
<ram:RateApplicablePercent>20.00</ram:RateApplicablePercent>
</ram:ApplicableTradeTax>
<ram:ApplicableTradeTax>
<ram:CalculatedAmount>22.00</ram:CalculatedAmount>
<ram:TypeCode>VAT</ram:TypeCode>
<ram:BasisAmount>400.00</ram:BasisAmount>
<ram:CategoryCode>S</ram:CategoryCode>
<ram:RateApplicablePercent>5.50</ram:RateApplicablePercent>
</ram:ApplicableTradeTax>
<ram:SpecifiedTradeAllowanceCharge>
<ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator>
<ram:ActualAmount>100.00</ram:ActualAmount>
<ram:Reason>Discount</ram:Reason>
<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>20.00</ram:RateApplicablePercent></ram:CategoryTradeTax>
</ram:SpecifiedTradeAllowanceCharge>
<ram:SpecifiedTradePaymentTerms>
<ram:DueDateDateTime><udt:DateTimeString format="102">20240215</udt:DateTimeString></ram:DueDateDateTime>
</ram:SpecifiedTradePaymentTerms>
<ram:SpecifiedTradeSettlementHeaderMonetarySummation>
<ram:LineTotalAmount>1000.00</ram:LineTotalAmount>
<ram:AllowanceTotalAmount>100.00</ram:AllowanceTotalAmount>
<ram:TaxBasisTotalAmount>900.00</ram:TaxBasisTotalAmount>
<ram:TaxTotalAmount currencyID="EUR">122.00</ram:TaxTotalAmount>
<ram:GrandTotalAmount>1022.00</ram:GrandTotalAmount>
<ram:TotalPrepaidAmount>22.00</ram:TotalPrepaidAmount>
<ram:DuePayableAmount>1000.00</ram:DuePayableAmount>
</ram:SpecifiedTradeSettlementHeaderMonetarySummation>
</ram:ApplicableHeaderTradeSettlement>
</rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>`

const secondTax = `<ram:ApplicableTradeTax>
<ram:CalculatedAmount>22.00</ram:CalculatedAmount>
<ram:TypeCode>VAT</ram:TypeCode>
<ram:BasisAmount>400.00</ram:BasisAmount>
<ram:CategoryCode>S</ram:CategoryCode>
<ram:RateApplicablePercent>5.50</ram:RateApplicablePercent>
</ram:ApplicableTradeTax>`

func run(t testing.TB, data string, p invoice.Profile) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, p), Report: r, Source: rules.SourceFacturX})
	return r
}

// ruleSet returns the sorted distinct rules of r.
func ruleSet(r *rules.Report) string {
	seen := map[string]bool{}
	var ids []string
	for _, f := range r.Findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			ids = append(ids, f.Rule)
		}
	}
	sort.Strings(ids)
	return strings.Join(ids, " ")
}

// TestFixtures checks that the fixtures are valid, for the schema too.
func TestFixtures(t *testing.T) {
	for name, data := range map[string]string{"MINIMUM": minimum, "BASIC WL": basicWL} {
		root, err := xmltree.Parse([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		r := &rules.Report{}
		doc := rules.NewDoc(root, invoice.ProfileUnknown)
		if doc.Profile.String() != name {
			t.Errorf("%s: profile %s", name, doc.Profile)
		}
		schema.Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceSyntax})
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceFacturX})
		if len(r.Findings) > 0 {
			t.Errorf("%s: unexpected findings:\n%s", name, r)
		}
	}
}

type mutation struct {
	name     string
	old, new string
	want     string // sorted distinct rules
}

var bwlMutations = []mutation{
	{"seller name", "<ram:Name>Seller SARL</ram:Name>", "", "BR-06 FX-SCH-A-000030"},
	{"BT-24 white space", "<ram:ID>urn:factur-x.eu:1p0:basicwl</ram:ID>", "<ram:ID> urn:factur-x.eu:1p0:basicwl</ram:ID>", "FX-SCH-A-000026"},
	{"VAT prefix", "FR32123456789", "XX32123456789", "BR-CO-09"},
	{"seller identifiers", `<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:PostcodeCode>75001</ram:PostcodeCode><ram:CityName>Paris</ram:CityName><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">`, `<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">`, "BR-CO-26"},
	{"grand total", "<ram:GrandTotalAmount>1022.00", "<ram:GrandTotalAmount>1023.00", "BR-CO-15 BR-CO-16"},
	{"tax basis total", "<ram:TaxBasisTotalAmount>900.00", "<ram:TaxBasisTotalAmount>901.00", "BR-CO-13 BR-CO-15"},
	{"allowance total", "<ram:AllowanceTotalAmount>100.00", "<ram:AllowanceTotalAmount>90.00", "BR-CO-11 BR-CO-13"},
	{"tax amount", "<ram:CalculatedAmount>100.00", "<ram:CalculatedAmount>102.00", "BR-CO-14 BR-CO-17 BR-S-09"},
	{"tax amount within tolerance", "<ram:CalculatedAmount>100.00", "<ram:CalculatedAmount>100.50", "BR-CO-14"},
	{"exemption on S", "<ram:BasisAmount>500.00</ram:BasisAmount>", "<ram:ExemptionReason>Why</ram:ExemptionReason><ram:BasisAmount>500.00</ram:BasisAmount>", "BR-S-10"},
	{"rate missing", "<ram:RateApplicablePercent>5.50</ram:RateApplicablePercent>", "", "BR-48 BR-CO-17 BR-S-09"},
	{"category code", "<ram:CategoryCode>S</ram:CategoryCode>\n<ram:RateApplicablePercent>5.50", "<ram:CategoryCode>X</ram:CategoryCode>\n<ram:RateApplicablePercent>5.50", "FX-SCH-A-000179"},
	{"intra-community", secondTax, `<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>400.00</ram:BasisAmount><ram:CategoryCode>K</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
		"BR-CO-14 BR-IC-10 BR-IC-11 BR-IC-12"},
	{"not subject to VAT", secondTax, `<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:ExemptionReason>Out of scope</ram:ExemptionReason><ram:BasisAmount>400.00</ram:BasisAmount><ram:CategoryCode>O</ram:CategoryCode></ram:ApplicableTradeTax>`,
		"BR-CO-14 BR-O-11 BR-O-13 BR-O-14"},
	{"exempt with tax", secondTax, `<ram:ApplicableTradeTax><ram:CalculatedAmount>22.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>400.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
		"BR-CO-17 BR-E-09 BR-E-10"},
	{"allowance reason", "<ram:Reason>Discount</ram:Reason>", "", "BR-33 BR-CO-21"},
	{"allowance category rate", "<ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>20.00</ram:RateApplicablePercent></ram:CategoryTradeTax>", "<ram:CategoryCode>E</ram:CategoryCode><ram:RateApplicablePercent>20.00</ram:RateApplicablePercent></ram:CategoryTradeTax>", "BR-E-06"},
	{"allowance indicator", "<udt:Indicator>false</udt:Indicator>", "<udt:Indicator>0</udt:Indicator>", "BR-CO-11 FX-NOT-USED"},
	{"allowance reason code", "<ram:Reason>Discount</ram:Reason>", "<ram:ReasonCode>999</ram:ReasonCode>", "FX-SCH-A-000186"},
	{"allowance amount decimals", "<ram:ActualAmount>100.00</ram:ActualAmount>", "<ram:ActualAmount>100.000</ram:ActualAmount>", "BR-DEC-01"},
	{"charge without total", "</ram:SpecifiedTradeAllowanceCharge>", `</ram:SpecifiedTradeAllowanceCharge><ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount><ram:Reason>Shipping</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>20</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>`, "BR-CO-12"},
	{"total VAT decimals", `<ram:TaxTotalAmount currencyID="EUR">122.00`, `<ram:TaxTotalAmount currencyID="EUR">122.001`, "BR-CO-14 BR-DEC-13"},
	{"line total decimals", "<ram:LineTotalAmount>1000.00", "<ram:LineTotalAmount>1000.000", "BR-DEC-09"},
	// BR-CO-25 and BR-CO-18 left the BASIC WL schematron in Factur-X 1.09.
	{"payment terms", "<ram:SpecifiedTradePaymentTerms>\n<ram:DueDateDateTime><udt:DateTimeString format=\"102\">20240215</udt:DateTimeString></ram:DueDateDateTime>\n</ram:SpecifiedTradePaymentTerms>", "", ""},
	{"blank account", "<ram:IBANID>FR7630006000011234567890189</ram:IBANID>", "<ram:IBANID> </ram:IBANID>", "BR-50"},
	{"full card number", "<ram:TypeCode>30</ram:TypeCode>", "<ram:TypeCode>30</ram:TypeCode><ram:ApplicableTradeSettlementFinancialCard><ram:ID>4111111111111111</ram:ID></ram:ApplicableTradeSettlementFinancialCard>", "BR-51"},
	{"masked card number", "<ram:TypeCode>30</ram:TypeCode>", "<ram:TypeCode>30</ram:TypeCode><ram:ApplicableTradeSettlementFinancialCard><ram:ID>  123   567890 </ram:ID></ram:ApplicableTradeSettlementFinancialCard>", ""},
	{"tax representative without address", "</ram:BuyerTradeParty>", `</ram:BuyerTradeParty><ram:SellerTaxRepresentativeTradeParty><ram:Name> </ram:Name><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTaxRepresentativeTradeParty>`, "BR-18 BR-19 BR-20 FX-SCH-A-000032"},
	{"charge reason code", "</ram:SpecifiedTradeAllowanceCharge>", "</ram:SpecifiedTradeAllowanceCharge>" + charge("<ram:ReasonCode>XYZ</ram:ReasonCode>"), "BR-CO-12 FX-SCH-A-000376"},
	{"charge reason code in UNTDID 7161", "</ram:SpecifiedTradeAllowanceCharge>", "</ram:SpecifiedTradeAllowanceCharge>" + charge("<ram:ReasonCode>ABL</ram:ReasonCode>"), "BR-CO-12"},
	{"charge reason code of list 5153", "</ram:SpecifiedTradeAllowanceCharge>", "</ram:SpecifiedTradeAllowanceCharge>" + charge(`<ram:ReasonCode listID="5153">XYZ</ram:ReasonCode>`), "BR-CO-12"},
	{"charge reason code of another list", "</ram:SpecifiedTradeAllowanceCharge>", "</ram:SpecifiedTradeAllowanceCharge>" + charge(`<ram:ReasonCode listID="7161">ABL</ram:ReasonCode>`), "BR-CO-12 FX-NOT-USED"},
	{"zero rated breakdown", secondTax, `<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>400.00</ram:BasisAmount><ram:CategoryCode>Z</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
		"BR-CO-14 BR-FXEXT-Z-08rev"},
	{"IBAN and proprietary", "<ram:IBANID>FR7630006000011234567890189</ram:IBANID>", "<ram:IBANID>FR7630006000011234567890189</ram:IBANID><ram:ProprietaryID>123</ram:ProprietaryID>", "BR-CO-27"},
	{"no account", "<ram:PayeePartyCreditorFinancialAccount><ram:IBANID>FR7630006000011234567890189</ram:IBANID></ram:PayeePartyCreditorFinancialAccount>", "<ram:PayeePartyCreditorFinancialAccount/>", "BR-50 BR-61 BR-CO-27"},
	{"type code", "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>", "FX-SCH-A-000023"},
	{"tax type code", "<ram:TypeCode>VAT</ram:TypeCode>\n<ram:BasisAmount>400.00", "<ram:TypeCode>GST</ram:TypeCode>\n<ram:BasisAmount>400.00", "BR-CO-17 FX-SCH-A-000023"},
	{"invalid date", "20240115", "20240231", "FX-DATE"},
	{"date format", `format="102">20240215`, `format="610">202402`, "FX-SCH-A-000022"},
	{"buyer address", "<ram:Name>Buyer SAS</ram:Name>\n<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>", "<ram:Name>Buyer SAS</ram:Name>", "BR-10 BR-11 FX-SCH-A-000032"},
	{"invoicing period", "<ram:SpecifiedTradeAllowanceCharge>", `<ram:BillingSpecifiedPeriod><ram:StartDateTime><udt:DateTimeString format="102">20240131</udt:DateTimeString></ram:StartDateTime><ram:EndDateTime><udt:DateTimeString format="102">20240101</udt:DateTimeString></ram:EndDateTime></ram:BillingSpecifiedPeriod><ram:SpecifiedTradeAllowanceCharge>`, "BR-29"},
	{"empty invoicing period", "<ram:SpecifiedTradeAllowanceCharge>", `<ram:BillingSpecifiedPeriod/><ram:SpecifiedTradeAllowanceCharge>`, "BR-CO-19"},
	{"two seller VAT identifiers", `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration>`, `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456780</ram:ID></ram:SpecifiedTaxRegistration>`, "FX-SCH-A-000033"},
	{"seller tax scheme", `<ram:ID schemeID="VA">FR32123456789</ram:ID>`, `<ram:ID schemeID="XX">FR32123456789</ram:ID>`, "BR-S-03 FX-NOT-USED"},
	{"total VAT in other currency", `<ram:TaxTotalAmount currencyID="EUR">122.00</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="EUR">122.00</ram:TaxTotalAmount><ram:TaxTotalAmount currencyID="USD">130.00</ram:TaxTotalAmount>`, "FX-NOT-USED"},
	{"accounting currency", "<ram:InvoiceCurrencyCode>", "<ram:TaxCurrencyCode>EUR</ram:TaxCurrencyCode><ram:InvoiceCurrencyCode>", "BR-53"},
	{"payee without name", "<ram:SpecifiedTradeSettlementPaymentMeans>", "<ram:PayeeTradeParty><ram:ID>P1</ram:ID></ram:PayeeTradeParty><ram:SpecifiedTradeSettlementPaymentMeans>", "BR-17 FX-SCH-A-000030"},
	{"global identifier scheme", "<ram:Name>Buyer SAS</ram:Name>", "<ram:GlobalID>3012345678901</ram:GlobalID><ram:Name>Buyer SAS</ram:Name>", "FX-SCH-A-000037"},
	{"electronic address scheme", "</ram:PostalTradeAddress>\n<ram:SpecifiedTaxRegistration><ram:ID schemeID=\"VA\">FR32", "</ram:PostalTradeAddress>\n<ram:URIUniversalCommunication><ram:URIID>billing@example.com</ram:URIID></ram:URIUniversalCommunication><ram:SpecifiedTaxRegistration><ram:ID schemeID=\"VA\">FR32", "BR-62 FX-SCH-A-000037"},
	{"currencyID not used", "<ram:GrandTotalAmount>", `<ram:GrandTotalAmount currencyID="EUR">`, "FX-NOT-USED"},
	{"tax representative", "</ram:BuyerTradeParty>", "</ram:BuyerTradeParty><ram:SellerTaxRepresentativeTradeParty><ram:Name>Rep</ram:Name><ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress></ram:SellerTaxRepresentativeTradeParty>", "BR-56 FX-SCH-A-000169"},
	{"deliver to country", "<ram:ApplicableHeaderTradeDelivery/>", "<ram:ApplicableHeaderTradeDelivery><ram:ShipToTradeParty><ram:PostalTradeAddress><ram:CountryID></ram:CountryID></ram:PostalTradeAddress></ram:ShipToTradeParty></ram:ApplicableHeaderTradeDelivery>", "BR-57"},
	{"preceding invoice", "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>", "</ram:SpecifiedTradeSettlementHeaderMonetarySummation><ram:InvoiceReferencedDocument><ram:IssuerAssignedID></ram:IssuerAssignedID></ram:InvoiceReferencedDocument>", "BR-55"},
	{"VAT point date code", "<ram:RateApplicablePercent>5.50</ram:RateApplicablePercent>", "<ram:DueDateTypeCode>3</ram:DueDateTypeCode><ram:RateApplicablePercent>5.50</ram:RateApplicablePercent>", "FX-SCH-A-000180"},
	{"charge indicator", "<ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator>", "", "BR-CO-11 CII-SR-463 FX-NOT-USED"},
	{"no breakdown", secondTax, "", "BR-CO-14"},
	{"invoice number", "<ram:ID>F-2024-002</ram:ID>", "<ram:ID></ram:ID>", "BR-02"},
	{"issue date format", `<udt:DateTimeString format="102">20240115</udt:DateTimeString></ram:IssueDateTime>`, `<udt:DateTimeString format="610">202401</udt:DateTimeString></ram:IssueDateTime>`, "BR-03 FX-SCH-A-000022"},
}

// charge returns a document level charge of 10.00 at 20 % with the given
// reason elements.
func charge(reason string) string {
	return `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount>` + reason +
		`<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>20</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>`
}

// minTax returns a mutation of the MINIMUM invoice that inserts elems
// before the totals, which the MINIMUM schema forbids but the MINIMUM
// schematron checks.
func minTax(name, elems, want string) mutation {
	const at = "<ram:SpecifiedTradeSettlementHeaderMonetarySummation>"
	return mutation{name, at, elems + at, want}
}

// breakdown returns a VAT breakdown of category cat.
func breakdown(cat, basis, calc, rest string) string {
	return "<ram:ApplicableTradeTax><ram:CalculatedAmount>" + calc + "</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode>" + rest +
		"<ram:BasisAmount>" + basis + "</ram:BasisAmount><ram:CategoryCode>" + cat + "</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>"
}

// minLine returns an invoice line of category cat at rate.
func minLine(cat, rate string) string {
	return "<ram:IncludedSupplyChainTradeLineItem><ram:SpecifiedLineTradeSettlement><ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>" + cat +
		"</ram:CategoryCode><ram:RateApplicablePercent>" + rate + "</ram:RateApplicablePercent></ram:ApplicableTradeTax></ram:SpecifiedLineTradeSettlement></ram:IncludedSupplyChainTradeLineItem>"
}

var minMutations = []mutation{
	{"buyer address", "<ram:Name>Buyer SAS</ram:Name>", "<ram:Name>Buyer SAS</ram:Name><ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>", "FX-NOT-USED"},
	{"buyer VAT identifier", "<ram:Name>Buyer SAS</ram:Name>", `<ram:Name>Buyer SAS</ram:Name><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR45987654321</ram:ID></ram:SpecifiedTaxRegistration>`, "FX-NOT-USED"},
	{"grand total", "<ram:GrandTotalAmount>600.00", "<ram:GrandTotalAmount>600.01", "FX-MIN-CO-15"},
	{"grand total without VAT", "<ram:GrandTotalAmount>600.00", "<ram:GrandTotalAmount>500.00", ""},
	{"seller address", "<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>", "", "BR-08 BR-09 FX-SCH-A-000032"},
	{"type code", "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>1</ram:TypeCode>", "FX-SCH-A-000023"},
	{"issue date format", `format="102">20240115`, `format="610">202401`, "BR-03 FX-SCH-A-000022"},
	{"buyer legal organization", "<ram:Name>Buyer SAS</ram:Name>", "<ram:Name>Buyer SAS</ram:Name><ram:SpecifiedLegalOrganization/>", "FX-SCH-A-000019"},
	{"decimals", "<ram:TaxBasisTotalAmount>500.00", "<ram:TaxBasisTotalAmount>500.001", "BR-DEC-12"},
	{"seller tax scheme", `schemeID="VA">FR32`, `schemeID="XX">FR32`, "FX-NOT-USED"},
	{"currencyID", "<ram:GrandTotalAmount>", `<ram:GrandTotalAmount currencyID="EUR">`, "FX-NOT-USED"},
	{"two VAT totals", `<ram:TaxTotalAmount currencyID="EUR">100.00</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="EUR">100.00</ram:TaxTotalAmount><ram:TaxTotalAmount currencyID="EUR">100.00</ram:TaxTotalAmount>`, "FX-MIN-CO-15 FX-SCH-A-000042"},
	{"seller name", "<ram:Name>Seller SARL</ram:Name>", "<ram:Name></ram:Name>", "BR-06"},
	{"BT-24", "<ram:ID>urn:factur-x.eu:1p0:minimum</ram:ID>", "<ram:ID></ram:ID>", "FX-PROFILE-01"},
	{"seller identifiers", `<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>`, "", ""},
	{"no seller identifier", `<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR32123456789</ram:ID></ram:SpecifiedTaxRegistration>`, "<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>", "BR-CO-26"},
	{"invalid date", "20240115", "2024-01-15", "FX-DATE"},

	// Rules of the MINIMUM schematron 1.09 whose contexts the MINIMUM schema
	// forbids.
	minTax("export breakdown", breakdown("G", "0.00", "0.00", "<ram:ExemptionReason>Export</ram:ExemptionReason>"), ""),
	minTax("export breakdown at the lower tolerance bound", breakdown("G", "1.00", "0.00", "<ram:ExemptionReason>Export</ram:ExemptionReason>"),
		"BR-FXEXT-G-08 BR-FXEXT-G-08b BR-FXEXT-G-08ini BR-FXEXT-G-08rev"),
	minTax("export breakdown beyond the tolerance", breakdown("G", "1.01", "2.00", ""),
		"BR-FXEXT-G-08 BR-FXEXT-G-08b BR-FXEXT-G-08ini BR-FXEXT-G-08rev BR-G-08 BR-G-09 BR-G-10"),
	minTax("exempt breakdown", breakdown("E", "0.00", "5.00", ""), "BR-E-10"),
	minTax("intra-community breakdown at the upper tolerance bound", breakdown("K", "-1.00", "0.00", "<ram:ExemptionReasonCode>VATEX-EU-IC</ram:ExemptionReasonCode>"),
		"BR-FXEXT-IC-08b BR-FXEXT-IC-08ini BR-FXEXT-IC-08rev BR-IC-08 BR-IC-11 BR-IC-12"),
	minTax("export allowance", `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>G</ram:CategoryCode><ram:RateApplicablePercent>5</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>`,
		"BR-G-06"),
	{"exempt and export lines", "<rsm:SupplyChainTradeTransaction>\n<ram:ApplicableHeaderTradeAgreement>\n<ram:BuyerReference>REF-42</ram:BuyerReference>\n<ram:SellerTradeParty>\n<ram:Name>Seller SARL</ram:Name>\n" +
		`<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">`,
		"<rsm:SupplyChainTradeTransaction>" + minLine("E", "5") + minLine("G", "0") + "\n<ram:ApplicableHeaderTradeAgreement>\n<ram:BuyerReference>REF-42</ram:BuyerReference>\n<ram:SellerTradeParty>\n<ram:Name>Seller SARL</ram:Name>\n" +
			`<ram:SpecifiedLegalOrganization><ram:ID schemeID="0002">12345678900011</ram:ID></ram:SpecifiedLegalOrganization>
<ram:PostalTradeAddress><ram:CountryID>FR</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">`,
		"BR-E-05 BR-G-02"},
}

func TestMutations(t *testing.T) {
	for _, set := range []struct {
		name string
		base string
		ms   []mutation
	}{{"BASIC WL", basicWL, bwlMutations}, {"MINIMUM", minimum, minMutations}} {
		for _, m := range set.ms {
			t.Run(set.name+"/"+m.name, func(t *testing.T) {
				if !strings.Contains(set.base, m.old) {
					t.Fatalf("%q not found", m.old)
				}
				r := run(t, strings.Replace(set.base, m.old, m.new, 1), invoice.ProfileUnknown)
				if got := ruleSet(r); got != m.want {
					t.Errorf("rules = %q, want %q\n%s", got, m.want, r)
				}
				for _, f := range r.Findings {
					if f.Path == "" || f.Line == 0 || f.Message == "" {
						t.Errorf("incomplete finding %+v", f)
					}
				}
			})
		}
	}
}

func TestProfile(t *testing.T) {
	en := readTestdata(t, "EN16931_Einfach.cii.xml")
	xr := readTestdata(t, "XRECHNUNG_Einfach.cii.xml")
	const bt24 = "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>"
	for _, tc := range []struct {
		name    string
		data    string
		profile invoice.Profile
		want    string
	}{
		{"EN 16931", en, invoice.ProfileUnknown, ""},
		{"XRECHNUNG", xr, invoice.ProfileUnknown, ""},
		{"XRECHNUNG checked as EN 16931", xr, invoice.ProfileEN16931, ""},
		{"EN 16931 checked as EXTENDED", en, invoice.ProfileExtended, "FX-SCH-A-000026"},
		{"unknown", strings.Replace(en, bt24, "<ram:ID>urn:example:invoice</ram:ID>", 1), invoice.ProfileUnknown, "FX-PROFILE-02"},
		{"missing", strings.Replace(en, bt24, "<ram:ID/>", 1), invoice.ProfileUnknown, "FX-PROFILE-01"},
		{"legacy", strings.Replace(en, bt24, "<ram:ID>urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic</ram:ID>", 1), invoice.ProfileUnknown, "FX-PROFILE-03"},
		{"upper case", strings.Replace(en, bt24, "<ram:ID>URN:CEN.EU:EN16931:2017</ram:ID>", 1), invoice.ProfileUnknown, "FX-PROFILE-03"},
		{"white space", strings.Replace(en, bt24, "<ram:ID>urn:cen.eu:en16931:2017 </ram:ID>", 1), invoice.ProfileUnknown, "FX-SCH-A-000026"},
		{"BASIC WL checked as MINIMUM", basicWL, invoice.ProfileMinimum, "FX-NOT-USED FX-SCH-A-000026"},
		{"missing, checked as MINIMUM", strings.Replace(minimum, "<ram:ID>urn:factur-x.eu:1p0:minimum</ram:ID>", "<ram:ID/>", 1), invoice.ProfileMinimum, "BR-01"},
		{"MINIMUM checked as BASIC WL", minimum, invoice.ProfileBasicWL, "BR-10 BR-11 BR-12 BR-CO-13 BR-CO-14 BR-CO-16 FX-SCH-A-000026 FX-SCH-A-000032"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ruleSet(run(t, tc.data, tc.profile)); got != tc.want {
				t.Errorf("rules = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLegacyProfile(t *testing.T) {
	for in, want := range map[string]string{
		"urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic":     invoice.URNBasic,
		"urn:cen.eu:en16931:2017#conformant#factur-x.eu:1p0:extended": invoice.URNExtended,
		"URN:FACTUR-X.EU:1P0:MINIMUM":                                 invoice.URNMinimum,
		"urn:example":                                                 "",
	} {
		p, got := legacyProfile(in)
		if want == "" && p != invoice.ProfileUnknown || want != "" && got != want {
			t.Errorf("legacyProfile(%q) = %s, %q; want %q", in, p, got, want)
		}
	}
}

func TestHelpers(t *testing.T) {
	for s, ok := range map[string]bool{"20240229": true, " 20240115\n": true, "20230229": false, "2024011": false, "2024-01-15": false, "": false} {
		if validDate102(s) != ok {
			t.Errorf("validDate102(%q) = %t", s, !ok)
		}
	}
	if firstRunes("ÉLx", 2) != "ÉL" || firstRunes("F", 2) != "F" {
		t.Error("firstRunes")
	}
	for s, want := range map[string][2]bool{"true": {true, true}, " 1 ": {true, true}, "false": {false, true}, "0": {false, true}, "yes": {false, false}} {
		v, ok := xsBoolean(s)
		if v != want[0] || ok != want[1] {
			t.Errorf("xsBoolean(%q) = %t, %t", s, v, ok)
		}
	}
}

// TestCatalog checks that every rule reported by the tests is in Catalog,
// with the same severity, and that Catalog has no duplicate.
func TestCatalog(t *testing.T) {
	sev := map[string]rules.Severity{}
	for _, ri := range Catalog() {
		if _, dup := sev[ri.ID]; dup {
			t.Errorf("%s twice in Catalog", ri.ID)
		}
		sev[ri.ID] = ri.Severity
		if ri.Description == "" {
			t.Errorf("%s has no description", ri.ID)
		}
	}
	check := func(r *rules.Report) {
		for _, f := range r.Findings {
			if s, ok := sev[f.Rule]; !ok || s != f.Severity {
				t.Errorf("finding %s (%s) does not match Catalog", f.Rule, f.Severity)
			}
		}
	}
	for _, m := range bwlMutations {
		check(run(t, strings.Replace(basicWL, m.old, m.new, 1), invoice.ProfileUnknown))
	}
	for _, m := range minMutations {
		check(run(t, strings.Replace(minimum, m.old, m.new, 1), invoice.ProfileUnknown))
	}
}

// knownErrors lists the corpus files on which the official schematron of
// their profile reports errors too, with the rules and the reason.
var knownErrors = map[string]struct {
	rules []string
	why   string
}{
	"Facture_UE_BASICWL.xml": {[]string{"BR-IC-11", "BR-IC-12"},
		"FNFE example (2017): intra-community supply without delivery date, invoicing period nor deliver to country, required by BR-IC-11 and BR-IC-12 since EN 16931 (2017)"},
	"Avoir_FR_type381_BASIC.xml": {[]string{"FX-PROFILE-03"},
		"FNFE example (2017): specification identifier of a Factur-X 1.0 draft, out of the code list of BT-24 (FX-SCH-A-000493)"},
	// ZUGFeRD 2.0 and 2.1 examples (intarsys, symtrax): codes out of the
	// lists of the Factur-X 1.09.2 EXTENDED schematron, which Mustang
	// rejects too.
	"zugferd_2p0_EXTENDED_Fremdwaehrung.xml": {[]string{"FX-SCH-A-000704", "FX-SCH-A-000973", "FX-SCH-A-001137"},
		"electronic address scheme 9958 (EAS), allowance reasons ABK (a charge code), ADZ and CAO out of UNTDID 5189"},
	"zugferd_2p1_EXTENDED_Fremdwaehrung.xml": {[]string{"FX-SCH-A-000704", "FX-SCH-A-000973", "FX-SCH-A-001137"},
		"electronic address scheme 9958 (EAS), allowance reasons ABK (a charge code), ADZ and CAO out of UNTDID 5189"},
	"zugferd_2p0_EXTENDED_InnergemeinschLieferungMehrereBestellungen.xml": {[]string{"FX-SCH-A-000704"},
		"electronic address scheme 9958, out of the EAS list"},
	"zugferd_2p1_EXTENDED_InnergemeinschLieferungMehrereBestellungen.xml": {[]string{"FX-SCH-A-000704"},
		"electronic address scheme 9958, out of the EAS list"},
	"zugferd_2p0_EXTENDED_Rechnungskorrektur.xml": {[]string{"FX-SCH-A-001084"},
		"package units BO and CT (UN/ECE recommendation 21), out of the list of PackageQuantity"},
	"zugferd_2p0_EXTENDED_Warenrechnung.xml": {[]string{"FX-SCH-A-001084"},
		"package units BC, BO, CT and PX (UN/ECE recommendation 21), out of the list of PackageQuantity"},
}

func TestCorpus(t *testing.T) {
	files := map[string][]byte{}
	for _, env := range []string{"FX_CORPUS", "FX_CORPUS_XML"} {
		dir := os.Getenv(env)
		if dir == "" {
			continue
		}
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".xml") {
				return nil
			}
			if data, err := os.ReadFile(path); err == nil {
				files[path] = data
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("FX_CORPUS and FX_CORPUS_XML not set or empty")
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	type stat struct{ files, clean, warned, known int }
	stats := map[string]*stat{}
	for _, path := range paths {
		root, err := xmltree.Parse(files[path])
		if err != nil || root.Name != "CrossIndustryInvoice" {
			continue
		}
		doc := rules.NewDoc(root, invoice.ProfileUnknown)
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceFacturX})
		s := stats[doc.Profile.String()]
		if s == nil {
			s = &stat{}
			stats[doc.Profile.String()] = s
		}
		s.files++
		known, isKnown := knownErrors[filepath.Base(path)]
		switch {
		case isKnown:
			s.known++
			for _, f := range r.Errors() {
				ok := false
				for _, id := range known.rules {
					ok = ok || f.Rule == id
				}
				if !ok {
					t.Errorf("%s: unexpected %v (known: %s)", path, f, known.why)
				}
			}
		case !r.Valid():
			t.Errorf("%s (%s):\n%s", path, doc.Profile, r)
		case len(r.Findings) > 0:
			s.warned++
			if doc.Profile == invoice.ProfileMinimum || doc.Profile == invoice.ProfileBasicWL || doc.Profile == invoice.ProfileUnknown {
				t.Logf("%s: warnings %s", filepath.Base(path), ruleSet(r))
			}
		default:
			s.clean++
		}
	}
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := stats[k]
		t.Logf("%-16s %3d files: %d without finding, %d with warnings only, %d known", k, s.files, s.clean, s.warned, s.known)
	}
}

func readTestdata(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func benchmark(b *testing.B, data string) {
	root, err := xmltree.Parse([]byte(data))
	if err != nil {
		b.Fatal(err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	r := &rules.Report{}
	c := &rules.Checker{Doc: doc, Report: r, Source: rules.SourceFacturX}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Findings = r.Findings[:0]
		Check(c)
	}
}

func BenchmarkCheckMinimum(b *testing.B) { benchmark(b, minimum) }

func BenchmarkCheckBasicWL(b *testing.B) { benchmark(b, basicWL) }

func BenchmarkCheckEN16931(b *testing.B) { benchmark(b, readTestdata(b, "EN16931_Einfach.cii.xml")) }
