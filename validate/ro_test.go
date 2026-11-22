package validate

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The CIUS-RO invoices of internal/rules/ro, written for its tests: a UBL
// invoice, a UBL credit note and the CII invoice that cii.Marshal writes
// from the first one.
var ciusROInvoices = []string{
	"../internal/rules/ro/testdata/cius-ro.ubl.xml",
	"../internal/rules/ro/testdata/cius-ro-cn.ubl.xml",
	"../internal/rules/ro/testdata/cius-ro.cii.xml",
}

// TestCIUSRO checks the CIUS-RO invoices of internal/rules/ro with the rule
// sets that apply to them: the Romanian rules apply to a CIUS-RO invoice
// whatever the rule sets, along with the rules of EN 16931.
func TestCIUSRO(t *testing.T) {
	for _, file := range ciusROInvoices {
		doc := readXML(t, file)
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Schema | EN16931}, {Rules: Schema}} {
			r := XML([]byte(doc), opts)
			if r.Profile != invoice.ProfileCIUSRO || len(r.Findings) > 0 {
				t.Errorf("%s (%+v): profile %s\n%s", file, opts, r.Profile, r)
			}
		}
	}
}

// TestCIUSROFindings checks the source and severity of the findings on a
// CIUS-RO invoice: the rules of the schematron of ANAF, or of its
// transposition to CII, and the check of the CIF in the Romania rule set.
func TestCIUSROFindings(t *testing.T) {
	for _, c := range []struct {
		file     int
		old, new string
		opts     *Options
		rule     string
		severity Severity
	}{
		// BR-RO-110: the county of a seller in Romania (BT-39) is a code of
		// ISO 3166-2:RO.
		{0, "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "<cbc:CountrySubentity>Bucuresti</cbc:CountrySubentity>", nil, "BR-RO-110", Error},
		{0, "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "<cbc:CountrySubentity>Bucuresti</cbc:CountrySubentity>", &Options{Rules: Schema}, "BR-RO-110", Error},
		// BR-RO-101: the city of a buyer in Bucharest (BT-52) is a sector.
		{1, "<cbc:CityName>SECTOR1</cbc:CityName>", "<cbc:CityName>Bucuresti</cbc:CityName>", nil, "BR-RO-101", Error},
		// BR-RO-040 in CII: the VAT point date code (BT-8) is 5, 29 or 72
		// (35 is not a code of UNTDID 2475 either: BR-CL-06 of EN 16931).
		{2, "<ram:DueDateTypeCode>29</ram:DueDateTypeCode>", "<ram:DueDateTypeCode>35</ram:DueDateTypeCode>", nil, "BR-RO-040", Error},
		// BR-RO-L1024: the item name (BT-153) has 100 characters at most.
		{2, "<ram:Name>Filtru</ram:Name>", "<ram:Name>" + strings.Repeat("F", 101) + "</ram:Name>", nil, "BR-RO-L1024", Error},
		// The check digit of the buyer's CIF (BT-48) is wrong.
		{0, "<cbc:CompanyID>RO18547290</cbc:CompanyID>", "<cbc:CompanyID>RO18547291</cbc:CompanyID>", nil, "RO-CIF-02", Warning},
		{2, `<ram:ID schemeID="VA">RO18547290</ram:ID>`, `<ram:ID schemeID="VA">RO1854729X</ram:ID>`, nil, "RO-CIF-01", Warning},
	} {
		doc := readXML(t, ciusROInvoices[c.file])
		if !strings.Contains(doc, c.old) {
			t.Fatalf("%q not found", c.old)
		}
		r := XML([]byte(strings.Replace(doc, c.old, c.new, 1)), c.opts)
		f := findings(r, "")
		f = slices.DeleteFunc(f, func(x Finding) bool { return x.Source != SourceRO })
		if len(f) != 1 || f[0].Rule != c.rule || f[0].Severity != c.severity {
			t.Errorf("%q → %q (%+v): want %s\n%s", c.old, c.new, c.opts, c.rule, r)
		}
	}
}

// TestRomaniaOption checks that the Romania rule set applies to an invoice
// that does not declare the CIUS-RO when it is selected, and not otherwise:
// the French invoice has no county code and not the identifier of the
// CIUS-RO.
func TestRomaniaOption(t *testing.T) {
	doc := []byte(readXML(t, frenchInvoice))
	for _, c := range []struct {
		opts *Options
		want bool
	}{
		{nil, false},
		{&Options{Auto: true}, false},
		{&Options{Rules: Default | Italy}, false},
		{&Options{Rules: Default | Romania}, true},
	} {
		r := XML(doc, c.opts)
		if got := slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.Rule == "BR-RO-001" }); got != c.want {
			t.Errorf("%+v: BR-RO-001 reported %v, want %v\n%s", c.opts, got, c.want, r)
		}
	}
}

