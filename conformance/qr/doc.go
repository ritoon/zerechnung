// Package qr has the symbols of package internal/qr read by two QR Code
// decoders: gozxing, a port of ZXing (Apache-2.0), which shares no code
// with the encoder, and the decoder of github.com/piglig/go-qr (MIT), the
// module the encoder comes from, which reports the version, the mask and
// the segments it reads. It is a separate module, so that neither becomes a
// dependency of package zerechnung.
package qr
