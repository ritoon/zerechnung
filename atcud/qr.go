package atcud

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/qrcode"
)

// QR is the content of the QR code of a document, field by field, as the
// table of section 4 of the specification defines them; the comment of
// each field gives its code. A text field left empty and an amount left
// unset are absent: String writes the optional ones only when they are
// present (section 3 f), and Check reports the mandatory ones.
type QR struct {
	IssuerNIF      string       // A: NIF of the issuer, nine digits, without the country prefix
	BuyerTaxID     string       // B: tax identifier of the buyer, without the country prefix; 999999990 for a final consumer
	BuyerCountry   string       // C: country of the buyer, ISO 3166-1 alpha-2, or Desconhecido
	DocumentType   string       // D: type of the document in SAF-T (PT): FT, FS, FR, NC, ND, GT, PF, RC…
	DocumentStatus string       // E: status of the document in SAF-T (PT): N (normal), A (cancelled)…
	Date           invoice.Date // F: date of the document
	DocumentID     string       // G: identification of the document in SAF-T (PT): FT AB2019/0035
	ATCUD          string       // H: the ATCUD, see Code

	// Spaces are the fiscal spaces of the document, I1 to I8, J1 to J8 and
	// K1 to K8: the first is mandatory, the other two optional.
	Spaces [3]Space

	NotSubject  invoice.Decimal // L: not subject to VAT, not taxable, or VAT not shown (margin schemes…)
	StampDuty   invoice.Decimal // M: total of the stamp duty (Imposto do Selo)
	TaxTotal    invoice.Decimal // N: total of the VAT and the stamp duty (TaxPayable)
	GrossTotal  invoice.Decimal // O: total of the document with taxes (GrossTotal)
	Withholding invoice.Decimal // P: total of the withholding taxes (WithholdingTaxAmount)
	Hash        string          // Q: four characters of the signature of the document, see HashChars
	Certificate string          // R: number of the certificate of the invoicing software (SoftwareCertificateNumber)
	Other       string          // S: other information, such as payment data, its parts separated by semicolons
}

// Space is a fiscal space of the QR code: its region and its taxable
// amounts and VAT, by rate, the totals of the document for each rate
// (section 3 k). The comments give the codes of the first fiscal space;
// the second and third use J and K instead of I.
type Space struct {
	// Region is the fiscal space (TaxCountryRegion of SAF-T (PT)): PT,
	// PT-AC (Azores), PT-MA (Madeira) or the ISO 3166-1 alpha-2 code of
	// another country. The first space holds 0 for a document that shows
	// no VAT rate (section 3 h).
	Region string // I1

	ExemptBase       invoice.Decimal // I2: taxable amount exempt from VAT, stamp duty operations included
	ReducedBase      invoice.Decimal // I3: taxable amount at the reduced rate
	ReducedVAT       invoice.Decimal // I4: VAT at the reduced rate
	IntermediateBase invoice.Decimal // I5: taxable amount at the intermediate rate
	IntermediateVAT  invoice.Decimal // I6: VAT at the intermediate rate
	NormalBase       invoice.Decimal // I7: taxable amount at the normal rate
	NormalVAT        invoice.Decimal // I8: VAT at the normal rate
}

// amounts returns pointers to the amounts of s, from code 2 to code 8.
func (s *Space) amounts() [7]*invoice.Decimal {
	return [7]*invoice.Decimal{&s.ExemptBase, &s.ReducedBase, &s.ReducedVAT,
		&s.IntermediateBase, &s.IntermediateVAT, &s.NormalBase, &s.NormalVAT}
}

// present reports whether s has a region or an amount.
func (s *Space) present() bool {
	if s.Region != "" {
		return true
	}
	for _, d := range s.amounts() {
		if d.IsSet() {
			return true
		}
	}
	return false
}

// spaceLetters are the letters of the codes of the three fiscal spaces.
const spaceLetters = "IJK"

