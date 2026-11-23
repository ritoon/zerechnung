package schema

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// checkUBLData parses a UBL document and runs Check on it.
func checkUBLData(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: &rules.Doc{UBL: root}, Report: r, Source: rules.SourceSyntax})
	return r
}

func TestUBLTestdata(t *testing.T) {
	for _, name := range []string{"EN16931_Einfach.ubl.xml", "XRECHNUNG_Einfach.ubl.xml", "ubl-tc434-creditnote1.xml"} {
		t.Run(name, func(t *testing.T) {
			r := checkUBLData(t, readFile(t, filepath.Join("..", "..", "testdata", name)))
			if len(r.Findings) > 0 {
				t.Errorf("unexpected findings:\n%s", r)
			}
		})
	}
}

// Fragments inserted by the UBL mutations.
const (
	ublExtStart = `<cec:UBLExtensions><cec:UBLExtension><cec:ExtensionContent>`
	ublExtEnd   = `</cec:ExtensionContent></cec:UBLExtension></cec:UBLExtensions>`
	ublForeign  = `<x:Signature xmlns:x="urn:example:x"><x:Any attr="1">anything <x:Deep/></x:Any></x:Signature>`
	ublFirst    = `<cbc:CustomizationID>`
	ublSubLEA   = `<cbc:LineExtensionAmount currencyID="EUR">1</cbc:LineExtensionAmount>`
	// ublHR is the Croatian extension HRFISK20Data (CIUS-HR), with every
	// element of its schema.
	ublHR = `<hrextac:HRFISK20Data xmlns:hrextac="urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1">` +
		`<hrextac:HRObracunPDVPoNaplati>Obračun PDV-a po naplaćenoj naknadi</hrextac:HRObracunPDVPoNaplati>` +
		`<hrextac:HRTaxTotal><cbc:TaxAmount currencyID="EUR">25.00</cbc:TaxAmount>` +
		`<hrextac:HRTaxSubtotal><cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">25.00</cbc:TaxAmount>` +
		`<hrextac:HRTaxCategory><cbc:ID>S</cbc:ID><cbc:Name>HR:PDV25</cbc:Name><cbc:Percent>25</cbc:Percent>` +
		`<cbc:TaxExemptionReasonCode>VATEX-EU-O</cbc:TaxExemptionReasonCode><cbc:TaxExemptionReason>a</cbc:TaxExemptionReason><cbc:TaxExemptionReason>b</cbc:TaxExemptionReason>` +
		`<hrextac:HRTaxScheme><cbc:ID>VAT</cbc:ID></hrextac:HRTaxScheme></hrextac:HRTaxCategory></hrextac:HRTaxSubtotal></hrextac:HRTaxTotal>` +
		`<hrextac:HRLegalMonetaryTotal><cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount>` +
		`<hrextac:OutOfScopeOfVATAmount currencyID="EUR">0.00</hrextac:OutOfScopeOfVATAmount></hrextac:HRLegalMonetaryTotal></hrextac:HRFISK20Data>`
)

// subLine returns a cac:SubInvoiceLine holding amount (its
// cbc:LineExtensionAmount, or nothing) and the sub-lines sub.
func subLine(amount, sub string) string {
	return `<cac:SubInvoiceLine><cbc:ID>1.1</cbc:ID>` + amount + `<cac:Item><cbc:Name>Part</cbc:Name></cac:Item>` + sub + `</cac:SubInvoiceLine>`
}

