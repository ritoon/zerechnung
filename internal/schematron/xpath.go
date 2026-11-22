package schematron

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file parses the subset of XPath 2.0 that the schematrons of the
// CIUS-PT and of the CIUS-RO use in their rule contexts, variables and tests:
// paths with the child, attribute, self, parent, ancestor, descendant and
// following-sibling axes and predicates, the node tests node() and text(),
// unions, castable as xs:date, the general comparisons, arithmetic (+, -, *,
// div), and, or, quantified expressions (some, every), variables and the
// functions of funcs. An expression outside this subset is refused when it
// is compiled: the tests of the rule sets compile every expression of their
// generated tables.

type tokKind uint8

const (
	tEOF    tokKind = iota
	tName           // QName, NCName or prefix:*
	tString         // string literal, unquoted
	tNumber         // numeric literal
	tVar            // $name, without the $
	tAxis           // axis name followed by ::
	tSym            // operator or punctuation
)

type token struct {
	kind tokKind
	val  string
}

func isNameStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }

func isNameChar(r rune) bool {
	return isNameStart(r) || r == '-' || r == '.' || unicode.IsDigit(r)
}

// lex splits an expression into tokens.
func lex(s string) ([]token, error) {
	var toks []token
	i := 0
	ncname := func() string {
		j := i
		for j < len(s) {
			r, n := utf8.DecodeRuneInString(s[j:])
			if !isNameChar(r) {
				break
			}
			j += n
		}
		name := s[i:j]
		i = j
		return name
	}
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"':
			var b strings.Builder
			j := i + 1
			for {
				if j >= len(s) {
					return nil, fmt.Errorf("unterminated string literal")
				}
				if s[j] == c {
					if j+1 < len(s) && s[j+1] == c {
						b.WriteByte(c)
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			toks = append(toks, token{tString, b.String()})
			i = j + 1
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
				j++
				if j < len(s) && (s[j] == '+' || s[j] == '-') {
					j++
				}
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					j++
				}
			}
			toks = append(toks, token{tNumber, s[i:j]})
			i = j
		case c == '$':
			i++
			name := ncname()
			if i < len(s) && s[i] == ':' && i+1 < len(s) && s[i+1] != ':' {
				i++
				name += ":" + ncname()
			}
			if name == "" {
				return nil, fmt.Errorf("$ without a variable name")
			}
			toks = append(toks, token{tVar, name})
		default:
			r, _ := utf8.DecodeRuneInString(s[i:])
			if isNameStart(r) {
				name := ncname()
				switch {
				case strings.HasPrefix(s[i:], "::"):
					i += 2
					toks = append(toks, token{tAxis, name})
				case strings.HasPrefix(s[i:], ":*"):
					i += 2
					toks = append(toks, token{tName, name + ":*"})
				case i+1 < len(s) && s[i] == ':':
					if r, _ := utf8.DecodeRuneInString(s[i+1:]); isNameStart(r) {
						i++
						name += ":" + ncname()
					}
					toks = append(toks, token{tName, name})
				default:
					toks = append(toks, token{tName, name})
				}
				continue
			}
			for _, op := range []string{"//", "!=", "<=", ">=", "..", "::"} {
				if strings.HasPrefix(s[i:], op) {
					toks = append(toks, token{tSym, op})
					i += len(op)
					goto next
				}
			}
			if strings.IndexByte("()[],/@|=<>+-*.", c) < 0 {
				return nil, fmt.Errorf("unexpected character %q", c)
			}
			toks = append(toks, token{tSym, string(c)})
			i++
		next:
		}
	}
	return append(toks, token{kind: tEOF}), nil
}

type parser struct {
	toks []token
	pos  int
	ns   map[string]string
}

// compile parses an XPath expression whose prefixes are those of ns.
func compile(src string, ns map[string]string) (e expr, err error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, ns: ns}
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(parseError)
			if !ok {
				panic(r)
			}
			e, err = nil, pe
		}
	}()
	e = p.expr()
	if t := p.peek(); t.kind != tEOF {
		p.fail("unexpected %q", t.val)
	}
	return e, nil
}

type parseError string

func (e parseError) Error() string { return string(e) }

func (p *parser) fail(format string, args ...any) {
	panic(parseError(fmt.Sprintf(format, args...)))
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) peekAt(i int) token {
	if p.pos+i < len(p.toks) {
		return p.toks[p.pos+i]
	}
	return token{kind: tEOF}
}
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) isSym(v string) bool { t := p.peek(); return t.kind == tSym && t.val == v }

// isOp reports whether the next token is the operator keyword v.
func (p *parser) isOp(v string) bool { t := p.peek(); return t.kind == tName && t.val == v }

