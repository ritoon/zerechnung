package en16931

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The EXTENDED-CTC-FR cases mutate the base invoice declared EXTENDED-CTC-FR.
// Their expected CII rules are the verdicts of the FNFE XSLT
// EXTENDED-CTC-FR-CII.xslt (France_RFE 1.4.0.04, run with Saxon), except for
// CII-SR-477, on which the XSLT stops with a type error.

const (
	tCTCFRAgreementEnd = "</ram:ApplicableHeaderTradeAgreement>"
	tLineAgreement     = "<ram:SpecifiedLineTradeAgreement>"
	tLineSettlement    = "<ram:SpecifiedLineTradeSettlement>\n"
	tItemName          = "<ram:Name>Widget</ram:Name>"
	tLineID            = "<ram:LineID>1</ram:LineID>"
	tNetPrice          = "<ram:ChargeAmount>10.00</ram:ChargeAmount></ram:NetPriceProductTradePrice>"
	lineAllowance      = `<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>0.00</ram:ActualAmount><ram:Reason>Discount</ram:Reason>`
)

// tCTCFRBase turns the base invoice into a valid EXTENDED-CTC-FR invoice: it
// declares the profile and, for BR-CO-25, has a payment due date.
var tCTCFRBase = []string{
	"urn:cen.eu:en16931:2017", invoice.URNExtendedCTCFR,
	sumStart, tPaymentTerms + sumStart,
}

const tPaymentTerms = `<ram:SpecifiedTradePaymentTerms><ram:DueDateDateTime><udt:DateTimeString format="102">20240215</udt:DateTimeString></ram:DueDateDateTime></ram:SpecifiedTradePaymentTerms>`

// ctcfr returns the base invoice declared EXTENDED-CTC-FR, edited by pairs.
func ctcfr(t *testing.T, pairs ...string) string {
	t.Helper()
	return edit(t, tBase, append(tCTCFRBase[:len(tCTCFRBase):len(tCTCFRBase)], pairs...)...)
}

