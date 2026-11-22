package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// corpusDirs are the directories of ZUGFeRD/corpus that the comparison
// covers: the CII, UBL and Factur-X samples of XML-Rechnung, and the valid
// and invalid ZUGFeRD 2 / Factur-X invoices. ZUGFeRD 1.0 (whose schematron in
// Mustang has no rule identifiers), the Peppol samples of 25 and 61 MB,
// fatturaPA and the unsorted directories are left out.
var corpusDirs = []string{
	"XML-Rechnung/CII",
	"XML-Rechnung/UBL",
	"XML-Rechnung/FX",
	"ZUGFeRDv2/correct",
	"ZUGFeRDv2/fail",
}

// entry is one invoice of the comparison.
type entry struct {
	Name   string // staged file name: 0001.xml, 0002.pdf…
	Source string // origin: corpus/<path> or kosit/<path>
}

func (e entry) isPDF() bool { return strings.HasSuffix(e.Name, ".pdf") }

// negative reports whether the invoice is invalid on purpose: the "fail"
// directories of the corpus, and the not_validating sample of XML-Rechnung.
func (e entry) negative() bool {
	return strings.Contains(e.Source, "/fail/") || strings.Contains(path.Base(e.Source), "not_validating")
}

// nl reports whether the invoice is one of the NLCIUS comparison, which only
// the NPa XSLT receive.
func (e entry) nl() bool { return strings.HasPrefix(e.Source, "nl/") }

// variant reports whether the invoice is the document of a mutation case of
// the EN 16931 or XRechnung rules, which only the KoSIT validator receives.
func (e entry) variant() bool { return strings.HasPrefix(e.Source, "variants/") }

// pt reports whether the invoice is one of the CIUS-PT comparison, which only
// the CIUS-PT schematron receives.
func (e entry) pt() bool { return strings.HasPrefix(e.Source, "pt/") }

// fr reports whether the invoice is one of the EXTENDED-CTC-FR comparison,
// which only the XSLT of the FNFE-MPE receive.
func (e entry) fr() bool { return strings.HasPrefix(e.Source, "fr/") }

// national reports whether the invoice is one of the NLCIUS, CIUS-PT or
// EXTENDED-CTC-FR comparisons, which KoSIT and Mustang do not receive.
func (e entry) national() bool { return e.nl() || e.pt() || e.fr() }

// nlTestDirs are the directories of the SimplerInvoicing test files (FX_NL_TESTS)
// for the versions of the NPa artefacts: SI-UBL 2.0.3, NLCIUS-CII 1.0.3 and
// the G-account extension 1.0.
var nlTestDirs = []string{"SI-UBL-2.0.3.2", "NLCIUS-CII-1.0.3", "si-ubl-2.0-ext-gaccount-1.0"}

// nlInputs are the inputs of the NLCIUS comparison. The invoices are staged
// under the origins nl/simplerinvoicing/<dir>/<file>, nl/zerechnung/<path>
// and nl/variants/<file>.
type nlInputs struct {
	npa      string // FX_NL: checkout of peppolautoriteit-nl/validation, whose xsl directory holds the XSLT
	tests    string // FX_NL_TESTS: the SimplerInvoicing test files of phive-rules
	repo     string // root of this repository, whose internal/rules/nl/testdata holds the NLCIUS invoices
	variants string // documents of the mutation cases of internal/rules/nl (go test -variants)
}

// ptInputs are the inputs of the CIUS-PT comparison. The invoices are staged
// under the origins pt/espap/<file>, pt/zerechnung/<path> and
// pt/variants/<file>.
type ptInputs struct {
	examples string // FX_PT: archive of the examples of eSPap
	repo     string // root of this repository, whose internal/rules/pt/testdata holds the CIUS-PT invoices
	variants string // documents of the mutation cases of internal/rules/pt (go test -variants)
}

