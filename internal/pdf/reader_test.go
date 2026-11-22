package pdf_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
)

// sample builds a document with a catalog, a page tree, an info dictionary
// and a compressed stream.
func sample() (*pdftest.Builder, []byte) {
	content := []byte("BT /F1 12 Tf 72 712 Td (Hello) Tj ET")
	b := &pdftest.Builder{Trailer: "/Root 1 0 R /Info 3 0 R /ID [<0102> <0102>]"}
	b.Add("<< /Type /Catalog /Pages 2 0 R >>")
	b.Add("<< /Type /Pages /Kids [4 0 R] /Count 1 >>")
	b.Add("<< /Producer (pdftest) /Title (\\376\\377\\000T\\000e\\000s\\000t) >>")
	b.Add("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 5 0 R >>")
	b.AddStream("<< /Filter /FlateDecode >>", pdftest.Flate(content))
	return b, content
}

func open(t *testing.T, data []byte) *pdf.Reader {
	t.Helper()
	r, err := pdf.Open(data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return r
}

// checkSample reads every object of the sample document.
func checkSample(t *testing.T, r *pdf.Reader, content []byte) {
	t.Helper()
	cat := r.Catalog()
	if typ, _ := cat.Name("Type"); typ != "Catalog" {
		t.Fatalf("catalog type %q", typ)
	}
	pages, ok := r.Dict(cat.Get("Pages"))
	if !ok {
		t.Fatal("no page tree")
	}
	if n, _ := pages.Int("Count"); n != 1 {
		t.Fatalf("Count %d", n)
	}
	kids := r.Resolve(pages.Get("Kids")).(pdf.Array)
	page, ok := r.Dict(kids[0])
	if !ok {
		t.Fatal("no page")
	}
	st, ok := r.Resolve(page.Get("Contents")).(*pdf.Stream)
	if !ok {
		t.Fatalf("contents: %T", r.Resolve(page.Get("Contents")))
	}
	got, err := r.Decode(st)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("contents %q, %v", got, err)
	}
	info, ok := r.Dict(r.Trailer().Get("Info"))
	if !ok {
		t.Fatal("no info")
	}
	if s := pdf.Text(r.Resolve(info.Get("Title")).(pdf.String)); s != "Test" {
		t.Fatalf("title %q", s)
	}
}

func TestLayouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     pdftest.Options
		stream   bool
		repaired bool
	}{
		{"classic", pdftest.Options{}, false, false},
		{"xref stream", pdftest.Options{Layout: pdftest.XrefStream}, true, false},
		{"xref stream uncompressed", pdftest.Options{Layout: pdftest.XrefStream, Uncompressed: true}, true, false},
		{"xref stream + predictor", pdftest.Options{Layout: pdftest.XrefStream, Predictor: true}, true, false},
		{"xref stream + object stream", pdftest.Options{Layout: pdftest.XrefStream, ObjStm: true, Predictor: true}, true, false},
		{"hybrid", pdftest.Options{Layout: pdftest.Hybrid, ObjStm: true}, false, false},
		{"hybrid with free entries", pdftest.Options{Layout: pdftest.Hybrid, ObjStm: true, HybridFree: true}, false, false},
		{"no xref", pdftest.Options{Layout: pdftest.NoXref}, false, true},
		{"shifted offsets", pdftest.Options{Shift: 7}, false, true},
		{"shifted xref stream", pdftest.Options{Layout: pdftest.XrefStream, ObjStm: true, Shift: 3}, false, true},
		// Offsets relative to the header instead of the file start are
		// accepted, and reported as a repair.
		{"garbage before header", pdftest.Options{Garbage: "junk\r\n"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, content := sample()
			r := open(t, b.Bytes(tc.opts))
			checkSample(t, r, content)
			if !tc.repaired && r.XrefStream() != tc.stream {
				t.Errorf("XrefStream() = %v", r.XrefStream())
			}
			if r.Repaired() != tc.repaired {
				t.Errorf("Repaired() = %v", r.Repaired())
			}
		})
	}
}

