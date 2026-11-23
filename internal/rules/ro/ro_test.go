package ro

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const baseCII = "testdata/cius-ro.cii.xml"

func parse(t testing.TB, doc string) *xmltree.Node {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// findingsCII returns the sorted identifiers of the rules of the
// transposition to CII that Check reports for the CII document doc, without
// the checks of the CIF.
func findingsCII(t testing.TB, doc string) []string {
	t.Helper()
	root := parse(t, doc)
	s, err := compiled()
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	s[1].Run(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r}, root, national)
	return ruleIDs(r)
}

// TestCatalog checks that every expression of the schematron and of its
// transposition compiles, and that Catalog lists each rule of the CIUS-RO
// once, with its flag, then the checks of the CIF.
func TestCatalog(t *testing.T) {
	if _, err := compiled(); err != nil {
		t.Fatal(err)
	}
	n := 0
	ids := map[string]assertion{}
	for _, p := range patterns {
		for _, r := range p.Rules {
			for _, a := range r.Asserts {
				n++
				if national(a.ID) {
					ids[a.ID] = a
				}
			}
		}
	}
	if n != assertionCount {
		t.Errorf("%d assertions, want %d", n, assertionCount)
	}
	listed := map[string]bool{}
	for _, r := range Catalog() {
		if listed[r.ID] {
			t.Errorf("%s listed twice", r.ID)
		}
		listed[r.ID] = true
		if r.Description == "" {
			t.Errorf("%s has no description", r.ID)
		}
		if strings.HasPrefix(r.ID, "RO-CIF-") {
			if r.Severity != rules.Warning {
				t.Errorf("%s: severity %v, want warning", r.ID, r.Severity)
			}
			continue
		}
		a, ok := ids[r.ID]
		if !ok {
			t.Errorf("%s is not a rule of the CIUS-RO", r.ID)
			continue
		}
		if r.Severity != a.Flag {
			t.Errorf("%s: severity %v, flag %v", r.ID, r.Severity, a.Flag)
		}
	}
	for id := range ids {
		if !listed[id] {
			t.Errorf("%s is not in Catalog", id)
		}
	}
	for _, id := range []string{"BR-27", "RO-CIF-01", "RO-CIF-02", "BR-RO-001", "BR-DEC-RO-1010"} {
		if listed[id] == (id == "BR-27") {
			t.Errorf("Catalog: %s listed %v", id, listed[id])
		}
	}
}

// TestMessages checks that no message of the catalog keeps a placeholder
// of a value-of element, nor the Romanian half of the message.
func TestMessages(t *testing.T) {
	for _, r := range Catalog() {
		if strings.Contains(r.Description, "%") || strings.Contains(r.Description, "#") {
			t.Errorf("%s: %q", r.ID, r.Description)
		}
	}
}

// TestBases checks that the base documents meet the rules of the CIUS-RO
// and the checks of the CIF.
func TestBases(t *testing.T) {
	for _, b := range bases {
		if got := findings(t, load(t, b.file)); len(got) > 0 {
			t.Errorf("%s: %v", b.file, got)
		}
	}
	doc := load(t, baseCII)
	if got := findingsCII(t, doc); len(got) > 0 {
		t.Errorf("%s: %v", baseCII, got)
	}
	root := parse(t, doc)
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r})
	if len(r.Findings) > 0 {
		t.Errorf("%s: %v", baseCII, r.Findings)
	}
}

// TestCreditNoteContexts checks that the rules whose context starts with
// /ubl:CreditNote, a root that does not exist, do not apply to a credit
// note, as with the schematron: a tax representative and a delivery address
// that break BR-RO-140 to BR-RO-212, document level charges with three
// decimals and a reason code of 101 characters raise nothing. The parity job
// checks these variants (creditNoteInserts) against Saxon.
func TestCreditNoteContexts(t *testing.T) {
	doc := load(t, baseCreditNote)
	for k, v := range creditNoteInserts {
		if got := findings(t, strings.Replace(doc, k, v+k, 1)); len(got) > 0 {
			t.Errorf("insertion before %q: %v", k, got)
		}
	}
	// The same elements in an invoice break the rules.
	inv := load(t, baseInvoice)
	inv = strings.Replace(inv, "  <cac:Delivery>\n", strings.Split(creditNoteInserts["  <cac:PaymentMeans>\n"], "  <cac:Delivery>\n")[0]+"  <cac:Delivery>\n", 1)
	got := findings(t, inv)
	for _, id := range []string{"BR-RO-140", "BR-RO-160"} {
		if !slices.Contains(got, id) {
			t.Errorf("invoice with a second tax representative: %v lacks %s", got, id)
		}
	}
}

