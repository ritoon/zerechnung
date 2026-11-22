package zerechnung

import "github.com/ritoon/zerechnung/internal/pdf"

// This file reads the content streams for the requirements of ISO 19005-2
// and 19005-3 that bear on them, as the veraPDF validation profiles read
// them: the content of the pages, of the form XObjects, tiling patterns and
// soft masks they use, of the glyphs of the Type 3 fonts they show and of
// the appearance streams of the annotations. Each is read in the context it
// is used in: its resources, the default colour spaces in force and the
// graphics state it starts with.
//
// The checks: the operators (PDF-71, 6.2.2), the resources a stream takes
// from the page instead of its own dictionary (PDF-71, 6.2.2), the syntax
// of the operands (PDF-44, 6.1.6; PDF-49, 6.1.13), the nesting of q
// operators (PDF-49, 6.1.13), the inline images (PDF-38, 6.1.10; PDF-56,
// 6.2.8; PDF-55, 6.2.6), the device colour spaces the colour and painting
// operators use (PDF-52, 6.2.4.3), overprinting with an ICCBased CMYK
// colour space (PDF-51, 6.2.4.2), the ri operator (PDF-55, 6.2.6), the
// transparency of a page without a group colour space nor a PDF/A output
// intent (PDF-58, 6.2.10) and the glyphs the text shows (PDF-72, 6.2.11,
// check_pdfa_glyphs.go).

// maxContentBytes bounds the decoded content the scan reads; the streams
// beyond are not read.
const maxContentBytes = 64 << 20

// Kinds of content streams read, part of the key under which a stream read
// in a context is remembered.
const (
	contentForm uint8 = iota + 1 // a form XObject or an appearance stream
	contentPattern
	contentMask // the group of a soft mask
	contentGlyph
)

// contentKey identifies a content stream read in a context.
type contentKey struct {
	num      uint32
	kind     uint8
	def      uint8
	noColour bool
	page     string // identity of the page resources, for a stream without resources of its own
}

// Results of reading a content stream.
const (
	contentReading uint8 = iota + 1
	contentOpaque
	contentTransparent
)

// contentState is the state of the content scan of a document.
type contentState struct {
	done      map[contentKey]uint8      // streams read, with their result
	encodings map[string]*[256]pdf.Name // glyph names of the Type 3 fonts, by font
	fonts     map[string]*glyphFont     // the fonts the text shows, by font (check_pdfa_glyphs.go)
	programs  map[string]*fontProgram   // the font programs read, by stream
	bytes     int                       // decoded content read
}

// colourUse is a colour space selected in a graphics state.
type colourUse struct {
	device      pdf.Name // DeviceRGB, DeviceCMYK or DeviceGray, when no default colour space replaces it
	cmykICC     bool     // an ICCBased CMYK colour space (6.2.4.2)
	transparent bool     // a tiling pattern whose content uses transparency
}

// gstate holds the parameters of the graphics state the checks follow.
type gstate struct {
	fill, stroke     colourUse
	tr               int64      // text rendering mode
	font             pdf.Object // font selected by Tf
	fontName         pdf.Name
	opFill, opStroke bool // overprint, non-stroking and stroking
	opm              int64
	ca, caStroke     float64 // constant alpha, non-stroking and stroking
	softMask, blend  bool    // a soft mask, a blend mode other than Normal
}

// pageContent is the page whose content, or the streams it uses, is read.
type pageContent struct {
	res pdf.Dict // resources of the page
	id  string   // identity of the resources
}

// contentCtx is the context in which a content stream is read.
type contentCtx struct {
	res      pdf.Dict // resources of the stream: the page's for the page content
	top      bool     // the content of the page
	pageTree bool     // page content whose resources come from the page tree
	page     *pageContent
	def      uint8 // default colour spaces in force
	where    string
	depth    int
	noColour bool // colour operators are ignored: uncoloured tiling pattern, Type 3 glyph after d1
	glyph    bool // the description of a Type 3 glyph
	gs       *gstate
}

// streamScan reads one content stream.
type streamScan struct {
	s           *pdfaScan
	x           contentCtx
	cur         gstate
	stack       []gstate
	flags       map[string]bool // findings already made for the stream
	transparent bool
	fontRef     pdf.Ref      // the last font of the text, by reference,
	font        *glyphFont   // and its glyph model (check_pdfa_glyphs.go)
	strs        []pdf.String // the strings of the text operator being read
}

