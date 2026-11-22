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

// TestRORun compares a run of the test XSLT whose assertions are BR-27, a
// rule of EN 16931 that the ROmodel phase restates, and BR-RO-010: only the
// rules of the CIUS-RO are compared, and a transformation stopped by the test
// of BR-27 compares nothing.
func TestRORun(t *testing.T) {
	dir := t.TempDir()
	xsl := strings.NewReplacer("BR-CO-25", "BR-27", "UBL-SR-01", "BR-RO-010").Replace(testSchXslt)
	os.WriteFile(filepath.Join(dir, roXSL), []byte(xsl), 0o644)
	os.WriteFile(filepath.Join(dir, "ro.list"), []byte(filepath.Join(dir, "in", "0001.xml")+"\n"), 0o644)
	run, err := newSchRun(dir, "ro.list", roXSL, roNational)
	if err != nil {
		t.Fatal(err)
	}
	const inv = "/Q{urn:oasis:names:specification:ubl:schema:xsd:Invoice-2}Invoice[1]"
	e := entry{Name: "0001.xml"}
	r, err := run.result(e, []byte(`<schematron-output xmlns="http://purl.oclc.org/dsdl/svrl">
<failed-assert id="BR-27" flag="fatal" location="`+inv+`/Q{urn:x}InvoiceLine[1]/Q{urn:y}Price[1]"><text>[BR-27]-Price.</text></failed-assert>
<failed-assert id="BR-RO-010" flag="warning" location="`+inv+`"><text>[BR-RO-010]-Type.</text></failed-assert>
</schematron-output>`))
	if want := map[string]string{"BR-RO-010@/Invoice[1]": levelWarning}; err != nil || !reflect.DeepEqual(r.Rules, want) || r.Stopped != "" {
		t.Errorf("findings: %+v, %v", r, err)
	}
	r, err = run.result(e, []byte(`<error code="FORG0001" line="9">Cannot convert string "1 0" to xs:double</error>`))
	if err != nil || len(r.Rules) != 0 || !strings.HasPrefix(r.Stopped, "FORG0001 in the test of BR-27") {
		t.Errorf("stopped by BR-27: %+v, %v", r, err)
	}
	mine := map[string]string{"BR-RO-020@/Invoice[1]": levelError}
	if got := diff(unlocated(mine), r, true); len(got) != 0 {
		t.Errorf("stopped by BR-27: %q", got)
	}
	r, err = run.result(e, []byte(`<error code="XPTY0004" line="19">A sequence of more than one item is not allowed</error>`))
	if err != nil || !reflect.DeepEqual(r.Rules, map[string]string{"BR-RO-010": levelWarning}) || !strings.HasPrefix(r.Stopped, "XPTY0004 in the test of BR-RO-010") {
		t.Errorf("stopped by BR-RO-010: %+v, %v", r, err)
	}
	if _, err := run.result(entry{Name: "0002.xml"}, nil); err == nil || !strings.Contains(err.Error(), "ro.list") {
		t.Errorf("invoice not in ro.list: %v", err)
	}
	for id, want := range map[string]bool{"BR-RO-001": true, "BR-DEC-RO-1010": true, "BR-27": false, "RO-CIF-01": false} {
		if roNational(id) != want {
			t.Errorf("roNational(%s) = %v", id, !want)
		}
	}
}

