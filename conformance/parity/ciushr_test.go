package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestHRRun compares a run of the test XSLT whose assertions are renamed
// HR-BR-1 and HR-BR-5 (a warning in the test XSLT): every assertion of the CIUS-HR schematron is
// compared, and a transformation stopped by the test of one compares that
// assertion only.
func TestHRRun(t *testing.T) {
	dir := t.TempDir()
	xsl := strings.NewReplacer("BR-CO-25", "HR-BR-1", "UBL-SR-01", "HR-BR-5").Replace(testSchXslt)
	os.WriteFile(filepath.Join(dir, hrXSL), []byte(xsl), 0o644)
	os.WriteFile(filepath.Join(dir, "hr.list"), []byte(filepath.Join(dir, "in", "0001.xml")+"\n"), 0o644)
	run, err := newSchRun(dir, "hr.list", hrXSL, nil)
	if err != nil {
		t.Fatal(err)
	}
	const inv = "/Q{urn:oasis:names:specification:ubl:schema:xsd:Invoice-2}Invoice[1]"
	e := entry{Name: "0001.xml"}
	r, err := run.result(e, []byte(`<schematron-output xmlns="http://purl.oclc.org/dsdl/svrl">
<failed-assert id="HR-BR-1" flag="fatal" location="`+inv+`/Q{urn:x}ID[1]"><text>[HR-BR-1] - Broj računa.</text></failed-assert>
<failed-assert id="HR-BR-5" flag="warning" location="`+inv+`"><text>[HR-BR-5] - Identifikator.</text></failed-assert>
</schematron-output>`))
	want := map[string]string{"HR-BR-1@/Invoice[1]/ID[1]": levelError, "HR-BR-5@/Invoice[1]": levelWarning}
	if err != nil || !reflect.DeepEqual(r.Rules, want) || r.Stopped != "" {
		t.Errorf("findings: %+v, %v", r, err)
	}
	r, err = run.result(e, []byte(`<error code="XPTY0004" line="19">A sequence of more than one item is not allowed</error>`))
	if err != nil || !reflect.DeepEqual(r.Rules, map[string]string{"HR-BR-5": levelWarning}) || !strings.HasPrefix(r.Stopped, "XPTY0004 in the test of HR-BR-5") {
		t.Errorf("stopped by HR-BR-5: %+v, %v", r, err)
	}
	if got := diff(unlocated(map[string]string{"HR-BR-5@/Invoice[1]": levelWarning, "HR-BR-1@/Invoice[1]/ID[1]": levelError}), r, true); len(got) != 0 {
		t.Errorf("stopped by HR-BR-5, raised by the package: %q", got)
	}
	if got := diff(unlocated(map[string]string{"HR-BR-1@/Invoice[1]/ID[1]": levelError}), r, true); !slices.Equal(got, []string{"-HR-BR-5"}) {
		t.Errorf("stopped by HR-BR-5, not raised by the package: %q", got)
	}
	// An error in an xsl:function (u:ctrlOIB) or in a variable evaluated
	// lazily: the assertion is the one at a line of the call stack, or at a
	// line that the message names.
	for _, c := range []struct {
		report string
		want   map[string]string
	}{
		{`<error code="XPTY0004" line="2" stack="5 9 3">A sequence of more than one item is not allowed as the first argument of fn:starts-with(). Found while atomizing the first argument of fn:substring() in {$oib} on line 2</error>`,
			map[string]string{"HR-BR-1": levelError}},
		{`<error code="XPTY0004" line="2">A sequence of more than one item is not allowed as the first operand of '*'. Found while atomizing the first operand of '&lt;=' in {$payableAmount} on line 19</error>`,
			map[string]string{"HR-BR-5": levelWarning}},
	} {
		r, err := run.result(e, []byte(c.report))
		if err != nil || !reflect.DeepEqual(r.Rules, c.want) || r.Failure != "" || r.Stopped == "" {
			t.Errorf("%s: %+v, %v", c.report, r, err)
		}
	}
	r, err = run.result(e, []byte(`<error code="XPTY0004" line="2" stack="3 x">y</error>`))
	if err == nil || !strings.Contains(err.Error(), "bad stack") {
		t.Errorf("bad stack: %+v, %v", r, err)
	}
	r, err = run.result(e, []byte(`<error code="XPTY0004" line="2" stack="3">y on line 5</error>`))
	if err != nil || len(r.Rules) != 0 || !strings.HasPrefix(r.Failure, "XPTY0004, line 2 of cius-hr.xsl") {
		t.Errorf("no assertion: %+v, %v", r, err)
	}
	if _, err := run.result(entry{Name: "0002.xml"}, nil); err == nil || !strings.Contains(err.Error(), "hr.list") {
		t.Errorf("invoice not in hr.list: %v", err)
	}
	for id, want := range map[string]bool{"HR-BR-1": true, "HR-BR-CL-1": true, "HR-BR-S-10": true, "HR-OIB-01": false, "HR-SYNTAX-01": false} {
		if hrSchematron(id) != want {
			t.Errorf("hrSchematron(%s) = %v", id, !want)
		}
	}
}

