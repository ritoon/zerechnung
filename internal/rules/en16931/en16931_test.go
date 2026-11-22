package en16931

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The unit tests mutate a small valid EN 16931 invoice. Each case lists the
// exact set of rules it must report. The invoice has a payment due date
// (BT-9), as BR-CO-25 requires of a positive amount due: no CEN artefact
// checks BR-CO-25 on CII, but the documents of the cases go to other
// validators (TestVariants), which may. The expected sets of the cases checked
// in CEN mode are the verdicts of the official CEN XSLT 1.3.17, except for
// CII-SR-010 and CII-SR-477, on which the XSLT stops with a type error.

const (
	tHeader = `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100" xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100">
<rsm:ExchangedDocumentContext>
<ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>urn:cen.eu:en16931:2017</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter>
</rsm:ExchangedDocumentContext>
<rsm:ExchangedDocument>
<ram:ID>INV-1</ram:ID>
<ram:TypeCode>380</ram:TypeCode>
<ram:IssueDateTime><udt:DateTimeString format="102">20240115</udt:DateTimeString></ram:IssueDateTime>
</rsm:ExchangedDocument>
<rsm:SupplyChainTradeTransaction>
`
	tLine = `<ram:IncludedSupplyChainTradeLineItem>
<ram:AssociatedDocumentLineDocument><ram:LineID>1</ram:LineID></ram:AssociatedDocumentLineDocument>
<ram:SpecifiedTradeProduct><ram:Name>Widget</ram:Name></ram:SpecifiedTradeProduct>
<ram:SpecifiedLineTradeAgreement><ram:NetPriceProductTradePrice><ram:ChargeAmount>10.00</ram:ChargeAmount></ram:NetPriceProductTradePrice></ram:SpecifiedLineTradeAgreement>
<ram:SpecifiedLineTradeDelivery><ram:BilledQuantity unitCode="C62">10</ram:BilledQuantity></ram:SpecifiedLineTradeDelivery>
<ram:SpecifiedLineTradeSettlement>
<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>
<ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>100.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>
</ram:SpecifiedLineTradeSettlement>
</ram:IncludedSupplyChainTradeLineItem>
`
	tAgreement = `<ram:ApplicableHeaderTradeAgreement>
<ram:SellerTradeParty><ram:Name>Seller</ram:Name><ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTradeParty>
<ram:BuyerTradeParty><ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress></ram:BuyerTradeParty>
</ram:ApplicableHeaderTradeAgreement>
<ram:ApplicableHeaderTradeDelivery></ram:ApplicableHeaderTradeDelivery>
`
	tSettlement = `<ram:ApplicableHeaderTradeSettlement>
<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
<ram:ApplicableTradeTax><ram:CalculatedAmount>19.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>
` + tPaymentTerms + `
<ram:SpecifiedTradeSettlementHeaderMonetarySummation>
<ram:LineTotalAmount>100.00</ram:LineTotalAmount>
<ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>
<ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>
<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>
<ram:DuePayableAmount>119.00</ram:DuePayableAmount>
</ram:SpecifiedTradeSettlementHeaderMonetarySummation>
</ram:ApplicableHeaderTradeSettlement>
`
	tFooter = `</rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>
`
	tBase = tHeader + tLine + tAgreement + tSettlement + tFooter
	// tPaymentTerms is the payment due date (BT-9) of tBase and tFull.
	tPaymentTerms = `<ram:SpecifiedTradePaymentTerms><ram:DueDateDateTime><udt:DateTimeString format="102">20240215</udt:DateTimeString></ram:DueDateDateTime></ram:SpecifiedTradePaymentTerms>`
)

// edit replaces each old string of pairs (old, new, old, new...) in s; every
// old string must occur exactly once.
func edit(t *testing.T, s string, pairs ...string) string {
	t.Helper()
	for i := 0; i+1 < len(pairs); i += 2 {
		if n := strings.Count(s, pairs[i]); n != 1 {
			t.Fatalf("%q occurs %d times", pairs[i], n)
		}
		s = strings.Replace(s, pairs[i], pairs[i+1], 1)
	}
	return s
}

