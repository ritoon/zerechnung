package peppol

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// TestOfficial runs the CII unit tests of the Peppol BIS Billing 3.0
// repository (FX_PEPPOL, the root of a clone of OpenPEPPOL/peppol-bis-invoice-3
// at release 3.0.21 or later): the folders rules/unit-CII-*, which the
// official build runs against PEPPOL-EN16931-CII.sch.
//
// Each test of a test set gives the expected outcome of some rules:
// <success>id</success> when the rule must not fire, <error>id</error> or
// <warning>id</warning> when it fires once (or number times) with that
// severity. Rules not listed are not checked.
func TestOfficial(t *testing.T) {
	dir := os.Getenv("FX_PEPPOL")
	if dir == "" {
		t.Skip("FX_PEPPOL is not set")
	}
	folders, err := filepath.Glob(filepath.Join(dir, "rules", "unit-CII-*"))
	if err != nil {
		t.Fatal(err)
	}
	tests, sets := 0, 0
	covered := map[string]bool{}
	for _, folder := range folders {
		files, _ := filepath.Glob(filepath.Join(folder, "*.xml")) // as the build: .xml only
		for _, file := range files {
			sets++
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			set, err := xmltree.Parse(data)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			for i, test := range set.Elems("test") {
				tests++
				name := filepath.Base(folder) + "/" + filepath.Base(file) + "#" + strconv.Itoa(i+1)
				root := test.Elem("CrossIndustryInvoice")
				if root == nil {
					t.Errorf("%s: no CrossIndustryInvoice", name)
					continue
				}
				root.Parent = nil
				r := &rules.Report{}
				Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r, Source: rules.SourcePeppol})
				checkFindings(t, r)
				for _, a := range test.Elem("assert").Children {
					id := strings.TrimSpace(a.Text)
					covered[id] = true
					if byID()[id] == nil {
						t.Errorf("%s: rule %s is not in Catalog", name, id)
					}
					want, sev := 1, rules.Severity(0)
					switch a.Name {
					case "success":
						want = 0
					case "error":
						sev = rules.Error
					case "warning":
						sev = rules.Warning
					default:
						continue
					}
					if n := a.Attr("number"); n != "" {
						want, _ = strconv.Atoi(n)
					}
					got := 0
					for _, f := range r.Findings {
						if f.Rule == id && (sev == 0 || f.Severity == sev) {
							got++
						}
					}
					if got != want {
						t.Errorf("%s: %s %s fired %d times, want %d\n%s", name, a.Name, id, got, want, r)
					}
				}
			}
		}
	}
	t.Logf("%d test sets, %d tests, %d rules covered", sets, tests, len(covered))
	if tests == 0 {
		t.Error("no unit test in FX_PEPPOL")
	}
	for _, r := range Catalog {
		if !covered[r.ID] {
			t.Logf("no official unit test for %s", r.ID)
		}
	}
}

// TestOfficialUBL runs the UBL unit tests of the Peppol BIS Billing 3.0
// repository (FX_PEPPOL): the folders rules/unit-UBL-*, which the official
// build runs against PEPPOL-EN16931-UBL.sch. The expectations are read as
// for TestOfficial, except that the number of a <success> is not a count:
// the rule must not fire. Every assertion of UBLCatalog must fail in an
// official test or in a mutation case (TestUBLMutations).
func TestOfficialUBL(t *testing.T) {
	dir := os.Getenv("FX_PEPPOL")
	if dir == "" {
		t.Skip("FX_PEPPOL is not set")
	}
	folders, err := filepath.Glob(filepath.Join(dir, "rules", "unit-UBL-*"))
	if err != nil {
		t.Fatal(err)
	}
	tests, sets, asserts := 0, 0, 0
	covered, failing := map[string]bool{}, map[string]bool{}
	for _, folder := range folders {
		files, _ := filepath.Glob(filepath.Join(folder, "*.xml")) // as the build: .xml only
		for _, file := range files {
			sets++
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			set, err := xmltree.Parse(data)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			for i, test := range set.Elems("test") {
				tests++
				name := filepath.Base(folder) + "/" + filepath.Base(file) + "#" + strconv.Itoa(i+1)
				// The document is an Invoice or a CreditNote, or for some
				// identifier rules an Order: the contexts of these rules do
				// not depend on the root.
				var root *xmltree.Node
				for _, c := range test.Children {
					if c.Name != "assert" {
						root = c
						break
					}
				}
				if root == nil {
					t.Errorf("%s: no document", name)
					continue
				}
				root.Parent = nil
				r := checkUBLDoc(root)
				checkUBLFindings(t, r)
				for _, a := range test.Elem("assert").Children {
					id := strings.TrimSpace(a.Text)
					want, sev := 1, rules.Severity(0)
					switch a.Name {
					case "success":
						want = 0
					case "error":
						sev = rules.Error
					case "warning":
						sev = rules.Warning
					default:
						continue
					}
					asserts++
					covered[id] = true
					if ublByID()[id] == nil {
						t.Errorf("%s: rule %s is not in UBLCatalog", name, id)
					}
					if n := a.Attr("number"); n != "" && a.Name != "success" {
						want, _ = strconv.Atoi(n)
					}
					failing[id] = failing[id] || want > 0
					got := 0
					for _, f := range r.Findings {
						if f.Rule == id && (sev == 0 || f.Severity == sev) {
							got++
						}
					}
					if got != want {
						t.Errorf("%s: %s %s fired %d times, want %d\n%s", name, a.Name, id, got, want, r)
					}
				}
			}
		}
	}
	t.Logf("%d test sets, %d tests, %d assertions, %d rules covered", sets, tests, asserts, len(covered))
	if tests == 0 {
		t.Error("no unit test in FX_PEPPOL")
	}
	for _, c := range ublMutationCases() {
		for _, id := range c.want {
			failing[id] = true
		}
	}
	for _, r := range UBLCatalog {
		if !failing[r.ID] {
			t.Errorf("neither an official unit test nor a mutation case fails %s", r.ID)
		}
	}
}

// checkUBLDoc runs the Peppol rules on a UBL document.
func checkUBLDoc(root *xmltree.Node) *rules.Report {
	r := &rules.Report{}
	Check(&rules.Checker{Doc: &rules.Doc{UBL: root}, Report: r, Source: rules.SourcePeppol})
	return r
}

// checkUBLFindings verifies that every finding is in UBLCatalog, with its
// severity, and located.
func checkUBLFindings(t *testing.T, r *rules.Report) {
	t.Helper()
	for _, f := range r.Findings {
		info, ok := ublByID()[f.Rule]
		if !ok {
			t.Errorf("rule %s is not in UBLCatalog", f.Rule)
			continue
		}
		if f.Severity != info.Severity || f.Source != rules.SourcePeppol || f.Message == "" || f.Path == "" ||
			!strings.HasPrefix(f.Message, "["+f.Rule+"]-"+info.Description) {
			t.Errorf("bad finding %+v", f)
		}
	}
}
