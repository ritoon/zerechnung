package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/validate"
)

// Levels of a rule, the same for every validator.
const (
	levelError   = "error"
	levelWarning = "warning"
	levelInfo    = "info"
)

// schemaToken stands for any finding of the XML schema (or of the XML
// well-formedness) in the compared sets: the validators word these errors
// differently (Xerces or Saxon messages, XSD-* rules), so only their presence
// is compared.
const schemaToken = "SCHEMA"

// result is what one validator says about one invoice.
type result struct {
	Covered bool              `json:"covered"`           // the validator checked the invoice
	Note    string            `json:"note,omitempty"`    // scenario, or why the invoice is not covered
	Rules   map[string]string `json:"rules"`             // compared rule (or equivalent, see equivalents) → highest level
	Other   []string          `json:"other,omitempty"`   // findings outside the comparison
	Failure string            `json:"failure,omitempty"` // the validator failed on the invoice
	Stopped string            `json:"stopped,omitempty"` // the validator stopped on a dynamic error: Rules holds the assertion that raised it only

	// alt holds the rules that may have raised the dynamic error of Stopped
	// when the report does not tell which one (see resolve).
	alt []alternative
}

// alternative is a rule that may have raised a dynamic error, at its level.
type alternative struct{ rule, level string }

func newResult() result { return result{Covered: true, Rules: map[string]string{}} }

// resolve settles the rule that raised the dynamic error of r among its
// alternatives: the first one that mine (the rules of this package, without
// element) raises, else the first one, which then makes a difference.
func (r *result) resolve(mine map[string]string) {
	if len(r.alt) == 0 {
		return
	}
	pick := r.alt[0]
	for _, a := range r.alt {
		if rank(mine[a.rule]) > 0 {
			pick = a
			break
		}
	}
	r.add(pick.rule, pick.level)
	r.alt = nil
}

// add records a rule at a level, keeping the highest level.
func (r *result) add(rule, level string) {
	if old, ok := r.Rules[rule]; !ok || rank(level) > rank(old) {
		r.Rules[rule] = level
	}
}

// rank orders the levels: error, warning, information, then no level ("",
// the rule is not raised).
func rank(level string) int {
	switch level {
	case levelError:
		return 3
	case levelWarning:
		return 2
	case "":
		return 0
	}
	return 1
}

// valid reports whether no compared rule is an error.
func (r *result) valid() bool {
	for _, l := range r.Rules {
		if l == levelError {
			return false
		}
	}
	return true
}

func (r *result) other(s string) {
	r.Other = append(r.Other, s)
}

// compared tells whether a rule identifier takes part in the comparison: the
// rules of the official schematrons that this package also implements under
// the same identifiers, and schemaToken. The BR-FR rules are left out:
// Mustang 2.26.0 runs the French schematron 1.3.0 on invoices between two
// French parties, this package runs version 1.4.0.04, on demand only.
func compared(rule string) bool {
	switch {
	case rule == schemaToken:
		return true
	case strings.HasPrefix(rule, "BR-FR-"):
		return false
	case strings.HasPrefix(rule, "BR-"),
		strings.HasPrefix(rule, "CII-"),
		strings.HasPrefix(rule, "UBL-"),
		strings.HasPrefix(rule, "FX-SCH-"),
		strings.HasPrefix(rule, "PEPPOL-"):
		return true
	}
	return false
}

