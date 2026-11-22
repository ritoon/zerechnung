package zerechnung

import (
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/validate"
)

// TestCheckHybrid checks that the XML of a PDF is validated as a Factur-X
// invoice: an EN 16931 invoice gets the Factur-X schematron of its profile,
// which admits a currencyID on every amount (CII-DT-031 of the CEN rules)
// and has its own code lists.
func TestCheckHybrid(t *testing.T) {
	x := strings.Replace(string(readTestdata(t, "EN16931_Einfach.cii.xml")),
		"<ram:LineTotalAmount>473.00</ram:LineTotalAmount>", `<ram:LineTotalAmount currencyID="EUR">473.00</ram:LineTotalAmount>`, 1)
	if r := validate.XML([]byte(x), nil); !r.Has("CII-DT-031") {
		t.Errorf("XML alone: no CII-DT-031\n%s", r)
	}
	out, err := Embed(base(pdftest.Options{}), []byte(x), &EmbedOptions{ModDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := Check(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Valid() || r.Has("CII-DT-031") {
		t.Errorf("PDF: %s", r)
	}

	// A type code out of the lists: BR-CL-01 of the CEN rules for the XML
	// alone, FX-SCH-A-000555 of the Factur-X schematron in a PDF.
	bad := strings.Replace(x, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>", 1)
	if r := validate.XML([]byte(bad), nil); !r.Has("BR-CL-01") || r.Has("FX-SCH-A-000555") {
		t.Errorf("XML alone: %s", r)
	}
	if r := validate.XML([]byte(bad), &validate.Options{Hybrid: true}); r.Has("BR-CL-01") || !r.Has("FX-SCH-A-000555") {
		t.Errorf("XML with Hybrid: %s", r)
	}
	out, err = Embed(base(pdftest.Options{}), []byte(bad), &EmbedOptions{ModDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _ := Check(out, &validate.Options{}); r.Has("BR-CL-01") || !r.Has("FX-SCH-A-000555") {
		t.Errorf("PDF: %s", r)
	}
}

// TestCheckHybridSchema checks that the XML of a PDF is checked against the
// Factur-X schema of the EN 16931 profile, and the XML alone against the CII
// D16B schema of the CEN syntax binding, as the KoSIT validator does: a test
// indicator, outside the EN 16931 data set, only gets the warning CII-SR-002
// in the XML alone, and is not allowed in a Factur-X invoice.
func TestCheckHybridSchema(t *testing.T) {
	x := strings.Replace(string(readTestdata(t, "EN16931_Einfach.cii.xml")),
		"<rsm:ExchangedDocumentContext>", "<rsm:ExchangedDocumentContext><ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>", 1)
	if r := validate.XML([]byte(x), nil); !r.Valid() || r.Has("XSD-UNEXPECTED") || !r.Has("CII-SR-002") {
		t.Errorf("XML alone: %s", r)
	}
	if r := validate.XML([]byte(x), &validate.Options{Hybrid: true}); r.Valid() || !r.Has("XSD-UNEXPECTED") {
		t.Errorf("XML with Hybrid: %s", r)
	}
	out, err := Embed(base(pdftest.Options{}), []byte(x), &EmbedOptions{ModDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _ := Check(out, nil); r.Valid() || !r.Has("XSD-UNEXPECTED") {
		t.Errorf("PDF: %s", r)
	}
}
