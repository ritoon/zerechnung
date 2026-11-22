package zerechnung

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/font"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/xmp"
)

// This file holds the PDF/A requirements of ISO 19005-2 and 19005-3 that
// bear on the objects of the file rather than on the content streams
// (PDF-41 to PDF-69), with the numbering of the veraPDF validation
// profiles for PDF/A-2b and PDF/A-3b. The content streams are read by
// check_pdfa_content.go, their glyphs by check_pdfa_glyphs.go. They are not
// applied to a document that declares PDF/A-4, whose requirements differ
// (ISO 19005-4).

// Default colour spaces set in a resource dictionary (ISO 32000-1, 8.6.5.6).
const (
	defaultRGB uint8 = 1 << iota
	defaultCMYK
	defaultGray
)

// standardFilters are the stream filters of ISO 32000-1, Table 6.
var standardFilters = map[pdf.Name]bool{
	"ASCIIHexDecode": true, "ASCII85Decode": true, "FlateDecode": true, "RunLengthDecode": true,
	"CCITTFaxDecode": true, "JBIG2Decode": true, "DCTDecode": true, "JPXDecode": true,
}

// filterIssue returns why PDF/A forbids filter f with decode parameters
// parms (6.1.7.2), or "".
func (s *pdfaScan) filterIssue(f pdf.Name, parms pdf.Object) string {
	switch {
	case f == "LZWDecode" || f == "LZW":
		return "the stream uses the LZWDecode filter, which PDF/A forbids"
	case f == "Crypt":
		if d, ok := s.c.r.Dict(parms); ok && s.name(d, "Name") == "Identity" {
			return ""
		}
		return "the stream uses the Crypt filter with a crypt filter other than Identity, which PDF/A forbids"
	case !standardFilters[f]:
		return fmt.Sprintf("the stream uses the filter %s, which is not a standard filter of ISO 32000-1 (Table 6)", f)
	}
	return ""
}

// filters returns the filters of a stream dictionary and their decode
// parameters.
func (s *pdfaScan) filters(d pdf.Dict) (names []pdf.Name, parms []pdf.Object) {
	r := s.c.r
	switch f := r.Resolve(d.Get("Filter")).(type) {
	case pdf.Name:
		names = []pdf.Name{f}
		parms = []pdf.Object{d.Get("DecodeParms")}
	case pdf.Array:
		dp, _ := r.Resolve(d.Get("DecodeParms")).(pdf.Array)
		for i, x := range f {
			n, _ := r.Resolve(x).(pdf.Name)
			names = append(names, n)
			var p pdf.Object
			if i < len(dp) {
				p = dp[i]
			}
			parms = append(parms, p)
		}
	}
	return names, parms
}

// fileStructure checks the file structure of 6.1 on the bytes of the file
// and on every object in use: header (PDF-41), end of file (PDF-42),
// cross-reference tables (PDF-43), hexadecimal strings (PDF-44), streams
// (PDF-45, PDF-38), indirect objects (PDF-47) and implementation limits
// (PDF-49).
func (s *pdfaScan) fileStructure() {
	r := s.c.r
	b := r.Data()

	// 6.1.2: "%PDF-1.n" at byte zero, a single EOL marker, then a comment
	// of at least four bytes above 127.
	ok := len(b) >= 9 && string(b[:7]) == "%PDF-1." && b[7] >= '0' && b[7] <= '7'
	i := 8
	if ok {
		switch {
		case b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n':
			i += 2
		case b[i] == '\n' || b[i] == '\r':
			i++
		default:
			ok = false
		}
	}
	if !ok {
		s.report("PDF-41", "header", "the file does not start with %%PDF-1.n (n from 0 to 7) followed by a single end-of-line marker")
	} else if i+5 > len(b) || b[i] != '%' || b[i+1] < 128 || b[i+2] < 128 || b[i+3] < 128 || b[i+4] < 128 {
		s.report("PDF-41", "header", "the header is not followed by a comment of at least four bytes above 127")
	}

	// 6.1.3: nothing after the last %%EOF but a single EOL marker.
	if k := bytes.LastIndex(b, []byte("%%EOF")); k < 0 {
		s.report("PDF-42", "end of file", "the file has no end-of-file marker (%%%%EOF)")
	} else if rest := b[k+5:]; len(rest) > 0 && string(rest) != "\n" && string(rest) != "\r" && string(rest) != "\r\n" {
		s.report("PDF-42", "end of file", "%d bytes follow the last end-of-file marker", len(rest))
	}

	// 6.1.4: the xref keyword and the subsection header are separated by a
	// single EOL marker.
	for _, off := range r.XrefTables() {
		j := int(off)
		for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\r' || b[j] == '\n') {
			j++
		}
		if !bytes.HasPrefix(b[j:], []byte("xref")) {
			continue
		}
		j += 4
		switch {
		case j+1 < len(b) && b[j] == '\r' && b[j+1] == '\n':
			j += 2
		case j < len(b) && (b[j] == '\n' || b[j] == '\r'):
			j++
		default:
			j = -1
		}
		if j < 0 || j >= len(b) || b[j] < '0' || b[j] > '9' {
			s.report("PDF-43", fmt.Sprintf("cross-reference table at offset %d", off), "the xref keyword and the subsection header are not separated by a single end-of-line marker")
		}
	}

	// The objects in use: syntax, stream dictionaries and filters.
	locs, total, more := r.InUse(maxScanObjects)
	if total > 8388607 {
		s.report("PDF-49", "cross-reference", "the file has %d indirect objects; PDF/A admits 8388607", total)
	}
	_ = more
	for _, loc := range locs {
		r.Syntax(loc, func(is pdf.SyntaxIssue) {
			where := fmt.Sprintf("object %d", is.Num)
			switch is.Kind {
			case pdf.SyntaxHex:
				s.report("PDF-44", where, "%s", is.Detail)
			case pdf.SyntaxObject:
				s.report("PDF-47", where, "%s", is.Detail)
			case pdf.SyntaxStream:
				s.report("PDF-45", where, "%s", is.Detail)
			case pdf.SyntaxLimit:
				s.report("PDF-49", where, "%s", is.Detail)
			}
		})
		st, ok := r.Object(loc.Num).(*pdf.Stream)
		if !ok {
			continue
		}
		where := fmt.Sprintf("object %d", loc.Num)
		for _, key := range []pdf.Name{"F", "FFilter", "FDecodeParms"} {
			if st.Dict.Has(key) {
				s.report("PDF-45", where+" /"+string(key), "the stream dictionary has a /%s entry (external stream data), which PDF/A forbids", key)
			}
		}
		// Streams reached by the scan of the pages had their filters
		// checked there, with a more telling path.
		ref := pdf.Ref{Num: loc.Num, Gen: loc.Gen}
		if s.seen[ref]&roleStream != 0 {
			continue
		}
		if s.name(st.Dict, "Type") == "Metadata" && ref != s.metaRef() {
			if data, err := r.Decode(st); err == nil {
				s.packet(xmp.ParsePacket(data), where)
			}
		}
		names, parms := s.filters(st.Dict)
		for k, f := range names {
			if msg := s.filterIssue(f, parms[k]); msg != "" {
				s.report("PDF-38", where+" /Filter", "%s", msg)
				break
			}
		}
	}
}

