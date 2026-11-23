package schema

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// check parses data and runs Check, with the profile read from BT-24.
func check(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	return checkHybrid(t, data, false)
}

// checkHybrid is check on the XML of a Factur-X PDF when hybrid is set.
func checkHybrid(t testing.TB, data []byte, hybrid bool) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	d := rules.NewDoc(root, invoice.ProfileUnknown)
	d.Hybrid = hybrid
	r := &rules.Report{}
	Check(&rules.Checker{Doc: d, Report: r, Source: rules.SourceSyntax})
	return r
}

func readFile(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// replace returns src with the first occurrence of old replaced by new; old
// must be present.
func replace(t testing.TB, src []byte, old, new string) []byte {
	t.Helper()
	s := string(src)
	if !strings.Contains(s, old) {
		t.Fatalf("mutation: %q not found", old)
	}
	return []byte(strings.Replace(s, old, new, 1))
}

func TestTestdata(t *testing.T) {
	for _, name := range []string{"EN16931_Einfach.cii.xml", "EN16931_Rabatte.cii.xml", "XRECHNUNG_Einfach.cii.xml"} {
		t.Run(name, func(t *testing.T) {
			r := check(t, readFile(t, filepath.Join("..", "..", "testdata", name)))
			if len(r.Findings) > 0 {
				t.Errorf("unexpected findings:\n%s", r)
			}
		})
	}
}

// minimal is a valid MINIMUM invoice.
const minimal = `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <rsm:ExchangedDocumentContext>
    <ram:GuidelineSpecifiedDocumentContextParameter>
      <ram:ID>urn:factur-x.eu:1p0:minimum</ram:ID>
    </ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:ExchangedDocumentContext>
  <rsm:ExchangedDocument>
    <ram:ID>FA-2017-0009</ram:ID>
    <ram:TypeCode>380</ram:TypeCode>
    <ram:IssueDateTime>
      <udt:DateTimeString format="102">20171113</udt:DateTimeString>
    </ram:IssueDateTime>
  </rsm:ExchangedDocument>
  <rsm:SupplyChainTradeTransaction>
    <ram:ApplicableHeaderTradeAgreement>
      <ram:BuyerReference>SERVEXP</ram:BuyerReference>
      <ram:SellerTradeParty>
        <ram:Name>Au bon moulin</ram:Name>
        <ram:SpecifiedLegalOrganization>
          <ram:ID schemeID="0002">99999999800010</ram:ID>
        </ram:SpecifiedLegalOrganization>
        <ram:PostalTradeAddress>
          <ram:CountryID>FR</ram:CountryID>
        </ram:PostalTradeAddress>
        <ram:SpecifiedTaxRegistration>
          <ram:ID schemeID="VA">FR11999999998</ram:ID>
        </ram:SpecifiedTaxRegistration>
      </ram:SellerTradeParty>
      <ram:BuyerTradeParty>
        <ram:Name>Ma jolie boutique</ram:Name>
      </ram:BuyerTradeParty>
    </ram:ApplicableHeaderTradeAgreement>
    <ram:ApplicableHeaderTradeDelivery/>
    <ram:ApplicableHeaderTradeSettlement>
      <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
        <ram:TaxBasisTotalAmount>624.90</ram:TaxBasisTotalAmount>
        <ram:TaxTotalAmount currencyID="EUR">46.25</ram:TaxTotalAmount>
        <ram:GrandTotalAmount>671.15</ram:GrandTotalAmount>
        <ram:DuePayableAmount>470.15</ram:DuePayableAmount>
      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
    </ram:ApplicableHeaderTradeSettlement>
  </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>`

// mutation is a change to a valid invoice and the findings it must produce.
type mutation struct {
	name     string
	base     string // "en16931" (testdata/EN16931_Einfach.cii.xml), "xrechnung" (testdata/XRECHNUNG_Einfach.cii.xml), "minimum" (minimal), "ubl" or "ubl-cn" (see ublMutations)
	old, new string
	more     []string // further old, new pairs
	rules    []string // expected rule of each finding, in order
	msg      string   // regular expression the first message must match
	// hybrid checks the document as the XML of a Factur-X PDF: an EN 16931
	// invoice is then checked against the Factur-X schema of the profile,
	// otherwise against the CII D16B schema.
	hybrid bool
}

var mutations = []mutation{
	{
		name: "moved before", base: "minimum",
		old:   "<ram:BuyerReference>SERVEXP</ram:BuyerReference>",
		new:   "",
		more:  []string{"</ram:SellerTradeParty>", "</ram:SellerTradeParty><ram:BuyerReference>SERVEXP</ram:BuyerReference>"},
		rules: []string{RuleOrder},
		msg:   `^ram:BuyerReference is not allowed at this position in ram:ApplicableHeaderTradeAgreement: it must come before ram:SellerTradeParty; expected here: ram:BuyerTradeParty$`,
	},
	{
		name: "moved later", base: "en16931",
		old:   "<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>\n",
		new:   "",
		more:  []string{"</ram:SpecifiedTradePaymentTerms>", "</ram:SpecifiedTradePaymentTerms><ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>"},
		rules: []string{RuleOrder},
		msg:   `^ram:InvoiceCurrencyCode is not allowed at this position in ram:ApplicableHeaderTradeSettlement: it must come before ram:ApplicableTradeTax`,
	},
	{
		name: "deleted", base: "minimum",
		old:   "<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>",
		new:   "",
		rules: []string{RuleMissing},
		msg:   `^ram:InvoiceCurrencyCode is missing in ram:ApplicableHeaderTradeSettlement \(expected before ram:SpecifiedTradeSettlementHeaderMonetarySummation\)$`,
	},
	{
		name: "deleted last", base: "minimum",
		old:   "<ram:DuePayableAmount>470.15</ram:DuePayableAmount>",
		new:   "",
		rules: []string{RuleMissing},
		msg:   `^ram:DuePayableAmount is missing in ram:SpecifiedTradeSettlementHeaderMonetarySummation$`,
	},
	{
		name: "duplicated", base: "en16931",
		old:   "<ram:TypeCode>380</ram:TypeCode>",
		new:   "<ram:TypeCode>380</ram:TypeCode><ram:TypeCode>380</ram:TypeCode>",
		rules: []string{RuleCount},
		msg:   `^ram:TypeCode may occur only once in rsm:ExchangedDocument$`,
	},
	{
		name: "too many", base: "minimum",
		old:   "<ram:TaxTotalAmount currencyID=\"EUR\">46.25</ram:TaxTotalAmount>",
		new:   strings.Repeat("<ram:TaxTotalAmount currencyID=\"EUR\">46.25</ram:TaxTotalAmount>", 3),
		rules: []string{RuleCount},
		msg:   `^ram:TaxTotalAmount may occur at most 2 times`,
	},
	{
		name: "unknown element", base: "en16931",
		old:   "<ram:TypeCode>380</ram:TypeCode>",
		new:   "<ram:TypeCode>380</ram:TypeCode><ram:Colour>blue</ram:Colour>",
		rules: []string{RuleUnexpected},
		msg:   `^ram:Colour is not allowed in rsm:ExchangedDocument; expected here: `,
	},
	{
		name: "larger profile element", base: "minimum",
		old:   "</ram:IssueDateTime>",
		new:   "</ram:IssueDateTime><ram:IncludedNote><ram:Content>x</ram:Content></ram:IncludedNote>",
		rules: []string{RuleUnexpected},
		msg:   `^ram:IncludedNote is not allowed in rsm:ExchangedDocument in profile MINIMUM \(it is allowed from profile BASIC WL\); expected here: no further element$`,
	},
	{
		name: "lines in MINIMUM", base: "minimum",
		old:   "<ram:ApplicableHeaderTradeAgreement>",
		new:   "<ram:IncludedSupplyChainTradeLineItem/><ram:ApplicableHeaderTradeAgreement>",
		rules: []string{RuleUnexpected},
		msg:   `allowed from profile BASIC\)`,
	},
	{
		name: "bad decimal", base: "en16931",
		old:   "<ram:LineTotalAmount>473.00</ram:LineTotalAmount>",
		new:   "<ram:LineTotalAmount>473,00</ram:LineTotalAmount>",
		rules: []string{RuleType},
		msg:   `^ram:LineTotalAmount: value "473,00" is not a valid xs:decimal$`,
	},
	{
		name: "empty decimal", base: "minimum",
		old:   "<ram:GrandTotalAmount>671.15</ram:GrandTotalAmount>",
		new:   "<ram:GrandTotalAmount> </ram:GrandTotalAmount>",
		rules: []string{RuleType},
	},
	{
		name: "bad boolean", base: "rabatte",
		old:   "<udt:Indicator>false</udt:Indicator>",
		new:   "<udt:Indicator>no</udt:Indicator>",
		rules: []string{RuleType},
		msg:   `^udt:Indicator: value "no" is not a valid xs:boolean$`,
	},
	{
		name: "missing attribute", base: "minimum",
		old:   `<udt:DateTimeString format="102">`,
		new:   `<udt:DateTimeString>`,
		rules: []string{RuleAttributeMissing},
		msg:   `^attribute format is required on udt:DateTimeString$`,
	},
	{
		name: "unknown attribute", base: "minimum",
		old:   `<ram:ID>FA-2017-0009</ram:ID>`,
		new:   `<ram:ID lang="fr">FA-2017-0009</ram:ID>`,
		rules: []string{RuleAttribute},
		msg:   `^attribute lang is not allowed on ram:ID \(allowed: schemeID\)$`,
	},
	{
		name: "prefixed attribute", base: "minimum",
		old:   `<ram:ID>FA-2017-0009</ram:ID>`,
		new:   `<ram:ID xsi:nil="true">FA-2017-0009</ram:ID>`,
		rules: []string{RuleAttribute},
	},
	{
		name: "xsi:schemaLocation", base: "minimum",
		old: `<rsm:ExchangedDocumentContext>`,
		new: `<rsm:ExchangedDocumentContext xsi:schemaLocation="urn:x x.xsd">`,
	},
	{
		name: "wrong namespace", base: "en16931",
		old:   "<ram:TypeCode>380</ram:TypeCode>",
		new:   "<udt:TypeCode>380</udt:TypeCode>",
		rules: []string{RuleNamespace},
		msg:   `^udt:TypeCode is in namespace "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100", expected "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" \(ram:TypeCode\)$`,
	},
	{
		name: "text in element content", base: "minimum",
		old:   "<ram:IssueDateTime>",
		new:   "<ram:IssueDateTime>2017-11-13",
		rules: []string{RuleText},
		msg:   `^ram:IssueDateTime holds elements only, text "2017-11-13" is not allowed$`,
	},
	{
		name: "element in simple content", base: "minimum",
		old:   "<ram:ID>FA-2017-0009</ram:ID>",
		new:   "<ram:ID><ram:Value>FA-2017-0009</ram:Value></ram:ID>",
		rules: []string{RuleUnexpected},
		msg:   `^ram:Value is not allowed: ram:ID holds a value, not elements$`,
	},
	{
		name: "choice missing", base: "minimum",
		old:   `<udt:DateTimeString format="102">20171113</udt:DateTimeString>`,
		new:   ``,
		rules: []string{RuleMissing},
		msg:   `^udt:DateTimeString is missing in ram:IssueDateTime$`,
	},
	{
		name: "choice repeated", base: "minimum",
		old:   `<udt:DateTimeString format="102">20171113</udt:DateTimeString>`,
		new:   strings.Repeat(`<udt:DateTimeString format="102">20171113</udt:DateTimeString>`, 2),
		rules: []string{RuleCount},
	},
	{
		name: "choice unknown", base: "minimum",
		old:   `<udt:DateTimeString format="102">20171113</udt:DateTimeString>`,
		new:   `<udt:DateString format="102">20171113</udt:DateString>`,
		rules: []string{RuleUnexpected, RuleMissing},
	},
	{
		name: "base64", base: "en16931",
		old:   "</ram:BuyerTradeParty>",
		new:   `</ram:BuyerTradeParty><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A1</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject mimeCode="text/plain" filename="a.txt">SGVsbG8</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`,
		rules: []string{RuleType},
		msg:   `is not a valid xs:base64Binary$`,
	},
	{
		name: "valid base64", base: "en16931",
		old: "</ram:BuyerTradeParty>",
		new: "</ram:BuyerTradeParty><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A1</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject mimeCode=\"text/plain\" filename=\"a.txt\">SGVs\n bG8=</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>",
	},
	{
		name: "missing required attributes", base: "en16931", hybrid: true,
		old:   "</ram:BuyerTradeParty>",
		new:   `</ram:BuyerTradeParty><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A1</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject>SGVsbG8=</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`,
		rules: []string{RuleAttributeMissing, RuleAttributeMissing},
	},
	{
		// mimeCode and filename are optional in CII D16B.
		name: "EN 16931 attachment without attributes", base: "en16931",
		old: "</ram:BuyerTradeParty>",
		new: `</ram:BuyerTradeParty><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A1</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject>SGVsbG8=</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`,
	},
	{
		name: "unknown profile", base: "en16931",
		old:   "urn:cen.eu:en16931:2017<",
		new:   "urn:example:profile<",
		rules: []string{RuleProfile},
		msg:   `^unknown profile \(BT-24 "urn:example:profile"\): the structure is checked against the EXTENDED schema`,
	},
	{
		name: "XRECHNUNG accepts EN 16931", base: "en16931",
		old: "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>",
		new: "<ram:ID>urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0</ram:ID>",
	},
	{
		name: "XRECHNUNG accepts D16B", base: "xrechnung",
		old: "<ram:ID>471102</ram:ID>",
		new: "<ram:ID>471102</ram:ID><ram:Name>Rechnung</ram:Name>",
		more: []string{
			"<ram:TypeCode>380</ram:TypeCode>", `<ram:TypeCode listID="1001" listAgencyID="6">380</ram:TypeCode>`,
			"</ram:IssueDateTime>", "</ram:IssueDateTime><ram:CopyIndicator><udt:Indicator>false</udt:Indicator></ram:CopyIndicator><ram:LanguageID>de</ram:LanguageID>",
			"</ram:SpecifiedTradeSettlementHeaderMonetarySummation>", "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>" + invoiceReference,
		},
	},
	{
		name: "EN 16931 accepts D16B", base: "en16931",
		old: "<rsm:ExchangedDocumentContext>",
		new: "<rsm:ExchangedDocumentContext><ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>",
		more: []string{
			"<ram:TypeCode>380</ram:TypeCode>", `<ram:TypeCode listID="1001" listAgencyID="6">380</ram:TypeCode>`,
			"</ram:IssueDateTime>", "</ram:IssueDateTime><ram:CopyIndicator><udt:Indicator>false</udt:Indicator></ram:CopyIndicator><ram:LanguageID>de</ram:LanguageID>",
		},
	},
	{
		name: "D16B element in Factur-X EN 16931", base: "en16931", hybrid: true,
		old:   "<ram:TypeCode>380</ram:TypeCode>",
		new:   "<ram:TypeCode>380</ram:TypeCode><ram:LanguageID>de</ram:LanguageID>",
		rules: []string{RuleUnexpected},
	},
	{
		name: "test indicator in Factur-X EN 16931", base: "en16931", hybrid: true,
		old:   "<rsm:ExchangedDocumentContext>",
		new:   "<rsm:ExchangedDocumentContext><ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>",
		rules: []string{RuleUnexpected},
	},
	{
		name: "XRECHNUNG date and time", base: "xrechnung",
		old: `<udt:DateTimeString format="102">20241115</udt:DateTimeString>`,
		new: "<udt:DateTime>2024-11-15T10:30:00+01:00</udt:DateTime>",
	},
	{
		name: "XRECHNUNG bad date and time", base: "xrechnung",
		old:   `<udt:DateTimeString format="102">20241115</udt:DateTimeString>`,
		new:   "<udt:DateTime>2024-11-15</udt:DateTime>",
		rules: []string{RuleType},
		msg:   `^udt:DateTime: value "2024-11-15" .* xs:dateTime$`,
	},
	{
		name: "XRECHNUNG fixed attribute", base: "xrechnung",
		old:   "<ram:TypeCode>380</ram:TypeCode>",
		new:   `<ram:TypeCode listID="UNTDID 1001">380</ram:TypeCode>`,
		rules: []string{RuleType},
		msg:   `^attribute listID of ram:TypeCode: value "UNTDID 1001" .*1001`,
	},
	{
		name: "XRECHNUNG one preceding invoice", base: "xrechnung",
		old:   "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>",
		new:   "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>" + invoiceReference + invoiceReference,
		rules: []string{RuleCount},
	},
	{
		name: "EN 16931 one preceding invoice", base: "en16931",
		old:   "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>",
		new:   "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>" + invoiceReference + invoiceReference,
		rules: []string{RuleCount},
	},
	{
		name: "Factur-X EN 16931 several preceding invoices", base: "en16931", hybrid: true,
		old: "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>",
		new: "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>" + invoiceReference + invoiceReference,
	},
	{
		name: "EXTENDED-CTC-FR uses EXTENDED", base: "en16931",
		old:   "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>",
		new:   "<ram:ID>urn:cen.eu:en16931:2017#conformant#urn.cpro.gouv.fr:1p0:extended-ctc-fr</ram:ID>",
		more:  []string{"</ram:IssueDateTime>", "</ram:IssueDateTime><ram:Purpose>D22B only</ram:Purpose>"},
		rules: []string{RuleUnexpected},
	},
	{
		name: "root", base: "minimum",
		old:   "<rsm:CrossIndustryInvoice ",
		new:   "<rsm:CrossIndustryDocument ",
		more:  []string{"</rsm:CrossIndustryInvoice>", "</rsm:CrossIndustryDocument>"},
		rules: []string{RuleRoot},
	},
	{
		name: "root namespace", base: "minimum",
		old:   `xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"`,
		new:   `xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:13"`,
		rules: []string{RuleNamespace, RuleNamespace, RuleNamespace, RuleNamespace},
	},
}

// invoiceReference is a preceding invoice reference (BG-3).
const invoiceReference = "<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>471101</ram:IssuerAssignedID></ram:InvoiceReferencedDocument>"

// apply returns the mutated document.
func (m *mutation) apply(t testing.TB) []byte {
	t.Helper()
	data := replace(t, mutationBase(t, m.base), m.old, m.new)
	for i := 0; i+1 < len(m.more); i += 2 {
		data = replace(t, data, m.more[i], m.more[i+1])
	}
	return data
}

func mutationBase(t testing.TB, base string) []byte {
	switch base {
	case "minimum":
		return []byte(minimal)
	case "en16931":
		return readFile(t, filepath.Join("..", "..", "testdata", "EN16931_Einfach.cii.xml"))
	case "rabatte":
		return readFile(t, filepath.Join("..", "..", "testdata", "EN16931_Rabatte.cii.xml"))
	case "xrechnung":
		return readFile(t, filepath.Join("..", "..", "testdata", "XRECHNUNG_Einfach.cii.xml"))
	case "ubl":
		return readFile(t, filepath.Join("..", "..", "testdata", "EN16931_Einfach.ubl.xml"))
	case "ubl-cn":
		return readFile(t, filepath.Join("..", "..", "testdata", "ubl-tc434-creditnote1.xml"))
	}
	t.Fatalf("unknown base %q", base)
	return nil
}

func TestMutations(t *testing.T) {
	r := check(t, []byte(minimal))
	if len(r.Findings) > 0 {
		t.Fatalf("base MINIMUM invoice: unexpected findings:\n%s", r)
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			r := checkHybrid(t, m.apply(t), m.hybrid)
			var got []string
			for _, f := range r.Findings {
				got = append(got, f.Rule)
			}
			if strings.Join(got, " ") != strings.Join(m.rules, " ") {
				t.Fatalf("rules = %v, want %v\n%s", got, m.rules, r)
			}
			if m.msg != "" && !regexp.MustCompile(m.msg).MatchString(r.Findings[0].Message) {
				t.Errorf("message %q does not match %q", r.Findings[0].Message, m.msg)
			}
			for _, f := range r.Findings {
				if f.Path == "" || f.Line == 0 {
					t.Errorf("finding without location: %v", f)
				}
				if f.Source != rules.SourceSyntax {
					t.Errorf("source = %q", f.Source)
				}
			}
		})
	}
}

