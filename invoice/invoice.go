// Package invoice is a Go model of an electronic invoice following the
// semantic data model of EN 16931-1, with the extensions used by the
// Factur-X / ZUGFeRD EXTENDED and EXTENDED-CTC-FR profiles.
//
// Field comments give the business term (BT) or group (BG) of EN 16931; terms
// marked EXT exist only in EXTENDED profiles. The structure follows the CII
// syntax closely enough that an invoice read from XML is written back without
// loss: the elements of the EXTENDED data set that have no field of their own,
// and the attributes that none carries, are kept in the Extensions field of
// the value that encloses them (see Extension).
//
// Amounts, quantities and rates are Decimal values: they keep the number of
// decimals they were given. Calculate fills in the derived amounts (line net
// amounts, totals, VAT breakdown).
package invoice

// Invoice is an invoice or a credit note.
type Invoice struct {
	// Profile is the Factur-X profile of the invoice (BT-24). When the invoice
	// is read from XML it is derived from SpecificationID.
	Profile Profile
	// SpecificationID is the raw specification identifier (BT-24) read from
	// the XML. Writers use Profile.URN() unless SpecificationID identifies the
	// same profile (this keeps, for example, the XRechnung version).
	SpecificationID string
	// BusinessProcess is the business process type (BT-23). In France it is
	// the "cadre de facturation" (B1, S1, M1…).
	BusinessProcess string
	// TestIndicator marks a test document (CII TestIndicator, EXT).
	TestIndicator bool
	// CopyIndicator marks the document as a copy of the original (CII
	// CopyIndicator, EXT).
	CopyIndicator bool

	Number    string // BT-1 Invoice number
	TypeCode  string // BT-3 Invoice type code (UNTDID 1001: 380, 381, 384, 389…)
	IssueDate Date   // BT-2 Invoice issue date
	Notes     []Note // BG-1 Invoice notes
	Language  string // Document language (EXT, ISO 639-1)

	Currency    string // BT-5 Invoice currency code (ISO 4217)
	TaxCurrency string // BT-6 VAT accounting currency code

	TaxPointDate Date // BT-7 Value added tax point date
	// TaxPointDateCode is the VAT point date code (BT-8): TaxPointInvoiceDate,
	// TaxPointDeliveryDate or TaxPointPaymentDate. In France it carries the
	// option for paying the VAT on debits (TaxPointDebits). BT-7 and BT-8
	// are exclusive (BR-CO-03).
	TaxPointDateCode string

	BuyerReference string // BT-10 Buyer reference (Leitweg-ID in Germany)

	ProjectReference string // BT-11 Project reference
	ProjectName      string // Project name (mandatory in CII when BT-11 is present)

	ContractReference        string // BT-12 Contract reference
	PurchaseOrderReference   string // BT-13 Purchase order reference
	SalesOrderReference      string // BT-14 Sales order reference
	ReceivingAdviceReference string // BT-15 Receiving advice reference
	DespatchAdviceReference  string // BT-16 Despatch advice reference

	// AdditionalDocuments holds the additional supporting documents (BG-24,
	// type code 916) and, in CII, the tender or lot reference (BT-17, type
	// code 50) and the invoiced object identifier (BT-18, type code 130). Use
	// TenderReference and InvoicedObject to read the last two.
	AdditionalDocuments []AdditionalDocument

	BuyerAccountingReference string // BT-19 Buyer accounting reference

	PrecedingInvoices []DocumentReference // BG-3 Preceding invoice references

	Seller                  Party  // BG-4 Seller
	Buyer                   Party  // BG-7 Buyer
	SellerTaxRepresentative *Party // BG-11 Seller tax representative party
	Payee                   *Party // BG-10 Payee
	SalesAgent              *Party // Sales agent (EXT)
	BuyerAgent              *Party // Buyer agent (EXT)
	Invoicer                *Party // Invoicer (EXT)
	Invoicee                *Party // Invoicee (EXT)
	Payer                   *Party // Payer (EXT)
	ProductEndUser          *Party // Product end user (EXT)

	// DeliveryTerms are the delivery terms of the invoice, such as an
	// Incoterms code (EXT).
	DeliveryTerms *DeliveryTerms
	// UltimateCustomerOrders are the orders of the ultimate customer that the
	// invoice refers to (EXT).
	UltimateCustomerOrders []DocumentReference

	ShipTo         *Party             // BG-13 Delivery information: BT-70 name, BT-71 location identifier, BG-15 address
	UltimateShipTo *Party             // Ultimate deliver-to party (EXT)
	ShipFrom       *Party             // Ship-from party (EXT)
	DeliveryDate   Date               // BT-72 Actual delivery date
	DeliveryNote   *DocumentReference // Delivery note reference, with its date (EXT)

	InvoicingPeriod Period // BG-14 Invoicing period

	RemittanceInformation string         // BT-83 Remittance information
	CreditorReferenceID   string         // BT-90 Bank assigned creditor identifier
	PaymentMeans          []PaymentMeans // BG-16 Payment instructions (one entry per means of payment)
	PaymentTerms          []PaymentTerms // BT-20, BT-9, BT-89 (one entry, several in EXTENDED)

	// CurrencyExchange is the exchange rate from the invoice currency (BT-5)
	// to the VAT accounting currency (BT-6), used for BT-111 (EXT).
	CurrencyExchange *CurrencyExchange

	AllowancesCharges []AllowanceCharge // BG-20 Document level allowances, BG-21 charges
	// LogisticsCharges are the logistics service charges (EXT: freight,
	// packing, insurance…). They count as document level charges in the
	// charge total BT-108 and in the VAT breakdown.
	LogisticsCharges []LogisticsCharge
	VATBreakdown     []VATBreakdown // BG-23 VAT breakdown
	Totals           Totals         // BG-22 Document totals

	// ThirdPartyCharges are the charges collected on behalf of a third party
	// (BG-34, EXTENDED from Factur-X 1.09): not part of the invoice total,
	// they are added to the amount due for payment (BT-115).
	ThirdPartyCharges []ThirdPartyCharge

	Lines []Line // BG-25 Invoice lines

	// Italy is the national block of the Italian FatturaPA format (see
	// Italy), read and written by package fatturapa only.
	Italy *Italy

	// Extensions are the elements and attributes of the document that the
	// other fields do not carry, outside the values that have Extensions of
	// their own (see Extension).
	Extensions []Extension
}

