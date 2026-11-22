package en16931

import (
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Rules of the pattern EN16931-CII-Model, except the VAT category rules
// (vat.go).

// lineID returns the identifier of a line for messages.
func lineID(line *xmltree.Node) string {
	return quote(text(find(line, "AssociatedDocumentLineDocument", "LineID")))
}

// productClassification checks BR-65 (context //ram:DesignatedProductClassification).
func (k *checker) productClassification(n *xmltree.Node) {
	code := n.Elem("ClassCode")
	if code == nil {
		return
	}
	for it, c := elems(n, "ClassCode"); c != nil; c = it.next() {
		if v, ok := c.LookupAttr("listID"); ok {
			if trim(v) == "" {
				break
			}
			return
		}
	}
	k.fail("BR-65", code, "item classification identifier (BT-158) %q has no scheme identifier (listID)", quote(code.Text))
}

// normLen returns string-length(normalize-space(s)).
func normLen(s string) int {
	n, space := 0, false
	for _, r := range trim(s) {
		if r < 0x80 && isSpace(byte(r)) {
			space = true
			continue
		}
		if space {
			n++
			space = false
		}
		n++
	}
	return n
}

// normEqual reports whether normalize-space(a) = normalize-space(b).
func normEqual(a, b string) bool {
	a, b = trim(a), trim(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isSpace(a[i]) || isSpace(b[j]) {
			if !isSpace(a[i]) || !isSpace(b[j]) {
				return false
			}
			for i < len(a) && isSpace(a[i]) {
				i++
			}
			for j < len(b) && isSpace(b[j]) {
				j++
			}
			continue
		}
		if a[i] != b[j] {
			return false
		}
		i++
		j++
	}
	return i == len(a) && j == len(b)
}

// financialCard checks BR-51 (context //ram:ApplicableTradeSettlementFinancialCard).
func (k *checker) financialCard(n *xmltree.Node) {
	if n.Parent.Name == "SpecifiedTradeSettlementPaymentMeans" {
		k.financialCards++
	}
	if id := n.Elem("ID"); id != nil && normLen(id.Text) > 10 {
		k.fail("BR-51", id, "payment card primary account number (BT-87) has %d characters, at most 10 may be shown", normLen(id.Text))
	}
}

// paymentMeans checks BR-49, BR-50 and BR-61, and collects the facts of
// CII-SR-467 and CII-SR-468.
func (k *checker) paymentMeans(pm *xmltree.Node) {
	tc := pm.Elem("TypeCode")
	if tc == nil {
		k.fail("BR-49", pm, "a payment instruction (BG-16) has no payment means type code (BT-81)")
	}
	for it, t := elems(pm, "TypeCode"); t != nil; t = it.next() {
		if k.pmType == nil {
			k.pmType = t
		} else if !normEqual(t.Text, k.pmType.Text) {
			k.pmTypeDiffer = true
		}
	}
	for it, t := elems(pm, "Information"); t != nil; t = it.next() {
		if k.pmInfo == nil {
			k.pmInfo = t
		} else if !normEqual(t.Text, k.pmInfo.Text) {
			k.pmInfoDiffer = true
		}
	}
	if !childText(pm, "TypeCode", "30") && !childText(pm, "TypeCode", "58") {
		return
	}
	for it, acc := elems(pm, "PayeePartyCreditorFinancialAccount"); acc != nil; acc = it.next() {
		iban, prop := acc.Elem("IBANID"), acc.Elem("ProprietaryID")
		if blank(iban) && blank(prop) {
			k.fail("BR-50", acc, "credit transfer (payment means %s) without payment account identifier (BT-84)", quote(tc.Text))
		}
		if iban == nil && prop == nil {
			k.fail("BR-61", acc, "credit transfer (payment means %s) without payment account identifier (BT-84)", quote(tc.Text))
		}
	}
}

// documentAllowanceCharge checks the document level allowance (BG-20) and
// charge (BG-21) rules (contexts
// //ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeAllowanceCharge/ram:ChargeIndicator[udt:Indicator='false'|'true']).
func (k *checker) documentAllowanceCharge(ac *xmltree.Node) {
	for it, ci := elems(ac, "ChargeIndicator"); ci != nil; ci = it.next() {
		allowance := childText(ci, "Indicator", "false")
		charge := !allowance && childText(ci, "Indicator", "true")
		if !allowance && !charge {
			continue
		}
		amount := ac.Elem("ActualAmount")
		hasCategory := false
		for it, t := elems(ac, "CategoryTradeTax"); t != nil; t = it.next() {
			if isVAT(t) && t.Elem("CategoryCode") != nil {
				hasCategory = true
			}
		}
		hasReason := ac.Elem("Reason") != nil || ac.Elem("ReasonCode") != nil
		basis := ac.Elem("BasisAmount")
		if allowance {
			if amount == nil {
				k.fail("BR-31", ac, "a document level allowance (BG-20) has no amount (BT-92)")
			}
			if !hasCategory {
				k.fail("BR-32", ac, "a document level allowance (BG-20) has no VAT category code (BT-95)")
			}
			if !hasReason {
				k.fail("BR-33", ac, "a document level allowance (BG-20) has neither a reason (BT-97) nor a reason code (BT-98)")
				k.fail("BR-CO-21", ac, "a document level allowance (BG-20) has neither a reason (BT-97) nor a reason code (BT-98)")
			}
			if tooManyDecimals(amount) {
				k.fail("BR-DEC-01", amount, "document level allowance amount (BT-92) %q has more than 2 decimals", quote(amount.Text))
			}
			if tooManyDecimals(basis) {
				k.fail("BR-DEC-02", basis, "document level allowance base amount (BT-93) %q has more than 2 decimals", quote(basis.Text))
			}
		} else {
			if amount == nil {
				k.fail("BR-36", ac, "a document level charge (BG-21) has no amount (BT-99)")
			}
			if !hasCategory {
				k.fail("BR-37", ac, "a document level charge (BG-21) has no VAT category code (BT-102)")
			}
			if !hasReason {
				k.fail("BR-38", ac, "a document level charge (BG-21) has neither a reason (BT-104) nor a reason code (BT-105)")
				k.fail("BR-CO-22", ac, "a document level charge (BG-21) has neither a reason (BT-104) nor a reason code (BT-105)")
			}
			if tooManyDecimals(amount) {
				k.fail("BR-DEC-05", amount, "document level charge amount (BT-99) %q has more than 2 decimals", quote(amount.Text))
			}
			if tooManyDecimals(basis) {
				k.fail("BR-DEC-06", basis, "document level charge base amount (BT-100) %q has more than 2 decimals", quote(basis.Text))
			}
		}
	}
}

// lineAllowanceCharge checks the invoice line allowance (BG-27) and charge
// (BG-28) rules (contexts
// //ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeAllowanceCharge/ram:ChargeIndicator[udt:Indicator = 'false'|'true']).
func (k *checker) lineAllowanceCharge(ac *xmltree.Node) {
	for it, ci := elems(ac, "ChargeIndicator"); ci != nil; ci = it.next() {
		allowance := childText(ci, "Indicator", "false")
		charge := !allowance && childText(ci, "Indicator", "true")
		if !allowance && !charge {
			continue
		}
		amount := ac.Elem("ActualAmount")
		hasReason := ac.Elem("Reason") != nil || ac.Elem("ReasonCode") != nil
		basis := ac.Elem("BasisAmount")
		if allowance {
			if amount == nil {
				k.fail("BR-41", ac, "an invoice line allowance (BG-27) has no amount (BT-136)")
			}
			if !hasReason {
				k.fail("BR-42", ac, "an invoice line allowance (BG-27) has neither a reason (BT-139) nor a reason code (BT-140)")
				k.fail("BR-CO-23", ac, "an invoice line allowance (BG-27) has neither a reason (BT-139) nor a reason code (BT-140)")
			}
			if tooManyDecimals(amount) {
				k.fail("BR-DEC-24", amount, "invoice line allowance amount (BT-136) %q has more than 2 decimals", quote(amount.Text))
			}
			if tooManyDecimals(basis) {
				k.fail("BR-DEC-25", basis, "invoice line allowance base amount (BT-137) %q has more than 2 decimals", quote(basis.Text))
			}
		} else {
			if amount == nil {
				k.fail("BR-43", ac, "an invoice line charge (BG-28) has no amount (BT-141)")
			}
			if !hasReason {
				k.fail("BR-44", ac, "an invoice line charge (BG-28) has neither a reason (BT-144) nor a reason code (BT-145)")
				k.fail("BR-CO-24", ac, "an invoice line charge (BG-28) has neither a reason (BT-144) nor a reason code (BT-145)")
			}
			if tooManyDecimals(amount) {
				k.fail("BR-DEC-27", amount, "invoice line charge amount (BT-141) %q has more than 2 decimals", quote(amount.Text))
			}
			if tooManyDecimals(basis) {
				k.fail("BR-DEC-28", basis, "invoice line charge base amount (BT-142) %q has more than 2 decimals", quote(basis.Text))
			}
		}
	}
}

// period checks BR-29/BR-CO-19 (invoicing period) or BR-30/BR-CO-20 (line
// period).
func (k *checker) period(n *xmltree.Node, order, filled, what string) {
	hasStart, hasEnd := n.Elem("StartDateTime") != nil, n.Elem("EndDateTime") != nil
	if !hasStart && !hasEnd {
		k.fail(filled, n, "%s has neither a start date nor an end date", what)
		return
	}
	if !hasStart || !hasEnd {
		return
	}
	// (EndDateTime/udt:DateTimeString[@format='102']) >=
	// (StartDateTime/udt:DateTimeString[@format='102']): untyped values
	// compared as strings, true when any pair satisfies it.
	var s0, e0 string
	for it, e := elems(n, "EndDateTime"); e != nil; e = it.next() {
		for it, ed := elems(e, "DateTimeString"); ed != nil; ed = it.next() {
			if f, _ := ed.LookupAttr("format"); f != "102" {
				continue
			}
			e0 = ed.Text
			for it, s := elems(n, "StartDateTime"); s != nil; s = it.next() {
				for it, sd := elems(s, "DateTimeString"); sd != nil; sd = it.next() {
					if f, _ := sd.LookupAttr("format"); f != "102" {
						continue
					}
					s0 = sd.Text
					if ed.Text >= sd.Text {
						return
					}
				}
			}
		}
	}
	k.fail(order, n, "%s ends (%q) before it starts (%q)", what, quote(e0), quote(s0))
}

// payee checks BR-17 (context //ram:PayeeTradeParty).
func (k *checker) payee(n *xmltree.Node) {
	name := n.Elem("Name")
	seller := n.Parent.Parent.Elem("ApplicableHeaderTradeAgreement").Elem("SellerTradeParty")
	switch {
	case name == nil:
		k.fail("BR-17", n, "the payee (BG-10) has no name (BT-59)")
	case sameText(n, seller, "Name"):
		k.fail("BR-17", n, "the payee (BG-10) has the name of the seller (%q): a payee different from the seller must be named", quote(name.Text))
	case sameText(n, seller, "ID"):
		k.fail("BR-17", n, "the payee (BG-10) has the identifier of the seller")
	case sameLegalID(n, seller):
		k.fail("BR-17", n, "the payee (BG-10) has the legal registration identifier of the seller")
	}
}

// sameText reports whether a child of a named name has the text of a child of
// b named name (XPath general comparison a/name = b/name).
func sameText(a, b *xmltree.Node, name string) bool {
	for it, x := elems(a, name); x != nil; x = it.next() {
		for it, y := elems(b, name); y != nil; y = it.next() {
			if x.Text == y.Text {
				return true
			}
		}
	}
	return false
}

// sameLegalID reports whether a/ram:SpecifiedLegalOrganization/ram:ID =
// b/ram:SpecifiedLegalOrganization/ram:ID.
func sameLegalID(a, b *xmltree.Node) bool {
	for it, x := elems(a, "SpecifiedLegalOrganization"); x != nil; x = it.next() {
		for it, y := elems(b, "SpecifiedLegalOrganization"); y != nil; y = it.next() {
			if sameText(x, y, "ID") {
				return true
			}
		}
	}
	return false
}

// sellerIdentifiers checks BR-CO-26 (context //ram:SellerTradeParty).
func (k *checker) sellerIdentifiers(n *xmltree.Node) {
	if n.Elem("ID") != nil || n.Elem("GlobalID") != nil || exists(n, "SpecifiedLegalOrganization", "ID") || hasTaxRegistration(n, "VA") {
		return
	}
	k.fail("BR-CO-26", n, "the seller has no identifier (BT-29), legal registration identifier (BT-30) or VAT identifier (BT-31)")
}

// taxRepresentative checks BR-18, BR-19, BR-20 and BR-56 (context
// //ram:SellerTaxRepresentativeTradeParty).
func (k *checker) taxRepresentative(n *xmltree.Node) {
	if blank(n.Elem("Name")) {
		k.fail("BR-18", n, "the seller tax representative (BG-11) has no name (BT-62)")
	}
	addr := n.Elem("PostalTradeAddress")
	if addr == nil {
		k.fail("BR-19", n, "the seller tax representative (BG-11) has no postal address (BG-12)")
	}
	if blank(addr.Elem("CountryID")) {
		k.fail("BR-20", n, "the seller tax representative postal address (BG-12) has no country code (BT-69)")
	}
	var vat *xmltree.Node
	for it, r := elems(n, "SpecifiedTaxRegistration"); r != nil; r = it.next() {
		for it, id := elems(r, "ID"); id != nil; id = it.next() {
			if s, ok := id.LookupAttr("schemeID"); ok && s == "VA" && vat == nil {
				vat = id
			}
		}
	}
	if blank(vat) {
		k.fail("BR-56", n, "the seller tax representative (BG-11) has no VAT identifier (BT-63)")
	}
}

// line checks the invoice line rules (context //ram:IncludedSupplyChainTradeLineItem).
func (k *checker) line(n *xmltree.Node) {
	id := lineID(n)
	if blank(find(n, "AssociatedDocumentLineDocument", "LineID")) {
		k.fail("BR-21", n, "an invoice line (BG-25) has no line identifier (BT-126)")
	}
	qty := find(n, "SpecifiedLineTradeDelivery", "BilledQuantity")
	if qty == nil {
		k.fail("BR-22", n, "line %q has no invoiced quantity (BT-129)", id)
	}
	unit := false
	for it, del := elems(n, "SpecifiedLineTradeDelivery"); del != nil; del = it.next() {
		for it, q := elems(del, "BilledQuantity"); q != nil; q = it.next() {
			if _, ok := q.LookupAttr("unitCode"); ok {
				unit = true
			}
		}
	}
	if !unit {
		k.fail("BR-23", n, "line %q has no invoiced quantity unit of measure code (BT-130)", id)
	}
	net := find(n, "SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount")
	if net == nil {
		k.fail("BR-24", n, "line %q has no line net amount (BT-131)", id)
	}
	if blank(find(n, "SpecifiedTradeProduct", "Name")) {
		k.fail("BR-25", n, "line %q has no item name (BT-153)", id)
	}
	net0, netOK := prices(n, "NetPriceProductTradePrice")
	gross0, grossOK := prices(n, "GrossPriceProductTradePrice")
	if net0 == nil {
		k.fail("BR-26", n, "line %q has no item net price (BT-146)", id)
		k.fail("BR-27", n, "line %q has no item net price (BT-146)", id)
	} else if !netOK {
		k.fail("BR-27", net0, "line %q: item net price (BT-146) %q is negative", id, quote(net0.Text))
	}
	if gross0 != nil && !grossOK {
		k.fail("BR-28", gross0, "line %q: item gross price (BT-148) %q is negative", id, quote(gross0.Text))
	}
	if gids := find(n, "SpecifiedTradeProduct"); gids != nil && gids.Elem("GlobalID") != nil {
		ok := false
		for it, g := elems(gids, "GlobalID"); g != nil; g = it.next() {
			if v, has := g.LookupAttr("schemeID"); has {
				ok = trim(v) != ""
				break
			}
		}
		if !ok {
			k.fail("BR-64", gids.Elem("GlobalID"), "line %q: item standard identifier (BT-157) has no scheme identifier", id)
		}
	}
	cat := false
	for it, s := elems(n, "SpecifiedLineTradeSettlement"); s != nil; s = it.next() {
		for it, t := elems(s, "ApplicableTradeTax"); t != nil; t = it.next() {
			if isVAT(t) && t.Elem("CategoryCode") != nil {
				cat = true
			}
		}
	}
	if !cat {
		k.fail("BR-CO-04", n, "line %q has no invoiced item VAT category code (BT-151)", id)
	}
	if !k.headerTax {
		k.fail("BR-CO-18", n, "the invoice has no VAT breakdown (BG-23)")
	}
	if tooManyDecimals(net) {
		k.fail("BR-DEC-23", net, "line %q: line net amount (BT-131) %q has more than 2 decimals", id, quote(net.Text))
	}
}

// prices returns the first ram:SpecifiedLineTradeAgreement/price/ram:ChargeAmount
// of a line, and whether one of them is >= 0.
func prices(line *xmltree.Node, price string) (first *xmltree.Node, nonNegative bool) {
	for it, a := elems(line, "SpecifiedLineTradeAgreement"); a != nil; a = it.next() {
		for it, p := elems(a, price); p != nil; p = it.next() {
			for it, c := elems(p, "ChargeAmount"); c != nil; c = it.next() {
				if first == nil {
					first = c
				}
				if v, ok := dec(c); ok && v.Sign() >= 0 {
					nonNegative = true
				}
			}
		}
	}
	return first, nonNegative
}

// textIn reports whether s equals the text of a child of n named name.
func textIn(s string, n *xmltree.Node, name string) bool {
	for it, c := elems(n, name); c != nil; c = it.next() {
		if c.Text == s {
			return true
		}
	}
	return false
}

// document checks the rules of context /rsm:CrossIndustryInvoice, BR-55 and
// BR-57.
func (k *checker) document() {
	d := k.d
	root := d.Root
	header := root.Elem("ExchangedDocument")
	agreement := d.Agreement
	seller := agreement.Elem("SellerTradeParty")
	buyer := agreement.Elem("BuyerTradeParty")
	loc := func(nodes ...*xmltree.Node) *xmltree.Node {
		for _, n := range nodes {
			if n != nil {
				return n
			}
		}
		return root
	}
	if blank(find(root, "ExchangedDocumentContext", "GuidelineSpecifiedDocumentContextParameter", "ID")) {
		k.fail("BR-01", loc(d.Context), "the invoice has no specification identifier (BT-24)")
	}
	if blank(header.Elem("ID")) {
		k.fail("BR-02", loc(header), "the invoice has no invoice number (BT-1)")
	}
	var issue *xmltree.Node
	for it, dt := elems(header, "IssueDateTime"); dt != nil; dt = it.next() {
		for it, s := elems(dt, "DateTimeString"); s != nil; s = it.next() {
			if f, _ := s.LookupAttr("format"); f == "102" && issue == nil {
				issue = s
			}
		}
	}
	if blank(issue) {
		k.fail("BR-03", loc(header), "the invoice has no issue date (BT-2) in format 102")
	}
	if blank(header.Elem("TypeCode")) {
		k.fail("BR-04", loc(header), "the invoice has no invoice type code (BT-3)")
	}
	if blank(d.Settlement.Elem("InvoiceCurrencyCode")) {
		k.fail("BR-05", loc(d.Settlement, d.Transaction), "the invoice has no invoice currency code (BT-5)")
	}
	if blank(seller.Elem("Name")) {
		k.fail("BR-06", loc(seller, agreement), "the invoice has no seller name (BT-27)")
	}
	if blank(buyer.Elem("Name")) {
		k.fail("BR-07", loc(buyer, agreement), "the invoice has no buyer name (BT-44)")
	}
	sellerAddr := seller.Elem("PostalTradeAddress")
	if sellerAddr == nil {
		k.fail("BR-08", loc(seller, agreement), "the invoice has no seller postal address (BG-5)")
	}
	if blank(sellerAddr.Elem("CountryID")) {
		k.fail("BR-09", loc(sellerAddr, seller, agreement), "the seller postal address (BG-5) has no country code (BT-40)")
	}
	buyerAddr := buyer.Elem("PostalTradeAddress")
	if buyerAddr == nil {
		k.fail("BR-10", loc(buyer, agreement), "the invoice has no buyer postal address (BG-8)")
	}
	if blank(buyerAddr.Elem("CountryID")) {
		k.fail("BR-11", loc(buyerAddr, buyer, agreement), "the buyer postal address (BG-8) has no country code (BT-55)")
	}
	if len(d.Lines) == 0 {
		k.fail("BR-16", loc(d.Transaction), "the invoice has no invoice line (BG-25)")
	}
	if u, ok := k.uriScheme("SellerTradeParty"); !ok {
		k.fail("BR-62", u, "the seller electronic address (BT-34) has no scheme identifier")
	}
	if u, ok := k.uriScheme("BuyerTradeParty"); !ok {
		k.fail("BR-63", u, "the buyer electronic address (BT-49) has no scheme identifier")
	}
	k.totalWithVAT()
	if k.mode == modeExtendedCTCFR {
		k.ctcfrDocument()
	} else {
		k.categoriesPresent()
	}
	if k.anyCode[catB] {
		if k.countryNotIT {
			k.fail("BR-B-01", root, "an invoice with VAT category B (split payment) must be a domestic Italian invoice: all country codes must be IT")
		}
		if k.anyCode[catS] {
			k.fail("BR-B-02", root, "an invoice with VAT category B (split payment) must not contain VAT category S (standard rated)")
		}
	}
	for it, r := elems(k.d.Settlement, "InvoiceReferencedDocument"); r != nil; r = it.next() {
		if blank(r.Elem("IssuerAssignedID")) {
			k.fail("BR-55", r, "a preceding invoice reference (BG-3) has no invoice reference (BT-25)")
		}
	}
	// The CII schema allows a single deliver to party with a single address:
	// only the first ones are checked.
	if a := d.Delivery.Elem("ShipToTradeParty").Elem("PostalTradeAddress"); a != nil && blank(a.Elem("CountryID")) {
		k.fail("BR-57", a, "the deliver to address (BG-15) has no country code (BT-80)")
	}
}

// uriScheme checks BR-62 or BR-63 for the given party of the header
// agreement: normalize-space(party/ram:URIUniversalCommunication[1]/ram:URIID/@schemeID)
// is not empty, or not(party/ram:URIUniversalCommunication). It returns the first
// electronic address.
func (k *checker) uriScheme(party string) (*xmltree.Node, bool) {
	var first *xmltree.Node
	schemes, nonEmpty := 0, false
	for _, a := range children(k.d.Transaction) {
		if a.Name != "ApplicableHeaderTradeAgreement" {
			continue
		}
		for _, p := range a.Children {
			if p.Name != party {
				continue
			}
			u := p.Elem("URIUniversalCommunication")
			if u == nil {
				continue
			}
			if first == nil {
				first = u
			}
			for _, id := range u.Children {
				if id.Name != "URIID" {
					continue
				}
				if v, ok := id.LookupAttr("schemeID"); ok {
					schemes++
					if trim(v) != "" {
						nonEmpty = true
					}
				}
			}
		}
	}
	// Several scheme identifiers make the official XPath fail with a type
	// error; one non-empty identifier is accepted here.
	return first, first == nil || (schemes > 0 && nonEmpty)
}

// totalWithVAT checks BR-CO-15: for every invoice currency, the invoice
// total with VAT is the total without VAT plus the single VAT total in that
// currency, or the total without VAT. As in the official rule, the amounts
// are the first ones of all the header settlements.
func (k *checker) totalWithVAT() {
	var grandN, basisN, first *xmltree.Node
	// The buffer keeps the common case off the heap with Go 1.24, whose
	// compiler does not place small appended slices on the stack.
	var buf [4]*xmltree.Node
	taxes := buf[:0]
	for _, s := range children(k.d.Transaction) {
		if s.Name != "ApplicableHeaderTradeSettlement" {
			continue
		}
		for _, m := range s.Children {
			if m.Name != "SpecifiedTradeSettlementHeaderMonetarySummation" {
				continue
			}
			if first == nil {
				first = m
			}
			for _, a := range m.Children {
				switch a.Name {
				case "GrandTotalAmount":
					if grandN == nil {
						grandN = a
					}
				case "TaxBasisTotalAmount":
					if basisN == nil {
						basisN = a
					}
				case "TaxTotalAmount":
					taxes = append(taxes, a)
				}
			}
		}
	}
	grand, gok := dec(grandN)
	basis, bok := dec(basisN)
	for it, cur := elems(k.d.Settlement, "InvoiceCurrencyCode"); cur != nil; cur = it.next() {
		var tax *xmltree.Node
		n := 0
		for _, t := range taxes {
			if v, ok := t.LookupAttr("currencyID"); ok && v == cur.Text {
				if n == 0 {
					tax = t
				}
				n++
			}
		}
		if gok && bok {
			if grand.Equal(basis) {
				continue
			}
			if v, ok := dec(tax); ok && n == 1 && grand.Equal(round2(basis.Add(v))) {
				continue
			}
		}
		k.fail("BR-CO-15", at(grandN, at(first, cur)), "invoice total amount with VAT (BT-112) %q is not invoice total amount without VAT (BT-109) %q + invoice total VAT amount (BT-110) %q (%d VAT total(s) in %q)",
			quote(text(grandN)), quote(text(basisN)), quote(text(tax)), n, quote(cur.Text))
	}
}

// at returns n, or fallback when n is nil.
func at(n, fallback *xmltree.Node) *xmltree.Node {
	if n != nil {
		return n
	}
	return fallback
}

// summation checks the rules of context
// //ram:SpecifiedTradeSettlementHeaderMonetarySummation.
func (k *checker) summation(m *xmltree.Node) {
	lineTotal := m.Elem("LineTotalAmount")
	basisTotal := m.Elem("TaxBasisTotalAmount")
	grandTotal := m.Elem("GrandTotalAmount")
	due := m.Elem("DuePayableAmount")
	if lineTotal == nil {
		k.fail("BR-12", m, "the invoice has no sum of invoice line net amounts (BT-106)")
	}
	if basisTotal == nil {
		k.fail("BR-13", m, "the invoice has no invoice total amount without VAT (BT-109)")
	}
	if grandTotal == nil {
		k.fail("BR-14", m, "the invoice has no invoice total amount with VAT (BT-112)")
	}
	if due == nil {
		k.fail("BR-15", m, "the invoice has no amount due for payment (BT-115)")
	}

	st := k.d.Settlement
	taxCur := st.Elem("TaxCurrencyCode")
	taxTotal := m.Elem("TaxTotalAmount")

	// BR-53
	if taxCur != nil {
		ok := false
		for it, t := elems(m, "TaxTotalAmount"); t != nil; t = it.next() {
			if v, has := t.LookupAttr("currencyID"); has && textIn(v, st, "TaxCurrencyCode") {
				ok = true
			}
		}
		for it, tc := elems(st, "TaxCurrencyCode"); tc != nil; tc = it.next() {
			if textIn(tc.Text, st, "InvoiceCurrencyCode") {
				ok = false
			}
		}
		if !ok {
			k.fail("BR-53", m, "VAT accounting currency code (BT-6) %q is given but there is no invoice total VAT amount in accounting currency (BT-111) in a currency other than the invoice currency", quote(taxCur.Text))
		}
	}

	// BR-CO-10
	sum := zero
	if tr := m.Parent.Parent; tr != nil {
		for it, l := elems(tr, "IncludedSupplyChainTradeLineItem"); l != nil; l = it.next() {
			for it, s := elems(l, "SpecifiedLineTradeSettlement"); s != nil; s = it.next() {
				for it, ms := elems(s, "SpecifiedTradeSettlementLineMonetarySummation"); ms != nil; ms = it.next() {
					for it, a := elems(ms, "LineTotalAmount"); a != nil; a = it.next() {
						if v, ok := dec(a); ok {
							sum = sum.Add(v)
						}
					}
				}
			}
		}
	}
	if v, ok := dec(lineTotal); !ok || !v.Equal(round2(sum)) {
		k.fail("BR-CO-10", at(lineTotal, m), "sum of invoice line net amounts (BT-106) %q is not the sum of the line net amounts (BT-131) %s", quote(text(lineTotal)), round2(sum).String())
	}

	// BR-CO-11 and BR-CO-12
	var allowances, charges = zero, zero
	var hasAllowance, hasCharge bool
	for it, ac := elems(k.d.Settlement, "SpecifiedTradeAllowanceCharge"); ac != nil; ac = it.next() {
		a, c := chargeIndicator(ac)
		v, ok := dec(ac.Elem("ActualAmount"))
		if a {
			hasAllowance = true
			if ok {
				allowances = allowances.Add(v)
			}
		}
		if c {
			hasCharge = true
			if ok {
				charges = charges.Add(v)
			}
		}
	}
	k.totalOf("BR-CO-11", m, "AllowanceTotalAmount", hasAllowance, round2(allowances), "sum of allowances on document level (BT-107)", "document level allowance amounts (BT-92)")
	k.totalOf("BR-CO-12", m, "ChargeTotalAmount", hasCharge, round2(charges), "sum of charges on document level (BT-108)", "document level charge amounts (BT-99)")

	// BR-CO-13: TaxBasisTotalAmount = round(LineTotal - AllowanceTotal + ChargeTotal).
	allowTotal, chargeTotal := m.Elem("AllowanceTotalAmount"), m.Elem("ChargeTotalAmount")
	ok13 := false
	if t, ok := dec(basisTotal); ok {
		if l, ok := dec(lineTotal); ok {
			exp, ok := l, true
			if allowTotal != nil {
				a, aok := dec(allowTotal)
				exp, ok = exp.Sub(a), ok && aok
			}
			if chargeTotal != nil {
				c, cok := dec(chargeTotal)
				exp, ok = exp.Add(c), ok && cok
			}
			ok13 = ok && t.Equal(round2(exp))
		}
	}
	if !ok13 {
		k.fail("BR-CO-13", at(basisTotal, m), "invoice total amount without VAT (BT-109) %q is not the sum of line net amounts (BT-106) %q - allowances (BT-107) %q + charges (BT-108) %q",
			quote(text(basisTotal)), quote(text(lineTotal)), quote(text(allowTotal)), quote(text(chargeTotal)))
	}

	// BR-CO-16: DuePayable = GrandTotal - TotalPrepaid + Rounding (exact).
	prepaid, rounding := m.Elem("TotalPrepaidAmount"), m.Elem("RoundingAmount")
	ok16 := false
	if dv, ok := dec(due); ok {
		if g, ok := dec(grandTotal); ok {
			exp, ok := g, true
			if prepaid != nil {
				p, pok := dec(prepaid)
				exp, ok = exp.Sub(p), ok && pok
			}
			if rounding != nil {
				r, rok := dec(rounding)
				exp, ok = exp.Add(r), ok && rok
			}
			ok16 = ok && dv.Equal(exp)
		}
	}
	if !ok16 {
		k.fail("BR-CO-16", at(due, m), "amount due for payment (BT-115) %q is not invoice total with VAT (BT-112) %q - paid amount (BT-113) %q + rounding amount (BT-114) %q",
			quote(text(due)), quote(text(grandTotal)), quote(text(prepaid)), quote(text(rounding)))
	}

	// BR-CO-14 (context TaxTotalAmount[@currencyID = InvoiceCurrencyCode]).
	if k.mode == modeExtendedCTCFR {
		k.ctcfrVATTotal(m)
		k.ctcfrTotals(m)
	} else {
		k.vatTotal(m)
	}

	// BR-DEC
	decimals := []struct {
		rule, what string
		n          *xmltree.Node
	}{
		{"BR-DEC-09", "sum of invoice line net amounts (BT-106)", lineTotal},
		{"BR-DEC-10", "sum of allowances on document level (BT-107)", allowTotal},
		{"BR-DEC-11", "sum of charges on document level (BT-108)", chargeTotal},
		{"BR-DEC-12", "invoice total amount without VAT (BT-109)", basisTotal},
		{"BR-DEC-14", "invoice total amount with VAT (BT-112)", grandTotal},
		{"BR-DEC-16", "paid amount (BT-113)", prepaid},
		{"BR-DEC-17", "rounding amount (BT-114)", rounding},
		{"BR-DEC-18", "amount due for payment (BT-115)", due},
	}
	for _, x := range decimals {
		if tooManyDecimals(x.n) {
			k.fail(x.rule, x.n, "%s %q has more than 2 decimals", x.what, quote(x.n.Text))
		}
	}
	// BR-DEC-13: one TaxTotalAmount in the invoice currency with at most two
	// significant decimals, or one in another currency.
	if taxTotal != nil {
		twoDecimals := twoDecimals
		if k.mode == modeExtendedCTCFR {
			twoDecimals = twoDecimalsDouble
		}
		ok13, ok15 := false, false
		for it, t := elems(m, "TaxTotalAmount"); t != nil; t = it.next() {
			cur, has := t.LookupAttr("currencyID")
			inInv := has && textIn(cur, st, "InvoiceCurrencyCode")
			if !inInv || twoDecimals(t) {
				ok13 = true
			}
			if taxCur == nil || (has && textIn(cur, st, "TaxCurrencyCode") && twoDecimals(t)) {
				ok15 = true
			}
		}
		if !ok13 {
			k.fail("BR-DEC-13", taxTotal, "invoice total VAT amount (BT-110) %q has more than 2 decimals", quote(taxTotal.Text))
		}
		if !ok15 {
			k.fail("BR-DEC-15", taxTotal, "invoice total VAT amount in accounting currency (BT-111) has more than 2 decimals or is missing")
		}
	}
}

// vatTotal checks BR-CO-14 on the summation m: the invoice total VAT amount
// (BT-110) in the invoice currency equals the sum of the VAT category tax
// amounts (BT-117), rounded to 2 decimals.
func (k *checker) vatTotal(m *xmltree.Node) {
	st := k.d.Settlement
	vat := zero
	for it, t := elems(st, "ApplicableTradeTax"); t != nil; t = it.next() {
		for it, c := elems(t, "CalculatedAmount"); c != nil; c = it.next() {
			if v, ok := dec(c); ok {
				vat = vat.Add(v)
			}
		}
	}
	for it, t := elems(m, "TaxTotalAmount"); t != nil; t = it.next() {
		if v, ok := t.LookupAttr("currencyID"); !ok || !textIn(v, st, "InvoiceCurrencyCode") {
			continue
		}
		if v, ok := dec(t); !ok || !v.Equal(round2(vat)) {
			k.fail("BR-CO-14", t, "invoice total VAT amount (BT-110) %q is not the sum of the VAT category tax amounts (BT-117) %s", quote(t.Text), round2(vat).String())
		}
	}
}

// twoDecimals reports whether xs:decimal(t) = round(xs:decimal(t) * 100) div 100.
func twoDecimals(t *xmltree.Node) bool {
	v, ok := dec(t)
	return ok && v.Equal(round2(v))
}

// totalOf checks BR-CO-11 or BR-CO-12: the total element equals the rounded
// sum of the document level allowances or charges.
func (k *checker) totalOf(rule string, m *xmltree.Node, name string, has bool, sum invoice.Decimal, what, of string) {
	if !has && m.Elem(name) == nil {
		return
	}
	for it, t := elems(m, name); t != nil; t = it.next() {
		if v, ok := dec(t); ok && v.Equal(sum) {
			return
		}
	}
	k.fail(rule, at(m.Elem(name), m), "%s %q is not the sum of the %s %s", what, quote(text(m.Elem(name))), of, sum.String())
}
