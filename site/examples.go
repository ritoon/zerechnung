package main

import (
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// example is a worked example: a complete Go program, cut into steps.
type example struct {
	Lang   string // language of the page that shows it
	Name   string // directory name: de-1-baeckerei
	Anchor string // HTML id: baeckerei
	order  int

	Title  string
	Tags   []string
	Intro  []template.HTML // paragraphs
	Steps  []step
	Output string        // what the program prints
	Source template.HTML // the whole program, highlighted
}

// step is one step of an example: its explanation and its code.
type step struct {
	Title string
	Text  []template.HTML // paragraphs
	Code  template.HTML   // highlighted
}

// exampleDirRE is the form of an example directory: language, order, name.
var exampleDirRE = regexp.MustCompile(`^([a-z]{2})-([0-9]+)-([a-z0-9-]+)$`)

// loadExamples reads the example programs, grouped by language and sorted.
func loadExamples(fsys fs.FS, langs []*lang) (map[string][]*example, error) {
	files, err := fs.Glob(fsys, "examples/*/main.go")
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(langs))
	for _, l := range langs {
		known[l.Code] = true
	}
	byLang := make(map[string][]*example)
	for _, name := range files {
		dir := path.Base(path.Dir(name))
		m := exampleDirRE.FindStringSubmatch(dir)
		if m == nil {
			return nil, fmt.Errorf("%s: name the directory <lang>-<n>-<name>, as de-1-baeckerei", name)
		}
		if !known[m[1]] {
			return nil, fmt.Errorf("%s: no translation for the language %q", name, m[1])
		}
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		ex, err := parseExample(m[1], string(src))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		ex.Name, ex.Anchor = dir, m[3]
		ex.order, _ = strconv.Atoi(m[2])
		byLang[ex.Lang] = append(byLang[ex.Lang], ex)
	}
	for _, exs := range byLang {
		slices.SortFunc(exs, func(a, b *example) int { return a.order - b.order })
	}
	return byLang, nil
}