// Note is an invoice note (BG-1) or a line note.
type Note struct {
	SubjectCode string // BT-21 Invoice note subject code (UNTDID 4451: AAI, PMT, PMD, AAB, TXD…)
	Content     string // BT-22 Invoice note
	ContentCode string // Note content code (EXT)

	Extensions []Extension // see Extension
}

// Identifier is an identifier with its optional scheme.
type Identifier struct {
	ID     string
	Scheme string // ISO 6523 ICD for party identifiers (0002 SIREN, 0009 SIRET, 0088 GLN…), EAS for electronic addresses
}

// IsZero reports whether the identifier is empty.
func (id Identifier) IsZero() bool { return id.ID == "" && id.Scheme == "" }

// Party is a trade party: seller, buyer, payee, tax representative,
// deliver-to party or one of the EXTENDED parties. Fields that do not exist
// for a given role are ignored by writers.
type Party struct {
	// IDs are the party identifiers (BT-29, BT-46, BT-60, BT-71). In CII an
	// identifier without scheme is written as ram:ID, one with a scheme as
	// ram:GlobalID.
	IDs  []Identifier
	Name string // BT-27, BT-44, BT-59, BT-62, BT-70

	// Description is the seller additional legal information (BT-33).
	Description string

	// LegalRegistration is the legal registration identifier (BT-30, BT-47,
	// BT-61), with its scheme (0002 for a SIREN).
	LegalRegistration Identifier
	// TradingName is the trading name (BT-28, BT-45).
	TradingName string

	Contact *Contact // BG-6, BG-9 contact
	Address Address  // BG-5, BG-8, BG-12, BG-15 postal address

	// ElectronicAddress is the electronic address (BT-34, BT-49) with its EAS
	// scheme (0225 for the French directory, 0204 for a Leitweg-ID, EM for an
	// e-mail address).
	ElectronicAddress Identifier

	VATID             string // BT-31, BT-48, BT-63 VAT identifier (CII scheme VA)
	TaxRegistrationID string // BT-32 Seller tax registration identifier (CII scheme FC)

	Italy *ItalyParty // FatturaPA data of the party (package fatturapa, see Italy)

	// Extensions hold, for instance, the role code, the contacts after the
	// first one, or the fax number of the contact (EXT, see Extension).
	Extensions []Extension
}