func TestCIFCheckDigit(t *testing.T) {
	for _, c := range []struct {
		cif  string
		want bool
	}{
		{"14399840", true},
		{"18547290", true},
		{"32198764", true},
		{"16001231", true},
		{"41526372", true},
		{"14399841", false},
		{"19", true},  // 1 × 2 × 10 mod 11 = 9
		{"10", false}, // the check digit of 1 is 9
		{"1234567897", true},
		{"1234567890", false},
		{"1", false},
		{"12345678901", false},
		{"RO14399840", false},
		{"1439984O", false},
	} {
		if got := ValidCIF(c.cif); got != c.want {
			t.Errorf("ValidCIF(%q) = %v, want %v", c.cif, got, c.want)
		}
	}
}

// TestCheckCIF checks the warnings on the Romanian VAT identifiers of the
// seller, the buyer and the tax representative, and on the tax
// registration identifier of a seller in Romania.
func TestCheckCIF(t *testing.T) {
	const doc = `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100">
<rsm:SupplyChainTradeTransaction><ram:ApplicableHeaderTradeAgreement>
<ram:SellerTradeParty><ram:PostalTradeAddress><ram:CountryID>COUNTRY</ram:CountryID></ram:PostalTradeAddress>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">SELLER</ram:ID></ram:SpecifiedTaxRegistration>
<ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">REGISTRATION</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTradeParty>
<ram:BuyerTradeParty><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">BUYER</ram:ID></ram:SpecifiedTaxRegistration></ram:BuyerTradeParty>
<ram:SellerTaxRepresentativeTradeParty><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">TAXREP</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTaxRepresentativeTradeParty>
</ram:ApplicableHeaderTradeAgreement></rsm:SupplyChainTradeTransaction></rsm:CrossIndustryInvoice>`
	const (
		seller = "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:SellerTradeParty/ram:SpecifiedTaxRegistration"
		buyer  = "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:BuyerTradeParty/ram:SpecifiedTaxRegistration/ram:ID"
		taxRep = "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:SellerTaxRepresentativeTradeParty/ram:SpecifiedTaxRegistration/ram:ID"
	)
	for _, c := range []struct {
		name                                         string
		country, seller, registration, buyer, taxRep string
		want                                         []string
	}{
		{"valid", "RO", "RO14399840", "14399840", "RO18547290", "RO32198764", nil},
		{"registration with RO", "RO", "RO14399840", "RO14399840", "RO18547290", "RO32198764", nil},
		{"wrong check digits", "RO", "RO14399841", "14399842", "RO18547291", "RO32198765", []string{
			"RO-CIF-02 " + seller + "[1]/ram:ID",
			"RO-CIF-02 " + seller + "[2]/ram:ID",
			"RO-CIF-02 " + buyer,
			"RO-CIF-02 " + taxRep,
		}},
		{"malformed", "RO", "RO1", "J40/1234/2020", "RO 18547290", "RO12345678901", []string{
			"RO-CIF-01 " + seller + "[1]/ram:ID",
			"RO-CIF-01 " + seller + "[2]/ram:ID",
			"RO-CIF-01 " + buyer,
			"RO-CIF-01 " + taxRep,
		}},
		// Foreign VAT identifiers, and the tax registration identifier of
		// a seller outside Romania, are not checked.
		{"foreign", "FR", "FR40303265045", "J40/1234/2020", "DE123456789", "ATU12345678", nil},
		{"Romanian VAT identifier of a foreign seller", "FR", "RO14399841", "X", "", "", []string{
			"RO-CIF-02 " + seller + "[1]/ram:ID",
		}},
	} {
		root := parse(t, strings.NewReplacer("COUNTRY", c.country, "SELLER", c.seller, "REGISTRATION", c.registration, "BUYER", c.buyer, "TAXREP", c.taxRep).Replace(doc))
		r := &rules.Report{}
		checkCIF(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r, Source: rules.SourceRO})
		var got []string
		for _, f := range r.Findings {
			got = append(got, f.Rule+" "+f.Path)
			if f.Severity != rules.Warning || f.Source != rules.SourceRO {
				t.Errorf("%s: %s: severity %v, source %s", c.name, f.Rule, f.Severity, f.Source)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}

// TestCodes checks that the lists of package codes are those of the
// schematron (variables ISO-3166-RO-CODES and SECTOR-RO-CODES).
func TestCodes(t *testing.T) {
	re := regexp.MustCompile(`'([^']*)'`)
	for name, want := range map[string][]string{"ISO-3166-RO-CODES": codes.RomanianCounties, "SECTOR-RO-CODES": codes.BucharestSectors} {
		var got []string
		for _, l := range patterns[0].Lets {
			if l.Name == name {
				for _, m := range re.FindAllStringSubmatch(l.Value, -1) {
					got = append(got, m[1])
				}
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: schematron %q, package codes %q", name, got, want)
		}
	}
}

// ciiLeftOut are the rules of the CIUS-RO that the transposition to CII
// leaves out (see ciiPatterns).
var ciiLeftOut = []string{"BR-DEC-RO-13", "BR-DEC-RO-15", "BR-DEC-RO-23", "BR-RO-020_2", "BR-RO-A051", "BR-RO-A052", "BR-RO-A500", "BR-RO-DT003_CN", "BR-RO-L1019"}

// TestCIITransposition checks that the transposition to CII holds every
// rule of the CIUS-RO but those of ciiLeftOut, under the flag of the UBL
// rule, and that each of them is raised by a case of TestCII.
func TestCIITransposition(t *testing.T) {
	ubl := map[string]assertion{}
	for _, r := range patterns[0].Rules {
		for _, a := range r.Asserts {
			if national(a.ID) {
				ubl[a.ID] = a
			}
		}
	}
	tested := map[string]bool{}
	for _, c := range ciiCases {
		for _, id := range c.want {
			tested[id] = true
		}
	}
	got := map[string]bool{}
	for _, r := range ciiPatterns()[0].Rules {
		for _, a := range r.Asserts {
			if got[a.ID] {
				t.Errorf("%s transposed twice", a.ID)
			}
			got[a.ID] = true
			if u, ok := ubl[a.ID]; !ok || u.Flag != a.Flag || u.Report != a.Report {
				t.Errorf("%s: not the UBL rule (%v)", a.ID, ok)
			}
			if !tested[a.ID] {
				t.Errorf("%s is raised by no case of TestCII", a.ID)
			}
		}
	}
	for id := range ubl {
		if got[id] == slices.Contains(ciiLeftOut, id) {
			t.Errorf("%s: transposed %v, left out %v", id, got[id], slices.Contains(ciiLeftOut, id))
		}
	}
}

// x returns a value of n characters.
func x(n int) string { return strings.Repeat("x", n) }

// ciiCases are changes of the CII base document, applied in order, with the
// rules of the transposition they raise.
var ciiCases = []struct {
	edits []edit
	want  []string
}{
	// Dates (format 102: YYYYMMDD).
	{[]edit{{1, ">20261120</udt:DateTimeString>", ">20261131</udt:DateTimeString>"}}, []string{"BR-RO-DT001"}},
	{[]edit{{1, ">20261020</qdt:DateTimeString>", ">2026-10-20</qdt:DateTimeString>"}}, []string{"BR-RO-DT001"}},
	{[]edit{{1, ">20261120</udt:DateTimeString>", ">20240229</udt:DateTimeString>"}}, nil},
	{[]edit{{1, ">20261120</udt:DateTimeString>", "> 20261120</udt:DateTimeString>"}}, []string{"BR-RO-DT001"}},
	{[]edit{{1, "<ram:DueDateTypeCode>29</ram:DueDateTypeCode>", `<ram:TaxPointDate><udt:DateString format="102">20260230</udt:DateString></ram:TaxPointDate>`}}, []string{"BR-RO-DT002"}},
	{[]edit{{1, ">20261220</udt:DateTimeString>", ">20261220Z</udt:DateTimeString>"}}, []string{"BR-RO-DT003"}},
	{[]edit{{1, ">20261118</udt:DateTimeString>", ">2026111</udt:DateTimeString>"}}, []string{"BR-RO-DT004"}},
	{[]edit{{1, ">20261101</udt:DateTimeString>", ">20261301</udt:DateTimeString>"}}, []string{"BR-RO-DT005"}},
	{[]edit{{1, ">20261130</udt:DateTimeString>", ">2026-11-30</udt:DateTimeString>"}}, []string{"BR-RO-DT006"}},
	// Document type, specification identifier, number, currencies.
	{[]edit{{1, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>386</ram:TypeCode>"}}, []string{"BR-RO-020_1"}},
	{[]edit{{1, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>381</ram:TypeCode>"}}, nil},
	{[]edit{{1, "CIUS-RO:1.0.1</ram:ID>", "CIUS-RO:1.0.0</ram:ID>"}}, []string{"BR-RO-001"}},
	{[]edit{{1, "<ram:ID>ZR2026-0417</ram:ID>", "<ram:ID>ZR-ABC</ram:ID>"}}, []string{"BR-RO-010"}},
	{[]edit{{1, "<ram:ID>ZR2026-0417</ram:ID>", "<ram:ID>1" + x(200) + "</ram:ID>"}}, []string{"BR-RO-L155"}},
	{[]edit{{1, "<ram:TaxCurrencyCode>RON</ram:TaxCurrencyCode>", ""}}, []string{"BR-RO-030"}},
	{[]edit{{1, "<ram:TaxCurrencyCode>RON</ram:TaxCurrencyCode>", "<ram:TaxCurrencyCode>EUR</ram:TaxCurrencyCode>"}}, []string{"BR-RO-030"}},
	{[]edit{{1, "<ram:TaxCurrencyCode>RON</ram:TaxCurrencyCode>", ""}, {1, "<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>", "<ram:InvoiceCurrencyCode>RON</ram:InvoiceCurrencyCode>"}}, nil},
	// Notes.
	{[]edit{{1, "    <ram:IncludedNote>\n      <ram:Content>Factură pentru servicii de mentenanță</ram:Content>\n    </ram:IncludedNote>\n", strings.Repeat("    <ram:IncludedNote>\n      <ram:Content>Notă</ram:Content>\n    </ram:IncludedNote>\n", 21)}}, []string{"BR-RO-A020"}},
	{[]edit{{1, "<ram:Content>Factură pentru servicii de mentenanță</ram:Content>", "<ram:Content>" + x(301) + "</ram:Content>"}}, []string{"BR-RO-L302"}},
	{[]edit{{1, "<ram:Content>Factură pentru servicii de mentenanță</ram:Content>", "<ram:Content>" + strings.Repeat("ă", 300) + "</ram:Content>"}}, nil},
	// Postcodes, cities, address lines.
	{[]edit{{1, "<ram:PostcodeCode>030167</ram:PostcodeCode>", "<ram:PostcodeCode>" + x(21) + "</ram:PostcodeCode>"}}, []string{"BR-RO-L0201"}},
	{[]edit{{1, "<ram:PostcodeCode>400129</ram:PostcodeCode>", "<ram:PostcodeCode>" + x(21) + "</ram:PostcodeCode>"}}, []string{"BR-RO-L0202"}},
	{[]edit{{1, "<ram:PostcodeCode>010061</ram:PostcodeCode>", "<ram:PostcodeCode>" + x(21) + "</ram:PostcodeCode>"}}, []string{"BR-RO-L0203"}},
	{[]edit{{1, "<ram:PostcodeCode>407280</ram:PostcodeCode>", "<ram:PostcodeCode>" + x(21) + "</ram:PostcodeCode>"}}, []string{"BR-RO-L0204"}},
	{[]edit{{1, "<ram:CityName>SECTOR3</ram:CityName>", "<ram:CityName>" + x(51) + "</ram:CityName>"}}, []string{"BR-RO-100", "BR-RO-L0501"}},
	{[]edit{{1, "<ram:CityName>Cluj-Napoca</ram:CityName>", "<ram:CityName>" + x(51) + "</ram:CityName>"}}, []string{"BR-RO-L0502"}},
	{[]edit{{1, "<ram:CityName>SECTOR1</ram:CityName>", "<ram:CityName>" + x(51) + "</ram:CityName>"}}, []string{"BR-RO-160", "BR-RO-L0503"}},
	{[]edit{{1, "<ram:CityName>Florești</ram:CityName>", "<ram:CityName>" + x(51) + "</ram:CityName>"}}, []string{"BR-RO-L0504"}},
	{[]edit{{1, "<ram:LineOne>Strada Exemplu 10</ram:LineOne>", "<ram:LineOne>" + x(151) + "</ram:LineOne>"}}, []string{"BR-RO-L151"}},
	{[]edit{{1, "<ram:LineOne>Bulevardul Eroilor 5</ram:LineOne>", "<ram:LineOne>" + x(151) + "</ram:LineOne>"}}, []string{"BR-RO-L152"}},
	{[]edit{{1, "<ram:LineOne>Calea Victoriei 1</ram:LineOne>", "<ram:LineOne>" + x(151) + "</ram:LineOne>"}}, []string{"BR-RO-L153"}},
	{[]edit{{1, "<ram:LineOne>Strada Fabricii 3</ram:LineOne>", "<ram:LineOne>" + x(151) + "</ram:LineOne>"}}, []string{"BR-RO-L154"}},
	{[]edit{{1, "<ram:LineTwo>Bloc A</ram:LineTwo>", "<ram:LineTwo>" + x(101) + "</ram:LineTwo>"}}, []string{"BR-RO-L1002"}},
	{[]edit{{1, "<ram:LineThree>Etaj 2</ram:LineThree>", "<ram:LineThree>" + x(101) + "</ram:LineThree>"}}, []string{"BR-RO-L1003"}},
	{[]edit{{1, "<ram:LineTwo>Scara B</ram:LineTwo>", "<ram:LineTwo>" + x(101) + "</ram:LineTwo>"}}, []string{"BR-RO-L1007"}},
	{[]edit{{1, "<ram:LineThree>Apartament 4</ram:LineThree>", "<ram:LineThree>" + x(101) + "</ram:LineThree>"}}, []string{"BR-RO-L1008"}},
	{[]edit{{1, "<ram:LineTwo>Corp C</ram:LineTwo>", "<ram:LineTwo>" + x(101) + "</ram:LineTwo>"}}, []string{"BR-RO-L1012"}},
	{[]edit{{1, "<ram:LineThree>Birou 9</ram:LineThree>", "<ram:LineThree>" + x(101) + "</ram:LineThree>"}}, []string{"BR-RO-L1013"}},
	{[]edit{{1, "<ram:LineTwo>Hala 2</ram:LineTwo>", "<ram:LineTwo>" + x(101) + "</ram:LineTwo>"}}, []string{"BR-RO-L1014"}},
	{[]edit{{1, "<ram:LineThree>Poarta 1</ram:LineThree>", "<ram:LineThree>" + x(101) + "</ram:LineThree>"}}, []string{"BR-RO-L1015"}},
	// Contacts, names.
	{[]edit{{1, "<ram:PersonName>Ana Popescu</ram:PersonName>", "<ram:PersonName>" + x(101) + "</ram:PersonName>"}}, []string{"BR-RO-L1004"}},
	{[]edit{{1, "<ram:CompleteNumber>+40 21 123 4567</ram:CompleteNumber>", "<ram:CompleteNumber>" + x(101) + "</ram:CompleteNumber>"}}, []string{"BR-RO-L1005"}},
	{[]edit{{1, "<ram:URIID>facturi@exemplu.ro</ram:URIID>", "<ram:URIID>" + x(101) + "</ram:URIID>"}}, []string{"BR-RO-L1006"}},
	{[]edit{{1, "<ram:PersonName>Ion Ionescu</ram:PersonName>", "<ram:PersonName>" + x(101) + "</ram:PersonName>"}}, []string{"BR-RO-L1009"}},
	{[]edit{{1, "<ram:CompleteNumber>+40 264 555 010</ram:CompleteNumber>", "<ram:CompleteNumber>" + x(101) + "</ram:CompleteNumber>"}}, []string{"BR-RO-L1010"}},
	{[]edit{{1, "<ram:URIID>achizitii@client.ro</ram:URIID>", "<ram:URIID>" + x(101) + "</ram:URIID>"}}, []string{"BR-RO-L1011"}},
	{[]edit{{1, "<ram:Name>Exemplu Software SRL</ram:Name>", "<ram:Name>" + x(201) + "</ram:Name>"}}, []string{"BR-RO-L201"}},
	{[]edit{{1, "<ram:TradingBusinessName>Exemplu Software</ram:TradingBusinessName>", "<ram:TradingBusinessName>" + x(201) + "</ram:TradingBusinessName>"}}, []string{"BR-RO-L202"}},
	{[]edit{{1, "<ram:Name>Client Cluj SA</ram:Name>", "<ram:Name>" + x(201) + "</ram:Name>"}}, []string{"BR-RO-L203"}},
	{[]edit{{1, "<ram:TradingBusinessName>Client Cluj</ram:TradingBusinessName>", "<ram:TradingBusinessName>" + x(201) + "</ram:TradingBusinessName>"}}, []string{"BR-RO-L204"}},
	{[]edit{{1, "<ram:Name>Factoring Exemplu IFN</ram:Name>", "<ram:Name>" + x(201) + "</ram:Name>"}}, []string{"BR-RO-L205"}},
	{[]edit{{1, "<ram:Name>Reprezentare Fiscală SRL</ram:Name>", "<ram:Name>" + x(201) + "</ram:Name>"}}, []string{"BR-RO-L206"}},
	{[]edit{{1, "<ram:Name>Depozit Florești</ram:Name>", "<ram:Name>" + x(201) + "</ram:Name>"}}, []string{"BR-RO-L207"}},
	{[]edit{{1, "<ram:Description>Capital social 200 RON</ram:Description>", "<ram:Description>" + x(1001) + "</ram:Description>"}}, []string{"BR-RO-L1000"}},
	// References.
	{[]edit{{1, "<ram:IssuerAssignedID>CTR-2025-19</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0302"}},
	{[]edit{{1, "<ram:IssuerAssignedID>PO-4512</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0303"}},
	{[]edit{{1, "<ram:IssuerAssignedID>SO-881</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0304"}},
	{[]edit{{1, "<ram:IssuerAssignedID>NIR-15</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0305"}},
	{[]edit{{1, "<ram:IssuerAssignedID>AV-77</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0306"}},
	{[]edit{{1, "<ram:IssuerAssignedID>LIC-2026-3</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0307"}},
	{[]edit{{1, "<ram:IssuerAssignedID>FOAIE-1</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L0308"}},
	{[]edit{{1, "<ram:Name>Foaie de prezență</ram:Name>", "<ram:Name>" + x(101) + "</ram:Name>"}}, []string{"BR-RO-L1020"}},
	{[]edit{{1, "<ram:URIID>https://exemplu.ro/raport/1</ram:URIID>", "<ram:URIID>" + x(201) + "</ram:URIID>"}}, []string{"BR-RO-L210"}},
	{[]edit{{1, `filename="prezenta.csv"`, `filename="` + x(201) + `"`}}, []string{"BR-RO-L211"}},
	{[]edit{{1, "<ram:IssuerAssignedID>ZR2026-0390</ram:IssuerAssignedID>", "<ram:IssuerAssignedID>" + x(201) + "</ram:IssuerAssignedID>"}}, []string{"BR-RO-L156"}},
	// Accounting, payment.
	{[]edit{{1, "<ram:ID>Proiect 12</ram:ID>", "<ram:ID>" + x(101) + "</ram:ID>"}}, []string{"BR-RO-L1001"}},
	{[]edit{{1, "<ram:Description>Plata în 30 de zile</ram:Description>", "<ram:Description>" + x(301) + "</ram:Description>"}}, []string{"BR-RO-L301", "BR-RO-L302"}},
	{[]edit{{1, "<ram:PaymentReference>ZR2026-0417</ram:PaymentReference>", "<ram:PaymentReference>" + x(141) + "</ram:PaymentReference>"}}, []string{"BR-RO-L140"}},
	{[]edit{{1, "<ram:AccountName>Factoring Exemplu IFN</ram:AccountName>", "<ram:AccountName>" + x(201) + "</ram:AccountName>"}}, []string{"BR-RO-L208"}},
	{[]edit{{1, "<ram:Information>Transfer</ram:Information>", "<ram:Information>Transfer</ram:Information><ram:ApplicableTradeSettlementFinancialCard><ram:ID>1234</ram:ID><ram:CardholderName>" + x(201) + "</ram:CardholderName></ram:ApplicableTradeSettlementFinancialCard>"}}, []string{"BR-RO-L209"}},
	{[]edit{{1, "<ram:Information>Transfer</ram:Information>", "<ram:Information>" + x(101) + "</ram:Information>"}}, []string{"BR-RO-L1016"}},
	// Identifiers of the seller and of the buyer.
	{[]edit{{1, `<ram:ID schemeID="VA">RO14399840</ram:ID>`, `<ram:ID schemeID="VA"> </ram:ID>`}}, nil},
	{[]edit{{1, `<ram:ID schemeID="VA">RO14399840</ram:ID>`, `<ram:ID schemeID="VA"> </ram:ID>`}, {1, `<ram:ID schemeID="VA">RO32198764</ram:ID>`, ""}}, []string{"BR-RO-065"}},
	{[]edit{{1, `<ram:ID schemeID="VA">RO14399840</ram:ID>`, ""}, {1, `<ram:ID schemeID="VA">RO32198764</ram:ID>`, ""}, {4, "<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>O</ram:CategoryCode>"}}, []string{"BR-RO-065"}},
	{[]edit{{1, `<ram:ID schemeID="VA">RO18547290</ram:ID>`, ""}, {1, "<ram:ID>J12/567/2001</ram:ID>", ""}}, []string{"BR-RO-120"}},
	{[]edit{{1, `<ram:ID schemeID="VA">RO18547290</ram:ID>`, ""}, {1, "<ram:ID>J12/567/2001</ram:ID>", ""}, {1, "<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>O</ram:CategoryCode>"}, {1, "<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>O</ram:CategoryCode>"}, {2, "<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>O</ram:CategoryCode>"}, {2, "<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>O</ram:CategoryCode>"}}, nil},
	// Addresses of the seller and of the buyer.
	{[]edit{{1, "<ram:LineOne>Strada Exemplu 10</ram:LineOne>", ""}}, []string{"BR-RO-081"}},
	{[]edit{{1, "<ram:LineOne>Bulevardul Eroilor 5</ram:LineOne>", "<ram:LineOne> </ram:LineOne>"}}, []string{"BR-RO-082"}},
	{[]edit{{1, "<ram:CityName>SECTOR3</ram:CityName>", ""}}, []string{"BR-RO-091", "BR-RO-100"}},
	{[]edit{{1, "<ram:CityName>Cluj-Napoca</ram:CityName>", ""}}, []string{"BR-RO-092"}},
	{[]edit{{1, "<ram:CityName>SECTOR3</ram:CityName>", "<ram:CityName>București</ram:CityName>"}}, []string{"BR-RO-100"}},
	{[]edit{{1, "<ram:CityName>SECTOR3</ram:CityName>", "<ram:CityName> SECTOR6 </ram:CityName>"}}, nil},
	{[]edit{{1, "<ram:CountrySubDivisionName>RO-B</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>RO-XX</ram:CountrySubDivisionName>"}}, []string{"BR-RO-110"}},
	{[]edit{{1, "<ram:CountryID>RO</ram:CountryID>", "<ram:CountryID>BG</ram:CountryID>"}, {1, "<ram:CountrySubDivisionName>RO-B</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>Sofia</ram:CountrySubDivisionName>"}}, nil},
	{[]edit{{1, "<ram:CountrySubDivisionName>RO-CJ</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>RO-B</ram:CountrySubDivisionName>"}}, []string{"BR-RO-101"}},
	{[]edit{{1, "<ram:CountrySubDivisionName>RO-CJ</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>Cluj</ram:CountrySubDivisionName>"}}, []string{"BR-RO-111"}},
	// Address of the tax representative.
	{[]edit{{1, "<ram:LineOne>Calea Victoriei 1</ram:LineOne>", ""}}, []string{"BR-RO-140"}},
	{[]edit{{1, "<ram:CityName>SECTOR1</ram:CityName>", ""}}, []string{"BR-RO-150", "BR-RO-160"}},
	{[]edit{{1, "<ram:CityName>SECTOR1</ram:CityName>", "<ram:CityName>SECTOR7</ram:CityName>"}}, []string{"BR-RO-160"}},
	{[]edit{{2, "<ram:CountrySubDivisionName>RO-B</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>RO-XX</ram:CountrySubDivisionName>"}}, []string{"BR-RO-170"}},
	// Delivery address.
	{[]edit{{1, "<ram:LineOne>Strada Fabricii 3</ram:LineOne>", ""}}, []string{"BR-RO-180"}},
	{[]edit{{1, "<ram:CityName>Florești</ram:CityName>", ""}}, []string{"BR-RO-201"}},
	{[]edit{{2, "<ram:CountrySubDivisionName>RO-CJ</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>RO-B</ram:CountrySubDivisionName>"}}, []string{"BR-RO-202"}},
	{[]edit{{2, "<ram:CountrySubDivisionName>RO-CJ</ram:CountrySubDivisionName>", ""}}, []string{"BR-RO-211", "BR-RO-212"}},
	{[]edit{{2, "<ram:CountrySubDivisionName>RO-CJ</ram:CountrySubDivisionName>", "<ram:CountrySubDivisionName>RO-XX</ram:CountrySubDivisionName>"}}, []string{"BR-RO-212"}},
	// Value added tax point date code (UNTDID 2475).
	{[]edit{{1, "<ram:DueDateTypeCode>29</ram:DueDateTypeCode>", "<ram:DueDateTypeCode>35</ram:DueDateTypeCode>"}}, []string{"BR-RO-040"}},
	{[]edit{{1, "<ram:DueDateTypeCode>29</ram:DueDateTypeCode>", "<ram:DueDateTypeCode>5</ram:DueDateTypeCode>"}}, nil},
	{[]edit{{1, "<ram:DueDateTypeCode>29</ram:DueDateTypeCode>", "<ram:DueDateTypeCode>72</ram:DueDateTypeCode>"}}, nil},
	// Lines.
	{[]edit{{1, "<ram:Name>Mentenanță</ram:Name>", "<ram:Name>" + x(101) + "</ram:Name>"}}, []string{"BR-RO-L1024"}},
	{[]edit{{1, "<ram:ID>Centru 4</ram:ID>", "<ram:ID>" + x(101) + "</ram:ID>"}}, []string{"BR-RO-L1021"}},
	{[]edit{{1, "<ram:Description>Mentenanță echipamente</ram:Description>", "<ram:Description>" + x(201) + "</ram:Description>"}}, []string{"BR-RO-L212"}},
	{[]edit{{1, "<ram:Content>Intervenții lunare</ram:Content>", "<ram:Content>" + x(301) + "</ram:Content>"}}, []string{"BR-RO-L302", "BR-RO-L303"}},
	{[]edit{{1, "<ram:Description>Nivel</ram:Description>", "<ram:Description>" + x(51) + "</ram:Description>"}}, []string{"BR-RO-L0505"}},
	{[]edit{{1, "<ram:Value>Standard</ram:Value>", "<ram:Value>" + x(101) + "</ram:Value>"}}, []string{"BR-RO-L1025"}},
	// Allowances and charges: the lines come first in CII.
	{[]edit{{1, "<ram:ActualAmount>10.00</ram:ActualAmount>", "<ram:ActualAmount>10.001</ram:ActualAmount>"}}, []string{"BR-DEC-RO-24"}},
	{[]edit{{1, "<ram:BasisAmount>300.00</ram:BasisAmount>", "<ram:BasisAmount>300.001</ram:BasisAmount>"}}, []string{"BR-DEC-RO-25"}},
	{[]edit{{1, "<ram:Reason>Discount</ram:Reason>", "<ram:Reason>" + x(101) + "</ram:Reason>"}}, []string{"BR-RO-L1022"}},
	{[]edit{{1, "<ram:ActualAmount>5.00</ram:ActualAmount>", "<ram:ActualAmount>5.001</ram:ActualAmount>"}}, []string{"BR-DEC-RO-27"}},
	{[]edit{{2, "<ram:BasisAmount>300.00</ram:BasisAmount>", "<ram:BasisAmount>300.001</ram:BasisAmount>"}}, []string{"BR-DEC-RO-28"}},
	{[]edit{{1, "<ram:Reason>Deplasare</ram:Reason>", "<ram:Reason>" + x(101) + "</ram:Reason>"}}, []string{"BR-RO-L1023"}},
	{[]edit{{1, "<ram:ActualAmount>15.00</ram:ActualAmount>", "<ram:ActualAmount>15.001</ram:ActualAmount>"}}, []string{"BR-DEC-RO-01"}},
	{[]edit{{3, "<ram:BasisAmount>300.00</ram:BasisAmount>", "<ram:BasisAmount>300.001</ram:BasisAmount>"}}, []string{"BR-DEC-RO-02"}},
	{[]edit{{2, "<ram:ReasonCode>95</ram:ReasonCode>", "<ram:ReasonCode>" + x(101) + "</ram:ReasonCode>"}}, []string{"BR-RO-L1017"}},
	{[]edit{{2, "<ram:ActualAmount>10.00</ram:ActualAmount>", "<ram:ActualAmount>10.001</ram:ActualAmount>"}}, []string{"BR-DEC-RO-05"}},
	{[]edit{{1, "<ram:BasisAmount>100.00</ram:BasisAmount>", "<ram:BasisAmount>100.001</ram:BasisAmount>"}}, []string{"BR-DEC-RO-06"}},
	{[]edit{{2, "<ram:ReasonCode>FC</ram:ReasonCode>", "<ram:ReasonCode>" + x(101) + "</ram:ReasonCode>"}}, []string{"BR-RO-L1018"}},
	{[]edit{{2, "<udt:Indicator>false</udt:Indicator>", "<udt:Indicator>X</udt:Indicator>"}, {1, "<ram:ActualAmount>15.00</ram:ActualAmount>", "<ram:ActualAmount>15.001</ram:ActualAmount>"}}, nil},
	// Totals and VAT breakdown.
	{[]edit{{1, "<ram:LineTotalAmount>355.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>355.001</ram:LineTotalAmount>"}}, []string{"BR-DEC-RO-09"}},
	{[]edit{{1, "<ram:AllowanceTotalAmount>15.00</ram:AllowanceTotalAmount>", "<ram:AllowanceTotalAmount>15.001</ram:AllowanceTotalAmount>"}}, []string{"BR-DEC-RO-10"}},
	{[]edit{{1, "<ram:ChargeTotalAmount>10.00</ram:ChargeTotalAmount>", "<ram:ChargeTotalAmount>10.001</ram:ChargeTotalAmount>"}}, []string{"BR-DEC-RO-11"}},
	{[]edit{{1, "<ram:TaxBasisTotalAmount>350.00</ram:TaxBasisTotalAmount>", "<ram:TaxBasisTotalAmount>350.001</ram:TaxBasisTotalAmount>"}}, []string{"BR-DEC-RO-12"}},
	{[]edit{{1, "<ram:GrandTotalAmount>416.50</ram:GrandTotalAmount>", "<ram:GrandTotalAmount>416.501</ram:GrandTotalAmount>"}}, []string{"BR-DEC-RO-14"}},
	{[]edit{{1, "<ram:TotalPrepaidAmount>16.50</ram:TotalPrepaidAmount>", "<ram:TotalPrepaidAmount>16.501</ram:TotalPrepaidAmount>"}}, []string{"BR-DEC-RO-16"}},
	{[]edit{{1, "<ram:RoundingAmount>0.00</ram:RoundingAmount>", "<ram:RoundingAmount>0.001</ram:RoundingAmount>"}}, []string{"BR-DEC-RO-17"}},
	{[]edit{{1, "<ram:DuePayableAmount>400.00</ram:DuePayableAmount>", "<ram:DuePayableAmount>400.001</ram:DuePayableAmount>"}}, []string{"BR-DEC-RO-18"}},
	{[]edit{{1, "<ram:BasisAmount>350.00</ram:BasisAmount>", "<ram:BasisAmount>350.001</ram:BasisAmount>"}}, []string{"BR-DEC-RO-1009"}},
	{[]edit{{1, "<ram:CalculatedAmount>66.50</ram:CalculatedAmount>", "<ram:CalculatedAmount>66.501</ram:CalculatedAmount>"}}, []string{"BR-DEC-RO-1010"}},
}

// TestCII checks the rules of the transposition to CII on changes of the
// CII base document.
func TestCII(t *testing.T) {
	base := load(t, baseCII)
	for _, c := range ciiCases {
		doc := base
		for _, e := range c.edits {
			doc = apply(t, doc, e)
		}
		if got := findingsCII(t, doc); !slices.Equal(got, c.want) {
			t.Errorf("%q → %q:\n got  %v\n want %v", c.edits[0].old, c.edits[0].new, got, c.want)
		}
	}
}

// TestCheck checks that Check applies the schematron to a UBL invoice and
// its transposition to a CII one, then the checks of the CIF.
func TestCheck(t *testing.T) {
	cii := strings.Replace(load(t, baseCII), "RO14399840", "RO14399841", 1)
	cii = strings.Replace(cii, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>386</ram:TypeCode>", 1)
	root := parse(t, cii)
	r := &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: r})
	if got := ruleIDs(r); !slices.Equal(got, []string{"BR-RO-020_1", "RO-CIF-02"}) {
		t.Errorf("CII: %v", got)
	}
	ubl := parse(t, strings.Replace(load(t, baseInvoice), "<cbc:InvoiceTypeCode>380<", "<cbc:InvoiceTypeCode>386<", 1))
	r = &rules.Report{}
	// The CII view of a UBL invoice is not needed by the schematron: an
	// empty one leaves the checks of the CIF without identifier.
	view := parse(t, `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`)
	Check(&rules.Checker{Doc: rules.NewUBLDoc(ubl, view, nil, invoice.ProfileUnknown), Report: r})
	if got := ruleIDs(r); !slices.Equal(got, []string{"BR-RO-020_1"}) {
		t.Errorf("UBL: %v", got)
	}
}
