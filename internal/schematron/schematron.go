// Package schematron runs ISO Schematron patterns whose contexts, variables
// and tests use a subset of XPath 2.0, the way an XSLT 2.0 processor runs
// the stylesheet a schematron compiles into. The rule sets whose schematron
// is reproduced as published (packages internal/rules/pt, internal/rules/ro
// and internal/rules/hr for national CIUS, internal/rules/cdv for the French
// statuses of the life cycle of an invoice, internal/rules/flux10 for the
// French e-reporting) generate their patterns and run them with this
// package.
//
// # Evaluation
//
// The expressions are evaluated by an interpreter of the subset of XPath 2.0
// they use (xpath.go, eval.go, funcs.go), with the semantics of an XSLT 2.0
// processor: general comparisons of untyped values as strings or as
// xs:double, value comparisons (eq, ne, lt…) of untyped values as strings,
// exact xs:decimal arithmetic (a division keeps 18 decimals, rounded half
// down, as Saxon does), round() halves towards positive infinity, regular
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

// Function is a function that the schematron declares (xsl:function, as an
// XSLT 2.0 query binding allows): its name, a prefixed QName, the types of
// its result and parameters (the as attributes), its variables
// (xsl:variable) and the expression of its result (xsl:sequence). Its body
// has no context item and sees its parameters and variables only.
type Function struct {
	Name   string
	As     string
	Params []Param
	Lets   []Let
	Result string
}

// Param is a parameter of a function (xsl:param).
type Param struct{ Name, As string }

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

// Compile compiles patterns whose prefixes are those of ns, and whose
// expressions may call the functions fns. It returns the first expression
// that does not compile, outside the subset of XPath 2.0 that the package
// implements.
func Compile(ns map[string]string, patterns []Pattern, fns ...Function) (*Schema, error) {
	user, err := compileFunctions(ns, fns)
	if err != nil {
		return nil, err
	}
	compile := func(src string, ns map[string]string) (expr, error) { return compileWith(src, ns, user) }
	compileLets := func(lets []Let, ns map[string]string) ([]cLet, error) { return compileLetsWith(lets, ns, user) }
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
			ctx, err := compileContext(r.Context, ns, user)
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

// compileFunctions compiles the functions, each of which may call itself
// and the others.
func compileFunctions(ns map[string]string, fns []Function) (map[string]*userFunc, error) {
	if len(fns) == 0 {
		return nil, nil
	}
	user := map[string]*userFunc{}
	for _, f := range fns {
		if _, dup := user[f.Name]; dup {
			return nil, fmt.Errorf("function %s declared twice", f.Name)
		}
		as, err := parseSeqType(f.As)
		if err != nil {
			return nil, fmt.Errorf("function %s: %w", f.Name, err)
		}
		u := &userFunc{name: f.Name, as: as}
		for _, p := range f.Params {
			t, err := parseSeqType(p.As)
			if err != nil {
				return nil, fmt.Errorf("function %s, parameter $%s: %w", f.Name, p.Name, err)
			}
			u.params = append(u.params, userParam{p.Name, t})
		}
		user[f.Name] = u
	}
	for _, f := range fns {
		u := user[f.Name]
		var err error
		if u.lets, err = compileLetsWith(f.Lets, ns, user); err != nil {
			return nil, fmt.Errorf("function %s: %w", f.Name, err)
		}
		if u.result, err = compileWith(f.Result, ns, user); err != nil {
			return nil, fmt.Errorf("function %s: %s: %w", f.Name, f.Result, err)
		}
	}
	return user, nil
}

func compileLetsWith(lets []Let, ns map[string]string, user map[string]*userFunc) ([]cLet, error) {
	var out []cLet
	for _, l := range lets {
		e, err := compileWith(l.Value, ns, user)
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
func compileContext(src string, ns map[string]string, user map[string]*userFunc) (expr, error) {
	e, err := compileWith(src, ns, user)
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
