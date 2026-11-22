// Command gen generates the rule catalogs of package en16931:
//
//   - catalog_gen.go, from the CII validation artefacts of CEN TC 434
//     (EN 16931);
//   - ctcfr_gen.go (-ctcfr), from the FNFE-MPE schematrons of the French
//     EXTENDED-CTC-FR profile for CII and UBL.
//
// By default it reads the release archive en16931-cii-<version>.zip, either
// from a local file (-zip) or downloaded from the GitHub release of
// ConnectingEurope/eInvoicing-EN16931, checks its SHA-256 and writes one
// rules.RuleInfo per assertion of the preprocessed schematron
// (schematron/preprocessed/EN16931-CII-validation-preprocessed.sch): its
// identifier, its severity (flag fatal: Error, warning: Warning) and its
// official message.
//
// With -ctcfr, it reads the schematrons EXTENDED-CTC-FR-CII.sch and
// EXTENDED-CTC-FR-UBL.sch of the France_RFE package 1.4.0.04
// (FNFE_RFE_INVOICE/{CII,UBL}/EXTENDED-CTC-FR/schematron), either from -src
// (a checkout of github.com/fnfempe/France_RFE) or downloaded from the
// repository at the commit of the release tag, and checks their SHA-256. It
// writes all the assertions of the CII schematron the same way: the model, CII
// syntax and code list patterns, which the schematron derives from the CEN
// ones, and the two patterns of its own (line subtypes, item attribute
// codes). The assertions commented out in the file are not read. An
// assertion without identifier gets the one its message starts with
// ([BR-FXEXT-04]); a message that also holds diagnostics (xsl:value-of) is
// the text after its last identifier in brackets. The lists of the code list
// assertions (contains(' A B ', concat(' ', ., ' '))) and of BR-CO-09 are
// written as a table too. Of the UBL schematron, whose syntax rules, code
// lists and rules of its own package internal/rules/ubl generates and
// checks, it writes the identifiers and severities of the model and code
// list assertions (ctcfrUBLRules): the rules that the package may report on
// the CII view of an EXTENDED-CTC-FR invoice in UBL.
//
// In both cases, the syntax assertions whose test is not(path) or
// count(path) <= n (also written "le n"), or count(path) = n, are also
// written as tables that package en16931 checks generically; the other
// syntax assertions are written by hand. Assertions that can never fail are
// left out (see skipped).
//
// Usage (from internal/rules/en16931, through go generate):
//
//	go run ./gen [-zip en16931-cii-1.3.17.zip] [-o catalog_gen.go]
//	go run ./gen -ctcfr [-src path] [-o ctcfr_gen.go]
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	release = "validation-1.3.17"
	archive = "en16931-cii-1.3.17.zip"
	url     = "https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/" + release + "/" + archive
	sum     = "e5f3235824b6b38454b1d54cf3f14e01efb5b33971fbefc4e64997d0ef619e89"

	schFile = "schematron/preprocessed/EN16931-CII-validation-preprocessed.sch"
)

// The EXTENDED-CTC-FR schematron of the France_RFE package.
const (
	ctcfrRelease = "1.4.0.04"
	ctcfrCommit  = "97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d" // tag v1.4.0.04
	ctcfrFile    = "FNFE_RFE_INVOICE/CII/EXTENDED-CTC-FR/schematron/EXTENDED-CTC-FR-CII.sch"
	ctcfrSum     = "768cd0610d163df37786984f9b9e52e466d1f987a427964161a7ab301ad41c7b"

	ctcfrUBLFile = "FNFE_RFE_INVOICE/UBL/EXTENDED-CTC-FR/schematron/EXTENDED-CTC-FR-UBL.sch"
	ctcfrUBLSum  = "ba8f8f2611b950519cc1eb88698dec2b1dbf91f7495f55d4b682540f99e6077e"
)

