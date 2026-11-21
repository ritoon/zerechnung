// Package rules holds the types shared by the validation rule sets: findings,
// reports, and the parsed CII document the rules run on (for a UBL invoice,
// a CII view of it, see NewUBLDoc).
//
// Each rule set lives in its own package (internal/rules/en16931,
// internal/rules/ubl, internal/rules/facturx, internal/rules/fr,
// internal/rules/de, internal/rules/choruspro, internal/rules/peppol,
// internal/rules/nl, internal/rules/it) and exports:
//
//	func Check(c *rules.Checker)  // runs every rule of the set
//	var Catalog []rules.RuleInfo  // the rules the set implements
//
// The public package validate re-exports Finding, Severity and Report.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Severity is the severity of a finding. It maps the schematron flags
// "fatal", "warning" and "information".
type Severity int

// Severities.
const (
	Error   Severity = iota + 1 // the invoice is not valid
	Warning                     // the invoice is valid, but something should be fixed
	Info                        // information only
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	case Info:
		return "info"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Sources of findings.
const (
	SourceSyntax    = "syntax"     // XML well-formedness and CII structure
	SourceEN16931   = "EN 16931"   // CEN business rules, code lists and CII syntax rules
	SourceFacturX   = "Factur-X"   // profile rules of Factur-X / ZUGFeRD
	SourceFR        = "BR-FR"      // French rules (FNFE-MPE schematrons)
	SourceDE        = "BR-DE"      // German rules (XRechnung)
	SourcePDF       = "PDF"        // PDF container (PDF/A-3, attachment, XMP)
	SourceChorusPro = "Chorus Pro" // French B2G checks for Chorus Pro
	SourcePeppol    = "Peppol"     // Peppol BIS Billing 3.0 rules
	SourceNL        = "BR-NL"      // Dutch rules (NLCIUS, SI-UBL 2.0)
	SourceIT        = "BR-IT"      // Italian rules (CIUS-IT)
)

// Finding is a rule violation.
type Finding struct {
	Rule     string   `json:"rule"`           // rule identifier, as in the official artefacts ("BR-CO-10", "BR-FR-05_BT-22_PMT")
	Severity Severity `json:"severity"`       // error, warning or info
	Message  string   `json:"message"`        // human readable description, with the offending value when useful
	Path     string   `json:"path,omitempty"` // XPath-like location of the element, or a PDF object
	Line     int      `json:"line,omitempty"` // line of the element in the XML, when known
	Source   string   `json:"source"`         // rule set
}

func (f Finding) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s [%s] %s: %s", f.Severity, f.Source, f.Rule, f.Message)
	if f.Path != "" {
		b.WriteString(" (")
		b.WriteString(f.Path)
		if f.Line > 0 {
			fmt.Fprintf(&b, ", line %d", f.Line)
		}
		b.WriteString(")")
	}
	return b.String()
}

// RuleInfo describes a rule implemented by a rule set.
type RuleInfo struct {
	ID          string
	Severity    Severity
	Description string
}

// Report is the result of a validation.
type Report struct {
	Profile  invoice.Profile `json:"-"`
	Findings []Finding       `json:"findings"`
}

// Add appends findings to the report.
func (r *Report) Add(f ...Finding) { r.Findings = append(r.Findings, f...) }

// Valid reports whether the report holds no error.
func (r *Report) Valid() bool {
	for _, f := range r.Findings {
		if f.Severity == Error {
			return false
		}
	}
	return true
}

// Errors returns the findings of severity Error.
func (r *Report) Errors() []Finding { return r.filter(Error) }

// Warnings returns the findings of severity Warning.
func (r *Report) Warnings() []Finding { return r.filter(Warning) }

func (r *Report) filter(s Severity) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == s {
			out = append(out, f)
		}
	}
	return out
}

// Has reports whether the report holds a finding for the rule. The rule
// matches exactly, or as a prefix followed by "_" or "-" ("BR-FR-05" matches
// "BR-FR-05_BT-22_PMT").
func (r *Report) Has(rule string) bool {
	for _, f := range r.Findings {
		if f.Rule == rule || (strings.HasPrefix(f.Rule, rule) && len(f.Rule) > len(rule) &&
			(f.Rule[len(rule)] == '_' || f.Rule[len(rule)] == '-')) {
			return true
		}
	}
	return false
}

