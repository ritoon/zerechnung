package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"math"
)

// Errors returned by Open.
var (
	// ErrEncrypted is returned for encrypted documents. PDF/A forbids
	// encryption, so a Factur-X PDF is never encrypted.
	ErrEncrypted = errors.New("pdf: the document is encrypted (PDF/A forbids encryption)")
	// ErrNotPDF is returned when no PDF header is found.
	ErrNotPDF = errors.New("pdf: not a PDF file (no %PDF- header)")
	// ErrNoCatalog is returned when the document catalog cannot be found,
	// even after rebuilding the cross-reference data.
	ErrNoCatalog = errors.New("pdf: document catalog not found")
)

// Reader reads objects of a PDF held in memory. It is not safe for
// concurrent use.
type Reader struct {
	data      []byte
	base      int    // offset of the %PDF- header
	version   string // header version ("1.7")
	startxref int64  // offset of the newest cross-reference section, -1 if unknown

	sections []*section // newest first
	trailer  Dict
	rec      *recovery // index rebuilt by scanning, built on demand
	recOnly  bool      // the cross-reference data is unusable: rec is authoritative
	repaired bool      // some object was not where the cross-reference data says

	cache map[uint32]Object
	stms  map[uint32]*objStm
	depth int
	kvs   []KV // scratch space of the parsers (see parser.scratch)
}

// entry is a cross-reference entry.
type entry struct {
	typ uint8  // 0 free, 1 in use at off, 2 compressed in object stream off
	gen uint16 // generation (type 1)
	off int64  // byte offset (type 1) or object stream number (type 2)
	idx uint32 // index in the object stream (type 2)
}

// section is one cross-reference section: a classic table or a stream.
type section struct {
	off     int64
	stream  bool
	subs    []sub            // classic table subsections
	lenient map[uint32]entry // classic table read entry by entry
	xs      *xrefStream
	hybrid  *section // /XRefStm of a hybrid file, read after the table
	trailer Dict
}

type sub struct {
	first, count uint32
	off, width   int
}

type xrefStream struct {
	data   []byte
	w      [3]int
	index  []uint32 // first, count pairs
	rowLen int
}

// Open parses the header, the trailer and the cross-reference data of a PDF.
// Objects are read on demand. It returns ErrEncrypted for encrypted files.
func Open(data []byte) (*Reader, error) {
	r := &Reader{data: data, startxref: -1}
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	i := bytes.Index(head, []byte("%PDF-"))
	if i < 0 {
		return nil, ErrNotPDF
	}
	r.base = i
	v := data[i+5:]
	n := 0
	for n < len(v) && n < 8 && (isDigit(v[n]) || v[n] == '.') {
		n++
	}
	r.version = string(v[:n])

	if err := r.loadXref(); err != nil || r.trailer.Get("Root") == nil {
		r.rebuild()
	}
	if r.trailer.Has("Encrypt") {
		return nil, ErrEncrypted
	}
	if _, ok := r.Resolve(r.trailer.Get("Root")).(Dict); !ok {
		if !r.recOnly {
			r.rebuild()
			if r.trailer.Has("Encrypt") {
				return nil, ErrEncrypted
			}
		}
		if _, ok := r.Resolve(r.trailer.Get("Root")).(Dict); !ok {
			return nil, ErrNoCatalog
		}
	}
	return r, nil
}

// Data returns the bytes the reader was opened on.
func (r *Reader) Data() []byte { return r.data }

// Version returns the version in the file header, such as "1.7".
func (r *Reader) Version() string { return r.version }

// Trailer returns the trailer dictionary (the newest one, completed with the
// Root, Info and ID entries of older ones when missing).
func (r *Reader) Trailer() Dict { return r.trailer }

// NewestTrailer returns the newest trailer dictionary as written, without
// the entries of older ones: the trailer of the last incremental update, or
// the first-page trailer of a linearized file.
func (r *Reader) NewestTrailer() Dict {
	if len(r.sections) == 0 {
		return r.trailer
	}
	return r.sections[0].trailer
}

// Catalog returns the document catalog.
func (r *Reader) Catalog() Dict {
	d, _ := r.Resolve(r.trailer.Get("Root")).(Dict)
	return d
}

