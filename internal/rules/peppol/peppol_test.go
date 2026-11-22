package peppol

import (
	"math"
	"math/big"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const base = "testdata/peppol.xml" // Peppol BIS Billing 3.0 CII invoice without finding

func load(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r, Source: rules.SourcePeppol})
	return r
}

// edit replaces one occurrence (nth, from 1) or every occurrence (nth = 0)
// of old by new. The edit fails the test when old is not found.
type edit struct {
	old, new string
	nth      int
}

func rep(old, new string) edit         { return edit{old, new, 1} }
func repN(n int, old, new string) edit { return edit{old, new, n} }
func repAll(old, new string) edit      { return edit{old, new, 0} }
func remove(old string) edit           { return edit{old, "", 1} }
func before(anchor, xml string) edit   { return edit{anchor, xml + anchor, 1} }
func after(anchor, xml string) edit    { return edit{anchor, anchor + xml, 1} }
func edits(e ...[]edit) []edit         { return slices.Concat(e...) }
func one(e ...edit) []edit             { return e }
func ids(id ...string) []string        { return id }
func elem(name, value string) string   { return "<ram:" + name + ">" + value + "</ram:" + name + ">" }

func apply(t testing.TB, doc string, es []edit) string {
	t.Helper()
	for _, e := range es {
		if e.nth == 0 {
			if !strings.Contains(doc, e.old) {
				t.Fatalf("edit: %q not found", e.old)
			}
			doc = strings.ReplaceAll(doc, e.old, e.new)
			continue
		}
		at, off := -1, 0
		for range e.nth {
			i := strings.Index(doc[off:], e.old)
			if i < 0 {
				t.Fatalf("edit: occurrence %d of %q not found", e.nth, e.old)
			}
			at, off = off+i, off+i+len(e.old)
		}
		doc = doc[:at] + e.new + doc[at+len(e.old):]
	}
	return doc
}

// Anchors of the base invoice.
const (
	aProcess       = "<ram:ID>" + ProcessBilling + "</ram:ID>"
	aSpec          = "<ram:ID>" + SpecificationID + "</ram:ID>"
	aTypeCode      = "<ram:TypeCode>380</ram:TypeCode>"
	aNoteContent   = "<ram:Content>Services and equipment for October 2026.</ram:Content>"
	aNoteEnd       = "</ram:IncludedNote>"
	aIssueDate     = `<udt:DateTimeString format="102">20261102</udt:DateTimeString>`
	aProduct1      = "<ram:Name>Network maintenance</ram:Name>"
	aLineAgreement = "<ram:SpecifiedLineTradeAgreement>"
	aLine1Sum      = "<ram:LineTotalAmount>750.00</ram:LineTotalAmount>"
	aStart         = `<udt:DateTimeString format="102">20261001</udt:DateTimeString>` // line 1, then invoice
	aLineEnd       = `<udt:DateTimeString format="102">20261015</udt:DateTimeString>`
	aBasisQuantity = `<ram:BasisQuantity unitCode="C62">1</ram:BasisQuantity>` // gross, then net price of line 2
	aPercent       = "<ram:CalculationPercent>5</ram:CalculationPercent>"
	aBasisAmount   = "<ram:BasisAmount>240.00</ram:BasisAmount>"
	aAllowanceCode = "<ram:ReasonCode>95</ram:ReasonCode>"
	aFalse         = "<udt:Indicator>false</udt:Indicator>"
	aTrue          = "<udt:Indicator>true</udt:Indicator>"
	aObjectCode    = "<ram:TypeCode>130</ram:TypeCode>" // line 2, then document level
	aLineSettleEnd = "</ram:SpecifiedLineTradeSettlement>"
	aBuyerRef      = "<ram:BuyerReference>PO-4512</ram:BuyerReference>"
	aSellerName    = "<ram:Name>Example Networks Ltd</ram:Name>"
	aSellerGLN     = `<ram:GlobalID schemeID="0088">5012345000015</ram:GlobalID>`
	aSellerURI     = `<ram:URIID schemeID="0088">5012345000015</ram:URIID>`
	aSellerVAT     = `<ram:ID schemeID="VA">GB123456789</ram:ID>`
	aSellerEnd     = "</ram:SellerTradeParty>"
	aCountry       = "<ram:CountryID>GB</ram:CountryID>" // seller, then buyer
	aStreet        = "<ram:LineOne>1 Example Street</ram:LineOne>"
	aCity          = "<ram:CityName>London</ram:CityName>"
	aPostcode      = "<ram:PostcodeCode>EC1A 1AA</ram:PostcodeCode>"
	aBuyerName     = "<ram:Name>Sample Retail plc</ram:Name>"
	aBuyerURI      = `<ram:URIID schemeID="0088">5012345000091</ram:URIID>`
	aBuyerCity     = "<ram:CityName>Manchester</ram:CityName>"
	aBuyerEnd      = "</ram:BuyerTradeParty>"
	aAgreementEnd  = "</ram:ApplicableHeaderTradeAgreement>"
	aMimeCode      = `mimeCode="application/pdf"`
	aPaymentRef    = "<ram:PaymentReference>INV-2026-0042</ram:PaymentReference>"
	aCurrency      = "<ram:InvoiceCurrencyCode>GBP</ram:InvoiceCurrencyCode>"
	aPMCode        = "<ram:TypeCode>30</ram:TypeCode>"
	aIBAN          = "<ram:IBANID>GB29NWBK60161331926819</ram:IBANID>"
	aPMEnd         = "</ram:SpecifiedTradeSettlementPaymentMeans>"
	aChargeCode    = "<ram:ReasonCode>FC</ram:ReasonCode>"
	aChargeReason  = "<ram:Reason>Freight</ram:Reason>"
	aTerms         = "<ram:SpecifiedTradePaymentTerms>"
	aTaxTotal      = `<ram:TaxTotalAmount currencyID="GBP">200.00</ram:TaxTotalAmount>`
	aDue           = "<ram:DuePayableAmount>1200.00</ram:DuePayableAmount>"
	aRate          = "<ram:RateApplicablePercent>20</ram:RateApplicablePercent>"
)

