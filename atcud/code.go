package atcud

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// FieldError reports a field of the QR code that does not follow the
// specification, named by its code in the table of section 4 (A to S, I1
// to K8), H for the ATCUD.
type FieldError struct {
	Field string // A, B, C… or "" when the field cannot be told
	Msg   string
}

// Error returns the field and what is wrong with it.
func (e *FieldError) Error() string {
	if e.Field == "" {
		return "atcud: " + e.Msg
	}
	return "atcud: field " + e.Field + ": " + e.Msg
}

// Code is an ATCUD, the unique code of a document (código único do
// documento): the validation code that the AT assigned to its series and
// its sequential number in the series, which String writes separated by a
// hyphen (Portaria n.º 195/2020, article 3.º).
type Code struct {
	// ValidationCode is the validation code of the series, eight
	// characters at least, which the AT returns when the series is
	// communicated to it (article 2.º).
	ValidationCode string
	// Number is the sequential number of the document in the series, its
	// digits as the document identification writes them after the slash:
	// 0035 for FT AB2019/0035.
	Number string
}

// String returns the ATCUD: CSDF7T5H-0035. The document prints it after
// "ATCUD:" (article 4.º).
func (c Code) String() string { return c.ValidationCode + "-" + c.Number }

// New returns the ATCUD of the document identified by documentID (field
// InvoiceNo, DocumentNumber or PaymentRefNo of SAF-T (PT), such as
// FT AB2019/0035) in the series of validation code validationCode: the
// sequential number is the run of digits after the last slash of
// documentID (article 3.º, n.º 3).
func New(validationCode, documentID string) (Code, error) {
	n, ok := sequentialNumber(documentID)
	if !ok {
		return Code{}, &FieldError{Field: "G", Msg: fmt.Sprintf(
			"the document identification %q does not end with a slash and the sequential number (Portaria n.º 195/2020, article 3.º, n.º 3)", documentID)}
	}
	c := Code{ValidationCode: validationCode, Number: n}
	return c, c.Check()
}

// ParseCode reads an ATCUD, with or without the prefix "ATCUD:" that the
// document prints before it, and checks it.
func ParseCode(s string) (Code, error) {
	s = strings.TrimPrefix(s, "ATCUD:")
	code, number, ok := strings.Cut(s, "-")
	if !ok {
		return Code{}, &FieldError{Field: "H", Msg: fmt.Sprintf(
			"the ATCUD %q is not the validation code of the series and the sequential number separated by a hyphen (Portaria n.º 195/2020, article 3.º, n.º 2)", s)}
	}
	c := Code{ValidationCode: code, Number: number}
	return c, c.Check()
}

// Check reports the first part of c that does not follow article 3.º of
// Portaria n.º 195/2020, as a *FieldError of field H: a validation code
// shorter than eight characters, or with other characters than letters and
// digits, the only ones the codes of the AT hold; a sequential number that
// is not a run of digits.
func (c Code) Check() error {
	fail := func(format string, args ...any) error {
		return &FieldError{Field: "H", Msg: fmt.Sprintf(format, args...)}
	}
	if n := utf8.RuneCountInString(c.ValidationCode); n < 8 {
		return fail("the validation code of the series %q has %d characters, 8 at least (Portaria n.º 195/2020, article 3.º, n.º 1)", c.ValidationCode, n)
	}
	for _, r := range c.ValidationCode {
		if !isAlnum(r) {
			return fail("the validation code of the series %q holds %q: letters and digits only", c.ValidationCode, r)
		}
	}
	if c.Number == "" || !isDigits(c.Number) {
		return fail("the sequential number %q is not a run of digits (Portaria n.º 195/2020, article 3.º, n.º 3)", c.Number)
	}
	if n := len(c.String()); n > 70 {
		return fail("the ATCUD has %d characters, 70 at most", n)
	}
	return nil
}

// sequentialNumber returns the digits after the last slash of documentID.
func sequentialNumber(documentID string) (string, bool) {
	i := strings.LastIndexByte(documentID, '/')
	if i < 0 || i == len(documentID)-1 || !isDigits(documentID[i+1:]) {
		return "", false
	}
	return documentID[i+1:], true
}

func isAlnum(r rune) bool {
	return '0' <= r && r <= '9' || 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z'
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
