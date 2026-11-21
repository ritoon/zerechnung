package invoice

import (
	"reflect"
	"strconv"
	"strings"
)

// Spain holds the data of the Spanish Facturae format (versions 3.2 to
// 3.2.2) that the EN 16931 fields of the model do not carry. It is the
// national block of the invoice; Party.Spain, Line.Spain, VATBreakdown.Spain,
// PaymentMeans.Spain and AdditionalDocument.Spain hold the Facturae data of
// these values. Package facturae reads and writes these blocks; packages cii
// and ubl do not carry them (see NationalData).
//
// The Facturae data that EN 16931 carries goes to the other fields of the
// model (package facturae documents the correspondence): the invoice number
// (BT-1) is the series (InvoiceSeriesCode) followed by the number
// (InvoiceNumber), the tax identification number of a party is its VAT
// identifier (BT-31, BT-48), with the prefix ES for a party resident in
// Spain, the value added tax (IVA), the Canary Islands general indirect tax
// (IGIC) and the tax on production, services and imports of Ceuta and
// Melilla (IPSI) are the VAT breakdown (categories S, Z, E, O, L and M), the
// discounts and charges are allowances and charges… The national blocks only
// hold the rest: the taxes withheld (IRPF), the equivalence surcharge
// (recargo de equivalencia), the taxes other than VAT, the subsidies, the
// administrative centres of a public buyer (DIR3 codes)… An invoice built
// from EN 16931 data alone needs no national block: package facturae
// derives each Facturae value from the EN 16931 fields.
type Spain struct {
	// File header (FileHeader).

	// Modality is the Modality when it is not I (a file holding a single
	// invoice): L for a batch.
	Modality string
	// IssuerType is the InvoiceIssuerType when it is not the one derived
	// from the invoice: EM (issued by the seller), RE (by the buyer) for a
	// self-billed invoice (BT-3 389), TE (by a third party) when ThirdParty
	// is set.
	IssuerType string
	// ThirdParty is the third party that issues the invoice on behalf of
	// the seller (ThirdParty): its tax identification number (VATID or
	// Spain.TaxID), its name and its address.
	ThirdParty *Party
	// BatchID is the BatchIdentifier when it is not the seller's tax
	// identification number followed by the invoice number (BT-1).
	BatchID string
	// BatchTotalEuros, BatchOutstandingEuros and BatchExecutableEuros are
	// the EquivalentInEuros of the totals of the batch (TotalInvoicesAmount,
	// TotalOutstandingAmount, TotalExecutableAmount), for an invoice in
	// another currency.
	BatchTotalEuros       Decimal
	BatchOutstandingEuros Decimal
	BatchExecutableEuros  Decimal

	// Invoice header (InvoiceHeader) and issue data (InvoiceIssueData).

	// SeriesCode is the InvoiceSeriesCode: the invoice number (BT-1) is the
	// series followed by the InvoiceNumber.
	SeriesCode string
	// DocumentType is the InvoiceDocumentType when it is not derived: FC
	// (complete invoice) by default, AF (self-billing) for BT-3 389; FA is
	// an abbreviated invoice.
	DocumentType string
	// Class is the InvoiceClass when it is not derived from BT-3 (OO, an
	// original invoice, and OR, a corrective invoice, see
	// SpanishClassOfTypeCode): OC (summary), CO, CR and CC (copies).
	Class string
	// Corrective holds the data of a corrective invoice (Corrective) that
	// the preceding invoice reference (BG-3) does not carry.
	Corrective   *Corrective
	PlaceOfIssue *PlaceOfIssue // PlaceOfIssue

	// Taxes (TaxesOutputs, TaxesWithheld).

	// Taxes are the taxes of TaxesOutputs other than the VAT breakdown
	// (BG-23): those of tax type 05 to 29 (excise duties, insurance premium
	// tax…). Their amounts are part of the total tax outputs
	// (TotalTaxOutputs), not of the VAT total (BT-110).
	Taxes []SpainTax
	// Withholdings are the taxes withheld (TaxesWithheld): the personal
	// income tax (IRPF, type 04) of a professional, for example. They are
	// deducted from the invoice total (InvoiceTotal).
	Withholdings []SpainTax

	// Invoice totals (InvoiceTotals) beyond EN 16931.

	Subsidies []Subsidy // Subsidies
	// PaymentsOnAccount are the payments on account (PaymentsOnAccount):
	// their total is the paid amount (BT-113).
	PaymentsOnAccount    []PaymentOnAccount
	ReimbursableExpenses []ReimbursableExpense // ReimbursableExpenses (suplidos)
	// ReimbursableTotal is the TotalReimbursableExpenses when it is not the
	// sum of the ReimbursableExpenses.
	ReimbursableTotal Decimal
	FinancialExpenses Decimal         // TotalFinancialExpenses
	AmountWithheld    *AmountWithheld // AmountsWithheld: retention as a guarantee
	PaymentInKind     *PaymentInKind  // PaymentInKind (3.2.2)
	// InvoiceTotal, OutstandingAmount and ExecutableAmount are the
	// InvoiceTotal, TotalOutstandingAmount and TotalExecutableAmount when
	// they are not the ones derived from the EN 16931 totals and the data
	// above, as the Facturae schemas define them.
	InvoiceTotal      Decimal
	OutstandingAmount Decimal
	ExecutableAmount  Decimal

	// Additional data (AdditionalData).

	RelatedInvoice string // RelatedInvoice
}