// Repaired reports whether the cross-reference data was found damaged:
// rebuilt by scanning, or pointing to wrong offsets.
func (r *Reader) Repaired() bool { return r.recOnly || r.repaired }

// StartXref returns the offset of the newest cross-reference section, or -1
// when the cross-reference data had to be rebuilt.
func (r *Reader) StartXref() int64 {
	if r.recOnly {
		return -1
	}
	return r.startxref
}

// XrefStream reports whether the newest cross-reference section is a
// cross-reference stream.
func (r *Reader) XrefStream() bool {
	return len(r.sections) > 0 && r.sections[0].stream
}

// Resolve follows indirect references; other objects are returned as is.
// A reference to a missing object resolves to nil (the null object).
func (r *Reader) Resolve(o Object) Object {
	for i := 0; i < 32; i++ {
		ref, ok := o.(Ref)
		if !ok {
			return o
		}
		o = r.Object(ref.Num)
	}
	return nil
}

// Dict resolves o and returns it as a dictionary (a stream yields its
// dictionary).
func (r *Reader) Dict(o Object) (Dict, bool) {
	return AsDict(r.Resolve(o))
}

// Object returns the object with the given number, or nil when it does not
// exist or cannot be read.
func (r *Reader) Object(num uint32) Object {
	if o, ok := r.cache[num]; ok {
		return o
	}
	if r.depth > 64 {
		return nil
	}
	r.depth++
	o := r.load(num, nil)
	r.depth--
	if r.cache == nil {
		r.cache = make(map[uint32]Object, 16)
	}
	r.cache[num] = o
	return o
}

// DictSkipArrays returns the dictionary o resolves to, as Dict does, but
// without the arrays of its entries other than keep, which are skipped
// rather than read: a font dictionary is read without its glyph widths,
// hundreds of numbers that a reader of the font descriptor does not need.
// Only the arrays of the outermost dictionary are left out. A dictionary
// read that way is not cached; one already read in full is returned whole.
func (r *Reader) DictSkipArrays(o Object, keep ...Name) (Dict, bool) {
	ref, ok := o.(Ref)
	if !ok {
		return r.Dict(o)
	}
	if _, cached := r.cache[ref.Num]; cached || r.depth > 64 {
		return r.Dict(o)
	}
	r.depth++
	v := r.load(ref.Num, &selection{keep: keep})
	r.depth--
	return r.Dict(v)
}

// selection restricts what a parser reads (see DictSkipArrays).
type selection struct {
	keep []Name // keys whose arrays are read
}

// load reads object num; sel, when not nil, restricts what is read.
func (r *Reader) load(num uint32, sel *selection) Object {
	if !r.recOnly {
		e, found := r.lookup(num)
		if found {
			switch e.typ {
			case 0:
				return nil
			case 1:
				if o, err := r.parseAt(e.off, num, sel); err == nil {
					return o
				}
				if r.base > 0 {
					if o, err := r.parseAt(e.off+int64(r.base), num, sel); err == nil {
						r.repaired = true
						return o
					}
				}
			case 2:
				if e.off <= math.MaxUint32 {
					if o, err := r.fromStm(uint32(e.off), e.idx, num, sel); err == nil {
						return o
					}
				}
			}
		}
		// Not in the cross-reference data, or not where it should be:
		// fall back to the scanned index.
		r.repaired = true
	}
	rec := r.recovery()
	re, ok := rec.objs[num]
	if !ok {
		return nil
	}
	if re.inStm {
		o, _ := r.fromStm(re.stm, re.idx, num, sel)
		return o
	}
	o, _ := r.parseAt(re.off, num, sel)
	return o
}

// lookup finds the cross-reference entry of num, newest section first.
func (r *Reader) lookup(num uint32) (entry, bool) {
	for _, s := range r.sections {
		if e, ok := r.find(s, num); ok {
			return e, true
		}
	}
	return entry{}, false
}

