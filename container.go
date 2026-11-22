package zerechnung

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/internal/xmp"
	"github.com/ritoon/zerechnung/invoice"
)

// Errors of the PDF container layer.
var (
	// ErrNoInvoice is returned by Extract when the PDF has no invoice XML
	// attachment.
	ErrNoInvoice = errors.New("zerechnung: no invoice XML attachment in the PDF")
	// ErrEncrypted is returned for encrypted PDFs, which PDF/A forbids.
	ErrEncrypted = pdf.ErrEncrypted
	// ErrNotPDFA is wrapped by the error of Embed and Create, with
	// EmbedOptions.RequirePDFA, when the host PDF does not declare PDF/A-2,
	// PDF/A-3 or PDF/A-4.
	ErrNotPDFA = errors.New("zerechnung: the PDF does not declare PDF/A-2, PDF/A-3 or PDF/A-4")
)

// XMP namespaces of the hybrid-invoice schema.
const (
	// NamespaceFacturX is used by Factur-X, ZUGFeRD 2.1 and later, and
	// XRechnung hybrids.
	NamespaceFacturX = xmp.NSFacturX
	// NamespaceZUGFeRD2 is used by ZUGFeRD 2.0.
	NamespaceZUGFeRD2 = xmp.NSZUGFeRD2
	// NamespaceZUGFeRD1 is used by ZUGFeRD 1.0.
	NamespaceZUGFeRD1 = xmp.NSZUGFeRD1
	// NamespaceZUGFeRD1RC is used by files made with the ZUGFeRD 1.0
	// release candidate (2014).
	NamespaceZUGFeRD1RC = xmp.NSZUGFeRD1RC
)

// Attachment is a file embedded in a PDF, normally the invoice XML.
type Attachment struct {
	Filename     string    // /UF (or /F) of the file specification
	XML          []byte    // decoded content of the embedded file
	Relationship string    // /AFRelationship ("Alternative", "Data", "Source"...), "" when absent
	MimeType     string    // /Subtype of the embedded file, decoded ("text/xml")
	Description  string    // /Desc of the file specification
	ModDate      time.Time // /Params /ModDate; zero when absent or unreadable
	InAF         bool      // listed in the /AF array of the document catalog
	Metadata     *Metadata // XMP metadata of the document; nil when absent or unreadable
	PDFVersion   string    // PDF version: the header's, or the catalog's /Version when later
}

// Metadata is what the XMP metadata of the document says about the hybrid
// invoice.
type Metadata struct {
	// NamespaceURI is the hybrid-invoice namespace found: NamespaceFacturX,
	// NamespaceZUGFeRD2, NamespaceZUGFeRD1 (or NamespaceZUGFeRD1RC); "" when
	// the XMP has no hybrid-invoice property.
	NamespaceURI     string
	DocumentType     string // "INVOICE"
	DocumentFileName string // "factur-x.xml"
	Version          string // "1.0"
	ConformanceLevel string // "EN 16931"
	PDFAPart         int    // pdfaid:part; 0 when absent or not a number
	PDFAConformance  string // pdfaid:conformance ("B"; "F" or "E" for PDF/A-4)
	PDFARev          string // pdfaid:rev, the year of the PDF/A-4 edition ("2020"); "" when absent
}

// Attachment names, in order of preference.
var invoiceNames = []string{"factur-x.xml", "xrechnung.xml", "zugferd-invoice.xml"}

// nameRank returns the preference of an attachment name (lower is better),
// -1 for names that are not invoice names, and -2 for order-x.xml.
func nameRank(name string) int {
	for i, n := range invoiceNames {
		if strings.EqualFold(name, n) {
			return i
		}
	}
	if strings.EqualFold(name, "order-x.xml") {
		return -2
	}
	return -1
}

// fileSpec is an embedded file found in the document.
type fileSpec struct {
	ref      pdf.Ref // zero for a direct dictionary
	dict     pdf.Dict
	name     string // /UF, /F... of the file specification
	treeName string // key in the EmbeddedFiles name tree
	inTree   bool
	inAF     bool // listed in the /AF array of the catalog
	inPageAF bool // listed in the /AF array of a page
}

func (fs *fileSpec) filename() string {
	if fs.name != "" {
		return fs.name
	}
	return fs.treeName
}

func (fs *fileSpec) rank() int {
	r := nameRank(fs.name)
	if r == -1 {
		return nameRank(fs.treeName)
	}
	return r
}

// container gives access to the embedded files of a PDF.
type container struct {
	r       *pdf.Reader
	catalog pdf.Dict
	specs   []*fileSpec
	pages   bool // page-level /AF arrays were collected
}

func openContainer(data []byte) (*container, error) {
	r, err := pdf.Open(data)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	c := &container{r: r, catalog: r.Catalog()}
	c.collect()
	return c, nil
}

// specName returns the file name of a file specification.
func specName(r *pdf.Reader, d pdf.Dict) string {
	for _, k := range []pdf.Name{"UF", "F", "Unix", "Mac", "DOS"} {
		if s, ok := r.Resolve(d.Get(k)).(pdf.String); ok && len(s) > 0 {
			return pdf.Text(s)
		}
	}
	return ""
}

// add records a file specification, merging duplicates.
func (c *container) add(o pdf.Object, treeName string, inTree, inAF, inPageAF bool) {
	ref, _ := o.(pdf.Ref)
	d, ok := c.r.Resolve(o).(pdf.Dict)
	if !ok {
		return
	}
	for _, fs := range c.specs {
		if (ref != pdf.Ref{} && fs.ref == ref) || (ref == pdf.Ref{} && sameDict(fs.dict, d)) {
			fs.inTree = fs.inTree || inTree
			fs.inAF = fs.inAF || inAF
			fs.inPageAF = fs.inPageAF || inPageAF
			if fs.treeName == "" {
				fs.treeName = treeName
			}
			return
		}
	}
	if !d.Has("EF") {
		return // not an embedded file
	}
	c.specs = append(c.specs, &fileSpec{ref: ref, dict: d, name: specName(c.r, d), treeName: treeName, inTree: inTree, inAF: inAF, inPageAF: inPageAF})
}

