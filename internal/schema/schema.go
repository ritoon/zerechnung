// Package schema checks the structure of an invoice against its XML schema:
// a CII invoice against the schema of its profile, a UBL invoice against the
// OASIS UBL 2.1 schema of its document (Invoice or CreditNote), a FatturaPA
// document against the FatturaPA 1.2 schema, a Facturae document against
// the Facturae schema of its version. It checks the allowed
// elements, their order, cardinalities, attributes, namespaces and simple
// types.
//
// The schemas are compiled ahead of time by the generator in ./gen, so
// validation needs no schema engine and does not allocate while the document
// is valid:
//
//   - tables_gen.go holds the schemas of the five Factur-X profiles
//     (MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED). Types that are
//     identical in several profiles are shared. PEPPOL invoices are checked
//     against the EN 16931 schema and EXTENDED-CTC-FR invoices against the
//     EXTENDED schema. An invoice whose profile is unknown is checked
//     against the EXTENDED schema, the most permissive, with a warning.
//   - cii_d16b_tables_gen.go holds the complete UN/CEFACT CII D16B schema
//     of the KoSIT validator configuration for XRechnung 3.0.2, against
//     which XRECHNUNG invoices are checked, as KoSIT does. It accepts the
//     elements of D16B outside the EN 16931 data set, which the EN 16931
//     syntax rules (CII-SR) report with the severities of the KoSIT
//     configuration, and it allows a single ram:InvoiceReferencedDocument,
//     where the Factur-X schema allows several.
//   - ubl_tables_gen.go holds the UBL 2.1 schemas of Invoice and CreditNote
//     (the runtime schemas of the OASIS package). The content of the UBL
//     extensions (ext:ExtensionContent, a lax wildcard) is not checked: only
//     the namespace and the number of its elements are.
//   - fatturapa_tables_gen.go holds the FatturaPA 1.2.4 schema of the
//     Agenzia delle Entrate, which accepts the FPA12 and FPR12 documents. It
//     differs from the schema 1.2.3, in force until 10/01/2027, only by the
//     tax regime RF21. The content of the XML signature (ds:Signature) is
//     not checked.
//   - facturae_tables_gen.go holds the schemas of the Spanish Facturae
//     format, versions 3.2, 3.2.1 and 3.2.2, against which CheckFacturae
//     checks a Facturae document. The XML signature (ds:Signature) and the
//     content of the Extensions elements are not checked.
//
// EXTENDED-CTC-FR invoices keep the Factur-X EXTENDED schema although the
// FNFE-MPE package validates them against the complete CII D22B schema: the
// EXTENDED schema is included in D22B (TestSchemaInclusion checks it on the
// D22B tables, compiled for the tests only), so every invoice it accepts
// passes the FNFE schema, and the FNFE EXTENDED-CTC-FR schematron restricts
// D22B back to the EXTENDED data set, partly with fatal rules (CII-SR-009,
// CII-SR-477 to 494...) that the D22B schema alone would let through.
package schema

//go:generate go run ./gen -o tables_gen.go ${FX_XSD_DIR}
//go:generate go run ./gen -cii D16B -o cii_d16b_tables_gen.go
//go:generate go run ./gen -cii D22B -o cii_d22b_tables_gen_test.go
//go:generate go run ./gen -ubl -o ubl_tables_gen.go
//go:generate go run ./gen -fatturapa -o fatturapa_tables_gen.go
//go:generate go run ./gen -facturae -o facturae_tables_gen.go

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Rule identifiers.
const (
	RuleProfile          = "XSD-PROFILE"           // unknown profile: checked against EXTENDED (warning)
	RuleRoot             = "XSD-ROOT"              // the root element is not the one of the schema
	RuleNamespace        = "XSD-NAMESPACE"         // element in the wrong namespace
	RuleUnexpected       = "XSD-UNEXPECTED"        // element not allowed in its parent
	RuleOrder            = "XSD-ORDER"             // element allowed in its parent, but misplaced
	RuleMissing          = "XSD-MISSING"           // required element missing
	RuleCount            = "XSD-COUNT"             // element repeated more than allowed
	RuleAttribute        = "XSD-ATTRIBUTE"         // attribute not declared
	RuleAttributeMissing = "XSD-ATTRIBUTE-MISSING" // required attribute missing
	RuleType             = "XSD-TYPE"              // value not valid for its simple type
	RuleText             = "XSD-TEXT"              // text in an element that only holds elements
)

