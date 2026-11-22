package qr

import (
	"fmt"
	"strings"
	"testing"
)

// This file reads a symbol back, without error correction, to check what the
// encoder wrote. It relies on the tables of ISO/IEC 18004 written out below,
// not on the code of the package, except for the block structure
// (eccCodeWordsPerBlock, numErrorCorrectionBlocks), which the capacity tests
// check against published capacities.

// formatInfo lists the 15 bits of the format information, masked, the most
// significant first, by level and mask.
var formatInfo = map[string][8]string{
	"L": {"111011111000100", "111001011110011", "111110110101010", "111100010011101", "110011000101111", "110001100011000", "110110001000001", "110100101110110"},
	"M": {"101010000010010", "101000100100101", "101111001111100", "101101101001011", "100010111111001", "100000011001110", "100111110010111", "100101010100000"},
	"Q": {"011010101011111", "011000001101000", "011111100110001", "011101000000110", "010010010110100", "010000110000011", "010111011011010", "010101111101101"},
	"H": {"001011010001001", "001001110111110", "001110011100111", "001100111010000", "000011101100010", "000001001010101", "000110100001100", "000100000111011"},
}

// versionInfo is the 18-bit version information, from version 7 on.
var versionInfo = [41]int{
	7: 0x07C94, 0x085BC, 0x09A99, 0x0A4D3, 0x0BBF6, 0x0C762, 0x0D847, 0x0E60D,
	0x0F928, 0x10B78, 0x1145D, 0x12A17, 0x13532, 0x149A6, 0x15683, 0x168C9,
	0x177EC, 0x18EC4, 0x191E1, 0x1AFAB, 0x1B08E, 0x1CC1A, 0x1D33F, 0x1ED75,
	0x1F250, 0x209D5, 0x216F0, 0x228BA, 0x2379F, 0x24B0B, 0x2542E, 0x26A64,
	0x27541, 0x28C69,
}

// alignmentCenters lists the row and column coordinates of the centers of
// the alignment patterns, by version.
var alignmentCenters = [41][]int{
	2: {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34},
	{6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}, {6, 30, 54}, {6, 32, 58}, {6, 34, 62},
	{6, 26, 46, 66}, {6, 26, 48, 70}, {6, 26, 50, 74}, {6, 30, 54, 78}, {6, 30, 56, 82}, {6, 30, 58, 86}, {6, 34, 62, 90},
	{6, 28, 50, 72, 94}, {6, 26, 50, 74, 98}, {6, 30, 54, 78, 102}, {6, 28, 54, 80, 106}, {6, 32, 58, 84, 110}, {6, 30, 58, 86, 114}, {6, 34, 62, 90, 118},
	{6, 26, 50, 74, 98, 122}, {6, 30, 54, 78, 102, 126}, {6, 26, 52, 78, 104, 130}, {6, 30, 56, 82, 108, 134}, {6, 34, 60, 86, 112, 138}, {6, 30, 58, 86, 114, 142}, {6, 34, 62, 90, 118, 146},
	{6, 30, 54, 78, 102, 126, 150}, {6, 24, 50, 76, 102, 128, 154}, {6, 28, 54, 80, 106, 132, 158}, {6, 32, 58, 84, 110, 136, 162}, {6, 26, 54, 82, 110, 138, 166}, {6, 30, 58, 86, 114, 142, 170},
}

// totalCodewords is the number of codewords of each version.
var totalCodewords = [41]int{
	0, 26, 44, 70, 100, 134, 172, 196, 242, 292, 346,
	404, 466, 532, 581, 655, 733, 815, 901, 991, 1085,
	1156, 1258, 1364, 1474, 1588, 1706, 1828, 1921, 2051, 2185,
	2323, 2465, 2611, 2761, 2876, 3034, 3196, 3362, 3532, 3706,
}