// Corrective holds the data of a corrective invoice (Corrective). The number
// of the corrected invoice and its date (3.2.2) are the preceding invoice
// reference (BG-3): the series (InvoiceSeriesCode) followed by the number.
type Corrective struct {
	// SeriesCode is the InvoiceSeriesCode of the corrected invoice, which
	// the preceding invoice reference (BT-25) begins with.
	SeriesCode string
	// ReasonCode is the ReasonCode: 01 to 16 (a correction of the data of
	// the invoice) or 80 to 85 (a correction of the taxable base).
	ReasonCode string
	// ReasonDescription is the ReasonDescription when it is not the one of
	// ReasonCode (see CorrectiveReason).
	ReasonDescription string
	TaxPeriod         Period // TaxPeriod: the tax period of the corrected invoice
	// Method is the CorrectionMethod: 01 (full items), 02 (corrected items
	// only), 03 (bulk deal in a given period), 04 (authorized by the tax
	// agency).
	Method string
	// MethodDescription is the CorrectionMethodDescription when it is not
	// the one of Method (see CorrectionMethodName).
	MethodDescription string
	AdditionalReason  string // AdditionalReasonDescription
}

// PlaceOfIssue is the place where an invoice is issued (PlaceOfIssue).
type PlaceOfIssue struct {
	PostCode    string // PostCode
	Description string // PlaceOfIssueDescription
}

// SpainTax is a tax of a Facturae invoice or line that is not in the VAT
// breakdown: a tax of TaxesOutputs of type 05 to 29, or a tax withheld.
type SpainTax struct {
	// Type is the TaxTypeCode: 04 (IRPF, personal income tax), 05 (other),
	// 06 to 29.
	Type         string
	Rate         Decimal // TaxRate
	TaxableBase  Decimal // TaxableBase/TotalAmount
	TaxableEuros Decimal // TaxableBase/EquivalentInEuros
	Amount       Decimal // TaxAmount/TotalAmount
	AmountEuros  Decimal // TaxAmount/EquivalentInEuros
	// Output holds the special taxable base and the equivalence surcharge
	// of a tax of TaxesOutputs.
	Output *SpainVAT
}

// Subsidy is a subsidy deducted from the invoice total (Subsidy).
type Subsidy struct {
	Description string  // SubsidyDescription
	Rate        Decimal // SubsidyRate
	Amount      Decimal // SubsidyAmount
}

// PaymentOnAccount is a payment on account (PaymentOnAccount).
type PaymentOnAccount struct {
	Date   Date    // PaymentOnAccountDate
	Amount Decimal // PaymentOnAccountAmount
}

// ReimbursableExpense is an expense paid on behalf of the buyer and invoiced
// to it (ReimbursableExpenses, suplido).
type ReimbursableExpense struct {
	Seller     *TaxIdentification // ReimbursableExpensesSellerParty
	Buyer      *TaxIdentification // ReimbursableExpensesBuyerParty
	IssueDate  Date               // IssueDate of the invoice of the expense
	Number     string             // InvoiceNumber
	SeriesCode string             // InvoiceSeriesCode
	Amount     Decimal            // ReimbursableExpensesAmount
}

