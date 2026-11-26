package peppol

import (
	"iter"
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The rules of PEPPOL-EN16931-UBL.sch, checked on the UBL document itself.
//
// The UBL schematron is not a translation of the CII one: it has rules of its
// own (empty elements, currencies, price allowances, the Greek, Icelandic and
// German rules, type codes and tax exemption reasons), other contexts for the
// common rules, and other messages. Where a test is the same in both
// syntaxes (the formats of the ICD identifiers, the check digits, the code
// lists), the functions are shared.
//
// As for CII, the schematron is implemented as written, including where its
// XPath does not do what its message says:
//
//   - PEPPOL-EN16931-R080 is only checked on a credit note, and the Danish
//     payment means rules DK-R-005 to DK-R-011 only on an invoice;
//   - PEPPOL-EN16931-R051 and PEPPOL-EN16931-CL007 require a currencyID on
//     every amount they check, R051 except on the invoice total VAT amount
//     in accounting currency (cac:TaxTotal without cac:TaxSubtotal);
//   - NL-R-001 only applies to a cbc:CreditNoteTypeCode, and IS-R-009 and
//     IS-R-010 look for the due date at the document level, where a credit
//     note does not carry it;
//   - DE-R-022 compares the file names case-sensitively, despite its
//     message.
//
// The country of the seller ($supplierCountry, used among others by the
// Norwegian, Italian and Greek rules and by SK-R-001) is the prefix of its
// first VAT identifier, or of that of the tax representative, and only then
// the country of its address; the Danish, Icelandic, Dutch and German
// rules, and the Slovak rules on the addresses (SK-R-002 to SK-R-007), look
// at the addresses.
//
// Where the reference would stop on an XPath error, the rules follow the
// policy of the CII rules: several values given to a single-valued function
// count as the first one, and an assertion that needs a value that cannot be
// cast (a decimal, a date, an integer) is skipped.

// Namespaces of UBL 2.1.
const (
	nsInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	nsCac        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	nsCbc        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
)

// Business process identifiers (BT-23) of the French Peppol profiles, which
// the UBL schematron counts as profile 01.
const (
	processFranceRegulated    = "urn:peppol:france:billing:regulated"
	processFranceNonRegulated = "urn:peppol:france:billing:non-regulated"
)

// Paths from the root element.
const (
	uSupplier        = "cac:AccountingSupplierParty/cac:Party"
	uCustomer        = "cac:AccountingCustomerParty/cac:Party"
	uTaxRep          = "cac:TaxRepresentativeParty"
	uCountry         = "cac:PostalAddress/cac:Country/cbc:IdentificationCode"
	uSupplierCountry = uSupplier + "/" + uCountry
	uCustomerCountry = uCustomer + "/" + uCountry
)

// ublChecker holds the state of a run on a UBL document, with the global
// variables of the schematron.
type ublChecker struct {
	*rules.Checker
	root       *xmltree.Node
	invoice    bool // the root is ubl-invoice:Invoice (/ubl-invoice:Invoice)
	creditNote bool // the root is ubl-creditnote:CreditNote
	sb         bool // the rules are those of Peppol BIS Self-Billing (PEPPOL-EN16931-UBL-SB.sch)

	profile                    string // $profile: "01", "02" or "Unknown"
	supplierCountry            string // $supplierCountry
	customerCountry            string // $customerCountry
	accountingSupplierCountry  string // $accountingSupplierCountry (Greek rules)
	supplierDE, customerDE     bool   // $supplierCountryIsDE, $customerCountryIsDE
	supplierNL, customerNL     bool   // $supplierCountryIsNL, $customerCountryIsNL
	taxRepNL                   bool   // $taxRepresentativeCountryIsNL
	supplierPostalSK           bool   // $supplierPostalCountryIsSK
	customerPostalSK           bool   // $customerPostalCountryIsSK
	greekSender, greekReceiver bool   // $isGreekSender, $isGreekReceiver

	// supplierCode and customerCode are the raw country codes of the
	// addresses, as concatenated by $DKSupplierCountry and $SupplierCountry
	// (Iceland), $DKCustomerCountry and $CustomerCountry.
	supplierCode, customerCode string

	currency []*xmltree.Node // $documentCurrencyCode

	// Swedish rules: some seller has its address in Sweden (seSupplier), and
	// a VAT identifier starting with SE too (seSupplierVAT); some buyer has
	// its address in Sweden (seCustomer). The schematron looks for them
	// anywhere in the document (//cac:AccountingSupplierParty/cac:Party).
	seSupplier, seSupplierVAT, seCustomer bool
}

// checkUBL runs the rules of the UBL schematron on the UBL document root:
// those of Peppol BIS Billing, or with sb those of Peppol BIS Self-Billing,
// which has no national rules.
func checkUBL(c *rules.Checker, root *xmltree.Node, sb bool) {
	k := &ublChecker{Checker: c, root: root, sb: sb}
	k.invoice = isElem(root, nsInvoice, "Invoice")
	k.creditNote = isElem(root, nsCreditNote, "CreditNote")
	k.variables()
	root.Walk(func(n *xmltree.Node) bool {
		// One call per pattern: within a pattern, an element is checked by
		// the first rule whose context it matches.
		k.empty(n)
		k.creditNoteRules(n)
		k.element(n)
		if !k.sb {
			k.slovakiaAddress(n)
			k.norway(n)
			k.denmark(n)
			k.italy(n)
			k.sweden(n)
			k.greece(n)
			k.greeceDomestic(n)
			k.iceland(n)
			k.netherlands(n)
			k.germany(n)
			k.slovakia(n)
		}
		k.codeList(n)
		k.cardinality(n)
		return true
	})
}

// fail records a finding for rule id located at n, or at the root element
// when n is nil.
func (k *ublChecker) fail(id string, n *xmltree.Node, detail string, args ...any) {
	if n == nil {
		n = k.root
	}
	catalog := ublByID()
	if k.sb {
		catalog = sbByID()
	}
	report(k.Checker, catalog, id, n, detail, args...)
}

// check records a finding for rule id when ok is false.
func (k *ublChecker) check(ok bool, id string, n *xmltree.Node, detail string, args ...any) {
	if !ok {
		k.fail(id, n, detail, args...)
	}
}

// variables computes the global variables of the schematron.
func (k *ublChecker) variables() {
	r := k.root
	process := ProcessBilling
	if k.sb {
		process = ProcessSelfBilling
	}
	switch norm(sv(ufirst(r, "cbc:ProfileID"))) {
	case process, processFranceRegulated, processFranceNonRegulated:
		k.profile = "01"
	case ProcessBillingWithResponse:
		k.profile = "02"
	default:
		k.profile = "Unknown"
	}
	k.supplierCountry = countryOf(r, uSupplier, uTaxRep)
	k.customerCountry = countryOf(r, uCustomer, "")
	k.accountingSupplierCountry = countryOf(r, uSupplier, "")
	address := func(path string) string { return strings.ToUpper(nsv(ufirst(r, path))) }
	k.supplierDE = address(uSupplierCountry) == "DE"
	k.customerDE = address(uCustomerCountry) == "DE"
	k.supplierNL = address(uSupplierCountry) == "NL"
	k.customerNL = address(uCustomerCountry) == "NL"
	k.taxRepNL = address(uTaxRep+"/"+uCountry) == "NL"
	k.supplierPostalSK = address(uSupplierCountry) == "SK"
	k.customerPostalSK = address(uCustomerCountry) == "SK"
	k.greekSender = k.supplierCountry == "GR" || k.supplierCountry == "EL"
	k.greekReceiver = k.customerCountry == "GR" || k.customerCountry == "EL"
	if k.invoice || k.creditNote {
		k.supplierCode = sv(ufirst(r, uSupplierCountry))
		k.customerCode = sv(ufirst(r, uCustomerCountry))
	}
	k.currency = ulist(r, "cbc:DocumentCurrencyCode")
	r.Walk(func(n *xmltree.Node) bool {
		if !uis(n, "cac:Party") {
			return true
		}
		se := uanyEq(n, uCountry, "SE")
		switch {
		case uparent(n, "cac:AccountingSupplierParty"):
			k.seSupplier = k.seSupplier || se
			k.seSupplierVAT = k.seSupplierVAT || se && swedishVATParty(n)
		case uparent(n, "cac:AccountingCustomerParty"):
			k.seCustomer = k.seCustomer || se
		}
		return true
	})
}

// countryOf computes $supplierCountry (with the path of the tax
// representative) or $customerCountry (without): the first two letters of
// the VAT identifier of the party, else of the tax representative, else the
// country of the party address, upper-cased, else 'XX'.
func countryOf(r *xmltree.Node, party, taxRep string) string {
	if p, ok := vatPrefixOf(ulist(r, party+"/cac:PartyTaxScheme")); ok {
		return strings.ToUpper(norm(p))
	}
	if taxRep != "" {
		if p, ok := vatPrefixOf(ulist(r, taxRep+"/cac:PartyTaxScheme")); ok {
			return strings.ToUpper(norm(p))
		}
	}
	if c := ufirst(r, party+"/"+uCountry); c != nil {
		return strings.ToUpper(nsv(c))
	}
	return "XX"
}

// vatPrefixOf evaluates, as the condition of an if,
// cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/substring(cbc:CompanyID, 1, 2)
// over schemes: it returns the first value, and whether it is not empty.
func vatPrefixOf(schemes []*xmltree.Node) (string, bool) {
	for _, s := range schemes {
		if isVATScheme(s) {
			p := substring(sv(ufirst(s, "cbc:CompanyID")), 1, 2)
			return p, p != ""
		}
	}
	return "", false
}

// isVATScheme reports whether a cac:PartyTaxScheme has a tax scheme 'VAT'
// (cac:TaxScheme/cbc:ID = 'VAT', on the raw value).
func isVATScheme(s *xmltree.Node) bool { return uanyEq(s, "cac:TaxScheme/cbc:ID", "VAT") }

// taxSchemeID returns normalize-space(cac:TaxScheme/cbc:ID) of a
// cac:PartyTaxScheme.
func taxSchemeID(s *xmltree.Node) string { return nsv(ufirst(s, "cac:TaxScheme/cbc:ID")) }

// swedishVATParty reports whether a party has a VAT identifier starting
// with SE:
// cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/substring(cbc:CompanyID, 1, 2) = 'SE'.
func swedishVATParty(p *xmltree.Node) bool {
	for _, s := range ulist(p, "cac:PartyTaxScheme") {
		if isVATScheme(s) && substring(sv(ufirst(s, "cbc:CompanyID")), 1, 2) == "SE" {
			return true
		}
	}
	return false
}

// UBL helpers. Unlike the CII rules, the UBL rules match the elements by
// namespace and local name, as the schematron does.

// isElem reports whether n is the element name of namespace space.
func isElem(n *xmltree.Node, space, name string) bool {
	return n != nil && n.Name == name && n.Space == space
}

// isDoc reports whether n is an ubl-invoice:Invoice or an
// ubl-creditnote:CreditNote.
func isDoc(n *xmltree.Node) bool {
	return isElem(n, nsInvoice, "Invoice") || isElem(n, nsCreditNote, "CreditNote")
}

// uis reports whether n is the UBL element qname, "cac:Name" or "cbc:Name".
func uis(n *xmltree.Node, qname string) bool {
	if n == nil {
		return false
	}
	prefix, name, _ := strings.Cut(qname, ":")
	if n.Name != name {
		return false
	}
	switch prefix {
	case "cac":
		return n.Space == nsCac
	case "cbc":
		return n.Space == nsCbc
	}
	return false
}

// uparent reports whether the parent of n is the UBL element qname.
func uparent(n *xmltree.Node, qname string) bool { return n != nil && uis(n.Parent, qname) }

// uall yields the elements matching a path of UBL names below n
// ("cac:Party/cbc:EndpointID"), in document order.
func uall(n *xmltree.Node, path string) iter.Seq[*xmltree.Node] {
	return func(yield func(*xmltree.Node) bool) {
		uwalk(n, path, yield)
	}
}

func uwalk(n *xmltree.Node, path string, yield func(*xmltree.Node) bool) bool {
	if n == nil {
		return true
	}
	seg, rest, _ := strings.Cut(path, "/")
	for _, c := range n.Children {
		if !uis(c, seg) {
			continue
		}
		if rest == "" {
			if !yield(c) {
				return false
			}
		} else if !uwalk(c, rest, yield) {
			return false
		}
	}
	return true
}

// ulist returns the elements matching path below n.
func ulist(n *xmltree.Node, path string) []*xmltree.Node {
	var out []*xmltree.Node
	for c := range uall(n, path) {
		out = append(out, c)
	}
	return out
}

// ufirst returns the first element matching path below n, or nil.
func ufirst(n *xmltree.Node, path string) *xmltree.Node {
	for c := range uall(n, path) {
		return c
	}
	return nil
}

// uexists reports whether an element matches path below n.
func uexists(n *xmltree.Node, path string) bool { return ufirst(n, path) != nil }

// ucount returns the number of elements matching path below n.
func ucount(n *xmltree.Node, path string) int {
	i := 0
	for range uall(n, path) {
		i++
	}
	return i
}

// uanyEq reports whether an element matching path has the raw string value
// v (path = 'v').
func uanyEq(n *xmltree.Node, path, v string) bool {
	for c := range uall(n, path) {
		if sv(c) == v {
			return true
		}
	}
	return false
}

// uanyIn reports whether an element matching path has one of the raw string
// values vs (path = ('a', 'b', …)).
func uanyIn(n *xmltree.Node, path string, vs ...string) bool {
	for c := range uall(n, path) {
		if in(sv(c), vs...) {
			return true
		}
	}
	return false
}

// uanyAttrIn reports whether an element matching path has the attribute
// attr with one of the values vs (path/@attr = ('a', 'b', …)).
func uanyAttrIn(n *xmltree.Node, path, attr string, vs ...string) bool {
	for c := range uall(n, path) {
		if a, ok := c.LookupAttr(attr); ok && in(a, vs...) {
			return true
		}
	}
	return false
}

// ufirstAttr returns normalize-space(path/@attr): the first attribute attr
// found on the elements matching path, "" when none has it.
func ufirstAttr(n *xmltree.Node, path, attr string) string {
	for c := range uall(n, path) {
		if a, ok := c.LookupAttr(attr); ok {
			return norm(a)
		}
	}
	return ""
}

// nonBlank reports whether an element matching path has a value that is not
// only white space (path[boolean(normalize-space(.))]).
func nonBlank(n *xmltree.Node, path string) bool {
	for c := range uall(n, path) {
		if nsv(c) != "" {
			return true
		}
	}
	return false
}

// atRoot reports whether n is a child of the root, when the root is an
// Invoice or a CreditNote (/ubl-invoice:Invoice/… | /ubl-creditnote:CreditNote/…).
func (k *ublChecker) atRoot(n *xmltree.Node) bool {
	return (k.invoice || k.creditNote) && n != nil && n.Parent == k.root
}
