package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// schemaTemplate is a small schema in the style of the Factur-X ones; %s
// receives extra particles of the root sequence and %s extra declarations.
const schemaTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
    targetNamespace="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
    elementFormDefault="qualified">
  <xs:element name="CrossIndustryInvoice" type="rsm:InvoiceType"/>
  <xs:complexType name="InvoiceType">
    <xs:sequence>
      <xs:element name="ID" type="rsm:IDType"/>
      <xs:element name="Amount" type="rsm:AmountType" minOccurs="0" maxOccurs="unbounded"/>
      %s
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="IDType">
    <xs:simpleContent>
      <xs:extension base="rsm:CodeContentType">
        <xs:attribute name="schemeID" type="xs:string" use="required"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>
  <xs:complexType name="AmountType">
    <xs:simpleContent>
      <xs:extension base="xs:decimal"/>
    </xs:simpleContent>
  </xs:complexType>
  <xs:simpleType name="CodeContentType">
    <xs:restriction base="xs:string">
      <xs:pattern value="[A-Z]{2}"/>
      <xs:maxLength value="2"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:complexType name="NoteType">
    <xs:choice maxOccurs="2">
      <xs:element name="Text" type="rsm:AmountType"/>
      <xs:element name="Flag" type="rsm:AmountType"/>
    </xs:choice>
  </xs:complexType>
  %s
