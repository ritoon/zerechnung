// Package en16931 implements the validation rules of EN 16931-1 for the CII
// syntax, as published by CEN TC 434 in its validation artefacts (release
// Release): the business rules BR-* and BR-CO-*, the decimal rules BR-DEC-*,
// the VAT category rules (BR-S, BR-Z, BR-E, BR-AE, BR-IC, BR-G, BR-O, BR-IP
// for IGIC, BR-IG for IPSI, BR-B), the code list rules BR-CL-* and the CII
// syntax rules CII-SR-* and CII-DT-*.
//
// The rules reproduce the XPath of the official schematron rather than the
// prose of the standard, so that the verdicts are the same as those of the
// official artefacts:
//
//   - amounts are exact decimals (xs:decimal), never floating point numbers;
//     round() rounds halves towards positive infinity;
//   - string comparisons are exact (no trimming) unless the rule uses
//     normalize-space(); code lists accept a value when, once trimmed, it
//     contains no space and is in the list;
//   - within a pattern, a node is checked by the first rule whose context
//     matches it only, so some rules never run on some nodes (for instance
//     the VAT breakdown rules BR-45 to BR-48 do not apply to the breakdowns
//     of category O, L and M, which have their own rules);
//   - assertions that can never fail in the official artefacts are not
//     implemented: BR-CO-05 to BR-CO-08, BR-IP-08, BR-IP-09, BR-IG-08,
//     BR-IG-09, CII-SR-039 to CII-SR-044 and CII-DT-010 to CII-DT-012.
//
// Severities are the official ones (fatal: Error, warning: Warning), with
// adjustments that follow the reference validators of the profiles:
//
//   - XRECHNUNG: the severities set by the KoSIT validator configuration
//     XRechnung 3.0.2 (CII-SR-452, 453, 454, 465 and 466 are errors, BR-CL-21
//     and BR-CL-23 warnings, CII-SR-475 and 476 information);
//   - BASIC, EXTENDED, and EN 16931 in a Factur-X PDF (rules.Doc.FacturX):
//     the reference is the Factur-X 1.09.2 schematron of the profile
//     (FNFE-MPE / FeRD), which reuses the CEN business rules with an older
//     base, checks the code lists with its own lists (package facturx
//     implements them: the BR-CL rules are not reported), keeps only some
//     CII syntax rules (CII-SR-463 to 470, 472, 473, and 474 to 476 for
//     EN 16931; CII-DT-097 is kept as an addition), makes CII-SR-465 and
//     CII-SR-466 errors and cannot fail CII-SR-464 (not reported);
//   - EXTENDED and EXTENDED-CTC-FR: Factur-X EXTENDED is only conformant
//     with EN 16931 and its schematron keeps a subset of the CEN rules.
//     The rules of that subset keep their severity; the other business rules
//     are reported as warnings, and the other CII syntax rules (which
//     restrict CII to the EN 16931 data set) are not reported. The code
//     lists of EXTENDED-CTC-FR are those of the CEN, as warnings;
//   - EXTENDED-CTC-FR, in CII: the rules are those of the FNFE-MPE schematron
//     EXTENDED-CTC-FR-CII.sch (France_RFE CTCFRRelease, see CTCFRCatalog),
//     with its severities, instead of the CEN ones: the CEN business rules
//     it keeps, its own versions of those it changes (line subtypes, VAT
//     breakdowns by rate and exemption reason, totals with a tolerance), its
//     code lists, and its CII syntax rules, which restrict CII to the
//     EXTENDED-CTC-FR data set as the CEN rules restrict it to the EN 16931
//     one (see ctcfr.go);
//   - EXTENDED-CTC-FR, in UBL: the business rules that the schematron
//     EXTENDED-CTC-FR-UBL.sch keeps from the CEN UBL artefacts, with its
//     severities and the identifiers BR-AF and BR-AG of categories L and M
//     (see ctcfrUBLRules). Package internal/rules/ubl checks the other
//     rules of the schematron on the UBL document.
//
// For a UBL invoice, Check runs on the CII view built by package
// internal/rules/ubl (rules.Doc.UBL is set) and applies the UBL artefacts of
// the same release where they differ: the rules of categories L and M are
// BR-IG and BR-IP (swapped with respect to CII), with their own tests and
// tolerances, BR-CO-17 has a strict tolerance, the taxable amounts of the
// zero rated categories (rule 08) are exact unrounded sums, BR-51 is a
// warning, and the XRechnung levels are those of the UBL scenarios. The
// rules whose UBL context or test the view cannot render (the code lists,
// the decimal rules, BR-CO-03, BR-53 and a few others, see ublNative) are
// left to package ubl, which checks them on the UBL document.
//
// An EN 16931 CII invoice (BT-24 urn:cen.eu:en16931:2017) on its own gets
// the CEN rules, as the KoSIT validator gives them: its identifier is the one
// of any CII invoice that follows EN 16931 without a CIUS, Factur-X or not.
// It gets the Factur-X schematron when it is the XML of a Factur-X PDF
// (validate.Options.Hybrid, set by zerechnung.Check).
//
// MINIMUM and BASIC WL invoices are not EN 16931 invoices: Check does nothing
// for them.
package en16931

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

