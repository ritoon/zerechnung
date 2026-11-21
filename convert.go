package zerechnung

import (
	"crypto/md5"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/icc"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmp"
)

// ErrNotConvertible is wrapped by the error of ConvertPDFA, and of Embed
// with EmbedOptions.ConvertPDFA, when the PDF cannot be converted to PDF/A.
var ErrNotConvertible = errors.New("zerechnung: the PDF cannot be converted to PDF/A-3")

// ConversionError lists why ConvertPDFA cannot convert a PDF. It wraps
// ErrNotConvertible.
type ConversionError struct {
	// Reasons are the obstacles met, each with the place where it was met
	// (page, resource, annotation, object number of the converted file).
	Reasons []string
}

func (e *ConversionError) Error() string {
	const shown = 8
	r := e.Reasons
	more := ""
	if len(r) > shown {
		more = fmt.Sprintf("; and %d more", len(r)-shown)
		r = r[:shown]
	}
	return ErrNotConvertible.Error() + ": " + strings.Join(r, "; ") + more
}

// Unwrap returns ErrNotConvertible.
func (e *ConversionError) Unwrap() error { return ErrNotConvertible }

// ConvertOptions tune ConvertPDFA. The zero value (or nil) is ready to use.
type ConvertOptions struct {
	// ModDate is written to xmp:ModifyDate and xmp:MetadataDate, to the
	// ModDate of the document information dictionary when it has one, and
	// to the embedded files that have no modification date; time.Now()
	// when zero. Set it for deterministic output.
	ModDate time.Time
}

// maxConvertObjects bounds the objects ConvertPDFA reads: the limit of
// PDF/A (ISO 19005-2, 6.1.13) is 8388607 indirect objects.
const maxConvertObjects = 1 << 22

// maxNesting is the deepest nesting of q operators PDF/A admits
// (ISO 19005-2, 6.1.13).
const maxNesting = 28

