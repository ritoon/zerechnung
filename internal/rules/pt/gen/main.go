// Command gen generates the rule tables and catalogs of package pt from the
// CIUS-PT schematron of eSPap (Entidade de Serviços Partilhados da
// Administração Pública).
//
// It reads the archive urn_feap.gov.pt_cius-pt_schematron.zip, either from a
// local file (-zip) or downloaded from the page "Normas" of eSPap, checks its
// SHA-256 and reads the schematron urn_feap.gov.pt_CIUS-PT_2.1.1.sch with the
// files it includes. The abstract patterns (abstract/*.sch) are instantiated
// with the parameters of their UBL patterns (UBL/*.sch), as an ISO
// schematron processor does: each $name of a context, a variable or a test
// is replaced by the value of the parameter name, the longest names first.
// It writes:
//
//   - namespaces: the prefixes the schematron declares;
//   - patterns: the four patterns of the schematron (UBL-syntax, UBL-model,
//     UBL-datatype, UBL-condition) in the order of the schema, each with its
//     variables and its rules, each rule with its context, its variables and
//     its assertions (identifier, flag, test, message), in schematron order;
//   - Catalog and CENCatalog: one rules.RuleInfo per assertion identifier,
//     those of the CIUS-PT (BR-CIUS-PT-*, DT-CIUS-PT-*) and the others (the
//     UBL syntax rules and the rules of EN 16931 as the CIUS-PT restates
//     them), with the official message.
//
// Usage (from internal/rules/pt, through go generate):
//
//	go run ./gen [-zip urn_feap.gov.pt_cius-pt_schematron.zip] [-o rules_gen.go]
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
	"net/http/cookiejar"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	version = "2.1.1"
	archive = "urn_feap.gov.pt_cius-pt_schematron.zip"
	page    = "https://www.espap.gov.pt/spfin/normas/Paginas/normas.aspx"
	url     = "https://www.espap.gov.pt/Imagens/Documento.ashx?id=267"
	sum     = "89e908d2290d361a3ed9fc5c3dd5c7a661da249d4a3c4515b53a8f4b8cb6d005"

	mainFile = "urn_feap.gov.pt_CIUS-PT_" + version + ".sch"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("pt/gen: ")
	zipPath := flag.String("zip", "", "path of "+archive+" (downloaded from eSPap when empty)")
	out := flag.String("o", "rules_gen.go", "output file")
	flag.Parse()

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
	s, err := read(zr)
	if err != nil {
		log.Fatal(err)
	}
	src, err := s.generate()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}

// load reads the archive from path, or downloads it: eSPap serves its
// documents to a client that first opened the page that links them, with
// the cookies of that page.
func load(file string) ([]byte, error) {
	if file != "" {
		return os.ReadFile(file)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute, Jar: jar}
	for _, u := range []string{page, url} {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
		}
		if u == url {
			return body, nil
		}
	}
	panic("unreachable")
}

// Elements of the schematron files.
type (
	schemaXML struct {
		NS []struct {
			Prefix string `xml:"prefix,attr"`
			URI    string `xml:"uri,attr"`
		} `xml:"ns"`
		Includes []struct {
			Href string `xml:"href,attr"`
		} `xml:"include"`
	}
	patternXML struct {
		ID       string     `xml:"id,attr"`
		Abstract string     `xml:"abstract,attr"`
		IsA      string     `xml:"is-a,attr"`
		Params   []paramXML `xml:"param"`
		Items    []itemXML  `xml:",any"`
	}
	paramXML struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	// itemXML is a child of a pattern: a let or a rule.
	itemXML struct {
		XMLName xml.Name
		Name    string     `xml:"name,attr"`
		Value   string     `xml:"value,attr"`
		Context string     `xml:"context,attr"`
		Lets    []letXML   `xml:"let"`
		Checks  []checkXML `xml:",any"`
	}
	letXML struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	checkXML struct {
		XMLName xml.Name
		ID      string `xml:"id,attr"`
		Flag    string `xml:"flag,attr"`
		Test    string `xml:"test,attr"`
		Text    string `xml:",chardata"`
	}
)

type let struct{ name, value string }

