package zerechnung

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
)

// clauseRules maps the clauses of the veraPDF corpus (the prefix of its test
// names) to the rules that report a failure of the clause. A finding of any
// severity counts.
var clauseRules = map[string]string{
	"6-1-2": "PDF-41", "6-1-3": "PDF-01 PDF-08 PDF-42", "6-1-4": "PDF-43", "6-1-6": "PDF-44",
	"6-1-7-1": "PDF-45", "6-1-8": "PDF-46", "6-1-9": "PDF-47", "6-1-10": "PDF-38", "6-1-12": "PDF-48",
	"6-1-13": "PDF-49", "6-2-2": "PDF-55 PDF-71", "6-2-3": "PDF-06 PDF-07 PDF-50", "6-2-4-2": "PDF-51",
	"6-2-4-3": "PDF-52", "6-2-4-4": "PDF-49 PDF-52 PDF-53", "6-2-4-5": "PDF-51 PDF-52",
	"6-2-5": "PDF-54 PDF-55", "6-2-6": "PDF-55", "6-2-8": "PDF-55 PDF-56", "6-2-9": "PDF-57",
	"6-2-10": "PDF-52 PDF-58", "6-2-11-3": "PDF-59", "6-2-11-4": "PDF-31 PDF-59 PDF-72", "6-2-11-5": "PDF-59 PDF-72",
	"6-2-11-6": "PDF-60", "6-2-11-8": "PDF-31 PDF-72", "6-3-1": "PDF-61", "6-3-2": "PDF-62", "6-3-3": "PDF-63",
	"6-4-1": "PDF-34 PDF-64", "6-4-2": "PDF-64", "6-5-1": "PDF-33", "6-5-2": "PDF-34",
	"6-6-2-1": "PDF-02 PDF-03 PDF-65", "6-6-2-3": "PDF-66 PDF-67", "6-6-4": "PDF-66", "6-6-6": "PDF-66 PDF-67",
	"6-8": "PDF-20 PDF-36 PDF-37 PDF-39", "6-9": "PDF-68",
}

// pdfaRules are the rules a valid file of the corpus must not raise as
// errors: the PDF/A requirements, without the identification (PDF-04,
// PDF-05, PDF-29, PDF-30: the corpus is PDF/A-2b and PDF/A-3b) and the
// rules of the invoice attachment. The associated files (PDF-36, PDF-37,
// PDF-39) are PDF/A-3 requirements.
var pdfaRules = strings.Fields(`PDF-01 PDF-02 PDF-03 PDF-06 PDF-07 PDF-08 PDF-31 PDF-32 PDF-33 PDF-34
	PDF-35 PDF-36 PDF-37 PDF-38 PDF-39 PDF-41 PDF-42 PDF-43 PDF-44 PDF-45 PDF-46 PDF-47 PDF-48 PDF-49
	PDF-50 PDF-51 PDF-52 PDF-53 PDF-54 PDF-55 PDF-56 PDF-57 PDF-58 PDF-59 PDF-60 PDF-61 PDF-62 PDF-63
	PDF-64 PDF-65 PDF-66 PDF-67 PDF-68 PDF-69 PDF-70 PDF-71 PDF-72`)

// veraPDFUncovered lists the invalid files of the corpus that CheckContainer
// does not report, by reason. A listed file that gets reported fails the
// test, so that the list stays exact.
var veraPDFUncovered = func() map[string]string {
	m := map[string]string{}
	for reason, files := range map[string]string{
		"a value type without pdfaType:field, which the veraPDF profile does not require": `6-6-2-3-3-t03-fail-b`,
		"the PDF/A identification: PDF-04 and PDF-05 expect PDF/A-3 or PDF/A-4 and report every file of the corpus": `6-6-4-t01-fail-c
			6-6-4-t01-fail-d 6-6-4-t01-fail-e 6-6-4-t01-fail-f`,
	} {
		for _, f := range strings.Fields(files) {
			m[f] = reason
		}
	}
	return m
}()

// veraPDFValid lists the valid files of the corpus that raise an error, by
// reason. None does; a file added here must say why.
var veraPDFValid = map[string]string{}

var veraPDFName = regexp.MustCompile(`^veraPDF test suite (.+)-(pass|fail)-[a-z]+\.pdf$`)

// TestVeraPDFCorpus runs CheckContainer on the PDF/A-2b and PDF/A-3b test
// files of the veraPDF corpus (FX_VERAPDF): a valid file raises no error of
// the PDF/A rules, an invalid file raises a rule of its clause (clauseRules),
// except the files listed in veraPDFValid and veraPDFUncovered.
func TestVeraPDFCorpus(t *testing.T) {
	root := os.Getenv("FX_VERAPDF")
	if root == "" {
		t.Skip("FX_VERAPDF not set")
	}
	pdfa := map[string]bool{}
	for _, id := range pdfaRules {
		pdfa[id] = true
	}
	var valid, invalid, reported int
	seen := map[string]bool{}
	for _, dir := range []string{"PDF_A-2b", "PDF_A-3b"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			m := veraPDFName.FindStringSubmatch(d.Name())
			if m == nil {
				return nil
			}
			name := strings.TrimSuffix(strings.TrimPrefix(d.Name(), "veraPDF test suite "), ".pdf")
			seen[name] = true
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			_, findings, err := CheckContainer(data)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				return nil
			}
			if m[2] == "pass" {
				valid++
				var errs []string
				for _, f := range findings {
					if f.Severity == rules.Error && pdfa[f.Rule] {
						errs = append(errs, f.String())
					}
				}
				switch reason, listed := veraPDFValid[name]; {
				case len(errs) > 0 && !listed:
					t.Errorf("%s: valid file, errors:\n\t%s", name, strings.Join(errs, "\n\t"))
				case len(errs) == 0 && listed:
					t.Errorf("%s: no error any more, remove it from veraPDFValid (%s)", name, reason)
				}
				return nil
			}
			invalid++
			want := map[string]bool{}
			clause := ""
			for c := range clauseRules {
				if strings.HasPrefix(m[1], c+"-") && len(c) > len(clause) {
					clause = c
				}
			}
			for _, id := range strings.Fields(clauseRules[clause]) {
				want[id] = true
			}
			got := false
			var rulesRaised []string
			for _, f := range findings {
				rulesRaised = append(rulesRaised, f.Rule)
				got = got || want[f.Rule]
			}
			if got {
				reported++
			}
			switch reason, listed := veraPDFUncovered[name]; {
			case !got && !listed:
				t.Errorf("%s: invalid file (clause %s), none of %s raised; raised: %v", name, clause, clauseRules[clause], rulesRaised)
			case got && listed:
				t.Errorf("%s: reported now, remove it from veraPDFUncovered (%s)", name, reason)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var missing []string
	for name := range veraPDFUncovered {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 || invalid == 0 {
		t.Errorf("%d invalid files; listed files not found: %v", invalid, missing)
	}
	t.Logf("%d valid files; %d invalid files, %d reported, %d not reported (veraPDFUncovered)", valid, invalid, reported, len(veraPDFUncovered))
}