</xs:schema>
`

// writeSchemas writes the five main schemas into a new directory; the
// EXTENDED one gets the extra particles and declarations.
func writeSchemas(t *testing.T, extraParticles, extraDecls string) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{"MINIMUM", "BASICWL", "BASIC", "EN16931", "EXTENDED"} {
		parts, decls := "", ""
		if p == "EXTENDED" {
			parts, decls = extraParticles, extraDecls
		}
		name := filepath.Join(dir, "Test_2.3_"+p+".xsd")
		if err := os.WriteFile(name, []byte(fmt.Sprintf(schemaTemplate, parts, decls)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGenerate(t *testing.T) {
	dir := writeSchemas(t, `<xs:element name="Note" type="rsm:NoteType" minOccurs="0"/>`, "")
	src, err := generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		`const schemaVersion = "2.3"`,
		`"[A-Z]{2}"`, // pattern facet
		`"schemeID"`, // attribute
		`"Note"`,     // EXTENDED only particle
		`"Flag"`,     // choice alternative
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	// IDType is shared by the five profiles, InvoiceType is not.
	if n := bytes.Count(src, []byte(`name: "rsm:IDType"`)); n != 1 {
		t.Errorf("IDType emitted %d times\n%s", n, src)
	}
	if n := bytes.Count(src, []byte(`name: "rsm:InvoiceType"`)); n != 2 {
		t.Errorf("InvoiceType emitted %d times\n%s", n, src)
	}
}

func TestUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, parts, decls, err string
	}{
		{"all", `<xs:element name="X" type="rsm:AllType"/>`,
			`<xs:complexType name="AllType"><xs:all><xs:element name="A" type="rsm:AmountType"/></xs:all></xs:complexType>`, "all"},
		{"nested group", `<xs:element name="X" type="rsm:NestedType"/>`,
			`<xs:complexType name="NestedType"><xs:sequence><xs:choice><xs:element name="A" type="rsm:AmountType"/></xs:choice></xs:sequence></xs:complexType>`, "nested"},
		{"duplicate name", `<xs:element name="ID" type="rsm:AmountType" minOccurs="0"/>`, "", "twice"},
		{"recursion", `<xs:element name="X" type="rsm:LoopType"/>`,
			`<xs:complexType name="LoopType"><xs:sequence><xs:element name="Y" type="rsm:LoopType" minOccurs="0"/></xs:sequence></xs:complexType>`, "recursive"},
		{"unknown type", `<xs:element name="X" type="rsm:MissingType"/>`, "", "MissingType"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generate(writeSchemas(t, tc.parts, tc.decls))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.err)
			}
		})
	}
}

// TestCorrigendumVersion checks that the tables take the highest version of
// the main schemas, as when a corrigendum renames only the changed profiles.
func TestCorrigendumVersion(t *testing.T) {
	dir := writeSchemas(t, "", "")
	for _, p := range []string{"BASIC", "EXTENDED"} {
		if err := os.Rename(filepath.Join(dir, "Test_2.3_"+p+".xsd"), filepath.Join(dir, "Test_2.3.10_"+p+".xsd")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(dir, "Test_2.3_EN16931.xsd"), filepath.Join(dir, "Test_2.3.9_EN16931.xsd")); err != nil {
		t.Fatal(err)
	}
	src, err := generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte(`const schemaVersion = "2.3.10"`)) {
		t.Errorf("generated source lacks version 2.3.10\n%s", src[:300])
	}
	for _, c := range []struct {
		v, cur string
		want   bool
	}{
		{"1.09", "", true}, {"1.09.2", "1.09", true}, {"1.09", "1.09.2", false},
		{"1.10", "1.09.2", true}, {"1.09", "1.09", false}, {"2.3.10", "2.3.9", true},
	} {
		if got := newerVersion(c.v, c.cur); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.v, c.cur, got)
		}
	}
}

func TestMissingProfile(t *testing.T) {
	dir := writeSchemas(t, "", "")
	if err := os.Remove(filepath.Join(dir, "Test_2.3_BASICWL.xsd")); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(dir); err == nil || !strings.Contains(err.Error(), "BASIC WL") {
		t.Errorf("error = %v", err)
	}
}

// TestUpToDate checks that tables_gen.go matches the schemas in the
// directory named by FX_XSD_DIR (the official Factur-X package).
func TestUpToDate(t *testing.T) {
	dir := os.Getenv("FX_XSD_DIR")
	if dir == "" {
		t.Skip("FX_XSD_DIR not set")
	}
	src, err := generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, cur) {
		t.Error("tables_gen.go is out of date: run go generate in internal/schema")
	}
}

// ublFiles is a small schema set laid out as the UBL 2.1 package, with the
// constructions of the UBL schemas: references to global elements, imports
// between namespaces, a recursive type, simple content extensions and
// restrictions, xs:time and the lax wildcard of the extensions.
var ublFiles = map[string]string{
	"xsdrt/maindoc/UBL-Invoice-2.1.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
    xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" schemaLocation="../common/cac.xsd"/>
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" schemaLocation="../common/cbc.xsd"/>
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2" schemaLocation="../common/ext.xsd"/>
  <xsd:element name="Invoice" type="InvoiceType"/>
  <xsd:complexType name="InvoiceType">
    <xsd:sequence>
      <xsd:element ref="ext:UBLExtensions" minOccurs="0" maxOccurs="1"/>
      <xsd:element ref="cbc:ID" minOccurs="1" maxOccurs="1"/>
      <xsd:element ref="cbc:IssueTime" minOccurs="0" maxOccurs="1"/>
      <xsd:element ref="cac:InvoiceLine" minOccurs="1" maxOccurs="unbounded"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`,
	"xsdrt/maindoc/UBL-CreditNote-2.1.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
    xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" schemaLocation="../common/cac.xsd"/>
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" schemaLocation="../common/cbc.xsd"/>
  <xsd:element name="CreditNote" type="CreditNoteType"/>
  <xsd:complexType name="CreditNoteType">
    <xsd:sequence>
      <xsd:element ref="cbc:ID" minOccurs="1" maxOccurs="1"/>
      <xsd:element ref="cac:InvoiceLine" minOccurs="1" maxOccurs="unbounded"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`,
	"xsdrt/common/cac.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" schemaLocation="cbc.xsd"/>
  <xsd:element name="InvoiceLine" type="InvoiceLineType"/>
  <xsd:element name="SubInvoiceLine" type="InvoiceLineType"/>
  <xsd:complexType name="InvoiceLineType">
    <xsd:sequence>
      <xsd:element ref="cbc:ID" minOccurs="1" maxOccurs="1"/>
      <xsd:element ref="cbc:Amount" minOccurs="1" maxOccurs="1"/>
      <xsd:element ref="SubInvoiceLine" minOccurs="0" maxOccurs="unbounded"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`,
	"xsdrt/common/cbc.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    xmlns:udt="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2" schemaLocation="udt.xsd"/>
  <xsd:element name="Amount" type="AmountType"/>
  <xsd:element name="ID" type="IDType"/>
  <xsd:element name="IssueTime" type="IssueTimeType"/>
  <xsd:complexType name="AmountType"><xsd:simpleContent><xsd:extension base="udt:AmountType"/></xsd:simpleContent></xsd:complexType>
  <xsd:complexType name="IDType"><xsd:simpleContent><xsd:extension base="udt:IdentifierType"/></xsd:simpleContent></xsd:complexType>
  <xsd:complexType name="IssueTimeType"><xsd:simpleContent><xsd:extension base="udt:TimeType"/></xsd:simpleContent></xsd:complexType>
