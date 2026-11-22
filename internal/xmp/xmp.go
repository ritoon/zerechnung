// Package xmp reads and writes the XMP metadata of Factur-X / ZUGFeRD PDFs:
// the PDF/A identification (pdfaid), the hybrid-invoice schema (fx) and the
// PDF/A extension schema that describes it.
//
// Packets are parsed by a small XML tokenizer that keeps byte offsets, so
// that Merge edits an existing packet in place and preserves everything it
// does not manage.
package xmp

import (
	"errors"
	"strings"
	"time"
)

// Namespaces of the hybrid-invoice XMP schema.
const (
	// NSFacturX is the Factur-X namespace (also used by ZUGFeRD 2.1 and
	// later, and by XRechnung hybrids).
	NSFacturX = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"
	// NSZUGFeRD2 is the ZUGFeRD 2.0 namespace.
	NSZUGFeRD2 = "urn:zugferd:pdfa:CrossIndustryDocument:invoice:2p0#"
	// NSZUGFeRD1 is the ZUGFeRD 1.0 namespace.
	NSZUGFeRD1 = "urn:ferd:pdfa:CrossIndustryDocument:invoice:1p0#"
	// NSZUGFeRD1RC is the namespace of the ZUGFeRD 1.0 release candidate,
	// found in some 2014 files.
	NSZUGFeRD1RC = "urn:ferd:pdfa:invoice:rc#"
)

// Other namespaces.
const (
	nsRDF           = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	nsX             = "adobe:ns:meta/"
	nsXML           = "http://www.w3.org/XML/1998/namespace"
	nsPDFAID        = "http://www.aiim.org/pdfa/ns/id/"
	nsXMP           = "http://ns.adobe.com/xap/1.0/"
	nsDC            = "http://purl.org/dc/elements/1.1/"
	nsPDF           = "http://ns.adobe.com/pdf/1.3/"
	nsPDFAExtension = "http://www.aiim.org/pdfa/ns/extension/"
	nsPDFASchema    = "http://www.aiim.org/pdfa/ns/schema#"
	nsPDFAProperty  = "http://www.aiim.org/pdfa/ns/property#"
)

// InvoiceNamespaces lists the hybrid-invoice namespaces, preferred first.
var InvoiceNamespaces = []string{NSFacturX, NSZUGFeRD2, NSZUGFeRD1, NSZUGFeRD1RC}

func isInvoiceNS(s string) bool {
	return s == NSFacturX || s == NSZUGFeRD2 || s == NSZUGFeRD1 || s == NSZUGFeRD1RC
}

// Info is what an XMP packet says about a hybrid invoice.
type Info struct {
	About           string // rdf:about of the first rdf:Description
	PDFAPart        string // pdfaid:part, "" when absent
	PDFAConformance string // pdfaid:conformance, "" when absent
	PDFARev         string // pdfaid:rev, the year of the PDF/A-4 edition ("2020"), "" when absent
	Invoice         *Invoice
	Schemas         []Schema // PDF/A extension schemas (pdfaExtension:schemas)
	ModifyDate      string   // xmp:ModifyDate
	CreateDate      string   // xmp:CreateDate
	MetadataDate    string   // xmp:MetadataDate
	AboutMismatch   bool     // rdf:Description elements carry different rdf:about values
	// InfoProps holds the properties that have an equivalent entry in the
	// PDF document information dictionary, keyed by the name of that entry:
	// Title (dc:title), Author (dc:creator), Subject (dc:description),
	// Keywords (pdf:Keywords), Creator (xmp:CreatorTool), Producer
	// (pdf:Producer), CreationDate (xmp:CreateDate) and ModDate
	// (xmp:ModifyDate). Only the properties found in the packet are listed.
	// dc:title and dc:description give their x-default alternative (the
	// first one when none is marked x-default), dc:creator its entries
	// joined with ", ".
	InfoProps map[string]string
}

