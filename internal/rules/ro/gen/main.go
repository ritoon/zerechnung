// Command gen generates the rule tables and the catalog of package ro from
// the CIUS-RO validation artefacts of ANAF (Agenția Națională de
// Administrare Fiscală) and of the Romanian ministry of finance.
//
// It reads the archive ro16931-ubl-1.0.9.zip, either from a local file
// (-zip) or downloaded from the copy that the repository phax/phive-rules
// keeps of it (the site of the ministry, mfinante.gov.ro, refuses automated
// clients), checks its SHA-256 and reads the schematron
// EN16931-CIUS_RO-UBL-validation.sch: its namespace prefixes, and the
// pattern ROmodel of the file it includes, cius-ro/RO16931-rules.sch, which
// holds the rules of the CIUS-RO. The other included files are the
// artefacts of EN 16931 1.3.8 of CEN/TC 434, which the EN 16931 rule set
// implements in its own version. It writes:
//
//   - namespaces: the prefixes the schematron declares;
//   - patterns: the pattern ROmodel, with its variables and its rules, each
//     rule with its context and its assertions (identifier, flag, test,
//     message), in schematron order;
//   - schematronCatalog: one rules.RuleInfo per identifier of a rule of the
//     CIUS-RO (BR-RO-*, BR-DEC-RO-*), with its message.
//
// The messages are bilingual, the Romanian text first, then "#" and the
// English text: the tables keep the English text, whose value-of and name
// elements become values of the message (schematron.Assertion.Values). The
// catalog describes each rule by its message without the parentheses that
// hold such values.
//
// Usage (from internal/rules/ro, through go generate):
//
//	go run ./gen [-zip ro16931-ubl-1.0.9.zip] [-o rules_gen.go]
package main

import (
	"archive/zip"
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
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	version = "1.0.9"
	date    = "05.06.2024"
	archive = "ro16931-ubl-" + version + ".zip"
	page    = "https://mfinante.gov.ro/web/efactura/informatii-tehnice"
	commit  = "07840eb381ea43b0f3f8c9f346c826186afcdc4b"
	url     = "https://raw.githubusercontent.com/phax/phive-rules/" + commit + "/phive-rules-cius-ro/docs/" + version + "/" + archive
	sum     = "818e7dd77e429f5dbde7c60b43dddbdf1a9a82a1773c359caa80a36ef89ea7f2"

	mainFile    = "EN16931-CIUS_RO-UBL-validation.sch"
	patternName = "ROmodel"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("ro/gen: ")
	zipPath := flag.String("zip", "", "path of "+archive+" (downloaded from phax/phive-rules when empty)")
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
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
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
		ID    string    `xml:"id,attr"`
		Items []itemXML `xml:",any"`
	}
	// itemXML is a child of the pattern: a let or a rule.
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
		Inner   []byte `xml:",innerxml"`
	}
)

type let struct{ name, value string }

type assertion struct {
	id, flag, test string
	msg            string   // the English message, a format when values is set
	values         []string // expressions of its value-of and name elements
	report         bool
}

type rule struct {
	context string
	lets    []let
	asserts []assertion
}

type schematron struct {
	ns    [][2]string
	lets  []let
	rules []rule
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
	for _, inc := range sx.Includes {
		b, ok := files[path.Join(dir, inc.Href)]
		if !ok {
			return nil, fmt.Errorf("%s: not in %s", inc.Href, archive)
		}
		var p patternXML
		if err := xml.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", inc.Href, err)
		}
		if p.ID != patternName {
			continue
		}
		if s.rules != nil {
			return nil, fmt.Errorf("pattern %s included twice", patternName)
		}
		if err := s.pattern(p); err != nil {
			return nil, fmt.Errorf("%s: %w", inc.Href, err)
		}
	}
	if s.rules == nil {
		return nil, fmt.Errorf("pattern %s not found", patternName)
	}
	return s, nil
}

func (s *schematron) pattern(p patternXML) error {
	for _, it := range p.Items {
		switch it.XMLName.Local {
		case "let":
			s.lets = append(s.lets, let{it.Name, it.Value})
		case "rule":
			r := rule{context: it.Context}
			for _, l := range it.Lets {
				r.lets = append(r.lets, let{l.Name, l.Value})
			}
			for _, c := range it.Checks {
				kind := c.XMLName.Local
				if kind != "assert" && kind != "report" {
					continue
				}
				if c.ID == "" {
					return fmt.Errorf("%s without id in rule %s", kind, it.Context)
				}
				if c.Flag != "fatal" && c.Flag != "warning" {
					return fmt.Errorf("%s: flag %q", c.ID, c.Flag)
				}
				msg, values, err := message(c.Inner)
				if err != nil {
					return fmt.Errorf("%s: %w", c.ID, err)
				}
				r.asserts = append(r.asserts, assertion{
					id: c.ID, flag: c.Flag, test: c.Test, report: kind == "report",
					msg: msg, values: values,
				})
			}
			s.rules = append(s.rules, r)
		case "title", "p":
		default:
			return fmt.Errorf("unexpected element %s in pattern %s", it.XMLName.Local, p.ID)
		}
	}
	return nil
}

