package it

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Anchors of extBase.
const (
	extDiscount     = "<cac:AllowanceCharge>\n          <cbc:ChargeIndicator>false</cbc:ChargeIndicator>\n          <cbc:MultiplierFactorNumeric>2.00</cbc:MultiplierFactorNumeric>\n          <cbc:Amount currencyID=\"EUR\">19.96</cbc:Amount>\n        </cac:AllowanceCharge>"
	extWithholdings = "<cac:WithholdingTaxTotal>\n    <cbc:TaxAmount currencyID=\"EUR\">150.00</cbc:TaxAmount>\n"
	extSubtotal     = "    <cac:TaxSubtotal>\n      <cbc:TaxableAmount currencyID=\"EUR\">750.00</cbc:TaxableAmount>\n      <cbc:TaxAmount currencyID=\"EUR\">150.00</cbc:TaxAmount>\n      <cac:TaxCategory>\n        <cbc:ID>S</cbc:ID>\n        <cbc:Percent>20.00</cbc:Percent>\n        <cac:TaxScheme>\n          <cbc:ID>SWT</cbc:ID>\n          <cbc:TaxTypeCode>A</cbc:TaxTypeCode>\n        </cac:TaxScheme>\n      </cac:TaxCategory>\n    </cac:TaxSubtotal>\n"
	extWithholdURI  = "<ext:ExtensionURI>urn:fdc:agid.gov.it:fatturapa:TipoRitenuta</ext:ExtensionURI>"
	extWithholdExt  = "<ext:UBLExtension>\n      " + extWithholdURI + "\n      <ext:ExtensionContent>\n        <cbc:TypeCode>RT02</cbc:TypeCode>\n      </ext:ExtensionContent>\n    </ext:UBLExtension>"
	extAgentAddress = "<cac:PostalAddress>\n          <cbc:StreetName>Via Dante 7</cbc:StreetName>\n          <cbc:CityName>Milano</cbc:CityName>\n          <cbc:PostalZone>20121</cbc:PostalZone>\n          <cbc:CountrySubentity>MI</cbc:CountrySubentity>\n          <cac:Country>\n            <cbc:IdentificationCode>IT</cbc:IdentificationCode>\n          </cac:Country>\n        </cac:PostalAddress>"
	extSPName       = "<cac:PartyName>\n            <cbc:Name>Servizi Fatturazione S.p.A.</cbc:Name>\n          </cac:PartyName>"
	extSPScheme     = "<cbc:CompanyID>IT80012345676</cbc:CompanyID>\n            <cac:TaxScheme>\n              <cbc:ID>VAT</cbc:ID>"
	extSPEnd        = "</cac:PartyTaxScheme>\n        </cac:Party>\n      </cac:ServiceProviderParty>"
	extCF           = `<cbc:ID schemeID="0210">RSSMRA85T10A562S</cbc:ID>` // 1: third-party intermediary, 2: carrier
	extVAT          = "<cbc:CompanyID>IT80012345676</cbc:CompanyID>"      // 1: third-party intermediary, 2: tax representative, 3: carrier
	extCarrierVAT   = "        <cbc:CompanyID>IT80012345676</cbc:CompanyID>\n        <cac:TaxScheme>\n          <cbc:ID>VAT</cbc:ID>"
	extDespatchDoc  = "    <cbc:ID>DDT-2026/118</cbc:ID>\n    <cbc:IssueDate>2026-10-30</cbc:IssueDate>\n  </cac:DespatchDocumentReference>"
	extLineDespatch = "        <cbc:ID>DDT-2026/118</cbc:ID>\n        <cbc:IssueDate>2026-10-30</cbc:IssueDate>\n      </cac:DocumentReference>"
	extLineOrder    = "      <cac:OrderReference>\n        <cbc:ID>ORD-2026-17</cbc:ID>\n        <cbc:IssueDate>2026-10-01</cbc:IssueDate>\n      </cac:OrderReference>\n"
	extHeaderOrder  = "  <cac:OrderReference>\n    <cbc:ID>ORD-2026-17</cbc:ID>\n  </cac:OrderReference>\n"
	extSellerIT     = "<cbc:IdentificationCode>IT</cbc:IdentificationCode>" // 1: seller
	extSpec         = "<cbc:CustomizationID>" + spec + "</cbc:CustomizationID>"
)

