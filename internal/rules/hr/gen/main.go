// Command gen generates the rule tables and the catalog of package hr from
// the schematron of the Croatian CIUS and its extension (CIUS-HR and EXT-HR
// 2025) that the Croatian tax administration (Porezna uprava) publishes for
// Fiskalizacija 2.0.
//
// It reads the archive HRUBLSchematron_13032026-2.zip, either from a local
// file (-zip) or downloaded from the site of the tax administration,
// checks its SHA-256 and reads the schematron
// HR-CIUS-EXT-EN16931-UBL.sch: its namespace prefixes, its two functions
// (xsl:function u:ctrlOIB, the check digit of an OIB, and u:check3010), its
// two patterns and the pattern HRCodesmodel of the file it includes,
// codelist/HR-CIUS-EXT-EN16931-UBL-codes.sch. It writes:
//
//   - namespaces: the prefixes the schematron declares;
//   - functions: the functions, with their parameters, variables and
//     result;
//   - patterns: the three patterns in schematron order, with their rules,
//     each rule with its context, its variables and its assertions
//     (identifier, flag, test, message);
//   - schematronCatalog: one rules.RuleInfo per identifier, in schematron
//     order.
//
// The patterns of the main file have no identifier: the tables call them
// HRempty (the empty elements, HR-BR-33) and HRmodel (the other rules).
// The messages of the schematron are in Croatian, preceded by the
// identifier of the rule ("[HR-BR-1] - "): each assertion keeps its
// Croatian message as a comment, and gets the English message that
// english gives for its identifier, whose verbs %s receive the values of
// the value-of elements of the Croatian message. The catalog describes each
// rule by its English message without these values.
//
// Usage (from internal/rules/hr, through go generate):
//
//	go run ./gen [-zip HRUBLSchematron_13032026-2.zip] [-o rules_gen.go]
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
	"unicode"
	"unicode/utf8"
)

const (
	version = "1.0.0"
	date    = "13.03.2026"
	archive = "HRUBLSchematron_13032026-2.zip"
	page    = "https://porezna.gov.hr/fiskalizacija/bezgotovinski-racuni/eracun"
	url     = "https://porezna.gov.hr/fiskalizacija/api/dokumenti/197"
	sum     = "d6250b00b1cd385c835e3dd4c3abc71a683e9b890f04f41c0ae1e04f3c3e790c"

	mainFile = "HR-CIUS-EXT-EN16931-UBL.sch"

	nsSchematron = "http://purl.oclc.org/dsdl/schematron"
	nsXSL        = "http://www.w3.org/1999/XSL/Transform"
)

// inlinePatterns names the patterns of the main file, which have no
// identifier, in order.
var inlinePatterns = []string{"HRempty", "HRmodel"}

