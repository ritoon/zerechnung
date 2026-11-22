package pdf

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Loc locates an object in use: at a byte offset of the file, or in an
// object stream.
type Loc struct {
	Num uint32
	Gen uint16
	Off int64  // offset of "num gen obj"; -1 for an object of an object stream
	Stm uint32 // object stream of a compressed object
	Idx uint32 // index in the object stream
}

// InUse returns the objects in use, as the newest cross-reference entries
// locate them (or the index rebuilt by scanning a damaged file), in
// increasing number order. At most max objects are returned; more reports
// whether others were left out. total is the number of objects in use, or
// a lower bound when more is true.
func (r *Reader) InUse(max int) (locs []Loc, total int, more bool) {
	if r.recOnly {
		rec := r.recovery()
		for num, e := range rec.objs {
			if len(locs) >= max {
				more = true
				break
			}
			if e.inStm {
				locs = append(locs, Loc{Num: num, Off: -1, Stm: e.stm, Idx: e.idx})
			} else {
				locs = append(locs, Loc{Num: num, Off: e.off})
			}
		}
		sort.Slice(locs, func(i, j int) bool { return locs[i].Num < locs[j].Num })
		return locs, len(rec.objs), more
	}
	nums := map[uint32]struct{}{}
	add := func(first, count uint32) {
		for n := uint64(first); n < uint64(first)+uint64(count) && n <= math.MaxUint32; n++ {
			if len(nums) > max {
				more = true
				return
			}
			nums[uint32(n)] = struct{}{}
		}
	}
	var collect func(s *section)
	collect = func(s *section) {
		switch {
		case s.stream:
			for i := 0; i+1 < len(s.xs.index); i += 2 {
				add(s.xs.index[i], s.xs.index[i+1])
			}
		case s.lenient != nil:
			for n := range s.lenient {
				add(n, 1)
			}
		default:
			for _, ss := range s.subs {
				add(ss.first, ss.count)
			}
		}
		if s.hybrid != nil {
			collect(s.hybrid)
		}
	}
	for _, s := range r.sections {
		collect(s)
	}
	for num := range nums {
		e, ok := r.lookup(num)
		if !ok {
			continue
		}
		switch e.typ {
		case 1:
			locs = append(locs, Loc{Num: num, Gen: e.gen, Off: e.off})
		case 2:
			if e.off <= math.MaxUint32 {
				locs = append(locs, Loc{Num: num, Off: -1, Stm: uint32(e.off), Idx: e.idx})
			}
		}
	}
	sort.Slice(locs, func(i, j int) bool { return locs[i].Num < locs[j].Num })
	total = len(locs)
	if len(locs) > max {
		locs, more = locs[:max], true
	}
	return locs, total, more
}

// XrefTables returns the offsets of the classic cross-reference tables of
// the chain, newest first. It returns nil when the cross-reference data had
// to be rebuilt.
func (r *Reader) XrefTables() []int64 {
	if r.recOnly {
		return nil
	}
	var offs []int64
	for _, s := range r.sections {
		if !s.stream {
			offs = append(offs, s.off)
		}
	}
	return offs
}

// SyntaxKind is a kind of deviation from the strict syntax that PDF/A
// requires (ISO 19005-2 and 19005-3, 6.1).
type SyntaxKind uint8

// Kinds of syntax deviations.
const (
	// SyntaxHex: a hexadecimal string has an odd number of digits or a
	// character other than a hexadecimal digit and white space (6.1.6).
	SyntaxHex SyntaxKind = iota + 1
	// SyntaxObject: "num gen obj" or endobj does not follow the spacing of
	// 6.1.9: single white-space characters inside the header, an EOL marker
	// before the object number and endobj and after obj and endobj.
	SyntaxObject
	// SyntaxStream: the stream keyword is not followed by CRLF or LF, the
	// endstream keyword is not preceded by an EOL marker, or /Length does
	// not match the data (6.1.7.1).
	SyntaxStream
	// SyntaxLimit: an integer, real number, string or name exceeds an
	// implementation limit of 6.1.13.
	SyntaxLimit
)

