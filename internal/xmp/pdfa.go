package xmp

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The PDF/A requirements on XMP packets of ISO 19005-2 and 19005-3,
// 6.6.2.1 to 6.6.2.3, as the veraPDF validation profiles number them.

// IssueKind classifies a PDF/A issue of an XMP packet.
type IssueKind uint8

// The kinds of issues, by clause of ISO 19005-2 and 19005-3.
const (
	IssuePacket    IssueKind = iota // 6.6.2.1: packet header, encoding, well-formedness
	IssueProperty                   // 6.6.2.3.1: property of a schema neither predefined nor described
	IssueExtension                  // 6.6.2.3.2, 6.6.2.3.3: PDF/A extension schema description
)

// Issue is a PDF/A issue of an XMP packet.
type Issue struct {
	Kind    IssueKind
	Message string
}

// Declared lists the properties described by PDF/A extension schemas, by
// namespace URI.
type Declared map[string]map[string]bool

// Declared returns the properties the extension schemas of the packet
// describe.
func (in *Info) Declared() Declared {
	d := Declared{}
	for _, s := range in.Schemas {
		m := d[s.NamespaceURI]
		if m == nil {
			m = map[string]bool{}
			d[s.NamespaceURI] = m
		}
		for _, p := range s.Properties {
			m[p] = true
		}
	}
	return d
}

// predefined lists the properties of the schemas PDF/A predefines: those of
// the XMP Specification 2005 and of ISO 19005. A nil set admits every
// property of the namespace: the large schemas (Exif, TIFF, Photoshop,
// Camera Raw, dynamic media, media management) are not listed property by
// property.
var predefined = func() map[string]map[string]bool {
	m := map[string]map[string]bool{}
	for ns, props := range map[string]string{
		nsDC:                                  "contributor coverage creator date description format identifier language publisher relation rights source subject title type",
		nsXMP:                                 "Advisory BaseURL CreateDate CreatorTool Identifier Label MetadataDate ModifyDate Nickname Rating Thumbnails",
		"http://ns.adobe.com/xap/1.0/rights/": "Certificate Marked Owner UsageTerms WebStatement",
		nsPDF:                                 "Keywords PDFVersion Producer Trapped",
		nsPDFAID:                              "part conformance amd corr rev",
		"http://ns.adobe.com/xap/1.0/bj/":     "JobRef",
		"http://ns.adobe.com/xap/1.0/t/pg/":   "MaxPageSize NPages Fonts Colorants PlateNames",
		nsPDFAExtension:                       "schemas",
	} {
		set := map[string]bool{}
		for _, p := range strings.Fields(props) {
			set[p] = true
		}
		m[ns] = set
	}
	for _, ns := range []string{
		"http://ns.adobe.com/xap/1.0/mm/",
		"http://ns.adobe.com/photoshop/1.0/",
		"http://ns.adobe.com/tiff/1.0/",
		"http://ns.adobe.com/exif/1.0/",
		"http://ns.adobe.com/exif/1.0/aux/",
		"http://ns.adobe.com/camera-raw-settings/1.0/",
		"http://ns.adobe.com/xmp/1.0/DynamicMedia/",
	} {
		m[ns] = nil
	}
	return m
}()

// CheckPDFA checks an XMP packet against the PDF/A requirements: the packet
// header has no bytes nor encoding attribute and the packet is UTF-8
// well-formed RDF/XML (6.6.2.1); every property belongs to a predefined
// schema or is described by an extension schema of the packet or of main,
// the extension schemas of the document metadata (6.6.2.3.1); the extension
// schemas are described as 6.6.2.3.3 requires. The properties of the
// hybrid-invoice schemas are left to the caller.
func CheckPDFA(data []byte, main Declared, fn func(Issue)) { ParsePacket(data).CheckPDFA(main, fn) }

