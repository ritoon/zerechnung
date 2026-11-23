package zerechnung

import (
	"bytes"
	"fmt"
	"math"
	"strconv"

	"github.com/ritoon/zerechnung/internal/font"
	"github.com/ritoon/zerechnung/internal/pdf"
)

// This file checks the glyphs the text of the content streams shows
// (ISO 19005-2 and 19005-3, 6.2.11.4.1, 6.2.11.5 and 6.2.11.8): the
// embedded font program defines them, their widths in the font dictionary
// match those of the program, and none is .notdef. It also checks that the
// CharSet of a subset Type 1 font and the CIDSet of a subset CIDFont list
// the glyphs of their program (6.2.11.4.2). The font programs are read by
// internal/font: TrueType, CFF (Type1C, CIDFontType0C, OpenType) and the
// glyph descriptions of Type 3 fonts; Type 1 programs (FontFile) are not
// read. Codes are mapped to glyphs as veraPDF 1.30 does, whose rules
// 6.2.11.4.1-2, 6.2.11.5-1 and 6.2.11.8-1 these checks follow.

// glyphProblem is a kind of PDF-72 finding, reported once per font.
type glyphProblem uint8

const (
	glyphNotdef glyphProblem = 1 << iota
	glyphMissing
	glyphWidth
)

// glyphKind is how a font maps its codes to glyphs.
type glyphKind uint8

const (
	glyphNone     glyphKind = iota // not checked
	glyphCFF                       // Type 1 font with a CFF program: by glyph name
	glyphType1                     // Type 1 font with a Type 1 program, or none: names only
	glyphTrueType                  // TrueType font: through the cmap table
	glyphType3                     // Type 3 font: by glyph name, in CharProcs
	glyphCID0                      // CIDFontType0 with a CID-keyed CFF program
	glyphCID2                      // CIDFontType2 with a TrueType program
)

// glyphFont is what the glyph checks read of a font.
type glyphFont struct {
	kind glyphKind

	// Simple fonts.
	names    [256]string  // glyph names of the PDF encoding, "" for none
	gidNames *[256]string // TrueType: the names the codes are looked up by, nil to look up the codes
	symbolic bool         // TrueType: the codes are looked up through (3,0) or (1,0)
	first    int64
	last     int64
	widths   []float64 // Widths, NaN for an entry that is not a number; nil without FirstChar, LastChar and Widths
	missing  float64   // MissingWidth of the font descriptor, 0 without one

	// Type 0 fonts.
	cmap     *cidMap
	w        map[uint32]float64 // W
	dw       float64            // DW, 1000 without one
	cidToGID []byte             // CIDToGIDMap stream, nil for Identity

	cff *font.CFF
	tt  *font.TrueType
	t3  map[string]float64 // Type 3: width of the glyph descriptions by name, NaN when it cannot be read

	seen     [8]uint64       // simple fonts: codes checked, with their visibility, as bits code<<1 | visible
	checked  map[uint64]bool // Type 0 fonts: codes checked, with their size and visibility; made on first use
	reported glyphProblem
}

// glyphFont returns the glyph model of a font dictionary, built once per
// font.
func (s *pdfaScan) glyphFont(o pdf.Object) *glyphFont {
	cs := s.contentState()
	id := keyOf(o)
	if f := cs.fonts[id]; id != (objKey{}) && f != nil {
		return f
	}
	f := &glyphFont{}
	if d, ok := s.c.r.Dict(o); ok {
		switch s.name(d, "Subtype") {
		case "Type0":
			s.readCIDFont(d, f)
		case "Type3":
			s.readType3(d, f)
		case "Type1", "MMType1":
			s.readType1(d, f)
		case "TrueType":
			s.readTrueType(d, f)
		}
	}
	if id != (objKey{}) {
		cs.fonts[id] = f
	}
	return f
}

