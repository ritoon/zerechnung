package hr

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
// CIUS-HR schematron that a change of one of the three UBL base documents of
// testdata can make fail, that the package reports exactly the rules that
// the reference reports for that variant: the schematron of the tax
// administration (HR-CIUS-EXT-EN16931-UBL.sch, 1.0.0) compiled by SchXslt
// and run by Saxon-HE. They are rebuilt in two steps:
//
//	go test ./internal/rules/hr -run '^TestVariants$' -variants DIR
//	(run the schematron on DIR/*.xml, one SVRL report DIR/<name>.svrl each)
//	go test ./internal/rules/hr -run '^TestUpdateCases$' -variants DIR
//
// TestVariants writes every candidate mutation of the base documents, and
// their list (DIR/manifest.tsv); TestUpdateCases picks, for each rule that
// the reports raise, the variant that raises the fewest other rules, and
// writes cases_gen_test.go. The parity job (conformance/parity.sh) compares
// every variant with the schematron.
var variantsDir = flag.String("variants", "", "directory of the variants of the mutation cases (TestVariants, TestUpdateCases)")

const (
	baseInvoice    = "testdata/cius-hr.ubl.xml"
	baseCreditNote = "testdata/cius-hr-cn.ubl.xml"
	baseOutOfScope = "testdata/cius-hr-o.ubl.xml"
)

// bases are the UBL base documents, with the names of their constants: an
// invoice with the standard, reduced and exempt rates and an item out of
// the scope of VAT, a credit note with VAT on a cash basis, and the invoice
// of a seller outside the VAT system.
var bases = []struct{ file, name string }{
	{baseInvoice, "baseInvoice"},
	{baseCreditNote, "baseCreditNote"},
	{baseOutOfScope, "baseOutOfScope"},
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

// findings returns the sorted identifiers of the rules of the CIUS-HR
// schematron that the package reports for the UBL document doc, without the
// check of the electronic addresses.
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
	s.Run(&rules.Checker{Doc: &rules.Doc{UBL: root, Root: root}, Report: r}, root, all)
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
	reLeaf   = regexp.MustCompile(`<(cbc:[A-Za-z]+|hrextac:[A-Za-z]+)((?: [A-Za-z]+="[^"]*")*)>([^<]*)</(?:cbc|hrextac):[A-Za-z]+>`)
	reAttr   = regexp.MustCompile(` ([A-Za-z]+)="([^"]*)"`)
	reNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	reDate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reOpen   = regexp.MustCompile(`(?m)^( *)<((?:cac|hrextac):[A-Za-z]+)>\n`)
	reTag    = regexp.MustCompile(`<(/?)([A-Za-z]+:[A-Za-z0-9]+)[^>]*?(/?)>`)
)

var (
	taxCategories = []string{"S", "Z", "E", "AE", "O", "K", "G", "L", "M"}
	labels        = []string{"HR:PDV25", "HR:PDV13", "HR:PDV5", "HR:Z", "HR:K", "HR:G", "HR:AE", "HR:E", "HR:POVNAK", "HR:PP", "HR:PPMV", "HR:O", "HR:N", "HR:XX", "PDV25", "hr:pdv25", "HR:PDV25 HR:E"}
	taxSchemes    = []string{"VAT", "OTH", "FRE", "TAX"}
)

