package cdar

import (
	"errors"
	"strconv"
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmlenc"
	"github.com/ritoon/zerechnung/invoice"
)

// MarshalOptions are the options of Marshal.
type MarshalOptions struct {
	// Indent writes one element per line, indented with two spaces.
	Indent bool
}

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// Marshal writes m as a CDAR D22B message, its elements in the order of the
// schema, with the prefixes rsm, ram, udt and qdt.
//
// The dates and times are written in format 204 (CCYYMMDDHHMMSS), the
// invoice date and the dates of the characteristics in format 102
// (CCYYMMDD). The indicator of multiple references (MDT-74) and the
// indicator of change of each characteristic (MDT-209) are always written,
// as the examples of France_RFE do. The aggregates that the schema requires
// are written even when m has no value for them (the profile, MDT-3), and
// left out when m has none at all (the recipients, the statuses): the
// validation of the message (package validate) then reports them missing.
func Marshal(m *Message, opts *MarshalOptions) ([]byte, error) {
	if m == nil {
		return nil, errors.New("cdar: nil message")
	}
	var o MarshalOptions
	if opts != nil {
		o = *opts
	}
	w := &writer{b: make([]byte, 0, 4096), indent: o.Indent}
	w.b = append(w.b, xmlDeclaration...)
	w.b = append(w.b, "<rsm:CrossDomainAcknowledgementAndResponse"...)
	w.attr("xmlns:rsm", Namespace)
	w.attr("xmlns:ram", rules.NSRam)
	w.attr("xmlns:udt", rules.NSUdt)
	w.attr("xmlns:qdt", rules.NSQdt)
	w.b = append(w.b, '>')
	w.depth++

	w.start("rsm:ExchangedDocumentContext")
	if m.Test {
		w.indicator("ram:TestIndicator", "udt:Indicator", true)
	}
	if m.BusinessProcess != "" {
		w.start("ram:BusinessProcessSpecifiedDocumentContextParameter")
		w.leafReq("ram:ID", m.BusinessProcess)
		w.end("ram:BusinessProcessSpecifiedDocumentContextParameter")
	}
	w.start("ram:GuidelineSpecifiedDocumentContextParameter")
	w.leafReq("ram:ID", m.Guideline)
	w.end("ram:GuidelineSpecifiedDocumentContextParameter")
	w.end("rsm:ExchangedDocumentContext")

	w.start("rsm:ExchangedDocument")
	w.leaf("ram:ID", m.ID)
	w.leaf("ram:Name", m.Name)
	w.dateTime("ram:IssueDateTime", m.IssueTime)
	w.party("ram:SenderTradeParty", m.Sender)
	w.party("ram:IssuerTradeParty", m.Issuer)
	for i := range m.Recipients {
		w.party("ram:RecipientTradeParty", &m.Recipients[i])
	}
	w.end("rsm:ExchangedDocument")

	for i := range m.Acknowledgements {
		w.acknowledgement(&m.Acknowledgements[i])
	}
	w.end("rsm:CrossDomainAcknowledgementAndResponse")
	if o.Indent {
		w.b = append(w.b, '\n')
	}
	return w.b, nil
}

// writer appends XML to a byte slice.
type writer struct {
	b      []byte
	depth  int
	indent bool
}

// indentation is sliced to write a newline followed by the indentation of
// the current depth.
const indentation = "\n                                                                "

func (w *writer) newline() {
	if w.indent {
		w.b = append(w.b, indentation[:min(1+2*w.depth, len(indentation))]...)
	}
}

func (w *writer) start(name string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
	w.depth++
}

