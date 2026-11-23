package ereporting

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

func readTestdata(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// schemaFindings returns the findings of the check of data against the
// e-reporting schema.
func schemaFindings(t testing.TB, data []byte) []rules.Finding {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	schema.CheckEReporting(&rules.Checker{Report: r, Source: "syntax"}, root)
	return r.Findings
}

func TestParseTransactions(t *testing.T) {
	rep, err := Parse(readTestdata(t, "transactions.xml"))
	if err != nil {
		t.Fatal(err)
	}
	d := rep.Document
	if d.ID != "TR-2026-09-0001" || d.TypeCode != "IN" || !d.IssueDateTime.Equal(time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("document = %+v", d)
	}
	if d.Sender != (Party{ID: "0042", SchemeID: "0238", Name: "Plateforme Exemple", RoleCode: "WK"}) {
		t.Errorf("sender = %+v", d.Sender)
	}
	if d.Issuer.URI != "ventes@vendeur.example" || d.Issuer.RoleCode != "SE" {
		t.Errorf("issuer = %+v", d.Issuer)
	}
	if rep.Payments != nil {
		t.Error("payments read from a transactions report")
	}
	tr := rep.Transactions
	if tr == nil || len(tr.Invoices) != 2 || len(tr.Transactions) != 1 {
		t.Fatalf("transactions = %+v", tr)
	}
	if tr.Period != (Period{invoice.MustDate("2026-09-01"), invoice.MustDate("2026-09-30")}) {
		t.Errorf("period = %v", tr.Period)
	}
	inv := tr.Invoices[0]
	if inv.ID != "F2026-0915" || inv.TypeCode != "380" || inv.BusinessProcess != "S1" || inv.ProfileID != "urn.cpro.gouv.fr:1p0:ereporting" {
		t.Errorf("invoice = %+v", inv)
	}
	if inv.Buyer == nil || inv.Buyer.IDScheme != "0223" || inv.Buyer.Country != "DE" || inv.Seller.TaxIDQualifier != "VAT" {
		t.Errorf("parties = %+v, %+v", inv.Seller, inv.Buyer)
	}
	if len(inv.AllowanceCharges) != 1 || inv.AllowanceCharges[0].Charge || inv.AllowanceCharges[0].Amount.String() != "50.00" {
		t.Errorf("allowances = %+v", inv.AllowanceCharges)
	}
	if inv.TaxExclusiveAmount.String() != "950.00" || inv.TaxAmount.String() != "0.00" || inv.TaxCurrency != "EUR" {
		t.Errorf("totals = %v %v %v", inv.TaxExclusiveAmount, inv.TaxAmount, inv.TaxCurrency)
	}
	if len(inv.TaxSubtotals) != 1 || inv.TaxSubtotals[0].ExemptionReasonCode != "VATEX-EU-AE" {
		t.Errorf("VAT breakdown = %+v", inv.TaxSubtotals)
	}
	l := inv.Lines[0]
	if l.Quantity.String() != "10" || l.UnitCode != "DAY" || l.Price == nil || l.Price.Gross.String() != "100.00" || l.ProductName != "Conseil" {
		t.Errorf("line = %+v", l)
	}
	cn := tr.Invoices[1]
	if len(cn.References) != 1 || cn.References[0].ID != "F2026-0915" || cn.Lines[0].Delivery.Location.City != "Berlin" ||
		!cn.Lines[0].AllowanceCharges[0].Charge {
		t.Errorf("credit note = %+v", cn)
	}
	tt := tr.Transactions[0]
	if tt.Category != "TLB1" || tt.Count == nil || *tt.Count != 42 || len(tt.TaxSubtotals) != 2 || tt.TaxSubtotals[1].Rate.String() != "5.5" {
		t.Errorf("transaction totals = %+v", tt)
	}
}

func TestParsePayments(t *testing.T) {
	rep, err := Parse(readTestdata(t, "payments.xml"))
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Payments
	if rep.Transactions != nil || p == nil || len(p.Invoices) != 1 || len(p.Transactions) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	ip := p.Invoices[0]
	if ip.ID != "F2026-0812" || ip.Payment.Date != invoice.MustDate("2026-09-10") || len(ip.Payment.SubTotals) != 2 {
		t.Errorf("invoice payment = %+v", ip)
	}
	if s := ip.Payment.SubTotals[1]; s.Currency != "" || s.Amount.String() != "110.00" || s.Rate.String() != "10" {
		t.Errorf("subtotal = %+v", s)
	}
	if p.Transactions[0].SubTotals[0].Amount.String() != "360.00" {
		t.Errorf("transactions = %+v", p.Transactions)
	}
}

// TestRoundTrip checks that Marshal writes the test reports, read by Parse,
// as they are, and that they are valid against the schema.
func TestRoundTrip(t *testing.T) {
	for _, name := range []string{"transactions.xml", "payments.xml"} {
		t.Run(name, func(t *testing.T) {
			data := readTestdata(t, name)
			if f := schemaFindings(t, data); len(f) > 0 {
				t.Fatalf("schema findings on the test report: %v", f)
			}
			rep, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := Marshal(rep, &MarshalOptions{Indent: true})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, data) {
				t.Errorf("Marshal differs from the report read:\n%s", out)
			}
			compact, err := Marshal(rep, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(compact, []byte(xmlDeclaration)) || bytes.ContainsRune(compact[len(xmlDeclaration):], '\n') {
				t.Errorf("Marshal without Indent writes newlines:\n%s", compact)
			}
			again, err := Parse(compact)
			if err != nil || !reflect.DeepEqual(again, rep) {
				t.Errorf("compact report read back differs (err %v)", err)
			}
		})
	}
}

// fullReport returns a report in which every field of the model is set.
func fullReport() *Report {
	d := invoice.MustDate
	dec := invoice.MustDecimal
	count := int64(3)
	return &Report{
		Document: Document{
			ID: "TR 1", Name: "Rapport & co", IssueDateTime: time.Date(2026, 9, 1, 23, 59, 58, 0, time.UTC), TypeCode: "RE",
			Sender: Party{ID: "0042", SchemeID: "0238", Name: "Plateforme", RoleCode: "WK", URI: "pdp@example.org"},
			Issuer: Party{ID: "123456782", SchemeID: "0002", Name: "Acheteur", RoleCode: "BY", URI: "achats@example.org"},
		},
		Transactions: &TransactionsReport{
			Period: Period{d("2026-08-01"), d("2026-08-31")},
			Invoices: []Invoice{{
				ID: "F-1", IssueDate: d("2026-08-02"), TypeCode: "384", Currency: "USD", DueDate: d("2026-09-02"), TaxDueDateTypeCode: "29",
				Notes:           []Note{{Code: "AAI", Text: "Note <1>"}, {Text: "sans sujet"}},
				BusinessProcess: "B1", ProfileID: "urn.cpro.gouv.fr:1p0:ereporting",
				References:        []Reference{{ID: "F-0", IssueDate: d("2026-07-01")}, {ID: "F-00"}},
				Seller:            Company{ID: "US123", IDScheme: "0227", TaxID: "US999", TaxIDQualifier: "VAT", Country: "US"},
				Buyer:             &Company{ID: "123456782", IDScheme: "0002", TaxID: "FR32123456782", TaxIDQualifier: "VAT", Country: "FR"},
				TaxRepresentative: &TaxRepresentative{TaxID: "FR11987654321", TaxIDScheme: "VA"},
				Deliveries: []Delivery{
					{Date: d("2026-08-03"), Location: &Address{Line1: "1 rue A", Line2: "Bât. B", Line3: "Porte C", City: "Paris", PostalCode: "75001", Subdivision: "IDF", Country: "FR"}},
					{Date: d("2026-08-04")},
				},
				Period: Period{Start: d("2026-08-01")},
				AllowanceCharges: []AllowanceCharge{
					{Charge: true, Amount: dec("10.00"), TaxCategory: "S", TaxRate: dec("20")},
					{Amount: dec("5")},
				},
				TaxExclusiveAmount: dec("105.00"), TaxAmount: dec("21.00"), TaxCurrency: "EUR",
				TaxSubtotals: []TaxSubtotal{
					{TaxableAmount: dec("105.00"), TaxAmount: dec("21.00"), Category: "S", Rate: dec("20")},
					{TaxableAmount: dec("0"), TaxAmount: dec("0"), Category: "E", Rate: dec("0"), ExemptionReason: "Exonération", ExemptionReasonCode: "VATEX-FR-CGI261"},
				},
				Lines: []Line{{
					Notes:    []Note{{Code: "AAA", Text: "ligne"}},
					Quantity: dec("2.5000"), UnitCode: "KGM",
					Reference:        &Reference{ID: "F-0", IssueDate: d("2026-07-01")},
					Delivery:         &LineDelivery{Name: "Dépôt", Location: &LineLocation{Line1: "2 rue D", City: "Lyon", PostalCode: "69001", Subdivision: "ARA", Country: "FR"}},
					Period:           Period{d("2026-08-01"), d("2026-08-15")},
					AllowanceCharges: []LineAllowanceCharge{{Charge: false, Amount: dec("1.00")}, {Charge: true, Amount: dec("2.00")}},
					Price:            &Price{Net: dec("40.000000"), Discount: dec("2"), Gross: dec("42")},
					ProductName:      "Article",
				}, {}},
			}},
			Transactions: []TransactionTotals{{
				Date: d("2026-08-05"), Currency: "EUR", TaxDueDateTypeCode: "72", Category: "TPS1",
				TaxExclusiveAmount: dec("100.00"), TaxTotal: dec("10.00"), Count: &count,
				TaxSubtotals: []RateTotal{{Rate: dec("10"), TaxableAmount: dec("100.00"), TaxTotal: dec("10.00")}},
			}},
		},
		Payments: &PaymentsReport{
			Period:       Period{d("2026-08-01"), d("2026-08-31")},
			Invoices:     []InvoicePayment{{ID: "F-1", IssueDate: d("2026-08-02"), Payment: Payment{Date: d("2026-08-20"), SubTotals: []PaymentSubtotal{{Rate: dec("20"), Currency: "EUR", Amount: dec("126.00")}}}}},
			Transactions: []Payment{{Date: d("2026-08-21"), SubTotals: []PaymentSubtotal{{Rate: dec("5.5"), Amount: dec("10")}, {Rate: dec("0"), Amount: dec("1")}}}},
		},
	}
}

// TestMarshalFull checks that every field of the model is written where the
// schema places it and read back.
func TestMarshalFull(t *testing.T) {
	rep := fullReport()
	for _, indent := range []bool{false, true} {
		out, err := Marshal(rep, &MarshalOptions{Indent: indent})
		if err != nil {
			t.Fatal(err)
		}
		if f := schemaFindings(t, out); len(f) > 0 {
			t.Errorf("schema findings: %v\n%s", f, out)
		}
		got, err := Parse(out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, rep) {
			t.Errorf("report read back differs:\n got %+v\nwant %+v\n%s", got, rep, out)
		}
	}
}

// TestMarshalRequired checks that the mandatory elements are written, empty,
// when the report has no value for them, and the optional ones are not.
func TestMarshalRequired(t *testing.T) {
	rep := &Report{Transactions: &TransactionsReport{
		Invoices:     []Invoice{{Lines: []Line{{Delivery: &LineDelivery{Location: &LineLocation{}}}}}},
		Transactions: []TransactionTotals{{TaxSubtotals: []RateTotal{{}}}},
	}}
	out, err := Marshal(rep, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"<Id></Id><IssueDateTime>", "<Id schemeId=\"\"></Id>", "<DateTimeString></DateTimeString>", "<StartDate></StartDate>",
		"<CompanyId schemeId=\"\"></CompanyId>", "<TaxAmount CurrencyCode=\"\"></TaxAmount>", "<CountryId></CountryId>",
		"<TaxPercent></TaxPercent>", "<CategoryCode></CategoryCode>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("%s not written:\n%s", want, s)
		}
	}
	for _, absent := range []string{"<Buyer>", "<DueDate>", "<InvoicePeriod>", "<TransactionsCount>", "<BilledQuantity", "<Product>", "<PaymentsReport>"} {
		if strings.Contains(s, absent) {
			t.Errorf("%s written:\n%s", absent, s)
		}
	}
	// The empty decimals are reported by the schema.
	var types int
	for _, f := range schemaFindings(t, out) {
		if f.Rule == schema.RuleType {
			types++
		}
	}
	if types == 0 {
		t.Error("no XSD-TYPE finding on the empty decimals")
	}
	if _, err := Marshal(nil, nil); err == nil {
		t.Error("Marshal(nil) succeeded")
	}
}

