package pdf

import (
	"bytes"
	"errors"
	"math"
	"sync"
)

// ErrInlineImage is returned by ScanContent when the data of an inline image
// has no end (no EI operator after ID).
var ErrInlineImage = errors.New("pdf: inline image without EI")

// maxOperands bounds the operands kept for an operator: no operator of ISO
// 32000-1 takes more than a few dozen (scn with a DeviceN colour space of 32
// colourants and a pattern name).
const maxOperands = 64

// ScanContent reads the operators of a content stream (ISO 32000-1, 7.8.2)
// and calls fn with each operator and its operands. An inline image (BI,
// ID, data, EI) is reported once, as the operator "BI" with a single
// operand: the dictionary of its parameters, with the keys as written,
// abbreviated or not. Comments are skipped. The streams of a page are
// scanned as one: the caller joins them, with white space between them.
//
// Scanning stops at the end of data, at the first error fn returns, which
// ScanContent returns, or at an inline image without end (ErrInlineImage).
// Syntax errors in operands are skipped, as viewers do.
//
// The slice of operands is reused from one operator to the next: fn must
// copy it to keep it. ScanOperands reads the same operands without boxing
// them in Objects, which allocates far less.
func ScanContent(data []byte, fn func(op string, args []Object) error) error {
	var objs []Object
	// The strings are decoded in slices of their own, as Reader does: fn
	// may keep the Objects it is given, not the slice that holds them.
	return scanOperands(data, false, func(op string, args []Operand) error {
		objs = objs[:0]
		for i := range args {
			objs = append(objs, args[i].Object())
		}
		return fn(op, objs)
	})
}

// OperandKind is the type of an Operand.
type OperandKind uint8

// The kinds of operands. A dictionary holds the properties of BDC and DP,
// or the parameters of an inline image. OperandOther holds what a content
// stream should not have as an operand: an indirect reference.
const (
	OperandNull OperandKind = iota
	OperandBool
	OperandInt
	OperandReal
	OperandString
	OperandName
	OperandArray
	OperandDict
	OperandOther
)

// Operand is an operand of a content stream operator, as ScanOperands reads
// it. Kind tells its type, the As methods and Object its value.
//
// An Operand takes 32 bytes, twice an Object: a content stream holds arrays
// of thousands of numbers (TJ), and fewer bytes per element keep the memory
// a stream costs close to its size. A number is held in n; a string, a
// name, an array or a dictionary through a pointer into the memory of the
// scan, which ScanOperands lends until the next operator.
type Operand struct {
	kind OperandKind
	// OperandBool: 0 or 1; OperandInt: the integer; OperandReal: its IEEE
	// 754 bits; OperandOther: a Ref, Num<<16 | Gen, when v is nil.
	n uint64
	// *String, *Name, *[]Operand or *Dict; for OperandOther, an Object
	// other than a Ref.
	v any
}

// Kind returns the type of the operand.
func (o *Operand) Kind() OperandKind { return o.kind }

// Object returns the operand as ScanContent gives it: Bool, Int, Real,
// String, Name, Array, Dict or Ref, nil for null. It allocates.
func (o *Operand) Object() Object {
	switch o.kind {
	case OperandBool:
		return Bool(o.n != 0)
	case OperandInt:
		return intObject(int64(o.n))
	case OperandReal:
		return Real(math.Float64frombits(o.n))
	case OperandString:
		return *o.v.(*String)
	case OperandName:
		return *o.v.(*Name)
	case OperandArray:
		elems := *o.v.(*[]Operand)
		arr := make(Array, len(elems))
		for i := range elems {
			arr[i] = elems[i].Object()
		}
		return arr
	case OperandDict:
		return *o.v.(*Dict)
	case OperandOther:
		if o.v != nil {
			return o.v
		}
		return Ref{Num: uint32(o.n >> 16), Gen: uint16(o.n)}
	}
	return nil
}

// AsInt returns the operand as an integer, as AsInt does with an Object: an
// integer, or a real with an integral value.
func (o *Operand) AsInt() (int64, bool) {
	switch o.kind {
	case OperandInt:
		return int64(o.n), true
	case OperandReal:
		if f := math.Float64frombits(o.n); float64(int64(f)) == f {
			return int64(f), true
		}
	}
	return 0, false
}

// AsName returns the operand as a name.
func (o *Operand) AsName() (Name, bool) {
	if o.kind != OperandName {
		return "", false
	}
	return *o.v.(*Name), true
}

// AsString returns the operand as a string.
func (o *Operand) AsString() (String, bool) {
	if o.kind != OperandString {
		return nil, false
	}
	return *o.v.(*String), true
}

