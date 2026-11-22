package zerechnung

import (
	"fmt"
	"testing"

	"github.com/ritoon/zerechnung/internal/font/fonttest"
	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/invoice"
)

// TestCheckContainerGlyphs covers the checks of the glyphs the text shows
// and of the CharSet and CIDSet of subset fonts (check_pdfa_glyphs.go),
// with the font programs of internal/font/fonttest: a TrueType program
// whose (3,1) cmap maps A and B to the glyphs 1 and 2 (1000 units per em,
// advances 500, 600, 600), and a CFF program of the glyphs .notdef (500), A
// (650) and A.sc (600).
func TestCheckContainerGlyphs(t *testing.T) {
	for _, tc := range glyphCases() {
		t.Run(tc.name, func(t *testing.T) { checkContainerRules(t, tc.f, tc.rules) })
	}
}

// glyphCase is a PDF of TestCheckContainerGlyphs and the rules it breaks.
type glyphCase struct {
	name  string
	f     *fixture
	rules string
}

// glyphCases returns the PDFs of TestCheckContainerGlyphs. Those of a kind
// of font embed the same program, byte for byte, valid or not: the tests
// of Checker share them.
func glyphCases() []glyphCase {
	en := ciiXML(invoice.URNEN16931)
	// page returns a fixture whose page shows text with the font F1, which
	// font adds with b.
	page := func(text string, font func(b *pdftest.Builder) int) *fixture {
		f := valid(en)
		f.extra = func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >>",
				b.AddStream("<< >>", []byte("BT /F1 12 Tf "+text+" ET")), font(b))
		}
		return f
	}
	descriptor := func(b *pdftest.Builder, name, entries string) string {
		return fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 "+
			"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 %s >>", name, entries)
	}
	trueType := func(widths, encoding string) func(b *pdftest.Builder) int {
		return func(b *pdftest.Builder) int {
			prog := b.AddStream("<< >>", fonttest.TrueType())
			return b.Add(fmt.Sprintf("<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Test /FirstChar 65 /LastChar 66 "+
				"/Widths %s /Encoding %s /FontDescriptor %s >>", widths, encoding, descriptor(b, "ABCDEF+Test", fmt.Sprintf("/FontFile2 %d 0 R", prog))))
		}
	}
	cff := func(base, charSet string) func(b *pdftest.Builder) int {
		return func(b *pdftest.Builder) int {
			prog := b.AddStream("<< /Subtype /Type1C >>", fonttest.CFF())
			fd := fmt.Sprintf("/FontFile3 %d 0 R", prog)
			if charSet != "" {
				fd += " /CharSet (" + charSet + ")"
			}
			return b.Add(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /FirstChar 65 /LastChar 66 /Widths [650 600] "+
				"/Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [66 /A.sc] >> /FontDescriptor %s >>",
				base, descriptor(b, base, fd)))
		}
	}
	cid := func(w string, cidSet []byte) func(b *pdftest.Builder) int {
		return func(b *pdftest.Builder) int {
			prog := b.AddStream("<< >>", fonttest.TrueType())
			fd := fmt.Sprintf("/FontFile2 %d 0 R", prog)
			if cidSet != nil {
				fd += fmt.Sprintf(" /CIDSet %d 0 R", b.AddStream("<< >>", cidSet))
			}
			desc := b.Add(fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Test "+
				"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /CIDToGIDMap /Identity /DW 600 /W %s /FontDescriptor %s >>",
				w, descriptor(b, "ABCDEF+Test", fd)))
			return b.Add(fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Test /Encoding /Identity-H /DescendantFonts [%d 0 R] >>", desc))
		}
	}
	type3 := func(differences, glyph string) func(b *pdftest.Builder) int {
		return func(b *pdftest.Builder) int {
			proc := b.AddStream("<< >>", []byte(glyph+" 0 0 10 10 re f"))
			return b.Add(fmt.Sprintf("<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] "+
				"/CharProcs << /a %d 0 R /.notdef %d 0 R >> /Encoding << /Type /Encoding /Differences %s >> /FirstChar 97 /LastChar 97 /Widths [10] /Resources << >> >>",
				proc, proc, differences))
		}
	}
	return []glyphCase{
		// TrueType: codes looked up by their glyph name in the (3,1) cmap.
		{"TrueType glyphs", page("(AB) Tj", trueType("[600 600]", "/WinAnsiEncoding")), ""},
		{"TrueType glyph missing", page("(C) Tj", trueType("[600 600]", "/WinAnsiEncoding")), "PDF-72"},
		{"TrueType .notdef", page("<00> Tj", trueType("[600 600]", "/WinAnsiEncoding")), "PDF-72"},
		{"TrueType width", page("[(A) 10 (B)] TJ", trueType("[700 600]", "/WinAnsiEncoding")), "PDF-72"},
		{"TrueType width within one unit", page("(A) Tj", trueType("[601 600]", "/WinAnsiEncoding")), ""},
		{"invisible text, glyph missing", page("3 Tr (C) Tj", trueType("[600 600]", "/WinAnsiEncoding")), ""},
		{"invisible text, .notdef", page("3 Tr <00> Tj", trueType("[600 600]", "/WinAnsiEncoding")), "PDF-72"},
		{"text of the \" operator", page("0 0 (C) \"", trueType("[600 600]", "/WinAnsiEncoding")), ""},
		{"Differences in the Adobe Glyph List", page("(A) Tj",
			trueType("[600 600]", "<< /BaseEncoding /WinAnsiEncoding /Differences [66 /B] >>")), ""},
		{"Differences not in the Adobe Glyph List", page("(A) Tj",
			trueType("[600 600]", "<< /BaseEncoding /WinAnsiEncoding /Differences [66 /B.alt2] >>")), "PDF-60"},
		// CFF: codes looked up by glyph name in the charset.
		{"CFF glyphs", page("(AB) Tj", cff("ABCDEF+TestCFF", "")), ""},
		{"CFF glyph missing", page("(C) Tj", cff("ABCDEF+TestCFF", "")), "PDF-72"},
		{"CharSet of the glyphs", page("(A) Tj", cff("ABCDEF+TestCFF", "/A/A.sc")), ""},
		{"CharSet without a glyph", page("(A) Tj", cff("ABCDEF+TestCFF", "/A")), "PDF-72"},
		{"CharSet with a glyph too many", page("(A) Tj", cff("ABCDEF+TestCFF", "/A/A.sc/B")), "PDF-72"},
		{"CharSet of a font not subset", page("(A) Tj", cff("TestCFF", "/A")), ""},
		// CIDFontType2, Identity-H: CIDs and glyphs are the codes.
		{"CID glyphs", page("<00010002> Tj", cid("[1 [600 600]]", nil)), ""},
		{"CID 0", page("<0000> Tj", cid("[1 [600 600]]", nil)), "PDF-72"},
		// A missing glyph of a CIDFont is also named .notdef.
		{"CID glyph missing", page("<0005> Tj", cid("[1 [600 600]]", nil)), "PDF-72,PDF-72"},
		// The last byte reads as 00FF.
		{"odd byte of a two-byte code", page("<000100> Tj", cid("[1 [600 600]]", nil)), "PDF-72,PDF-72"},
		{"CID width", page("<0001> Tj", cid("[1 [700]]", nil)), "PDF-72"},
		{"CID default width", page("<0002> Tj", cid("[1 [600]]", nil)), ""},
		{"CIDSet of the glyphs", page("<0001> Tj", cid("[1 [600 600]]", []byte{0x60})), ""},
		{"CIDSet without a glyph", page("<0001> Tj", cid("[1 [600 600]]", []byte{0x20})), "PDF-72"},
		{"CIDSet with a glyph too many", page("<0001> Tj", cid("[1 [600 600]]", []byte{0x70})), "PDF-72"},
		// Type 3: glyph descriptions by name, widths in glyph space.
		{"Type 3 glyph", page("(a) Tj", type3("[97 /a]", "10 0 d0")), ""},
		{"Type 3 glyph missing", page("(b) Tj", type3("[97 /a]", "10 0 d0")), "PDF-72"},
		{"Type 3 width", page("(a) Tj", type3("[97 /a]", "20 0 d0")), "PDF-72"},
		{"Type 3 .notdef", page("(a) Tj", type3("[97 /.notdef]", "10 0 d0")), "PDF-72"},
	}
}
