package de

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const base = "testdata/xrechnung.xml" // XRechnung 3.0 CIUS invoice without finding

func load(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newDoc returns the document Check runs on for a parsed root: the CII
// document, the UBL document with its CII view, or nil for another root.
func newDoc(root *xmltree.Node) *rules.Doc {
	switch {
	case root.Name == "CrossIndustryInvoice":
		return rules.NewDoc(root, invoice.ProfileUnknown)
	case ubl.IsUBL(root):
		return ubl.View(root, invoice.ProfileUnknown)
	}
	return nil
}

// catalogOf returns the catalog of the syntax of doc.
func catalogOf(doc *rules.Doc) []rules.RuleInfo {
	if doc.UBL != nil {
		return UBLCatalog
	}
	return Catalog
}

// checkDoc runs Check on doc.
func checkDoc(doc *rules.Doc) *rules.Report {
	r := &rules.Report{}
	Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceDE})
	return r
}

// check runs Check on a parsed CII document.
func check(root *xmltree.Node) *rules.Report {
	return checkDoc(rules.NewDoc(root, invoice.ProfileUnknown))
}

func run(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return check(root)
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
	aSpec        = "<ram:ID>" + CIUSID + "</ram:ID>"
	aTypeCode    = "<ram:TypeCode>380</ram:TypeCode>"
	aPMCode      = "<ram:TypeCode>58</ram:TypeCode>"
	aIBAN        = "<ram:IBANID>DE89370400440532013000</ram:IBANID>"
	aPayeeAcct   = "<ram:PayeePartyCreditorFinancialAccount>"
	aPMEnd       = "</ram:SpecifiedTradeSettlementPaymentMeans>"
	aCurrency    = "<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>"
	aSkonto      = "#SKONTO#TAGE=14#PROZENT=2.00#"
	aDescEnd     = "</ram:Description>"
	aSellerVAT   = `<ram:ID schemeID="VA">DE123456789</ram:ID>`
	aSellerGLN   = `<ram:GlobalID schemeID="0088">4000001000005</ram:GlobalID>`
	aShipToGLN   = `<ram:GlobalID schemeID="0088">4000001000098</ram:GlobalID>`
	aProductGTIN = `<ram:GlobalID schemeID="0160">4000001000111</ram:GlobalID>`
	aSellerURI   = `<ram:URIID schemeID="EM">rechnung@flottenservice.example</ram:URIID>`
	aProduct1    = "<ram:Name>Wartung Fahrzeugflotte</ram:Name>"
	aBuyerEnd    = "</ram:BuyerTradeParty>"
	aARDName     = "<ram:Name>Lieferschein</ram:Name>"
	aARDURI      = "<ram:URIID>https://flottenservice.example/lieferscheine/311</ram:URIID>"
	aAttachment  = `<ram:AttachmentBinaryObject mimeCode="application/pdf" filename="lieferschein-311.pdf">JVBERi0xLjQK</ram:AttachmentBinaryObject>`
	aARDEnd      = "</ram:AdditionalReferencedDocument>"
	aPerson      = "<ram:PersonName>Erika Beispiel</ram:PersonName>"
	aPhone       = "<ram:CompleteNumber>+49 30 1234567</ram:CompleteNumber>"
	aEmail       = "<ram:URIID>rechnung@flottenservice.example</ram:URIID>"
	aIssueDate   = `<udt:DateTimeString format="102">20261102</udt:DateTimeString>`
	aDelivery    = "ActualDeliverySupplyChainEvent>"
	aNetQty      = `<ram:BasisQuantity unitCode="HUR">1</ram:BasisQuantity>
        </ram:NetPriceProductTradePrice>`
	aLine1Sum = "<ram:LineTotalAmount>125.00</ram:LineTotalAmount>"
	aLineID2  = "<ram:LineID>2</ram:LineID>"
)

var (
	// directDebit turns the SEPA credit transfer of the base invoice into a
	// valid SEPA direct debit (59, BG-19 complete).
	directDebit = one(
		rep(aPMCode, elem("TypeCode", "59")),
		repAll("PayeePartyCreditorFinancialAccount>", "PayerPartyDebtorFinancialAccount>"),
		rep("DE89370400440532013000", "DE02120300000000202051"),
		before(aCurrency, elem("CreditorReferenceID", "DE98ZZZ09999999999")),
		after(aDescEnd, elem("DirectDebitMandateID", "MANDAT-0815")),
	)
	// extension declares the XRechnung extension.
	extension = one(rep(aSpec, "<ram:ID>"+ExtensionID+"</ram:ID>"))
	// cvdData adds valid commercial vehicle data, without declaring CVDID.
	cvdData = one(
		after(aProduct1, cva("clean")+classCode("CVD", "M1")),
		after(aBuyerEnd, contract+tender),
	)
	// cvd declares the CVD and adds valid commercial vehicle data.
	cvd = edits(one(rep(aSpec, "<ram:ID>"+CVDID+"</ram:ID>")), cvdData)
)

const (
	contract = "<ram:ContractReferencedDocument>" + "<ram:IssuerAssignedID>V-2026-77</ram:IssuerAssignedID>" + "</ram:ContractReferencedDocument>"
	tender   = "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>LOS-3</ram:IssuerAssignedID><ram:TypeCode>50</ram:TypeCode></ram:AdditionalReferencedDocument>"
	card     = "<ram:ApplicableTradeSettlementFinancialCard><ram:ID>1234</ram:ID></ram:ApplicableTradeSettlementFinancialCard>"
)

