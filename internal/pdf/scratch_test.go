package pdf

import (
	"bytes"
	"testing"
)

// TestDecodeScratch decodes the samples through one Scratch, released after
// every round: the buffers a round gives back, dirty with the data of
// another sample, must decode the next ones exactly as fresh buffers do.
// The hints cover an exact size, none (the default estimate) and one too
// small, which makes the decoder grow the lent buffer.
func TestDecodeScratch(t *testing.T) {
	r := &Reader{}
	var sc Scratch
	streams := samples()
	for round := range 4 {
		for i := range streams {
			data := streams[(i+round)%len(streams)]
			st := &Stream{Dict: Dict{{"Filter", Name("FlateDecode")}}, Raw: Deflate(data)}
			for _, hint := range []int{len(data), 0, len(data) / 3} {
				got, err := r.DecodeScratch(st, hint, &sc)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("round %d, sample %d (%d bytes), hint %d: %d bytes, %v", round, i, len(data), hint, len(got), err)
				}
			}
		}
		for _, p := range sc.bufs {
			if c := cap(*p); c < 1<<minPooledShift || c > 1<<maxPooledShift {
				t.Fatalf("kept a buffer of capacity %d", c)
			}
		}
		sc.Release()
		if len(sc.bufs) != 0 {
			t.Fatalf("%d buffers left after Release", len(sc.bufs))
		}
	}
}

// TestDecodeScratchUnfiltered checks that the data of an unfiltered stream,
// which aliases the input, is not lent: Release must not put the input in
// the pool.
func TestDecodeScratchUnfiltered(t *testing.T) {
	var sc Scratch
	raw := bytes.Repeat([]byte("q 1 0 0 1 0 0 cm Q\n"), 1000)
	got, err := (&Reader{}).DecodeScratch(&Stream{Raw: raw}, 0, &sc)
	if err != nil || &got[0] != &raw[0] {
		t.Fatalf("unfiltered data copied or failed: %v", err)
	}
	if len(sc.bufs) != 0 {
		t.Fatalf("the input was kept: %d buffers", len(sc.bufs))
	}
}

// TestScratchReuse checks that a released buffer is lent again: decoding
// the same stream after Release allocates no output buffer. One small
// allocation remains, for the filter list of the stream.
func TestScratchReuse(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes sync.Pool drop buffers at random")
	}
	data := bytes.Repeat([]byte("BT /F1 12 Tf 72 712 Td (Rechnung) Tj ET\n"), 2000)
	st := &Stream{Dict: Dict{{"Filter", Name("FlateDecode")}}, Raw: Deflate(data)}
	r := &Reader{}
	var sc Scratch
	if _, err := r.DecodeScratch(st, len(data), &sc); err != nil {
		t.Fatal(err)
	}
	sc.Release()
	if n := testing.AllocsPerRun(10, func() {
		if _, err := r.DecodeScratch(st, len(data), &sc); err != nil {
			t.Fatal(err)
		}
		sc.Release()
	}); n > 1 {
		t.Errorf("%v allocations per decoding, want at most 1", n)
	}
}
