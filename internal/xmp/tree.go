package xmp

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// node is an XML element of an XMP packet. Names and values are slices of
// the packet; offsets locate the element so that it can be edited in place.
type node struct {
	prefix, local []byte
	space         string
	attrs         []attr
	children      []*node
	parent        *node
	start         int // offset of '<'
	startEnd      int // offset just after the '>' of the start tag
	endStart      int // offset of the '<' of the end tag (startEnd when self-closing)
	end           int // offset just after the element
}

type attr struct {
	prefix, local []byte
	space         string
	value         []byte // raw value, entities not decoded
	start, end    int    // from the attribute name to the closing quote
}

func (n *node) is(space, local string) bool {
	return n != nil && n.space == space && string(n.local) == local
}

func (n *node) attr(space, local string) (string, bool) {
	for _, a := range n.attrs {
		if a.space == space && string(a.local) == local {
			return unescape(a.value), true
		}
	}
	return "", false
}

// child returns the first child element with the given name.
func (n *node) child(space, local string) *node {
	for _, c := range n.children {
		if c.is(space, local) {
			return c
		}
	}
	return nil
}

// text returns the decoded character data of a leaf element.
func (n *node) text(data []byte) string {
	if n.endStart <= n.startEnd {
		return ""
	}
	return decodeText(data[n.startEnd:n.endStart])
}

var errMalformed = errors.New("xmp: malformed XML")

type nsDecl struct {
	prefix []byte
	uri    string
	depth  int
}

// parser builds the element tree of an XMP packet. The nodes, their
// children and their attributes are cut from blocks shared by the packet,
// instead of slices grown element by element.
type parser struct {
	data    []byte
	ns      []nsDecl
	block   []node
	pending []*node // children of the open elements, those of the innermost last
	kids    []*node // block of the children slices
	attrBuf []attr  // attributes of the start tag being read
	attrs   []attr  // block of the attribute slices
}

// cut returns a copy of s, of capacity its length, from *block, which it
// renews if it is full.
func cut[T any](block *[]T, s []T) []T {
	if len(s) == 0 {
		return nil
	}
	if cap(*block)-len(*block) < len(s) {
		*block = make([]T, 0, max(64, len(s)))
	}
	n := len(*block)
	*block = append(*block, s...)
	return (*block)[n:len(*block):len(*block)]
}

func (p *parser) newNode() *node {
	if len(p.block) == cap(p.block) {
		p.block = make([]node, 0, 32)
	}
	p.block = p.block[:len(p.block)+1]
	return &p.block[len(p.block)-1]
}

func (p *parser) lookup(prefix []byte) string {
	for i := len(p.ns) - 1; i >= 0; i-- {
		if bytes.Equal(p.ns[i].prefix, prefix) {
			return p.ns[i].uri
		}
	}
	if string(prefix) == "xml" {
		return nsXML
	}
	return ""
}

// parse returns the root element of data.
func parse(data []byte) (*node, error) {
	// The blocks are sized for the packet: an element per start tag, an
	// attribute at most per '='.
	tags := bytes.Count(data, []byte("<")) - bytes.Count(data, []byte("</"))
	p := &parser{data: data, block: make([]node, 0, tags), kids: make([]*node, 0, tags),
		attrs: make([]attr, 0, bytes.Count(data, []byte("=")))}
	var stack []*node
	var first []int // index in p.pending of the first child of each element of stack
	var root *node
	i := 0
	for {
		k := bytes.IndexByte(data[i:], '<')
		if k < 0 {
			break
		}
		i += k
		rest := data[i:]
		switch {
		case bytes.HasPrefix(rest, []byte("<?")):
			j := bytes.Index(rest, []byte("?>"))
			if j < 0 {
				return nil, errMalformed
			}
			i += j + 2
		case bytes.HasPrefix(rest, []byte("<!--")):
			j := bytes.Index(rest[4:], []byte("-->"))
			if j < 0 {
				return nil, errMalformed
			}
			i += 4 + j + 3
		case bytes.HasPrefix(rest, []byte("<![CDATA[")):
			j := bytes.Index(rest, []byte("]]>"))
			if j < 0 {
				return nil, errMalformed
			}
			i += j + 3
		case bytes.HasPrefix(rest, []byte("<!")):
			// DOCTYPE, possibly with an internal subset.
			j, depth := 2, 0
			for ; j < len(rest); j++ {
				if rest[j] == '[' {
					depth++
				} else if rest[j] == ']' {
					depth--
				} else if rest[j] == '>' && depth <= 0 {
					break
				}
			}
			if j >= len(rest) {
				return nil, errMalformed
			}
			i += j + 1
		case bytes.HasPrefix(rest, []byte("</")):
			j := bytes.IndexByte(rest, '>')
			if j < 0 || len(stack) == 0 {
				return nil, errMalformed
			}
			name := bytes.TrimRight(rest[2:j], " \t\r\n")
			top := stack[len(stack)-1]
			if !sameName(name, top) {
				return nil, fmt.Errorf("xmp: element <%s> closed by </%s>", qname(top), name)
			}
			top.endStart = i
			top.end = i + j + 1
			f := first[len(first)-1]
			top.children = cut(&p.kids, p.pending[f:])
			p.pending = p.pending[:f]
			p.popNS(len(stack))
			stack, first = stack[:len(stack)-1], first[:len(first)-1]
			i += j + 1
		default:
			n, end, self, err := p.startTag(i, len(stack)+1)
			if err != nil {
				return nil, err
			}
			if len(stack) > 0 {
				n.parent = stack[len(stack)-1]
				p.pending = append(p.pending, n)
			} else if root == nil {
				root = n
			} else {
				return nil, errors.New("xmp: more than one root element")
			}
			i = end
			if self {
				n.endStart, n.end = end, end
				p.popNS(len(stack) + 1)
			} else {
				stack = append(stack, n)
				first = append(first, len(p.pending))
			}
		}
	}
	if root == nil || len(stack) > 0 {
		return nil, errMalformed
	}
	return root, nil
}

