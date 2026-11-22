package render

import (
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/ritoon/zerechnung/internal/font"
)

//go:embed fonts/Go-Regular.ttf fonts/Go-Bold.ttf
var fontFiles embed.FS

// face is a TrueType font program and its metrics, shared by the documents.
type face struct {
	name    string // PostScript name, without subset tag
	data    []byte
	tt      *font.TrueType
	upem    float64
	ascent  float64 // in 1000 units of glyph space
	descent float64
	capH    float64
	bbox    [4]float64
	italic  float64
	stemV   int
	bold    bool
	cmap    font.CmapID
	low     []uint16 // glyphs of the characters below cacheLimit
	glyf    []byte
	loca    []uint32 // NumGlyphs+1 offsets in glyf
	tables  map[string][]byte
}

var (
	facesOnce sync.Once
	faces     [2]*face // regular, bold
	facesErr  error
)

// loadFaces parses the embedded fonts once.
func loadFaces() ([2]*face, error) {
	facesOnce.Do(func() {
		for i, f := range []struct {
			file, name string
			bold       bool
		}{{"fonts/Go-Regular.ttf", "Go-Regular", false}, {"fonts/Go-Bold.ttf", "Go-Bold", true}} {
			data, err := fontFiles.ReadFile(f.file)
			if err != nil {
				facesErr = err
				return
			}
			fc, err := newFace(f.name, data, f.bold)
			if err != nil {
				facesErr = fmt.Errorf("render: %s: %w", f.file, err)
				return
			}
			faces[i] = fc
		}
	})
	return faces, facesErr
}

func be16(b []byte, at int) int {
	if at < 0 || at+2 > len(b) {
		return 0
	}
	return int(binary.BigEndian.Uint16(b[at:]))
}

func bes16(b []byte, at int) int { return int(int16(be16(b, at))) }

// newFace reads the tables of a TrueType font program that the renderer
// needs: metrics for the font descriptor, glyph data for subsetting.
func newFace(name string, data []byte, bold bool) (*face, error) {
	tt, err := font.ParseTrueType(data)
	if err != nil {
		return nil, err
	}
	f := &face{name: name, data: data, tt: tt, upem: float64(tt.UnitsPerEm), bold: bold, tables: map[string][]byte{}}
	if f.upem == 0 {
		return nil, errors.New("no unitsPerEm")
	}
	n := be16(data, 4)
	for i := 0; i < n; i++ {
		at := 12 + 16*i
		tag := string(data[at : at+4])
		off := int(binary.BigEndian.Uint32(data[at+8:]))
		size := int(binary.BigEndian.Uint32(data[at+12:]))
		if off < 0 || size < 0 || off+size > len(data) {
			return nil, fmt.Errorf("table %q out of bounds", tag)
		}
		f.tables[tag] = data[off : off+size]
	}
	for _, t := range []string{"head", "hhea", "hmtx", "maxp", "loca", "glyf"} {
		if f.tables[t] == nil {
			return nil, fmt.Errorf("no %s table", t)
		}
	}
	scale := func(v int) float64 { return math.Round(float64(v) * 1000 / f.upem) }
	head, hhea := f.tables["head"], f.tables["hhea"]
	f.bbox = [4]float64{scale(bes16(head, 36)), scale(bes16(head, 38)), scale(bes16(head, 40)), scale(bes16(head, 42))}
	f.ascent, f.descent = scale(bes16(hhea, 4)), scale(bes16(hhea, 6))
	f.capH = f.ascent
	if os2 := f.tables["OS/2"]; be16(os2, 0) >= 2 && len(os2) >= 90 {
		f.capH = scale(bes16(os2, 88))
	}
	if post := f.tables["post"]; len(post) >= 8 {
		f.italic = float64(int32(binary.BigEndian.Uint32(post[4:]))) / 65536
	}
	f.stemV = 80
	if bold {
		f.stemV = 140
	}
	switch {
	case tt.HasCmap(font.CmapID{Platform: 3, Encoding: 10}):
		f.cmap = font.CmapID{Platform: 3, Encoding: 10}
	case tt.HasCmap(font.CmapID{Platform: 3, Encoding: 1}):
		f.cmap = font.CmapID{Platform: 3, Encoding: 1}
	default:
		return nil, errors.New("no Unicode cmap")
	}
	f.low = make([]uint16, cacheLimit)
	for r := range f.low {
		if g := tt.Lookup(f.cmap, uint32(r)); g < tt.NumGlyphs {
			f.low[r] = uint16(g)
		}
	}
	f.glyf = f.tables["glyf"]
	long := bes16(head, 50) == 1
	loca := f.tables["loca"]
	f.loca = make([]uint32, tt.NumGlyphs+1)
	for i := range f.loca {
		if long {
			if 4*i+4 > len(loca) {
				return nil, errors.New("short loca table")
			}
			f.loca[i] = binary.BigEndian.Uint32(loca[4*i:])
		} else {
			if 2*i+2 > len(loca) {
				return nil, errors.New("short loca table")
			}
			f.loca[i] = uint32(binary.BigEndian.Uint16(loca[2*i:])) * 2
		}
		if int(f.loca[i]) > len(f.glyf) || (i > 0 && f.loca[i] < f.loca[i-1]) {
			return nil, errors.New("malformed loca table")
		}
	}
	return f, nil
}