func (s *pdfaScan) contentState() *contentState {
	if s.content == nil {
		s.content = &contentState{done: map[contentKey]uint8{}, encodings: map[string]*[256]pdf.Name{},
			fonts: map[string]*glyphFont{}, programs: map[string]*fontProgram{}}
	}
	return s.content
}

// decodeContent decodes a content stream, or returns nil when it cannot be
// decoded or the content read is over maxContentBytes. The buffer is lent
// by the scratch of the scan: the content must not be used once the scan
// ends.
func (s *pdfaScan) decodeContent(st *pdf.Stream) []byte {
	cs := s.contentState()
	if cs.bytes > maxContentBytes {
		return nil
	}
	data, err := s.c.r.DecodeScratch(st, 0, &s.scratch)
	if err != nil {
		return nil
	}
	cs.bytes += len(data)
	return data
}

// deviceColour returns the colour space a device colour space selects:
// none to check when a default colour space replaces it.
func deviceColour(n pdf.Name, def uint8) colourUse {
	switch n {
	case "DeviceGray", "G":
		if def&defaultGray == 0 {
			return colourUse{device: "DeviceGray"}
		}
	case "DeviceRGB", "RGB":
		if def&defaultRGB == 0 {
			return colourUse{device: "DeviceRGB"}
		}
	case "DeviceCMYK", "CMYK":
		if def&defaultCMYK == 0 {
			return colourUse{device: "DeviceCMYK"}
		}
		return colourUse{cmykICC: true}
	}
	return colourUse{}
}

// initialState returns the graphics state a content stream starts with.
func initialState(def uint8) gstate {
	gray := deviceColour("DeviceGray", def)
	return gstate{fill: gray, stroke: gray, ca: 1, caStroke: 1}
}

// pageContent reads the content of a page, of resources res, and returns
// it with whether the content uses transparency.
func (s *pdfaScan) pageContent(page pdf.Dict, res pdf.Object, def uint8, where string) (*pageContent, bool) {
	r := s.c.r
	rd, _ := r.Dict(res)
	pc := &pageContent{res: rd, id: identity(res)}
	var data []byte
	switch v := r.Resolve(page.Get("Contents")).(type) {
	case *pdf.Stream:
		data = s.decodeContent(v)
	case pdf.Array:
		for _, x := range v {
			if st, ok := r.Resolve(x).(*pdf.Stream); ok {
				data = append(append(data, s.decodeContent(st)...), '\n')
			}
		}
	}
	gs := initialState(def)
	x := contentCtx{
		res: rd, top: true, pageTree: page.Get("Resources") == nil && rd != nil,
		page: pc, def: def, where: where + " /Contents", gs: &gs,
	}
	return pc, s.readContent(data, x)
}

// appearances reads the appearance streams of an annotation of the page pc
// and returns whether the annotation uses transparency.
func (s *pdfaScan) appearances(o pdf.Object, pc *pageContent, where string) bool {
	r := s.c.r
	d, ok := r.Dict(o)
	if !ok {
		return false
	}
	t := false
	if bm, ok := r.Resolve(d.Get("BM")).(pdf.Name); ok && bm != "Normal" {
		t = true
	}
	ap, ok := r.Dict(d.Get("AP"))
	if !ok {
		return t
	}
	x := contentCtx{page: pc}
	for _, key := range []pdf.Name{"N", "R", "D"} {
		v := ap.Get(key)
		path := where + " /AP /" + string(key)
		switch states := r.Resolve(v).(type) {
		case *pdf.Stream:
			t = s.formContent(v, x, path, nil) || t
		case pdf.Dict:
			for _, st := range states {
				t = s.formContent(st.Val, x, path+" /"+string(st.Key), nil) || t
			}
		}
	}
	return t
}

