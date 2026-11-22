// Command report turns the raw outputs of run.sh (bench_*.txt and
// survey_*.txt) into the Markdown comparison tables used on the wiki.
//
// Usage: go run . ../results/2026-11-16 > ../results/2026-11-16/tables.md
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// run is the median of the -count executions of one benchmark.
type run struct{ ns, bytes, allocs float64 }

var (
	benchLine  = regexp.MustCompile(`^Benchmark(\S+?)-\d+\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ MB/s)?\s+(\d+) B/op\s+(\d+) allocs/op`)
	surveyLine = regexp.MustCompile(`SURVEY (validate|extract|parse): (\d+)/(\d+)`)
)

// Display names, matching the abbreviations used on the wiki.
var names = map[string]string{
	"speedata": "speedata", "andee": "andee", "dotwave": "dotwave", "goblde": "gobl.de",
	"angelo": "angelo", "papierleicht": "papierleicht", "dcnws": "DCNWS", "pdf0": "pdf0",
	"litho": "litho", "kanjo": "kanjo", "taha": "taha", "fx": "fX", "gopdfattach": "gopdfattach",
	"pdfcpu": "pdfcpu", "floor": "plancher", "facturx": "**ZeRechnung (ce dépôt)**",
}

type group struct {
	title   string
	benches []string
	labels  []string
	missing string // shown when a library has no result for a column
	extra   string // survey column inserted after the name ("validate", "")
}

var groups = []group{
	{"Extraction", []string{"Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended"},
		[]string{"EN 16931, 149 Ko", "EN 16931, 1,1 Mo", "MINIMUM FNFE, 89 Ko", "EXTENDED ZF 2.0, 157 Ko"}, "échec", ""},
	{"Parsing", []string{"Parse_Small", "Parse_Lines", "Parse_Large"},
		[]string{"13 Ko", "19 Ko", "1,3 Mo"}, "—", ""},
	{"Validation", []string{"ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large"},
		[]string{"13 Ko", "19 Ko", "1,3 Mo"}, "—", "validate"},
	{"Écriture", []string{"Write_Small", "Write_Lines"},
		[]string{"13 Ko", "19 Ko"}, "—", ""},
	{"Injection", []string{"EmbedClassic_Small", "Embed_Small"},
		[]string{"hôte à xref classique", "hôte à flux xref"}, "échec", ""},
	{"Contrôle", []string{"CheckPDF_Small", "CheckPDF_Large"},
		[]string{"149 Ko", "1,1 Mo"}, "—", ""},
	{"Planchers", []string{"Floor_XMLRawToken_Small", "Floor_XMLRawToken_Lines", "Floor_XMLRawToken_Large", "Floor_Memcpy_PDFSmall"},
		[]string{"RawToken 13 Ko", "RawToken 19 Ko", "RawToken 1,3 Mo", "memcpy 149 Ko"}, "—", ""},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: report <results dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	results, err := loadBenches(dir) // bench -> lib -> run
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	surveys := loadSurveys(dir) // lib -> kind -> "n/total"

	fmt.Printf("<!-- Généré par bench/ecosystem/report à partir de %s -->\n", filepath.Base(dir))
	printSurveys(surveys)
	for _, g := range groups {
		printGroup(g, results, surveys)
	}
	printAllocs(results)
}

// allocColumns are the columns of the allocation table: one call of each
// operation, on the inputs of the site's charts.
var allocColumns = []struct{ bench, label string }{
	{"Extract_Small", "Extraction, PDF de 149 Ko"},
	{"Parse_Small", "Lecture, XML de 13 Ko"},
	{"ParseValidate_Small", "Validation, XML de 13 Ko"},
	{"ParseValidate_Large", "Validation, XML de 1,3 Mo"},
	{"Write_Small", "Écriture, XML de 13 Ko"},
	{"Embed_Small", "Injection, hôte à flux xref"},
	{"CheckPDF_Small", "Contrôle, PDF de 149 Ko"},
}

