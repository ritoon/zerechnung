package qrbill

import "github.com/ritoon/zerechnung/qrcode"

// Symbol checks b and encodes its content in a QR Code symbol, as chapter
// 6 of the guidelines requires: error correction level M, byte mode, in
// the smallest version that holds it, 25 at most. Draw it with the Swiss
// cross over its center, with Render: printed, the symbol measures 46 ×
// 46 mm without its quiet zone, whatever its version (chapter 6.4), from
// a vector graphic such as the SVG drawing.
func (b *Bill) Symbol() (*qrcode.Symbol, error) {
	if err := b.Check(); err != nil {
		return nil, err
	}
	return qrcode.Encode([]byte(b.String()), qrcode.Options{Level: qrcode.Medium, MaxVersion: 25})
}

// Render returns the drawing of sym as the guidelines ask: the Swiss cross
// over its center (see Cross), a quiet zone of four modules (chapter
// 6.4.1), scale pixels or units per module. Pass it to the PNG, SVG or
// Image method of sym.
func Render(sym *qrcode.Symbol, scale int) *qrcode.Render {
	return &qrcode.Render{Scale: scale, QuietZone: 4, Overlay: Cross(sym.Size())}
}

// The geometry of the Swiss cross, in the units of the graphic
// CH-Kreuz_7mm.svg that SIX publishes in its Download Centre ("Black-White
// Cross for Swiss QR Code"): a square of 19.8 units for 7 mm; a black
// square from 0.7 to 19.1 under a white border drawn by a stroke of
// 1.4357 units on its edge, which leaves it black from 1.41785 to
// 18.38215; a white bar of 3.3 × 11 units at (8.3, 4) and one of 11 × 3.3
// units at (4.4, 7.9).
const (
	crossSide   = 19.8
	crossBlack  = 0.7 + 1.4357/2
	crossBar    = 3.3
	crossLength = 11
	crossBarX   = 8.3 // left side of the vertical bar
	crossBarY   = 4   // top of the vertical bar
	crossArmX   = 4.4 // left side of the horizontal bar
	crossArmY   = 7.9 // top of the horizontal bar
)

// Cross returns the Swiss cross that the guidelines put over the center of
// a symbol of size modules per side, as rectangles for the Overlay of a
// qrcode.Render: 7 × 7 mm on a symbol of 46 × 46 mm (chapter 6.4.2 and
// the Style Guide QR-bill, version 1.1), drawn as the graphic of SIX, a
// white cross on a black square with a white border. The error correction
// of level M restores the modules it hides.
func Cross(size int) []qrcode.Rect {
	n := float64(size)
	u := n * 7 / 46 / crossSide // modules per unit of the graphic
	o := n/2 - crossSide/2*u    // top left corner of the cross
	return []qrcode.Rect{
		{X: o, Y: o, W: crossSide * u, H: crossSide * u, Light: true},
		{X: o + crossBlack*u, Y: o + crossBlack*u, W: (crossSide - 2*crossBlack) * u, H: (crossSide - 2*crossBlack) * u},
		{X: o + crossBarX*u, Y: o + crossBarY*u, W: crossBar * u, H: crossLength * u, Light: true},
		{X: o + crossArmX*u, Y: o + crossArmY*u, W: crossLength * u, H: crossBar * u, Light: true},
	}
}
