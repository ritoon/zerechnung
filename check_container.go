package zerechnung

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmp"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// ContainerCatalog lists the checks of CheckContainer. They cover what the
// Factur-X specification (and ZUGFeRD, XRechnung hybrids) requires from the
// PDF container — the invoice attachment, its file specification and the
// XMP metadata — and the PDF/A requirements a reader can verify without
// rendering the pages. PDF-41 to PDF-69, PDF-71 and PDF-72 follow ISO
// 19005-2 and 19005-3 clause by clause, as the veraPDF validation profiles
// for PDF/A-2b and PDF/A-3b number them: file structure, colour spaces,
// graphics states, images, fonts, annotations, forms, XMP metadata,
// optional content, and what the content streams of the pages use
// (operators, resources, colours, inline images, transparency, glyphs).
// They are not applied to a document that declares PDF/A-4. PDF-70 applies
// to that document alone, in place of PDF-35: ISO 19005-4 restricts the
// document information dictionary (6.1.3), as rules 6.1.3-4 and 6.1.3-5 of
// the veraPDF profile for PDF/A-4 check it. The checks do not replace a
// PDF/A validator: Type 1 font programs are not read, and the level A and
// U requirements (logical structure, Unicode mapping) are not checked.
var ContainerCatalog = []rules.RuleInfo{
	{ID: "PDF-01", Severity: rules.Error, Description: "The PDF is not encrypted (PDF/A forbids encryption; the attachment cannot be read)."},
	{ID: "PDF-02", Severity: rules.Error, Description: "The document catalog has an XMP metadata stream (/Metadata)."},
	{ID: "PDF-03", Severity: rules.Error, Description: "The XMP metadata is a readable RDF/XML packet."},
	{ID: "PDF-04", Severity: rules.Error, Description: "The XMP metadata declares PDF/A-3 or PDF/A-4 (pdfaid:part is 3 or 4), the parts that allow embedded XML files."},
	{ID: "PDF-05", Severity: rules.Error, Description: "The XMP metadata declares a PDF/A conformance level that allows embedded XML files (pdfaid:conformance is A, B or U for PDF/A-3, F or E for PDF/A-4)."},
	{ID: "PDF-06", Severity: rules.Warning, Description: "The catalog has a PDF/A output intent (/OutputIntents with /S /GTS_PDFA1). PDF/A requires it as soon as a device-dependent colour space is used (see PDF-52)."},
	{ID: "PDF-07", Severity: rules.Error, Description: "The PDF/A output intent has an ICC profile (/DestOutputProfile stream)."},
	{ID: "PDF-08", Severity: rules.Error, Description: "The trailer has a file identifier (/ID array of two strings, the second not empty), as PDF/A requires."},
	{ID: "PDF-09", Severity: rules.Error, Description: "The PDF has an invoice XML attachment that can be decoded."},
	{ID: "PDF-10", Severity: rules.Error, Description: "The invoice attachment has an invoice file name: factur-x.xml, xrechnung.xml, or the legacy ZUGFeRD names zugferd-invoice.xml and ZUGFeRD-invoice.xml."},
	{ID: "PDF-11", Severity: rules.Warning, Description: "The invoice attachment has the name of its profile in current specifications: factur-x.xml, or xrechnung.xml for XRECHNUNG. Legacy ZUGFeRD names are reported here."},
	{ID: "PDF-12", Severity: rules.Error, Description: "The file specification of the invoice has an /AFRelationship entry (PDF/A-3 associated file)."},
	{ID: "PDF-13", Severity: rules.Error, Description: "The /AFRelationship of the invoice is Data, Alternative or Source."},
	{ID: "PDF-14", Severity: rules.Warning, Description: "The /AFRelationship matches the profile: Data for MINIMUM and BASIC WL, Alternative or Source for BASIC and above. Data is not reported for BASIC and above when the French rules apply: France admits it (Factur-X 1.09, 6.2.2)."},
	{ID: "PDF-15", Severity: rules.Error, Description: "The file specification of the invoice is listed in the /AF array of the document catalog."},
	{ID: "PDF-16", Severity: rules.Error, Description: "The embedded file stream has an XML MIME type (/Subtype /text#2Fxml)."},
	{ID: "PDF-17", Severity: rules.Warning, Description: "The MIME type of the embedded file is text/xml rather than application/xml."},
	{ID: "PDF-18", Severity: rules.Error, Description: "The embedded file stream has a modification date (/Params /ModDate), as PDF/A-3 requires."},
	{ID: "PDF-19", Severity: rules.Warning, Description: "The modification date of the embedded file is a valid PDF date (D:YYYYMMDDHHmmSSOHH'mm')."},
	{ID: "PDF-20", Severity: rules.Error, Description: "The file specification of the invoice has both a /F and a /UF file name, as PDF/A-3 requires."},
	{ID: "PDF-21", Severity: rules.Error, Description: "The XMP metadata has the hybrid-invoice schema (Factur-X, ZUGFeRD 2.0 or ZUGFeRD 1.0 namespace)."},
	{ID: "PDF-22", Severity: rules.Error, Description: "The XMP DocumentType is INVOICE."},
	{ID: "PDF-23", Severity: rules.Error, Description: "The XMP DocumentFileName is the name of the invoice attachment."},
	{ID: "PDF-24", Severity: rules.Error, Description: "The XMP ConformanceLevel is a known level and matches the profile of the XML (BT-24). EN 16931 is accepted for an XRECHNUNG invoice, EXTENDED for EXTENDED-CTC-FR."},
	{ID: "PDF-25", Severity: rules.Error, Description: "The XMP metadata describes the hybrid-invoice schema and its properties in a PDF/A extension schema (pdfaExtension:schemas)."},
	{ID: "PDF-26", Severity: rules.Warning, Description: "The XMP Version of the hybrid-invoice schema is set (\"1.0\" for Factur-X)."},
	{ID: "PDF-27", Severity: rules.Warning, Description: "The XMP namespace matches the XML syntax: Factur-X or ZUGFeRD 2.0 for CrossIndustryInvoice, ZUGFeRD 1.0 for CrossIndustryDocument."},
	{ID: "PDF-28", Severity: rules.Error, Description: "The PDF has a single invoice attachment: no other embedded file is named factur-x.xml, xrechnung.xml or zugferd-invoice.xml."},
	{ID: "PDF-29", Severity: rules.Warning, Description: "A PDF/A-4 declaration gives the edition of ISO 19005-4 it follows (pdfaid:rev, a four-digit year: 2020)."},
	{ID: "PDF-30", Severity: rules.Warning, Description: "The PDF version matches the declared PDF/A part: PDF 1.7 or earlier for PDF/A-3, PDF 2.0 for PDF/A-4."},
	{ID: "PDF-31", Severity: rules.Warning, Description: "The fonts of the pages (in the resources of the pages, form XObjects, patterns, Type 3 fonts and annotation appearances) embed their font program (/FontFile, /FontFile2 or /FontFile3 in the font descriptor). PDF/A requires it for the fonts used for rendering; whether the content draws visible text with them is not analysed."},
	{ID: "PDF-32", Severity: rules.Error, Description: "The document has no document-level JavaScript (/JavaScript in the catalog /Names), which PDF/A forbids."},
	{ID: "PDF-33", Severity: rules.Error, Description: "No action forbidden by PDF/A (Launch, Sound, Movie, ResetForm, ImportData, Hide, SetOCGState, Rendition, Trans, GoTo3DView, JavaScript, SetState, NOP, or a named action other than NextPage, PrevPage, FirstPage and LastPage) in the /OpenAction, the outlines, the annotations or a chain of /Next actions."},
	{ID: "PDF-34", Severity: rules.Error, Description: "No additional-actions dictionary (/AA) in the document catalog, the pages, the annotations or the form fields, as PDF/A requires."},
	{ID: "PDF-35", Severity: rules.Warning, Description: "The entries of the document information dictionary (/Info) that have an XMP equivalent (Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate) have the same value in the XMP metadata. ISO 19005-1 required it (6.7.3); ISO 19005-2 and 19005-3 no longer do (6.1.5), but a reader may show either value. Not checked for PDF/A-4, which only admits a document information dictionary holding ModDate alone (6.1.3)."},
	{ID: "PDF-36", Severity: rules.Error, Description: "Every embedded file besides the invoice has an /AFRelationship entry (PDF/A-3 associated file)."},
	{ID: "PDF-37", Severity: rules.Error, Description: "Every embedded file besides the invoice has a MIME type (/Subtype of the embedded file stream), as PDF/A-3 requires."},
	{ID: "PDF-38", Severity: rules.Error, Description: "No stream uses a filter PDF/A forbids: LZWDecode, Crypt with a crypt filter other than Identity, or a filter ISO 32000-1 does not define (ISO 19005-3, 6.1.7.2). The streams of the pages, their resources, the annotation appearances, the font programs, the embedded files and the metadata are checked, then every other stream of the file. The filters of the inline images of the content streams are checked too."},
	{ID: "PDF-39", Severity: rules.Error, Description: "Every embedded file besides the invoice is an associated file, listed in an /AF array (of the document catalog, a page, an annotation, a form or image XObject, a marked-content property list of the resources or a structure element), as PDF/A-3 requires. Files associated otherwise (property lists inside the page content, document parts) are not found."},
	{ID: "PDF-40", Severity: rules.Error, Description: "Where the German rules apply, the /AFRelationship of a BASIC, EN 16931, EXTENDED or XRECHNUNG invoice is Alternative: Germany admits no other value (Factur-X 1.09, 5.3 and 6.2.2). They apply to XRECHNUNG invoices; Check also applies them when validate.Options selects the German rule sets (Germany, GermanyProfile, or Auto for a seller in Germany)."},
	{ID: "PDF-41", Severity: rules.Error, Description: "The file header is %PDF-1.n (n from 0 to 7) at byte zero, followed by a single end-of-line marker and a comment of at least four bytes above 127 (6.1.2)."},
	{ID: "PDF-42", Severity: rules.Error, Description: "Nothing follows the last end-of-file marker (%%EOF) but an optional single end-of-line marker (6.1.3)."},
	{ID: "PDF-43", Severity: rules.Error, Description: "In every cross-reference table, the xref keyword and the subsection header are separated by a single end-of-line marker (6.1.4)."},
	{ID: "PDF-44", Severity: rules.Error, Description: "Hexadecimal strings, of the objects and of the content streams, hold an even number of hexadecimal digits and no other character than white space (6.1.6)."},
	{ID: "PDF-45", Severity: rules.Error, Description: "Streams are well formed (6.1.7.1): the stream keyword is followed by CR LF or LF, endstream is preceded by an end-of-line marker, /Length is the number of bytes of the data, and no stream dictionary has an /F, /FFilter or /FDecodeParms entry (external data)."},
	{ID: "PDF-46", Severity: rules.Error, Description: "Font names, colourant names of Separation and DeviceN colour spaces and structure type names are valid UTF-8 (6.1.8)."},
	{ID: "PDF-47", Severity: rules.Error, Description: "Indirect objects are well formed (6.1.9): object number, generation number and obj keyword separated by single white-space characters, the object number and the endobj keyword preceded by an end-of-line marker, obj and endobj followed by one."},
	{ID: "PDF-48", Severity: rules.Error, Description: "The permissions dictionary of the catalog (/Perms) has only UR3 and DocMDP entries, and the signature references of DocMDP have no /DigestLocation, /DigestMethod or /DigestValue (6.1.12)."},
	{ID: "PDF-49", Severity: rules.Error, Description: "The implementation limits of PDF/A are respected (6.1.13): integers within 32 bits, real numbers within ±3.403 × 10^38 and not closer to zero than ±1.175 × 10^-38, strings of at most 32767 bytes, names of at most 127 bytes, at most 8388607 indirect objects, at most 32 DeviceN colourants, page boundaries of 3 to 14400 units. In the content streams: at most 28 nested q operators, and the limits of the numbers and strings; a CMap maps no code to a CID above 65535."},
	{ID: "PDF-50", Severity: rules.Error, Description: "The output intents agree (6.2.3): every /DestOutputProfile is the same indirect object, an output (prtr) or monitor (mntr) ICC profile of version 4 at most in GRAY, RGB or CMYK, and a PDF/X output intent has no /DestOutputProfileRef."},
	{ID: "PDF-51", Severity: rules.Error, Description: "The ICC profiles of the ICCBased colour spaces of the resources, images and transparency groups are input, output, monitor or colour space profiles (scnr, prtr, mntr, spac) of version 4 at most, in GRAY, RGB, CMYK or Lab, and the content does not paint in an ICCBased CMYK colour space with overprinting on and the overprint mode 1 (6.2.4.2)."},
	{ID: "PDF-52", Severity: rules.Warning, Description: "DeviceRGB, DeviceCMYK and DeviceGray, where a colour space of the resources, an image or a transparency group uses them, are covered by a default colour space (DefaultRGB, DefaultCMYK, DefaultGray) or by a PDF/A output intent of the same colour model, any output intent for DeviceGray (6.2.4.3). The content streams are read: their colour operators (rg, k, g, cs…), the colour spaces they select, their inline images and the painting of uncoloured patterns and Type 3 glyphs."},
	{ID: "PDF-53", Severity: rules.Warning, Description: "The spot colours of DeviceN colour spaces have an entry in the Colorants dictionary, and the Separation colour spaces of the same name have the same alternate space and tint transform (6.2.4.4)."},
	{ID: "PDF-54", Severity: rules.Error, Description: "No graphics state has a transfer function (/TR, /TR2 other than Default) or an /HTP entry, and the halftones are of type 1 or 5, without /HalftoneName, with a transfer function only for the non-primary colourants (6.2.5)."},
	{ID: "PDF-55", Severity: rules.Error, Description: "The rendering intents of the graphics states and images are RelativeColorimetric, AbsoluteColorimetric, Perceptual or Saturation (6.2.6). So are the ri operator and the inline images of the content streams."},
	{ID: "PDF-56", Severity: rules.Error, Description: "Images have no /Alternates or /OPI entry, Interpolate is not true, and BitsPerComponent is 1, 2, 4, 8 or 16, 1 for an image mask (6.2.8). The inline images of the content streams are checked as well, and JPEG 2000 data (6.2.8.3) is a JP2 file of 1, 3 or 4 components of 1 to 38 bits, without a bits per component box and, for an image without /ColorSpace, with one colour specification of method 1 to 3 that is not CIELab (enumerated colour space 19)."},
	{ID: "PDF-57", Severity: rules.Error, Description: "No form XObject has an /OPI entry, a /Subtype2 /PS or /PS entry, or is a reference XObject (/Ref), and no XObject is a PostScript XObject (6.2.9)."},
	{ID: "PDF-58", Severity: rules.Error, Description: "Graphics states and annotations use the blend modes of ISO 32000-1 only, and a page whose content uses transparency (a soft mask, a constant alpha below 1, a blend mode other than Normal, a transparency group) has a group colour space when the document has no PDF/A output intent (6.2.10)."},
	{ID: "PDF-59", Severity: rules.Warning, Description: "The font dictionaries are well formed (6.2.11.2, 6.2.11.3): Type, Subtype and BaseFont; FirstChar, LastChar and a Widths array of LastChar - FirstChar + 1 entries for a simple font other than an unembedded standard font; for a Type 0 font, a CIDFont whose CIDSystemInfo matches the CMap, a predefined or embedded CMap with a consistent WMode that uses only predefined CMaps, a CIDToGIDMap for an embedded Type 2 CIDFont; a FontFile3 of subtype Type1C, CIDFontType0C or OpenType. The glyphs of the font programs are checked by PDF-72."},
	{ID: "PDF-60", Severity: rules.Warning, Description: "TrueType fonts are encoded as 6.2.11.6 requires: a non-symbolic font has MacRomanEncoding or WinAnsiEncoding (as /Encoding or /BaseEncoding) and a non-symbolic cmap encoding in its program; the glyph names of the Differences of a non-symbolic font are in the Adobe Glyph List, and its program has a Microsoft Unicode (3,1) cmap encoding to look them up; a symbolic font has no /Encoding and a cmap table with a single encoding or a Microsoft Symbol (3,0) encoding."},
	{ID: "PDF-61", Severity: rules.Error, Description: "The annotations are of a type of ISO 32000-1 PDF/A admits: neither 3D, Sound, Screen or Movie, nor a type ISO 32000-1 does not define (6.3.1)."},
	{ID: "PDF-62", Severity: rules.Error, Description: "The annotations other than Popup have flags (/F), with Print set and Invisible, Hidden, NoView and ToggleNoView clear (6.3.2)."},
	{ID: "PDF-63", Severity: rules.Error, Description: "The annotations have an appearance dictionary with a normal appearance (N) only: a stream, or a subdictionary for a button widget. Popup and Link annotations and those of zero size need none (6.3.3)."},
	{ID: "PDF-64", Severity: rules.Error, Description: "The interactive forms are static (6.4): NeedAppearances is not true, there is no XFA form and no NeedsRendering entry in the catalog, and no widget annotation has an action (/A)."},
	{ID: "PDF-65", Severity: rules.Error, Description: "The XMP metadata streams are well formed (6.6.2.1): the metadata stream of the catalog has /Type /Metadata and /Subtype /XML, and every XMP packet of the file is well-formed RDF/XML encoded in UTF-8, without bytes or encoding attribute in its xpacket header."},
	{ID: "PDF-66", Severity: rules.Error, Description: "Every XMP property belongs to a schema predefined by the XMP Specification 2005 or ISO 19005, or is described by a PDF/A extension schema of its packet or of the document metadata (6.6.2.3.1), and the properties of the PDF/A identification schema have the prefix pdfaid (6.6.4). The values of the predefined properties have the value type of their schema. The property names of the large predefined schemas (Exif, TIFF, Photoshop, Camera Raw, dynamic media, media management) and the values of the properties described by an extension schema are not checked; the properties of the hybrid-invoice schema are checked by PDF-25."},
	{ID: "PDF-67", Severity: rules.Error, Description: "The PDF/A extension schemas are described as 6.6.2.3.3 requires: a pdfaExtension:schemas Bag; schemas with pdfaSchema:schema, namespaceURI and prefix; properties with pdfaProperty:name, valueType, category (external or internal) and description; value types with pdfaType:type, namespaceURI, prefix and description; fields with pdfaField:name, valueType and description; the required prefixes and no other field. Whether the value types are defined is not checked."},
	{ID: "PDF-68", Severity: rules.Error, Description: "The optional content configurations have a unique /Name, no /AS entry, and an /Order array, when present, that lists every optional content group (6.9)."},
	{ID: "PDF-69", Severity: rules.Error, Description: "The document has no alternate presentations (/AlternatePresentations in the catalog /Names), no presentation steps (/PresSteps of a page) and no /Requirements entry in the catalog (6.10, 6.11)."},
	{ID: "PDF-70", Severity: rules.Error, Description: "A document that declares PDF/A-4 has no document information dictionary (/Info in the trailer) unless the catalog has a /PieceInfo entry, and then one holding ModDate alone (ISO 19005-4, 6.1.3): PDF 2.0 deprecates /Info in favour of the XMP metadata. Not checked for the other parts, whose /Info PDF-35 compares with the XMP metadata."},
	{ID: "PDF-71", Severity: rules.Error, Description: "The content streams (of the pages, the form XObjects, tiling patterns and soft masks they use, the glyphs of the Type 3 fonts they show and the annotation appearances) use only the operators of ISO 32000-1, even between BX and EX, and take their named resources from their own resource dictionary: not from the page tree for the content of a page, not from the page for another stream (6.2.2)."},
	{ID: "PDF-72", Severity: rules.Error, Description: "The glyphs the text of the content streams shows are defined by the embedded font program (6.2.11.4.1) with the widths of the font dictionary, to one unit in a thousand (6.2.11.5), and none is the .notdef glyph (6.2.11.8); the CharSet of a subset Type 1 font and the CIDSet of a subset CIDFont list the glyphs of the program, no more and no fewer (6.2.11.4.2). Codes are mapped to glyphs as veraPDF does. The TrueType and CFF programs and the glyph descriptions of Type 3 fonts are read; Type 1 programs, the glyphs and widths of invisible text (rendering mode 3) and the Type 0 fonts whose CMap is predefined but not Identity are not checked."},
}