func sameName(name []byte, n *node) bool {
	if len(n.prefix) == 0 {
		return bytes.Equal(name, n.local)
	}
	return len(name) == len(n.prefix)+1+len(n.local) &&
		bytes.HasPrefix(name, n.prefix) && name[len(n.prefix)] == ':' && bytes.HasSuffix(name, n.local)
}

func qname(n *node) string {
	if len(n.prefix) == 0 {
		return string(n.local)
	}
	return string(n.prefix) + ":" + string(n.local)
}

func (p *parser) popNS(depth int) {
	for len(p.ns) > 0 && p.ns[len(p.ns)-1].depth >= depth {
		p.ns = p.ns[:len(p.ns)-1]
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func isNameEnd(c byte) bool { return isSpace(c) || c == '/' || c == '>' || c == '=' }

func splitName(b []byte) (prefix, local []byte) {
	if i := bytes.IndexByte(b, ':'); i >= 0 {
		return b[:i], b[i+1:]
	}
	return nil, b
}

// startTag parses the start tag at offset i.
func (p *parser) startTag(i, depth int) (n *node, end int, self bool, err error) {
	data := p.data
	j := i + 1
	for j < len(data) && !isNameEnd(data[j]) {
		j++
	}
	if j == i+1 || j >= len(data) {
		return nil, 0, false, errMalformed
	}
	n = p.newNode()
	n.start = i
	n.prefix, n.local = splitName(data[i+1 : j])
	p.attrBuf = p.attrBuf[:0]
	for {
		for j < len(data) && isSpace(data[j]) {
			j++
		}
		if j >= len(data) {
			return nil, 0, false, errMalformed
		}
		if data[j] == '>' {
			j++
			break
		}
		if data[j] == '/' {
			if j+1 < len(data) && data[j+1] == '>' {
				j += 2
				self = true
				break
			}
			return nil, 0, false, errMalformed
		}
		as := j
		for j < len(data) && !isNameEnd(data[j]) {
			j++
		}
		name := data[as:j]
		for j < len(data) && isSpace(data[j]) {
			j++
		}
		if len(name) == 0 || j >= len(data) || data[j] != '=' {
			return nil, 0, false, errMalformed
		}
		j++
		for j < len(data) && isSpace(data[j]) {
			j++
		}
		if j >= len(data) || (data[j] != '"' && data[j] != '\'') {
			return nil, 0, false, errMalformed
		}
		q := data[j]
		k := bytes.IndexByte(data[j+1:], q)
		if k < 0 {
			return nil, 0, false, errMalformed
		}
		value := data[j+1 : j+1+k]
		j += k + 2
		prefix, local := splitName(name)
		switch {
		case prefix == nil && string(local) == "xmlns":
			p.ns = append(p.ns, nsDecl{nil, namespaceURI(value), depth})
		case string(prefix) == "xmlns":
			p.ns = append(p.ns, nsDecl{local, namespaceURI(value), depth})
		default:
			p.attrBuf = append(p.attrBuf, attr{prefix: prefix, local: local, value: value, start: as, end: j})
		}
	}
	n.attrs = cut(&p.attrs, p.attrBuf)
	n.startEnd = j
	n.space = p.lookup(n.prefix)
	for k := range n.attrs {
		if a := &n.attrs[k]; a.prefix != nil {
			a.space = p.lookup(a.prefix)
		}
	}
	return n, j, self, nil
}

// namespaceURI returns the URI of the value of an xmlns attribute.
func namespaceURI(value []byte) string {
	if bytes.IndexByte(value, '&') >= 0 {
		return internURI([]byte(unescape(value)))
	}
	return internURI(value)
}

// internURI returns the URI b holds, without allocating the namespace URIs
// XMP readers care about.
func internURI(b []byte) string {
	switch string(b) {
	case nsRDF:
		return nsRDF
	case nsX:
		return nsX
	case nsPDFAID:
		return nsPDFAID
	case nsXMP:
		return nsXMP
	case nsDC:
		return nsDC
	case nsPDF:
		return nsPDF
	case nsPDFAExtension:
		return nsPDFAExtension
	case nsPDFASchema:
		return nsPDFASchema
	case nsPDFAProperty:
		return nsPDFAProperty
	case NSFacturX:
		return NSFacturX
	case NSZUGFeRD2:
		return NSZUGFeRD2
	case NSZUGFeRD1:
		return NSZUGFeRD1
	case NSZUGFeRD1RC:
		return NSZUGFeRD1RC
	}
	return string(b)
}

// unescape decodes the predefined entities and character references.
func unescape(b []byte) string {
	if bytes.IndexByte(b, '&') < 0 {
		return string(b)
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '&' {
			out = append(out, b[i])
			continue
		}
		j := bytes.IndexByte(b[i:], ';')
		if j < 0 {
			out = append(out, b[i:]...)
			break
		}
		ent := b[i+1 : i+j]
		switch string(ent) {
		case "lt":
			out = append(out, '<')
		case "gt":
			out = append(out, '>')
		case "amp":
			out = append(out, '&')
		case "quot":
			out = append(out, '"')
		case "apos":
			out = append(out, '\'')
		default:
			var r uint64
			var err error
			switch {
			case len(ent) > 2 && ent[0] == '#' && (ent[1] == 'x' || ent[1] == 'X'):
				r, err = strconv.ParseUint(string(ent[2:]), 16, 32)
			case len(ent) > 1 && ent[0] == '#':
				r, err = strconv.ParseUint(string(ent[1:]), 10, 32)
			default:
				err = errMalformed
			}
			if err != nil || !utf8.ValidRune(rune(r)) {
				out = append(out, b[i:i+j+1]...)
			} else {
				out = utf8.AppendRune(out, rune(r))
			}
		}
		i += j
	}
	return string(out)
}

// decodeText decodes element content: entities, CDATA sections; comments
// and processing instructions are dropped.
func decodeText(b []byte) string {
	if bytes.IndexByte(b, '<') < 0 {
		return unescape(b)
	}
	var out []byte
	for len(b) > 0 {
		k := bytes.IndexByte(b, '<')
		if k < 0 {
			out = append(out, unescape(b)...)
			break
		}
		out = append(out, unescape(b[:k])...)
		b = b[k:]
		switch {
		case bytes.HasPrefix(b, []byte("<![CDATA[")):
			j := bytes.Index(b, []byte("]]>"))
			if j < 0 {
				return string(out)
			}
			out = append(out, b[9:j]...)
			b = b[j+3:]
		case bytes.HasPrefix(b, []byte("<!--")):
			j := bytes.Index(b, []byte("-->"))
			if j < 0 {
				return string(out)
			}
			b = b[j+3:]
		default:
			j := bytes.IndexByte(b, '>')
			if j < 0 {
				return string(out)
			}
			b = b[j+1:]
		}
	}
	return string(out)
}

// toUTF8 converts a packet encoded in UTF-16 (with or without byte order
// mark) to UTF-8. Other packets are returned unchanged.
func toUTF8(b []byte) []byte {
	var be bool
	switch {
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		be, b = true, b[2:]
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		be, b = false, b[2:]
	case len(b) >= 2 && b[0] == 0 && b[1] != 0:
		be = true
	case len(b) >= 2 && b[0] != 0 && b[1] == 0:
		be = false
	default:
		return b
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if be {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return []byte(string(utf16.Decode(u)))
}
