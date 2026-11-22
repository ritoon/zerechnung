package zerechnung

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sync"

	"github.com/ritoon/zerechnung/internal/font"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// CheckerOptions configures a Checker.
type CheckerOptions struct {
	// FontCache is the size of the cache of font programs, in bytes of
	// decoded programs (see Checker). Zero or less: no cache. A program
	// larger than an eighth of it is not kept.
	FontCache int64
}

// Checker checks hybrid PDFs as Check does, and keeps in a cache the font
// programs it reads for the PDF/A checks of the glyphs (validate.PDFAFull).
// The invoices of an issuer come from the same software and often embed
// the same fonts, byte for byte: a server that checks many of them decodes
// and reads each program once, then finds it in the cache. Decompressing
// the fonts takes about half the time of Check: from the second invoice with
// the same fonts, the check takes about 40 % less time.
//
// A program is found by the SHA-256 of its encoded data, of its filters
// and their parameters, and of the font file key (FontFile2, FontFile3):
// two different programs never share an entry, and the findings are those
// of Check. What depends on the document (encoding, widths, glyphs shown)
// is not cached. The cache keeps the most recently used programs within
// CheckerOptions.FontCache bytes of decoded data, plus a fixed overhead per
// program.
//
// A Checker is safe for concurrent use by multiple goroutines. Its zero
// value, like a nil *Checker, checks without a cache.
type Checker struct {
	fonts *fontCache
}

// NewChecker returns a Checker configured by opts, which may be nil.
func NewChecker(opts *CheckerOptions) *Checker {
	c := &Checker{}
	if opts != nil && opts.FontCache > 0 {
		c.fonts = newFontCache(opts.FontCache)
	}
	return c
}

// Check is Check with the cache of c.
func (c *Checker) Check(pdf []byte, opts *validate.Options) (*validate.Report, *Attachment, error) {
	return check(pdf, opts, c.cache())
}

// ReadAndCheck is ReadAndCheck with the cache of c.
func (c *Checker) ReadAndCheck(pdf []byte, opts *validate.Options) (*invoice.Invoice, *validate.Report, *Attachment, error) {
	return readAndCheck(pdf, opts, c.cache())
}

// cache returns the font cache of c, nil when it has none.
func (c *Checker) cache() *fontCache {
	if c == nil {
		return nil
	}
	return c.fonts
}

// fontEntryOverhead is what an entry of the font cache counts besides its
// decoded program: its key, its list element and the tables read.
const fontEntryOverhead = 512

// fontKey identifies a font program: the SHA-256 of what decodes and
// reads it (see programKey).
type fontKey [sha256.Size]byte

// fontCache is the cache of font programs of a Checker: an LRU list bounded
// by the bytes of the decoded programs it keeps.
type fontCache struct {
	max, maxEntry int64 // bytes kept, and bytes of the largest entry kept

	mu      sync.Mutex
	size    int64 // bytes of the entries kept
	entries map[fontKey]*list.Element
	lru     list.List // of *fontEntry, the most recently used first
	hits    int64
	misses  int64
}

// fontEntry is a font program read, or one that cannot be read (tt and cff
// nil). The tables are read-only once parsed, and safe to share.
type fontEntry struct {
	key  fontKey
	size int64
	tt   *font.TrueType
	cff  *font.CFF
}

func newFontCache(max int64) *fontCache {
	return &fontCache{max: max, maxEntry: max / 8, entries: map[fontKey]*list.Element{}}
}

// get returns the entry of key, nil when the cache does not have it.
func (fc *fontCache) get(key fontKey) *fontEntry {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	el, ok := fc.entries[key]
	if !ok {
		fc.misses++
		return nil
	}
	fc.hits++
	fc.lru.MoveToFront(el)
	return el.Value.(*fontEntry)
}

