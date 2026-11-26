// Package validate checks CII and UBL invoices: CII invoices against the XML
// schema of their profile, the EN 16931 business rules, the
// Factur-X profile rules, and optionally the French (BR-FR) and German
// (BR-DE) rules, the profiles that France and Germany admit, the checks of
// Chorus Pro for French B2G invoices, the Peppol BIS Billing 3.0 rules
// (and those of Peppol BIS Self-Billing 3.0 for a self-billed invoice), the
// Dutch rules of NLCIUS (BR-NL), the Italian rules of the CIUS-IT (BR-IT),
// the Portuguese rules of the CIUS-PT, the Romanian rules of the CIUS-RO
// (BR-RO), the Slovak requirements of the eFaktúra beyond Peppol BIS
// (SK-*) and the Croatian rules of the CIUS-HR (HR-BR).
//
// The syntax is detected from the root element. A UBL 2.1 Invoice or
// CreditNote is checked against the OASIS UBL 2.1 XML schema of its document
// (the Schema rule set, with the same XSD-* rules as CII; the content of the
// UBL extensions is not checked, but for the Croatian extension HRFISK20Data,
// checked against its schema) and against the EN 16931 rules of the UBL
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
// A UBL invoice of the CIUS-PT (BT-24 urn:cen.eu:en16931:2017#compliant#
// urn:feap.gov.pt:CIUS-PT:2.1.1) is checked against the schematron of eSPap,
// which the validator of the Portuguese public administration applies
// instead of the artefacts of CEN/TC 434: in the EN 16931 rule set, the UBL
// syntax rules and the rules of EN 16931 in the versions of the CIUS-PT
// replace those of the CEN artefacts (its code lists accept the Portuguese
// document types and VAT categories, and its totals a tolerance of ±1.00);
// in the Portugal rule set, its own rules (BR-CIUS-PT, DT-CIUS-PT) and the
// check of the Portuguese VAT identifier (PT-NIF-*). See package
// internal/rules/pt.
//
// An invoice of the CIUS-RO (BT-24 urn:cen.eu:en16931:2017#compliant#
// urn:efactura.mfinante.ro:CIUS-RO:1.0.1, or another version of it, which
// BR-RO-001 reports) is checked against the rules of EN 16931 as any other,
// and in the Romania rule set against the schematron of ANAF (validation
// artefacts 1.0.9): for a UBL invoice, as published, on the UBL document;
// for a CII invoice, which the CIUS-RO admits but for which ANAF publishes
// no schematron, its transposition to CII. The check of the Romanian fiscal
// identification code (RO-CIF-*) follows. See package internal/rules/ro.
//
// A Slovak invoice of the eFaktúra (mandatory from 1 January 2027) is a
// Peppol BIS Billing 3.0 invoice, which BT-24 does not tell from another
// Peppol invoice: the Peppol rules apply to it whatever the rule sets, and
// the Slovakia rule set, which Options.Auto adds for a seller in Slovakia
// (BT-40 = SK), checks what the transposition of Peppol BIS to the Slovak
// legislation of the Finančná správa requires beyond it: the postal
// addresses, the electronic addresses of scheme 0245 (DIČ) of the Slovak
// parties, the VAT identifiers (IČ DPH), the IČO and the legal information
// of a Slovak seller, the date of the taxable supply and the reference of
// a corrective invoice (SK-01 to SK-15). See package internal/rules/sk.
//
// A self-billed invoice of Peppol BIS Self-Billing 3.0 (BT-24
// urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:selfbilling:3.0,
// type codes 389, 527 or 261), which the buyer issues on behalf of the
// seller, gets in the Peppol rule set the rules of the Self-Billing
// schematron (release 3.1.0) instead of those of Billing: the same rules
// but for its identifiers and type codes, without the national rules nor
// PEPPOL-EN16931-P0112. As for a Peppol invoice, they apply whatever the
// rule sets. Peppol BIS Self-Billing has a UBL syntax only. The national
// rule sets apply to it as to any invoice: the Slovak eFaktúra, for one,
// carries self-billed invoices and credit notes with the requirements of
// the Slovakia rule set.
//
// An invoice of the CIUS-HR (BT-24 urn:cen.eu:en16931:2017#compliant#
// urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0, or
// the identifier of the CIUS without its extension, which HR-BR-5 reports)
// is checked against the rules of EN 16931 as any other, and in the Croatia
// rule set against the schematron of the Croatian tax administration
// (schematron 1.0.0 of the specification 1.8), on the UBL document, with
// the check of the OIB of the electronic addresses (HR-OIB-01). The CIUS-HR
// is defined for UBL only: a CII invoice that declares it gets
// HR-SYNTAX-01. The UBL syntax rules of the CEN artefacts report, as
// warnings, the elements outside the binding of EN 16931 that the CIUS-HR
// uses (UBL-CR-001 for the extension HRFISK20Data, UBL-CR-006 for the issue
// time…). See package internal/rules/hr.
//
// A ZUGFeRD 1.0 invoice (rsm:CrossIndustryDocument, see cii.IsZUGFeRD1)
// gets a minimal structural check, part of the Schema rule set: a ZUGFeRD
// 1.0 specification identifier (ZF1-01), values that can be read (ZF1-02)
// and the data that identifies an invoice (ZF1-03). ZUGFeRD 1.0 predates
// EN 16931 and CII D16B: the other rule sets, written for invoices that
// follow them, do not apply, which the warning ZF1-04 says whatever the rule
// sets.
//
// A Facturae document (the Spanish format, versions 3.2 to 3.2.2, see
// package facturae) is checked against the XML schema of its version, part
// of the Schema rule set, and for values that can be read (FE-01). Facturae
// is not a syntax of EN 16931: the other rule sets do not apply, which the
// warning FE-02 says whatever the rule sets. The FACe rule set, for the
// invoices sent to the Spanish public administrations, requires its XAdES
// signature and checks it against the signature policy of Facturae (the
// ES-SIG rules of facturae.VerifySignature); without it, the signature is
// not checked, as an invoice between businesses need not be signed yet.
//
// A CII invoice of the BASIC or EXTENDED profile is checked against the
// Factur-X 1.09.2 schematron of its profile, the reference of these
// profiles, which only exist in Factur-X and ZUGFeRD: the CEN rules it
// keeps, with its severities, and its code lists (FX-SCH-A-*). So is an
// invoice of the EN 16931 profile when it is the XML of a Factur-X PDF
// (Options.Hybrid, which zerechnung.Check sets). Otherwise an EN 16931 CII
// invoice, whose specification identifier urn:cen.eu:en16931:2017 says
// nothing more than "EN 16931 without a CIUS", is checked against the CEN
// rules, as the KoSIT validator does. The schema follows: the Factur-X
// schema of the EN 16931 profile in a Factur-X PDF, the complete CII D16B
// schema of the CEN syntax binding otherwise, which accepts the elements of
// D16B outside the EN 16931 data set (the CEN syntax rules CII-SR report
// them, mostly as warnings) but a single preceding invoice reference (BG-3).
// See packages internal/rules/en16931, internal/rules/facturx and
// internal/schema.
//
// A FatturaPA document, the national format of the Italian SDI, is checked
// against the FatturaPA 1.2 schema (the Schema rule set, with the same XSD-*
// rules), which accepts the FPA12 and FPR12 documents, and against the
// controls of the SDI on its content that need none of its data, whatever
// the rule sets: rules named after their error code (00400, 00423…), listed
// in the Italy rule set. The other rule sets are written for EN 16931
// invoices and do not apply. To apply the CIUS-IT rules to the data of a
// FatturaPA invoice, read it with package fatturapa and validate the UBL
// that ubl.Marshal writes.
//
// A status of the life cycle of a French invoice (CDV), a CDAR D22B message
// (see package cdar), is checked against the CDAR D22B schema of the
// France_RFE package of the FNFE-MPE (the Schema rule set, with the same
// XSD-* rules), and against the rules BR-FR-CDV of its schematron
// (BR-FR-CDV-Schematron-CDAR 1.4.0.04) whatever the rule sets, as the
// controls of the SDI on a FatturaPA document: rules listed in the France
// rule set, source SourceCDV. The rules of the invoices do not apply. See
// package internal/rules/cdv.
//
// A report of the French e-reporting (flux 10, see package ereporting) is
// checked against the schemas of the flux 10 of France_RFE (the Schema rule
// set, with the same XSD-* rules), for the values that cannot be read
// (ER-01), such as a date that is not AAAAMMJJ, and against the rules of
// the PPF on its content, those of the schematron Flux10 of France_RFE
// 1.4.0.04, whatever the rule sets, as a CDAR status: rules listed in the
// France rule set, source SourceFlux10. A report is not an invoice: the
// rules of the invoices do not apply. See package internal/rules/flux10.
//
// Validation never stops at the first problem: every finding is collected in
// a Report. A document that is not well-formed XML, or neither a CII, a
// ZUGFeRD 1.0, a UBL, a FatturaPA nor a Facturae invoice, a CDAR status nor
// an e-reporting report, gives a report with a single syntax error.
package validate

