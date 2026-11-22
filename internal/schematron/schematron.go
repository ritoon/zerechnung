// Package schematron runs ISO Schematron patterns whose contexts, variables
// and tests use a subset of XPath 2.0, the way an XSLT 2.0 processor runs
// the stylesheet a schematron compiles into. The rule sets of the national
// CIUS whose schematron is reproduced as published (packages
// internal/rules/pt and internal/rules/ro) generate their patterns and run
// them with this package.
//
// # Evaluation
//
// The expressions are evaluated by an interpreter of the subset of XPath 2.0
// they use (xpath.go, eval.go, funcs.go), with the semantics of an XSLT 2.0
// processor: comparison of untyped values as strings or as xs:double, exact
// xs:decimal arithmetic (a division keeps 18 decimals, rounded half down, as
// Saxon does), round() halves towards positive infinity, regular
// expressions of XPath. In each pattern, an element is checked by the first
// rule whose context it matches, and by no other. A test that raises an
// XPath error, which stops a validator built on an XSLT processor, counts as
// a failed assertion.
package schematron

import (
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Let is a variable of a pattern or of a rule.
type Let struct{ Name, Value string }

// Assertion is an assert of the schematron, or a report when Report is set:
// an assert fails when its test is false, a report when it is true.
type Assertion struct {
	ID     string
	Flag   rules.Severity
	Report bool
	Test   string
	// Msg is the message of a failed assertion. When Values is set, it is
	// a format whose verbs %s receive the values, a literal % being %%.
	Msg string
	// Values are the expressions of the value-of and name elements of the
	// message, in order, evaluated on the element checked.
	Values []string
}

// Rule is a rule of a pattern: its context, an XSLT pattern, its variables
// and its assertions.
type Rule struct {
	Context string
	Lets    []Let
	Asserts []Assertion
}

// Pattern is a pattern of the schematron, abstract patterns being
// instantiated.
type Pattern struct {
	ID    string
	Lets  []Let
	Rules []Rule
}

type (
	cLet struct {
		name string
		e    expr
	}
	cAssert struct {
		a      *Assertion
		test   expr
		values []expr
	}
	cRule struct {
		context expr
		src     string
		lets    []cLet
		asserts []cAssert
	}
	cPattern struct {
		id    string
		lets  []cLet
		rules []cRule
	}
)

// Schema is a compiled schematron.
type Schema struct {
	pats []cPattern
}

// Compile compiles patterns whose prefixes are those of ns. It returns the
// first expression that does not compile, outside the subset of XPath 2.0
// that the package implements.
func Compile(ns map[string]string, patterns []Pattern) (*Schema, error) {
	s := &Schema{pats: make([]cPattern, len(patterns))}
	for i := range patterns {
		p := &patterns[i]
		cp := cPattern{id: p.ID}
		var err error
		if cp.lets, err = compileLets(p.Lets, ns); err != nil {
			return nil, fmt.Errorf("pattern %s: %w", p.ID, err)
		}
		for j := range p.Rules {
			r := &p.Rules[j]
			ctx, err := compileContext(r.Context, ns)
			if err != nil {
				return nil, fmt.Errorf("pattern %s, context %s: %w", p.ID, r.Context, err)
			}
			cr := cRule{context: ctx, src: r.Context}
			if cr.lets, err = compileLets(r.Lets, ns); err != nil {
				return nil, fmt.Errorf("pattern %s, context %s: %w", p.ID, r.Context, err)
			}
			for k := range r.Asserts {
				a := &r.Asserts[k]
				t, err := compile(a.Test, ns)
				if err != nil {
					return nil, fmt.Errorf("%s: %s: %w", a.ID, a.Test, err)
				}
				ca := cAssert{a: a, test: t}
				for _, v := range a.Values {
					e, err := compile(v, ns)
					if err != nil {
						return nil, fmt.Errorf("%s: message value %s: %w", a.ID, v, err)
					}
					ca.values = append(ca.values, e)
				}
				if n := strings.Count(strings.ReplaceAll(a.Msg, "%%", ""), "%s"); len(a.Values) > 0 && n != len(a.Values) {
					return nil, fmt.Errorf("%s: %d values for %d verbs in the message", a.ID, len(a.Values), n)
				}
				cr.asserts = append(cr.asserts, ca)
			}
			cp.rules = append(cp.rules, cr)
		}
		s.pats[i] = cp
	}
	return s, nil
}

func compileLets(lets []Let, ns map[string]string) ([]cLet, error) {
	var out []cLet
	for _, l := range lets {
		e, err := compile(l.Value, ns)
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", l.Name, err)
		}
		out = append(out, cLet{l.Name, e})
	}
	return out, nil
}

