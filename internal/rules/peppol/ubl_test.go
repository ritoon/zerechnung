package peppol

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

const ublBase = "testdata/peppol.ubl.xml" // the invoice of base in UBL 2.1, without finding

// runUBL runs the Peppol rules on a UBL document.
func runUBL(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return checkUBLDoc(root)
}

// Anchors of the UBL invoice of base.
const (
	uCustomization  = "<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0</cbc:CustomizationID>"
	uProfileID      = "<cbc:ProfileID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</cbc:ProfileID>"
	uIssueDate      = "<cbc:IssueDate>2026-11-02</cbc:IssueDate>"
	uDueDate        = "<cbc:DueDate>2026-12-02</cbc:DueDate>"
	uTypeCode       = "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>"
	uNote           = "<cbc:Note>Services and equipment for October 2026.</cbc:Note>"
	uCurrency       = "<cbc:DocumentCurrencyCode>GBP</cbc:DocumentCurrencyCode>"
	uBuyerRef       = "<cbc:BuyerReference>PO-4512</cbc:BuyerReference>"
	uPeriodEnd      = "<cbc:EndDate>2026-10-31</cbc:EndDate>"
	uDocRefs        = "<cac:AdditionalDocumentReference>"
	uSellerEndpoint = `<cbc:EndpointID schemeID="0088">5012345000015</cbc:EndpointID>`
	uBuyerEndpoint  = `<cbc:EndpointID schemeID="0088">5012345000091</cbc:EndpointID>`
	uGB             = "<cbc:IdentificationCode>GB</cbc:IdentificationCode>" // seller, then buyer
	uSellerStreet   = "<cbc:StreetName>1 Example Street</cbc:StreetName>"
	uSellerCity     = "<cbc:CityName>London</cbc:CityName>"
	uBuyerStreet    = "<cbc:StreetName>2 Sample Road</cbc:StreetName>"
	uSellerVAT      = "<cbc:CompanyID>GB123456789</cbc:CompanyID>"
	uSellerLegal    = "<cbc:RegistrationName>Example Networks Ltd</cbc:RegistrationName>"
	uBuyerLegal     = "<cbc:RegistrationName>Sample Retail plc</cbc:RegistrationName>"
	uMeansCode      = "<cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>"
	uAccount        = "<cbc:ID>GB29NWBK60161331926819</cbc:ID>"
	uChargeTrue     = "<cbc:ChargeIndicator>true</cbc:ChargeIndicator>"
	uChargeFalse    = "<cbc:ChargeIndicator>false</cbc:ChargeIndicator>" // line allowance, then price allowance
	uChargeCode     = "<cbc:AllowanceChargeReasonCode>FC</cbc:AllowanceChargeReasonCode>"
	uAllowanceCode  = "<cbc:AllowanceChargeReasonCode>95</cbc:AllowanceChargeReasonCode>"
	uDocCharge      = `<cbc:Amount currencyID="GBP">22.00</cbc:Amount>`
	uPercent        = "<cbc:Percent>20</cbc:Percent>" // charge, tax subtotal, line 1, line 2
	uLine1Amount    = `<cbc:LineExtensionAmount currencyID="GBP">750.00</cbc:LineExtensionAmount>`
	uStartDate      = "<cbc:StartDate>2026-10-01</cbc:StartDate>"        // document, then line 1
	uObjectType     = "<cbc:DocumentTypeCode>130</cbc:DocumentTypeCode>" // document, then line 2
	uLineObject     = "<cac:DocumentReference>"
	uMultiplier     = "<cbc:MultiplierFactorNumeric>5</cbc:MultiplierFactorNumeric>"
	uLineDiscount   = `<cbc:Amount currencyID="GBP">12.00</cbc:Amount>`
	uLineBase       = `<cbc:BaseAmount currencyID="GBP">240.00</cbc:BaseAmount>`
	uBaseQuantity   = `<cbc:BaseQuantity unitCode="C62">1</cbc:BaseQuantity>`
	uPriceDiscount  = `<cbc:Amount currencyID="GBP">0.00</cbc:Amount>`
)

// sellerIn and buyerIn move the address of the seller or of the buyer to a
// country; buyerIn comes first in a list of edits.
func sellerIn(country string) edit {
	return repN(1, uGB, "<cbc:IdentificationCode>"+country+"</cbc:IdentificationCode>")
}

