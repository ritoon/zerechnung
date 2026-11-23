package schema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// cdarTestdata are the examples of statuses of France_RFE copied into the
// testdata of package cdar.
const cdarTestdata = "../../cdar/testdata"

// checkCDAR parses data and runs CheckCDAR.
func checkCDAR(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	CheckCDAR(&rules.Checker{Report: r, Source: rules.SourceSyntax}, root)
	return r
}

func TestCDARTestdata(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(cdarTestdata, "*.xml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no CDAR example in %s (%v)", cdarTestdata, err)
	}
	for _, path := range paths {
		if r := checkCDAR(t, readFile(t, path)); len(r.Findings) > 0 {
			t.Errorf("%s: unexpected findings:\n%s", filepath.Base(path), r)
		}
	}
	if CDARVersion() != "D22B" {
		t.Errorf("CDARVersion() = %q", CDARVersion())
	}
}

// cdarRefused is the status 210 Refusée of the use case UC3.
const cdarRefused = cdarTestdata + "/UC3_F202500005_04-CDV-210_Refusee.xml"

// cdarMutations are edits of cdarRefused that check each kind of finding.
var cdarMutations = []struct {
	name, old, new string
	rule, msg      string // rule "" = valid
}{
	{name: "no recipient", old: "<ram:RecipientTradeParty>\n               <ram:GlobalID schemeID=\"0002\">100000009</ram:GlobalID>\n               <ram:Name>VENDEUR</ram:Name>",
		new: "<ram:Recipient>\n               <ram:GlobalID schemeID=\"0002\">100000009</ram:GlobalID>\n               <ram:Name>VENDEUR</ram:Name>", rule: RuleUnexpected, msg: "ram:Recipient is not allowed in rsm:ExchangedDocument"},
	{name: "no status detail", old: "<ram:SequenceNumeric>1</ram:SequenceNumeric>"},
	{name: "order", old: "<ram:TypeCode>23</ram:TypeCode>\n          <ram:IssueDateTime>\n               <udt:DateTimeString format=\"204\">20250701180000</udt:DateTimeString>\n          </ram:IssueDateTime>",
		new: "<ram:IssueDateTime>\n               <udt:DateTimeString format=\"204\">20250701180000</udt:DateTimeString>\n          </ram:IssueDateTime>\n          <ram:TypeCode>23</ram:TypeCode>", rule: RuleOrder, msg: "TypeCode"},
	{name: "twice", old: "<ram:TypeCode>23</ram:TypeCode>", new: "<ram:TypeCode>23</ram:TypeCode><ram:TypeCode>23</ram:TypeCode>", rule: RuleCount, msg: "TypeCode"},
	{name: "bad indicator", old: "<udt:Indicator>false</udt:Indicator>", new: "<udt:Indicator>non</udt:Indicator>", rule: RuleType, msg: "xs:boolean"},
	{name: "bad sequence number", old: "<ram:SequenceNumeric>1</ram:SequenceNumeric>", new: "<ram:SequenceNumeric>un</ram:SequenceNumeric>", rule: RuleType, msg: "xs:decimal"},
	{name: "unknown attribute", old: "<ram:IssuerAssignedID>", new: `<ram:IssuerAssignedID foo="bar">`, rule: RuleAttribute, msg: "foo"},
	{name: "text in an aggregate", old: "<ram:SpecifiedDocumentStatus>", new: "<ram:SpecifiedDocumentStatus>texte", rule: RuleText},
	{name: "no guideline", old: "<ram:GuidelineSpecifiedDocumentContextParameter>\n               <ram:ID>urn.cpro.gouv.fr:1p0:CDV:invoice</ram:ID>\n          </ram:GuidelineSpecifiedDocumentContextParameter>", rule: RuleMissing, msg: "GuidelineSpecifiedDocumentContextParameter"},
	{name: "foreign namespace", old: "<ram:ReasonCode>TX_TVA_ERR</ram:ReasonCode>", new: `<x:ReasonCode xmlns:x="urn:x">TX_TVA_ERR</x:ReasonCode>`, rule: RuleNamespace},
}

// reComment is an XML comment and the spaces before it.
var reComment = regexp.MustCompile(`[ \t]*<!--[^>]*-->`)

func TestCDARMutations(t *testing.T) {
	// Without its comments, which follow most elements on their line.
	base := reComment.ReplaceAllString(strings.ReplaceAll(string(readFile(t, cdarRefused)), "\r\n", "\n"), "")
	for _, m := range cdarMutations {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(base, m.old) {
				t.Fatalf("%q not in %s", m.old, cdarRefused)
			}
			doc := strings.Replace(base, m.old, m.new, 1)
			if m.name == "no recipient" {
				doc = strings.Replace(doc, "</ram:RecipientTradeParty>", "</ram:Recipient>", 1)
			}
			r := checkCDAR(t, []byte(doc))
			if m.rule == "" {
				if len(r.Findings) > 0 {
					t.Errorf("unexpected findings:\n%s", r)
				}
				return
			}
			if !r.Has(m.rule) {
				t.Fatalf("want %s, got:\n%s", m.rule, r)
			}
			if m.msg != "" && !strings.Contains(r.String(), m.msg) {
				t.Errorf("want a message with %q, got:\n%s", m.msg, r)
			}
		})
	}
}

func TestCDARRoot(t *testing.T) {
	r := checkCDAR(t, []byte(`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`))
	if !r.Has(RuleRoot) || len(r.Findings) != 1 {
		t.Errorf("CII root: %s", r)
	}
	r = checkCDAR(t, []byte(`<CrossDomainAcknowledgementAndResponse xmlns="urn:x"/>`))
	if !r.Has(RuleNamespace) || !r.Has(RuleMissing) {
		t.Errorf("wrong namespace: %s", r)
	}
	r = checkCDAR(t, []byte(`<rsm:CrossDomainAcknowledgementAndResponse xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossDomainAcknowledgementAndResponse:100"/>`))
	if !r.Has(RuleMissing) || !strings.Contains(r.String(), "ExchangedDocument") || !strings.Contains(r.String(), "AcknowledgementDocument") {
		t.Errorf("empty message: %s", r)
	}
	CheckCDAR(&rules.Checker{Report: &rules.Report{}}, nil)
}

// TestFNFECDAR checks the examples of statuses of the FNFE-MPE package
// (FX_FNFE, a checkout of github.com/fnfempe/France_RFE): each is valid
// against the CDAR D22B schema.
func TestFNFECDAR(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST", "CDAR", "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 50 {
		t.Fatalf("%d CDAR examples under %s, want at least 50", len(paths), dir)
	}
	for _, path := range paths {
		if r := checkCDAR(t, readFile(t, path)); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", filepath.Base(path), r)
		}
	}
	t.Logf("%d CDAR examples valid against the CDAR D22B schema", len(paths))
}