func TestFindingLocation(t *testing.T) {
	src := replace(t, []byte(minimal), "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>380</ram:TypeCode>\n<ram:Colour/>")
	r := check(t, src)
	if len(r.Findings) != 1 {
		t.Fatalf("findings:\n%s", r)
	}
	f := r.Findings[0]
	if f.Path != "/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:Colour" || f.Line != 11 || f.Severity != rules.Error {
		t.Errorf("finding = %+v", f)
	}
}

// TestProfileOverride checks that the profile given by the caller wins over
// BT-24, and that each profile uses its own schema.
func TestProfileOverride(t *testing.T) {
	data := readFile(t, filepath.Join("..", "..", "testdata", "EN16931_Einfach.cii.xml"))
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		p    invoice.Profile
		fail bool
	}{
		{invoice.ProfileMinimum, true}, {invoice.ProfileBasicWL, true}, {invoice.ProfileBasic, true},
		{invoice.ProfileEN16931, false}, {invoice.ProfileXRechnung, false},
		{invoice.ProfileExtendedCTCFR, false}, {invoice.ProfileExtended, false},
	} {
		r := &rules.Report{}
		Check(&rules.Checker{Doc: rules.NewDoc(root, tc.p), Report: r, Source: rules.SourceSyntax})
		if r.Valid() == tc.fail {
			t.Errorf("%s: valid = %t\n%s", tc.p, r.Valid(), r)
		}
	}
}

