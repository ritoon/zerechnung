package pdf

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
)

//go:generate go run ./gen -o ints_gen.go

// Character classes (ISO 32000-1, 7.2.2).
const (
	regular = iota
	white
	delim
)

var class = func() (c [256]uint8) {
	for _, b := range []byte{0, '\t', '\n', '\f', '\r', ' '} {
		c[b] = white
	}
	for _, b := range []byte("()<>[]{}/%") {
		c[b] = delim
	}
	return
}()

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isRegular(c byte) bool { return class[c] == regular }

// maxDepth bounds the nesting of arrays and dictionaries.
const maxDepth = 256

var (
	errEOF       = errors.New("pdf: unexpected end of data")
	errSyntax    = errors.New("pdf: syntax error")
	errTooDeep   = errors.New("pdf: objects nested too deeply")
	errNotObject = errors.New("pdf: no object header at offset")
)

// keyword is a bare keyword met where an object was expected ("endobj",
// "stream", "R"...). It never escapes the parser.
type keyword string

// parser reads objects from b. r, when set, lends it its scratch space.
type parser struct {
	b     []byte
	pos   int
	r     *Reader
	depth int
	kvs   *[]KV      // scratch space of dict, shared by the parsers of a Reader
	sel   *selection // when set, dict skips some arrays (see DictSkipArrays)

	// The unsigned integer read after an integer by number, looking for
	// "gen R", ends at aheadEnd. When it is not a generation number, it is
	// the next object: number takes it from there instead of reading it
	// again, which halves the work on arrays of numbers.
	aheadAt, aheadEnd int
	ahead             int64
}

// scratch returns the space in which dict collects the entries of the
// dictionaries being read, nested ones on top of the others.
func (p *parser) scratch() *[]KV {
	if p.kvs == nil {
		if p.r != nil {
			p.kvs = &p.r.kvs
		} else {
			p.kvs = new([]KV)
		}
		if *p.kvs == nil {
			*p.kvs = make([]KV, 0, 32)
		}
	}
	return p.kvs
}

// skip skips white space and comments.
func (p *parser) skip() {
	b := p.b
	for p.pos < len(b) {
		c := b[p.pos]
		if class[c] == white {
			p.pos++
			continue
		}
		if c == '%' {
			for p.pos < len(b) && b[p.pos] != '\n' && b[p.pos] != '\r' {
				p.pos++
			}
			continue
		}
		return
	}
}

// word returns the run of regular characters at the current position.
func (p *parser) word() []byte {
	start := p.pos
	for p.pos < len(p.b) && isRegular(p.b[p.pos]) {
		p.pos++
	}
	return p.b[start:p.pos]
}

// uint reads an unsigned decimal integer; ok is false when there is none.
func (p *parser) uint() (int64, bool) {
	start := p.pos
	var n int64
	for p.pos < len(p.b) && isDigit(p.b[p.pos]) {
		if n > (math.MaxInt64-9)/10 {
			return 0, false
		}
		n = n*10 + int64(p.b[p.pos]-'0')
		p.pos++
	}
	return n, p.pos > start
}

// object reads the next object. Bare keywords are returned as keyword.
func (p *parser) object() (Object, error) {
	p.skip()
	if p.pos >= len(p.b) {
		return nil, errEOF
	}
	c := p.b[p.pos]
	switch {
	case c == '/':
		p.pos++
		return p.name(), nil
	case c == '(':
		return p.literal()
	case c == '<':
		if p.pos+1 < len(p.b) && p.b[p.pos+1] == '<' {
			return p.dict()
		}
		return p.hex()
	case c == '[':
		return p.array()
	case isDigit(c) || c == '+' || c == '-' || c == '.':
		return p.number(), nil
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		p.pos++
		return keyword(p.b[p.pos-1 : p.pos]), nil
	}
	w := p.word()
	switch string(w) {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "null":
		return nil, nil
	}
	if len(w) == 0 {
		p.pos++
		return nil, errSyntax
	}
	if k, ok := keywords[string(w)]; ok {
		return k, nil
	}
	return keyword(w), nil
}

