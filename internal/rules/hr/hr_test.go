package hr

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

func parse(t testing.TB, doc string) *xmltree.Node {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// check returns the findings of Check for the UBL document doc.
func check(t testing.TB, doc string) []rules.Finding {
	t.Helper()
	root := parse(t, doc)
	view := parse(t, `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`)
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewUBLDoc(root, view, nil, invoice.ProfileCIUSHR), Report: r})
	return r.Findings
}

// TestCatalog checks that every expression of the schematron compiles, and
// that Catalog lists each rule of the schematron once, with its flag, then
// the checks of the package.
func TestCatalog(t *testing.T) {
	if _, err := compiled(); err != nil {
		t.Fatal(err)
	}
	n := 0
	ids := map[string]assertion{}
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				n++
				ids[a.ID] = a
			}
		}
	}
	if n != assertionCount {
		t.Errorf("%d assertions, want %d", n, assertionCount)
	}
	listed := map[string]bool{}
	for _, r := range Catalog() {
		if listed[r.ID] {
			t.Errorf("%s listed twice", r.ID)
		}
		listed[r.ID] = true
		if r.Description == "" {
			t.Errorf("%s has no description", r.ID)
		}
		switch r.ID {
		case RuleSyntax:
			if r.Severity != rules.Error {
				t.Errorf("%s: severity %v, want error", r.ID, r.Severity)
			}
			continue
		case RuleAddressOIB:
			if r.Severity != rules.Warning {
				t.Errorf("%s: severity %v, want warning", r.ID, r.Severity)
			}
			continue
		}
		a, ok := ids[r.ID]
		if !ok {
			t.Errorf("%s is not a rule of the schematron", r.ID)
			continue
		}
		if r.Severity != a.Flag || a.Flag != rules.Error {
			t.Errorf("%s: severity %v, flag %v", r.ID, r.Severity, a.Flag)
		}
	}
	for id := range ids {
		if !listed[id] {
			t.Errorf("%s is not in Catalog", id)
		}
	}
	if got, want := len(Catalog()), len(ids)+2; got != want {
		t.Errorf("%d rules in Catalog, want %d", got, want)
	}
}

// TestMessages checks that no description of the catalog keeps a
// placeholder of a value-of element or a line number, and that each message
// with values has as many placeholders.
func TestMessages(t *testing.T) {
	for _, r := range Catalog() {
		if strings.Contains(r.Description, "%") || strings.HasPrefix(r.Description, "Line") {
			t.Errorf("%s: %q", r.ID, r.Description)
		}
	}
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				if n := strings.Count(a.Msg, "%s"); n != len(a.Values) {
					t.Errorf("%s: %d placeholders, %d values in %q", a.ID, n, len(a.Values), a.Msg)
				}
			}
		}
	}
}

// TestBases checks that the base documents meet the rules of the CIUS-HR
// and the check of the electronic addresses.
func TestBases(t *testing.T) {
	for _, b := range bases {
		doc := load(t, b.file)
		if got := findings(t, doc); len(got) > 0 {
			t.Errorf("%s: %v", b.file, got)
		}
		if got := check(t, doc); len(got) > 0 {
			t.Errorf("%s: %v", b.file, got)
		}
	}
}

// TestCodes checks that the schematron lists the HR VAT category labels of
// table HR-TB-2 (HR-BR-CL-1), and accepts as business process types those
// of table HR-TB-1 (HR-BR-34), as package codes gives them.
func TestCodes(t *testing.T) {
	var cl1 []string
	re := regexp.MustCompile(`contains\(' ([^']*) '`)
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				if a.ID == "HR-BR-CL-1" {
					cl1 = strings.Fields(re.FindStringSubmatch(a.Test)[1])
				}
			}
		}
	}
	slices.Sort(cl1)
	var labels []string
	for _, r := range codes.CroatianVATLabels {
		labels = append(labels, r.Label)
	}
	if !slices.Equal(cl1, labels) {
		t.Errorf("HR-BR-CL-1 lists %q, table HR-TB-2 %q", cl1, labels)
	}
	base := load(t, baseInvoice)
	for _, p := range []string{"P0", "P1", "P2", "P9", "P10", "P11", "P12", "P13", "P19", "P01", "p1", "P99", "P99:", "P99:Kupac", "X1"} {
		got := findings(t, strings.Replace(base, "<cbc:ProfileID>P1</cbc:ProfileID>", "<cbc:ProfileID>"+p+"</cbc:ProfileID>", 1))
		if raised := slices.Contains(got, "HR-BR-34"); raised == codes.CroatianProcess(p) {
			t.Errorf("ProfileID %q: HR-BR-34 raised %v, CroatianProcess %v", p, raised, codes.CroatianProcess(p))
		}
	}
	for _, r := range codes.CroatianVATLabels {
		got := findings(t, strings.Replace(base, "<cbc:Name>HR:PDV25</cbc:Name>", "<cbc:Name>"+r.Label+"</cbc:Name>", 1))
		if slices.Contains(got, "HR-BR-CL-1") {
			t.Errorf("%s: HR-BR-CL-1 raised", r.Label)
		}
	}
}