func (p *parser) expect(v string) {
	if !p.isSym(v) {
		p.fail("expected %q, got %q", v, p.peek().val)
	}
	p.pos++
}

// expr parses Expr: ExprSingle ("," ExprSingle)*.
func (p *parser) expr() expr {
	e := p.single()
	if !p.isSym(",") {
		return e
	}
	s := seqExpr{e}
	for p.isSym(",") {
		p.pos++
		s = append(s, p.single())
	}
	return s
}

// single parses ExprSingle: a quantified expression or OrExpr.
func (p *parser) single() expr {
	if t := p.peek(); t.kind == tName && (t.val == "some" || t.val == "every") && p.peekAt(1).kind == tVar {
		p.pos++
		q := &quantExpr{every: t.val == "every"}
		for {
			v := p.next()
			if v.kind != tVar {
				p.fail("expected a variable in %s", t.val)
			}
			if !p.isOp("in") {
				p.fail("expected in")
			}
			p.pos++
			q.names = append(q.names, v.val)
			q.ins = append(q.ins, p.single())
			if !p.isSym(",") {
				break
			}
			p.pos++
		}
		if !p.isOp("satisfies") {
			p.fail("expected satisfies")
		}
		p.pos++
		q.sat = p.single()
		return q
	}
	return p.or()
}

func (p *parser) or() expr {
	e := p.and()
	for p.isOp("or") {
		p.pos++
		e = &orExpr{e, p.and()}
	}
	return e
}

func (p *parser) and() expr {
	e := p.comparison()
	for p.isOp("and") {
		p.pos++
		e = &andExpr{e, p.comparison()}
	}
	return e
}

func (p *parser) comparison() expr {
	e := p.additive()
	if t := p.peek(); t.kind == tSym {
		switch t.val {
		case "=", "!=", "<", "<=", ">", ">=":
			p.pos++
			return &cmpExpr{op: t.val, a: e, b: p.additive()}
		}
	}
	return e
}

func (p *parser) additive() expr {
	e := p.multiplicative()
	for p.isSym("+") || p.isSym("-") {
		op := p.next().val
		e = &arithExpr{op: op, a: e, b: p.multiplicative()}
	}
	return e
}

func (p *parser) multiplicative() expr {
	e := p.union()
	for p.isSym("*") || p.isOp("div") {
		op := p.next().val
		e = &arithExpr{op: op, a: e, b: p.union()}
	}
	return e
}

func (p *parser) union() expr {
	e := p.castable()
	for p.isSym("|") || p.isOp("union") {
		p.pos++
		e = &unionExpr{e, p.castable()}
	}
	return e
}

// castable parses CastableExpr: UnaryExpr ("castable" "as" SingleType)?,
// for the single type xs:date.
func (p *parser) castable() expr {
	e := p.unary()
	if t := p.peekAt(1); !p.isOp("castable") || t.kind != tName || t.val != "as" {
		return e
	}
	p.pos += 2
	if t := p.next(); t.kind != tName || t.val != "xs:date" {
		p.fail("castable as %s is not supported", t.val)
	}
	return &castableExpr{e}
}

func (p *parser) unary() expr {
	if p.isSym("-") {
		p.pos++
		return &negExpr{p.unary()}
	}
	if p.isSym("+") {
		p.pos++
		return p.unary()
	}
	return p.path()
}

// startsStep reports whether the next token can start a step.
func (p *parser) startsStep() bool {
	t := p.peek()
	switch t.kind {
	case tName, tString, tNumber, tVar, tAxis:
		return true
	case tSym:
		switch t.val {
		case "*", "@", ".", "..", "(":
			return true
		}
	}
	return false
}

func (p *parser) path() expr {
	pe := &pathExpr{}
	switch {
	case p.isSym("/"):
		p.pos++
		pe.absolute = true
		if !p.startsStep() {
			return pe
		}
	case p.isSym("//"):
		p.pos++
		pe.absolute = true
		s := p.step()
		s.desc = true
		pe.steps = append(pe.steps, s)
	}
	if len(pe.steps) == 0 {
		pe.steps = append(pe.steps, p.step())
	}
	for p.isSym("/") || p.isSym("//") {
		desc := p.next().val == "//"
		s := p.step()
		s.desc = desc
		pe.steps = append(pe.steps, s)
	}
	if !pe.absolute && len(pe.steps) == 1 && pe.steps[0].prim != nil && len(pe.steps[0].preds) == 0 {
		return pe.steps[0].prim
	}
	return pe
}