// countries holds the countries whose usage of the container applies to an
// invoice. Factur-X 1.09 (5.3 and 6.2.2) lets France and Germany differ on
// the /AFRelationship of BASIC, EN 16931 and EXTENDED invoices: France
// admits Alternative, Source and Data, Germany only Alternative.
type countries uint8

const (
	inFrance countries = 1 << iota
	inGermany
)

// countriesOf returns the countries whose rules the rule sets select.
func countriesOf(set validate.RuleSet) countries {
	var c countries
	if set&validate.France != 0 {
		c |= inFrance
	}
	if set&(validate.Germany|validate.GermanyProfile) != 0 {
		c |= inGermany
	}
	return c
}

// containerSeverity returns the severity of a rule of ContainerCatalog.
func containerSeverity(id string) rules.Severity {
	for _, r := range ContainerCatalog {
		if r.ID == id {
			return r.Severity
		}
	}
	return rules.Error
}

// containerCheck accumulates findings.
type containerCheck struct {
	findings []rules.Finding
	level    validate.PDFALevel // the PDF/A checks to apply
}

func (k *containerCheck) add(id, path, format string, args ...any) {
	k.findings = append(k.findings, rules.Finding{
		Rule:     id,
		Severity: containerSeverity(id),
		Message:  fmt.Sprintf(format, args...),
		Path:     path,
		Source:   rules.SourcePDF,
	})
}

