package qrbill

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/invoice"
)

// maxContent is the maximum length of the content, separators included
// (chapter 6.2): the capacity in bytes of a symbol of version 25 at level
// M, which chapter 6.2 gives as the version of the longest content.
const maxContent = 997

// maxAdditional is the maximum length of the unstructured message and the
// billing information together (chapter 4.3.3).
const maxAdditional = 140

// notifications are the unstructured messages of a notification, by
// language (chapter 4.4, table 10).
var notifications = map[string]string{
	"de": "NICHT ZUR ZAHLUNG VERWENDEN",
	"fr": "NE PAS UTILISER POUR LE PAIEMENT",
	"it": "NON UTILIZZARE PER IL PAGAMENTO",
	"en": "DO NOT USE FOR PAYMENT",
	"rm": "BETG DUVRAR PER IL PAJAMENT",
}

// Notification returns the unstructured message of a QR-bill that only
// notifies the invoice recipient, "DO NOT USE FOR PAYMENT" in the language
// lang (ISO 639-1: de, fr, it, en or rm), and whether the guidelines give
// one in that language: such a QR-bill has the amount 0.00 (chapter 4.4,
// table 10).
func Notification(lang string) (string, bool) {
	m, ok := notifications[lang]
	return m, ok
}

// Check reports each line of b that does not follow version b.Guidelines
// of the guidelines, as a *FieldError each, joined:
//
//   - an account that is not an IBAN of 21 characters of Switzerland or
//     Liechtenstein, without spaces and with valid check digits (ISO 13616);
//   - an address without name, postal code, town or country, a country that
//     is not ISO 3166-1 alpha-2, a value longer than its line admits;
//   - an amount out of 0.01 to 999,999,999.99, or with more than two
//     decimals; 0.00 is the amount of a notification, whose message is
//     that of Notification in one of its languages (chapter 4.4), and of
//     nothing else; such a message with another amount;
//   - a currency other than CHF or EUR;
//   - a reference type that does not match the account: QRR with a QR-IBAN,
//     SCOR or NON with an IBAN (chapter 7.1); with version 2.4, a QR-IBAN or
//     a QR reference in EUR (chapters 2.10 and 2.12.1);
//   - a QR reference that is not 27 digits, all zeros, or whose check digit
//     (modulo 10 recursive, Annex B) is wrong; a Creditor Reference that is
//     not RF, its check digits (modulo 97-10) and 1 to 21 letters or digits
//     (ISO 11649); a reference with type NON;
//   - a message and billing information longer than 140 characters
//     together, billing information that does not begin with // and the
//     code of its syntax, and with //S1 the findings of ParseS1; more than
//     two alternative procedures, one longer than 100 characters;
//   - a character outside the character set of chapter 4.1.1 (Basic Latin,
//     Latin-1 Supplement, Latin Extended-A, Ș ș Ț ț and €), a line break in
//     a value;
//   - a content longer than 997 characters (chapter 6.2), counted in bytes
//     of UTF-8, which a symbol of version 25 at level M holds.
func (b *Bill) Check() error {
	var c checker
	b.check(&c)
	return errors.Join(c.errs...)
}

// checker collects the errors of a check, except those of the lines that
// skip holds, whose error is already recorded.
type checker struct {
	errs []error
	skip map[int]bool
}

func (c *checker) fail(line int, format string, args ...any) {
	if line != 0 && c.skip[line] {
		return
	}
	c.errs = append(c.errs, &FieldError{Line: line, Element: elements[line], Msg: fmt.Sprintf(format, args...)})
}

func (b *Bill) check(c *checker) {
	qrIBAN := false
	switch v := b.Account; {
	case v == "":
		c.fail(lineIBAN, "the IBAN of the creditor is missing")
	case !validIBAN(v):
		c.fail(lineIBAN, "%q is not an IBAN of Switzerland or Liechtenstein: 21 letters and digits without spaces, CH or LI and the check digits of ISO 13616", v)
	default:
		qrIBAN = IsQRIBAN(v)
	}
	c.address(lineCreditor, &b.Creditor)
	b.checkAmount(c)
	if v := b.Currency; v != "CHF" && v != "EUR" {
		c.fail(lineCurrency, "%q is not CHF or EUR, the only currencies of the QR-bill", v)
	}
	if b.Debtor != nil {
		c.address(lineDebtor, b.Debtor)
	}
	b.checkReference(c, qrIBAN)
	b.checkAdditional(c)
	if n := len(b.String()); n > maxContent {
		c.fail(0, "the content is %d bytes long: 997 at most, separators included, which a symbol of version 25 at level M holds (chapter 6.2)", n)
	}
}

func (b *Bill) checkAmount(c *checker) {
	notification := false
	for _, v := range notifications {
		notification = notification || b.Message == v
	}
	d := b.Amount
	if notification && (!d.IsSet() || !d.IsZero()) {
		c.fail(lineAmount, "a notification (%q) has the amount 0.00 (chapter 4.4)", b.Message)
	}
	switch {
	case !d.IsSet():
	case d.Scale() > 2 && !d.Equal(d.Round(2)):
		c.fail(lineAmount, "%s has more than two decimals", d)
	case d.IsZero():
		if !notification {
			c.fail(lineAmount, "0.00 is the amount of a notification only, whose message is %q or its translation (chapter 4.4)", notifications["en"])
		}
	case d.Sign() < 0 || d.Cmp(invoice.MustDecimal("999999999.99")) > 0:
		c.fail(lineAmount, "%s is not between 0.01 and 999,999,999.99 (table 8)", d)
	}
}