func (r *Reader) find(s *section, num uint32) (entry, bool) {
	if s.stream {
		return s.xs.find(num)
	}
	// In a hybrid file, objects stored in object streams are usually free
	// entries of the table: a free entry defers to the /XRefStm stream.
	if s.lenient != nil {
		if e, ok := s.lenient[num]; ok && (e.typ != 0 || s.hybrid == nil) {
			return e, true
		}
	} else {
		for _, ss := range s.subs {
			if num >= ss.first && num-ss.first < ss.count {
				e, ok := readEntry(r.data, ss.off+int(num-ss.first)*ss.width)
				if !ok {
					// Irregular entries: read the table token by token.
					s.lenient = r.readTableLenient(s.off)
					e, ok = s.lenient[num]
					if !ok {
						break
					}
				}
				if e.typ == 0 && s.hybrid != nil {
					break
				}
				return e, true
			}
		}
	}
	if s.hybrid != nil {
		return r.find(s.hybrid, num)
	}
	return entry{}, false
}

// readEntry reads a 20-byte classic xref entry "oooooooooo ggggg n".
func readEntry(b []byte, at int) (entry, bool) {
	if at < 0 || at+18 > len(b) {
		return entry{}, false
	}
	e := b[at : at+18]
	var off int64
	for i := 0; i < 10; i++ {
		c := e[i]
		if !isDigit(c) {
			return entry{}, false
		}
		off = off*10 + int64(c-'0')
	}
	if e[10] != ' ' || e[16] != ' ' {
		return entry{}, false
	}
	var gen int
	for i := 11; i < 16; i++ {
		c := e[i]
		if !isDigit(c) {
			return entry{}, false
		}
		gen = gen*10 + int(c-'0')
	}
	switch e[17] {
	case 'n':
		if gen > math.MaxUint16 {
			gen = math.MaxUint16
		}
		return entry{typ: 1, gen: uint16(gen), off: off}, true
	case 'f':
		return entry{typ: 0}, true
	}
	return entry{}, false
}

func (x *xrefStream) find(num uint32) (entry, bool) {
	row := 0
	for i := 0; i+1 < len(x.index); i += 2 {
		first, count := x.index[i], x.index[i+1]
		if num >= first && num-first < count {
			row += int(num - first)
			at := row * x.rowLen
			if at+x.rowLen > len(x.data) {
				return entry{}, false
			}
			b := x.data[at:]
			field := func(n int) (v int64) {
				for k := 0; k < x.w[n]; k++ {
					v = v<<8 | int64(b[0])
					b = b[1:]
				}
				return v
			}
			typ := int64(1)
			if x.w[0] > 0 {
				typ = field(0)
			}
			f2, f3 := field(1), field(2)
			switch typ {
			case 0:
				return entry{typ: 0}, true
			case 1:
				if f3 > math.MaxUint16 {
					f3 = math.MaxUint16
				}
				return entry{typ: 1, off: f2, gen: uint16(f3)}, true
			case 2:
				if f3 > math.MaxUint32 {
					return entry{}, false
				}
				return entry{typ: 2, off: f2, idx: uint32(f3)}, true
			}
			return entry{typ: 0}, true // unknown types are null objects
		}
		row += int(count)
	}
	return entry{}, false
}

// loadXref reads the cross-reference chain from startxref.
func (r *Reader) loadXref() error {
	off, ok := r.findStartxref()
	if !ok {
		return errors.New("pdf: startxref not found")
	}
	r.startxref = off
	seen := map[int64]bool{}
	for len(r.sections) < 1024 {
		if seen[off] {
			break
		}
		seen[off] = true
		s, err := r.loadSection(off, false)
		if err != nil && r.base > 0 {
			if s, err = r.loadSection(off+int64(r.base), false); err == nil {
				r.repaired = true
			}
		}
		if err != nil {
			if len(r.sections) == 0 {
				return err
			}
			r.repaired = true
			break
		}
		r.sections = append(r.sections, s)
		if !s.stream {
			if x, ok := s.trailer.Int("XRefStm"); ok {
				if hs, err := r.loadSection(x, true); err == nil {
					s.hybrid = hs
				} else if r.base > 0 {
					if hs, err := r.loadSection(x+int64(r.base), true); err == nil {
						s.hybrid = hs
					}
				}
			}
		}
		prev, ok := s.trailer.Int("Prev")
		if !ok || prev < 0 {
			break
		}
		off = prev
	}
	// The newest trailer wins; older ones fill missing entries.
	t := r.sections[0].trailer.Clone()
	for _, s := range r.sections[1:] {
		for _, k := range []Name{"Root", "Info", "ID", "Encrypt"} {
			if !t.Has(k) && s.trailer.Has(k) {
				t.Set(k, s.trailer.Get(k))
			}
		}
	}
	r.trailer = t
	return nil
}

