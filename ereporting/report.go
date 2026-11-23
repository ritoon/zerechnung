package ereporting

import (
	"time"

	"github.com/ritoon/zerechnung/invoice"
)

// Report is an e-reporting report, the root element Report (TB-0). It holds
// the transmission header and either the transactions (flux 10.1 and 10.3)
// or the payments (flux 10.2 and 10.4); the schema admits both, or neither,
// which the rules of the PPF do not.
type Report struct {
	Document     Document            // TB-1 ReportDocument
	Transactions *TransactionsReport // TB-2 TransactionsReport
	Payments     *PaymentsReport     // TB-3 PaymentsReport
}

// Document is the header of the transmission (TB-1, ReportDocument).
type Document struct {
	ID   string // TT-1 Id
	Name string // TT-2 Name
	// IssueDateTime is the creation time of the transmission (TT-3,
	// IssueDateTime/DateTimeString), written AAAAMMJJHHMMSS. The format has
	// no time zone: the time is read in UTC, and written as the wall clock
	// of its location.
	IssueDateTime time.Time
	TypeCode      string // TT-4 TypeCode: IN (initial) or RE (corrective)
	Sender        Party  // TG-3 Sender: the platform that transmits
	Issuer        Party  // TG-5 Issuer: the declarant
}

// Party is the sender (TG-3) or the declarant (TG-5) of a transmission.
type Party struct {
	ID       string // TT-8, TT-13 Id
	SchemeID string // TT-7, TT-12 Id/@schemeId
	Name     string // TT-9, TT-14 Name
	RoleCode string // TT-10, TT-15 RoleCode
	URI      string // TT-11, TT-16 URIUniversalCommunication/URIID
}

// Period is a period of dates, written AAAAMMJJ.
type Period struct {
	Start invoice.Date // StartDate
	End   invoice.Date // EndDate
}

// IsZero reports whether the period has neither start nor end.
func (p Period) IsZero() bool { return p.Start.IsZero() && p.End.IsZero() }

// TransactionsReport is the report of the transactions (TB-2): the
// invoices of the international business-to-business transactions (flux
// 10.1) and the daily totals of the other transactions (flux 10.3).
type TransactionsReport struct {
	Period       Period              // TG-7 ReportPeriod: TT-17, TT-18
	Invoices     []Invoice           // TG-8 Invoice
	Transactions []TransactionTotals // TG-31 Transactions
}

// Invoice is an invoice of the transactions report (TG-8).
type Invoice struct {
	ID                 string       // TT-19 ID
	IssueDate          invoice.Date // TT-20 IssueDate
	TypeCode           string       // TT-21 TypeCode (UNTDID 1001)
	Currency           string       // TT-22 CurrencyCode
	DueDate            invoice.Date // TT-201 DueDate
	TaxDueDateTypeCode string       // TT-24 TaxDueDateTypeCode
	Notes              []Note       // TG-9 IncludedNote: TT-26 Subject, TT-27 Content
	BusinessProcess    string       // TT-28 BusinessProcess/ID: the billing framework (B1, S1…)
	ProfileID          string       // TT-29 BusinessProcess/TypeID
	References         []Reference  // TG-11 ReferencedDocument: TT-30, TT-31
	Seller             Company      // TG-12 Seller
	Buyer              *Company     // TG-14 Buyer
	// TaxRepresentative is the VAT identifier of the tax representative
	// of the seller (TG-16 SellerTaxRepresentative, TT-122).
	TaxRepresentative *TaxRepresentative
	Deliveries        []Delivery        // TG-17 Delivery
	Period            Period            // TG-18 InvoicePeriod: TT-42, TT-43
	AllowanceCharges  []AllowanceCharge // TG-20, TG-21 AllowanceCharge
	// TaxExclusiveAmount is the total without VAT (TT-51,
	// MonetaryTotal/TaxExclusiveAmount).
	TaxExclusiveAmount invoice.Decimal
	TaxAmount          invoice.Decimal // TT-52 MonetaryTotal/TaxAmount
	TaxCurrency        string          // TT-52 MonetaryTotal/TaxAmount/@CurrencyCode
	TaxSubtotals       []TaxSubtotal   // TG-23 TaxSubTotal
	Lines              []Line          // TG-24 Line
}

// Note is a note of an invoice (TG-9 IncludedNote: Subject and Content) or
// of an invoice line (TG-61 Note: Code and Comment).
type Note struct {
	Code string // TT-26 Subject, TT-61-0 Code
	Text string // TT-27 Content, TT-61-1 Comment
}

// Reference is a preceding invoice (TG-11, and TG-40 for a line).
type Reference struct {
	ID        string       // TT-30, TT-300 ID
	IssueDate invoice.Date // TT-31, TT-301 IssueDate
}

// Company is the seller (TG-12) or the buyer (TG-14) of an invoice.
type Company struct {
	ID       string // TT-33, TT-36 CompanyId
	IDScheme string // CompanyId/@schemeId (0002 SIREN, 0223, 0227…)
	TaxID    string // TT-34, TT-38 TaxRegistrationId
	// TaxIDQualifier is the qualifier of the VAT identifier
	// (TaxRegistrationId/@qualifyingId, VAT).
	TaxIDQualifier string
	Country        string // TT-35, TT-39 PostalAddress/CountryId
}

// TaxRepresentative is the tax representative of the seller (TG-16).
type TaxRepresentative struct {
	TaxID       string // TT-122 TaxRegistrationId
	TaxIDScheme string // TaxRegistrationId/@schemeId
}

// Delivery is a delivery of an invoice (TG-17).
type Delivery struct {
	Date     invoice.Date // TT-41 Date
	Location *Address     // TG-19 Location
}