// CheckContainer checks the PDF container of a hybrid invoice: the PDF/A-3
// or PDF/A-4 identification and XMP metadata, the invoice attachment and its
// file specification, the consistency of the XMP hybrid-invoice schema
// with the attachment and its profile (BT-24), and the PDF/A requirements
// that do not depend on rendering the pages (fonts, actions, document
// information, other attachments, filters). The rules are listed in
// ContainerCatalog; every finding has Source rules.SourcePDF.
//
// The national usage of the /AFRelationship depends on the seller's country,
// which the container does not tell: CheckContainer applies the German one
// (PDF-40) to XRECHNUNG invoices only. Check applies the French and German
// usages whenever validate.Options selects their rule sets.
//
// The returned attachment is the one Extract returns, or nil when the PDF is
// encrypted (finding PDF-01) or has no readable invoice attachment (finding
// PDF-09). The error is reserved for files that cannot be read as a PDF.
func CheckContainer(pdfData []byte) (*Attachment, []rules.Finding, error) {
	return checkContainer(pdfData, nil, validate.PDFAFull)
}

// checkContainer implements CheckContainer. national, when not nil, returns
// the countries whose rules apply to the invoice XML; the German rules
// always apply to XRECHNUNG. level selects the PDF/A checks.
func checkContainer(pdfData []byte, national func(xml []byte) countries, level validate.PDFALevel) (*Attachment, []rules.Finding, error) {
	k := &containerCheck{level: level}
	c, err := openContainer(pdfData)
	if err != nil {
		if errors.Is(err, ErrEncrypted) {
			k.add("PDF-01", "trailer /Encrypt", "the PDF is encrypted")
			return nil, k.findings, nil
		}
		return nil, nil, err
	}

	info := k.document(c)

	fs, data, st, err := c.invoice()
	if err != nil {
		if errors.Is(err, ErrNoInvoice) {
			k.add("PDF-09", "catalog /Names /EmbeddedFiles", "no invoice XML attachment (factur-x.xml, xrechnung.xml, zugferd-invoice.xml or a CII XML file)")
		} else {
			k.add("PDF-09", specPath(fs), "the invoice attachment cannot be decoded: %v", err)
		}
		k.pdfa(c, fs, info)
		return nil, k.findings, nil
	}
	a := c.attachment(fs, data, st, metadataOf(info))
	var others []string
	for _, o := range c.specs {
		if o != fs && o.rank() >= 0 {
			others = append(others, strconv.Quote(o.filename()))
		}
	}
	if len(others) > 0 {
		k.add("PDF-28", "catalog /Names /EmbeddedFiles", "besides %q, the PDF has other invoice attachments: %s", a.Filename, strings.Join(others, ", "))
	}
	root, id := specificationID(data)
	profile, zf1 := xmlProfile(root, id)
	var where countries
	if national != nil {
		where = national(data)
	}
	if profile == invoice.ProfileXRechnung {
		where |= inGermany
	}
	k.attachment(c, fs, st, a, profile, zf1, where)
	if info != nil {
		k.metadata(info, a, root, profile, zf1)
	}
	// Last: the scan adds the files of page-level /AF arrays to c.specs,
	// which PDF-28 does not look at.
	k.pdfa(c, fs, info)
	return a, k.findings, nil
}

