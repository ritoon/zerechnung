package zerechnung

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/xmp"
)

// Limits of the PDF/A scan.
const (
	maxScanObjects = 200000 // objects visited
	maxScanDepth   = 32     // nesting of page tree nodes, forms, outlines and action chains
	maxReported    = 10     // findings per rule before the rest is summed up
)

// forbiddenActions are the action types PDF/A-2 and PDF/A-3 forbid
// (ISO 19005-3, 6.6.1), with the deprecated set-state and no-op actions.
var forbiddenActions = map[pdf.Name]bool{
	"Launch": true, "Sound": true, "Movie": true, "ResetForm": true, "ImportData": true,
	"Hide": true, "SetOCGState": true, "Rendition": true, "Trans": true, "GoTo3DView": true,
	"JavaScript": true, "SetState": true, "NOP": true,
}

// allowedNamed are the named actions PDF/A allows.
var allowedNamed = map[pdf.Name]bool{"NextPage": true, "PrevPage": true, "FirstPage": true, "LastPage": true}

// Roles in which an object is visited by the scan. The same object may be
// reached in several roles (a widget is both a form field and an
// annotation), each of which is checked once.
const (
	roleNode uint16 = 1 << iota
	roleResources
	roleFont
	roleXObject
	roleStream
	roleAnnot
	roleField
	roleOutline
	roleAction
	roleAF
	roleStruct
)

// pdfaScan checks the PDF/A requirements that can be verified without
// rendering the pages: embedded fonts (PDF-31), JavaScript and actions
// (PDF-32 to PDF-34), the other embedded files (PDF-36, PDF-37, PDF-39) and
// the LZWDecode filter (PDF-38). It walks the document catalog, the
// outlines, the form fields and the page tree with the resources of the
// pages, form XObjects, patterns, soft masks, Type 3 fonts and annotation
// appearances, and the structure tree when an embedded file is not found in
// another /AF array. The content streams are not parsed.
type pdfaScan struct {
	c      *container
	k      *containerCheck
	seen   map[pdf.Ref]uint16 // roles in which an indirect object was visited
	visits int
	fonts  map[string]bool  // fonts reported, by name and subtype
	files  []*fileSpec      // file specifications of FileAttachment annotations
	afRefs map[pdf.Ref]bool // file specifications listed in an /AF array besides the catalog's
	counts map[string]int   // findings per rule

	// The requirements of ISO 19005-2 and 19005-3 (check_pdfa_iso.go).
	iso   bool                       // they apply: the document does not declare PDF/A-4
	outCS string                     // colour space of the PDF/A output intent profile
	iccs  map[pdf.Ref]iccHeader      // ICC profiles read
	seps  map[pdf.Name][2]pdf.Object // alternate space and tint transform of the Separation colour spaces
	xmp   xmp.Declared               // properties described by the extension schemas of the document metadata

	content *contentState // the content streams read (check_pdfa_content.go)
}

// pdfa runs the PDF/A scan. invoice is the file specification of the
// invoice attachment, checked by CheckContainer itself; it may be nil. info
// is the XMP metadata, or nil: the requirements of ISO 19005-2 and 19005-3
// (PDF-41 to PDF-69) apply unless it declares PDF/A-4.
func (k *containerCheck) pdfa(c *container, invoice *fileSpec, info *xmp.Info) {
	// fonts, afRefs and counts, rarely needed, are made on first use.
	s := &pdfaScan{c: c, k: k, seen: map[pdf.Ref]uint16{}}
	s.iso = info == nil || strings.TrimSpace(info.PDFAPart) != "4"
	r := c.r
	cat := c.catalog
	if s.iso {
		s.outCS = s.outputSpace()
		s.documentLevel()
	}
	if s.present(cat, "AA") {
		s.report("PDF-34", "catalog /AA", "the document catalog has an additional-actions dictionary (/AA)")
	}
	if names, ok := r.Dict(cat.Get("Names")); ok && s.present(names, "JavaScript") {
		s.report("PDF-32", "catalog /Names /JavaScript", "the document has document-level JavaScript (/Names /JavaScript)")
	}
	s.action(cat.Get("OpenAction"), "catalog /OpenAction", 0)
	s.outlines()
	s.fields()
	s.pages()

	s.stream(cat.Get("Metadata"), "catalog /Metadata")
	if intents, ok := r.Resolve(cat.Get("OutputIntents")).(pdf.Array); ok {
		for i, o := range intents {
			if d, ok := r.Dict(o); ok {
				s.stream(d.Get("DestOutputProfile"), fmt.Sprintf("catalog /OutputIntents [%d] /DestOutputProfile", i))
			}
		}
	}
	s.embeddedFiles(invoice)
	if s.iso {
		s.structureNames()
		s.metadata()
		s.fileStructure() // last: it skips the streams the walk checked
	}

	ids := make([]string, 0, len(s.counts))
	for id := range s.counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if n := s.counts[id] - maxReported; n > 0 {
			k.add(id, "", "%d more occurrences not listed", n)
		}
	}
}

