package en16931

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The syntax tests raise each CII syntax rule of the generated tables of the
// CEN schematron (syntaxNots and syntaxCounts) and of the EXTENDED-CTC-FR
// one (ctcfrNots and ctcfrCounts): for each entry and each alternative of
// its context, they build the elements of the context then of the path in
// tBase, place the element or the attribute that must not exist, or repeat
// the counted element once more than the rule allows, and check that the
// rule is reported with the severity of Catalog (CTCFRCatalog). The same
// document without the faulty element (the path up to its parent, or the
// counted element exactly as many times as allowed) must not report the
// rule. syntaxHandCases does the same for the syntax rules written by hand.

// ciiNamespaces maps the prefixes of the syntax paths to their namespace.
var ciiNamespaces = map[string]string{
	"rsm": rules.NSRsm,
	"ram": rules.NSRam,
	"udt": rules.NSUdt,
	"qdt": rules.NSQdt,
}

// kindNodes gives, for each kind of name context (kindContexts), the path
// from the document element of an element that the context selects and that
// is not the context of an absolute rule, so that the name rules run on it.
// The elements missing from tBase are built.
var kindNodes = map[kind]string{
	kindPaymentMeans:         "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeSettlementPaymentMeans",
	kindAllowanceCharge:      "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeAllowanceCharge",
	kindGrossAllowanceCharge: "rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeAgreement/ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge",
	kindDocContext:           "rsm:ExchangedDocumentContext/ram:GuidelineSpecifiedDocumentContextParameter",
	kindID:                   "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:SellerTradeParty/ram:SpecifiedTaxRegistration/ram:ID",
	kindTypeCode:             "rsm:ExchangedDocument/ram:TypeCode",
	kindRefDoc:               "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:ContractReferencedDocument",
	kindAmount:               "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeSettlementHeaderMonetarySummation/ram:LineTotalAmount",
	kindQuantity:             "rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeDelivery/ram:BilledQuantity",
	kindTradeTax:             "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax",
	kindPeriod:               "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:BillingSpecifiedPeriod",
	kindAddress:              "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement/ram:SellerTradeParty/ram:PostalTradeAddress",
}

// syntaxContexts returns the paths from the document element of the nodes on
// which a syntax rule of context ctx runs: the element of kindNodes for a
// name context, each alternative of an absolute context otherwise.
func syntaxContexts(t *testing.T, ctx string) []string {
	t.Helper()
	if kd, ok := kindContexts()[ctx]; ok {
		p, ok := kindNodes[kd]
		if !ok {
			t.Fatalf("no element for the name context %s", ctx)
		}
		return []string{p}
	}
	var out []string
	for alt := range strings.SplitSeq(ctx, "|") {
		alt = strings.TrimSpace(alt)
		p, ok := strings.CutPrefix(alt, "/rsm:CrossIndustryInvoice")
		if !ok {
			t.Fatalf("unsupported context %s", ctx)
		}
		out = append(out, strings.TrimPrefix(p, "/"))
	}
	return out
}

// splitPath returns the steps of a path ("ram:A/ram:B/@c"), none for "".
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// newElem appends to n an empty child element named step ("ram:Name").
func newElem(t *testing.T, n *xmltree.Node, step string) *xmltree.Node {
	t.Helper()
	prefix, local, _ := strings.Cut(step, ":")
	space, ok := ciiNamespaces[prefix]
	if !ok || strings.HasPrefix(step, "@") {
		t.Fatalf("unsupported step %q", step)
	}
	c := &xmltree.Node{Name: local, Prefix: prefix, Space: space, Parent: n}
	n.Children = append(n.Children, c)
	return c
}