// ours validates an invoice with this package: the default options, which
// add the German rules to an XRECHNUNG invoice and the Peppol rules to a
// Peppol invoice, like the reference validators. The findings on the PDF
// container are left out (the references check PDF/A with veraPDF, not with
// the rules of this package).
func ours(name string, data []byte) result {
	res := newResult()
	var rep *validate.Report
	if strings.HasSuffix(name, ".pdf") {
		r, att, err := zerechnung.Check(data, nil)
		switch {
		case err != nil:
			res.Failure = err.Error()
			return res
		case att == nil:
			res.Note = "no invoice in the PDF"
		}
		rep = r
	} else {
		rep = validate.XML(data, nil)
	}
	for _, f := range rep.Findings {
		level := f.Severity.String()
		switch {
		case f.Source == validate.SourcePDF:
			res.other(f.Rule)
		case f.Source == validate.SourceSyntax:
			res.add(schemaToken, levelError)
		case compared(f.Rule) || equivalentTarget(f.Rule):
			res.add(f.Rule, level)
		default:
			res.other(f.Rule)
		}
	}
	return res
}

// kositLevel maps the levels of the KoSIT report (error, warning,
// information).
func kositLevel(s string) string {
	switch s {
	case "error":
		return levelError
	case "warning":
		return levelWarning
	}
	return levelInfo
}