// report adds a finding, up to maxReported per rule.
func (s *pdfaScan) report(id, path, format string, args ...any) {
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	s.counts[id]++
	if s.counts[id] <= maxReported {
		s.k.add(id, path, format, args...)
	}
}

// visit reports whether o should be visited in role: an indirect object is
// visited once per role, and the scan stops after maxScanObjects visits.
func (s *pdfaScan) visit(o pdf.Object, role uint16) bool {
	if s.visits >= maxScanObjects {
		return false
	}
	if ref, ok := o.(pdf.Ref); ok {
		if s.seen[ref]&role != 0 {
			return false
		}
		s.seen[ref] |= role
	}
	s.visits++
	return true
}

// present reports whether d has a non-null entry key.
func (s *pdfaScan) present(d pdf.Dict, key pdf.Name) bool {
	return s.c.r.Resolve(d.Get(key)) != nil
}

func (s *pdfaScan) name(d pdf.Dict, key pdf.Name) pdf.Name {
	n, _ := s.c.r.Resolve(d.Get(key)).(pdf.Name)
	return n
}

func (s *pdfaScan) text(d pdf.Dict, key pdf.Name) string {
	if t, ok := s.c.r.Resolve(d.Get(key)).(pdf.String); ok {
		return pdf.Text(t)
	}
	return ""
}

// action checks an action and the actions chained to it by /Next.
func (s *pdfaScan) action(o pdf.Object, where string, depth int) {
	if o == nil || depth > maxScanDepth || !s.visit(o, roleAction) {
		return
	}
	r := s.c.r
	if list, ok := r.Resolve(o).(pdf.Array); ok { // /Next may be an array
		for i, x := range list {
			s.action(x, fmt.Sprintf("%s [%d]", where, i), depth+1)
		}
		return
	}
	d, ok := r.Resolve(o).(pdf.Dict)
	if !ok {
		return // a destination
	}
	switch typ := s.name(d, "S"); {
	case forbiddenActions[typ]:
		s.report("PDF-33", where, "the action is a %s action, which PDF/A forbids", typ)
	case typ == "Named":
		if n := s.name(d, "N"); !allowedNamed[n] {
			s.report("PDF-33", where, "the action is the named action %s; PDF/A only allows NextPage, PrevPage, FirstPage and LastPage", n)
		}
	}
	s.action(d.Get("Next"), where+" /Next", depth+1)
}

// outlines checks the actions of the outline items.
func (s *pdfaScan) outlines() {
	r := s.c.r
	root, ok := r.Dict(s.c.catalog.Get("Outlines"))
	if !ok {
		return
	}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		for o != nil && depth <= maxScanDepth && s.visit(o, roleOutline) {
			d, ok := r.Dict(o)
			if !ok {
				return
			}
			s.action(d.Get("A"), fmt.Sprintf("outline item %q /A", s.text(d, "Title")), 0)
			walk(d.Get("First"), depth+1)
			o = d.Get("Next")
		}
	}
	walk(root.Get("First"), 0)
}