// formContent reads a form XObject or an appearance stream used in the context x
// and returns whether it uses transparency: a transparency group or
// content that does. gs is the graphics state it starts with, nil for the
// initial one.
func (s *pdfaScan) formContent(o pdf.Object, x contentCtx, where string, gs *gstate) bool {
	st, ok := s.c.r.Resolve(o).(*pdf.Stream)
	if !ok {
		return false
	}
	t := false
	if g, ok := s.c.r.Dict(st.Dict.Get("Group")); ok && s.name(g, "S") == "Transparency" {
		t = true
	}
	return s.nested(o, st.Dict.Get("Resources"), contentForm, x, where, gs, x.noColour) || t
}

// nested reads the content stream o, of resources res, used in the context
// parent, and returns whether it uses transparency. A stream is read once
// in a context. A pattern does not see the default colour spaces of the
// stream that uses it, only those of its own resources.
func (s *pdfaScan) nested(o, res pdf.Object, kind uint8, parent contentCtx, where string, gs *gstate, noColour bool) bool {
	r := s.c.r
	st, ok := r.Resolve(o).(*pdf.Stream)
	if !ok || parent.depth >= maxScanDepth {
		return false
	}
	rd, own := r.Dict(res)
	def := parent.def
	if kind == contentPattern {
		def = 0
	}
	if own {
		def |= s.defaults(rd)
	}
	key := contentKey{kind: kind, def: def, noColour: noColour}
	if !own && parent.page != nil {
		key.page = parent.page.id
	}
	ref, isRef := o.(pdf.Ref)
	cs := s.contentState()
	if isRef {
		key.num = ref.Num
		switch cs.done[key] {
		case contentReading, contentOpaque:
			return false
		case contentTransparent:
			return true
		}
		cs.done[key] = contentReading
	}
	if gs == nil {
		init := initialState(def)
		gs = &init
	}
	x := contentCtx{
		res: rd, page: parent.page, def: def, where: where, depth: parent.depth + 1,
		noColour: noColour, glyph: kind == contentGlyph, gs: gs,
	}
	t := s.readContent(s.decodeContent(st), x)
	if isRef {
		cs.done[key] = contentOpaque
		if t {
			cs.done[key] = contentTransparent
		}
	}
	return t
}

// readContent reads a content stream in the context x and returns whether
// it uses transparency.
func (s *pdfaScan) readContent(data []byte, x contentCtx) bool {
	if len(data) == 0 {
		return false
	}
	sc := &streamScan{s: s, x: x, cur: *x.gs}
	pdf.ContentSyntax(data, func(is pdf.SyntaxIssue) {
		switch is.Kind {
		case pdf.SyntaxHex:
			sc.once("PDF-44", "hex", "%s", is.Detail)
		case pdf.SyntaxLimit:
			sc.once("PDF-49", "limit", "%s", is.Detail)
		}
	})
	_ = pdf.ScanOperands(data, sc.op)
	return sc.transparent
}

// once reports a finding on the stream, once per key.
func (sc *streamScan) once(id, key, format string, args ...any) {
	if sc.flags[key] {
		return
	}
	if sc.flags == nil {
		sc.flags = map[string]bool{}
	}
	sc.flags[key] = true
	sc.s.report(id, sc.x.where, format, args...)
}

// resource returns the resource name of category the stream uses. A
// stream other than the page content takes the names its own resources
// lack from those of the page, which veraPDF reports (6.2.2), as it reports
// page content whose resources come from the page tree.
func (sc *streamScan) resource(category, name pdf.Name) pdf.Object {
	r := sc.s.c.r
	x := &sc.x
	lookup := func(res pdf.Dict) pdf.Object {
		d, _ := r.Dict(res.Get(category))
		return d.Get(name)
	}
	if o := lookup(x.res); o != nil {
		if x.pageTree {
			sc.once("PDF-71", "inherited", "the content uses the resource /%s /%s of the page tree; PDF/A requires the page to have its own resource dictionary (6.2.2)", category, name)
		}
		return o
	}
	if x.top || x.page == nil {
		return nil
	}
	if o := lookup(x.page.res); o != nil {
		sc.once("PDF-71", "inherited", "the content stream uses the resource /%s /%s, which its own resource dictionary lacks: it is taken from the resources of the page, which PDF/A forbids (6.2.2)", category, name)
		return o
	}
	return nil
}

// device checks the use of a device colour space, once per family.
func (sc *streamScan) device(c colourUse) {
	if c.device == "" || sc.flags["cs "+string(c.device)] {
		return
	}
	if sc.flags == nil {
		sc.flags = map[string]bool{}
	}
	sc.flags["cs "+string(c.device)] = true
	sc.s.deviceSpace(c.device, sc.x.where, sc.x.def)
}