//go:generate go run ./gen -o catalog_gen.go
//go:generate go run ./gen -ctcfr -o ctcfr_gen.go

// Check runs the rules on c.Doc and records the findings in c.Report.
func Check(c *rules.Checker) {
	d := c.Doc
	if d == nil || d.Root == nil {
		return
	}
	if p := d.Profile; p != invoice.ProfileUnknown && !p.EN16931() {
		return
	}
	var k checker
	k.init(c)
	k.walk(d.Root, k.syn.trie, 0)
	k.finish()
}

// mode selects the severity adjustments of a profile.
type mode uint8

const (
	modeCEN mode = iota
	modeXRechnung
	modeXRechnungExtension
	modeXRechnungCVD
	modeExtended
	modeExtendedCTCFR // the rules of the EXTENDED-CTC-FR schematron for CII
	modeCTCFRUBL      // the CEN UBL rules kept by the EXTENDED-CTC-FR schematron for UBL
	modeBasic         // BASIC: the CEN rules kept by the Factur-X schematron of BASIC
	modeEN16931FX     // EN 16931 in a Factur-X PDF: the CEN rules kept by the Factur-X schematron of EN 16931
)

// Category indexes of the VAT category codes of the EN 16931 subset.
const (
	catS = iota
	catZ
	catE
	catAE
	catK
	catG
	catO
	catL
	catM
	catB
	nCat
)

var catCodes = [nCat]string{"S", "Z", "E", "AE", "K", "G", "O", "L", "M", "B"}

// catIndex returns the index of a VAT category code compared exactly, or -1.
func catIndex(s string) int {
	switch s {
	case "S":
		return catS
	case "Z":
		return catZ
	case "E":
		return catE
	case "AE":
		return catAE
	case "K":
		return catK
	case "G":
		return catG
	case "O":
		return catO
	case "L":
		return catL
	case "M":
		return catM
	case "B":
		return catB
	}
	return -1
}

// nodeList is a list of nodes kept inline up to two elements, so that the
// checker does not allocate for the usual documents.
type nodeList struct {
	n    int
	head [2]*xmltree.Node
	more []*xmltree.Node
}

func (l *nodeList) add(n *xmltree.Node) {
	if l.n < len(l.head) {
		l.head[l.n] = n
	} else {
		l.more = append(l.more, n)
	}
	l.n++
}

func (l *nodeList) at(i int) *xmltree.Node {
	if i < len(l.head) {
		return l.head[i]
	}
	return l.more[i-len(l.head)]
}

type seenKey struct {
	rule string
	ctx  *xmltree.Node
}

