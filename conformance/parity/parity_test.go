package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const kositReport = `<?xml version="1.0" encoding="UTF-8"?>
<rep:report xmlns:rep="http://www.xoev.de/de/validator/varl/1" xmlns:s="http://www.xoev.de/de/validator/framework/1/scenarios" valid="false">
  <rep:engine><rep:name>KoSIT Validator 1.6.3</rep:name></rep:engine>
  <rep:timestamp>2026-11-17T10:00:00Z</rep:timestamp>
  <rep:documentIdentification><rep:documentHash><rep:hashAlgorithm>SHA-256</rep:hashAlgorithm><rep:hashValue>x</rep:hashValue></rep:documentHash><rep:documentReference>0001.xml</rep:documentReference></rep:documentIdentification>
  <rep:scenarioMatched>
    <s:scenario><s:name>EN16931 XRechnung (CII)</s:name><s:description/><s:createReport><s:resource><s:name>Validation report for XRechnung</s:name><s:location>r.xsl</s:location></s:resource><s:customLevel level="error">CII-SR-465</s:customLevel><s:customLevel level="information">CII-SR-475</s:customLevel></s:createReport></s:scenario>
    <rep:validationStepResult id="val-xsd" valid="false">
      <s:resource><s:name>XML Schema for UN/CEFACT XML (SCRDM - CII D16B)</s:name><s:location>resources/cii/16b/xsd/x.xsd</s:location></s:resource>
      <rep:message id="val-xsd.1" level="error" code="cvc-complex-type.2.4.a" lineNumber="3">Invalid content</rep:message>
    </rep:validationStepResult>
    <rep:validationStepResult id="val-sch.1" valid="false">
      <s:resource><s:name>Schematron rules for EN16931 (CII)</s:name><s:location>x.xsl</s:location></s:resource>
      <rep:message id="val-sch.1.1" level="error" code="BR-CO-10">[BR-CO-10]-Sum of Invoice line net amount</rep:message>
      <rep:message id="val-sch.1.2" level="information" code="BR-CL-25">[BR-CL-25]-Endpoint identifier scheme</rep:message>
      <rep:message id="val-sch.1.3" level="warning" code="BR-CO-10">again, lower</rep:message>
      <rep:message id="val-sch.1.4" level="warning" code="CII-SR-465">[CII-SR-465] raised to error by the scenario</rep:message>
      <rep:message id="val-sch.1.5" level="warning" code="CII-SR-475">[CII-SR-475] lowered to information by the scenario</rep:message>
    </rep:validationStepResult>
    <rep:validationStepResult id="val-sch.2" valid="false">
      <s:resource><s:name>Schematron rules for CIUS XRechnung (CII)</s:name><s:location>y.xsl</s:location></s:resource>
      <rep:message id="val-sch.2.1" level="warning" code="BR-DE-21">[BR-DE-21] Specification identifier</rep:message>
      <rep:message id="val-sch.2.2" level="error" code="BR-FR-01">not compared</rep:message>
    </rep:validationStepResult>
  </rep:scenarioMatched>
  <rep:assessment><rep:reject><rep:explanation/></rep:reject></rep:assessment>
</rep:report>`

func TestParseKoSIT(t *testing.T) {
	r, err := parseKoSIT([]byte(kositReport))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{schemaToken: levelError, "BR-CO-10": levelError, "BR-CL-25": levelInfo, "BR-DE-21": levelWarning, "CII-SR-465": levelError, "CII-SR-475": levelInfo}
	if !r.Covered || r.Note != "EN16931 XRechnung (CII)" || !reflect.DeepEqual(r.Rules, want) {
		t.Errorf("got %+v, want rules %v", r, want)
	}
	if !reflect.DeepEqual(r.Other, []string{"BR-FR-01"}) {
		t.Errorf("other = %q", r.Other)
	}

	none := `<rep:report xmlns:rep="http://www.xoev.de/de/validator/varl/1" valid="false"><rep:noScenarioMatched><rep:documentData/></rep:noScenarioMatched><rep:assessment/></rep:report>`
	if r, err := parseKoSIT([]byte(none)); err != nil || r.Covered {
		t.Errorf("no scenario: %+v, %v", r, err)
	}
	broken := `<rep:report xmlns:rep="http://www.xoev.de/de/validator/varl/1" valid="false"><rep:noScenarioMatched><rep:validationStepResult id="wf" valid="false"><rep:message id="m" level="error" code="SAXParseException">not well-formed</rep:message></rep:validationStepResult></rep:noScenarioMatched></rep:report>`
	if r, err := parseKoSIT([]byte(broken)); err != nil || !r.Covered || r.Rules[schemaToken] != levelError {
		t.Errorf("not well-formed: %+v, %v", r, err)
	}
	if _, err := parseKoSIT([]byte(`<x/>`)); err == nil {
		t.Error("not a report: no error")
	}
}

const mustangReport = `<validation filename="0002.pdf" datetime="2026-11-17 10:00:00"><pdf><info><pdfAVersion>3</pdfAVersion></info><error type="7">XMP Metadata: ConformanceLevel contains invalid value</error><summary status="invalid"/></pdf><xml><info><version>2</version><profile>urn:cen.eu:en16931:2017</profile><validator version="2.26.0"></validator><rules><fired>460</fired><failed>3</failed></rules><duration unit="ms">350</duration></info><messages><error type="18">cvc-complex-type.2.4.a: Invalid content was found</error><error type="24" location="/*:CrossIndustryInvoice[1]" criterion="(ram:LineTotalAmount)">[BR-CO-10]-Sum of Invoice line net amount [ID BR-CO-10] from /xslt/ZF_250/FACTUR-X_EN16931.xslt)</error><warning type="24" location="/x" criterion="y">[CII-SR-308]-LineID should not be present [ID CII-SR-308] from /xslt/ZF_250/FACTUR-X_EN16931.xslt)</warning><notice type="27">[BR-DE-21] [ID BR-DE-21] from /xslt/XR_30/XRechnung-CII-validation.xslt)</notice><warning type="10">Arithmetical issue: line total</warning><error type="24">[BR-FR-12] French rule [ID BR-FR-12] from /xslt/x.xsl)</error></messages><summary status="invalid"/></xml><summary status="invalid"/></validation>`

