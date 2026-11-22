// Command gen generates the rule catalog and the tables of package ubl from
// the UBL validation artefacts of CEN TC 434 (EN 16931).
//
// It reads the release archive en16931-ubl-<version>.zip, either from a local
// file (-zip) or downloaded from the GitHub release of
// ConnectingEurope/eInvoicing-EN16931, checks its SHA-256 and reads the
// preprocessed schematron
// (schematron/preprocessed/EN16931-UBL-validation-preprocessed.sch). It
// writes:
//
//   - Catalog: one rules.RuleInfo per assertion of the UBL syntax pattern
//     (UBL-CR, UBL-SR and UBL-DT): its identifier, its severity (flag fatal:
//     Error, warning: Warning) and its official message;
//   - crRules: the UBL-CR assertions of the form not(path),
//     not((cac:InvoiceLine|cac:CreditNoteLine)/path) and not(//path), the
//     elements and attributes that EN 16931 does not use; the few other
//     UBL-CR assertions are written by hand in package ubl (see crSpecial);
//   - the code lists of the UBL artefacts that differ from the CII ones:
//     the document type codes of an invoice and of a credit note (BR-CL-01)
//     and the VAT point date codes (BR-CL-06), and the exact list string of
//     BR-CL-08, whose test looks for a substring.
//
// The other code lists of the UBL artefacts must be those of package
// internal/codes (generated from the CII artefacts of the same release): gen
// checks that each code of each list is accepted by internal/codes.
//
// With -ctcfr, it reads the schematron EXTENDED-CTC-FR-UBL.sch of the
// France_RFE package 1.4.0.04 of the FNFE-MPE
// (FNFE_RFE_INVOICE/UBL/EXTENDED-CTC-FR/schematron), either from -src (a
// checkout of github.com/fnfempe/France_RFE) or downloaded from the
// repository at the commit of the release tag, checks its SHA-256 and writes
// ctcfr_gen.go:
//
//   - CTCFRCatalog: one rules.RuleInfo per assertion of the schematron, in
//     its order (model rules, UBL syntax rules, code lists, line subtypes and
//     item attribute codes). A message that also holds diagnostics
//     (xsl:value-of) is the text after its last identifier in brackets. The
//     assertions commented out in the file are not read;
//   - ctcfrCRRules: the UBL-CR assertions not(path) of the schematron, as
//     crRules for the CEN artefacts (the same assertions are written by
//     hand, see crSpecial);
//   - ctcfrLists: the list of each assertion whose test looks a value up in a
//     list (contains(' A B ', ...), or the MIME codes of BR-FREXT-CL-24), the
//     codes separated by single spaces with a leading and a trailing space.
//     BR-CL-01 has a list for each document type (BR-CL-01 for an Invoice,
//     BR-CL-01/credit for a CreditNote).
//
// Usage (from internal/rules/ubl, through go generate):
//
//	go run ./gen [-zip en16931-ubl-1.3.17.zip] [-o catalog_gen.go]
//	go run ./gen -ctcfr [-src France_RFE] [-o ctcfr_gen.go]
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

	"github.com/ritoon/zerechnung/internal/codes"
)

const (
	release = "validation-1.3.17"
	archive = "en16931-ubl-1.3.17.zip"
	url     = "https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/" + release + "/" + archive
	sum     = "bf02a0e3f6cbe692a4a28e70be7bdcaded5612b4169b543e13d6cb4f33c29e92"

	schFile = "schematron/preprocessed/EN16931-UBL-validation-preprocessed.sch"
)

// The EXTENDED-CTC-FR schematron for UBL of the France_RFE package (the
// release of en16931.CTCFRRelease).
const (
	ctcfrRelease = "1.4.0.04"
	ctcfrCommit  = "97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d" // tag v1.4.0.04
	ctcfrFile    = "FNFE_RFE_INVOICE/UBL/EXTENDED-CTC-FR/schematron/EXTENDED-CTC-FR-UBL.sch"
	ctcfrURL     = "https://raw.githubusercontent.com/fnfempe/France_RFE/" + ctcfrCommit + "/" + ctcfrFile
	ctcfrSum     = "ba8f8f2611b950519cc1eb88698dec2b1dbf91f7495f55d4b682540f99e6077e"
)