// findStartxref reads the offset after the last "startxref" keyword.
func (r *Reader) findStartxref() (int64, bool) {
	tail := r.data
	if len(tail) > 4096 {
		tail = tail[len(tail)-4096:]
	}
	i := bytes.LastIndex(tail, []byte("startxref"))
	if i < 0 {
		i = bytes.LastIndex(r.data, []byte("startxref"))
		if i < 0 {
			return 0, false
		}
		tail = r.data
	}
	p := parser{b: tail, pos: i + 9}
	p.skip()
	n, ok := p.uint()
	return n, ok
}

// loadSection reads the cross-reference section at off.
func (r *Reader) loadSection(off int64, streamOnly bool) (*section, error) {
	if off < 0 || off >= int64(len(r.data)) {
		return nil, fmt.Errorf("pdf: cross-reference offset %d out of range", off)
	}
	p := parser{b: r.data, pos: int(off), r: r}
	p.skip()
	if !streamOnly && bytes.HasPrefix(r.data[p.pos:], []byte("xref")) {
		return r.loadTable(off, p.pos+4)
	}
	o, err := r.parseAt(off, 0, nil)
	if err != nil {
		return nil, err
	}
	st, ok := o.(*Stream)
	if !ok {
		return nil, errors.New("pdf: no cross-reference section at offset")
	}
	if t, _ := st.Dict.Name("Type"); t != "XRef" && !st.Dict.Has("W") {
		return nil, errors.New("pdf: no cross-reference stream at offset")
	}
	xs, err := r.readXrefStream(st)
	if err != nil {
		return nil, err
	}
	return &section{off: off, stream: true, xs: xs, trailer: st.Dict}, nil
}

func (r *Reader) readXrefStream(st *Stream) (*xrefStream, error) {
	w, ok := st.Dict.Get("W").(Array)
	if !ok || len(w) < 3 {
		return nil, errors.New("pdf: cross-reference stream without /W")
	}
	x := &xrefStream{}
	for i := 0; i < 3; i++ {
		n, ok := AsInt(w[i])
		if !ok || n < 0 || n > 8 {
			return nil, errors.New("pdf: bad /W in cross-reference stream")
		}
		x.w[i] = int(n)
		x.rowLen += int(n)
	}
	if x.rowLen == 0 {
		return nil, errors.New("pdf: bad /W in cross-reference stream")
	}
	if idx, ok := st.Dict.Get("Index").(Array); ok && len(idx) >= 2 {
		for _, v := range idx {
			n, ok := AsInt(v)
			if !ok || n < 0 || n > math.MaxUint32 {
				return nil, errors.New("pdf: bad /Index in cross-reference stream")
			}
			x.index = append(x.index, uint32(n))
		}
	} else {
		size, _ := st.Dict.Int("Size")
		if size < 0 || size > math.MaxUint32 {
			size = 0
		}
		x.index = []uint32{0, uint32(size)}
	}
	data, err := r.decode(st, 0, nil)
	if err != nil && len(data) == 0 {
		return nil, err
	}
	x.data = data
	return x, nil
}

// loadTable reads a classic cross-reference table; pos is just after the
// "xref" keyword.
func (r *Reader) loadTable(off int64, pos int) (*section, error) {
	b := r.data
	p := parser{b: b, pos: pos, r: r}
	s := &section{off: off}
	for {
		p.skip()
		if p.pos >= len(b) {
			return nil, errEOF
		}
		if bytes.HasPrefix(b[p.pos:], []byte("trailer")) {
			p.pos += 7
			break
		}
		first, ok1 := p.uint()
		for p.pos < len(b) && (b[p.pos] == ' ' || b[p.pos] == '\t') {
			p.pos++
		}
		count, ok2 := p.uint()
		if !ok1 || !ok2 || first > math.MaxUint32 || count > math.MaxUint32 {
			return r.lenientSection(off)
		}
		p.skip()
		start := p.pos
		if count == 0 {
			continue
		}
		width := 0
		if start+20 <= len(b) {
			switch {
			case (b[start+18] == ' ' || b[start+18] == '\r') && (b[start+19] == '\n' || b[start+19] == '\r'):
				width = 20
			case b[start+18] == '\n' || b[start+18] == '\r':
				width = 19
			}
		}
		if width == 0 {
			return r.lenientSection(off)
		}
		e, ok := readEntry(b, start)
		if !ok {
			return r.lenientSection(off)
		}
		// A table starting at 1 whose first entry is the head of the free
		// list was meant to start at 0.
		if first == 1 && e.typ == 0 && bytes.HasPrefix(b[start:], []byte("0000000000 65535 f")) {
			first = 0
		}
		end := start + int(count)*width
		if end > len(b) || end < start {
			return r.lenientSection(off)
		}
		if _, ok := readEntry(b, end-width); !ok {
			return r.lenientSection(off)
		}
		s.subs = append(s.subs, sub{first: uint32(first), count: uint32(count), off: start, width: width})
		p.pos = end
	}
	t, err := p.object()
	if err != nil {
		return nil, err
	}
	d, ok := t.(Dict)
	if !ok {
		return nil, errors.New("pdf: no trailer dictionary")
	}
	s.trailer = d
	return s, nil
}