// TestOIBCheckDigit checks ValidOIB against the function u:ctrlOIB of the
// schematron, through HR-BR-9 on the OIB of the operator (HR-BT-5): the
// OIB of the official examples and of the base documents, each with its
// ten other check digits, and random numbers with and without the prefix
// HR, which the function accepts.
func TestOIBCheckDigit(t *testing.T) {
	valid := []string{"69152712380", "47803662159", "82031549070", "33559012720", "12017830464", "11111111119", "00000000001"}
	var tries []string
	for _, v := range valid {
		if !ValidOIB(v) {
			t.Errorf("ValidOIB(%q) = false", v)
		}
		for d := '0'; d <= '9'; d++ {
			tries = append(tries, v[:10]+string(d))
		}
	}
	r := rand.New(rand.NewPCG(1, 2))
	for range 300 {
		v := fmt.Sprintf("%011d", r.Int64N(1e11))
		tries = append(tries, v, "HR"+v, v[:10]+string(oibCheckDigit(v[:10])))
	}
	tries = append(tries, "6915271238", "691527123800", "6915271238X", "HR", "", "hr69152712380")
	base := load(t, baseInvoice)
	old := "<cbc:ID>82031549070</cbc:ID>"
	if !strings.Contains(base, old) {
		t.Fatalf("%s: no %s", baseInvoice, old)
	}
	for _, v := range tries {
		got := findings(t, strings.Replace(base, old, "<cbc:ID>"+v+"</cbc:ID>", 1))
		if v == "" {
			continue // an empty element: HR-BR-33 as well
		}
		want := ValidOIB(strings.TrimPrefix(v, "HR"))
		if slices.Contains(got, "HR-BR-9") == want {
			t.Errorf("OIB %q: HR-BR-9 raised %v, ValidOIB %v", v, !want, want)
		}
	}
	if ValidOIB("HR69152712380") || ValidOIB(" 69152712380") {
		t.Error("ValidOIB accepts the prefix HR or white space")
	}
}

// TestCheck checks that Check reports HR-SYNTAX-01 for a CII invoice and
// HR-OIB-01 for an electronic address of scheme 9934 that is not an OIB.
func TestCheck(t *testing.T) {
	root := parse(t, `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`)
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileCIUSHR), Report: r})
	if got := ruleIDs(r); !slices.Equal(got, []string{RuleSyntax}) || r.Findings[0].Severity != rules.Error {
		t.Errorf("CII: %v", r.Findings)
	}
	base := load(t, baseInvoice)
	for _, c := range []struct {
		old, new string
		want     []string
	}{
		{`<cbc:EndpointID schemeID="9934">47803662159</cbc:EndpointID>`, `<cbc:EndpointID schemeID="9934">47803662158</cbc:EndpointID>`, []string{RuleAddressOIB}},
		{`<cbc:EndpointID schemeID="9934">47803662159</cbc:EndpointID>`, `<cbc:EndpointID schemeID="9934">HR47803662159</cbc:EndpointID>`, []string{RuleAddressOIB}},
		{`<cbc:EndpointID schemeID="9934">47803662159</cbc:EndpointID>`, `<cbc:EndpointID schemeID="0088">4780366215</cbc:EndpointID>`, nil},
		{`<cbc:EndpointID schemeID="9934">69152712380</cbc:EndpointID>`, `<cbc:EndpointID schemeID=" 9934 ">69152712381</cbc:EndpointID>`, []string{RuleAddressOIB}},
	} {
		if !strings.Contains(base, c.old) {
			t.Fatalf("%s: no %s", baseInvoice, c.old)
		}
		fs := check(t, strings.Replace(base, c.old, c.new, 1))
		var got []string
		for _, f := range fs {
			got = append(got, f.Rule)
			if f.Rule == RuleAddressOIB && f.Severity != rules.Warning {
				t.Errorf("%s: severity %v", f.Rule, f.Severity)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.new, got, c.want)
		}
	}
}
