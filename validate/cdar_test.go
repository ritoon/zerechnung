package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules/cdv"
	"github.com/ritoon/zerechnung/invoice"
)

const cdarStatus = "../cdar/testdata/UC3_F202500005_04-CDV-210_Refusee.xml" // status 210 Refusée of France_RFE

// TestCDAR checks a valid CDAR status: whatever the rule sets, nothing is
// reported, and the report has no profile.
func TestCDAR(t *testing.T) {
	doc := []byte(readXML(t, cdarStatus))
	for _, opts := range []*Options{nil, {Rules: Schema}, {Rules: Default | France | Germany | Peppol}, {Auto: true}, {Profile: invoice.ProfileEN16931}} {
		r := XML(doc, opts)
		if len(r.Findings) > 0 || !r.Valid() || r.Profile != invoice.ProfileUnknown {
			t.Errorf("%+v: %v, profile %v", opts, r.Findings, r.Profile)
		}
	}
}

// TestCDARFindings checks the schema and the rules BR-FR-CDV on altered
// copies of a valid status. Without the Schema rule set, only the rules
// BR-FR-CDV are left.
func TestCDARFindings(t *testing.T) {
	noStatus := []string{"BR-FR-CDV-12_MDT-105", "BR-FR-CDV-CL-09_MDT-113_200", "BR-FR-CDV-CL-09_MDT-113_213", "BR-FR-CDV-CL-09_MDT-113_206",
		"BR-FR-CDV-CL-09_MDT-113_208", "BR-FR-CDV-CL-09_MDT-113_221", "BR-FR-CDV-CL-09_MDT-113_501"}
	for _, c := range []struct {
		name, old, new string
		rules          []string // with the default rule sets
		cdv            []string // without the Schema rule set
	}{
		{"reason outside the lists", "<ram:ReasonCode>TX_TVA_ERR</ram:ReasonCode>", "<ram:ReasonCode>XXX</ram:ReasonCode>",
			[]string{"BR-FR-CDV-CL-09_MDT-113", "BR-FR-CDV-CL-09_MDT-113_210"}, []string{"BR-FR-CDV-CL-09_MDT-113", "BR-FR-CDV-CL-09_MDT-113_210"}},
		// Without the status code, the reason is checked against the lists of
		// the other statuses, as Saxon does with the XSLT of France_RFE.
		{"no status code", "<ram:ProcessConditionCode>210</ram:ProcessConditionCode>", "", noStatus, noStatus},
		{"unreadable sequence number", "<ram:SequenceNumeric>1</ram:SequenceNumeric>", "<ram:SequenceNumeric>un</ram:SequenceNumeric>",
			[]string{"XSD-TYPE"}, nil},
		{"no profile", "<ram:GuidelineSpecifiedDocumentContextParameter>", "<ram:Foo>",
			[]string{"XSD-UNEXPECTED", "XSD-MISSING", "BR-FR-CDV-02_MDT-3"}, []string{"BR-FR-CDV-02_MDT-3"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := readXML(t, cdarStatus)
			if !strings.Contains(doc, c.old) {
				t.Fatalf("%q not found", c.old)
			}
			doc = strings.Replace(doc, c.old, c.new, 1)
			if c.old == "<ram:GuidelineSpecifiedDocumentContextParameter>" {
				doc = strings.Replace(doc, "</ram:GuidelineSpecifiedDocumentContextParameter>", "</ram:Foo>", 1)
			}
			r := XML([]byte(doc), nil)
			if got := ruleIDs(r); !slices.Equal(got, c.rules) {
				t.Errorf("findings = %v", r.Findings)
			}
			for _, f := range r.Findings {
				want := SourceCDV
				if strings.HasPrefix(f.Rule, "XSD-") {
					want = SourceSyntax
				}
				if f.Source != want || f.Line == 0 || !strings.HasPrefix(f.Path, "/rsm:CrossDomainAcknowledgementAndResponse") {
					t.Errorf("finding %+v", f)
				}
			}
			if r.Valid() {
				t.Error("valid")
			}
			if got := ruleIDs(XML([]byte(doc), &Options{Rules: EN16931 | France})); !slices.Equal(got, c.cdv) {
				t.Errorf("without Schema: %v", got)
			}
		})
	}
	r := XML([]byte(`<x:Foo xmlns:x="urn:x"/>`), nil)
	if len(r.Findings) != 1 || !strings.Contains(r.Findings[0].Message, "a CDAR status rsm:CrossDomainAcknowledgementAndResponse") ||
		!strings.Contains(r.Findings[0].Message, "nor a French e-reporting Report") {
		t.Errorf("another document: %v", r.Findings)
	}
}

// TestCatalogCDAR checks that the France rule set lists the rules BR-FR-CDV,
// once, and that no other rule set does.
func TestCatalogCDAR(t *testing.T) {
	count := func(set RuleSet) int {
		n := 0
		for _, r := range Catalog(set) {
			if strings.HasPrefix(r.ID, "BR-FR-CDV-") || r.ID == "BR-FR-04_MDT-91" {
				n++
			}
		}
		return n
	}
	if got, want := count(France), len(cdv.Catalog()); got != want || want != 43 {
		t.Errorf("Catalog(France): %d rules BR-FR-CDV, want %d (43)", got, want)
	}
	if got := count(^RuleSet(0) &^ France); got != 0 {
		t.Errorf("Catalog without France: %d rules BR-FR-CDV", got)
	}
}
