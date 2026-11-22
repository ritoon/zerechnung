package en16931

import (
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// walk visits n and its descendants in document order. t is the trie node of
// n (nil when n is not on an absolute path of the syntax rules).
func (k *checker) walk(n *xmltree.Node, t *tnode, depth int) {
	if depth >= len(k.stack) {
		return
	}
	if depth == 0 && (t == nil || t.name != n.Name) {
		t = nil
	}
	k.stack[depth] = n
	k.depth = depth
	ctx := false
	if t != nil {
		k.trieChecks(n, t)
		ctx = t.ctx
	}
	if !ctx {
		k.kindChecks(n)
	}
	k.visit(n)
	for _, c := range n.Children {
		var tc *tnode
		if t != nil {
			tc = t.child(c.Name)
		}
		k.walk(c, tc, depth+1)
	}
}

// visit runs the model and code list rules whose context is n, and collects
// the facts needed by the document level rules.
func (k *checker) visit(n *xmltree.Node) {
	if n.Parent == nil {
		return
	}
	switch n.Name {
	// Code lists and facts.
	case "TypeCode":
		switch n.Parent.Name {
		case "ExchangedDocument":
			k.codeList("BR-CL-01", n, n.Text)
		case "SpecifiedTradeSettlementPaymentMeans":
			k.codeList("BR-CL-16", n, n.Text)
		case "ApplicableProductCharacteristic":
			if k.mode == modeExtendedCTCFR {
				k.codeList("BR-FXEXT-04", n, n.Text)
			}
		}
	case "TaxTotalAmount":
		if v, ok := n.LookupAttr("currencyID"); ok {
			k.codeList("BR-CL-03", n, v)
		}
	case "InvoiceCurrencyCode":
		k.codeList("BR-CL-04", n, n.Text)
	case "TaxCurrencyCode":
		k.codeList("BR-CL-05", n, n.Text)
	case "DueDateTypeCode":
		k.codeList("BR-CL-06", n, n.Text)
		k.dueDateTypeCode = true
		if n.Parent.Name == "ApplicableTradeTax" {
			switch {
			case k.dueCodes == 0:
				k.dueCode, k.dueCodes = n.Text, 1
			case n.Text != k.dueCode:
				k.dueCodes = 2
			}
		}
	case "ReferenceTypeCode":
		k.codeList("BR-CL-07", n, n.Text)
	case "SubjectCode":
		k.codeList("BR-CL-08", n, n.Text)
	case "GlobalID":
		if v, ok := n.LookupAttr("schemeID"); ok {
			switch {
			case n.Parent.Name == "SpecifiedTradeProduct":
				k.codeList("BR-CL-21", n, v)
			case k.hasAncestor("SpecifiedTradeProduct"):
			case k.hasAncestor("ShipToTradeParty"):
				if k.parentIs("ShipToTradeParty", "ApplicableHeaderTradeDelivery") {
					k.codeList("BR-CL-26", n, v)
				}
			default:
				k.codeList("BR-CL-10", n, v)
			}
		}
	case "ID":
		v, scheme := n.LookupAttr("schemeID")
		switch {
		case scheme && !k.hasAncestor("SpecifiedTaxRegistration"):
			k.codeList("BR-CL-11", n, v)
		case n.Parent.Name == "OriginTradeCountry":
			k.codeList("BR-CL-15", n, n.Text)
		}
		if scheme && v == "VA" && n.Parent.Name == "SpecifiedTaxRegistration" {
			k.vatPrefix(n)
		}
	case "ClassCode":
		if v, ok := n.LookupAttr("listID"); ok {
			k.codeList("BR-CL-13", n, v)
		}
	case "CountryID":
		k.codeList("BR-CL-14", n, n.Text)
		if n.Text != "IT" {
			k.countryNotIT = true
		}
	case "CategoryCode":
		switch n.Parent.Name {
		case "CategoryTradeTax":
			k.codeList("BR-CL-17", n, n.Text)
		case "ApplicableTradeTax":
			k.codeList("BR-CL-18", n, n.Text)
		}
		if i := catIndex(n.Text); i >= 0 {
			k.anyCode[i] = true
		}
	case "ReasonCode":
		if k.mode == modeExtendedCTCFR {
			k.ctcfrReasonCode(n)
			break
		}
		if n.Parent.Name == "SpecifiedTradeAllowanceCharge" {
			allowance, charge := chargeIndicator(n.Parent)
			switch {
			case allowance:
				k.codeList("BR-CL-19", n, n.Text)
			case charge:
				k.codeList("BR-CL-20", n, n.Text)
			}
		}
	case "ExemptionReasonCode":
		k.codeList("BR-CL-22", n, n.Text)
	case "BasisQuantity", "BilledQuantity":
		if v, ok := n.LookupAttr("unitCode"); ok {
			k.codeList("BR-CL-23", n, v)
		}
	case "AttachmentBinaryObject":
		if v, ok := n.LookupAttr("mimeCode"); ok {
			k.codeList("BR-CL-24", n, v)
		}
	case "URIID":
		if n.Parent.Name == "URIUniversalCommunication" {
			if v, ok := n.LookupAttr("schemeID"); ok {
				k.codeList("BR-CL-25", n, v)
			}
		}
	case "DeliveryTypeCode":
		if k.mode == modeExtendedCTCFR && n.Parent.Name == "ApplicableTradeDeliveryTerms" {
			k.codeList("BR-FREXT-CL-27", n, n.Text)
		}
	case "TaxPointDate":
		k.taxPointDate = true
	case "PaymentReference":
		if n.Parent.Name == "ApplicableHeaderTradeSettlement" {
			k.paymentRefs++
		}

	// Model rules.
	case "DesignatedProductClassification":
		k.productClassification(n)
	case "AdditionalReferencedDocument":
		if blank(n.Elem("IssuerAssignedID")) {
			k.fail("BR-52", n, "an additional supporting document (BG-24) has no supporting document reference (BT-122)")
		}
	case "ApplicableTradeSettlementFinancialCard":
		k.financialCard(n)
	case "PayerPartyDebtorFinancialAccount":
		if n.Parent.Name == "SpecifiedTradeSettlementPaymentMeans" {
			k.debtorAccounts++
		}
	case "SpecifiedTradeSettlementPaymentMeans":
		k.paymentMeans(n)
	case "SpecifiedTradeAllowanceCharge":
		switch n.Parent.Name {
		case "ApplicableHeaderTradeSettlement":
			k.documentAllowanceCharge(n)
		case "SpecifiedLineTradeSettlement":
			k.lineAllowanceCharge(n)
		}
	case "CategoryTradeTax":
		k.categoryTradeTax(n)
	case "ApplicableTradeTax":
		k.applicableTradeTax(n)
	case "BillingSpecifiedPeriod":
		switch n.Parent.Name {
		case "SpecifiedLineTradeSettlement":
			k.period(n, "BR-30", "BR-CO-20", "invoice line period (BG-26)")
		case "ApplicableHeaderTradeSettlement":
			k.period(n, "BR-29", "BR-CO-19", "invoicing period (BG-14)")
		}
	case "ApplicableProductCharacteristic":
		if k.mode == modeExtendedCTCFR {
			k.ctcfrCharacteristic(n)
		} else if n.Elem("Description") == nil || n.Elem("Value") == nil {
			k.fail("BR-54", n, "an item attribute (BG-32) lacks its name (BT-160) or its value (BT-161)")
		}
	case "PayeeTradeParty":
		k.payee(n)
	case "SellerTradeParty":
		k.sellerIdentifiers(n)
	case "SellerTaxRepresentativeTradeParty":
		k.taxRepresentative(n)
	case "SpecifiedTradeSettlementHeaderMonetarySummation":
		k.summ.add(n)
	case "ApplicableHeaderTradeSettlement":
		k.settl.add(n)
	case "IncludedSupplyChainTradeLineItem":
		k.line(n)
		if k.mode == modeExtendedCTCFR {
			k.ctcfrLine(n)
		}
	}
}

// finish runs the rules that need facts collected over the whole document.
func (k *checker) finish() {
	k.syntaxRoot()
	k.document()
	for i := 0; i < k.summ.n; i++ {
		k.summation(k.summ.at(i))
	}
	for i := 0; i < k.settl.n; i++ {
		if k.mode == modeExtendedCTCFR {
			k.ctcfrBreakdowns(k.settl.at(i))
		} else {
			k.breakdowns(k.settl.at(i))
		}
	}
}
