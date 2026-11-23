package font

import (
	"encoding/binary"
	"math"
	"strconv"
)

// CFF is a Compact Font Format program (Adobe Technical Note 5176), name-
// keyed (Type1C) or CID-keyed (CIDFontType0C): its glyphs, their names or
// CIDs and their advance widths. It is read-only once parsed: its methods
// are safe for concurrent use.
type CFF struct {
	CID         bool // CID-keyed: the charset maps glyphs to CIDs
	charset     []uint16
	strings     [][]byte
	charStrings [][]byte
	gsubrs      [][]byte
	matrix      [6]float64
	privates    []cffPrivate // one, or one per Font DICT of a CID-keyed font
	fdSelect    []uint8      // Font DICT of each glyph, CID-keyed fonts
	encoding    map[byte]int // built-in encoding: code to glyph, name-keyed fonts
	byName      map[string]int
	byCID       map[int]int
	data        []byte
}

type cffPrivate struct {
	defaultWidthX, nominalWidthX float64
	subrs                        [][]byte
	matrix                       [6]float64 // FontMatrix of the Font DICT, zero when it has none
}

// cffDict is a DICT: operands by operator (two-byte operators are 1200+b).
type cffDict map[int][]float64

func (d cffDict) num(op int, def float64) float64 {
	if v := d[op]; len(v) > 0 {
		return v[0]
	}
	return def
}

// ParseCFF reads a CFF program: the first font of its FontSet.
func ParseCFF(data []byte) (*CFF, error) {
	if len(data) < 4 || data[0] != 1 {
		return nil, ErrFormat
	}
	p := &cffParser{b: data, pos: int(data[2])}
	if _, ok := p.index(); !ok { // Name INDEX
		return nil, ErrFormat
	}
	tops, ok := p.index()
	if !ok || len(tops) == 0 {
		return nil, ErrFormat
	}
	strs, ok := p.index()
	if !ok {
		return nil, ErrFormat
	}
	gsubrs, ok := p.index()
	if !ok {
		return nil, ErrFormat
	}
	top := parseDict(tops[0])
	c := &CFF{data: data, strings: strs, gsubrs: gsubrs, matrix: [6]float64{0.001, 0, 0, 0.001, 0, 0}}
	if m := top[1207]; len(m) == 6 {
		copy(c.matrix[:], m)
	}
	c.CID = top[1230] != nil
	cs := int(top.num(17, 0))
	if cs <= 0 || cs >= len(data) {
		return nil, ErrFormat
	}
	p.pos = cs
	if c.charStrings, ok = p.index(); !ok {
		return nil, ErrFormat
	}
	n := len(c.charStrings)
	c.readCharset(int(top.num(15, 0)), n)
	if c.CID {
		fdOff := int(top.num(1236, 0))
		p.pos = fdOff
		fds, ok := p.index()
		if !ok || fdOff == 0 {
			return nil, ErrFormat
		}
		for _, fd := range fds {
			fdict := parseDict(fd)
			priv := c.private(fdict)
			if m := fdict[1207]; len(m) == 6 {
				copy(priv.matrix[:], m)
			}
			c.privates = append(c.privates, priv)
		}
		c.readFDSelect(int(top.num(1237, 0)), n)
	} else {
		c.privates = []cffPrivate{c.private(top)}
		c.readEncoding(int(top.num(16, 0)))
	}
	return c, nil
}

// private reads the Private DICT a Top or Font DICT points to.
func (c *CFF) private(d cffDict) cffPrivate {
	var priv cffPrivate
	pr := d[18]
	if len(pr) != 2 {
		return priv
	}
	size, off := int(pr[0]), int(pr[1])
	data := c.raw()
	if off <= 0 || size < 0 || off+size > len(data) {
		return priv
	}
	pd := parseDict(data[off : off+size])
	priv.defaultWidthX = pd.num(20, 0)
	priv.nominalWidthX = pd.num(21, 0)
	if subrs := int(pd.num(19, 0)); subrs > 0 {
		p := &cffParser{b: data, pos: off + subrs}
		priv.subrs, _ = p.index()
	}
	return priv
}

// raw returns the whole program.
func (c *CFF) raw() []byte { return c.data }