// fieldCodes are the codes of the fields, in the order of the table of
// section 4, which the content follows (section 3 b).
var fieldCodes = func() []string {
	codes := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	for _, l := range spaceLetters {
		for i := 1; i <= 8; i++ {
			codes = append(codes, string(l)+strconv.Itoa(i))
		}
	}
	return append(codes, "L", "M", "N", "O", "P", "Q", "R", "S")
}()

// mandatory holds the codes of the mandatory fields, marked + in the table.
var mandatory = map[string]bool{"A": true, "B": true, "C": true, "D": true, "E": true, "F": true, "G": true, "H": true,
	"I1": true, "N": true, "O": true, "Q": true, "R": true}

var fieldIndex = func() map[string]int {
	m := make(map[string]int, len(fieldCodes))
	for i, c := range fieldCodes {
		m[c] = i
	}
	return m
}()

// String returns the content of the QR code: the fields in the order of
// the table, each its code, a colon and its value, separated by asterisks
// (section 3 a and b). The amounts have two decimals, rounded half away
// from zero (section 3 c). The mandatory fields are always written, empty
// when absent; String does not check the values: Check does.
func (q *QR) String() string {
	var b strings.Builder
	add := func(code, v string) {
		if b.Len() > 0 {
			b.WriteByte('*')
		}
		b.WriteString(code)
		b.WriteByte(':')
		b.WriteString(v)
	}
	amount := func(code string, d invoice.Decimal, mandatory bool) {
		switch {
		case d.IsSet():
			add(code, d.StringFixed(2))
		case mandatory:
			add(code, "")
		}
	}
	add("A", q.IssuerNIF)
	add("B", q.BuyerTaxID)
	add("C", q.BuyerCountry)
	add("D", q.DocumentType)
	add("E", q.DocumentStatus)
	add("F", q.Date.Format102())
	add("G", q.DocumentID)
	add("H", q.ATCUD)
	for i := range q.Spaces {
		s := &q.Spaces[i]
		if i > 0 && !s.present() {
			continue
		}
		l := spaceLetters[i : i+1]
		add(l+"1", s.Region)
		for k, d := range s.amounts() {
			amount(l+strconv.Itoa(k+2), *d, false)
		}
	}
	amount("L", q.NotSubject, false)
	amount("M", q.StampDuty, false)
	amount("N", q.TaxTotal, true)
	amount("O", q.GrossTotal, true)
	amount("P", q.Withholding, false)
	add("Q", q.Hash)
	add("R", q.Certificate)
	if q.Other != "" {
		add("S", q.Other)
	}
	return b.String()
}

// Symbol checks q and encodes its content in a QR Code symbol, as section
// 2 of the specification requires: error correction level M, byte mode,
// version 9 at least. Section 2 also asks for a symbol of 30 × 30 mm at
// least, with a margin of 0.25 cm: draw it with the PNG or SVG method of
// the symbol, at that size, in the body of the document, on its first or
// last page (Portaria n.º 195/2020, article 6.º), the ATCUD printed
// immediately above it (article 4.º).
func (q *QR) Symbol() (*qrcode.Symbol, error) {
	if err := q.Check(); err != nil {
		return nil, err
	}
	return qrcode.Encode([]byte(q.String()), qrcode.Options{Level: qrcode.Medium, MinVersion: 9})
}