func ctcfrCases() []ruleCase {
	e := func(pairs ...string) func(t *testing.T) string {
		return func(t *testing.T) string { return ctcfr(t, pairs...) }
	}
	agr := func(s string) []string { return []string{tCTCFRAgreementEnd, s + tCTCFRAgreementEnd} }
	return []ruleCase{
		{name: "base invoice", doc: e(), want: ""},
		{name: "CII-SR-013 document name", doc: e("<ram:TypeCode>380</ram:TypeCode>", "<ram:Name>Facture</ram:Name><ram:TypeCode>380</ram:TypeCode>"), want: "CII-SR-013"},
		{name: "data admitted by EXTENDED-CTC-FR", doc: e(
			// Line notes with a subject (CII-SR-039 to 041 in the CEN
			// schematron, where they never run).
			tLineID, tLineID+"<ram:IncludedNote><ram:Content>a</ram:Content><ram:SubjectCode>AAI</ram:SubjectCode></ram:IncludedNote><ram:IncludedNote><ram:Content>b</ram:Content></ram:IncludedNote>",
			// Item attribute type and measure (CII-SR-068, CII-SR-070).
			tItemName, tItemName+`<ram:ApplicableProductCharacteristic><ram:TypeCode>COLOR_TEXT</ram:TypeCode><ram:Description>Colour</ram:Description><ram:ValueMeasure unitCode="MTR">1</ram:ValueMeasure></ram:ApplicableProductCharacteristic>`,
			// Line order reference and Incoterm (CII-SR-106, CII-SR-108).
			tLineAgreement, tLineAgreement+"<ram:BuyerOrderReferencedDocument><ram:IssuerAssignedID>PO-1</ram:IssuerAssignedID><ram:LineID>1</ram:LineID></ram:BuyerOrderReferencedDocument><ram:ApplicableTradeDeliveryTerms><ram:DeliveryTypeCode>FCA</ram:DeliveryTypeCode></ram:ApplicableTradeDeliveryTerms>",
			// Credit transfer without account (CII-SR-470).
			settlStart, settlStart+"<ram:SpecifiedTradeSettlementPaymentMeans><ram:TypeCode>30</ram:TypeCode></ram:SpecifiedTradeSettlementPaymentMeans>"), want: ""},
		{name: "CII-SR-042 line note identifier", doc: e(tLineID, tLineID+"<ram:IncludedNote><ram:ID>1</ram:ID><ram:Content>a</ram:Content></ram:IncludedNote>"), want: "CII-SR-042"},
		{name: "CII-FREXT-SR-069 item attribute descriptions", doc: e(tItemName, tItemName+"<ram:ApplicableProductCharacteristic><ram:Description>a</ram:Description><ram:Description>b</ram:Description><ram:Value>1</ram:Value></ram:ApplicableProductCharacteristic>"), want: "CII-FREXT-SR-069"},
		{name: "CII-FREXT-SR-106 line delivery terms", doc: e(tLineAgreement, tLineAgreement+"<ram:ApplicableTradeDeliveryTerms><ram:DeliveryTypeCode>FCA</ram:DeliveryTypeCode><ram:Description>x</ram:Description></ram:ApplicableTradeDeliveryTerms>"), want: "CII-FREXT-SR-106"},
		{name: "CII-FREXT-SR-293 header delivery location", doc: e(agr("<ram:ApplicableTradeDeliveryTerms><ram:DeliveryTypeCode>FCA</ram:DeliveryTypeCode><ram:RelevantTradeLocation><ram:ID>x</ram:ID><ram:Name>Le Havre</ram:Name></ram:RelevantTradeLocation></ram:ApplicableTradeDeliveryTerms>")...), want: "CII-FREXT-SR-293"},
		{name: "Incoterm and location name", doc: e(agr("<ram:ApplicableTradeDeliveryTerms><ram:DeliveryTypeCode>FCA</ram:DeliveryTypeCode><ram:RelevantTradeLocation><ram:Name>Le Havre</ram:Name></ram:RelevantTradeLocation></ram:ApplicableTradeDeliveryTerms>")...), want: ""},
		{name: "CII-FREXT-SR-366 currency exchange", doc: e(settlStart, settlStart+"<ram:TaxApplicableTradeCurrencyExchange><ram:SourceCurrencyCode>EUR</ram:SourceCurrencyCode><ram:TargetCurrencyCode>USD</ram:TargetCurrencyCode><ram:ConversionRate>1.1</ram:ConversionRate><ram:MarketID>x</ram:MarketID></ram:TaxApplicableTradeCurrencyExchange>"), want: "CII-FREXT-SR-366"},
		{name: "CII-FREXT-SR-439 net price count", doc: e(tNetPrice, "<ram:ChargeAmount>10.00</ram:ChargeAmount>"+tNetPrice), want: "CII-FREXT-SR-439 CII-SR-441"},
		{name: "CII-SR-454 line taxes", doc: e(lineTax, lineTax+lineTax), want: "CII-SR-454"},
		{name: "CII-SR-471 allowance rates", doc: e(tLineSettlement, tLineSettlement+lineAllowance+"<ram:RateApplicablePercent>1</ram:RateApplicablePercent><ram:RateApplicablePercent>2</ram:RateApplicablePercent></ram:SpecifiedTradeAllowanceCharge>"), want: "CII-SR-471"},
		{name: "CII-SR-475 CII-SR-476 documents 916", doc: e(agr(`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:Name>a</ram:Name><ram:AttachmentBinaryObject mimeCode="application/pdf" filename="a.pdf">QUJD</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>B</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:Name>b</ram:Name><ram:AttachmentBinaryObject mimeCode="application/pdf" filename="b.pdf">QUJD</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`)...), want: "CII-SR-475 CII-SR-476"},
		{name: "CII-EXT-DT-018 type codes", doc: e(append(agr("<ram:ContractReferencedDocument><ram:IssuerAssignedID>C</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode></ram:ContractReferencedDocument>"),
			settlStart, settlStart+"<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>F-0</ram:IssuerAssignedID><ram:TypeCode>380</ram:TypeCode></ram:InvoiceReferencedDocument>")...), want: "CII-EXT-DT-018"},
		{name: "CII-FREXT-DT-024 reference type codes", doc: e(agr("<ram:BuyerOrderReferencedDocument><ram:IssuerAssignedID>PO</ram:IssuerAssignedID><ram:ReferenceTypeCode>ON</ram:ReferenceTypeCode></ram:BuyerOrderReferencedDocument><ram:ContractReferencedDocument><ram:IssuerAssignedID>C</ram:IssuerAssignedID><ram:ReferenceTypeCode>CT</ram:ReferenceTypeCode></ram:ContractReferencedDocument>")...), want: "CII-FREXT-DT-024"},
		{name: "CII-FREXTDT-027 issue dates", doc: e(agr(`<ram:BuyerOrderReferencedDocument><ram:IssuerAssignedID>PO</ram:IssuerAssignedID><ram:FormattedIssueDateTime><qdt:DateTimeString format="102">20240101</qdt:DateTimeString></ram:FormattedIssueDateTime></ram:BuyerOrderReferencedDocument><ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:TypeCode>50</ram:TypeCode><ram:FormattedIssueDateTime><qdt:DateTimeString format="102">20240101</qdt:DateTimeString></ram:FormattedIssueDateTime></ram:AdditionalReferencedDocument>`)...), want: "CII-FREXTDT-027"},
		{name: "CII-DT-015 header supporting document", doc: e(agr("<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:URIID>https://example.com/a</ram:URIID><ram:TypeCode>50</ram:TypeCode></ram:AdditionalReferencedDocument>")...), want: "CII-DT-015"},
		{name: "CII-SR-495 CII-SR-496 CII-DT-105 CII-DT-106 not in the schematron", doc: e(append(agr(`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>A</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode><ram:AttachmentBinaryObject>QUJD</ram:AttachmentBinaryObject></ram:AdditionalReferencedDocument>`),
			settlStart, settlStart+strings.Repeat("<ram:SpecifiedTradeSettlementPaymentMeans><ram:TypeCode>48</ram:TypeCode><ram:ApplicableTradeSettlementFinancialCard><ram:ID>1234</ram:ID></ram:ApplicableTradeSettlementFinancialCard><ram:PayerPartyDebtorFinancialAccount><ram:IBANID>DE02</ram:IBANID></ram:PayerPartyDebtorFinancialAccount></ram:SpecifiedTradeSettlementPaymentMeans>", 2))...), want: ""},
		// CII-SR-038 and CII-SR-339 name elements that the schema does not
		// allow where the FNFE schematron looks for them.
		{name: "CII-SR-038 line status reason", doc: e("<ram:SpecifiedTradeProduct>", "<ram:LineStatusReasonCode>x</ram:LineStatusReasonCode><ram:SpecifiedTradeProduct>"), want: "CII-SR-038"},
		{name: "CII-SR-339 due amount in the settlement", doc: e("<ram:DuePayableAmount>119.00</ram:DuePayableAmount>\n", "", settlStart, settlStart+"<ram:DuePayableAmount>119.00</ram:DuePayableAmount>"), want: "BR-15 BR-CO-16 CII-SR-339"},
		{name: "CII-SR-477 line total count", doc: e("<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis", "<ram:LineTotalAmount>100.00</ram:LineTotalAmount><ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis"), want: "CII-SR-477"},
	}
}

