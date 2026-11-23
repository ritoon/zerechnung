package cdv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
)

// TestCatalog checks that every expression of the schematron compiles, and
// that Catalog lists each rule of the schematron once, with its flag.
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
	if n != assertionCount || len(patterns) != 28 {
		t.Errorf("%d assertions in %d patterns, want %d in 28", n, len(patterns), assertionCount)
	}
	listed := map[string]bool{}
	warnings := 0
	for _, r := range Catalog() {
		if listed[r.ID] {
			t.Errorf("%s listed twice", r.ID)
		}
		listed[r.ID] = true
		if r.Description == "" {
			t.Errorf("%s has no description", r.ID)
		}
		a, ok := ids[r.ID]
		if !ok {
			t.Errorf("%s is not a rule of the schematron", r.ID)
			continue
		}
		if r.Severity != a.Flag {
			t.Errorf("%s: severity %v, flag %v", r.ID, r.Severity, a.Flag)
		}
		if r.Severity == rules.Warning {
			warnings++
		}
	}
	if len(listed) != len(ids) {
		t.Errorf("%d rules in Catalog, %d in the schematron", len(listed), len(ids))
	}
	if warnings != 3 {
		t.Errorf("%d warnings, want 3 (BR-FR-CDV-13_MDT-129-2, BR-FR-CDV-CL-06)", warnings)
	}
	if Version != "1.4.0.04" {
		t.Errorf("Version = %q", Version)
	}
}

// TestMessages checks that no description of the catalog keeps a
// placeholder of a value-of element or the identifier that opens the
// messages of the schematron, and that each message with values has as many
// placeholders.
func TestMessages(t *testing.T) {
	for _, r := range Catalog() {
		if strings.Contains(r.Description, "%") || strings.Contains(r.Description, "[BR-FR") || strings.Contains(r.Description, "Veuillez") {
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

// TestExamples checks that the examples of statuses of testdata raise no
// rule, and that the findings of a status that breaks rules carry the
// source of the checker, their path and the value of the element.
func TestExamples(t *testing.T) {
	paths, err := filepath.Glob(testdata + "*.xml")
	if err != nil || len(paths) < 12 {
		t.Fatalf("%d examples in %s (%v)", len(paths), testdata, err)
	}
	for _, path := range paths {
		if got := findings(t, normalize(readFile(t, path))); len(got) > 0 {
			t.Errorf("%s: %v", filepath.Base(path), got)
		}
	}
	doc := normalize(readFile(t, testdata+bases[3].file))
	doc = strings.Replace(doc, "<ram:ReasonCode>TX_TVA_ERR</ram:ReasonCode>", "<ram:ReasonCode>XXX</ram:ReasonCode>", 1)
	r := check(t, doc)
	if len(r.Findings) != 2 {
		t.Fatalf("findings: %s", r)
	}
	for _, f := range r.Findings {
		if f.Source != rules.SourceCDV || !strings.Contains(f.Path, "ReasonCode") || !strings.Contains(f.Message, "XXX") || f.Line == 0 {
			t.Errorf("finding %+v", f)
		}
	}
	Check(&rules.Checker{Report: &rules.Report{}}, nil)
}

// TestFNFE checks the examples of statuses of the FNFE-MPE package (FX_FNFE,
// a checkout of github.com/fnfempe/France_RFE): none raises a rule.
func TestFNFE(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST", "CDAR", "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 50 {
		t.Fatalf("%d CDAR examples under %s, want at least 50", len(paths), dir)
	}
	for _, path := range paths {
		if got := findings(t, string(readFile(t, path))); len(got) > 0 {
			t.Errorf("%s: %v", filepath.Base(path), got)
		}
	}
	t.Logf("%d examples of statuses without a BR-FR-CDV finding", len(paths))
}
