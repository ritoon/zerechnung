// Package peppol implements the rules of Peppol BIS Billing 3.0, as
// published by OpenPEPPOL in release 3.0.21 (unchanged in the hotfix
// 3.0.22): the 90 assertions of PEPPOL-EN16931-CII.sch for a CII invoice
// (Catalog), the 165 assertions of PEPPOL-EN16931-UBL.sch for a UBL invoice
// (UBLCatalog), in Go, with their exact identifiers, flags and messages.
//
// The rules of the CII schematron are:
//
//   - 25 Peppol rules (PEPPOL-EN16931-R*): business process and
//     specification identifiers, buyer reference, electronic addresses,
//     notes, allowances and charges, VAT totals in the accounting currency,
//     line periods, line amounts, base quantities;
//   - 17 checks of identifiers by ICD scheme (PEPPOL-COMMON-R*): GLN,
//     Norwegian, Danish, Belgian, Italian, Swedish, Australian and Dutch
//     identifiers, wherever a ram:ID, ram:GlobalID or ram:URIID carries the
//     scheme;
//   - 41 national rules of Norway (NO-R-*), Denmark (DK-R-*), Italy (IT-R-*),
//     Sweden (SE-R-*) and the Netherlands (NL-R-*), which apply when the
//     seller (and for some rules the buyer) is established in that country;
//   - 7 restricted code lists and formats (PEPPOL-EN16931-CL*, P0100,
//     F001).
//
// The rules of the UBL schematron are checked on the UBL document itself
// (rules.Doc.UBL), not on its CII view: they have other contexts and other
// tests, and 76 of them have no CII counterpart. They are the empty
// elements (PEPPOL-EN16931-R008), 27 Peppol rules, 17 checks of identifiers
// by ICD scheme, the national rules of Norway, Denmark, Italy, Sweden and
// the Netherlands, and of Greece (GR-*), Iceland (IS-R-*) and Germany
// (DE-R-*), and 18 code lists and formats, among them the type codes
// (P0100, P0101, P0112) and the VAT exemption reasons (P0104 to P0111).
// The faithfulness notes of the UBL rules are in ubl.go.
//
// The flags map to the severities of package rules: fatal to Error and
// warning to Warning. The rules of EN 16931 (BR-*, BR-CL-*, CII-SR-*), which
// a Peppol validator runs as well, are those of package en16931.
//
// Check applies the rules whatever the specification identifier BT-24: the
// caller decides when they apply (on request, or when BT-24 is the Peppol
// identifier). The rules check BT-24 themselves (PEPPOL-EN16931-R004).
//
// Values are evaluated as XPath 2.0 evaluates them in the reference
// implementation: general comparisons on the raw string value of the
// elements and attributes, normalize-space on XML white space only, string
// comparison of the dates of the line periods, exact decimal arithmetic for
// the amounts. Within a pattern, an element is checked by the first rule
// whose context it matches, as in Schematron: an allowance or charge that
// fails PEPPOL-EN16931-R041 or R042 is not checked by R040 and R043.
//
// # Faithfulness notes
//
// The CII schematron is implemented as written, including where its XPath
// does not do what its message says:
//
//   - The VAT identifier is looked up with schemeID 'VAT' (the UBL value),
//     where CII uses 'VA': in a CII invoice, the supplier country
//     ($supplierCountry, used by the Norwegian and Italian rules) is in
//     practice the country of the seller address, and NO-R-001, SE-R-001
//     and SE-R-002 only check identifiers with schemeID 'VAT'.
//   - PEPPOL-EN16931-P0100 has the context ram:ExchangedDocument/ram:TypeCode,
//     where CII uses rsm:ExchangedDocument: it only fires for an exchanged
//     document in the ram namespace.
//   - DK-R-007 looks for the mandate reference in
//     ram:SpecifiedTradePaymentTerms below the payment means, where CII puts
//     it below the settlement: a Danish domestic direct debit (49) always
//     fails it.
//   - PEPPOL-EN16931-R002 also fails for a single note with a subject code.
//   - Where the reference would stop on an XPath type error (several values
//     given to a single-valued function), the first value in document order
//     is used; where a value cannot be cast to a number (an amount that is
//     not a decimal), the assertion that needs it is skipped, the reference
//     stopping with an error.
package peppol

