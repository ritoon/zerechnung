package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

func TestFNFEStylesheet(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{`<rsm:CrossIndustryInvoice xmlns:rsm="` + nsCII + `"/>`, fnfeXSLCII},
		{`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`, fnfeXSLUBL},
		{`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`, fnfeXSLUBL},
		{`<Order xmlns="urn:oasis:names:specification:ubl:schema:xsd:Order-2"/>`, ""},
	} {
		if got := fnfeStylesheet([]byte(tc.doc)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.doc, got, tc.want)
		}
	}
}

func TestOursFR(t *testing.T) {
	for _, tc := range []struct {
		file string
		want map[string]string
	}{
		{"extended-mv.xml", map[string]string{}},
		{"extended-mv.ubl.xml", map[string]string{}},
		{"extended-bd.ubl.xml", map[string]string{"BR-FREXT-S-08ini": levelWarning}},
	} {
		data, err := os.ReadFile(filepath.Join("../..", fnfeRepoDir, tc.file))
		if err != nil {
			t.Fatal(err)
		}
		if !declaresCTCFR(data) {
			t.Errorf("%s: not EXTENDED-CTC-FR", tc.file)
		}
		if r := oursFR(tc.file, data); !reflect.DeepEqual(r.Rules, tc.want) || r.Failure != "" {
			t.Errorf("%s: %+v", tc.file, r)
		}
	}
}

func TestStageFNFE(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	rfe, repo, variants := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, d := range corpusDirs {
		os.MkdirAll(filepath.Join(corpus, d), 0o755)
	}
	os.MkdirAll(filepath.Join(kosit, "src/test"), 0o755)
	cii := `<rsm:CrossIndustryInvoice xmlns:rsm="` + nsCII + `"><!--` + invoice.URNExtendedCTCFR + `--></rsm:CrossIndustryInvoice>`
	ubl := `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`
	write(rfe, fnfeExampleDirs[0]+"/a.xml", cii)
	write(rfe, fnfeExampleDirs[1]+"/b.xml", ubl)
	write(rfe, "FNFE_RFE_INVOICE/Z.example/TEST/CII/EN16931/c.xml", cii)
	write(repo, fnfeRepoDir+"/extended.xml", cii)
	write(repo, fnfeRepoDir+"/basic.xml", `<rsm:CrossIndustryInvoice xmlns:rsm="`+nsCII+`"/>`)
	write(variants, "ubl/ubl-001-x.xml", ubl)
	write(variants, "ubl/expected.tsv", "ubl-001-x.xml\tBR-FREXT-01\n")
	fr := fnfeInputs{rfe: rfe, repo: repo, variants: variants}
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hrInputs{}, fr, cdvInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "fr/fnfe/" + fnfeExampleDirs[0] + "/a.xml"},
		{"0002.xml", "fr/fnfe/" + fnfeExampleDirs[1] + "/b.xml"},
		{"0003.xml", "fr/variants/ubl/ubl-001-x.xml"},
		{"0004.xml", "fr/zerechnung/" + fnfeRepoDir + "/extended.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if !entries[0].fr() || !entries[0].national() {
		t.Error("fr() or national() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "fnfe.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], filepath.Join(rfe, fnfeXSLCII)+"\t") ||
		!strings.HasPrefix(lines[1], filepath.Join(rfe, fnfeXSLUBL)+"\t") || !strings.HasSuffix(lines[3], "0004.xml") {
		t.Errorf("fnfe.list:\n%s", list)
	}
	for _, l := range []string{"kosit.list", "mustang.list", "npa.list", "pt.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); len(b) != 0 {
			t.Errorf("%s:\n%s", l, b)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(work, "expected.tsv")); string(b) != "fr/variants/ubl/ubl-001-x.xml\tBR-FREXT-01\n" {
		t.Errorf("expected.tsv:\n%s", b)
	}

	write(variants, "ubl/bad.xml", "<Order/>")
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hrInputs{}, fr, cdvInputs{}, ""); err == nil {
		t.Error("a variant that is not an invoice: no error")
	}
}

// TestFNFESection checks the EXTENDED-CTC-FR part of the report.
func TestFNFESection(t *testing.T) {
	all := []comparison{
		{Reference: "fnfe", Invoice: "fr/fnfe/" + fnfeExampleDirs[0] + "/a.xml"},
		{Reference: "fnfe", Invoice: "fr/zerechnung/" + fnfeRepoDir + "/extended.xml"},
		{Reference: "fnfe", Invoice: "fr/variants/cii-001-x.xml"},
		{Reference: "fnfe", Invoice: "fr/variants/ubl-001-y.xml", Ref: result{Rules: map[string]string{"BR-AF-09": levelError}, Stopped: "XPTY0004 in the test of BR-AF-09: z"}},
		{Reference: "npa", Invoice: "nl/variants/x.xml"},
	}
	var b strings.Builder
	fnfeSection(&b, all)
	md := b.String()
	for _, want := range []string{
		"1 exemples EXTENDED-CTC-FR de France_RFE, 1 factures EXTENDED-CTC-FR du dépôt et 2 variantes",
		"| `variants/ubl-001-y.xml` | XPTY0004 in the test of BR-AF-09: z |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	b.Reset()
	fnfeSection(&b, all[4:])
	if b.Len() != 0 {
		t.Errorf("no EXTENDED-CTC-FR file: %q", b.String())
	}
}