func (c *CFF) readCharset(off, n int) {
	c.charset = make([]uint16, n)
	data := c.raw()
	switch off {
	case 0: // ISOAdobe: glyph i has SID i
		for i := range c.charset {
			if i < 229 {
				c.charset[i] = uint16(i)
			}
		}
		return
	case 1, 2: // Expert and ExpertSubset: not read
		return
	}
	if off >= len(data) {
		return
	}
	format := data[off]
	pos := off + 1
	be16 := func() (int, bool) {
		if pos+2 > len(data) {
			return 0, false
		}
		v := int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2
		return v, true
	}
	for gid := 1; gid < n; {
		switch format {
		case 0:
			sid, ok := be16()
			if !ok {
				return
			}
			c.charset[gid] = uint16(sid)
			gid++
		case 1, 2:
			first, ok := be16()
			if !ok {
				return
			}
			var left int
			if format == 1 {
				if pos >= len(data) {
					return
				}
				left = int(data[pos])
				pos++
			} else if left, ok = be16(); !ok {
				return
			}
			for k := 0; k <= left && gid < n; k++ {
				c.charset[gid] = uint16(first + k)
				gid++
			}
		default:
			return
		}
	}
}

func (c *CFF) readEncoding(off int) {
	c.encoding = map[byte]int{}
	data := c.raw()
	if off == 0 || off == 1 { // Standard or Expert encoding
		if off == 0 {
			for code, name := range standardEncoding {
				if name == "" {
					continue
				}
				if gid, ok := c.GID(name); ok {
					c.encoding[byte(code)] = gid
				}
			}
		}
		return
	}
	if off >= len(data) {
		return
	}
	format := data[off]
	pos := off + 1
	if pos >= len(data) {
		return
	}
	switch format & 0x7f {
	case 0:
		n := int(data[pos])
		pos++
		for gid := 1; gid <= n && pos < len(data); gid++ {
			c.encoding[data[pos]] = gid
			pos++
		}
	case 1:
		ranges := int(data[pos])
		pos++
		gid := 1
		for r := 0; r < ranges && pos+2 <= len(data); r++ {
			first, left := int(data[pos]), int(data[pos+1])
			pos += 2
			for k := 0; k <= left && first+k < 256; k++ {
				c.encoding[byte(first+k)] = gid
				gid++
			}
		}
	default:
		return
	}
	if format&0x80 != 0 && pos < len(data) {
		sups := int(data[pos])
		pos++
		for s := 0; s < sups && pos+3 <= len(data); s++ {
			code, sid := data[pos], int(binary.BigEndian.Uint16(data[pos+1:]))
			pos += 3
			for gid, x := range c.charset {
				if int(x) == sid {
					c.encoding[code] = gid
					break
				}
			}
		}
	}
}

func (c *CFF) readFDSelect(off, n int) {
	data := c.raw()
	if off <= 0 || off >= len(data) {
		return
	}
	c.fdSelect = make([]uint8, n)
	switch data[off] {
	case 0:
		for gid := 0; gid < n && off+1+gid < len(data); gid++ {
			c.fdSelect[gid] = data[off+1+gid]
		}
	case 3:
		pos := off + 1
		if pos+2 > len(data) {
			return
		}
		ranges := int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2
		for r := 0; r < ranges && pos+5 <= len(data); r++ {
			first, fd := int(binary.BigEndian.Uint16(data[pos:])), data[pos+2]
			next := int(binary.BigEndian.Uint16(data[pos+3:]))
			for gid := first; gid < next && gid < n; gid++ {
				c.fdSelect[gid] = fd
			}
			pos += 3
		}
	}
}

// NumGlyphs returns the number of glyphs.
func (c *CFF) NumGlyphs() int { return len(c.charStrings) }

// sidName returns the name of a string identifier.
func (c *CFF) sidName(sid int) string {
	if sid < len(cffStandardStrings) {
		return cffStandardStrings[sid]
	}
	if i := sid - len(cffStandardStrings); i < len(c.strings) {
		return string(c.strings[i])
	}
	return ""
}

// GlyphName returns the name of a glyph of a name-keyed font.
func (c *CFF) GlyphName(gid int) string {
	if c.CID || gid < 0 || gid >= len(c.charset) {
		return ""
	}
	return c.sidName(int(c.charset[gid]))
}

// GID returns the glyph of a name-keyed font that has the name.
func (c *CFF) GID(name string) (int, bool) {
	if c.byName == nil {
		c.byName = map[string]int{}
		for gid := len(c.charset) - 1; gid >= 0; gid-- {
			c.byName[c.GlyphName(gid)] = gid
		}
	}
	gid, ok := c.byName[name]
	return gid, ok
}