// ctcfrBase is the base URL of the files of the France_RFE package at the
// commit of the release tag.
const ctcfrBase = "https://raw.githubusercontent.com/fnfempe/France_RFE/" + ctcfrCommit + "/"

// ctcfrUBLPatterns are the patterns of the UBL schematron whose assertions
// ctcfrUBLRules lists.
var ctcfrUBLPatterns = map[string]bool{"UBL-model": true, "Codesmodel": true}

// skipped lists the assertions of the CEN schematron that never fail in the
// official artefacts and are therefore not implemented. Assertions whose
// test is true() are skipped as well.
var skipped = map[string]string{
	// Same context as CII-SR-035..038 and CII-SR-221, declared after them in
	// the same pattern: the first rule wins and these never run.
	"CII-SR-039": "shadowed", "CII-SR-040": "shadowed", "CII-SR-041": "shadowed",
	"CII-SR-042": "shadowed", "CII-SR-043": "shadowed", "CII-SR-044": "shadowed",
	// Context rsm:ExchangedDocument/ram:TypeCode is also matched by the
	// earlier rule //ram:TypeCode (CII-DT-008, CII-DT-009).
	"CII-DT-010": "shadowed", "CII-DT-011": "shadowed", "CII-DT-012": "shadowed",
	// The test iterates over ../ram:RateApplicablePercent of a VAT breakdown,
	// that is over an element that cannot exist: always true.
	"BR-IP-08": "always true", "BR-IG-08": "always true",
}

// skippedCTCFR lists the assertions of the EXTENDED-CTC-FR schematron that
// never fail. The schematron gives CII-SR-035..038 and CII-SR-221 the
// context of the line (ram:IncludedSupplyChainTradeLineItem): CII-SR-042..044,
// whose context is ram:AssociatedDocumentLineDocument, are no longer shadowed
// and do run (CII-SR-039..041 are commented out).
var skippedCTCFR = map[string]string{
	"CII-DT-010": "shadowed", "CII-DT-011": "shadowed", "CII-DT-012": "shadowed",
	// The test starts with not(self::ram:ContractReferencedDocument), whose
	// context node is a ram:ReferenceTypeCode: always true.
	"BR-EXT-CL-07": "always true",
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("en16931/gen: ")
	zipPath := flag.String("zip", "", "path of "+archive+" (downloaded from GitHub when empty)")
	ctcfr := flag.Bool("ctcfr", false, "generate the EXTENDED-CTC-FR syntax rules from the FNFE-MPE schematron")
	srcPath := flag.String("src", "", "with -ctcfr: a checkout of France_RFE (the schematrons are downloaded when empty)")
	out := flag.String("o", "", "output file (default catalog_gen.go, ctcfr_gen.go with -ctcfr)")
	flag.Parse()

	var src []byte
	var err error
	switch {
	case *ctcfr && *zipPath == "":
		if *out == "" {
			*out = "ctcfr_gen.go"
		}
		src, err = generateCTCFR(*srcPath)
	case !*ctcfr && *srcPath == "":
		if *out == "" {
			*out = "catalog_gen.go"
		}
		src, err = generateCEN(*zipPath)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}

func generateCEN(zipPath string) ([]byte, error) {
	var data []byte
	var err error
	if zipPath != "" {
		data, err = os.ReadFile(zipPath)
	} else {
		data, err = download(url)
	}
	if err != nil {
		return nil, err
	}
	if err := checkSum(archive, data, sum); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	sch, err := readFile(zr, schFile)
	if err != nil {
		return nil, err
	}
	return generate(sch, cenSource)
}

// readCTCFR returns the file of the France_RFE package read from the
// checkout dir, or downloaded when dir is empty, after checking its SHA-256.
func readCTCFR(dir, file, sum string) ([]byte, error) {
	var data []byte
	var err error
	if dir == "" {
		data, err = download(ctcfrBase + file)
	} else {
		data, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	}
	if err != nil {
		return nil, err
	}
	if err := checkSum(filepath.Base(file), data, sum); err != nil {
		return nil, err
	}
	return data, nil
}

func generateCTCFR(dir string) ([]byte, error) {
	sch, err := readCTCFR(dir, ctcfrFile, ctcfrSum)
	if err != nil {
		return nil, err
	}
	ubl, err := readCTCFR(dir, ctcfrUBLFile, ctcfrUBLSum)
	if err != nil {
		return nil, err
	}
	src, err := generate(sch, ctcfrSource)
	if err != nil {
		return nil, err
	}
	table, err := ublRules(ubl)
	if err != nil {
		return nil, err
	}
	return format.Source(append(src, table...))
}

// ublRules returns the declaration of ctcfrUBLRules: the identifiers and
// severities of the assertions of the patterns ctcfrUBLPatterns of the UBL
// schematron.
func ublRules(sch []byte) ([]byte, error) {
	var s schema
	if err := xml.Unmarshal(sch, &s); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `
// ctcfrUBLRules maps the model and code list rules of the EXTENDED-CTC-FR
// schematron for UBL (%s, SHA-256
// %s) to their severity. For an
// EXTENDED-CTC-FR invoice in UBL, the package reports, on the CII view, only
// the rules of this table: package internal/rules/ubl checks the syntax
// rules, the code lists and the rules of the schematron on the UBL document.
// The map is built on first use.
var ctcfrUBLRules = sync.OnceValue(func() map[string]rules.Severity {
	return map[string]rules.Severity{
`, ctcfrUBLFile, ctcfrUBLSum)
	seen := map[string]bool{}
	for _, p := range s.Patterns {
		if !ctcfrUBLPatterns[p.ID] {
			continue
		}
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				if a.ID == "" || seen[a.ID] {
					return nil, fmt.Errorf("%s: assertion without identifier or duplicate (%q)", ctcfrUBLFile, a.ID)
				}
				seen[a.ID] = true
				sev := "rules.Error"
				switch a.Flag {
				case "fatal":
				case "warning":
					sev = "rules.Warning"
				default:
					return nil, fmt.Errorf("%s: unknown flag %q", a.ID, a.Flag)
				}
				fmt.Fprintf(&b, "\t%q: %s,\n", a.ID, sev)
			}
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("%s: no model assertion found", ctcfrUBLFile)
	}
	b.WriteString("}\n})\n")
	return b.Bytes(), nil
}

