package pt

import (
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

// TestCatalogs checks that every expression of the schematron compiles and
// that each of its assertions is listed once, in Catalog when it is a rule
// of the CIUS-PT itself, in CENCatalog otherwise.
func TestCatalogs(t *testing.T) {
	if _, err := compiled(); err != nil {
		t.Fatal(err)
	}
	n := 0
	ids := map[string]bool{}
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				n++
				ids[a.ID] = true
			}
		}
	}
	if n != assertionCount {
		t.Errorf("%d assertions, want %d", n, assertionCount)
	}
	listed := map[string]bool{}
	for name, c := range map[string][]rules.RuleInfo{"Catalog": Catalog(), "CENCatalog": CENCatalog} {
		for _, r := range c {
			if listed[r.ID] {
				t.Errorf("%s: %s listed twice", name, r.ID)
			}
			listed[r.ID] = true
			if strings.HasPrefix(r.ID, "PT-") {
				continue
			}
			if !ids[r.ID] {
				t.Errorf("%s: %s is not an assertion of the schematron", name, r.ID)
			}
			if national(r.ID) != (name == "Catalog") {
				t.Errorf("%s: %s is in the wrong catalog", name, r.ID)
			}
			if r.Description == "" {
				t.Errorf("%s: %s has no description", name, r.ID)
			}
		}
	}
	for id := range ids {
		if !listed[id] {
			t.Errorf("%s is in no catalog", id)
		}
	}
	for _, id := range []string{"PT-SYNTAX-01", "PT-NIF-01", "PT-NIF-02", "BR-CIUS-PT-01", "DT-CIUS-PT-001"} {
		if !slices.ContainsFunc(Catalog(), func(r rules.RuleInfo) bool { return r.ID == id }) {
			t.Errorf("Catalog lacks %s", id)
		}
	}
}

// TestBases checks that the base documents of the mutation cases are valid:
// no rule of the schematron fails (as with Saxon, see TestCases), and their
// NIFs are well formed.
func TestBases(t *testing.T) {
	for _, b := range bases {
		doc := load(t, b.file)
		if got := findings(t, doc); len(got) > 0 {
			t.Errorf("%s: %v", b.file, got)
		}
		root := parse(t, doc)
		r := &rules.Report{}
		checkNIF(&rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r}, root)
		if len(r.Findings) > 0 {
			t.Errorf("%s: %v", b.file, r.Findings)
		}
	}
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
		e, err := compile(src, namespaces)
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
	e, err := compileContext("a[normalize-space(b) = 'x']")
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.eval(top)
	if err != nil || len(got) != 1 || got[0].n != root.Children[1] {
		t.Errorf("context: %v, %v", got, err)
	}
	e, err = compile("//a[normalize-space(b) = 'x']", namespaces)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.eval(top); err == nil {
		t.Error("expression: no error")
	}
}

func TestNIFCheckDigit(t *testing.T) {
	for nif, want := range map[string]byte{
		"501234560": '0', // 11 - 122 mod 11 = 10: 0
		"600012344": '4',
		"503000019": '9',
		"503000000": '0', // 11 - 66 mod 11 = 11: 0
	} {
		if got := nifCheckDigit(nif); got != want {
			t.Errorf("%s: check digit %c, want %c", nif, got, want)
		}
	}
}

// TestCheckNIF checks the warnings on the Portuguese VAT identifiers of the
// seller, the buyer and the tax representative.
func TestCheckNIF(t *testing.T) {
	party := func(role, id string) string {
		return "<cac:" + role + "><cac:Party><cac:PartyTaxScheme><cbc:CompanyID>" + id +
			"</cbc:CompanyID></cac:PartyTaxScheme></cac:Party></cac:" + role + ">"
	}
	doc := `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
		xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
		xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">` +
		party("AccountingSupplierParty", "PT50123456") +
		party("AccountingCustomerParty", "PT600012345") +
		`<cac:TaxRepresentativeParty><cac:PartyTaxScheme><cbc:CompanyID>PT501234560</cbc:CompanyID></cac:PartyTaxScheme></cac:TaxRepresentativeParty>` +
		`</Invoice>`
	root := parse(t, doc)
	r := &rules.Report{}
	checkNIF(&rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r, Source: rules.SourcePT}, root)
	want := []string{
		"PT-NIF-01 /Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID",
		"PT-NIF-02 /Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID",
	}
	var got []string
	for _, f := range r.Findings {
		got = append(got, f.Rule+" "+f.Path)
		if f.Severity != rules.Warning || f.Source != rules.SourcePT {
			t.Errorf("%s: severity %v, source %s", f.Rule, f.Severity, f.Source)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// Foreign VAT identifiers are not checked.
	root = parse(t, strings.NewReplacer("PT50123456", "ES12345678Z", "PT600012345", "FR40303265045").Replace(doc))
	r = &rules.Report{}
	checkNIF(&rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r}, root)
	if len(r.Findings) > 0 {
		t.Errorf("foreign identifiers: %v", r.Findings)
	}
}

// TestCheckCII checks that a CII invoice gets PT-SYNTAX-01 only, and that
// CheckCEN leaves it to the rules of EN 16931.
func TestCheckCII(t *testing.T) {
	root := parse(t, `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`)
	r := &rules.Report{}
	c := &rules.Checker{Doc: &rules.Doc{Root: root}, Report: r}
	CheckCEN(c)
	Check(c)
	if len(r.Findings) != 1 || r.Findings[0].Rule != "PT-SYNTAX-01" || r.Findings[0].Severity != rules.Error {
		t.Errorf("%v", r.Findings)
	}
}
