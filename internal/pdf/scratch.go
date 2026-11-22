package pdf

import (
	"math/bits"
	"sync"
)

// Scratch lends the output buffers of the streams decoded by DecodeScratch
// and takes them back on Release, for later decodings to reuse: a check
// that decodes the same kind of streams document after document (font
// programs) then allocates nothing for them once the buffers are made.
//
// The data DecodeScratch returned must not be used after Release, nor kept
// by anything that outlives it. The zero value is ready to use; a Scratch
// must not be used by several goroutines at once.
type Scratch struct {
	bufs []*[]byte
	held *[]byte // the holder of the buffer get took from the pool, for keep to reuse
}

// Buffers are pooled by size class: class i holds buffers of capacity
// 1<<(i+minPooledShift). Smaller buffers cost little to allocate; larger
// ones are rare and would stay pinned in the pool.
const (
	minPooledShift = 12 // 4 KiB
	maxPooledShift = 24 // 16 MiB
)

var bufPools [maxPooledShift - minPooledShift + 1]sync.Pool

// get returns a buffer of length n, from the pool when its size is pooled.
// The buffer is not cleared. A nil Scratch allocates.
func (sc *Scratch) get(n int) []byte {
	if sc == nil || n <= 0 || n > 1<<maxPooledShift {
		return make([]byte, n)
	}
	shift := max(bits.Len(uint(n-1)), minPooledShift) // smallest power of two >= n
	if p, _ := bufPools[shift-minPooledShift].Get().(*[]byte); p != nil {
		sc.held = p
		return (*p)[:n]
	}
	return make([]byte, n, 1<<shift)
}

// keep records the buffer the last get returned, possibly grown since, to
// give back on Release.
func (sc *Scratch) keep(b []byte) {
	if sc == nil {
		return
	}
	p := sc.held
	sc.held = nil
	if cap(b) < 1<<minPooledShift || cap(b) > 1<<maxPooledShift {
		return
	}
	if p == nil {
		p = new([]byte)
	}
	*p = b
	sc.bufs = append(sc.bufs, p)
}

// Release gives the buffers back to the pool. The data the decodings
// returned is invalid from then on.
func (sc *Scratch) Release() {
	for i, p := range sc.bufs {
		// The largest class whose buffers b can stand for.
		shift := bits.Len(uint(cap(*p))) - 1
		*p = (*p)[:0]
		bufPools[shift-minPooledShift].Put(p)
		sc.bufs[i] = nil
	}
	sc.bufs = sc.bufs[:0]
}