func TestParseMustang(t *testing.T) {
	r, err := parseMustang([]byte(mustangReport))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{schemaToken: levelError, "BR-CO-10": levelError, "CII-SR-308": levelWarning, "BR-DE-21": levelInfo}
	if !r.Covered || !reflect.DeepEqual(r.Rules, want) {
		t.Errorf("rules = %v, want %v", r.Rules, want)
	}
	if len(r.Other) != 3 || !strings.HasPrefix(r.Other[0], "pdf: XMP") || !strings.Contains(r.Other[1], "type 10") || r.Other[2] != "BR-FR-12" {
		t.Errorf("other = %q", r.Other)
	}
	if _, err := parseMustang([]byte(`<rep:report/>`)); err == nil {
		t.Error("not a report: no error")
	}
}

func TestKnownAndDiff(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "known.txt")
	os.WriteFile(name, []byte("# comment\n\nmustang corpus/ZUGFeRDv2/fail/* -BR-FR-* reason one\nkosit * +BR-CL-25 reason two\n"), 0o644)
	known, err := readKnown(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 2 || known[0].reason != "reason one" {
		t.Fatalf("known = %+v", known)
	}
	for _, tc := range []struct {
		k              int
		ref, src, diff string
		want           bool
	}{
		{0, "mustang", "corpus/ZUGFeRDv2/fail/a.pdf", "-BR-FR-12", true},
		{0, "kosit", "corpus/ZUGFeRDv2/fail/a.pdf", "-BR-FR-12", false},
		{0, "mustang", "corpus/ZUGFeRDv2/fail/x/a.pdf", "-BR-FR-12", true},
		{0, "mustang", "corpus/ZUGFeRDv2/correct/a.pdf", "-BR-FR-12", false},
		{0, "mustang", "corpus/ZUGFeRDv2/fail/a.pdf", "+BR-FR-12", false},
		{1, "kosit", "kosit/src/test/a.xml", "+BR-CL-25", true},
		{1, "kosit", "kosit/src/test/a.xml", "+BR-CL-2", false},
	} {
		if got := known[tc.k].match(tc.ref, tc.src, tc.diff); got != tc.want {
			t.Errorf("%d %s %s %s: %v, want %v", tc.k, tc.ref, tc.src, tc.diff, got, tc.want)
		}
	}
	for _, tc := range []struct {
		pattern, s string
		want       bool
	}{
		{"*", "corpus/a/b.pdf", true},
		{"corpus/*/b.pdf", "corpus/a/x/b.pdf", true},
		{"corpus/*/b.pdf", "corpus/a/b.pdfx", false},
		{"*XRechnung*", "corpus/fail/XRechnung 1.XML", true},
		{"a*a", "a", false},
		{"a*a", "aa", true},
		{"corpus/a.pdf", "corpus/a.pdf", true},
		{"corpus/a.pdf", "corpus/b.pdf", false},
	} {
		if got := glob(tc.pattern, tc.s); got != tc.want {
			t.Errorf("glob(%q, %q) = %v", tc.pattern, tc.s, got)
		}
	}
	os.WriteFile(name, []byte("kosit * BR-CL-25 no sign\n"), 0o644)
	if _, err := readKnown(name); err == nil {
		t.Error("line without sign accepted")
	}

	// Errors only, a Factur-X code list assertion being the same as its CEN
	// rule (FX-SCH-A-000571, BR-CL-25), and FX-PROFILE an equivalent only.
	mine := map[string]string{schemaToken: levelError, "BR-CO-10": levelError, "BR-CO-11": levelInfo, "BR-CL-25": levelError, "FX-PROFILE-02": levelError, "BR-CL-23": levelWarning}
	ref := map[string]string{"BR-CO-11": levelError, "BR-CO-12": levelWarning, "FX-SCH-A-000571": levelError, "FX-SCH-A-000636": levelError, "FX-SCH-A-000613": levelError}
	got := diff(mine, result{Rules: ref}, false)
	if want := []string{"+BR-CO-10", "+SCHEMA", "-BR-CO-11", "-FX-SCH-A-000613"}; !reflect.DeepEqual(got, want) {
		t.Errorf("diff = %q, want %q", got, want)
	}
	c := comparison{Ours: result{Rules: mine}, Ref: result{Rules: ref}}
	for d, want := range map[string]string{
		"-FX-SCH-A-000613": "`-FX-SCH-A-000613` (warning / error)",
		"-BR-CO-11":        "`-BR-CO-11` (info / error)",
		"+BR-CO-10":        "`+BR-CO-10`",
	} {
		if got := level(c, d); got != want {
			t.Errorf("level(%s) = %s, want %s", d, got, want)
		}
	}
}