// Catalog lists the rules implemented by this package.
var Catalog = []rules.RuleInfo{
	{ID: RuleProfile, Severity: rules.Warning, Description: "The profile (BT-24) is unknown: the structure is checked against the EXTENDED schema, the most permissive."},
	{ID: RuleRoot, Severity: rules.Error, Description: "The root element shall be rsm:CrossIndustryInvoice (CII), Invoice or CreditNote (UBL 2.1), p:FatturaElettronica (FatturaPA), or Facturae in the namespace of Facturae 3.2, 3.2.1 or 3.2.2."},
	{ID: RuleNamespace, Severity: rules.Error, Description: "Each element shall be in the namespace defined by the schema (rsm, ram, udt or qdt in CII; cac, cbc or ext in UBL; none below the root in FatturaPA and Facturae)."},
	{ID: RuleUnexpected, Severity: rules.Error, Description: "Each element shall be allowed in its parent by the schema (for CII, the schema of the profile; the CII D16B schema for XRECHNUNG)."},
	{ID: RuleOrder, Severity: rules.Error, Description: "Child elements shall appear in the order defined by the schema."},
	{ID: RuleMissing, Severity: rules.Error, Description: "Elements required by the schema (minOccurs > 0) shall be present."},
	{ID: RuleCount, Severity: rules.Error, Description: "An element shall not occur more often than allowed by the schema (maxOccurs)."},
	{ID: RuleAttribute, Severity: rules.Error, Description: "Only the attributes declared by the schema are allowed."},
	{ID: RuleAttributeMissing, Severity: rules.Error, Description: "Attributes required by the schema shall be present."},
	{ID: RuleType, Severity: rules.Error, Description: "Element and attribute values shall match their simple type (xs:decimal, xs:double, xs:boolean, xs:base64Binary, xs:date, xs:time, xs:dateTime, xs:language, enumerations, fixed values, lengths, patterns, bounds)."},
	{ID: RuleText, Severity: rules.Error, Description: "Elements whose content is made of elements shall not contain text."},
}

// simpleKind is the lexical space checked for a simple type.
type simpleKind uint8

const (
	kString   simpleKind = iota // xs:string, xs:token and other unconstrained string types
	kDecimal                    // xs:decimal
	kInteger                    // xs:integer and its derived types
	kBoolean                    // xs:boolean
	kBase64                     // xs:base64Binary
	kDate                       // xs:date
	kTime                       // xs:time
	kDateTime                   // xs:dateTime
	kDouble                     // xs:double and xs:float
)

// contentKind is the content model of a complex type.
type contentKind uint8

const (
	contentEmpty    contentKind = iota // no element, no text
	contentElements                    // a sequence or a choice of elements
	contentSimple                      // text of a simple type, with attributes
	contentAny                         // elements matching a wildcard (xs:any), content not checked
	contentSkip                        // element of a schema that is not compiled (ds:Signature): attributes and content not checked
)

// unbounded is the maxOccurs value meaning "unbounded".
const unbounded = 255

// Profiles of the tables, from the smallest to the largest.
const (
	pMinimum = iota
	pBasicWL
	pBasic
	pEN16931
	pExtended
	numProfiles
)

var profileNames = [numProfiles]string{"MINIMUM", "BASIC WL", "BASIC", "EN 16931", "EXTENDED"}

// ctype is a compiled element type.
type ctype struct {
	name       string // type name, with the conventional prefix ("ram:TradePartyType")
	content    contentKind
	choice     bool  // the elements form a choice rather than a sequence
	min, max   uint8 // occurrences of the choice, or of the wildcard
	simple     simpleKind
	other      uint8  // wildcard: index in namespaces of the namespace excluded by ##other
	facets     uint16 // index in facetTable, 0 = none
	simpleName string // name of the simple type, for messages
	first, n   uint16 // particles[first : first+n]
	afirst, an uint16 // attributes[afirst : afirst+an]
	profiles   uint8  // bit mask of the profiles using this type
	// alternatives[altFirst : altFirst+altN] are the choices of sequences
	// that the generator flattened into the sequence of the type.
	altFirst, altN uint16
	def            string // default value of the element, checked when it is empty
}

// alternative is one sequence of a choice nested in the sequence of a type
// (xs:choice of xs:sequence, as in the Anagrafica of FatturaPA:
// Denominazione, or Nome and Cognome). Its particles, first to first+n-1
// in the particles of the type, are flattened into the sequence with
// minOccurs 0; need has the bit i set when particle first+i is required in
// the alternative. Exactly one alternative of each group must be present.
type alternative struct {
	group, first, n, need uint8
}

// particle is a child element of a type.
type particle struct {
	name     string
	ns       uint8  // index in namespaces
	typ      uint16 // index in types
	min, max uint8  // max is unbounded for maxOccurs="unbounded"
}

// attribute is an attribute declaration.
type attribute struct {
	name     string
	kind     simpleKind
	facets   uint16
	required bool
}

// facets restrict a simple type.
type facets struct {
	enum           []string
	patterns       []string
	minLen, maxLen int // -1 = none
	collapse       bool
	// minIncl and maxIncl are the inclusive bounds of a decimal, integer or
	// date type ("" = none).
	minIncl, maxIncl string
}

// tables is a compiled schema set: the Factur-X profiles or UBL 2.1.
type tables struct {
	namespaces []string
	types      []ctype
	particles  []particle
	attributes []attribute
	facetTable []facets
	regexps    [][]*regexp.Regexp // compiled patterns of facetTable
	profiles   bool               // types carry the profiles using them (Factur-X)
	// alternatives of the choices nested in sequences (FatturaPA).
	alts []alternative
}

