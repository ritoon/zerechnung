package pt

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The CIUS-PT asks for a VAT identifier preceded by its country code
// (DT-CIUS-PT-029, -046.1, -061.1) but does not check the Portuguese one,
// the NIF (número de identificação fiscal): nine digits, the last a check
// digit modulo 11. checkNIF reports a malformed NIF as a warning, as the
// examples of eSPap themselves carry NIFs whose check digit is wrong.

// nifParties are the VAT identifiers checkNIF reads: of the seller (BT-31),
// of the buyer (BT-48) and of the seller's tax representative (BT-63).
var nifParties = []struct{ path, bt string }{
	{"AccountingSupplierParty/Party/PartyTaxScheme/CompanyID", "BT-31"},
	{"AccountingCustomerParty/Party/PartyTaxScheme/CompanyID", "BT-48"},
	{"TaxRepresentativeParty/PartyTaxScheme/CompanyID", "BT-63"},
}

func checkNIF(c *rules.Checker, root *xmltree.Node) {
	for _, p := range nifParties {
		for _, n := range root.FindAll(p.path) {
			v := normalizeSpace(stringValue(item{k: kNode, n: n}))
			nif, ok := strings.CutPrefix(v, "PT")
			if !ok {
				continue
			}
			if !nineDigits(nif) {
				c.Fail("PT-NIF-01", rules.Warning, n,
					"the Portuguese VAT identifier %s (%s) is not PT followed by the nine digits of a NIF", p.bt, v)
				continue
			}
			if want := nifCheckDigit(nif); nif[8] != want {
				c.Fail("PT-NIF-02", rules.Warning, n,
					"the check digit of the NIF %s (%s) is %c, not %c", nif, p.bt, want, nif[8])
			}
		}
	}
}

func nineDigits(s string) bool {
	if len(s) != 9 {
		return false
	}
	for i := 0; i < 9; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// nifCheckDigit returns the check digit of a NIF: 11 minus the sum of its
// first eight digits weighted 9 to 2, modulo 11, or 0 when that is 10 or
// 11.
func nifCheckDigit(nif string) byte {
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(nif[i]-'0') * (9 - i)
	}
	d := 11 - sum%11
	if d >= 10 {
		d = 0
	}
	return byte('0' + d)
}
