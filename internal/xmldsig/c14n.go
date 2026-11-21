package xmldsig

import (
	"errors"
	"sort"
	"strings"
)

// Canonicalization algorithms.
const (
	C14N10             = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	C14N10WithComments = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments"
	C14N11             = "http://www.w3.org/2006/12/xml-c14n11"
	C14N11WithComments = "http://www.w3.org/2006/12/xml-c14n11#WithComments"
	ExcC14N            = "http://www.w3.org/2001/10/xml-exc-c14n#"
	ExcC14NWithComment = "http://www.w3.org/2001/10/xml-exc-c14n#WithComments"
)

// c14nMethod is a canonicalization algorithm.
type c14nMethod struct {
	exclusive bool
	v11       bool
	comments  bool
	// prefixes is the InclusiveNamespaces PrefixList of an exclusive
	// canonicalization ("#default" for the default namespace).
	prefixes map[string]bool
}

func c14nFor(uri string) (c14nMethod, bool) {
	switch uri {
	case C14N10:
		return c14nMethod{}, true
	case C14N10WithComments:
		return c14nMethod{comments: true}, true
	case C14N11:
		return c14nMethod{v11: true}, true
	case C14N11WithComments:
		return c14nMethod{v11: true, comments: true}, true
	case ExcC14N:
		return c14nMethod{exclusive: true}, true
	case ExcC14NWithComment:
		return c14nMethod{exclusive: true, comments: true}, true
	}
	return c14nMethod{}, false
}

var errXMLBase = errors.New("xmldsig: xml:base in the ancestors of a canonicalized subtree is not supported")

// canonicalize returns the canonical form (Canonical XML 1.0 or 1.1, or
// Exclusive XML Canonicalization) of a node-set made of the subtree of apex
// (the whole document when apex is nil) without the subtrees of the
// excluded elements. Comments are output when the method keeps them and
// withComments is set: the node-sets of same-document references have none.
func canonicalize(doc *document, apex *node, excluded func(*node) bool, m c14nMethod, withComments bool) ([]byte, error) {
	c := &canon{m: m, comments: m.comments && withComments, excluded: excluded,
		scope: map[string]string{}, out: map[string]string{}}
	if apex != nil {
		// The namespaces the ancestors of the apex declare, outermost first.
		var chain []*node
		for e := apex.parent; e != nil; e = e.parent {
			chain = append(chain, e)
		}
		for i := len(chain) - 1; i >= 0; i-- {
			for _, d := range chain[i].nsDecls {
				c.scope[d.prefix] = d.uri
			}
		}
	}
	if apex == nil {
		for _, n := range doc.before {
			if c.misc(n) {
				c.b.WriteByte('\n')
			}
		}
		if err := c.element(doc.root, true); err != nil {
			return nil, err
		}
		for _, n := range doc.after {
			if c.wouldOutput(n) {
				c.b.WriteByte('\n')
				c.misc(n)
			}
		}
		return []byte(c.b.String()), nil
	}
	if err := c.element(apex, true); err != nil {
		return nil, err
	}
	return []byte(c.b.String()), nil
}

type canon struct {
	b        strings.Builder
	m        c14nMethod
	comments bool
	excluded func(*node) bool
	// scope maps each prefix in scope of the current element to its
	// namespace URI ("" for the default namespace), the xml one aside; out
	// maps each prefix the output ancestors declared to its URI. Both are
	// updated on entering an element and restored on leaving it, so that
	// canonicalization stays linear in the size of the document.
	scope, out map[string]string
}

// saved is the value of a prefix in scope or out before an element changed
// it.
type saved struct {
	prefix, uri string
	had         bool
}

func set(m map[string]string, prefix, uri string, undo *[]saved) {
	old, had := m[prefix]
	*undo = append(*undo, saved{prefix, old, had})
	m[prefix] = uri
}

func restore(m map[string]string, undo []saved) {
	for i := len(undo) - 1; i >= 0; i-- {
		if u := undo[i]; u.had {
			m[u.prefix] = u.uri
		} else {
			delete(m, u.prefix)
		}
	}
}

// lookup returns the namespace URI bound to prefix in the current scope.
func (c *canon) lookup(prefix string) (string, bool) {
	if uri, ok := c.scope[prefix]; ok {
		return uri, true
	}
	return "", prefix == ""
}

func (c *canon) wouldOutput(n *node) bool {
	return n.kind == piNode || (n.kind == commentNode && c.comments)
}

// misc writes a comment or a processing instruction, and reports whether
// it wrote something.
func (c *canon) misc(n *node) bool {
	switch n.kind {
	case commentNode:
		if !c.comments {
			return false
		}
		c.b.WriteString("<!--")
		c.b.WriteString(n.data)
		c.b.WriteString("-->")
	case piNode:
		c.b.WriteString("<?")
		c.b.WriteString(n.target)
		if n.data != "" {
			c.b.WriteByte(' ')
			c.b.WriteString(n.data)
		}
		c.b.WriteString("?>")
	default:
		return false
	}
	return true
}

