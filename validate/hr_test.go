package validate

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The CIUS-HR invoices of internal/rules/hr, written for its tests: an
// invoice with the Croatian VAT breakdown and every element of the
// CIUS-HR, a credit note with VAT on a cash basis, and the invoice of a
// seller outside the VAT system.
var ciusHRInvoices = []string{
	"../internal/rules/hr/testdata/cius-hr.ubl.xml",
	"../internal/rules/hr/testdata/cius-hr-cn.ubl.xml",
	"../internal/rules/hr/testdata/cius-hr-o.ubl.xml",
}

// TestCIUSHR checks the CIUS-HR invoices of internal/rules/hr with the rule
// sets that apply to them: the Croatian rules apply to a CIUS-HR invoice
// whatever the rule sets, along with the rules of EN 16931. The elements
// that the CIUS-HR adds to the syntax binding (the UBL extension, the
// issue time, the operator…) raise the warnings UBL-CR of EN 16931, and
// nothing else.
func TestCIUSHR(t *testing.T) {
	for _, file := range ciusHRInvoices {
		doc := readXML(t, file)
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Schema | EN16931}, {Rules: Schema}} {
			r := XML([]byte(doc), opts)
			if r.Profile != invoice.ProfileCIUSHR {
				t.Errorf("%s (%+v): profile %s", file, opts, r.Profile)
			}
			for _, f := range r.Findings {
				if f.Source != SourceEN16931 || f.Severity != Warning || !strings.HasPrefix(f.Rule, "UBL-CR-") {
					t.Errorf("%s (%+v): %s", file, opts, f)
				}
			}
		}
	}
}

// TestCIUSHRFindings checks the source and severity of the findings on a
// CIUS-HR invoice: the rules of the schematron of the tax administration,
// and the check of the OIB of the electronic addresses in the Croatia rule
// set.
func TestCIUSHRFindings(t *testing.T) {
	for _, c := range []struct {
		file     int
		old, new string
		opts     *Options
		rule     string
		severity Severity
	}{
		// HR-BR-37: the operator label (HR-BT-4).
		{0, "<cbc:Name>Operater 3</cbc:Name>", "", nil, "HR-BR-37", Error},
		{0, "<cbc:Name>Operater 3</cbc:Name>", "", &Options{Rules: Schema}, "HR-BR-37", Error},
		// HR-BR-2: the issue time (HR-BT-2) is written hh:mm:ss.
		{1, "<cbc:IssueTime>14:05:00</cbc:IssueTime>", "<cbc:IssueTime>14:05</cbc:IssueTime>", nil, "HR-BR-2", Error},
		// HR-BR-34: the business process (BT-23) is P1 to P12 or P99:….
		{2, "<cbc:ProfileID>P3</cbc:ProfileID>", "<cbc:ProfileID>P13</cbc:ProfileID>", nil, "HR-BR-34", Error},
		// The check digit of the OIB of the buyer's electronic address
		// (BT-49) is wrong.
		{2, `<cbc:EndpointID schemeID="9934">12017830464</cbc:EndpointID>`, `<cbc:EndpointID schemeID="9934">12017830465</cbc:EndpointID>`, nil, "HR-OIB-01", Warning},
	} {
		doc := readXML(t, ciusHRInvoices[c.file])
		if !strings.Contains(doc, c.old) {
			t.Fatalf("%q not found", c.old)
		}
		r := XML([]byte(strings.Replace(doc, c.old, c.new, 1)), c.opts)
		f := findings(r, "")
		f = slices.DeleteFunc(f, func(x Finding) bool { return x.Source != SourceHR })
		if len(f) != 1 || f[0].Rule != c.rule || f[0].Severity != c.severity {
			t.Errorf("%q → %q (%+v): want %s\n%s", c.old, c.new, c.opts, c.rule, r)
		}
	}
}