func newTables(ns []string, ts []ctype, ps []particle, as []attribute, fs []facets, profiles bool) *tables {
	t := &tables{namespaces: ns, types: ts, particles: ps, attributes: as, facetTable: fs, profiles: profiles}
	t.regexps = make([][]*regexp.Regexp, len(fs))
	for i, f := range fs {
		for _, p := range f.patterns {
			t.regexps[i] = append(t.regexps[i], regexp.MustCompile("^(?:"+p+")$"))
		}
	}
	return t
}

var (
	ciiTables  = newTables(namespaces[:], types[:], particles[:], attributes[:], facetTable[:], true)
	d16bTables = newTables(d16bNamespaces[:], d16bTypes[:], d16bParticles[:], d16bAttributes[:], d16bFacetTable[:], false)
	ublTables  = newTables(ublNamespaces[:], ublTypes[:], ublParticles[:], ublAttributes[:], ublFacetTable[:], false)
	fpaTables  = func() *tables {
		t := newTables(fpaNamespaces[:], fpaTypes[:], fpaParticles[:], fpaAttributes[:], fpaFacetTable[:], false)
		t.alts = fpaAlternatives[:]
		return t
	}()
	facturaeTables = func() *tables {
		t := newTables(facturaeNamespaces[:], facturaeTypes[:], facturaeParticles[:], facturaeAttributes[:], facturaeFacetTable[:], false)
		t.alts = facturaeAlternatives[:]
		return t
	}()
)

// profileIndex returns the Factur-X schema used for an invoice profile. The
// writer places the elements of an XRECHNUNG invoice by the EN 16931 schema
// (see Root), whose content models are those of D16B, in the same order.
func profileIndex(p invoice.Profile) int {
	switch p {
	case invoice.ProfileMinimum:
		return pMinimum
	case invoice.ProfileBasicWL:
		return pBasicWL
	case invoice.ProfileBasic:
		return pBasic
	case invoice.ProfileEN16931, invoice.ProfileXRechnung, invoice.ProfilePeppol, invoice.ProfileNLCIUS, invoice.ProfileCIUSIT, invoice.ProfileCIUSPT:
		return pEN16931
	}
	return pExtended
}

// ciiSchema returns the schema a CII invoice of profile p is checked
// against: the tables, the root element and, for the Factur-X tables, the
// index of the profile.
func ciiSchema(p invoice.Profile) (*tables, *particle, int) {
	if p == invoice.ProfileXRechnung {
		return d16bTables, &d16bRoots[0], 0
	}
	i := profileIndex(p)
	return ciiTables, &roots[i], i
}

// Version returns the version of the Factur-X schemas the tables were
// generated from.
func Version() string { return schemaVersion }

// D16BVersion returns the version of the complete UN/CEFACT CII schema
// XRECHNUNG invoices are checked against ("D16B").
func D16BVersion() string { return d16bSchemaVersion }

// UBLVersion returns the version of the UBL schemas the tables were
// generated from.
func UBLVersion() string { return ublSchemaVersion }

// FatturaPAVersion returns the version of the FatturaPA schema the tables
// were generated from.
func FatturaPAVersion() string { return fpaSchemaVersion }

// FatturaPAEnumeration returns the values of the simple type name of the
// FatturaPA schema ("TipoDocumentoType"), or nil when the schema has no
// such type or the type is not an enumeration. The caller owns the slice.
func FatturaPAEnumeration(name string) []string {
	for i := range fpaTypes {
		t := &fpaTypes[i]
		if t.name == "p:"+name && t.content == contentSimple && t.facets != 0 {
			return append([]string(nil), fpaFacetTable[t.facets].enum...)
		}
	}
	return nil
}

// CheckFatturaPA validates the structure of root, a FatturaPA document,
// against the FatturaPA 1.2 schema and records the findings in c.Report.
// c.Doc may be nil.
func CheckFatturaPA(c *rules.Checker, root *xmltree.Node) {
	v := validator{c: c, t: fpaTables}
	r := &fpaRoots[0]
	if root.Name != r.name || root.Space != fpaNamespaces[r.ns] {
		c.Fail(RuleRoot, rules.Error, root, "the root element is %s in namespace %q, expected FatturaElettronica in namespace %q",
			display(root), root.Space, fpaNamespaces[r.ns])
		return
	}
	v.element(root, &fpaTypes[r.typ])
}

// FacturaeVersion returns the latest version of the Facturae schemas the
// tables were generated from ("3.2.2"); the tables also hold the earlier
// versions 3.2 and 3.2.1.
func FacturaeVersion() string { return facturaeSchemaVersion }

// FacturaeVersions lists the namespaces of the Facturae versions the tables
// hold, with their version: 3.2, 3.2.1 and 3.2.2.
func FacturaeVersions() map[string]string {
	return map[string]string{
		facturaeNamespaces[facturaeRoots[0].ns]: "3.2",
		facturaeNamespaces[facturaeRoots[1].ns]: "3.2.1",
		facturaeNamespaces[facturaeRoots[2].ns]: "3.2.2",
	}
}