// AsArray returns the elements of the operand when it is an array.
func (o *Operand) AsArray() ([]Operand, bool) {
	if o.kind != OperandArray {
		return nil, false
	}
	return *o.v.(*[]Operand), true
}

// AsDict returns the operand as a dictionary.
func (o *Operand) AsDict() (Dict, bool) {
	if o.kind != OperandDict {
		return nil, false
	}
	return *o.v.(*Dict), true
}

// ScanOperands is ScanContent with operands that are not boxed in Objects.
// The operands, the elements of their arrays and the bytes of their strings
// are valid during the call of fn only: ScanOperands reuses their memory
// for the next operator, and fn must copy what it keeps. Names and Dicts
// can be kept.
func ScanOperands(data []byte, fn func(op string, args []Operand) error) error {
	return scanOperands(data, true, fn)
}

// scanOperands is ScanOperands; the arena decodes the strings when
// arenaStrings is set, which lends their bytes until the next operator.
func scanOperands(data []byte, arenaStrings bool, fn func(op string, args []Operand) error) error {
	a := operandArenas.Get().(*operandArena)
	defer a.release()
	a.ownStrings = !arenaStrings
	a.p = parser{b: data, kvs: &a.kvs, names: &a.cache}
	p, args := &a.p, a.args[:0]
	defer func() { a.args = args }()
	for {
		var o Operand
		// Nothing of the operands beyond maxOperands stays in the arena,
		// however many precede the operator.
		full := len(args) >= maxOperands
		var m arenaMark
		if full {
			m = a.mark()
		}
		kw, isOp, err := p.operand(&o, a)
		if err != nil {
			if errors.Is(err, errEOF) {
				return nil
			}
			// An operand that cannot be read: skip it, keep the others.
			// It left nothing in the arena (see operand).
			continue
		}
		if !isOp {
			if !full {
				args = append(args, o)
			} else {
				a.drop(m)
			}
			continue
		}
		op := string(kw)
		switch op {
		case "]", ">", ")", "{", "}":
			// Stray delimiters: not operators.
			continue
		case "BI":
			d, err := p.inlineImage()
			if err != nil {
				return err
			}
			args = append(args[:0], Operand{kind: OperandDict, v: a.dicts.put(d)})
			if err := fn("BI", args); err != nil {
				return err
			}
		default:
			if err := fn(op, args); err != nil {
				return err
			}
		}
		args = args[:0]
		a.reset()
	}
}

// operandArena holds the memory of the operands of an operator: the
// elements of their arrays, the decoded bytes of their strings and what the
// Operands point to. It is reset after each operator; what it lent before
// stays valid until then, even when it grows. A scan takes an arena from
// operandArenas and puts it back at its end: a scan run by fn takes
// another one.
type operandArena struct {
	elems slab[Operand]   // elements of the arrays read
	stack []Operand       // elements of the arrays being read, nested ones on top
	bytes []byte          // decoded strings
	strs  slab[String]    // strings
	arrs  slab[[]Operand] // arrays
	dicts slab[Dict]      // dictionaries
	names slab[Name]      // names the parser does not remember

	p     parser    // the parser of the scan
	args  []Operand // the operands of the operator
	kvs   []KV      // the scratch space of p
	cache nameCache // the names p remembers, from a scan to the next

	ownStrings bool // decode each string in a slice of its own instead of bytes
}

var operandArenas = sync.Pool{New: func() any { return new(operandArena) }}

// An arena that grew beyond these limits is not put back in operandArenas:
// it would keep the memory of an exceptional stream.
const (
	maxPooledOperands = 1 << 12
	maxPooledBytes    = 1 << 16
)

// release puts the arena back in operandArenas, unless it grew too much.
func (a *operandArena) release() {
	a.p = parser{} // drop the data
	if max(cap(a.elems.s), cap(a.stack), cap(a.strs.s), cap(a.arrs.s), cap(a.dicts.s), cap(a.names.s), cap(a.kvs)) > maxPooledOperands ||
		cap(a.bytes) > maxPooledBytes {
		return
	}
	a.reset()
	clear(a.args[:cap(a.args)])
	a.args = a.args[:0]
	clear(a.kvs[:cap(a.kvs)])
	a.kvs = a.kvs[:0]
	if len(a.cache.m) > maxNames/2 {
		// Room for the names of the next scan.
		a.cache.reset()
	}
	operandArenas.Put(a)
}

// strings returns where to decode a string: nil for a slice of its own.
func (a *operandArena) strings() *[]byte {
	if a.ownStrings {
		return nil
	}
	return &a.bytes
}

