package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestUpToDate checks that rules_gen.go is the output of the generator for
// the schematron of the checkout of France_RFE in FX_FNFE
// (conformance/fetch.sh).
func TestUpToDate(t *testing.T) {
	root := os.Getenv("FX_FNFE")
	if root == "" {
		t.Skip("FX_FNFE not set")
	}
	data, err := os.ReadFile(filepath.Join(root, dir, mainFile))
	if err != nil {
		t.Fatal(err)
	}
	got, err := generate(data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../rules_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("rules_gen.go is not up to date: run go generate in internal/rules/flux10")
	}
}

// TestGenerateErrors checks that the generator refuses a file whose SHA-256
// is not the one of the schematron, a file that is not a schematron, and the
// constructs that the tables cannot reproduce.
func TestGenerateErrors(t *testing.T) {
	if _, err := generate([]byte("<schema/>")); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("generate: %v, want a SHA-256 error", err)
	}
	const head = `<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt2">`
	const msg = `[G1.01] Message. | Source : Annexe 7 v1.8 G1.01`
	for _, doc := range []string{
		"<x",
		`<schema xmlns="urn:x" queryBinding="xslt2"/>`,
		`<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt"/>`,
		head + `</schema>`,
		head + `<ns prefix="a" uri="urn:a"/></schema>`,
		head + `<pattern><rule context="/a"><assert test="b" flag="fatal">` + msg + `</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"/><rule context="/b"/></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert id="X" test="b" flag="fatal">` + msg + `</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert test="b" flag="info">` + msg + `</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert test="b" flag="fatal">Message.</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert test="b" flag="fatal">[G1.01] <value-of select="c"/> | Source : Annexe 7 v1.8 G1.01</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert test="b" flag="fatal">[G1.01] <emph>c</emph>. | Source : Annexe 7 v1.8 G1.01</assert></rule></pattern></schema>`,
		head + `<pattern id="P"><rule context="/a"><assert test="b" flag="fatal">` + msg + `</assert><let name="v" value="1"/></rule></pattern></schema>`,
	} {
		if _, err := read([]byte(doc)); err == nil {
			t.Errorf("read accepted %s", doc)
		}
	}
}

// TestRead checks the identifiers of the assertions, their messages and the
// descriptions of the catalog on a small schematron.
func TestRead(t *testing.T) {
	s, err := read([]byte(`<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt2"><title>T</title>
<pattern id="F10-A"><rule context="/R/A[@x = 'y'] |
   /R/B"><let name="v" value="string(.)"/>
  <assert test="not(contains($v, '  '))
     and true()" flag="fatal">[G1.05] <name/> : 100 %. | Source : Annexe 7 v1.8 G1.05</assert>
  <assert test="$v" flag="fatal">[G1.53] Total (<value-of select="$v"/>) égal à la somme (<value-of select="sum(.)"/>). | Source : Annexe 7 v1.8 G1.53</assert>
</rule></pattern></schema>`))
	if err != nil {
		t.Fatal(err)
	}
	r := s.patterns[0].rule
	if s.title != "T" || r.context != "/R/A[@x = 'y'] | /R/B" || len(r.asserts) != 2 {
		t.Fatalf("%+v", s)
	}
	a, b := r.asserts[0], r.asserts[1]
	if a.id != "F10-A" || a.test != "not(contains($v, '  '))\n     and true()" || a.msg != "[G1.05] %s : 100 %%. | Source : Annexe 7 v1.8 G1.05" ||
		!slices.Equal(a.values, []string{"name()"}) || a.desc != "[G1.05] A ou B : 100 %." {
		t.Errorf("first assertion %+v", a)
	}
	if b.id != "F10-A-2" || !slices.Equal(b.values, []string{"$v", "sum(.)"}) || b.desc != "[G1.53] Total égal à la somme." {
		t.Errorf("second assertion %+v", b)
	}
	if got := squeeze(a.test); got != "not(contains($v, '  ')) and true()" {
		t.Errorf("squeeze: %q", got)
	}
	if got := squeeze(` a  =  "b  c" `); got != `a = "b  c"` {
		t.Errorf("squeeze: %q", got)
	}
}