// put adds e, unless it is too large or another goroutine added it first,
// and evicts the least recently used entries beyond the size of the cache.
func (fc *fontCache) put(e *fontEntry) {
	if e.size > fc.maxEntry {
		return
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if el, ok := fc.entries[e.key]; ok {
		fc.lru.MoveToFront(el)
		return
	}
	fc.entries[e.key] = fc.lru.PushFront(e)
	fc.size += e.size
	for fc.size > fc.max {
		el := fc.lru.Back()
		old := el.Value.(*fontEntry)
		fc.lru.Remove(el)
		delete(fc.entries, old.key)
		fc.size -= old.size
	}
}

// programKey returns the key of the program of st: the SHA-256 of the font
// file key, of the size hint, of the filters and their parameters, resolved,
// and of the encoded data, which is all that decodes and reads it. ok is
// false when the parameters cannot be hashed (a stream, or too deep): the
// program is then read without the cache.
func programKey(r *pdf.Reader, st *pdf.Stream, key pdf.Name, hint int) (k fontKey, ok bool) {
	h := sha256.New()
	var buf []byte
	buf = appendHashed(buf, key)
	buf = binary.BigEndian.AppendUint64(buf, uint64(hint))
	if buf, ok = appendObject(buf, r, st.Dict.Get("Filter"), 0); !ok {
		return k, false
	}
	if buf, ok = appendObject(buf, r, st.Dict.Get("DecodeParms"), 0); !ok {
		return k, false
	}
	buf = binary.BigEndian.AppendUint64(buf, uint64(len(st.Raw)))
	h.Write(buf)
	h.Write(st.Raw)
	h.Sum(k[:0])
	return k, true
}

// appendHashed appends a name or string, length first.
func appendHashed[T ~string | ~[]byte](buf []byte, s T) []byte {
	buf = binary.BigEndian.AppendUint64(buf, uint64(len(s)))
	return append(buf, s...)
}

// appendObject appends an encoding of o, resolved, from which o can be read
// back: two objects with the same encoding decode a stream the same way.
func appendObject(buf []byte, r *pdf.Reader, o pdf.Object, depth int) ([]byte, bool) {
	if depth > 8 {
		return buf, false
	}
	switch v := r.Resolve(o).(type) {
	case nil:
		return append(buf, 'n'), true
	case pdf.Bool:
		if v {
			return append(buf, 'b', 1), true
		}
		return append(buf, 'b', 0), true
	case pdf.Int:
		return binary.BigEndian.AppendUint64(append(buf, 'i'), uint64(v)), true
	case pdf.Real:
		return binary.BigEndian.AppendUint64(append(buf, 'r'), math.Float64bits(float64(v))), true
	case pdf.Name:
		return appendHashed(append(buf, 'N'), v), true
	case pdf.String:
		return appendHashed(append(buf, 'S'), v), true
	case pdf.Array:
		buf = binary.BigEndian.AppendUint64(append(buf, 'A'), uint64(len(v)))
		for _, x := range v {
			var ok bool
			if buf, ok = appendObject(buf, r, x, depth+1); !ok {
				return buf, false
			}
		}
		return buf, true
	case pdf.Dict:
		buf = binary.BigEndian.AppendUint64(append(buf, 'D'), uint64(len(v)))
		for _, kv := range v {
			buf = appendHashed(buf, kv.Key)
			var ok bool
			if buf, ok = appendObject(buf, r, kv.Val, depth+1); !ok {
				return buf, false
			}
		}
		return buf, true
	}
	return buf, false // a stream, or a reference that does not resolve
}

// readProgram reads the program of a FontFile2 or FontFile3 stream, from
// the font cache of the Checker when there is one.
func (s *pdfaScan) readProgram(st *pdf.Stream, key pdf.Name) (*font.TrueType, *font.CFF) {
	hint := s.programHint(st, key)
	fc := s.k.fonts
	var k fontKey
	ok := false
	if fc != nil {
		k, ok = programKey(s.c.r, st, key, hint)
	}
	if !ok {
		// The buffer is lent by the scratch of the scan: the program is
		// not used once the scan ends.
		data, err := s.c.r.DecodeScratch(st, hint, &s.scratch)
		if err != nil {
			return nil, nil
		}
		return parseProgram(data, key)
	}
	if e := fc.get(k); e != nil {
		return e.tt, e.cff
	}
	// The cache keeps the decoded data: it cannot come from the scratch of
	// the scan, nor alias the PDF of the caller.
	e := &fontEntry{key: k, size: fontEntryOverhead}
	if data, err := s.c.r.DecodeSize(st, hint); err == nil {
		if len(data) > 0 && len(st.Raw) > 0 && &data[0] == &st.Raw[0] {
			data = bytes.Clone(data)
		}
		e.size += int64(len(data))
		e.tt, e.cff = parseProgram(data, key)
	}
	fc.put(e)
	return e.tt, e.cff
}
