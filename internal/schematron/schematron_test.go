package schematron

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

func parse(t *testing.T, doc string) *xmltree.Node {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// testNS are the prefixes of the expressions of the tests.
var testNS = map[string]string{
	"xs":  "http://www.w3.org/2001/XMLSchema",
	"cbc": "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
	"cac": "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2",
}

// xpathDoc is the document the expressions of TestXPath are evaluated on,
// from its root element.
const xpathDoc = `<r><v>-0.00</v><w>-0</w><n>10</n><m>9</m><d>2.5</d><e>X y</e>` +
	`<b>x</b><b>y</b><s> a  b </s></r>`

// TestXPath checks the semantics of the interpreter on the points where an
// XSLT 2.0 processor departs from a naive reading of the schematron.
func TestXPath(t *testing.T) {
	root := parse(t, xpathDoc)
	d := newDoc(root)
	eval := func(src string) (bool, error) {
		e, err := compile(src, testNS)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return ebvOf(e, &ctx{d: d, it: item{k: kNode, n: root}, pos: 1, size: 1})
	}
	for _, src := range []string{
		// Untyped values compare as strings with each other, as xs:double
		// with a number.
		"not(n > m)", "n > 9", "xs:decimal(n) > xs:decimal(m)", "n = '10'",
		// Saxon orders the untyped "-0.00" cast to xs:double below 0, not
		// the untyped "-0".
		"v < 0", "not(v = 0)", "w = 0",
		// xs:decimal arithmetic is exact; a division keeps 18 decimals,
		// rounded half down.
		"xs:decimal('0.1') + xs:decimal('0.2') = 0.3",
		"xs:decimal(10) div 3 = 3.333333333333333333",
		"xs:decimal(2) div 3 = 0.666666666666666667",
		"1 div 2000000000000000000 = 0",
		"1 div 8 = 0.125",
		// round() rounds halves towards positive infinity.
		"round(2.5) = 3", "round(-2.5) = -2", "round(xs:decimal(d)) = 3",
		// substring() counts from 1 and rounds its arguments.
		"substring('abcdef', 2, 3) = 'bcd'", "substring('abc', 0) = 'abc'", "substring('12345', 1.5, 2.6) = '234'",
		"normalize-space(s) = 'a b'", "string-length('ação') = 4",
		"substring-before(substring-after('#A#v#', '#A#'), '#') = 'v'",
		// Regular expressions of XPath: . does not match a newline but with
		// the flag s, \d is any decimal digit of Unicode.
		"matches('A1', '^[a-z]\\d$', 'i')", "not(matches('a\nb', '^a.b$'))", "matches('a\nb', '^a.b$', 's')",
		"matches('٣', '^\\d$')", "matches('abc', '^(.{1,3})$')", "not(matches('abcd', '^(.{1,3})$'))",
		"count(b) = 2", "b = 'y'", "every $x in b satisfies $x != ''", "some $x in b satisfies $x = 'y'",
		"contains(' AED EUR ', concat(' ', 'EUR', ' '))", "not(exists(missing))", "empty(missing)",
	} {
		ok, err := eval(src)
		if err != nil || !ok {
			t.Errorf("%s: %v, %v", src, ok, err)
		}
	}
	// A function given several items where it allows one, or a value that
	// does not cast, raises an error.
	for _, src := range []string{"normalize-space(b) = 'x'", "xs:decimal(e) > 0", "xs:decimal('') = 0", "string-length(b) > 0"} {
		if _, err := eval(src); err == nil {
			t.Errorf("%s: no error", src)
		}
	}
}

// TestPatternError checks that an element whose context predicate raises
// an error does not match the rule, as with an XSLT 3.0 processor, while
// the same predicate in an expression stops its evaluation.
func TestPatternError(t *testing.T) {
	root := parse(t, `<r><a><b>x</b><b>x</b></a><a><b>x</b></a></r>`)
	d := newDoc(root)
	top := &ctx{d: d, it: item{k: kNode}, pos: 1, size: 1}
	e, err := compileContext("a[normalize-space(b) = 'x']", testNS, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.eval(top)
	if err != nil || len(got) != 1 || got[0].n != root.Children[1] {
		t.Errorf("context: %v, %v", got, err)
	}
	e, err = compile("//a[normalize-space(b) = 'x']", testNS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.eval(top); err == nil {
		t.Error("expression: no error")
	}
}

// TestXPathRO checks the parts of XPath 2.0 that the CIUS-RO schematron
// uses beyond the CIUS-PT one, as Saxon-HE 12 evaluates them.
func TestXPathRO(t *testing.T) {
	root := parse(t, `<r><c>RO</c><c>RO</c><s>RO-B</s><f>false</f><t> 1 </t><x>X</x>`+
		`<a><i>S</i><k>VAT</k></a><a><i>O</i><k>X</k></a>`+
		`<d>2026-11-20</d><p> 2026-11-20</p><y>0000-01-01</y><z>2026-02-29</z><l>2024-02-29</l><u>2026-11-20Z</u></r>`)
	d := newDoc(root)
	eval := func(src string) (bool, error) {
		e, err := compile(src, testNS)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return ebvOf(e, &ctx{d: d, it: item{k: kNode, n: root}, pos: 1, size: 1})
	}
	for _, src := range []string{
		// text() selects the text children, string-length(text()) the
		// length of the only one, white space included.
		"string-length(p/text()) = 11", "count(d/text()) = 1", "empty(a/text())",
		// following-sibling.
		"a/i[following-sibling::k = 'VAT'] = 'S'", "count(a/i[following-sibling::k = 'VAT']) = 1", "empty(k/following-sibling::*)",
		// boolean() and the comparison of an untyped value with a boolean,
		// cast to xs:boolean.
		"boolean(s)", "not(boolean(missing))", "boolean(normalize-space(s))", "not(boolean(normalize-space(' ')))",
		"f = false()", "t = true()", "not(f = true())",
		// castable as xs:date: XSD 1.1 accepts the year 0000; a time zone
		// is allowed; 29 February only in a leap year.
		"d castable as xs:date", "string(d) castable as xs:date", "y castable as xs:date", "u castable as xs:date",
		"l castable as xs:date", "not(z castable as xs:date)", "not(p castable as xs:date) or true()", "not('2026-13-01' castable as xs:date)",
		"not(missing castable as xs:date)", "not(c castable as xs:date)",
		// An and is false when one of its operands is false, an or true
		// when one of its operands is true, whatever an error in the other.
		"not(normalize-space(c) = 'RO' and normalize-space(s) = 'X')",
		"not(normalize-space(s) = 'X' and normalize-space(c) = 'RO')",
		"normalize-space(c) = 'RO' or normalize-space(s) = 'RO-B'",
		"not(xs:decimal(x) = 1 and s = 'X')",
	} {
		ok, err := eval(src)
		if err != nil || !ok {
			t.Errorf("%s: %v, %v", src, ok, err)
		}
	}
	// The error is raised when the other operand does not decide.
	for _, src := range []string{
		"normalize-space(c) = 'RO' and normalize-space(s) = 'RO-B'",
		"normalize-space(c) = 'RO' or normalize-space(s) = 'X'",
		"x = true()", "string-length(c/text()) = 2 and true()",
	} {
		if _, err := eval(src); err == nil {
			t.Errorf("%s: no error", src)
		}
	}
}

// TestRun checks the claims of the rules in a pattern, the variables, the
// messages with values, the reports and the failure of a test that raises
// an error. The assertions bear identifiers of rules of EN 16931, which
// other tests raise: a test binary built with the tag ruletrace records the
// rules it reports, which must be in a catalog (TestRuleCoverage).
func TestRun(t *testing.T) {
	root := parse(t, `<r><a>5</a><a>2</a><b>x</b><b>y</b></r>`)
	s, err := Compile(testNS, []Pattern{{
		ID:   "p",
		Lets: []Let{{Name: "max", Value: "1"}},
		Rules: []Rule{
			// The first rule hides a[1] from the next ones, although BR-01
			// is not kept.
			{Context: "a[. = '5']", Asserts: []Assertion{{ID: "BR-01", Flag: rules.Error, Test: "false()", Msg: "first rule"}}},
			{Context: "a", Lets: []Let{{Name: "v", Value: "xs:decimal(.)"}}, Asserts: []Assertion{
				{ID: "BR-02", Flag: rules.Error, Test: "$v <= $max", Msg: "%s is %s, above 1 (100%%)", Values: []string{"name()", "."}},
				{ID: "BR-03", Flag: rules.Warning, Report: true, Test: "$v = 2", Msg: "two"},
				{ID: "BR-04", Flag: rules.Error, Test: "true()", Msg: "never"},
			}},
			{Context: "/r", Asserts: []Assertion{{ID: "BR-05", Flag: rules.Error, Test: "normalize-space(b) = 'x'", Msg: "error"}}},
			{Context: "a", Asserts: []Assertion{{ID: "BR-06", Flag: rules.Error, Test: "false()", Msg: "hidden"}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	s.Run(&rules.Checker{Doc: &rules.Doc{Root: root}, Report: r}, root, func(id string) bool { return id != "BR-01" })
	var got []string
	for _, f := range r.Findings {
		got = append(got, f.Rule+" "+f.Severity.String()+" "+f.Path+" "+f.Message)
	}
	want := []string{
		"BR-05 error /r error",
		"BR-02 error /r/a[2] a is 2, above 1 (100%)",
		"BR-03 warning /r/a[2] two",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A message whose verbs do not match its values does not compile.
	if _, err := Compile(testNS, []Pattern{{ID: "p", Rules: []Rule{{Context: "a", Asserts: []Assertion{{ID: "R", Test: "true()", Msg: "%s", Values: []string{".", "."}}}}}}}); err == nil {
		t.Error("message with 1 verb and 2 values: no error")
	}
}

// TestXPathHR checks the parts of XPath 2.0 and XSLT 2.0 that the
// schematron of the Croatian CIUS uses beyond the CIUS-RO one, as Saxon-HE
// 12 evaluates them: if expressions, the operator mod, number(), the
// constructor xs:date() and the comparison of dates.
func TestXPathHR(t *testing.T) {
	root := parse(t, `<r><d>2026-01-01</d><e> 2025-12-31 </e><z>2026-01-01+01:00</z><w>2026-01-01-00:00</w>`+
		`<bad>2026-02-30</bad><n>7</n><x>X</x><a>1.5</a><a>2</a></r>`)
	d := newDoc(root)
	eval := func(src string) (seq, error) {
		e, err := compile(src, testNS)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return e.eval(&ctx{d: d, it: item{k: kNode, n: root}, pos: 1, size: 1})
	}
	for _, src := range []string{
		"if (n) then true() else false()", "if (missing) then false() else true()",
		"(if (/r) then n else x) = 7", "(if (n = 8) then n else n * -1) = -7",
		// mod keeps the sign of the dividend; on xs:double, as fmod.
		"10 mod 3 = 1", "-10 mod 3 = -1", "10 mod -3 = 1", "xs:decimal('5.5') mod 2 = 1.5",
		"(number(n) + 10) mod 10 = 7", "number('x') != number('x')", "string(number('') ) = 'NaN'",
		"number(n) = 7", "number(substring('123', 2, 1)) = 2", "number(true()) = 1",
		// xs:date: comparison by starting instant, white space collapsed,
		// time zone +00:00 or -00:00 written Z.
		"xs:date(d) >= xs:date('2026-01-01')", "xs:date(e) < xs:date('2026-01-01')",
		"xs:date(z) < xs:date(d)", "xs:date(w) = xs:date(d)", "string(xs:date(w)) = '2026-01-01Z'",
		"xs:date(d) < xs:date('2100-01-01')", "xs:date('-0001-12-31') < xs:date('0000-01-01')",
		"not(xs:date(missing) >= xs:date('1900-01-01'))", "empty(xs:date(missing))",
		"d = xs:date('2026-01-01')", "xs:date(d) != xs:date(e)",
	} {
		v, err := eval(src)
		ok, err2 := ebv(v)
		if err != nil || err2 != nil || !ok {
			t.Errorf("%s: %v, %v %v", src, v, err, err2)
		}
	}
	for _, src := range []string{
		"xs:date(bad) > xs:date('1900-01-01')", "xs:date(x)", "xs:date(a)", "boolean(xs:date(d))",
		"5 mod 0", "xs:date(d) > 1", "xs:date(d) + 1",
	} {
		v, err := eval(src)
		if err == nil {
			if _, err = ebv(v); err == nil {
				t.Errorf("%s: no error", src)
			}
		}
	}
}

// ctrlOIB is the function u:ctrlOIB of the schematron of the Croatian CIUS
// (HR-CIUS-EXT-EN16931-UBL.sch of 13/03/2026): the check digit of an OIB
// (ISO 7064, MOD 11,10), with or without the prefix HR.
var ctrlOIB = Function{
	Name: "u:ctrlOIB", As: "xs:boolean", Params: []Param{{Name: "o"}},
	Lets: []Let{
		{Name: "oib", Value: "if (starts-with($o, 'HR')) then substring($o, 3) else $o"},
		{Name: "valid", Value: "matches($oib,'^[0-9]{11}$')"},
		{Name: "z1", Value: "number(substring($oib, 1, 1))"}, {Name: "z2", Value: "number(substring($oib, 2, 1))"},
		{Name: "z3", Value: "number(substring($oib, 3, 1))"}, {Name: "z4", Value: "number(substring($oib, 4, 1))"},
		{Name: "z5", Value: "number(substring($oib, 5, 1))"}, {Name: "z6", Value: "number(substring($oib, 6, 1))"},
		{Name: "z7", Value: "number(substring($oib, 7, 1))"}, {Name: "z8", Value: "number(substring($oib, 8, 1))"},
		{Name: "z9", Value: "number(substring($oib, 9, 1))"}, {Name: "z10", Value: "number(substring($oib, 10, 1))"},
		{Name: "ctrl", Value: "number(substring($oib, 11, 1))"},
		{Name: "inter1", Value: "if (($z1 + 10) mod 10 = 0) then 9 else (($z1 + 10) mod 10 * 2) mod 11"},
		{Name: "inter2", Value: "if (($z2 + $inter1) mod 10 = 0) then 9 else (($z2 + $inter1) mod 10 * 2) mod 11"},
		{Name: "inter3", Value: "if (($z3 + $inter2) mod 10 = 0) then 9 else (($z3 + $inter2) mod 10 * 2) mod 11"},
		{Name: "inter4", Value: "if (($z4 + $inter3) mod 10 = 0) then 9 else (($z4 + $inter3) mod 10 * 2) mod 11"},
		{Name: "inter5", Value: "if (($z5 + $inter4) mod 10 = 0) then 9 else (($z5 + $inter4) mod 10 * 2) mod 11"},
		{Name: "inter6", Value: "if (($z6 + $inter5) mod 10 = 0) then 9 else (($z6 + $inter5) mod 10 * 2) mod 11"},
		{Name: "inter7", Value: "if (($z7 + $inter6) mod 10 = 0) then 9 else (($z7 + $inter6) mod 10 * 2) mod 11"},
		{Name: "inter8", Value: "if (($z8 + $inter7) mod 10 = 0) then 9 else (($z8 + $inter7) mod 10 * 2) mod 11"},
		{Name: "inter9", Value: "if (($z9 + $inter8) mod 10 = 0) then 9 else (($z9 + $inter8) mod 10 * 2) mod 11"},
		{Name: "inter10", Value: "if (($z10 + $inter9) mod 10 = 0) then 9 else (($z10 + $inter9) mod 10 * 2) mod 11"},
		{Name: "calcCtrl", Value: "if (11 - $inter10 = 10) then 0 else 11 - $inter10"},
	},
	Result: "$valid and ($calcCtrl = $ctrl)",
}

// check3010 is the function u:check3010 of the same schematron: at most 30
// characters and 10 decimals.
var check3010 = Function{
	Name: "u:check3010", As: "xs:boolean", Params: []Param{{Name: "broj", As: "xs:anyAtomicType?"}},
	Lets:   []Let{{Name: "d", Value: "string($broj)"}},
	Result: "string-length($d) <= 30 and string-length(substring-after($d,'.')) <= 10",
}

// oibKey returns the check digit of the ten digits of an OIB, as ISO 7064
// (MOD 11,10) computes it.
func oibKey(digits string) byte {
	p := 10
	for _, c := range digits {
		s := (int(c-'0') + p) % 10
		if s == 0 {
			s = 10
		}
		p = s * 2 % 11
	}
	return byte('0' + (11-p)%10)
}

// TestFunctions checks the functions of a schematron (xsl:function): the
// conversion of their parameters and result, their variables, the absence
// of a context item in their body, and the OIB check of the Croatian CIUS
// against ISO 7064.
func TestFunctions(t *testing.T) {
	ns := map[string]string{"u": "utils", "xs": "http://www.w3.org/2001/XMLSchema"}
	var oibs []string
	for i := range 300 {
		ten := fmt.Sprintf("%010d", (i*7919*104729+12345)%10000000000)
		good := ten + string(oibKey(ten))
		bad := ten + string('0'+(oibKey(ten)-'0'+1)%10)
		oibs = append(oibs, good, bad)
	}
	var doc strings.Builder
	doc.WriteString("<r>")
	for _, o := range oibs {
		doc.WriteString("<o>" + o + "</o>")
	}
	doc.WriteString("<o>HR" + oibs[0] + "</o><o>1234567890</o><o>HR1234567890X</o><o></o><q>1.12345678901</q><q>123.1234567890</q><q>" + strings.Repeat("9", 31) + "</q></r>")
	root := parse(t, doc.String())
	d := newDoc(root)
	user, err := compileFunctions(ns, []Function{ctrlOIB, check3010,
		{Name: "u:ctx", Result: "string(.)"}, {Name: "u:str", As: "xs:string", Params: []Param{{Name: "s", As: "xs:string*"}}, Result: "$s[1]"}})
	if err != nil {
		t.Fatal(err)
	}
	eval := func(src string, n *xmltree.Node) (bool, error) {
		e, err := compileWith(src, ns, user)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return ebvOf(e, &ctx{d: d, it: item{k: kNode, n: n}, pos: 1, size: 1, vars: &binding{name: "x", val: seq{{k: kString, s: "caller"}}}})
	}
	for i, o := range root.Children[:len(oibs)] {
		got, err := eval("u:ctrlOIB(.)", o)
		if want := i%2 == 0; err != nil || got != want {
			t.Errorf("u:ctrlOIB(%s) = %v, %v; want %v", o.Text, got, err, want)
		}
	}
	for _, c := range []struct {
		src  string
		want bool
	}{
		{"u:ctrlOIB(o[" + fmt.Sprint(len(oibs)+1) + "])", true}, // with HR
		{"u:ctrlOIB(o[" + fmt.Sprint(len(oibs)+2) + "])", false},
		{"u:ctrlOIB(o[" + fmt.Sprint(len(oibs)+3) + "])", false},
		{"u:ctrlOIB(o[" + fmt.Sprint(len(oibs)+4) + "])", false},
		{"u:ctrlOIB(missing)", false},
		{"u:check3010(q[1])", false}, {"u:check3010(q[2])", true}, {"u:check3010(q[3])", false},
		{"u:check3010(missing)", true}, {"u:check3010('12.5')", true},
		{"u:str(('a', 'b')) = 'a'", true},
	} {
		got, err := eval(c.src, root)
		if err != nil || got != c.want {
			t.Errorf("%s = %v, %v; want %v", c.src, got, err, c.want)
		}
	}
	// A parameter of type xs:anyAtomicType? given two items, a function
	// that uses the context item, the caller's variables or an argument
	// that does not cast raise an error.
	for _, src := range []string{"u:check3010(q)", "u:ctrlOIB(o)", "u:ctx()", "u:str(1)"} {
		if _, err := eval(src, root); err == nil {
			t.Errorf("%s: no error", src)
		}
	}
	for _, f := range []Function{
		{Name: "u:f", Result: "$x"}, {Name: "u:f", As: "xs:integer", Result: "1"}, {Name: "u:f", Result: "u:g()"},
	} {
		if _, err := Compile(ns, []Pattern{{ID: "p", Rules: []Rule{{Context: "r", Asserts: []Assertion{{ID: "R", Test: "u:f()", Msg: "m"}}}}}}, f); err == nil && f.Result != "$x" {
			t.Errorf("%+v: no error", f)
		} else if f.Result == "$x" {
			// An unbound variable compiles and raises an error when used.
			s, _ := Compile(ns, []Pattern{{ID: "p", Rules: []Rule{{Context: "r", Asserts: []Assertion{{ID: "BR-01", Flag: rules.Error, Test: "u:f()", Msg: "m"}}}}}}, f)
			r := &rules.Report{}
			s.Run(&rules.Checker{Doc: &rules.Doc{Root: root}, Report: r}, root, func(string) bool { return true })
			if len(r.Findings) != 1 {
				t.Errorf("unbound variable in a function: %v", r.Findings)
			}
		}
	}
}
