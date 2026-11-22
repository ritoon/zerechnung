package zerechnung

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// readCheckPDFs returns PDFs whose invoice can be read, and PDFs whose
// invoice cannot: without one, encrypted, with a UBL invoice, with an XML
// that cannot be parsed or with a value that cannot be read, and a file
// that is not a PDF.
func readCheckPDFs(t testing.TB) map[string][]byte {
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	encrypted := valid(ciiXML(invoice.URNEN16931))
	encrypted.encrypted = true
	return map[string][]byte{
		"EN16931_Einfach.pdf": read("testdata/EN16931_Einfach.pdf"),
		"no invoice":          read("testdata/host.pdf"),
		"minimal invoice":     valid(ciiXML(invoice.URNEN16931)).build(),
		"ZUGFeRD 1.0":         valid(read("testdata/ZUGFeRD_1p0_COMFORT_SEPA_Prenotification.xml")).build(),
		"encrypted":           encrypted.build(),
		"UBL invoice":         valid(read("testdata/EN16931_Einfach.ubl.xml")).build(),
		"XML not well-formed": valid([]byte("<rsm:CrossIndustryInvoice")).build(),
		"value that cannot be read": valid(bytes.Replace(read("testdata/EN16931_Einfach.cii.xml"),
			[]byte("<ram:DuePayableAmount>"), []byte("<ram:DuePayableAmount>x"), 1)).build(),
		"not a PDF": []byte("not a pdf"),
	}
}

// TestReadAndCheck checks that ReadAndCheck gives the invoice and the error
// of Read, and the report and the attachment of Check, with and without a
// Checker.
func TestReadAndCheck(t *testing.T) {
	checker := NewChecker(&CheckerOptions{FontCache: 32 << 20})
	for name, data := range readCheckPDFs(t) {
		for _, opts := range []*validate.Options{nil, {Auto: true}, {PDFA: validate.PDFABasic}} {
			wantReport, wantAtt, checkErr := Check(data, opts)
			wantInv, readAtt, readErr := Read(data)
			for _, c := range []*Checker{nil, checker} {
				t.Run(fmt.Sprintf("%s/%+v/cache %v", name, opts, c != nil), func(t *testing.T) {
					readAndCheck := ReadAndCheck
					if c != nil {
						readAndCheck = c.ReadAndCheck
					}
					inv, r, att, err := readAndCheck(data, opts)
					if checkErr != nil {
						if inv != nil || r != nil || att != nil || fmt.Sprint(err) != fmt.Sprint(checkErr) {
							t.Errorf("invoice %v, report %v, attachment %v, error %v; Check fails with %v", inv != nil, r != nil, att != nil, err, checkErr)
						}
						return
					}
					if !reflect.DeepEqual(r, wantReport) {
						t.Errorf("report:\n%v\nCheck reports:\n%v", r.Findings, wantReport.Findings)
					}
					if !reflect.DeepEqual(att, wantAtt) || !reflect.DeepEqual(att, readAtt) {
						t.Errorf("attachment %+v, Check gives %+v, Read %+v", att, wantAtt, readAtt)
					}
					if !reflect.DeepEqual(inv, wantInv) {
						t.Errorf("invoice %+v, Read gives %+v", inv, wantInv)
					}
					if fmt.Sprint(err) != fmt.Sprint(readErr) || reflect.TypeOf(err) != reflect.TypeOf(readErr) {
						t.Errorf("error %v (%T), Read gives %v (%T)", err, err, readErr, readErr)
					}
				})
			}
		}
	}
}

func benchReadAndCheck(b *testing.B, opts *validate.Options) {
	data, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("separately", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := Read(data); err != nil {
				b.Fatal(err)
			}
			if _, _, err := Check(data, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ReadAndCheck", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		for b.Loop() {
			if _, _, _, err := ReadAndCheck(data, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkReadAndCheck measures Read followed by Check, and ReadAndCheck,
// with every PDF/A check; BenchmarkReadAndCheckBasic with
// validate.PDFABasic.
func BenchmarkReadAndCheck(b *testing.B) { benchReadAndCheck(b, nil) }
func BenchmarkReadAndCheckBasic(b *testing.B) {
	benchReadAndCheck(b, &validate.Options{PDFA: validate.PDFABasic})
}