// tryCodes are values tried in the elements and attributes of these names;
// a key name@parent applies to the elements of that name in an aggregate of
// that local name only.
var tryCodes = map[string][]string{
	"cbc:CustomizationID":            {"urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0", "urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.1"},
	"cbc:ProfileID":                  {"P1", "P9", "P10", "P12", "P13", "P0", "P01", "p1", "P99", "P99:Kupac", "P99:" + strings.Repeat("x", 197)},
	"cbc:InvoiceTypeCode":            {"380", "381", "384", "386", "389", "751", "326", "393", "875"},
	"cbc:CreditNoteTypeCode":         {"381", "380", "396", "383"},
	"cbc:DocumentCurrencyCode":       {"EUR", "USD"},
	"cbc:IdentificationCode":         {"HR", "SI", "hr"},
	"cbc:ChargeIndicator":            {"true", "false", "1", "0"},
	"cbc:ID@TaxCategory":             taxCategories,
	"cbc:ID@ClassifiedTaxCategory":   taxCategories,
	"cbc:ID@HRTaxCategory":           taxCategories,
	"cbc:Name@TaxCategory":           labels,
	"cbc:Name@ClassifiedTaxCategory": labels,
	"cbc:Name@HRTaxCategory":         labels,
	"cbc:Percent":                    {"25", "13", "5", "0", "10", "25.12345678901", strings.Repeat("1", 31)},
	"cbc:ID@TaxScheme":               taxSchemes,
	"cbc:ID@HRTaxScheme":             taxSchemes,
	"cbc:ID@SellerContact":           {"82031549070", "HR82031549070", "82031549071", "8203154907", "HR 82031549070", "hr82031549070"},
	"cbc:CompanyID@PartyTaxScheme":   {"HR69152712380", "69152712380", "HR69152712381", "HR6915271238", "SI12345678", "hr69152712380"},
	"cbc:EndpointID":                 {"69152712380", "69152712381", "HR69152712380", "6915271238"},
	"cbc:ItemClassificationCode":     {"58.29.11", "62.20.20", "01.11.11", "99.99.99", "58.29", "58.29.11 62.20.20"},
	"cbc:IssueTime":                  {"09:30:15", "9:30:15", "09:30", "09:30:15Z", "09:30:15.5", "25:00:00", "09:30:15+01:00"},
	"cbc:CopyIndicator":              {"true", "false", "1"},
	"cbc:TaxExemptionReasonCode":     {"VATEX-EU-132", "VATEX-EU-O", "HR:X"},
	"cbc:PaymentMeansCode":           {"30", "42", "10"},
	"hrextac:HRObracunPDVPoNaplati":  {"Obračun po naplaćenoj naknadi"},
	"currencyID":                     {"EUR", "USD"},
	"unitCode":                       {"H87", "C62", "HUR"},
	"schemeID":                       {"9934", "0088", "9925"},
	"listID":                         {"CG", "STI", "HS", "cg"},
}

// dates are values tried in the dates: the bounds of the rules, and the
// formats and days that xs:date accepts or rejects.
var dates = []string{"2025-12-31", "2026-01-01", "2099-12-31", "2100-01-01", "1899-12-31", "1900-01-01", "2026-11-20Z", "2026-11-20+02:00", "20261120", "2026-11-20T10:00:00", "2026-02-29", "2026-13-01", "2026-11-2X"}

// inserts are edits of each base document beyond the candidates of each
// element: references at document level beside those of the lines, a tax
// representative in Croatia with an invalid OIB, and an exemption reason
// given by its code.
var inserts = map[string][]edit{
	baseInvoice: {
		{1, "  <cac:BillingReference>\n", "  <cac:OrderReference>\n    <cbc:ID>NAR-2026-3</cbc:ID>\n  </cac:OrderReference>\n  <cac:BillingReference>\n"},
		{1, "  <cac:ContractDocumentReference>\n", "  <cac:DespatchDocumentReference>\n    <cbc:ID>OTP-2026-3</cbc:ID>\n  </cac:DespatchDocumentReference>\n  <cac:ContractDocumentReference>\n"},
		{1, "  <cac:ContractDocumentReference>\n", "  <cac:ReceiptDocumentReference>\n    <cbc:ID>PRI-2026-3</cbc:ID>\n  </cac:ReceiptDocumentReference>\n  <cac:ContractDocumentReference>\n"},
		{1, "  <cac:Delivery>\n", "  <cac:TaxRepresentativeParty>\n    <cac:PartyName>\n      <cbc:Name>Porezni zastupnik d.o.o.</cbc:Name>\n    </cac:PartyName>\n    <cac:PostalAddress>\n      <cbc:CityName>Osijek</cbc:CityName>\n      <cac:Country>\n        <cbc:IdentificationCode>HR</cbc:IdentificationCode>\n      </cac:Country>\n    </cac:PostalAddress>\n    <cac:PartyTaxScheme>\n      <cbc:CompanyID>HR12345678901</cbc:CompanyID>\n      <cac:TaxScheme>\n        <cbc:ID>VAT</cbc:ID>\n      </cac:TaxScheme>\n    </cac:PartyTaxScheme>\n  </cac:TaxRepresentativeParty>\n  <cac:Delivery>\n"},
	},
	baseOutOfScope: {
		{1, "<cbc:TaxExemptionReason>Nije u sustavu PDV-a</cbc:TaxExemptionReason>", "<cbc:TaxExemptionReasonCode>VATEX-EU-O</cbc:TaxExemptionReasonCode>"},
		{4, "<cbc:TaxExemptionReason>Nije u sustavu PDV-a</cbc:TaxExemptionReason>", "<cbc:TaxExemptionReasonCode>VATEX-EU-O</cbc:TaxExemptionReasonCode>"},
	},
}

