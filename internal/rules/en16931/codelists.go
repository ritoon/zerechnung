package en16931

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// codeListRule describes a BR-CL rule: the lookup function and the subject of
// its message.
type codeListRule struct {
	ok   func(string) bool
	what string
	list string
}

var codeListRules = map[string]codeListRule{
	"BR-CL-01": {codes.DocumentType, "invoice type code (BT-3)", "UNTDID 1001"},
	"BR-CL-03": {codes.Currency, "currencyID", "ISO 4217 alpha-3"},
	"BR-CL-04": {codes.Currency, "invoice currency code (BT-5)", "ISO 4217 alpha-3"},
	"BR-CL-05": {codes.Currency, "VAT accounting currency code (BT-6)", "ISO 4217 alpha-3"},
	"BR-CL-06": {codes.TaxPointDateCode, "value added tax point date code (BT-8)", "UNTDID 2005 restricted to 5, 29 and 72"},
	"BR-CL-07": {codes.ReferenceType, "object identifier scheme (BT-18)", "UNTDID 1153"},
	"BR-CL-08": {codes.TextSubject, "invoice note subject code (BT-21)", "UNTDID 4451"},
	"BR-CL-10": {codes.ICD, "identifier scheme", "ISO 6523 ICD"},
	"BR-CL-11": {codes.ICD, "identifier scheme", "ISO 6523 ICD"},
	"BR-CL-13": {codes.ItemClassification, "item classification scheme (BT-158)", "UNTDID 7143"},
	"BR-CL-14": {codes.Country, "country code", "ISO 3166-1 alpha-2"},
	"BR-CL-15": {codes.Country, "item country of origin (BT-159)", "ISO 3166-1 alpha-2"},
	"BR-CL-16": {codes.PaymentMeans, "payment means type code (BT-81)", "UNTDID 4461"},
	"BR-CL-17": {codes.VATCategory, "allowance or charge VAT category code", "UNTDID 5305 (EN 16931 subset)"},
	"BR-CL-18": {codes.VATCategory, "VAT category code", "UNTDID 5305 (EN 16931 subset)"},
	"BR-CL-19": {codes.AllowanceReason, "allowance reason code", "UNTDID 5189"},
	"BR-CL-20": {codes.ChargeReason, "charge reason code", "UNTDID 7161"},
	"BR-CL-21": {codes.ICD, "item standard identifier scheme (BT-157)", "ISO 6523 ICD"},
	"BR-CL-22": {codes.VATEX, "VAT exemption reason code (BT-121)", "CEF VATEX"},
	"BR-CL-23": {codes.Unit, "unit code", "UN/ECE Recommendations 20 and 21"},
	"BR-CL-24": {codes.MIME, "attached document MIME code (BT-125)", "the accepted MIME codes"},
	"BR-CL-25": {codes.EAS, "electronic address scheme (BT-34, BT-49)", "CEF EAS"},
	"BR-CL-26": {codes.ICD, "deliver to location identifier scheme (BT-71)", "ISO 6523 ICD"},
}

// codeList checks value against the code list of rule, as the official rule
// does: normalize-space(value) must contain no space and be in the list. The
// MIME codes of BR-CL-24 are compared exactly.
func (k *checker) codeList(rule string, n *xmltree.Node, value string) {
	if k.mode == modeExtendedCTCFR {
		k.ctcfrCodeList(rule, n, value)
		return
	}
	r := codeListRules[rule]
	v, ok := value, true
	if rule != "BR-CL-24" {
		v, ok = code(value)
	}
	if ok && r.ok(v) {
		return
	}
	k.fail(rule, n, "%s %q is not in %s", r.what, quote(value), r.list)
}

// vatPrefix checks BR-CO-09 on a VAT identifier: its first two characters
// must be a country prefix.
func (k *checker) vatPrefix(n *xmltree.Node) {
	s := n.Text
	// substring(., 1, 2): the first two characters.
	end, chars := 0, 0
	for end < len(s) && chars < 2 {
		c := s[end]
		switch {
		case c < 0x80:
			end++
		case c >= 0xf0:
			end += 4
		case c >= 0xe0:
			end += 3
		default:
			end += 2
		}
		chars++
	}
	if end > len(s) {
		end = len(s)
	}
	prefix := s[:end]
	if k.mode == modeExtendedCTCFR {
		// The list of the EXTENDED-CTC-FR schematron.
		if strings.Contains(ctcfrLists["BR-CO-09"], " "+prefix+" ") {
			return
		}
	} else if codes.VATPrefix(prefix) {
		return
	}
	k.fail("BR-CO-09", n, "VAT identifier %q does not start with an ISO 3166-1 alpha-2 country prefix", quote(s))
}

// ctcfrCodeList checks value against the list of the EXTENDED-CTC-FR
// schematron for rule, or for the rule that replaces it: normalize-space(value)
// must contain no space and be in the list. BR-CL-22 compares the value in
// upper case and BR-FREXT-CL-24 compares the MIME code exactly. The rules
// that the schematron removes (BR-CL-07) have no list and never fail.
func (k *checker) ctcfrCodeList(rule string, n *xmltree.Node, value string) {
	if r, ok := ctcfrRenamed[rule]; ok {
		rule = r
	}
	list, ok := ctcfrLists[rule]
	if !ok {
		return
	}
	var v string
	switch rule {
	case "BR-FREXT-CL-24":
		v = value
		ok = !strings.ContainsAny(v, " ") && strings.Contains(list, " "+v+" ")
	case "BR-CL-22":
		v = trim(value)
		ok = !strings.Contains(normalize(v), " ") && strings.Contains(list, " "+normalize(strings.ToUpper(v))+" ")
	default:
		v = normalize(value)
		ok = !strings.Contains(v, " ") && strings.Contains(list, " "+v+" ")
	}
	if ok {
		return
	}
	what, name := codeListRules[rule].what, codeListRules[rule].list
	if t, ok := ctcfrCodeListText[rule]; ok {
		what, name = t[0], t[1]
	}
	k.fail(rule, n, "%s %q is not in %s", what, quote(value), name)
}

// normalize returns s as XPath normalize-space() would: trimmed, with the
// inner runs of white space replaced by a single space.
func normalize(s string) string {
	v := trim(s)
	for i := 0; i < len(v); i++ {
		if c := v[i]; c != ' ' && isSpace(c) || c == ' ' && v[i+1] == ' ' {
			return strings.Join(strings.FieldsFunc(v, func(r rune) bool { return r < 0x80 && isSpace(byte(r)) }), " ")
		}
	}
	return v
}
