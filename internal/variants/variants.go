// Package variants writes the documents of the mutation cases of the rule
// packages, so that the parity job (conformance/parity.sh) compares them
// with the reference validators: the KoSIT validator for the EN 16931 and
// XRechnung rules, the NPa XSLT for the Dutch rules.
//
// A rule package declares no flag of its own: importing this package adds
// -variants to its test binary, and its TestVariants test writes there
//
//	go test ./internal/rules/de -run '^TestVariants$' -variants DIR
//
// one file per mutation case, <prefix>-<NNN>-<case>.xml, and DIR/expected.tsv,
// which gives for each file the rules its case expects, space separated (an
// empty list for a case without finding). The parity job checks that the
// reference raises them.
package variants

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var dir = flag.String("variants", "", "write the documents of the mutation cases to this directory")

// Writer writes the documents of the mutation cases to the directory of
// -variants. Without the flag, it writes nothing, so that TestVariants still
// builds every document.
type Writer struct {
	dir   string
	n     int
	files map[string]bool
	index strings.Builder
}

// New returns a Writer for the directory of -variants, which it creates.
func New(t testing.TB) *Writer {
	t.Helper()
	w := &Writer{dir: *dir, files: map[string]bool{}}
	if w.dir != "" {
		if err := os.MkdirAll(w.dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// Add writes the document of the case number i of a set: prefix-NNN-slug.xml,
// with the rules the case expects.
func (w *Writer) Add(t testing.TB, prefix string, i int, name string, doc []byte, want []string) {
	t.Helper()
	file := fmt.Sprintf("%s-%03d-%s.xml", prefix, i, Slug(name))
	if w.files[file] {
		t.Fatalf("variants: %s written twice", file)
	}
	w.files[file] = true
	w.n++
	if w.dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(w.dir, file), doc, 0o644); err != nil {
		t.Fatal(err)
	}
	rules := append([]string(nil), want...)
	sort.Strings(rules)
	fmt.Fprintf(&w.index, "%s\t%s\n", file, strings.Join(rules, " "))
}

// Close writes expected.tsv and logs the number of documents.
func (w *Writer) Close(t testing.TB) {
	t.Helper()
	if w.dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(w.dir, "expected.tsv"), []byte(w.index.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d variants written to %s", w.n, w.dir)
}

// Slug turns a case name into a file name: lower case, a hyphen for every
// run of other characters than letters and digits.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}