// cacheLimit bounds the characters whose glyph is looked up once, when the
// font is loaded: the alphabets and punctuation of the European languages.
const cacheLimit = 0x3000

// glyph returns the glyph of r, 0 when the font has none.
func (f *face) glyph(r rune) int {
	if r >= 0 && r < rune(len(f.low)) {
		return int(f.low[r])
	}
	if r < 0 || r > 0x10ffff {
		return 0
	}
	g := f.tt.Lookup(f.cmap, uint32(r))
	if g >= f.tt.NumGlyphs {
		return 0
	}
	return g
}

// advance returns the advance width of a glyph in 1000 units of glyph
// space, as the PDF/A checks compute it from the font program.
func (f *face) advance(gid int) float64 {
	a, _ := f.tt.Advance(gid)
	return float64(a) * 1000 / f.upem
}

// width returns the width in 1000 units of the glyph that the font dictionary
// declares: the advance width rounded to the integer.
func (f *face) width(gid int) int { return int(math.Round(f.advance(gid))) }

// usedFont is a face as used by one document: the glyphs it shows and the
// characters they stand for.
type usedFont struct {
	*face
	res     string       // resource name, F1 or F2
	toUni   map[int]rune // glyph → character, for the ToUnicode CMap
	replace int          // glyph of '?', shown for a character the font lacks
}

func newUsedFont(f *face, res string) *usedFont {
	u := &usedFont{face: f, res: res, toUni: map[int]rune{}}
	u.replace = f.glyph('?')
	return u
}

// normalize maps the characters the renderer does not show as such to the
// ones it shows: spaces of every kind to the space, other control characters
// to nothing.
func normalize(r rune) (rune, bool) {
	switch {
	case r == '\t' || r == '\u00a0' || r == '\u2007' || r == '\u202f' || r == '\u2009' || r == '\u200a':
		return ' ', true
	case r < 0x20 || (r >= 0x7f && r < 0xa0) || r == '\u00ad' || r == '\u200b' || r == 0xfeff:
		return 0, false
	}
	return r, true
}

// encode returns the glyphs that show s, recording them as used. A character
// that the font lacks is shown as '?': PDF/A forbids showing .notdef.
func (u *usedFont) encode(s string) []int {
	gids := make([]int, 0, len(s))
	for _, r := range s {
		r, ok := normalize(r)
		if !ok {
			continue
		}
		g := u.glyph(r)
		if g == 0 {
			g, r = u.replace, '?'
		}
		if _, seen := u.toUni[g]; !seen {
			u.toUni[g] = r
		}
		gids = append(gids, g)
	}
	return gids
}

// measure returns the width of s in points at size.
func (f *face) measure(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		r, ok := normalize(r)
		if !ok {
			continue
		}
		g := f.glyph(r)
		if g == 0 {
			g = f.glyph('?')
		}
		w += float64(f.width(g))
	}
	return w * size / 1000
}

// usedGlyphs returns the glyphs shown, sorted.
func (u *usedFont) usedGlyphs() []int {
	gids := make([]int, 0, len(u.toUni))
	for g := range u.toUni {
		gids = append(gids, g)
	}
	sort.Ints(gids)
	return gids
}

// subsetTag returns the six capital letters that name the subset, derived
// from the glyphs it holds so that the output is reproducible.
func (u *usedFont) subsetTag() string {
	h := sha256.New()
	for _, g := range u.usedGlyphs() {
		fmt.Fprintf(h, "%d,", g)
	}
	sum := h.Sum(nil)
	var b [6]byte
	for i := range b {
		b[i] = 'A' + sum[i]%26
	}
	return string(b[:])
}

// baseFont is the name of the subset font: tag, plus sign, PostScript name.
func (u *usedFont) baseFont() string { return u.subsetTag() + "+" + u.name }

// widths returns the /W array of the CIDFont for the glyphs shown.
func (u *usedFont) widths() string {
	var b strings.Builder
	b.WriteString("[")
	gids := u.usedGlyphs()
	for i := 0; i < len(gids); {
		j := i
		for j+1 < len(gids) && gids[j+1] == gids[j]+1 {
			j++
		}
		fmt.Fprintf(&b, " %d [", gids[i])
		for k := i; k <= j; k++ {
			if k > i {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, "%d", u.width(gids[k]))
		}
		b.WriteString("]")
		i = j + 1
	}
	b.WriteString(" ]")
	return b.String()
}

