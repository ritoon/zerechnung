# The ZeRechnung website

The presentation site of ZeRechnung, published on GitHub Pages by the [Site workflow](../.github/workflows/site.yml) at every push to `main`. Each language has four pages: the home page, worked examples, benchmarks and compliance.

It is a separate Go module, so the package itself stays untouched. Its only dependency is the package of this repository, as it is in the tree (`replace … => ../`): the examples are real programs that call it. `go run .` generates static HTML: no JavaScript framework, no external font or script, nothing that tracks visitors.

| Path | Content |
|---|---|
| `i18n/` | One JSON file per language: its code, its name in that language, the texts and the translated page paths (`*_slug`) |
| `templates/` | One template per page (`home`, `examples`, `benchmarks`, `compliance`), the shared parts, the language chooser at the root and the 404 page (`html/template`) |
| `examples/` | The worked examples: one Go program per directory, `<lang>-<n>-<name>` |
| `data/` | `benchmarks.json` (the results published in the wiki) and `compliance.json` (the standards by country) |
| `static/` | Style sheet, script, images, copied as is |
| `main.go`, `examples.go`, `data.go` | The generator |

## Build and preview

```bash
cd site
go test ./...
go run . -out public -base http://localhost:8000/
python3 -m http.server -d public 8000
```

Then open http://localhost:8000/. Without `-base`, the canonical links and the sitemap point to the published site. `go test -short` skips running the examples.

## Add an example

An example is a complete program in `examples/<lang>-<n>-<name>/main.go`, shown on the examples page of the language `<lang>` at position `<n>`. Each language has at least three. The page cuts the program into steps, so it follows this layout:

```go
// Title: Une fromagerie du Jura facture un bouchon lyonnais
// Tags: Factur-X, EN 16931, TVA 5,5 % et 20 %
//
// The introduction: one or more paragraphs, separated by an empty comment
// line.
package main

import (…)

func main() {
	// ## The title of the first step
	// Its explanation, in comment lines right after the title.
	code of the step

	// ## The title of the next step
	…
}

// Output:
// what the program prints
```

- Tags are separated by a comma and a space, so `5,5 %` stays one tag.
- All the code is inside `main`, cut into at least three steps, each with an explanation.
- Comments, titles and printed labels are in the language of the page; identifiers stay in Go.
- The story is fictitious but true to the country: its companies, its products, its VAT rates, its formats and rules. Identifiers (SIREN, Partita IVA, Leitweg-ID, IBAN…) have valid check digits.
- An example that starts from a PDF reads it from its own directory (`facture.pdf`, `rechnung.pdf`, `fattura.pdf`, `factura.pdf`, `fatura.pdf` or `invoice.pdf`, a copy of `testdata/host.pdf`). What it writes is ignored by git.
- The output is deterministic: no current date, no random value.

`go test` builds and runs every example and compares what it prints with its `// Output:` block.

## Benchmarks and compliance

The two pages read their figures from `data/`. Update `benchmarks.json` when the wiki publishes new results: each chart lists its rows from the fastest, with the time in microseconds or the reason of a failure, and the scale is computed from them. `compliance.json` lists, per country, the B2G and B2B obligations and the formats ZeRechnung covers, and for the countries with a guide the checkpoints and the rules that verify them. The texts they refer to are keys of the i18n files, and `go test` checks that they exist.

## Translate

Translations are welcome, in every language of Europe.

To add a language, copy `i18n/en.json` to `i18n/<code>.json`, where `<code>` is the ISO 639-1 code of the language (`it`, `pl`, `sv`…). Then:

- set `"lang"` to that code and `"name"` to the name of the language in that language: `"Italiano"`, `"Polski"`, `"Svenska"`;
- translate every text, and keep the keys as they are;
- translate the page paths (`examples_slug`, `benchmarks_slug`, `compliance_slug`): lower-case letters, digits and hyphens;
- write at least three examples for the language (see above);
- set `"reviewed"` to `true` only if a native speaker has proofread the texts. Until then, the page says that the translation has not been reviewed and links to its file.

To correct a translation, edit its file: the "Edit this translation" link at the bottom of each page opens it on GitHub.

A few conventions:

- Text between backquotes is code and stays as is: `` `go get` ``, `` `invoice.Invoice` ``. `**text**` is shown in bold. No other markup is allowed.
- `{1}`, `{2}` are replaced by values (a year, a number, a date): keep them.
- Format, standard and tool names are not translated: Factur-X, ZUGFeRD, XRechnung, Peppol, KoSIT, Mustang, veraPDF, EN 16931, PDF/A-3.
- The numbers of the accuracy section and the code examples are in the template, the same for every language.
- French spacing before `: ; ! ?` is added by the generator: write a normal space.

`go test` fails when a translation lacks a text, leaves one empty, or has one that `en.json` does not have, and when a language has fewer than three examples.
