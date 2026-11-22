package qr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	zxqr "github.com/makiuchi-d/gozxing/qrcode"
	piglig "github.com/piglig/go-qr"
	"github.com/ritoon/zerechnung/internal/qr"
)

// render draws c with a quiet zone of 4 modules, scale pixels per module.
func render(c *qr.Code, scale int) *image.Gray {
	const quiet = 4
	side := (c.Size() + 2*quiet) * scale
	img := image.NewGray(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	for y := range c.Size() {
		for x := range c.Size() {
			if !c.Module(x, y) {
				continue
			}
			for dy := range scale {
				row := ((y+quiet)*scale + dy) * img.Stride
				for dx := range scale {
					img.Pix[row+(x+quiet)*scale+dx] = 0
				}
			}
		}
	}
	return img
}

// zxing decodes img with gozxing and returns the text and the error
// correction level. With utf8, the byte segments are read as UTF-8;
// otherwise gozxing guesses their character set, as most scanners do.
func zxing(img image.Image, utf8 bool) (string, string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", "", err
	}
	hints := map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_PURE_BARCODE: true}
	if utf8 {
		hints[gozxing.DecodeHintType_CHARACTER_SET] = "UTF-8"
	}
	r, err := zxqr.NewQRCodeReader().Decode(bmp, hints)
	if err != nil {
		return "", "", err
	}
	level, _ := r.GetResultMetadata()[gozxing.ResultMetadataType_ERROR_CORRECTION_LEVEL].(string)
	return r.GetText(), level, nil
}

// detailed decodes img with the decoder of github.com/piglig/go-qr, the
// module the encoder comes from, but a code path of its own. Unlike
// gozxing, it reports the version, the mask and the bytes of each segment.
func detailed(img image.Image) (*piglig.DecodeResult, []byte, error) {
	r, err := piglig.DecodeDetailed(img)
	if err != nil {
		return nil, nil, err
	}
	var b []byte
	for _, seg := range r.Segments {
		b = append(b, seg.Bytes...)
	}
	return r, b, nil
}

// check has c, which holds data, read by both decoders. gozxing turns the
// bytes it reads into text, so it is left out for data that is not UTF-8.
func check(t *testing.T, c *qr.Code, data []byte, textual bool) {
	t.Helper()
	img := render(c, 4)
	name := fmt.Sprintf("v%d-%s mask %d", c.Version(), c.Level(), c.Mask())
	if textual {
		text, level, err := zxing(img, true)
		if err != nil {
			t.Errorf("%s: gozxing: %v", name, err)
		} else {
			if text != string(data) {
				t.Errorf("%s: gozxing reads %q", name, text)
			}
			if level != c.Level().String() {
				t.Errorf("%s: gozxing reads level %s", name, level)
			}
		}
	}
	r, payload, err := detailed(img)
	if err != nil {
		t.Errorf("%s: piglig: %v", name, err)
		return
	}
	if !bytes.Equal(payload, data) {
		t.Errorf("%s: piglig reads %q", name, payload)
	}
	if r.Version != c.Version() || int(r.Ecc) != int(c.Level()) || r.Mask != c.Mask() {
		t.Errorf("%s: piglig reads v%d, level %d, mask %d", name, r.Version, r.Ecc, r.Mask)
	}
}

