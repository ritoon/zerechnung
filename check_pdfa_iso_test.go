package zerechnung

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/internal/xmp"
	"github.com/ritoon/zerechnung/invoice"
)

// TestCheckContainerISO19005 covers the PDF/A-2 and PDF/A-3 rules (PDF-41 to
// PDF-69, and the filters of PDF-38): each case alters the valid fixture to
// break one requirement, or writes a close construct that PDF/A admits.
// TestVeraPDFCorpus runs the same rules on the veraPDF test files.
func TestCheckContainerISO19005(t *testing.T) {
	en := ciiXML(invoice.URNEN16931)
	mod := func(fn func(f *fixture)) *fixture {
		f := valid(en)
		fn(f)
		return f
	}
	extra := func(fn func(b *pdftest.Builder) (catalog, page string)) *fixture {
		return mod(func(f *fixture) { f.extra = fn })
	}
	catalog := func(entries string) *fixture {
		return extra(func(*pdftest.Builder) (string, string) { return entries, "" })
	}
	page := func(entries string) *fixture {
		return extra(func(*pdftest.Builder) (string, string) { return "", entries })
	}
	// replace returns a valid fixture whose bytes have the first occurrence
	// of old replaced by new, of the same length: the offsets stay right.
	replace := func(old, new string) *fixture {
		if len(old) != len(new) {
			panic("replace: " + old + " and " + new + " differ in length")
		}
		return mod(func(f *fixture) {
			f.post = func(b []byte) []byte {
				i := bytes.Index(b, []byte(old))
				if i < 0 {
					panic("replace: " + old + " not found")
				}
				copy(b[i:], new)
				return b
			}
		})
	}
	// resources returns a valid fixture whose page has the resource entries
	// written by fn.
	resources := func(fn func(b *pdftest.Builder) string) *fixture {
		return extra(func(b *pdftest.Builder) (string, string) { return "", "/Resources << " + fn(b) + " >>" })
	}
	res := func(entries string) *fixture {
		return resources(func(*pdftest.Builder) string { return entries })
	}
	profile := func(b *pdftest.Builder, class, space string, version byte, n int) int {
		return b.AddStream(fmt.Sprintf("<< /N %d >>", n), pdftest.ICC(class, space, version))
	}
	// intents returns a valid fixture whose output intents are written by
	// fn instead of the sRGB one.
	intents := func(fn func(b *pdftest.Builder) string) *fixture {
		return mod(func(f *fixture) {
			f.noOutputIntent = true
			f.extra = func(b *pdftest.Builder) (string, string) { return "/OutputIntents [" + fn(b) + "]", "" }
		})
	}
	intent := func(s string, profile int) string {
		return fmt.Sprintf("<< /Type /OutputIntent /S /%s /OutputConditionIdentifier (sRGB) /DestOutputProfile %d 0 R >>", s, profile)
	}
	iccBased := func(class, space string, version byte, n int) *fixture {
		return resources(func(b *pdftest.Builder) string {
			return fmt.Sprintf("/ColorSpace << /CS0 [/ICCBased %d 0 R] >>", profile(b, class, space, version, n))
		})
	}
	tint := "<< /FunctionType 2 /Domain [0 1] /C0 [1 1 1] /C1 [1 0 0] /N 1 >>"
	gs := func(entries string) *fixture {
		return res("/ExtGState << /GS0 << /Type /ExtGState " + entries + " >> >>")
	}
	xobject := func(dict string, data []byte) *fixture {
		return resources(func(b *pdftest.Builder) string {
			return fmt.Sprintf("/XObject << /X1 %d 0 R >>", b.AddStream(dict, data))
		})
	}
	image := func(entries string) *fixture {
		return xobject("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 "+entries+" >>", []byte{0, 0, 0})
	}
	rgb := "/ColorSpace /DeviceRGB /BitsPerComponent 8 "
	descriptor := func(b *pdftest.Builder, flags int, key, dict string) int {
		return b.Add(fmt.Sprintf("<< /Type /FontDescriptor /FontName /ABCDEF+Font /Flags %d /%s %d 0 R >>", flags, key, b.AddStream(dict, []byte("font program"))))
	}
	// font returns a valid fixture whose page uses the font written by fn.
	font := func(fn func(b *pdftest.Builder) string) *fixture {
		return resources(func(b *pdftest.Builder) string { return fmt.Sprintf("/Font << /F1 %d 0 R >>", b.Add(fn(b))) })
	}
	trueType := func(flags int, entries string) *fixture {
		return font(func(b *pdftest.Builder) string {
			return fmt.Sprintf("<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Arial %s /FontDescriptor %d 0 R >>", entries, descriptor(b, flags, "FontFile2", "<< >>"))
		})
	}
	composite := func(cidFont, encoding string) *fixture {
		return font(func(b *pdftest.Builder) string {
			cid := b.Add(fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Arial /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> %s /FontDescriptor %d 0 R >>", cidFont, descriptor(b, 32, "FontFile2", "<< >>")))
			return fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Arial /Encoding %s /DescendantFonts [%d 0 R] >>", encoding, cid)
		})
	}
	appearance := func(b *pdftest.Builder) int {
		return b.AddStream("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] >>", nil)
	}
	// annotation returns a valid fixture whose page has an annotation of
	// type sub with the entries; %[1]d in entries is an appearance stream.
	annotation := func(sub, entries string) *fixture {
		return extra(func(b *pdftest.Builder) (string, string) {
			entries := strings.ReplaceAll(entries, "%[1]d", fmt.Sprint(appearance(b)))
			return "", fmt.Sprintf("/Annots [<< /Type /Annot /Subtype /%s /Rect [0 0 10 10] %s >>]", sub, entries)
		})
	}
	ap := "/F 4 /AP << /N %[1]d 0 R >>"
	// rdf returns a valid fixture whose XMP has an additional description.
	rdf := func(description string) *fixture {
		return mod(func(f *fixture) {
			f.xmp = replaced(validXMP(), "</rdf:RDF>", `<rdf:Description rdf:about="" `+description+"</rdf:Description></rdf:RDF>")
		})
	}
	ocg := func(config string) *fixture {
		return extra(func(b *pdftest.Builder) (string, string) {
			g := b.Add("<< /Type /OCG /Name (Layer) >>")
			return fmt.Sprintf("/OCProperties << /OCGs [%d 0 R] %s >>", g, strings.ReplaceAll(config, "%[1]d", fmt.Sprint(g))), ""
		})
	}

	for _, tc := range []struct {
		name  string
		f     *fixture
		rules string // expected rules, comma separated
	}{
		// PDF-08: the second string of the file identifier.
		{"empty file identifier", replace(" <00112233445566778899AABBCCDDEEFF>]", " <>"+strings.Repeat(" ", 32)+"]"), "PDF-08"},

		// PDF-38: filters.
		{"non-Identity Crypt filter", extra(func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Contents %d 0 R", b.AddStream("<< /Filter /Crypt /DecodeParms << /Name /StdCF >> >>", []byte("data")))
		}), "PDF-38"},
		{"Identity Crypt filter", extra(func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Contents %d 0 R", b.AddStream("<< /Filter /Crypt /DecodeParms << /Name /Identity >> >>", []byte("q Q")))
		}), ""},
		{"non-standard filter", extra(func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Contents %d 0 R", b.AddStream("<< /Filter [/FlateDecode /BrotliDecode] >>", []byte("data")))
		}), "PDF-38"},

		// PDF-41 to PDF-49: file structure.
		{"header comment of ASCII bytes", replace("%\xe2\xe3\xcf\xd3", "%abcd"), "PDF-41"},
		{"header version 1.8", replace("%PDF-1.7", "%PDF-1.8"), "PDF-41"},
		{"data after the end-of-file marker", mod(func(f *fixture) {
			f.post = func(b []byte) []byte { return append(b, "garbage\n"...) }
		}), "PDF-42"},
		{"CR LF after the end-of-file marker", mod(func(f *fixture) {
			f.post = func(b []byte) []byte { return append(b[:len(b)-1], "\r\n"...) }
		}), ""},
		{"xref keyword followed by a space", replace("xref\n0 ", "xref 0 "), "PDF-43"},
		{"odd hexadecimal string", catalog("/Lang <656>"), "PDF-44"},
		{"hexadecimal string with white space", catalog("/Lang <65 6E>"), ""},
		{"stream with external data", extra(func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Contents %d 0 R", b.AddStream("<< /F (data.bin) >>", nil))
		}), "PDF-45"},
		// Without its end-of-line marker, the data is a byte longer than /Length.
		{"endstream after a space", replace("\nendstream", " endstream"), "PDF-45,PDF-45"},
		{"structure type not in UTF-8", catalog("/StructTreeRoot << /Type /StructTreeRoot /K << /Type /StructElem /S /Paragraphe#E9 >> >>"), "PDF-46"},
		{"structure type in UTF-8", catalog("/StructTreeRoot << /Type /StructTreeRoot /K << /Type /StructElem /S /Paragraphe#C3#A9 >> >>"), ""},
		{"colourant not in UTF-8", res("/ColorSpace << /CS0 [/Separation /Rouge#E9 /DeviceRGB " + tint + "] >>"), "PDF-46"},
		{"endobj after a space", replace(">>\nendobj", ">> endobj"), "PDF-47"},
		{"permissions other than UR3 and DocMDP", catalog("/Perms << /Other << /Type /Sig >> >>"), "PDF-48"},
		{"UR3 permissions", catalog("/Perms << /UR3 << /Type /Sig >> >>"), ""},
		{"integer above 32 bits", catalog("/Size 2147483648"), "PDF-49"},
		{"largest integer", catalog("/Size 2147483647"), ""},
		{"name of 128 bytes", catalog("/Key /" + strings.Repeat("a", 128)), "PDF-49"},
		{"string of 32768 bytes", catalog("/Lang (" + strings.Repeat("a", 32768) + ")"), "PDF-49"},
		{"page box of 2 units", page("/CropBox [0 0 2 2]"), "PDF-49"},
		{"DeviceN of 33 colourants", res("/ColorSpace << /CS0 [/DeviceN [" + strings.Repeat("/Cyan ", 33) + "] /DeviceCMYK " + tint + "] >>"), "PDF-49,PDF-52"},

		// PDF-50: output intents.
		{"output intent with an input profile", intents(func(b *pdftest.Builder) string {
			return intent("GTS_PDFA1", profile(b, "scnr", "RGB ", 2, 3))
		}), "PDF-50"},
		{"output intent with a Lab profile", intents(func(b *pdftest.Builder) string {
			return intent("GTS_PDFA1", profile(b, "mntr", "Lab ", 2, 3))
		}), "PDF-50"},
		{"output intent with a version 5 profile", intents(func(b *pdftest.Builder) string {
			return intent("GTS_PDFA1", profile(b, "mntr", "RGB ", 5, 3))
		}), "PDF-50"},
		{"output intent with a CMYK printer profile", intents(func(b *pdftest.Builder) string {
			return intent("GTS_PDFA1", profile(b, "prtr", "CMYK", 4, 4))
		}), ""},
		{"output intents with different profiles", intents(func(b *pdftest.Builder) string {
			return intent("GTS_PDFA1", profile(b, "mntr", "RGB ", 2, 3)) + intent("GTS_PDFX", profile(b, "mntr", "RGB ", 2, 3))
		}), "PDF-50"},
		{"output intents with the same profile", intents(func(b *pdftest.Builder) string {
			p := profile(b, "mntr", "RGB ", 2, 3)
			return intent("GTS_PDFA1", p) + intent("GTS_PDFX", p)
		}), ""},

		// PDF-51 to PDF-53: colour spaces.
		{"ICCBased abstract profile", iccBased("abst", "RGB ", 2, 3), "PDF-51"},
		{"ICCBased version 5 profile", iccBased("scnr", "RGB ", 5, 3), "PDF-51"},
		{"ICCBased Lab profile", iccBased("spac", "Lab ", 4, 3), ""},
		{"DeviceCMYK with an RGB output intent", res("/ColorSpace << /CS0 [/Indexed /DeviceCMYK 0 <00000000>] >>"), "PDF-52"},
		{"DeviceRGB with an RGB output intent", res("/ColorSpace << /CS0 [/Indexed /DeviceRGB 0 <000000>] >>"), ""},
		{"DeviceCMYK with a DefaultCMYK", resources(func(b *pdftest.Builder) string {
			return fmt.Sprintf("/ColorSpace << /DefaultCMYK [/ICCBased %d 0 R] /CS0 [/Indexed /DeviceCMYK 0 <00000000>] >>", profile(b, "prtr", "CMYK", 4, 4))
		}), ""},
		{"DeviceCMYK image", image("/ColorSpace /DeviceCMYK /BitsPerComponent 8"), "PDF-52"},
		{"DeviceGray without output intent", mod(func(f *fixture) {
			f.noOutputIntent = true
			f.extra = func(*pdftest.Builder) (string, string) {
				return "", "/Resources << /ColorSpace << /CS0 [/Indexed /DeviceGray 0 <00>] >> >>"
			}
		}), "PDF-06,PDF-52"},
		{"DeviceN spot colour without Colorants", res("/ColorSpace << /CS0 [/DeviceN [/Spot /Cyan] /DeviceRGB " + tint + "] >>"), "PDF-53"},
		{"DeviceN spot colour in Colorants", res("/ColorSpace << /CS0 [/DeviceN [/Spot] /DeviceRGB " + tint + " << /Colorants << /Spot [/Separation /Spot /DeviceRGB " + tint + "] >> >>] >>"), ""},
		{"Separations of the same name differ", res("/ColorSpace << /CS0 [/Separation /Spot /DeviceRGB " + tint + "] /CS1 [/Separation /Spot /DeviceRGB " + strings.Replace(tint, "[1 0 0]", "[0 1 0]", 1) + "] >>"), "PDF-53"},
		{"Separations of the same name agree", res("/ColorSpace << /CS0 [/Separation /Spot /DeviceRGB " + tint + "] /CS1 [/Separation /Spot /DeviceRGB " + tint + "] >>"), ""},

		// PDF-54, PDF-55, PDF-58: graphics states.
		{"transfer function", gs("/TR /Identity"), "PDF-54"},
		{"default TR2", gs("/TR2 /Default"), ""},
		{"halftone of type 6", gs("/HT << /Type /Halftone /HalftoneType 6 /Width 1 /Height 1 >>"), "PDF-54"},
		{"halftone of type 1", gs("/HT << /Type /Halftone /HalftoneType 1 /Frequency 60 /Angle 45 /SpotFunction /Round >>"), ""},
		{"unknown rendering intent", gs("/RI /Vivid"), "PDF-55"},
		{"perceptual rendering intent", gs("/RI /Perceptual"), ""},
		{"unknown blend mode", gs("/BM /Dissolve"), "PDF-58"},
		{"Multiply blend mode", gs("/BM /Multiply"), ""},

		// PDF-55 to PDF-57: images and XObjects.
		{"image with an unknown rendering intent", image(rgb + "/Intent /Vivid"), "PDF-55"},
		{"interpolated image", image(rgb + "/Interpolate true"), "PDF-56"},
		{"image with alternates", image(rgb + "/Alternates []"), "PDF-56"},
		{"image of 3 bits per component", image("/ColorSpace /DeviceRGB /BitsPerComponent 3"), "PDF-56"},
		{"image mask", image("/ImageMask true /BitsPerComponent 1"), ""},
		{"form XObject with OPI", xobject("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /OPI << /2.0 << >> >> >>", nil), "PDF-57"},
		{"reference XObject", xobject("<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Ref << /F (other.pdf) /Page 0 >> >>", nil), "PDF-57"},
		{"PostScript XObject", xobject("<< /Type /XObject /Subtype /PS >>", []byte("showpage")), "PDF-57"},

		// PDF-59, PDF-60: fonts.
		{"embedded font without widths", trueType(32, "/Encoding /WinAnsiEncoding"), "PDF-59"},
		{"widths of the wrong length", trueType(32, "/Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 34 /Widths [250 300]"), "PDF-59"},
		{"CFF font program of subtype Type1", font(func(b *pdftest.Builder) string {
			return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Font /FirstChar 32 /LastChar 32 /Widths [250] /FontDescriptor %d 0 R >>", descriptor(b, 32, "FontFile3", "<< /Subtype /Type1 >>"))
		}), "PDF-59"},
		{"CFF font program of subtype Type1C", font(func(b *pdftest.Builder) string {
			return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Font /FirstChar 32 /LastChar 32 /Widths [250] /FontDescriptor %d 0 R >>", descriptor(b, 32, "FontFile3", "<< /Subtype /Type1C >>"))
		}), ""},
		{"embedded CIDFont without CIDToGIDMap", composite("", "/Identity-H"), "PDF-59"},
		{"CMap neither predefined nor embedded", composite("/CIDToGIDMap /Identity", "/Custom-H"), "PDF-59"},
		{"non-symbolic TrueType font without encoding", trueType(32, "/FirstChar 32 /LastChar 33 /Widths [250 300]"), "PDF-60"},
		{"symbolic TrueType font with an encoding", trueType(4, "/Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 33 /Widths [250 300]"), "PDF-60"},
		{"non-symbolic TrueType font with a base encoding", trueType(32, "/Encoding << /BaseEncoding /MacRomanEncoding /Differences [32 /space] >> /FirstChar 32 /LastChar 33 /Widths [250 300]"), ""},

		// PDF-61 to PDF-64: annotations and forms.
		{"Sound annotation", annotation("Sound", ap), "PDF-61"},
		{"unknown annotation type", annotation("Hologram", ap), "PDF-61"},
		{"annotation without flags", annotation("Text", "/AP << /N %[1]d 0 R >>"), "PDF-62"},
		{"hidden annotation", annotation("Text", "/F 6 /AP << /N %[1]d 0 R >>"), "PDF-62"},
		{"Popup without flags nor appearance", annotation("Popup", ""), ""},
		{"annotation without appearance", annotation("Text", "/F 4"), "PDF-63"},
		{"down appearance", annotation("Text", "/F 4 /AP << /N %[1]d 0 R /D %[1]d 0 R >>"), "PDF-63"},
		{"Link without appearance", annotation("Link", "/F 4"), ""},
		{"NeedAppearances", catalog("/AcroForm << /Fields [] /NeedAppearances true >>"), "PDF-64"},
		{"NeedsRendering", catalog("/NeedsRendering true"), "PDF-64"},
		{"widget action", extra(func(b *pdftest.Builder) (string, string) {
			w := b.Add(fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (amount) /Rect [0 0 10 10] /P 3 0 R /F 4 /AP << /N %d 0 R >> /A << /S /GoTo /D [3 0 R /Fit] >> >>", appearance(b)))
			return fmt.Sprintf("/AcroForm << /Fields [%d 0 R] >>", w), fmt.Sprintf("/Annots [%d 0 R]", w)
		}), "PDF-64"},

		// PDF-65 to PDF-67: XMP metadata.
		{"metadata stream of subtype XMP", replace("/Type /Metadata /Subtype /XML", "/Type /Metadata /Subtype /XMP"), "PDF-65"},
		{"xpacket header with bytes", mod(func(f *fixture) {
			f.xmp = replaced(validXMP(), `id="W5M0MpCehiHzreSzNTczkc9d"`, `id="W5M0MpCehiHzreSzNTczkc9d" bytes="4096"`)
		}), "PDF-65"},
		{"unreadable page metadata", extra(func(b *pdftest.Builder) (string, string) {
			return "", fmt.Sprintf("/Metadata %d 0 R", b.AddStream("<< /Type /Metadata /Subtype /XML >>", []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF`)))
		}), "PDF-65"},
		{"property of an undescribed schema", rdf(`xmlns:foo="http://example.com/foo/"><foo:bar>x</foo:bar>`), "PDF-66"},
		{"property a predefined schema does not define", rdf(`xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:colour>red</dc:colour>`), "PDF-66"},
		{"property of the wrong value type", rdf(`xmlns:xmpTPg="http://ns.adobe.com/xap/1.0/t/pg/"><xmpTPg:NPages>one</xmpTPg:NPages>`), "PDF-66"},
		{"property of the right value type", rdf(`xmlns:xmpTPg="http://ns.adobe.com/xap/1.0/t/pg/"><xmpTPg:NPages>1</xmpTPg:NPages>`), ""},
		{"property of a large predefined schema", rdf(`xmlns:exif="http://ns.adobe.com/exif/1.0/"><exif:LensModel>50 mm</exif:LensModel>`), ""},
		{"extension schema without prefix", mod(func(f *fixture) {
			f.xmp = replaced(validXMP(), "<pdfaSchema:prefix>fx</pdfaSchema:prefix>", "")
		}), "PDF-67"},
		{"extension property without category", mod(func(f *fixture) {
			f.xmp = replaced(validXMP(), "<pdfaProperty:category>external</pdfaProperty:category>", "")
		}), "PDF-67"},

		// PDF-68: optional content.
		{"optional content", ocg("/D << /Name (Default) /Order [%[1]d 0 R] >>"), ""},
		{"optional content configuration without name", ocg("/D << /Order [%[1]d 0 R] >>"), "PDF-68"},
		{"optional content configurations of the same name", ocg("/D << /Name (Default) >> /Configs [<< /Name (Default) >>]"), "PDF-68"},
		{"optional content configuration with AS", ocg("/D << /Name (Default) /AS [] >>"), "PDF-68"},
		{"optional content group left out of Order", ocg("/D << /Name (Default) /Order [] >>"), "PDF-68"},

		// PDF-69: presentations and requirements.
		{"alternate presentations", mod(func(f *fixture) { f.names = " /AlternatePresentations << /Names [] >>" }), "PDF-69"},
		{"presentation steps", page("/PresSteps << /Type /NavNode >>"), "PDF-69"},
		{"requirements", catalog("/Requirements [<< /Type /Requirement /S /EnableJavaScripts >>]"), "PDF-69"},

		// PDF/A-4 is out of the scope of these rules.
		{"PDF/A-4", mod(func(f *fixture) {
			f.version, f.noInfo = "2.0", true
			f.xmp = xmpWith(func(p *xmp.Params) { p.PDFAPart = "4" })
			f.extra = func(*pdftest.Builder) (string, string) {
				return "/Requirements [<< /Type /Requirement /S /EnableJavaScripts >>]", "/PresSteps << /Type /NavNode >>"
			}
		}), ""},
	} {
		t.Run(tc.name, func(t *testing.T) { checkContainerRules(t, tc.f, tc.rules) })
	}
}
