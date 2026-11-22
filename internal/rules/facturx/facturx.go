// Package facturx implements the Factur-X / ZUGFeRD profile rules: what each
// profile requires and allows beyond EN 16931.
//
// For every profile it checks the specification identifier BT-24: present,
// known, and matching the profile the invoice is checked against. An
// obsolete spelling of a known identifier (FX-PROFILE-03) is an error where
// the Factur-X schematron applies, whose code list of BT-24 only holds the
// current identifiers, and a warning otherwise.
//
// For MINIMUM and BASIC WL, whose invoices are not complete EN 16931
// invoices and are therefore not checked by package en16931, it implements
// the official Factur-X 1.09 schematrons of these profiles
// (Factur-X_1.09_MINIMUM.sch and Factur-X_1.09_BASICWL.sch, FNFE-MPE /
// FeRD, 2026-06-10):
//
//   - the EN 16931 business rules they reuse, under their BR-* or CII-SR-*
//     identifier: mandatory elements, totals (BR-CO-11 to BR-CO-16 computed
//     from the header amounts, as BASIC WL has no lines), VAT breakdown and
//     VAT category rules at document level, decimals (BR-DEC-*);
//   - the BR-FXEXT-*-08 rules on the taxable amount of the VAT categories
//     G, K, O and Z that they share with EXTENDED (see extended.go);
//   - the structural assertions that the XML schema of the profile does not
//     express (cardinalities of variants, required scheme identifiers, code
//     values), under their FX-SCH-A-* identifier;
//   - the "not used" reports, as warnings (FX-NOT-USED).
//
// The rules follow the XPath of the schematron rather than the prose of the
// standard: string comparisons are exact (no trimming) unless the rule uses
// normalize-space(), amounts are exact decimals and round() rounds halves
// towards positive infinity. Assertions that cannot fail (BR-CO-05,
// BR-CO-06, BR-AF-09, BR-AG-09, CII-SR-464) and those the schema of the
// profile already enforces (FX-SCH-A-000021, the "exactly one" child
// counts) are not implemented. The code lists that need the ISO 3166,
// ISO 4217, ISO 6523, EAS, VATEX, UNTDID 4451 and 4461 lists are left
// out (FX-SCH-A-000031, 000036, 000040, 000045, 000162, 000181, 000196 and
// part of 000023).
//
// Two rules are additions: FX-DATE checks that dates in format 102 are real
// calendar days, and FX-MIN-CO-15 (a warning) applies BR-CO-15 to MINIMUM
// invoices, which the official MINIMUM schematron does not.
//
// For EXTENDED it implements the 61 BR-FXEXT-*
// assertions of the EXTENDED validation XSLT of Factur-X 1.09.2 (FeRD,
// ZUGFeRD 2.5.2), with the severities of the XSLT: line subtypes and groups
// (BT-X-8, BT-X-304), totals with tolerances, logistics service charges
// (BG-X-42) and charges on behalf of a third party (BG-34), VAT breakdown
// per category, rate and exemption reason, item attributes, notes and tax
// registrations (see extended.go). Package en16931 checks the CEN rules that
// the XSLT keeps for this profile. EXTENDED-CTC-FR has its own schematron,
// which package en16931 implements with the CEN rules it keeps.
//
// For BASIC, EN 16931 and EXTENDED, where the Factur-X 1.09.2 schematron of
// the profile applies (rules.Doc.FacturX: always for BASIC and EXTENDED, for
// EN 16931 in a Factur-X PDF only), it implements the code list assertions
// of the schematron, under their FX-SCH-A-* identifier: 48 for BASIC, 58 for
// EN 16931 and 209 for EXTENDED, generated from FACTUR-X_<profile>.sch and
// its codedb file by ./gen (see codedb.go). Their lists, those of FNFE-MPE
// and FeRD, replace the CEN code lists (BR-CL-*), which package en16931 does
// not report there. The other assertions of these schematrons are the CEN
// business rules, which package en16931 checks, and structural assertions
// that the XML schema of the profile already expresses. An EN 16931 invoice
// on its own gets the CEN code lists, as the KoSIT validator gives them:
// its identifier is the one of any CII invoice that follows EN 16931.
package facturx

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Check runs the rules on c.Doc and records the findings in c.Report.
func Check(c *rules.Checker) {
	d := c.Doc
	if d == nil || d.Root == nil {
		return
	}
	k := checker{c: c, d: d}
	switch d.Profile {
	case invoice.ProfileMinimum:
		k.p = mMin
	case invoice.ProfileBasicWL:
		k.p = mBWL
	case invoice.ProfileBasic:
		k.p = mBasic
	case invoice.ProfileEN16931:
		k.p = mEN
	case invoice.ProfileExtended, invoice.ProfileExtendedCTCFR:
		k.p = mExt
	default:
		k.p = mOther
	}
	k.profile()
	if k.p&mLight != 0 {
		k.light()
	}
	if k.p == mExt && d.Profile != invoice.ProfileExtendedCTCFR {
		k.extended()
	}
	if d.FacturX() {
		// BASIC, EXTENDED, and EN 16931 in a Factur-X PDF: the code lists
		// of the Factur-X schematron, which package en16931 leaves out.
		k.checkCodes(d.Profile)
	}
}

