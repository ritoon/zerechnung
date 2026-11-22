// Copied from github.com/piglig/go-qr v1.1.0 (qr_code.go, layout.go), MIT
// License, Copyright (c) 2023 piglig; see LICENSE.

package qr

import "errors"

// Errors returned by the encoder, to be tested with errors.Is.
var (
	// ErrInvalidArgument reports an error correction level, a version range
	// or an ECI assignment number out of range.
	ErrInvalidArgument = errors.New("qr: invalid argument")
	// ErrDataTooLong reports data that no allowed version can hold at the
	// requested error correction level.
	ErrDataTooLong = errors.New("qr: data too long")
	// ErrUnencodableChar reports a character that the requested mode cannot
	// represent.
	ErrUnencodableChar = errors.New("qr: unencodable character")
)

// Level is the error correction level of a symbol.
type Level int

// The four error correction levels.
const (
	Low      Level = iota // L: about 7% of the codewords can be restored.
	Medium                // M: about 15% of the codewords can be restored.
	Quartile              // Q: about 25% of the codewords can be restored.
	High                  // H: about 30% of the codewords can be restored.
)

// String returns the letter of the level: L, M, Q or H.
func (l Level) String() string {
	if l < Low || l > High {
		return "?"
	}
	return "LMQH"[l : l+1]
}

// formatBits returns the two bits that stand for the level in the format
// information.
func (l Level) formatBits() int {
	return [...]int{1, 0, 3, 2}[l]
}

// The range of the versions of QR Code Model 2.
const (
	MinVersion = 1
	MaxVersion = 40
)

// eccCodeWordsPerBlock is the number of error correction codewords of each
// block, by level and version (index 0 is padding).
var eccCodeWordsPerBlock = [4][41]int8{
	// Version: 0,  1,  2,  3,  4,  5,  6,  7,  8,  9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40
	{-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},  // Low
	{-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28}, // Medium
	{-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30}, // Quartile
	{-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30}, // High
}

// numErrorCorrectionBlocks is the number of error correction blocks, by
// level and version (index 0 is padding).
var numErrorCorrectionBlocks = [4][41]int8{
	// Version: 0, 1, 2, 3, 4, 5, 6, 7, 8, 9,10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40
	{-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},              // Low
	{-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},     // Medium
	{-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},  // Quartile
	{-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81}, // High
}

// numRawDataModules returns the number of modules of a symbol of version
// ver that hold codewords, data and error correction, once the function
// patterns are taken away.
func numRawDataModules(ver int) int {
	size := ver*4 + 17
	res := size * size
	res -= 8 * 8 * 3       // finder patterns and their separators
	res -= 15*2 + 1        // format information and the dark module
	res -= (size - 16) * 2 // timing patterns
	if ver >= 2 {
		numAlign := ver/7 + 2
		res -= (numAlign - 1) * (numAlign - 1) * 25 // alignment patterns
		res -= (numAlign - 2) * 2 * 20              // overlap with the timing patterns
		if ver >= 7 {
			res -= 6 * 3 * 2 // version information
		}
	}
	return res
}

// numDataCodewords returns the number of data codewords of a symbol of
// version ver at level l.
func numDataCodewords(ver int, l Level) int {
	return numRawDataModules(ver)/8 -
		int(eccCodeWordsPerBlock[l][ver])*int(numErrorCorrectionBlocks[l][ver])
}

// Code is a QR Code symbol, without its quiet zone. It is immutable.
type Code struct {
	version int
	size    int
	level   Level
	mask    int
	modules [][]bool // modules[y][x] is true for a dark module
}

// Version returns the version of the symbol, from 1 to 40.
func (c *Code) Version() int { return c.version }

// Size returns the number of modules on each side: 17 + 4 × version.
func (c *Code) Size() int { return c.size }

// Level returns the error correction level of the symbol.
func (c *Code) Level() Level { return c.level }

// Mask returns the mask pattern of the symbol, from 0 to 7.
func (c *Code) Mask() int { return c.mask }

// Module reports whether the module in column x and row y is dark, (0, 0)
// being the top left corner. Modules outside the symbol are light.
func (c *Code) Module(x, y int) bool {
	return 0 <= x && x < c.size && 0 <= y && y < c.size && c.modules[y][x]
}

// getBit returns the i-th bit of x, bit 0 being the least significant.
func getBit(x, i int) bool {
	return (x>>uint(i))&1 != 0
}