// checker holds the state of one run.
type checker struct {
	c    *rules.Checker
	d    *rules.Doc
	mode mode
	syn  *syntaxSet         // syntax rules of the profile
	ubl  bool               // c.Doc is the CII view of a UBL invoice
	cats *[nCat]vatCategory // VAT category rules of the syntax

	stack [64]*xmltree.Node // ancestors of the visited node, by depth
	depth int
	seen  []seenKey // syntax findings already reported, per context node
	summ  nodeList  // ram:SpecifiedTradeSettlementHeaderMonetarySummation
	settl nodeList  // ram:ApplicableHeaderTradeSettlement

	// Facts known before the walk.
	sellerVAFC   bool // seller VAT identifier (schemeID VA) or tax registration (FC)
	sellerVA     bool
	taxRepVA     bool
	buyerVA      bool
	buyerLegalID bool
	billedUnit   bool // a line BilledQuantity has a unitCode (CII-DT-033)
	headerTax    bool // a VAT breakdown exists (BR-CO-18)

	// Facts collected during the walk.
	catHdr, catLine, catTax [nCat]int // ApplicableTradeTax / CategoryTradeTax elements per category
	catDetail               [nCat]int // ApplicableTradeTax elements of plain and DETAIL lines per category (EXTENDED-CTC-FR)
	anyCode                 [nCat]bool
	countryNotIT            bool // a ram:CountryID is not IT (BR-B-01)
	taxPointDate            bool // a ram:TaxPointDate exists (BR-CO-03)
	dueDateTypeCode         bool // a ram:DueDateTypeCode exists (BR-CO-03)
	applicableNotO          bool // an ApplicableTradeTax has a category other than O
	categoryNotO            bool // a CategoryTradeTax has a category other than O
	dueCode                 string
	dueCodes                int // distinct DueDateTypeCode values of ApplicableTradeTax (0, 1, 2 = several)
	pmType, pmInfo          *xmltree.Node
	pmTypeDiffer            bool
	pmInfoDiffer            bool
	paymentRefs             int
	financialCards          int
	debtorAccounts          int

	// catHdr, catLine and catTax, of tax type VAT only (UBL:
	// cac:TaxScheme/normalize-space(upper-case(cbc:ID)) = 'VAT').
	catHdrVAT, catLineVAT, catTaxVAT [nCat]int
}

func (k *checker) init(c *rules.Checker) {
	k.c = c
	k.d = c.Doc
	d := k.d
	k.cats = &vatCategories
	if d.UBL != nil {
		k.ubl = true
		k.cats = &vatCategoriesUBL
	}
	k.mode = profileMode(d)
	k.syn = cenSyntax
	switch k.mode {
	case modeExtendedCTCFR:
		k.syn = ctcfrSyntax
		k.cats = &vatCategoriesCTCFR
	case modeCTCFRUBL:
		k.cats = &vatCategoriesUBLCTCFR
	}

	seller := d.Agreement.Elem("SellerTradeParty")
	for it, r := elems(seller, "SpecifiedTaxRegistration"); r != nil; r = it.next() {
		for it, id := elems(r, "ID"); id != nil; id = it.next() {
			switch s, _ := id.LookupAttr("schemeID"); s {
			case "VA":
				k.sellerVA = true
				k.sellerVAFC = true
			case "FC":
				k.sellerVAFC = true
			}
		}
	}
	k.taxRepVA = hasTaxRegistration(d.Agreement.Elem("SellerTaxRepresentativeTradeParty"), "VA")
	buyer := d.Agreement.Elem("BuyerTradeParty")
	k.buyerVA = hasTaxRegistration(buyer, "VA")
	for it, o := elems(buyer, "SpecifiedLegalOrganization"); o != nil; o = it.next() {
		if o.Elem("ID") != nil {
			k.buyerLegalID = true
		}
	}
	for _, l := range d.Lines {
		for it, del := elems(l, "SpecifiedLineTradeDelivery"); del != nil; del = it.next() {
			for it, q := elems(del, "BilledQuantity"); q != nil; q = it.next() {
				if _, ok := q.LookupAttr("unitCode"); ok {
					k.billedUnit = true
				}
			}
		}
	}
	for it, s := elems(d.Transaction, "ApplicableHeaderTradeSettlement"); s != nil; s = it.next() {
		if s.Elem("ApplicableTradeTax") != nil {
			k.headerTax = true
		}
	}
}