import (
	"crypto/x509"
	"slices"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/ereporting"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/cdv"
	"github.com/ritoon/zerechnung/internal/rules/choruspro"
	"github.com/ritoon/zerechnung/internal/rules/de"
	"github.com/ritoon/zerechnung/internal/rules/en16931"
	"github.com/ritoon/zerechnung/internal/rules/facturx"
	"github.com/ritoon/zerechnung/internal/rules/flux10"
	"github.com/ritoon/zerechnung/internal/rules/fr"
	"github.com/ritoon/zerechnung/internal/rules/hr"
	"github.com/ritoon/zerechnung/internal/rules/it"
	"github.com/ritoon/zerechnung/internal/rules/nl"
	"github.com/ritoon/zerechnung/internal/rules/peppol"
	"github.com/ritoon/zerechnung/internal/rules/pt"
	"github.com/ritoon/zerechnung/internal/rules/ro"
	"github.com/ritoon/zerechnung/internal/rules/sk"
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
	SourceCDV     = rules.SourceCDV
	SourceFlux10  = rules.SourceFlux10
	SourceDE      = rules.SourceDE
	SourcePDF     = rules.SourcePDF

	SourceChorusPro = rules.SourceChorusPro
	SourcePeppol    = rules.SourcePeppol
	SourceNL        = rules.SourceNL
	SourceIT        = rules.SourceIT
	SourcePT        = rules.SourcePT
	SourceRO        = rules.SourceRO
	SourceSK        = rules.SourceSK
	SourceHR        = rules.SourceHR
	SourceSDI       = rules.SourceSDI
	SourceSignature = rules.SourceSignature
	SourceFacturae  = rules.SourceFacturae
)