// remainderBits returns the number of modules left after the codewords.
func remainderBits(ver int) int {
	switch {
	case ver >= 2 && ver <= 6:
		return 7
	case ver >= 14 && ver <= 20, ver >= 28 && ver <= 34:
		return 3
	case ver >= 21 && ver <= 27:
		return 4
	}
	return 0
}

// The mode indicators.
const (
	indNumeric      = 1
	indAlphanumeric = 2
	indByte         = 4
	indECI          = 7
)

// charCountWidth returns the width of the character count indicator.
func charCountWidth(ind, ver int) int {
	r := 0
	if ver >= 27 {
		r = 2
	} else if ver >= 10 {
		r = 1
	}
	switch ind {
	case indNumeric:
		return [3]int{10, 12, 14}[r]
	case indAlphanumeric:
		return [3]int{9, 11, 13}[r]
	}
	return [3]int{8, 16, 16}[r]
}

// masked reports whether mask pattern m inverts the module in row i and
// column j, by the formulas of ISO/IEC 18004.
func masked(m, i, j int) bool {
	switch m {
	case 0:
		return (i+j)%2 == 0
	case 1:
		return i%2 == 0
	case 2:
		return j%3 == 0
	case 3:
		return (i+j)%3 == 0
	case 4:
		return (i/2+j/3)%2 == 0
	case 5:
		return (i*j)%2+(i*j)%3 == 0
	case 6:
		return ((i*j)%2+(i*j)%3)%2 == 0
	}
	return ((i+j)%2+(i*j)%3)%2 == 0
}

// readSegment is a segment read back from a symbol.
type readSegment struct {
	mode int    // mode indicator
	eci  int    // ECI assignment number, for an ECI segment
	text string // digits, characters or bytes
}

// symbol is what readSymbol finds in a Code.
type symbol struct {
	version   int
	level     string
	mask      int
	codewords []byte // as placed, interleaved
	segments  []readSegment
}

// payload returns the data of the segments, ECI segments left out.
func (s symbol) payload() string {
	var b strings.Builder
	for _, seg := range s.segments {
		b.WriteString(seg.text)
	}
	return b.String()
}

// readSymbol reads c back and checks its structure: function patterns,
// format and version information, Reed-Solomon codewords, padding.
func readSymbol(t testing.TB, c *Code) symbol {
	t.Helper()
	s, err := decodeSymbol(c)
	if err != nil {
		t.Fatalf("v%d: %v", c.Version(), err)
	}
	return s
}