// lenientSection reads a damaged classic table entry by entry.
func (r *Reader) lenientSection(off int64) (*section, error) {
	s := &section{off: off}
	s.lenient = r.readTableLenient(off)
	// Find the trailer after the table.
	i := bytes.Index(r.data[off:], []byte("trailer"))
	if i < 0 {
		return nil, errors.New("pdf: no trailer after the cross-reference table")
	}
	p := parser{b: r.data, pos: int(off) + i + 7, r: r}
	t, err := p.object()
	if err != nil {
		return nil, err
	}
	d, ok := t.(Dict)
	if !ok {
		return nil, errors.New("pdf: no trailer dictionary")
	}
	s.trailer = d
	return s, nil
}

// readTableLenient reads the entries of a classic table token by token.
func (r *Reader) readTableLenient(off int64) map[uint32]entry {
	m := map[uint32]entry{}
	p := parser{b: r.data, pos: int(off), r: r}
	p.skip()
	if !bytes.HasPrefix(r.data[p.pos:], []byte("xref")) {
		return m
	}
	p.pos += 4
	for {
		p.skip()
		if p.pos >= len(p.b) || bytes.HasPrefix(p.b[p.pos:], []byte("trailer")) {
			return m
		}
		first, ok1 := p.uint()
		p.skip()
		count, ok2 := p.uint()
		if !ok1 || !ok2 {
			return m
		}
		for i := int64(0); i < count; i++ {
			p.skip()
			o, ok1 := p.uint()
			p.skip()
			g, ok2 := p.uint()
			p.skip()
			if !ok1 || !ok2 || p.pos >= len(p.b) {
				return m
			}
			t := p.b[p.pos]
			p.pos++
			num := first + i
			if num < 0 || num > math.MaxUint32 {
				continue
			}
			if _, dup := m[uint32(num)]; dup {
				continue
			}
			switch t {
			case 'n':
				if g > math.MaxUint16 {
					g = math.MaxUint16
				}
				m[uint32(num)] = entry{typ: 1, off: o, gen: uint16(g)}
			case 'f':
				m[uint32(num)] = entry{typ: 0}
			default:
				return m
			}
		}
	}
}

// parseAt reads the indirect object at off. When num is not 0, the object
// header must carry that number. sel, when not nil, restricts what is read.
func (r *Reader) parseAt(off int64, num uint32, sel *selection) (Object, error) {
	if off < 0 || off >= int64(len(r.data)) {
		return nil, errNotObject
	}
	p := parser{b: r.data, pos: int(off), r: r, sel: sel}
	p.skip()
	n, ok := p.uint()
	if !ok {
		return nil, errNotObject
	}
	p.skip()
	if _, ok := p.uint(); !ok {
		return nil, errNotObject
	}
	p.skip()
	if !bytes.HasPrefix(r.data[p.pos:], []byte("obj")) {
		return nil, errNotObject
	}
	if num != 0 && n != int64(num) {
		return nil, errNotObject
	}
	p.pos += 3
	o, err := p.object()
	if err != nil {
		return nil, err
	}
	if _, ok := o.(keyword); ok {
		return nil, nil // "n g obj endobj": null
	}
	if d, ok := o.(Dict); ok {
		p.skip()
		if bytes.HasPrefix(r.data[p.pos:], []byte("stream")) {
			return r.stream(&p, d), nil
		}
	}
	return o, nil
}

