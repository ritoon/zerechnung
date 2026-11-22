package ro

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
// CIUS-RO schematron that a change of one of the two UBL base documents of
// testdata can make fail, that Check reports exactly the rules of the
// CIUS-RO that the reference reports for that variant: the schematron of
// ANAF (EN16931-CIUS_RO-UBL-validation.sch, 1.0.9) compiled by SchXslt and
// run by Saxon-HE, whose rules of EN 16931 are left out. They are rebuilt in
// two steps:
//
//	go test ./internal/rules/ro -run '^TestVariants$' -variants DIR
//	(run the schematron on DIR/*.xml, one SVRL report DIR/<name>.svrl each)
//	go test ./internal/rules/ro -run '^TestUpdateCases$' -variants DIR
//
// TestVariants writes every candidate mutation of the base documents, and
// their list (DIR/manifest.tsv); TestUpdateCases picks, for each rule of the
// CIUS-RO that the reports raise, the variant that raises the fewest other
// rules of the CIUS-RO, and writes cases_gen_test.go. The parity job
// (conformance/parity.sh) compares every variant with the schematron.
var variantsDir = flag.String("variants", "", "directory of the variants of the mutation cases (TestVariants, TestUpdateCases)")

const (
	baseInvoice    = "testdata/cius-ro.ubl.xml"
	baseCreditNote = "testdata/cius-ro-cn.ubl.xml"
)

// bases are the UBL base documents, with the names of their constants.
var bases = []struct{ file, name string }{
	{baseInvoice, "baseInvoice"},
	{baseCreditNote, "baseCreditNote"},
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

// findings returns the sorted identifiers of the rules of the CIUS-RO
// schematron that Check reports for the UBL document doc, without the
// checks of the CIF.
func findings(t testing.TB, doc string) []string {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := compiled()
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	s[0].Run(&rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r}, root, national)
	return ruleIDs(r)
}

func ruleIDs(r *rules.Report) []string {
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
	reLeaf   = regexp.MustCompile(`<(cbc:[A-Za-z]+)((?: [A-Za-z]+="[^"]*")*)>([^<]*)</cbc:[A-Za-z]+>`)
	reAttr   = regexp.MustCompile(` ([A-Za-z]+)="([^"]*)"`)
	reNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	reDate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reOpen   = regexp.MustCompile(`(?m)^( *)<(cac:[A-Za-z]+)>\n`)
)

// tryCodes are values tried in the elements and attributes of these names.
var tryCodes = map[string][]string{
	"cbc:CustomizationID":      {"urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.0", "urn:cen.eu:en16931:2017"},
	"cbc:InvoiceTypeCode":      {"380", "381", "384", "389", "751", "386", "326"},
	"cbc:CreditNoteTypeCode":   {"381", "380", "396"},
	"cbc:DocumentCurrencyCode": {"RON", "EUR", "USD"},
	"cbc:TaxCurrencyCode":      {"RON", "EUR"},
	"cbc:DescriptionCode":      {"3", "35", "432", "1", "5", "29"},
	"cbc:IdentificationCode":   {"RO", "FR", "ro"},
	"cbc:CountrySubentity":     {"RO-B", "RO-CJ", "RO-VS", "RO-XX", "Cluj", "ro-cj"},
	"cbc:CityName":             {"SECTOR1", "SECTOR6", "SECTOR7", "București", "sector1"},
	"cbc:ChargeIndicator":      {"true", "false", "1"},
	"cbc:ID@TaxCategory":       {"S", "O", "E", "Z", "AE", "K", "G", "L", "M"},
	"cbc:PaymentMeansCode":     {"30", "42", "48"},
	"currencyID":               {"RON", "EUR"},
}

// dates are values tried in the dates: the formats and the days that
// xs:date accepts or rejects.
var dates = []string{"2026-11-20Z", "2026-11-20+02:00", "20261120", "2026-11-20T10:00:00", " 2026-11-20", "2026-02-29", "2024-02-29", "2026-13-01", "0000-01-01", "2026-11-2X"}

