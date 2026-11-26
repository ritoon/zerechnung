package peppol

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"sync"
)

// Catalog lists the 94 rules of PEPPOL-EN16931-CII.sch, release 3.1.0, in
// the order of the schematron, with the severity of their flag: 95
// assertions, PEPPOL-EN16931-R043 having two with the same message. The
// descriptions are the messages of the schematron.
var Catalog = []rules.RuleInfo{
	// Peppol rules (PEPPOL-EN16931-R*).
	{ID: "PEPPOL-EN16931-R001", Severity: rules.Error, Description: "Business process MUST be provided."},
	{ID: "PEPPOL-EN16931-R007", Severity: rules.Error, Description: "Business process MUST have an approved profile identifier."},
	{ID: "PEPPOL-EN16931-R004", Severity: rules.Error, Description: "Specification identifier MUST begin with the value 'urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0' and follow the format rules for the identifier."},
	{ID: "PEPPOL-EN16931-R003", Severity: rules.Error, Description: "A buyer reference or purchase order reference MUST be provided."},
	{ID: "PEPPOL-EN16931-R006", Severity: rules.Error, Description: "Only one invoiced object is allowed on document level"},
	{ID: "PEPPOL-EN16931-R080", Severity: rules.Error, Description: "Only one project reference is allowed on document level"},
	{ID: "PEPPOL-EN16931-R005", Severity: rules.Error, Description: "VAT accounting currency code MUST be different from invoice currency code when provided."},
	{ID: "PEPPOL-EN16931-R053", Severity: rules.Error, Description: "Only one tax total with tax subtotals MUST be provided."},
	{ID: "PEPPOL-EN16931-R054", Severity: rules.Error, Description: "Only one tax total without tax subtotals MUST be provided when tax currency code is provided."},
	{ID: "PEPPOL-EN16931-R055", Severity: rules.Error, Description: "Invoice total VAT amount and Invoice total VAT amount in accounting currency MUST have the same operational sign"},
	{ID: "PEPPOL-EN16931-R002", Severity: rules.Error, Description: "No more than one note is allowed on document level."},
	{ID: "PEPPOL-EN16931-R010", Severity: rules.Error, Description: "Buyer electronic address MUST be provided"},
	{ID: "PEPPOL-EN16931-R020", Severity: rules.Error, Description: "Seller electronic address MUST be provided"},
	{ID: "PEPPOL-EN16931-R041", Severity: rules.Error, Description: "Allowance/charge base amount MUST be provided when allowance/charge percentage is provided."},
	{ID: "PEPPOL-EN16931-R042", Severity: rules.Error, Description: "Allowance/charge percentage MUST be provided when allowance/charge base amount is provided."},
	{ID: "PEPPOL-EN16931-R040", Severity: rules.Error, Description: "Allowance/charge amount must equal base amount * percentage/100 if base amount and percentage exists"},
	{ID: "PEPPOL-EN16931-R043", Severity: rules.Error, Description: "Allowance/charge ChargeIndicator value MUST equal 'true' or 'false'"},
	{ID: "PEPPOL-EN16931-R061", Severity: rules.Error, Description: "Mandate reference MUST be provided for direct debit."},
	{ID: "PEPPOL-EN16931-R110", Severity: rules.Error, Description: "Start date of line period MUST be within invoice period."},
	{ID: "PEPPOL-EN16931-R111", Severity: rules.Error, Description: "End date of line period MUST be within invoice period."},
	{ID: "PEPPOL-EN16931-R120", Severity: rules.Error, Description: "Invoice line net amount MUST equal (Invoiced quantity * (Item net price/item price base quantity) + Sum of invoice line charge amount - sum of invoice line allowance amount"},
	{ID: "PEPPOL-EN16931-R100", Severity: rules.Error, Description: "Only one invoiced object is allowed pr line"},
	{ID: "PEPPOL-EN16931-R101", Severity: rules.Error, Description: "Element Document reference can only be used for Invoice line object"},
	{ID: "PEPPOL-EN16931-R121", Severity: rules.Error, Description: "Base quantity MUST be a positive number above zero."},
	{ID: "PEPPOL-EN16931-R130", Severity: rules.Error, Description: "Unit code of price base quantity MUST be same as invoiced quantity."},

	// Identifiers by ICD scheme (PEPPOL-COMMON-R*).
	{ID: "PEPPOL-COMMON-R040", Severity: rules.Error, Description: "GLN13 MUST have a valid format according to GS1 rules."},
	{ID: "PEPPOL-COMMON-R041", Severity: rules.Error, Description: "Norwegian organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R042", Severity: rules.Error, Description: "Danish organization number (CVR) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R052", Severity: rules.Error, Description: "Danish chamber of commerce number (P) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R053", Severity: rules.Error, Description: "Danish ERSTORG number (SE) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R043", Severity: rules.Error, Description: "Belgian enterprise number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R062", Severity: rules.Warning, Description: "Belgian VAT number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R044", Severity: rules.Warning, Description: "IPA Code (Codice Univoco Unità Organizzativa) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R045", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R046", Severity: rules.Warning, Description: "Tax Code (Codice Fiscale) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R047", Severity: rules.Warning, Description: "Italian VAT Code (Partita Iva) SHOULD be stated in the correct format"},
	{ID: "PEPPOL-COMMON-R049", Severity: rules.Error, Description: "Swedish organization number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R050", Severity: rules.Error, Description: "Australian Business Number (ABN) MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R054", Severity: rules.Error, Description: "Dutch Chamber of Commerce (KVK) numbers (0106) MUST be stated in the correct format (12345678)."},
	{ID: "PEPPOL-COMMON-R055", Severity: rules.Error, Description: "Dutch organization identification numbers (0190) MUST be stated in the correct format (12345678901234567890)."},
	{ID: "PEPPOL-COMMON-R056-1", Severity: rules.Error, Description: "Dutch VAT numbers (9944) MUST be stated in the correct format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R056-2", Severity: rules.Error, Description: "Dutch VAT numbers MUST have the format (NL123456789B12)."},
	{ID: "PEPPOL-COMMON-R057", Severity: rules.Error, Description: "Dutch Chamber of Commerce Establishment numbers (0217) MUST be stated in the correct format (123456789012)."},
	{ID: "PEPPOL-COMMON-R058", Severity: rules.Warning, Description: "Luxembourg VAT number MUST be stated in the correct format."},
	{ID: "PEPPOL-COMMON-R059", Severity: rules.Warning, Description: "Luxembourg Register of Legal Persons number (Matricule) MUST be stated in the correct format."},

	// Norway.
	{ID: "NO-R-002", Severity: rules.Warning, Description: "Most invoice issuers are required to append \"Foretaksregisteret\" to their invoice. \"Dersom selger er aksjeselskap, allmennaksjeselskap eller filial av utenlandsk selskap skal også ordet «Foretaksregisteret» fremgå av salgsdokumentet, jf. foretaksregisterloven § 10-2.\""},
	{ID: "NO-R-001", Severity: rules.Error, Description: "For Norwegian suppliers, a VAT number MUST be the country code prefix NO followed by a valid Norwegian organization number (nine numbers) followed by the letters MVA."},

	// Denmark.
	{ID: "DK-R-002", Severity: rules.Error, Description: "Danish suppliers MUST provide legal entity."},
	{ID: "DK-R-004", Severity: rules.Error, Description: "When specifying non-VAT Taxes for Danish customers, Danish suppliers MUST use the AllowanceChargeReasonCode=\"ZZZ\" and MUST be specified in AllowanceChargeReason; Either as the 4-digit Tax category or must include a #, but the # is not allowed as first and last character"},
	{ID: "DK-R-013", Severity: rules.Error, Description: "For Danish Suppliers it is mandatory to use schemeID when GlobalID is used for SellerTradeParty or BuyerTradeParty"},
	{ID: "DK-R-014", Severity: rules.Error, Description: "For Danish Suppliers it is mandatory to specify schemeID as \"0184\" when SpecifiedLegalOrganization is used for SellerTradeParty"},
	{ID: "DK-R-017", Severity: rules.Error, Description: "For Danish Customers it is mandatory to specify schemeID as \"0184\" (DK CVR-number) when SpecifiedLegalOrganization is used for BuyerTradeParty"},
	{ID: "DK-R-016", Severity: rules.Error, Description: "For Danish Suppliers, a Credit note cannot have a negative total (DuePayableAmount)"},
	{ID: "DK-R-005", Severity: rules.Error, Description: "For Danish suppliers the following Payment means type codes are allowed: 1, 10, 31, 42, 48, 49, 50, 58, 59, 93 and 97"},
	{ID: "DK-R-006", Severity: rules.Error, Description: "For Danish suppliers, bank account and registration account are mandatory if payment means is 31 or 42"},
	{ID: "DK-R-007", Severity: rules.Error, Description: "For Danish suppliers DirectDebitMandateID and CreditorReferenceID are mandatory when payment means is 49"},
	{ID: "DK-R-008", Severity: rules.Error, Description: "For Danish Suppliers PaymentReference is mandatory and MUST start with 01#, 04# or 15# (kortartkode), and PayeePartyCreditorFinancialAccount/IBANID (Giro kontonummer) is mandatory and must be 7 characters long, when payment means equals 50 (Giro)"},
	{ID: "DK-R-009", Severity: rules.Error, Description: "For Danish Suppliers if the PaymentReference is prefixed with 04# or 015# the 16 digits instruction Id must be added to the PaymentReference eg. \"04#1234567890123456\" when Payment means equals 50 (Giro)"},
	{ID: "DK-R-010", Severity: rules.Error, Description: "For Danish Suppliers using PaymentMeansCode 93, PaymentID is mandatory. The first three characters of the PaymentID MUST be 71#, 73# or 75# (kortartskode), and PayeeFinancialAccount/ID MUST be exactly 8 characters long."},
	{ID: "DK-R-011", Severity: rules.Error, Description: "For Danish Suppliers if the PaymentReference is prefixed with 71# or 75# the 15-16 digits instruction Id must be added to the PaymentReference eg. \"71#1234567890123456\" when payment Method equals 93 (FIK)"},
	{ID: "DK-R-003", Severity: rules.Error, Description: "If ItemClassification is provided from Danish suppliers, UNSPSC version 19.05.01 or 26.08.01 MUST be used"},

	// Italy.
	{ID: "IT-R-001", Severity: rules.Error, Description: "BT-32 (Seller tax registration identifier) - For Italian suppliers BT-32 minimum length 11 and maximum length shall be 16. Per i fornitori italiani il BT-32 deve avere una lunghezza tra 11 e 16 caratteri"},
	{ID: "IT-R-002", Severity: rules.Error, Description: "BT-35 (Seller address line 1) - Italian suppliers MUST provide the postal address line 1 - I fornitori italiani devono indicare l'indirizzo postale."},
	{ID: "IT-R-003", Severity: rules.Error, Description: "BT-37 (Seller city) - Italian suppliers MUST provide the postal address city - I fornitori italiani devono indicare la città di residenza."},
	{ID: "IT-R-004", Severity: rules.Error, Description: "BT-38 (Seller post code) - Italian suppliers MUST provide the postal address post code - I fornitori italiani devono indicare il CAP di residenza."},

	// Sweden.
	{ID: "SE-R-001", Severity: rules.Error, Description: "For Swedish suppliers, Swedish VAT-numbers must consist of 14 characters."},
	{ID: "SE-R-002", Severity: rules.Error, Description: "For Swedish suppliers, the Swedish VAT-numbers must have the trailing 12 characters in numeric form"},
	{ID: "SE-R-003", Severity: rules.Error, Description: "Swedish organisation numbers SHOULD be numeric."},
	{ID: "SE-R-004", Severity: rules.Error, Description: "Swedish organisation numbers consist of 10 characters."},
	{ID: "SE-R-013", Severity: rules.Error, Description: "The last digit of a Swedish organization number must be valid according to the Luhn algorithm."},
	{ID: "SE-R-005", Severity: rules.Error, Description: "For Swedish suppliers, when using Seller tax registration identifier, 'Godkänd för F-skatt' must be stated"},
	{ID: "SE-R-006", Severity: rules.Error, Description: "For Swedish suppliers invoicing Swedish buyers, only standard VAT rate of 6, 12 or 25 are used"},
	{ID: "SE-R-007", Severity: rules.Warning, Description: "For Swedish suppliers using Plusgiro, the Account ID SHOULD be numeric"},
	{ID: "SE-R-010", Severity: rules.Warning, Description: "For Swedish suppliers using Plusgiro, the Account ID SHOULD have 2-8 characters"},
	{ID: "SE-R-008", Severity: rules.Warning, Description: "For Swedish suppliers using Bankgiro, the Account ID SHOULD be numeric"},
	{ID: "SE-R-009", Severity: rules.Warning, Description: "For Swedish suppliers using Bankgiro, the Account ID SHOULD have 7-8 characters"},
	{ID: "SE-R-011", Severity: rules.Warning, Description: "For Swedish suppliers using Swedish Bankgiro or Plusgiro, the proper way to indicate this is to use Code 30 for PaymentMeans and FinancialInstitutionBranch ID with code SE:BANKGIRO or SE:PLUSGIRO"},
	{ID: "SE-R-012", Severity: rules.Warning, Description: "For domestic transactions between Swedish trading partners, credit transfer SHOULD be indicated by PaymentMeansCode=”30”"},

	// Netherlands.
	{ID: "NL-R-001", Severity: rules.Error, Description: "For suppliers in the Netherlands, if the document is a creditnote, the document MUST contain an invoice reference (ram:ApplicableHeaderTradeSettlement/ram:InvoiceReferencedDocument/ram:IssuerAssignedID)"},
	{ID: "NL-R-002", Severity: rules.Error, Description: "For suppliers in the Netherlands the supplier's address (ram:SellerTradeParty/ram:PostalTradeAddress) MUST contain street name (ram:LineOne), city (ram:CityName) and post code (ram:PostcodeCode)"},
	{ID: "NL-R-003", Severity: rules.Error, Description: "For suppliers in the Netherlands, the legal entity identifier MUST be either a KVK or OIN number (schemeID 0106 or 0190)"},
	{ID: "NL-R-004", Severity: rules.Error, Description: "For suppliers in the Netherlands, if the customer is in the Netherlands, the customer address (ram:BuyerTradeParty/ram:PostalTradeAddress) MUST contain street name (ram:LineOne), city (ram:CityName) and post code (ram:PostcodeCode)"},
	{ID: "NL-R-005", Severity: rules.Error, Description: "For suppliers in the Netherlands, if the customer is in the Netherlands, the customer's legal entity identifier MUST be either a KVK or OIN number (schemeID 0106 or 0190)"},
	{ID: "NL-R-006", Severity: rules.Error, Description: "For suppliers in the Netherlands, if the fiscal representative is in the Netherlands, the representative's address (ram:SellerTaxRepresentativeTradeParty/ram:PostalTradeAddress) MUST contain street name (ram:LineOne), city (ram:CityName) and post code (ram:PostcodeCode)"},
	{ID: "NL-R-007", Severity: rules.Error, Description: "For suppliers in the Netherlands, the supplier MUST provide a means of payment (ram:SpecifiedTradeSettlementPaymentMeans) if the payment is from customer to supplier"},
	{ID: "NL-R-008", Severity: rules.Error, Description: "For suppliers in the Netherlands, if the customer is in the Netherlands, the payment means code (ram:SpecifiedTradeSettlementPaymentMeans/ram:TypeCode) MUST be one of 30, 48, 49, 57, 58 or 59"},
	{ID: "NL-R-009", Severity: rules.Error, Description: "For suppliers in the Netherlands, if an order line reference (ram:BuyerOrderReferencedDocument/ram:LineID) is used, there must be an order reference on the document level (rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:BuyerOrderReferencedDocument/ram:IssuerAssignedID)"},

	// Restricted code lists and formatting.
	{ID: "PEPPOL-EN16931-CL001", Severity: rules.Error, Description: "Mime code MUST be according to subset of IANA code list."},
	{ID: "PEPPOL-EN16931-CL002", Severity: rules.Error, Description: "Reason code MUST be according to subset of UNCL 5189 D.16B."},
	{ID: "PEPPOL-EN16931-CL003", Severity: rules.Error, Description: "Reason code MUST be according to UNCL 7161 D.16B."},
	{ID: "PEPPOL-EN16931-CL007", Severity: rules.Error, Description: "Currency code must be according to ISO 4217:2005"},
	{ID: "PEPPOL-EN16931-P0100", Severity: rules.Error, Description: "Invoice type code MUST be set according to the profile."},
	{ID: "PEPPOL-EN16931-F001", Severity: rules.Error, Description: "A date MUST be formatted YYYYMMDD."},
	{ID: "PEPPOL-EN16931-CL008", Severity: rules.Error, Description: "Electronic address identifier scheme must be from the codelist \"Electronic Address Identifier Scheme\""},
}

// byID indexes Catalog.
var byID = sync.OnceValue(func() map[string]*rules.RuleInfo {
	m := make(map[string]*rules.RuleInfo, len(Catalog))
	for i := range Catalog {
		m[Catalog[i].ID] = &Catalog[i]
	}
	return m
})