// metaRef returns the reference of the metadata stream of the catalog.
func (s *pdfaScan) metaRef() pdf.Ref {
	ref, _ := s.c.catalog.Get("Metadata").(pdf.Ref)
	return ref
}

// metadata checks the metadata stream of the catalog (6.6.2.1) and its XMP
// packet (PDF-65 to PDF-67). A packet that cannot be read is reported by
// PDF-03.
func (s *pdfaScan) metadata() {
	r := s.c.r
	st, ok := r.Resolve(s.c.catalog.Get("Metadata")).(*pdf.Stream)
	if !ok {
		return
	}
	if t := s.name(st.Dict, "Type"); t != "Metadata" {
		s.report("PDF-65", "catalog /Metadata /Type", "the metadata stream has /Type %q, not /Metadata", t)
	}
	if t := s.name(st.Dict, "Subtype"); t != "XML" {
		s.report("PDF-65", "catalog /Metadata /Subtype", "the metadata stream has /Subtype %q, not /XML", t)
	}
	// The stream as document read it: the packet is nil when the stream
	// cannot be decoded, and what it declares nil when the packet cannot
	// be read.
	p, info := s.k.xmp, s.k.xmpInfo
	if p == nil || info == nil {
		return
	}
	s.xmp = info.Declared()
	s.packet(p, "catalog /Metadata")
}

// packet checks an XMP packet against the PDF/A requirements.
func (s *pdfaScan) packet(p *xmp.Packet, where string) {
	p.CheckPDFA(s.xmp, func(is xmp.Issue) {
		switch is.Kind {
		case xmp.IssuePacket:
			s.report("PDF-65", where, "%s", is.Message)
		case xmp.IssueProperty:
			s.report("PDF-66", where, "%s", is.Message)
		case xmp.IssueExtension:
			s.report("PDF-67", where, "%s", is.Message)
		}
	})
}

// documentLevel checks the requirements on the catalog and the trailer:
// output intents (PDF-50), permissions (PDF-48), interactive forms
// (PDF-64), optional content (PDF-68), alternate presentations and
// requirements (PDF-69).
func (s *pdfaScan) documentLevel() {
	r := s.c.r
	cat := s.c.catalog

	// 6.2.3 Output intents.
	if intents, ok := r.Resolve(cat.Get("OutputIntents")).(pdf.Array); ok {
		var profile pdf.Object
		for i, o := range intents {
			d, ok := r.Dict(o)
			if !ok {
				continue
			}
			where := fmt.Sprintf("catalog /OutputIntents [%d]", i)
			if s.name(d, "S") == "GTS_PDFX" && d.Has("DestOutputProfileRef") {
				s.report("PDF-50", where+" /DestOutputProfileRef", "the PDF/X output intent has a /DestOutputProfileRef entry, which PDF/A forbids")
			}
			p := d.Get("DestOutputProfile")
			if p == nil {
				continue
			}
			if profile == nil {
				profile = p
			} else if pr, ok := profile.(pdf.Ref); !ok || p != pdf.Object(pr) {
				s.report("PDF-50", where+" /DestOutputProfile", "the output intents have different /DestOutputProfile objects; PDF/A requires the same indirect object")
			}
			if h, ok := s.icc(p); ok {
				switch {
				case h.invalidData:
					s.report("PDF-50", where+" /DestOutputProfile", "the ICC profile of the output intent cannot be read")
				case h.class != "prtr" && h.class != "mntr":
					s.report("PDF-50", where+" /DestOutputProfile", "the ICC profile of the output intent has device class %q, not an output (prtr) or monitor (mntr) profile", h.class)
				case h.space != "RGB " && h.space != "CMYK" && h.space != "GRAY":
					s.report("PDF-50", where+" /DestOutputProfile", "the ICC profile of the output intent has colour space %q, not GRAY, RGB or CMYK", strings.TrimSpace(h.space))
				case h.major >= 5:
					s.report("PDF-50", where+" /DestOutputProfile", "the ICC profile of the output intent has version %d, above the ICC.1:2010 (version 4) profiles PDF/A admits", h.major)
				}
			}
		}
	}

	// 6.1.12 Permissions.
	if perms, ok := r.Dict(cat.Get("Perms")); ok {
		for _, kv := range perms {
			if kv.Key != "UR3" && kv.Key != "DocMDP" {
				s.report("PDF-48", "catalog /Perms /"+string(kv.Key), "the permissions dictionary has a /%s entry; PDF/A admits only UR3 and DocMDP", kv.Key)
			}
		}
		if sig, ok := r.Dict(perms.Get("DocMDP")); ok {
			if refs, ok := r.Resolve(sig.Get("Reference")).(pdf.Array); ok {
				for i, x := range refs {
					if d, ok := r.Dict(x); ok {
						for _, key := range []pdf.Name{"DigestLocation", "DigestMethod", "DigestValue"} {
							if d.Has(key) {
								s.report("PDF-48", fmt.Sprintf("catalog /Perms /DocMDP /Reference [%d] /%s", i, key), "the signature reference dictionary has a /%s entry, which PDF/A forbids with DocMDP", key)
							}
						}
					}
				}
			}
		}
	}

	// 6.4 Interactive forms.
	if form, ok := r.Dict(cat.Get("AcroForm")); ok {
		if b, _ := r.Resolve(form.Get("NeedAppearances")).(pdf.Bool); b {
			s.report("PDF-64", "catalog /AcroForm /NeedAppearances", "the interactive form has NeedAppearances true, which PDF/A forbids")
		}
		if form.Has("XFA") {
			s.report("PDF-64", "catalog /AcroForm /XFA", "the interactive form has an XFA form (/XFA), which PDF/A forbids")
		}
	}
	if b, _ := r.Resolve(cat.Get("NeedsRendering")).(pdf.Bool); b {
		s.report("PDF-64", "catalog /NeedsRendering", "the catalog has NeedsRendering true, which PDF/A forbids")
	}

	// 6.9 Optional content.
	if oc, ok := r.Dict(cat.Get("OCProperties")); ok {
		s.optionalContent(oc)
	}

	// 6.10 and 6.11.
	if names, ok := r.Dict(cat.Get("Names")); ok && s.present(names, "AlternatePresentations") {
		s.report("PDF-69", "catalog /Names /AlternatePresentations", "the name dictionary has alternate presentations (/AlternatePresentations), which PDF/A forbids")
	}
	if cat.Has("Requirements") {
		s.report("PDF-69", "catalog /Requirements", "the catalog has a /Requirements entry, which PDF/A forbids")
	}
}