// keywords are the keywords of the file structure and the operators of the
// content streams (ISO 32000-1, Annex A), boxed once so that reading them
// does not allocate.
var keywords = func() map[string]Object {
	m := map[string]Object{}
	for _, k := range strings.Fields(`obj endobj stream endstream R xref trailer startxref
		b B b* B* BDC BI BMC BT BX c cm CS cs d d0 d1 Do DP EI EMC ET EX f F f* G g gs h i ID
		j J K k l m M MP n q Q re RG rg ri s S SC sc SCN scn sh T* Tc Td TD Tf TJ Tj TL Tm Tr
		Ts Tw Tz v w W W* y ' "`) {
		m[k] = keyword(k)
	}
	return m
}()

// name reads a name after the slash.
func (p *parser) name() Name {
	w := p.word()
	if bytes.IndexByte(w, '#') < 0 {
		return internName(w)
	}
	out := make([]byte, 0, len(w))
	for i := 0; i < len(w); i++ {
		if w[i] == '#' && i+2 < len(w) && isHex(w[i+1]) && isHex(w[i+2]) {
			out = append(out, unhex(w[i+1])<<4|unhex(w[i+2]))
			i += 2
			continue
		}
		out = append(out, w[i])
	}
	return Name(out)
}

// internName avoids allocating the most common names.
func internName(w []byte) Name {
	switch string(w) {
	case "Type":
		return "Type"
	case "Subtype":
		return "Subtype"
	case "Length":
		return "Length"
	case "Filter":
		return "Filter"
	case "FlateDecode":
		return "FlateDecode"
	case "DecodeParms":
		return "DecodeParms"
	case "Names":
		return "Names"
	case "Kids":
		return "Kids"
	case "Limits":
		return "Limits"
	case "EmbeddedFiles":
		return "EmbeddedFiles"
	case "EmbeddedFile":
		return "EmbeddedFile"
	case "Filespec":
		return "Filespec"
	case "F":
		return "F"
	case "UF":
		return "UF"
	case "EF":
		return "EF"
	case "Desc":
		return "Desc"
	case "AF":
		return "AF"
	case "AFRelationship":
		return "AFRelationship"
	case "Alternative":
		return "Alternative"
	case "Data":
		return "Data"
	case "Source":
		return "Source"
	case "Params":
		return "Params"
	case "ModDate":
		return "ModDate"
	case "CreationDate":
		return "CreationDate"
	case "Size":
		return "Size"
	case "CheckSum":
		return "CheckSum"
	case "Root":
		return "Root"
	case "Info":
		return "Info"
	case "ID":
		return "ID"
	case "Prev":
		return "Prev"
	case "Catalog":
		return "Catalog"
	case "Pages":
		return "Pages"
	case "Page":
		return "Page"
	case "Metadata":
		return "Metadata"
	case "XML":
		return "XML"
	case "OutputIntents":
		return "OutputIntents"
	case "Count":
		return "Count"
	case "Parent":
		return "Parent"
	case "Resources":
		return "Resources"
	case "MediaBox":
		return "MediaBox"
	case "Contents":
		return "Contents"
	case "Font":
		return "Font"
	case "XObject":
		return "XObject"
	case "ProcSet":
		return "ProcSet"
	case "Producer":
		return "Producer"
	case "Creator":
		return "Creator"
	case "Title":
		return "Title"
	case "Author":
		return "Author"
	case "ObjStm":
		return "ObjStm"
	case "XRef":
		return "XRef"
	case "N":
		return "N"
	case "First":
		return "First"
	case "W":
		return "W"
	case "Index":
		return "Index"
	case "Predictor":
		return "Predictor"
	case "Columns":
		return "Columns"
	case "text/xml":
		return "text/xml"
	case "FontDescriptor":
		return "FontDescriptor"
	case "FontName":
		return "FontName"
	case "FontBBox":
		return "FontBBox"
	case "FontFile":
		return "FontFile"
	case "FontFile2":
		return "FontFile2"
	case "FontFile3":
		return "FontFile3"
	case "Flags":
		return "Flags"
	case "ItalicAngle":
		return "ItalicAngle"
	case "Ascent":
		return "Ascent"
	case "Descent":
		return "Descent"
	case "CapHeight":
		return "CapHeight"
	case "StemV":
		return "StemV"
	case "Length1":
		return "Length1"
	case "Length2":
		return "Length2"
	case "Length3":
		return "Length3"
	case "BaseFont":
		return "BaseFont"
	case "Encoding":
		return "Encoding"
	case "WinAnsiEncoding":
		return "WinAnsiEncoding"
	case "MacRomanEncoding":
		return "MacRomanEncoding"
	case "FirstChar":
		return "FirstChar"
	case "LastChar":
		return "LastChar"
	case "Widths":
		return "Widths"
	case "Type0":
		return "Type0"
	case "Type1":
		return "Type1"
	case "Type3":
		return "Type3"
	case "TrueType":
		return "TrueType"
	case "DescendantFonts":
		return "DescendantFonts"
	case "ToUnicode":
		return "ToUnicode"
	case "Annots":
		return "Annots"
	case "ColorSpace":
		return "ColorSpace"
	case "DefaultRGB":
		return "DefaultRGB"
	case "ICCBased":
		return "ICCBased"
	case "DeviceRGB":
		return "DeviceRGB"
	case "DeviceGray":
		return "DeviceGray"
	case "DeviceCMYK":
		return "DeviceCMYK"
	case "ExtGState":
		return "ExtGState"
	case "Shading":
		return "Shading"
	case "Pattern":
		return "Pattern"
	case "SMask":
		return "SMask"
	case "Image":
		return "Image"
	case "Form":
		return "Form"
	case "BBox":
		return "BBox"
	case "Matrix":
		return "Matrix"
	case "Width":
		return "Width"
	case "Height":
		return "Height"
	case "BitsPerComponent":
		return "BitsPerComponent"
	case "Rotate":
		return "Rotate"
	case "Group":
		return "Group"
	case "Version":
		return "Version"
	case "PageMode":
		return "PageMode"
	case "UseNone":
		return "UseNone"
	case "OpenAction":
		return "OpenAction"
	case "Outlines":
		return "Outlines"
	case "AcroForm":
		return "AcroForm"
	case "StructTreeRoot":
		return "StructTreeRoot"
	case "MarkInfo":
		return "MarkInfo"
	case "Lang":
		return "Lang"
	case "OutputIntent":
		return "OutputIntent"
	case "DestOutputProfile":
		return "DestOutputProfile"
	case "GTS_PDFA1":
		return "GTS_PDFA1"
	case "S":
		return "S"
	}
	return Name(w)
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// literal reads a literal string; the result aliases the input when it holds
// no escape and no carriage return.
func (p *parser) literal() (Object, error) {
	b := p.b
	start := p.pos + 1
	// Fast path: no backslash, no CR, no nested parenthesis.
	for i := start; i < len(b); i++ {
		switch b[i] {
		case ')':
			p.pos = i + 1
			return String(b[start:i:i]), nil
		case '\\', '(', '\r':
			return p.literalSlow()
		}
	}
	p.pos = len(b)
	return String(b[start:len(b):len(b)]), nil
}

func (p *parser) literalSlow() (Object, error) {
	b := p.b
	i := p.pos + 1
	out := make([]byte, 0, 32)
	level := 1
	for i < len(b) {
		c := b[i]
		i++
		switch c {
		case '(':
			level++
		case ')':
			level--
			if level == 0 {
				p.pos = i
				return String(out), nil
			}
		case '\r':
			// An unescaped end of line is read as a line feed.
			if i < len(b) && b[i] == '\n' {
				i++
			}
			c = '\n'
		case '\\':
			if i >= len(b) {
				break
			}
			e := b[i]
			i++
			switch e {
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case '\r':
				if i < len(b) && b[i] == '\n' {
					i++
				}
				continue
			case '\n':
				continue
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && i < len(b) && b[i] >= '0' && b[i] <= '7'; k++ {
						v = v*8 + int(b[i]-'0')
						i++
					}
					c = byte(v)
				} else {
					c = e // \( \) \\ and unknown escapes
				}
			}
		}
		out = append(out, c)
	}
	p.pos = len(b)
	return String(out), nil
}