</xsd:schema>`,
	"xsdrt/common/udt.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns:ccts-cct="urn:un:unece:uncefact:data:specification:CoreComponentTypeSchemaModule:2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:un:unece:uncefact:data:specification:CoreComponentTypeSchemaModule:2" schemaLocation="cct.xsd"/>
  <xsd:complexType name="AmountType">
    <xsd:simpleContent>
      <xsd:restriction base="ccts-cct:AmountType">
        <xsd:attribute name="currencyID" type="xsd:normalizedString" use="required"/>
      </xsd:restriction>
    </xsd:simpleContent>
  </xsd:complexType>
  <xsd:complexType name="IdentifierType"><xsd:simpleContent><xsd:extension base="ccts-cct:IdentifierType"/></xsd:simpleContent></xsd:complexType>
  <xsd:complexType name="TimeType"><xsd:simpleContent><xsd:extension base="xsd:time"/></xsd:simpleContent></xsd:complexType>
</xsd:schema>`,
	"xsdrt/common/cct.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:un:unece:uncefact:data:specification:CoreComponentTypeSchemaModule:2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:complexType name="AmountType">
    <xsd:simpleContent>
      <xsd:extension base="xsd:decimal">
        <xsd:attribute name="currencyID" type="xsd:normalizedString" use="optional"/>
        <xsd:attribute name="currencyCodeListVersionID" type="xsd:normalizedString" use="optional"/>
      </xsd:extension>
    </xsd:simpleContent>
  </xsd:complexType>
  <xsd:complexType name="IdentifierType">
    <xsd:simpleContent>
      <xsd:extension base="xsd:normalizedString">
        <xsd:attribute name="schemeID" type="xsd:normalizedString" use="optional"/>
        <xsd:attribute name="languageID" type="xsd:language" use="optional"/>
      </xsd:extension>
    </xsd:simpleContent>
  </xsd:complexType>
</xsd:schema>`,
	"xsdrt/common/ext.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:include schemaLocation="extcontent.xsd"/>
  <xsd:element name="UBLExtensions" type="UBLExtensionsType"/>
  <xsd:complexType name="UBLExtensionsType">
    <xsd:sequence><xsd:element ref="ExtensionContent" minOccurs="1" maxOccurs="unbounded"/></xsd:sequence>
  </xsd:complexType>
  <xsd:element name="ExtensionContent" type="ExtensionContentType"/>
</xsd:schema>`,
	"xsdrt/common/extcontent.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
    targetNamespace="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:example:signature" schemaLocation="sig.xsd"/>
  <xsd:complexType name="ExtensionContentType">
    <xsd:sequence>
      <xsd:any namespace="##other" processContents="lax" minOccurs="1" maxOccurs="1"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`,
	// Imported for lax validation only, never referenced: never read.
	"xsdrt/common/sig.xsd": `not a schema`,
	// The schema of an extension, in the style of the Croatian one: it
	// refers to cbc elements and extends a udt type with nothing.
	"xsdrt/common/HRExtensionAggregateComponents-1.xsd": `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns="urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    xmlns:udt="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2"
    targetNamespace="urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"
    elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" schemaLocation="cbc.xsd"/>
  <xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2" schemaLocation="udt.xsd"/>
  <xsd:element name="HRFISK20Data" type="HRFISK20DataType"/>
  <xsd:complexType name="HRFISK20DataType">
    <xsd:annotation><xsd:documentation>annotations are allowed</xsd:documentation></xsd:annotation>
    <xsd:sequence>
      <xsd:element ref="cbc:Amount" minOccurs="1" maxOccurs="1"/>
      <xsd:element name="OutOfScopeOfVATAmount" type="OutOfScopeOfVATAmountType" minOccurs="0" maxOccurs="1"/>
    </xsd:sequence>
  </xsd:complexType>
  <xsd:complexType name="OutOfScopeOfVATAmountType"><xsd:simpleContent><xsd:extension base="udt:AmountType"/></xsd:simpleContent></xsd:complexType>