// RuleSet selects groups of rules.
type RuleSet uint

// Rule sets.
const (
	Schema         RuleSet = 1 << iota // structure against the XML schema: of the profile (CII), UBL 2.1 (UBL), FatturaPA 1.2.4, Facturae of its version, CDAR D22B (status), flux 10 of France_RFE (e-reporting report); minimal structural check of ZUGFeRD 1.0
	EN16931                            // CEN business rules and code lists, and the UBL syntax rules for UBL
	FacturX                            // Factur-X / ZUGFeRD profile rules (CII only)
	France                             // French rules BR-FR (2026 reform) and the profiles the reform admits (FR-PROFILE-*); lists the rules BR-FR-CDV of the statuses and those of the schematron Flux10 of the e-reporting, which apply to a CDAR status and to an e-reporting report whatever the rule sets
	Germany                            // German rules BR-DE (XRechnung) and the format of the Leitweg-ID and of the GEBA (DE-LEITWEG-*, DE-GEBA-*)
	GermanyProfile                     // profiles the BMF accepts as e-invoices (DE-PROFILE-01: not MINIMUM nor BASIC WL)
	ChorusPro                          // French B2G checks for Chorus Pro (CPRO-*), see Options.PublicBuyer
	Peppol                             // Peppol BIS Billing 3.0 rules (PEPPOL-*) and their national rules (NO, DK, IT, SE, NL; for UBL also GR, IS, DE, SK); for a PEPPOL-SELF-BILLING invoice, the rules of Peppol BIS Self-Billing 3.0, without national rules
	Netherlands                        // Dutch rules of NLCIUS (BR-NL), and for UBL the SI-UBL 2.0 version check (SI-V20-INV-R000) and the rules of the G-account extension (BR-GA)
	Italy                              // Italian rules of the CIUS-IT (BR-IT) and of its extensions (BR-IT-DE), the check of its specification identifier (IT-PROFILE-01) and of the Partita IVA and Codice Fiscale (IT-PIVA-*, IT-CF-*)
	Portugal                           // Portuguese rules of the CIUS-PT schematron of eSPap (BR-CIUS-PT, DT-CIUS-PT), the check of the NIF (PT-NIF-*) and of the UBL syntax of a CIUS-PT invoice (PT-SYNTAX-01)
	FACe                               // Spanish B2G checks for FACe on a Facturae document: its XAdES-EPES signature is required and checked against the signature policy of Facturae (ES-SIG-*, see facturae.VerifySignature)
	Romania                            // Romanian rules of the CIUS-RO schematron of ANAF (BR-RO, BR-DEC-RO), transposed to CII for a CII invoice, and the check of the CIF (RO-CIF-*)
	Slovakia                           // Slovak requirements of the eFaktúra beyond Peppol BIS Billing 3.0 (SK-*): addresses, electronic addresses of scheme 0245 (DIČ), IČ DPH, IČO, legal information, date of the taxable supply, corrective invoice
	Croatia                            // Croatian rules of the CIUS-HR schematron of the tax administration (HR-BR), the check of the OIB of the electronic addresses (HR-OIB-01) and of the UBL syntax of a CIUS-HR invoice (HR-SYNTAX-01)

	// Default is the set used when Options.Rules is zero: schema, EN 16931
	// and profile rules. Whatever the rule sets, the German rules apply to
	// XRECHNUNG invoices, the Peppol rules to invoices whose BT-24 is the
	// Peppol BIS Billing 3.0 or Self-Billing 3.0 identifier, the Dutch rules
	// to NLCIUS invoices,
	// the Italian rules to CIUS-IT invoices, the Portuguese rules to
	// CIUS-PT invoices, the Romanian rules to CIUS-RO invoices and the
	// Croatian rules to CIUS-HR invoices (see RulesFor).
	Default = Schema | EN16931 | FacturX
)

