package validate

import (
	"os"
	"testing"

	"github.com/ritoon/zerechnung/internal/race"
)

// TestAllocs checks that XML and Parse stay within their budget of
// allocations on the CII and UBL invoices of testdata (see TestAllocs of
// package zerechnung).
func TestAllocs(t *testing.T) {
	if race.Enabled {
		t.Skip("the race detector allocates")
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	cii, ubl := read("../testdata/EN16931_Einfach.cii.xml"), read("../testdata/XRECHNUNG_Einfach.ubl.xml")
	for _, tc := range []struct {
		name   string
		budget float64
		f      func() error
	}{
		{"XML/CII", 100, func() error { XML(cii, nil); return nil }},
		{"XML/UBL", 8700, func() error { XML(ubl, &Options{Auto: true}); return nil }},
		{"Parse/CII", 140, func() error { _, _, err := Parse(cii, nil); return err }},
	} {
		var err error
		n := testing.AllocsPerRun(50, func() { err = tc.f() })
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if n > tc.budget {
			t.Errorf("%s: %v allocations per call, budget %v", tc.name, n, tc.budget)
		}
	}
}