// TaxIdentification is the tax identification of a party
// (TaxIdentification), as written.
type TaxIdentification struct {
	PersonType    string // PersonTypeCode: F (natural person) or J (legal person)
	ResidenceType string // ResidenceTypeCode: R (resident in Spain), U (in another member state), E (foreigner)
	Number        string // TaxIdentificationNumber
}

// AmountWithheld is an amount withheld from the payment as a guarantee
// (AmountsWithheld).
type AmountWithheld struct {
	Reason string  // WithholdingReason
	Rate   Decimal // WithholdingRate
	Amount Decimal // WithholdingAmount
}

// PaymentInKind is a payment in kind (PaymentInKind).
type PaymentInKind struct {
	Reason string  // PaymentInKindReason
	Amount Decimal // PaymentInKindAmount
}

// SpainParty holds the Facturae data of a party that EN 16931 does not
// carry.
type SpainParty struct {
	// PersonType is the PersonTypeCode when it is not derived: F (natural
	// person) for a party with a given name or a surname, J (legal person)
	// otherwise.
	PersonType string
	// ResidenceType is the ResidenceTypeCode when it is not derived from the
	// country of the party: R (resident in Spain), U (resident in another
	// member state of the European Union), E (foreigner).
	ResidenceType string
	// TaxID is the TaxIdentificationNumber when it is not derived from the
	// VAT identifier (BT-31, BT-48) or the legal registration identifier
	// (BT-30, BT-47), as package facturae documents it.
	TaxID string
	// GivenName, FirstSurname and SecondSurname are the Name, FirstSurname
	// and SecondSurname of a natural person (Individual), whose name (BT-27,
	// BT-44) is their concatenation (see SpanishName).
	GivenName     string
	FirstSurname  string
	SecondSurname string
	Registration  *Registration // RegistrationData of a legal person
	Fax           string        // ContactDetails/TeleFax
	Web           string        // ContactDetails/WebAddress
	CNAE          string        // ContactDetails/CnoCnae: activity code
	INETownCode   string        // ContactDetails/INETownCode
	OtherContact  string        // ContactDetails/AdditionalContactDetails
	// Centres are the administrative centres of the party
	// (AdministrativeCentres): for a public buyer, the accounting office
	// (role 01), the managing body (02) and the processing unit (03),
	// identified by their DIR3 codes.
	Centres []AdministrativeCentre
}

// Registration is the registration of a company in the commercial register
// (RegistrationData).
type Registration struct {
	Book       string // Book
	Location   string // RegisterOfCompaniesLocation
	Sheet      string // Sheet
	Folio      string // Folio
	Section    string // Section
	Volume     string // Volume
	Additional string // AdditionalRegistrationData
}

// AdministrativeCentre is an administrative centre of a party
// (AdministrativeCentre).
type AdministrativeCentre struct {
	Code string // CentreCode: a DIR3 code for a public administration
	// Role is the RoleTypeCode: 01 (fiscal; for FACe, the accounting
	// office), 02 (receiver; the managing body), 03 (payer; the processing
	// unit), 04 (buyer) to 09 (issuer).
	Role          string
	Name          string   // Name
	FirstSurname  string   // FirstSurname
	SecondSurname string   // SecondSurname
	Address       Address  // AddressInSpain or OverseasAddress
	Contact       *Contact // ContactDetails: Telephone, ElectronicMail and ContactPersons
	Fax           string   // ContactDetails/TeleFax
	Web           string   // ContactDetails/WebAddress
	CNAE          string   // ContactDetails/CnoCnae
	INETownCode   string   // ContactDetails/INETownCode
	OtherContact  string   // ContactDetails/AdditionalContactDetails
	PhysicalGLN   string   // PhysicalGLN
	LogicalPoint  string   // LogicalOperationalPoint
	Description   string   // CentreDescription
}