// IsZero reports whether the party holds no data.
func (p *Party) IsZero() bool {
	return p == nil || (len(p.IDs) == 0 && p.Name == "" && p.Description == "" &&
		p.LegalRegistration.IsZero() && p.TradingName == "" && p.Contact == nil &&
		p.Address.IsZero() && p.ElectronicAddress.IsZero() && p.VATID == "" && p.TaxRegistrationID == "" &&
		p.Italy == nil && len(p.Extensions) == 0)
}

// Contact is a contact point (BG-6, BG-9).
type Contact struct {
	PersonName     string // BT-41, BT-56 (CII PersonName)
	DepartmentName string // BT-41, BT-56 (CII DepartmentName)
	Phone          string // BT-42, BT-57
	Email          string // BT-43, BT-58
}

// Address is a postal address (BG-5, BG-8, BG-12, BG-15).
type Address struct {
	Line1              string // BT-35, BT-50, BT-64, BT-75
	Line2              string // BT-36, BT-51, BT-65, BT-76
	Line3              string // BT-162, BT-163, BT-164, BT-165
	City               string // BT-37, BT-52, BT-66, BT-77
	PostCode           string // BT-38, BT-53, BT-67, BT-78
	CountrySubdivision string // BT-39, BT-54, BT-68, BT-79
	CountryCode        string // BT-40, BT-55, BT-69, BT-80 (ISO 3166-1 alpha-2)
}

// IsZero reports whether the address is empty.
func (a Address) IsZero() bool { return a == Address{} }

// Period is a period of time (BG-14, BG-26).
type Period struct {
	Start       Date   // BT-73, BT-134
	End         Date   // BT-74, BT-135
	Description string // EXT
}

// IsZero reports whether the period is empty.
func (p Period) IsZero() bool { return p.Start.IsZero() && p.End.IsZero() && p.Description == "" }

// DocumentReference is a reference to another document: a preceding invoice
// (BG-3) or, in EXTENDED, an invoice referenced by a line, a delivery note or
// an order of the ultimate customer.
type DocumentReference struct {
	ID        string // BT-25 Preceding invoice reference
	IssueDate Date   // BT-26 Preceding invoice issue date
	TypeCode  string // Type code of the referenced document (EXT)
	LineID    string // Referenced line (EXT)

	Extensions []Extension // see Extension
}

// IsZero reports whether the reference holds no data.
func (r *DocumentReference) IsZero() bool {
	return r == nil || (r.ID == "" && r.IssueDate.IsZero() && r.TypeCode == "" && r.LineID == "" && len(r.Extensions) == 0)
}

// AdditionalDocument is an additional supporting document (BG-24), or in CII
// the tender reference (TypeCode 50) or invoiced object identifier (TypeCode
// 130).
type AdditionalDocument struct {
	ID          string // BT-122 Supporting document reference (BT-17, BT-18 for type codes 50 and 130)
	TypeCode    string // 916 for BG-24 (default when empty), 50 for BT-17, 130 for BT-18
	Description string // BT-123 Supporting document description
	URI         string // BT-124 External document location

	Attachment         []byte // BT-125 Attached document
	AttachmentMimeCode string // BT-125-1 Attached document MIME code
	AttachmentFilename string // BT-125-2 Attached document filename

	ReferenceTypeCode string // BT-18-1 Scheme identifier of the invoiced object (UNTDID 1153)
	IssueDate         Date   // Document date (EXT)

	Extensions []Extension // see Extension
}

