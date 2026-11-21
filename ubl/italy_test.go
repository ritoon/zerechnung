package ubl

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
)

// extitFile is a domestic CIUS-IT invoice with the Italian extensions
// (EXT-IT): the seven UBL extensions and the UBL elements of appendix 4.2.
const extitFile = "../internal/rules/it/testdata/extit.ubl.xml"

// extitUnmodeled is what extitFile holds outside the mapping of appendix
// 4.2: the names of the person in the carrier party, which has a name of its
// own, the identifier of the shipment, which FatturaPA lacks, and the
// taxable amount of the withholding.
var extitUnmodeled = []string{
	"cac:Delivery/cac:CarrierParty/cac:Person/FirstName",
	"cac:Delivery/cac:CarrierParty/cac:Person/FamilyName",
	"cac:Delivery/cac:Shipment/ID",
	"cac:WithholdingTaxTotal/cac:TaxSubtotal/TaxableAmount",
}

// TestReadEXTIT reads the extensions and the elements of EXT-IT into the
// national blocks, as package fatturapa reads the FatturaPA data they carry.
func TestReadEXTIT(t *testing.T) {
	inv := mustParse(t, readFile(t, extitFile))
	d := invoice.MustDecimal
	cf := func(code string) invoice.Identifier { return invoice.Identifier{ID: code, Scheme: "0210"} }
	want := &invoice.Italy{
		IssuingSystem: "ZeRechnung",
		Intermediary:  &invoice.Party{Name: "Servizi Fatturazione S.p.A.", LegalRegistration: cf("RSSMRA85T10A562S"), VATID: "IT80012345676"},
		Issuer:        "TZ",
		// TipoDocumento TD01 is implied by the type code 380.
		Withholdings:       []invoice.Withholding{{Type: "RT02", Amount: d("150.00"), Rate: d("20.00"), Reason: "A"}},
		Discounts:          []invoice.Discount{{Type: "SC", Percentage: d("2.00"), Amount: d("19.96")}},
		Art73:              true,
		PurchaseOrders:     []invoice.RelatedDocument{{Lines: []string{"1"}, ID: "ORD-2026-17", Date: invoice.NewDate(2026, 10, 1), Item: "1"}},
		TransportDocuments: []invoice.TransportDocument{{Number: "DDT-2026/118", Date: invoice.NewDate(2026, 10, 30), Lines: []string{"1"}}},
		Transport: &invoice.Transport{
			Carrier:        &invoice.Party{Name: "Trasporti Esempio S.n.c.", LegalRegistration: cf("RSSMRA85T10A562S"), VATID: "IT80012345676"},
			CarrierLicence: "RM1234567X", Means: "Furgone", Reason: "Vendita", Packages: "2",
			Description: "Switch di rete imballati", WeightUnit: "KGM", GrossWeight: d("12.50"), NetWeight: d("10.00"),
			StartDate: invoice.NewDate(2026, 10, 30), DeliveryTerms: "DAP",
		},
		MainInvoice: &invoice.DocumentReference{ID: "FT-2026/0030", IssueDate: invoice.NewDate(2026, 10, 14)},
	}
	if got, w := jsonOf(t, inv.Italy), jsonOf(t, want); got != w {
		t.Errorf("Italy:\n%s\nwant\n%s", got, w)
	}
	establishment := &invoice.Address{Line1: "Via Dante 7", City: "Milano", PostCode: "20121", CountrySubdivision: "MI", CountryCode: "IT"}
	if got, w := jsonOf(t, inv.Seller.Italy), jsonOf(t, &invoice.ItalyParty{Establishment: establishment}); got != w {
		t.Errorf("Seller.Italy (RF01 implied):\n%s\nwant\n%s", got, w)
	}
	if rep := inv.SellerTaxRepresentative; rep.LegalRegistration != cf("80012345676") ||
		!slices.Equal(rep.IDs, []invoice.Identifier{{ID: "EORI:IT1234567890123"}}) {
		t.Errorf("tax representative: %+v %+v", rep.LegalRegistration, rep.IDs)
	}
	if vi := inv.VATBreakdown[0].Italy; vi == nil || vi.Rounding.String() != "0.00" {
		t.Errorf("VATBreakdown[0].Italy: %+v", vi)
	}
	if pit := inv.PaymentMeans[0].Italy; pit == nil || *pit != (invoice.ItalyPayment{EarlyDiscount: d("10.00"), LatePenalty: d("25.00")}) {
		t.Errorf("PaymentMeans[0].Italy: %+v", pit)
	}
	if inv.PurchaseOrderReference != "ORD-2026-17" || inv.DespatchAdviceReference != "DDT-2026/118" || inv.Lines[0].OrderLineReference != "1" ||
		len(inv.PrecedingInvoices) != 1 || inv.PrecedingInvoices[0].ID != "FT-2026/0031" {
		t.Errorf("EN 16931 references: %q %q %q %+v", inv.PurchaseOrderReference, inv.DespatchAdviceReference, inv.Lines[0].OrderLineReference, inv.PrecedingInvoices)
	}
	if got := inv.Unmodeled(); !slices.Equal(got, extitUnmodeled) {
		t.Errorf("unmodeled %q, want %q", got, extitUnmodeled)
	}

	// Without the Italian extension in the specification identifier, the
	// same elements are kept as extensions.
	plain := strings.Replace(string(readFile(t, extitFile)), invoice.ItalianExtension+"<", "<", 1)
	inv = mustParse(t, []byte(plain))
	if inv.Italy != nil || inv.Seller.Italy != nil || inv.VATBreakdown[0].Italy != nil || inv.PaymentMeans[0].Italy != nil ||
		len(inv.SellerTaxRepresentative.IDs) != 2 || inv.Lines[0].OrderLineReference != "1" {
		t.Errorf("without EXT-IT: national data read")
	}
	got := strings.Join(inv.Unmodeled(), " | ")
	for _, s := range []string{"UBLExtensions", "IssueDate", "AgentParty", "ServiceProviderParty", "CarrierParty", "Despatch", "Shipment", "DeliveryTerms", "SettlementDiscountAmount", "WithholdingTaxTotal", "DespatchLineReference"} {
		if !strings.Contains(got, s) {
			t.Errorf("without EXT-IT: %s not kept in %q", s, got)
		}
	}
}