// Options tune a validation. The zero value is ready to use.
type Options struct {
	// Profile overrides the profile read from BT-24. When BT-24 names an
	// edition of EN 16931-1 that the package does not support, such as
	// EN 16931-1:2026, the rules (which implement EN 16931-1:2017) only
	// apply if Profile is set; EN16931-EDITION-01 is reported in any case.
	Profile invoice.Profile
	// Rules selects the rule sets; zero means Default. The German rules for
	// an XRECHNUNG invoice, the Peppol rules for a Peppol or Peppol
	// Self-Billing invoice, the
	// Dutch rules for an NLCIUS invoice, the Italian rules for a CIUS-IT
	// invoice, the Portuguese rules for a CIUS-PT invoice, the Romanian
	// rules for a CIUS-RO invoice and the Croatian rules for a CIUS-HR
	// invoice are added whatever Rules: selecting other sets, such as
	// Default | France, never drops them.
	Rules RuleSet
	// Auto adds the national rules of the seller's country (BT-40): the
	// French rules (France) for a seller in France, the BMF profile check
	// (GermanyProfile) for a seller in Germany, the Slovak requirements
	// (Slovakia) for a seller in Slovakia, whose invoices declare Peppol
	// BIS Billing 3.0 in BT-24 like any other Peppol invoice. The German
	// rules BR-DE, the Peppol rules, the Dutch rules BR-NL, the Italian
	// rules BR-IT, the Portuguese rules, the Romanian rules BR-RO and the
	// Croatian rules HR-BR need no option: they apply to XRECHNUNG, Peppol,
	// NLCIUS, CIUS-IT, CIUS-PT, CIUS-RO and CIUS-HR invoices, which declare
	// themselves in BT-24. Auto never adds the Peppol, the Dutch, the
	// Italian, the Portuguese, the Romanian or the Croatian rules
	// otherwise: most Factur-X and ZUGFeRD invoices do not meet them (no
	// electronic address, no buyer reference, no Romanian county code).
	Auto bool
	// RejectBasicWL makes the French rules report the profile BASIC WL as an
	// error (FR-PROFILE-03). The reform admits BASIC WL for now, and no end
	// date has been published: set it to refuse BASIC WL already, or once
	// the profile is no longer admitted.
	RejectBasicWL bool
	// Hybrid tells that the XML is the invoice of a Factur-X or ZUGFeRD PDF.
	// An EN 16931 invoice is then checked against the Factur-X 1.09.2
	// schematron and schema of the EN 16931 profile instead of the CEN rules
	// alone and the CII D16B schema: its specification identifier (BT-24)
	// does not tell a Factur-X invoice from a plain EN 16931 CII invoice, the
	// PDF does. zerechnung.Check and zerechnung.Create set it; set it when
	// validating the XML of a Factur-X PDF extracted by other means. BASIC
	// and EXTENDED invoices are checked against their Factur-X schematron
	// whatever Hybrid.
	Hybrid bool
	// PDFA selects the PDF/A checks zerechnung.Check applies to the PDF
	// container: all of them by default (PDFAFull), or those that need not
	// read the pages and the font programs (PDFABasic). It has no effect on
	// the validation of an XML document.
	PDFA PDFALevel

	// PublicBuyer gives the settings of the public buyer in the Chorus Pro
	// directory, used by the ChorusPro rules.
	PublicBuyer PublicBuyer

	// ReceivedAt is the time the SdI receives a FatturaPA document: the date
	// of the invoice must not be later (00403). Under the FACe rule set, it
	// is the time FACe receives a Facturae document: the signing
	// certificate must be valid then, and the signing time not later. The
	// current time when zero.
	ReceivedAt time.Time

	// Trust gives what zerechnung.Check verifies the certificates of the
	// signatures of a PDF against, and, under the FACe rule set, what XML
	// verifies the certificate of the signature of a Facturae document
	// against. It has no effect on the validation of other XML documents.
	Trust TrustOptions
}

