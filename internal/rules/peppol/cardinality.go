package peppol

import (
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The pattern inferred-mandatory-cardinality of the UBL schematrons of
// release 3.1.0, in Peppol BIS Billing (PEPPOL-EN16931-UBL.sch) as in
// Peppol BIS Self-Billing (PEPPOL-EN16931-UBL-SB.sch): when an element of
// the semantic model is present, each of its mandatory children appears
// exactly once (count(child) = 1). Its 120 rules, PEPPOL-EN16931-R141 to
// R276, are all fatal; the schematron leaves out those that would only
// repeat a cardinality 1..1 of the UBL 2.1 schemas.
//
// The contexts are paths anchored at ubl-invoice:Invoice or
// ubl-creditnote:CreditNote, all distinct: an element matches at most one
// of them.

// cardinalityRule is a rule of the pattern: its identifier and the child
// that must appear exactly once.
type cardinalityRule struct{ id, child string }

// cardinalityContext is a context of the pattern: the namespace of the
// document element (nsInvoice or nsCreditNote), the path of UBL names below
// it ("" for the document element itself) and its rules.
type cardinalityContext struct {
	space string
	path  string
	rules []cardinalityRule
}

// cardinalityContexts are the contexts of the pattern, in the order of the
// schematron.
var cardinalityContexts = []cardinalityContext{
	{nsCreditNote, "", []cardinalityRule{
		{"PEPPOL-EN16931-R205", "cbc:CreditNoteTypeCode"},
		{"PEPPOL-EN16931-R206", "cbc:CustomizationID"},
		{"PEPPOL-EN16931-R207", "cbc:DocumentCurrencyCode"},
		{"PEPPOL-EN16931-R208", "cbc:ProfileID"},
	}},
	{nsCreditNote, "cac:AccountingCustomerParty", []cardinalityRule{{"PEPPOL-EN16931-R141", "cac:Party"}}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party", []cardinalityRule{
		{"PEPPOL-EN16931-R142", "cac:PartyLegalEntity"},
		{"PEPPOL-EN16931-R146", "cac:PostalAddress"},
		{"PEPPOL-EN16931-R150", "cbc:EndpointID"},
	}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R143", "cbc:RegistrationName"}}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R145", "cbc:CompanyID"}}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R144", "cbc:ID"}}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R148", "cac:Country"}}},
	{nsCreditNote, "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R149", "cbc:IdentificationCode"}}},
	{nsCreditNote, "cac:AccountingSupplierParty", []cardinalityRule{{"PEPPOL-EN16931-R151", "cac:Party"}}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party", []cardinalityRule{
		{"PEPPOL-EN16931-R152", "cac:PartyLegalEntity"},
		{"PEPPOL-EN16931-R156", "cac:PostalAddress"},
		{"PEPPOL-EN16931-R160", "cbc:EndpointID"},
	}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R153", "cbc:RegistrationName"}}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R155", "cbc:CompanyID"}}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R154", "cbc:ID"}}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R158", "cac:Country"}}},
	{nsCreditNote, "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R159", "cbc:IdentificationCode"}}},
	{nsCreditNote, "cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference", []cardinalityRule{{"PEPPOL-EN16931-R161", "cbc:URI"}}},
	{nsCreditNote, "cac:AllowanceCharge", []cardinalityRule{{"PEPPOL-EN16931-R162", "cac:TaxCategory"}}},
	{nsCreditNote, "cac:AllowanceCharge/cac:TaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R165", "cbc:ID"}}},
	{nsCreditNote, "cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R164", "cbc:ID"}}},
	{nsCreditNote, "cac:BillingReference", []cardinalityRule{{"PEPPOL-EN16931-R166", "cac:InvoiceDocumentReference"}}},
	{nsCreditNote, "cac:CreditNoteLine", []cardinalityRule{
		{"PEPPOL-EN16931-R168", "cac:Item"},
		{"PEPPOL-EN16931-R177", "cac:Price"},
		{"PEPPOL-EN16931-R178", "cbc:CreditedQuantity"},
		{"PEPPOL-EN16931-R179", "cbc:LineExtensionAmount"},
	}},
	{nsCreditNote, "cac:CreditNoteLine/cac:DocumentReference", []cardinalityRule{{"PEPPOL-EN16931-R167", "cbc:DocumentTypeCode"}}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item", []cardinalityRule{
		{"PEPPOL-EN16931-R170", "cac:ClassifiedTaxCategory"},
		{"PEPPOL-EN16931-R176", "cbc:Name"},
	}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty", []cardinalityRule{{"PEPPOL-EN16931-R169", "cbc:Value"}}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R173", "cbc:ID"}}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R172", "cbc:ID"}}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item/cac:CommodityClassification", []cardinalityRule{{"PEPPOL-EN16931-R174", "cbc:ItemClassificationCode"}}},
	{nsCreditNote, "cac:CreditNoteLine/cac:Item/cac:OriginCountry", []cardinalityRule{{"PEPPOL-EN16931-R175", "cbc:IdentificationCode"}}},
	{nsCreditNote, "cac:Delivery/cac:DeliveryLocation/cac:Address", []cardinalityRule{{"PEPPOL-EN16931-R181", "cac:Country"}}},
	{nsCreditNote, "cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R182", "cbc:IdentificationCode"}}},
	{nsCreditNote, "cac:Delivery/cac:DeliveryParty", []cardinalityRule{{"PEPPOL-EN16931-R183", "cac:PartyName"}}},
	{nsCreditNote, "cac:LegalMonetaryTotal", []cardinalityRule{
		{"PEPPOL-EN16931-R184", "cbc:LineExtensionAmount"},
		{"PEPPOL-EN16931-R185", "cbc:TaxExclusiveAmount"},
		{"PEPPOL-EN16931-R186", "cbc:TaxInclusiveAmount"},
	}},
	{nsCreditNote, "cac:PayeeParty", []cardinalityRule{{"PEPPOL-EN16931-R188", "cac:PartyName"}}},
	{nsCreditNote, "cac:PayeeParty/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R187", "cbc:CompanyID"}}},
	{nsCreditNote, "cac:PaymentMeans/cac:PayeeFinancialAccount", []cardinalityRule{{"PEPPOL-EN16931-R190", "cbc:ID"}}},
	{nsCreditNote, "cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch", []cardinalityRule{{"PEPPOL-EN16931-R189", "cbc:ID"}}},
	{nsCreditNote, "cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount", []cardinalityRule{{"PEPPOL-EN16931-R191", "cbc:ID"}}},
	{nsCreditNote, "cac:PaymentTerms", []cardinalityRule{{"PEPPOL-EN16931-R192", "cbc:Note"}}},
	{nsCreditNote, "cac:TaxRepresentativeParty", []cardinalityRule{
		{"PEPPOL-EN16931-R193", "cac:PartyName"},
		{"PEPPOL-EN16931-R194", "cac:PartyTaxScheme"},
		{"PEPPOL-EN16931-R197", "cac:PostalAddress"},
	}},
	{nsCreditNote, "cac:TaxRepresentativeParty/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R196", "cbc:CompanyID"}}},
	{nsCreditNote, "cac:TaxRepresentativeParty/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R195", "cbc:ID"}}},
	{nsCreditNote, "cac:TaxRepresentativeParty/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R199", "cac:Country"}}},
	{nsCreditNote, "cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R200", "cbc:IdentificationCode"}}},
	{nsCreditNote, "cac:TaxTotal/cac:TaxSubtotal", []cardinalityRule{{"PEPPOL-EN16931-R204", "cbc:TaxableAmount"}}},
	{nsCreditNote, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R203", "cbc:ID"}}},
	{nsCreditNote, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R202", "cbc:ID"}}},
	{nsInvoice, "", []cardinalityRule{
		{"PEPPOL-EN16931-R273", "cbc:CustomizationID"},
		{"PEPPOL-EN16931-R274", "cbc:DocumentCurrencyCode"},
		{"PEPPOL-EN16931-R275", "cbc:InvoiceTypeCode"},
		{"PEPPOL-EN16931-R276", "cbc:ProfileID"},
	}},
	{nsInvoice, "cac:AccountingCustomerParty", []cardinalityRule{{"PEPPOL-EN16931-R210", "cac:Party"}}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party", []cardinalityRule{
		{"PEPPOL-EN16931-R211", "cac:PartyLegalEntity"},
		{"PEPPOL-EN16931-R215", "cac:PostalAddress"},
		{"PEPPOL-EN16931-R219", "cbc:EndpointID"},
	}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R212", "cbc:RegistrationName"}}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R214", "cbc:CompanyID"}}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R213", "cbc:ID"}}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R217", "cac:Country"}}},
	{nsInvoice, "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R218", "cbc:IdentificationCode"}}},
	{nsInvoice, "cac:AccountingSupplierParty", []cardinalityRule{{"PEPPOL-EN16931-R220", "cac:Party"}}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party", []cardinalityRule{
		{"PEPPOL-EN16931-R221", "cac:PartyLegalEntity"},
		{"PEPPOL-EN16931-R225", "cac:PostalAddress"},
		{"PEPPOL-EN16931-R229", "cbc:EndpointID"},
	}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R222", "cbc:RegistrationName"}}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R224", "cbc:CompanyID"}}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R223", "cbc:ID"}}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R227", "cac:Country"}}},
	{nsInvoice, "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R228", "cbc:IdentificationCode"}}},
	{nsInvoice, "cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference", []cardinalityRule{{"PEPPOL-EN16931-R230", "cbc:URI"}}},
	{nsInvoice, "cac:AllowanceCharge", []cardinalityRule{{"PEPPOL-EN16931-R231", "cac:TaxCategory"}}},
	{nsInvoice, "cac:AllowanceCharge/cac:TaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R234", "cbc:ID"}}},
	{nsInvoice, "cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R233", "cbc:ID"}}},
	{nsInvoice, "cac:BillingReference", []cardinalityRule{{"PEPPOL-EN16931-R235", "cac:InvoiceDocumentReference"}}},
	{nsInvoice, "cac:Delivery/cac:DeliveryLocation/cac:Address", []cardinalityRule{{"PEPPOL-EN16931-R237", "cac:Country"}}},
	{nsInvoice, "cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R238", "cbc:IdentificationCode"}}},
	{nsInvoice, "cac:Delivery/cac:DeliveryParty", []cardinalityRule{{"PEPPOL-EN16931-R239", "cac:PartyName"}}},
	{nsInvoice, "cac:InvoiceLine", []cardinalityRule{
		{"PEPPOL-EN16931-R249", "cac:Price"},
		{"PEPPOL-EN16931-R250", "cbc:InvoicedQuantity"},
	}},
	{nsInvoice, "cac:InvoiceLine/cac:DocumentReference", []cardinalityRule{{"PEPPOL-EN16931-R240", "cbc:DocumentTypeCode"}}},
	{nsInvoice, "cac:InvoiceLine/cac:Item", []cardinalityRule{
		{"PEPPOL-EN16931-R242", "cac:ClassifiedTaxCategory"},
		{"PEPPOL-EN16931-R248", "cbc:Name"},
	}},
	{nsInvoice, "cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty", []cardinalityRule{{"PEPPOL-EN16931-R241", "cbc:Value"}}},
	{nsInvoice, "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R245", "cbc:ID"}}},
	{nsInvoice, "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R244", "cbc:ID"}}},
	{nsInvoice, "cac:InvoiceLine/cac:Item/cac:CommodityClassification", []cardinalityRule{{"PEPPOL-EN16931-R246", "cbc:ItemClassificationCode"}}},
	{nsInvoice, "cac:InvoiceLine/cac:Item/cac:OriginCountry", []cardinalityRule{{"PEPPOL-EN16931-R247", "cbc:IdentificationCode"}}},
	{nsInvoice, "cac:LegalMonetaryTotal", []cardinalityRule{
		{"PEPPOL-EN16931-R252", "cbc:LineExtensionAmount"},
		{"PEPPOL-EN16931-R253", "cbc:TaxExclusiveAmount"},
		{"PEPPOL-EN16931-R254", "cbc:TaxInclusiveAmount"},
	}},
	{nsInvoice, "cac:PayeeParty", []cardinalityRule{{"PEPPOL-EN16931-R256", "cac:PartyName"}}},
	{nsInvoice, "cac:PayeeParty/cac:PartyLegalEntity", []cardinalityRule{{"PEPPOL-EN16931-R255", "cbc:CompanyID"}}},
	{nsInvoice, "cac:PaymentMeans/cac:PayeeFinancialAccount", []cardinalityRule{{"PEPPOL-EN16931-R258", "cbc:ID"}}},
	{nsInvoice, "cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch", []cardinalityRule{{"PEPPOL-EN16931-R257", "cbc:ID"}}},
	{nsInvoice, "cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount", []cardinalityRule{{"PEPPOL-EN16931-R259", "cbc:ID"}}},
	{nsInvoice, "cac:PaymentTerms", []cardinalityRule{{"PEPPOL-EN16931-R260", "cbc:Note"}}},
	{nsInvoice, "cac:TaxRepresentativeParty", []cardinalityRule{
		{"PEPPOL-EN16931-R261", "cac:PartyName"},
		{"PEPPOL-EN16931-R262", "cac:PartyTaxScheme"},
		{"PEPPOL-EN16931-R265", "cac:PostalAddress"},
	}},
	{nsInvoice, "cac:TaxRepresentativeParty/cac:PartyTaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R264", "cbc:CompanyID"}}},
	{nsInvoice, "cac:TaxRepresentativeParty/cac:PartyTaxScheme/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R263", "cbc:ID"}}},
	{nsInvoice, "cac:TaxRepresentativeParty/cac:PostalAddress", []cardinalityRule{{"PEPPOL-EN16931-R267", "cac:Country"}}},
	{nsInvoice, "cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country", []cardinalityRule{{"PEPPOL-EN16931-R268", "cbc:IdentificationCode"}}},
	{nsInvoice, "cac:TaxTotal/cac:TaxSubtotal", []cardinalityRule{{"PEPPOL-EN16931-R272", "cbc:TaxableAmount"}}},
	{nsInvoice, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory", []cardinalityRule{{"PEPPOL-EN16931-R271", "cbc:ID"}}},
	{nsInvoice, "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme", []cardinalityRule{{"PEPPOL-EN16931-R270", "cbc:ID"}}},
}

