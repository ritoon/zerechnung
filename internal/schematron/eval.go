package schematron

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// kind is the type of an item of an XPath sequence.
type kind uint8

const (
	kNode    kind = iota // element, attribute or document node
	kString              // xs:string
	kUntyped             // xs:untypedAtomic: the atomized value of a node
	kInteger             // xs:integer
	kDecimal             // xs:decimal
	kDouble              // xs:double
	kBool                // xs:boolean
)

// item is an item of an XPath sequence. A node is an element n, the
// attribute a-1 of n when a > 0, the text node of n when a is textNode, or
// the document node when n is nil.
type item struct {
	k kind
	n *xmltree.Node
	a int
	s string   // kString, kUntyped
	r *big.Rat // kInteger, kDecimal
	f float64  // kDouble
	b bool     // kBool
}

type seq []item

// textNode is the field a of the item of a text node: the character data
// directly inside element n (n.Text), a single text node when it is not
// empty. The tree does not keep the text nodes of an element apart, nor
// their place among its child elements: text() selects that one node, and
// node() the child elements only.
const textNode = -1

func (i item) numeric() bool { return i.k == kInteger || i.k == kDecimal || i.k == kDouble }

func (i item) sameNode(j item) bool { return i.n == j.n && i.a == j.a }

// errDynamic is a dynamic or type error of XPath. A validator built on an
// XSLT processor stops on such an error; Check reports the assertion whose
// test raised it as failed.
type errDynamic string

func (e errDynamic) Error() string { return string(e) }

func dynErr(format string, args ...any) error { return errDynamic(fmt.Sprintf(format, args...)) }

// doc indexes the elements of a document: their document order and, by
// name, the elements of the descendant searches (//cbc:ID).
type doc struct {
	root   *xmltree.Node
	order  map[*xmltree.Node]int
	all    []*xmltree.Node
	byName map[qname][]*xmltree.Node
}

type qname struct{ space, local string }

func newDoc(root *xmltree.Node) *doc {
	d := &doc{root: root, order: map[*xmltree.Node]int{}, byName: map[qname][]*xmltree.Node{}}
	root.Walk(func(n *xmltree.Node) bool {
		d.order[n] = len(d.all)
		d.all = append(d.all, n)
		k := qname{n.Space, n.Name}
		d.byName[k] = append(d.byName[k], n)
		return true
	})
	return d
}

// before reports whether node item x comes before y in document order.
func (d *doc) before(x, y item) bool {
	ox, oy := -1, -1
	if x.n != nil {
		ox = d.order[x.n]
	}
	if y.n != nil {
		oy = d.order[y.n]
	}
	if ox != oy {
		return ox < oy
	}
	// The text node of an element comes after its attributes.
	ax, ay := x.a, y.a
	if ax == textNode {
		ax = math.MaxInt
	}
	if ay == textNode {
		ay = math.MaxInt
	}
	return ax < ay
}

// sortNodes sorts node items in document order and removes duplicates.
func (d *doc) sortNodes(s seq) seq {
	if len(s) < 2 {
		return s
	}
	slices.SortStableFunc(s, func(x, y item) int {
		switch {
		case x.sameNode(y):
			return 0
		case d.before(x, y):
			return -1
		}
		return 1
	})
	return slices.CompactFunc(s, item.sameNode)
}

// ctx is the dynamic context of an evaluation.
type ctx struct {
	d         *doc
	it        item
	pos, size int
	vars      *binding
}

type binding struct {
	name string
	val  seq
	err  error
	next *binding
}

func (c *ctx) with(it item, pos, size int) *ctx {
	n := *c
	n.it, n.pos, n.size = it, pos, size
	return &n
}

type expr interface {
	eval(c *ctx) (seq, error)
}

