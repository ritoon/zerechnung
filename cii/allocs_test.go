package cii

import (
	"os"
	"testing"

	"github.com/ritoon/zerechnung/internal/race"
)

// TestAllocs checks that Parse stays within its budget of allocations on
// testdata/EN16931_Einfach.cii.xml (see TestAllocs of package zerechnung).
func TestAllocs(t *testing.T) {
	if race.Enabled {
		t.Skip("the race detector allocates")
	}
	data, err := os.ReadFile("../testdata/EN16931_Einfach.cii.xml")
	if err != nil {
		t.Fatal(err)
	}
	const budget = 130
	n := testing.AllocsPerRun(50, func() {
		if _, err = Parse(data); err != nil {
			t.Fatal(err)
		}
	})
	if n > budget {
		t.Errorf("Parse: %v allocations per call, budget %v", n, budget)
	}
}