// toUnicode returns the ToUnicode CMap of the glyphs shown.
func (u *usedFont) toUnicode() []byte {
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	gids := u.usedGlyphs()
	for len(gids) > 0 {
		n := min(len(gids), 100)
		fmt.Fprintf(&b, "%d beginbfchar\n", n)
		for _, g := range gids[:n] {
			fmt.Fprintf(&b, "<%04X> <", g)
			for _, c := range utf16.Encode([]rune{u.toUni[g]}) {
				fmt.Fprintf(&b, "%04X", c)
			}
			b.WriteString(">\n")
		}
		b.WriteString("endbfchar\n")
		gids = gids[n:]
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return []byte(b.String())
}

// Composite glyph flags (ISO/IEC 14496-22, glyf table).
const (
	argsAreWords   = 0x0001
	haveScale      = 0x0008
	moreComponents = 0x0020
	haveXYScale    = 0x0040
	haveTwoByTwo   = 0x0080
)

// glyphData returns the glyf data of a glyph.
func (f *face) glyphData(g int) []byte {
	if g < 0 || g+1 >= len(f.loca) {
		return nil
	}
	return f.glyf[f.loca[g]:f.loca[g+1]]
}

// components returns the glyphs a composite glyph is made of.
func components(data []byte) []int {
	if len(data) < 10 || bes16(data, 0) >= 0 {
		return nil
	}
	var out []int
	at := 10
	for at+4 <= len(data) {
		flags := be16(data, at)
		out = append(out, be16(data, at+2))
		at += 4
		if flags&argsAreWords != 0 {
			at += 4
		} else {
			at += 2
		}
		switch {
		case flags&haveScale != 0:
			at += 2
		case flags&haveXYScale != 0:
			at += 4
		case flags&haveTwoByTwo != 0:
			at += 8
		}
		if flags&moreComponents == 0 {
			break
		}
	}
	return out
}

// subset returns the font program reduced to the glyphs shown: the other
// glyphs keep their index and metrics but lose their outline, so the CIDs of
// the content streams (glyph indices, with /CIDToGIDMap /Identity) stay
// valid. Layout tables that refer to removed glyphs are dropped.
func (u *usedFont) subset() []byte {
	keep := map[int]bool{}
	var add func(g, depth int)
	add = func(g, depth int) {
		if keep[g] || depth > 8 {
			return
		}
		keep[g] = true
		for _, c := range components(u.glyphData(g)) {
			add(c, depth+1)
		}
	}
	add(0, 0)
	for g := range u.toUni {
		add(g, 0)
	}

	n := len(u.loca) - 1
	var glyf []byte
	loca := make([]byte, 4*(n+1))
	for g := 0; g < n; g++ {
		binary.BigEndian.PutUint32(loca[4*g:], uint32(len(glyf)))
		if keep[g] {
			glyf = append(glyf, u.glyphData(g)...)
			for len(glyf)%4 != 0 {
				glyf = append(glyf, 0)
			}
		}
	}
	binary.BigEndian.PutUint32(loca[4*n:], uint32(len(glyf)))

	head := append([]byte(nil), u.tables["head"]...)
	binary.BigEndian.PutUint32(head[8:], 0)  // checkSumAdjustment, set below
	binary.BigEndian.PutUint16(head[50:], 1) // indexToLocFormat: long offsets

	tables := map[string][]byte{"head": head, "glyf": glyf, "loca": loca}
	for _, t := range []string{"OS/2", "cmap", "cvt ", "fpgm", "gasp", "hhea", "hmtx", "maxp", "name", "prep"} {
		if d, ok := u.tables[t]; ok {
			tables[t] = d
		}
	}
	// The glyph names of the post table are of no use to a CIDFont: version
	// 3.0 keeps the header only.
	if post := u.tables["post"]; len(post) >= 32 {
		post = append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(post, 0x00030000)
		tables["post"] = post
	}
	return assemble(tables)
}

// checksum is the table checksum of ISO/IEC 14496-22.
func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

// assemble writes a TrueType font program from its tables, sorted by tag,
// and sets the checkSumAdjustment of the head table.
func assemble(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	pow, log := 1, 0
	for pow*2 <= n {
		pow *= 2
		log++
	}
	out := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(out, 0x00010000)
	binary.BigEndian.PutUint16(out[4:], uint16(n))
	binary.BigEndian.PutUint16(out[6:], uint16(16*pow))
	binary.BigEndian.PutUint16(out[8:], uint16(log))
	binary.BigEndian.PutUint16(out[10:], uint16(16*n-16*pow))
	headAt := 0
	for i, t := range tags {
		d := tables[t]
		at := 12 + 16*i
		copy(out[at:], t)
		binary.BigEndian.PutUint32(out[at+4:], checksum(d))
		binary.BigEndian.PutUint32(out[at+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[at+12:], uint32(len(d)))
		if t == "head" {
			headAt = len(out)
		}
		out = append(out, d...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	binary.BigEndian.PutUint32(out[headAt+8:], 0xB1B0AFBA-checksum(out))
	return out
}