// ublMutations change a valid UBL invoice (base "ubl", the EN16931_Einfach
// invoice of testdata) or credit note (base "ubl-cn") in one place each.
var ublMutations = []mutation{
	// Order.
	{
		name: "ubl order", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>\n  <cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate>\n  <cbc:ID>471102</cbc:ID>",
		rules: []string{RuleOrder},
		msg:   `^cbc:ID is not allowed at this position in Invoice: it must come before cbc:IssueDate; expected here: .*cac:AccountingSupplierParty`,
	},
	{
		name: "ubl order after an aggregate", base: "ubl",
		old:   "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>\n  <cac:AccountingSupplierParty>",
		new:   "<cac:AccountingSupplierParty>",
		more:  []string{"</cac:AccountingSupplierParty>", "</cac:AccountingSupplierParty><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>"},
		rules: []string{RuleOrder},
		msg:   `^cbc:DocumentCurrencyCode is not allowed at this position in Invoice: it must come before cac:AccountingSupplierParty`,
	},
	// Cardinality.
	{
		name: "ubl missing", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   "",
		rules: []string{RuleMissing},
		msg:   `^cbc:ID is missing in Invoice \(expected before cbc:IssueDate\)$`,
	},
	{
		name: "ubl missing at the end", base: "ubl",
		old:   `<cbc:PayableAmount currencyID="EUR">529.87</cbc:PayableAmount>`,
		new:   "",
		rules: []string{RuleMissing},
		msg:   `^cbc:PayableAmount is missing in cac:LegalMonetaryTotal$`,
	},
	{
		name: "ubl repeated", base: "ubl",
		old:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate><cbc:IssueDate>2018-03-06</cbc:IssueDate>",
		rules: []string{RuleCount},
		msg:   `^cbc:IssueDate may occur only once in Invoice$`,
	},
	{
		name: "ubl repeated in an aggregate", base: "ubl",
		old:   "<cbc:Percent>7</cbc:Percent>",
		new:   "<cbc:Percent>7</cbc:Percent><cbc:Percent>7</cbc:Percent>",
		rules: []string{RuleCount},
		msg:   `^cbc:Percent may occur only once in cac:TaxCategory$`,
	},
	// Simple types.
	{
		name: "ubl date", base: "ubl",
		old:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new:   "<cbc:IssueDate>2018-02-30</cbc:IssueDate>",
		rules: []string{RuleType},
		msg:   `^cbc:IssueDate: value "2018-02-30" is not a valid xs:date$`,
	},
	{
		name: "ubl time", base: "ubl",
		old:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate><cbc:IssueTime>25:00:00</cbc:IssueTime>",
		rules: []string{RuleType},
		msg:   `^cbc:IssueTime: value "25:00:00" is not a valid xs:time$`,
	},
	{
		name: "ubl valid time", base: "ubl",
		old: "<cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new: "<cbc:IssueDate>2018-03-05</cbc:IssueDate><cbc:IssueTime>12:30:00.5+01:00</cbc:IssueTime>",
	},
	{
		name: "ubl decimal", base: "ubl",
		old:   `<cbc:PayableAmount currencyID="EUR">529.87</cbc:PayableAmount>`,
		new:   `<cbc:PayableAmount currencyID="EUR">529,87</cbc:PayableAmount>`,
		rules: []string{RuleType},
		msg:   `^cbc:PayableAmount: value "529,87" is not a valid xs:decimal$`,
	},
	{
		name: "ubl boolean", base: "ubl",
		old:   "</cac:PaymentTerms>",
		new:   `</cac:PaymentTerms><cac:AllowanceCharge><cbc:ChargeIndicator>yes</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">1</cbc:Amount></cac:AllowanceCharge>`,
		rules: []string{RuleType},
		msg:   `^cbc:ChargeIndicator: value "yes" is not a valid xs:boolean$`,
	},
	{
		name: "ubl valid boolean", base: "ubl",
		old: "</cac:PaymentTerms>",
		new: `</cac:PaymentTerms><cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">1</cbc:Amount></cac:AllowanceCharge>`,
	},
	{
		name: "ubl language", base: "ubl",
		old:   "<cbc:Note>Rechnung",
		new:   `<cbc:Note languageID="de_DE">Rechnung`,
		rules: []string{RuleType},
		msg:   `^attribute languageID of cbc:Note: value "de_DE" does not match the pattern of`,
	},
	{
		name: "ubl valid language", base: "ubl",
		old: "<cbc:Note>Rechnung",
		new: `<cbc:Note languageID="de-DE">Rechnung`,
	},
	// Attributes.
	{
		name: "ubl required attribute", base: "ubl",
		old:   `<cbc:PayableAmount currencyID="EUR">`,
		new:   `<cbc:PayableAmount>`,
		rules: []string{RuleAttributeMissing},
		msg:   `^attribute currencyID is required on cbc:PayableAmount$`,
	},
	{
		name: "ubl optional attribute", base: "ubl", // unitCode is optional on a quantity
		old: `<cbc:InvoicedQuantity unitCode="H87">20`,
		new: `<cbc:InvoicedQuantity>20`,
	},
	{
		name: "ubl undeclared attribute", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   `<cbc:ID foo="x">471102</cbc:ID>`,
		rules: []string{RuleAttribute},
		msg:   `^attribute foo is not allowed on cbc:ID \(allowed: schemeID, schemeName, schemeAgencyID, `,
	},
	{
		name: "ubl qualified attribute", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   `<cbc:ID cec:schemeID="x">471102</cbc:ID>`,
		rules: []string{RuleAttribute},
	},
	{
		name: "ubl xsi attribute", base: "ubl",
		old: "<cbc:ID>471102</cbc:ID>",
		new: `<cbc:ID xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="urn:x x.xsd">471102</cbc:ID>`,
	},
	// Namespaces and unexpected elements.
	{
		name: "ubl namespace", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   "<cac:ID>471102</cac:ID>",
		rules: []string{RuleNamespace},
		msg:   `^cac:ID is in namespace "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2", expected "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" \(cbc:ID\)$`,
	},
	{
		name: "ubl document namespace", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   "<ID>471102</ID>",
		rules: []string{RuleNamespace},
		msg:   `^ID is in namespace "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2", expected .* \(cbc:ID\)$`,
	},
	{
		name: "ubl unexpected", base: "ubl",
		old:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate>",
		new:   "<cbc:IssueDate>2018-03-05</cbc:IssueDate><cbc:Colour>red</cbc:Colour>",
		rules: []string{RuleUnexpected},
		msg:   `^cbc:Colour is not allowed in Invoice; expected here: cbc:IssueTime, cbc:DueDate, `,
	},
	{
		name: "ubl element in a value", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   "<cbc:ID><cbc:Name>471102</cbc:Name></cbc:ID>",
		rules: []string{RuleUnexpected},
		msg:   `^cbc:Name is not allowed: cbc:ID holds a value, not elements$`,
	},
	{
		name: "ubl text", base: "ubl",
		old:   "<cac:AccountingSupplierParty>",
		new:   "<cac:AccountingSupplierParty>Lieferant",
		rules: []string{RuleText},
		msg:   `^cac:AccountingSupplierParty holds elements only, text "Lieferant" is not allowed$`,
	},
	// Recursive types: a sub-line has the type of an invoice line.
	{
		name: "ubl sub-line", base: "ubl",
		old: "</cac:Price>",
		new: "</cac:Price>" + subLine(ublSubLEA, subLine(ublSubLEA, "")),
	},
	{
		name: "ubl nested sub-line", base: "ubl",
		old:   "</cac:Price>",
		new:   "</cac:Price>" + subLine(ublSubLEA, subLine("", "")),
		rules: []string{RuleMissing},
		msg:   `^cbc:LineExtensionAmount is missing in cac:SubInvoiceLine \(expected before cac:Item\)$`,
	},
	// Extensions: ext:ExtensionContent holds one element of another
	// namespace, whose content is not checked.
	{
		name: "ubl extension", base: "ubl",
		old: ublFirst,
		new: ublExtStart + ublForeign + ublExtEnd + ublFirst,
	},
	{
		name: "ubl extension in the document namespace", base: "ubl",
		old: ublFirst,
		new: ublExtStart + `<Anything><cbc:Colour/></Anything>` + ublExtEnd + ublFirst,
	},
	{
		name: "ubl extension namespace", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + `<cec:ExtensionAgencyID>x</cec:ExtensionAgencyID>` + ublExtEnd + ublFirst,
		rules: []string{RuleUnexpected},
		msg:   `^ext:ExtensionAgencyID is not allowed in ext:ExtensionContent; expected here: an element in a namespace other than "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"$`,
	},
	{
		name: "ubl extension without namespace", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + `<Data xmlns=""/>` + ublExtEnd + ublFirst,
		rules: []string{RuleUnexpected},
	},
	{
		name: "ubl empty extension", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublExtEnd + ublFirst,
		rules: []string{RuleMissing},
		msg:   `^an element in a namespace other than ".*CommonExtensionComponents-2" is missing in ext:ExtensionContent$`,
	},
	{
		name: "ubl two extension elements", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublForeign + ublForeign + ublExtEnd + ublFirst,
		rules: []string{RuleCount},
		msg:   `^x:Signature: ext:ExtensionContent holds only one element$`,
	},
	{
		name: "ubl extension text", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + "text" + ublForeign + ublExtEnd + ublFirst,
		rules: []string{RuleText},
	},
	{
		name: "ubl extension without content", base: "ubl",
		old:   ublFirst,
		new:   `<cec:UBLExtensions><cec:UBLExtension><cbc:ID>1</cbc:ID></cec:UBLExtension></cec:UBLExtensions>` + ublFirst,
		rules: []string{RuleMissing},
		msg:   `^ext:ExtensionContent is missing in ext:UBLExtension$`,
	},
	{
		name: "ubl extensions misplaced", base: "ubl",
		old:   "<cbc:ID>471102</cbc:ID>",
		new:   "<cbc:ID>471102</cbc:ID>" + ublExtStart + ublForeign + ublExtEnd,
		rules: []string{RuleOrder},
		msg:   `^ext:UBLExtensions is not allowed at this position in Invoice: it must come before cbc:CustomizationID`,
	},
	// The Croatian extension: validated against its schema (lax
	// validation of ext:ExtensionContent).
	{
		name: "ubl Croatian extension", base: "ubl",
		old: ublFirst,
		new: ublExtStart + ublHR + ublExtEnd + ublFirst,
	},
	{
		name: "ubl Croatian extension without tax scheme", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`<hrextac:HRTaxScheme><cbc:ID>VAT</cbc:ID></hrextac:HRTaxScheme>`, ""},
		rules: []string{RuleMissing},
		msg:   `^hrextac:HRTaxScheme is missing in hrextac:HRTaxCategory$`,
	},
	{
		name: "ubl Croatian extension unknown element", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`</hrextac:HRLegalMonetaryTotal>`, `</hrextac:HRLegalMonetaryTotal><hrextac:HRNapomena>x</hrextac:HRNapomena>`},
		rules: []string{RuleUnexpected},
		msg:   `^hrextac:HRNapomena is not allowed in hrextac:HRFISK20Data`,
	},
	{
		name: "ubl Croatian extension amount", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`currencyID="EUR">0.00</hrextac:OutOfScopeOfVATAmount>`, `currencyID="EUR">nula</hrextac:OutOfScopeOfVATAmount>`},
		rules: []string{RuleType},
		msg:   `^hrextac:OutOfScopeOfVATAmount: value "nula" is not a valid xs:decimal$`,
	},
	{
		name: "ubl Croatian extension currency", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`<cbc:TaxExclusiveAmount currencyID="EUR">`, `<cbc:TaxExclusiveAmount>`},
		rules: []string{RuleAttributeMissing},
	},
	{
		name: "ubl Croatian extension order", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`<cbc:Name>HR:PDV25</cbc:Name><cbc:Percent>25</cbc:Percent>`, `<cbc:Percent>25</cbc:Percent><cbc:Name>HR:PDV25</cbc:Name>`},
		rules: []string{RuleOrder},
	},
	{
		name: "ubl Croatian extension namespace", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + ublHR + ublExtEnd + ublFirst,
		more:  []string{`<hrextac:HRTaxTotal><cbc:TaxAmount currencyID="EUR">25.00</cbc:TaxAmount>`, `<hrextac:HRTaxTotal><hrextac:TaxAmount currencyID="EUR">25.00</hrextac:TaxAmount>`},
		rules: []string{RuleNamespace},
		msg:   `^hrextac:TaxAmount is in namespace "urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1", expected ".*CommonBasicComponents-2" \(cbc:TaxAmount\)$`,
	},
	{
		name: "ubl Croatian extension unknown root", base: "ubl",
		old:   ublFirst,
		new:   ublExtStart + `<hrextac:HRFISK21Data xmlns:hrextac="urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"/>` + ublExtEnd + ublFirst,
		rules: []string{RuleUnexpected},
		msg:   `^hrextac:HRFISK21Data is not allowed in ext:ExtensionContent; expected here: hrextac:HRFISK20Data$`,
	},
	// Credit note.
	{
		name: "ubl invoice line in a credit note", base: "ubl-cn",
		old:   "<cac:CreditNoteLine>",
		new:   "<cac:InvoiceLine>",
		more:  []string{"</cac:CreditNoteLine>", "</cac:InvoiceLine>"},
		rules: []string{RuleUnexpected, RuleMissing},
		msg:   `^cac:InvoiceLine is not allowed in CreditNote; expected here: cac:CreditNoteLine$`,
	},
	{
		name: "ubl invoiced quantity in a credit note line", base: "ubl-cn",
		old:   `<cbc:CreditedQuantity unitCode="C62">1.00</cbc:CreditedQuantity>`,
		new:   `<cbc:InvoicedQuantity unitCode="C62">1.00</cbc:InvoicedQuantity>`,
		rules: []string{RuleUnexpected},
		msg:   `^cbc:InvoicedQuantity is not allowed in cac:CreditNoteLine; expected here: `,
	},
	{
		name: "ubl credited quantity", base: "ubl-cn",
		old:   `<cbc:CreditedQuantity unitCode="C62">1.00</cbc:CreditedQuantity>`,
		new:   `<cbc:CreditedQuantity unitCode="C62">one</cbc:CreditedQuantity>`,
		rules: []string{RuleType},
		msg:   `^cbc:CreditedQuantity: value "one" is not a valid xs:decimal$`,
	},
	{
		name: "ubl credit note type code", base: "ubl-cn",
		old:   "<cbc:CreditNoteTypeCode>381</cbc:CreditNoteTypeCode>",
		new:   "<cbc:InvoiceTypeCode>381</cbc:InvoiceTypeCode>",
		rules: []string{RuleUnexpected},
	},
}

