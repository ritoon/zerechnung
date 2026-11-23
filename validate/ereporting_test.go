package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

const (
	ereportingTransactions = "../ereporting/testdata/transactions.xml" // transactions report (flux 10.1 and 10.3)
	ereportingPayments     = "../ereporting/testdata/payments.xml"     // payments report (flux 10.2 and 10.4)
)

// TestEReporting checks valid e-reporting reports: whatever the rule sets,
// only the warning ER-02 is reported.
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
			if got := ruleIDs(r); !slices.Equal(got, []string{ruleERScope}) {
				t.Errorf("%s %+v: findings = %v", name, opts, r.Findings)
				continue
			}
			f := r.Findings[0]
			if f.Severity != Warning || f.Source != SourceSyntax || f.Path != "/Report" || f.Line != 2 ||
				!strings.Contains(f.Message, "France_RFE 1.4.0.04") {
				t.Errorf("%s %+v: ER-02 = %+v", name, opts, f)
			}
			if !r.Valid() || r.Profile != invoice.ProfileUnknown {
				t.Errorf("%s %+v: valid = %v, profile = %v", name, opts, r.Valid(), r.Profile)
			}
		}
	}
}

// TestEReportingFindings checks the schema and ER-01 on altered copies of
// valid reports. A part that the schema check reports is not reported again
// by ER-01. Without the Schema rule set, only ER-02 is left.
func TestEReportingFindings(t *testing.T) {
	const inv = "/Report/TransactionsReport/Invoice[1]"
	for _, c := range []struct {
		name, file, old, new string
		rules                []string
		path                 string
	}{
		{"unreadable date", ereportingTransactions, "<DueDate>20261015</DueDate>", "<DueDate>2026-10-15</DueDate>",
			[]string{ruleERValue, ruleERScope}, inv + "/DueDate"},
		{"unreadable time", ereportingPayments, "<DateTimeString>20261011094500</DateTimeString>", "<DateTimeString>11/10/2026</DateTimeString>",
			[]string{ruleERValue, ruleERScope}, "/Report/ReportDocument/IssueDateTime/DateTimeString"},
		{"unreadable decimal", ereportingTransactions, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>227,50</TaxTotal>",
			[]string{"XSD-TYPE", ruleERScope}, "/Report/TransactionsReport/Transactions/TaxTotal"},
		{"unreadable boolean", ereportingTransactions, `ChargeIndicator="false"`, `ChargeIndicator="non"`,
			[]string{"XSD-TYPE", ruleERScope}, inv + "/AllowanceCharge"},
		{"unknown element", ereportingPayments, "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><Comment>x</Comment>",
			[]string{"XSD-UNEXPECTED", ruleERScope}, "/Report/ReportDocument/Comment"},
		{"unknown attribute on a date", ereportingTransactions, "<DueDate>20261015</DueDate>", `<DueDate format="102">2026-10-15</DueDate>`,
			[]string{"XSD-ATTRIBUTE", ruleERValue, ruleERScope}, inv + "/DueDate"},
		{"missing element", ereportingTransactions, "<CurrencyCode>EUR</CurrencyCode>", "",
			[]string{"XSD-MISSING", ruleERScope}, inv},
		{"missing attribute", ereportingPayments, `<Id schemeId="0238">0042</Id>`, `<Id>0042</Id>`,
			[]string{"XSD-ATTRIBUTE-MISSING", ruleERScope}, "/Report/ReportDocument/Sender/Id"},
		{"misplaced element", ereportingPayments, "<TypeCode>IN</TypeCode>", "",
			[]string{"XSD-MISSING", ruleERScope}, "/Report/ReportDocument"},
		{"wrong root namespace", ereportingPayments, "<Report>", `<Report xmlns="urn:x">`,
			[]string{"CII"}, "/Report"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := strings.Replace(readXML(t, c.file), c.old, c.new, 1)
			r := XML([]byte(doc), nil)
			if got := ruleIDs(r); !slices.Equal(got, c.rules) {
				t.Fatalf("findings = %v, want rules %v", r.Findings, c.rules)
			}
			if r.Findings[0].Path != c.path || (r.Findings[0].Severity == Error) == r.Valid() {
				t.Errorf("first finding = %+v, want path %s", r.Findings[0], c.path)
			}
			if c.rules[0] == "CII" {
				return
			}
			// Without the Schema rule set, the structure and the values
			// are not checked.
			r = XML([]byte(doc), &Options{Rules: EN16931 | France})
			if got := ruleIDs(r); !slices.Equal(got, []string{ruleERScope}) {
				t.Errorf("without Schema: findings = %v", r.Findings)
			}
		})
	}
}

func TestEReportingCatalog(t *testing.T) {
	ids := map[string]bool{}
	for _, r := range Catalog(Schema) {
		ids[r.ID] = true
	}
	if !ids[ruleERValue] || !ids[ruleERScope] {
		t.Errorf("Catalog(Schema) lacks the e-reporting rules")
	}
	for _, r := range Catalog(^Schema) {
		if r.ID == ruleERValue || r.ID == ruleERScope {
			t.Errorf("%s listed outside the Schema rule set", r.ID)
		}
	}
}