// setColour selects the colour space c for filling or stroking, through a
// colour operator.
func (sc *streamScan) setColour(c colourUse, stroke bool) {
	if sc.x.noColour {
		return
	}
	sc.device(c)
	if stroke {
		sc.cur.stroke = c
	} else {
		sc.cur.fill = c
	}
}

// namedColour returns the colour space of the colour space resource o. The
// device colour spaces of the resources are left to the resources walk,
// which reports them.
func (sc *streamScan) namedColour(o pdf.Object) colourUse {
	r := sc.s.c.r
	switch v := r.Resolve(o).(type) {
	case pdf.Name:
		if (v == "DeviceCMYK" || v == "CMYK") && sc.x.def&defaultCMYK != 0 {
			return colourUse{cmykICC: true}
		}
	case pdf.Array:
		if len(v) > 1 {
			if family, _ := r.Resolve(v[0]).(pdf.Name); family == "ICCBased" {
				if h, ok := sc.s.icc(v[1]); ok && h.space == "CMYK" {
					return colourUse{cmykICC: true}
				}
			}
		}
	}
	return colourUse{}
}

// paint records a painting operator that fills or strokes. colour tells a
// path or text, painted with the current colour spaces, from an image, a
// form or a shading.
func (sc *streamScan) paint(fill, stroke, colour bool) {
	g := &sc.cur
	if colour && !sc.x.noColour {
		if fill {
			sc.device(g.fill)
		}
		if stroke {
			sc.device(g.stroke)
		}
		if g.opm == 1 && (fill && g.opFill && g.fill.cmykICC || stroke && g.opStroke && g.stroke.cmykICC) {
			sc.once("PDF-51", "overprint", "the content paints in an ICCBased CMYK colour space with overprinting on and the overprint mode 1, which PDF/A forbids (6.2.4.2)")
		}
	}
	if g.softMask || g.blend ||
		fill && (g.ca < 1 || colour && g.fill.transparent) ||
		stroke && (g.caStroke < 1 || colour && g.stroke.transparent) {
		sc.transparent = true
	}
}

// number returns a numeric operand.
func number(o pdf.Object) (float64, bool) {
	switch v := o.(type) {
	case pdf.Int:
		return float64(v), true
	case pdf.Real:
		return float64(v), true
	}
	return 0, false
}