// ConvertPDFA converts a PDF 1.x file to PDF/A-3, level B (or the level A
// or U its XMP metadata already declares), without rendering it and
// without any program besides the Go standard library. It rewrites the
// whole file, so that its syntax is the one PDF/A requires, and:
//
//   - adds a PDF/A output intent: the ICC profile of an output intent the
//     document already has, or an sRGB profile computed by the package;
//   - declares PDF/A-3 in the XMP metadata, merged with the existing packet
//     (rebuilt from the document information dictionary when it holds
//     properties PDF/A does not admit), and gives the trailer an /ID;
//   - removes JavaScript, the actions PDF/A forbids (from the open action,
//     the outlines and the annotations, with the actions of widgets),
//     additional actions (/AA), alternate presentations, presentation
//     steps, /Requirements, an XFA form and usage rights;
//   - re-encodes LZW streams with Flate, and removes from graphics states
//     transfer functions, /HTP and non-standard halftones, rendering
//     intents and blend modes PDF/A does not admit (an unknown blend mode
//     is rendered Normal) and overprint mode 1, from images /Alternates,
//     /OPI and Interpolate, from form XObjects their PostScript and
//     reference entries; a PostScript XObject, which viewers do not render,
//     becomes an empty form;
//   - gives visible annotations the Print flag and keeps their normal
//     appearance only; names the optional content configurations, removes
//     their /AS and completes their /Order;
//   - writes in each page the resources it inherits from the page tree, and
//     in each form XObject, pattern, appearance stream and Type 3 font that
//     has none the resources it uses (an appearance stream of a widget
//     takes them from the default resources of the form);
//   - removes the /CIDSet and /CharSet of the font descriptors, optional
//     in PDF/A-2 and PDF/A-3 and wrong as soon as they are incomplete, and
//     the object metadata streams that PDF/A would reject; writes the
//     default /CIDToGIDMap (Identity) of Type 2 CIDFonts;
//   - completes the file specifications as PDF/A-3 requires: /AFRelationship
//     (Unspecified), and for an embedded file /F and /UF, MIME type and
//     modification date; a file specification that no /AF array lists is
//     listed in the /AF array of the catalog (of its annotation for a file
//     attachment).
//
// It reads the content streams the pages use, with the form XObjects,
// patterns, soft masks, Type 3 glyphs and annotation appearances they use,
// and refuses what it cannot convert without a PDF engine: a font that
// shows visible text without its font program embedded, DeviceCMYK (or
// DeviceRGB) without an output intent or default colour space of its
// colour model (the sRGB profile covers RGB and grey only), inline images
// with LZW data, Interpolate or a non-standard filter, operators ISO
// 32000-1 does not define, operands beyond the limits of PDF/A or
// malformed hexadecimal strings, rendering intents of the ri operator
// PDF/A does not admit, q operators nested more than 28 deep. It also
// refuses PDF 2.0 files (PDF/A-4 is not written), signed documents
// (rewriting the file would invalidate the signatures) and dynamic XFA
// forms. Last, it checks the converted file with the PDF/A rules of
// CheckContainer (PDF-01 to PDF-08 and PDF-31 to PDF-72, the warnings
// PDF-53, PDF-59 and PDF-60 included): any error left is a reason to
// refuse, such as a glyph the font program lacks or JPEG 2000 data PDF/A
// does not admit.
//
// The error is a *ConversionError, which wraps ErrNotConvertible and lists
// every reason found, or ErrEncrypted, or an error reading the PDF. A
// converted file is not proof of PDF/A conformance: what CheckContainer
// does not read (Type 1 font programs, the value types of XMP extension
// schemas, the requirements of levels A and U) is not converted either.
// The CI checks the converted files with veraPDF.
func ConvertPDFA(pdfData []byte, opts *ConvertOptions) ([]byte, error) {
	var o ConvertOptions
	if opts != nil {
		o = *opts
	}
	if o.ModDate.IsZero() {
		o.ModDate = time.Now()
	}
	o.ModDate = o.ModDate.Truncate(time.Second)

	r, err := pdf.Open(pdfData)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	c := &converter{
		r:       r,
		date:    o.ModDate,
		objs:    map[uint32]pdf.Object{},
		seen:    map[pdf.Ref]uint16{},
		actions: map[pdf.Ref]pdf.Object{},
		afRefs:  map[pdf.Ref]bool{},
		fonts:   map[pdf.Ref]bool{},

		read:      map[readKey]bool{},
		inherited: map[pdf.Ref]given{},
	}
	v := (&container{r: r, catalog: r.Catalog()}).version()
	if !versionLess(v, "2.0") {
		return nil, &ConversionError{Reasons: []string{fmt.Sprintf("header: the PDF version is %s; ConvertPDFA writes PDF/A-3, based on PDF 1.7, not PDF/A-4", v)}}
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	c.signatures()
	c.generic()
	c.document()
	c.pages()
	c.associateFiles()
	space := c.outputIntent()
	if c.rgbAt != "" && space != "RGB " && space != "?" {
		c.refuse(c.rgbAt, "DeviceRGB is used without a DefaultRGB colour space, and the output intent profile is %s", strings.TrimSpace(space))
	}
	if c.cmykAt != "" && space != "CMYK" && space != "?" {
		c.refuse(c.cmykAt, "DeviceCMYK is used without a DefaultCMYK colour space nor a CMYK output intent; the conversion only adds an sRGB output intent, which does not cover CMYK: produce the PDF in RGB, or with a CMYK PDF/A output intent")
	}
	c.metadata()
	out := c.write(pdfData)

	reasons := c.reasons
	seen := map[string]bool{}
	for _, s := range reasons {
		seen[s] = true
	}
	for _, s := range pdfaErrors(out) {
		if !seen[s] {
			seen[s] = true
			reasons = append(reasons, s)
		}
	}
	if len(reasons) > 0 {
		return nil, &ConversionError{Reasons: reasons}
	}
	return out, nil
}

// refusedWarnings are the warnings of CheckContainer that ConvertPDFA takes
// as errors: what they report, veraPDF reports as a failure, and the
// conversion does not repair.
var refusedWarnings = map[string]bool{"PDF-53": true, "PDF-59": true, "PDF-60": true}

// pdfaErrors returns the errors CheckContainer reports on the PDF/A
// requirements of a document, the invoice aside, and the warnings of
// refusedWarnings.
func pdfaErrors(data []byte) []string {
	c, err := openContainer(data)
	if err != nil {
		return []string{err.Error()}
	}
	k := &containerCheck{}
	info := k.document(c)
	k.pdfa(c, nil, info)
	var out []string
	for _, f := range k.findings {
		if f.Severity != rules.Error && !refusedWarnings[f.Rule] {
			continue
		}
		s := f.Rule + ": " + f.Message
		if f.Path != "" {
			s = f.Rule + " (" + f.Path + "): " + f.Message
		}
		out = append(out, s)
	}
	return out
}

// converter holds the objects of the document being converted: deep copies
// of the objects reachable from the trailer, by object number, which the
// conversion modifies in place. An entry is removed from a dictionary by
// setting its value to null (del): the writer skips null entries, and every
// copy of the dictionary sees the change.
type converter struct {
	r    *pdf.Reader
	date time.Time
	objs map[uint32]pdf.Object
	next uint32 // next free object number
	root uint32
	info uint32 // 0 when the document has no information dictionary

	seen    map[pdf.Ref]uint16     // roles in which an object was visited
	actions map[pdf.Ref]pdf.Object // actions cleaned, by reference
	afRefs  map[pdf.Ref]bool       // file specifications listed in an /AF array
	fonts   map[pdf.Ref]bool       // unembedded fonts reported
	reasons []string

	read      map[readKey]bool  // content streams read, by context
	inherited map[pdf.Ref]given // resources given to content streams that had none

	// Where DeviceRGB and DeviceCMYK are first used without a default
	// colour space ("" when they are not).
	rgbAt, cmykAt string
}

func (c *converter) refuse(where, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if where != "" {
		msg = where + ": " + msg
	}
	for _, s := range c.reasons {
		if s == msg {
			return
		}
	}
	c.reasons = append(c.reasons, msg)
}

// clone returns a deep copy of o; the raw data of streams is shared.
func clone(o pdf.Object) pdf.Object {
	switch v := o.(type) {
	case pdf.Dict:
		d := make(pdf.Dict, len(v))
		for i, kv := range v {
			d[i] = pdf.KV{Key: kv.Key, Val: clone(kv.Val)}
		}
		return d
	case pdf.Array:
		a := make(pdf.Array, len(v))
		for i, x := range v {
			a[i] = clone(x)
		}
		return a
	case *pdf.Stream:
		return &pdf.Stream{Dict: clone(v.Dict).(pdf.Dict), Raw: v.Raw}
	}
	return o
}

// refs calls fn with the references held by o, streams included.
func refs(o pdf.Object, fn func(pdf.Ref)) {
	switch v := o.(type) {
	case pdf.Ref:
		fn(v)
	case pdf.Dict:
		for _, kv := range v {
			refs(kv.Val, fn)
		}
	case pdf.Array:
		for _, x := range v {
			refs(x, fn)
		}
	case *pdf.Stream:
		refs(v.Dict, fn)
	}
}

// load copies the objects reachable from the root and the document
// information dictionary.
func (c *converter) load() error {
	t := c.r.Trailer()
	queued := map[uint32]bool{}
	var queue []uint32
	enqueue := func(ref pdf.Ref) {
		if !queued[ref.Num] {
			queued[ref.Num] = true
			queue = append(queue, ref.Num)
		}
	}
	top := func(key pdf.Name) uint32 {
		switch v := t.Get(key).(type) {
		case pdf.Ref:
			enqueue(v)
			return v.Num
		case pdf.Dict:
			// A direct dictionary in the trailer: it gets a number.
			d := clone(v)
			refs(d, enqueue)
			c.next = max(c.next, 1<<31) // numbered after every object
			return c.alloc(d).Num
		}
		return 0
	}
	c.root = top("Root")
	c.info = top("Info")
	for len(queue) > 0 {
		num := queue[0]
		queue = queue[1:]
		if len(c.objs) >= maxConvertObjects {
			return fmt.Errorf("zerechnung: the PDF has more than %d objects", maxConvertObjects)
		}
		o := clone(c.r.Object(num))
		c.objs[num] = o
		c.next = max(c.next, num+1)
		refs(o, enqueue)
	}
	if size, ok := t.Int("Size"); ok && size > int64(c.next) && size < 1<<31 {
		c.next = uint32(size)
	}
	if _, ok := c.objs[c.root].(pdf.Dict); !ok {
		return fmt.Errorf("zerechnung: %w", pdf.ErrNoCatalog)
	}
	return nil
}

// alloc adds a new object.
func (c *converter) alloc(o pdf.Object) pdf.Ref {
	if c.next == 0 {
		c.next = 1
	}
	ref := pdf.Ref{Num: c.next}
	c.next++
	c.objs[ref.Num] = o
	return ref
}

// resolve follows references through the copied objects.
func (c *converter) resolve(o pdf.Object) pdf.Object {
	for range 32 {
		ref, ok := o.(pdf.Ref)
		if !ok {
			return o
		}
		o = c.objs[ref.Num]
	}
	return nil
}

func (c *converter) dict(o pdf.Object) (pdf.Dict, bool) {
	return pdf.AsDict(c.resolve(o))
}

func (c *converter) name(d pdf.Dict, key pdf.Name) pdf.Name {
	n, _ := c.resolve(d.Get(key)).(pdf.Name)
	return n
}

func (c *converter) text(d pdf.Dict, key pdf.Name) string {
	if s, ok := c.resolve(d.Get(key)).(pdf.String); ok {
		return pdf.Text(s)
	}
	return ""
}

// store records d as the new value of o: in place of the object when o is
// a reference. It returns what the holder of o keeps: o itself, or d for a
// direct dictionary.
func (c *converter) store(o pdf.Object, d pdf.Object) pdf.Object {
	if ref, ok := o.(pdf.Ref); ok {
		if st, ok := c.objs[ref.Num].(*pdf.Stream); ok {
			if dd, ok := d.(pdf.Dict); ok {
				st.Dict = dd
				return o
			}
		}
		c.objs[ref.Num] = d
		return o
	}
	if st, ok := o.(*pdf.Stream); ok {
		if dd, ok := d.(pdf.Dict); ok {
			st.Dict = dd
			return o
		}
	}
	return d
}

// visit reports whether o is to be visited in role: an indirect object is
// visited once per role.
func (c *converter) visit(o pdf.Object, role uint16) bool {
	if ref, ok := o.(pdf.Ref); ok {
		if c.seen[ref]&role != 0 {
			return false
		}
		c.seen[ref] |= role
	}
	return true
}

// del removes key from d in place (see converter).
func del(d pdf.Dict, keys ...pdf.Name) {
	for i := range d {
		for _, k := range keys {
			if d[i].Key == k {
				d[i].Val = nil
			}
		}
	}
}

// set sets key to v in d, or removes it when v is nil; the result must be
// stored, d may have grown.
func set(d pdf.Dict, key pdf.Name, v pdf.Object) pdf.Dict {
	if v == nil {
		del(d, key)
		return d
	}
	d.Set(key, v)
	return d
}

// signatures refuses signed documents: rewriting the file invalidates the
// signatures.
func (c *converter) signatures() {
	cat := c.objs[c.root].(pdf.Dict)
	if perms, ok := c.dict(cat.Get("Perms")); ok && c.resolve(perms.Get("DocMDP")) != nil {
		c.refuse("catalog /Perms /DocMDP", "the document is certified (signed); converting it would invalidate the signature")
	}
	form, ok := c.dict(cat.Get("AcroForm"))
	if !ok {
		return
	}
	var walk func(o pdf.Object, ft pdf.Name, depth int)
	walk = func(o pdf.Object, ft pdf.Name, depth int) {
		if depth > maxScanDepth || !c.visit(o, roleField) {
			return
		}
		d, ok := c.dict(o)
		if !ok {
			return
		}
		if t := c.name(d, "FT"); t != "" {
			ft = t
		}
		if ft == "Sig" && c.resolve(d.Get("V")) != nil {
			c.refuse(fmt.Sprintf("form field %q", c.text(d, "T")), "the document is signed; converting it would invalidate the signature")
		}
		if kids, ok := c.resolve(d.Get("Kids")).(pdf.Array); ok {
			for _, k := range kids {
				walk(k, ft, depth+1)
			}
		}
	}
	if fields, ok := c.resolve(form.Get("Fields")).(pdf.Array); ok {
		for _, f := range fields {
			walk(f, "", 0)
		}
	}
	clear(c.seen)
}

// generic applies the changes that do not depend on where an object is
// used to every object: additional actions, font descriptors, CIDFonts,
// graphics states, file specifications, images, form and PostScript
// XObjects, LZW streams and object metadata streams. It records the file
// specifications listed in /AF arrays.
func (c *converter) generic() {
	nums := make([]uint32, 0, len(c.objs))
	for n := range c.objs {
		nums = append(nums, n)
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	var docXMP xmp.Declared
	if st, ok := c.resolve(c.objs[c.root].(pdf.Dict).Get("Metadata")).(*pdf.Stream); ok {
		if data, err := c.r.Decode(st); err == nil {
			if info, err := xmp.Parse(data); err == nil {
				docXMP = info.Declared()
			}
		}
	}
	for _, n := range nums {
		c.objs[n] = c.fixObject(c.objs[n], fmt.Sprintf("object %d", n), n == c.root, docXMP, 0)
	}
}

func (c *converter) fixObject(o pdf.Object, where string, catalog bool, docXMP xmp.Declared, depth int) pdf.Object {
	if depth > 256 {
		return o
	}
	switch v := o.(type) {
	case pdf.Array:
		for i := range v {
			v[i] = c.fixObject(v[i], where, false, docXMP, depth+1)
		}
	case pdf.Dict:
		for i := range v {
			v[i].Val = c.fixObject(v[i].Val, where, false, docXMP, depth+1)
		}
		return c.fixDict(v, catalog, docXMP)
	case *pdf.Stream:
		for i := range v.Dict {
			v.Dict[i].Val = c.fixObject(v.Dict[i].Val, where, false, docXMP, depth+1)
		}
		v.Dict = c.fixDict(v.Dict, false, docXMP)
		c.fixStream(v, where)
	}
	return o
}

func (c *converter) fixDict(d pdf.Dict, catalog bool, docXMP xmp.Declared) pdf.Dict {
	del(d, "AA")
	switch c.name(d, "Type") {
	case "FontDescriptor":
		del(d, "CIDSet", "CharSet")
	case "ExtGState":
		c.extGState(d)
	}
	if c.name(d, "Subtype") == "CIDFontType2" && c.resolve(d.Get("CIDToGIDMap")) == nil {
		// Identity, the default of ISO 32000-1 (9.7.4.2), which PDF/A
		// wants written (6.2.11.3.2).
		d = set(d, "CIDToGIDMap", pdf.Name("Identity"))
	}
	// The graphics states of a resource dictionary or a shading pattern.
	if states, ok := c.dict(d.Get("ExtGState")); ok {
		if d.Get("PatternType") != nil {
			c.extGState(states)
		} else {
			for _, gs := range states {
				c.extGState(gs.Val)
			}
		}
	}
	if _, ok := c.dict(d.Get("EF")); ok {
		d = c.fileSpec(d)
	} else if c.name(d, "Type") == "Filespec" && c.resolve(d.Get("AFRelationship")) == nil {
		// veraPDF wants /AFRelationship in every file specification (6.8).
		d = set(d, "AFRelationship", pdf.Name("Unspecified"))
	}
	c.associate(d.Get("AF"))
	if !catalog && d.Get("Metadata") != nil {
		if st, ok := c.resolve(d.Get("Metadata")).(*pdf.Stream); ok {
			data, err := c.r.Decode(st)
			bad := err != nil
			if !bad {
				xmp.CheckPDFA(data, docXMP, func(xmp.Issue) { bad = true })
			}
			if bad {
				del(d, "Metadata")
			}
		}
	}
	return d
}

// fileSpec completes the file specification of an embedded file as
// PDF/A-3 requires (6.8).
func (c *converter) fileSpec(d pdf.Dict) pdf.Dict {
	if c.resolve(d.Get("AFRelationship")) == nil {
		d = set(d, "AFRelationship", pdf.Name("Unspecified"))
	}
	name := c.text(d, "UF")
	if name == "" {
		name = c.text(d, "F")
	}
	if name != "" {
		if c.resolve(d.Get("UF")) == nil {
			d = set(d, "UF", pdf.EncodeText(name))
		}
		if c.resolve(d.Get("F")) == nil {
			d = set(d, "F", pdf.EncodeText(name))
		}
	}
	ef, _ := c.dict(d.Get("EF"))
	for _, key := range []pdf.Name{"F", "UF"} {
		st, ok := c.resolve(ef.Get(key)).(*pdf.Stream)
		if !ok {
			continue
		}
		if c.resolve(st.Dict.Get("Subtype")) == nil {
			st.Dict = set(st.Dict, "Subtype", pdf.Name(mimeType(name)))
		}
		params, _ := c.dict(st.Dict.Get("Params"))
		if c.resolve(params.Get("ModDate")) == nil {
			params = set(params.Clone(), "ModDate", pdf.String(pdf.FormatDate(c.date)))
			st.Dict = set(st.Dict, "Params", c.store(st.Dict.Get("Params"), params))
		}
	}
	return d
}

// fixStream changes images, form and PostScript XObjects, and LZW streams.
func (c *converter) fixStream(st *pdf.Stream, where string) {
	d := st.Dict
	if c.name(d, "Type") != "EmbeddedFile" {
		switch c.name(d, "Subtype") {
		case "Image":
			del(d, "Alternates", "OPI")
			if b, _ := c.resolve(d.Get("Interpolate")).(pdf.Bool); b {
				del(d, "Interpolate")
			}
			if ri := c.resolve(d.Get("Intent")); ri != nil {
				if n, _ := ri.(pdf.Name); !renderingIntents[n] {
					del(d, "Intent")
				}
			}
		case "Form":
			del(d, "OPI", "PS", "Ref")
			if c.name(d, "Subtype2") == "PS" {
				del(d, "Subtype2")
			}
		case "PS":
			// Viewers do not render PostScript XObjects: an empty form
			// draws the same.
			st.Dict = pdf.Dict{{Key: "Type", Val: pdf.Name("XObject")}, {Key: "Subtype", Val: pdf.Name("Form")},
				{Key: "BBox", Val: pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(0), pdf.Int(0)}}}
			st.Raw = nil
			return
		}
	}
	c.transcodeLZW(st, where)
}

// filters returns the filters of a stream dictionary and their decode
// parameters.
func (c *converter) filters(d pdf.Dict) (names []pdf.Name, parms []pdf.Object) {
	switch f := c.resolve(d.Get("Filter")).(type) {
	case pdf.Name:
		return []pdf.Name{f}, []pdf.Object{c.resolve(d.Get("DecodeParms"))}
	case pdf.Array:
		dp, _ := c.resolve(d.Get("DecodeParms")).(pdf.Array)
		for i, x := range f {
			n, _ := c.resolve(x).(pdf.Name)
			names = append(names, n)
			var p pdf.Object
			if i < len(dp) {
				p = c.resolve(dp[i])
			}
			parms = append(parms, p)
		}
	}
	return names, parms
}

// transcodeLZW replaces the LZWDecode filters of a stream, which PDF/A
// forbids, with FlateDecode: the filters up to the last LZWDecode one are
// decoded, the data compressed with Flate, the next filters kept.
func (c *converter) transcodeLZW(st *pdf.Stream, where string) {
	names, parms := c.filters(st.Dict)
	last := -1
	for i, n := range names {
		if n == "LZWDecode" || n == "LZW" {
			last = i
		}
	}
	if last < 0 {
		return
	}
	prefix := make(pdf.Array, last+1)
	prefixParms := make(pdf.Array, last+1)
	for i := 0; i <= last; i++ {
		prefix[i], prefixParms[i] = names[i], parms[i]
	}
	data, err := c.r.Decode(&pdf.Stream{Dict: pdf.Dict{{Key: "Filter", Val: prefix}, {Key: "DecodeParms", Val: prefixParms}}, Raw: st.Raw})
	if err != nil {
		c.refuse(where, "the LZWDecode stream cannot be decoded to be re-encoded with Flate: %v", err)
		return
	}
	newNames := pdf.Array{pdf.Name("FlateDecode")}
	newParms := pdf.Array{nil}
	hasParms := false
	for i := last + 1; i < len(names); i++ {
		newNames = append(newNames, names[i])
		newParms = append(newParms, parms[i])
		hasParms = hasParms || parms[i] != nil
	}
	st.Raw = pdf.Deflate(data)
	d := st.Dict
	if len(newNames) == 1 {
		d = set(d, "Filter", newNames[0])
	} else {
		d = set(d, "Filter", newNames)
	}
	switch {
	case !hasParms:
		d = set(d, "DecodeParms", nil)
	case len(newParms) == 1:
		d = set(d, "DecodeParms", newParms[0])
	default:
		d = set(d, "DecodeParms", newParms)
	}
	st.Dict = d
}

// document changes the catalog: actions, name dictionary, interactive form,
// usage rights, optional content and outlines.
func (c *converter) document() {
	cat := c.objs[c.root].(pdf.Dict)
	del(cat, "Version", "Requirements")
	if o := cat.Get("OpenAction"); o != nil {
		cat = set(cat, "OpenAction", c.action(o, 0))
	}
	if names, ok := c.dict(cat.Get("Names")); ok {
		del(names, "JavaScript", "AlternatePresentations")
	}
	if b, _ := c.resolve(cat.Get("NeedsRendering")).(pdf.Bool); b {
		c.refuse("catalog /NeedsRendering", "the document is a dynamic XFA form, whose pages a viewer renders from the XFA data")
	} else {
		del(cat, "NeedsRendering")
		if form, ok := c.dict(cat.Get("AcroForm")); ok {
			del(form, "XFA")
		}
	}
	if perms, ok := c.dict(cat.Get("Perms")); ok {
		// Usage rights are signed: the rewritten file would void them.
		del(perms, "UR", "UR3")
		if c.resolve(perms.Get("DocMDP")) == nil {
			del(cat, "Perms")
		}
	}
	if oc, ok := c.dict(cat.Get("OCProperties")); ok {
		c.optionalContent(oc)
	}
	c.outlines(cat.Get("Outlines"))
	c.objs[c.root] = cat
}

// action returns the action o without the actions PDF/A forbids: nil when
// the action itself is forbidden, which drops the actions chained to it,
// the action with its /Next actions cleaned otherwise. A destination is
// returned as is.
func (c *converter) action(o pdf.Object, depth int) pdf.Object {
	d, ok := c.resolve(o).(pdf.Dict)
	if !ok || depth > maxScanDepth {
		return o
	}
	ref, isRef := o.(pdf.Ref)
	if isRef {
		if v, ok := c.actions[ref]; ok {
			return v
		}
		c.actions[ref] = o // guards against cycles
	}
	var res pdf.Object
	typ := c.name(d, "S")
	if !forbiddenActions[typ] && (typ != "Named" || allowedNamed[c.name(d, "N")]) {
		switch next := c.resolve(d.Get("Next")).(type) {
		case pdf.Dict:
			d = set(d, "Next", c.action(d.Get("Next"), depth+1))
		case pdf.Array:
			var kept pdf.Array
			for _, x := range next {
				if a := c.action(x, depth+1); a != nil {
					kept = append(kept, a)
				}
			}
			if len(kept) == 0 {
				d = set(d, "Next", nil)
			} else {
				d = set(d, "Next", c.store(d.Get("Next"), kept))
			}
		}
		res = c.store(o, d)
	}
	if isRef {
		c.actions[ref] = res
	}
	return res
}

// outlines cleans the actions of the outline items.
func (c *converter) outlines(o pdf.Object) {
	root, ok := c.dict(o)
	if !ok {
		return
	}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		for o != nil && depth <= maxScanDepth && c.visit(o, roleOutline) {
			d, ok := c.dict(o)
			if !ok {
				return
			}
			if a := d.Get("A"); a != nil {
				d = set(d, "A", c.action(a, 0))
				c.store(o, d)
			}
			walk(d.Get("First"), depth+1)
			o = d.Get("Next")
		}
	}
	walk(root.Get("First"), 0)
}

