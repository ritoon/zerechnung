package qrbill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// Guidelines is a version of the Swiss Implementation Guidelines for the
// QR-bill, whose rules Check applies. The content of a Swiss QR Code does
// not tell them apart: both write version 0200 in line 2.
type Guidelines int

const (
	// Guidelines24 is version 2.4 of 24 February 2026, in force from
	// 14 November 2026: the QR-IBAN and the QR reference are for invoices
	// in CHF only (chapters 2.10 and 2.12.1).
	Guidelines24 Guidelines = iota
	// Guidelines23 is version 2.3 of 20 November 2023, in force from
	// 21 November 2025 and valid until November 2027 (change control of
	// version 2.4): it also admits the QR-IBAN and the QR reference in EUR.
	Guidelines23
)

// String returns the version: 2.4 or 2.3.
func (g Guidelines) String() string {
	if g == Guidelines23 {
		return "2.3"
	}
	return "2.4"
}

// ReferenceType is the type of the payment reference, line 28.
type ReferenceType string

// The three types of reference (table 8, element Tp).
const (
	QRReference       ReferenceType = "QRR"  // QR reference, with a QR-IBAN
	CreditorReference ReferenceType = "SCOR" // Creditor Reference ISO 11649, with an IBAN
	NoReference       ReferenceType = "NON"  // no reference, with an IBAN
)

// Bill is the content of a Swiss QR Code, line by line, as table 8 of
// chapter 4.2.2 of the guidelines defines it; the comment of each field
// gives its lines. The header (lines 1 to 3: SPC, 0200, 1), the address
// type S of each address, the ultimate creditor (lines 12 to 18, which
// stay empty) and the trailer EPD (line 31) are fixed: String writes them
// and Parse checks them.
type Bill struct {
	// Guidelines is the version of the guidelines whose rules Check
	// applies: the zero value is version 2.4.
	Guidelines Guidelines

	Account  string          // line 4: IBAN or QR-IBAN of the creditor, 21 characters without spaces, CH or LI
	Creditor Address         // lines 5 to 11: the creditor, who holds the account
	Amount   invoice.Decimal // line 19: the amount, two decimals; unset when the debtor chooses it
	Currency string          // line 20: CHF or EUR
	Debtor   *Address        // lines 21 to 27: the ultimate debtor ("Payable by"), nil when absent

	ReferenceType ReferenceType // line 28: QRR, SCOR or NON
	Reference     string        // line 29: the QR reference or the Creditor Reference, empty with NON
	Message       string        // line 30: unstructured message ("Additional information")

	// BillInformation is line 32, the coded billing information for the
	// invoice recipient, which begins with // and the code of its syntax:
	// //S1/10/… for the syntax S1 of SIX (see S1).
	BillInformation string
	// AlternativeSchemes are lines 33 and 34, the parameters of at most
	// two alternative procedures, such as eBill/B/simon.muster@example.com.
	AlternativeSchemes []string
}

// Address is a structured address, the only type the guidelines admit
// since version 2.3 (address type S, chapter 4.3.1).
type Address struct {
	Name           string // first name and last name, or company name: 70 characters at most
	Street         string // street or P.O. box: 70 characters at most
	BuildingNumber string // 16 characters at most
	PostCode       string // without country prefix: 16 characters at most
	Town           string // 35 characters at most
	Country        string // ISO 3166-1 alpha-2 code
}

// FieldError reports a line of the content that does not follow the
// guidelines, named by its number and its element in table 8.
type FieldError struct {
	Line    int    // 1 to 34, or 0 for the content as a whole
	Element string // QRType, IBAN, Cdtr.Name, Amt, Ref… ("" with line 0)
	Msg     string
}

// Error returns the line, its element and what is wrong with it.
func (e *FieldError) Error() string {
	if e.Line == 0 {
		return "qrbill: " + e.Msg
	}
	return fmt.Sprintf("qrbill: line %d (%s): %s", e.Line, e.Element, e.Msg)
}

// Additional reports whether the error concerns a line of status A, the
// billing information or an alternative procedure (lines 32 to 34): the
// guidelines forbid to reject a QR-bill for them (chapter 4.2.1, table 7).
func (e *FieldError) Additional() bool { return e.Line >= lineBillInfo }

