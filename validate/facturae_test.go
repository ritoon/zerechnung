package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

const facturaeInvoice = "../facturae/testdata/b2g.xml" // Facturae 3.2.2, B2G invoice sent through FACe

// TestFacturae checks a valid Facturae invoice: whatever the rule sets,
// only the warning FE-02 is reported.
func TestFacturae(t *testing.T) {
	doc := []byte(readXML(t, facturaeInvoice))
	for _, opts := range []*Options{
		nil,
		{Rules: Schema},
		{Rules: Default | France | Germany | GermanyProfile | ChorusPro | Peppol | Netherlands | Italy},
		{Rules: EN16931},
		{Auto: true},
	} {
		r := XML(doc, opts)
		if got := ruleIDs(r); !slices.Equal(got, []string{ruleFEScope}) {
			t.Errorf("%+v: findings = %v", opts, r.Findings)
			continue
		}
		f := r.Findings[0]
		if f.Severity != Warning || f.Source != SourceSyntax || f.Path != "/fe:Facturae" || f.Line != 2 ||
			!strings.Contains(f.Message, "Facturae 3.2.2 is not a syntax of EN 16931") {
			t.Errorf("%+v: FE-02 = %+v", opts, f)
		}
		if !r.Valid() || r.Profile != invoice.ProfileUnknown {
			t.Errorf("%+v: valid = %v, profile = %v", opts, r.Valid(), r.Profile)
		}
	}
}

// TestFacturaeFindings checks the schema and FE-01 on altered copies of a
// valid Facturae invoice. Without the Schema rule set, only FE-02 is left.
func TestFacturaeFindings(t *testing.T) {
	const invoices = "/fe:Facturae/Invoices/Invoice"
	for _, c := range []struct {
		name, old, new string
		rules          []string
		path           string
	}{
		{"value outside its list", "<InvoiceClass>OO</InvoiceClass>", "<InvoiceClass>XX</InvoiceClass>",
			[]string{"XSD-TYPE", ruleFEScope}, invoices + "/InvoiceHeader/InvoiceClass"},
		{"missing element", "<InvoiceDocumentType>FC</InvoiceDocumentType>", "",
			[]string{"XSD-MISSING", ruleFEScope}, invoices + "/InvoiceHeader"},
		{"unreadable date", "<IssueDate>2026-11-02</IssueDate>", "<IssueDate>02/11/2026</IssueDate>",
			[]string{"XSD-TYPE", ruleFEValue, ruleFEScope}, invoices + "/InvoiceIssueData/IssueDate"},
		{"unreadable country", "<CountryCode>ESP</CountryCode>", "<CountryCode>XYZ</CountryCode>",
			[]string{"XSD-TYPE", ruleFEValue, ruleFEScope}, "/fe:Facturae/Parties/SellerParty/LegalEntity/AddressInSpain/CountryCode"},
		{"two alternatives", "<IBAN>ES9121000418450200051332</IBAN>", "<IBAN>ES9121000418450200051332</IBAN><AccountNumber>00491500051234567892</AccountNumber>",
			[]string{"XSD-UNEXPECTED", ruleFEScope}, invoices + "/PaymentDetails/Installment/AccountToBeCredited/AccountNumber"},
		{"no alternative", "<IBAN>ES9121000418450200051332</IBAN>", "",
			[]string{"XSD-MISSING", ruleFEScope}, invoices + "/PaymentDetails/Installment/AccountToBeCredited"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := readXML(t, facturaeInvoice)
			if !strings.Contains(doc, c.old) {
				t.Fatalf("%q not found", c.old)
			}
			doc = strings.Replace(doc, c.old, c.new, 1)
			r := XML([]byte(doc), nil)
			if got := ruleIDs(r); !slices.Equal(got, c.rules) {
				t.Fatalf("findings = %v", r.Findings)
			}
			if f := r.Findings[0]; f.Path != c.path || f.Severity != Error || f.Line == 0 {
				t.Errorf("first finding = %+v, want the path %s", f, c.path)
			}
			if r.Valid() {
				t.Error("valid")
			}
			if r := XML([]byte(doc), &Options{Rules: EN16931 | France}); !slices.Equal(ruleIDs(r), []string{ruleFEScope}) {
				t.Errorf("without Schema: %v", r.Findings)
			}
		})
	}
}

func TestCatalogFacturae(t *testing.T) {
	ids := func(c []RuleInfo) []string {
		var out []string
		for _, r := range c {
			if strings.HasPrefix(r.ID, "FE-") {
				out = append(out, r.ID)
			}
		}
		return out
	}
	if got := ids(Catalog(Schema)); !slices.Equal(got, []string{ruleFEValue, ruleFEScope}) {
		t.Errorf("Catalog(Schema) FE rules = %v", got)
	}
	if got := ids(Catalog(EN16931 | FacturX | France | Germany | Peppol | Netherlands)); got != nil {
		t.Errorf("Catalog without Schema lists %v", got)
	}
}