// simpleWidths reads the widths of a simple font: Widths, used for the
// codes from FirstChar to LastChar, and MissingWidth.
func (s *pdfaScan) simpleWidths(d, fd pdf.Dict, f *glyphFont) {
	r := s.c.r
	first, ok1 := pdf.AsInt(r.Resolve(d.Get("FirstChar")))
	last, ok2 := pdf.AsInt(r.Resolve(d.Get("LastChar")))
	if ws, ok := r.Resolve(d.Get("Widths")).(pdf.Array); ok && ok1 && ok2 && len(ws) > 0 {
		f.first, f.last = first, last
		f.widths = make([]float64, len(ws))
		for i, w := range ws {
			v, ok := pdfNumber(r.Resolve(w))
			if !ok {
				v = math.NaN()
			}
			f.widths[i] = v
		}
	}
	if fd != nil {
		f.missing, _ = pdfNumber(r.Resolve(fd.Get("MissingWidth")))
	}
}

// dictWidth returns the width the dictionary of a simple font gives a
// code.
func (f *glyphFont) dictWidth(code uint32) float64 {
	if f.widths != nil && int64(code) >= f.first && int64(code) <= f.last {
		if i := int64(code) - f.first; i < int64(len(f.widths)) {
			return f.widths[i]
		}
		return math.NaN()
	}
	return f.missing
}

// pdfEncoding reads the glyph names of the encoding of a simple font, as
// veraPDF names the glyphs: the table of WinAnsiEncoding or
// MacRomanEncoding, as /Encoding or /BaseEncoding, its unused codes
// .notdef; no name for another encoding (StandardEncoding included) or
// none; the Differences over them. It returns false for MacExpertEncoding,
// whose table internal/font does not have.
func (s *pdfaScan) pdfEncoding(d pdf.Dict, names *[256]string) bool {
	r := s.c.r
	table := func(n pdf.Name) bool {
		if n == "MacExpertEncoding" {
			return false
		}
		if n != "WinAnsiEncoding" && n != "MacRomanEncoding" {
			return true
		}
		t, _ := font.Encoding(string(n))
		for code, name := range t {
			if name == "" {
				name = ".notdef"
			}
			names[code] = name
		}
		return true
	}
	switch e := r.Resolve(d.Get("Encoding")).(type) {
	case pdf.Name:
		return table(e)
	case pdf.Dict:
		if b, ok := r.Resolve(e.Get("BaseEncoding")).(pdf.Name); ok && !table(b) {
			return false
		}
		differences(r, e, names)
	}
	return true
}

// differences applies the Differences of an encoding dictionary to names.
func differences(r *pdf.Reader, enc pdf.Dict, names *[256]string) {
	diffs, _ := r.Resolve(enc.Get("Differences")).(pdf.Array)
	code := int64(-1)
	for _, x := range diffs {
		switch v := r.Resolve(x).(type) {
		case pdf.Int:
			code = int64(v)
		case pdf.Name:
			if code >= 0 && code < 256 {
				names[code] = string(v)
			}
			if code >= 0 {
				code++
			}
		}
	}
}

func (s *pdfaScan) readType1(d pdf.Dict, f *glyphFont) {
	r := s.c.r
	if !s.pdfEncoding(d, &f.names) {
		return
	}
	f.kind = glyphType1
	fd, _ := r.Dict(d.Get("FontDescriptor"))
	if fd == nil {
		return
	}
	s.simpleWidths(d, fd, f)
	if p := s.program(fd); p.cff != nil && !p.cff.CID {
		f.kind, f.cff, f.tt = glyphCFF, p.cff, p.tt
	}
}

