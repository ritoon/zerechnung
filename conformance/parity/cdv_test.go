package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// cdvStatus is a CDAR status of the repository, a copy of an example of
// France_RFE.
const cdvStatus = "../../cdar/testdata/UC3_F202500005_04-CDV-210_Refusee.xml"

func TestOursCDV(t *testing.T) {
	data, err := os.ReadFile(cdvStatus)
	if err != nil {
		t.Fatal(err)
	}
	if r := oursCDV("a.xml", data); len(r.Rules) != 0 || r.Failure != "" {
		t.Errorf("valid status: %+v", r)
	}
	// A reason outside the lists, and an unreadable sequence number, which
	// the schema reports and the XSLT does not see.
	data = bytes.Replace(data, []byte("<ram:ReasonCode>TX_TVA_ERR<"), []byte("<ram:ReasonCode>XXX<"), 1)
	data = bytes.Replace(data, []byte("<ram:SequenceNumeric>1<"), []byte("<ram:SequenceNumeric>un<"), 1)
	want := map[string]string{"BR-FR-CDV-CL-09_MDT-113": levelError, "BR-FR-CDV-CL-09_MDT-113_210": levelError}
	if r := oursCDV("a.xml", data); !reflect.DeepEqual(r.Rules, want) {
		t.Errorf("invalid status: %+v", r)
	}
}

func TestStageCDV(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	rfe, variants := t.TempDir(), t.TempDir()
	status, err := os.ReadFile(cdvStatus)
	if err != nil {
		t.Fatal(err)
	}
	write := func(root, name string, content []byte) {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, content, 0o644)
	}
	for _, d := range corpusDirs {
		os.MkdirAll(filepath.Join(corpus, d), 0o755)
	}
	os.MkdirAll(filepath.Join(kosit, "src/test"), 0o755)
	write(rfe, cdvExampleDir+"/UC1/a.xml", status)
	write(rfe, cdvExampleDir+"/readme.md", []byte("not a status"))
	write(rfe, "FNFE_RFE_INVOICE/Z.example/TEST/CII/EN16931/c.xml", status)
	write(variants, "cdv00001.xml", status)
	write(variants, "manifest.tsv", []byte("cdv00001.xml\tx\n"))
	cdv := cdvInputs{rfe: rfe, variants: variants}
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hrInputs{}, fnfeInputs{}, cdv, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "cdv/fnfe/" + cdvExampleDir + "/UC1/a.xml"},
		{"0002.xml", "cdv/variants/cdv00001.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if !entries[0].cdv() || !entries[0].national() || entries[0].fr() {
		t.Error("cdv(), national() or fr() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "cdv.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], filepath.Join(rfe, cdvXSL)+"\t") || !strings.HasSuffix(lines[1], "0002.xml") {
		t.Errorf("cdv.list:\n%s", list)
	}
	for _, l := range []string{"kosit.list", "mustang.list", "npa.list", "fnfe.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); len(b) != 0 {
			t.Errorf("%s:\n%s", l, b)
		}
	}

	write(variants, "bad.xml", []byte(`<rsm:CrossIndustryInvoice xmlns:rsm="`+nsCII+`"/>`))
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, roInputs{}, hrInputs{}, fnfeInputs{}, cdv, ""); err == nil || !strings.Contains(err.Error(), "not a CDAR message") {
		t.Errorf("a variant that is not a status: %v", err)
	}
}

// TestCDVResult reads the report of the XSLT BR-FR-CDV: every assertion is
// compared at its level, and a status of the comparison only goes to that
// XSLT.
func TestCDVResult(t *testing.T) {
	work := t.TempDir()
	xsl := filepath.Join(work, "cdv.xslt")
	os.WriteFile(xsl, []byte(`<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" version="2.0"/>`), 0o644)
	os.WriteFile(filepath.Join(work, "cdv.list"), []byte(xsl+"\t"+filepath.Join(work, "in", "0001.xml")+"\n"), 0o644)
	run, err := newXSLRun(work, "cdv.list")
	if err != nil {
		t.Fatal(err)
	}
	svrl := `<svrl:schematron-output xmlns:svrl="http://purl.oclc.org/dsdl/svrl">
<svrl:failed-assert id="BR-FR-CDV-CL-09_MDT-113" flag="error" location="/a"><svrl:text>x</svrl:text></svrl:failed-assert>
<svrl:failed-assert id="BR-FR-CDV-15_MDT-115" flag="warning" location="/b"><svrl:text>y</svrl:text></svrl:failed-assert>
</svrl:schematron-output>`
	e := entry{Name: "0001.xml", Source: "cdv/variants/cdv00001.xml"}
	r, err := run.result(e, []byte(svrl))
	want := map[string]string{"BR-FR-CDV-CL-09_MDT-113": levelError, "BR-FR-CDV-15_MDT-115": levelWarning}
	if err != nil || !reflect.DeepEqual(r.Rules, want) || r.Stopped != "" {
		t.Errorf("result: %+v, %v", r, err)
	}
	for _, ref := range references {
		if ref.applies(e) != (ref.Key == "cdv") {
			t.Errorf("%s applies to a status: %v", ref.Key, ref.applies(e))
		}
	}
}

// TestCDVXSL indexes the XSLT BR-FR-CDV of France_RFE (FX_FNFE): every
// assertion of the rules of internal/rules/cdv, the rule BR-FR-04 of the
// document type included.
func TestCDVXSL(t *testing.T) {
	rfe := os.Getenv("FX_FNFE")
	if rfe == "" {
		t.Skip("FX_FNFE not set")
	}
	idx, err := indexXSL(filepath.Join(rfe, cdvXSL))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, a := range idx.asserts {
		ids[a.ID] = true
	}
	if len(idx.asserts) != 43 || !ids["BR-FR-04_MDT-91"] || !ids["BR-FR-CDV-CL-09_MDT-113_210"] {
		t.Errorf("%d assertions: %v", len(idx.asserts), ids)
	}
}

// TestCDVSection checks the part of the report on the statuses.
func TestCDVSection(t *testing.T) {
	all := []comparison{
		{Reference: "cdv", Invoice: "cdv/fnfe/" + cdvExampleDir + "/a.xml"},
		{Reference: "cdv", Invoice: "cdv/variants/cdv00001.xml"},
		{Reference: "cdv", Invoice: "cdv/variants/cdv00002.xml", Ref: result{Rules: map[string]string{"BR-FR-CDV-14_MDT-207": levelError}, Stopped: "XPTY0004 in the test of BR-FR-CDV-14_MDT-207: z"}},
		{Reference: "fnfe", Invoice: "fr/variants/x.xml"},
	}
	var b strings.Builder
	cdvSection(&b, all)
	md := b.String()
	for _, want := range []string{
		"1 exemples CDAR de France_RFE et 2 variantes",
		"| `variants/cdv00002.xml` | XPTY0004 in the test of BR-FR-CDV-14_MDT-207: z |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	b.Reset()
	cdvSection(&b, all[3:])
	if b.Len() != 0 {
		t.Errorf("no status: %q", b.String())
	}
}