func TestVersion(t *testing.T) {
	b, _ := sample()
	b.Version = "1.4"
	if v := open(t, b.Bytes(pdftest.Options{})).Version(); v != "1.4" {
		t.Fatalf("version %q", v)
	}
}

func TestNotPDF(t *testing.T) {
	for _, data := range []string{"", "hello", "%PDF-1.7\n"} {
		if _, err := pdf.Open([]byte(data)); err == nil {
			t.Errorf("%q: no error", data)
		}
	}
}

func TestEncrypted(t *testing.T) {
	b, _ := sample()
	enc := b.Add("<< /Filter /Standard /V 2 /R 3 /O <00> /U <00> /P -4 >>")
	b.Trailer += fmt.Sprintf(" /Encrypt %d 0 R", enc)
	if _, err := pdf.Open(b.Bytes(pdftest.Options{})); !errors.Is(err, pdf.ErrEncrypted) {
		t.Fatalf("err = %v", err)
	}
}

func TestBadLength(t *testing.T) {
	// A wrong /Length is corrected by looking for endstream.
	for _, length := range []string{"5", "9999", "7 0 R"} {
		b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
		b.Add("<< /Type /Catalog /S 2 0 R >>")
		b.Add("null")
		data := b.Bytes(pdftest.Options{Layout: pdftest.NoXref})
		obj := "2 0 obj\n<< /Length " + length + " >>\nstream\nhello world\nendstream\nendobj\n"
		data = bytes.Replace(data, []byte("2 0 obj\nnull\nendobj\n"), []byte(obj), 1)
		r := open(t, data)
		st, ok := r.Resolve(r.Catalog().Get("S")).(*pdf.Stream)
		if !ok {
			t.Fatalf("length %s: no stream", length)
		}
		if string(st.Raw) != "hello world" {
			t.Errorf("length %s: raw %q", length, st.Raw)
		}
	}
}

func TestIncrementalUpdate(t *testing.T) {
	for _, layout := range []pdftest.Layout{pdftest.Classic, pdftest.XrefStream, pdftest.Hybrid} {
		b, content := sample()
		orig := b.Bytes(pdftest.Options{Layout: layout, ObjStm: true})
		r := open(t, orig)

		// Two successive updates form a /Prev chain.
		u := pdf.NewUpdate(r)
		ref := u.Alloc()
		u.Put(ref, pdf.Dict{{Key: "Note", Val: pdf.EncodeText("first")}})
		cat := r.Catalog().Clone()
		cat.Set("Note", ref)
		u.Put(pdf.Ref{Num: 1}, cat)
		out := u.Bytes(pdf.Dict{{Key: "Root", Val: pdf.Ref{Num: 1}}, {Key: "Info", Val: pdf.Ref{Num: 3}}})
		if !bytes.HasPrefix(out, orig) {
			t.Fatal("original bytes changed")
		}
		r2 := open(t, out)
		if r2.XrefStream() != (layout == pdftest.XrefStream) {
			t.Errorf("layout %d: update xref stream = %v", layout, r2.XrefStream())
		}
		u2 := pdf.NewUpdate(r2)
		ref2 := u2.Alloc()
		if ref2.Num <= ref.Num {
			t.Fatalf("Alloc reused %v", ref2)
		}
		u2.PutStream(ref2, pdf.Dict{{Key: "Type", Val: pdf.Name("Test")}}, []byte("data"))
		cat2 := r2.Catalog().Clone()
		cat2.Set("Data", ref2)
		u2.Put(pdf.Ref{Num: 1}, cat2)
		out2 := u2.Bytes(pdf.Dict{{Key: "Root", Val: pdf.Ref{Num: 1}}, {Key: "Info", Val: pdf.Ref{Num: 3}}})

		r3 := open(t, out2)
		if r3.Repaired() {
			t.Errorf("layout %d: repaired", layout)
		}
		checkSample(t, r3, content)
		note, _ := r3.Dict(r3.Catalog().Get("Note"))
		if s, _ := note.Get("Note").(pdf.String); pdf.Text(s) != "first" {
			t.Errorf("layout %d: note %q", layout, s)
		}
		st, _ := r3.Resolve(r3.Catalog().Get("Data")).(*pdf.Stream)
		if st == nil || string(st.Raw) != "data" {
			t.Errorf("layout %d: stream %v", layout, st)
		}
		if _, ok := r3.Trailer().Int("Prev"); !ok {
			t.Errorf("layout %d: no /Prev", layout)
		}
	}
}