type (
	orExpr    struct{ a, b expr }
	andExpr   struct{ a, b expr }
	unionExpr struct{ a, b expr }
	negExpr   struct{ e expr }
	seqExpr   []expr
	literal   struct{ v item }
	varRef    struct{ name string }
	ctxItem   struct{}
	cmpExpr   struct {
		op   string
		a, b expr
	}
	arithExpr struct {
		op   string
		a, b expr
	}
	quantExpr struct {
		every bool
		names []string
		ins   []expr
		sat   expr
	}
	call struct {
		name string
		args []expr
		fn   func(c *ctx, f *call, args []seq) (seq, error)
		re   matcher // precompiled pattern of matches()
	}
	pathExpr struct {
		absolute bool
		steps    []step
	}
	// castableExpr is e castable as xs:date.
	castableExpr struct{ e expr }
)

type axis uint8

const (
	axChild axis = iota
	axAttribute
	axSelf
	axParent
	axAncestor
	axAncestorOrSelf
	axDescendant
	axDescendantOrSelf
	axFollowingSibling
)

type testKind uint8

const (
	testName  testKind = iota // a QName, or an unprefixed name
	testAny                   // *
	testSpace                 // prefix:*
	testNode                  // node()
	testText                  // text()
)

type nodeTest struct {
	kind         testKind
	space, local string
}

// step is a step of a path: an axis step, or a primary expression (prim)
// evaluated for each node of the previous step. desc is a // before it.
// lenient is set on the steps of the context of a rule, an XSLT pattern:
// a node whose predicate raises an error does not match it, as in XSLT 3.0
// (Saxon warns and goes on to the next rule), where a predicate error stops
// the evaluation of an expression.
type step struct {
	desc    bool
	lenient bool
	axis    axis
	test    nodeTest
	prim    expr
	preds   []expr
}

func boolSeq(b bool) seq { return seq{{k: kBool, b: b}} }

// An or expression is true when one of its operands is true, an and
// expression false when one of its operands is false, whatever an error in
// the other operand: XPath 2.0 lets a processor evaluate the operands in any
// order, and Saxon does not raise the error of an operand whose value does
// not decide the result (normalize-space() of two elements and false() is
// false). The error is raised when the other operand does not decide it.

func (e *orExpr) eval(c *ctx) (seq, error) {
	a, errA := ebvOf(e.a, c)
	if errA == nil && a {
		return boolSeq(true), nil
	}
	b, errB := ebvOf(e.b, c)
	switch {
	case errB == nil && b:
		return boolSeq(true), nil
	case errA != nil:
		return boolSeq(false), errA
	}
	return boolSeq(false), errB
}

func (e *andExpr) eval(c *ctx) (seq, error) {
	a, errA := ebvOf(e.a, c)
	if errA == nil && !a {
		return boolSeq(false), nil
	}
	b, errB := ebvOf(e.b, c)
	switch {
	case errB == nil && !b:
		return boolSeq(false), nil
	case errA != nil:
		return boolSeq(false), errA
	}
	return boolSeq(errB == nil), errB
}

func ebvOf(e expr, c *ctx) (bool, error) {
	s, err := e.eval(c)
	if err != nil {
		return false, err
	}
	return ebv(s)
}

// ebv returns the effective boolean value of s.
func ebv(s seq) (bool, error) {
	if len(s) == 0 {
		return false, nil
	}
	if s[0].k == kNode {
		return true, nil
	}
	if len(s) > 1 {
		return false, dynErr("effective boolean value of a sequence of %d atomic values", len(s))
	}
	switch i := s[0]; i.k {
	case kBool:
		return i.b, nil
	case kString, kUntyped:
		return i.s != "", nil
	case kInteger, kDecimal:
		return i.r.Sign() != 0, nil
	default:
		return i.f != 0 && !math.IsNaN(i.f), nil
	}
}

func (e *unionExpr) eval(c *ctx) (seq, error) {
	a, err := e.a.eval(c)
	if err != nil {
		return nil, err
	}
	b, err := e.b.eval(c)
	if err != nil {
		return nil, err
	}
	out := append(append(seq{}, a...), b...)
	for _, i := range out {
		if i.k != kNode {
			return nil, dynErr("union of atomic values")
		}
	}
	return c.d.sortNodes(out), nil
}