// CheckPDFA checks the packet as the function CheckPDFA does.
func (p *Packet) CheckPDFA(main Declared, fn func(Issue)) {
	report := func(k IssueKind, format string, args ...any) {
		fn(Issue{Kind: k, Message: fmt.Sprintf(format, args...)})
	}
	data := p.raw
	if h := packetHeader(data); h != nil {
		if headerAttr(h, "bytes") {
			report(IssuePacket, "the xpacket header has a bytes attribute, which PDF/A forbids")
		}
		if headerAttr(h, "encoding") {
			report(IssuePacket, "the xpacket header has an encoding attribute, which PDF/A forbids")
		}
	}
	switch {
	case len(data) >= 2 && (data[0] == 0xFE && data[1] == 0xFF || data[0] == 0xFF && data[1] == 0xFE || data[0] == 0 || data[1] == 0):
		report(IssuePacket, "the XMP packet is encoded in UTF-16; PDF/A requires UTF-8")
	case !utf8.Valid(data):
		report(IssuePacket, "the XMP packet is not valid UTF-8")
	}
	// The packet in UTF-8: toUTF8 converts what the case above reports as
	// UTF-16 and leaves the other packets as they are.
	data = p.data
	root, err := p.root, p.err
	if err != nil {
		report(IssuePacket, "the XMP packet is not well-formed XML: %v", err)
		return
	}
	rdf := findRDF(root)
	if rdf == nil {
		report(IssuePacket, "the XMP packet has no rdf:RDF element")
		return
	}

	own := Declared{}
	for _, d := range rdf.children {
		for _, c := range d.children {
			if c.is(nsPDFAExtension, "schemas") {
				checkSchemas(data, c, own, report)
			}
		}
	}

	reported := map[string]bool{}
	property := func(space string, prefix, local []byte) {
		if space == nsRDF || space == nsXML || space == "" && prefix == nil || isInvoiceNS(space) {
			return
		}
		name := string(local)
		if space == nsPDFAID && string(prefix) != "pdfaid" {
			report(IssueProperty, "the property pdfaid:%s has the prefix %q; PDF/A requires pdfaid", name, prefix)
		}
		if set, ok := predefined[space]; ok && (set == nil || set[name]) {
			return
		}
		if main[space][name] || own[space][name] {
			return
		}
		q := qnameOf(prefix, local)
		if reported[space+" "+name] {
			return
		}
		reported[space+" "+name] = true
		if _, ok := predefined[space]; ok {
			report(IssueProperty, "the property %s is not defined by its predefined schema (%s)", q, space)
			return
		}
		report(IssueProperty, "the property %s (namespace %s) belongs to no predefined schema and is not described by a PDF/A extension schema", q, space)
	}
	typed := func(space string, prefix, local []byte, n *node, attrValue string) {
		typ := valueTypes[space][string(local)]
		if typ == "" || reported[space+" "+string(local)] {
			return
		}
		var msg string
		if n != nil {
			msg = checkValue(data, n, typ)
		} else {
			msg = checkAttr(attrValue, typ)
		}
		if msg != "" {
			reported[space+" "+string(local)] = true
			report(IssueProperty, "the property %s does not have the value type of its schema, %s: %s", qnameOf(prefix, local), typeLabel(typ), msg)
		}
	}
	for _, d := range rdf.children {
		if !d.is(nsRDF, "Description") {
			continue
		}
		for _, a := range d.attrs {
			property(a.space, a.prefix, a.local)
			typed(a.space, a.prefix, a.local, nil, unescape(a.value))
		}
		for _, c := range d.children {
			property(c.space, c.prefix, c.local)
			typed(c.space, c.prefix, c.local, c, "")
		}
	}
}

// headerAttr reports whether the xpacket header h has the attribute name:
// a match of \sname\s*= in the sense of package regexp, whose \s is a
// space, a tab, a line feed, a form feed or a carriage return.
func headerAttr(h []byte, name string) bool {
	for k := 1; k+len(name) <= len(h); k++ {
		if !reSpace(h[k-1]) || string(h[k:k+len(name)]) != name {
			continue
		}
		j := k + len(name)
		for j < len(h) && reSpace(h[j]) {
			j++
		}
		if j < len(h) && h[j] == '=' {
			return true
		}
	}
	return false
}

// reSpace reports whether c is white space as \s of package regexp
// matches it.
func reSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// packetHeader returns the xpacket processing instruction at the start of
// the packet, or nil.
func packetHeader(data []byte) []byte {
	i := bytes.Index(data, []byte("<?xpacket"))
	if i < 0 || i > 64 {
		return nil
	}
	j := bytes.Index(data[i:], []byte("?>"))
	if j < 0 {
		return nil
	}
	return data[i : i+j]
}

func qnameOf(prefix, local []byte) string {
	if len(prefix) == 0 {
		return string(local)
	}
	return string(prefix) + ":" + string(local)
}

// The namespaces of the PDF/A value types of 6.6.2.3.3.
const (
	nsPDFAType  = "http://www.aiim.org/pdfa/ns/type#"
	nsPDFAField = "http://www.aiim.org/pdfa/ns/field#"
)

// field is a field of a structure in an extension schema description.
type field struct {
	space, prefix, local, value string
	node                        *node // nil for an attribute
}

// structFields returns the fields of a structure: the rdf:li with
// rdf:parseType="Resource", or the rdf:Description inside it.
func structFields(data []byte, li *node) []field {
	var out []field
	visit := func(n *node) {
		for _, a := range n.attrs {
			if a.space == nsRDF || a.space == nsXML {
				continue
			}
			out = append(out, field{a.space, string(a.prefix), string(a.local), unescape(a.value), nil})
		}
		for _, c := range n.children {
			if c.is(nsRDF, "Description") {
				continue
			}
			v := ""
			if len(c.children) == 0 {
				v = c.text(data)
			}
			out = append(out, field{c.space, string(c.prefix), string(c.local), v, c})
		}
	}
	visit(li)
	if d := li.child(nsRDF, "Description"); d != nil {
		visit(d)
	}
	return out
}

// items returns the rdf:li of the array of a field, and the array type
// (Bag, Seq, Alt), or "" when the field holds no array.
func items(n *node) (string, []*node) {
	if n == nil || len(n.children) != 1 || n.children[0].space != nsRDF {
		return "", nil
	}
	arr := n.children[0]
	var lis []*node
	for _, c := range arr.children {
		if c.is(nsRDF, "li") {
			lis = append(lis, c)
		}
	}
	return string(arr.local), lis
}

