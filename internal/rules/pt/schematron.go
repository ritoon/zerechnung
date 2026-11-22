package pt

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The tables of rules_gen.go hold the schematron as eSPap publishes it; this
// file runs it the way an ISO schematron processor does.

// let is a variable of a pattern or of a rule.
type let struct{ Name, Value string }

// assertion is an assert of the schematron, or a report when Report is set:
// an assert fails when its test is false, a report when it is true.
type assertion struct {
	ID     string
	Flag   rules.Severity
	Report bool
	Test   string
	Msg    string
}

type rule struct {
	Context string
	Lets    []let
	Asserts []assertion
}

type pattern struct {
	ID    string
	Lets  []let
	Rules []rule
}

type (
	cLet struct {
		name string
		e    expr
	}
	cAssert struct {
		a    *assertion
		test expr
	}
	cRule struct {
		context expr
		lets    []cLet
		asserts []cAssert
		cen     bool // holds an assertion of CENCatalog
		pt      bool // holds an assertion of the CIUS-PT
	}
	cPattern struct {
		id    string
		lets  []cLet
		rules []cRule
		cen   bool
		pt    bool
	}
)

// compiled returns the compiled patterns, or the first expression that does
// not compile.
var compiled = sync.OnceValues(func() ([]cPattern, error) {
	out := make([]cPattern, len(patterns))
	for i := range patterns {
		p := &patterns[i]
		cp := cPattern{id: p.ID}
		var err error
		if cp.lets, err = compileLets(p.Lets); err != nil {
			return nil, fmt.Errorf("pattern %s: %w", p.ID, err)
		}
		for j := range p.Rules {
			r := &p.Rules[j]
			ctx, err := compileContext(r.Context)
			if err != nil {
				return nil, fmt.Errorf("pattern %s, context %s: %w", p.ID, r.Context, err)
			}
			cr := cRule{context: ctx}
			if cr.lets, err = compileLets(r.Lets); err != nil {
				return nil, fmt.Errorf("pattern %s, context %s: %w", p.ID, r.Context, err)
			}
			for k := range r.Asserts {
				a := &r.Asserts[k]
				t, err := compile(a.Test, namespaces)
				if err != nil {
					return nil, fmt.Errorf("%s: %s: %w", a.ID, a.Test, err)
				}
				cr.asserts = append(cr.asserts, cAssert{a, t})
				if national(a.ID) {
					cr.pt = true
				} else {
					cr.cen = true
				}
			}
			cp.cen = cp.cen || cr.cen
			cp.pt = cp.pt || cr.pt
			cp.rules = append(cp.rules, cr)
		}
		out[i] = cp
	}
	return out, nil
})

func compileLets(lets []let) ([]cLet, error) {
	var out []cLet
	for _, l := range lets {
		e, err := compile(l.Value, namespaces)
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
func compileContext(src string) (expr, error) {
	e, err := compile(src, namespaces)
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

// national reports whether an assertion is a rule of the CIUS-PT itself
// (BR-CIUS-PT, DT-CIUS-PT), checked in the Portugal rule set.
func national(id string) bool { return strings.Contains(id, "CIUS-PT") }

// run applies the schematron to the UBL document of c and reports the
// failed assertions of the CIUS-PT (cen false) or the others (cen true).
//
// In each pattern, an element is checked by the first rule whose context it
// matches and by no other, whatever the assertions of that rule: a rule
// with no assertion of the selected group still hides the elements it
// matches from the next rules. The variables of a pattern are evaluated
// once, from the document node; those of a rule for each element it checks,
// in order. A test whose evaluation raises an XPath error (a cast of a
// value that is not a number, a function given several items where it
// allows one) counts as a failed assert, or a fired report: a validator
// built on an XSLT processor stops on such an error.
func run(c *rules.Checker, root *xmltree.Node, cen bool) {
	pats, err := compiled()
	if err != nil {
		panic("pt: " + err.Error())
	}
	d := newDoc(root)
	top := &ctx{d: d, it: item{k: kNode}, pos: 1, size: 1}
	for pi := range pats {
		p := &pats[pi]
		if cen && !p.cen || !cen && !p.pt {
			continue
		}
		env := bindLets(top, p.lets)
		// claim[i] is the rule, plus one, that checks d.all[i].
		claim := make([]int32, len(d.all))
		for ri := range p.rules {
			nodes, err := p.rules[ri].context.eval(top)
			if err != nil {
				panic(fmt.Sprintf("pt: pattern %s: context %s: %v", p.id, patterns[pi].Rules[ri].Context, err))
			}
			for _, n := range nodes {
				if n.k != kNode || n.n == nil || n.a > 0 {
					continue
				}
				if i := d.order[n.n]; claim[i] == 0 {
					claim[i] = int32(ri) + 1
				}
			}
		}
		for i, ri := range claim {
			if ri == 0 {
				continue
			}
			r := &p.rules[ri-1]
			if cen && !r.cen || !cen && !r.pt {
				continue
			}
			n := d.all[i]
			rc := &ctx{d: d, it: item{k: kNode, n: n}, pos: 1, size: 1, vars: env}
			rc.vars = bindLets(rc, r.lets)
			for _, a := range r.asserts {
				if national(a.a.ID) == cen {
					continue
				}
				ok, err := ebvOf(a.test, rc)
				if err != nil || ok == a.a.Report {
					c.Fail(a.a.ID, a.a.Flag, n, "%s", a.a.Msg)
				}
			}
		}
	}
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