// parseKoSIT reads a report of the KoSIT validator (rep:report). The messages
// of a validation step against an XML schema, and a failed well-formedness
// check, count as schemaToken; the messages of the schematron steps carry the
// rule identifier in their code attribute, and the level of the schematron.
// The levels that the scenario changes (s:customLevel, which the report
// repeats) replace them, as in the assessment of the document. An invoice
// that matches no scenario of the configuration is not covered. KoSIT runs
// no schematron on an invoice that the XML schema rejects: its result is then
// Stopped, and only the schema is compared.
func parseKoSIT(data []byte) (result, error) {
	res := newResult()
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		stack    []string
		matched  bool
		noMatch  bool
		resource string // name of the resource of the current step
		scenario string
		schema   bool                  // a message of a schema step
		rules    bool                  // a schematron step ran
		custom   = map[string]string{} // rule → level set by the scenario
		customAt string                // level of the s:customLevel being read
		messages [][2]string           // compared rule, level
		text     strings.Builder
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
			switch t.Name.Local {
			case "scenarioMatched":
				matched = true
			case "noScenarioMatched":
				noMatch = true
			case "validationStepResult":
				resource = ""
			case "message":
				code, level := attr(t, "code"), kositLevel(attr(t, "level"))
				switch {
				case noMatch || strings.Contains(resource, "Schema") && !strings.Contains(resource, "Schematron"):
					res.add(schemaToken, levelError)
					schema = true
				case compared(code):
					messages = append(messages, [2]string{code, level})
				default:
					res.other(code)
				}
			case "customLevel":
				customAt = kositLevel(attr(t, "level"))
			}
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			switch {
			case t.Name.Local == "customLevel":
				custom[strings.TrimSpace(text.String())] = customAt
			case t.Name.Local == "name" && len(stack) >= 2:
				switch stack[len(stack)-2] {
				case "resource":
					resource = strings.TrimSpace(text.String())
					if strings.Contains(resource, "Schematron") && len(stack) >= 3 && stack[len(stack)-3] == "validationStepResult" {
						rules = true
					}
				case "scenario":
					scenario = strings.TrimSpace(text.String())
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	for _, m := range messages {
		if l, ok := custom[m[0]]; ok {
			m[1] = l
		}
		res.add(m[0], m[1])
	}
	switch {
	case matched:
		res.Note = scenario
		if schema && !rules {
			res.Stopped = "XML Schema : KoSIT n'exécute pas les schematrons"
		}
	case noMatch && len(res.Rules) > 0:
		res.Note = "not well-formed"
	case noMatch:
		res.Covered = false
		res.Note = "no scenario of the configuration matches"
	default:
		return res, fmt.Errorf("not a KoSIT report")
	}
	return res, nil
}

// kositFailures reads the table that the KoSIT validator prints at the end of
// its log and returns, by staged file name (0042.xml), the description of the
// documents it could not assess (UNDEFINED): a schematron that stops on a
// dynamic error, such as an XPath cast of a sequence of two items, leaves in
// the report a valid step without any message, and only the log tells. Each
// row of the table spans several lines, the first of which fills the columns
// Schema, Schematron and Acceptance; the file name ends the File column, and
// the description is cut every 60 characters, within words. The table ends
// the log. A missing log gives no failure.
func kositFailures(file string) (map[string]string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var (
		results            bool
		name, desc, accept strings.Builder
		open               bool
	)
	flush := func() {
		if open && accept.String() == "UNDEFINED" {
			if m := stagedName.FindStringSubmatch(name.String()); m != nil {
				out[m[1]] = strings.Join(strings.Fields(desc.String()), " ")
			}
		}
		open = false
		name.Reset()
		desc.Reset()
		accept.Reset()
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if !results {
			results = strings.HasPrefix(line, "Results:")
			continue
		}
		// | File | Schema | Schematron | Acceptance | Error/Description |
		// A description may hold line breaks: the lines that do not start
		// with a cell continue it, and are left out.
		cols := strings.SplitN(line, "|", 6)
		if len(cols) != 6 || cols[0] != "" {
			continue
		}
		if strings.TrimSpace(cols[1]) == "File" {
			continue
		}
		if strings.TrimSpace(cols[2]) != "" {
			flush()
			open = true
			accept.WriteString(strings.TrimSpace(cols[4]))
		}
		if open {
			name.WriteString(strings.TrimSpace(cols[1]))
			desc.WriteString(strings.TrimSuffix(cols[5], "|"))
		}
	}
	flush()
	return out, nil
}

// kositFailure starts the Stopped of a KoSIT result that kositFailures
// reports.
const kositFailure = "erreur dynamique (kosit.log) : "

// stagedName is the name of a staged invoice at the end of a path: four
// digits at least (0042.xml, 15332.xml), as stage numbers the invoices.
var stagedName = regexp.MustCompile(`(\d{4,}\.xml)$`)

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// mustangID finds the rule identifier that Mustang appends to the message of
// a failed schematron assertion: "… [ID BR-CO-10] from /xslt/…".
var mustangID = regexp.MustCompile(`\[ID ([^\]\s]+)\]`)

// mustangSchemaSection is the type that Mustang gives to the errors of the
// XML schema.
const mustangSchemaSection = "18"

// parseMustang reads the result of ZUGFeRDValidator.validate: <validation>
// with a <pdf> and an <xml> part, each holding error, warning, notice, fatal
// and exception elements. The findings of the PDF part are outside the
// comparison, and so are the findings of the XML part that do not come from
// a schematron or from the XML schema (Mustang's own arithmetic checks).
func parseMustang(data []byte) (result, error) {
	res := newResult()
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		stack   []string
		item    string // severity element being read
		section string
		text    strings.Builder
		seen    bool
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "validation" {
				seen = true
			}
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "error", "warning", "notice", "fatal", "exception":
				if item == "" {
					item, section = t.Name.Local, attr(t, "type")
					text.Reset()
				}
			}
		case xml.CharData:
			if item != "" {
				text.Write(t)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			if item == "" || t.Name.Local != item {
				continue
			}
			level := levelError
			switch item {
			case "warning":
				level = levelWarning
			case "notice":
				level = levelInfo
			}
			msg := strings.TrimSpace(text.String())
			inPDF := false
			for _, s := range stack {
				if s == "pdf" {
					inPDF = true
				}
			}
			switch m := mustangID.FindStringSubmatch(msg); {
			case inPDF:
				res.other("pdf: " + clip(msg))
			case m != nil && compared(m[1]):
				res.add(m[1], level)
			case m != nil:
				res.other(m[1])
			case section == mustangSchemaSection:
				res.add(schemaToken, levelError)
			default:
				res.other(fmt.Sprintf("%s type %s: %s", item, section, clip(msg)))
			}
			item = ""
		}
	}
	if !seen {
		return res, fmt.Errorf("not a Mustang report")
	}
	return res, nil
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

// sortedRules returns the rules of a set, sorted.
func sortedRules[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
