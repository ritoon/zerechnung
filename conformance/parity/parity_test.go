package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	if err := stage(corpus, kosit, work, nlInputs{}, ""); err != nil {
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
	// none.
	for line, want := range map[int]string{7: "BR-CO-16", 4: "BR-CO-16", 5: "BR-CO-16", 21: "BR-NL-7", 19: "BR-NL-7", 18: "BR-NL-7", 35: "BR-NL-34", 43: "BR-NL-99", 33: "", 34: "", 8: ""} {
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
	want := []svrlFinding{{"BR-NL-7", "fatal", "[BR-NL-7] The type code."}, {"BR-NL-34", "warning", "[BR-NL-34] A note."}, {"BR-NL-99", "", "Reported."}}
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
	run, err := newNPaRun(dir)
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
	run, err = newNPaRun(t.TempDir())
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
	if err := stage(corpus, kosit, work, nl, ""); err != nil {
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
	if err := stage(corpus, kosit, work, nl, ""); err == nil {
		t.Error("a variant that is not an invoice: no error")
	}
}

func TestSlices(t *testing.T) {
	m := map[string]bool{"b": true, "a": false}
	if got := sortedRules(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("sortedRules = %q", got)
	}
}
