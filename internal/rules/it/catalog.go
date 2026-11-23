package it

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"sync"
)

// Rules that are not CIUS-IT rules. The technical rules publish no
// schematron, and none of them checks the format of the Italian
// identifiers: their identifiers are specific to this package.
const (
	RuleProfile    = "IT-PROFILE-01" // BT-24 matches the countries of the seller and the buyer
	RulePIVAFormat = "IT-PIVA-01"    // format of a Partita IVA
	RulePIVAKey    = "IT-PIVA-02"    // check digit of a Partita IVA
	RuleCFFormat   = "IT-CF-01"      // format of a Codice Fiscale
	RuleCFKey      = "IT-CF-02"      // check character of a Codice Fiscale
)

// Wording shared by several rules.
const (
	amount15 = "the length of the element cannot exceed 15 characters, 2 decimal digits included."
	price21  = "the length of the element must not exceed 21 characters, with at most 8 decimal digits."
	vat30    = "the length of the element cannot exceed 30 characters and it must begin with two alphabetic characters."
	name80   = "for a legal person, the length of the element must not exceed 80 characters; for a natural person, the element must be preceded by the string \"Nome#Cognome:\" and the first name and the surname must not exceed 60 characters each."
	province = "one of the codes of the list of Italian provinces must be used; otherwise the SDI does not map the value and attaches it to the FatturaPA invoice it produces."
)