func (b *Bill) checkReference(c *checker, qrIBAN bool) {
	ref := b.Reference
	switch t := b.ReferenceType; t {
	case QRReference:
		if !qrIBAN && !c.skip[lineIBAN] && b.Account != "" && validIBAN(b.Account) {
			c.fail(lineRefType, "a QR reference needs a QR-IBAN, whose institution identification is between 30000 and 31999 (chapter 4.3.2)")
		}
		if b.Guidelines != Guidelines23 && b.Currency == "EUR" {
			c.fail(lineRefType, "the QR-IBAN and the QR reference are for invoices in CHF only since version 2.4 of the guidelines; version 2.3, valid until November 2027, admits them in EUR (chapter 2.12.1)")
		}
		if !validQRReference(ref) {
			c.fail(lineRef, "%q is not a QR reference: 27 digits, not all zeros, the last one the check digit modulo 10 recursive (chapter 2.12.1, Annex B)", ref)
		}
	case CreditorReference, NoReference:
		if qrIBAN {
			c.fail(lineRefType, "%s goes with an IBAN: a QR-IBAN needs a QR reference, type QRR (chapter 4.3.2)", t)
		}
		if t == NoReference {
			if ref != "" {
				c.fail(lineRef, "a payment without reference (NON) leaves the reference empty (table 8)")
			}
		} else if validQRReference(ref) {
			c.fail(lineRef, "%q is a QR reference, which goes with a QR-IBAN: an IBAN takes a Creditor Reference (chapter 4.3.2)", ref)
		} else if !validCreditorReference(ref) {
			c.fail(lineRef, "%q is not a Creditor Reference: RF, two check digits modulo 97-10 and 1 to 21 letters or digits, 25 characters at most (ISO 11649, chapter 2.12.2)", ref)
		}
	default:
		c.fail(lineRefType, "%q is not QRR, SCOR or NON", string(t))
	}
}

func (b *Bill) checkAdditional(c *checker) {
	c.text(lineMessage, b.Message, maxAdditional, false)
	if info := b.BillInformation; info != "" {
		if c.text(lineBillInfo, info, maxAdditional, false) {
			switch {
			case utf8.RuneCountInString(b.Message)+utf8.RuneCountInString(info) > maxAdditional:
				c.fail(lineBillInfo, "the unstructured message and the billing information are %d characters long together: 140 at most (chapter 4.3.3)",
					utf8.RuneCountInString(b.Message)+utf8.RuneCountInString(info))
			case len(info) < 4 || !strings.HasPrefix(info, "//"):
				c.fail(lineBillInfo, "%q does not begin with // and the two characters of the code of its syntax, such as //S1 (chapter 4.3.3)", info)
			case strings.HasPrefix(info, "//S1"):
				if _, err := ParseS1(info); err != nil {
					c.errs = append(c.errs, err.(interface{ Unwrap() []error }).Unwrap()...)
				}
			}
		}
	}
	if len(b.AlternativeSchemes) > 2 {
		c.fail(lineAltFirst, "%d alternative procedures: two at most (table 8)", len(b.AlternativeSchemes))
	}
	for i, v := range b.AlternativeSchemes[:min(len(b.AlternativeSchemes), 2)] {
		c.text(lineAltFirst+i, v, 100, false)
	}
}

// address checks the address whose type is in line base: the group is
// filled, so its name, postal code, town and country are mandatory (table
// 8, status M for the creditor, D for the ultimate debtor).
func (c *checker) address(base int, a *Address) {
	c.text(base+1, a.Name, 70, true)
	c.text(base+2, a.Street, 70, false)
	c.text(base+3, a.BuildingNumber, 16, false)
	c.text(base+4, a.PostCode, 16, true)
	c.text(base+5, a.Town, 35, true)
	if v := a.Country; c.text(base+6, v, 2, true) && !codes.Country(v) {
		c.fail(base+6, "%q is not an ISO 3166-1 alpha-2 country code", v)
	}
}

// text checks the value v of a line: present if mandatory, size characters
// at most, in the character set of chapter 4.1.1. It reports whether v is
// present and valid.
func (c *checker) text(line int, v string, size int, mandatory bool) bool {
	if v == "" {
		if mandatory {
			c.fail(line, "the line is mandatory and empty")
		}
		return false
	}
	ok := true
	if n := utf8.RuneCountInString(v); n > size {
		c.fail(line, "%d characters: %d at most (table 8)", n, size)
		ok = false
	}
	for _, r := range v {
		if !allowedRune(r) {
			c.fail(line, "%q is not in the character set of the Swiss QR Code (chapter 4.1.1)", r)
			return false
		}
	}
	return ok
}

// allowedRune reports whether r is in the character set of chapter 4.1.1:
// Basic Latin (U+0020 to U+007E), Latin-1 Supplement (U+00A0 to U+00FF),
// Latin Extended-A (U+0100 to U+017F), Ș ș Ț ț (U+0218 to U+021B) and €
// (U+20AC). An invalid UTF-8 sequence reads as U+FFFD, which it is not.
func allowedRune(r rune) bool {
	return r >= 0x20 && r <= 0x7E || r >= 0xA0 && r <= 0x17F || r >= 0x218 && r <= 0x21B || r == 0x20AC
}
