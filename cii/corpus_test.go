package cii

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The corpus test reads every CII document it can find:
//
//   - FX_CORPUS: the ZUGFeRD corpus (XML-Rechnung/CII/*.cii.xml, and the
//     XML files of the ZUGFeRDv2/correct and ZUGFeRDv1/correct trees);
//   - FX_CORPUS_XML: any tree of XML files extracted from PDF/A-3 invoices,
//     such as the ZUGFeRD 1.0 invoices of the PDFs of ZUGFeRDv1;
//   - ../testdata/*.xml.
//
// For each document it checks the round trip Parse → Marshal (same profile)
// → Parse, the XSD conformance of the output, and the lossless EXTENDED round
// trip of the model. The output of a source without VAT breakdown (BG-23)
// misses it too: the XSD error that this gives is the only one expected. When
// the same-profile output reports no loss, it must hold the whole tree of the
// original (see canonTree for the forms that are normalized). It also
// measures which leaves of the original document are missing from the
// rewritten ones: none may be missing silently.
//
// A ZUGFeRD 1.0 document is compared through its CII D16B view (see
// zugferd1View): Marshal converts it to Factur-X, and the output must hold
// the view, BT-24 aside, whose ZUGFeRD 1.0 identifier is replaced.

// corpusFile is a document of the corpus. Documents of FX_CORPUS and
// testdata must be well-formed (strict); extracted ones may be broken.
type corpusFile struct {
	name, path string
	strict     bool
}

func corpusFiles() []corpusFile {
	var files []corpusFile
	strict := true
	add := func(root, rel string) {
		files = append(files, corpusFile{name: rel, path: filepath.Join(root, rel), strict: strict})
	}
	walk := func(root, sub string, keep func(string) bool) {
		base := filepath.Join(root, sub)
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !keep(p) {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			add(root, rel)
			return nil
		})
	}
	isXML := func(p string) bool { return strings.HasSuffix(strings.ToLower(p), ".xml") }
	if root := os.Getenv("FX_CORPUS"); root != "" {
		walk(root, "XML-Rechnung/CII", isXML)
		walk(root, "ZUGFeRDv2/correct", isXML)
		walk(root, "ZUGFeRDv1/correct", isXML)
	}
	walk("../testdata", ".", isXML)
	if root := os.Getenv("FX_CORPUS_XML"); root != "" {
		strict = false
		walk(root, ".", isXML)
	}
	return files
}

// lossStats aggregates the leaves lost by the round trip, per path.
type lossStats struct {
	count map[string]int
	files map[string]map[string]bool
}

func (s *lossStats) add(path, file string) {
	if s.count == nil {
		s.count = map[string]int{}
		s.files = map[string]map[string]bool{}
	}
	s.count[path]++
	if s.files[path] == nil {
		s.files[path] = map[string]bool{}
	}
	s.files[path][file] = true
}

func (s *lossStats) report() string {
	paths := make([]string, 0, len(s.count))
	for p := range s.count {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		if s.count[paths[i]] != s.count[paths[j]] {
			return s.count[paths[i]] > s.count[paths[j]]
		}
		return paths[i] < paths[j]
	})
	var b strings.Builder
	for _, p := range paths {
		var sample string
		for f := range s.files[p] {
			if sample == "" || f < sample {
				sample = f
			}
		}
		fmt.Fprintf(&b, "  %5d leaves in %3d files  %s  (e.g. %s)\n", s.count[p], len(s.files[p]), p, filepath.Base(sample))
	}
	return b.String()
}

// normalizePath strips the root element and makes the leaf paths of a
// document comparable across documents.
func normalizePath(p string) string {
	return strings.TrimPrefix(p, "/CrossIndustryInvoice")
}