// The numbers of the lines that the code names.
const (
	lineIBAN     = 4
	lineCreditor = 5 // address type of the creditor; the address follows
	lineUltimate = 12
	lineAmount   = 19
	lineCurrency = 20
	lineDebtor   = 21
	lineRefType  = 28
	lineRef      = 29
	lineMessage  = 30
	lineTrailer  = 31
	lineBillInfo = 32
	lineAltFirst = 33
	maxLines     = 34
)

// elements are the names of the elements of the lines, in table 8.
var elements = func() [maxLines + 1]string {
	addr := []string{"AdrTp", "Name", "StrtNmOrAdrLine1", "BldgNbOrAdrLine2", "PstCd", "TwnNm", "Ctry"}
	var e [maxLines + 1]string
	copy(e[1:], []string{"QRType", "Version", "Coding", "IBAN"})
	for i, a := range addr {
		e[lineCreditor+i] = "Cdtr." + a
		e[lineUltimate+i] = "UltmtCdtr." + a
		e[lineDebtor+i] = "UltmtDbtr." + a
	}
	e[lineAmount], e[lineCurrency] = "Amt", "Ccy"
	e[lineRefType], e[lineRef], e[lineMessage], e[lineTrailer] = "Tp", "Ref", "Ustrd", "Trailer"
	e[lineBillInfo], e[lineAltFirst], e[lineAltFirst+1] = "StrdBkgInf", "AltPmt", "AltPmt"
	return e
}()