func TestUpdateRepaired(t *testing.T) {
	// The update of a damaged file indexes every object, without /Prev.
	b, content := sample()
	r := open(t, b.Bytes(pdftest.Options{Shift: 11}))
	if !r.Repaired() {
		t.Fatal("not repaired")
	}
	u := pdf.NewUpdate(r)
	ref := u.Alloc()
	u.Put(ref, pdf.Int(42))
	out := u.Bytes(pdf.Dict{{Key: "Root", Val: pdf.Ref{Num: 1}}, {Key: "Info", Val: pdf.Ref{Num: 3}}})
	r2 := open(t, out)
	if r2.Repaired() {
		t.Error("update still needs repair")
	}
	if _, ok := r2.Trailer().Int("Prev"); ok {
		t.Error("unexpected /Prev")
	}
	checkSample(t, r2, content)
	if v := r2.Resolve(ref); v != pdf.Int(42) {
		t.Errorf("new object %v", v)
	}
}

func TestFilters(t *testing.T) {
	text := []byte("Factur-X <rsm:CrossIndustryInvoice> \x00\xff 0123456789")
	png := func(data []byte, cols int) []byte {
		// PNG Sub predictor on rows of cols bytes.
		var out []byte
		for i := 0; i < len(data); i += cols {
			row := data[i:min(i+cols, len(data))]
			out = append(out, 1)
			prev := byte(0)
			for _, c := range row {
				out = append(out, c-prev)
				prev = c
			}
		}
		return out
	}
	rows := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 4)
	for _, tc := range []struct {
		name string
		dict string
		raw  []byte
		want []byte
	}{
		{"none", "<< >>", text, text},
		{"flate", "<< /Filter /FlateDecode >>", pdftest.Flate(text), text},
		{"flate abbreviated", "<< /Filter /Fl >>", pdftest.Flate(text), text},
		{"hex", "<< /Filter /ASCIIHexDecode >>", []byte("48 65 6c6C\n6f7>"), []byte("Hellop")},
		{"ascii85", "<< /Filter /ASCII85Decode >>", []byte("87cURD]i,\"Ebo80~>"), []byte("Hello World!")},
		{"ascii85 z", "<< /Filter /A85 >>", []byte("z87cURDZ~>"), []byte("\x00\x00\x00\x00Hello")},
		{"chain", "<< /Filter [/ASCIIHexDecode /FlateDecode] >>", []byte(fmt.Sprintf("%x>", pdftest.Flate(text))), text},
		{"png predictor", "<< /Filter /FlateDecode /DecodeParms << /Predictor 11 /Columns 8 >> >>", pdftest.Flate(png(rows, 8)), rows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
			b.Add("<< /Type /Catalog /S 2 0 R >>")
			b.AddStream(tc.dict, tc.raw)
			r := open(t, b.Bytes(pdftest.Options{}))
			got, err := r.Decode(r.Resolve(r.Catalog().Get("S")).(*pdf.Stream))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnsupportedFilter(t *testing.T) {
	b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
	b.Add("<< /Type /Catalog /S 2 0 R >>")
	b.AddStream("<< /Filter /DCTDecode >>", []byte{0xff, 0xd8})
	r := open(t, b.Bytes(pdftest.Options{}))
	_, err := r.Decode(r.Resolve(r.Catalog().Get("S")).(*pdf.Stream))
	var ufe *pdf.UnsupportedFilterError
	if !errors.As(err, &ufe) || ufe.Filter != "DCTDecode" {
		t.Fatalf("err = %v", err)
	}
}

func TestObjects(t *testing.T) {
	b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
	b.Add(`<< /Type /Catalog /O 2 0 R >>`)
	b.Add(`<< /Name /A#20B#2Fc /Lit (a\(b\)c\n\101\7\
d) /Nested (x(y)z) /Hex <48 65 6C 6C 6F 7> /Int -12 /Real -.5 /Real2 3. /Plus +4
/Arr [1 2 0 R /N true false null [ ] << >>] /Dup 1 /Dup 2 /Null null /Ref 1 0 R /CRLF (a` + "\r\n" + `b) /Empty () >>`)
	r := open(t, b.Bytes(pdftest.Options{}))
	d, ok := r.Dict(r.Catalog().Get("O"))
	if !ok {
		t.Fatal("no object")
	}
	want := map[pdf.Name]pdf.Object{
		"Name":   pdf.Name("A B/c"),
		"Lit":    pdf.String("a(b)c\nA\x07d"),
		"Nested": pdf.String("x(y)z"),
		"Hex":    pdf.String("Hellop"),
		"Int":    pdf.Int(-12),
		"Real":   pdf.Real(-0.5),
		"Real2":  pdf.Real(3),
		"Plus":   pdf.Int(4),
		"Arr":    pdf.Array{pdf.Int(1), pdf.Ref{Num: 2}, pdf.Name("N"), pdf.Bool(true), pdf.Bool(false), nil, pdf.Array{}, pdf.Dict{}},
		"Dup":    pdf.Int(1),
		"Ref":    pdf.Ref{Num: 1},
		"CRLF":   pdf.String("a\nb"),
		"Empty":  pdf.String(""),
	}
	for k, v := range want {
		got := d.Get(k)
		if fmt.Sprint(got) != fmt.Sprint(v) || reflect.TypeOf(got) != reflect.TypeOf(v) {
			t.Errorf("%s = %#v, want %#v", k, got, v)
		}
	}
	if d.Has("Null") {
		t.Error("null entry kept")
	}
}

func TestDeepNesting(t *testing.T) {
	b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
	b.Add("<< /Type /Catalog /O 2 0 R >>")
	b.Add(strings.Repeat("[", 10000) + strings.Repeat("]", 10000))
	r := open(t, b.Bytes(pdftest.Options{}))
	r.Resolve(r.Catalog().Get("O")) // must not overflow the stack
}

func TestRefCycle(t *testing.T) {
	b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
	b.Add("<< /Type /Catalog /O 2 0 R >>")
	b.Add("3 0 R")
	b.Add("2 0 R")
	r := open(t, b.Bytes(pdftest.Options{}))
	if v := r.Resolve(r.Catalog().Get("O")); v != nil {
		if _, ok := v.(pdf.Ref); !ok {
			t.Errorf("cycle resolved to %#v", v)
		}
	}
}

func TestAppendObject(t *testing.T) {
	o := pdf.Dict{
		{Key: "Type", Val: pdf.Name("Filespec")},
		{Key: "Odd", Val: pdf.Name("a b#/c")},
		{Key: "F", Val: pdf.String("factur-x.xml")},
		{Key: "Bin", Val: pdf.String("\xfe\xff\x00a")},
		{Key: "Esc", Val: pdf.String("a(b)\\")},
		{Key: "Arr", Val: pdf.Array{pdf.Int(-3), pdf.Real(1.5), pdf.Bool(true), nil, pdf.Ref{Num: 7, Gen: 1}}},
		{Key: "Sub", Val: pdf.Dict{{Key: "K", Val: pdf.Int(1)}}},
	}
	b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
	b.Add("<< /Type /Catalog /O 2 0 R >>")
	b.Add(string(pdf.AppendObject(nil, o)))
	r := open(t, b.Bytes(pdftest.Options{}))
	got, _ := r.Dict(r.Catalog().Get("O"))
	if fmt.Sprint(got) != fmt.Sprint(o) {
		t.Fatalf("round trip:\n%v\n%v", got, o)
	}
}

func TestText(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"\xfe\xff\x00F\x00\xe9", "Fé"},
		{"\xff\xfeF\x00\xe9\x00", "Fé"},
		{"\xef\xbb\xbfFé", "Fé"},
		{"caf\xe9 \x80\x85\x93", "café •–ﬁ"},
		{"\xfe\xff\xd8\x3d\xde\x00", "\U0001F600"},
	} {
		if got := pdf.Text([]byte(tc.in)); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, s := range []string{"factur-x.xml", "Facture n° 12 – €", "日本"} {
		if got := pdf.Text(pdf.EncodeText(s)); got != s {
			t.Errorf("EncodeText(%q) round trip: %q", s, got)
		}
	}
	if e := pdf.EncodeText("ascii"); string(e) != "ascii" {
		t.Errorf("EncodeText(ascii) = %q", e)
	}
}

func TestDates(t *testing.T) {
	cet := time.FixedZone("", 3600)
	for _, tc := range []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"D:20261116120000+01'00'", time.Date(2026, 11, 16, 12, 0, 0, 0, cet), true},
		{"D:20261116120000+01'00", time.Date(2026, 11, 16, 12, 0, 0, 0, cet), true},
		{"D:20261116120000Z", time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), true},
		{"D:20261116120000Z00'00'", time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), true},
		{"20261116", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), true},
		{"D:2026", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{" D:20261116120000-05'30' ", time.Date(2026, 11, 16, 12, 0, 0, 0, time.FixedZone("", -5*3600-30*60)), true},
		{"D:20261316", time.Time{}, false},
		{"D:20260231", time.Time{}, false},
		{"D:26", time.Time{}, false},
		{"", time.Time{}, false},
	} {
		got, ok := pdf.ParseDate(tc.in)
		if ok != tc.ok || (ok && !got.Equal(tc.want)) {
			t.Errorf("ParseDate(%q) = %v, %v", tc.in, got, ok)
		}
	}
	d := time.Date(2026, 11, 16, 14, 30, 5, 999, cet)
	if s := pdf.FormatDate(d); s != "D:20261116143005+01'00'" {
		t.Errorf("FormatDate = %q", s)
	}
	if s := pdf.FormatDate(d.UTC()); s != "D:20261116133005Z" {
		t.Errorf("FormatDate UTC = %q", s)
	}
	if s := pdf.FormatDate(d.In(time.FixedZone("", -(9*3600 + 30*60)))); !strings.HasSuffix(s, "-09'30'") {
		t.Errorf("FormatDate negative offset = %q", s)
	}
}

