package zerechnung

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/invoice"
)

// TestCheckContainerContent covers the checks of the content streams
// (check_pdfa_content.go), of the JPEG 2000 data of the images and of the
// newest trailer. The fixtures have an sRGB output intent.
func TestCheckContainerContent(t *testing.T) {
	en := ciiXML(invoice.URNEN16931)
	mod := func(fn func(f *fixture)) *fixture {
		f := valid(en)
		fn(f)
		return f
	}
	extra := func(fn func(b *pdftest.Builder) (catalog, page string)) *fixture {
		return mod(func(f *fixture) { f.extra = fn })
	}
	// content returns a fixture whose page has the content data and the
	// resources res, built with b.
	content := func(data string, res func(b *pdftest.Builder) string) *fixture {
		return extra(func(b *pdftest.Builder) (string, string) {
			r := ""
			if res != nil {
				r = res(b)
			}
			return "", fmt.Sprintf("/Contents %d 0 R /Resources << %s >>", b.AddStream("<< >>", []byte(data)), r)
		})
	}
	resources := func(r string) func(*pdftest.Builder) string {
		return func(*pdftest.Builder) string { return r }
	}
	iccCMYK := func(b *pdftest.Builder) int {
		return b.AddStream("<< /N 4 >>", pdftest.ICC("prtr", "CMYK", 2))
	}
	type3 := func(glyph string) func(b *pdftest.Builder) string {
		return func(b *pdftest.Builder) string {
			proc := b.AddStream("<< >>", []byte(glyph))
			font := b.Add(fmt.Sprintf("<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] "+
				"/CharProcs << /a %d 0 R >> /Encoding << /Type /Encoding /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [10] /Resources << >> >>", proc))
			return fmt.Sprintf("/Font << /F1 %d 0 R >>", font)
		}
	}
	pattern := func(paintType int) func(b *pdftest.Builder) string {
		return func(b *pdftest.Builder) string {
			p := b.AddStream(fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType %d /TilingType 1 /BBox [0 0 1 1] /XStep 1 /YStep 1 /Resources << >> >>", paintType),
				[]byte("0 0 0 1 k 0 0 1 1 re f"))
			return fmt.Sprintf("/Pattern << /P0 %d 0 R >> /ColorSpace << /CS0 [/Pattern /DeviceRGB] >>", p)
		}
	}
	image := func(dict string, data []byte) *fixture {
		return extra(func(b *pdftest.Builder) (string, string) {
			im := b.AddStream("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /Filter /JPXDecode "+dict+" >>", data)
			return "", fmt.Sprintf("/Resources << /XObject << /Im1 %d 0 R >> >>", im)
		})
	}
	transparency := func(page string) *fixture {
		f := content("/GS0 gs 0 0 10 10 re f", resources("/ExtGState << /GS0 << /ca 0.5 >> >>"))
		f.noOutputIntent = true
		if page != "" {
			inner := f.extra
			f.extra = func(b *pdftest.Builder) (string, string) {
				c, p := inner(b)
				return c, p + " " + page
			}
		}
		return f
	}
	for _, tc := range []struct {
		name  string
		f     *fixture
		rules string
	}{
		{"content", content("q 1 0 0 rg 0.5 g 0 0 10 10 re f BT (a) Tj ET Q", nil), ""},

		// PDF-71: operators and resources (6.2.2).
		{"undefined operator", content("0 0 10 10 re foo f", nil), "PDF-71"},
		{"undefined operator between BX and EX", content("BX foo EX", nil), "PDF-71"},
		{"resource taken from the page by a form", content("/X0 Do", func(b *pdftest.Builder) string {
			x := b.AddStream("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] >>", []byte("/GS0 gs"))
			return fmt.Sprintf("/XObject << /X0 %d 0 R >> /ExtGState << /GS0 << /CA 1 >> >>", x)
		}), "PDF-71"},
		{"form with its own resources", content("/X0 Do", func(b *pdftest.Builder) string {
			x := b.AddStream("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Resources << /ExtGState << /GS0 << /CA 1 >> >> >> >>", []byte("/GS0 gs"))
			return fmt.Sprintf("/XObject << /X0 %d 0 R >>", x)
		}), ""},
		{"resources of the page tree", mod(func(f *fixture) {
			f.pagesExtra = " /Resources << /ExtGState << /GS0 << /CA 1 >> >> >>"
			f.extra = func(b *pdftest.Builder) (string, string) {
				return "", fmt.Sprintf("/Contents %d 0 R", b.AddStream("<< >>", []byte("/GS0 gs")))
			}
		}), "PDF-71"},

		// PDF-44 and PDF-49: operands and nesting (6.1.6, 6.1.13).
		{"odd hexadecimal string", content("BT <414> Tj ET", nil), "PDF-44"},
		{"integer out of range", content("0 2157483648 m", nil), "PDF-49"},
		{"q operators nested 29 deep", content(strings.Repeat("q ", 29)+strings.Repeat("Q ", 29), nil), "PDF-49"},
		{"q operators nested 28 deep", content(strings.Repeat("q ", 28)+strings.Repeat("Q ", 28), nil), ""},

		// Inline images (6.1.10, 6.2.8, 6.2.6, 6.2.4.3).
		{"inline image with the LZW filter", content("BI /W 1 /H 1 /BPC 8 /CS /G /F /LZW ID x EI", nil), "PDF-38"},
		{"inline image with Interpolate", content("BI /W 1 /H 1 /BPC 8 /CS /G /I true ID x EI", nil), "PDF-56"},
		{"inline image of 3 bits per component", content("BI /W 1 /H 1 /BPC 3 /CS /G ID x EI", nil), "PDF-56"},
		{"inline image with an unknown rendering intent", content("BI /W 1 /H 1 /BPC 8 /CS /G /Intent /Custom ID x EI", nil), "PDF-55"},
		{"inline image in DeviceCMYK", content("BI /W 1 /H 1 /BPC 8 /CS /CMYK ID xxxx EI", nil), "PDF-52"},

		// PDF-52: device colour spaces (6.2.4.3).
		{"DeviceCMYK colour operator", content("0 0 0 1 k 0 0 10 10 re f", nil), "PDF-52"},
		{"DeviceCMYK under DefaultCMYK", content("0 0 0 1 k 0 0 10 10 re f", func(b *pdftest.Builder) string {
			return fmt.Sprintf("/ColorSpace << /DefaultCMYK [/ICCBased %d 0 R] >>", iccCMYK(b))
		}), ""},
		{"Type 3 glyph in DeviceCMYK", content("BT /F1 1 Tf (a) Tj ET", type3("10 0 d0 0 0 0 1 k 0 0 10 10 re f")), "PDF-52"},
		{"Type 3 glyph in DeviceCMYK after d1", content("BT /F1 1 Tf (a) Tj ET", type3("10 0 0 0 10 10 d1 0 0 0 1 k 0 0 10 10 re f")), ""},
		{"coloured tiling pattern in DeviceCMYK", content("/CS0 cs 1 0 0 /P0 scn 0 0 10 10 re f", pattern(1)), "PDF-52"},
		{"uncoloured tiling pattern", content("/CS0 cs 1 0 0 /P0 scn 0 0 10 10 re f", pattern(2)), ""},

		// PDF-51 and PDF-55: overprint and rendering intent (6.2.4.2, 6.2.6).
		{"overprint mode 1 in an ICCBased CMYK colour space", content("/CS0 cs 0 0 0 1 sc /GS0 gs 0 0 10 10 re f", func(b *pdftest.Builder) string {
			return fmt.Sprintf("/ColorSpace << /CS0 [/ICCBased %d 0 R] >> /ExtGState << /GS0 << /OP true /OPM 1 >> >>", iccCMYK(b))
		}), "PDF-51"},
		{"overprint mode 0 in an ICCBased CMYK colour space", content("/CS0 cs 0 0 0 1 sc /GS0 gs 0 0 10 10 re f", func(b *pdftest.Builder) string {
			return fmt.Sprintf("/ColorSpace << /CS0 [/ICCBased %d 0 R] >> /ExtGState << /GS0 << /OP true /OPM 0 >> >>", iccCMYK(b))
		}), ""},
		{"ri operator with an unknown intent", content("/Custom ri", nil), "PDF-55"},

		// PDF-58: transparency without a PDF/A output intent (6.2.10).
		{"transparency without output intent", transparency(""), "PDF-06,PDF-52,PDF-58"},
		{"transparency with a page group colour space", transparency("/Group << /S /Transparency /CS /DeviceGray >>"), "PDF-06,PDF-52,PDF-52"},

		// PDF-56: JPEG 2000 data (6.2.8.3).
		{"JPEG 2000 image", image("", jp2(3, 8, false, colr(1, 0, 16))), ""},
		{"JPEG 2000 image of 2 channels", image("", jp2(2, 8, false, colr(1, 0, 16))), "PDF-56"},
		{"JPEG 2000 image of varying bit depths", image("", jp2(3, 0, true, colr(1, 0, 16))), "PDF-56"},
		{"JPEG 2000 image of 40 bits", image("", jp2(1, 40, false, colr(1, 0, 17))), "PDF-56"},
		{"JPEG 2000 image of colour specification method 4", image("", jp2(3, 8, false, colr(4, 0, 0))), "PDF-56"},
		{"JPEG 2000 image of colour specification method 4 with a /ColorSpace", image("/ColorSpace /DeviceRGB", jp2(3, 8, false, colr(4, 0, 0))), ""},
		{"JPEG 2000 image in CIEJab", image("", jp2(3, 8, false, colr(1, 0, 19))), "PDF-56"},
		{"JPEG 2000 image of two colour specifications", image("", jp2(3, 8, false, colr(1, 0, 16), colr(1, 0, 17))), "PDF-56"},
		{"JPEG 2000 image of two colour specifications, one approximate", image("", jp2(3, 8, false, colr(1, 1, 16), colr(1, 0, 17))), ""},
		{"JPEG 2000 codestream", image("", []byte{0xff, 0x4f, 0xff, 0x51}), "PDF-56"},

		// PDF-08: the newest trailer (6.1.3).
		{"incremental update without /ID", mod(func(f *fixture) {
			f.post = func(b []byte) []byte {
				r, err := pdf.Open(b)
				if err != nil {
					panic(err)
				}
				// The update rewrites the document information dictionary.
				info := r.Trailer().Get("Info")
				u := pdf.NewUpdate(r)
				u.Put(info.(pdf.Ref), r.Resolve(info))
				return u.Bytes(pdf.Dict{{Key: "Root", Val: r.Trailer().Get("Root")}, {Key: "Info", Val: info}})
			}
		}), "PDF-08"},
	} {
		t.Run(tc.name, func(t *testing.T) { checkContainerRules(t, tc.f, tc.rules) })
	}
}

