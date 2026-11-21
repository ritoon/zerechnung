package fatturapa

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

// The test invoices are written by hand, in the layout of Marshal with
// Indent: an invoice between private parties (TD24, discounts, transport,
// instalments, attachment) and an invoice of a lawyer to a public
// administration (withholding tax, pension fund, split payment, third
// party transmitter). Parties and identifiers are fictitious; their check
// digits are valid.
const (
	privateInvoice = "testdata/fpr12.xml"
	paInvoice      = "testdata/fpa12.xml"
	ciusitInvoice  = "../internal/rules/it/testdata/ciusit.xml"
	germanInvoice  = "../testdata/EN16931_Einfach.cii.xml"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parse(t *testing.T, name string) *invoice.Invoice {
	t.Helper()
	inv, err := Parse(read(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return inv
}

// TestRoundTrip reads the test invoices and writes them back: Marshal gives
// the same document, without loss, and reading it gives the same invoice.
func TestRoundTrip(t *testing.T) {
	for _, name := range []string{privateInvoice, paInvoice} {
		data := read(t, name)
		inv := parse(t, name)
		out, err := Marshal(inv, &MarshalOptions{Indent: true, Strict: true})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, data) {
			t.Errorf("%s: Marshal differs from the source:\n%s", name, out)
		}
		back, err := Parse(out)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(back, inv) {
			t.Errorf("%s: the invoice read back differs", name)
		}
	}
}

// check compares the values of an invoice, formatted with %v, to those
// expected.
func check(t *testing.T, name string, got map[string]any, want map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if g := fmt.Sprint(got[k]); g != want[k] {
			t.Errorf("%s: %s = %q, want %q", name, k, g, want[k])
		}
	}
}

func TestParsePrivate(t *testing.T) {
	inv := parse(t, privateInvoice)
	it := inv.Italy
	if it == nil || inv.Seller.Italy == nil || len(inv.Lines) != 4 || len(inv.VATBreakdown) != 2 || len(inv.PaymentMeans) != 2 ||
		len(inv.AdditionalDocuments) != 1 || it.Transport == nil || it.StampDuty == nil || inv.ShipTo == nil {
		t.Fatalf("unexpected structure: %+v", inv)
	}
	l0, l1, l2, l3 := &inv.Lines[0], &inv.Lines[1], &inv.Lines[2], &inv.Lines[3]
	check(t, privateInvoice, map[string]any{
		"profile":          inv.Profile,
		"number":           inv.Number,
		"type":             inv.TypeCode,
		"document type":    it.DocumentType,
		"issue date":       inv.IssueDate,
		"currency":         inv.Currency,
		"note":             inv.Notes[0].Content,
		"format":           it.Format,
		"transmitter":      it.TransmitterCountry + it.TransmitterCode,
		"sequence":         it.SequenceNumber,
		"system":           it.IssuingSystem,
		"transmitter mail": it.TransmitterEmail,
		"stamp duty":       it.StampDuty.Amount,
		"seller":           inv.Seller.Name,
		"seller VAT":       inv.Seller.VATID,
		"seller CF":        inv.Seller.LegalRegistration,
		"seller IDs":       inv.Seller.IDs,
		"seller address":   inv.Seller.Address,
		"seller contact":   *inv.Seller.Contact,
		"seller italy":     fmt.Sprintf("%s %s %q %q %q", inv.Seller.Italy.ShareCapital, inv.Seller.Italy.SoleShareholder, inv.Seller.Italy.Fax, inv.Seller.Italy.TaxRegime, inv.Seller.Italy.Liquidation),
		"buyer":            inv.Buyer.Name,
		"buyer VAT":        inv.Buyer.VATID,
		"buyer address":    inv.Buyer.ElectronicAddress,
		"order":            inv.PurchaseOrderReference,
		"other orders":     it.PurchaseOrders,
		"despatch advice":  inv.DespatchAdviceReference,
		"ddt":              it.TransportDocuments,
		"ship to":          inv.ShipTo.Address,
		"delivery date":    inv.DeliveryDate,
		"transport":        fmt.Sprintf("%s %s %s %s", it.Transport.Means, it.Transport.Packages, it.Transport.GrossWeight, it.Transport.DeliveryTime),
		"line 0 prices":    fmt.Sprintf("%s %s %s", l0.GrossPrice, l0.PriceDiscount, l0.NetPrice),
		"line 0 item":      fmt.Sprintf("%s %v", l0.Item.SellerID, l0.Item.GlobalID),
		"line 0 italy":     fmt.Sprintf("%v %v", l0.Italy.Codes, l0.Italy.Discounts),
		"line 0 quantity":  fmt.Sprintf("%s %s %s %s%s", l0.Quantity, l0.UnitCode, l0.NetAmount, l0.VATCategoryCode, l0.VATRate),
		"line 1 prices":    fmt.Sprintf("%s %s %s", l1.GrossPrice, l1.PriceDiscount, l1.NetPrice),
		"line 1 italy":     l1.Italy,
		"line 2 kind":      l2.Item.Attributes,
		"line 3":           fmt.Sprintf("%s%s %v", l3.VATCategoryCode, l3.VATRate, l3.Item.Attributes),
		"vat 0":            fmt.Sprintf("%s%s %s %s %v", inv.VATBreakdown[0].CategoryCode, inv.VATBreakdown[0].Rate, inv.VATBreakdown[0].TaxableAmount, inv.VATBreakdown[0].TaxAmount, *inv.VATBreakdown[0].Italy),
		"vat 1":            fmt.Sprintf("%s%s %q %q %v", inv.VATBreakdown[1].CategoryCode, inv.VATBreakdown[1].Rate, inv.VATBreakdown[1].ExemptionReason, inv.VATBreakdown[1].ExemptionReasonCode, *inv.VATBreakdown[1].Italy),
		"totals":           fmt.Sprintf("%s %s %s %s %s %s", inv.Totals.LineTotal, inv.Totals.ChargeTotal, inv.Totals.TaxBasisTotal, inv.Totals.TaxTotal, inv.Totals.GrandTotal, inv.Totals.DuePayable),
		"terms":            inv.PaymentTerms,
		"means 0":          fmt.Sprintf("%s %s %s %v", inv.PaymentMeans[0].TypeCode, inv.PaymentMeans[0].PayeeIBAN, inv.PaymentMeans[0].PayeeBIC, *inv.PaymentMeans[0].Italy),
		"means 1":          fmt.Sprintf("%s %s %v", inv.PaymentMeans[1].TypeCode, inv.PaymentMeans[1].PayeeIBAN, *inv.PaymentMeans[1].Italy),
		"attachment":       fmt.Sprintf("%s %s %s %s %q", inv.AdditionalDocuments[0].ID, inv.AdditionalDocuments[0].AttachmentFilename, inv.AdditionalDocuments[0].AttachmentMimeCode, inv.AdditionalDocuments[0].Description, inv.AdditionalDocuments[0].Attachment),
		"unmodeled":        inv.Unmodeled(),
	}, map[string]string{
		"profile":          "CIUS-IT",
		"number":           "FT-2026/0042",
		"type":             "380",
		"document type":    "TD24",
		"issue date":       "2026-11-20",
		"currency":         "EUR",
		"note":             "Fattura differita per le consegne di ottobre 2026.",
		"format":           "",
		"transmitter":      "",
		"sequence":         "00042",
		"system":           "ZER",
		"transmitter mail": "fatture@esempio-reti.it",
		"stamp duty":       "2.00",
		"seller":           "Esempio Reti S.r.l.",
		"seller VAT":       "IT12345678903",
		"seller CF":        "{12345678903 0210}",
		"seller IDs":       "[{REA:RM#1234567 }]",
		"seller address":   "{Via Nazionale 12  Roma 00184 RM IT}",
		"seller contact":   "{  0612345678 amministrazione@esempio-reti.it}",
		"seller italy":     `10000.00 SM "0612345679" "" ""`,
		"buyer":            "Officine Esempio S.p.A.",
		"buyer VAT":        "IT09876543217",
		"buyer address":    "{ABC1234 0205}",
		"order":            "ORD-2026-17",
		"other orders":     "[{[2] ORD-2026-18 2026-10-02 4   }]",
		"despatch advice":  "DDT-311",
		"ddt":              "[{DDT-311 2026-10-15 [1]}]",
		"ship to":          "{Via dei Magazzini 3  Milano 20139 MI IT}",
		"delivery date":    "2026-10-15",
		"transport":        "Furgone 3 42.5 2026-10-15T10:30:00",
		"line 0 prices":    "500.00 50.00 450.00",
		"line 0 item":      "SW-24P {8001234567890 0160}",
		"line 0 italy":     "[{INTERNO A-77}] [{SC 10.00 }]",
		"line 0 quantity":  "2.00 PZ 900.00 S22.00",
		"line 1 prices":    "40.00 5.00 35.00",
		"line 1 italy":     "<nil>",
		"line 2 kind":      "[{TipoCessionePrestazione AC  []}]",
		"line 3":           "Z0.00 [{NATURA N1  []}]",
		"vat 0":            "S22.00 1025.00 225.50 {20.00   }",
		"vat 1":            `Z0.00 "" "" {   Escluso art. 15 DPR 633/72}`,
		"totals":           "1027.00  1027.00 225.50 1252.50 1252.50",
		"terms":            "[{TP01 2026-12-20    []}]",
		"means 0":          "58 IT60X0542811101000000123456 BPPIITRRXXX {      626.25      Banca Esempio       }",
		"means 1":          "58 IT60X0542811101000000123456 {     2027-01-20 626.25             }",
		"attachment":       `ddt-311.pdf ddt-311.pdf application/pdf Documento di trasporto "%PDF-1.4\n%%EOF\n"`,
		"unmodeled":        "[]",
	})
}

func TestParsePA(t *testing.T) {
	inv := parse(t, paInvoice)
	it := inv.Italy
	if it == nil || len(inv.AllowancesCharges) != 1 || len(inv.Lines) != 1 || len(inv.VATBreakdown) != 1 || it.Intermediary == nil {
		t.Fatalf("unexpected structure: %+v", inv)
	}
	ac, l, v := &inv.AllowancesCharges[0], &inv.Lines[0], &inv.VATBreakdown[0]
	check(t, paInvoice, map[string]any{
		"format":            it.Format,
		"transmitter":       it.TransmitterCountry + it.TransmitterCode,
		"intermediary":      it.Intermediary.Name + " " + it.Intermediary.VATID,
		"issuer":            it.Issuer,
		"withholdings":      it.Withholdings,
		"seller":            inv.Seller.Name,
		"seller CF":         inv.Seller.LegalRegistration,
		"seller IDs":        inv.Seller.IDs,
		"seller italy":      fmt.Sprintf("%s %s", inv.Seller.Italy.Title, inv.Seller.Italy.RegisterProvince),
		"accounting":        inv.BuyerAccountingReference,
		"buyer":             inv.Buyer.Name + " " + inv.Buyer.VATID,
		"buyer CF":          inv.Buyer.LegalRegistration,
		"buyer address":     inv.Buyer.ElectronicAddress,
		"buyer reference":   inv.BuyerReference,
		"order":             inv.PurchaseOrderReference,
		"contract":          inv.ContractReference,
		"project":           inv.ProjectReference,
		"tender":            inv.TenderReference(),
		"fund":              fmt.Sprintf("%t %s %s %s %s%s %q %q", ac.Charge, ac.Amount, ac.BaseAmount, ac.Percentage, ac.VATCategoryCode, ac.VATRate, ac.Reason, ac.Italy.AdministrationRef),
		"line":              fmt.Sprintf("%s%s %v %s", l.VATCategoryCode, l.VATRate, *l.Italy, l.AccountingReference),
		"vat":               fmt.Sprintf("%s%s %s %s %v", v.CategoryCode, v.Rate, v.TaxableAmount, v.TaxAmount, *v.Italy),
		"totals":            fmt.Sprintf("%s %s %s %s %s %s", inv.Totals.LineTotal, inv.Totals.ChargeTotal, inv.Totals.TaxBasisTotal, inv.Totals.TaxTotal, inv.Totals.GrandTotal, inv.Totals.DuePayable),
		"terms":             inv.PaymentTerms[0].Description,
		"payment amount":    inv.PaymentMeans[0].Italy.Amount,
		"payment due date":  inv.PaymentTerms[0].DueDate,
		"tax point (BT-8)":  inv.TaxPointDateCode,
		"national data (0)": inv.NationalData()[0],
	}, map[string]string{
		"format":            "",
		"transmitter":       "IT02468135799",
		"intermediary":      "Studio Contabile Esempio IT02468135799",
		"issuer":            "TZ",
		"withholdings":      "[{RT01 200.00 20.00 A}]",
		"seller":            "Nome#Cognome:Maria#Rossi",
		"seller CF":         "{RSSMRA85T50A562W 0210}",
		"seller IDs":        "[{ALBO:Ordine degli Avvocati di Roma#A12345#2010-05-17 }]",
		"seller italy":      "Avv. RM",
		"accounting":        "UFF-LEGALE",
		"buyer":             "Comune di Esempio ",
		"buyer CF":          "{80012345676 0210}",
		"buyer address":     "{UFABCD 0201}",
		"buyer reference":   "#ORD-PA-88#2026-09-01#80012345676#",
		"order":             "ORD-PA-88",
		"contract":          "CTR-2026-12",
		"project":           "J12F26000010001",
		"tender":            "B12C345678",
		"fund":              `true 40.00 1000.00 4.00 B22.00 "TC04" "CPA"`,
		"line":              "B22.00 {[] [] true []} CAP-1234",
		"vat":               "B22.00 1040.00 228.80 {   Scissione dei pagamenti art. 17-ter DPR 633/72}",
		"totals":            "1000.00 40.00 1040.00 228.80 1268.80 1268.80",
		"terms":             "TP02 " + splitPaymentTerms,
		"payment amount":    "840.00",
		"payment due date":  "2026-12-18",
		"tax point (BT-8)":  "",
		"national data (0)": "EXT Italy.TransmitterCountry",
	})
}

// TestCIUSIT converts the CIUS-IT invoice of the Italian rules from CII to
// FatturaPA and back: the FatturaPA document reads as the same invoice for
// the data that FatturaPA carries, and the CII written from it is still a
// valid CIUS-IT invoice.
func TestCIUSIT(t *testing.T) {
	src, err := cii.Parse(read(t, ciusitInvoice))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Marshal(src, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) {
		t.Fatalf("strict Marshal: %v", err)
	}
	want := []string{"BT-41", "BT-34", "BT-132", "BG-27", "BT-20"}
	if !reflect.DeepEqual(le.Terms, want) {
		t.Errorf("lost terms %q, want %q", le.Terms, want)
	}
	out, err := Marshal(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, s := range []string{
		"<FormatoTrasmissione>FPA12</FormatoTrasmissione>",
		"<CodiceDestinatario>UFABCD</CodiceDestinatario>",
		"<IdCodice>12345678903</IdCodice>",
		"<ProgressivoInvio>FT20260042</ProgressivoInvio>",
		"<CodiceTipo>Identificativo del prodotto</CodiceTipo>",
		"<Descrizione>Manutenzione della rete Intervento mensile sugli apparati di rete degli uffici comunali</Descrizione>",
		"<PrezzoUnitario>114.00</PrezzoUnitario>",
		"<TipoCessionePrestazione>AC</TipoCessionePrestazione>",
		"<CodiceCIG>B12C345678</CodiceCIG>",
		"<CondizioniPagamento>TP02</CondizioniPagamento>",
		"<ModalitaPagamento>MP05</ModalitaPagamento>",
		"<CodicePagamento>FT-2026/0042</CodicePagamento>",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("%s not written:\n%s", s, out)
		}
	}
	if inv.Italy != nil {
		t.Errorf("national block read from an EN 16931 invoice: %+v", *inv.Italy)
	}
	if inv.Number != src.Number || inv.IssueDate != src.IssueDate || inv.Seller.VATID != src.Seller.VATID ||
		inv.Buyer.ElectronicAddress != src.Buyer.ElectronicAddress || inv.TenderReference() != src.TenderReference() ||
		inv.PurchaseOrderReference != src.PurchaseOrderReference || inv.RemittanceInformation != src.RemittanceInformation ||
		!inv.Totals.TaxBasisTotal.Equal(src.Totals.TaxBasisTotal) || !inv.Totals.GrandTotal.Equal(src.Totals.GrandTotal) ||
		!inv.Totals.DuePayable.Equal(src.Totals.DuePayable) || len(inv.Lines) != 3 {
		t.Errorf("invoice read back:\n%+v\nsource:\n%+v", inv, src)
	}
	back, err := cii.Marshal(inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := validate.XML(back, nil); r.Profile != invoice.ProfileCIUSIT || len(r.Findings) > 0 {
		t.Errorf("CII written back: profile %s\n%s", r.Profile, r)
	}
}

// TestNationalData writes the test invoices in CII and UBL: strict mode
// reports the national data as lost, and only it.
func TestNationalData(t *testing.T) {
	inv := parse(t, paInvoice)
	want := []string{
		"EXT Italy.TransmitterCountry", "EXT Italy.TransmitterCode", "EXT Italy.SequenceNumber", "EXT Italy.Intermediary",
		"EXT Italy.Issuer", "EXT Italy.Withholdings", "EXT Seller.Italy.Title", "EXT Seller.Italy.RegisterProvince",
		"EXT Lines[0].Italy.Withholding", "EXT VATBreakdown[0].Italy.LegalReference", "EXT PaymentMeans[0].Italy.Amount",
		"EXT AllowancesCharges[0].Italy.AdministrationRef",
	}
	if got := inv.NationalData(); !reflect.DeepEqual(got, want) {
		t.Errorf("NationalData = %q, want %q", got, want)
	}
	_, err := cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
	var ce *cii.LossError
	if !errors.As(err, &ce) || !reflect.DeepEqual(ce.Terms, want) {
		t.Errorf("cii.Marshal: %v", err)
	}
	_, err = ubl.Marshal(inv, &ubl.MarshalOptions{Strict: true})
	var ue *ubl.LossError
	if !errors.As(err, &ue) || !reflect.DeepEqual(ue.Terms, want) {
		t.Errorf("ubl.Marshal: %v", err)
	}
	inv.Italy, inv.Seller.Italy, inv.Lines[0].Italy, inv.VATBreakdown[0].Italy = nil, nil, nil, nil
	inv.PaymentMeans[0].Italy, inv.AllowancesCharges[0].Italy = nil, nil
	if _, err := cii.Marshal(inv, &cii.MarshalOptions{Strict: true}); err != nil {
		t.Errorf("cii.Marshal without national data: %v", err)
	}
}

// TestMarshalEN writes an invoice built from EN 16931 data alone: the
// FatturaPA values are derived from the EN 16931 fields.
func TestMarshalEN(t *testing.T) {
	src, err := cii.Parse(read(t, germanInvoice))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(src, &MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`versione="FPR12"`,
		"<IdPaese>DE</IdPaese>\n          <IdCodice>123456789</IdCodice>",
		"<ProgressivoInvio>471102</ProgressivoInvio>",
		"<CodiceDestinatario>XXXXXXX</CodiceDestinatario>",
		"<RegimeFiscale>RF01</RegimeFiscale>",
		"<TipoDocumento>TD01</TipoDocumento>",
		"<UnitaMisura>H87</UnitaMisura>",
		"<DataOraConsegna>2018-03-05T00:00:00</DataOraConsegna>",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("%q not written:\n%s", s, out)
		}
	}
	inv, err := Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if inv.Number != src.Number || len(inv.Lines) != len(src.Lines) || !inv.Totals.GrandTotal.Equal(src.Totals.GrandTotal) {
		t.Errorf("invoice read back: %+v", inv)
	}
	_, err = Marshal(src, &MarshalOptions{Strict: true})
	var le *LossError
	want := []string{"BT-29", "BT-32", "BT-46", "BT-21", "BT-20"}
	if !errors.As(err, &le) || !reflect.DeepEqual(le.Terms, want) {
		t.Errorf("strict Marshal: %v, want the loss of %q", err, want)
	}
}

// TestLotto reads a document with two bodies: ParseAll reads two invoices
// that share no value, Parse the first one with an issue.
func TestLotto(t *testing.T) {
	data := string(read(t, privateInvoice))
	i, j := strings.Index(data, "  <FatturaElettronicaBody>"), strings.Index(data, "</p:FatturaElettronica>")
	second := strings.Replace(data[i:j], "<Numero>FT-2026/0042</Numero>", "<Numero>FT-2026/0043</Numero>", 1)
	lotto := []byte(data[:j] + second + data[j:])
	invs, err := ParseAll(lotto)
	if err != nil || len(invs) != 2 {
		t.Fatalf("ParseAll: %d invoices, %v", len(invs), err)
	}
	if invs[0].Number != "FT-2026/0042" || invs[1].Number != "FT-2026/0043" || invs[1].Seller.Name != invs[0].Seller.Name {
		t.Errorf("invoices: %s %s %s", invs[0].Number, invs[1].Number, invs[1].Seller.Name)
	}
	if invs[0].Seller.Contact == invs[1].Seller.Contact || invs[0].Italy == invs[1].Italy {
		t.Error("the invoices of a lotto share values")
	}
	inv, err := Parse(lotto)
	var ie *IssuesError
	if inv == nil || inv.Number != "FT-2026/0042" || !errors.As(err, &ie) || len(ie.Issues) != 1 ||
		!strings.Contains(ie.Issues[0].Message, "2 invoices") {
		t.Errorf("Parse: %v", err)
	}
}

// TestTolerant reads a document with another prefix, a signature, an
// element that the format does not define and a malformed amount.
func TestTolerant(t *testing.T) {
	data := string(read(t, privateInvoice))
	data = strings.ReplaceAll(data, "p:FatturaElettronica", "ns3:FatturaElettronica")
	data = strings.Replace(data, "xmlns:p=", `xmlns:ds="http://www.w3.org/2000/09/xmldsig#" xmlns:ns3=`, 1)
	data = strings.Replace(data, "</FatturaElettronicaBody>", "</FatturaElettronicaBody><ds:Signature><ds:SignedInfo/></ds:Signature>", 1)
	data = strings.Replace(data, "<Divisa>EUR</Divisa>", `<Divisa>EUR</Divisa><Extra a="1">x</Extra>`, 1)
	data = strings.Replace(data, "<ImportoTotaleDocumento>1252.50", "<ImportoTotaleDocumento>1252,50", 1)
	inv, err := Parse([]byte(data))
	var ie *IssuesError
	if inv == nil || !errors.As(err, &ie) || len(ie.Issues) != 1 {
		t.Fatalf("Parse: %v", err)
	}
	if is := ie.Issues[0]; is.Path != "/p:FatturaElettronica/FatturaElettronicaBody/DatiGenerali/DatiGeneraliDocumento/ImportoTotaleDocumento" ||
		is.Line != 81 {
		t.Errorf("issue %v", is)
	}
	if got := inv.Unmodeled(); !reflect.DeepEqual(got, []string{"FatturaElettronicaBody/DatiGenerali/DatiGeneraliDocumento/Extra"}) {
		t.Errorf("Unmodeled = %q", got)
	}
	if inv.Totals.GrandTotal.String() != "1252.50" {
		t.Errorf("GrandTotal = %s, want the computed total", inv.Totals.GrandTotal)
	}
	_, err = Marshal(inv, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) || !reflect.DeepEqual(le.Terms, []string{"EXT FatturaElettronicaBody/DatiGenerali/DatiGeneraliDocumento/Extra"}) {
		t.Errorf("strict Marshal: %v", err)
	}
}

func TestNotFatturaPA(t *testing.T) {
	for _, data := range []string{
		string(read(t, ciusitInvoice)),
		strings.Replace(string(read(t, paInvoice)), Namespace, "urn:example", 1),
	} {
		inv, err := Parse([]byte(data))
		if inv != nil || !errors.Is(err, ErrNotFatturaPA) {
			t.Errorf("Parse = %v, %v", inv, err)
		}
	}
	if _, err := Parse([]byte("<p:FatturaElettronica")); err == nil || errors.Is(err, ErrNotFatturaPA) {
		t.Errorf("Parse of malformed XML: %v", err)
	}
}

// officialExamples are the example documents of FatturaPA 1.2 published on
// www.fatturapa.gov.it, with their SHA-256. Their terms of use do not allow
// copying them into this repository: TestOfficialExamples reads them from
// the directory named by FX_FATTURAPA, where conformance/fetch.sh downloads
// them.
var officialExamples = map[string]string{
	"IT01234567890_FPA01.xml": "e26b32f39ac87824d3255f3a9d2b5f06de6cd52dfba705ad057227a5a6d8c30b",
	"IT01234567890_FPA02.xml": "872befe17bad12323d0091b664bd8341dbd758b90511873b7badc563f167f3e2",
	"IT01234567890_FPA03.xml": "3acfe0322b4cc6312f397d2d40f7e6d2bf79e734779eddf13784185db57c506b",
	"IT01234567890_FPR01.xml": "73e61f2f71ca83937fdc2b64dfa71e45f025087b44ad73d835d8c080dc70b286",
	"IT01234567890_FPR02.xml": "f75334471b7411f4dbd9c05c403e6c6c06cbeb5f4d0d832851bc84adebdc0cf4",
	"IT01234567890_FPR03.xml": "3a9001230c6a7a0d76004a7eacecb025589ee3a3369d6d995b06eda165b6f358",
}

// TestOfficialExamples reads the official examples: no issue, no
// extension, and each invoice written back reads as the same invoice.
func TestOfficialExamples(t *testing.T) {
	dir := os.Getenv("FX_FATTURAPA")
	if dir == "" {
		t.Skip("FX_FATTURAPA is not set")
	}
	for name, sum := range officialExamples {
		data := read(t, filepath.Join(dir, name))
		if h := sha256.Sum256(data); hex.EncodeToString(h[:]) != sum {
			t.Fatalf("%s: SHA-256 %x, want %s", name, h, sum)
		}
		invs, err := ParseAll(data)
		if err != nil || len(invs) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
		for i, inv := range invs {
			if u := inv.Unmodeled(); len(u) > 0 {
				t.Errorf("%s #%d: extensions %q", name, i, u)
			}
			out, err := Marshal(inv, &MarshalOptions{Strict: true})
			if err != nil {
				t.Errorf("%s #%d: %v", name, i, err)
				continue
			}
			back, err := Parse(out)
			if err != nil {
				t.Errorf("%s #%d: %v", name, i, err)
			}
			if !reflect.DeepEqual(back, inv) {
				t.Errorf("%s #%d: the invoice read back differs", name, i)
			}
		}
	}
}
