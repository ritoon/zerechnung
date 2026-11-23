// Command gen generates the rule tables and the catalog of package cdv from
// the schematron BR-FR-CDV of the FNFE-MPE, which checks the statuses of the
// life cycle of the French invoices (CDV, in the CDAR syntax) exchanged
// between the approved platforms and sent to the public invoicing portal
// (PPF).
//
// It reads BR-FR-CDV-Schematron-CDAR.sch of the France_RFE package 1.4.0.04
// (FNFE_RFE_INVOICE/CDAR/schematron), the fatal version, either from a
// local file (-sch) or downloaded from the France_RFE repository at the
// commit of the release tag, checks its SHA-256 and reads its namespace
// prefixes, its functions (xsl:function, among which the closed lists of
// the status codes, of the reasons and of the requested actions) and its
// patterns. It writes:
//
//   - namespaces: the prefixes the schematron declares;
//   - functions: the functions that the rules call, directly or through
//     another function, with their parameters, variables and result (the
//     schematron also declares functions of the schematron of the
//     invoices, on dates, identifiers and amounts, that none of its rules
//     calls: they are left out);
//   - patterns: the patterns in schematron order, with their rules, each
//     rule with its context, its variables and its assertions (identifier,
//     flag, test, message);
//   - schematronCatalog: one rules.RuleInfo per identifier, in schematron
//     order.
//
// The messages of the schematron are in French, preceded by the identifier
// of the rule ("[BR-FR-CDV-01/MDG-3] : "): each assertion keeps its message
// without that prefix, its value-of elements written %s. The catalog
// describes each rule by its message without the values and without the
// advice that ends it ("Veuillez…").
//
// Usage (from internal/rules/cdv, through go generate):
//
//	go run ./gen [-sch BR-FR-CDV-Schematron-CDAR.sch] [-o rules_gen.go]
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
	"strings"
	"time"
)

