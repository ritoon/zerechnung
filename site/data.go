package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"slices"
	"strconv"
	"strings"
)

// benchData holds the benchmark results shown on the benchmarks page, read
// from data/benchmarks.json. The figures come from the wiki pages Benchmarks
// and Benchmarks-autres-langages; the texts are in the translations.
type benchData struct {
	Date    string        `json:"date"`
	Version string        `json:"version"`
	Go      []*benchChart `json:"go"`   // ZeRechnung and the other Go libraries
	Poly    []*benchChart `json:"poly"` // the fastest library of each language
}

// benchChart is one operation on one input: a bar per library.
type benchChart struct {
	Op    string      `json:"op"`    // key of the name of the operation
	Input string      `json:"input"` // key of the description of the input
	Floor float64     `json:"floor"` // the encoding/xml floor in µs, if shown
	Rows  []*benchRow `json:"rows"`

	Ticks   []benchTick `json:"-"` // a line per power of ten
	FloorAt float64     `json:"-"` // position of the floor, in percent
	Flagged bool        `json:"-"` // whether the rows say how many valid invoices they flag
	Gain    float64     `json:"-"` // how many times ZeRechnung is faster than the next library
}

// benchRow is the result of one library.
type benchRow struct {
	Lib     string  `json:"lib"`
	Lang    string  `json:"lang"`    // programming language, on the page of all languages
	US      float64 `json:"us"`      // time per operation, in µs; 0 if it failed
	Fail    string  `json:"fail"`    // key of why it failed
	Note    string  `json:"note"`    // key of a remark on the result
	Flagged *int    `json:"flagged"` // valid invoices of the corpus it reports as invalid

	Ours   bool    `json:"-"`
	Best   bool    `json:"-"` // fastest of the chart
	Width  float64 `json:"-"` // length of the bar, in percent
	Factor float64 `json:"-"` // time divided by the fastest time
}

// benchTick is a grid line of a chart.
type benchTick struct {
	US float64
	At float64 // position, in percent
}

// loadBench reads data/benchmarks.json and lays out its charts: bar lengths
// are on a logarithmic scale, a grid line per power of ten.
func loadBench(fsys fs.FS, langs []*lang) (*benchData, error) {
	const name = "data/benchmarks.json"
	d := new(benchData)
	if err := decode(fsys, name, d); err != nil {
		return nil, err
	}
	src := sourceText(langs)
	for _, c := range slices.Concat(d.Go, d.Poly) {
		keys := []string{c.Op, c.Input}
		var timed []*benchRow
		for _, r := range c.Rows {
			keys = append(keys, r.Fail, r.Note)
			r.Ours = r.Lib == "ZeRechnung"
			if r.Flagged != nil {
				c.Flagged = true
			}
			switch {
			case r.US > 0 && r.Fail == "":
				timed = append(timed, r)
			case r.US == 0 && r.Fail != "":
			default:
				return nil, fmt.Errorf("%s: %s, %s: give either a time or a reason for the failure", name, c.Op, r.Lib)
			}
		}
		if !slices.IsSortedFunc(timed, func(a, b *benchRow) int { return cmpFloat(a.US, b.US) }) {
			return nil, fmt.Errorf("%s: %s %s: sort the rows from the fastest", name, c.Op, c.Input)
		}
		if len(timed) < 2 || !slices.ContainsFunc(timed, func(r *benchRow) bool { return r.Ours }) {
			return nil, fmt.Errorf("%s: %s %s: at least ZeRechnung and one other library", name, c.Op, c.Input)
		}
		for _, k := range keys {
			if _, ok := src[k]; k != "" && !ok {
				return nil, fmt.Errorf("%s: %s: no text %q in %s.json", name, c.Op, k, sourceLang)
			}
		}

		// The scale runs from the power of ten below the fastest time to the
		// power of ten above the slowest.
		best, worst := timed[0].US, timed[len(timed)-1].US
		lo := int(math.Ceil(math.Log10(best))) - 1
		hi := int(math.Ceil(math.Log10(worst)))
		pos := func(us float64) float64 {
			return math.Round(1000*(math.Log10(us)-float64(lo))/float64(hi-lo)) / 10
		}
		for e := lo; e <= hi; e++ {
			us := math.Pow(10, float64(e))
			c.Ticks = append(c.Ticks, benchTick{US: us, At: pos(us)})
		}
		for _, r := range timed {
			r.Width = pos(r.US)
			r.Factor = r.US / best
		}
		timed[0].Best = true
		if c.Floor > 0 {
			c.FloorAt = pos(c.Floor)
		}
		if timed[0].Ours {
			c.Gain = timed[1].US / best
		}
	}
	return d, nil
}