func decodeSymbol(c *Code) (symbol, error) {
	var s symbol
	n := c.Size()
	if n < 21 || n > 177 || (n-17)%4 != 0 {
		return s, fmt.Errorf("size %d", n)
	}
	s.version = (n - 17) / 4
	if c.Version() != s.version {
		return s, fmt.Errorf("Version() = %d, size %d", c.Version(), n)
	}
	fn := make([][]bool, n)
	for i := range fn {
		fn[i] = make([]bool, n)
	}
	var errs []string
	expect := func(x, y int, dark bool, what string) {
		fn[y][x] = true
		if c.Module(x, y) != dark {
			errs = append(errs, fmt.Sprintf("%s at (%d,%d)", what, x, y))
		}
	}

	// Finder patterns with their separators.
	for _, o := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				x, y := o[0]+dx, o[1]+dy
				if x < 0 || y < 0 || x >= n || y >= n {
					continue
				}
				d := max(abs(dx-3), abs(dy-3))
				expect(x, y, d != 2 && d != 4, "finder pattern")
			}
		}
	}
	// Timing patterns.
	for i := 8; i < n-8; i++ {
		expect(i, 6, i%2 == 0, "timing pattern")
		expect(6, i, i%2 == 0, "timing pattern")
	}
	// Alignment patterns, except where the finder patterns are.
	centers := alignmentCenters[s.version]
	for a, cy := range centers {
		for b, cx := range centers {
			last := len(centers) - 1
			if a == 0 && b == 0 || a == 0 && b == last || a == last && b == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					expect(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1, "alignment pattern")
				}
			}
		}
	}
	expect(8, n-8, true, "dark module")

	// Format information: bit i of each copy, bit 0 the least significant.
	var pos1, pos2 [15][2]int
	for i := range 6 {
		pos1[i] = [2]int{8, i}
	}
	pos1[6], pos1[7], pos1[8] = [2]int{8, 7}, [2]int{8, 8}, [2]int{7, 8}
	for i := 9; i < 15; i++ {
		pos1[i] = [2]int{14 - i, 8}
	}
	for i := range 8 {
		pos2[i] = [2]int{n - 1 - i, 8}
	}
	for i := 8; i < 15; i++ {
		pos2[i] = [2]int{8, n - 15 + i}
	}
	readBits := func(pos [][2]int) int {
		v := 0
		for i, p := range pos {
			fn[p[1]][p[0]] = true
			if c.Module(p[0], p[1]) {
				v |= 1 << i
			}
		}
		return v
	}
	f1, f2 := readBits(pos1[:]), readBits(pos2[:])
	if f1 != f2 {
		return s, fmt.Errorf("format information copies differ: %015b, %015b", f1, f2)
	}
	format := fmt.Sprintf("%015b", f1)
	s.mask = -1
	for lvl, row := range formatInfo {
		for m, bits := range row {
			if bits == format {
				s.level, s.mask = lvl, m
			}
		}
	}
	if s.mask < 0 {
		return s, fmt.Errorf("format information %s is no valid code word", format)
	}
	if s.level != c.Level().String() || s.mask != c.Mask() {
		return s, fmt.Errorf("format information %s-%d, Level() %s, Mask() %d", s.level, s.mask, c.Level(), c.Mask())
	}

	// Version information.
	if s.version >= 7 {
		var tr, bl [18][2]int
		for i := range 18 {
			tr[i] = [2]int{n - 11 + i%3, i / 3}
			bl[i] = [2]int{i / 3, n - 11 + i%3}
		}
		v1, v2 := readBits(tr[:]), readBits(bl[:])
		if v1 != versionInfo[s.version] || v2 != versionInfo[s.version] {
			return s, fmt.Errorf("version information %05X and %05X, want %05X", v1, v2, versionInfo[s.version])
		}
	}
	if len(errs) > 0 {
		return s, fmt.Errorf("%d wrong function modules, first %s", len(errs), errs[0])
	}

	// Codewords, two columns at a time from the right, up then down.
	var bits []bool
	up := true
	for right := n - 1; right > 0; right -= 2 {
		if right == 6 {
			right--
		}
		for k := range n {
			y := k
			if up {
				y = n - 1 - k
			}
			for _, x := range []int{right, right - 1} {
				if !fn[y][x] {
					bits = append(bits, c.Module(x, y) != masked(s.mask, y, x))
				}
			}
		}
		up = !up
	}
	total := totalCodewords[s.version]
	if len(bits) != total*8+remainderBits(s.version) {
		return s, fmt.Errorf("%d data modules, want %d", len(bits), total*8+remainderBits(s.version))
	}
	for _, b := range bits[total*8:] {
		if b {
			return s, fmt.Errorf("remainder bit set")
		}
	}
	s.codewords = make([]byte, total)
	for i := range total * 8 {
		if bits[i] {
			s.codewords[i/8] |= 0x80 >> (i % 8)
		}
	}

	// Blocks: the short ones first, the long ones have one more data codeword.
	lvl := Level(strings.Index("LMQH", s.level))
	numBlocks := int(numErrorCorrectionBlocks[lvl][s.version])
	ecLen := int(eccCodeWordsPerBlock[lvl][s.version])
	shortData := total/numBlocks - ecLen
	numLong := total % numBlocks
	blocks := make([][]byte, numBlocks)
	k := 0
	for i := 0; i <= shortData; i++ {
		for j := range blocks {
			if i < shortData || j >= numBlocks-numLong {
				blocks[j] = append(blocks[j], s.codewords[k])
				k++
			}
		}
	}
	for range ecLen {
		for j := range blocks {
			blocks[j] = append(blocks[j], s.codewords[k])
			k++
		}
	}
	var data []byte
	for j, b := range blocks {
		if !rsValid(b, ecLen) {
			return s, fmt.Errorf("block %d: nonzero syndrome", j)
		}
		data = append(data, b[:len(b)-ecLen]...)
	}

	segs, err := parseSegments(data, s.version)
	s.segments = segs
	return s, err
}