// FuzzOpen checks that the reader never panics on damaged input.
func FuzzOpen(f *testing.F) {
	for _, o := range []pdftest.Options{{}, {Layout: pdftest.XrefStream, ObjStm: true, Predictor: true}, {Layout: pdftest.Hybrid, ObjStm: true}, {Layout: pdftest.NoXref}} {
		b, _ := sample()
		f.Add(b.Bytes(o))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := pdf.Open(data)
		if err != nil {
			return
		}
		var walk func(o pdf.Object, depth int)
		walk = func(o pdf.Object, depth int) {
			if depth > 8 {
				return
			}
			switch v := r.Resolve(o).(type) {
			case pdf.Dict:
				for _, kv := range v {
					walk(kv.Val, depth+1)
				}
			case pdf.Array:
				for _, x := range v {
					walk(x, depth+1)
				}
			case *pdf.Stream:
				r.Decode(v)
				walk(v.Dict, depth+1)
			}
		}
		walk(r.Trailer(), 0)
	})
}

func TestDictSkipArrays(t *testing.T) {
	font := "<< /Type /Font /Subtype /TrueType /Widths [506 226 1038] /BaseFont /Calibri /Encoding << /Differences [32 /space] >> /DescendantFonts [3 0 R] /FontDescriptor 3 0 R >>"
	for _, o := range []pdftest.Options{{}, {Layout: pdftest.XrefStream, ObjStm: true}, {Layout: pdftest.NoXref}} {
		b := &pdftest.Builder{Trailer: "/Root 1 0 R"}
		b.Add("<< /Type /Catalog /F 2 0 R >>")
		b.Add(font)
		b.Add("<< /Type /FontDescriptor /FontBBox [0 0 1 1] >>")
		r := open(t, b.Bytes(o))
		ref := r.Catalog().Get("F")

		d, ok := r.DictSkipArrays(ref, "DescendantFonts")
		if !ok {
			t.Fatalf("%+v: no dictionary", o)
		}
		if d.Has("Widths") {
			t.Errorf("%+v: /Widths read", o)
		}
		if n, _ := d.Name("BaseFont"); n != "Calibri" {
			t.Errorf("%+v: /BaseFont %q", o, n)
		}
		if list, ok := d.Get("DescendantFonts").(pdf.Array); !ok || len(list) != 1 {
			t.Errorf("%+v: /DescendantFonts %#v", o, d.Get("DescendantFonts"))
		}
		if enc, ok := d.Get("Encoding").(pdf.Dict); !ok || !enc.Has("Differences") {
			t.Errorf("%+v: nested arrays must be read: /Encoding %#v", o, d.Get("Encoding"))
		}
		if d.Get("FontDescriptor") != (pdf.Ref{Num: 3}) {
			t.Errorf("%+v: /FontDescriptor %#v", o, d.Get("FontDescriptor"))
		}

		// Not cached: the whole dictionary is still read in full.
		whole, _ := r.Dict(ref)
		if w, ok := whole.Get("Widths").(pdf.Array); !ok || len(w) != 3 {
			t.Fatalf("%+v: /Widths %#v after DictSkipArrays", o, whole.Get("Widths"))
		}
		// Once cached, the whole dictionary is returned.
		if d, _ := r.DictSkipArrays(ref); !d.Has("Widths") {
			t.Errorf("%+v: cached dictionary not returned whole", o)
		}
		// Direct objects are returned as they are.
		if d, _ := r.DictSkipArrays(whole); !d.Has("Widths") {
			t.Errorf("%+v: direct dictionary changed", o)
		}
	}
}

