package validate

import (
	"os"
	"testing"
)

// benchXML measures XML on an invoice of the testdata: the schema, the
// rules of EN 16931 and those of the national rule sets its specification
// identifier selects.
func benchXML(b *testing.B, name string) {
	data, err := os.ReadFile(name)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		XML(data, nil)
	}
}

func BenchmarkXMLCII(b *testing.B)       { benchXML(b, "../testdata/EN16931_Einfach.cii.xml") }
func BenchmarkXMLUBL(b *testing.B)       { benchXML(b, "../testdata/EN16931_Einfach.ubl.xml") }
func BenchmarkXMLXRechnung(b *testing.B) { benchXML(b, "../testdata/XRECHNUNG_Einfach.cii.xml") }
func BenchmarkXMLFacturae(b *testing.B)  { benchXML(b, facturaeInvoice) }