// jp2 returns JPEG 2000 data in the JP2 file format: a header box of nc
// components of bpc (the stored value) bits, a bits per component box when
// bpcc is set, and the colour specification boxes colrs.
func jp2(nc uint16, bpc byte, bpcc bool, colrs ...[]byte) []byte {
	box := func(typ string, content []byte) []byte {
		b := binary.BigEndian.AppendUint32(nil, uint32(8+len(content)))
		return append(append(b, typ...), content...)
	}
	ihdr := binary.BigEndian.AppendUint32(nil, 1) // height
	ihdr = binary.BigEndian.AppendUint32(ihdr, 1) // width
	ihdr = binary.BigEndian.AppendUint16(ihdr, nc)
	ihdr = append(ihdr, bpc-1, 7, 0, 0) // BPC, C, UnkC, IPR
	header := box("ihdr", ihdr)
	if bpcc {
		header = append(header, box("bpcc", []byte{7, 7, 15})...)
	}
	for _, c := range colrs {
		header = append(header, box("colr", c)...)
	}
	out := append([]byte(nil), jpxSignature...)
	out = append(out, box("ftyp", []byte("jp2 \x00\x00\x00\x00jp2 "))...)
	out = append(out, box("jp2h", header)...)
	return append(out, box("jp2c", []byte{0xff, 0x4f, 0xff, 0xd9})...)
}