// isStep reports whether n is the element named step ("ram:Name").
func isStep(n *xmltree.Node, step string) bool {
	prefix, local, _ := strings.Cut(step, ":")
	return n.Name == local && n.Space == ciiNamespaces[prefix]
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

// ciiDocument returns the document root as CII XML.
func ciiDocument(root *xmltree.Node) string {
	var decl strings.Builder
	for _, p := range []string{"rsm", "ram", "udt", "qdt"} {
		fmt.Fprintf(&decl, ` xmlns:%s="%s"`, p, ciiNamespaces[p])
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	writeXML(&b, root, decl.String())
	b.WriteString("\n")
	return b.String()
}

// syntaxCase is a document built from tBase, declared EXTENDED-CTC-FR when
// ctcfr is set, by build, which places the faulty element when faulty is
// set, and the rule it must report.
type syntaxCase struct {
	name  string
	rule  string
	ctcfr bool
	build func(t *testing.T, root *xmltree.Node, faulty bool)
}

// buildNot returns the build function of the assertion not(path) on the
// node at ctx.
func buildNot(ctx, path string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		steps := splitPath(path)
		last := steps[len(steps)-1]
		n := descend(t, descend(t, root, splitPath(ctx)), steps[:len(steps)-1])
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

// buildCount returns the build function of the assertion count(path) <= limit
// or count(path) = limit on the node at ctx: the counted element appears
// limit times, or limit+1 times in the faulty document.
func buildCount(ctx, path string, limit int) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		steps := splitPath(path)
		n := descend(t, root, splitPath(ctx))
		parent := descend(t, n, steps[:len(steps)-1])
		want := limit
		if faulty {
			want++
		}
		c := countPath(n, steps)
		if c > want {
			t.Fatalf("tBase has %d %s", c, path)
		}
		for ; c < want; c++ {
			newElem(t, parent, steps[len(steps)-1])
		}
	}
}

// syntaxTableCases returns a case for each entry of the tables nots and
// counts and each alternative of its context.
func syntaxTableCases(t *testing.T, nots []syntaxNot, counts []syntaxCount, ctcfr bool) []syntaxCase {
	var cases []syntaxCase
	add := func(rule, ctx, path string, build func(string) func(*testing.T, *xmltree.Node, bool)) {
		for i, c := range syntaxContexts(t, ctx) {
			name := rule + " " + path
			if i > 0 {
				name = fmt.Sprintf("%s (context %d)", name, i+1)
			}
			cases = append(cases, syntaxCase{name: name, rule: rule, ctcfr: ctcfr, build: build(c)})
		}
	}
	for _, r := range nots {
		add(r.Rule, r.Context, r.Path, func(ctx string) func(*testing.T, *xmltree.Node, bool) {
			return buildNot(ctx, r.Path)
		})
	}
	for _, r := range counts {
		add(r.Rule, r.Context, r.Path, func(ctx string) func(*testing.T, *xmltree.Node, bool) {
			return buildCount(ctx, r.Path, r.N)
		})
	}
	return cases
}

// runSyntaxCase checks that the faulty document of c reports c.rule, only
// with its severity in Catalog (CTCFRCatalog for an EXTENDED-CTC-FR case),
// and that the document without the faulty element does not.
func runSyntaxCase(t *testing.T, c syntaxCase) {
	t.Helper()
	base := tBase
	sev, ok := severity()[c.rule]
	if c.ctcfr {
		base = strings.Replace(base, "urn:cen.eu:en16931:2017", invoice.URNExtendedCTCFR, 1)
		r := ctcfrRules()[c.rule]
		ok = r != nil
		if ok {
			sev = r.Severity
		}
	}
	if !ok {
		t.Fatalf("%s is not in the catalog", c.rule)
	}
	for _, faulty := range []bool{false, true} {
		root, err := xmltree.Parse([]byte(base))
		if err != nil {
			t.Fatal(err)
		}
		c.build(t, root, faulty)
		doc := ciiDocument(root)
		r, _ := findings(t, doc, invoice.ProfileUnknown)
		n := 0
		for _, f := range r.Findings {
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
}

// TestSyntaxTables raises each rule of the tables of the CEN schematron
// (syntaxNots, syntaxCounts) and of the EXTENDED-CTC-FR one (ctcfrNots,
// ctcfrCounts), the latter on tBase declared EXTENDED-CTC-FR.
func TestSyntaxTables(t *testing.T) {
	for _, c := range syntaxTableCases(t, syntaxNots, syntaxCounts, false) {
		t.Run(c.name, func(t *testing.T) { runSyntaxCase(t, c) })
	}
	for _, c := range syntaxTableCases(t, ctcfrNots, ctcfrCounts, true) {
		t.Run("EXTENDED-CTC-FR "+c.name, func(t *testing.T) { runSyntaxCase(t, c) })
	}
}

// Paths from the document element of tBase.
const (
	pAgreement      = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement"
	pDelivery       = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeDelivery"
	pSettlement     = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement"
	pProduct        = "rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedTradeProduct"
	pLineSettlement = "rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeSettlement"
	pContractRef    = pAgreement + "/ram:ContractReferencedDocument"
	pLineTax        = pLineSettlement + "/ram:ApplicableTradeTax"
	pAllowanceTax   = pSettlement + "/ram:SpecifiedTradeAllowanceCharge/ram:CategoryTradeTax"
	pHeaderTaxType  = pSettlement + "/ram:ApplicableTradeTax/ram:TypeCode"
	pHeaderTaxDue   = pSettlement + "/ram:ApplicableTradeTax/ram:DueDateTypeCode"
	pLineTaxDue     = pLineTax + "/ram:DueDateTypeCode"
)

// Paths from a ram:SpecifiedTradeSettlementPaymentMeans.
const (
	pPayeeFinancial  = "ram:PayeePartyCreditorFinancialAccount/ram:IBANID"
	pFinancialCardID = "ram:ApplicableTradeSettlementFinancialCard/ram:ID"
	pDebtorAccountID = "ram:PayerPartyDebtorFinancialAccount/ram:IBANID"
)

// addElem appends to n the element step with text, and returns it.
func addElem(t *testing.T, n *xmltree.Node, step, text string) *xmltree.Node {
	t.Helper()
	c := newElem(t, n, step)
	c.Text = text
	return c
}

// addPath appends to the node at path ctx below n (built when missing) the
// elements of path, the last one with text, and returns the last one.
func addPath(t *testing.T, n *xmltree.Node, ctx, path, text string) *xmltree.Node {
	t.Helper()
	steps := splitPath(path)
	n = descend(t, n, splitPath(ctx))
	for _, s := range steps[:len(steps)-1] {
		n = newElem(t, n, s)
	}
	return addElem(t, n, steps[len(steps)-1], text)
}

// refDocs returns the build function that adds to the node at ctx one
// additional referenced document of type code, two in the faulty document.
func refDocs(ctx, code string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		n := 1
		if faulty {
			n = 2
		}
		parent := descend(t, root, splitPath(ctx))
		for i := range n {
			d := newElem(t, parent, "ram:AdditionalReferencedDocument")
			addElem(t, d, "ram:IssuerAssignedID", fmt.Sprintf("D%d", i+1))
			addElem(t, d, "ram:TypeCode", code)
		}
	}
}

// paymentMeans returns the build function that adds payment means of the
// type codes and the texts (BT-82) infos, one per payment means, the faulty
// ones in the faulty document.
func paymentMeans(codes, faultyCodes, infos, faultyInfos []string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		if faulty {
			codes, infos = faultyCodes, faultyInfos
		}
		settl := descend(t, root, splitPath(pSettlement))
		for i, code := range codes {
			pm := newElem(t, settl, "ram:SpecifiedTradeSettlementPaymentMeans")
			addElem(t, pm, "ram:TypeCode", code)
			if i < len(infos) {
				addElem(t, pm, "ram:Information", infos[i])
			}
		}
	}
}

// attachment returns the build function that adds a supporting document
// (BG-24) with an attached document carrying the attributes attrs, all but
// faultyLack in the faulty document.
func attachment(faultyLack string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		d := addPath(t, root, pAgreement, "ram:AdditionalReferencedDocument", "")
		addElem(t, d, "ram:IssuerAssignedID", "D1")
		addElem(t, d, "ram:TypeCode", "916")
		a := addElem(t, d, "ram:AttachmentBinaryObject", "AA==")
		for _, attr := range [][2]string{{"mimeCode", "application/pdf"}, {"filename", "d1.pdf"}} {
			if !faulty || attr[0] != faultyLack {
				setAttr(a, attr[0], attr[1])
			}
		}
	}
}