// readTrueType reads a simple TrueType font. Its codes are looked up as
// veraPDF does: those of a non-symbolic font with an encoding by their
// glyph name, through the (3,1) cmap and the Adobe Glyph List, the (1,0)
// cmap and MacRomanEncoding, then the post table; the others through the
// (3,0) or (1,0) cmap. A non-symbolic font with an encoding other than
// WinAnsiEncoding and MacRomanEncoding, which veraPDF does not read, is
// not checked (PDF-60 reports it).
func (s *pdfaScan) readTrueType(d pdf.Dict, f *glyphFont) {
	r := s.c.r
	fd, _ := r.Dict(d.Get("FontDescriptor"))
	if fd == nil {
		return
	}
	p := s.program(fd)
	if p.key != "FontFile2" || p.tt == nil {
		return
	}
	flags, _ := pdf.AsInt(r.Resolve(fd.Get("Flags")))
	enc := r.Resolve(d.Get("Encoding"))
	f.symbolic = flags&4 != 0 || enc == nil
	if !f.symbolic {
		var gid *[256]string // read only once set
		switch e := enc.(type) {
		case pdf.Name:
			if e != "WinAnsiEncoding" && e != "MacRomanEncoding" {
				return
			}
			gid, _ = font.Encoding(string(e)) // shared: not to be modified
		case pdf.Dict:
			b, _ := r.Resolve(e.Get("BaseEncoding")).(pdf.Name)
			if b == "MacExpertEncoding" {
				return
			}
			gid = new([256]string)
			t, ok := font.Encoding(string(b))
			if !ok || b == "StandardEncoding" {
				t, _ = font.Encoding("StandardEncoding")
			}
			*gid = *t
			differences(r, e, gid)
			std, _ := font.Encoding("StandardEncoding")
			for code, n := range gid {
				if n == "" || n == ".notdef" {
					gid[code] = std[code]
				}
			}
		default:
			return
		}
		f.gidNames = gid
		if !s.pdfEncoding(d, &f.names) {
			return
		}
		// A code the PDF encoding does not name takes the name it is
		// looked up by, or .notdef.
		for code, n := range f.names {
			if n == "" {
				f.names[code] = gid[code]
				if gid[code] == "" {
					f.names[code] = ".notdef"
				}
			}
		}
	}
	for _, id := range []font.CmapID{{Platform: 3, Encoding: 1}, {Platform: 1, Encoding: 0}, {Platform: 3, Encoding: 0}} {
		if p.tt.HasCmap(id) && !p.tt.CanLookup(id) {
			return // a cmap internal/font does not read
		}
	}
	s.simpleWidths(d, fd, f)
	f.kind, f.tt = glyphTrueType, p.tt
}

func (s *pdfaScan) readType3(d pdf.Dict, f *glyphFont) {
	r := s.c.r
	if !s.pdfEncoding(d, &f.names) {
		return
	}
	f.kind = glyphType3
	fd, _ := r.Dict(d.Get("FontDescriptor"))
	s.simpleWidths(d, fd, f)
	f.t3 = map[string]float64{}
	procs, _ := r.Dict(d.Get("CharProcs"))
	for _, kv := range procs {
		w := math.NaN()
		if st, ok := r.Resolve(kv.Val).(*pdf.Stream); ok {
			if data := s.decodeContent(st); data != nil {
				w = type3Width(data)
			}
		}
		f.t3[string(kv.Key)] = w
	}
}

// readCIDFont reads a Type 0 font whose CMap is Identity-H, Identity-V or
// embedded without using another CMap, and whose CIDFont has a TrueType or
// CID-keyed CFF program.
func (s *pdfaScan) readCIDFont(d pdf.Dict, f *glyphFont) {
	r := s.c.r
	switch enc := r.Resolve(d.Get("Encoding")).(type) {
	case pdf.Name:
		if enc == "Identity-H" || enc == "Identity-V" {
			f.cmap = identityCMap
		}
	case *pdf.Stream:
		if data, err := r.Decode(enc); err == nil {
			if m := parseCMap(data); len(m.space) > 0 && !hasUseCMap(data) && !enc.Dict.Has("UseCMap") {
				f.cmap = m
			}
		}
	}
	var cid pdf.Dict
	if list, ok := r.Resolve(d.Get("DescendantFonts")).(pdf.Array); ok && len(list) > 0 {
		cid, _ = r.Dict(list[0])
	}
	if f.cmap == nil || cid == nil {
		return
	}
	fd, _ := r.Dict(cid.Get("FontDescriptor"))
	if fd == nil {
		return
	}
	p := s.program(fd)
	switch s.name(cid, "Subtype") {
	case "CIDFontType0":
		if p.cff == nil || !p.cff.CID {
			return
		}
		f.kind, f.cff = glyphCID0, p.cff
	case "CIDFontType2":
		if p.key != "FontFile2" || p.tt == nil {
			return
		}
		if st, ok := r.Resolve(cid.Get("CIDToGIDMap")).(*pdf.Stream); ok {
			data, err := r.Decode(st)
			if err != nil {
				return
			}
			f.cidToGID = data
		}
		f.kind, f.tt = glyphCID2, p.tt
	default:
		return
	}
	f.dw = 1000
	if v, ok := pdfNumber(r.Resolve(cid.Get("DW"))); ok {
		f.dw = v
	}
	f.w = cidWidths(r, cid.Get("W"))
}

