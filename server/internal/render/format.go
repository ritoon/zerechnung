package render

import (
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// currencySymbols are the symbols written for the most common currencies;
// the others are written with their ISO 4217 code.
var currencySymbols = map[string]string{
	"EUR": "€", "USD": "$", "GBP": "£", "JPY": "¥",
}

// num formats d with at least minScale decimals and the separators of the
// language. An unset value gives "".
func (l *labels) num(d invoice.Decimal, minScale int) string {
	if !d.IsSet() {
		return ""
	}
	s := d.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	for len(frac) < minScale {
		frac += "0"
	}
	var b strings.Builder
	if neg {
		b.WriteString("-")
	}
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(l.thousands)
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteString(l.decimal)
		b.WriteString(frac)
	}
	return b.String()
}

// amount formats an amount with at least two decimals.
func (l *labels) amount(d invoice.Decimal) string { return l.num(d, 2) }

// money formats an amount with its currency.
func (l *labels) money(d invoice.Decimal, currency string) string {
	if !d.IsSet() {
		return ""
	}
	v := l.amount(d)
	sym, ok := currencySymbols[currency]
	if !ok {
		sym = currency
	}
	if sym == "" {
		return v
	}
	if l.currencyFirst {
		if ok {
			if strings.HasPrefix(v, "-") {
				return "-" + sym + v[1:]
			}
			return sym + v
		}
		return sym + " " + v
	}
	return v + " " + sym
}

// rate formats a VAT rate or a percentage, without trailing zeros.
func (l *labels) rate(d invoice.Decimal) string {
	if !d.IsSet() {
		return ""
	}
	s := l.num(d, 0)
	if strings.Contains(s, l.decimal) {
		s = strings.TrimRight(strings.TrimRight(s, "0"), l.decimal)
	}
	return s + l.percent + "%"
}

// date formats a date, "" for the zero date.
func (l *labels) date(d invoice.Date) string {
	if d.IsZero() {
		return ""
	}
	if l.dateFormat == "" {
		m := int(d.Month())
		if m < 1 || m > 12 {
			return d.String()
		}
		return itoa(d.Day()) + " " + l.months[m-1] + " " + itoa(d.Year())
	}
	return d.Time(nil).Format(l.dateFormat)
}

// periodText formats a period, "" when it has no date.
func (l *labels) periodText(p invoice.Period) string {
	start, end := l.date(p.Start), l.date(p.End)
	switch {
	case start != "" && end != "":
		return start + " – " + end
	case start != "":
		return start + " –"
	case end != "":
		return "– " + end
	}
	return p.Description
}

// unit returns the label of a unit of measure code, the code itself when
// the language has no label for it.
func (l *labels) unit(code string) string {
	if s, ok := l.units[code]; ok {
		return s
	}
	return code
}

// title returns the title of the document for its type code (UNTDID 1001).
func (l *labels) title(typeCode string) string {
	switch typeCode {
	case "381", "261", "262", "296", "308", "396", "420", "458", "532":
		return l.creditNote
	case "384", "471", "472", "473":
		return l.correctedInvoice
	case "386":
		return l.prepaymentInvoice
	case "389", "527":
		return l.selfBilledInvoice
	case "326":
		return l.partialInvoice
	case "383", "80", "84":
		return l.debitNote
	}
	return l.invoice
}

// category returns the label of a VAT category code.
func (l *labels) category(code string) string {
	if s, ok := l.categories[code]; ok {
		return s
	}
	return code
}

// meansLabel returns the label of a payment means code.
func (l *labels) meansLabel(code string) string {
	if s, ok := l.means[code]; ok {
		return s
	}
	return code
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}

// iban groups an IBAN by four characters, as it is printed.
func iban(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}
