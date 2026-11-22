package pt

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The mutation cases of cases_gen_test.go check, for each rule of the
// schematron that a change of one of the three base documents of testdata
// can make fail, that Check and CheckCEN report exactly the rules the reference
// reports for that variant: the schematron of eSPap compiled by SchXslt and
// run by Saxon-HE. They are rebuilt in two steps:
//
//	go test ./internal/rules/pt -run '^TestVariants$' -variants DIR
//	(run the schematron on DIR/*.xml, one SVRL report DIR/<name>.svrl each)
//	go test ./internal/rules/pt -run '^TestUpdateCases$' -variants DIR
//
// TestVariants writes every candidate mutation of the base documents, and
// their list (DIR/manifest.tsv); TestUpdateCases picks, for each rule that
// the reports raise, the variant that raises the fewest other rules, and
// writes cases_gen_test.go. A variant on which Saxon stops (an XPath error,
// such as the cast of "X y" to xs:decimal) is left out.
var variantsDir = flag.String("variants", "", "directory of the variants of the mutation cases (TestVariants, TestUpdateCases)")

const (
	baseInvoice    = "testdata/cius-pt.ubl.xml"
	baseCreditNote = "testdata/cius-pt-nc.ubl.xml"
	baseRich       = "testdata/cius-pt-rich.ubl.xml"
)

// bases are the base documents, with the names of their constants.
var bases = []struct{ file, name string }{
	{baseInvoice, "baseInvoice"},
	{baseCreditNote, "baseCreditNote"},
	{baseRich, "baseRich"},
}

// edit replaces occurrence nth (1-based) of old by new.
type edit struct {
	nth      int
	old, new string
}

func apply(t testing.TB, doc string, e edit) string {
	t.Helper()
	at, off := -1, 0
	for range e.nth {
		i := strings.Index(doc[off:], e.old)
		if i < 0 {
			t.Fatalf("edit: occurrence %d of %q not found", e.nth, e.old)
		}
		at, off = off+i, off+i+len(e.old)
	}
	return doc[:at] + e.new + doc[at+len(e.old):]
}