func buyerIn(country string) edit {
	return repN(2, uGB, "<cbc:IdentificationCode>"+country+"</cbc:IdentificationCode>")
}

// creditNote turns the invoice of base into a credit note.
func creditNote() []edit {
	return one(
		rep(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`, `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"`),
		rep("</Invoice>", "</CreditNote>"),
		rep(uTypeCode, "<cbc:CreditNoteTypeCode>381</cbc:CreditNoteTypeCode>"),
		remove(uDueDate),
		repAll("cac:InvoiceLine>", "cac:CreditNoteLine>"),
		repAll("cbc:InvoicedQuantity", "cbc:CreditedQuantity"),
	)
}

// Danish seller and buyer, the seller with its CVR number.
var denmark = one(buyerIn("DK"), sellerIn("DK"), after(uSellerLegal, `<cbc:CompanyID schemeID="0184">12345678</cbc:CompanyID>`))

// German seller and buyer, the seller with its contact.
var germany = one(buyerIn("DE"), sellerIn("DE"),
	after("</cac:PartyLegalEntity>", "<cac:Contact><cbc:Name>Accounts</cbc:Name><cbc:Telephone>+49 30 1234</cbc:Telephone><cbc:ElectronicMail>accounts@example.com</cbc:ElectronicMail></cac:Contact>"))

// Greek seller, by its VAT identifier (EL and a valid TIN).
var greece = one(sellerIn("GR"), rep(uSellerVAT, "<cbc:CompanyID>EL123456783</cbc:CompanyID>"))

// greekInvoice is the invoice of a Greek seller that meets the Greek rules:
// invoice number in six segments, names of the parties, TIN as electronic
// address, MARK number and invoice URL.
func greekInvoice() []edit {
	return edits(greece, one(
		rep("<cbc:ID>INV-2026-0042</cbc:ID>", "<cbc:ID>123456783|02/11/2026|0|1.1|A|42</cbc:ID>"),
		rep(uSellerEndpoint, `<cbc:EndpointID schemeID="9933">123456783</cbc:EndpointID>`),
		after("</cac:PartyIdentification>", "<cac:PartyName><cbc:Name>Example Networks</cbc:Name></cac:PartyName>"),
		after(uBuyerEndpoint, "<cac:PartyName><cbc:Name>Sample Retail</cbc:Name></cac:PartyName>"),
		before(uDocRefs, "<cac:AdditionalDocumentReference><cbc:ID>400001234567890</cbc:ID><cbc:DocumentDescription>##M.AR.K##</cbc:DocumentDescription></cac:AdditionalDocumentReference>"+
			"<cac:AdditionalDocumentReference><cbc:ID>1</cbc:ID><cbc:DocumentDescription>##INVOICE|URL##</cbc:DocumentDescription>"+
			"<cac:Attachment><cac:ExternalReference><cbc:URI>https://example.com/invoice/42</cbc:URI></cac:ExternalReference></cac:Attachment></cac:AdditionalDocumentReference>"),
	))
}

// Icelandic seller with its legal identifier (kennitala).
var iceland = one(sellerIn("IS"), after(uSellerLegal, `<cbc:CompanyID schemeID="0196">5012345678</cbc:CompanyID>`))

func eindagi(id string) edit {
	return before(uDocRefs, "<cac:AdditionalDocumentReference><cbc:ID>"+id+"</cbc:ID><cbc:DocumentDescription>EINDAGI</cbc:DocumentDescription></cac:AdditionalDocumentReference>")
}

// Slovak seller and buyer, the seller with its Slovak VAT identifier.
var slovakia = one(buyerIn("SK"), sellerIn("SK"), rep(uSellerVAT, "<cbc:CompanyID>SK2020273893</cbc:CompanyID>"))

// externalReference returns a document reference to an external file.
func externalReference(uri string) string {
	return "<cac:AdditionalDocumentReference><cbc:ID>TERMS</cbc:ID><cac:Attachment><cac:ExternalReference><cbc:URI>" + uri +
		"</cbc:URI></cac:ExternalReference></cac:Attachment></cac:AdditionalDocumentReference>"
}

func projectReference(id string) edit {
	return before(uDocRefs, "<cac:AdditionalDocumentReference><cbc:ID>"+id+"</cbc:ID><cbc:DocumentTypeCode>50</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>")
}

func ublMutationCases() []mutationCase {
	return []mutationCase{
		{"base", nil, nil},

		// Rules of the document.
		{"empty element", one(rep(uNote, "<cbc:Note> </cbc:Note>")), ids("PEPPOL-EN16931-R008")},
		{"no business process", one(remove(uProfileID)), ids("PEPPOL-EN16931-R001", "PEPPOL-EN16931-R007", "PEPPOL-EN16931-R276")},
		{"business process format", one(rep(uProfileID, "<cbc:ProfileID>urn:fdc:peppol.eu:2017:poacc:billing:1:1.0</cbc:ProfileID>")), ids("PEPPOL-EN16931-R007")},
		{"specification identifier", one(rep(uCustomization, "<cbc:CustomizationID>urn:cen.eu:en16931:2017</cbc:CustomizationID>")), ids("PEPPOL-EN16931-R004")},
		{"no buyer reference", one(remove(uBuyerRef)), ids("PEPPOL-EN16931-R003")},
		{"tax currency", one(after(uCurrency, "<cbc:TaxCurrencyCode>GBP</cbc:TaxCurrencyCode>")), ids("PEPPOL-EN16931-R005", "PEPPOL-EN16931-R054")},
		{"no electronic addresses", one(remove(uSellerEndpoint), remove(uBuyerEndpoint)),
			ids("PEPPOL-EN16931-R010", "PEPPOL-EN16931-R020", "PEPPOL-EN16931-R219", "PEPPOL-EN16931-R229")},
		{"credit note without electronic addresses", edits(creditNote(), one(remove(uSellerEndpoint), remove(uBuyerEndpoint))),
			ids("PEPPOL-EN16931-R010", "PEPPOL-EN16931-R020", "PEPPOL-EN16931-R150", "PEPPOL-EN16931-R160")},
		{"two type codes", one(after(uTypeCode, uTypeCode)), ids("PEPPOL-EN16931-R275")},
		{"allowance amount", one(rep(uLineDiscount, `<cbc:Amount currencyID="GBP">13.00</cbc:Amount>`)), ids("PEPPOL-EN16931-R040", "PEPPOL-EN16931-R120")},
		{"allowance without base", one(remove(uLineBase)), ids("PEPPOL-EN16931-R041")},
		{"allowance without percentage", one(remove(uMultiplier)), ids("PEPPOL-EN16931-R042")},
		{"charge indicator", one(rep(uChargeTrue, "<cbc:ChargeIndicator>yes</cbc:ChargeIndicator>")), ids("PEPPOL-EN16931-R043")},
		{"direct debit without mandate", one(rep(uMeansCode, "<cbc:PaymentMeansCode>49</cbc:PaymentMeansCode>")), ids("PEPPOL-EN16931-R061")},
		{"other currency", one(rep(uDocCharge, `<cbc:Amount currencyID="EUR">22.00</cbc:Amount>`)), ids("PEPPOL-EN16931-R051")},
		{"line period", one(repN(2, uStartDate, "<cbc:StartDate>2026-09-30</cbc:StartDate>")), ids("PEPPOL-EN16931-R110")},
		{"line amount", one(rep(uLine1Amount, `<cbc:LineExtensionAmount currencyID="GBP">751.00</cbc:LineExtensionAmount>`)), ids("PEPPOL-EN16931-R120")},
		{"base quantity", one(rep(uBaseQuantity, `<cbc:BaseQuantity unitCode="C62">0</cbc:BaseQuantity>`)), ids("PEPPOL-EN16931-R121")},
		{"two invoiced objects", one(before(uLineObject, "<cac:DocumentReference><cbc:ID>SN-1</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:DocumentReference>")), ids("PEPPOL-EN16931-R100")},
		{"invoiced object type", one(repN(2, uObjectType, "<cbc:DocumentTypeCode>50</cbc:DocumentTypeCode>")), ids("PEPPOL-EN16931-R101")},
		{"charge on price", one(repN(2, uChargeFalse, uChargeTrue)), ids("PEPPOL-EN16931-R044")},
		{"net price", one(rep(uPriceDiscount, `<cbc:Amount currencyID="GBP">1.00</cbc:Amount>`)), ids("PEPPOL-EN16931-R046")},
		{"base quantity unit", one(rep(uBaseQuantity, `<cbc:BaseQuantity unitCode="HUR">1</cbc:BaseQuantity>`)), ids("PEPPOL-EN16931-R130")},
		{"credit note", creditNote(), nil},
		{"credit note project references", edits(creditNote(), one(projectReference("P1"), projectReference("P2"))), ids("PEPPOL-EN16931-R080")},
		{"invoice project references", one(projectReference("P1"), projectReference("P2")), nil},

		// Code lists and formats.
		{"MIME code", one(rep(`mimeCode="application/pdf"`, `mimeCode="application/zip"`)), ids("PEPPOL-EN16931-CL001")},
		{"allowance reason code", one(rep(uAllowanceCode, "<cbc:AllowanceChargeReasonCode>FC</cbc:AllowanceChargeReasonCode>")), ids("PEPPOL-EN16931-CL002")},
		{"charge reason code", one(rep(uChargeCode, "<cbc:AllowanceChargeReasonCode>95</cbc:AllowanceChargeReasonCode>")), ids("PEPPOL-EN16931-CL003")},
		{"period description code", one(after(uPeriodEnd, "<cbc:DescriptionCode>1</cbc:DescriptionCode>")), ids("PEPPOL-EN16931-CL006")},
		{"unknown currency", one(rep(uDocCharge, `<cbc:Amount currencyID="ABC">22.00</cbc:Amount>`)), ids("PEPPOL-EN16931-CL007", "PEPPOL-EN16931-R051")},
		{"invoice type code", one(rep(uTypeCode, "<cbc:InvoiceTypeCode>999</cbc:InvoiceTypeCode>")), ids("PEPPOL-EN16931-P0100")},
		{"self-billed invoice outside Germany", one(rep(uTypeCode, "<cbc:InvoiceTypeCode>389</cbc:InvoiceTypeCode>")), ids("PEPPOL-EN16931-P0112")},
		{"self-billed invoice in Germany", one(rep(uTypeCode, "<cbc:InvoiceTypeCode>389</cbc:InvoiceTypeCode>"), buyerIn("DE"), sellerIn("DE")), ids("DE-R-002")},
		{"partial invoice in Germany", one(rep(uTypeCode, "<cbc:InvoiceTypeCode>326</cbc:InvoiceTypeCode>"), buyerIn("DE"), sellerIn("DE")),
			ids("DE-R-002")},
		{"credit note type code", edits(creditNote(), one(rep("<cbc:CreditNoteTypeCode>381<", "<cbc:CreditNoteTypeCode>380<"))), ids("PEPPOL-EN16931-P0101")},
		{"date format", one(rep(uIssueDate, "<cbc:IssueDate>2026-11-2</cbc:IssueDate>")), ids("PEPPOL-EN16931-F001")},
		{"electronic address scheme", one(rep(uBuyerEndpoint, `<cbc:EndpointID schemeID="9999">5012345000091</cbc:EndpointID>`)), ids("PEPPOL-EN16931-CL008")},
		{"exemption reason", one(repN(2, uPercent, uPercent+"<cbc:TaxExemptionReasonCode>VATEX-EU-IC</cbc:TaxExemptionReasonCode>")), ids("PEPPOL-EN16931-P0106")},

		// Identifiers.
		{"GLN", one(rep(uSellerEndpoint, `<cbc:EndpointID schemeID="0088">5012345000016</cbc:EndpointID>`)), ids("PEPPOL-COMMON-R040")},
		{"GLN of 12 digits", one(rep(uSellerEndpoint, `<cbc:EndpointID schemeID="0088">501234500019</cbc:EndpointID>`)), ids("PEPPOL-COMMON-R040")},
		{"Belgian enterprise number", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0208">2123456791</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R043")},
		{"valid Belgian enterprise number", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0208">0123456749</cbc:CompanyID>`)), nil},
		{"Belgian VAT number", one(rep(uSellerVAT, "<cbc:CompanyID>BE0123456748</cbc:CompanyID>")), ids("PEPPOL-COMMON-R062")},
		{"valid Belgian VAT number", one(rep(uSellerVAT, "<cbc:CompanyID> be0123456749 </cbc:CompanyID>")), nil},
		// The rules of the schemes 0088 to 0208 come before that of the
		// Belgian VAT number.
		{"Belgian VAT number with scheme 0096", one(rep(uSellerVAT, `<cbc:CompanyID schemeID="0096">BE0123456749</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R052")},
		{"Luxembourgish VAT number", one(rep(uSellerVAT, "<cbc:CompanyID>LU12345678</cbc:CompanyID>")), ids("PEPPOL-COMMON-R058")},
		{"valid Luxembourgish VAT number", one(rep(uSellerVAT, "<cbc:CompanyID>lu12345613</cbc:CompanyID>")), nil},
		// The rule of the scheme 0217 comes before that of the Luxembourgish
		// VAT number.
		{"Luxembourgish VAT number with scheme 0217", one(rep(uSellerVAT, `<cbc:CompanyID schemeID="0217">LU12345613</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R057")},
		{"Luxembourgish national identifier", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0240">19852012343</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R059")},
		{"valid Luxembourgish national identifier", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0240">19852012342</cbc:CompanyID>`)), nil},
		{"Norwegian organization number", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0192">123456789</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R041")},
		{"valid Norwegian organization number", one(after(uBuyerLegal, `<cbc:CompanyID schemeID="0192">123456785</cbc:CompanyID>`)), nil},
		{"Dutch VAT number", one(rep(uSellerVAT, "<cbc:CompanyID>NL123456789</cbc:CompanyID>")), ids("PEPPOL-COMMON-R056-2")},
		{"valid Dutch VAT number", one(rep(uSellerVAT, "<cbc:CompanyID> NL123456789B01 </cbc:CompanyID>")), nil},
		{"Italian fiscal code as endpoint", one(rep(uBuyerEndpoint, `<cbc:EndpointID schemeID="9907">ABC</cbc:EndpointID>`)), ids("PEPPOL-COMMON-R046", "PEPPOL-EN16931-CL008")},

		// National rules.
		// The country of the seller is that of its VAT identifier, its address
		// otherwise.
		{"Norway", one(sellerIn("NO"), rep(uSellerVAT, "<cbc:CompanyID>NO123456789</cbc:CompanyID>")), ids("NO-R-001", "NO-R-002")},
		{"Norway by address", one(sellerIn("NO")), nil},
		{"Sweden", one(sellerIn("SE"), rep(uSellerVAT, "<cbc:CompanyID>SE123456789X</cbc:CompanyID>")), ids("SE-R-001", "SE-R-002")},
		{"Sweden domestic", one(buyerIn("SE"), sellerIn("SE"), rep(uSellerVAT, "<cbc:CompanyID>SE556036079301</cbc:CompanyID>")), ids("SE-R-006")},
		{"Sweden domestic rates", one(buyerIn("SE"), sellerIn("SE"), rep(uSellerVAT, "<cbc:CompanyID>SE556036079301</cbc:CompanyID>"),
			repAll(uPercent, "<cbc:Percent>25</cbc:Percent>")), nil},
		{"Netherlands", one(sellerIn("NL"), after(uSellerLegal, `<cbc:CompanyID schemeID="0088">5012345000015</cbc:CompanyID>`)), ids("NL-R-003")},
		{"Italy", one(rep(uSellerVAT, "<cbc:CompanyID>IT12345678901</cbc:CompanyID>"), remove(uSellerCity)), ids("IT-R-003")},
		{"Denmark", one(sellerIn("DK")), ids("DK-R-002")},
		{"Denmark domestic", edits(denmark, one(
			after(uBuyerLegal, "<cbc:CompanyID>87654321</cbc:CompanyID>"),
			rep(uChargeCode, "<cbc:AllowanceChargeReasonCode>ZZZ</cbc:AllowanceChargeReasonCode>"),
			before("<cac:ClassifiedTaxCategory>", `<cac:CommodityClassification><cbc:ItemClassificationCode listID="TST" listVersionID="1.0">43211500</cbc:ItemClassificationCode></cac:CommodityClassification>`),
		)), ids("DK-R-003", "DK-R-004", "DK-R-005", "DK-R-017")},
		{"Denmark giro", edits(denmark, one(rep(uMeansCode, "<cbc:PaymentMeansCode>50</cbc:PaymentMeansCode>"))), ids("DK-R-008")},
		{"Denmark giro account", edits(denmark, one(rep(uMeansCode, "<cbc:PaymentMeansCode>50</cbc:PaymentMeansCode>"),
			rep(uAccount, "<cbc:ID>1234567</cbc:ID>"), rep("<cbc:PaymentID>INV-2026-0042</cbc:PaymentID>", "<cbc:PaymentID>01#123</cbc:PaymentID>"))), nil},
		{"Germany", edits(germany, one(rep(uMeansCode, "<cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>"))), nil},
		{"Germany IBAN", edits(germany, one(rep(uMeansCode, "<cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>"), rep(uAccount, "<cbc:ID>GB29NWBK60161331926818</cbc:ID>"))), ids("DE-R-019")},
		{"Germany card", edits(germany, one(rep(uMeansCode, "<cbc:PaymentMeansCode>48</cbc:PaymentMeansCode>"))), ids("DE-R-024-1", "DE-R-024-2")},
		{"Germany direct debit", edits(germany, one(rep(uMeansCode, "<cbc:PaymentMeansCode>59</cbc:PaymentMeansCode>"))),
			ids("DE-R-020", "DE-R-025-1", "DE-R-025-2", "PEPPOL-EN16931-R061")},
		{"Germany cash discount", edits(germany, one(after("</cac:PaymentMeans>", "<cac:PaymentTerms><cbc:Note>#SKONTO#TAGE=7#PROZENT=2.00#\n</cbc:Note></cac:PaymentTerms>"))), nil},
		{"Germany attachment URL", edits(germany, one(before(uDocRefs, externalReference("example.com/terms")))), ids("DE-R-T02")},
		{"Germany attachment URL with spaces", edits(germany, one(before(uDocRefs, externalReference(" https://example.com/terms ")))), nil},
		{"Germany cash discount without line break", edits(germany, one(after("</cac:PaymentMeans>", "<cac:PaymentTerms><cbc:Note>#SKONTO#TAGE=7#PROZENT=2.00#</cbc:Note></cac:PaymentTerms>"))), ids("DE-R-018")},
		{"Greece", greece, ids("GR-R-001-1", "GR-R-001-2", "GR-R-001-3", "GR-R-001-4", "GR-R-001-6", "GR-R-001-7",
			"GR-R-002", "GR-R-004-1", "GR-S-008-1", "GR-R-005", "GR-R-009")},
		{"Greece complete", greekInvoice(), nil},
		// GR-R-001-5, on the fourth segment of the invoice number, was
		// removed by release 3.1.0.
		{"Greece other document type", edits(greekInvoice(), one(rep("|0|1.1|A|", "|0|9.9|A|"))), nil},
		{"Greece domestic", edits(one(buyerIn("GR")), greekInvoice()), ids("GR-R-006", "GR-R-010")},
		{"Greece MARK", edits(greekInvoice(), one(rep("<cbc:ID>400001234567890</cbc:ID>", "<cbc:ID>0400001234567890</cbc:ID>"))), ids("GR-R-004-2")},
		{"Iceland without legal identifier", one(sellerIn("IS")), ids("IS-R-002")},
		{"Iceland", iceland, nil},
		{"Iceland type and address", edits(iceland, one(rep(uTypeCode, "<cbc:InvoiceTypeCode>383</cbc:InvoiceTypeCode>"), remove(uSellerStreet))), ids("IS-R-001", "IS-R-003")},
		{"Iceland buyer", edits(one(buyerIn("IS")), iceland), ids("IS-R-004")},
		{"Iceland buyer address", edits(one(buyerIn("IS"), remove(uBuyerStreet)), iceland), ids("IS-R-004", "IS-R-005")},
		{"Iceland claim account", edits(iceland, one(rep(uMeansCode, "<cbc:PaymentMeansCode>9</cbc:PaymentMeansCode>"))), ids("IS-R-006")},
		{"Iceland transfer account", edits(iceland, one(rep(uMeansCode, "<cbc:PaymentMeansCode>42</cbc:PaymentMeansCode>"))), ids("IS-R-007")},
		{"Iceland EINDAGI", edits(iceland, one(eindagi("2026-12-02"))), nil},
		{"Iceland EINDAGI before due date", edits(iceland, one(eindagi("2026-12-01"))), ids("IS-R-010")},
		{"Slovakia", slovakia, nil},
		{"Slovakia seller address", edits(slovakia, one(remove(uSellerStreet), remove(uSellerCity), remove("<cbc:PostalZone>EC1A 1AA</cbc:PostalZone>"))),
			ids("SK-R-002", "SK-R-003", "SK-R-004")},
		{"Slovakia buyer address", edits(slovakia, one(remove(uBuyerStreet), remove("<cbc:CityName>Manchester</cbc:CityName>"), remove("<cbc:PostalZone>M1 1AA</cbc:PostalZone>"))),
			ids("SK-R-005", "SK-R-006", "SK-R-007")},
		{"Slovakia foreign buyer", one(sellerIn("SK"), remove(uSellerStreet)), nil},
		{"Slovakia corrected invoice", edits(slovakia, one(rep(uTypeCode, "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>"))), ids("SK-R-001")},
		{"Slovakia corrected invoice with its reference", edits(slovakia, one(rep(uTypeCode, "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>"),
			before(uDocRefs, "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-2026-0041</cbc:ID></cac:InvoiceDocumentReference></cac:BillingReference>"))), nil},
		{"Iceland EINDAGI without due date", edits(iceland, one(eindagi("01.12.2026"), remove(uDueDate))), ids("IS-R-008", "IS-R-009", "IS-R-010")},
	}
}

func TestUBLMutations(t *testing.T) {
	doc := load(t, ublBase)
	for _, c := range ublMutationCases() {
		t.Run(c.name, func(t *testing.T) {
			r := runUBL(t, []byte(apply(t, doc, c.edits)))
			got := r.Rules()
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("rules = %q, want %q\n%s", got, want, r)
			}
			checkUBLFindings(t, r)
		})
	}
}

// TestUBLDispatch checks that Check applies the rules of the UBL
// schematron to the UBL document of a Doc, and not those of the CII
// schematron.
func TestUBLDispatch(t *testing.T) {
	doc := apply(t, load(t, ublBase), one(remove(uBuyerRef)))
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: &rules.Doc{UBL: root}, Report: r, Source: rules.SourcePeppol})
	if got := r.Rules(); !slices.Equal(got, []string{"PEPPOL-EN16931-R003"}) {
		t.Errorf("rules = %q\n%s", got, r)
	}
	if f := r.Findings[0]; f.Path != "/Invoice" || f.Line != 5 {
		t.Errorf("finding not located in the UBL document: %+v", f)
	}
}

