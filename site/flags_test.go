package main

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// TestFlags checks that every flag is well-formed SVG, keyed by an ISO 3166-1
// alpha-2 code.
func TestFlags(t *testing.T) {
	codeRE := regexp.MustCompile(`^[A-Z]{2}$`)
	p := new(page)
	for code := range flags {
		if !codeRE.MatchString(code) {
			t.Errorf("flag %q: not an ISO 3166-1 alpha-2 code", code)
		}
		svg, err := p.Flag(code)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(svg)))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("flag %s: %v", code, err)
				break
			}
		}
	}
	if _, err := p.Flag("XX"); err == nil {
		t.Error("no error for a country without a flag")
	}
}

func TestLoadLangsChecksFlag(t *testing.T) {
	fsys := fstest.MapFS{
		"i18n/en.json": {Data: []byte(`{"lang":"en","name":"English","flag":"EN","text":{}}`)},
	}
	if _, err := loadLangs(fsys); err == nil || !strings.Contains(err.Error(), `"flag" "EN"`) {
		t.Fatalf("got %v, want an error on the flag", err)
	}
}

// TestBuildShowsFlags checks that the compliance page shows the flag of
// each country and that the language menu shows the flag of each language.
func TestBuildShowsFlags(t *testing.T) {
	out := t.TempDir()
	if err := build(out, "https://example.org/zerechnung/"); err != nil {
		t.Fatal(err)
	}
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := loadComp(dataFS, langs)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range langs {
		for _, s := range sections {
			html := readFile(t, out, s.path(l)+"index.html")
			for _, m := range langs {
				if !strings.Contains(html, flags[m.Flag]+`</svg> `+m.Name+`</a>`) {
					t.Errorf("%s: no flag before %s in the language menu", s.path(l), m.Name)
				}
			}
			if s.name != "compliance" {
				continue
			}
			if strings.Contains(html, `class="cc"`) {
				t.Errorf("%s: country codes instead of flags", s.path(l))
			}
			for _, c := range comp.Countries {
				if !strings.Contains(html, flags[c.Code]+`</svg> `+string(markup(typography(l.Code, l.Text["country_"+c.Code])))) {
					t.Errorf("%s: no flag before the name of %s", s.path(l), c.Code)
				}
			}
		}
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
