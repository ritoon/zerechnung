package cdar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// ErrNotCDAR is returned (wrapped) by Parse when the document is well-formed
// XML but not a CDAR message, for example a CII invoice.
var ErrNotCDAR = errors.New("cdar: not a CDAR message (rsm:CrossDomainAcknowledgementAndResponse)")

// Issue is a value that could not be read (a malformed date, an unknown
// format), or an element that the model does not hold. The corresponding
// field of the message is left empty.
type Issue struct {
	Path    string // location of the element, as given by xmltree.Node.Path
	Line    int    // line of the element in the document, 1-based
	Message string
}

// String formats the issue as "line <Line>: <Path>: <Message>".
func (i Issue) String() string {
	return "line " + strconv.Itoa(i.Line) + ": " + i.Path + ": " + i.Message
}

// IssuesError is the error returned by Parse, together with the message,
// when some values could not be read. Everything else was read: a caller
// that tolerates incomplete data can use the message and inspect Issues
// with errors.As.
type IssuesError struct {
	Issues []Issue
}

// Error gives the number of issues and the first one.
func (e *IssuesError) Error() string {
	if len(e.Issues) == 1 {
		return "cdar: unreadable value at " + e.Issues[0].String()
	}
	return "cdar: " + strconv.Itoa(len(e.Issues)) + " unreadable values, first at " + e.Issues[0].String()
}

// Parse reads a CDAR message.
//
// It fails, with a nil message, only when data is not well-formed XML or its
// root is not rsm:CrossDomainAcknowledgementAndResponse in namespace
// Namespace (the error then wraps ErrNotCDAR). When some values cannot be
// read, or the message holds elements that the model does not, Parse
// returns the message with the corresponding fields empty and an
// *IssuesError listing them.
func Parse(data []byte) (*Message, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("cdar: %w", err)
	}
	if !IsCDAR(root) {
		return nil, fmt.Errorf("%w: the root element is %s in namespace %q", ErrNotCDAR, root.Name, root.Space)
	}
	m, issues := ParseTree(root)
	if len(issues) > 0 {
		return m, &IssuesError{Issues: issues}
	}
	return m, nil
}

// ParseTree reads an already parsed CDAR message, and returns the values
// that could not be read and the elements that the model does not hold. It
// does not check the root element: see IsCDAR.
func ParseTree(root *xmltree.Node) (*Message, []Issue) {
	p := &parser{}
	m := &Message{}
	for _, c := range root.Children {
		switch c.Name {
		case "ExchangedDocumentContext":
			p.context(c, m)
		case "ExchangedDocument":
			p.exchanged(c, m)
		case "AcknowledgementDocument":
			m.Acknowledgements = append(m.Acknowledgements, p.acknowledgement(c))
		default:
			p.unknown(c)
		}
	}
	return m, p.issues
}

// parser collects the issues of a message.
type parser struct {
	issues []Issue
}

func (p *parser) issue(n *xmltree.Node, format string, args ...any) {
	p.issues = append(p.issues, Issue{Path: n.Path(), Line: n.Line, Message: fmt.Sprintf(format, args...)})
}

// unknown reports an element that the model does not hold.
func (p *parser) unknown(n *xmltree.Node) {
	p.issue(n, "element %s is not read", n.Name)
}

// text returns the trimmed text of a leaf element, and reports its
// attributes and child elements, which the model does not hold.
func (p *parser) text(n *xmltree.Node) string {
	for _, a := range n.Attrs {
		p.issue(n, "attribute %s is not read", a.Name)
	}
	p.leaf(n)
	return n.Trim()
}

// leaf reports the child elements of an element read as a value.
func (p *parser) leaf(n *xmltree.Node) {
	for _, c := range n.Children {
		p.unknown(c)
	}
}

// once returns the value of a leaf element that the model holds once, and
// reports a second occurrence.
func (p *parser) once(n *xmltree.Node, field *string) {
	if *field != "" {
		p.issue(n, "repeated element %s: only the first is read", n.Name)
		return
	}
	*field = p.text(n)
}

// id returns the value and the scheme of an identifier (udt:IDType).
func (p *parser) id(n *xmltree.Node) invoice.Identifier {
	p.leaf(n)
	id := invoice.Identifier{ID: n.Trim()}
	for _, a := range n.Attrs {
		if a.Name == "schemeID" {
			id.Scheme = strings.TrimSpace(a.Value)
		} else {
			p.issue(n, "attribute %s is not read", a.Name)
		}
	}
	return id
}