// InfoEquivalents lists the entries of the PDF document information
// dictionary that have an XMP equivalent (ISO 19005-1, Table 1), with the
// qualified name of that property.
var InfoEquivalents = []struct{ Entry, Property string }{
	{"Title", "dc:title"},
	{"Author", "dc:creator"},
	{"Subject", "dc:description"},
	{"Keywords", "pdf:Keywords"},
	{"Creator", "xmp:CreatorTool"},
	{"Producer", "pdf:Producer"},
	{"CreationDate", "xmp:CreateDate"},
	{"ModDate", "xmp:ModifyDate"},
}

// infoEntry returns the document information entry equivalent to an XMP
// property, or "".
func infoEntry(space, local string) string {
	switch space {
	case nsDC:
		switch local {
		case "title":
			return "Title"
		case "creator":
			return "Author"
		case "description":
			return "Subject"
		}
	case nsPDF:
		switch local {
		case "Keywords", "Producer":
			return local
		}
	case nsXMP:
		switch local {
		case "CreatorTool":
			return "Creator"
		case "CreateDate":
			return "CreationDate"
		case "ModifyDate":
			return "ModDate"
		}
	}
	return ""
}

func (in *Info) setInfoProp(entry, value string) {
	if in.InfoProps == nil {
		in.InfoProps = make(map[string]string, len(InfoEquivalents))
	}
	in.InfoProps[entry] = value
}

// setList reads a property whose value is an rdf:Alt, rdf:Seq or rdf:Bag
// and that has a document information equivalent: dc:title and
// dc:description (language alternatives), dc:creator (ordered list).
func (in *Info) setList(data []byte, n *node) {
	entry := infoEntry(n.space, string(n.local))
	if entry == "" {
		return
	}
	var list *node
	for _, c := range n.children {
		if c.is(nsRDF, "Alt") || c.is(nsRDF, "Seq") || c.is(nsRDF, "Bag") {
			list = c
			break
		}
	}
	if list == nil {
		return
	}
	var (
		items []string
		def   = -1 // index of the x-default alternative
	)
	for _, li := range list.children {
		if !li.is(nsRDF, "li") || len(li.children) > 0 {
			continue
		}
		if lang, _ := li.attr(nsXML, "lang"); def < 0 && strings.EqualFold(lang, "x-default") {
			def = len(items)
		}
		items = append(items, li.text(data))
	}
	value := ""
	switch {
	case entry == "Author":
		value = strings.Join(items, ", ")
	case def >= 0:
		value = items[def]
	case len(items) > 0:
		value = items[0]
	}
	in.setInfoProp(entry, value)
}

// ParseDate parses an XMP date: ISO 8601 as XMP restricts it ("2026",
// "2026-11", "2026-11-16", "2026-11-16T14:30+01:00",
// "2026-11-16T14:30:00Z", with optional fractional seconds). A date and
// time without time zone is read as UTC.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
		"2006-01",
		"2006",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Invoice holds the properties of the hybrid-invoice schema.
type Invoice struct {
	NamespaceURI     string // one of InvoiceNamespaces
	DocumentType     string
	DocumentFileName string
	Version          string
	ConformanceLevel string
}

// Schema is a PDF/A extension schema description.
type Schema struct {
	NamespaceURI string
	Prefix       string
	Name         string
	Properties   []string // pdfaProperty:name of the described properties
}

// Schema returns the extension schema describing namespace uri, or nil.
func (in *Info) Schema(uri string) *Schema {
	for i := range in.Schemas {
		if in.Schemas[i].NamespaceURI == uri {
			return &in.Schemas[i]
		}
	}
	return nil
}

// ErrNoRDF is returned by Parse when the packet holds no rdf:RDF element.
var ErrNoRDF = errors.New("xmp: no rdf:RDF element")

// Packet is an XMP packet read once, for Info and CheckPDFA to share. The
// packet is not modified once read.
type Packet struct {
	raw  []byte // as given
	data []byte // converted to UTF-8 when given in UTF-16
	root *node
	err  error // why the packet is not well-formed XML
}

// ParsePacket reads an XMP packet.
func ParsePacket(data []byte) *Packet {
	p := &Packet{raw: data, data: toUTF8(data)}
	p.root, p.err = parse(p.data)
	return p
}

// Parse reads an XMP packet.
func Parse(data []byte) (*Info, error) { return ParsePacket(data).Info() }