// stream reads the data of a stream whose dictionary d has just been parsed;
// p is on the "stream" keyword.
func (r *Reader) stream(p *parser, d Dict) *Stream {
	length := int64(-1)
	if n, ok := AsInt(r.Resolve(d.Get("Length"))); ok {
		length = n
	}
	start, end := streamExtent(p.b, p.pos, length)
	return &Stream{Dict: d, Raw: p.b[start:end:end]}
}

// streamExtent returns the bounds of the data of a stream; pos is on the
// "stream" keyword and length is the value of /Length (-1 when unknown).
// A wrong length is replaced by a search for "endstream".
func streamExtent(b []byte, pos int, length int64) (start, end int) {
	pos += 6
	// End of line after "stream": CRLF or LF; tolerate a lone CR and
	// spaces before it.
	k := pos
	for k < len(b) && (b[k] == ' ' || b[k] == '\t') {
		k++
	}
	if k < len(b) && (b[k] == '\r' || b[k] == '\n') {
		pos = k
	}
	if pos < len(b) && b[pos] == '\r' {
		pos++
	}
	if pos < len(b) && b[pos] == '\n' {
		pos++
	}
	start = pos
	if length >= 0 && int64(start)+length <= int64(len(b)) {
		end = start + int(length)
		if endstreamAt(b, end) {
			return start, end
		}
	}
	i := bytes.Index(b[start:], []byte("endstream"))
	if i < 0 {
		return start, len(b)
	}
	end = start + i
	if end > start && b[end-1] == '\n' {
		end--
	}
	if end > start && b[end-1] == '\r' {
		end--
	}
	return start, end
}

// endstreamAt reports whether "endstream" follows offset i, after optional
// white space.
func endstreamAt(b []byte, i int) bool {
	for k := 0; k < 4 && i < len(b) && class[b[i]] == white; k++ {
		i++
	}
	return bytes.HasPrefix(b[i:], []byte("endstream"))
}

// objStm is a decoded object stream.
type objStm struct {
	data  []byte
	first int
	nums  []uint32
	offs  []int
}

func (r *Reader) objStm(num uint32) *objStm {
	if s, ok := r.stms[num]; ok {
		return s
	}
	if r.stms == nil {
		r.stms = map[uint32]*objStm{}
	}
	r.stms[num] = nil // guard against recursion
	st, ok := r.Object(num).(*Stream)
	if !ok {
		return nil
	}
	os := r.decodeObjStm(st)
	r.stms[num] = os
	return os
}

func (r *Reader) decodeObjStm(st *Stream) *objStm {
	data, err := r.decode(st, 0, nil)
	if err != nil && len(data) == 0 {
		return nil
	}
	n, _ := st.Dict.Int("N")
	first, _ := st.Dict.Int("First")
	if n < 0 || first < 0 || first > int64(len(data)) {
		return nil
	}
	os := &objStm{data: data, first: int(first)}
	p := parser{b: data[:first], r: r}
	for i := int64(0); i < n; i++ {
		p.skip()
		num, ok1 := p.uint()
		p.skip()
		off, ok2 := p.uint()
		if !ok1 || !ok2 || num > math.MaxUint32 || first+off > int64(len(data)) {
			break
		}
		os.nums = append(os.nums, uint32(num))
		os.offs = append(os.offs, int(off))
	}
	return os
}

// fromStm reads object num, stored at index idx of object stream stm.
func (r *Reader) fromStm(stm, idx, num uint32, sel *selection) (Object, error) {
	os := r.objStm(stm)
	if os == nil {
		return nil, errNotObject
	}
	i := int(idx)
	if i >= len(os.nums) || os.nums[i] != num {
		i = -1
		for k, n := range os.nums {
			if n == num {
				i = k
				break
			}
		}
		if i < 0 {
			return nil, errNotObject
		}
	}
	p := parser{b: os.data, pos: os.first + os.offs[i], r: r, sel: sel}
	o, err := p.object()
	if err != nil {
		return nil, err
	}
	if _, ok := o.(keyword); ok {
		return nil, nil
	}
	return o, nil
}