// hex reads a hexadecimal string.
func (p *parser) hex() (Object, error) {
	b := p.b
	i := p.pos + 1
	out := make([]byte, 0, 16)
	var hi byte
	half := false
	for i < len(b) {
		c := b[i]
		i++
		if c == '>' {
			p.pos = i
			if half {
				out = append(out, hi<<4)
			}
			return String(out), nil
		}
		if !isHex(c) {
			if class[c] == white {
				continue
			}
			// Tolerate garbage: skip it.
			continue
		}
		if half {
			out = append(out, hi<<4|unhex(c))
			half = false
		} else {
			hi = unhex(c)
			half = true
		}
	}
	p.pos = len(b)
	return String(out), nil
}

// number reads a number; an unsigned integer followed by another one and "R"
// is read as an indirect reference.
func (p *parser) number() Object {
	b := p.b
	start := p.pos
	neg := false
	for p.pos < len(b) && (b[p.pos] == '+' || b[p.pos] == '-') {
		neg = b[p.pos] == '-'
		p.pos++
	}
	signed := p.pos > start
	intStart := p.pos
	var n int64
	var isInt bool
	if p.pos == p.aheadAt && p.aheadEnd > p.aheadAt {
		n, isInt, p.pos = p.ahead, true, p.aheadEnd
	} else {
		n, isInt = p.uint()
	}
	if p.pos < len(b) && b[p.pos] == '.' {
		// Real number.
		p.pos++
		for p.pos < len(b) && isDigit(b[p.pos]) {
			p.pos++
		}
		f, err := strconv.ParseFloat(string(b[intStart:p.pos]), 64)
		if err != nil {
			f = 0
		}
		// Skip trailing garbage glued to the number ("1.2.3", "4-").
		for p.pos < len(b) && isRegular(b[p.pos]) {
			p.pos++
		}
		if neg {
			f = -f
		}
		return Real(f)
	}
	if !isInt {
		// Overflow or lone sign.
		for p.pos < len(b) && isRegular(b[p.pos]) {
			p.pos++
		}
		digits := b[intStart:p.pos]
		if f, err := strconv.ParseFloat(string(digits), 64); err == nil && len(digits) > 0 {
			if neg {
				f = -f
			}
			return Real(f)
		}
		return Int(0)
	}
	// Garbage glued to the integer ("12abc"): skip it.
	if p.pos < len(b) && isRegular(b[p.pos]) {
		for p.pos < len(b) && isRegular(b[p.pos]) {
			p.pos++
		}
		if neg {
			n = -n
		}
		return intObject(n)
	}
	if !signed && n <= math.MaxUint32 {
		// Look ahead for "gen R".
		save := p.pos
		p.skip()
		at := p.pos
		if gen, ok := p.uint(); ok {
			end := p.pos
			if gen <= math.MaxUint16 {
				p.skip()
				if p.pos < len(b) && b[p.pos] == 'R' && (p.pos+1 == len(b) || !isRegular(b[p.pos+1])) {
					p.pos++
					return Ref{Num: uint32(n), Gen: uint16(gen)}
				}
			}
			p.aheadAt, p.aheadEnd, p.ahead = at, end, gen
		}
		p.pos = save
	}
	if neg {
		n = -n
	}
	return intObject(n)
}

