package zerechnung

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/validate"
)

// checkerPDF is a PDF of the tests of Checker, with what Check finds.
type checkerPDF struct {
	name string
	data []byte
	want *validate.Report
	att  *Attachment
}

// checkerPDFs returns the PDFs of the glyph tests, which embed the same
// TrueType and CFF programs in valid and invalid documents, and those of
// testdata, whose programs are compressed.
func checkerPDFs(t testing.TB) []checkerPDF {
	var pdfs []checkerPDF
	for _, gc := range glyphCases() {
		pdfs = append(pdfs, checkerPDF{name: gc.name, data: gc.f.build()})
	}
	for _, name := range []string{"testdata/EN16931_Einfach.pdf", "testdata/host.pdf"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		pdfs = append(pdfs, checkerPDF{name: name, data: data})
	}
	for i := range pdfs {
		r, att, err := Check(pdfs[i].data, nil)
		if err != nil {
			t.Fatalf("%s: %v", pdfs[i].name, err)
		}
		pdfs[i].want, pdfs[i].att = r, att
	}
	return pdfs
}

func (p *checkerPDF) compare(t testing.TB, c *Checker) {
	t.Helper()
	r, att, err := c.Check(p.data, nil)
	if err != nil {
		t.Fatalf("%s: %v", p.name, err)
	}
	if !reflect.DeepEqual(r, p.want) || !reflect.DeepEqual(att, p.att) {
		t.Errorf("%s: Checker.Check finds %v, Check finds %v", p.name, r.Findings, p.want.Findings)
	}
}

// TestChecker checks that a Checker finds what Check finds, from the first
// check of a program and then from its cache, which the valid and invalid
// PDFs share.
func TestChecker(t *testing.T) {
	pdfs := checkerPDFs(t)
	c := NewChecker(&CheckerOptions{FontCache: 32 << 20})
	for range 2 {
		for i := range pdfs {
			pdfs[i].compare(t, c)
		}
	}
	// The TrueType and CFF programs of fonttest, and the three TrueType
	// programs of EN16931_Einfach.pdf.
	fc := c.fonts
	if len(fc.entries) != 5 || fc.misses != 5 || fc.hits == 0 {
		t.Errorf("%d programs cached, %d misses, %d hits; want 5, 5 and some", len(fc.entries), fc.misses, fc.hits)
	}
	// Without a cache.
	for _, c := range []*Checker{nil, {}, NewChecker(nil), NewChecker(&CheckerOptions{FontCache: -1})} {
		if c != nil && c.fonts != nil {
			t.Errorf("%+v has a cache", c)
		}
		pdfs[0].compare(t, c)
	}
}

// TestCheckerConcurrent checks the PDFs from several goroutines that share
// a Checker, whose cache is small enough to evict programs meanwhile; run
// it with -race.
func TestCheckerConcurrent(t *testing.T) {
	pdfs := checkerPDFs(t)
	c := NewChecker(&CheckerOptions{FontCache: 600 << 10}) // two programs of EN16931_Einfach.pdf
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 2 * len(pdfs) {
				pdfs[(g+i)%len(pdfs)].compare(t, c)
			}
		}()
	}
	wg.Wait()
	if c.fonts.size > c.fonts.max {
		t.Errorf("the cache keeps %d bytes, more than %d", c.fonts.size, c.fonts.max)
	}
}

// TestFontCacheEviction covers the bounds of the cache: the least recently
// used programs go first, and a program larger than an eighth of the cache
// is not kept.
func TestFontCacheEviction(t *testing.T) {
	fc := newFontCache(8 * fontEntryOverhead)
	key := func(i int) fontKey { return fontKey{byte(i)} }
	for i := range 8 {
		fc.put(&fontEntry{key: key(i), size: fontEntryOverhead})
	}
	if fc.get(key(0)) == nil { // 0 is now the most recently used
		t.Fatal("the cache lost an entry before it was full")
	}
	fc.put(&fontEntry{key: key(8), size: fontEntryOverhead})
	if fc.get(key(1)) != nil {
		t.Error("the least recently used entry was kept")
	}
	for _, i := range []int{0, 2, 7, 8} {
		if fc.get(key(i)) == nil {
			t.Errorf("entry %d was evicted", i)
		}
	}
	if len(fc.entries) != 8 || fc.lru.Len() != 8 || fc.size != 8*fontEntryOverhead {
		t.Errorf("%d entries, %d in the list, %d bytes", len(fc.entries), fc.lru.Len(), fc.size)
	}
	fc.put(&fontEntry{key: key(9), size: fontEntryOverhead + 1})
	if fc.get(key(9)) != nil {
		t.Error("an entry larger than an eighth of the cache was kept")
	}
	// A program added twice, by two checks that missed it together.
	fc.put(&fontEntry{key: key(8), size: fontEntryOverhead})
	if len(fc.entries) != 8 || fc.size != 8*fontEntryOverhead {
		t.Errorf("%d entries, %d bytes after adding an entry again", len(fc.entries), fc.size)
	}

	// Programs too large for the cache are read, not kept.
	pdfs := checkerPDFs(t)
	c := NewChecker(&CheckerOptions{FontCache: 64 << 10})
	for i := range pdfs {
		pdfs[i].compare(t, c)
	}
	for _, el := range c.fonts.entries {
		if e := el.Value.(*fontEntry); e.size > c.fonts.maxEntry {
			t.Errorf("an entry of %d bytes was kept", e.size)
		}
	}
}

