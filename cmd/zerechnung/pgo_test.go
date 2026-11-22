package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/validate"
)

// BenchmarkCommands does the work of the validate, embed and json commands,
// in memory: validate and json on a hybrid PDF and on CII, UBL, FatturaPA
// and Facturae invoices of testdata, embed of the CII invoice. When
// FX_CORPUS is set, validate and json also read its PDFs and XML documents
// of at most corpusMax bytes, which may fail. Its CPU profile is
// default.pgo, which go build and go install use for this package (see
// CONTRIBUTING.md).
func BenchmarkCommands(b *testing.B) {
	// corpusMax leaves out the two PEPPOL invoices of 25 and 61 MB of the
	// corpus, which would take most of the profile.
	const corpusMax = 4 << 20
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			b.Fatal(err)
		}
		return data
	}
	var docs, corpus [][]byte
	for _, name := range []string{
		"../../testdata/EN16931_Einfach.pdf",
		"../../testdata/EN16931_Einfach.cii.xml",
		"../../testdata/XRECHNUNG_Einfach.ubl.xml",
		"../../fatturapa/testdata/fpa12.xml",
		"../../facturae/testdata/b2g.xml",
	} {
		docs = append(docs, read(name))
	}
	if root := os.Getenv("FX_CORPUS"); root != "" {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if ext := strings.ToLower(filepath.Ext(p)); err != nil || d.IsDir() || ext != ".pdf" && ext != ".xml" {
				return err
			}
			if info, err := d.Info(); err != nil || info.Size() > corpusMax {
				return err
			}
			corpus = append(corpus, read(p))
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	defer func(w io.Writer) { stderr = w }(stderr)
	stderr = io.Discard // the warnings of the corpus
	host, xml := read("../../testdata/host.pdf"), docs[1]
	b.ReportAllocs()
	for b.Loop() {
		for _, data := range docs {
			if _, err := report(data, &validate.Options{Auto: true}, true, io.Discard); err != nil {
				b.Fatal(err)
			}
			if err := writeJSON(data, io.Discard); err != nil {
				b.Fatal(err)
			}
		}
		for _, data := range corpus {
			_, _ = report(data, &validate.Options{Auto: true}, true, io.Discard)
			_ = writeJSON(data, io.Discard)
		}
		if _, err := zerechnung.Embed(host, xml, nil); err != nil {
			b.Fatal(err)
		}
	}
}