func TestCorpusRoundTrip(t *testing.T) {
	files := corpusFiles()
	if os.Getenv("FX_CORPUS") == "" && os.Getenv("FX_CORPUS_XML") == "" {
		t.Skip("FX_CORPUS and FX_CORPUS_XML not set")
	}
	var (
		read, notXML, zugferd1, otherRoot, withIssues, withLosses int
		trees, extTrees                                           int
		invalid, noVAT                                            int
		unexpected, profile, notFacturX, unreadable               lossStats
		added                                                     lossStats
		perProfile                                                = map[invoice.Profile]int{}
	)
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := Parse(data)
		var ie *IssuesError
		issues := errors.As(err, &ie)
		switch {
		case errors.As(err, &ie):
			withIssues++
			t.Logf("%s: %v", f.name, ie.Issues)
		case errors.Is(err, ErrNotCII):
			otherRoot++
			continue
		case err != nil:
			if f.strict {
				t.Errorf("%s: %v", f.name, err)
			}
			notXML++
			continue
		}
		// orig is the document, or the CII D16B view of a ZUGFeRD 1.0 one.
		orig, _ := xmltree.Parse(data)
		zf1 := IsZUGFeRD1(orig)
		if zf1 {
			zugferd1++
			orig, _ = zugferd1View(orig)
			if f.strict && inv.Profile == invoice.ProfileUnknown {
				t.Errorf("%s: no profile matched to the ZUGFeRD 1.0 identifier %q", f.name, inv.SpecificationID)
			}
		} else if f.strict && strings.Contains(f.name, "ZUGFeRDv1") {
			t.Errorf("%s: not read as a ZUGFeRD 1.0 document", f.name)
		}
		read++
		perProfile[inv.Profile]++

		target := inv.Profile
		if target == invoice.ProfileUnknown {
			target = invoice.ProfileEN16931
		}
		out, err := Marshal(inv, &MarshalOptions{Profile: target, Strict: true})
		var le *LossError
		lossy := errors.As(err, &le)
		if lossy {
			withLosses++
			t.Logf("%s: %v cannot carry %v", f.name, target, le.Terms)
			out, err = Marshal(inv, &MarshalOptions{Profile: target})
		}
		if err != nil {
			t.Errorf("%s: Marshal: %v", f.name, err)
			continue
		}
		// Marshal does not make up a VAT breakdown (BG-23), mandatory from
		// BASIC WL on: when the source has none, as some ZUGFeRD 1.0
		// invoices, the output misses it as the source does.
		var tolerated []string
		if len(inv.VATBreakdown) == 0 && levelOf(target) >= lvlBasicWL {
			noVAT++
			t.Logf("%s: no VAT breakdown (BG-23)", f.name)
			tolerated = append(tolerated, missingVATBreakdown)
		}
		checkXSDNamed(t, f.name, target, out, tolerated...)
		back, err := Parse(out)
		if err != nil {
			t.Errorf("%s: re-reading the output: %v", f.name, err)
			continue
		}
		if zf1 {
			// Marshal writes the Factur-X identifier of the profile.
			if back.SpecificationID != target.URN() {
				t.Errorf("%s: BT-24 of the converted ZUGFeRD 1.0 invoice is %q, want %q", f.name, back.SpecificationID, target.URN())
			}
			back.SpecificationID = inv.SpecificationID
		}
		if inv.BusinessProcess == "" && (target == invoice.ProfileXRechnung || target == invoice.ProfilePeppol) {
			// Marshal writes the business process type of Peppol billing,
			// which PEPPOL-EN16931-R001 requires (BT-23).
			if back.BusinessProcess != invoice.ProcessPeppolBilling {
				t.Errorf("%s: BT-23 of the invoice written in %v is %q, want %q", f.name, target, back.BusinessProcess, invoice.ProcessPeppolBilling)
			}
			back.BusinessProcess = ""
		}
		if !lossy {
			if d := diffInvoices(inv, back); d != "" {
				t.Errorf("%s: round trip in %v changes the model:\n%s", f.name, target, d)
			}
		}
		// Writing what was read back is a fixed point.
		if out2, _ := Marshal(back, &MarshalOptions{Profile: target}); string(out2) != string(out) {
			t.Errorf("%s: second write differs from the first", f.name)
		}
		// The model survives an EXTENDED round trip unchanged, unless the
		// source holds data that no Factur-X profile allows.
		ext, err := Marshal(inv, &MarshalOptions{Profile: invoice.ProfileExtended, Strict: true})
		outsideFacturX := errors.As(err, &le)
		if outsideFacturX {
			invalid++
			t.Logf("%s: no Factur-X profile can carry %v", f.name, le.Terms)
			ext, err = Marshal(inv, &MarshalOptions{Profile: invoice.ProfileExtended})
		} else if err == nil {
			backExt := mustParse(t, ext)
			backExt.Profile, backExt.SpecificationID = inv.Profile, inv.SpecificationID
			if d := diffInvoices(inv, backExt); d != "" {
				t.Errorf("%s: EXTENDED round trip changes the model:\n%s", f.name, d)
			}
		}
		if err != nil {
			t.Errorf("%s: EXTENDED: %v", f.name, err)
			continue
		}

		// Leaves of the original missing from the outputs: those missing
		// from the EXTENDED output are not carried by the model; those only
		// missing from the same-profile output are outside that profile.
		same, _ := xmltree.Parse(out)
		full, _ := xmltree.Parse(ext)
		norm := treeNorm{cur: inv.Currency, taxCur: inv.TaxCurrency, spec: !zf1}
		// Marshal may only add what the target schema requires (an empty
		// mandatory element, a missing indicator…): the leaves added are
		// listed below.
		// The values that cannot be read are lost: such a document is not
		// compared with its outputs.
		if !lossy && !issues {
			trees++
			if d := diffTrees(orig, same, norm); len(d.missing) > 0 || d.order != "" {
				t.Errorf("%s: the %v output does not hold the tree of the original:\n%v", f.name, target, d)
			}
		}
		// The model carries the whole tree of any document that follows a
		// Factur-X profile: the EXTENDED output holds it, BT-24 aside.
		if !outsideFacturX && !issues {
			extTrees++
			norm.spec = false
			if d := diffTrees(orig, full, norm); len(d.missing) > 0 || d.order != "" {
				t.Errorf("%s: the EXTENDED output does not hold the tree of the original:\n%v", f.name, d)
			}
		}
		lo, ls, lf := leaves(orig), leaves(same), leaves(full)
		for l, n := range lo {
			if l.text == "" && !strings.Contains(l.path, "/@") {
				continue // empty element
			}
			p := normalizePath(l.path)
			for i := lf[l]; i < n; i++ {
				switch {
				case strings.HasPrefix(p, "/ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/"):
					// BT-24 changes with the profile.
				case strings.HasSuffix(p, "/@schemaLocation"),
					strings.HasSuffix(p, "/@currencyID") && (l.text == inv.Currency || l.text == inv.TaxCurrency):
					// Normalized forms, see canonTree.
				case issues:
					// Reported by Parse as unreadable, or in a document that
					// has unreadable values.
					unreadable.add(p, f.name)
				case outsideFacturX:
					// Reported by Marshal: the source breaks the Factur-X
					// rules (e.g. a reference type on a non-130 document).
					notFacturX.add(p, f.name)
				default:
					unexpected.add(p, f.name)
					t.Logf("%s: lost %s = %q", f.name, p, l.text)
				}
			}
			for i := ls[l]; i < min(n, lf[l]); i++ {
				profile.add(p, f.name)
				if !lossy {
					t.Errorf("%s: %s = %q dropped without a loss being reported", f.name, p, l.text)
				}
			}
		}
		for l, n := range ls {
			for i := lo[l]; i < n; i++ {
				added.add(normalizePath(l.path), f.name)
			}
		}
	}
	t.Logf("%d documents read (%v), %d of them ZUGFeRD 1.0, %d with unreadable values, %d not fitting their profile, %d not fitting any profile, %d without the VAT breakdown of their profile; skipped: %d not XML, %d other roots",
		read, perProfile, zugferd1, withIssues, withLosses, invalid, noVAT, notXML, otherRoot)
	t.Logf("leaves outside the profile of their document (reported as losses):\n%s", profile.report())
	t.Logf("%d documents rewritten with their whole tree in their profile, %d in EXTENDED", trees, extTrees)
	t.Logf("leaves no Factur-X profile allows (reported as losses):\n%s", notFacturX.report())
	t.Logf("leaves of documents with unreadable values:\n%s", unreadable.report())
	t.Logf("leaves added or rewritten by Marshal:\n%s", added.report())
	if len(unexpected.count) > 0 {
		t.Errorf("unexpected losses:\n%s", unexpected.report())
	}
	if read == 0 {
		t.Error("no CII document read")
	}
}