// TestWriteEXTIT writes the national data back in the extensions and the
// elements of EXT-IT when the specification identifier written declares the
// extension, and reports it as lost otherwise.
func TestWriteEXTIT(t *testing.T) {
	inv := mustParse(t, readFile(t, extitFile))
	inv.Italy.DocumentType = "TD24" // more precise than the type code 380
	inv.Seller.Italy.TaxRegime = "RF19"

	_, err := Marshal(inv, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) {
		t.Fatalf("strict: got %v, want the extensions of the fixture", err)
	}
	var want []string
	for _, u := range extitUnmodeled {
		want = append(want, "EXT "+u)
	}
	if !slices.Equal(le.Terms, want) {
		t.Errorf("strict: lost %q, want %q", le.Terms, want)
	}

	out := mustMarshal(t, inv, nil)
	for _, s := range []string{
		` xmlns:ext="` + NamespaceEXT + `"><ext:UBLExtensions><ext:UBLExtension><ext:ExtensionURI>urn:fdc:agid.gov.it:fatturapa:SistemaEmittente</ext:ExtensionURI><ext:ExtensionContent><cbc:Description>ZeRechnung</cbc:Description></ext:ExtensionContent></ext:UBLExtension>`,
		`fatturapa:SoggettoEmittente</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>TZ</cbc:TypeCode>`,
		`fatturapa:TipoDocumento</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>TD24</cbc:TypeCode>`,
		`fatturapa:TipoRitenuta</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>RT02</cbc:TypeCode>`,
		`fatturapa:Art73</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>SI</cbc:TypeCode>`,
		`fatturapa:RiepilogoIVA:Arrotondamento::1</ext:ExtensionURI><ext:ExtensionContent><cbc:Amount currencyID="EUR">0.00</cbc:Amount>`,
		`fatturapa:ScontoMaggiorazione</ext:ExtensionURI><ext:ExtensionContent><cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:MultiplierFactorNumeric>2.00</cbc:MultiplierFactorNumeric><cbc:Amount currencyID="EUR">19.96</cbc:Amount></cac:AllowanceCharge>`,
		`<cac:OrderReference><cbc:ID>ORD-2026-17</cbc:ID></cac:OrderReference>`,
		`</cac:InvoiceDocumentReference><cac:AdditionalDocumentReference><cbc:ID>FT-2026/0030</cbc:ID><cbc:IssueDate>2026-10-14</cbc:IssueDate></cac:AdditionalDocumentReference></cac:BillingReference>`,
		`<cac:DespatchDocumentReference><cbc:ID>DDT-2026/118</cbc:ID><cbc:IssueDate>2026-10-30</cbc:IssueDate></cac:DespatchDocumentReference>`,
		`<cbc:CompanyID>IT12345678903</cbc:CompanyID><cbc:TaxLevelCode>RF19</cbc:TaxLevelCode>`,
		`<cac:AgentParty><cac:PostalAddress><cbc:StreetName>Via Dante 7</cbc:StreetName>`,
		`<cac:ServiceProviderParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="0210">RSSMRA85T10A562S</cbc:ID></cac:PartyIdentification><cac:PartyName><cbc:Name>Servizi Fatturazione S.p.A.</cbc:Name></cac:PartyName>`,
		`<cac:TaxRepresentativeParty><cac:PartyIdentification><cbc:ID schemeID="0210">80012345676</cbc:ID></cac:PartyIdentification><cac:PartyIdentification><cbc:ID>EORI:IT1234567890123</cbc:ID></cac:PartyIdentification>`,
		`<cac:Person><cac:IdentityDocumentReference><cbc:ID>RM1234567X</cbc:ID><cbc:DocumentTypeCode>40</cbc:DocumentTypeCode></cac:IdentityDocumentReference></cac:Person></cac:CarrierParty>`,
		`<cac:Despatch><cbc:ActualDespatchDate>2026-10-30</cbc:ActualDespatchDate><cbc:Instructions>Vendita</cbc:Instructions></cac:Despatch>`,
		`<cac:Shipment><cbc:ID>NA</cbc:ID><cbc:GrossWeightMeasure unitCode="KGM">12.50</cbc:GrossWeightMeasure><cbc:NetWeightMeasure unitCode="KGM">10.00</cbc:NetWeightMeasure><cbc:TotalTransportHandlingUnitQuantity>2</cbc:TotalTransportHandlingUnitQuantity>`,
		`</cac:Delivery><cac:DeliveryTerms><cbc:ID>DAP</cbc:ID></cac:DeliveryTerms>`,
		`<cbc:SettlementDiscountAmount currencyID="EUR">10.00</cbc:SettlementDiscountAmount><cbc:PenaltyAmount currencyID="EUR">25.00</cbc:PenaltyAmount>`,
		`</cac:TaxTotal><cac:WithholdingTaxTotal><cbc:TaxAmount currencyID="EUR">150.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxAmount currencyID="EUR">150.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20.00</cbc:Percent><cac:TaxScheme><cbc:ID>SWT</cbc:ID><cbc:TaxTypeCode>A</cbc:TaxTypeCode></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:WithholdingTaxTotal>`,
		`<cac:OrderLineReference><cbc:LineID>1</cbc:LineID><cac:OrderReference><cbc:ID>ORD-2026-17</cbc:ID><cbc:IssueDate>2026-10-01</cbc:IssueDate></cac:OrderReference></cac:OrderLineReference><cac:DespatchLineReference><cbc:LineID>NA</cbc:LineID><cac:DocumentReference><cbc:ID>DDT-2026/118</cbc:ID><cbc:IssueDate>2026-10-30</cbc:IssueDate></cac:DocumentReference></cac:DespatchLineReference>`,
	} {
		if !bytes.Contains(out, []byte(s)) {
			t.Errorf("want %s in\n%s", s, out)
		}
	}
	roundTrip(t, "EXT-IT", inv)

	// Under the CIUS-IT identifier of a cross-border invoice, nothing of
	// EXT-IT is written and the national data is lost.
	_, err = Marshal(inv, &MarshalOptions{CustomizationID: invoice.URNCIUSIT, Strict: true})
	if !errors.As(err, &le) {
		t.Fatalf("without EXT-IT: got %v", err)
	}
	got := strings.Join(le.Terms, " | ")
	for _, term := range inv.NationalData() {
		if !strings.Contains(got, term) {
			t.Errorf("without EXT-IT: %s not reported in %q", term, got)
		}
	}
	out = mustMarshal(t, inv, &MarshalOptions{CustomizationID: invoice.URNCIUSIT})
	for _, s := range []string{"ext:", "TaxLevelCode", "AgentParty", "ServiceProviderParty", "CarrierParty", "Despatch>", "DeliveryTerms", "SettlementDiscountAmount", "WithholdingTaxTotal", "DespatchLineReference", "<cbc:IssueDate>2026-10-01"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("without EXT-IT: %s written", s)
		}
	}
}