// optionalContent names the optional content configurations, removes
// their /AS entry and completes their /Order array (6.9).
func (c *converter) optionalContent(oc pdf.Dict) {
	ocgs, _ := c.resolve(oc.Get("OCGs")).(pdf.Array)
	used := map[string]bool{}
	fix := func(o pdf.Object, def string) pdf.Object {
		d, ok := c.dict(o)
		if !ok {
			return o
		}
		del(d, "AS")
		name := c.text(d, "Name")
		if name == "" || used[name] {
			name = def
			for i := 2; used[name]; i++ {
				name = fmt.Sprintf("%s %d", def, i)
			}
			d = set(d, "Name", pdf.EncodeText(name))
		}
		used[name] = true
		if order, ok := c.resolve(d.Get("Order")).(pdf.Array); ok {
			listed := map[pdf.Ref]bool{}
			var walk func(a pdf.Array, depth int)
			walk = func(a pdf.Array, depth int) {
				for _, x := range a {
					if sub, ok := c.resolve(x).(pdf.Array); ok && depth < maxScanDepth {
						walk(sub, depth+1)
					} else if ref, ok := x.(pdf.Ref); ok {
						listed[ref] = true
					}
				}
			}
			walk(order, 0)
			grown := order
			for _, x := range ocgs {
				if ref, ok := x.(pdf.Ref); ok && !listed[ref] {
					grown = append(grown, ref)
					listed[ref] = true
				}
			}
			if len(grown) != len(order) {
				d = set(d, "Order", c.store(d.Get("Order"), grown))
			}
		}
		return c.store(o, d)
	}
	if o := oc.Get("D"); o != nil {
		oc = set(oc, "D", fix(o, "Default"))
	}
	if list, ok := c.resolve(oc.Get("Configs")).(pdf.Array); ok {
		for i := range list {
			list[i] = fix(list[i], fmt.Sprintf("Configuration %d", i+1))
		}
	}
	c.store(c.objs[c.root].(pdf.Dict).Get("OCProperties"), oc)
}

