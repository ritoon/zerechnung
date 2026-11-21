package validate

import (
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	ublsyntax "github.com/ritoon/zerechnung/ubl"
)

const (
	ciusitInvoice    = "../internal/rules/it/testdata/ciusit.xml"     // domestic CIUS-IT, seller in Italy
	ciusitUBLInvoice = "../internal/rules/it/testdata/ciusit.ubl.xml" // ciusitInvoice in UBL 2.1
)

// TestCIUSIT checks the CIUS-IT invoices of internal/rules/it with every
// rule set that applies, and when the Italian rules apply: on a CIUS-IT
// invoice (BT-24) whatever the rule sets, or on request, never by default
// nor with Auto.
func TestCIUSIT(t *testing.T) {
	for file, ipa := range map[string]string{
		ciusitInvoice:    `<ram:URIID schemeID="0201">UFABCD</ram:URIID>`,
		ciusitUBLInvoice: `<cbc:EndpointID schemeID="0201">UFABCD</cbc:EndpointID>`,
	} {
		doc := readXML(t, file)
		r := XML([]byte(doc), &Options{Auto: true})
		if r.Profile != invoice.ProfileCIUSIT || len(r.Findings) > 0 {
			t.Errorf("%s: profile %s\n%s", file, r.Profile, r)
		}
		// An IPA code in lower case breaks BR-IT-200 whatever the rule
		// sets; the finding is located in the document validated.
		if !strings.Contains(doc, ipa) {
			t.Fatalf("%s: BT-49 not found", file)
		}
		lower := []byte(strings.Replace(doc, "UFABCD", "ufabcd", 1))
		for _, opts := range []*Options{
			nil,
			{Auto: true},
			{Rules: Default | France},
			{Rules: Schema | EN16931},
		} {
			r := XML(lower, opts)
			f := findings(r, "BR-IT-")
			if len(f) != 1 || f[0].Rule != "BR-IT-200" || f[0].Source != SourceIT || f[0].Severity != Error ||
				!strings.HasSuffix(f[0].Path, "/ram:URIID") && !strings.HasSuffix(f[0].Path, "/cbc:EndpointID") {
				t.Errorf("%s with BT-49 in lower case (%+v):\n%s", file, opts, r)
			}
		}
	}

	// The Italian rules apply to other invoices on request only.
	french := []byte(readXML(t, frenchInvoice))
	for _, opts := range []*Options{nil, {Auto: true}} {
		if r := XML(french, opts); findings(r, "BR-IT-") != nil || findings(r, "IT-P") != nil || findings(r, "IT-CF-") != nil {
			t.Errorf("Italian rules applied to a Factur-X invoice (%+v):\n%s", opts, r)
		}
	}
	r := XML(french, &Options{Rules: Default | Italy})
	if findings(r, "BR-IT-") == nil {
		t.Errorf("Rules: Default | Italy on a Factur-X invoice:\n%s", r)
	}
}

func TestRulesForCIUSIT(t *testing.T) {
	ciusit := readXML(t, ciusitInvoice)
	german := readXML(t, germanInvoice)
	for _, c := range []struct {
		name, doc string
		opts      Options
		want      RuleSet
	}{
		{"CIUS-IT", ciusit, Options{}, Default | Italy},
		{"CIUS-IT with Auto", ciusit, Options{Auto: true}, Default | Italy},
		{"CIUS-IT explicit rules", ciusit, Options{Rules: Schema | France}, Schema | France | Italy},
		{"CIUS-IT cross-border", strings.Replace(ciusit, invoice.URNCIUSIT+invoice.ItalianExtension, invoice.URNCIUSIT, 1), Options{}, Default | Italy},
		{"CIUS-IT profile option", german, Options{Profile: invoice.ProfileCIUSIT}, Default | Italy},
		{"Italian seller without CIUS-IT", strings.Replace(german, "<ram:CountryID>DE</ram:CountryID>", "<ram:CountryID>IT</ram:CountryID>", 1), Options{Auto: true}, Default},
	} {
		root, err := xmltree.Parse([]byte(c.doc))
		if err != nil {
			t.Fatal(err)
		}
		if got := RulesFor(rules.NewDoc(root, c.opts.Profile), &c.opts); got != c.want {
			t.Errorf("%s: RulesFor = %b, want %b", c.name, got, c.want)
		}
	}
}

func TestCatalogIT(t *testing.T) {
	ids := map[string]int{}
	for _, r := range Catalog(Italy) {
		ids[r.ID]++
	}
	for _, id := range []string{"BR-IT-010", "BR-IT-DC-510", "BR-IT-530", "IT-PROFILE-01", "IT-PIVA-02", "IT-CF-02"} {
		if ids[id] != 1 {
			t.Errorf("Catalog(Italy) lists %s %d times", id, ids[id])
		}
	}
	if len(ids) != 64 {
		t.Errorf("Catalog(Italy): %d rules, want 64", len(ids))
	}
	// The Italian national rules of Peppol (IT-R-*) are Peppol rules.
	for _, r := range Catalog(Default | Peppol | Netherlands) {
		for _, prefix := range []string{"BR-IT-", "IT-PROFILE-", "IT-PIVA-", "IT-CF-"} {
			if strings.HasPrefix(r.ID, prefix) {
				t.Errorf("%s outside Catalog(Italy)", r.ID)
			}
		}
	}
}

