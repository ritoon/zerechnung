package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// reference is a reference validator.
type reference struct {
	Key     string                       // kosit, mustang, npa: the name used in known.txt
	Title   string                       // for the report
	dir     string                       // sub-directory of the work directory holding its reports
	report  func(e entry) string         // name of the report of an invoice
	parse   func([]byte) (result, error) // reads a report; nil for npa, whose reports npaRun.result reads
	applies func(e entry) bool           // the validator was given the invoice
	ours    func(string, []byte) result  // validates the invoice with this package
	levels  bool                         // warnings and informations are compared too, not only errors
}

var references = []reference{
	{
		Key:     "kosit",
		Title:   "KoSIT validator 1.6.3, configuration XRechnung 3.0.2 du 2026-08-31",
		dir:     "kosit",
		report:  func(e entry) string { return strings.TrimSuffix(e.Name, ".xml") + "-report.xml" },
		parse:   parseKoSIT,
		applies: func(e entry) bool { return !e.isPDF() && !e.nl() },
		ours:    ours,
	},
	{
		Key:     "mustang",
		Title:   "Mustang 2.26.0",
		dir:     "mustang",
		report:  func(e entry) string { return e.Name + ".xml" },
		parse:   parseMustang,
		applies: func(e entry) bool { return !e.nl() && !e.variant() },
		ours:    ours,
	},
	{
		Key:     "npa",
		Title:   "XSLT de la NPa (Saxon-HE 12.10) : NLCIUS-CII 1.0.3.13, SI-UBL 2.0.3.13, G-rekening 1.0.2.13",
		dir:     "npa",
		report:  func(e entry) string { return e.Name + ".svrl" },
		applies: func(e entry) bool { return e.nl() },
		ours:    oursNL,
		levels:  true,
	},
}

// comparison is the comparison of one invoice with one reference.
type comparison struct {
	Reference  string   `json:"reference"`
	Invoice    string   `json:"invoice"`
	Negative   bool     `json:"negative"`
	Ours       result   `json:"ours"`
	Ref        result   `json:"ref"`
	Expected   []string `json:"expected,omitempty"`   // rules that the mutation case of a variant expects
	Known      []string `json:"known,omitempty"`      // justified differences, "+RULE: reason"
	Unexpected []string `json:"unexpected,omitempty"` // "+RULE" raised as an error by this package only, "-RULE" by the reference only, "?RULE" expected by the mutation case and not raised by the reference
}

func (c *comparison) identical() bool { return len(c.Known) == 0 && len(c.Unexpected) == 0 }

// knownDiff is a line of known.txt: a difference that is understood and
// justified.
type knownDiff struct {
	ref    string // kosit, mustang, npa or *
	glob   string // origin of the invoice, * matching any string (/ included); for ~RULE, syntax/pattern
	diff   string // +RULE, -RULE, ?RULE (expected by a mutation case, not raised by the reference) or ~RULE (an NLCIUS assertion that the XSLT never raise); a final * matches a prefix
	reason string
	line   int
	used   bool
}

func (k *knownDiff) match(ref, src, diff string) bool {
	if k.ref != "*" && k.ref != ref {
		return false
	}
	return glob(k.glob, src) && matchRule(k.diff, diff)
}

// glob reports whether s matches pattern, where * matches any string,
// slashes included, and every other character itself.
func glob(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return len(s) >= len(last) && strings.HasSuffix(s, last)
}

// readKnown reads known.txt: one difference per line, "reference invoice
// difference reason", # starting a comment. A difference is +RULE, -RULE,
// ?RULE or ~RULE.
func readKnown(name string) ([]*knownDiff, error) {
	if name == "" {
		return nil, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []*knownDiff
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.ContainsRune("+-?~", rune(fields[2][0])) {
			return nil, fmt.Errorf("%s:%d: want \"reference invoice ±RULE reason\" or \"reference syntax/pattern ~RULE reason\"", name, n)
		}
		out = append(out, &knownDiff{
			ref: fields[0], glob: fields[1], diff: fields[2],
			reason: strings.Join(fields[3:], " "), line: n,
		})
	}
	return out, sc.Err()
}

// report is the JSON report.
type report struct {
	Comparisons   []comparison    `json:"comparisons"`
	NeverFired    []neverFired    `json:"never_fired,omitempty"`
	KoSITCoverage []kositCoverage `json:"kosit_coverage,omitempty"`
}