// optionalContent checks the optional content configurations (6.9).
func (s *pdfaScan) optionalContent(oc pdf.Dict) {
	r := s.c.r
	ocgs := map[pdf.Ref]bool{}
	if list, ok := r.Resolve(oc.Get("OCGs")).(pdf.Array); ok {
		for _, x := range list {
			if ref, ok := x.(pdf.Ref); ok {
				ocgs[ref] = true
			}
		}
	}
	type config struct {
		d     pdf.Dict
		where string
	}
	var configs []config
	if d, ok := r.Dict(oc.Get("D")); ok {
		configs = append(configs, config{d, "catalog /OCProperties /D"})
	}
	if list, ok := r.Resolve(oc.Get("Configs")).(pdf.Array); ok {
		for i, x := range list {
			if d, ok := r.Dict(x); ok {
				configs = append(configs, config{d, fmt.Sprintf("catalog /OCProperties /Configs [%d]", i)})
			}
		}
	}
	names := map[string]bool{}
	for _, cf := range configs {
		name := s.text(cf.d, "Name")
		switch {
		case name == "":
			s.report("PDF-68", cf.where+" /Name", "the optional content configuration has no /Name")
		case names[name]:
			s.report("PDF-68", cf.where+" /Name", "the name %q of the optional content configuration is not unique", name)
		}
		names[name] = true
		if cf.d.Has("AS") {
			s.report("PDF-68", cf.where+" /AS", "the optional content configuration has an /AS entry, which PDF/A forbids")
		}
		order, ok := r.Resolve(cf.d.Get("Order")).(pdf.Array)
		if !ok {
			continue
		}
		listed := map[pdf.Ref]bool{}
		var walk func(a pdf.Array, depth int)
		walk = func(a pdf.Array, depth int) {
			for _, x := range a {
				if ref, ok := x.(pdf.Ref); ok {
					if sub, ok := r.Resolve(ref).(pdf.Array); ok && depth < maxScanDepth {
						walk(sub, depth+1)
					} else {
						listed[ref] = true
					}
				} else if sub, ok := x.(pdf.Array); ok && depth < maxScanDepth {
					walk(sub, depth+1)
				}
			}
		}
		walk(order, 0)
		missing := 0
		for ref := range ocgs {
			if !listed[ref] {
				missing++
			}
		}
		if missing > 0 {
			s.report("PDF-68", cf.where+" /Order", "the /Order array of the optional content configuration does not list %d of the optional content groups", missing)
		}
	}
}

// iccHeader is the header of an ICC profile.
type iccHeader struct {
	major       int
	class       string // device class: prtr, mntr, scnr, spac...
	space       string // colour space: "RGB ", "CMYK", "GRAY", "Lab "...
	invalidData bool
}

// icc reads the header of the ICC profile stream o.
func (s *pdfaScan) icc(o pdf.Object) (iccHeader, bool) {
	ref, isRef := o.(pdf.Ref)
	if isRef {
		if h, ok := s.iccs[ref]; ok {
			return h, true
		}
	}
	st, ok := s.c.r.Resolve(o).(*pdf.Stream)
	if !ok {
		return iccHeader{}, false
	}
	data, err := s.c.r.Decode(st)
	var h iccHeader
	if err != nil || len(data) < 128 {
		h.invalidData = true
	} else {
		h = iccHeader{major: int(data[8]), class: string(data[12:16]), space: string(data[16:20])}
	}
	if isRef {
		if s.iccs == nil {
			s.iccs = map[pdf.Ref]iccHeader{}
		}
		s.iccs[ref] = h
	}
	return h, true
}

// outputSpace returns the colour space of the PDF/A output intent profile
// ("RGB ", "CMYK", "GRAY"), or "" when there is none.
func (s *pdfaScan) outputSpace() string {
	r := s.c.r
	intents, _ := r.Resolve(s.c.catalog.Get("OutputIntents")).(pdf.Array)
	for _, o := range intents {
		d, ok := r.Dict(o)
		if !ok || s.name(d, "S") != "GTS_PDFA1" {
			continue
		}
		if h, ok := s.icc(d.Get("DestOutputProfile")); ok && !h.invalidData {
			return h.space
		}
		return "?"
	}
	return ""
}

// defaults returns the default colour spaces set by a resource dictionary.
func (s *pdfaScan) defaults(res pdf.Dict) uint8 {
	cs, ok := s.c.r.Dict(res.Get("ColorSpace"))
	if !ok {
		return 0
	}
	var def uint8
	if cs.Has("DefaultRGB") {
		def |= defaultRGB
	}
	if cs.Has("DefaultCMYK") {
		def |= defaultCMYK
	}
	if cs.Has("DefaultGray") {
		def |= defaultGray
	}
	return def
}

// colourSpace checks a colour space and the colour spaces it is built on:
// device colour spaces against the output intent and the default colour
// spaces (PDF-52), ICC profiles (PDF-51), Separation and DeviceN colour
// spaces (PDF-53, PDF-49, PDF-46).
func (s *pdfaScan) colourSpace(o pdf.Object, where string, def uint8, depth int) {
	if o == nil || depth > maxScanDepth {
		return
	}
	r := s.c.r
	switch v := r.Resolve(o).(type) {
	case pdf.Name:
		s.deviceSpace(v, where, def)
	case pdf.Array:
		if len(v) == 0 {
			return
		}
		family, _ := r.Resolve(v[0]).(pdf.Name)
		switch family {
		case "DeviceRGB", "DeviceCMYK", "DeviceGray", "RGB", "CMYK", "G":
			s.deviceSpace(family, where, def)
		case "ICCBased":
			if len(v) > 1 {
				if h, ok := s.icc(v[1]); ok {
					switch {
					case h.invalidData:
						s.report("PDF-51", where, "the ICC profile of the ICCBased colour space cannot be read")
					case h.class != "prtr" && h.class != "mntr" && h.class != "scnr" && h.class != "spac":
						s.report("PDF-51", where, "the ICC profile of the ICCBased colour space has device class %q, which PDF/A does not admit", h.class)
					case h.space != "RGB " && h.space != "CMYK" && h.space != "GRAY" && h.space != "Lab ":
						s.report("PDF-51", where, "the ICC profile of the ICCBased colour space has colour space %q, not GRAY, RGB, CMYK or Lab", strings.TrimSpace(h.space))
					case h.major >= 5:
						s.report("PDF-51", where, "the ICC profile of the ICCBased colour space has version %d, above the version 4 profiles PDF/A admits", h.major)
					}
				}
			}
		case "Indexed", "I":
			if len(v) > 1 {
				s.colourSpace(v[1], where+" (base)", def, depth+1)
			}
		case "Pattern":
			if len(v) > 1 {
				s.colourSpace(v[1], where+" (underlying)", def, depth+1)
			}
		case "Separation":
			if len(v) >= 4 {
				name, _ := r.Resolve(v[1]).(pdf.Name)
				s.unicodeName(name, where, "colourant")
				s.separation(name, v[2], v[3], where)
				s.colourSpace(v[2], where+" (alternate)", def, depth+1)
			}
		case "DeviceN":
			if len(v) >= 4 {
				s.deviceN(v, where, def, depth)
			}
		}
	}
}

