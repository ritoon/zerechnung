package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/validate"
)

// The XSLT that the NPa publishes for its schematrons (FX_NL/xsl): one for
// NLCIUS-CII, one for SI-UBL 2.0 and one for SI-UBL 2.0 with the G-account
// extension. Each includes the EN 16931 rules of its syntax.
const (
	xslCII      = "nlcius-cii-1.0.xsl"
	xslUBL      = "si-ubl-2.0.xsl"
	xslGAccount = "si-ubl-2.0-ext-gaccount.xsl"
)

// gaccountMarker is the part of the UBL customization identifier that selects
// the artefacts of the G-account extension.
const gaccountMarker = "#conformant#urn:fdc:nen.nl:gaccount:v1.0"

// Namespaces of the invoices.
const (
	nsCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	nsCBC = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
)

// npaStylesheet returns the XSLT that a validator picks for an invoice: the
// NLCIUS-CII one for a CII invoice; for a UBL Invoice or CreditNote, the
// G-account one when its cbc:CustomizationID (a child of the root) contains
// the G-account identifier, the SI-UBL 2.0 one otherwise. It returns "" for
// another document.
func npaStylesheet(data []byte) string {
	d := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	var (
		ubl  bool
		read bool // reading the cbc:CustomizationID
		id   strings.Builder
	)
	for {
		tok, err := d.Token()
		if err != nil {
			if ubl {
				return xslUBL
			}
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case depth == 1 && t.Name.Space == nsCII && t.Name.Local == "CrossIndustryInvoice":
				return xslCII
			case depth == 1 && strings.HasPrefix(t.Name.Space, "urn:oasis:names:specification:ubl:schema:xsd:") &&
				(t.Name.Local == "Invoice" || t.Name.Local == "CreditNote"):
				ubl = true
			case depth == 1:
				return ""
			case depth == 2 && t.Name.Space == nsCBC && t.Name.Local == "CustomizationID":
				read = true
			}
		case xml.CharData:
			if read {
				id.Write(t)
			}
		case xml.EndElement:
			if read {
				if strings.Contains(id.String(), gaccountMarker) {
					return xslGAccount
				}
				return xslUBL
			}
			depth--
		}
	}
}

// syntaxOf returns the syntax of an XSLT of the NPa: cii or ubl.
func syntaxOf(xsl string) string {
	if filepath.Base(xsl) == xslCII {
		return "cii"
	}
	return "ubl"
}

// nlPatterns are the patterns of the NPa artefacts that the package
// reimplements (package internal/rules/nl): the others hold the EN 16931
// rules.
var nlPatterns = map[string]bool{"nlcius": true, "SI-UBL-VERSION": true, "g-account-extension": true}

// xslAssertion is an assertion (or a report) of a schematron, as compiled in
// an XSLT of the NPa.
type xslAssertion struct {
	Pattern string
	ID      string // the identifier, or the one in brackets that opens the message
	Flag    string
	Message string // whitespace normalized
	from    int    // lines of the test (xsl:when or xsl:if) in the XSLT
	to      int
	end     int    // line of the end of the svrl:failed-assert (svrl:successful-report)
	tmpl    int    // index of the xsl:template of the rule, in templates
	test    string // the test, in an XSLT compiled by SchXslt
	scope   int    // index of the xsl:template that holds it, in scopes (SchXslt)
}

// template is the start tag of the xsl:template of a schematron rule, and
// the number of assertions it holds.
type template struct {
	from, to, asserts int
}

// scope is an xsl:template of an XSLT compiled by SchXslt: the lines of its
// start tag, and its variables, those of the rules it holds.
type scope struct {
	from, to int
	vars     []xslVariable
}

// xslVariable is a variable of a schematron rule, as SchXslt compiles it.
type xslVariable struct {
	name     string
	src      string // the xsl:variable element, as written
	from, to int    // its lines
}

// xslIndex holds the assertions of an XSLT and the templates of its rules.
type xslIndex struct {
	asserts   []xslAssertion
	templates []template
	scopes    []scope // SchXslt
}