// TestOursHR runs the package on the CIUS-HR invoices of the repository,
// which give no finding, and on variants: one without BT-24, which the
// package still checks as a CIUS-HR invoice, one without the operator label,
// and one with a wrong OIB in an electronic address, a check that the
// schematron does not have.
func TestOursHR(t *testing.T) {
	for _, f := range []string{"cius-hr.ubl.xml", "cius-hr-cn.ubl.xml", "cius-hr-o.ubl.xml"} {
		data, err := os.ReadFile(filepath.Join("../../internal/rules/hr/testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if r := oursHR(f, data); len(r.Rules) != 0 || r.Failure != "" {
			t.Errorf("%s: %+v", f, r)
		}
		if f != "cius-hr.ubl.xml" {
			continue
		}
		noBT24 := regexp.MustCompile(`<cbc:CustomizationID>[^<]*</cbc:CustomizationID>`).ReplaceAllString(string(data), "")
		if r := oursHR(f, []byte(noBT24)); !reflect.DeepEqual(r.Rules, map[string]string{"HR-BR-5@/Invoice[1]": levelError}) {
			t.Errorf("%s without BT-24: %+v", f, r.Rules)
		}
		noOperator := strings.Replace(string(data), "<cbc:Name>Operater 3</cbc:Name>", "", 1)
		if r := oursHR(f, []byte(noOperator)); !reflect.DeepEqual(r.Rules, map[string]string{"HR-BR-37@/Invoice[1]/AccountingSupplierParty[1]": levelError}) {
			t.Errorf("%s without the operator label: %+v", f, r.Rules)
		}
		badOIB := strings.Replace(string(data), `<cbc:EndpointID schemeID="9934">47803662159<`, `<cbc:EndpointID schemeID="9934">47803662158<`, 1)
		if r := oursHR(f, []byte(badOIB)); !slices.Contains(r.Other, "HR-OIB-01") || len(r.Rules) != 0 {
			t.Errorf("%s with a bad OIB: %+v", f, r)
		}
	}
}

func TestStageHR(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	repo, variants, examples := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, d := range corpusDirs {
		os.MkdirAll(filepath.Join(corpus, d), 0o755)
	}
	os.MkdirAll(filepath.Join(kosit, "src/test"), 0o755)
	ubl := `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`
	write(repo, "internal/rules/hr/testdata/cius-hr.ubl.xml", ubl)
	write(variants, "v00001.xml", ubl)
	write(variants, "manifest.tsv", "v00001.xml\tx")
	archive := filepath.Join(examples, "primjeri-eracuna.zip")
	f, _ := os.Create(archive)
	z := zip.NewWriter(f)
	for _, n := range []string{"eRacun-PDV25.xml", "Neoporezivi racun.xml", "procitaj.txt"} {
		w, _ := z.Create(n)
		w.Write([]byte(ubl))
	}
	z.Close()
	f.Close()
	hr := hrInputs{examples: archive, repo: repo, variants: variants}
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hr, fnfeInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "hr/porezna/Neoporezivi racun.xml"},
		{"0002.xml", "hr/porezna/eRacun-PDV25.xml"},
		{"0003.xml", "hr/variants/v00001.xml"},
		{"0004.xml", "hr/zerechnung/internal/rules/hr/testdata/cius-hr.ubl.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if !entries[0].hr() || !entries[0].national() || entries[0].ro() || entries[0].pt() {
		t.Error("hr() or national() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "hr.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 4 || !strings.HasSuffix(lines[0], "0001.xml") || !strings.HasSuffix(lines[3], "0004.xml") {
		t.Errorf("hr.list:\n%s", list)
	}
	for _, l := range []string{"kosit.list", "mustang.list", "pt.list", "ro.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); len(b) != 0 {
			t.Errorf("%s:\n%s", l, b)
		}
	}
	os.Remove(archive)
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hr, fnfeInputs{}, ""); err == nil {
		t.Error("missing archive: no error")
	}
}

// TestHRSection checks the CIUS-HR part of the report: files by origin and
// stops by error code.
func TestHRSection(t *testing.T) {
	all := []comparison{
		{Reference: "hr", Invoice: "hr/porezna/eRacun-PDV25.xml"},
		{Reference: "hr", Invoice: "hr/zerechnung/internal/rules/hr/testdata/cius-hr.ubl.xml"},
		{Reference: "hr", Invoice: "hr/variants/v00001.xml", Ref: result{Rules: map[string]string{"HR-BR-29": levelError}, Stopped: "FORG0001 in the test of HR-BR-29: x"}},
		{Reference: "hr", Invoice: "hr/variants/v00002.xml", Unexpected: []string{"+HR-BR-1@/Invoice[1]/ID[1]"}},
		{Reference: "hr", Invoice: "hr/variants/v00003.xml"},
		{Reference: "ro", Invoice: "ro/variants/v00001.xml"},
	}
	var b strings.Builder
	hrSection(&b, all)
	md := b.String()
	for _, want := range []string{
		"### CIUS-HR : schematron de la Porezna uprava",
		"| Exemples de la Porezna uprava (`FX_HR`) | 1 | 1 | 0 | 0 | 0 |",
		"| Factures du dépôt (`internal/rules/hr/testdata`) | 1 | 1 | 0 | 0 | 0 |",
		" | 3 | 1 | 1 | 0 | 1 |",
		"| `FORG0001` : valeur qui ne se convertit pas en nombre | 1 | `HR-BR-29` (1) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	b.Reset()
	hrSection(&b, all[5:])
	if b.Len() != 0 {
		t.Errorf("no CIUS-HR file: %q", b.String())
	}
}