// deviceSpace checks the use of a device colour space (6.2.4.3).
func (s *pdfaScan) deviceSpace(n pdf.Name, where string, def uint8) {
	out := s.outCS
	switch n {
	case "DeviceRGB", "RGB":
		if def&defaultRGB == 0 && out != "RGB " && out != "?" {
			s.report("PDF-52", where, "DeviceRGB is used without a DefaultRGB colour space nor a PDF/A output intent with an RGB profile%s", s.outNote())
		}
	case "DeviceCMYK", "CMYK":
		if def&defaultCMYK == 0 && out != "CMYK" && out != "?" {
			s.report("PDF-52", where, "DeviceCMYK is used without a DefaultCMYK colour space nor a PDF/A output intent with a CMYK profile%s", s.outNote())
		}
	case "DeviceGray", "G":
		if def&defaultGray == 0 && out == "" {
			s.report("PDF-52", where, "DeviceGray is used without a DefaultGray colour space nor a PDF/A output intent")
		}
	}
}

func (s *pdfaScan) outNote() string {
	if s.outCS == "" {
		return " (the document has no PDF/A output intent)"
	}
	return fmt.Sprintf(" (the output intent profile is %s)", strings.TrimSpace(s.outCS))
}

// processColourants are the colourant names that are not spot colours.
var processColourants = map[pdf.Name]bool{"Cyan": true, "Magenta": true, "Yellow": true, "Black": true, "None": true}

// deviceN checks a DeviceN colour space: the number of colourants
// (6.1.13), their names (6.1.8), the Colorants dictionary (6.2.4.4) and
// the Separation arrays it lists.
func (s *pdfaScan) deviceN(v pdf.Array, where string, def uint8, depth int) {
	r := s.c.r
	names, _ := r.Resolve(v[1]).(pdf.Array)
	if len(names) > 32 {
		s.report("PDF-49", where, "the DeviceN colour space has %d colourants; PDF/A admits 32", len(names))
	}
	var colorants pdf.Dict
	if len(v) > 4 {
		if attrs, ok := r.Dict(v[4]); ok {
			colorants, _ = r.Dict(attrs.Get("Colorants"))
		}
	}
	for _, x := range names {
		n, _ := r.Resolve(x).(pdf.Name)
		s.unicodeName(n, where, "colourant")
		if !processColourants[n] && colorants.Get(n) == nil {
			s.report("PDF-53", where, "the spot colour %q of the DeviceN colour space has no entry in the Colorants dictionary", n)
		}
	}
	for _, kv := range colorants {
		s.colourSpace(kv.Val, where+" /Colorants /"+string(kv.Key), def, depth+1)
	}
	s.colourSpace(v[2], where+" (alternate)", def, depth+1)
}

// separation records a Separation colour space and checks that the ones of
// the same name have the same alternate space and tint transform (6.2.4.4).
func (s *pdfaScan) separation(name pdf.Name, alt, tint pdf.Object, where string) {
	if name == "" || name == "All" || name == "None" {
		return
	}
	if s.seps == nil {
		s.seps = map[pdf.Name][2]pdf.Object{}
	}
	prev, ok := s.seps[name]
	if !ok {
		s.seps[name] = [2]pdf.Object{alt, tint}
		return
	}
	if !s.sameObject(prev[0], alt, 0) || !s.sameObject(prev[1], tint, 0) {
		s.report("PDF-53", where, "the Separation colour spaces named %q have different alternate spaces or tint transforms", name)
	}
}

// sameObject compares two objects as PDF/A compares Separation arrays:
// whether an object is direct or indirect, and how a stream is compressed,
// are ignored.
func (s *pdfaScan) sameObject(a, b pdf.Object, depth int) bool {
	if depth > 16 {
		return true
	}
	r := s.c.r
	if ra, ok := a.(pdf.Ref); ok {
		if rb, ok := b.(pdf.Ref); ok && ra == rb {
			return true
		}
	}
	a, b = r.Resolve(a), r.Resolve(b)
	switch x := a.(type) {
	case pdf.Array:
		y, ok := b.(pdf.Array)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !s.sameObject(x[i], y[i], depth+1) {
				return false
			}
		}
		return true
	case pdf.Dict:
		y, ok := b.(pdf.Dict)
		return ok && s.sameDict(x, y, depth)
	case *pdf.Stream:
		y, ok := b.(*pdf.Stream)
		if !ok || !s.sameDict(x.Dict, y.Dict, depth) {
			return false
		}
		dx, err1 := r.Decode(x)
		dy, err2 := r.Decode(y)
		return err1 == nil && err2 == nil && bytes.Equal(dx, dy)
	case pdf.String:
		y, ok := b.(pdf.String)
		return ok && bytes.Equal(x, y)
	case pdf.Int:
		switch y := b.(type) {
		case pdf.Int:
			return x == y
		case pdf.Real:
			return float64(x) == float64(y)
		}
		return false
	case pdf.Real:
		switch y := b.(type) {
		case pdf.Int:
			return float64(x) == float64(y)
		case pdf.Real:
			return math.Abs(float64(x-y)) < 1e-9
		}
		return false
	}
	return a == b
}

// streamKeys are the stream dictionary entries that describe the encoding.
var streamKeys = map[pdf.Name]bool{"Length": true, "Filter": true, "DecodeParms": true, "DL": true}

