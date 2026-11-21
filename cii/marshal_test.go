package cii

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

func mustMarshal(t *testing.T, inv *invoice.Invoice, opts *MarshalOptions) []byte {
	t.Helper()
	out, err := Marshal(inv, opts)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return out
}

func mustParse(t *testing.T, data []byte) *invoice.Invoice {
	t.Helper()
	inv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, data)
	}
	return inv
}

// TestMarshalProfiles writes the full invoice in every profile and checks
// that the output is well-formed, follows the profile XSD, reads back as the
// same profile, and is a fixed point (writing what was read gives the same
// bytes).
func TestMarshalProfiles(t *testing.T) {
	for _, p := range invoice.Profiles {
		for _, indent := range []bool{false, true} {
			t.Run(p.String()+map[bool]string{false: "", true: "/indent"}[indent], func(t *testing.T) {
				inv := fullInvoice()
				opts := &MarshalOptions{Profile: p, Indent: indent}
				out := mustMarshal(t, inv, opts)
				if _, err := xmltree.Parse(out); err != nil {
					t.Fatalf("not well-formed: %v", err)
				}
				checkXSD(t, p, out)
				back := mustParse(t, out)
				if back.Profile != p {
					t.Errorf("profile read back = %v, want %v", back.Profile, p)
				}
				again := mustMarshal(t, back, opts)
				if !bytes.Equal(out, again) {
					t.Errorf("second write differs:\n%s\n---\n%s", out, again)
				}
				if indent {
					flat := mustMarshal(t, inv, &MarshalOptions{Profile: p})
					if !reflect.DeepEqual(mustParse(t, flat), back) {
						t.Error("indented and flat outputs read back differently")
					}
				}
			})
		}
	}
}

// TestMarshalExtendedLossless checks that every field of the model survives
// an EXTENDED round trip.
func TestMarshalExtendedLossless(t *testing.T) {
	for _, p := range []invoice.Profile{invoice.ProfileExtended, invoice.ProfileExtendedCTCFR} {
		inv := fullInvoice()
		inv.Profile = p
		inv.SpecificationID = p.URN()
		out, err := Marshal(inv, &MarshalOptions{Strict: true})
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		back := mustParse(t, out)
		if !reflect.DeepEqual(inv, back) {
			t.Errorf("%v: round trip differs\n%s", p, diffInvoices(inv, back))
		}
	}
}

// TestMarshalEN16931Content checks the EN 16931 projection of the full
// invoice: everything but the EXTENDED fields and the extensions.
func TestMarshalEN16931Content(t *testing.T) {
	inv := fullInvoice()
	back := mustParse(t, mustMarshal(t, inv, &MarshalOptions{Profile: invoice.ProfileEN16931}))
	want := fullInvoice()
	want.Profile = invoice.ProfileEN16931
	want.SpecificationID = invoice.URNEN16931
	want.TestIndicator, want.CopyIndicator = false, false
	want.Language = ""
	want.ProductEndUser, want.UltimateShipTo, want.ShipFrom = nil, nil, nil
	want.DeliveryTerms, want.DeliveryNote, want.CurrencyExchange = nil, nil, nil
	want.UltimateCustomerOrders, want.LogisticsCharges = nil, nil
	want.Extensions, want.Seller.Extensions = nil, nil
	want.PaymentTerms[0].Extensions, want.VATBreakdown[0].Extensions = nil, nil
	want.Notes[0].ContentCode = ""
	want.SalesAgent, want.BuyerAgent, want.Invoicer, want.Invoicee, want.Payer = nil, nil, nil, nil, nil
	want.InvoicingPeriod.Description = ""
	want.PaymentTerms = want.PaymentTerms[:1]
	want.PaymentTerms[0].PartialAmount = invoice.Decimal{}
	want.PrecedingInvoices[0].TypeCode, want.PrecedingInvoices[0].LineID = "", ""
	want.AdditionalDocuments[0].IssueDate = invoice.Date{}
	want.Buyer.Description = ""
	want.Buyer.TaxRegistrationID = ""
	want.PaymentMeans[3].PayerAccountName, want.PaymentMeans[3].PayerBIC = "", ""
	want.ThirdPartyCharges = nil
	trimParty := func(p *invoice.Party, keep func(*invoice.Party) invoice.Party) { *p = keep(p) }
	trimParty(want.SellerTaxRepresentative, func(p *invoice.Party) invoice.Party {
		return invoice.Party{Name: p.Name, Address: p.Address, VATID: p.VATID}
	})
	trimParty(want.Payee, func(p *invoice.Party) invoice.Party {
		return invoice.Party{IDs: p.IDs, Name: p.Name, LegalRegistration: p.LegalRegistration}
	})
	trimParty(want.ShipTo, func(p *invoice.Party) invoice.Party {
		return invoice.Party{IDs: p.IDs, Name: p.Name, Address: p.Address}
	})
	l := &want.Lines[0]
	l.Extensions, l.Item.Attributes[0].Extensions = nil, nil
	l.Item.ReferencedProducts, l.PriceAllowancesCharges = nil, nil
	l.DeliveryTerms, l.UltimateCustomerOrders, l.UltimateShipTo, l.DeliveryNote = nil, nil, nil, nil
	l.Item.Attributes[0].TypeCode = ""
	l.Item.Classifications[0].Name = ""
	l.OrderReference = ""
	l.ShipTo = nil
	l.DeliveryDate = invoice.Date{}
	l.Period.Description = ""
	l.AllowancesCharges[0].VATCategoryCode, l.AllowancesCharges[0].VATRate = "", invoice.Decimal{}
	l.InvoiceReference = nil
	l2 := &want.Lines[1]
	l2.ParentID, l2.StatusCode, l2.StatusReasonCode = "", "", ""
	l2.Extensions = nil
	if !reflect.DeepEqual(want, back) {
		t.Errorf("EN 16931 projection differs\n%s", diffInvoices(want, back))
	}
}