// Parse reads the content of a QR code and checks it as Check does. It
// returns the fields it could read with the error, which joins a
// *FieldError for each field that does not follow the specification: a
// part without a colon or with an unknown code, a field out of the order of
// the table or repeated, an optional field left empty (section 3 f), an
// amount that is not written with two decimals and a point, a date that is
// not YYYYMMDD, then the findings of Check. A content that Parse accepts is
// the one that String writes back.
func Parse(s string) (*QR, error) {
	q := new(QR)
	c := checker{skip: make(map[string]bool)}
	lexical := func(field, format string, args ...any) {
		c.fail(field, format, args...)
		c.skip[field] = true
	}
	present := make(map[string]bool)
	last := -1
	for i, part := range strings.Split(s, "*") {
		code, v, ok := strings.Cut(part, ":")
		idx, known := fieldIndex[code]
		switch {
		case !ok:
			lexical("", "part %d %q is not a code, a colon and a value (section 3 a)", i+1, part)
			continue
		case !known:
			lexical("", "part %d %q has no code of the table of section 4", i+1, part)
			continue
		case present[code]:
			lexical(code, "the field appears twice")
			continue
		case idx < last:
			lexical(code, "the field comes after field %s: the fields follow the order of the table of section 4 (section 3 b)", fieldCodes[last])
			continue
		}
		present[code] = true
		last = idx
		if v == "" && !mandatory[code] {
			lexical(code, "the field is empty: an optional field without information is left out (section 3 f)")
			continue
		}
		switch code {
		case "A":
			q.IssuerNIF = v
		case "B":
			q.BuyerTaxID = v
		case "C":
			q.BuyerCountry = v
		case "D":
			q.DocumentType = v
		case "E":
			q.DocumentStatus = v
		case "F":
			d, err := invoice.ParseDate(v, "102")
			if len(v) != 8 || !isDigits(v) || err != nil || !d.Valid() {
				lexical(code, "%q is not a date YYYYMMDD", v)
				continue
			}
			q.Date = d
		case "G":
			q.DocumentID = v
		case "H":
			q.ATCUD = v
		case "L", "M", "N", "O", "P":
			d := map[string]*invoice.Decimal{"L": &q.NotSubject, "M": &q.StampDuty, "N": &q.TaxTotal, "O": &q.GrossTotal, "P": &q.Withholding}[code]
			if !c.readAmount(code, v, d) {
				c.skip[code] = true
			}
		case "Q":
			q.Hash = v
		case "R":
			q.Certificate = v
		case "S":
			q.Other = v
		default: // I1 to K8
			s := &q.Spaces[strings.IndexByte(spaceLetters, code[0])]
			if code[1] == '1' {
				s.Region = v
			} else if !c.readAmount(code, v, s.amounts()[code[1]-'2']) {
				c.skip[code] = true
			}
		}
	}
	q.check(&c)
	return q, errors.Join(c.errs...)
}

// readAmount reads v, an amount of field code, into d: digits, a point
// and two decimals (section 3 c). It reports whether v has that form; when
// it has not, the error is recorded and d holds v if it is a number.
func (c *checker) readAmount(code, v string, d *invoice.Decimal) bool {
	i := strings.IndexByte(v, '.')
	if i < 1 || len(v)-i != 3 || !isDigits(v[:i]) || !isDigits(v[i+1:]) || i > 1 && v[0] == '0' {
		c.fail(code, "%q is not an amount in digits with two decimals after a point, without thousands separator nor leading zero (section 3 c)", v)
		if x, err := invoice.ParseDecimal(v); err == nil {
			*d = x
		}
		return false
	}
	x, err := invoice.ParseDecimal(v)
	if err != nil {
		c.fail(code, "%q is not an amount: %v", v, err)
		return false
	}
	*d = x
	return true
}

// Check reports the fields of q that do not follow the specification, as a
// *FieldError each, joined:
//
//   - a mandatory field that is absent (A to H, I1, N, O, Q and R), a
//     value longer than the maximum size of the table (section 3 i), with a
//     space at either end (section 3 a), with an asterisk, the separator of
//     the fields, or that is not UTF-8;
//   - A that is not a NIF: nine digits, the last one the check digit
//     modulo 11;
//   - C that is not an ISO 3166-1 alpha-2 code nor Desconhecido; D that is
//     not two capital letters, E one;
//   - G that does not end with a slash and the sequential number of the
//     document, H that is not an ATCUD of that number (Portaria n.º
//     195/2020, article 3.º);
//   - a fiscal space that is not PT, PT-AC, PT-MA nor a country code, 0
//     outside I1, a fiscal space twice, amounts of a fiscal space without
//     its region, a document without VAT rate (I1:0) with amounts of fiscal
//     spaces (section 3 g and h);
//   - a negative amount, or one longer than 16 characters;
//   - Q that is not four characters of a Base64 signature, R that is not a
//     number of at most four digits.
//
// The specification puts no rule between the amounts: Check does not
// compare the totals N and O with the amounts of the fiscal spaces.
func (q *QR) Check() error {
	var c checker
	q.check(&c)
	return errors.Join(c.errs...)
}