// key identifies an assertion in a syntax: a pattern may hold several
// assertions with the same identifier (BR-NL-8 in UBL).
func (a xslAssertion) key(syntax string) string {
	return syntax + "/" + a.Pattern + "\x00" + a.ID + "\x00" + a.Message
}

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

// assertionID returns the identifier of an assertion: its id, or the one in
// brackets that opens its message ("[BR-NL-34] The use…", "[SI-INV-R000]-This
// is not…").
func assertionID(id, msg string) string {
	if id != "" {
		return id
	}
	if strings.HasPrefix(msg, "[") {
		if i := strings.Index(msg, "]"); i > 1 {
			return msg[1:i]
		}
	}
	return ""
}

// indexXSL reads the assertions of an XSLT compiled from a schematron by the
// ISO skeleton: a comment <!--PATTERN name--> opens the templates of each
// pattern, one xsl:template per rule, and each assertion is an xsl:when (or,
// for a report, an xsl:if) whose test fails into an svrl:failed-assert
// (svrl:successful-report) holding the id and the flag as xsl:attribute,
// and the message as svrl:text.
func indexXSL(file string) (xslIndex, error) {
	var idx xslIndex
	data, err := os.ReadFile(file)
	if err != nil {
		return idx, err
	}
	const (
		nsXSL  = "http://www.w3.org/1999/XSL/Transform"
		nsSVRL = "http://purl.oclc.org/dsdl/svrl"
	)
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		pattern  string
		cur      *xslAssertion
		from, to int    // lines of the last test
		field    string // id, flag, text: what the character data goes to
		text     strings.Builder
		id, flag string
	)
	for {
		before, _ := d.InputPos()
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return idx, fmt.Errorf("%s: %w", file, err)
		}
		switch t := tok.(type) {
		case xml.Comment:
			if p, ok := strings.CutPrefix(strings.TrimSpace(string(t)), "PATTERN "); ok {
				pattern = strings.TrimSpace(p)
			}
		case xml.StartElement:
			switch {
			case t.Name.Space == nsXSL && t.Name.Local == "template":
				end, _ := d.InputPos()
				idx.templates = append(idx.templates, template{from: before, to: end})
			case t.Name.Space == nsXSL && (t.Name.Local == "when" || t.Name.Local == "if"):
				from = before
				to, _ = d.InputPos()
			case t.Name.Space == nsSVRL && (t.Name.Local == "failed-assert" || t.Name.Local == "successful-report"):
				cur = &xslAssertion{Pattern: pattern, from: from, to: to, tmpl: len(idx.templates) - 1}
				id, flag = "", ""
			case cur != nil && t.Name.Space == nsXSL && t.Name.Local == "attribute":
				field = attr(t, "name")
				text.Reset()
			case cur != nil && t.Name.Space == nsSVRL && t.Name.Local == "text":
				field = "text"
				text.Reset()
			}
		case xml.CharData:
			if field != "" {
				text.Write(t)
			}
		case xml.EndElement:
			switch {
			case cur == nil:
			case t.Name.Space == nsXSL && t.Name.Local == "attribute":
				switch field {
				case "id":
					id = strings.TrimSpace(text.String())
				case "flag":
					flag = strings.TrimSpace(text.String())
				}
				field = ""
			case t.Name.Space == nsSVRL && t.Name.Local == "text":
				cur.Message = normalize(text.String())
				field = ""
			case t.Name.Space == nsSVRL && (t.Name.Local == "failed-assert" || t.Name.Local == "successful-report"):
				cur.ID, cur.Flag = assertionID(id, cur.Message), flag
				cur.end, _ = d.InputPos()
				if cur.tmpl >= 0 {
					idx.templates[cur.tmpl].asserts++
				}
				idx.asserts = append(idx.asserts, *cur)
				cur = nil
			}
		}
	}
	return idx, nil
}