// sameDict reports whether two direct dictionaries are the same object.
func sameDict(a, b pdf.Dict) bool {
	return len(a) > 0 && len(a) == len(b) && &a[0] == &b[0]
}

// collect gathers the embedded files of the EmbeddedFiles name tree and of
// the catalog /AF array.
func (c *container) collect() {
	if names, ok := c.r.Dict(c.catalog.Get("Names")); ok {
		c.walkNameTree(names.Get("EmbeddedFiles"), 0, func(key pdf.String, val pdf.Object) {
			c.add(val, pdf.Text(key), true, false, false)
		})
	}
	if af, ok := c.r.Resolve(c.catalog.Get("AF")).(pdf.Array); ok {
		for _, o := range af {
			c.add(o, "", false, true, false)
		}
	}
}

// collectPages adds the embedded files of page-level /AF arrays.
func (c *container) collectPages() {
	if c.pages {
		return
	}
	c.pages = true
	seen := map[pdf.Ref]bool{}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		if ref, ok := o.(pdf.Ref); ok {
			if seen[ref] {
				return
			}
			seen[ref] = true
		}
		d, ok := c.r.Dict(o)
		if !ok || depth > 32 || len(seen) > 10000 {
			return
		}
		if af, ok := c.r.Resolve(d.Get("AF")).(pdf.Array); ok {
			for _, x := range af {
				c.add(x, "", false, false, true)
			}
		}
		if kids, ok := c.r.Resolve(d.Get("Kids")).(pdf.Array); ok {
			for _, k := range kids {
				walk(k, depth+1)
			}
		}
	}
	walk(c.catalog.Get("Pages"), 0)
}

// walkNameTree calls fn for each leaf of a name tree.
func (c *container) walkNameTree(o pdf.Object, depth int, fn func(pdf.String, pdf.Object)) {
	node, ok := c.r.Dict(o)
	if !ok || depth > 32 {
		return
	}
	if names, ok := c.r.Resolve(node.Get("Names")).(pdf.Array); ok {
		for i := 0; i+1 < len(names); i += 2 {
			if key, ok := c.r.Resolve(names[i]).(pdf.String); ok {
				fn(key, names[i+1])
			}
		}
	}
	if kids, ok := c.r.Resolve(node.Get("Kids")).(pdf.Array); ok {
		for _, k := range kids {
			c.walkNameTree(k, depth+1, fn)
		}
	}
}

// embeddedStream returns the embedded file stream of a file specification.
func (c *container) embeddedStream(fs *fileSpec) (*pdf.Stream, bool) {
	ef, ok := c.r.Dict(fs.dict.Get("EF"))
	if !ok {
		return nil, false
	}
	for _, k := range []pdf.Name{"F", "UF"} {
		if st, ok := c.r.Resolve(ef.Get(k)).(*pdf.Stream); ok {
			return st, true
		}
	}
	return nil, false
}

// content decodes the embedded file of fs. The result never aliases the
// PDF.
func (c *container) content(fs *fileSpec) ([]byte, *pdf.Stream, error) {
	st, ok := c.embeddedStream(fs)
	if !ok {
		return nil, nil, fmt.Errorf("zerechnung: attachment %q has no embedded file stream", fs.filename())
	}
	hint := 0
	if params, ok := c.r.Dict(st.Dict.Get("Params")); ok {
		if n, ok := pdf.AsInt(c.r.Resolve(params.Get("Size"))); ok && n > 0 && n < 1<<30 {
			hint = int(n)
		}
	}
	if hint == 0 {
		if n, ok := pdf.AsInt(c.r.Resolve(st.Dict.Get("DL"))); ok && n > 0 && n < 1<<30 {
			hint = int(n)
		}
	}
	data, err := c.r.DecodeSize(st, hint)
	if err != nil {
		return nil, st, fmt.Errorf("zerechnung: attachment %q: %w", fs.filename(), err)
	}
	if len(data) > 0 && len(st.Raw) > 0 && &data[0] == &st.Raw[0] {
		data = bytes.Clone(data)
	}
	return data, st, nil
}

// invoice finds the invoice attachment: by name first (factur-x.xml,
// xrechnung.xml, zugferd-invoice.xml), then the first XML attachment whose
// root is a CII invoice. order-x.xml is ignored.
func (c *container) invoice() (*fileSpec, []byte, *pdf.Stream, error) {
	for pass := 0; pass < 2; pass++ {
		if pass == 1 {
			c.collectPages()
		}
		var best *fileSpec
		for _, fs := range c.specs {
			if r := fs.rank(); r >= 0 && (best == nil || r < best.rank()) {
				best = fs
			}
		}
		if best != nil {
			data, st, err := c.content(best)
			return best, data, st, err
		}
		for _, fs := range c.specs {
			if fs.rank() == -2 {
				continue
			}
			data, st, err := c.content(fs)
			if err != nil {
				continue
			}
			switch xmlRoot(data) {
			case "CrossIndustryInvoice", "CrossIndustryDocument":
				return fs, data, st, nil
			}
		}
	}
	return nil, nil, nil, ErrNoInvoice
}

// attachment builds the public view of an embedded file.
func (c *container) attachment(fs *fileSpec, data []byte, st *pdf.Stream, meta *Metadata) *Attachment {
	a := &Attachment{
		Filename:   fs.filename(),
		XML:        data,
		InAF:       fs.inAF,
		Metadata:   meta,
		PDFVersion: c.version(),
	}
	if rel, ok := c.r.Resolve(fs.dict.Get("AFRelationship")).(pdf.Name); ok {
		a.Relationship = string(rel)
	}
	if d, ok := c.r.Resolve(fs.dict.Get("Desc")).(pdf.String); ok {
		a.Description = pdf.Text(d)
	}
	if st != nil {
		if sub, ok := c.r.Resolve(st.Dict.Get("Subtype")).(pdf.Name); ok {
			a.MimeType = string(sub)
		}
		if params, ok := c.r.Dict(st.Dict.Get("Params")); ok {
			if s, ok := c.r.Resolve(params.Get("ModDate")).(pdf.String); ok {
				if t, ok := pdf.ParseDate(pdf.Text(s)); ok {
					a.ModDate = t
				}
			}
		}
	}
	return a
}