// checker collects the errors of a check, except those of the fields that
// skip holds, whose error is already recorded.
type checker struct {
	errs []error
	skip map[string]bool
}

func (c *checker) fail(field, format string, args ...any) {
	if field != "" && c.skip[field] {
		return
	}
	c.errs = append(c.errs, &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)})
}

func (q *QR) check(c *checker) {
	if c.text("A", q.IssuerNIF, 9, true) && !validNIF(q.IssuerNIF) {
		c.fail("A", "%q is not a NIF: nine digits, the last one the check digit modulo 11", q.IssuerNIF)
	}
	c.text("B", q.BuyerTaxID, 30, true)
	if v := q.BuyerCountry; c.text("C", v, 12, true) && v != "Desconhecido" && !isCapitals(v, 2) {
		c.fail("C", "%q is not an ISO 3166-1 alpha-2 country code nor Desconhecido (Country of SAF-T (PT))", v)
	}
	if c.text("D", q.DocumentType, 2, true) && !isCapitals(q.DocumentType, 2) {
		c.fail("D", "%q is not a document type of SAF-T (PT), two capital letters", q.DocumentType)
	}
	if c.text("E", q.DocumentStatus, 1, true) && !isCapitals(q.DocumentStatus, 1) {
		c.fail("E", "%q is not a document status of SAF-T (PT), a capital letter", q.DocumentStatus)
	}
	switch {
	case q.Date.IsZero():
		c.fail("F", "missing: the field is mandatory")
	case !q.Date.Valid():
		c.fail("F", "%s is not a date", q.Date.Format102())
	}
	number, numbered := "", false
	if c.text("G", q.DocumentID, 60, true) {
		if number, numbered = sequentialNumber(q.DocumentID); !numbered {
			c.fail("G", "%q does not end with a slash and the sequential number of the document (Portaria n.º 195/2020, article 3.º, n.º 3)", q.DocumentID)
		}
	}
	if c.text("H", q.ATCUD, 70, true) {
		code, n, _ := strings.Cut(q.ATCUD, "-")
		switch err := (Code{ValidationCode: code, Number: n}).Check(); {
		case !strings.Contains(q.ATCUD, "-"):
			c.fail("H", "%q is not the validation code of the series and the sequential number separated by a hyphen (Portaria n.º 195/2020, article 3.º, n.º 2)", q.ATCUD)
		case err != nil:
			c.fail("H", "%s", err.(*FieldError).Msg)
		case numbered && n != number:
			c.fail("H", "the sequential number %s of the ATCUD is not %s, the number of the document identification G (Portaria n.º 195/2020, article 3.º, n.º 3)", n, number)
		}
	}
	q.checkSpaces(c)
	c.amount("L", q.NotSubject, false)
	c.amount("M", q.StampDuty, false)
	c.amount("N", q.TaxTotal, true)
	c.amount("O", q.GrossTotal, true)
	c.amount("P", q.Withholding, false)
	if c.text("Q", q.Hash, 4, true) && (len(q.Hash) != 4 || !isBase64(q.Hash)) {
		c.fail("Q", "%q is not the four characters of a Base64 signature (Portaria n.º 363/2010, article 6.º, n.º 3)", q.Hash)
	}
	if c.text("R", q.Certificate, 4, true) && !isDigits(q.Certificate) {
		c.fail("R", "%q is not the number of a certificate of the AT", q.Certificate)
	}
	c.text("S", q.Other, 65, false)
}

