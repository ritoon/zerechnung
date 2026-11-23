package ereporting

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// ErrNotReport is wrapped by the error of Parse when the root element of the
// document is not Report in no namespace.
var ErrNotReport = errors.New("ereporting: not an e-reporting report (root element Report in no namespace)")

// Issue is a part of a report that Parse could not read: a value that does
// not have the format of its element, or an element or attribute that the
// schema does not define.
type Issue struct {
	Path    string // location of the element, as given by xmltree.Node.Path
	Line    int    // line of the element in the document, 1-based
	Message string
	// Undefined is set when the part is not defined by the schema: an
	// element, an attribute, text, or an element repeated more often than
	// the schema allows. It is unset for a value that cannot be read.
	Undefined bool
}

// String formats the issue as "line <Line>: <Path>: <Message>".
func (i Issue) String() string {
	return "line " + strconv.Itoa(i.Line) + ": " + i.Path + ": " + i.Message
}

// IssuesError is the error returned by Parse, together with the report, when
// some parts of the document could not be read. Everything else was read: a
// value that cannot be read leaves its field empty.
type IssuesError struct {
	Issues []Issue
}

// Error gives the number of issues and the first one.
func (e *IssuesError) Error() string {
	if len(e.Issues) == 1 {
		return "ereporting: unread part at " + e.Issues[0].String()
	}
	return "ereporting: " + strconv.Itoa(len(e.Issues)) + " unread parts, first at " + e.Issues[0].String()
}

// IsReport reports whether root is the root element of an e-reporting
// report: Report, in no namespace.
func IsReport(root *xmltree.Node) bool {
	return root != nil && root.Name == rootName && root.Space == ""
}

// rootName is the local name of the root element of a report.
const rootName = "Report"

// Parse reads an e-reporting report.
//
// It fails, with a nil report, only when data is not well-formed XML or its
// root is not Report in no namespace (the error then wraps ErrNotReport).
// When some values cannot be read, or the document holds elements or
// attributes that the schema does not define, Parse returns the report with
// the corresponding fields empty and an *IssuesError listing them.
//
// Reading is tolerant: elements are recognized in any order, values are
// trimmed, and a single element that occurs more than once is read once,
// the others being reported. Parse does not check the structure of the
// report against its schema, nor its values against the rules of the PPF:
// validate.XML checks the structure. The dates are read in the format
// AAAAMMJJ, the time of the transmission (TT-3) in the format
// AAAAMMJJHHMMSS, the amounts, rates and quantities as xs:decimal, keeping
// their decimals.
func Parse(data []byte) (*Report, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("ereporting: %w", err)
	}
	if !IsReport(root) {
		return nil, notReport(root)
	}
	rep, issues := ParseTree(root)
	if len(issues) > 0 {
		return rep, &IssuesError{Issues: issues}
	}
	return rep, nil
}

// ParseTree reads an e-reporting report from an already parsed tree (see
// Parse). If root is not a report, it returns no report and a single issue
// that says why.
func ParseTree(root *xmltree.Node) (*Report, []Issue) {
	if !IsReport(root) {
		is := Issue{Message: notReport(root).Error(), Undefined: true}
		if root != nil {
			is.Path, is.Line = root.Path(), root.Line
		}
		return nil, []Issue{is}
	}
	var r reader
	rep := &Report{}
	r.fields(root, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ReportDocument":
			r.document(c, &rep.Document)
		case "TransactionsReport":
			rep.Transactions = r.transactions(c)
		case "PaymentsReport":
			rep.Payments = r.payments(c)
		default:
			return false
		}
		return true
	})
	return rep, r.issues
}

// notReport returns the error of a root element that is not Report in no
// namespace.
func notReport(root *xmltree.Node) error {
	if root == nil {
		return ErrNotReport
	}
	if root.Space != "" {
		return fmt.Errorf("%w: the root element is %s in namespace %q", ErrNotReport, root.Name, root.Space)
	}
	return fmt.Errorf("%w: the root element is %s", ErrNotReport, root.Name)
}

// reader reads a report and collects the issues.
type reader struct {
	issues []Issue
}

// invalid reports the value of n, which cannot be read.
func (r *reader) invalid(n *xmltree.Node, msg string) {
	r.issues = append(r.issues, Issue{Path: n.Path(), Line: n.Line, Message: msg})
}