// missingVATBreakdown is the XSD error of a CII document without BG-23.
const missingVATBreakdown = "/rsm:CrossIndustryInvoice/SupplyChainTradeTransaction/ApplicableHeaderTradeSettlement: missing <ApplicableTradeTax>"

// checkXSDNamed is checkXSD with the document name in the message. The errors
// listed in tolerated are expected: they are logged, the others reported.
func checkXSDNamed(t *testing.T, name string, p invoice.Profile, out []byte, tolerated ...string) {
	t.Helper()
	s := loadXSD(t, p)
	if s == nil {
		return
	}
	root, err := xmltree.Parse(out)
	if err != nil {
		t.Errorf("%s: output is not well-formed: %v", name, err)
		return
	}
	errs := s.check(root)
	for i := 0; i < len(errs); {
		if slices.Contains(tolerated, errs[i]) {
			t.Logf("%s: %v output, expected XSD error: %s", name, p, errs[i])
			errs = slices.Delete(errs, i, i+1)
			continue
		}
		i++
	}
	if len(errs) > 0 {
		if len(errs) > 5 {
			errs = append(errs[:5], fmt.Sprintf("… %d more", len(errs)-5))
		}
		t.Errorf("%s: %v output does not follow the XSD:\n  %s", name, p, strings.Join(errs, "\n  "))
	}
}