import (
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Identifiers of Peppol BIS Billing 3.0.
const (
	// SpecificationID is the specification identifier (BT-24) of Peppol BIS
	// Billing 3.0; PEPPOL-EN16931-R004 requires BT-24 to start with it.
	SpecificationID = "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	// ProcessBilling is the business process identifier (BT-23) of profile
	// 01, billing.
	ProcessBilling = invoice.ProcessPeppolBilling
	// ProcessBillingWithResponse is the business process identifier (BT-23)
	// of profile 02, billing with response.
	ProcessBillingWithResponse = "urn:peppol:bis:billing_with_response"
)

// Check runs the Peppol rules on c.Doc and records the findings in c.Report:
// the rules of the CII schematron on a CII invoice, those of the UBL
// schematron on the UBL document of a UBL invoice (c.Doc.UBL).
func Check(c *rules.Checker) {
	if c == nil || c.Doc == nil {
		return
	}
	if c.Doc.UBL != nil {
		checkUBL(c, c.Doc.UBL)
		return
	}
	if c.Doc.Root == nil {
		return
	}
	k := &checker{Checker: c, root: c.Doc.Root}
	k.cii = k.root.Name == "CrossIndustryInvoice" && k.root.Space == rules.NSRsm
	k.variables()
	k.root.Walk(func(n *xmltree.Node) bool {
		k.collect(n)
		k.element(n)  // main pattern: PEPPOL-EN16931-R*, PEPPOL-COMMON-R*
		k.codeList(n) // restricted code lists and formats
		return true
	})
	k.norway()
	k.denmark()
	k.italy()
	k.sweden()
	k.netherlands()
}

// checker wraps rules.Checker with the variables of the schematron.
type checker struct {
	*rules.Checker
	root *xmltree.Node
	cii  bool // the root is rsm:CrossIndustryInvoice: absolute paths apply

	profile         string          // $profile: "01", "02" or "Unknown"
	supplierCountry string          // $supplierCountry
	currency        []*xmltree.Node // $documentCurrencyCode
	taxCurrency     []*xmltree.Node // $taxCurrencyCode

	// Elements collected by the walk for the national rules.
	sellers     []*xmltree.Node // ram:SellerTradeParty
	taxes       []*xmltree.Node // ram:ApplicableTradeTax, ram:CategoryTradeTax
	settlements []*xmltree.Node // ram:ApplicableHeaderTradeSettlement
}

// collect keeps the elements that the national rules check wherever they
// are in the document.
func (k *checker) collect(n *xmltree.Node) {
	switch n.Name {
	case "SellerTradeParty":
		k.sellers = append(k.sellers, n)
	case "ApplicableTradeTax", "CategoryTradeTax":
		k.taxes = append(k.taxes, n)
	case "ApplicableHeaderTradeSettlement":
		k.settlements = append(k.settlements, n)
	}
}

// fail records a finding for rule id located at n, or at the root element
// when n is nil.
func (k *checker) fail(id string, n *xmltree.Node, detail string, args ...any) {
	if n == nil {
		n = k.root
	}
	report(k.Checker, byID, id, n, detail, args...)
}

// report records a finding for the rule id of catalog, located at n. The
// message is the one of the schematron, followed by the detail, if any.
func report(c *rules.Checker, catalog map[string]*rules.RuleInfo, id string, n *xmltree.Node, detail string, args ...any) {
	r := catalog[id]
	if r == nil {
		panic("peppol: unknown rule " + id)
	}
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(id)
	b.WriteString("]-")
	b.WriteString(r.Description)
	if detail != "" {
		b.WriteString("; ")
		if len(args) > 0 {
			fmt.Fprintf(&b, detail, args...)
		} else {
			b.WriteString(detail)
		}
	}
	c.Fail(id, r.Severity, n, "%s", b.String())
}

// check records a finding for rule id when ok is false.
func (k *checker) check(ok bool, id string, n *xmltree.Node, detail string, args ...any) {
	if !ok {
		k.fail(id, n, detail, args...)
	}
}

// abs returns the elements matching an absolute path from
// /rsm:CrossIndustryInvoice, given in local names without the root.
func (k *checker) abs(path string) []*xmltree.Node {
	if !k.cii {
		return nil
	}
	var out []*xmltree.Node
	for n := range all(k.root, path) {
		out = append(out, n)
	}
	return out
}

// absFirst returns the first element matching an absolute path, or nil.
func (k *checker) absFirst(path string) *xmltree.Node {
	if !k.cii {
		return nil
	}
	return first(k.root, path)
}

// Paths from the root element (rsm:CrossIndustryInvoice), in local names.
const (
	pBusinessProcess = "ExchangedDocumentContext/BusinessProcessSpecifiedDocumentContextParameter/ID"
	pTypeCode        = "ExchangedDocument/TypeCode"
	pTransaction     = "SupplyChainTradeTransaction"
	pAgreement       = pTransaction + "/ApplicableHeaderTradeAgreement"
	pSeller          = pAgreement + "/SellerTradeParty"
	pBuyer           = pAgreement + "/BuyerTradeParty"
	pTaxRep          = pAgreement + "/SellerTaxRepresentativeTradeParty"
	pLines           = pTransaction + "/IncludedSupplyChainTradeLineItem"
	pSettlement      = pTransaction + "/ApplicableHeaderTradeSettlement"
	pPaymentMeans    = pSettlement + "/SpecifiedTradeSettlementPaymentMeans"
	pSummation       = pSettlement + "/SpecifiedTradeSettlementHeaderMonetarySummation"
)

// variables computes the global variables of the schematron.
func (k *checker) variables() {
	switch norm(sv(k.absFirst(pBusinessProcess))) {
	case ProcessBilling:
		k.profile = "01"
	case ProcessBillingWithResponse:
		k.profile = "02"
	default:
		k.profile = "Unknown"
	}
	k.currency = k.abs(pSettlement + "/InvoiceCurrencyCode")
	k.taxCurrency = k.abs(pSettlement + "/TaxCurrencyCode")
	k.supplierCountry = k.supplierCountryOf()
}

// supplierCountryOf computes $supplierCountry: the first two letters of the
// VAT identifier (schemeID 'VAT') of the first seller, else of the seller
// tax representative, else the country of the seller address, else 'XX'.
func (k *checker) supplierCountryOf() string {
	if !k.cii {
		return "XX"
	}
	seller := k.root.Elem("SupplyChainTradeTransaction").Elem("ApplicableHeaderTradeAgreement").Elem("SellerTradeParty")
	if p := vatPrefix(seller.Elems("SpecifiedTaxRegistration")); p != "" {
		return strings.ToUpper(norm(p))
	}
	var regs []*xmltree.Node
	for r := range all(k.root, pTaxRep+"/SpecifiedTaxRegistration") {
		regs = append(regs, r)
	}
	if p := vatPrefix(regs); p != "" {
		return strings.ToUpper(norm(p))
	}
	if c := first(k.root, pSeller+"/PostalTradeAddress/CountryID"); c != nil {
		return strings.ToUpper(nsv(c))
	}
	return "XX"
}

// vatPrefix returns the first value of
// SpecifiedTaxRegistration[ram:ID/@schemeID = 'VAT']/substring(ram:ID, 1, 2)
// over regs, "" when there is none.
func vatPrefix(regs []*xmltree.Node) string {
	for _, r := range regs {
		if isVATRegistration(r) {
			return substring(sv(r.Elem("ID")), 1, 2)
		}
	}
	return ""
}

// isVATRegistration reports whether a ram:SpecifiedTaxRegistration has an
// identifier with schemeID 'VAT' (ram:ID/@schemeID = 'VAT').
func isVATRegistration(r *xmltree.Node) bool { return anyAttrIn(r, "ID", "schemeID", "VAT") }