// object returns a document reference with a type code: 130 for an invoiced
// object, 50 for a project reference.
func object(code string) string {
	return "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>REF-" + code + "</ram:IssuerAssignedID>" + elem("TypeCode", code) + "</ram:AdditionalReferencedDocument>"
}

func taxTotal(currency, amount string) string {
	return `<ram:TaxTotalAmount currencyID="` + currency + `">` + amount + `</ram:TaxTotalAmount>`
}

// legalOrg returns a legal registration; scheme "" omits the attribute.
func legalOrg(scheme, id string) string {
	if scheme != "" {
		scheme = ` schemeID="` + scheme + `"`
	}
	return "<ram:SpecifiedLegalOrganization><ram:ID" + scheme + ">" + id + "</ram:ID></ram:SpecifiedLegalOrganization>"
}

func taxRegistration(scheme, id string) string {
	return `<ram:SpecifiedTaxRegistration><ram:ID schemeID="` + scheme + `">` + id + `</ram:ID></ram:SpecifiedTaxRegistration>`
}

func bic(id string) string {
	return "<ram:PayeeSpecifiedCreditorFinancialInstitution>" + elem("BICID", id) + "</ram:PayeeSpecifiedCreditorFinancialInstitution>"
}

func classCode(listID, version string) string {
	return `<ram:DesignatedProductClassification><ram:ClassCode listID="` + listID + `" listVersionID="` + version + `">1234</ram:ClassCode></ram:DesignatedProductClassification>`
}

// buyerID adds a buyer identifier (ram:GlobalID) in an ICD scheme.
func buyerID(scheme, id string) []edit {
	return one(before(aBuyerName, `<ram:GlobalID schemeID="`+scheme+`">`+id+`</ram:GlobalID>`))
}

// buyerURI replaces the electronic address of the buyer.
func buyerURI(scheme, id string) []edit {
	return one(rep(aBuyerURI, `<ram:URIID schemeID="`+scheme+`">`+id+`</ram:URIID>`))
}

func pm(code string) edit { return rep(aPMCode, elem("TypeCode", code)) }

func country(code string) edit { return rep(aCountry, elem("CountryID", code)) }

const (
	order     = "<ram:BuyerOrderReferencedDocument><ram:IssuerAssignedID>PO-1</ram:IssuerAssignedID></ram:BuyerOrderReferencedDocument>"
	lineOrder = "<ram:BuyerOrderReferencedDocument><ram:LineID>3</ram:LineID></ram:BuyerOrderReferencedDocument>"
	precedent = `<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>INV-2026-0041</ram:IssuerAssignedID></ram:InvoiceReferencedDocument>`
	taxRep    = "<ram:SellerTaxRepresentativeTradeParty><ram:Name>Representative BV</ram:Name><ram:PostalTradeAddress><ram:CityName>Utrecht</ram:CityName><ram:CountryID>NL</ram:CountryID></ram:PostalTradeAddress>" +
		`<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">NL123456789B01</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTaxRepresentativeTradeParty>`
)

var (
	// taxCurrency declares a VAT accounting currency, with its total.
	taxCurrency = one(after(aCurrency, elem("TaxCurrencyCode", "EUR")), after(aTaxTotal, taxTotal("EUR", "230.00")))
	// norway moves the seller to Norway, with its VAT identifier (schemeID
	// 'VAT', as the schematron expects) and the Foretaksregisteret mention.
	norway = one(rep(aSellerVAT, `<ram:ID schemeID="VAT">NO974760673MVA</ram:ID>`), before(aSellerEnd, taxRegistration("FC", "Foretaksregisteret")))
	// dkCustomer moves seller and buyer to Denmark, with the seller CVR
	// number; its payment means code 30 is not allowed in Denmark.
	dkCustomer = one(repAll(aCountry, elem("CountryID", "DK")), after(aSellerName, legalOrg("0184", "DK12345678")))
	// dkDomestic is a valid domestic Danish invoice.
	dkDomestic = edits(dkCustomer, one(pm("58")))
	// italy moves the seller to Italy.
	italy = one(country("IT"))
	// seAddress moves the seller to Sweden.
	seAddress = one(country("SE"))
	// seVAT moves the seller to Sweden, with its VAT identifier (schemeID
	// 'VAT') and Swedish VAT rates.
	seVAT = edits(seAddress, one(rep(aSellerVAT, `<ram:ID schemeID="VAT">SE556036079301</ram:ID>`), repAll(aRate, elem("RateApplicablePercent", "25"))))
	// nlSeller moves the seller to the Netherlands, with its KvK number.
	nlSeller = one(country("NL"), after(aSellerName, legalOrg("0106", "12345678")))
	// nlCustomer moves the buyer to the Netherlands too.
	nlCustomer = edits(nlSeller, one(country("NL")))
)