// extCases lists, for each rule of appendix 3, mutations of extBase that
// break it, and some that keep it valid (no rule expected).
var extCases = []struct {
	name  string
	edits []edit
	want  []string // rules expected, sorted
}{
	{"BR-IT-DE-001", []edit{rep("<cbc:TypeCode>TZ</cbc:TypeCode>", "<cbc:TypeCode>XX</cbc:TypeCode>")}, []string{"BR-IT-DE-001"}},
	{"BR-IT-DE-001 missing", []edit{rep("<cbc:TypeCode>TZ</cbc:TypeCode>", "<cbc:Description>TZ</cbc:Description>")}, []string{"BR-IT-DE-001"}},
	{"BR-IT-DE-001 CC", []edit{rep("<cbc:TypeCode>TZ</cbc:TypeCode>", "<cbc:TypeCode>CC</cbc:TypeCode>")}, nil},
	{"BR-IT-DE-002", []edit{rep("<cbc:TypeCode>TD01</cbc:TypeCode>", "<cbc:TypeCode>TD07</cbc:TypeCode>")}, []string{"BR-IT-DE-002"}},
	{"BR-IT-DE-002 TD29", []edit{rep("<cbc:TypeCode>TD01</cbc:TypeCode>", "<cbc:TypeCode>TD29</cbc:TypeCode>")}, nil},
	{"BR-IT-DE-003", []edit{rep("<cbc:TypeCode>RT02</cbc:TypeCode>", "<cbc:TypeCode>RT07</cbc:TypeCode>")}, []string{"BR-IT-DE-003"}},
	{"BR-IT-DE-004", []edit{rep("<cbc:TypeCode>SI</cbc:TypeCode>", "<cbc:TypeCode>NO</cbc:TypeCode>")}, []string{"BR-IT-DE-004"}},
	{"BR-IT-DE-005", []edit{rep(`<cbc:Amount currencyID="EUR">0.00</cbc:Amount>`, `<cbc:Amount currencyID="EUR">1234567890123.00</cbc:Amount>`)}, []string{"BR-IT-DE-005"}},
	{"BR-IT-DE-005 missing", []edit{rep(`<cbc:Amount currencyID="EUR">0.00</cbc:Amount>`, `<cbc:Note>0.00</cbc:Note>`)}, []string{"BR-IT-DE-005"}},
	{"BR-IT-DE-006", []edit{rep(extDiscount, "<cbc:Note>Sconto</cbc:Note>")}, []string{"BR-IT-DE-006"}},
	{"BR-IT-DE-007", []edit{rep(">19.96<", ">19.123456789<")}, []string{"BR-IT-DE-007"}},
	{"BR-IT-DE-007 8 decimals", []edit{rep(">19.96<", ">1234567890123.1234567<")}, nil},
	{"BR-IT-DE-007 missing", []edit{rep("\n          <cbc:Amount currencyID=\"EUR\">19.96</cbc:Amount>", "")}, []string{"BR-IT-DE-007"}},
	{"BR-IT-DE-008", []edit{rep(">2.00</cbc:MultiplierFactorNumeric>", ">1000</cbc:MultiplierFactorNumeric>")}, []string{"BR-IT-DE-008"}},
	{"BR-IT-DE-008 100", []edit{rep(">2.00</cbc:MultiplierFactorNumeric>", ">100.00</cbc:MultiplierFactorNumeric>")}, nil},
	{"BR-IT-DE-009", []edit{rep(extWithholdings+extSubtotal+"  </cac:WithholdingTaxTotal>\n", "")}, []string{"BR-IT-DE-009"}},
	{"BR-IT-DE-010", []edit{rep(extWithholdings, "<cac:WithholdingTaxTotal>\n    <cbc:TaxAmount currencyID=\"EUR\">1234567890123.00</cbc:TaxAmount>\n")}, []string{"BR-IT-DE-010"}},
	{"BR-IT-DE-011 none", []edit{rep(extSubtotal, "")}, []string{"BR-IT-DE-011"}},
	{"BR-IT-DE-011 three", []edit{rep(extSubtotal, extSubtotal+extSubtotal+extSubtotal)}, []string{"BR-IT-DE-011", "BR-IT-DE-013"}},
	{"BR-IT-DE-012", []edit{repN(2, `<cbc:TaxAmount currencyID="EUR">150.00</cbc:TaxAmount>`, `<cbc:TaxAmount currencyID="EUR">150.125</cbc:TaxAmount>`)}, []string{"BR-IT-DE-012"}},
	{"BR-IT-DE-013 category", []edit{rep("<cbc:ID>S</cbc:ID>\n        <cbc:Percent>20.00</cbc:Percent>", "<cbc:ID>E</cbc:ID>\n        <cbc:Percent>20.00</cbc:Percent>")}, []string{"BR-IT-DE-013"}},
	{"BR-IT-DE-013 rate", []edit{rep("<cbc:Percent>20.00</cbc:Percent>\n        <cac:TaxScheme>\n          <cbc:ID>SWT", "<cbc:Percent>1000</cbc:Percent>\n        <cac:TaxScheme>\n          <cbc:ID>SWT")}, []string{"BR-IT-DE-013"}},
	{"BR-IT-DE-013 scheme", []edit{rep("<cbc:ID>SWT</cbc:ID>", "<cbc:ID>VAT</cbc:ID>")}, []string{"BR-IT-DE-013"}},
	{"BR-IT-DE-013 reason", []edit{rep("\n          <cbc:TaxTypeCode>A</cbc:TaxTypeCode>", "")}, []string{"BR-IT-DE-013"}},
	{"BR-IT-DE-013 second type", []edit{rep(extSubtotal, extSubtotal+extSubtotal)}, []string{"BR-IT-DE-013"}},
	{"BR-IT-DE-013 two types", []edit{
		rep(extSubtotal, extSubtotal+extSubtotal),
		rep(extWithholdExt, strings.Replace(extWithholdExt, "TipoRitenuta<", "TipoRitenuta::1<", 1)+"\n    "+strings.Replace(extWithholdExt, "TipoRitenuta<", "TipoRitenuta::2<", 1)),
	}, nil},
	{"BR-IT-DE-014", []edit{rep("\n      <cbc:IssueDate>2026-10-14</cbc:IssueDate>", "")}, []string{"BR-IT-DE-014"}},
	{"BR-IT-DE-015", []edit{rep("<cbc:ID>FT-2026/0030</cbc:ID>", "<cbc:ID>"+x(21)+"</cbc:ID>")}, []string{"BR-IT-DE-015"}},
	{"BR-IT-DE-016", []edit{rep(extDespatchDoc, "    <cbc:ID>DDT-2026/118</cbc:ID>\n  </cac:DespatchDocumentReference>")}, []string{"BR-IT-DE-016"}},
	{"BR-IT-DE-017", []edit{rep("<cbc:TaxLevelCode>RF01</cbc:TaxLevelCode>", "<cbc:TaxLevelCode>RF03</cbc:TaxLevelCode>")}, []string{"BR-IT-DE-017"}},
	{"BR-IT-DE-018", []edit{rep("\n          <cbc:CountrySubentity>MI</cbc:CountrySubentity>", "")}, []string{"BR-IT-DE-018"}},
	{"BR-IT-DE-018 no address", []edit{rep(extAgentAddress, "<cac:PartyName>\n          <cbc:Name>Sede di Milano</cbc:Name>\n        </cac:PartyName>")}, []string{"BR-IT-DE-018"}},
	{"BR-IT-DE-019", []edit{rep("Via Dante 7", x(61))}, []string{"BR-IT-DE-019"}},
	{"BR-IT-DE-020", []edit{rep("<cbc:CityName>Milano</cbc:CityName>", "<cbc:CityName>"+x(61)+"</cbc:CityName>")}, []string{"BR-IT-DE-020"}},
	{"BR-IT-DE-021", []edit{rep("<cbc:PostalZone>20121</cbc:PostalZone>", "<cbc:PostalZone>2012</cbc:PostalZone>")}, []string{"BR-IT-DE-021"}},
	{"BR-IT-DE-022", []edit{rep("<cbc:CountrySubentity>MI</cbc:CountrySubentity>", "<cbc:CountrySubentity>MIL</cbc:CountrySubentity>")}, []string{"BR-IT-DE-022"}},
	{"BR-IT-DE-023 VAT number", []edit{repN(1, extVAT, "")}, []string{"BR-IT-DE-023"}},
	{"BR-IT-DE-023 scheme", []edit{rep(extSPScheme, strings.Replace(extSPScheme, "VAT<", "GST<", 1))}, []string{"BR-IT-DE-023"}},
	{"BR-IT-DE-023 name", []edit{rep(extSPName, "")}, []string{"BR-IT-DE-023"}},
	{"BR-IT-DE-023 person", []edit{rep(extSPName, ""), rep(extSPEnd, "</cac:PartyTaxScheme>\n          <cac:Person>\n            <cbc:FirstName>Anna</cbc:FirstName>\n            <cbc:FamilyName>Bianchi</cbc:FamilyName>\n          </cac:Person>\n        </cac:Party>\n      </cac:ServiceProviderParty>")}, nil},
	{"BR-IT-DE-024", []edit{rep(`<cbc:ID schemeID="0210">80012345676</cbc:ID>`, `<cbc:ID schemeID="0210">8001234567</cbc:ID>`)}, []string{"BR-IT-DE-024"}},
	{"BR-IT-DE-025", []edit{rep("<cbc:ID>EORI:IT1234567890123</cbc:ID>", "<cbc:ID>IT1234567890123</cbc:ID>")}, []string{"BR-IT-DE-025"}},
	{"BR-IT-DE-025 length", []edit{rep("<cbc:ID>EORI:IT1234567890123</cbc:ID>", "<cbc:ID>EORI:IT12345</cbc:ID>")}, []string{"BR-IT-DE-025"}},
	{"BR-IT-DE-026", []edit{repN(2, extCF, `<cbc:ID schemeID="0210">RSSMRA85T10A562S1</cbc:ID>`)}, []string{"BR-IT-DE-026"}},
	{"BR-IT-DE-026 scheme", []edit{repN(2, extCF, `<cbc:ID>RSSMRA85T10A562S</cbc:ID>`)}, []string{"BR-IT-DE-026"}},
	{"BR-IT-DE-026 CF:", []edit{repN(2, extCF, `<cbc:ID>CF:RSSMRA85T10A562S</cbc:ID>`)}, nil},
	{"BR-IT-DE-027 name", []edit{rep("Trasporti Esempio S.n.c.", x(81))}, []string{"BR-IT-DE-027"}},
	{"BR-IT-DE-027 person", []edit{rep("<cbc:FamilyName>Rossi</cbc:FamilyName>", "<cbc:FamilyName>"+x(61)+"</cbc:FamilyName>")}, []string{"BR-IT-DE-027"}},
	{"BR-IT-DE-028", []edit{repN(3, extVAT, "<cbc:CompanyID>IT"+strings.Repeat("1", 29)+"</cbc:CompanyID>")}, []string{"BR-IT-DE-028"}},
	{"BR-IT-DE-029", []edit{rep(extCarrierVAT, strings.Replace(extCarrierVAT, "VAT<", "GST<", 1))}, []string{"BR-IT-DE-029"}},
	{"BR-IT-DE-030 type", []edit{rep("<cbc:DocumentTypeCode>40</cbc:DocumentTypeCode>", "<cbc:DocumentTypeCode>41</cbc:DocumentTypeCode>")}, []string{"BR-IT-DE-030"}},
	{"BR-IT-DE-030 length", []edit{rep("RM1234567X", x(21))}, []string{"BR-IT-DE-030"}},
	{"BR-IT-DE-031", []edit{rep(">12.50</cbc:GrossWeightMeasure>", ">12345.6</cbc:GrossWeightMeasure>")}, []string{"BR-IT-DE-031"}},
	{"BR-IT-DE-031 decimals", []edit{rep(">10.00</cbc:NetWeightMeasure>", ">10.125</cbc:NetWeightMeasure>")}, []string{"BR-IT-DE-031"}},
	{"BR-IT-DE-031 negative", []edit{rep(">10.00</cbc:NetWeightMeasure>", ">-10.00</cbc:NetWeightMeasure>")}, []string{"BR-IT-DE-031"}},
	{"BR-IT-DE-031 largest", []edit{rep(">12.50</cbc:GrossWeightMeasure>", ">9999.99</cbc:GrossWeightMeasure>")}, nil},
	{"BR-IT-DE-032", []edit{rep(">2</cbc:TotalTransportHandlingUnitQuantity>", ">10000</cbc:TotalTransportHandlingUnitQuantity>")}, []string{"BR-IT-DE-032"}},
	{"BR-IT-DE-032 zero", []edit{rep(">2</cbc:TotalTransportHandlingUnitQuantity>", ">0</cbc:TotalTransportHandlingUnitQuantity>")}, []string{"BR-IT-DE-032"}},
	{"BR-IT-DE-032 fraction", []edit{rep(">2</cbc:TotalTransportHandlingUnitQuantity>", ">2.5</cbc:TotalTransportHandlingUnitQuantity>")}, []string{"BR-IT-DE-032"}},
	{"BR-IT-DE-032 decimals", []edit{rep(">2</cbc:TotalTransportHandlingUnitQuantity>", ">9999.00</cbc:TotalTransportHandlingUnitQuantity>")}, nil},
	{"BR-IT-DE-033", []edit{rep("Switch di rete imballati", x(101))}, []string{"BR-IT-DE-033"}},
	{"BR-IT-DE-034", []edit{rep("Furgone", x(81))}, []string{"BR-IT-DE-034"}},
	{"BR-IT-DE-035", []edit{rep("<cbc:Instructions>Vendita</cbc:Instructions>", "<cbc:Instructions>"+x(101)+"</cbc:Instructions>")}, []string{"BR-IT-DE-035"}},
	{"BR-IT-DE-036", []edit{rep("<cbc:ID>DAP</cbc:ID>", "<cbc:ID>DA</cbc:ID>")}, []string{"BR-IT-DE-036"}},
	{"BR-IT-DE-037", []edit{rep(">10.00</cbc:SettlementDiscountAmount>", ">1234567890123.00</cbc:SettlementDiscountAmount>")}, []string{"BR-IT-DE-037"}},
	{"BR-IT-DE-038", []edit{rep(">25.00</cbc:PenaltyAmount>", ">25.125</cbc:PenaltyAmount>")}, []string{"BR-IT-DE-038"}},
	{"BR-IT-DE-039", []edit{rep(extLineOrder, strings.Replace(extLineOrder, "ORD-2026-17", x(21), 1))}, []string{"BR-IT-DE-039"}},
	{"BR-IT-DE-040", []edit{rep("<cbc:LineID>NA</cbc:LineID>", "<cbc:LineID>0</cbc:LineID>")}, []string{"BR-IT-DE-040"}},
	{"BR-IT-DE-041", []edit{rep(extLineDespatch, strings.Replace(extLineDespatch, "DDT-2026/118", x(21), 1))}, []string{"BR-IT-DE-041"}},
	{"BR-IT-DE-042", []edit{rep(extLineDespatch, "        <cbc:ID>DDT-2026/118</cbc:ID>\n      </cac:DocumentReference>")}, []string{"BR-IT-DE-042"}},
	{"BR-IT-DE-050", []edit{repN(1, extCF, `<cbc:ID>RSSMRA85T10A562S</cbc:ID>`)}, []string{"BR-IT-DE-050"}},
	{"BR-IT-DE-050 length", []edit{repN(1, extCF, `<cbc:ID schemeID="0210">RSSMRA85</cbc:ID>`)}, []string{"BR-IT-DE-050"}},
	{"BR-IT-DE-050 foreign", []edit{repN(1, extCF, `<cbc:ID>RSSMRA85T10A562S</cbc:ID>`), repN(1, extVAT, "<cbc:CompanyID>FR32123456789</cbc:CompanyID>")}, nil},
	{"BR-IT-DE-051", []edit{rep("Servizi Fatturazione S.p.A.", x(81))}, []string{"BR-IT-DE-051"}},
	{"BR-IT-DE-052", []edit{rep(extHeaderOrder, ""), rep(extLineOrder, "")}, []string{"BR-IT-DE-052"}},
	{"BR-IT-DE-052 order in the line", []edit{rep(extHeaderOrder, "")}, nil},
	{"cross-border", []edit{
		rep(extSpec, "<cbc:CustomizationID>"+invoice.URNCIUSIT+"</cbc:CustomizationID>"),
		repN(1, extSellerIT, "<cbc:IdentificationCode>FR</cbc:IdentificationCode>"),
		rep("IT12345678903", "FR32123456789"),
		rep("0612345678", "06"),
		rep("<cbc:TypeCode>TZ</cbc:TypeCode>", "<cbc:TypeCode>XX</cbc:TypeCode>"),
		rep(extHeaderOrder, ""), rep(extLineOrder, ""),
	}, nil},
}