// cardinalityByName indexes cardinalityContexts by the local name of the
// element they match, with the path split into its names.
var cardinalityByName = sync.OnceValue(func() map[string][]cardinalityMatch {
	m := map[string][]cardinalityMatch{}
	for i := range cardinalityContexts {
		c := &cardinalityContexts[i]
		var names []string
		if c.path != "" {
			names = strings.Split(c.path, "/")
		}
		last := "Invoice"
		if c.space == nsCreditNote {
			last = "CreditNote"
		}
		if len(names) > 0 {
			_, last, _ = strings.Cut(names[len(names)-1], ":")
		}
		m[last] = append(m[last], cardinalityMatch{c, names})
	}
	return m
})

// cardinalityMatch is a context with its path split into UBL names.
type cardinalityMatch struct {
	*cardinalityContext
	names []string
}

// matches reports whether n matches the context: its ancestors carry the
// names of the path, below an element ubl-invoice:Invoice or
// ubl-creditnote:CreditNote.
func (c cardinalityMatch) matches(n *xmltree.Node) bool {
	for i := len(c.names) - 1; i >= 0; i-- {
		if !uis(n, c.names[i]) {
			return false
		}
		n = n.Parent
	}
	if c.space == nsCreditNote {
		return isElem(n, nsCreditNote, "CreditNote")
	}
	return isElem(n, nsInvoice, "Invoice")
}

// cardinality checks the rules of the pattern on n.
func (k *ublChecker) cardinality(n *xmltree.Node) {
	for _, c := range cardinalityByName()[n.Name] {
		if !c.matches(n) {
			continue
		}
		for _, r := range c.rules {
			if got := ucount(n, r.child); got != 1 {
				k.fail(r.id, n, "%d %s", got, r.child)
			}
		}
		return
	}
}