// TestCatalogRomania checks that the Romania rule set lists the rules of
// the CIUS-RO once each, with the checks of the CIF, and not the rule of
// EN 16931 that its schematron restates.
func TestCatalogRomania(t *testing.T) {
	ids := map[string]int{}
	for _, r := range Catalog(Romania) {
		ids[r.ID]++
	}
	for id, n := range ids {
		if n != 1 {
			t.Errorf("Catalog(Romania) lists %s %d times", id, n)
		}
	}
	for _, id := range []string{"BR-RO-001", "BR-RO-020_2", "BR-RO-L1019", "BR-DEC-RO-1010", "RO-CIF-01", "RO-CIF-02"} {
		if ids[id] != 1 {
			t.Errorf("Catalog(Romania) lacks %s", id)
		}
	}
	if ids["BR-27"] != 0 {
		t.Error("Catalog(Romania) lists BR-27, a rule of EN 16931")
	}
	if len(ids) != 123 {
		t.Errorf("Catalog(Romania): %d rules, want 123", len(ids))
	}
}

// TestCIUSROExamples checks the examples of invoices of ANAF (FX_RO, the
// directory of conformance/fetch.sh; not copied into the repository, as
// the schematron): they declare the CIUS-RO 1.0.0 (BR-RO-001), and some
// break other Romanian rules, the same that the schematron of ANAF compiled
// by SchXslt and run by Saxon-HE raises for the UBL ones (the parity job
// checks them). Their identifiers are invented: the checks of the CIF warn.
func TestCIUSROExamples(t *testing.T) {
	dir := os.Getenv("FX_RO")
	if dir == "" {
		t.Skip("FX_RO not set (conformance/fetch.sh)")
	}
	want := map[string][]string{
		"creditNote_ex.xml":     {"BR-RO-001"},
		"eInvoice_ex.xml":       {"BR-RO-001"},
		"ubl_b2g_example0.xml":  {"BR-RO-001", "BR-RO-111", "BR-RO-170"},
		"ubl_b2g_example1.xml":  {"BR-RO-001", "BR-RO-110", "BR-RO-111"},
		"ubl_b2g_example2.xml":  {"BR-RO-001", "BR-RO-170"},
		"ubl_b2g_example3.xml":  {"BR-RO-001", "BR-RO-030"},
		"ubl_b2g_example4.xml":  {"BR-RO-001", "BR-RO-030"},
		"ubl_b2g_example5.xml":  {"BR-RO-001", "BR-RO-030"},
		"ubl_b2g_example6.xml":  {"BR-RO-001", "BR-RO-111", "BR-RO-170"},
		"ubl_b2g_example7.xml":  {"BR-RO-001", "BR-RO-110", "BR-RO-111"},
		"ubl_b2g_example8.xml":  {"BR-RO-001", "BR-RO-170"},
		"ubl_b2g_example9.xml":  {"BR-RO-001", "BR-RO-030"},
		"ubl_b2g_example10.xml": {"BR-RO-001", "BR-RO-030"},
		// The transposition to CII: BT-6 is not RON (BR-RO-030), the buyer
		// has neither a VAT identifier nor a legal registration identifier
		// (BR-RO-120), a delivery address has no county (BR-RO-211).
		"CII_example0.xml":  {"BR-RO-001", "BR-RO-030"},
		"CII_example1.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120"},
		"CII_example2.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120"},
		"CII_example3.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120"},
		"CII_example4.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120", "BR-RO-211"},
		"CII_example6.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120"},
		"CII_example7.xml":  {"BR-RO-001", "BR-RO-030"},
		"CII_example8.xml":  {"BR-RO-001", "BR-RO-030"},
		"CII_example9.xml":  {"BR-RO-001", "BR-RO-030", "BR-RO-120"},
		"CII_example10.xml": {"BR-RO-001", "BR-RO-030"},
	}
	seen := map[string]bool{}
	for _, archive := range []string{"eFactura_UBL2.1_exempleXML.zip", "exemple_Invoice_CreditNote.zip", "eFactura_CII_exempleXML.zip"} {
		zr, err := zip.OpenReader(filepath.Join(dir, archive))
		if err != nil {
			t.Fatal(err)
		}
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
			name := filepath.Base(f.Name)
			seen[name] = true
			r := XML(data, nil)
			if r.Profile != invoice.ProfileCIUSRO {
				t.Errorf("%s: profile %s", name, r.Profile)
			}
			var got []string
			for _, x := range r.Findings {
				switch {
				case x.Source != SourceRO:
				case strings.HasPrefix(x.Rule, "RO-CIF-"):
					if x.Severity != Warning {
						t.Errorf("%s: %s", name, x)
					}
				case x.Severity != Error:
					t.Errorf("%s: %s", name, x)
				case !slices.Contains(got, x.Rule):
					got = append(got, x.Rule)
				}
			}
			slices.Sort(got)
			if w, ok := want[name]; !ok || !slices.Equal(got, w) {
				t.Errorf("%s: Romanian rules %v, want %v", name, got, w)
			}
		}
		zr.Close()
	}
	if len(seen) != len(want) {
		t.Errorf("%d examples, want %d", len(seen), len(want))
	}
}