// at returns the assertion that raised a dynamic error reported at line:
// the one whose test spans the line; as Saxon may report the location of the
// svrl:failed-assert that the test guards (in the XSLT of the FNFE-MPE), the
// one whose test and failed assertion span the line; or, as Saxon may report
// the location of the template, the only assertion of the rule whose
// xsl:template start tag spans the line.
func (idx xslIndex) at(line int) (xslAssertion, bool) {
	for _, a := range idx.asserts {
		if a.from <= line && line <= a.to {
			return a, true
		}
	}
	for _, a := range idx.asserts {
		if a.from <= line && line <= a.end {
			return a, true
		}
	}
	for _, a := range idx.asserts {
		if a.tmpl < 0 {
			continue
		}
		if t := idx.templates[a.tmpl]; t.from <= line && line <= t.to && t.asserts == 1 {
			return a, true
		}
	}
	return xslAssertion{}, false
}

// flagLevel maps the flag of an assertion of the NPa artefacts.
func flagLevel(flag string) string {
	switch flag {
	case "fatal", "error":
		return levelError
	case "warning":
		return levelWarning
	}
	return levelInfo
}

// svrlFinding is a failed assertion (or a successful report) of an SVRL
// report.
type svrlFinding struct {
	id, flag, message string
	location          string // XPath of the context node
}

// svrlReport is what NPaBatch (or CiusPtBatch) writes for an invoice: an
// SVRL report, or the dynamic error that stopped the transformation.
type svrlReport struct {
	findings []svrlFinding
	errCode  string
	errLine  int
	errStack []int // lines of the calls that led to the error, innermost first (CiusPtBatch)
	errMsg   string
}

// parseSVRL reads a report written by NPaBatch or CiusPtBatch.
func parseSVRL(data []byte) (svrlReport, error) {
	var rep svrlReport
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		cur  *svrlFinding
		text strings.Builder
		in   bool // reading svrl:text, or the error message
		root string
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return rep, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root == "" {
				root = t.Name.Local
				if root == "error" {
					rep.errCode = attr(t, "code")
					rep.errLine, _ = strconv.Atoi(attr(t, "line"))
					for _, f := range strings.Fields(attr(t, "stack")) {
						n, err := strconv.Atoi(f)
						if err != nil {
							return rep, fmt.Errorf("bad stack %q", attr(t, "stack"))
						}
						rep.errStack = append(rep.errStack, n)
					}
					in = true
					text.Reset()
				}
				continue
			}
			switch t.Name.Local {
			case "failed-assert", "successful-report":
				cur = &svrlFinding{id: attr(t, "id"), flag: attr(t, "flag"), location: attr(t, "location")}
			case "text":
				if cur != nil {
					in = true
					text.Reset()
				}
			}
		case xml.CharData:
			if in {
				text.Write(t)
			}
		case xml.EndElement:
			switch {
			case root == "error" && t.Name.Local == "error":
				rep.errMsg = normalize(text.String())
				in = false
			case cur != nil && t.Name.Local == "text":
				cur.message = normalize(text.String())
				in = false
			case cur != nil && (t.Name.Local == "failed-assert" || t.Name.Local == "successful-report"):
				cur.id = assertionID(cur.id, cur.message)
				rep.findings = append(rep.findings, *cur)
				cur = nil
			}
		}
	}
	if root != "schematron-output" && root != "error" {
		return rep, fmt.Errorf("not an SVRL report")
	}
	return rep, nil
}

// xslRun is the run of XSLT compiled by the ISO skeleton, by NPaBatch: the
// NPa XSLT (npa.list), the EXTENDED-CTC-FR XSLT of the FNFE-MPE (fnfe.list)
// or its XSLT BR-FR-CDV of the statuses (cdv.list). It holds the stylesheet given to each staged invoice, the
// assertions of each stylesheet, and the NLCIUS assertions that failed on
// at least one invoice.
type xslRun struct {
	list    string              // npa.list, fnfe.list, cdv.list
	xsl     map[string]string   // staged name → XSLT
	indexes map[string]xslIndex // XSLT → assertions
	fired   map[string]bool     // key of an assertion of an NLCIUS pattern → it failed
}