// TestCroatiaOption checks that the Croatia rule set applies to an invoice
// that does not declare the CIUS-HR when it is selected, and not otherwise:
// the French invoice is a CII invoice, which the CIUS-HR does not define.
func TestCroatiaOption(t *testing.T) {
	doc := []byte(readXML(t, frenchInvoice))
	for _, c := range []struct {
		opts *Options
		want bool
	}{
		{nil, false},
		{&Options{Auto: true}, false},
		{&Options{Rules: Default | Romania}, false},
		{&Options{Rules: Default | Croatia}, true},
	} {
		r := XML(doc, c.opts)
		if got := slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.Rule == "HR-SYNTAX-01" }); got != c.want {
			t.Errorf("%+v: HR-SYNTAX-01 reported %v, want %v\n%s", c.opts, got, c.want, r)
		}
	}
}

// TestCatalogCroatia checks that the Croatia rule set lists the rules of
// the CIUS-HR once each, with the checks of the package.
func TestCatalogCroatia(t *testing.T) {
	ids := map[string]int{}
	for _, r := range Catalog(Croatia) {
		ids[r.ID]++
	}
	for id, n := range ids {
		if n != 1 {
			t.Errorf("Catalog(Croatia) lists %s %d times", id, n)
		}
	}
	for _, id := range []string{"HR-BR-1", "HR-BR-9", "HR-BR-34", "HR-BR-CL-1", "HR-SYNTAX-01", "HR-OIB-01"} {
		if ids[id] != 1 {
			t.Errorf("Catalog(Croatia) lacks %s", id)
		}
	}
	if len(ids) != 64 {
		t.Errorf("Catalog(Croatia): %d rules, want 64", len(ids))
	}
}

// TestCIUSHRExamples checks the examples of invoices of the tax
// administration (FX_HR, the archive "Primjeri eRacuna.zip" of
// conformance/fetch.sh; not copied into the repository, as the
// schematron): they break the same Croatian rules as the schematron of the
// tax administration compiled by SchXslt and run by Saxon-HE (the parity
// job checks them). They are dated 2025, before the CIUS-HR applies
// (HR-BR-40), and their OIB are invented: the OIB of the operator
// (HR-BR-9) and the VAT identifiers in Croatia (HR-BR-53) are errors, the
// electronic addresses warn (HR-OIB-01). An item of the leasing invoice
// has no code of the KPD (HR-BR-25).
func TestCIUSHRExamples(t *testing.T) {
	name := os.Getenv("FX_HR")
	if name == "" {
		t.Skip("FX_HR not set (conformance/fetch.sh)")
	}
	common := []string{"HR-BR-40", "HR-BR-53", "HR-BR-9"}
	want := map[string][]string{
		"Neoporezivi racun.xml":                    {"HR-BR-40", "HR-BR-9"},
		"Racun za leasing PDV PPMV-aktivacija.xml": {"HR-BR-25", "HR-BR-40", "HR-BR-53", "HR-BR-9"},
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
		base := path.Base(f.Name)
		r := XML(data, nil)
		if r.Profile != invoice.ProfileCIUSHR {
			t.Errorf("%s: profile %s", base, r.Profile)
		}
		var got []string
		oib := false
		for _, x := range r.Findings {
			switch {
			case x.Source != SourceHR:
			case x.Rule == "HR-OIB-01":
				oib = true
				if x.Severity != Warning {
					t.Errorf("%s: %s", base, x)
				}
			case x.Severity != Error:
				t.Errorf("%s: %s", base, x)
			case !slices.Contains(got, x.Rule):
				got = append(got, x.Rule)
			}
		}
		slices.Sort(got)
		w, ok := want[base]
		if !ok {
			w = common
		}
		if !slices.Equal(got, w) || !oib {
			t.Errorf("%s: Croatian rules %v (HR-OIB-01 %v), want %v", base, got, oib, w)
		}
	}
	if n != 20 {
		t.Errorf("%d examples, want 20", n)
	}
}