func cva(value string) string {
	return "<ram:ApplicableProductCharacteristic>" + elem("Description", "cva") + elem("Value", value) + "</ram:ApplicableProductCharacteristic>"
}

func classCode(listID, value string) string {
	return `<ram:DesignatedProductClassification><ram:ClassCode listID="` + listID + `">` + value + `</ram:ClassCode></ram:DesignatedProductClassification>`
}

func attachment(name string) string {
	return `<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A-2</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject mimeCode="application/pdf" filename="` + name + `">JVBERi0xLjQK</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`
}

type mutationCase struct {
	name  string
	edits []edit
	want  []string // exact set of rule identifiers reported; empty for no finding
}

// mutationCases returns the mutations of the base invoice. Every assertion of
// Catalog fails in at least one case (see TestCoverage).
func mutationCases() []mutationCase {
	return []mutationCase{
		{"valid CIUS", nil, nil},
		{"valid extension", extension, nil},
		{"valid CVD", cvd, nil},
		{"valid direct debit", directDebit, nil},

		// Root rules.
		{"BR-DE-30 no creditor identifier", edits(directDebit, one(remove(elem("CreditorReferenceID", "DE98ZZZ09999999999")))), ids("BR-DE-30")},
		{"BR-DE-30 creditor identifier only", one(before(aCurrency, elem("CreditorReferenceID", "DE98ZZZ09999999999"))), ids("BR-DE-23-b", "BR-DE-30", "BR-DE-31")},
		{"BR-DE-31 no debited account", edits(directDebit, one(repAll("IBANID>", "ProprietaryID>"))), ids("BR-DE-20", "BR-DE-31")},
		{"BR-DE-1 no payment means", one(repAll("SpecifiedTradeSettlementPaymentMeans>", "SpecifiedTradeSettlementPaymentMeansX>")), ids("BR-DE-1")},
		{"BR-DE-15 blank buyer reference", one(rep(elem("BuyerReference", "04011000-12345-03"), elem("BuyerReference", " "))), ids("BR-DE-15", "PEPPOL-EN16931-R008")},
		{"BR-DE-16 no seller VAT identifier", one(remove(aSellerVAT)), ids("BR-DE-16", "PEPPOL-EN16931-R008")},
		{"BR-DE-16 tax registration FC", one(rep(aSellerVAT, `<ram:ID schemeID=" FC ">12/345/67890</ram:ID>`)), nil},
		{"BR-DE-16 tax representative", one(remove(aSellerVAT), after(aBuyerEnd, "<ram:SellerTaxRepresentativeTradeParty><ram:Name>Vertreter</ram:Name></ram:SellerTaxRepresentativeTradeParty>")), ids("PEPPOL-EN16931-R008")},
		{"BR-DE-16 lines in category O", one(remove(aSellerVAT), repAll(elem("CategoryCode", "S"), elem("CategoryCode", "O"))), ids("PEPPOL-EN16931-R008")},
		// The string value of a formatted ram:CategoryTradeTax is not 'VAT'.
		{"BR-DE-16 formatted charge tax", one(remove(aSellerVAT), repAll(elem("CategoryCode", "S"), elem("CategoryCode", "O")),
			before("<ram:SpecifiedTradePaymentTerms>", "<ram:SpecifiedTradeAllowanceCharge><ram:CategoryTradeTax>\n<ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>")), ids("PEPPOL-EN16931-R008", "PEPPOL-EN16931-R043-1")},
		{"BR-DE-16 compact charge tax", one(remove(aSellerVAT), repAll(elem("CategoryCode", "S"), elem("CategoryCode", "O")),
			before("<ram:SpecifiedTradePaymentTerms>", "<ram:SpecifiedTradeAllowanceCharge><ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>"+
				"<ram:SpecifiedTradeAllowanceCharge><ram:CategoryTradeTax><ram:CategoryCode>S</ram:CategoryCode></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>")), ids("BR-DE-16", "PEPPOL-EN16931-R008", "PEPPOL-EN16931-R043-1")},
		{"BR-DE-17 type 325", one(rep(aTypeCode, elem("TypeCode", "325"))), ids("BR-DE-17")},
		{"BR-DE-17 type 877", one(rep(aTypeCode, elem("TypeCode", " 877 "))), nil},
		{"BR-DE-18 percentage without decimals", one(rep(aSkonto, "#SKONTO#TAGE=14#PROZENT=2#")), ids("BR-DE-18")},
		{"BR-DE-18 lower case", one(rep(aSkonto, "#skonto#TAGE=14#PROZENT=2.00#")), ids("BR-DE-18")},
		{"BR-DE-18 no final line break", one(rep(aSkonto+"\n"+aDescEnd, aSkonto+aDescEnd)), ids("BR-DE-18")},
		{"BR-DE-18 base amount", one(rep(aSkonto, "#SKONTO#TAGE=14#PROZENT=2.00#BASISBETRAG=-100.00#")), nil},
		{"BR-DE-18 two entries", one(rep(aSkonto, aSkonto+"\n  #SKONTO#TAGE=30#PROZENT=1.00#  ")), nil},
		{"BR-DE-22 duplicate file name", one(after(aARDEnd, attachment("lieferschein-311.pdf"))), ids("BR-DE-22")},
		{"BR-DE-22 distinct file names", one(after(aARDEnd, attachment("stundenzettel.pdf"))), nil},
		{"BR-DE-26 corrected invoice without reference", one(rep(aTypeCode, elem("TypeCode", "384"))), ids("BR-DE-26")},
		{"BR-DE-26 corrected invoice with reference", one(rep(aTypeCode, elem("TypeCode", "384")),
			after("</ram:SpecifiedTradeSettlementHeaderMonetarySummation>", `<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>RE-2026-0999</ram:IssuerAssignedID><ram:FormattedIssueDateTime><qdt:DateTimeString format="102">20261001</qdt:DateTimeString></ram:FormattedIssueDateTime></ram:InvoiceReferencedDocument>`)), nil},

		// BT-24.
		{"BR-DE-21 EN 16931", one(rep(aSpec, "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>")), ids("BR-DE-21")},
		{"BR-DE-21 XRechnung 2.3", one(rep(aSpec, "<ram:ID>urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_2.3</ram:ID>")), ids("BR-DE-21")},

		// Seller, buyer and deliver to.
		{"BR-DE-2 no seller contact", one(repAll("DefinedTradeContact>", "DefinedTradeContactX>")), ids("BR-DE-2")},
		{"BR-DE-3 blank seller city", one(rep(elem("CityName", "Berlin"), elem("CityName", ""))), ids("BR-DE-3", "PEPPOL-EN16931-R008")},
		{"BR-DE-4 no seller post code", one(remove(elem("PostcodeCode", "10115"))), ids("BR-DE-4")},
		{"BR-DE-5 no contact point", one(remove(aPerson)), ids("BR-DE-5")},
		{"BR-DE-5 department", one(rep(aPerson, elem("DepartmentName", "Buchhaltung"))), nil},
		{"BR-DE-6 no telephone", one(remove(aPhone)), ids("BR-DE-27", "BR-DE-6", "PEPPOL-EN16931-R008")},
		{"BR-DE-7 no email", one(remove(aEmail)), ids("BR-DE-28", "BR-DE-7", "PEPPOL-EN16931-R008")},
		{"BR-DE-27 two digits", one(rep(aPhone, elem("CompleteNumber", "+49 OO IO"))), ids("BR-DE-27")},
		{"BR-DE-28 one label", one(rep(aEmail, elem("URIID", "rechnung@flottenservice"))), ids("BR-DE-28")},
		{"BR-DE-28 two @", one(rep(aEmail, elem("URIID", "rechnung@@flottenservice.example"))), ids("BR-DE-28")},
		{"BR-DE-28 space", one(rep(aEmail, elem("URIID", "erika beispiel@flottenservice.example"))), ids("BR-DE-28")},
		{"BR-DE-28 surrounding space", one(rep(aEmail, elem("URIID", "  erika.beispiel@flottenservice.example  "))), nil},
		{"BR-DE-8 blank buyer city", one(rep(elem("CityName", "Hamburg"), elem("CityName", "  "))), ids("BR-DE-8", "PEPPOL-EN16931-R008")},
		{"BR-DE-9 no buyer post code", one(remove(elem("PostcodeCode", "20095"))), ids("BR-DE-9")},
		{"BR-DE-10 no deliver to city", one(remove(elem("CityName", "Bremen"))), ids("BR-DE-10")},
		{"BR-DE-11 no deliver to post code", one(remove(elem("PostcodeCode", "28195"))), ids("BR-DE-11")},

		// Supporting documents and dates.
		{"BR-TMP-2 relative URL", one(rep(aARDURI, elem("URIID", "flottenservice.example/lieferscheine/311"))), ids("BR-TMP-2")},
		{"BR-TMP-2 one character scheme", one(rep(aARDURI, elem("URIID", "h://x"))), ids("BR-TMP-2")},
		{"BR-TMP-2 URN", one(rep(aARDURI, elem("URIID", "urn:isbn:9783161484100"))), nil},
		{"BR-TMP-4 two descriptions", one(after(aARDName, elem("Name", "Stundenzettel"))), ids("BR-TMP-4")},
		{"BR-TMP-5 two attachments", one(after(aAttachment, `<ram:AttachmentBinaryObject mimeCode="application/pdf" filename="b.pdf">JVBERi0xLjQK</ram:AttachmentBinaryObject>`)), ids("BR-TMP-5")},
		{"BR-TMP-7 format 610", one(rep(aIssueDate, `<udt:DateTimeString format="610">202611</udt:DateTimeString>`)), ids("BR-TMP-7")},
		{"BR-TMP-7 ISO date", one(rep(aIssueDate, `<udt:DateTimeString format="102">2026-11-02</udt:DateTimeString>`)), ids("BR-TMP-7")},
		{"BR-TMP-7 tax point date", one(after("<ram:CalculatedAmount>23.75</ram:CalculatedAmount>", `<ram:TaxPointDate><udt:DateString>20261030</udt:DateString></ram:TaxPointDate>`)), ids("BR-TMP-7")},
		{"BR-TMP-7 qdt:DateString is not checked", one(after("<ram:CalculatedAmount>23.75</ram:CalculatedAmount>", `<ram:TaxPointDate><qdt:DateString>2026</qdt:DateString></ram:TaxPointDate>`)), nil},

		// Payment means.
		{"BR-DE-19 IBAN checksum", one(rep(aIBAN, elem("IBANID", "DE89370400440532013001"))), ids("BR-DE-19")},
		{"BR-DE-19 IBAN with spaces", one(rep(aIBAN, elem("IBANID", "DE89 3704 0044 0532 0130 00"))), nil},
		{"BR-DE-19 credit transfer 30", one(rep(aPMCode, elem("TypeCode", "30")), rep(aIBAN, elem("IBANID", "1234567"))), nil},
		{"BR-DE-23-a no credit transfer", one(rep(aPMCode, elem("TypeCode", "30")), repAll("PayeePartyCreditorFinancialAccount>", "PayeeSpecifiedCreditorFinancialInstitution>")), ids("BR-DE-23-a")},
		{"BR-DE-23-b card", one(before(aPMEnd, card)), ids("BR-DE-23-b")},
		{"BR-DE-24-a/b card code", one(rep(aPMCode, elem("TypeCode", "48"))), ids("BR-DE-24-a", "BR-DE-24-b")},
		{"BR-DE-24 card", one(rep(aPMCode, elem("TypeCode", "54")), rep(aPayeeAcct, card+aPayeeAcct), repAll("PayeePartyCreditorFinancialAccount>", "X>")), nil},
		{"BR-DE-24-a no card", one(rep(aPMCode, elem("TypeCode", "55")), repAll("PayeePartyCreditorFinancialAccount>", "X>")), ids("BR-DE-24-a")},
		{"BR-DE-24-b card and account", one(rep(aPMCode, elem("TypeCode", "48")), before(aPMEnd, card)), ids("BR-DE-24-b")},
		{"BR-DE-20 IBAN checksum", edits(directDebit, one(rep("DE02120300000000202051", "DE02120300000000202052"))), ids("BR-DE-20")},
		{"BR-DE-25 direct debit code only", one(rep(aPMCode, elem("TypeCode", "59"))), ids("BR-DE-20", "BR-DE-25-a", "BR-DE-25-b", "PEPPOL-EN16931-R061")},
		{"BR-DE-25-b creditor institution", edits(directDebit, one(before(aPMEnd, "<ram:PayeeSpecifiedCreditorFinancialInstitution><ram:BICID>COBADEFFXXX</ram:BICID></ram:PayeeSpecifiedCreditorFinancialInstitution>"))), ids("BR-DE-25-b")},
		{"BR-DE-14 no VAT rate", one(repN(2, elem("RateApplicablePercent", "19"), "")), ids("BR-DE-14")},

		// Delivery dates and base quantities.
		{"BR-DE-TMP-32 no delivery date", one(repAll(aDelivery, "X"+aDelivery)), ids("BR-DE-TMP-32")},
		{"BR-DE-TMP-32 invoicing period", one(repAll(aDelivery, "X"+aDelivery),
			before("<ram:SpecifiedTradePaymentTerms>", `<ram:BillingSpecifiedPeriod><ram:StartDateTime><udt:DateTimeString format="102">20261001</udt:DateTimeString></ram:StartDateTime></ram:BillingSpecifiedPeriod>`)), nil},
		{"BR-TMP-3 different quantities", one(rep(aNetQty, `<ram:BasisQuantity unitCode="HUR">1.0</ram:BasisQuantity>
        </ram:NetPriceProductTradePrice>`)), ids("BR-TMP-3")},
		{"BR-TMP-3 different units", one(rep(aNetQty, `<ram:BasisQuantity unitCode="H87">1</ram:BasisQuantity>
        </ram:NetPriceProductTradePrice>`)), ids("BR-TMP-3", "PEPPOL-EN16931-R130")},
		{"BR-TMP-3 unit on the net price only", one(rep(`<ram:BasisQuantity unitCode="HUR">1</ram:BasisQuantity>`, `<ram:BasisQuantity>1</ram:BasisQuantity>`)), nil},

		// Extension.
		{"BR-DEX-15 sub line", edits(extension, one(after(aLineID2, elem("ParentLineID", "1")))), ids("BR-DEX-15")},
		{"BR-DEX-15 needs the extension", one(after(aLineID2, elem("ParentLineID", "1"))), nil},
		{"BR-DEX-04 unknown scheme", edits(extension, one(rep(aSellerGLN, `<ram:GlobalID schemeID="9999">4000001000005</ram:GlobalID>`))), ids("BR-DEX-04")},
		{"BR-DEX-04 XR01", edits(extension, one(rep(aSellerGLN, `<ram:GlobalID schemeID="XR01">4000001000005</ram:GlobalID>`))), nil},
		{"BR-DEX-04 empty scheme passes", edits(extension, one(rep(aSellerGLN, `<ram:GlobalID schemeID=" ">4000001000005</ram:GlobalID>`))), nil},
		{"BR-DEX-04 two schemes", edits(extension, one(rep(aSellerGLN, `<ram:GlobalID schemeID="0088 0060">4000001000005</ram:GlobalID>`))), ids("BR-DEX-04")},
		{"BR-DEX-04 needs the extension", one(rep(aSellerGLN, `<ram:GlobalID schemeID="9999">4000001000005</ram:GlobalID>`)), nil},
		{"BR-DEX-05 legal organization", edits(extension, one(after(aSellerGLN, `<ram:SpecifiedLegalOrganization><ram:ID schemeID="HRB">HRB 1234</ram:ID></ram:SpecifiedLegalOrganization>`))), ids("BR-DEX-05")},
		{"BR-DEX-05 tax registration is not checked", edits(extension, one(rep(aSellerVAT, `<ram:ID schemeID="XX">DE123456789</ram:ID>`))), ids("BR-DE-16")},
		{"BR-DEX-06 item identifier", edits(extension, one(rep(aProductGTIN, `<ram:GlobalID schemeID="GTIN">4000001000111</ram:GlobalID>`))), ids("BR-DEX-06")},
		{"BR-DEX-07 electronic address", edits(extension, one(rep(aSellerURI, `<ram:URIID schemeID="XX">rechnung@flottenservice.example</ram:URIID>`))), ids("BR-DEX-07")},
		{"BR-DEX-07 XR02", edits(extension, one(rep(aSellerURI, `<ram:URIID schemeID="XR02">rechnung@flottenservice.example</ram:URIID>`))), nil},
		{"BR-DEX-08 deliver to location", edits(extension, one(rep(aShipToGLN, `<ram:GlobalID schemeID="9999">4000001000098</ram:GlobalID>`))), ids("BR-DEX-08")},
		{"BR-DEX-01 MIME code", edits(extension, one(rep(`mimeCode="application/pdf"`, `mimeCode="application/zip"`))), ids("BR-DEX-01")},
		{"BR-DEX-01 XML attachment", edits(extension, one(rep(`mimeCode="application/pdf"`, `mimeCode="application/xml"`))), nil},
		{"BR-DEX-01 no MIME code", edits(extension, one(rep(`mimeCode="application/pdf" `, ""))), ids("BR-DEX-01")},

		// Commercial vehicle data.
		{"BR-DE-CVD-03 no CVD classification", edits(cvd, one(rep(`listID="CVD"`, `listID="AA"`))), ids("BR-DE-CVD-03", "BR-DE-CVD-06-b")},
		{"BR-DE-CVD-06-b two classifications", edits(cvd, one(after(aProduct1, classCode("CVD", "M2")))), ids("BR-DE-CVD-06-b")},
		{"BR-DE-CVD-06-a two attributes", edits(cvd, one(after(aProduct1, cva("other")))), ids("BR-DE-CVD-06-a")},
		{"BR-TMP-CVD-01 unknown list", edits(cvd, one(after(aProduct1, classCode("XYZ", "1")))), ids("BR-TMP-CVD-01")},
		{"BR-TMP-CVD-01 empty list passes", edits(cvd, one(after(aProduct1, classCode("", "1")))), nil},
		{"BR-DE-CVD-04 vehicle category", edits(cvd, one(rep(`listID="CVD">M1<`, `listID="CVD">L1<`))), ids("BR-DE-CVD-04")},
		{"BR-DE-CVD-05 attribute value", edits(cvd, one(rep(elem("Value", "clean"), elem("Value", "diesel")))), ids("BR-DE-CVD-05")},
		{"BR-DE-CVD-01 no contract", edits(cvd, one(remove(contract))), ids("BR-DE-CVD-01")},
		{"BR-DE-CVD-02 no tender", edits(cvd, one(remove(tender))), ids("BR-DE-CVD-02")},
		{"BR-DE-CVD-* need the CVD identifier", one(after(aProduct1, cva("diesel")+classCode("CVD", "L1"))), nil},

		// Peppol rules of the XRechnung schematron.
		{"PEPPOL-EN16931-R001 no business process", one(remove(aProcess)), ids("PEPPOL-EN16931-R001")},
		{"PEPPOL-EN16931-R001 blank business process", one(rep(aProcessID, elem("ID", " "))), ids("PEPPOL-EN16931-R008")},
		{"PEPPOL-EN16931-R005 VAT accounting currency of the invoice", one(before(aCurrency, elem("TaxCurrencyCode", "EUR"))), ids("PEPPOL-EN16931-R005", "PEPPOL-EN16931-R054")},
		{"PEPPOL-EN16931-R053 two totals in the invoice currency", one(after(aTaxTotal, aTaxTotal)), ids("PEPPOL-EN16931-R053")},
		{"PEPPOL-EN16931-R054 no total in the accounting currency", one(before(aCurrency, elem("TaxCurrencyCode", "USD"))), ids("PEPPOL-EN16931-R054", "PEPPOL-EN16931-R055")},
		{"PEPPOL-EN16931-R054 total in the accounting currency", one(before(aCurrency, elem("TaxCurrencyCode", "USD")), after(aTaxTotal, taxTotalUSD("31.00"))), nil},
		{"PEPPOL-EN16931-R054 total without accounting currency", one(after(aTaxTotal, taxTotalUSD("31.00"))), ids("PEPPOL-EN16931-R054")},
		{"PEPPOL-EN16931-R055 opposite signs", one(before(aCurrency, elem("TaxCurrencyCode", "USD")), after(aTaxTotal, taxTotalUSD("-31.00"))), ids("PEPPOL-EN16931-R055")},
		{"PEPPOL-EN16931-R010 no buyer electronic address", one(remove(aBuyerURIComm)), ids("PEPPOL-EN16931-R010")},
		{"PEPPOL-EN16931-R020 no seller electronic address", one(remove(aSellerURIComm)), ids("PEPPOL-EN16931-R020")},
		{"PEPPOL-EN16931-R041 percentage without base amount", one(before(aTerms, tradeAllowance("false", "10", "", "5.00"))), ids("PEPPOL-EN16931-R041")},
		{"PEPPOL-EN16931-R042 base amount without percentage", one(before(aTerms, tradeAllowance("false", "", "50.00", "5.00"))), ids("PEPPOL-EN16931-R042")},
		// R041 and R042 match first: R040 and R043-1 are not tested.
		{"PEPPOL-EN16931-R041 before R043-1", one(before(aTerms, tradeAllowance("1", "10", "", "5.00"))), ids("PEPPOL-EN16931-R041")},
		{"PEPPOL-EN16931-R040 amount", one(before(aTerms, tradeAllowance("false", "10", "50.00", "6.00"))), ids("PEPPOL-EN16931-R040")},
		{"PEPPOL-EN16931-R040 within the slack", one(before(aTerms, tradeAllowance("false", "10", "50.00", "5.02"))), nil},
		{"PEPPOL-EN16931-R040 HUF slack", edits(one(before(aTerms, tradeAllowance("false", "10", "50.00", "5.40"))), huf), nil},
		{"PEPPOL-EN16931-R040 beyond the HUF slack", edits(one(before(aTerms, tradeAllowance("false", "10", "50.00", "5.60"))), huf), ids("PEPPOL-EN16931-R040")},
		{"PEPPOL-EN16931-R043-1 indicator 1", one(before(aTerms, tradeAllowance("1", "", "", "5.00"))), ids("PEPPOL-EN16931-R043-1")},
		{"PEPPOL-EN16931-R043-1 spaced indicator", one(before(aTerms, tradeAllowance(" true ", "", "", "5.00"))), nil},
		{"PEPPOL-EN16931-R043-1 line allowance", one(before(aLineSum, tradeAllowance("0", "", "", "0.00"))), ids("PEPPOL-EN16931-R043-1")},
		{"PEPPOL-EN16931-R043-2 price allowance", one(before(aGrossEnd, priceAllowance("1", "0.00"))), ids("PEPPOL-EN16931-R043-2", "PEPPOL-EN16931-R044")},
		{"PEPPOL-EN16931-R044 charge on the price", one(before(aGrossEnd, priceAllowance("true", "0.00"))), ids("PEPPOL-EN16931-R044")},
		// R044 compares the indicator with 'false' as it is written.
		{"PEPPOL-EN16931-R044 spaced indicator", one(before(aGrossEnd, priceAllowance(" false ", "0.00"))), ids("PEPPOL-EN16931-R044")},
		{"PEPPOL-EN16931-R046 net price", one(before(aGrossEnd, priceAllowance("false", "1.00"))), ids("PEPPOL-EN16931-R046")},
		{"PEPPOL-EN16931-R046 gross price minus allowance", one(rep(elem("ChargeAmount", "12.50"), elem("ChargeAmount", "13.50")), before(aGrossEnd, priceAllowance("false", "1.00"))), nil},
		// The net price of line 2 becomes a gross price.
		{"PEPPOL-EN16931-R046 no net price", one(repN(2, "<ram:NetPriceProductTradePrice>", "<ram:GrossPriceProductTradePrice>"), repN(2, "</ram:NetPriceProductTradePrice>", "</ram:GrossPriceProductTradePrice>")), ids("PEPPOL-EN16931-R046", "PEPPOL-EN16931-R120")},
		{"PEPPOL-EN16931-R110 line period starts before", one(before(aTerms, billingPeriod("20261001", "20261031")), before(aLineSum, billingPeriod("20260930", "20261031"))), ids("PEPPOL-EN16931-R110")},
		{"PEPPOL-EN16931-R111 line period ends after", one(before(aTerms, billingPeriod("20261001", "20261031")), before(aLineSum, billingPeriod("20261001", "20261101"))), ids("PEPPOL-EN16931-R111")},
		{"PEPPOL-EN16931-R110 needs an invoice start date", one(before(aTerms, billingPeriod("", "20261031")), before(aLineSum, billingPeriod("20260901", "20261031"))), nil},
		{"PEPPOL-EN16931-R120 line net amount", one(rep(aQty1, `<ram:BilledQuantity unitCode="HUR">11</ram:BilledQuantity>`)), ids("PEPPOL-EN16931-R120")},
		{"PEPPOL-EN16931-R120 within the slack", one(repAll(elem("ChargeAmount", "12.50"), elem("ChargeAmount", "12.502"))), nil},
		{"PEPPOL-EN16931-R120 line allowance", one(before(aLineSum, tradeAllowance("false", "", "", "5.00"))), ids("PEPPOL-EN16931-R120")},
		{"PEPPOL-EN16931-R120 base quantity of the gross price", one(repAll(elem("ChargeAmount", "12.50"), elem("ChargeAmount", "25.00")), rep(aQtyHUR, `<ram:BasisQuantity unitCode="HUR">2</ram:BasisQuantity>`), rep(aNetQty, "</ram:NetPriceProductTradePrice>")), nil},
		// A zero base quantity counts as 1 in R120.
		{"PEPPOL-EN16931-R121 zero base quantity", one(repAll(aQtyHUR, `<ram:BasisQuantity unitCode="HUR">0</ram:BasisQuantity>`)), ids("PEPPOL-EN16931-R121")},
		{"PEPPOL-EN16931-R101 line object", one(after(aLineSumEnd, lineObject("916"))), ids("PEPPOL-EN16931-R101")},
		{"PEPPOL-EN16931-R101 type code 130", one(after(aLineSumEnd, lineObject("130"))), nil},
		// The delivery becomes an empty element, its content a comment; the
		// invoicing period replaces the delivery date (BR-DE-TMP-32).
		{"PEPPOL-EN16931-R008 empty delivery", one(rep("<ram:ApplicableHeaderTradeDelivery>", "<ram:ApplicableHeaderTradeDelivery/><!--"), rep("</ram:ApplicableHeaderTradeDelivery>", "-->"),
			before(aTerms, billingPeriod("20261001", "20261031"))), nil},
	}
}

