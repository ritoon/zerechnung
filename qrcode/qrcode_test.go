package qrcode

import (
	"bytes"
	"errors"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func encode(t *testing.T, data string, o Options) *Symbol {
	t.Helper()
	s, err := Encode([]byte(data), o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEncode(t *testing.T) {
	s := encode(t, "A:123456789*B:999999990", Options{Level: Medium, MinVersion: 9})
	if s.Version() != 9 || s.Size() != 53 || s.Level() != Medium || s.Level().String() != "M" {
		t.Errorf("v%d, size %d, level %s", s.Version(), s.Size(), s.Level())
	}
	// The finder pattern at the top left corner, and nothing outside.
	if !s.Module(0, 0) || s.Module(7, 0) || !s.Module(3, 3) || s.Module(-1, 0) || s.Module(0, 53) {
		t.Error("modules of the finder pattern")
	}
	if _, err := Encode(bytes.Repeat([]byte("x"), 18), Options{MaxVersion: 1}); !errors.Is(err, ErrDataTooLong) {
		t.Errorf("18 bytes in version 1-L: %v", err)
	}
	if _, err := Encode(nil, Options{MinVersion: 41}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("version 41: %v", err)
	}
}

// TestImage checks that each module covers Scale × Scale pixels of its
// colour, after the quiet zone.
func TestImage(t *testing.T) {
	s := encode(t, "ZeRechnung", Options{Level: Quartile})
	for _, tc := range []struct {
		r            *Render
		scale, quiet int
	}{
		{nil, 4, 4},
		{&Render{Scale: 3, QuietZone: 2}, 3, 2},
		{&Render{Scale: 1, QuietZone: -1}, 1, 0},
	} {
		img := s.Image(tc.r)
		side := (s.Size() + 2*tc.quiet) * tc.scale
		if b := img.Bounds(); b.Dx() != side || b.Dy() != side {
			t.Fatalf("%+v: bounds %v, want %d", tc.r, b, side)
		}
		for y := range side {
			for x := range side {
				mx, my := x/tc.scale-tc.quiet, y/tc.scale-tc.quiet
				if dark := img.ColorIndexAt(x, y) == 1; dark != s.Module(mx, my) {
					t.Fatalf("%+v: pixel (%d, %d) dark %v, module (%d, %d) dark %v", tc.r, x, y, dark, mx, my, !dark)
				}
			}
		}
	}
}

func TestPNG(t *testing.T) {
	s := encode(t, "ZeRechnung", Options{})
	var b bytes.Buffer
	if err := s.PNG(&b, &Render{Scale: 2}); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&b)
	if err != nil {
		t.Fatal(err)
	}
	want := s.Image(&Render{Scale: 2})
	if img.Bounds() != want.Bounds() {
		t.Fatalf("bounds %v, want %v", img.Bounds(), want.Bounds())
	}
	for y := range want.Bounds().Dy() {
		for x := range want.Bounds().Dx() {
			r, _, _, _ := img.At(x, y).RGBA()
			if dark := r < 0x8000; dark != (want.ColorIndexAt(x, y) == 1) {
				t.Fatalf("pixel (%d, %d)", x, y)
			}
		}
	}
}

// TestSVG reads back the modules from the subpaths of the drawing.
func TestSVG(t *testing.T) {
	s := encode(t, "A:500000000*B:123456789*C:PT", Options{Level: Medium})
	var b bytes.Buffer
	err := s.SVG(&b, &Render{Scale: 5, Overlay: []Rect{{X: 10, Y: 10, W: 2.5, H: 1, Light: true}, {X: 1, Y: 1, W: 0, H: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	svg := b.String()
	side := s.Size() + 8
	head := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + strconv.Itoa(side) + " " + strconv.Itoa(side) +
		`" width="` + strconv.Itoa(5*side) + `" height="` + strconv.Itoa(5*side) + `"`
	if !strings.HasPrefix(svg, head) || !strings.HasSuffix(svg, "</svg>\n") {
		t.Errorf("drawing %.200s", svg)
	}
	if !strings.Contains(svg, `<rect x="14" y="14" width="2.5" height="1" fill="#fff"/></svg>`) {
		t.Error("overlay: the rectangle is missing or the empty one is drawn")
	}
	d := regexp.MustCompile(`<path fill="#000" d="([^"]*)"/>`).FindStringSubmatch(svg)
	if d == nil {
		t.Fatal("no path")
	}
	dark := make(map[[2]int]bool)
	for _, m := range regexp.MustCompile(`M(\d+) (\d+)h(\d+)v1h-(\d+)z`).FindAllStringSubmatch(d[1], -1) {
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		n, _ := strconv.Atoi(m[3])
		if m[3] != m[4] {
			t.Fatalf("subpath %s", m[0])
		}
		for i := range n {
			dark[[2]int{x + i - 4, y - 4}] = true
		}
	}
	for y := -4; y < s.Size()+4; y++ {
		for x := -4; x < s.Size()+4; x++ {
			if dark[[2]int{x, y}] != s.Module(x, y) {
				t.Fatalf("module (%d, %d)", x, y)
			}
		}
	}
}

func TestOverlay(t *testing.T) {
	s := encode(t, "ZeRechnung", Options{Level: High})
	r := &Render{Scale: 4, Overlay: []Rect{
		{X: 0, Y: 0, W: 7, H: 7, Light: true}, // hides the finder pattern
		{X: 9.5, Y: 9.5, W: 1, H: 1},          // a dark square across four modules
		{X: 15, Y: 15, W: -1, H: 2},           // no area
		{X: -10, Y: 18, W: 100, H: 1},         // beyond the image, clipped
	}}
	img := s.Image(r)
	at := func(x, y float64) bool { return img.ColorIndexAt(int((x+4)*4), int((y+4)*4)) == 1 }
	if at(0, 0) || at(3, 3) || at(6.9, 6.9) {
		t.Error("the light rectangle does not hide the finder pattern")
	}
	if !at(9.5, 9.5) || !at(10.4, 10.4) {
		t.Error("the dark rectangle is not drawn")
	}
	if !at(-4, 18) || !at(float64(s.Size())+3.9, 18) {
		t.Error("the band is not drawn across the image")
	}
	if at(15, 15) != s.Module(15, 15) {
		t.Error("a rectangle of no area is drawn")
	}
}
