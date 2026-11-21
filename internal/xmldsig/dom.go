package xmldsig

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MaxSize bounds the length of the documents Parse reads, and MaxDepth the
// nesting of their elements.
const (
	MaxSize  = 64 << 20
	MaxDepth = 256
)

const (
	nsXML   = "http://www.w3.org/XML/1998/namespace"
	nsXMLNS = "http://www.w3.org/2000/xmlns/"
)

type kind uint8

const (
	elementNode kind = iota
	textNode
	commentNode
	piNode
)

// node is a node of the XPath data model that canonicalization needs:
// elements with their namespace declarations and attributes, text,
// comments and processing instructions, in document order.
type node struct {
	kind kind

	// Element.
	prefix, local string
	space         string   // namespace URI
	nsDecls       []nsDecl // namespace declarations, in document order
	attrs         []attr   // other attributes, in document order
	children      []*node
	parent        *node

	// Text and comment: data. Processing instruction: target and data.
	target string
	data   string
}

type nsDecl struct {
	prefix string // "" for the default namespace
	uri    string
}

type attr struct {
	prefix, local string
	space         string
	value         string
}

// document is a parsed XML document.
type document struct {
	root *node
	// before and after are the comments and processing instructions outside
	// the root element.
	before, after []*node
	// rootEnd is the offset in the input of the end tag of the root.
	rootEnd int
}

// attrValue returns the value of the attribute with no namespace and the
// given local name.
func (n *node) attrValue(local string) (string, bool) {
	for _, a := range n.attrs {
		if a.space == "" && a.local == local {
			return a.value, true
		}
	}
	return "", false
}

// is reports whether n is an element with the namespace and local name.
func (n *node) is(space, local string) bool {
	return n != nil && n.kind == elementNode && n.space == space && n.local == local
}

// elems returns the child elements of n with the namespace and local name.
func (n *node) elems(space, local string) []*node {
	var out []*node
	for _, c := range n.children {
		if c.is(space, local) {
			out = append(out, c)
		}
	}
	return out
}

// elem returns the only child element of n with the namespace and local
// name, or nil when there is none or several.
func (n *node) elem(space, local string) *node {
	e := n.elems(space, local)
	if len(e) != 1 {
		return nil
	}
	return e[0]
}

// text returns the character data directly in n.
func (n *node) text() string {
	var b strings.Builder
	for _, c := range n.children {
		if c.kind == textNode {
			b.WriteString(c.data)
		}
	}
	return b.String()
}

// childElements returns the child elements of n.
func (n *node) childElements() []*node {
	var out []*node
	for _, c := range n.children {
		if c.kind == elementNode {
			out = append(out, c)
		}
	}
	return out
}

// lookupNS returns the namespace URI bound to prefix in the scope of n.
func (n *node) lookupNS(prefix string) (string, bool) {
	if prefix == "xml" {
		return nsXML, true
	}
	for e := n; e != nil; e = e.parent {
		for _, d := range e.nsDecls {
			if d.prefix == prefix {
				return d.uri, true
			}
		}
	}
	if prefix == "" {
		return "", true
	}
	return "", false
}

// walk calls fn for n and its descendant elements, in document order.
func (n *node) walk(fn func(*node)) {
	if n.kind != elementNode {
		return
	}
	fn(n)
	for _, c := range n.children {
		c.walk(fn)
	}
}