// crSpecial lists the UBL-CR assertions that are not a plain not(path): they
// are implemented by hand.
var crSpecial = map[string]bool{
	"UBL-CR-002": true, // not(cbc:UBLVersionID) or cbc:UBLVersionID = '2.1'
	"UBL-CR-412": true, // not(cac:PaymentMeans/cbc:PaymentDueDate) or ../cn:CreditNote
	"UBL-CR-665": true, // AdditionalDocumentReference of type other than 130 with a schemeID
	"UBL-CR-666": true, // AdditionalDocumentReference of type 130 with an attachment
	"UBL-CR-673": true, // AdditionalDocumentReference of type 130 with a description
}

// codeLists maps the BR-CL rules whose lists are those of internal/codes to
// the lookup function.
var codeLists = map[string]func(string) bool{
	"BR-CL-03": codes.Currency, "BR-CL-04": codes.Currency, "BR-CL-05": codes.Currency,
	"BR-CL-07": codes.ReferenceType, "BR-CL-10": codes.ICD, "BR-CL-11": codes.ICD,
	"BR-CL-13": codes.ItemClassification, "BR-CL-14": codes.Country, "BR-CL-15": codes.Country,
	"BR-CL-16": codes.PaymentMeans, "BR-CL-17": codes.VATCategory, "BR-CL-18": codes.VATCategory,
	"BR-CL-19": codes.AllowanceReason, "BR-CL-20": codes.ChargeReason, "BR-CL-21": codes.ICD,
	"BR-CL-22": codes.VATEX, "BR-CL-23": codes.Unit, "BR-CL-25": codes.EAS, "BR-CL-26": codes.ICD,
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("ubl/gen: ")
	zipPath := flag.String("zip", "", "path of "+archive+" (downloaded from GitHub when empty)")
	ctcfr := flag.Bool("ctcfr", false, "generate the EXTENDED-CTC-FR rules from the FNFE-MPE schematron")
	srcPath := flag.String("src", "", "with -ctcfr: a checkout of France_RFE (the schematron is downloaded when empty)")
	out := flag.String("o", "", "output file (default catalog_gen.go, ctcfr_gen.go with -ctcfr)")
	flag.Parse()

	if *ctcfr {
		if *zipPath != "" {
			flag.Usage()
			os.Exit(2)
		}
		if *out == "" {
			*out = "ctcfr_gen.go"
		}
		src, err := generateCTCFR(*srcPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*out, src, 0o644); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *srcPath != "" {
		flag.Usage()
		os.Exit(2)
	}
	if *out == "" {
		*out = "catalog_gen.go"
	}
	data, err := load(*zipPath)
	if err != nil {
		log.Fatal(err)
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != sum {
		log.Fatalf("%s: SHA-256 %s, want %s", archive, got, sum)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Fatal(err)
	}
	sch, err := readFile(zr, schFile)
	if err != nil {
		log.Fatal(err)
	}
	src, err := generate(sch)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}

func load(path string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
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

const pathRE = `(?:cac|cbc|ext):[A-Za-z0-9]+(?:/(?:cac|cbc|ext):[A-Za-z0-9]+)*(?:/@[A-Za-z]+)?`

var (
	rePrefix = regexp.MustCompile(`^\s*\[[^\]]+\]\s*-?\s*`)
	reRoot   = regexp.MustCompile(`^not\((` + pathRE + `)\)$`)
	reLine   = regexp.MustCompile(`^not\(\(cac:InvoiceLine\|cac:CreditNoteLine\)/(` + pathRE + `)\)$`)
	reAny    = regexp.MustCompile(`^not\(//(` + pathRE + `|@[A-Za-z]+)\)$`)
	reList   = regexp.MustCompile(`'( [^']+ )'`)
)

const (
	syntaxPattern = "UBL-syntax"
	rootContext   = "/ubl:Invoice | /cn:CreditNote"
)

func generate(sch []byte) ([]byte, error) {
	var s schema
	if err := xml.Unmarshal(sch, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", schFile, err)
	}
	var b, cr bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/rules/ubl/gen; DO NOT EDIT.

// Source: CEN TC 434 EN 16931 validation artefacts for UBL, release %s
// (%s, SHA-256 %s),
// file %s.
// The artefacts are licensed under the European Union Public Licence (EUPL)
// version 1.2; the rule identifiers, severities, messages and code lists are
// reproduced from them.

package ubl

import "github.com/ritoon/zerechnung/internal/rules"

// Release is the release of the EN 16931 validation artefacts the rules
// implement.
const Release = %q

// Catalog lists the UBL syntax rules implemented by this package (UBL-CR,
// UBL-SR and UBL-DT), in the order of the official schematron. Its
// descriptions are the official messages. The business rules and code
// lists, whose identifiers are the same in CII and UBL, are in the catalog
// of package en16931.
var Catalog = []rules.RuleInfo{
`, release, url, sum, schFile, release)
	seen := map[string]bool{}
	special := map[string]bool{}
	lists := map[string]string{}
	n := 0
	for _, p := range s.Patterns {
		for _, r := range p.Rules {
			ctx := strings.Join(strings.Fields(r.Context), " ")
			for _, a := range r.Asserts {
				if seen[a.ID] {
					return nil, fmt.Errorf("duplicate assertion %s", a.ID)
				}
				seen[a.ID] = true
				test := strings.Join(strings.Fields(a.Test), " ")
				if strings.HasPrefix(a.ID, "BR-CL-") {
					if err := codeList(a.ID, test, lists); err != nil {
						return nil, err
					}
					continue
				}
				if p.ID != syntaxPattern {
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
				msg := strings.Join(strings.Fields(rePrefix.ReplaceAllString(a.Text, "")), " ")
				fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", a.ID, sev, msg)
				n++
				if !strings.HasPrefix(a.ID, "UBL-CR-") {
					continue
				}
				if ctx != rootContext {
					return nil, fmt.Errorf("%s: context %q, want %q", a.ID, ctx, rootContext)
				}
				if crSpecial[a.ID] {
					special[a.ID] = true
					continue
				}
				var scope, path string
				if m := reRoot.FindStringSubmatch(test); m != nil {
					scope, path = "crRoot", m[1]
				} else if m := reLine.FindStringSubmatch(test); m != nil {
					scope, path = "crLine", m[1]
				} else if m := reAny.FindStringSubmatch(test); m != nil {
					scope, path = "crAny", m[1]
				} else {
					return nil, fmt.Errorf("%s: unexpected test %q (add it to crSpecial and implement it)", a.ID, test)
				}
				fmt.Fprintf(&cr, "\t{%q, %s, %q},\n", a.ID, scope, path)
			}
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// crRules lists the UBL-CR assertions not(path) (scope crRoot),\n// not((cac:InvoiceLine|cac:CreditNoteLine)/path) (crLine) and not(//path)\n// (crAny): the element or attribute at path must not exist.\nvar crRules = []crRule{\n")
	b.Write(cr.Bytes())
	b.WriteString("}\n\n")
	for id := range crSpecial {
		if !special[id] {
			return nil, fmt.Errorf("special assertion %s not found", id)
		}
	}
	for _, c := range []struct{ name, key, doc string }{
		{"invoiceTypeCodes", "BR-CL-01/invoice", "the document type codes (BT-3) of an Invoice (BR-CL-01)"},
		{"creditNoteTypeCodes", "BR-CL-01/credit", "the document type codes (BT-3) of a CreditNote (BR-CL-01)"},
		{"taxPointDateCodes", "BR-CL-06", "the VAT point date codes (BT-8) of UBL (BR-CL-06)"},
		{"noteSubjectCodes", "BR-CL-08", "the note subject codes (BT-21) of BR-CL-08, in the order of the\n// artefacts: the rule looks for the code as a substring of this list"},
	} {
		l, ok := lists[c.key]
		if !ok {
			return nil, fmt.Errorf("no list for %s", c.key)
		}
		fmt.Fprintf(&b, "// %s lists %s.\nconst %s = %q\n\n", c.name, c.doc, c.name, l)
	}
	log.Printf("%d rules", n)
	return format.Source(b.Bytes())
}

// codeList records the lists of the code list rules that package ubl needs,
// and checks that the others are the lists of internal/codes.
func codeList(id, test string, lists map[string]string) error {
	found := reList.FindAllStringSubmatch(test, -1)
	switch id {
	case "BR-CL-01":
		if len(found) != 2 || !strings.Contains(test, "self::cbc:InvoiceTypeCode") {
			return fmt.Errorf("%s: unexpected test %q", id, test)
		}
		lists["BR-CL-01/invoice"], lists["BR-CL-01/credit"] = found[0][1], found[1][1]
	case "BR-CL-06", "BR-CL-08":
		if len(found) != 1 {
			return fmt.Errorf("%s: unexpected test %q", id, test)
		}
		lists[id] = found[0][1]
	case "BR-CL-24":
		// A chain of @mimeCode = '…' comparisons: the MIME codes of
		// internal/codes.
		for _, m := range regexp.MustCompile(`@mimeCode = '([^']+)'`).FindAllStringSubmatch(test, -1) {
			if !codes.MIME(m[1]) {
				return fmt.Errorf("%s: MIME code %q not in internal/codes", id, m[1])
			}
		}
	default:
		ok := codeLists[id]
		if ok == nil {
			return fmt.Errorf("%s: unknown code list rule", id)
		}
		if len(found) == 0 {
			return fmt.Errorf("%s: no list in %q", id, test)
		}
		for _, c := range strings.Fields(found[0][1]) {
			if !ok(c) {
				return fmt.Errorf("%s: code %q not in internal/codes", id, c)
			}
		}
	}
	return nil
}