func TestValues(t *testing.T) {
	for _, tc := range []struct {
		k    simpleKind
		s    string
		want bool
	}{
		{kDecimal, "12.50", true}, {kDecimal, " -0.5\n", true}, {kDecimal, "+3", true}, {kDecimal, ".5", true}, {kDecimal, "7.", true},
		{kDecimal, "", false}, {kDecimal, ".", false}, {kDecimal, "1e3", false}, {kDecimal, "1 000", false}, {kDecimal, "1.2.3", false}, {kDecimal, "--1", false},
		{kInteger, "42", true}, {kInteger, "-0", true}, {kInteger, "4.0", false}, {kInteger, "", false},
		{kBoolean, "true", true}, {kBoolean, " 0 ", true}, {kBoolean, "TRUE", false}, {kBoolean, "yes", false},
		{kBase64, "SGVsbG8=", true}, {kBase64, "SGVs bG8h", true}, {kBase64, "", true}, {kBase64, "SGVsbA==", true},
		{kBase64, "SGVsbG8", false}, {kBase64, "SGVs=G8=", false}, {kBase64, "SGV===", false}, {kBase64, "SGV*bG8=", false},
		{kDate, "2024-02-29", true}, {kDate, "2024-02-29Z", true}, {kDate, "2024-02-29+01:00", true},
		{kDate, "20240229", false}, {kDate, "2024-13-01", false}, {kDate, "2024-02-29T00:00", false},
		{kDate, "2023-02-29", false}, {kDate, "2024-02-30", false}, {kDate, "2024-04-31", false}, {kDate, "2024-12-31", true},
		{kDate, "2000-02-29", true}, {kDate, "1900-02-29", false}, {kDate, "2024-01-01-14:00", true}, {kDate, "2024-01-01+14:01", false},
		{kDate, "2024-01-01+15:00", false}, {kDate, "2024-01-01+05:60", false},
		{kTime, "12:30:00", true}, {kTime, "00:00:00.000Z", true}, {kTime, "23:59:59.5+02:00", true}, {kTime, " 24:00:00 ", true},
		{kTime, "24:00:00.000", true}, {kTime, "24:00:01", false}, {kTime, "24:00:00.1", false}, {kTime, "12:60:00", false},
		{kTime, "12:00:60", false}, {kTime, "12:00", false}, {kTime, "12:00:00.", false}, {kTime, "12:00:00+0200", false},
		{kTime, "1:00:00", false}, {kTime, "2024-02-29T12:00:00", false},
		{kDateTime, "2024-02-29T12:30:00", true}, {kDateTime, " 2024-02-29T12:30:00.25Z ", true}, {kDateTime, "2024-02-29T12:30:00-05:00", true},
		{kDateTime, "-0044-03-15T00:00:00", true}, {kDateTime, "2024-12-31T24:00:00", true},
		{kDateTime, "2024-02-29", false}, {kDateTime, "2024-02-29T12:30", false}, {kDateTime, "2024-02-29 12:30:00", false},
		{kDateTime, "2023-02-29T12:30:00", false}, {kDateTime, "2024-02-29ZT12:30:00", false}, {kDateTime, "2024-02-29T25:00:00", false},
		{kDateTime, "20240229T123000", false}, {kDateTime, "2024-02-29T12:30:00+15:00", false}, {kDateTime, "", false},
		{kString, "anything", true},
	} {
		if got := ciiTables().checkValue(tc.s, tc.k, 0) == ""; got != tc.want {
			t.Errorf("checkValue(%q, %d) valid = %t, want %t", tc.s, tc.k, got, tc.want)
		}
	}
}

