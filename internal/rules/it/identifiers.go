package it

import "strings"

// pivaFormat returns "" when s has the format of a Partita IVA, the Italian
// VAT number, or why it has not: 11 digits (the company number, 7 digits,
// the code of the tax office, 3 digits, and the check digit). The check
// digit is not verified (see ValidPIVA).
func pivaFormat(s string) string {
	if len(s) != 11 || !digits(s) {
		return "it must have 11 digits"
	}
	return ""
}

// pivaCheckDigit returns the check digit of the first ten digits of a
// Partita IVA: the Luhn algorithm, as defined by the Ministerial Decree of
// 23/12/1976. The digits in odd positions (first, third…) are added; those
// in even positions are doubled, 9 being subtracted from a double above 9,
// and added; the check digit brings the sum to a multiple of 10.
func pivaCheckDigit(first10 string) byte {
	sum := 0
	for i := range 10 {
		d := int(first10[i] - '0')
		if i%2 == 1 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}

// ValidPIVA reports whether s is a Partita IVA: 11 digits whose last one is
// the check digit of the others. Surrounding white space and the prefix IT
// are not accepted.
func ValidPIVA(s string) bool {
	return pivaFormat(s) == "" && pivaCheckDigit(s[:10]) == s[10]
}

// Positions of a natural person's Codice Fiscale (from 0) that hold digits,
// which may be replaced by the letters of omocodia to tell apart two persons
// with the same code.
var cfDigitPositions = [...]int{6, 7, 9, 10, 12, 13, 14}

// omocodia lists the letters that replace the digits 0 to 9 in a Codice
// Fiscale.
const omocodia = "LMNPQRSTUV"

// cfMonths lists the letters of the months, January to December.
const cfMonths = "ABCDEHLMPRST"

// cfFormat returns "" when s has the format of a Codice Fiscale, or why it
// has not. The Codice Fiscale of a legal person, or the temporary one of a
// natural person, has 11 digits, like a Partita IVA. That of a natural
// person has 16 upper-case letters and digits (Ministerial Decree of
// 23/12/1976): 3 letters of the surname, 3 of the name, the year of birth
// (2 digits), the month (a letter of cfMonths), the day (2 digits, plus 40
// for a woman: 01 to 31 or 41 to 71), the code of the municipality or of
// the foreign country (a letter and 3 digits), and the check letter. Each
// digit may be replaced by its letter of omocodia (L for 0, M for 1… V for
// 9). The check character is not verified (see ValidCF).
func cfFormat(s string) string {
	if len(s) == 11 {
		if !digits(s) {
			return "a code of 11 characters (legal person) must have 11 digits"
		}
		return ""
	}
	if len(s) != 16 {
		return "it must have 16 characters (natural person) or 11 digits (legal person)"
	}
	for i := range 16 {
		if c := s[i]; !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return "a code of 16 characters must have upper-case letters and digits only"
		}
	}
	for _, i := range [...]int{0, 1, 2, 3, 4, 5, 8, 11, 15} {
		if c := s[i]; c < 'A' || c > 'Z' {
			return "a code of 16 characters must have letters in positions 1 to 6, 9, 12 and 16"
		}
	}
	plain := []byte(s)
	for _, i := range cfDigitPositions {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case strings.IndexByte(omocodia, c) >= 0:
			plain[i] = byte('0' + strings.IndexByte(omocodia, c))
		default:
			return "a code of 16 characters must have digits, or letters L to V in their place, in positions 7, 8, 10, 11 and 13 to 15"
		}
	}
	if strings.IndexByte(cfMonths, s[8]) < 0 {
		return "the month of birth, in position 9, must be one of the letters A, B, C, D, E, H, L, M, P, R, S, T"
	}
	if day := int(plain[9]-'0')*10 + int(plain[10]-'0'); day < 1 || day > 31 && day < 41 || day > 71 {
		return "the day of birth, in positions 10 and 11, must be 01 to 31, or 41 to 71 for a woman"
	}
	return ""
}

