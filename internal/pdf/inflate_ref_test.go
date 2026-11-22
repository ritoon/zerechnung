package pdf

// Reference copy of the previous DEFLATE decoder, for the differential tests.

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"math/rand"
	"sync"
	"testing"
)

const refEntLink = 1 << 4

type refHuffman struct {
	table []uint32
	bits  uint32
	mask  uint64
}

// build constructs the decoding table for the given code lengths. As in
// zlib, incomplete codes are only accepted when they hold a single code of
// length 1, or no code at all (every lookup then fails).
func (h *refHuffman) build(lengths []uint8, primary uint32) error {
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
			p := refReverse(uint32(c), uint32(l)) & uint32(h.mask)
			subLen[p] = max(subLen[p], l)
		}
	}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		rev := refReverse(uint32(next[l]), uint32(l))
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
			h.table[p] = off<<16 | sb<<8 | refEntLink | primary
		}
		e := uint32(sym)<<16 | (uint32(l) - primary)
		for i := rev >> primary; i < 1<<sb; i += 1 << (uint32(l) - primary) {
			h.table[off+i] = e
		}
	}
	return nil
}

func refReverse(code, n uint32) uint32 {
	var r uint32
	for i := uint32(0); i < n; i++ {
		r = r<<1 | code&1
		code >>= 1
	}
	return r
}

// refDecoder holds the state of one decompression.
type refDecoder struct {
	in   []byte
	pos  int
	bits uint64
	n    uint32 // number of valid bits in bits
	over int    // bytes read past the end of the input (as zeros)
	out  []byte
	w    int
	lit  refHuffman
	dist refHuffman
	cl   refHuffman
}

var refDecoders sync.Pool

// refill makes at least 56 bits available (zeros past the end of input).
func (d *refDecoder) refill() {
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

func (d *refDecoder) take(n uint32) uint32 {
	v := uint32(d.bits & (1<<n - 1))
	d.bits >>= n
	d.n -= n
	return v
}

// sym decodes one symbol; at least 15 bits must be available.
func (d *refDecoder) sym(h *refHuffman) (uint32, bool) {
	e := h.table[d.bits&h.mask]
	if e&refEntLink != 0 {
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
func (d *refDecoder) grow(need int) bool {
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
func (d *refDecoder) truncated() bool {
	return d.over > 0 && int(d.n) < 8*d.over
}

func refFixedTables() {
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
	if err := refFixedLit.build(l[:], litBits); err != nil {
		panic("pdf: fixed literal/length code: " + err.Error())
	}
	var dl [32]uint8 // codes 30 and 31 complete the code but never occur
	for i := range dl {
		dl[i] = 5
	}
	if err := refFixedDist.build(dl[:], distBits); err != nil {
		panic("pdf: fixed distance code: " + err.Error())
	}
}

// refInflateRawIn decompresses raw deflate data into the output buffer lent
// by sc, when not nil. The refDecoder never reads output it has not written,
// so a lent buffer needs no clearing.
func refInflateRawIn(src []byte, hint int, sc *Scratch) ([]byte, error) {
	d, _ := refDecoders.Get().(*refDecoder)
	if d == nil {
		d = &refDecoder{}
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
	refDecoders.Put(d)
	return out, err
}

func (d *refDecoder) run() error {
	for {
		d.refill()
		final := d.take(1)
		switch d.take(2) {
		case 0:
			if err := d.stored(); err != nil {
				return err
			}
		case 1:
			refFixedOnce.Do(refFixedTables)
			if err := d.block(&refFixedLit, &refFixedDist); err != nil {
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

func (d *refDecoder) stored() error {
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

func (d *refDecoder) dynamic() error {
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

// block decodes the symbols of a compressed block. The refDecoder state is
// kept in local variables in the hot loop.
func (d *refDecoder) block(lit, dist *refHuffman) error {
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
		if e&refEntLink != 0 {
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
		if e&refEntLink != 0 {
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

var (
	refFixedOnce sync.Once
	refFixedLit  refHuffman
	refFixedDist refHuffman
)

// sameAsRef compares the decoder with the previous one on c: same output,
// same error, with and without scratch and for several hints.
func sameAsRef(t *testing.T, c []byte, sc *Scratch) {
	t.Helper()
	want, werr := refInflateRawIn(c, 0, nil)
	for _, hint := range []int{0, 1, len(want), len(want) + 1, 3 * len(want)} {
		wantH, werrH := refInflateRawIn(c, hint, nil)
		if !bytes.Equal(wantH, want) || werrH != werr {
			t.Fatalf("reference differs with hint %d", hint)
		}
		for _, s := range []*Scratch{nil, sc} {
			got, err := inflateRawIn(c, hint, s)
			if err != werr || !bytes.Equal(got, want) {
				t.Fatalf("hint %d scratch %v: got %d bytes, %v; want %d bytes, %v", hint, s != nil, len(got), err, len(want), werr)
			}
		}
	}
}

func TestInflateSameAsRef(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var sc Scratch
	defer sc.Release()
	var streams [][]byte
	for _, data := range samples() {
		for _, level := range []int{flate.HuffmanOnly, flate.NoCompression, 1, 6, 9} {
			streams = append(streams, deflateRaw(t, data, level, level == 6))
		}
	}
	for i, c := range streams {
		sameAsRef(t, c, &sc)
		for k := 0; k < 40 && len(c) > 0; k++ {
			m := append([]byte(nil), c...)
			switch k % 4 {
			case 0: // flip a bit
				j := rng.Intn(len(m))
				m[j] ^= 1 << rng.Intn(8)
			case 1: // truncate
				m = m[:rng.Intn(len(m))]
			case 2: // random byte
				m[rng.Intn(len(m))] = byte(rng.Intn(256))
			case 3: // garbage run
				j := rng.Intn(len(m))
				for e := min(len(m), j+1+rng.Intn(16)); j < e; j++ {
					m[j] = byte(rng.Intn(256))
				}
			}
			sameAsRef(t, m, &sc)
		}
		if t.Failed() {
			t.Fatalf("stream %d", i)
		}
	}
}

func FuzzInflateSameAsRef(f *testing.F) {
	for _, s := range samples()[:8] {
		f.Add(deflateRaw(f, s, 6, false))
		f.Add(deflateRaw(f, s, flate.HuffmanOnly, false))
	}
	var sc Scratch
	f.Fuzz(func(t *testing.T, c []byte) {
		sameAsRef(t, c, &sc)
		sc.Release()
	})
}