func TestFacets(t *testing.T) {
	f := facets{enum: []string{"VAT", "GST"}, minLen: -1, maxLen: -1, collapse: true}
	if msg := f.check(" VAT ", nil); msg != "" {
		t.Errorf("collapsed enumeration: %s", msg)
	}
	if msg := f.check("TVA", nil); !strings.Contains(msg, "VAT, GST") {
		t.Errorf("enumeration: %q", msg)
	}
	f = facets{minLen: 2, maxLen: 3}
	for s, ok := range map[string]bool{"a": false, "ab": true, "abé": true, "abcd": false} {
		if got := f.check(s, nil) == ""; got != ok {
			t.Errorf("length %q: valid = %t", s, got)
		}
	}
	f = facets{minLen: -1, maxLen: -1}
	res := []*regexp.Regexp{regexp.MustCompile(`^(?:[A-Z]{2})$`)}
	if f.check("FR", res) != "" || f.check("FRA", res) == "" {
		t.Error("pattern")
	}
}

// TestTables checks invariants of the generated tables.
func TestTables(t *testing.T) {
	if Version() == "" {
		t.Error("no schema version")
	}
	for i := range types {
		ty := &types[i]
		if int(ty.first)+int(ty.n) > len(particles) || int(ty.afirst)+int(ty.an) > len(attributes) {
			t.Fatalf("type %d (%s) out of bounds", i, ty.name)
		}
		if ty.profiles == 0 || ty.profiles >= 1<<numProfiles {
			t.Errorf("type %d (%s): profiles %#x", i, ty.name, ty.profiles)
		}
		seen := map[string]bool{}
		for _, p := range particles[ty.first : ty.first+ty.n] {
			if seen[p.name] {
				t.Errorf("type %s: %s twice", ty.name, p.name)
			}
			seen[p.name] = true
			if int(p.typ) >= len(types) || int(p.ns) >= len(namespaces) {
				t.Errorf("type %s: particle %s out of bounds", ty.name, p.name)
			}
			// A child type is used by every profile using its parent.
			if types[p.typ].profiles&ty.profiles != ty.profiles {
				t.Errorf("type %s: child %s not in the same profiles", ty.name, p.name)
			}
		}
	}
	for p, r := range roots {
		if types[r.typ].profiles&(1<<p) == 0 {
			t.Errorf("root of %s has the wrong profile", profileNames[p])
		}
	}
}

