package qrbill

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/invoice"
)

// S1 is the billing information of line 32 in the syntax S1 of SIX
// (formerly Swico), version 1.2, as Annex D of the guidelines reproduces
// it (table 28): //S1, then each tag and its value. The comment of each
// field gives its tag; a field left empty is left out.
type S1 struct {
	InvoiceNumber     string       // /10/: number of the invoice
	InvoiceDate       invoice.Date // /11/: date of the invoice
	CustomerReference string       // /20/: reference of the customer
	// VATNumber is /30/, the UID of the invoice issuer without CHE, its
	// separators and its suffix MWST, TVA, IVA or VAT: 106017086 for
	// CHE-106.017.086 MWST.
	VATNumber string
	// VATDate and VATEndDate are /31/, the date of the service, or the
	// first and last days of the service.
	VATDate, VATEndDate invoice.Date
	// VATRates are /32/: a single rate without amount for the whole
	// amount of the invoice, or each rate with its net amount.
	VATRates []VATRate
	// ImportTax is /33/: each rate with the amount of pure VAT on goods
	// that are imported.
	ImportTax []VATRate
	// Conditions are /40/: each discount with its period in days from
	// the date of the invoice, 0:30 for payable within 30 days.
	Conditions []Discount
}

// VATRate is a rate of VAT in percent and an amount, unset for a rate
// that applies to the whole amount of the invoice.
type VATRate struct {
	Rate, Amount invoice.Decimal
}

// Discount is a condition of payment: a discount in percent if paid
// within a number of days.
type Discount struct {
	Percent invoice.Decimal
	Days    int
}

// The tags of the syntax S1, in the ascending order of the line.
const (
	tagInvoiceNumber = 10
	tagInvoiceDate   = 11
	tagCustomerRef   = 20
	tagVATNumber     = 30
	tagVATDate       = 31
	tagVATRates      = 32
	tagImportTax     = 33
	tagConditions    = 40
)

// String returns the billing information: //S1, then the tags that have a
// value, in ascending order, each value escaped (/ as \/, \ as \\). The
// dates are written YYMMDD, the numbers with a decimal point and without
// trailing zeros, lists separated by semicolons.
func (s *S1) String() string {
	var b strings.Builder
	b.WriteString("//S1")
	add := func(tag int, v string) {
		if v != "" {
			fmt.Fprintf(&b, "/%d/%s", tag, v)
		}
	}
	add(tagInvoiceNumber, escapeS1(s.InvoiceNumber))
	add(tagInvoiceDate, yymmdd(s.InvoiceDate))
	add(tagCustomerRef, escapeS1(s.CustomerReference))
	add(tagVATNumber, escapeS1(s.VATNumber))
	add(tagVATDate, yymmdd(s.VATDate)+yymmdd(s.VATEndDate))
	add(tagVATRates, rateList(s.VATRates))
	add(tagImportTax, rateList(s.ImportTax))
	conds := make([]string, len(s.Conditions))
	for i, d := range s.Conditions {
		conds[i] = number(d.Percent) + ":" + strconv.Itoa(d.Days)
	}
	add(tagConditions, strings.Join(conds, ";"))
	return b.String()
}

