// Package qr encodes QR Code symbols (ISO/IEC 18004, Model 2, versions 1 to
// 40), for the tax and payment codes that some countries require on an
// invoice: the ATCUD code in Portugal, the Swiss QR-bill, the EPC payment
// code, VERI*FACTU in Spain...
//
// The package is a trimmed copy of the encoder of github.com/piglig/go-qr
// v1.1.0, distributed under the MIT License (Copyright (c) 2023 piglig, see
// the LICENSE file of this directory). The changes:
//
//   - only the encoder is kept: no rendering, decoding, Kanji mode or
//     optimal segmentation;
//   - the error correction level is used as given, never raised: the
//     specifications fix it, level M in most of them;
//   - the version range and the byte mode can be imposed, as the
//     Portuguese specification requires version 9 or above in byte mode;
//   - the Reed-Solomon encoder is inlined and the internal errors that the
//     validation of the arguments makes impossible are gone.
//
// The mask is chosen by the penalty rules of ISO/IEC 18004, among the eight
// patterns. A Code is the bare symbol: package qrcode, the public face of
// the encoder, draws it with its quiet zone.
package qr
