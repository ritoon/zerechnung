// Package atcud builds and checks the ATCUD and the QR code that the
// Portuguese invoices carry (Decreto-Lei n.º 28/2019, article 7.º), the
// ATCUD on every invoice since 1 January 2023, and draws the QR code.
//
// # ATCUD
//
// The ATCUD (código único do documento) is the validation code that the
// Autoridade Tributária e Aduaneira (AT) assigns to a series of documents
// when the series is communicated to it, a hyphen, and the sequential
// number of the document in the series: CSDF7T5H-0035 for the document
// FT AB2019/0035. Code holds it, New builds it from the validation code and
// the identification of the document, ParseCode reads and checks it. The
// rules are those of Portaria n.º 195/2020 of 13 August 2020, article 3.º:
// a validation code of eight characters at least, a sequential number made
// of the digits that follow the slash of the identification of the
// document.
//
// # QR code
//
// The content of the QR code follows the technical specifications of the
// AT, "Especificações técnicas — Código de barras bidimensional — Código
// QR", version 1.1 of October 2020, to which article 5.º of the Portaria
// refers: fields A to S, each its code, a colon and its value, separated
// by asterisks, in the order of the table of section 4, the amounts in
// euros with two decimals. QR holds them field by field. Build fills them
// from an invoice.Invoice and the data of the AT that the invoice does not
// carry (Data: validation code of the series, four characters of the
// signature, number of the certificate of the software), String writes the
// content, Parse reads a content and checks it, Check reports each field
// that does not follow the specification as a *FieldError named by the
// code of the field. HashChars extracts the four characters of the
// signature that field Q holds (Portaria n.º 363/2010, article 6.º).
//
// Symbol encodes the content in a qrcode.Symbol as section 2 of the
// specification requires: error correction level M, byte mode, version 9
// at least. The symbol is drawn by its PNG and SVG methods; placing it on
// the document is the job of the software that renders the invoice: in its
// body, 30 × 30 mm at least, on the first or the last page, with the ATCUD
// printed immediately above it as "ATCUD:" and the code (Portaria,
// articles 4.º and 6.º). The package does not write it in a PDF.
//
// The examples of section 5 of the specification are reproduced by the
// tests of the package. Examples 6 and 7, as the AT prints them, put a
// space after the colon of fields G and H, which section 3 a) forbids:
// Parse reports them.
//
// # Beyond the package
//
// The CIUS-PT does not carry the ATCUD nor the QR code in the XML (package
// validate checks its structured notes only). The signature of the
// documents (Portaria n.º 363/2010), the certification of the invoicing
// software, the communication of the series and of the invoices to the AT
// (e-fatura) and the SAF-T (PT) file are out of the package.
package atcud