type mutationCase struct {
	name  string
	edits []edit
	want  []string // exact set of rule identifiers reported; empty for no finding
}

// mutationCases returns the mutations of the base invoice. Every assertion of
// Catalog fails in at least one case (see TestCoverage).
func mutationCases() []mutationCase {
	return []mutationCase{
		{"valid", nil, nil},
		{"valid profile 02", one(rep(aProcess, "<ram:ID>"+ProcessBillingWithResponse+"</ram:ID>")), nil},
		{"valid tax currency", taxCurrency, nil},
		{"valid Norway", norway, nil},
		{"valid Denmark", dkDomestic, nil},
		{"valid Italy", italy, nil},
		{"valid Sweden", seVAT, nil},
		{"valid Netherlands", nlCustomer, nil},

		// Business process and specification identifier.
		{"R001 no business process", one(repAll("BusinessProcessSpecifiedDocumentContextParameter>", "X>")), ids("PEPPOL-EN16931-R001", "PEPPOL-EN16931-R007")},
		{"R007 unknown profile", one(rep(aProcess, "<ram:ID>urn:fdc:peppol.eu:2017:poacc:billing:05:1.0</ram:ID>")), ids("PEPPOL-EN16931-R007")},
		{"R004 EN 16931", one(rep(aSpec, "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>")), ids("PEPPOL-EN16931-R004")},
		{"R004 double colon", one(rep(aSpec, "<ram:ID>"+SpecificationID+"::x</ram:ID>")), ids("PEPPOL-EN16931-R004")},
		{"R004 extension of the specification", one(rep(aSpec, "<ram:ID> "+SpecificationID+"#conformant#urn:example </ram:ID>")), nil},

		// Document level.
		{"R003 no buyer reference", one(remove(aBuyerRef)), ids("PEPPOL-EN16931-R003")},
		{"R003 order reference", one(remove(aBuyerRef), after(aBuyerEnd, order)), nil},
		{"R006 two invoiced objects", one(before(aAgreementEnd, object("130"))), ids("PEPPOL-EN16931-R006")},
		{"R080 two project references", one(before(aAgreementEnd, object("50")+object("50"))), ids("PEPPOL-EN16931-R080")},
		{"R080 one project reference", one(before(aAgreementEnd, object("50"))), nil},
		{"R002 two notes", one(after(aNoteEnd, "<ram:IncludedNote><ram:Content>Second note</ram:Content></ram:IncludedNote>")), ids("PEPPOL-EN16931-R002")},
		{"R002 note subject", one(after(aNoteContent, elem("SubjectCode", "AAI"))), ids("PEPPOL-EN16931-R002")},
		{"R010 no buyer electronic address", one(remove(aBuyerURI)), ids("PEPPOL-EN16931-R010")},
		{"R020 no seller electronic address", one(remove(aSellerURI)), ids("PEPPOL-EN16931-R020")},

		// Currencies and VAT totals.
		{"R005 tax currency equal to invoice currency", one(after(aCurrency, elem("TaxCurrencyCode", "GBP"))), ids("PEPPOL-EN16931-R005", "PEPPOL-EN16931-R054")},
		{"R053 two tax totals", one(after(aTaxTotal, aTaxTotal)), ids("PEPPOL-EN16931-R053")},
		{"R053 tax total in another currency", one(rep(aTaxTotal, taxTotal("EUR", "230.00"))), ids("PEPPOL-EN16931-R053", "PEPPOL-EN16931-R054")},
		{"R054 tax total without tax currency", one(after(aTaxTotal, taxTotal("EUR", "230.00"))), ids("PEPPOL-EN16931-R054")},
		{"R055 opposite signs", one(after(aCurrency, elem("TaxCurrencyCode", "EUR")), after(aTaxTotal, taxTotal("EUR", "-230.00"))), ids("PEPPOL-EN16931-R055")},

		// Allowances and charges.
		{"R041 percentage without base amount", one(remove(aBasisAmount)), ids("PEPPOL-EN16931-R041")},
		{"R042 base amount without percentage", one(remove(aPercent)), ids("PEPPOL-EN16931-R042")},
		{"R040 amount", one(rep(aPercent, elem("CalculationPercent", "6"))), ids("PEPPOL-EN16931-R040")},
		{"R040 tolerance", one(rep(aPercent, elem("CalculationPercent", "5.005"))), nil},
		{"R043 line allowance indicator", one(rep(aFalse, "<udt:Indicator>no</udt:Indicator>")), ids("PEPPOL-EN16931-R043", "PEPPOL-EN16931-R120")},
		{"R043 document charge indicator", one(rep(aTrue, "<udt:Indicator>1</udt:Indicator>")), ids("PEPPOL-EN16931-R043")},

		// Payment means.
		{"R061 direct debit without mandate", one(pm("59")), ids("PEPPOL-EN16931-R061")},
		{"R061 direct debit", one(pm("59"), after(aTerms, elem("DirectDebitMandateID", "M-1"))), nil},

		// Lines.
		{"R110 line period starts before the invoice period", one(rep(aStart, `<udt:DateTimeString format="102">20260930</udt:DateTimeString>`)), ids("PEPPOL-EN16931-R110")},
		{"R111 line period ends after the invoice period", one(rep(aLineEnd, `<udt:DateTimeString format="102">20261105</udt:DateTimeString>`)), ids("PEPPOL-EN16931-R111")},
		{"R110 no invoice period", one(rep(aStart, `<udt:DateTimeString format="102">20260930</udt:DateTimeString>`),
			repN(2, "<ram:BillingSpecifiedPeriod>", "<ram:X>"), repN(2, "</ram:BillingSpecifiedPeriod>", "</ram:X>")), nil},
		{"R120 line net amount", one(rep(aLine1Sum, elem("LineTotalAmount", "750.03"))), ids("PEPPOL-EN16931-R120")},
		{"R120 tolerance", one(rep(aLine1Sum, elem("LineTotalAmount", "750.02"))), nil},
		{"R100 two line objects", one(repN(2, aLineSettleEnd, object("130")+aLineSettleEnd)), ids("PEPPOL-EN16931-R100")},
		{"R101 line object type", one(rep(aObjectCode, elem("TypeCode", "916"))), ids("PEPPOL-EN16931-R101")},
		{"R121 zero base quantity", one(rep(aBasisQuantity, `<ram:BasisQuantity unitCode="C62">0</ram:BasisQuantity>`)), ids("PEPPOL-EN16931-R121")},
		{"R130 base quantity unit", one(repN(2, aBasisQuantity, `<ram:BasisQuantity unitCode="H87">1</ram:BasisQuantity>`)), ids("PEPPOL-EN16931-R130")},

		// Identifiers of the ICD schemes.
		{"COMMON-R040 GLN", one(rep(aSellerGLN, `<ram:GlobalID schemeID="0088">5012345000016</ram:GlobalID>`)), ids("PEPPOL-COMMON-R040")},
		{"COMMON-R040 GLN with spaces", one(rep(aSellerGLN, `<ram:GlobalID schemeID="0088"> 5012345000015 </ram:GlobalID>`)), nil},
		{"COMMON-R041 Norwegian organization number", buyerURI("0192", "974760674"), ids("PEPPOL-COMMON-R041")},
		{"COMMON-R041 valid", buyerURI("0192", "974760673"), nil},
		{"COMMON-R042 Danish CVR", buyerID("0184", "DK1234567"), ids("PEPPOL-COMMON-R042")},
		{"COMMON-R042 eight digits", buyerID("0184", "12345678"), nil},
		{"COMMON-R052 Danish SE", buyerID("0096", "123456789A"), ids("PEPPOL-COMMON-R052")},
		{"COMMON-R052 valid", buyerID("0096", "1234567890"), nil},
		{"COMMON-R053 Danish P", buyerID("0198", "12345678"), ids("PEPPOL-COMMON-R053")},
		{"COMMON-R053 valid", buyerID("0198", "DK12345678"), nil},
		{"COMMON-R043 Belgian enterprise number", buyerID("0208", "0123456748"), ids("PEPPOL-COMMON-R043")},
		{"COMMON-R043 valid", buyerID("0208", "0123456749"), nil},
		{"COMMON-R044 codice IPA", buyerID("0201", "AB12C"), ids("PEPPOL-COMMON-R044")},
		{"COMMON-R044 valid", buyerID("0201", "AB12CD"), nil},
		{"COMMON-R045 codice fiscale", buyerID("0210", "RSSMRA85T10A562"), ids("PEPPOL-COMMON-R045")},
		{"COMMON-R045 valid", buyerID("0210", "RSSMRA85T10A562S"), nil},
		{"COMMON-R046 codice fiscale address", buyerURI("9907", "123"), ids("PEPPOL-COMMON-R046", "PEPPOL-EN16931-CL008")},
		{"COMMON-R046 valid", buyerURI("9907", "12345678901"), ids("PEPPOL-EN16931-CL008")},
		{"COMMON-R046 URIID only", buyerID("9907", "123"), nil},
		{"COMMON-R047 partita IVA", buyerID("0211", "IT12345678901"), ids("PEPPOL-COMMON-R047")},
		{"COMMON-R047 valid", buyerID("0211", "IT12345678903"), nil},
		{"COMMON-R049 Swedish organization number", buyerID("0007", "5560360794"), ids("PEPPOL-COMMON-R049")},
		{"COMMON-R049 valid", buyerID("0007", "5560360793"), nil},
		{"COMMON-R050 ABN", buyerID("0151", "51824753557"), ids("PEPPOL-COMMON-R050")},
		{"COMMON-R050 valid", buyerID("0151", "51824753556"), nil},
		{"COMMON-R054 KvK", buyerID("0106", "1234567"), ids("PEPPOL-COMMON-R054")},
		{"COMMON-R055 OIN", buyerID("0190", "1234567890123456789"), ids("PEPPOL-COMMON-R055")},
		{"COMMON-R055 valid", buyerID("0190", "12345678901234567890"), nil},
		{"COMMON-R056-1 Dutch VAT address", buyerID("9944", "NL123456789"), ids("PEPPOL-COMMON-R056-1")},
		{"COMMON-R056-1 valid", buyerID("9944", "NL123456789B01"), nil},
		{"COMMON-R056-2 Dutch VAT identifier", one(rep(aSellerVAT, `<ram:ID schemeID="VA">NL123456789</ram:ID>`)), ids("PEPPOL-COMMON-R056-2")},
		{"COMMON-R056-2 valid", one(rep(aSellerVAT, `<ram:ID schemeID="VA">NL123456789B01</ram:ID>`)), nil},
		{"COMMON-R057 Philippine TIN", buyerID("0217", "12345678901"), ids("PEPPOL-COMMON-R057")},

		// Code lists and formats.
		{"CL001 MIME code", one(rep(aMimeCode, `mimeCode="application/zip"`)), ids("PEPPOL-EN16931-CL001")},
		{"CL002 allowance reason code", one(rep(aAllowanceCode, elem("ReasonCode", "999"))), ids("PEPPOL-EN16931-CL002")},
		{"CL003 charge reason code", one(rep(aChargeCode, elem("ReasonCode", "XYZ"))), ids("PEPPOL-EN16931-CL003")},
		{"CL007 currency", one(repAll("GBP", "ABC")), ids("PEPPOL-EN16931-CL007")},
		// The context of PEPPOL-EN16931-P0100 is ram:ExchangedDocument, which
		// a CII invoice does not have (its document is rsm:ExchangedDocument).
		{"P0100 type code", one(repAll("rsm:ExchangedDocument>", "ram:ExchangedDocument>"), rep(aTypeCode, elem("TypeCode", "325"))), ids("PEPPOL-EN16931-P0100")},
		{"P0100 does not apply to rsm:ExchangedDocument", one(rep(aTypeCode, elem("TypeCode", "325"))), nil},
		{"F001 ISO date", one(rep(aIssueDate, `<udt:DateTimeString format="102">2026-11-02</udt:DateTimeString>`)), ids("PEPPOL-EN16931-F001")},
		{"F001 format 610", one(rep(aIssueDate, `<udt:DateTimeString format="610">202611</udt:DateTimeString>`)), ids("PEPPOL-EN16931-F001")},
		{"F001 twentieth century", one(rep(aIssueDate, `<udt:DateTimeString format="102">19991102</udt:DateTimeString>`)), ids("PEPPOL-EN16931-F001")},
		{"CL008 electronic address scheme", one(rep(aSellerURI, `<ram:URIID schemeID="EM">billing@example.com</ram:URIID>`)), ids("PEPPOL-EN16931-CL008")},

		// Norway.
		{"NO-R-002 no Foretaksregisteret", one(country("NO")), ids("NO-R-002")},
		{"NO-R-002 Foretaksregisteret", one(country("NO"), before(aSellerEnd, taxRegistration("FC", "Foretaksregisteret"))), nil},
		{"NO-R-001 VAT identifier", edits(norway, one(rep("NO974760673MVA", "NO974760674MVA"))), ids("NO-R-001")},
		{"NO-R-001 no MVA", edits(norway, one(rep("NO974760673MVA", "NO974760673"))), ids("NO-R-001")},
		// $supplierCountry reads the identifiers whose schemeID is 'VAT', the
		// UBL value: a CII VAT identifier (schemeID 'VA') is not read.
		{"NO-R-* VA identifier", one(rep(aSellerVAT, `<ram:ID schemeID="VA">NO974760674MVA</ram:ID>`)), nil},

		// Denmark.
		{"DK-R-002 no legal registration", one(country("DK")), ids("DK-R-002")},
		{"DK-R-014 legal registration scheme", one(country("DK"), after(aSellerName, legalOrg("0096", "1234567890"))), ids("DK-R-014")},
		{"DK-R-004 reason", edits(dkDomestic, one(rep(aChargeCode, elem("ReasonCode", "ZZZ")))), ids("DK-R-004")},
		{"DK-R-004 tax category", edits(dkDomestic, one(rep(aChargeCode, elem("ReasonCode", "ZZZ")), rep(aChargeReason, elem("Reason", "1234")))), nil},
		{"DK-R-004 hashed reason", edits(dkDomestic, one(rep(aChargeCode, elem("ReasonCode", "ZZZ")), rep(aChargeReason, elem("Reason", "VAT#S")))), nil},
		{"DK-R-013 identifier without scheme", edits(dkDomestic, one(rep(aSellerGLN, "<ram:GlobalID>5012345000015</ram:GlobalID>"))), ids("DK-R-013")},
		{"DK-R-016 negative credit note", edits(dkDomestic, one(rep(aTypeCode, elem("TypeCode", "381")), rep(aDue, elem("DuePayableAmount", "-1200.00")))), ids("DK-R-016")},
		{"DK-R-005 payment means code", dkCustomer, ids("DK-R-005")},
		{"DK-R-006 no BIC", edits(dkCustomer, one(pm("31"))), ids("DK-R-006")},
		{"DK-R-006 BIC", edits(dkCustomer, one(pm("31"), before(aPMEnd, bic("NWBKGB2L")))), nil},
		{"DK-R-007 direct debit", edits(dkCustomer, one(pm("49"))), ids("DK-R-007", "PEPPOL-EN16931-R061")},
		// DK-R-007 looks for the mandate below the payment means, where CII
		// has none.
		{"DK-R-007 mandate in the payment terms", edits(dkCustomer, one(pm("49"), before(aCurrency, elem("CreditorReferenceID", "DK-CRED-1")),
			after(aTerms, elem("DirectDebitMandateID", "M-1")))), ids("DK-R-007")},
		{"DK-R-008 FIK reference", edits(dkCustomer, one(pm("50"))), ids("DK-R-008")},
		{"DK-R-008 valid", edits(dkCustomer, one(pm("50"), rep(aPaymentRef, elem("PaymentReference", "01#1234")), rep(aIBAN, elem("IBANID", "1234567")))), nil},
		{"DK-R-009 FIK reference length", edits(dkCustomer, one(pm("50"), rep(aPaymentRef, elem("PaymentReference", "04#123")), rep(aIBAN, elem("IBANID", "1234567")))), ids("DK-R-009")},
		{"DK-R-009 valid", edits(dkCustomer, one(pm("50"), rep(aPaymentRef, elem("PaymentReference", "04#1234567890123456")), rep(aIBAN, elem("IBANID", "1234567")))), nil},
		{"DK-R-010 giro reference", edits(dkCustomer, one(pm("93"))), ids("DK-R-010")},
		{"DK-R-011 giro reference length", edits(dkCustomer, one(pm("93"), rep(aPaymentRef, elem("PaymentReference", "71#123")), rep(aIBAN, elem("IBANID", "12345678")))), ids("DK-R-011")},
		{"DK-R-003 classification version", edits(dkDomestic, one(after(aProduct1, classCode("TST", "1.0")))), ids("DK-R-003")},
		{"DK-R-003 valid", edits(dkDomestic, one(after(aProduct1, classCode("TST", "19.05.01")))), nil},

		// Italy.
		{"IT-R-001 codice fiscale", edits(italy, one(rep(aSellerVAT, `<ram:ID schemeID="FC">rssmra85</ram:ID>`))), ids("IT-R-001")},
		{"IT-R-001 valid", edits(italy, one(rep(aSellerVAT, `<ram:ID schemeID="FC">RSSMRA85T10A562S</ram:ID>`))), nil},
		{"IT-R-002 no street", edits(italy, one(remove(aStreet))), ids("IT-R-002")},
		{"IT-R-003 no city", edits(italy, one(remove(aCity))), ids("IT-R-003")},
		{"IT-R-004 no post code", edits(italy, one(remove(aPostcode))), ids("IT-R-004")},

		// Sweden.
		{"SE-R-001 VAT identifier length", edits(seVAT, one(rep("SE556036079301", "SE5560360793"))), ids("SE-R-001")},
		{"SE-R-002 VAT identifier digits", edits(seVAT, one(rep("SE556036079301", "SE55603607930A"))), ids("SE-R-002")},
		{"SE-R-006 VAT rate", edits(seAddress, one(rep(aSellerVAT, `<ram:ID schemeID="VAT">SE556036079301</ram:ID>`))), ids("SE-R-006")},
		{"SE-R-003 organization number digits", edits(seAddress, one(after(aSellerName, legalOrg("", "556036079A")))), ids("SE-R-003", "SE-R-013")},
		{"SE-R-004 organization number length", edits(seAddress, one(after(aSellerName, legalOrg("", "556036079")))), ids("SE-R-004", "SE-R-013")},
		{"SE-R-013 organization number check digit", edits(seAddress, one(after(aSellerName, legalOrg("", "5560360794")))), ids("SE-R-013")},
		{"SE-R-005 F-skatt", edits(seAddress, one(after(aSellerName, legalOrg("0007", "5560360793")), before(aSellerEnd, taxRegistration("FC", "F-skatt")))), ids("SE-R-005")},
		{"SE-R-005 valid", edits(seAddress, one(after(aSellerName, legalOrg("0007", "5560360793")), before(aSellerEnd, taxRegistration("FC", "Godkänd för F-skatt")))), nil},
		{"SE-R-007 plusgiro digits", edits(seAddress, one(after(aIBAN, elem("ProprietaryID", "12A")), before(aPMEnd, bic("SE:PLUSGIRO")))), ids("SE-R-007")},
		{"SE-R-010 plusgiro length", edits(seAddress, one(after(aIBAN, elem("ProprietaryID", "123456789")), before(aPMEnd, bic("SE:PLUSGIRO")))), ids("SE-R-010")},
		{"SE-R-008 bankgiro digits", edits(seAddress, one(after(aIBAN, elem("ProprietaryID", "12A4567")), before(aPMEnd, bic("SE:BANKGIRO")))), ids("SE-R-008")},
		{"SE-R-009 bankgiro length", edits(seAddress, one(after(aIBAN, elem("ProprietaryID", "123")), before(aPMEnd, bic("SE:BANKGIRO")))), ids("SE-R-009")},
		{"SE-R-009 valid", edits(seAddress, one(after(aIBAN, elem("ProprietaryID", "1234567")), before(aPMEnd, bic("SE:BANKGIRO")))), nil},
		{"SE-R-011 FIK", edits(seAddress, one(pm("50"))), ids("SE-R-011")},
		{"SE-R-012 domestic cheque", edits(seAddress, one(pm("31"), country("SE"))), ids("SE-R-012")},
		{"SE-R-012 foreign buyer", edits(seAddress, one(pm("31"))), nil},

		// The Netherlands.
		{"NL-R-001 credit note without preceding invoice", edits(nlSeller, one(rep(aTypeCode, elem("TypeCode", "381")))), ids("NL-R-001")},
		{"NL-R-001 valid", edits(nlSeller, one(rep(aTypeCode, elem("TypeCode", "381")), before(aTerms, precedent))), nil},
		{"NL-R-002 no seller street", edits(nlSeller, one(remove(aStreet))), ids("NL-R-002")},
		{"NL-R-003 seller registration scheme", one(country("NL"), after(aSellerName, legalOrg("XX", "12345678"))), ids("NL-R-003")},
		{"NL-R-004 no buyer city", edits(nlCustomer, one(remove(aBuyerCity))), ids("NL-R-004")},
		{"NL-R-005 buyer registration scheme", edits(nlCustomer, one(after(aBuyerName, legalOrg("XX", "87654321")))), ids("NL-R-005")},
		{"NL-R-006 tax representative address", edits(nlSeller, one(after(aBuyerEnd, taxRep))), ids("NL-R-006")},
		{"NL-R-007 no payment means", edits(nlSeller, one(repAll("SpecifiedTradeSettlementPaymentMeans>", "X>"))), ids("NL-R-007")},
		{"NL-R-007 nothing due", edits(nlSeller, one(repAll("SpecifiedTradeSettlementPaymentMeans>", "X>"), rep(aDue, elem("DuePayableAmount", "0.00")))), nil},
		{"NL-R-008 payment means code", edits(nlCustomer, one(pm("31"))), ids("NL-R-008")},
		{"NL-R-009 line order reference", edits(nlSeller, one(after(aLineAgreement, lineOrder))), ids("NL-R-009")},
		{"NL-R-009 valid", edits(nlSeller, one(after(aLineAgreement, lineOrder), after(aBuyerEnd, order))), nil},
	}
}

