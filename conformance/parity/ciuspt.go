package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// The CIUS-PT comparison: the schematron of eSPap (FX_PT_SCH), compiled into
// an XSLT by SchXslt 1.10.1 (parity.sh writes it to the work directory as
// ptXSL) and run by Saxon-HE 12.10 (CiusPtBatch.java), against package
// internal/rules/pt. The rules are compared at every level and on the
// element where they are raised: the compared rules are keyed RULE@PATH,
// PATH being the canonical location of the element (see ptPath).

// ptXSL is the name of the XSLT compiled from the CIUS-PT schematron, in the
// work directory.
const ptXSL = "cius-pt.xsl"

// indexSchXslt reads the assertions of an XSLT that SchXslt 1.10.1 compiles
// from a schematron: each assertion (report) is an xsl:if whose test fails
// (succeeds) into an svrl:failed-assert (svrl:successful-report) that holds
// the id and the flag as attributes, and the message as svrl:text. The lines
// of each assertion are those of the start tag of its xsl:if, which Saxon
// gives for an error in the test; it gives those of the start tag of the
// schxslt:rule element that wraps the assertions of a rule instead when that
// rule holds a single assertion (BR-27 of the CIUS-RO schematron), which
// the index records as the template of the assertion. The index records too
// the xsl:template elements, with the variables of the rules (the
// xsl:variable elements children of an xsl:template), and the template and
// test of each assertion: Saxon may report an error in a variable at the line
// of the variable, or at the start tag of the template (see byVariable).
func indexSchXslt(file string) (xslIndex, error) {
	var idx xslIndex
	data, err := os.ReadFile(file)
	if err != nil {
		return idx, err
	}
	const (
		nsXSL     = "http://www.w3.org/1999/XSL/Transform"
		nsSVRL    = "http://purl.oclc.org/dsdl/svrl"
		nsSchXslt = "https://doi.org/10.5281/zenodo.1495494"
	)
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		cur      *xslAssertion
		from, to int    // lines of the last xsl:if start tag
		test     string // and its test
		text     strings.Builder
		in       bool // reading the svrl:text of cur
		rule     = -1 // index of the schxslt:rule being read, in idx.templates
		depth    int  // of the element being read
		sc       = -1 // index of the xsl:template being read, in idx.scopes
		scDepth  int  // its depth
		open     = -1 // index of the variable of the rule being read, in its vars
		start    int64
	)
	for {
		before, _ := d.InputPos()
		offset := d.InputOffset()
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return idx, fmt.Errorf("%s: %w", file, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case t.Name.Space == nsXSL && t.Name.Local == "template":
				after, _ := d.InputPos()
				idx.scopes = append(idx.scopes, scope{from: before, to: after})
				sc, scDepth = len(idx.scopes)-1, depth
			case t.Name.Space == nsXSL && t.Name.Local == "variable" && sc >= 0 && depth == scDepth+1:
				s := &idx.scopes[sc]
				s.vars = append(s.vars, xslVariable{name: attr(t, "name"), from: before})
				open, start = len(s.vars)-1, offset
			case t.Name.Space == nsXSL && t.Name.Local == "if":
				from = before
				to, _ = d.InputPos()
				test = attr(t, "test")
			case t.Name.Space == nsSchXslt && t.Name.Local == "rule":
				after, _ := d.InputPos()
				idx.templates = append(idx.templates, template{from: before, to: after})
				rule = len(idx.templates) - 1
			case t.Name.Space == nsSVRL && (t.Name.Local == "failed-assert" || t.Name.Local == "successful-report"):
				cur = &xslAssertion{ID: attr(t, "id"), Flag: attr(t, "flag"), from: from, to: to, tmpl: rule, test: test, scope: sc}
			case cur != nil && t.Name.Space == nsSVRL && t.Name.Local == "text":
				in = true
				text.Reset()
			}
		case xml.CharData:
			if in {
				text.Write(t)
			}
		case xml.EndElement:
			depth--
			switch {
			case t.Name.Space == nsXSL && t.Name.Local == "variable" && open >= 0 && depth == scDepth:
				v := &idx.scopes[sc].vars[open]
				v.to, _ = d.InputPos()
				v.src = string(data[start:d.InputOffset()])
				open = -1
			case t.Name.Space == nsXSL && t.Name.Local == "template" && depth == scDepth-1:
				sc = -1
			case t.Name.Space == nsSchXslt && t.Name.Local == "rule":
				rule = -1
			case cur == nil:
			case t.Name.Space == nsSVRL && t.Name.Local == "text":
				cur.Message = normalize(text.String())
				in = false
			case t.Name.Space == nsSVRL && (t.Name.Local == "failed-assert" || t.Name.Local == "successful-report"):
				cur.ID = assertionID(cur.ID, cur.Message)
				if cur.tmpl >= 0 {
					idx.templates[cur.tmpl].asserts++
				}
				idx.asserts = append(idx.asserts, *cur)
				cur = nil
			}
		}
	}
	if len(idx.asserts) == 0 {
		return idx, fmt.Errorf("%s: no assertion", file)
	}
	return idx, nil
}