func (s *pdfaScan) sameDict(x, y pdf.Dict, depth int) bool {
	n := 0
	for _, kv := range x {
		if streamKeys[kv.Key] {
			continue
		}
		n++
		if !s.sameObject(kv.Val, y.Get(kv.Key), depth+1) {
			return false
		}
	}
	m := 0
	for _, kv := range y {
		if !streamKeys[kv.Key] {
			m++
		}
	}
	return n == m
}

// unicodeName checks that a font, colourant or structure type name is
// valid UTF-8 (6.1.8).
func (s *pdfaScan) unicodeName(n pdf.Name, where, what string) {
	if n != "" && !utf8.ValidString(string(n)) {
		s.report("PDF-46", where, "the %s name %q is not a valid UTF-8 sequence", what, n)
	}
}

// renderingIntents are the rendering intents of ISO 32000-1, Table 70.
var renderingIntents = map[pdf.Name]bool{"RelativeColorimetric": true, "AbsoluteColorimetric": true, "Perceptual": true, "Saturation": true}

// blendModes are the blend modes of ISO 32000-1, 11.3.5.
var blendModes = map[pdf.Name]bool{
	"Normal": true, "Compatible": true, "Multiply": true, "Screen": true, "Overlay": true, "Darken": true,
	"Lighten": true, "ColorDodge": true, "ColorBurn": true, "HardLight": true, "SoftLight": true,
	"Difference": true, "Exclusion": true, "Hue": true, "Saturation": true, "Color": true, "Luminosity": true,
}

// extGState checks a graphics state parameter dictionary: transfer
// functions and halftones (PDF-54), rendering intent (PDF-55) and blend
// mode (PDF-58).
func (s *pdfaScan) extGState(g pdf.Dict, where string) {
	r := s.c.r
	if g.Has("TR") {
		s.report("PDF-54", where+" /TR", "the graphics state has a transfer function (/TR), which PDF/A forbids")
	}
	if tr2 := r.Resolve(g.Get("TR2")); tr2 != nil {
		if n, _ := tr2.(pdf.Name); n != "Default" {
			s.report("PDF-54", where+" /TR2", "the graphics state has a /TR2 entry other than Default, which PDF/A forbids")
		}
	}
	if g.Has("HTP") {
		s.report("PDF-54", where+" /HTP", "the graphics state has an /HTP entry, which PDF/A forbids")
	}
	if ht := g.Get("HT"); ht != nil {
		s.halftone(ht, where+" /HT", "", 0)
	}
	if ri := r.Resolve(g.Get("RI")); ri != nil {
		if n, _ := ri.(pdf.Name); !renderingIntents[n] {
			s.report("PDF-55", where+" /RI", "the rendering intent %v is not one of RelativeColorimetric, AbsoluteColorimetric, Perceptual and Saturation", ri)
		}
	}
	switch bm := r.Resolve(g.Get("BM")).(type) {
	case pdf.Name:
		if !blendModes[bm] {
			s.report("PDF-58", where+" /BM", "the blend mode %s is not defined by ISO 32000-1", bm)
		}
	case pdf.Array:
		for _, x := range bm {
			if n, _ := r.Resolve(x).(pdf.Name); !blendModes[n] {
				s.report("PDF-58", where+" /BM", "the blend mode %v is not defined by ISO 32000-1", r.Resolve(x))
			}
		}
	}
}

// halftone checks a halftone and the halftones of the colourants of a type
// 5 halftone (6.2.5).
func (s *pdfaScan) halftone(o pdf.Object, where string, colourant pdf.Name, depth int) {
	r := s.c.r
	d, ok := r.Dict(o)
	if !ok || depth > 1 {
		return // the name Default
	}
	typ, _ := pdf.AsInt(r.Resolve(d.Get("HalftoneType")))
	if typ != 1 && typ != 5 {
		s.report("PDF-54", where+" /HalftoneType", "the halftone type is %d; PDF/A admits 1 and 5", typ)
	}
	if d.Has("HalftoneName") {
		s.report("PDF-54", where+" /HalftoneName", "the halftone has a /HalftoneName entry, which PDF/A forbids")
	}
	if colourant != "Default" {
		primary := colourant == "" || colourant == "Cyan" || colourant == "Magenta" || colourant == "Yellow" || colourant == "Black"
		has := d.Has("TransferFunction")
		switch {
		case primary && has:
			s.report("PDF-54", where+" /TransferFunction", "the halftone has a transfer function, which ISO 32000-1 only admits for the non-primary colourants of a type 5 halftone")
		case !primary && !has:
			s.report("PDF-54", where, "the halftone of the colourant %s has no transfer function, which ISO 32000-1 requires", colourant)
		}
	}
	if typ == 5 {
		for _, kv := range d {
			switch kv.Key {
			case "Type", "HalftoneType", "HalftoneName":
				continue
			}
			s.halftone(kv.Val, where+" /"+string(kv.Key), kv.Key, depth+1)
		}
	}
}

// imageKind tells how an image is used.
type imageKind uint8

const (
	imageXObject imageKind = iota // an image XObject
	imageSMask                    // the soft mask of an image
	imageMask                     // the explicit mask of an image
)

// image checks an image XObject or mask (6.2.8) and its colour space.
func (s *pdfaScan) image(d pdf.Dict, where string, def uint8, kind imageKind) {
	r := s.c.r
	if d.Has("Alternates") {
		s.report("PDF-56", where+" /Alternates", "the image has alternate images (/Alternates), which PDF/A forbids")
	}
	if d.Has("OPI") {
		s.report("PDF-56", where+" /OPI", "the image has an /OPI entry, which PDF/A forbids")
	}
	if b, _ := r.Resolve(d.Get("Interpolate")).(pdf.Bool); b {
		s.report("PDF-56", where+" /Interpolate", "the image has Interpolate true, which PDF/A forbids")
	}
	isMask, _ := r.Resolve(d.Get("ImageMask")).(pdf.Bool)
	mask := bool(isMask) || kind == imageMask
	if bpc, ok := pdf.AsInt(r.Resolve(d.Get("BitsPerComponent"))); ok {
		switch {
		case mask:
			if bpc != 1 {
				s.report("PDF-56", where+" /BitsPerComponent", "the image mask has %d bits per component; PDF/A requires 1", bpc)
			}
		case bpc != 1 && bpc != 2 && bpc != 4 && bpc != 8 && bpc != 16:
			s.report("PDF-56", where+" /BitsPerComponent", "the image has %d bits per component; PDF/A admits 1, 2, 4, 8 and 16", bpc)
		}
	}
	if ri := r.Resolve(d.Get("Intent")); ri != nil {
		if n, _ := ri.(pdf.Name); !renderingIntents[n] {
			s.report("PDF-55", where+" /Intent", "the rendering intent %v is not one of RelativeColorimetric, AbsoluteColorimetric, Perceptual and Saturation", ri)
		}
	}
	// A soft mask is DeviceGray by definition: its colour space is not
	// rendered as such.
	if !mask && kind == imageXObject {
		s.colourSpace(d.Get("ColorSpace"), where+" /ColorSpace", def, 0)
	}
}