// cfOdd gives the value of a character in an odd position (first, third…)
// of a Codice Fiscale, for the digits 0 to 9 then the letters A to Z (a
// digit has the value of the letter of the same rank: 0 as A, 1 as B…).
var cfOdd = [...]int{1, 0, 5, 7, 9, 13, 15, 17, 19, 21, 2, 4, 18, 20, 11, 3, 6, 8, 12, 14, 16, 10, 22, 25, 24, 23}

// cfCheckChar returns the check character of the first 15 characters of a
// natural person's Codice Fiscale: the characters in odd positions are
// converted by cfOdd, those in even positions by their rank (0 to 9 for a
// digit, 0 to 25 for a letter A to Z), and the remainder of the division of
// their sum by 26 gives the letter (0 for A… 25 for Z).
func cfCheckChar(first15 string) byte {
	sum := 0
	for i := range 15 {
		v := 0
		if c := first15[i]; c >= '0' && c <= '9' {
			v = int(c - '0')
		} else {
			v = int(c - 'A')
		}
		if i%2 == 0 {
			v = cfOdd[v]
		}
		sum += v
	}
	return byte('A' + sum%26)
}

// cfCheck returns the expected check character of a Codice Fiscale in the
// format of cfFormat.
func cfCheck(s string) byte {
	if len(s) == 11 {
		return pivaCheckDigit(s[:10])
	}
	return cfCheckChar(s[:15])
}

// ValidCF reports whether s is a Codice Fiscale: the format of cfFormat and
// a valid check character. Surrounding white space and lower-case letters
// are not accepted.
func ValidCF(s string) bool {
	return cfFormat(s) == "" && cfCheck(s) == s[len(s)-1]
}

// validIBAN reports whether s is an IBAN in its electronic form (ISO 13616):
// a country code of two upper-case letters, two check digits and 11 to 30
// upper-case letters and digits, the whole number, the first four
// characters moved to the end and the letters counting as 10 to 35,
// leaving a remainder of 1 when divided by 97 (ISO/IEC 7064 MOD 97-10).
func validIBAN(s string) bool {
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case i < 2 && (c < 'A' || c > 'Z'):
			return false
		case i >= 2 && i < 4 && (c < '0' || c > '9'):
			return false
		case !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'):
			return false
		}
	}
	r := 0
	for _, c := range []byte(s[4:] + s[:4]) {
		if c >= 'A' {
			r = (r*100 + int(c-'A'+10)) % 97
		} else {
			r = (r*10 + int(c-'0')) % 97
		}
	}
	return r == 1
}

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// provinces lists the codes (sigle) of the Italian provinces and
// metropolitan cities, which the CIUS-IT requires in the country
// subdivisions BT-39, BT-54 and BT-79 of an Italian address, and the
// FatturaPA format in Provincia: the 107 codes in use, and CI, OG, OT and
// VS, the codes of four Sardinian provinces abolished in 2016. These are
// accepted too: the rules only decide whether the SDI maps the value, and
// an old code is no reason to warn.
var provinces = func() map[string]bool {
	const codes = "AG AL AN AO AP AQ AR AT AV BA BG BI BL BN BO BR BS BT BZ CA CB CE CH CI CL CN CO CR CS CT CZ " +
		"EN FC FE FG FI FM FR GE GO GR IM IS KR LC LE LI LO LT LU MB MC ME MI MN MO MS MT NA NO NU OG OR OT " +
		"PA PC PD PE PG PI PN PO PR PT PU PV PZ RA RC RE RG RI RM RN RO SA SI SO SP SR SS SU SV " +
		"TA TE TN TO TP TR TS TV UD VA VB VC VE VI VR VS VT VV"
	m := map[string]bool{}
	for c := range strings.FieldsSeq(codes) {
		m[c] = true
	}
	return m
}()