func TestExtensions(t *testing.T) {
	base := load(t, extBase)
	for _, tc := range extCases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, []byte(mutate(t, base, tc.edits...)))
			if got := r.Rules(); !slices.Equal(got, tc.want) {
				t.Errorf("rules %v, want %v\n%s", got, tc.want, r)
			}
			for _, f := range r.Findings {
				if f.Source != rules.SourceIT || f.Severity != severities()[f.Rule] || !strings.HasPrefix(f.Message, "["+f.Rule+"] "+descriptions()[f.Rule]) ||
					f.Line == 0 || !strings.HasPrefix(f.Path, "/Invoice") {
					t.Errorf("finding %+v", f)
				}
			}
		})
	}
}

// TestExtensionsCreditNote checks the rules of the invoice lines on a
// credit note.
func TestExtensionsCreditNote(t *testing.T) {
	doc := mutate(t, load(t, extBase),
		rep("<Invoice xmlns=\"urn:oasis:names:specification:ubl:schema:xsd:Invoice-2\"", "<CreditNote xmlns=\"urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2\""),
		rep("</Invoice>", "</CreditNote>"),
		rep("<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:CreditNoteTypeCode>381</cbc:CreditNoteTypeCode>"),
		repAll("cac:InvoiceLine>", "cac:CreditNoteLine>"),
		repAll("cbc:InvoicedQuantity", "cbc:CreditedQuantity"),
		rep("<cbc:LineID>NA</cbc:LineID>", "<cbc:LineID>0</cbc:LineID>"),
		rep(extHeaderOrder, ""), rep(extLineOrder, ""),
	)
	r := run(t, []byte(doc))
	if got := r.Rules(); !slices.Equal(got, []string{"BR-IT-DE-040", "BR-IT-DE-052"}) {
		t.Errorf("rules %v\n%s", got, r)
	}
	for _, f := range r.Findings {
		if !strings.HasPrefix(f.Path, "/CreditNote/cac:CreditNoteLine[1]/") {
			t.Errorf("finding %+v", f)
		}
	}
}

