package zerechnung

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// isoRule reports whether id is one of the requirements of ISO 19005-2 and
// 19005-3 that validate.PDFABasic skips: PDF-41 to PDF-69, PDF-71, PDF-72.
func isoRule(id string) bool {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "PDF-"))
	return err == nil && ((n >= 41 && n <= 69) || n == 71 || n == 72)
}

// containerRules returns the rules of the container findings of a report,
// sorted.
func containerRules(r *validate.Report) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Source == rules.SourcePDF {
			out = append(out, f.Rule)
		}
	}
	slices.Sort(out)
	return out
}

// TestCheckPDFALevel checks that validate.PDFABasic reports the container
// findings of the full check but those of PDF-41 to PDF-69, PDF-71 and
// PDF-72, and that the full check stays the default.
func TestCheckPDFALevel(t *testing.T) {
	en := ciiXML(invoice.URNEN16931)
	mod := func(fn func(f *fixture)) []byte {
		f := valid(en)
		fn(f)
		return f.build()
	}
	js := "<< /S /JavaScript /JS (app.alert(1)) >>"
	cases := []struct {
		name string
		pdf  []byte
		iso  bool // the full check reports a rule that PDFABasic skips
	}{
		{"valid", mod(func(*fixture) {}), false},
		{"header version 1.8", mod(func(f *fixture) {
			f.post = func(b []byte) []byte { return bytes.Replace(b, []byte("%PDF-1.7"), []byte("%PDF-1.8"), 1) }
		}), true},
		{"undefined operator", mod(func(f *fixture) {
			f.extra = func(b *pdftest.Builder) (string, string) {
				return "", fmt.Sprintf("/Contents %d 0 R /Resources << >>", b.AddStream("<< >>", []byte("0 0 10 10 re foo f")))
			}
		}), true},
		{"document JavaScript and no AFRelationship", mod(func(f *fixture) {
			f.names = " /JavaScript << /Names [(init) " + js + "] >>"
			f.files[0].rel = ""
		}), false},
	}
	if data, err := os.ReadFile("testdata/EN16931_Einfach.pdf"); err == nil {
		cases = append(cases, struct {
			name string
			pdf  []byte
			iso  bool
		}{"EN16931_Einfach.pdf", data, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full, _, err := Check(tc.pdf, nil)
			if err != nil {
				t.Fatal(err)
			}
			if explicit, _, _ := Check(tc.pdf, &validate.Options{PDFA: validate.PDFAFull}); !slices.Equal(containerRules(explicit), containerRules(full)) {
				t.Errorf("PDFAFull %v, default %v", containerRules(explicit), containerRules(full))
			}
			basic, _, err := Check(tc.pdf, &validate.Options{PDFA: validate.PDFABasic})
			if err != nil {
				t.Fatal(err)
			}
			want := slices.DeleteFunc(containerRules(full), isoRule)
			if got := containerRules(basic); !slices.Equal(got, want) {
				t.Errorf("PDFABasic %v, want %v (full check %v)", got, want, containerRules(full))
			}
			if got := slices.ContainsFunc(containerRules(full), isoRule); got != tc.iso {
				t.Errorf("full check %v: ISO 19005 rule reported %v, want %v", containerRules(full), got, tc.iso)
			}
			if len(basic.Findings)-len(containerRules(basic)) != len(full.Findings)-len(containerRules(full)) {
				t.Errorf("the XML findings differ: %d with PDFABasic, %d with the full check",
					len(basic.Findings)-len(containerRules(basic)), len(full.Findings)-len(containerRules(full)))
			}
		})
	}
}