// fields checks the additional actions of the form fields. Widget
// annotations, merged with their field or not, are checked with the
// annotations of the pages.
func (s *pdfaScan) fields() {
	r := s.c.r
	form, ok := r.Dict(s.c.catalog.Get("AcroForm"))
	if !ok {
		return
	}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		if depth > maxScanDepth || !s.visit(o, roleField) {
			return
		}
		d, ok := r.Dict(o)
		if !ok {
			return
		}
		if s.name(d, "Subtype") != "Widget" && s.present(d, "AA") {
			s.report("PDF-34", fmt.Sprintf("form field %q /AA", s.text(d, "T")), "the form field has an additional-actions dictionary (/AA)")
		}
		if kids, ok := r.Resolve(d.Get("Kids")).(pdf.Array); ok {
			for _, kid := range kids {
				walk(kid, depth+1)
			}
		}
	}
	if fields, ok := r.Resolve(form.Get("Fields")).(pdf.Array); ok {
		for _, f := range fields {
			walk(f, 0)
		}
	}
}

// pages walks the page tree. Resources are inherited from the ancestors of
// a page.
func (s *pdfaScan) pages() {
	r := s.c.r
	n := 0
	// inherited holds the resources and the inheritable page boundaries.
	var walk func(o pdf.Object, inherited map[pdf.Name]pdf.Object, depth int)
	walk = func(o pdf.Object, inherited map[pdf.Name]pdf.Object, depth int) {
		if depth > maxScanDepth || !s.visit(o, roleNode) {
			return
		}
		d, ok := r.Dict(o)
		if !ok {
			return
		}
		for _, key := range []pdf.Name{"Resources", "MediaBox", "CropBox"} {
			if x := d.Get(key); x != nil {
				m := make(map[pdf.Name]pdf.Object, 3)
				for k, v := range inherited {
					m[k] = v
				}
				m[key] = x
				inherited = m
			}
		}
		res := inherited["Resources"]
		if kids, ok := r.Resolve(d.Get("Kids")).(pdf.Array); ok && s.name(d, "Type") != "Page" {
			for _, kid := range kids {
				walk(kid, inherited, depth+1)
			}
			return
		}
		n++
		where := fmt.Sprintf("page %d", n)
		s.associate(d.Get("AF"))
		if s.present(d, "AA") {
			s.report("PDF-34", where+" /AA", "the page has an additional-actions dictionary (/AA)")
		}
		var def uint8
		if resDict, ok := r.Dict(res); ok {
			def = s.defaults(resDict)
		}
		if s.iso {
			s.pageBoxes(d, inherited, where)
			if g, ok := r.Dict(d.Get("Group")); ok {
				s.colourSpace(g.Get("CS"), where+" /Group /CS", def, 0)
			}
			if d.Has("PresSteps") {
				s.report("PDF-69", where+" /PresSteps", "the page has presentation steps (/PresSteps), which PDF/A forbids")
			}
		}
		switch contents := d.Get("Contents"); v := r.Resolve(contents).(type) {
		case *pdf.Stream:
			s.stream(contents, where+" /Contents")
		case pdf.Array:
			for i, x := range v {
				s.stream(x, fmt.Sprintf("%s /Contents [%d]", where, i))
			}
		}
		s.resources(res, where+" /Resources", 0, 0)
		var pc *pageContent
		transparent := false
		if s.iso {
			pc, transparent = s.pageContent(d, res, def, where)
		}
		if annots, ok := r.Resolve(d.Get("Annots")).(pdf.Array); ok {
			for i, a := range annots {
				path := fmt.Sprintf("%s /Annots [%d]", where, i)
				s.annotation(a, path)
				if s.iso {
					transparent = s.appearances(a, pc, path) || transparent
				}
			}
		}
		if transparent && s.outCS == "" {
			s.pageTransparency(d, where)
		}
	}
	walk(s.c.catalog.Get("Pages"), nil, 0)
}

