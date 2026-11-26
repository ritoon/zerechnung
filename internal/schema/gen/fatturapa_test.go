package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// fpaSchema is a small schema in the style of the FatturaPA one: unqualified
// local elements, a choice of sequences in a sequence, bounds, a default
// value, block escapes and a reference to the XML signature.
const fpaSchema = `<?xml version="1.0" encoding="utf-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:ds="http://www.w3.org/2000/09/xmldsig#"
    xmlns="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"
    targetNamespace="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2" version="1.2.4">
  <xs:import namespace="http://www.w3.org/2000/09/xmldsig#" schemaLocation="http://www.w3.org/TR/2002/REC-xmldsig-core-20020212/xmldsig-core-schema.xsd"/>
  <xs:element name="FatturaElettronica" type="FatturaElettronicaType"/>
  <xs:complexType name="FatturaElettronicaType">
    <xs:sequence>
      <xs:element name="Anagrafica" type="AnagraficaType"/>
      <xs:element name="NumeroLinea" type="NumeroLineaType"/>
      <xs:element name="Data" type="DataType"/>
      <xs:element name="Nazione" type="NazioneType" default="IT"/>
      <xs:element name="IdPaese" type="NazioneType"/>
      <xs:element ref="ds:Signature" minOccurs="0"/>
    </xs:sequence>
    <xs:attribute name="versione" type="FormatoType" use="required"/>
  </xs:complexType>
  <xs:complexType name="AnagraficaType">
    <xs:sequence>
      <xs:choice>
        <xs:sequence>
          <xs:element name="Denominazione" type="LatinType"/>
        </xs:sequence>
        <xs:sequence>
          <xs:element name="Nome" type="LatinType"/>
          <xs:element name="Cognome" type="LatinType"/>
        </xs:sequence>
      </xs:choice>
      <xs:element name="Titolo" type="LatinType" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="LatinType">
    <xs:restriction base="xs:normalizedString">
      <xs:pattern value="[\p{IsBasicLatin}\p{IsLatin-1Supplement}]{1,80}"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="NumeroLineaType">
    <xs:restriction base="xs:integer">
      <xs:minInclusive value="1"/>
      <xs:maxInclusive value="9999"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="DataType">
    <xs:restriction base="xs:date">
      <xs:minInclusive value="1970-01-01"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="NazioneType">
    <xs:restriction base="xs:string">
      <xs:pattern value="(\p{IsBasicLatin}{2})"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="FormatoType">
    <xs:restriction base="xs:string">
      <xs:enumeration value="FPA12"/>
      <xs:enumeration value="FPR12"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>
`

func compileTestFPA(t *testing.T, edits ...string) ([]byte, error) {
	t.Helper()
	s := fpaSchema
	for i := 0; i+1 < len(edits); i += 2 {
		if !strings.Contains(s, edits[i]) {
			t.Fatalf("edit: %q not found", edits[i])
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	return compileFatturaPA(fstest.MapFS{"fpa.xsd": &fstest.MapFile{Data: []byte(s)}}, "fpa.xsd", "1.2.4")
}

func TestGenerateFatturaPA(t *testing.T) {
	src, err := compileTestFPA(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "fatturapa_tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		`const fpaSchemaVersion = "1.2.4"`,
		`{name: "ds:Signature (not checked)", content: contentSkip}`,
		`{"Signature", 2, 0, 0, 1}`,
		`{"FatturaElettronica", 1, `,
		`{"Denominazione", 0, `, // unqualified
		`altFirst: 0, altN: 2`,
		"{1, 0, 1, 0x01}, // p:AnagraficaType",
		"{1, 1, 2, 0x03}, // p:AnagraficaType",
		`patterns: []string{"[\\x{0}-\\x{7F}\\x{80}-\\x{FF}]{1,80}"}`,
		`patterns: []string{"([\\x{0}-\\x{7F}]{2})"}`,
		`minIncl: "1", maxIncl: "9999"`,
		`minIncl: "1970-01-01"`,
		`def: "IT"`,
		`{"versione", kString, `,
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	// The flattened elements of the choice are optional in the sequence.
	for _, name := range []string{"Denominazione", "Nome", "Cognome"} {
		if !regexp.MustCompile(`\{"` + name + `", 0, \d+, 0, 1\}`).Match(src) {
			t.Errorf("%s should be optional once flattened\n%s", name, src)
		}
	}
	if bytes.Count(src, []byte(`name: "p:NazioneType"`)) != 2 {
		t.Errorf("Nazione, with a default value, and IdPaese should have two types\n%s", src)
	}
	again, err := compileTestFPA(t)
	if err != nil || !bytes.Equal(src, again) {
		t.Errorf("second generation differs (err %v)", err)
	}
}

func TestUnsupportedFatturaPA(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edits []string
		err   string
	}{
		{"other block", []string{`\p{IsLatin-1Supplement}`, `\p{IsGreek}`}, "IsGreek"},
		{"bound of a string", []string{`<xs:pattern value="(\p{IsBasicLatin}{2})"/>`, `<xs:minInclusive value="AA"/>`}, "bounds of a kString type"},
		{"bad bound", []string{`<xs:maxInclusive value="9999"/>`, `<xs:maxInclusive value="9.5"/>`}, "not a kInteger value"},
		{"one alternative", []string{`<xs:sequence>
          <xs:element name="Nome" type="LatinType"/>
          <xs:element name="Cognome" type="LatinType"/>
        </xs:sequence>`, ``}, "two alternatives"},
		{"repeated choice", []string{`<xs:choice>`, `<xs:choice maxOccurs="2">`}, "repeated nested xs:choice"},
		{"wildcard in a choice", []string{`<xs:element name="Denominazione" type="LatinType"/>`, `<xs:any namespace="##other" processContents="lax"/>`}, "xs:any"},
		{"name twice", []string{`<xs:element name="Titolo" type="LatinType" minOccurs="0"/>`, `<xs:element name="Nome" type="LatinType" minOccurs="0"/>`}, "used twice"},
		{"default on a complex type", []string{`<xs:element name="Anagrafica" type="AnagraficaType"/>`, `<xs:element name="Anagrafica" type="AnagraficaType" default="x"/>`}, "default value"},
		{"other signature element", []string{`<xs:element ref="ds:Signature" minOccurs="0"/>`, `<xs:element ref="ds:Signature" minOccurs="0" nillable="true"/>`}, "nillable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileTestFPA(t, tc.edits...)
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.err)
			}
		})
	}
}