// indicator reads an indicator (udt:IndicatorType): udt:Indicator or
// udt:IndicatorString, true or false.
func (p *parser) indicator(n *xmltree.Node) bool {
	v := false
	for _, c := range n.Children {
		switch c.Name {
		case "Indicator", "IndicatorString":
			s := p.text(c)
			switch s {
			case "true", "1":
				v = true
			case "false", "0":
			default:
				p.issue(c, "invalid indicator %q: true or false expected", s)
			}
		default:
			p.unknown(c)
		}
	}
	return v
}

// dateTimeFormat is the layout of a date and time of format 204.
const dateTimeFormat = "20060102150405"

// dateTime reads a date and time of format 204 (udt:DateTimeType), or an
// xs:dateTime (udt:DateTime).
func (p *parser) dateTime(n *xmltree.Node) time.Time {
	var t time.Time
	for _, c := range n.Children {
		switch c.Name {
		case "DateTimeString":
			s := c.Trim()
			p.leaf(c)
			f := ""
			for _, a := range c.Attrs {
				if a.Name == "format" {
					f = strings.TrimSpace(a.Value)
				} else {
					p.issue(c, "attribute %s is not read", a.Name)
				}
			}
			if f != "204" {
				p.issue(c, "date and time format %q not read: 204 (CCYYMMDDHHMMSS) expected", f)
				continue
			}
			v, err := time.Parse(dateTimeFormat, s)
			if err != nil {
				p.issue(c, "invalid date and time %q: format 204 expects CCYYMMDDHHMMSS", s)
				continue
			}
			t = v
		case "DateTime":
			s := p.text(c)
			v, err := time.Parse(time.RFC3339, s)
			if err != nil {
				if v, err = time.Parse("2006-01-02T15:04:05", s); err != nil {
					p.issue(c, "invalid date and time %q", s)
					continue
				}
			}
			t = v
		default:
			p.unknown(c)
		}
	}
	return t
}

// date reads a date of format 102 (qdt:FormattedDateTimeType, or a
// udt:DateTimeType of that format).
func (p *parser) date(n *xmltree.Node) invoice.Date {
	var d invoice.Date
	for _, c := range n.Children {
		if c.Name != "DateTimeString" {
			p.unknown(c)
			continue
		}
		p.leaf(c)
		f := ""
		for _, a := range c.Attrs {
			if a.Name == "format" {
				f = strings.TrimSpace(a.Value)
			} else {
				p.issue(c, "attribute %s is not read", a.Name)
			}
		}
		if f != "102" {
			p.issue(c, "date format %q not read: 102 (CCYYMMDD) expected", f)
			continue
		}
		v, err := invoice.ParseDate(c.Trim(), f)
		if err != nil {
			p.issue(c, "%v", err)
			continue
		}
		d = v
	}
	return d
}

// decimal reads an amount or a percentage, and the currency of an amount.
func (p *parser) decimal(n *xmltree.Node, currency *string) invoice.Decimal {
	p.leaf(n)
	for _, a := range n.Attrs {
		if a.Name == "currencyID" && currency != nil {
			*currency = strings.TrimSpace(a.Value)
		} else {
			p.issue(n, "attribute %s is not read", a.Name)
		}
	}
	d, err := invoice.ParseDecimal(n.Trim())
	if err != nil {
		p.issue(n, "invalid number %q", n.Trim())
		return invoice.Decimal{}
	}
	return d
}

func (p *parser) context(n *xmltree.Node, m *Message) {
	for _, c := range n.Children {
		switch c.Name {
		case "TestIndicator":
			m.Test = p.indicator(c)
		case "BusinessProcessSpecifiedDocumentContextParameter":
			m.BusinessProcess = p.parameter(c)
		case "GuidelineSpecifiedDocumentContextParameter":
			m.Guideline = p.parameter(c)
		default:
			p.unknown(c)
		}
	}
}

// parameter reads the identifier of a context parameter.
func (p *parser) parameter(n *xmltree.Node) string {
	var v string
	for _, c := range n.Children {
		if c.Name == "ID" {
			p.once(c, &v)
		} else {
			p.unknown(c)
		}
	}
	return v
}

func (p *parser) exchanged(n *xmltree.Node, m *Message) {
	for _, c := range n.Children {
		switch c.Name {
		case "ID":
			p.once(c, &m.ID)
		case "Name":
			p.once(c, &m.Name)
		case "IssueDateTime":
			m.IssueTime = p.dateTime(c)
		case "SenderTradeParty":
			m.Sender = p.party(c)
		case "IssuerTradeParty":
			m.Issuer = p.party(c)
		case "RecipientTradeParty":
			m.Recipients = append(m.Recipients, *p.party(c))
		default:
			p.unknown(c)
		}
	}
}