// document checks the XMP identification, the output intent and the file
// identifier. It returns the parsed XMP metadata, or nil.
func (k *containerCheck) document(c *container) *xmp.Info {
	info, present, err := c.xmpInfo()
	switch {
	case !present:
		k.add("PDF-02", "catalog /Metadata", "the document catalog has no XMP metadata stream")
	case info == nil && err == nil:
		k.add("PDF-03", "catalog /Metadata", "the XMP metadata stream cannot be decoded")
	case err != nil:
		k.add("PDF-03", "catalog /Metadata", "the XMP metadata cannot be read: %v", err)
		info = nil
	}
	if info != nil {
		k.identification(c, info)
		// PDF/A-4 deprecates the document information dictionary: it
		// admits one holding ModDate alone, with no XMP equivalence.
		if strings.TrimSpace(info.PDFAPart) == "4" {
			k.docInfoPDFA4(c)
		} else {
			k.docInfo(c, info)
		}
	}

	found := false
	if intents, ok := c.r.Resolve(c.catalog.Get("OutputIntents")).(pdf.Array); ok {
		for i, o := range intents {
			d, ok := c.r.Dict(o)
			if !ok {
				continue
			}
			if s, _ := c.r.Resolve(d.Get("S")).(pdf.Name); s != "GTS_PDFA1" {
				continue
			}
			found = true
			if _, ok := c.r.Resolve(d.Get("DestOutputProfile")).(*pdf.Stream); !ok {
				k.add("PDF-07", fmt.Sprintf("catalog /OutputIntents [%d]", i), "the PDF/A output intent has no /DestOutputProfile ICC profile stream")
			}
			break
		}
	}
	if !found {
		k.add("PDF-06", "catalog /OutputIntents", "the catalog has no PDF/A output intent (/S /GTS_PDFA1)")
	}

	if id, ok := c.r.Resolve(c.r.Trailer().Get("ID")).(pdf.Array); !ok || len(id) != 2 {
		k.add("PDF-08", "trailer /ID", "the trailer has no file identifier (/ID array of two strings)")
	} else if last, _ := c.r.Resolve(id[1]).(pdf.String); len(last) == 0 {
		k.add("PDF-08", "trailer /ID", "the file identifier is empty")
	} else if !c.r.NewestTrailer().Has("ID") {
		// veraPDF reads the identifier of the newest trailer alone (6.1.3).
		k.add("PDF-08", "trailer /ID", "the newest trailer of the file (the first-page trailer of a linearized file, or the trailer of the last incremental update) has no file identifier; only an older trailer has one")
	}
	return info
}

