package ubl

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The CIUS-HR documents of package internal/rules/hr: an invoice with the
// Croatian VAT breakdown and every element of the CIUS-HR, a credit note
// with VAT on a cash basis, and the invoice of a seller outside the VAT
// system.
const (
	hrInvoice    = "../internal/rules/hr/testdata/cius-hr.ubl.xml"
	hrCreditNote = "../internal/rules/hr/testdata/cius-hr-cn.ubl.xml"
	hrOutOfScope = "../internal/rules/hr/testdata/cius-hr-o.ubl.xml"
)

// TestReadCroatia reads the elements of the CIUS-HR and the extension
// HRFISK20Data into the national blocks.
func TestReadCroatia(t *testing.T) {
	inv := mustParse(t, readFile(t, hrInvoice))
	if inv.Profile != invoice.ProfileCIUSHR {
		t.Errorf("profile %v", inv.Profile)
	}
	d := invoice.MustDecimal
	want := &invoice.Croatia{
		IssueTime: "09:30:15", OperatorName: "Operater 3", OperatorOIB: "82031549070",
		TaxTotal: d("130.40"),
		VATBreakdown: []invoice.CroatiaVAT{
			{TaxableAmount: d("480.00"), TaxAmount: d("120.00"), CategoryCode: "S", Category: "HR:PDV25", Rate: d("25"), TaxScheme: "VAT"},
			{TaxableAmount: d("80.00"), TaxAmount: d("10.40"), CategoryCode: "S", Category: "HR:PDV13", Rate: d("13"), TaxScheme: "VAT"},
		},
		TaxBasisTotal: d("590.00"), OutOfScopeAmount: d("3.00"),
	}
	got := inv.Croatia
	if got == nil || len(got.VATBreakdown) != 4 {
		t.Fatalf("Croatia: %+v", got)
	}
	head := *got
	head.VATBreakdown = head.VATBreakdown[:2]
	if a, b := jsonOf(t, &head), jsonOf(t, want); a != b {
		t.Errorf("Croatia:\n%s\nwant\n%s", a, b)
	}
	if v := got.VATBreakdown[3]; v.CategoryCode != "O" || v.Category != "HR:PP" || v.TaxScheme != "LOC" {
		t.Errorf("VATBreakdown[3]: %+v", v)
	}
	if r := inv.PrecedingInvoices[0].Croatia; r == nil || r.Description == "" {
		t.Errorf("PrecedingInvoices[0].Croatia: %+v", r)
	}
	if c := inv.AllowancesCharges[0].Croatia; c == nil || c.Category != "HR:PDV25" {
		t.Errorf("AllowancesCharges[0].Croatia: %+v", c)
	}
	if c := inv.AllowancesCharges[1].Croatia; c == nil || c.Category != "HR:PP" || c.ExemptionReason == "" {
		t.Errorf("AllowancesCharges[1].Croatia: %+v", c)
	}
	l := &inv.Lines[0]
	if l.OrderLineReference != "1" || l.OrderReference != "NAR-2026-17" || l.Croatia == nil || l.Croatia.Category != "HR:PDV25" ||
		jsonOf(t, l.Croatia.DespatchLine) != jsonOf(t, &invoice.DocumentReference{ID: "OTP-2026-55", LineID: "3"}) {
		t.Errorf("Lines[0]: %q %q %+v", l.OrderLineReference, l.OrderReference, l.Croatia)
	}
	if c := inv.Lines[1].Croatia; c == nil || c.ReceiptLine == nil || c.ReceiptLine.ID != "PRI-2026-8" || c.ReceiptLine.LineID != "1" {
		t.Errorf("Lines[1].Croatia: %+v", c)
	}
	if c := inv.Lines[2].Croatia; c == nil || c.Category != "HR:E" || c.ExemptionReason == "" {
		t.Errorf("Lines[2].Croatia: %+v", c)
	}
	for _, name := range []string{hrInvoice, hrCreditNote, hrOutOfScope} {
		if u := mustParse(t, readFile(t, name)).Unmodeled(); len(u) > 0 {
			t.Errorf("%s: unmodeled %q", name, u)
		}
	}
	cn := mustParse(t, readFile(t, hrCreditNote))
	if cn.CopyIndicator || cn.Croatia == nil || cn.Croatia.CashAccounting == "" || cn.Croatia.OperatorOIB != "HR82031549070" {
		t.Errorf("credit note: copy %v, Croatia %+v", cn.CopyIndicator, cn.Croatia)
	}

	// Without the identifier of the CIUS-HR, the same elements are kept as
	// extensions.
	plain := strings.Replace(string(readFile(t, hrInvoice)), invoice.URNCIUSHR+"<", invoice.URNEN16931+"<", 1)
	inv = mustParse(t, []byte(plain))
	if inv.Croatia != nil || inv.PrecedingInvoices[0].Croatia != nil || inv.AllowancesCharges[1].Croatia != nil || inv.Lines[0].Croatia != nil || inv.Lines[0].OrderReference != "" {
		t.Error("without the CIUS-HR: national data read")
	}
	all := strings.Join(inv.Unmodeled(), " | ")
	for _, s := range []string{"UBLExtensions", "IssueTime", "SellerContact", "DocumentDescription", "TaxCategory/Name", "DespatchLineReference", "ReceiptLineReference", "OrderReference"} {
		if !strings.Contains(all, s) {
			t.Errorf("without the CIUS-HR: %s not kept in %q", s, all)
		}
	}

	// The core identifier, without the extension, is the CIUS-HR too.
	core := strings.Replace(string(readFile(t, hrInvoice)), invoice.URNCIUSHR+"<", invoice.URNCIUSHRCore+"<", 1)
	if inv := mustParse(t, []byte(core)); inv.Croatia == nil || len(inv.Croatia.VATBreakdown) != 4 {
		t.Error("core identifier: national data not read")
	}
}