// profileMode returns the severity adjustments for the profile of d.
func profileMode(d *rules.Doc) mode {
	switch d.Profile {
	case invoice.ProfileXRechnung:
		spec := d.SpecificationID()
		switch {
		case strings.Contains(spec, "#conformant#urn:xeinkauf.de:kosit:extension:xrechnung"):
			return modeXRechnungExtension
		case strings.Contains(spec, "xrechnung:cvd"):
			return modeXRechnungCVD
		}
		return modeXRechnung
	case invoice.ProfileBasic:
		if d.FacturX() {
			return modeBasic
		}
	case invoice.ProfileEN16931:
		if d.FacturX() {
			return modeEN16931FX
		}
	case invoice.ProfileExtended:
		// The Factur-X EXTENDED schematron only exists for CII.
		if d.UBL == nil {
			return modeExtended
		}
	case invoice.ProfileExtendedCTCFR:
		if d.UBL != nil {
			return modeCTCFRUBL
		}
		return modeExtendedCTCFR
	}
	return modeCEN
}

// level returns the severity that the KoSIT configuration gives to rule id in
// the XRechnung scenario m, if it changes it.
func level(d *rules.Doc, m mode, id string) (rules.Severity, bool) {
	levels := xrechnungLevels
	if d.UBL != nil {
		if id == "UBL-CR-646" && m == modeXRechnung && d.UBL.Name == "CreditNote" {
			return 0, false
		}
		levels = xrechnungLevelsUBL
	}
	s, ok := levels[m][id]
	return s, ok
}

// Level returns the severity of rule id in d: sev, or the severity that the
// KoSIT validator configuration gives to the rule in the XRechnung scenario
// of d. Package internal/rules/ubl uses it for the rules it checks on the UBL
// document itself.
func Level(d *rules.Doc, id string, sev rules.Severity) rules.Severity {
	if s, ok := level(d, profileMode(d), id); ok {
		return s
	}
	return sev
}

// hasTaxRegistration reports whether party has a SpecifiedTaxRegistration/ID
// with the given schemeID.
func hasTaxRegistration(party *xmltree.Node, scheme string) bool {
	for it, r := elems(party, "SpecifiedTaxRegistration"); r != nil; r = it.next() {
		for it, id := elems(r, "ID"); id != nil; id = it.next() {
			if s, ok := id.LookupAttr("schemeID"); ok && s == scheme {
				return true
			}
		}
	}
	return false
}

// severity maps the rule identifiers of Catalog to their severity.
var severity = func() map[string]rules.Severity {
	m := make(map[string]rules.Severity, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Severity
	}
	return m
}()

// description maps the rule identifiers of Catalog to their description.
var description = func() map[string]string {
	m := make(map[string]string, len(Catalog))
	for _, r := range Catalog {
		m[r.ID] = r.Description
	}
	return m
}()

// ctcfrRules maps the rule identifiers of CTCFRCatalog to their entry.
var ctcfrRules = func() map[string]*rules.RuleInfo {
	m := make(map[string]*rules.RuleInfo, len(CTCFRCatalog))
	for i := range CTCFRCatalog {
		m[CTCFRCatalog[i].ID] = &CTCFRCatalog[i]
	}
	return m
}()

// desc returns the description of rule id: the message of the
// EXTENDED-CTC-FR schematron for its syntax rules, the CEN one otherwise.
func (k *checker) desc(id string) string {
	if k.syn.ctcfr {
		if r := ctcfrRules[id]; r != nil {
			return r.Description
		}
	}
	return description[id]
}

