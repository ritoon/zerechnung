package hr

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The OIB (osobni identifikacijski broj) identifies a Croatian person or
// company: eleven digits, the last one the check digit of the first ten by
// the hybrid system of ISO 7064 (MOD 11,10). A Croatian VAT identifier is
// HR followed by it.

// oibCheckDigit returns the check digit of the ten digits ten (ISO 7064,
// MOD 11,10): starting from 10, each digit is added modulo 10, a result of
// 0 counting as 10, then doubled modulo 11; the check digit is 11 minus the
// last product, 10 giving 0.
func oibCheckDigit(ten string) byte {
	p := 10
	for i := range 10 {
		s := (int(ten[i]-'0') + p) % 10
		if s == 0 {
			s = 10
		}
		p = s * 2 % 11
	}
	return byte('0' + (11-p)%10)
}

// ValidOIB reports whether s is an OIB: eleven digits whose last one is the
// check digit of the others. The prefix HR and white space are not
// accepted. It is the function u:ctrlOIB of the schematron (HR-BR-9,
// HR-BR-53) without its prefix HR.
func ValidOIB(s string) bool {
	if len(s) != 11 {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return oibCheckDigit(s[:10]) == s[10]
}

// schemeOIB is the scheme of the code list EAS (Electronic Address Scheme)
// of a Croatian VAT number, which the electronic addresses of the official
// examples give as an OIB without the prefix HR.
const schemeOIB = "9934"

// checkAddresses reports, as warnings, the electronic addresses of scheme
// 9934 of the seller (BT-34) and of the buyer (BT-49) that are not an OIB.
func checkAddresses(c *rules.Checker, root *xmltree.Node) {
	for _, role := range []string{"AccountingSupplierParty", "AccountingCustomerParty"} {
		party := root.Elem(role).Elem("Party")
		for _, e := range party.Elems("EndpointID") {
			if strings.TrimSpace(e.Attr("schemeID")) != schemeOIB {
				continue
			}
			if v := strings.TrimSpace(e.Text); !ValidOIB(v) {
				c.Fail(RuleAddressOIB, rules.Warning, e,
					"the electronic address %q of scheme 9934 is not a valid OIB: eleven digits, the last one the check digit of ISO 7064 (MOD 11,10)", v)
			}
		}
	}
}
