// Package qrcode encodes and draws the QR Code symbols (ISO/IEC 18004,
// Model 2) that some countries require on an invoice: the QR code of the
// Portuguese invoices (package atcud), and later the Swiss QR-bill, the EPC
// payment code or the VERI*FACTU code of Spain.
//
// Encode puts data in a symbol in byte mode, the mode those specifications
// ask for, at the error correction level and within the versions they fix.
// The encoder is package internal/qr, a trimmed copy of the encoder of
// github.com/piglig/go-qr v1.1.0 (MIT License).
//
// A Symbol is the matrix of its modules, which Module reads one by one.
// Image, PNG and SVG draw it with a quiet zone, 4 modules wide unless
// Render sets another width. Render.Overlay draws rectangles over the
// symbol, as the Swiss cross that the QR-bill puts at the center of its
// code: the error correction restores the modules they hide, as long as
// they hide few enough of them for the level of the symbol.
//
// The package draws the symbol, nothing else: placing it on the invoice, at
// the size the specification requires, is the job of the software that
// renders the document.
package qrcode