// SyntaxIssue is a deviation of an object from the strict syntax.
type SyntaxIssue struct {
	Kind   SyntaxKind
	Num    uint32 // object number
	Detail string
}

// Implementation limits of ISO 19005-2 and 19005-3, 6.1.13.
const (
	maxStringLen = 32767
	maxNameLen   = 127
	maxReal      = 3.403e38
	minReal      = 1.175e-38
)

// Syntax checks the strict syntax of the object at loc and calls fn for
// each deviation: the spacing of the object header and of endobj, the
// stream keywords and length, the hexadecimal strings and the limits on
// numbers, strings and names. The data of streams (content streams among
// them) is not read. Objects of object streams are checked for their
// strings, names and numbers only.
func (r *Reader) Syntax(loc Loc, fn func(SyntaxIssue)) {
	l := lint{num: loc.Num, fn: fn}
	if loc.Off < 0 {
		os := r.objStm(loc.Stm)
		if os == nil {
			return
		}
		i := int(loc.Idx)
		if i >= len(os.nums) || os.nums[i] != loc.Num {
			i = -1
			for k, n := range os.nums {
				if n == loc.Num {
					i = k
				}
			}
			if i < 0 {
				return
			}
		}
		start := os.first + os.offs[i]
		end := len(os.data)
		for _, o := range os.offs {
			if at := os.first + o; at > start && at < end {
				end = at
			}
		}
		if start > end {
			return
		}
		l.b, l.pos, l.end = os.data[:end], start, end
		l.scan()
		return
	}

	b := r.data
	off := loc.Off
	if !objectHeaderAt(b, off, loc.Num) && r.base > 0 && objectHeaderAt(b, off+int64(r.base), loc.Num) {
		off += int64(r.base)
	}
	if off < 0 || off >= int64(len(b)) {
		return
	}
	l.b, l.pos, l.end = b, int(off), len(b)
	l.header()
	kw, at := l.scan()
	if kw == "stream" {
		var length int64 = -1
		if st, ok := r.Object(loc.Num).(*Stream); ok {
			if n, ok := AsInt(r.Resolve(st.Dict.Get("Length"))); ok {
				length = n
			}
		}
		l.stream(at, length)
		kw, at = l.scan()
	}
	if kw != "endobj" {
		l.report(SyntaxObject, "no endobj keyword after the object")
		return
	}
	if !isEOL(b, at-1) {
		l.report(SyntaxObject, "the endobj keyword is not preceded by an end-of-line marker")
	}
	if at+6 < len(b) && !isEOL(b, at+6) {
		l.report(SyntaxObject, "the endobj keyword is not followed by an end-of-line marker")
	}
}

// objectHeaderAt reports whether "num gen obj" starts at off.
func objectHeaderAt(b []byte, off int64, num uint32) bool {
	if off < 0 || off >= int64(len(b)) {
		return false
	}
	p := parser{b: b, pos: int(off)}
	n, ok := p.uint()
	return ok && n == int64(num)
}

func isEOL(b []byte, i int) bool {
	return i >= 0 && i < len(b) && (b[i] == '\n' || b[i] == '\r')
}

// lint scans the tokens of an object in b[pos:end].
type lint struct {
	b        []byte
	pos, end int
	num      uint32
	fn       func(SyntaxIssue)
}

func (l *lint) report(k SyntaxKind, format string, args ...any) {
	l.fn(SyntaxIssue{Kind: k, Num: l.num, Detail: fmt.Sprintf(format, args...)})
}

