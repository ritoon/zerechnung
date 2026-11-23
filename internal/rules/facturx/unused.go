package facturx

import (
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// unusedItem is an element or attribute that the official schematron of a
// profile reports as "not used": the schema accepts it, receivers ignore it.
type unusedItem struct {
	path []string // local names from the root element
	attr string   // attribute name, "" for the element itself
	mask uint8
}

// unusedItems lists the "not used" reports of Factur-X_1.09_MINIMUM.sch and
// Factur-X_1.09_BASICWL.sch, except the variants selected by a value, which
// unused handles: seller tax registrations with a scheme other than VA and
// FC, total VAT amounts in another currency, allowances whose charge
// indicator is neither "true" nor "false" and, in BASIC WL, charge reason
// codes whose list identifier is not 5153. Below a
// ram:SpecifiedTradeAllowanceCharge, the items only apply when the charge
// indicator is "true" or "false".
var unusedItems = sync.OnceValue(func() []unusedItem {
	const (
		tx  = "SupplyChainTradeTransaction/"
		agr = tx + "ApplicableHeaderTradeAgreement/"
		del = tx + "ApplicableHeaderTradeDelivery/"
		set = tx + "ApplicableHeaderTradeSettlement/"
		sum = set + "SpecifiedTradeSettlementHeaderMonetarySummation/"
		ac  = set + "SpecifiedTradeAllowanceCharge/"
	)
	spec := []struct {
		path, attr string
		mask       uint8
	}{
		{"ExchangedDocument/ID", "schemeID", mLight},
		{"ExchangedDocumentContext/BusinessProcessSpecifiedDocumentContextParameter/ID", "schemeID", mLight},
		{"ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/ID", "schemeID", mLight},
		{agr + "BuyerOrderReferencedDocument/IssuerAssignedID", "schemeID", mLight},
		{agr + "BuyerTradeParty/PostalTradeAddress", "", mMin},
		{agr + "BuyerTradeParty/SpecifiedTaxRegistration", "", mMin},
		{sum + "TaxBasisTotalAmount", "currencyID", mLight},
		{sum + "GrandTotalAmount", "currencyID", mLight},
		{sum + "DuePayableAmount", "currencyID", mLight},

		{agr + "BuyerOrderReferencedDocument/FormattedIssueDateTime", "", mBWL},
		{agr + "BuyerTradeParty/ID", "schemeID", mBWL},
		{agr + "BuyerTradeParty/SpecifiedLegalOrganization/TradingBusinessName", "", mBWL},
		{agr + "ContractReferencedDocument/FormattedIssueDateTime", "", mBWL},
		{agr + "ContractReferencedDocument/IssuerAssignedID", "schemeID", mBWL},
		{agr + "SellerTaxRepresentativeTradeParty/GlobalID", "", mBWL},
		{agr + "SellerTaxRepresentativeTradeParty/ID", "", mBWL},
		{agr + "SellerTaxRepresentativeTradeParty/SpecifiedLegalOrganization", "", mBWL},
		{agr + "SellerTaxRepresentativeTradeParty/URIUniversalCommunication", "", mBWL},
		{agr + "SellerTradeParty/ID", "schemeID", mBWL},
		{del + "DespatchAdviceReferencedDocument/FormattedIssueDateTime", "", mBWL},
		{del + "DespatchAdviceReferencedDocument/IssuerAssignedID", "schemeID", mBWL},
		{del + "ShipToTradeParty/ID", "schemeID", mBWL},
		{del + "ShipToTradeParty/SpecifiedLegalOrganization", "", mBWL},
		{del + "ShipToTradeParty/SpecifiedTaxRegistration", "", mBWL},
		{del + "ShipToTradeParty/URIUniversalCommunication", "", mBWL},
		{set + "ApplicableTradeTax/BasisAmount", "currencyID", mBWL},
		{set + "ApplicableTradeTax/CalculatedAmount", "currencyID", mBWL},
		{set + "CreditorReferenceID", "schemeID", mBWL},
		{set + "InvoiceReferencedDocument/IssuerAssignedID", "schemeID", mBWL},
		{set + "PayeeTradeParty/ID", "schemeID", mBWL},
		{set + "PayeeTradeParty/PostalTradeAddress", "", mBWL},
		{set + "PayeeTradeParty/SpecifiedLegalOrganization/TradingBusinessName", "", mBWL},
		{set + "PayeeTradeParty/SpecifiedTaxRegistration", "", mBWL},
		{set + "PayeeTradeParty/URIUniversalCommunication", "", mBWL},
		{set + "ReceivableSpecifiedTradeAccountingAccount/ID", "schemeID", mBWL},
		{ac + "ActualAmount", "currencyID", mBWL},
		{ac + "BasisAmount", "currencyID", mBWL},
		{ac + "CategoryTradeTax/BasisAmount", "", mBWL},
		{ac + "CategoryTradeTax/CalculatedAmount", "", mBWL},
		{ac + "CategoryTradeTax/DueDateTypeCode", "", mBWL},
		{ac + "CategoryTradeTax/ExemptionReason", "", mBWL},
		{ac + "CategoryTradeTax/ExemptionReasonCode", "", mBWL},
		{set + "SpecifiedTradePaymentTerms/DirectDebitMandateID", "schemeID", mBWL},
		{sum + "LineTotalAmount", "currencyID", mBWL},
		{sum + "ChargeTotalAmount", "currencyID", mBWL},
		{sum + "AllowanceTotalAmount", "currencyID", mBWL},
		{sum + "TotalPrepaidAmount", "currencyID", mBWL},
		{set + "SpecifiedTradeSettlementPaymentMeans/PayeePartyCreditorFinancialAccount/IBANID", "schemeID", mBWL},
		{set + "SpecifiedTradeSettlementPaymentMeans/PayeePartyCreditorFinancialAccount/ProprietaryID", "schemeID", mBWL},
		{set + "SpecifiedTradeSettlementPaymentMeans/PayerPartyDebtorFinancialAccount/IBANID", "schemeID", mBWL},
	}
	items := make([]unusedItem, len(spec))
	for i, s := range spec {
		items[i] = unusedItem{path: strings.Split(s.path, "/"), attr: s.attr, mask: s.mask}
	}
	return items
})

// unused reports the elements and attributes the profile does not use.
func (k *checker) unused() {
	for i := range unusedItems() {
		if u := &unusedItems()[i]; u.mask&k.p != 0 {
			k.unusedWalk(k.d.Root, u, 0)
		}
	}
	// Variants selected by a value.
	if s := k.seller; s != nil {
		for _, r := range s.Children {
			if r.Name == "SpecifiedTaxRegistration" {
				if sc := schemeOf(r); sc != "VA" && sc != "FC" {
					k.fail("FX-NOT-USED", r, "a seller tax registration with scheme %q is not used in profile %s (only VA and FC are): receivers ignore it", sc, k.profileName())
				}
			}
		}
	}
	for _, t := range kids(k.d.Summation) {
		if t.Name != "TaxTotalAmount" {
			continue
		}
		cur, ok := t.LookupAttr("currencyID")
		if !ok || (k.icc == nil || cur != k.icc.Text) && (k.tcc == nil || cur != k.tcc.Text) {
			k.fail("FX-NOT-USED", t, "a total VAT amount in currency %q is not used in profile %s (only the invoice currency and the VAT accounting currency are): receivers ignore it", cur, k.profileName())
		}
	}
	for _, ac := range kids(k.d.Settlement) {
		if ac.Name != "SpecifiedTradeAllowanceCharge" {
			continue
		}
		ind := text(ac.Elem("ChargeIndicator").Elem("Indicator"))
		if ind != "true" && ind != "false" {
			k.fail("FX-NOT-USED", ac, "a document level allowance or charge whose charge indicator is %q is not used in profile %s (expected true or false): receivers ignore it", ind, k.profileName())
		}
		if ind != "true" || k.p != mBWL {
			continue
		}
		for _, rc := range ac.Children {
			if list, ok := rc.LookupAttr("listID"); rc.Name == "ReasonCode" && ok && list != "5153" {
				k.fail("FX-NOT-USED", rc, "a charge reason code (BT-105) with list identifier %q is not used in profile %s (only none or 5153): receivers ignore it", list, k.profileName())
			}
		}
	}
}

// unusedWalk reports the descendants of n matching u.path[i:].
func (k *checker) unusedWalk(n *xmltree.Node, u *unusedItem, i int) {
	for _, ch := range n.Children {
		if ch.Name != u.path[i] {
			continue
		}
		if i == len(u.path)-1 {
			if u.attr == "" {
				k.fail("FX-NOT-USED", ch, "%s is not used in profile %s: receivers ignore it", display(ch), k.profileName())
			} else if _, ok := ch.LookupAttr(u.attr); ok {
				k.fail("FX-NOT-USED", ch, "attribute %s of %s is not used in profile %s: receivers ignore it", u.attr, display(ch), k.profileName())
			}
			continue
		}
		if ch.Name == "SpecifiedTradeAllowanceCharge" {
			if ind := text(ch.Elem("ChargeIndicator").Elem("Indicator")); ind != "true" && ind != "false" {
				continue
			}
		}
		k.unusedWalk(ch, u, i+1)
	}
}