// Catalog lists the rules of Check: the 59 rules of the CIUS-IT that the
// invoice data lets it check, in the order of appendix 2 of the technical
// rules, the 45 rules of the Italian extensions, in the order of appendix
// 3, then the checks of the identifiers. The descriptions follow the
// English version of the technical rules (2.5, the latest translated),
// corrected where the Italian text, which prevails, differs: BR-IT-190 and
// BR-IT-200 admit only an IPA code, BR-IT-171 also limits the name of a
// legal person, BR-IT-DE-004 cites the legal reference of version 2.7. The
// descriptions of the extension rules name the UBL elements of appendix 4.2
// and the FatturaPA elements they map to.
var Catalog = []rules.RuleInfo{
	{ID: "BR-IT-010", Severity: rules.Error, Description: "BT-1 (Invoice number): the length of the element cannot exceed 20 characters and it must contain at least one digit."},
	{ID: "BR-IT-020", Severity: rules.Error, Description: "BT-11 (Project reference): the length of the element cannot exceed 15 characters."},
	{ID: "BR-IT-030", Severity: rules.Error, Description: "BT-12 (Contract reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-040", Severity: rules.Error, Description: "BT-13 (Purchase order reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-050", Severity: rules.Error, Description: "BT-15 (Receiving advice reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-060", Severity: rules.Error, Description: "BT-16 (Despatch advice reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-070", Severity: rules.Error, Description: "BT-17 (Tender or lot reference): the length of the element cannot exceed 15 characters."},
	{ID: "BR-IT-071", Severity: rules.Error, Description: "BT-18 (Invoiced object identifier): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-080", Severity: rules.Error, Description: "BT-19 (Buyer accounting reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-081", Severity: rules.Error, Description: "BT-22 (Invoice note): the length of the element cannot exceed 200 characters."},
	{ID: "BR-IT-090", Severity: rules.Error, Description: "BT-25 (Preceding Invoice number): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-091", Severity: rules.Error, Description: "BT-27 (Seller name): " + name80},
	{ID: "BR-IT-DC-100", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT: a BT-29 (Seller identifier) that begins with \"EORI:\" must have a length between 18 and 22 characters; one that begins with \"ALBO:\" cannot exceed 137 characters (ALBO:ProfessionalRegister#RegisterListingNumber#RegistrationDate, the date as YYYY-MM-DD); one that begins with \"REA:\" must have a length between 8 and 27 characters and be written REA:Office#REANumber."},
	{ID: "BR-IT-DC-110", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, a BT-30 (Seller legal registration identifier) that begins with \"CF:\" must have a length between 14 and 19 characters, and one with the scheme identifier 0210 a length between 11 and 16 characters."},
	{ID: "BR-IT-120", Severity: rules.Error, Description: "BT-31 (Seller VAT identifier): " + vat30},
	{ID: "BR-IT-122", Severity: rules.Error, Description: "BT-33 (Seller additional legal information): the length of the element cannot exceed 21 characters."},
	{ID: "BR-IT-DC-140", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-35 (Seller address line 1), BT-37 (Seller city) and BT-38 (Seller post code) must be filled in."},
	{ID: "BR-IT-DC-141", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-36 (Seller address line 2) must contain the house number or the acronym \"s.n.c.\" (no house number)."},
	{ID: "BR-IT-DC-150", Severity: rules.Warning, Description: "If BT-40 (Seller country code) is IT, for BT-39 (Seller country subdivision) " + province},
	{ID: "BR-IT-DC-161", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-42 (Seller contact telephone number) must consist of 5 to 12 characters."},
	{ID: "BR-IT-171", Severity: rules.Error, Description: "BT-44 (Buyer name): " + name80},
	{ID: "BR-IT-180", Severity: rules.Error, Description: "BT-48 (Buyer VAT identifier): " + vat30},
	{ID: "BR-IT-190", Severity: rules.Error, Description: "BT-49 (Buyer electronic address) must contain an IPA code: the only value provided for BT-49-1 (Buyer electronic address identification scheme identifier) is 0201."},
	{ID: "BR-IT-200", Severity: rules.Error, Description: "If BT-49-1 (Buyer electronic address identification scheme identifier) is 0201, BT-49 (Buyer electronic address) must contain an IPA code of 6 upper-case alphanumeric characters."},
	{ID: "BR-IT-210", Severity: rules.Error, Description: "BT-50 (Buyer address line 1), BT-52 (Buyer city) and BT-53 (Buyer post code) must be filled in: all the elements of the buyer address are mandatory except the street number."},
	{ID: "BR-IT-220", Severity: rules.Warning, Description: "For BT-54 (Buyer country subdivision) " + province},
	{ID: "BR-IT-DC-221", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-59 (Payee name) must consist of at most 200 characters."},
	{ID: "BR-IT-222", Severity: rules.Error, Description: "BT-62 (Seller tax representative name): " + name80},
	{ID: "BR-IT-230", Severity: rules.Error, Description: "BT-63 (Seller tax representative VAT identifier): " + vat30},
	{ID: "BR-IT-240", Severity: rules.Error, Description: "If BT-80 (Deliver to country code) is IT, BT-75 (Deliver to address line 1), BT-77 (Deliver to city) and BT-78 (Deliver to post code) must be filled in."},
	{ID: "BR-IT-250", Severity: rules.Warning, Description: "If BT-80 (Deliver to country code) is IT, for BT-79 (Deliver to country subdivision) " + province},
	{ID: "BR-IT-260", Severity: rules.Error, Description: "BG-16 (Payment instructions) is mandatory."},
	{ID: "BR-IT-261", Severity: rules.Error, Description: "BT-20 (Payment terms) must be filled in."},
	{ID: "BR-IT-DC-262", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-83 (Remittance information) must consist of at most 60 characters."},
	{ID: "BR-IT-270", Severity: rules.Error, Description: "BT-84 (Payment account identifier) must be an IBAN."},
	{ID: "BR-IT-280", Severity: rules.Error, Description: "BT-86 (Payment service provider identifier): the length of the element must be between 8 and 11 characters (BIC)."},
	{ID: "BR-IT-290", Severity: rules.Error, Description: "BT-92 (Document level allowance amount) and BT-99 (Document level charge amount): " + amount15},
	{ID: "BR-IT-300", Severity: rules.Error, Description: "BT-112 (Invoice total amount with VAT): " + amount15},
	{ID: "BR-IT-310", Severity: rules.Error, Description: "BT-114 (Rounding amount): " + amount15},
	{ID: "BR-IT-320", Severity: rules.Error, Description: "BT-115 (Amount due for payment): " + amount15},
	{ID: "BR-IT-330", Severity: rules.Error, Description: "BT-116 (VAT category taxable amount): " + amount15},
	{ID: "BR-IT-340", Severity: rules.Error, Description: "BT-117 (VAT category tax amount): " + amount15},
	{ID: "BR-IT-350", Severity: rules.Error, Description: "BT-118 (VAT category code), BT-95 (Document level allowance VAT category code), BT-102 (Document level charge VAT category code) and BT-151 (Invoiced item VAT category code): the only accepted values are AE, E, S, G, K, Z and B."},
	{ID: "BR-IT-DC-351", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT, BT-120 (VAT exemption reason text) must consist of at most 105 Latin characters (Unicode blocks Basic Latin and Latin-1 Supplement)."},
	{ID: "BR-IT-360", Severity: rules.Error, Description: "If BT-122 (Supporting document reference) is filled in, at least one of BT-124 (External document location) and BT-125 (Attached document) must be filled in."},
	{ID: "BR-IT-361", Severity: rules.Error, Description: "BT-126 (Invoice line identifier): the element must be numeric and not greater than 9999."},
	{ID: "BR-IT-370", Severity: rules.Error, Description: "BT-128 (Invoice line object identifier): the length of the element cannot exceed 100 characters."},
	{ID: "BR-IT-380", Severity: rules.Error, Description: "BT-129 (Invoiced quantity): " + price21},
	{ID: "BR-IT-390", Severity: rules.Error, Description: "BT-131 (Invoice line net amount): " + amount15},
	{ID: "BR-IT-400", Severity: rules.Error, Description: "BT-132 (Referenced purchase order line reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-410", Severity: rules.Error, Description: "BT-133 (Invoice line Buyer accounting reference): the length of the element cannot exceed 20 characters."},
	{ID: "BR-IT-420", Severity: rules.Error, Description: "BT-136 (Invoice line allowance amount) and BT-141 (Invoice line charge amount): " + amount15},
	{ID: "BR-IT-430", Severity: rules.Error, Description: "BT-146 (Item net price): " + price21},
	{ID: "BR-IT-440", Severity: rules.Error, Description: "BT-155 (Item Seller's identifier): the length of the element cannot exceed 35 characters."},
	{ID: "BR-IT-470", Severity: rules.Error, Description: "BT-158 (Item classification identifier): the length of the element cannot exceed 35 characters."},
	{ID: "BR-IT-DC-510", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT and the invoice is subject to split payment, which a VAT category code B shows, it must use the VAT category B only and say so in the payment terms (BT-20): \"The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)\"."},
	{ID: "BR-IT-520", Severity: rules.Error, Description: "At least one of BT-48 (Buyer VAT identifier) and BT-47 (Buyer legal registration identifier) must be filled in. BT-47, if present, must begin with \"CF:\" and have a length between 14 and 19 characters, or have the scheme identifier 0210 and a length between 11 and 16 characters."},
	{ID: "BR-IT-521", Severity: rules.Error, Description: "The EORI code (BT-46 Buyer identifier) must be preceded by the prefix \"EORI:\" and the code must have between 13 and 17 alphanumeric characters."},
	{ID: "BR-IT-530", Severity: rules.Error, Description: "BT-153 (Item name) and BT-154 (Item description), which the SDI joins into one description, must hold at most 1000 characters of the Unicode blocks Basic Latin and Latin-1 Supplement."},

	{ID: "BR-IT-DE-001", Severity: rules.Error, Description: "The extension of the issuer (SoggettoEmittente, FatturaPA 1.6) must contain a cbc:TypeCode element valued exclusively with the values CC or TZ."},
	{ID: "BR-IT-DE-002", Severity: rules.Error, Description: "The extension of the document type (TipoDocumento, FatturaPA 2.1.1.1) must contain a cbc:TypeCode element valued exclusively with the values of the document type of the FatturaPA specification."},
	{ID: "BR-IT-DE-003", Severity: rules.Error, Description: "The extension of the withholding type (TipoRitenuta, FatturaPA 2.1.1.5.1) must contain a cbc:TypeCode element valued exclusively with the values of the withholding type of the FatturaPA specification."},
	{ID: "BR-IT-DE-004", Severity: rules.Error, Description: "The extension Art73 (FatturaPA 2.1.1.12) must contain a cbc:TypeCode element valued exclusively with the value SI, to apply article 91 of Legislative Decree 10/2026 (formerly article 73 of Presidential Decree 633/72)."},
	{ID: "BR-IT-DE-005", Severity: rules.Error, Description: "The extension of the rounding of a VAT summary (RiepilogoIVA:Arrotondamento, FatturaPA 2.2.2.4) must contain a cbc:Amount element whose length cannot exceed 15 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-006", Severity: rules.Error, Description: "The extension of the discounts and surcharges on the document total (ScontoMaggiorazione, FatturaPA 2.1.1.8) must contain one or more cac:AllowanceCharge elements."},
	{ID: "BR-IT-DE-007", Severity: rules.Error, Description: "The amount of a discount or surcharge of the extension ScontoMaggiorazione (cbc:Amount) must contain from 4 to 21 characters, 2 decimal digits included (at most 8)."},
	{ID: "BR-IT-DE-008", Severity: rules.Error, Description: "The percentage of a discount or surcharge of the extension ScontoMaggiorazione (cbc:MultiplierFactorNumeric), if present, must contain from 4 to 6 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-009", Severity: rules.Error, Description: "The withholding data (FatturaPA 2.1.1.5) are given in a cac:WithholdingTaxTotal element: an invoice with the extension of the withholding type (TipoRitenuta) must contain one."},
	{ID: "BR-IT-DE-010", Severity: rules.Error, Description: "The total amount of the withholdings (cac:WithholdingTaxTotal/cbc:TaxAmount) must contain from 4 to 15 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-011", Severity: rules.Error, Description: "From 1 to at most 2 withholdings (cac:TaxSubtotal) can be included in a cac:WithholdingTaxTotal."},
	{ID: "BR-IT-DE-012", Severity: rules.Error, Description: "The amount of each withholding (cac:TaxSubtotal/cbc:TaxAmount, FatturaPA 2.1.1.5.2) must contain from 4 to 15 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-013", Severity: rules.Error, Description: "For each withholding, the withholding type (extension TipoRitenuta), the withholding amount (cbc:TaxAmount), the withholding category (cac:TaxCategory) with the identifier S (cbc:ID), the withholding rate (cbc:Percent) of 4 to 6 characters, 2 decimal digits included, the withholding scheme (cac:TaxScheme/cbc:ID) SWT and the reason for payment (cac:TaxScheme/cbc:TaxTypeCode) must be given."},
	{ID: "BR-IT-DE-014", Severity: rules.Error, Description: "To refer to a main invoice relating to the transport of goods (cac:BillingReference/cac:AdditionalDocumentReference, FatturaPA 2.1.10), its details must be given with the cbc:ID and cbc:IssueDate elements."},
	{ID: "BR-IT-DE-015", Severity: rules.Error, Description: "The identifier of the main invoice (cac:BillingReference/cac:AdditionalDocumentReference/cbc:ID, FatturaPA 2.1.10.1) cannot exceed 20 characters."},
	{ID: "BR-IT-DE-016", Severity: rules.Error, Description: "The date of the transport document (cac:DespatchDocumentReference/cbc:IssueDate, FatturaPA 2.1.8.2) must be given at document level."},
	{ID: "BR-IT-DE-017", Severity: rules.Error, Description: "The tax regime of the seller (cac:PartyTaxScheme/cbc:TaxLevelCode, FatturaPA 1.2.1.8) must be valued exclusively with the values of the tax regime code list (RegimeFiscale)."},
	{ID: "BR-IT-DE-018", Severity: rules.Error, Description: "The permanent establishment of the seller (cac:AgentParty, FatturaPA 1.2.3) must give the complete address: street and house number (cbc:StreetName), municipality (cbc:CityName), postcode (cbc:PostalZone), province (cbc:CountrySubentity) and country (cac:Country/cbc:IdentificationCode)."},
	{ID: "BR-IT-DE-019", Severity: rules.Error, Description: "The street and house number of the permanent establishment (cbc:StreetName, FatturaPA 1.2.3.1 and 1.2.3.2) cannot exceed 60 characters."},
	{ID: "BR-IT-DE-020", Severity: rules.Error, Description: "The municipality of the permanent establishment (cbc:CityName, FatturaPA 1.2.3.4) cannot exceed 60 characters."},
	{ID: "BR-IT-DE-021", Severity: rules.Error, Description: "The postcode of the permanent establishment (cbc:PostalZone, FatturaPA 1.2.3.3) must consist of 5 digits."},
	{ID: "BR-IT-DE-022", Severity: rules.Error, Description: "The province of the permanent establishment (cbc:CountrySubentity, FatturaPA 1.2.3.5) cannot exceed 2 characters."},
	{ID: "BR-IT-DE-023", Severity: rules.Error, Description: "The third-party intermediary or issuer of the seller (cac:ServiceProviderParty/cac:Party, FatturaPA 1.5) must contain the VAT number (cac:PartyTaxScheme/cbc:CompanyID), the tax scheme VAT, and the company name (cac:PartyName/cbc:Name) or the natural person (cac:Person, first name and surname)."},
	{ID: "BR-IT-DE-024", Severity: rules.Error, Description: "The tax code (Codice Fiscale) of the seller tax representative (cac:TaxRepresentativeParty/cac:PartyIdentification/cbc:ID, FatturaPA 1.3.1.2) has the schemeID 0210 and must have from 11 to 16 alphanumeric characters."},
	{ID: "BR-IT-DE-025", Severity: rules.Error, Description: "The EORI code of the seller tax representative (cac:TaxRepresentativeParty/cac:PartyIdentification/cbc:ID, FatturaPA 1.3.1.3.5) must be preceded by the prefix \"EORI:\" and the code must have from 13 to 17 alphanumeric characters."},
	{ID: "BR-IT-DE-026", Severity: rules.Error, Description: "The tax code (Codice Fiscale) of the carrier (cac:Delivery/cac:CarrierParty/cac:PartyIdentification/cbc:ID, FatturaPA 2.1.9.1.2) has the schemeID 0210 and must have from 11 to 16 alphanumeric characters."},
	{ID: "BR-IT-DE-027", Severity: rules.Error, Description: "The name of the carrier (cac:PartyName/cbc:Name, FatturaPA 2.1.9.1.3.1), if given, cannot exceed 80 characters; its first name and surname (cac:Person/cbc:FirstName and cbc:FamilyName), if given instead, cannot exceed 60 characters each."},
	{ID: "BR-IT-DE-028", Severity: rules.Error, Description: "The VAT number of the carrier (cac:PartyTaxScheme/cbc:CompanyID, FatturaPA 2.1.9.1.1) cannot exceed 30 characters."},
	{ID: "BR-IT-DE-029", Severity: rules.Error, Description: "The tax scheme identifier of the carrier (cac:PartyTaxScheme/cac:TaxScheme/cbc:ID) must be VAT."},
	{ID: "BR-IT-DE-030", Severity: rules.Error, Description: "The driving licence number of the carrier (cac:Person/cac:IdentityDocumentReference/cbc:ID, FatturaPA 2.1.9.1.4), if given, cannot exceed 20 characters, and the document type (cbc:DocumentTypeCode) must be 40."},
	{ID: "BR-IT-DE-031", Severity: rules.Error, Description: "The gross or net weight of the shipment (cbc:GrossWeightMeasure, cbc:NetWeightMeasure, FatturaPA 2.1.9.7 and 2.1.9.8) must have from 4 to 7 characters, 1 or 2 decimal digits included."},
	{ID: "BR-IT-DE-032", Severity: rules.Error, Description: "The number of packages of the shipment (cbc:TotalTransportHandlingUnitQuantity, FatturaPA 2.1.9.4) can be from 1 to 9999."},
	{ID: "BR-IT-DE-033", Severity: rules.Error, Description: "The description of the goods (cac:Shipment/cac:GoodsItem/cbc:Description, FatturaPA 2.1.9.5) cannot exceed 100 characters."},
	{ID: "BR-IT-DE-034", Severity: rules.Error, Description: "The means of transport (cac:Shipment/cac:ShipmentStage/cbc:TransportMeansTypeCode, FatturaPA 2.1.9.2) cannot exceed 80 characters."},
	{ID: "BR-IT-DE-035", Severity: rules.Error, Description: "The reason for transport (cac:Delivery/cac:Despatch/cbc:Instructions, FatturaPA 2.1.9.3) cannot exceed 100 characters."},
	{ID: "BR-IT-DE-036", Severity: rules.Error, Description: "The type of goods delivery (cac:DeliveryTerms/cbc:ID, FatturaPA 2.1.9.11) must have 3 characters."},
	{ID: "BR-IT-DE-037", Severity: rules.Error, Description: "The early payment discount (cac:PaymentTerms/cbc:SettlementDiscountAmount, FatturaPA 2.4.2.17) must have at least 4 characters and cannot exceed 15 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-038", Severity: rules.Error, Description: "The penalty for late payment (cac:PaymentTerms/cbc:PenaltyAmount, FatturaPA 2.4.2.19) must have at least 4 characters and cannot exceed 15 characters, 2 decimal digits included."},
	{ID: "BR-IT-DE-039", Severity: rules.Error, Description: "The order number of an invoice line (cac:OrderLineReference/cac:OrderReference/cbc:ID, FatturaPA 2.1.2.2) cannot exceed 20 characters."},
	{ID: "BR-IT-DE-040", Severity: rules.Error, Description: "The reference to a transport document line from an invoice line (cac:DespatchLineReference/cbc:LineID), which the SDI does not support, must always be NA."},
	{ID: "BR-IT-DE-041", Severity: rules.Error, Description: "The transport document number of an invoice line (cac:DespatchLineReference/cac:DocumentReference/cbc:ID, FatturaPA 2.1.8.1) cannot exceed 20 characters."},
	{ID: "BR-IT-DE-042", Severity: rules.Error, Description: "The date of the transport document (cac:DespatchLineReference/cac:DocumentReference/cbc:IssueDate, FatturaPA 2.1.8.2) must be given in the invoice line."},
	{ID: "BR-IT-DE-050", Severity: rules.Error, Description: "If the third-party intermediary or issuer is Italian, its tax code (cac:PartyIdentification/cbc:ID, FatturaPA 1.5.1.2), qualified by the schemeID 0210, must be given, with from 11 to 16 alphanumeric characters."},
	{ID: "BR-IT-DE-051", Severity: rules.Error, Description: "The name of the third-party intermediary (cac:PartyName/cbc:Name, FatturaPA 1.5.1.3.1) cannot exceed 80 characters."},
	{ID: "BR-IT-DE-052", Severity: rules.Error, Description: "If BT-40 (Seller country code) is IT and an invoice line references a purchase order line (BT-132 Referenced purchase order line reference), the purchase order reference must be given in the header (BT-13 Purchase order reference) or in the line."},

	{ID: RuleProfile, Severity: rules.Warning, Description: "The specification identifier (BT-24) of a CIUS-IT invoice should match the countries of the seller (BT-40) and of the buyer (BT-55): " + urnDomestic + " for a domestic invoice (seller in Italy), " + urnCrossBorder + " for a cross-border one (seller outside Italy, buyer in Italy)."},
	{ID: RulePIVAFormat, Severity: rules.Warning, Description: "A Partita IVA should have 11 digits: in a VAT identifier (BT-31, BT-48, BT-63) after the prefix IT, in a party identifier (BT-29, BT-46), a legal registration identifier (BT-30, BT-47) or an electronic address (BT-34, BT-49) of scheme 0211."},
	{ID: RulePIVAKey, Severity: rules.Warning, Description: "The check digit of a Partita IVA, its eleventh digit, should be valid (Luhn algorithm on the first ten digits)."},
	{ID: RuleCFFormat, Severity: rules.Warning, Description: "A Codice Fiscale should have the format of the code of a natural person (16 characters: 6 letters, 2 digits, a month letter, 2 digits, a letter, 3 digits and a check letter, a digit possibly replaced by a letter L to V) or of a legal person (11 digits): in a legal registration identifier (BT-30, BT-47) that begins with \"CF:\" or of scheme 0210, in a party identifier (BT-29, BT-46) or an electronic address (BT-34, BT-49) of scheme 0210, and in the tax registration identifier (BT-32) of a seller in Italy."},
	{ID: RuleCFKey, Severity: rules.Warning, Description: "The check character of a Codice Fiscale should be valid: the last letter of the code of a natural person (Ministerial Decree of 23/12/1976), the last digit of the code of a legal person (as for a Partita IVA)."},
}

// descriptions maps a rule identifier to its description.
var descriptions = sync.OnceValue(func() map[string]string {
	m := make(map[string]string, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Description
	}
	return m
})

// severities maps a rule identifier to its severity.
var severities = sync.OnceValue(func() map[string]rules.Severity {
	m := make(map[string]rules.Severity, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Severity
	}
	return m
})
