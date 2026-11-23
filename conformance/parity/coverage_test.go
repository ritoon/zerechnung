package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestStageVariants(t *testing.T) {
	corpus, kosit, work, variants, nlVariants := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, d := range corpusDirs {
		os.MkdirAll(filepath.Join(corpus, d), 0o755)
	}
	os.MkdirAll(filepath.Join(kosit, "src/test"), 0o755)
	cii := `<rsm:CrossIndustryInvoice xmlns:rsm="` + nsCII + `"/>`
	write(corpus, "XML-Rechnung/CII/a.cii.xml", cii)
	write(variants, "de/cii-001-br-de-1.xml", cii)
	write(variants, "de/expected.tsv", "cii-001-br-de-1.xml\tBR-DE-1 PEPPOL-EN16931-R008\n")
	write(variants, "ubl/ubl-000-base.xml", cii)
	write(variants, "ubl/expected.tsv", "ubl-000-base.xml\t\n\n")
	write(nlVariants, "cii-003-br-nl-1.xml", cii)
	write(nlVariants, "expected.tsv", "cii-003-br-nl-1.xml\tBR-NL-1\n")
	tests, repo := t.TempDir(), t.TempDir()
	for _, d := range nlTestDirs {
		os.MkdirAll(filepath.Join(tests, d), 0o755)
	}
	os.MkdirAll(filepath.Join(repo, "internal/rules/nl/testdata"), 0o755)
	nl := nlInputs{npa: t.TempDir(), tests: tests, repo: repo, variants: nlVariants}
	if err := stage(corpus, kosit, work, nl, ptInputs{}, roInputs{}, hrInputs{}, fnfeInputs{}, cdvInputs{}, variants); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "corpus/XML-Rechnung/CII/a.cii.xml"},
		{"0002.xml", "nl/variants/cii-003-br-nl-1.xml"},
		{"0003.xml", "variants/de/cii-001-br-de-1.xml"},
		{"0004.xml", "variants/ubl/ubl-000-base.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %q", entries)
	}
	if entries[0].variant() || entries[1].variant() || !entries[2].variant() {
		t.Error("variant() is wrong")
	}
	kositList, _ := os.ReadFile(filepath.Join(work, "kosit.list"))
	if n := strings.Count(string(kositList), "\n"); n != 3 || !strings.Contains(string(kositList), "0004.xml") {
		t.Errorf("kosit.list:\n%s", kositList)
	}
	mustangList, _ := os.ReadFile(filepath.Join(work, "mustang.list"))
	if n := strings.Count(string(mustangList), "\n"); n != 1 {
		t.Errorf("mustang.list, without the variants:\n%s", mustangList)
	}
	got, err := readExpectedRules(work)
	if err != nil {
		t.Fatal(err)
	}
	wantRules := map[string][]string{
		"nl/variants/cii-003-br-nl-1.xml": {"BR-NL-1"},
		"variants/de/cii-001-br-de-1.xml": {"BR-DE-1", "PEPPOL-EN16931-R008"},
		"variants/ubl/ubl-000-base.xml":   {},
	}
	if !reflect.DeepEqual(got, wantRules) {
		t.Errorf("expected rules = %q", got)
	}

	write(variants, "de/expected.tsv", "de/cii-001-br-de-1.xml\tBR-DE-1\n")
	if err := stage(corpus, kosit, work, nl, ptInputs{}, roInputs{}, hrInputs{}, fnfeInputs{}, cdvInputs{}, variants); err == nil {
		t.Error("expected.tsv with a path: no error")
	}
	if rules, err := readExpectedRules(t.TempDir()); err != nil || rules != nil {
		t.Errorf("no expected.tsv: %v, %v", rules, err)
	}
}

func TestMissing(t *testing.T) {
	ref := newResult()
	ref.add("BR-DE-1", levelWarning)
	expected := []string{"BR-DE-1", "BR-DE-2", "BR-DE-3", "BR-FR-01"}
	got := missing(expected, ref, []string{"+BR-DE-2"})
	// BR-DE-1 is raised (at any level), BR-DE-2 is already a difference,
	// BR-FR-01 takes no part in the comparison.
	if !slices.Equal(got, []string{"?BR-DE-3"}) {
		t.Errorf("missing = %q", got)
	}
	ref.Stopped = "XML Schema"
	if got := missing(expected, ref, nil); got != nil {
		t.Errorf("stopped reference: missing = %q", got)
	}
	k, err := readKnown(writeKnown(t, "kosit variants/de/* ?BR-DE-3 reason\n"))
	if err != nil || len(k) != 1 || !k[0].match("kosit", "variants/de/cii-001.xml", "?BR-DE-3") {
		t.Errorf("known ?RULE: %v, %v", k, err)
	}
}