// twoIDs returns the build function that adds party below the node at ctx,
// with an ID, and a GlobalID in the faulty document.
func twoIDs(ctx, party string) func(*testing.T, *xmltree.Node, bool) {
	return func(t *testing.T, root *xmltree.Node, faulty bool) {
		p := addPath(t, root, ctx, party, "")
		addElem(t, p, "ram:ID", "P1")
		if faulty {
			setAttr(addElem(t, p, "ram:GlobalID", "4000001123452"), "schemeID", "0088")
		}
		addElem(t, p, "ram:Name", "Party")
	}
}

// syntaxHandCases returns a case for each CII syntax rule written by hand
// in syntax.go, which the tables do not describe: an element admitted in
// some places only, a count over several nodes, a value.
func syntaxHandCases() []syntaxCase {
	return []syntaxCase{
		// Data of an additional supporting document of type 916 only.
		{name: "CII-DT-021 contract reference name", rule: "CII-DT-021", build: buildNot(pContractRef, "ram:Name")},
		{name: "CII-DT-022 contract reference attachment", rule: "CII-DT-022", build: buildNot(pContractRef, "ram:AttachmentBinaryObject")},
		{name: "CII-DT-024 contract reference type code", rule: "CII-DT-024", build: buildNot(pContractRef, "ram:ReferenceTypeCode")},
		{name: "CII-DT-027 contract reference issue date", rule: "CII-DT-027", build: buildNot(pContractRef, "ram:FormattedIssueDateTime")},
		{name: "CII-DT-105 attachment without MIME code", rule: "CII-DT-105", build: attachment("mimeCode")},
		{name: "CII-DT-106 attachment without file name", rule: "CII-DT-106", build: attachment("filename")},
		// Data of the header VAT breakdown, or of ApplicableTradeTax only.
		{name: "CII-DT-037 tax type code", rule: "CII-DT-037", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			if faulty {
				descend(t, root, splitPath(pHeaderTaxType)).Text = "GST"
			}
		}},
		{name: "CII-DT-041 line tax basis", rule: "CII-DT-041", build: buildNot(pLineTax, "ram:BasisAmount")},
		{name: "CII-DT-054 line tax point date", rule: "CII-DT-054", build: buildNot(pLineTax, "ram:TaxPointDate")},
		{name: "CII-DT-058 line tax due date code", rule: "CII-DT-058", build: buildNot(pLineTax, "ram:DueDateTypeCode")},
		{name: "CII-DT-052 allowance exemption reason code", rule: "CII-DT-052", build: buildNot(pAllowanceTax, "ram:ExemptionReasonCode")},
		{name: "CII-DT-098 allowance exemption reason", rule: "CII-DT-098", build: buildNot(pAllowanceTax, "ram:ExemptionReason")},
		{name: "CII-SR-462 due date codes", rule: "CII-SR-462", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			descend(t, root, splitPath(pHeaderTaxDue)).Text = "5"
			code := "5"
			if faulty {
				code = "29"
			}
			descend(t, root, splitPath(pLineTaxDue)).Text = code
		}},
		// Parties with both an identifier and a global identifier.
		{name: "CII-SR-449 ship-to identifiers", rule: "CII-SR-449", build: twoIDs(pDelivery, "ram:ShipToTradeParty")},
		{name: "CII-SR-451 payee identifiers", rule: "CII-SR-451", build: twoIDs(pSettlement, "ram:PayeeTradeParty")},
		{name: "CII-SR-046 item global identifier scheme", rule: "CII-SR-046", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			id := addPath(t, root, pProduct, "ram:GlobalID", "4000001123452")
			if !faulty {
				setAttr(id, "schemeID", "0160")
			}
		}},
		// Additional referenced documents of the same type code.
		{name: "CII-SR-457 tender references", rule: "CII-SR-457", build: refDocs(pAgreement, "50")},
		{name: "CII-SR-458 invoiced object references", rule: "CII-SR-458", build: refDocs(pAgreement, "130")},
		{name: "CII-SR-474 line object references", rule: "CII-SR-474", build: refDocs(pLineSettlement, "130")},
		// Payment instructions.
		{name: "CII-SR-467 payment means type codes", rule: "CII-SR-467", build: paymentMeans([]string{"10", "10"}, []string{"10", "97"}, nil, nil)},
		{name: "CII-SR-468 payment means texts", rule: "CII-SR-468", build: paymentMeans([]string{"10", "10"}, []string{"10", "10"}, []string{"Cash", "Cash"}, []string{"Cash", "Coins"})},
		{name: "CII-SR-470 credit transfer account", rule: "CII-SR-470", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			pm := addPath(t, root, pSettlement, "ram:SpecifiedTradeSettlementPaymentMeans", "")
			addElem(t, pm, "ram:TypeCode", "30")
			if !faulty {
				addPath(t, pm, "", pPayeeFinancial, "DE89370400440532013000")
			}
		}},
		{name: "CII-SR-495 payment cards", rule: "CII-SR-495", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			pm := addPath(t, root, pSettlement, "ram:SpecifiedTradeSettlementPaymentMeans", "")
			addElem(t, pm, "ram:TypeCode", "48")
			addPath(t, pm, "", pFinancialCardID, "1234")
			if faulty {
				addPath(t, pm, "", pFinancialCardID, "5678")
			}
		}},
		{name: "CII-SR-496 payment mandates", rule: "CII-SR-496", build: func(t *testing.T, root *xmltree.Node, faulty bool) {
			pm := addPath(t, root, pSettlement, "ram:SpecifiedTradeSettlementPaymentMeans", "")
			addElem(t, pm, "ram:TypeCode", "59")
			addPath(t, pm, "", pDebtorAccountID, "DE89370400440532013000")
			if faulty {
				addPath(t, pm, "", pDebtorAccountID, "DE02120300000000202051")
			}
		}},
	}
}

