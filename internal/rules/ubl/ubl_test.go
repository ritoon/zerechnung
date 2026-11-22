package ubl

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/en16931"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The unit tests mutate a small valid EN 16931 UBL invoice. Each case lists
// the exact set of rules it must report, from the business rules run on the
// CII view (package en16931) and the UBL rules of this package.

const tInvoice = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID>
<cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:DueDate>2024-02-15</cbc:DueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>
<!--notes-->
<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<!--references-->
<cac:AccountingSupplierParty><cac:Party>
<cac:PostalAddress><!--address--><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
<cac:PartyTaxScheme><cbc:CompanyID>DE123456789</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
<cac:PartyLegalEntity><cbc:RegistrationName>Seller</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
<!--buyer-->
<cac:PostalAddress><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
<cac:PartyLegalEntity><cbc:RegistrationName>Buyer</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<!--payment-->
<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>
</cac:TaxTotal>
<cac:LegalMonetaryTotal>
<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
<cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount>
<cbc:TaxInclusiveAmount currencyID="EUR">119.00</cbc:TaxInclusiveAmount>
<cbc:PayableAmount currencyID="EUR">119.00</cbc:PayableAmount>
</cac:LegalMonetaryTotal>
<cac:InvoiceLine>
<cbc:ID>1</cbc:ID>
<!--line-->
<cbc:InvoicedQuantity unitCode="C62">10</cbc:InvoicedQuantity>
<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
<cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory><!--item--></cac:Item>
<cac:Price><cbc:PriceAmount currencyID="EUR">10.00</cbc:PriceAmount></cac:Price>
</cac:InvoiceLine>
</Invoice>
`

// cenSpecification is the specification identifier of tInvoice.
const cenSpecification = "<cbc:CustomizationID>urn:cen.eu:en16931:2017</cbc:CustomizationID>"

// creditNote turns tInvoice into a credit note: BT-9 moves to
// cac:PaymentMeans/cbc:PaymentDueDate.
func creditNote(s string) string {
	r := strings.NewReplacer(
		"<Invoice ", "<CreditNote ", "</Invoice>", "</CreditNote>", "schema:xsd:Invoice-2", "schema:xsd:CreditNote-2",
		"<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:CreditNoteTypeCode>381</cbc:CreditNoteTypeCode>",
		"<cbc:DueDate>2024-02-15</cbc:DueDate>\n", "",
		"<!--payment-->", "<cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode><cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate></cac:PaymentMeans>",
		"cac:InvoiceLine>", "cac:CreditNoteLine>", "cbc:InvoicedQuantity", "cbc:CreditedQuantity",
	)
	return r.Replace(s)
}

// check runs the EN 16931 rules on the CII view and the UBL rules on doc.
func check(t *testing.T, doc string) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !IsUBL(root) {
		t.Fatalf("IsUBL = false")
	}
	d := View(root, invoice.ProfileUnknown)
	r := &rules.Report{}
	en16931.Check(&rules.Checker{Doc: d, Report: r, Source: rules.SourceEN16931})
	Check(&rules.Checker{Doc: d, Report: r, Source: rules.SourceEN16931})
	return r
}

// checkCase is a mutation of tInvoice.
type checkCase struct {
	name     string
	old, new string // replacement in tInvoice
	old2     string // second replacement
	new2     string
	credit   bool // apply creditNote
	want     []string
}

// doc returns the document of the case.
func (c checkCase) doc(t testing.TB) string {
	t.Helper()
	doc := tInvoice
	if c.credit {
		doc = creditNote(doc)
	}
	for _, r := range [][2]string{{c.old, c.new}, {c.old2, c.new2}} {
		if r[0] == "" {
			continue
		}
		if !strings.Contains(doc, r[0]) {
			t.Fatalf("%q not found in the template", r[0])
		}
		doc = strings.Replace(doc, r[0], r[1], 1)
	}
	return doc
}

func checkCases() []checkCase {
	ext := `<ext:UBLExtensions xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"><ext:UBLExtension><ext:ExtensionContent/></ext:UBLExtension></ext:UBLExtensions>`
	// The payment elements of the Dutch G-account extension, and its
	// specification identifier.
	gaPayment := `<cac:PaymentMeans><cbc:ID>GACCOUNT</cbc:ID><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode></cac:PaymentMeans><cac:PaymentTerms><!--terms--><cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID><cbc:Amount currencyID="EUR">119.00</cbc:Amount></cac:PaymentTerms>`
	gaSpec := "<cbc:CustomizationID>" + invoice.URNNLCIUS + rules.GAccountExtension + "</cbc:CustomizationID>"
	return []checkCase{
		{name: "valid"},
		{name: "valid credit note", credit: true},
		{name: "extensions", old: "<cbc:CustomizationID>", new: ext + "<cbc:CustomizationID>", want: []string{"UBL-CR-001"}},
		{name: "UBL 2.1", old: "<cbc:CustomizationID>", new: "<cbc:UBLVersionID>2.1</cbc:UBLVersionID><cbc:CustomizationID>"},
		{name: "UBL 2.0", old: "<cbc:CustomizationID>", new: "<cbc:UBLVersionID>2.0</cbc:UBLVersionID><cbc:CustomizationID>", want: []string{"UBL-CR-002"}},
		{name: "payment due date in invoice", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode><cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate></cac:PaymentMeans>`, want: []string{"UBL-CR-412"}},
		{name: "line UUID", old: "<!--line-->", new: "<cbc:UUID>x</cbc:UUID>", want: []string{"UBL-CR-515"}},
		{name: "unit code list", old: `unitCode="C62"`, new: `unitCode="C62" unitCodeListID="UNECERec20"`, want: []string{"UBL-CR-663"}},
		{name: "tax scheme company scheme", old: "<cbc:CompanyID>DE123456789", new: `<cbc:CompanyID schemeID="VAT">DE123456789`, want: []string{"UBL-CR-652"}},
		{name: "supporting document scheme", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID schemeID="AAA">D1</cbc:ID></cac:AdditionalDocumentReference>`, want: []string{"UBL-CR-665", "UBL-SR-43"}},
		{name: "supporting document type", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cbc:DocumentTypeCode>916</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`, want: []string{"UBL-SR-43"}},
		{name: "invoiced object", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID schemeID="AAA">D1</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`},
		{name: "invoiced object with attachment", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode><cbc:DocumentDescription>x</cbc:DocumentDescription><cac:Attachment><cac:ExternalReference><cbc:URI>https://example.com/d1</cbc:URI></cac:ExternalReference></cac:Attachment></cac:AdditionalDocumentReference>`, want: []string{"UBL-CR-666", "UBL-CR-673"}},
		{name: "invoiced object scheme", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID schemeID="XXX">D1</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`, want: []string{"BR-CL-07"}},
		{name: "two invoiced objects", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference><cac:AdditionalDocumentReference><cbc:ID>D2</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`, want: []string{"UBL-SR-04"}},
		{name: "credit note preceding invoice scheme", credit: true, old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID schemeID="AAA">P1</cbc:ID><cbc:DocumentTypeCode>50</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`, want: []string{"UBL-CR-665"}},
		{name: "address lines", old: "<!--address-->", new: `<cac:AddressLine><cbc:Line>a</cbc:Line></cac:AddressLine><cac:AddressLine><cbc:Line>b</cbc:Line></cac:AddressLine>`, want: []string{"UBL-SR-51"}},
		{name: "two line tax categories", old: "<!--item-->", new: `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`, want: []string{"UBL-SR-48"}},
		{name: "payee is seller", old: "<!--payment-->", new: `<cac:PayeeParty><cac:PartyName><cbc:Name>Seller</cbc:Name></cac:PartyName></cac:PayeeParty>`, want: []string{"UBL-SR-19", "UBL-SR-20", "UBL-SR-21"}},
		{name: "payee", old: "<!--payment-->", new: `<cac:PayeeParty><cac:PartyName><cbc:Name>Factor</cbc:Name></cac:PartyName></cac:PayeeParty>`},
		{name: "payment identifiers", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode><cbc:PaymentID>A</cbc:PaymentID></cac:PaymentMeans><cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode><cbc:PaymentID>B</cbc:PaymentID></cac:PaymentMeans>`, want: []string{"UBL-SR-44"}},
		{name: "payment means codes", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode></cac:PaymentMeans><cac:PaymentMeans><cbc:PaymentMeansCode>10</cbc:PaymentMeansCode></cac:PaymentMeans>`, want: []string{"UBL-SR-47"}},
		{name: "amount decimals", old: `<cbc:PayableAmount currencyID="EUR">119.00<`, new: `<cbc:PayableAmount currencyID="EUR">119.000<`, want: []string{"BR-DEC-18", "UBL-DT-01"}},
		{name: "binary object", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject>AA==</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>`, want: []string{"UBL-DT-06", "UBL-DT-07"}},
		{name: "binary object mime", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="text/plain" filename="d1.txt">AA==</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>`, want: []string{"BR-CL-24"}},
		{name: "scheme name", old: "<cbc:ID>INV-1</cbc:ID>", new: `<cbc:ID schemeName="x">INV-1</cbc:ID>`, want: []string{"UBL-DT-08"}},
		{name: "name attribute", old: "<cbc:InvoiceTypeCode>", new: `<cbc:InvoiceTypeCode name="x">`, want: []string{"UBL-DT-18"}},
		{name: "payment means name", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode name="Cash">10</cbc:PaymentMeansCode></cac:PaymentMeans>`},
		{name: "payment means names", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode name="Cash">10</cbc:PaymentMeansCode></cac:PaymentMeans><cac:PaymentMeans><cbc:PaymentMeansCode name="Cash">10</cbc:PaymentMeansCode></cac:PaymentMeans>`, want: []string{"UBL-SR-46"}},
		{name: "seller identifiers of another tax scheme", old: "</cac:PartyTaxScheme>", new: `</cac:PartyTaxScheme><cac:PartyTaxScheme><cbc:CompanyID>201/113/40209</cbc:CompanyID><cbc:CompanyID>201/113/40210</cbc:CompanyID><cac:TaxScheme><cbc:ID>FC</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>`, want: []string{"UBL-SR-13"}},
		{name: "G-account elements", old: "<!--payment-->", new: gaPayment, want: []string{"UBL-CR-411", "UBL-CR-453", "UBL-CR-459"}},
		{name: "G-account elements with the extension", old: "<!--payment-->", new: gaPayment, old2: cenSpecification, new2: gaSpec},
		{name: "G-account extension keeps the other syntax rules", old: "<!--payment-->", new: strings.Replace(gaPayment, "<!--terms-->", "<cbc:ID>T1</cbc:ID>", 1), old2: cenSpecification, new2: gaSpec, want: []string{"UBL-CR-452"}},
		{name: "G-account extension of another identifier", old: "<!--payment-->", new: gaPayment, old2: cenSpecification, new2: "<cbc:CustomizationID>urn:cen.eu:en16931:2017" + rules.GAccountExtension + "</cbc:CustomizationID>"},
		{name: "invoice type code of a credit note", old: "<cbc:InvoiceTypeCode>380<", new: "<cbc:InvoiceTypeCode>381<", want: []string{"BR-CL-01"}},
		{name: "credit note type code of an invoice", credit: true, old: "<cbc:CreditNoteTypeCode>381<", new: "<cbc:CreditNoteTypeCode>380<", want: []string{"BR-CL-01"}},
		{name: "note subject", old: "<!--notes-->", new: "<cbc:Note>#AAI#Text</cbc:Note><cbc:Note>#AB#Text</cbc:Note><cbc:Note>#AAI Text</cbc:Note>"},
		{name: "note subject code", old: "<!--notes-->", new: "<cbc:Note>#ZZY#Text</cbc:Note>", want: []string{"BR-CL-08"}},
		{name: "seller SEPA identifier", old: "<cac:PostalAddress><!--address-->", new: `<cac:PartyIdentification><cbc:ID schemeID="SEPA">DE98ZZZ09999999999</cbc:ID></cac:PartyIdentification><cac:PostalAddress>`},
		{name: "buyer SEPA identifier", old: "<!--buyer-->", new: `<cac:PartyIdentification><cbc:ID schemeID="SEPA">DE98ZZZ09999999999</cbc:ID></cac:PartyIdentification>`, want: []string{"BR-CL-10"}},
		{name: "tax point date code", old: "<!--references-->", new: `<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>`},
		{name: "tax point date code list", old: "<!--references-->", new: `<cac:InvoicePeriod><cbc:DescriptionCode>5</cbc:DescriptionCode></cac:InvoicePeriod>`, want: []string{"BR-CL-06"}},
		{name: "tax point date", old: "<!--notes-->", new: `<cbc:TaxPointDate>2024-01-10</cbc:TaxPointDate>`},
		{name: "tax point date and code", old: "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>\n<!--references-->", new: `<cbc:TaxPointDate>2024-01-10</cbc:TaxPointDate><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode><cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>`, want: []string{"BR-CO-03"}},
		{name: "VAT accounting currency", old: "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>", new: "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode><cbc:TaxCurrencyCode>USD</cbc:TaxCurrencyCode>", want: []string{"BR-53"}},
		{name: "amount currency", old: `<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
<cac:Item>`, new: `<cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount>
<cac:Item>`, want: []string{"BR-CL-03"}},
		{name: "unit code", old: `unitCode="C62"`, new: `unitCode="XYZ1"`, want: []string{"BR-CL-23"}},
		{name: "country code", old: "<!--address--><cac:Country><cbc:IdentificationCode>DE<", new: "<!--address--><cac:Country><cbc:IdentificationCode>XX<", want: []string{"BR-CL-14"}},

		// Business rules whose UBL context or test differs from CII.
		{name: "seller without address", old: "<cac:PostalAddress><!--address--><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>", new: "", want: []string{"BR-08"}},
		{name: "seller address without country", old: "<cac:PostalAddress><!--address--><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>", new: "<cac:PostalAddress><cbc:CityName>Berlin</cbc:CityName></cac:PostalAddress>", want: []string{"BR-09"}},
		{name: "payee with the seller trading name", old: "<cac:PostalAddress><!--address-->", new: "<cac:PartyName><cbc:Name>Trade</cbc:Name></cac:PartyName><cac:PostalAddress>",
			old2: "<!--payment-->", new2: `<cac:PayeeParty><cac:PartyName><cbc:Name>Trade</cbc:Name></cac:PartyName></cac:PayeeParty>`, want: []string{"BR-17"}},
		{name: "payee without name", old: "<!--payment-->", new: `<cac:PayeeParty><cac:PartyIdentification><cbc:ID>P1</cbc:ID></cac:PartyIdentification></cac:PayeeParty>`, want: []string{"BR-17", "UBL-SR-19", "UBL-SR-20", "UBL-SR-21"}},
		{name: "credit transfer without account", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode></cac:PaymentMeans>`, want: []string{"BR-61"}},
		{name: "credit transfer", old: "<!--payment-->", new: `<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>DE89370400440532013000</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`},
		{name: "endpoint without scheme", old: "<cac:PostalAddress><!--address-->", new: "<cbc:EndpointID>seller@example.com</cbc:EndpointID><cac:PostalAddress>", want: []string{"BR-62"}},
		{name: "endpoint with an empty scheme", old: "<cac:PostalAddress><!--address-->", new: `<cbc:EndpointID schemeID="">seller@example.com</cbc:EndpointID><cac:PostalAddress>`, want: []string{"BR-CL-25"}},
		{name: "decimals as text", old: `<cbc:TaxExclusiveAmount currencyID="EUR">100.00<`, new: `<cbc:TaxExclusiveAmount currencyID="EUR">100.000<`, want: []string{"BR-DEC-12", "UBL-DT-01"}},
		{name: "empty supporting document reference", old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID> </cbc:ID></cac:AdditionalDocumentReference>`, want: []string{"BR-52"}},
		{name: "empty tender reference", old: "<!--references-->", new: `<cac:OriginatorDocumentReference><cbc:ID></cbc:ID></cac:OriginatorDocumentReference>`},
		{name: "credit note empty project reference", credit: true, old: "<!--references-->", new: `<cac:AdditionalDocumentReference><cbc:ID></cbc:ID><cbc:DocumentTypeCode>50</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`, want: []string{"BR-52"}},
		{name: "preceding invoice without reference", old: "<!--references-->", new: `<cac:BillingReference><cac:InvoiceDocumentReference><cbc:IssueDate>2024-01-01</cbc:IssueDate></cac:InvoiceDocumentReference></cac:BillingReference>`, want: []string{"BR-55", "UBL-SR-07"}},
		{name: "tax representative without VAT identifier", old: "<!--payment-->", new: `<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep</cbc:Name></cac:PartyName><cac:PostalAddress><cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>`, want: []string{"BR-56"}},
		{name: "tax representative without address", old: "<!--payment-->", new: `<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep</cbc:Name></cac:PartyName><cac:PartyTaxScheme><cbc:CompanyID>DE987654321</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme></cac:TaxRepresentativeParty>`, want: []string{"BR-19"}},
		{name: "item identifier without scheme", old: "<!--item-->", new: "<cac:StandardItemIdentification><cbc:ID>4000001123452</cbc:ID></cac:StandardItemIdentification>", want: []string{"BR-64"}},
		{name: "item classification without scheme", old: "<!--item-->", new: "<cac:CommodityClassification><cbc:ItemClassificationCode>9873242</cbc:ItemClassificationCode></cac:CommodityClassification>", want: []string{"BR-65"}},
		// BR-x-01: the UBL tests differ from the CII ones. A category that
		// appears on the lines needs exactly one breakdown of that category
		// (BR-O-01), a standard rated breakdown needs standard rated lines
		// (BR-S-01), and a breakdown alone satisfies BR-Z-01.
		{name: "lines not subject to VAT", old: `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent>`, new: `<cac:ClassifiedTaxCategory><cbc:ID>O</cbc:ID>`, want: []string{"BR-O-01", "BR-O-02", "BR-S-01", "BR-S-08"}},
		// BR-CO-14 compares each tax total with its own breakdown.
		{name: "second tax total with a breakdown", old: "</cac:TaxTotal>", new: `</cac:TaxTotal><cac:TaxTotal><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>Z</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>`, want: []string{"BR-CO-15"}},
		{name: "tax total of another breakdown", old: "</cac:TaxTotal>", new: `</cac:TaxTotal><cac:TaxTotal><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>Z</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>`, want: []string{"BR-CO-14", "BR-CO-15"}},
		{name: "zero rated breakdown without lines", old: "</cac:TaxTotal>", new: `<cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>Z</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>`},
	}
}

func TestCheck(t *testing.T) {
	for _, c := range checkCases() {
		t.Run(c.name, func(t *testing.T) {
			r := check(t, c.doc(t))
			got := r.Rules()
			want := append([]string(nil), c.want...)
			sort.Strings(want)
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("rules = %v, want %v\n%s", got, want, r)
			}
			for _, f := range r.Findings {
				if f.Message == "" || f.Path == "" || f.Line == 0 {
					t.Errorf("incomplete finding %+v", f)
				}
			}
		})
	}
}

// TestLocation checks that the findings of the business rules, which run on
// the CII view, are located in the UBL document.
func TestLocation(t *testing.T) {
	doc := strings.Replace(tInvoice, `<cbc:TaxInclusiveAmount currencyID="EUR">119.00<`, `<cbc:TaxInclusiveAmount currencyID="EUR">118.00<`, 1)
	r := check(t, doc)
	var f *rules.Finding
	for i := range r.Findings {
		if r.Findings[i].Rule == "BR-CO-15" {
			f = &r.Findings[i]
		}
	}
	if f == nil {
		t.Fatalf("no BR-CO-15:\n%s", r)
	}
	if want := "/Invoice/cac:LegalMonetaryTotal/cbc:TaxInclusiveAmount"; f.Path != want {
		t.Errorf("path = %q, want %q", f.Path, want)
	}
	if want := strings.Count(tInvoice[:strings.Index(tInvoice, "<cbc:TaxInclusiveAmount")], "\n") + 1; f.Line != want {
		t.Errorf("line = %d, want %d", f.Line, want)
	}
}

// TestSeverity checks the official severities and the XRechnung levels.
func TestSeverity(t *testing.T) {
	doc := strings.Replace(tInvoice, "<cbc:CustomizationID>", "<cbc:UBLVersionID>2.0</cbc:UBLVersionID><cbc:CustomizationID>", 1)
	doc = strings.Replace(doc, "<!--line-->", "<cbc:UUID>x</cbc:UUID>", 1)
	r := check(t, doc)
	sev := map[string]rules.Severity{}
	for _, f := range r.Findings {
		sev[f.Rule] = f.Severity
	}
	if sev["UBL-CR-002"] != rules.Warning || sev["UBL-CR-515"] != rules.Warning {
		t.Errorf("severities = %v, want warnings", sev)
	}
}

// TestNoUBL checks that Check ignores a CII document and the profiles that
// are not EN 16931 invoices.
func TestNoUBL(t *testing.T) {
	root, err := xmltree.Parse([]byte(`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if IsUBL(root) {
		t.Errorf("IsUBL(CII) = true")
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r})
	Check(&rules.Checker{Doc: nil, Report: r})
	ubl, err := xmltree.Parse([]byte(strings.Replace(tInvoice, "<!--line-->", "<cbc:UUID>x</cbc:UUID>", 1)))
	if err != nil {
		t.Fatal(err)
	}
	Check(&rules.Checker{Doc: View(ubl, invoice.ProfileMinimum), Report: r})
	if len(r.Findings) > 0 {
		t.Errorf("findings:\n%s", r)
	}
}

// TestCatalog checks that every UBL-CR assertion of the table has its
// description in the catalog, and that the identifiers are unique.
func TestCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Catalog {
		if seen[r.ID] {
			t.Errorf("duplicate %s", r.ID)
		}
		seen[r.ID] = true
		if r.Description == "" || r.Severity == 0 {
			t.Errorf("incomplete %+v", r)
		}
	}
	for _, cr := range crRules {
		if !seen[cr.id] {
			t.Errorf("%s not in Catalog", cr.id)
		}
	}
	for _, id := range []string{"UBL-CR-002", "UBL-CR-412", "UBL-CR-665", "UBL-CR-666", "UBL-CR-673", "UBL-SR-01", "UBL-SR-59", "UBL-DT-01", "UBL-DT-28"} {
		if !seen[id] {
			t.Errorf("%s not in Catalog", id)
		}
	}
	if n := len(Catalog); n != 759 {
		t.Errorf("len(Catalog) = %d, want 759", n)
	}
}