// Address is the delivery address of an invoice (TG-19).
type Address struct {
	Line1       string // TT-103 LineOne
	Line2       string // TT-104 LineTwo
	Line3       string // TT-105 LineThree
	City        string // TT-106 CityName
	PostalCode  string // TT-107 PostalZone
	Subdivision string // TT-108 CountrySubentity
	Country     string // TT-44 CountryId
}

// AllowanceCharge is an allowance (TG-20) or a charge (TG-21) of an
// invoice.
type AllowanceCharge struct {
	Charge      bool            // @ChargeIndicator
	Amount      invoice.Decimal // TT-45, TT-48 Amount
	TaxCategory string          // TT-46, TT-49 TaxCategoryCode
	TaxRate     invoice.Decimal // TT-47, TT-50 TaxPercent
}

// TaxSubtotal is a line of the VAT breakdown of an invoice (TG-23).
type TaxSubtotal struct {
	TaxableAmount invoice.Decimal // TT-54 TaxableAmount
	TaxAmount     invoice.Decimal // TT-55 TaxAmount
	Category      string          // TT-56 TaxCategory/Code
	Rate          invoice.Decimal // TT-57 TaxCategory/Percent
	// ExemptionReason and ExemptionReasonCode are the reason of the VAT
	// exemption (TT-58 TaxCategory/TaxExemptionReason, TT-59
	// TaxCategory/TaxExemptionReasonCode).
	ExemptionReason     string
	ExemptionReasonCode string
}

// Line is an invoice line (TG-24).
type Line struct {
	Notes            []Note                // TG-61 Note
	Quantity         invoice.Decimal       // TT-62 BilledQuantity
	UnitCode         string                // TT-62 BilledQuantity/@UnitCode
	Reference        *Reference            // TG-40 ReferencedDocument
	Delivery         *LineDelivery         // TG-41 Delivery
	Period           Period                // TG-25 InvoicePeriod: TT-65, TT-66
	AllowanceCharges []LineAllowanceCharge // TG-26, TG-27 AllowanceCharge
	Price            *Price                // TG-28 Price
	ProductName      string                // TT-76 Product/Name
}

// LineDelivery is the delivery of an invoice line (TG-41).
type LineDelivery struct {
	Name     string        // TT-302 Name
	Location *LineLocation // TG-42 Location
}

// LineLocation is the delivery address of an invoice line (TG-42).
type LineLocation struct {
	Line1       string // TT-303 LineOne
	City        string // TT-304 CityName
	PostalCode  string // TT-305 PostalZone
	Subdivision string // TT-306 CountrySubentity
	Country     string // TT-307 CountryId
}

// LineAllowanceCharge is an allowance (TG-26) or a charge (TG-27) of an
// invoice line.
type LineAllowanceCharge struct {
	Charge bool            // @ChargeIndicator
	Amount invoice.Decimal // TT-67, TT-68 Amount
}

// Price is the price of the item of an invoice line (TG-28).
type Price struct {
	Net      invoice.Decimal // TT-69 PriceAmount
	Discount invoice.Decimal // TT-70 AllowanceChargeAmount
	Gross    invoice.Decimal // TT-71 AllowanceChargeBaseAmount
}

// TransactionTotals are the totals of the transactions of one day and one
// category (TG-31 Transactions, flux 10.3).
type TransactionTotals struct {
	Date               invoice.Date    // TT-77 Date
	Currency           string          // TT-78 TransactionsCurrency
	TaxDueDateTypeCode string          // TT-80 TaxDueDateTypeCode
	Category           string          // TT-81 CategoryCode (TLB1, TPS1, TNT1, TMA1)
	TaxExclusiveAmount invoice.Decimal // TT-82 TaxExclusiveAmount
	TaxTotal           invoice.Decimal // TT-83 TaxTotal
	// Count is the number of transactions (TT-85 TransactionsCount), nil
	// when absent.
	Count        *int64
	TaxSubtotals []RateTotal // TG-32 TaxSubtotal
}

// RateTotal is the VAT breakdown of transaction totals for one rate (TG-32).
type RateTotal struct {
	Rate          invoice.Decimal // TT-86 TaxPercent
	TaxableAmount invoice.Decimal // TT-87 TaxableAmount
	TaxTotal      invoice.Decimal // TT-88 TaxTotal
}

// PaymentsReport is the report of the payments (TB-3): the payments of
// invoices (flux 10.2) and of other transactions (flux 10.4).
type PaymentsReport struct {
	Period   Period           // TG-33 ReportPeriod: TT-89, TT-90
	Invoices []InvoicePayment // TG-34 Invoice
	// Transactions are the payments of transactions without invoice (TG-37
	// Transactions, each holding one Payment, TG-38).
	Transactions []Payment
}

// InvoicePayment is the payment of an invoice (TG-34).
type InvoicePayment struct {
	ID        string       // TT-91 InvoiceID
	IssueDate invoice.Date // TT-102 IssueDate
	Payment   Payment      // TG-35 Payment
}

// Payment is a payment, of an invoice (TG-35) or of transactions (TG-38).
type Payment struct {
	Date      invoice.Date      // TT-92, TT-96 Date
	SubTotals []PaymentSubtotal // TG-36, TG-39 SubTotals
}

// PaymentSubtotal is the amount received for one VAT rate (TG-36, TG-39).
type PaymentSubtotal struct {
	Rate     invoice.Decimal // TT-93, TT-97 TaxPercent
	Currency string          // TT-94, TT-98 CurrencyCode
	Amount   invoice.Decimal // TT-95, TT-99 Amount
}