// TestCIUSITRoundTrip reads the CIUS-IT invoices and writes them back in
// both syntaxes: the results are still valid domestic CIUS-IT invoices,
// whose specification identifier carries the Italian extension.
func TestCIUSITRoundTrip(t *testing.T) {
	for _, file := range []string{ciusitInvoice, ciusitUBLInvoice} {
		data := []byte(readXML(t, file))
		parse := cii.Parse
		if strings.HasSuffix(file, ".ubl.xml") {
			parse = ublsyntax.Parse
		}
		inv, err := parse(data)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if inv.Profile != invoice.ProfileCIUSIT {
			t.Fatalf("%s: profile %s", file, inv.Profile)
		}
		ciiData, err := cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		ublData, err := ublsyntax.Marshal(inv, &ublsyntax.MarshalOptions{Strict: true})
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, out := range [][]byte{ciiData, ublData} {
			if r := XML(out, nil); r.Profile != invoice.ProfileCIUSIT || len(r.Findings) > 0 ||
				!strings.Contains(string(out), invoice.URNCIUSIT+invoice.ItalianExtension+"<") {
				t.Errorf("%s: profile %s\n%s\n%s", file, r.Profile, r, out)
			}
		}
	}
}

// TestCIUSITSplitPayment builds a domestic CIUS-IT invoice in split payment
// (VAT category B) addressed to an Italian public administration, lets
// Calculate fill in its amounts and writes it in both syntaxes: the results
// pass every rule, BR-CO-17 and BR-IT-DC-510 included.
func TestCIUSITSplitPayment(t *testing.T) {
	d := invoice.MustDecimal
	address := func(street, number, postCode string) invoice.Address {
		return invoice.Address{Line1: street, Line2: number, City: "Roma", PostCode: postCode, CountrySubdivision: "RM", CountryCode: "IT"}
	}
	inv := &invoice.Invoice{
		Profile:   invoice.ProfileCIUSIT,
		Number:    "FT-2026/0057",
		TypeCode:  "380",
		IssueDate: invoice.NewDate(2026, time.November, 20),
		Currency:  "EUR",
		Seller: invoice.Party{
			Name:              "Esempio Reti S.r.l.",
			LegalRegistration: invoice.Identifier{ID: "12345678903", Scheme: "0210"},
			Address:           address("Via Nazionale", "12", "00184"),
			ElectronicAddress: invoice.Identifier{ID: "12345678903", Scheme: "0211"},
			VATID:             "IT12345678903",
		},
		Buyer: invoice.Party{
			Name:              "Comune di Esempio",
			LegalRegistration: invoice.Identifier{ID: "80012345676", Scheme: "0210"},
			Address:           address("Piazza del Municipio", "1", "00100"),
			ElectronicAddress: invoice.Identifier{ID: "UFABCD", Scheme: "0201"},
		},
		PaymentMeans: []invoice.PaymentMeans{{TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IT60X0542811101000000123456"}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)",
			DueDate:     invoice.NewDate(2026, time.December, 20),
		}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Manutenzione della rete"},
			Quantity:        d("40"),
			UnitCode:        "HUR",
			NetPrice:        d("80.00"),
			VATCategoryCode: invoice.VATSplitPayment,
			VATRate:         d("22"),
		}},
	}
	inv.Calculate()
	if tt := inv.Totals; tt.TaxTotal.String() != "704.00" || tt.GrandTotal.String() != "3904.00" || tt.DuePayable.String() != "3904.00" {
		t.Fatalf("totals: %+v", tt)
	}
	ciiData, err := cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	ublData, err := ublsyntax.Marshal(inv, &ublsyntax.MarshalOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range [][]byte{ciiData, ublData} {
		if r := XML(out, nil); r.Profile != invoice.ProfileCIUSIT || len(r.Findings) > 0 {
			t.Errorf("profile %s\n%s\n%s", r.Profile, r, out)
		}
	}

	// Without the mention of split payment in the payment terms, the
	// invoice breaks BR-IT-DC-510.
	inv.PaymentTerms[0].Description = "Bonifico a 30 giorni data fattura"
	ciiData, err = cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	r := XML(ciiData, nil)
	if f := findings(r, "BR-IT-DC-510"); len(f) != 1 || len(r.Findings) != 1 {
		t.Errorf("without the mention of split payment:\n%s", r)
	}
}