// compileContext compiles the context of a rule, an XSLT pattern, into the
// expression that selects the nodes it matches from the document node:
// each relative path of the union is searched at any depth, as
// cac:InvoiceLine matches every cac:InvoiceLine element, and its steps are
// lenient: a node whose predicate raises an error does not match.
func compileContext(src string, ns map[string]string) (expr, error) {
	e, err := compile(src, ns)
	if err != nil {
		return nil, err
	}
	var anchor func(e expr) (expr, error)
	anchor = func(e expr) (expr, error) {
		switch x := e.(type) {
		case *unionExpr:
			a, err := anchor(x.a)
			if err != nil {
				return nil, err
			}
			b, err := anchor(x.b)
			if err != nil {
				return nil, err
			}
			return &unionExpr{a, b}, nil
		case *pathExpr:
			if !x.absolute && x.steps[0].prim != nil {
				return nil, fmt.Errorf("pattern step is not a node test")
			}
			y := &pathExpr{absolute: true, steps: append([]step(nil), x.steps...)}
			if !x.absolute {
				y.steps[0].desc = true
			}
			for i := range y.steps {
				y.steps[i].lenient = true
			}
			return y, nil
		}
		return nil, fmt.Errorf("unsupported pattern")
	}
	return anchor(e)
}

// Run applies the schema to the document whose root element is root and
// reports with c the failed assertions whose identifier keep accepts.
//
// In each pattern, an element is checked by the first rule whose context it
// matches and by no other, whatever the assertions of that rule: a rule
// with no assertion that keep accepts still hides the elements it matches
// from the next rules. The variables of a pattern are evaluated once, from
// the document node; those of a rule for each element it checks, in order.
// A test whose evaluation raises an XPath error (a cast of a value that is
// not a number, a function given several items where it allows one) counts
// as a failed assert, or a fired report: a validator built on an XSLT
// processor stops on such an error.
func (s *Schema) Run(c *rules.Checker, root *xmltree.Node, keep func(id string) bool) {
	d := newDoc(root)
	top := &ctx{d: d, it: item{k: kNode}, pos: 1, size: 1}
	for pi := range s.pats {
		p := &s.pats[pi]
		kept := make([]bool, len(p.rules))
		active := false
		for ri := range p.rules {
			for _, a := range p.rules[ri].asserts {
				if keep(a.a.ID) {
					kept[ri], active = true, true
					break
				}
			}
		}
		if !active {
			continue
		}
		env := bindLets(top, p.lets)
		// claim[i] is the rule, plus one, that checks d.all[i].
		claim := make([]int32, len(d.all))
		for ri := range p.rules {
			nodes, err := p.rules[ri].context.eval(top)
			if err != nil {
				panic(fmt.Sprintf("schematron: pattern %s: context %s: %v", p.id, p.rules[ri].src, err))
			}
			for _, n := range nodes {
				if n.k != kNode || n.n == nil || n.a != 0 {
					continue
				}
				if i := d.order[n.n]; claim[i] == 0 {
					claim[i] = int32(ri) + 1
				}
			}
		}
		for i, ri := range claim {
			if ri == 0 || !kept[ri-1] {
				continue
			}
			r := &p.rules[ri-1]
			n := d.all[i]
			rc := &ctx{d: d, it: item{k: kNode, n: n}, pos: 1, size: 1, vars: env}
			rc.vars = bindLets(rc, r.lets)
			for _, a := range r.asserts {
				if !keep(a.a.ID) {
					continue
				}
				ok, err := ebvOf(a.test, rc)
				if err == nil && ok != a.a.Report {
					continue
				}
				if len(a.values) == 0 {
					c.Fail(a.a.ID, a.a.Flag, n, "%s", a.a.Msg)
					continue
				}
				args := make([]any, len(a.values))
				for k, v := range a.values {
					args[k] = valueOf(v, rc)
				}
				c.Fail(a.a.ID, a.a.Flag, n, a.a.Msg, args...)
			}
		}
	}
}

// valueOf returns the string value-of writes for e: its atomized items,
// separated by a space, or nothing when its evaluation fails.
func valueOf(e expr, c *ctx) string {
	v, err := e.eval(c)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(v))
	for _, it := range atomize(v) {
		parts = append(parts, itemString(it))
	}
	return strings.Join(parts, " ")
}

// bindLets evaluates variables in order, each seeing the previous ones; a
// variable whose evaluation fails raises the error where it is used.
func bindLets(c *ctx, lets []cLet) *binding {
	env := c.vars
	for _, l := range lets {
		cc := *c
		cc.vars = env
		v, err := l.e.eval(&cc)
		env = &binding{name: l.name, val: v, err: err, next: env}
	}
	return env
}

// NormalizeSpace implements normalize-space(): XML white space collapsed to
// single spaces and trimmed.
func NormalizeSpace(s string) string { return normalizeSpace(s) }

// StringValue returns the string value of an element: its text and that of
// its descendants, in document order.
func StringValue(n *xmltree.Node) string { return stringValue(item{k: kNode, n: n}) }
