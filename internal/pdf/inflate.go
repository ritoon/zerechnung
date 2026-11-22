package pdf

import (
	"encoding/binary"
	"errors"
	"sync"
)

// This file implements a DEFLATE decoder (RFC 1951) specialised for data
// held in memory: the whole output is the history window, input bits are
// refilled 8 bytes at a time, and Huffman codes are decoded through a
// two-level lookup table. It is several times faster than compress/flate,
// whose decoder reads the input one byte at a time through an interface.

var (
	errCorrupt   = errors.New("corrupt deflate data")
	errTruncated = errors.New("truncated deflate stream")
	errTooLarge  = errors.New("decoded stream exceeds the size limit")
)

// maxDecoded bounds the size of a decoded stream, as a protection against
// decompression bombs in untrusted PDFs. Deflate cannot expand data more than
// about 1032 times, which also bounds the initial allocation.
var maxDecoded = 256 << 20

// zlibHeader reports whether b starts with a valid zlib header without a
// preset dictionary.
func zlibHeader(b []byte) bool {
	return len(b) >= 2 && b[0]&0x0f == 8 && b[0]>>4 <= 7 && (uint16(b[0])<<8|uint16(b[1]))%31 == 0 && b[1]&0x20 == 0
}

// inflate decompresses zlib (or raw deflate) data. The Adler-32 checksum is
// not verified. hint, when positive, is the expected decoded size. On a
// truncated stream it returns the data decoded so far with an error.
func inflate(src []byte, hint int) ([]byte, error) { return inflateIn(src, hint, nil) }

// inflateIn is inflate with the output buffer lent by sc, when not nil.
func inflateIn(src []byte, hint int, sc *Scratch) ([]byte, error) {
	if zlibHeader(src) {
		out, err := inflateRawIn(src[2:], hint, sc)
		if err == nil || len(out) > 0 {
			return out, err
		}
	}
	return inflateRawIn(src, hint, sc)
}

// Huffman table entries: sym<<16 | flags | length. A link to a subtable is
// offset<<16 | subBits<<8 | entLink | primaryBits.
const (
	entLink    = 1 << 4
	litBits    = 10 // primary table bits for literal/length codes
	distBits   = 8  // primary table bits for distance codes
	clBits     = 7  // code length codes are at most 7 bits long
	maxCodeLen = 15
)

type huffman struct {
	table []uint32
	bits  uint32
	mask  uint64
}

// build constructs the decoding table for the given code lengths. As in
// zlib, incomplete codes are only accepted when they hold a single code of
// length 1, or no code at all (every lookup then fails).
func (h *huffman) build(lengths []uint8, primary uint32) error {
	var count [maxCodeLen + 1]int
	maxLen := 0
	for _, l := range lengths {
		count[l]++
		if int(l) > maxLen {
			maxLen = int(l)
		}
	}
	count[0] = 0
	h.bits = primary
	h.mask = 1<<primary - 1
	size := 1 << primary
	if cap(h.table) >= size {
		h.table = h.table[:size]
		clear(h.table)
	} else {
		h.table = make([]uint32, size, size*2)
	}
	if maxLen == 0 {
		return nil
	}
	// Canonical codes (RFC 1951, 3.2.2).
	var next [maxCodeLen + 1]int
	code, left := 0, 1
	for l := 1; l <= maxCodeLen; l++ {
		code = (code + count[l-1]) << 1
		next[l] = code
		left = left<<1 - count[l]
		if left < 0 {
			return errCorrupt // over-subscribed
		}
	}
	if left > 0 && !(maxLen == 1 && count[1] == 1) {
		return errCorrupt // incomplete
	}
	// Codes longer than the primary table go to subtables, one per primary
	// index, sized for the longest code sharing that index.
	var (
		subLen [1 << litBits]uint8
		subOff [1 << litBits]uint32
	)
	if maxLen > int(primary) {
		nc := next
		for _, l := range lengths {
			if l == 0 {
				continue
			}
			c := nc[l]
			nc[l]++
			if uint32(l) <= primary {
				continue
			}
			p := reverse(uint32(c), uint32(l)) & uint32(h.mask)
			subLen[p] = max(subLen[p], l)
		}
	}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		rev := reverse(uint32(next[l]), uint32(l))
		next[l]++
		if uint32(l) <= primary {
			e := uint32(sym)<<16 | uint32(l)
			for i := rev; i < uint32(size); i += 1 << l {
				h.table[i] = e
			}
			continue
		}
		p := rev & uint32(h.mask)
		sb := uint32(subLen[p]) - primary
		off := subOff[p]
		if off == 0 {
			off = uint32(len(h.table))
			h.table = append(h.table, make([]uint32, 1<<sb)...)
			subOff[p] = off
			h.table[p] = off<<16 | sb<<8 | entLink | primary
		}
		e := uint32(sym)<<16 | (uint32(l) - primary)
		for i := rev >> primary; i < 1<<sb; i += 1 << (uint32(l) - primary) {
			h.table[off+i] = e
		}
	}
	return nil
}