func writeKnown(t *testing.T, content string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "known.txt")
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestParseKoSITStopped(t *testing.T) {
	r, err := parseKoSIT([]byte(kositReport))
	if err != nil || r.Stopped != "" {
		t.Errorf("schematron steps after the schema: stopped = %q, %v", r.Stopped, err)
	}
	// The KoSIT validator runs no schematron on an invoice that the XML
	// schema rejects: the report has the schema step only.
	schemaOnly := kositReport[:strings.Index(kositReport, `<rep:validationStepResult id="val-sch.1"`)] + `</rep:scenarioMatched></rep:report>`
	r, err = parseKoSIT([]byte(schemaOnly))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stopped == "" || !reflect.DeepEqual(r.Rules, map[string]string{schemaToken: levelError}) {
		t.Errorf("schema only: %+v", r)
	}
	mine := map[string]string{schemaToken: levelError, "BR-CO-10": levelError}
	if d := diff(mine, r, false); d != nil {
		t.Errorf("stopped: diff = %q", d)
	}
	if d := diff(mine, r, true); d != nil {
		t.Errorf("stopped, levels: diff = %q", d)
	}
}

// kositLog is the end of a log of the KoSIT validator, as it prints it for
// long paths: the second document could not be assessed, the description of
// the third holds a line break, and its name has five digits.
const kositLog = `Processing of 3 objects completed in 2,580ms
Results:
--------------------------------------------------------------------------------------------------------------------------------------------------------
|File                                                        |Schema |Schematron|Acceptance|Error/Description                                           |
|/home/runner/work/zerechnung/zerechnung/conformance/.arti...|   Y   |    Y     |ACCEPTABLE|                                                            |
|acts/parity/in/0001.xml                                     |       |          |          |                                                            |
|/home/runner/work/zerechnung/zerechnung/conformance/.arti...|   Y   |    N     |UNDEFINED |Error processing schematron validation Schematron rules for |
|acts/parity/in/0002.xml                                     |       |          |          |EN16931 (CII). Error is A sequence of more than one item is |
|                                                            |       |          |          |not allowed                                                 |
|/home/runner/work/zerechnung/zerechnung/conformance/.arti...|   Y   |    N     |UNDEFINED |Error processing schematron validation Schematron rules for |
|ts/parity/in/15332.xml                                      |       |          |          |than one item { ("
", "
") } is not allowed as the first ...|
|/home/runner/work/zerechnung/zerechnung/conformance/.arti...|   N   |    Y     |  REJECT  |cvc-complex-type.2.4.a: Invalid content was found starting w|
|acts/parity/in/0004.xml                                     |       |          |          |ith element                                                 |


##############################
#     Validation failed!     #
##############################
`

func TestKositFailures(t *testing.T) {
	name := filepath.Join(t.TempDir(), "kosit.log")
	if err := os.WriteFile(name, []byte(kositLog), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := kositFailures(name)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"0002.xml":  "Error processing schematron validation Schematron rules for EN16931 (CII). Error is A sequence of more than one item is not allowed",
		"15332.xml": `Error processing schematron validation Schematron rules for than one item { ("`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("failures = %q", got)
	}
	if got, err := kositFailures(filepath.Join(t.TempDir(), "none.log")); err != nil || got != nil {
		t.Errorf("no log: %v, %v", got, err)
	}
}

const kositScenarios = `<scenarios xmlns="http://www.xoev.de/de/validator/framework/1/scenarios">
  <scenario><name>EN16931 (CII)</name>
    <validateWithSchematron><resource><name>EN</name><location>resources/cii/xsl/EN16931-CII-validation.xsl</location></resource></validateWithSchematron>
  </scenario>
  <scenario><name>XRechnung (CII)</name>
    <validateWithSchematron><resource><name>EN</name><location>resources/cii/xsl/EN16931-CII-validation.xsl</location></resource></validateWithSchematron>
    <validateWithSchematron><resource><name>DE</name><location>
      resources/xrechnung/xsl/XRechnung-UBL-validation.xsl
    </location></resource></validateWithSchematron>
  </scenario>
</scenarios>`

func kositXSLT(asserts ...string) string {
	var b strings.Builder
	b.WriteString(`<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:svrl="http://purl.oclc.org/dsdl/svrl" version="2.0"><xsl:template match="/">`)
	for _, a := range asserts {
		id, flag, _ := strings.Cut(a, "/")
		b.WriteString(`<xsl:if test="x"><svrl:failed-assert location="x" flag="` + flag + `" id="` + id + `"><svrl:text>t</svrl:text></svrl:failed-assert></xsl:if>`)
	}
	b.WriteString(`<svrl:successful-report id="BR-DE-1" flag="information"/></xsl:template></xsl:stylesheet>`)
	return b.String()
}