// printAllocs prints, for each library, the bytes and the allocations of one
// call (go test -benchmem), ZeRechnung first, with the encoding/xml floor as a
// reference under the reading column.
func printAllocs(res map[string]map[string]run) {
	libs := map[string]bool{}
	best := map[string]float64{}
	for _, c := range allocColumns {
		for l, r := range res[c.bench] {
			libs[l] = true
			if v, ok := best[c.bench]; !ok || r.bytes < v {
				best[c.bench] = r.bytes
			}
		}
	}
	if len(libs) == 0 {
		return
	}
	order := keys(libs)
	sort.Slice(order, func(i, j int) bool {
		if (order[i] == "facturx") != (order[j] == "facturx") {
			return order[i] == "facturx"
		}
		return strings.ToLower(display(order[i])) < strings.ToLower(display(order[j]))
	})
	cell := func(r run) string { return fmtBytes(r.bytes) + " · " + thousands(r.allocs) }

	fmt.Print("\n#### Allocations par appel\n\n")
	head := []string{"Librairie"}
	for _, c := range allocColumns {
		head = append(head, c.label)
	}
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(head)))
	for _, l := range order {
		row := []string{display(l)}
		for _, c := range allocColumns {
			r, ok := res[c.bench][l]
			switch {
			case !ok:
				row = append(row, "—")
			case r.bytes <= best[c.bench]:
				row = append(row, "**"+cell(r)+"**")
			default:
				row = append(row, cell(r))
			}
		}
		fmt.Println("| " + strings.Join(row, " | ") + " |")
	}
	if r, ok := res["Floor_XMLRawToken_Small"]["floor"]; ok {
		row := []string{"*plancher : jetons `encoding/xml`*"}
		for _, c := range allocColumns {
			if c.bench == "Parse_Small" {
				row = append(row, "*"+cell(r)+"*")
			} else {
				row = append(row, "")
			}
		}
		fmt.Println("| " + strings.Join(row, " | ") + " |")
	}
	fmt.Println("\nOctets alloués · nombre d'allocations, pour un appel (`go test -benchmem`, médiane des exécutions). « — » : opération non proposée, ou en échec. En gras : le moins d'octets alloués de la colonne.")
}

func loadBenches(dir string) (map[string]map[string]run, error) {
	files, err := filepath.Glob(filepath.Join(dir, "bench_*.txt"))
	if err != nil {
		return nil, err
	}
	raw := map[string]map[string][]run{}
	for _, f := range files {
		lib := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "bench_"), ".txt")
		err := eachLine(f, func(line string) {
			m := benchLine.FindStringSubmatch(line)
			if m == nil {
				return
			}
			ns, _ := strconv.ParseFloat(m[2], 64)
			by, _ := strconv.ParseFloat(m[3], 64)
			al, _ := strconv.ParseFloat(m[4], 64)
			if raw[m[1]] == nil {
				raw[m[1]] = map[string][]run{}
			}
			raw[m[1]][lib] = append(raw[m[1]][lib], run{ns, by, al})
		})
		if err != nil {
			return nil, err
		}
	}
	out := map[string]map[string]run{}
	for b, libs := range raw {
		out[b] = map[string]run{}
		for l, rs := range libs {
			out[b][l] = run{
				median(rs, func(r run) float64 { return r.ns }),
				median(rs, func(r run) float64 { return r.bytes }),
				median(rs, func(r run) float64 { return r.allocs }),
			}
		}
	}
	return out, nil
}

func loadSurveys(dir string) map[string]map[string]string {
	out := map[string]map[string]string{}
	files, _ := filepath.Glob(filepath.Join(dir, "survey_*.txt"))
	for _, f := range files {
		lib := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "survey_"), ".txt")
		eachLine(f, func(line string) {
			if m := surveyLine.FindStringSubmatch(line); m != nil {
				if out[lib] == nil {
					out[lib] = map[string]string{}
				}
				out[lib][m[1]] = m[2] + "/" + m[3]
			}
		})
	}
	return out
}