// checkSchemas checks the PDF/A extension schema container (6.6.2.3.3) and
// records the properties it describes. Whether a value type is defined is
// not checked: veraPDF admits an undescribed one (test 6-6-2-3-2-t01-pass-d
// of its corpus).
func checkSchemas(data []byte, schemas *node, own Declared, report func(IssueKind, string, ...any)) {
	if string(schemas.prefix) != "pdfaExtension" {
		report(IssueExtension, "the extension schema container has the prefix %q; PDF/A requires pdfaExtension", schemas.prefix)
	}
	kind, lis := items(schemas)
	if kind != "Bag" {
		report(IssueExtension, "pdfaExtension:schemas is not a Bag")
	}
	type desc struct {
		where, prefix string
		fields        []field
	}
	var props []desc // properties, value types and fields
	for i, li := range lis {
		where := fmt.Sprintf("extension schema %d", i+1)
		fs := structFields(data, li)
		present := map[string]bool{}
		var uri string
		for _, f := range fs {
			if f.space != nsPDFASchema {
				report(IssueExtension, "%s has the field %s, which the PDF/A Schema value type does not define", where, qnameOf([]byte(f.prefix), []byte(f.local)))
				continue
			}
			if f.prefix != "pdfaSchema" {
				report(IssueExtension, "%s: the field %s has the prefix %q; PDF/A requires pdfaSchema", where, f.local, f.prefix)
			}
			present[f.local] = true
			switch f.local {
			case "schema", "prefix":
				if f.node != nil && len(f.node.children) > 0 {
					report(IssueExtension, "%s: pdfaSchema:%s is not a Text value", where, f.local)
				}
			case "namespaceURI":
				uri = f.value
			case "property", "valueType":
				k, sub := items(f.node)
				if k != "Seq" {
					report(IssueExtension, "%s: pdfaSchema:%s is not a Seq", where, f.local)
				}
				for j, pli := range sub {
					pw := fmt.Sprintf("%s, %s %d", where, f.local, j+1)
					pfs := structFields(data, pli)
					if f.local == "property" {
						props = append(props, desc{where: pw, prefix: "pdfaProperty", fields: pfs})
						for _, pf := range pfs {
							if pf.space == nsPDFAProperty && pf.local == "name" {
								if own[uri] == nil {
									own[uri] = map[string]bool{}
								}
								own[uri][pf.value] = true
							}
						}
						continue
					}
					props = append(props, desc{where: pw, prefix: "pdfaType", fields: pfs})
					for _, pf := range pfs {
						if pf.space == nsPDFAType && pf.local == "field" {
							_, flis := items(pf.node)
							for m, fli := range flis {
								props = append(props, desc{where: fmt.Sprintf("%s, field %d", pw, m+1), prefix: "pdfaField", fields: structFields(data, fli)})
							}
						}
					}
				}
			default:
				report(IssueExtension, "%s has the field pdfaSchema:%s, which the PDF/A Schema value type does not define", where, f.local)
			}
		}
		for _, req := range []string{"schema", "namespaceURI", "prefix"} {
			if !present[req] {
				report(IssueExtension, "%s has no pdfaSchema:%s", where, req)
			}
		}
	}

	spaces := map[string]string{"pdfaProperty": nsPDFAProperty, "pdfaType": nsPDFAType, "pdfaField": nsPDFAField}
	required := map[string][]string{
		"pdfaProperty": {"name", "valueType", "category", "description"},
		"pdfaType":     {"type", "namespaceURI", "prefix", "description"},
		"pdfaField":    {"name", "valueType", "description"},
	}
	defined := map[string]map[string]bool{
		"pdfaProperty": {"name": true, "valueType": true, "category": true, "description": true},
		"pdfaType":     {"type": true, "namespaceURI": true, "prefix": true, "description": true, "field": true},
		"pdfaField":    {"name": true, "valueType": true, "description": true},
	}
	for _, d := range props {
		ns := spaces[d.prefix]
		present := map[string]bool{}
		for _, f := range d.fields {
			if f.space != ns || !defined[d.prefix][f.local] {
				report(IssueExtension, "%s has the field %s, which the PDF/A value type does not define", d.where, qnameOf([]byte(f.prefix), []byte(f.local)))
				continue
			}
			if f.prefix != d.prefix {
				report(IssueExtension, "%s: the field %s has the prefix %q; PDF/A requires %s", d.where, f.local, f.prefix, d.prefix)
			}
			present[f.local] = true
			switch {
			case f.local == "category" && f.value != "external" && f.value != "internal":
				report(IssueExtension, "%s: pdfaProperty:category is %q, not external or internal", d.where, f.value)
			case f.local == "field":
				if k, _ := items(f.node); k != "Seq" {
					report(IssueExtension, "%s: pdfaType:field is not a Seq", d.where)
				}
			}
		}
		for _, req := range required[d.prefix] {
			if !present[req] {
				report(IssueExtension, "%s has no %s:%s", d.where, d.prefix, req)
			}
		}
	}
}