// String returns the content of the Swiss QR Code: the lines separated by
// a line feed, which chapter 4.1.4 admits as CR + LF, without a line break
// after the last one. Lines 32 to 34, of status A, are left out after the
// last that is filled. String does not check the values: Check does.
func (b *Bill) String() string {
	lines := make([]string, 0, maxLines)
	lines = append(lines, "SPC", "0200", "1", b.Account)
	lines = appendAddress(lines, &b.Creditor)
	lines = append(lines, "", "", "", "", "", "", "") // ultimate creditor, for future use
	lines = append(lines, b.Amount.StringFixed(2), b.Currency)
	if !b.Amount.IsSet() {
		lines[lineAmount-1] = ""
	}
	if b.Debtor != nil {
		lines = appendAddress(lines, b.Debtor)
	} else {
		lines = append(lines, "", "", "", "", "", "", "")
	}
	lines = append(lines, string(b.ReferenceType), b.Reference, b.Message, "EPD", b.BillInformation)
	lines = append(lines, b.AlternativeSchemes...)
	for len(lines) > lineTrailer && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func appendAddress(lines []string, a *Address) []string {
	return append(lines, "S", a.Name, a.Street, a.BuildingNumber, a.PostCode, a.Town, a.Country)
}

// Parse reads the content of a Swiss QR Code and checks it against the
// rules of version g of the guidelines, as Check does. It returns the
// lines it could read with the error, which joins a *FieldError for each
// line that does not follow the guidelines: a header other than SPC,
// 0200 and 1 (a content that does not begin with SPC is not read further),
// lines separated by something else than CR + LF or LF, or by both, fewer
// than 31 lines or more than 34, an address type other than S, an
// ultimate creditor that is filled, an amount without its two decimals or
// with a leading zero, a trailer other than EPD, then the findings of
// Check.
//
// Lines 32 to 34 are of status A: Parse accepts them empty, as the
// examples of Annex A show them, or left out, as chapter 4.1.4 asks, and
// a line break after the last line. A content that Parse accepts without
// those is the one that String writes back, CR + LF turned into LF.
func Parse(s string, g Guidelines) (*Bill, error) {
	b := &Bill{Guidelines: g}
	c := checker{skip: make(map[int]bool)}
	lexical := func(line int, format string, args ...any) {
		c.fail(line, format, args...)
		c.skip[line] = true
	}
	lines := strings.Split(s, "\n")
	crlf := strings.HasSuffix(lines[0], "\r")
	mixed := false
	for i, l := range lines[:len(lines)-1] {
		trimmed, cr := strings.CutSuffix(l, "\r")
		if cr != crlf && !mixed {
			lexical(0, "line %d ends with another line break than line 1: the same line break, CR + LF or LF, separates every line (chapter 4.1.4)", i+1)
			mixed = true
		}
		lines[i] = trimmed
	}
	if lines[0] != "SPC" {
		return b, errors.Join(append(c.errs, &FieldError{Line: 1, Element: elements[1], Msg: fmt.Sprintf(
			"%q is not SPC: the content is not a Swiss QR Code", lines[0])})...)
	}
	for len(lines) > lineTrailer && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	switch {
	case len(lines) < lineTrailer:
		lexical(0, "the content has %d lines: every line up to the trailer EPD, line 31, is present, empty if need be (chapter 4.1.4)", len(lines))
		for n := len(lines) + 1; n <= lineTrailer; n++ {
			c.skip[n] = true // missing, which the error above reports
		}
		lines = append(lines, make([]string, lineTrailer-len(lines))...)
	case len(lines) > maxLines:
		lexical(0, "the content has %d lines: 34 at most, with two alternative procedures (table 8)", len(lines))
		lines = lines[:maxLines]
	}
	line := func(n int) string { return lines[n-1] }
	if v := line(2); v != "0200" {
		lexical(2, "%q is not 0200, the only version of the major version 02 (table 8)", v)
	}
	if v := line(3); v != "1" {
		lexical(3, "%q is not 1, the coding type UTF-8 restricted to the Latin character set (table 8)", v)
	}
	b.Account = line(lineIBAN)
	parseAddress(lexical, lines, lineCreditor, &b.Creditor)
	for n := lineUltimate; n < lineUltimate+7; n++ {
		if line(n) != "" {
			lexical(n, "the ultimate creditor is not filled in: it is reserved for future use (table 8)")
		}
	}
	if v := line(lineAmount); v != "" {
		if d, ok := parseAmount(v); ok {
			b.Amount = d
		} else {
			lexical(lineAmount, "%q is not an amount with two decimals, a decimal point and no leading zero: 1949.75 (table 8)", v)
		}
	}
	b.Currency = line(lineCurrency)
	empty := true
	for n := lineDebtor; n < lineDebtor+7; n++ {
		empty = empty && line(n) == ""
	}
	if !empty {
		b.Debtor = new(Address)
		parseAddress(lexical, lines, lineDebtor, b.Debtor)
	}
	b.ReferenceType = ReferenceType(line(lineRefType))
	b.Reference = line(lineRef)
	b.Message = line(lineMessage)
	if v := line(lineTrailer); v != "EPD" {
		lexical(lineTrailer, "%q is not EPD, the trailer that ends the payment data (table 8)", v)
	}
	if len(lines) >= lineBillInfo {
		b.BillInformation = line(lineBillInfo)
	}
	if len(lines) > lineBillInfo {
		b.AlternativeSchemes = lines[lineBillInfo:]
	}
	b.check(&c)
	if len(s) > maxContent && len(b.String()) <= maxContent {
		c.fail(0, "the content is %d bytes long: 997 at most, separators included (chapter 6.2)", len(s))
	}
	return b, errors.Join(c.errs...)
}

// parseAddress reads the address that begins with its type in line base.
func parseAddress(lexical func(int, string, ...any), lines []string, base int, a *Address) {
	switch t := lines[base-1]; t {
	case "S":
	case "K":
		lexical(base, "the combined address (K) is no longer admitted since version 2.3 of the guidelines: S, a structured address (chapter 4.3.1)")
	default:
		lexical(base, "%q is not S, the address type of a structured address (table 8)", t)
	}
	a.Name, a.Street, a.BuildingNumber = lines[base], lines[base+1], lines[base+2]
	a.PostCode, a.Town, a.Country = lines[base+3], lines[base+4], lines[base+5]
}

// parseAmount reads an amount of line 19: digits without leading zero, a
// decimal point and two decimals.
func parseAmount(v string) (invoice.Decimal, bool) {
	whole, frac, ok := strings.Cut(v, ".")
	if !ok || len(frac) != 2 || !isDigits(frac) || whole == "" || !isDigits(whole) ||
		len(whole) > 1 && whole[0] == '0' || len(v) > 12 {
		return invoice.Decimal{}, false
	}
	d, err := invoice.ParseDecimal(v)
	return d, err == nil
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
