package facturx

import (
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// codeIDs are the identifiers of the code list assertions.
var codeIDs = func() map[string]bool {
	m := map[string]bool{}
	for _, r := range codeCatalog() {
		m[r.id] = true
	}
	return m
}()

// runHybrid checks data as profile p, as the XML of a PDF when hybrid is
// set, and returns the report.
func runHybrid(t testing.TB, data string, hybrid bool) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	d := rules.NewDoc(root, invoice.ProfileUnknown)
	d.Hybrid = hybrid
	r := &rules.Report{}
	Check(&rules.Checker{Doc: d, Report: r, Source: rules.SourceFacturX})
	return r
}

// codeFindings returns the sorted distinct code list assertions of r.
func codeFindings(r *rules.Report) string {
	seen := map[string]bool{}
	var ids []string
	for _, f := range r.Findings {
		if codeIDs[f.Rule] && !seen[f.Rule] {
			seen[f.Rule] = true
			ids = append(ids, f.Rule)
		}
	}
	sort.Strings(ids)
	return strings.Join(ids, " ")
}

const (
	codeBT24     = "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>"
	codeBuyer    = "</ram:PostalTradeAddress>\n      </ram:BuyerTradeParty>"
	codeTerms    = "<ram:SpecifiedTradePaymentTerms>"
	codeQuantity = `<ram:BilledQuantity unitCode="H87">20.0000</ram:BilledQuantity>`
	codeTaxTotal = `<ram:TaxTotalAmount currencyID="EUR">56.87</ram:TaxTotalAmount>`
)

// allowance returns a document level allowance (charge false) or charge with
// the reason code element reason.
func allowance(charge bool, reason string) string {
	ind := "false"
	if charge {
		ind = "true"
	}
	return "<ram:SpecifiedTradeAllowanceCharge><ram:ChargeIndicator><udt:Indicator>" + ind +
		"</udt:Indicator></ram:ChargeIndicator><ram:ActualAmount>1.00</ram:ActualAmount>" + reason +
		"<ram:CategoryTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode>" +
		"<ram:RateApplicablePercent>19.00</ram:RateApplicablePercent></ram:CategoryTradeTax></ram:SpecifiedTradeAllowanceCharge>" + codeTerms
}