// Info returns what the packet declares, as Parse does.
func (p *Packet) Info() (*Info, error) {
	if p.err != nil {
		return nil, p.err
	}
	data := p.data
	rdf := findRDF(p.root)
	if rdf == nil {
		return nil, ErrNoRDF
	}
	info := &Info{}
	invoices := map[string]*Invoice{}
	first := true
	for _, d := range rdf.children {
		if !d.is(nsRDF, "Description") {
			continue
		}
		about, _ := d.attr(nsRDF, "about")
		if first {
			info.About, first = about, false
		} else if about != info.About {
			info.AboutMismatch = true
		}
		for _, a := range d.attrs {
			info.setProp(invoices, a.space, string(a.local), unescape(a.value))
		}
		for _, c := range d.children {
			if c.is(nsPDFAExtension, "schemas") {
				info.readSchemas(data, c)
				continue
			}
			if len(c.children) == 0 {
				info.setProp(invoices, c.space, string(c.local), c.text(data))
			} else {
				info.setList(data, c)
			}
		}
	}
	for _, ns := range InvoiceNamespaces {
		if inv := invoices[ns]; inv != nil {
			info.Invoice = inv
			break
		}
	}
	return info, nil
}

func findRDF(n *node) *node {
	if n.is(nsRDF, "RDF") {
		return n
	}
	for _, c := range n.children {
		if r := findRDF(c); r != nil {
			return r
		}
	}
	return nil
}

func (in *Info) setProp(invoices map[string]*Invoice, space, local, value string) {
	if entry := infoEntry(space, local); entry != "" {
		in.setInfoProp(entry, value)
	}
	switch {
	case space == nsPDFAID:
		switch local {
		case "part":
			in.PDFAPart = value
		case "conformance":
			in.PDFAConformance = value
		case "rev":
			in.PDFARev = value
		}
	case space == nsXMP:
		switch local {
		case "ModifyDate":
			in.ModifyDate = value
		case "CreateDate":
			in.CreateDate = value
		case "MetadataDate":
			in.MetadataDate = value
		}
	case isInvoiceNS(space):
		inv := invoices[space]
		if inv == nil {
			inv = &Invoice{NamespaceURI: space}
			invoices[space] = inv
		}
		switch local {
		case "DocumentType":
			inv.DocumentType = value
		case "DocumentFileName":
			inv.DocumentFileName = value
		case "Version":
			inv.Version = value
		case "ConformanceLevel":
			inv.ConformanceLevel = value
		}
	}
}

// fields returns the value of the leaf child elements and property
// attributes of a structure (rdf:li with rdf:parseType="Resource", or an
// rdf:Description inside the rdf:li).
func fields(data []byte, n *node, space string, fn func(local, value string, c *node)) {
	visit := func(n *node) {
		for _, a := range n.attrs {
			if a.space == space {
				fn(string(a.local), unescape(a.value), nil)
			}
		}
		for _, c := range n.children {
			if c.space == space {
				v := ""
				if len(c.children) == 0 {
					v = c.text(data)
				}
				fn(string(c.local), v, c)
			}
		}
	}
	visit(n)
	if d := n.child(nsRDF, "Description"); d != nil {
		visit(d)
	}
}

func (in *Info) readSchemas(data []byte, schemas *node) {
	for _, bag := range schemas.children {
		for _, li := range bag.children {
			if !li.is(nsRDF, "li") {
				continue
			}
			var s Schema
			fields(data, li, nsPDFASchema, func(local, value string, c *node) {
				switch local {
				case "namespaceURI":
					s.NamespaceURI = value
				case "prefix":
					s.Prefix = value
				case "schema":
					s.Name = value
				case "property":
					if c == nil {
						return
					}
					for _, seq := range c.children {
						for _, pli := range seq.children {
							if !pli.is(nsRDF, "li") {
								continue
							}
							fields(data, pli, nsPDFAProperty, func(local, value string, _ *node) {
								if local == "name" {
									s.Properties = append(s.Properties, value)
								}
							})
						}
					}
				}
			})
			in.Schemas = append(in.Schemas, s)
		}
	}
}
