package ubl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/en16931"
	"github.com/ritoon/zerechnung/invoice"
)

// The EXTENDED-CTC-FR tests mutate tInvoice declared as an EXTENDED-CTC-FR
// invoice. Each case lists the exact set of rules it must report (errors and
// warnings), from package en16931 on the CII view and from this package; the
// verdicts are those of the XSLT of the FNFE-MPE schematron
// EXTENDED-CTC-FR-UBL.sch run by Saxon.

// tCTCFR is tInvoice declared as an EXTENDED-CTC-FR invoice.
var tCTCFR = strings.Replace(tInvoice, "<cbc:CustomizationID>urn:cen.eu:en16931:2017<", "<cbc:CustomizationID>"+invoice.URNExtendedCTCFR+"<", 1)

// Fragments of the EXTENDED-CTC-FR cases.
const (
	tLine1 = "<cac:InvoiceLine>\n<cbc:ID>1</cbc:ID>\n<!--line-->"
	tTotal = `<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
<cbc:TaxExclusiveAmount`
	tBasis   = `<cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>`
	tVATS    = `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`
	tLineEnd = "</cac:InvoiceLine>\n</Invoice>"
)

// subLine returns an invoice line of identifier id, of the subtype status
// (none when empty) under the parent line (none when empty), with the net
// amount (none when empty) and the VAT category S 19 %.
func subLine(id, parent, status, amount string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<cac:InvoiceLine>\n<cbc:ID>%s</cbc:ID>\n<cbc:InvoicedQuantity unitCode=\"C62\">1</cbc:InvoicedQuantity>\n", id)
	if amount != "" {
		fmt.Fprintf(&b, "<cbc:LineExtensionAmount currencyID=\"EUR\">%s</cbc:LineExtensionAmount>\n", amount)
	}
	if status != "" || parent != "" {
		b.WriteString("<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-1</cbc:ID>")
		if status != "" {
			fmt.Fprintf(&b, "<cbc:DocumentStatusCode>%s</cbc:DocumentStatusCode>", status)
		}
		b.WriteString("</cac:InvoiceDocumentReference>")
		if parent != "" {
			fmt.Fprintf(&b, "<cac:BillingReferenceLine><cbc:ID>%s</cbc:ID></cac:BillingReferenceLine>", parent)
		}
		b.WriteString("</cac:BillingReference>\n")
	}
	fmt.Fprintf(&b, "<cac:Item><cbc:Name>Part %s</cbc:Name>%s</cac:Item>\n<cac:Price><cbc:PriceAmount currencyID=\"EUR\">1.00</cbc:PriceAmount></cac:Price>\n</cac:InvoiceLine>\n", id, tVATS)
	return b.String()
}

// withLines appends lines after the line of tCTCFR.
func withLines(lines ...string) []string {
	return []string{tLineEnd, "</cac:InvoiceLine>\n" + strings.Join(lines, "") + "</Invoice>"}
}

// ctcfrCase is an EXTENDED-CTC-FR case: replacements (old, new pairs) in
// tCTCFR, optionally turned into a credit note.
type ctcfrCase struct {
	name   string
	repl   []string
	credit bool
	want   []string
}

func (tc ctcfrCase) doc(t testing.TB) string {
	if len(tc.repl)%2 != 0 {
		t.Fatalf("%s: odd replacements", tc.name)
	}
	doc := tCTCFR
	for i := 0; i < len(tc.repl); i += 2 {
		if !strings.Contains(doc, tc.repl[i]) {
			t.Fatalf("%s: %q not found", tc.name, tc.repl[i])
		}
		doc = strings.Replace(doc, tc.repl[i], tc.repl[i+1], 1)
	}
	if tc.credit {
		doc = creditNote(doc)
	}
	return doc
}

func ctcfrCases() []ctcfrCase {
	return append(ctcfrBaseCases(), categoryCases()...)
}

