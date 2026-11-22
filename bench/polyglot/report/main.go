// Command report merges the Go results of this repository's package
// (bench_facturx.txt, survey_facturx.txt, produced by bench/ecosystem/run.sh)
// with the results of the other languages (bench_<lang>.jsonl,
// survey_<lang>.jsonl: TypeScript, Python, Rust, Java, C#, C, C++, PHP) into
// Markdown comparison tables. When the resources pass ran
// (resources_<lang>.jsonl, Go included), it adds the CPU time and peak memory
// tables.
//
// Usage: go run . ../results/2026-11-17 > ../results/2026-11-17/tables.md
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// result is one library on one benchmark.
type result struct {
	lib, lang, version string
	status             string  // ok, unsupported, fail
	ns                 float64 // median ns/op
	first              float64 // first call in the process, 0 if unknown
	note               string
}

type key struct{ lib, lang string }

// resource is one library on one benchmark in the resources pass: a process
// that ran that benchmark only.
type resource struct {
	status string
	ns     float64 // median wall ns/op of the pass
	cpu    float64 // median CPU ns/op over the same series
	rss    float64 // peak resident set size of the process, bytes
	child  bool    // the library runs child processes, not counted
}

// goOneCore is the second Go round of the resources pass, GOMAXPROCS=1.
const goOneCore = "ZeRechnung, GOMAXPROCS=1"

type group struct {
	title   string
	benches []string
	labels  []string
	survey  string // survey column shown after the name ("validate" or "")
}

var groups = []group{
	{"Extraction", []string{"Extract_Small", "Extract_Large", "Extract_Minimum", "Extract_Extended"},
		[]string{"EN 16931, 149 Ko", "EN 16931, 1,1 Mo", "MINIMUM FNFE, 89 Ko", "EXTENDED ZF 2.0, 157 Ko"}, ""},
	{"Lecture XML → modèle", []string{"Parse_Small", "Parse_Lines", "Parse_Large"},
		[]string{"13 Ko", "19 Ko", "1,3 Mo"}, ""},
	{"Lecture et validation", []string{"ParseValidate_Small", "ParseValidate_Lines", "ParseValidate_Large"},
		[]string{"13 Ko", "19 Ko", "1,3 Mo"}, "validate"},
	{"Écriture modèle → XML", []string{"Write_Small", "Write_Lines"},
		[]string{"13 Ko", "19 Ko"}, ""},
	{"Injection dans un PDF", []string{"EmbedClassic_Small", "Embed_Small"},
		[]string{"hôte à xref classique", "hôte à flux xref"}, ""},
	{"Contrôle d'un PDF hybride", []string{"CheckPDF_Small", "CheckPDF_Large"},
		[]string{"149 Ko", "1,1 Mo"}, ""},
}

var (
	goBench  = regexp.MustCompile(`^Benchmark(\S+?)-\d+\s+\d+\s+([\d.]+) ns/op`)
	goSurvey = regexp.MustCompile(`SURVEY (validate|extract|parse): (\d+)/(\d+)`)
	goVer    = regexp.MustCompile(`^go version (go\S+)`)
)

var langNames = map[string]string{
	"go": "Go", "typescript": "TypeScript", "python": "Python", "rust": "Rust",
	"java": "Java", "csharp": "C#", "c": "C", "cpp": "C++", "php": "PHP",
}

