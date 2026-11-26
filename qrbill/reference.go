package qrbill

import (
	"fmt"
	"strconv"
	"strings"
)

// IsQRIBAN reports whether iban is a QR-IBAN: an IBAN of Switzerland or
// Liechtenstein, 21 characters without spaces, whose institution
// identification (QR-IID, characters 5 to 9) is between 30000 and 31999
// (chapters 2.8 and 2.10). A QR-IBAN goes with a QR reference, and only
// with one.
func IsQRIBAN(iban string) bool {
	if !validIBAN(iban) {
		return false
	}
	iid, err := strconv.Atoi(iban[4:9])
	return err == nil && iid >= 30000 && iid <= 31999
}

// validIBAN reports whether s is an IBAN of Switzerland or Liechtenstein
// (table 8, line 4): CH or LI, two check digits, five digits of the
// institution identification and twelve letters or digits of the account,
// whose check digits are right (ISO 13616, modulo 97).
func validIBAN(s string) bool {
	if len(s) != 21 || s[:2] != "CH" && s[:2] != "LI" || !isDigits(s[2:9]) {
		return false
	}
	for i := 9; i < len(s); i++ {
		if !isAlnumUpper(s[i]) {
			return false
		}
	}
	return mod97(s[4:]+s[:4]) == 1
}

// mod97 returns the remainder of the division by 97 of s, its letters
// replaced by numbers (A = 10 … Z = 35), as ISO 13616 and ISO 11649 do. s
// holds digits and capital letters only.
func mod97(s string) int {
	r := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' {
			r = (r*100 + int(c-'A') + 10) % 97
		} else {
			r = (r*10 + int(c-'0')) % 97
		}
	}
	return r
}

func isAlnumUpper(c byte) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' }

// mod10Table is the table of the check digit modulo 10 recursive (Annex
// B, figure 21): the carry-forward of the next digit is
// mod10Table[(carry + digit) % 10].
var mod10Table = [10]int{0, 9, 4, 6, 8, 2, 7, 1, 3, 5}

// mod10 returns the check digit modulo 10 recursive of the digits of s.
func mod10(s string) byte {
	carry := 0
	for i := 0; i < len(s); i++ {
		carry = mod10Table[(carry+int(s[i]-'0'))%10]
	}
	return byte('0' + (10-carry)%10)
}

// validQRReference reports whether s is a QR reference: 27 digits, not
// all zeros, the last one the check digit modulo 10 recursive of the 26
// others (chapter 2.12.1, Annex B).
func validQRReference(s string) bool {
	return len(s) == 27 && isDigits(s) && strings.Trim(s, "0") != "" && mod10(s[:26]) == s[26]
}

// NewQRReference returns the QR reference of digits, up to 26 digits that
// the invoice issuer chooses, such as the number of the invoice or of the
// customer: digits padded with leading zeros to 26 digits, followed by
// their check digit modulo 10 recursive (chapter 2.12.1, Annex B). Spaces
// in digits are ignored, so that a reference printed in groups of five
// can be given as printed. Some banks fix the first digits of the
// reference: ask yours.
func NewQRReference(digits string) (string, error) {
	digits = strings.ReplaceAll(digits, " ", "")
	if len(digits) > 26 || !isDigits(digits) || strings.Trim(digits, "0") == "" {
		return "", &FieldError{Line: lineRef, Element: elements[lineRef], Msg: fmt.Sprintf(
			"%q is not 1 to 26 digits, not all zeros, the QR reference without its check digit", digits)}
	}
	digits = strings.Repeat("0", 26-len(digits)) + digits
	return digits + string(mod10(digits)), nil
}

// validCreditorReference reports whether s is a Creditor Reference of ISO
// 11649: RF, two check digits and 1 to 21 letters or digits, whose
// remainder modulo 97 is 1 once RF and its check digits are moved to the
// end. The banks draw no distinction between lower and upper case (table
// 8), so neither does it.
func validCreditorReference(s string) bool {
	s = strings.ToUpper(s)
	if len(s) < 5 || len(s) > 25 || s[:2] != "RF" || !isDigits(s[2:4]) {
		return false
	}
	for i := 4; i < len(s); i++ {
		if !isAlnumUpper(s[i]) {
			return false
		}
	}
	return mod97(s[4:]+s[:4]) == 1
}

// NewCreditorReference returns the Creditor Reference of ISO 11649 of
// ref, 1 to 21 letters or digits that the invoice issuer chooses: RF, its
// check digits modulo 97-10, and ref in capitals (chapter 2.12.2).
// Spaces in ref are ignored.
func NewCreditorReference(ref string) (string, error) {
	ref = strings.ToUpper(strings.ReplaceAll(ref, " ", ""))
	ok := ref != "" && len(ref) <= 21
	for i := 0; ok && i < len(ref); i++ {
		ok = isAlnumUpper(ref[i])
	}
	if !ok {
		return "", &FieldError{Line: lineRef, Element: elements[lineRef], Msg: fmt.Sprintf(
			"%q is not 1 to 21 letters or digits, the Creditor Reference without RF and its check digits", ref)}
	}
	return fmt.Sprintf("RF%02d%s", 98-mod97(ref+"RF00"), ref), nil
}