// TestOurs runs the package on invoices of the repository: a valid CII
// invoice and a hybrid PDF.
func TestOurs(t *testing.T) {
	data, err := os.ReadFile("../../testdata/EN16931_Einfach.cii.xml")
	if err != nil {
		t.Fatal(err)
	}
	if r := ours("0001.xml", data); !r.Covered || r.Failure != "" || !r.valid() {
		t.Errorf("EN16931_Einfach: %+v", r)
	}
	if r := ours("0002.xml", []byte("<a>")); r.Rules[schemaToken] != levelError {
		t.Errorf("not well-formed: %+v", r)
	}
	pdfs, _ := filepath.Glob("../../testdata/*.pdf")
	if len(pdfs) == 0 {
		t.Skip("no PDF in testdata")
	}
	data, err = os.ReadFile(pdfs[0])
	if err != nil {
		t.Fatal(err)
	}
	if r := ours("0003.pdf", data); !r.Covered || r.Failure != "" {
		t.Errorf("%s: %+v", pdfs[0], r)
	}
}

func TestStage(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, d := range corpusDirs {
		os.MkdirAll(filepath.Join(corpus, d), 0o755)
	}
	write(corpus, "XML-Rechnung/CII/a.cii.xml", "<a/>")
	write(corpus, "ZUGFeRDv2/fail/b.pdf", "%PDF")
	write(corpus, "ZUGFeRDv2/fail/XRechnung 1.XML", "<c/>")
	write(corpus, "ZUGFeRDv2/fail/notes.txt", "x")
	write(corpus, "ZUGFeRDv1/correct/d.pdf", "%PDF")
	write(kosit, "src/test/business-cases/standard/01.01a-INVOICE_ubl.xml", "<e/>")
	if err := stage(corpus, kosit, work, nlInputs{}, ptInputs{}, fnfeInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "corpus/XML-Rechnung/CII/a.cii.xml"},
		{"0002.xml", "corpus/ZUGFeRDv2/fail/XRechnung 1.XML"},
		{"0003.pdf", "corpus/ZUGFeRDv2/fail/b.pdf"},
		{"0004.xml", "kosit/src/test/business-cases/standard/01.01a-INVOICE_ubl.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if !entries[1].negative() || entries[0].negative() {
		t.Error("negative() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "kosit.list"))
	if n := strings.Count(string(list), "\n"); n != 3 || strings.Contains(string(list), ".pdf") {
		t.Errorf("kosit.list:\n%s", list)
	}
	if b, _ := os.ReadFile(filepath.Join(work, "in", "0002.xml")); string(b) != "<c/>" {
		t.Errorf("staged copy = %q", b)
	}
}

func TestNPaStylesheet(t *testing.T) {
	const (
		inv = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">`
		ga  = "urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0" + gaccountMarker
	)
	for _, tc := range []struct{ doc, want string }{
		{`<rsm:CrossIndustryInvoice xmlns:rsm="` + nsCII + `"/>`, xslCII},
		{inv + `<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</cbc:CustomizationID></Invoice>`, xslUBL},
		{inv + `<cbc:CustomizationID>` + ga + `</cbc:CustomizationID></Invoice>`, xslGAccount},
		{inv + `<x><cbc:CustomizationID>` + ga + `</cbc:CustomizationID></x><cbc:CustomizationID>other</cbc:CustomizationID></Invoice>`, xslUBL},
		{inv + `</Invoice>`, xslUBL},
		{`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`, xslUBL},
		{`<Order xmlns="urn:oasis:names:specification:ubl:schema:xsd:Order-2"/>`, ""},
		{`<rsm:CrossIndustryDocument xmlns:rsm="urn:x"/>`, ""},
		{`not XML`, ""},
	} {
		if got := npaStylesheet([]byte(tc.doc)); got != tc.want {
			t.Errorf("%.60s: %q, want %q", tc.doc, got, tc.want)
		}
	}
	if syntaxOf("/x/"+xslCII) != "cii" || syntaxOf(xslGAccount) != "ubl" {
		t.Error("syntaxOf")
	}
}

// testXSL is an XSLT in the form of the ISO skeleton: a pattern of EN 16931,
// and a pattern nlcius of two rules, the second of two assertions (one
// without id) on one line each.
const testXSL = `<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:svrl="http://purl.oclc.org/dsdl/svrl" version="2.0">
   <!--PATTERN EN16931-model-->
   <xsl:template match="/Invoice"
                 mode="M1">
      <xsl:choose>
         <xsl:when test="xs:decimal(Amount) &gt; 0"/>
         <xsl:otherwise>
            <svrl:failed-assert test="x">
               <xsl:attribute name="id">BR-CO-16</xsl:attribute>
               <xsl:attribute name="flag">fatal</xsl:attribute>
               <svrl:text>[BR-CO-16]-Amount due.</svrl:text>
            </svrl:failed-assert>
         </xsl:otherwise>
      </xsl:choose>
   </xsl:template>
   <!--PATTERN nlcius-->
   <xsl:template match="TypeCode[$si]"
                 mode="M2">
      <xsl:choose>
         <xsl:when test=". = 380"/>
         <xsl:otherwise>
            <svrl:failed-assert test=". = 380">
               <xsl:attribute name="id">BR-NL-7</xsl:attribute>
               <xsl:attribute name="flag">fatal</xsl:attribute>
               <xsl:attribute name="location"><xsl:apply-templates select="." mode="path"/></xsl:attribute>
               <svrl:text>[BR-NL-7] The type
                  code.</svrl:text>
            </svrl:failed-assert>
         </xsl:otherwise>
      </xsl:choose>
   </xsl:template>
   <xsl:template match="Note" mode="M2">
      <xsl:choose>
         <xsl:when test="false()"/>
         <xsl:otherwise>
            <svrl:failed-assert test="false()">
               <xsl:attribute name="flag">warning</xsl:attribute>
               <svrl:text>[BR-NL-34] A note.</svrl:text>
            </svrl:failed-assert>
         </xsl:otherwise>
      </xsl:choose>
      <xsl:if test="@x">
         <svrl:successful-report test="@x">
            <xsl:attribute name="id">BR-NL-99</xsl:attribute>
            <svrl:text>Reported.</svrl:text>
         </svrl:successful-report>
      </xsl:if>
   </xsl:template>
</xsl:stylesheet>
`

func TestIndexXSL(t *testing.T) {
	file := filepath.Join(t.TempDir(), xslCII)
	os.WriteFile(file, []byte(testXSL), 0o644)
	idx, err := indexXSL(file)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range idx.asserts {
		got = append(got, fmt.Sprintf("%s %s %s %s %d-%d", a.Pattern, a.ID, a.Flag, a.Message, a.from, a.to))
	}
	want := []string{
		"EN16931-model BR-CO-16 fatal [BR-CO-16]-Amount due. 7-7",
		"nlcius BR-NL-7 fatal [BR-NL-7] The type code. 21-21",
		"nlcius BR-NL-34 warning [BR-NL-34] A note. 35-35",
		"nlcius BR-NL-99  Reported. 43-43",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assertions:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The template of the rule Note holds two assertions: its lines locate
	// none. The lines of an svrl:failed-assert locate its assertion.
	for line, want := range map[int]string{7: "BR-CO-16", 4: "BR-CO-16", 5: "BR-CO-16", 8: "BR-CO-16", 9: "BR-CO-16", 21: "BR-NL-7", 19: "BR-NL-7", 18: "BR-NL-7", 35: "BR-NL-34", 43: "BR-NL-99", 33: "", 34: "", 15: ""} {
		a, ok := idx.at(line)
		if a.ID != want || ok != (want != "") {
			t.Errorf("at(%d) = %s, %v, want %s", line, a.ID, ok, want)
		}
	}
	if _, err := indexXSL(filepath.Join(t.TempDir(), "none.xsl")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestParseSVRL(t *testing.T) {
	svrl := `<svrl:schematron-output xmlns:svrl="http://purl.oclc.org/dsdl/svrl">
  <svrl:active-pattern id="nlcius"/>
  <svrl:fired-rule context="TypeCode"/>
  <svrl:failed-assert test=". = 380" id="BR-NL-7" flag="fatal" location="/a"><svrl:text>[BR-NL-7] The type
     code.</svrl:text></svrl:failed-assert>
  <svrl:failed-assert test="false()" flag="warning" location="/b"><svrl:text>[BR-NL-34] A note.</svrl:text></svrl:failed-assert>
  <svrl:successful-report test="@x" id="BR-NL-99" location="/c"><svrl:text>Reported.</svrl:text></svrl:successful-report>
</svrl:schematron-output>`
	rep, err := parseSVRL([]byte(svrl))
	if err != nil {
		t.Fatal(err)
	}
	want := []svrlFinding{{"BR-NL-7", "fatal", "[BR-NL-7] The type code.", "/a"}, {"BR-NL-34", "warning", "[BR-NL-34] A note.", "/b"}, {"BR-NL-99", "", "Reported.", "/c"}}
	if !reflect.DeepEqual(rep.findings, want) || rep.errCode != "" {
		t.Errorf("findings = %q", rep.findings)
	}
	rep, err = parseSVRL([]byte(`<error code="FORG0001" line="21">Cannot convert "38O"</error>`))
	if err != nil || rep.errCode != "FORG0001" || rep.errLine != 21 || rep.errMsg != `Cannot convert "38O"` {
		t.Errorf("error report: %+v, %v", rep, err)
	}
	if _, err := parseSVRL([]byte(`<rep:report xmlns:rep="x"/>`)); err == nil {
		t.Error("not an SVRL report: no error")
	}
	if assertionID("", "no bracket") != "" || assertionID("", "[SI-INV-R000]-This") != "SI-INV-R000" || assertionID("A", "[B] x") != "A" {
		t.Error("assertionID")
	}
	for flag, want := range map[string]string{"fatal": levelError, "warning": levelWarning, "information": levelInfo} {
		if got := flagLevel(flag); got != want {
			t.Errorf("flagLevel(%s) = %s", flag, got)
		}
	}
}

func TestDiffLevels(t *testing.T) {
	mine := map[string]string{"BR-NL-19": levelWarning, "BR-NL-2": levelError, "BR-NL-3": levelWarning, "BR-CO-10": levelError}
	ref := result{Rules: map[string]string{"BR-NL-2": levelError, "BR-NL-3": levelError, "BR-NL-20": levelWarning, "BR-CO-10": levelError}}
	got := diff(mine, ref, true)
	if want := []string{"+BR-NL-19", "-BR-NL-20", "-BR-NL-3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("diff = %q, want %q", got, want)
	}
	c := comparison{Ours: result{Rules: mine}, Ref: ref}
	for d, want := range map[string]string{
		"+BR-NL-19": "`+BR-NL-19` (warning / —)",
		"-BR-NL-20": "`-BR-NL-20` (— / warning)",
		"-BR-NL-3":  "`-BR-NL-3` (warning / error)",
		"!npa: x":   "`!npa: x`",
	} {
		if got := level(c, d); got != want {
			t.Errorf("level(%s) = %s, want %s", d, got, want)
		}
	}
	// A reference stopped on a dynamic error: its assertion only.
	stopped := result{Rules: map[string]string{"BR-CO-16": levelError}, Stopped: "FORG0001"}
	if got := diff(mine, stopped, true); !reflect.DeepEqual(got, []string{"-BR-CO-16"}) {
		t.Errorf("stopped: %q", got)
	}
	if got := diff(map[string]string{"BR-CO-16": levelError, "BR-NL-7": levelError}, stopped, true); len(got) != 0 {
		t.Errorf("stopped, raised: %q", got)
	}
}

// TestNPaRun compares a run of the test XSLT: reports, a dynamic error, and
// the assertions of the nlcius pattern that never fail.
func TestNPaRun(t *testing.T) {
	dir := t.TempDir()
	xsl := filepath.Join(dir, xslCII)
	os.WriteFile(xsl, []byte(testXSL), 0o644)
	os.MkdirAll(filepath.Join(dir, "npa"), 0o755)
	os.WriteFile(filepath.Join(dir, "npa.list"), []byte(xsl+"\t"+filepath.Join(dir, "in", "0001.xml")+"\n"+xsl+"\t"+filepath.Join(dir, "in", "0002.xml")+"\n"), 0o644)
	run, err := newXSLRun(dir, "npa.list")
	if err != nil {
		t.Fatal(err)
	}
	r, err := run.result(entry{Name: "0001.xml"}, []byte(`<svrl:schematron-output xmlns:svrl="http://purl.oclc.org/dsdl/svrl">
<svrl:failed-assert id="BR-CO-16" flag="fatal"><svrl:text>[BR-CO-16]-Amount due.</svrl:text></svrl:failed-assert>
<svrl:failed-assert flag="warning"><svrl:text>[BR-NL-34] A note.</svrl:text></svrl:failed-assert>
</svrl:schematron-output>`))
	if want := map[string]string{"BR-CO-16": levelError, "BR-NL-34": levelWarning}; err != nil || !reflect.DeepEqual(r.Rules, want) || r.Note != xslCII {
		t.Errorf("0001: %+v, %v", r, err)
	}
	r, err = run.result(entry{Name: "0002.xml"}, []byte(`<error code="FORG0001" line="19">Cannot convert</error>`))
	if err != nil || !reflect.DeepEqual(r.Rules, map[string]string{"BR-NL-7": levelError}) || !strings.HasPrefix(r.Stopped, "FORG0001 in the test of BR-NL-7") {
		t.Errorf("0002: %+v, %v", r, err)
	}
	r, err = run.result(entry{Name: "0002.xml"}, []byte(`<error code="XTDE0000" line="33">Unknown</error>`))
	if err != nil || r.Failure == "" || len(r.Rules) != 0 {
		t.Errorf("unlocated error: %+v, %v", r, err)
	}
	if _, err := run.result(entry{Name: "0003.xml"}, nil); err == nil {
		t.Error("invoice not in npa.list: no error")
	}
	never := run.neverFired()
	if len(never) != 1 || never[0].ID != "BR-NL-99" || never[0].where() != "cii/nlcius" {
		t.Errorf("never fired = %+v", never)
	}

	// No npa.list: no NLCIUS comparison.
	run, err = newXSLRun(t.TempDir(), "npa.list")
	if err != nil || len(run.neverFired()) != 0 {
		t.Errorf("no npa.list: %+v, %v", run, err)
	}
}

func TestKnownNeverFired(t *testing.T) {
	name := filepath.Join(t.TempDir(), "known.txt")
	os.WriteFile(name, []byte("npa ubl/nlcius ~BR-NL-32-* never applies\n"), 0o644)
	known, err := readKnown(name)
	if err != nil {
		t.Fatal(err)
	}
	n := neverFired{Syntax: "ubl", Pattern: "nlcius", ID: "BR-NL-32-2"}
	if !known[0].match("npa", n.where(), "~"+n.ID) || known[0].match("npa", "cii/nlcius", "~"+n.ID) || known[0].match("npa", n.where(), "-"+n.ID) {
		t.Error("~ line does not match as it should")
	}
}

// TestOursNL runs the package on the NLCIUS invoices of the repository,
// which give no finding, and on an invoice without buyer reference.
func TestOursNL(t *testing.T) {
	for _, f := range []string{"nlcius.xml", "nlcius.ubl.xml", "gaccount.ubl.xml"} {
		data, err := os.ReadFile(filepath.Join("../../internal/rules/nl/testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if r := oursNL(f, data); len(r.Rules) != 0 || r.Failure != "" {
			t.Errorf("%s: %+v", f, r)
		}
		if f == "nlcius.ubl.xml" {
			bad := strings.Replace(string(data), "<cbc:BuyerReference>", "<cbc:Note/><cbc:BuyerReference>", 1)
			if r := oursNL(f, []byte(bad)); r.Rules["SI-UBL-2"] != levelWarning {
				t.Errorf("%s with an empty note: %+v", f, r)
			}
		}
	}
}

func TestStageNL(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	tests, repo, variants, npaDir := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
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
	ubl := `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`
	write(corpus, "XML-Rechnung/CII/a.cii.xml", cii)
	write(tests, "SI-UBL-2.0.3.2/SI-UBL-2.0_ok_base.xml", ubl)
	write(tests, "SI-UBL-2.0.3.2/README", "x")
	write(tests, "SI-UBL-1.2/old.xml", ubl)
	write(tests, "NLCIUS-CII-1.0.3/NLCIUS-CII_ok_example.xml", cii)
	os.MkdirAll(filepath.Join(tests, "si-ubl-2.0-ext-gaccount-1.0"), 0o755)
	write(repo, "internal/rules/nl/testdata/nlcius.xml", cii)
	write(variants, "ubl-001-x.xml", ubl)
	nl := nlInputs{npa: npaDir, tests: tests, repo: repo, variants: variants}
	if err := stage(corpus, kosit, work, nl, ptInputs{}, fnfeInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "corpus/XML-Rechnung/CII/a.cii.xml"},
		{"0002.xml", "nl/simplerinvoicing/NLCIUS-CII-1.0.3/NLCIUS-CII_ok_example.xml"},
		{"0003.xml", "nl/simplerinvoicing/SI-UBL-2.0.3.2/SI-UBL-2.0_ok_base.xml"},
		{"0004.xml", "nl/variants/ubl-001-x.xml"},
		{"0005.xml", "nl/zerechnung/internal/rules/nl/testdata/nlcius.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if entries[0].nl() || !entries[1].nl() {
		t.Error("nl() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "npa.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], filepath.Join(npaDir, "xsl", xslCII)+"\t") ||
		!strings.HasPrefix(lines[1], filepath.Join(npaDir, "xsl", xslUBL)+"\t") || !strings.HasSuffix(lines[3], "0005.xml") {
		t.Errorf("npa.list:\n%s", list)
	}
	for _, l := range []string{"kosit.list", "mustang.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); strings.Count(string(b), "\n") != 1 {
			t.Errorf("%s:\n%s", l, b)
		}
	}

	write(variants, "bad.xml", "<Order/>")
	if err := stage(corpus, kosit, work, nl, ptInputs{}, fnfeInputs{}, ""); err == nil {
		t.Error("a variant that is not an invoice: no error")
	}
}

func TestSlices(t *testing.T) {
	m := map[string]bool{"b": true, "a": false}
	if got := sortedRules(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("sortedRules = %q", got)
	}
}

// testSchXslt has the shape of an XSLT that SchXslt 1.10.1 compiles from a
// schematron: an xsl:if per assertion, around its svrl:failed-assert (or,
// for a report, its svrl:successful-report).
const testSchXslt = `<xsl:transform xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:schxslt="https://doi.org/10.5281/zenodo.1495494" version="2.0">
   <xsl:template match="cbc:PayableAmount" priority="2" mode="d19e21">
      <schxslt:rule pattern="d19e21">
         <svrl:fired-rule xmlns:svrl="http://purl.oclc.org/dsdl/svrl">
            <xsl:if test="exists($documentUri)">
               <xsl:attribute name="document" select="$documentUri"/>
            </xsl:if>
         </svrl:fired-rule>
         <xsl:if test="not(xs:decimal(.) >= 0)">
            <svrl:failed-assert xmlns:svrl="http://purl.oclc.org/dsdl/svrl"
                                location="{schxslt:location(.)}"
                                flag="fatal"
                                id="BR-CO-25">
               <svrl:text>[BR-CO-25]-Amount
                  due.</svrl:text>
            </svrl:failed-assert>
         </xsl:if>
         <xsl:if
             test="not(count(.) = 1)">
            <svrl:failed-assert xmlns:svrl="http://purl.oclc.org/dsdl/svrl" location="{schxslt:location(.)}" flag="warning">
               <svrl:text>[UBL-SR-01]-Once.</svrl:text>
            </svrl:failed-assert>
         </xsl:if>
         <xsl:if test="@x">
            <svrl:successful-report xmlns:svrl="http://purl.oclc.org/dsdl/svrl" location="{schxslt:location(.)}" id="DT-01">
               <svrl:text>Reported.</svrl:text>
            </svrl:successful-report>
         </xsl:if>
      </schxslt:rule>
   </xsl:template>
</xsl:transform>
`

func TestIndexSchXslt(t *testing.T) {
	file := filepath.Join(t.TempDir(), ptXSL)
	os.WriteFile(file, []byte(testSchXslt), 0o644)
	idx, err := indexSchXslt(file)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range idx.asserts {
		got = append(got, fmt.Sprintf("%s %s %s %d-%d", a.ID, a.Flag, a.Message, a.from, a.to))
	}
	want := []string{
		"BR-CO-25 fatal [BR-CO-25]-Amount due. 9-9",
		"UBL-SR-01 warning [UBL-SR-01]-Once. 18-19",
		"DT-01  Reported. 24-24",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assertions:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for line, want := range map[int]string{9: "BR-CO-25", 19: "UBL-SR-01", 18: "UBL-SR-01", 24: "DT-01", 8: "", 5: "", 12: "", 2: ""} {
		a, ok := idx.at(line)
		if a.ID != want || ok != (want != "") {
			t.Errorf("at(%d) = %s, %v, want %s", line, a.ID, ok, want)
		}
	}
	empty := filepath.Join(t.TempDir(), "empty.xsl")
	os.WriteFile(empty, []byte(`<xsl:transform xmlns:xsl="http://www.w3.org/1999/XSL/Transform"/>`), 0o644)
	if _, err := indexSchXslt(empty); err == nil {
		t.Error("no assertion: no error")
	}
	if _, err := indexSchXslt(filepath.Join(t.TempDir(), "none.xsl")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestPTPaths(t *testing.T) {
	for loc, want := range map[string]string{
		"/Q{urn:oasis:names:specification:ubl:schema:xsd:Invoice-2}Invoice[1]":                                                                                           "/Invoice[1]",
		"/Q{urn:oasis:names:specification:ubl:schema:xsd:Invoice-2}Invoice[1]/Q{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}InvoiceLine[2]": "/Invoice[1]/InvoiceLine[2]",
		"/Q{http://example.com/a/b}Root[1]/Q{}Plain[3]/@Q{}currencyID":                                                                                                   "/Root[1]/Plain[3]/@currencyID",
		"/": "/",
	} {
		if got, err := saxonPath(loc); err != nil || got != want {
			t.Errorf("saxonPath(%s) = %s, %v, want %s", loc, got, err, want)
		}
	}
	for _, bad := range []string{"Invoice[1]", "/Q{urn:x", "/Invoice[1]//ID[1]"} {
		if _, err := saxonPath(bad); err == nil {
			t.Errorf("saxonPath(%s): no error", bad)
		}
	}
	for p, want := range map[string]string{
		"/Invoice":                            "/Invoice[1]",
		"/Invoice/cac:InvoiceLine[2]/cbc:ID":  "/Invoice[1]/InvoiceLine[2]/ID[1]",
		"/CreditNote/cac:TaxTotal/x:Other[3]": "/CreditNote[1]/TaxTotal[1]/Other[3]",
		"":                                    "/",
	} {
		if got := ptPath(p); got != want {
			t.Errorf("ptPath(%s) = %s, want %s", p, got, want)
		}
	}
	got := unlocated(map[string]string{"BR-61@/Invoice[1]/PaymentMeans[1]": levelWarning, "BR-61@/Invoice[1]/PaymentMeans[2]": levelError, "BR-01@/Invoice[1]": levelError})
	if want := map[string]string{"BR-61": levelError, "BR-01": levelError}; !reflect.DeepEqual(got, want) {
		t.Errorf("unlocated = %v", got)
	}
}

// TestPTRun compares a run of the test XSLT: reports on elements, a dynamic
// error in the test of an assertion, and one that locates no assertion.
func TestPTRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ptXSL), []byte(testSchXslt), 0o644)
	os.WriteFile(filepath.Join(dir, "pt.list"), []byte(filepath.Join(dir, "in", "0001.xml")+"\n"+filepath.Join(dir, "in", "0002.xml")+"\n"), 0o644)
	run, err := newPTRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	const inv = "/Q{urn:oasis:names:specification:ubl:schema:xsd:Invoice-2}Invoice[1]"
	r, err := run.result(entry{Name: "0001.xml"}, []byte(`<schematron-output xmlns="http://purl.oclc.org/dsdl/svrl">
<failed-assert id="BR-CO-25" flag="fatal" location="`+inv+`/Q{urn:x}LegalMonetaryTotal[1]/Q{urn:y}PayableAmount[1]"><text>[BR-CO-25]-Amount due.</text></failed-assert>
<failed-assert id="UBL-SR-01" flag="warning" location="`+inv+`"><text>[UBL-SR-01]-Once.</text></failed-assert>
<failed-assert id="UBL-SR-01" flag="warning" location="`+inv+`"><text>[UBL-SR-01]-Once.</text></failed-assert>
</schematron-output>`))
	want := map[string]string{"BR-CO-25@/Invoice[1]/LegalMonetaryTotal[1]/PayableAmount[1]": levelError, "UBL-SR-01@/Invoice[1]": levelWarning}
	if err != nil || !reflect.DeepEqual(r.Rules, want) || r.Stopped != "" {
		t.Errorf("0001: %+v, %v", r, err)
	}
	r, err = run.result(entry{Name: "0002.xml"}, []byte(`<error code="FORG0001" line="9">Cannot convert string "X y" to xs:decimal</error>`))
	if err != nil || !reflect.DeepEqual(r.Rules, map[string]string{"BR-CO-25": levelError}) || !strings.HasPrefix(r.Stopped, "FORG0001 in the test of BR-CO-25") {
		t.Errorf("0002: %+v, %v", r, err)
	}
	r, err = run.result(entry{Name: "0002.xml"}, []byte(`<error code="XTDE0000" line="2">Unknown</error>`))
	if err != nil || r.Failure == "" || len(r.Rules) != 0 {
		t.Errorf("unlocated error: %+v, %v", r, err)
	}
	if _, err := run.result(entry{Name: "0002.xml"}, []byte(`<schematron-output><failed-assert id="X" location="Invoice"/></schematron-output>`)); err == nil {
		t.Error("bad location: no error")
	}
	if _, err := run.result(entry{Name: "0003.xml"}, nil); err == nil {
		t.Error("invoice not in pt.list: no error")
	}

	// A stopped transformation: the assertion only, whatever its element.
	stopped := result{Rules: map[string]string{"BR-CO-25": levelError}, Stopped: "FORG0001"}
	mine := map[string]string{"BR-CO-25@/Invoice[1]/LegalMonetaryTotal[1]/PayableAmount[1]": levelError, "BR-CO-13@/Invoice[1]": levelError}
	if got := diff(unlocated(mine), stopped, true); len(got) != 0 {
		t.Errorf("stopped, raised: %q", got)
	}
	if got := diff(mine, result{Rules: want}, true); !reflect.DeepEqual(got, []string{"+BR-CO-13@/Invoice[1]", "-UBL-SR-01@/Invoice[1]"}) {
		t.Errorf("located diff: %q", got)
	}

	// No pt.list: no CIUS-PT comparison, and no XSLT to read.
	if run, err := newPTRun(t.TempDir()); err != nil || len(run.files) != 0 {
		t.Errorf("no pt.list: %+v, %v", run, err)
	}
	empty := t.TempDir()
	os.WriteFile(filepath.Join(empty, "pt.list"), []byte(filepath.Join(empty, "0001.xml")+"\n"), 0o644)
	if _, err := newPTRun(empty); err == nil {
		t.Error("pt.list without the compiled XSLT: no error")
	}
}

// TestOursPT runs the package on the CIUS-PT invoices of the repository,
// which give no finding, and on variants, including one without BT-24, which
// the package still checks as a CIUS-PT invoice.
func TestOursPT(t *testing.T) {
	for _, f := range []string{"cius-pt.ubl.xml", "cius-pt-nc.ubl.xml", "cius-pt-rich.ubl.xml"} {
		data, err := os.ReadFile(filepath.Join("../../internal/rules/pt/testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if r := oursPT(f, data); len(r.Rules) != 0 || r.Failure != "" {
			t.Errorf("%s: %+v", f, r)
		}
		if f != "cius-pt.ubl.xml" {
			continue
		}
		noID := strings.Replace(string(data), "<cbc:ID>FT ZR2026/41</cbc:ID>", "", 1)
		if r := oursPT(f, []byte(noID)); !reflect.DeepEqual(r.Rules, map[string]string{"BR-02@/Invoice[1]": levelError}) {
			t.Errorf("%s without BT-1: %+v", f, r.Rules)
		}
		noBT24 := regexp.MustCompile(`<cbc:CustomizationID>[^<]*</cbc:CustomizationID>`).ReplaceAllString(string(data), "")
		if r := oursPT(f, []byte(noBT24)); !reflect.DeepEqual(r.Rules, map[string]string{"BR-01@/Invoice[1]": levelError}) {
			t.Errorf("%s without BT-24: %+v", f, r.Rules)
		}
		badNIF := strings.Replace(string(data), "<cbc:CompanyID>PT501234560<", "<cbc:CompanyID>PT501234561<", 1)
		if r := oursPT(f, []byte(badNIF)); !slices.Contains(r.Other, "PT-NIF-02") || len(r.Rules) != 0 {
			t.Errorf("%s with a bad NIF: %+v", f, r)
		}
	}
}

func TestStagePT(t *testing.T) {
	corpus, kosit, work := t.TempDir(), t.TempDir(), t.TempDir()
	repo, variants := t.TempDir(), t.TempDir()
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
	write(corpus, "XML-Rechnung/UBL/a.ubl.xml", ubl)
	write(repo, "internal/rules/pt/testdata/cius-pt.ubl.xml", ubl)
	write(variants, "v00001.xml", ubl)
	write(variants, "manifest.tsv", "v00001.xml\tx")
	examples := filepath.Join(t.TempDir(), "examples.zip")
	f, _ := os.Create(examples)
	z := zip.NewWriter(f)
	for name, content := range map[string]string{"I_CIUS-PT_syntax-model - F.xml": ubl, "Casos de Uso.xlsx": "x", "dir/": ""} {
		w, _ := z.Create(name)
		w.Write([]byte(content))
	}
	z.Close()
	f.Close()
	pt := ptInputs{examples: examples, repo: repo, variants: variants}
	if err := stage(corpus, kosit, work, nlInputs{}, pt, fnfeInputs{}, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := readIndex(work)
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{
		{"0001.xml", "corpus/XML-Rechnung/UBL/a.ubl.xml"},
		{"0002.xml", "pt/espap/I_CIUS-PT_syntax-model - F.xml"},
		{"0003.xml", "pt/variants/v00001.xml"},
		{"0004.xml", "pt/zerechnung/internal/rules/pt/testdata/cius-pt.ubl.xml"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q", entries)
	}
	if entries[0].pt() || entries[0].national() || !entries[1].pt() || !entries[1].national() {
		t.Error("pt() or national() is wrong")
	}
	list, _ := os.ReadFile(filepath.Join(work, "pt.list"))
	lines := strings.Split(strings.TrimSpace(string(list)), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "0002.xml") || !strings.HasSuffix(lines[2], "0004.xml") {
		t.Errorf("pt.list:\n%s", list)
	}
	if b, _ := os.ReadFile(filepath.Join(work, "in", "0002.xml")); string(b) != ubl {
		t.Errorf("staged example = %q", b)
	}
	for _, l := range []string{"kosit.list", "mustang.list"} {
		if b, _ := os.ReadFile(filepath.Join(work, l)); strings.Count(string(b), "\n") != 1 {
			t.Errorf("%s:\n%s", l, b)
		}
	}
	pt.examples = filepath.Join(t.TempDir(), "none.zip")
	if err := stage(corpus, kosit, work, nlInputs{}, pt, fnfeInputs{}, ""); err == nil {
		t.Error("missing archive: no error")
	}
}

// TestPTSection checks the CIUS-PT part of the report: files by origin and
// stops by error code.
func TestPTSection(t *testing.T) {
	all := []comparison{
		{Reference: "pt", Invoice: "pt/espap/a.xml"},
		{Reference: "pt", Invoice: "pt/zerechnung/internal/rules/pt/testdata/b.xml"},
		{Reference: "pt", Invoice: "pt/variants/v00001.xml", Ref: result{Rules: map[string]string{"BR-27": levelError}, Stopped: "FORG0001 in the test of BR-27: x"}},
		{Reference: "pt", Invoice: "pt/variants/v00002.xml", Ref: result{Rules: map[string]string{"BR-27": levelError}, Stopped: "FORG0001 in the test of BR-27: y"}},
		{Reference: "pt", Invoice: "pt/variants/v00003.xml", Ref: result{Rules: map[string]string{"BR-61": levelError}, Stopped: "XPTY0004 in the test of BR-61: z"}},
		{Reference: "pt", Invoice: "pt/variants/v00004.xml", Unexpected: []string{"+BR-01@/Invoice[1]"}},
		{Reference: "npa", Invoice: "nl/variants/x.xml"},
	}
	var b strings.Builder
	ptSection(&b, all)
	md := b.String()
	for _, want := range []string{
		"| Exemples d'eSPap (`FX_PT`) | 1 | 1 | 0 | 0 | 0 |",
		"| Factures du dépôt (`internal/rules/pt/testdata`) | 1 | 1 | 0 | 0 | 0 |",
		" | 4 | 0 | 3 | 0 | 1 |",
		"| `FORG0001` : valeur qui ne se convertit pas en nombre | 2 | `BR-27` (2) |",
		"| `XPTY0004` : plusieurs valeurs là où le test n'en attend qu'une | 1 | `BR-61` (1) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	b.Reset()
	ptSection(&b, all[6:])
	if b.Len() != 0 {
		t.Errorf("no CIUS-PT file: %q", b.String())
	}
}
