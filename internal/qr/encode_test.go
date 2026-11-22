package qr

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
)

// Payloads close to the real ones: the field layouts follow the
// specifications, the values are made up.
var (
	payloadPT = "A:123456789*B:999999990*C:PT*D:FT*E:N*F:20261122*G:FT AB2026/0035*H:JJMRT4WK-35*" +
		"I1:PT*I7:12000.00*I8:2760.00*N:2760.00*O:14760.00*Q:bhGU*R:9999"
	payloadCH = strings.Join([]string{"SPC", "0200", "1", "CH4431999123000889012", "S", "Robert Schneider AG",
		"Rue du Lac", "1268", "2501", "Biel", "CH", "", "", "", "", "", "", "", "1949.75", "CHF", "S",
		"Pia-Maria Rutschmann-Schnyder", "Grosse Marktgasse", "28", "9400", "Rorschach", "CH", "QRR",
		"210000000003139471430009017", "Facture n° 2026-123, prestations d'été", "EPD",
		"//S1/10/10201409/11/190512/20/1400.000-53/30/106017086/31/180508/32/7.7/40/2:10;0:30",
		"Name AV1: UV;UltraPay005;12345", "Name AV2: XY;XYService;54321"}, "\n")
	payloadEPC = strings.Join([]string{"BCD", "002", "1", "SCT", "COBADEFFXXX", "Müller Handwerk GmbH",
		"DE89370400440532013000", "EUR1234.56", "", "RF18539007547034", ""}, "\n")
	payloadES = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678%26G33&fecha=01-09-2026&importe=241.4"
)

// europeCase is the QR code of an invoice in a European country, with the
// constraints of its specification.
type europeCase struct {
	name     string
	encode   func(Options) (*Code, error)
	opts     Options
	payload  string
	mode     int // mode indicator of every segment
	minLevel bool
}

func text(s string) func(Options) (*Code, error) {
	return func(o Options) (*Code, error) { return EncodeText(s, o) }
}

func europeCases() []europeCase {
	ptUpper := strings.Replace(payloadPT, "Q:bhGU", "Q:BHGU", 1)
	b32 := strings.Repeat("0123456789ABCDEFGHIJKLMNOPQRSTUV", 4)[:120]
	byteMode := func(s string) func(Options) (*Code, error) {
		return func(o Options) (*Code, error) { return EncodeBytes([]byte(s), o) }
	}
	return []europeCase{
		// Portaria 195/2020: version 9 or above, level M, byte mode.
		{"PT ATCUD", byteMode(payloadPT), Options{Medium, 9, 0}, payloadPT, indByte, false},
		{"PT ATCUD, upper case only", byteMode(ptUpper), Options{Medium, 9, 0}, ptUpper, indByte, false},
		// Swiss Implementation Guidelines QR-bill: version 25 at most, level M.
		{"CH QR-bill", text(payloadCH), Options{Medium, 0, 25}, payloadCH, indByte, false},
		// Orden HAC/1177/2024 (VERI*FACTU): level M.
		{"ES VERI*FACTU", text(payloadES), Options{Medium, 0, 0}, payloadES, indByte, false},
		// EPC069-12: version 13 at most, level M.
		{"EPC payment code", text(payloadEPC), Options{Medium, 0, 13}, payloadEPC, indByte, false},
		// pagoPA notice: version 4, level M.
		{"IT pagoPA", text("PAGOPA|002|301000000012345678|01234567890|123456"), Options{Medium, 4, 4},
			"PAGOPA|002|301000000012345678|01234567890|123456", indByte, false},
		// PAY by square: version 17 at most, level L.
		{"SK PAY by square", text(b32), Options{Low, 0, 17}, b32, indAlphanumeric, false},
		// Slovenian fiscal receipt: 60 digits, version 2, level M.
		{"SI receipt", text("223175087923687075112234402528973166755123456781508151013321"), Options{Medium, 2, 2},
			"223175087923687075112234402528973166755123456781508151013321", indNumeric, false},
		// Belgian cash register system: version 2, level M.
		{"BE SCE", text("HTTPS://FDM.EXAMPLE.BE/V/ABCD1234EFGH5"), Options{Medium, 2, 2},
			"HTTPS://FDM.EXAMPLE.BE/V/ABCD1234EFGH5", indAlphanumeric, false},
		// Croatian fiscal receipt: level L at least.
		{"HR receipt", text("https://porezna.gov.hr/rn?jir=6f4a1c2e-9b3d-4e5f-8a7b-1c2d3e4f5a6b&datv=20261122_1015&izn=1250"),
			Options{Low, 0, 0}, "https://porezna.gov.hr/rn?jir=6f4a1c2e-9b3d-4e5f-8a7b-1c2d3e4f5a6b&datv=20261122_1015&izn=1250", indByte, true},
	}
}

