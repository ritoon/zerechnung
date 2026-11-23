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
)

const ciiHeader = `<?xml version="1.0" encoding="UTF-8"?>
<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
    xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
    xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100"
    xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
    elementFormDefault="qualified" `

// ciiTestFiles is a small CII schema set laid out as the UN/CEFACT ones,
// with their constructions: imports between the rsm, ram, qdt and udt
// namespaces, a recursive type (ram:GroupedWorkItemType), a choice with an
// xs:dateTime element and attributes with fixed values.
var ciiTestFiles = []struct{ name, data string }{
	{"CrossIndustryInvoice_100pTEST.xsd", ciiHeader + `targetNamespace="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100">
  <xsd:import namespace="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" schemaLocation="ram.xsd"/>
  <xsd:element name="CrossIndustryInvoice" type="rsm:CrossIndustryInvoiceType"/>
  <xsd:complexType name="CrossIndustryInvoiceType">
    <xsd:sequence>
      <xsd:element name="ExchangedDocument" type="ram:ExchangedDocumentType"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`},
	{"ram.xsd", ciiHeader + `targetNamespace="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100">
  <xsd:import namespace="urn:un:unece:uncefact:data:standard:QualifiedDataType:100" schemaLocation="qdt.xsd"/>
  <xsd:import namespace="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100" schemaLocation="udt.xsd"/>
  <xsd:complexType name="ExchangedDocumentType">
    <xsd:sequence>
      <xsd:element name="ID" type="udt:IDType"/>
      <xsd:element name="TypeCode" type="qdt:DocumentCodeType" minOccurs="0"/>
      <xsd:element name="IssueDateTime" type="udt:DateTimeType"/>
      <xsd:element name="IncludedWorkItem" type="ram:GroupedWorkItemType" minOccurs="0" maxOccurs="unbounded"/>
    </xsd:sequence>
  </xsd:complexType>
  <xsd:complexType name="GroupedWorkItemType">
    <xsd:sequence>
      <xsd:element name="ID" type="udt:IDType"/>
      <xsd:element name="ItemGroupedWorkItem" type="ram:GroupedWorkItemType" minOccurs="0" maxOccurs="unbounded"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`},
	{"qdt.xsd", ciiHeader + `targetNamespace="urn:un:unece:uncefact:data:standard:QualifiedDataType:100">
  <xsd:simpleType name="DocumentCodeContentType">
    <xsd:restriction base="xsd:token">
      <xsd:enumeration value="380"/>
      <xsd:enumeration value="381"/>
    </xsd:restriction>
  </xsd:simpleType>
  <xsd:simpleType name="DocumentCodeListAgencyIDContentType">
    <xsd:restriction base="xsd:token"/>
  </xsd:simpleType>
  <xsd:complexType name="DocumentCodeType">
    <xsd:simpleContent>
      <xsd:extension base="qdt:DocumentCodeContentType">
        <xsd:attribute name="listID" type="xsd:token" fixed="1001"/>
        <xsd:attribute name="listAgencyID" type="qdt:DocumentCodeListAgencyIDContentType" fixed="6"/>
        <xsd:attribute name="name" type="xsd:string"/>
      </xsd:extension>
    </xsd:simpleContent>
  </xsd:complexType>
</xsd:schema>`},
	{"udt.xsd", ciiHeader + `targetNamespace="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
  <xsd:complexType name="IDType">
    <xsd:simpleContent>
      <xsd:extension base="xsd:token">
        <xsd:attribute name="schemeID" type="xsd:token"/>
      </xsd:extension>
    </xsd:simpleContent>
  </xsd:complexType>
  <xsd:complexType name="DateTimeType">
    <xsd:choice>
      <xsd:element name="DateTimeString">
        <xsd:complexType>
          <xsd:simpleContent>
            <xsd:extension base="xsd:string">
              <xsd:attribute name="format" type="xsd:string"/>
            </xsd:extension>
          </xsd:simpleContent>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="DateTime" type="xsd:dateTime"/>
    </xsd:choice>
  </xsd:complexType>
</xsd:schema>`},
}