// undefined reports a part of n that the schema does not define.
func (r *reader) undefined(n *xmltree.Node, msg string) {
	r.issues = append(r.issues, Issue{Path: n.Path(), Line: n.Line, Message: msg, Undefined: true})
}

// fields reads the child elements of n with f (see elems), and reports the
// attributes of n, which has none.
func (r *reader) fields(n *xmltree.Node, f func(c *xmltree.Node) bool, many ...string) {
	r.attrs(n)
	r.elems(n, f, many...)
}

// elems reads the child elements of n with f, which returns false for an
// element it does not know. An element that f does not know or that is in a
// namespace, and the second occurrence of an element that is not one of the
// repeatable ones (many), are reported and not read. So is the text of n,
// which an element made of elements does not have.
func (r *reader) elems(n *xmltree.Node, f func(c *xmltree.Node) bool, many ...string) {
	if strings.TrimSpace(n.Text) != "" {
		r.undefined(n, n.Name+" holds elements, not text: its text is not read")
	}
	var seen []string
	for _, c := range n.Children {
		if c.Space != "" {
			r.undefined(c, "element "+c.Name+" in namespace "+strconv.Quote(c.Space)+" is not an element of the e-reporting schema: it is not read")
			continue
		}
		if !slices.Contains(many, c.Name) {
			if slices.Contains(seen, c.Name) {
				r.undefined(c, c.Name+" occurs more than once in "+n.Name+", where the schema allows it once: only the first is read")
				continue
			}
			seen = append(seen, c.Name)
		}
		if !f(c) {
			r.undefined(c, c.Name+" is not an element of "+n.Name+" in the e-reporting schema: it is not read")
		}
	}
}

// inner reads n, an element that holds the single element name, with f.
func (r *reader) inner(n *xmltree.Node, name string, f func(c *xmltree.Node)) {
	r.fields(n, func(c *xmltree.Node) bool {
		if c.Name != name {
			return false
		}
		f(c)
		return true
	})
}

// attrs reports the attributes of n that are not in allowed. The attributes
// of the XML Schema instance (xsi:schemaLocation…), recognized by their
// conventional prefix, are allowed everywhere.
func (r *reader) attrs(n *xmltree.Node, allowed ...string) {
	for _, a := range n.Attrs {
		switch {
		case a.Prefix == "xsi":
		case a.Prefix != "":
			r.undefined(n, "attribute "+a.Prefix+":"+a.Name+" of "+n.Name+" is not defined by the e-reporting schema: it is not read")
		case !slices.Contains(allowed, a.Name):
			r.undefined(n, "attribute "+a.Name+" of "+n.Name+" is not defined by the e-reporting schema: it is not read")
		}
	}
}

// attr returns the trimmed value of the attribute name of n.
func attr(n *xmltree.Node, name string) string { return strings.TrimSpace(n.Attr(name)) }

// text returns the trimmed text of the leaf n. Its attributes other than
// allowed and its child elements are reported.
func (r *reader) text(n *xmltree.Node, allowed ...string) string {
	r.attrs(n, allowed...)
	for _, c := range n.Children {
		r.undefined(c, "element "+c.Name+" in "+n.Name+", which holds a value: it is not read")
	}
	return strings.TrimSpace(n.Text)
}

// date reads a date in the format AAAAMMJJ.
func (r *reader) date(n *xmltree.Node) invoice.Date {
	s := r.text(n)
	if s == "" {
		return invoice.Date{}
	}
	if len(s) != 8 || strings.Trim(s, "0123456789") != "" {
		r.invalid(n, "invalid date "+strconv.Quote(s)+": the format is AAAAMMJJ")
		return invoice.Date{}
	}
	d, err := invoice.ParseDate(s, "102")
	if err != nil {
		r.invalid(n, err.Error())
		return invoice.Date{}
	}
	return d
}

// dateTimeLayout is the format of the time of a transmission (TT-3),
// AAAAMMJJHHMMSS.
const dateTimeLayout = "20060102150405"

// dateTime reads a time in the format AAAAMMJJHHMMSS, in UTC.
func (r *reader) dateTime(n *xmltree.Node) time.Time {
	s := r.text(n)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(dateTimeLayout, s)
	if err != nil || len(s) != len(dateTimeLayout) {
		r.invalid(n, "invalid date and time "+strconv.Quote(s)+": the format is AAAAMMJJHHMMSS")
		return time.Time{}
	}
	return t
}

