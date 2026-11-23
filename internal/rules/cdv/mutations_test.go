package cdv

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
// schematron BR-FR-CDV that a change of one of the base statuses can make
// fail, that the package reports exactly the rules that the reference
// reports for that variant: the XSLT that the FNFE-MPE publishes for the
// schematron (France_RFE 1.4.0.04, CDAR/2xslt), run by Saxon-HE. They are
// rebuilt in two steps:
//
//	go test ./internal/rules/cdv -run '^TestVariants$' -variants DIR
//	(run the XSLT on DIR/*.xml, one SVRL report DIR/<name>.svrl each)
//	go test ./internal/rules/cdv -run '^TestUpdateCases$' -variants DIR
//
// TestVariants writes every candidate mutation of the base statuses, and
// their list (DIR/manifest.tsv); TestUpdateCases picks, for each rule that
// the reports raise, the variant that raises the fewest other rules, and
// writes cases_gen_test.go. The parity job (conformance/parity.sh) compares
// every variant with the XSLT.
var variantsDir = flag.String("variants", "", "directory of the variants of the mutation cases (TestVariants, TestUpdateCases)")

// testdata holds the examples of statuses of France_RFE.
const testdata = "../../../cdar/testdata/"

// base is a base status: an example of testdata, without its comments and
// with LF line ends, then changed by edits.
type base struct {
	file, name string
	edits      []edit
}

// senderB2G is the sender of a status of the public sector: the PPF acting
// for the public buyer (GlobalID 9999 of scheme 0238), for which the reasons
// of the status 210 follow the list of BR-FR-CDV-CL-09_MDT-113_210B2G.
var senderB2G = edit{1, "<ram:SenderTradeParty>\n", "<ram:SenderTradeParty>\n               <ram:GlobalID schemeID=\"0238\">9999</ram:GlobalID>\n"}

// bases are the base statuses, with the names of their constants: a status
// 200 Déposée sent to the PPF alone, 212 Encaissée with its collected
// amount, 213 Rejetée (transmission phase), 210 Refusée (processing phase)
// between companies and in the public sector, and 207 En litige with a
// requested action and changed values.
var bases = []base{
	{"UC1_F202500003_01-CDV-200_Deposee_POUR_PPF.xml", "baseDepositedPPF", nil},
	{"UC1_F202500003_07-CDV-212_Encaissee.xml", "baseCollected", nil},
	{"UC2_F202500004_02-CDV-213_Rejetee.xml", "baseRejected", nil},
	{"UC3_F202500005_04-CDV-210_Refusee.xml", "baseRefused", nil},
	{"UC3_F202500005_04-CDV-210_Refusee.xml", "baseRefusedB2G", []edit{senderB2G}},
	{"UC4_F202500006_04-CDV-207_En_litige.xml", "baseDisputed", nil},
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

// reComment is an XML comment and the spaces before it: most elements of
// the examples are followed by one on their line.
var reComment = regexp.MustCompile(`[ \t]*<!--[^>]*-->`)

// normalize returns an example without its byte order mark, its comments
// and its CR.
func normalize(data []byte) string {
	s := strings.TrimPrefix(string(data), "\uFEFF")
	return reComment.ReplaceAllString(strings.ReplaceAll(s, "\r\n", "\n"), "")
}

func readFile(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// load returns the document of a base status.
func load(t testing.TB, b base) string {
	t.Helper()
	doc := normalize(readFile(t, testdata+b.file))
	for _, e := range b.edits {
		doc = apply(t, doc, e)
	}
	return doc
}

// findings returns the sorted identifiers of the rules of the schematron
// that the package reports for the CDAR message doc.
func findings(t testing.TB, doc string) []string {
	t.Helper()
	return ruleIDs(check(t, doc))
}

// check returns the report of Check for the CDAR message doc.
func check(t testing.TB, doc string) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Report: r, Source: rules.SourceCDV}, root)
	return r
}