// occurrence returns the rank, among the occurrences of s in doc, of the one
// at offset at.
func occurrence(doc, s string, at int) int {
	return strings.Count(doc[:at], s) + 1
}

// parent returns the local name of the element that contains offset at.
func parent(doc string, at int) string {
	var stack []string
	for _, m := range reTag.FindAllStringSubmatch(doc[:at], -1) {
		switch {
		case m[3] == "/":
		case m[1] == "/":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, m[2])
		}
	}
	if len(stack) == 0 {
		return ""
	}
	_, local, _ := strings.Cut(stack[len(stack)-1], ":")
	return local
}

// candidates returns the candidate mutations of a base document: for each
// basic element, its removal, its repetition, an empty, textual or long
// value, other numbers, decimals, dates and codes, the removal or change of
// its attributes; for each aggregate, its removal and its repetition; and
// the edits of inserts.
func candidates(base, doc string) []mutation {
	var out []mutation
	add := func(at int, old, new string) {
		out = append(out, mutation{base, edit{occurrence(doc, old, at), old, new}})
	}
	long := []string{strings.Repeat("x", 1025), strings.Repeat("x", 4097), strings.Repeat("ž", 40), " " + strings.Repeat("x", 19) + " "}
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
		if reNumber.MatchString(value) {
			for _, d := range []string{"0.01", "1.00", "-1.01", "0.001", "0.00000000001"} {
				add(m[0], whole, elem(attrs, addDecimal(value, d)))
			}
			add(m[0], whole, elem(attrs, "0"))
			add(m[0], whole, elem(attrs, "-"+value))
			add(m[0], whole, elem(attrs, value+"0"))
			add(m[0], whole, elem(attrs, value+".5"))
			add(m[0], whole, elem(attrs, strings.Repeat("0", max(31-len(value), 0))+value))
		}
		if reDate.MatchString(value) {
			for _, d := range dates {
				add(m[0], whole, elem(attrs, d))
			}
		}
		codes := tryCodes[name+"@"+parent(doc, m[0])]
		if codes == nil {
			codes = tryCodes[name]
		}
		for _, c := range codes {
			if c != value {
				add(m[0], whole, elem(attrs, c))
			}
		}
		for _, a := range reAttr.FindAllStringSubmatch(attrs, -1) {
			add(m[0], whole, elem(strings.Replace(attrs, a[0], "", 1), value))
			add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="X Y"`, 1), value))
			for _, c := range tryCodes[a[1]] {
				if c != a[2] {
					add(m[0], whole, elem(strings.Replace(attrs, a[0], ` `+a[1]+`="`+c+`"`, 1), value))
				}
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
	for _, e := range inserts[base] {
		out = append(out, mutation{base, e})
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
// reports of an SVRL report, or ok false when the report is an <error>
// element (Saxon stopped).
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
				if a.Name.Local == "id" {
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
	fmt.Fprintf(&b, "package hr\n\n")
	fmt.Fprintf(&b, "// cases are variants of the base documents, each with the rules of the\n// CIUS-HR that the schematron of the tax administration compiled by SchXslt\n// and run by Saxon-HE reports for it: for each of the %d rules raised by the\n// %d candidate mutations (%d of which stop Saxon), the variant that raises\n// the fewest other rules.\n", len(ids), total, stopped)
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
