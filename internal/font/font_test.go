package font

import (
	"testing"

	"github.com/ritoon/zerechnung/internal/font/fonttest"
)

// TestNames pins the shape of the name tables and the codes where the
// encodings differ, which a shift or a bad edit would break.
func TestNames(t *testing.T) {
	count := func(t [256]string) int {
		n := 0
		for _, s := range t {
			if s != "" {
				n++
			}
		}
		return n
	}
	if n := count(standardEncoding); n != 149 {
		t.Errorf("StandardEncoding: %d codes", n)
	}
	if n := count(winAnsiEncoding); n != 224 {
		t.Errorf("WinAnsiEncoding: %d codes", n)
	}
	if n := count(macRomanEncoding); n != 208 {
		t.Errorf("MacRomanEncoding: %d codes", n)
	}
	for _, tc := range []struct {
		table [256]string
		code  byte
		want  string
	}{
		{standardEncoding, 047, "quoteright"}, {standardEncoding, 0140, "quoteleft"}, {standardEncoding, 0365, "dotlessi"},
		{winAnsiEncoding, 047, "quotesingle"}, {winAnsiEncoding, 0200, "Euro"}, {winAnsiEncoding, 0240, "space"},
		{winAnsiEncoding, 0255, "hyphen"}, {winAnsiEncoding, 0237, "Ydieresis"}, {winAnsiEncoding, 0201, "bullet"}, {winAnsiEncoding, 037, ""},
		{macRomanEncoding, 0333, "currency"}, {macRomanEncoding, 0312, "space"}, {macRomanEncoding, 0336, "fi"},
		{macRomanEncoding, 0255, ""}, {macRomanEncoding, 0360, ""},
	} {
		if got := tc.table[tc.code]; got != tc.want {
			t.Errorf("code %#o: %q, want %q", tc.code, got, tc.want)
		}
	}
	if cffStandardStrings[0] != ".notdef" || cffStandardStrings[228] != "zcaron" || cffStandardStrings[390] != "Semibold" {
		t.Error("CFF standard strings shifted")
	}
	if macGlyphNames[3] != "space" || macGlyphNames[257] != "dcroat" {
		t.Error("Macintosh glyph names shifted")
	}
	m := agl()
	if len(m) != 4281 || m["Euro"] != 0x20ac || m["quoteright"] != 0x2019 || m["space"] != 0x20 || m["dalethatafpatah"] != 0x05d3 {
		t.Errorf("Adobe Glyph List: %d names", len(m))
	}
	if !InGlyphList("grave") || InGlyphList("grav") {
		t.Error("InGlyphList")
	}
}

func TestGlyphList(t *testing.T) {
	for _, tc := range []struct {
		name string
		want rune
		ok   bool
	}{
		{"A", 'A', true}, {"eacute", 'é', true}, {"alpha", 'α', true}, {"Euro", 0x20ac, true},
		{"uni20AC", 0, false}, {"g123", 0, false}, {"A.sc", 0, false},
	} {
		if got, ok := GlyphList(tc.name); got != tc.want || ok != tc.ok {
			t.Errorf("%s: %U %v, want %U %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
	if c, ok := MacRomanCode("eacute"); !ok || c != 0216 {
		t.Errorf("MacRomanCode(eacute): %#o %v", c, ok)
	}
	if _, ok := MacRomanCode("Euro"); ok {
		t.Error("MacRomanCode(Euro)")
	}
}

func TestTrueType(t *testing.T) {
	tt, err := ParseTrueType(fonttest.TrueType())
	if err != nil {
		t.Fatal(err)
	}
	if tt.UnitsPerEm != 1000 || tt.NumGlyphs != 3 {
		t.Errorf("unitsPerEm %d, numGlyphs %d", tt.UnitsPerEm, tt.NumGlyphs)
	}
	for gid, want := range []int{500, 600, 600} {
		if w, ok := tt.Advance(gid); !ok || w != want {
			t.Errorf("advance %d: %d %v", gid, w, ok)
		}
	}
	if _, ok := tt.Advance(3); ok {
		t.Error("advance of glyph 3")
	}
	ms := CmapID{3, 1}
	if !tt.HasCmap(ms) || tt.HasCmap(CmapID{3, 0}) || len(tt.Cmaps()) != 1 {
		t.Errorf("cmaps %v", tt.Cmaps())
	}
	for code, want := range map[uint32]int{'A': 1, 'B': 2, 'C': 0, '@': 0} {
		if got := tt.Lookup(ms, code); got != want {
			t.Errorf("lookup %q: %d, want %d", code, got, want)
		}
	}
	if c, ok := tt.SampleCode(ms); !ok || c != 'A' {
		t.Errorf("sample code %q %v", c, ok)
	}
	if gid, ok := tt.PostGID("B.alt2"); !ok || gid != 2 {
		t.Errorf("PostGID(B.alt2): %d %v", gid, ok)
	}
	for gid, want := range []string{".notdef", "A", "B.alt2", ""} {
		if got := tt.GlyphName(gid); got != want {
			t.Errorf("name %d: %q, want %q", gid, got, want)
		}
	}
	if _, err := ParseTrueType([]byte("not a font")); err == nil {
		t.Error("garbage parsed")
	}
}

func TestCFF(t *testing.T) {
	c, err := ParseCFF(fonttest.CFF())
	if err != nil {
		t.Fatal(err)
	}
	if c.CID || c.NumGlyphs() != 3 {
		t.Fatalf("CID %v, %d glyphs", c.CID, c.NumGlyphs())
	}
	if got := c.Names(); len(got) != 3 || got[0] != ".notdef" || got[1] != "A" || got[2] != "A.sc" {
		t.Errorf("names %q", got)
	}
	if gid, ok := c.GID("A.sc"); !ok || gid != 2 {
		t.Errorf("GID A.sc: %d %v", gid, ok)
	}
	if gid, ok := c.EncodingGID('A'); !ok || gid != 1 {
		t.Errorf("encoding A: %d %v", gid, ok)
	}
	for gid, want := range []float64{500, 650, 600} {
		if w, ok := c.Width(gid); !ok || w != want {
			t.Errorf("width %d: %v %v, want %v", gid, w, ok, want)
		}
	}
	if m := c.FontMatrix(); m[0] != 0.001 {
		t.Errorf("matrix %v", m)
	}
}
