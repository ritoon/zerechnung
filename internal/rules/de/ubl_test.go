package de

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const ublBase = "testdata/xrechnung.ubl.xml" // the invoice of base in UBL 2.1, without finding

// runUBL parses a UBL document and runs Check on it.
func runUBL(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ubl.IsUBL(root) {
		t.Fatalf("root %s is not a UBL invoice", root.Name)
	}
	return checkDoc(ubl.View(root, invoice.ProfileUnknown))
}

func cbc(name, value string) string { return "<cbc:" + name + ">" + value + "</cbc:" + name + ">" }

// Anchors of the UBL base invoice.
const (
	uSpec        = "<cbc:CustomizationID>" + CIUSID + "</cbc:CustomizationID>"
	uTypeCode    = "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>"
	uIssueDate   = "<cbc:IssueDate>2026-11-02</cbc:IssueDate>"
	uBuyerRef    = "<cbc:BuyerReference>04011000-12345-03</cbc:BuyerReference>"
	uARD         = "<cac:AdditionalDocumentReference>"
	uARDEnd      = "</cac:AdditionalDocumentReference>"
	uURI         = "<cbc:URI>https://flottenservice.example/lieferscheine/311</cbc:URI>"
	uAttachment  = `<cbc:EmbeddedDocumentBinaryObject mimeCode="application/pdf" filename="lieferschein-311.pdf">JVBERi0xLjQK</cbc:EmbeddedDocumentBinaryObject>`
	uEndpoint    = `<cbc:EndpointID schemeID="EM">rechnung@flottenservice.example</cbc:EndpointID>`
	uSellerGLN   = `<cbc:ID schemeID="0088">4000001000005</cbc:ID>`
	uSellerIDEnd = "</cac:PartyIdentification>"
	uSellerVAT   = "<cbc:CompanyID>DE123456789</cbc:CompanyID>"
	uSellerTax   = `<cac:PartyTaxScheme>
        <cbc:CompanyID>DE123456789</cbc:CompanyID>
        <cac:TaxScheme>
          <cbc:ID>VAT</cbc:ID>
        </cac:TaxScheme>
      </cac:PartyTaxScheme>`
	uSellerLegal = `<cbc:CompanyID schemeID="0198">HRB12345</cbc:CompanyID>`
	uContactName = "<cbc:Name>Erika Beispiel</cbc:Name>"
	uPhone       = "<cbc:Telephone>+49 30 1234567</cbc:Telephone>"
	uEmail       = "<cbc:ElectronicMail>rechnung@flottenservice.example</cbc:ElectronicMail>"
	uCustomerEnd = "</cac:AccountingCustomerParty>"
	uBuyerParty  = `<cbc:EndpointID schemeID="EM">eingang@amt.example</cbc:EndpointID>`
	uDelivery    = "<cbc:ActualDeliveryDate>2026-10-30</cbc:ActualDeliveryDate>"
	uShipToGLN   = `<cbc:ID schemeID="0088">4000001000098</cbc:ID>`
	uPMCode      = "<cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>"
	uIBAN        = "<cbc:ID>DE89370400440532013000</cbc:ID>"
	uPayeeAcct   = `<cac:PayeeFinancialAccount>
      <cbc:ID>DE89370400440532013000</cbc:ID>
    </cac:PayeeFinancialAccount>`
	uPMEnd       = "</cac:PaymentMeans>"
	uSkonto      = "#SKONTO#TAGE=14#PROZENT=2.00#"
	uNoteEnd     = "</cbc:Note>\n  </cac:PaymentTerms>"
	uTermsEnd    = "</cac:PaymentTerms>"
	uTaxTotal    = "<cac:TaxTotal>"
	uRate19      = "<cbc:Percent>19</cbc:Percent>"
	uPayable     = `<cbc:PayableAmount currencyID="EUR">234.35</cbc:PayableAmount>`
	uLine1Item   = "<cac:Item>\n      <cbc:Name>Wartung Fahrzeugflotte</cbc:Name>"
	uProduct1    = "<cbc:Name>Wartung Fahrzeugflotte</cbc:Name>"
	uProductGTIN = `<cbc:ID schemeID="0160">4000001000111</cbc:ID>`
	uLine1Tax    = "</cac:StandardItemIdentification>"
	uLine1TaxEnd = "</cac:ClassifiedTaxCategory>\n    </cac:Item>"
	uLine1Price  = "</cac:Price>"
	uCatS        = "<cbc:ID>S</cbc:ID>"
)

const (
	sepaID    = `<cac:PartyIdentification><cbc:ID schemeID="SEPA">DE98ZZZ09999999999</cbc:ID></cac:PartyIdentification>`
	uMandate  = `<cac:PaymentMandate><cbc:ID>MANDAT-0815</cbc:ID><cac:PayerFinancialAccount><cbc:ID>DE02120300000000202051</cbc:ID></cac:PayerFinancialAccount></cac:PaymentMandate>`
	uCard     = `<cac:CardAccount><cbc:PrimaryAccountNumberID>1234</cbc:PrimaryAccountNumberID><cbc:NetworkID>VISA</cbc:NetworkID></cac:CardAccount>`
	uContract = "<cac:ContractDocumentReference><cbc:ID>V-2026-77</cbc:ID></cac:ContractDocumentReference>"
	uTender   = "<cac:OriginatorDocumentReference><cbc:ID>LOS-3</cbc:ID></cac:OriginatorDocumentReference>"
	uPeriod   = "<cac:InvoicePeriod><cbc:StartDate>2026-10-01</cbc:StartDate><cbc:EndDate>2026-10-31</cbc:EndDate></cac:InvoicePeriod>"
)

