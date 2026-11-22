package de

import "github.com/ritoon/zerechnung/internal/rules"

// Catalog lists the 74 assertions of the XRechnung CII schematron, in the
// order of the schematron, with the severity of their flag: the 22 of the
// Peppol patterns that it includes first, then its own 52. Check applies
// them to a CII invoice; UBLCatalog lists the assertions it applies to a UBL
// invoice. The Peppol rules shared with the UBL schematron have a
// description of both syntaxes.
var Catalog = []rules.RuleInfo{
	// peppol-cii-pattern-1: Peppol rules adapted by KoSIT.
	{ID: "PEPPOL-EN16931-R001", Severity: rules.Error, Description: "The Business process type (BT-23) must be provided."},
	{ID: "PEPPOL-EN16931-R005", Severity: rules.Error, Description: "The VAT accounting currency code (BT-6) must differ from the Invoice currency code (BT-5)."},
	{ID: "PEPPOL-EN16931-R053", Severity: rules.Error, Description: "At most one Invoice total VAT amount (BT-110) may be given in the Invoice currency; in UBL, exactly one tax total with VAT breakdown."},
	{ID: "PEPPOL-EN16931-R054", Severity: rules.Error, Description: "The Invoice total VAT amount in accounting currency (BT-111) must be given once if and only if the VAT accounting currency code (BT-6) is."},
	{ID: "PEPPOL-EN16931-R055", Severity: rules.Error, Description: "The Invoice total VAT amount (BT-110) and the Invoice total VAT amount in accounting currency (BT-111) must have the same sign."},
	{ID: "PEPPOL-EN16931-R010", Severity: rules.Error, Description: "The Buyer electronic address (BT-49) must be provided."},
	{ID: "PEPPOL-EN16931-R020", Severity: rules.Error, Description: "The Seller electronic address (BT-34) must be provided."},
	{ID: "PEPPOL-EN16931-R041", Severity: rules.Error, Description: "An allowance or charge with a percentage must have a base amount."},
	{ID: "PEPPOL-EN16931-R042", Severity: rules.Error, Description: "An allowance or charge with a base amount must have a percentage."},
	{ID: "PEPPOL-EN16931-R040", Severity: rules.Error, Description: "The amount of an allowance or charge with a base amount and a percentage must be base amount x percentage / 100, give or take 0.02 (0.5 in HUF)."},
	{ID: "PEPPOL-EN16931-R043-1", Severity: rules.Error, Description: "The indicator of a document or line allowance or charge must be 'true' or 'false'."},
	{ID: "PEPPOL-EN16931-R043-2", Severity: rules.Error, Description: "The indicator of an item price discount must be 'true' or 'false'."},
	{ID: "PEPPOL-EN16931-R061", Severity: rules.Error, Description: "A direct debit (payment means code 49 or 59) must have a Mandate reference identifier (BT-89)."},
	{ID: "PEPPOL-EN16931-R110", Severity: rules.Error, Description: "The Invoice line period start date (BT-134) must not be before the Invoicing period start date (BT-73)."},
	{ID: "PEPPOL-EN16931-R111", Severity: rules.Error, Description: "The Invoice line period end date (BT-135) must not be after the Invoicing period end date (BT-74)."},
	{ID: "PEPPOL-EN16931-R120", Severity: rules.Warning, Description: "The Invoice line net amount (BT-131) should be quantity x (net price / base quantity) + line charges - line allowances, give or take 0.02 (0.5 in HUF)."},
	{ID: "PEPPOL-EN16931-R101", Severity: rules.Error, Description: "An Invoice line object identifier (BT-128) must have the type code 130."},
	{ID: "PEPPOL-EN16931-R121", Severity: rules.Error, Description: "The Item price base quantity (BT-149) must be above zero."},
	{ID: "PEPPOL-EN16931-R130", Severity: rules.Error, Description: "The unit of the Item price base quantity (BT-150) must be the unit of the Invoiced quantity (BT-130)."},

	// peppol-cii-pattern-2: empty elements.
	{ID: "PEPPOL-EN16931-R008", Severity: rules.Error, Description: "The document must not contain empty elements."},

	// peppol-cii-pattern-3: gross price.
	{ID: "PEPPOL-EN16931-R044", Severity: rules.Error, Description: "An Item price discount (BT-147) must not be a charge."},
	{ID: "PEPPOL-EN16931-R046", Severity: rules.Error, Description: "The Item net price (BT-146) must be the Item gross price (BT-148) minus the Item price discount (BT-147)."},

	// cii-pattern: XRechnung CIUS.
	{ID: "BR-DE-30", Severity: rules.Error, Description: "If DIRECT DEBIT (BG-19) is present, the Bank assigned creditor identifier (BT-90) must be transmitted."},
	{ID: "BR-DE-31", Severity: rules.Error, Description: "If DIRECT DEBIT (BG-19) is present, the Debited account identifier (BT-91) must be transmitted."},
	{ID: "BR-DE-1", Severity: rules.Error, Description: "An invoice must contain information on PAYMENT INSTRUCTIONS (BG-16)."},
	{ID: "BR-DE-15", Severity: rules.Error, Description: "The element Buyer reference (BT-10) must be transmitted."},
	{ID: "BR-DE-16", Severity: rules.Error, Description: "If the VAT category codes S, Z, E, AE, K, G, L or M are used, the Seller VAT identifier (BT-31), the Seller tax registration identifier (BT-32) or SELLER TAX REPRESENTATIVE PARTY (BG-11) must be transmitted."},
	{ID: "BR-DE-17", Severity: rules.Warning, Description: "The Invoice type code (BT-3) should be 326, 380, 384, 389, 381, 875, 876 or 877."},
	{ID: "BR-DE-18", Severity: rules.Error, Description: "Cash discount lines in Payment terms (BT-20) must match #SKONTO#TAGE=n#PROZENT=n.nn[#BASISBETRAG=n.nn]# and end with a line break."},
	{ID: "BR-DE-22", Severity: rules.Error, Description: "The filename attributes of the attached documents (BT-125) must be unique."},
	{ID: "BR-DE-26", Severity: rules.Warning, Description: "If the Invoice type code (BT-3) is 384 (Corrected invoice), PRECEDING INVOICE REFERENCE (BG-3) should be present."},
	{ID: "BR-DE-21", Severity: rules.Warning, Description: "The Specification identifier (BT-24) should match the identifier of the XRechnung standard."},
	{ID: "BR-DE-2", Severity: rules.Error, Description: "The group SELLER CONTACT (BG-6) must be transmitted."},
	{ID: "BR-DE-3", Severity: rules.Error, Description: "The element Seller city (BT-37) must be transmitted."},
	{ID: "BR-DE-4", Severity: rules.Error, Description: "The element Seller post code (BT-38) must be transmitted."},
	{ID: "BR-DE-5", Severity: rules.Error, Description: "The element Seller contact point (BT-41) must be transmitted."},
	{ID: "BR-DE-6", Severity: rules.Error, Description: "The element Seller contact telephone number (BT-42) must be transmitted."},
	{ID: "BR-DE-7", Severity: rules.Error, Description: "The element Seller contact email address (BT-43) must be transmitted."},
	{ID: "BR-DE-27", Severity: rules.Warning, Description: "The Seller contact telephone number (BT-42) should contain at least three digits."},
	{ID: "BR-DE-28", Severity: rules.Warning, Description: "The Seller contact email address (BT-43) should contain exactly one @ sign, a local part and a domain of at least two labels, without white space."},
	{ID: "BR-DE-8", Severity: rules.Error, Description: "The element Buyer city (BT-52) must be transmitted."},
	{ID: "BR-DE-9", Severity: rules.Error, Description: "The element Buyer post code (BT-53) must be transmitted."},
	{ID: "BR-TMP-2", Severity: rules.Error, Description: "The External document location (BT-124) must contain an absolute URL with a valid scheme."},
	{ID: "BR-TMP-4", Severity: rules.Warning, Description: "The Supporting document description (BT-123) may appear at most once in BG-24."},
	{ID: "BR-TMP-5", Severity: rules.Warning, Description: "The Attached document (BT-125) may appear at most once in BG-24."},
	{ID: "BR-TMP-7", Severity: rules.Warning, Description: "Dates in CII must have the attribute format=\"102\" and the form YYYYMMDD."},
	{ID: "BR-DE-10", Severity: rules.Error, Description: "The element Deliver to city (BT-77) must be transmitted if DELIVER TO ADDRESS (BG-15) is transmitted."},
	{ID: "BR-DE-11", Severity: rules.Error, Description: "The element Deliver to post code (BT-78) must be transmitted if DELIVER TO ADDRESS (BG-15) is transmitted."},
	{ID: "BR-DE-19", Severity: rules.Warning, Description: "The Payment account identifier (BT-84) should be a valid IBAN when the Payment means type code (BT-81) is 58."},
	{ID: "BR-DE-23-a", Severity: rules.Error, Description: "With a credit transfer code (30, 58) in BT-81, CREDIT TRANSFER (BG-17) must be transmitted."},
	{ID: "BR-DE-23-b", Severity: rules.Error, Description: "With a credit transfer code (30, 58) in BT-81, BG-18 and BG-19 must not be transmitted."},
	{ID: "BR-DE-24-a", Severity: rules.Error, Description: "With a payment card code (48, 54, 55) in BT-81, PAYMENT CARD INFORMATION (BG-18) must be transmitted."},
	{ID: "BR-DE-24-b", Severity: rules.Error, Description: "With a payment card code (48, 54, 55) in BT-81, BG-17 and BG-19 must not be transmitted."},
	{ID: "BR-DE-20", Severity: rules.Warning, Description: "The Debited account identifier (BT-91) should be a valid IBAN when the Payment means type code (BT-81) is 59."},
	{ID: "BR-DE-25-a", Severity: rules.Error, Description: "With a direct debit code (59) in BT-81, DIRECT DEBIT (BG-19) must be transmitted."},
	{ID: "BR-DE-25-b", Severity: rules.Error, Description: "With a direct debit code (59) in BT-81, BG-17 and BG-18 must not be transmitted."},
	{ID: "BR-DE-14", Severity: rules.Error, Description: "The element VAT category rate (BT-119) must be transmitted."},
	{ID: "BR-DE-TMP-32", Severity: rules.Info, Description: "An invoice should give BT-72, BG-14 or BG-26 on every line as delivery or service date."},
	{ID: "BR-TMP-3", Severity: rules.Error, Description: "Base quantities (BT-149) and their unit codes (BT-150) of the gross and net prices must be identical when both are given."},

	// cii-extension-pattern: XRechnung extension.
	{ID: "BR-DEX-15", Severity: rules.Warning, Description: "Sub invoice lines (ram:ParentLineID) are not supported by XRechnung."},
	{ID: "BR-DEX-04", Severity: rules.Error, Description: "Scheme identifiers of party identifiers (global identifiers in CII) must belong to the ISO 6523 ICD list extended with XR01, XR02, XR03; in UBL, SEPA is also allowed for the seller and the payee."},
	{ID: "BR-DEX-05", Severity: rules.Error, Description: "Scheme identifiers of identifiers (all identifiers but tax registrations in CII, legal registration identifiers in UBL) must belong to the ISO 6523 ICD list extended with XR01, XR02, XR03."},
	{ID: "BR-DEX-06", Severity: rules.Error, Description: "Scheme identifiers of Item standard identifiers (BT-157) must belong to the ISO 6523 ICD list extended with XR01, XR02, XR03."},
	{ID: "BR-DEX-07", Severity: rules.Error, Description: "Scheme identifiers of electronic addresses must belong to the CEF EAS list extended with XR01, XR02, XR03."},
	{ID: "BR-DEX-08", Severity: rules.Error, Description: "Scheme identifiers of Deliver to location identifiers (BT-71) must belong to the ISO 6523 ICD list extended with XR01, XR02, XR03."},
	{ID: "BR-DEX-01", Severity: rules.Error, Description: "The MIME code of an Attached document (BT-125) must be allowed; application/xml is allowed with the extension."},

	// cii-cvd-pattern: commercial vehicle data.
	{ID: "BR-DE-CVD-03", Severity: rules.Error, Description: "At least one invoice line must have an Item classification identifier (BT-158) of scheme 'CVD' and an Item attribute name (BT-160) 'cva'."},
	{ID: "BR-DE-CVD-06-b", Severity: rules.Error, Description: "A line with the Item attribute name 'cva' must have exactly one Item classification identifier of scheme 'CVD'."},
	{ID: "BR-DE-CVD-06-a", Severity: rules.Error, Description: "A line with an Item classification identifier of scheme 'CVD' must have exactly one Item attribute name 'cva'."},
	{ID: "BR-TMP-CVD-01", Severity: rules.Error, Description: "The scheme identifier of the Item classification identifier (BT-158) must come from UNTDID 7143."},
	{ID: "BR-DE-CVD-04", Severity: rules.Error, Description: "An Item classification identifier of scheme 'CVD' must be a vehicle category M1, M2, M3, N1, N2 or N3."},
	{ID: "BR-DE-CVD-05", Severity: rules.Error, Description: "The Item attribute value of the attribute 'cva' must be clean, zero-emission or other."},
	{ID: "BR-DE-CVD-01", Severity: rules.Error, Description: "The element Contract reference (BT-12) must be transmitted."},
	{ID: "BR-DE-CVD-02", Severity: rules.Error, Description: "The element Tender or lot reference (BT-17) must be transmitted."},
}