// findings runs Check and returns the sorted distinct rules found, failing
// the test when a rule is not in Catalog or CTCFRCatalog.
func findings(t *testing.T, doc string, profile invoice.Profile) (*rules.Report, []string) {
	t.Helper()
	r := check(t, []byte(doc), profile)
	set := map[string]bool{}
	for _, f := range r.Findings {
		if _, ok := severity[f.Rule]; !ok && ctcfrRules[f.Rule] == nil {
			t.Errorf("rule %s is not in Catalog", f.Rule)
		}
		if f.Message == "" || f.Path == "" || f.Line == 0 {
			t.Errorf("%s: incomplete finding %+v", f.Rule, f)
		}
		set[f.Rule] = true
	}
	var out []string
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return r, out
}

func TestBaseIsValid(t *testing.T) {
	r, got := findings(t, tBase, invoice.ProfileUnknown)
	if len(got) != 0 {
		t.Fatalf("base invoice: %v", r.Findings)
	}
}

// Snippets inserted in the base invoice.
const (
	allowance  = `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>10.00</ram:ActualAmount><ram:Reason>Discount</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>`
	breakdownS = `<ram:ApplicableTradeTax><ram:CalculatedAmount>19.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>`
	lineTax    = `<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>`
	tSellerVAT = `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration>`
	buyerAddr  = `<ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress>`
	settlStart = "<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>\n"
	sumStart   = "<ram:SpecifiedTradeSettlementHeaderMonetarySummation>\n"
)

// zeroRated returns the base invoice with VAT category cat at rate 0 on the
// line and in the breakdown, with the breakdown fields extra.
func zeroRated(t *testing.T, cat, extra string) string {
	return edit(t, tBase,
		lineTax, `<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>`+cat+`</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
		breakdownS, `<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>`+cat+`</ram:CategoryCode>`+extra+`<ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
		`<ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="EUR">0.00</ram:TaxTotalAmount>`,
		`<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>`, `<ram:GrandTotalAmount>100.00</ram:GrandTotalAmount>`,
		`<ram:DuePayableAmount>119.00</ram:DuePayableAmount>`, `<ram:DuePayableAmount>100.00</ram:DuePayableAmount>`)
}

type ruleCase struct {
	name    string
	doc     func(t *testing.T) string
	profile invoice.Profile
	want    string // space separated rules
}