// english gives the English message of each rule, as a format whose verbs
// %s receive the values of its Croatian message. It is a translation of
// the Croatian message, corrected where the message names the wrong
// business term (HR-BR-13, HR-BR-55) and completed where it says less than
// the test (HR-BR-40 and the other date ranges admit their first day).
var english = map[string]string{
	"HR-BR-33":    "An invoice shall not contain empty XML elements, except the element that holds the electronic signature of the invoice",
	"HR-BR-1":     "The Invoice number (BT-1) shall not contain white space",
	"HR-BR-40":    "The Invoice issue date (BT-2) (%s) shall be on or after 01.01.2026 and before 01.01.2100",
	"HR-BR-48":    "The Value added tax point date (BT-7) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-49":    "The Invoicing period start date (BT-73) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-50":    "The Invoicing period end date (BT-74) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-2":     "An invoice shall have an Invoice issue time (HR-BT-2), written hh:mm:ss",
	"HR-BR-34":    "The Business process type (BT-23) shall be given: P1 to P12 (table HR-TB-1), or P99: followed by a label of the Buyer",
	"HR-BR-42":    "The Business process type (BT-23) shall not exceed 200 characters",
	"HR-BR-43":    "The Contract reference (BT-12) shall not exceed 1024 characters",
	"HR-BR-44":    "The Actual delivery date (BT-72) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-5":     "The Specification identifier (BT-24) shall be 'urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0'",
	"HR-BR-S-1":   "An invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is 'Standard rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-S-2":   "An invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is 'Standard rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-S-3":   "An invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is 'Standard rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-Z-1":   "An invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is 'Zero rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-Z-2":   "An invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is 'Zero rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-Z-3":   "An invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is 'Zero rated' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-E-1":   "An invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is 'Exempt from VAT' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-E-2":   "An invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is 'Exempt from VAT' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-E-3":   "An invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is 'Exempt from VAT' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-AE-1":  "An invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is 'Reverse charge' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-AE-2":  "An invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is 'Reverse charge' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-AE-3":  "An invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is 'Reverse charge' shall contain the Buyer VAT identifier (BT-48)",
	"HR-BR-26":    "An invoice that contains an Invoice line (BG-25), a Document level allowance (BG-20) or a Document level charge (BG-21) where the VAT category code (BT-151, BT-95 or BT-102) is 'Exempt from VAT' shall contain the HR VAT breakdown (HR-BG-2)",
	"HR-BR-27":    "The HR total amount subject to VAT (HR-BT-23) shall equal the sum of Invoice line net amounts (BT-131), minus the sum of Document level allowance amounts (BT-92), plus the sum of Document level charge amounts (BT-99), minus the HR amount out of the scope of VAT (HR-BT-24)",
	"HR-BR-29":    "Invoice lines, charges and allowances outside the VAT taxable amount shall have the HR VAT category code (HR-BT-18) 'O' and an HR VAT category tax amount (HR-BT-17) of 0",
	"HR-BR-30":    "The HR document totals (HR-BG-3) shall be given when the invoice contains Invoice lines (BG-25) or Document level charges (BG-21) outside the VAT taxable amount, unless their VAT category is O (Not subject to VAT) with the HR VAT category label HR:O",
	"HR-BR-32":    "The HR amount out of the scope of VAT (HR-BT-24) shall equal the sum of the net amounts of the Invoice lines (BT-131) outside the VAT taxable amount, minus the Document level allowances (BT-92) and plus the Document level charges (BT-99) outside it",
	"HR-BR-54":    "The HR invoice total VAT amount (HR-BT-25) shall equal the sum of the HR VAT category tax amounts (HR-BT-17)",
	"HR-BR-4":     "When the Amount due for payment (BT-115) is positive, the Payment due date (BT-9) shall be given",
	"HR-BR-41":    "The Payment due date (BT-9) (%s) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-55":    "The HR invoice total VAT amount (HR-BT-25) shall equal the Invoice total VAT amount (BT-110)",
	"HR-BR-56":    "The item prices (BT-146, BT-147, BT-148), VAT rates (BT-119, BT-96, BT-103, BT-152, HR-BT-19) and quantities (BT-129, BT-149) shall have at most 10 decimals and 30 characters",
	"HR-BR-11":    "Each Document level charge (BG-21) not subject to VAT or exempt from VAT shall have the HR VAT category label of the charge (HR-BT-6)",
	"HR-BR-13":    "Each Document level charge (BG-21) not subject to VAT or exempt from VAT shall have a VAT exemption reason (HR-BT-7) or a VAT exemption reason code (HR-BT-8) of the charge",
	"HR-BR-6":     "Each Preceding invoice reference (BG-3) shall have the Preceding invoice issue date (BT-26)",
	"HR-BR-45":    "The Preceding invoice issue date (BT-26) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-7":     "The Seller electronic address (BT-34) shall be given",
	"HR-BR-37":    "An invoice shall contain the operator label (HR-BT-4)",
	"HR-BR-9":     "An invoice shall contain a valid OIB of the operator (HR-BT-5)",
	"HR-BR-53":    "The VAT identifier (BT-31, BT-48, BT-63) of a party in Croatia shall be a valid OIB, with or without the prefix HR",
	"HR-BR-10":    "The Buyer electronic address (BT-49) shall be given",
	"HR-BR-17":    "When an invoice line references a purchase order (HR-BT-9), the Purchase order reference (BT-13) shall not be used",
	"HR-BR-18":    "When an invoice line references a despatch advice (HR-BT-10), the Despatch advice reference (BT-16) shall not be used",
	"HR-BR-19":    "When an invoice line references a receiving advice (HR-BT-11), the Receiving advice reference (BT-15) shall not be used",
	"HR-BR-20":    "The Item price base quantity (BT-149), when given, shall be greater than zero",
	"HR-BR-51":    "The Invoice line period start date (BT-134) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-52":    "The Invoice line period end date (BT-135) shall be on or after 01.01.1900 and before 01.01.2100",
	"HR-BR-21":    "Line %s: the unit of measure of the Item price base quantity (BT-150), when given, shall be that of the Invoiced quantity (BT-130)",
	"HR-BR-16":    "Line %s: each Invoice line (BG-25) not subject to VAT or exempt from VAT shall have the HR VAT category label of the item (HR-BT-12)",
	"HR-BR-25":    "Line %s: each item shall have an Item classification identifier (BT-158) of the Croatian classification of products by activity KPD (CPA), list identifier CG, except in a prepayment invoice and a credit note",
	"HR-BR-36":    "Line %s: each Invoice line (BG-25) not subject to VAT or exempt from VAT shall have a VAT exemption reason (HR-BT-13) or a VAT exemption reason code (HR-BT-14)",
	"HR-BR-46":    "Line %s: the Item name (BT-153) shall not exceed 1024 characters",
	"HR-BR-47":    "Line %s: the Item description (BT-154) shall not exceed 4096 characters",
	"HR-BR-28":    "In the HR VAT breakdown (HR-BG-2), the HR VAT category tax amount (HR-BT-17) shall be 0 for the HR VAT category codes E and O",
	"HR-BR-S-10":  "In each HR VAT breakdown (HR-BG-2) where the HR VAT category code (HR-BT-18) is 'Standard rated', the HR VAT category rate (HR-BT-19) shall be greater than 0",
	"HR-BR-Z-10":  "In each HR VAT breakdown (HR-BG-2) where the HR VAT category code (HR-BT-18) is 'Zero rated', the HR VAT category rate (HR-BT-19) shall be 0",
	"HR-BR-E-10":  "In each HR VAT breakdown (HR-BG-2) where the HR VAT category code (HR-BT-18) is 'Exempt from VAT', the HR VAT category rate (HR-BT-19) shall be 0",
	"HR-BR-AE-10": "In each HR VAT breakdown (HR-BG-2) where the HR VAT category code (HR-BT-18) is 'Reverse charge', the HR VAT category rate (HR-BT-19) shall be 0",
	"HR-BR-CL-1":  "The HR VAT category label (HR-BT-6, HR-BT-12, HR-BT-22) shall be one of table HR-TB-2 of the specification",
	"HR-BR-CL-2":  "Line %s: an Item classification identifier (BT-158) of list CG shall be a code of the Croatian classification of products by activity KPD (CPA)",
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("hr/gen: ")
	zipPath := flag.String("zip", "", "path of "+archive+" (downloaded from porezna.gov.hr when empty)")
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

// node is an element of a schematron file, with its attributes, children
// and content.
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
	croatian       string   // the Croatian message, without its identifier
	values         []string // expressions of its value-of elements
	report         bool
}