// counts are the numbers of copies of an element tried beyond the maximum
// that a rule of the CIUS-RO sets.
var counts = map[string]int{
	"cbc:Note":                        21,
	"cac:AdditionalItemProperty":      51,
	"cac:AdditionalDocumentReference": 51,
	"cac:InvoiceDocumentReference":    2,
}

// creditNoteInserts are elements inserted into the credit note, before the
// element that ends each key: a tax representative and a delivery whose
// address breaks the rules of the CIUS-RO, document level allowances and
// charges with three decimals. The rules on them (BR-RO-140 to BR-RO-212,
// BR-DEC-RO-01 to BR-DEC-RO-06) do not apply to a credit note: their
// context starts with /ubl:CreditNote.
var creditNoteInserts = map[string]string{
	"  <cac:PaymentMeans>\n": "  <cac:TaxRepresentativeParty>\n    <cac:PartyName>\n      <cbc:Name>Reprezentant</cbc:Name>\n    </cac:PartyName>\n    <cac:PostalAddress>\n      <cbc:CityName>Sector 9</cbc:CityName>\n      <cbc:CountrySubentity>RO-B</cbc:CountrySubentity>\n      <cac:Country>\n        <cbc:IdentificationCode>RO</cbc:IdentificationCode>\n      </cac:Country>\n    </cac:PostalAddress>\n    <cac:PartyTaxScheme>\n      <cbc:CompanyID>RO32198764</cbc:CompanyID>\n      <cac:TaxScheme>\n        <cbc:ID>VAT</cbc:ID>\n      </cac:TaxScheme>\n    </cac:PartyTaxScheme>\n  </cac:TaxRepresentativeParty>\n  <cac:Delivery>\n    <cac:DeliveryLocation>\n      <cac:Address>\n        <cbc:CountrySubentity>RO-XX</cbc:CountrySubentity>\n        <cac:Country>\n          <cbc:IdentificationCode>RO</cbc:IdentificationCode>\n        </cac:Country>\n      </cac:Address>\n    </cac:DeliveryLocation>\n  </cac:Delivery>\n",
	"  <cac:TaxTotal>\n":     "  <cac:AllowanceCharge>\n    <cbc:ChargeIndicator>true</cbc:ChargeIndicator>\n    <cbc:AllowanceChargeReasonCode>" + strings.Repeat("x", 101) + "</cbc:AllowanceChargeReasonCode>\n    <cbc:Amount currencyID=\"RON\">0.005</cbc:Amount>\n    <cbc:BaseAmount currencyID=\"RON\">1.005</cbc:BaseAmount>\n    <cac:TaxCategory>\n      <cbc:ID>S</cbc:ID>\n      <cbc:Percent>19</cbc:Percent>\n      <cac:TaxScheme>\n        <cbc:ID>VAT</cbc:ID>\n      </cac:TaxScheme>\n    </cac:TaxCategory>\n  </cac:AllowanceCharge>\n",
}

// occurrence returns the rank, among the occurrences of s in doc, of the one
// at offset at.
func occurrence(doc, s string, at int) int {
	return strings.Count(doc[:at], s) + 1
}