// TenderReference returns the tender or lot reference (BT-17).
func (inv *Invoice) TenderReference() string {
	for _, d := range inv.AdditionalDocuments {
		if d.TypeCode == "50" {
			return d.ID
		}
	}
	return ""
}

// InvoicedObject returns the invoiced object identifier (BT-18) and its
// scheme (BT-18-1).
func (inv *Invoice) InvoicedObject() Identifier {
	for _, d := range inv.AdditionalDocuments {
		if d.TypeCode == "130" {
			return Identifier{ID: d.ID, Scheme: d.ReferenceTypeCode}
		}
	}
	return Identifier{}
}

// PaymentMeans is one means of payment (BG-16) with its credit transfer
// (BG-17), payment card (BG-18) or direct debit (BG-19) details.
type PaymentMeans struct {
	// ID identifies the means of payment, so that payment terms can refer
	// to it (NL-GA-04 of the Dutch G-account extension, see
	// GAccountExtension, which requires the value GACCOUNT on the means
	// paid to the G-account). Only UBL with that extension carries it.
	ID string

	TypeCode    string // BT-81 Payment means type code (UNTDID 4461: 30, 58, 59, 48, 49…)
	Information string // BT-82 Payment means text

	PayeeIBAN          string // BT-84 Payment account identifier, as an IBAN
	PayeeProprietaryID string // BT-84 Payment account identifier, when it is not an IBAN
	PayeeAccountName   string // BT-85 Payment account name
	PayeeBIC           string // BT-86 Payment service provider identifier

	CardID         string // BT-87 Payment card primary account number (at most the last 4 to 6 digits)
	CardholderName string // BT-88 Payment card holder name

	PayerIBAN        string // BT-91 Debited account identifier
	PayerAccountName string // BT-216 Debited account name (EXTENDED)
	PayerBIC         string // BT-215 Debited account payment service provider identifier (EXTENDED)

	Italy *ItalyPayment // FatturaPA payment detail (package fatturapa, see Italy)

	Extensions []Extension // see Extension
}

// ThirdPartyCharge is a charge collected on behalf of a third party (BG-34).
type ThirdPartyCharge struct {
	Reason string  // BT-180 Charges specification
	Amount Decimal // BT-179 Charge amount collected on behalf of a third party

	Extensions []Extension // see Extension
}

// DeliveryTerms are delivery terms (EXT).
type DeliveryTerms struct {
	Code string // Delivery terms code (Incoterms: EXW, FCA, DAP…)
	// LocationCountryCode and LocationName give the place the terms refer
	// to (EXT, CII RelevantTradeLocation).
	LocationCountryCode string
	LocationName        string
}

// CurrencyExchange is a currency exchange rate (EXT).
type CurrencyExchange struct {
	SourceCurrency string  // Source currency code: the invoice currency (BT-5)
	TargetCurrency string  // Target currency code: the VAT accounting currency (BT-6)
	Rate           Decimal // Conversion rate
	Date           Date    // Date of the conversion rate
}

// LogisticsCharge is a logistics service charge (EXT).
type LogisticsCharge struct {
	Description string  // Description of the service (freight, packing…)
	Amount      Decimal // Amount of the charge, without VAT

	// VATCategoryCode and VATRate are the VAT of the charge. EXTENDED
	// allows several taxes per charge: the first is read here, the others
	// are kept as extensions.
	VATCategoryCode string
	VATRate         Decimal

	Extensions []Extension // see Extension
}