// decimal reads an amount, a rate or a quantity (xs:decimal), with the
// attributes allowed.
func (r *reader) decimal(n *xmltree.Node, allowed ...string) invoice.Decimal {
	s := r.text(n, allowed...)
	if s == "" {
		return invoice.Decimal{}
	}
	d, err := invoice.ParseDecimal(s)
	if err != nil {
		r.invalid(n, "invalid decimal "+strconv.Quote(s))
		return invoice.Decimal{}
	}
	return d
}

// integer reads a number (xs:integer); nil when it is absent or cannot be
// read.
func (r *reader) integer(n *xmltree.Node) *int64 {
	s := r.text(n)
	if s == "" {
		return nil
	}
	i, err := strconv.ParseInt(strings.TrimPrefix(s, "+"), 10, 64)
	if err != nil {
		r.invalid(n, "invalid integer "+strconv.Quote(s))
		return nil
	}
	return &i
}

// charge reads the attribute ChargeIndicator (xs:boolean) of n.
func (r *reader) charge(n *xmltree.Node) bool {
	switch s := attr(n, "ChargeIndicator"); s {
	case "true", "1":
		return true
	case "false", "0":
	case "":
		if _, ok := n.LookupAttr("ChargeIndicator"); ok {
			r.invalid(n, "invalid ChargeIndicator \"\": true or false expected")
		}
	default:
		r.invalid(n, "invalid ChargeIndicator "+strconv.Quote(s)+": true or false expected")
	}
	return false
}

func (r *reader) document(n *xmltree.Node, d *Document) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Id":
			d.ID = r.text(c)
		case "Name":
			d.Name = r.text(c)
		case "IssueDateTime":
			r.inner(c, "DateTimeString", func(k *xmltree.Node) { d.IssueDateTime = r.dateTime(k) })
		case "TypeCode":
			d.TypeCode = r.text(c)
		case "Sender":
			r.party(c, &d.Sender)
		case "Issuer":
			r.party(c, &d.Issuer)
		default:
			return false
		}
		return true
	})
}

func (r *reader) party(n *xmltree.Node, p *Party) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Id":
			p.ID, p.SchemeID = r.text(c, "schemeId"), attr(c, "schemeId")
		case "Name":
			p.Name = r.text(c)
		case "RoleCode":
			p.RoleCode = r.text(c)
		case "URIUniversalCommunication":
			r.inner(c, "URIID", func(k *xmltree.Node) { p.URI = r.text(k) })
		default:
			return false
		}
		return true
	})
}

func (r *reader) period(n *xmltree.Node) Period {
	var p Period
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "StartDate":
			p.Start = r.date(c)
		case "EndDate":
			p.End = r.date(c)
		default:
			return false
		}
		return true
	})
	return p
}

func (r *reader) transactions(n *xmltree.Node) *TransactionsReport {
	t := &TransactionsReport{}
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ReportPeriod":
			t.Period = r.period(c)
		case "Invoice":
			t.Invoices = append(t.Invoices, r.invoice(c))
		case "Transactions":
			t.Transactions = append(t.Transactions, r.totals(c))
		default:
			return false
		}
		return true
	}, "Invoice", "Transactions")
	return t
}

func (r *reader) invoice(n *xmltree.Node) Invoice {
	var inv Invoice
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ID":
			inv.ID = r.text(c)
		case "IssueDate":
			inv.IssueDate = r.date(c)
		case "TypeCode":
			inv.TypeCode = r.text(c)
		case "CurrencyCode":
			inv.Currency = r.text(c)
		case "DueDate":
			inv.DueDate = r.date(c)
		case "TaxDueDateTypeCode":
			inv.TaxDueDateTypeCode = r.text(c)
		case "IncludedNote":
			inv.Notes = append(inv.Notes, r.note(c, "Subject", "Content"))
		case "BusinessProcess":
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "ID":
					inv.BusinessProcess = r.text(k)
				case "TypeID":
					inv.ProfileID = r.text(k)
				default:
					return false
				}
				return true
			})
		case "ReferencedDocument":
			inv.References = append(inv.References, r.reference(c))
		case "Seller":
			inv.Seller = r.company(c)
		case "Buyer":
			b := r.company(c)
			inv.Buyer = &b
		case "SellerTaxRepresentative":
			tr := &TaxRepresentative{}
			r.inner(c, "TaxRegistrationId", func(k *xmltree.Node) {
				tr.TaxID, tr.TaxIDScheme = r.text(k, "schemeId"), attr(k, "schemeId")
			})
			inv.TaxRepresentative = tr
		case "Delivery":
			inv.Deliveries = append(inv.Deliveries, r.delivery(c))
		case "InvoicePeriod":
			inv.Period = r.period(c)
		case "AllowanceCharge":
			inv.AllowanceCharges = append(inv.AllowanceCharges, r.allowanceCharge(c))
		case "MonetaryTotal":
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "TaxExclusiveAmount":
					inv.TaxExclusiveAmount = r.decimal(k)
				case "TaxAmount":
					inv.TaxAmount, inv.TaxCurrency = r.decimal(k, "CurrencyCode"), attr(k, "CurrencyCode")
				default:
					return false
				}
				return true
			})
		case "TaxSubTotal":
			inv.TaxSubtotals = append(inv.TaxSubtotals, r.taxSubtotal(c))
		case "Line":
			inv.Lines = append(inv.Lines, r.line(c))
		default:
			return false
		}
		return true
	}, "IncludedNote", "ReferencedDocument", "Delivery", "AllowanceCharge", "TaxSubTotal", "Line")
	return inv
}