func TestParseIssues(t *testing.T) {
	data := readTestdata(t, "payments.xml")
	for _, tc := range []struct {
		name, old, new string
		path, msg      string
		undefined      bool
	}{
		{"date", "<Date>20260910</Date>", "<Date>2026-09-10</Date>", "/Report/PaymentsReport/Invoice/Payment/Date", `invalid date "2026-09-10": the format is AAAAMMJJ`, false},
		{"day", "<Date>20260910</Date>", "<Date>20260931</Date>", "/Report/PaymentsReport/Invoice/Payment/Date", "no such day", false},
		{"date and time", "20261011094500", "2026-10-11T09:45:00", "/Report/ReportDocument/IssueDateTime/DateTimeString", "the format is AAAAMMJJHHMMSS", false},
		{"decimal", "<Amount>110.00</Amount>", "<Amount>110,00</Amount>", "/Report/PaymentsReport/Invoice/Payment/SubTotals[2]/Amount", `invalid decimal "110,00"`, false},
		{"unknown element", "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><Extra/>", "/Report/ReportDocument/Extra", "Extra is not an element of ReportDocument", true},
		{"namespace", "<TypeCode>IN</TypeCode>", `<x:TypeCode xmlns:x="urn:x">IN</x:TypeCode>`, "/Report/ReportDocument/x:TypeCode", `in namespace "urn:x"`, true},
		{"repeated", "<TypeCode>IN</TypeCode>", "<TypeCode>IN</TypeCode><TypeCode>RE</TypeCode>", "/Report/ReportDocument/TypeCode[2]", "only the first is read", true},
		{"attribute", "<TypeCode>IN</TypeCode>", `<TypeCode lang="fr">IN</TypeCode>`, "/Report/ReportDocument/TypeCode", "attribute lang of TypeCode", true},
		{"prefixed attribute", "<TypeCode>IN</TypeCode>", `<TypeCode xmlns:x="urn:x" x:a="1">IN</TypeCode>`, "/Report/ReportDocument/TypeCode", "attribute x:a of TypeCode", true},
		{"text", "<ReportDocument>", "<ReportDocument>texte", "/Report/ReportDocument", "holds elements, not text", true},
		{"child of a leaf", "<TypeCode>IN</TypeCode>", "<TypeCode>IN<B/></TypeCode>", "/Report/ReportDocument/TypeCode/B", "element B in TypeCode", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := bytes.Replace(data, []byte(tc.old), []byte(tc.new), 1)
			rep, err := Parse(doc)
			var ie *IssuesError
			if rep == nil || !errors.As(err, &ie) {
				t.Fatalf("Parse = %v, %v; want a report and an *IssuesError", rep, err)
			}
			if len(ie.Issues) != 1 || ie.Issues[0].Path != tc.path || !strings.Contains(ie.Issues[0].Message, tc.msg) || ie.Issues[0].Undefined != tc.undefined {
				t.Errorf("issues = %+v, want one at %s mentioning %q (undefined %v)", ie.Issues, tc.path, tc.msg, tc.undefined)
			}
			if !strings.Contains(err.Error(), tc.path) || ie.Issues[0].Line == 0 {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestParseValueIssues(t *testing.T) {
	data := readTestdata(t, "transactions.xml")
	for _, tc := range []struct{ name, old, new, msg string }{
		{"integer", "<TransactionsCount>42</TransactionsCount>", "<TransactionsCount>4.2</TransactionsCount>", `invalid integer "4.2"`},
		{"charge", `ChargeIndicator="false"`, `ChargeIndicator="no"`, `invalid ChargeIndicator "no"`},
		{"empty charge", `ChargeIndicator="true"`, `ChargeIndicator=""`, `invalid ChargeIndicator ""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Parse(bytes.Replace(data, []byte(tc.old), []byte(tc.new), 1))
			if rep == nil || err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("Parse = %v, %v; want an issue mentioning %q", rep, err, tc.msg)
			}
		})
	}
	// Two issues: the error gives their number.
	doc := bytes.Replace(data, []byte("<DueDate>20261015</DueDate>"), []byte("<DueDate>x</DueDate><Extra/>"), 1)
	if _, err := Parse(doc); err == nil || !strings.Contains(err.Error(), "2 unread parts") {
		t.Errorf("error = %v", err)
	}
	// Signs and the xsi attributes are accepted.
	doc = bytes.Replace(data, []byte("<TransactionsCount>42</TransactionsCount>"), []byte("<TransactionsCount>+42</TransactionsCount>"), 1)
	doc = bytes.Replace(doc, []byte("<Report>"), []byte(`<Report xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="ereporting.xsd">`), 1)
	doc = bytes.Replace(doc, []byte(`ChargeIndicator="true"`), []byte(`ChargeIndicator="1"`), 1)
	doc = bytes.Replace(doc, []byte(`ChargeIndicator="false"`), []byte(`ChargeIndicator="0"`), 1)
	rep, err := Parse(doc)
	if err != nil || *rep.Transactions.Transactions[0].Count != 42 || !rep.Transactions.Invoices[1].Lines[0].AllowanceCharges[0].Charge {
		t.Errorf("Parse = %v", err)
	}
}

func TestParseNotReport(t *testing.T) {
	for _, tc := range []struct{ doc, msg string }{
		{`<Invoice/>`, "the root element is Invoice"},
		{`<Report xmlns="urn:x"/>`, `in namespace "urn:x"`},
	} {
		rep, err := Parse([]byte(tc.doc))
		if rep != nil || !errors.Is(err, ErrNotReport) || !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("Parse(%s) = %v, %v", tc.doc, rep, err)
		}
		root, _ := xmltree.Parse([]byte(tc.doc))
		if IsReport(root) {
			t.Errorf("IsReport(%s)", tc.doc)
		}
		if rep, issues := ParseTree(root); rep != nil || len(issues) != 1 || issues[0].Path == "" {
			t.Errorf("ParseTree(%s) = %v, %v", tc.doc, rep, issues)
		}
	}
	if _, err := Parse([]byte("<Report>")); err == nil || errors.Is(err, ErrNotReport) {
		t.Errorf("Parse of a truncated document: %v", err)
	}
	if rep, issues := ParseTree(nil); rep != nil || len(issues) != 1 || IsReport(nil) {
		t.Errorf("ParseTree(nil) = %v, %v", rep, issues)
	}
}

// FuzzParse checks that Parse never panics, and that a report read is
// written by Marshal as a document that Parse reads as the same report.
func FuzzParse(f *testing.F) {
	f.Add(readTestdata(f, "transactions.xml"))
	f.Add(readTestdata(f, "payments.xml"))
	f.Add([]byte("<Report><ReportDocument><Id> a </Id><TypeCode/></ReportDocument><TransactionsReport><Invoice><Buyer/><Line><Delivery><Location/></Delivery></Line></Invoice></TransactionsReport></Report>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		rep, _ := Parse(data)
		if rep == nil {
			return
		}
		out, err := Marshal(rep, nil)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Parse(out)
		if err != nil {
			t.Fatalf("report written does not read back: %v\n%s", err, out)
		}
		if !reflect.DeepEqual(again, rep) {
			t.Fatalf("report read back differs:\n got %+v\nwant %+v\n%s", again, rep, out)
		}
	})
}