// checker holds the state of one run.
type checker struct {
	c *rules.Checker
	d *rules.Doc
	p uint8 // mask of the profile checked

	// Shortcuts set by light.
	seller, buyer, taxRep, payee, shipTo *xmltree.Node
	icc                                  *xmltree.Node // ram:InvoiceCurrencyCode
	tcc                                  *xmltree.Node // ram:TaxCurrencyCode
}

// fail records a finding for rule id when it applies to the profile.
func (k *checker) fail(id string, n *xmltree.Node, format string, args ...any) {
	r := byID(id)
	if r == nil {
		panic("facturx: unknown rule " + id)
	}
	if r.mask&k.p == 0 {
		return
	}
	if n == nil {
		n = k.d.Root
	}
	k.c.Fail(id, r.sev, n, format, args...)
}

// applies reports whether rule id applies to the profile checked. Callers
// test it before fail when the arguments would otherwise be allocated for a
// finding that fail drops.
func (k *checker) applies(id string) bool { return byID(id).mask&k.p != 0 }

// profileName returns the name of the profile checked.
func (k *checker) profileName() string { return k.d.Profile.String() }

// profile checks the specification identifier BT-24.
func (k *checker) profile() {
	d := k.d
	idNode := d.Context.Elem("GuidelineSpecifiedDocumentContextParameter").Elem("ID")
	loc := idNode
	if loc == nil {
		loc = d.Context
	}
	raw := text(idNode)
	urn := strings.TrimSpace(raw)
	if urn == "" {
		if k.p&mLight == 0 { // BR-01 reports it for MINIMUM and BASIC WL
			k.fail("FX-PROFILE-01", loc, "the specification identifier (BT-24) is missing: the Factur-X profile cannot be determined")
		}
		return
	}
	declared := invoice.ProfileFromURN(urn)
	if declared == invoice.ProfileUnknown {
		if p, fixed := legacyProfile(urn); p != invoice.ProfileUnknown {
			sev := rules.Error
			if !d.FacturX() {
				sev = rules.Warning // the CEN rules do not check BT-24
			}
			k.c.Fail("FX-PROFILE-03", sev, loc, "the specification identifier (BT-24) %q is an obsolete spelling of %q (profile %s); current validators do not recognize it", urn, fixed, p)
		} else {
			k.fail("FX-PROFILE-02", loc, "the specification identifier (BT-24) %q is not a known Factur-X, ZUGFeRD, XRechnung, Peppol, NLCIUS or CIUS-IT identifier (Factur-X profiles: %s, %s, %s, %s, %s)",
				urn, invoice.URNMinimum, invoice.URNBasicWL, invoice.URNBasic, invoice.URNEN16931, invoice.URNExtended)
		}
		return
	}
	switch {
	case d.Profile != invoice.ProfileUnknown && declared != d.Profile && !specializes(declared, d.Profile):
		k.fail("FX-SCH-A-000026", loc, "the specification identifier (BT-24) %q identifies profile %s, but the invoice is checked as %s (expected %q)", urn, declared, d.Profile, d.Profile.URN())
	case raw != urn && facturXProfile(d.Profile):
		k.fail("FX-SCH-A-000026", loc, "the specification identifier (BT-24) %q has leading or trailing white space: it must be exactly %q", raw, urn)
	}
}