func escapeS1(v string) string {
	return strings.NewReplacer(`\`, `\\`, `/`, `\/`).Replace(v)
}

func yymmdd(d invoice.Date) string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%02d%02d%02d", d.Year()%100, d.Month(), d.Day())
}

func rateList(rates []VATRate) string {
	parts := make([]string, len(rates))
	for i, r := range rates {
		parts[i] = number(r.Rate)
		if r.Amount.IsSet() {
			parts[i] += ":" + number(r.Amount)
		}
	}
	return strings.Join(parts, ";")
}

// number writes d with a decimal point, without trailing zeros: 7.7, 8,
// 0.3.
func number(d invoice.Decimal) string {
	v := d.String()
	if strings.Contains(v, ".") {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	return v
}

// ParseS1 reads billing information in the syntax S1 and checks it against
// the rules of Annex D: the prefix //S1, the tags of table 28 in ascending
// order and each once, an unescaped / or \ nowhere else, dates YYMMDD
// (YYMMDDYYMMDD for a period), rates and amounts with a decimal point,
// lists separated by semicolons. A tag without value counts as left out.
// The error joins a *FieldError of line 32 for each finding.
func ParseS1(s string) (*S1, error) {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, &FieldError{Line: lineBillInfo, Element: elements[lineBillInfo], Msg: "S1: " + fmt.Sprintf(format, args...)})
	}
	rest, ok := strings.CutPrefix(s, "//S1")
	if !ok {
		fail("%q does not begin with //S1", s)
		return nil, errors.Join(errs...)
	}
	parts, ok := splitS1(rest)
	if !ok {
		fail("an unescaped \\ at the end of the line: \\ is written \\\\ and / is written \\/ in a value")
	}
	if len(parts) > 0 && parts[0] != "" || len(parts)%2 == 0 {
		fail("%q is not a sequence of /tag/value", rest)
		return nil, errors.Join(errs...)
	}
	s1 := new(S1)
	last := 0
	for i := 1; i+1 < len(parts); i += 2 {
		tag, v := parts[i], parts[i+1]
		n, err := strconv.Atoi(tag)
		if err != nil || len(tag) != 2 {
			fail("%q is not a tag of two digits", tag)
			continue
		}
		if n <= last {
			fail("tag %d comes after tag %d: the tags are in ascending order, each once", n, last)
			continue
		}
		last = n
		if v == "" {
			continue
		}
		switch n {
		case tagInvoiceNumber:
			s1.InvoiceNumber = v
		case tagInvoiceDate:
			s1.InvoiceDate, ok = parseYYMMDD(v)
			if !ok {
				fail("tag 11: %q is not a date YYMMDD", v)
			}
		case tagCustomerRef:
			s1.CustomerReference = v
		case tagVATNumber:
			s1.VATNumber = v
			if len(v) != 9 || !isDigits(v) {
				fail("tag 30: %q is not the nine digits of a UID, without CHE, separators and suffix", v)
			}
		case tagVATDate:
			okStart, okEnd := false, len(v) == 6
			if len(v) == 6 || len(v) == 12 {
				s1.VATDate, okStart = parseYYMMDD(v[:6])
			}
			if len(v) == 12 {
				s1.VATEndDate, okEnd = parseYYMMDD(v[6:])
			}
			if !okStart || !okEnd {
				fail("tag 31: %q is not a date YYMMDD nor a period YYMMDDYYMMDD", v)
			}
		case tagVATRates, tagImportTax:
			rates, ok := parseRates(v, n == tagImportTax)
			if !ok {
				fail("tag %d: %q is not a list of rate:amount separated by semicolons", n, v)
			}
			if n == tagVATRates {
				s1.VATRates = rates
			} else {
				s1.ImportTax = rates
			}
		case tagConditions:
			for _, c := range strings.Split(v, ";") {
				p, d, ok := strings.Cut(c, ":")
				pct, err1 := invoice.ParseDecimal(p)
				days, err2 := strconv.Atoi(d)
				if !ok || err1 != nil || err2 != nil || !isDigits(d) || pct.Sign() < 0 {
					fail("tag 40: %q is not a discount:days", c)
					continue
				}
				s1.Conditions = append(s1.Conditions, Discount{Percent: pct, Days: days})
			}
		default:
			fail("tag %d is not a tag of the syntax S1 (10, 11, 20, 30, 31, 32, 33 or 40)", n)
		}
	}
	return s1, errors.Join(errs...)
}

// splitS1 splits s at each unescaped /, and unescapes the parts. It
// reports false for a \ that ends s.
func splitS1(s string) ([]string, bool) {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 == len(s) {
				return append(parts, cur.String()), false
			}
			i++
			cur.WriteByte(s[i])
		case '/':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	return append(parts, cur.String()), true
}

func parseYYMMDD(v string) (invoice.Date, bool) {
	if len(v) != 6 || !isDigits(v) {
		return invoice.Date{}, false
	}
	y, _ := strconv.Atoi(v[:2])
	m, _ := strconv.Atoi(v[2:4])
	d, _ := strconv.Atoi(v[4:])
	date := invoice.NewDate(2000+y, time.Month(m), d)
	return date, date.Valid() && int(date.Month()) == m && date.Day() == d
}

// parseRates reads a list of rates of /32/ or /33/. A rate without amount
// is admitted alone in /32/ only.
func parseRates(v string, amounts bool) ([]VATRate, bool) {
	items := strings.Split(v, ";")
	rates := make([]VATRate, 0, len(items))
	for _, item := range items {
		r, a, hasAmount := strings.Cut(item, ":")
		rate, err := invoice.ParseDecimal(r)
		if err != nil || rate.Sign() < 0 || (amounts || len(items) > 1) && !hasAmount {
			return rates, false
		}
		vr := VATRate{Rate: rate}
		if hasAmount {
			if vr.Amount, err = invoice.ParseDecimal(a); err != nil {
				return rates, false
			}
		}
		rates = append(rates, vr)
	}
	return rates, true
}