// messageLine matches the lines of the stylesheet that Saxon names in the
// message of a dynamic error, such as the use of a variable evaluated lazily
// ("Found while atomizing the first operand of '<=' in {$payableAmount} on
// line 556").
var messageLine = regexp.MustCompile(` on line ([0-9]+)`)

// atError returns the assertion whose test raised the dynamic error of rep:
// the one at the line of the error, else at a line that its message names
// (the use of a variable evaluated lazily, whose declaration raised the
// error), else at a line of its call stack (the call of an xsl:function that
// raised it, as u:ctrlOIB in the CIUS-HR schematron), in this order. When no
// line is that of an assertion, the error may come from a variable of a rule
// that Saxon evaluated before its use (see byVariable): atError returns then
// the assertions that may have raised it, in the order of the stylesheet.
func (idx xslIndex) atError(rep svrlReport) []xslAssertion {
	lines := []int{rep.errLine}
	for _, m := range messageLine.FindAllStringSubmatch(rep.errMsg, -1) {
		n, _ := strconv.Atoi(m[1])
		lines = append(lines, n)
	}
	lines = append(lines, rep.errStack...)
	for _, n := range lines {
		if a, ok := idx.at(n); ok {
			return []xslAssertion{a}
		}
	}
	for _, n := range lines {
		if as := idx.byVariable(n); len(as) > 0 {
			return as
		}
	}
	return nil
}

// byVariable returns the assertions that may have raised a dynamic error
// that Saxon reports at line in the variables of a rule: at the line of a
// variable, or at the start tag of the xsl:template that holds the rule,
// without telling which variable (Saxon 12.10 does so for the variable
// payableAmount of the CIUS-HR schematron). As the package raises the error
// of a variable where it is used (see bindLets in internal/schematron), the
// error of a variable comes from the first assertion of the rule whose test
// uses it, directly or through another variable of the rule; at the start
// tag, it may come from any variable. byVariable returns these assertions in
// the order of the stylesheet, and nothing when line is not in the variables
// or the start tag of a template, or when no test uses the variables.
func (idx xslIndex) byVariable(line int) []xslAssertion {
	for i, s := range idx.scopes {
		var names []string
		for _, v := range s.vars {
			if s.from <= line && line <= s.to || v.from <= line && line <= v.to {
				names = append(names, v.name)
			}
		}
		if len(names) == 0 {
			continue
		}
		first := map[int]bool{} // index in idx.asserts
		for _, name := range names {
			used := s.usersOf(name)
			for j, a := range idx.asserts {
				if a.scope == i && slices.ContainsFunc(used, func(n string) bool { return usesVariable(a.test, n) }) {
					first[j] = true
					break
				}
			}
		}
		var out []xslAssertion
		for j, a := range idx.asserts {
			if first[j] {
				out = append(out, a)
			}
		}
		return out
	}
	return nil
}

// usersOf returns a variable of s and the variables of s that use it,
// directly or through another one.
func (s scope) usersOf(name string) []string {
	out := []string{name}
	for changed := true; changed; {
		changed = false
		for _, v := range s.vars {
			if slices.Contains(out, v.name) {
				continue
			}
			if slices.ContainsFunc(out, func(n string) bool { return usesVariable(v.src, n) }) {
				out = append(out, v.name)
				changed = true
			}
		}
	}
	return out
}

// usesVariable reports whether an XPath expression (or an XSLT element)
// refers to the variable name: $name, not followed by a character of a name.
func usesVariable(expr, name string) bool {
	for rest := expr; ; {
		i := strings.Index(rest, "$"+name)
		if i < 0 {
			return false
		}
		rest = rest[i+1+len(name):]
		if r, _ := utf8.DecodeRuneInString(rest); rest == "" || !(r == '-' || r == '.' || r == '_' || r == ':' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return true
		}
	}
}

// schRun is the run of a schematron compiled by SchXslt, the CIUS-PT, CIUS-RO
// or CIUS-HR one: the staged invoices given to Saxon (list), and the
// assertions of the compiled XSLT (xsl).
type schRun struct {
	list, xsl string
	files     map[string]bool // staged name → given to Saxon
	index     xslIndex
	keep      func(id string) bool // the assertions compared; nil for all
}

// newSchRun reads the list of the invoices given to Saxon and, when there is
// one, the compiled XSLT, both in the work directory. keep tells the
// assertions to compare, nil for all.
func newSchRun(work, list, xsl string, keep func(string) bool) (*schRun, error) {
	run := &schRun{list: list, xsl: xsl, files: map[string]bool{}, keep: keep}
	data, err := os.ReadFile(filepath.Join(work, list))
	if os.IsNotExist(err) {
		return run, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			run.files[filepath.Base(line)] = true
		}
	}
	if len(run.files) == 0 {
		return run, nil
	}
	if run.index, err = indexSchXslt(filepath.Join(work, xsl)); err != nil {
		return nil, err
	}
	return run, nil
}

// compared reports whether an assertion of the schematron takes part in the
// comparison.
func (run *schRun) compared(id string) bool { return run.keep == nil || run.keep(id) }