func TestEurope(t *testing.T) {
	for _, tc := range europeCases() {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.encode(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			s := readSymbol(t, c)
			lo, hi := tc.opts.versions()
			if s.version < lo || s.version > hi {
				t.Errorf("version %d, want %d to %d", s.version, lo, hi)
			}
			if s.level != tc.opts.Level.String() {
				t.Errorf("level %s, want %s", s.level, tc.opts.Level)
			}
			if s.payload() != tc.payload {
				t.Errorf("payload %q, want %q", s.payload(), tc.payload)
			}
			for _, seg := range s.segments {
				if seg.mode != tc.mode {
					t.Errorf("segment in mode %d, want %d", seg.mode, tc.mode)
				}
			}
			if len(s.segments) != 1 {
				t.Errorf("%d segments, want 1", len(s.segments))
			}
			t.Logf("v%d-%s mask %d, %d×%d modules", s.version, s.level, s.mask, c.Size(), c.Size())
		})
	}
}

// TestByteModeForced shows why EncodeBytes exists: the upper case variant of
// the Portuguese code fits the alphanumeric mode, which EncodeText picks,
// while the specification requires the byte mode.
func TestByteModeForced(t *testing.T) {
	ptUpper := strings.Replace(payloadPT, "Q:bhGU", "Q:BHGU", 1)
	c, err := EncodeText(ptUpper, Options{Level: Medium, MinVersion: 9})
	if err != nil {
		t.Fatal(err)
	}
	if s := readSymbol(t, c); s.segments[0].mode != indAlphanumeric {
		t.Errorf("EncodeText: mode %d, want alphanumeric", s.segments[0].mode)
	}
	c, err = EncodeBytes([]byte(ptUpper), Options{Level: Medium, MinVersion: 9})
	if err != nil {
		t.Fatal(err)
	}
	if s := readSymbol(t, c); s.segments[0].mode != indByte {
		t.Errorf("EncodeBytes: mode %d, want byte", s.segments[0].mode)
	}
}