type rule struct {
	context string
	lets    []let
	asserts []assertion
}

type pattern struct {
	id    string
	rules []rule
}

type schematron struct {
	ns       [][2]string
	funcs    []function
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
	var root node
	if err := xml.Unmarshal(mainData, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", mainFile, err)
	}
	s := &schematron{}
	inline := 0
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
			if c.attr("id") != "" || inline == len(inlinePatterns) {
				return nil, fmt.Errorf("%s: unexpected pattern %q", mainFile, c.attr("id"))
			}
			p, err := readPattern(inlinePatterns[inline], c)
			if err != nil {
				return nil, err
			}
			inline++
			s.patterns = append(s.patterns, p)
		case c.XMLName.Local == "include":
			href := c.attr("href")
			b, ok := files[path.Join(dir, href)]
			if !ok {
				return nil, fmt.Errorf("%s: not in %s", href, archive)
			}
			var pn node
			if err := xml.Unmarshal(b, &pn); err != nil {
				return nil, fmt.Errorf("%s: %w", href, err)
			}
			if pn.XMLName.Space != nsSchematron || pn.XMLName.Local != "pattern" || pn.attr("id") == "" {
				return nil, fmt.Errorf("%s: not a pattern with an identifier", href)
			}
			p, err := readPattern(pn.attr("id"), &pn)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", href, err)
			}
			s.patterns = append(s.patterns, p)
		case c.XMLName.Local == "title":
		default:
			return nil, fmt.Errorf("%s: unexpected element %s", mainFile, c.XMLName.Local)
		}
	}
	if inline != len(inlinePatterns) {
		return nil, fmt.Errorf("%s: %d patterns without identifier, want %d", mainFile, inline, len(inlinePatterns))
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
					if a.croatian, a.values, err = message(c.Inner); err != nil {
						return p, fmt.Errorf("pattern %s: %s: %w", id, a.id, err)
					}
					r.asserts = append(r.asserts, a)
				default:
					return p, fmt.Errorf("pattern %s: unexpected element %s in rule %s", id, kind, r.context)
				}
			}
			p.rules = append(p.rules, r)
		case "title", "p":
		default:
			return p, fmt.Errorf("pattern %s: unexpected element %s", id, it.XMLName.Local)
		}
	}
	return p, nil
}

var (
	reSpace = regexp.MustCompile(`\s+`)
	// rePrefix is the identifier that starts a message: "[HR-BR-1] - ".
	rePrefix = regexp.MustCompile(`^\[[^\]]+\]\s*-?\s*`)
)