func TestGoPattern(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{`(\p{IsBasicLatin}{1,60})`, `([\x{0}-\x{7F}]{1,60})`},
		{`[\p{IsBasicLatin}\p{IsLatin-1Supplement}]{1,1000}`, `[\x{0}-\x{7F}\x{80}-\x{FF}]{1,1000}`},
		{`[\-]?[0-9]{1,11}\.[0-9]{2}`, `[\-]?[0-9]{1,11}\.[0-9]{2}`},
		{`\[\p{IsBasicLatin}`, `\[[\x{0}-\x{7F}]`},
	} {
		if got, err := goPattern(c.in); err != nil || got != c.out {
			t.Errorf("goPattern(%q) = %q, %v; want %q", c.in, got, err, c.out)
		}
	}
}

func TestFatturaPAChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), fpaFile)
	if err := os.WriteFile(path, []byte("not the FatturaPA schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generateFatturaPA(path); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v", err)
	}
}

// The FatturaPA schema 1.2.3, in force until 10/01/2027.
const (
	fpa123File = "Schema_VFPA12_V1.2.3.xsd"
	fpa123Sum  = "152944f6eef9f5d69ef6e955ee173b32142b00a8c1c5222fc97dfab5910e8a8c"
)

// TestUpToDateFatturaPA checks that fatturapa_tables_gen.go matches the
// schema 1.2.4 of the directory named by FX_FATTURAPA_XSD, and that the
// schema 1.2.3 of the same directory gives the same tables but for the tax
// regime RF21, which 1.2.4 adds.
func TestUpToDateFatturaPA(t *testing.T) {
	dir := os.Getenv("FX_FATTURAPA_XSD")
	if dir == "" {
		t.Skip("FX_FATTURAPA_XSD not set")
	}
	src, err := generateFatturaPA(filepath.Join(dir, fpaFile))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "fatturapa_tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, cur) {
		t.Error("fatturapa_tables_gen.go is out of date: run go generate -run -fatturapa in internal/schema")
	}

	data, err := os.ReadFile(filepath.Join(dir, fpa123File))
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != fpa123Sum {
		t.Fatalf("%s: SHA-256 %s, want %s", fpa123File, got, fpa123Sum)
	}
	old, err := compileFatturaPA(fstest.MapFS{fpaFile: &fstest.MapFile{Data: data}}, fpaFile, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := string(src)
	for _, r := range [][2]string{
		{`"RF20", "RF21"`, `"RF20"`},
		{`const fpaSchemaVersion = "1.2.4"`, `const fpaSchemaVersion = "1.2.3"`},
		{"from the FatturaPA 1.2.4 XML schemas", "from the FatturaPA 1.2.3 XML schemas"},
		{"//\t0b238434b0ad24c4 ", "//\t152944f6eef9f5d6 "},
	} {
		if !strings.Contains(want, r[0]) {
			t.Fatalf("%q not in the tables of 1.2.4", r[0])
		}
		want = strings.Replace(want, r[0], r[1], 1)
	}
	if string(old) != want {
		t.Error("the tables of the schema 1.2.3 differ from those of 1.2.4 by more than the tax regime RF21")
	}
}