// version returns the PDF version: the header's, or the catalog's /Version
// when later.
func (c *container) version() string {
	v := c.r.Version()
	if cv, ok := c.r.Resolve(c.catalog.Get("Version")).(pdf.Name); ok && versionLess(v, string(cv)) {
		return string(cv)
	}
	return v
}

func versionLess(a, b string) bool {
	am, an, _ := strings.Cut(a, ".")
	bm, bn, _ := strings.Cut(b, ".")
	ai, _ := strconv.Atoi(am)
	bi, _ := strconv.Atoi(bm)
	if ai != bi {
		return ai < bi
	}
	aj, _ := strconv.Atoi(an)
	bj, _ := strconv.Atoi(bn)
	return aj < bj
}

// xmpPacket returns the decoded XMP metadata stream of the catalog.
func (c *container) xmpPacket() ([]byte, bool) {
	st, ok := c.r.Resolve(c.catalog.Get("Metadata")).(*pdf.Stream)
	if !ok {
		return nil, false
	}
	data, err := c.r.Decode(st)
	if err != nil {
		return nil, false
	}
	return data, true
}

// xmpInfo parses the XMP metadata. present reports whether a metadata
// stream exists.
func (c *container) xmpInfo() (info *xmp.Info, present bool, err error) {
	p, present := c.xmpParsed()
	if p == nil {
		return nil, present, nil
	}
	info, err = p.Info()
	return info, true, err
}

// xmpParsed reads the XMP metadata stream, nil when there is none or it
// cannot be decoded. present reports whether a metadata stream exists.
func (c *container) xmpParsed() (p *xmp.Packet, present bool) {
	data, ok := c.xmpPacket()
	if !ok {
		return nil, c.catalog.Has("Metadata")
	}
	return xmp.ParsePacket(data), true
}

// requirePDFA returns an error wrapping ErrNotPDFA when the XMP metadata of
// the document does not declare PDF/A-2, PDF/A-3 or PDF/A-4 with a
// conformance level of that part: A, B or U for PDF/A-2 and PDF/A-3; none,
// F or E for PDF/A-4. Embed declares PDF/A-3 on a PDF/A-2 host and
// PDF/A-4f on a plain PDF/A-4 host, which their rules allow.
func (c *container) requirePDFA() error {
	info, present, err := c.xmpInfo()
	switch {
	case !present:
		return fmt.Errorf("%w: the document catalog has no XMP metadata stream", ErrNotPDFA)
	case err != nil:
		return fmt.Errorf("%w: the XMP metadata cannot be read: %v", ErrNotPDFA, err)
	case info == nil:
		return fmt.Errorf("%w: the XMP metadata stream cannot be decoded", ErrNotPDFA)
	}
	part, conf := strings.TrimSpace(info.PDFAPart), strings.TrimSpace(info.PDFAConformance)
	switch {
	case part == "":
		return fmt.Errorf("%w: the XMP metadata has no pdfaid:part", ErrNotPDFA)
	case part != "2" && part != "3" && part != "4":
		return fmt.Errorf("%w: pdfaid:part is %q", ErrNotPDFA, part)
	case part == "4" && conf != "" && conf != "F" && conf != "E":
		return fmt.Errorf("%w: pdfaid:conformance is %q; PDF/A-4 has no level, or level F or E", ErrNotPDFA, conf)
	case part != "4" && conf != "A" && conf != "B" && conf != "U":
		return fmt.Errorf("%w: pdfaid:conformance is %q; PDF/A-%s has levels A, B and U", ErrNotPDFA, conf, part)
	}
	return nil
}

func metadataOf(info *xmp.Info) *Metadata {
	if info == nil {
		return nil
	}
	m := &Metadata{PDFAConformance: strings.TrimSpace(info.PDFAConformance), PDFARev: strings.TrimSpace(info.PDFARev)}
	m.PDFAPart, _ = strconv.Atoi(strings.TrimSpace(info.PDFAPart))
	if inv := info.Invoice; inv != nil {
		m.NamespaceURI = inv.NamespaceURI
		m.DocumentType = strings.TrimSpace(inv.DocumentType)
		m.DocumentFileName = strings.TrimSpace(inv.DocumentFileName)
		m.Version = strings.TrimSpace(inv.Version)
		m.ConformanceLevel = strings.TrimSpace(inv.ConformanceLevel)
	}
	return m
}

// Extract returns the invoice XML attachment of a Factur-X, ZUGFeRD or
// XRechnung hybrid PDF, with its file specification details and the XMP
// metadata of the document.
//
// The attachment is looked up in the EmbeddedFiles name tree and the /AF
// array of the catalog (then of the pages), by name — factur-x.xml,
// xrechnung.xml, zugferd-invoice.xml or ZUGFeRD-invoice.xml, order-x.xml
// being ignored — and failing that as the first attachment whose XML root is
// a CrossIndustryInvoice or CrossIndustryDocument. The returned XML is the
// decoded embedded file, byte for byte.
//
// Extract returns ErrNoInvoice when there is no such attachment and
// ErrEncrypted for encrypted PDFs.
func Extract(pdfData []byte) (*Attachment, error) {
	c, err := openContainer(pdfData)
	if err != nil {
		return nil, err
	}
	fs, data, st, err := c.invoice()
	if err != nil {
		return nil, err
	}
	info, _, _ := c.xmpInfo()
	return c.attachment(fs, data, st, metadataOf(info)), nil
}

// ExtractAll returns every embedded file of the PDF referenced by the
// EmbeddedFiles name tree or an /AF array, invoice or not, in document
// order. The XML field of each attachment holds its decoded content.
func ExtractAll(pdfData []byte) ([]*Attachment, error) {
	c, err := openContainer(pdfData)
	if err != nil {
		return nil, err
	}
	c.collectPages()
	info, _, _ := c.xmpInfo()
	meta := metadataOf(info)
	out := make([]*Attachment, 0, len(c.specs))
	for _, fs := range c.specs {
		data, st, err := c.content(fs)
		if err != nil {
			return out, err
		}
		out = append(out, c.attachment(fs, data, st, meta))
	}
	return out, nil
}