</xsd:schema>`,
}

// ublFS returns ublFiles with the replacements edits: file, old, new.
func ublFS(t *testing.T, edits ...string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range ublFiles {
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
	}
	for i := 0; i+2 < len(edits); i += 3 {
		f := fsys[edits[i]]
		if f == nil || !bytes.Contains(f.Data, []byte(edits[i+1])) {
			t.Fatalf("edit of %s: %q not found", edits[i], edits[i+1])
		}
		f.Data = bytes.Replace(f.Data, []byte(edits[i+1]), []byte(edits[i+2]), 1)
	}
	return fsys
}

func TestGenerateUBL(t *testing.T) {
	src, err := generateUBL(ublFS(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "ubl_tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		`const ublSchemaVersion = "2.1"`,
		`var ublRoots = [...]particle{`,
		`{"Invoice", `, `{"CreditNote", `,
		`{name: "ext:ExtensionContentType", content: contentAny, min: 1, max: 1, other: 4}`,
		`{name: "udt:TimeType", content: contentSimple, simple: kTime, simpleName: "xs:time"}`,
		`{"currencyID", kString, 0, true}`,                 // required by the restriction
		`{"currencyCodeListVersionID", kString, 0, false}`, // inherited
		`{"languageID", kString, 1, false}`,                // xs:language pattern
		`xsdrt/common/extcontent.xsd`,
		// The extension root, apart from the document roots.
		`var ublExtensions = [...]particle{`,
		`{"HRFISK20Data", 1, `,
		`{name: "hrextac:HRFISK20DataType", content: contentElements, first: `,
		`{"OutOfScopeOfVATAmount", 1, `,
		`xsdrt/common/HRExtensionAggregateComponents-1.xsd`,
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	for _, bad := range []string{
		"sig.xsd",     // lazy import, never read
		`name: "cbc:`, // the cbc types are the udt ones
		`name: "hrextac:OutOfScopeOfVATAmountType"`, // the udt type it extends
		`ccts-cct:AmountType`,                       // only the base of a restriction
		"profiles:",
	} {
		if bytes.Contains(src, []byte(bad)) {
			t.Errorf("generated source contains %s\n%s", bad, src)
		}
	}
	// cac:SubInvoiceLine has the type of cac:InvoiceLine (recursion).
	re := regexp.MustCompile(`\{"(InvoiceLine|SubInvoiceLine)", \d+, (\d+), `)
	types := map[string]string{}
	for _, m := range re.FindAllSubmatch(src, -1) {
		types[string(m[1])] = string(m[2])
	}
	if types["InvoiceLine"] == "" || types["InvoiceLine"] != types["SubInvoiceLine"] {
		t.Errorf("InvoiceLine and SubInvoiceLine types: %v\n%s", types, src)
	}
	// Reproducible output.
	again, err := generateUBL(ublFS(t))
	if err != nil || !bytes.Equal(src, again) {
		t.Errorf("second generation differs (err %v)", err)
	}
}

func TestUnsupportedUBL(t *testing.T) {
	const (
		cbc  = "xsdrt/common/cbc.xsd"
		udt  = "xsdrt/common/udt.xsd"
		cct  = "xsdrt/common/cct.xsd"
		extc = "xsdrt/common/extcontent.xsd"
		cac  = "xsdrt/common/cac.xsd"
		any  = `<xsd:any namespace="##other" processContents="lax" minOccurs="1" maxOccurs="1"/>`
		req  = `<xsd:attribute name="currencyID" type="xsd:normalizedString" use="required"/>`
	)
	for _, tc := range []struct {
		name  string
		edits []string
		err   string
	}{
		{"strict wildcard", []string{extc, `processContents="lax"`, `processContents="strict"`}, "processContents"},
		{"default wildcard", []string{extc, ` processContents="lax"`, ``}, "processContents"},
		{"wildcard namespace", []string{extc, `##other`, `##any`}, "##any"},
		{"wildcard and element", []string{extc, any, any + `<xsd:element name="X" type="xsd:string"/>`}, "alone"},
		{"wildcard in a choice", []string{extc, "<xsd:sequence>", "<xsd:choice>", extc, "</xsd:sequence>", "</xsd:choice>"}, "alone"},
		{"new attribute in a restriction", []string{udt, req, req + `<xsd:attribute name="foo" type="xsd:string"/>`}, `"foo"`},
		{"facet in a restriction", []string{udt, req, req + `<xsd:minInclusive value="0"/>`}, "minInclusive"},
		{"restriction of a simple type", []string{udt, `base="ccts-cct:AmountType"`, `base="xsd:decimal"`}, "not a complex type"},
		{"attribute declared twice", []string{cct, `<xsd:attribute name="currencyCodeListVersionID"`, `<xsd:attribute name="currencyID"`}, "twice"},
		{"fixed value", []string{cac, `<xsd:element ref="cbc:Amount" minOccurs="1"`, `<xsd:element ref="cbc:Amount" fixed="1" minOccurs="1"`}, "fixed"},
		{"nillable element", []string{cbc, `<xsd:element name="Amount" type="AmountType"/>`, `<xsd:element name="Amount" type="AmountType" nillable="true"/>`}, "nillable"},
		{"fixed attribute not a string", []string{cct, `<xsd:attribute name="currencyCodeListVersionID" type="xsd:normalizedString" use="optional"/>`,
			`<xsd:attribute name="currencyCodeListVersionID" type="xsd:decimal" use="optional" fixed="2"/>`}, "not a string type"},
		{"namespace not imported", []string{cbc, `<xsd:import namespace="urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2" schemaLocation="udt.xsd"/>`, ``}, "not imported"},
		{"defined twice", []string{cbc, `<xsd:element name="ID" type="IDType"/>`, `<xsd:element name="ID" type="IDType"/><xsd:element name="ID" type="AmountType"/>`}, "already defined"},
		{"namespace used but not imported", []string{cac,
			`<xsd:element ref="cbc:Amount" minOccurs="1" maxOccurs="1"/>`, `<xsd:element ref="cbc:Amount" minOccurs="1" maxOccurs="1"/><xsd:element ref="sig:Signature"/>`,
			cac, `elementFormDefault=`, `xmlns:sig="urn:example:signature" elementFormDefault=`}, "namespace urn:example:signature is used but not imported"},
		{"abstract type", []string{cac, `<xsd:complexType name="InvoiceLineType">`, `<xsd:complexType name="InvoiceLineType" abstract="true">`}, "abstract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generateUBL(ublFS(t, tc.edits...))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.err)
			}
		})
	}
}

// TestUBLLazyImport checks that a schema imported for lax validation is read
// once one of its components is referenced.
func TestUBLLazyImport(t *testing.T) {
	_, err := generateUBL(ublFS(t,
		"xsdrt/common/ext.xsd", `<xsd:include schemaLocation="extcontent.xsd"/>`,
		`<xsd:include schemaLocation="extcontent.xsd"/><xsd:import namespace="urn:example:signature" schemaLocation="sig.xsd"/>`,
		"xsdrt/common/ext.xsd", `<xsd:element ref="ExtensionContent"`, `<xsd:element ref="sig:Signature"/><xsd:element ref="ExtensionContent"`,
		"xsdrt/common/ext.xsd", `elementFormDefault=`, `xmlns:sig="urn:example:signature" elementFormDefault=`))
	if err == nil || !strings.Contains(err.Error(), "sig.xsd") {
		t.Errorf("error = %v, want one about sig.xsd", err)
	}
}

// TestUBLRoots checks that the extension roots are not document roots.
func TestUBLRoots(t *testing.T) {
	src, err := generateUBL(ublFS(t))
	if err != nil {
		t.Fatal(err)
	}
	_, roots, ok := bytes.Cut(src, []byte("var ublRoots = "))
	roots, _, _ = bytes.Cut(roots, []byte("}\n"))
	if !ok || bytes.Contains(roots, []byte("HRFISK20Data")) || bytes.Count(roots, []byte("\n")) != 3 {
		t.Errorf("ublRoots:\n%s", roots)
	}
}

// TestUBLMissingExtension checks that the schema of an extension is
// required: the Croatian one is in the package of the tax administration.
func TestUBLMissingExtension(t *testing.T) {
	fsys := ublFS(t)
	delete(fsys, ublExtensionDocs[0].file)
	if _, err := generateUBL(fsys); err == nil || !strings.Contains(err.Error(), "HRExtensionAggregateComponents-1.xsd") {
		t.Errorf("error = %v", err)
	}
}

// TestUBLHRArchive checks the Croatian package: its SHA-256, then that of
// the schema of the extension inside.
func TestUBLHRArchive(t *testing.T) {
	ubl := os.Getenv("FX_UBL_ZIP")
	if ubl == "" {
		t.Skip("FX_UBL_ZIP not set")
	}
	path := filepath.Join(t.TempDir(), "hr.zip")
	if err := os.WriteFile(path, []byte("not the Croatian package"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generateUBLArchive(ubl, path); err == nil || !strings.Contains(err.Error(), hrArchive) || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v", err)
	}
}

func TestUBLArchiveChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), ublArchive)
	if err := os.WriteFile(path, []byte("not the OASIS package"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generateUBLArchive(path, path); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v", err)
	}
}

// TestUpToDateUBL checks that ubl_tables_gen.go matches the OASIS UBL 2.1
// package named by FX_UBL_ZIP (UBL-2.1.zip) and the Croatian package named
// by FX_HR_XSD ("UBL2.1 eRačun.zip").
func TestUpToDateUBL(t *testing.T) {
	path, hr := os.Getenv("FX_UBL_ZIP"), os.Getenv("FX_HR_XSD")
	if path == "" || hr == "" {
		t.Skip("FX_UBL_ZIP or FX_HR_XSD not set")
	}
	src, err := generateUBLArchive(path, hr)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "ubl_tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, cur) {
		t.Error("ubl_tables_gen.go is out of date: run go generate -run -ubl in internal/schema")
	}
}
