package schema

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The e-reporting test reports of package ereporting.
const (
	ereportingTransactions = "../../ereporting/testdata/transactions.xml"
	ereportingPayments     = "../../ereporting/testdata/payments.xml"
)

// checkEReporting parses data and runs CheckEReporting.
func checkEReporting(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	CheckEReporting(&rules.Checker{Report: r, Source: rules.SourceSyntax}, root)
	return r
}

func TestEReportingTestdata(t *testing.T) {
	for _, name := range []string{ereportingTransactions, ereportingPayments} {
		if r := checkEReporting(t, readFile(t, name)); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", name, r)
		}
	}
	if EReportingVersion() != "1.4.0.04" {
		t.Errorf("EReportingVersion() = %q", EReportingVersion())
	}
}

// ereportingMutations alter the transactions report of package ereporting;
// rule is the expected finding, or "" for a report still valid.
var ereportingMutations = []struct {
	name, old, new, rule, msg string
}{
	{"empty optional name", "<Name>Transactions septembre 2026</Name>", "", "", ""},
	{"missing identifier", "<Id>TR-2026-09-0001</Id>", "", RuleMissing, "Id is missing in ReportDocument"},
	{"misplaced type code", "<TypeCode>IN</TypeCode>\n    <Sender>", "<Sender>", RuleMissing, "TypeCode"},
	{"unknown element", "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><Extra/>", RuleUnexpected, "Extra"},
	{"qualified element", "<TypeCode>IN</TypeCode>", `<TypeCode xmlns="report">IN</TypeCode>`, RuleNamespace, "TypeCode"},
	{"repeated element", "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><TypeCode>RE</TypeCode>", RuleCount, "TypeCode"},
	{"missing scheme", `<Id schemeId="0238">0042</Id>`, "<Id>0042</Id>", RuleAttributeMissing, "schemeId"},
	{"unknown attribute", `<Id schemeId="0238">0042</Id>`, `<Id schemeId="0238" type="x">0042</Id>`, RuleAttribute, "type"},
	{"decimal", "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>227,50</TaxTotal>", RuleType, "xs:decimal"},
	{"integer", "<TransactionsCount>42</TransactionsCount>", "<TransactionsCount>4.2</TransactionsCount>", RuleType, "xs:integer"},
	{"boolean", `ChargeIndicator="false"`, `ChargeIndicator="no"`, RuleType, "xs:boolean"},
	{"missing boolean", `ChargeIndicator="false"`, "", RuleAttributeMissing, "ChargeIndicator"},
	{"missing VAT breakdown", "<TaxSubtotal>\n        <TaxPercent>20</TaxPercent>\n        <TaxableAmount>1000.00</TaxableAmount>\n        <TaxTotal>200.00</TaxTotal>\n      </TaxSubtotal>\n      <TaxSubtotal>\n        <TaxPercent>5.5</TaxPercent>\n        <TaxableAmount>500.00</TaxableAmount>\n        <TaxTotal>27.50</TaxTotal>\n      </TaxSubtotal>", "", RuleMissing, "TaxSubtotal"},
	{"text in an aggregate", "<ReportPeriod>", "<ReportPeriod>texte", RuleText, "ReportPeriod"},
	{"both reports", "</TransactionsReport>", "</TransactionsReport><PaymentsReport><ReportPeriod><StartDate>20260901</StartDate><EndDate>20260930</EndDate></ReportPeriod></PaymentsReport>", "", ""},
	{"date as a string", "<IssueDate>20260915</IssueDate>", "<IssueDate>15/09/2026</IssueDate>", "", ""},
}

func TestEReportingMutations(t *testing.T) {
	src := readFile(t, ereportingTransactions)
	for _, c := range ereportingMutations {
		t.Run(c.name, func(t *testing.T) {
			r := checkEReporting(t, replace(t, src, c.old, c.new))
			if c.rule == "" {
				if len(r.Findings) > 0 {
					t.Errorf("unexpected findings:\n%s", r)
				}
				return
			}
			for _, f := range r.Findings {
				if f.Rule == c.rule && strings.Contains(f.Message, c.msg) {
					return
				}
			}
			t.Errorf("no %s finding with %q:\n%s", c.rule, c.msg, r)
		})
	}
}

func TestEReportingRoot(t *testing.T) {
	for _, doc := range []string{`<Report xmlns="report"/>`, `<Rapport/>`} {
		r := checkEReporting(t, []byte(doc))
		if len(r.Findings) != 1 || r.Findings[0].Rule != RuleRoot || !strings.Contains(r.Findings[0].Message, "expected Report in no namespace") {
			t.Errorf("%s: findings:\n%s", doc, r)
		}
	}
	CheckEReporting(&rules.Checker{Report: &rules.Report{}}, nil)
}

// TestEReportingXmllint checks that xmllint (libxml2), given the e-reporting
// schemas of the checkout of France_RFE named by FX_FNFE, accepts and
// rejects the same reports as CheckEReporting: the test reports and the
// mutations.
func TestEReportingXmllint(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE is not set")
	}
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Fatal("FX_FNFE is set but xmllint is missing")
	}
	xsd := filepath.Join(dir, "FNFE_RFE_INVOICE", "Flux10", "xsd", "ereporting.xsd")
	docs := map[string][]byte{"transactions": readFile(t, ereportingTransactions), "payments": readFile(t, ereportingPayments)}
	for _, c := range ereportingMutations {
		docs[c.name] = replace(t, docs["transactions"], c.old, c.new)
	}
	tmp := t.TempDir()
	var invalid int
	for name, data := range docs {
		path := filepath.Join(tmp, "report.xml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(xmllint, "--noout", "--nonet", "--schema", xsd, path).CombinedOutput()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatal(err)
		}
		// xmllint exits with 3 when the document is not valid, and with
		// another code on any other error.
		if err != nil && exit.ExitCode() != 3 {
			t.Fatalf("%s: xmllint: %v\n%s", name, err, out)
		}
		libxml := err == nil
		r := checkEReporting(t, data)
		if ours := len(r.Findings) == 0; ours != libxml {
			t.Errorf("%s: valid for xmllint %t, for CheckEReporting %t\nxmllint: %s\nCheckEReporting:\n%s", name, libxml, ours, out, r)
		}
		if !libxml {
			invalid++
		}
	}
	if invalid == 0 || invalid == len(docs) {
		t.Errorf("%d documents of %d invalid", invalid, len(docs))
	}
}