// TrustOptions give what the certificates of the signatures of a PDF are
// checked against, by zerechnung.Check and zerechnung.VerifySignatures.
// Nothing is fetched from the network. The zero value checks the
// signatures and the validity of their certificates, but neither their
// chain nor their revocation, beyond the data the document carries.
type TrustOptions struct {
	// Roots are the trusted certification authorities, such as those of
	// the EU trusted lists for qualified signatures and seals. Without
	// them, the chains are not checked.
	Roots *x509.CertPool
	// Intermediates are certificates that may complete the chains,
	// besides those the signatures and the document security store carry.
	Intermediates []*x509.Certificate
	// CRLs and OCSP are revocation data obtained by the caller (CRLs
	// parsed by x509.ParseRevocationList, OCSP responses in DER), used
	// with those of the document security store (DSS).
	CRLs []*x509.RevocationList
	OCSP [][]byte
	// At is the time of the verification: a certificate must still be
	// valid then when no time-stamp proves that the signature was made
	// earlier. time.Now() when zero.
	At time.Time
}

// PDFALevel selects the PDF/A checks of the PDF container (see
// Options.PDFA and zerechnung.ContainerCatalog).
type PDFALevel uint8

// PDF/A check levels.
const (
	// PDFAFull applies every check of zerechnung.ContainerCatalog: the
	// content streams of the pages and the font programs they use are read
	// (PDF-41 to PDF-69, PDF-71 and PDF-72, the requirements of ISO 19005-2
	// and 19005-3 checked clause by clause).
	PDFAFull PDFALevel = iota
	// PDFABasic skips PDF-41 to PDF-69, PDF-71 and PDF-72, as for a
	// document that declares PDF/A-4: no content stream, font program,
	// image, colour space or file structure is read, which takes most of
	// the time and memory of a check. The checks of the invoice attachment,
	// of the XMP metadata and of the PDF/A identification remain (PDF-01 to
	// PDF-30, PDF-40, PDF-70), with those that do not read the pages:
	// embedded fonts, JavaScript, actions, document information, associated
	// files (PDF-31 to PDF-39), and the filters of the streams of the
	// pages, their resources, the embedded files and the metadata (PDF-38,
	// without the other streams of the file). A PDF can pass them and still
	// not be PDF/A.
	PDFABasic
)

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