// PaymentTerms are payment terms (BT-20) with the due date (BT-9) and the
// direct debit mandate (BT-89).
//
// With the Dutch G-account extension (see GAccountExtension), an invoice has
// two payment terms that split the amount due for payment (BT-115) between
// two means of payment: each one names its means in PaymentMeansID and gives
// its share in PartialAmount.
type PaymentTerms struct {
	Description          string  // BT-20 Payment terms
	DueDate              Date    // BT-9 Payment due date
	DirectDebitMandateID string  // BT-89 Mandate reference identifier
	PartialAmount        Decimal // Partial payment amount (EXT, NL-GA-03 Payment amount of the G-account extension)
	// PaymentMeansID refers to the means of payment whose ID it equals
	// (NL-GA-02 of the G-account extension). Only UBL with that extension
	// carries it.
	PaymentMeansID string

	// Extensions hold, for instance, the payment discount and penalty
	// terms (EXT, see Extension).
	Extensions []Extension
}

// IsZero reports whether the payment terms hold no data.
func (t *PaymentTerms) IsZero() bool {
	return t.Description == "" && t.DueDate.IsZero() && t.DirectDebitMandateID == "" &&
		!t.PartialAmount.IsSet() && t.PaymentMeansID == "" && len(t.Extensions) == 0
}

// AllowanceCharge is an allowance or a charge, at document level (BG-20,
// BG-21) or at line level (BG-27, BG-28). The VAT fields are only used at
// document level.
type AllowanceCharge struct {
	// Charge is true for a charge, false for an allowance.
	Charge bool

	Amount     Decimal // BT-92, BT-99, BT-136, BT-141
	BaseAmount Decimal // BT-93, BT-100, BT-137, BT-142
	Percentage Decimal // BT-94, BT-101, BT-138, BT-143
	Reason     string  // BT-97, BT-104, BT-139, BT-144
	ReasonCode string  // BT-98, BT-105, BT-140, BT-145 (UNTDID 5189 for allowances, 7161 for charges)

	VATCategoryCode string  // BT-95, BT-102
	VATRate         Decimal // BT-96, BT-103

	Italy *ItalyCharge // FatturaPA pension fund contribution (package fatturapa, see Italy)

	Extensions []Extension // see Extension
}

// VATBreakdown is one VAT breakdown entry (BG-23).
type VATBreakdown struct {
	CategoryCode        string  // BT-118 VAT category code (UNTDID 5305: S, Z, E, AE, K, G, O, L, M, B)
	Rate                Decimal // BT-119 VAT category rate (absent for category O)
	TaxableAmount       Decimal // BT-116 VAT category taxable amount
	TaxAmount           Decimal // BT-117 VAT category tax amount
	ExemptionReason     string  // BT-120 VAT exemption reason text
	ExemptionReasonCode string  // BT-121 VAT exemption reason code (VATEX)

	Italy *ItalyVAT // FatturaPA data of the VAT summary (package fatturapa, see Italy)

	Extensions []Extension // see Extension
}

// Totals are the document totals (BG-22).
type Totals struct {
	LineTotal             Decimal // BT-106 Sum of invoice line net amounts
	AllowanceTotal        Decimal // BT-107 Sum of allowances on document level
	ChargeTotal           Decimal // BT-108 Sum of charges on document level
	TaxBasisTotal         Decimal // BT-109 Invoice total amount without VAT
	TaxTotal              Decimal // BT-110 Invoice total VAT amount
	TaxTotalInTaxCurrency Decimal // BT-111 Invoice total VAT amount in accounting currency
	GrandTotal            Decimal // BT-112 Invoice total amount with VAT
	PaidAmount            Decimal // BT-113 Paid amount
	RoundingAmount        Decimal // BT-114 Rounding amount
	DuePayable            Decimal // BT-115 Amount due for payment
}

