// Copied from github.com/piglig/go-qr v1.1.0 (builder.go, util.go), MIT
// License, Copyright (c) 2023 piglig; see LICENSE.

package qr

import "math"

// builder is the mutable grid on which a symbol is laid out. Besides the
// modules, it marks the modules of the function patterns (isFunction), which
// the codewords and the mask leave alone.
type builder struct {
	version    int
	size       int
	level      Level
	modules    [][]bool // dark or light state of every module
	isFunction [][]bool // true for the modules of the function patterns
}

// newBuilder returns a blank grid for version ver at level l.
func newBuilder(ver int, l Level) *builder {
	size := ver*4 + 17
	b := &builder{version: ver, size: size, level: l}
	// One allocation per grid, the rows share it.
	b.modules = make([][]bool, size)
	b.isFunction = make([][]bool, size)
	modBacking := make([]bool, size*size)
	fnBacking := make([]bool, size*size)
	for i := range size {
		b.modules[i] = modBacking[i*size : (i+1)*size]
		b.isFunction[i] = fnBacking[i*size : (i+1)*size]
	}
	return b
}

// toCode freezes the grid into a Code with mask msk. The grid is handed
// over: the builder is not used afterwards.
func (q *builder) toCode(msk int) *Code {
	return &Code{
		version: q.version,
		size:    q.size,
		level:   q.level,
		mask:    msk,
		modules: q.modules,
	}
}

// chooseBestMask returns the mask pattern of lowest penalty. The patterns
// are scored on a copy of the grid, which keeps its unmasked modules; the
// format information of each mask is drawn before it is scored.
func (q *builder) chooseBestMask() int {
	minPenalty := math.MaxInt32
	best := 0
	scratch := make([][]bool, q.size)
	scratchBacking := make([]bool, q.size*q.size)
	for r := range q.size {
		scratch[r] = scratchBacking[r*q.size : (r+1)*q.size]
	}
	tmpl := getTemplate(q.version)
	for i := range 8 {
		q.drawFormatBits(i)
		q.writeMaskedGrid(scratch, tmpl.maskPatterns[i])
		penalty := q.penaltyScore(scratch, minPenalty)
		if penalty < minPenalty {
			best = i
			minPenalty = penalty
		}
	}
	return best
}

// setFunctionModule sets the module (x, y) and marks it as part of a
// function pattern.
func (q *builder) setFunctionModule(x, y int, isDark bool) {
	q.modules[y][x] = isDark
	q.isFunction[y][x] = true
}

// addEccAndInterleave splits the data codewords into blocks, appends to each
// block its Reed-Solomon codewords and interleaves the blocks, as the
// version and level require. data holds numDataCodewords codewords.
func (q *builder) addEccAndInterleave(data []byte) []byte {
	numBlocks := int(numErrorCorrectionBlocks[q.level][q.version])
	blockEccLen := int(eccCodeWordsPerBlock[q.level][q.version])
	rawCodewords := numRawDataModules(q.version) / 8
	numShortBlocks := numBlocks - rawCodewords%numBlocks
	shortBlockLen := rawCodewords / numBlocks

	blocks := make([][]byte, numBlocks)
	rsDiv := rsDivisor(blockEccLen)
	for i, k := 0, 0; i < numBlocks; i++ {
		datLen := shortBlockLen - blockEccLen
		if i >= numShortBlocks {
			datLen++
		}
		dat := data[k : k+datLen]
		k += datLen
		// Short blocks get a placeholder byte, skipped when interleaving.
		block := make([]byte, shortBlockLen+1)
		copy(block, dat)
		copy(block[len(block)-blockEccLen:], rsRemainder(dat, rsDiv))
		blocks[i] = block
	}

	res := make([]byte, rawCodewords)
	for i, k := 0, 0; i < len(blocks[0]); i++ {
		for j := range blocks {
			if i != shortBlockLen-blockEccLen || j >= numShortBlocks {
				res[k] = blocks[j][i]
				k++
			}
		}
	}
	return res
}