// TestMarshalStrict checks the business terms reported as lost.
func TestMarshalStrict(t *testing.T) {
	tests := []struct {
		profile invoice.Profile
		want    []string // must be reported
		notWant []string // must not be reported
	}{
		{invoice.ProfileExtended, nil, nil},
		{invoice.ProfileEN16931,
			[]string{"EXT TestIndicator", "EXT Language", "EXT Note.ContentCode", "EXT SalesAgent", "EXT Payer", "BT-215", "BT-216", "BG-34",
				"EXT Line.ParentID", "EXT Line.OrderReference", "EXT Line.ShipTo", "EXT PaymentTerms[2:]",
				"EXT Payee.Address", "EXT ShipTo.Contact", "EXT Buyer.Description", "EXT AdditionalDocument.IssueDate",
				"EXT CopyIndicator", "EXT ProductEndUser", "EXT DeliveryTerms", "EXT UltimateCustomerOrders",
				"EXT UltimateShipTo", "EXT ShipFrom", "EXT DeliveryNote", "EXT CurrencyExchange", "EXT LogisticsCharges",
				"EXT PrecedingInvoice.LineID", "EXT Line.DeliveryTerms", "EXT Line.UltimateCustomerOrders",
				"EXT Line.UltimateShipTo", "EXT Line.DeliveryNote", "EXT Line.PriceAllowancesCharges", "EXT Item.ReferencedProducts",
				// Extensions, with their location from the root.
				"EXT ExchangedDocument/Name", "EXT ExchangedDocument/ID/@schemeID",
				"EXT SupplyChainTradeTransaction/ApplicableHeaderTradeAgreement/QuotationReferencedDocument",
				"EXT SupplyChainTradeTransaction/ApplicableHeaderTradeAgreement/SellerTradeParty/RoleCode",
				"EXT SupplyChainTradeTransaction/ApplicableHeaderTradeAgreement/SellerTradeParty/PostalTradeAddress/PostcodeCode/@listID",
				"EXT SupplyChainTradeTransaction/IncludedSupplyChainTradeLineItem/SpecifiedTradeProduct/BrandName",
				"EXT SupplyChainTradeTransaction/IncludedSupplyChainTradeLineItem/SpecifiedLineTradeDelivery/ChargeFreeQuantity",
				"EXT SupplyChainTradeTransaction/ApplicableHeaderTradeSettlement/ApplicableTradeTax/LineTotalBasisAmount",
				"EXT SupplyChainTradeTransaction/ApplicableHeaderTradeSettlement/SpecifiedTradePaymentTerms/ApplicableTradePaymentDiscountTerms"},
			[]string{"BG-25", "BT-33", "BT-132", "BG-24"}},
		{invoice.ProfileXRechnung, []string{"EXT SalesAgent"}, []string{"BG-25", "BG-3"}},
		{invoice.ProfileBasic,
			[]string{"BT-33", "BG-6", "BT-11", "BT-14", "BT-15", "BG-24", "BT-17", "BT-18", "BT-82", "BT-85", "BT-86",
				"BG-18", "BT-114", "BT-7", "BT-132", "BT-155", "BT-154", "BG-32", "BT-158", "BT-159", "BT-128", "BT-133",
				"BT-137", "BT-138", "BT-45"},
			[]string{"BG-25", "BT-8", "BG-3", "BT-29"}},
		{invoice.ProfileBasicWL,
			[]string{"BG-25", "BT-33", "BT-7"},
			[]string{"BT-8", "BG-23", "BG-16", "BG-20", "BG-10", "BG-11", "BG-13"}},
		{invoice.ProfileMinimum,
			[]string{"BG-25", "BG-1", "BG-23", "BG-16", "BG-20", "BG-21", "BG-10", "BG-11", "BG-13", "BT-29",
				"BT-46", "BG-8", "BT-48", "BT-106", "BT-6", "BT-111", "BT-113", "BG-5", "BT-72", "BT-12", "BT-16", "BG-3", "BT-19"},
			[]string{"BT-13", "BT-10", "BT-31", "BT-32", "BT-30", "BT-47"}},
	}
	for _, tt := range tests {
		t.Run(tt.profile.String(), func(t *testing.T) {
			_, err := Marshal(fullInvoice(), &MarshalOptions{Profile: tt.profile, Strict: true})
			if tt.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var le *LossError
			if !errors.As(err, &le) {
				t.Fatalf("error = %v, want *LossError", err)
			}
			if le.Profile != tt.profile {
				t.Errorf("LossError.Profile = %v", le.Profile)
			}
			got := map[string]bool{}
			for _, term := range le.Terms {
				if got[term] {
					t.Errorf("term %s reported twice", term)
				}
				got[term] = true
			}
			for _, w := range tt.want {
				if !got[w] {
					t.Errorf("missing %q in %v", w, le.Terms)
				}
			}
			for _, w := range tt.notWant {
				if got[w] {
					t.Errorf("unexpected %q in %v", w, le.Terms)
				}
			}
			// Without Strict the same invoice is written.
			if _, err := Marshal(fullInvoice(), &MarshalOptions{Profile: tt.profile}); err != nil {
				t.Errorf("non-strict Marshal: %v", err)
			}
		})
	}
}