// Line is an invoice line (BG-25).
type Line struct {
	ID               string // BT-126 Invoice line identifier
	ParentID         string // Parent line identifier (EXT, sub-lines)
	StatusCode       string // Line status code (EXT)
	StatusReasonCode string // Line status reason code (EXT: DETAIL, GROUP, INFORMATION)
	// Note is the invoice line note (BT-127). EXTENDED allows several notes
	// with codes: Note is the content of the first one, the codes and the
	// other notes are kept as extensions.
	Note string

	Item Item // BG-31 Item information

	GrossPrice            Decimal // BT-148 Item gross price
	PriceDiscount         Decimal // BT-147 Item price discount
	NetPrice              Decimal // BT-146 Item net price
	PriceBaseQuantity     Decimal // BT-149 Item price base quantity
	PriceBaseQuantityUnit string  // BT-150 Item price base quantity unit of measure code

	// PriceAllowancesCharges detail the allowances and charges applied to
	// the gross price (EXT), when the price carries more than a single
	// allowance amount. PriceDiscount (BT-147) is then the sum of the
	// allowances. When set, EXTENDED writers write these entries instead of
	// PriceDiscount; the VAT fields are not used.
	PriceAllowancesCharges []AllowanceCharge

	DeliveryTerms          *DeliveryTerms      // Line delivery terms (EXT)
	OrderReference         string              // Purchase order referenced at line level (EXT)
	OrderLineReference     string              // BT-132 Referenced purchase order line reference
	UltimateCustomerOrders []DocumentReference // Orders of the ultimate customer, with their line (EXT)

	Quantity Decimal // BT-129 Invoiced quantity
	UnitCode string  // BT-130 Invoiced quantity unit of measure code (UN/ECE Rec 20/21)

	ShipTo         *Party             // Line deliver-to party (EXT)
	UltimateShipTo *Party             // Line ultimate deliver-to party (EXT)
	DeliveryDate   Date               // Line actual delivery date (EXT)
	DeliveryNote   *DocumentReference // Line delivery note reference, with its line and date (EXT)

	VATCategoryCode string  // BT-151 Invoiced item VAT category code
	VATRate         Decimal // BT-152 Invoiced item VAT rate

	Period            Period            // BG-26 Invoice line period
	AllowancesCharges []AllowanceCharge // BG-27 line allowances, BG-28 line charges
	NetAmount         Decimal           // BT-131 Invoice line net amount

	InvoiceReference    *DocumentReference // Invoice referenced by the line (EXT)
	ObjectIdentifier    Identifier         // BT-128 Invoice line object identifier, with its scheme (BT-128-1)
	AccountingReference string             // BT-133 Invoice line Buyer accounting reference

	Italy *ItalyLine // FatturaPA data of the line (package fatturapa, see Italy)

	// Extensions are the elements and attributes of the line that the
	// other fields do not carry, such as the brand name of the item or the
	// package quantity (EXT, see Extension).
	Extensions []Extension
}

// Item is the item of an invoice line (BG-31).
type Item struct {
	GlobalID    Identifier // BT-157 Item standard identifier, with its scheme (BT-157-1, 0160 for a GTIN)
	SellerID    string     // BT-155 Item Seller's identifier
	BuyerID     string     // BT-156 Item Buyer's identifier
	Name        string     // BT-153 Item name
	Description string     // BT-154 Item description

	Attributes      []ItemAttribute  // BG-32 Item attributes
	Classifications []Classification // BT-158 Item classification identifiers
	OriginCountry   string           // BT-159 Item country of origin

	// ReferencedProducts are the products included in the item, such as
	// the components of a set (EXT).
	ReferencedProducts []ReferencedProduct
}

// ItemAttribute is an item attribute (BG-32).
type ItemAttribute struct {
	Name     string // BT-160 Item attribute name
	Value    string // BT-161 Item attribute value
	TypeCode string // Attribute type code (EXT)

	Extensions []Extension // see Extension
}

// Classification is an item classification identifier (BT-158).
type Classification struct {
	Code          string // BT-158 Item classification identifier
	ListID        string // BT-158-1 Scheme identifier (UNTDID 7143)
	ListVersionID string // BT-158-2 Scheme version identifier
	Name          string // Class name (EXT)

	Extensions []Extension // see Extension
}