// Rules returns the distinct rule identifiers of the findings, sorted.
func (r *Report) Rules() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range r.Findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			out = append(out, f.Rule)
		}
	}
	sort.Strings(out)
	return out
}

// String returns one line per finding.
func (r *Report) String() string {
	if len(r.Findings) == 0 {
		return "valid: no finding"
	}
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// Namespace URIs of CII.
const (
	NSRsm = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	NSRam = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	NSUdt = "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
	NSQdt = "urn:un:unece:uncefact:data:standard:QualifiedDataType:100"
)

// Doc is a parsed CII invoice, with shortcuts to its main parts. Any of the
// shortcuts may be nil when the element is missing.
type Doc struct {
	Root        *xmltree.Node // rsm:CrossIndustryInvoice
	Profile     invoice.Profile
	Context     *xmltree.Node   // rsm:ExchangedDocumentContext
	Header      *xmltree.Node   // rsm:ExchangedDocument
	Transaction *xmltree.Node   // rsm:SupplyChainTradeTransaction
	Agreement   *xmltree.Node   // ram:ApplicableHeaderTradeAgreement
	Delivery    *xmltree.Node   // ram:ApplicableHeaderTradeDelivery
	Settlement  *xmltree.Node   // ram:ApplicableHeaderTradeSettlement
	Summation   *xmltree.Node   // ram:SpecifiedTradeSettlementHeaderMonetarySummation
	Lines       []*xmltree.Node // ram:IncludedSupplyChainTradeLineItem

	// UBL is the root of the UBL Invoice or CreditNote when the document is
	// a UBL invoice: Root is then a CII view of it (see NewUBLDoc), and the
	// findings are located in the UBL document. It is nil for a CII invoice.
	UBL    *xmltree.Node
	origin map[*xmltree.Node]*xmltree.Node

	// Hybrid tells that the invoice is the XML of a Factur-X or ZUGFeRD
	// PDF: the Factur-X schematron then applies to the EN 16931 profile too
	// (see FacturX).
	Hybrid bool
}

// FacturX reports whether the invoice is checked against the Factur-X
// 1.09.2 schematron of its profile rather than against the CEN rules alone.
//
// MINIMUM, BASIC WL, BASIC and EXTENDED only exist in Factur-X and ZUGFeRD:
// their specification identifier (BT-24) says that the invoice is a Factur-X
// invoice, and their official schematron is the Factur-X one. The
// identifier of the EN 16931 profile, urn:cen.eu:en16931:2017, is also the
// one of any CII invoice that follows EN 16931 without a CIUS: a CII
// invoice that carries it is an EN 16931 invoice, checked against the CEN
// rules as the KoSIT validator does, unless it is the XML of a Factur-X PDF
// (Hybrid). EXTENDED-CTC-FR, XRECHNUNG and PEPPOL have their own reference
// rules, and the UBL syntax has no Factur-X schematron.
func (d *Doc) FacturX() bool {
	if d == nil || d.UBL != nil {
		return false
	}
	switch d.Profile {
	case invoice.ProfileMinimum, invoice.ProfileBasicWL, invoice.ProfileBasic, invoice.ProfileExtended:
		return true
	case invoice.ProfileEN16931:
		return d.Hybrid
	}
	return false
}

// NewDoc wraps the root element of a CII invoice. The profile is read from
// BT-24 (leniently, see invoice.DetectProfile) unless profile is not
// ProfileUnknown.
func NewDoc(root *xmltree.Node, profile invoice.Profile) *Doc {
	d := &Doc{Root: root}
	d.Context = root.Elem("ExchangedDocumentContext")
	d.Header = root.Elem("ExchangedDocument")
	d.Transaction = root.Elem("SupplyChainTradeTransaction")
	d.Agreement = d.Transaction.Elem("ApplicableHeaderTradeAgreement")
	d.Delivery = d.Transaction.Elem("ApplicableHeaderTradeDelivery")
	d.Settlement = d.Transaction.Elem("ApplicableHeaderTradeSettlement")
	d.Summation = d.Settlement.Elem("SpecifiedTradeSettlementHeaderMonetarySummation")
	d.Lines = d.Transaction.Elems("IncludedSupplyChainTradeLineItem")
	if profile == invoice.ProfileUnknown {
		profile = invoice.DetectProfile(d.SpecificationID())
	}
	d.Profile = profile
	return d
}

// NewUBLDoc wraps the CII view of a UBL invoice: view is a CII tree that
// carries the business terms of the UBL document ubl, and origin maps each
// node of view to the UBL node it comes from. The rule sets written for CII
// run on the view; Checker.Fail locates their findings in the UBL document.
func NewUBLDoc(ubl, view *xmltree.Node, origin map[*xmltree.Node]*xmltree.Node, profile invoice.Profile) *Doc {
	d := NewDoc(view, profile)
	d.UBL = ubl
	d.origin = origin
	return d
}

// Origin returns the node of the original document that n stands for: the UBL
// node a node of a CII view comes from, n itself otherwise.
func (d *Doc) Origin(n *xmltree.Node) *xmltree.Node {
	if d == nil || d.origin == nil || n == nil {
		return n
	}
	if o, ok := d.origin[n]; ok {
		return o
	}
	return n
}

// GAccountExtension ends the specification identifier (BT-24) of an SI-UBL
// 2.0 invoice that uses the Dutch G-account extension:
// urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0 followed by
// it.
const GAccountExtension = invoice.GAccountExtension

// GAccount reports whether the invoice is a UBL invoice whose specification
// identifier, its first cbc:CustomizationID, contains GAccountExtension. The
// validation artefacts of the Dutch Peppol Authority for the extension
// (si-ubl-2.0-ext-gaccount.sch) then apply instead of those of SI-UBL 2.0:
// they add the rules BR-GA (package nl) and leave out three UBL syntax rules
// (package ubl). The NPa publishes no such artefacts for the CII syntax.
func (d *Doc) GAccount() bool {
	if d == nil || d.UBL == nil {
		return false
	}
	id := d.UBL.Elem("CustomizationID")
	return id != nil && strings.Contains(id.Text, GAccountExtension)
}

// SpecificationID returns BT-24.
func (d *Doc) SpecificationID() string {
	return d.Context.Value("GuidelineSpecifiedDocumentContextParameter/ID")
}

// Checker collects the findings of one rule set.
type Checker struct {
	Doc    *Doc
	Report *Report
	Source string
}

// Fail records a finding located at n (which may be nil).
func (c *Checker) Fail(rule string, sev Severity, n *xmltree.Node, format string, args ...any) {
	f := Finding{Rule: rule, Severity: sev, Source: c.Source}
	if len(args) > 0 {
		f.Message = fmt.Sprintf(format, args...)
	} else {
		f.Message = format
	}
	if n != nil {
		n = c.Doc.Origin(n)
		f.Path = n.Path()
		f.Line = n.Line
	}
	c.Report.Add(f)
}

// Assert records a finding when ok is false and returns ok.
func (c *Checker) Assert(ok bool, rule string, sev Severity, n *xmltree.Node, format string, args ...any) bool {
	if !ok {
		c.Fail(rule, sev, n, format, args...)
	}
	return ok
}

// Dec parses the trimmed text of n as a decimal. It returns false when n is
// nil or its text is not a decimal.
func Dec(n *xmltree.Node) (invoice.Decimal, bool) {
	if n == nil {
		return invoice.Decimal{}, false
	}
	d, err := invoice.ParseDecimal(n.Text)
	return d, err == nil
}

// SumDec returns the sum of the decimal values of nodes; values that do not
// parse are ignored. The result is set (zero) for no node.
func SumDec(nodes []*xmltree.Node) invoice.Decimal {
	s := invoice.NewDecimal(0, 0)
	for _, n := range nodes {
		if d, ok := Dec(n); ok {
			s = s.Add(d)
		}
	}
	return s
}
