// Command site builds the presentation site of ZeRechnung: for each
// language, a home page, a page of worked examples, a page of benchmarks and
// a page of compliance by country, from the templates, the translations, the
// example programs and the data files embedded below, plus a language chooser
// at the root, a 404 page and a sitemap.
//
//	go run . -out public
//
// Each file of i18n/ is one language. A language is added by copying
// i18n/en.json and translating its texts; the build fails if a translation
// lacks a text or has one the English file does not.
//
// Each directory of examples/ is a complete Go program shown on the
// examples page of one language (see parseExample). The tests run every
// program and compare what it prints with the output shown on the page.
//
// The files of data/ hold the figures of the benchmarks and compliance pages;
// their texts are in the translations.
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
	"strconv"
	"strings"
)

//go:embed i18n/*.json
var i18nFS embed.FS

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

//go:embed examples/*/main.go
var examplesFS embed.FS

//go:embed data/*.json
var dataFS embed.FS

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
	// Flag is the country whose flag the language menu shows before the
	// name, as an ISO 3166-1 alpha-2 code of flags.go: "DE", "GB".
	Flag string `json:"flag"`
	// Reviewed is false until a native speaker has proofread the texts;
	// the page then says so and asks for corrections.
	Reviewed bool              `json:"reviewed"`
	Text     map[string]string `json:"text"`
}

// section is a page of the site, generated in every language.
type section struct {
	name string // name of its template, without .html, and of the links to it
	slug string // key of the translated last element of its path; none for home
	meta string // prefix of the keys of its title and description
}

var sections = []section{
	{name: "home"},
	{name: "examples", slug: "examples_slug", meta: "examples_"},
	{name: "benchmarks", slug: "benchmarks_slug", meta: "benchmarks_"},
	{name: "compliance", slug: "compliance_slug", meta: "compliance_"},
}

// path returns the path of the section in l from the site root: /de/,
// /de/beispiele/, /fr/exemples/.
func (s section) path(l *lang) string {
	if s.slug == "" {
		return l.Code + "/"
	}
	return l.Code + "/" + l.Text[s.slug] + "/"
}

// page is the data of one generated page.
type page struct {
	Lang        *lang
	Langs       []*lang
	Section     string                   // name of the section of the page
	T           map[string]template.HTML // texts, with `code` and **strong** marked up
	P           map[string]string        // the same texts as plain text, for attributes
	Title       string
	Description string
	Canonical   string
	XDefault    string // absolute URL of the hreflang x-default alternate
	Base        string // absolute URL of the site, ending with a slash
	Root        string // path from the page to the site root, ending with a slash
	Repo        string
	MenuRows    int
	Examples    []*example // the examples of the language
	Bench       *benchData
	Comp        *compData

	path   func(*lang) string // path of the same page in another language
	assets map[string]string  // static file name to content hash
}

// Href returns the link from the page to the same page in l.
func (p *page) Href(l *lang) string { return p.Root + p.path(l) }

// Abs returns the absolute URL of the same page in l.
func (p *page) Abs(l *lang) string { return p.Base + p.path(l) }

// Link returns the link from the page to a section, in the language of the
// page.
func (p *page) Link(name string) (string, error) {
	for _, s := range sections {
		if s.name == name {
			return p.Root + s.path(p.Lang), nil
		}
	}
	return "", fmt.Errorf("no section %q", name)
}

// Fmt returns the text of key with {1}, {2}… replaced by args.
func (p *page) Fmt(key string, args ...string) (template.HTML, error) {
	t, ok := p.T[key]
	if !ok {
		return "", fmt.Errorf("no text %q", key)
	}
	s := string(t)
	for i, a := range args {
		s = strings.ReplaceAll(s, "{"+strconv.Itoa(i+1)+"}", html.EscapeString(a))
	}
	return template.HTML(s), nil
}

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