// Anchors of the base invoice for the Peppol rules.
const (
	aProcessID     = "<ram:ID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</ram:ID>"
	aProcess       = "<ram:BusinessProcessSpecifiedDocumentContextParameter>\n      " + aProcessID + "\n    </ram:BusinessProcessSpecifiedDocumentContextParameter>"
	aTaxTotal      = `<ram:TaxTotalAmount currencyID="EUR">29.35</ram:TaxTotalAmount>`
	aBuyerURIComm  = "<ram:URIUniversalCommunication>\n          <ram:URIID schemeID=\"EM\">eingang@amt.example</ram:URIID>\n        </ram:URIUniversalCommunication>"
	aSellerURIComm = "<ram:URIUniversalCommunication>\n          " + aSellerURI + "\n        </ram:URIUniversalCommunication>"
	aTerms         = "<ram:SpecifiedTradePaymentTerms>"
	aGrossEnd      = "</ram:GrossPriceProductTradePrice>"
	aLineSum       = "<ram:SpecifiedTradeSettlementLineMonetarySummation>"
	aLineSumEnd    = "</ram:SpecifiedTradeSettlementLineMonetarySummation>"
	aQty1          = `<ram:BilledQuantity unitCode="HUR">10</ram:BilledQuantity>`
	aQtyHUR        = `<ram:BasisQuantity unitCode="HUR">1</ram:BasisQuantity>`
)