// The last rule identifier in brackets of a message that also holds
// diagnostics (xsl:value-of): the text after it is the message proper.
var reLastID = regexp.MustCompile(`\[[A-Z]+-[^\]]*\]\s*(?:warning\s*)?-?\s*`)

// reAnyList matches the list of a test contains(' A B ', ...).
var reAnyList = regexp.MustCompile(`contains\(\s*'( [^']* )'\s*,`)

// reMime matches the values of the MIME code test of BR-FREXT-CL-24.
var reMime = regexp.MustCompile(`@mimeCode\s*=\s*'([^']*)'`)

// ctcfrSpecial lists the UBL-CR assertions of the EXTENDED-CTC-FR schematron
// that are not a plain not(path): those of the CEN artefacts (crSpecial).
var ctcfrSpecial = crSpecial

// readCTCFR returns the EXTENDED-CTC-FR schematron for UBL read from the
// checkout dir, or downloaded when dir is empty, after checking its SHA-256.
func readCTCFR(dir string) ([]byte, error) {
	var data []byte
	var err error
	if dir == "" {
		client := &http.Client{Timeout: 2 * time.Minute}
		resp, err := client.Get(ctcfrURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: %s", ctcfrURL, resp.Status)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, err
		}
	} else if data, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(ctcfrFile))); err != nil {
		return nil, err
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != ctcfrSum {
		return nil, fmt.Errorf("%s: SHA-256 %s, want %s", filepath.Base(ctcfrFile), got, ctcfrSum)
	}
	return data, nil
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

// generateCTCFR returns the source of ctcfr_gen.go.
func generateCTCFR(dir string) ([]byte, error) {
	sch, err := readCTCFR(dir)
	if err != nil {
		return nil, err
	}
	var s schema
	if err := xml.Unmarshal(sch, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", ctcfrFile, err)
	}
	var b, cr, lists bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/rules/ubl/gen; DO NOT EDIT.

// Source: FNFE-MPE France_RFE %s (tag v%s, commit %s),
// file %s
// (SHA-256 %s).
// The schematron is derived from the EN 16931 schematron, licensed under the
// European Union Public Licence (EUPL); the rule identifiers, severities,
// messages and code lists are reproduced from it.

package ubl

import "github.com/ritoon/zerechnung/internal/rules"

// CTCFRCatalog lists the rules of the EXTENDED-CTC-FR schematron for UBL
// (EXTENDED-CTC-FR-UBL.sch), which apply to EXTENDED-CTC-FR invoices in UBL
// instead of the CEN ones, in the order of the schematron: model rules (the
// CEN rules it keeps, changes or replaces, and its BR-FREXT rules), UBL
// syntax rules, code lists, item attribute codes and line subtypes. Its
// descriptions are the messages of the schematron. Package en16931 reports
// the CEN model rules that the schematron keeps; this package checks the
// others.
var CTCFRCatalog = []rules.RuleInfo{
`, ctcfrRelease, ctcfrRelease, ctcfrCommit, ctcfrFile, ctcfrSum)
	seen := map[string]bool{}
	special := map[string]bool{}
	n := 0
	for _, p := range s.Patterns {
		for _, r := range p.Rules {
			ctx := strings.Join(strings.Fields(r.Context), " ")
			for _, a := range r.Asserts {
				if a.ID == "" {
					return nil, fmt.Errorf("pattern %q: assertion without identifier", p.ID)
				}
				if seen[a.ID] {
					return nil, fmt.Errorf("duplicate assertion %s", a.ID)
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
				fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", a.ID, sev, message(a.Text, a.Inner))
				n++
				test := strings.Join(strings.Fields(a.Test), " ")
				if p.ID != syntaxPattern {
					if a.ID == "BR-CL-01" {
						found := reAnyList.FindAllStringSubmatch(test, -1)
						if len(found) != 2 || !strings.Contains(test, "self::cbc:InvoiceTypeCode") {
							return nil, fmt.Errorf("%s: unexpected test %q", a.ID, test)
						}
						fmt.Fprintf(&lists, "\t%q: %q,\n\t%q: %q,\n", a.ID, found[0][1], a.ID+"/credit", found[1][1])
					} else if m := reAnyList.FindStringSubmatch(test); m != nil {
						fmt.Fprintf(&lists, "\t%q: %q,\n", a.ID, m[1])
					} else if m := reMime.FindAllStringSubmatch(test, -1); m != nil {
						var l strings.Builder
						for _, v := range m {
							l.WriteString(" " + v[1])
						}
						fmt.Fprintf(&lists, "\t%q: %q,\n", a.ID, l.String()+" ")
					}
					continue
				}
				if !strings.HasPrefix(a.ID, "UBL-CR-") {
					continue
				}
				if ctx != rootContext {
					return nil, fmt.Errorf("%s: context %q, want %q", a.ID, ctx, rootContext)
				}
				if ctcfrSpecial[a.ID] {
					special[a.ID] = true
					continue
				}
				var scope, path string
				if m := reRoot.FindStringSubmatch(test); m != nil {
					scope, path = "crRoot", m[1]
				} else if m := reLine.FindStringSubmatch(test); m != nil {
					scope, path = "crLine", m[1]
				} else if m := reAny.FindStringSubmatch(test); m != nil {
					scope, path = "crAny", m[1]
				} else {
					return nil, fmt.Errorf("%s: unexpected test %q (add it to ctcfrSpecial and implement it)", a.ID, test)
				}
				fmt.Fprintf(&cr, "\t{%q, %s, %q},\n", a.ID, scope, path)
			}
		}
	}
	for id := range ctcfrSpecial {
		if !special[id] {
			return nil, fmt.Errorf("special assertion %s not found", id)
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// ctcfrCRRules lists the UBL-CR assertions not(path) of the EXTENDED-CTC-FR\n// schematron, with the scopes of crRules.\nvar ctcfrCRRules = []crRule{\n")
	b.Write(cr.Bytes())
	b.WriteString("}\n\n")
	b.WriteString("// ctcfrLists maps the assertions of the EXTENDED-CTC-FR schematron that look a\n// value up in a list to the list: the codes separated by single spaces, with\n// a leading and a trailing space.\nvar ctcfrLists = map[string]string{\n")
	b.Write(lists.Bytes())
	b.WriteString("}\n")
	log.Printf("%d rules", n)
	return format.Source(b.Bytes())
}