// ExamplesURL returns the GitHub folder of the example programs.
func (p *page) ExamplesURL() string { return p.Repo + "/tree/main/site/examples" }

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
	examples, err := loadExamples(examplesFS, langs)
	if err != nil {
		return err
	}
	assets, err := hashAssets(staticFS)
	if err != nil {
		return err
	}
	tmpl, err := template.New("").Option("missingkey=error").Funcs(template.FuncMap{
		"dict":  dict,
		"inc":   func(i int) int { return i + 1 },
		"odd":   func(i int) bool { return i%2 == 1 },
		"dec":   func(i int) int { return i - 1 },
		"lower": strings.ToLower,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return err
	}

	bench, err := loadBench(dataFS, langs)
	if err != nil {
		return err
	}
	comp, err := loadComp(dataFS, langs)
	if err != nil {
		return err
	}

	var en *lang
	for _, l := range langs {
		if l.Code == sourceLang {
			en = l
		}
	}
	newPage := func(l *lang, s section) *page {
		p := &page{
			Lang:     l,
			Langs:    langs,
			Section:  s.name,
			T:        make(map[string]template.HTML, len(l.Text)),
			P:        make(map[string]string, len(l.Text)),
			XDefault: base,
			Base:     base,
			Root:     strings.Repeat("../", strings.Count(s.path(l), "/")),
			Repo:     repoURL,
			MenuRows: min(len(langs), menuRows),
			Examples: examples[l.Code],
			Bench:    bench,
			Comp:     comp,
			path:     s.path,
			assets:   assets,
		}
		for k, v := range l.Text {
			v = typography(l.Code, v)
			p.T[k] = markup(v)
			p.P[k] = plain(v)
		}
		p.Canonical = base + s.path(l)
		if s.slug != "" {
			p.XDefault = base + s.path(en)
		}
		p.Title = p.P[s.meta+"meta_title"]
		p.Description = p.P[s.meta+"meta_description"]
		return p
	}

	for _, l := range langs {
		for _, s := range sections {
			p := newPage(l, s)
			file := filepath.Join(out, filepath.FromSlash(s.path(l)), "index.html")
			if err := render(tmpl, s.name+".html", p, file); err != nil {
				return err
			}
		}
	}
	// The root page lists the languages and sends the visitor to the one
	// of the browser. The 404 page is served at any depth, so its links
	// start from the root of the site.
	root := newPage(en, sections[0])
	root.Root = ""
	root.Canonical = base
	if err := render(tmpl, "root.html", root, filepath.Join(out, "index.html")); err != nil {
		return err
	}
	notFound := newPage(en, sections[0])
	notFound.Root = basePath
	notFound.Canonical = ""
	if err := render(tmpl, "404.html", notFound, filepath.Join(out, "404.html")); err != nil {
		return err
	}
	if err := copyStatic(staticFS, out); err != nil {
		return err
	}
	sm := sitemap(base, langs, en)
	if err := os.WriteFile(filepath.Join(out, "sitemap.xml"), sm, 0o644); err != nil {
		return err
	}
	robots := "User-agent: *\nAllow: /\n\nSitemap: " + base + "sitemap.xml\n"
	return os.WriteFile(filepath.Join(out, "robots.txt"), []byte(robots), 0o644)
}

// dict makes a map of its arguments, keys and values in turn, to pass
// several values to a template.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// slugRE is the form of a translated path element.
var slugRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

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
		if _, ok := flags[l.Flag]; !ok {
			return nil, fmt.Errorf("%s: \"flag\" %q: not a country of flags.go", name, l.Flag)
		}
		slugs := make(map[string]string)
		for _, s := range sections {
			v, ok := l.Text[s.slug]
			if s.slug == "" || !ok {
				continue
			}
			if !slugRE.MatchString(v) {
				return nil, fmt.Errorf("%s: %s %q: use lower-case letters, digits and hyphens", name, s.slug, v)
			}
			if k, dup := slugs[v]; dup {
				return nil, fmt.Errorf("%s: %s and %s are both %q", name, k, s.slug, v)
			}
			slugs[v] = s.slug
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

// sitemap lists the pages of every language, each with its alternates, as
// search engines expect for a multilingual site.
func sitemap(base string, langs []*lang, en *lang) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, s := range sections {
		xDefault := base + s.path(en)
		if s.slug == "" {
			xDefault = base
		}
		for _, l := range langs {
			fmt.Fprintf(&b, "  <url>\n    <loc>%s</loc>\n", html.EscapeString(base+s.path(l)))
			for _, alt := range langs {
				fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", alt.Code, html.EscapeString(base+s.path(alt)))
			}
			fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n  </url>\n", html.EscapeString(xDefault))
		}
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}