type assertion struct {
	id, flag, test, msg string
	report              bool
}

type rule struct {
	context string
	lets    []let
	asserts []assertion
}

type pattern struct {
	id    string
	lets  []let
	rules []rule
}

type schematron struct {
	ns       [][2]string
	patterns []pattern
}

func read(zr *zip.Reader) (*schematron, error) {
	files := map[string][]byte{}
	dir := ""
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = b
		if path.Base(f.Name) == mainFile {
			dir = path.Dir(f.Name)
		}
	}
	mainData, ok := files[path.Join(dir, mainFile)]
	if !ok {
		return nil, fmt.Errorf("%s: not in %s", mainFile, archive)
	}
	var sx schemaXML
	if err := xml.Unmarshal(mainData, &sx); err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}
	s := &schematron{}
	for _, n := range sx.NS {
		s.ns = append(s.ns, [2]string{n.Prefix, n.URI})
	}
	abstract := map[string]patternXML{}
	for _, inc := range sx.Includes {
		name := path.Join(dir, inc.Href)
		b, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("%s: not in %s", inc.Href, archive)
		}
		var p patternXML
		if err := xml.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", inc.Href, err)
		}
		switch {
		case p.Abstract == "true":
			abstract[p.ID] = p
		case p.IsA != "":
			a, ok := abstract[p.IsA]
			if !ok {
				return nil, fmt.Errorf("%s: abstract pattern %s not included before", inc.Href, p.IsA)
			}
			params := map[string]string{}
			for _, pr := range p.Params {
				params[pr.Name] = pr.Value
			}
			pat, err := instantiate(p.ID, a, params)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", inc.Href, err)
			}
			s.patterns = append(s.patterns, pat)
		default:
			pat, err := instantiate(p.ID, p, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", inc.Href, err)
			}
			s.patterns = append(s.patterns, pat)
		}
	}
	return s, nil
}

// substitute replaces each $name of s by the value of the parameter name,
// the longest names first; a name must not be followed by a character of a
// name (so that $BR-CO-1 is not a prefix of $BR-CO-10).
func substitute(s string, params map[string]string, names []string) string {
	if len(params) == 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		rest := s[i+1:]
		done := false
		for _, n := range names {
			if !strings.HasPrefix(rest, n) {
				continue
			}
			if len(rest) > len(n) && isNameChar(rest[len(n)]) {
				continue
			}
			b.WriteString(params[n])
			i += len(n)
			done = true
			break
		}
		if !done {
			b.WriteByte('$')
		}
	}
	return b.String()
}

func isNameChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

var reSpace = regexp.MustCompile(`\s+`)