func (q *QR) checkSpaces(c *checker) {
	noRate := q.Spaces[0].Region == "0"
	for i := range q.Spaces {
		s := &q.Spaces[i]
		l := spaceLetters[i : i+1]
		if i > 0 && !s.present() {
			continue
		}
		switch {
		case i > 0 && noRate:
			c.fail(l+"1", "a document without VAT rate (I1:0) has a single fiscal space (section 3 h)")
		case !c.text(l+"1", s.Region, 5, true):
		case s.Region == "0" && i > 0:
			c.fail(l+"1", "0 stands for a document without VAT rate in I1 only (section 3 h)")
		case s.Region != "0" && s.Region != "PT-AC" && s.Region != "PT-MA" && !isCapitals(s.Region, 2):
			c.fail(l+"1", "%q is not a fiscal space: PT, PT-AC, PT-MA or the ISO 3166-1 alpha-2 code of another country (TaxCountryRegion of SAF-T (PT))", s.Region)
		default:
			for j := range i {
				if q.Spaces[j].Region == s.Region {
					c.fail(l+"1", "the fiscal space %s is already that of field %c1", s.Region, spaceLetters[j])
				}
			}
		}
		for k, d := range s.amounts() {
			code := l + strconv.Itoa(k+2)
			if noRate && i == 0 && d.IsSet() {
				c.fail(code, "a document without VAT rate (I1:0) has no amount by rate (section 3 h)")
				continue
			}
			c.amount(code, *d, false)
		}
	}
}

// text checks the rules common to the text fields and reports whether v is
// present and passes them.
func (c *checker) text(field, v string, size int, mandatory bool) bool {
	switch {
	case v == "":
		if mandatory {
			c.fail(field, "missing: the field is mandatory")
		}
		return false
	case !utf8.ValidString(v):
		c.fail(field, "%q is not UTF-8", v)
	case strings.TrimSpace(v) != v:
		c.fail(field, "%q starts or ends with a space: the field is its code, a colon and its value, without spaces (section 3 a)", v)
	case strings.Contains(v, "*"):
		c.fail(field, "%q holds an asterisk, the separator of the fields (section 3 b)", v)
	case utf8.RuneCountInString(v) > size:
		c.fail(field, "%q has %d characters, %d at most (section 3 i)", v, utf8.RuneCountInString(v), size)
	default:
		return true
	}
	return false
}

// amount checks an amount: present if mandatory, not negative, 16
// characters at most once written with two decimals.
func (c *checker) amount(field string, d invoice.Decimal, mandatory bool) {
	switch {
	case !d.IsSet():
		if mandatory {
			c.fail(field, "missing: the field is mandatory")
		}
	case d.Sign() < 0:
		c.fail(field, "%s is negative", d.StringFixed(2))
	case len(d.StringFixed(2)) > 16:
		c.fail(field, "%s has %d characters, 16 at most (section 3 i)", d.StringFixed(2), len(d.StringFixed(2)))
	}
}

// validNIF reports whether s is a Portuguese NIF: nine digits, the last
// one 11 minus the sum of the first eight weighted 9 to 2, modulo 11, or 0
// when that is 10 or 11.
func validNIF(s string) bool {
	if len(s) != 9 || !isDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * (9 - i)
	}
	check := 11 - sum%11
	if check >= 10 {
		check = 0
	}
	return int(s[8]-'0') == check
}

// isCapitals reports whether s is n capital letters of ASCII.
func isCapitals(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < n; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

// isBase64 reports whether s holds characters of the Base64 alphabet only,
// the padding left out.
func isBase64(s string) bool {
	for _, r := range s {
		if !isAlnum(r) && r != '+' && r != '/' {
			return false
		}
	}
	return true
}