// header checks "num gen obj" at l.pos and leaves l.pos after obj.
func (l *lint) header() {
	b := l.b
	start := l.pos
	// A cross-reference offset may point at the white space before the
	// object number: the offsets are not a PDF/A requirement.
	for start < len(b) && class[b[start]] == white {
		start++
	}
	ok := start == 0 || isEOL(b, start-1)
	i := start
	digits := func() bool {
		k := i
		for i < len(b) && isDigit(b[i]) {
			i++
		}
		return i > k
	}
	single := func() bool {
		if i < len(b) && class[b[i]] == white {
			i++
			return i >= len(b) || class[b[i]] != white
		}
		return false
	}
	ok = digits() && single() && digits() && single() && ok
	if !bytes.HasPrefix(b[i:], []byte("obj")) {
		// Find the keyword to go on.
		k := bytes.Index(b[i:min(len(b), i+64)], []byte("obj"))
		if k < 0 {
			l.report(SyntaxObject, "no obj keyword in the object header")
			return
		}
		i += k
		ok = false
	}
	i += 3
	if !isEOL(b, i) {
		ok = false
	}
	if !ok {
		l.report(SyntaxObject, "the object header %q does not follow the PDF/A spacing (end-of-line marker, number, single space, generation, single space, obj, end-of-line marker)", headerText(b, start, i))
	}
	l.pos = i
}

func headerText(b []byte, start, end int) string {
	if end > start+40 {
		end = start + 40
	}
	return string(b[start:end])
}

// stream checks the stream keyword at at, the data length and the
// endstream keyword, and leaves l.pos after endstream.
func (l *lint) stream(at int, length int64) {
	b := l.b
	i := at + 6
	switch {
	case i < len(b) && b[i] == '\n':
		i++
	case i+1 < len(b) && b[i] == '\r' && b[i+1] == '\n':
		i += 2
	default:
		l.report(SyntaxStream, "the stream keyword is not followed by CRLF or LF")
		for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
			i++
		}
	}
	start := i
	// The data ends where /Length says, before an end-of-line marker and
	// endstream; otherwise at the first endstream.
	if length >= 0 && int64(start)+length < int64(len(b)) {
		e := start + int(length)
		eol := 0
		switch {
		case bytes.HasPrefix(b[e:], []byte("\r\nendstream")):
			eol = 2
		case bytes.HasPrefix(b[e:], []byte("\nendstream")), bytes.HasPrefix(b[e:], []byte("\rendstream")):
			eol = 1
		}
		if eol > 0 {
			l.pos = e + eol + 9
			return
		}
	}
	k := bytes.Index(b[start:], []byte("endstream"))
	if k < 0 {
		l.report(SyntaxStream, "no endstream keyword after the stream data")
		l.pos = len(b)
		return
	}
	e := start + k
	eol := 0
	switch {
	case e-2 >= start && b[e-2] == '\r' && b[e-1] == '\n':
		eol = 2
	case e-1 >= start && (b[e-1] == '\n' || b[e-1] == '\r'):
		eol = 1
	default:
		l.report(SyntaxStream, "the endstream keyword is not preceded by an end-of-line marker")
	}
	if real := int64(e - eol - start); length != real {
		if length < 0 {
			l.report(SyntaxStream, "the stream has no valid /Length; its data is %d bytes long", real)
		} else {
			l.report(SyntaxStream, "/Length is %d, but the stream data is %d bytes long", length, real)
		}
	}
	l.pos = e + 9
}

// scan reads tokens up to a keyword other than true, false, null and R,
// and returns it with its offset; "" at the end of the data.
func (l *lint) scan() (kw string, at int) {
	b := l.b
	for l.pos < l.end {
		c := b[l.pos]
		switch {
		case class[c] == white:
			l.pos++
		case c == '%':
			for l.pos < l.end && b[l.pos] != '\n' && b[l.pos] != '\r' {
				l.pos++
			}
		case c == '/':
			l.name()
		case c == '(':
			l.literal()
		case c == '<':
			if l.pos+1 < l.end && b[l.pos+1] == '<' {
				l.pos += 2
			} else {
				l.hex()
			}
		case c == '>' || c == '[' || c == ']' || c == '{' || c == '}' || c == ')':
			l.pos++
		case isDigit(c) || c == '+' || c == '-' || c == '.':
			l.number()
		default:
			start := l.pos
			for l.pos < l.end && isRegular(b[l.pos]) {
				l.pos++
			}
			if l.pos == start {
				l.pos++
				continue
			}
			switch w := b[start:l.pos]; string(w) {
			case "true", "false", "null", "R":
			default:
				if k, ok := keywords[string(w)]; ok {
					return string(k.(keyword)), start // no allocation for the known keywords
				}
				return string(w), start
			}
		}
	}
	return "", l.pos
}