// huf turns the invoice currency into the Hungarian forint, whose slack is
// 0.5 in the Peppol rules.
var huf = one(rep(aCurrency, elem("InvoiceCurrencyCode", "HUF")), repAll(`currencyID="EUR"`, `currencyID="HUF"`))

// tradeAllowance returns an allowance or charge of the document or of a line
// with the indicator, the optional percentage and base amount, and the
// amount.
func tradeAllowance(indicator, percent, base, amount string) string {
	s := "<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>" + indicator + "</udt:Indicator></ram:ChargeIndicator>"
	if percent != "" {
		s += elem("CalculationPercent", percent)
	}
	if base != "" {
		s += elem("BasisAmount", base)
	}
	return s + elem("ActualAmount", amount) + elem("Reason", "Rabatt") +
		"<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>19</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>"
}

// priceAllowance returns an allowance of the gross price.
func priceAllowance(indicator, amount string) string {
	return "<ram:AppliedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>" + indicator + "</udt:Indicator></ram:ChargeIndicator>" + elem("ActualAmount", amount) + "</ram:AppliedTradeAllowanceCharge>"
}

// billingPeriod returns a period with the start and end dates (CCYYMMDD); an
// empty date is left out.
func billingPeriod(start, end string) string {
	s := "<ram:BillingSpecifiedPeriod>"
	if start != "" {
		s += `<ram:StartDateTime><udt:DateTimeString format="102">` + start + "</udt:DateTimeString></ram:StartDateTime>"
	}
	if end != "" {
		s += `<ram:EndDateTime><udt:DateTimeString format="102">` + end + "</udt:DateTimeString></ram:EndDateTime>"
	}
	return s + "</ram:BillingSpecifiedPeriod>"
}

