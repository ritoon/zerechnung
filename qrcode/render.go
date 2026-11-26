package qrcode

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"strconv"
)

// Render sets how a symbol is drawn. A nil *Render draws it with the
// defaults: 4 pixels or units per module, a quiet zone of 4 modules.
type Render struct {
	// Scale is the side of a module: in pixels for Image and PNG, in
	// user units for the width and height of the SVG drawing. Zero
	// stands for 4.
	Scale int
	// QuietZone is the width of the light margin around the symbol, in
	// modules. Zero stands for 4, the minimum of ISO/IEC 18004; a
	// negative value draws no quiet zone.
	QuietZone int
	// Overlay are rectangles drawn over the symbol, in this order, such as
	// the Swiss cross at the center of the QR-bill.
	Overlay []Rect
}

// Rect is a rectangle drawn over a symbol. Its position and its size are
// in modules, from the top left corner of the symbol, the quiet zone left
// out, and may be fractional: Image and PNG round them to the pixel. A
// rectangle of no area draws nothing.
type Rect struct {
	X, Y, W, H float64
	// Light draws the rectangle in the light colour of the symbol, white;
	// otherwise it is drawn in the dark colour, black.
	Light bool
}

// params returns the scale, the quiet zone and the overlay of r, the
// defaults filled in.
func (r *Render) params() (scale, quiet int, overlay []Rect) {
	scale, quiet = 4, 4
	if r == nil {
		return scale, quiet, nil
	}
	if r.Scale > 0 {
		scale = r.Scale
	}
	switch {
	case r.QuietZone > 0:
		quiet = r.QuietZone
	case r.QuietZone < 0:
		quiet = 0
	}
	return scale, quiet, r.Overlay
}

// palette holds the two colours of a symbol: light at index 0, so that a
// new image is light, and dark at index 1.
var palette = color.Palette{color.Gray{Y: 0xFF}, color.Gray{Y: 0}}

// Image draws the symbol, its quiet zone and the overlay of r in a
// two-colour image, white and black, Scale pixels per module.
func (s *Symbol) Image(r *Render) *image.Paletted {
	scale, quiet, overlay := r.params()
	n := s.c.Size()
	side := (n + 2*quiet) * scale
	img := image.NewPaletted(image.Rect(0, 0, side, side), palette)
	for y := range n {
		for x := range n {
			if s.c.Module(x, y) {
				fill(img, (x+quiet)*scale, (y+quiet)*scale, (x+quiet+1)*scale, (y+quiet+1)*scale, 1)
			}
		}
	}
	px := func(v float64) int { return int(math.Round((v + float64(quiet)) * float64(scale))) }
	for _, o := range overlay {
		if !(o.W > 0 && o.H > 0) {
			continue
		}
		var c uint8 = 1
		if o.Light {
			c = 0
		}
		fill(img, px(o.X), px(o.Y), px(o.X+o.W), px(o.Y+o.H), c)
	}
	return img
}

// fill sets the pixels of [x0, x1) × [y0, y1) to the colour of index c,
// within the bounds of img.
func fill(img *image.Paletted, x0, y0, x1, y1 int, c uint8) {
	b := img.Bounds()
	x0, y0 = max(x0, b.Min.X), max(y0, b.Min.Y)
	x1, y1 = min(x1, b.Max.X), min(y1, b.Max.Y)
	if x0 >= x1 {
		return
	}
	for y := y0; y < y1; y++ {
		row := img.Pix[img.PixOffset(x0, y):img.PixOffset(x1, y)]
		for i := range row {
			row[i] = c
		}
	}
}

// PNG writes the image that Image draws to w, in the PNG format.
func (s *Symbol) PNG(w io.Writer, r *Render) error {
	return png.Encode(w, s.Image(r))
}

// SVG writes the symbol, its quiet zone and the overlay of r to w as an SVG
// drawing: a white square, the dark modules as a single black path, and
// the rectangles of the overlay. The view box counts one unit per module,
// and the width and height are Scale units per module, without a unit of
// length: whoever places the drawing on the document sets its size.
func (s *Symbol) SVG(w io.Writer, r *Render) error {
	scale, quiet, overlay := r.params()
	n := s.c.Size()
	side := n + 2*quiet
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges">`,
		side, side, side*scale, side*scale)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, side, side)
	// One subpath per run of dark modules in a row.
	for y := range n {
		for x := 0; x < n; {
			if !s.c.Module(x, y) {
				x++
				continue
			}
			x0 := x
			for x < n && s.c.Module(x, y) {
				x++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x0+quiet, y+quiet, x-x0, x-x0)
		}
	}
	b.WriteString(`"/>`)
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	for _, o := range overlay {
		if !(o.W > 0 && o.H > 0) {
			continue
		}
		colour := "#000"
		if o.Light {
			colour = "#fff"
		}
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`,
			num(o.X+float64(quiet)), num(o.Y+float64(quiet)), num(o.W), num(o.H), colour)
	}
	b.WriteString("</svg>\n")
	_, err := w.Write(b.Bytes())
	return err
}