func TestKositCoverage(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("scenarios.xml", kositScenarios)
	write("resources/cii/xsl/EN16931-CII-validation.xsl", kositXSLT("BR-01/fatal", "BR-CO-10/fatal", "BR-S-08/fatal", "CII-SR-001/warning", "BR-01/fatal"))
	write("resources/xrechnung/xsl/XRechnung-UBL-validation.xsl", kositXSLT("BR-DE-15/fatal", "PEPPOL-EN16931-R008/fatal", "X-1/warning"))
	xsls, err := kositStylesheets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(xsls) != 2 || xsls[0].Syntax != "cii" || xsls[1].Syntax != "ubl" {
		t.Fatalf("stylesheets = %+v", xsls)
	}
	if want := []string{"BR-01", "BR-CO-10", "BR-S-08", "CII-SR-001", "BR-DE-1"}; !slices.Equal(xsls[0].IDs, want) {
		t.Errorf("IDs = %q, want %q", xsls[0].IDs, want)
	}
	if xsls[0].Flags["CII-SR-001"] != "warning" || xsls[0].Flags["BR-DE-1"] != "information" {
		t.Errorf("flags = %v", xsls[0].Flags)
	}

	raise := func(rules ...string) result {
		r := newResult()
		for _, id := range rules {
			r.add(id, levelWarning)
		}
		return r
	}
	all := []comparison{
		{Reference: "kosit", Invoice: "corpus/a.xml", Ref: raise("BR-01")},
		{Reference: "kosit", Invoice: "variants/en16931/cii-001.xml", Ref: raise("BR-01", "BR-S-08")},
		{Reference: "kosit", Invoice: "variants/de/ubl-001.xml", Ref: raise("BR-DE-15", "BR-S-08")},
		{Reference: "mustang", Invoice: "corpus/a.xml", Ref: raise("BR-CO-10")},
	}
	syntax := map[string]string{"corpus/a.xml": "cii", "variants/en16931/cii-001.xml": "cii", "variants/de/ubl-001.xml": "ubl"}
	cov := coverageOf(xsls, all, syntax)
	if len(cov) != 2 {
		t.Fatalf("coverage = %+v", cov)
	}
	c := cov[0]
	if c.Stylesheet != "EN16931-CII-validation.xsl" || c.Assertions != 5 || c.Corpus != 1 || c.Variants != 2 || c.Raised != 2 {
		t.Errorf("CII coverage = %+v", c)
	}
	if !slices.Equal(c.Never, []string{"BR-CO-10", "CII-SR-001", "BR-DE-1"}) {
		t.Errorf("never raised = %q", c.Never)
	}
	wantFamilies := []familyCount{{"BR", 1, 1}, {"BR-CO", 1, 0}, {"TVA", 1, 1}, {"CII-SR", 1, 0}, {"BR-DE", 1, 0}}
	if !reflect.DeepEqual(c.Families, wantFamilies) {
		t.Errorf("families = %+v", c.Families)
	}
	if u := cov[1]; u.Raised != 1 || !slices.Equal(u.Never, []string{"PEPPOL-EN16931-R008", "X-1", "BR-DE-1"}) || u.Families[len(u.Families)-1].Family != "autres" {
		t.Errorf("UBL coverage = %+v", u)
	}

	md := markdown(all, nil, nil, cov)
	for _, s := range []string{
		"configuration XRechnung 3.0.2 du 2026-08-31 : variants des tests de mutation | 2 |",
		"1 de `internal/rules/de`, 1 de `internal/rules/en16931`",
		"| `EN16931-CII-validation.xsl` | 5 | 1 | 2 | 2 (40.0 %) |",
		"| **Total** | 9 | | | 3 (33.3 %) |",
		"EN16931-CII-validation.xsl : 3 assertions jamais levées",
	} {
		if !strings.Contains(md, s) {
			t.Errorf("report without %q:\n%s", s, md)
		}
	}

	write("scenarios.xml", strings.Replace(kositScenarios, "XRechnung-UBL-validation.xsl", "XRechnung-validation.xsl", 1))
	if _, err := kositStylesheets(dir); err == nil {
		t.Error("an XSLT of no syntax: no error")
	}
	if _, err := kositStylesheets(t.TempDir()); err == nil {
		t.Error("no scenarios.xml: no error")
	}
}

func TestDocSyntax(t *testing.T) {
	for doc, want := range map[string]string{
		`<rsm:CrossIndustryInvoice xmlns:rsm="` + nsCII + `"/>`:                           "cii",
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`:       "ubl",
		`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`: "ubl",
		`<Order/>`: "",
	} {
		if got := docSyntax([]byte(doc)); got != want {
			t.Errorf("docSyntax(%s) = %q, want %q", doc, got, want)
		}
	}
}
