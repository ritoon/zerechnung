package de

import (
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Rules of the XRechnung UBL schematron (XRechnung-UBL-validation.sch):
// patterns ubl-pattern, ubl-extension-pattern and ubl-cvd-pattern. The Peppol
// patterns that it includes are in peppol.go.

// ublChecker runs the rules on a UBL Invoice or CreditNote.
type ublChecker struct {
	*checker
	line  string   // name of the lines of the root element: InvoiceLine or CreditNoteLine
	slack *big.Rat // $slackValue of the Peppol patterns
}

// checkUBL runs the rules of the UBL schematron on the UBL document of c.
func checkUBL(c *rules.Checker) {
	root := c.Doc.UBL
	k := &ublChecker{checker: &checker{Checker: c, root: root}, line: "InvoiceLine"}
	if root.Name == "CreditNote" {
		k.line = "CreditNoteLine"
	}
	k.ext = k.customization(ExtensionID)
	k.cvd = k.customization(CVDID)

	k.ublPeppol() // peppol-ubl-pattern-1 and peppol-ubl-pattern-2: PEPPOL-EN16931-R*

	// ubl-pattern
	k.ublDocument()     // BR-DE-1, 15, 16, 17, 18, 21, 22, 26, 30, 31, BR-DE-TMP-32
	k.ublExternalRefs() // BR-TMP-2
	k.ublDates()        // BR-TMP-6
	k.ublSeller()       // BR-DE-2 to BR-DE-7, BR-DE-27, BR-DE-28
	k.ublAddresses()    // BR-DE-8 to BR-DE-11
	k.ublPaymentMeans() // BR-DE-19, 20, 23, 24, 25
	k.ublVATRates()     // BR-DE-14
	k.ublExtension()    // ubl-extension-pattern: BR-DEX-*
	k.ublVehicles()     // ubl-cvd-pattern: BR-DE-CVD-*, BR-TMP-CVD-01
}

// customization reports whether a text of BT-24 equals id
// (cbc:CustomizationID/text() = id).
func (k *ublChecker) customization(id string) bool {
	for n := range all(k.root, "CustomizationID") {
		if n.Text == id {
			return true
		}
	}
	return false
}

// typeCodes are the invoice type codes of BR-DE-17.
var typeCodes = []string{"326", "380", "384", "389", "381", "875", "876", "877"}

// ublDocument checks the rules whose context is the root element.
func (k *ublChecker) ublDocument() {
	r := k.root
	if !exists(r, "PaymentMeans") {
		k.fail("BR-DE-1", rules.Error, nil, "[BR-DE-1] An invoice must contain information on PAYMENT INSTRUCTIONS (BG-16, cac:PaymentMeans).")
	}
	if !nonBlank(r, "BuyerReference") {
		k.fail("BR-DE-15", rules.Error, first(r, "BuyerReference"), "[BR-DE-15] The element Buyer reference (BT-10, cbc:BuyerReference) must be transmitted; value: %q.", sv(first(r, "BuyerReference")))
	}

	// BR-DE-16: the allowance variable without condition on the tax scheme
	// ($BT-95-UBL-CN) is tested for invoices too.
	used := anyIn(r, "InvoiceLine/Item/ClassifiedTaxCategory/ID", taxCodes...) ||
		anyIn(r, "CreditNoteLine/Item/ClassifiedTaxCategory/ID", taxCodes...)
	for ac := range all(r, "AllowanceCharge") {
		if anyIn(ac, "ChargeIndicator", "false", "true") && anyIn(ac, "TaxCategory/ID", taxCodes...) {
			used = true
		}
	}
	if used && !exists(r, "TaxRepresentativeParty") && !nonBlank(r, "AccountingSupplierParty/Party/PartyTaxScheme/CompanyID") {
		k.fail("BR-DE-16", rules.Error, nil, "[BR-DE-16] If the VAT category codes S, Z, E, AE, K, G, L or M are used in an invoice, at least one of the elements Seller VAT identifier (BT-31) or Seller tax registration identifier (BT-32) (cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID), or SELLER TAX REPRESENTATIVE PARTY (BG-11, cac:TaxRepresentativeParty) must be transmitted.")
	}

	invType, cnType := first(r, "InvoiceTypeCode"), first(r, "CreditNoteTypeCode")
	typeNode := orNode(invType, cnType)
	if !in(nsv(invType), typeCodes...) && !in(nsv(cnType), typeCodes...) {
		k.fail("BR-DE-17", rules.Warning, typeNode, "[BR-DE-17] The Invoice type code (BT-3, cbc:InvoiceTypeCode or cbc:CreditNoteTypeCode) should only be one of the UNTDID 1001 codes 326 (Partial invoice), 380 (Commercial invoice), 384 (Corrected invoice), 389 (Self-billed invoice), 381 (Credit note), 875 (Partial construction invoice), 876 (Partial final construction invoice) and 877 (Final construction invoice); value: %q.", nsv(typeNode))
	}

	// BR-DE-18: first note of each payment terms.
	var notes []*xmltree.Node
	for terms := range all(r, "PaymentTerms") {
		if n := first(terms, "Note"); n != nil {
			notes = append(notes, n)
		}
	}
	k.skonto(notes)

	k.ublFileNames() // BR-DE-22

	if !anyIn(r, "CustomizationID", CIUSID, ExtensionID, CVDID) {
		k.fail("BR-DE-21", rules.Warning, first(r, "CustomizationID"), "[BR-DE-21] The Specification identifier (BT-24, cbc:CustomizationID) should syntactically match the identifier of the XRechnung standard (%s, or the identifier of the extension or of the CVD); value: %q.", CIUSID, sv(first(r, "CustomizationID")))
	}

	if (nsv(invType) == "384" || nsv(cnType) == "384") && !exists(r, "BillingReference/InvoiceDocumentReference") {
		k.fail("BR-DE-26", rules.Warning, typeNode, "[BR-DE-26] If the Invoice type code (BT-3) is 384 (Corrected invoice), PRECEDING INVOICE REFERENCE (BG-3, cac:BillingReference/cac:InvoiceDocumentReference) should be present at least once.")
	}

	if mandate := first(r, "PaymentMeans/PaymentMandate"); mandate != nil {
		if !anyAttrEq(r, "AccountingSupplierParty/Party/PartyIdentification/ID", "schemeID", "SEPA") &&
			!anyAttrEq(r, "PayeeParty/PartyIdentification/ID", "schemeID", "SEPA") {
			k.fail("BR-DE-30", rules.Error, mandate, "[BR-DE-30] If DIRECT DEBIT (BG-19, cac:PaymentMandate) is present, the Bank assigned creditor identifier (BT-90) must be transmitted: a cac:PartyIdentification/cbc:ID with schemeID=\"SEPA\" of the seller or of the payee.")
		}
		if !exists(r, "PaymentMeans/PaymentMandate/PayerFinancialAccount/ID") {
			k.fail("BR-DE-31", rules.Error, mandate, "[BR-DE-31] If DIRECT DEBIT (BG-19, cac:PaymentMandate) is present, the Debited account identifier (BT-91, cac:PayerFinancialAccount/cbc:ID) must be transmitted.")
		}
	}

	if !exists(r, "Delivery/ActualDeliveryDate") && !exists(r, "InvoicePeriod") {
		every := true
		for _, line := range r.Children {
			if (line.Name == "InvoiceLine" || line.Name == "CreditNoteLine") && !exists(line, "InvoicePeriod") {
				every = false
				break
			}
		}
		if !every {
			k.fail("BR-DE-TMP-32", rules.Info, nil, "[BR-DE-TMP-32] An invoice should give the delivery or service date with either the Actual delivery date (BT-72, cac:Delivery/cbc:ActualDeliveryDate), the Invoicing period (BG-14, cac:InvoicePeriod) or an Invoice line period (BG-26) on every line.")
		}
	}
}

// ublFileNames checks BR-DE-22: no cac:AdditionalDocumentReference of the
// root has an attachment file name equal to the one of a preceding sibling.
func (k *ublChecker) ublFileNames() {
	seen := map[string]bool{}
	for doc := range all(k.root, "AdditionalDocumentReference") {
		var names []string
		for obj := range all(doc, "Attachment/EmbeddedDocumentBinaryObject") {
			if f, ok := obj.LookupAttr("filename"); ok {
				names = append(names, f)
			}
		}
		for _, f := range names {
			if seen[f] {
				k.fail("BR-DE-22", rules.Error, nil, "[BR-DE-22] Not all filename attributes of the attached documents (BT-125, cbc:EmbeddedDocumentBinaryObject) are unique; repeated file name: %q.", f)
				return
			}
		}
		for _, f := range names {
			seen[f] = true
		}
	}
}

// ublExternalRefs checks BR-TMP-2 on the external references of the
// additional supporting documents: the URI is an absolute URL.
func (k *ublChecker) ublExternalRefs() {
	for ref := range all(k.root, "AdditionalDocumentReference/Attachment/ExternalReference") {
		u := first(ref, "URI")
		if !validURL(sv(u)) {
			k.fail("BR-TMP-2", rules.Error, orNode(u, ref), "[BR-TMP-2] The External document location (BT-124, cbc:URI) must contain an absolute URL with a valid scheme; value: %q.", sv(u))
		}
	}
}

// dateNames are the cbc elements checked by BR-TMP-6.
var dateNames = []string{"IssueDate", "DueDate", "StartDate", "EndDate", "ActualDeliveryDate", "TaxPointDate", "PaymentDueDate"}

// ublDates checks BR-TMP-6: every date element has the form YYYY-MM-DD.
func (k *ublChecker) ublDates() {
	k.root.Walk(func(n *xmltree.Node) bool {
		if n.Space == ubl.NSCbc && in(n.Name, dateNames...) {
			if v := norm(n.Text); !isoDate(v) {
				k.fail("BR-TMP-6", rules.Warning, n, "[BR-TMP-6] Dates in UBL must have the form YYYY-MM-DD; value: %q.", v)
			}
		}
		return true
	})
}

// isoDate implements matches(s, '^\d{4}-\d{2}-\d{2}$').
func isoDate(s string) bool {
	if utf8.RuneCountInString(s) != 10 {
		return false
	}
	i := 0
	for _, r := range s {
		if i == 4 || i == 7 {
			if r != '-' {
				return false
			}
		} else if !isDigit(r) {
			return false
		}
		i++
	}
	return true
}

// ublSeller checks BR-DE-2 to BR-DE-7, BR-DE-27 and BR-DE-28 on the seller,
// its address and its contact.
func (k *ublChecker) ublSeller() {
	const party = "AccountingSupplierParty/Party"
	for sup := range all(k.root, "AccountingSupplierParty") {
		if !exists(sup, "Party/Contact") {
			k.fail("BR-DE-2", rules.Error, sup, "[BR-DE-2] The group SELLER CONTACT (BG-6, cac:Contact) must be transmitted.")
		}
	}
	for addr := range all(k.root, party+"/PostalAddress") {
		if !nonBlank(addr, "CityName") {
			k.fail("BR-DE-3", rules.Error, addr, "[BR-DE-3] The element Seller city (BT-37, cbc:CityName) must be transmitted; value: %q.", sv(first(addr, "CityName")))
		}
		if !nonBlank(addr, "PostalZone") {
			k.fail("BR-DE-4", rules.Error, addr, "[BR-DE-4] The element Seller post code (BT-38, cbc:PostalZone) must be transmitted; value: %q.", sv(first(addr, "PostalZone")))
		}
	}
	for contact := range all(k.root, party+"/Contact") {
		if !nonBlank(contact, "Name") {
			k.fail("BR-DE-5", rules.Error, contact, "[BR-DE-5] The element Seller contact point (BT-41, cbc:Name) must be transmitted.")
		}
		if !nonBlank(contact, "Telephone") {
			k.fail("BR-DE-6", rules.Error, contact, "[BR-DE-6] The element Seller contact telephone number (BT-42, cbc:Telephone) must be transmitted.")
		}
		if !nonBlank(contact, "ElectronicMail") {
			k.fail("BR-DE-7", rules.Error, contact, "[BR-DE-7] The element Seller contact email address (BT-43, cbc:ElectronicMail) must be transmitted.")
		}
		phoneNode := first(contact, "Telephone")
		if phone := nsv(phoneNode); !validPhone(phone) {
			k.fail("BR-DE-27", rules.Warning, orNode(phoneNode, contact), "[BR-DE-27] The Seller contact telephone number (BT-42) should contain at least three digits; value: %q.", phone)
		}
		emailNode := first(contact, "ElectronicMail")
		if email := nsv(emailNode); !validEmail(email) {
			k.fail("BR-DE-28", rules.Warning, orNode(emailNode, contact), "[BR-DE-28] The Seller contact email address (BT-43) should contain exactly one @ sign, with at least one character before it and a domain of at least two dot-separated labels after it, without white space; value: %q.", email)
		}
	}
}

// ublAddresses checks BR-DE-8 to BR-DE-11: city and post code of the buyer
// and deliver to addresses.
func (k *ublChecker) ublAddresses() {
	for _, a := range []struct {
		path             string
		city, postcode   string
		cityMsg, postMsg string
	}{
		{"AccountingCustomerParty/Party/PostalAddress", "BR-DE-8", "BR-DE-9",
			"[BR-DE-8] The element Buyer city (BT-52, cbc:CityName) must be transmitted; value: %q.",
			"[BR-DE-9] The element Buyer post code (BT-53, cbc:PostalZone) must be transmitted; value: %q."},
		{"Delivery/DeliveryLocation/Address", "BR-DE-10", "BR-DE-11",
			"[BR-DE-10] The element Deliver to city (BT-77, cbc:CityName) must be transmitted if the group DELIVER TO ADDRESS (BG-15) is transmitted; value: %q.",
			"[BR-DE-11] The element Deliver to post code (BT-78, cbc:PostalZone) must be transmitted if the group DELIVER TO ADDRESS (BG-15) is transmitted; value: %q."},
	} {
		for addr := range all(k.root, a.path) {
			if !nonBlank(addr, "CityName") {
				k.fail(a.city, rules.Error, addr, a.cityMsg, sv(first(addr, "CityName")))
			}
			if !nonBlank(addr, "PostalZone") {
				k.fail(a.postcode, rules.Error, addr, a.postMsg, sv(first(addr, "PostalZone")))
			}
		}
	}
}

// ublPaymentMeans checks BR-DE-19, BR-DE-20 and BR-DE-23 to BR-DE-25 on each
// payment means of the root, according to its type code (BT-81).
func (k *ublChecker) ublPaymentMeans() {
	const payee, card, mandate = "PayeeFinancialAccount", "CardAccount", "PaymentMandate"
	for pm := range all(k.root, "PaymentMeans") {
		code := nsv(first(pm, "PaymentMeansCode"))
		switch code {
		case "30", "58":
			if code == "58" {
				id := first(pm, payee+"/ID")
				if iban := sv(id); !checkIBAN(iban) {
					k.fail("BR-DE-19", rules.Warning, orNode(id, pm), "[BR-DE-19] The Payment account identifier (BT-84, cac:PayeeFinancialAccount/cbc:ID) should contain a valid IBAN when the Payment means type code (BT-81) is 58 (SEPA credit transfer); value: %q.", iban)
				}
			}
			if !exists(pm, payee) {
				k.fail("BR-DE-23-a", rules.Error, pm, "[BR-DE-23-a] If the Payment means type code (BT-81) is a credit transfer code (30, 58), CREDIT TRANSFER (BG-17, cac:PayeeFinancialAccount) must be transmitted; type code: %q.", code)
			}
			if exists(pm, card) || exists(pm, mandate) {
				k.fail("BR-DE-23-b", rules.Error, pm, "[BR-DE-23-b] If the Payment means type code (BT-81) is a credit transfer code (30, 58), PAYMENT CARD INFORMATION (BG-18, cac:CardAccount) and DIRECT DEBIT (BG-19, cac:PaymentMandate) must not be transmitted; type code: %q.", code)
			}
		case "48", "54", "55":
			if !exists(pm, card) {
				k.fail("BR-DE-24-a", rules.Error, pm, "[BR-DE-24-a] If the Payment means type code (BT-81) is a payment card code (48, 54, 55), PAYMENT CARD INFORMATION (BG-18, cac:CardAccount) must be transmitted; type code: %q.", code)
			}
			if exists(pm, payee) || exists(pm, mandate) {
				k.fail("BR-DE-24-b", rules.Error, pm, "[BR-DE-24-b] If the Payment means type code (BT-81) is a payment card code (48, 54, 55), CREDIT TRANSFER (BG-17, cac:PayeeFinancialAccount) and DIRECT DEBIT (BG-19, cac:PaymentMandate) must not be transmitted; type code: %q.", code)
			}
		case "59":
			id := first(pm, mandate+"/PayerFinancialAccount/ID")
			if iban := sv(id); !checkIBAN(iban) {
				k.fail("BR-DE-20", rules.Warning, orNode(id, pm), "[BR-DE-20] The Debited account identifier (BT-91, cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID) should contain a valid IBAN when the Payment means type code (BT-81) is 59 (SEPA direct debit); value: %q.", iban)
			}
			if !exists(pm, mandate) {
				k.fail("BR-DE-25-a", rules.Error, pm, "[BR-DE-25-a] If the Payment means type code (BT-81) is a direct debit code (59), DIRECT DEBIT (BG-19, cac:PaymentMandate) must be transmitted.")
			}
			if exists(pm, payee) || exists(pm, card) {
				k.fail("BR-DE-25-b", rules.Error, pm, "[BR-DE-25-b] If the Payment means type code (BT-81) is a direct debit code (59), CREDIT TRANSFER (BG-17, cac:PayeeFinancialAccount) and PAYMENT CARD INFORMATION (BG-18, cac:CardAccount) must not be transmitted.")
			}
		}
	}
}

// ublVATRates checks BR-DE-14: every VAT breakdown (BG-23) has a rate
// (BT-119).
func (k *ublChecker) ublVATRates() {
	for st := range all(k.root, "TaxTotal/TaxSubtotal") {
		if !nonBlank(st, "TaxCategory/Percent") {
			k.fail("BR-DE-14", rules.Error, st, "[BR-DE-14] The element VAT category rate (BT-119, cac:TaxCategory/cbc:Percent) must be transmitted; value: %q.", sv(first(st, "TaxCategory/Percent")))
		}
	}
}

// isCbc and isCac report whether n is the element name of the UBL basic or
// aggregate components.
func isCbc(n *xmltree.Node, name string) bool {
	return n != nil && n.Name == name && n.Space == ubl.NSCbc
}

func isCac(n *xmltree.Node, name string) bool {
	return n != nil && n.Name == name && n.Space == ubl.NSCac
}

// ublExtension runs the rules of ubl-extension-pattern, which apply when
// BT-24 is ExtensionID.
func (k *ublChecker) ublExtension() {
	if !k.ext {
		return
	}
	r := k.root
	invoiceRoot := r.Name == "Invoice"
	if invoiceRoot {
		k.subInvoiceLines() // BR-DEX-02, BR-DEX-03
	}
	r.Walk(func(n *xmltree.Node) bool {
		scheme, hasScheme := n.LookupAttr("schemeID")
		switch {
		case isCbc(n, "EmbeddedDocumentBinaryObject"):
			if mime, ok := n.LookupAttr("mimeCode"); !ok || !in(mime, mimeCodes...) {
				k.fail("BR-DEX-01", rules.Error, n, "[BR-DEX-01] The Attached document (BT-125, cbc:EmbeddedDocumentBinaryObject) uses a MIME code that is not allowed: %q; with the extension, application/xml is allowed in addition to application/pdf, image/png, image/jpeg, text/csv and the OOXML and ODF spreadsheet codes.", mime)
			}
		case isCac(n, "LegalMonetaryTotal"):
			k.amountDue(n) // BR-DEX-09
		case !hasScheme:
		case isCbc(n, "ID") && isCac(n.Parent, "PartyIdentification"):
			sepa := inList(" SEPA ", scheme) && (hasAncestor(n, "AccountingSupplierParty") || hasAncestor(n, "PayeeParty"))
			if !inList(iso6523Ext, scheme) && !sepa {
				k.fail("BR-DEX-04", rules.Error, n, "[BR-DEX-04] Any scheme identifier of a Party identifier (cac:PartyIdentification/cbc:ID) must be coded using the ISO 6523 ICD list, extended with XR01, XR02 and XR03, or be SEPA for the seller and the payee; schemeID: %q.", scheme)
			}
		case isCbc(n, "CompanyID") && isCac(n.Parent, "PartyLegalEntity"):
			if !inList(iso6523Ext, scheme) {
				k.fail("BR-DEX-05", rules.Error, n, "[BR-DEX-05] Any scheme identifier of a legal registration identifier (cac:PartyLegalEntity/cbc:CompanyID) must be coded using the ISO 6523 ICD list, extended with XR01, XR02 and XR03; schemeID: %q.", scheme)
			}
		case isCbc(n, "ID") && isCac(n.Parent, "StandardItemIdentification"):
			if !inList(iso6523Ext, scheme) {
				k.fail("BR-DEX-06", rules.Error, n, "[BR-DEX-06] Any scheme identifier of an Item standard identifier (BT-157, cac:StandardItemIdentification/cbc:ID) must be coded using the ISO 6523 ICD list, extended with XR01, XR02 and XR03; schemeID: %q.", scheme)
			}
		case isCbc(n, "EndpointID"):
			if !inList(cefEASExt, scheme) {
				k.fail("BR-DEX-07", rules.Error, n, "[BR-DEX-07] Any scheme identifier of an electronic address (BT-34, BT-49, cbc:EndpointID) must belong to the CEF EAS code list, extended with XR01, XR02 and XR03; schemeID: %q.", scheme)
			}
		case isCbc(n, "ID") && isCac(n.Parent, "DeliveryLocation"):
			if !inList(iso6523Ext, scheme) {
				k.fail("BR-DEX-08", rules.Error, n, "[BR-DEX-08] Any scheme identifier of a Deliver to location identifier (BT-71, cac:DeliveryLocation/cbc:ID) must be coded using the ISO 6523 ICD list, extended with XR01, XR02 and XR03; schemeID: %q.", scheme)
			}
		}
		return true
	})
	if invoiceRoot {
		k.thirdPartyPayments() // BR-DEX-10 to BR-DEX-14
	}
}

// subInvoiceLines checks BR-DEX-02 and BR-DEX-03 on the sub invoice lines
// (BG-DEX-01) of an invoice.
func (k *ublChecker) subInvoiceLines() {
	r := k.root
	sums := true
	for line := range all(r, "InvoiceLine") {
		if exists(line, "SubInvoiceLine") && !subLinesSum(line) {
			sums = false
			break
		}
	}
	taxes := true
	r.Walk(func(n *xmltree.Node) bool {
		if !isCac(n, "SubInvoiceLine") {
			return true
		}
		if sums && exists(n, "SubInvoiceLine") && !subLinesSum(n) {
			sums = false
		}
		for item := range all(n, "Item") {
			if count(item, "ClassifiedTaxCategory") != 1 {
				taxes = false
			}
		}
		return true
	})
	if !sums {
		k.fail("BR-DEX-02", rules.Warning, nil, "[BR-DEX-02] The Invoice line net amount (BT-131, cbc:LineExtensionAmount) of an INVOICE LINE (BG-25) or a SUB INVOICE LINE (BG-DEX-01) should be the sum of the net amounts of its direct SUB INVOICE LINES (cac:SubInvoiceLine).")
	}
	if !taxes {
		k.fail("BR-DEX-03", rules.Error, nil, "[BR-DEX-03] A SUB INVOICE LINE (BG-DEX-01) must contain exactly one SUB INVOICE LINE VAT INFORMATION (BG-DEX-06, cac:Item/cac:ClassifiedTaxCategory).")
	}
}

// subLinesSum reports whether the net amount of line equals the sum of the
// net amounts of its direct sub invoice lines
// (xs:decimal(cbc:LineExtensionAmount) =
// sum(cac:SubInvoiceLine/xs:decimal(cbc:LineExtensionAmount))). A missing
// net amount of line, or a value that is not a decimal, fails.
func subLinesSum(line *xmltree.Node) bool {
	want, ok := xsDecimal(first(line, "LineExtensionAmount"))
	if !ok {
		return false
	}
	sum := invoice.DecimalFromInt(0)
	for sub := range all(line, "SubInvoiceLine") {
		a := first(sub, "LineExtensionAmount")
		if a == nil {
			continue
		}
		v, ok := xsDecimal(a)
		if !ok {
			return false
		}
		sum = sum.Add(v)
	}
	return want.Equal(sum)
}

// xsDecimal implements xs:decimal(n): the string value of n, without the
// XML white space around it, as a decimal. It fails when n is nil or the
// value is not a decimal.
func xsDecimal(n *xmltree.Node) (invoice.Decimal, bool) {
	if n == nil {
		return invoice.Decimal{}, false
	}
	s := strings.Trim(sv(n), " \t\n\r")
	if s == "" || strings.TrimSpace(s) != s {
		return invoice.Decimal{}, false
	}
	d, err := invoice.ParseDecimal(s)
	return d, err == nil
}

// amountDue checks BR-DEX-09 on a cac:LegalMonetaryTotal: Amount due for
// payment (BT-115) - Rounding amount (BT-114) = Invoice total amount with
// VAT (BT-112) - Paid amount (BT-113) + the third party payment amounts
// (BT-DEX-002), both sides rounded half up to 2 decimals.
func (k *ublChecker) amountDue(t *xmltree.Node) {
	optional := func(name string) (invoice.Decimal, bool) {
		if n := first(t, name); n != nil {
			return xsDecimal(n)
		}
		return invoice.DecimalFromInt(0), true
	}
	payable, ok1 := xsDecimal(first(t, "PayableAmount"))
	total, ok2 := xsDecimal(first(t, "TaxInclusiveAmount"))
	prepaid, ok3 := optional("PrepaidAmount")
	rounding, ok4 := optional("PayableRoundingAmount")
	third, ok5 := invoice.DecimalFromInt(0), true
	if nonBlank(t.Parent, "PrepaidPayment/PaidAmount") {
		for pp := range all(t.Parent, "PrepaidPayment") {
			a := first(pp, "PaidAmount")
			if a == nil {
				continue
			}
			v, ok := xsDecimal(a)
			if !ok {
				ok5 = false
				break
			}
			third = third.Add(v)
		}
	}
	if ok1 && ok2 && ok3 && ok4 && ok5 &&
		payable.Sub(rounding).RoundHalfCeil(2).Equal(total.Sub(prepaid).Add(third).RoundHalfCeil(2)) {
		return
	}
	k.fail("BR-DEX-09", rules.Error, t, "[BR-DEX-09] Amount due for payment (BT-115) = Invoice total amount with VAT (BT-112) - Paid amount (BT-113) + Rounding amount (BT-114) + Σ Third party payment amount (BT-DEX-002, cac:PrepaidPayment/cbc:PaidAmount); due: %q, total: %q, paid: %q, rounding: %q, third party: %s.",
		sv(first(t, "PayableAmount")), sv(first(t, "TaxInclusiveAmount")), sv(first(t, "PrepaidAmount")), sv(first(t, "PayableRoundingAmount")), third)
}

// thirdPartyPayments checks BR-DEX-10 to BR-DEX-14 on the third party
// payments (BG-DEX-09, cac:PrepaidPayment) of an invoice.
func (k *ublChecker) thirdPartyPayments() {
	r := k.root
	for pp := range all(r, "PrepaidPayment") {
		if !nonBlank(pp, "ID") {
			k.fail("BR-DEX-10", rules.Error, pp, "[BR-DEX-10] The element Third party payment type (BT-DEX-001, cbc:ID) must be transmitted if the group THIRD PARTY PAYMENT (BG-DEX-09) is transmitted.")
		}
		if !nonBlank(pp, "PaidAmount") {
			k.fail("BR-DEX-11", rules.Error, pp, "[BR-DEX-11] The element Third party payment amount (BT-DEX-002, cbc:PaidAmount) must be transmitted if the group THIRD PARTY PAYMENT (BG-DEX-09) is transmitted.")
		}
		if !nonBlank(pp, "InstructionID") {
			k.fail("BR-DEX-12", rules.Error, pp, "[BR-DEX-12] The element Third party payment description (BT-DEX-003, cbc:InstructionID) must be transmitted if the group THIRD PARTY PAYMENT (BG-DEX-09) is transmitted.")
		}
		amount := sv(first(pp, "PaidAmount"))
		if _, decimals, ok := strings.Cut(amount, "."); ok && utf8.RuneCountInString(decimals) > 2 {
			k.fail("BR-DEX-13", rules.Error, pp, "[BR-DEX-13] The Third party payment amount (BT-DEX-002) must have at most 2 decimals; value: %q.", amount)
		}
		currency := false
		for a := range all(pp, "PaidAmount") {
			if c, ok := a.LookupAttr("currencyID"); ok && anyEq(r, "DocumentCurrencyCode", c) {
				currency = true
			}
		}
		if !currency {
			k.fail("BR-DEX-14", rules.Error, pp, "[BR-DEX-14] The currency of the Third party payment amount (BT-DEX-002, cbc:PaidAmount/@currencyID) must be the Invoice currency code (BT-5); currency: %q, invoice currency: %q.", attr(first(pp, "PaidAmount"), "currencyID"), sv(first(r, "DocumentCurrencyCode")))
		}
	}
}

// ublVehicles runs the rules of ubl-cvd-pattern, which apply when BT-24 is
// CVDID.
func (k *ublChecker) ublVehicles() {
	if !k.cvd {
		return
	}
	r := k.root
	const class = "CommodityClassification/ItemClassificationCode"
	const prop = "AdditionalItemProperty"
	if !nonBlank(r, "OriginatorDocumentReference/ID") {
		k.fail("BR-DE-CVD-02", rules.Error, nil, "[BR-DE-CVD-02] The element Tender or lot reference (BT-17, cac:OriginatorDocumentReference/cbc:ID) must be transmitted.")
	}
	if !nonBlank(r, "ContractDocumentReference/ID") {
		k.fail("BR-DE-CVD-01", rules.Error, nil, "[BR-DE-CVD-01] The element Contract reference (BT-12, cac:ContractDocumentReference/cbc:ID) must be transmitted.")
	}
	found := false
	for _, path := range []string{"InvoiceLine/Item", "CreditNoteLine/Item"} {
		for item := range all(r, path) {
			if anyAttrEq(item, class, "listID", "CVD") && anyEq(item, prop+"/Name", "cva") {
				found = true
			}
		}
	}
	if !found {
		k.fail("BR-DE-CVD-03", rules.Error, nil, "[BR-DE-CVD-03] An invoice must contain at least one INVOICE LINE (BG-25) whose Item classification identifier (BT-158, cbc:ItemClassificationCode) has the scheme identifier 'CVD' and whose Item attribute name (BT-160, cac:AdditionalItemProperty/cbc:Name) is 'cva'.")
	}
	for item := range all(r, k.line+"/Item") {
		cvdClasses := 0
		for c := range all(item, class) {
			if a, ok := c.LookupAttr("listID"); ok && a == "CVD" {
				cvdClasses++
			}
		}
		cvaProps := 0
		for p := range all(item, prop) {
			if anyEq(p, "Name", "cva") {
				cvaProps++
			}
		}
		if cvdClasses > 0 && cvaProps != 1 {
			k.fail("BR-DE-CVD-06-a", rules.Error, item, "[BR-DE-CVD-06-a] If an Item classification identifier (BT-158) with the scheme identifier 'CVD' is given, the same invoice line must contain exactly one Item attribute name (BT-160) 'cva'; found: %d.", cvaProps)
		}
		if cvaProps > 0 && cvdClasses != 1 {
			k.fail("BR-DE-CVD-06-b", rules.Error, item, "[BR-DE-CVD-06-b] If the Item attribute name (BT-160) 'cva' is given, the same invoice line must contain exactly one Item classification identifier (BT-158) with the scheme identifier 'CVD'; found: %d.", cvdClasses)
		}
	}
	for c := range all(r, k.line+"/Item/"+class) {
		list := attr(c, "listID")
		if !inList(untdid7143CVD, list) {
			k.fail("BR-TMP-CVD-01", rules.Error, c, "[BR-TMP-CVD-01] The scheme identifier of the Item classification identifier (BT-158, cbc:ItemClassificationCode/@listID) must be taken from the code list UNTDID 7143; listID: %q.", list)
		}
		if norm(list) == "CVD" && !in(nsv(c), cvdVehicleCategories...) {
			k.fail("BR-DE-CVD-04", rules.Error, c, "[BR-DE-CVD-04] An Item classification identifier (BT-158) with the scheme identifier 'CVD' must contain one of the vehicle categories M1, M2, M3, N1, N2, N3; value: %q.", nsv(c))
		}
	}
	for p := range all(r, k.line+"/Item/"+prop) {
		if !anyEq(p, "Name", "cva") {
			continue
		}
		if v := nsv(first(p, "Value")); !in(v, cvaCodes...) {
			k.fail("BR-DE-CVD-05", rules.Error, p, "[BR-DE-CVD-05] If the Item attribute name (BT-160) is 'cva', the Item attribute value (BT-161, cbc:Value) must be one of clean, zero-emission, other; value: %q.", v)
		}
	}
}