// IsZero reports whether the classification holds no data.
func (c *Classification) IsZero() bool {
	return c.Code == "" && c.ListID == "" && c.ListVersionID == "" && c.Name == "" && len(c.Extensions) == 0
}

// ReferencedProduct is a product included in the item of a line (EXT).
type ReferencedProduct struct {
	ID         string       // Product identifier (CII ID)
	GlobalIDs  []Identifier // Standard identifiers, with their scheme (0160 for a GTIN)
	SellerID   string       // Seller's identifier
	BuyerID    string       // Buyer's identifier
	IndustryID string       // Identifier assigned by the industry
	Name       string       // Product name
	// Description is the product description.
	Description string
	// UnitQuantity is the quantity of the product in one unit of the item.
	UnitQuantity     Decimal
	UnitQuantityUnit string // Unit of measure code of UnitQuantity

	Extensions []Extension // see Extension
}

// Common code values.
const (
	// Invoice type codes (BT-3, UNTDID 1001).
	TypeCommercialInvoice = "380"
	TypeCreditNote        = "381"
	TypeCorrectedInvoice  = "384"
	TypePrepaymentInvoice = "386"
	TypeSelfBilledInvoice = "389"
	TypePartialInvoice    = "326"

	// VAT point date codes (BT-8), CII values. UBL writes 3, 35 and 432
	// for the same dates. BT-8 and the VAT point date (BT-7) are exclusive
	// (BR-CO-03).
	TaxPointInvoiceDate  = "5"  // the VAT is due on the invoice issue date (UBL 3)
	TaxPointDeliveryDate = "29" // the VAT is due on the delivery date (UBL 35)
	TaxPointPaymentDate  = "72" // the VAT is due on the payment date (UBL 432)

	// TaxPointDebits is the BT-8 code of a French supplier of services who
	// has opted to pay the VAT on debits ("option pour le paiement de la
	// taxe d'après les débits", a mention made mandatory by the e-invoicing
	// reform, article 242 nonies A of annex II to the CGI): the code of the
	// invoice date, 5 in CII (3 in UBL),
	// as set by rule G1.43 of the DGFiP external specifications v3.2. BT-8 is
	// mandatory only with the option, which the invoice cannot show: no rule
	// checks it. The DGFiP specifications v2.3 used 29 (35 in UBL) instead.
	TaxPointDebits = TaxPointInvoiceDate

	// VAT category codes (BT-118, BT-151, UNTDID 5305).
	VATStandard        = "S"
	VATZeroRated       = "Z"
	VATExempt          = "E"
	VATReverseCharge   = "AE"
	VATIntraCommunity  = "K"
	VATExport          = "G"
	VATOutOfScope      = "O"
	VATCanaryIslands   = "L"
	VATCeutaAndMelilla = "M"
	VATSplitPayment    = "B" // Italian split payment (scissione dei pagamenti, art. 17-ter DPR 633/1972)

	// Payment means codes (BT-81, UNTDID 4461).
	PaymentCash               = "10"
	PaymentCheque             = "20"
	PaymentCreditTransfer     = "30"
	PaymentCard               = "48"
	PaymentDirectDebit        = "49"
	PaymentStandingAgreement  = "57"
	PaymentSEPACreditTransfer = "58"
	PaymentSEPADirectDebit    = "59"

	// Party identifier schemes (ISO 6523 ICD) and electronic address schemes (EAS).
	SchemeSIREN       = "0002"
	SchemeSIRET       = "0009"
	SchemeGLN         = "0088"
	SchemeFRDirectory = "0225" // French e-invoicing directory (annuaire) address
	SchemeLeitwegID   = "0204"
	SchemeGEBA        = "0246" // German Electronic Business Address, based on the W-IdNr
	SchemeEmail       = "EM"
)