// langs is the order of the languages in every table.
var langs = []string{"go", "typescript", "python", "rust", "java", "csharp", "c", "cpp", "php"}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: report <results dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	res := map[string]map[key]result{} // bench -> lib -> result
	surveys := map[key]map[string]string{}
	notes := map[key]map[string][]string{} // lib -> note -> benches

	if err := loadGo(dir, res, surveys); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "bench_*.jsonl"))
	for _, f := range files {
		if err := loadJSONBench(f, res, notes); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	files, _ = filepath.Glob(filepath.Join(dir, "survey_*.jsonl"))
	for _, f := range files {
		if err := loadJSONSurvey(f, surveys); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	fmt.Printf("<!-- Généré par bench/polyglot/report à partir de %s -->\n", filepath.Base(dir))
	printSurveys(surveys)
	for _, g := range groups {
		printGroup(g, res, surveys)
	}
	rs := map[string]map[key]resource{}
	files, _ = filepath.Glob(filepath.Join(dir, "resources_*.jsonl"))
	for _, f := range files {
		if err := loadResources(f, rs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if len(rs) > 0 {
		printResourcesByLang(rs)
		for _, g := range groups {
			printResGroup(g, rs, "cpu")
		}
		for _, g := range groups {
			printResGroup(g, rs, "mem")
		}
	}
	printNotes(notes)
}

func loadGo(dir string, res map[string]map[key]result, surveys map[key]map[string]string) error {
	k := key{"ZeRechnung (ce dépôt)", "go"}
	raw := map[string][]float64{}
	version := ""
	err := eachLine(filepath.Join(dir, "bench_facturx.txt"), func(line string) {
		if m := goVer.FindStringSubmatch(line); m != nil {
			version = m[1]
		}
		if m := goBench.FindStringSubmatch(line); m != nil {
			ns, _ := strconv.ParseFloat(m[2], 64)
			raw[m[1]] = append(raw[m[1]], ns)
		}
	})
	if err != nil {
		return err
	}
	for b, v := range raw {
		put(res, b, k, result{lib: k.lib, lang: "go", version: version, status: "ok", ns: median(v)})
	}
	eachLine(filepath.Join(dir, "survey_facturx.txt"), func(line string) {
		if m := goSurvey.FindStringSubmatch(line); m != nil {
			if surveys[k] == nil {
				surveys[k] = map[string]string{}
			}
			surveys[k][m[1]] = m[2] + "/" + m[3]
		}
	})
	return nil
}

func loadJSONBench(path string, res map[string]map[key]result, notes map[key]map[string][]string) error {
	return eachLine(path, func(line string) {
		var r struct {
			Lang, Lib, Version, Bench, Status, Note string
			NsPerOp                                 []float64 `json:"ns_per_op"`
			FirstCallNs                             float64   `json:"first_call_ns"`
		}
		if strings.TrimSpace(line) == "" || json.Unmarshal([]byte(line), &r) != nil {
			return
		}
		k := key{r.Lib, r.Lang}
		out := result{lib: r.Lib, lang: r.Lang, version: r.Version, status: r.Status, first: r.FirstCallNs, note: r.Note}
		if r.Status == "ok" && len(r.NsPerOp) > 0 {
			out.ns = median(r.NsPerOp)
		}
		put(res, r.Bench, k, out)
		// Notes of unsupported operations only say why; keep the others.
		if r.Note != "" && r.Status != "unsupported" {
			if notes[k] == nil {
				notes[k] = map[string][]string{}
			}
			notes[k][r.Note] = append(notes[k][r.Note], r.Bench)
		}
	})
}

func loadJSONSurvey(path string, surveys map[key]map[string]string) error {
	return eachLine(path, func(line string) {
		var r struct {
			Lang, Lib, Op string
			Ok, Total     int
		}
		if strings.TrimSpace(line) == "" || json.Unmarshal([]byte(line), &r) != nil {
			return
		}
		k := key{r.Lib, r.Lang}
		if surveys[k] == nil {
			surveys[k] = map[string]string{}
		}
		surveys[k][r.Op] = fmt.Sprintf("%d/%d", r.Ok, r.Total)
	})
}

func loadResources(path string, rs map[string]map[key]resource) error {
	return eachLine(path, func(line string) {
		var r struct {
			Lang, Lib, Bench, Status, Note string
			NsPerOp                        []float64 `json:"ns_per_op"`
			CPUNsPerOp                     []float64 `json:"cpu_ns_per_op"`
			MaxRSSBytes                    float64   `json:"max_rss_bytes"`
		}
		if strings.TrimSpace(line) == "" || json.Unmarshal([]byte(line), &r) != nil {
			return
		}
		out := resource{status: r.Status, rss: r.MaxRSSBytes, child: strings.Contains(r.Note, "processus enfants")}
		if r.Status == "ok" && len(r.NsPerOp) > 0 && len(r.CPUNsPerOp) > 0 {
			out.ns, out.cpu = median(r.NsPerOp), median(r.CPUNsPerOp)
		} else if r.Status == "ok" {
			out.status = "fail"
		}
		k := key{r.Lib, r.Lang}
		if rs[r.Bench] == nil {
			rs[r.Bench] = map[key]resource{}
		}
		rs[r.Bench][k] = out
	})
}

func put(res map[string]map[key]result, bench string, k key, r result) {
	if res[bench] == nil {
		res[bench] = map[key]result{}
	}
	res[bench][k] = r
}

func printSurveys(s map[key]map[string]string) {
	fmt.Print("\n#### Exactitude sur le corpus\n\n")
	fmt.Println("Référence : les schematrons officiels (CEN EN 16931 et KoSIT) signalent 6 des 26 XML « valides » du corpus (BR-CL-25 deux fois, CII-SR-465 quatre fois).")
	fmt.Println()
	fmt.Println("| Librairie | Langage | XML valides signalés | XML lus | PDF extraits |")
	fmt.Println("|---|---|---|---|---|")
	ks := make([]key, 0, len(s))
	for k := range s {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return less(ks[i], ks[j]) })
	for _, k := range ks {
		fmt.Printf("| %s | %s | %s | %s | %s |\n", name(k), langNames[k.lang], orDash(s[k]["validate"]), orDash(s[k]["parse"]), orDash(s[k]["extract"]))
	}
}

func printGroup(g group, res map[string]map[key]result, surveys map[key]map[string]string) {
	libs := map[key]bool{}
	best := map[string]float64{}
	goKey := key{"ZeRechnung (ce dépôt)", "go"}
	for _, b := range g.benches {
		for k, r := range res[b] {
			if r.status == "unsupported" {
				continue
			}
			libs[k] = true
			if r.status == "ok" {
				if v, ok := best[b]; !ok || r.ns < v {
					best[b] = r.ns
				}
			}
		}
	}
	if len(libs) == 0 {
		return
	}
	first := g.benches[0]
	okNs := func(k key) (float64, bool) {
		r, ok := res[first][k]
		return r.ns, ok && r.status == "ok"
	}
	order := make([]key, 0, len(libs))
	for k := range libs {
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		a, aok := okNs(order[i])
		b, bok := okNs(order[j])
		switch {
		case aok && bok:
			return a < b
		case aok != bok:
			return aok
		default:
			return less(order[i], order[j])
		}
	})

	fmt.Printf("\n#### %s\n\n", g.title)
	head := []string{"Librairie", "Langage"}
	if g.survey != "" {
		head = append(head, "Valides signalés /26")
	}
	head = append(head, g.labels...)
	head = append(head, "Premier appel")
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(head)))
	for _, k := range order {
		row := []string{name(k), langNames[k.lang]}
		if g.survey != "" {
			row = append(row, orDash(surveys[k][g.survey]))
		}
		for _, b := range g.benches {
			r, ok := res[b][k]
			switch {
			case !ok || r.status == "unsupported":
				row = append(row, "—")
			case r.status != "ok":
				row = append(row, "échec")
			default:
				cell := fmtNs(r.ns)
				if r.ns <= best[b]*1.0001 {
					cell = "**" + cell + "**"
				}
				if base, ok := res[b][goKey]; ok && base.status == "ok" && k != goKey {
					cell += fmt.Sprintf(" (×%s)", ratio(r.ns/base.ns))
				}
				row = append(row, cell)
			}
		}
		if r, ok := res[first][k]; ok && r.status == "ok" && r.first > 0 {
			row = append(row, fmtNs(r.first))
		} else {
			row = append(row, "—")
		}
		fmt.Println("| " + strings.Join(row, " | ") + " |")
	}
	fmt.Printf("\nEn gras : le plus rapide de la colonne. (×n) : temps rapporté à celui du package Go de ce dépôt. Tri sur la colonne « %s » ; « Premier appel » : durée du premier appel dans le processus, même colonne (non mesurée pour Go).\n", g.labels[0])
}

