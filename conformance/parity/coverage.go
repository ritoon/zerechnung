package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// kositXSL is a schematron XSLT of the KoSIT configuration and its
// assertions.
type kositXSL struct {
	Location string   // in the configuration: resources/cii/16b/xsl/EN16931-CII-validation.xsl
	Syntax   string   // cii or ubl
	IDs      []string // identifiers of its assertions, each once, in the order of the XSLT
	Flags    map[string]string
}

// kositStylesheets reads the scenarios of the KoSIT configuration in dir and
// returns the schematron XSLT they run (validateWithSchematron), each once,
// with their assertions.
func kositStylesheets(dir string) ([]kositXSL, error) {
	data, err := os.ReadFile(filepath.Join(dir, "scenarios.xml"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Scenarios []struct {
			Schematron []struct {
				Location string `xml:"resource>location"`
			} `xml:"validateWithSchematron"`
		} `xml:"scenario"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("scenarios.xml: %w", err)
	}
	seen := map[string]bool{}
	var out []kositXSL
	for _, s := range doc.Scenarios {
		for _, r := range s.Schematron {
			loc := strings.TrimSpace(r.Location)
			if loc == "" || seen[loc] {
				continue
			}
			seen[loc] = true
			x := kositXSL{Location: loc, Flags: map[string]string{}}
			switch base := strings.ToUpper(path.Base(loc)); {
			case strings.Contains(base, "CII"):
				x.Syntax = "cii"
			case strings.Contains(base, "UBL"):
				x.Syntax = "ubl"
			default:
				return nil, fmt.Errorf("%s: neither CII nor UBL", loc)
			}
			if err := x.index(filepath.Join(dir, filepath.FromSlash(loc))); err != nil {
				return nil, err
			}
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Location < out[j].Location })
	return out, nil
}

// index reads the assertions of an XSLT compiled by SchXslt, as in the KoSIT
// configuration: each assertion (or report) is an svrl:failed-assert
// (svrl:successful-report) that carries its identifier and its flag as
// attributes.
func (x *kositXSL) index(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	const nsSVRL = "http://purl.oclc.org/dsdl/svrl"
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		t, ok := tok.(xml.StartElement)
		if !ok || t.Name.Space != nsSVRL || (t.Name.Local != "failed-assert" && t.Name.Local != "successful-report") {
			continue
		}
		id := attr(t, "id")
		if id == "" {
			return fmt.Errorf("%s: an assertion without identifier", file)
		}
		if _, ok := x.Flags[id]; !ok {
			x.IDs = append(x.IDs, id)
			x.Flags[id] = attr(t, "flag")
		}
	}
	if len(x.IDs) == 0 {
		return fmt.Errorf("%s: no assertion", file)
	}
	return nil
}

// families group the assertions of the KoSIT XSLT in the report, in this
// order: the first pattern that matches an identifier gives its family.
var families = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"BR", regexp.MustCompile(`^BR-\d`)},
	{"BR-CO", regexp.MustCompile(`^BR-CO-`)},
	{"BR-CL", regexp.MustCompile(`^BR-CL-`)},
	{"BR-DEC", regexp.MustCompile(`^BR-DEC-`)},
	{"TVA", regexp.MustCompile(`^BR-(AE|AF|AG|B|E|G|IC|IG|IP|O|S|Z)-`)},
	{"CII-SR", regexp.MustCompile(`^CII-SR-`)},
	{"CII-DT", regexp.MustCompile(`^CII-DT-`)},
	{"UBL-CR", regexp.MustCompile(`^UBL-CR-`)},
	{"UBL-SR", regexp.MustCompile(`^UBL-SR-`)},
	{"UBL-DT", regexp.MustCompile(`^UBL-DT-`)},
	{"BR-DE", regexp.MustCompile(`^BR-(DE|DEX|TMP)-`)},
	{"PEPPOL", regexp.MustCompile(`^PEPPOL-`)},
}

func family(id string) string {
	for _, f := range families {
		if f.pattern.MatchString(id) {
			return f.name
		}
	}
	return "autres"
}

// kositCoverage is the share of the assertions of a KoSIT XSLT that the
// validator raised, at any level, on at least one invoice.
type kositCoverage struct {
	Stylesheet string         `json:"stylesheet"`
	Syntax     string         `json:"syntax"`
	Assertions int            `json:"assertions"`
	Corpus     int            `json:"corpus"`   // raised on an invoice of the corpus or of the KoSIT test suite
	Variants   int            `json:"variants"` // raised on a variant
	Raised     int            `json:"raised"`   // raised on either
	Families   []familyCount  `json:"families"`
	Never      []string       `json:"never,omitempty"` // never raised
	byFamily   map[string]int // assertions by family
}

type familyCount struct {
	Family     string `json:"family"`
	Assertions int    `json:"assertions"`
	Raised     int    `json:"raised"`
}

// coverageOf counts, for each XSLT, the assertions that KoSIT raised on the
// compared invoices. syntax gives the syntax of each invoice by origin.
func coverageOf(xsls []kositXSL, all []comparison, syntax map[string]string) []kositCoverage {
	corpus, variants := map[string]bool{}, map[string]bool{} // syntax/id
	for _, c := range all {
		if c.Reference != "kosit" || !c.Ref.Covered {
			continue
		}
		set := corpus
		if strings.HasPrefix(c.Invoice, "variants/") {
			set = variants
		}
		for id := range c.Ref.Rules {
			set[syntax[c.Invoice]+"/"+id] = true
		}
	}
	var out []kositCoverage
	for _, x := range xsls {
		cov := kositCoverage{Stylesheet: path.Base(x.Location), Syntax: x.Syntax, Assertions: len(x.IDs), byFamily: map[string]int{}}
		raised := map[string]int{}
		for _, id := range x.IDs {
			k := x.Syntax + "/" + id
			f := family(id)
			cov.byFamily[f]++
			if corpus[k] {
				cov.Corpus++
			}
			if variants[k] {
				cov.Variants++
			}
			if corpus[k] || variants[k] {
				cov.Raised++
				raised[f]++
			} else {
				cov.Never = append(cov.Never, id)
			}
		}
		for _, f := range families {
			if n := cov.byFamily[f.name]; n > 0 {
				cov.Families = append(cov.Families, familyCount{f.name, n, raised[f.name]})
			}
		}
		if n := cov.byFamily["autres"]; n > 0 {
			cov.Families = append(cov.Families, familyCount{"autres", n, raised["autres"]})
		}
		out = append(out, cov)
	}
	return out
}

// docSyntax returns the syntax of an invoice: cii, ubl, or "" for another
// document.
func docSyntax(data []byte) string {
	switch npaStylesheet(data) {
	case xslCII:
		return "cii"
	case "":
		return ""
	}
	return "ubl"
}