// associate records the file specifications listed in an /AF array.
func (c *converter) associate(o pdf.Object) {
	if list, ok := c.resolve(o).(pdf.Array); ok {
		for _, x := range list {
			if ref, ok := x.(pdf.Ref); ok {
				c.afRefs[ref] = true
			}
		}
	}
}

// pages walks the page tree and reads the content of the pages and of
// their annotations.
func (c *converter) pages() {
	cat := c.objs[c.root].(pdf.Dict)
	var dr pdf.Object // default resources of the interactive form
	if form, ok := c.dict(cat.Get("AcroForm")); ok {
		dr = form.Get("DR")
	}
	n := 0
	var walk func(o, res pdf.Object, depth int)
	walk = func(o, res pdf.Object, depth int) {
		if depth > maxScanDepth || !c.visit(o, roleNode) {
			return
		}
		d, ok := c.dict(o)
		if !ok {
			return
		}
		own := d.Get("Resources") != nil
		if own {
			res = d.Get("Resources")
		}
		if kids, ok := c.resolve(d.Get("Kids")).(pdf.Array); ok && c.name(d, "Type") != "Page" {
			for _, k := range kids {
				walk(k, res, depth+1)
			}
			return
		}
		n++
		where := fmt.Sprintf("page %d", n)
		del(d, "PresSteps")
		if !own && res != nil {
			// The resources inherited from the page tree are written in
			// the page: veraPDF wants each content stream to have its own
			// (6.2.2).
			d = set(d, "Resources", res)
			c.store(o, d)
		}
		var def uint8
		if rd, ok := c.dict(res); ok {
			def = c.defaults(rd)
		}
		if g, ok := c.dict(d.Get("Group")); ok {
			c.colourSpace(g.Get("CS"), where+" /Group /CS", def, 0)
		}
		var content []byte
		ok = true
		switch v := c.resolve(d.Get("Contents")).(type) {
		case *pdf.Stream:
			content, ok = c.decode(v, where+" /Contents")
		case pdf.Array:
			for i, x := range v {
				if st, isStream := c.resolve(x).(*pdf.Stream); isStream {
					data, good := c.decode(st, fmt.Sprintf("%s /Contents [%d]", where, i))
					ok = ok && good
					content = append(append(content, data...), '\n')
				}
			}
		}
		if ok {
			c.scan(content, scanCtx{res: res, def: def, where: where + " /Contents"})
		}
		if annots, ok := c.resolve(d.Get("Annots")).(pdf.Array); ok {
			for i := range annots {
				annots[i] = c.annotation(annots[i], dr, fmt.Sprintf("%s /Annots [%d]", where, i))
			}
		}
	}
	walk(cat.Get("Pages"), nil, 0)
}

// decode decodes a content stream; a stream that cannot be decoded is a
// reason to refuse, its content not being known.
func (c *converter) decode(st *pdf.Stream, where string) ([]byte, bool) {
	data, err := c.r.Decode(st)
	if err != nil {
		c.refuse(where, "the content stream cannot be decoded: %v", err)
		return nil, false
	}
	return data, true
}