// lineObject returns a referenced document of a line with the type code.
func lineObject(code string) string {
	return "<ram:AdditionalReferencedDocument>" + elem("IssuerAssignedID", "OBJ-1") + elem("TypeCode", code) + "</ram:AdditionalReferencedDocument>"
}

// taxTotalUSD returns an invoice total VAT amount in US dollars.
func taxTotalUSD(amount string) string {
	return `<ram:TaxTotalAmount currencyID="USD">` + amount + "</ram:TaxTotalAmount>"
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
			checkFindings(t, r, Catalog)
		})
	}
}

// checkFindings verifies that every finding is in catalog, with its
// severity, and located.
func checkFindings(t *testing.T, r *rules.Report, catalog []rules.RuleInfo) {
	t.Helper()
	for _, f := range r.Findings {
		info, ok := catalogEntry(catalog, f.Rule)
		if !ok {
			t.Errorf("rule %s is not in the catalog", f.Rule)
			continue
		}
		if f.Severity != info.Severity || f.Source != rules.SourceDE || f.Message == "" || f.Path == "" {
			t.Errorf("bad finding %+v", f)
		}
	}
}

func catalogEntry(catalog []rules.RuleInfo, id string) (rules.RuleInfo, bool) {
	for _, r := range catalog {
		if r.ID == id {
			return r, true
		}
	}
	return rules.RuleInfo{}, false
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
	}
	if len(Catalog) != 74 || sev[rules.Error] != 61 || sev[rules.Warning] != 12 || sev[rules.Info] != 1 {
		t.Errorf("%d assertions (%d errors, %d warnings, %d information), want 74 (61, 12, 1)", len(Catalog), sev[rules.Error], sev[rules.Warning], sev[rules.Info])
	}
}