// TestIndexSchXsltRule checks that a dynamic error that Saxon locates on the
// start tag of the schxslt:rule element of a rule is the one of its
// assertion when the rule holds a single one, as BR-27 in the CIUS-RO
// schematron, and of none otherwise.
func TestIndexSchXsltRule(t *testing.T) {
	const xsl = `<xsl:transform xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:schxslt="https://doi.org/10.5281/zenodo.1495494" version="2.0">
   <xsl:template match="x">
      <schxslt:rule pattern="d18e3463">
         <xsl:if test="exists($documentUri)"/>
         <xsl:if test="not((.) &gt;=0)">
            <svrl:failed-assert xmlns:svrl="http://purl.oclc.org/dsdl/svrl" location="{schxslt:location(.)}" flag="fatal" id="BR-27">
               <svrl:text>[BR-27]-Price.</svrl:text>
            </svrl:failed-assert>
         </xsl:if>
      </schxslt:rule>
      <schxslt:rule
          pattern="d18e3464">
         <xsl:if test="a"><svrl:failed-assert xmlns:svrl="http://purl.oclc.org/dsdl/svrl" id="BR-RO-010"/></xsl:if>
         <xsl:if test="b"><svrl:failed-assert xmlns:svrl="http://purl.oclc.org/dsdl/svrl" id="BR-RO-020"/></xsl:if>
      </schxslt:rule>
   </xsl:template>
</xsl:transform>
`
	file := filepath.Join(t.TempDir(), roXSL)
	os.WriteFile(file, []byte(xsl), 0o644)
	idx, err := indexSchXslt(file)
	if err != nil {
		t.Fatal(err)
	}
	for line, want := range map[int]string{3: "BR-27", 5: "BR-27", 4: "", 11: "", 12: "", 13: "BR-RO-010", 14: "BR-RO-020", 2: ""} {
		a, ok := idx.at(line)
		if a.ID != want || ok != (want != "") {
			t.Errorf("at(%d) = %s, %v, want %s", line, a.ID, ok, want)
		}
	}
}

// TestOursRO runs the package on the UBL CIUS-RO invoices of the repository,
// which give no finding, and on variants, including one without BT-24, which
// the package still checks as a CIUS-RO invoice, and one with a wrong CIF,
// a check that the schematron does not have.
func TestOursRO(t *testing.T) {
	for _, f := range []string{"cius-ro.ubl.xml", "cius-ro-cn.ubl.xml"} {
		data, err := os.ReadFile(filepath.Join("../../internal/rules/ro/testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if r := oursRO(f, data); len(r.Rules) != 0 || r.Failure != "" {
			t.Errorf("%s: %+v", f, r)
		}
		if f != "cius-ro-cn.ubl.xml" {
			continue
		}
		noBT24 := regexp.MustCompile(`<cbc:CustomizationID>[^<]*</cbc:CustomizationID>`).ReplaceAllString(string(data), "")
		if r := oursRO(f, []byte(noBT24)); !reflect.DeepEqual(r.Rules, map[string]string{"BR-RO-001@/CreditNote[1]": levelError}) {
			t.Errorf("%s without BT-24: %+v", f, r.Rules)
		}
		county := strings.Replace(string(data), "<cbc:CountrySubentity>RO-TM<", "<cbc:CountrySubentity>Timis<", 1)
		if r := oursRO(f, []byte(county)); !reflect.DeepEqual(r.Rules, map[string]string{"BR-RO-110@/CreditNote[1]": levelError}) {
			t.Errorf("%s with a bad county: %+v", f, r.Rules)
		}
		badCIF := strings.Replace(string(data), "<cbc:CompanyID>RO41526372<", "<cbc:CompanyID>RO41526373<", 1)
		if r := oursRO(f, []byte(badCIF)); !slices.Contains(r.Other, "RO-CIF-02") || len(r.Rules) != 0 {
			t.Errorf("%s with a bad CIF: %+v", f, r)
		}
	}
}

func TestStageRO(t *testing.T) {
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
	cii := `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`
	write(repo, "internal/rules/ro/testdata/cius-ro.ubl.xml", ubl)
	write(repo, "internal/rules/ro/testdata/cius-ro.cii.xml", cii)
	write(variants, "v00001.xml", ubl)
	write(variants, "manifest.tsv", "v00001.xml\tx")
	archive := func(name string, files map[string]string) {
		f, _ := os.Create(filepath.Join(examples, name))
		z := zip.NewWriter(f)
		for n, content := range files {
			w, _ := z.Create(n)
			w.Write([]byte(content))
		}
		z.Close()
		f.Close()
	}
	archive("eFactura_UBL2.1_exempleXML.zip", map[string]string{"eFactura_UBL2.1_exempleXML/": "", "eFactura_UBL2.1_exempleXML/ubl_b2g_example0.xml": ubl})
	archive("exemple_Invoice_CreditNote.zip", map[string]string{"exemple_Invoice_CreditNote/eInvoice_ex.xml": ubl})
	archive("eFactura_CII_exempleXML.zip", map[string]string{"CII_example0.xml": cii})
	ro := roInputs{examples: examples, repo: repo, variants: variants}
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, ro, fnfeInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "ro/anaf/eFactura_UBL2.1_exempleXML/ubl_b2g_example0.xml"},
		{"0002.xml", "ro/anaf/exemple_Invoice_CreditNote/eInvoice_ex.xml"},
		{"0003.xml", "ro/variants/v00001.xml"},
		{"0004.xml", "ro/zerechnung/internal/rules/ro/testdata/cius-ro.ubl.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if !entries[0].ro() || !entries[0].national() || entries[0].pt() {
		t.Error("ro() or national() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "ro.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 4 || !strings.HasSuffix(lines[0], "0001.xml") || !strings.HasSuffix(lines[3], "0004.xml") {
		t.Errorf("ro.list:\n%s", list)
	}
	for _, l := range []string{"kosit.list", "mustang.list", "pt.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); len(b) != 0 {
			t.Errorf("%s:\n%s", l, b)
		}
	}
	os.Remove(filepath.Join(examples, "exemple_Invoice_CreditNote.zip"))
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, ro, fnfeInputs{}, ""); err == nil {
		t.Error("missing archive: no error")
	}
}