func (a *operandArena) reset() {
	a.elems.reset()
	a.bytes = a.bytes[:0]
	a.strs.reset()
	a.arrs.reset()
	a.dicts.reset()
	a.names.reset()
}

// arenaMark is what the arena holds at a time, to drop what came after.
type arenaMark struct {
	bytes                           int
	elems, strs, arrs, dicts, names slabMark
}

func (a *operandArena) mark() arenaMark {
	return arenaMark{
		bytes: len(a.bytes),
		elems: a.elems.mark(), strs: a.strs.mark(), arrs: a.arrs.mark(),
		dicts: a.dicts.mark(), names: a.names.mark(),
	}
}

// drop forgets what the arena lent after m: the memory of an operand that
// will not be passed to fn.
func (a *operandArena) drop(m arenaMark) {
	a.bytes = a.bytes[:m.bytes]
	a.elems.drop(m.elems)
	a.strs.drop(m.strs)
	a.arrs.drop(m.arrs)
	a.dicts.drop(m.dicts)
	a.names.drop(m.names)
}

// keep copies the elements of an array into the arena.
func (a *operandArena) keep(elems []Operand) []Operand {
	if len(elems) == 0 {
		return nil
	}
	return a.elems.putSlice(elems)
}

// name reads a name after the slash: the copy the parser remembers, or one
// in the arena.
func (a *operandArena) name(p *parser) *Name {
	w := p.word()
	if n := p.names.name(w); n != nil {
		return n
	}
	return a.names.put(nameOf(w))
}

// slab lends pointers to the values put in it, valid until reset or drop,
// even when it grows: a full block is left to what points into it.
type slab[T any] struct {
	s      []T
	blocks int // the blocks made, to tell drop whether s is a new one
}

// slabMark is the length of a slab at a time, and its block.
type slabMark struct{ n, block int }

// put copies v into the slab and returns the copy.
func (b *slab[T]) put(v T) *T {
	if len(b.s) == cap(b.s) {
		b.grow(1)
	}
	b.s = append(b.s, v)
	return &b.s[len(b.s)-1]
}

// putSlice copies vs into the slab and returns the copy.
func (b *slab[T]) putSlice(vs []T) []T {
	if cap(b.s)-len(b.s) < len(vs) {
		b.grow(len(vs))
	}
	n := len(b.s)
	b.s = append(b.s, vs...)
	return b.s[n:len(b.s):len(b.s)]
}

// grow starts a new block, with room for n values at least.
func (b *slab[T]) grow(n int) {
	b.s = make([]T, 0, max(2*cap(b.s), n, 16))
	b.blocks++
}

func (b *slab[T]) mark() slabMark { return slabMark{len(b.s), b.blocks} }

// drop forgets the values put after m. A block made since holds only them.
func (b *slab[T]) drop(m slabMark) {
	n := m.n
	if b.blocks != m.block {
		n = 0
	}
	if len(b.s) > n {
		clear(b.s[n:]) // drop what they referenced
		b.s = b.s[:n]
	}
}

func (b *slab[T]) reset() {
	if len(b.s) > 0 {
		clear(b.s)
		b.s = b.s[:0]
	}
}

// operand reads the next operand into o, or the next operator, returned as
// kw with isOp set. It reads what object reads, with the same errors.
func (p *parser) operand(o *Operand, a *operandArena) (kw keyword, isOp bool, err error) {
	p.skip()
	if p.pos >= len(p.b) {
		return "", false, errEOF
	}
	switch c := p.b[p.pos]; {
	case c == '/':
		p.pos++
		*o = Operand{kind: OperandName, v: a.name(p)}
		return "", false, nil
	case c == '(':
		s, err := p.literalBytes(a.strings())
		if err == nil {
			*o = Operand{kind: OperandString, v: a.strs.put(s)}
		}
		return "", false, err
	case c == '<' && (p.pos+1 >= len(p.b) || p.b[p.pos+1] != '<'):
		s, err := p.hexBytes(a.strings())
		if err == nil {
			*o = Operand{kind: OperandString, v: a.strs.put(s)}
		}
		return "", false, err
	case c == '[':
		// Only an array can fail after it used the arena: strings do
		// not fail, dictionaries do not use it.
		m := a.mark()
		elems, err := p.operandArray(a)
		if err != nil {
			a.drop(m) // the arrays it holds, read before the error
			return "", false, err
		}
		*o = Operand{kind: OperandArray, v: a.arrs.put(elems)}
		return "", false, nil
	case c == '<':
		d, err := p.dictValue()
		if err == nil {
			*o = Operand{kind: OperandDict, v: a.dicts.put(d)}
		}
		return "", false, err
	case isDigit(c) || c == '+' || c == '-' || c == '.':
		switch kind, n, f, gen := p.scanNumber(); kind {
		case numReal:
			*o = Operand{kind: OperandReal, n: math.Float64bits(f)}
		case numRef:
			*o = Operand{kind: OperandOther, n: uint64(n)<<16 | uint64(gen)}
		default:
			*o = Operand{kind: OperandInt, n: uint64(n)}
		}
		return "", false, nil
	}
	v, err := p.object()
	if err != nil {
		return "", false, err
	}
	switch v := v.(type) {
	case keyword:
		return v, true, nil
	case nil:
		*o = Operand{}
	case Bool:
		*o = Operand{kind: OperandBool}
		if v {
			o.n = 1
		}
	default:
		*o = Operand{kind: OperandOther, v: v}
	}
	return "", false, nil
}