// form checks a form XObject (6.2.9) and its transparency group.
func (s *pdfaScan) form(d pdf.Dict, where string, def uint8) {
	r := s.c.r
	if d.Has("OPI") {
		s.report("PDF-57", where+" /OPI", "the form XObject has an /OPI entry, which PDF/A forbids")
	}
	if s.name(d, "Subtype2") == "PS" || d.Has("PS") {
		s.report("PDF-57", where, "the form XObject is a PostScript form (/Subtype2 /PS or /PS), which PDF/A forbids")
	}
	if d.Has("Ref") {
		s.report("PDF-57", where+" /Ref", "the form XObject is a reference XObject (/Ref), which PDF/A forbids")
	}
	if g, ok := r.Dict(d.Get("Group")); ok {
		s.colourSpace(g.Get("CS"), where+" /Group /CS", def, 0)
	}
}

// standard14 are the names of the standard Type 1 fonts.
var standard14 = map[string]bool{
	"Times-Roman": true, "Times-Bold": true, "Times-Italic": true, "Times-BoldItalic": true,
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true, "Helvetica-BoldOblique": true,
	"Courier": true, "Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
	"Symbol": true, "ZapfDingbats": true,
}

// predefinedCMaps are the CMaps of ISO 32000-1, Table 118, which need not
// be embedded, with the ordering of their character collection.
var predefinedCMaps = func() map[string]string {
	m := map[string]string{"Identity-H": "", "Identity-V": ""}
	for ordering, names := range map[string]string{
		"GB1":    "GB-EUC-H GB-EUC-V GBpc-EUC-H GBpc-EUC-V GBK-EUC-H GBK-EUC-V GBKp-EUC-H GBKp-EUC-V GBK2K-H GBK2K-V UniGB-UCS2-H UniGB-UCS2-V UniGB-UTF16-H UniGB-UTF16-V",
		"CNS1":   "B5pc-H B5pc-V HKscs-B5-H HKscs-B5-V ETen-B5-H ETen-B5-V ETenms-B5-H ETenms-B5-V CNS-EUC-H CNS-EUC-V UniCNS-UCS2-H UniCNS-UCS2-V UniCNS-UTF16-H UniCNS-UTF16-V",
		"Japan1": "83pv-RKSJ-H 90ms-RKSJ-H 90ms-RKSJ-V 90msp-RKSJ-H 90msp-RKSJ-V 90pv-RKSJ-H Add-RKSJ-H Add-RKSJ-V EUC-H EUC-V Ext-RKSJ-H Ext-RKSJ-V H V UniJIS-UCS2-H UniJIS-UCS2-V UniJIS-UCS2-HW-H UniJIS-UCS2-HW-V UniJIS-UTF16-H UniJIS-UTF16-V",
		"Korea1": "KSC-EUC-H KSC-EUC-V KSCms-UHC-H KSCms-UHC-V KSCms-UHC-HW-H KSCms-UHC-HW-V KSCpc-EUC-H UniKS-UCS2-H UniKS-UCS2-V UniKS-UTF16-H UniKS-UTF16-V",
	} {
		for _, n := range strings.Fields(names) {
			m[n] = ordering
		}
	}
	return m
}()

// fontDict checks a font dictionary (6.2.11.2, 6.2.11.3, 6.2.11.6): its
// entries (PDF-59), the encoding of a TrueType font and the cmap table of
// its program (PDF-60), and its name (PDF-46).
func (s *pdfaScan) fontDict(d pdf.Dict, where string) {
	r := s.c.r
	sub := s.name(d, "Subtype")
	if t := s.name(d, "Type"); t != "Font" {
		s.report("PDF-59", where+" /Type", "the font dictionary has /Type %q, not /Font", t)
	}
	switch sub {
	case "Type1", "MMType1", "TrueType", "Type3", "Type0":
	default:
		s.report("PDF-59", where+" /Subtype", "the font has /Subtype %q, which ISO 32000-1 does not define", sub)
		return
	}
	base := s.name(d, "BaseFont")
	if sub != "Type3" && base == "" {
		s.report("PDF-59", where+" /BaseFont", "the font has no /BaseFont")
	}
	s.unicodeName(base, where+" /BaseFont", "font")

	if sub == "Type0" {
		s.compositeFont(d, where)
		return
	}
	fd, _ := r.Dict(d.Get("FontDescriptor"))
	s.fontDescriptor(fd, where)
	if fd != nil && (sub == "Type1" || sub == "MMType1") {
		s.charSet(d, fd, where)
	}
	embedded := fd != nil && (fd.Has("FontFile") || fd.Has("FontFile2") || fd.Has("FontFile3"))
	if sub == "Type3" || !standard14[string(base)] || embedded {
		first, ok1 := pdf.AsInt(r.Resolve(d.Get("FirstChar")))
		last, ok2 := pdf.AsInt(r.Resolve(d.Get("LastChar")))
		widths, ok3 := r.Resolve(d.Get("Widths")).(pdf.Array)
		switch {
		case !ok1:
			s.report("PDF-59", where+" /FirstChar", "the simple font has no /FirstChar")
		case !ok2:
			s.report("PDF-59", where+" /LastChar", "the simple font has no /LastChar")
		case !ok3:
			s.report("PDF-59", where+" /Widths", "the simple font has no /Widths array")
		case int64(len(widths)) != last-first+1:
			s.report("PDF-59", where+" /Widths", "the /Widths array has %d entries; LastChar - FirstChar + 1 is %d", len(widths), last-first+1)
		}
	}
	if sub == "TrueType" {
		s.trueType(d, fd, where)
	}
}

// fontDescriptor checks the font descriptor of a font: its name and the
// subtype of a FontFile3 program.
func (s *pdfaScan) fontDescriptor(fd pdf.Dict, where string) {
	if fd == nil {
		return
	}
	s.unicodeName(s.name(fd, "FontName"), where+" /FontDescriptor /FontName", "font")
	if ff, ok := s.c.r.Dict(fd.Get("FontFile3")); ok {
		switch st := s.name(ff, "Subtype"); st {
		case "", "Type1C", "CIDFontType0C", "OpenType":
		default:
			s.report("PDF-59", where+" /FontDescriptor /FontFile3 /Subtype", "the font program has /Subtype %s, not Type1C, CIDFontType0C or OpenType", st)
		}
	}
}