// printResourcesByLang summarises the resources pass per language: spread of
// the peak memory and of the CPU / wall ratio over all the processes.
func printResourcesByLang(rs map[string]map[key]resource) {
	rss := map[string][]float64{}
	use := map[string][]float64{}
	libs := map[string]map[string]bool{}
	for _, m := range rs {
		for k, r := range m {
			if r.status != "ok" || r.rss <= 0 || r.child || k.lib == goOneCore {
				continue
			}
			rss[k.lang] = append(rss[k.lang], r.rss)
			if r.ns > 0 {
				use[k.lang] = append(use[k.lang], r.cpu/r.ns)
			}
			if libs[k.lang] == nil {
				libs[k.lang] = map[string]bool{}
			}
			libs[k.lang][k.lib] = true
		}
	}
	fmt.Print("\n#### Ressources par langage\n\n")
	fmt.Println("| Langage | Librairies | Processus | Pic mémoire min. | médian | max. | CPU / temps réel médian | max. |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	for _, l := range langs {
		v := rss[l]
		if len(v) == 0 {
			continue
		}
		u := use[l]
		fmt.Printf("| %s | %d | %d | %s | %s | %s | %s | %s |\n", langNames[l], len(libs[l]), len(v),
			fmtMiB(slices.Min(v)), fmtMiB(median(v)), fmtMiB(slices.Max(v)), pct(median(u)), pct(slices.Max(u)))
	}
	fmt.Println("\nUn processus par librairie et par benchmark (passe `resources`), sans la série Go à `GOMAXPROCS=1` ni les librairies qui lancent des processus enfants : chargement de la librairie, vérification, premier appel, calibrage et série chronométrée. Pic mémoire : mémoire résidente maximale (max RSS) du processus, runtime compris ; 1 Mio = 1 048 576 octets. CPU / temps réel : temps CPU du processus (utilisateur + système, tous les fils) rapporté au temps réel sur la série chronométrée ; 100 % = un cœur occupé, au-delà = du travail en parallèle (ramasse-miettes, JIT).")
}

