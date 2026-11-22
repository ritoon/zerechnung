// Copied from github.com/piglig/go-qr v1.1.0 (qr_segment.go, bit_buffer.go),
// MIT License, Copyright (c) 2023 piglig; see LICENSE.

package qr

import (
	"fmt"
	"strings"
)

// mode is the encoding mode of a segment.
type mode struct {
	bits          int    // mode indicator
	charCountBits [3]int // width of the character count, versions 1-9, 10-26 and 27-40
	maxChars      int    // most characters that version 40 at level L holds
}

// numCharCountBits returns the width of the character count indicator in a
// symbol of version ver.
func (m mode) numCharCountBits(ver int) int {
	return m.charCountBits[(ver+7)/17]
}

// The modes of ISO/IEC 18004 that the package encodes.
var (
	modeNumeric      = mode{0x1, [3]int{10, 12, 14}, 7089}
	modeAlphanumeric = mode{0x2, [3]int{9, 11, 13}, 4296}
	modeByte         = mode{0x4, [3]int{8, 16, 16}, 2953}
	modeECI          = mode{0x7, [3]int{0, 0, 0}, 0}
)

// alphanumericCharset lists the characters of the alphanumeric mode, each at
// the index that is its value.
const alphanumericCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// Segment is a run of data encoded in a single mode.
type Segment struct {
	mode     mode
	numChars int
	data     bitBuffer
}

// Bytes returns a segment that holds data in byte mode. ISO/IEC 18004 reads
// byte mode as ISO/IEC 8859-1 in the absence of an ECI; most decoders read
// UTF-8 as such.
func Bytes(data []byte) (Segment, error) {
	if len(data) > modeByte.maxChars {
		return Segment{}, fmt.Errorf("%w: %d bytes", ErrDataTooLong, len(data))
	}
	var bb bitBuffer
	for _, b := range data {
		bb.appendBits(int(b), 8)
	}
	return Segment{modeByte, len(data), bb}, nil
}

// Numeric returns a segment that holds digits in numeric mode.
func Numeric(digits string) (Segment, error) {
	if !isNumeric(digits) {
		return Segment{}, fmt.Errorf("%w: %q in numeric mode", ErrUnencodableChar, digits)
	}
	if len(digits) > modeNumeric.maxChars {
		return Segment{}, fmt.Errorf("%w: %d digits", ErrDataTooLong, len(digits))
	}
	var bb bitBuffer
	for i := 0; i < len(digits); {
		n := min(len(digits)-i, 3)
		num := 0
		for _, c := range []byte(digits[i : i+n]) {
			num = num*10 + int(c-'0')
		}
		bb.appendBits(num, n*3+1)
		i += n
	}
	return Segment{modeNumeric, len(digits), bb}, nil
}

// Alphanumeric returns a segment that holds text in alphanumeric mode:
// digits, upper case letters, space and $%*+-./:.
func Alphanumeric(text string) (Segment, error) {
	if !isAlphanumeric(text) {
		return Segment{}, fmt.Errorf("%w: %q in alphanumeric mode", ErrUnencodableChar, text)
	}
	if len(text) > modeAlphanumeric.maxChars {
		return Segment{}, fmt.Errorf("%w: %d characters", ErrDataTooLong, len(text))
	}
	var bb bitBuffer
	i := 0
	for ; i+2 <= len(text); i += 2 {
		v := strings.IndexByte(alphanumericCharset, text[i]) * 45
		v += strings.IndexByte(alphanumericCharset, text[i+1])
		bb.appendBits(v, 11)
	}
	if i < len(text) {
		bb.appendBits(strings.IndexByte(alphanumericCharset, text[i]), 6)
	}
	return Segment{modeAlphanumeric, len(text), bb}, nil
}

// ECI returns the segment that tells the decoder the character set of the
// segments that follow, by its ECI assignment number: 26 for UTF-8, 3 for
// ISO/IEC 8859-1.
func ECI(assignment int) (Segment, error) {
	var bb bitBuffer
	switch {
	case assignment < 0:
		return Segment{}, fmt.Errorf("%w: ECI assignment number %d", ErrInvalidArgument, assignment)
	case assignment < 1<<7:
		bb.appendBits(assignment, 8)
	case assignment < 1<<14:
		bb.appendBits(0b10, 2)
		bb.appendBits(assignment, 14)
	case assignment < 1e6:
		bb.appendBits(0b110, 3)
		bb.appendBits(assignment, 21)
	default:
		return Segment{}, fmt.Errorf("%w: ECI assignment number %d", ErrInvalidArgument, assignment)
	}
	return Segment{modeECI, 0, bb}, nil
}

// textSegments returns text as a single segment in the most compact mode
// that holds it whole: numeric, alphanumeric or byte (UTF-8). The empty text
// gives no segment.
func textSegments(text string) ([]Segment, error) {
	var seg Segment
	var err error
	switch {
	case text == "":
		return nil, nil
	case isNumeric(text):
		seg, err = Numeric(text)
	case isAlphanumeric(text):
		seg, err = Alphanumeric(text)
	default:
		seg, err = Bytes([]byte(text))
	}
	if err != nil {
		return nil, err
	}
	return []Segment{seg}, nil
}

// isNumeric reports whether s is made of ASCII digits only.
func isNumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// isAlphanumeric reports whether every byte of s is in alphanumericCharset.
func isAlphanumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(alphanumericCharset, s[i]) < 0 {
			return false
		}
	}
	return true
}

// totalBits returns the number of bits that segs take in a symbol of
// version ver, or -1 if a character count does not fit in its indicator.
func totalBits(segs []Segment, ver int) int {
	res := 0
	for _, seg := range segs {
		ccbits := seg.mode.numCharCountBits(ver)
		if seg.numChars >= 1<<ccbits {
			return -1
		}
		res += 4 + ccbits + seg.data.len()
	}
	return res
}

// bitBuffer is an append-only sequence of bits, packed eight to a byte, the
// most significant bit first.
type bitBuffer struct {
	data []byte
	n    int // number of bits
}

// len returns the number of bits in the buffer.
func (b *bitBuffer) len() int { return b.n }

// bit returns the i-th bit of the buffer.
func (b *bitBuffer) bit(i int) bool {
	return b.data[i>>3]&(0x80>>uint(i&7)) != 0
}

// appendBit appends one bit.
func (b *bitBuffer) appendBit(set bool) {
	if b.n>>3 >= len(b.data) {
		b.data = append(b.data, 0)
	}
	if set {
		b.data[b.n>>3] |= 0x80 >> uint(b.n&7)
	}
	b.n++
}

// appendBits appends the length low bits of val, the most significant
// first. The callers keep length between 0 and 31 and val below 1<<length.
func (b *bitBuffer) appendBits(val, length int) {
	for i := length - 1; i >= 0; i-- {
		b.appendBit((val>>uint(i))&1 != 0)
	}
}

// appendBuffer appends every bit of other.
func (b *bitBuffer) appendBuffer(other *bitBuffer) {
	for i := 0; i < other.n; i++ {
		b.appendBit(other.bit(i))
	}
}