// drawCodewords fills the modules outside the function patterns with the
// bits of data, in the zigzag order of ISO/IEC 18004: columns of two modules
// from the right, upwards then downwards, skipping the vertical timing
// pattern. The remainder bits stay light.
func (q *builder) drawCodewords(data []byte) {
	i := 0
	for right := q.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := range q.size {
			for j := range 2 {
				x := right - j
				upward := ((right + 1) & 2) == 0
				y := vert
				if upward {
					y = q.size - 1 - vert
				}
				if !q.isFunction[y][x] && i < len(data)*8 {
					q.modules[y][x] = getBit(int(data[i>>3]), 7-(i&7))
					i++
				}
			}
		}
	}
}

// drawAlignmentPattern draws an alignment pattern centered on (x, y).
func (q *builder) drawAlignmentPattern(x, y int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			q.setFunctionModule(x+dx, y+dy, max(abs(dx), abs(dy)) != 1)
		}
	}
}

// drawFinderPattern draws a finder pattern centered on (x, y), with its
// separator.
func (q *builder) drawFinderPattern(x, y int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			dist := max(abs(dx), abs(dy))
			xx, yy := x+dx, y+dy
			if 0 <= xx && xx < q.size && 0 <= yy && yy < q.size {
				q.setFunctionModule(xx, yy, dist != 2 && dist != 4)
			}
		}
	}
}

// drawVersion draws the two copies of the version information, present from
// version 7 on.
func (q *builder) drawVersion() {
	if q.version < 7 {
		return
	}
	rem := q.version
	for range 12 {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	bits := q.version<<12 | rem
	for i := range 18 {
		bit := getBit(bits, i)
		a := q.size - 11 + i%3
		b := i / 3
		q.setFunctionModule(a, b, bit)
		q.setFunctionModule(b, a, bit)
	}
}

// drawFunctionPatterns draws the finder, timing and alignment patterns, the
// version information and a placeholder for the format information.
func (q *builder) drawFunctionPatterns() {
	for i := range q.size {
		q.setFunctionModule(6, i, i%2 == 0)
		q.setFunctionModule(i, 6, i%2 == 0)
	}

	q.drawFinderPattern(3, 3)
	q.drawFinderPattern(q.size-4, 3)
	q.drawFinderPattern(3, q.size-4)

	alignPatPos := q.alignmentPatternPositions()
	numAlign := len(alignPatPos)
	for i := range numAlign {
		for j := range numAlign {
			// Not on the finder patterns.
			if !(i == 0 && j == 0 || i == 0 && j == numAlign-1 || i == numAlign-1 && j == 0) {
				q.drawAlignmentPattern(alignPatPos[i], alignPatPos[j])
			}
		}
	}

	q.drawFormatBits(0)
	q.drawVersion()
}

// alignmentPatternPositions returns the coordinates of the centers of the
// alignment patterns, the same on both axes; none for version 1.
func (q *builder) alignmentPatternPositions() []int {
	if q.version == 1 {
		return nil
	}
	numAlign := q.version/7 + 2
	step := 26
	if q.version != 32 {
		step = (q.version*4 + numAlign*2 + 1) / (numAlign*2 - 2) * 2
	}
	res := make([]int, numAlign)
	res[0] = 6
	for i, pos := numAlign-1, q.size-7; i >= 1; i, pos = i-1, pos-step {
		res[i] = pos
	}
	return res
}

// drawFormatBits draws the two copies of the format information: the level
// and the mask msk, with their BCH code, masked by 0x5412.
func (q *builder) drawFormatBits(msk int) {
	data := q.level.formatBits()<<3 | msk
	rem := data
	for range 10 {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412

	// Around the top left finder pattern.
	for i := 0; i <= 5; i++ {
		q.setFunctionModule(8, i, getBit(bits, i))
	}
	q.setFunctionModule(8, 7, getBit(bits, 6))
	q.setFunctionModule(8, 8, getBit(bits, 7))
	q.setFunctionModule(7, 8, getBit(bits, 8))
	for i := 9; i < 15; i++ {
		q.setFunctionModule(14-i, 8, getBit(bits, i))
	}

	// Under the top right and beside the bottom left finder patterns.
	for i := range 8 {
		q.setFunctionModule(q.size-1-i, 8, getBit(bits, i))
	}
	for i := 8; i < 15; i++ {
		q.setFunctionModule(8, q.size-15+i, getBit(bits, i))
	}
	q.setFunctionModule(8, q.size-8, true) // the dark module
}

// abs returns the absolute value of x.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