// TestMarshalPrecedingInvoices checks that XRECHNUNG keeps a single
// preceding invoice reference (BG-3), as the CII D16B schema of XRechnung
// allows, and EN 16931 all of them.
func TestMarshalPrecedingInvoices(t *testing.T) {
	inv := fullInvoice()
	inv.PrecedingInvoices = append(inv.PrecedingInvoices, invoice.DocumentReference{ID: "F-2026-001", IssueDate: dt("2026-10-15")})
	for _, tc := range []struct {
		profile invoice.Profile
		want    []string
	}{
		{invoice.ProfileXRechnung, []string{"F-2026-000"}},
		{invoice.ProfileEN16931, []string{"F-2026-000", "F-2026-001"}},
	} {
		back := mustParse(t, mustMarshal(t, inv, &MarshalOptions{Profile: tc.profile}))
		var got []string
		for _, r := range back.PrecedingInvoices {
			got = append(got, r.ID)
		}
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%s: BG-3 = %v, want %v", tc.profile, got, tc.want)
		}
	}
	_, err := Marshal(inv, &MarshalOptions{Profile: invoice.ProfileXRechnung, Strict: true})
	var le *LossError
	if !errors.As(err, &le) || !slices.Contains(le.Terms, "BG-3") {
		t.Errorf("strict XRECHNUNG: error = %v, want BG-3 lost", err)
	}
}

func TestMarshalSpecificationID(t *testing.T) {
	const xr21 = "urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_2.1"
	inv := fullInvoice()
	inv.Profile = invoice.ProfileXRechnung
	inv.SpecificationID = xr21
	cases := []struct {
		target invoice.Profile
		want   string
	}{
		{invoice.ProfileUnknown, xr21},
		{invoice.ProfileXRechnung, xr21},
		{invoice.ProfileEN16931, invoice.URNEN16931},
		{invoice.ProfileBasic, invoice.URNBasic},
	}
	for _, c := range cases {
		back := mustParse(t, mustMarshal(t, inv, &MarshalOptions{Profile: c.target}))
		if back.SpecificationID != c.want {
			t.Errorf("target %v: BT-24 = %q, want %q", c.target, back.SpecificationID, c.want)
		}
	}
	// Default profile.
	inv = &invoice.Invoice{Number: "1"}
	back := mustParse(t, mustMarshal(t, inv, nil))
	if back.Profile != invoice.ProfileEN16931 || back.SpecificationID != invoice.URNEN16931 {
		t.Errorf("default profile: %v %q", back.Profile, back.SpecificationID)
	}
}