// TestCatalog checks that every rule reported by the tests is in Catalog.
func TestCatalog(t *testing.T) {
	ids := map[string]bool{}
	for _, ri := range Catalog {
		if ids[ri.ID] {
			t.Errorf("%s twice in Catalog", ri.ID)
		}
		ids[ri.ID] = true
		if ri.Description == "" || (ri.Severity != rules.Error && ri.Severity != rules.Warning) {
			t.Errorf("bad Catalog entry %+v", ri)
		}
	}
	reported := map[string]rules.Severity{}
	for _, m := range mutations {
		r := check(t, m.apply(t))
		for _, f := range r.Findings {
			reported[f.Rule] = f.Severity
		}
	}
	for _, m := range ublMutations {
		r := checkUBLData(t, m.apply(t))
		for _, f := range r.Findings {
			reported[f.Rule] = f.Severity
		}
	}
	for id, sev := range reported {
		if !ids[id] {
			t.Errorf("rule %s reported but not in Catalog", id)
		}
		for _, ri := range Catalog {
			if ri.ID == id && ri.Severity != sev {
				t.Errorf("rule %s reported with severity %s, Catalog says %s", id, sev, ri.Severity)
			}
		}
	}
	for id := range ids {
		if _, ok := reported[id]; !ok {
			t.Errorf("rule %s in Catalog is never exercised by the tests", id)
		}
	}
}

