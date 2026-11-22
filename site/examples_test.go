package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// minExamples is the number of examples each language must have.
const minExamples = 3

func TestEveryLanguageHasExamples(t *testing.T) {
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := loadExamples(examplesFS, langs)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range langs {
		if n := len(examples[l.Code]); n < minExamples {
			t.Errorf("%s: %d examples, want at least %d", l.Code, n, minExamples)
		}
	}
}

// TestExamplesRun runs every example program in a directory of its own,
// with a copy of the files of its directory, and compares what it prints
// with the output shown on the page.
func TestExamplesRun(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs every example")
	}
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := loadExamples(examplesFS, langs)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", bin+string(filepath.Separator), "./examples/...")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./examples/...: %v\n%s", err, out)
	}
	for _, exs := range examples {
		for _, ex := range exs {
			t.Run(ex.Name, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				files, err := os.ReadDir(filepath.Join("examples", ex.Name))
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range files {
					if f.IsDir() || strings.HasSuffix(f.Name(), ".go") {
						continue
					}
					data, err := os.ReadFile(filepath.Join("examples", ex.Name, f.Name()))
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, f.Name()), data, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command(filepath.Join(bin, ex.Name))
				cmd.Dir = dir
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("%v\n%s%s", err, stdout.String(), stderr.String())
				}
				if got := normalize(stdout.String()); got != normalize(ex.Output) {
					t.Errorf("the program prints:\n%s\nthe page shows:\n%s", got, ex.Output)
				}
			})
		}
	}
}

// normalize trims the spaces at the end of each line and of the text.
func normalize(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

const sample = `// Title: A title
// Tags: Factur-X, VAT 5,5 %
//
// First paragraph,
// on two lines.
//
// Second paragraph.
package main

import "fmt"

func main() {
	// ## One
	// Explains one.
	x := 1 // a comment in the code

	// ## Two
	// Explains two,
	//
	// in two paragraphs.

	// A comment before the code of two.
	x++

	// ## Three
	// Explains three.
	fmt.Println(x, "done")
}

// Output:
// 2 done
`

func TestParseExample(t *testing.T) {
	ex, err := parseExample("en", sample)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Title != "A title" {
		t.Errorf("title %q", ex.Title)
	}
	if got := strings.Join(ex.Tags, "|"); got != "Factur-X|VAT 5,5 %" {
		t.Errorf("tags %q", got)
	}
	if len(ex.Intro) != 2 || ex.Intro[0] != "First paragraph, on two lines." {
		t.Errorf("intro %q", ex.Intro)
	}
	if len(ex.Steps) != 3 {
		t.Fatalf("%d steps, want 3", len(ex.Steps))
	}
	two := ex.Steps[1]
	if two.Title != "Two" || len(two.Text) != 2 {
		t.Errorf("step two: %q, %q", two.Title, two.Text)
	}
	if !strings.HasPrefix(string(two.Code), `<span class="cm">// A comment before the code of two.</span>`) {
		t.Errorf("step two code: %s", two.Code)
	}
	if ex.Output != "2 done" {
		t.Errorf("output %q", ex.Output)
	}
	src := string(ex.Source)
	for _, gone := range []string{"Title:", "Explains", "Output:", "## "} {
		if strings.Contains(src, gone) {
			t.Errorf("the whole program contains %q:\n%s", gone, src)
		}
	}
	if !strings.Contains(src, `<span class="cm">// Two</span>`) {
		t.Errorf("the whole program lacks the title of step two:\n%s", src)
	}
}

func TestParseExampleErrors(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"no title", strings.Replace(sample, "// Title: A title\n", "", 1), "Title"},
		{"two steps", strings.Replace(sample, "\t// ## Three\n\t// Explains three.\n", "", 1), "at least 3"},
		{"no output", sample[:strings.Index(sample, "// Output:")], "Output"},
		{"no explanation", strings.Replace(sample, "\t// Explains one.\n", "", 1), "explanation"},
	}
	for _, tt := range tests {
		if _, err := parseExample("en", tt.src); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v, want an error about %q", tt.name, err, tt.want)
		}
	}
}

func TestHighlight(t *testing.T) {
	got := string(highlight("if x := \"a<b\"; x != nil { // done\n\treturn `raw`\n}"))
	want := `<span class="k">if</span> x := <span class="s">&#34;a&lt;b&#34;</span>; x != <span class="k">nil</span> { <span class="cm">// done</span>` +
		"\n\t" + `<span class="k">return</span> <span class="s">` + "`raw`" + `</span>` + "\n}"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