func TestCTCFRRules(t *testing.T) {
	for _, tc := range append(ctcfrCases(), ctcfrModelCases()...) {
		t.Run(tc.name, func(t *testing.T) {
			r, got := findings(t, tc.doc(t), tc.profile)
			want := strings.Fields(tc.want)
			sort.Strings(want)
			if g := strings.Join(got, " "); g != strings.Join(want, " ") {
				t.Errorf("got %q, want %q", g, tc.want)
				for _, f := range r.Findings {
					t.Logf("  %s", f)
				}
			}
		})
	}
}

// TestCTCFRSeverities checks that the syntax rules of EXTENDED-CTC-FR have
// the severities and messages of the FNFE schematron, and that the other
// profiles keep theirs.
func TestCTCFRSeverities(t *testing.T) {
	find := func(doc, rule string) *rules.Finding {
		t.Helper()
		r, _ := findings(t, doc, invoice.ProfileUnknown)
		for i, f := range r.Findings {
			if f.Rule == rule {
				return &r.Findings[i]
			}
		}
		t.Fatalf("%s not reported", rule)
		return nil
	}
	cases := []struct {
		doc, rule string
		sev       rules.Severity
	}{
		{ctcfr(t, "<ram:TypeCode>380</ram:TypeCode>", "<ram:Name>Facture</ram:Name><ram:TypeCode>380</ram:TypeCode>"), "CII-SR-013", rules.Warning},
		{ctcfr(t, tItemName, tItemName+"<ram:ApplicableProductCharacteristic><ram:Description>a</ram:Description><ram:Value>1</ram:Value><ram:Value>2</ram:Value></ram:ApplicableProductCharacteristic>"), "CII-FREXT-SR-072", rules.Error},
		{ctcfr(t, lineTax, lineTax+lineTax), "CII-SR-454", rules.Warning},
		{ctcfr(t, "<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>", "<ram:GrandTotalAmount>119.00</ram:GrandTotalAmount><ram:GrandTotalAmount>119.00</ram:GrandTotalAmount>"), "CII-SR-482", rules.Error},
	}
	for _, c := range cases {
		if f := find(c.doc, c.rule); f.Severity != c.sev {
			t.Errorf("%s: %v, want %v", c.rule, f.Severity, c.sev)
		}
	}
	// CII-SR-472 keeps its identifier with the message of the FNFE
	// schematron, which differs from the CEN one. Count rules add the
	// number found to the message.
	doc := tLineSettlement + lineAllowance + strings.Repeat("<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode></ram:CategoryTradeTax>", 2) + "</ram:SpecifiedTradeAllowanceCharge>"
	f := find(ctcfr(t, tLineSettlement, doc), "CII-SR-472")
	if want := ctcfrRules["CII-SR-472"].Description; !strings.HasPrefix(f.Message, want) || want == description["CII-SR-472"] {
		t.Errorf("CII-SR-472: message %q, want %q (CEN %q)", f.Message, want, description["CII-SR-472"])
	}
	if f := find(edit(t, tBase, tLineSettlement, doc), "CII-SR-472"); !strings.HasPrefix(f.Message, description["CII-SR-472"]) {
		t.Errorf("CII-SR-472 (CEN): message %q", f.Message)
	}
}