func (p *parser) party(n *xmltree.Node) *Party {
	pt := &Party{}
	for _, c := range n.Children {
		switch c.Name {
		case "GlobalID":
			pt.IDs = append(pt.IDs, p.id(c))
		case "Name":
			p.once(c, &pt.Name)
		case "RoleCode":
			p.once(c, &pt.Role)
		case "URIUniversalCommunication":
			for _, u := range c.Children {
				if u.Name == "URIID" {
					pt.Address = p.id(u)
				} else {
					p.unknown(u)
				}
			}
		default:
			p.unknown(c)
		}
	}
	return pt
}

func (p *parser) acknowledgement(n *xmltree.Node) Acknowledgement {
	var a Acknowledgement
	for _, c := range n.Children {
		switch c.Name {
		case "MultipleReferencesIndicator":
			a.MultipleReferences = p.indicator(c)
		case "TypeCode":
			p.once(c, &a.TypeCode)
		case "IssueDateTime":
			a.IssueTime = p.dateTime(c)
		case "ReferenceReferencedDocument":
			a.Invoices = append(a.Invoices, p.status(c))
		default:
			p.unknown(c)
		}
	}
	return a
}

func (p *parser) status(n *xmltree.Node) Status {
	var s Status
	for _, c := range n.Children {
		switch c.Name {
		case "IssuerAssignedID":
			p.once(c, &s.InvoiceID)
		case "StatusCode":
			p.once(c, &s.StatusCode)
		case "TypeCode":
			p.once(c, &s.InvoiceType)
		case "ReceiptDateTime":
			s.ReceiptTime = p.dateTime(c)
		case "ReferenceTypeCode":
			p.once(c, &s.ReferenceType)
		case "FormattedIssueDateTime":
			s.InvoiceDate = p.date(c)
		case "ProcessConditionCode":
			p.once(c, &s.Code)
		case "ProcessCondition":
			p.once(c, &s.Label)
		case "IssuerTradeParty":
			s.Seller = p.party(c)
		case "RecipientTradeParty":
			s.Recipients = append(s.Recipients, *p.party(c))
		case "SpecifiedDocumentStatus":
			s.Details = append(s.Details, p.detail(c))
		default:
			p.unknown(c)
		}
	}
	return s
}

func (p *parser) detail(n *xmltree.Node) Detail {
	var d Detail
	for _, c := range n.Children {
		switch c.Name {
		case "ReasonCode":
			p.once(c, &d.ReasonCode)
		case "Reason":
			p.once(c, &d.Reason)
		case "ProcessConditionCode":
			p.once(c, &d.Code)
		case "ProcessCondition":
			p.once(c, &d.Label)
		case "RequestedActionCode":
			p.once(c, &d.ActionCode)
		case "RequestedAction":
			p.once(c, &d.Action)
		case "SequenceNumeric":
			s := p.text(c)
			v, err := strconv.Atoi(s)
			if err != nil || v < 1 {
				p.issue(c, "invalid sequence number %q: a positive integer expected", s)
				continue
			}
			d.Sequence = v
		case "IncludedNote":
			d.Notes = append(d.Notes, p.note(c))
		case "SpecifiedDocumentCharacteristic":
			d.Characteristics = append(d.Characteristics, p.characteristic(c))
		default:
			p.unknown(c)
		}
	}
	return d
}

func (p *parser) note(n *xmltree.Node) Note {
	var nt Note
	for _, c := range n.Children {
		switch c.Name {
		case "Content":
			p.once(c, &nt.Content)
		case "SubjectCode":
			p.once(c, &nt.Subject)
		default:
			p.unknown(c)
		}
	}
	return nt
}

func (p *parser) characteristic(n *xmltree.Node) Characteristic {
	var ch Characteristic
	for _, c := range n.Children {
		switch c.Name {
		case "ID":
			p.once(c, &ch.ID)
		case "TypeCode":
			p.once(c, &ch.TypeCode)
		case "ValueChangedIndicator":
			ch.Changed = p.indicator(c)
		case "Name":
			p.once(c, &ch.Name)
		case "Location":
			p.once(c, &ch.Location)
		case "ValueAmount":
			ch.Amount = p.decimal(c, &ch.Currency)
		case "ValueDateTime":
			ch.Date = p.date(c)
		case "ValuePercent":
			ch.Percent = p.decimal(c, nil)
		default:
			p.unknown(c)
		}
	}
	return ch
}