// parseExample cuts an example program into the parts the page shows. The
// program follows this layout:
//
//	// Title: the title of the example
//	// Tags: Factur-X, EN 16931, TVA 5,5 %
//	//
//	// The introduction: one or more paragraphs, separated by an empty
//	// comment line.
//	package main
//
//	import (...)
//
//	func main() {
//		// ## The title of the first step
//		// Its explanation, in comment lines right after the title.
//		code of the step
//
//		// ## The title of the next step
//		...
//	}
//
//	// Output:
//	// what the program prints
//
// A step's code starts with its first line that is not a comment. The
// whole program is shown too, without the introduction, the explanations
// and the output.
func parseExample(lang, src string) (*example, error) {
	ex := &example{Lang: lang}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")

	// Header: title, tags and introduction, up to the package clause.
	i := 0
	var intro []string
	for ; i < len(lines) && !strings.HasPrefix(lines[i], "package "); i++ {
		c, ok := strings.CutPrefix(lines[i], "//")
		if !ok {
			return nil, fmt.Errorf("line %d: only comments may come before the package clause", i+1)
		}
		c = strings.TrimPrefix(c, " ")
		switch {
		case strings.HasPrefix(c, "Title: ") && ex.Title == "":
			ex.Title = strings.TrimSpace(strings.TrimPrefix(c, "Title: "))
		case strings.HasPrefix(c, "Tags: ") && ex.Tags == nil:
			// A comma inside a number, as in "5,5 %", does not split.
			for _, t := range strings.Split(strings.TrimPrefix(c, "Tags: "), ", ") {
				if t = strings.TrimSpace(t); t != "" {
					ex.Tags = append(ex.Tags, t)
				}
			}
		default:
			intro = append(intro, c)
		}
	}
	if ex.Title == "" {
		return nil, fmt.Errorf("no \"// Title:\" line")
	}
	ex.Intro = paragraphs(lang, intro)
	if len(ex.Intro) == 0 {
		return nil, fmt.Errorf("no introduction")
	}

	// The program without the header, the explanations and the output.
	var full []string
	for ; i < len(lines) && lines[i] != "func main() {"; i++ {
		full = append(full, lines[i])
	}
	if i == len(lines) {
		return nil, fmt.Errorf("no func main() {")
	}
	full = append(full, lines[i])
	i++

	// The steps, up to the end of main.
	var cur *step
	var text, code []string
	inText := false
	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = paragraphs(lang, text)
		for len(code) > 0 && strings.TrimSpace(code[0]) == "" {
			code = code[1:]
		}
		for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
			code = code[:len(code)-1]
		}
		for j, c := range code {
			code[j] = strings.TrimPrefix(c, "\t")
		}
		cur.Code = highlight(strings.Join(code, "\n"))
		ex.Steps = append(ex.Steps, *cur)
		cur, text, code = nil, nil, nil
	}
	for ; i < len(lines) && lines[i] != "}"; i++ {
		line := lines[i]
		if t, ok := strings.CutPrefix(line, "\t// ## "); ok {
			flush()
			cur = &step{Title: strings.TrimSpace(t)}
			inText = true
			full = append(full, "\t// "+cur.Title)
			continue
		}
		if inText {
			if c, ok := strings.CutPrefix(line, "\t//"); ok {
				text = append(text, strings.TrimPrefix(c, " "))
				continue
			}
			inText = false
		}
		if cur == nil {
			if strings.TrimSpace(line) != "" {
				return nil, fmt.Errorf("line %d: code before the first step (\"\t// ## title\")", i+1)
			}
			continue
		}
		code = append(code, line)
		full = append(full, line)
	}
	flush()
	if i == len(lines) {
		return nil, fmt.Errorf("main has no closing brace at the start of a line")
	}
	full = append(full, "}")
	if len(ex.Steps) < 3 {
		return nil, fmt.Errorf("%d steps, want at least 3", len(ex.Steps))
	}
	for _, s := range ex.Steps {
		if len(s.Text) == 0 {
			return nil, fmt.Errorf("step %q has no explanation", s.Title)
		}
	}

	// The output.
	for i++; i < len(lines) && lines[i] != "// Output:"; i++ {
	}
	var out []string
	for i++; i < len(lines); i++ {
		c, ok := strings.CutPrefix(lines[i], "//")
		if !ok {
			break
		}
		out = append(out, strings.TrimPrefix(c, " "))
	}
	ex.Output = strings.TrimSpace(strings.Join(out, "\n"))
	if ex.Output == "" {
		return nil, fmt.Errorf("no \"// Output:\" comment after main")
	}

	// Blank lines collapse where an explanation was removed.
	var compact []string
	for _, l := range full {
		if strings.TrimSpace(l) == "" && len(compact) > 0 && strings.TrimSpace(compact[len(compact)-1]) == "" {
			continue
		}
		compact = append(compact, l)
	}
	ex.Source = highlight(strings.TrimSpace(strings.Join(compact, "\n")))
	return ex, nil
}

// paragraphs joins comment lines into paragraphs, split at empty lines.
func paragraphs(lang string, lines []string) []template.HTML {
	var ps []template.HTML
	var cur []string
	end := func() {
		if len(cur) > 0 {
			ps = append(ps, markup(typography(lang, strings.Join(cur, " "))))
			cur = nil
		}
	}
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			end()
			continue
		}
		cur = append(cur, strings.TrimSpace(l))
	}
	end()
	return ps
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	"true": true, "false": true, "nil": true,
}

// highlight escapes Go source and marks its comments, strings and keywords
// with the classes cm, s and k.
func highlight(src string) template.HTML {
	var b strings.Builder
	span := func(class, s string) {
		b.WriteString(`<span class="` + class + `">` + html.EscapeString(s) + `</span>`)
	}
	isIdent := func(c byte) bool {
		return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case strings.HasPrefix(src[i:], "//"):
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src) - i
			}
			span("cm", src[i:i+j])
			i += j
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(src))
			span("s", src[i:j])
			i = j
		case c == '`':
			j := strings.IndexByte(src[i+1:], '`')
			if j < 0 {
				j = len(src) - i - 2
			}
			span("s", src[i:i+j+2])
			i += j + 2
		case isIdent(c) && (c < '0' || c > '9'):
			j := i
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			if w := src[i:j]; goKeywords[w] {
				span("k", w)
			} else {
				b.WriteString(w)
			}
			i = j
		default:
			j := i + 1
			for j < len(src) && !isIdent(src[j]) && src[j] != '"' && src[j] != '\'' && src[j] != '`' && src[j] != '/' {
				j++
			}
			b.WriteString(html.EscapeString(src[i:j]))
			i = j
		}
	}
	return template.HTML(b.String())
}