// knownStructureErrors lists the corpus files that do not match the
// Factur-X 1.09.2 schema of their profile: the rules they break and why. Most
// of them come from an older kit (ZUGFeRD 2.0 / 2.1, Factur-X 1.0) whose
// schemas differ from 1.09.2. Those marked fail are in a fail directory of
// the corpus: their errors are the point of the sample, so each listed rule
// must still be reported. The fail sample "XRechnung 722825203.XML" is not
// listed: its elements outside EN 16931 (ram:DeliveryNoteReferencedDocument
// on the lines, an address in the buyer's legal organization) are in the CII
// D16B schema of XRECHNUNG, which KoSIT applies too; it fails BR-DE-18 (see
// internal/rules/de).
var knownStructureErrors = map[string]struct {
	rules []string
	why   string
	fail  bool
}{
	"MustangGnuaccountingBeispielRE-20190610_507.xml": {rules: []string{RuleUnexpected},
		why: "ZUGFeRD 2.0 sample: ram:GermanBankleitzahlID (D16B) was dropped from the Factur-X schemas"},
	"zugferd_2p0_EN16931_1_Teilrechnung.xml": {rules: []string{RuleMissing},
		why: "ZUGFeRD 2.0 kit: line price allowance without ram:ChargeIndicator, required by the 1.09.2 schema"},
	"zugferd_2p0_EN16931_Rabatte.xml": {rules: []string{RuleMissing},
		why: "ZUGFeRD 2.0 kit: line price allowance without ram:ChargeIndicator, required by the 1.09.2 schema"},
	"zugferd_2p0_EN16931_Rechnungskorrektur.xml": {rules: []string{RuleMissing},
		why: "ZUGFeRD 2.0 kit: line price allowance without ram:ChargeIndicator, required by the 1.09.2 schema"},
	"zugferd_2p0_MINIMUM.xml": {rules: []string{RuleUnexpected},
		why: "ZUGFeRD 2.0 kit: buyer street address in MINIMUM, where the schema (1.0.07 to 1.09.2) only allows ram:CountryID"},
	"zugferd_2p1_MINIMUM_Buchungshilfe.xml": {rules: []string{RuleUnexpected},
		why: "ZUGFeRD 2.1 kit: street addresses and ram:LineTotalAmount in MINIMUM, not in the MINIMUM schema"},
	"zugferd_2p1_MINIMUM_Rechnung.xml": {rules: []string{RuleUnexpected},
		why: "ZUGFeRD 2.1 kit: street addresses in MINIMUM, where the schema only allows ram:CountryID"},
}

