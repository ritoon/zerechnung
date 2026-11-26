// Command gen generates the rule tables and the catalog of package flux10
// from the schematron Flux10 that the FNFE-MPE publishes for the reports of
// the French e-reporting (flux 10), which the approved platforms send to the
// public invoicing portal (PPF).
//
// It reads Flux10.sch of the France_RFE package 1.4.0.04
// (FNFE_RFE_INVOICE/Flux10), either from a local file (-sch) or downloaded
// from the France_RFE repository at the commit of the release tag, checks
// its SHA-256 and reads its patterns. It writes:
//
//   - patterns: the patterns in schematron order, each with its rule: its
//     context, its variables and its assertions (identifier, flag, test,
//     message);
//   - schematronCatalog: one rules.RuleInfo per assertion, in schematron
//     order.
//
// # Identifiers
//
// The assertions of the schematron have no id attribute: each message opens
// with the code of the management rule of annex 7 of the external
// specifications of the DGFiP that it applies ("[G1.104] "), and the same
// code recurs within a pattern and across patterns (G1.09, the format of a
// date, in 17 patterns). The patterns have identifiers, unique and stable
// ("F10-TRANSMISSION-G1.104"): an assertion takes the identifier of its
// pattern, the second and following assertions of a pattern with a suffix
// -2, -3… in schematron order ("F10-TRANSMISSION-G1.104-2"). The identifier
// names the pattern of the schematron, which a search of the artefact finds,
// and the message keeps the code of annex 7.
//
// # Messages
//
// The messages are the French ones of the schematron, with their code and
// their source ("| Source : Annexe 7 v1.8 G1.104"), as the XSLT of the
// schematron writes them: their name elements (the name of the element
// checked) and value-of elements are written %s, a literal % %%. The
// catalog describes each rule by its message without its source and
// without its values, the name elements replaced by the names of the
// elements that the context of the rule matches.
//
// Usage (from internal/rules/flux10, through go generate):
//
//	go run ./gen [-sch Flux10.sch] [-o rules_gen.go]
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	version  = "1.4.0.04"
	tag      = "v1.4.0.04"
	commit   = "97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d"
	mainFile = "Flux10.sch"
	dir      = "FNFE_RFE_INVOICE/Flux10/"
	url      = "https://raw.githubusercontent.com/fnfempe/France_RFE/" + commit + "/" + dir + mainFile
	sum      = "a7670ca8b522b9b439099b0bd8ac9408ceb6bebefd8fb724350e5a5cc8b07127"

	nsSchematron = "http://purl.oclc.org/dsdl/schematron"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("flux10/gen: ")
	schPath := flag.String("sch", "", "path of "+mainFile+" (downloaded from the France_RFE repository when empty)")
	out := flag.String("o", "rules_gen.go", "output file")
	flag.Parse()

	data, err := load(*schPath)
	if err != nil {
		log.Fatal(err)
	}
	src, err := generate(data)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}

// generate checks the SHA-256 of data, the schematron, and returns the Go
// source of its tables.
func generate(data []byte) ([]byte, error) {
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != sum {
		return nil, fmt.Errorf("%s: SHA-256 %s, want %s", mainFile, got, sum)
	}
	s, err := read(data)
	if err != nil {
		return nil, err
	}
	return s.generate()
}

func load(file string) ([]byte, error) {
	if file != "" {
		return os.ReadFile(file)
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
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// node is an element of the schematron, with its attributes, children and
// content.
type node struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []node     `xml:",any"`
	Inner    []byte     `xml:",innerxml"`
}

func (n *node) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

type let struct{ name, value string }

type assertion struct {
	id, flag, test string
	msg            string   // the message, its name and value-of elements written %s
	values         []string // expressions of its name and value-of elements
	desc           string   // the description of the catalog
}

type rule struct {
	context string
	lets    []let
	asserts []assertion
}

type pattern struct {
	id   string
	rule rule
}

type schematron struct {
	title    string
	patterns []pattern
}

func read(data []byte) (*schematron, error) {
	var root node
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}
	if root.XMLName.Space != nsSchematron || root.XMLName.Local != "schema" {
		return nil, fmt.Errorf("%s: not a schematron", mainFile)
	}
	if qb := root.attr("queryBinding"); qb != "xslt2" {
		return nil, fmt.Errorf("%s: query binding %q, want xslt2", mainFile, qb)
	}
	s := &schematron{}
	for i := range root.Children {
		c := &root.Children[i]
		if c.XMLName.Space != nsSchematron {
			return nil, fmt.Errorf("%s: unexpected element %s", mainFile, c.XMLName.Local)
		}
		switch c.XMLName.Local {
		case "title":
			s.title = strings.TrimSpace(reSpace.ReplaceAllString(string(c.Inner), " "))
		case "pattern":
			p, err := readPattern(c)
			if err != nil {
				return nil, err
			}
			s.patterns = append(s.patterns, p)
		default:
			// ns, let, phase, xsl:function…: the schematron of 1.4.0.04 has
			// none, and the tables could not reproduce them.
			return nil, fmt.Errorf("%s: unexpected element %s", mainFile, c.XMLName.Local)
		}
	}
	if len(s.patterns) == 0 {
		return nil, fmt.Errorf("%s: no pattern", mainFile)
	}
	return s, nil
}

