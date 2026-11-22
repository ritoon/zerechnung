package de

import (
	"math/big"
	"testing"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

func TestXSRat(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string // "" when the value is not a decimal
	}{
		{"12.50", "25/2"},
		{" \n+0.5\t", "1/2"},
		{"-.5", "-1/2"},
		{"5.", "5"},
		{"10.000000000000000000", "10"}, // 18 decimals, beyond int64
		{"168125.0000000", "168125"},
		{"", ""},
		{".", ""},
		{"+", ""},
		{"1e5", ""},
		{"1/2", ""},
		{"0x10", ""},
		{"1_000", ""},
		{"1,5", ""},
		{"1.2.3", ""},
		{" 1", ""},
	} {
		got, ok := xsRat(&xmltree.Node{Text: c.in})
		switch {
		case c.want == "" && ok:
			t.Errorf("xsRat(%q) = %v, want an error", c.in, got)
		case c.want != "" && !ok:
			t.Errorf("xsRat(%q) failed, want %s", c.in, c.want)
		case c.want != "":
			if want, _ := new(big.Rat).SetString(c.want); got.Cmp(want) != 0 {
				t.Errorf("xsRat(%q) = %v, want %s", c.in, got, c.want)
			}
		}
	}
	if _, ok := xsRat(nil); ok {
		t.Error("xsRat(nil) succeeded")
	}
}