// operandArray reads an array after "[", as array does, its elements
// kept in the arena.
func (p *parser) operandArray(a *operandArena) ([]Operand, error) {
	p.pos++ // [
	if p.depth++; p.depth > maxDepth {
		return nil, errTooDeep
	}
	defer func() { p.depth-- }()
	base := len(a.stack)
	defer func() {
		clear(a.stack[base:])
		a.stack = a.stack[:base]
	}()
	for {
		p.skip()
		if p.pos >= len(p.b) {
			return a.keep(a.stack[base:]), nil // truncated: keep what was read
		}
		if p.b[p.pos] == ']' {
			p.pos++
			return a.keep(a.stack[base:]), nil
		}
		save := p.pos
		var o Operand
		kw, isOp, err := p.operand(&o, a)
		if err == errTooDeep {
			return nil, err
		}
		if err != nil {
			continue
		}
		if isOp {
			switch kw {
			case "endobj", "stream", "endstream", "obj", ">":
				// Unterminated array: stop before the keyword.
				p.pos = save
				return a.keep(a.stack[base:]), nil
			}
			o = Operand{} // unknown keyword read as null
		}
		if len(a.stack) == cap(a.stack) {
			// Double: append grows a large slice by a quarter, which
			// allocates five times its size in all for an array of
			// thousands of numbers.
			stack := make([]Operand, len(a.stack), max(2*cap(a.stack), 16))
			copy(stack, a.stack)
			a.stack = stack
		}
		a.stack = append(a.stack, o)
	}
}

// inlineImage reads the parameters of an inline image after BI, and skips
// its data up to the EI operator.
func (p *parser) inlineImage() (Dict, error) {
	var d Dict
	for {
		o, err := p.object()
		if errors.Is(err, errEOF) {
			return d, ErrInlineImage
		}
		if err != nil {
			continue
		}
		if kw, ok := o.(keyword); ok {
			if kw == "ID" {
				break
			}
			continue
		}
		key, ok := o.(Name)
		if !ok {
			continue
		}
		v, err := p.object()
		if errors.Is(err, errEOF) {
			return d, ErrInlineImage
		}
		if _, ok := v.(keyword); ok {
			// "ID" (or another keyword) where a value was expected.
			if v == keyword("ID") {
				break
			}
			continue
		}
		d.Set(key, v)
	}
	end, ok := inlineDataEnd(p.b, p.pos)
	if !ok {
		return d, ErrInlineImage
	}
	p.pos = end
	return d, nil
}

// inlineDataEnd returns the offset after the EI operator that ends the data
// of an inline image, the ID operator ending at pos. A single white-space
// character follows ID, then the data; EI ends it: the first one preceded
// by white space and followed by white space, a delimiter or the end of the
// stream.
func inlineDataEnd(b []byte, pos int) (int, bool) {
	if pos < len(b) && class[b[pos]] == white {
		pos++
	}
	for i := pos; ; {
		k := bytes.Index(b[i:], []byte("EI"))
		if k < 0 {
			return 0, false
		}
		k += i
		before := k == pos || class[b[k-1]] == white
		after := k+2 == len(b) || class[b[k+2]] != regular
		if before && after {
			return k + 2, true
		}
		i = k + 1
	}
}

// ContentSyntax checks the operands of a content stream as Syntax checks
// the tokens of an object, and calls fn for each deviation: hexadecimal
// strings (6.1.6) and the limits on numbers, strings and names (6.1.13).
// The data of inline images is skipped. The issues have no object number.
func ContentSyntax(data []byte, fn func(SyntaxIssue)) {
	l := lint{b: data, end: len(data), fn: fn}
	for {
		kw, _ := l.scan()
		switch kw {
		case "":
			return
		case "ID":
			end, ok := inlineDataEnd(data, l.pos)
			if !ok {
				return
			}
			l.pos = end
		}
	}
}