func load(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// findings returns the sorted identifiers of the rules of the schematron
// that Check and CheckCEN report for doc, without the checks of the NIF.
func findings(t testing.TB, doc string) []string {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	c := &rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r}
	run(c, root, true)
	run(c, root, false)
	var ids []string
	for _, f := range r.Findings {
		ids = append(ids, f.Rule)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// mutation is a candidate change of a base document.
type mutation struct {
	base string
	e    edit
}

var (
	reLeaf    = regexp.MustCompile(`<(cbc:[A-Za-z]+)((?: [A-Za-z]+="[^"]*")*)>([^<]*)</cbc:[A-Za-z]+>`)
	reAttr    = regexp.MustCompile(` ([A-Za-z]+)="([^"]*)"`)
	reNumber  = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	reOpen    = regexp.MustCompile(`(?m)^( *)<(cac:[A-Za-z]+)>\n`)
	reNotPath = regexp.MustCompile(`^not\(((?:(?:cac|cbc|ext):[A-Za-z0-9]+/?)+)\)$`)
	reNotAttr = regexp.MustCompile(`^not\(//@([A-Za-z]+)\)$`)
	reNote    = regexp.MustCompile(`^#([A-Z@]+)(-[0-9]{3})?#(.*)$`)
)

// codes are values tried in the elements and attributes of these names.
var codes = map[string][]string{
	"cbc:InvoiceTypeCode":           {"380", "381", "383", "386", "389", "FS", "FR", "ND", "NC", "RP", "RE", "CS", "LD", "RA", "999"},
	"cbc:CreditNoteTypeCode":        {"381", "380", "FT"},
	"cbc:ID@TaxCategory":            {"S", "NOR", "AA", "INT", "E", "ISE", "OUT", "Z", "AE", "K", "G", "O", "L", "M", "NA", "XX"},
	"cbc:DocumentCurrencyCode":      {"USD", "XXX", "EU"},
	"cbc:PaymentMeansCode":          {"1", "10", "31", "42", "48", "49", "57", "58", "59", "ZZZ"},
	"cbc:IdentificationCode":        {"ES", "FR", "XX"},
	"cbc:AllowanceChargeReasonCode": {"41", "100", "ADF", "XYZ"},
	"cbc:TaxExemptionReasonCode":    {"M01", "M99", "M42", "X1"},
	"cbc:DescriptionCode":           {"3", "432", "1"},
	"cbc:DocumentTypeCode":          {"50", "XX"},
	"cbc:ChargeIndicator":           {"true", "false", "1"},
	"listID":                        {"STI", "ZZZ"},
	"mimeCode":                      {"application/pdf", "text/html"},
	"currencyID":                    {"USD", "XXX"},
	"unitCode":                      {"C62", "XYZ"},
	"schemeID":                      {"0002", "0088", "EM", "9999", "ZZZ"},
}

// notes are values tried in the notes and in the names of the additional
// item properties, the structured notes of the CIUS-PT.
var notes = []string{
	"#SOURCECURRENCYCODE#USD#", "#CALCULATIONRATE#1.1#", "#ADDITIONALPROPERTY#a#b#",
	"#NUMBER@ATCERTIFIEDPROGRAM#", "#HASHCODE@ATCERTIFIEDPROGRAM##", "#DESCRIPTION@ATCERTIFIEDPROGRAM#x#",
	"#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-001#10.00#", "#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-001#IRS#",
	"#ENTITY@ATMPAYMENT#12345#", "#REFERENCE@ATMPAYMENT#123456789#",
	"#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#", "#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#",
	"#ID@INVOICEDOCUMENTREFERENCE@BILLINGREFERENCE-001#", "#DESCRIPTION@INVOICEPERIOD#x#",
}

// occurrence returns the rank, among the occurrences of s in doc, of the one
// at offset at.
func occurrence(doc, s string, at int) int {
	return strings.Count(doc[:at], s) + 1
}

// candidates returns the candidate mutations of a base document: for each
// basic element, its removal, its repetition, an empty or textual value,
// other numbers and codes, the removal or change of its attributes; for each
// aggregate, its removal and its repetition; structured notes, valid and
// malformed; and, for an invoice, the elements and attributes that the UBL
// syntax rules forbid.
func candidates(base, doc string) []mutation {
	var out []mutation
	add := func(at int, old, new string) {
		out = append(out, mutation{base, edit{occurrence(doc, old, at), old, new}})
	}
	for _, m := range reLeaf.FindAllStringSubmatchIndex(doc, -1) {
		whole := doc[m[0]:m[1]]
		name, attrs, value := doc[m[2]:m[3]], doc[m[4]:m[5]], doc[m[6]:m[7]]
		elem := func(attrs, value string) string { return "<" + name + attrs + ">" + value + "</" + name + ">" }
		add(m[0], whole, "")
		add(m[0], whole, whole+whole)
		add(m[0], whole, elem(attrs, ""))
		add(m[0], whole, elem(attrs, "X y"))
		if reNumber.MatchString(value) {
			for _, d := range []string{"0.01", "0.99", "1.00", "1.01", "-1.01", "-0.5", "0.004", "0.005"} {
				add(m[0], whole, elem(attrs, addDecimal(value, d)))
			}
			add(m[0], whole, elem(attrs, "-"+value))
			add(m[0], whole, elem(attrs, value+"1"))
		}
		key := name
		if name == "cbc:ID" && strings.HasSuffix(strings.TrimRight(doc[:m[0]], " \n"), "TaxCategory>") {
			key = "cbc:ID@TaxCategory"
		}
		for _, c := range codes[key] {
			if c != value {
				add(m[0], whole, elem(attrs, c))
			}
		}
		if name == "cbc:Note" || name == "cbc:Name" && strings.HasPrefix(value, "#") {
			for _, n := range notes {
				add(m[0], whole, elem(attrs, n))
			}
			if p := reNote.FindStringSubmatch(value); p != nil {
				prefix := "#" + p[1] + p[2] + "#"
				for _, v := range []string{
					prefix + "#", prefix + strings.Repeat("x", 151) + "#", prefix + "X y#",
					prefix + "a#" + strings.Repeat("x", 151) + "#",
					strings.TrimSuffix(value, "#"), value + "x#",
				} {
					add(m[0], whole, elem(attrs, v))
				}
				if p[2] != "" {
					add(m[0], whole, elem(attrs, "#"+p[1]+"-1#"+p[3]))
					add(m[0], whole, elem(attrs, "#"+p[1]+"-002#"+p[3]))
				}
			}
		}
		for _, a := range reAttr.FindAllStringSubmatch(attrs, -1) {
			add(m[0], whole, elem(strings.Replace(attrs, a[0], "", 1), value))
			add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="X Y"`, 1), value))
			for _, c := range codes[a[1]] {
				add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="`+c+`"`, 1), value))
			}
		}
	}
	for _, m := range reOpen.FindAllStringSubmatchIndex(doc, -1) {
		indent, name := doc[m[2]:m[3]], doc[m[4]:m[5]]
		end := strings.Index(doc[m[0]:], "\n"+indent+"</"+name+">\n")
		if end < 0 {
			continue
		}
		block := doc[m[0] : m[0]+end+len("\n"+indent+"</"+name+">\n")]
		add(m[0], block, "")
		add(m[0], block, block+block)
	}
	if strings.Contains(doc, "</Invoice>") {
		for _, r := range patterns[0].Rules {
			for _, a := range r.Asserts {
				switch m := reNotPath.FindStringSubmatch(a.Test); {
				case m != nil && (r.Context == "/ubl:Invoice" || r.Context == "cac:InvoiceLine"):
					steps := strings.Split(m[1], "/")
					x := "1"
					for i := len(steps) - 1; i >= 0; i-- {
						decl := ""
						if i == 0 && strings.Contains(m[1], "ext:") {
							decl = ` xmlns:ext="` + namespaces["ext"] + `"`
						}
						x = "<" + steps[i] + decl + ">" + x + "</" + steps[i] + ">"
					}
					anchor := "</Invoice>"
					if r.Context == "cac:InvoiceLine" {
						anchor = "</cac:InvoiceLine>"
					}
					add(strings.Index(doc, anchor), anchor, x+anchor)
				case reNotAttr.MatchString(a.Test):
					name := reNotAttr.FindStringSubmatch(a.Test)[1]
					add(strings.Index(doc, "<cbc:ID>"), "<cbc:ID>", `<cbc:ID `+name+`="x">`)
				}
			}
		}
		add(strings.Index(doc, "<cbc:ID>"), "<cbc:ID>", `<cbc:ID name="x">`)
	}
	return out
}