func compare(work, kositConfig, knownFile, mdFile, jsonFile string) (bool, error) {
	entries, err := readIndex(work)
	if err != nil {
		return false, err
	}
	expected, err := readExpectedRules(work)
	if err != nil {
		return false, err
	}
	var xsls []kositXSL
	if kositConfig != "" {
		if xsls, err = kositStylesheets(kositConfig); err != nil {
			return false, err
		}
	}
	syntax := map[string]string{} // origin → cii, ubl
	failed, err := kositFailures(filepath.Join(work, "kosit.log"))
	if err != nil {
		return false, err
	}
	known, err := readKnown(knownFile)
	if err != nil {
		return false, err
	}
	npa, err := newNPaRun(work)
	if err != nil {
		return false, err
	}
	justify := func(ref, src, d string) (string, bool) {
		for _, k := range known {
			if k.match(ref, src, d) {
				k.used = true
				return k.reason, true
			}
		}
		return "", false
	}
	var all []comparison
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(work, "in", e.Name))
		if err != nil {
			return false, err
		}
		syntax[e.Source] = docSyntax(data)
		mine := map[string]result{} // by reference
		for _, ref := range references {
			if !ref.applies(e) {
				continue
			}
			rdata, err := os.ReadFile(filepath.Join(work, ref.dir, ref.report(e)))
			if err != nil {
				return false, fmt.Errorf("%s (%s): %w", e.Source, ref.Key, err)
			}
			var r result
			if ref.Key == "npa" {
				r, err = npa.result(e, rdata)
			} else {
				r, err = ref.parse(rdata)
			}
			if err != nil {
				return false, fmt.Errorf("%s: %s report: %w", e.Source, ref.Key, err)
			}
			if f, ok := failed[e.Name]; ok && ref.Key == "kosit" {
				r.Stopped = kositFailure + f
			}
			if !r.Covered {
				if e.variant() {
					// A variant is written for the reference: one it does
					// not check would no longer test its rules.
					all = append(all, comparison{Reference: ref.Key, Invoice: e.Source, Ref: r,
						Unexpected: []string{"!" + ref.Key + ": " + r.Note}})
				}
				continue
			}
			m, ok := mine[ref.Key]
			if !ok {
				m = ref.ours(e.Name, data)
				mine[ref.Key] = m
			}
			c := comparison{Reference: ref.Key, Invoice: e.Source, Negative: e.negative(), Ours: m, Ref: r, Expected: expected[e.Source]}
			// The warnings and informations of the variants are compared too:
			// many of the rules they break are warnings.
			diffs := diff(m.Rules, r, ref.levels || e.variant())
			diffs = append(diffs, missing(c.Expected, r, diffs)...)
			if m.Failure != "" {
				diffs = append(diffs, "!ZeRechnung: "+m.Failure)
			}
			if r.Failure != "" {
				diffs = append(diffs, "!"+ref.Key+": "+r.Failure)
			}
			for _, d := range diffs {
				if why, ok := justify(ref.Key, e.Source, d); ok {
					c.Known = append(c.Known, d+" : "+why)
				} else {
					c.Unexpected = append(c.Unexpected, d)
				}
			}
			all = append(all, c)
		}
	}

	ok := true
	for _, c := range all {
		if len(c.Unexpected) > 0 {
			ok = false
		}
	}
	never := npa.neverFired()
	for i, n := range never {
		if why, found := justify("npa", n.where(), "~"+n.ID); found {
			never[i].Known = why
		} else {
			ok = false
		}
	}
	var unused []string
	for _, k := range known {
		if !k.used {
			unused = append(unused, fmt.Sprintf("known.txt:%d: %s %s %s matches no difference", k.line, k.ref, k.glob, k.diff))
		}
	}
	coverage := coverageOf(xsls, all, syntax)
	md := markdown(all, never, unused, coverage)
	fmt.Print(md)
	if mdFile != "" {
		if err := os.WriteFile(mdFile, []byte(md), 0o644); err != nil {
			return false, err
		}
	}
	if jsonFile != "" {
		b, err := json.MarshalIndent(report{Comparisons: all, NeverFired: never, KoSITCoverage: coverage}, "", "  ")
		if err != nil {
			return false, err
		}
		if err := os.WriteFile(jsonFile, append(b, '\n'), 0o644); err != nil {
			return false, err
		}
	}
	return ok, nil
}

