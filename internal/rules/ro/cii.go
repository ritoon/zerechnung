package ro

import (
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/internal/codes"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// ANAF publishes the schematron of the CIUS-RO for the UBL syntax only,
// although the order OMF 4092/2022 also admits CII D16B. ciiPatterns
// transposes its rules to CII: each assertion of the UBL schematron that
// can fail on a valid UBL document, with the same identifier, the same
// flag and, where the meaning is the same, the same message, checks the
// same business terms at their CII paths. The transposition follows the
// rules as the schematron applies them, not as their messages describe
// them:
//
//   - an assertion that never fails is left out: BR-DEC-RO-13 and
//     BR-DEC-RO-15, whose context /ubl:Invoice | cac:CreditNote matches
//     no element that an earlier rule has not taken (the root of an
//     invoice) or no element at all (cac:CreditNote is not the root of a
//     credit note); BR-DEC-RO-23 and BR-RO-L1019, whose contexts come after
//     rules on the same elements; BR-RO-A051, BR-RO-A052 and BR-RO-A500,
//     which count the context element itself or a child that UBL does not
//     repeat;
//   - the rules of the UBL credit note (BR-RO-020_2, BR-RO-DT003_CN) have
//     no CII counterpart, as a CII credit note is a CrossIndustryInvoice of
//     type 381: BR-RO-020_1 admits 381, as its message says, and
//     BR-RO-DT003 checks the payment due date (BT-9) of both;
//   - the dates are written YYYYMMDD (format 102) in CII: BR-RO-DT001 to
//     BR-RO-DT006 check that the text has eight characters and is a date,
//     as their UBL versions check the ten characters of YYYY-MM-DD;
//   - the value added tax point date code (BT-8) uses UNTDID 2475 in CII:
//     BR-RO-040 admits 5, 29 and 72, the codes that EN 16931 maps to 3, 35
//     and 432 of UNTDID 2005;
//   - BR-RO-L1017 and BR-RO-L1018 check the length of the reason code
//     (BT-98, BT-105), as their UBL tests do, although their messages name
//     the reason (BT-97, BT-104);
//   - BR-RO-065 and BR-RO-120 read the VAT categories of the document
//     level allowances and charges and of the lines, as the UBL tests do
//     on an invoice; BR-RO-L0308 to BR-RO-L211 apply to the additional
//     referenced documents of the header but those of type 50 (the tender
//     or lot reference, BT-17, which BR-RO-L0307 checks), as UBL holds
//     BT-17 in cac:OriginatorDocumentReference.
var ciiPatterns = buildCIIPatterns()

// ciiNamespaces are the prefixes of ciiPatterns.
var ciiNamespaces = map[string]string{
	"rsm": rules.NSRsm,
	"ram": rules.NSRam,
	"udt": rules.NSUdt,
	"qdt": rules.NSQdt,
	"xs":  "http://www.w3.org/2001/XMLSchema",
}

// Paths of the CII invoice, relative to its root element.
const (
	ciiAgreement = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement"
	ciiDelivery  = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeDelivery"
	ciiSettle    = "rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement"
	ciiSeller    = ciiAgreement + "/ram:SellerTradeParty"
	ciiBuyer     = ciiAgreement + "/ram:BuyerTradeParty"
	ciiTaxRep    = ciiAgreement + "/ram:SellerTaxRepresentativeTradeParty"
	ciiShipTo    = ciiDelivery + "/ram:ShipToTradeParty"
)

// ciiDate is the test of BR-RO-DT001 to BR-RO-DT006: eight characters
// that form a date YYYYMMDD.
const ciiDate = "string-length(text()) = 8 and (concat(substring(., 1, 4), '-', substring(., 5, 2), '-', substring(., 7, 2)) castable as xs:date)"

// ciiVATCategories is true when the VAT category of a document level
// allowance (of the tax scheme VAT) or charge, or of a line, is one that
// BR-RO-065 and BR-RO-120 name.
const ciiVATCategories = `(` + ciiSettle + `/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = 'false']/ram:CategoryTradeTax[ram:TypeCode = 'VAT']/ram:CategoryCode = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or ` +
	`(` + ciiSettle + `/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = 'true']/ram:CategoryTradeTax/ram:CategoryCode = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or ` +
	`(rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax/ram:CategoryCode = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M'))`

func maxLen(path string, n int) string {
	return fmt.Sprintf("string-length(normalize-space(%s)) <= %d", path, n)
}

func decimals(path string) string {
	return fmt.Sprintf("string-length(substring-after(%s, '.')) <= 2", path)
}

func nonBlank(path string) string { return path + "[boolean(normalize-space(.))]" }

// bucharest is the report of BR-RO-100 and its siblings on the address at
// path ("" for the context element): in Romania, in Bucharest, and its city
// is not a sector.
func bucharest(addr string) string {
	return fmt.Sprintf("normalize-space(%[1]sram:CountryID) = 'RO' and normalize-space(%[1]sram:CountrySubDivisionName) = 'RO-B' and not(normalize-space(%[1]sram:CityName) = $SECTOR-RO-CODES)", addr)
}

// county is the report of BR-RO-110 and its siblings on the address at
// path: in Romania, and its subdivision is not a code of ISO 3166-2:RO.
func county(addr string) string {
	return fmt.Sprintf("normalize-space(%[1]sram:CountryID) = 'RO' and not(normalize-space(%[1]sram:CountrySubDivisionName) = $ISO-3166-RO-CODES)", addr)
}

// xpathStrings returns the XPath sequence of the strings ss.
func xpathStrings(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = "'" + s + "'"
	}
	return "(" + strings.Join(q, ", ") + ")"
}