// glyphInfo is what a code shows.
type glyphInfo struct {
	name           string  // the glyph name veraPDF gives it, "" for none
	present, known bool    // the program defines the glyph; known when that is read
	progW, dictW   float64 // widths in thousandths of text space units (glyph space for Type 3)
	hasProgW       bool
}

// showGlyphs checks the glyphs strings of a text-showing operator show
// with the current font.
func (sc *streamScan) showGlyphs(strs []pdf.String) {
	if sc.cur.font == nil {
		return
	}
	s := sc.s
	ref, isRef := sc.cur.font.(pdf.Ref)
	f := sc.font
	if !isRef || ref != sc.fontRef || f == nil {
		f = s.glyphFont(sc.cur.font)
		if isRef {
			sc.fontRef, sc.font = ref, f
		}
	}
	if f.kind == glyphNone {
		return
	}
	visible := sc.cur.tr != 3
	for _, str := range strs {
		for len(str) > 0 {
			code, size := uint32(str[0]), 1
			if f.cmap != nil {
				code, size = f.cmap.next(str)
			}
			str = str[min(size, len(str)):]
			if f.cmap == nil {
				// A simple font: a code of one byte.
				bit := code << 1
				if visible {
					bit |= 1
				}
				if f.seen[bit>>6]&(1<<(bit&63)) != 0 {
					continue
				}
				f.seen[bit>>6] |= 1 << (bit & 63)
			} else {
				key := uint64(code)<<8 | uint64(size)<<1
				if visible {
					key |= 1
				}
				if f.checked[key] || len(f.checked) > 1<<16 {
					continue
				}
				if f.checked == nil {
					f.checked = map[uint64]bool{}
				}
				f.checked[key] = true
			}
			g := s.glyphOf(f, code)
			if g.name == ".notdef" {
				sc.glyphReport(f, glyphNotdef, code, size, "the text shows the code %s, mapped to the .notdef glyph (6.2.11.8)")
			}
			if !visible {
				continue
			}
			// veraPDF counts the code 0 as present, but in a Type 3 font.
			if g.known && !g.present && (code != 0 || f.kind == glyphType3) {
				sc.glyphReport(f, glyphMissing, code, size, "the text shows the code %s, whose glyph%s the embedded font program does not define (6.2.11.4.1)", glyphName(g.name))
			}
			if g.present && g.hasProgW && !math.IsNaN(g.dictW) && math.Abs(g.progW-g.dictW) > 1 {
				sc.glyphReport(f, glyphWidth, code, size, "the glyph of the code %s is %s wide in the font dictionary but %s in the font program (6.2.11.5)", fmtWidth(g.dictW), fmtWidth(g.progW))
			}
		}
	}
}

// glyphReport reports a problem of a font once, the code of size bytes
// first in the arguments of format.
func (sc *streamScan) glyphReport(f *glyphFont, p glyphProblem, code uint32, size int, format string, args ...any) {
	if f.reported&p != 0 {
		return
	}
	f.reported |= p
	args = append([]any{fmt.Sprintf("<%0*X>", 2*size, code)}, args...)
	sc.s.report("PDF-72", sc.x.where+" /Font /"+string(sc.cur.fontName), format, args...)
}

func glyphName(n string) string {
	if n == "" || n == ".notdef" {
		return ""
	}
	return " " + n
}

func fmtWidth(w float64) string { return strconv.FormatFloat(w, 'f', -1, 64) }

