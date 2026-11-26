package peppol

import (
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// Peppol BIS Self-Billing 3.0, release 3.1.0 (November 2026, mandatory
// from 22/02/2027): the buyer issues the invoice, or the credit note, on
// behalf of the seller. The document is a UBL 2.1 invoice of Peppol BIS
// Billing 3.0, with its own specification identifier (BT-24), business
// process (BT-23) and type codes (389 and 527 for an invoice, 261 for a
// credit note); the seller and the buyer keep their roles.
//
// Its schematron, PEPPOL-EN16931-UBL-SB.sch, is the UBL schematron of
// Peppol BIS Billing 3.0 that UBLCatalog implements (release 3.1.0 too),
// without the national rules, which each country is to review for
// self-billing, and without PEPPOL-EN16931-P0112. It differs from it only
// by:
//
//   - PEPPOL-EN16931-R004, which requires the Self-Billing identifier in
//     BT-24;
//   - the profile ($profile): 01 is the Self-Billing process
//     (urn:fdc:peppol.eu:2017:poacc:selfbilling:01:1.0) or, as for Billing,
//     one of the two French Peppol processes, 02 billing with response;
//   - PEPPOL-EN16931-P0100 and P0101, which admit, in profile 01 only, the
//     type codes 389 and 527 for an invoice and 261 for a credit note;
//   - the order of the identifier rules: the schemes 0096 and 0198
//     (PEPPOL-COMMON-R052 and R053) come after the Belgian VAT identifier
//     (PEPPOL-COMMON-R062), which therefore checks a VAT identifier
//     starting with BE that carries one of these schemes.
//
// The other rules, the rules of cardinality included, their tests, flags
// and messages are those of PEPPOL-EN16931-UBL.sch. A Self-Billing invoice
// gets the rules of EN 16931 of packages en16931 and ubl, as a Billing one:
// Peppol publishes the same CEN-EN16931-UBL.sch with both. Peppol BIS
// Self-Billing has no CII syntax: Check applies the rules of Billing to a
// CII invoice, whatever its profile.

// Identifiers of Peppol BIS Self-Billing 3.0.
const (
	// SelfBillingSpecificationID is the specification identifier (BT-24) of
	// Peppol BIS Self-Billing 3.0; PEPPOL-EN16931-R004 of Self-Billing
	// requires BT-24 to start with it.
	SelfBillingSpecificationID = invoice.URNPeppolSB
	// ProcessSelfBilling is the business process identifier (BT-23) of
	// profile 01, self-billing.
	ProcessSelfBilling = invoice.ProcessPeppolSelfBilling
)

// SelfBillingCatalog lists the 185 assertions of PEPPOL-EN16931-UBL-SB.sch,
// release 3.1.0, in the order of the schematron, with the severity of their
// flag. The descriptions are the messages of the schematron, with their
// white space normalized. Every rule has the identifier, the severity and,
// except PEPPOL-EN16931-R004, the message of the rule of UBLCatalog.
var SelfBillingCatalog = []rules.RuleInfo{
	// Empty elements.
	{ID: "PEPPOL-EN16931-R008", Severity: rules.Error, Description: "Document MUST not contain empty elements."},

	// Credit notes.
	{ID: "PEPPOL-EN16931-R080", Severity: rules.Error, Description: "Only one project reference is allowed on document level"},

	// Peppol rules (PEPPOL-EN16931-R*).
	{ID: "PEPPOL-EN16931-R001", Severity: rules.Error, Description: "Business process MUST be provided."},
	{ID: "PEPPOL-EN16931-R007", Severity: rules.Error, Description: "Business process MUST have an approved identifier."},
	{ID: "PEPPOL-EN16931-R002", Severity: rules.Error, Description: "No more than one note is allowed on document level, unless both the buyer and seller are German organizations."},
	{ID: "PEPPOL-EN16931-R003", Severity: rules.Error, Description: "A buyer reference or purchase order reference MUST be provided."},
	{ID: "PEPPOL-EN16931-R004", Severity: rules.Error, Description: "Specification identifier MUST have the value 'urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:selfbilling:3.0' and follow the format rules for the identifier."},
	{ID: "PEPPOL-EN16931-R053", Severity: rules.Error, Description: "Only one tax total with tax subtotals MUST be provided."},
	{ID: "PEPPOL-EN16931-R054", Severity: rules.Error, Description: "Only one tax total without tax subtotals MUST be provided when tax currency code is provided."},
	{ID: "PEPPOL-EN16931-R055", Severity: rules.Error, Description: "Invoice total VAT amount and Invoice total VAT amount in accounting currency MUST have the same operational sign"},
	{ID: "PEPPOL-EN16931-R005", Severity: rules.Error, Description: "VAT accounting currency code MUST be different from invoice currency code when provided."},
	{ID: "PEPPOL-EN16931-R010", Severity: rules.Error, Description: "Buyer electronic address MUST be provided"},
	{ID: "PEPPOL-EN16931-R020", Severity: rules.Error, Description: "Seller electronic address MUST be provided"},
	{ID: "PEPPOL-EN16931-R041", Severity: rules.Error, Description: "Allowance/charge base amount MUST be provided when allowance/charge percentage is provided."},
	{ID: "PEPPOL-EN16931-R042", Severity: rules.Error, Description: "Allowance/charge percentage MUST be provided when allowance/charge base amount is provided."},
	{ID: "PEPPOL-EN16931-R040", Severity: rules.Error, Description: "Allowance/charge amount must equal base amount * percentage/100 if base amount and percentage exists"},
	{ID: "PEPPOL-EN16931-R043", Severity: rules.Error, Description: "Allowance/charge ChargeIndicator value MUST equal 'true' or 'false'"},
	{ID: "PEPPOL-EN16931-R061", Severity: rules.Error, Description: "Mandate reference MUST be provided for direct debit."},
	{ID: "PEPPOL-EN16931-R051", Severity: rules.Error, Description: "All currencyID attributes must have the same value as the invoice currency code (BT-5), except for the invoice total VAT amount in accounting currency (BT-111)."},
	{ID: "PEPPOL-EN16931-R110", Severity: rules.Error, Description: "Start date of line period MUST be within invoice period."},
	{ID: "PEPPOL-EN16931-R111", Severity: rules.Error, Description: "End date of line period MUST be within invoice period."},
	{ID: "PEPPOL-EN16931-R120", Severity: rules.Error, Description: "Invoice line net amount MUST equal (Invoiced quantity * (Item net price/item price base quantity) + Sum of invoice line charge amount - sum of invoice line allowance amount"},
	{ID: "PEPPOL-EN16931-R121", Severity: rules.Error, Description: "Base quantity MUST be a positive number above zero."},
	{ID: "PEPPOL-EN16931-R100", Severity: rules.Error, Description: "Only one invoiced object is allowed pr line"},
	{ID: "PEPPOL-EN16931-R101", Severity: rules.Error, Description: "Element Document reference can only be used for Invoice line object"},
	{ID: "PEPPOL-EN16931-R044", Severity: rules.Error, Description: "Charge on price level is NOT allowed. Only value 'false' allowed."},
	{ID: "PEPPOL-EN16931-R046", Severity: rules.Error, Description: "Item net price MUST equal (Gross price - Allowance amount) when gross price is provided."},
	{ID: "PEPPOL-EN16931-R130", Severity: rules.Error, Description: "Unit code of price base quantity MUST be same as invoiced quantity."},

	// Identifiers by ICD scheme (PEPPOL-COMMON-R*).
	{ID: "PEPPOL-COMMON-R040", Severity: rules.Error, Description: "GLN13 MUST have a valid format according to GS1 rules."},
	{ID: "PEPPOL-COMMON-R041", Severity: rules.Error, Description: "Norwegian organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R042", Severity: rules.Error, Description: "Danish organization number (CVR) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R043", Severity: rules.Error, Description: "Belgian enterprise number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R062", Severity: rules.Warning, Description: "Belgian VAT number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R044", Severity: rules.Warning, Description: "IPA Code (Codice Univoco Unità Organizzativa) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R045", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R046", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R047", Severity: rules.Warning, Description: "Italian VAT Code (Partita Iva) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R049", Severity: rules.Error, Description: "Swedish organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R050", Severity: rules.Error, Description: "Australian Business Number (ABN) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R052", Severity: rules.Error, Description: "Danish chamber of commerce number (P) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R053", Severity: rules.Error, Description: "Danish ERSTORG number (SE) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R054", Severity: rules.Error, Description: "Dutch Chamber of Commerce (KVK) numbers (0106) MUST be stated in the correct format (12345678)."},
	{ID: "PEPPOL-COMMON-R055", Severity: rules.Error, Description: "Dutch organization identification numbers (0190) MUST be stated in the correct format (12345678901234567890)."},
	{ID: "PEPPOL-COMMON-R056-1", Severity: rules.Error, Description: "Dutch VAT numbers (9944) MUST be stated in the correct format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R056-2", Severity: rules.Error, Description: "Dutch VAT numbers MUST have the format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R057", Severity: rules.Error, Description: "Dutch Chamber of Commerce Establishment numbers (0217) MUST be stated in the correct format (123456789012)."},
	{ID: "PEPPOL-COMMON-R058", Severity: rules.Warning, Description: "Luxembourg VAT number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R059", Severity: rules.Warning, Description: "Luxembourg Register of Legal Persons number (Matricule) MUST be stated in the correct format."},

	// Restricted code lists and formatting.
	{ID: "PEPPOL-EN16931-CL001", Severity: rules.Error, Description: "Mime code MUST be according to subset of IANA code list."},
	{ID: "PEPPOL-EN16931-CL002", Severity: rules.Error, Description: "Reason code MUST be according to subset of UNCL 5189 D.16B."},
	{ID: "PEPPOL-EN16931-CL003", Severity: rules.Error, Description: "Reason code MUST be according to UNCL 7161 D.16B."},
	{ID: "PEPPOL-EN16931-CL006", Severity: rules.Error, Description: "Invoice period description code MUST be according to UNCL 2005 D.16B."},
	{ID: "PEPPOL-EN16931-CL007", Severity: rules.Error, Description: "Currency code must be according to ISO 4217:2005"},
	{ID: "PEPPOL-EN16931-P0100", Severity: rules.Error, Description: "Invoice type code MUST be set according to the profile."},
	{ID: "PEPPOL-EN16931-P0101", Severity: rules.Error, Description: "Credit note type code MUST be set according to the profile."},
	{ID: "PEPPOL-EN16931-F001", Severity: rules.Error, Description: "A date MUST be formatted YYYY-MM-DD."},
	{ID: "PEPPOL-EN16931-CL008", Severity: rules.Error, Description: "Electronic address identifier scheme must be from the codelist \"Electronic Address Identifier Scheme\""},
	{ID: "PEPPOL-EN16931-P0104", Severity: rules.Error, Description: "Tax Category G MUST be used when exemption reason code is VATEX-EU-G"},
	{ID: "PEPPOL-EN16931-P0105", Severity: rules.Error, Description: "Tax Category O MUST be used when exemption reason code is VATEX-EU-O"},
	{ID: "PEPPOL-EN16931-P0106", Severity: rules.Error, Description: "Tax Category K MUST be used when exemption reason code is VATEX-EU-IC"},
	{ID: "PEPPOL-EN16931-P0107", Severity: rules.Error, Description: "Tax Category AE MUST be used when exemption reason code is VATEX-EU-AE"},
	{ID: "PEPPOL-EN16931-P0108", Severity: rules.Error, Description: "Tax Category E MUST be used when exemption reason code is VATEX-EU-D"},
	{ID: "PEPPOL-EN16931-P0109", Severity: rules.Error, Description: "Tax Category E MUST be used when exemption reason code is VATEX-EU-F"},
	{ID: "PEPPOL-EN16931-P0110", Severity: rules.Error, Description: "Tax Category E MUST be used when exemption reason code is VATEX-EU-I"},
	{ID: "PEPPOL-EN16931-P0111", Severity: rules.Error, Description: "Tax Category E MUST be used when exemption reason code is VATEX-EU-J"},

	// Inferred mandatory cardinality: an element, when present, contains
	// exactly one of each of its mandatory children.
	{ID: "PEPPOL-EN16931-R205", Severity: rules.Error, Description: "ubl-creditnote:CreditNote MUST contain exactly one cbc:CreditNoteTypeCode."},
	{ID: "PEPPOL-EN16931-R206", Severity: rules.Error, Description: "ubl-creditnote:CreditNote MUST contain exactly one cbc:CustomizationID."},
	{ID: "PEPPOL-EN16931-R207", Severity: rules.Error, Description: "ubl-creditnote:CreditNote MUST contain exactly one cbc:DocumentCurrencyCode."},
	{ID: "PEPPOL-EN16931-R208", Severity: rules.Error, Description: "ubl-creditnote:CreditNote MUST contain exactly one cbc:ProfileID."},
	{ID: "PEPPOL-EN16931-R141", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty MUST contain exactly one cac:Party."},
	{ID: "PEPPOL-EN16931-R142", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cac:PartyLegalEntity."},
	{ID: "PEPPOL-EN16931-R146", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R150", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cbc:EndpointID."},
	{ID: "PEPPOL-EN16931-R143", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity MUST contain exactly one cbc:RegistrationName."},
	{ID: "PEPPOL-EN16931-R145", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R144", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R148", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R149", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R151", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty MUST contain exactly one cac:Party."},
	{ID: "PEPPOL-EN16931-R152", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cac:PartyLegalEntity."},
	{ID: "PEPPOL-EN16931-R156", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R160", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cbc:EndpointID."},
	{ID: "PEPPOL-EN16931-R153", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity MUST contain exactly one cbc:RegistrationName."},
	{ID: "PEPPOL-EN16931-R155", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R154", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R158", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R159", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R161", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference MUST contain exactly one cbc:URI."},
	{ID: "PEPPOL-EN16931-R162", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AllowanceCharge MUST contain exactly one cac:TaxCategory."},
	{ID: "PEPPOL-EN16931-R165", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AllowanceCharge/cac:TaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R164", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R166", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:BillingReference MUST contain exactly one cac:InvoiceDocumentReference."},
	{ID: "PEPPOL-EN16931-R168", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine MUST contain exactly one cac:Item."},
	{ID: "PEPPOL-EN16931-R177", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine MUST contain exactly one cac:Price."},
	{ID: "PEPPOL-EN16931-R178", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine MUST contain exactly one cbc:CreditedQuantity."},
	{ID: "PEPPOL-EN16931-R179", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine MUST contain exactly one cbc:LineExtensionAmount."},
	{ID: "PEPPOL-EN16931-R167", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:DocumentReference MUST contain exactly one cbc:DocumentTypeCode."},
	{ID: "PEPPOL-EN16931-R170", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item MUST contain exactly one cac:ClassifiedTaxCategory."},
	{ID: "PEPPOL-EN16931-R176", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item MUST contain exactly one cbc:Name."},
	{ID: "PEPPOL-EN16931-R169", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty MUST contain exactly one cbc:Value."},
	{ID: "PEPPOL-EN16931-R173", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R172", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R174", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item/cac:CommodityClassification MUST contain exactly one cbc:ItemClassificationCode."},
	{ID: "PEPPOL-EN16931-R175", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:CreditNoteLine/cac:Item/cac:OriginCountry MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R181", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R182", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R183", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:Delivery/cac:DeliveryParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R184", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:LegalMonetaryTotal MUST contain exactly one cbc:LineExtensionAmount."},
	{ID: "PEPPOL-EN16931-R185", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:LegalMonetaryTotal MUST contain exactly one cbc:TaxExclusiveAmount."},
	{ID: "PEPPOL-EN16931-R186", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:LegalMonetaryTotal MUST contain exactly one cbc:TaxInclusiveAmount."},
	{ID: "PEPPOL-EN16931-R188", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PayeeParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R187", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PayeeParty/cac:PartyLegalEntity MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R190", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PaymentMeans/cac:PayeeFinancialAccount MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R189", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R191", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R192", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:PaymentTerms MUST contain exactly one cbc:Note."},
	{ID: "PEPPOL-EN16931-R193", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R194", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty MUST contain exactly one cac:PartyTaxScheme."},
	{ID: "PEPPOL-EN16931-R197", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R196", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R195", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R199", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R200", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R204", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxTotal/cac:TaxSubtotal MUST contain exactly one cbc:TaxableAmount."},
	{ID: "PEPPOL-EN16931-R203", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R202", Severity: rules.Error, Description: "ubl-creditnote:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R273", Severity: rules.Error, Description: "ubl-invoice:Invoice MUST contain exactly one cbc:CustomizationID."},
	{ID: "PEPPOL-EN16931-R274", Severity: rules.Error, Description: "ubl-invoice:Invoice MUST contain exactly one cbc:DocumentCurrencyCode."},
	{ID: "PEPPOL-EN16931-R275", Severity: rules.Error, Description: "ubl-invoice:Invoice MUST contain exactly one cbc:InvoiceTypeCode."},
	{ID: "PEPPOL-EN16931-R276", Severity: rules.Error, Description: "ubl-invoice:Invoice MUST contain exactly one cbc:ProfileID."},
	{ID: "PEPPOL-EN16931-R210", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty MUST contain exactly one cac:Party."},
	{ID: "PEPPOL-EN16931-R211", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cac:PartyLegalEntity."},
	{ID: "PEPPOL-EN16931-R215", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R219", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party MUST contain exactly one cbc:EndpointID."},
	{ID: "PEPPOL-EN16931-R212", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity MUST contain exactly one cbc:RegistrationName."},
	{ID: "PEPPOL-EN16931-R214", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R213", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R217", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R218", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R220", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty MUST contain exactly one cac:Party."},
	{ID: "PEPPOL-EN16931-R221", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cac:PartyLegalEntity."},
	{ID: "PEPPOL-EN16931-R225", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R229", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party MUST contain exactly one cbc:EndpointID."},
	{ID: "PEPPOL-EN16931-R222", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity MUST contain exactly one cbc:RegistrationName."},
	{ID: "PEPPOL-EN16931-R224", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R223", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R227", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R228", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R230", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference MUST contain exactly one cbc:URI."},
	{ID: "PEPPOL-EN16931-R231", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AllowanceCharge MUST contain exactly one cac:TaxCategory."},
	{ID: "PEPPOL-EN16931-R234", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AllowanceCharge/cac:TaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R233", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R235", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:BillingReference MUST contain exactly one cac:InvoiceDocumentReference."},
	{ID: "PEPPOL-EN16931-R237", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R238", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R239", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:Delivery/cac:DeliveryParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R249", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine MUST contain exactly one cac:Price."},
	{ID: "PEPPOL-EN16931-R250", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine MUST contain exactly one cbc:InvoicedQuantity."},
	{ID: "PEPPOL-EN16931-R240", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:DocumentReference MUST contain exactly one cbc:DocumentTypeCode."},
	{ID: "PEPPOL-EN16931-R242", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item MUST contain exactly one cac:ClassifiedTaxCategory."},
	{ID: "PEPPOL-EN16931-R248", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item MUST contain exactly one cbc:Name."},
	{ID: "PEPPOL-EN16931-R241", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty MUST contain exactly one cbc:Value."},
	{ID: "PEPPOL-EN16931-R245", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R244", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R246", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item/cac:CommodityClassification MUST contain exactly one cbc:ItemClassificationCode."},
	{ID: "PEPPOL-EN16931-R247", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:InvoiceLine/cac:Item/cac:OriginCountry MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R252", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:LegalMonetaryTotal MUST contain exactly one cbc:LineExtensionAmount."},
	{ID: "PEPPOL-EN16931-R253", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:LegalMonetaryTotal MUST contain exactly one cbc:TaxExclusiveAmount."},
	{ID: "PEPPOL-EN16931-R254", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:LegalMonetaryTotal MUST contain exactly one cbc:TaxInclusiveAmount."},
	{ID: "PEPPOL-EN16931-R256", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PayeeParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R255", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PayeeParty/cac:PartyLegalEntity MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R258", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PaymentMeans/cac:PayeeFinancialAccount MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R257", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R259", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R260", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:PaymentTerms MUST contain exactly one cbc:Note."},
	{ID: "PEPPOL-EN16931-R261", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty MUST contain exactly one cac:PartyName."},
	{ID: "PEPPOL-EN16931-R262", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty MUST contain exactly one cac:PartyTaxScheme."},
	{ID: "PEPPOL-EN16931-R265", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty MUST contain exactly one cac:PostalAddress."},
	{ID: "PEPPOL-EN16931-R264", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty/cac:PartyTaxScheme MUST contain exactly one cbc:CompanyID."},
	{ID: "PEPPOL-EN16931-R263", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty/cac:PartyTaxScheme/cac:TaxScheme MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R267", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress MUST contain exactly one cac:Country."},
	{ID: "PEPPOL-EN16931-R268", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country MUST contain exactly one cbc:IdentificationCode."},
	{ID: "PEPPOL-EN16931-R272", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxTotal/cac:TaxSubtotal MUST contain exactly one cbc:TaxableAmount."},
	{ID: "PEPPOL-EN16931-R271", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory MUST contain exactly one cbc:ID."},
	{ID: "PEPPOL-EN16931-R270", Severity: rules.Error, Description: "ubl-invoice:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme MUST contain exactly one cbc:ID."},
}

// sbByID indexes SelfBillingCatalog.
var sbByID = sync.OnceValue(func() map[string]*rules.RuleInfo {
	m := make(map[string]*rules.RuleInfo, len(SelfBillingCatalog))
	for i := range SelfBillingCatalog {
		m[SelfBillingCatalog[i].ID] = &SelfBillingCatalog[i]
	}
	return m
})

// selfBilling reports whether the rules of Peppol BIS Self-Billing apply to
// doc: its profile, read from BT-24 or set by the caller, is
// invoice.ProfilePeppolSelfBilling.
func selfBilling(doc *rules.Doc) bool { return doc.Profile == invoice.ProfilePeppolSelfBilling }