// Payloads close to the real ones: the field layouts follow the
// specifications, the values are made up.
var europe = []struct {
	name  string
	data  string
	opts  qr.Options
	bytes bool // byte mode imposed
}{
	{"PT ATCUD", "A:123456789*B:999999990*C:PT*D:FT*E:N*F:20261122*G:FT AB2026/0035*H:JJMRT4WK-35*" +
		"I1:PT*I7:12000.00*I8:2760.00*N:2760.00*O:14760.00*Q:bhGU*R:9999",
		qr.Options{Level: qr.Medium, MinVersion: 9}, true},
	{"CH QR-bill", strings.Join([]string{"SPC", "0200", "1", "CH4431999123000889012", "S", "Robert Schneider AG",
		"Rue du Lac", "1268", "2501", "Biel", "CH", "", "", "", "", "", "", "", "1949.75", "CHF", "S",
		"Pia-Maria Rutschmann-Schnyder", "Grosse Marktgasse", "28", "9400", "Rorschach", "CH", "QRR",
		"210000000003139471430009017", "Facture n° 2026-123, prestations d'été", "EPD",
		"//S1/10/10201409/11/190512/20/1400.000-53/30/106017086/31/180508/32/7.7/40/2:10;0:30",
		"Name AV1: UV;UltraPay005;12345", "Name AV2: XY;XYService;54321"}, "\n"),
		qr.Options{Level: qr.Medium, MaxVersion: 25}, false},
	{"EPC payment code", strings.Join([]string{"BCD", "002", "1", "SCT", "COBADEFFXXX", "Müller Handwerk GmbH",
		"DE89370400440532013000", "EUR1234.56", "", "RF18539007547034", ""}, "\n"),
		qr.Options{Level: qr.Medium, MaxVersion: 13}, false},
	{"ES VERI*FACTU", "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678%26G33&fecha=01-09-2026&importe=241.4",
		qr.Options{Level: qr.Medium}, false},
	{"IT pagoPA", "PAGOPA|002|301000000012345678|01234567890|123456",
		qr.Options{Level: qr.Medium, MinVersion: 4, MaxVersion: 4}, false},
	{"SK PAY by square", strings.Repeat("0123456789ABCDEFGHIJKLMNOPQRSTUV", 4)[:120],
		qr.Options{Level: qr.Low, MaxVersion: 17}, false},
	{"SI receipt", "223175087923687075112234402528973166755123456781508151013321",
		qr.Options{Level: qr.Medium, MinVersion: 2, MaxVersion: 2}, false},
	{"BE SCE", "HTTPS://FDM.EXAMPLE.BE/V/ABCD1234EFGH5",
		qr.Options{Level: qr.Medium, MinVersion: 2, MaxVersion: 2}, false},
	{"HR receipt", "https://porezna.gov.hr/rn?jir=6f4a1c2e-9b3d-4e5f-8a7b-1c2d3e4f5a6b&datv=20261122_1015&izn=1250",
		qr.Options{Level: qr.Low}, false},
}

func TestEurope(t *testing.T) {
	for _, tc := range europe {
		t.Run(tc.name, func(t *testing.T) {
			var c *qr.Code
			var err error
			if tc.bytes {
				c, err = qr.EncodeBytes([]byte(tc.data), tc.opts)
			} else {
				c, err = qr.EncodeText(tc.data, tc.opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			check(t, c, []byte(tc.data), true)
			// Without the hint, as a scanner reads it.
			if text, _, err := zxing(render(c, 4), false); err != nil || text != tc.data {
				t.Errorf("gozxing without the UTF-8 hint: %q, %v", text, err)
			}
			t.Logf("v%d-%s mask %d", c.Version(), c.Level(), c.Mask())
		})
	}
}

// TestVersions fills a symbol of every version and level, in the three
// modes, and has it read: every block structure and alignment pattern
// layout goes through the decoders, with masks of every kind.
func TestVersions(t *testing.T) {
	modes := []struct {
		name string
		char byte
		enc  func([]byte, qr.Options) (*qr.Code, error)
	}{
		{"numeric", '7', func(b []byte, o qr.Options) (*qr.Code, error) { return qr.EncodeText(string(b), o) }},
		{"alphanumeric", 'Z', func(b []byte, o qr.Options) (*qr.Code, error) { return qr.EncodeText(string(b), o) }},
		{"byte", 0, qr.EncodeBytes},
	}
	masks := map[int]bool{}
	for ver := 1; ver <= 40; ver++ {
		for _, l := range []qr.Level{qr.Low, qr.Medium, qr.Quartile, qr.High} {
			m := modes[(ver+int(l))%3]
			o := qr.Options{Level: l, MinVersion: ver, MaxVersion: ver}
			payload := func(n int) []byte {
				b := make([]byte, n)
				for i := range b {
					if m.char != 0 {
						b[i] = m.char
					} else {
						b[i] = byte(i*131 + ver)
					}
				}
				return b
			}
			// The longest payload that the version holds.
			lo, hi := 1, 7090
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if _, err := m.enc(payload(mid), o); err == nil {
					lo = mid
				} else if errors.Is(err, qr.ErrDataTooLong) {
					hi = mid - 1
				} else {
					t.Fatal(err)
				}
			}
			data := payload(lo)
			c, err := m.enc(data, o)
			if err != nil {
				t.Fatal(err)
			}
			masks[c.Mask()] = true
			check(t, c, data, m.char != 0)
		}
	}
	t.Logf("masks chosen: %v", masks)
}