var reSpace = regexp.MustCompile(`\s+`)

// message returns the English text of a bilingual message (after "#"), as a
// format whose verbs %s stand for its value-of and name elements, and the
// expressions of these elements.
func message(inner []byte) (string, []string, error) {
	const ns = "http://purl.oclc.org/dsdl/schematron"
	d := xml.NewDecoder(bytes.NewReader(append(append([]byte(`<m xmlns="`+ns+`">`), inner...), "</m>"...)))
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
			switch t.Name.Local {
			case "name":
				b.WriteString("\x00")
				values = append(values, "name()")
			case "value-of":
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
			default:
				return "", nil, fmt.Errorf("unexpected element %s in a message", t.Name.Local)
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			b.Write(t)
		}
	}
	text := b.String()
	ro, en, ok := strings.Cut(text, "#")
	if !ok || strings.Contains(en, "#") {
		return "", nil, fmt.Errorf("message is not \"Romanian#English\": %q", text)
	}
	// The values of the Romanian text are not kept.
	values = values[strings.Count(ro, "\x00"):]
	en = strings.TrimSpace(reSpace.ReplaceAllString(en, " "))
	en = strings.ReplaceAll(en, "%", "%%")
	return strings.ReplaceAll(en, "\x00", "%s"), values, nil
}

// reValueGroup is a parenthesized part of a message that holds a value.
var reValueGroup = regexp.MustCompile(`\s*\([^()]*%s[^()]*\)`)

// description returns the message without the parentheses that hold its
// values, any other value written "…", for the catalog.
func description(msg string) string {
	d := reValueGroup.ReplaceAllString(msg, "")
	d = strings.ReplaceAll(d, "%s", "…")
	d = strings.ReplaceAll(d, "%%", "%")
	d = strings.ReplaceAll(d, " .", ".")
	return strings.TrimSpace(d)
}

func severity(flag string) string {
	if flag == "warning" {
		return "rules.Warning"
	}
	return "rules.Error"
}

// national reports whether an assertion is a rule of the CIUS-RO itself:
// BR-RO-*, BR-DEC-RO-*. The pattern also restates BR-27 of EN 16931.
func national(id string) bool { return strings.Contains(id, "-RO-") }

func (s *schematron) generate() ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/rules/ro/gen; DO NOT EDIT.

// Source: CIUS-RO validation artefacts %s of %s, published on
// %s
// (archive %s, SHA-256
// %s),
// as kept by github.com/phax/phive-rules at commit
// %s:
// file %s and the pattern %s it includes. The
// rule identifiers, flags, tests and English messages are reproduced from
// them.

package ro

import "github.com/ritoon/zerechnung/internal/rules"

// Version is the version of the validation artefacts whose rules the
// package implements.
const Version = %q

// namespaces are the prefixes the schematron declares.
var namespaces = map[string]string{
`, version, date, page, archive, sum, commit, mainFile, patternName, version)
	for _, n := range s.ns {
		fmt.Fprintf(&b, "\t%q: %q,\n", n[0], n[1])
	}
	fmt.Fprintf(&b, "}\n\n// patterns is the pattern %s of the schematron.\nvar patterns = []pattern{\n\t{ID: %q,", patternName, patternName)
	writeLets := func(lets []let) {
		if len(lets) == 0 {
			return
		}
		b.WriteString(" Lets: []let{")
		for _, l := range lets {
			fmt.Fprintf(&b, "{Name: %q, Value: %q}, ", l.name, l.value)
		}
		b.WriteString("},")
	}
	writeLets(s.lets)
	b.WriteString(" Rules: []rule{\n")
	type entry struct{ id, flag, desc string }
	var cat []entry
	seen := map[string]bool{}
	total := 0
	for _, r := range s.rules {
		fmt.Fprintf(&b, "\t\t{Context: %q,", r.context)
		writeLets(r.lets)
		b.WriteString(" Asserts: []assertion{\n")
		for _, a := range r.asserts {
			total++
			fmt.Fprintf(&b, "\t\t\t{ID: %q, Flag: %s,", a.id, severity(a.flag))
			if a.report {
				b.WriteString(" Report: true,")
			}
			fmt.Fprintf(&b, " Test: %q, Msg: %q", a.test, a.msg)
			if len(a.values) > 0 {
				b.WriteString(", Values: []string{")
				for i, v := range a.values {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "%q", v)
				}
				b.WriteString("}")
			}
			b.WriteString("},\n")
			if seen[a.id] || !national(a.id) {
				continue
			}
			seen[a.id] = true
			cat = append(cat, entry{a.id, a.flag, description(a.msg)})
		}
		b.WriteString("\t\t}},\n")
	}
	b.WriteString("\t}},\n}\n\n")
	b.WriteString("// schematronCatalog lists the rules of the CIUS-RO itself (BR-RO-*,\n// BR-DEC-RO-*), in schematron order, with their English messages.\nvar schematronCatalog = []rules.RuleInfo{\n")
	for _, e := range cat {
		fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", e.id, severity(e.flag), e.desc)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// assertionCount is the number of assertions of the pattern.\nconst assertionCount = %d\n", total)
	return format.Source(b.Bytes())
}