// glyphOf returns what a code shows with a font.
func (s *pdfaScan) glyphOf(f *glyphFont, code uint32) glyphInfo {
	var g glyphInfo
	switch f.kind {
	case glyphCID0, glyphCID2:
		cid, mapped := f.cmap.cid(code)
		g.dictW = f.dw
		if w, ok := f.w[cid]; ok {
			g.dictW = w
		}
		gid := 0
		if f.kind == glyphCID0 {
			gid, g.present = f.cff.GIDOfCID(int(cid))
			g.present = g.present && gid != 0
		} else {
			gid, g.present = f.cid2GID(cid)
		}
		g.present = g.present && mapped
		g.known = true
		if !g.present {
			g.name = ".notdef"
			return g
		}
		if f.kind == glyphCID0 {
			g.progW, g.hasProgW = cffWidth(f.cff, gid)
		} else {
			g.progW, g.hasProgW = ttWidth(f.tt, gid)
		}
		return g
	}
	g.name = f.names[code&0xff]
	g.dictW = f.dictWidth(code)
	switch f.kind {
	case glyphType1:
		// The program is not read.
	case glyphCFF:
		if g.name == "" {
			break // veraPDF looks the code up in the built-in encoding
		}
		gid, ok := f.cff.GID(g.name)
		g.present, g.known = ok, true
		if ok {
			g.progW, g.hasProgW = cffWidth(f.cff, gid)
		}
	case glyphType3:
		w, ok := f.t3[g.name]
		g.present, g.known = ok && g.name != "", true
		if g.present && !math.IsNaN(w) {
			g.progW, g.hasProgW = w, true
		}
	case glyphTrueType:
		gid := f.trueTypeGID(code & 0xff)
		g.present, g.known = gid > 0 && gid < f.tt.NumGlyphs, true
		if g.present {
			g.progW, g.hasProgW = ttWidth(f.tt, gid)
		}
	}
	return g
}

// cid2GID maps a CID of a CIDFontType2 font to its glyph, and reports
// whether veraPDF counts it present: not the CID 0, within the
// CIDToGIDMap, with a maxp table and a glyph below its number of glyphs.
func (f *glyphFont) cid2GID(cid uint32) (int, bool) {
	gid := int(cid)
	if f.cidToGID != nil {
		if 2*gid+1 >= len(f.cidToGID) {
			return 0, false
		}
		gid = int(f.cidToGID[2*gid])<<8 | int(f.cidToGID[2*gid+1])
	}
	return gid, cid != 0 && f.tt.Table("maxp") != nil && gid < f.tt.NumGlyphs
}

func cffWidth(c *font.CFF, gid int) (float64, bool) {
	w, ok := c.Width(gid)
	if !ok {
		return 0, false
	}
	return w * c.GlyphMatrix(gid)[0] * 1000, true
}

func ttWidth(tt *font.TrueType, gid int) (float64, bool) {
	w, ok := tt.Advance(gid)
	if !ok || tt.UnitsPerEm == 0 {
		return 0, false
	}
	return float64(w) * 1000 / float64(tt.UnitsPerEm), true
}

// trueTypeGID maps a code of a simple TrueType font to a glyph, 0 for
// none.
func (f *glyphFont) trueTypeGID(code uint32) int {
	tt := f.tt
	ms, mac, sym := font.CmapID{Platform: 3, Encoding: 1}, font.CmapID{Platform: 1, Encoding: 0}, font.CmapID{Platform: 3, Encoding: 0}
	if f.symbolic {
		if tt.HasCmap(sym) {
			sample, _ := tt.SampleCode(sym)
			switch high := sample & 0xff00; high {
			case 0, 0xf000, 0xf100, 0xf200:
				return tt.Lookup(sym, high+code)
			}
			return 0
		}
		return tt.Lookup(mac, code)
	}
	name := f.gidNames[code]
	if name == "" || name == ".notdef" {
		return 0
	}
	gid := 0
	if u, ok := font.GlyphList(name); ok {
		gid = tt.Lookup(ms, uint32(u))
	}
	if gid == 0 && tt.HasCmap(mac) {
		if c, ok := font.MacRomanCode(name); ok {
			gid = tt.Lookup(mac, uint32(c))
		}
	}
	if gid == 0 {
		gid, _ = tt.PostGID(name)
	}
	return gid
}

func pdfNumber(o pdf.Object) (float64, bool) {
	switch v := o.(type) {
	case pdf.Int:
		return float64(v), true
	case pdf.Real:
		return float64(v), true
	}
	return 0, false
}

// fontProgram is an embedded font program, read once.
type fontProgram struct {
	key pdf.Name // FontFile, FontFile2 or FontFile3; "" when the font is not embedded
	cff *font.CFF
	tt  *font.TrueType // a TrueType program, or the tables of an OpenType one
}