// GIDOfCID returns the glyph of a CID-keyed font that has the CID.
func (c *CFF) GIDOfCID(cid int) (int, bool) {
	if c.byCID == nil {
		c.byCID = map[int]int{}
		for gid := len(c.charset) - 1; gid >= 0; gid-- {
			c.byCID[int(c.charset[gid])] = gid
		}
	}
	gid, ok := c.byCID[cid]
	return gid, ok
}

// CIDs returns the CIDs of the glyphs of a CID-keyed font.
func (c *CFF) CIDs() []int {
	out := make([]int, len(c.charset))
	for i, x := range c.charset {
		out[i] = int(x)
	}
	return out
}

// Names returns the names of the glyphs of a name-keyed font.
func (c *CFF) Names() []string {
	out := make([]string, len(c.charset))
	for i := range c.charset {
		out[i] = c.GlyphName(i)
	}
	return out
}

// EncodingGID returns the glyph the built-in encoding of a name-keyed font
// maps code to.
func (c *CFF) EncodingGID(code byte) (int, bool) {
	gid, ok := c.encoding[code]
	return gid, ok
}

// FontMatrix returns the matrix of the Top DICT.
func (c *CFF) FontMatrix() [6]float64 { return c.matrix }

// GlyphMatrix returns the matrix that maps the glyph space of a glyph to
// text space: the FontMatrix of its Font DICT, for a CID-keyed font, times
// that of the Top DICT, or that of the Top DICT alone.
func (c *CFF) GlyphMatrix(gid int) [6]float64 {
	priv := c.privateOf(gid)
	if priv == nil || priv.matrix == ([6]float64{}) {
		return c.matrix
	}
	a, b := priv.matrix, c.matrix
	return [6]float64{
		a[0]*b[0] + a[1]*b[2], a[0]*b[1] + a[1]*b[3],
		a[2]*b[0] + a[3]*b[2], a[2]*b[1] + a[3]*b[3],
		a[4]*b[0] + a[5]*b[2] + b[4], a[4]*b[1] + a[5]*b[3] + b[5],
	}
}

func (c *CFF) privateOf(gid int) *cffPrivate {
	fd := 0
	if gid >= 0 && gid < len(c.fdSelect) {
		fd = int(c.fdSelect[gid])
	}
	if fd >= len(c.privates) {
		return nil
	}
	return &c.privates[fd]
}

// Width returns the advance width of a glyph in glyph space: the width
// operand of its charstring added to nominalWidthX, or defaultWidthX.
func (c *CFF) Width(gid int) (float64, bool) {
	if gid < 0 || gid >= len(c.charStrings) {
		return 0, false
	}
	priv := c.privateOf(gid)
	if priv == nil {
		return 0, false
	}
	w, has, ok := charStringWidth(c.charStrings[gid], priv.subrs, c.gsubrs)
	if !ok {
		return 0, false
	}
	if has {
		return priv.nominalWidthX + w, true
	}
	return priv.defaultWidthX, true
}

// subrBias returns the bias of the subroutine numbers of a Type 2
// charstring.
func subrBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	}
	return 32768
}

// charStringWidth reads a Type 2 charstring up to its first stack-clearing
// operator and returns the width operand, when it has one.
func charStringWidth(cs []byte, subrs, gsubrs [][]byte) (w float64, has, ok bool) {
	var stack []float64
	var run func(cs []byte, depth int) (done bool)
	clear := func(odd bool) {
		if odd && len(stack) > 0 {
			w, has = stack[0], true
		}
		ok = true
	}
	run = func(cs []byte, depth int) bool {
		if depth > 10 {
			return true
		}
		for i := 0; i < len(cs); {
			b := cs[i]
			switch {
			case b == 28 && i+2 < len(cs):
				stack = append(stack, float64(int16(binary.BigEndian.Uint16(cs[i+1:]))))
				i += 3
				continue
			case b >= 32 && b <= 246:
				stack = append(stack, float64(int(b)-139))
				i++
				continue
			case b >= 247 && b <= 250 && i+1 < len(cs):
				stack = append(stack, float64((int(b)-247)*256+int(cs[i+1])+108))
				i += 2
				continue
			case b >= 251 && b <= 254 && i+1 < len(cs):
				stack = append(stack, float64(-(int(b)-251)*256-int(cs[i+1])-108))
				i += 2
				continue
			case b == 255 && i+4 < len(cs):
				stack = append(stack, float64(int32(binary.BigEndian.Uint32(cs[i+1:])))/65536)
				i += 5
				continue
			}
			i++
			switch b {
			case 1, 3, 18, 23: // hstem, vstem, hstemhm, vstemhm
				clear(len(stack)%2 == 1)
				return true
			case 19, 20: // hintmask, cntrmask
				clear(len(stack)%2 == 1)
				return true
			case 21: // rmoveto
				clear(len(stack) > 2)
				return true
			case 4, 22: // vmoveto, hmoveto
				clear(len(stack) > 1)
				return true
			case 14: // endchar
				clear(len(stack) == 1 || len(stack) == 5)
				return true
			case 10, 29: // callsubr, callgsubr
				if len(stack) == 0 {
					return true
				}
				list := subrs
				if b == 29 {
					list = gsubrs
				}
				n := int(stack[len(stack)-1]) + subrBias(len(list))
				stack = stack[:len(stack)-1]
				if n < 0 || n >= len(list) {
					return true
				}
				if run(list[n], depth+1) {
					return true
				}
			case 11: // return
				return false
			default:
				// Any other operator comes after the first stack-clearing
				// one in a well-formed charstring.
				clear(false)
				return true
			}
		}
		return false
	}
	run(cs, 0)
	return w, has, ok
}

