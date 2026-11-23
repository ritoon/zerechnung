package zerechnung

import (
	"bytes"
	"regexp"
	"testing"
)

// The regular expressions cmapWMode and nextUseCMap replace: the oracle of
// TestCMapOperators and FuzzCMapOperators.
var (
	wmodeDef  = regexp.MustCompile(`/WMode\s+(\d+)\s+def`)
	useCMapOp = regexp.MustCompile(`/([^\s/\[\]<>(){}%]+)\s+usecmap`)
)

var cmapSamples = []string{
	"", "/WMode 0 def", "/WMode 1 def", "/WMode\t12\ndef", "/WMode 1def", "/WMode1 def", "/WMode  def",
	"/WMode 0 def /WMode 1 def", "/WMode x def /WMode 1 def", "/WMode 1 de", "/WMode\v1 def", "/WMode\f1\rdef",
	"//WMode 1 def", "/WMode /WMode 2 def", "/WMode 1 /WMode 2 def", "/WMode 12345678901234567890 def",
	"/Identity-H usecmap", "/UniJIS-UCS2-H usecmap /Other usecmap", "/A\tusecmap", "/A usecmapx",
	"/Ausecmap", "/ usecmap", "//A usecmap", "/A/B usecmap", "/A[ usecmap", "/A% usecmap",
	"/A\x00 usecmap", "/\xc3\xa9t\xc3\xa9 usecmap", "/\xff\xfe usecmap", "/A  \n usecmap /B usecmap",
	"/A usecma", "/A usecmap/B usecmap", "/A /B usecmap", "begincmap /CMapName /X def /Y usecmap endcmap",
}

func TestCMapOperators(t *testing.T) {
	for _, s := range cmapSamples {
		checkCMapOperators(t, []byte(s))
	}
}

func FuzzCMapOperators(f *testing.F) {
	for _, s := range cmapSamples {
		f.Add([]byte(s))
	}
	f.Fuzz(checkCMapOperators)
}

func checkCMapOperators(t *testing.T, data []byte) {
	t.Helper()
	got, ok := cmapWMode(data)
	m := wmodeDef.FindSubmatch(data)
	if ok != (m != nil) || ok && !bytes.Equal(got, m[1]) {
		t.Errorf("cmapWMode(%q) = %q, %v; the regular expression finds %q", data, got, ok, m)
	}
	var names [][]byte
	for name, i, ok := nextUseCMap(data, 0); ok; name, i, ok = nextUseCMap(data, i) {
		names = append(names, name)
	}
	want := useCMapOp.FindAllSubmatch(data, -1)
	if len(names) != len(want) {
		t.Fatalf("nextUseCMap(%q) finds %q; the regular expression finds %q", data, names, want)
	}
	for i := range names {
		if !bytes.Equal(names[i], want[i][1]) {
			t.Errorf("nextUseCMap(%q) finds %q; the regular expression finds %q", data, names, want)
		}
	}
	if hasUseCMap(data) != useCMapOp.Match(data) {
		t.Errorf("hasUseCMap(%q) = %v, the regular expression says %v", data, hasUseCMap(data), useCMapOp.Match(data))
	}
}