// op reads an operator.
func (sc *streamScan) op(op string, args []pdf.Operand) error {
	name := pdf.Name("")
	if len(args) > 0 {
		name, _ = args[0].AsName()
	}
	if !contentOps[op] {
		if len(op) > 32 {
			op = op[:32] + "..."
		}
		sc.once("PDF-71", "op "+op, "the content stream holds the operator %q, which ISO 32000-1 does not define; PDF/A forbids it, even between BX and EX (6.2.2)", op)
		return nil
	}
	g := &sc.cur
	switch op {
	case "q":
		sc.stack = append(sc.stack, sc.cur)
		if len(sc.stack) > maxNesting {
			sc.once("PDF-49", "nesting", "q operators are nested %d deep; PDF/A admits %d (6.1.13)", len(sc.stack), maxNesting)
		}
	case "Q":
		if n := len(sc.stack); n > 0 {
			sc.cur = sc.stack[n-1]
			sc.stack = sc.stack[:n-1]
		}
	case "g", "G":
		sc.setColour(deviceColour("DeviceGray", sc.x.def), op == "G")
	case "rg", "RG":
		sc.setColour(deviceColour("DeviceRGB", sc.x.def), op == "RG")
	case "k", "K":
		sc.setColour(deviceColour("DeviceCMYK", sc.x.def), op == "K")
	case "cs", "CS":
		switch name {
		case "DeviceGray", "DeviceRGB", "DeviceCMYK":
			sc.setColour(deviceColour(name, sc.x.def), op == "CS")
		case "Pattern":
			sc.setColour(colourUse{}, op == "CS")
		default:
			o := sc.resource("ColorSpace", name)
			sc.setColour(sc.namedColour(o), op == "CS")
		}
	case "scn", "SCN":
		if sc.x.noColour || len(args) == 0 {
			break
		}
		if n, ok := args[len(args)-1].AsName(); ok {
			sc.setColour(sc.pattern(n), op == "SCN")
		}
	case "gs":
		sc.extGState(name)
	case "ri":
		if !renderingIntents[name] && len(args) > 0 {
			sc.once("PDF-55", "ri "+string(name), "the ri operator sets the rendering intent %v, which is not one of RelativeColorimetric, AbsoluteColorimetric, Perceptual and Saturation (6.2.6)", args[0].Object())
		}
	case "S", "s":
		sc.paint(false, true, true)
	case "f", "F", "f*":
		sc.paint(true, false, true)
	case "B", "B*", "b", "b*":
		sc.paint(true, true, true)
	case "sh":
		sc.resource("Shading", name)
		sc.paint(true, false, false)
	case "Do":
		sc.xobject(name)
	case "BI":
		if len(args) == 1 && args[0].Kind == pdf.OperandDict {
			sc.inlineImage(args[0].Dict)
		}
		sc.paint(true, false, false)
	case "BDC", "DP":
		if len(args) == 2 {
			if n, ok := args[1].AsName(); ok {
				sc.resource("Properties", n)
			}
		}
	case "Tf":
		g.font = sc.resource("Font", name)
		g.fontName = name
	case "Tr":
		if len(args) > 0 {
			g.tr, _ = args[0].AsInt()
		}
	case "Tj", "'", "\"":
		if len(args) > 0 {
			s, _ := args[len(args)-1].AsString()
			// veraPDF reads no glyph from ", whose first operand is a number.
			sc.strs = append(sc.strs[:0], s)
			sc.text(sc.strs, op != "\"")
		}
	case "TJ":
		if len(args) > 0 {
			sc.strs = sc.strs[:0]
			if args[0].Kind == pdf.OperandArray {
				for i := range args[0].Array {
					if s, ok := args[0].Array[i].AsString(); ok {
						sc.strs = append(sc.strs, s)
					}
				}
			}
			sc.text(sc.strs, true)
		}
	case "d1":
		if sc.x.glyph {
			sc.x.noColour = true
		}
	}
	return nil
}

// pattern selects the pattern name, reads it when it is a tiling pattern
// and returns the colour space it sets. An uncoloured tiling pattern
// (PaintType 2) is painted in the colour of the stream that uses it: its
// colour operators are ignored.
func (sc *streamScan) pattern(name pdf.Name) colourUse {
	r := sc.s.c.r
	o := sc.resource("Pattern", name)
	st, ok := r.Resolve(o).(*pdf.Stream)
	if !ok {
		return colourUse{}
	}
	paintType, _ := pdf.AsInt(r.Resolve(st.Dict.Get("PaintType")))
	uncoloured := paintType == 2
	gs := sc.cur
	gs.fill.transparent, gs.stroke.transparent = false, false
	if uncoloured {
		gs.fill, gs.stroke = colourUse{}, colourUse{}
	}
	t := sc.s.nested(o, st.Dict.Get("Resources"), contentPattern, sc.x, sc.x.where+" /Pattern /"+string(name), &gs, uncoloured)
	return colourUse{transparent: t}
}

// extGState applies the graphics state parameter dictionary name.
func (sc *streamScan) extGState(name pdf.Name) {
	r := sc.s.c.r
	gd, ok := r.Dict(sc.resource("ExtGState", name))
	if !ok {
		return
	}
	g := &sc.cur
	if v := r.Resolve(gd.Get("SMask")); v != nil {
		mask, isDict := v.(pdf.Dict)
		g.softMask = isDict
		if isDict {
			if gr, ok := r.Resolve(mask.Get("G")).(*pdf.Stream); ok {
				sc.s.nested(mask.Get("G"), gr.Dict.Get("Resources"), contentMask, sc.x, sc.x.where+" /ExtGState /"+string(name)+" /SMask /G", nil, false)
			}
		}
	}
	if v := r.Resolve(gd.Get("BM")); v != nil {
		switch bm := v.(type) {
		case pdf.Name:
			g.blend = bm != "Normal"
		case pdf.Array:
			g.blend = len(bm) != 1
			if len(bm) == 1 {
				first, _ := r.Resolve(bm[0]).(pdf.Name)
				g.blend = first != "Normal"
			}
		default:
			g.blend = true
		}
	}
	if v, ok := number(r.Resolve(gd.Get("ca"))); ok {
		g.ca = v
	}
	if v, ok := number(r.Resolve(gd.Get("CA"))); ok {
		g.caStroke = v
	}
	if v, ok := r.Resolve(gd.Get("OP")).(pdf.Bool); ok {
		g.opStroke = bool(v)
		if gd.Get("op") == nil {
			g.opFill = bool(v)
		}
	}
	if v, ok := r.Resolve(gd.Get("op")).(pdf.Bool); ok {
		g.opFill = bool(v)
	}
	if v, ok := pdf.AsInt(r.Resolve(gd.Get("OPM"))); ok {
		g.opm = v
	}
}

