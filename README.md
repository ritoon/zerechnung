# ZeRechnung

[![Go Reference](https://pkg.go.dev/badge/github.com/ritoon/zerechnung.svg)](https://pkg.go.dev/github.com/ritoon/zerechnung)
[![CI](https://github.com/ritoon/zerechnung/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ritoon/zerechnung/actions/workflows/ci.yml)
[![Conformance](https://github.com/ritoon/zerechnung/actions/workflows/conformance.yml/badge.svg?branch=main)](https://github.com/ritoon/zerechnung/actions/workflows/conformance.yml)
[![Reference parity](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fritoon%2Fzerechnung%2Fbadges%2Fbadge-parity.json)](#how-accurate-is-it)
[![Go Report Card](https://goreportcard.com/badge/github.com/ritoon/zerechnung)](https://goreportcard.com/report/github.com/ritoon/zerechnung)
[![codecov](https://codecov.io/gh/ritoon/zerechnung/graph/badge.svg)](https://codecov.io/gh/ritoon/zerechnung)
[![Go version](https://img.shields.io/github/go-mod/go-version/ritoon/zerechnung)](go.mod)
[![MIT license](https://img.shields.io/github/license/ritoon/zerechnung)](LICENSE)

<p align="center">
  <img src="docs/logo.png" alt="ZeRechnung, Go package for Factur-X and ZUGFeRD: the Go gopher reads an invoice next to a typewriter, a paper boat sits on the n, and a stack of invoices is marked FR and DE">
</p>

ZeRechnung is a Go package for e-invoices in the Factur-X, ZUGFeRD and XRechnung formats.

Keep generating your invoice PDFs the way you do today. ZeRechnung adds the XML invoice and the metadata the standard requires, and the PDF becomes a compliant e-invoice. It also reads and validates the invoices you receive, whether they come as hybrid PDFs or as plain XML (CII or UBL), signs hybrid PDFs in PAdES and verifies their signatures. It reads, writes, validates, signs and verifies FatturaPA, the national format of the Italian SDI, and Facturae, the Spanish format of FACe and of the B2B reform. For the French reform, it also reads, writes and validates the statuses of the life cycle of an invoice (CDAR). It reads, writes and validates the reports of the French e-reporting (flux 10), the transactions and payments that are not e-invoiced. For Portugal, it builds and checks the ATCUD and the QR code that every invoice carries, and draws the symbol in PNG or SVG. For Switzerland, it builds, reads and checks the Swiss QR Code of the QR-bill, and draws it with its Swiss cross.

- Pure Go, standard library only. No CGO, no Java, no external binary, no network calls.
- All Factur-X / ZUGFeRD 2.x profiles, from MINIMUM to EXTENDED, plus XRechnung, Peppol BIS Billing 3.0, NLCIUS, CIUS-IT and CIUS-RO (Romania, RO e-Factura), in CII and UBL 2.1, CIUS-PT and CIUS-HR (Croatia, Fiskalizacija 2.0) in UBL 2.1, and the Slovak eFaktúra (Peppol BIS with the requirements of the Finančná správa).
- Validation compared with the reference validators (KoSIT, Mustang) on every pull request, with the French 2026 reform, German, Dutch, Italian, Portuguese, Romanian, Croatian, Slovak and Peppol rules built in.
- PDF/A-3 conversion of the PDFs that need no PDF engine for it, with the reasons for a refusal otherwise, checked by veraPDF in the CI.
- Fast and light: reading and validating a typical invoice takes about 25 µs and allocates 23 KB.

> [!WARNING]
> **ZeRechnung is still under development and is not ready for production.** It is published as an example only.
>
> Its main goal is to cover the European countries. It may not cover every case, neither in the standards it supports nor in its features.
>
> The project follows [semantic versioning](https://semver.org). Until v1, the API may change from one release to the next. From v1 on, we will consider the package stable enough for you to use it outside our own projects.
>
> Feedback from the community is welcome: open an [issue](https://github.com/ritoon/zerechnung/issues) with your comments on the project.

## Why this project

ZeRechnung is not meant to make VAT carousel fraud easier. Its purpose is to make e-invoicing interoperable between the countries of Europe.

The European Union is supposed to be a free-trade area. Yet a company that invoices a customer in a neighbouring country meets a different format, a different national CIUS, a different platform and a different set of rules at each border. Even with a common European standard, EN 16931, lawmakers have turned the single market into a viscous one, constrained by norms and rules whose only justification seems to be the working time and the posts of the people who write them. ZeRechnung tries to absorb that complexity so that businesses can exchange invoices across borders without having to master every national variant.

Europe would gain from one universal invoice format, accepted as is in every country. It would make trade between countries simpler and would need far fewer government employees whose only role is to complicate things.

## Install

```bash
go get github.com/ritoon/zerechnung
```

ZeRechnung needs Go 1.24 or later.

## Quick start

### Turn your PDF into a Factur-X invoice

Describe the invoice with the `invoice` package, then hand it to `Create` together with the PDF your application already renders:

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	host, err := os.ReadFile("invoice.pdf") // rendered by your app, ideally as PDF/A-3
	if err != nil {
		log.Fatal(err)
	}

	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:   invoice.ProfileEN16931,
		Number:    "INV-2026-0042",
		TypeCode:  invoice.TypeCommercialInvoice,
		IssueDate: invoice.NewDate(2026, time.October, 15),
		Currency:  "EUR",
		Seller: invoice.Party{
			Name:    "Acme Software SAS",
			VATID:   "FR32123456789",
			Address: invoice.Address{City: "Paris", PostCode: "75001", CountryCode: "FR"},
		},
		Buyer: invoice.Party{
			Name:    "Example Client SARL",
			Address: invoice.Address{City: "Lyon", PostCode: "69002", CountryCode: "FR"},
		},
		PaymentTerms: []invoice.PaymentTerms{{DueDate: invoice.NewDate(2026, time.November, 14)}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Support contract, October 2026"},
			Quantity:        d("2"),
			UnitCode:        "DAY",
			NetPrice:        d("450.00"),
			VATCategoryCode: invoice.VATStandard,
			VATRate:         d("20.00"),
		}},
	}

	pdf, err := zerechnung.Create(host, inv, &zerechnung.CreateOptions{
		Calculate: true,                // fill in line amounts, totals and the VAT breakdown
		Validate:  &validate.Options{}, // refuse to produce an invalid invoice
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("invoice-facturx.pdf", pdf, 0o644); err != nil {
		log.Fatal(err)
	}
}
```

A few things happen behind that call:

- `Calculate` works out the amounts the standard derives from the lines (here 900.00 net, 180.00 VAT, 1080.00 due), using exact decimals.
- With `Validate`, `Create` checks the XML first and returns a `*zerechnung.ValidationError` with the full report if anything is wrong. Nothing is written in that case.
- The PDF is changed with an incremental update. Your original bytes stay untouched, so an existing signature remains valid, though it does not cover the invoice: [sign after embedding](#sign-a-hybrid-pdf-in-pades).

Your PDF should already be PDF/A-3 (or PDF/A-4 for PDF 2.0), which most PDF generators can do with an option. If yours cannot, ZeRechnung can [convert it to PDF/A-3](#convert-a-pdf-to-pdfa-3) when that needs no PDF engine, and tells you why otherwise. The [Creating invoices](https://github.com/ritoon/zerechnung/wiki/Creating-invoices) page explains what the package writes and how to refuse a PDF that is not PDF/A.

### Read an invoice you received

```go
inv, _, err := zerechnung.Read(pdf)
if err != nil {
	log.Fatal(err)
}
fmt.Println(inv.Number, inv.Seller.Name, inv.Totals.DuePayable, inv.Currency)
for _, line := range inv.Lines {
	fmt.Println(line.Item.Name, line.Quantity, line.NetAmount)
}
```

```text
INV-2026-0042 Acme Software SAS 1080.00 EUR
Support contract, October 2026 2 900.00
```

`Read` accepts Factur-X and every ZUGFeRD version, including 1.0. Data the model has no field for stays in the invoice: `inv.Unmodeled()` lists it, and writing the invoice back puts it in place. More in [Reading invoices](https://github.com/ritoon/zerechnung/wiki/Reading-invoices).

### Validate an invoice

```go
report, _, err := zerechnung.Check(pdf, &validate.Options{Auto: true})
if err != nil {
	log.Fatal(err) // the file is not a readable PDF
}
if !report.Valid() {
	for _, f := range report.Errors() {
		fmt.Println(f)
	}
}
```

`Auto: true` adds the national rules of the seller's country. The seller above is in France, so the 2026 reform rules apply, and they find what the minimal invoice is missing:

```text
...
error [BR-FR] BR-FR-10_BT-30: BR-FR-10/BT-30 : le SIREN du vendeur (ram:ID avec schemeID 0002) est obligatoire ...
error [BR-FR] BR-FR-12_BT-49: BR-FR-12/BT-49 : l'adresse électronique de l'acheteur (BT-49) est obligatoire ...
error [BR-FR] BR-FR-13_BT-34: BR-FR-13/BT-34 : l'adresse électronique du vendeur (BT-34) est obligatoire ...
```

Each finding carries the official rule ID, its severity, the XPath of the element and its line. Messages come straight from the official artefacts, so the French rules speak French. A complete French invoice that passes these rules is in [example_test.go](example_test.go).

To pick the rules yourself:

| To check | Use |
|---|---|
| The standard: schema, EN 16931, Factur-X profile rules | `nil` or `&validate.Options{}` |
| The national rules of the seller's country | `&validate.Options{Auto: true}` |
| The French 2026 reform | `Rules: validate.Default \| validate.France` |
| XRechnung (BR-DE) | `Rules: validate.Default \| validate.Germany` |
| French public sector (Chorus Pro) | `Rules: validate.Default \| validate.France \| validate.ChorusPro` |
| Peppol BIS Billing 3.0 or NLCIUS | `validate.Peppol`, `validate.Netherlands` |
| CIUS-IT, the Italian CIUS of EN 16931 (BR-IT) | `validate.Italy` |
| CIUS-PT, the Portuguese CIUS of EN 16931 (BR-CIUS-PT, DT-CIUS-PT) | `validate.Portugal` |
| CIUS-RO, the Romanian CIUS of EN 16931 (BR-RO, BR-DEC-RO) | `validate.Romania` |
| The Slovak eFaktúra, beyond Peppol BIS (SK-*) | `validate.Slovakia`, or `Auto` for a seller in Slovakia |
| CIUS-HR, the Croatian CIUS of EN 16931 and its extension (HR-BR) | `validate.Croatia` |
| An XML file instead of a PDF | `validate.XML(xml, opts)` |
| A French status of the life cycle (CDAR, BR-FR-CDV) | `validate.XML(xml, nil)`: the rules apply whatever the options |
| A French e-reporting report (flux 10, schematron Flux10) | `validate.XML(xml, nil)`: the rules apply whatever the options |

XRechnung, Peppol, NLCIUS, CIUS-IT, CIUS-PT, CIUS-RO and CIUS-HR invoices declare themselves, so their rules apply without any option. A Slovak invoice declares Peppol BIS like any other: the Peppol rules apply to it, and `Auto` adds the Slovak requirements for a seller in Slovakia. All the options and rule sets are described in [Validating invoices](https://github.com/ritoon/zerechnung/wiki/Validating-invoices).

### Read and validate at once

```go
inv, report, _, err := zerechnung.ReadAndCheck(pdf, &validate.Options{Auto: true})
```

`ReadAndCheck` reads the PDF and parses the XML once, where `Read` followed by `Check` does both twice: the invoice and the error are those of `Read`, the report that of `Check`. For an XML file, `validate.Parse(xml, opts)` does the same with the `Parse` function of its syntax (`cii`, `ubl`, `fatturapa` or `facturae`) and `validate.XML`. On the 13 KB example invoice, reading and validating the XML takes 30 µs instead of 40 µs; with `PDFA: validate.PDFABasic`, reading and checking the 149 KB PDF takes 87 µs instead of 131 µs.

### Work with the XML directly

The `cii`, `ubl`, `fatturapa` and `facturae` packages read and write the XML with the same `invoice.Invoice` model, so converting between syntaxes is a parse and a marshal:

```go
// Write the CII XML for a given profile.
xml, err := cii.Marshal(inv, &cii.MarshalOptions{Profile: invoice.ProfileBasic})

// Convert a UBL invoice to CII.
fromUBL, err := ubl.Parse(ublXML)
ciiXML, err := cii.Marshal(fromUBL, nil)

// Convert a FatturaPA invoice to UBL, with its Italian data in the
// extensions of a domestic invoice (EXT-IT), and a CII invoice to FatturaPA.
fromSDI, err := fatturapa.Parse(sdiXML)
ublXML, err = ubl.Marshal(fromSDI, nil)
sdiXML, err = fatturapa.Marshal(inv, nil)

// Convert a Facturae invoice to UBL, and a UBL invoice to Facturae 3.2.2.
fromFACe, err := facturae.Parse(faceXML)
ublXML, err = ubl.Marshal(fromFACe, nil)
faceXML, err = facturae.Marshal(fromUBL, nil)

// Embed an XML invoice that another system produced.
pdf, err := zerechnung.Embed(host, xml, nil)
```

## More examples

### Attach supporting documents

```go
pdf, err := zerechnung.Embed(host, xml, &zerechnung.EmbedOptions{
	Attachments: []zerechnung.File{
		{Name: "order-4512.pdf", Data: order, Description: "Purchase order 4512"},
		{Name: "timesheet.csv", Data: timesheet}, // MIME type guessed from the extension
	},
})
```

### Refuse a PDF that is not PDF/A

```go
pdf, err := zerechnung.Create(host, inv, &zerechnung.CreateOptions{
	Embed: zerechnung.EmbedOptions{RequirePDFA: true},
})
if errors.Is(err, zerechnung.ErrNotPDFA) {
	// the host PDF does not declare PDF/A-2, PDF/A-3 or PDF/A-4
}
```

### Convert a PDF to PDF/A-3

```go
pdf, err := zerechnung.Embed(host, xml, &zerechnung.EmbedOptions{ConvertPDFA: true})
var ce *zerechnung.ConversionError
if errors.As(err, &ce) { // wraps zerechnung.ErrNotConvertible
	for _, r := range ce.Reasons {
		log.Println(r) // page 1 /Contents: the font "Helvetica" (Type1) shows text but its font program is not embedded…
	}
}
```

`ConvertPDFA` (or `EmbedOptions.ConvertPDFA`, for a host that does not declare PDF/A) rewrites a PDF 1.x file as PDF/A-3, in pure Go and without rendering it. It adds a PDF/A output intent (an sRGB ICC profile computed by the package, unless the PDF has one), declares PDF/A-3 in the XMP metadata, removes JavaScript, forbidden actions and other entries PDF/A does not allow, re-encodes LZW streams with Flate, cleans graphics states, images and annotations, and completes the embedded files. It refuses, with every reason and where it was found, what it cannot fix without a PDF engine: a font that shows visible text without being embedded, a glyph its font program lacks or whose width differs, DeviceCMYK without a CMYK profile, a JPEG 2000 image PDF/A does not allow, a hidden annotation or one without an appearance, a signed document, PDF 2.0. The converted file must then pass the PDF/A checks of `Check`, and the CI runs veraPDF on the converted files (see [How accurate is it?](#how-accurate-is-it)).

### Show what is wrong when Create refuses an invoice

```go
var verr *zerechnung.ValidationError
if errors.As(err, &verr) {
	for _, f := range verr.Report.Errors() {
		fmt.Println(f.Rule, f.Message, f.Path)
	}
}
```

### Check an invoice for Chorus Pro

```go
report := validate.XML(xml, &validate.Options{
	Rules:       validate.Default | validate.France | validate.ChorusPro,
	PublicBuyer: validate.PublicBuyer{ServiceCodeRequired: true, EngagementRequired: true},
})
```

### Check many invoices on a server

The invoices of one issuer come from the same software and usually embed the same fonts. A `Checker` keeps the font programs it reads, so that the next PDF that embeds them skips their decompression, about half the time of `Check`. Create one and share it between requests: it is safe for concurrent use.

```go
var checker = zerechnung.NewChecker(&zerechnung.CheckerOptions{FontCache: 32 << 20}) // 32 MiB of decoded fonts

func handle(pdf []byte) (*validate.Report, error) {
	report, _, err := checker.Check(pdf, &validate.Options{Auto: true})
	return report, err
}
```

A font is found by the SHA-256 of its stream, so the findings are those of `Check`. From the second PDF with the same fonts, the full check of the 149 KB example takes about 300 µs instead of 530 µs.

### Write an e-reporting report (France)

From September 2026 for large and mid-size companies, September 2027 for the others, the French reform asks the companies to report to the tax administration, through their platform, the transactions that are not e-invoiced (with consumers, with companies established abroad) and the payments of services. The `ereporting` package reads and writes these reports (flux 10 of the PPF specifications), and `validate.XML` checks them against the schemas of France_RFE and the rules of the PPF on their content:

```go
rep := &ereporting.Report{
	Document: ereporting.Document{ID: "TR-2026-09-0001", IssueDateTime: time.Now(), TypeCode: "IN",
		Sender: ereporting.Party{ID: "0042", SchemeID: "0238", Name: "Plateforme agréée", RoleCode: "WK"},
		Issuer: ereporting.Party{ID: "123456782", SchemeID: "0002", Name: "Fromagerie du Val", RoleCode: "SE"}},
	Transactions: &ereporting.TransactionsReport{
		Period: ereporting.Period{Start: invoice.NewDate(2026, 9, 1), End: invoice.NewDate(2026, 9, 30)},
		// Invoices: the invoices to foreign companies; Transactions: the daily totals of the sales to consumers.
	},
}
xml, err := ereporting.Marshal(rep, &ereporting.MarshalOptions{Indent: true})
report := validate.XML(xml, nil) // the XSD-* rules, ER-01 and the 150 rules of the schematron Flux10 (F10-*)
```

### Declare VAT on debits (France)

```go
inv.TaxPointDateCode = invoice.TaxPointDebits
```

### Report the status of an invoice (France)

In the French reform, the platforms exchange the statuses of the life cycle of each invoice (Déposée, Rejetée, Refusée, Encaissée…) as CDAR messages, and send them to the PPF. The `cdar` package reads, writes and builds them; `validate.XML` checks them against the CDAR D22B schema and the BR-FR-CDV rules:

```go
// The status 200 Déposée of an invoice the platform accepted, for the seller and the PPF.
status, err := cdar.Deposited(inv, &cdar.BuildOptions{Platform: cdar.Party{IDs: []invoice.Identifier{{ID: "0003", Scheme: cdar.SchemePlatform}}, Name: "PA-E Vendeur"}})

// Or the status 213 Rejetée, one reason REJ_SEMAN per error of the validation report.
status, err = cdar.Rejected(inv, report, nil)

xml, err := cdar.Marshal(status, &cdar.MarshalOptions{Indent: true})
fmt.Println(validate.XML(xml, nil).Valid()) // true

// On reception: the referenced invoice, its status and the reasons.
msg, err := cdar.Parse(xml)
```

### Sign a hybrid PDF in PAdES

`Sign` adds a PAdES signature (ETSI EN 319 142-1) to a PDF, as an incremental update that keeps PDF/A conformance. Sign once the invoice is embedded, so that the signature covers it, or set `EmbedOptions.Sign` to do both in one call. The key only signs a digest, and nothing is fetched: the time-stamp tokens come from the `Timestamper` you give, which sends the RFC 3161 request to your time-stamping authority.

```go
signed, err := zerechnung.Sign(hybrid, &zerechnung.SignOptions{
	Signer: key, Chain: []*x509.Certificate{cert, issuingCA},
	Reason: "Invoice", Location: "Paris",
	Timestamp: tsa, // optional: a signature time-stamp makes it B-T
})
// Long-term validation: the chain and the revocation data (B-LT), then a
// document time-stamp over them (B-LTA).
signed, err = zerechnung.AddValidationData(signed, &zerechnung.ValidationData{Certificates: chain, OCSP: responses})
signed, err = zerechnung.AddDocumentTimestamp(signed, tsa, 0)

// On reception: Check reports the signatures under SIG-01 to SIG-14, and
// VerifySignatures describes them (signer, seal or signature, qualified,
// PAdES level, what changed after each one).
report, sigs, err := zerechnung.VerifySignatures(signed, &validate.TrustOptions{Roots: qualifiedCAs})
```

`SignOptions.Certify` certifies the document instead (DocMDP). `Embed` refuses a certified PDF (`ErrCertified`), since its certification permits no embedded file; into a PDF with an approval signature it embeds the invoice, which the signature then does not cover, and `Check` warns about it (SIG-05).

### Sign and verify a FatturaPA file

The SDI accepts FatturaPA files signed in CAdES (`.xml.p7m`) or XAdES, and requires the signature for invoices to public administrations. The key only signs a digest, so it can stay in a smart card, an HSM or a remote signing service:

```go
opts := &fatturapa.SignOptions{Signer: key, Chain: []*x509.Certificate{cert, issuingCA}}
p7m, err := fatturapa.SignCAdES(sdiXML, opts) // or SignXAdES
name, err := fatturapa.FileName("IT", "01234567890", "00001", true) // IT01234567890_00001.xml.p7m

// On reception: the document, once every signature verifies.
doc, sig, err := fatturapa.Unwrap(p7m)

// The checks of the SDI, under its error codes (00100 expired, 00102 invalid, 00104 untrusted...).
report := fatturapa.VerifySignature(p7m, &fatturapa.VerifyOptions{Roots: qualifiedCAs, OCSP: responses, Name: name})
```

### Sign and verify a Facturae invoice

FACe only receives signed Facturae invoices, and the Spanish B2B reform asks for an advanced electronic signature too (Real Decreto 238/2026, article 7.3). `facturae.Sign` writes the XAdES-EPES signature of the signature policy of Facturae v3.1, enveloped in the document; the key can stay in its device here too:

```go
signed, err := facturae.Sign(faceXML, &facturae.SignOptions{Signer: key, Chain: []*x509.Certificate{cert, issuingCA}})

// On reception: who signed, when, as what, under which policy, once the signature verifies.
sig, err := facturae.ReadSignature(signed)

// The policy, the signing time, the role and the certificate (ES-SIG-01 not signed, ES-SIG-03 policy, ES-SIG-09 untrusted...).
report := facturae.VerifySignature(signed, &facturae.VerifyOptions{Roots: qualifiedCAs, OCSP: responses})
```

### Print the ATCUD and the QR code (Portugal)

Every Portuguese invoice carries an ATCUD, the validation code of its series and its sequential number, and a QR code whose content the tax authority (AT) defines field by field. The `atcud` package builds both from the invoice and the data of the AT, checks those of the invoices you receive, and returns the symbol: placing it on the document stays with the software that renders it.

```go
q, err := atcud.Build(inv, &atcud.Data{
	ValidationCode: "ZRSERIE7", // returned by the AT for the series of the invoice
	Hash:           "Zr4q",     // or atcud.HashChars(signature)
	Certificate:    "9999",     // the certificate number of your invoicing software
})
fmt.Println("ATCUD:" + q.ATCUD) // ATCUD:ZRSERIE7-41, printed right above the QR code
sym, err := q.Symbol()          // level M, byte mode, version 9 at least
err = sym.SVG(w, nil)           // or sym.PNG(w, &qrcode.Render{Scale: 8}); 30 × 30 mm at least on the page

// On reception: every field that breaks the specification, named by its code (A to S).
q, err = atcud.Parse(content)
```

The `qrcode` package draws the symbol, with rectangles over it if needed, such as the cross of the Swiss QR-bill.

### Print the Swiss QR Code (Switzerland)

The QR-bill is the payment part of the invoices of Switzerland and Liechtenstein: its Swiss QR Code holds the account, the creditor, the amount, the reference and the debtor, as the Implementation Guidelines of SIX define them. The `qrbill` package builds the content from the invoice, checks the codes you receive line by line, and returns the symbol with its Swiss cross: the layout of the payment part and the receipt stays with the software that renders the invoice.

```go
ref, err := qrbill.NewQRReference("2026123")                 // 26 digits and their check digit, for a QR-IBAN
bill, err := qrbill.Build(inv, &qrbill.Data{Reference: ref}) // or a Creditor Reference (RF…) with an IBAN
sym, err := bill.Symbol()                                    // level M, version 25 at most
err = sym.SVG(w, qrbill.Render(sym, 4))                      // with the Swiss cross; 46 × 46 mm on the page

// On reception: every line that breaks the guidelines, named by its number and element.
bill, err = qrbill.Parse(content, qrbill.Guidelines24) // or qrbill.Guidelines23, valid until November 2027
```

The runnable examples are in [example_test.go](example_test.go) and on [pkg.go.dev](https://pkg.go.dev/github.com/ritoon/zerechnung).

## Command line

```bash
go install github.com/ritoon/zerechnung/cmd/zerechnung@latest
```

```text
$ zerechnung info testdata/EN16931_Einfach.pdf
PDF version:        1.4
Attachment:         factur-x.xml (13396 bytes, text/xml)
AFRelationship:     Alternative (in /AF: true)
Modification date:  2022-02-18T17:13:44+01:00
PDF/A:              3B
XMP namespace:      urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#
XMP profile:        EN 16931 (document INVOICE, file factur-x.xml, version 1.0)
Profile (BT-24):    EN 16931
Invoice:            471102, type 380, issued 2018-03-05
Seller:             Lieferant GmbH (DE)
Buyer:              Kunden AG Mitte (DE)
Total:              473.00 EUR without VAT, 529.87 with VAT, 529.87 due
Lines:              2

$ zerechnung validate testdata/EN16931_Einfach.pdf
Profile: EN 16931
warning [PDF] PDF-06: the catalog has no PDF/A output intent (/S /GTS_PDFA1) (catalog /OutputIntents)
0 error(s), 1 warning(s)
```

| Command | What it does |
|---|---|
| `zerechnung info invoice.pdf` | Summarises the PDF and the invoice |
| `zerechnung extract [-o out.xml] invoice.pdf` | Extracts the XML |
| `zerechnung validate [-fr] [-de] [-auto] [-face] [-pdfa basic] [-roots roots.pem] [-json] ... file` | Validates a PDF or an XML file (CII, UBL, FatturaPA or Facturae, whose signature it describes, a French CDAR status or a French e-reporting report), and exits with code 1 on errors; `-face` requires the signature of a Facturae invoice and checks it, `-pdfa basic` skips the PDF/A checks that read the pages and the fonts, `-roots` checks the signatures of a PDF, and with `-face` that of a Facturae invoice, against trusted roots |
| `zerechnung embed -xml invoice.xml [-attach file]... [-convert-pdfa] input.pdf` | Embeds an XML invoice in a PDF, converting it to PDF/A-3 first with `-convert-pdfa` |
| `zerechnung pdfa [-o out.pdf] input.pdf` | Converts a PDF to PDF/A-3, or prints why it cannot, one reason per line |
| `zerechnung sign -key key.pem -cert cert.pem [-certify 1\|2\|3] [-o out.pdf] input.pdf` | Signs a PDF in PAdES Baseline B-B with a PEM key and its certificate chain, or certifies it |
| `zerechnung json file` | Prints the parsed invoice as JSON (CII, UBL, FatturaPA, signed `.xml.p7m` included, or Facturae; an array for a FatturaPA or Facturae batch), the parsed CDAR status or the parsed e-reporting report |
| `zerechnung rules [-fr] [-de] ...` | Lists the rules that are implemented |

Since `validate` exits with code 1 when it finds errors, you can drop it into a CI pipeline as is. Run `zerechnung help` for every flag.

## HTTP server and Docker image

`zerechnung-server` exposes the package as an HTTP API for microservices, in a [Docker image](https://hub.docker.com/r/ritoon/zerechnung) for Linux and Windows Server. One request turns invoice data into a signed Factur-X PDF: rendering as PDF/A-3, CII XML, validation and PAdES signature. Other endpoints validate, sign, verify, extract and convert invoices. Authentication uses a JWT or basic auth, the signing key is a PKCS#12 or PEM file, and logs can go to syslog, GELF, OpenTelemetry, Loki or HTTP.

```bash
docker run -d -p 8080:8080 \
  -e ZR_BASIC_USERS='billing:change-me' \
  -e ZR_SIGN_PKCS12_FILE=/run/secrets/sign.p12 -e ZR_SIGN_PKCS12_PASSWORD='p12 password' \
  -v "$PWD/company.p12:/run/secrets/sign.p12:ro" \
  ritoon/zerechnung

curl -u billing:change-me -H 'Content-Type: application/json' \
  --data-binary @server/examples/invoice-request.json -o invoice.pdf http://localhost:8080/v1/invoices
```

The [server's README](server/README.md) documents the endpoints, the settings and the deployment with Compose and Kubernetes. The server is a module of its own, as it uses the network (JWKS, log collectors), which this package never does.

## What's supported

| Format | Read | Write | Validate |
|---|---|---|---|
| Factur-X 1.0x / ZUGFeRD 2.x (CII): MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED, EXTENDED-CTC-FR | yes | yes | yes |
| XRechnung (CII and UBL) | yes | yes | yes |
| Peppol BIS Billing 3.0 (CII and UBL) | yes | yes | yes |
| NLCIUS, the Netherlands (CII and UBL) | yes | yes | yes |
| CIUS-IT, Italy (CII and UBL), with its extensions EXT-IT in UBL | yes | yes | yes |
| CIUS-PT, Portugal (UBL) | yes | yes | yes |
| CIUS-RO, Romania (UBL and CII) | yes | yes | yes |
| eFaktúra, Slovakia (Peppol BIS in UBL) | yes | yes | yes, with the requirements of the Finančná správa beyond Peppol BIS |
| CIUS-HR, Croatia (UBL), with its extension EXT-HR (Fiskalizacija 2.0) | yes | yes | yes |
| UBL 2.1 Invoice and CreditNote (EN 16931) | yes | yes | yes |
| FatturaPA 1.2, Italy (FPA12 and FPR12) | yes | yes | XSD 1.2.4 and the SDI checks on the content; CIUS-IT and EXT-IT rules through UBL |
| FatturaPA signatures, CAdES (`.xml.p7m`) and XAdES | yes | yes, Baseline B | the SDI checks (§5.1.1) and the file size |
| PAdES signatures of hybrid PDFs | yes | yes, Baseline B-B, B-T, B-LT and B-LTA, approval or certification | integrity, PAdES baseline level, DocMDP, the changes after each signature, trust and revocation against the data you pass |
| Facturae 3.2, 3.2.1 and 3.2.2, Spain | yes | yes (3.2.2) | XSD of its version; EN 16931 rules through CII or UBL |
| Facturae signatures, XAdES-EPES | yes | yes, under the policy of Facturae v3.1 | the policy, the role of the signer and the certificate (FACe rule set) |
| ZUGFeRD 1.0 | yes | as Factur-X | minimal checks |
| CDAR D22B statuses of the life cycle of an invoice, France (CDV) | yes | yes, and builds the statuses 200 Déposée and 213 Rejetée | XSD D22B and the BR-FR-CDV rules |
| E-reporting reports, France (flux 10): transactions and payments | yes | yes | XSD of France_RFE 1.4.0.04, the values that cannot be read (ER-01) and the rules of the PPF (schematron Flux10) |
| ATCUD and QR code, Portugal (Portaria n.º 195/2020, specifications of the AT 1.1) | yes | yes, from the invoice, and the symbol in PNG or SVG | the format of each field, the ATCUD against the document number |
| Swiss QR Code of the QR-bill, Switzerland and Liechtenstein (Implementation Guidelines 2.4 and 2.3 of SIX) | yes | yes, from the invoice, and the symbol with the Swiss cross in PNG or SVG | each line: account and reference, check digits, lengths, character set, amount, billing information S1 |

Validation covers the XSD schemas, the EN 16931 rules (CEN 1.3.17), the Factur-X 1.09.2 profile rules, the French BR-FR rules (2026 reform), the EXTENDED-CTC-FR rules of the FNFE-MPE (France_RFE 1.4.0.04, CII and UBL), the German BR-DE rules (XRechnung 3.0.2), Peppol BIS Billing 3.0, the Dutch BR-NL rules, the Italian CIUS-IT rules and those of its extensions (BR-IT, BR-IT-DE, technical rules 2.7), the Portuguese CIUS-PT rules (schematron 2.1.1 of eSPap), the Romanian CIUS-RO rules (schematron 1.0.9 of ANAF, also transposed to CII), the Slovak requirements of the eFaktúra (transposition 1.13 of the Finančná správa), the Croatian CIUS-HR rules and those of its extension (schematron of the Porezna uprava of 13 March 2026), the BR-FR-CDV rules of the French statuses of the life cycle (France_RFE 1.4.0.04), the Chorus Pro checks, the FACe checks of the Facturae signature, the schemas of the French e-reporting reports and the 150 rules of the PPF on their content (flux 10 and schematron Flux10 of France_RFE 1.4.0.04), 72 checks of the PDF container and 14 checks of its signatures. The rule counts and the official artefacts behind each set are on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy).

## How accurate is it?

Every pull request runs the reference validators on the official test corpora and compares the results invoice by invoice:

- KoSIT validator (XRechnung): the same errors on all 128 invoices.
- Mustang: the same errors on 239 of 282 invoices. Each of the 43 differences comes from the references themselves and is explained in [known.txt](conformance/parity/known.txt).
- Dutch NPa validators (NLCIUS): the same rules on 283 of 290 files, with the 7 differences justified.
- FNFE-MPE XSLT (EXTENDED-CTC-FR, CII and UBL), run by Saxon: the same rules, at the same levels, on 226 of 227 files (the 10 official examples, the 4 test invoices and 213 variants), with the 1 difference justified.
- FNFE-MPE XSLT of the statuses of the life cycle (BR-FR-CDV), run by Saxon: the same rules, at the same levels, on all 2,149 CDAR statuses (the 59 official examples and 2,090 variants).
- eSPap schematron (CIUS-PT), compiled by SchXslt and run by Saxon: the same rules, at the same levels and on the same elements, on the 15 official examples, the 3 test invoices and the 5,462 of their 5,765 variants that Saxon completes. On the other 303, Saxon stops on an XPath type error in the test of an assertion, which the package holds failed: it raises that assertion every time.
- ANAF schematron (CIUS-RO, the pattern of its Romanian rules), compiled by SchXslt and run by Saxon: the same rules, at the same levels and on the same elements, on the 13 official UBL examples, the 2 UBL test invoices and the 2,703 of their 2,892 variants that Saxon completes. On the other 189, Saxon stops on an XPath type error: in the test of a Romanian rule, which the package holds failed and raises every time, or in that of BR-27, a rule of EN 16931 left out of the comparison.
- Porezna uprava schematron (CIUS-HR and its extension), compiled by SchXslt and run by Saxon: the same rules, at the same levels and on the same elements, on the 20 official examples, the 3 test invoices and the 5,128 of their 5,662 variants that Saxon completes. On the other 534, Saxon stops on an XPath type error in the test of an assertion, which the package holds failed: it raises that assertion every time.
- veraPDF corpus, PDF/A-2b and PDF/A-3b: `Check` reports 609 of the 614 invalid test files under a rule of the right clause, and no PDF/A error on the 384 valid ones. The 5 files it misses are listed with their reason in [check_pdfa_corpus_test.go](check_pdfa_corpus_test.go).
- OpenSSL, xmlsec1 and EU DSS on FatturaPA signatures, both ways: each verifies the CAdES and XAdES files the package signs (DSS finds them CAdES and XAdES Baseline B), and the package verifies the 13 files they sign.
- pdfsig and EU DSS on PAdES signatures, both ways: pdfsig verifies the signed hybrid invoices, DSS finds them PAdES-BASELINE-B, -T and -LTA as written (TOTAL_PASSED for B-LTA, which carries its revocation data), and the package verifies the PDF that DSS signs. veraPDF passes the signed PDF/A-3 invoices.
- xmlsec1 and EU DSS on Facturae signatures, both ways: each verifies the files `facturae.Sign` writes (DSS finds them XAdES Baseline B under the policy of Facturae v3.1, with the role of the signer and `ds:KeyInfo` signed), and the package verifies the 6 files they sign.
- veraPDF on the files `ConvertPDFA` writes: of the 998 test files of the same corpus, 836 are converted and 835 of them pass; the other one is a known failure (a value type of an XMP extension schema), listed in [convert_corpus_test.go](convert_corpus_test.go). The non-conforming PDFs the tests generate all pass once converted.

A new difference fails the build. The method is described on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy#measured-accuracy).

## Performance

Measured on an Apple M4 Pro with Go 1.27.4 (commit `3a3b11a`, November 22, 2026), against the fastest other Go package that handles the same operation:

| Operation | ZeRechnung | Best other Go package |
|---|---|---|
| Extract the XML from a 149 KB PDF | 30 µs | 73 µs |
| Read and fully validate a 13 KB invoice | 25 µs | 128 µs |
| Embed the XML in a PDF | 99 µs | 529 µs |
| Check a 149 KB hybrid PDF | 832 µs | 1.11 ms |

`Check` applies the PDF/A requirements in depth: it reads every object of the PDF, its content streams and its font programs included, to catch what veraPDF would reject; v0.1.0 checked the same PDF in 74 µs without them. Most of the time goes to decompressing its three font programs. Since the measurements below, the buffers of the font programs and content streams are reused from one call to the next, the font programs decompress about 1.6 times faster, the XMP packet is parsed once and the operands of the content streams are read without boxing them: the full check of the same PDF takes about 560 µs and allocates about 160 KB in 850 allocations, instead of 832 µs and 701 KB in 2,376, and `Extract` takes 23 µs and allocates 37 KB in 187 allocations. From the second PDF with the same fonts, a [`Checker`](#check-many-invoices-on-a-server) brings the full check down to about 330 µs. When the PDF/A conformance is checked elsewhere, or only the invoice and its attachment matter, `validate.Options{PDFA: validate.PDFABasic}` (`-pdfa basic` on the command line) skips the checks that read the pages, the fonts and the file structure (PDF-41 to PDF-69, PDF-71, PDF-72): the same check then takes about 80 µs and allocates 87 KB.

What one call allocates, as `go test -benchmem` counts it, against the other Go package that allocates the least:

| Operation | ZeRechnung | Lightest other Go package |
|---|---|---|
| Extract the XML from a 149 KB PDF | 39 KB in 228 allocations | 184 KB in 4,926 |
| Read a 13 KB invoice | 28 KB in 104 allocations | 53 KB in 1,165 |
| Read and fully validate a 13 KB invoice | 23 KB in 75 allocations | 67 KB in 1,462 |
| Write a 13 KB invoice | 8 KB in 1 allocation | 32 KB in 175 |
| Embed the XML in a PDF | 245 KB in 456 allocations | 1.3 MB in 9,593 |
| Check a 149 KB hybrid PDF | 701 KB in 2,376 allocations | 896 KB in 24,617 |

Importing the package allocates about 180 KB at startup, standard library included, instead of 2.9 MB before: the tables of the schemas, the rule sets and their regular expressions are built at their first use, which adds about 0.2 ms to the first invoice of each format. The tests fail when an import goes beyond its budget (`internal/initcost`), or when `Extract`, `Check`, `cii.Parse` or `validate.XML` goes beyond its budget of allocations (`TestAllocs`). The `zerechnung` command is built with a CPU profile ([PGO](CONTRIBUTING.md#profil-pgo-de-la-commande)), which makes it 1 to 3 % faster.

A large PDF need not be copied to the heap: the package never writes to its input, and the streams it reads point into it, so the input can be a file mapped in memory with `syscall.Mmap` on Unix systems ([`Example_mmap`](example_mmap_test.go)).

The benchmarks have their own repository, [ritoon/invoice-bench](https://github.com/ritoon/invoice-bench): the full tables are in [ecosystem/results](https://github.com/ritoon/invoice-bench/blob/main/ecosystem/results/2026-11-22/tables.md), and the comparison with the TypeScript, Python, Rust, Java, C#, C, C++ and PHP libraries is in [polyglot](https://github.com/ritoon/invoice-bench/tree/main/polyglot). The method is on the wiki (in French): [Benchmarks](https://github.com/ritoon/zerechnung/wiki/Benchmarks).

## Limitations

- PDF/A conversion is partial. `ConvertPDFA` does not embed fonts, convert CMYK colours, create annotation appearances or write PDF/A-4: such PDFs are refused, with the reasons. Without conversion, your PDF must already meet PDF/A-2, PDF/A-3 or PDF/A-4.
- `Check` reports most PDF/A problems, those of the text and its fonts included, but it does not replace a full validator such as veraPDF: it does not read Type 1 font programs (CFF, TrueType and OpenType ones are read) or the value types of XMP extension schemas, and it applies only part of the PDF/A-4 requirements.
- PDF/A levels A and U are not checked. Factur-X and ZUGFeRD only need level B (basic), which is what `ConvertPDFA` writes; a PDF that declares level A or U is checked as level B, without its logical structure (tags) or the Unicode mapping of its text.
- Encrypted PDFs are detected, not decrypted.
- ZUGFeRD 1.0 is read only, and written back as Factur-X.
- XRechnung invoices are checked against XRechnung 3.0.2, the version in force since 1 February 2024 and, as the summer of 2026 brought no normative release, until 31 July 2027 at least; the artefacts come from the KoSIT validator configuration of 31 August 2026. XRechnung 4.0, which follows EN 16931-1:2026 and its 2026 syntax bindings, is not supported: only its specification document is published, as a pre-version of 15 September 2026 that is not meant for productive use, and its schematrons, XSD and KoSIT configuration wait for the CEN validation artefacts (the final version is expected in the spring of 2027). An invoice that declares it gets `EN16931-EDITION-01` and no other rule, as any EN 16931-1:2026 invoice does.
- FatturaPA is validated against its XSD schema 1.2.4 (the content of the XML signature excepted) and against the 40 checks of the SDI on the content that need none of its data (codes 00300 to 00476 of appendix 1 of its technical specifications, under these codes): the tax codes are checked by their format and check character, not looked up in the Anagrafe tributaria, and neither the duplicate invoices (00404) nor the recipient codes (00311, 00312) are checked. The date of the invoice is compared with `Options.ReceivedAt`, the current time by default (00403). Convert it to UBL to apply the CIUS-IT rules and those of its extensions. `Marshal` writes one invoice per file. CIUS-IT invoices are checked against the rules of appendix 2 of the Italian technical rules, except BR-IT-DC-291 and BR-IT-DC-480, which depend on facts the invoice does not carry, and against the 45 rules of the Italian extensions of appendix 3 (BR-IT-DE-*, EXT-IT) when the seller is in Italy: on the UBL extensions and elements of the domestic invoice, which only UBL carries (BR-IT-DE-052 also applies to CII). `ubl.Parse` and `ubl.Marshal` map them to the FatturaPA data of the model (appendix 4.2) when the specification identifier declares the extension, so FatturaPA converted to UBL and back keeps what they carry; the rest, such as the bank details of FatturaPA or the CIG and CUP codes that BT-17 and BT-11 cannot hold, is reported by strict mode. CII carries none of it.
- FatturaPA signatures are written in Baseline B, without timestamps (B-T and above). `VerifySignature` fetches nothing: it checks trust against the roots you pass (the list of qualified authorities the SDI accepts is not embedded) and revocation against the CRLs and OCSP responses you pass.
- Facturae is validated against the XSD schema of its version (the content of the XML signature and of the extensions excepted): no official mapping to EN 16931 is published, so the `facturae` package follows its own, described in its documentation, and the EN 16931 rules apply once the invoice is converted to UBL or CII. Spain has published no Schematron nor NIF check for its B2B invoices, so there is no Spanish rule set: the FACe rule set only checks the signature.
- Facturae signatures are written in the basic format of the signature policy of Facturae v3.1 (XAdES-EPES), without the timestamp and validation data of its second format (XAdES-X-L). `VerifySignature` fetches nothing, as for FatturaPA: trust against the roots you pass (neither the Spanish trusted list nor @firma is embedded), revocation against the CRLs and OCSP responses you pass. A signature in SHA-1 is refused. Rewriting a signed invoice with `Marshal` drops its signature: sign the new document again.
- CIUS-PT invoices are checked against the schematron 2.1.1 of eSPap, which replaces the CEN artefacts for them, as on the validator of the Portuguese public administration; the CIUS-PT has no CII binding, so a CII invoice that declares it gets `PT-SYNTAX-01`. The check digit of a Portuguese VAT identifier (NIF) is only a warning (`PT-NIF-02`), as the official examples carry invented NIFs. The CIUS-PT does not put the ATCUD, the QR code or the certificate of the invoicing software in the XML: only the format of the structured notes that carry the number and hash of a certified program is checked. SAF-T (PT), the e-fatura communication and the transmission are out of scope.
- The ATCUD and the QR code of Portuguese invoices follow Portaria n.º 195/2020 and version 1.1 (October 2020) of the technical specifications of the AT. `atcud.Build` fills a single fiscal space from the VAT breakdown, in the categories of the CIUS-PT: a breakdown of category S counts at the normal rate, so give a reduced or intermediate rate category RED or INT; the stamp duty and the other fiscal spaces are set by hand. It refuses an invoice in another currency than the euro, as the QR code holds euros and the invoice carries no exchange rate. `Parse` and `Check` check the format of each field and the ATCUD against the document number; they do not compare the totals with the amounts by rate, which the specification does not relate, and refuse the space after the colon that examples 6 and 7 of the specification print. The validation code of the series comes from the AT, and the package neither signs the document (Portaria n.º 363/2010) nor places the symbol on it: that is the job of the certified invoicing software.
- The Swiss QR Code follows the Swiss Implementation Guidelines for the QR-bill of SIX, version 2.4 (in force from 14 November 2026) and version 2.3 (valid until November 2027): both write version 0200, and only 2.3 admits the QR-IBAN and the QR reference in EUR, so `Parse` takes the version to apply, which `Bill.Check` reads from `Bill.Guidelines`. `qrbill.Build` puts the address line of EN 16931 in the street and leaves the building number empty, which the guidelines tolerate; it leaves out a buyer whose address lacks a postal code, a town or a country, and makes a notification ("DO NOT USE FOR PAYMENT", amount 0.00) of an invoice with nothing to pay. It does not invent the reference: a QR-IBAN needs one, from the remittance information (BT-83) or `Data.Reference`. Example 4 of Annex A prints a Creditor Reference whose check digits are wrong (RF72… for RF24…): `Parse` refuses it, as the guidelines require the check digits of ISO 11649. Errors in the billing information and the alternative procedures, which a bank must not refuse a QR-bill for, are told apart (`FieldError.Additional`). The package returns the content and the symbol: the layout of the payment part and the receipt (210 × 105 mm, headings, perforation), the PDF of the HTTP server, eBill and the payment orders to the banks are out of scope.
- CIUS-RO invoices are checked against the 121 Romanian rules of the schematron 1.0.9 of ANAF, on top of EN 16931, as RO e-Factura does. The schematron binds them to UBL only: the package transposes them to CII, which the Romanian order admits, under the same identifiers. The schematron is applied as published, with its slips: seven assertions can never fail (listed in [rule-coverage.txt](testdata/rule-coverage.txt)), and the rules on the tax representative, the delivery address and the document level allowances and charges do not apply to a credit note, as their context names a root that does not exist. The check digit of a Romanian tax identifier (CUI/CIF) in BT-31, BT-32, BT-48 and BT-63 is only a warning (`RO-CIF-01`, `RO-CIF-02`), as the official examples carry invented ones. The SPV of ANAF (upload, signature of the ministry, conversion to PDF), RO e-Transport and SAF-T (D406) are out of scope.
- Slovak eFaktúra invoices are Peppol BIS Billing 3.0 invoices, and Slovakia publishes no schematron: the Slovakia rule set (`SK-01` to `SK-15`) checks what the transposition of Peppol BIS to the Slovak legislation (version 1.13 of the Finančná správa) and its FAQ require beyond Peppol BIS, with identifiers of its own. A requirement that holds "if assigned" (IČ DPH, IČO, register entry) is an error only when the invoice tells that it holds (a VAT category that only a VAT payer invoices, reverse charge, the legal form of a company in the seller's name), a warning otherwise. Only the format of the DIČ, IČ DPH and IČO is checked: no official check digit is published. The tax data document (TDD SK) that the access points report, its rules, Peppol transmission and Peppol Self-Billing are out of scope.
- CIUS-HR invoices are checked against the 62 rules of the schematron of the Croatian tax administration (Porezna uprava, version of 13 March 2026, for the specification 1.8 of the CIUS-HR and its extension EXT-HR), on top of EN 16931: the issue time and the operator required by the law on fiscalisation, the business process (P1 to P12, or P99: and a label), the code of the Croatian classification of products KPD, the Croatian VAT categories of table HR-TB-2 and the Croatian VAT breakdown of the extension HRFISK20Data, the OIB of the operator and of the parties in Croatia. The Schema rule set checks the extension against its schema, from the package "UBL2.1 eRačun" of the tax administration. The elements that the CIUS-HR adds to the syntax binding raise the warnings UBL-CR of EN 16931. The CIUS-HR has no CII binding, so a CII invoice that declares it gets `HR-SYNTAX-01`. The OIB of an electronic address of scheme 9934 is only a warning (`HR-OIB-01`), as the official examples carry invented ones. `ubl.Parse` and `ubl.Marshal` read and write the Croatian data in the national blocks of the model (`invoice.Croatia`); strict mode reports them when the invoice is written under another specification identifier, or in CII. The fiscalisation and the reports to the tax administration (eIzvještavanje), the access points (AMS, MPS), the signature, MIKROeRAČUN and the invoices to consumers are out of scope.
- The statuses of the life cycle of French invoices are read, written and validated in CDAR D22B, the syntax of France_RFE 1.4.0.04, against the 43 assertions of its schematron BR-FR-CDV, applied as published: six of them can never fail (listed in [rule-coverage.txt](testdata/rule-coverage.txt)). `cdar.Deposited` and `cdar.Rejected` build the statuses 200 Déposée and 213 Rejetée that the platform of the seller emits, for the seller and the PPF or for the PPF alone; the other statuses are written by filling the model. The exchange of the statuses between the platforms and with the PPF, and the platform mode, are out of scope.
- French e-reporting reports (flux 10) are checked against the schemas of France_RFE 1.4.0.04, for the values that `ereporting.Parse` cannot read (`ER-01`): a date that is not AAAAMMJJ, a time of transmission that is not AAAAMMJJHHMMSS, and against the 150 assertions of its schematron Flux10, the rules of the PPF drawn from annex 7 v1.8 of the specifications of the DGFiP (identifiers, dates, code lists, totals), applied as published. Its assertions have no identifier: a rule is named after its pattern, with a suffix `-2`, `-3`… for the second and following assertions of a pattern (`F10-TRANSMISSION-G1.104-2`), and its message keeps the code of annex 7 (`[G1.104]`). The checks that need the directories of the PPF (existence of a SIREN, of a country or currency code) are out of reach, and the BR-FR-MAP rules, which map an invoice to the data it reports, are not published as an artefact. The package does not build the report from invoices, nor send it: the transmission to the PPF is the platform's job.
- PAdES signatures are qualified only if your certificate and signing device are: the package reads what the certificate declares (QCStatements), and checks trust against the roots you pass, as no EU trusted list is embedded. Nothing is fetched: time-stamp tokens come from your `Timestamper`, revocation data from you. Signature fields are invisible, without an appearance. `VerifySignatures` classifies the changes after a signature by the objects they touch, without rendering the pages. Encrypted PDFs are refused, and veraPDF checks the signed PDF/A-3 files only, not PDF/A-4.
- Only EN 16931-1:2017 is supported. An invoice whose specification identifier (BT-24) names EN 16931-1:2026, such as `urn:cen.eu:en16931:2026`, gets the error `EN16931-EDITION-01` and no other rule, since the 2026 validation artefacts are not published yet. Force a profile (`Options.Profile`, `-profile`) to apply the 2017 rules anyway. `Create` and `embed` do not write this edition.
- The rules are rewritten in Go from the official artefacts, so a new version of an artefact needs a code update.

The full list is on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy#limitations).

## Packages

| Package | Role |
|---|---|
| `zerechnung` | Create, read, extract, embed, check, sign and verify hybrid PDFs |
| `invoice` | The EN 16931 model, exact decimals, dates, profiles, total calculation |
| `cii` | Read and write CII XML, profile by profile |
| `ubl` | Read and write UBL 2.1 XML (Invoice, CreditNote) |
| `fatturapa` | Read, write, sign and verify FatturaPA 1.2 files (Italy) |
| `facturae` | Read Facturae 3.2, 3.2.1 and 3.2.2 XML, write Facturae 3.2.2, sign and verify it in XAdES-EPES (Spain) |
| `cdar` | Read, write and build the statuses of the life cycle of French invoices (CDAR D22B) |
| `ereporting` | Read and write the reports of the French e-reporting (flux 10) |
| `atcud` | Build and check the ATCUD and the QR code of Portuguese invoices |
| `qrbill` | Build, read and check the Swiss QR Code of the QR-bill, and draw it with the Swiss cross |
| `qrcode` | Encode QR Code symbols and draw them in PNG or SVG |
| `validate` | Validation: schemas, EN 16931, Factur-X, BR-FR, BR-FR-CDV, BR-DE, Peppol, BR-NL, BR-IT, CIUS-PT, BR-RO, SK, HR-BR, Chorus Pro, FACe, e-reporting (Flux10) |
| `cmd/zerechnung` | The command-line tool |
| `server` | The HTTP server and its Docker image (a module of its own) |

## Documentation

The [wiki](https://github.com/ritoon/zerechnung/wiki) has the details this page leaves out:

- [Creating invoices](https://github.com/ritoon/zerechnung/wiki/Creating-invoices): what `Create` and `Embed` write, PDF/A, attachments, `/AFRelationship` by country, VAT on debits.
- [Reading invoices](https://github.com/ritoon/zerechnung/wiki/Reading-invoices): tolerant reading, unknown data, UBL, ZUGFeRD 1.0.
- [Validating invoices](https://github.com/ritoon/zerechnung/wiki/Validating-invoices): options, rule sets and national rules.
- [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy): profiles, rules, measured accuracy, limitations.
- [Testing](https://github.com/ritoon/zerechnung/wiki/Testing): running the tests and the conformance suite.

Background on why the project exists, a criterion-by-criterion comparison with the other Go libraries and an overview of e-invoicing rules across Europe are also on the wiki, in French.

## Contributing

Contributions are welcome. Run the tests with:

```bash
go test ./...
```

[CONTRIBUTING.md](CONTRIBUTING.md) lists the package's principles and the checks to run before a pull request, and [Testing](https://github.com/ritoon/zerechnung/wiki/Testing) explains how to run the tests against the official artefacts. The project follows the [Contributor Covenant code of conduct](CODE_OF_CONDUCT.md). Please report security issues privately, as described in [SECURITY.md](SECURITY.md).

## License

ZeRechnung is released under the [MIT license](LICENSE).

The validation rules are rewritten in Go from official artefacts (CEN, FNFE-MPE, FeRD, KoSIT, OpenPEPPOL, the Dutch Peppol authority), which are not included in the repository. Their versions and licenses are listed on [License and provenance](https://github.com/ritoon/zerechnung/wiki/License-and-provenance).

The QR code encoder of [`internal/qr`](internal/qr) is a trimmed copy of the encoder of [piglig/go-qr](https://github.com/piglig/go-qr) v1.1.0, released under the MIT license; its notice is in [internal/qr/LICENSE](internal/qr/LICENSE).