var (
	// uDirectDebit turns the SEPA credit transfer of the base invoice into a
	// valid SEPA direct debit (59, BG-19 complete, creditor identifier of
	// the seller).
	uDirectDebit = one(
		rep(uPMCode, cbc("PaymentMeansCode", "59")),
		rep(uPayeeAcct, uMandate),
		after(uSellerIDEnd, sepaID),
	)
	// uExtension declares the XRechnung extension.
	uExtension = one(rep(uSpec, cbc("CustomizationID", ExtensionID)))
	// uCVDData adds valid commercial vehicle data, without declaring CVDID.
	uCVDData = one(
		after(uLine1Tax, uClass("CVD", "M1")),
		rep(uLine1TaxEnd, "</cac:ClassifiedTaxCategory>"+uCVA("clean")+"\n    </cac:Item>"),
		before(uARD, uTender+uContract),
	)
	// uCVD declares the CVD and adds valid commercial vehicle data.
	uCVD = edits(one(rep(uSpec, cbc("CustomizationID", CVDID))), uCVDData)
	// uPrepaid adds a valid third party payment of 10.00 EUR to the
	// extension.
	uPrepaid = edits(uExtension, one(
		before(uTaxTotal, uPrepaidPayment("PP-1", `<cbc:PaidAmount currencyID="EUR">10.00</cbc:PaidAmount>`, "Zuschuss")),
		rep(uPayable, `<cbc:PayableAmount currencyID="EUR">244.35</cbc:PayableAmount>`),
	))
	// creditNote turns the invoice into a credit note.
	creditNote = one(
		rep(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`, `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"`),
		rep("</Invoice>", "</CreditNote>"),
		rep(uTypeCode, cbc("CreditNoteTypeCode", "381")),
		repAll("cac:InvoiceLine>", "cac:CreditNoteLine>"),
		repAll("cbc:InvoicedQuantity", "cbc:CreditedQuantity"),
	)
)

func uCVA(value string) string {
	return "<cac:AdditionalItemProperty>" + cbc("Name", "cva") + cbc("Value", value) + "</cac:AdditionalItemProperty>"
}

func uClass(listID, value string) string {
	return `<cac:CommodityClassification><cbc:ItemClassificationCode listID="` + listID + `">` + value + `</cbc:ItemClassificationCode></cac:CommodityClassification>`
}

func uAttachmentRef(name string) string {
	return `<cac:AdditionalDocumentReference><cbc:ID>A-2</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="application/pdf" filename="` + name + `">JVBERi0xLjQK</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>`
}

func uPrepaidPayment(id, amount, instruction string) string {
	return "<cac:PrepaidPayment>" + cbc("ID", id) + amount + cbc("InstructionID", instruction) + "</cac:PrepaidPayment>"
}

// uAllowance returns an allowance or charge with the indicator and the
// optional percentage and base amount.
func uAllowance(indicator, percent, base string) string {
	s := "<cac:AllowanceCharge>" + cbc("ChargeIndicator", indicator) + cbc("AllowanceChargeReason", "Rabatt")
	if percent != "" {
		s += cbc("MultiplierFactorNumeric", percent)
	}
	s += `<cbc:Amount currencyID="EUR">5.00</cbc:Amount>`
	if base != "" {
		s += `<cbc:BaseAmount currencyID="EUR">` + base + `</cbc:BaseAmount>`
	}
	return s + "<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>"
}

// uSubLine returns a sub invoice line with its net amount and taxes VAT
// categories.
func uSubLine(id, amount string, taxes int, sub ...string) string {
	s := "<cac:SubInvoiceLine>" + cbc("ID", id) + `<cbc:InvoicedQuantity unitCode="HUR">1</cbc:InvoicedQuantity>` +
		`<cbc:LineExtensionAmount currencyID="EUR">` + amount + `</cbc:LineExtensionAmount><cac:Item><cbc:Name>Teil</cbc:Name>`
	for range taxes {
		s += "<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>"
	}
	return s + `</cac:Item><cac:Price><cbc:PriceAmount currencyID="EUR">` + amount + `</cbc:PriceAmount></cac:Price>` + strings.Join(sub, "") + "</cac:SubInvoiceLine>"
}

// ublCases returns the mutations of the UBL base invoice. Every assertion of
// UBLCatalog fails in at least one case (see TestUBLCoverage).
func ublCases() []mutationCase {
	return []mutationCase{
		{"valid CIUS", nil, nil},
		{"valid credit note", creditNote, nil},
		{"valid extension", uExtension, nil},
		{"valid CVD", uCVD, nil},
		{"valid CVD credit note", edits(uCVD, creditNote), nil},
		{"valid direct debit", uDirectDebit, nil},
		{"valid extension direct debit", edits(uExtension, uDirectDebit), nil},
		{"valid third party payment", uPrepaid, nil},

		// Root rules.
		{"BR-DE-1 no payment means", one(repAll("cac:PaymentMeans>", "cac:PaymentMeansX>")), ids("BR-DE-1")},
		{"BR-DE-15 blank buyer reference", one(rep(uBuyerRef, cbc("BuyerReference", " "))), ids("BR-DE-15", "PEPPOL-EN16931-R008")},
		{"BR-DE-15 no buyer reference", one(remove(uBuyerRef)), ids("BR-DE-15")},
		{"BR-DE-16 no seller VAT identifier", one(remove(uSellerTax)), ids("BR-DE-16")},
		{"BR-DE-16 blank VAT identifier", one(rep(uSellerVAT, cbc("CompanyID", " "))), ids("BR-DE-16", "PEPPOL-EN16931-R008")},
		{"BR-DE-16 tax representative", one(remove(uSellerTax), after(uCustomerEnd, "<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Vertreter</cbc:Name></cac:PartyName></cac:TaxRepresentativeParty>")), nil},
		// The VAT breakdown is not part of the condition.
		{"BR-DE-16 lines in category O", one(remove(uSellerTax), repN(3, uCatS, cbc("ID", "O")), repN(3, uCatS, cbc("ID", "O"))), nil},
		{"BR-DE-16 document charge in category S", one(remove(uSellerTax), repN(3, uCatS, cbc("ID", "O")), repN(3, uCatS, cbc("ID", "O")),
			before(uTaxTotal, uAllowance("true", "", ""))), ids("BR-DE-16")},
		// The charge indicator is compared untrimmed.
		{"BR-DE-16 spaced charge indicator", one(remove(uSellerTax), repN(3, uCatS, cbc("ID", "O")), repN(3, uCatS, cbc("ID", "O")),
			before(uTaxTotal, uAllowance(" false", "", ""))), nil},
		{"BR-DE-17 type 325", one(rep(uTypeCode, cbc("InvoiceTypeCode", "325"))), ids("BR-DE-17")},
		{"BR-DE-17 type 877", one(rep(uTypeCode, cbc("InvoiceTypeCode", " 877 "))), nil},
		{"BR-DE-17 credit note 383", edits(creditNote, one(rep(cbc("CreditNoteTypeCode", "381"), cbc("CreditNoteTypeCode", "383")))), ids("BR-DE-17")},
		{"BR-DE-17 no type code", one(remove(uTypeCode)), ids("BR-DE-17")},
		{"BR-DE-18 percentage without decimals", one(rep(uSkonto, "#SKONTO#TAGE=14#PROZENT=2#")), ids("BR-DE-18")},
		{"BR-DE-18 no final line break", one(rep(uSkonto+"\n"+uNoteEnd, uSkonto+uNoteEnd)), ids("BR-DE-18")},
		{"BR-DE-18 base amount", one(rep(uSkonto, "#SKONTO#TAGE=14#PROZENT=2.00#BASISBETRAG=-100.00#")), nil},
		{"BR-DE-18 second note is not checked", one(before(uTermsEnd, cbc("Note", "#skonto#"))), nil},
		{"BR-DE-18 second payment terms", one(after(uTermsEnd, "<cac:PaymentTerms>"+cbc("Note", "#SKONTO#TAGE=7#PROZENT=3#\n")+"</cac:PaymentTerms>")), ids("BR-DE-18")},
		{"BR-DE-21 EN 16931", one(rep(uSpec, cbc("CustomizationID", "urn:cen.eu:en16931:2017"))), ids("BR-DE-21")},
		{"BR-DE-21 spaced identifier", one(rep(uSpec, cbc("CustomizationID", " "+CIUSID))), ids("BR-DE-21")},
		{"BR-DE-22 duplicate file name", one(after(uARDEnd, uAttachmentRef("lieferschein-311.pdf"))), ids("BR-DE-22")},
		{"BR-DE-22 distinct file names", one(after(uARDEnd, uAttachmentRef("stundenzettel.pdf"))), nil},
		{"BR-DE-26 corrected invoice without reference", one(rep(uTypeCode, cbc("InvoiceTypeCode", "384"))), ids("BR-DE-26")},
		{"BR-DE-26 corrected invoice with reference", one(rep(uTypeCode, cbc("InvoiceTypeCode", "384")),
			before(uARD, "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>RE-2026-0999</cbc:ID></cac:InvoiceDocumentReference></cac:BillingReference>")), nil},
		{"BR-DE-30 no creditor identifier", one(rep(uPMCode, cbc("PaymentMeansCode", "59")), rep(uPayeeAcct, uMandate)), ids("BR-DE-30")},
		{"BR-DE-30 creditor identifier of the payee", one(rep(uPMCode, cbc("PaymentMeansCode", "59")), rep(uPayeeAcct, uMandate),
			after(uCustomerEnd, "<cac:PayeeParty>"+sepaID+"<cac:PartyName><cbc:Name>Inkasso</cbc:Name></cac:PartyName></cac:PayeeParty>")), nil},
		{"BR-DE-30 scheme sepa", one(rep(uPMCode, cbc("PaymentMeansCode", "59")), rep(uPayeeAcct, uMandate), after(uSellerIDEnd, strings.Replace(sepaID, "SEPA", "sepa", 1))), ids("BR-DE-30")},
		{"BR-DE-31 no debited account", edits(uDirectDebit, one(rep("<cac:PayerFinancialAccount><cbc:ID>DE02120300000000202051</cbc:ID></cac:PayerFinancialAccount>", ""))), ids("BR-DE-20", "BR-DE-31")},
		{"BR-DE-TMP-32 no delivery date", one(remove(uDelivery)), ids("BR-DE-TMP-32")},
		{"BR-DE-TMP-32 invoicing period", one(remove(uDelivery), before(uARD, uPeriod)), nil},
		{"BR-DE-TMP-32 period on every line", one(remove(uDelivery), repAll("</cbc:LineExtensionAmount>\n    <cac:Item>", "</cbc:LineExtensionAmount>"+uPeriod+"<cac:Item>")), nil},
		{"BR-DE-TMP-32 period on one line", one(remove(uDelivery), rep("</cbc:LineExtensionAmount>\n    <cac:Item>", "</cbc:LineExtensionAmount>"+uPeriod+"<cac:Item>")), ids("BR-DE-TMP-32")},

		// Supporting documents and dates.
		{"BR-TMP-2 relative URL", one(rep(uURI, cbc("URI", "flottenservice.example/lieferscheine/311"))), ids("BR-TMP-2")},
		{"BR-TMP-2 one character scheme", one(rep(uURI, cbc("URI", "h://x"))), ids("BR-TMP-2")},
		{"BR-TMP-2 no URI", one(remove(uURI)), ids("BR-TMP-2", "PEPPOL-EN16931-R008")},
		{"BR-TMP-2 URN", one(rep(uURI, cbc("URI", "urn:isbn:9783161484100"))), nil},
		{"BR-TMP-6 date and time", one(rep(uIssueDate, cbc("IssueDate", "2026-11-02T10:00:00"))), ids("BR-TMP-6")},
		{"BR-TMP-6 basic format", one(rep(uIssueDate, cbc("IssueDate", "20261102"))), ids("BR-TMP-6")},
		{"BR-TMP-6 surrounding spaces", one(rep(uIssueDate, cbc("IssueDate", " 2026-11-02\n"))), nil},
		{"BR-TMP-6 line period", one(rep("</cbc:LineExtensionAmount>\n    <cac:Item>", "</cbc:LineExtensionAmount><cac:InvoicePeriod><cbc:StartDate>01.10.2026</cbc:StartDate></cac:InvoicePeriod><cac:Item>")), ids("BR-TMP-6")},
		{"BR-TMP-6 payment due date", one(after(uPMCode, cbc("PaymentDueDate", "2026/12/02"))), ids("BR-TMP-6")},

		// Seller, buyer and deliver to.
		{"BR-DE-2 no seller contact", one(repAll("cac:Contact>", "cac:ContactX>")), ids("BR-DE-2")},
		{"BR-DE-3 blank seller city", one(rep(cbc("CityName", "Berlin"), cbc("CityName", ""))), ids("BR-DE-3", "PEPPOL-EN16931-R008")},
		{"BR-DE-4 no seller post code", one(remove(cbc("PostalZone", "10115"))), ids("BR-DE-4")},
		{"BR-DE-5 no contact point", one(remove(uContactName)), ids("BR-DE-5")},
		{"BR-DE-6 no telephone", one(remove(uPhone)), ids("BR-DE-27", "BR-DE-6")},
		{"BR-DE-7 no email", one(remove(uEmail)), ids("BR-DE-28", "BR-DE-7")},
		{"BR-DE-27 two digits", one(rep(uPhone, cbc("Telephone", "+49 OO IO"))), ids("BR-DE-27")},
		{"BR-DE-28 one label", one(rep(uEmail, cbc("ElectronicMail", "rechnung@flottenservice"))), ids("BR-DE-28")},
		{"BR-DE-28 surrounding space", one(rep(uEmail, cbc("ElectronicMail", "  erika.beispiel@flottenservice.example  "))), nil},
		{"BR-DE-8 blank buyer city", one(rep(cbc("CityName", "Hamburg"), cbc("CityName", "  "))), ids("BR-DE-8", "PEPPOL-EN16931-R008")},
		{"BR-DE-9 no buyer post code", one(remove(cbc("PostalZone", "20095"))), ids("BR-DE-9")},
		{"BR-DE-10 no deliver to city", one(remove(cbc("CityName", "Bremen"))), ids("BR-DE-10")},
		{"BR-DE-11 no deliver to post code", one(remove(cbc("PostalZone", "28195"))), ids("BR-DE-11")},

		// Payment means.
		{"BR-DE-19 IBAN checksum", one(rep(uIBAN, cbc("ID", "DE89370400440532013001"))), ids("BR-DE-19")},
		{"BR-DE-19 IBAN with spaces", one(rep(uIBAN, cbc("ID", "DE89 3704 0044 0532 0130 00"))), nil},
		{"BR-DE-19 credit transfer 30", one(rep(uPMCode, cbc("PaymentMeansCode", "30")), rep(uIBAN, cbc("ID", "1234567"))), nil},
		{"BR-DE-19 spaced code", one(rep(uPMCode, cbc("PaymentMeansCode", " 58 ")), rep(uIBAN, cbc("ID", "1234567"))), ids("BR-DE-19")},
		{"BR-DE-23-a no credit transfer", one(rep(uPMCode, cbc("PaymentMeansCode", "30")), remove(uPayeeAcct)), ids("BR-DE-23-a")},
		{"BR-DE-23-b card", one(before(uPayeeAcct, uCard)), ids("BR-DE-23-b")},
		{"BR-DE-23-b mandate", one(before(uPMEnd, uMandate), after(uSellerIDEnd, sepaID)), ids("BR-DE-23-b")},
		{"BR-DE-23-b mandate without creditor identifier", one(before(uPMEnd, uMandate)), ids("BR-DE-23-b", "BR-DE-30")},
		{"BR-DE-24-a/b card code", one(rep(uPMCode, cbc("PaymentMeansCode", "48"))), ids("BR-DE-24-a", "BR-DE-24-b")},
		{"BR-DE-24 card", one(rep(uPMCode, cbc("PaymentMeansCode", "54")), rep(uPayeeAcct, uCard)), nil},
		{"BR-DE-24-b card and mandate", one(rep(uPMCode, cbc("PaymentMeansCode", "55")), rep(uPayeeAcct, uCard+uMandate), after(uSellerIDEnd, sepaID)), ids("BR-DE-24-b")},
		{"BR-DE-20 IBAN checksum", edits(uDirectDebit, one(rep("DE02120300000000202051", "DE02120300000000202052"))), ids("BR-DE-20")},
		{"BR-DE-25 direct debit code only", one(rep(uPMCode, cbc("PaymentMeansCode", "59"))), ids("BR-DE-20", "BR-DE-25-a", "BR-DE-25-b", "PEPPOL-EN16931-R061")},
		{"BR-DE-25-b card", edits(uDirectDebit, one(before(uMandate, uCard))), ids("BR-DE-25-b")},
		{"BR-DE-14 no VAT rate", one(remove(uRate19)), ids("BR-DE-14")},
		{"BR-DE-14 blank VAT rate", one(rep(uRate19, cbc("Percent", " "))), ids("BR-DE-14", "PEPPOL-EN16931-R008")},

		// Extension.
		{"BR-DEX-01 zip attachment", edits(uExtension, one(rep(uAttachment, strings.Replace(uAttachment, "application/pdf", "application/zip", 1)))), ids("BR-DEX-01")},
		{"BR-DEX-01 XML attachment", edits(uExtension, one(rep(uAttachment, strings.Replace(uAttachment, "application/pdf", "application/xml", 1)))), nil},
		{"BR-DEX-01 needs the extension", one(rep(uAttachment, strings.Replace(uAttachment, "application/pdf", "application/zip", 1))), nil},
		{"BR-DEX-02 sub lines", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "100.00", 1)+uSubLine("1.2", "25.00", 1)))), nil},
		{"BR-DEX-02 wrong sum", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "100.00", 1)+uSubLine("1.2", "20.00", 1)))), ids("BR-DEX-02")},
		{"BR-DEX-02 nested sub lines", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "125.00", 1, uSubLine("1.1.1", "100.00", 1), uSubLine("1.1.2", "20.00", 1))))), ids("BR-DEX-02")},
		{"BR-DEX-02 nested sub lines sum", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "125.00", 1, uSubLine("1.1.1", "100.00", 1), uSubLine("1.1.2", "25.00", 1))))), nil},
		{"BR-DEX-02 not a decimal", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "125,00", 1)))), ids("BR-DEX-02")},
		{"BR-DEX-02 needs the extension", one(after(uLine1Price, uSubLine("1.1", "100.00", 1))), nil},
		{"BR-DEX-03 no VAT category", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "125.00", 0)))), ids("BR-DEX-03")},
		{"BR-DEX-03 two VAT categories", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "125.00", 2)))), ids("BR-DEX-03")},
		{"BR-DEX-02/03 apply to invoices only", edits(uExtension, one(after(uLine1Price, uSubLine("1.1", "1.00", 0))), creditNote), nil},
		{"BR-DEX-09 wrong amount due", edits(uExtension, one(rep(uPayable, `<cbc:PayableAmount currencyID="EUR">234.36</cbc:PayableAmount>`))), ids("BR-DEX-09")},
		{"BR-DEX-09 rounded to cents", edits(uExtension, one(rep(uPayable, `<cbc:PayableAmount currencyID="EUR">234.354</cbc:PayableAmount>`))), nil},
		{"BR-DEX-09 half rounds up", edits(uExtension, one(rep(uPayable, `<cbc:PayableAmount currencyID="EUR">234.345</cbc:PayableAmount>`))), nil},
		{"BR-DEX-09 paid and rounding amounts", edits(uExtension, one(rep(uPayable, `<cbc:PrepaidAmount currencyID="EUR">34.35</cbc:PrepaidAmount><cbc:PayableRoundingAmount currencyID="EUR">0.01</cbc:PayableRoundingAmount><cbc:PayableAmount currencyID="EUR">200.01</cbc:PayableAmount>`))), nil},
		{"BR-DEX-09 credit note", edits(uExtension, one(rep(uPayable, `<cbc:PayableAmount currencyID="EUR">200.00</cbc:PayableAmount>`)), creditNote), ids("BR-DEX-09")},
		{"BR-DEX-09 no amount due", edits(uExtension, one(remove(uPayable))), ids("BR-DEX-09")},
		{"BR-DEX-09 needs the extension", one(rep(uPayable, `<cbc:PayableAmount currencyID="EUR">234.36</cbc:PayableAmount>`)), nil},
		{"BR-DEX-04 unknown scheme", edits(uExtension, one(rep(uSellerGLN, `<cbc:ID schemeID="9999">4000001000005</cbc:ID>`))), ids("BR-DEX-04")},
		{"BR-DEX-04 XR01", edits(uExtension, one(rep(uSellerGLN, `<cbc:ID schemeID="XR01">4000001000005</cbc:ID>`))), nil},
		{"BR-DEX-04 empty scheme passes", edits(uExtension, one(rep(uSellerGLN, `<cbc:ID schemeID="">4000001000005</cbc:ID>`))), nil},
		{"BR-DEX-04 SEPA of the buyer", edits(uExtension, one(after(uBuyerParty, sepaID))), ids("BR-DEX-04")},
		{"BR-DEX-04 SEPA of the payee", edits(uExtension, one(after(uCustomerEnd, "<cac:PayeeParty>"+sepaID+"<cac:PartyName><cbc:Name>Inkasso</cbc:Name></cac:PartyName></cac:PayeeParty>"))), nil},
		{"BR-DEX-05 legal registration", edits(uExtension, one(rep(uSellerLegal, `<cbc:CompanyID schemeID="HRB">HRB12345</cbc:CompanyID>`))), ids("BR-DEX-05")},
		{"BR-DEX-05 VAT identifier is not checked", edits(uExtension, one(rep(uSellerVAT, `<cbc:CompanyID schemeID="VAT">DE123456789</cbc:CompanyID>`))), nil},
		{"BR-DEX-06 item identifier", edits(uExtension, one(rep(uProductGTIN, `<cbc:ID schemeID="GTIN">4000001000111</cbc:ID>`))), ids("BR-DEX-06")},
		{"BR-DEX-07 endpoint", edits(uExtension, one(rep(uEndpoint, `<cbc:EndpointID schemeID="XX">rechnung@flottenservice.example</cbc:EndpointID>`))), ids("BR-DEX-07")},
		{"BR-DEX-07 XR02", edits(uExtension, one(rep(uEndpoint, `<cbc:EndpointID schemeID="XR02">rechnung@flottenservice.example</cbc:EndpointID>`))), nil},
		{"BR-DEX-08 deliver to location", edits(uExtension, one(rep(uShipToGLN, `<cbc:ID schemeID="9999">4000001000098</cbc:ID>`))), ids("BR-DEX-08")},
		{"BR-DEX-10 blank payment type", edits(uPrepaid, one(rep(cbc("ID", "PP-1"), cbc("ID", " ")))), ids("BR-DEX-10", "PEPPOL-EN16931-R008")},
		{"BR-DEX-11 no amount", edits(uPrepaid, one(remove(`<cbc:PaidAmount currencyID="EUR">10.00</cbc:PaidAmount>`))), ids("BR-DEX-09", "BR-DEX-11", "BR-DEX-14")},
		{"BR-DEX-12 no description", edits(uPrepaid, one(remove(cbc("InstructionID", "Zuschuss")))), ids("BR-DEX-12")},
		{"BR-DEX-13 three decimals", edits(uPrepaid, one(rep(`>10.00</cbc:PaidAmount>`, `>10.000</cbc:PaidAmount>`))), ids("BR-DEX-13")},
		{"BR-DEX-14 other currency", edits(uPrepaid, one(rep(`<cbc:PaidAmount currencyID="EUR">`, `<cbc:PaidAmount currencyID="USD">`))), ids("BR-DEX-14")},
		{"BR-DEX-10 to 14 apply to invoices only", edits(uExtension, one(before(uTaxTotal, uPrepaidPayment(" ", `<cbc:PaidAmount currencyID="USD">0.001</cbc:PaidAmount>`, ""))), creditNote), ids("PEPPOL-EN16931-R008")},
		{"BR-DEX-10 to 14 need the extension", one(before(uTaxTotal, uPrepaidPayment(" ", "", ""))), ids("PEPPOL-EN16931-R008")},

		// Commercial vehicle data.
		{"BR-DE-CVD-02 no tender reference", edits(uCVD, one(remove(uTender))), ids("BR-DE-CVD-02")},
		{"BR-DE-CVD-01 no contract reference", edits(uCVD, one(remove(uContract))), ids("BR-DE-CVD-01")},
		{"BR-DE-CVD-03 no vehicle line", edits(uCVD, one(remove(uClass("CVD", "M1")), remove(uCVA("clean")))), ids("BR-DE-CVD-03")},
		{"BR-DE-CVD-06-a no cva", edits(uCVD, one(remove(uCVA("clean")))), ids("BR-DE-CVD-03", "BR-DE-CVD-06-a")},
		{"BR-DE-CVD-06-a two cva", edits(uCVD, one(after(uCVA("clean"), uCVA("other")))), ids("BR-DE-CVD-06-a")},
		{"BR-DE-CVD-06-b two classes", edits(uCVD, one(after(uClass("CVD", "M1"), uClass("CVD", "N1")))), ids("BR-DE-CVD-06-b")},
		{"BR-DE-CVD-06-b no class", edits(uCVD, one(remove(uClass("CVD", "M1")))), ids("BR-DE-CVD-03", "BR-DE-CVD-06-b")},
		{"BR-TMP-CVD-01 unknown list", edits(uCVD, one(after(uClass("CVD", "M1"), uClass("XX", "4711")))), ids("BR-TMP-CVD-01")},
		{"BR-TMP-CVD-01 no list passes", edits(uCVD, one(after(uClass("CVD", "M1"), "<cac:CommodityClassification>"+cbc("ItemClassificationCode", "4711")+"</cac:CommodityClassification>"))), nil},
		{"BR-DE-CVD-04 category L1", edits(uCVD, one(rep(uClass("CVD", "M1"), uClass("CVD", "L1")))), ids("BR-DE-CVD-04")},
		{"BR-DE-CVD-04 credit note", edits(uCVD, one(rep(uClass("CVD", "M1"), uClass("CVD", "L1"))), creditNote), ids("BR-DE-CVD-04")},
		{"BR-DE-CVD-05 diesel", edits(uCVD, one(rep(uCVA("clean"), uCVA("diesel")))), ids("BR-DE-CVD-05")},
		{"BR-DE-CVD-* need the CVD identifier", one(after(uLine1Tax, uClass("CVD", "L1")+uClass("XX", "1")), rep(uLine1TaxEnd, "</cac:ClassifiedTaxCategory>"+uCVA("diesel")+uCVA("x")+"\n    </cac:Item>")), nil},

		// Charge indicators.
		{"PEPPOL-EN16931-R043 document allowance", one(before(uTaxTotal, uAllowance("1", "", ""))), ids("PEPPOL-EN16931-R043")},
		{"PEPPOL-EN16931-R043 spaced indicator", one(before(uTaxTotal, uAllowance(" true ", "", ""))), nil},
		{"PEPPOL-EN16931-R043 percentage and base amount", one(before(uTaxTotal, uAllowance("1", "10", "50.00"))), ids("PEPPOL-EN16931-R043")},
		// R041 and R042 match first: R043 is not tested.
		{"PEPPOL-EN16931-R041 percentage only", one(before(uTaxTotal, uAllowance("1", "10", ""))), ids("PEPPOL-EN16931-R041")},
		{"PEPPOL-EN16931-R042 base amount only", one(before(uTaxTotal, uAllowance("1", "", "50.00"))), ids("PEPPOL-EN16931-R042")},
		{"PEPPOL-EN16931-R043 line allowance", one(rep(uLine1Item, uAllowance("False", "", "")+uLine1Item)), ids("PEPPOL-EN16931-R043")},
		{"PEPPOL-EN16931-R043 credit note line", edits(one(rep(uLine1Item, uAllowance("", "", "")+uLine1Item)), creditNote), ids("PEPPOL-EN16931-R008", "PEPPOL-EN16931-R043")},

		// Peppol rules of the XRechnung schematron.
		{"PEPPOL-EN16931-R001 no business process", one(remove(uProfile)), ids("PEPPOL-EN16931-R001")},
		{"PEPPOL-EN16931-R005 VAT accounting currency of the invoice", one(after(uCurrency, cbc("TaxCurrencyCode", "EUR"))), ids("PEPPOL-EN16931-R005", "PEPPOL-EN16931-R054")},
		{"PEPPOL-EN16931-R053 two tax totals with subtotals", one(before(uMonetaryTotal, uTaxTotalZero)), ids("PEPPOL-EN16931-R053")},
		{"PEPPOL-EN16931-R054 no tax total in the accounting currency", one(after(uCurrency, cbc("TaxCurrencyCode", "USD"))), ids("PEPPOL-EN16931-R054", "PEPPOL-EN16931-R055")},
		{"PEPPOL-EN16931-R054 tax total in the accounting currency", one(after(uCurrency, cbc("TaxCurrencyCode", "USD")), before(uMonetaryTotal, uTaxTotalUSD("31.00"))), nil},
		{"PEPPOL-EN16931-R054 tax total without accounting currency", one(before(uMonetaryTotal, uTaxTotalUSD("31.00"))), ids("PEPPOL-EN16931-R054")},
		{"PEPPOL-EN16931-R055 opposite signs", one(after(uCurrency, cbc("TaxCurrencyCode", "USD")), before(uMonetaryTotal, uTaxTotalUSD("-31.00"))), ids("PEPPOL-EN16931-R055")},
		{"PEPPOL-EN16931-R010 no buyer electronic address", one(remove(uBuyerParty)), ids("PEPPOL-EN16931-R010")},
		{"PEPPOL-EN16931-R020 no seller electronic address", one(remove(uEndpoint)), ids("PEPPOL-EN16931-R020")},
		{"PEPPOL-EN16931-R040 amount", one(before(uTaxTotal, uAllowance("false", "10", "60.00"))), ids("PEPPOL-EN16931-R040")},
		{"PEPPOL-EN16931-R040 within the slack", one(before(uTaxTotal, uAllowance("false", "10", "50.20"))), nil},
		{"PEPPOL-EN16931-R040 HUF slack", one(before(uTaxTotal, uAllowance("false", "10", "54.00")), rep(uCurrency, cbc("DocumentCurrencyCode", "HUF")), repAll(`currencyID="EUR"`, `currencyID="HUF"`)), nil},
		{"PEPPOL-EN16931-R040 beyond the HUF slack", one(before(uTaxTotal, uAllowance("false", "10", "56.00")), rep(uCurrency, cbc("DocumentCurrencyCode", "HUF")), repAll(`currencyID="EUR"`, `currencyID="HUF"`)), ids("PEPPOL-EN16931-R040")},
		{"PEPPOL-EN16931-R110 line period starts before", one(before(uARD, uPeriod), before(uLine1Item, uLinePeriod("2026-09-30", "2026-10-31"))), ids("PEPPOL-EN16931-R110")},
		{"PEPPOL-EN16931-R111 line period ends after", one(before(uARD, uPeriod), before(uLine1Item, uLinePeriod("2026-10-01", "2026-11-01"))), ids("PEPPOL-EN16931-R111")},
		{"PEPPOL-EN16931-R111 credit note", edits(one(before(uARD, uPeriod), before(uLine1Item, uLinePeriod("2026-10-01", "2026-11-01"))), creditNote), ids("PEPPOL-EN16931-R111")},
		{"PEPPOL-EN16931-R110 needs an invoice start date", one(before(uARD, "<cac:InvoicePeriod><cbc:EndDate>2026-10-31</cbc:EndDate></cac:InvoicePeriod>"), before(uLine1Item, uLinePeriod("2026-09-01", "2026-10-31"))), nil},
		{"PEPPOL-EN16931-R120 line net amount", one(rep(uQty1, `<cbc:InvoicedQuantity unitCode="HUR">11</cbc:InvoicedQuantity>`)), ids("PEPPOL-EN16931-R120")},
		{"PEPPOL-EN16931-R120 credit note", edits(one(rep(uQty1, `<cbc:InvoicedQuantity unitCode="HUR">11</cbc:InvoicedQuantity>`)), creditNote), ids("PEPPOL-EN16931-R120")},
		{"PEPPOL-EN16931-R120 within the slack", one(rep(uPrice1, `<cbc:PriceAmount currencyID="EUR">12.502</cbc:PriceAmount>`)), nil},
		{"PEPPOL-EN16931-R120 line allowance", one(rep(uLine1Item, uAllowance("false", "", "")+uLine1Item)), ids("PEPPOL-EN16931-R120")},
		// A zero base quantity counts as 1 in R120.
		{"PEPPOL-EN16931-R121 zero base quantity", one(rep(uBaseQty, `<cbc:BaseQuantity unitCode="HUR">0</cbc:BaseQuantity>`)), ids("PEPPOL-EN16931-R121")},
		{"PEPPOL-EN16931-R101 line object", one(before(uLine1Item, uLineObject("916"))), ids("PEPPOL-EN16931-R101")},
		{"PEPPOL-EN16931-R101 type code 130", one(before(uLine1Item, uLineObject("130"))), nil},
		{"PEPPOL-EN16931-R044 charge on the price", one(before(uLine1Price, uPriceAllowance("true", ""))), ids("PEPPOL-EN16931-R044")},
		{"PEPPOL-EN16931-R044 spaced indicator", one(before(uLine1Price, uPriceAllowance(" false ", ""))), nil},
		{"PEPPOL-EN16931-R046 net price", one(before(uLine1Price, uPriceAllowance("false", "13.00"))), ids("PEPPOL-EN16931-R046")},
		{"PEPPOL-EN16931-R046 base amount minus allowance", one(before(uLine1Price, uPriceAllowance("false", "13.50"))), nil},
		{"PEPPOL-EN16931-R130 unit of the base quantity", one(rep(uBaseQty, `<cbc:BaseQuantity unitCode="H87">1</cbc:BaseQuantity>`)), ids("PEPPOL-EN16931-R130")},
		{"PEPPOL-EN16931-R008 empty order reference identifier", one(after(uBuyerRef, "<cac:OrderReference><cbc:ID></cbc:ID></cac:OrderReference>")), nil},
		{"PEPPOL-EN16931-R008 empty order reference", one(after(uBuyerRef, "<cac:OrderReference><cbc:ID>4711</cbc:ID><cbc:SalesOrderID/></cac:OrderReference>")), ids("PEPPOL-EN16931-R008")},
	}
}