// readPattern reads a pattern, which has an identifier and a single rule.
func readPattern(n *node) (pattern, error) {
	p := pattern{id: n.attr("id")}
	if p.id == "" {
		return p, fmt.Errorf("%s: pattern without identifier", mainFile)
	}
	if len(n.Children) != 1 || n.Children[0].XMLName.Local != "rule" {
		return p, fmt.Errorf("pattern %s: %d children, want a single rule", p.id, len(n.Children))
	}
	it := &n.Children[0]
	r := rule{context: squeeze(it.attr("context"))}
	names := contextNames(r.context)
	for j := range it.Children {
		c := &it.Children[j]
		switch kind := c.XMLName.Local; kind {
		case "let":
			if len(r.asserts) > 0 {
				return p, fmt.Errorf("pattern %s: variable after an assertion", p.id)
			}
			r.lets = append(r.lets, let{c.attr("name"), c.attr("value")})
		case "assert":
			if c.attr("id") != "" {
				return p, fmt.Errorf("pattern %s: assertion with an identifier %s", p.id, c.attr("id"))
			}
			a := assertion{id: p.id, flag: c.attr("flag"), test: c.attr("test")}
			if k := len(r.asserts); k > 0 {
				a.id = fmt.Sprintf("%s-%d", p.id, k+1)
			}
			if a.flag != "fatal" && a.flag != "warning" {
				return p, fmt.Errorf("pattern %s: %s: flag %q", p.id, a.id, a.flag)
			}
			var err error
			if a.msg, a.values, a.desc, err = message(c.Inner, names); err != nil {
				return p, fmt.Errorf("pattern %s: %s: %w", p.id, a.id, err)
			}
			r.asserts = append(r.asserts, a)
		default:
			return p, fmt.Errorf("pattern %s: unexpected element %s in rule %s", p.id, kind, r.context)
		}
	}
	if len(r.asserts) == 0 {
		return p, fmt.Errorf("pattern %s: no assertion", p.id)
	}
	p.rule = r
	return p, nil
}

var (
	reSpace = regexp.MustCompile(`\s+`)
	// reCode is the code of annex 7 that opens a message: "[G1.104] ".
	reCode = regexp.MustCompile(`^\[[A-Z]\d+\.\d+\] `)
	// reSource is the source that ends a message:
	// " | Source : Annexe 7 v1.8 G1.104".
	reSource = regexp.MustCompile(` \| Source : Annexe 7 v[\d.]+ [A-Z]\d+\.\d+$`)
	// reStep is the last step of a path, without its predicates.
	reStep = regexp.MustCompile(`([\w-]+)(\[[^\]]*\])*$`)
)