// newXSLRun reads the list of a run in the work directory: one line per
// invoice, the XSLT and the staged file separated by a tab.
func newXSLRun(work, list string) (*xslRun, error) {
	run := &xslRun{list: list, xsl: map[string]string{}, indexes: map[string]xslIndex{}, fired: map[string]bool{}}
	data, err := os.ReadFile(filepath.Join(work, list))
	if os.IsNotExist(err) {
		return run, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		xsl, file, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("%s: bad line %q", list, line)
		}
		run.xsl[filepath.Base(file)] = xsl
		if _, ok := run.indexes[xsl]; !ok {
			idx, err := indexXSL(xsl)
			if err != nil {
				return nil, err
			}
			run.indexes[xsl] = idx
		}
	}
	return run, nil
}

// result turns the report of an invoice into a result, and records the
// NLCIUS assertions that failed. Every assertion of the report is compared,
// at its level. When the transformation stopped on a dynamic error, the
// assertion whose test raised it counts as failed, as the package does (see
// the package documentation of internal/rules/nl), and the result is
// partial: the other assertions are unknown.
func (run *xslRun) result(e entry, data []byte) (result, error) {
	res := newResult()
	xsl, ok := run.xsl[e.Name]
	if !ok {
		return res, fmt.Errorf("not in %s", run.list)
	}
	rep, err := parseSVRL(data)
	if err != nil {
		return res, err
	}
	index := run.indexes[xsl]
	syntax := syntaxOf(xsl)
	res.Note = filepath.Base(xsl)
	fire := func(a xslAssertion) {
		if nlPatterns[a.Pattern] {
			run.fired[a.key(syntax)] = true
		}
	}
	if rep.errCode != "" || rep.errMsg != "" {
		a, ok := index.at(rep.errLine)
		if !ok {
			res.Failure = fmt.Sprintf("%s, line %d of %s: %s", rep.errCode, rep.errLine, filepath.Base(xsl), rep.errMsg)
			return res, nil
		}
		res.add(a.ID, flagLevel(a.Flag))
		res.Stopped = fmt.Sprintf("%s in the test of %s: %s", rep.errCode, a.ID, rep.errMsg)
		fire(a)
		return res, nil
	}
	for _, f := range rep.findings {
		res.add(f.id, flagLevel(f.flag))
		for _, a := range index.asserts {
			if a.ID == f.id && a.Message == f.message {
				fire(a)
				break
			}
		}
	}
	return res, nil
}

// neverFired is an assertion of an NLCIUS pattern that failed on no invoice.
type neverFired struct {
	Syntax  string `json:"syntax"`
	Pattern string `json:"pattern"`
	ID      string `json:"id"`
	Flag    string `json:"flag"`
	Message string `json:"message"`
	Known   string `json:"known,omitempty"` // justification
}

// where is the origin that known.txt gives for an assertion that never fails:
// syntax/pattern.
func (n neverFired) where() string { return n.Syntax + "/" + n.Pattern }

// neverFired lists the assertions of the NLCIUS patterns of the stylesheets
// that failed on no invoice, each once, sorted.
func (run *xslRun) neverFired() []neverFired {
	var out []neverFired
	seen := map[string]bool{}
	xsls := make([]string, 0, len(run.indexes))
	for x := range run.indexes {
		xsls = append(xsls, x)
	}
	sort.Strings(xsls)
	for _, xsl := range xsls {
		syntax := syntaxOf(xsl)
		for _, a := range run.indexes[xsl].asserts {
			k := a.key(syntax)
			if !nlPatterns[a.Pattern] || run.fired[k] || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, neverFired{Syntax: syntax, Pattern: a.Pattern, ID: a.ID, Flag: a.Flag, Message: a.Message})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].where() != out[j].where() {
			return out[i].where() < out[j].where()
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// oursNL validates an invoice of the NLCIUS comparison with this package, the
// Dutch rules selected as the validator selects the NPa artefacts, and keeps
// the findings of the rule sets that the XSLT include: EN 16931 (CEN rules,
// and the UBL syntax rules) and the Dutch rules. The findings of the XML
// schema, of the Factur-X profiles and of Peppol are left out.
func oursNL(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, &validate.Options{Rules: validate.Default | validate.Netherlands})
	for _, f := range rep.Findings {
		switch f.Source {
		case validate.SourceEN16931, validate.SourceNL:
			res.add(f.Rule, f.Severity.String())
		default:
			res.other(f.Rule)
		}
	}
	return res
}