// specializes reports whether invoices declaring profile declared conform to
// profile checked: EXTENDED-CTC-FR is a subset of EXTENDED, XRECHNUNG,
// PEPPOL, NLCIUS, CIUS-IT, CIUS-PT and CIUS-RO are CIUS of EN 16931.
func specializes(declared, checked invoice.Profile) bool {
	switch declared {
	case invoice.ProfileExtendedCTCFR:
		return checked == invoice.ProfileExtended
	case invoice.ProfileXRechnung, invoice.ProfilePeppol, invoice.ProfileNLCIUS, invoice.ProfileCIUSIT, invoice.ProfileCIUSPT, invoice.ProfileCIUSRO:
		return checked == invoice.ProfileEN16931
	}
	return false
}

// facturXProfile reports whether p is one of the five Factur-X profiles,
// whose schematrons compare BT-24 with their identifiers exactly.
func facturXProfile(p invoice.Profile) bool {
	switch p {
	case invoice.ProfileMinimum, invoice.ProfileBasicWL, invoice.ProfileBasic, invoice.ProfileEN16931, invoice.ProfileExtended:
		return true
	}
	return false
}

// legacyProfile recognizes obsolete spellings of the specification
// identifiers: wrong case, ":compliant:" or ":conformant:" for
// "#compliant#" or "#conformant#" (Factur-X 1.0 drafts), and a missing
// "urn:" after the separator. It returns the profile and the corrected
// identifier.
func legacyProfile(urn string) (invoice.Profile, string) {
	u := strings.ToLower(urn)
	const cen = "urn:cen.eu:en16931:2017"
	if rest, ok := strings.CutPrefix(u, cen); ok && len(rest) > 1 && (rest[0] == ':' || rest[0] == '#') {
		kind, tail, ok := strings.Cut(rest[1:], string(rest[0]))
		if ok && (kind == "compliant" || kind == "conformant") {
			if !strings.HasPrefix(tail, "urn:") {
				tail = "urn:" + tail
			}
			u = cen + "#" + kind + "#" + tail
		}
	}
	if u == urn {
		return invoice.ProfileUnknown, ""
	}
	return invoice.ProfileFromURN(u), u
}

// text returns the character data of n, untrimmed, as the string value of a
// leaf element in XPath; "" when n is nil.
func text(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	return n.Text
}

// filled reports whether n exists and its string value is not empty, as the
// XPath comparison of n with the empty string.
func filled(n *xmltree.Node) bool { return n != nil && n.Text != "" }

// count returns the number of children of n named name.
func count(n *xmltree.Node, name string) int {
	if n == nil {
		return 0
	}
	c := 0
	for _, ch := range n.Children {
		if ch.Name == name {
			c++
		}
	}
	return c
}

// display returns the conventional qualified name of n, such as
// "ram:SellerTradeParty".
func display(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	switch n.Space {
	case rules.NSRam:
		return "ram:" + n.Name
	case rules.NSRsm:
		return "rsm:" + n.Name
	case rules.NSUdt:
		return "udt:" + n.Name
	case rules.NSQdt:
		return "qdt:" + n.Name
	}
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}
