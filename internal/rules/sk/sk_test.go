package sk

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const (
	ublInvoice = "testdata/sk.ubl.xml"
	ciiInvoice = "testdata/sk.cii.xml"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// check runs Check on an invoice, UBL or CII.
func check(t *testing.T, data string) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	var doc *rules.Doc
	if ubl.IsUBL(root) {
		doc = ubl.View(root, invoice.ProfileUnknown)
	} else {
		doc = rules.NewDoc(root, invoice.ProfileUnknown)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceSK})
	return r
}

// edit applies the replacements of pairs (old, new, old, new…) to doc,
// each old string being required.
func edit(t *testing.T, doc string, pairs ...string) string {
	t.Helper()
	for i := 0; i < len(pairs); i += 2 {
		if !strings.Contains(doc, pairs[i]) {
			t.Fatalf("%q not found", pairs[i])
		}
		doc = strings.Replace(doc, pairs[i], pairs[i+1], 1)
	}
	return doc
}

// TestValidInvoices checks that the Slovak invoices of testdata pass the
// rules.
func TestValidInvoices(t *testing.T) {
	for _, f := range []string{ublInvoice, ciiInvoice} {
		if r := check(t, read(t, f)); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", f, r)
		}
	}
}

type finding struct {
	rule     string
	severity rules.Severity
}

