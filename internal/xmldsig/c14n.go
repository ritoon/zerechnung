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
	c := &canon{m: m, comments: m.comments && withComments, excluded: excluded}
	if apex == nil {
		for _, n := range doc.before {
			if c.misc(n) {
				c.b.WriteByte('\n')
			}
		}
		if err := c.element(doc.root, nil, true); err != nil {
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
	if err := c.element(apex, nil, true); err != nil {
		return nil, err
	}
	return []byte(c.b.String()), nil
}

type canon struct {
	b        strings.Builder
	m        c14nMethod
	comments bool
	excluded func(*node) bool
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

// rendered maps a prefix to the namespace URI declared for it in the output
// so far ("" for the default namespace).
type rendered map[string]string

// element writes n and its subtree. ctx is the namespace context rendered
// by the output ancestors; apex tells that n has no output ancestor.
func (c *canon) element(n *node, ctx rendered, apex bool) error {
	if c.excluded != nil && c.excluded(n) {
		return nil
	}
	// Namespace declarations to render.
	var decls []nsDecl
	next := rendered{}
	for k, v := range ctx {
		next[k] = v
	}
	if c.m.exclusive {
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
			uri, ok := n.lookupNS(p)
			if !ok || p == "xml" {
				continue
			}
			if prev, had := ctx[p]; (had && prev == uri) || (!had && p == "" && uri == "") {
				continue
			}
			decls = append(decls, nsDecl{p, uri})
			next[p] = uri
		}
	} else {
		for p, uri := range inScope(n) {
			if prev, had := ctx[p]; (had && prev == uri) || (!had && p == "" && uri == "") {
				continue
			}
			decls = append(decls, nsDecl{p, uri})
			next[p] = uri
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
			if err := c.element(k, next, false); err != nil {
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
	return nil
}

// inScope returns the namespaces in scope of n, the xml one aside; an empty
// default namespace is returned as "" when it undeclares one.
func inScope(n *node) map[string]string {
	out := map[string]string{}
	for e := n; e != nil; e = e.parent {
		for _, d := range e.nsDecls {
			if _, ok := out[d.prefix]; !ok {
				out[d.prefix] = d.uri
			}
		}
	}
	delete(out, "xml")
	return out
}

// inheritedXMLAttrs returns the attributes of the xml namespace that the apex
// of a subtree inherits from its ancestors in Canonical XML: xml:lang,
// xml:space and, in 1.0 only, xml:base. Canonical XML 1.1 fixes up xml:base
// instead, which is not supported.
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
			switch a.local {
			case "lang", "space":
			case "base":
				if c.m.v11 {
					return nil, errXMLBase
				}
			default:
				continue
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