func TestECI(t *testing.T) {
	eci, err := ECI(26)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Bytes([]byte("Müller Handwerk GmbH"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Encode([]Segment{eci, data}, Options{Level: Medium})
	if err != nil {
		t.Fatal(err)
	}
	s := readSymbol(t, c)
	if len(s.segments) != 2 || s.segments[0].mode != indECI || s.segments[0].eci != 26 ||
		s.segments[1].mode != indByte || s.segments[1].text != "Müller Handwerk GmbH" {
		t.Errorf("segments %+v", s.segments)
	}
	for _, n := range []int{0, 127, 128, 16383, 16384, 999999} {
		eci, err := ECI(n)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Encode([]Segment{eci}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if s := readSymbol(t, c); len(s.segments) != 1 || s.segments[0].eci != n {
			t.Errorf("ECI %d: segments %+v", n, s.segments)
		}
	}
}

// TestKnownCodewords checks the codewords of two symbols published with
// their bits: the example of ISO/IEC 18004 Annex I and the "HELLO WORLD"
// example of the Thonky QR Code tutorial.
func TestKnownCodewords(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []byte
	}{
		{"01234567", []byte{
			16, 32, 12, 86, 97, 128, 236, 17, 236, 17, 236, 17, 236, 17, 236, 17,
			165, 36, 212, 193, 237, 54, 199, 135, 44, 85}},
		{"HELLO WORLD", []byte{
			32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17,
			196, 35, 39, 119, 235, 215, 231, 226, 93, 23}},
	} {
		c, err := EncodeText(tc.text, Options{Level: Medium, MaxVersion: 1})
		if err != nil {
			t.Fatal(err)
		}
		if s := readSymbol(t, c); !bytes.Equal(s.codewords, tc.want) {
			t.Errorf("%s: codewords\n%v, want\n%v", tc.text, s.codewords, tc.want)
		}
	}
}

// TestFunctionPatterns reads back a symbol of every version and level, which
// checks the format and version information against the tables of ISO/IEC
// 18004. Every mask is tried in versions 1 and 7 (the first with version
// information), one per level in the others.
func TestFunctionPatterns(t *testing.T) {
	segs, err := textSegments("ZERECHNUNG")
	if err != nil {
		t.Fatal(err)
	}
	for ver := MinVersion; ver <= MaxVersion; ver++ {
		for l := Low; l <= High; l++ {
			for msk := range 8 {
				if ver != 1 && ver != 7 && msk != (ver+int(l))%8 {
					continue
				}
				c, err := encode(segs, l, ver, ver, msk)
				if err != nil {
					t.Fatal(err)
				}
				s := readSymbol(t, c)
				if s.version != ver || s.level != l.String() || s.mask != msk || s.payload() != "ZERECHNUNG" {
					t.Fatalf("v%d-%s mask %d: read v%d-%s mask %d %q", ver, l, msk, s.version, s.level, s.mask, s.payload())
				}
			}
		}
	}
}

func TestAlignmentPatternPositions(t *testing.T) {
	for ver := MinVersion; ver <= MaxVersion; ver++ {
		got := newBuilder(ver, Low).alignmentPatternPositions()
		want := alignmentCenters[ver]
		if len(got) != len(want) {
			t.Fatalf("v%d: %v, want %v", ver, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("v%d: %v, want %v", ver, got, want)
			}
		}
	}
}

func TestRawDataModules(t *testing.T) {
	for ver := MinVersion; ver <= MaxVersion; ver++ {
		if got, want := numRawDataModules(ver), totalCodewords[ver]*8+remainderBits(ver); got != want {
			t.Errorf("v%d: %d, want %d", ver, got, want)
		}
	}
}

// TestCapacity checks the capacities of ISO/IEC 18004, Table 7, and the
// limits that specifications state: 331 bytes for the EPC code (version 13,
// level M) and 997 for the Swiss QR-bill (version 25, level M).
func TestCapacity(t *testing.T) {
	for _, tc := range []struct {
		ver                     int
		level                   Level
		numeric, alpha, byteLen int
	}{
		{1, Low, 41, 25, 17},
		{1, Medium, 34, 20, 14},
		{1, Quartile, 27, 16, 11},
		{1, High, 17, 10, 7},
		{2, Medium, 63, 38, 26},
		{4, Medium, 149, 90, 62},
		{9, Low, 552, 335, 230},
		{9, Medium, 432, 262, 180},
		{9, Quartile, 312, 189, 130},
		{9, High, 235, 143, 98},
		{13, Medium, 0, 0, 331},
		{25, Medium, 0, 0, 997},
		{40, Low, 7089, 4296, 2953},
		{40, High, 3057, 1852, 1273},
	} {
		o := Options{Level: tc.level, MinVersion: tc.ver, MaxVersion: tc.ver}
		for _, m := range []struct {
			name string
			n    int
			enc  func(n int) (*Code, error)
		}{
			{"numeric", tc.numeric, func(n int) (*Code, error) { return EncodeText(strings.Repeat("7", n), o) }},
			{"alphanumeric", tc.alpha, func(n int) (*Code, error) { return EncodeText(strings.Repeat("A", n), o) }},
			{"byte", tc.byteLen, func(n int) (*Code, error) { return EncodeBytes(bytes.Repeat([]byte{'a'}, n), o) }},
		} {
			if m.n == 0 {
				continue
			}
			c, err := m.enc(m.n)
			if err != nil {
				t.Errorf("v%d-%s %s: %d characters: %v", tc.ver, tc.level, m.name, m.n, err)
				continue
			}
			readSymbol(t, c)
			if _, err := m.enc(m.n + 1); !errors.Is(err, ErrDataTooLong) {
				t.Errorf("v%d-%s %s: %d characters: %v, want ErrDataTooLong", tc.ver, tc.level, m.name, m.n+1, err)
			}
		}
	}
}

// TestVersionChoice checks that the smallest version of the range that
// holds the data is chosen, and that the level is never raised.
func TestVersionChoice(t *testing.T) {
	for n := 0; n <= 2953; n += 31 {
		data := bytes.Repeat([]byte{'a'}, n)
		c, err := EncodeBytes(data, Options{Level: Low})
		if err != nil {
			t.Fatal(err)
		}
		if c.Level() != Low {
			t.Fatalf("%d bytes: level %s", n, c.Level())
		}
		if c.Version() > 1 {
			if _, err := EncodeBytes(data, Options{Level: Low, MaxVersion: c.Version() - 1}); err == nil {
				t.Fatalf("%d bytes: version %d chosen, %d holds them", n, c.Version(), c.Version()-1)
			}
		}
	}
	c, err := EncodeText("1", Options{Level: Medium, MinVersion: 9})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version() != 9 || c.Level() != Medium {
		t.Errorf("v%d-%s, want v9-M", c.Version(), c.Level())
	}
}

// referencePenalty computes the penalty of a symbol by the rules of ISO/IEC
// 18004, plainly, as Project Nayuki's generator reads them: the finder-like
// pattern is 1:1:3:1:1 at any scale with a light run 4 times as wide on
// either side (each side counts), the quiet zone counting as light.
func referencePenalty(c *Code) int {
	n := c.Size()
	res := 0
	line := func(at func(i int) bool) {
		// Rule 1.
		run := 1
		for i := 1; i <= n; i++ {
			if i < n && at(i) == at(i-1) {
				run++
				continue
			}
			if run >= 5 {
				res += 3 + run - 5
			}
			run = 1
		}
		// Rule 3, on the run lengths of the line padded with light modules.
		runs := []int{n}
		dark := false
		for i := range n {
			if at(i) != dark {
				runs = append(runs, 0)
				dark = !dark
			}
			runs[len(runs)-1]++
		}
		if dark {
			runs = append(runs, n)
		} else {
			runs[len(runs)-1] += n
		}
		for i := 1; i+5 < len(runs); i += 2 { // runs[i] is dark
			u := runs[i]
			if runs[i+1] != u || runs[i+2] != 3*u || runs[i+3] != u || runs[i+4] != u {
				continue
			}
			before, after := runs[i-1], runs[i+5]
			if before >= 4*u && after >= u {
				res += 40
			}
			if after >= 4*u && before >= u {
				res += 40
			}
		}
	}
	for y := range n {
		line(func(x int) bool { return c.Module(x, y) })
	}
	for x := range n {
		line(func(y int) bool { return c.Module(x, y) })
	}
	// Rule 2 and the count of dark modules.
	dark := 0
	for y := range n {
		for x := range n {
			d := c.Module(x, y)
			if d {
				dark++
			}
			if x+1 < n && y+1 < n && c.Module(x+1, y) == d && c.Module(x, y+1) == d && c.Module(x+1, y+1) == d {
				res += 3
			}
		}
	}
	// Rule 4: 10 points for each full 5% of deviation from half dark.
	total := n * n
	res += ((abs(dark*20-total*10)+total-1)/total - 1) * 10
	return res
}

// TestMaskChoice checks the penalty score against referencePenalty for every
// mask, and that the mask chosen has the lowest penalty.
func TestMaskChoice(t *testing.T) {
	inputs := []string{"", "1", "HELLO WORLD", payloadPT, payloadCH, payloadEPC, payloadES,
		strings.Repeat("0", 500), strings.Repeat("zerechnung ", 100)}
	for _, in := range inputs {
		segs, err := textSegments(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range []Level{Low, Medium, High} {
			auto, err := encode(segs, l, MinVersion, MaxVersion, -1)
			if err != nil {
				t.Fatal(err)
			}
			best, bestMask := math.MaxInt, -1
			for msk := range 8 {
				c, err := encode(segs, l, MinVersion, MaxVersion, msk)
				if err != nil {
					t.Fatal(err)
				}
				want := referencePenalty(c)
				b := &builder{version: c.version, size: c.size}
				if got := b.penaltyScore(c.modules, math.MaxInt32); got != want {
					t.Errorf("%.20q v%d-%s mask %d: penalty %d, reference %d", in, c.version, l, msk, got, want)
				}
				if want < best {
					best, bestMask = want, msk
				}
			}
			if auto.Mask() != bestMask {
				t.Errorf("%.20q v%d-%s: mask %d chosen, lowest penalty with mask %d", in, auto.Version(), l, auto.Mask(), bestMask)
			}
		}
	}
}

func TestErrors(t *testing.T) {
	seg, _ := Bytes([]byte("x"))
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"level", func() error { _, err := Encode([]Segment{seg}, Options{Level: 4}); return err }(), ErrInvalidArgument},
		{"negative level", func() error { _, err := Encode([]Segment{seg}, Options{Level: -1}); return err }(), ErrInvalidArgument},
		{"min version", func() error { _, err := Encode([]Segment{seg}, Options{MinVersion: 41}); return err }(), ErrInvalidArgument},
		{"max version", func() error { _, err := Encode([]Segment{seg}, Options{MaxVersion: 41}); return err }(), ErrInvalidArgument},
		{"inverted range", func() error { _, err := Encode([]Segment{seg}, Options{MinVersion: 5, MaxVersion: 4}); return err }(), ErrInvalidArgument},
		{"negative version", func() error { _, err := Encode([]Segment{seg}, Options{MinVersion: -1}); return err }(), ErrInvalidArgument},
		{"mask", func() error { _, err := encode([]Segment{seg}, Low, 1, 40, 8); return err }(), ErrInvalidArgument},
		{"ECI", func() error { _, err := ECI(1000000); return err }(), ErrInvalidArgument},
		{"negative ECI", func() error { _, err := ECI(-1); return err }(), ErrInvalidArgument},
		{"numeric", func() error { _, err := Numeric("12a"); return err }(), ErrUnencodableChar},
		{"empty numeric", func() error { _, err := Numeric(""); return err }(), ErrUnencodableChar},
		{"alphanumeric", func() error { _, err := Alphanumeric("abc"); return err }(), ErrUnencodableChar},
		{"bytes", func() error { _, err := Bytes(make([]byte, 2954)); return err }(), ErrDataTooLong},
		{"digits", func() error { _, err := Numeric(strings.Repeat("1", 7090)); return err }(), ErrDataTooLong},
		{"characters", func() error { _, err := Alphanumeric(strings.Repeat("A", 4297)); return err }(), ErrDataTooLong},
		{"version range", func() error {
			_, err := EncodeBytes(make([]byte, 27), Options{Level: Medium, MaxVersion: 2})
			return err
		}(), ErrDataTooLong},
		{"character count", func() error {
			// 256 bytes need a 16-bit count, from version 10 on.
			_, err := EncodeBytes(make([]byte, 256), Options{Level: Low, MaxVersion: 9})
			return err
		}(), ErrDataTooLong},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, tc.err, tc.want)
		}
	}
}

