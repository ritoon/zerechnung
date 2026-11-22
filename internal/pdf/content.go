package pdf

import (
	"bytes"
	"errors"
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
// it. Kind tells which field holds its value.
type Operand struct {
	Kind  OperandKind
	Bool  bool      // OperandBool
	Int   int64     // OperandInt
	Real  float64   // OperandReal
	Str   String    // OperandString, decoded
	Name  Name      // OperandName
	Array []Operand // OperandArray
	Dict  Dict      // OperandDict
	Obj   Object    // OperandOther: a Ref
}

// Object returns the operand as ScanContent gives it: Bool, Int, Real,
// String, Name, Array, Dict or Ref, nil for null. It allocates.
func (o *Operand) Object() Object {
	switch o.Kind {
	case OperandBool:
		return Bool(o.Bool)
	case OperandInt:
		return intObject(o.Int)
	case OperandReal:
		return Real(o.Real)
	case OperandString:
		return o.Str
	case OperandName:
		return o.Name
	case OperandArray:
		arr := make(Array, len(o.Array))
		for i := range o.Array {
			arr[i] = o.Array[i].Object()
		}
		return arr
	case OperandDict:
		return o.Dict
	case OperandOther:
		return o.Obj
	}
	return nil
}

// AsInt returns the operand as an integer, as AsInt does with an Object: an
// integer, or a real with an integral value.
func (o *Operand) AsInt() (int64, bool) {
	switch o.Kind {
	case OperandInt:
		return o.Int, true
	case OperandReal:
		if float64(int64(o.Real)) == o.Real {
			return int64(o.Real), true
		}
	}
	return 0, false
}

// AsName returns the operand as a name.
func (o *Operand) AsName() (Name, bool) { return o.Name, o.Kind == OperandName }

// AsString returns the operand as a string.
func (o *Operand) AsString() (String, bool) { return o.Str, o.Kind == OperandString }

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
	p := &parser{b: data, cacheNames: true}
	var (
		args []Operand
		a    = operandArena{ownStrings: !arenaStrings}
	)
	for {
		var o Operand
		kw, isOp, err := p.operand(&o, &a)
		switch {
		case errors.Is(err, errEOF):
			return nil
		case err != nil:
			// An operand that cannot be read: skip it, keep the others.
			continue
		}
		if !isOp {
			if len(args) < maxOperands {
				args = append(args, o)
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
			args = append(args[:0], Operand{Kind: OperandDict, Dict: d})
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
// elements of their arrays and the decoded bytes of their strings. It is
// reset after each operator; what it lent before stays valid until then,
// even when it grows.
type operandArena struct {
	elems []Operand // elements of the arrays read
	stack []Operand // elements of the arrays being read, nested ones on top
	bytes []byte    // decoded strings

	ownStrings bool // decode each string in a slice of its own instead of bytes
}

// strings returns where to decode a string: nil for a slice of its own.
func (a *operandArena) strings() *[]byte {
	if a.ownStrings {
		return nil
	}
	return &a.bytes
}

func (a *operandArena) reset() {
	clear(a.elems) // drop what the operands referenced
	a.elems = a.elems[:0]
	a.bytes = a.bytes[:0]
}

// keep copies the elements of an array into the arena.
func (a *operandArena) keep(elems []Operand) []Operand {
	if len(elems) == 0 {
		return nil
	}
	if cap(a.elems)-len(a.elems) < len(elems) {
		// A new block: the arrays already lent keep the old one.
		a.elems = make([]Operand, 0, max(2*cap(a.elems), len(elems), 64))
	}
	n := len(a.elems)
	a.elems = append(a.elems, elems...)
	return a.elems[n:len(a.elems):len(a.elems)]
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
		*o = Operand{Kind: OperandName, Name: p.name()}
		return "", false, nil
	case c == '(':
		s, err := p.literalBytes(a.strings())
		*o = Operand{Kind: OperandString, Str: s}
		return "", false, err
	case c == '<' && (p.pos+1 >= len(p.b) || p.b[p.pos+1] != '<'):
		s, err := p.hexBytes(a.strings())
		*o = Operand{Kind: OperandString, Str: s}
		return "", false, err
	case c == '[':
		elems, err := p.operandArray(a)
		*o = Operand{Kind: OperandArray, Array: elems}
		return "", false, err
	case c == '<':
		d, err := p.dictValue()
		*o = Operand{Kind: OperandDict, Dict: d}
		return "", false, err
	case isDigit(c) || c == '+' || c == '-' || c == '.':
		switch kind, n, f, gen := p.scanNumber(); kind {
		case numReal:
			*o = Operand{Kind: OperandReal, Real: f}
		case numRef:
			*o = Operand{Kind: OperandOther, Obj: Ref{Num: uint32(n), Gen: gen}}
		default:
			*o = Operand{Kind: OperandInt, Int: n}
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
		*o = Operand{Kind: OperandBool, Bool: bool(v)}
	default:
		*o = Operand{Kind: OperandOther, Obj: v}
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
