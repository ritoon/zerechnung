package render

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ritoon/zerechnung/internal/icc"
	"github.com/ritoon/zerechnung/internal/xmp"
)

// producer is written in /Producer and pdf:Producer.
const producer = "ZeRechnung server"

// pdfWriter writes the objects of a PDF file and its cross-reference table.
type pdfWriter struct {
	buf     bytes.Buffer
	offsets []int // by object number, from 1
}

// alloc reserves an object number.
func (w *pdfWriter) alloc() int {
	w.offsets = append(w.offsets, -1)
	return len(w.offsets)
}

// obj writes object num with the body given.
func (w *pdfWriter) obj(num int, body string) {
	w.offsets[num-1] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n%s\nendobj\n", num, body)
}

// stream writes a stream object; dict holds the entries besides /Length and
// /Filter. The data is compressed with Flate when compress is set.
func (w *pdfWriter) stream(num int, dict string, data []byte, compress bool) {
	filter := ""
	if compress {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		_, _ = zw.Write(data) // writing to a bytes.Buffer does not fail
		zw.Close()
		data = z.Bytes()
		filter = " /Filter /FlateDecode"
	}
	w.offsets[num-1] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n<< %s /Length %d%s >>\nstream\n", num, dict, len(data), filter)
	w.buf.Write(data)
	w.buf.WriteString("\nendstream\nendobj\n")
}

// finish writes the cross-reference table and the trailer. The file
// identifier is a digest of the file, so that the same document gives the
// same bytes.
func (w *pdfWriter) finish(root, info int) []byte {
	sum := md5.Sum(w.buf.Bytes())
	id := fmt.Sprintf("<%X>", sum[:])
	xref := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", len(w.offsets)+1)
	for _, off := range w.offsets {
		fmt.Fprintf(&w.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&w.buf, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R /ID [%s %s] >>\nstartxref\n%d\n%%%%EOF\n",
		len(w.offsets)+1, root, info, id, id, xref)
	return w.buf.Bytes()
}

// pdfText encodes s as a PDF text string: PDFDocEncoding when s is ASCII,
// UTF-16BE with a byte order mark otherwise.
func pdfText(s string) string {
	ascii := true
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
		return "(" + r.Replace(s) + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, c := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", c)
	}
	b.WriteString(">")
	return b.String()
}

// pdfDate formats t as a PDF date: D:YYYYMMDDHHmmSS and the offset to UTC.
func pdfDate(t time.Time) string {
	s := "D:" + t.Format("20060102150405")
	_, off := t.Zone()
	if off == 0 {
		return s + "Z"
	}
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	return fmt.Sprintf("%s%s%02d'%02d'", s, sign, off/3600, off%3600/60)
}

// num formats a coordinate for a content stream.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// document writes the PDF/A-3b file of the pages drawn on c.
func (c *canvas) document(title, lang string, t time.Time) []byte {
	t = t.Truncate(time.Second)
	w := &pdfWriter{}
	w.buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	catalog, pages, info, meta, intent := w.alloc(), w.alloc(), w.alloc(), w.alloc(), w.alloc()

	fontRefs := map[*usedFont]int{}
	for _, f := range []*usedFont{c.reg, c.bold} {
		if len(f.toUni) == 0 {
			continue
		}
		fontRefs[f] = w.writeFont(f)
	}
	var fontRes strings.Builder
	for _, f := range []*usedFont{c.reg, c.bold} {
		if ref, ok := fontRefs[f]; ok {
			fmt.Fprintf(&fontRes, " /%s %d 0 R", f.res, ref)
		}
	}

	var kids []string
	for _, p := range c.pages {
		page, content := w.alloc(), w.alloc()
		w.stream(content, "", p.Bytes(), true)
		w.obj(page, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << /Font <<%s >> >> /Contents %d 0 R >>",
			pages, num(pageW), num(pageH), fontRes.String(), content))
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
	}
	w.obj(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))

	profile := icc.SRGB()
	iccRef := w.alloc()
	w.stream(iccRef, "/N 3", profile, true)
	w.obj(intent, fmt.Sprintf("<< /Type /OutputIntent /S /GTS_PDFA1 /OutputConditionIdentifier %s /Info %s /DestOutputProfile %d 0 R >>",
		pdfText(icc.Description), pdfText(icc.Description), iccRef))

	packet := xmp.Merge(nil, xmp.Params{
		PDFAOnly:        true,
		PDFAPart:        "3",
		PDFAConformance: "B",
		Date:            t,
		DocInfo: xmp.DocInfo{
			Title:        title,
			Creator:      producer,
			Producer:     producer,
			CreationDate: t,
		},
	})
	w.stream(meta, "/Type /Metadata /Subtype /XML", packet, false)

	date := pdfText(pdfDate(t))
	w.obj(info, fmt.Sprintf("<< /Title %s /Creator %s /Producer %s /CreationDate %s /ModDate %s >>",
		pdfText(title), pdfText(producer), pdfText(producer), date, date))
	w.obj(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Metadata %d 0 R /OutputIntents [%d 0 R] /Lang %s /ViewerPreferences << /DisplayDocTitle true >> >>",
		pages, meta, intent, pdfText(lang)))
	return w.finish(catalog, info)
}

// writeFont writes a Type 0 font with its CIDFont, font descriptor, subset
// font program and ToUnicode CMap, and returns the number of the Type 0
// font dictionary.
func (w *pdfWriter) writeFont(f *usedFont) int {
	type0, cid, desc, file, toUni := w.alloc(), w.alloc(), w.alloc(), w.alloc(), w.alloc()
	base := f.baseFont()
	program := f.subset()
	w.stream(file, fmt.Sprintf("/Length1 %d", len(program)), program, true)
	flags := 4 // symbolic: the glyphs are selected by CID, not by a standard encoding
	w.obj(desc, fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%s %s %s %s] /ItalicAngle %s /Ascent %s /Descent %s /CapHeight %s /StemV %d /FontFile2 %d 0 R >>",
		base, flags, num(f.bbox[0]), num(f.bbox[1]), num(f.bbox[2]), num(f.bbox[3]), num(f.italic),
		num(f.ascent), num(f.descent), num(f.capH), f.stemV, file))
	w.obj(cid, fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /DW 1000 /W %s /CIDToGIDMap /Identity >>",
		base, desc, f.widths()))
	w.stream(toUni, "", f.toUnicode(), true)
	w.obj(type0, fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
		base, cid, toUni))
	return type0
}
