package schematron

import (
	"slices"
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
	e, err := compileContext("a[normalize-space(b) = 'x']", testNS)
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
// an error.
func TestRun(t *testing.T) {
	root := parse(t, `<r><a>5</a><a>2</a><b>x</b><b>y</b></r>`)
	s, err := Compile(testNS, []Pattern{{
		ID:   "p",
		Lets: []Let{{Name: "max", Value: "1"}},
		Rules: []Rule{
			// The first rule hides a[1] from the next ones, although R-1
			// is not kept.
			{Context: "a[. = '5']", Asserts: []Assertion{{ID: "R-1", Flag: rules.Error, Test: "false()", Msg: "first rule"}}},
			{Context: "a", Lets: []Let{{Name: "v", Value: "xs:decimal(.)"}}, Asserts: []Assertion{
				{ID: "R-2", Flag: rules.Error, Test: "$v <= $max", Msg: "%s is %s, above 1 (100%%)", Values: []string{"name()", "."}},
				{ID: "R-3", Flag: rules.Warning, Report: true, Test: "$v = 2", Msg: "two"},
				{ID: "R-4", Flag: rules.Error, Test: "true()", Msg: "never"},
			}},
			{Context: "/r", Asserts: []Assertion{{ID: "R-5", Flag: rules.Error, Test: "normalize-space(b) = 'x'", Msg: "error"}}},
			{Context: "a", Asserts: []Assertion{{ID: "R-6", Flag: rules.Error, Test: "false()", Msg: "hidden"}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	s.Run(&rules.Checker{Doc: &rules.Doc{Root: root}, Report: r}, root, func(id string) bool { return id != "R-1" })
	var got []string
	for _, f := range r.Findings {
		got = append(got, f.Rule+" "+f.Severity.String()+" "+f.Path+" "+f.Message)
	}
	want := []string{
		"R-5 error /r error",
		"R-2 error /r/a[2] a is 2, above 1 (100%)",
		"R-3 warning /r/a[2] two",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A message whose verbs do not match its values does not compile.
	if _, err := Compile(testNS, []Pattern{{ID: "p", Rules: []Rule{{Context: "a", Asserts: []Assertion{{ID: "R", Test: "true()", Msg: "%s", Values: []string{".", "."}}}}}}}); err == nil {
		t.Error("message with 1 verb and 2 values: no error")
	}
}
