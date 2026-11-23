package ubl

import (
	"encoding/base64"

	"github.com/ritoon/zerechnung/internal/xmlenc"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// appendElement appends n and its content as XML, for an extension. UBL
// elements get the conventional cac, cbc and ext prefixes, declared on the
// root of the documents written by Marshal; other elements keep their prefix
// and get a namespace declaration. Attributes in other namespaces than the
// element's (xsi:…) are not kept.
func appendElement(b []byte, n *xmltree.Node) []byte {
	name, decl := qname(n), ""
	if prefixOf(n.Space) == "" && n.Space != "" {
		if n.Prefix != "" {
			decl = "xmlns:" + n.Prefix + `="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		} else {
			decl = `xmlns="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		}
	}
	b = append(b, '<')
	b = append(b, name...)
	if decl != "" {
		b = append(b, ' ')
		b = append(b, decl...)
	}
	for _, a := range n.Attrs {
		if a.Prefix != "" && a.Prefix != "xml" {
			continue
		}
		b = append(b, ' ')
		if a.Prefix != "" {
			b = append(b, a.Prefix...)
			b = append(b, ':')
		}
		b = append(b, a.Name...)
		b = append(b, '=', '"')
		b = xmlenc.AppendEscaped(b, a.Value, true)
		b = append(b, '"')
	}
	b = append(b, '>')
	if len(n.Children) == 0 {
		b = xmlenc.AppendEscaped(b, n.Text, false)
	} else {
		for _, c := range n.Children {
			b = appendElement(b, c)
		}
	}
	b = append(b, '<', '/')
	b = append(b, name...)
	return append(b, '>')
}

// writer appends XML to a byte slice. Element names are written as given
// (with their cac or cbc prefix).
type writer struct {
	b      []byte
	depth  int
	indent bool
	credit bool   // writing a CreditNote
	cur    string // invoice currency (BT-5), the currencyID of the amounts
	// gaccount is set when the specification identifier written declares
	// the Dutch G-account extension, which adds payment means identifiers
	// and several payment terms to the binding.
	gaccount bool
	// italy is set when the specification identifier written declares the
	// Italian extension (EXT-IT): the national data that appendix 4.2 of the
	// Italian technical rules binds to UBL is written (see italy.go).
	italy     bool
	it        *invoice.Italy
	orderAt   int              // index in it.PurchaseOrders of the order of the document, or -1
	ddtAt     int              // index in it.TransportDocuments of the transport document of the document, or -1
	lineOrder map[string]int   // index in it.PurchaseOrders of the order of each invoice line, by line identifier
	lineDDT   map[string][]int // indexes in it.TransportDocuments of the transport documents of each invoice line
	carried   map[string]bool  // national data written in full (see carry)
	// croatia is set when the specification identifier written is the one
	// of the CIUS-HR: the Croatian data is written (see croatia.go).
	croatia bool

	losses []string
}

// indentation is sliced to write a newline followed by the indentation of
// the current depth.
const indentation = "\n                                                                                "

func (w *writer) newline() {
	if !w.indent {
		return
	}
	n := 1 + 2*w.depth
	if n > len(indentation) {
		n = len(indentation)
	}
	w.b = append(w.b, indentation[:n]...)
}

// start writes the start tag of an aggregate.
func (w *writer) start(name string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
	w.depth++
}

// end writes the end tag of an aggregate.
func (w *writer) end(name string) {
	w.depth--
	w.newline()
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// open writes the start of a leaf start tag, leaving it open for attributes.
func (w *writer) open(name string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
}

// attr writes an attribute in an open start tag.
func (w *writer) attr(name, value string) {
	w.b = append(w.b, ' ')
	w.b = append(w.b, name...)
	w.b = append(w.b, '=', '"')
	w.b = xmlenc.AppendEscaped(w.b, value, true)
	w.b = append(w.b, '"')
}

// close ends an open start tag, writes value and the end tag.
func (w *writer) close(name, value string) {
	w.b = append(w.b, '>')
	w.b = xmlenc.AppendEscaped(w.b, value, false)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// leafReq writes a leaf element, even when value is empty.
func (w *writer) leafReq(name, value string) {
	w.open(name)
	w.close(name, value)
}

// leaf writes a leaf element when value is not empty.
func (w *writer) leaf(name, value string) {
	if value != "" {
		w.leafReq(name, value)
	}
}

// leafAttr writes a leaf element with an optional attribute, when value or
// the attribute value is not empty.
func (w *writer) leafAttr(name, attrName, attrValue, value string) {
	if value == "" && attrValue == "" {
		return
	}
	w.open(name)
	if attrValue != "" {
		w.attr(attrName, attrValue)
	}
	w.close(name, value)
}

// ident writes an identifier with its schemeID.
func (w *writer) ident(name string, id invoice.Identifier) {
	w.leafAttr(name, "schemeID", id.Scheme, id.ID)
}

// amountReq writes an amount in currency cur, even when d is not set.
func (w *writer) amountReq(name string, d invoice.Decimal, cur string) {
	w.open(name)
	w.attr("currencyID", cur)
	w.b = append(w.b, '>')
	if d.IsSet() {
		w.b = d.Append(w.b)
	}
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// amount writes an amount in the invoice currency when d is set.
func (w *writer) amount(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.amountReq(name, d, w.cur)
	}
}

// dec writes a decimal without currency (a rate, a percentage) when d is
// set.
func (w *writer) dec(name string, d invoice.Decimal) {
	if !d.IsSet() {
		return
	}
	w.open(name)
	w.b = append(w.b, '>')
	w.b = d.Append(w.b)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// quantity writes a quantity with its unit code.
func (w *writer) quantity(name string, d invoice.Decimal, unit string) {
	if !d.IsSet() && unit == "" {
		return
	}
	w.open(name)
	if unit != "" {
		w.attr("unitCode", unit)
	}
	w.b = append(w.b, '>')
	if d.IsSet() {
		w.b = d.Append(w.b)
	}
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// date writes a date (xs:date) when d is set.
func (w *writer) date(name string, d invoice.Date) {
	if d.IsZero() {
		return
	}
	w.leafReq(name, d.String())
}

// binary writes a base64 binary object with its MIME code and file name.
func (w *writer) binary(name string, data []byte, mime, filename string) {
	w.open(name)
	w.attr("mimeCode", mime)
	w.attr("filename", filename)
	w.b = append(w.b, '>')
	w.b = base64.StdEncoding.AppendEncode(w.b, data)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// lose records a business term that UBL cannot carry.
func (w *writer) lose(term string) {
	for _, t := range w.losses {
		if t == term {
			return
		}
	}
	w.losses = append(w.losses, term)
}

// loseText records term as lost when value is not empty.
func (w *writer) loseText(value, term string) {
	if value != "" {
		w.lose(term)
	}
}

// loseIf records term as lost when cond holds.
func (w *writer) loseIf(cond bool, term string) {
	if cond {
		w.lose(term)
	}
}