// colr returns the content of a colour specification box.
func colr(meth, approx byte, enum uint32) []byte {
	b := []byte{meth, 0, approx}
	if meth == 1 {
		b = binary.BigEndian.AppendUint32(b, enum)
	}
	return b
}

func TestParseCMap(t *testing.T) {
	for _, tc := range []struct {
		cmap string
		want uint32
	}{
		{"1 begincidrange <0000> <ffff> 0 endcidrange", 65535},
		{"2 begincidrange <0000> <00ff> 0 <0100> <01ff> 65500 endcidrange", 65755},
		{"1 begincidchar <41> 70000 endcidchar", 70000},
		{"1 beginnotdefrange <00> <1f> 1 endnotdefrange 1 begincidchar <20> 3 endcidchar", 3},
		{"/CMapName /X def", 0},
	} {
		if got := parseCMap([]byte(tc.cmap)).maxCID(); got != tc.want {
			t.Errorf("%q: %d, want %d", tc.cmap, got, tc.want)
		}
	}

	// Codes of one byte from 00 to 80, of two bytes from 8140 to 9FFC.
	m := parseCMap([]byte(`2 begincodespacerange <00> <80> <8140> <9ffc> endcodespacerange
		1 begincidrange <8140> <817e> 633 endcidrange
		1 begincidchar <41> 34 endcidchar
		1 beginnotdefrange <00> <1f> 1 endnotdefrange`))
	type code struct {
		code uint32
		size int
		cid  uint32
		ok   bool
	}
	var got []code
	for s := []byte("\x41\x81\x42\x05\x9f\x20\xa0\x81"); len(s) > 0; {
		c, n := m.next(s)
		cid, ok := m.cid(c)
		got = append(got, code{c, n, cid, ok})
		s = s[min(n, len(s)):]
	}
	// 9F 20 lies in no range, A0 starts none, and the last byte, 81, reads
	// as 81FF, in no range: all three read as the code 0.
	want := []code{{0x41, 1, 34, true}, {0x8142, 2, 635, true}, {0x05, 1, 1, false}, {0, 2, 1, false}, {0, 1, 1, false}, {0, 2, 1, false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("codes %v, want %v", got, want)
	}
	if c, n := identityCMap.next([]byte{0x12}); c != 0x12ff || n != 2 {
		t.Errorf("identity, one byte: %x %d", c, n)
	}
}