// annotation checks the actions and appearance streams of an annotation,
// and collects the embedded file of a file attachment annotation.
func (s *pdfaScan) annotation(o pdf.Object, where string) {
	if !s.visit(o, roleAnnot) {
		return
	}
	r := s.c.r
	d, ok := r.Dict(o)
	if !ok {
		return
	}
	if s.present(d, "AA") {
		s.report("PDF-34", where+" /AA", "the annotation has an additional-actions dictionary (/AA)")
	}
	if s.iso {
		s.annotationDict(d, where)
	}
	s.associate(d.Get("AF"))
	s.action(d.Get("A"), where+" /A", 0)
	if ap, ok := r.Dict(d.Get("AP")); ok {
		for _, key := range []pdf.Name{"N", "R", "D"} {
			x := ap.Get(key)
			if _, ok := r.Resolve(x).(*pdf.Stream); ok {
				s.xobject(x, where+" /AP /"+string(key), 0, 0)
			} else if states, ok := r.Resolve(x).(pdf.Dict); ok {
				for _, st := range states {
					s.xobject(st.Val, where+" /AP /"+string(key)+" /"+string(st.Key), 0, 0)
				}
			}
		}
	}
	if s.name(d, "Subtype") == "FileAttachment" {
		fsObj := d.Get("FS")
		if fs, ok := r.Resolve(fsObj).(pdf.Dict); ok && fs.Has("EF") {
			ref, _ := fsObj.(pdf.Ref)
			s.files = append(s.files, &fileSpec{ref: ref, dict: fs, name: specName(r, fs)})
		}
	}
}

// resources checks the fonts, XObjects, patterns, shadings, soft masks,
// colour spaces and graphics states of a resource dictionary. def holds the
// default colour spaces of the enclosing content streams.
func (s *pdfaScan) resources(o pdf.Object, where string, depth int, def uint8) {
	if o == nil || depth > maxScanDepth || !s.visit(o, roleResources) {
		return
	}
	r := s.c.r
	d, ok := r.Dict(o)
	if !ok {
		return
	}
	def |= s.defaults(d)
	if fonts, ok := r.Dict(d.Get("Font")); ok {
		for _, f := range fonts {
			s.font(f.Val, where+" /Font /"+string(f.Key), depth, def)
		}
	}
	if xobjects, ok := r.Dict(d.Get("XObject")); ok {
		for _, x := range xobjects {
			s.xobject(x.Val, where+" /XObject /"+string(x.Key), depth, def)
		}
	}
	if patterns, ok := r.Dict(d.Get("Pattern")); ok {
		for _, p := range patterns {
			path := where + " /Pattern /" + string(p.Key)
			switch v := r.Resolve(p.Val).(type) {
			case *pdf.Stream: // tiling pattern
				s.xobject(p.Val, path, depth, def)
			case pdf.Dict: // shading pattern
				s.shading(v.Get("Shading"), path+" /Shading", def)
				if g, ok := r.Dict(v.Get("ExtGState")); ok && s.iso {
					s.extGState(g, path+" /ExtGState")
				}
			}
		}
	}
	if shadings, ok := r.Dict(d.Get("Shading")); ok {
		for _, sh := range shadings {
			s.shading(sh.Val, where+" /Shading /"+string(sh.Key), def)
		}
	}
	if props, ok := r.Dict(d.Get("Properties")); ok {
		for _, p := range props {
			if pl, ok := r.Dict(p.Val); ok {
				s.associate(pl.Get("AF"))
			}
		}
	}
	if states, ok := r.Dict(d.Get("ExtGState")); ok {
		for _, gs := range states {
			if g, ok := r.Dict(gs.Val); ok {
				path := where + " /ExtGState /" + string(gs.Key)
				if s.iso {
					s.extGState(g, path)
				}
				if mask, ok := r.Resolve(g.Get("SMask")).(pdf.Dict); ok {
					s.xobject(mask.Get("G"), path+" /SMask /G", depth, def)
				}
			}
		}
	}
	if s.iso {
		if spaces, ok := r.Dict(d.Get("ColorSpace")); ok {
			for _, cs := range spaces {
				s.colourSpace(cs.Val, where+" /ColorSpace /"+string(cs.Key), def, 0)
			}
		}
	}
}

