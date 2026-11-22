package ubl

import (
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The business rules below have, in the UBL artefacts, a context or a test
// that differs from the CII artefacts in a way the CII view cannot render:
// the country code rules apply only to an existing address, the scheme
// identifier rules check that the attribute exists (not that it is filled
// in), the payee is compared with the seller trading name, the total VAT
// amount is compared with the breakdown of its own tax total, and the decimal
// rules count the characters after the first "." instead of comparing the
// value with its rounding. Package en16931 leaves them to this package for a
// UBL document (see en16931.ublNative).

// first returns the first node at path relative to n, or nil.
func first(n *xmltree.Node, path string) *xmltree.Node {
	if s := selectPath([]*xmltree.Node{n}, path); len(s) > 0 {
		return s[0]
	}
	return nil
}

// text returns the text of n, "" for nil.
func text(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	return n.Text
}

// isLine reports whether n is an invoice or credit note line.
func isLine(n *xmltree.Node) bool {
	return is(n, "cac", "InvoiceLine") || is(n, "cac", "CreditNoteLine")
}

// business checks the business rules of the document element (BR-CO-18).
func (k *checker) business() {
	if first(k.root, "cac:TaxTotal/cac:TaxSubtotal") == nil {
		k.fail("BR-CO-18", nil, "the invoice has no VAT breakdown (cac:TaxTotal/cac:TaxSubtotal, BG-23)")
	}
}

// businessRules checks the business rules whose context is the element n.
func (k *checker) businessRules(n *xmltree.Node) {
	switch n.Space {
	case NSCac:
		k.businessAggregate(n)
	case NSCbc:
		k.businessBasic(n)
	}
}

// businessAggregate checks the business rules whose context is the UBL
// aggregate n.
func (k *checker) businessAggregate(n *xmltree.Node) {
	p := n.Parent
	switch n.Name {
	case "PostalAddress":
		var id, what string
		switch {
		case is(p, "cac", "Party") && parentIs(p, "cac", "AccountingSupplierParty"):
			id, what = "BR-09", "the seller postal address (BG-5) has no country code (BT-40)"
		case is(p, "cac", "Party") && parentIs(p, "cac", "AccountingCustomerParty"):
			id, what = "BR-11", "the buyer postal address (BG-8) has no country code (BT-55)"
		case is(p, "cac", "TaxRepresentativeParty"):
			id, what = "BR-20", "the seller tax representative postal address (BG-12) has no country code (BT-69)"
		default:
			return
		}
		if normalize(text(first(n, "cac:Country/cbc:IdentificationCode"))) == "" {
			k.fail(id, n, "%s", what)
		}
	case "Address":
		if is(p, "cac", "DeliveryLocation") && parentIs(p, "cac", "Delivery") && first(n, "cac:Country/cbc:IdentificationCode") == nil {
			k.fail("BR-57", n, "the deliver to address (BG-15) has no country code (BT-80)")
		}
	case "PayeeParty":
		k.payee(n)
	case "TaxRepresentativeParty":
		vat := false
		for _, s := range elems(n, "cac", "PartyTaxScheme") {
			for _, id := range selectPath([]*xmltree.Node{s}, "cac:TaxScheme/cbc:ID") {
				vat = vat || strings.ToUpper(normalize(id.Text)) == "VAT" && len(elems(s, "cbc", "CompanyID")) > 0
			}
		}
		if !vat {
			k.fail("BR-56", n, "the seller tax representative (BG-11) has no VAT identifier (BT-63)")
		}
	case "AdditionalDocumentReference":
		if normalize(text(first(n, "cbc:ID"))) == "" {
			k.fail("BR-52", n, "an additional supporting document (BG-24) has no supporting document reference (BT-122)")
		}
	case "BillingReference":
		if first(n, "cac:InvoiceDocumentReference/cbc:ID") == nil {
			k.fail("BR-55", n, "a preceding invoice reference (BG-3) has no invoice reference (BT-25)")
		}
	case "PaymentMeans":
		code := normalize(text(first(n, "cbc:PaymentMeansCode")))
		if (code == "30" || code == "58") && first(n, "cac:PayeeFinancialAccount/cbc:ID") == nil {
			k.fail("BR-61", n, "credit transfer (payment means %s) without payment account identifier (BT-84)", code)
		}
	case "AllowanceCharge":
		var ids [2]string
		var what string
		switch {
		case p == k.root && chargeIs(n, false):
			ids, what = [2]string{"BR-DEC-01", "BR-DEC-02"}, "document level allowance"
		case p == k.root && chargeIs(n, true):
			ids, what = [2]string{"BR-DEC-05", "BR-DEC-06"}, "document level charge"
		case isLine(p) && chargeIs(n, false):
			ids, what = [2]string{"BR-DEC-24", "BR-DEC-25"}, "invoice line allowance"
		case isLine(p) && chargeIs(n, true):
			ids, what = [2]string{"BR-DEC-27", "BR-DEC-28"}, "invoice line charge"
		default:
			return
		}
		k.decimals(ids[0], n, "cbc:Amount", what+" amount")
		k.decimals(ids[1], n, "cbc:BaseAmount", what+" base amount")
	case "LegalMonetaryTotal":
		for _, r := range []struct{ id, path, what string }{
			{"BR-DEC-09", "cbc:LineExtensionAmount", "sum of invoice line net amounts (BT-106)"},
			{"BR-DEC-10", "cbc:AllowanceTotalAmount", "sum of allowances on document level (BT-107)"},
			{"BR-DEC-11", "cbc:ChargeTotalAmount", "sum of charges on document level (BT-108)"},
			{"BR-DEC-12", "cbc:TaxExclusiveAmount", "invoice total amount without VAT (BT-109)"},
			{"BR-DEC-14", "cbc:TaxInclusiveAmount", "invoice total amount with VAT (BT-112)"},
			{"BR-DEC-16", "cbc:PrepaidAmount", "paid amount (BT-113)"},
			{"BR-DEC-17", "cbc:PayableRoundingAmount", "rounding amount (BT-114)"},
			{"BR-DEC-18", "cbc:PayableAmount", "amount due for payment (BT-115)"},
		} {
			k.decimals(r.id, n, r.path, r.what)
		}
	case "InvoiceLine", "CreditNoteLine":
		k.decimals("BR-DEC-23", n, "cbc:LineExtensionAmount", "invoice line net amount (BT-131)")
	case "TaxTotal":
		if p == k.root {
			k.taxTotal(n)
		}
	case "TaxSubtotal":
		if is(p, "cac", "TaxTotal") {
			k.decimals("BR-DEC-19", n, "cbc:TaxableAmount", "VAT category taxable amount (BT-116)")
			k.decimals("BR-DEC-20", n, "cbc:TaxAmount", "VAT category tax amount (BT-117)")
		}
	}
}

// taxTotal checks BR-CO-14 on a tax total of the document: with a VAT
// breakdown, its tax amount is the sum of the tax amounts of its own
// subtotals, rounded to two decimals, halves up. The UBL artefacts check each
// cac:TaxTotal on its own, whatever its currency, where the CII view adds up
// the subtotals of all of them. A value that is not a decimal fails it (the
// reference stops on the cast).
func (k *checker) taxTotal(n *xmltree.Node) {
	subtotals := elems(n, "cac", "TaxSubtotal")
	if len(subtotals) == 0 {
		return
	}
	sum, ok := invoice.NewDecimal(0, 0), true
	for _, s := range subtotals {
		if a := first(s, "cbc:TaxAmount"); a != nil {
			v, valid := rules.Dec(a)
			sum, ok = sum.Add(v), ok && valid
		}
	}
	sum = sum.RoundHalfCeil(2)
	amount := first(n, "cbc:TaxAmount")
	if v, valid := rules.Dec(amount); ok && valid && v.Equal(sum) {
		return
	}
	at := n
	if amount != nil {
		at = amount
	}
	k.fail("BR-CO-14", at, "invoice total VAT amount (BT-110) %q is not the sum of the VAT category tax amounts (BT-117) %s of its tax total", text(amount), sum.String())
}

// businessBasic checks the business rules whose context is the UBL basic
// element n: the scheme identifier rules BR-62 to BR-65.
func (k *checker) businessBasic(n *xmltree.Node) {
	p := n.Parent
	switch n.Name {
	case "EndpointID":
		if !is(p, "cac", "Party") || hasAttr(n, "schemeID") {
			return
		}
		switch {
		case parentIs(p, "cac", "AccountingSupplierParty"):
			k.fail("BR-62", n, "the seller electronic address (BT-34) has no scheme identifier")
		case parentIs(p, "cac", "AccountingCustomerParty"):
			k.fail("BR-63", n, "the buyer electronic address (BT-49) has no scheme identifier")
		}
	case "ID":
		if is(p, "cac", "StandardItemIdentification") && parentIs(p, "cac", "Item") && isLine(p.Parent.Parent) && !hasAttr(n, "schemeID") {
			k.fail("BR-64", n, "the item standard identifier (BT-157) has no scheme identifier")
		}
	case "ItemClassificationCode":
		if is(p, "cac", "CommodityClassification") && parentIs(p, "cac", "Item") && isLine(p.Parent.Parent) && !hasAttr(n, "listID") {
			k.fail("BR-65", n, "the item classification identifier (BT-158) has no scheme identifier")
		}
	}
}

// payee checks BR-17 on the payee n: it must be named, and differ from the
// seller, whose trading name (cac:PartyName/cbc:Name) and identifiers the
// UBL artefacts compare it with.
func (k *checker) payee(n *xmltree.Node) {
	names := selectPath([]*xmltree.Node{n}, "cac:PartyName/cbc:Name")
	if len(names) == 0 {
		k.fail("BR-17", n, "the payee (BG-10) has no name (BT-59)")
		return
	}
	seller := []*xmltree.Node{n.Parent}
	if equalAny(names, selectPath(seller, "cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name")) {
		k.fail("BR-17", n, "the payee (BG-10) has the trading name of the seller (%q): a payee different from the seller must be named", names[0].Text)
		return
	}
	if equalAny(selectPath([]*xmltree.Node{n}, "cac:PartyIdentification/cbc:ID"), selectPath(seller, "cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID")) {
		k.fail("BR-17", n, "the payee (BG-10) has the identifier of the seller")
	}
}

// equalAny reports whether a value of a equals a value of b (XPath general
// comparison a = b).
func equalAny(a, b []*xmltree.Node) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Text == y.Text {
				return true
			}
		}
	}
	return false
}

// decimals checks a BR-DEC rule: the amount at path relative to n has at most
// two characters after the first "." (string-length(substring-after(path,
// '.')) <= 2).
func (k *checker) decimals(id string, n *xmltree.Node, path, what string) {
	a := first(n, path)
	if a == nil {
		return
	}
	if _, frac, ok := strings.Cut(a.Text, "."); ok && len([]rune(frac)) > 2 {
		k.fail(id, a, "%s %q has more than 2 decimals", what, a.Text)
	}
}