func TestUBLCatalog(t *testing.T) {
	seen := map[string]bool{}
	sev := map[rules.Severity]int{}
	for _, r := range UBLCatalog {
		if seen[r.ID] {
			t.Errorf("duplicate %s", r.ID)
		}
		seen[r.ID] = true
		sev[r.Severity]++
		if r.Description == "" || strings.Contains(r.Description, "  ") {
			t.Errorf("bad description for %s: %q", r.ID, r.Description)
		}
		if ublByID()[r.ID] == nil || ublByID()[r.ID].Severity != r.Severity {
			t.Errorf("ublByID()[%s] is not the catalog entry", r.ID)
		}
		// A rule of both schematrons has the same severity.
		if c := byID()[r.ID]; c != nil && c.Severity != r.Severity {
			t.Errorf("%s: severity %v in UBLCatalog, %v in Catalog", r.ID, r.Severity, c.Severity)
		}
	}
	if len(UBLCatalog) != 294 || sev[rules.Error] != 264 || sev[rules.Warning] != 30 {
		t.Errorf("%d assertions (%d errors, %d warnings), want 294 (264, 30)", len(UBLCatalog), sev[rules.Error], sev[rules.Warning])
	}
}

// TestUBLEmpty checks that incomplete or foreign documents do not make the
// rules panic, and give only findings of the catalog.
func TestUBLEmpty(t *testing.T) {
	const (
		inv = `xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"`
		cn  = `xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"`
	)
	for _, doc := range []string{
		`<Invoice ` + inv + `/>`,
		`<CreditNote ` + cn + `/>`,
		`<Invoice ` + inv + `><cac:AccountingSupplierParty/><cac:AccountingCustomerParty><cac:Party/></cac:AccountingCustomerParty><cac:InvoiceLine><cac:Price><cbc:BaseQuantity unitCode="C62"/><cac:AllowanceCharge/></cac:Price></cac:InvoiceLine></Invoice>`,
		`<CreditNote ` + cn + `><cac:AccountingSupplierParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:Party></cac:AccountingSupplierParty><cac:AccountingCustomerParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:Party></cac:AccountingCustomerParty><cac:PaymentMeans/><cac:CreditNoteLine/></CreditNote>`,
		`<Order xmlns="urn:oasis:names:specification:ubl:schema:xsd:Order-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"><cbc:EndpointID schemeID="0088">1</cbc:EndpointID></Order>`,
		`<x/>`,
	} {
		r := runUBL(t, []byte(doc))
		checkUBLFindings(t, r)
	}
	// Each country of the national patterns, on an invoice reduced to its
	// parties.
	for _, country := range []string{"SK", "NO", "DK", "IT", "SE", "GR", "IS", "NL", "DE"} {
		doc := `<Invoice ` + inv + `><cbc:InvoiceTypeCode/><cac:AccountingSupplierParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>` + country +
			`</cbc:IdentificationCode></cac:Country></cac:PostalAddress><cac:PartyTaxScheme><cbc:CompanyID/><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme><cac:PartyLegalEntity><cbc:CompanyID/></cac:PartyLegalEntity></cac:Party></cac:AccountingSupplierParty>` +
			`<cac:AccountingCustomerParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>` + country +
			`</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:Party></cac:AccountingCustomerParty><cac:PaymentMeans><cbc:PaymentMeansCode/><cac:PayeeFinancialAccount/></cac:PaymentMeans><cac:InvoiceLine><cac:Item/></cac:InvoiceLine></Invoice>`
		r := runUBL(t, []byte(doc))
		if len(r.Findings) == 0 {
			t.Errorf("%s: no finding", country)
		}
		checkUBLFindings(t, r)
	}
}