// XML validates a CII or UBL invoice, a ZUGFeRD 1.0 invoice, a FatturaPA
// document, a Facturae document, a CDAR status or a report of the French
// e-reporting.
func XML(data []byte, opts *Options) *Report {
	root, err := xmltree.Parse(data)
	return xmlReport(data, root, err, opts)
}

// xmlReport returns the report of XML on data, which xmltree.Parse read as
// root, or failed to read with err.
func xmlReport(data []byte, root *xmltree.Node, err error, opts *Options) *Report {
	if opts == nil {
		opts = &Options{}
	}
	if err != nil {
		r := &Report{}
		r.Add(Finding{Rule: "XML", Severity: Error, Source: SourceSyntax, Message: err.Error()})
		return r
	}
	r := Tree(root, opts)
	if facturae.IsFacturae(root) && opts.Rules&FACe != 0 && signedTree(root) {
		checkFacturaeSignature(data, opts, r)
	}
	return r
}

// Tree validates an already parsed CII or UBL invoice, a ZUGFeRD 1.0
// invoice, a FatturaPA document, a Facturae document, a CDAR status or a
// report of the French e-reporting. A tree does not
// keep what the signature of a Facturae document covers: under the FACe
// rule set, Tree only reports a missing signature, XML checks it.
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
	case fatturapa.IsFatturaPA(root):
		if set := opts.Rules; set == 0 || set&Schema != 0 {
			schema.CheckFatturaPA(&rules.Checker{Report: r, Source: SourceSyntax}, root)
		}
		// A FatturaPA document is Italian: the controls of the SdI apply
		// whatever the rule sets, as the Italian rules apply to a CIUS-IT
		// invoice.
		it.CheckSDI(&rules.Checker{Report: r, Source: SourceSDI}, root, opts.ReceivedAt)
		return r
	case facturae.IsFacturae(root):
		checkFacturae(root, opts, r)
		return r
	case rules.IsCDAR(root):
		checkCDAR(root, opts, r)
		return r
	case ereporting.IsReport(root):
		checkEReporting(root, opts, r)
		return r
	default:
		r.Add(Finding{Rule: "CII", Severity: Error, Source: SourceSyntax, Path: "/" + root.Name, Line: root.Line,
			Message: "the root element is neither rsm:CrossIndustryInvoice in namespace " + rules.NSRsm +
				", a ZUGFeRD 1.0 rsm:CrossIndustryDocument, a UBL 2.1 Invoice or CreditNote, a FatturaPA FatturaElettronica, a Facturae 3.2, 3.2.1 or 3.2.2 document, a CDAR status rsm:CrossDomainAcknowledgementAndResponse nor a French e-reporting Report"})
		return r
	}
	r.Profile = doc.Profile
	if checkEdition(doc, opts, r) {
		return r
	}
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
			// The validator of eSPap applies the CIUS-PT schematron, which
			// restates the UBL syntax rules and the rules of EN 16931 in
			// its own versions, instead of the CEN artefacts.
			if pt.Applies(c.Doc) {
				pt.CheckCEN(c)
				return
			}
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
	// CII view of a UBL invoice, and located in the UBL document; the rules
	// of the Italian extensions (BR-IT-DE) read the UBL document itself.
	run(Italy, SourceIT, it.Check)
	// The CIUS-PT schematron is a UBL one, checked on the UBL document.
	run(Portugal, SourcePT, pt.Check)
	// So is the CIUS-RO schematron; a CII invoice gets its transposition.
	run(Romania, SourceRO, ro.Check)
	// The Slovak requirements run on the CII view of a UBL invoice.
	run(Slovakia, SourceSK, sk.Check)
	// The CIUS-HR schematron is a UBL one too.
	run(Croatia, SourceHR, hr.Check)
	return r
}