func TestEmpty(t *testing.T) {
	c, err := EncodeText("", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s := readSymbol(t, c); s.version != 1 || len(s.segments) != 0 {
		t.Errorf("v%d, segments %+v", s.version, s.segments)
	}
	if c.Module(-1, 0) || c.Module(0, c.Size()) {
		t.Error("module outside the symbol is dark")
	}
	if Level(7).String() != "?" {
		t.Errorf("Level(7).String() = %q", Level(7).String())
	}
}

// TestConcurrent encodes from several goroutines, for the race detector:
// the templates of the versions are shared.
func TestConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 20 {
				data := bytes.Repeat([]byte{byte('a' + g)}, 40*i+g)
				c, err := EncodeBytes(data, Options{Level: Medium})
				if err != nil {
					t.Error(err)
					return
				}
				if s, err := decodeSymbol(c); err != nil || s.payload() != string(data) {
					t.Errorf("%d bytes: %v", len(data), err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func FuzzEncodeBytes(f *testing.F) {
	f.Add([]byte(payloadPT), uint8(1), uint8(9))
	f.Add([]byte(payloadCH), uint8(1), uint8(1))
	f.Add([]byte{}, uint8(3), uint8(40))
	f.Fuzz(func(t *testing.T, data []byte, level, minVer uint8) {
		o := Options{Level: Level(level % 4), MinVersion: int(minVer%40) + 1}
		c, err := EncodeBytes(data, o)
		if errors.Is(err, ErrDataTooLong) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		s := readSymbol(t, c)
		if s.payload() != string(data) || s.version < o.MinVersion || s.level != o.Level.String() {
			t.Errorf("v%d-%s %q", s.version, s.level, s.payload())
		}
	})
}

func BenchmarkEncode(b *testing.B) {
	for _, bc := range []struct {
		name string
		data string
		opts Options
	}{
		{"PT", payloadPT, Options{Level: Medium, MinVersion: 9}},
		{"CH", payloadCH, Options{Level: Medium, MaxVersion: 25}},
		{"EPC", payloadEPC, Options{Level: Medium, MaxVersion: 13}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			data := []byte(bc.data)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := EncodeBytes(data, bc.opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
