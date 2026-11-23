package zerechnung

import (
	"os"
	"path/filepath"
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

// BenchmarkEmbed and BenchmarkEmbedClassic embed the XML in the host PDFs of
// the benchmarks of github.com/ritoon/invoice-bench, the first with a
// cross-reference stream, the second with a classic cross-reference table.
// FX_BENCH_HOSTS is the directory that holds them: ecosystem/ in a clone of
// invoice-bench, after ./run.sh setup. Without it, both are skipped, and
// BenchmarkEmbedSmall, which embeds the XML in testdata/host.pdf (one page,
// 864 bytes, a classic cross-reference table), measures Embed.
func BenchmarkEmbed(b *testing.B)        { benchEmbed(b, benchHost(b, "host.pdf"), nil) }
func BenchmarkEmbedClassic(b *testing.B) { benchEmbed(b, benchHost(b, "host_classic.pdf"), nil) }
func BenchmarkEmbedSmall(b *testing.B)   { benchEmbed(b, "testdata/host.pdf", nil) }

func benchHost(b *testing.B, name string) string {
	dir := os.Getenv("FX_BENCH_HOSTS")
	if dir == "" {
		b.Skip("FX_BENCH_HOSTS is not set: BenchmarkEmbedSmall measures Embed on testdata/host.pdf")
	}
	return filepath.Join(dir, name)
}