// RulesFor returns the rule sets that apply to doc with opts: opts.Rules
// (Default when zero), plus the German rules for an XRECHNUNG invoice, the
// Peppol rules for a Peppol or Peppol Self-Billing invoice, the Dutch rules for an NLCIUS invoice,
// the Italian rules for a CIUS-IT invoice, the Portuguese rules for a
// CIUS-PT invoice, the Romanian rules for a CIUS-RO invoice and the
// Croatian rules for a CIUS-HR invoice whatever opts.Rules, plus the
// national rules of the seller's country with opts.Auto (France,
// GermanyProfile, Slovakia). The profile is the
// one of doc, which Options.Profile overrides.
func RulesFor(doc *rules.Doc, opts *Options) RuleSet {
	set := opts.Rules
	if set == 0 {
		set = Default
	}
	switch doc.Profile {
	case invoice.ProfileXRechnung:
		set |= Germany
	case invoice.ProfilePeppol, invoice.ProfilePeppolSelfBilling:
		set |= Peppol
	case invoice.ProfileNLCIUS:
		set |= Netherlands
	case invoice.ProfileCIUSIT:
		set |= Italy
	case invoice.ProfileCIUSPT:
		set |= Portugal
	case invoice.ProfileCIUSRO:
		set |= Romania
	case invoice.ProfileCIUSHR:
		set |= Croatia
	}
	if opts.Auto {
		switch doc.Agreement.Value("SellerTradeParty/PostalTradeAddress/CountryID") {
		case "FR":
			set |= France
		case "DE":
			set |= GermanyProfile
		case "SK":
			set |= Slovakia
		}
	}
	return set
}

// Catalog returns the rules implemented by the selected rule sets, for the
// CII and the UBL syntax. A French, German, Peppol or Dutch rule of both
// syntaxes appears once. The Portugal rule set lists the rules of the
// CIUS-PT and, of the rules of its schematron that it restates from EN 16931
// and the UBL syntax rules (which CheckCEN reports in the EN 16931 rule set
// for a CIUS-PT invoice), those that the CEN artefacts do not define.
func Catalog(set RuleSet) []RuleInfo {
	var out []RuleInfo
	add := func(s RuleSet, c []RuleInfo) {
		if set&s != 0 {
			out = append(out, c...)
		}
	}
	add(Schema, schema.Catalog)
	add(Schema, zugferd1Catalog)
	add(Schema, facturaeCatalog)
	add(Schema, ereportingCatalog)
	add(EN16931, editionCatalog)
	add(EN16931, en16931.Catalog)
	add(EN16931, newRules(en16931.CTCFRCatalog, en16931.Catalog))
	add(EN16931, ubl.Catalog)
	add(EN16931, newRules(ubl.CTCFRCatalog, slices.Concat(en16931.Catalog, en16931.CTCFRCatalog, ubl.Catalog)))
	add(FacturX, facturx.Catalog())
	add(France, fr.Catalog)
	add(France, newRules(fr.UBLCatalog, fr.Catalog))
	add(France, fr.ProfileCatalog)
	add(France, cdv.Catalog())
	add(France, flux10.Catalog())
	add(Germany, de.Catalog)
	add(Germany, newRules(de.UBLCatalog, de.Catalog))
	add(Germany, de.IdentifierCatalog)
	add(GermanyProfile, de.ProfileCatalog)
	add(ChorusPro, choruspro.Catalog)
	add(Peppol, peppol.Catalog)
	add(Peppol, newRules(peppol.UBLCatalog, peppol.Catalog))
	// The rules of Peppol BIS Self-Billing 3.1.0 are all rules of Billing,
	// listed once as Billing describes them (Self-Billing has another
	// message for PEPPOL-EN16931-R004).
	add(Peppol, newRules(peppol.SelfBillingCatalog, slices.Concat(peppol.Catalog, peppol.UBLCatalog)))
	add(Netherlands, nl.Catalog)
	add(Netherlands, newRules(nl.UBLCatalog, nl.Catalog))
	add(Italy, it.Catalog)
	add(Italy, it.SDICatalog)
	add(Portugal, pt.Catalog())
	// The rules of the CIUS-PT schematron that the CEN artefacts do not
	// define (BR-AA-*, UBL-SR-38…): CheckCEN reports them in the EN 16931
	// rule set for a CIUS-PT invoice.
	add(Portugal, newRules(pt.CENCatalog, slices.Concat(en16931.Catalog, ubl.Catalog)))
	add(FACe, faceCatalog())
	add(Romania, ro.Catalog())
	add(Slovakia, sk.Catalog)
	add(Croatia, hr.Catalog())
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
