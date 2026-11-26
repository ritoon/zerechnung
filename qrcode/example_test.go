package qrcode_test

import (
	"bytes"
	"fmt"

	"github.com/ritoon/zerechnung/qrcode"
)

// This example draws a symbol in PNG, with a light square over its center
// and a dark one inside, as the Swiss QR-bill draws its cross: the error
// correction restores the modules they hide.
func ExampleSymbol_PNG() {
	sym, err := qrcode.Encode([]byte("SPC\n0200\n1\nCH4431999123000889012"), qrcode.Options{Level: qrcode.Medium})
	if err != nil {
		panic(err)
	}
	c := float64(sym.Size()) / 2
	var png bytes.Buffer
	err = sym.PNG(&png, &qrcode.Render{Scale: 8, Overlay: []qrcode.Rect{
		{X: c - 3, Y: c - 3, W: 6, H: 6, Light: true},
		{X: c - 2.5, Y: c - 2.5, W: 5, H: 5},
	}})
	if err != nil {
		panic(err)
	}
	fmt.Printf("version %d-%s, %d modules, %d × %d pixels\n", sym.Version(), sym.Level(), sym.Size(), 8*(sym.Size()+8), 8*(sym.Size()+8))
	// Output: version 3-M, 29 modules, 296 × 296 pixels
}