// message returns the text of a message, without its identifier, its
// value-of elements written %s, and the expressions of these elements.
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
			b.Write(t)
		}
	}
	text := strings.TrimSpace(reSpace.ReplaceAllString(b.String(), " "))
	text = rePrefix.ReplaceAllString(text, "")
	return strings.ReplaceAll(text, "\x00", "%s"), values, nil
}

var (
	// reValueGroup is a parenthesized part of a message that holds a value.
	reValueGroup = regexp.MustCompile(`\s*\([^()]*%s[^()]*\)`)
	// reLine is the line number that starts a message on an invoice line.
	reLine = regexp.MustCompile(`^Line %s: `)
)

// description returns the English message without its values, for the
// catalog.
func description(msg string) string {
	d := reLine.ReplaceAllString(msg, "")
	d = reValueGroup.ReplaceAllString(d, "")
	d = strings.ReplaceAll(d, "%%", "%")
	r, n := utf8.DecodeRuneInString(d)
	return string(unicode.ToUpper(r)) + d[n:]
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
	fmt.Fprintf(&b, `// Code generated by internal/rules/hr/gen; DO NOT EDIT.

// Source: schematron of the Croatian CIUS and its extension (CIUS-HR and
// EXT-HR 2025) %s of %s, published on
// %s
// (archive %s, SHA-256
// %s):
// file %s and the pattern it includes. The rule
// identifiers, flags, tests and functions are reproduced from them; the
// Croatian messages are kept as comments, with the English messages of the
// package.

package hr

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schematron"
)

// Version is the version of the schematron whose rules the package
// implements.
const Version = %q

// namespaces are the prefixes the schematron declares.
var namespaces = map[string]string{
`, version, date, page, archive, sum, mainFile, version)
	for _, n := range s.ns {
		fmt.Fprintf(&b, "\t%q: %q,\n", n[0], n[1])
	}
	b.WriteString("}\n\n// functions are the functions the schematron declares (xsl:function).\nvar functions = []schematron.Function{\n")
	for _, f := range s.funcs {
		fmt.Fprintf(&b, "\t{Name: %q, As: %q, Params: []schematron.Param{", f.name, f.as)
		for _, p := range f.params {
			fmt.Fprintf(&b, "{Name: %q, As: %q}, ", p.name, p.as)
		}
		b.WriteString("}, Lets: []let{\n")
		for _, l := range f.lets {
			fmt.Fprintf(&b, "\t\t{Name: %q, Value: %q},\n", l.name, strings.TrimSpace(reSpace.ReplaceAllString(l.value, " ")))
		}
		fmt.Fprintf(&b, "\t}, Result: %q},\n", f.result)
	}
	b.WriteString("}\n\n// patterns are the patterns of the schematron, in schematron order.\nvar patterns = []pattern{\n")
	type entry struct{ id, flag, desc string }
	var cat []entry
	seen := map[string]bool{}
	total := 0
	for _, p := range s.patterns {
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
				msg, ok := english[a.id]
				if !ok {
					return nil, fmt.Errorf("%s: no English message", a.id)
				}
				if n := strings.Count(msg, "%s"); n != len(a.values) {
					return nil, fmt.Errorf("%s: %d verbs in the English message for %d values", a.id, n, len(a.values))
				}
				fmt.Fprintf(&b, "\t\t\t// %s\n", strings.ReplaceAll(a.croatian, "%s", "…"))
				fmt.Fprintf(&b, "\t\t\t{ID: %q, Flag: %s,", a.id, severity(a.flag))
				if a.report {
					b.WriteString(" Report: true,")
				}
				fmt.Fprintf(&b, " Test: %q, Msg: %q", strings.TrimSpace(reSpace.ReplaceAllString(a.test, " ")), msg)
				if len(a.values) > 0 {
					fmt.Fprintf(&b, ", Values: []string{%s}", quoteList(a.values))
				}
				b.WriteString("},\n")
				if seen[a.id] {
					continue
				}
				seen[a.id] = true
				cat = append(cat, entry{a.id, a.flag, description(msg)})
			}
			b.WriteString("\t\t}},\n")
		}
		b.WriteString("\t}},\n")
	}
	b.WriteString("}\n\n")
	for id := range english {
		if !seen[id] {
			return nil, fmt.Errorf("%s: English message of no assertion", id)
		}
	}
	b.WriteString("// schematronCatalog lists the rules of the schematron, in schematron\n// order, with their English messages.\nvar schematronCatalog = []rules.RuleInfo{\n")
	for _, e := range cat {
		fmt.Fprintf(&b, "\t{ID: %q, Severity: %s, Description: %q},\n", e.id, severity(e.flag), e.desc)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// assertionCount is the number of assertions of the patterns.\nconst assertionCount = %d\n", total)
	return format.Source(b.Bytes())
}