// TestROSection checks the CIUS-RO part of the report: files by origin and
// stops by error code, including those on BR-27, which compare nothing.
func TestROSection(t *testing.T) {
	all := []comparison{
		{Reference: "ro", Invoice: "ro/anaf/eFactura_UBL2.1_exempleXML/ubl_b2g_example0.xml"},
		{Reference: "ro", Invoice: "ro/zerechnung/internal/rules/ro/testdata/cius-ro.ubl.xml"},
		{Reference: "ro", Invoice: "ro/variants/v00001.xml", Ref: result{Rules: map[string]string{}, Stopped: "FORG0001 in the test of BR-27: x"}},
		{Reference: "ro", Invoice: "ro/variants/v00002.xml", Ref: result{Rules: map[string]string{"BR-RO-A999": levelError}, Stopped: "XPTY0004 in the test of BR-RO-A999: y"}},
		{Reference: "ro", Invoice: "ro/variants/v00003.xml"},
		{Reference: "pt", Invoice: "pt/variants/v00001.xml"},
	}
	var b strings.Builder
	roSection(&b, all)
	md := b.String()
	for _, want := range []string{
		"### CIUS-RO : schematron de l'ANAF",
		"| Exemples UBL de l'ANAF (`FX_RO`) | 1 | 1 | 0 | 0 | 0 |",
		"| Factures UBL du dépôt (`internal/rules/ro/testdata`) | 1 | 1 | 0 | 0 | 0 |",
		" | 3 | 1 | 2 | 0 | 0 |",
		"| `FORG0001` : valeur qui ne se convertit pas en nombre | 1 | `BR-27` (1) |",
		"| `XPTY0004` : plusieurs valeurs là où le test n'en attend qu'une | 1 | `BR-RO-A999` (1) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	b.Reset()
	roSection(&b, all[5:])
	if b.Len() != 0 {
		t.Errorf("no CIUS-RO file: %q", b.String())
	}
	if got := stoppedAssertion("FORG0001 in the test of BR-RO-L1024: x: y"); got != "BR-RO-L1024" {
		t.Errorf("stoppedAssertion = %q", got)
	}
	if got := stoppedAssertion("schema"); got != "" {
		t.Errorf("stoppedAssertion = %q", got)
	}
}
