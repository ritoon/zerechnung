package pdf

// Reference copy of the previous ScanContent, which boxed every operand in
// an Object, for the differential tests of ScanOperands.

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func scanContentRef(data []byte, fn func(op string, args []Object) error) error {
	p := &parser{b: data}
	var args []Object
	for {
		o, err := p.object()
		switch {
		case errors.Is(err, errEOF):
			return nil
		case err != nil:
			continue
		}
		kw, isOp := o.(keyword)
		if !isOp {
			if len(args) < maxOperands {
				args = append(args, o)
			}
			continue
		}
		op := string(kw)
		switch op {
		case "]", ">", ")", "{", "}":
			continue
		case "BI":
			d, err := p.inlineImage()
			if err != nil {
				return err
			}
			if err := fn("BI", []Object{d}); err != nil {
				return err
			}
		default:
			if err := fn(op, args); err != nil {
				return err
			}
		}
		args = args[:0]
	}
}

// scanned is what a scan of a content stream reports.
type scanned struct {
	ops  []string
	args [][]Object
	err  error
}

func scanRef(data []byte) scanned {
	var s scanned
	s.err = scanContentRef(data, func(op string, args []Object) error {
		s.ops = append(s.ops, op)
		s.args = append(s.args, append([]Object(nil), args...))
		return nil
	})
	return s
}

func scanNew(data []byte) scanned {
	var s scanned
	s.err = ScanContent(data, func(op string, args []Object) error {
		s.ops = append(s.ops, op)
		s.args = append(s.args, append([]Object(nil), args...))
		return nil
	})
	return s
}

// scanTyped converts the operands of ScanOperands during the call, since
// they are valid during the call only.
func scanTyped(data []byte) scanned {
	var s scanned
	s.err = ScanOperands(data, func(op string, args []Operand) error {
		s.ops = append(s.ops, op)
		objs := make([]Object, len(args))
		for i := range args {
			objs[i] = cloneObject(args[i].Object())
		}
		s.args = append(s.args, objs)
		return nil
	})
	return s
}

func cloneObject(o Object) Object {
	switch v := o.(type) {
	case String:
		return String(bytes.Clone(v))
	case Array:
		out := make(Array, len(v))
		for i := range v {
			out[i] = cloneObject(v[i])
		}
		return out
	}
	return o
}