var wmodeDef = regexp.MustCompile(`/WMode\s+(\d+)\s+def`)
var useCMapOp = regexp.MustCompile(`/([^\s/\[\]<>(){}%]+)\s+usecmap`)

// compositeFont checks a Type 0 font: its CIDFont, the CIDSystemInfo of
// the CIDFont and of the CMap, and the CMap (6.2.11.3).
func (s *pdfaScan) compositeFont(d pdf.Dict, where string) {
	r := s.c.r
	var cid pdf.Dict
	if list, ok := r.Resolve(d.Get("DescendantFonts")).(pdf.Array); ok && len(list) > 0 {
		cid, _ = r.DictSkipArrays(list[0])
	}
	if cid == nil {
		s.report("PDF-59", where+" /DescendantFonts", "the Type 0 font has no CIDFont")
		return
	}
	cw := where + " /DescendantFonts [0]"
	sub := s.name(cid, "Subtype")
	if sub != "CIDFontType0" && sub != "CIDFontType2" {
		s.report("PDF-59", cw+" /Subtype", "the CIDFont has /Subtype %q, not CIDFontType0 or CIDFontType2", sub)
	}
	s.unicodeName(s.name(cid, "BaseFont"), cw+" /BaseFont", "font")
	fd, _ := r.Dict(cid.Get("FontDescriptor"))
	s.fontDescriptor(fd, cw)
	if fd != nil {
		s.cidSet(cid, fd, cw)
	}
	if sub == "CIDFontType2" && fd != nil && fd.Has("FontFile2") {
		switch m := r.Resolve(cid.Get("CIDToGIDMap")).(type) {
		case nil:
			s.report("PDF-59", cw+" /CIDToGIDMap", "the embedded Type 2 CIDFont has no /CIDToGIDMap")
		case *pdf.Stream:
		default:
			if m != pdf.Name("Identity") {
				s.report("PDF-59", cw+" /CIDToGIDMap", "the /CIDToGIDMap of the CIDFont is %v, neither a stream nor /Identity", m)
			}
		}
	}
	sysInfo := func(o pdf.Object) (registry, ordering string, supplement int64, ok bool) {
		si, ok := r.Dict(o)
		if !ok {
			return "", "", 0, false
		}
		supplement, _ = pdf.AsInt(r.Resolve(si.Get("Supplement")))
		return s.text(si, "Registry"), s.text(si, "Ordering"), supplement, true
	}
	cReg, cOrd, cSup, cOK := sysInfo(cid.Get("CIDSystemInfo"))

	switch enc := r.Resolve(d.Get("Encoding")).(type) {
	case pdf.Name:
		ordering, predefined := predefinedCMaps[string(enc)]
		switch {
		case !predefined:
			s.report("PDF-59", where+" /Encoding", "the CMap %s is neither embedded nor one of the predefined CMaps of ISO 32000-1 (Table 118)", enc)
		case ordering != "" && cOK && (cReg != "Adobe" || cOrd != ordering):
			s.report("PDF-59", cw+" /CIDSystemInfo", "the CIDSystemInfo of the CIDFont (%s-%s) does not match the character collection of the CMap %s (Adobe-%s)", cReg, cOrd, enc, ordering)
		}
	case *pdf.Stream:
		ew := where + " /Encoding"
		if reg, ord, sup, ok := sysInfo(enc.Dict.Get("CIDSystemInfo")); ok && cOK && (reg != cReg || ord != cOrd || cSup > sup) {
			s.report("PDF-59", cw+" /CIDSystemInfo", "the CIDSystemInfo of the CIDFont (%s-%s-%d) does not match the one of the CMap (%s-%s-%d)", cReg, cOrd, cSup, reg, ord, sup)
		}
		data, err := r.Decode(enc)
		if err == nil {
			if m := wmodeDef.FindSubmatch(data); m != nil {
				dictWMode, _ := pdf.AsInt(r.Resolve(enc.Dict.Get("WMode")))
				if string(m[1]) != fmt.Sprint(dictWMode) {
					s.report("PDF-59", ew+" /WMode", "the CMap dictionary has /WMode %d but the CMap program %s", dictWMode, m[1])
				}
			}
			if cid := parseCMap(data).maxCID(); cid > 65535 {
				s.report("PDF-49", ew, "the CMap maps a code to the CID %d; PDF/A admits CIDs up to 65535 (6.1.13)", cid)
			}
			for _, m := range useCMapOp.FindAllSubmatch(data, -1) {
				if _, ok := predefinedCMaps[string(m[1])]; !ok {
					s.report("PDF-59", ew, "the CMap uses the CMap %s, which is not one of the predefined CMaps", m[1])
				}
			}
		}
		switch u := r.Resolve(enc.Dict.Get("UseCMap")).(type) {
		case pdf.Name:
			if _, ok := predefinedCMaps[string(u)]; !ok {
				s.report("PDF-59", ew+" /UseCMap", "the CMap uses the CMap %s, which is not one of the predefined CMaps", u)
			}
		case *pdf.Stream:
			s.report("PDF-59", ew+" /UseCMap", "the CMap uses an embedded CMap; PDF/A only admits the predefined CMaps")
		}
	}
}

// trueType checks the encoding of a TrueType font and the cmap table of its
// embedded program (6.2.11.6).
func (s *pdfaScan) trueType(d, fd pdf.Dict, where string) {
	r := s.c.r
	if fd == nil {
		return // not embedded: PDF-31
	}
	flags, _ := pdf.AsInt(r.Resolve(fd.Get("Flags")))
	symbolic := flags&4 != 0
	enc := r.Resolve(d.Get("Encoding"))
	var diffs pdf.Array
	if symbolic {
		if enc != nil {
			s.report("PDF-60", where+" /Encoding", "the symbolic TrueType font has an /Encoding entry, which PDF/A forbids")
		}
	} else {
		var name pdf.Name
		switch e := enc.(type) {
		case pdf.Name:
			name = e
		case pdf.Dict:
			name = s.name(e, "BaseEncoding")
			diffs, _ = r.Resolve(e.Get("Differences")).(pdf.Array)
		}
		if name != "MacRomanEncoding" && name != "WinAnsiEncoding" {
			s.report("PDF-60", where+" /Encoding", "the non-symbolic TrueType font has neither MacRomanEncoding nor WinAnsiEncoding as encoding (or base encoding)")
		}
	}
	tt := s.program(fd).tt
	if tt == nil || s.program(fd).key != "FontFile2" {
		return
	}
	ms, sym := font.CmapID{Platform: 3, Encoding: 1}, font.CmapID{Platform: 3, Encoding: 0}
	n, has30 := len(tt.Cmaps()), tt.HasCmap(sym)
	ff := where + " /FontDescriptor /FontFile2"
	switch {
	case symbolic && n != 1 && !has30:
		s.report("PDF-60", ff, "the cmap table of the symbolic TrueType font has %d encodings and no Microsoft Symbol (3,0) encoding", n)
	case !symbolic && (n == 0 || (has30 && n == 1)):
		s.report("PDF-60", ff, "the cmap table of the non-symbolic TrueType font has no non-symbolic encoding")
	}
	if len(diffs) == 0 {
		return
	}
	if !tt.HasCmap(ms) {
		s.report("PDF-60", ff, "the non-symbolic TrueType font has a /Differences array but its cmap table has no Microsoft Unicode (3,1) encoding to look the glyph names up")
		return
	}
	for _, x := range diffs {
		if n, ok := r.Resolve(x).(pdf.Name); ok && !font.InGlyphList(string(n)) {
			s.report("PDF-60", where+" /Encoding /Differences", "the glyph name %s of the non-symbolic TrueType font is not in the Adobe Glyph List", n)
			return
		}
	}
}