// CheckFacturae validates the structure of a Facturae document against the
// schema of its version, given by the namespace of its root element
// (Facturae 3.2, 3.2.1 or 3.2.2), and records the findings in c.Report. The
// XML signature (ds:Signature) and the content of the Extensions elements
// are not checked.
func CheckFacturae(c *rules.Checker, root *xmltree.Node) {
	if root == nil {
		return
	}
	v := validator{c: c, t: facturaeTables}
	for i := range facturaeRoots {
		r := &facturaeRoots[i]
		if root.Name == r.name && root.Space == facturaeNamespaces[r.ns] {
			v.element(root, &facturaeTypes[r.typ])
			return
		}
	}
	c.Fail(RuleRoot, rules.Error, root, "the root element is %s in namespace %q, expected Facturae in the namespace of Facturae 3.2, 3.2.1 or 3.2.2 (%q)",
		display(root), root.Space, facturaeNamespaces[facturaeRoots[len(facturaeRoots)-1].ns])
}

// Check validates the structure of c.Doc and records the findings in
// c.Report: a CII invoice against the schema of c.Doc.Profile, a UBL invoice
// (c.Doc.UBL) against the UBL 2.1 schema of its root element.
func Check(c *rules.Checker) {
	if c.Doc == nil {
		return
	}
	if c.Doc.UBL != nil {
		checkUBL(c, c.Doc.UBL)
		return
	}
	if c.Doc.Root == nil {
		return
	}
	if c.Doc.Profile == invoice.ProfileUnknown {
		at := c.Doc.Context.Find("GuidelineSpecifiedDocumentContextParameter/ID")
		if at == nil {
			at = c.Doc.Root
		}
		c.Fail(RuleProfile, rules.Warning, at,
			"unknown profile (BT-24 %q): the structure is checked against the EXTENDED schema, the most permissive", c.Doc.SpecificationID())
	}
	t, r, p := ciiSchema(c.Doc.Profile)
	v := validator{c: c, t: t, p: p}
	root := c.Doc.Root
	if root.Name != r.name {
		c.Fail(RuleRoot, rules.Error, root, "the root element is %s, expected rsm:%s", display(root), r.name)
		return
	}
	if root.Space != t.namespaces[r.ns] {
		c.Fail(RuleNamespace, rules.Error, root, "%s is in namespace %q, expected %q", display(root), root.Space, t.namespaces[r.ns])
	}
	v.element(root, &t.types[r.typ])
}

// checkUBL validates the UBL document root.
func checkUBL(c *rules.Checker, root *xmltree.Node) {
	v := validator{c: c, t: ublTables}
	for i := range ublRoots {
		r := &ublRoots[i]
		if root.Name == r.name && root.Space == ublNamespaces[r.ns] {
			v.element(root, &ublTypes[r.typ])
			return
		}
	}
	c.Fail(RuleRoot, rules.Error, root, "the root element is %s in namespace %q, expected Invoice in namespace %q or CreditNote in namespace %q",
		display(root), root.Space, ublNamespaces[ublRoots[0].ns], ublNamespaces[ublRoots[1].ns])
}

type validator struct {
	c *rules.Checker
	t *tables
	p int // profile index (Factur-X)
}

func (v *validator) fail(rule string, n *xmltree.Node, format string, args ...any) {
	v.c.Fail(rule, rules.Error, n, format, args...)
}

// element validates n, an element of type t.
func (v *validator) element(n *xmltree.Node, t *ctype) {
	if t.content == contentSkip {
		return
	}
	if len(n.Attrs) > 0 || t.an > 0 {
		v.attributes(n, t)
	}
	if t.content == contentSimple {
		if len(n.Children) > 0 {
			v.fail(RuleUnexpected, n.Children[0], "%s is not allowed: %s holds a value, not elements", display(n.Children[0]), display(n))
		}
		text := n.Text
		if text == "" && len(n.Children) == 0 && t.def != "" {
			text = t.def // the value of an empty element with a default
		}
		if t.simple != kString || t.facets != 0 {
			if msg := v.t.checkValue(text, t.simple, t.facets); msg != "" {
				v.fail(RuleType, n, "%s: value %q %s %s", display(n), short(text), msg, typeLabel(t.simpleName, t.simple))
			}
		}
		return
	}
	if !blank(n.Text) {
		v.fail(RuleText, n, "%s holds elements only, text %q is not allowed", display(n), short(n.Text))
	}
	switch {
	case t.content == contentAny:
		v.wildcard(n, t)
	case t.choice:
		v.choice(n, t)
	default:
		v.sequence(n, t)
	}
}

// wildcard validates the children of n against the wildcard of t
// (xs:any namespace="##other"): elements in a namespace, other than the one
// t excludes, whose content is not checked.
func (v *validator) wildcard(n *xmltree.Node, t *ctype) {
	other := v.t.namespaces[t.other]
	matched := 0
	for _, ch := range n.Children {
		if ch.Space == other || ch.Space == "" {
			v.fail(RuleUnexpected, ch, "%s is not allowed in %s; expected here: an element in a namespace other than %q", display(ch), display(n), other)
			continue
		}
		matched++
		if t.max != unbounded && matched > int(t.max) {
			if t.max == 1 {
				v.fail(RuleCount, ch, "%s: %s holds only one element", display(ch), display(n))
			} else {
				v.fail(RuleCount, ch, "%s: %s holds at most %d elements", display(ch), display(n), t.max)
			}
		}
	}
	if matched < int(t.min) && matched == len(n.Children) {
		v.fail(RuleMissing, n, "an element in a namespace other than %q is missing in %s", other, display(n))
	}
}

