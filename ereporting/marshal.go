package ereporting

import (
	"errors"
	"strconv"
	"time"

	"github.com/ritoon/zerechnung/internal/xmlenc"
	"github.com/ritoon/zerechnung/invoice"
)

// MarshalOptions are the options of Marshal.
type MarshalOptions struct {
	// Indent writes one element per line, indented with two spaces.
	Indent bool
}

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// Marshal writes rep as an e-reporting report, in the order of the schema
// of France_RFE.
//
// The model follows the schema field by field, so nothing of rep is lost.
// Mandatory elements are always written, empty when rep has no value for
// them, so that a validator reports the missing value; an optional element
// is written when it holds a value, or for a pointer field when it is not
// nil. The dates are written in the format AAAAMMJJ, the time of the
// transmission (TT-3) in the format AAAAMMJJHHMMSS, from the wall clock of
// its location, and the decimals as they were read or built, with their
// decimals. Marshal neither checks the values against the rules of the PPF
// nor derives a value from the others: validate.XML checks the structure of
// the report written.
func Marshal(rep *Report, opts *MarshalOptions) ([]byte, error) {
	if rep == nil {
		return nil, errors.New("ereporting: nil report")
	}
	w := writer{b: make([]byte, 0, 2048), indent: opts != nil && opts.Indent}
	w.b = append(w.b, xmlDeclaration...)
	w.start(rootName)
	w.document(&rep.Document)
	if t := rep.Transactions; t != nil {
		w.start("TransactionsReport")
		w.period("ReportPeriod", t.Period, true)
		for i := range t.Invoices {
			w.invoice(&t.Invoices[i])
		}
		for i := range t.Transactions {
			w.totals(&t.Transactions[i])
		}
		w.end("TransactionsReport")
	}
	if p := rep.Payments; p != nil {
		w.start("PaymentsReport")
		w.period("ReportPeriod", p.Period, true)
		for i := range p.Invoices {
			ip := &p.Invoices[i]
			w.start("Invoice")
			w.leafReq("InvoiceID", ip.ID)
			w.dateReq("IssueDate", ip.IssueDate)
			w.payment(&ip.Payment)
			w.end("Invoice")
		}
		for i := range p.Transactions {
			w.start("Transactions")
			w.payment(&p.Transactions[i])
			w.end("Transactions")
		}
		w.end("PaymentsReport")
	}
	w.end(rootName)
	if w.indent {
		w.b = append(w.b, '\n')
	}
	return w.b, nil
}

// writer writes the elements of a report.
type writer struct {
	b      []byte
	depth  int
	indent bool
}

// indentation is sliced to write a newline followed by the indentation of
// the current depth.
const indentation = "\n                                                  "

func (w *writer) newline() {
	if w.indent && len(w.b) > len(xmlDeclaration) {
		w.b = append(w.b, indentation[:min(1+2*w.depth, len(indentation))]...)
	}
}

// open writes the start of the start tag of name, left open for
// attributes.
func (w *writer) open(name string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
}

// start writes the start tag of an aggregate.
func (w *writer) start(name string) {
	w.open(name)
	w.b = append(w.b, '>')
	w.depth++
}