// TestSyntaxHand raises each CII syntax rule of syntaxHandCases.
func TestSyntaxHand(t *testing.T) {
	for _, c := range syntaxHandCases() {
		t.Run(c.name, func(t *testing.T) { runSyntaxCase(t, c) })
	}
}

// TestSyntaxKindNodes checks that each element of kindNodes is of its kind
// and is not the context of an absolute syntax rule of the CEN or the
// EXTENDED-CTC-FR schematron, so that the name rules of its kind run on it.
func TestSyntaxKindNodes(t *testing.T) {
	for kd := kindNone + 1; kd < nKinds; kd++ {
		path, ok := kindNodes[kd]
		if !ok {
			t.Errorf("no element for kind %d", kd)
			continue
		}
		root, err := xmltree.Parse([]byte(tBase))
		if err != nil {
			t.Fatal(err)
		}
		if n := descend(t, root, splitPath(path)); nodeKind(n) != kd {
			t.Errorf("%s: kind %d, want %d", path, nodeKind(n), kd)
		}
		for _, set := range []*syntaxSet{cenSyntax(), ctcfrSyntax()} {
			tn := set.trie
			for _, step := range splitPath(path) {
				_, local, _ := strings.Cut(step, ":")
				if tn = tn.child(local); tn == nil {
					break
				}
			}
			if tn != nil && tn.ctx {
				t.Errorf("%s is the context of an absolute rule (EXTENDED-CTC-FR: %v)", path, set.ctcfr)
			}
		}
	}
}
