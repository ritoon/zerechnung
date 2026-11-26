package qr

import (
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/qrbill"
	"github.com/ritoon/zerechnung/qrcode"
)

// TestQRBill has the symbols of package qrbill, drawn with the Swiss cross
// of Render, read by both decoders, for the six examples of Annex A of the
// Swiss Implementation Guidelines for the QR-bill, version 2.4 (tables 17
// to 22; example 4 with the check digits of its Creditor Reference
// corrected): the content, level M, a single segment in byte mode, version
// 25 at most. The examples are drawn at 2 and 4 pixels per module, the
// cross then 4 to 7 pixels wide per arm.
func TestQRBill(t *testing.T) {
	ucr := strings.Repeat("\n", 7)
	muster := "S\nMax Muster & Söhne (sample company)\nMusterstrasse\n123\n8000\nSeldwyla\nCH\n"
	simon := "S\nSimon Muster\nMusterstrasse\n1\n8000\nSeldwyla\nCH\n"
	sarah := "S\nSarah Beispiel\nMusterstrasse\n1\n8000\nSeldwyla\nCH\n"
	examples := []string{
		"SPC\n0200\n1\nCH6431961000004421557\n" + muster + ucr + "50.00\nCHF\n" + simon +
			"QRR\n000008207791225857421286694\nPayment of travel\nEPD",
		"SPC\n0200\n1\nCH4431999123000889012\n" + muster + ucr + "1949.75\nCHF\n" + simon +
			"QRR\n210000000003139471430009017\nOrder from 15.10.2020\nEPD\n" +
			"//S1/10/1234/11/201021/30/102673386/32/7.7/40/0:30\neBill/B/simon.muster@example.com",
		"SPC\n0200\n1\nCH5204835012345671000\nS\nMuster Stiftung (sample foundation)\nP.O. box\n\n3001\nBern\nCH\n" +
			ucr + "\nCHF\n" + ucr + "NON\n\n\nEPD",
		"SPC\n0200\n1\nCH5800791123000889012\nS\nMuster Krankenkasse (sample health insurer)\nMusterstrasse\n12\n8000\nSeldwyla\nCH\n" +
			ucr + "211.00\nCHF\n" + sarah + "SCOR\nRF240191230100405JSH0438\n\nEPD",
		"SPC\n0200\n1\nCH5800791123000889012\nS\nMax Muster & Söhne (sample company)\nMusterstrasse\n123\n9490\nVaduz\nLI\n" +
			ucr + "199.95\nCHF\n" + sarah + "SCOR\nRF18539007547034\n\nEPD",
		"SPC\n0200\n1\nCH5800791123000889012\n" + muster + ucr + "199.95\nCHF\n" +
			"S\nSarah Beispiel\nMusterstrasse\n1\n78462\nKonstanz\nDE\nSCOR\nRF18539007547034\n\nEPD",
	}
	for i, content := range examples {
		bill, err := qrbill.Parse(content, qrbill.Guidelines24)
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		sym, err := bill.Symbol()
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		for _, scale := range []int{2, 4} {
			img := decodePNG(t, sym, qrbill.Render(sym, scale))
			for _, utf8 := range []bool{true, false} {
				text, level, err := zxing(img, utf8)
				if err != nil || text != content || level != "M" {
					t.Errorf("example %d, scale %d: gozxing (UTF-8 hint %v) reads %q, level %s, %v", i+1, scale, utf8, text, level, err)
				}
			}
			r, payload, err := detailed(img)
			if err != nil {
				t.Errorf("example %d, scale %d: piglig: %v", i+1, scale, err)
				continue
			}
			const byteMode = 4
			if string(payload) != content || r.Version != sym.Version() || r.Version > 25 || int(r.Ecc) != int(qrcode.Medium) ||
				len(r.Segments) != 1 || r.Segments[0].Mode != byteMode {
				t.Errorf("example %d, scale %d: piglig reads v%d, level %d, %d segments, %q", i+1, scale, r.Version, r.Ecc, len(r.Segments), payload)
			}
		}
	}
}