// SpainLine holds the Facturae data of an invoice line that EN 16931 does
// not carry.
type SpainLine struct {
	IssuerContract        string // IssuerContractReference
	IssuerContractDate    Date   // IssuerContractDate
	IssuerTransaction     string // IssuerTransactionReference
	IssuerTransactionDate Date   // IssuerTransactionDate
	ReceiverContract      string // ReceiverContractReference
	ReceiverContractDate  Date   // ReceiverContractDate
	// ReceiverTransactionDate is the ReceiverTransactionDate: the date of
	// the buyer's order, whose reference is the order of the line
	// (Line.OrderReference).
	ReceiverTransactionDate Date
	File                    string // FileReference
	FileDate                Date   // FileDate
	// DeliveryNotes are the delivery notes of the line after the first,
	// which is Line.DeliveryNote (DeliveryNotesReferences).
	DeliveryNotes []DocumentReference
	// UnitOfMeasure is the UnitOfMeasure when the unit code (BT-130) does
	// not give it (see UnitOfMeasureOfCode): 05 (other), 14 (Tetra Briks),
	// 27 (6-packs), 29 (portions).
	UnitOfMeasure string
	// TotalCost is the TotalCost when it is not the quantity (BT-129) times
	// the unit price (BT-146).
	TotalCost Decimal
	// VAT holds the Facturae data of the VAT of the line (the first tax of
	// its TaxesOutputs) that the VAT category and rate (BT-151, BT-152) do
	// not give.
	VAT *SpainVAT
	// Taxes are the taxes of TaxesOutputs of the line after its VAT;
	// Withholdings are its taxes withheld (TaxesWithheld).
	Taxes        []SpainTax
	Withholdings []SpainTax
	// SpecialEvent is the SpecialTaxableEventCode when the VAT category of
	// the line does not give it (see SpecialEventOfCategory).
	// SpecialEventReason is the SpecialTaxableEventReason when it is not
	// the exemption reason (BT-120, BT-121) of the category.
	SpecialEvent       string
	SpecialEventReason string
	TransactionDate    Date // TransactionDate
}

// SpainVAT holds the Facturae data of a tax of TaxesOutputs that EN 16931
// does not carry: of the VAT breakdown (VATBreakdown.Spain), of the VAT of a
// line (SpainLine.VAT) or of another tax (SpainTax.Output).
type SpainVAT struct {
	// TaxableBase and TaxAmount are the TaxableBase and the TaxAmount of the
	// VAT of a line, which EN 16931 does not give, when they are not its net
	// amount (BT-131) and the base times the rate (BT-152).
	TaxableBase   Decimal
	TaxAmount     Decimal
	TaxableEuros  Decimal // TaxableBase/EquivalentInEuros
	TaxEuros      Decimal // TaxAmount/EquivalentInEuros
	SpecialBase   Decimal // SpecialTaxableBase/TotalAmount
	SpecialAmount Decimal // SpecialTaxAmount/TotalAmount
	// SurchargeRate and SurchargeAmount are the equivalence surcharge
	// (recargo de equivalencia) of a retailer: EquivalenceSurcharge and
	// EquivalenceSurchargeAmount. The surcharge is part of the total tax
	// outputs (TotalTaxOutputs), not of the VAT total (BT-110).
	SurchargeRate      Decimal
	SurchargeAmount    Decimal
	SpecialBaseEuros   Decimal // SpecialTaxableBase/EquivalentInEuros
	SpecialAmountEuros Decimal // SpecialTaxAmount/EquivalentInEuros
	SurchargeEuros     Decimal // EquivalenceSurchargeAmount/EquivalentInEuros
}

// SpainPayment holds the Facturae data of an installment (Installment) that
// EN 16931 does not carry. Each installment is a payment means of the model,
// whose type code (BT-81) is read from the PaymentMeans (see
// TypeCodeOfSpanishPaymentMeans).
type SpainPayment struct {
	// Means is the PaymentMeans when BT-81 does not give it (see
	// SpanishPaymentMeansOfTypeCode).
	Means string
	// DueDate is the InstallmentDueDate when it is not the payment due date
	// (BT-9).
	DueDate Date
	// Amount is the InstallmentAmount when it is not the amount due for
	// payment (BT-115).
	Amount Decimal
	// CreditBank, CreditBranch and CreditBranchAddress are the BankCode,
	// BranchCode and branch address of the AccountToBeCredited.
	CreditBank          string
	CreditBranch        string
	CreditBranchAddress *Address
	// DebitAccount is the AccountNumber of the AccountToBeDebited, which is
	// not an IBAN (BT-91); DebitBank, DebitBranch and DebitBranchAddress are
	// its BankCode, BranchCode and branch address.
	DebitAccount       string
	DebitBank          string
	DebitBranch        string
	DebitBranchAddress *Address
	// Reconciliation is the PaymentReconciliationReference when it is not
	// the remittance information (BT-83).
	Reconciliation      string
	CollectionInfo      string // CollectionAdditionalInformation
	RegulatoryReporting string // RegulatoryReportingData
	DebitReconciliation string // DebitReconciliationReference
}

