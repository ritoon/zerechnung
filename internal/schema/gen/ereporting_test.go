package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// ereportingFS returns a small schema set in the style of the e-reporting
// one: a main schema without target namespace, importing schemas whose
// types are in their target namespace and whose local elements are not
// qualified, a simple content extension of a simple type of a third schema
// with a required attribute, xs:anyURI and an xs:boolean attribute. extra
// is added to the sequence of the report document.
func ereportingFS(extra string) fstest.MapFS {
	return fstest.MapFS{
		"ereporting.xsd": {Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:rep="report" xmlns:pay="payment">
  <xs:import namespace="report" schemaLocation="report.xsd"/>
  <xs:import namespace="payment" schemaLocation="payment.xsd"/>
  <xs:element name="Report" type="ReportType"/>
  <xs:complexType name="ReportType">
    <xs:sequence>
      <xs:element name="ReportDocument" type="rep:ReportDocumentType" minOccurs="1" maxOccurs="1"/>
      <xs:element name="PaymentsReport" type="pay:PaymentsReportType" minOccurs="0" maxOccurs="1"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)},
		"report.xsd": {Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="report" xmlns:rep="report" xmlns:param="parametre">
  <xs:import namespace="parametre" schemaLocation="parametre.xsd"/>
  <xs:complexType name="ReportDocumentType">
    <xs:sequence>
      <xs:element name="Id" minOccurs="1" maxOccurs="1">
        <xs:complexType>
          <xs:simpleContent>
            <xs:extension base="param:SenderIdType">
              <xs:attribute name="schemeId" use="required" type="xs:string"/>
            </xs:extension>
          </xs:simpleContent>
        </xs:complexType>
      </xs:element>
      <xs:element name="URIID" type="xs:anyURI" minOccurs="0"/>
      ` + extra + `
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)},
		"parametre.xsd": {Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="parametre" xmlns:param="parametre">
  <xs:simpleType name="SenderIdType">
    <xs:restriction base="xs:string">
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`)},
		"payment.xsd": {Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="payment" xmlns:pay="payment">
  <xs:complexType name="PaymentsReportType">
    <xs:sequence>
      <xs:element name="Charge" minOccurs="0" maxOccurs="unbounded">
        <xs:complexType>
          <xs:sequence>
            <xs:element name="Amount" type="xs:decimal"/>
          </xs:sequence>
          <xs:attribute name="ChargeIndicator" use="required" type="xs:boolean"/>
        </xs:complexType>
      </xs:element>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)},
	}
}

func TestGenerateEReporting(t *testing.T) {
	src, err := compileEReporting(ereportingFS(""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "ereporting_tables_gen.go", src, 0); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	for _, want := range []string{
		`const ereportingSchemaVersion = "1.4.0.04"`,
		`var ereportingNamespaces = [...]string{""}`, // no element is qualified
		`var ereportingRoots = [...]particle{`,
		`{"Report", 0, `,
		`{name: "rep:ReportDocumentType", content: contentElements`,
		`{name: "pay:PaymentsReportType", content: contentElements`,
		`{name: "{}ReportType", content: contentElements`,
		`{"Charge", 0, `, `0, unbounded}`,
		`{"schemeId", kString, 0, true}`,
		`{"ChargeIndicator", kBoolean, 0, true}`,
		`{name: "xs:anyURI", content: contentSimple, simple: kString}`,
		ereportingDir,
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("generated source lacks %s\n%s", want, src)
		}
	}
	again, err := compileEReporting(ereportingFS(""))
	if err != nil || !bytes.Equal(src, again) {
		t.Errorf("second generation differs (err %v)", err)
	}
}

func TestUnsupportedEReporting(t *testing.T) {
	for _, tc := range []struct{ name, extra, err string }{
		{"wildcard in a choice", `<xs:choice><xs:any namespace="##other"/><xs:element name="A" type="xs:string"/></xs:choice>`, "xs:any"},
		{"unknown type", `<xs:element name="A" type="param:Missing"/>`, "Missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileEReporting(ereportingFS(tc.extra))
			if err == nil || !strings.Contains(err.Error(), tc.err) || !strings.HasPrefix(err.Error(), "e-reporting: ") {
				t.Errorf("error = %v, want one mentioning %q", err, tc.err)
			}
		})
	}
}

func TestEReportingChecksum(t *testing.T) {
	dir := t.TempDir()
	for _, f := range ereportingFiles {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte("<xs:schema/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := generateEReporting(dir); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v", err)
	}
	if _, err := generateEReporting(t.TempDir()); err == nil {
		t.Error("no error for a directory without the schemas")
	}
}

// TestUpToDateEReporting checks that ereporting_tables_gen.go matches the
// e-reporting schemas of the checkout of github.com/fnfempe/France_RFE
// named by FX_FNFE.
func TestUpToDateEReporting(t *testing.T) {
	src := os.Getenv("FX_FNFE")
	if src == "" {
		t.Skip("FX_FNFE not set")
	}
	got, err := generateEReporting(filepath.Join(src, "FNFE_RFE_INVOICE", "Flux10", "xsd"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "ereporting_tables_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, cur) {
		t.Error("ereporting_tables_gen.go is out of date: run go generate -run -ereporting in internal/schema")
	}
}