// ciiTestSet writes ciiTestFiles to a directory, with the replacements
// edits (file, old, new), and returns it with the set pinning their SHA-256.
func ciiTestSet(t *testing.T, edits ...string) (ciiSet, string) {
	t.Helper()
	dir := t.TempDir()
	set := ciiSet{prefix: "test", ns: nsRSM, root: ciiRoot, title: "UN/CEFACT CII TEST", origin: "test schemas", dir: "https://example.com/cii/"}
	for _, f := range ciiTestFiles {
		data := f.data
		for i := 0; i+2 < len(edits); i += 3 {
			if edits[i] == f.name {
				if !strings.Contains(data, edits[i+1]) {
					t.Fatalf("edit of %s: %q not found", f.name, edits[i+1])
				}
				data = strings.Replace(data, edits[i+1], edits[i+2], 1)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256([]byte(data))
		set.files = append(set.files, pinnedFile{f.name, hex.EncodeToString(h[:])})
	}
	return set, dir
}

func generateTestCII(t *testing.T, edits ...string) ([]byte, error) {
	t.Helper()
	set, dir := ciiTestSet(t, edits...)
	fsys, err := ciiFiles(set, dir)
	if err != nil {
		t.Fatal(err)
	}
	return compileCII("TEST", set, fsys)
}

func TestGenerateCII(t *testing.T) {
	src, err := generateTestCII(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "cii_test_tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		"from the UN/CEFACT CII TEST XML schemas; DO NOT EDIT.",
		"// Source: test schemas,\n// https://example.com/cii/ (SHA-256 of each file):",
		"CrossIndustryInvoice_100pTEST.xsd\n",
		`const testSchemaVersion = "TEST"`,
		`var testRoots = [...]particle{`,
		`{"CrossIndustryInvoice", 1, `,
		`{name: "udt:DateTimeType", content: contentElements, choice: true, min: 1, max: 1, `,
		`{name: "xs:dateTime", content: contentSimple, simple: kDateTime, simpleName: "xs:dateTime"}`,
		// The fixed values are one-value enumerations.
		`{enum: []string{"1001"}, minLen: -1, maxLen: -1, collapse: true}`,
		`{enum: []string{"6"}, minLen: -1, maxLen: -1, collapse: true}`,
		`{enum: []string{"380", "381"}, minLen: -1, maxLen: -1, collapse: true}`,
		`{"name", kString, 0, false}`,
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	if bytes.Contains(src, []byte("profiles:")) {
		t.Errorf("generated source has profiles\n%s", src)
	}
	// ram:IncludedWorkItem and ram:ItemGroupedWorkItem have the same type,
	// which contains itself.
	re := regexp.MustCompile(`\{"(IncludedWorkItem|ItemGroupedWorkItem)", \d+, (\d+), `)
	types := map[string]string{}
	for _, m := range re.FindAllSubmatch(src, -1) {
		types[string(m[1])] = string(m[2])
	}
	if types["IncludedWorkItem"] == "" || types["IncludedWorkItem"] != types["ItemGroupedWorkItem"] {
		t.Errorf("work item types: %v\n%s", types, src)
	}
	again, err := generateTestCII(t)
	if err != nil || !bytes.Equal(src, again) {
		t.Errorf("second generation differs (err %v)", err)
	}
}

func TestFixedAttribute(t *testing.T) {
	const qdt = "qdt.xsd"
	for _, tc := range []struct {
		name  string
		edits []string
		want  string // in the source, or in the error when err
		err   bool
	}{
		{"fixed value in the enumeration", []string{qdt, `<xsd:attribute name="name" type="xsd:string"/>`,
			`<xsd:attribute name="name" type="qdt:DocumentCodeContentType" fixed="381"/>`}, `{enum: []string{"381"}, minLen: -1, maxLen: -1, collapse: true}`, false},
		{"fixed value outside the enumeration", []string{qdt, `<xsd:attribute name="name" type="xsd:string"/>`,
			`<xsd:attribute name="name" type="qdt:DocumentCodeContentType" fixed="999"/>`}, `fixed value "999" is not in the enumeration`, true},
		{"fixed value on a decimal", []string{qdt, `<xsd:attribute name="name" type="xsd:string"/>`,
			`<xsd:attribute name="name" type="xsd:decimal" fixed="1"/>`}, "not a string type", true},
		{"fixed value on a string", []string{qdt, `<xsd:attribute name="name" type="xsd:string"/>`,
			`<xsd:attribute name="name" type="xsd:string" fixed=" a  b "/>`}, `{enum: []string{" a  b "}, minLen: -1, maxLen: -1, collapse: false}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, err := generateTestCII(t, tc.edits...)
			switch {
			case tc.err && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("error = %v, want one mentioning %q", err, tc.want)
			case !tc.err && err != nil:
				t.Fatal(err)
			case !tc.err && !bytes.Contains(src, []byte(tc.want)):
				t.Errorf("generated source lacks %s\n%s", tc.want, src)
			}
		})
	}
}

func TestCIIFiles(t *testing.T) {
	if _, err := generateCII("D99A", ""); err == nil || !strings.Contains(err.Error(), "unknown CII schema version") {
		t.Errorf("unknown version: error = %v", err)
	}
	// A file that is not the pinned one.
	set, dir := ciiTestSet(t)
	if err := os.WriteFile(filepath.Join(dir, "udt.xsd"), []byte("<xsd:schema/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ciiFiles(set, dir); err == nil || !strings.Contains(err.Error(), "udt.xsd: SHA-256") {
		t.Errorf("changed file: error = %v", err)
	}
	// A missing file.
	if _, err := generateCII("D22B", t.TempDir()); err == nil || !strings.Contains(err.Error(), "CrossIndustryInvoice_100pD22B.xsd") {
		t.Errorf("missing file: error = %v", err)
	}
	// An archive that is not the KoSIT one.
	path := filepath.Join(t.TempDir(), "conf.zip")
	if err := os.WriteFile(path, []byte("not the KoSIT archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generateCII("D16B", path); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("archive: error = %v", err)
	}
}

// TestUpToDateCII checks that cii_d16b_tables_gen.go matches the KoSIT
// validator configuration archive named by FX_KOSIT_CONF, and
// cii_d22b_tables_gen_test.go the checkout of github.com/fnfempe/France_RFE
// named by FX_FNFE.
func TestUpToDateCII(t *testing.T) {
	for _, tc := range []struct{ v, env, sub, file string }{
		{"D16B", "FX_KOSIT_CONF", "", "cii_d16b_tables_gen.go"},
		{"D22B", "FX_FNFE", filepath.Join("FNFE_RFE_INVOICE", "CII", "1xsd-CII_D22B_uncoupled"), "cii_d22b_tables_gen_test.go"},
	} {
		t.Run(tc.v, func(t *testing.T) {
			src := os.Getenv(tc.env)
			if src == "" {
				t.Skip(tc.env + " not set")
			}
			got, err := generateCII(tc.v, filepath.Join(src, tc.sub))
			if err != nil {
				t.Fatal(err)
			}
			cur, err := os.ReadFile(filepath.Join("..", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, cur) {
				t.Errorf("%s is out of date: run go generate -run -cii in internal/schema", tc.file)
			}
		})
	}
}

// TestUpToDateCDAR checks that cdar_tables_gen.go matches the CDAR schema
// of the checkout of github.com/fnfempe/France_RFE named by FX_FNFE.
func TestUpToDateCDAR(t *testing.T) {
	src := os.Getenv("FX_FNFE")
	if src == "" {
		t.Skip("FX_FNFE not set")
	}
	got, err := generateCDAR(filepath.Join(src, "FNFE_RFE_INVOICE", "CDAR", "1xsd-CDAR_D22B_uncoupled"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "cdar_tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, cur) {
		t.Error("cdar_tables_gen.go is out of date: run go generate -run -cdar in internal/schema")
	}
}

// TestGenerateCDARErrors checks that the CDAR schema files are checked.
func TestGenerateCDARErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := generateCDAR(dir); err == nil || !strings.Contains(err.Error(), "CrossDomainAcknowledgementAndResponse_100pD22B.xsd") {
		t.Errorf("missing file: error = %v", err)
	}
	for _, f := range cdarSet.files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte("<x/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := generateCDAR(dir); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("altered file: error = %v", err)
	}
}