// compData is the compliance page, read from data/compliance.json: for each
// country, the e-invoicing obligations and what ZeRechnung checks. The
// facts come from the wiki pages Normes-européennes-par-pays and
// Ce-que-valide-ZeRechnung; the texts are in the translations.
type compData struct {
	AsOf      string         `json:"asof"` // date of the facts, YYYY-MM-DD
	Countries []*compCountry `json:"countries"`
	Details   []*compDetail  `json:"details"`
}

// compCountry is one row of the overview. The name of the country is the
// text country_<code>.
type compCountry struct {
	Code  string      `json:"code"`
	EU    bool        `json:"eu"`
	B2G   string      `json:"b2g"` // "since 2014", "from 2027", "none"… (see Status)
	B2B   string      `json:"b2b"`
	Cover [][2]string `json:"cover"` // format, and yes, partial or no
}

// compDetail lists the checkpoints of one country.
type compDetail struct {
	Code   string      `json:"code"`
	Guide  string      `json:"guide"` // page of the wiki with a guide for the country
	Checks []compCheck `json:"checks"`
}

// compCheck is one checkpoint: what a country requires, whether ZeRechnung
// checks it, and with which rule sets.
type compCheck struct {
	Text   string   `json:"text"`   // key of the text
	Status string   `json:"status"` // yes, partial or no
	Rules  []string `json:"rules"`  // rule sets or functions of ZeRechnung
}

var statusKinds = map[string]bool{
	"since": true, "from": true, "planned": true, "reception": true,
	"request": true, "voluntary": false, "none": false,
}

func loadComp(fsys fs.FS, langs []*lang) (*compData, error) {
	const name = "data/compliance.json"
	d := new(compData)
	if err := decode(fsys, name, d); err != nil {
		return nil, err
	}
	src := sourceText(langs)
	need := func(key string) error {
		if _, ok := src[key]; !ok {
			return fmt.Errorf("%s: no text %q in %s.json", name, key, sourceLang)
		}
		return nil
	}
	seen := make(map[string]bool)
	for _, c := range d.Countries {
		if seen[c.Code] {
			return nil, fmt.Errorf("%s: %s twice", name, c.Code)
		}
		seen[c.Code] = true
		if err := need("country_" + c.Code); err != nil {
			return nil, err
		}
		for _, s := range []string{c.B2G, c.B2B} {
			kind, year, _ := strings.Cut(s, " ")
			withYear, ok := statusKinds[kind]
			if !ok || withYear != (year != "") {
				return nil, fmt.Errorf("%s: %s: status %q", name, c.Code, s)
			}
		}
		for _, cv := range c.Cover {
			if !validStatus(cv[1]) {
				return nil, fmt.Errorf("%s: %s: %s: status %q", name, c.Code, cv[0], cv[1])
			}
		}
	}
	for _, dt := range d.Details {
		if !seen[dt.Code] {
			return nil, fmt.Errorf("%s: details of %s, which the overview does not list", name, dt.Code)
		}
		if err := need("cp_" + strings.ToLower(dt.Code) + "_intro"); err != nil {
			return nil, err
		}
		for _, ck := range dt.Checks {
			if err := need(ck.Text); err != nil {
				return nil, err
			}
			if !validStatus(ck.Status) {
				return nil, fmt.Errorf("%s: %s: status %q", name, ck.Text, ck.Status)
			}
		}
	}
	return d, nil
}

func validStatus(s string) bool { return s == "yes" || s == "partial" || s == "no" }

