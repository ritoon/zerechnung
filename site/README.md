# The ZeRechnung website

The presentation site of ZeRechnung, one page per language, published on GitHub Pages by the [Site workflow](../.github/workflows/site.yml) at every push to `main`.

It is a separate Go module with no dependency, so the package itself stays untouched. `go run .` generates static HTML: no JavaScript framework, no external font or script, nothing that tracks visitors.

| Path | Content |
|---|---|
| `i18n/` | One JSON file per language: its code, its name in that language, and the texts |
| `templates/` | The page, the language chooser at the root and the 404 page (`html/template`) |
| `static/` | Style sheet, script, images, copied as is |
| `main.go` | The generator |

## Build and preview

```bash
cd site
go test ./...
go run . -out public -base http://localhost:8000/
python3 -m http.server -d public 8000
```

Then open http://localhost:8000/. Without `-base`, the canonical links and the sitemap point to the published site.

## Translate

Translations are welcome, in every language of Europe.

To add a language, copy `i18n/en.json` to `i18n/<code>.json`, where `<code>` is the ISO 639-1 code of the language (`it`, `pl`, `sv`…). Then:

- set `"lang"` to that code and `"name"` to the name of the language in that language: `"Italiano"`, `"Polski"`, `"Svenska"`;
- translate every text, and keep the keys as they are;
- set `"reviewed"` to `true` only if a native speaker has proofread the texts. Until then, the page says that the translation has not been reviewed and links to its file.

To correct a translation, edit its file: the "Edit this translation" link at the bottom of each page opens it on GitHub.

A few conventions:

- Text between backquotes is code and stays as is: `` `go get` ``, `` `invoice.Invoice` ``. `**text**` is shown in bold. No other markup is allowed.
- Format, standard and tool names are not translated: Factur-X, ZUGFeRD, XRechnung, Peppol, KoSIT, Mustang, veraPDF, EN 16931, PDF/A-3.
- The numbers of the accuracy section and the code examples are in the template, the same for every language.
- French spacing before `: ; ! ?` is added by the generator: write a normal space.

`go test` fails when a translation lacks a text, leaves one empty, or has one that `en.json` does not have.