func ruleCases() []ruleCase {
	e := func(pairs ...string) func(t *testing.T) string {
		return func(t *testing.T) string { return edit(t, tBase, pairs...) }
	}
	return slices.Concat([]ruleCase{
		// Business rules.
		{name: "BR-02 invoice number", doc: e("<ram:ID>INV-1</ram:ID>", "<ram:ID> </ram:ID>"), want: "BR-02"},
		{name: "BR-03 issue date format", doc: e(`format="102">20240115`, `format="610">20240115`), want: "BR-03"},
		{name: "BR-05 currency", doc: e(settlStart, ""), want: "BR-05"},
		{name: "BR-06 seller name", doc: e("<ram:Name>Seller</ram:Name>", ""), want: "BR-06"},
		{name: "BR-11 buyer country", doc: e(buyerAddr, `<ram:Name>Buyer</ram:Name><ram:PostalTradeAddress></ram:PostalTradeAddress>`), want: "BR-11"},
		{name: "BR-16 no line", doc: e(tLine, ""), want: "BR-16 BR-CO-10 BR-S-08"},
		{name: "BR-21 BR-25 line id and item name", doc: e("<ram:LineID>1</ram:LineID>", "<ram:LineID></ram:LineID>", "<ram:Name>Widget</ram:Name>", ""), want: "BR-21 BR-25"},
		{name: "BR-23 unit code", doc: e(` unitCode="C62"`, ""), want: "BR-23"},
		{name: "BR-27 negative net price", doc: e("<ram:ChargeAmount>10.00</ram:ChargeAmount>", "<ram:ChargeAmount>-10.00</ram:ChargeAmount>"), want: "BR-27"},
		{name: "BR-17 payee named as seller", doc: e(settlStart, settlStart+`<ram:PayeeTradeParty><ram:Name>Seller</ram:Name></ram:PayeeTradeParty>`), want: "BR-17"},
		{name: "BR-29 period order", doc: e(settlStart, settlStart+`<ram:BillingSpecifiedPeriod><ram:StartDateTime><udt:DateTimeString format="102">20240201</udt:DateTimeString></ram:StartDateTime><ram:EndDateTime><udt:DateTimeString format="102">20240101</udt:DateTimeString></ram:EndDateTime></ram:BillingSpecifiedPeriod>`), want: "BR-29"},
		{name: "BR-CO-19 empty period", doc: e(settlStart, settlStart+`<ram:BillingSpecifiedPeriod></ram:BillingSpecifiedPeriod>`), want: "BR-CO-19"},
		{name: "BR-31 BR-32 BR-33 allowance", doc: e(settlStart, settlStart+`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator></ram:SpecifiedTradeAllowanceCharge>`), want: "BR-31 BR-32 BR-33 BR-CO-11 BR-CO-21"},
		{name: "BR-36 charge", doc: e(settlStart, settlStart+`<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator><ram:Reason>Freight</ram:Reason><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>`), want: "BR-36 BR-CO-12"},
		{name: "BR-CO-04 line category", doc: e(lineTax, `<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>`), want: "BR-CO-04 BR-S-08"},
		{name: "BR-CO-09 VAT prefix", doc: e(">DE123456789<", ">XX123456789<"), want: "BR-CO-09"},
		{name: "BR-CO-10 line total", doc: e("<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis", "<ram:LineTotalAmount>100.01</ram:LineTotalAmount>\n<ram:TaxBasis"), want: "BR-CO-10 BR-CO-13"},
		{name: "BR-CO-11 to BR-CO-13 allowance totals", doc: e(settlStart, settlStart+allowance, breakdownS, `<ram:ApplicableTradeTax><ram:CalculatedAmount>17.10</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>90.00</ram:BasisAmount><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>`), want: "BR-CO-11 BR-CO-14"},
		{name: "allowance consistent", doc: e(settlStart, settlStart+allowance,
			breakdownS, `<ram:ApplicableTradeTax><ram:CalculatedAmount>17.10</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>90.00</ram:BasisAmount><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>`,
			sumStart+"<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasisTotalAmount>100.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">19.00</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>\n<ram:DuePayableAmount>119.00</ram:DuePayableAmount>",
			sumStart+"<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:AllowanceTotalAmount>10.00</ram:AllowanceTotalAmount>\n<ram:TaxBasisTotalAmount>90.00</ram:TaxBasisTotalAmount>\n<ram:TaxTotalAmount currencyID=\"EUR\">17.10</ram:TaxTotalAmount>\n<ram:GrandTotalAmount>107.10</ram:GrandTotalAmount>\n<ram:DuePayableAmount>107.10</ram:DuePayableAmount>"), want: ""},
		{name: "BR-CO-16 amount due", doc: e("<ram:DuePayableAmount>119.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>118.99</ram:DuePayableAmount>"), want: "BR-CO-16"},
		{name: "BR-CO-17 and BR-S-09 tolerance", doc: e("<ram:CalculatedAmount>19.00</ram:CalculatedAmount>", "<ram:CalculatedAmount>20.00</ram:CalculatedAmount>",
			`<ram:TaxTotalAmount currencyID="EUR">19.00</ram:TaxTotalAmount>`, `<ram:TaxTotalAmount currencyID="EUR">20.00</ram:TaxTotalAmount>`,
			"<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>", "<ram:GrandTotalAmount>120.00</ram:GrandTotalAmount>",
			"<ram:DuePayableAmount>119.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>120.00</ram:DuePayableAmount>"), want: "BR-S-09"},
		{name: "BR-CO-26 seller identifier", doc: e(tSellerVAT, ""), want: "BR-CO-26 BR-S-02"},
		{name: "BR-DEC amounts", doc: e("<ram:ChargeAmount>10.00</ram:ChargeAmount>", "<ram:ChargeAmount>10.0000</ram:ChargeAmount>", "<ram:DuePayableAmount>119.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>119.000</ram:DuePayableAmount>"), want: "BR-DEC-18"},
		{name: "BR-B-01 split payment outside Italy", doc: func(t *testing.T) string { return zeroRated(t, "B", "") }, want: "BR-B-01"},
		// Code lists.
		{name: "BR-CL-01 type code", doc: e("<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>"), want: "BR-CL-01"},
		{name: "BR-CL-04 currency normalized", doc: e("<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>", "<ram:InvoiceCurrencyCode> EUR </ram:InvoiceCurrencyCode>"), want: "BR-CO-15"},
		{name: "BR-CL-04 invalid currency", doc: e("<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>", "<ram:InvoiceCurrencyCode>EU R</ram:InvoiceCurrencyCode>"), want: "BR-CL-04 BR-CO-15"},
		{name: "BR-CL-14 Kosovo not in 1.3.17", doc: e(buyerAddr, `<ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>XK</ram:CountryID></ram:PostalTradeAddress>`), want: "BR-CL-14"},
		{name: "BR-CL-14 invalid country", doc: e(buyerAddr, `<ram:Name>Buyer</ram:Name><ram:PostalTradeAddress><ram:CountryID>ZZ</ram:CountryID></ram:PostalTradeAddress>`), want: "BR-CL-14"},
		{name: "BR-CL-23 unit", doc: e(`unitCode="C62"`, `unitCode="QQQ"`), want: "BR-CL-23"},
		{name: "BR-CL-25 EAS", doc: e("<ram:Name>Seller</ram:Name>", `<ram:Name>Seller</ram:Name><ram:URIUniversalCommunication><ram:URIID schemeID="9958">x</ram:URIID></ram:URIUniversalCommunication>`), want: "BR-CL-25"},
		{name: "BR-62 no scheme", doc: e("<ram:Name>Seller</ram:Name>", `<ram:Name>Seller</ram:Name><ram:URIUniversalCommunication><ram:URIID>x</ram:URIID></ram:URIUniversalCommunication>`), want: "BR-62"},
		{name: "BR-CL-22 VATEX case insensitive", doc: func(t *testing.T) string {
			return zeroRated(t, "E", "<ram:ExemptionReasonCode>vatex-eu-132</ram:ExemptionReasonCode>")
		}, want: ""},
		// VAT categories.
		{name: "BR-S-05 zero rate", doc: e(lineTax, `<ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>0</ram:RateApplicablePercent></ram:ApplicableTradeTax>`), want: "BR-S-05 BR-S-08"},
		{name: "BR-S-10 exemption", doc: e("<ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>\n"+tPaymentTerms, "<ram:CategoryCode>S</ram:CategoryCode><ram:ExemptionReason>x</ram:ExemptionReason><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:ApplicableTradeTax>\n"+tPaymentTerms), want: "BR-S-10"},
		{name: "BR-Z valid", doc: func(t *testing.T) string { return zeroRated(t, "Z", "") }, want: ""},
		{name: "BR-Z-10 exemption", doc: func(t *testing.T) string { return zeroRated(t, "Z", "<ram:ExemptionReason>x</ram:ExemptionReason>") }, want: "BR-Z-10"},
		{name: "BR-E-10 no exemption", doc: func(t *testing.T) string { return zeroRated(t, "E", "") }, want: "BR-E-10"},
		{name: "BR-E-08 taxable amount", doc: func(t *testing.T) string {
			return edit(t, zeroRated(t, "E", "<ram:ExemptionReasonCode>VATEX-EU-132</ram:ExemptionReasonCode>"),
				"<ram:BasisAmount>100.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode>", "<ram:BasisAmount>102.00</ram:BasisAmount><ram:CategoryCode>E</ram:CategoryCode>")
		}, want: "BR-E-08"},
		{name: "BR-AE-02 buyer identifier", doc: func(t *testing.T) string {
			return zeroRated(t, "AE", "<ram:ExemptionReason>Reverse charge</ram:ExemptionReason>")
		}, want: "BR-AE-02"},
		{name: "BR-IC-11 BR-IC-12 delivery", doc: func(t *testing.T) string {
			d := zeroRated(t, "K", "<ram:ExemptionReason>Intra-community</ram:ExemptionReason>")
			return edit(t, d, buyerAddr, buyerAddr+`<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR12345678901</ram:ID></ram:SpecifiedTaxRegistration>`)
		}, want: "BR-IC-11 BR-IC-12"},
		{name: "BR-G-02 seller VAT", doc: func(t *testing.T) string {
			d := zeroRated(t, "G", "<ram:ExemptionReason>Export</ram:ExemptionReason>")
			return edit(t, d, `schemeID="VA">DE123456789`, `schemeID="FC">201/113/40209`)
		}, want: "BR-CO-26 BR-G-02"},
		{name: "BR-O-02 BR-O-05 not subject", doc: func(t *testing.T) string {
			d := zeroRated(t, "O", "<ram:ExemptionReason>Not subject</ram:ExemptionReason>")
			return edit(t, d, "<ram:CategoryCode>O</ram:CategoryCode><ram:ExemptionReason>Not subject</ram:ExemptionReason><ram:RateApplicablePercent>0</ram:RateApplicablePercent>", "<ram:CategoryCode>O</ram:CategoryCode><ram:ExemptionReason>Not subject</ram:ExemptionReason>")
		}, want: "BR-O-02 BR-O-05"},
		{name: "BR-O-11 other categories", doc: e(breakdownS, breakdownS+`<ram:ApplicableTradeTax><ram:CalculatedAmount>0.00</ram:CalculatedAmount><ram:TypeCode>VAT</ram:TypeCode><ram:BasisAmount>0.00</ram:BasisAmount><ram:CategoryCode>O</ram:CategoryCode><ram:ExemptionReason>x</ram:ExemptionReason></ram:ApplicableTradeTax>`), want: "BR-O-01 BR-O-11 BR-O-12"},
		{name: "BR-IP-05 IGIC rate", doc: func(t *testing.T) string { return zeroRated(t, "L", "") }, want: "BR-IP-05"},
		{name: "BR-IG-10 IPSI exemption", doc: func(t *testing.T) string { return zeroRated(t, "M", "<ram:ExemptionReason>x</ram:ExemptionReason>") }, want: "BR-IG-10"},
		{name: "BR-S-01 missing breakdown", doc: e(breakdownS, ""), want: "BR-CO-14 BR-CO-18 BR-S-01"},
		// CII syntax.
		{name: "CII-SR-002 test indicator", doc: e("<rsm:ExchangedDocumentContext>\n", "<rsm:ExchangedDocumentContext>\n<ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>"), want: "CII-SR-002"},
		{name: "CII-SR-010 specification identifier count", doc: e("<ram:ID>urn:cen.eu:en16931:2017</ram:ID>", "<ram:ID>urn:cen.eu:en16931:2017</ram:ID><ram:ID>x</ram:ID>"), want: "CII-SR-010"},
		{name: "CII-DT-031 currencyID on amount", doc: e("<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis", "<ram:LineTotalAmount currencyID=\"EUR\">100.00</ram:LineTotalAmount>\n<ram:TaxBasis"), want: "CII-DT-031"},
		{name: "CII-DT-097 date format 102", doc: e("20240115", "20241315"), want: "CII-DT-097"},
		{name: "CII-SR-466 buyer contact", doc: e(buyerAddr, buyerAddr+"<ram:DefinedTradeContact><ram:PersonName>A</ram:PersonName><ram:DepartmentName>B</ram:DepartmentName></ram:DefinedTradeContact>"), want: "CII-SR-466"},
		{name: "CII-SR-469 payment references", doc: e(settlStart, settlStart+"<ram:PaymentReference>a</ram:PaymentReference><ram:PaymentReference>b</ram:PaymentReference>"), want: "CII-SR-469"},
		{name: "CII-SR-477 line total count", doc: e("<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis", "<ram:LineTotalAmount>100.00</ram:LineTotalAmount><ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis"), want: "CII-SR-477"},
		{name: "CII-DT-033 unit code without billed unit", doc: e(` unitCode="C62"`, "", "<ram:ChargeAmount>10.00</ram:ChargeAmount>", `<ram:ChargeAmount>10.00</ram:ChargeAmount><ram:BasisQuantity unitCode="C62">1</ram:BasisQuantity>`), want: "BR-23 CII-DT-033"},
		// Profiles.
		{name: "MINIMUM is skipped", doc: e("urn:cen.eu:en16931:2017", "urn:factur-x.eu:1p0:minimum", "<ram:Name>Seller</ram:Name>", ""), want: ""},
		{name: "BASIC WL is skipped", doc: e("urn:cen.eu:en16931:2017", "urn:factur-x.eu:1p0:basicwl", "<ram:Name>Seller</ram:Name>", ""), want: ""},
		{name: "XRECHNUNG severities", doc: e("urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", `unitCode="C62"`, `unitCode="QQQ"`), want: "BR-CL-23"},
		{name: "EXTENDED relaxations", doc: e("urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended",
			"<rsm:ExchangedDocumentContext>\n", "<rsm:ExchangedDocumentContext>\n<ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>",
			"<ram:DuePayableAmount>119.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>118.99</ram:DuePayableAmount>",
			"<ram:Name>Seller</ram:Name>", ""), want: "BR-06 BR-CO-16"},
	}, vatCases(), fullCases())
}