// printResGroup prints the CPU ("cpu") or peak memory ("mem") table of a group.
func printResGroup(g group, rs map[string]map[key]resource, kind string) {
	val := func(r resource) float64 {
		if kind == "cpu" {
			return r.cpu
		}
		return r.rss
	}
	libs := map[key]bool{}
	best := map[string]float64{}
	goKey := key{"ZeRechnung (ce dépôt)", "go"}
	for _, b := range g.benches {
		for k, r := range rs[b] {
			if r.status == "unsupported" {
				continue
			}
			libs[k] = true
			if r.status == "ok" && val(r) > 0 && !r.child {
				if v, ok := best[b]; !ok || val(r) < v {
					best[b] = val(r)
				}
			}
		}
	}
	if len(libs) == 0 {
		return
	}
	first := g.benches[0]
	okVal := func(k key) (float64, bool) {
		r, ok := rs[first][k]
		return val(r), ok && r.status == "ok" && val(r) > 0
	}
	order := make([]key, 0, len(libs))
	for k := range libs {
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		a, aok := okVal(order[i])
		b, bok := okVal(order[j])
		switch {
		case aok && bok:
			return a < b
		case aok != bok:
			return aok
		default:
			return less(order[i], order[j])
		}
	})

	title := "Temps CPU"
	if kind == "mem" {
		title = "Pic mémoire"
	}
	fmt.Printf("\n#### %s : %s\n\n", title, g.title)
	head := append([]string{"Librairie", "Langage"}, g.labels...)
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(head)))
	for _, k := range order {
		row := []string{name(k), langNames[k.lang]}
		for _, b := range g.benches {
			r, ok := rs[b][k]
			switch {
			case !ok || r.status == "unsupported":
				row = append(row, "—")
			case r.status != "ok" || val(r) <= 0:
				row = append(row, "échec")
			case kind == "cpu":
				cell := fmtNs(r.cpu)
				if r.cpu <= best[b]*1.0001 {
					cell = "**" + cell + "**"
				}
				row = append(row, cell+" ("+pct(r.cpu/r.ns)+")"+dagger(r))
			default:
				cell := fmtMiB(r.rss)
				if r.rss <= best[b]*1.0001 {
					cell = "**" + cell + "**"
				}
				if base, ok := rs[b][goKey]; ok && base.status == "ok" && base.rss > 0 && k != goKey {
					cell += fmt.Sprintf(" (×%s)", ratio(r.rss/base.rss))
				}
				row = append(row, cell+dagger(r))
			}
		}
		fmt.Println("| " + strings.Join(row, " | ") + " |")
	}
	fmt.Printf("\n%s : le package Go limité à un cœur ; son ramasse-miettes partage alors le cœur du fil mesuré. † : la librairie lance des processus enfants à chaque appel, dont le CPU et la mémoire ne sont pas comptés.\n", goOneCore)
	if kind == "cpu" {
		fmt.Printf("Temps CPU par opération (utilisateur + système, tous les fils du processus) sur la série chronométrée ; entre parenthèses, rapporté au temps réel de la même série (100 %% = un cœur). En gras : le plus faible de la colonne. Tri sur la colonne « %s ».\n", g.labels[0])
	} else {
		fmt.Printf("Pic de mémoire résidente du processus qui n'a exécuté que ce benchmark, runtime compris. En gras : le plus faible de la colonne. (×n) : rapporté au package Go de ce dépôt. Tri sur la colonne « %s ».\n", g.labels[0])
	}
}