func instantiate(id string, p patternXML, params map[string]string) (pattern, error) {
	names := make([]string, 0, len(params))
	for n := range params {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	sub := func(s string) string { return substitute(s, params, names) }
	out := pattern{id: id}
	for _, it := range p.Items {
		switch it.XMLName.Local {
		case "let":
			out.lets = append(out.lets, let{it.Name, sub(it.Value)})
		case "rule":
			r := rule{context: sub(it.Context)}
			for _, l := range it.Lets {
				r.lets = append(r.lets, let{l.Name, sub(l.Value)})
			}
			for _, c := range it.Checks {
				kind := c.XMLName.Local
				if kind != "assert" && kind != "report" {
					continue
				}
				if c.ID == "" {
					return out, fmt.Errorf("pattern %s: %s without id in rule %s", id, kind, it.Context)
				}
				test := sub(c.Test)
				switch c.Flag {
				case "fatal", "warning":
				default:
					return out, fmt.Errorf("pattern %s: %s: flag %q", id, c.ID, c.Flag)
				}
				r.asserts = append(r.asserts, assertion{
					id: c.ID, flag: c.Flag, test: test, report: kind == "report",
					msg: strings.TrimSpace(reSpace.ReplaceAllString(c.Text, " ")),
				})
			}
			out.rules = append(out.rules, r)
		case "param", "title", "p":
		default:
			return out, fmt.Errorf("pattern %s: unexpected element %s", id, it.XMLName.Local)
		}
	}
	return out, nil
}

// rePrefix is the identifier that starts an official message:
// "[BR-CIUS-PT-01]-".
var rePrefix = regexp.MustCompile(`^\[[^\]]+\]\s*-?\s*`)

func severity(flag string) string {
	if flag == "warning" {
		return "rules.Warning"
	}
	return "rules.Error"
}

// national reports whether an assertion is a rule of the CIUS-PT itself.
func national(id string) bool { return strings.Contains(id, "CIUS-PT") }

func (s *schematron) generate() ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/rules/pt/gen; DO NOT EDIT.

// Source: CIUS-PT %s validation artefacts of eSPap, %s
// (%s, SHA-256 %s),
// file %s and the files it includes. The rule identifiers, flags, tests
// and messages are reproduced from them.

package pt

import "github.com/ritoon/zerechnung/internal/rules"

// Version is the version of the CIUS-PT whose schematron the rules
// implement.
const Version = %q

// namespaces are the prefixes the schematron declares.
var namespaces = map[string]string{
`, version, page, archive, sum, mainFile, version)
	for _, n := range s.ns {
		fmt.Fprintf(&b, "\t%q: %q,\n", n[0], n[1])
	}
	b.WriteString("}\n\n// patterns are the patterns of the schematron, in the order of the schema.\nvar patterns = []pattern{\n")
	type entry struct{ id, flag, msg string }
	var cat, cen []entry
	seen := map[string]bool{}
	total := 0
	for _, p := range s.patterns {
		fmt.Fprintf(&b, "\t{ID: %q,", p.id)
		if len(p.lets) > 0 {
			b.WriteString(" Lets: []let{")
			for _, l := range p.lets {
				fmt.Fprintf(&b, "{%q, %q}, ", l.name, l.value)
			}
			b.WriteString("},")
		}
		b.WriteString(" Rules: []rule{\n")
		for _, r := range p.rules {
			fmt.Fprintf(&b, "\t\t{Context: %q,", r.context)
			if len(r.lets) > 0 {
				b.WriteString(" Lets: []let{")
				for _, l := range r.lets {
					fmt.Fprintf(&b, "{%q, %q}, ", l.name, l.value)
				}
				b.WriteString("},")
			}
			b.WriteString(" Asserts: []assertion{\n")
			for _, a := range r.asserts {
				total++
				msg := rePrefix.ReplaceAllString(a.msg, "")
				report := ""
				if a.report {
					report = " Report: true,"
				}
				fmt.Fprintf(&b, "\t\t\t{ID: %q, Flag: %s,%s Test: %q, Msg: %q},\n",
					a.id, severity(a.flag), report, a.test, msg)
				if seen[a.id] {
					continue
				}
				seen[a.id] = true
				e := entry{a.id, a.flag, msg}
				if national(a.id) {
					cat = append(cat, e)
				} else {
					cen = append(cen, e)
				}
			}
			b.WriteString("\t\t}},\n")
		}
		b.WriteString("\t}},\n")
	}
	b.WriteString("}\n\n")
	writeCatalog := func(name, doc string, es []entry) {
		b.WriteString(doc)
		fmt.Fprintf(&b, "var %s = []rules.RuleInfo{\n", name)
		for _, e := range es {
			fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", e.id, severity(e.flag), e.msg)
		}
		b.WriteString("}\n\n")
	}
	writeCatalog("schematronCatalog", "// schematronCatalog lists the rules of the CIUS-PT itself (BR-CIUS-PT,\n// DT-CIUS-PT), in schematron order, with their official messages.\n", cat)
	writeCatalog("CENCatalog", "// CENCatalog lists the other rules of the CIUS-PT schematron, in\n// schematron order, with their official messages: the UBL syntax rules\n// (UBL-CR, UBL-SR, UBL-DT) and the rules of EN 16931 as the CIUS-PT restates\n// them. CheckCEN applies them, in the EN 16931 rule set, instead of the\n// rules of packages en16931 and ubl.\n", cen)
	fmt.Fprintf(&b, "// assertionCount is the number of assertions of the schematron.\nconst assertionCount = %d\n", total)
	return format.Source(b.Bytes())
}