// candidates returns the candidate mutations of a base document: for each
// basic element, its removal, its repetition, an empty, textual or long
// value, other numbers, decimals, dates and codes, the removal or change of
// its attributes; for each aggregate, its removal, its repetition and, for
// those whose number a rule limits, more copies than allowed; and, for the
// credit note, the elements of creditNoteInserts.
func candidates(base, doc string) []mutation {
	var out []mutation
	add := func(at int, old, new string) {
		out = append(out, mutation{base, edit{occurrence(doc, old, at), old, new}})
	}
	long := []string{strings.Repeat("x", 1001), strings.Repeat("ă", 40), " " + strings.Repeat("x", 19) + " "}
	for _, m := range reLeaf.FindAllStringSubmatchIndex(doc, -1) {
		whole := doc[m[0]:m[1]]
		name, attrs, value := doc[m[2]:m[3]], doc[m[4]:m[5]], doc[m[6]:m[7]]
		elem := func(attrs, value string) string { return "<" + name + attrs + ">" + value + "</" + name + ">" }
		add(m[0], whole, "")
		add(m[0], whole, whole+whole)
		add(m[0], whole, elem(attrs, ""))
		add(m[0], whole, elem(attrs, " "))
		add(m[0], whole, elem(attrs, "X y"))
		for _, v := range long {
			add(m[0], whole, elem(attrs, v))
		}
		if n := counts[name]; n > 0 {
			add(m[0], whole, strings.Repeat(whole, n))
		}
		if reNumber.MatchString(value) {
			for _, d := range []string{"0.01", "1.00", "-1.01", "0.001", "0.005"} {
				add(m[0], whole, elem(attrs, addDecimal(value, d)))
			}
			add(m[0], whole, elem(attrs, "-"+value))
			add(m[0], whole, elem(attrs, value+"0"))
			add(m[0], whole, elem(attrs, value+".5"))
		}
		if reDate.MatchString(value) {
			for _, d := range dates {
				add(m[0], whole, elem(attrs, d))
			}
		}
		key := name
		if name == "cbc:ID" && strings.HasSuffix(strings.TrimRight(doc[:m[0]], " \n"), "TaxCategory>") {
			key = "cbc:ID@TaxCategory"
		}
		for _, c := range tryCodes[key] {
			if c != value {
				add(m[0], whole, elem(attrs, c))
			}
		}
		for _, a := range reAttr.FindAllStringSubmatch(attrs, -1) {
			add(m[0], whole, elem(strings.Replace(attrs, a[0], "", 1), value))
			add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="X Y"`, 1), value))
			add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="`+strings.Repeat("x", 201)+`"`, 1), value))
			for _, c := range tryCodes[a[1]] {
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
		if n := counts[name]; n > 0 {
			add(m[0], block, strings.Repeat(block, n))
		}
	}
	if strings.Contains(doc, "</CreditNote>") {
		keys := make([]string, 0, len(creditNoteInserts))
		for k := range creditNoteInserts {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			add(strings.Index(doc, k), k, creditNoteInserts[k]+k)
		}
	}
	return out
}

// addDecimal returns v + d with at least the decimals of both.
func addDecimal(v, d string) string {
	scale := max(decimalsOf(v), decimalsOf(d), 2)
	x, _ := strconv.ParseFloat(v, 64)
	y, _ := strconv.ParseFloat(d, 64)
	return strconv.FormatFloat(x+y, 'f', scale, 64)
}

func decimalsOf(s string) int {
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
// reports of the rules of the CIUS-RO in an SVRL report, or ok false when
// the report is an <error> element (Saxon stopped).
func svrlIDs(data []byte) (ids []string, ok bool) {
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
			return nil, false
		case "failed-assert", "successful-report":
			for _, a := range se.Attr {
				if a.Name.Local == "id" && national(a.Value) {
					ids = append(ids, a.Value)
				}
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), true
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
	sc.Buffer(nil, 1<<22)
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
		want, ok := svrlIDs(data)
		if !ok {
			stopped++
			continue
		}
		v := variant{p[1], edit{nth, old, new}, want}
		for _, id := range want {
			// The fewest other rules, then the shortest edit.
			if b, seen := best[id]; !seen || len(want) < len(b.want) ||
				len(want) == len(b.want) && len(old)+len(new) < len(b.e.old)+len(b.e.new) {
				best[id] = v
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for id := range best {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	seen := map[string]bool{}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by TestUpdateCases (mutations_test.go); DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package ro\n\n")
	fmt.Fprintf(&b, "// cases are variants of the base documents, each with the rules of the\n// CIUS-RO that the schematron of ANAF compiled by SchXslt and run by Saxon-HE\n// reports for it: for each of the %d rules raised by the %d candidate\n// mutations (%d of which stop Saxon), the variant that raises the fewest\n// other rules.\n", len(ids), total, stopped)
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
				if _, ok := best[a.ID]; !ok && national(a.ID) && !slices.Contains(never, a.ID) {
					never = append(never, a.ID)
				}
			}
		}
	}
	fmt.Fprintf(&b, "\n// untested lists the %d rules that no candidate mutation raises.\nvar untested = []string{%s}\n", len(never), quoteAll(never))
	if err := os.WriteFile("cases_gen_test.go", b.Bytes(), 0o644); err != nil {
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
