package pdf

import (
	"bytes"
	"compress/zlib"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// inflateSample is a FlateDecode stream and its size once decoded.
type inflateSample struct {
	data    []byte
	decoded int
}

// inflateData returns the FlateDecode streams of BenchmarkInflateData, by
// kind of data. The font programs (three TrueType programs) and the invoice
// XML are those of testdata/EN16931_Einfach.pdf, as its generator
// compressed them. Its content streams are not compressed, and it has no
// object stream: its content streams, and the text of its objects as an
// object stream would hold it, are compressed with zlib at the default
// level, as most generators do. With FX_VERAPDF, the directory of the
// veraPDF corpus, the TrueType programs of its files are added.
func inflateData(b *testing.B) map[string][]inflateSample {
	pdf, err := os.ReadFile("../../testdata/EN16931_Einfach.pdf")
	if err != nil {
		b.Skip(err)
	}
	r, err := Open(pdf)
	if err != nil {
		b.Fatal(err)
	}
	data := map[string][]inflateSample{}
	add := func(kind string, raw []byte) {
		dec, err := inflate(raw, 0)
		if err != nil {
			b.Fatalf("%s: %v", kind, err)
		}
		data[kind] = append(data[kind], inflateSample{raw, len(dec)})
	}
	compress := func(plain []byte) []byte {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(plain)
		w.Close()
		return buf.Bytes()
	}
	for n := uint32(1); n < 64; n++ {
		st, ok := r.Object(n).(*Stream)
		if !ok {
			continue
		}
		switch t, _ := st.Dict.Name("Type"); {
		case st.Dict.Has("Length1"):
			add("fonts", st.Raw)
		case t == "EmbeddedFile":
			add("xml", st.Raw)
		case t == "" && !st.Dict.Has("Filter") && !st.Dict.Has("N"):
			add("content", compress(st.Raw))
		}
	}
	// The objects that are not streams, as the text of an object stream.
	var objects []byte
	for _, m := range regexp.MustCompile(`(?s)\d+ 0 obj\s*(.*?)\s*endobj`).FindAllSubmatch(pdf, -1) {
		if !bytes.Contains(m[1], []byte("stream")) {
			objects = append(append(objects, m[1]...), '\n')
		}
	}
	add("objects", compress(objects))
	if dir := os.Getenv("FX_VERAPDF"); dir != "" {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") || len(data["corpus-fonts"]) >= 40 {
				return nil
			}
			b, _ := os.ReadFile(p)
			r, err := Open(b)
			if err != nil {
				return nil
			}
			for n := uint32(1); n < 200; n++ {
				if st, ok := r.Object(n).(*Stream); ok && st.Dict.Has("Length1") {
					if f, _ := st.Dict.Name("Filter"); f == "FlateDecode" {
						if dec, err := inflate(st.Raw, 0); err == nil {
							data["corpus-fonts"] = append(data["corpus-fonts"], inflateSample{st.Raw, len(dec)})
						}
					}
				}
			}
			return nil
		})
	}
	for _, kind := range []string{"fonts", "xml", "content", "objects"} {
		if len(data[kind]) == 0 {
			b.Fatalf("no %s in the test PDF", kind)
		}
	}
	return data
}

// BenchmarkInflateData measures the decoder on each kind of data that
// Extract and Check decompress: font programs, invoice XML, content
// streams and object streams. The throughput counts the decoded bytes; the
// buffers are lent by a Scratch, as in Check.
func BenchmarkInflateData(b *testing.B) {
	data := inflateData(b)
	for _, kind := range []string{"fonts", "xml", "content", "objects", "corpus-fonts"} {
		samples := data[kind]
		if len(samples) == 0 {
			continue
		}
		b.Run(kind, func(b *testing.B) {
			size := 0
			for _, s := range samples {
				size += s.decoded
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			var sc Scratch
			for b.Loop() {
				for _, s := range samples {
					if _, err := inflateIn(s.data, s.decoded, &sc); err != nil {
						b.Fatal(err)
					}
				}
				sc.Release()
			}
		})
	}
}