func (e *negExpr) eval(c *ctx) (seq, error) {
	s, err := e.e.eval(c)
	if err != nil || len(s) == 0 {
		return nil, err
	}
	return arith("-", seq{{k: kInteger, r: new(big.Rat)}}, s)
}

func (e seqExpr) eval(c *ctx) (seq, error) {
	var out seq
	for _, x := range e {
		s, err := x.eval(c)
		if err != nil {
			return nil, err
		}
		out = append(out, s...)
	}
	return out, nil
}

func (e *literal) eval(*ctx) (seq, error) { return seq{e.v}, nil }

func (e *varRef) eval(c *ctx) (seq, error) {
	for b := c.vars; b != nil; b = b.next {
		if b.name == e.name {
			return b.val, b.err
		}
	}
	return nil, dynErr("unbound variable $%s", e.name)
}

func (ctxItem) eval(c *ctx) (seq, error) { return seq{c.it}, nil }

func (e *quantExpr) eval(c *ctx) (seq, error) {
	var walk func(i int, vars *binding) (bool, error)
	walk = func(i int, vars *binding) (bool, error) {
		if i == len(e.names) {
			cc := *c
			cc.vars = vars
			return ebvOf(e.sat, &cc)
		}
		cc := *c
		cc.vars = vars
		in, err := e.ins[i].eval(&cc)
		if err != nil {
			return false, err
		}
		for _, it := range in {
			ok, err := walk(i+1, &binding{name: e.names[i], val: seq{it}, next: vars})
			if err != nil {
				return false, err
			}
			if ok != e.every {
				return ok, nil
			}
		}
		return e.every, nil
	}
	ok, err := walk(0, c.vars)
	return boolSeq(ok), err
}

func (e *call) eval(c *ctx) (seq, error) {
	args := make([]seq, len(e.args))
	for i, a := range e.args {
		s, err := a.eval(c)
		if err != nil {
			return nil, err
		}
		args[i] = s
	}
	return e.fn(c, e, args)
}

// atomize returns the typed values of s: a node becomes the untyped atomic
// value of its string value.
func atomize(s seq) seq {
	out := make(seq, len(s))
	for i, it := range s {
		if it.k == kNode {
			out[i] = item{k: kUntyped, s: stringValue(it)}
		} else {
			out[i] = it
		}
	}
	return out
}