func TestUBLMutations(t *testing.T) {
	for _, m := range ublMutations {
		t.Run(m.name, func(t *testing.T) {
			r := checkUBLData(t, m.apply(t))
			var got []string
			for _, f := range r.Findings {
				got = append(got, f.Rule)
			}
			if strings.Join(got, " ") != strings.Join(m.rules, " ") {
				t.Fatalf("rules = %v, want %v\n%s", got, m.rules, r)
			}
			if m.msg != "" && !regexp.MustCompile(m.msg).MatchString(r.Findings[0].Message) {
				t.Errorf("message %q does not match %q", r.Findings[0].Message, m.msg)
			}
			for _, f := range r.Findings {
				if f.Path == "" || f.Line == 0 || f.Severity != rules.Error || f.Source != rules.SourceSyntax {
					t.Errorf("finding %+v", f)
				}
			}
		})
	}
}

func TestUBLFindingLocation(t *testing.T) {
	src := replace(t, readFile(t, filepath.Join("..", "..", "testdata", "EN16931_Einfach.ubl.xml")),
		"</cac:Price>", "</cac:Price>"+subLine(ublSubLEA, subLine("", "")))
	r := checkUBLData(t, src)
	if len(r.Findings) != 1 {
		t.Fatalf("findings:\n%s", r)
	}
	f := r.Findings[0]
	if f.Path != "/Invoice/cac:InvoiceLine[1]/cac:SubInvoiceLine/cac:SubInvoiceLine" || f.Line != 133 {
		t.Errorf("finding = %+v", f)
	}
}