// corpusFiles returns the CII invoices (root rsm:CrossIndustryInvoice) found
// under the directories named by FX_CORPUS and FX_CORPUS_XML. Files that are
// not well-formed CII invoices are ignored.
func corpusFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	for _, env := range []string{"FX_CORPUS", "FX_CORPUS_XML"} {
		dir := os.Getenv(env)
		if dir == "" {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".xml") || strings.Contains(path, "PEPPOL") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if root, err := xmltree.Parse(data); err == nil && root.Name == "CrossIndustryInvoice" {
				files[path] = data
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		t.Skip("FX_CORPUS and FX_CORPUS_XML not set or empty")
	}
	return files
}

func TestCorpus(t *testing.T) {
	files := corpusFiles(t)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	stats := map[string][2]int{} // profile: valid, known exceptions
	for _, path := range paths {
		root, _ := xmltree.Parse(files[path])
		doc := rules.NewDoc(root, invoice.ProfileUnknown)
		r := &rules.Report{}
		Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceSyntax})
		s := stats[doc.Profile.String()]
		known, isKnown := knownStructureErrors[filepath.Base(path)]
		switch {
		case r.Valid() && !isKnown:
			s[0]++
		case isKnown:
			s[1]++
			for _, f := range r.Errors() {
				if !contains(known.rules, f.Rule) {
					t.Errorf("%s: unexpected %v (known exception: %s)", path, f, known.why)
				}
			}
			switch {
			case known.fail:
				for _, id := range known.rules {
					if !contains(r.Rules(), id) {
						t.Errorf("%s: %s not reported (expected failure: %s)", path, id, known.why)
					}
				}
			case r.Valid():
				t.Logf("%s: listed as a known exception but now valid", path)
			}
		default:
			t.Errorf("%s (%s):\n%s", path, doc.Profile, r)
		}
		stats[doc.Profile.String()] = s
	}
	var keys []string
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-16s %3d valid, %d known exceptions", k, stats[k][0], stats[k][1])
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func benchmarkFile(b *testing.B, path string) {
	data := readFile(b, path)
	root, err := xmltree.Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	r := &rules.Report{}
	c := &rules.Checker{Doc: doc, Report: r, Source: rules.SourceSyntax}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Check(c)
	}
	if len(r.Findings) > 0 {
		b.Fatalf("findings:\n%s", r)
	}
}