func TestMarshalCIIDetails(t *testing.T) {
	out := string(mustMarshal(t, fullInvoice(), &MarshalOptions{Profile: invoice.ProfileEN16931}))
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?><rsm:CrossIndustryInvoice xmlns:rsm="` + NamespaceRSM + `" xmlns:qdt="` + NamespaceQDT + `" xmlns:ram="` + NamespaceRAM + `" xmlns:udt="` + NamespaceUDT + `">`,
		`<ram:IssueDateTime><udt:DateTimeString format="102">20261116</udt:DateTimeString></ram:IssueDateTime>`,
		`<ram:FormattedIssueDateTime><qdt:DateTimeString format="102">20261001</qdt:DateTimeString></ram:FormattedIssueDateTime>`,
		`<ram:TaxTotalAmount currencyID="EUR">39.00</ram:TaxTotalAmount><ram:TaxTotalAmount currencyID="USD">42.12</ram:TaxTotalAmount>`,
		`<ram:ID>Seller-ID-1</ram:ID><ram:GlobalID schemeID="0088">Seller-GLN</ram:GlobalID>`,
		`<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">FR12123456789</ram:ID></ram:SpecifiedTaxRegistration><ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">FC-Seller</ram:ID></ram:SpecifiedTaxRegistration>`,
		`<ram:URIUniversalCommunication><ram:URIID schemeID="EM">Seller@peppol.example</ram:URIID></ram:URIUniversalCommunication>`,
		`<ram:TaxPointDate><udt:DateString format="102">20261110</udt:DateString></ram:TaxPointDate><ram:DueDateTypeCode>5</ram:DueDateTypeCode>`,
		`<ram:SpecifiedProcuringProject><ram:ID>PRJ-1</ram:ID><ram:Name>Project One</ram:Name></ram:SpecifiedProcuringProject>`,
		`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>TENDER-1</ram:IssuerAssignedID><ram:TypeCode>50</ram:TypeCode></ram:AdditionalReferencedDocument>`,
		`<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>OBJ-1</ram:IssuerAssignedID><ram:TypeCode>130</ram:TypeCode><ram:ReferenceTypeCode>AAA</ram:ReferenceTypeCode></ram:AdditionalReferencedDocument>`,
		`<ram:URIID>https://example.com/doc?a=1&amp;b=2</ram:URIID><ram:TypeCode>916</ram:TypeCode><ram:Name>Timesheet</ram:Name><ram:AttachmentBinaryObject mimeCode="application/pdf" filename="doc.pdf">aGVsbG8sIHdvcmxkAP8=</ram:AttachmentBinaryObject>`,
		`<ram:BasisQuantity unitCode="C62">1</ram:BasisQuantity>`,
		`<ram:BilledQuantity unitCode="C62">2.000</ram:BilledQuantity>`,
		`<ram:ChargeIndicator><udt:Indicator>false</udt:Indicator></ram:ChargeIndicator>`,
		`<ram:ChargeIndicator><udt:Indicator>true</udt:Indicator></ram:ChargeIndicator>`,
		`<ram:ID>F-2026-001 &lt;&amp;&gt;</ram:ID>`,
		`<ram:Content>Note 1 "quoted" &amp; &lt;tagged&gt;</ram:Content>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %s", want)
		}
	}
	if n := strings.Count(out, "currencyID"); n != 2 {
		t.Errorf("%d currencyID attributes, want 2 (TaxTotalAmount only)", n)
	}
	if n := strings.Count(out, "<ram:TaxPointDate>"); n != 1 {
		t.Errorf("BT-7 written %d times, want once (CII-SR-461)", n)
	}
}

func TestMarshalEscaping(t *testing.T) {
	inv := &invoice.Invoice{
		Number:   "A&B<C>D\"E'F",
		TypeCode: "380",
		Notes: []invoice.Note{
			{Content: "line1\r\nline2\ttab \x01ctl \xffbad ]]> end"},
		},
		Seller: invoice.Party{Name: "Seller", ElectronicAddress: invoice.Identifier{ID: "x", Scheme: "a\"b<c&\n"}},
	}
	out := mustMarshal(t, inv, nil)
	if _, err := xmltree.Parse(out); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, out)
	}
	back := mustParse(t, out)
	if back.Number != inv.Number {
		t.Errorf("number = %q", back.Number)
	}
	if want := "line1\r\nline2\ttab �ctl �bad ]]> end"; back.Notes[0].Content != want {
		t.Errorf("note = %q, want %q", back.Notes[0].Content, want)
	}
	if back.Seller.ElectronicAddress.Scheme != "a\"b<c&" {
		t.Errorf("attribute = %q", back.Seller.ElectronicAddress.Scheme)
	}
}

func TestMarshalNil(t *testing.T) {
	if _, err := Marshal(nil, nil); err == nil {
		t.Error("Marshal(nil) succeeded")
	}
}

// TestMarshalMandatoryEmpty checks that an empty invoice still gives a
// document with the mandatory structure of every profile.
func TestMarshalMandatoryEmpty(t *testing.T) {
	for _, p := range invoice.Profiles {
		inv := &invoice.Invoice{Lines: []invoice.Line{{}}, VATBreakdown: []invoice.VATBreakdown{{}}}
		out := mustMarshal(t, inv, &MarshalOptions{Profile: p})
		checkXSD(t, p, out)
		back := mustParse(t, out)
		again := mustMarshal(t, back, &MarshalOptions{Profile: p})
		if !bytes.Equal(out, again) {
			t.Errorf("%v: not a fixed point", p)
		}
	}
}