func printNotes(notes map[key]map[string][]string) {
	if len(notes) == 0 {
		return
	}
	fmt.Print("\n#### Notes des harnais\n\nAppels mesurés et particularités, pour les opérations réussies ou en échec (les raisons des opérations non prises en charge sont dans les fichiers `COVERAGE.md`).\n\n")
	ks := make([]key, 0, len(notes))
	for k := range notes {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return less(ks[i], ks[j]) })
	for _, k := range ks {
		texts := make([]string, 0, len(notes[k]))
		for t := range notes[k] {
			texts = append(texts, t)
		}
		sort.Strings(texts)
		parts := make([]string, len(texts))
		for i, t := range texts {
			parts[i] = fmt.Sprintf("%s [%s]", t, strings.Join(notes[k][t], ", "))
		}
		fmt.Printf("- **%s** (%s) : %s\n", name(k), langNames[k.lang], strings.Join(parts, " ; "))
	}
}

func less(a, b key) bool {
	if a.lang != b.lang {
		return langOrder(a.lang) < langOrder(b.lang)
	}
	return strings.ToLower(a.lib) < strings.ToLower(b.lib)
}

func langOrder(l string) int {
	if i := slices.Index(langs, l); i >= 0 {
		return i
	}
	return len(langs)
}

func name(k key) string {
	if k.lang == "go" {
		return "**" + k.lib + "**"
	}
	return k.lib
}

func eachLine(path string, fn func(string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		fn(sc.Text())
	}
	return sc.Err()
}

func median(v []float64) float64 {
	v = slices.Clone(v)
	slices.Sort(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}

func fmtNs(ns float64) string {
	switch {
	case ns >= 1e9:
		return comma(ns/1e9, 2) + " s"
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

func fmtMiB(b float64) string {
	v := b / (1 << 20)
	switch {
	case v >= 100:
		return comma(v, 0) + " Mio"
	case v >= 1:
		return comma(v, 1) + " Mio"
	default:
		return comma(v, 2) + " Mio"
	}
}

func dagger(r resource) string {
	if r.child {
		return " †"
	}
	return ""
}

// pct formats a CPU / wall ratio as a percentage of one core.
func pct(r float64) string {
	return comma(r*100, 0) + " %"
}

func ratio(r float64) string {
	if r >= 10 {
		return comma(r, 0)
	}
	return comma(r, 1)
}

func comma(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	s = strings.Replace(s, ".", ",", 1)
	// Group thousands with a narrow space for large integers parts.
	intPart, frac, hasFrac := strings.Cut(s, ",")
	if len(intPart) > 3 {
		var b strings.Builder
		for i, c := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteString(" ")
			}
			b.WriteRune(c)
		}
		intPart = b.String()
	}
	if hasFrac {
		return intPart + "," + frac
	}
	return intPart
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