// Anchors of the UBL base invoice for the Peppol rules.
const (
	uProfile       = "<cbc:ProfileID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</cbc:ProfileID>"
	uCurrency      = "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>"
	uMonetaryTotal = "<cac:LegalMonetaryTotal>"
	uQty1          = `<cbc:InvoicedQuantity unitCode="HUR">10</cbc:InvoicedQuantity>`
	uPrice1        = `<cbc:PriceAmount currencyID="EUR">12.50</cbc:PriceAmount>`
	uBaseQty       = `<cbc:BaseQuantity unitCode="HUR">1</cbc:BaseQuantity>`
	// uTaxTotalZero is a second tax total with a VAT breakdown.
	uTaxTotalZero = `<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount>` +
		"<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>"
)

// uTaxTotalUSD returns a tax total without VAT breakdown in US dollars.
func uTaxTotalUSD(amount string) string {
	return `<cac:TaxTotal><cbc:TaxAmount currencyID="USD">` + amount + "</cbc:TaxAmount></cac:TaxTotal>"
}

// uLinePeriod returns the period of a line.
func uLinePeriod(start, end string) string {
	return "<cac:InvoicePeriod>" + cbc("StartDate", start) + cbc("EndDate", end) + "</cac:InvoicePeriod>"
}

// uLineObject returns a document reference of a line with the type code.
func uLineObject(code string) string {
	return "<cac:DocumentReference>" + cbc("ID", "OBJ-1") + cbc("DocumentTypeCode", code) + "</cac:DocumentReference>"
}