func checkSum(name string, data []byte, want string) error {
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != want {
		return fmt.Errorf("%s: SHA-256 %s, want %s", name, got, want)
	}
	return nil
}

func download(u string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func readFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name || strings.HasSuffix(f.Name, "/"+name) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s: not in %s", name, archive)
}

type schema struct {
	Patterns []struct {
		ID    string `xml:"id,attr"`
		Rules []struct {
			Context string `xml:"context,attr"`
			Asserts []struct {
				ID    string `xml:"id,attr"`
				Flag  string `xml:"flag,attr"`
				Test  string `xml:"test,attr"`
				Text  string `xml:",chardata"`
				Inner string `xml:",innerxml"`
			} `xml:"assert"`
		} `xml:"rule"`
	} `xml:"pattern"`
}

var (
	rePrefix = regexp.MustCompile(`^\s*\[[^\]]+\]\s*-?\s*`)
	// The identifier a message starts with.
	reID = regexp.MustCompile(`^\s*\[([A-Z][A-Z0-9-]*)\]`)
	// The last rule identifier in brackets of a message that also holds
	// diagnostics (xsl:value-of): the text after it is the message proper.
	reLastID = regexp.MustCompile(`\[[A-Z]+-[^\]]*\]\s*(?:warning\s*)?-?\s*`)
	// contains(' A B C ', concat(' ', ..., ' ')): the list of a code list
	// assertion.
	reList = regexp.MustCompile(`contains\(\s*'( [^']* )'\s*,\s*concat\(`)
	// @mimeCode = 'type': the values of BR-FREXT-CL-24.
	reMime = regexp.MustCompile(`@mimeCode\s*=\s*'([^']*)'`)
	// not(path): an element or attribute that must not exist.
	reNot = regexp.MustCompile(`^not\s*\(\s*((?:[a-z]+:[A-Za-z]+)(?:/[a-z]+:[A-Za-z]+)*(?:/@[A-Za-z]+)?|@[A-Za-z]+)\s*\)$`)
	// count(path) <= n (or le n) or count(path) = n.
	reCount = regexp.MustCompile(`^\(?\s*count\s*\(\s*((?:[a-z]+:[A-Za-z]+)(?:/[a-z]+:[A-Za-z]+)*)\s*\)\s*(<=|=|le)\s*(\d+)\s*\)?$`)
)