func ctcfrBaseCases() []ctcfrCase {
	group := []string{"<cbc:LineExtensionAmount currencyID=\"EUR\">100.00</cbc:LineExtensionAmount>\n<cac:Item>",
		"<cbc:LineExtensionAmount currencyID=\"EUR\">100.00</cbc:LineExtensionAmount>\n<cac:Item>"}
	return []ctcfrCase{
		{name: "valid"},
		{name: "valid credit note", credit: true},

		// Syntax rules.
		{name: "delivery terms", repl: []string{"<!--payment-->", `<cac:DeliveryTerms><cbc:ID>FCA</cbc:ID><cac:DeliveryLocation><cbc:Name>Paris</cbc:Name></cac:DeliveryLocation></cac:DeliveryTerms>`}},
		{name: "delivery terms special terms", repl: []string{"<!--payment-->", `<cac:DeliveryTerms><cbc:ID>FCA</cbc:ID><cbc:SpecialTerms>x</cbc:SpecialTerms></cac:DeliveryTerms>`}, want: []string{"UBL-FREXT-CR-410"}},
		{name: "delivery terms location identifier", repl: []string{"<!--payment-->", `<cac:DeliveryTerms><cbc:ID>FCA</cbc:ID><cac:DeliveryLocation><cbc:ID>L1</cbc:ID></cac:DeliveryLocation></cac:DeliveryTerms>`}, want: []string{"UBL-FREXT-CR-410"}},
		{name: "delivery terms code", repl: []string{"<!--payment-->", `<cac:DeliveryTerms><cbc:ID>XYZ</cbc:ID></cac:DeliveryTerms>`}, want: []string{"BR-FREXT-CL-27"}},
		{name: "line delivery terms", repl: []string{"<cac:Item><cbc:Name>Widget", `<cac:DeliveryTerms><cbc:ID>EXW</cbc:ID><cbc:SpecialTerms>x</cbc:SpecialTerms></cac:DeliveryTerms><cac:Item><cbc:Name>Widget`}, want: []string{"UBL-FREXT-CR-645"}},
		{name: "tax exchange rate", repl: []string{"<!--payment-->", `<cac:TaxExchangeRate><cbc:SourceCurrencyCode>EUR</cbc:SourceCurrencyCode><cbc:TargetCurrencyCode>USD</cbc:TargetCurrencyCode><cbc:CalculationRate>1.1</cbc:CalculationRate><cbc:Date>2024-01-15</cbc:Date></cac:TaxExchangeRate>`}},
		{name: "tax exchange rate operator", repl: []string{"<!--payment-->", `<cac:TaxExchangeRate><cbc:SourceCurrencyCode>EUR</cbc:SourceCurrencyCode><cbc:TargetCurrencyCode>USD</cbc:TargetCurrencyCode><cbc:CalculationRate>1.1</cbc:CalculationRate><cbc:MathematicOperatorCode>Multiply</cbc:MathematicOperatorCode></cac:TaxExchangeRate>`}, want: []string{"CII-FREXT-CR-490"}},
		{name: "deliver to party name", repl: []string{"<!--payment-->", `<cac:Delivery><cac:DeliveryParty><cac:PartyName><cbc:Name>A</cbc:Name></cac:PartyName></cac:DeliveryParty></cac:Delivery>`}},
		{name: "deliver to party identifier", repl: []string{"<!--payment-->", `<cac:Delivery><cac:DeliveryParty><cac:PartyIdentification><cbc:ID>D1</cbc:ID></cac:PartyIdentification></cac:DeliveryParty></cac:Delivery>`}, want: []string{"UBL-CR-392"}},
		{name: "two deliver to party names", repl: []string{"<!--payment-->", `<cac:Delivery><cac:DeliveryParty><cac:PartyName><cbc:Name>A</cbc:Name></cac:PartyName><cac:PartyName><cbc:Name>B</cbc:Name></cac:PartyName></cac:DeliveryParty></cac:Delivery>`}, want: []string{"UBL-SR-25"}},
		{name: "two line tax categories", repl: []string{"<!--item-->", tVATS}, want: []string{"UBL-FREXT-SR-48"}},
		{name: "two payment identifiers", repl: []string{"<!--payment-->", `<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cbc:PaymentID>A</cbc:PaymentID><cac:PayeeFinancialAccount><cbc:ID>FR7630006000011234567890189</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans><cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cbc:PaymentID>B</cbc:PaymentID><cac:PayeeFinancialAccount><cbc:ID>FR7630006000011234567890189</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`}, want: []string{"UBL-SR-44"}},
		{name: "preceding invoice without number", repl: []string{"<!--references-->", `<cac:BillingReference><cac:InvoiceDocumentReference><cbc:IssueDate>2023-12-01</cbc:IssueDate></cac:InvoiceDocumentReference></cac:BillingReference>`}, want: []string{"BR-55", "UBL-SR-07"}},
		{name: "preceding invoice", repl: []string{"<!--references-->", `<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-0</cbc:ID></cac:InvoiceDocumentReference></cac:BillingReference>`}},

		// Code lists.
		{name: "buyer identifier scheme BY", repl: []string{"<!--buyer-->", `<cac:PartyIdentification><cbc:ID schemeID="BY">B1</cbc:ID></cac:PartyIdentification>`}},
		{name: "buyer identifier scheme", repl: []string{"<!--buyer-->", `<cac:PartyIdentification><cbc:ID schemeID="XX">B1</cbc:ID></cac:PartyIdentification>`}, want: []string{"BR-FREXT-CL-10"}},
		{name: "buyer SEPA identifier", repl: []string{"<!--buyer-->", `<cac:PartyIdentification><cbc:ID schemeID="SEPA">B1</cbc:ID></cac:PartyIdentification>`}, want: []string{"BR-FREXT-CL-10"}},
		{name: "charge reason code", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode>XYZ</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}, want: []string{"BR-FREXT-CL-20"}},
		{name: "charge reason code of list 7161", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode listID="7161">ABK</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}},
		{name: "charge tax code", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode listID="5153">AAA</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}},
		{name: "charge tax code VAT", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode listID="5153">VAT</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}, want: []string{"BR-FREXT-CL-20-TAX"}},
		{name: "charge reason list", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode listID="1234">ABK</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}, want: []string{"BR-FREXT-CL-20-LISTID"}},
		{name: "charge reason empty list", repl: []string{"<cac:Item><cbc:Name>Widget", lineCharge(`<cbc:AllowanceChargeReasonCode listID="">XYZ</cbc:AllowanceChargeReasonCode>`) + "<cac:Item><cbc:Name>Widget"}},
		{name: "attachment XML", repl: []string{"<!--references-->", `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="application/xml" filename="d1.xml">AA==</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>`}},
		{name: "attachment text", repl: []string{"<!--references-->", `<cac:AdditionalDocumentReference><cbc:ID>D1</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="text/plain" filename="d1.txt">AA==</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>`}, want: []string{"BR-FREXT-CL-24"}},
		{name: "note subject of the CEN list", repl: []string{"<!--notes-->", "<cbc:Note>#BAT#Text</cbc:Note>"}, want: []string{"BR-CL-08"}},
		{name: "note subject", repl: []string{"<!--notes-->", "<cbc:Note>#AAI#Text</cbc:Note>"}},
		{name: "item attribute code", repl: []string{"<!--item-->", `<cac:AdditionalItemProperty><cbc:Name>Colour</cbc:Name><cbc:NameCode>XYZ</cbc:NameCode><cbc:Value>Red</cbc:Value></cac:AdditionalItemProperty>`}, want: []string{"BR-FREXT-04"}},
		{name: "exemption reason in lower case", repl: []string{"<cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory><!--item-->", "<cbc:Percent>19</cbc:Percent><cbc:TaxExemptionReasonCode>vatex-eu-79-c</cbc:TaxExemptionReasonCode><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory><!--item-->"}, want: []string{"BR-FREXT-S-08rev"}},
		{name: "VAT identifier prefix", repl: []string{"<cbc:CompanyID>DE123456789", "<cbc:CompanyID>QQ123456789"}, want: []string{"BR-CO-09"}},
		{name: "VAT identifier prefix EL", repl: []string{"<cbc:CompanyID>DE123456789", "<cbc:CompanyID>EL123456789"}},
		{name: "VAT identifier prefix of one character", repl: []string{"<cbc:CompanyID>DE123456789", "<cbc:CompanyID>1"}},

		// BR-CO-25.
		{name: "no due date", repl: []string{"<cbc:DueDate>2024-02-15</cbc:DueDate>\n", ""}, want: []string{"BR-CO-25"}},
		{name: "payment terms", repl: []string{"<cbc:DueDate>2024-02-15</cbc:DueDate>\n", "", "<!--payment-->", "<cac:PaymentTerms><cbc:Note>30 days</cbc:Note></cac:PaymentTerms>"}},
		{name: "credit note without due date", credit: true, repl: []string{"<!--payment-->", ""}},

		// Lines.
		{name: "line without quantity", repl: []string{`<cbc:InvoicedQuantity unitCode="C62">10</cbc:InvoicedQuantity>`, ""}, want: []string{"BR-FREXT-22", "BR-FREXT-23"}},
		{name: "line without unit", repl: []string{`<cbc:InvoicedQuantity unitCode="C62">10<`, `<cbc:InvoicedQuantity>10<`}, want: []string{"BR-FREXT-23"}},
		{name: "line without price", repl: []string{`<cac:Price><cbc:PriceAmount currencyID="EUR">10.00</cbc:PriceAmount></cac:Price>`, ""}, want: []string{"BR-FREXT-26"}},
		{name: "line without VAT category", repl: []string{"<cac:Item><cbc:Name>Widget</cbc:Name>" + tVATS, "<cac:Item><cbc:Name>Widget</cbc:Name>"}, want: []string{"BR-FREXT-CO-04", "BR-FREXT-S-01", "BR-FREXT-S-08", "BR-FREXT-S-08ini", "BR-FREXT-S-08rev", "UBL-FREXT-SR-48"}},
		{name: "item attribute without name", repl: []string{"<!--item-->", `<cac:AdditionalItemProperty><cbc:Value>Red</cbc:Value></cac:AdditionalItemProperty>`}, want: []string{"BR-FREXT-54-1"}},
		{name: "item attribute value and quantity", repl: []string{"<!--item-->", `<cac:AdditionalItemProperty><cbc:Name>Weight</cbc:Name><cbc:Value>2</cbc:Value><cbc:ValueQuantity unitCode="KGM">2</cbc:ValueQuantity></cac:AdditionalItemProperty>`}, want: []string{"BR-FREXT-54-2"}},
		{name: "item attribute quantity without unit", repl: []string{"<!--item-->", `<cac:AdditionalItemProperty><cbc:Name>Weight</cbc:Name><cbc:ValueQuantity>2</cbc:ValueQuantity></cac:AdditionalItemProperty>`}, want: []string{"BR-FREXT-54-2"}},
		{name: "item attribute quantity", repl: []string{"<!--item-->", `<cac:AdditionalItemProperty><cbc:Name>Weight</cbc:Name><cbc:ValueQuantity unitCode="KGM">2</cbc:ValueQuantity></cac:AdditionalItemProperty>`}},

		// Line subtypes.
		{name: "group with details", repl: append(group, withLines(subLine("2", "1", "DETAIL", "60.00"), subLine("3", "1", "DETAIL", "40.00"))...),
			want: []string{"BR-FREXT-CO-10", "BR-FREXT-CO-13", "BR-FREXT-S-08", "BR-FREXT-S-08ini", "BR-FREXT-S-08rev"}},
		{name: "group line with details", repl: append([]string{tLine1, tLine1 + "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-1</cbc:ID><cbc:DocumentStatusCode>GROUP</cbc:DocumentStatusCode></cac:InvoiceDocumentReference></cac:BillingReference>"},
			withLines(subLine("2", "1", "DETAIL", "60.00"), subLine("3", "1", "DETAIL", "40.00"))...)},
		{name: "group line with wrong sum", repl: append([]string{tLine1, tLine1 + "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-1</cbc:ID><cbc:DocumentStatusCode>GROUP</cbc:DocumentStatusCode></cac:InvoiceDocumentReference></cac:BillingReference>"},
			withLines(subLine("2", "1", "DETAIL", "60.00"), subLine("3", "1", "DETAIL", "40.01"))...),
			want: []string{"BR-FREXT-08"}},
		{name: "group line with group without amount", repl: append([]string{tLine1, tLine1 + "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-1</cbc:ID><cbc:DocumentStatusCode>GROUP</cbc:DocumentStatusCode></cac:InvoiceDocumentReference></cac:BillingReference>"},
			withLines(subLine("2", "1", "GROUP", ""), subLine("3", "2", "DETAIL", "100.00"))...),
			want: []string{"BR-FREXT-08", "BR-FREXT-12"}},
		{name: "information line", repl: withLines(subLine("2", "1", "INFORMATION", "5.00"))},
		{name: "line subtype", repl: withLines(subLine("2", "1", "OTHER", "5.00")), want: []string{"BR-FREXT-05"}},
		{name: "parent line without subtype", repl: withLines(subLine("2", "1", "", "5.00")), want: []string{"BR-FREXT-06"}},
		{name: "unknown parent line", repl: withLines(subLine("2", "9", "INFORMATION", "5.00")), want: []string{"BR-FREXT-11"}},
		{name: "own parent line", repl: withLines(subLine("2", "2", "INFORMATION", "5.00")), want: []string{"BR-FREXT-11"}},
		{name: "information line without quantity", repl: withLines(strings.Replace(subLine("2", "1", "INFORMATION", ""), `<cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>`, "", 1))},
		{name: "line referring to another invoice", repl: []string{tLine1, tLine1 + "<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>INV-0</cbc:ID></cac:InvoiceDocumentReference></cac:BillingReference>"}},

		// VAT breakdowns.
		{name: "taxable amount within tolerance in decimals", repl: []string{tBasis, `<cbc:TaxableAmount currencyID="EUR">100.01</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>`},
			want: []string{"BR-FREXT-S-08", "BR-FREXT-S-08ini", "BR-FREXT-S-08rev"}},
		{name: "taxable amount", repl: []string{tBasis, `<cbc:TaxableAmount currencyID="EUR">100.02</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>`},
			want: []string{"BR-FREXT-S-08", "BR-FREXT-S-08ini", "BR-FREXT-S-08rev"}},
		{name: "tax amount within tolerance", repl: []string{tBasis, `<cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.01</cbc:TaxAmount>`, `<cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxSubtotal>`, `<cbc:TaxAmount currencyID="EUR">19.01</cbc:TaxAmount>
<cac:TaxSubtotal>`, "119.00<", "119.01<", "119.00<", "119.01<"}},
		{name: "tax amount", repl: []string{tBasis, `<cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.02</cbc:TaxAmount>`, `<cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxSubtotal>`, `<cbc:TaxAmount currencyID="EUR">19.02</cbc:TaxAmount>
<cac:TaxSubtotal>`, "119.00<", "119.02<", "119.00<", "119.02<"}, want: []string{"BR-FREXT-S-09"}},
		{name: "exemption reason of the breakdown only", repl: []string{"<cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>", "<cbc:Percent>19</cbc:Percent><cbc:TaxExemptionReason>Reason</cbc:TaxExemptionReason><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>"},
			want: []string{"BR-FREXT-S-08rev"}},
		{name: "rate of the breakdown", repl: []string{"<cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>", "<cbc:Percent>19.00</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>"}},
		{name: "breakdown without lines", repl: []string{"</cac:TaxTotal>", `<cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>G</cbc:ID><cbc:Percent>0</cbc:Percent><cbc:TaxExemptionReasonCode>VATEX-EU-G</cbc:TaxExemptionReasonCode><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>`},
			want: []string{"BR-FREXT-G-01"}},
		{name: "tax amount of category L within tolerance", repl: category("L", "7", "", "", "100.00", "7.99")},
		{name: "tax amount of category L", repl: category("L", "7", "", "", "100.00", "8.00"), want: []string{"BR-AF-09"}},
		{name: "exemption reason of category L", repl: category("L", "7", "", "<cbc:TaxExemptionReason>Reason</cbc:TaxExemptionReason>", "100.00", "7.00"), want: []string{"BR-AF-10", "BR-FREXT-AF-08rev"}},
		{name: "line without net amount", repl: []string{"<cbc:LineExtensionAmount currencyID=\"EUR\">100.00</cbc:LineExtensionAmount>\n<cac:Item>", "<cac:Item>"},
			want: []string{"BR-FREXT-24", "BR-FREXT-CO-10", "BR-FREXT-CO-13", "BR-FREXT-S-08", "BR-FREXT-S-08ini", "BR-FREXT-S-08rev"}},

		// Totals.
		{name: "line total within tolerance", repl: []string{tTotal, `<cbc:LineExtensionAmount currencyID="EUR">100.01</cbc:LineExtensionAmount>
<cbc:TaxExclusiveAmount`}},
		{name: "line total", repl: []string{tTotal, `<cbc:LineExtensionAmount currencyID="EUR">100.02</cbc:LineExtensionAmount>
<cbc:TaxExclusiveAmount`}, want: []string{"BR-FREXT-CO-10"}},
		{name: "allowance total without allowances", repl: []string{"<cbc:TaxExclusiveAmount", `<cbc:AllowanceTotalAmount currencyID="EUR">0.00</cbc:AllowanceTotalAmount><cbc:TaxExclusiveAmount`}},
		{name: "allowance total", repl: []string{"<cbc:TaxExclusiveAmount", `<cbc:AllowanceTotalAmount currencyID="EUR">5.00</cbc:AllowanceTotalAmount><cbc:TaxExclusiveAmount`}, want: []string{"BR-FREXT-CO-11"}},
		{name: "charge total", repl: []string{"<cbc:TaxExclusiveAmount", `<cbc:ChargeTotalAmount currencyID="EUR">5.00</cbc:ChargeTotalAmount><cbc:TaxExclusiveAmount`}, want: []string{"BR-FREXT-CO-12"}},
		{name: "total without VAT", repl: []string{`<cbc:TaxExclusiveAmount currencyID="EUR">100.00<`, `<cbc:TaxExclusiveAmount currencyID="EUR">100.02<`}, want: []string{"BR-FREXT-CO-13", "BR-FREXT-CO-15"}},
		{name: "total with VAT", repl: []string{`<cbc:TaxInclusiveAmount currencyID="EUR">119.00<`, `<cbc:TaxInclusiveAmount currencyID="EUR">119.02<`}, want: []string{"BR-CO-16", "BR-FREXT-CO-15"}},
		{name: "document allowance", repl: []string{"<!--payment-->", `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>95</cbc:AllowanceChargeReasonCode><cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>`,
			tBasis, `<cbc:TaxableAmount currencyID="EUR">90.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">17.10</cbc:TaxAmount>`, `<cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxSubtotal>`, `<cbc:TaxAmount currencyID="EUR">17.10</cbc:TaxAmount>
<cac:TaxSubtotal>`, "<cbc:TaxExclusiveAmount currencyID=\"EUR\">100.00<", "<cbc:AllowanceTotalAmount currencyID=\"EUR\">10.00</cbc:AllowanceTotalAmount><cbc:TaxExclusiveAmount currencyID=\"EUR\">90.00<",
			"119.00<", "107.10<", "119.00<", "107.10<"}},
	}
}