func buildCIIPatterns() []pattern {
	// The UBL assertions, by identifier, whose flag and message the CII
	// ones take unless they give their own.
	ubl := map[string]assertion{}
	for _, r := range patterns[0].Rules {
		for _, a := range r.Asserts {
			ubl[a.ID] = a
		}
	}
	a := func(id, test string) assertion {
		u, ok := ubl[id]
		if !ok {
			panic("ro: " + id + " is not an assertion of the schematron")
		}
		return assertion{ID: id, Flag: u.Flag, Test: test, Msg: u.Msg, Values: u.Values}
	}
	report := func(id, test string) assertion {
		r := a(id, test)
		r.Report = true
		return r
	}
	date := func(id, bts string) assertion {
		r := a(id, ciiDate)
		r.Msg = "A date (" + bts + ") MUST be formatted YYYYMMDD. (%s = '%s')"
		r.Values = []string{"name(..)", "."}
		return r
	}
	seller, buyer, taxRep, shipTo := ciiSeller+"/", ciiBuyer+"/", ciiTaxRep+"/", ciiShipTo+"/"
	addr := "ram:PostalTradeAddress/"
	return []pattern{{
		ID: "ROmodel-CII",
		Lets: []let{
			{Name: "RO-CIUS-ID", Value: "'" + invoice.URNCIUSRO + "'"},
			{Name: "ISO-3166-RO-CODES", Value: xpathStrings(codes.RomanianCounties)},
			{Name: "SECTOR-RO-CODES", Value: xpathStrings(codes.BucharestSectors)},
		},
		Rules: []rule{
			{Context: "rsm:ExchangedDocument/ram:IssueDateTime/udt:DateTimeString | ram:InvoiceReferencedDocument/ram:FormattedIssueDateTime/qdt:DateTimeString",
				Asserts: []assertion{date("BR-RO-DT001", "BT-2, BT-26")}},
			{Context: "ram:ApplicableTradeTax/ram:TaxPointDate/udt:DateString",
				Asserts: []assertion{date("BR-RO-DT002", "BT-7")}},
			{Context: "ram:SpecifiedTradePaymentTerms/ram:DueDateDateTime/udt:DateTimeString",
				Asserts: []assertion{date("BR-RO-DT003", "BT-9")}},
			{Context: "ram:ActualDeliverySupplyChainEvent/ram:OccurrenceDateTime/udt:DateTimeString",
				Asserts: []assertion{date("BR-RO-DT004", "BT-72")}},
			{Context: "ram:BillingSpecifiedPeriod/ram:StartDateTime/udt:DateTimeString",
				Asserts: []assertion{date("BR-RO-DT005", "BT-73, BT-134")}},
			{Context: "ram:BillingSpecifiedPeriod/ram:EndDateTime/udt:DateTimeString",
				Asserts: []assertion{date("BR-RO-DT006", "BT-74, BT-135")}},
			{Context: "rsm:ExchangedDocument/ram:TypeCode", Asserts: []assertion{
				a("BR-RO-020_1", "not(contains(normalize-space(.), ' ')) and contains(' 380 384 389 751 381 ', concat(' ', normalize-space(.), ' '))"),
			}},
			{Context: "/rsm:CrossIndustryInvoice", Asserts: []assertion{
				a("BR-RO-001", "rsm:ExchangedDocumentContext/ram:GuidelineSpecifiedDocumentContextParameter/ram:ID = $RO-CIUS-ID"),
				a("BR-RO-A020", "count(rsm:ExchangedDocument/ram:IncludedNote) <= 20"),
				a("BR-RO-L0201", maxLen(seller+addr+"ram:PostcodeCode", 20)),
				a("BR-RO-L0202", maxLen(buyer+addr+"ram:PostcodeCode", 20)),
				a("BR-RO-L0203", maxLen(taxRep+addr+"ram:PostcodeCode", 20)),
				a("BR-RO-L0204", maxLen(shipTo+addr+"ram:PostcodeCode", 20)),
				a("BR-RO-L155", maxLen("rsm:ExchangedDocument/ram:ID", 200)),
				a("BR-RO-L0302", maxLen(ciiAgreement+"/ram:ContractReferencedDocument/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0303", maxLen(ciiAgreement+"/ram:BuyerOrderReferencedDocument/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0304", maxLen(ciiAgreement+"/ram:SellerOrderReferencedDocument/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0305", maxLen(ciiDelivery+"/ram:ReceivingAdviceReferencedDocument/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0306", maxLen(ciiDelivery+"/ram:DespatchAdviceReferencedDocument/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0307", maxLen(ciiAgreement+"/ram:AdditionalReferencedDocument[ram:TypeCode = '50']/ram:IssuerAssignedID", 200)),
				a("BR-RO-L0501", maxLen(seller+addr+"ram:CityName", 50)),
				a("BR-RO-L0502", maxLen(buyer+addr+"ram:CityName", 50)),
				a("BR-RO-L0503", maxLen(taxRep+addr+"ram:CityName", 50)),
				a("BR-RO-L0504", maxLen(shipTo+addr+"ram:CityName", 50)),
				a("BR-RO-L1001", maxLen(ciiSettle+"/ram:ReceivableSpecifiedTradeAccountingAccount/ram:ID", 100)),
				a("BR-RO-L1002", maxLen(seller+addr+"ram:LineTwo", 100)),
				a("BR-RO-L1003", maxLen(seller+addr+"ram:LineThree", 100)),
				a("BR-RO-L1004", maxLen(seller+"ram:DefinedTradeContact/ram:PersonName", 100)),
				a("BR-RO-L1005", maxLen(seller+"ram:DefinedTradeContact/ram:TelephoneUniversalCommunication/ram:CompleteNumber", 100)),
				a("BR-RO-L1006", maxLen(seller+"ram:DefinedTradeContact/ram:EmailURIUniversalCommunication/ram:URIID", 100)),
				a("BR-RO-L1007", maxLen(buyer+addr+"ram:LineTwo", 100)),
				a("BR-RO-L1008", maxLen(buyer+addr+"ram:LineThree", 100)),
				a("BR-RO-L1009", maxLen(buyer+"ram:DefinedTradeContact/ram:PersonName", 100)),
				a("BR-RO-L1010", maxLen(buyer+"ram:DefinedTradeContact/ram:TelephoneUniversalCommunication/ram:CompleteNumber", 100)),
				a("BR-RO-L1011", maxLen(buyer+"ram:DefinedTradeContact/ram:EmailURIUniversalCommunication/ram:URIID", 100)),
				a("BR-RO-L1012", maxLen(taxRep+addr+"ram:LineTwo", 100)),
				a("BR-RO-L1013", maxLen(taxRep+addr+"ram:LineThree", 100)),
				a("BR-RO-L1014", maxLen(shipTo+addr+"ram:LineTwo", 100)),
				a("BR-RO-L1015", maxLen(shipTo+addr+"ram:LineThree", 100)),
				a("BR-RO-L151", maxLen(seller+addr+"ram:LineOne", 150)),
				a("BR-RO-L152", maxLen(buyer+addr+"ram:LineOne", 150)),
				a("BR-RO-L153", maxLen(taxRep+addr+"ram:LineOne", 150)),
				a("BR-RO-L154", maxLen(shipTo+addr+"ram:LineOne", 150)),
				a("BR-RO-L201", maxLen(seller+"ram:Name", 200)),
				a("BR-RO-L202", maxLen(seller+"ram:SpecifiedLegalOrganization/ram:TradingBusinessName", 200)),
				a("BR-RO-L203", maxLen(buyer+"ram:Name", 200)),
				a("BR-RO-L204", maxLen(buyer+"ram:SpecifiedLegalOrganization/ram:TradingBusinessName", 200)),
				a("BR-RO-L205", maxLen(ciiSettle+"/ram:PayeeTradeParty/ram:Name", 200)),
				a("BR-RO-L206", maxLen(taxRep+"ram:Name", 200)),
				a("BR-RO-L207", maxLen(shipTo+"ram:Name", 200)),
				a("BR-RO-L301", maxLen(ciiSettle+"/ram:SpecifiedTradePaymentTerms/ram:Description", 300)),
				a("BR-RO-L1000", maxLen(seller+"ram:Description", 1000)),
				a("BR-RO-L140", maxLen(ciiSettle+"/ram:PaymentReference", 140)),
				a("BR-RO-010", "matches(normalize-space(rsm:ExchangedDocument/ram:ID), '([0-9])')"),
				a("BR-RO-030", "(normalize-space("+ciiSettle+"/ram:TaxCurrencyCode) = 'RON' and normalize-space("+ciiSettle+"/ram:InvoiceCurrencyCode) != 'RON') or "+
					"(normalize-space("+ciiSettle+"/ram:TaxCurrencyCode) = 'RON' and normalize-space("+ciiSettle+"/ram:InvoiceCurrencyCode) = 'RON') or "+
					"(normalize-space("+ciiSettle+"/ram:TaxCurrencyCode) != 'RON' and normalize-space("+ciiSettle+"/ram:InvoiceCurrencyCode) = 'RON') or "+
					"(not(exists("+ciiSettle+"/ram:TaxCurrencyCode)) and normalize-space("+ciiSettle+"/ram:InvoiceCurrencyCode) = 'RON')"),
				a("BR-RO-065", "not("+ciiVATCategories+") or ("+taxRep+"ram:SpecifiedTaxRegistration/ram:ID, "+nonBlank(seller+"ram:SpecifiedTaxRegistration/ram:ID")+")"),
				a("BR-RO-081", nonBlank(seller+addr+"ram:LineOne")),
				a("BR-RO-082", nonBlank(buyer+addr+"ram:LineOne")),
				a("BR-RO-091", nonBlank(seller+addr+"ram:CityName")),
				a("BR-RO-092", nonBlank(buyer+addr+"ram:CityName")),
				report("BR-RO-100", bucharest(seller+addr)),
				report("BR-RO-110", county(seller+addr)),
				report("BR-RO-101", bucharest(buyer+addr)),
				report("BR-RO-111", county(buyer+addr)),
				a("BR-RO-120", "not("+ciiVATCategories+") or ("+buyer+"ram:SpecifiedLegalOrganization/ram:ID, "+nonBlank(buyer+"ram:SpecifiedTaxRegistration/ram:ID")+")"),
			}},
			{Context: "/rsm:CrossIndustryInvoice/" + ciiTaxRep + "/ram:PostalTradeAddress", Asserts: []assertion{
				a("BR-RO-140", nonBlank("ram:LineOne")),
				a("BR-RO-150", nonBlank("ram:CityName")),
				report("BR-RO-160", bucharest("")),
				report("BR-RO-170", county("")),
			}},
			{Context: "ram:SpecifiedTradeSettlementPaymentMeans", Asserts: []assertion{
				a("BR-RO-L208", maxLen("ram:PayeePartyCreditorFinancialAccount/ram:AccountName", 200)),
				a("BR-RO-L209", maxLen("ram:ApplicableTradeSettlementFinancialCard/ram:CardholderName", 200)),
				a("BR-RO-L1016", maxLen("ram:Information", 100)),
			}},
			{Context: "/rsm:CrossIndustryInvoice/" + ciiShipTo + "/ram:PostalTradeAddress", Asserts: []assertion{
				a("BR-RO-180", nonBlank("ram:LineOne")),
				a("BR-RO-201", nonBlank("ram:CityName")),
				report("BR-RO-202", bucharest("")),
				a("BR-RO-211", nonBlank("ram:CountrySubDivisionName")),
				report("BR-RO-212", county("")),
			}},
			{Context: "ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax/ram:DueDateTypeCode", Asserts: []assertion{
				func() assertion {
					r := a("BR-RO-040", "not(contains(normalize-space(.), ' ')) and contains(' 5 29 72 ', concat(' ', normalize-space(.), ' '))")
					r.Msg = "Value added tax point date code MUST be coded using a restriction of UNTDID 2475 (only 5, 29 and 72)."
					return r
				}(),
			}},
			{Context: "ram:IncludedSupplyChainTradeLineItem", Asserts: []assertion{
				a("BR-RO-L1024", maxLen("ram:SpecifiedTradeProduct/ram:Name", 100)),
				a("BR-RO-L1021", maxLen("ram:SpecifiedLineTradeSettlement/ram:ReceivableSpecifiedTradeAccountingAccount/ram:ID", 100)),
				a("BR-RO-L212", maxLen("ram:SpecifiedTradeProduct/ram:Description", 200)),
				a("BR-RO-L303", maxLen("ram:AssociatedDocumentLineDocument/ram:IncludedNote/ram:Content", 300)),
			}},
			{Context: "ram:SpecifiedTradeProduct/ram:ApplicableProductCharacteristic", Asserts: []assertion{
				a("BR-RO-L0505", maxLen("ram:Description", 50)),
				a("BR-RO-L1025", maxLen("ram:Value", 100)),
			}},
			{Context: "ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = false()]", Asserts: []assertion{
				a("BR-DEC-RO-01", decimals("ram:ActualAmount")),
				a("BR-DEC-RO-02", decimals("ram:BasisAmount")),
				a("BR-RO-L1017", maxLen("ram:ReasonCode", 100)),
			}},
			{Context: "ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = true()]", Asserts: []assertion{
				a("BR-DEC-RO-05", decimals("ram:ActualAmount")),
				a("BR-DEC-RO-06", decimals("ram:BasisAmount")),
				a("BR-RO-L1018", maxLen("ram:ReasonCode", 100)),
			}},
			{Context: "ram:SpecifiedTradeSettlementHeaderMonetarySummation", Asserts: []assertion{
				a("BR-DEC-RO-09", decimals("ram:LineTotalAmount")),
				a("BR-DEC-RO-10", decimals("ram:AllowanceTotalAmount")),
				a("BR-DEC-RO-11", decimals("ram:ChargeTotalAmount")),
				a("BR-DEC-RO-12", decimals("ram:TaxBasisTotalAmount")),
				a("BR-DEC-RO-14", decimals("ram:GrandTotalAmount")),
				a("BR-DEC-RO-16", decimals("ram:TotalPrepaidAmount")),
				a("BR-DEC-RO-17", decimals("ram:RoundingAmount")),
				a("BR-DEC-RO-18", decimals("ram:DuePayableAmount")),
			}},
			{Context: "ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax", Asserts: []assertion{
				a("BR-DEC-RO-1009", decimals("ram:BasisAmount")),
				a("BR-DEC-RO-1010", decimals("ram:CalculatedAmount")),
			}},
			{Context: "ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = false()]", Asserts: []assertion{
				a("BR-DEC-RO-24", decimals("ram:ActualAmount")),
				a("BR-DEC-RO-25", decimals("ram:BasisAmount")),
				a("BR-RO-L1022", maxLen("ram:Reason", 100)),
			}},
			{Context: "ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeAllowanceCharge[ram:ChargeIndicator/udt:Indicator = true()]", Asserts: []assertion{
				a("BR-DEC-RO-27", decimals("ram:ActualAmount")),
				a("BR-DEC-RO-28", decimals("ram:BasisAmount")),
				a("BR-RO-L1023", maxLen("ram:Reason", 100)),
			}},
			{Context: "ram:IncludedNote/ram:Content | ram:SpecifiedTradePaymentTerms/ram:Description", Asserts: []assertion{
				a("BR-RO-L302", "string-length(normalize-space(.)) <= 300"),
			}},
			{Context: "ram:ApplicableHeaderTradeAgreement/ram:AdditionalReferencedDocument[not(ram:TypeCode = '50')]", Asserts: []assertion{
				a("BR-RO-L0308", maxLen("ram:IssuerAssignedID", 200)),
				a("BR-RO-L1020", maxLen("ram:Name", 100)),
				a("BR-RO-L210", maxLen("ram:URIID", 200)),
				a("BR-RO-L211", maxLen("ram:AttachmentBinaryObject/@filename", 200)),
			}},
			{Context: "ram:ApplicableHeaderTradeSettlement/ram:InvoiceReferencedDocument", Asserts: []assertion{
				a("BR-RO-L156", maxLen("ram:IssuerAssignedID", 200)),
			}},
		},
	}}
}