// TestCodeLists checks the code list assertions of the Factur-X schematrons
// of BASIC, EN 16931 and EXTENDED, with the first-match semantics of their
// pattern.
func TestCodeLists(t *testing.T) {
	en := readTestdata(t, "EN16931_Einfach.cii.xml")
	as := func(urn string, pairs ...string) string {
		s := strings.Replace(en, codeBT24, "<ram:ID>"+urn+"</ram:ID>", 1)
		for i := 0; i+1 < len(pairs); i += 2 {
			if strings.Count(s, pairs[i]) == 0 {
				t.Fatalf("%q not found", pairs[i])
			}
			s = strings.Replace(s, pairs[i], pairs[i+1], 1)
		}
		return s
	}
	uri := func(scheme string) string {
		return "</ram:PostalTradeAddress><ram:URIUniversalCommunication><ram:URIID schemeID=\"" + scheme +
			"\">buyer@example.org</ram:URIID></ram:URIUniversalCommunication></ram:BuyerTradeParty>"
	}
	for _, tc := range []struct {
		name   string
		data   string
		hybrid bool
		want   string
	}{
		{"EXTENDED", as(invoice.URNExtended), false, ""},
		{"EXTENDED EAS 9958", as(invoice.URNExtended, codeBuyer, uri("9958")), false, "FX-SCH-A-000704"},
		{"EXTENDED EAS EM", as(invoice.URNExtended, codeBuyer, uri("EM")), false, ""},
		{"EXTENDED allowance ABK", as(invoice.URNExtended, codeTerms, allowance(false, "<ram:ReasonCode>ABK</ram:ReasonCode>")), false, "FX-SCH-A-000973"},
		{"EXTENDED allowance 95", as(invoice.URNExtended, codeTerms, allowance(false, "<ram:ReasonCode>95</ram:ReasonCode>")), false, ""},
		{"EXTENDED empty reason", as(invoice.URNExtended, codeTerms, allowance(false, "<ram:ReasonCode></ram:ReasonCode>")), false, ""},
		{"EXTENDED charge QQQ", as(invoice.URNExtended, codeTerms, allowance(true, "<ram:ReasonCode>QQQ</ram:ReasonCode>")), false, "FX-SCH-A-000978"},
		// A list identifier other than 5153 takes the reason code from
		// the rule that checks it.
		{"EXTENDED charge QQQ, list X", as(invoice.URNExtended, codeTerms, allowance(true, `<ram:ReasonCode listID="X">QQQ</ram:ReasonCode>`)), false, ""},
		{"EXTENDED charge QQQ, list 5153", as(invoice.URNExtended, codeTerms, allowance(true, `<ram:ReasonCode listID="5153">QQQ</ram:ReasonCode>`)), false, "FX-SCH-A-000390"},
		{"EXTENDED package unit", as(invoice.URNExtended, codeQuantity, codeQuantity+`<ram:PackageQuantity unitCode="CT">1</ram:PackageQuantity>`), false, "FX-SCH-A-001084"},
		{"EXTENDED package unit XPP", as(invoice.URNExtended, codeQuantity, codeQuantity+`<ram:PackageQuantity unitCode="XPP">1</ram:PackageQuantity>`), false, ""},
		// The total VAT amount is checked against the list of its
		// currency, the invoice or the VAT accounting one, else not at all.
		{"EXTENDED VAT total in another currency", as(invoice.URNExtended, codeTaxTotal, `<ram:TaxTotalAmount currencyID="QQQ">56.87</ram:TaxTotalAmount>`), false, ""},
		{"EXTENDED unknown currency", as(invoice.URNExtended, "<ram:InvoiceCurrencyCode>EUR", "<ram:InvoiceCurrencyCode>QQQ", codeTaxTotal, `<ram:TaxTotalAmount currencyID="QQQ">56.87</ram:TaxTotalAmount>`), false, "FX-SCH-A-000861 FX-SCH-A-001006"},
		{"BASIC type code", as(invoice.URNBasic, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>"), false, "FX-SCH-A-000492"},
		{"BASIC unit", as(invoice.URNBasic, codeQuantity, `<ram:BilledQuantity unitCode="QQQ">20.0000</ram:BilledQuantity>`), false, "FX-SCH-A-000534"},
		{"BASIC EAS 9958", as(invoice.URNBasic, codeBuyer, uri("9958")), false, "FX-SCH-A-000498"},
		// EN 16931: only in a Factur-X PDF; package en16931 applies the
		// CEN code lists otherwise.
		{"EN 16931 type code", as(invoice.URNEN16931, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>"), false, ""},
		{"EN 16931 type code, hybrid", as(invoice.URNEN16931, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>"), true, "FX-SCH-A-000555"},
		{"EN 16931 EAS 9958, hybrid", as(invoice.URNEN16931, codeBuyer, uri("9958")), true, "FX-SCH-A-000571"},
		{"EN 16931, hybrid", as(invoice.URNEN16931), true, ""},
		// XRECHNUNG and EXTENDED-CTC-FR have their own reference rules.
		{"XRECHNUNG type code, hybrid", as(invoice.URNXRechnung, "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>999</ram:TypeCode>"), true, ""},
		{"EXTENDED-CTC-FR EAS 9958", as(invoice.URNExtendedCTCFR, codeBuyer, uri("9958")), true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runHybrid(t, tc.data, tc.hybrid)
			if got := codeFindings(r); got != tc.want {
				t.Errorf("code list rules = %q, want %q\n%s", got, tc.want, r)
			}
			for _, f := range r.Findings {
				if codeIDs[f.Rule] && (f.Severity != rules.Error || f.Path == "" || f.Line == 0 || !strings.Contains(f.Message, "code list")) {
					t.Errorf("incomplete finding %+v", f)
				}
			}
		})
	}
}

// TestLegacyProfileLevel checks that an obsolete BT-24 is an error where the
// Factur-X schematron applies, a warning otherwise.
func TestLegacyProfileLevel(t *testing.T) {
	en := readTestdata(t, "EN16931_Einfach.cii.xml")
	for _, tc := range []struct {
		urn    string
		hybrid bool
		want   rules.Severity
	}{
		{"urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic", false, rules.Error},
		{"URN:FACTUR-X.EU:1P0:MINIMUM", false, rules.Error},
		{"URN:CEN.EU:EN16931:2017", false, rules.Warning},
		{"URN:CEN.EU:EN16931:2017", true, rules.Error},
	} {
		r := runHybrid(t, strings.Replace(en, codeBT24, "<ram:ID>"+tc.urn+"</ram:ID>", 1), tc.hybrid)
		var got rules.Severity
		for _, f := range r.Findings {
			if f.Rule == "FX-PROFILE-03" {
				got = f.Severity
			}
		}
		if got != tc.want {
			t.Errorf("%s (hybrid %t): FX-PROFILE-03 %v, want %v", tc.urn, tc.hybrid, got, tc.want)
		}
	}
}

// TestCodeCatalog checks the catalog entries of the code list assertions:
// one per assertion of the three schematrons, less the three on BT-24.
func TestCodeCatalog(t *testing.T) {
	n := map[uint8]int{}
	for _, r := range codeCatalog() {
		n[r.mask]++
		if byID(r.id) == nil || byID(r.id).sev != rules.Error {
			t.Errorf("%s: not in the catalog as an error", r.id)
		}
	}
	if n[mBasic] != 48 || n[mEN] != 58 || n[mExt] != 209 {
		t.Errorf("assertions: %d BASIC, %d EN 16931, %d EXTENDED; want 48, 58, 209", n[mBasic], n[mEN], n[mExt])
	}
	for _, rs := range [][]codeRule{codeRulesBasic, codeRulesEN16931, codeRulesExtended} {
		for _, r := range rs {
			if r.id != "" && (r.list >= len(codeLists) || len(codeLists[r.list]) == 0 || !sort.StringsAreSorted(codeLists[r.list])) {
				t.Errorf("%s: list %d empty or not sorted", r.id, r.list)
			}
			for _, w := range r.when {
				if w.step >= strings.Count(r.path, "/")+1 {
					t.Errorf("%s: predicate on step %d of %s", r.id, w.step, r.path)
				}
			}
		}
	}
}

func TestHasValue(t *testing.T) {
	root, err := xmltree.Parse([]byte(`<a><b><c>1</c><c>2</c></b><d x="2"/></a>`))
	if err != nil {
		t.Fatal(err)
	}
	d := root.Elem("d")
	for _, tc := range []struct {
		path, v string
		want    bool
	}{
		{"../b/c", "2", true},
		{"../b/c", "3", false},
		{"/a/b/c", "1", true},
		{"/x/b/c", "1", false},
		{"../d/../b/c", "2", true},
		{"/a", "12", true},
		{".", "", true},
	} {
		if got := hasValue(d, tc.path, tc.v); got != tc.want {
			t.Errorf("hasValue(d, %q, %q) = %t", tc.path, tc.v, got)
		}
	}
	c := codeCond{op: codeAttrText, attr: "x", path: "../b/c"}
	if !c.eval(d) {
		t.Error("@x=../b/c is false")
	}
}

func BenchmarkCheckCodeLists(b *testing.B) {
	en := readTestdata(b, "EN16931_Einfach.cii.xml")
	for _, bc := range []struct {
		name string
		urn  string
	}{
		{"BASIC", invoice.URNBasic},
		{"EN16931 hybrid", invoice.URNEN16931},
		{"EXTENDED", invoice.URNExtended},
	} {
		b.Run(bc.name, func(b *testing.B) {
			data := strings.Replace(en, codeBT24, "<ram:ID>"+bc.urn+"</ram:ID>", 1)
			data = strings.Replace(data, "<ram:SpecifiedTradePaymentTerms>", allowance(true, "<ram:ReasonCode>FC</ram:ReasonCode>"), 1)
			root, err := xmltree.Parse([]byte(data))
			if err != nil {
				b.Fatal(err)
			}
			d := rules.NewDoc(root, invoice.ProfileUnknown)
			d.Hybrid = true
			k := checker{c: &rules.Checker{Doc: d, Report: &rules.Report{}}, d: d, p: mAny}
			b.ReportAllocs()
			for b.Loop() {
				k.checkCodes(d.Profile)
			}
		})
	}
}