// category returns the replacements that turn tCTCFR into an invoice of
// VAT category code at the rate percent (none when empty), whose line and
// breakdown carry the exemption reasons lineReason and breakdownReason
// (elements), of taxable amount basis and tax amount tax.
func category(code, percent, lineReason, breakdownReason, basis, tax string) []string {
	return []string{tVATS, "<cac:ClassifiedTaxCategory>" + taxCategory(code, percent, lineReason) + `<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`,
		tBasis, `<cbc:TaxableAmount currencyID="EUR">` + basis + `</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">` + tax + `</cbc:TaxAmount>`,
		`<cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
<cac:TaxSubtotal>`, `<cbc:TaxAmount currencyID="EUR">` + tax + `</cbc:TaxAmount>
<cac:TaxSubtotal>`,
		"<cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent>", taxCategory(code, percent, breakdownReason),
		"119.00<", total(tax), "119.00<", total(tax)}
}

// taxCategory returns the elements of a VAT category before cac:TaxScheme.
func taxCategory(code, percent, reason string) string {
	c := "<cbc:ID>" + code + "</cbc:ID>"
	if percent != "" {
		c += "<cbc:Percent>" + percent + "</cbc:Percent>"
	}
	return c + reason
}

// reason returns the element of the VAT exemption reason code c.
func reason(c string) string {
	return "<cbc:TaxExemptionReasonCode>" + c + "</cbc:TaxExemptionReasonCode>"
}