// TestUpdatedAfter checks the objects that the updates appended after a
// revision define, for each layout, and that a repaired file tells nothing.
func TestUpdatedAfter(t *testing.T) {
	for _, layout := range []pdftest.Layout{pdftest.Classic, pdftest.XrefStream, pdftest.Hybrid} {
		b, _ := sample()
		orig := b.Bytes(pdftest.Options{Layout: layout, ObjStm: true})
		r := open(t, orig)
		if nums, ok := r.UpdatedAfter(int64(len(orig))); !ok || len(nums) != 0 {
			t.Errorf("layout %d: updated after the end of the file: %v, %v", layout, nums, ok)
		}
		u := pdf.NewUpdate(r)
		ref := u.Alloc()
		u.Put(ref, pdf.Dict{{Key: "Note", Val: pdf.EncodeText("first")}})
		cat := r.Catalog().Clone()
		cat.Set("Note", ref)
		u.Put(pdf.Ref{Num: 1}, cat)
		out := u.Bytes(pdf.Dict{{Key: "Root", Val: pdf.Ref{Num: 1}}, {Key: "Info", Val: pdf.Ref{Num: 3}}})
		r2 := open(t, out)
		nums, ok := r2.UpdatedAfter(int64(len(orig)))
		if !ok || !slices.Contains(nums, 1) || !slices.Contains(nums, ref.Num) || slices.Contains(nums, 2) || !slices.IsSorted(nums) {
			t.Errorf("layout %d: updated after the first revision: %v, %v, want 1 and %d", layout, nums, ok, ref.Num)
		}
		if all, ok := r2.UpdatedAfter(0); !ok || !slices.Contains(all, 2) {
			t.Errorf("layout %d: updated after 0: %v, %v", layout, all, ok)
		}
	}
	b, _ := sample()
	r := open(t, b.Bytes(pdftest.Options{Shift: 11}))
	if _, ok := r.UpdatedAfter(0); ok {
		t.Error("a repaired file tells its revisions")
	}
}