// TestEXTITLists writes several withholdings, discounts and transport
// documents: two withholdings per cac:WithholdingTaxTotal, one extension per
// type of withholding and per discount, with its ordinal after the first.
func TestEXTITLists(t *testing.T) {
	d := invoice.MustDecimal
	inv := mustParse(t, readFile(t, extitFile))
	it := inv.Italy
	it.Withholdings = append(it.Withholdings,
		invoice.Withholding{Type: "RT04", Amount: d("7.50"), Rate: d("1.00"), Reason: "ZO"},
		invoice.Withholding{Type: "RT01", Amount: d("2.00"), Rate: d("4.00"), Reason: "A"})
	it.Discounts = append(it.Discounts, invoice.Discount{Type: "MG", Amount: d("5.00")})
	it.Transport.PickupTime = "2026-10-29T09:15:00"
	it.TransportDocuments = append(it.TransportDocuments, invoice.TransportDocument{Number: "DDT-2026/119", Date: invoice.NewDate(2026, 10, 31), Lines: []string{"1", "2"}})
	it.PurchaseOrders = append(it.PurchaseOrders, invoice.RelatedDocument{Lines: []string{"2"}, ID: "ORD-2026-18", Date: invoice.NewDate(2026, 10, 2), Item: "4"})

	out := mustMarshal(t, inv, nil)
	for _, s := range []string{
		`fatturapa:TipoRitenuta::2</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>RT04</cbc:TypeCode>`,
		`fatturapa:TipoRitenuta::3</ext:ExtensionURI><ext:ExtensionContent><cbc:TypeCode>RT01</cbc:TypeCode>`,
		`fatturapa:ScontoMaggiorazione::2</ext:ExtensionURI><ext:ExtensionContent><cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">5.00</cbc:Amount>`,
		`<cac:WithholdingTaxTotal><cbc:TaxAmount currencyID="EUR">157.50</cbc:TaxAmount>`,
		`<cac:WithholdingTaxTotal><cbc:TaxAmount currencyID="EUR">2.00</cbc:TaxAmount>`,
		`<cac:Despatch><cbc:RequestedDespatchDate>2026-10-29</cbc:RequestedDespatchDate><cbc:RequestedDespatchTime>09:15:00</cbc:RequestedDespatchTime>`,
		`<cbc:ID>DDT-2026/118</cbc:ID><cbc:IssueDate>2026-10-30</cbc:IssueDate></cac:DocumentReference></cac:DespatchLineReference><cac:DespatchLineReference><cbc:LineID>NA</cbc:LineID><cac:DocumentReference><cbc:ID>DDT-2026/119</cbc:ID>`,
		`<cbc:ID>2</cbc:ID><cbc:InvoicedQuantity unitCode="C62">2</cbc:InvoicedQuantity><cbc:LineExtensionAmount currencyID="EUR">228.00</cbc:LineExtensionAmount><cac:OrderLineReference><cbc:LineID>4</cbc:LineID><cac:OrderReference><cbc:ID>ORD-2026-18</cbc:ID>`,
	} {
		if !bytes.Contains(out, []byte(s)) {
			t.Errorf("want %s in\n%s", s, out)
		}
	}
	back := mustParse(t, out)
	// The item of the order of line 2 is read as its order line reference
	// (BT-132), which carries it.
	inv.Lines[1].OrderLineReference = "4"
	sameInvoice(t, "lists", inv, back)

	// A pickup at midnight has no time; the date alone is read as midnight.
	inv = mustParse(t, readFile(t, extitFile))
	inv.Italy.Transport.PickupTime = "2026-10-29T00:00:00"
	if out := mustMarshal(t, inv, nil); bytes.Contains(out, []byte("RequestedDespatchTime")) || !bytes.Contains(out, []byte("<cbc:RequestedDespatchDate>2026-10-29<")) {
		t.Errorf("pickup at midnight:\n%s", out)
	}
	roundTrip(t, "midnight", inv)

	// A credit note has no withholding in UBL 2.1, hence no type of
	// withholding either.
	inv = mustParse(t, readFile(t, extitFile))
	inv.TypeCode = "381"
	_, err := Marshal(inv, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) || !slices.Contains(le.Terms, "EXT Italy.Withholdings") {
		t.Errorf("credit note: got %v", err)
	}
	if out := mustMarshal(t, inv, nil); bytes.Contains(out, []byte("TipoRitenuta")) || bytes.Contains(out, []byte("WithholdingTaxTotal")) {
		t.Errorf("credit note: withholding written\n%s", out)
	}

	// Data that UBL cannot carry is reported: the time of the delivery, a
	// transport document of the document beside the one of BT-16, an order
	// with a CIG, a carrier with an EORI code, a third discount type.
	inv = mustParse(t, readFile(t, extitFile))
	it = inv.Italy
	it.Transport.DeliveryTime = "2026-10-31T10:30:00"
	it.TransportDocuments = append(it.TransportDocuments, invoice.TransportDocument{Number: "DDT-2026/120", Date: invoice.NewDate(2026, 10, 31)})
	it.PurchaseOrders[0].CIG = "B12C345678"
	it.Intermediary.Address.City = "Roma"
	it.Discounts = append(it.Discounts, invoice.Discount{Type: "XX", Amount: d("1.00")})
	_, err = Marshal(inv, &MarshalOptions{Strict: true})
	if !errors.As(err, &le) {
		t.Fatalf("partial data: got %v", err)
	}
	for _, term := range []string{"EXT Italy.Transport", "EXT Italy.TransportDocuments", "EXT Italy.PurchaseOrders", "EXT Italy.Intermediary", "EXT Italy.Discounts"} {
		if !slices.Contains(le.Terms, term) {
			t.Errorf("partial data: %s not reported in %q", term, le.Terms)
		}
	}
}

