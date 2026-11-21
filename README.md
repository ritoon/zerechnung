# ZeRechnung

[![Go Reference](https://pkg.go.dev/badge/github.com/ritoon/zerechnung.svg)](https://pkg.go.dev/github.com/ritoon/zerechnung)
[![CI](https://github.com/ritoon/zerechnung/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ritoon/zerechnung/actions/workflows/ci.yml)
[![Conformance](https://github.com/ritoon/zerechnung/actions/workflows/conformance.yml/badge.svg?branch=main)](https://github.com/ritoon/zerechnung/actions/workflows/conformance.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ritoon/zerechnung)](https://goreportcard.com/report/github.com/ritoon/zerechnung)
[![codecov](https://codecov.io/gh/ritoon/zerechnung/graph/badge.svg)](https://codecov.io/gh/ritoon/zerechnung)
[![Go version](https://img.shields.io/github/go-mod/go-version/ritoon/zerechnung)](go.mod)
[![MIT license](https://img.shields.io/github/license/ritoon/zerechnung)](LICENSE)

<p align="center">
  <img src="docs/logo.png" alt="ZeRechnung, Go package for Factur-X and ZUGFeRD: the Go gopher reads an invoice next to a typewriter, a paper boat sits on the n, and a stack of invoices is marked FR and DE">
</p>

ZeRechnung is a Go package for e-invoices in the Factur-X, ZUGFeRD and XRechnung formats.

Keep generating your invoice PDFs the way you do today. ZeRechnung adds the XML invoice and the metadata the standard requires, and the PDF becomes a compliant e-invoice. It also reads and validates the invoices you receive, whether they come as hybrid PDFs or as plain XML (CII or UBL), and reads, writes, signs and verifies FatturaPA, the national format of the Italian SDI.

- Pure Go, standard library only. No CGO, no Java, no external binary, no network calls.
- All Factur-X / ZUGFeRD 2.x profiles, from MINIMUM to EXTENDED, plus XRechnung, Peppol BIS Billing 3.0, NLCIUS and CIUS-IT, in CII and UBL 2.1.
- Validation compared with the reference validators (KoSIT, Mustang) on every pull request, with the French 2026 reform, German, Dutch, Italian and Peppol rules built in.
- PDF/A-3 conversion of the PDFs that need no PDF engine for it, with the reasons for a refusal otherwise, checked by veraPDF in the CI.
- Fast: reading and validating a typical invoice takes about 25 µs.

> **Status: v0.** The API may still change before v1.

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
- The PDF is changed with an incremental update. Your original bytes stay untouched, so an existing signature remains valid.

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
| An XML file instead of a PDF | `validate.XML(xml, opts)` |

XRechnung, Peppol, NLCIUS and CIUS-IT invoices declare themselves, so their rules apply without any option. All the options and rule sets are described in [Validating invoices](https://github.com/ritoon/zerechnung/wiki/Validating-invoices).

### Work with the XML directly

The `cii`, `ubl` and `fatturapa` packages read and write the XML with the same `invoice.Invoice` model, so converting between syntaxes is a parse and a marshal:

```go
// Write the CII XML for a given profile.
xml, err := cii.Marshal(inv, &cii.MarshalOptions{Profile: invoice.ProfileBasic})

// Convert a UBL invoice to CII.
fromUBL, err := ubl.Parse(ublXML)
ciiXML, err := cii.Marshal(fromUBL, nil)

// Convert a FatturaPA invoice to CII, and a CII invoice to FatturaPA.
fromSDI, err := fatturapa.Parse(sdiXML)
ciiXML, err = cii.Marshal(fromSDI, nil)
sdiXML, err = fatturapa.Marshal(inv, nil)

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

### Declare VAT on debits (France)

```go
inv.TaxPointDateCode = invoice.TaxPointDebits
```

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
| `zerechnung validate [-fr] [-de] [-auto] [-json] ... file` | Validates a PDF or an XML file, and exits with code 1 on errors |
| `zerechnung embed -xml invoice.xml [-attach file]... [-convert-pdfa] input.pdf` | Embeds an XML invoice in a PDF, converting it to PDF/A-3 first with `-convert-pdfa` |
| `zerechnung pdfa [-o out.pdf] input.pdf` | Converts a PDF to PDF/A-3, or prints why it cannot, one reason per line |
| `zerechnung json file` | Prints the parsed invoice as JSON (CII, UBL or FatturaPA, signed `.xml.p7m` included; an array for a FatturaPA batch) |
| `zerechnung rules [-fr] [-de] ...` | Lists the rules that are implemented |

Since `validate` exits with code 1 when it finds errors, you can drop it into a CI pipeline as is. Run `zerechnung help` for every flag.

## What's supported

| Format | Read | Write | Validate |
|---|---|---|---|
| Factur-X 1.0x / ZUGFeRD 2.x (CII): MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED, EXTENDED-CTC-FR | yes | yes | yes |
| XRechnung (CII and UBL) | yes | yes | yes |
| Peppol BIS Billing 3.0 (CII and UBL) | yes | yes | yes |
| NLCIUS, the Netherlands (CII and UBL) | yes | yes | yes |
| CIUS-IT, Italy (CII and UBL) | yes | yes | yes |
| UBL 2.1 Invoice and CreditNote (EN 16931) | yes | yes | yes |
| FatturaPA 1.2, Italy (FPA12 and FPR12) | yes | yes | through CII (CIUS-IT) |
| FatturaPA signatures, CAdES (`.xml.p7m`) and XAdES | yes | yes, Baseline B | the SDI checks (§5.1.1) |
| ZUGFeRD 1.0 | yes | as Factur-X | minimal checks |

Validation covers the XSD schemas, the EN 16931 rules (CEN 1.3.17), the Factur-X 1.09.2 profile rules, the French BR-FR rules (2026 reform), the German BR-DE rules (XRechnung 3.0.2), Peppol BIS Billing 3.0, the Dutch BR-NL rules, the Italian CIUS-IT rules (BR-IT, technical rules 2.7), the Chorus Pro checks and 72 checks of the PDF container. The rule counts and the official artefacts behind each set are on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy).

## How accurate is it?

Every pull request runs the reference validators on the official test corpora and compares the results invoice by invoice:

- KoSIT validator (XRechnung): the same errors on all 128 invoices.
- Mustang: the same errors on 241 of 282 invoices. Each of the 41 differences comes from the references themselves and is explained in [known.txt](conformance/parity/known.txt).
- Dutch NPa validators (NLCIUS): the same rules on 283 of 290 files, with the 7 differences justified.
- veraPDF corpus, PDF/A-2b and PDF/A-3b: `Check` reports 609 of the 614 invalid test files under a rule of the right clause, and no PDF/A error on the 384 valid ones. The 5 files it misses are listed with their reason in [check_pdfa_corpus_test.go](check_pdfa_corpus_test.go).
- OpenSSL, xmlsec1 and EU DSS on FatturaPA signatures, both ways: each verifies the CAdES and XAdES files the package signs (DSS finds them CAdES and XAdES Baseline B), and the package verifies the 13 files they sign.
- veraPDF on the files `ConvertPDFA` writes: of the 998 test files of the same corpus, 836 are converted and 835 of them pass; the other one is a known failure (a value type of an XMP extension schema), listed in [convert_corpus_test.go](convert_corpus_test.go). The non-conforming PDFs the tests generate all pass once converted.

A new difference fails the build. The method is described on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy#measured-accuracy).

## Performance

Measured on an Apple M4 Pro with Go 1.26.4 (v0.1.0), against the fastest other Go package that handles the same operation:

| Operation | ZeRechnung | Best other Go package |
|---|---|---|
| Extract the XML from a 149 KB PDF | 30 µs | 75 µs |
| Read and fully validate a 13 KB invoice | 24 µs | 126 µs |
| Embed the XML in a PDF | 89 µs | 522 µs |
| Check a 149 KB hybrid PDF | 74 µs | 1.14 ms |

Since v0.1.0, `Check` applies the PDF/A requirements in depth: it reads every object of the PDF, its content streams and its font programs included. These checks are always on, since they catch what veraPDF would reject. Checking the same 149 KB PDF now takes about 0.8 ms, 0.2 ms of it for the content streams and the fonts.

The full tables are in [bench/ecosystem/results](bench/ecosystem/results/2026-11-17/tables.md), and a comparison with 32 TypeScript, Python and Rust libraries is in [bench/polyglot](bench/polyglot). The method is on the wiki (in French): [Benchmarks](https://github.com/ritoon/zerechnung/wiki/Benchmarks).

## Limitations

- PDF/A conversion is partial. `ConvertPDFA` does not embed fonts, convert CMYK colours, create annotation appearances or write PDF/A-4: such PDFs are refused, with the reasons. Without conversion, your PDF must already meet PDF/A-2, PDF/A-3 or PDF/A-4.
- `Check` reports most PDF/A problems, those of the text and its fonts included, but it does not replace a full validator such as veraPDF: it does not read Type 1 font programs (CFF, TrueType and OpenType ones are read) or the value types of XMP extension schemas, and it applies only part of the PDF/A-4 requirements.
- PDF/A levels A and U are not checked. Factur-X and ZUGFeRD only need level B (basic), which is what `ConvertPDFA` writes; a PDF that declares level A or U is checked as level B, without its logical structure (tags) or the Unicode mapping of its text.
- Encrypted PDFs are detected, not decrypted.
- ZUGFeRD 1.0 is read only, and written back as Factur-X.
- FatturaPA is read and written, but not validated against its XSD schema nor the SDI checks: convert it to CII to apply the CIUS-IT rules. `Marshal` writes one invoice per file. CIUS-IT invoices are checked against the rules of appendix 2 of the Italian technical rules, except BR-IT-DC-291 and BR-IT-DC-480, which depend on facts the invoice does not carry; the rules of the Italian extensions (BR-IT-DE-*) are not checked.
- FatturaPA signatures are written in Baseline B, without timestamps (B-T and above). `VerifySignature` fetches nothing: it checks trust against the roots you pass (the list of qualified authorities the SDI accepts is not embedded) and revocation against the CRLs and OCSP responses you pass.
- For EXTENDED-CTC-FR, only the CII syntax rules of the FNFE schematron are applied, not its business rules (BR-FREXT).
- Only EN 16931-1:2017 is supported. An invoice whose specification identifier (BT-24) names EN 16931-1:2026, such as `urn:cen.eu:en16931:2026`, gets the error `EN16931-EDITION-01` and no other rule, since the 2026 validation artefacts are not published yet. Force a profile (`Options.Profile`, `-profile`) to apply the 2017 rules anyway. `Create` and `embed` do not write this edition.
- The rules are rewritten in Go from the official artefacts, so a new version of an artefact needs a code update.

The full list is on [Coverage and accuracy](https://github.com/ritoon/zerechnung/wiki/Coverage-and-accuracy#limitations).

## Packages

| Package | Role |
|---|---|
| `zerechnung` | Create, read, extract, embed and check hybrid PDFs |
| `invoice` | The EN 16931 model, exact decimals, dates, profiles, total calculation |
| `cii` | Read and write CII XML, profile by profile |
| `ubl` | Read and write UBL 2.1 XML (Invoice, CreditNote) |
| `fatturapa` | Read, write, sign and verify FatturaPA 1.2 files (Italy) |
| `validate` | Validation: schemas, EN 16931, Factur-X, BR-FR, BR-DE, Peppol, BR-NL, BR-IT, Chorus Pro |
| `cmd/zerechnung` | The command-line tool |

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