func reverse(code, n uint32) uint32 {
	var r uint32
	for i := uint32(0); i < n; i++ {
		r = r<<1 | code&1
		code >>= 1
	}
	return r
}

// decoder holds the state of one decompression.
type decoder struct {
	in   []byte
	pos  int
	bits uint64
	n    uint32 // number of valid bits in bits
	over int    // bytes read past the end of the input (as zeros)
	out  []byte
	w    int
	lit  huffman
	dist huffman
	cl   huffman
}

var decoders sync.Pool

// refill makes at least 56 bits available (zeros past the end of input).
func (d *decoder) refill() {
	if d.pos+8 <= len(d.in) {
		d.bits |= binary.LittleEndian.Uint64(d.in[d.pos:]) << d.n
		d.pos += int((63 - d.n) >> 3)
		d.n |= 56
		return
	}
	for d.n <= 56 {
		if d.pos < len(d.in) {
			d.bits |= uint64(d.in[d.pos]) << d.n
		} else {
			d.over++
		}
		d.pos++
		d.n += 8
	}
}

func (d *decoder) take(n uint32) uint32 {
	v := uint32(d.bits & (1<<n - 1))
	d.bits >>= n
	d.n -= n
	return v
}

// sym decodes one symbol; at least 15 bits must be available.
func (d *decoder) sym(h *huffman) (uint32, bool) {
	e := h.table[d.bits&h.mask]
	if e&entLink != 0 {
		d.bits >>= h.bits
		d.n -= h.bits
		sb := e >> 8 & 0xF
		e = h.table[e>>16+uint32(d.bits&(1<<sb-1))]
	}
	l := e & 0xF
	if l == 0 {
		return 0, false
	}
	d.bits >>= l
	d.n -= l
	return e >> 16, true
}

// grow makes room for need more bytes of output. It returns false when the
// output would exceed maxDecoded.
func (d *decoder) grow(need int) bool {
	if d.w+need <= len(d.out) {
		return true
	}
	if d.w+need > maxDecoded {
		return false
	}
	size := 2*len(d.out) + need
	if size < 1024 {
		size = 1024
	}
	if size > maxDecoded {
		size = maxDecoded
	}
	nb := make([]byte, size)
	copy(nb, d.out[:d.w])
	d.out = nb
	return true
}

// truncated reports whether bits past the end of the input were consumed.
func (d *decoder) truncated() bool {
	return d.over > 0 && int(d.n) < 8*d.over
}