// stringValue returns the string value of a node.
func stringValue(it item) string {
	switch {
	case it.n == nil:
		return ""
	case it.a > 0:
		return it.n.Attrs[it.a-1].Value
	case it.a == textNode, len(it.n.Children) == 0:
		return it.n.Text
	}
	var b strings.Builder
	var walk func(n *xmltree.Node)
	walk = func(n *xmltree.Node) {
		b.WriteString(n.Text)
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(it.n)
	return b.String()
}

func (e *cmpExpr) eval(c *ctx) (seq, error) {
	a, err := e.a.eval(c)
	if err != nil {
		return nil, err
	}
	b, err := e.b.eval(c)
	if err != nil {
		return nil, err
	}
	ok, err := general(e.op, atomize(a), atomize(b))
	return boolSeq(ok), err
}

// general implements the general comparisons: true when a pair of values
// of a and b compares as op.
func general(op string, a, b seq) (bool, error) {
	for _, x := range a {
		for _, y := range b {
			cast := false
			switch {
			case x.k == kUntyped && y.k == kUntyped:
				x.k, y.k = kString, kString
			case x.k == kUntyped && y.numeric():
				d, err := toDouble(x)
				if err != nil {
					return false, err
				}
				x, cast = d, true
			case y.k == kUntyped && x.numeric():
				d, err := toDouble(y)
				if err != nil {
					return false, err
				}
				y, cast = d, true
			case x.k == kUntyped && y.k == kBool:
				b, err := toBoolean(x)
				if err != nil {
					return false, err
				}
				x = b
			case y.k == kUntyped && x.k == kBool:
				b, err := toBoolean(y)
				if err != nil {
					return false, err
				}
				y = b
			case x.k == kUntyped:
				x.k = kString
			case y.k == kUntyped:
				y.k = kString
			}
			ok, err := compareValues(op, x, y, cast)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// compareValues compares two atomic values. signedZero orders -0 below 0,
// as Saxon does when it compares a value cast from an untyped one, such
// as the content "-0.00" of an element, with a number: Saxon finds
// "-0.00" = 0 false and "-0.00" < 0 true (but "-0" = 0 true, the integer
// form giving 0).
func compareValues(op string, x, y item, signedZero bool) (bool, error) {
	var c int
	switch {
	case x.numeric() && y.numeric():
		if x.k == kDouble || y.k == kDouble {
			fx, fy := toFloat(x), toFloat(y)
			if math.IsNaN(fx) || math.IsNaN(fy) {
				return op == "!=", nil
			}
			c = cmpFloat(fx, fy)
			if c == 0 && fx == 0 && signedZero {
				c = boolInt(!math.Signbit(fx)) - boolInt(!math.Signbit(fy))
			}
		} else {
			c = x.r.Cmp(y.r)
		}
	case x.k == kString && y.k == kString:
		c = strings.Compare(x.s, y.s)
	case x.k == kBool && y.k == kBool:
		c = boolInt(x.b) - boolInt(y.b)
	default:
		return false, dynErr("cannot compare %s with %s", typeName(x), typeName(y))
	}
	switch op {
	case "=":
		return c == 0, nil
	case "!=":
		return c != 0, nil
	case "<":
		return c < 0, nil
	case "<=":
		return c <= 0, nil
	case ">":
		return c > 0, nil
	}
	return c >= 0, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func typeName(i item) string {
	return [...]string{"node", "xs:string", "xs:untypedAtomic", "xs:integer", "xs:decimal", "xs:double", "xs:boolean"}[i.k]
}

func toFloat(i item) float64 {
	if i.k == kDouble {
		return i.f
	}
	f, _ := i.r.Float64()
	return f
}

// toBoolean casts an untyped value to xs:boolean: true or 1, false or 0,
// surrounded by white space or not.
func toBoolean(i item) (item, error) {
	switch strings.Trim(i.s, " \t\n\r") {
	case "true", "1":
		return item{k: kBool, b: true}, nil
	case "false", "0":
		return item{k: kBool, b: false}, nil
	}
	return item{}, dynErr("cannot cast %q to xs:boolean", i.s)
}

// toDouble casts an untyped value to xs:double.
func toDouble(i item) (item, error) {
	s := strings.Trim(i.s, " \t\n\r")
	switch s {
	case "INF":
		return item{k: kDouble, f: math.Inf(1)}, nil
	case "-INF":
		return item{k: kDouble, f: math.Inf(-1)}, nil
	case "NaN":
		return item{k: kDouble, f: math.NaN()}, nil
	}
	if !reDouble.MatchString(s) {
		return item{}, dynErr("cannot cast %q to xs:double", i.s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return item{}, dynErr("cannot cast %q to xs:double", i.s)
	}
	f, _ := r.Float64()
	if f == 0 && s[0] == '-' && strings.ContainsAny(s, ".eE") {
		f = math.Copysign(0, -1)
	}
	return item{k: kDouble, f: f}, nil
}

// toNumber returns the numeric value of an atomic item for arithmetic: an
// untyped value is cast to xs:double.
func toNumber(i item) (item, error) {
	switch {
	case i.numeric():
		return i, nil
	case i.k == kUntyped:
		return toDouble(i)
	}
	return item{}, dynErr("arithmetic on %s", typeName(i))
}

func (e *arithExpr) eval(c *ctx) (seq, error) {
	a, err := e.a.eval(c)
	if err != nil {
		return nil, err
	}
	b, err := e.b.eval(c)
	if err != nil {
		return nil, err
	}
	return arith(e.op, a, b)
}

// divScale is the number of decimals of an xs:decimal division, as Saxon
// computes it (BigDecimalValue.DIVIDE_PRECISION), the quotient being
// rounded half down.
const divScale = 18

func arith(op string, a, b seq) (seq, error) {
	a, b = atomize(a), atomize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil, nil
	}
	if len(a) > 1 || len(b) > 1 {
		return nil, dynErr("arithmetic on a sequence of more than one item")
	}
	x, err := toNumber(a[0])
	if err != nil {
		return nil, err
	}
	y, err := toNumber(b[0])
	if err != nil {
		return nil, err
	}
	if x.k == kDouble || y.k == kDouble {
		fx, fy := toFloat(x), toFloat(y)
		var f float64
		switch op {
		case "+":
			f = fx + fy
		case "-":
			f = fx - fy
		case "*":
			f = fx * fy
		default:
			f = fx / fy
		}
		return seq{{k: kDouble, f: f}}, nil
	}
	k := kDecimal
	if x.k == kInteger && y.k == kInteger && op != "div" {
		k = kInteger
	}
	r := new(big.Rat)
	switch op {
	case "+":
		r.Add(x.r, y.r)
	case "-":
		r.Sub(x.r, y.r)
	case "*":
		r.Mul(x.r, y.r)
	default:
		if y.r.Sign() == 0 {
			return nil, dynErr("decimal division by zero")
		}
		r = roundHalfDown(r.Quo(x.r, y.r), divScale)
	}
	return seq{{k: k, r: r}}, nil
}

var bigTen = big.NewInt(10)

// roundHalfDown rounds r to scale decimals, halves towards zero.
func roundHalfDown(r *big.Rat, scale int) *big.Rat {
	m := new(big.Int).Exp(bigTen, big.NewInt(int64(scale)), nil)
	num := new(big.Int).Mul(r.Num(), m)
	q, rem := new(big.Int).QuoRem(num, r.Denom(), new(big.Int))
	rem.Abs(rem).Mul(rem, big.NewInt(2))
	if rem.Cmp(r.Denom()) > 0 {
		if r.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return new(big.Rat).SetFrac(q, m)
}

func (e *pathExpr) eval(c *ctx) (seq, error) {
	var cur seq
	if e.absolute {
		cur = seq{{k: kNode}}
	} else {
		cur = seq{c.it}
	}
	for i, s := range e.steps {
		var err error
		cur, err = s.apply(c, cur, i == 0 && !e.absolute)
		if err != nil {
			return nil, err
		}
	}
	return cur, nil
}

// apply evaluates the step for each item of in. The first step of a
// relative path is a primary expression evaluated with the context item
// itself, which may be an atomic value.
func (s *step) apply(c *ctx, in seq, first bool) (seq, error) {
	if s.prim != nil && first && !s.desc {
		v, err := s.prim.eval(c)
		if err != nil {
			return nil, err
		}
		return filter(c, v, s.preds, s.lenient)
	}
	for _, it := range in {
		if it.k != kNode {
			return nil, dynErr("path step on an atomic value")
		}
	}
	if s.desc {
		if out, ok, err := s.fastDescendants(c, in); ok {
			return out, err
		}
		var all seq
		for _, it := range in {
			all = append(all, descendantsOrSelf(c, it)...)
		}
		in = c.d.sortNodes(all)
	}
	var out seq
	atomic := false
	for i, it := range in {
		var r seq
		var err error
		if s.prim != nil {
			r, err = s.prim.eval(c.with(it, i+1, len(in)))
		} else {
			r = s.axisNodes(c, it)
		}
		if err != nil {
			return nil, err
		}
		if r, err = filter(c, r, s.preds, s.lenient); err != nil {
			return nil, err
		}
		for _, x := range r {
			atomic = atomic || x.k != kNode
		}
		out = append(out, r...)
	}
	if atomic {
		for _, x := range out {
			if x.k == kNode {
				return nil, dynErr("path mixing nodes and atomic values")
			}
		}
		return out, nil
	}
	if len(in) > 1 || s.axis == axAncestor || s.axis == axAncestorOrSelf {
		out = c.d.sortNodes(out)
	}
	return out, nil
}

// fastDescendants evaluates //name from the document node with the name
// index: the elements of that name in document order. It applies when the
// predicates do not depend on the position.
func (s *step) fastDescendants(c *ctx, in seq) (seq, bool, error) {
	if s.prim != nil || s.axis != axChild || len(in) != 1 || in[0].n != nil || positional(s.preds) {
		return nil, false, nil
	}
	var nodes []*xmltree.Node
	switch s.test.kind {
	case testName:
		nodes = c.d.byName[qname{s.test.space, s.test.local}]
	case testAny, testNode:
		nodes = c.d.all
	default:
		return nil, false, nil
	}
	out := make(seq, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, item{k: kNode, n: n})
	}
	out, err := filter(c, out, s.preds, s.lenient)
	return out, true, err
}

// positional reports whether a predicate may depend on the context
// position: a numeric literal, or any expression but a comparison, a
// boolean operator or a call to not() or exists().
func positional(preds []expr) bool {
	for _, p := range preds {
		switch e := p.(type) {
		case *cmpExpr, *orExpr, *andExpr:
		case *call:
			if e.name != "not" && e.name != "exists" {
				return true
			}
		case *pathExpr:
		default:
			return true
		}
	}
	return false
}

// descendantsOrSelf returns it and its descendant elements in document
// order.
func descendantsOrSelf(c *ctx, it item) seq {
	out := seq{it}
	if it.a != 0 {
		return out
	}
	var walk func(n *xmltree.Node)
	walk = func(n *xmltree.Node) {
		out = append(out, item{k: kNode, n: n})
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	for _, ch := range children(c, it) {
		walk(ch)
	}
	return out
}

// children returns the child elements of a node: the root element for the
// document node.
func children(c *ctx, it item) []*xmltree.Node {
	switch {
	case it.a != 0:
		return nil
	case it.n == nil:
		return []*xmltree.Node{c.d.root}
	}
	return it.n.Children
}

// axisNodes returns the nodes of the axis of the step from it that pass the
// node test, in axis order.
func (s *step) axisNodes(c *ctx, it item) seq {
	var out seq
	elem := func(n *xmltree.Node) {
		if s.test.matchElem(n) {
			out = append(out, item{k: kNode, n: n})
		}
	}
	switch s.axis {
	case axChild:
		if s.test.kind == testText {
			if it.a == 0 && it.n != nil && it.n.Text != "" {
				out = append(out, item{k: kNode, n: it.n, a: textNode})
			}
			return out
		}
		for _, ch := range children(c, it) {
			elem(ch)
		}
	case axAttribute:
		if it.n == nil || it.a != 0 {
			return nil
		}
		for i, a := range it.n.Attrs {
			if s.test.matchAttr(a) {
				out = append(out, item{k: kNode, n: it.n, a: i + 1})
			}
		}
	case axSelf:
		if it.a != 0 || it.n == nil {
			if s.test.kind == testNode || s.test.kind == testText && it.a == textNode {
				out = append(out, it)
			}
			return out
		}
		elem(it.n)
	case axParent:
		switch {
		case it.a != 0:
			elem(it.n)
		case it.n != nil && it.n.Parent != nil:
			elem(it.n.Parent)
		case it.n != nil && s.test.kind == testNode:
			out = append(out, item{k: kNode})
		}
	case axAncestor, axAncestorOrSelf:
		n := it.n
		if s.axis == axAncestorOrSelf && it.a == 0 && n != nil {
			elem(n)
		}
		if it.a == 0 && n != nil {
			n = n.Parent
		}
		for ; n != nil; n = n.Parent {
			elem(n)
		}
	case axDescendant, axDescendantOrSelf:
		all := descendantsOrSelf(c, it)
		if s.axis == axDescendant || it.n == nil || it.a != 0 {
			all = all[1:]
		}
		for _, x := range all {
			elem(x.n)
		}
		if s.axis == axDescendantOrSelf && it.n == nil && s.test.kind == testNode {
			out = append(seq{it}, out...)
		}
	case axFollowingSibling:
		if it.a != 0 || it.n == nil || it.n.Parent == nil {
			return nil
		}
		sib := it.n.Parent.Children
		for i, n := range sib {
			if n == it.n {
				for _, f := range sib[i+1:] {
					elem(f)
				}
				break
			}
		}
	}
	return out
}

func (t nodeTest) matchElem(n *xmltree.Node) bool {
	switch t.kind {
	case testAny, testNode:
		return true
	case testSpace:
		return n.Space == t.space
	case testText:
		return false
	}
	return n.Name == t.local && n.Space == t.space
}

func (t nodeTest) matchAttr(a xmltree.Attr) bool {
	switch t.kind {
	case testAny, testNode:
		return true
	case testSpace, testText:
		return false
	}
	return a.Prefix == "" && a.Name == t.local
}

// filter applies predicates to s. An error raised by a predicate stops
// the evaluation, or, when lenient, drops the item.
func filter(c *ctx, s seq, preds []expr, lenient bool) (seq, error) {
	for _, p := range preds {
		var out seq
		for i, it := range s {
			v, err := p.eval(c.with(it, i+1, len(s)))
			keep := false
			if err == nil {
				if len(v) == 1 && v[0].numeric() {
					keep = toFloat(v[0]) == float64(i+1)
				} else {
					keep, err = ebv(v)
				}
			}
			if err != nil {
				if lenient {
					continue
				}
				return nil, err
			}
			if keep {
				out = append(out, it)
			}
		}
		s = out
	}
	return s, nil
}

// eval implements castable as xs:date: true when the atomized operand is a
// single string or untyped value whose lexical form, white space collapsed,
// is that of an xs:date. Casting another type to xs:date fails.
func (e *castableExpr) eval(c *ctx) (seq, error) {
	v, err := e.e.eval(c)
	if err != nil {
		return nil, err
	}
	v = atomize(v)
	if len(v) != 1 || v[0].k != kString && v[0].k != kUntyped {
		return boolSeq(false), nil
	}
	return boolSeq(isDate(strings.Trim(v[0].s, " \t\n\r"))), nil
}

// isDate reports whether s is the lexical form of an xs:date: a year of
// four digits or more (no leading zero beyond four), optionally negative, a
// month and a day that exist in it, and an optional time zone (Z, or ±hh:mm
// up to ±14:00). As in XSD 1.1, which XPath 3 processors such as Saxon 12
// follow, year 0000 exists and is a leap year (1 BCE).
func isDate(s string) bool {
	s = strings.TrimPrefix(s, "-")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < 4 || i > 4 && s[0] == '0' || i > 18 || len(s) < i+6 {
		return false
	}
	year, _ := strconv.ParseInt(s[:i], 10, 64)
	rest := s[i:]
	if rest[0] != '-' || rest[3] != '-' {
		return false
	}
	month, ok1 := twoDigits(rest[1:3])
	day, ok2 := twoDigits(rest[4:6])
	if !ok1 || !ok2 || month < 1 || month > 12 || day < 1 || day > daysIn(year, month) {
		return false
	}
	switch tz := rest[6:]; {
	case tz == "", tz == "Z":
		return true
	case len(tz) == 6 && (tz[0] == '+' || tz[0] == '-') && tz[3] == ':':
		h, ok1 := twoDigits(tz[1:3])
		m, ok2 := twoDigits(tz[4:6])
		return ok1 && ok2 && m < 60 && (h < 14 || h == 14 && m == 0)
	}
	return false
}

func twoDigits(s string) (int, bool) {
	if len(s) != 2 || s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return 0, false
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

func daysIn(year int64, month int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}