// element writes n and its subtree; apex tells that n has no output
// ancestor.
func (c *canon) element(n *node, apex bool) error {
	if c.excluded != nil && c.excluded(n) {
		return nil
	}
	var scopeUndo, outUndo []saved
	for _, d := range n.nsDecls {
		set(c.scope, d.prefix, d.uri, &scopeUndo)
	}
	// Namespace declarations to render: those whose URI differs from what
	// the output ancestors declared, an empty default namespace being
	// rendered only to undeclare one.
	var decls []nsDecl
	render := func(prefix, uri string) {
		if prefix == "xml" {
			return
		}
		if prev, had := c.out[prefix]; (had && prev == uri) || (!had && prefix == "" && uri == "") {
			return
		}
		decls = append(decls, nsDecl{prefix, uri})
		set(c.out, prefix, uri, &outUndo)
	}
	switch {
	case c.m.exclusive:
		// The visibly used prefixes and those of the InclusiveNamespaces
		// PrefixList.
		used := map[string]bool{n.prefix: true}
		for _, a := range n.attrs {
			if a.prefix != "" && a.prefix != "xml" {
				used[a.prefix] = true
			}
		}
		for p := range c.m.prefixes {
			if p == "#default" {
				used[""] = true
			} else {
				used[p] = true
			}
		}
		for p := range used {
			if uri, ok := c.lookup(p); ok {
				render(p, uri)
			}
		}
	case apex:
		for p, uri := range c.scope {
			render(p, uri)
		}
	default:
		// The parent is output with every namespace in its scope: only
		// the declarations of n can differ.
		for _, d := range n.nsDecls {
			render(d.prefix, d.uri)
		}
	}
	sort.Slice(decls, func(i, j int) bool { return decls[i].prefix < decls[j].prefix })

	attrs := append([]attr(nil), n.attrs...)
	if apex && !c.m.exclusive {
		inherited, err := c.inheritedXMLAttrs(n)
		if err != nil {
			return err
		}
		attrs = append(attrs, inherited...)
	}
	sort.Slice(attrs, func(i, j int) bool {
		if attrs[i].space != attrs[j].space {
			return attrs[i].space < attrs[j].space
		}
		return attrs[i].local < attrs[j].local
	})

	c.b.WriteByte('<')
	c.b.WriteString(qname(n.prefix, n.local))
	for _, d := range decls {
		if d.prefix == "" {
			c.b.WriteString(` xmlns="`)
		} else {
			c.b.WriteString(` xmlns:`)
			c.b.WriteString(d.prefix)
			c.b.WriteString(`="`)
		}
		escapeAttr(&c.b, d.uri)
		c.b.WriteByte('"')
	}
	for _, a := range attrs {
		c.b.WriteByte(' ')
		c.b.WriteString(qname(a.prefix, a.local))
		c.b.WriteString(`="`)
		escapeAttr(&c.b, a.value)
		c.b.WriteByte('"')
	}
	c.b.WriteByte('>')
	for _, k := range n.children {
		switch k.kind {
		case elementNode:
			if err := c.element(k, false); err != nil {
				return err
			}
		case textNode:
			escapeText(&c.b, k.data)
		default:
			c.misc(k)
		}
	}
	c.b.WriteString("</")
	c.b.WriteString(qname(n.prefix, n.local))
	c.b.WriteByte('>')
	restore(c.out, outUndo)
	restore(c.scope, scopeUndo)
	return nil
}

// inheritedXMLAttrs returns the attributes of the xml namespace that the apex
// of a subtree inherits from its ancestors in Canonical XML: all of them in
// 1.0, xml:lang and xml:space only in 1.1, which does not inherit xml:id
// and fixes up xml:base instead (not supported).
func (c *canon) inheritedXMLAttrs(n *node) ([]attr, error) {
	have := map[string]bool{}
	for _, a := range n.attrs {
		if a.space == nsXML {
			have[a.local] = true
		}
	}
	var out []attr
	for e := n.parent; e != nil; e = e.parent {
		for _, a := range e.attrs {
			if a.space != nsXML || have[a.local] {
				continue
			}
			if c.m.v11 {
				switch a.local {
				case "lang", "space":
				case "base":
					return nil, errXMLBase
				default:
					continue
				}
			}
			have[a.local] = true
			out = append(out, a)
		}
	}
	return out, nil
}

func escapeAttr(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		case '\t':
			b.WriteString("&#x9;")
		case '\n':
			b.WriteString("&#xA;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
	}
}

func escapeText(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
	}
}