func printSurveys(s map[string]map[string]string) {
	fmt.Print("\n#### Exactitude sur le corpus\n\n")
	fmt.Println("| Librairie | XML valides rejetés | XML parsés | PDF extraits |")
	fmt.Println("|---|---|---|---|")
	libs := keys(s)
	sort.Slice(libs, func(i, j int) bool { return display(libs[i]) < display(libs[j]) })
	for _, l := range libs {
		fmt.Printf("| %s | %s | %s | %s |\n", display(l), orDash(s[l]["validate"]), orDash(s[l]["parse"]), orDash(s[l]["extract"]))
	}
}

func printGroup(g group, res map[string]map[string]run, surveys map[string]map[string]string) {
	libs := map[string]bool{}
	best := map[string]float64{}
	for _, b := range g.benches {
		for l, r := range res[b] {
			libs[l] = true
			if v, ok := best[b]; !ok || r.ns < v {
				best[b] = r.ns
			}
		}
	}
	if len(libs) == 0 {
		return
	}
	first := g.benches[0]
	order := keys(libs)
	sort.Slice(order, func(i, j int) bool {
		a, aok := res[first][order[i]]
		b, bok := res[first][order[j]]
		switch {
		case aok && bok:
			return a.ns < b.ns
		case aok != bok:
			return aok
		default:
			return display(order[i]) < display(order[j])
		}
	})

	fmt.Printf("\n#### %s\n\n", g.title)
	head := []string{"Librairie"}
	if g.extra != "" {
		head = append(head, "Valides rejetés /26")
	}
	head = append(head, g.labels...)
	head = append(head, "Mémoire/op", "Allocations/op")
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(head)))
	for _, l := range order {
		row := []string{display(l)}
		if g.extra != "" {
			row = append(row, orDash(surveys[l][g.extra]))
		}
		for _, b := range g.benches {
			r, ok := res[b][l]
			switch {
			case !ok:
				row = append(row, g.missing)
			case r.ns <= best[b]*1.0001:
				row = append(row, "**"+fmtNs(r.ns)+"**")
			default:
				row = append(row, fmt.Sprintf("%s (×%s)", fmtNs(r.ns), comma(r.ns/best[b], 1)))
			}
		}
		if r, ok := res[first][l]; ok {
			row = append(row, fmtBytes(r.bytes), thousands(r.allocs))
		} else {
			row = append(row, "—", "—")
		}
		fmt.Println("| " + strings.Join(row, " | ") + " |")
	}
	fmt.Printf("\nMémoire et allocations : colonne « %s ». Tri sur cette même colonne.\n", g.labels[0])
}

func eachLine(path string, fn func(string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		fn(sc.Text())
	}
	return sc.Err()
}

func median(rs []run, f func(run) float64) float64 {
	v := make([]float64, len(rs))
	for i, r := range rs {
		v[i] = f(r)
	}
	slices.Sort(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}

func fmtNs(ns float64) string {
	switch {
	case ns >= 1e6:
		return comma(ns/1e6, 2) + " ms"
	case ns >= 1e5:
		return comma(ns/1e3, 0) + " µs"
	case ns >= 1e3:
		return comma(ns/1e3, 1) + " µs"
	default:
		return comma(ns, 0) + " ns"
	}
}

func fmtBytes(b float64) string {
	switch {
	case b >= 1000<<20:
		return comma(b/(1<<30), 1) + " Go"
	case b >= 1000<<10:
		return comma(b/(1<<20), 1) + " Mo"
	case b >= 1<<10:
		return comma(b/(1<<10), 0) + " Ko"
	default:
		return comma(b, 0) + " o"
	}
}

// comma formats with a French decimal comma.
func comma(v float64, prec int) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', prec, 64), ".", ",", 1)
}

// thousands groups digits with a space, French style.
func thousands(v float64) string {
	s := strconv.FormatInt(int64(math.Round(v)), 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func display(lib string) string {
	if n, ok := names[lib]; ok {
		return n
	}
	return lib
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
