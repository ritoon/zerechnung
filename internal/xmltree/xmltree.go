// Package xmltree is a small read-only XML tree used to parse and validate
// CII invoices.
//
// Elements are matched by local name: the prefix used in the document does not
// matter, and paths given to Find, FindAll and Value may carry a prefix
// ("ram:ID") or not ("ID"). The namespace URI of each element is still
// resolved, so callers can check it when it matters (the document root).
package xmltree

import (
	"strconv"
	"strings"
)

// Node is an XML element.
type Node struct {
	Name     string  // local name
	Space    string  // namespace URI, resolved from the xmlns declarations in scope
	Prefix   string  // prefix used in the document ("" when none)
	Attrs    []Attr  // attributes, in document order (namespace declarations excluded)
	Text     string  // character data directly inside the element, untrimmed
	Children []*Node // child elements, in document order
	Parent   *Node
	Line     int // line of the start tag, 1-based
}

// Attr is an XML attribute.
type Attr struct {
	Name   string // local name
	Prefix string
	Value  string
}

// MaxDepth bounds the nesting of elements accepted by Parse. CII invoices
// nest about a dozen levels deep; the limit protects the recursive helpers of
// this package and its callers from hostile documents.
const MaxDepth = 256

// Elem returns the first child element with the given local name, or nil.
func (n *Node) Elem(name string) *Node {
	if n == nil {
		return nil
	}
	name = local(name)
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Elems returns the child elements with the given local name.
func (n *Node) Elems(name string) []*Node {
	if n == nil {
		return nil
	}
	name = local(name)
	var out []*Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// Find returns the first element matching a relative path of local names
// separated by "/" ("ram:ApplicableHeaderTradeSettlement/ram:InvoiceCurrencyCode"),
// or nil. Every branch is explored: the result is the first match in document
// order. An empty path returns n itself.
func (n *Node) Find(path string) *Node {
	if n == nil {
		return nil
	}
	if path == "" {
		return n
	}
	seg, rest, _ := strings.Cut(path, "/")
	seg = local(seg)
	for _, c := range n.Children {
		if c.Name != seg {
			continue
		}
		if rest == "" {
			return c
		}
		if m := c.Find(rest); m != nil {
			return m
		}
	}
	return nil
}

// FindAll returns every element matching a relative path, in document order.
func (n *Node) FindAll(path string) []*Node {
	if n == nil {
		return nil
	}
	if path == "" {
		return []*Node{n}
	}
	var out []*Node
	n.findAll(path, &out)
	return out
}

func (n *Node) findAll(path string, out *[]*Node) {
	seg, rest, _ := strings.Cut(path, "/")
	seg = local(seg)
	for _, c := range n.Children {
		if c.Name != seg {
			continue
		}
		if rest == "" {
			*out = append(*out, c)
		} else {
			c.findAll(rest, out)
		}
	}
}

// Value returns the trimmed text of the first element matching path, or "".
func (n *Node) Value(path string) string {
	return n.Find(path).Trim()
}

// Exists reports whether an element matches path.
func (n *Node) Exists(path string) bool { return n.Find(path) != nil }

// Count returns the number of elements matching path.
func (n *Node) Count(path string) int { return len(n.FindAll(path)) }

// Trim returns the text of n with leading and trailing white space removed,
// or "" when n is nil.
func (n *Node) Trim() string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Text)
}

// Normalized returns the text of n as XPath normalize-space() would: trimmed,
// with every run of white space replaced by a single space.
func (n *Node) Normalized() string {
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(n.Text), " ")
}

// Attr returns the value of the attribute with the given local name, or "".
func (n *Node) Attr(name string) string {
	v, _ := n.LookupAttr(name)
	return v
}

// LookupAttr returns the value of an attribute and whether it is present.
func (n *Node) LookupAttr(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	name = local(name)
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Walk calls fn for n and each descendant, depth first in document order.
// When fn returns false the children of that node are skipped.
func (n *Node) Walk(fn func(*Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Path returns an XPath-like location of n, such as
// "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem[2]/ram:AssociatedDocumentLineDocument".
// Prefixes are the conventional CII ones (rsm, ram, udt, qdt), UBL ones
// (cac, cbc, ext, none for the root) and FatturaPA one (p, for the root)
// when the namespace is known, the document prefix otherwise. A position is added when
// the parent has several children with the same name.
func (n *Node) Path() string {
	if n == nil {
		return ""
	}
	var segs []string
	for c := n; c != nil; c = c.Parent {
		s := c.qualified()
		if p := c.Parent; p != nil {
			idx, total := 0, 0
			for _, sib := range p.Children {
				if sib.Name == c.Name {
					total++
					if sib == c {
						idx = total
					}
				}
			}
			if total > 1 {
				s += "[" + strconv.Itoa(idx) + "]"
			}
		}
		segs = append(segs, s)
	}
	var b strings.Builder
	for i := len(segs) - 1; i >= 0; i-- {
		b.WriteByte('/')
		b.WriteString(segs[i])
	}
	return b.String()
}

// Conventional prefixes of the CII, UBL and FatturaPA namespaces. The roots
// of UBL documents (Invoice, CreditNote) get no prefix.
var prefixes = map[string]string{
	"urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100":                       "rsm",
	"urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100": "ram",
	"urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100":                        "udt",
	"urn:un:unece:uncefact:data:standard:QualifiedDataType:100":                          "qdt",
	"urn:oasis:names:specification:ubl:schema:xsd:Invoice-2":                             "",
	"urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2":                          "",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2":           "cac",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2":               "cbc",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2":           "ext",
	"http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2":                      "p",
}

func (n *Node) qualified() string {
	if p, ok := prefixes[n.Space]; ok {
		if p == "" {
			return n.Name
		}
		return p + ":" + n.Name
	}
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

func local(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}