// Countries returns the countries of the overview, in or out of the EU,
// sorted by their name in the language of the page.
func (p *page) Countries(eu bool) []*compCountry {
	var cs []*compCountry
	for _, c := range p.Comp.Countries {
		if c.EU == eu {
			cs = append(cs, c)
		}
	}
	slices.SortFunc(cs, func(a, b *compCountry) int {
		return strings.Compare(fold(p.P["country_"+a.Code]), fold(p.P["country_"+b.Code]))
	})
	return cs
}

// Country returns the name of a country in the language of the page.
func (p *page) Country(code string) template.HTML { return p.T["country_"+code] }

// Status writes an obligation of the overview: "since 2014", "from 2027".
func (p *page) Status(s string) (template.HTML, error) {
	kind, year, _ := strings.Cut(s, " ")
	if kind == "none" {
		return `<span class="none" aria-label="` + template.HTML(template.HTMLEscapeString(p.P["cp_none"])) + `">—</span>`, nil
	}
	if year == "" {
		return p.Fmt("cp_" + kind)
	}
	return p.Fmt("cp_"+kind, year)
}

// Dur writes a duration given in µs, as the wiki does: 3,0 µs, 124 µs,
// 1,14 ms.
func (p *page) Dur(us float64) string {
	return p.num(us, false)
}

// Tick writes the duration of a grid line: 10 µs, 1 ms.
func (p *page) Tick(us float64) string {
	return p.num(us, true)
}

func (p *page) num(us float64, tick bool) string {
	v, unit, prec := us, "µs", 1
	switch {
	case us < 1:
		v, unit, prec = us*1000, "ns", 0
	case us >= 1e6:
		v, unit, prec = us/1e6, "s", 2
	case us >= 1000:
		v, unit, prec = us/1000, "ms", 2
	case us >= 100:
		prec = 0
	}
	if tick {
		prec = 0
	}
	return p.decimal(strconv.FormatFloat(v, 'f', prec, 64)) + " " + unit
}

// Times writes a ratio: ×2,5, ×15.
func (p *page) Times(f float64) string {
	prec := 1
	if f >= 10 {
		prec = 0
	}
	return "×" + p.decimal(strconv.FormatFloat(f, 'f', prec, 64))
}

// decimal uses the decimal separator of the language of the page.
func (p *page) decimal(s string) string {
	if p.Lang.Code == "en" {
		return s
	}
	return strings.Replace(s, ".", ",", 1)
}

// Date writes a YYYY-MM-DD date in the language of the page.
func (p *page) Date(iso string) (string, error) {
	y, m, ok := strings.Cut(iso, "-")
	m, d, ok2 := strings.Cut(m, "-")
	mi, err := strconv.Atoi(m)
	if !ok || !ok2 || err != nil || mi < 1 || mi > 12 {
		return "", fmt.Errorf("date %q: want YYYY-MM-DD", iso)
	}
	months := strings.Split(p.P["months"], ",")
	if len(months) != 12 {
		return "", fmt.Errorf("i18n/%s.json: months must list the 12 months, separated by commas", p.Lang.Code)
	}
	d = strings.TrimPrefix(d, "0")
	month := strings.TrimSpace(months[mi-1])
	switch p.Lang.Code {
	case "en":
		return d + " " + month + " " + y, nil
	case "de":
		return d + ". " + month + " " + y, nil
	case "es", "pt":
		return d + " de " + month + " de " + y, nil
	default:
		return d + " " + month + " " + y, nil
	}
}

// decode reads a JSON file, refusing unknown fields.
func decode(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func sourceText(langs []*lang) map[string]string {
	for _, l := range langs {
		if l.Code == sourceLang {
			return l.Text
		}
	}
	return nil
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// fold lowers s and removes the accents of Latin letters, so that names
// sort as a reader expects: Österreich among the O.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := accents[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}

var accents = func() map[rune]rune {
	m := make(map[rune]rune)
	for base, list := range map[rune]string{
		'a': "àáâãäåā", 'c': "çčć", 'e': "èéêëēě", 'i': "ìíîïī", 'n': "ñň",
		'o': "òóôõöøō", 'u': "ùúûüūů", 'y': "ýÿ", 's': "šś", 'z': "žźż", 'r': "ř",
	} {
		for _, r := range list {
			m[r] = base
		}
	}
	return m
}()