// SpainAttachment holds the Facturae data of an attachment (Attachment)
// that EN 16931 does not carry.
type SpainAttachment struct {
	// Compression is the AttachmentCompressionAlgorithm: ZIP, GZIP or NONE.
	// A compressed attachment is kept as written (BT-125).
	Compression string
	// Encoding is the AttachmentEncoding, when the attachment gives it:
	// BASE64, BER, DER or NONE. The attachment (BT-125) is the decoded
	// AttachmentData for BASE64 (and when no encoding is given), its text
	// otherwise.
	Encoding string
	// Format is the AttachmentFormat when the MIME code (BT-125-1) does not
	// give it.
	Format string
}

// Invoice classes (InvoiceClass).
const (
	SpanishOriginal   = "OO" // original invoice
	SpanishCorrective = "OR" // corrective invoice (factura rectificativa)
)

// SpanishClassOfTypeCode returns the Facturae invoice class (InvoiceClass)
// of an invoice type code (BT-3): OR (corrective invoice) for a corrected
// invoice (384), a corrective self-billed invoice (471) and a credit note
// (381 and the other credit note codes), OO (original) otherwise.
func SpanishClassOfTypeCode(code string) string {
	if code = strings.TrimSpace(code); code == "384" || code == "471" || IsCreditNoteType(code) {
		return SpanishCorrective
	}
	return SpanishOriginal
}

// IsCreditNoteType reports whether an invoice type code (BT-3) is a credit
// note code of UNTDID 1001: 381 and the codes that EN 16931 writes as UBL
// CreditNote documents.
func IsCreditNoteType(code string) bool {
	switch strings.TrimSpace(code) {
	case "381", "81", "83", "261", "262", "296", "308", "396", "420", "458", "532":
		return true
	}
	return false
}

// correctiveReasons gives the ReasonDescription of each ReasonCode, as the
// Facturae schemas enumerate them.
var correctiveReasons = map[string]string{
	"01": "Número de la factura",
	"02": "Serie de la factura",
	"03": "Fecha expedición",
	"04": "Nombre y apellidos/Razón Social-Emisor",
	"05": "Nombre y apellidos/Razón Social-Receptor",
	"06": "Identificación fiscal Emisor/obligado",
	"07": "Identificación fiscal Receptor",
	"08": "Domicilio Emisor/Obligado",
	"09": "Domicilio Receptor",
	"10": "Detalle Operación",
	"11": "Porcentaje impositivo a aplicar",
	"12": "Cuota tributaria a aplicar",
	"13": "Fecha/Periodo a aplicar",
	"14": "Clase de factura",
	"15": "Literales legales",
	"16": "Base imponible",
	"80": "Cálculo de cuotas repercutidas",
	"81": "Cálculo de cuotas retenidas",
	"82": "Base imponible modificada por devolución de envases / embalajes",
	"83": "Base imponible modificada por descuentos y bonificaciones",
	"84": "Base imponible modificada por resolución firme, judicial o administrativa",
	"85": "Base imponible modificada cuotas repercutidas no satisfechas. Auto de declaración de concurso",
}

// CorrectiveReason returns the ReasonDescription of a Facturae ReasonCode
// (01 to 16, 80 to 85), or "" for an unknown code.
func CorrectiveReason(code string) string { return correctiveReasons[code] }

// correctionMethods gives the CorrectionMethodDescription of each
// CorrectionMethod.
var correctionMethods = map[string]string{
	"01": "Rectificación íntegra",
	"02": "Rectificación por diferencias",
	"03": "Rectificación por descuento por volumen de operaciones durante un periodo",
	"04": "Autorizadas por la Agencia Tributaria",
}

// CorrectionMethodName returns the CorrectionMethodDescription of a Facturae
// CorrectionMethod (01 to 04), or "" for an unknown method.
func CorrectionMethodName(method string) string { return correctionMethods[method] }

// Tax types (TaxTypeCode) of the VAT breakdown, and of the personal income
// tax.
const (
	SpanishVAT  = "01" // IVA: value added tax
	SpanishIPSI = "02" // IPSI: tax on production, services and imports of Ceuta and Melilla
	SpanishIGIC = "03" // IGIC: Canary Islands general indirect tax
	SpanishIRPF = "04" // IRPF: personal income tax, withheld
)

