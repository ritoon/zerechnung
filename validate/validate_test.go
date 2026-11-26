package validate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/de"
	"github.com/ritoon/zerechnung/internal/rules/fr"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

const (
	frenchInvoice = "../internal/rules/fr/testdata/en16931.xml"    // seller in France, EN 16931
	germanInvoice = "../testdata/EN16931_Einfach.cii.xml"          // seller in Germany, EN 16931
	peppolInvoice = "../internal/rules/peppol/testdata/peppol.xml" // Peppol BIS Billing 3.0, seller in the United Kingdom
	bt24EN16931   = "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>"

	xrechnungInvoice    = "../testdata/XRECHNUNG_Einfach.cii.xml"            // XRECHNUNG, seller in Germany
	xrechnungUBLInvoice = "../testdata/XRECHNUNG_Einfach.ubl.xml"            // xrechnungInvoice in UBL 2.1
	peppolUBLInvoice    = "../internal/rules/peppol/testdata/peppol.ubl.xml" // peppolInvoice in UBL 2.1
)

func readXML(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// withProfile replaces BT-24 of an EN 16931 invoice by the identifier of p.
func withProfile(t *testing.T, doc string, p invoice.Profile) []byte {
	t.Helper()
	if !strings.Contains(doc, bt24EN16931) {
		t.Fatal("BT-24 not found")
	}
	return []byte(strings.Replace(doc, bt24EN16931, "<ram:ID>"+p.URN()+"</ram:ID>", 1))
}

func TestRulesForAuto(t *testing.T) {
	german := readXML(t, germanInvoice)
	xrechnung := readXML(t, xrechnungInvoice)
	for _, c := range []struct {
		name, doc string
		opts      Options
		want      RuleSet
	}{
		{"French seller", readXML(t, frenchInvoice), Options{Auto: true}, Default | France},
		{"German seller", german, Options{Auto: true}, Default | GermanyProfile},
		{"Belgian seller", strings.Replace(german, "<ram:CountryID>DE</ram:CountryID>", "<ram:CountryID>BE</ram:CountryID>", 1), Options{Auto: true}, Default},
		{"German seller without Auto", german, Options{}, Default},
		{"explicit rules", german, Options{Auto: true, Rules: Schema}, Schema | GermanyProfile},
		{"XRECHNUNG", xrechnung, Options{Auto: true}, Default | Germany | GermanyProfile},
		{"XRECHNUNG without Auto", xrechnung, Options{}, Default | Germany},
		// The German rules of XRECHNUNG and the Peppol rules are not lost
		// when other rule sets are selected.
		{"XRECHNUNG with French rules", xrechnung, Options{Rules: Default | France}, Default | France | Germany},
		{"XRECHNUNG explicit rules", xrechnung, Options{Rules: Schema}, Schema | Germany},
		{"XRECHNUNG profile option, explicit rules", german, Options{Profile: invoice.ProfileXRechnung, Rules: Default | France}, Default | France | Germany},
		{"Peppol", readXML(t, peppolInvoice), Options{}, Default | Peppol},
		{"Peppol with Auto", readXML(t, peppolInvoice), Options{Auto: true}, Default | Peppol},
		{"Peppol with French rules", readXML(t, peppolInvoice), Options{Rules: Default | France}, Default | France | Peppol},
		{"Peppol explicit rules", readXML(t, peppolInvoice), Options{Rules: Schema}, Schema | Peppol},
		{"Peppol profile option", german, Options{Profile: invoice.ProfilePeppol}, Default | Peppol},
		{"Peppol profile option, explicit rules", german, Options{Profile: invoice.ProfilePeppol, Rules: Default | ChorusPro}, Default | ChorusPro | Peppol},
		{"French seller with Peppol", readXML(t, frenchInvoice), Options{Auto: true, Rules: Default | Peppol}, Default | Peppol | France},
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

// findings returns the findings of r whose rule starts with prefix.
func findings(r *Report, prefix string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if strings.HasPrefix(f.Rule, prefix) {
			out = append(out, f)
		}
	}
	return out
}

func TestFrenchProfiles(t *testing.T) {
	doc := readXML(t, frenchInvoice)
	for _, c := range []struct {
		profile invoice.Profile
		opts    Options
		rule    string
		sev     Severity
	}{
		{invoice.ProfileMinimum, Options{Rules: Default | France}, "FR-PROFILE-01", Error},
		{invoice.ProfileMinimum, Options{Auto: true}, "FR-PROFILE-01", Error},
		{invoice.ProfileMinimum, Options{}, "", 0}, // French rules not selected
		{invoice.ProfileBasic, Options{Auto: true}, "FR-PROFILE-02", Warning},
		{invoice.ProfileBasicWL, Options{Auto: true}, "", 0},
		{invoice.ProfileBasicWL, Options{Auto: true, RejectBasicWL: true}, "FR-PROFILE-03", Error},
		{invoice.ProfileBasicWL, Options{RejectBasicWL: true}, "", 0}, // French rules not selected
		{invoice.ProfileEN16931, Options{Auto: true, RejectBasicWL: true}, "", 0},
		{invoice.ProfileExtendedCTCFR, Options{Auto: true}, "", 0},
		{invoice.ProfileExtended, Options{Auto: true}, "", 0},
	} {
		r := XML(withProfile(t, doc, c.profile), &c.opts)
		got := findings(r, "FR-PROFILE")
		if c.rule == "" {
			if len(got) != 0 {
				t.Errorf("%s %+v: unexpected %v", c.profile, c.opts, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Rule != c.rule || got[0].Severity != c.sev || got[0].Source != SourceFR {
			t.Errorf("%s %+v: got %v, want %s (%s)", c.profile, c.opts, got, c.rule, c.sev)
		}
		if findings(r, "DE-PROFILE") != nil {
			t.Errorf("%s: German profile check applied to a French seller", c.profile)
		}
	}
	// The profile forced by Options.Profile is the one checked.
	r := XML([]byte(doc), &Options{Rules: Default | France, Profile: invoice.ProfileMinimum})
	if !r.Has("FR-PROFILE-01") {
		t.Errorf("forced MINIMUM not reported:\n%s", r)
	}
}

func TestGermanProfiles(t *testing.T) {
	doc := readXML(t, germanInvoice)
	for _, c := range []struct {
		profile invoice.Profile
		opts    Options
		want    bool
	}{
		{invoice.ProfileMinimum, Options{Auto: true}, true},
		{invoice.ProfileBasicWL, Options{Auto: true}, true},
		{invoice.ProfileBasicWL, Options{Rules: Default | GermanyProfile}, true},
		{invoice.ProfileBasicWL, Options{}, false},
		{invoice.ProfileBasicWL, Options{Rules: Default | Germany}, false},
		{invoice.ProfileBasic, Options{Auto: true}, false},
		{invoice.ProfileEN16931, Options{Auto: true}, false},
		{invoice.ProfileExtended, Options{Auto: true}, false},
	} {
		r := XML(withProfile(t, doc, c.profile), &c.opts)
		got := findings(r, "DE-PROFILE")
		if !c.want {
			if len(got) != 0 {
				t.Errorf("%s %+v: unexpected %v", c.profile, c.opts, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Rule != "DE-PROFILE-01" || got[0].Severity != Error || got[0].Source != SourceDE {
			t.Errorf("%s %+v: got %v", c.profile, c.opts, got)
		}
		if c.opts.Auto && findings(r, "BR-DE") != nil {
			t.Errorf("%s: Auto applied the XRechnung rules to a ZUGFeRD invoice:\n%s", c.profile, r)
		}
		if findings(r, "FR-PROFILE") != nil || findings(r, "BR-FR") != nil {
			t.Errorf("%s: French rules applied to a German seller", c.profile)
		}
	}
}

func TestCatalogProfiles(t *testing.T) {
	has := func(set RuleSet, id string) bool {
		for _, r := range Catalog(set) {
			if r.ID == id {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		set  RuleSet
		id   string
		want bool
	}{
		{France, "FR-PROFILE-01", true},
		{France, "FR-PROFILE-02", true},
		{France, "FR-PROFILE-03", true},
		{Default, "FR-PROFILE-01", false},
		{GermanyProfile, "DE-PROFILE-01", true},
		{Germany, "DE-PROFILE-01", false},
		{Germany, "BR-DE-15", true},
		{GermanyProfile, "BR-DE-15", false},
		{Germany, "DE-LEITWEG-01", true},
		{Germany, "DE-GEBA-02", true},
		{Default, "DE-LEITWEG-01", false},
		{GermanyProfile, "DE-GEBA-01", false},
		{Peppol, "PEPPOL-EN16931-R001", true},
		{Peppol, "NO-R-001", true},
		{Default, "PEPPOL-EN16931-R001", false},
	} {
		if got := has(c.set, c.id); got != c.want {
			t.Errorf("Catalog(%b) has %s = %v, want %v", c.set, c.id, got, c.want)
		}
	}
}

// einfachPeppol lists the rules of the Peppol findings on
// EN16931_Einfach.ubl.xml: the invoice declares Peppol BIS Billing 3.0, but
// has no electronic address nor buyer reference, and lacks what the German
// rules of Peppol require when both parties are in Germany.
const einfachPeppol = "DE-R-001 DE-R-002 DE-R-015 PEPPOL-EN16931-R003 PEPPOL-EN16931-R010 PEPPOL-EN16931-R020 PEPPOL-EN16931-R219 PEPPOL-EN16931-R229"

func TestUBL(t *testing.T) {
	for _, c := range []struct {
		file    string
		profile invoice.Profile
		peppol  string // rules of the Peppol findings
		other   string // rules of the other findings
	}{
		{"../testdata/EN16931_Einfach.ubl.xml", invoice.ProfilePeppol, einfachPeppol, ""},
		{"../internal/rules/peppol/testdata/peppol.ubl.xml", invoice.ProfilePeppol, "", ""},
		// The Leitweg-ID 04011000-12345-34 of the corpus invoice has the
		// check digits 34 instead of 03.
		{"../testdata/XRECHNUNG_Einfach.ubl.xml", invoice.ProfileXRechnung, "", "DE-LEITWEG-02"},
		{"../testdata/ubl-tc434-creditnote1.xml", invoice.ProfileEN16931, "", ""},
	} {
		r := XML([]byte(readXML(t, c.file)), &Options{Auto: true})
		var other, peppol []string
		for _, f := range r.Findings {
			if f.Source == SourcePeppol {
				peppol = append(peppol, f.Rule)
			} else {
				other = append(other, f.Rule)
			}
		}
		slices.Sort(peppol)
		slices.Sort(other)
		if strings.Join(other, " ") != c.other || r.Profile != c.profile || strings.Join(peppol, " ") != c.peppol {
			t.Errorf("%s: profile %s, Peppol findings %v, want %s, other findings %v, want %s:\n%s", c.file, r.Profile, peppol, c.peppol, other, c.other, r)
		}
		if findings(r, "FX-") != nil || findings(r, "XSD-PROFILE") != nil {
			t.Errorf("%s: CII rules applied to UBL:\n%s", c.file, r)
		}
	}

	// The findings are located in the UBL document.
	doc := strings.Replace(readXML(t, "../testdata/EN16931_Einfach.ubl.xml"), ">529.87</cbc:PayableAmount>", ">1.234</cbc:PayableAmount>", 1)
	r := XML([]byte(doc), nil)
	if got, want := strings.Join(r.Rules(), " "), "BR-CO-16 BR-DEC-18 "+einfachPeppol+" UBL-DT-01"; got != want {
		t.Errorf("rules = %s, want %s\n%s", got, want, r)
	}
	for _, f := range r.Errors() {
		if f.Source != SourcePeppol && (f.Path != "/Invoice/cac:LegalMonetaryTotal/cbc:PayableAmount" || f.Line != 109) {
			t.Errorf("finding not located in the UBL document: %s", f)
		}
	}
	if f := findings(r, "PEPPOL-EN16931-R020"); len(f) != 1 || f[0].Source != SourcePeppol ||
		f[0].Path != "/Invoice/cac:AccountingSupplierParty/cac:Party" || f[0].Line != 18 {
		t.Errorf("PEPPOL-EN16931-R020 not located in the UBL document: %v", f)
	}

	// Rule sets: the Schema rule set checks the UBL 2.1 schema; the Peppol
	// rule set, added to a Peppol invoice whatever Rules, applies the UBL
	// schematron of Peppol; the German rules apply to an XRechnung UBL
	// invoice.
	for _, set := range []RuleSet{Schema, Peppol} {
		r = XML([]byte(doc), &Options{Rules: set})
		if got := strings.Join(r.Rules(), " "); got != einfachPeppol {
			t.Errorf("Rules %b: rules = %s, want %s\n%s", set, got, einfachPeppol, r)
		}
	}
	bad := strings.Replace(doc, "<cbc:IssueDate>2018-03-05</cbc:IssueDate>", "<cbc:IssueDate>2018-03-05</cbc:IssueDate><cbc:IssueDate>2018-03-05</cbc:IssueDate>", 1)
	r = XML([]byte(bad), &Options{Rules: Schema})
	if f := findings(r, schema.RuleCount); len(f) != 1 || f[0].Source != SourceSyntax ||
		f[0].Path != "/Invoice/cbc:IssueDate[2]" || f[0].Line != 6 || len(r.Errors()) != 1+len(strings.Fields(einfachPeppol)) {
		t.Errorf("Schema only, repeated cbc:IssueDate:\n%s", r)
	}
	if r = XML([]byte(bad), &Options{Rules: EN16931}); findings(r, "XSD-") != nil {
		t.Errorf("EN16931 only: schema checked\n%s", r)
	}
	xr := readXML(t, "../testdata/XRECHNUNG_Einfach.ubl.xml")
	root, err := xmltree.Parse([]byte(xr))
	if err != nil {
		t.Fatal(err)
	}
	if got := RulesFor(ubl.View(root, invoice.ProfileUnknown), &Options{Auto: true}); got != Default|Germany|GermanyProfile {
		t.Errorf("RulesFor(XRechnung UBL) = %b", got)
	}
	r = XML([]byte(strings.Replace(xr, "<cbc:BuyerReference>04011000-12345-34</cbc:BuyerReference>", "", 1)), nil)
	if de := findings(r, "BR-DE-15"); len(de) != 1 || de[0].Source != SourceDE {
		t.Errorf("XRechnung UBL without buyer reference:\n%s", r)
	}

	// Neither CII nor UBL.
	r = XML([]byte(`<Invoice xmlns="urn:example:invoice"/>`), nil)
	if r.Valid() || !r.Has("CII") {
		t.Errorf("unknown root: %s", r)
	}
}

func TestCatalogUBL(t *testing.T) {
	has := func(set RuleSet, id string) bool {
		for _, r := range Catalog(set) {
			if r.ID == id {
				return true
			}
		}
		return false
	}
	if !has(EN16931, "UBL-CR-001") || !has(EN16931, "UBL-SR-01") || !has(Schema, schema.RuleOrder) || has(Schema, "UBL-CR-001") || has(Schema, "XSD-UBL") ||
		!has(Peppol, "PEPPOL-EN16931-R008") || !has(Peppol, "GR-R-002") || has(Default, "GR-R-002") {
		t.Errorf("UBL rules missing from the catalog")
	}
	// The national rule sets list the identifiers of both syntaxes, once.
	for _, c := range []struct {
		set     RuleSet
		cii     string
		ubl     string
		catalog int
	}{
		{France, "BR-FR-31_BT-21", "BR-FR-30_BT-21", len(fr.Catalog) + len(fr.ProfileCatalog)},
		{Germany, "BR-TMP-7", "BR-TMP-6", len(de.Catalog)},
	} {
		if !has(c.set, c.cii) || !has(c.set, c.ubl) {
			t.Errorf("Catalog(%b): %s or %s missing", c.set, c.cii, c.ubl)
		}
		seen := map[string]bool{}
		for _, r := range Catalog(c.set) {
			if seen[r.ID] {
				t.Errorf("Catalog(%b): duplicate %s", c.set, r.ID)
			}
			seen[r.ID] = true
		}
		if len(seen) <= c.catalog {
			t.Errorf("Catalog(%b): %d rules, no UBL rule beyond the %d CII ones", c.set, len(seen), c.catalog)
		}
	}
	if !has(Germany, "PEPPOL-EN16931-R043") || has(Default, "PEPPOL-EN16931-R043") {
		t.Errorf("PEPPOL-EN16931-R043 of the XRechnung UBL schematron is not in Catalog(Germany) only")
	}
}

// TestPeppolExamples validates the official examples of Peppol BIS Billing
// 3.0, rules/examples and rules/snippets of the directory named by
// FX_PEPPOL (a clone of OpenPEPPOL/peppol-bis-invoice-3): with the default
// rule sets, which add the Peppol rules to these UBL invoices, none has an
// error.
func TestPeppolExamples(t *testing.T) {
	dir := os.Getenv("FX_PEPPOL")
	if dir == "" {
		t.Skip("FX_PEPPOL not set")
	}
	var files []string
	for _, pattern := range []string{"rules/examples/*.xml", "rules/snippets/*.xml"} {
		m, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatalf("no example in %s", dir)
	}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if r := XML(b, nil); r.Profile != invoice.ProfilePeppol || !r.Valid() {
			t.Errorf("%s: profile %s\n%s", name, r.Profile, r)
		}
	}
	t.Logf("%d examples", len(files))
}

// TestPeppol checks when the Peppol rules apply: on a Peppol invoice
// (BT-24), or on request, never by default nor with Auto.
func TestPeppol(t *testing.T) {
	r := XML([]byte(readXML(t, peppolInvoice)), nil)
	if r.Profile != invoice.ProfilePeppol || len(r.Findings) > 0 {
		t.Errorf("Peppol invoice: profile %s\n%s", r.Profile, r)
	}
	// A Peppol invoice without electronic addresses: the Peppol rules
	// apply by default.
	doc := strings.ReplaceAll(readXML(t, peppolInvoice), "URIUniversalCommunication>", "X>")
	r = XML([]byte(doc), nil)
	if len(findings(r, "PEPPOL-EN16931-R010")) != 1 || len(findings(r, "PEPPOL-EN16931-R020")) != 1 {
		t.Errorf("Peppol invoice without electronic address:\n%s", r)
	}
	for _, f := range findings(r, "PEPPOL") {
		if f.Source != SourcePeppol {
			t.Errorf("source of %s: %q", f.Rule, f.Source)
		}
	}

	french := []byte(readXML(t, frenchInvoice))
	for _, opts := range []*Options{nil, {Auto: true}} {
		if r := XML(french, opts); findings(r, "PEPPOL") != nil {
			t.Errorf("Peppol rules applied to a Factur-X invoice (%+v):\n%s", opts, r)
		}
	}
	r = XML(french, &Options{Rules: Default | Peppol})
	if findings(r, "PEPPOL-EN16931-R004") == nil {
		t.Errorf("Rules: Default | Peppol did not apply the Peppol rules:\n%s", r)
	}
}

// TestDeclaredRulesKept checks that the rules an invoice declares in BT-24
// (BR-DE for XRECHNUNG, Peppol for a Peppol invoice) are applied whatever
// rule sets are selected.
func TestDeclaredRulesKept(t *testing.T) {
	xrechnung := readXML(t, xrechnungInvoice)
	const buyerRef = "<ram:BuyerReference>04011000-12345-34</ram:BuyerReference>"
	if !strings.Contains(xrechnung, buyerRef) {
		t.Fatal("BT-10 not found")
	}
	noBuyerRef := []byte(strings.Replace(xrechnung, buyerRef, "", 1))
	// A Peppol invoice without electronic addresses, in CII and in UBL.
	noAddress := []byte(strings.ReplaceAll(readXML(t, peppolInvoice), "URIUniversalCommunication>", "X>"))
	noAddressUBL := []byte(strings.Replace(readXML(t, peppolUBLInvoice), `<cbc:EndpointID schemeID="0088">5012345000091</cbc:EndpointID>`, "", 1))
	for _, opts := range []*Options{
		nil,
		{Auto: true},
		{Rules: Default | France},
		{Rules: Default | France | ChorusPro, Auto: true},
		{Rules: Schema | EN16931},
	} {
		if r := XML(noBuyerRef, opts); len(findings(r, "BR-DE-15")) != 1 {
			t.Errorf("XRECHNUNG without BT-10 (%+v): BR-DE-15 not reported:\n%s", opts, r)
		}
		if r := XML(noAddress, opts); len(findings(r, "PEPPOL-EN16931-R010")) != 1 {
			t.Errorf("Peppol invoice without electronic address (%+v): PEPPOL-EN16931-R010 not reported:\n%s", opts, r)
		}
		if r := XML(noAddressUBL, opts); len(findings(r, "PEPPOL-EN16931-R010")) != 1 {
			t.Errorf("Peppol UBL invoice without electronic address (%+v): PEPPOL-EN16931-R010 not reported:\n%s", opts, r)
		}
	}
}

// TestPeppolRoundTrip reads the Peppol invoice and writes it back: the
// result is still a valid Peppol invoice.
func TestPeppolRoundTrip(t *testing.T) {
	inv, err := cii.Parse([]byte(readXML(t, peppolInvoice)))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Profile != invoice.ProfilePeppol {
		t.Fatalf("profile %s", inv.Profile)
	}
	data, err := cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := XML(data, nil); r.Profile != invoice.ProfilePeppol || len(r.Findings) > 0 {
		t.Errorf("profile %s\n%s\n%s", r.Profile, r, data)
	}
}
