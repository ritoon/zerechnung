package zerechnung

import (
	"os"
	"testing"

	"github.com/ritoon/zerechnung/internal/race"
	"github.com/ritoon/zerechnung/validate"
)

// TestAllocs checks that Extract, Check, Checker.Check and ReadAndCheck stay
// within their budget of allocations on testdata/EN16931_Einfach.pdf.
// testing.AllocsPerRun counts allocations, which do not vary from one run to
// the next as times do. The budgets leave room for the versions of Go
// tested by the CI; a change that goes beyond one is measured, and the
// budget raised only when the cost is justified.
func TestAllocs(t *testing.T) {
	if race.Enabled {
		t.Skip("the race detector allocates")
	}
	pdf, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		t.Fatal(err)
	}
	checker := NewChecker(&CheckerOptions{FontCache: 32 << 20})
	for _, tc := range []struct {
		name   string
		budget float64
		f      func() error
	}{
		{"Extract", 225, func() error { _, err := Extract(pdf); return err }},
		{"Check", 1000, func() error { _, _, err := Check(pdf, nil); return err }},
		{"Check/PDFABasic", 650, func() error {
			_, _, err := Check(pdf, &validate.Options{PDFA: validate.PDFABasic})
			return err
		}},
		{"Checker.Check", 1000, func() error { _, _, err := checker.Check(pdf, nil); return err }},
		{"ReadAndCheck", 1050, func() error { _, _, _, err := ReadAndCheck(pdf, nil); return err }},
	} {
		var err error
		n := testing.AllocsPerRun(20, func() { err = tc.f() })
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if n > tc.budget {
			t.Errorf("%s: %v allocations per call, budget %v", tc.name, n, tc.budget)
		}
	}
}