func ruleIDs(r *rules.Report) []string {
	var ids []string
	for _, f := range r.Findings {
		ids = append(ids, f.Rule)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// mutation is a candidate change of a base status.
type mutation struct {
	base string // name
	e    edit
}

var (
	reLeaf = regexp.MustCompile(`<((?:ram|udt|qdt):[A-Za-z]+)((?: [A-Za-z]+="[^"]*")*)>([^<]*)</(?:ram|udt|qdt):[A-Za-z]+>`)
	reAttr = regexp.MustCompile(` ([A-Za-z]+)="([^"]*)"`)
	reOpen = regexp.MustCompile(`(?m)^( *)<((?:rsm|ram):[A-Za-z]+)>\n`)
	reTag  = regexp.MustCompile(`<(/?)([A-Za-z]+:[A-Za-z0-9]+)[^>]*?(/?)>`)
)

// roles are the roles of the parties of the lists of the schematron, and
// one outside them.
var roles = []string{"BY", "AB", "DL", "SE", "SR", "PE", "PR", "II", "IV", "WK", "DFH", "XX"}

// tryCodes are values tried in the elements and attributes of these names;
// a key name@parent applies to the elements of that name in an aggregate of
// that local name only.
var tryCodes = map[string][]string{
	"ram:ID@BusinessProcessSpecifiedDocumentContextParameter": {"REGULATED", "NON_REGULATED", "B2C", "B2CINT", "B2BINT", "OUTOFSCOPE", "B2G", "A1", "regulated"},
	"ram:ID@GuidelineSpecifiedDocumentContextParameter":       {"urn.cpro.gouv.fr:1p0:CDV:invoice", "urn.cpro.gouv.fr:1p0:CDV:einvoicingF2", "urn.cpro.gouv.fr:1p0:CDV", "urn.cpro.gouv.fr:1p0:einvoicingF2"},
	"ram:RoleCode":                                 roles,
	"ram:TypeCode@AcknowledgementDocument":         {"23", "305", "380"},
	"ram:TypeCode@ReferenceReferencedDocument":     {"380", "381", "389", "393", "501", "261", "503", "326", "875", "23"},
	"ram:TypeCode@SpecifiedDocumentCharacteristic": {"MEN", "MPA", "RAP", "MAPTTC", "DIV", "MAJ", "XXX", "men"},
	"ram:StatusCode":                               {"10", "51", "43", "8", "48", "45", "39", "37", "50", "49", "47", "46", "1", "5", "99"},
	"ram:ProcessConditionCode":                     {"200", "201", "202", "205", "206", "207", "208", "210", "212", "213", "220", "221", "501", "601", "215", "999"},
	"ram:ReasonCode":                               {"NON_TRANSMISE", "DOUBLON", "TX_TVA_ERR", "ROUTAGE_ERR", "IRR_SYNTAX", "RETRAIT_MAN_SERV", "AUTRE", "JUSTIF_ABS", "CMD_ERR", "DEST_INC", "REJ_SEMAN", "LIVR_INCOMP", "XXX"},
	"ram:RequestedActionCode":                      {"NOA", "PIN", "NIN", "CNF", "CNP", "CNA", "OTH", "XXX"},
	"ram:GlobalID":                                 {"0000", "9999"},
	"schemeID":                                     {"0002", "0238", "0009", "0225"},
	"currencyID":                                   {"EUR", "USD"},
	"format":                                       {"204", "102"},
}

// refIssuer is the seller in the referenced invoice of the base statuses
// (MDG-39), into which inserts adds its role.
const refIssuer = "                    <ram:GlobalID schemeID=\"0002\">100000009</ram:GlobalID>\n               </ram:IssuerTradeParty>\n"

// inserts are edits of each base status beyond the candidates of each
// element: the role of the seller (MDT-132) and a recipient (MDG-40) in the
// referenced invoice, and a status code in a status detail (MDT-115).
var inserts = map[string][]edit{
	"baseCollected": func() []edit {
		var out []edit
		for _, r := range []string{"SE", "BY"} {
			out = append(out, edit{1, refIssuer, strings.Replace(refIssuer, "\n               </", "\n                    <ram:RoleCode>"+r+"</ram:RoleCode>\n               </", 1)})
		}
		for _, r := range []string{"BY", "WK", "XX"} {
			out = append(out, edit{1, refIssuer, refIssuer + "               <ram:RecipientTradeParty>\n                    <ram:GlobalID schemeID=\"0002\">200000008</ram:GlobalID>\n                    <ram:RoleCode>" + r + "</ram:RoleCode>\n               </ram:RecipientTradeParty>\n"})
		}
		return out
	}(),
	"baseDisputed": func() []edit {
		var out []edit
		for _, c := range []string{"207", "200", "210", "221", "501", "999"} {
			out = append(out, edit{1, "<ram:RequestedActionCode>", "<ram:ProcessConditionCode>" + c + "</ram:ProcessConditionCode>\n                    <ram:RequestedActionCode>"})
		}
		return out
	}(),
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

// candidates returns the candidate mutations of a base status: for each
// basic element, its removal, its repetition, an empty or textual value and
// other codes, the removal or change of its attributes; for each
// aggregate, its removal and its repetition; and the edits of inserts.
func candidates(name, doc string) []mutation {
	var out []mutation
	add := func(at int, old, new string) {
		out = append(out, mutation{name, edit{occurrence(doc, old, at), old, new}})
	}
	for _, m := range reLeaf.FindAllStringSubmatchIndex(doc, -1) {
		whole := doc[m[0]:m[1]]
		elem, attrs, value := doc[m[2]:m[3]], doc[m[4]:m[5]], doc[m[6]:m[7]]
		mk := func(attrs, value string) string { return "<" + elem + attrs + ">" + value + "</" + elem + ">" }
		add(m[0], whole, "")
		add(m[0], whole, whole+whole)
		add(m[0], whole, mk(attrs, ""))
		add(m[0], whole, mk(attrs, " "))
		add(m[0], whole, mk(attrs, "X y"))
		codes := tryCodes[elem+"@"+parent(doc, m[0])]
		if codes == nil {
			codes = tryCodes[elem]
		}
		for _, c := range codes {
			if c != value {
				add(m[0], whole, mk(attrs, c))
			}
		}
		for _, a := range reAttr.FindAllStringSubmatch(attrs, -1) {
			add(m[0], whole, mk(strings.Replace(attrs, a[0], "", 1), value))
			add(m[0], whole, mk(strings.Replace(attrs, a[0], ` `+a[1]+`="X Y"`, 1), value))
			for _, c := range tryCodes[a[1]] {
				if c != a[2] {
					add(m[0], whole, mk(strings.Replace(attrs, a[0], ` `+a[1]+`="`+c+`"`, 1), value))
				}
			}
		}
	}
	for _, m := range reOpen.FindAllStringSubmatchIndex(doc, -1) {
		indent, elem := doc[m[2]:m[3]], doc[m[4]:m[5]]
		end := strings.Index(doc[m[0]:], "\n"+indent+"</"+elem+">\n")
		if end < 0 {
			continue
		}
		block := doc[m[0] : m[0]+end+len("\n"+indent+"</"+elem+">\n")]
		add(m[0], block, "")
		add(m[0], block, block+block)
	}
	for _, e := range inserts[name] {
		out = append(out, mutation{name, e})
	}
	return out
}

// TestVariants writes the candidate mutations of the base statuses to
// -variants, with their list.
func TestVariants(t *testing.T) {
	var lines []string
	n := 0
	for _, b := range bases {
		doc := load(t, b)
		for _, m := range candidates(b.name, doc) {
			n++
			name := fmt.Sprintf("cdv%05d.xml", n)
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
			lines = append(lines, strings.Join([]string{name, b.name, strconv.Itoa(m.e.nth), strconv.Quote(m.e.old), strconv.Quote(m.e.new)}, "\t"))
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
	fmt.Fprintf(&b, "package cdv\n\n")
	fmt.Fprintf(&b, "// cases are variants of the base statuses, each with the rules of the\n// schematron BR-FR-CDV that the XSLT of the FNFE-MPE run by Saxon-HE reports\n// for it: for each of the %d rules raised by the %d candidate mutations (%d\n// of which stop Saxon), the variant that raises the fewest other rules.\n", len(ids), total, stopped)
	fmt.Fprintf(&b, "var cases = []struct {\n\tbase string\n\te    edit\n\twant []string\n}{\n")
	for _, id := range ids {
		v := best[id]
		key := v.base + "\x00" + strconv.Itoa(v.e.nth) + v.e.old + "\x00" + v.e.new
		if seen[key] {
			continue
		}
		seen[key] = true
		fmt.Fprintf(&b, "\t{%q, edit{%d, %q, %q}, []string{%s}},\n", v.base, v.e.nth, v.e.old, v.e.new, quoteAll(v.want))
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
		docs[b.name] = load(t, b)
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