// parse reads an XML document encoded in UTF-8. Documents with a DOCTYPE
// are refused: no entity other than the predefined ones is expanded, and
// no attribute gets a default value from a DTD.
func parse(data []byte) (*document, error) {
	if len(data) > MaxSize {
		return nil, fmt.Errorf("xmldsig: document larger than %d bytes", MaxSize)
	}
	base := 0
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		base = 3
	}
	input := data[base:]
	d := xml.NewDecoder(bytes.NewReader(input))
	doc := &document{}
	scope := nsScope{}
	var stack []*node
	prev := int64(0)
	seenDecl := false
	for first := true; ; first = false {
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xmldsig: %w", err)
		}
		cur := d.InputOffset()
		raw := input[prev:cur]
		start := prev
		prev = cur
		var parent *node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) >= MaxDepth {
				return nil, errors.New("xmldsig: elements nested too deep")
			}
			if parent == nil && doc.root != nil {
				return nil, errors.New("xmldsig: several root elements")
			}
			n, err := newElement(t, raw, parent, scope)
			if err != nil {
				return nil, err
			}
			if parent == nil {
				doc.root = n
			} else {
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if parent == nil || parent.prefix != t.Name.Space || parent.local != t.Name.Local {
				return nil, fmt.Errorf("xmldsig: unexpected end tag </%s>", qname(t.Name.Space, t.Name.Local))
			}
			scope.pop(parent.nsDecls)
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				doc.rootEnd = base + int(start)
			}
		case xml.CharData:
			if parent == nil {
				if len(bytes.TrimLeft(t, " \t\r\n")) != 0 {
					return nil, errors.New("xmldsig: character data outside the root element")
				}
				continue
			}
			if k := len(parent.children); k > 0 && parent.children[k-1].kind == textNode {
				parent.children[k-1].data += string(t)
			} else {
				parent.children = append(parent.children, &node{kind: textNode, data: string(t), parent: parent})
			}
		case xml.Comment:
			n := &node{kind: commentNode, data: normalizeNewlines(t), parent: parent}
			doc.attach(n, parent)
		case xml.ProcInst:
			if strings.EqualFold(t.Target, "xml") {
				if !first || seenDecl {
					return nil, errors.New("xmldsig: misplaced XML declaration")
				}
				seenDecl = true
				continue
			}
			n := &node{kind: piNode, target: t.Target, data: normalizeNewlines(t.Inst), parent: parent}
			doc.attach(n, parent)
		case xml.Directive:
			return nil, errors.New("xmldsig: documents with a DOCTYPE or another declaration are not accepted")
		}
	}
	if doc.root == nil {
		return nil, errors.New("xmldsig: no root element")
	}
	if len(stack) != 0 {
		return nil, errors.New("xmldsig: unexpected end of document")
	}
	return doc, nil
}