const syntaxPattern = "EN16931-CII-Syntax"

// source describes the generated file of one schematron.
type source struct {
	header  string            // comments and declarations before the catalog
	catalog string            // name and documentation of the catalog variable
	nots    string            // name of the not(path) table
	counts  string            // name of the count(path) table
	lists   string            // name of the code list table, if any
	skipped map[string]string // assertions left out
}

// message returns the message of an assertion without its leading
// identifier. When the message also holds diagnostics (xsl:value-of
// elements, whose text is empty here), the message proper follows the last
// identifier in brackets.
func message(text, inner string) string {
	if strings.Contains(inner, "<") {
		if loc := reLastID.FindAllStringIndex(text, -1); loc != nil {
			text = text[loc[len(loc)-1][1]:]
		}
	}
	return strings.Join(strings.Fields(rePrefix.ReplaceAllString(text, "")), " ")
}

// codeList returns the list of a code list assertion of the EXTENDED-CTC-FR
// schematron, as written in its test (codes separated by single spaces, with
// a leading and a trailing space), or "" when the test holds none.
func codeList(test string) string {
	if m := reList.FindStringSubmatch(test); m != nil {
		return m[1]
	}
	if m := reMime.FindAllStringSubmatch(test, -1); m != nil {
		var b strings.Builder
		for _, v := range m {
			b.WriteString(" ")
			b.WriteString(v[1])
		}
		return b.String() + " "
	}
	return ""
}

var cenSource = source{
	header: fmt.Sprintf(`// Code generated by internal/rules/en16931/gen; DO NOT EDIT.

// Source: CEN TC 434 EN 16931 validation artefacts for CII, release %s
// (%s, SHA-256 %s),
// file %s.
// The artefacts are licensed under the European Union Public Licence (EUPL)
// version 1.2; the rule identifiers, severities and messages are reproduced
// from them.

package en16931

import "github.com/ritoon/zerechnung/internal/rules"

// Release is the release of the EN 16931 validation artefacts the rules
// implement.
const Release = %q

`, release, url, sum, schFile, release),
	catalog: `// Catalog lists the rules implemented by this package, in the order of the
// official schematron (model rules, CII syntax rules, code lists). Its
// descriptions are the official messages.
var Catalog`,
	nots:    "syntaxNots",
	counts:  "syntaxCounts",
	skipped: skipped,
}

var ctcfrSource = source{
	header: fmt.Sprintf(`// Code generated by internal/rules/en16931/gen; DO NOT EDIT.

// Source: FNFE-MPE France_RFE %s (tag v%s, commit %s),
// file %s
// (SHA-256 %s).
// The schematron is derived from the EN 16931 schematron, licensed under the
// European Union Public Licence (EUPL); the rule identifiers, severities and
// messages are reproduced from it.

package en16931

import (
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
)

// CTCFRRelease is the release of the FNFE-MPE France_RFE package whose
// EXTENDED-CTC-FR rules the package implements.
const CTCFRRelease = %q

`, ctcfrRelease, ctcfrRelease, ctcfrCommit, ctcfrFile, ctcfrSum, ctcfrRelease),
	catalog: `// CTCFRCatalog lists the rules of the EXTENDED-CTC-FR schematron for CII
// (EXTENDED-CTC-FR-CII.sch), which apply to EXTENDED-CTC-FR invoices instead
// of the CEN ones, in the order of the schematron: model rules (the CEN
// rules it keeps, changes or replaces, and its BR-FREXT and BR-FXEXT rules),
// CII syntax rules, code lists, line subtypes and item attribute codes. Its
// descriptions are the messages of the schematron.
var CTCFRCatalog`,
	nots:    "ctcfrNots",
	counts:  "ctcfrCounts",
	lists:   "ctcfrLists",
	skipped: skippedCTCFR,
}

