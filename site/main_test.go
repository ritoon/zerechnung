package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadLangs(t *testing.T) {
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) < 2 {
		t.Fatalf("got %d languages, want the English one and at least one other", len(langs))
	}
	for i := 1; i < len(langs); i++ {
		if langs[i-1].Code >= langs[i].Code {
			t.Errorf("languages not sorted: %s before %s", langs[i-1].Code, langs[i].Code)
		}
	}
}

func TestLoadLangsReportsMissingTexts(t *testing.T) {
	fsys := fstest.MapFS{
		"i18n/en.json": {Data: []byte(`{"lang":"en","name":"English","reviewed":true,"text":{"a":"A","b":"B"}}`)},
		"i18n/xx.json": {Data: []byte(`{"lang":"xx","name":"Xx","text":{"a":" ","c":"C"}}`)},
	}
	_, err := loadLangs(fsys)
	if err == nil {
		t.Fatal("no error for an incomplete translation")
	}
	for _, want := range []string{"i18n/xx.json", "missing: b", "empty: a", "not in en.json: c"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestLoadLangsChecksCode(t *testing.T) {
	fsys := fstest.MapFS{
		"i18n/en.json": {Data: []byte(`{"lang":"en","name":"English","text":{}}`)},
		"i18n/de.json": {Data: []byte(`{"lang":"fr","name":"Français","text":{}}`)},
	}
	if _, err := loadLangs(fsys); err == nil || !strings.Contains(err.Error(), `want "de"`) {
		t.Fatalf("got %v, want an error on the language code", err)
	}
}

func TestMarkup(t *testing.T) {
	tests := []struct{ in, html, plain string }{
		{"`go get` <b>", "<code>go get</code> &lt;b&gt;", "go get <b>"},
		{"a **bold** word", "a <strong>bold</strong> word", "a bold word"},
		{"l'API", "l&#39;API", "l'API"},
		{"en Factur-X.", `en <span class="nw">Factur-X</span>.`, "en Factur-X."},
		{"PDF/A-3 file", `<span class="nw">PDF/A-3</span> file`, "PDF/A-3 file"},
	}
	for _, tt := range tests {
		if got := string(markup(tt.in)); got != tt.html {
			t.Errorf("markup(%q) = %q, want %q", tt.in, got, tt.html)
		}
		if got := plain(tt.in); got != tt.plain {
			t.Errorf("plain(%q) = %q, want %q", tt.in, got, tt.plain)
		}
	}
}

func TestTypography(t *testing.T) {
	tests := []struct{ code, in, want string }{
		{"en", "about 25 µs", "about 25 µs"},
		{"en", "Version 0: the API", "Version 0: the API"},
		{"fr", "Version 0 : l'API", "Version 0 : l'API"},
		{"fr", "Quelle fiabilité ?", "Quelle fiabilité ?"},
		{"fr", "« oui »", "« oui »"},
	}
	for _, tt := range tests {
		if got := typography(tt.code, tt.in); got != tt.want {
			t.Errorf("typography(%q, %q) = %q, want %q", tt.code, tt.in, got, tt.want)
		}
	}
}

func TestBuild(t *testing.T) {
	out := t.TempDir()
	const base = "https://example.org/zerechnung/"
	if err := build(out, base); err != nil {
		t.Fatal(err)
	}
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}

	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, name := range []string{"index.html", "404.html", "sitemap.xml", "robots.txt", "style.css", "site.js", "gopher.webp", "favicon.svg"} {
		read(name)
	}

	var en *lang
	for _, l := range langs {
		if l.Code == sourceLang {
			en = l
		}
	}
	hrefRE := regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	preRE := regexp.MustCompile(`(?s)<pre>.*?</pre>`)
	for _, l := range langs {
		for _, sec := range sections {
			name := filepath.Join(filepath.FromSlash(sec.path(l)), "index.html")
			html := read(name)
			if !strings.Contains(html, `<html lang="`+l.Code+`">`) {
				t.Errorf("%s: no <html lang=%q>", name, l.Code)
			}
			if !strings.Contains(html, `<link rel="canonical" href="`+base+sec.path(l)+`">`) {
				t.Errorf("%s: no canonical link", name)
			}
			for _, alt := range langs {
				if !strings.Contains(html, `hreflang="`+alt.Code+`" href="`+base+sec.path(alt)+`"`) {
					t.Errorf("%s: no hreflang link to %s", name, alt.Code)
				}
			}
			xDefault := base + sec.path(en)
			if sec.slug == "" {
				xDefault = base
			}
			if !strings.Contains(html, `hreflang="x-default" href="`+xDefault+`"`) {
				t.Errorf("%s: x-default is not %s", name, xDefault)
			}
			// Go code may contain {{ and backquotes; the texts may not.
			if text := preRE.ReplaceAllString(html, ""); strings.Contains(text, "{{") || strings.Contains(text, "`") {
				t.Errorf("%s: unrendered template or markup", name)
			}
			if got, want := strings.Contains(html, `class="notice"`), !l.Reviewed; got != want {
				t.Errorf("%s: review notice shown = %v, want %v", name, got, want)
			}
			// Every relative link leads to a generated file.
			for _, m := range hrefRE.FindAllStringSubmatch(html, -1) {
				href := m[1]
				if strings.Contains(href, ":") || strings.HasPrefix(href, "#") {
					continue
				}
				href, _, _ = strings.Cut(href, "?")
				href, _, _ = strings.Cut(href, "#")
				target := filepath.Join(out, filepath.FromSlash(sec.path(l)), filepath.FromSlash(href))
				if strings.HasSuffix(href, "/") {
					target = filepath.Join(target, "index.html")
				}
				if _, err := os.Stat(target); err != nil {
					t.Errorf("%s: link %q leads nowhere", name, m[1])
				}
			}
		}
	}

	sitemap := read("sitemap.xml")
	if got, want := strings.Count(sitemap, "<loc>"), len(langs)*len(sections); got != want {
		t.Errorf("sitemap has %d pages, want %d", got, want)
	}
	if !strings.Contains(read("robots.txt"), "Sitemap: "+base+"sitemap.xml") {
		t.Error("robots.txt does not point to the sitemap")
	}
	// The 404 page is served at any depth: its links start from the root.
	if !strings.Contains(read("404.html"), `href="/zerechnung/en/"`) {
		t.Error("404.html: links are not absolute paths")
	}
}

func TestBuildRejectsRelativeBase(t *testing.T) {
	if err := build(t.TempDir(), "/zerechnung/"); err == nil {
		t.Fatal("no error for a relative base URL")
	}
}