// IsSpanishVAT reports whether a Facturae tax type (TaxTypeCode) is in the
// VAT breakdown of the model: 01 (IVA), 02 (IPSI) or 03 (IGIC).
func IsSpanishVAT(taxType string) bool {
	return taxType == SpanishVAT || taxType == SpanishIPSI || taxType == SpanishIGIC
}

// SpanishTaxTypeOfCategory returns the Facturae tax type (TaxTypeCode) of a
// VAT category code (BT-118, BT-151): 03 (IGIC) for L, 02 (IPSI) for M, 01
// (IVA) for the others.
func SpanishTaxTypeOfCategory(category string) string {
	switch strings.TrimSpace(category) {
	case VATCanaryIslands:
		return SpanishIGIC
	case VATCeutaAndMelilla:
		return SpanishIPSI
	}
	return SpanishVAT
}

// CategoryOfSpanishTax returns the VAT category code (BT-118, BT-151) of a
// Facturae tax of type 01 (IVA), 02 (IPSI) or 03 (IGIC) with the given rate
// and special taxable event (SpecialTaxableEventCode): L for IGIC and M for
// IPSI; for IVA, S when the rate is not zero, E for an exempt operation
// (01), O for an operation not subject to VAT (02), Z otherwise. It returns
// "" for the other tax types, which are not VAT.
func CategoryOfSpanishTax(taxType string, rate Decimal, event string) string {
	switch taxType {
	case SpanishIGIC:
		return VATCanaryIslands
	case SpanishIPSI:
		return VATCeutaAndMelilla
	case SpanishVAT:
	default:
		return ""
	}
	if rate.IsSet() && !rate.IsZero() {
		return VATStandard
	}
	switch event {
	case "01":
		return VATExempt
	case "02":
		return VATOutOfScope
	}
	return VATZeroRated
}

// SpecialEventOfCategory returns the Facturae special taxable event
// (SpecialTaxableEventCode) of a VAT category: 01 (taxable and exempt) for
// E, K (intra-community supply) and G (export), 02 (not subject) for O and
// AE (reverse charge, where the seller charges no VAT), and "" for the other
// categories.
func SpecialEventOfCategory(category string) string {
	switch strings.TrimSpace(category) {
	case VATExempt, VATIntraCommunity, VATExport:
		return "01"
	case VATOutOfScope, VATReverseCharge:
		return "02"
	}
	return ""
}

// unitsOfMeasure gives, for each Facturae UnitOfMeasure, the unit code of
// UN/ECE recommendations 20 and 21 (BT-130) that the Facturae schemas give
// it, with the prefix X that EN 16931 puts before the packaging codes of
// recommendation 21; 01 (units) is C62 and 18 (dozens) DZN. The units 05
// (other), 14 (Tetra Briks), 27 (6-packs) and 29 (portions) have none.
var unitsOfMeasure = map[string]string{
	"01": "C62", "02": "HUR", "03": "KGM", "04": "LTR", "06": "XBX", "07": "XDS", "08": "XBA",
	"09": "XJY", "10": "XBG", "11": "XCO", "12": "XBO", "13": "XCI", "15": "CLT", "16": "CMT",
	"17": "XBI", "18": "DZN", "19": "XCS", "20": "XDJ", "21": "GRM", "22": "KMT", "23": "XCA",
	"24": "XBH", "25": "MTR", "26": "MMT", "28": "XPK", "30": "XRO", "31": "XEN", "32": "XTB",
	"33": "MTQ", "34": "SEC", "35": "WTT", "36": "KWH",
}

// UnitCodeOfUnitOfMeasure returns the unit code (BT-130) of a Facturae
// UnitOfMeasure (01…), or "" when the unit has none (see unitsOfMeasure) or
// is unknown.
func UnitCodeOfUnitOfMeasure(unit string) string { return unitsOfMeasure[unit] }

// UnitOfMeasureOfCode returns the Facturae UnitOfMeasure of a unit code
// (BT-130), or "" when Facturae has no unit for it.
func UnitOfMeasureOfCode(code string) string {
	code = strings.TrimSpace(code)
	for u, c := range unitsOfMeasure {
		if c == code {
			return u
		}
	}
	return ""
}

