package zerechnung

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// convertRefusedValid lists the valid files of the veraPDF corpus that
// ConvertPDFA refuses, by reason.
var convertRefusedValid = map[string]string{
	"6-1-12-t02-pass-a": "a signed document",
}

// convertKnownFailures lists the files of the veraPDF corpus that
// ConvertPDFA converts but whose conversion veraPDF rejects, by reason:
// what the conversion does not read. A listed file whose conversion passes,
// or that is no longer converted, fails the test, so that the list stays
// exact.
var convertKnownFailures = func() map[string]string {
	m := map[string]string{}
	for reason, files := range map[string]string{
		"a value type of an XMP extension schema without pdfaType:field (6.6.2.3)": `6-6-2-3-3-t03-fail-b`,
	} {
		for _, f := range strings.Fields(files) {
			m[f] = reason
		}
	}
	return m
}()

// corpusPrefix starts the names of the converted files of the corpus in
// FX_PDFA_OUT; generated- starts those of the tests of convert_test.go.
const corpusPrefix = "corpus-"

// corpusID returns the name of a test of the veraPDF corpus, such as
// 6-1-12-t02-pass-a, from the name of its file or of its converted file.
func corpusID(name string) string {
	if i := strings.LastIndex(name, "veraPDF test suite "); i >= 0 {
		name = name[i+len("veraPDF test suite "):]
	}
	return strings.TrimSuffix(name, ".pdf")
}

// TestConvertCorpus converts every file of the veraPDF corpus (FX_VERAPDF,
// PDF/A-2b and PDF/A-3b, valid and invalid files): a valid file converts,
// but those of convertRefusedValid, an invalid one converts or is refused,
// and a converted file converts again. ConvertPDFA checks what it writes
// with the PDF/A rules of CheckContainer.
//
// With FX_PDFA_OUT, the converted files are written in that directory, for
// veraPDF (conformance/verapdf.sh). With FX_PDFA_REPORT as well, the text
// report of veraPDF on that directory is checked: every converted file
// passes, but those of convertKnownFailures.
func TestConvertCorpus(t *testing.T) {
	root := os.Getenv("FX_VERAPDF")
	if root == "" {
		t.Skip("FX_VERAPDF not set")
	}
	out := os.Getenv("FX_PDFA_OUT")
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	converted, refused := 0, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".pdf") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		id := corpusID(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		res, err := ConvertPDFA(data, &ConvertOptions{ModDate: testDate})
		_, refusable := convertRefusedValid[id]
		switch {
		case err == nil:
			converted++
			if refusable {
				t.Errorf("%s: converted, but listed in convertRefusedValid", rel)
			}
			if _, err := ConvertPDFA(res, &ConvertOptions{ModDate: testDate}); err != nil {
				t.Errorf("%s: the converted file does not convert again: %v", rel, err)
			}
			if out != "" {
				name := corpusPrefix + strings.ReplaceAll(filepath.ToSlash(rel), "/", "_")
				if err := os.WriteFile(filepath.Join(out, name), res, 0o644); err != nil {
					return err
				}
			}
		case errors.Is(err, ErrNotConvertible), errors.Is(err, ErrEncrypted):
			refused++
			if strings.Contains(id, "-pass-") && !refusable {
				t.Errorf("%s: valid file refused: %v", rel, err)
			}
		default:
			t.Errorf("%s: %v", rel, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if converted+refused == 0 {
		t.Fatalf("no PDF in %s", root)
	}
	t.Logf("%d files converted, %d refused", converted, refused)

	if report := os.Getenv("FX_PDFA_REPORT"); report != "" {
		if out == "" {
			t.Fatal("FX_PDFA_REPORT without FX_PDFA_OUT")
		}
		checkVeraPDFReport(t, out, report)
	}
}

// checkVeraPDFReport checks the text report of veraPDF (--format text) on
// the files of dir: every file is reported, and passes, but the files of
// the corpus listed in convertKnownFailures, which fail.
func checkVeraPDFReport(t *testing.T, dir, report string) {
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "PASS ") && !strings.HasPrefix(line, "FAIL ") {
			continue
		}
		rest := strings.TrimSpace(line[5:])
		if i := strings.LastIndex(rest, " "); i > 0 {
			rest = rest[:i] // the flavour, such as 3b
		}
		status[filepath.Base(rest)] = line[:4]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	passed, failed := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".pdf") {
			continue
		}
		var reason string
		known := false
		if strings.HasPrefix(name, corpusPrefix) {
			id := corpusID(name)
			seen[id] = true
			reason, known = convertKnownFailures[id]
		}
		st, ok := status[name]
		if !ok {
			t.Errorf("%s: not in the veraPDF report", name)
			continue
		}
		switch {
		case st == "PASS" && known:
			t.Errorf("%s: passes veraPDF, but listed in convertKnownFailures (%s)", name, reason)
		case st == "FAIL" && !known:
			t.Errorf("%s: veraPDF rejects the converted file", name)
		}
		if st == "PASS" {
			passed++
		} else {
			failed++
		}
	}
	var missing []string
	for id := range convertKnownFailures {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		t.Errorf("%s: listed in convertKnownFailures, but not converted", id)
	}
	t.Logf("veraPDF: %d converted files pass, %d fail", passed, failed)
}