// TestFiles checks that the valid UBL examples of testdata, of the corpus
// (FX_CORPUS, XML-Rechnung/UBL) and of the KoSIT test suite (FX_KOSIT)
// report no error.
func TestFiles(t *testing.T) {
	files, _ := filepath.Glob("../../../testdata/*.ubl.xml")
	more, _ := filepath.Glob("../../../testdata/ubl-*.xml")
	files = append(files, more...)
	if dir := os.Getenv("FX_CORPUS"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "XML-Rechnung", "UBL", "*.xml"))
		files = append(files, more...)
	}
	if dir := os.Getenv("FX_KOSIT"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "src", "test", "*", "*", "*_ubl.xml"))
		files = append(files, more...)
	}
	// Known errors: the scheme 9958 (Leitweg-ID) is not in the EAS list of
	// the CEN artefacts, and one corpus file is invalid on purpose.
	known := map[string][]string{
		"EN16931_ElektronischeAdresse.ubl.xml":                                    {"BR-CL-25"},
		"EN16931_RechnungsUebertragung.ubl.xml":                                   {"BR-CL-25"},
		"not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.ubl.xml": {"BR-CO-10", "BR-CO-15"},
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r := check(t, string(data))
		var errs []string
		for _, x := range r.Errors() {
			errs = append(errs, x.Rule)
		}
		sort.Strings(errs)
		if want := known[filepath.Base(f)]; strings.Join(errs, " ") != strings.Join(want, " ") {
			t.Errorf("%s: errors %v, want %v\n%s", filepath.Base(f), errs, want, r)
		}
	}
}