// spanishPaymentMeans gives, for each Facturae PaymentMeans, the payment
// means type code (BT-81, UNTDID 4461) read from it.
var spanishPaymentMeans = map[string]string{
	"01": "10",  // in cash
	"02": "49",  // direct debit
	"03": "1",   // receipt: instrument not defined
	"04": "30",  // credit transfer
	"05": "70",  // accepted bill of exchange: bill drawn by the creditor on the debtor
	"06": "1",   // documentary credit
	"07": "1",   // contract award
	"08": "21",  // bill of exchange: banker's draft
	"09": "60",  // transferable promissory note: promissory note
	"10": "61",  // non transferable promissory note: promissory note signed by the debtor
	"11": "20",  // cheque
	"12": "1",   // open account reimbursement
	"13": "ZZZ", // special payment: mutually defined
	"14": "97",  // set-off by reciprocal credits: clearing between partners
	"15": "1",   // payment by postgiro
	"16": "25",  // certified cheque
	"17": "1",   // banker's draft
	"18": "1",   // cash on delivery
	"19": "48",  // payment by card
}

// TypeCodeOfSpanishPaymentMeans returns the payment means type code (BT-81)
// read from a Facturae PaymentMeans (01 to 19): 10 for cash, 49 for a direct
// debit, 30 for a credit transfer, 20 for a cheque, 48 for a card…, 1
// (instrument not defined) for the means that UNTDID 4461 does not name. It
// returns "" for an unknown means.
func TypeCodeOfSpanishPaymentMeans(means string) string { return spanishPaymentMeans[means] }

// SpanishPaymentMeansOfTypeCode returns the Facturae PaymentMeans of a
// payment means type code (BT-81): 04 (credit transfer) for 30, 31, 42 and
// 58, 02 (direct debit) for 49 and 59, 19 (card) for 48, 54 and 55, the means
// whose type code it is for 10, 20, 21, 25, 60, 61, 70, 97 and ZZZ, and 13
// (special payment) otherwise.
func SpanishPaymentMeansOfTypeCode(code string) string {
	switch code = strings.TrimSpace(code); code {
	case "30", "31", "42", "58":
		return "04"
	case "49", "59":
		return "02"
	case "48", "54", "55":
		return "19"
	case "10":
		return "01"
	case "20":
		return "11"
	case "21":
		return "08"
	case "25":
		return "16"
	case "60":
		return "09"
	case "61":
		return "10"
	case "70":
		return "05"
	case "97":
		return "14"
	}
	return "13"
}

// SpanishName returns the name of a natural person as Facturae gives it:
// the given name, the first surname and the second surname, separated by a
// space.
func SpanishName(given, first, second string) string {
	var b strings.Builder
	for _, s := range [...]string{given, first, second} {
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}

// nationalSpain appends to out the data of the Spanish blocks of inv, as
// NationalData lists it.
func (inv *Invoice) nationalSpain(out []string) []string {
	add := func(prefix string, v any) {
		out = appendSet(out, "EXT "+prefix+".", reflect.ValueOf(v).Elem())
	}
	if inv.Spain != nil {
		add("Spain", inv.Spain)
	}
	for _, p := range []struct {
		role  string
		party *Party
	}{
		{"Seller", &inv.Seller}, {"Buyer", &inv.Buyer}, {"SellerTaxRepresentative", inv.SellerTaxRepresentative},
		{"Payee", inv.Payee}, {"ShipTo", inv.ShipTo},
	} {
		if p.party != nil && p.party.Spain != nil {
			add(p.role+".Spain", p.party.Spain)
		}
	}
	for i := range inv.Lines {
		if s := inv.Lines[i].Spain; s != nil {
			add("Lines["+strconv.Itoa(i)+"].Spain", s)
		}
	}
	for i := range inv.VATBreakdown {
		if s := inv.VATBreakdown[i].Spain; s != nil {
			add("VATBreakdown["+strconv.Itoa(i)+"].Spain", s)
		}
	}
	for i := range inv.PaymentMeans {
		if s := inv.PaymentMeans[i].Spain; s != nil {
			add("PaymentMeans["+strconv.Itoa(i)+"].Spain", s)
		}
	}
	for i := range inv.AdditionalDocuments {
		if s := inv.AdditionalDocuments[i].Spain; s != nil {
			add("AdditionalDocuments["+strconv.Itoa(i)+"].Spain", s)
		}
	}
	return out
}