// shading checks the filters and the colour space of a shading.
func (s *pdfaScan) shading(o pdf.Object, where string, def uint8) {
	s.stream(o, where)
	if !s.iso {
		return
	}
	if d, ok := s.c.r.Dict(o); ok {
		s.colourSpace(d.Get("ColorSpace"), where+" /ColorSpace", def, 0)
	} else if st, ok := s.c.r.Resolve(o).(*pdf.Stream); ok {
		s.colourSpace(st.Dict.Get("ColorSpace"), where+" /ColorSpace", def, 0)
	}
}

// xobject checks a form or image XObject, a tiling pattern or an
// appearance stream: its filters and, for a content stream, its resources.
func (s *pdfaScan) xobject(o pdf.Object, where string, depth int, def uint8) {
	st, ok := s.c.r.Resolve(o).(*pdf.Stream)
	if !ok {
		return
	}
	s.stream(o, where)
	if depth > maxScanDepth || !s.visit(o, roleXObject) {
		return
	}
	s.associate(st.Dict.Get("AF"))
	switch s.name(st.Dict, "Subtype") {
	case "Image":
		if s.iso {
			s.image(st.Dict, where, def, imageXObject)
			s.jpx(st, where)
		}
		for _, m := range []struct {
			key  pdf.Name
			kind imageKind
		}{{"SMask", imageSMask}, {"Mask", imageMask}} {
			x := st.Dict.Get(m.key)
			s.stream(x, where+" /"+string(m.key))
			if mask, ok := s.c.r.Resolve(x).(*pdf.Stream); ok && s.iso {
				s.image(mask.Dict, where+" /"+string(m.key), def, m.kind)
			}
		}
		return
	case "PS":
		if s.iso {
			s.report("PDF-57", where, "the XObject is a PostScript XObject, which PDF/A forbids")
		}
		return
	}
	if s.iso {
		s.form(st.Dict, where, def)
	}
	s.resources(st.Dict.Get("Resources"), where+" /Resources", depth+1, def)
}

// font checks that a font embeds its font program, and, where ISO 19005-2
// and 19005-3 apply, its font dictionary. Type 3 fonts, whose glyphs are
// content streams, have no font program: their resources are checked
// instead.
func (s *pdfaScan) font(o pdf.Object, where string, depth int, def uint8) {
	if !s.visit(o, roleFont) {
		return
	}
	// The glyph widths of a CIDFont (/W), thousands of numbers, are
	// skipped. Those of a simple font (/Widths) are 256 at most.
	r := s.c.r
	d, ok := r.DictSkipArrays(o, "DescendantFonts", "Widths")
	if !ok {
		return
	}
	sub := s.name(d, "Subtype")
	if s.iso {
		s.fontDict(d, where)
	}
	desc := d
	switch sub {
	case "Type3":
		s.resources(d.Get("Resources"), where+" /Resources", depth+1, def)
		return
	case "Type0":
		desc = nil
		if list, ok := r.Resolve(d.Get("DescendantFonts")).(pdf.Array); ok && len(list) > 0 {
			desc, _ = r.DictSkipArrays(list[0])
		}
	}
	fd, ok := r.Dict(desc.Get("FontDescriptor"))
	if ok {
		for _, key := range []pdf.Name{"FontFile", "FontFile2", "FontFile3"} {
			if _, ok := r.Resolve(fd.Get(key)).(*pdf.Stream); ok {
				s.stream(fd.Get(key), where+" /FontDescriptor /"+string(key))
				return
			}
		}
	}
	name := string(s.name(d, "BaseFont"))
	if s.fonts[name+" "+string(sub)] {
		return
	}
	if s.fonts == nil {
		s.fonts = map[string]bool{}
	}
	s.fonts[name+" "+string(sub)] = true
	if !ok {
		s.report("PDF-31", where, "font %q (%s) is not embedded: it has no font descriptor", name, sub)
		return
	}
	s.report("PDF-31", where, "font %q (%s) is not embedded: its font descriptor has no /FontFile, /FontFile2 or /FontFile3", name, sub)
}

