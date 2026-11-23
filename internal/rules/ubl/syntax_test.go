package ubl

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The syntax tests raise each UBL-CR rule of the generated tables crRules
// (CEN schematron) and ctcfrCRRules (EXTENDED-CTC-FR schematron, on tCTCFR),
// each UBL-DT rule of forbiddenAttrs and the UBL-SR rules of srCases: for
// each entry, they build the elements of the path in tInvoice (or in its
// credit note for a path that names one), place the element or the
// attribute that must not exist, or repeat the counted element once more
// than the rule allows, and check that the rule is reported with the
// severity of Catalog (CTCFRCatalog). The same document without the faulty
// element (the path up to its parent, or the counted element exactly as
// many times as allowed) must not report the rule.

// ublNamespaces maps the prefixes of the paths to their namespace.
var ublNamespaces = map[string]string{
	"cac": NSCac,
	"cbc": NSCbc,
	"ext": NSExt,
}

// srCases lists the UBL-SR rules that limit the number of elements at path
// to n, or, when n is 0, that the element at path breaks on its own (an
// empty party tax scheme, a financial account without identifier...). The
// UBL-SR rules on the values of the elements are tested in checkCases.
var srCases = []struct {
	rule  string
	scope crScope // crRoot or crLine
	path  string
	n     int
}{
	{"UBL-SR-01", crRoot, "cac:ContractDocumentReference/cbc:ID", 1},
	{"UBL-SR-02", crRoot, "cac:ReceiptDocumentReference/cbc:ID", 1},
	{"UBL-SR-03", crRoot, "cac:DespatchDocumentReference/cbc:ID", 1},
	{"UBL-SR-05", crRoot, "cac:PaymentTerms/cbc:Note", 1},
	{"UBL-SR-06", crRoot, "cac:BillingReference/cac:InvoiceDocumentReference", 1},
	{"UBL-SR-08", crRoot, "cac:InvoicePeriod", 1},
	{"UBL-SR-09", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName", 1},
	{"UBL-SR-10", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name", 1},
	{"UBL-SR-11", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID", 1},
	{"UBL-SR-12", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID", 1},
	{"UBL-SR-14", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm", 1},
	{"UBL-SR-15", crRoot, "cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName", 1},
	{"UBL-SR-16", crRoot, "cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification/cbc:ID", 1},
	{"UBL-SR-17", crRoot, "cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID", 1},
	{"UBL-SR-22", crRoot, "cac:TaxRepresentativeParty/cac:PartyName/cbc:Name", 1},
	{"UBL-SR-23", crRoot, "cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID", 1},
	{"UBL-SR-24", crRoot, "cac:Delivery", 1},
	{"UBL-SR-26", crRoot, "cac:PaymentMeans/cbc:PaymentID", 1},
	{"UBL-SR-27", crRoot, "cac:PaymentMeans/cbc:PaymentMeansCode", 1},
	{"UBL-SR-28", crRoot, "cac:PaymentMeans/cac:PaymentMandate/cbc:ID", 1},
	{"UBL-SR-32", crRoot, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:TaxExemptionReason", 1},
	{"UBL-SR-33", crRoot, "cac:AdditionalDocumentReference/cbc:DocumentDescription", 1},
	{"UBL-SR-34", crLine, "cbc:Note", 1},
	{"UBL-SR-35", crLine, "cac:OrderLineReference/cbc:LineID", 1},
	{"UBL-SR-36", crLine, "cac:InvoicePeriod", 1},
	{"UBL-SR-37", crLine, "cac:Price/cac:AllowanceCharge/cbc:Amount", 1},
	{"UBL-SR-39", crRoot, "cac:ProjectReference/cbc:ID", 1},
	{"UBL-SR-40", crRoot, "cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name", 1},
	{"UBL-SR-42", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme", 2},
	{"UBL-SR-45", crRoot, "cac:PaymentMeans/cbc:PaymentDueDate", 1},
	{"UBL-SR-48", crLine, "cac:Item/cac:ClassifiedTaxCategory", 1},
	{"UBL-SR-49", crRoot, "cac:InvoicePeriod/cbc:DescriptionCode", 1},
	{"UBL-SR-50", crLine, "cac:Item/cbc:Description", 1},
	{"UBL-SR-51", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:AddressLine", 1},
	{"UBL-SR-52", crLine, "cac:DocumentReference", 1},
	{"UBL-SR-53", crRoot, "cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme", 0},
	{"UBL-SR-54", crRoot, "cac:PaymentMeans/cac:CardAccount", 1},
	{"UBL-SR-55", crRoot, "cac:PaymentMeans/cac:PaymentMandate", 1},
	{"UBL-SR-56", crRoot, "cac:OriginatorDocumentReference/cbc:ID", 1},
	{"UBL-SR-57", crRoot, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity", 1},
	{"UBL-SR-58", crRoot, "cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch", 0},
	{"UBL-SR-59", crRoot, "cac:PaymentMeans/cac:PayeeFinancialAccount", 0},
}

// splitPath returns the steps of a path ("cac:A/cbc:B/@c"), none for "".
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// newElem appends to n an empty child element named step ("cac:Name").
func newElem(t *testing.T, n *xmltree.Node, step string) *xmltree.Node {
	t.Helper()
	prefix, local, _ := strings.Cut(step, ":")
	space, ok := ublNamespaces[prefix]
	if !ok {
		t.Fatalf("unsupported step %q", step)
	}
	c := &xmltree.Node{Name: local, Prefix: prefix, Space: space, Parent: n}
	n.Children = append(n.Children, c)
	return c
}

// isStep reports whether n is the element named step ("cac:Name").
func isStep(n *xmltree.Node, step string) bool {
	prefix, local, _ := strings.Cut(step, ":")
	return n.Name == local && n.Space == ublNamespaces[prefix]
}

// descend returns the element at path below n, following the first child of
// each name and appending the children that are missing.
func descend(t *testing.T, n *xmltree.Node, path []string) *xmltree.Node {
	t.Helper()
next:
	for _, step := range path {
		for _, c := range n.Children {
			if isStep(c, step) {
				n = c
				continue next
			}
		}
		n = newElem(t, n, step)
	}
	return n
}

// countPath returns the number of elements at path below n (count(path)).
func countPath(n *xmltree.Node, path []string) int {
	if len(path) == 0 {
		return 1
	}
	c := 0
	for _, k := range n.Children {
		if isStep(k, path[0]) {
			c += countPath(k, path[1:])
		}
	}
	return c
}

// setAttr sets the attribute name of n to value.
func setAttr(n *xmltree.Node, name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name && n.Attrs[i].Prefix == "" {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xmltree.Attr{Name: name, Value: value})
}

// writeXML writes n and its descendants as XML, with the namespace
// declarations decl on n, one element per line.
func writeXML(b *strings.Builder, n *xmltree.Node, decl string) {
	name := n.Name
	if n.Prefix != "" {
		name = n.Prefix + ":" + name
	}
	fmt.Fprintf(b, "<%s%s", name, decl)
	for _, a := range n.Attrs {
		an := a.Name
		if a.Prefix != "" {
			an = a.Prefix + ":" + an
		}
		fmt.Fprintf(b, ` %s="`, an)
		_ = xml.EscapeText(b, []byte(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	if len(n.Children) == 0 {
		_ = xml.EscapeText(b, []byte(n.Text))
	} else {
		b.WriteString("\n")
		for _, c := range n.Children {
			writeXML(b, c, "")
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(b, "</%s>", name)
}

// ublDocument returns the document root as UBL XML.
func ublDocument(root *xmltree.Node) string {
	var decl strings.Builder
	fmt.Fprintf(&decl, ` xmlns="%s"`, root.Space)
	for _, p := range []string{"cac", "cbc", "ext"} {
		fmt.Fprintf(&decl, ` xmlns:%s="%s"`, p, ublNamespaces[p])
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	writeXML(&b, root, decl.String())
	b.WriteString("\n")
	return b.String()
}

// syntaxCase is a document built from tInvoice (tCTCFR when ctcfr is set),
// or from its credit note when credit is set, by build, which places the
// faulty element when faulty is set, and the rule it must report.
type syntaxCase struct {
	name   string
	rule   string
	credit bool
	ctcfr  bool
	build  func(t *testing.T, root *xmltree.Node, faulty bool)
}

// scopeNode returns the node from which a path of scope s starts: the
// document element, its first line, or for crAny the parent of the first
// element named as the first step of path, the document element if there is
// none.
func scopeNode(t *testing.T, root *xmltree.Node, s crScope, path []string) *xmltree.Node {
	t.Helper()
	switch s {
	case crRoot:
		return root
	case crLine:
		for _, c := range root.Children {
			if isStep(c, "cac:InvoiceLine") || isStep(c, "cac:CreditNoteLine") {
				return c
			}
		}
		t.Fatal("no line")
	case crAny:
		var found *xmltree.Node
		if len(path) > 0 && !strings.HasPrefix(path[0], "@") {
			root.Walk(func(n *xmltree.Node) bool {
				if found == nil && n != root && isStep(n, path[0]) {
					found = n.Parent
				}
				return found == nil
			})
		}
		if found != nil {
			return found
		}
		return root
	}
	t.Fatalf("unknown scope %d", s)
	return nil
}

// buildNot returns the build function of the assertion that the element or
// attribute at path, from the node of scope s, must not exist.
func buildNot(s crScope, path string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		steps := splitPath(path)
		last := steps[len(steps)-1]
		n := descend(t, scopeNode(t, root, s, steps), steps[:len(steps)-1])
		if !faulty {
			return
		}
		if attr, ok := strings.CutPrefix(last, "@"); ok {
			setAttr(n, attr, "x")
		} else {
			newElem(t, n, last)
		}
	}
}

// buildCount returns the build function of the assertion count(path) <=
// limit from the node of scope s: the counted element appears limit times,
// or limit+1 times in the faulty document.
func buildCount(s crScope, path string, limit int) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		steps := splitPath(path)
		n := scopeNode(t, root, s, steps)
		parent := descend(t, n, steps[:len(steps)-1])
		want := limit
		if faulty {
			want++
		}
		c := countPath(n, steps)
		if c > want {
			t.Fatalf("tInvoice has %d %s", c, path)
		}
		for ; c < want; c++ {
			newElem(t, parent, steps[len(steps)-1])
		}
	}
}

// syntaxCases returns a case for each entry of crRules, ctcfrCRRules,
// forbiddenAttrs and srCases.
func syntaxCases() []syntaxCase {
	var cases []syntaxCase
	for _, cr := range crRules {
		cases = append(cases, syntaxCase{
			name:   cr.id + " " + cr.path,
			rule:   cr.id,
			credit: strings.Contains(cr.path, "CreditNote"),
			build:  buildNot(cr.scope, cr.path),
		})
	}
	for _, cr := range ctcfrCRRules {
		cases = append(cases, syntaxCase{
			name:   "EXTENDED-CTC-FR " + cr.id + " " + cr.path,
			rule:   cr.id,
			credit: strings.Contains(cr.path, "CreditNote"),
			ctcfr:  true,
			build:  buildNot(cr.scope, cr.path),
		})
	}
	for _, f := range forbiddenAttrs {
		cases = append(cases, syntaxCase{name: f.id + " @" + f.attr, rule: f.id, build: buildNot(crRoot, "cbc:ID/@"+f.attr)})
	}
	for _, sr := range srCases {
		c := syntaxCase{name: sr.rule + " " + sr.path, rule: sr.rule, build: buildCount(sr.scope, sr.path, sr.n)}
		if sr.n == 0 {
			c.build = buildNot(sr.scope, sr.path)
		}
		cases = append(cases, c)
	}
	return cases
}

// TestSyntaxTables checks that the faulty document of each syntax case
// reports its rule, only with its severity in Catalog (CTCFRCatalog for an
// EXTENDED-CTC-FR case), and that the document without the faulty element
// does not.
func TestSyntaxTables(t *testing.T) {
	for _, c := range syntaxCases() {
		t.Run(c.name, func(t *testing.T) {
			base, sevs := tInvoice, severity()
			if c.ctcfr {
				base, sevs = tCTCFR, ctcfrSeverity()
			}
			sev, ok := sevs[c.rule]
			if !ok {
				t.Fatalf("%s is not in the catalog", c.rule)
			}
			if c.credit {
				base = creditNote(base)
			}
			for _, faulty := range []bool{false, true} {
				root, err := xmltree.Parse([]byte(base))
				if err != nil {
					t.Fatal(err)
				}
				c.build(t, root, faulty)
				doc := ublDocument(root)
				r := check(t, doc)
				n := 0
				for _, f := range r.Findings {
					if f.Message == "" || f.Path == "" || f.Line == 0 {
						t.Errorf("incomplete finding %+v", f)
					}
					if f.Rule != c.rule {
						continue
					}
					n++
					if f.Severity != sev {
						t.Errorf("%s: severity %v, want %v", f, f.Severity, sev)
					}
				}
				switch {
				case faulty && n == 0:
					t.Errorf("%s not reported:\n%s\n%s", c.rule, r, doc)
				case !faulty && n > 0:
					t.Errorf("%s reported without the faulty element:\n%s\n%s", c.rule, r, doc)
				}
			}
		})
	}
}