// program reads the font program of a font descriptor: TrueType or CFF,
// or OpenType, whose CFF table holds the glyphs and hmtx the widths. A
// Type 1 program, or one that cannot be read, has neither.
func (s *pdfaScan) program(fd pdf.Dict) *fontProgram {
	r := s.c.r
	for _, key := range []pdf.Name{"FontFile", "FontFile2", "FontFile3"} {
		o := fd.Get(key)
		st, ok := r.Resolve(o).(*pdf.Stream)
		if !ok {
			continue
		}
		cs := s.contentState()
		id := keyOf(o)
		if p := cs.programs[id]; id != (objKey{}) && p != nil {
			return p
		}
		p := &fontProgram{key: key}
		if id != (objKey{}) {
			cs.programs[id] = p
		}
		if key == "FontFile" {
			return p // Type 1 programs are not read: no need to decode them
		}
		p.tt, p.cff = s.readProgram(st, key)
		return p
	}
	return &fontProgram{}
}

// programHint returns the size of a font program once decoded, to size
// the buffer: /Length1 for a TrueType program. Without it, the default
// estimate, four times the encoded length, is about twice what a deflated
// TrueType program takes.
func (s *pdfaScan) programHint(st *pdf.Stream, key pdf.Name) int {
	if key == "FontFile2" {
		if n, ok := pdf.AsInt(s.c.r.Resolve(st.Dict.Get("Length1"))); ok && n > 0 && n <= math.MaxInt32 {
			return int(n)
		}
	}
	return 0
}

// parseProgram reads a decoded FontFile2 or FontFile3 program: TrueType,
// CFF, or OpenType, whose CFF table holds the glyphs and hmtx the widths.
// Both are nil when it cannot be read.
func parseProgram(data []byte, key pdf.Name) (tt *font.TrueType, cff *font.CFF) {
	if key == "FontFile2" {
		tt, _ = font.ParseTrueType(data)
		return tt, nil
	}
	if c, err := font.ParseCFF(data); err == nil {
		return nil, c
	}
	if t, err := font.ParseTrueType(data); err == nil {
		if c, err := font.ParseCFF(t.Table("CFF ")); err == nil {
			return t, c
		}
	}
	return nil, nil
}

// type3Width returns the width wx a d0 or d1 operator at the start of the
// description of a Type 3 glyph sets, NaN without one.
func type3Width(data []byte) float64 {
	var nums []float64
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '%':
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0:
			i++
		case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(data) && (data[j] == '.' || (data[j] >= '0' && data[j] <= '9')) {
				j++
			}
			v, err := strconv.ParseFloat(string(data[i:j]), 64)
			if err != nil {
				return math.NaN()
			}
			nums = append(nums, v)
			i = j
		default:
			j := i
			for j < len(data) && data[j] > ' ' && !bytes.ContainsRune([]byte("()<>[]{}/%"), rune(data[j])) {
				j++
			}
			if op := string(data[i:j]); (op == "d0" && len(nums) == 2) || (op == "d1" && len(nums) == 6) {
				return nums[0]
			}
			return math.NaN()
		}
	}
	return math.NaN()
}

// cidWidths reads the W array of a CIDFont.
func cidWidths(r *pdf.Reader, o pdf.Object) map[uint32]float64 {
	m := map[uint32]float64{}
	w, _ := r.Resolve(o).(pdf.Array)
	for i := 0; i < len(w); {
		first, ok := pdf.AsInt(r.Resolve(w[i]))
		if !ok || first < 0 || i+1 >= len(w) {
			break
		}
		switch v := r.Resolve(w[i+1]).(type) {
		case pdf.Array:
			for k, x := range v {
				if n, ok := pdfNumber(r.Resolve(x)); ok && len(m) < 1<<16 {
					m[uint32(first)+uint32(k)] = n
				}
			}
			i += 2
		default:
			last, ok := pdf.AsInt(v)
			if !ok || i+2 >= len(w) {
				return m
			}
			n, _ := pdfNumber(r.Resolve(w[i+2]))
			for c := first; c <= last && len(m) < 1<<16; c++ {
				m[uint32(c)] = n
			}
			i += 3
		}
	}
	return m
}

