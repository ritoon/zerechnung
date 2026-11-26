package qr

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/atcud"
	"github.com/ritoon/zerechnung/qrcode"
)

// decodePNG draws sym in PNG with r and decodes the file.
func decodePNG(t *testing.T, sym *qrcode.Symbol, r *qrcode.Render) image.Image {
	t.Helper()
	var b bytes.Buffer
	if err := sym.PNG(&b, r); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&b)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// TestATCUD has the symbols of package atcud read by both decoders, for
// examples 1 to 5 of section 5 of the specification of the AT (version
// 1.1): the content, level M, a single segment in byte mode, and the
// version of the figure of the specification.
func TestATCUD(t *testing.T) {
	examples := []struct {
		content string
		version int
	}{
		{"A:123456789*B:999999990*C:PT*D:FT*E:N*F:20191231*G:FT AB2019/0035*H:CSDF7T5H-0035*" +
			"I1:PT*I2:12000.00*I3:15000.00*I4:900.00*I5:50000.00*I6:6500.00*I7:80000.00*I8:18400.00*" +
			"J1:PT-AC*J2:10000.00*J3:25000.56*J4:1000.02*J5:75000.00*J6:6750.00*J7:100000.00*J8:18000.00*" +
			"K1:PT-MA*K2:5000.00*K3:12500.00*K4:625.00*K5:25000.00*K6:3000.00*K7:40000.00*K8:8800.00*" +
			"L:100.00*M:25.00*N:64000.02*O:513600.58*P:100.00*Q:kLp0*R:9999*S:TB;PT00000000000000000000000;513500.58", 17},
		{"A:123456789*B:999999990*C:PT*D:FS*E:N*F:20190812*G:FS CDVF/12345*H:CDF7T5HD-12345*" +
			"I1:PT*I7:0.65*I8:0.15*N:0.15*O:0.80*Q:YhGV*R:9999*S:NU;0.80", 9},
		{"A:500000000*B:123456789*C:PT*D:PF*E:N*F:20190123*G:PF G2019CB/145789*H:HB6FT7RV-145789*" +
			"I1:PT*I2:12345.34*I3:12532.65*I4:751.96*I5:52789.00*I6:6862.57*I7:32425.69*I8:7457.91*N:15072.44*O:125165.12*Q:r/fY*R:9999", 10},
		{"A:500000000*B:123456789*C:PT*D:GT*E:N*F:20190720*G:GT G234CB/50987*H:GTVX4Y8B-50987*" +
			"I1:0*N:0.00*O:0.00*Q:5uIg*R:9999", 9},
		{"A:123456789*B:4443332215*C:FR*D:FT*E:N*F:20190526*G:ABC BNH/4561*H:DK5ZJ2HN-4561*" +
			"I1:FR*I7:100.00*I8:20.00*N:20.00*O:120.00*Q:YJRE*R:9999", 9},
	}
	for i, ex := range examples {
		q, err := atcud.Parse(ex.content)
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		sym, err := q.Symbol()
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		img := decodePNG(t, sym, &qrcode.Render{Scale: 2})
		for _, utf8 := range []bool{true, false} {
			text, level, err := zxing(img, utf8)
			if err != nil || text != ex.content || level != "M" {
				t.Errorf("example %d: gozxing (UTF-8 hint %v) reads %q, level %s, %v", i+1, utf8, text, level, err)
			}
		}
		r, payload, err := detailed(img)
		if err != nil {
			t.Fatalf("example %d: piglig: %v", i+1, err)
		}
		const byteMode = 4
		if string(payload) != ex.content || r.Version != ex.version || int(r.Ecc) != int(qrcode.Medium) ||
			len(r.Segments) != 1 || r.Segments[0].Mode != byteMode {
			t.Errorf("example %d: piglig reads v%d, level %d, %d segments, %q", i+1, r.Version, r.Ecc, len(r.Segments), payload)
		}
	}
}

// TestOverlay draws a QR-bill symbol with a cross at its center, 7 mm on
// a code of 46 mm as the Swiss cross of the QR-bill: both decoders read it
// through the error correction.
func TestOverlay(t *testing.T) {
	data := strings.Join([]string{"SPC", "0200", "1", "CH4431999123000889012", "S", "Robert Schneider AG",
		"Rue du Lac", "1268", "2501", "Biel", "CH", "", "", "", "", "", "", "", "1949.75", "CHF", "S",
		"Pia-Maria Rutschmann-Schnyder", "Grosse Marktgasse", "28", "9400", "Rorschach", "CH", "QRR",
		"210000000003139471430009017", "Facture n° 2026-123", "EPD"}, "\n")
	sym, err := qrcode.Encode([]byte(data), qrcode.Options{Level: qrcode.Medium, MaxVersion: 25})
	if err != nil {
		t.Fatal(err)
	}
	n := float64(sym.Size())
	side := n * 7 / 46
	c := n / 2
	arm, width := side*0.6, side*0.2
	cross := []qrcode.Rect{
		{X: c - side/2, Y: c - side/2, W: side, H: side, Light: true},        // white border
		{X: c - side/2 + 0.5, Y: c - side/2 + 0.5, W: side - 1, H: side - 1}, // black square
		{X: c - arm/2, Y: c - width/2, W: arm, H: width, Light: true},        // white bars
		{X: c - width/2, Y: c - arm/2, W: width, H: arm, Light: true},
	}
	img := decodePNG(t, sym, &qrcode.Render{Scale: 4, Overlay: cross})
	if text, _, err := zxing(img, true); err != nil || text != data {
		t.Errorf("gozxing reads %q, %v", text, err)
	}
	if _, payload, err := detailed(img); err != nil || string(payload) != data {
		t.Errorf("piglig reads %q, %v", payload, err)
	}
}