// squeeze returns an XPath expression with each run of white space outside
// its string literals written as a single space, and trimmed: the literals
// keep their spaces (G1.05 and G1.104 test contains($v, '  '), two spaces).
func squeeze(expr string) string {
	var b strings.Builder
	var quote rune
	space := false
	for _, r := range strings.TrimSpace(expr) {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// contextNames returns the names of the elements that a context matches:
// the last step of each path of the union.
func contextNames(context string) []string {
	var names []string
	for _, path := range strings.Split(context, "|") {
		if m := reStep.FindStringSubmatch(strings.TrimSpace(path)); m != nil && !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	return names
}

// message returns the text of a message, its name and value-of elements
// written %s (a literal % written %%), the expressions of these elements,
// and the description of the catalog: the message without its source and
// its values, each name element replaced by names.
func message(inner []byte, names []string) (msg string, values []string, desc string, err error) {
	d := xml.NewDecoder(bytes.NewReader(append(append([]byte(`<m xmlns="`+nsSchematron+`">`), inner...), "</m>"...)))
	var b, db strings.Builder
	depth := 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			switch t.Name.Local {
			case "name":
				if len(names) == 0 {
					return "", nil, "", errors.New("name element in a rule whose context names no element")
				}
				b.WriteString("\x00")
				values = append(values, "name()")
				db.WriteString(strings.Join(names, " ou "))
			case "value-of":
				sel := ""
				for _, a := range t.Attr {
					if a.Name.Local == "select" {
						sel = a.Value
					}
				}
				if sel == "" {
					return "", nil, "", errors.New("value-of without select")
				}
				b.WriteString("\x00")
				values = append(values, sel)
				db.WriteString("\x00")
			default:
				return "", nil, "", fmt.Errorf("unexpected element %s in a message", t.Name.Local)
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			b.WriteString(strings.ReplaceAll(string(t), "%", "%%"))
			db.Write(t)
		}
	}
	msg = strings.TrimSpace(reSpace.ReplaceAllString(b.String(), " "))
	if !reCode.MatchString(msg) || !reSource.MatchString(msg) {
		return "", nil, "", fmt.Errorf("message %q without a code of annex 7 and its source", msg)
	}
	desc = strings.TrimSpace(reSpace.ReplaceAllString(db.String(), " "))
	desc = reSource.ReplaceAllString(desc, "")
	desc = strings.ReplaceAll(desc, " (\x00)", "")
	if strings.Contains(desc, "\x00") {
		return "", nil, "", fmt.Errorf("message %q: a value outside parentheses", msg)
	}
	if !strings.HasSuffix(desc, ".") {
		desc += "."
	}
	return strings.ReplaceAll(msg, "\x00", "%s"), values, desc, nil
}

func severity(flag string) string {
	if flag == "warning" {
		return "rules.Warning"
	}
	return "rules.Error"
}

func quoteList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(q, ", ")
}

func (s *schematron) generate() ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/rules/flux10/gen; DO NOT EDIT.

// Source: schematron Flux10 of the e-reporting of the FNFE-MPE (%s),
// file %s%s of the France_RFE repository, version %s,
// tag %s (commit %s,
// SHA-256 %s).
// The tests, flags, contexts, variables and messages are reproduced from it;
// the identifiers are those of its patterns (see gen).

package flux10

import "github.com/ritoon/zerechnung/internal/rules"

// Version is the version of the France_RFE package whose schematron the
// package implements.
const Version = %q

// patterns are the patterns of the schematron, in schematron order.
var patterns = []pattern{
`, s.title, dir, mainFile, version, tag, commit, sum, version)
	var cat []assertion
	seen := map[string]bool{}
	for _, p := range s.patterns {
		r := p.rule
		fmt.Fprintf(&b, "\t{ID: %q, Rules: []rule{{Context: %q,", p.id, r.context)
		if len(r.lets) > 0 {
			b.WriteString(" Lets: []let{\n")
			for _, l := range r.lets {
				fmt.Fprintf(&b, "\t\t{Name: %q, Value: %q},\n", l.name, squeeze(l.value))
			}
			b.WriteString("\t},")
		}
		b.WriteString(" Asserts: []assertion{\n")
		for _, a := range r.asserts {
			fmt.Fprintf(&b, "\t\t{ID: %q, Flag: %s, Test: %q, Msg: %q", a.id, severity(a.flag), squeeze(a.test), a.msg)
			if len(a.values) > 0 {
				fmt.Fprintf(&b, ", Values: []string{%s}", quoteList(a.values))
			}
			b.WriteString("},\n")
			if seen[a.id] {
				return nil, fmt.Errorf("%s: two assertions", a.id)
			}
			seen[a.id] = true
			cat = append(cat, a)
		}
		b.WriteString("\t}}}},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// schematronCatalog lists the rules of the schematron, in schematron\n// order, with their messages.\nvar schematronCatalog = []rules.RuleInfo{\n")
	for _, a := range cat {
		fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", a.id, severity(a.flag), a.desc)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// assertionCount is the number of assertions of the patterns.\nconst assertionCount = %d\n", len(cat))
	return format.Source(b.Bytes())
}