// xmlRoot returns the local name of the root element of an XML document,
// or "" when none is found.
func xmlRoot(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	for {
		i := bytes.IndexByte(b, '<')
		if i < 0 || i+1 >= len(b) {
			return ""
		}
		b = b[i:]
		switch {
		case bytes.HasPrefix(b, []byte("<?")):
			j := bytes.Index(b, []byte("?>"))
			if j < 0 {
				return ""
			}
			b = b[j+2:]
		case bytes.HasPrefix(b, []byte("<!--")):
			j := bytes.Index(b, []byte("-->"))
			if j < 0 {
				return ""
			}
			b = b[j+3:]
		case bytes.HasPrefix(b, []byte("<!")):
			j := bytes.IndexByte(b, '>')
			if j < 0 {
				return ""
			}
			b = b[j+1:]
		default:
			j := 1
			for j < len(b) && !isXMLNameEnd(b[j]) {
				j++
			}
			name := b[1:j]
			if k := bytes.IndexByte(name, ':'); k >= 0 {
				name = name[k+1:]
			}
			return string(name)
		}
	}
}

func isXMLNameEnd(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '/' || c == '>'
}

// Paths of BT-24 from the root element: Factur-X and ZUGFeRD 2
// (CrossIndustryInvoice), ZUGFeRD 1.0 (CrossIndustryDocument).
const (
	bt24Path    = "ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/ID"
	bt24PathZF1 = "SpecifiedExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/ID"
)

// specificationID reads BT-24
// (rsm:ExchangedDocumentContext/ram:GuidelineSpecifiedDocumentContextParameter/ram:ID)
// and the local name of the root element. It scans the start of the
// document only, and falls back to a full parse with internal/xmltree when
// the quick scan does not find the identifier or cannot read it the way
// xmltree does.
func specificationID(doc []byte) (root, id string) {
	if root, id, ok := scanSpecificationID(doc); ok {
		return root, id
	}
	t, err := xmltree.Parse(doc)
	if err != nil {
		return xmlRoot(doc), ""
	}
	id = t.Value(bt24Path)
	if id == "" {
		id = t.Value(bt24PathZF1)
	}
	return t.Name, id
}

// scanSpecificationID finds BT-24 by tracking the element path, without
// building a tree. ok is false when the identifier is not found before the
// document context element ends, and whenever the scan could read something
// else than xmltree: a DOCTYPE or a CDATA section, an XML declaration of
// another encoding than UTF-8, or BT-24 empty or holding markup, character
// references or carriage returns. Ordinary invoices hold none of these.
func scanSpecificationID(b []byte) (root, id string, ok bool) {
	var path [4][]byte // local names of the open elements at depth 1 to 4
	depth := 0
	textStart := -1
	i := 0
	for {
		k := bytes.IndexByte(b[i:], '<')
		if k < 0 {
			return root, "", false
		}
		i += k
		rest := b[i:]
		if textStart >= 0 && !bytes.HasPrefix(rest, []byte("</")) {
			// An element, a comment or a processing instruction inside
			// BT-24: xmltree joins the pieces of text around it.
			return root, "", false
		}
		switch {
		case bytes.HasPrefix(rest, []byte("<?")):
			j := bytes.Index(rest[2:], []byte("?>"))
			if j < 0 || !keepsUTF8(rest[2:2+j]) {
				return root, "", false
			}
			i += j + 4
		case bytes.HasPrefix(rest, []byte("<!--")):
			j := bytes.Index(rest[4:], []byte("-->"))
			if j < 0 {
				return root, "", false
			}
			i += j + 7
		case bytes.HasPrefix(rest, []byte("<!")):
			// A DOCTYPE or a CDATA section may hold '<' and '>' that are
			// not markup: leave them to xmltree.
			return root, "", false
		case bytes.HasPrefix(rest, []byte("</")):
			j := bytes.IndexByte(rest, '>')
			if j < 0 {
				return root, "", false
			}
			if textStart >= 0 {
				id, ok := bt24Text(b[textStart:i])
				return root, id, ok
			}
			if depth == 2 && isContext(path[1]) {
				return root, "", false // context closed without BT-24
			}
			if depth == 0 {
				return root, "", false // end tag without a start tag: not well-formed
			}
			depth--
			i += j + 1
		default:
			j := 1
			for j < len(rest) && !isXMLNameEnd(rest[j]) {
				j++
			}
			name := localName(rest[1:j])
			e := j // no attribute: '>' right after the name
			if j >= len(rest) || rest[j] != '>' {
				if e = tagEnd(rest[j:]); e < 0 {
					return root, "", false
				}
				e += j
			}
			self := rest[e-1] == '/'
			depth++
			if depth == 1 {
				root = string(name)
			}
			if depth <= 4 {
				path[depth-1] = name
			}
			if depth == 4 && isContext(path[1]) && string(path[2]) == "GuidelineSpecifiedDocumentContextParameter" && string(name) == "ID" {
				if self {
					return root, "", false // empty BT-24
				}
				textStart = i + e + 1
			}
			if self {
				depth--
			}
			i += e + 1
		}
	}
}

func isContext(name []byte) bool {
	return string(name) == "ExchangedDocumentContext" || string(name) == "SpecifiedExchangedDocumentContext"
}

// localName strips the prefix of an element name as xmltree does: a colon
// that starts or ends the name belongs to the local name.
func localName(name []byte) []byte {
	if c := bytes.IndexByte(name, ':'); c > 0 && c < len(name)-1 {
		return name[c+1:]
	}
	return name
}

// tagEnd returns the index of the '>' that ends a start tag, b following
// its name, or -1. Attribute values may hold '>': they are skipped.
func tagEnd(b []byte) int {
	e := bytes.IndexByte(b, '>')
	for i := 0; e >= 0; {
		q := e // first quote before the '>', if any
		if k := bytes.IndexByte(b[i:e], '"'); k >= 0 {
			q = i + k
		}
		if k := bytes.IndexByte(b[i:q], '\''); k >= 0 {
			q = i + k
		}
		if q == e {
			return e
		}
		c := bytes.IndexByte(b[q+1:], b[q])
		if c < 0 {
			return -1
		}
		i = q + 1 + c + 1
		if i > e { // the '>' was in the value
			if e = bytes.IndexByte(b[i:], '>'); e >= 0 {
				e += i
			}
		}
	}
	return -1
}

