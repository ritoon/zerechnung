package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

const germanUBLInvoice = "../testdata/EN16931_Einfach.ubl.xml" // germanInvoice in UBL 2.1

// edition2026 returns the invoice name with its specification identifier
// (BT-24) moved to EN 16931-1:2026.
func edition2026(t *testing.T, name string) []byte {
	t.Helper()
	doc := readXML(t, name)
	const from, to = "urn:cen.eu:en16931:2017", "urn:cen.eu:en16931:2026"
	if strings.Count(doc, from) != 1 {
		t.Fatalf("%s: BT-24 not found once", name)
	}
	return []byte(strings.Replace(doc, from, to, 1))
}

// TestEdition2026 checks that an invoice whose specification identifier
// names EN 16931-1:2026 gives only EN16931-EDITION-01, located at BT-24,
// whatever the rule sets: the rules of the package implement EN 16931-1:2017.
func TestEdition2026(t *testing.T) {
	for _, c := range []struct {
		name, path string
	}{
		{germanInvoice, "/rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext/ram:GuidelineSpecifiedDocumentContextParameter/ram:ID"},
		{germanUBLInvoice, "/Invoice/cbc:CustomizationID"},
	} {
		doc := edition2026(t, c.name)
		for _, opts := range []*Options{
			nil,
			{Rules: Schema},
			{Rules: Default | France | Germany | GermanyProfile | ChorusPro | Peppol | Netherlands},
			{Auto: true, RejectBasicWL: true},
		} {
			r := XML(doc, opts)
			if got := ruleIDs(r); !slices.Equal(got, []string{ruleEdition}) {
				t.Errorf("%s %+v: findings = %v", c.name, opts, r.Findings)
				continue
			}
			f := r.Findings[0]
			if f.Severity != Error || f.Source != SourceEN16931 || f.Path != c.path || f.Line == 0 ||
				!strings.Contains(f.Message, "EN 16931-1:2026") || !strings.Contains(f.Message, "no other rule") {
				t.Errorf("%s %+v: %s = %+v", c.name, opts, ruleEdition, f)
			}
			if r.Valid() || r.Profile == invoice.ProfileUnknown {
				t.Errorf("%s %+v: valid = %v, profile = %v", c.name, opts, r.Valid(), r.Profile)
			}
		}
	}
}

// TestEdition2026Forced checks that a forced profile applies the rules of
// EN 16931-1:2017 to an EN 16931-1:2026 invoice, EN16931-EDITION-01 still
// being reported first.
func TestEdition2026Forced(t *testing.T) {
	r := XML(edition2026(t, germanInvoice), &Options{Profile: invoice.ProfileEN16931})
	ids := ruleIDs(r)
	if len(ids) == 0 || ids[0] != ruleEdition || !strings.Contains(r.Findings[0].Message, "applied as requested") {
		t.Fatalf("findings = %v", r.Findings)
	}
	// The Factur-X rules ran: they do not know the 2026 identifier.
	if !slices.Contains(ids, "FX-PROFILE-02") || r.Profile != invoice.ProfileEN16931 {
		t.Errorf("findings = %v, profile = %v", r.Findings, r.Profile)
	}
}

// TestEdition2017 checks that the invoices of EN 16931-1:2017, and those
// whose identifier names no edition, do not give EN16931-EDITION-01.
func TestEdition2017(t *testing.T) {
	german := readXML(t, germanInvoice)
	for name, doc := range map[string][]byte{
		"CII":      []byte(german),
		"UBL":      []byte(readXML(t, germanUBLInvoice)),
		"French":   []byte(readXML(t, frenchInvoice)),
		"MINIMUM":  withProfile(t, german, invoice.ProfileMinimum),
		"BASIC WL": withProfile(t, german, invoice.ProfileBasicWL),
		"EXTENDED": withProfile(t, german, invoice.ProfileExtended),
	} {
		if f := findings(XML(doc, nil), ruleEdition); len(f) != 0 {
			t.Errorf("%s: %v", name, f)
		}
	}
}

// TestCatalogEdition checks that EN16931-EDITION-01 is listed once, in the
// EN16931 rule set.
func TestCatalogEdition(t *testing.T) {
	n := 0
	for _, r := range Catalog(EN16931) {
		if r.ID == ruleEdition {
			n++
			if r.Severity != Error || r.Description == "" {
				t.Errorf("%+v", r)
			}
		}
	}
	if n != 1 {
		t.Errorf("%s listed %d times in Catalog(EN16931)", ruleEdition, n)
	}
	for _, r := range Catalog(FacturX) {
		if r.ID == ruleEdition {
			t.Errorf("%s listed in Catalog(FacturX)", ruleEdition)
		}
	}
}

// xrechnungIDs gives the specification identifiers (BT-24) an XRechnung 4.0
// invoice may declare. XRechnung 4.0 follows EN 16931-1:2026, so its
// identifier names that edition; the version suffix is not published yet
// (the pre-version of 15 September 2026 is a specification document, without
// schematrons, XSD or KoSIT validator configuration), so both the suffix of
// XRechnung 3.0 and a 4.0 one are tried.
var xrechnungIDs = []string{
	strings.Replace(invoice.URNXRechnung, "en16931:2017", "en16931:2026", 1),
	"urn:cen.eu:en16931:2026#compliant#urn:xeinkauf.de:kosit:xrechnung_4.0",
}

// TestXRechnung40 checks that an invoice which declares XRechnung 4.0 is not
// checked against the rules of XRechnung 3.0.2: it gives
// EN16931-EDITION-01 alone, whatever the rule sets, although the German
// rules apply to an XRECHNUNG invoice without any option. The profile is
// still read as XRECHNUNG, so the report names it. XRechnung 3.0.x stays in
// force until 31 July 2027 at least, and the validation artefacts of 4.0 are
// not published: a forced profile applies the rules of 3.0.2 anyway, and
// BR-DE-21 then reports an identifier its schematron does not know.
func TestXRechnung40(t *testing.T) {
	for _, name := range []string{xrechnungInvoice, xrechnungUBLInvoice} {
		doc := readXML(t, name)
		if strings.Count(doc, invoice.URNXRechnung) != 1 {
			t.Fatalf("%s: BT-24 not found once", name)
		}
		for _, id := range xrechnungIDs {
			b := []byte(strings.Replace(doc, invoice.URNXRechnung, id, 1))
			for _, opts := range []*Options{
				nil,
				{Rules: Schema},
				{Rules: Default | Germany | GermanyProfile},
				{Auto: true},
			} {
				r := XML(b, opts)
				if got := ruleIDs(r); !slices.Equal(got, []string{ruleEdition}) {
					t.Errorf("%s %q %+v: findings = %v", name, id, opts, r.Findings)
					continue
				}
				if !strings.Contains(r.Findings[0].Message, "EN 16931-1:2026") || r.Valid() ||
					r.Profile != invoice.ProfileXRechnung {
					t.Errorf("%s %q %+v: %+v, valid = %v, profile = %v", name, id, opts, r.Findings[0], r.Valid(), r.Profile)
				}
			}
			r := XML(b, &Options{Profile: invoice.ProfileXRechnung})
			ids := ruleIDs(r)
			if len(ids) == 0 || ids[0] != ruleEdition || !slices.Contains(ids, "BR-DE-21") {
				t.Errorf("%s %q forced: findings = %v", name, id, ids)
			}
		}
	}
}