func TestMutations(t *testing.T) {
	doc := load(t, base)
	for _, c := range mutationCases() {
		t.Run(c.name, func(t *testing.T) {
			r := run(t, []byte(apply(t, doc, c.edits)))
			got := r.Rules()
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("rules = %q, want %q\n%s", got, want, r)
			}
			checkFindings(t, r)
		})
	}
}

// checkFindings verifies that every finding is in Catalog, with its
// severity, and located.
func checkFindings(t *testing.T, r *rules.Report) {
	t.Helper()
	for _, f := range r.Findings {
		info, ok := byID()[f.Rule]
		if !ok {
			t.Errorf("rule %s is not in Catalog", f.Rule)
			continue
		}
		if f.Severity != info.Severity || f.Source != rules.SourcePeppol || f.Message == "" || f.Path == "" ||
			!strings.HasPrefix(f.Message, "["+f.Rule+"]-"+info.Description) {
			t.Errorf("bad finding %+v", f)
		}
	}
}

// TestCoverage checks that every assertion of Catalog fails in at least one
// mutation case.
func TestCoverage(t *testing.T) {
	failing := map[string]bool{}
	for _, c := range mutationCases() {
		for _, id := range c.want {
			failing[id] = true
		}
	}
	for _, r := range Catalog {
		if !failing[r.ID] {
			t.Errorf("no mutation case fails %s", r.ID)
		}
	}
}