// TestEXTITKept keeps the extensions that EXT-IT does not define, or whose
// value has no place in the model, at the place of ext:UBLExtensions.
func TestEXTITKept(t *testing.T) {
	doc := string(readFile(t, extitFile))
	other := `<ext:UBLExtension><ext:ExtensionURI>urn:example:other</ext:ExtensionURI><ext:ExtensionContent><cbc:Note>x</cbc:Note></ext:ExtensionContent></ext:UBLExtension>`
	art73 := "fatturapa:Art73</ext:ExtensionURI>\n      <ext:ExtensionContent>\n        <cbc:TypeCode>SI</cbc:TypeCode>"
	rounding := "RiepilogoIVA:Arrotondamento::1<"
	for _, s := range []string{art73, rounding} {
		if !strings.Contains(doc, s) {
			t.Fatalf("%q not found", s)
		}
	}
	doc = strings.NewReplacer(
		"<ext:UBLExtensions>", "<ext:UBLExtensions>"+other,
		art73, strings.Replace(art73, ">SI<", ">NO<", 1),
		rounding, "RiepilogoIVA:Arrotondamento::2<",
	).Replace(doc)
	inv := mustParse(t, []byte(doc))
	if inv.Italy.Art73 || inv.VATBreakdown[0].Italy != nil {
		t.Errorf("Art73 %v, VATBreakdown[0].Italy %+v", inv.Italy.Art73, inv.VATBreakdown[0].Italy)
	}
	got := inv.Unmodeled()
	if len(got) != 3+len(extitUnmodeled) || !slices.Equal(got[3:], extitUnmodeled) {
		t.Fatalf("unmodeled %q", got)
	}
	for i, s := range []string{"urn:example:other", "fatturapa:Art73", "Arrotondamento::2"} {
		if !strings.HasPrefix(got[i], "ext:UBLExtensions/UBLExtension") || !strings.Contains(inv.Extensions[i].XML, s) {
			t.Errorf("extension %d: %q, %s", i, got[i], inv.Extensions[i].XML)
		}
	}
}

// TestFatturaPAEXTIT converts the national data from FatturaPA to UBL and
// back: what the extensions of EXT-IT carry is written without loss, and
// the FatturaPA document written again is the same.
func TestFatturaPAEXTIT(t *testing.T) {
	first, err := fatturapa.Marshal(mustParse(t, readFile(t, extitFile)), &fatturapa.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := fatturapa.Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.NationalData()) < 10 {
		t.Fatalf("national data %q", inv.NationalData())
	}
	out, err := Marshal(inv, &MarshalOptions{Strict: true})
	if err != nil {
		t.Fatalf("FatturaPA to UBL: %v", err)
	}
	if !bytes.Contains(out, []byte(invoice.URNCIUSIT+invoice.ItalianExtension+"<")) {
		t.Errorf("not a domestic invoice:\n%s", out)
	}
	again, err := fatturapa.Marshal(mustParse(t, out), &fatturapa.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, again) {
		t.Errorf("FatturaPA written again:\n%s\nwant\n%s", again, first)
	}
}