var (
	lenBase  = [29]uint16{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
	lenExtra = [29]uint8{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
	distBase = [30]uint16{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
	distExtr = [30]uint8{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
	clOrder  = [19]uint8{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}
)

var (
	fixedOnce sync.Once
	fixedLit  huffman
	fixedDist huffman
)

func fixedTables() {
	var l [288]uint8
	for i := range l {
		switch {
		case i < 144:
			l[i] = 8
		case i < 256:
			l[i] = 9
		case i < 280:
			l[i] = 7
		default:
			l[i] = 8
		}
	}
	if err := fixedLit.build(l[:], litBits); err != nil {
		panic("pdf: fixed literal/length code: " + err.Error())
	}
	var dl [32]uint8 // codes 30 and 31 complete the code but never occur
	for i := range dl {
		dl[i] = 5
	}
	if err := fixedDist.build(dl[:], distBits); err != nil {
		panic("pdf: fixed distance code: " + err.Error())
	}
}

// inflateRaw decompresses raw deflate data.
func inflateRaw(src []byte, hint int) ([]byte, error) { return inflateRawIn(src, hint, nil) }

// inflateRawIn is inflateRaw with the output buffer lent by sc, when not
// nil. The decoder never reads output it has not written, so a lent buffer
// needs no clearing.
func inflateRawIn(src []byte, hint int, sc *Scratch) ([]byte, error) {
	d, _ := decoders.Get().(*decoder)
	if d == nil {
		d = &decoder{}
	}
	d.in, d.pos, d.bits, d.n, d.over, d.w = src, 0, 0, 0, 0, 0
	if hint <= 0 || hint > 1032*len(src)+1024 {
		// No size announced, or an announced size that deflate cannot
		// reach from this input: guess, and grow as needed.
		hint = 4*len(src) + 64
	}
	d.out = sc.get(min(hint, maxDecoded))
	err := d.run()
	if err != nil && d.truncated() {
		err = errTruncated
	}
	out := d.out[:d.w]
	sc.keep(d.out)
	d.in, d.out = nil, nil
	decoders.Put(d)
	return out, err
}

func (d *decoder) run() error {
	for {
		d.refill()
		final := d.take(1)
		switch d.take(2) {
		case 0:
			if err := d.stored(); err != nil {
				return err
			}
		case 1:
			fixedOnce.Do(fixedTables)
			if err := d.block(&fixedLit, &fixedDist); err != nil {
				return err
			}
		case 2:
			if err := d.dynamic(); err != nil {
				return err
			}
			if err := d.block(&d.lit, &d.dist); err != nil {
				return err
			}
		default:
			return errCorrupt
		}
		if d.truncated() {
			return errTruncated
		}
		if final == 1 {
			return nil
		}
	}
}

func (d *decoder) stored() error {
	// Drop the bits up to the byte boundary, then give back the whole
	// bytes still buffered (pos also counts the zeros read past the end).
	d.take(d.n & 7)
	d.pos -= int(d.n >> 3)
	d.bits, d.n, d.over = 0, 0, 0
	if d.pos+4 > len(d.in) {
		return errTruncated
	}
	n := int(d.in[d.pos]) | int(d.in[d.pos+1])<<8
	nn := int(d.in[d.pos+2]) | int(d.in[d.pos+3])<<8
	d.pos += 4
	if n != ^nn&0xffff {
		return errCorrupt
	}
	if d.pos+n > len(d.in) {
		if !d.grow(len(d.in) - d.pos) {
			return errTooLarge
		}
		d.w += copy(d.out[d.w:], d.in[d.pos:])
		d.pos = len(d.in)
		return errTruncated
	}
	if !d.grow(n) {
		return errTooLarge
	}
	d.w += copy(d.out[d.w:], d.in[d.pos:d.pos+n])
	d.pos += n
	return nil
}

func (d *decoder) dynamic() error {
	d.refill()
	nlit := int(d.take(5)) + 257
	ndist := int(d.take(5)) + 1
	ncl := int(d.take(4)) + 4
	if nlit > 286 || ndist > 30 {
		return errCorrupt
	}
	var cl [19]uint8
	for i := 0; i < ncl; i++ {
		if d.n < 3 {
			d.refill()
		}
		cl[clOrder[i]] = uint8(d.take(3))
	}
	if err := d.cl.build(cl[:], clBits); err != nil {
		return err
	}
	var lengths [286 + 30]uint8
	for i := 0; i < nlit+ndist; {
		d.refill()
		s, ok := d.sym(&d.cl)
		if !ok {
			return errCorrupt
		}
		switch {
		case s < 16:
			lengths[i] = uint8(s)
			i++
		case s == 16:
			if i == 0 {
				return errCorrupt
			}
			rep := 3 + int(d.take(2))
			if i+rep > nlit+ndist {
				return errCorrupt
			}
			for k := 0; k < rep; k++ {
				lengths[i] = lengths[i-1]
				i++
			}
		default:
			var rep int
			if s == 17 {
				rep = 3 + int(d.take(3))
			} else {
				rep = 11 + int(d.take(7))
			}
			if i+rep > nlit+ndist {
				return errCorrupt
			}
			for k := 0; k < rep; k++ {
				lengths[i] = 0
				i++
			}
		}
		if d.truncated() {
			return errTruncated
		}
	}
	if lengths[256] == 0 {
		return errCorrupt // no end-of-block code
	}
	if err := d.lit.build(lengths[:nlit], litBits); err != nil {
		return err
	}
	return d.dist.build(lengths[nlit:nlit+ndist], distBits)
}

// block decodes the symbols of a compressed block. The decoder state is
// kept in local variables in the hot loop.
func (d *decoder) block(lit, dist *huffman) error {
	var (
		bits, n = d.bits, d.n
		in, pos = d.in, d.pos
		out, w  = d.out, d.w
		lt, lm  = lit.table, lit.mask
		lb      = lit.bits
		dt, dm  = dist.table, dist.mask
		db      = dist.bits
	)
	save := func() { d.bits, d.n, d.pos, d.out, d.w = bits, n, pos, out, w }
	for {
		if n < 48 {
			if pos+8 <= len(in) {
				bits |= binary.LittleEndian.Uint64(in[pos:]) << n
				pos += int((63 - n) >> 3)
				n |= 56
			} else {
				save()
				d.refill()
				bits, n, pos = d.bits, d.n, d.pos
			}
		}
		e := lt[bits&lm]
		if e&entLink != 0 {
			bits >>= lb
			n -= lb
			e = lt[e>>16+uint32(bits&(1<<(e>>8&0xF)-1))]
		}
		l := e & 0xF
		if l == 0 {
			save()
			return errCorrupt
		}
		bits >>= l
		n -= l
		sym := e >> 16
		if d.over > 0 && int(n) < 8*d.over {
			save()
			return errTruncated
		}
		if sym < 256 {
			if w >= len(out) {
				d.w = w
				if !d.grow(1) {
					save()
					return errTooLarge
				}
				out = d.out
			}
			out[w] = byte(sym)
			w++
			continue
		}
		if sym == 256 {
			save()
			return nil
		}
		sym -= 257
		if sym >= 29 {
			save()
			return errCorrupt
		}
		eb := uint32(lenExtra[sym])
		length := int(lenBase[sym]) + int(bits&(1<<eb-1))
		bits >>= eb
		n -= eb
		e = dt[bits&dm]
		if e&entLink != 0 {
			bits >>= db
			n -= db
			e = dt[e>>16+uint32(bits&(1<<(e>>8&0xF)-1))]
		}
		l = e & 0xF
		ds := e >> 16
		if l == 0 || ds >= 30 {
			save()
			return errCorrupt
		}
		bits >>= l
		n -= l
		eb = uint32(distExtr[ds])
		dst := int(distBase[ds]) + int(bits&(1<<eb-1))
		bits >>= eb
		n -= eb
		if d.over > 0 && int(n) < 8*d.over {
			save()
			return errTruncated
		}
		if dst > w {
			save()
			return errCorrupt
		}
		if w+length > len(out) {
			d.w = w
			if !d.grow(length) {
				save()
				return errTooLarge
			}
			out = d.out
		}
		start := w - dst
		o := out[w : w+length]
		switch {
		case dst >= length:
			copy(o, out[start:start+length])
		case dst == 1:
			b := out[start]
			for i := range o {
				o[i] = b
			}
		default:
			src := out[start : start+length]
			for i := range o {
				o[i] = src[i]
			}
		}
		w += length
	}
}