// xobject paints the XObject name: an image uses transparency with a soft
// mask, a form with a transparency group or content that does.
func (sc *streamScan) xobject(name pdf.Name) {
	r := sc.s.c.r
	o := sc.resource("XObject", name)
	sc.paint(true, false, false)
	st, ok := r.Resolve(o).(*pdf.Stream)
	if !ok {
		return
	}
	switch sc.s.name(st.Dict, "Subtype") {
	case "Image":
		_, smask := r.Resolve(st.Dict.Get("SMask")).(*pdf.Stream)
		inData, _ := pdf.AsInt(r.Resolve(st.Dict.Get("SMaskInData")))
		if smask || inData > 0 {
			sc.transparent = true
		}
	case "Form":
		gs := sc.cur
		if sc.s.formContent(o, sc.x, sc.x.where+" /XObject /"+string(name), &gs) {
			sc.transparent = true
		}
	}
}

// text paints the strings of a text-showing operator, according to the
// text rendering mode, reads the glyphs they show of a Type 3 font and,
// with glyphs, checks those glyphs (check_pdfa_glyphs.go).
func (sc *streamScan) text(strs []pdf.String, glyphs bool) {
	g := &sc.cur
	var fill, stroke bool
	switch g.tr {
	case 0, 4:
		fill = true
	case 1, 5:
		stroke = true
	case 2, 6:
		fill, stroke = true, true
	}
	sc.paint(fill, stroke, true)
	if glyphs {
		sc.showGlyphs(strs)
	}
	if t := sc.type3(strs); t && (fill || stroke) {
		sc.transparent = true
	}
}

// type3 reads the descriptions of the glyphs strs show with the current
// font, when it is a Type 3 font, and returns whether one uses
// transparency.
func (sc *streamScan) type3(strs []pdf.String) bool {
	r := sc.s.c.r
	font := sc.cur.font
	d, ok := r.Dict(font)
	if !ok || sc.s.name(d, "Subtype") != "Type3" {
		return false
	}
	procs, ok := r.Dict(d.Get("CharProcs"))
	if !ok {
		return false
	}
	names := sc.s.type3Names(font, d)
	where := sc.x.where + " /Font /" + string(sc.cur.fontName) + " /CharProcs /"
	var seen [256]bool
	t := false
	for _, str := range strs {
		for _, b := range str {
			if seen[b] || names[b] == "" {
				continue
			}
			seen[b] = true
			p := procs.Get(names[b])
			if p == nil {
				continue
			}
			gs := sc.cur
			t = sc.s.nested(p, d.Get("Resources"), contentGlyph, sc.x, where+string(names[b]), &gs, sc.x.noColour) || t
		}
	}
	return t
}

// type3Names returns the glyph names of the codes of a Type 3 font, from
// the differences of its encoding.
func (s *pdfaScan) type3Names(font pdf.Object, d pdf.Dict) *[256]pdf.Name {
	cs := s.contentState()
	id := identity(font)
	if names := cs.encodings[id]; id != "" && names != nil {
		return names
	}
	r := s.c.r
	names := new([256]pdf.Name)
	if enc, ok := r.Dict(d.Get("Encoding")); ok {
		if diffs, ok := r.Resolve(enc.Get("Differences")).(pdf.Array); ok {
			code := int64(-1)
			for _, x := range diffs {
				switch v := r.Resolve(x).(type) {
				case pdf.Int:
					code = int64(v)
				case pdf.Name:
					if code >= 0 && code < 256 {
						names[code] = v
					}
					if code >= 0 {
						code++
					}
				}
			}
		}
	}
	if id != "" {
		cs.encodings[id] = names
	}
	return names
}