func TestRules(t *testing.T) {
	for _, tc := range ruleCases() {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.doc(t)
			r, got := findings(t, doc, tc.profile)
			if g := strings.Join(got, " "); g != tc.want {
				t.Errorf("got %q, want %q", g, tc.want)
				for _, f := range r.Findings {
					t.Logf("  %s", f)
				}
			}
		})
	}
}

// TestSeverities checks the official severities and the profile adjustments.
func TestSeverities(t *testing.T) {
	sev := func(doc string, rule string) rules.Severity {
		t.Helper()
		r, _ := findings(t, doc, invoice.ProfileUnknown)
		for _, f := range r.Findings {
			if f.Rule == rule {
				return f.Severity
			}
		}
		t.Fatalf("%s not reported", rule)
		return 0
	}
	contact := edit(t, tBase, buyerAddr, buyerAddr+"<ram:DefinedTradeContact><ram:PersonName>A</ram:PersonName><ram:DepartmentName>B</ram:DepartmentName></ram:DefinedTradeContact>", `unitCode="C62"`, `unitCode="QQQ"`)
	if s := sev(contact, "CII-SR-466"); s != rules.Warning {
		t.Errorf("CII-SR-466 (CEN): %v, want warning", s)
	}
	if s := sev(contact, "BR-CL-23"); s != rules.Error {
		t.Errorf("BR-CL-23 (CEN): %v, want error", s)
	}
	xr := edit(t, contact, "urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0")
	if s := sev(xr, "CII-SR-466"); s != rules.Error {
		t.Errorf("CII-SR-466 (XRECHNUNG): %v, want error", s)
	}
	if s := sev(xr, "BR-CL-23"); s != rules.Warning {
		t.Errorf("BR-CL-23 (XRECHNUNG): %v, want warning", s)
	}
	ext := edit(t, tBase, "urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended",
		"<ram:DuePayableAmount>119.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>118.99</ram:DuePayableAmount>",
		"<ram:Name>Seller</ram:Name>", "")
	if s := sev(ext, "BR-CO-16"); s != rules.Warning {
		t.Errorf("BR-CO-16 (EXTENDED): %v, want warning", s)
	}
	if s := sev(ext, "BR-06"); s != rules.Error {
		t.Errorf("BR-06 (EXTENDED): %v, want error", s)
	}
	// BR-27 and BR-28 are not among the CEN rules of the EXTENDED XSLT
	// (1.09.2): negative unit prices are allowed there.
	negative := edit(t, tBase, "<ram:NetPriceProductTradePrice><ram:ChargeAmount>10.00</ram:ChargeAmount>",
		`<ram:GrossPriceProductTradePrice><ram:ChargeAmount>-10.00</ram:ChargeAmount></ram:GrossPriceProductTradePrice><ram:NetPriceProductTradePrice><ram:ChargeAmount>-10.00</ram:ChargeAmount>`)
	for _, id := range []string{"BR-27", "BR-28"} {
		if s := sev(negative, id); s != rules.Error {
			t.Errorf("%s (CEN): %v, want error", id, s)
		}
	}
	negExt := edit(t, negative, "urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended")
	r, _ := findings(t, negExt, invoice.ProfileUnknown)
	if errs := r.Errors(); len(errs) > 0 {
		t.Errorf("EXTENDED with negative prices: errors %v", errs)
	}
	for _, id := range []string{"BR-27", "BR-28"} {
		if s := sev(negExt, id); s != rules.Warning {
			t.Errorf("%s (EXTENDED): %v, want warning", id, s)
		}
	}
}