// TestWriteCroatia writes the documents back as they were read: the same
// elements in the same order, but the copy indicator false, which is the
// default. Another specification identifier loses the Croatian data.
func TestWriteCroatia(t *testing.T) {
	lines := func(b []byte) []string {
		var out []string
		for l := range strings.SplitSeq(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" && l != "<cbc:CopyIndicator>false</cbc:CopyIndicator>" {
				out = append(out, l)
			}
		}
		return out
	}
	for _, name := range []string{hrInvoice, hrCreditNote, hrOutOfScope} {
		data := readFile(t, name)
		inv := mustParse(t, data)
		out := mustMarshal(t, inv, &MarshalOptions{Indent: true, Strict: true})
		if got, want := lines(out), lines(data); !slices.Equal(got, want) {
			for i := range min(len(got), len(want)) {
				if got[i] != want[i] {
					t.Errorf("%s: line %d is %q, want %q", name, i+1, got[i], want[i])
					break
				}
			}
			t.Errorf("%s: %d lines written, %d read", name, len(got), len(want))
		}
		roundTrip(t, name, inv)
	}

	inv := mustParse(t, readFile(t, hrInvoice))
	inv.CopyIndicator = true
	inv.Lines[1].Croatia.ReceiptLine.LineID = ""
	inv.Lines[2].OrderReference = "NAR-2026-18"
	out := string(mustMarshal(t, inv, nil))
	for _, s := range []string{
		`<cbc:ID>17-POS1-1</cbc:ID><cbc:CopyIndicator>true</cbc:CopyIndicator><cbc:IssueDate>`,
		`<cac:ReceiptLineReference><cbc:LineID>NA</cbc:LineID><cac:DocumentReference><cbc:ID>PRI-2026-8</cbc:ID>`,
		`<cac:OrderLineReference><cbc:LineID>NA</cbc:LineID><cac:OrderReference><cbc:ID>NAR-2026-18</cbc:ID></cac:OrderReference></cac:OrderLineReference>`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %s", s)
		}
	}
	back := mustParse(t, []byte(out))
	if !back.CopyIndicator || back.Lines[1].Croatia.ReceiptLine.LineID != "" || back.Lines[2].OrderLineReference != "" || back.Lines[2].OrderReference != "NAR-2026-18" {
		t.Error("NA line identifiers or copy indicator not read back")
	}

	_, err := Marshal(inv, &MarshalOptions{Strict: true, CustomizationID: invoice.URNEN16931})
	var le *LossError
	if !errors.As(err, &le) {
		t.Fatalf("EN 16931: got %v, want the Croatian data lost", err)
	}
	want := []string{"EXT CopyIndicator", "EXT Line.OrderReference"}
	want = append(want, inv.NationalData()...)
	slices.Sort(want)
	got := slices.Sorted(slices.Values(le.Terms))
	if !slices.Equal(got, want) {
		t.Errorf("EN 16931: lost %q, want %q", got, want)
	}
}

func ExampleMarshal_ciusHR() {
	inv := &invoice.Invoice{
		Profile:   invoice.ProfileCIUSHR,
		Number:    "1-POS1-1",
		IssueDate: invoice.NewDate(2026, 3, 16),
		TypeCode:  "380",
		Currency:  "EUR",
		Croatia: &invoice.Croatia{
			IssueTime:    "09:30:15",
			OperatorName: "Operater 1",
			OperatorOIB:  "82031549070",
			TaxTotal:     invoice.MustDecimal("25.00"),
			VATBreakdown: []invoice.CroatiaVAT{{
				TaxableAmount: invoice.MustDecimal("100.00"), TaxAmount: invoice.MustDecimal("25.00"),
				CategoryCode: "S", Category: "HR:PDV25", Rate: invoice.MustDecimal("25"), TaxScheme: "VAT",
			}},
			TaxBasisTotal:    invoice.MustDecimal("100.00"),
			OutOfScopeAmount: invoice.MustDecimal("0.00"),
		},
	}
	out, err := Marshal(inv, &MarshalOptions{Indent: true})
	if err != nil {
		panic(err)
	}
	s := string(out)
	fmt.Println(strings.TrimSpace(s[strings.Index(s, "<cbc:CustomizationID>"):strings.Index(s, "<cbc:InvoiceTypeCode>")]))
	fmt.Println(strings.Contains(s, "<hrextac:HRFISK20Data>"), strings.Contains(s, "<cac:SellerContact>"))
	// Output:
	// <cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0</cbc:CustomizationID>
	//   <cbc:ID>1-POS1-1</cbc:ID>
	//   <cbc:IssueDate>2026-03-16</cbc:IssueDate>
	//   <cbc:IssueTime>09:30:15</cbc:IssueTime>
	// true true
}