// TestProgramKey checks that the key of a program changes with what decodes
// and reads it, and only with that.
func TestProgramKey(t *testing.T) {
	stream := func(filter, parms pdf.Object, raw string, extra ...pdf.KV) *pdf.Stream {
		d := pdf.Dict{{Key: "Filter", Val: filter}, {Key: "DecodeParms", Val: parms}}
		return &pdf.Stream{Dict: append(d, extra...), Raw: []byte(raw)}
	}
	parms := func(predictor int) pdf.Dict {
		return pdf.Dict{{Key: "Predictor", Val: pdf.Int(predictor)}, {Key: "Columns", Val: pdf.Int(4)}}
	}
	key := func(st *pdf.Stream, name pdf.Name, hint int) fontKey {
		t.Helper()
		k, ok := programKey(nil, st, name, hint) // direct objects: no reader needed
		if !ok {
			t.Fatalf("no key for %v", st.Dict)
		}
		return k
	}
	base := key(stream(pdf.Name("FlateDecode"), parms(12), "abc"), "FontFile2", 100)
	// The other entries of the stream do not decode it.
	if k := key(stream(pdf.Name("FlateDecode"), parms(12), "abc", pdf.KV{Key: "Length", Val: pdf.Int(3)}), "FontFile2", 100); k != base {
		t.Error("the key depends on /Length")
	}
	for name, k := range map[string]fontKey{
		"font file":     key(stream(pdf.Name("FlateDecode"), parms(12), "abc"), "FontFile3", 100),
		"hint":          key(stream(pdf.Name("FlateDecode"), parms(12), "abc"), "FontFile2", 101),
		"filter":        key(stream(pdf.Name("LZWDecode"), parms(12), "abc"), "FontFile2", 100),
		"filter array":  key(stream(pdf.Array{pdf.Name("FlateDecode")}, parms(12), "abc"), "FontFile2", 100),
		"parameters":    key(stream(pdf.Name("FlateDecode"), parms(10), "abc"), "FontFile2", 100),
		"no parameters": key(stream(pdf.Name("FlateDecode"), nil, "abc"), "FontFile2", 100),
		"data":          key(stream(pdf.Name("FlateDecode"), parms(12), "abd"), "FontFile2", 100),
		"longer data":   key(stream(pdf.Name("FlateDecode"), parms(12), "abcd"), "FontFile2", 100),
	} {
		if k == base {
			t.Errorf("the key does not depend on the %s", name)
		}
	}
	// A stream among the parameters is not hashed: the program is read
	// without the cache.
	if _, ok := programKey(nil, stream(pdf.Name("FlateDecode"), pdf.Dict{{Key: "X", Val: &pdf.Stream{}}}, "abc"), "FontFile2", 0); ok {
		t.Error("a stream among the parameters has a key")
	}
}

func benchChecker(b *testing.B, names ...string) {
	var pdfs [][]byte
	size := 0
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			b.Fatal(err)
		}
		pdfs = append(pdfs, data)
		size += len(data)
	}
	c := NewChecker(&CheckerOptions{FontCache: 32 << 20})
	b.SetBytes(int64(size / len(pdfs)))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, _, err := c.Check(pdfs[i%len(pdfs)], nil); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// BenchmarkChecker measures Checker.Check on the same PDF, its fonts found
// in the cache from the second check on; BenchmarkCheckerAlternate on two
// PDFs in turn, with different fonts.
func BenchmarkChecker(b *testing.B) { benchChecker(b, "testdata/EN16931_Einfach.pdf") }
func BenchmarkCheckerAlternate(b *testing.B) {
	benchChecker(b, "testdata/EN16931_Einfach.pdf", "testdata/host.pdf")
}
