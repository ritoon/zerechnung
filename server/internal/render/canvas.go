package render

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

// Page geometry, in points: A4 with margins of 17 mm.
const (
	pageW   = 595.28
	pageH   = 841.89
	left    = 48.0
	right   = pageW - 48.0
	width   = right - left
	top     = pageH - 46.0
	bottom  = 62.0 // lowest baseline of the content; the footer is below
	footerY = 36.0
)

// rgb is a colour of the DeviceRGB space, which the sRGB output intent
// characterises.
type rgb [3]float64

var (
	ink    = rgb{0.13, 0.14, 0.16}
	muted  = rgb{0.40, 0.42, 0.46}
	accent = rgb{0.09, 0.25, 0.45}
	rule   = rgb{0.80, 0.82, 0.85}
	band   = rgb{0.93, 0.95, 0.97}
)

func (c rgb) fill() string   { return num(c[0]) + " " + num(c[1]) + " " + num(c[2]) + " rg" }
func (c rgb) stroke() string { return num(c[0]) + " " + num(c[1]) + " " + num(c[2]) + " RG" }

// canvas draws the pages of a document, top to bottom, and starts a new page
// when the content reaches the bottom margin.
type canvas struct {
	l         *labels
	reg, bold *usedFont
	pages     []*bytes.Buffer
	cur       *bytes.Buffer
	y         float64 // baseline of the next line
	// onNewPage draws the header of the pages after the first one, and the
	// header of a table being drawn across pages.
	onNewPage []func()
}

func newCanvas(l *labels, fs [2]*face) *canvas {
	c := &canvas{l: l, reg: newUsedFont(fs[0], "F1"), bold: newUsedFont(fs[1], "F2")}
	c.addPage()
	return c
}

func (c *canvas) addPage() {
	c.cur = &bytes.Buffer{}
	c.pages = append(c.pages, c.cur)
	c.y = top
}

// need makes room for h points below the current position, on a new page
// when the current one has not enough. It reports whether a page was added.
func (c *canvas) need(h float64) bool {
	if c.y-h >= bottom {
		return false
	}
	c.addPage()
	for _, f := range c.onNewPage {
		f()
	}
	return true
}

// text draws s with its baseline at y.
func (c *canvas) text(f *usedFont, size, x, y float64, s string, col rgb) {
	gids := f.encode(s)
	if len(gids) == 0 {
		return
	}
	var hex strings.Builder
	for _, g := range gids {
		fmt.Fprintf(&hex, "%04X", g)
	}
	fmt.Fprintf(c.cur, "BT /%s %s Tf %s 1 0 0 1 %s %s Tm <%s> Tj ET\n", f.res, num(size), col.fill(), num(x), num(y), hex.String())
}

// textRight draws s ending at x.
func (c *canvas) textRight(f *usedFont, size, x, y float64, s string, col rgb) {
	c.text(f, size, x-f.measure(s, size), y, s, col)
}

// rect fills a rectangle whose lower left corner is (x, y).
func (c *canvas) rect(x, y, w, h float64, col rgb) {
	fmt.Fprintf(c.cur, "%s %s %s %s %s re f\n", col.fill(), num(x), num(y), num(w), num(h))
}

// hline draws a horizontal rule.
func (c *canvas) hline(x1, x2, y, w float64, col rgb) {
	fmt.Fprintf(c.cur, "%s %s w %s %s m %s %s l S\n", col.stroke(), num(w), num(x1), num(y), num(x2), num(y))
}

// wrap breaks s into lines no wider than w at size; a word wider than w is
// broken between characters. Line breaks of s are kept.
func wrap(f *face, size float64, s string, w float64) []string {
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		words := strings.FieldsFunc(para, unicode.IsSpace)
		if len(words) == 0 {
			continue
		}
		line := ""
		for _, word := range words {
			for f.measure(word, size) > w {
				// Break a long word: the part that fits on the line.
				cut := len(word)
				for cut > 0 && f.measure(word[:cut], size) > w {
					cut = prevRune(word, cut)
				}
				if cut == 0 {
					cut = nextRune(word, 0)
				}
				if line != "" {
					out = append(out, line)
					line = ""
				}
				out = append(out, word[:cut])
				word = word[cut:]
			}
			if word == "" {
				continue
			}
			switch {
			case line == "":
				line = word
			case f.measure(line+" "+word, size) <= w:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// prevRune returns the index of the rune that ends before i.
func prevRune(s string, i int) int {
	for i--; i > 0 && !isRuneStart(s[i]); i-- {
	}
	return max(i, 0)
}

// nextRune returns the index of the rune after the one at i.
func nextRune(s string, i int) int {
	for i++; i < len(s) && !isRuneStart(s[i]); i++ {
	}
	return i
}

func isRuneStart(b byte) bool { return b&0xc0 != 0x80 }

// fit shortens s with an ellipsis so that it is no wider than w.
func fit(f *face, size float64, s string, w float64) string {
	if f.measure(s, size) <= w {
		return s
	}
	for s != "" && f.measure(s+"…", size) > w {
		s = s[:prevRune(s, len(s))]
	}
	return s + "…"
}