// sequence validates the children of n against the sequence of t.
//
// The schemas satisfy the unique particle attribution constraint and the
// generator guarantees that local names are unique in a content model, so
// children are matched greedily by local name. When a child does not fit,
// the error is classified (missing, misplaced, unexpected) and matching
// resumes so that a single mistake gives a single finding.
func (v *validator) sequence(n *xmltree.Node, t *ctype) {
	ps := v.t.particles[t.first : t.first+t.n]
	pi, cnt := 0, 0 // current particle and its occurrences so far
	kids := n.Children
	for k, ch := range kids {
		if pi < len(ps) && ps[pi].name == ch.Name {
			cnt++
			v.matched(n, ch, &ps[pi], cnt)
			continue
		}
		j := pi + 1
		for j < len(ps) && ps[j].name != ch.Name {
			j++
		}
		if j < len(ps) {
			// Moving forward: the particles skipped must be optional, unless
			// they appear later (they are then reported as misplaced).
			for s := pi; s < j; s++ {
				have := 0
				if s == pi {
					have = cnt
				}
				if have < int(ps[s].min) && !hasChild(kids[k+1:], ps[s].name) {
					v.fail(RuleMissing, n, "%s is missing in %s (expected before %s)", v.t.display(&ps[s]), display(n), v.t.display(&ps[j]))
				}
			}
			pi, cnt = j, 1
			v.matched(n, ch, &ps[pi], 1)
			continue
		}
		if e := find(ps[:min(pi, len(ps))], ch.Name); e >= 0 {
			// Name the first preceding sibling that must follow ch.
			before := v.t.display(&ps[pi])
			for _, prev := range kids[:k] {
				if find(ps, prev.Name) > e {
					before = display(prev)
					break
				}
			}
			v.fail(RuleOrder, ch, "%s is not allowed at this position in %s: it must come before %s; expected here: %s",
				display(ch), display(n), before, v.t.expected(ps, pi, cnt))
			v.namespace(ch, &ps[e])
			v.element(ch, &v.t.types[ps[e].typ])
			continue
		}
		v.unexpected(n, t, ch, v.t.expected(ps, pi, cnt))
	}
	for s := pi; s < len(ps); s++ {
		have := 0
		if s == pi {
			have = cnt
		}
		if have < int(ps[s].min) {
			v.fail(RuleMissing, n, "%s is missing in %s", v.t.display(&ps[s]), display(n))
		}
	}
	if t.altN > 0 {
		v.alternatives(n, ps, v.t.alts[t.altFirst:t.altFirst+t.altN])
	}
}

// alternatives checks the choices of sequences flattened into the sequence
// ps of n: exactly one alternative of each group is present, complete.
func (v *validator) alternatives(n *xmltree.Node, ps []particle, alts []alternative) {
	for i := 0; i < len(alts); {
		j := i
		for j < len(alts) && alts[j].group == alts[i].group {
			j++
		}
		group := alts[i:j]
		var names []string
		chosen := -1
		optional := false
		for k := range group {
			a := &group[k]
			if a.need == 0 {
				// An alternative whose particles are all optional (or the
				// empty alternative of a choice that may be absent): the
				// group may be absent.
				optional = true
			}
			var seq []string
			for p := int(a.first); p < int(a.first+a.n); p++ {
				seq = append(seq, v.t.display(&ps[p]))
			}
			names = append(names, strings.Join(seq, " and "))
			first := firstChild(n.Children, ps[a.first:a.first+a.n])
			if first == nil {
				continue
			}
			if chosen >= 0 {
				v.fail(RuleUnexpected, first, "%s is not allowed in %s with %s: they are alternatives", display(first), display(n), names[chosen])
				continue
			}
			chosen = k
			for p := 0; p < int(a.n); p++ {
				if a.need&(1<<p) != 0 && !hasChild(n.Children, ps[int(a.first)+p].name) {
					v.fail(RuleMissing, n, "%s is missing in %s (with %s)", v.t.display(&ps[int(a.first)+p]), display(n), display(first))
				}
			}
		}
		if chosen < 0 && !optional {
			v.fail(RuleMissing, n, "%s is missing in %s", orList(names), display(n))
		}
		i = j
	}
}

// firstChild returns the first of kids named like one of ps, or nil.
func firstChild(kids []*xmltree.Node, ps []particle) *xmltree.Node {
	for _, k := range kids {
		if find(ps, k.Name) >= 0 {
			return k
		}
	}
	return nil
}