// TestFindings breaks one requirement at a time, in UBL and in CII, and
// checks the findings.
func TestFindings(t *testing.T) {
	const (
		sellerVAT  = "<cac:PartyTaxScheme>\n        <cbc:CompanyID>SK2020123456</cbc:CompanyID>\n        <cac:TaxScheme>\n          <cbc:ID>VAT</cbc:ID>\n        </cac:TaxScheme>\n      </cac:PartyTaxScheme>\n"
		buyerVAT   = "<cac:PartyTaxScheme>\n        <cbc:CompanyID>SK2021987654</cbc:CompanyID>\n        <cac:TaxScheme>\n          <cbc:ID>VAT</cbc:ID>\n        </cac:TaxScheme>\n      </cac:PartyTaxScheme>\n"
		legalForm  = "<cbc:CompanyLegalForm>Spoločnosť s ručením obmedzeným zapísaná v Obchodnom registri Mestského súdu Bratislava III, oddiel Sro, vložka č. 12345/B</cbc:CompanyLegalForm>\n"
		ciiForm    = "<ram:Description>Spoločnosť s ručením obmedzeným zapísaná v Obchodnom registri Mestského súdu Bratislava III, oddiel Sro, vložka č. 12345/B</ram:Description>"
		ciiSeller  = `<ram:ID schemeID="VA">SK2020123456</ram:ID>`
		ciiBuyer   = `<ram:ID schemeID="VA">SK2021987654</ram:ID>`
		taxPoint   = "<cbc:TaxPointDate>2027-01-12</cbc:TaxPointDate>\n"
		ciiPoint   = `<ram:TaxPointDate>`
		catS       = "<cbc:ID>S</cbc:ID>\n        <cbc:Percent>23</cbc:Percent>"
		ciiCatS    = "<ram:CategoryCode>S</ram:CategoryCode>"
		sellerName = "<cbc:RegistrationName>Príklad Softvér s.r.o.</cbc:RegistrationName>"
		buyerName  = "<cbc:RegistrationName>Odberateľ Košice a.s.</cbc:RegistrationName>"
	)
	err := func(rule string) finding { return finding{rule, rules.Error} }
	warn := func(rule string) finding { return finding{rule, rules.Warning} }
	for _, c := range []struct {
		name  string
		file  string
		pairs []string
		want  []finding
	}{
		// SK-01 to SK-03: postal addresses.
		{"seller street", ublInvoice, []string{"<cbc:StreetName>Hlavná 1</cbc:StreetName>\n", ""}, []finding{err(RuleSellerAddress)}},
		{"seller city and post code", ciiInvoice, []string{"<ram:PostcodeCode>811 01</ram:PostcodeCode>", "", "<ram:CityName>Bratislava</ram:CityName>", ""}, []finding{err(RuleSellerAddress)}},
		{"buyer post code", ublInvoice, []string{"<cbc:PostalZone>040 01</cbc:PostalZone>\n", ""}, []finding{err(RuleBuyerAddress)}},
		{"buyer city", ciiInvoice, []string{"<ram:CityName>Košice</ram:CityName>", ""}, []finding{err(RuleBuyerAddress)}},
		{"tax representative without address lines", ublInvoice, []string{"<cac:Delivery>", "<cac:TaxRepresentativeParty>\n    <cac:PartyName><cbc:Name>Zástupca</cbc:Name></cac:PartyName>\n    <cac:PostalAddress><cac:Country><cbc:IdentificationCode>SK</cbc:IdentificationCode></cac:Country></cac:PostalAddress>\n    <cac:PartyTaxScheme><cbc:CompanyID>SK2022111116</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>\n  </cac:TaxRepresentativeParty>\n  <cac:Delivery>"}, []finding{err(RuleTaxRepAddress)}},

		// SK-04 to SK-06: electronic addresses of scheme 0245.
		{"seller endpoint 9950", ublInvoice, []string{`<cbc:EndpointID schemeID="0245">2020123456</cbc:EndpointID>`, `<cbc:EndpointID schemeID="9950">SK2020123456</cbc:EndpointID>`}, []finding{err(RuleSellerEndpoint)}},
		{"seller endpoint missing", ciiInvoice, []string{`<ram:URIUniversalCommunication>
          <ram:URIID schemeID="0245">2020123456</ram:URIID>
        </ram:URIUniversalCommunication>`, ""}, []finding{err(RuleSellerEndpoint)}},
		{"buyer endpoint GLN", ublInvoice, []string{`<cbc:EndpointID schemeID="0245">2021987654</cbc:EndpointID>`, `<cbc:EndpointID schemeID="0088">5790000435968</cbc:EndpointID>`}, []finding{err(RuleBuyerEndpoint)}},
		{"buyer abroad", ublInvoice, []string{`<cbc:EndpointID schemeID="0245">2021987654</cbc:EndpointID>`, `<cbc:EndpointID schemeID="9930">DE123456789</cbc:EndpointID>`, "<cbc:IdentificationCode>SK</cbc:IdentificationCode>\n        </cac:Country>\n      </cac:PostalAddress>\n      <cac:PartyTaxScheme>\n        <cbc:CompanyID>SK2021987654", "<cbc:IdentificationCode>DE</cbc:IdentificationCode>\n        </cac:Country>\n      </cac:PostalAddress>\n      <cac:PartyTaxScheme>\n        <cbc:CompanyID>DE123456789"}, nil},
		{"DIČ of nine digits", ublInvoice, []string{`<cbc:EndpointID schemeID="0245">2020123456</cbc:EndpointID>`, `<cbc:EndpointID schemeID="0245">202012345</cbc:EndpointID>`}, []finding{err(RuleDIC)}},
		{"DIČ with the prefix SK", ciiInvoice, []string{`<ram:URIID schemeID="0245">2021987654</ram:URIID>`, `<ram:URIID schemeID="0245">SK2021987654</ram:URIID>`}, []finding{err(RuleDIC)}},
		{"buyer identifier of scheme 0245", ublInvoice, []string{"<cac:AccountingCustomerParty>\n    <cac:Party>\n      <cbc:EndpointID schemeID=\"0245\">2021987654</cbc:EndpointID>\n", "<cac:AccountingCustomerParty>\n    <cac:Party>\n      <cbc:EndpointID schemeID=\"0245\">2021987654</cbc:EndpointID>\n      <cac:PartyIdentification><cbc:ID schemeID=\"0245\">20219876</cbc:ID></cac:PartyIdentification>\n"}, []finding{err(RuleDIC)}},

		// SK-07 to SK-09: VAT identifiers.
		{"seller VAT identifier, category S", ublInvoice, []string{sellerVAT, ""}, []finding{err(RuleSellerVATID)}},
		{"seller VAT identifier, tax representative", ublInvoice, []string{sellerVAT, "", "<cac:Delivery>", "<cac:TaxRepresentativeParty>\n    <cac:PartyName><cbc:Name>Zástupca</cbc:Name></cac:PartyName>\n    <cac:PostalAddress><cbc:StreetName>Mostová 2</cbc:StreetName><cbc:CityName>Bratislava</cbc:CityName><cbc:PostalZone>811 02</cbc:PostalZone><cac:Country><cbc:IdentificationCode>SK</cbc:IdentificationCode></cac:Country></cac:PostalAddress>\n    <cac:PartyTaxScheme><cbc:CompanyID>SK2022111116</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>\n  </cac:TaxRepresentativeParty>\n  <cac:Delivery>"}, nil},
		{"seller VAT identifier, category E", ciiInvoice, []string{ciiSeller, "", ciiCatS, "<ram:CategoryCode>E</ram:CategoryCode>", ciiCatS, "<ram:CategoryCode>E</ram:CategoryCode>", ciiCatS, "<ram:CategoryCode>E</ram:CategoryCode>"}, []finding{warn(RuleSellerVATID)}},
		{"seller VAT identifier, category O", ciiInvoice, []string{ciiSeller, "", ciiBuyer, "", ciiCatS, "<ram:CategoryCode>O</ram:CategoryCode>", ciiCatS, "<ram:CategoryCode>O</ram:CategoryCode>", ciiCatS, "<ram:CategoryCode>O</ram:CategoryCode>"}, nil},
		{"buyer VAT identifier, category S", ublInvoice, []string{buyerVAT, ""}, []finding{warn(RuleBuyerVATID)}},
		{"buyer VAT identifier, reverse charge", ublInvoice, []string{buyerVAT, "", catS, "<cbc:ID>AE</cbc:ID>\n        <cbc:Percent>0</cbc:Percent>"}, []finding{err(RuleBuyerVATID)}},
		{"buyer VAT identifier, reverse charge in CII", ciiInvoice, []string{ciiBuyer, "", ciiCatS, "<ram:CategoryCode>AE</ram:CategoryCode>"}, []finding{err(RuleBuyerVATID)}},
		{"IČ DPH of nine digits", ublInvoice, []string{"<cbc:CompanyID>SK2020123456</cbc:CompanyID>", "<cbc:CompanyID>SK202012345</cbc:CompanyID>"}, []finding{err(RuleICDPH)}},
		{"IČ DPH with a space", ciiInvoice, []string{ciiBuyer, `<ram:ID schemeID="VA">SK 2021987654</ram:ID>`}, []finding{err(RuleICDPH)}},
		{"foreign VAT identifier", ciiInvoice, []string{ciiBuyer, `<ram:ID schemeID="VA">CZ12345678</ram:ID>`}, nil},

		// SK-10 to SK-13: IČO and legal information.
		{"seller IČO, s.r.o.", ublInvoice, []string{"<cbc:CompanyID>12345678</cbc:CompanyID>\n", ""}, []finding{err(RuleSellerICO)}},
		{"seller IČO, sole trader", ublInvoice, []string{"<cbc:CompanyID>12345678</cbc:CompanyID>\n", "", sellerName, "<cbc:RegistrationName>Ján Príklad</cbc:RegistrationName>"}, []finding{warn(RuleSellerICO)}},
		{"buyer IČO, a.s.", ciiInvoice, []string{"<ram:ID>87654321</ram:ID>", ""}, []finding{err(RuleBuyerICO)}},
		{"buyer IČO, public body", ublInvoice, []string{"<cbc:CompanyID>87654321</cbc:CompanyID>\n", "", buyerName, "<cbc:RegistrationName>Mesto Košice</cbc:RegistrationName>"}, []finding{warn(RuleBuyerICO)}},
		{"IČO of seven digits", ublInvoice, []string{"<cbc:CompanyID>12345678</cbc:CompanyID>", "<cbc:CompanyID>1234567</cbc:CompanyID>"}, []finding{err(RuleICO)}},
		{"register number as IČO", ciiInvoice, []string{"<ram:ID>87654321</ram:ID>", "<ram:ID>Sa 1234/V</ram:ID>"}, []finding{err(RuleICO)}},
		{"legal registration of scheme 0245", ublInvoice, []string{"<cbc:CompanyID>12345678</cbc:CompanyID>", `<cbc:CompanyID schemeID="0245">2020123456</cbc:CompanyID>`}, nil},
		{"legal information, s.r.o.", ublInvoice, []string{legalForm, ""}, []finding{err(RuleSellerLegalInfo)}},
		{"legal information, spol. s r. o.", ciiInvoice, []string{ciiForm, "", "<ram:Name>Príklad Softvér s.r.o.</ram:Name>", "<ram:Name>Príklad Softvér, spol. s r. o.</ram:Name>"}, []finding{err(RuleSellerLegalInfo)}},
		{"legal information, sole trader", ublInvoice, []string{legalForm, "", sellerName, "<cbc:RegistrationName>Ján Príklad</cbc:RegistrationName>"}, []finding{warn(RuleSellerLegalInfo)}},

		// SK-14: date of the taxable supply.
		{"VAT point date", ublInvoice, []string{taxPoint, ""}, []finding{err(RuleTaxPointDate)}},
		{"VAT point date in CII", ciiInvoice, []string{ciiPoint + `
          <udt:DateString format="102">20270112</udt:DateString>
        </ram:TaxPointDate>`, ""}, []finding{err(RuleTaxPointDate)}},
		{"VAT point date code", ublInvoice, []string{taxPoint, "", "<cac:AccountingSupplierParty>", "<cac:InvoicePeriod>\n    <cbc:DescriptionCode>35</cbc:DescriptionCode>\n  </cac:InvoicePeriod>\n  <cac:AccountingSupplierParty>"}, nil},
		{"delivery on the issue date", ublInvoice, []string{taxPoint, "", "<cbc:ActualDeliveryDate>2027-01-12</cbc:ActualDeliveryDate>", "<cbc:ActualDeliveryDate>2027-01-15</cbc:ActualDeliveryDate>"}, nil},
		{"no delivery date", ciiInvoice, []string{ciiPoint + `
          <udt:DateString format="102">20270112</udt:DateString>
        </ram:TaxPointDate>`, "", `<ram:ActualDeliverySupplyChainEvent>
        <ram:OccurrenceDateTime>
          <udt:DateTimeString format="102">20270112</udt:DateTimeString>
        </ram:OccurrenceDateTime>
      </ram:ActualDeliverySupplyChainEvent>`, ""}, nil},

		// SK-15: corrective invoice.
		{"corrective invoice", ublInvoice, []string{"<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>"}, []finding{err(RuleCorrection)}},
		{"corrective invoice with its reference", ublInvoice, []string{"<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>", "<cac:AccountingSupplierParty>", "<cac:BillingReference>\n    <cac:InvoiceDocumentReference>\n      <cbc:ID>FA-2026-0950</cbc:ID>\n    </cac:InvoiceDocumentReference>\n  </cac:BillingReference>\n  <cac:AccountingSupplierParty>"}, nil},
		{"corrective invoice in CII", ciiInvoice, []string{"<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>384</ram:TypeCode>"}, []finding{err(RuleCorrection)}},
	} {
		r := check(t, edit(t, read(t, c.file), c.pairs...))
		var got []finding
		for _, f := range r.Findings {
			got = append(got, finding{f.Rule, f.Severity})
			if f.Source != rules.SourceSK || !strings.HasPrefix(f.Message, "["+f.Rule+"] ") {
				t.Errorf("%s: source %q, message %q", c.name, f.Source, f.Message)
			}
			if c.file == ublInvoice && (f.Line == 0 || strings.Contains(f.Path, "rsm:") || strings.Contains(f.Path, "ram:")) {
				t.Errorf("%s: %s is not located in the UBL document: %s, line %d", c.name, f.Rule, f.Path, f.Line)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: findings %v, want %v\n%s", c.name, got, c.want, r)
		}
	}
}

// TestTaxPointDateMessage checks that the message of SK-14 gives the dates
// as the UBL document writes them.
func TestTaxPointDateMessage(t *testing.T) {
	r := check(t, edit(t, read(t, ublInvoice), "<cbc:TaxPointDate>2027-01-12</cbc:TaxPointDate>\n", ""))
	if len(r.Findings) != 1 || !strings.Contains(r.Findings[0].Message, "(BT-72, 2027-01-12) differs from the invoice issue date (BT-2, 2027-01-15)") {
		t.Errorf("findings:\n%s", r)
	}
}

func TestValidIdentifiers(t *testing.T) {
	for _, c := range []struct {
		name  string
		valid func(string) bool
		ok    []string
		ko    []string
	}{
		{"ValidDIC", ValidDIC, []string{"2020123456", "1012345678"}, []string{"", "202012345", "20201234567", "SK2020123456", "2020 123456", "202012345X"}},
		{"ValidICDPH", ValidICDPH, []string{"SK2020123456"}, []string{"", "2020123456", "SK202012345", "SK20201234567", "sk2020123456", "SK 2020123456", "CZ2020123456"}},
		{"ValidICO", ValidICO, []string{"12345678", "00151742", "35763469"}, []string{"", "1234567", "123456789", "1234567A", "123 45678"}},
	} {
		for _, s := range c.ok {
			if !c.valid(s) {
				t.Errorf("%s(%q) = false", c.name, s)
			}
		}
		for _, s := range c.ko {
			if c.valid(s) {
				t.Errorf("%s(%q) = true", c.name, s)
			}
		}
	}
}

func TestCompanyForm(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"Príklad Softvér s.r.o.", "s.r.o."},
		{"Príklad Softvér, s. r. o.", "s.r.o."},
		{"Príklad Softvér spol. s r.o.", "s.r.o."},
		{"PRÍKLAD SPOL. S R. O.", "s.r.o."},
		{"Príklad, spoločnosť s ručením obmedzeným", "s.r.o."},
		{"Odberateľ Košice a.s.", "a.s."},
		{"Odberateľ Košice, a. s.", "a.s."},
		{"Slovenská sporiteľňa, akciová spoločnosť", "a.s."},
		{"Nová firma j. s. a.", "j.s.a."},
		{"Novák & partneri, k.s.", "k.s."},
		{"Novák a syn v.o.s.", "v.o.s."},
		{"Poľnohospodárske družstvo Hlboké", "družstvo"},
		{"Ján Príklad", ""},
		{"Ján Príklad - PRÍKLAD", ""},
		{"Mesto Košice", ""},
		{"Atlas", ""},
		{"Kovovýroba Matas", ""},
		{"Vos", ""},
		{"s.r.o.", ""},
		{"", ""},
	} {
		if got := CompanyForm(c.name); got != c.want {
			t.Errorf("CompanyForm(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestCatalogComplete checks that the catalog lists every rule of the
// package once, in order.
func TestCatalogComplete(t *testing.T) {
	want := []string{RuleSellerAddress, RuleBuyerAddress, RuleTaxRepAddress, RuleSellerEndpoint, RuleBuyerEndpoint,
		RuleDIC, RuleSellerVATID, RuleBuyerVATID, RuleICDPH, RuleSellerICO, RuleBuyerICO, RuleICO,
		RuleSellerLegalInfo, RuleTaxPointDate, RuleCorrection}
	var got []string
	for _, r := range Catalog {
		got = append(got, r.ID)
		if r.Description == "" || r.Severity != rules.Error {
			t.Errorf("%s: severity %s, description %q", r.ID, r.Severity, r.Description)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("catalog %v, want %v", got, want)
	}
}
