// Package font reads what the PDF/A checks need from embedded font
// programs: the glyphs a TrueType or CFF program defines, their names and
// advance widths. It does not render glyphs.
package font

import (
	"encoding/binary"
	"errors"
)

// ErrFormat is returned for data that is not a font program of the
// expected format.
var ErrFormat = errors.New("font: malformed font program")

// TrueType is a TrueType font program (ISO/IEC 14496-22): the tables of its
// glyphs, metrics and character maps.
type TrueType struct {
	UnitsPerEm  int
	NumGlyphs   int
	NumHMetrics int      // numberOfHMetrics of the hhea table
	advances    []uint16 // advance widths of hmtx, numberOfHMetrics entries
	cmaps       []CmapID
	subtables   map[CmapID][]byte
	post        []string // glyph names of the post table, by glyph index
	postFormat  uint32
	byName      map[string]int
	data        []byte
	tables      map[string][]byte
}

// CmapID identifies an encoding of a cmap table.
type CmapID struct{ Platform, Encoding uint16 }

// ParseTrueType reads a TrueType font program. A table it needs but cannot
// read leaves the font without what that table gives.
func ParseTrueType(data []byte) (*TrueType, error) {
	if len(data) < 12 {
		return nil, ErrFormat
	}
	switch binary.BigEndian.Uint32(data) {
	case 0x00010000, 0x74727565, 0x4f54544f: // 1.0, 'true', 'OTTO'
	default:
		return nil, ErrFormat
	}
	n := int(binary.BigEndian.Uint16(data[4:]))
	t := &TrueType{data: data, tables: map[string][]byte{}}
	for i := 0; i < n; i++ {
		at := 12 + 16*i
		if at+16 > len(data) {
			return nil, ErrFormat
		}
		off := uint64(binary.BigEndian.Uint32(data[at+8:]))
		size := uint64(binary.BigEndian.Uint32(data[at+12:]))
		if off > uint64(len(data)) {
			continue
		}
		size = min(size, uint64(len(data))-off)
		t.tables[string(data[at:at+4])] = data[off : off+size]
	}
	if head := t.tables["head"]; len(head) >= 20 {
		t.UnitsPerEm = int(binary.BigEndian.Uint16(head[18:]))
	}
	metrics := 0
	if hhea, hmtx := t.tables["hhea"], t.tables["hmtx"]; len(hhea) >= 36 {
		metrics = int(binary.BigEndian.Uint16(hhea[34:]))
		t.NumHMetrics = metrics
		for i := 0; i < metrics && 4*i+2 <= len(hmtx); i++ {
			t.advances = append(t.advances, binary.BigEndian.Uint16(hmtx[4*i:]))
		}
	}
	if maxp := t.tables["maxp"]; len(maxp) >= 6 {
		t.NumGlyphs = int(binary.BigEndian.Uint16(maxp[4:]))
	} else {
		t.NumGlyphs = metrics // as veraPDF counts the glyphs of a font without maxp
	}
	t.readCmap()
	t.readPost()
	return t, nil
}

// Table returns the data of a table, nil when the font has none.
func (t *TrueType) Table(tag string) []byte { return t.tables[tag] }

// Advance returns the advance width of a glyph, in font units. The glyphs
// after the last long metric of hmtx have its advance width.
func (t *TrueType) Advance(gid int) (int, bool) {
	if gid < 0 || len(t.advances) == 0 || (t.NumGlyphs > 0 && gid >= t.NumGlyphs) {
		return 0, false
	}
	if gid >= len(t.advances) {
		gid = len(t.advances) - 1
	}
	return int(t.advances[gid]), true
}

// Cmaps returns the encodings of the cmap table, in the order of the table.
func (t *TrueType) Cmaps() []CmapID { return t.cmaps }

// HasCmap reports whether the cmap table has the encoding id.
func (t *TrueType) HasCmap(id CmapID) bool {
	_, ok := t.subtables[id]
	return ok
}

// PostGID returns the glyph a post table of format 2 names name.
func (t *TrueType) PostGID(name string) (int, bool) {
	if t.postFormat != 0x00020000 {
		return 0, false
	}
	if t.byName == nil {
		t.byName = make(map[string]int, len(t.post))
		for gid := len(t.post) - 1; gid >= 0; gid-- {
			t.byName[t.post[gid]] = gid
		}
	}
	gid, ok := t.byName[name]
	return gid, ok
}

// GlyphName returns the name the post table gives a glyph, "" when it
// gives none.
func (t *TrueType) GlyphName(gid int) string {
	if gid < 0 || gid >= len(t.post) {
		return ""
	}
	return t.post[gid]
}