// TestCTCFROtherProfiles checks that the EXTENDED-CTC-FR syntax rules apply
// to EXTENDED-CTC-FR only: an EXTENDED invoice keeps the CEN syntax rules of
// the EXTENDED XSLT, an EN 16931 invoice the CEN ones.
func TestCTCFROtherProfiles(t *testing.T) {
	extra := []string{
		tItemName, tItemName + "<ram:ApplicableProductCharacteristic><ram:Description>a</ram:Description><ram:Description>b</ram:Description><ram:Value>1</ram:Value></ram:ApplicableProductCharacteristic>",
		settlStart, settlStart + "<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>F-0</ram:IssuerAssignedID><ram:TypeCode>380</ram:TypeCode></ram:InvoiceReferencedDocument>",
	}
	for _, c := range []struct {
		urn, want string
	}{
		{invoice.URNExtendedCTCFR, "CII-FREXT-SR-069"},
		{invoice.URNExtended, ""},
		{invoice.URNEN16931, "CII-DT-018 CII-SR-069"},
	} {
		doc := edit(t, tBase, append([]string{"urn:cen.eu:en16931:2017", c.urn, sumStart, tPaymentTerms + sumStart}, extra...)...)
		r, got := findings(t, doc, invoice.ProfileUnknown)
		if g := strings.Join(got, " "); g != c.want {
			t.Errorf("%s: got %q, want %q", c.urn, g, c.want)
			for _, f := range r.Findings {
				t.Logf("  %s", f)
			}
		}
	}
}

// TestCTCFRCatalog checks that the rules reported for EXTENDED-CTC-FR are in
// CTCFRCatalog, that every rule of CTCFRCatalog can be reported, and that
// the catalog holds the assertions of the FNFE schematron: 434 CII-SR (38
// errors), 95 CII-DT and 9 syntax rules of its own, less the 3 CII-DT it
// shadows, and 228 business rules and code lists.
func TestCTCFRCatalog(t *testing.T) {
	reported := map[string]bool{}
	for _, r := range ctcfrNots {
		reported[r.Rule] = true
	}
	for _, r := range ctcfrCounts {
		reported[r.Rule] = true
	}
	// The hand written rules: the identifiers of the source files that are
	// in CTCFRCatalog, under their EXTENDED-CTC-FR name.
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, "_gen.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ruleID.FindAllStringSubmatch(string(data), -1) {
			id := m[1]
			if r, ok := ctcfrRenamed[id]; ok {
				id = r
			}
			if ctcfrRules[id] != nil {
				reported[id] = true
			}
		}
	}
	reported["CII-DT-097"] = true // reported from kindChecks, written in syntax.go too
	// The rules built from a prefix and a number, or named in tables.
	for _, v := range vatCategoriesCTCFR {
		for n := 1; n <= 12; n++ {
			if id := fmt.Sprintf("%s%02d", v.prefix, n); ctcfrRules[id] != nil {
				reported[id] = true
			}
		}
	}
	for _, c := range ctcfrCategories {
		for _, id := range []string{c.one, c.b, c.ini, c.rev} {
			reported[id] = true
		}
	}
	reported["BR-FXEXT-S-09"] = true
	for id := range ctcfrCodeListText {
		reported[id] = true
	}
	counts := map[string]int{}
	errs := 0
	seen := map[string]bool{}
	for _, r := range CTCFRCatalog {
		if seen[r.ID] {
			t.Errorf("%s twice in CTCFRCatalog", r.ID)
		}
		seen[r.ID] = true
		if !reported[r.ID] {
			t.Errorf("%s is in CTCFRCatalog but never reported", r.ID)
		}
		if r.Description == "" || (r.Severity != rules.Error && r.Severity != rules.Warning) {
			t.Errorf("%s: bad catalog entry %+v", r.ID, r)
		}
		switch {
		case strings.HasPrefix(r.ID, "CII-SR-"):
			counts["CII-SR"]++
			if r.Severity == rules.Error {
				errs++
			}
		case strings.HasPrefix(r.ID, "CII-DT-"):
			counts["CII-DT"]++
		case strings.HasPrefix(r.ID, "CII-"):
			counts["CII"]++
		default:
			counts["BR"]++
		}
	}
	if counts["CII-SR"] != 434 || errs != 38 || counts["CII-DT"] != 92 || counts["CII"] != 9 || counts["BR"] != 228 {
		t.Errorf("CTCFRCatalog: %v, %d CII-SR errors; want 434 CII-SR (38 errors), 92 CII-DT, 9 CII, 228 BR", counts, errs)
	}
	for id := range reported {
		if !seen[id] {
			t.Errorf("%s is reported for EXTENDED-CTC-FR but not in CTCFRCatalog", id)
		}
	}
}