// identification checks the PDF/A identification of the XMP metadata
// against what a hybrid invoice needs: PDF/A-3 (any level), or PDF/A-4 at
// level F or E, which allow embedded files of any type. Plain PDF/A-4 only
// allows embedded PDF/A files.
func (k *containerCheck) identification(c *container, info *xmp.Info) {
	part := strings.TrimSpace(info.PDFAPart)
	switch part {
	case "3", "4":
	case "":
		k.add("PDF-04", "XMP pdfaid:part", "pdfaid:part is missing: the document declares neither PDF/A-3 nor PDF/A-4")
	default:
		k.add("PDF-04", "XMP pdfaid:part", "pdfaid:part is %q, not 3 or 4: PDF/A-%s does not allow embedded XML files", part, part)
	}

	conf := strings.TrimSpace(info.PDFAConformance)
	if part == "4" {
		switch conf {
		case "F", "E":
		case "":
			k.add("PDF-05", "XMP pdfaid:conformance", "pdfaid:conformance is missing: plain PDF/A-4 only allows embedded PDF/A files; an XML attachment needs PDF/A-4f (F) or PDF/A-4e (E)")
		default:
			k.add("PDF-05", "XMP pdfaid:conformance", "pdfaid:conformance is %q; the PDF/A-4 levels that allow an XML attachment are F and E", conf)
		}
		switch rev := strings.TrimSpace(info.PDFARev); {
		case rev == "":
			k.add("PDF-29", "XMP pdfaid:rev", "pdfaid:rev is missing: a PDF/A-4 file gives the year of the ISO 19005-4 edition it follows (2020)")
		case !isYear(rev):
			k.add("PDF-29", "XMP pdfaid:rev", "pdfaid:rev is %q, not a four-digit year", rev)
		}
	} else {
		switch conf {
		case "A", "B", "U":
		case "":
			k.add("PDF-05", "XMP pdfaid:conformance", "pdfaid:conformance is missing")
		default:
			k.add("PDF-05", "XMP pdfaid:conformance", "pdfaid:conformance is %q, not A, B or U", conf)
		}
	}

	v, where := c.version(), "header"
	if v != c.r.Version() {
		where = "catalog /Version"
	}
	switch {
	case part == "3" && !versionLess(v, "2.0"):
		k.add("PDF-30", where, "the PDF version is %s, but PDF/A-3 is based on PDF 1.7: a PDF 2.0 file can only be PDF/A-4", v)
	case part == "4" && versionLess(v, "2.0"):
		k.add("PDF-30", where, "the PDF version is %s, but PDF/A-4 is based on PDF 2.0", v)
	}
}

