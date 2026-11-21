// Command site builds the presentation site of ZeRechnung: one page per
// language, from the template and the translations embedded below, plus a
// language chooser at the root, a 404 page and a sitemap.
//
//	go run . -out public
//
// Each file of i18n/ is one language. A language is added by copying
// i18n/en.json and translating its texts; the build fails if a translation
// lacks a text or has one the English file does not.
package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

//go:embed i18n/*.json
var i18nFS embed.FS

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	defaultBase = "https://ritoon.github.io/zerechnung/"
	repoURL     = "https://github.com/ritoon/zerechnung"
	sourceLang  = "en"
	// menuRows is the number of languages per column in the language menu.
	menuRows = 12
)

// lang is one translation of the site, read from i18n/<code>.json.
type lang struct {
	Code string `json:"lang"`
	// Name is the name of the language in that language: "Deutsch".
	Name string `json:"name"`
	// Reviewed is false until a native speaker has proofread the texts;
	// the page then says so and asks for corrections.
	Reviewed bool              `json:"reviewed"`
	Text     map[string]string `json:"text"`
}

// page is the data of one generated page.
type page struct {
	Lang        *lang
	Langs       []*lang
	T           map[string]template.HTML // texts, with `code` and **strong** marked up
	P           map[string]string        // the same texts as plain text, for attributes
	Title       string
	Description string
	Canonical   string
	Base        string // absolute URL of the site, ending with a slash
	Root        string // path from the page to the site root, ending with a slash
	Repo        string
	MenuRows    int
	assets      map[string]string // static file name to content hash
}

// Href returns the link from the page to the page of l.
func (p *page) Href(l *lang) string { return p.Root + l.Code + "/" }

// Abs returns the absolute URL of the page of l.
func (p *page) Abs(l *lang) string { return p.Base + l.Code + "/" }

// Asset returns the link to a static file, with its content hash so that
// browsers fetch it again when it changes.
func (p *page) Asset(name string) (string, error) {
	h, ok := p.assets[name]
	if !ok {
		return "", fmt.Errorf("unknown static file %q", name)
	}
	return p.Root + name + "?v=" + h, nil
}

// EditURL returns the GitHub page that edits the translation of l.
func (p *page) EditURL(l *lang) string {
	return p.Repo + "/edit/main/site/i18n/" + l.Code + ".json"
}

// Codes returns the language codes, for the redirection of the root page.
func (p *page) Codes() []string {
	codes := make([]string, len(p.Langs))
	for i, l := range p.Langs {
		codes[i] = l.Code
	}
	return codes
}

func main() {
	out := flag.String("out", "public", "output `directory`")
	base := flag.String("base", defaultBase, "absolute `URL` of the site, for canonical links and the sitemap")
	flag.Parse()
	if err := build(*out, *base); err != nil {
		log.Fatal(err)
	}
}

// build writes the whole site to out.
func build(out, base string) error {
	u, err := url.Parse(base)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("-base %q: not an absolute URL", base)
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	basePath := u.Path
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	langs, err := loadLangs(i18nFS)
	if err != nil {
		return err
	}
	assets, err := hashAssets(staticFS)
	if err != nil {
		return err
	}
	tmpl, err := template.New("").Option("missingkey=error").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return err
	}

	var en *lang
	for _, l := range langs {
		if l.Code == sourceLang {
			en = l
		}
	}
	newPage := func(l *lang, root, canonical string) *page {
		p := &page{
			Lang:      l,
			Langs:     langs,
			T:         make(map[string]template.HTML, len(l.Text)),
			P:         make(map[string]string, len(l.Text)),
			Canonical: canonical,
			Base:      base,
			Root:      root,
			Repo:      repoURL,
			MenuRows:  min(len(langs), menuRows),
			assets:    assets,
		}
		for k, v := range l.Text {
			v = typography(l.Code, v)
			p.T[k] = markup(v)
			p.P[k] = plain(v)
		}
		p.Title = p.P["meta_title"]
		p.Description = p.P["meta_description"]
		return p
	}

	for _, l := range langs {
		p := newPage(l, "../", base+l.Code+"/")
		if err := render(tmpl, "page.html", p, filepath.Join(out, l.Code, "index.html")); err != nil {
			return err
		}
	}
	// The root page lists the languages and sends the visitor to the one
	// of the browser. The 404 page is served at any depth, so its links
	// start from the root of the site.
	if err := render(tmpl, "root.html", newPage(en, "", base), filepath.Join(out, "index.html")); err != nil {
		return err
	}
	if err := render(tmpl, "404.html", newPage(en, basePath, ""), filepath.Join(out, "404.html")); err != nil {
		return err
	}
	if err := copyStatic(staticFS, out); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "sitemap.xml"), sitemap(base, langs), 0o644); err != nil {
		return err
	}
	robots := "User-agent: *\nAllow: /\n\nSitemap: " + base + "sitemap.xml\n"
	return os.WriteFile(filepath.Join(out, "robots.txt"), []byte(robots), 0o644)
}