// note reads a note whose code and text are the elements code and text.
func (r *reader) note(n *xmltree.Node, code, text string) Note {
	var no Note
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case code:
			no.Code = r.text(c)
		case text:
			no.Text = r.text(c)
		default:
			return false
		}
		return true
	})
	return no
}

func (r *reader) reference(n *xmltree.Node) Reference {
	var ref Reference
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ID":
			ref.ID = r.text(c)
		case "IssueDate":
			ref.IssueDate = r.date(c)
		default:
			return false
		}
		return true
	})
	return ref
}

func (r *reader) company(n *xmltree.Node) Company {
	var co Company
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "CompanyId":
			co.ID, co.IDScheme = r.text(c, "schemeId"), attr(c, "schemeId")
		case "TaxRegistrationId":
			co.TaxID, co.TaxIDQualifier = r.text(c, "qualifyingId"), attr(c, "qualifyingId")
		case "PostalAddress":
			r.inner(c, "CountryId", func(k *xmltree.Node) { co.Country = r.text(k) })
		default:
			return false
		}
		return true
	})
	return co
}

func (r *reader) delivery(n *xmltree.Node) Delivery {
	var d Delivery
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Date":
			d.Date = r.date(c)
		case "Location":
			a := &Address{}
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "LineOne":
					a.Line1 = r.text(k)
				case "LineTwo":
					a.Line2 = r.text(k)
				case "LineThree":
					a.Line3 = r.text(k)
				case "CityName":
					a.City = r.text(k)
				case "PostalZone":
					a.PostalCode = r.text(k)
				case "CountrySubentity":
					a.Subdivision = r.text(k)
				case "CountryId":
					a.Country = r.text(k)
				default:
					return false
				}
				return true
			})
			d.Location = a
		default:
			return false
		}
		return true
	})
	return d
}

func (r *reader) allowanceCharge(n *xmltree.Node) AllowanceCharge {
	ac := AllowanceCharge{Charge: r.charge(n)}
	r.attrs(n, "ChargeIndicator")
	r.elems(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Amount":
			ac.Amount = r.decimal(c)
		case "TaxCategoryCode":
			ac.TaxCategory = r.text(c)
		case "TaxPercent":
			ac.TaxRate = r.decimal(c)
		default:
			return false
		}
		return true
	})
	return ac
}

func (r *reader) taxSubtotal(n *xmltree.Node) TaxSubtotal {
	var ts TaxSubtotal
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TaxableAmount":
			ts.TaxableAmount = r.decimal(c)
		case "TaxAmount":
			ts.TaxAmount = r.decimal(c)
		case "TaxCategory":
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "Code":
					ts.Category = r.text(k)
				case "Percent":
					ts.Rate = r.decimal(k)
				case "TaxExemptionReason":
					ts.ExemptionReason = r.text(k)
				case "TaxExemptionReasonCode":
					ts.ExemptionReasonCode = r.text(k)
				default:
					return false
				}
				return true
			})
		default:
			return false
		}
		return true
	})
	return ts
}

