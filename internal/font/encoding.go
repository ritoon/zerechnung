package font

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// glyphList is the Adobe Glyph List 2.0, unchanged, with its licence.
//
//go:embed glyphlist.txt
var glyphList string

// agl maps the names of the Adobe Glyph List to the first character of
// their value.
var agl = sync.OnceValue(func() map[string]rune {
	m := make(map[string]rune, 4300)
	for _, line := range strings.Split(glyphList, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ";")
		if !ok || strings.HasPrefix(name, "#") {
			continue
		}
		first, _, _ := strings.Cut(value, " ")
		if v, err := strconv.ParseUint(first, 16, 32); err == nil {
			m[name] = rune(v)
		}
	}
	return m
})

// InGlyphList reports whether name is a name of the Adobe Glyph List.
func InGlyphList(name string) bool {
	_, ok := agl()[name]
	return ok
}

// Encoding returns the glyph names of a standard encoding of ISO 32000-1
// (StandardEncoding, WinAnsiEncoding, MacRomanEncoding) by code.
func Encoding(name string) (*[256]string, bool) {
	switch name {
	case "StandardEncoding":
		return &standardEncoding, true
	case "WinAnsiEncoding":
		return &winAnsiEncoding, true
	case "MacRomanEncoding":
		return &macRomanEncoding, true
	}
	return nil, false
}

// GlyphList returns the character the Adobe Glyph List gives a glyph
// name: the first one of a name that stands for several.
func GlyphList(name string) (rune, bool) {
	r, ok := agl()[name]
	return r, ok
}

// MacRomanCode returns the code of a glyph name in MacRomanEncoding.
func MacRomanCode(name string) (byte, bool) {
	for code, n := range macRomanEncoding {
		if n == name && n != "" {
			return byte(code), true
		}
	}
	return 0, false
}
