package peppol

import (
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// Peppol BIS Self-Billing 3.0, release 3.0.2 (May 2026, mandatory from
// 17/08/2026): the buyer issues the invoice, or the credit note, on behalf
// of the seller. The document is a UBL 2.1 invoice of Peppol BIS Billing
// 3.0, with its own specification identifier (BT-24), business process
// (BT-23) and type codes (389 and 527 for an invoice, 261 for a credit
// note); the seller and the buyer keep their roles.
//
// Its schematron, PEPPOL-EN16931-UBL-SB.sch, is the UBL schematron of
// Peppol BIS Billing 3.0 that UBLCatalog implements (release 3.0.22),
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
//     type codes 389 and 527 for an invoice and 261 for a credit note.
//
// The other rules, their tests, flags and messages are those of
// PEPPOL-EN16931-UBL.sch; the order of the identifier rules changes, which
// changes nothing, each identifier having a single scheme. A Self-Billing
// invoice gets the rules of EN 16931 of packages en16931 and ubl, as a
// Billing one: Peppol publishes the same CEN-EN16931-UBL.sch with both.
// Peppol BIS Self-Billing has no CII syntax: Check applies the rules of
// Billing to a CII invoice, whatever its profile.
//
// Release 3.1.0 (November 2026, mandatory from 22/02/2027) adds rules of
// cardinality (PEPPOL-EN16931-R141 to R276) and Luxembourgish identifiers;
// it is not implemented, as Billing 3.1.0 is not.

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

// SelfBillingCatalog lists the 62 assertions of PEPPOL-EN16931-UBL-SB.sch,
// release 3.0.2, in the order of the schematron, with the severity of their
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
	{ID: "PEPPOL-COMMON-R040", Severity: rules.Error, Description: "GLN must have a valid format according to GS1 rules."},
	{ID: "PEPPOL-COMMON-R041", Severity: rules.Error, Description: "Norwegian organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R042", Severity: rules.Error, Description: "Danish organization number (CVR) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R043", Severity: rules.Error, Description: "Belgian enterprise number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R044", Severity: rules.Warning, Description: "IPA Code (Codice Univoco Unità Organizzativa) must be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R045", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) must be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R046", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) must be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R047", Severity: rules.Warning, Description: "Italian VAT Code (Partita Iva) must be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R049", Severity: rules.Error, Description: "Swedish organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R050", Severity: rules.Error, Description: "Australian Business Number (ABN) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R052", Severity: rules.Error, Description: "Danish chamber of commerce number (P) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R053", Severity: rules.Error, Description: "Danish ERSTORG number (SE) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R054", Severity: rules.Warning, Description: "Dutch Chamber of Commerce (KVK) numbers (0106) MUST be stated in the correct format (12345678)."},
	{ID: "PEPPOL-COMMON-R055", Severity: rules.Warning, Description: "Dutch organization identification numbers (0190) MUST be stated in the correct format (12345678901234567890)."},
	{ID: "PEPPOL-COMMON-R056-1", Severity: rules.Warning, Description: "Dutch VAT numbers (9944) MUST be stated in the correct format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R056-2", Severity: rules.Warning, Description: "Dutch VAT numbers MUST have the format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R057", Severity: rules.Warning, Description: "Dutch Chamber of Commerce Establishment numbers (0217) MUST be stated in the correct format (123456789012)."},

	// Restricted code lists and formatting.
	{ID: "PEPPOL-EN16931-CL001", Severity: rules.Error, Description: "Mime code must be according to subset of IANA code list."},
	{ID: "PEPPOL-EN16931-CL002", Severity: rules.Error, Description: "Reason code MUST be according to subset of UNCL 5189 D.16B."},
	{ID: "PEPPOL-EN16931-CL003", Severity: rules.Error, Description: "Reason code MUST be according to UNCL 7161 D.16B."},
	{ID: "PEPPOL-EN16931-CL006", Severity: rules.Error, Description: "Invoice period description code must be according to UNCL 2005 D.16B."},
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