func TestCatalog(t *testing.T) {
	seen := map[string]bool{}
	sev := map[rules.Severity]int{}
	for _, r := range Catalog {
		if seen[r.ID] {
			t.Errorf("duplicate %s", r.ID)
		}
		seen[r.ID] = true
		sev[r.Severity]++
		if r.Description == "" {
			t.Errorf("no description for %s", r.ID)
		}
		if byID()[r.ID] == nil || byID()[r.ID].Severity != r.Severity {
			t.Errorf("byID()[%s] is not the catalog entry", r.ID)
		}
	}
	if len(Catalog) != 90 || sev[rules.Error] != 73 || sev[rules.Warning] != 17 {
		t.Errorf("%d assertions (%d errors, %d warnings), want 90 (73, 17)", len(Catalog), sev[rules.Error], sev[rules.Warning])
	}
}

func TestEmpty(t *testing.T) {
	Check(nil)
	Check(&rules.Checker{})
	for _, doc := range []string{
		`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`,
		`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"><rsm:ExchangedDocumentContext/></rsm:CrossIndustryInvoice>`,
		`<x/>`,
	} {
		checkFindings(t, run(t, []byte(doc)))
	}
}

func TestCheckDigits(t *testing.T) {
	for _, c := range []struct {
		name string
		f    func(string) bool
		in   string
		want bool
	}{
		{"gln", gln, "5012345000015", true},
		{"gln", gln, "4000001000005", true},
		{"gln", gln, "5012345000016", false},
		{"gln", gln, "", false},
		{"mod11", mod11, "974760673", true},
		{"mod11", mod11, "974760674", false},
		{"mod11", mod11, "000000000", false},
		{"mod11", mod11, "", false},
		{"mod97BE", mod97BE, "0123456749", true},
		{"mod97BE", mod97BE, "0123456748", false},
		{"codiceIPA", codiceIPA, "AB12CD", true},
		{"codiceIPA", codiceIPA, "ab12cd", true},
		{"codiceIPA", codiceIPA, "AB12C", false},
		{"codiceIPA", codiceIPA, "AB-2CD", false},
		{"codiceFiscale", codiceFiscale, "RSSMRA85T10A562S", true},
		{"codiceFiscale", codiceFiscale, "12345678901", true},
		{"codiceFiscale", codiceFiscale, "RSSMRA85T10A562", false},
		{"codiceFiscale", codiceFiscale, "1234567890A", false},
		{"codiceFiscale", codiceFiscale, "RSSMRA8AT10A562S", false},
		{"partitaIVA", partitaIVA, "IT12345678903", true},
		{"partitaIVA", partitaIVA, "it12345678903", true},
		{"partitaIVA", partitaIVA, "IT12345678901", false},
		{"partitaIVA", partitaIVA, "IT1234567890", false},
		{"partitaIVA", partitaIVA, "IT-1234567890", false},
		{"partitaIVA", partitaIVA, "12345678901", true}, // no country prefix: not checked
		{"swedishOrgNumber", swedishOrgNumber, "5560360793", true},
		{"swedishOrgNumber", swedishOrgNumber, "5560360794", false},
		{"swedishOrgNumber", swedishOrgNumber, "556036079", false},
		{"swedishOrgNumber", swedishOrgNumber, "55603607a3", false},
		{"swedishOrgNumber", swedishOrgNumber, "", false},
		{"abn", abn, "51824753556", true},
		{"abn", abn, "51824753557", false},
		{"abn", abn, "5182475355", false},
		{"dutchVAT", dutchVAT, "NL123456789B01", true},
		{"dutchVAT", dutchVAT, "NL123456789", false},
	} {
		if got := c.f(c.in); got != c.want {
			t.Errorf("%s(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestDouble(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1", 1, true},
		{" -1.5 ", -1.5, true},
		{".5", 0.5, true},
		{"5.", 5, true},
		{"1e3", 1000, true},
		{"1E-2", 0.01, true},
		{"INF", math.Inf(1), true},
		{"-INF", math.Inf(-1), true},
		{"1e400", math.Inf(1), true},
		{"", 0, false},
		{".", 0, false},
		{"1e", 0, false},
		{"abc", 0, false},
		{"0x10", 0, false},
		{"Inf", 0, false},
		{"1,5", 0, false},
	} {
		got, ok := double(c.in)
		if ok != c.ok || ok && got != c.want {
			t.Errorf("double(%q) = %v, %v, want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	if v, ok := double("NaN"); !ok || !math.IsNaN(v) {
		t.Errorf("double(NaN) = %v, %v", v, ok)
	}
}

func TestSubstring(t *testing.T) {
	for _, c := range []struct {
		s        string
		start, n int
		want     string
	}{
		{"12345", 1, 2, "12"},
		{"12345", 0, 4, "123"},
		{"12345", 3, 12, "345"},
		{"12345", 0, 1, ""},
		{"12345", 6, 1, ""},
		{"é12", 1, 2, "é1"},
	} {
		if got := substring(c.s, c.start, c.n); got != c.want {
			t.Errorf("substring(%q, %d, %d) = %q, want %q", c.s, c.start, c.n, got, c.want)
		}
	}
	if got := substringFrom("NO974760673MVA", 3); got != "974760673MVA" {
		t.Errorf("substringFrom = %q", got)
	}
}

func TestRoundHalfCeil2(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"0.125", "0.13"},
		{"-0.125", "-0.12"},
		{"1.004", "1.00"},
		{"-1.006", "-1.01"},
		{"12", "12.00"},
	} {
		v, _ := new(big.Rat).SetString(c.in)
		if got := roundHalfCeil2(v).FloatString(2); got != c.want {
			t.Errorf("roundHalfCeil2(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func BenchmarkCheck(b *testing.B) {
	root, err := xmltree.Parse([]byte(load(b, base)))
	if err != nil {
		b.Fatal(err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	b.ReportAllocs()
	for b.Loop() {
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourcePeppol})
		if len(r.Findings) > 0 {
			b.Fatal(r)
		}
	}
}