// normalizeNewlines turns each CR LF pair and each lone CR into LF, as XML
// 1.0 §2.11 requires: encoding/xml does it in character data, not in
// comments and processing instructions.
func normalizeNewlines(b []byte) string {
	if bytes.IndexByte(b, '\r') < 0 {
		return string(b)
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return string(bytes.ReplaceAll(b, []byte("\r"), []byte("\n")))
}

// nsScope holds the namespace declarations in scope while parsing: for each
// prefix, the URIs declared for it by the open elements, innermost last.
type nsScope map[string][]string

func (s nsScope) push(decls []nsDecl) {
	for _, d := range decls {
		s[d.prefix] = append(s[d.prefix], d.uri)
	}
}

func (s nsScope) pop(decls []nsDecl) {
	for _, d := range decls {
		if u := s[d.prefix]; len(u) > 1 {
			s[d.prefix] = u[:len(u)-1]
		} else {
			delete(s, d.prefix)
		}
	}
}

// lookup returns the namespace URI bound to prefix.
func (s nsScope) lookup(prefix string) (string, bool) {
	if prefix == "xml" {
		return nsXML, true
	}
	if u := s[prefix]; len(u) > 0 {
		return u[len(u)-1], true
	}
	if prefix == "" {
		return "", true
	}
	return "", false
}

func (doc *document) attach(n, parent *node) {
	switch {
	case parent != nil:
		parent.children = append(parent.children, n)
	case doc.root == nil:
		doc.before = append(doc.before, n)
	default:
		doc.after = append(doc.after, n)
	}
}

func qname(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

// newElement builds an element from its start tag. raw is the start tag as
// written, from which the attribute values are read again: encoding/xml
// does not normalize literal white space in attribute values, as XML 1.0
// §3.3.3 requires, and canonicalization works on normalized values.
func newElement(t xml.StartElement, raw []byte, parent *node, scope nsScope) (*node, error) {
	n := &node{kind: elementNode, prefix: t.Name.Space, local: t.Name.Local, parent: parent}
	values, err := rawAttrValues(raw)
	if err != nil {
		return nil, err
	}
	if len(values) != len(t.Attr) {
		return nil, errors.New("xmldsig: attributes cannot be read")
	}
	type pending struct {
		prefix, local, value string
	}
	var others []pending
	written := make(map[xml.Name]bool, len(t.Attr))
	for i, a := range t.Attr {
		// XML 1.0 §3.1 (Unique Att Spec): a name, namespace declarations
		// included, appears once in a start tag. A parser that took the
		// last of two declarations of a prefix would read other names
		// than the ones canonicalized.
		if written[a.Name] {
			return nil, fmt.Errorf("xmldsig: attribute %s repeated", qname(a.Name.Space, a.Name.Local))
		}
		written[a.Name] = true
		v := values[i]
		switch {
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			n.nsDecls = append(n.nsDecls, nsDecl{"", v})
		case a.Name.Space == "xmlns":
			if a.Name.Local == "xmlns" || v == "" || (a.Name.Local == "xml") != (v == nsXML) {
				return nil, fmt.Errorf("xmldsig: invalid namespace declaration xmlns:%s", a.Name.Local)
			}
			n.nsDecls = append(n.nsDecls, nsDecl{a.Name.Local, v})
		default:
			others = append(others, pending{a.Name.Space, a.Name.Local, v})
		}
	}
	if n.prefix == "xmlns" {
		return nil, errors.New("xmldsig: element with the xmlns prefix")
	}
	scope.push(n.nsDecls)
	space, ok := scope.lookup(n.prefix)
	if !ok {
		return nil, fmt.Errorf("xmldsig: undeclared namespace prefix %q", n.prefix)
	}
	n.space = space
	seen := map[[2]string]bool{}
	for _, a := range others {
		var space string
		if a.prefix != "" {
			var ok bool
			if space, ok = scope.lookup(a.prefix); !ok {
				return nil, fmt.Errorf("xmldsig: undeclared namespace prefix %q", a.prefix)
			}
		}
		key := [2]string{space, a.local}
		if seen[key] {
			return nil, fmt.Errorf("xmldsig: attribute %s repeated", qname(a.prefix, a.local))
		}
		seen[key] = true
		n.attrs = append(n.attrs, attr{a.prefix, a.local, space, a.value})
	}
	return n, nil
}

// rawAttrValues reads the attribute values of a start tag, normalized as
// XML 1.0 §3.3.3 does for CDATA attributes: references expanded, each
// literal white space character (a CR LF pair counting as one) replaced by
// a space. The tag has already been checked by encoding/xml.
func rawAttrValues(raw []byte) ([]string, error) {
	bad := errors.New("xmldsig: malformed start tag")
	i := 1 // after '<'
	for i < len(raw) && !isSpace(raw[i]) && raw[i] != '/' && raw[i] != '>' {
		i++
	}
	var out []string
	for {
		for i < len(raw) && isSpace(raw[i]) {
			i++
		}
		if i >= len(raw) {
			return nil, bad
		}
		if raw[i] == '/' || raw[i] == '>' {
			return out, nil
		}
		for i < len(raw) && raw[i] != '=' && !isSpace(raw[i]) {
			i++
		}
		for i < len(raw) && isSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			return nil, bad
		}
		i++
		for i < len(raw) && isSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || (raw[i] != '"' && raw[i] != '\'') {
			return nil, bad
		}
		q := raw[i]
		i++
		j := bytes.IndexByte(raw[i:], q)
		if j < 0 {
			return nil, bad
		}
		v, err := normalizeAttr(raw[i : i+j])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		i += j + 1
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func normalizeAttr(v []byte) (string, error) {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '\r':
			if i+1 < len(v) && v[i+1] == '\n' {
				i++
			}
			b.WriteByte(' ')
		case '\n', '\t':
			b.WriteByte(' ')
		case '&':
			j := bytes.IndexByte(v[i:], ';')
			if j < 0 {
				return "", errors.New("xmldsig: malformed reference in an attribute value")
			}
			r, ok := entity(string(v[i+1 : i+j]))
			if !ok {
				return "", fmt.Errorf("xmldsig: unknown entity &%s;", v[i+1:i+j])
			}
			b.WriteRune(r)
			i += j
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

func entity(name string) (rune, bool) {
	switch name {
	case "lt":
		return '<', true
	case "gt":
		return '>', true
	case "amp":
		return '&', true
	case "quot":
		return '"', true
	case "apos":
		return '\'', true
	}
	if strings.HasPrefix(name, "#") {
		var r int64
		base := int64(10)
		digits := name[1:]
		if strings.HasPrefix(digits, "x") {
			base, digits = 16, digits[1:]
		}
		if digits == "" || len(digits) > 8 {
			return 0, false
		}
		for _, c := range digits {
			var d int64
			switch {
			case '0' <= c && c <= '9':
				d = int64(c - '0')
			case base == 16 && 'a' <= c && c <= 'f':
				d = int64(c-'a') + 10
			case base == 16 && 'A' <= c && c <= 'F':
				d = int64(c-'A') + 10
			default:
				return 0, false
			}
			r = r*base + d
		}
		if r > 0x10FFFF {
			return 0, false
		}
		return rune(r), true
	}
	return 0, false
}