// defaults returns the default colour spaces set by a resource dictionary.
func (c *converter) defaults(res pdf.Dict) uint8 {
	cs, ok := c.dict(res.Get("ColorSpace"))
	if !ok {
		return 0
	}
	var def uint8
	if cs.Get("DefaultRGB") != nil {
		def |= defaultRGB
	}
	if cs.Get("DefaultCMYK") != nil {
		def |= defaultCMYK
	}
	if cs.Get("DefaultGray") != nil {
		def |= defaultGray
	}
	return def
}

// device records the use of a device colour space.
func (c *converter) device(n pdf.Name, where string, def uint8) {
	switch n {
	case "DeviceRGB", "RGB":
		if def&defaultRGB == 0 && c.rgbAt == "" {
			c.rgbAt = where
		}
	case "DeviceCMYK", "CMYK":
		if def&defaultCMYK == 0 && c.cmykAt == "" {
			c.cmykAt = where
		}
	}
}

// colourSpace records the device colour spaces a colour space is built on.
// DeviceGray is covered by any output intent.
func (c *converter) colourSpace(o pdf.Object, where string, def uint8, depth int) {
	if o == nil || depth > maxScanDepth {
		return
	}
	switch v := c.resolve(o).(type) {
	case pdf.Name:
		c.device(v, where, def)
	case pdf.Array:
		if len(v) == 0 {
			return
		}
		switch family, _ := c.resolve(v[0]).(pdf.Name); family {
		case "DeviceRGB", "DeviceCMYK", "RGB", "CMYK":
			c.device(family, where, def)
		case "Indexed", "I", "Pattern":
			if len(v) > 1 {
				c.colourSpace(v[1], where, def, depth+1)
			}
		case "Separation", "DeviceN":
			if len(v) > 2 {
				c.colourSpace(v[2], where+" (alternate)", def, depth+1)
			}
			if family == "DeviceN" && len(v) > 4 {
				if attrs, ok := c.dict(v[4]); ok {
					if proc, ok := c.dict(attrs.Get("Process")); ok {
						c.colourSpace(proc.Get("ColorSpace"), where+" (process)", def, depth+1)
					}
				}
			}
		}
	}
}

func indexOf(d pdf.Dict, key pdf.Name) int {
	for i := range d {
		if d[i].Key == key {
			return i
		}
	}
	return -1
}

// shading records the colour space of a shading.
func (c *converter) shading(o pdf.Object, where string, def uint8) {
	if d, ok := c.dict(o); ok {
		c.colourSpace(d.Get("ColorSpace"), where+" /ColorSpace", def, 0)
	}
}

// extGState removes from a graphics state what PDF/A forbids (6.2.5,
// 6.2.6, 6.2.10) and overprint mode 1, which 6.2.4.2 forbids with an
// ICCBased CMYK colour space: overprint mode 0 only differs in the
// simulation of overprinting.
func (c *converter) extGState(o pdf.Object) pdf.Object {
	g, ok := c.dict(o)
	if !ok {
		return o
	}
	del(g, "TR", "HTP")
	if tr2 := c.resolve(g.Get("TR2")); tr2 != nil {
		if n, _ := tr2.(pdf.Name); n != "Default" {
			del(g, "TR2")
		}
	}
	if ht := g.Get("HT"); ht != nil && !c.halftoneOK(ht, "", 0) {
		del(g, "HT")
	}
	if ri := c.resolve(g.Get("RI")); ri != nil {
		if n, _ := ri.(pdf.Name); !renderingIntents[n] {
			del(g, "RI")
		}
	}
	if opm, _ := pdf.AsInt(c.resolve(g.Get("OPM"))); opm == 1 {
		del(g, "OPM")
	}
	switch bm := c.resolve(g.Get("BM")).(type) {
	case pdf.Name:
		if !blendModes[bm] {
			// ISO 32000-1, 11.6.3: an unknown blend mode is Normal.
			g[indexOf(g, "BM")].Val = pdf.Name("Normal")
		}
	case pdf.Array:
		// The first blend mode the viewer knows is used.
		mode := pdf.Name("Normal")
		for _, x := range bm {
			if n, _ := c.resolve(x).(pdf.Name); blendModes[n] {
				mode = n
				break
			}
		}
		g[indexOf(g, "BM")].Val = mode
	}
	return o
}

// halftoneOK reports whether a halftone meets 6.2.5.
func (c *converter) halftoneOK(o pdf.Object, colourant pdf.Name, depth int) bool {
	d, ok := c.dict(o)
	if !ok {
		return true // the name Default
	}
	if depth > 1 || d.Get("HalftoneName") != nil {
		return false
	}
	typ, _ := pdf.AsInt(c.resolve(d.Get("HalftoneType")))
	if typ != 1 && typ != 5 {
		return false
	}
	if colourant != "Default" {
		primary := colourant == "" || colourant == "Cyan" || colourant == "Magenta" || colourant == "Yellow" || colourant == "Black"
		if primary == (d.Get("TransferFunction") != nil) {
			return false
		}
	}
	if typ == 5 {
		for _, kv := range d {
			switch kv.Key {
			case "Type", "HalftoneType", "HalftoneName":
				continue
			}
			if kv.Val != nil && !c.halftoneOK(kv.Val, kv.Key, depth+1) {
				return false
			}
		}
	}
	return true
}

// scanCtx is the context in which a content stream is read: its resource
// dictionary, the default colour spaces in effect, its place for the
// messages and the depth of the forms, patterns and glyphs that led to it.
type scanCtx struct {
	res   pdf.Object
	def   uint8
	where string
	depth int
}

// readKey identifies the reading of a form XObject, a tiling pattern, an
// appearance stream or the glyphs of a Type 3 font: the same object is read
// again only with other default colour spaces or, when it inherits them,
// other resources.
type readKey struct {
	num    uint32
	def    uint8
	parent string
}

// identity identifies a resource dictionary: its object number, or the
// address of a direct dictionary.
func identity(o pdf.Object) string {
	switch v := o.(type) {
	case pdf.Ref:
		return fmt.Sprintf("R%d", v.Num)
	case pdf.Dict:
		if len(v) > 0 {
			return fmt.Sprintf("%p", &v[0])
		}
	}
	return ""
}

// enter returns the context in which the content of o (a form XObject, a
// tiling pattern, an appearance stream or a Type 3 font, of dictionary d)
// is read when it is used in x, and whether it is to be read. Without its
// own /Resources, o uses those of x. A pattern does not see the default
// colour spaces of x, only those of its own resources.
func (c *converter) enter(o pdf.Object, d pdf.Dict, x scanCtx, where string, pattern bool) (in scanCtx, own, read bool) {
	if x.depth >= maxScanDepth {
		c.refuse(where, "forms, patterns, soft masks and glyphs are nested more than %d deep; the conversion does not read them", maxScanDepth)
		return in, false, false
	}
	ref, _ := o.(pdf.Ref)
	prev, wasGiven := c.inherited[ref]
	res := d.Get("Resources")
	own = res != nil && !wasGiven
	if !own {
		res = x.res
	}
	def := x.def
	if pattern {
		def = 0
	}
	if rd, ok := c.dict(res); ok && own {
		def |= c.defaults(rd)
	}
	key := readKey{num: ref.Num, def: def}
	if !own {
		key.parent = identity(x.res)
	}
	if ref.Num != 0 {
		if c.read[key] {
			return in, own, false
		}
		c.read[key] = true
	}
	if wasGiven {
		// Already read, and given the resources it uses: they must be
		// the same here.
		if !reflect.DeepEqual(prev.res, c.minimal(x.res, prev.used)) {
			c.refuse(where, "the content stream has no resource dictionary of its own and is used with different resources; PDF/A requires it to have its own (6.2.2)")
		}
		return in, own, false
	}
	return scanCtx{res: res, def: def, where: where, depth: x.depth + 1}, own, true
}

// resUse lists the named resources a content stream uses, by category
// (Font, XObject...).
type resUse map[pdf.Name]map[pdf.Name]bool

func (u *resUse) add(category, name pdf.Name) {
	if *u == nil {
		*u = resUse{}
	}
	if (*u)[category] == nil {
		(*u)[category] = map[pdf.Name]bool{}
	}
	(*u)[category][name] = true
}

