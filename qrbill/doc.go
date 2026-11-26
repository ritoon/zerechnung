// Package qrbill builds, reads and checks the content of the Swiss QR Code
// of the QR-bill, the payment part of the invoices of Switzerland and
// Liechtenstein, and draws the symbol with its Swiss cross.
//
// The rules are those of the Swiss Implementation Guidelines for the
// QR-bill of SIX Interbank Clearing: version 2.4 of 24 February 2026, in
// force from 14 November 2026, and version 2.3 of 20 November 2023, which
// remains valid until November 2027. Both write the same content, version
// 0200, and differ in one rule: since version 2.4 the QR-IBAN and the QR
// reference are for invoices in CHF only. Bill.Guidelines selects the
// version that Check applies, 2.4 unless set to Guidelines23.
//
// # Content
//
// Bill holds the content line by line, as table 8 of chapter 4.2.2
// defines it: the account (IBAN or QR-IBAN), the creditor, the amount and
// the currency (CHF or EUR), the ultimate debtor, the type of reference
// (QRR, SCOR or NON) and the reference, the unstructured message, the
// billing information and the alternative procedures. Build fills it from
// an invoice.Invoice and the data that the invoice does not carry (Data),
// String writes the content, Parse reads a content and checks it, Check
// reports each line that breaks the guidelines as a *FieldError, named by
// its number and its element. The addresses are structured (type S), the
// only type since version 2.3.
//
// NewQRReference builds a QR reference, 26 digits and their check digit
// modulo 10 recursive (Annex B); NewCreditorReference builds a Creditor
// Reference of ISO 11649; IsQRIBAN tells a QR-IBAN, whose institution
// identification is between 30000 and 31999. S1 writes and ParseS1 reads
// the billing information in the syntax S1 of SIX (formerly Swico),
// version 1.2, which Annex D reproduces.
//
// The examples of Annex A of version 2.4 are reproduced by the tests of
// the package, and their symbols are read by two decoders in the tests of
// conformance/qr.
//
// # Symbol
//
// Symbol encodes the content in a qrcode.Symbol as chapter 6 requires:
// error correction level M, byte mode, version 25 at most. Render returns
// how to draw it, with the Swiss cross over its center (Cross: 7 × 7 mm
// on 46 × 46 mm, the geometry of the graphic of SIX) and a quiet zone of
// four modules, for the PNG and SVG methods of the symbol.
//
// # Beyond the package
//
// The package returns the content and the symbol; the layout of the
// payment part and the receipt (chapter 3 and the Style Guide QR-bill:
// 210 × 105 mm, headings in the language of the invoice, the symbol 46 ×
// 46 mm, the perforation) stays with the software that renders the
// invoice, and the HTTP server does not draw it in its PDF. eBill, the
// alternative procedures beyond their parameters, and the transmission of
// payments to the banks (pain.001) are out of the package.
package qrbill
