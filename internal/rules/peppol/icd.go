package peppol

import (
	"regexp"
	"sync"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// identifier checks a ram:URIID, ram:ID or ram:GlobalID against the format
// of its ICD scheme (PEPPOL-COMMON-R*). The scheme 9907 is only checked on
// a ram:URIID.
func (k *checker) identifier(n *xmltree.Node) {
	scheme, ok := n.LookupAttr("schemeID")
	if !ok || scheme == "9907" && n.Name != "URIID" {
		return
	}
	v := nsv(n)
	if id, valid := icdRule(scheme, v, sv(n)); id != "" {
		k.check(valid, id, n, "scheme %s, value: %q", scheme, v)
	}
}

// icdRule returns the PEPPOL-COMMON rule that checks an identifier with
// the ICD scheme, "" when there is none, and whether the identifier passes
// it: v is the identifier with normalize-space(), s its string value
// (string(), which the Danish rules use). The functions follow the u:*
// functions of the schematrons, character by character; the tests are the
// same in CII and UBL.
func icdRule(scheme, v, s string) (id string, valid bool) {
	switch scheme {
	case "0088":
		return "PEPPOL-COMMON-R040", allDigits(v) && gln(v)
	case "0192":
		return "PEPPOL-COMMON-R041", digits(v, 9) && mod11(v)
	case "0184":
		return "PEPPOL-COMMON-R042", length(s) == 10 && substring(s, 1, 2) == "DK" && onlyDigits(substring(s, 3, 8)) ||
			length(s) == 8 && onlyDigits(substring(s, 1, 8))
	case "0096":
		return "PEPPOL-COMMON-R052", length(s) == 10 && onlyDigits(substring(s, 1, 10))
	case "0198":
		return "PEPPOL-COMMON-R053", length(s) == 10 && substring(s, 1, 2) == "DK" && onlyDigits(substring(s, 3, 8))
	case "0208":
		return "PEPPOL-COMMON-R043", digits(v, 10) && mod97BE(v)
	case "0201":
		return "PEPPOL-COMMON-R044", codiceIPA(v)
	case "0210":
		return "PEPPOL-COMMON-R045", codiceFiscale(v)
	case "9907":
		return "PEPPOL-COMMON-R046", codiceFiscale(v)
	case "0211":
		return "PEPPOL-COMMON-R047", partitaIVA(v)
	case "0007":
		return "PEPPOL-COMMON-R049", length(v) == 10 && isNumber(v) && swedishOrgNumber(v)
	case "0151":
		return "PEPPOL-COMMON-R050", abn(v)
	case "0106":
		return "PEPPOL-COMMON-R054", digits(v, 8)
	case "0190":
		return "PEPPOL-COMMON-R055", digits(v, 20)
	case "9944":
		return "PEPPOL-COMMON-R056-1", dutchVAT(v)
	case "0217":
		return "PEPPOL-COMMON-R057", digits(v, 12)
	}
	return "", false
}

// onlyDigits reports whether s holds ASCII digits only, or is empty: what
// the schematron tests by removing the digits with translate() and checking
// that nothing is left.
func onlyDigits(s string) bool { return s == "" || allDigits(s) }

var reDutchVAT = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^NL[0-9]{9}B[0-9]{2}$`) })

// dutchVAT matches a Dutch VAT number, NL123456789B12.
func dutchVAT(s string) bool { return reDutchVAT().MatchString(s) }

// gln implements u:gln: the GS1 check digit (weights 3 and 1 from the right)
// of a string of digits.
func gln(s string) bool {
	n := len(s) - 1
	if n < 0 {
		return false
	}
	sum := 0
	for i := 0; i < n; i++ {
		d := int(s[n-1-i] - '0')
		sum += d * (1 + (i+1)%2*2)
	}
	return (10-sum%10)%10 == int(s[n]-'0')
}

// mod11 implements u:mod11: the modulus 11 check digit of Norwegian
// organization numbers (weights 2 to 7 from the right), on a positive
// number.
func mod11(s string) bool {
	r := []rune(s)
	n := len(r) - 1
	if n < 0 {
		return false
	}
	sum := 0
	for i := 0; i < n; i++ {
		sum += (int(r[n-1-i]) - '0') * (i%6 + 2)
	}
	f, ok := double(s)
	if !ok || !(f > 0) || r[n] < '0' || r[n] > '9' {
		return false
	}
	return (11-sum%11)%11 == int(r[n]-'0')
}

// mod97BE implements u:mod97-0208 on ten digits: the last two are 97 minus
// the first eight modulo 97.
func mod97BE(s string) bool {
	v := 0
	for i := 0; i < 8; i++ {
		v = v*10 + int(s[i]-'0')
	}
	return int(s[8]-'0')*10+int(s[9]-'0') == 97-v%97
}

// codiceIPA implements u:checkCodiceIPA: six ASCII letters or digits.
func codiceIPA(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c, l := s[i], s[i]|0x20; !(c >= '0' && c <= '9' || l >= 'a' && l <= 'z') {
			return false
		}
	}
	return true
}

// codiceFiscale implements u:checkCF: 16 characters following the pattern
// of a personal tax code, or 11 characters castable as an integer.
func codiceFiscale(s string) bool {
	switch length(s) {
	case 16:
		return letters(substring(s, 1, 6)) && castableInteger(substring(s, 7, 2)) &&
			letters(substring(s, 9, 1)) && castableInteger(substring(s, 10, 2)) &&
			castableInteger(substring(s, 15, 1)) && letters(substring(s, 16, 1))
	case 11:
		return castableInteger(s)
	}
	return false
}

// partitaIVA implements u:checkPIVAseIT: an identifier starting with IT or
// it must be followed by 11 digits whose Luhn-like sum (u:addPIVA) is a
// multiple of 10; other identifiers pass.
func partitaIVA(s string) bool {
	country := substring(s, 1, 2)
	if country != "IT" && country != "it" {
		return true
	}
	code := substringFrom(s, 3)
	if length(code) != 11 || !castableInteger(code) {
		return false
	}
	// u:addPIVA: digits at odd positions count as such, digits at even
	// positions through the table 0246813579; it stops at the first suffix
	// that is not castable as an integer (the empty string).
	sum := 0
	even := false
	for rest := code; castableInteger(rest); rest = substringFrom(rest, 2) {
		c := rest[0]
		if c < '0' || c > '9' {
			return false // xs:integer() of a sign or a space: an error in the reference
		}
		d := int(c - '0')
		if even {
			d = int("0246813579"[d] - '0')
		}
		sum += d
		even = !even
	}
	return sum%10 == 0
}

// swedishOrgNumber implements u:checkSEOrgnr: a string of digits whose
// tenth digit is the Luhn check digit of the first nine.
func swedishOrgNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			// \d also matches other Unicode digits, which number() then
			// rejects: an error in the reference.
			return false
		}
	}
	main := substring(s, 1, 9)
	check := substring(s, 10, 1)
	if check == "" {
		return false
	}
	sum := 0
	for pos := 1; pos <= len(main); pos++ {
		d := int(main[len(main)-pos] - '0')
		if pos%2 == 1 {
			sum += d*2%10 + d*2/10
		} else {
			sum += d
		}
	}
	return (10-sum%10)%10 == int(check[0]-'0')
}

// abn implements u:abn: the weighted sum of the first eleven characters of
// an Australian Business Number (the first digit minus one) is a multiple of
// 89. Shorter strings fail.
func abn(s string) bool {
	weights := [11]int{10, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
	r := []rune(s)
	if len(r) < 11 {
		return false
	}
	sum := int(r[0]) - 49
	sum *= weights[0]
	for i := 1; i < 11; i++ {
		sum += (int(r[i]) - 48) * weights[i]
	}
	return sum%89 == 0
}