// intObject returns n as an Object. Integers below smallIntLimit, such as
// glyph widths and page coordinates, come boxed from smallInts: converting
// them to an interface would allocate.
func intObject(n int64) Object {
	if uint64(n) < smallIntLimit {
		return smallInts[n]
	}
	return Int(n)
}

// array reads an array.
func (p *parser) array() (Object, error) {
	p.pos++ // [
	if p.depth++; p.depth > maxDepth {
		return nil, errTooDeep
	}
	defer func() { p.depth-- }()
	arr := make(Array, 0, p.arrayLen())
	for {
		p.skip()
		if p.pos >= len(p.b) {
			return arr, nil // truncated: keep what was read
		}
		if p.b[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		save := p.pos
		o, err := p.object()
		if err == errTooDeep {
			return nil, err
		}
		if err != nil {
			continue
		}
		if k, ok := o.(keyword); ok {
			switch k {
			case "endobj", "stream", "endstream", "obj", ">":
				// Unterminated array: stop before the keyword.
				p.pos = save
				return arr, nil
			}
			o = nil // unknown keyword read as null
		}
		arr = append(arr, o)
	}
}

// skipArray moves past an array, after "[", where array would stop, without
// building it. Numbers, names and keywords are runs of regular characters,
// which array reads whole too; strings and dictionaries are read and
// dropped.
func (p *parser) skipArray() error {
	p.pos++ // [
	if p.depth++; p.depth > maxDepth {
		return errTooDeep
	}
	defer func() { p.depth-- }()
	b := p.b
	for {
		// White space and unsigned numbers, with what is glued to them as
		// number reads it, make the bulk of the arrays skipped.
		for p.pos < len(b) {
			c := b[p.pos]
			if class[c] == white {
				p.pos++
				continue
			}
			if !isDigit(c) {
				break
			}
			for p.pos++; p.pos < len(b) && isRegular(b[p.pos]); p.pos++ {
			}
		}
		if p.pos >= len(b) {
			return nil // truncated
		}
		start := p.pos
		switch c := b[p.pos]; {
		case c == '%':
			p.skip()
		case c == ']':
			p.pos++
			return nil
		case c == '>':
			return nil // unterminated array: array stops before ">"
		case c == '[':
			if err := p.skipArray(); err != nil {
				return err
			}
		case c == '(' || c == '<':
			if _, err := p.object(); err == errTooDeep {
				return err
			}
		case c == '/':
			p.pos++
			p.word()
		case class[c] == delim: // ")", "{" or "}": read as null
			p.pos++
		default:
			switch string(p.word()) {
			case "endobj", "stream", "endstream", "obj":
				// Unterminated array: stop before the keyword.
				p.pos = start
				return nil
			}
		}
	}
}

// maxArrayLen bounds the capacity arrayLen gives an array before reading it.
const maxArrayLen = 1 << 16

// arrayLen returns the capacity to give the array that starts at the current
// position, after "[": the number of its elements when they are numbers,
// names, references, booleans or null, as in the arrays of glyph widths,
// which have hundreds of elements and would otherwise grow step by step.
// Counting stops at a string, a nested array or dictionary, a comment or an
// unknown keyword, with a small margin for what follows.
func (p *parser) arrayLen() int {
	b := p.b
	n := 0
	for i := p.pos; i < len(b) && n < maxArrayLen; {
		c := b[i]
		switch {
		case class[c] == white:
			i++
			continue
		case c == ']':
			return n
		case c == '/':
			i++
		case isDigit(c) || c == '+' || c == '-' || c == '.':
		case c == 'R':
			// "num gen R" is one element: num and gen were counted.
			if i+1 == len(b) || !isRegular(b[i+1]) {
				n = max(n-1, 0)
				i++
				continue
			}
			return n + 4
		case c == 't' || c == 'f' || c == 'n': // true, false, null
		default:
			return n + 4
		}
		for i < len(b) && isRegular(b[i]) {
			i++
		}
		n++
	}
	return min(n, maxArrayLen)
}

// dict reads a dictionary. The entries are collected in the scratch space
// of the parser, then copied to a dictionary of their number.
func (p *parser) dict() (Object, error) {
	p.pos += 2 // <<
	if p.depth++; p.depth > maxDepth {
		return nil, errTooDeep
	}
	kvs := p.scratch()
	base := len(*kvs)
	defer func() {
		p.depth--
		clear((*kvs)[base:])
		*kvs = (*kvs)[:base]
	}()
	done := func() Dict {
		d := make(Dict, len(*kvs)-base)
		copy(d, (*kvs)[base:])
		return d
	}
	var skippedKeys [4]Name
	skipped := skippedKeys[:0] // keys whose array was skipped
	for {
		p.skip()
		if p.pos >= len(p.b) {
			return done(), nil
		}
		c := p.b[p.pos]
		if c == '>' {
			if p.pos+1 < len(p.b) && p.b[p.pos+1] == '>' {
				p.pos += 2
			} else {
				p.pos++
			}
			return done(), nil
		}
		if c != '/' {
			// Not a key: skip one object (tolerance) or stop on a keyword.
			save := p.pos
			o, err := p.object()
			if err == errTooDeep {
				return nil, err
			}
			if k, ok := o.(keyword); ok && (k == "endobj" || k == "stream" || k == "endstream" || k == "obj") {
				p.pos = save
				return done(), nil
			}
			continue
		}
		p.pos++
		key := p.name()
		p.skip()
		if p.pos < len(p.b) && p.b[p.pos] == '>' {
			// Key without value.
			continue
		}
		if p.sel != nil && p.depth == 1 && p.pos < len(p.b) && p.b[p.pos] == '[' && !slices.Contains(p.sel.keep, key) {
			if err := p.skipArray(); err != nil {
				return nil, err
			}
			skipped = append(skipped, key) // the first of duplicate keys wins
			continue
		}
		save := p.pos
		val, err := p.object()
		if err == errTooDeep {
			return nil, err
		}
		if err != nil {
			val = nil
		}
		if k, ok := val.(keyword); ok {
			switch k {
			case "endobj", "stream", "endstream", "obj":
				p.pos = save
				return done(), nil
			}
			val = nil
		}
		// A null value is equivalent to an absent entry; the first of
		// duplicate keys wins.
		if val != nil && !Dict((*kvs)[base:]).Has(key) && !slices.Contains(skipped, key) {
			*kvs = append(*kvs, KV{key, val})
		}
	}
}