// stream checks the filters of a stream (6.1.7.2).
func (s *pdfaScan) stream(o pdf.Object, where string) {
	st, ok := s.c.r.Resolve(o).(*pdf.Stream)
	if !ok || !s.visit(o, roleStream) {
		return
	}
	names, parms := s.filters(st.Dict)
	for i, f := range names {
		if msg := s.filterIssue(f, parms[i]); msg != "" {
			s.report("PDF-38", where, "%s", msg)
			return
		}
	}
}

// associate records the file specifications listed in an /AF array. A
// direct file specification cannot be listed anywhere else: only references
// are recorded.
func (s *pdfaScan) associate(o pdf.Object) {
	if o == nil || !s.visit(o, roleAF) {
		return
	}
	if af, ok := s.c.r.Resolve(o).(pdf.Array); ok {
		for _, x := range af {
			if ref, ok := x.(pdf.Ref); ok {
				if s.afRefs == nil {
					s.afRefs = map[pdf.Ref]bool{}
				}
				s.afRefs[ref] = true
			}
		}
	}
}

// associated reports whether a file specification is listed in the /AF
// array of the catalog or of a page, or in one recorded by associate.
func (s *pdfaScan) associated(fs *fileSpec) bool {
	return fs.inAF || fs.inPageAF || (fs.ref != pdf.Ref{} && s.afRefs[fs.ref])
}

// structure records the /AF arrays of the structure elements.
func (s *pdfaScan) structure() {
	r := s.c.r
	root, ok := r.Dict(s.c.catalog.Get("StructTreeRoot"))
	if !ok {
		return
	}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		if o == nil || depth > maxScanDepth || !s.visit(o, roleStruct) {
			return
		}
		switch v := r.Resolve(o).(type) {
		case pdf.Array:
			for _, x := range v {
				walk(x, depth)
			}
		case pdf.Dict:
			s.associate(v.Get("AF"))
			walk(v.Get("K"), depth+1)
		}
	}
	walk(root.Get("K"), 0)
}

