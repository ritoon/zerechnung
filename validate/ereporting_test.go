package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules/flux10"
	"github.com/ritoon/zerechnung/invoice"
)

const (
	ereportingTransactions = "../ereporting/testdata/transactions.xml" // transactions report (flux 10.1 and 10.3)
	ereportingPayments     = "../ereporting/testdata/payments.xml"     // payments report (flux 10.2 and 10.4)
)

// TestEReporting checks valid e-reporting reports: whatever the rule sets,
// nothing is reported.
func TestEReporting(t *testing.T) {
	for _, name := range []string{ereportingTransactions, ereportingPayments} {
		doc := []byte(readXML(t, name))
		for _, opts := range []*Options{
			nil,
			{Rules: Schema},
			{Rules: Default | France | ChorusPro | Peppol},
			{Rules: EN16931},
			{Auto: true},
		} {
			r := XML(doc, opts)
			if len(r.Findings) > 0 || !r.Valid() || r.Profile != invoice.ProfileUnknown {
				t.Errorf("%s %+v: findings = %v, profile = %v", name, opts, r.Findings, r.Profile)
			}
		}
	}
}

// TestEReportingFindings checks the schema, ER-01 and the rules of the
// schematron Flux10 on altered copies of valid reports. A part that the
// schema check reports is not reported again by ER-01. Without the Schema
// rule set, only the rules of the schematron are left.
func TestEReportingFindings(t *testing.T) {
	const inv = "/Report/TransactionsReport/Invoice[1]"
	for _, c := range []struct {
		name, file, old, new string
		rules                []string // with the default rule sets
		flux10               []string // the rules of the schematron Flux10 among them
		path                 string   // of the first finding
	}{
		{"unreadable date", ereportingTransactions, "<DueDate>20261015</DueDate>", "<DueDate>2026-10-15</DueDate>",
			[]string{ruleERValue, "F10-FACTURE-G1.09b"}, []string{"F10-FACTURE-G1.09b"}, inv + "/DueDate"},
		{"unreadable time", ereportingPayments, "<DateTimeString>20261011094500</DateTimeString>", "<DateTimeString>11/10/2026</DateTimeString>",
			[]string{ruleERValue, "F10-TRANSMISSION-G7.53"}, []string{"F10-TRANSMISSION-G7.53"}, "/Report/ReportDocument/IssueDateTime/DateTimeString"},
		{"unreadable decimal", ereportingTransactions, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>227,50</TaxTotal>",
			[]string{"XSD-TYPE", "F10-TRANSAC-G1.14b"}, []string{"F10-TRANSAC-G1.14b"}, "/Report/TransactionsReport/Transactions/TaxTotal"},
		{"unreadable boolean", ereportingTransactions, `ChargeIndicator="false"`, `ChargeIndicator="non"`,
			[]string{"XSD-TYPE"}, nil, inv + "/AllowanceCharge"},
		{"unknown element", ereportingPayments, "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><Comment>x</Comment>",
			[]string{"XSD-UNEXPECTED"}, nil, "/Report/ReportDocument/Comment"},
		{"unknown attribute on a date", ereportingTransactions, "<DueDate>20261015</DueDate>", `<DueDate format="102">2026-10-15</DueDate>`,
			[]string{"XSD-ATTRIBUTE", ruleERValue, "F10-FACTURE-G1.09b"}, []string{"F10-FACTURE-G1.09b"}, inv + "/DueDate"},
		{"missing element", ereportingTransactions, "<CurrencyCode>EUR</CurrencyCode>", "",
			[]string{"XSD-MISSING"}, nil, inv},
		{"missing attribute", ereportingPayments, `<Id schemeId="0238">0042</Id>`, `<Id>0042</Id>`,
			[]string{"XSD-ATTRIBUTE-MISSING", "F10-EMETTEUR-G6.22"}, []string{"F10-EMETTEUR-G6.22"}, "/Report/ReportDocument/Sender/Id"},
		{"misplaced element", ereportingPayments, "<TypeCode>IN</TypeCode>", "",
			[]string{"XSD-MISSING"}, nil, "/Report/ReportDocument"},
		{"rule of the PPF", ereportingTransactions, "<CategoryCode>TLB1</CategoryCode>", "<CategoryCode>TLB9</CategoryCode>",
			[]string{"F10-TRANSAC-G1.68"}, []string{"F10-TRANSAC-G1.68"}, "/Report/TransactionsReport/Transactions/CategoryCode"},
		{"wrong root namespace", ereportingPayments, "<Report>", `<Report xmlns="urn:x">`,
			[]string{"CII"}, nil, "/Report"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := strings.Replace(readXML(t, c.file), c.old, c.new, 1)
			r := XML([]byte(doc), nil)
			if got := ruleIDs(r); !slices.Equal(got, c.rules) {
				t.Fatalf("findings = %v, want rules %v", r.Findings, c.rules)
			}
			if r.Findings[0].Path != c.path || r.Valid() {
				t.Errorf("first finding = %+v, want path %s", r.Findings[0], c.path)
			}
			for _, f := range r.Findings {
				if strings.HasPrefix(f.Rule, "F10-") != (f.Source == SourceFlux10) || f.Severity != Error {
					t.Errorf("finding %+v", f)
				}
			}
			if c.rules[0] == "CII" {
				return
			}
			// Without the Schema rule set, the structure and the values
			// are not checked; the rules of the schematron are.
			r = XML([]byte(doc), &Options{Rules: EN16931})
			if got := ruleIDs(r); !slices.Equal(got, c.flux10) {
				t.Errorf("without Schema: findings = %v, want %v", r.Findings, c.flux10)
			}
		})
	}
}

// TestEReportingCatalog checks that ER-01 is listed in the Schema rule set,
// and the rules of the schematron Flux10 in the France rule set, alone.
func TestEReportingCatalog(t *testing.T) {
	count := func(set RuleSet) (er, f10 int) {
		for _, r := range Catalog(set) {
			switch {
			case r.ID == ruleERValue:
				er++
			case strings.HasPrefix(r.ID, "F10-"):
				f10++
			}
		}
		return er, f10
	}
	if er, f10 := count(Schema); er != 1 || f10 != 0 {
		t.Errorf("Catalog(Schema): %d ER-01, %d rules of Flux10", er, f10)
	}
	if er, f10 := count(France); er != 0 || f10 != len(flux10.Catalog()) || f10 != 150 {
		t.Errorf("Catalog(France): %d ER-01, %d rules of Flux10", er, f10)
	}
	if er, f10 := count(^(Schema | France)); er != 0 || f10 != 0 {
		t.Errorf("Catalog outside Schema and France: %d ER-01, %d rules of Flux10", er, f10)
	}
}