func TestExtensionURI(t *testing.T) {
	root, err := xmltree.Parse([]byte(`<Invoice xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"><ext:UBLExtensions>
		<ext:UBLExtension><ext:ExtensionURI>urn:fdc:agid.gov.it:fatturapa:TipoRitenuta</ext:ExtensionURI></ext:UBLExtension>
		<ext:UBLExtension><ext:ExtensionURI> urn:fdc:agid.gov.it:fatturapa:TipoRitenuta::2 </ext:ExtensionURI></ext:UBLExtension>
		<ext:UBLExtension><ext:ExtensionURI>urn:fdc:agid.gov.it:fatturapa:RiepilogoIVA:Arrotondamento::x</ext:ExtensionURI></ext:UBLExtension>
		<ext:UBLExtension><ext:ExtensionURI>urn:fdc:agid.gov.it:fatturapa:TipoRitenuta::0</ext:ExtensionURI></ext:UBLExtension>
		<ext:UBLExtension><ext:ExtensionURI>urn:example:other</ext:ExtensionURI></ext:UBLExtension>
		<ext:UBLExtension/>
	</ext:UBLExtensions></Invoice>`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range extensions(root) {
		got = append(got, e.name+"/"+strconv.Itoa(e.index))
	}
	want := []string{"TipoRitenuta/0", "TipoRitenuta/2", "RiepilogoIVA:Arrotondamento/-1", "TipoRitenuta/-1"}
	if !slices.Equal(got, want) {
		t.Errorf("extensions: %q, want %q", got, want)
	}
}

func TestPackages(t *testing.T) {
	for v, want := range map[string]bool{
		"1": true, "9999": true, "0042": true, "12.0": true, "12.": true,
		"0": false, "10000": false, "12.5": false, "-1": false, "+1": false, "": false, "1e3": false, ".0": false,
	} {
		if got := packages(v); got != want {
			t.Errorf("packages(%q) = %v", v, got)
		}
	}
}

// TestCodeLists checks the code lists read from the FatturaPA schema.
func TestCodeLists(t *testing.T) {
	for name, list := range map[string][]string{"issuers": issuers, "document types": documentTypes, "withholding types": withholdingTypes, "tax regimes": taxRegimes} {
		if len(list) < 2 {
			t.Errorf("%s: %q", name, list)
		}
	}
	for _, td := range documentTypes {
		if invoice.TypeCodeOfDocumentType(td) == "" {
			t.Errorf("document type %s unknown to package invoice", td)
		}
	}
}