// embeddedFiles checks the embedded files besides the invoice: an
// /AFRelationship, a MIME type and an /AF array listing them, as PDF/A-3
// requires for every embedded file. The filters of every embedded file
// stream are checked too.
func (s *pdfaScan) embeddedFiles(invoice *fileSpec) {
	c := s.c
	r := c.r
	c.collectPages()
	files := append(append([]*fileSpec(nil), c.specs...), s.files...)
	var seen, unlisted []*fileSpec
	for _, fs := range files {
		dup := false
		for _, o := range seen {
			if o == fs || (fs.ref != pdf.Ref{} && o.ref == fs.ref) || (fs.ref == pdf.Ref{} && sameDict(o.dict, fs.dict)) {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		seen = append(seen, fs)
		path := specPath(fs)
		if ef, ok := r.Dict(fs.dict.Get("EF")); ok {
			for _, key := range []pdf.Name{"F", "UF"} {
				s.stream(ef.Get(key), path+" /EF /"+string(key))
			}
		}
		if invoice != nil && (fs == invoice || (fs.ref != pdf.Ref{} && fs.ref == invoice.ref)) {
			continue
		}
		name := strconv.Quote(fs.filename())
		if s.name(fs.dict, "AFRelationship") == "" {
			s.report("PDF-36", path+" /AFRelationship", "the file specification of the embedded file %s has no /AFRelationship", name)
		}
		if st, ok := c.embeddedStream(fs); ok && s.name(st.Dict, "Subtype") == "" {
			s.report("PDF-37", "embedded file "+name+" /Subtype", "the embedded file %s has no MIME type (/Subtype)", name)
		}
		if !s.associated(fs) {
			unlisted = append(unlisted, fs)
		}
	}

	// PDF-39: the structure tree is only walked when a file is not listed
	// elsewhere. A scan cut short by maxScanObjects reports nothing, the
	// /AF arrays not visited being unknown.
	if len(unlisted) == 0 {
		return
	}
	s.structure()
	if s.visits >= maxScanObjects {
		return
	}
	for _, fs := range unlisted {
		if !s.associated(fs) {
			name := strconv.Quote(fs.filename())
			s.report("PDF-39", specPath(fs), "the embedded file %s is not listed in any /AF array (catalog, page, annotation, XObject, marked-content property list or structure element)", name)
		}
	}
}

// docInfo checks that the entries of the document information dictionary
// that have an XMP equivalent have the same value in the XMP metadata. An
// empty entry needs no equivalent. ISO 19005-1 required it (6.7.3), ISO
// 19005-2 and 19005-3 no longer do (6.1.5): the veraPDF corpus holds a valid
// PDF/A-2 file whose /Info differs from its XMP. PDF-35 is thus a warning.
func (k *containerCheck) docInfo(c *container, info *xmp.Info) {
	d, ok := c.r.Dict(c.r.Trailer().Get("Info"))
	if !ok {
		return
	}
	for _, e := range xmp.InfoEquivalents {
		s, ok := c.r.Resolve(d.Get(pdf.Name(e.Entry))).(pdf.String)
		if !ok {
			continue
		}
		v := pdf.Text(s)
		x, has := info.InfoProps[e.Entry]
		path := "trailer /Info /" + e.Entry
		switch {
		case !has:
			if v != "" {
				k.add("PDF-35", path, "/Info /%s is %q, but the XMP metadata has no %s", e.Entry, v, e.Property)
			}
		case e.Entry == "CreationDate" || e.Entry == "ModDate":
			if !sameDate(v, x) {
				k.add("PDF-35", path, "/Info /%s is %q, XMP %s is %q: the dates differ", e.Entry, v, e.Property, x)
			}
		case v != x:
			k.add("PDF-35", path, "/Info /%s is %q, XMP %s is %q", e.Entry, v, e.Property, x)
		}
	}
}

// docInfoPDFA4 checks the document information dictionary of a document
// that declares PDF/A-4 (ISO 19005-4, 6.1.3). PDF 2.0 deprecates it in
// favour of the XMP metadata; PDF/A-4 admits it only when the catalog has a
// /PieceInfo entry, whose LastModified dates are compared with its ModDate,
// and then with that entry alone. The veraPDF profile checks the two as
// rules 6.1.3-4 and 6.1.3-5; a single finding is reported, the one that says
// what to change: remove the dictionary when the catalog has no /PieceInfo,
// reduce it to ModDate otherwise. A null /Info, or a reference to the null
// object, is an absent entry (ISO 32000-2, 7.3.9 and 7.3.10).
func (k *containerCheck) docInfoPDFA4(c *container) {
	o := c.r.Resolve(c.r.Trailer().Get("Info"))
	if o == nil {
		return
	}
	const path = "trailer /Info"
	if !c.catalog.Has("PieceInfo") {
		k.add("PDF-70", path, "the trailer has a document information dictionary and the catalog has no /PieceInfo entry: PDF/A-4 then forbids /Info, whose entries belong in the XMP metadata")
		return
	}
	d, ok := o.(pdf.Dict)
	if !ok {
		k.add("PDF-70", path, "/Info is not a dictionary; PDF/A-4 admits a document information dictionary holding ModDate alone")
		return
	}
	var others []string
	for _, kv := range d {
		if kv.Key != "ModDate" {
			others = append(others, "/"+string(kv.Key))
		}
	}
	switch {
	case len(others) > 0:
		k.add("PDF-70", path, "the document information dictionary holds %s; PDF/A-4 admits ModDate alone", strings.Join(others, ", "))
	case !d.Has("ModDate"):
		k.add("PDF-70", path, "the document information dictionary has no ModDate entry; PDF/A-4 admits one holding ModDate alone")
	}
}

// sameDate reports whether a PDF date and an XMP date are the same instant,
// to the second.
func sameDate(pdfDate, xmpDate string) bool {
	a, ok := pdf.ParseDate(pdfDate)
	if !ok {
		return false
	}
	b, ok := xmp.ParseDate(xmpDate)
	return ok && a.Equal(b.Truncate(time.Second))
}