// end writes the end tag of an aggregate.
func (w *writer) end(name string) {
	w.depth--
	w.newline()
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// attr writes an attribute in an open start tag.
func (w *writer) attr(name, value string) {
	w.b = append(w.b, ' ')
	w.b = append(w.b, name...)
	w.b = append(w.b, '=', '"')
	w.b = xmlenc.AppendEscaped(w.b, value, true)
	w.b = append(w.b, '"')
}

// close ends the start tag opened by open, writes value and the end tag.
func (w *writer) close(name, value string) {
	w.b = append(w.b, '>')
	w.b = xmlenc.AppendEscaped(w.b, value, false)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// leafReq writes a leaf element, even when value is empty.
func (w *writer) leafReq(name, value string) {
	w.open(name)
	w.close(name, value)
}

// leaf writes a leaf element when value is not empty.
func (w *writer) leaf(name, value string) {
	if value != "" {
		w.leafReq(name, value)
	}
}

// leafAttr writes a leaf element with the attribute an, value av; when
// required is false, only if value or av is not empty. The attribute is
// written when av is not empty, or always when attrRequired is set.
func (w *writer) leafAttr(name, value, an, av string, required, attrRequired bool) {
	if !required && value == "" && av == "" {
		return
	}
	w.open(name)
	if av != "" || attrRequired {
		w.attr(an, av)
	}
	w.close(name, value)
}

func (w *writer) dateReq(name string, d invoice.Date) { w.leafReq(name, d.Format102()) }

func (w *writer) date(name string, d invoice.Date) {
	if !d.IsZero() {
		w.dateReq(name, d)
	}
}

func (w *writer) decReq(name string, d invoice.Decimal) { w.leafReq(name, d.String()) }

func (w *writer) dec(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.decReq(name, d)
	}
}

// period writes a period, when it is not empty or required is set.
func (w *writer) period(name string, p Period, required bool) {
	if !required && p.IsZero() {
		return
	}
	w.start(name)
	if required {
		w.dateReq("StartDate", p.Start)
		w.dateReq("EndDate", p.End)
	} else {
		w.date("StartDate", p.Start)
		w.date("EndDate", p.End)
	}
	w.end(name)
}

func (w *writer) document(d *Document) {
	w.start("ReportDocument")
	w.leafReq("Id", d.ID)
	w.leaf("Name", d.Name)
	w.start("IssueDateTime")
	w.leafReq("DateTimeString", formatTime(d.IssueDateTime))
	w.end("IssueDateTime")
	w.leafReq("TypeCode", d.TypeCode)
	w.party("Sender", &d.Sender)
	w.party("Issuer", &d.Issuer)
	w.end("ReportDocument")
}

func (w *writer) party(name string, p *Party) {
	w.start(name)
	w.leafAttr("Id", p.ID, "schemeId", p.SchemeID, true, true)
	w.leafReq("Name", p.Name)
	w.leafReq("RoleCode", p.RoleCode)
	if p.URI != "" {
		w.start("URIUniversalCommunication")
		w.leafReq("URIID", p.URI)
		w.end("URIUniversalCommunication")
	}
	w.end(name)
}

func (w *writer) invoice(inv *Invoice) {
	w.start("Invoice")
	w.leafReq("ID", inv.ID)
	w.dateReq("IssueDate", inv.IssueDate)
	w.leafReq("TypeCode", inv.TypeCode)
	w.leafReq("CurrencyCode", inv.Currency)
	w.date("DueDate", inv.DueDate)
	w.leaf("TaxDueDateTypeCode", inv.TaxDueDateTypeCode)
	for _, n := range inv.Notes {
		w.start("IncludedNote")
		w.leaf("Subject", n.Code)
		w.leaf("Content", n.Text)
		w.end("IncludedNote")
	}
	w.start("BusinessProcess")
	w.leafReq("ID", inv.BusinessProcess)
	w.leafReq("TypeID", inv.ProfileID)
	w.end("BusinessProcess")
	for _, ref := range inv.References {
		w.start("ReferencedDocument")
		w.leafReq("ID", ref.ID)
		w.date("IssueDate", ref.IssueDate)
		w.end("ReferencedDocument")
	}
	w.company("Seller", &inv.Seller, true)
	if inv.Buyer != nil {
		w.company("Buyer", inv.Buyer, false)
	}
	if tr := inv.TaxRepresentative; tr != nil {
		w.start("SellerTaxRepresentative")
		w.leafAttr("TaxRegistrationId", tr.TaxID, "schemeId", tr.TaxIDScheme, true, true)
		w.end("SellerTaxRepresentative")
	}
	for i := range inv.Deliveries {
		d := &inv.Deliveries[i]
		w.start("Delivery")
		w.date("Date", d.Date)
		if a := d.Location; a != nil {
			w.start("Location")
			w.leaf("LineOne", a.Line1)
			w.leaf("LineTwo", a.Line2)
			w.leaf("LineThree", a.Line3)
			w.leaf("CityName", a.City)
			w.leaf("PostalZone", a.PostalCode)
			w.leaf("CountrySubentity", a.Subdivision)
			w.leaf("CountryId", a.Country)
			w.end("Location")
		}
		w.end("Delivery")
	}
	w.period("InvoicePeriod", inv.Period, false)
	for _, ac := range inv.AllowanceCharges {
		w.open("AllowanceCharge")
		w.attr("ChargeIndicator", strconv.FormatBool(ac.Charge))
		w.b = append(w.b, '>')
		w.depth++
		w.dec("Amount", ac.Amount)
		w.leaf("TaxCategoryCode", ac.TaxCategory)
		w.dec("TaxPercent", ac.TaxRate)
		w.end("AllowanceCharge")
	}
	w.start("MonetaryTotal")
	w.dec("TaxExclusiveAmount", inv.TaxExclusiveAmount)
	w.leafAttr("TaxAmount", inv.TaxAmount.String(), "CurrencyCode", inv.TaxCurrency, true, true)
	w.end("MonetaryTotal")
	for _, ts := range inv.TaxSubtotals {
		w.start("TaxSubTotal")
		w.decReq("TaxableAmount", ts.TaxableAmount)
		w.decReq("TaxAmount", ts.TaxAmount)
		w.start("TaxCategory")
		w.leaf("Code", ts.Category)
		w.decReq("Percent", ts.Rate)
		w.leaf("TaxExemptionReason", ts.ExemptionReason)
		w.leaf("TaxExemptionReasonCode", ts.ExemptionReasonCode)
		w.end("TaxCategory")
		w.end("TaxSubTotal")
	}
	for i := range inv.Lines {
		w.line(&inv.Lines[i])
	}
	w.end("Invoice")
}

// company writes the seller (required set: its identifier is required) or
// the buyer.
func (w *writer) company(name string, c *Company, required bool) {
	w.start(name)
	w.leafAttr("CompanyId", c.ID, "schemeId", c.IDScheme, required, true)
	w.leafAttr("TaxRegistrationId", c.TaxID, "qualifyingId", c.TaxIDQualifier, false, true)
	if c.Country != "" {
		w.start("PostalAddress")
		w.leafReq("CountryId", c.Country)
		w.end("PostalAddress")
	}
	w.end(name)
}

func (w *writer) line(l *Line) {
	w.start("Line")
	for _, n := range l.Notes {
		w.start("Note")
		w.leaf("Code", n.Code)
		w.leaf("Comment", n.Text)
		w.end("Note")
	}
	w.leafAttr("BilledQuantity", l.Quantity.String(), "UnitCode", l.UnitCode, false, false)
	if ref := l.Reference; ref != nil {
		w.start("ReferencedDocument")
		w.leaf("ID", ref.ID)
		w.date("IssueDate", ref.IssueDate)
		w.end("ReferencedDocument")
	}
	if d := l.Delivery; d != nil {
		w.start("Delivery")
		w.leaf("Name", d.Name)
		if loc := d.Location; loc != nil {
			w.start("Location")
			w.leaf("LineOne", loc.Line1)
			w.leaf("CityName", loc.City)
			w.leaf("PostalZone", loc.PostalCode)
			w.leaf("CountrySubentity", loc.Subdivision)
			w.leafReq("CountryId", loc.Country)
			w.end("Location")
		}
		w.end("Delivery")
	}
	w.period("InvoicePeriod", l.Period, false)
	for _, ac := range l.AllowanceCharges {
		w.open("AllowanceCharge")
		w.attr("ChargeIndicator", strconv.FormatBool(ac.Charge))
		w.b = append(w.b, '>')
		w.depth++
		w.decReq("Amount", ac.Amount)
		w.end("AllowanceCharge")
	}
	if p := l.Price; p != nil {
		w.start("Price")
		w.dec("PriceAmount", p.Net)
		w.dec("AllowanceChargeAmount", p.Discount)
		w.dec("AllowanceChargeBaseAmount", p.Gross)
		w.end("Price")
	}
	if l.ProductName != "" {
		w.start("Product")
		w.leafReq("Name", l.ProductName)
		w.end("Product")
	}
	w.end("Line")
}

func (w *writer) totals(t *TransactionTotals) {
	w.start("Transactions")
	w.dateReq("Date", t.Date)
	w.leafReq("TransactionsCurrency", t.Currency)
	w.leaf("TaxDueDateTypeCode", t.TaxDueDateTypeCode)
	w.leafReq("CategoryCode", t.Category)
	w.decReq("TaxExclusiveAmount", t.TaxExclusiveAmount)
	w.decReq("TaxTotal", t.TaxTotal)
	if t.Count != nil {
		w.leafReq("TransactionsCount", strconv.FormatInt(*t.Count, 10))
	}
	for _, rt := range t.TaxSubtotals {
		w.start("TaxSubtotal")
		w.decReq("TaxPercent", rt.Rate)
		w.decReq("TaxableAmount", rt.TaxableAmount)
		w.decReq("TaxTotal", rt.TaxTotal)
		w.end("TaxSubtotal")
	}
	w.end("Transactions")
}

func (w *writer) payment(p *Payment) {
	w.start("Payment")
	w.dateReq("Date", p.Date)
	for _, s := range p.SubTotals {
		w.start("SubTotals")
		w.decReq("TaxPercent", s.Rate)
		w.leaf("CurrencyCode", s.Currency)
		w.decReq("Amount", s.Amount)
		w.end("SubTotals")
	}
	w.end("Payment")
}

// formatTime returns t in the format of the time of a transmission, "" for
// the zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateTimeLayout)
}
