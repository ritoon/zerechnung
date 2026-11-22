package validate

import (
	"archive/zip"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The CIUS-PT invoices of internal/rules/pt, written for its tests.
var ciusPTInvoices = []string{
	"../internal/rules/pt/testdata/cius-pt.ubl.xml",
	"../internal/rules/pt/testdata/cius-pt-nc.ubl.xml",
	"../internal/rules/pt/testdata/cius-pt-rich.ubl.xml",
}

// TestCIUSPT checks the CIUS-PT invoices of internal/rules/pt with the rule
// sets that apply to them: the Portuguese rules apply to a CIUS-PT invoice
// whatever the rule sets, and its schematron replaces the CEN artefacts in
// the EN 16931 rule set, whose code lists refuse the Portuguese document
// type FT and VAT category RED of these invoices.
func TestCIUSPT(t *testing.T) {
	for _, file := range ciusPTInvoices {
		doc := readXML(t, file)
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Schema | EN16931}, {Rules: Schema}} {
			r := XML([]byte(doc), opts)
			if r.Profile != invoice.ProfileCIUSPT || len(r.Findings) > 0 {
				t.Errorf("%s (%+v): profile %s\n%s", file, opts, r.Profile, r)
			}
		}
	}
}

// TestCIUSPTFindings checks the source and severity of the findings on a
// CIUS-PT invoice: its own rules and the check of the NIF in the Portugal
// rule set, the rules of EN 16931 of its schematron in the EN 16931 rule
// set.
func TestCIUSPTFindings(t *testing.T) {
	doc := readXML(t, ciusPTInvoices[0])
	for _, c := range []struct {
		old, new string
		opts     *Options
		rule     string
		source   string
		severity Severity
	}{
		// BR-CIUS-PT-05: the seller's street (BT-35) is mandatory.
		{"<cbc:StreetName>Rua das Flores 12</cbc:StreetName>", "", nil, "BR-CIUS-PT-05", SourcePT, Error},
		{"<cbc:StreetName>Rua das Flores 12</cbc:StreetName>", "", &Options{Rules: Schema}, "BR-CIUS-PT-05", SourcePT, Error},
		// BR-25: the item name (BT-153) is mandatory, in the version of the
		// CIUS-PT schematron.
		{"<cbc:Name>Manutenção de equipamento</cbc:Name>", "", nil, "BR-25", SourceEN16931, Error},
		{"<cbc:Name>Manutenção de equipamento</cbc:Name>", "", &Options{Rules: Schema}, "", "", 0},
		// The check digit of the buyer's NIF (BT-48) is wrong.
		{"<cbc:CompanyID>PT600012344</cbc:CompanyID>", "<cbc:CompanyID>PT600012345</cbc:CompanyID>", nil, "PT-NIF-02", SourcePT, Warning},
	} {
		if !strings.Contains(doc, c.old) {
			t.Fatalf("%q not found", c.old)
		}
		r := XML([]byte(strings.Replace(doc, c.old, c.new, 1)), c.opts)
		var want []Finding
		if c.rule != "" {
			want = []Finding{{Rule: c.rule, Source: c.source, Severity: c.severity}}
		}
		if len(r.Findings) != len(want) ||
			len(want) == 1 && (r.Findings[0].Rule != c.rule || r.Findings[0].Source != c.source || r.Findings[0].Severity != c.severity) {
			t.Errorf("%q → %q (%+v): want %s\n%s", c.old, c.new, c.opts, c.rule, r)
		}
	}
}

// TestCIUSPTCII checks that a CII invoice that declares the CIUS-PT gets
// PT-SYNTAX-01: the CIUS-PT has no CII binding.
func TestCIUSPTCII(t *testing.T) {
	r := XML(withProfile(t, readXML(t, frenchInvoice), invoice.ProfileCIUSPT), nil)
	f := findings(r, "PT-")
	if r.Profile != invoice.ProfileCIUSPT || len(f) != 1 || f[0].Rule != "PT-SYNTAX-01" || f[0].Source != SourcePT || f[0].Severity != Error {
		t.Errorf("profile %s\n%s", r.Profile, r)
	}
}

// TestCatalogPortugal checks that the Portugal rule set lists the rules of
// the CIUS-PT once each, with those of its schematron that the CEN
// artefacts do not define, and not the others.
func TestCatalogPortugal(t *testing.T) {
	ids := map[string]int{}
	for _, r := range Catalog(Portugal) {
		ids[r.ID]++
	}
	for id, n := range ids {
		if n != 1 {
			t.Errorf("Catalog(Portugal) lists %s %d times", id, n)
		}
	}
	for _, id := range []string{"BR-CIUS-PT-01", "DT-CIUS-PT-001", "PT-SYNTAX-01", "PT-NIF-01", "PT-NIF-02", "BR-AA-01"} {
		if ids[id] != 1 {
			t.Errorf("Catalog(Portugal) lacks %s", id)
		}
	}
	for _, id := range []string{"BR-01", "BR-S-01", "UBL-CR-001"} {
		if ids[id] != 0 {
			t.Errorf("Catalog(Portugal) lists %s, a rule of the CEN artefacts", id)
		}
	}
}

// TestCIUSPTExamples checks the examples of eSPap (FX_PT, the archive of
// conformance/fetch.sh; their terms of use are not stated, so they are not
// copied into the repository): no error, and no warning but PT-NIF-02 on
// the invented NIFs of the buyer and of the tax representative.
func TestCIUSPTExamples(t *testing.T) {
	name := os.Getenv("FX_PT")
	if name == "" {
		t.Skip("FX_PT not set (conformance/fetch.sh)")
	}
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		n++
		r := XML(data, nil)
		if r.Profile != invoice.ProfileCIUSPT {
			t.Errorf("%s: profile %s", f.Name, r.Profile)
		}
		for _, x := range r.Findings {
			if x.Rule != "PT-NIF-02" || x.Severity != Warning {
				t.Errorf("%s: %s", f.Name, x)
			}
		}
	}
	if n != 15 {
		t.Errorf("%d examples, want 15", n)
	}
}