func (r *reader) line(n *xmltree.Node) Line {
	var l Line
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Note":
			l.Notes = append(l.Notes, r.note(c, "Code", "Comment"))
		case "BilledQuantity":
			l.Quantity, l.UnitCode = r.decimal(c, "UnitCode"), attr(c, "UnitCode")
		case "ReferencedDocument":
			ref := r.reference(c)
			l.Reference = &ref
		case "Delivery":
			d := &LineDelivery{}
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "Name":
					d.Name = r.text(k)
				case "Location":
					d.Location = r.lineLocation(k)
				default:
					return false
				}
				return true
			})
			l.Delivery = d
		case "InvoicePeriod":
			l.Period = r.period(c)
		case "AllowanceCharge":
			ac := LineAllowanceCharge{Charge: r.charge(c)}
			r.attrs(c, "ChargeIndicator")
			r.elems(c, func(k *xmltree.Node) bool {
				if k.Name != "Amount" {
					return false
				}
				ac.Amount = r.decimal(k)
				return true
			})
			l.AllowanceCharges = append(l.AllowanceCharges, ac)
		case "Price":
			p := &Price{}
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "PriceAmount":
					p.Net = r.decimal(k)
				case "AllowanceChargeAmount":
					p.Discount = r.decimal(k)
				case "AllowanceChargeBaseAmount":
					p.Gross = r.decimal(k)
				default:
					return false
				}
				return true
			})
			l.Price = p
		case "Product":
			r.inner(c, "Name", func(k *xmltree.Node) { l.ProductName = r.text(k) })
		default:
			return false
		}
		return true
	}, "Note", "AllowanceCharge")
	return l
}

func (r *reader) lineLocation(n *xmltree.Node) *LineLocation {
	loc := &LineLocation{}
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "LineOne":
			loc.Line1 = r.text(c)
		case "CityName":
			loc.City = r.text(c)
		case "PostalZone":
			loc.PostalCode = r.text(c)
		case "CountrySubentity":
			loc.Subdivision = r.text(c)
		case "CountryId":
			loc.Country = r.text(c)
		default:
			return false
		}
		return true
	})
	return loc
}

func (r *reader) totals(n *xmltree.Node) TransactionTotals {
	var t TransactionTotals
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Date":
			t.Date = r.date(c)
		case "TransactionsCurrency":
			t.Currency = r.text(c)
		case "TaxDueDateTypeCode":
			t.TaxDueDateTypeCode = r.text(c)
		case "CategoryCode":
			t.Category = r.text(c)
		case "TaxExclusiveAmount":
			t.TaxExclusiveAmount = r.decimal(c)
		case "TaxTotal":
			t.TaxTotal = r.decimal(c)
		case "TransactionsCount":
			t.Count = r.integer(c)
		case "TaxSubtotal":
			var rt RateTotal
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "TaxPercent":
					rt.Rate = r.decimal(k)
				case "TaxableAmount":
					rt.TaxableAmount = r.decimal(k)
				case "TaxTotal":
					rt.TaxTotal = r.decimal(k)
				default:
					return false
				}
				return true
			})
			t.TaxSubtotals = append(t.TaxSubtotals, rt)
		default:
			return false
		}
		return true
	}, "TaxSubtotal")
	return t
}

func (r *reader) payments(n *xmltree.Node) *PaymentsReport {
	p := &PaymentsReport{}
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ReportPeriod":
			p.Period = r.period(c)
		case "Invoice":
			var ip InvoicePayment
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "InvoiceID":
					ip.ID = r.text(k)
				case "IssueDate":
					ip.IssueDate = r.date(k)
				case "Payment":
					ip.Payment = r.payment(k)
				default:
					return false
				}
				return true
			})
			p.Invoices = append(p.Invoices, ip)
		case "Transactions":
			r.inner(c, "Payment", func(k *xmltree.Node) { p.Transactions = append(p.Transactions, r.payment(k)) })
		default:
			return false
		}
		return true
	}, "Invoice", "Transactions")
	return p
}

func (r *reader) payment(n *xmltree.Node) Payment {
	var p Payment
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Date":
			p.Date = r.date(c)
		case "SubTotals":
			var s PaymentSubtotal
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "TaxPercent":
					s.Rate = r.decimal(k)
				case "CurrencyCode":
					s.Currency = r.text(k)
				case "Amount":
					s.Amount = r.decimal(k)
				default:
					return false
				}
				return true
			})
			p.SubTotals = append(p.SubTotals, s)
		default:
			return false
		}
		return true
	}, "SubTotals")
	return p
}