// stage stages the invoices. variants, when not empty, holds one directory
// per rule package (de, en16931, ubl) with the documents of its mutation
// cases (go test -variants), staged under the origin variants/<package>/<file>
// for the KoSIT validator. The rules that each mutation case expects, read
// from the expected.tsv of its directory (package internal/variants), are
// written to DIR/expected.tsv by origin.
func stage(corpus, kosit, work string, nl nlInputs, pt ptInputs, fr fnfeInputs, variants string) error {
	var sources []string                        // origin
	read := map[string]func() ([]byte, error){} // origin → contents
	var expected strings.Builder
	readExpected := func(dir, prefix string) error {
		data, err := os.ReadFile(filepath.Join(dir, "expected.tsv"))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if line == "" {
				continue
			}
			file, rules, ok := strings.Cut(line, "\t")
			if !ok || strings.Contains(file, "/") {
				return fmt.Errorf("%s: bad line %q", filepath.Join(dir, "expected.tsv"), line)
			}
			fmt.Fprintf(&expected, "%s/%s\t%s\n", prefix, file, rules)
		}
		return nil
	}
	// readExpectedPackages reads the expected.tsv of each directory of root,
	// one per rule package.
	readExpectedPackages := func(root, prefix string) error {
		dirs, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, d := range dirs {
			if d.IsDir() {
				if err := readExpected(filepath.Join(root, d.Name()), prefix+"/"+d.Name()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	add := func(root, prefix, dir string, exts ...string) error {
		if len(exts) == 0 {
			exts = []string{".xml", ".pdf"}
		}
		return filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := strings.ToLower(filepath.Ext(p))
			if !slices.Contains(exts, ext) {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			src := prefix + "/" + filepath.ToSlash(rel)
			sources = append(sources, src)
			read[src] = func() ([]byte, error) { return os.ReadFile(p) }
			return nil
		})
	}
	// addZip adds the XML files of an archive.
	addZip := func(name, prefix string) error {
		z, err := zip.OpenReader(name)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if f.FileInfo().IsDir() || strings.ToLower(path.Ext(f.Name)) != ".xml" {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("%s: %s: %w", name, f.Name, err)
			}
			src := prefix + "/" + f.Name
			sources = append(sources, src)
			read[src] = func() ([]byte, error) { return data, nil }
		}
		return nil
	}
	for _, d := range corpusDirs {
		if err := add(corpus, "corpus", d); err != nil {
			return err
		}
	}
	if err := add(kosit, "kosit", "src/test"); err != nil {
		return err
	}
	if nl.npa != "" {
		for _, d := range nlTestDirs {
			if err := add(nl.tests, "nl/simplerinvoicing", d, ".xml"); err != nil {
				return err
			}
		}
		if err := add(nl.repo, "nl/zerechnung", "internal/rules/nl/testdata", ".xml"); err != nil {
			return err
		}
		if err := add(nl.variants, "nl/variants", ".", ".xml"); err != nil {
			return err
		}
		if err := readExpected(nl.variants, "nl/variants"); err != nil {
			return err
		}
	}
	if variants != "" {
		if err := add(variants, "variants", ".", ".xml"); err != nil {
			return err
		}
		if err := readExpectedPackages(variants, "variants"); err != nil {
			return err
		}
	}
	if pt.examples != "" {
		if err := addZip(pt.examples, "pt/espap"); err != nil {
			return err
		}
		if err := add(pt.repo, "pt/zerechnung", "internal/rules/pt/testdata", ".xml"); err != nil {
			return err
		}
		if err := add(pt.variants, "pt/variants", ".", ".xml"); err != nil {
			return err
		}
	}
	if fr.rfe != "" {
		for _, d := range fnfeExampleDirs {
			if err := add(fr.rfe, "fr/fnfe", d, ".xml"); err != nil {
				return err
			}
		}
		if err := add(fr.repo, "fr/zerechnung", fnfeRepoDir, ".xml"); err != nil {
			return err
		}
		if err := add(fr.variants, "fr/variants", ".", ".xml"); err != nil {
			return err
		}
		if err := readExpectedPackages(fr.variants, "fr/variants"); err != nil {
			return err
		}
		// Of the French invoices of the repository, the EXTENDED-CTC-FR ones.
		var err error
		sources = slices.DeleteFunc(sources, func(src string) bool {
			if err != nil || !strings.HasPrefix(src, "fr/zerechnung/") {
				return false
			}
			var data []byte
			data, err = read[src]()
			return err == nil && !declaresCTCFR(data)
		})
		if err != nil {
			return err
		}
	}
	sort.Strings(sources)

	in := filepath.Join(work, "in")
	if err := os.RemoveAll(in); err != nil {
		return err
	}
	if err := os.MkdirAll(in, 0o755); err != nil {
		return err
	}
	var index, kositList, mustangList, npaList, ptList, fnfeList strings.Builder
	nlFiles, ptFiles, frFiles, variantFiles := 0, 0, 0, 0
	for i, src := range sources {
		ext := strings.ToLower(filepath.Ext(src))
		e := entry{Name: fmt.Sprintf("%04d%s", i+1, ext), Source: src}
		data, err := read[src]()
		if err != nil {
			return err
		}
		staged := filepath.Join(in, e.Name)
		if err := os.WriteFile(staged, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(&index, "%s\t%s\n", e.Name, e.Source)
		abs, err := filepath.Abs(staged)
		if err != nil {
			return err
		}
		switch {
		case e.nl():
			xsl := npaStylesheet(data)
			if xsl == "" {
				return fmt.Errorf("%s: neither a CII nor a UBL invoice", src)
			}
			fmt.Fprintf(&npaList, "%s\t%s\n", filepath.Join(nl.npa, "xsl", xsl), abs)
			nlFiles++
		case e.pt():
			fmt.Fprintln(&ptList, abs)
			ptFiles++
		case e.fr():
			line, err := fnfeListLine(fr.rfe, src, abs, data)
			if err != nil {
				return err
			}
			fnfeList.WriteString(line)
			frFiles++
		case e.variant():
			fmt.Fprintln(&kositList, abs)
			variantFiles++
		case e.isPDF():
			fmt.Fprintln(&mustangList, abs)
		default:
			fmt.Fprintln(&kositList, abs)
			fmt.Fprintln(&mustangList, abs)
		}
	}
	for name, s := range map[string]string{"index.tsv": index.String(), "kosit.list": kositList.String(), "mustang.list": mustangList.String(), "npa.list": npaList.String(), "pt.list": ptList.String(), "fnfe.list": fnfeList.String(), "expected.tsv": expected.String()} {
		if err := os.WriteFile(filepath.Join(work, name), []byte(s), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%d invoices staged in %s, %d of them for the NPa XSLT, %d for the CIUS-PT schematron, %d for the EXTENDED-CTC-FR XSLT, %d variants for the KoSIT validator\n", len(sources), in, nlFiles, ptFiles, frFiles, variantFiles)
	return nil
}

// readIndex reads DIR/index.tsv.
func readIndex(work string) ([]entry, error) {
	data, err := os.ReadFile(filepath.Join(work, "index.tsv"))
	if err != nil {
		return nil, err
	}
	var entries []entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		name, src, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("index.tsv: bad line %q", line)
		}
		entries = append(entries, entry{Name: name, Source: src})
	}
	return entries, nil
}

// readExpectedRules reads DIR/expected.tsv: the rules that the mutation case
// of each variant expects, by origin.
func readExpectedRules(work string) (map[string][]string, error) {
	data, err := os.ReadFile(filepath.Join(work, "expected.tsv"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		src, rules, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("expected.tsv: bad line %q", line)
		}
		out[src] = strings.Fields(rules)
	}
	return out, nil
}
