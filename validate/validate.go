// Package validate checks CII and UBL invoices: CII invoices against the XML
// schema of their Factur-X profile, the EN 16931 business rules, the
// Factur-X profile rules, and optionally the French (BR-FR) and German
// (BR-DE) rules, the profiles that France and Germany admit, the checks of
// Chorus Pro for French B2G invoices, the Peppol BIS Billing 3.0 rules, the
// Dutch rules of NLCIUS (BR-NL) and the Italian rules of the CIUS-IT
// (BR-IT).
//
// The syntax is detected from the root element. A UBL 2.1 Invoice or
// CreditNote is checked against the OASIS UBL 2.1 XML schema of its document
// (the Schema rule set, with the same XSD-* rules as CII; the content of the
// UBL extensions is not checked) and against the EN 16931 rules of the UBL
// artefacts: the business rules and code lists, and the UBL syntax rules
// UBL-CR, UBL-SR and UBL-DT. The Factur-X profile rules, which are CII
// rules, do not apply. The French rules implement the UBL schematron of
// BR-FR (Flux 2), the German rules the XRechnung UBL schematron, the
// Peppol rules the UBL schematron of Peppol BIS Billing 3.0 and the Dutch
// rules the SI-UBL 2.0 schematron, with its G-account extension, on the UBL
// document itself, with their own rule identifiers, contexts and messages
// (see the UBLCatalog of each package). The Chorus Pro and the Italian
// rules run on a CII view of the UBL document. All findings are located in the UBL document.
//
// A ZUGFeRD 1.0 invoice (rsm:CrossIndustryDocument, see cii.IsZUGFeRD1)
// gets a minimal structural check, part of the Schema rule set: a ZUGFeRD
// 1.0 specification identifier (ZF1-01), values that can be read (ZF1-02)
// and the data that identifies an invoice (ZF1-03). ZUGFeRD 1.0 predates
// EN 16931 and CII D16B: the other rule sets, written for invoices that
// follow them, do not apply, which the warning ZF1-04 says whatever the rule
// sets.
//
// A CII invoice of the BASIC or EXTENDED profile is checked against the
// Factur-X 1.09.2 schematron of its profile, the reference of these
// profiles, which only exist in Factur-X and ZUGFeRD: the CEN rules it
// keeps, with its severities, and its code lists (FX-SCH-A-*). So is an
// invoice of the EN 16931 profile when it is the XML of a Factur-X PDF
// (Options.Hybrid, which zerechnung.Check sets). Otherwise an EN 16931 CII
// invoice, whose specification identifier urn:cen.eu:en16931:2017 says
// nothing more than "EN 16931 without a CIUS", is checked against the CEN
// rules, as the KoSIT validator does. See packages internal/rules/en16931
// and internal/rules/facturx.
//
// Validation never stops at the first problem: every finding is collected in
// a Report. A document that is not well-formed XML, or neither a CII, a
// ZUGFeRD 1.0 nor a UBL invoice, gives a report with a single syntax error.
package validate