// uPriceAllowance returns an allowance of 1.00 EUR on the price of a line,
// with the optional base amount (the gross price).
func uPriceAllowance(indicator, base string) string {
	s := "<cac:AllowanceCharge>" + cbc("ChargeIndicator", indicator) + `<cbc:Amount currencyID="EUR">1.00</cbc:Amount>`
	if base != "" {
		s += `<cbc:BaseAmount currencyID="EUR">` + base + "</cbc:BaseAmount>"
	}
	return s + "</cac:AllowanceCharge>"
}

func TestUBLMutations(t *testing.T) {
	doc := load(t, ublBase)
	for _, c := range ublCases() {
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

// checkUBLFindings verifies that every finding is in UBLCatalog, with its
// severity, located in the UBL document and without CII names.
func checkUBLFindings(t *testing.T, r *rules.Report) {
	t.Helper()
	checkFindings(t, r, UBLCatalog)
	for _, f := range r.Findings {
		if !strings.HasPrefix(f.Path, "/Invoice") && !strings.HasPrefix(f.Path, "/CreditNote") {
			t.Errorf("%s: path %q is not in the UBL document", f.Rule, f.Path)
		}
		if strings.Contains(f.Message, "ram:") {
			t.Errorf("%s: CII name in %q", f.Rule, f.Message)
		}
	}
}

// TestUBLCoverage checks that every assertion of UBLCatalog fails in at
// least one UBL mutation case.
func TestUBLCoverage(t *testing.T) {
	failing := map[string]bool{}
	for _, c := range ublCases() {
		for _, id := range c.want {
			failing[id] = true
		}
	}
	for _, r := range UBLCatalog {
		if !failing[r.ID] {
			t.Errorf("no UBL mutation case fails %s", r.ID)
		}
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
		if r.Description == "" || strings.Contains(r.Description, "ram:") {
			t.Errorf("bad description for %s: %q", r.ID, r.Description)
		}
		if c, ok := catalogEntry(Catalog, r.ID); ok && c != r {
			t.Errorf("%s differs from Catalog:\n%+v\n%+v", r.ID, r, c)
		}
	}
	if len(UBLCatalog) != 77 || sev[rules.Error] != 66 || sev[rules.Warning] != 10 || sev[rules.Info] != 1 {
		t.Errorf("%d assertions (%d errors, %d warnings, %d information), want 77 (66, 10, 1)", len(UBLCatalog), sev[rules.Error], sev[rules.Warning], sev[rules.Info])
	}
	var ciiOnly, ublOnly []string
	for _, r := range Catalog {
		if !seen[r.ID] {
			ciiOnly = append(ciiOnly, r.ID)
		}
	}
	for _, r := range UBLCatalog {
		if _, ok := catalogEntry(Catalog, r.ID); !ok {
			ublOnly = append(ublOnly, r.ID)
		}
	}
	slices.Sort(ciiOnly)
	slices.Sort(ublOnly)
	if want := []string{"BR-DEX-15", "BR-TMP-3", "BR-TMP-4", "BR-TMP-5", "BR-TMP-7", "PEPPOL-EN16931-R043-1", "PEPPOL-EN16931-R043-2"}; !slices.Equal(ciiOnly, want) {
		t.Errorf("CII only rules = %q, want %q", ciiOnly, want)
	}
	if want := []string{"BR-DEX-02", "BR-DEX-03", "BR-DEX-09", "BR-DEX-10", "BR-DEX-11", "BR-DEX-12", "BR-DEX-13", "BR-DEX-14", "BR-TMP-6", "PEPPOL-EN16931-R043"}; !slices.Equal(ublOnly, want) {
		t.Errorf("UBL only rules = %q, want %q", ublOnly, want)
	}
}

func TestUBLEmpty(t *testing.T) {
	for _, doc := range []string{
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`,
		`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`,
	} {
		r := runUBL(t, []byte(doc))
		checkUBLFindings(t, r)
		if want := []string{"BR-DE-1", "BR-DE-15", "BR-DE-17", "BR-DE-21", "PEPPOL-EN16931-R001", "PEPPOL-EN16931-R008", "PEPPOL-EN16931-R053"}; !slices.Equal(r.Rules(), want) {
			t.Errorf("%s: rules = %q, want %q", doc, r.Rules(), want)
		}
	}
}

// BenchmarkCheckUBL runs Check on the UBL base invoice of testdata.
func BenchmarkCheckUBL(b *testing.B) {
	root, err := xmltree.Parse([]byte(load(b, ublBase)))
	if err != nil {
		b.Fatal(err)
	}
	doc := ubl.View(root, invoice.ProfileUnknown)
	b.ReportAllocs()
	for b.Loop() {
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceDE})
		if !r.Valid() {
			b.Fatal(r)
		}
	}
}