// inlineImage checks an inline image: its filters (6.1.10), Interpolate and
// bits per component (6.2.8), rendering intent (6.2.6) and colour space
// (6.2.4.3).
func (sc *streamScan) inlineImage(d pdf.Dict) {
	s := sc.s
	get := func(long, short pdf.Name) pdf.Object {
		if v := d.Get(long); v != nil {
			return v
		}
		return d.Get(short)
	}
	var filters []pdf.Name
	switch f := get("Filter", "F").(type) {
	case pdf.Name:
		filters = []pdf.Name{f}
	case pdf.Array:
		for _, x := range f {
			if n, ok := x.(pdf.Name); ok {
				filters = append(filters, n)
			}
		}
	}
	for _, f := range filters {
		long := f
		if l, ok := inlineFilters[f]; ok {
			long = l
		}
		switch long {
		case "ASCIIHexDecode", "ASCII85Decode", "FlateDecode", "RunLengthDecode", "CCITTFaxDecode", "DCTDecode":
		case "LZWDecode":
			sc.once("PDF-38", "inline "+string(f), "an inline image uses the %s filter, which PDF/A forbids (6.1.10)", f)
		default:
			sc.once("PDF-38", "inline "+string(f), "an inline image uses the filter %s, which PDF/A does not admit for inline images (6.1.10)", f)
		}
	}
	if b, _ := get("Interpolate", "I").(pdf.Bool); b {
		sc.once("PDF-56", "inline I", "an inline image has Interpolate true, which PDF/A forbids (6.2.8)")
	}
	mask, _ := get("ImageMask", "IM").(pdf.Bool)
	if bpc, ok := pdf.AsInt(get("BitsPerComponent", "BPC")); ok {
		switch {
		case bool(mask):
			if bpc != 1 {
				sc.once("PDF-56", "inline BPC", "an inline image mask has %d bits per component; PDF/A requires 1 (6.2.8)", bpc)
			}
		case bpc != 1 && bpc != 2 && bpc != 4 && bpc != 8 && bpc != 16:
			sc.once("PDF-56", "inline BPC", "an inline image has %d bits per component; PDF/A admits 1, 2, 4, 8 and 16 (6.2.8)", bpc)
		}
	}
	if ri := d.Get("Intent"); ri != nil {
		if n, _ := ri.(pdf.Name); !renderingIntents[n] {
			sc.once("PDF-55", "inline Intent", "an inline image has the rendering intent %v, which is not one of RelativeColorimetric, AbsoluteColorimetric, Perceptual and Saturation (6.2.6)", ri)
		}
	}
	if mask {
		return
	}
	switch cs := get("ColorSpace", "CS").(type) {
	case pdf.Name:
		switch cs {
		case "G", "RGB", "CMYK", "DeviceGray", "DeviceRGB", "DeviceCMYK":
			sc.device(deviceColour(cs, sc.x.def))
		default:
			sc.resource("ColorSpace", cs)
		}
	case pdf.Array:
		if len(cs) > 1 && (cs[0] == pdf.Object(pdf.Name("I")) || cs[0] == pdf.Object(pdf.Name("Indexed"))) {
			if base, ok := cs[1].(pdf.Name); ok {
				switch base {
				case "G", "RGB", "CMYK", "DeviceGray", "DeviceRGB", "DeviceCMYK":
					sc.device(deviceColour(base, sc.x.def))
				default:
					sc.resource("ColorSpace", base)
				}
				return
			}
		}
		s.colourSpace(cs, sc.x.where+" (inline image)", sc.x.def, 0)
	}
}

// pageTransparency reports a page that uses transparency without a group
// colour space nor a PDF/A output intent (6.2.10).
func (s *pdfaScan) pageTransparency(page pdf.Dict, where string) {
	if g, ok := s.c.r.Dict(page.Get("Group")); ok && s.present(g, "CS") {
		return
	}
	s.report("PDF-58", where, "the page uses transparency but has no transparency group colour space (/Group /CS) and the document has no PDF/A output intent (6.2.10)")
}