// annotTypes are the annotation types PDF/A admits (6.3.1).
var annotTypes = map[pdf.Name]bool{
	"Text": true, "Link": true, "FreeText": true, "Line": true, "Square": true, "Circle": true, "Polygon": true,
	"PolyLine": true, "Highlight": true, "Underline": true, "Squiggly": true, "StrikeOut": true, "Stamp": true,
	"Caret": true, "Ink": true, "Popup": true, "FileAttachment": true, "Widget": true, "PrinterMark": true,
	"TrapNet": true, "Watermark": true, "Redact": true,
}

// annotationDict checks the type, flags and appearance of an annotation
// (6.3) and the action of a widget (6.4.1).
func (s *pdfaScan) annotationDict(d pdf.Dict, where string) {
	r := s.c.r
	sub := s.name(d, "Subtype")
	if !annotTypes[sub] {
		s.report("PDF-61", where+" /Subtype", "the annotation type %q is forbidden by PDF/A", sub)
	}
	f, hasF := pdf.AsInt(r.Resolve(d.Get("F")))
	switch {
	case !hasF && sub != "Popup":
		s.report("PDF-62", where+" /F", "the annotation has no flags (/F); PDF/A requires the Print flag")
	case hasF && (f&4 == 0 || f&(1|2|32|256) != 0):
		s.report("PDF-62", where+" /F", "the annotation flags are %d; PDF/A requires Print set and Invisible, Hidden, NoView and ToggleNoView clear", f)
	}
	zero := false
	if rect, ok := r.Resolve(d.Get("Rect")).(pdf.Array); ok && len(rect) == 4 {
		num := func(i int) float64 {
			switch v := r.Resolve(rect[i]).(type) {
			case pdf.Int:
				return float64(v)
			case pdf.Real:
				return float64(v)
			}
			return math.NaN()
		}
		zero = num(0) == num(2) && num(1) == num(3)
	}
	ap, hasAP := r.Dict(d.Get("AP"))
	switch {
	case !hasAP:
		if !zero && sub != "Popup" && sub != "Link" {
			s.report("PDF-63", where+" /AP", "the annotation has no appearance dictionary (/AP)")
		}
	default:
		for _, kv := range ap {
			if kv.Key != "N" {
				s.report("PDF-63", where+" /AP /"+string(kv.Key), "the appearance dictionary has a /%s entry; PDF/A admits only N", kv.Key)
			}
		}
		if n := r.Resolve(ap.Get("N")); n != nil {
			_, isStream := n.(*pdf.Stream)
			if sub == "Widget" && s.fieldType(d) == "Btn" {
				if isStream {
					s.report("PDF-63", where+" /AP /N", "the normal appearance of a button widget is a stream; PDF/A requires an appearance subdictionary")
				}
			} else if !isStream {
				s.report("PDF-63", where+" /AP /N", "the normal appearance of the annotation is not a stream")
			}
		}
	}
	if sub == "Widget" && d.Has("A") {
		s.report("PDF-64", where+" /A", "the widget annotation has an action (/A), which PDF/A forbids")
	}
}

// fieldType returns the field type of a widget annotation, inherited from
// its parent fields.
func (s *pdfaScan) fieldType(d pdf.Dict) pdf.Name {
	r := s.c.r
	for i := 0; i < maxScanDepth && d != nil; i++ {
		if ft := s.name(d, "FT"); ft != "" {
			return ft
		}
		d, _ = r.Dict(d.Get("Parent"))
	}
	return ""
}

// pageBoxes checks the size of the page boundaries (6.1.13).
func (s *pdfaScan) pageBoxes(page pdf.Dict, inherited map[pdf.Name]pdf.Object, where string) {
	r := s.c.r
	for _, key := range []pdf.Name{"MediaBox", "CropBox", "BleedBox", "TrimBox", "ArtBox"} {
		o := page.Get(key)
		if o == nil {
			o = inherited[key]
		}
		box, ok := r.Resolve(o).(pdf.Array)
		if !ok || len(box) != 4 {
			continue
		}
		var v [4]float64
		for i, x := range box {
			switch n := r.Resolve(x).(type) {
			case pdf.Int:
				v[i] = float64(n)
			case pdf.Real:
				v[i] = float64(n)
			}
		}
		w, h := math.Abs(v[2]-v[0]), math.Abs(v[3]-v[1])
		if w < 3 || h < 3 || w > 14400 || h > 14400 {
			s.report("PDF-49", where+" /"+string(key), "the page boundary is %g × %g units; PDF/A admits 3 to 14400 units in each direction", w, h)
		}
	}
}

// structureNames checks the structure type names of the structure tree
// (6.1.8).
func (s *pdfaScan) structureNames() {
	r := s.c.r
	root, ok := r.Dict(s.c.catalog.Get("StructTreeRoot"))
	if !ok {
		return
	}
	visits := 0
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		if o == nil || depth > maxScanDepth || visits > maxScanObjects {
			return
		}
		visits++
		switch v := r.Resolve(o).(type) {
		case pdf.Array:
			for _, x := range v {
				walk(x, depth)
			}
		case pdf.Dict:
			if t := s.name(v, "Type"); t == "MCR" || t == "OBJR" {
				return
			}
			s.unicodeName(s.name(v, "S"), "structure element /S", "structure type")
			walk(v.Get("K"), depth+1)
		}
	}
	if rm, ok := r.Dict(root.Get("RoleMap")); ok {
		for _, kv := range rm {
			s.unicodeName(kv.Key, "catalog /StructTreeRoot /RoleMap", "structure type")
		}
	}
	walk(root.Get("K"), 0)
}