func TestUBLRoot(t *testing.T) {
	for _, doc := range []string{
		`<Order xmlns="urn:oasis:names:specification:ubl:schema:xsd:Order-2"/>`,
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`,
	} {
		r := checkUBLData(t, []byte(doc))
		if got := strings.Join(r.Rules(), " "); got != RuleRoot {
			t.Errorf("%s: rules %s\n%s", doc, got, r)
		}
	}
}

// TestUBLTables checks invariants of the generated UBL tables.
func TestUBLTables(t *testing.T) {
	if UBLVersion() != "2.1" {
		t.Errorf("UBLVersion() = %q", UBLVersion())
	}
	for i := range ublTypes {
		ty := &ublTypes[i]
		if int(ty.first)+int(ty.n) > len(ublParticles) || int(ty.afirst)+int(ty.an) > len(ublAttributes) {
			t.Fatalf("type %d (%s) out of bounds", i, ty.name)
		}
		if ty.profiles != 0 || int(ty.facets) >= len(ublFacetTable) {
			t.Errorf("type %d (%s): profiles %#x, facets %d", i, ty.name, ty.profiles, ty.facets)
		}
		if ty.content == contentAny && (int(ty.other) >= len(ublNamespaces) || ublNamespaces[ty.other] != nsExt) {
			t.Errorf("type %s: wildcard excludes namespace %d", ty.name, ty.other)
		}
		seen := map[string]bool{}
		for _, p := range ublParticles[ty.first : ty.first+ty.n] {
			if seen[p.name] {
				t.Errorf("type %s: %s twice", ty.name, p.name)
			}
			seen[p.name] = true
			if int(p.typ) >= len(ublTypes) || int(p.ns) >= len(ublNamespaces) {
				t.Errorf("type %s: particle %s out of bounds", ty.name, p.name)
			}
		}
	}
	for _, a := range ublAttributes {
		if int(a.facets) >= len(ublFacetTable) {
			t.Errorf("attribute %s: facets %d", a.name, a.facets)
		}
	}
	// The two documents share the common types.
	inv, cn := &ublTypes[ublRoots[0].typ], &ublTypes[ublRoots[1].typ]
	find := func(ty *ctype, name string) *particle {
		for i := range ublParticles[ty.first : ty.first+ty.n] {
			if p := &ublParticles[int(ty.first)+i]; p.name == name {
				return p
			}
		}
		t.Fatalf("%s: no %s", ty.name, name)
		return nil
	}
	if find(inv, "AccountingSupplierParty").typ != find(cn, "AccountingSupplierParty").typ {
		t.Error("Invoice and CreditNote do not share cac:SupplierPartyType")
	}
	line := &ublTypes[find(inv, "InvoiceLine").typ]
	if find(line, "SubInvoiceLine").typ != find(inv, "InvoiceLine").typ {
		t.Error("cac:SubInvoiceLine does not have the type of cac:InvoiceLine")
	}
}

// TestUBLCroatia checks the CIUS-HR invoices of internal/rules/hr, which
// carry the Croatian extension, and the examples of the Croatian tax
// administration when FX_HR names them (the archive "Primjeri eRacuna.zip"
// of conformance/fetch.sh).
func TestUBLCroatia(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "rules", "hr", "testdata", "*.xml"))
	if len(files) < 3 {
		t.Fatalf("CIUS-HR invoices: %q", files)
	}
	ext := 0
	for _, f := range files {
		data := readFile(t, f)
		if strings.Contains(string(data), "<hrextac:HRFISK20Data>") {
			ext++
		}
		if r := checkUBLData(t, data); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", f, r)
		}
	}
	if ext < 2 {
		t.Errorf("%d CIUS-HR invoices with the Croatian extension, want 2", ext)
	}
	name := os.Getenv("FX_HR")
	if name == "" {
		return
	}
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		data, err := fs.ReadFile(zr, f.Name)
		if err != nil {
			t.Fatal(err)
		}
		n++
		if r := checkUBLData(t, data); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", f.Name, r)
		}
	}
	if n != 20 {
		t.Errorf("%d examples, want 20", n)
	}
}

// TestUBLCorpus checks that the UBL invoices of the corpus (FX_CORPUS,
// XML-Rechnung/UBL) and of the KoSIT test suite (FX_KOSIT) match the UBL
// 2.1 schema.
func TestUBLCorpus(t *testing.T) {
	var files []string
	if dir := os.Getenv("FX_CORPUS"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "XML-Rechnung", "UBL", "*.xml"))
		files = append(files, more...)
	}
	if dir := os.Getenv("FX_KOSIT"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "src", "test", "*", "*", "*_ubl.xml"))
		files = append(files, more...)
	}
	if len(files) == 0 {
		t.Skip("FX_CORPUS and FX_KOSIT not set or empty")
	}
	for _, f := range files {
		if r := checkUBLData(t, readFile(t, f)); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", f, r)
		}
	}
	t.Logf("%d UBL invoices", len(files))
}

func BenchmarkCheckUBL(b *testing.B) {
	root, err := xmltree.Parse(readFile(b, filepath.Join("..", "..", "testdata", "EN16931_Einfach.ubl.xml")))
	if err != nil {
		b.Fatal(err)
	}
	r := &rules.Report{}
	c := &rules.Checker{Doc: &rules.Doc{UBL: root}, Report: r, Source: rules.SourceSyntax}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Check(c)
	}
	if len(r.Findings) > 0 {
		b.Fatalf("findings:\n%s", r)
	}
}