// loadLangs reads every translation, sorted by language code, and checks
// that each has the texts of the English one, no more, no less.
func loadLangs(fsys fs.FS) ([]*lang, error) {
	files, err := fs.Glob(fsys, "i18n/*.json")
	if err != nil {
		return nil, err
	}
	var langs []*lang
	for _, name := range files {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		l := new(lang)
		if err := dec.Decode(l); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if want := strings.TrimSuffix(path.Base(name), ".json"); l.Code != want {
			return nil, fmt.Errorf("%s: \"lang\" is %q, want %q", name, l.Code, want)
		}
		if l.Name == "" {
			return nil, fmt.Errorf("%s: \"name\" is empty", name)
		}
		langs = append(langs, l)
	}
	slices.SortFunc(langs, func(a, b *lang) int { return strings.Compare(a.Code, b.Code) })

	i := slices.IndexFunc(langs, func(l *lang) bool { return l.Code == sourceLang })
	if i < 0 {
		return nil, fmt.Errorf("i18n/%s.json is missing", sourceLang)
	}
	var errs []error
	for _, l := range langs {
		if err := checkKeys(langs[i], l); err != nil {
			errs = append(errs, err)
		}
	}
	return langs, errors.Join(errs...)
}

// checkKeys reports the texts of src that l lacks or leaves empty, and the
// texts of l that src does not have.
func checkKeys(src, l *lang) error {
	var missing, extra, empty []string
	for k := range src.Text {
		v, ok := l.Text[k]
		switch {
		case !ok:
			missing = append(missing, k)
		case strings.TrimSpace(v) == "":
			empty = append(empty, k)
		}
	}
	for k := range l.Text {
		if _, ok := src.Text[k]; !ok {
			extra = append(extra, k)
		}
	}
	var msgs []string
	for _, s := range []struct {
		what string
		keys []string
	}{{"missing", missing}, {"empty", empty}, {"not in " + src.Code + ".json", extra}} {
		if len(s.keys) > 0 {
			slices.Sort(s.keys)
			msgs = append(msgs, s.what+": "+strings.Join(s.keys, ", "))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("i18n/%s.json: %s", l.Code, strings.Join(msgs, "; "))
}

var (
	codeRE   = regexp.MustCompile("`([^`]+)`")
	strongRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// A hyphenated name, kept on one line: Factur-X, CIUS-IT, PDF/A-3.
	hyphenRE = regexp.MustCompile(`[\pL\pN]+(?:[/.][\pL\pN]+)*(?:-[\pL\pN]+(?:[/.][\pL\pN]+)*)+`)
	// A number followed by a space and a word or a unit: "25 µs".
	numberRE = regexp.MustCompile(`(\d) (\pL)`)
	// French puts a no-break space before the colon, a narrow one before
	// ; ! ? » and after «.
	frColonRE  = regexp.MustCompile(` :`)
	frBeforeRE = regexp.MustCompile(` ([;!?»])`)
	frAfterRE  = regexp.MustCompile(`« `)
)

// typography replaces the spaces that must not break.
func typography(code, s string) string {
	s = numberRE.ReplaceAllString(s, "$1 $2")
	if code == "fr" {
		s = frColonRE.ReplaceAllString(s, " :")
		s = frBeforeRE.ReplaceAllString(s, " $1")
		s = frAfterRE.ReplaceAllString(s, "« ")
	}
	return s
}

// markup escapes s and turns `code` and **strong** into HTML. Translations
// contain no other markup. Hyphenated words are kept on one line, so that
// "Factur-X" never ends a line with "Factur-".
func markup(s string) template.HTML {
	s = html.EscapeString(s)
	s = hyphenRE.ReplaceAllString(s, `<span class="nw">$0</span>`)
	s = codeRE.ReplaceAllString(s, "<code>$1</code>")
	s = strongRE.ReplaceAllString(s, "<strong>$1</strong>")
	return template.HTML(s)
}

// plain removes the marks that markup turns into HTML.
func plain(s string) string {
	s = codeRE.ReplaceAllString(s, "$1")
	return strongRE.ReplaceAllString(s, "$1")
}

func render(tmpl *template.Template, name string, p *page, file string) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, p); err != nil {
		return fmt.Errorf("%s (%s): %w", name, p.Lang.Code, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, buf.Bytes(), 0o644)
}

func hashAssets(fsys fs.FS) (map[string]string, error) {
	hashes := make(map[string]string)
	err := fs.WalkDir(fsys, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashes[strings.TrimPrefix(name, "static/")] = hex.EncodeToString(sum[:4])
		return nil
	})
	return hashes, err
}

func copyStatic(fsys fs.FS, out string) error {
	return fs.WalkDir(fsys, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		file := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(name, "static/")))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		return os.WriteFile(file, data, 0o644)
	})
}

// sitemap lists the page of every language, each with its alternates, as
// search engines expect for a multilingual site.
func sitemap(base string, langs []*lang) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, l := range langs {
		fmt.Fprintf(&b, "  <url>\n    <loc>%s</loc>\n", html.EscapeString(base+l.Code+"/"))
		for _, alt := range langs {
			fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", alt.Code, html.EscapeString(base+alt.Code+"/"))
		}
		fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n  </url>\n", html.EscapeString(base))
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}