func (w *writer) end(name string) {
	w.depth--
	w.newline()
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

func (w *writer) attr(name, value string) {
	w.b = append(w.b, ' ')
	w.b = append(w.b, name...)
	w.b = append(w.b, '=', '"')
	w.b = xmlenc.AppendEscaped(w.b, value, true)
	w.b = append(w.b, '"')
}

// leafAttr writes a leaf element with an attribute, left out when empty.
func (w *writer) leafAttr(name, attr, attrValue, value string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
	if attrValue != "" {
		w.attr(attr, attrValue)
	}
	w.b = append(w.b, '>')
	w.b = xmlenc.AppendEscaped(w.b, value, false)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// leafReq writes a leaf element, even when value is empty.
func (w *writer) leafReq(name, value string) { w.leafAttr(name, "", "", value) }

// leaf writes a leaf element when value is not empty.
func (w *writer) leaf(name, value string) {
	if value != "" {
		w.leafReq(name, value)
	}
}

func (w *writer) indicator(name, form string, v bool) {
	w.start(name)
	w.leafReq(form, strconv.FormatBool(v))
	w.end(name)
}

// dateTime writes a date and time of format 204 when t is set.
func (w *writer) dateTime(name string, t time.Time) {
	if t.IsZero() {
		return
	}
	w.start(name)
	w.leafAttr("udt:DateTimeString", "format", "204", t.Format(dateTimeFormat))
	w.end(name)
}

// date writes a date of format 102 when d is set, its element in namespace
// prefix (qdt for a formatted date, udt for a date and time).
func (w *writer) date(name, prefix string, d invoice.Date) {
	if d.IsZero() {
		return
	}
	w.start(name)
	w.leafAttr(prefix+":DateTimeString", "format", "102", d.Format102())
	w.end(name)
}

func (w *writer) id(name string, id invoice.Identifier) {
	w.leafAttr(name, "schemeID", id.Scheme, id.ID)
}

func (w *writer) party(name string, p *Party) {
	if p == nil {
		return
	}
	w.start(name)
	for _, id := range p.IDs {
		w.id("ram:GlobalID", id)
	}
	w.leaf("ram:Name", p.Name)
	w.leaf("ram:RoleCode", p.Role)
	if !p.Address.IsZero() {
		w.start("ram:URIUniversalCommunication")
		w.id("ram:URIID", p.Address)
		w.end("ram:URIUniversalCommunication")
	}
	w.end(name)
}

func (w *writer) acknowledgement(a *Acknowledgement) {
	w.start("rsm:AcknowledgementDocument")
	w.indicator("ram:MultipleReferencesIndicator", "udt:Indicator", a.MultipleReferences)
	w.leaf("ram:TypeCode", a.TypeCode)
	w.dateTime("ram:IssueDateTime", a.IssueTime)
	for i := range a.Invoices {
		w.status(&a.Invoices[i])
	}
	w.end("rsm:AcknowledgementDocument")
}

func (w *writer) status(s *Status) {
	w.start("ram:ReferenceReferencedDocument")
	w.leaf("ram:IssuerAssignedID", s.InvoiceID)
	w.leaf("ram:StatusCode", s.StatusCode)
	w.leaf("ram:TypeCode", s.InvoiceType)
	w.dateTime("ram:ReceiptDateTime", s.ReceiptTime)
	w.leaf("ram:ReferenceTypeCode", s.ReferenceType)
	w.date("ram:FormattedIssueDateTime", "qdt", s.InvoiceDate)
	w.leaf("ram:ProcessConditionCode", s.Code)
	w.leaf("ram:ProcessCondition", s.Label)
	w.party("ram:IssuerTradeParty", s.Seller)
	for i := range s.Recipients {
		w.party("ram:RecipientTradeParty", &s.Recipients[i])
	}
	for i := range s.Details {
		w.detail(&s.Details[i])
	}
	w.end("ram:ReferenceReferencedDocument")
}

func (w *writer) detail(d *Detail) {
	w.start("ram:SpecifiedDocumentStatus")
	w.leaf("ram:ReasonCode", d.ReasonCode)
	w.leaf("ram:Reason", d.Reason)
	w.leaf("ram:ProcessConditionCode", d.Code)
	w.leaf("ram:ProcessCondition", d.Label)
	w.leaf("ram:RequestedActionCode", d.ActionCode)
	w.leaf("ram:RequestedAction", d.Action)
	if d.Sequence > 0 {
		w.leafReq("ram:SequenceNumeric", strconv.Itoa(d.Sequence))
	}
	for _, n := range d.Notes {
		w.start("ram:IncludedNote")
		w.leaf("ram:Content", n.Content)
		w.leaf("ram:SubjectCode", n.Subject)
		w.end("ram:IncludedNote")
	}
	for i := range d.Characteristics {
		w.characteristic(&d.Characteristics[i])
	}
	w.end("ram:SpecifiedDocumentStatus")
}

func (w *writer) characteristic(c *Characteristic) {
	w.start("ram:SpecifiedDocumentCharacteristic")
	w.leaf("ram:ID", c.ID)
	w.leaf("ram:TypeCode", c.TypeCode)
	w.indicator("ram:ValueChangedIndicator", "udt:IndicatorString", c.Changed)
	w.leaf("ram:Name", c.Name)
	w.leaf("ram:Location", c.Location)
	if c.Amount.IsSet() {
		w.leafAttr("ram:ValueAmount", "currencyID", c.Currency, c.Amount.String())
	}
	w.date("ram:ValueDateTime", "udt", c.Date)
	if c.Percent.IsSet() {
		w.leafReq("ram:ValuePercent", c.Percent.String())
	}
	w.end("ram:SpecifiedDocumentCharacteristic")
}