// name checks the length of a name, #xx escapes counting as one byte.
func (l *lint) name() {
	b := l.b
	l.pos++
	n := 0
	for l.pos < l.end && isRegular(b[l.pos]) {
		if b[l.pos] == '#' && l.pos+2 < l.end && isHex(b[l.pos+1]) && isHex(b[l.pos+2]) {
			l.pos += 2
		}
		l.pos++
		n++
	}
	if n > maxNameLen {
		l.report(SyntaxLimit, "a name is %d bytes long; the limit is %d", n, maxNameLen)
	}
}

// literal checks the decoded length of a literal string.
func (l *lint) literal() {
	b := l.b
	l.pos++
	depth, n := 1, 0
	for l.pos < l.end && depth > 0 {
		c := b[l.pos]
		l.pos++
		switch c {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				continue
			}
		case '\\':
			if l.pos >= l.end {
				break
			}
			e := b[l.pos]
			l.pos++
			switch {
			case e >= '0' && e <= '7':
				for k := 0; k < 2 && l.pos < l.end && b[l.pos] >= '0' && b[l.pos] <= '7'; k++ {
					l.pos++
				}
			case e == '\r':
				if l.pos < l.end && b[l.pos] == '\n' {
					l.pos++
				}
				continue // line continuation: no byte
			case e == '\n':
				continue
			}
		}
		n++
	}
	if n > maxStringLen {
		l.report(SyntaxLimit, "a string is %d bytes long; the limit is %d", n, maxStringLen)
	}
}

// hex checks a hexadecimal string.
func (l *lint) hex() {
	b := l.b
	l.pos++
	digits, bad := 0, false
	for l.pos < l.end && b[l.pos] != '>' {
		switch c := b[l.pos]; {
		case isHex(c):
			digits++
		case class[c] == white:
		default:
			bad = true
		}
		l.pos++
	}
	l.pos++
	switch {
	case bad:
		l.report(SyntaxHex, "a hexadecimal string holds a character other than a hexadecimal digit or white space")
	case digits%2 != 0:
		l.report(SyntaxHex, "a hexadecimal string has an odd number of digits (%d)", digits)
	}
	if n := (digits + 1) / 2; n > maxStringLen {
		l.report(SyntaxLimit, "a string is %d bytes long; the limit is %d", n, maxStringLen)
	}
}

// number checks the range of an integer or a real number.
func (l *lint) number() {
	b := l.b
	start := l.pos
	for l.pos < l.end && isRegular(b[l.pos]) {
		l.pos++
	}
	raw := b[start:l.pos]
	real := bytes.IndexByte(raw, '.') >= 0
	// A short token is within the limits: a real number out of range has
	// at least 39 digits before the point or 38 after it, an
	// integer out of range at least 10 digits.
	if real && len(raw) <= 30 || !real && len(raw) <= 9 {
		return
	}
	tok := string(raw)
	if real {
		f, err := strconv.ParseFloat(tok, 64)
		if err != nil && f == 0 {
			return
		}
		if a := math.Abs(f); a > maxReal || (a != 0 && a < minReal) {
			l.report(SyntaxLimit, "the real number %s is outside the range PDF/A admits (±3.403×10^38, no closer to zero than ±1.175×10^-38)", tok)
		}
		return
	}
	n, err := strconv.ParseInt(tok, 10, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return
		}
	}
	if err != nil || n > math.MaxInt32 || n < math.MinInt32 {
		l.report(SyntaxLimit, "the integer %s is outside the range PDF/A admits (-2147483648 to 2147483647)", tok)
	}
}