// subsetFont reports whether a font name has a subset tag, six upper-case
// letters and a plus sign.
func subsetFont(name pdf.Name) bool {
	if len(name) < 7 || name[6] != '+' {
		return false
	}
	for _, c := range name[:6] {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// charSet checks that the CharSet of a subset Type 1 font lists the glyphs
// of its CFF program, no more and no fewer, .notdef aside (6.2.11.4.2).
func (s *pdfaScan) charSet(d, fd pdf.Dict, where string) {
	r := s.c.r
	set, ok := r.Resolve(fd.Get("CharSet")).(pdf.String)
	if !ok || !subsetFont(s.name(d, "BaseFont")) {
		return
	}
	p := s.program(fd)
	if p.cff == nil || p.cff.CID {
		return // a Type 1 program, which internal/font does not read
	}
	listed := map[string]bool{}
	for _, n := range bytes.Split(set, []byte("/")) {
		if n := string(bytes.TrimSpace(n)); n != "" && n != ".notdef" {
			listed[n] = true
		}
	}
	defined := map[string]bool{}
	for _, n := range p.cff.Names() {
		if n != ".notdef" {
			defined[n] = true
			if !listed[n] {
				s.report("PDF-72", where+" /FontDescriptor /CharSet", "the CharSet of the subset font does not list the glyph %s of its font program (6.2.11.4.2)", n)
				return
			}
		}
	}
	for n := range listed {
		if !defined[n] {
			s.report("PDF-72", where+" /FontDescriptor /CharSet", "the CharSet of the subset font lists the glyph %s, which its font program does not define (6.2.11.4.2)", n)
			return
		}
	}
}

// maxCIDSet is the number of bytes of a CIDSet veraPDF reads.
const maxCIDSet = 16384

// cidSet checks that the CIDSet of a subset CIDFont lists the CIDs its
// program defines, and only them, as veraPDF reads them (6.2.11.4.2): the
// CIDs of a CID-keyed CFF program; for a TrueType program, those below
// the length of the CIDToGIDMap stream whose glyph exists, or, for
// Identity, those below the number of horizontal metrics. The CID 0 is
// left aside.
func (s *pdfaScan) cidSet(cid, fd pdf.Dict, where string) {
	r := s.c.r
	st, ok := r.Resolve(fd.Get("CIDSet")).(*pdf.Stream)
	if !ok || !subsetFont(s.name(cid, "BaseFont")) {
		return
	}
	p := s.program(fd)
	var list []int
	var defines func(int) bool
	switch s.name(cid, "Subtype") {
	case "CIDFontType0":
		if p.cff == nil {
			return
		}
		if !p.cff.CID {
			list, defines = nil, func(int) bool { return false }
			break
		}
		list = p.cff.CIDs()
		defines = func(c int) bool {
			gid, ok := p.cff.GIDOfCID(c)
			return ok && gid != 0
		}
	case "CIDFontType2":
		if p.key != "FontFile2" || p.tt == nil {
			return
		}
		f := &glyphFont{tt: p.tt}
		n := p.tt.NumHMetrics
		if m, ok := r.Resolve(cid.Get("CIDToGIDMap")).(*pdf.Stream); ok {
			data, err := r.Decode(m)
			if err != nil {
				return
			}
			f.cidToGID, n = data, len(data)/2
		}
		defines = func(c int) bool {
			_, ok := f.cid2GID(uint32(c))
			return ok
		}
		for c := 0; c < n && c < 1<<16; c++ {
			if f.cidToGID == nil || defines(c) {
				list = append(list, c)
			}
		}
	default:
		return
	}
	data, err := r.Decode(st)
	if err != nil {
		return
	}
	data = data[:min(len(data), maxCIDSet)]
	set := func(c int) bool { return c/8 < len(data) && data[c/8]&(0x80>>(c%8)) != 0 }
	w := where + " /FontDescriptor /CIDSet"
	for _, c := range list {
		if c != 0 && !set(c) {
			s.report("PDF-72", w, "the CIDSet of the subset font does not list the CID %d, which its font program defines (6.2.11.4.2)", c)
			return
		}
	}
	for c := 1; c < 8*len(data); c++ {
		if set(c) && !defines(c) {
			s.report("PDF-72", w, "the CIDSet of the subset font lists the CID %d, which its font program does not define (6.2.11.4.2)", c)
			return
		}
	}
}
