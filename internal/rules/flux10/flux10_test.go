package flux10

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
)

// TestCatalog checks that every expression of the schematron compiles, and
// that Catalog lists each assertion of the schematron once, as an error.
func TestCatalog(t *testing.T) {
	if _, err := compiled(); err != nil {
		t.Fatal(err)
	}
	n := 0
	ids := map[string]assertion{}
	for _, p := range patterns {
		if len(p.Rules) != 1 {
			t.Errorf("pattern %s: %d rules", p.ID, len(p.Rules))
		}
		for _, r := range p.Rules {
			for i, a := range r.Asserts {
				n++
				ids[a.ID] = a
				want := p.ID
				if i > 0 {
					want = p.ID + "-" + strconv.Itoa(i+1)
				}
				if a.ID != want {
					t.Errorf("assertion %d of pattern %s: identifier %s, want %s", i+1, p.ID, a.ID, want)
				}
			}
		}
	}
	if n != assertionCount || assertionCount != 150 || len(patterns) != 96 {
		t.Errorf("%d assertions in %d patterns, want 150 in 96", n, len(patterns))
	}
	listed := map[string]bool{}
	for _, r := range Catalog() {
		if listed[r.ID] {
			t.Errorf("%s listed twice", r.ID)
		}
		listed[r.ID] = true
		a, ok := ids[r.ID]
		if !ok {
			t.Errorf("%s is not a rule of the schematron", r.ID)
			continue
		}
		if r.Severity != rules.Error || a.Flag != rules.Error {
			t.Errorf("%s: severity %v, flag %v, want errors", r.ID, r.Severity, a.Flag)
		}
	}
	if len(listed) != len(ids) {
		t.Errorf("%d rules in Catalog, %d in the schematron", len(listed), len(ids))
	}
	if Version != "1.4.0.04" {
		t.Errorf("Version = %q", Version)
	}
}

// TestMessages checks that each message opens with the code of annex 7 and
// ends with its source, with as many placeholders as values, and that the
// descriptions of the catalog keep the code but neither a placeholder nor
// the source.
func TestMessages(t *testing.T) {
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				if n := strings.Count(a.Msg, "%s"); n != len(a.Values) {
					t.Errorf("%s: %d placeholders, %d values in %q", a.ID, n, len(a.Values), a.Msg)
				}
				code, _, ok := strings.Cut(strings.TrimPrefix(a.Msg, "["), "] ")
				if !ok || !strings.HasPrefix(a.Msg, "[") || !strings.HasSuffix(a.Msg, " | Source : Annexe 7 v1.8 "+code) {
					t.Errorf("%s: %q", a.ID, a.Msg)
				}
			}
		}
	}
	for _, r := range Catalog() {
		if strings.Contains(r.Description, "%") || strings.Contains(r.Description, "Source") ||
			!strings.HasPrefix(r.Description, "[") || !strings.HasSuffix(r.Description, ".") {
			t.Errorf("%s: %q", r.ID, r.Description)
		}
	}
}

// TestExamples checks that the reports of testdata raise no rule, and that
// the findings of a report that breaks rules carry the source of the
// checker, their path, their line and the values of their message.
func TestExamples(t *testing.T) {
	for _, b := range []string{"transactions", "payments"} {
		if got := findings(t, load(t, b)); len(got) > 0 {
			t.Errorf("%s: %v", b, got)
		}
	}
	doc := apply(t, load(t, "transactions"), edit{1, "<TaxExclusiveAmount>950.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>950.5</TaxExclusiveAmount>"})
	doc = apply(t, doc, edit{1, "<Amount>50.00</Amount>", "<Amount>5.000</Amount>"})
	r := check(t, doc)
	if len(r.Findings) != 2 {
		t.Fatalf("findings: %s", r)
	}
	for _, f := range r.Findings {
		if f.Source != rules.SourceFlux10 || f.Line == 0 || !strings.HasPrefix(f.Path, "/Report/TransactionsReport/Invoice") {
			t.Errorf("finding %+v", f)
		}
	}
	if f := r.Findings[0]; f.Rule != "F10-REMISE-G1.14" || f.Message != "[G1.14] Amount : doit comporter au maximum 2 décimales avec un séparateur point. | Source : Annexe 7 v1.8 G1.14" {
		t.Errorf("finding %+v", f)
	}
	if f := r.Findings[1]; f.Rule != "F10-TOTAL-G1.53" || !strings.HasPrefix(f.Message, "[G1.53] Le montant total hors taxe (950.5) doit être égal à la somme des bases d'imposition TVA (950), ") {
		t.Errorf("finding %+v", f)
	}
	Check(&rules.Checker{Report: &rules.Report{}}, nil)
}