// keepsUTF8 reports whether a processing instruction (its content between
// "<?" and "?>") leaves the document in UTF-8: xmltree converts the rest of
// the input when an XML declaration names ISO-8859-1 or windows-1252. The
// first encoding="..." found decides, as for xmltree; any doubt answers
// false.
func keepsUTF8(pi []byte) bool {
	_, v, found := bytes.Cut(pi, []byte("encoding="))
	if !found {
		return true
	}
	if len(v) == 0 || v[0] != '"' && v[0] != '\'' {
		return false
	}
	enc, _, closed := bytes.Cut(v[1:], v[:1])
	return closed && bytes.EqualFold(enc, []byte("utf-8"))
}

// bt24Text decodes the text of BT-24 as xmltree does: trimmed, with the
// predefined entities expanded. ok is false when the text is empty (the full
// parse then also tries the ZUGFeRD 1.0 path), holds a character reference,
// or a carriage return, which xmltree turns into a line feed.
func bt24Text(raw []byte) (id string, ok bool) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Contains(t, []byte("&#")) || bytes.IndexByte(t, '\r') >= 0 {
		return "", false
	}
	return unescapeXML(t), true
}

func unescapeXML(b []byte) string {
	if bytes.IndexByte(b, '&') < 0 {
		return string(b)
	}
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&quot;", "\"", "&apos;", "'")
	return r.Replace(string(b))
}

// EmbedOptions tune Embed. The zero value (or nil) is ready to use.
type EmbedOptions struct {
	// Profile of the invoice. When unset, it is read from BT-24 of the XML;
	// Embed fails when the identifier is unknown.
	Profile invoice.Profile
	// Filename of the attachment; Profile.Filename() when empty
	// ("factur-x.xml", or "xrechnung.xml" for XRECHNUNG).
	Filename string
	// Relationship is the /AFRelationship: "Data", "Alternative" or
	// "Source"; Profile.AFRelationship() when empty, which suits both France
	// and Germany. Embed refuses a value that does not fit the profile or
	// the seller's country (BT-40), as Factur-X 1.09 (6.2.2) tabulates them:
	// MINIMUM and BASIC WL take Data; XRECHNUNG, and BASIC and above for a
	// seller in Germany, take Alternative; BASIC and above take Alternative,
	// Source or Data otherwise (France admits all three).
	Relationship string
	// Description of the attachment (/Desc).
	Description string
	// ModDate is the modification date of the attachment and of the
	// document; time.Now() when zero. Set it for deterministic output.
	ModDate time.Time
	// Version is fx:Version; "1.0" when empty.
	Version string
	// DocumentType is fx:DocumentType; "INVOICE" when empty.
	DocumentType string
	// Attachments are supplementary files embedded besides the invoice: a
	// purchase order, a timesheet, a delivery note...
	Attachments []File
	// RequirePDFA makes Embed refuse a host PDF whose XMP metadata does not
	// declare PDF/A-2, PDF/A-3 or PDF/A-4 (pdfaid:part 2, 3 or 4, with a
	// conformance level of that part: A, B or U for PDF/A-2 and PDF/A-3;
	// none, F or E for PDF/A-4), instead of declaring PDF/A-3 or PDF/A-4f on
	// a document that may not conform. The error wraps ErrNotPDFA. Only the
	// declaration is read: CheckContainer reports the PDF/A requirements it
	// can verify without rendering, and a PDF/A validator such as veraPDF
	// checks the rest.
	RequirePDFA bool
	// ConvertPDFA converts a host PDF whose XMP metadata does not declare
	// PDF/A-2, PDF/A-3 or PDF/A-4 with ConvertPDFA before embedding the
	// invoice; a host that declares it is kept as is. When the conversion
	// fails, Embed fails with its error, which wraps ErrNotConvertible.
	// ModDate is the date of the conversion. RequirePDFA is then met by
	// the converted host.
	ConvertPDFA bool
	// Sign, when set, signs the result with Sign: the PAdES signature
	// then covers the PDF and the embedded invoice. A host whose
	// certification signature (DocMDP) forbids the change is refused
	// whatever Sign (see Embed).
	Sign *SignOptions
}

// File is a supplementary file that Embed attaches to the PDF besides the
// invoice.
type File struct {
	// Name of the file (/F and /UF of the file specification); required.
	// The invoice names (factur-x.xml, xrechnung.xml, zugferd-invoice.xml)
	// and EmbedOptions.Filename are refused, whatever the case.
	Name string
	// MimeType is the MIME type of the file (/Subtype of the embedded file
	// stream), such as "application/pdf". When empty, it is guessed from the
	// extension of Name (pdf, xml, csv, txt, json, png, jpg, gif, tif, svg,
	// zip, odt, ods, docx, xlsx, doc, xls), "application/octet-stream"
	// otherwise.
	MimeType string
	// Description of the file (/Desc); none when empty.
	Description string
	// Data is the content of the file.
	Data []byte
	// Relationship is the /AFRelationship of the file: "Supplement" when
	// empty, or "Data", "Source", "Alternative", "Unspecified".
	Relationship string
	// ModDate is the modification date of the file (/Params /ModDate);
	// EmbedOptions.ModDate when zero.
	ModDate time.Time
}

// mimeTypes maps file extensions to the MIME type of supplementary files.
// mime.TypeByExtension is not used: it reads the tables of the system, which
// would make the output depend on the machine.
var mimeTypes = map[string]string{
	".pdf":  "application/pdf",
	".xml":  "text/xml",
	".csv":  "text/csv",
	".txt":  "text/plain",
	".json": "application/json",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".svg":  "image/svg+xml",
	".zip":  "application/zip",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".doc":  "application/msword",
	".xls":  "application/vnd.ms-excel",
}

// mimeType guesses the MIME type of a file from the extension of its name.
func mimeType(name string) string {
	if t, ok := mimeTypes[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

// validMIME reports whether s is a MIME type without parameters:
// type/subtype, made of the characters RFC 6838 allows in names.
func validMIME(s string) bool {
	typ, sub, ok := strings.Cut(s, "/")
	return ok && mimeName(typ) && mimeName(sub)
}

func mimeName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("!#$&-^_.+", c) >= 0) {
			return false
		}
	}
	return true
}