var axes = map[string]axis{
	"child": axChild, "attribute": axAttribute, "self": axSelf, "parent": axParent,
	"ancestor": axAncestor, "ancestor-or-self": axAncestorOrSelf,
	"descendant": axDescendant, "descendant-or-self": axDescendantOrSelf,
	"following-sibling": axFollowingSibling,
}

func (p *parser) step() step {
	var s step
	t := p.peek()
	switch {
	case t.kind == tSym && t.val == "..":
		p.pos++
		s.axis, s.test = axParent, nodeTest{kind: testNode}
		return s
	case t.kind == tSym && t.val == "@":
		p.pos++
		s.axis = axAttribute
		s.test = p.nodeTest(true)
	case t.kind == tAxis:
		p.pos++
		a, ok := axes[t.val]
		if !ok {
			p.fail("unsupported axis %s", t.val)
		}
		s.axis = a
		s.test = p.nodeTest(a == axAttribute)
	case t.kind == tName && p.peekAt(1).kind == tSym && p.peekAt(1).val == "(" && t.val != "node" && t.val != "text":
		s.prim = p.primary()
	case t.kind == tName || t.kind == tSym && t.val == "*":
		s.axis = axChild
		s.test = p.nodeTest(false)
	default:
		s.prim = p.primary()
	}
	if s.prim == nil && s.test.kind == testText && s.axis != axChild && s.axis != axSelf {
		p.fail("text() is only supported on the child and self axes")
	}
	for p.isSym("[") {
		p.pos++
		s.preds = append(s.preds, p.expr())
		p.expect("]")
	}
	return s
}

func (p *parser) nodeTest(attr bool) nodeTest {
	t := p.next()
	switch {
	case t.kind == tSym && t.val == "*":
		return nodeTest{kind: testAny}
	case t.kind != tName:
		p.fail("expected a node test, got %q", t.val)
	case (t.val == "node" || t.val == "text") && p.isSym("("):
		p.pos++
		p.expect(")")
		if t.val == "text" {
			return nodeTest{kind: testText}
		}
		return nodeTest{kind: testNode}
	}
	prefix, local, ok := strings.Cut(t.val, ":")
	if !ok {
		return nodeTest{kind: testName, local: prefix}
	}
	if attr {
		p.fail("prefixed attribute %s is not supported", t.val)
	}
	uri, known := p.ns[prefix]
	if !known {
		p.fail("unknown prefix %s", prefix)
	}
	if local == "*" {
		return nodeTest{kind: testSpace, space: uri}
	}
	return nodeTest{kind: testName, space: uri, local: local}
}

func (p *parser) primary() expr {
	t := p.next()
	switch t.kind {
	case tString:
		return &literal{item{k: kString, s: t.val}}
	case tNumber:
		return &literal{parseNumber(p, t.val)}
	case tVar:
		return &varRef{t.val}
	case tSym:
		switch t.val {
		case ".":
			return ctxItem{}
		case "(":
			if p.isSym(")") {
				p.pos++
				return seqExpr{}
			}
			e := p.expr()
			p.expect(")")
			return e
		}
	case tName:
		if !p.isSym("(") {
			break
		}
		p.pos++
		c := &call{name: t.val}
		if !p.isSym(")") {
			c.args = append(c.args, p.single())
			for p.isSym(",") {
				p.pos++
				c.args = append(c.args, p.single())
			}
		}
		p.expect(")")
		f, ok := funcs[c.name]
		if !ok {
			p.fail("unsupported function %s", c.name)
		}
		if len(c.args) < f.min || f.max >= 0 && len(c.args) > f.max {
			p.fail("%s: %d arguments", c.name, len(c.args))
		}
		c.fn = f.fn
		if c.name == "matches" {
			pat, ok1 := c.args[1].(*literal)
			flags, ok2 := &literal{item{k: kString}}, true
			if len(c.args) == 3 {
				flags, ok2 = c.args[2].(*literal)
			}
			if ok1 && ok2 {
				m, err := compileRegex(pat.v.s, flags.v.s)
				if err != nil {
					p.fail("%s", err)
				}
				c.re = m
			}
		}
		return c
	}
	p.fail("unexpected %q", t.val)
	return nil
}

// parseNumber parses an integer, decimal or double literal.
func parseNumber(p *parser, s string) item {
	if strings.ContainsAny(s, "eE") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			p.fail("bad number %s", s)
		}
		return item{k: kDouble, f: f}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || strings.Count(s, ".") > 1 {
		p.fail("bad number %s", s)
	}
	if strings.Contains(s, ".") {
		return item{k: kDecimal, r: r}
	}
	return item{k: kInteger, r: r}
}
