package zerechnung

import (
	"os"
	"testing"

	"github.com/ritoon/zerechnung/validate"
)

func BenchmarkExtract(b *testing.B) {
	data, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		b.Skip(err)
	}
	if _, err := Extract(data); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Extract(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckContainer(b *testing.B) {
	data, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		b.Skip(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := CheckContainer(data); err != nil {
			b.Fatal(err)
		}
	}
}

func benchCheck(b *testing.B, opts *validate.Options) {
	data, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		b.Skip(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, att, err := Check(data, opts); err != nil || att == nil {
			b.Fatal(att, err)
		}
	}
}

// BenchmarkCheck measures Check with every PDF/A check, BenchmarkCheckBasic
// with validate.PDFABasic.
func BenchmarkCheck(b *testing.B) { benchCheck(b, nil) }
func BenchmarkCheckBasic(b *testing.B) {
	benchCheck(b, &validate.Options{PDFA: validate.PDFABasic})
}

func benchEmbed(b *testing.B, host string, opts *EmbedOptions) {
	data, err := os.ReadFile(host)
	if err != nil {
		b.Skip(err)
	}
	xml, err := os.ReadFile("testdata/EN16931_Einfach.cii.xml")
	if err != nil {
		b.Fatal(err)
	}
	if _, err := Embed(data, xml, opts); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Embed(data, xml, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEmbed(b *testing.B)        { benchEmbed(b, "bench/ecosystem/host.pdf", nil) }
func BenchmarkEmbedClassic(b *testing.B) { benchEmbed(b, "bench/ecosystem/host_classic.pdf", nil) }