// TestMalformed checks that incomplete documents do not crash the checker.
func TestMalformed(t *testing.T) {
	docs := []string{
		`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`,
		tHeader + tFooter,
		tHeader + tLine + tFooter,
		tHeader + tSettlement + tFooter,
		strings.Replace(tBase, "<ram:SpecifiedLineTradeSettlement>", "<ram:SpecifiedLineTradeSettlement><ram:SpecifiedTradeAllowanceCharge><ram:CategoryTradeTax><ram:CategoryCode>S</ram:CategoryCode></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>", 1),
	}
	for i, d := range docs {
		r := check(t, []byte(d), invoice.ProfileUnknown)
		if i > 0 && len(r.Errors()) == 0 {
			t.Errorf("document %d: no error", i)
		}
	}
}

var ruleID = regexp.MustCompile(`"((?:BR|CII)-[A-Z0-9-]*[0-9])"`)

// TestCatalogComplete checks that the rule identifiers written in the
// sources are in Catalog, or in CTCFRCatalog for the rules of the
// EXTENDED-CTC-FR schematron, and that every rule of Catalog can be reported.
func TestCatalogComplete(t *testing.T) {
	inSource := map[string]bool{}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "catalog_gen.go" || f == "ctcfr_gen.go" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ruleID.FindAllStringSubmatch(string(data), -1) {
			inSource[m[1]] = true
		}
	}
	for _, r := range syntaxNots {
		inSource[r.Rule] = true
	}
	for _, r := range syntaxCounts {
		inSource[r.Rule] = true
	}
	// VAT category rules are built from a prefix and a number.
	for _, v := range vatCategories {
		if v.prefix == "" {
			continue
		}
		for _, n := range []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10"} {
			if _, ok := severity[v.prefix+n]; ok {
				inSource[v.prefix+n] = true
			}
		}
	}
	for id := range inSource {
		if _, ok := severity[id]; !ok && ctcfrRules[id] == nil {
			t.Errorf("%s is used but not in Catalog", id)
		}
	}
	seen := map[string]bool{}
	for _, r := range Catalog {
		if seen[r.ID] {
			t.Errorf("%s twice in Catalog", r.ID)
		}
		seen[r.ID] = true
		if !inSource[r.ID] {
			t.Errorf("%s is in Catalog but never reported", r.ID)
		}
		if r.Description == "" || (r.Severity != rules.Error && r.Severity != rules.Warning) {
			t.Errorf("%s: bad catalog entry %+v", r.ID, r)
		}
	}
	for id := range extendedRules {
		if !seen[id] {
			t.Errorf("EXTENDED rule %s is not in Catalog", id)
		}
	}
	for _, m := range xrechnungLevels {
		for id := range m {
			if !seen[id] {
				t.Errorf("XRechnung level of %s, which is not in Catalog", id)
			}
		}
	}
}

// TestNoAllocs checks that a valid invoice is checked without allocating.
func TestNoAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates")
	}
	root, err := xmltree.Parse([]byte(tBase))
	if err != nil {
		t.Fatal(err)
	}
	chk := &rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: &rules.Report{}, Source: rules.SourceEN16931}
	if n := testing.AllocsPerRun(100, func() { Check(chk) }); n != 0 {
		t.Errorf("%v allocations per run", n)
	}
}