// gfMulRef multiplies in GF(2^8) modulo x^8+x^4+x^3+x^2+1.
func gfMulRef(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1D
		}
		b >>= 1
	}
	return p
}

// rsValid reports whether the block, data then ecLen codewords, has zero
// syndromes: its polynomial vanishes at 2^0 ... 2^(ecLen-1).
func rsValid(block []byte, ecLen int) bool {
	root := byte(1)
	for range ecLen {
		var v byte
		for _, c := range block {
			v = gfMulRef(v, root) ^ c
		}
		if v != 0 {
			return false
		}
		root = gfMulRef(root, 2)
	}
	return true
}

// parseSegments parses the data codewords: segments, terminator, padding.
func parseSegments(data []byte, ver int) ([]readSegment, error) {
	pos, end := 0, len(data)*8
	read := func(w int) (int, error) {
		if pos+w > end {
			return 0, fmt.Errorf("bit stream ends at %d, %d more bits wanted", pos, w)
		}
		v := 0
		for range w {
			v = v<<1 | int(data[pos/8]>>(7-pos%8)&1)
			pos++
		}
		return v, nil
	}
	var segs []readSegment
	for end-pos >= 4 {
		ind, _ := read(4)
		if ind == 0 {
			break
		}
		seg := readSegment{mode: ind}
		var b []byte
		switch ind {
		case indECI:
			v, err := read(8)
			if err != nil {
				return nil, err
			}
			switch {
			case v&0x80 == 0:
				seg.eci = v
			case v&0xC0 == 0x80:
				lo, err := read(8)
				if err != nil {
					return nil, err
				}
				seg.eci = (v&0x3F)<<8 | lo
			default:
				lo, err := read(16)
				if err != nil {
					return nil, err
				}
				seg.eci = (v&0x1F)<<16 | lo
			}
			segs = append(segs, seg)
			continue
		case indNumeric, indAlphanumeric, indByte:
		default:
			return nil, fmt.Errorf("mode indicator %04b", ind)
		}
		count, err := read(charCountWidth(ind, ver))
		if err != nil {
			return nil, err
		}
		for count > 0 {
			switch ind {
			case indNumeric:
				d := min(count, 3)
				v, err := read(d*3 + 1)
				if err != nil {
					return nil, err
				}
				b = fmt.Appendf(b, "%0*d", d, v)
				count -= d
			case indAlphanumeric:
				if count >= 2 {
					v, err := read(11)
					if err != nil {
						return nil, err
					}
					b = append(b, alphanumericCharset[v/45], alphanumericCharset[v%45])
					count -= 2
				} else {
					v, err := read(6)
					if err != nil {
						return nil, err
					}
					b = append(b, alphanumericCharset[v])
					count--
				}
			default:
				v, err := read(8)
				if err != nil {
					return nil, err
				}
				b = append(b, byte(v))
				count--
			}
		}
		seg.text = string(b)
		segs = append(segs, seg)
	}
	// Zero bits up to the byte boundary, then 0xEC and 0x11 by turns.
	for ; pos%8 != 0 && pos < end; pos++ {
		if data[pos/8]>>(7-pos%8)&1 != 0 {
			return nil, fmt.Errorf("nonzero bit after the terminator")
		}
	}
	for i, pad := pos/8, byte(0xEC); i < len(data); i, pad = i+1, pad^0xEC^0x11 {
		if data[i] != pad {
			return nil, fmt.Errorf("pad codeword %d is %#x, want %#x", i, data[i], pad)
		}
	}
	return segs, nil
}