// choice validates the children of n against the choice of t. Consecutive
// children matching the same alternative, within its maxOccurs, form one
// occurrence of the choice.
func (v *validator) choice(n *xmltree.Node, t *ctype) {
	ps := v.t.particles[t.first : t.first+t.n]
	occ, alt, cnt := 0, -1, 0
	for _, ch := range n.Children {
		j := find(ps, ch.Name)
		if j < 0 {
			v.unexpected(n, t, ch, v.t.alternatives(ps))
			continue
		}
		if j == alt && (ps[j].max == unbounded || cnt < int(ps[j].max)) {
			cnt++
		} else {
			if alt >= 0 && cnt < int(ps[alt].min) {
				v.fail(RuleMissing, n, "%s is missing in %s", v.t.display(&ps[alt]), display(n))
			}
			alt, cnt = j, 1
			occ++
			if t.max != unbounded && occ > int(t.max) {
				v.fail(RuleCount, ch, "%s: only %d of %s allowed in %s", display(ch), t.max, v.t.alternatives(ps), display(n))
			}
		}
		v.namespace(ch, &ps[j])
		v.element(ch, &v.t.types[ps[j].typ])
	}
	if alt >= 0 && cnt < int(ps[alt].min) {
		v.fail(RuleMissing, n, "%s is missing in %s", v.t.display(&ps[alt]), display(n))
	}
	if occ < int(t.min) {
		v.fail(RuleMissing, n, "%s is missing in %s", v.t.alternatives(ps), display(n))
	}
}

// matched validates ch, the cnt-th occurrence of particle p in n.
func (v *validator) matched(n, ch *xmltree.Node, p *particle, cnt int) {
	if p.max != unbounded && cnt > int(p.max) {
		if p.max == 1 {
			v.fail(RuleCount, ch, "%s may occur only once in %s", display(ch), display(n))
		} else {
			v.fail(RuleCount, ch, "%s may occur at most %d times in %s", display(ch), p.max, display(n))
		}
	}
	v.namespace(ch, p)
	v.element(ch, &v.t.types[p.typ])
}

func (v *validator) namespace(ch *xmltree.Node, p *particle) {
	if ns := v.t.namespaces[p.ns]; ch.Space != ns {
		v.fail(RuleNamespace, ch, "%s is in namespace %q, expected %q (%s)", display(ch), ch.Space, ns, v.t.display(p))
	}
}

// unexpected reports ch, a child of n that type t does not declare. When a
// larger profile allows it, the message says so.
func (v *validator) unexpected(n *xmltree.Node, t *ctype, ch *xmltree.Node, want string) {
	if q := v.allowedFrom(t, ch.Name); q >= 0 {
		v.fail(RuleUnexpected, ch, "%s is not allowed in %s in profile %s (it is allowed from profile %s); expected here: %s",
			display(ch), display(n), profileNames[v.p], profileNames[q], want)
		return
	}
	v.fail(RuleUnexpected, ch, "%s is not allowed in %s; expected here: %s", display(ch), display(n), want)
}

// allowedFrom returns the smallest profile above the current one whose
// version of type t declares a child named name, or -1 (always for UBL).
func (v *validator) allowedFrom(t *ctype, name string) int {
	if !v.t.profiles {
		return -1
	}
	for q := v.p + 1; q < numProfiles; q++ {
		for i := range v.t.types {
			u := &v.t.types[i]
			if u.profiles&(1<<q) == 0 || u.name != t.name {
				continue
			}
			if find(v.t.particles[u.first:u.first+u.n], name) >= 0 {
				return q
			}
		}
	}
	return -1
}

// attributes validates the attributes of n against those of t.
func (v *validator) attributes(n *xmltree.Node, t *ctype) {
	as := v.t.attributes[t.afirst : t.afirst+t.an]
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Prefix != "" {
			// xmltree does not resolve attribute namespaces: the conventional
			// xsi prefix is trusted for the XML Schema instance attributes.
			if a.Prefix == "xsi" && (a.Name == "schemaLocation" || a.Name == "noNamespaceSchemaLocation" || a.Name == "type") {
				continue
			}
			v.fail(RuleAttribute, n, "attribute %s:%s is not allowed on %s", a.Prefix, a.Name, display(n))
			continue
		}
		d := -1
		for j := range as {
			if as[j].name == a.Name {
				d = j
				break
			}
		}
		if d < 0 {
			v.fail(RuleAttribute, n, "attribute %s is not allowed on %s%s", a.Name, display(n), attrList(as))
			continue
		}
		if as[d].kind != kString || as[d].facets != 0 {
			if msg := v.t.checkValue(a.Value, as[d].kind, as[d].facets); msg != "" {
				v.fail(RuleType, n, "attribute %s of %s: value %q %s %s", a.Name, display(n), short(a.Value), msg, typeLabel("", as[d].kind))
			}
		}
	}
	for j := range as {
		if !as[j].required {
			continue
		}
		if _, ok := n.LookupAttr(as[j].name); !ok {
			v.fail(RuleAttributeMissing, n, "attribute %s is required on %s", as[j].name, display(n))
		}
	}
}

// display returns the name of a particle of t with its conventional prefix.
func (t *tables) display(p *particle) string {
	if pre := nsPrefix(t.namespaces[p.ns]); pre != "" {
		return pre + ":" + p.name
	}
	return p.name
}