// supplements checks the supplementary files of Embed and fills in their
// defaults. invoiceName is the name of the invoice attachment.
func supplements(files []File, invoiceName string, date time.Time) ([]File, error) {
	out := make([]File, len(files))
	for i, f := range files {
		switch {
		case f.Name == "":
			return nil, fmt.Errorf("zerechnung: attachment %d has no name", i+1)
		case nameRank(f.Name) >= 0 || strings.EqualFold(f.Name, invoiceName):
			return nil, fmt.Errorf("zerechnung: attachment name %q is reserved for the invoice", f.Name)
		}
		for _, g := range out[:i] {
			if strings.EqualFold(g.Name, f.Name) {
				return nil, fmt.Errorf("zerechnung: two attachments are named %q", f.Name)
			}
		}
		if f.Relationship == "" {
			f.Relationship = "Supplement"
		}
		switch f.Relationship {
		case "Supplement", "Data", "Source", "Alternative", "Unspecified":
		default:
			return nil, fmt.Errorf("zerechnung: attachment %q: AFRelationship %q not allowed (Supplement, Data, Source, Alternative or Unspecified)", f.Name, f.Relationship)
		}
		if f.MimeType == "" {
			f.MimeType = mimeType(f.Name)
		} else if !validMIME(f.MimeType) {
			return nil, fmt.Errorf("zerechnung: attachment %q: %q is not a MIME type (type/subtype)", f.Name, f.MimeType)
		}
		if f.ModDate.IsZero() {
			f.ModDate = date
		}
		f.ModDate = f.ModDate.Truncate(time.Second)
		out[i] = f
	}
	return out, nil
}

// sellerCountryPath is BT-40 from the root of a CII invoice.
const sellerCountryPath = "SupplyChainTradeTransaction/ApplicableHeaderTradeAgreement/SellerTradeParty/PostalTradeAddress/CountryID"

// checkRelationship refuses the /AFRelationship of the invoice that
// Factur-X 1.09 (5.3 and 6.2.2) does not admit for its profile and the
// seller's country: Data is the only value for MINIMUM and BASIC WL;
// Alternative is the only value for XRECHNUNG and, in Germany, for BASIC and
// above; France admits Alternative, Source and Data for BASIC and above. The
// XML is parsed for the seller's country (BT-40) only when it matters, that
// is for Data or Source with BASIC and above; an XML that cannot be parsed
// gives no country.
func checkRelationship(rel string, p invoice.Profile, xml []byte) error {
	switch rel {
	case "Data", "Alternative", "Source":
	default:
		return fmt.Errorf("zerechnung: AFRelationship %q not allowed (Data, Alternative or Source)", rel)
	}
	switch {
	case p == invoice.ProfileMinimum || p == invoice.ProfileBasicWL:
		if rel != "Data" {
			return fmt.Errorf("zerechnung: AFRelationship %s not allowed for profile %s: the XML is not a full invoice, the relationship is Data", rel, p)
		}
	case rel == "Alternative":
	case p == invoice.ProfileXRechnung:
		return fmt.Errorf("zerechnung: AFRelationship %s not allowed for profile %s: Germany admits only Alternative", rel, p)
	default:
		if t, err := xmltree.Parse(xml); err == nil && t.Value(sellerCountryPath) == "DE" {
			return fmt.Errorf("zerechnung: AFRelationship %s not allowed for a seller in Germany (BT-40 is DE): Germany admits only Alternative", rel)
		}
	}
	return nil
}

// putFile writes an embedded file stream and its file specification, and
// returns the reference of the file specification and the MD5 checksum of
// data. The description is left out when empty.
func putFile(u *pdf.Update, name, desc, rel, mime string, date time.Time, data []byte) (pdf.Ref, [md5.Size]byte) {
	sum := md5.Sum(data)
	efRef := u.Alloc()
	u.PutStream(efRef, pdf.Dict{
		{Key: "Type", Val: pdf.Name("EmbeddedFile")},
		{Key: "Subtype", Val: pdf.Name(mime)},
		{Key: "Filter", Val: pdf.Name("FlateDecode")},
		{Key: "Params", Val: pdf.Dict{
			{Key: "ModDate", Val: pdf.String(pdf.FormatDate(date))},
			{Key: "Size", Val: pdf.Int(len(data))},
			{Key: "CheckSum", Val: pdf.String(sum[:])},
		}},
	}, pdf.Deflate(data))

	fsRef := u.Alloc()
	fname := pdf.EncodeText(name)
	spec := pdf.Dict{
		{Key: "Type", Val: pdf.Name("Filespec")},
		{Key: "F", Val: fname},
		{Key: "UF", Val: fname},
	}
	if desc != "" {
		spec = append(spec, pdf.KV{Key: "Desc", Val: pdf.EncodeText(desc)})
	}
	spec = append(spec,
		pdf.KV{Key: "AFRelationship", Val: pdf.Name(rel)},
		pdf.KV{Key: "EF", Val: pdf.Dict{{Key: "F", Val: efRef}, {Key: "UF", Val: efRef}}},
	)
	u.Put(fsRef, spec)
	return fsRef, sum
}