func generate(sch []byte, src source) ([]byte, error) {
	var s schema
	if err := xml.Unmarshal(sch, &s); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(src.header)
	b.WriteString(src.catalog)
	b.WriteString(" = []rules.RuleInfo{\n")
	seen := map[string]bool{}
	n := 0
	var nots, counts, lists bytes.Buffer
	for _, p := range s.Patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				if a.ID == "" {
					m := reID.FindStringSubmatch(a.Text)
					if m == nil {
						return nil, fmt.Errorf("pattern %q: assertion without identifier", p.ID)
					}
					a.ID = m[1]
				}
				if seen[a.ID] {
					return nil, fmt.Errorf("duplicate assertion %s", a.ID)
				}
				seen[a.ID] = true
				if strings.TrimSpace(a.Test) == "true()" || src.skipped[a.ID] != "" {
					continue
				}
				sev := "rules.Error"
				switch a.Flag {
				case "fatal":
				case "warning":
					sev = "rules.Warning"
				default:
					return nil, fmt.Errorf("%s: unknown flag %q", a.ID, a.Flag)
				}
				msg := message(a.Text, a.Inner)
				if src.lists != "" && p.ID != syntaxPattern {
					if l := codeList(a.Test); l != "" {
						fmt.Fprintf(&lists, "\t%q: %q,\n", a.ID, l)
					}
				}
				if p.ID == syntaxPattern {
					if strings.Contains(a.Inner, "<") {
						return nil, fmt.Errorf("%s: the message holds markup", a.ID)
					}
					ctx := strings.Join(strings.Fields(r.Context), " ")
					test := strings.Join(strings.Fields(a.Test), " ")
					if m := reNot.FindStringSubmatch(test); m != nil {
						fmt.Fprintf(&nots, "\t{Rule: %q, Context: %q, Path: %q},\n", a.ID, ctx, m[1])
					} else if m := reCount.FindStringSubmatch(test); m != nil {
						fmt.Fprintf(&counts, "\t{Rule: %q, Context: %q, Path: %q, Exact: %v, N: %s},\n", a.ID, ctx, m[1], m[2] == "=", m[3])
					}
				}
				fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", a.ID, sev, msg)
				n++
			}
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("no assertion found")
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// %s lists the CII syntax assertions not(path): the element or\n// attribute at path, relative to the context, must not exist.\nvar %s = []syntaxNot{\n", src.nots, src.nots)
	b.Write(nots.Bytes())
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// %s lists the CII syntax assertions count(path) <= n and\n// count(path) = n.\nvar %s = []syntaxCount{\n", src.counts, src.counts)
	b.Write(counts.Bytes())
	b.WriteString("}\n")
	if src.lists != "" {
		fmt.Fprintf(&b, "\n// %s maps the code list assertions to their list: the codes\n// separated by single spaces, with a leading and a trailing space. The map\n// is built on first use.\nvar %s = sync.OnceValue(func() map[string]string {\n\treturn map[string]string{\n", src.lists, src.lists)
		b.Write(lists.Bytes())
		b.WriteString("}\n})\n")
	}
	for id := range src.skipped {
		if !seen[id] {
			return nil, fmt.Errorf("skipped assertion %s not found", id)
		}
	}
	log.Printf("%d rules", n)
	return format.Source(b.Bytes())
}
