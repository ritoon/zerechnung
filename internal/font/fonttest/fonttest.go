// Package fonttest builds small font programs for the tests of the font
// checks.
package fonttest

import (
	"encoding/binary"
	"sort"
)

// SFNT builds a font program of the given tables.
func SFNT(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	out := binary.BigEndian.AppendUint32(nil, 0x00010000)
	out = binary.BigEndian.AppendUint16(out, uint16(len(tags)))
	out = append(out, make([]byte, 6)...)
	off := 12 + 16*len(tags)
	var data []byte
	for _, tag := range tags {
		out = append(out, tag...)
		out = append(out, 0, 0, 0, 0)
		out = binary.BigEndian.AppendUint32(out, uint32(off+len(data)))
		out = binary.BigEndian.AppendUint32(out, uint32(len(tables[tag])))
		data = append(data, tables[tag]...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return append(out, data...)
}

// TrueType returns a font of 3 glyphs, 1000 units per em, advances 500,
// 600 and 600 (two long metrics), whose (3,1) cmap maps A and B to glyphs 1
// and 2, and whose post table names the glyphs.
func TrueType() []byte {
	be16 := func(b []byte, v ...int) []byte {
		for _, x := range v {
			b = binary.BigEndian.AppendUint16(b, uint16(x))
		}
		return b
	}
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:], 1000)
	maxp := be16(binary.BigEndian.AppendUint32(nil, 0x00005000), 3)
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:], 2)
	hmtx := be16(nil, 500, 0, 600, 0, 0)
	// Format 4: one segment A-B (delta -64), and the final 0xFFFF one.
	sub := be16(nil, 4, 32, 0, 4, 4, 1, 0)
	sub = be16(sub, 'B', 0xffff, 0, 'A', 0xffff, -64&0xffff, 1, 0, 0)
	cmap := be16(nil, 0, 1, 3, 1)
	cmap = binary.BigEndian.AppendUint32(cmap, 12)
	cmap = append(cmap, sub...)
	post := binary.BigEndian.AppendUint32(nil, 0x00020000)
	post = append(post, make([]byte, 28)...)
	post = be16(post, 3, 0, 36, 258)
	post = append(post, 6, 'B', '.', 'a', 'l', 't', '2')
	return SFNT(map[string][]byte{"head": head, "maxp": maxp, "hhea": hhea, "hmtx": hmtx, "cmap": cmap, "post": post})
}

// cffIndex encodes an INDEX with 1-byte offsets.
func cffIndex(items ...[]byte) []byte {
	if len(items) == 0 {
		return []byte{0, 0}
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(len(items)))
	out = append(out, 1, 1)
	off := 1
	for _, it := range items {
		off += len(it)
		out = append(out, byte(off))
	}
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// cffInt encodes a DICT integer on 5 bytes.
func cffInt(v int) []byte {
	return binary.BigEndian.AppendUint32([]byte{29}, uint32(int32(v)))
}

// CFF returns a name-keyed CFF font of 3 glyphs, with the standard
// encoding: .notdef (defaultWidthX, 500), A (650) and a custom glyph A.sc
// (600, its width set in a subroutine). nominalWidthX is 550.
func CFF() []byte {
	charStrings := cffIndex(
		[]byte{14},                     // endchar
		[]byte{139 + 100, 139, 22, 14}, // 100 0 hmoveto endchar: nominalWidthX + 100
		[]byte{139 - 107, 10, 14},      // -107 callsubr (subroutine 0) endchar
	)
	subrs := cffIndex([]byte{139 + 50, 139 + 50, 139, 21, 11}) // 50 50 0 rmoveto return: nominalWidthX + 50
	private := append(append(cffInt(500), 20), append(cffInt(550), 21)...)
	private = append(private, append(cffInt(len(private)+6), 19)...)
	header := []byte{1, 0, 4, 1}
	names := cffIndex([]byte("Test"))
	strs := cffIndex([]byte("A.sc"))
	gsubrs := cffIndex()
	// Top DICT: charset, CharStrings and Private offsets, fixed size.
	top := func(charset, cs, privOff int) []byte {
		d := append(cffInt(charset), 15)
		d = append(append(d, cffInt(cs)...), 17)
		d = append(append(append(d, cffInt(len(private))...), cffInt(privOff)...), 18)
		return d
	}
	topLen := len(cffIndex(top(0, 0, 0)))
	base := len(header) + len(names) + topLen + len(strs) + len(gsubrs)
	charset := []byte{0, 0, 34, 1, 135} // format 0: SID 34 (A), 391 (A.sc)
	cs := base + len(charset)
	privOff := cs + len(charStrings)
	out := append([]byte{}, header...)
	out = append(out, names...)
	out = append(out, cffIndex(top(base, cs, privOff))...)
	out = append(out, strs...)
	out = append(out, gsubrs...)
	out = append(out, charset...)
	out = append(out, charStrings...)
	out = append(out, private...)
	return append(out, subrs...)
}
