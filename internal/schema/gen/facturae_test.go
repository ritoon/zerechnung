package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// facturaeTemplate is a small schema in the style of the Facturae ones: no
// elementFormDefault (the local elements are not qualified), a reference to
// ds:Signature imported from the W3C site, a choice nested in a sequence, an
// optional nested choice, a complex content extension that adds nothing, a
// strict wildcard, xs:double with a pattern and a default value. %[1]s is
// the target namespace, %[2]s the version, %[3]s extra particles of the
// party type.
const facturaeTemplate = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:ds="http://www.w3.org/2000/09/xmldsig#" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns="%[1]s" targetNamespace="%[1]s" version="%[2]s">
  <xs:import namespace="http://www.w3.org/2000/09/xmldsig#" schemaLocation="http://www.w3.org/TR/xmldsig-core/xmldsig-core-schema.xsd"/>
  <xs:element name="Facturae">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="SchemaVersion" type="SchemaVersionType" default="%[2]s"/>
        <xs:element name="Party" type="PartyType"/>
        <xs:element name="Tax">
          <xs:complexType>
            <xs:complexContent>
              <xs:extension base="TaxType"/>
            </xs:complexContent>
          </xs:complexType>
        </xs:element>
        <xs:element name="Extensions" type="ExtensionsType" minOccurs="0"/>
        <xs:element ref="ds:Signature" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="PartyType">
    <xs:sequence>
      <xs:element name="Name" type="xs:string"/>
      <xs:choice>
        <xs:element name="AddressInSpain" type="xs:string"/>
        <xs:element name="OverseasAddress" type="xs:string"/>
      </xs:choice>
      <xs:choice minOccurs="0">
        <xs:element name="Phone" type="xs:string"/>
        <xs:element name="Mail" type="xs:string" minOccurs="0"/>
      </xs:choice>
      %[3]s
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="TaxType">
    <xs:sequence>
      <xs:element name="Rate" type="DoubleTwoDecimalType"/>
      <xs:element name="Quantity" type="xs:double"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="ExtensionsType">
    <xs:sequence>
      <xs:any namespace="##other" processContents="strict" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="SchemaVersionType">
    <xs:restriction base="xs:string">
      <xs:enumeration value="%[2]s"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="DoubleTwoDecimalType">
    <xs:restriction base="xs:double">
      <xs:pattern value="-?[0-9]+\.[0-9]{2}"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>
`

// facturaeFS returns the three schemas of facturaeFiles written from
// facturaeTemplate, the last one with the extra particles.
func facturaeFS(extra string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for i, f := range facturaeFiles {
		parts := ""
		if i == len(facturaeFiles)-1 {
			parts = extra
		}
		fsys[f.name] = &fstest.MapFile{Data: fmt.Appendf(nil, facturaeTemplate, f.ns, f.version, parts)}
	}
	return fsys
}

func TestGenerateFacturae(t *testing.T) {
	src, err := compileFacturae(facturaeFS(""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "facturae_tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		`const facturaeSchemaVersion = "3.2.2"`,
		`var facturaeRoots = [...]particle{`,
		`{name: "ds:Signature (not checked)", content: contentSkip}`,
		`{"Signature", 4, 0, 0, 1}`,                         // ds:Signature, optional
		`{"AddressInSpain", 0, `, `{"OverseasAddress", 0, `, // unqualified
		`altFirst: 0, altN: 5}`,               // two nested choices in PartyType
		`{1, 1, 1, 0x01}, // fe322:PartyType`, // required choice
		`{1, 2, 1, 0x01}, // fe322:PartyType`,
		`{2, 3, 1, 0x01}, // fe322:PartyType`, // optional choice
		`{2, 4, 1, 0x00}, // fe322:PartyType`,
		`{2, 5, 0, 0x00}, // fe322:PartyType`,         // its empty alternative
		`def: "3.2.2"`,                                // default value of SchemaVersion
		`content: contentAny, min: 0, max: unbounded`, // strict wildcard, unchecked
		`simple: kDouble, facets: `,                   // xs:double with a pattern
		`{name: "xs:double", content: contentSimple, simple: kDouble, simpleName: "xs:double"}`,
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	// The anonymous type of Tax is TaxType.
	if bytes.Contains(src, []byte(`type of {}Tax`)) {
		t.Errorf("an anonymous type is emitted\n%s", src)
	}
	// The alternatives are optional particles of the sequence.
	for _, alt := range []string{"AddressInSpain", "OverseasAddress", "Phone", "Mail"} {
		if !bytes.Contains(src, fmt.Appendf(nil, `{%q, 0, `, alt)) || !strings.Contains(particleLine(src, alt), ", 0, 1}") {
			t.Errorf("%s is not an optional particle: %s", alt, particleLine(src, alt))
		}
	}
	again, err := compileFacturae(facturaeFS(""))
	if err != nil || !bytes.Equal(src, again) {
		t.Errorf("second generation differs (err %v)", err)
	}
}

// particleLine returns the first line of src that declares the particle
// name.
func particleLine(src []byte, name string) string {
	for line := range strings.SplitSeq(string(src), "\n") {
		if strings.Contains(line, fmt.Sprintf(`{%q, `, name)) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func TestUnsupportedFacturae(t *testing.T) {
	for _, tc := range []struct{ name, extra, err string }{
		{"repeated choice", `<xs:choice maxOccurs="2"><xs:element name="A" type="xs:string"/></xs:choice>`, "repeated nested"},
		{"one alternative", `<xs:choice><xs:element name="A" type="xs:string"/></xs:choice>`, "two alternatives"},
		{"wildcard in a choice", `<xs:choice><xs:any namespace="##other"/><xs:element name="A" type="xs:string"/></xs:choice>`, "xs:any in a nested xs:choice"},
		{"name used twice", `<xs:choice><xs:element name="Name" type="xs:string"/></xs:choice>`, "twice"},
		{"alternative repeated", `<xs:choice><xs:element name="A" type="xs:string" minOccurs="2" maxOccurs="2"/></xs:choice>`, "minOccurs 2"},
		{"nested sequence", `<xs:sequence><xs:element name="A" type="xs:string"/></xs:sequence>`, "nested xs:sequence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileFacturae(facturaeFS(tc.extra))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.err)
			}
		})
	}
}

func TestFacturaeChecksum(t *testing.T) {
	dir := t.TempDir()
	for name, f := range facturaeFS("") {
		if err := os.WriteFile(filepath.Join(dir, name), f.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := generateFacturae(dir); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v", err)
	}
}

// TestUpToDateFacturae checks that facturae_tables_gen.go matches the
// Facturae schemas of the directory named by FX_FACTURAE_XSD.
func TestUpToDateFacturae(t *testing.T) {
	dir := os.Getenv("FX_FACTURAE_XSD")
	if dir == "" {
		t.Skip("FX_FACTURAE_XSD not set")
	}
	src, err := generateFacturae(dir)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "facturae_tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, cur) {
		t.Error("facturae_tables_gen.go is out of date: run go generate -run -facturae in internal/schema")
	}
}