func (u *resUse) merge(v resUse) {
	for category, names := range v {
		for n := range names {
			u.add(category, n)
		}
	}
}

// given are the resources given to a content stream that had none: the
// names it uses and the dictionary built for them.
type given struct {
	used resUse
	res  pdf.Dict
}

// resourceCategories are the categories of a resource dictionary, in the
// order they are written.
var resourceCategories = []pdf.Name{"ColorSpace", "ExtGState", "Font", "Pattern", "Properties", "Shading", "XObject"}

// minimal returns a resource dictionary with the entries of res that used
// lists, and the default colour spaces of res. It holds no reference to
// the content streams that res holds besides those used: a copy of res
// would make a cycle of a form or a Type 3 font that res lists.
func (c *converter) minimal(res pdf.Object, used resUse) pdf.Dict {
	rd, _ := c.dict(res)
	out := pdf.Dict{}
	for _, category := range resourceCategories {
		src, _ := c.dict(rd.Get(category))
		names := make([]string, 0, len(used[category]))
		for n := range used[category] {
			names = append(names, string(n))
		}
		if category == "ColorSpace" {
			names = append(names, "DefaultGray", "DefaultRGB", "DefaultCMYK")
		}
		sort.Strings(names)
		var sub pdf.Dict
		for _, n := range names {
			if v := src.Get(pdf.Name(n)); v != nil && sub.Get(pdf.Name(n)) == nil {
				sub = append(sub, pdf.KV{Key: pdf.Name(n), Val: v})
			}
		}
		if len(sub) > 0 {
			out = append(out, pdf.KV{Key: category, Val: sub})
		}
	}
	return out
}

// leave gives o the resources of its context it uses when its content,
// read in in, uses named resources and it has none of its own (6.2.2), and
// returns its dictionary, to be stored.
func (c *converter) leave(o pdf.Object, d pdf.Dict, in scanCtx, own bool, used resUse) pdf.Dict {
	if len(used) == 0 || own {
		return d
	}
	ref, isRef := o.(pdf.Ref)
	if in.res == nil || !isRef {
		c.refuse(in.where, "the content stream uses named resources but has no resource dictionary, which PDF/A requires (6.2.2)")
		return d
	}
	m := c.minimal(in.res, used)
	c.inherited[ref] = given{used: used, res: m}
	return set(d, "Resources", m)
}

// form reads a form XObject, a tiling pattern, a soft mask group or an
// appearance stream used in the context x: its transparency group and its
// content.
func (c *converter) form(o pdf.Object, x scanCtx, where string, pattern bool) {
	st, ok := c.resolve(o).(*pdf.Stream)
	if !ok {
		return
	}
	in, own, read := c.enter(o, st.Dict, x, where, pattern)
	if !read {
		return
	}
	if g, ok := c.dict(st.Dict.Get("Group")); ok {
		c.colourSpace(g.Get("CS"), where+" /Group /CS", in.def, 0)
	}
	data, ok := c.decode(st, where)
	if !ok {
		return
	}
	used := c.scan(data, in)
	st.Dict = c.leave(o, st.Dict, in, own, used)
}

// type3 reads the glyph descriptions of a Type 3 font selected in the
// context x.
func (c *converter) type3(o pdf.Object, x scanCtx, where string) {
	d, ok := c.dict(o)
	if !ok || c.name(d, "Subtype") != "Type3" {
		return
	}
	in, own, read := c.enter(o, d, x, where, false)
	if !read {
		return
	}
	var used resUse
	if procs, ok := c.dict(d.Get("CharProcs")); ok {
		for _, p := range procs {
			if st, ok := c.resolve(p.Val).(*pdf.Stream); ok {
				g := in
				g.where = where + " /CharProcs /" + string(p.Key)
				if data, ok := c.decode(st, g.where); ok {
					used.merge(c.scan(data, g))
				}
			}
		}
	}
	c.store(o, c.leave(o, d, in, own, used))
}

// pattern reads a pattern used in the context x.
func (c *converter) pattern(o pdf.Object, x scanCtx, where string) {
	switch v := c.resolve(o).(type) {
	case *pdf.Stream:
		c.form(o, x, where, true)
	case pdf.Dict:
		c.shading(v.Get("Shading"), where+" /Shading", 0)
	}
}

// xobject records the colour space of an image and reads a form XObject.
func (c *converter) xobject(o pdf.Object, x scanCtx, where string) {
	st, ok := c.resolve(o).(*pdf.Stream)
	if !ok {
		return
	}
	switch c.name(st.Dict, "Subtype") {
	case "Image":
		if mask, _ := c.resolve(st.Dict.Get("ImageMask")).(pdf.Bool); !mask {
			c.colourSpace(st.Dict.Get("ColorSpace"), where+" /ColorSpace", x.def, 0)
		}
	case "Form":
		c.form(o, x, where, false)
	}
}

// annotation cleans the actions of an annotation, gives a visible
// annotation the Print flag, keeps its normal appearance only (6.3) and
// reads it. An appearance stream without resources of its own uses the
// default resources of the interactive form, dr, for a widget. Hidden
// annotations, forbidden types and missing appearances are left to the
// PDF/A check, which reports them.
func (c *converter) annotation(o, dr pdf.Object, where string) pdf.Object {
	d, ok := c.dict(o)
	if !ok || !c.visit(o, roleAnnot) {
		return o
	}
	sub := c.name(d, "Subtype")
	if sub == "Widget" {
		del(d, "A")
	} else {
		dr = nil
		if a := d.Get("A"); a != nil {
			d = set(d, "A", c.action(a, 0))
		}
	}
	if sub != "Popup" {
		f, has := pdf.AsInt(c.resolve(d.Get("F")))
		if f&(2|32) == 0 { // neither Hidden nor NoView
			if nf := (f | 4) &^ (1 | 256); !has || nf != f {
				d = set(d, "F", pdf.Int(nf))
			}
		}
	}
	if ap, ok := c.dict(d.Get("AP")); ok {
		for _, kv := range ap {
			if kv.Key != "N" {
				del(ap, kv.Key)
			}
		}
		x := scanCtx{res: dr}
		switch n := c.resolve(ap.Get("N")).(type) {
		case *pdf.Stream:
			c.form(ap.Get("N"), x, where+" /AP /N", false)
		case pdf.Dict:
			for _, s := range n {
				c.form(s.Val, x, where+" /AP /N /"+string(s.Key), false)
			}
		}
	}
	if sub == "FileAttachment" {
		// The file of the annotation is associated with it (6.8).
		fs := d.Get("FS")
		if spec, ok := fs.(pdf.Dict); ok {
			fs = c.alloc(spec)
			d = set(d, "FS", fs)
		}
		if ref, ok := fs.(pdf.Ref); ok && !c.listedIn(d.Get("AF"), ref) {
			if _, ok := c.dict(ref); ok {
				af, _ := c.resolve(d.Get("AF")).(pdf.Array)
				d = set(d, "AF", append(af, ref))
			}
		}
	}
	c.associate(d.Get("AF"))
	return c.store(o, d)
}

// listedIn reports whether the array o lists ref.
func (c *converter) listedIn(o pdf.Object, ref pdf.Ref) bool {
	list, _ := c.resolve(o).(pdf.Array)
	for _, x := range list {
		if x == pdf.Object(ref) {
			return true
		}
	}
	return false
}

// associateFiles lists in the /AF array of the catalog the file
// specifications that no /AF array lists: PDF/A-3 wants every embedded
// file associated with the document or a part of it, and veraPDF every file
// specification (6.8).
func (c *converter) associateFiles() {
	var add []pdf.Object
	for _, num := range c.reachable() {
		d, ok := c.objs[num].(pdf.Dict)
		ref := pdf.Ref{Num: num}
		if !ok || c.afRefs[ref] || (c.name(d, "Type") != "Filespec" && d.Get("EF") == nil) {
			continue
		}
		add = append(add, ref)
		c.afRefs[ref] = true
	}
	if len(add) == 0 {
		return
	}
	cat := c.objs[c.root].(pdf.Dict)
	af, _ := c.resolve(cat.Get("AF")).(pdf.Array)
	af = append(af, add...)
	if _, ok := cat.Get("AF").(pdf.Ref); ok {
		c.store(cat.Get("AF"), af)
	} else {
		cat = set(cat, "AF", af)
	}
	c.objs[c.root] = cat
}