func TestXSDate(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1970-01-01", 0, true},
		{" 1970-01-02 ", 1440, true},
		{"1970-01-01Z", 0, true},
		{"1970-01-01+01:00", -60, true},
		{"1970-01-01-14:00", 840, true},
		{"2024-02-29", 28486080, true},
		{"0000-01-01", -1036120320, true},
		{"-0001-01-01", -1036645920, true},
		{"12345-01-01", 5456723040, true},
		{"2023-02-29", 0, false},
		{"2024-13-01", 0, false},
		{"2024-00-10", 0, false},
		{"2024-01-00", 0, false},
		{"2024-1-01", 0, false},
		{"024-01-01", 0, false},
		{"02024-01-01", 0, false},
		{"2024-01-01+14:01", 0, false},
		{"2024-01-01+1:00", 0, false},
		{"2024-01-01T00:00:00", 0, false},
		{"20240101", 0, false},
		{"", 0, false},
	} {
		got, ok := xsDate(c.in)
		if ok != c.ok || ok && got != c.want {
			t.Errorf("xsDate(%q) = %d, %v, want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTIN(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"123456783", true},
		{"094014298", true},
		{"094014299", false},
		{"123456780", false},
		{"12345678", false},
		{"12345678a", false},
		{"", false},
	} {
		if got := tin(c.in); got != c.want {
			t.Errorf("tin(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidIBAN(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"GB29NWBK60161331926819", true},
		{" GB29 NWBK 6016 1331 9268 19 ", true},
		{"DE89370400440532013000", true},
		{"GB29NWBK60161331926818", false},
		{"gb29NWBK60161331926819", false},
		{"GB2", false},
		{"", false},
	} {
		if got := validIBAN(c.in); got != c.want {
			t.Errorf("validIBAN(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestXSInteger(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string // "" when the cast fails
	}{
		{"0", "0"},
		{" +12 ", "12"},
		{"-007", "-7"},
		{"123456789012345678901234567890", "123456789012345678901234567890"},
		{"1.0", ""},
		{"1e3", ""},
		{"+", ""},
		{"", ""},
	} {
		v, ok := xsInteger(c.in)
		if got := ""; ok {
			got = v.String()
			if got != c.want {
				t.Errorf("xsInteger(%q) = %s, want %q", c.in, got, c.want)
			}
		} else if c.want != "" {
			t.Errorf("xsInteger(%q) fails, want %s", c.in, c.want)
		}
	}
}