func TestCheckIBAN(t *testing.T) {
	for _, c := range []struct {
		iban string
		ok   bool
	}{
		{"DE89370400440532013000", true},
		{" DE89 3704 0044 0532 0130 00 ", true},
		{"FR7630006000011234567890189", true},
		{"DE89370400440532013001", false},
		{"de89370400440532013000", false},
		{"DE8937040044053201300", false},
		{"DE89-3704-0044-0532-0130-00", false},
		{"", false},
		{"DE89", false},
	} {
		if got := checkIBAN(c.iban); got != c.ok {
			t.Errorf("checkIBAN(%q) = %v, want %v", c.iban, got, c.ok)
		}
	}
}

func TestLastToken(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"#SKONTO#TAGE=1#PROZENT=1.00#\n", "\n"},
		{"text\n#A#\n#B#x", "x"},
		{"no entry", "no entry"},
		{"##\n", "##\n"},
		{"###", ""},
		{"#a\n#", "#a\n#"},
	} {
		if got := lastToken(c.in); got != c.want {
			t.Errorf("lastToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEmpty(t *testing.T) {
	Check(nil)
	Check(&rules.Checker{})
	for _, doc := range []string{
		`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`,
		`<x/>`,
	} {
		checkFindings(t, run(t, []byte(doc)), Catalog)
	}
}

// eachInvoice calls fn for every XML file below dir whose root is a CII or a
// UBL invoice.
func eachInvoice(t *testing.T, dir string, fn func(path string, doc *rules.Doc)) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".xml") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		root, err := xmltree.Parse(data)
		if err != nil {
			return nil
		}
		if doc := newDoc(root); doc != nil {
			fn(path, doc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// corpusFailures lists the XRechnung invoices of the corpus, by file name,
// that break rules of this package: the rules they must report, and only
// those, and why.
var corpusFailures = map[string]struct {
	rules []string
	why   string
}{
	"XRechnung 722825203.XML": {rules: []string{"BR-DE-18"},
		why: "ZUGFeRD/corpus 82e886f, anonymised by scrambling the letters, including those of the cash " +
			"discount keywords: #SKONTO#TAGE=..#PROZENT=..# became #PZYKMU#TNIW=96#IQITDIB=6.71#"},
	"eicar.cii.xml": {rules: []string{"PEPPOL-EN16931-R008"},
		why: "test file of virus scanners written by Mustang: the country codes of the seller and of the buyer " +
			"are empty elements, which also break BR-09, BR-11 and BR-CL-14"},
}

// TestCorpus runs Check on the CII and UBL files of FX_CORPUS and
// FX_CORPUS_XML: the XRechnung invoices (BT-24 starting with CIUSID) have no
// error, except those of corpusFailures, and every reported rule is in the
// catalog of the syntax.
func TestCorpus(t *testing.T) {
	var dirs []string
	for _, env := range []string{"FX_CORPUS", "FX_CORPUS_XML"} {
		if d := os.Getenv(env); d != "" {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		t.Skip("FX_CORPUS and FX_CORPUS_XML are not set")
	}
	files, ublFiles, xr := 0, 0, 0
	for _, dir := range dirs {
		eachInvoice(t, dir, func(path string, doc *rules.Doc) {
			files++
			if doc.UBL != nil {
				ublFiles++
			}
			r := checkDoc(doc)
			checkFindings(t, r, catalogOf(doc))
			if !strings.HasPrefix(doc.SpecificationID(), CIUSID) {
				return
			}
			xr++
			if known, ok := corpusFailures[filepath.Base(path)]; ok {
				var got []string
				for _, f := range r.Errors() {
					if !slices.Contains(got, f.Rule) {
						got = append(got, f.Rule)
					}
				}
				slices.Sort(got)
				if !slices.Equal(got, known.rules) {
					t.Errorf("%s: errors %v, want %v (%s)", path, got, known.rules, known.why)
				}
			} else if !r.Valid() {
				t.Errorf("%s: %v", path, r.Errors())
			}
			if len(r.Findings) > 0 {
				t.Logf("%s: %v", filepath.Base(path), r.Rules())
			}
		})
	}
	t.Logf("%d files (%d UBL), %d XRechnung invoices", files, ublFiles, xr)
	if xr == 0 {
		t.Error("no XRechnung invoice in the corpus")
	}
}

// TestKoSIT runs Check on the CII and UBL instances of the KoSIT XRechnung
// test suite (FX_KOSIT, the root of xrechnung-testsuite): all are valid.
func TestKoSIT(t *testing.T) {
	dir := os.Getenv("FX_KOSIT")
	if dir == "" {
		t.Skip("FX_KOSIT is not set")
	}
	n, ublFiles := 0, 0
	eachInvoice(t, dir, func(path string, doc *rules.Doc) {
		n++
		if doc.UBL != nil {
			ublFiles++
		}
		r := checkDoc(doc)
		checkFindings(t, r, catalogOf(doc))
		if !r.Valid() {
			t.Errorf("%s: %v", path, r.Errors())
		}
		if len(r.Findings) > 0 {
			t.Logf("%s: %v", filepath.Base(path), r.Rules())
		}
	})
	t.Logf("%d instances (%d UBL)", n, ublFiles)
	if n == 0 || ublFiles == 0 {
		t.Error("no CII or no UBL instance in FX_KOSIT")
	}
}

func benchmark(b *testing.B, data []byte) {
	root, err := xmltree.Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	b.ReportAllocs()
	for b.Loop() {
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceDE})
		if !r.Valid() {
			b.Fatal(r)
		}
	}
}

// BenchmarkCheck runs Check on XRECHNUNG_Einfach of the corpus (FX_CORPUS).
func BenchmarkCheck(b *testing.B) {
	dir := os.Getenv("FX_CORPUS")
	if dir == "" {
		b.Skip("FX_CORPUS is not set")
	}
	benchmark(b, []byte(load(b, filepath.Join(dir, "XML-Rechnung", "CII", "XRECHNUNG_Einfach.cii.xml"))))
}

// BenchmarkCheckTestdata runs Check on the base invoice of testdata.
func BenchmarkCheckTestdata(b *testing.B) {
	benchmark(b, []byte(load(b, base)))
}