func BenchmarkCheckEN16931Einfach(b *testing.B) {
	benchmarkFile(b, filepath.Join("..", "..", "testdata", "EN16931_Einfach.cii.xml"))
}

func BenchmarkCheckXRechnungEinfach(b *testing.B) {
	benchmarkFile(b, filepath.Join("..", "..", "testdata", "XRECHNUNG_Einfach.cii.xml"))
}

func BenchmarkCheckMinimum(b *testing.B) {
	root, err := xmltree.Parse([]byte(minimal))
	if err != nil {
		b.Fatal(err)
	}
	c := &rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: &rules.Report{}, Source: rules.SourceSyntax}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Check(c)
	}
}

// BenchmarkCheckInvalid measures the error path (one finding per run).
func BenchmarkCheckInvalid(b *testing.B) {
	data := readFile(b, filepath.Join("..", "..", "testdata", "EN16931_Einfach.cii.xml"))
	data = replace(b, data, "<ram:LineTotalAmount>473.00</ram:LineTotalAmount>", "<ram:LineTotalAmount>473,00</ram:LineTotalAmount>")
	root, err := xmltree.Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Check(&rules.Checker{Doc: doc, Report: &rules.Report{}, Source: rules.SourceSyntax})
	}
}

// validBase64Ref is the former implementation of validBase64.
func validBase64Ref(s string) bool {
	n, pad := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case isSpace(c):
			continue
		case c == '=':
			pad++
			if pad > 2 {
				return false
			}
		case pad > 0:
			return false
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', isDigit(c), c == '+', c == '/':
		default:
			return false
		}
		n++
	}
	return n%4 == 0
}

func TestValidBase64(t *testing.T) {
	inputs := []string{"", " ", "aGVsbG8=", "aGVsbG8", "aGVs\nbG8=", "aGVsbG8==", "aGVsbG8===", "aGVsbG8=a", "aGVs bG8 = ", "abc*", "\xff"}
	rnd := rand.New(rand.NewPCG(1, 2))
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/= \t\r\n*\x00\xff"
	for range 5000 {
		b := make([]byte, rnd.IntN(16))
		for i := range b {
			b[i] = alphabet[rnd.IntN(len(alphabet))]
		}
		inputs = append(inputs, string(b))
	}
	for _, in := range inputs {
		if got, want := validBase64(in), validBase64Ref(in); got != want {
			t.Errorf("validBase64(%q) = %v, want %v", in, got, want)
		}
	}
}