// TestCTCFRCoverage checks that each business rule that the EXTENDED-CTC-FR
// schematron adds or whose test it changes has a case that breaks it. The
// CEN rules it keeps as they are have theirs in ruleCases.
func TestCTCFRCoverage(t *testing.T) {
	broken := map[string]bool{}
	for _, tc := range append(ctcfrCases(), ctcfrModelCases()...) {
		for _, id := range strings.Fields(tc.want) {
			broken[id] = true
		}
	}
	changed := []string{"BR-CO-09", "BR-CO-14", "BR-DEC-13", "BR-DEC-15", "BR-CL-22"}
	for _, id := range changed {
		if !broken[id] {
			t.Errorf("%s: no EXTENDED-CTC-FR case breaks it", id)
		}
	}
	n := 0
	for _, r := range CTCFRCatalog {
		if _, cen := severity[r.ID]; cen || strings.HasPrefix(r.ID, "CII-") {
			continue
		}
		n++
		if !broken[r.ID] {
			t.Errorf("%s: no EXTENDED-CTC-FR case breaks it", r.ID)
		}
	}
	if n != 77 {
		t.Errorf("%d rules of the EXTENDED-CTC-FR schematron are not CEN rules, want 77", n)
	}
}

// TestCTCFRNoAllocs checks that a valid EXTENDED-CTC-FR invoice is checked
// without allocating.
func TestCTCFRNoAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates")
	}
	root, err := xmltree.Parse([]byte(ctcfr(t)))
	if err != nil {
		t.Fatal(err)
	}
	chk := &rules.Checker{Doc: rules.NewDoc(root, invoice.ProfileUnknown), Report: &rules.Report{}, Source: rules.SourceEN16931}
	if n := testing.AllocsPerRun(100, func() { Check(chk) }); n != 0 {
		t.Errorf("%v allocations per run", n)
	}
}

// TestFNFE checks the CII examples of the FNFE-MPE France_RFE package
// (FX_FNFE, a checkout of github.com/fnfempe/France_RFE at tag v1.4.0.04):
// checked as EXTENDED-CTC-FR invoices, whatever the profile they declare,
// none of them breaks a syntax rule of the EXTENDED-CTC-FR schematron, as
// with the FNFE XSLT. The EXTENDED-CTC-FR examples are also checked with the
// profile they declare: they report no error and no warning.
func TestFNFE(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	base := filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST")
	var paths []string
	for _, sub := range []string{"CII", "FX"} {
		p, err := filepath.Glob(filepath.Join(base, sub, "*", "*.xml"))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p...)
	}
	sort.Strings(paths)
	declared := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		root, err := xmltree.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		profiles := []invoice.Profile{invoice.ProfileExtendedCTCFR}
		if rules.NewDoc(root, invoice.ProfileUnknown).Profile == invoice.ProfileExtendedCTCFR {
			profiles = append(profiles, invoice.ProfileUnknown)
			declared++
		}
		for _, p := range profiles {
			r := check(t, data, p)
			for _, f := range r.Findings {
				bad := strings.HasPrefix(f.Rule, "CII-")
				if p == invoice.ProfileUnknown {
					bad = bad || f.Severity == rules.Error || f.Severity == rules.Warning
				}
				if bad {
					t.Errorf("%s (%v): %s", filepath.Base(path), p, f)
				}
			}
		}
	}
	if len(paths) != 28 || declared != 5 {
		t.Errorf("%d CII examples, %d declared EXTENDED-CTC-FR; want 28 and 5", len(paths), declared)
	}
}