const (
	version  = "1.4.0.04"
	tag      = "v1.4.0.04"
	commit   = "97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d"
	mainFile = "BR-FR-CDV-Schematron-CDAR.sch"
	dir      = "FNFE_RFE_INVOICE/CDAR/schematron/"
	url      = "https://raw.githubusercontent.com/fnfempe/France_RFE/" + commit + "/" + dir + mainFile
	sum      = "3530c14fb95e0e701faf10beec38fedee6ed324dc2761f40bc855a9a060b090c"

	nsSchematron = "http://purl.oclc.org/dsdl/schematron"
	nsXSL        = "http://www.w3.org/1999/XSL/Transform"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("cdv/gen: ")
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

type param struct{ name, as string }

type function struct {
	name, as string
	params   []param
	lets     []let
	result   string
}

type assertion struct {
	id, flag, test string
	msg            string   // the message, without its identifier, its values written %s
	values         []string // expressions of its value-of elements
	report         bool
}

type rule struct {
	context string
	lets    []let
	asserts []assertion
}

type pattern struct {
	id, title string
	rules     []rule
}

type schematron struct {
	ns       [][2]string
	funcs    []function
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
	s := &schematron{}
	for i := range root.Children {
		c := &root.Children[i]
		switch {
		case c.XMLName.Space == nsXSL && c.XMLName.Local == "function":
			f, err := readFunction(c)
			if err != nil {
				return nil, err
			}
			s.funcs = append(s.funcs, f)
		case c.XMLName.Space != nsSchematron:
			return nil, fmt.Errorf("%s: unexpected element %s", mainFile, c.XMLName.Local)
		case c.XMLName.Local == "ns":
			s.ns = append(s.ns, [2]string{c.attr("prefix"), c.attr("uri")})
		case c.XMLName.Local == "pattern":
			if c.attr("id") == "" {
				return nil, fmt.Errorf("%s: pattern without identifier", mainFile)
			}
			p, err := readPattern(c.attr("id"), c)
			if err != nil {
				return nil, err
			}
			s.patterns = append(s.patterns, p)
		case c.XMLName.Local == "title":
		default:
			return nil, fmt.Errorf("%s: unexpected element %s", mainFile, c.XMLName.Local)
		}
	}
	if len(s.patterns) == 0 {
		return nil, fmt.Errorf("%s: no pattern", mainFile)
	}
	return s, nil
}

func readFunction(n *node) (function, error) {
	f := function{name: n.attr("name"), as: n.attr("as")}
	for i := range n.Children {
		c := &n.Children[i]
		if c.XMLName.Space != nsXSL {
			return f, fmt.Errorf("function %s: unexpected element %s", f.name, c.XMLName.Local)
		}
		switch c.XMLName.Local {
		case "param":
			if f.lets != nil || f.result != "" {
				return f, fmt.Errorf("function %s: parameter after the body", f.name)
			}
			f.params = append(f.params, param{c.attr("name"), c.attr("as")})
		case "variable":
			if f.result != "" {
				return f, fmt.Errorf("function %s: variable after the result", f.name)
			}
			f.lets = append(f.lets, let{c.attr("name"), c.attr("select")})
		case "sequence":
			if f.result != "" {
				return f, fmt.Errorf("function %s: two results", f.name)
			}
			f.result = c.attr("select")
		default:
			return f, fmt.Errorf("function %s: unexpected element %s", f.name, c.XMLName.Local)
		}
	}
	if f.name == "" || f.result == "" {
		return f, fmt.Errorf("function %q without a name or a result", f.name)
	}
	return f, nil
}

func readPattern(id string, n *node) (pattern, error) {
	p := pattern{id: id}
	for i := range n.Children {
		it := &n.Children[i]
		switch it.XMLName.Local {
		case "rule":
			r := rule{context: it.attr("context")}
			for j := range it.Children {
				c := &it.Children[j]
				switch kind := c.XMLName.Local; kind {
				case "let":
					r.lets = append(r.lets, let{c.attr("name"), c.attr("value")})
				case "assert", "report":
					a := assertion{id: c.attr("id"), flag: c.attr("flag"), test: c.attr("test"), report: kind == "report"}
					if a.id == "" {
						return p, fmt.Errorf("pattern %s: %s without id in rule %s", id, kind, r.context)
					}
					if a.flag != "fatal" && a.flag != "warning" {
						return p, fmt.Errorf("pattern %s: %s: flag %q", id, a.id, a.flag)
					}
					var err error
					if a.msg, a.values, err = message(c.Inner); err != nil {
						return p, fmt.Errorf("pattern %s: %s: %w", id, a.id, err)
					}
					r.asserts = append(r.asserts, a)
				default:
					return p, fmt.Errorf("pattern %s: unexpected element %s in rule %s", id, kind, r.context)
				}
			}
			p.rules = append(p.rules, r)
		case "title":
			p.title = strings.TrimSpace(reSpace.ReplaceAllString(string(it.Inner), " "))
		case "p":
		default:
			return p, fmt.Errorf("pattern %s: unexpected element %s", id, it.XMLName.Local)
		}
	}
	return p, nil
}

var (
	reSpace = regexp.MustCompile(`\s+`)
	// rePrefix is the identifier that starts a message:
	// "[BR-FR-CDV-01/MDG-3] : ".
	rePrefix = regexp.MustCompile(`^\[[^\]]+\]\s*:?\s*`)
)

// message returns the text of a message, without its identifier, its
// value-of elements written %s (a literal % written %%), and the
// expressions of these elements.
func message(inner []byte) (string, []string, error) {
	d := xml.NewDecoder(bytes.NewReader(append(append([]byte(`<m xmlns="`+nsSchematron+`">`), inner...), "</m>"...)))
	var b strings.Builder
	var values []string
	depth := 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			if t.Name.Local != "value-of" {
				return "", nil, fmt.Errorf("unexpected element %s in a message", t.Name.Local)
			}
			sel := ""
			for _, a := range t.Attr {
				if a.Name.Local == "select" {
					sel = a.Value
				}
			}
			if sel == "" {
				return "", nil, errors.New("value-of without select")
			}
			b.WriteString("\x00")
			values = append(values, sel)
		case xml.EndElement:
			depth--
		case xml.CharData:
			b.WriteString(strings.ReplaceAll(string(t), "%", "%%"))
		}
	}
	text := strings.TrimSpace(reSpace.ReplaceAllString(b.String(), " "))
	text = rePrefix.ReplaceAllString(text, "")
	return strings.ReplaceAll(text, "\x00", "%s"), values, nil
}

var (
	// reAdvice is the end of a message that gives the values found or
	// advises a correction.
	reAdvice = regexp.MustCompile(`\s*(Valeurs? actuelles? ?:|Veuillez|Code Statut : "%s" :).*$`)
	// reValue is a value of the message, with the colon before it.
	reValue = regexp.MustCompile(`\s*:?\s*"%s",?`)
)

// description returns the message of an assertion without its values and
// its advice, for the catalog.
func description(msg string) string {
	d := msg
	if rest, ok := strings.CutPrefix(d, `Code Statut : "%s" : l`); ok {
		d = "L" + rest
	}
	d = reAdvice.ReplaceAllString(d, "")
	d = reValue.ReplaceAllString(d, "")
	d = strings.ReplaceAll(d, "%%", "%")
	d = strings.TrimSpace(d)
	if !strings.HasSuffix(d, ".") {
		d = strings.TrimRight(d, " ,:") + "."
	}
	return d
}

// comment returns text as Go comment lines of at most 77 characters.
func comment(text string) string {
	var b, line strings.Builder
	for _, w := range strings.Fields(text) {
		if line.Len() > 0 && line.Len()+1+len(w) > 74 {
			b.WriteString("// " + line.String() + "\n")
			line.Reset()
		}
		if line.Len() > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(w)
	}
	if line.Len() > 0 {
		b.WriteString("// " + line.String() + "\n")
	}
	return b.String()
}

// reCall is the call of a function of the schematron.
var reCall = regexp.MustCompile(`\b(custom:[\w-]+)\s*\(`)

