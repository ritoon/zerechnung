package zerechnung

import (
	"bufio"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/validate"
)

var ruleCoverage = flag.String("rule-coverage", "", "file of the rules that the test suite raised (ZERECHNUNG_RULE_TRACE of a run built with -tags ruletrace)")

// ruleCoverageKnown lists the rules that the test suite does not raise yet.
const ruleCoverageKnown = "testdata/rule-coverage.txt"

// outsideCatalogs are the rules that the package reports without listing
// them in a catalog: the syntax findings of validate.XML and validate.Tree
// on a document that is not XML or not an invoice, and the rules of
// fatturapa.VerifySignature.
var outsideCatalogs = []string{
	"XML", "CII",
	fatturapa.RuleFileName, fatturapa.RuleFileSize, fatturapa.RuleCertExpired,
	fatturapa.RuleCertRevoked, fatturapa.RuleSignatureInvalid, fatturapa.RuleNoSigningTime,
	fatturapa.RuleUntrustedCA, fatturapa.RuleSigningTimeAfter, fatturapa.RuleCertInvalid,
	fatturapa.RuleNotSigned, fatturapa.RuleNotQualified, fatturapa.RuleSeal,
	fatturapa.RuleTrustNotChecked, fatturapa.RuleRevocationUnknown, fatturapa.RuleNotBaselineB,
	fatturapa.RuleNotFatturaPA,
}

// TestRuleCoverage checks that the test suite raises each rule of the
// catalogs (validate.Catalog of every rule set, ContainerCatalog and
// SignatureCatalog) and of
// outsideCatalogs at least once, unless testdata/rule-coverage.txt lists it.
// It is the dynamic counterpart of the TestCatalogComplete tests, which read
// the sources: a rule that is written but that no test makes fail could be
// unreachable, or wrong without any test seeing it.
//
// The rules raised come from a run of the whole suite with the official
// artefacts (conformance/fetch.sh), built with the tag ruletrace:
//
//	ZERECHNUNG_RULE_TRACE=$PWD/rules.txt go test -tags ruletrace ./...
//	go test -run '^TestRuleCoverage$' . -rule-coverage rules.txt
//
// The job "Tests sur les artefacts officiels" of conformance.yml runs both.
// Without -rule-coverage, the test only checks testdata/rule-coverage.txt.
// A rule of that file that the suite raises fails the test too, so that the
// file only lists the rules still not raised, as does a rule raised that is
// neither in a catalog nor in outsideCatalogs.
func TestRuleCoverage(t *testing.T) {
	rules := map[string]bool{}
	for _, r := range validate.Catalog(^validate.RuleSet(0)) {
		rules[r.ID] = true
	}
	for _, r := range ContainerCatalog {
		rules[r.ID] = true
	}
	for _, r := range SignatureCatalog {
		rules[r.ID] = true
	}
	for _, id := range outsideCatalogs {
		rules[id] = true
	}
	known := readRuleCoverageKnown(t)
	for id := range known {
		if !rules[id] {
			t.Errorf("%s: %s is in no catalog", ruleCoverageKnown, id)
		}
	}
	if *ruleCoverage == "" {
		return
	}
	data, err := os.ReadFile(*ruleCoverage)
	if err != nil {
		t.Fatal(err)
	}
	raised := map[string]bool{}
	for id := range strings.FieldsSeq(string(data)) {
		raised[id] = true
	}
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	n := 0
	for _, id := range ids {
		switch {
		case raised[id] && known[id]:
			t.Errorf("%s is raised by the test suite: remove it from %s", id, ruleCoverageKnown)
		case raised[id]:
			n++
		case !known[id]:
			t.Errorf("%s is raised by no test", id)
		}
	}
	var extra []string
	for id := range raised {
		if !rules[id] {
			extra = append(extra, id)
		}
	}
	slices.Sort(extra)
	for _, id := range extra {
		t.Errorf("%s is raised but is in no catalog", id)
	}
	t.Logf("%d rules of %d raised by the test suite, %d listed in %s", n, len(rules), len(known), ruleCoverageKnown)
}

// readRuleCoverageKnown reads testdata/rule-coverage.txt: one rule per line;
// a line that starts with # is a comment.
func readRuleCoverageKnown(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(ruleCoverageKnown)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	known := map[string]bool{}
	s := bufio.NewScanner(f)
	for line := 1; s.Scan(); line++ {
		text := strings.TrimSpace(s.Text())
		switch {
		case text == "" || strings.HasPrefix(text, "#"):
		case strings.ContainsAny(text, " \t"):
			t.Errorf("%s:%d: %q is not a single rule", ruleCoverageKnown, line, text)
		case known[text]:
			t.Errorf("%s:%d: %s twice", ruleCoverageKnown, line, text)
		default:
			known[text] = true
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return known
}