// missing lists, as "?RULE", the rules that the mutation case of a variant
// expects and that the reference does not raise, at any level: the variant
// no longer tests them at the reference. A rule that this package raises
// alone is already the difference "+RULE", and only the rules that take part
// in the comparison count. When the reference stopped on a dynamic error,
// its other assertions are unknown.
func missing(expected []string, ref result, diffs []string) []string {
	if ref.Stopped != "" {
		return nil
	}
	var out []string
	for _, rule := range expected {
		if _, ok := ref.Rules[rule]; ok || !compared(rule) || slices.Contains(diffs, "+"+rule) {
			continue
		}
		out = append(out, "?"+rule)
	}
	return out
}

// equivalents maps the assertions of the Factur-X 1.09.2 schematrons that
// Mustang applies to the rules of this package that check the same element
// against the same code list under another identifier. Mustang applies the
// Factur-X schematron of EN 16931 to an EN 16931 XML on its own, where this
// package applies the CEN rules, as KoSIT does: their BR-CL rules check the
// same elements against the CEN code lists. This package checks the
// specification identifier BT-24 with FX-PROFILE. A final * matches a
// prefix.
var equivalents = map[string][]string{
	"FX-SCH-A-000493": {"FX-PROFILE-*"}, // BASIC: BT-24
	"FX-SCH-A-000556": {"FX-PROFILE-*"}, // EN 16931: BT-24
	"FX-SCH-A-000636": {"FX-PROFILE-*"}, // EXTENDED: BT-24
	"FX-SCH-A-000571": {"BR-CL-25"},     // EN 16931: scheme of BT-49 (EAS)
	"FX-SCH-A-000581": {"BR-CL-11"},     // EN 16931: scheme of BT-30 (ICD)
	"FX-SCH-A-000612": {"BR-CL-23"},     // EN 16931: unit of BT-149
	"FX-SCH-A-000613": {"BR-CL-23"},     // EN 16931: unit of BT-130
}

func matchRule(pattern, rule string) bool {
	if p, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(rule, p)
	}
	return pattern == rule
}

// equivalentTarget reports whether a rule of this package is the equivalent
// of an assertion of a reference.
func equivalentTarget(rule string) bool {
	for _, targets := range equivalents {
		for _, t := range targets {
			if matchRule(t, rule) {
				return true
			}
		}
	}
	return false
}

// same reports whether the rule refRule of the reference and the rule mine
// of this package are the same check.
func same(refRule, mine string) bool {
	if refRule == mine {
		return true
	}
	for _, t := range equivalents[refRule] {
		if matchRule(t, mine) {
			return true
		}
	}
	return false
}

// errorsOf returns the rules of a set raised as errors.
func errorsOf(m map[string]string) []string {
	var out []string
	for _, r := range sortedRules(m) {
		if m[r] == levelError {
			out = append(out, r)
		}
	}
	return out
}

// diff lists the rules raised as errors by one side only: "+RULE" for this
// package, "-RULE" for the reference, a rule and its equivalent (see
// equivalents) being the same. A rule raised as an error by one side and as
// a warning or an information by the other is a difference: the level
// decides the verdict. With levels, the warnings and informations are
// compared too: a rule raised at a higher level by this package is "+RULE",
// at a higher level by the reference "-RULE". When the reference stopped (on
// a dynamic error, or after the XML schema), only the rules it raised are
// compared.
func diff(mine map[string]string, ref result, levels bool) []string {
	if levels {
		return diffLevels(mine, ref)
	}
	var out []string
	mineErr, refErr := errorsOf(mine), errorsOf(ref.Rules)
	if ref.Stopped != "" {
		mineErr = slices.DeleteFunc(mineErr, func(m string) bool {
			return !slices.ContainsFunc(refErr, func(r string) bool { return same(r, m) })
		})
	}
	for _, m := range mineErr {
		if !compared(m) {
			continue // an equivalent only
		}
		if !slices.ContainsFunc(refErr, func(r string) bool { return same(r, m) }) {
			out = append(out, "+"+m)
		}
	}
	for _, r := range refErr {
		if !slices.ContainsFunc(mineErr, func(m string) bool { return same(r, m) }) {
			out = append(out, "-"+r)
		}
	}
	return out
}

// diffLevels compares the rules at every level, under their identifiers.
func diffLevels(mine map[string]string, ref result) []string {
	rules := map[string]bool{}
	for r := range ref.Rules {
		rules[r] = true
	}
	if ref.Stopped == "" {
		for m := range mine {
			rules[m] = true
		}
	}
	var out []string
	for _, r := range sortedRules(rules) {
		switch lm, lr := rank(mine[r]), rank(ref.Rules[r]); {
		case lm > lr:
			out = append(out, "+"+r)
		case lm < lr:
			out = append(out, "-"+r)
		}
	}
	return out
}
