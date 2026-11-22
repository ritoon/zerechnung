package ro

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The Romanian fiscal identification code (CIF, cod de identificare
// fiscală, or CUI, cod unic de înregistrare, for a company) has two to ten
// digits, the last one a check digit; a VAT identifier is RO followed by
// it. The CIUS-RO requires a VAT identifier or a tax registration
// identifier of the seller (BR-RO-065) but does not check them: checkCIF
// reports a malformed CIF as a warning.

// Rules of the check of the CIF.
const (
	RuleCIFFormat = "RO-CIF-01"
	RuleCIFKey    = "RO-CIF-02"
)

// cifKey is the key of the check digit of a CIF: the weights of the first
// nine digits, the CIF without its check digit being padded with leading
// zeros to nine digits.
const cifKey = "753217532"

// cifCheckDigit returns the check digit of a CIF whose digits but the last
// are body (1 to 9 digits): the sum of the digits weighted by cifKey,
// multiplied by 10, modulo 11, 10 giving 0.
func cifCheckDigit(body string) byte {
	padded := strings.Repeat("0", 9-len(body)) + body
	sum := 0
	for i := range 9 {
		sum += int(padded[i]-'0') * int(cifKey[i]-'0')
	}
	d := sum * 10 % 11
	if d == 10 {
		d = 0
	}
	return byte('0' + d)
}

// cifFormat reports whether s is a CIF: two to ten digits.
func cifFormat(s string) bool {
	if len(s) < 2 || len(s) > 10 {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ValidCIF reports whether s is a Romanian CIF: two to ten digits whose
// last one is the check digit of the others. The prefix RO and white space
// are not accepted.
func ValidCIF(s string) bool {
	return cifFormat(s) && cifCheckDigit(s[:len(s)-1]) == s[len(s)-1]
}

// cifParties are the identifiers checkCIF reads, in the CII view of the
// invoice: the VAT identifiers of the seller (BT-31), of the buyer (BT-48)
// and of the seller's tax representative (BT-63), and the tax registration
// identifier of the seller (BT-32).
var cifParties = []struct {
	party, scheme, bt string
}{
	{"SellerTradeParty", "VA", "BT-31"},
	{"SellerTradeParty", "FC", "BT-32"},
	{"BuyerTradeParty", "VA", "BT-48"},
	{"SellerTaxRepresentativeTradeParty", "VA", "BT-63"},
}

func checkCIF(c *rules.Checker) {
	agr := c.Doc.Agreement
	sellerRO := strings.TrimSpace(agr.Value("SellerTradeParty/PostalTradeAddress/CountryID")) == "RO"
	for _, p := range cifParties {
		for _, n := range agr.FindAll(p.party + "/SpecifiedTaxRegistration/ID") {
			if strings.TrimSpace(n.Attr("schemeID")) != p.scheme {
				continue
			}
			checkCIFValue(c, n, p.bt, p.scheme == "FC", sellerRO)
		}
	}
}

// checkCIFValue checks the identifier n, named bt in the messages: a VAT
// identifier that starts with RO, or the tax registration identifier of a
// seller in Romania (registration), with or without RO.
func checkCIFValue(c *rules.Checker, n *xmltree.Node, bt string, registration, sellerRO bool) {
	v := n.Normalized()
	cif, prefixed := strings.CutPrefix(v, "RO")
	switch {
	case v == "":
		return
	case registration && !sellerRO, !registration && !prefixed:
		return
	case !cifFormat(cif):
		if registration {
			c.Fail(RuleCIFFormat, rules.Warning, n,
				"the tax registration identifier %s (%s) of a seller in Romania is not a CIF: 2 to 10 digits, with or without RO", bt, v)
		} else {
			c.Fail(RuleCIFFormat, rules.Warning, n,
				"the Romanian VAT identifier %s (%s) is not RO followed by the 2 to 10 digits of a CIF", bt, v)
		}
		return
	}
	if want := cifCheckDigit(cif[:len(cif)-1]); cif[len(cif)-1] != want {
		c.Fail(RuleCIFKey, rules.Warning, n,
			"the check digit of the CIF %s (%s) is %c, not %c", cif, bt, want, cif[len(cif)-1])
	}
}
