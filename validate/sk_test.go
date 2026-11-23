package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The Slovak invoices of internal/rules/sk: a Peppol BIS Billing 3.0
// invoice in UBL between two Slovak VAT payers, and the CII invoice that
// cii.Marshal writes from it.
var slovakInvoices = []string{
	"../internal/rules/sk/testdata/sk.ubl.xml",
	"../internal/rules/sk/testdata/sk.cii.xml",
}

// TestSlovakInvoices checks the Slovak invoices with every rule set that
// applies to them: they are Peppol invoices, and pass the Slovak rules
// too, which Auto adds for a seller in Slovakia.
func TestSlovakInvoices(t *testing.T) {
	for _, file := range slovakInvoices {
		doc := []byte(readXML(t, file))
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Default | Slovakia}, {Rules: Default | Slovakia | Peppol}} {
			r := XML(doc, opts)
			if r.Profile != invoice.ProfilePeppol || len(r.Findings) > 0 {
				t.Errorf("%s (%+v): profile %s\n%s", file, opts, r.Profile, r)
			}
		}
	}
}

// TestSlovakiaRuleSet checks when the Slovak rules apply: with Auto for a
// seller in Slovakia (BT-40), or when the Slovakia rule set is selected,
// but not because the invoice is a Peppol one, as BT-24 does not tell a
// Slovak invoice from another.
func TestSlovakiaRuleSet(t *testing.T) {
	const legalForm = "<cbc:CompanyLegalForm>Spoločnosť s ručením obmedzeným zapísaná v Obchodnom registri Mestského súdu Bratislava III, oddiel Sro, vložka č. 12345/B</cbc:CompanyLegalForm>\n"
	doc := readXML(t, slovakInvoices[0])
	if !strings.Contains(doc, legalForm) {
		t.Fatal("no BT-33")
	}
	noLegalForm := []byte(strings.Replace(doc, legalForm, "", 1))
	french := []byte(readXML(t, frenchInvoice))
	for _, c := range []struct {
		name string
		doc  []byte
		opts *Options
		want []string
	}{
		{"Slovak seller", noLegalForm, nil, nil},
		{"Slovak seller, Peppol", noLegalForm, &Options{Rules: Default | Peppol}, nil},
		{"Slovak seller, Auto", noLegalForm, &Options{Auto: true}, []string{"SK-13"}},
		{"Slovak seller, Slovakia", noLegalForm, &Options{Rules: Default | Slovakia}, []string{"SK-13"}},
		{"Slovak seller, schema only", noLegalForm, &Options{Rules: Schema | Slovakia}, []string{"SK-13"}},
		{"French seller, Auto", french, &Options{Auto: true}, nil},
		// The rules that do not depend on the country of a party apply
		// when the set is selected: the French invoice gives a delivery
		// date three days before its issue date, and no VAT point date.
		{"French seller, Slovakia", french, &Options{Rules: Default | Slovakia}, []string{"SK-14"}},
	} {
		r := XML(c.doc, c.opts)
		var got []string
		for _, f := range r.Findings {
			if f.Source == SourceSK {
				got = append(got, f.Rule)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Slovak findings %v, want %v\n%s", c.name, got, c.want, r)
		}
	}
}

// TestCatalogSlovakia checks that the Slovakia rule set lists the 15 Slovak
// rules, and that no other rule set lists them.
func TestCatalogSlovakia(t *testing.T) {
	var ids []string
	for _, r := range Catalog(Slovakia) {
		ids = append(ids, r.ID)
		if !strings.HasPrefix(r.ID, "SK-") {
			t.Errorf("Catalog(Slovakia) lists %s", r.ID)
		}
	}
	if len(ids) != 15 || ids[0] != "SK-01" || ids[14] != "SK-15" {
		t.Errorf("Catalog(Slovakia) = %v", ids)
	}
	for _, r := range Catalog(^RuleSet(0) &^ Slovakia) {
		if strings.HasPrefix(r.ID, "SK-") {
			t.Errorf("%s is listed outside the Slovakia rule set", r.ID)
		}
	}
}