func (t *TrueType) readCmap() {
	cmap := t.tables["cmap"]
	if len(cmap) < 4 {
		return
	}
	n := int(binary.BigEndian.Uint16(cmap[2:]))
	t.subtables = map[CmapID][]byte{}
	for i := 0; i < n; i++ {
		rec := 4 + 8*i
		if rec+8 > len(cmap) {
			return
		}
		id := CmapID{binary.BigEndian.Uint16(cmap[rec:]), binary.BigEndian.Uint16(cmap[rec+2:])}
		off := int(binary.BigEndian.Uint32(cmap[rec+4:]))
		t.cmaps = append(t.cmaps, id)
		if off < 0 || off >= len(cmap) {
			continue
		}
		if _, dup := t.subtables[id]; !dup {
			t.subtables[id] = cmap[off:]
		}
	}
}

// CanLookup reports whether Lookup reads the encoding id: whether the cmap
// table has it, in format 0, 4, 6 or 12.
func (t *TrueType) CanLookup(id CmapID) bool {
	sub, ok := t.subtables[id]
	if !ok || len(sub) < 2 {
		return false
	}
	switch binary.BigEndian.Uint16(sub) {
	case 0, 4, 6, 12:
		return true
	}
	return false
}

// SampleCode returns the first code the encoding id of the cmap table maps:
// the start of its first segment or group, or its first code.
func (t *TrueType) SampleCode(id CmapID) (uint32, bool) {
	sub, ok := t.subtables[id]
	if !ok || len(sub) < 2 {
		return 0, false
	}
	switch binary.BigEndian.Uint16(sub) {
	case 0:
		return 0, true
	case 4:
		if len(sub) < 14 {
			return 0, false
		}
		segs := int(binary.BigEndian.Uint16(sub[6:])) / 2
		at := 16 + 2*segs
		if segs == 0 || at+2 > len(sub) {
			return 0, false
		}
		return uint32(binary.BigEndian.Uint16(sub[at:])), true
	case 6:
		if len(sub) < 8 {
			return 0, false
		}
		return uint32(binary.BigEndian.Uint16(sub[6:])), true
	case 12:
		if len(sub) < 20 {
			return 0, false
		}
		return binary.BigEndian.Uint32(sub[16:]), true
	}
	return 0, false
}

// Lookup returns the glyph the encoding id of the cmap table maps code to.
// Formats 0, 4, 6 and 12 are read; 0 is returned for an unmapped code.
func (t *TrueType) Lookup(id CmapID, code uint32) int {
	sub, ok := t.subtables[id]
	if !ok || len(sub) < 2 {
		return 0
	}
	be16 := func(at int) int {
		if at < 0 || at+2 > len(sub) {
			return 0
		}
		return int(binary.BigEndian.Uint16(sub[at:]))
	}
	be32 := func(at int) uint32 {
		if at < 0 || at+4 > len(sub) {
			return 0
		}
		return binary.BigEndian.Uint32(sub[at:])
	}
	switch be16(0) {
	case 0:
		if code < 256 && 6+int(code) < len(sub) {
			return int(sub[6+code])
		}
	case 4:
		if code > 0xffff {
			return 0
		}
		segs := be16(6) / 2
		for i := 0; i < segs; i++ {
			end := be16(14 + 2*i)
			if int(code) > end {
				continue
			}
			startAt := 16 + 2*segs + 2*i
			start := be16(startAt)
			if int(code) < start {
				return 0
			}
			delta := be16(startAt + 2*segs)
			rangeAt := startAt + 4*segs
			rangeOff := be16(rangeAt)
			if rangeOff == 0 {
				return (int(code) + delta) & 0xffff
			}
			g := be16(rangeAt + rangeOff + 2*(int(code)-start))
			if g == 0 {
				return 0
			}
			return (g + delta) & 0xffff
		}
	case 6:
		first, count := be16(6), be16(8)
		if int(code) >= first && int(code) < first+count {
			return be16(10 + 2*(int(code)-first))
		}
	case 12:
		groups := int(be32(12))
		for i := 0; i < groups && 16+12*i+12 <= len(sub); i++ {
			at := 16 + 12*i
			start, end, gid := be32(at), be32(at+4), be32(at+8)
			if code >= start && code <= end {
				return int(gid + code - start)
			}
		}
	}
	return 0
}

func (t *TrueType) readPost() {
	post := t.tables["post"]
	if len(post) < 32 {
		return
	}
	t.postFormat = binary.BigEndian.Uint32(post)
	switch t.postFormat {
	case 0x00010000:
		t.post = append([]string(nil), macGlyphNames[:]...)
	case 0x00020000:
		if len(post) < 34 {
			return
		}
		n := int(binary.BigEndian.Uint16(post[32:]))
		if 34+2*n > len(post) {
			return
		}
		var custom []string
		for at := 34 + 2*n; at < len(post); {
			l := int(post[at])
			if at+1+l > len(post) {
				break
			}
			custom = append(custom, string(post[at+1:at+1+l]))
			at += 1 + l
		}
		t.post = make([]string, n)
		for i := range t.post {
			idx := int(binary.BigEndian.Uint16(post[34+2*i:]))
			switch {
			case idx < len(macGlyphNames):
				t.post[i] = macGlyphNames[idx]
			case idx-len(macGlyphNames) < len(custom):
				t.post[i] = custom[idx-len(macGlyphNames)]
			}
		}
	}
}