// usedFunctions returns the functions that the rules call, directly or
// through another function, in schematron order, and the names of the
// others.
func (s *schematron) usedFunctions() ([]function, []string) {
	byName := map[string]*function{}
	for i := range s.funcs {
		byName[s.funcs[i].name] = &s.funcs[i]
	}
	used := map[string]bool{}
	var visit func(src string)
	visit = func(src string) {
		for _, m := range reCall.FindAllStringSubmatch(src, -1) {
			f, ok := byName[m[1]]
			if !ok || used[f.name] {
				continue
			}
			used[f.name] = true
			for _, l := range f.lets {
				visit(l.value)
			}
			visit(f.result)
		}
	}
	for _, p := range s.patterns {
		for _, r := range p.rules {
			visit(r.context)
			for _, l := range r.lets {
				visit(l.value)
			}
			for _, a := range r.asserts {
				visit(a.test)
				for _, v := range a.values {
					visit(v)
				}
			}
		}
	}
	var in []function
	var out []string
	for _, f := range s.funcs {
		if used[f.name] {
			in = append(in, f)
		} else {
			out = append(out, f.name)
		}
	}
	return in, out
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
	fmt.Fprintf(&b, `// Code generated by internal/rules/cdv/gen; DO NOT EDIT.

// Source: schematron BR-FR-CDV of the FNFE-MPE, version %s (fatal),
// file %s%s
// of the France_RFE repository, tag %s (commit
// %s, SHA-256
// %s).
// The rule identifiers, flags, tests, functions and messages are reproduced
// from it.

package cdv

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schematron"
)

// Version is the version of the schematron whose rules the package
// implements.
const Version = %q

// namespaces are the prefixes the schematron declares.
var namespaces = map[string]string{
`, version, dir, mainFile, tag, commit, sum, version)
	for _, n := range s.ns {
		fmt.Fprintf(&b, "\t%q: %q,\n", n[0], n[1])
	}
	used, unused := s.usedFunctions()
	b.WriteString("}\n\n")
	b.WriteString(comment("functions are the functions the schematron declares (xsl:function) and its rules call. It also declares, unused: " + strings.Join(unused, ", ") + "."))
	b.WriteString("var functions = []schematron.Function{\n")
	for _, f := range used {
		fmt.Fprintf(&b, "\t{Name: %q, As: %q, Params: []schematron.Param{", f.name, f.as)
		for _, p := range f.params {
			fmt.Fprintf(&b, "{Name: %q, As: %q}, ", p.name, p.as)
		}
		b.WriteString("}")
		if len(f.lets) > 0 {
			b.WriteString(", Lets: []let{\n")
			for _, l := range f.lets {
				fmt.Fprintf(&b, "\t\t{Name: %q, Value: %q},\n", l.name, strings.TrimSpace(reSpace.ReplaceAllString(l.value, " ")))
			}
			b.WriteString("\t}")
		}
		fmt.Fprintf(&b, ", Result: %q},\n", strings.TrimSpace(reSpace.ReplaceAllString(f.result, " ")))
	}
	b.WriteString("}\n\n// patterns are the patterns of the schematron, in schematron order.\nvar patterns = []pattern{\n")
	type entry struct{ id, flag, desc string }
	var cat []entry
	seen := map[string]bool{}
	total := 0
	for _, p := range s.patterns {
		if p.title != "" {
			fmt.Fprintf(&b, "\t// %s\n", p.title)
		}
		fmt.Fprintf(&b, "\t{ID: %q, Rules: []rule{\n", p.id)
		for _, r := range p.rules {
			fmt.Fprintf(&b, "\t\t{Context: %q,", r.context)
			if len(r.lets) > 0 {
				b.WriteString(" Lets: []let{")
				for _, l := range r.lets {
					fmt.Fprintf(&b, "{Name: %q, Value: %q}, ", l.name, strings.TrimSpace(reSpace.ReplaceAllString(l.value, " ")))
				}
				b.WriteString("},")
			}
			b.WriteString(" Asserts: []assertion{\n")
			for _, a := range r.asserts {
				total++
				fmt.Fprintf(&b, "\t\t\t{ID: %q, Flag: %s,", a.id, severity(a.flag))
				if a.report {
					b.WriteString(" Report: true,")
				}
				fmt.Fprintf(&b, " Test: %q, Msg: %q", strings.TrimSpace(reSpace.ReplaceAllString(a.test, " ")), a.msg)
				if len(a.values) > 0 {
					fmt.Fprintf(&b, ", Values: []string{%s}", quoteList(a.values))
				}
				b.WriteString("},\n")
				if seen[a.id] {
					return nil, fmt.Errorf("%s: two assertions", a.id)
				}
				seen[a.id] = true
				cat = append(cat, entry{a.id, a.flag, description(a.msg)})
			}
			b.WriteString("\t\t}},\n")
		}
		b.WriteString("\t}},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// schematronCatalog lists the rules of the schematron, in schematron\n// order, with their messages.\nvar schematronCatalog = []rules.RuleInfo{\n")
	for _, e := range cat {
		fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", e.id, severity(e.flag), e.desc)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// assertionCount is the number of assertions of the patterns.\nconst assertionCount = %d\n", total)
	return format.Source(b.Bytes())
}