// fail records a finding for rule id located at n, with the severity of the
// rule adjusted for the profile.
func (k *checker) fail(id string, n *xmltree.Node, format string, args ...any) {
	if k.ubl && ublNative(id) {
		return
	}
	sev, ok := severity[id]
	if !ok {
		sev = rules.Error
	}
	if k.ubl {
		if s, ok := ublSeverity[id]; ok {
			sev = s
		}
	}
	switch k.mode {
	case modeExtendedCTCFR:
		// The rules of the EXTENDED-CTC-FR schematron, with its identifiers
		// and severities: the CEN rules it removes are not reported.
		if r, ok := ctcfrRenamed[id]; ok {
			id = r
		}
		r := ctcfrRules[id]
		if r == nil {
			return
		}
		sev = r.Severity
	case modeCTCFRUBL:
		// The CEN UBL rules that the EXTENDED-CTC-FR schematron for UBL
		// keeps, with its severities. BR-CO-09, whose list and test it
		// changes, is checked by package ubl.
		s, ok := ctcfrUBLRules[id]
		if !ok || id == "BR-CO-09" {
			return
		}
		sev = s
	case modeExtended:
		if strings.HasPrefix(id, "BR-CL-") {
			return // the code lists of the Factur-X schematron, see package facturx
		}
		if !extendedRules[id] {
			if strings.HasPrefix(id, "CII-") {
				return
			}
			sev = rules.Warning
			format += " (not enforced for Factur-X EXTENDED)"
		}
		if s, ok := facturXLevels[id]; ok {
			if s == notReported {
				return
			}
			sev = s
		}
	case modeBasic, modeEN16931FX:
		switch {
		case strings.HasPrefix(id, "BR-CL-"):
			return // the code lists of the Factur-X schematron, see package facturx
		case strings.HasPrefix(id, "CII-") && !facturXSyntax(k.mode, id):
			return
		}
		if s, ok := facturXLevels[id]; ok {
			if s == notReported {
				return
			}
			sev = s
		}
	case modeXRechnung, modeXRechnungExtension, modeXRechnungCVD:
		if s, ok := level(k.d, k.mode, id); ok {
			sev = s
		}
	}
	if n == nil {
		n = k.d.Root
	}
	k.c.Fail(id, sev, n, format, args...)
}

// ublNative reports whether the rule is checked on the UBL document itself
// rather than on its CII view: the CII syntax rules (UBL has its own), the
// code lists (whose contexts and lists differ between the syntaxes, see
// package internal/rules/ubl), the decimal rules (in UBL, at most two
// characters after the first ".", whatever the value; BR-DEC-13 and BR-DEC-15
// never fail there), and the business rules of ublRules.
func ublNative(id string) bool {
	switch {
	case strings.HasPrefix(id, "CII-"), strings.HasPrefix(id, "BR-CL-"), strings.HasPrefix(id, "BR-DEC-"):
		return true
	}
	return ublRules[id]
}

// ublRules lists the business rules whose context or test in the UBL
// artefacts the CII view cannot render (an address that must exist, an
// attribute that must exist rather than be filled in, the payee compared with
// the seller trading name, a context on the document, the VAT breakdown of
// each tax total): package ubl checks them on the UBL document.
var ublRules = map[string]bool{
	"BR-09": true, "BR-11": true, "BR-17": true, "BR-20": true, "BR-52": true, "BR-53": true,
	"BR-55": true, "BR-56": true, "BR-57": true, "BR-61": true, "BR-62": true, "BR-63": true,
	"BR-64": true, "BR-65": true, "BR-CO-03": true, "BR-CO-14": true, "BR-CO-18": true,
}

// ublSeverity holds the severities of the UBL artefacts that differ from the
// CII ones.
var ublSeverity = map[string]rules.Severity{
	"BR-51": rules.Warning, // primary account number of a card: flag warning in UBL, fatal in CII
}

// failOnce records a finding for rule id unless one was already recorded for
// the same context node.
func (k *checker) failOnce(id string, ctx, n *xmltree.Node, format string, args ...any) {
	for _, s := range k.seen {
		if s.rule == id && s.ctx == ctx {
			return
		}
	}
	k.seen = append(k.seen, seenKey{id, ctx})
	k.fail(id, n, format, args...)
}

// failDesc records a finding whose message is the official description of the
// rule.
func (k *checker) failDesc(id string, n *xmltree.Node) {
	k.fail(id, n, "%s", k.desc(id))
}

// parentIs reports whether the parent of the visited node is named names[0],
// its grandparent names[1], and so on.
func (k *checker) parentIs(names ...string) bool {
	for i, name := range names {
		j := k.depth - 1 - i
		if j < 0 || k.stack[j].Name != name {
			return false
		}
	}
	return true
}

// hasAncestor reports whether an ancestor of the visited node has the given
// local name.
func (k *checker) hasAncestor(name string) bool {
	for i := k.depth - 1; i >= 0; i-- {
		if k.stack[i].Name == name {
			return true
		}
	}
	return false
}