// sameObject compares two objects; an empty string equals a nil one.
func sameObject(a, b Object) bool {
	switch a := a.(type) {
	case String:
		b, ok := b.(String)
		return ok && bytes.Equal(a, b)
	case Array:
		b, ok := b.(Array)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !sameObject(a[i], b[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func compareScans(t *testing.T, name string, data []byte) {
	t.Helper()
	want := scanRef(data)
	for _, got := range []struct {
		how string
		s   scanned
	}{{"ScanContent", scanNew(data)}, {"ScanOperands", scanTyped(data)}} {
		if !errors.Is(got.s.err, want.err) || len(got.s.ops) != len(want.ops) {
			t.Fatalf("%s: %s: %d operators, error %v; the reference reads %d, error %v", name, got.how, len(got.s.ops), got.s.err, len(want.ops), want.err)
		}
		for i := range want.ops {
			if got.s.ops[i] != want.ops[i] || len(got.s.args[i]) != len(want.args[i]) {
				t.Fatalf("%s: %s: operator %d is %q %v; the reference reads %q %v", name, got.how, i, got.s.ops[i], got.s.args[i], want.ops[i], want.args[i])
			}
			for j := range want.args[i] {
				if !sameObject(got.s.args[i][j], want.args[i][j]) {
					t.Fatalf("%s: %s: operand %d of operator %d (%s) is %#v; the reference reads %#v", name, got.how, j, i, want.ops[i], got.s.args[i][j], want.args[i][j])
				}
			}
		}
	}
}

var contentSamples = []string{
	"",
	"q 1 0 0 1 72 720 cm % comment\n/GS1 gs 0 0 1 rg BT /F1 12 Tf [(Fac) -20 (ture)] TJ ET\n",
	"/OC << /MCID 3 >> BDC EMC /P <</MCID 0 /Alt (x\\)y)>> BDC",
	"BI /W 2 /H 1 /CS /RGB /BPC 8 /F /AHx ID 00FF00EI0000FF> EI Q\nBI /W 1 /H 1 /IM true ID \x80 EI\n0.5 g ] 1 0 0 RG",
	"BT <0012003A> Tj <00 12 0> Tj <> Tj () Tj (a(b)c) Tj (\\101\\n\\\r\nx) Tj (line\r\nend) Tj ET",
	"[(A) 120 (B) -3.5 (C) 1e3 (D) .5 (E) -.25 (F) +7 (G) --1 (H) 1.2.3] TJ",
	"[[1 2 [3 [4]]] /N (s) <41>] 0 d 1 0 R 2 0 obj endobj 4294967296 0 R 3 65536 R 7 0 Rx",
	"true false null 12abc 4- /A#20B /#41 /Name#zz Tf BX /X 1 2 foo EX",
	"[1 2 endobj 3] [4 5 > 6] [7 stream] [ (unterminated",
	"[1 /x true null foo 2] TJ [ ] TJ [] TJ",
	"9999999999999999999999 99999999999999999999.5 -9223372036854775808 18446744073709551616 x",
	"(unterminated string",
	"<unterminated hex",
	"<< /K [1 2] /D << /E 3 >> >> BDC << unterminated",
	"BI /W 1 ID",
	"/F1 1 Tf /F1 1 Tf /F2 1 Tf /F1 1 Tf " + strings.Repeat("/N 1 Tf ", 300),
	strings.Repeat("[", 300) + "1" + strings.Repeat("]", 300) + " TJ 2 w",
	strings.Repeat("1 ", 100) + "scn",
	"{ } ) > ] q Q",
}

func TestScanOperandsDifferential(t *testing.T) {
	for i, s := range contentSamples {
		compareScans(t, "sample "+string(rune('a'+i%26)), []byte(s))
	}
	// The decoded streams of the test PDFs: content streams, but also
	// fonts, images and XML, which exercise the error paths.
	for _, name := range []string{"../../testdata/EN16931_Einfach.pdf", "../../testdata/host.pdf"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Open(data)
		if err != nil {
			t.Fatal(err)
		}
		size, _ := AsInt(r.Trailer().Get("Size"))
		for num := uint32(1); num < uint32(size); num++ {
			if st, ok := r.Object(num).(*Stream); ok {
				if dec, err := r.Decode(st); err == nil {
					compareScans(t, name, dec)
				}
			}
		}
	}
}

func FuzzScanOperands(f *testing.F) {
	for _, s := range contentSamples {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		compareScans(t, "fuzz", data)
	})
}

// TestScanOperandsAllocs checks that ScanOperands reads the operators of a
// content stream without allocating, once its arena has grown, but for the
// dictionaries of BDC, DP and BI operators: the Dict, and its values that an
// Object holds only once boxed (here, a string).
func TestScanOperandsAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates")
	}
	for _, tc := range []struct {
		line    string
		perLine float64
	}{
		{"BT /F1 12 Tf 1 0 0 1 72.5 700 Tm [(Fac\\(t\\)ure) -250 (n\\303\\251) 33.3 <00410042>] TJ (x) Tj ET q 0.5 0 0 RG 1 w 10 10 m 20.25 20 l S Q\n", 0},
		{"/P <</MCID 1 /Lang (fr)>> BDC EMC\n", 2},
	} {
		allocs := func(n int) float64 {
			data := []byte(strings.Repeat(tc.line, n))
			fn := func(string, []Operand) error { return nil }
			return testing.AllocsPerRun(20, func() {
				if err := ScanOperands(data, fn); err != nil {
					t.Fatal(err)
				}
			})
		}
		one, many := allocs(1), allocs(41)
		if perLine := (many - one) / 40; perLine > tc.perLine {
			t.Errorf("ScanOperands allocates %v times per line %q, want at most %v", perLine, tc.line, tc.perLine)
		}
		if one > 20 {
			t.Errorf("ScanOperands allocates %v times on a single line %q, want at most 20", one, tc.line)
		}
	}
}

// BenchmarkScanContent measures ScanOperands and ScanContent on the page
// content stream of testdata/EN16931_Einfach.pdf.
func BenchmarkScanContent(b *testing.B) {
	data := pageContent(b)
	b.Run("operands", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		fn := func(string, []Operand) error { return nil }
		for b.Loop() {
			_ = ScanOperands(data, fn)
		}
	})
	b.Run("objects", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		fn := func(string, []Object) error { return nil }
		for b.Loop() {
			_ = ScanContent(data, fn)
		}
	})
}

// pageContent returns the decoded content stream of the first page of
// testdata/EN16931_Einfach.pdf.
func pageContent(tb testing.TB) []byte {
	tb.Helper()
	data, err := os.ReadFile("../../testdata/EN16931_Einfach.pdf")
	if err != nil {
		tb.Fatal(err)
	}
	r, err := Open(data)
	if err != nil {
		tb.Fatal(err)
	}
	cat, _ := r.Dict(r.Trailer().Get("Root"))
	pages, _ := r.Dict(cat.Get("Pages"))
	kids, _ := r.Resolve(pages.Get("Kids")).(Array)
	page, _ := r.Dict(kids[0])
	st, ok := r.Resolve(page.Get("Contents")).(*Stream)
	if !ok {
		tb.Fatal("the first page has no content stream")
	}
	dec, err := r.Decode(st)
	if err != nil {
		tb.Fatal(err)
	}
	return dec
}