// cffParser reads the INDEX structures of a CFF program.
type cffParser struct {
	b   []byte
	pos int
}

// index reads an INDEX: its objects alias the program.
func (p *cffParser) index() ([][]byte, bool) {
	b := p.b
	if p.pos < 0 || p.pos+2 > len(b) {
		return nil, false
	}
	count := int(binary.BigEndian.Uint16(b[p.pos:]))
	p.pos += 2
	if count == 0 {
		return nil, true
	}
	if p.pos >= len(b) {
		return nil, false
	}
	offSize := int(b[p.pos])
	p.pos++
	if offSize < 1 || offSize > 4 || p.pos+(count+1)*offSize > len(b) {
		return nil, false
	}
	offs := make([]int, count+1)
	for i := range offs {
		v := 0
		for k := 0; k < offSize; k++ {
			v = v<<8 | int(b[p.pos+i*offSize+k])
		}
		offs[i] = v
	}
	base := p.pos + (count+1)*offSize - 1
	out := make([][]byte, count)
	for i := 0; i < count; i++ {
		s, e := base+offs[i], base+offs[i+1]
		if offs[i] < 1 || s > e || e > len(b) {
			return nil, false
		}
		out[i] = b[s:e]
	}
	p.pos = base + offs[count]
	return out, true
}

// parseDict reads a DICT.
func parseDict(b []byte) cffDict {
	d := cffDict{}
	var ops []float64
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c <= 21:
			op := int(c)
			i++
			if c == 12 {
				if i >= len(b) {
					return d
				}
				op = 1200 + int(b[i])
				i++
			}
			d[op] = ops
			ops = nil
		case c == 28 && i+2 < len(b):
			ops = append(ops, float64(int16(binary.BigEndian.Uint16(b[i+1:]))))
			i += 3
		case c == 29 && i+4 < len(b):
			ops = append(ops, float64(int32(binary.BigEndian.Uint32(b[i+1:]))))
			i += 5
		case c == 30:
			v, n := cffReal(b[i+1:])
			ops = append(ops, v)
			i += 1 + n
		case c >= 32 && c <= 246:
			ops = append(ops, float64(int(c)-139))
			i++
		case c >= 247 && c <= 250 && i+1 < len(b):
			ops = append(ops, float64((int(c)-247)*256+int(b[i+1])+108))
			i += 2
		case c >= 251 && c <= 254 && i+1 < len(b):
			ops = append(ops, float64(-(int(c)-251)*256-int(b[i+1])-108))
			i += 2
		default:
			i++
		}
	}
	return d
}

// cffReal reads a real number operand (nibbles), and returns it with the
// number of bytes read.
func cffReal(b []byte) (float64, int) {
	var s []byte
	for i, c := range b {
		for _, nib := range []byte{c >> 4, c & 0xf} {
			switch {
			case nib <= 9:
				s = append(s, '0'+nib)
			case nib == 0xa:
				s = append(s, '.')
			case nib == 0xb:
				s = append(s, 'E')
			case nib == 0xc:
				s = append(s, 'E', '-')
			case nib == 0xe:
				s = append(s, '-')
			case nib == 0xf:
				v, err := strconv.ParseFloat(string(s), 64)
				if err != nil || math.IsInf(v, 0) {
					v = 0
				}
				return v, i + 1
			}
		}
	}
	return 0, len(b)
}