// Embed attaches an invoice XML to a PDF as Factur-X requires and returns
// the new PDF. The input is not modified.
//
// The result is an incremental update: the original bytes are kept as they
// are (existing signatures stay valid) and followed by the embedded file, its
// file specification (listed in the catalog /AF array and in the
// EmbeddedFiles name tree), a new version of the catalog and an XMP
// metadata stream merged from the existing one: the PDF/A identification,
// the hybrid-invoice (fx) properties and their PDF/A extension schema. The
// document is declared PDF/A-4f when it is a PDF 2.0 file or already
// declares PDF/A-4 (level E is kept), and PDF/A-3 otherwise, keeping the
// existing conformance level when it is A, B or U (B by default). An
// attachment of the same name and any other invoice attachment
// (factur-x.xml, xrechnung.xml, zugferd-invoice.xml) are replaced: the
// result carries a single invoice. Other attachments are kept.
//
// The entries of the document information dictionary (/Info) that the XMP
// metadata lacks (title, author, subject, keywords, creator, producer,
// creation date) are added to it, and the /Info ModDate follows
// xmp:ModifyDate. A document declared PDF/A-4 keeps its metadata in the XMP
// alone: PDF/A-4 admits a document information dictionary only when the
// catalog has a /PieceInfo entry, and then with ModDate alone (ISO 19005-4,
// 6.1.3). Embed removes /Info from such a document, or reduces it to
// ModDate when the catalog has /PieceInfo.
//
// The supplementary files of EmbedOptions.Attachments are written like the
// invoice: embedded file stream with its MIME type and /Params /ModDate,
// file specification with /F, /UF and /AFRelationship (Supplement by
// default), listed in the EmbeddedFiles name tree and after the invoice in
// the catalog /AF array. An attachment of the host with the same name is
// replaced. Embed fails when a supplementary file has no name, an invoice
// name, the name of another one, an AFRelationship PDF/A-3 does not define
// or a malformed MIME type.
//
// Embed fails when EmbedOptions.Relationship does not fit the profile or,
// for Data and Source, the seller's country (see EmbedOptions).
//
// On a signed host, the earlier signatures stay valid on the revision they
// cover, but the invoice is added after them: VerifySignatures and Check
// report it (SIG-05). Embed refuses a host certified by a DocMDP
// signature, with an error wrapping ErrCertified: no certification level
// permits embedding a file. With EmbedOptions.Sign, the result is signed
// with Sign, after the invoice is embedded.
//
// Embed does not convert the PDF to PDF/A unless EmbedOptions.ConvertPDFA
// is set: fonts, colour spaces and output intents are left as they are, and
// CheckContainer reports what is missing. With EmbedOptions.RequirePDFA,
// Embed refuses a host that does not declare PDF/A-2, PDF/A-3 or PDF/A-4,
// with an error wrapping ErrNotPDFA. With EmbedOptions.ConvertPDFA, such a
// host is first converted to PDF/A-3 by ConvertPDFA. Nor
// does it validate the XML: only BT-24 is read, when Profile is unset, and
// the seller's country (BT-40), when Relationship is Data or Source for
// BASIC and above.
func Embed(pdfData, xml []byte, opts *EmbedOptions) ([]byte, error) {
	var o EmbedOptions
	if opts != nil {
		o = *opts
	}
	if len(xml) == 0 {
		return nil, errors.New("zerechnung: empty XML")
	}
	profile := o.Profile
	if profile == invoice.ProfileUnknown {
		_, id := specificationID(xml)
		profile = invoice.ProfileFromURN(id)
		if profile == invoice.ProfileUnknown {
			if id == "" {
				return nil, errors.New("zerechnung: cannot read the profile: no specification identifier (BT-24) in the XML; set EmbedOptions.Profile")
			}
			return nil, fmt.Errorf("zerechnung: unknown specification identifier (BT-24) %q; set EmbedOptions.Profile", id)
		}
	}
	if o.Filename == "" {
		o.Filename = profile.Filename()
	}
	if o.Relationship == "" {
		o.Relationship = profile.AFRelationship()
	}
	if err := checkRelationship(o.Relationship, profile, xml); err != nil {
		return nil, err
	}
	if o.Description == "" {
		o.Description = "Invoice " + profile.ConformanceLevel() + " (" + o.Filename + ")"
	}
	if o.ModDate.IsZero() {
		o.ModDate = time.Now()
	}
	o.ModDate = o.ModDate.Truncate(time.Second)
	if o.Version == "" {
		o.Version = "1.0"
	}
	if o.DocumentType == "" {
		o.DocumentType = "INVOICE"
	}
	files, err := supplements(o.Attachments, o.Filename, o.ModDate)
	if err != nil {
		return nil, err
	}

	c, err := openContainer(pdfData)
	if err != nil {
		return nil, err
	}
	if o.ConvertPDFA && c.requirePDFA() != nil {
		if pdfData, err = ConvertPDFA(pdfData, &ConvertOptions{ModDate: o.ModDate}); err != nil {
			return nil, err
		}
		if c, err = openContainer(pdfData); err != nil {
			return nil, err
		}
	}
	if o.RequirePDFA {
		if err := c.requirePDFA(); err != nil {
			return nil, err
		}
	}
	if level := docMDP(c.r, c.catalog); level != NoCertification {
		return nil, fmt.Errorf("%w (%s): embedding a file is permitted at no level", ErrCertified, level)
	}
	r := c.r
	u := pdf.NewUpdate(r)
	date := pdf.String(pdf.FormatDate(o.ModDate))

	// Embedded files and their file specifications: the invoice, then the
	// supplementary files.
	fsRef, sum := putFile(u, o.Filename, o.Description, o.Relationship, "text/xml", o.ModDate, xml)
	fname := pdf.EncodeText(o.Filename)
	suppRefs := make([]pdf.Ref, len(files))
	suppSums := make([][md5.Size]byte, len(files))
	for i, f := range files {
		suppRefs[i], suppSums[i] = putFile(u, f.Name, f.Description, f.Relationship, f.MimeType, f.ModDate, f.Data)
	}

	cat := c.catalog.Clone()
	// A hybrid PDF carries a single invoice: the attachment of the same
	// name and any other invoice attachment are replaced, as are the
	// attachments named like a supplementary file.
	replacedName := func(name string) bool {
		if strings.EqualFold(name, o.Filename) || nameRank(name) >= 0 {
			return true
		}
		for _, f := range files {
			if strings.EqualFold(name, f.Name) {
				return true
			}
		}
		return false
	}
	replaced := func(fs *fileSpec) bool {
		return replacedName(fs.name) || (fs.name == "" && replacedName(fs.treeName))
	}

	// EmbeddedFiles name tree, flattened and sorted.
	type leaf struct {
		key pdf.String
		val pdf.Object
	}
	var leaves []leaf
	namesObj := cat.Get("Names")
	names, _ := r.Dict(namesObj)
	c.walkNameTree(names.Get("EmbeddedFiles"), 0, func(key pdf.String, val pdf.Object) {
		if replacedName(pdf.Text(key)) {
			return
		}
		if d, ok := r.Dict(val); ok && replacedName(specName(r, d)) {
			return
		}
		leaves = append(leaves, leaf{key, val})
	})
	leaves = append(leaves, leaf{fname, fsRef})
	for i, f := range files {
		leaves = append(leaves, leaf{pdf.EncodeText(f.Name), suppRefs[i]})
	}
	sort.SliceStable(leaves, func(i, j int) bool { return bytes.Compare(leaves[i].key, leaves[j].key) < 0 })
	arr := make(pdf.Array, 0, 2*len(leaves))
	for _, l := range leaves {
		arr = append(arr, l.key, l.val)
	}
	newNames := names.Clone()
	newNames.Set("EmbeddedFiles", pdf.Dict{{Key: "Names", Val: arr}})
	if ref, ok := namesObj.(pdf.Ref); ok {
		u.Put(ref, newNames)
	} else {
		cat.Set("Names", newNames)
	}

	// Catalog /AF.
	afObj := cat.Get("AF")
	var af pdf.Array
	if old, ok := r.Resolve(afObj).(pdf.Array); ok {
		for _, x := range old {
			if d, ok := r.Dict(x); ok && replacedName(specName(r, d)) {
				continue
			}
			skip := false
			for _, fs := range c.specs {
				if ref, ok := x.(pdf.Ref); ok && fs.ref == ref && replaced(fs) {
					skip = true
				}
			}
			if !skip {
				af = append(af, x)
			}
		}
	}
	af = append(af, fsRef)
	for _, ref := range suppRefs {
		af = append(af, ref)
	}
	if ref, ok := afObj.(pdf.Ref); ok {
		u.Put(ref, af)
	} else {
		cat.Set("AF", af)
	}

	// Document information dictionary.
	infoObj := r.Trailer().Get("Info")
	info, hasInfo := r.Dict(infoObj)
	text := func(k pdf.Name) string {
		if s, ok := r.Resolve(info.Get(k)).(pdf.String); ok {
			return pdf.Text(s)
		}
		return ""
	}
	var di xmp.DocInfo
	if hasInfo {
		di = xmp.DocInfo{Title: text("Title"), Author: text("Author"), Subject: text("Subject"),
			Keywords: text("Keywords"), Creator: text("Creator"), Producer: text("Producer")}
		if t, ok := pdf.ParseDate(text("CreationDate")); ok {
			di.CreationDate = t
		}
	}

	// XMP metadata, merged with the existing packet. A PDF 2.0 file cannot
	// be PDF/A-3 (based on PDF 1.7): it is declared PDF/A-4.
	existing, _ := c.xmpPacket()
	var part string
	if !versionLess(c.version(), "2.0") {
		part = "4"
	}
	packet := xmp.Merge(existing, xmp.Params{
		PDFAPart:         part,
		NamespaceURI:     xmp.NSFacturX,
		DocumentType:     o.DocumentType,
		DocumentFileName: o.Filename,
		Version:          o.Version,
		ConformanceLevel: profile.ConformanceLevel(),
		Date:             o.ModDate,
		DocInfo:          di,
	})
	metaRef, ok := cat.Get("Metadata").(pdf.Ref)
	if !ok {
		metaRef = u.Alloc()
		cat.Set("Metadata", metaRef)
	}
	u.PutStream(metaRef, pdf.Dict{{Key: "Type", Val: pdf.Name("Metadata")}, {Key: "Subtype", Val: pdf.Name("XML")}}, packet)

	// Keep /Info ModDate and xmp:ModifyDate equal. PDF/A-4 (ISO 19005-4,
	// 6.1.3) admits a document information dictionary only when the catalog
	// has a /PieceInfo entry, and then with ModDate alone: the XMP metadata,
	// which has just received its entries, takes its place. For a document
	// declared PDF/A-4, /Info is thus reduced to ModDate when the catalog has
	// /PieceInfo, and removed otherwise.
	trailer := pdf.Dict{}
	var pdfa4 bool
	if mi, err := xmp.Parse(packet); err == nil && mi != nil {
		pdfa4 = strings.TrimSpace(mi.PDFAPart) == "4"
	}
	switch {
	case pdfa4 && !cat.Has("PieceInfo"):
		// The new trailer has no /Info: ISO 32000-2 (7.5.6) has the newest
		// trailer repeat every entry it keeps, and veraPDF reads it alone.
		// Readers that complete it with the older ones, as this package
		// and PDFBox do, would still find an indirect /Info: that object
		// becomes null. A direct /Info of an older trailer stays visible
		// to them.
		if ref, ok := infoObj.(pdf.Ref); ok {
			u.Put(ref, nil)
		}
	case hasInfo && (pdfa4 || info.Has("ModDate")):
		ni := info.Clone()
		if pdfa4 {
			ni = pdf.Dict{}
		}
		ni.Set("ModDate", date)
		if ref, ok := infoObj.(pdf.Ref); ok {
			u.Put(ref, ni)
			trailer.Set("Info", ref)
		} else {
			trailer.Set("Info", ni)
		}
	case hasInfo:
		trailer.Set("Info", infoObj)
	}

	// New version of the catalog.
	rootRef, ok := r.Trailer().Get("Root").(pdf.Ref)
	if !ok {
		rootRef = u.Alloc()
	}
	u.Put(rootRef, cat)
	trailer.Set("Root", rootRef)

	// File identifier: keep the permanent part, change the second one.
	var first pdf.String
	if id, ok := r.Resolve(r.Trailer().Get("ID")).(pdf.Array); ok && len(id) > 0 {
		first, _ = r.Resolve(id[0]).(pdf.String)
	}
	h := md5.New()
	h.Write(first)
	h.Write(date)
	h.Write(sum[:])
	for _, s := range suppSums {
		h.Write(s[:])
	}
	fmt.Fprintf(h, "%d", len(pdfData))
	second := pdf.String(h.Sum(nil))
	if len(first) == 0 {
		first = second
	}
	trailer.Set("ID", pdf.Array{first, second})
	out := u.Bytes(trailer)
	if o.Sign != nil {
		return Sign(out, o.Sign)
	}
	return out, nil
}