// result turns the report of an invoice into a result: every failed
// assertion (fired report) compared, at its level and on its element. When
// the transformation stopped on a dynamic error, the assertion whose test
// raised it counts as failed, as the package does (see the package
// documentation of internal/schematron), and the result is partial: it holds
// that assertion only, without element, as Saxon does not say on which
// element it stopped, and nothing when the assertion is not compared. When
// the error may come from several assertions (an error in a variable of a
// rule, see byVariable), the result holds the compared ones as alternatives,
// which resolve settles against the package.
func (run *schRun) result(e entry, data []byte) (result, error) {
	res := newResult()
	if !run.files[e.Name] {
		return res, fmt.Errorf("not in %s", run.list)
	}
	rep, err := parseSVRL(data)
	if err != nil {
		return res, err
	}
	if rep.errCode != "" || rep.errMsg != "" {
		as := run.index.atError(rep)
		if len(as) == 0 {
			res.Failure = fmt.Sprintf("%s, line %d of %s: %s", rep.errCode, rep.errLine, run.xsl, rep.errMsg)
			return res, nil
		}
		var ids []string
		for _, a := range as {
			ids = append(ids, a.ID)
			if run.compared(a.ID) {
				res.alt = append(res.alt, alternative{a.ID, flagLevel(a.Flag)})
			}
		}
		if len(res.alt) == 1 {
			res.add(res.alt[0].rule, res.alt[0].level)
			res.alt = nil
		}
		res.Stopped = fmt.Sprintf("%s in the test of %s: %s", rep.errCode, strings.Join(ids, " or "), rep.errMsg)
		return res, nil
	}
	for _, f := range rep.findings {
		if !run.compared(f.id) {
			continue
		}
		p, err := saxonPath(f.location)
		if err != nil {
			return res, fmt.Errorf("%s: %w", f.id, err)
		}
		res.add(f.id+"@"+p, flagLevel(f.flag))
	}
	return res, nil
}

// saxonPath returns the canonical location (see ptPath) of a location that
// SchXslt writes: /Q{ns}local[n] for each element, @Q{ns}name for an
// attribute.
func saxonPath(loc string) (string, error) {
	if loc == "/" {
		return loc, nil // the document node
	}
	var b strings.Builder
	for s := loc; s != ""; {
		if s[0] != '/' {
			return "", fmt.Errorf("bad location %q", loc)
		}
		s = s[1:]
		b.WriteByte('/')
		if rest, ok := strings.CutPrefix(s, "@"); ok {
			b.WriteByte('@')
			s = rest
		}
		if strings.HasPrefix(s, "Q{") {
			i := strings.IndexByte(s, '}')
			if i < 0 {
				return "", fmt.Errorf("bad location %q", loc)
			}
			s = s[i+1:]
		}
		end := strings.IndexByte(s, '/')
		if end < 0 {
			end = len(s)
		}
		if end == 0 {
			return "", fmt.Errorf("bad location %q", loc)
		}
		b.WriteString(s[:end])
		s = s[end:]
	}
	return b.String(), nil
}

// ptPath returns the canonical location of the path of a finding of this
// package (/Invoice/cac:InvoiceLine[2]/cbc:ID, a step without rank being the
// only element of its name): one step per element, its local name and its
// rank among the siblings of the same name, /Invoice[1]/InvoiceLine[2]/ID[1],
// as SchXslt writes it once the namespaces are dropped.
func ptPath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	steps := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range steps {
		name, rank, ok := strings.Cut(s, "[")
		if j := strings.LastIndexByte(name, ':'); j >= 0 {
			name = name[j+1:]
		}
		if ok {
			steps[i] = name + "[" + rank
		} else {
			steps[i] = name + "[1]"
		}
	}
	return "/" + strings.Join(steps, "/")
}

// unlocated drops the elements of rules keyed RULE@PATH, keeping the highest
// level of each rule.
func unlocated(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, l := range m {
		rule, _, _ := strings.Cut(k, "@")
		if rank(l) > rank(out[rule]) {
			out[rule] = l
		}
	}
	return out
}

// oursPT validates an invoice of the CIUS-PT comparison with this package as
// an invoice of the CIUS-PT, whatever its BT-24 (the validator of eSPap
// applies the schematron to the invoices it receives, and a variant may
// change or remove BT-24), and keeps the findings of the rules of the
// schematron: those of the EN 16931 rule set, which the schematron replaces
// for a CIUS-PT invoice, and those of the Portugal rule set but the checks
// of the package (PT-NIF, PT-SYNTAX), which the schematron does not have.
// The findings are keyed RULE@PATH, see ptPath.
func oursPT(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, &validate.Options{Profile: invoice.ProfileCIUSPT, Rules: validate.EN16931 | validate.Portugal})
	for _, f := range rep.Findings {
		if f.Source == validate.SourceEN16931 || f.Source == validate.SourcePT && !strings.HasPrefix(f.Rule, "PT-") {
			res.add(f.Rule+"@"+ptPath(f.Path), f.Severity.String())
		} else {
			res.other(f.Rule)
		}
	}
	return res
}