import (
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/choruspro"
	"github.com/ritoon/zerechnung/internal/rules/de"
	"github.com/ritoon/zerechnung/internal/rules/en16931"
	"github.com/ritoon/zerechnung/internal/rules/facturx"
	"github.com/ritoon/zerechnung/internal/rules/fr"
	"github.com/ritoon/zerechnung/internal/rules/it"
	"github.com/ritoon/zerechnung/internal/rules/nl"
	"github.com/ritoon/zerechnung/internal/rules/peppol"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Finding is a rule violation.
type Finding = rules.Finding

// Severity is the severity of a finding.
type Severity = rules.Severity

// Report is the result of a validation.
type Report = rules.Report

// RuleInfo describes an implemented rule.
type RuleInfo = rules.RuleInfo

// Severities.
const (
	Error   = rules.Error
	Warning = rules.Warning
	Info    = rules.Info
)

// Sources of findings.
const (
	SourceSyntax  = rules.SourceSyntax
	SourceEN16931 = rules.SourceEN16931
	SourceFacturX = rules.SourceFacturX
	SourceFR      = rules.SourceFR
	SourceDE      = rules.SourceDE
	SourcePDF     = rules.SourcePDF

	SourceChorusPro = rules.SourceChorusPro
	SourcePeppol    = rules.SourcePeppol
	SourceNL        = rules.SourceNL
	SourceIT        = rules.SourceIT
)

// RuleSet selects groups of rules.
type RuleSet uint

// Rule sets.
const (
	Schema         RuleSet = 1 << iota // structure against the XML schema: of the profile (CII), UBL 2.1 (UBL); minimal structural check of ZUGFeRD 1.0
	EN16931                            // CEN business rules and code lists, and the UBL syntax rules for UBL
	FacturX                            // Factur-X / ZUGFeRD profile rules (CII only)
	France                             // French rules BR-FR (2026 reform) and the profiles the reform admits (FR-PROFILE-*)
	Germany                            // German rules BR-DE (XRechnung) and the format of the Leitweg-ID and of the GEBA (DE-LEITWEG-*, DE-GEBA-*)
	GermanyProfile                     // profiles the BMF accepts as e-invoices (DE-PROFILE-01: not MINIMUM nor BASIC WL)
	ChorusPro                          // French B2G checks for Chorus Pro (CPRO-*), see Options.PublicBuyer
	Peppol                             // Peppol BIS Billing 3.0 rules (PEPPOL-*) and their national rules (NO, DK, IT, SE, NL; for UBL also GR, IS, DE)
	Netherlands                        // Dutch rules of NLCIUS (BR-NL), and for UBL the SI-UBL 2.0 version check (SI-V20-INV-R000) and the rules of the G-account extension (BR-GA)
	Italy                              // Italian rules of the CIUS-IT (BR-IT), the check of its specification identifier (IT-PROFILE-01) and of the Partita IVA and Codice Fiscale (IT-PIVA-*, IT-CF-*)

	// Default is the set used when Options.Rules is zero: schema, EN 16931
	// and profile rules. Whatever the rule sets, the German rules apply to
	// XRECHNUNG invoices, the Peppol rules to invoices whose BT-24 is the
	// Peppol BIS Billing 3.0 identifier, the Dutch rules to NLCIUS invoices
	// and the Italian rules to CIUS-IT invoices (see RulesFor).
	Default = Schema | EN16931 | FacturX
)

// Options tune a validation. The zero value is ready to use.
type Options struct {
	// Profile overrides the profile read from BT-24.
	Profile invoice.Profile
	// Rules selects the rule sets; zero means Default. The German rules for
	// an XRECHNUNG invoice, the Peppol rules for a Peppol invoice, the
	// Dutch rules for an NLCIUS invoice and the Italian rules for a CIUS-IT
	// invoice are added whatever Rules: selecting other sets, such as
	// Default | France, never drops them.
	Rules RuleSet
	// Auto adds the national rules of the seller's country (BT-40): the
	// French rules (France) for a seller in France, the BMF profile check
	// (GermanyProfile) for a seller in Germany. The German rules BR-DE, the
	// Peppol rules, the Dutch rules BR-NL and the Italian rules BR-IT need no
	// option: they apply to XRECHNUNG, Peppol, NLCIUS and CIUS-IT invoices,
	// which declare themselves in BT-24. Auto never adds the Peppol, the
	// Dutch or the Italian rules otherwise: most Factur-X and ZUGFeRD
	// invoices do not meet them (no electronic address, no buyer reference).
	Auto bool
	// RejectBasicWL makes the French rules report the profile BASIC WL as an
	// error (FR-PROFILE-03). The reform admits BASIC WL for now, and no end
	// date has been published: set it to refuse BASIC WL already, or once
	// the profile is no longer admitted.
	RejectBasicWL bool
	// Hybrid tells that the XML is the invoice of a Factur-X or ZUGFeRD PDF.
	// An EN 16931 invoice is then checked against the Factur-X 1.09.2
	// schematron of the EN 16931 profile instead of the CEN rules alone:
	// its specification identifier (BT-24) does not tell a Factur-X invoice
	// from a plain EN 16931 CII invoice, the PDF does. zerechnung.Check and
	// zerechnung.Create set it; set it when validating the XML of a Factur-X
	// PDF extracted by other means. BASIC and EXTENDED invoices are checked
	// against their Factur-X schematron whatever Hybrid.
	Hybrid bool

	// PublicBuyer gives the settings of the public buyer in the Chorus Pro
	// directory, used by the ChorusPro rules.
	PublicBuyer PublicBuyer
}

// PublicBuyer holds the settings of a public structure in the Chorus Pro
// directory that change the ChorusPro rules. The zero value requires
// neither a service code nor an engagement number.
type PublicBuyer struct {
	// ServiceCodeRequired: the structure requires a service code, carried by
	// the buyer reference (BT-10).
	ServiceCodeRequired bool
	// EngagementRequired: the structure requires an engagement number,
	// carried by the purchase order reference (BT-13); Chorus Pro also
	// accepts the contract reference (BT-12) instead.
	EngagementRequired bool
}

// XML validates a CII or UBL invoice, or a ZUGFeRD 1.0 invoice.
func XML(data []byte, opts *Options) *Report {
	if opts == nil {
		opts = &Options{}
	}
	root, err := xmltree.Parse(data)
	if err != nil {
		r := &Report{}
		r.Add(Finding{Rule: "XML", Severity: Error, Source: SourceSyntax, Message: err.Error()})
		return r
	}
	return Tree(root, opts)
}

// Tree validates an already parsed CII or UBL invoice, or a ZUGFeRD 1.0
// invoice.
func Tree(root *xmltree.Node, opts *Options) *Report {
	if opts == nil {
		opts = &Options{}
	}
	r := &Report{}
	var doc *rules.Doc
	switch {
	case root.Name == "CrossIndustryInvoice" && root.Space == rules.NSRsm:
		doc = rules.NewDoc(root, opts.Profile)
		doc.Hybrid = opts.Hybrid
	case ubl.IsUBL(root):
		doc = ubl.View(root, opts.Profile)
	case cii.IsZUGFeRD1(root):
		checkZUGFeRD1(root, opts, r)
		return r
	default:
		r.Add(Finding{Rule: "CII", Severity: Error, Source: SourceSyntax, Path: "/" + root.Name, Line: root.Line,
			Message: "the root element is neither rsm:CrossIndustryInvoice in namespace " + rules.NSRsm +
				", a ZUGFeRD 1.0 rsm:CrossIndustryDocument, nor a UBL 2.1 Invoice or CreditNote"})
		return r
	}
	r.Profile = doc.Profile
	set := RulesFor(doc, opts)
	isUBL := doc.UBL != nil
	run := func(s RuleSet, source string, check func(*rules.Checker)) {
		if set&s != 0 {
			check(&rules.Checker{Doc: doc, Report: r, Source: source})
		}
	}
	run(Schema, SourceSyntax, schema.Check)
	if isUBL {
		run(EN16931, SourceEN16931, func(c *rules.Checker) {
			en16931.Check(c)
			ubl.Check(c)
		})
	} else {
		run(EN16931, SourceEN16931, en16931.Check)
		run(FacturX, SourceFacturX, facturx.Check)
	}
	run(France, SourceFR, func(c *rules.Checker) {
		fr.Check(c)
		fr.CheckProfile(c, opts.RejectBasicWL)
	})
	run(Germany, SourceDE, func(c *rules.Checker) {
		de.Check(c)
		de.CheckIdentifiers(c)
	})
	run(GermanyProfile, SourceDE, de.CheckProfile)
	run(ChorusPro, SourceChorusPro, func(c *rules.Checker) {
		choruspro.Check(c, choruspro.Settings{
			ServiceCodeRequired: opts.PublicBuyer.ServiceCodeRequired,
			EngagementRequired:  opts.PublicBuyer.EngagementRequired,
		})
	})
	// The Peppol rules of a UBL invoice are those of the UBL schematron,
	// checked on the UBL document: the CII rules would give false findings
	// on a CII view.
	run(Peppol, SourcePeppol, peppol.Check)
	// The Dutch rules of a UBL invoice are those of the SI-UBL 2.0
	// schematron, also checked on the UBL document.
	run(Netherlands, SourceNL, nl.Check)
	// The CIUS-IT has no schematron of its own: its rules are checked on the
	// CII view of a UBL invoice, and located in the UBL document.
	run(Italy, SourceIT, it.Check)
	return r
}

// RulesFor returns the rule sets that apply to doc with opts: opts.Rules
// (Default when zero), plus the German rules for an XRECHNUNG invoice, the
// Peppol rules for a Peppol invoice, the Dutch rules for an NLCIUS invoice
// and the Italian rules for a CIUS-IT invoice whatever opts.Rules, plus the
// national rules of the seller's country with opts.Auto. The profile is the
// one of doc, which Options.Profile overrides.
func RulesFor(doc *rules.Doc, opts *Options) RuleSet {
	set := opts.Rules
	if set == 0 {
		set = Default
	}
	switch doc.Profile {
	case invoice.ProfileXRechnung:
		set |= Germany
	case invoice.ProfilePeppol:
		set |= Peppol
	case invoice.ProfileNLCIUS:
		set |= Netherlands
	case invoice.ProfileCIUSIT:
		set |= Italy
	}
	if opts.Auto {
		switch doc.Agreement.Value("SellerTradeParty/PostalTradeAddress/CountryID") {
		case "FR":
			set |= France
		case "DE":
			set |= GermanyProfile
		}
	}
	return set
}

// Catalog returns the rules implemented by the selected rule sets, for the
// CII and the UBL syntax. A French, German, Peppol or Dutch rule of both
// syntaxes appears once.
func Catalog(set RuleSet) []RuleInfo {
	var out []RuleInfo
	add := func(s RuleSet, c []RuleInfo) {
		if set&s != 0 {
			out = append(out, c...)
		}
	}
	add(Schema, schema.Catalog)
	add(Schema, zugferd1Catalog)
	add(EN16931, en16931.Catalog)
	add(EN16931, newRules(en16931.CTCFRCatalog, en16931.Catalog))
	add(EN16931, ubl.Catalog)
	add(FacturX, facturx.Catalog)
	add(France, fr.Catalog)
	add(France, newRules(fr.UBLCatalog, fr.Catalog))
	add(France, fr.ProfileCatalog)
	add(Germany, de.Catalog)
	add(Germany, newRules(de.UBLCatalog, de.Catalog))
	add(Germany, de.IdentifierCatalog)
	add(GermanyProfile, de.ProfileCatalog)
	add(ChorusPro, choruspro.Catalog)
	add(Peppol, peppol.Catalog)
	add(Peppol, newRules(peppol.UBLCatalog, peppol.Catalog))
	add(Netherlands, nl.Catalog)
	add(Netherlands, newRules(nl.UBLCatalog, nl.Catalog))
	add(Italy, it.Catalog)
	return out
}

// newRules returns the rules of c whose identifier is not in old: the rules
// of a UBL schematron that the CII one does not define, or those of the
// EXTENDED-CTC-FR schematron that the EN 16931 one does not define. A rule
// of both is listed once, as the CII (or EN 16931) schematron describes it
// (the Peppol DK-R-003 is a warning there, an error in the UBL schematron).
func newRules(c, old []RuleInfo) []RuleInfo {
	seen := make(map[string]bool, len(old))
	for _, r := range old {
		seen[r.ID] = true
	}
	var out []RuleInfo
	for _, r := range c {
		if !seen[r.ID] {
			out = append(out, r)
		}
	}
	return out
}