// isYear reports whether s is made of four digits.
func isYear(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// attachment checks the file specification and the embedded file stream.
func (k *containerCheck) attachment(c *container, fs *fileSpec, st *pdf.Stream, a *Attachment, profile invoice.Profile, zf1 bool, where countries) {
	path := specPath(fs)
	name := a.Filename
	want := "factur-x.xml"
	if profile == invoice.ProfileXRechnung {
		want = "xrechnung.xml"
	}
	switch {
	case name == want:
	case strings.EqualFold(name, "zugferd-invoice.xml"):
		k.add("PDF-11", path, "the attachment uses the legacy ZUGFeRD name %q; Factur-X readers expect %q", name, want)
	case nameRank(name) >= 0:
		k.add("PDF-11", path, "the attachment is named %q; profile %s expects %q", name, profileLabel(profile, zf1), want)
	default:
		k.add("PDF-10", path, "the invoice attachment is named %q, which is not an invoice file name (expected %q)", name, want)
	}

	switch rel := a.Relationship; rel {
	case "":
		k.add("PDF-12", path+" /AFRelationship", "the file specification has no /AFRelationship")
	case "Data", "Alternative", "Source":
		k.relationship(path+" /AFRelationship", rel, profile, zf1, where)
	default:
		k.add("PDF-13", path+" /AFRelationship", "/AFRelationship is %s, not Data, Alternative or Source", rel)
	}

	if !a.InAF {
		k.add("PDF-15", "catalog /AF", "the file specification of %q is not listed in the /AF array of the catalog", name)
	}

	switch mime := a.MimeType; mime {
	case "text/xml":
	case "application/xml":
		k.add("PDF-17", "embedded file /Subtype", "the MIME type is application/xml; Factur-X uses text/xml")
	case "":
		k.add("PDF-16", "embedded file /Subtype", "the embedded file stream has no /Subtype MIME type")
	default:
		k.add("PDF-16", "embedded file /Subtype", "the MIME type is %q, not text/xml", mime)
	}

	var modDate pdf.Object
	if st != nil {
		if params, ok := c.r.Dict(st.Dict.Get("Params")); ok {
			modDate = c.r.Resolve(params.Get("ModDate"))
		}
	}
	switch s, ok := modDate.(pdf.String); {
	case modDate == nil:
		k.add("PDF-18", "embedded file /Params /ModDate", "the embedded file stream has no /Params /ModDate")
	case !ok:
		k.add("PDF-19", "embedded file /Params /ModDate", "/ModDate is not a string")
	default:
		if !strictDate(pdf.Text(s)) {
			k.add("PDF-19", "embedded file /Params /ModDate", "/ModDate %q is not a valid PDF date", pdf.Text(s))
		}
	}

	var missing []string
	for _, key := range []pdf.Name{"F", "UF"} {
		if s, ok := c.r.Resolve(fs.dict.Get(key)).(pdf.String); !ok || len(s) == 0 {
			missing = append(missing, "/"+string(key))
		}
	}
	if len(missing) > 0 {
		k.add("PDF-20", path, "the file specification has no %s entry", strings.Join(missing, " or "))
	}
}

// relationship checks the /AFRelationship of the invoice against its profile
// and the countries whose rules apply, as Factur-X 1.09 (6.2.2) tabulates
// them: Data for MINIMUM and BASIC WL in France and Germany; for BASIC and
// above, Alternative in Germany, and Alternative, Source or Data in France.
// Without national rules, Data is reported for BASIC and above: the XML is
// a full invoice, which Alternative (or Source) describes.
func (k *containerCheck) relationship(path, rel string, profile invoice.Profile, zf1 bool, where countries) {
	switch {
	case profile == invoice.ProfileUnknown:
	case profile == invoice.ProfileMinimum || profile == invoice.ProfileBasicWL:
		if rel != "Data" {
			k.add("PDF-14", path, "/AFRelationship is %s; profile %s expects Data", rel, profile)
		}
	case rel == "Alternative":
	case where&inGermany != 0:
		k.add("PDF-40", path, "/AFRelationship is %s; Germany admits only Alternative for profile %s", rel, profileLabel(profile, zf1))
	case rel == "Data" && where&inFrance == 0:
		k.add("PDF-14", path, "/AFRelationship is Data; profile %s expects Alternative or Source (France also admits Data)", profileLabel(profile, zf1))
	}
}

// metadata checks the hybrid-invoice schema of the XMP metadata.
func (k *containerCheck) metadata(info *xmp.Info, a *Attachment, root string, profile invoice.Profile, zf1 bool) {
	inv := info.Invoice
	if inv == nil {
		k.add("PDF-21", "XMP", "the XMP metadata has no Factur-X / ZUGFeRD schema (namespace %s)", xmp.NSFacturX)
		return
	}
	prefix := "XMP " + invoicePrefix(inv.NamespaceURI) + ":"

	if t := strings.TrimSpace(inv.DocumentType); t != "INVOICE" {
		if t == "" {
			k.add("PDF-22", prefix+"DocumentType", "DocumentType is missing")
		} else {
			k.add("PDF-22", prefix+"DocumentType", "DocumentType is %q, not INVOICE", t)
		}
	}

	if n := strings.TrimSpace(inv.DocumentFileName); n != a.Filename {
		if n == "" {
			k.add("PDF-23", prefix+"DocumentFileName", "DocumentFileName is missing")
		} else {
			k.add("PDF-23", prefix+"DocumentFileName", "DocumentFileName is %q, the attachment is named %q", n, a.Filename)
		}
	}

	level := strings.TrimSpace(inv.ConformanceLevel)
	declared := invoice.ProfileFromConformanceLevel(level)
	switch {
	case level == "":
		k.add("PDF-24", prefix+"ConformanceLevel", "ConformanceLevel is missing")
	case declared == invoice.ProfileUnknown:
		k.add("PDF-24", prefix+"ConformanceLevel", "ConformanceLevel %q is not a known level", level)
	case profile != invoice.ProfileUnknown && sameLevel(declared) != sameLevel(profile) &&
		!(profile == invoice.ProfileXRechnung && declared == invoice.ProfileEN16931):
		k.add("PDF-24", prefix+"ConformanceLevel", "ConformanceLevel is %q, the XML is %s (BT-24)", level, profileLabel(profile, zf1))
	}

	if s := info.Schema(inv.NamespaceURI); s == nil {
		k.add("PDF-25", "XMP pdfaExtension:schemas", "no PDF/A extension schema describes the namespace %s", inv.NamespaceURI)
	} else {
		var undescribed []string
		for _, p := range []struct{ name, value string }{
			{"DocumentType", inv.DocumentType},
			{"DocumentFileName", inv.DocumentFileName},
			{"Version", inv.Version},
			{"ConformanceLevel", inv.ConformanceLevel},
		} {
			if p.value != "" && !contains(s.Properties, p.name) {
				undescribed = append(undescribed, p.name)
			}
		}
		if len(undescribed) > 0 {
			k.add("PDF-25", "XMP pdfaExtension:schemas", "the PDF/A extension schema of %s does not describe %s", inv.NamespaceURI, strings.Join(undescribed, ", "))
		}
	}

	if strings.TrimSpace(inv.Version) == "" {
		k.add("PDF-26", prefix+"Version", "Version is missing")
	}

	switch root {
	case "CrossIndustryInvoice":
		if inv.NamespaceURI != xmp.NSFacturX && inv.NamespaceURI != xmp.NSZUGFeRD2 {
			k.add("PDF-27", "XMP", "the XMP uses the ZUGFeRD 1.0 namespace %s, but the XML is a CrossIndustryInvoice (Factur-X / ZUGFeRD 2)", inv.NamespaceURI)
		}
	case "CrossIndustryDocument":
		if inv.NamespaceURI != xmp.NSZUGFeRD1 && inv.NamespaceURI != xmp.NSZUGFeRD1RC {
			k.add("PDF-27", "XMP", "the XMP uses the namespace %s, but the XML is a ZUGFeRD 1.0 CrossIndustryDocument", inv.NamespaceURI)
		}
	}
}

// xmlProfile returns the profile of an invoice XML from its root element
// and BT-24. ZUGFeRD 1.0 identifiers (urn:ferd:CrossIndustryDocument:
// invoice:1p0:basic, comfort, extended) map to BASIC, EN 16931 and EXTENDED.
func xmlProfile(root, id string) (p invoice.Profile, zf1 bool) {
	if root == "CrossIndustryDocument" || strings.HasPrefix(strings.TrimSpace(id), "urn:ferd:") {
		id = strings.ToLower(strings.TrimSpace(id))
		switch {
		case strings.HasSuffix(id, ":basic"):
			return invoice.ProfileBasic, true
		case strings.HasSuffix(id, ":comfort"):
			return invoice.ProfileEN16931, true
		case strings.HasSuffix(id, ":extended"):
			return invoice.ProfileExtended, true
		}
		return invoice.ProfileUnknown, true
	}
	return invoice.ProfileFromURN(id), false
}

// sameLevel maps EXTENDED-CTC-FR to EXTENDED, PEPPOL, NLCIUS, CIUS-IT and
// CIUS-PT to EN 16931: the Factur-X specification defines no XMP ConformanceLevel
// of their own.
func sameLevel(p invoice.Profile) invoice.Profile {
	switch p {
	case invoice.ProfileExtendedCTCFR:
		return invoice.ProfileExtended
	case invoice.ProfilePeppol, invoice.ProfileNLCIUS, invoice.ProfileCIUSIT, invoice.ProfileCIUSPT:
		return invoice.ProfileEN16931
	}
	return p
}

// profileLabel names a profile in messages; ZUGFeRD 1.0 COMFORT is shown as
// such.
func profileLabel(p invoice.Profile, zf1 bool) string {
	if zf1 {
		switch p {
		case invoice.ProfileEN16931:
			return "ZUGFeRD 1.0 COMFORT"
		case invoice.ProfileBasic, invoice.ProfileExtended:
			return "ZUGFeRD 1.0 " + p.String()
		}
	}
	return p.String()
}

// invoicePrefix returns the conventional XMP prefix of a hybrid-invoice
// namespace.
func invoicePrefix(ns string) string {
	switch ns {
	case xmp.NSZUGFeRD2, xmp.NSZUGFeRD1, xmp.NSZUGFeRD1RC:
		return "zf"
	}
	return "fx"
}

// specPath locates a file specification in findings.
func specPath(fs *fileSpec) string {
	if fs == nil {
		return "file specification"
	}
	if fs.ref != (pdf.Ref{}) {
		return "file specification " + fs.ref.String()
	}
	return "file specification"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// strictDate reports whether s is a PDF date as ISO 32000 defines it:
// "D:YYYY" optionally followed by MM, DD, HH, mm, SS and a time zone (Z, or
// +HH'mm' / -HH'mm', the apostrophes being optional). Unlike
// pdf.ParseDate, which accepts what writers produce in practice, it rejects
// missing prefixes, out-of-range fields and trailing garbage.
func strictDate(s string) bool {
	s, ok := strings.CutPrefix(s, "D:")
	if !ok || len(s) < 4 {
		return false
	}
	num := func(n int, lo, hi int) bool {
		if len(s) < n {
			return false
		}
		v := 0
		for i := 0; i < n; i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
			v = v*10 + int(s[i]-'0')
		}
		s = s[n:]
		return v >= lo && v <= hi
	}
	if !num(4, 0, 9999) {
		return false
	}
	for _, f := range [][2]int{{1, 12}, {1, 31}, {0, 23}, {0, 59}, {0, 59}} {
		if len(s) == 0 || s[0] < '0' || s[0] > '9' {
			break
		}
		if !num(2, f[0], f[1]) {
			return false
		}
	}
	if len(s) == 0 {
		return true
	}
	switch s[0] {
	case 'Z':
		s = s[1:]
		// Some writers append 00'00' after Z.
		s = strings.TrimPrefix(s, "00'00'")
		s = strings.TrimPrefix(s, "00'00")
		return s == ""
	case '+', '-':
		s = s[1:]
		if !num(2, 0, 23) {
			return false
		}
		if len(s) == 0 {
			return true
		}
		s = strings.TrimPrefix(s, "'")
		if len(s) == 0 {
			return true
		}
		if !num(2, 0, 59) {
			return false
		}
		s = strings.TrimPrefix(s, "'")
		return s == ""
	}
	return false
}