// reachable returns the numbers of the objects reachable from the root and
// the document information dictionary, in the order they are reached.
func (c *converter) reachable() []uint32 {
	seen := map[uint32]bool{}
	var order []uint32
	reach := func(num uint32) {
		if !seen[num] && c.objs[num] != nil {
			seen[num] = true
			order = append(order, num)
		}
	}
	reach(c.root)
	if c.info != 0 {
		reach(c.info)
	}
	for i := 0; i < len(order); i++ {
		refs(c.objs[order[i]], func(r pdf.Ref) { reach(r.Num) })
	}
	return order
}

// contentOps are the operators of ISO 32000-1 (Annex A); ScanContent
// reports an inline image as BI.
var contentOps = map[string]bool{}

func init() {
	for _, op := range strings.Fields(`b B b* B* BDC BI BMC BT BX c cm CS cs d d0 d1 Do DP EMC ET EX
		f F f* G g gs h i j J K k l m M MP n q Q re RG rg ri s S SC sc SCN scn sh T* Tc Td TD Tf
		TJ Tj TL Tm Tr Ts Tw Tz v w W W* y ' "`) {
		contentOps[op] = true
	}
}

// scan reads a content stream in the context x: the syntax of its
// operands, its operators, the fonts that show text, the colour operators
// and the colour spaces, images, forms, patterns, shadings and soft masks
// it uses, the inline images, the rendering intents and the nesting of q
// operators. It returns the named resources the stream uses.
func (c *converter) scan(data []byte, x scanCtx) resUse {
	where := x.where
	bad := false
	pdf.ContentSyntax(data, func(is pdf.SyntaxIssue) {
		if !bad {
			bad = true
			c.refuse(where, "the content stream does not follow the syntax PDF/A requires (6.1.6, 6.1.13): %s", is.Detail)
		}
	})
	rd, _ := c.dict(x.res)
	var used resUse
	resource := func(category, name pdf.Name) pdf.Object {
		used.add(category, name)
		d, _ := c.dict(rd.Get(category))
		return d.Get(name)
	}
	type state struct {
		font pdf.Object
		tr   int64
	}
	var cur state
	var stack []state
	deep := false
	err := pdf.ScanContent(data, func(op string, args []pdf.Object) error {
		name := pdf.Name("")
		if len(args) > 0 {
			name, _ = args[0].(pdf.Name)
		}
		if !contentOps[op] {
			c.refuse(where, "the content stream holds the operator %q, which ISO 32000-1 does not define; PDF/A forbids it, even between BX and EX (6.2.2)", op)
			return nil
		}
		switch op {
		case "q":
			stack = append(stack, cur)
			if len(stack) > maxNesting && !deep {
				deep = true
				c.refuse(where, "q operators are nested %d deep; PDF/A admits %d", len(stack), maxNesting)
			}
		case "Q":
			if len(stack) > 0 {
				cur = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "Tf":
			cur.font = resource("Font", name)
			c.type3(cur.font, x, where+" /Font /"+string(name))
		case "Tr":
			if len(args) > 0 {
				cur.tr, _ = pdf.AsInt(args[0])
			}
		case "Tj", "TJ", "'", "\"":
			if cur.tr != 3 && cur.font != nil {
				c.fontUsed(cur.font, where)
			}
		case "rg", "RG":
			c.device("DeviceRGB", where, x.def)
		case "k", "K":
			c.device("DeviceCMYK", where, x.def)
		case "cs", "CS":
			switch name {
			case "DeviceRGB", "DeviceCMYK":
				c.device(name, where, x.def)
			case "DeviceGray", "Pattern":
			default:
				c.colourSpace(resource("ColorSpace", name), where+" /ColorSpace /"+string(name), x.def, 0)
			}
		case "scn", "SCN":
			if len(args) > 0 {
				if n, ok := args[len(args)-1].(pdf.Name); ok {
					c.pattern(resource("Pattern", n), x, where+" /Pattern /"+string(n))
				}
			}
		case "sh":
			c.shading(resource("Shading", name), where+" /Shading /"+string(name), x.def)
		case "Do":
			c.xobject(resource("XObject", name), x, where+" /XObject /"+string(name))
		case "gs":
			if g, ok := c.dict(resource("ExtGState", name)); ok {
				if mask, ok := c.dict(g.Get("SMask")); ok {
					c.form(mask.Get("G"), x, where+" /ExtGState /"+string(name)+" /SMask /G", false)
				}
			}
		case "BDC", "DP":
			if len(args) == 2 {
				if n, ok := args[1].(pdf.Name); ok {
					resource("Properties", n)
				}
			}
		case "ri":
			if !renderingIntents[name] {
				c.refuse(where, "the ri operator sets the rendering intent %s, which PDF/A does not admit", name)
			}
		case "BI":
			if len(args) == 1 {
				if d, ok := args[0].(pdf.Dict); ok {
					if n := c.inlineImage(d, rd, where, x.def); n != "" {
						used.add("ColorSpace", n)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		c.refuse(where, "the content stream cannot be read: %v", err)
	}
	return used
}

// fontUsed refuses a font that shows text without its font program.
func (c *converter) fontUsed(o pdf.Object, where string) {
	ref, isRef := o.(pdf.Ref)
	if isRef && c.fonts[ref] {
		return
	}
	d, ok := c.dict(o)
	if !ok {
		return
	}
	sub := c.name(d, "Subtype")
	if sub == "Type3" {
		return
	}
	desc := d
	if sub == "Type0" {
		desc = nil
		if list, ok := c.resolve(d.Get("DescendantFonts")).(pdf.Array); ok && len(list) > 0 {
			desc, _ = c.dict(list[0])
		}
	}
	if fd, ok := c.dict(desc.Get("FontDescriptor")); ok {
		for _, key := range []pdf.Name{"FontFile", "FontFile2", "FontFile3"} {
			if _, ok := c.resolve(fd.Get(key)).(*pdf.Stream); ok {
				return
			}
		}
	}
	if isRef {
		c.fonts[ref] = true
	}
	c.refuse(where, "the font %q (%s) shows text but its font program is not embedded, which PDF/A requires; embed the font when producing the PDF", c.name(d, "BaseFont"), sub)
}

// inlineFilters maps the abbreviated filter names of inline images.
var inlineFilters = map[pdf.Name]pdf.Name{
	"AHx": "ASCIIHexDecode", "A85": "ASCII85Decode", "LZW": "LZWDecode", "Fl": "FlateDecode",
	"RL": "RunLengthDecode", "CCF": "CCITTFaxDecode", "DCT": "DCTDecode",
}

// inlineImage checks an inline image (6.1.7.2, 6.2.8) and records its
// colour space, from the resources res when it is named. It returns the
// name of the colour space it takes from res, if any.
func (c *converter) inlineImage(d pdf.Dict, res pdf.Dict, where string, def uint8) pdf.Name {
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
		if long, ok := inlineFilters[f]; ok {
			f = long
		}
		switch f {
		case "LZWDecode":
			c.refuse(where, "an inline image uses the LZWDecode filter, which PDF/A forbids; the conversion does not rewrite the content streams")
		case "ASCIIHexDecode", "ASCII85Decode", "FlateDecode", "RunLengthDecode", "CCITTFaxDecode", "DCTDecode":
		default:
			c.refuse(where, "an inline image uses the filter %s, which PDF/A does not admit", f)
		}
	}
	if b, _ := get("Interpolate", "I").(pdf.Bool); b {
		c.refuse(where, "an inline image has Interpolate true, which PDF/A forbids; the conversion does not rewrite the content streams")
	}
	if ri := d.Get("Intent"); ri != nil {
		if n, _ := ri.(pdf.Name); !renderingIntents[n] {
			c.refuse(where, "an inline image has the rendering intent %v, which PDF/A does not admit", ri)
		}
	}
	if mask, _ := get("ImageMask", "IM").(pdf.Bool); mask {
		return ""
	}
	switch cs := get("ColorSpace", "CS").(type) {
	case pdf.Name:
		switch cs {
		case "G", "RGB", "CMYK", "DeviceGray", "DeviceRGB", "DeviceCMYK":
			c.device(cs, where, def)
		default:
			spaces, _ := c.dict(res.Get("ColorSpace"))
			c.colourSpace(spaces.Get(cs), where+" /ColorSpace /"+string(cs), def, 0)
			return cs
		}
	case pdf.Array:
		c.colourSpace(cs, where+" (inline image)", def, 0)
		// The base of an indexed colour space may be named.
		if len(cs) > 1 && (cs[0] == pdf.Object(pdf.Name("I")) || cs[0] == pdf.Object(pdf.Name("Indexed"))) {
			base, _ := cs[1].(pdf.Name)
			switch base {
			case "", "G", "RGB", "CMYK", "DeviceGray", "DeviceRGB", "DeviceCMYK":
			default:
				spaces, _ := c.dict(res.Get("ColorSpace"))
				c.colourSpace(spaces.Get(base), where+" /ColorSpace /"+string(base), def, 0)
				return base
			}
		}
	}
	return ""
}

// outputIntent gives the document a PDF/A output intent and returns the
// colour space of its profile ("RGB ", "CMYK", "GRAY"), or "?" when it
// cannot be read. The profile of an existing output intent is kept, as
// PDF/A requires all of them to share one (6.2.3); otherwise the sRGB
// profile of the package is embedded.
func (c *converter) outputIntent() string {
	cat := c.objs[c.root].(pdf.Dict)
	intents, _ := c.resolve(cat.Get("OutputIntents")).(pdf.Array)
	var profile pdf.Object
	var source pdf.Dict // intent whose profile is reused
	pdfa := -1
	for i, o := range intents {
		d, ok := c.dict(o)
		if !ok {
			continue
		}
		if p := d.Get("DestOutputProfile"); p != nil && profile == nil {
			profile, source = p, d
		}
		if c.name(d, "S") == "GTS_PDFA1" && pdfa < 0 {
			pdfa = i
		}
	}
	space := "RGB "
	if profile == nil {
		profile = c.alloc(&pdf.Stream{
			Dict: pdf.Dict{{Key: "N", Val: pdf.Int(3)}, {Key: "Filter", Val: pdf.Name("FlateDecode")}},
			Raw:  pdf.Deflate(icc.SRGB()),
		})
	} else if st, ok := c.resolve(profile).(*pdf.Stream); ok {
		if data, err := c.r.Decode(st); err == nil && len(data) >= 20 {
			space = string(data[16:20])
		} else {
			space = "?"
		}
	} else {
		space = "?"
	}
	if pdfa >= 0 {
		d, _ := c.dict(intents[pdfa])
		if d.Get("DestOutputProfile") == nil {
			intents[pdfa] = c.store(intents[pdfa], set(d, "DestOutputProfile", profile))
		}
	} else {
		oi := pdf.Dict{{Key: "Type", Val: pdf.Name("OutputIntent")}, {Key: "S", Val: pdf.Name("GTS_PDFA1")}}
		if source == nil {
			oi = append(oi,
				pdf.KV{Key: "OutputConditionIdentifier", Val: pdf.String(icc.Description)},
				pdf.KV{Key: "Info", Val: pdf.String(icc.Description)},
				pdf.KV{Key: "RegistryName", Val: pdf.String("http://www.color.org")})
		} else {
			for _, key := range []pdf.Name{"OutputCondition", "OutputConditionIdentifier", "RegistryName", "Info"} {
				if v := source.Get(key); v != nil {
					oi = append(oi, pdf.KV{Key: key, Val: v})
				}
			}
			if oi.Get("OutputConditionIdentifier") == nil {
				oi = append(oi, pdf.KV{Key: "OutputConditionIdentifier", Val: pdf.String("Custom")})
			}
		}
		oi = append(oi, pdf.KV{Key: "DestOutputProfile", Val: profile})
		intents = append(intents, c.alloc(oi))
	}
	if ref, ok := cat.Get("OutputIntents").(pdf.Ref); ok {
		c.objs[ref.Num] = intents
	} else {
		cat = set(cat, "OutputIntents", intents)
	}
	c.objs[c.root] = cat
	return space
}

// metadata declares PDF/A-3 in the XMP metadata of the catalog, merged with
// the existing packet unless PDF/A would reject what it holds, and keeps
// the ModDate of the document information dictionary equal to
// xmp:ModifyDate.
func (c *converter) metadata() {
	cat := c.objs[c.root].(pdf.Dict)
	var existing []byte
	if st, ok := c.resolve(cat.Get("Metadata")).(*pdf.Stream); ok {
		if data, err := c.r.Decode(st); err == nil {
			existing = data
		}
	}
	var di xmp.DocInfo
	info, hasInfo := c.objs[c.info].(pdf.Dict)
	if c.info == 0 {
		hasInfo = false
	}
	if hasInfo {
		di = xmp.DocInfo{Title: c.text(info, "Title"), Author: c.text(info, "Author"), Subject: c.text(info, "Subject"),
			Keywords: c.text(info, "Keywords"), Creator: c.text(info, "Creator"), Producer: c.text(info, "Producer")}
		if t, ok := pdf.ParseDate(c.text(info, "CreationDate")); ok {
			di.CreationDate = t
		}
		if info.Get("ModDate") != nil {
			info = set(info, "ModDate", pdf.String(pdf.FormatDate(c.date)))
			c.objs[c.info] = info
		}
	}
	p := xmp.Params{PDFAOnly: true, PDFAPart: "3", Date: c.date, DocInfo: di}
	packet := xmp.Merge(existing, p)
	if !packetOK(packet) {
		packet = xmp.Merge(nil, p)
	}
	st := &pdf.Stream{Dict: pdf.Dict{{Key: "Type", Val: pdf.Name("Metadata")}, {Key: "Subtype", Val: pdf.Name("XML")}}, Raw: packet}
	if ref, ok := cat.Get("Metadata").(pdf.Ref); ok {
		c.objs[ref.Num] = st
	} else {
		cat = set(cat, "Metadata", c.alloc(st))
	}
	c.objs[c.root] = cat
}

// packetOK reports whether an XMP packet meets the PDF/A requirements on
// the document metadata.
func packetOK(data []byte) bool {
	info, err := xmp.Parse(data)
	if err != nil {
		return false
	}
	ok := true
	xmp.CheckPDFA(data, info.Declared(), func(xmp.Issue) { ok = false })
	return ok
}

// write renumbers the objects reachable from the root and the document
// information dictionary, in the order they are reached, and writes the
// file.
func (c *converter) write(orig []byte) []byte {
	order := c.reachable()
	newNum := make(map[uint32]uint32, len(order))
	for i, num := range order {
		newNum[num] = uint32(i + 1)
	}
	var remap func(o pdf.Object) pdf.Object
	remap = func(o pdf.Object) pdf.Object {
		switch v := o.(type) {
		case pdf.Ref:
			if n, ok := newNum[v.Num]; ok {
				return pdf.Ref{Num: n}
			}
			return nil
		case pdf.Dict:
			d := make(pdf.Dict, 0, len(v))
			for _, kv := range v {
				if kv.Val == nil {
					continue
				}
				if x := remap(kv.Val); x != nil {
					d = append(d, pdf.KV{Key: kv.Key, Val: x})
				}
			}
			return d
		case pdf.Array:
			a := make(pdf.Array, len(v))
			for i, x := range v {
				a[i] = remap(x)
			}
			return a
		case *pdf.Stream:
			return &pdf.Stream{Dict: remap(v.Dict).(pdf.Dict), Raw: v.Raw}
		}
		return o
	}
	objs := make([]pdf.Object, len(order))
	for i, num := range order {
		objs[i] = remap(c.objs[num])
	}

	// File identifier: the permanent part of the original one, a new
	// second part.
	var first pdf.String
	if id, ok := c.r.Resolve(c.r.Trailer().Get("ID")).(pdf.Array); ok && len(id) > 0 {
		first, _ = c.r.Resolve(id[0]).(pdf.String)
	}
	if len(first) == 0 {
		sum := md5.Sum(orig)
		first = pdf.String(sum[:])
	}
	h := md5.New()
	h.Write(first)
	h.Write([]byte(pdf.FormatDate(c.date)))
	fmt.Fprintf(h, "%d", len(orig))
	trailer := pdf.Dict{{Key: "Root", Val: pdf.Ref{Num: newNum[c.root]}}}
	if n, ok := newNum[c.info]; ok && c.info != 0 {
		trailer = append(trailer, pdf.KV{Key: "Info", Val: pdf.Ref{Num: n}})
	}
	trailer = append(trailer, pdf.KV{Key: "ID", Val: pdf.Array{first, pdf.String(h.Sum(nil))}})
	return pdf.WriteFile("1.7", objs, trailer)
}