// display returns the name of an element with the conventional prefix of its
// namespace, or the prefix used in the document.
func display(n *xmltree.Node) string {
	if pre := nsPrefix(n.Space); pre != "" {
		return pre + ":" + n.Name
	}
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

func nsPrefix(ns string) string {
	switch ns {
	case rules.NSRsm:
		return "rsm"
	case rules.NSRam:
		return "ram"
	case rules.NSUdt:
		return "udt"
	case rules.NSQdt:
		return "qdt"
	case nsCac:
		return "cac"
	case nsCbc:
		return "cbc"
	case nsExt:
		return "ext"
	case nsFatturaPA:
		return "p"
	}
	return ""
}

// UBL namespaces of the elements that have a conventional prefix. The
// elements of the document namespaces (Invoice-2 and CreditNote-2) are
// displayed with the prefix of the document, usually none.
const (
	nsCac = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	nsCbc = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	nsExt = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
)

// nsFatturaPA is the namespace of the root element of FatturaPA; the other
// elements have none.
const nsFatturaPA = "http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"

// expected lists the elements that may come next in a sequence, the current
// particle being pi with cnt occurrences.
func (t *tables) expected(ps []particle, pi, cnt int) string {
	var names []string
	for s := pi; s < len(ps); s++ {
		have := 0
		if s == pi {
			have = cnt
		}
		if ps[s].max == unbounded || have < int(ps[s].max) {
			names = append(names, t.display(&ps[s]))
		}
		if have < int(ps[s].min) {
			break
		}
	}
	if len(names) == 0 {
		return "no further element"
	}
	return orList(names)
}

func (t *tables) alternatives(ps []particle) string {
	names := make([]string, len(ps))
	for i := range ps {
		names[i] = t.display(&ps[i])
	}
	return orList(names)
}

func orList(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " or " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

func attrList(as []attribute) string {
	if len(as) == 0 {
		return " (no attribute allowed)"
	}
	names := make([]string, len(as))
	for i := range as {
		names[i] = as[i].name
	}
	return " (allowed: " + strings.Join(names, ", ") + ")"
}

func find(ps []particle, name string) int {
	for i := range ps {
		if ps[i].name == name {
			return i
		}
	}
	return -1
}

func hasChild(kids []*xmltree.Node, name string) bool {
	for _, k := range kids {
		if k.Name == name {
			return true
		}
	}
	return false
}

func typeLabel(name string, k simpleKind) string {
	if name != "" {
		return name
	}
	switch k {
	case kDecimal:
		return "xs:decimal"
	case kInteger:
		return "xs:integer"
	case kBoolean:
		return "xs:boolean"
	case kBase64:
		return "xs:base64Binary"
	case kDate:
		return "xs:date"
	case kTime:
		return "xs:time"
	case kDateTime:
		return "xs:dateTime"
	case kDouble:
		return "xs:double"
	}
	return "xs:string"
}

// short truncates a value for messages.
func short(s string) string {
	s = trim(s)
	if len(s) > 40 {
		i := 37
		for i > 0 && !utf8.RuneStart(s[i]) {
			i--
		}
		return s[:i] + "..."
	}
	return s
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func blank(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isSpace(s[i]) {
			return false
		}
	}
	return true
}

// trim removes leading and trailing XML white space.
func trim(s string) string {
	i, j := 0, len(s)
	for i < j && isSpace(s[i]) {
		i++
	}
	for j > i && isSpace(s[j-1]) {
		j--
	}
	return s[i:j]
}

// checkValue checks a value against a simple type, whose facets are
// t.facetTable[fi]. It returns "" when the value is valid, else the reason
// ("is not a valid", "is not one of"...), completed by the caller with the
// type name.
func (t *tables) checkValue(s string, k simpleKind, fi uint16) string {
	ok := true
	switch k {
	case kDecimal:
		ok = validDecimal(trim(s))
	case kInteger:
		ok = validInteger(trim(s))
	case kBoolean:
		switch trim(s) {
		case "true", "false", "1", "0":
		default:
			ok = false
		}
	case kBase64:
		ok = validBase64(s)
	case kDate:
		ok = validDate(trim(s))
	case kTime:
		ok = validTime(trim(s))
	case kDateTime:
		ok = validDateTime(trim(s))
	case kDouble:
		ok = validDouble(trim(s))
	}
	if !ok {
		return "is not a valid"
	}
	if fi == 0 {
		return ""
	}
	if k != kString && k != kBase64 {
		// White space is collapsed in the values of these types.
		s = trim(s)
	}
	f := &t.facetTable[fi]
	if msg := f.check(s, t.regexps[fi]); msg != "" {
		return msg
	}
	if f.minIncl != "" && compare(s, f.minIncl, k) < 0 {
		return "is less than " + f.minIncl + ", the minimum of"
	}
	if f.maxIncl != "" && compare(s, f.maxIncl, k) > 0 {
		return "is greater than " + f.maxIncl + ", the maximum of"
	}
	return ""
}

// compare compares two valid values of kind k (decimal, integer or date):
// -1, 0 or +1. Dates are compared by their year, month and day.
func compare(a, b string, k simpleKind) int {
	if k == kDate {
		return strings.Compare(a[:min(len(a), 10)], b[:min(len(b), 10)])
	}
	x, err1 := invoice.ParseDecimal(a)
	y, err2 := invoice.ParseDecimal(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return x.Cmp(y)
}

// check checks s against the facets; res are the compiled patterns. It
// returns "" when s is valid, else the reason.
func (f *facets) check(s string, res []*regexp.Regexp) string {
	if f.collapse {
		s = strings.Join(strings.Fields(s), " ")
	}
	if len(f.enum) > 0 {
		found := false
		for _, e := range f.enum {
			if e == s {
				found = true
				break
			}
		}
		if !found {
			return "is not an allowed value (" + strings.Join(f.enum, ", ") + ") of"
		}
	}
	if f.minLen >= 0 || f.maxLen >= 0 {
		l := utf8.RuneCountInString(s)
		if (f.minLen >= 0 && l < f.minLen) || (f.maxLen >= 0 && l > f.maxLen) {
			return "has a length out of the bounds of"
		}
	}
	for _, re := range res {
		if !re.MatchString(s) {
			return "does not match the pattern of"
		}
	}
	return ""
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// validDecimal reports whether s is an xs:decimal lexical value: optional
// sign, digits, optional fraction; at least one digit; no exponent.
func validDecimal(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits, dot := 0, false
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case isDigit(c):
			digits++
		case c == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digits > 0
}

// validInteger reports whether s is an xs:integer lexical value.
func validInteger(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if i == len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// validBase64 reports whether s is an xs:base64Binary lexical value: groups
// of four characters of the base64 alphabet, white space allowed anywhere,
// with at most two padding characters at the end.
func validBase64(s string) bool {
	n, pad := 0, 0
	for i := 0; i < len(s); {
		if pad == 0 {
			j := i
			for j < len(s) && base64Alphabet[s[j]] {
				j++
			}
			n += j - i
			if i = j; i == len(s) {
				break
			}
		}
		switch c := s[i]; {
		case isSpace(c):
		case c == '=':
			if pad++; pad > 2 {
				return false
			}
			n++
		default:
			return false // data after padding, or not in the alphabet
		}
		i++
	}
	return n%4 == 0
}

// base64Alphabet[c] reports whether c is in the base64 alphabet (padding
// excluded).
var base64Alphabet = func() (t [256]bool) {
	for _, c := range []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/") {
		t[c] = true
	}
	return t
}()

// validDate reports whether s is an xs:date lexical value
// (-?YYYY-MM-DD with an optional time zone), the day existing in the month.
func validDate(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if !isDigit(s[i]) {
			return false
		}
	}
	y := int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
	m := int(s[5]-'0')*10 + int(s[6]-'0')
	d := int(s[8]-'0')*10 + int(s[9]-'0')
	if m < 1 || m > 12 || d < 1 || d > daysIn(m, y) {
		return false
	}
	return validTZ(s[10:])
}

// daysIn returns the number of days of month m (1 to 12) of year y.
func daysIn(m, y int) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// validTZ reports whether s is empty or an XML Schema time zone: Z, or
// +hh:mm or -hh:mm from -14:00 to +14:00.
func validTZ(s string) bool {
	switch {
	case s == "", s == "Z":
		return true
	case len(s) == 6 && (s[0] == '+' || s[0] == '-') && s[3] == ':' &&
		isDigit(s[1]) && isDigit(s[2]) && isDigit(s[4]) && isDigit(s[5]):
		h := int(s[1]-'0')*10 + int(s[2]-'0')
		m := int(s[4]-'0')*10 + int(s[5]-'0')
		return m <= 59 && (h < 14 || h == 14 && m == 0)
	}
	return false
}

// validDateTime reports whether s is an xs:dateTime lexical value: an
// xs:date without time zone, "T" and an xs:time.
func validDateTime(s string) bool {
	date, tm, ok := strings.Cut(s, "T")
	return ok && len(strings.TrimPrefix(date, "-")) == 10 && validDate(date) && validTime(tm)
}

// validTime reports whether s is an xs:time lexical value: hh:mm:ss with an
// optional fraction of second and an optional time zone; 24:00:00 is the
// end of the day.
func validTime(s string) bool {
	if len(s) < 8 || s[2] != ':' || s[5] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 3, 4, 6, 7} {
		if !isDigit(s[i]) {
			return false
		}
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	sec := int(s[6]-'0')*10 + int(s[7]-'0')
	rest := s[8:]
	zero := true // the fraction of second is zero
	if strings.HasPrefix(rest, ".") {
		i := 1
		for i < len(rest) && isDigit(rest[i]) {
			if rest[i] != '0' {
				zero = false
			}
			i++
		}
		if i == 1 {
			return false
		}
		rest = rest[i:]
	}
	if h == 24 {
		if m != 0 || sec != 0 || !zero {
			return false
		}
	} else if h > 23 || m > 59 || sec > 59 {
		return false
	}
	return validTZ(rest)
}

// validDouble reports whether s is an xs:double (or xs:float) lexical value
// of XML Schema 1.0: a decimal mantissa with an optional exponent, INF, -INF
// or NaN.
func validDouble(s string) bool {
	switch s {
	case "INF", "-INF", "NaN":
		return true
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		return validDecimal(s[:i]) && validInteger(s[i+1:])
	}
	return validDecimal(s)
}