// buyerVAT and intraDelivery complete the invoices of categories AE and K.
var (
	buyerVAT      = []string{"</cac:PostalAddress>\n<cac:PartyLegalEntity><cbc:RegistrationName>Buyer", "</cac:PostalAddress>\n<cac:PartyTaxScheme><cbc:CompanyID>FR12345678901</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>\n<cac:PartyLegalEntity><cbc:RegistrationName>Buyer"}
	intraDelivery = []string{"<!--payment-->", `<cac:Delivery><cbc:ActualDeliveryDate>2024-01-10</cbc:ActualDeliveryDate><cac:DeliveryLocation><cac:Address><cac:Country><cbc:IdentificationCode>FR</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>`}
)

// categoryCases returns, for each VAT category but S, a valid invoice of
// that category, an invoice whose taxable amount is wrong (BR-FREXT-*-08),
// for the categories with exemption reasons an invoice whose line has no
// exemption reason (BR-FREXT-*-08rev), and an invoice that fails
// BR-FREXT-*-01: a breakdown of the category without lines or, for AE and M,
// a line of the category whose breakdown is of category Z. (Package en16931
// reports BR-AE-01 for an AE breakdown without lines and not BR-IP-01 for an
// M breakdown without lines, unlike the UBL schematrons.)
func categoryCases() []ctcfrCase {
	type spec struct {
		code, id, percent, reason, tax string
		extra                          []string
		lineOnly                       []string // the rules of a line without breakdown
	}
	specs := []spec{
		{"Z", "Z", "0", "", "0.00", nil, nil},
		{"E", "E", "0", "VATEX-EU-132", "0.00", nil, nil},
		{"AE", "AE", "0", "VATEX-EU-AE", "0.00", buyerVAT, []string{"BR-AE-01", "BR-FREXT-Z-01"}},
		{"K", "IC", "0", "VATEX-EU-IC", "0.00", slices.Concat(buyerVAT, intraDelivery), nil},
		{"G", "G", "0", "VATEX-EU-G", "0.00", nil, nil},
		{"O", "O", "", "VATEX-EU-O", "0.00", nil, nil},
		{"L", "AF", "7", "", "7.00", nil, nil},
		{"M", "AG", "7", "", "7.00", nil, []string{"BR-AG-01", "BR-FREXT-Z-01"}},
	}
	var cases []ctcfrCase
	for _, c := range specs {
		r := ""
		if c.reason != "" {
			r = reason(c.reason)
		}
		cases = append(cases,
			ctcfrCase{name: "category " + c.code, repl: slices.Concat(category(c.code, c.percent, r, r, "100.00", c.tax), c.extra)},
			ctcfrCase{name: "category " + c.code + " taxable amount", repl: slices.Concat(category(c.code, c.percent, r, r, "100.02", c.tax), c.extra),
				want: []string{"BR-FREXT-" + c.id + "-08", "BR-FREXT-" + c.id + "-08ini", "BR-FREXT-" + c.id + "-08rev"}})
		if c.lineOnly != nil {
			repl := category(c.code, c.percent, r, "", "0.00", "0.00")
			repl[7] = taxCategory("Z", "0", "")
			cases = append(cases, ctcfrCase{name: "category " + c.code + " line without breakdown", repl: slices.Concat(repl, c.extra),
				want: slices.Concat([]string{"BR-FREXT-" + c.id + "-01"}, c.lineOnly)})
		} else {
			cases = append(cases, ctcfrCase{name: "category " + c.code + " breakdown without lines", repl: slices.Concat([]string{"</cac:TaxTotal>", `<cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">0.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">0.00</cbc:TaxAmount><cac:TaxCategory>` +
				taxCategory(c.code, c.percent, r) + `<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>`}, c.extra),
				want: []string{"BR-FREXT-" + c.id + "-01"}})
		}
		if c.reason != "" {
			cases = append(cases, ctcfrCase{name: "category " + c.code + " exemption reason of the breakdown only", repl: slices.Concat(category(c.code, c.percent, "", r, "100.00", c.tax), c.extra),
				want: []string{"BR-FREXT-" + c.id + "-08rev"}})
		}
	}
	return cases
}