// addDecimal returns v + d with at least the decimals of both.
func addDecimal(v, d string) string {
	scale := max(decimals(v), decimals(d), 2)
	x, _ := strconv.ParseFloat(v, 64)
	y, _ := strconv.ParseFloat(d, 64)
	return strconv.FormatFloat(x+y, 'f', scale, 64)
}

func decimals(s string) int {
	if _, f, ok := strings.Cut(s, "."); ok {
		return len(f)
	}
	return 0
}

// TestVariants writes the candidate mutations of the base documents to
// -variants, with their list.
func TestVariants(t *testing.T) {
	var lines []string
	n := 0
	for _, b := range bases {
		doc := load(t, b.file)
		for _, m := range candidates(b.file, doc) {
			n++
			name := fmt.Sprintf("v%05d.xml", n)
			v := apply(t, doc, m.e)
			if _, err := xmltree.Parse([]byte(v)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if *variantsDir == "" {
				continue
			}
			if err := os.WriteFile(filepath.Join(*variantsDir, name), []byte(v), 0o644); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, strings.Join([]string{name, b.file, strconv.Itoa(m.e.nth), strconv.Quote(m.e.old), strconv.Quote(m.e.new)}, "\t"))
		}
	}
	if *variantsDir == "" {
		t.Logf("%d variants (-variants DIR writes them)", n)
		return
	}
	if err := os.WriteFile(filepath.Join(*variantsDir, "manifest.tsv"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// svrlIDs returns the sorted identifiers of the failed asserts and fired
// reports of an SVRL report, or ok false when the report is an <error>
// element (Saxon stopped).
func svrlIDs(data []byte) (ids []string, ok bool, err error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		switch se.Name.Local {
		case "error":
			return nil, false, nil
		case "failed-assert", "successful-report":
			for _, a := range se.Attr {
				if a.Name.Local == "id" {
					ids = append(ids, a.Value)
				}
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), true, nil
}

// TestUpdateCases writes cases_gen_test.go from the variants of -variants
// and their SVRL reports.
func TestUpdateCases(t *testing.T) {
	if *variantsDir == "" {
		// Nothing to write. Not a skip: the job of the official artefacts
		// fails on any skipped test.
		t.Log("-variants DIR not set: cases_gen_test.go left as is")
		return
	}
	f, err := os.Open(filepath.Join(*variantsDir, "manifest.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type variant struct {
		base string
		e    edit
		want []string
	}
	best := map[string]variant{}
	stopped, total := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		p := strings.Split(sc.Text(), "\t")
		nth, _ := strconv.Atoi(p[2])
		old, _ := strconv.Unquote(p[3])
		new, _ := strconv.Unquote(p[4])
		data, err := os.ReadFile(filepath.Join(*variantsDir, p[0]+".svrl"))
		if err != nil {
			t.Fatal(err)
		}
		total++
		want, ok, err := svrlIDs(data)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			stopped++
			continue
		}
		v := variant{p[1], edit{nth, old, new}, want}
		for _, id := range want {
			if b, seen := best[id]; !seen || len(want) < len(b.want) {
				best[id] = v
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	// One case per variant, in the order of the first rule it covers.
	var ids []string
	for id := range best {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	seen := map[string]bool{}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by TestUpdateCases (mutations_test.go); DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package pt\n\n")
	fmt.Fprintf(&b, "// cases are variants of the base documents, each with the rules that the\n// schematron of eSPap compiled by SchXslt and run by Saxon-HE reports for it:\n// for each of the %d rules raised by the %d candidate mutations (%d of which\n// stop Saxon), the variant that raises the fewest other rules.\n", len(ids), total, stopped)
	fmt.Fprintf(&b, "var cases = []struct {\n\tbase string\n\te    edit\n\twant []string\n}{\n")
	for _, id := range ids {
		v := best[id]
		key := v.base + "\x00" + strconv.Itoa(v.e.nth) + v.e.old + "\x00" + v.e.new
		if seen[key] {
			continue
		}
		seen[key] = true
		var base string
		for _, x := range bases {
			if x.file == v.base {
				base = x.name
			}
		}
		fmt.Fprintf(&b, "\t{%s, edit{%d, %q, %q}, []string{%s}},\n", base, v.e.nth, v.e.old, v.e.new, quoteAll(v.want))
	}
	b.WriteString("}\n")
	var never []string
	for i := range patterns {
		for _, r := range patterns[i].Rules {
			for _, a := range r.Asserts {
				if _, ok := best[a.ID]; !ok && !slices.Contains(never, a.ID) {
					never = append(never, a.ID)
				}
			}
		}
	}
	fmt.Fprintf(&b, "\n// untested lists the %d rules that no candidate mutation raises.\nvar untested = []string{%s}\n", len(never), quoteAll(never))
	src := b.Bytes()
	if err := os.WriteFile("cases_gen_test.go", src, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d rules raised, %d never; %d variants, %d stop Saxon", len(ids), len(never), total, stopped)
}

func quoteAll(s []string) string {
	q := make([]string, len(s))
	for i, x := range s {
		q[i] = strconv.Quote(x)
	}
	return strings.Join(q, ", ")
}

// TestCases checks the mutation cases against the rules the reference
// reports for them.
func TestCases(t *testing.T) {
	docs := map[string]string{}
	for _, b := range bases {
		docs[b.file] = load(t, b.file)
	}
	covered := map[string]bool{}
	for _, c := range cases {
		got := findings(t, apply(t, docs[c.base], c.e))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s, occurrence %d of %q → %q:\n got  %v\n want %v", c.base, c.e.nth, c.e.old, c.e.new, got, c.want)
		}
		for _, id := range c.want {
			covered[id] = true
		}
	}
	for _, id := range untested {
		if covered[id] {
			t.Errorf("%s is both tested and listed as untested", id)
		}
	}
	t.Logf("%d cases, %d rules tested, %d untested", len(cases), len(covered), len(untested))
}