// total returns 100 + tax followed by "<".
func total(tax string) string {
	var a, b int
	fmt.Sscanf(tax, "%d.%d", &a, &b)
	return fmt.Sprintf("%d.%02d<", 100+a, b)
}

// lineCharge returns a line level charge of zero with the reason code rc.
func lineCharge(rc string) string {
	return `<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator>` + rc + `<cbc:Amount currencyID="EUR">0.00</cbc:Amount></cac:AllowanceCharge>`
}

// ctcfrRules returns the sorted rules reported for doc, errors and warnings.
func ctcfrRules(t *testing.T, doc string) ([]string, *rules.Report) {
	t.Helper()
	r := check(t, doc)
	var got []string
	for _, f := range r.Findings {
		if f.Severity == rules.Error || f.Severity == rules.Warning {
			got = append(got, f.Rule)
		}
	}
	sort.Strings(got)
	return got, r
}

func TestCTCFRRules(t *testing.T) {
	for _, tc := range ctcfrCases() {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.doc(t)
			got, r := ctcfrRules(t, doc)
			want := slices.Clone(tc.want)
			sort.Strings(want)
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("rules %v, want %v\n%s", got, want, r)
			}
			for _, f := range r.Findings {
				if f.Message == "" || f.Path == "" {
					t.Errorf("incomplete finding %+v", f)
				}
			}
		})
	}
}

// TestCTCFRCatalog checks that each rule of the EXTENDED-CTC-FR schematron
// for UBL is implemented: a CEN rule of package en16931 or of this package
// (package en16931 reports the CEN rules BR-IG-nn and BR-IP-nn as BR-AF-nn
// and BR-AG-nn, and checks BR-IG-09 and BR-IP-09 in UBL only), a rule of the
// schematron for CII, a rule that the source of this package names, or one
// of the assertions true() BR-CO-05 to BR-CO-08.
func TestCTCFRCatalog(t *testing.T) {
	known := map[string]bool{
		"BR-CO-05": true, "BR-CO-06": true, "BR-CO-07": true, "BR-CO-08": true,
		"BR-AF-09": true, "BR-AG-09": true,
	}
	for _, c := range [][]rules.RuleInfo{Catalog, en16931.Catalog, en16931.CTCFRCatalog} {
		for _, r := range c {
			known[r.ID] = true
		}
	}
	for _, r := range en16931.Catalog {
		if n, ok := strings.CutPrefix(r.ID, "BR-IG-"); ok {
			known["BR-AF-"+n] = true
		}
		if n, ok := strings.CutPrefix(r.ID, "BR-IP-"); ok {
			known["BR-AG-"+n] = true
		}
	}
	for _, cr := range ctcfrCRRules {
		known[cr.id] = true
	}
	for id := range ctcfrLists {
		known[id] = true
	}
	for _, c := range ctcfrBreakdowns {
		for _, s := range []string{"-01", "-08", "-08ini", "-08rev"} {
			known["BR-FREXT-"+c.id+s] = true
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`"((?:BR|UBL|CII)-[A-Z0-9-]+[a-z]*)"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, "_gen.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			known[m[1]] = true
		}
	}
	seen := map[string]bool{}
	for _, r := range CTCFRCatalog {
		if seen[r.ID] {
			t.Errorf("duplicate %s", r.ID)
		}
		seen[r.ID] = true
		if r.Description == "" {
			t.Errorf("%s: no description", r.ID)
		}
		if !known[r.ID] {
			t.Errorf("%s: not implemented", r.ID)
		}
	}
	if n := len(CTCFRCatalog); n != 953 {
		t.Errorf("len(CTCFRCatalog) = %d, want 953", n)
	}
	if n := len(ctcfrCRRules); n != 631 {
		t.Errorf("len(ctcfrCRRules) = %d, want 631", n)
	}
}

// TestCTCFRFiles checks that the EXTENDED-CTC-FR examples in UBL of the
// France_RFE package (FX_FNFE) report no error and no warning.
func TestCTCFRFiles(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST", "UBL", "EXTENDED-CTC-FR", "*.xml"))
	if len(files) != 5 {
		t.Fatalf("%d examples, want 5", len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if got, r := ctcfrRules(t, string(data)); len(got) > 0 {
			t.Errorf("%s: %v\n%s", filepath.Base(f), got, r)
		}
	}
}
