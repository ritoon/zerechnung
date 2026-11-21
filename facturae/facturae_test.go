package facturae_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

// The test invoices are written by hand, in the layout of Marshal with
// Indent: an invoice of a company to a town council sent through FACe
// (administrative centres, three VAT rates, discount and charge on lines,
// exempt line, attachment) and an invoice of a self-employed person to a
// company (individual, IRPF withholding, payment on account). Parties and
// identifiers are fictitious; their check characters are valid. No official
// Facturae 3.2.x example is published.
const (
	b2gInvoice          = "testdata/b2g.xml"
	professionalInvoice = "testdata/profesional.xml"
	germanInvoice       = "../testdata/EN16931_Einfach.cii.xml"
	creditNote          = "../testdata/ubl-tc434-creditnote1.xml"
	italianInvoice      = "../fatturapa/testdata/fpr12.xml"
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
	inv, err := facturae.Parse(read(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return inv
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

// onlyScope reports whether the report of a Facturae document holds only
// the warning on the scope of the checks (FE-02).
func onlyScope(r *validate.Report) bool {
	return len(r.Findings) == 1 && r.Findings[0].Rule == "FE-02"
}

// TestRoundTrip reads the test invoices and writes them back: Marshal gives
// the same document, without loss, and reading it gives the same invoice.
func TestRoundTrip(t *testing.T) {
	for _, name := range []string{b2gInvoice, professionalInvoice} {
		data := read(t, name)
		inv := parse(t, name)
		out, err := facturae.Marshal(inv, &facturae.MarshalOptions{Indent: true, Strict: true})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, data) {
			t.Errorf("%s: Marshal differs from the source:\n%s", name, out)
		}
		for _, opts := range []*facturae.MarshalOptions{nil, {Indent: true}} {
			out, err := facturae.Marshal(inv, opts)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			back, err := facturae.Parse(out)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !reflect.DeepEqual(back, inv) {
				t.Errorf("%s: the invoice read back differs", name)
			}
		}
	}
}

// TestSchema checks the test invoices against the Facturae 3.2.2 schema.
func TestSchema(t *testing.T) {
	for _, name := range []string{b2gInvoice, professionalInvoice} {
		if r := validate.XML(read(t, name), nil); !onlyScope(r) {
			t.Errorf("%s:\n%s", name, r)
		}
	}
}

func TestParseB2G(t *testing.T) {
	inv := parse(t, b2gInvoice)
	sp := inv.Spain
	if sp == nil || sp.PlaceOfIssue == nil || inv.Seller.Spain == nil || inv.Seller.Spain.Registration == nil || inv.Seller.Contact == nil ||
		inv.Buyer.Spain == nil || len(inv.Buyer.Spain.Centres) != 3 || len(inv.Lines) != 3 || len(inv.VATBreakdown) != 3 ||
		len(inv.PaymentMeans) != 1 || len(inv.PaymentTerms) != 1 || len(inv.AdditionalDocuments) != 2 || len(inv.Notes) != 3 ||
		len(inv.Lines[0].AllowancesCharges) != 1 || len(inv.Lines[1].AllowancesCharges) != 1 {
		t.Fatalf("unexpected structure: %+v", inv)
	}
	l0, l1, l2 := &inv.Lines[0], &inv.Lines[1], &inv.Lines[2]
	a0, a1 := &l0.AllowancesCharges[0], &l1.AllowancesCharges[0]
	v0, v2 := &inv.VATBreakdown[0], &inv.VATBreakdown[2]
	c0, c2 := &inv.Buyer.Spain.Centres[0], &inv.Buyer.Spain.Centres[2]
	att, pm, tot := &inv.AdditionalDocuments[1], &inv.PaymentMeans[0], &inv.Totals
	var notes []string
	for _, n := range inv.Notes {
		notes = append(notes, n.SubjectCode+":"+n.Content)
	}
	check(t, b2gInvoice, map[string]any{
		"number":              inv.Number,
		"series":              sp.SeriesCode,
		"type":                inv.TypeCode,
		"issue date":          inv.IssueDate,
		"tax point (BT-7)":    inv.TaxPointDate,
		"place of issue":      *sp.PlaceOfIssue,
		"period":              fmt.Sprintf("%s %s", inv.InvoicingPeriod.Start, inv.InvoicingPeriod.End),
		"currency":            inv.Currency + "/" + inv.TaxCurrency + "/" + inv.Language,
		"notes":               strings.Join(notes, " | "),
		"order":               inv.PurchaseOrderReference,
		"contract":            inv.ContractReference,
		"tender (BT-17)":      inv.TenderReference(),
		"attachment":          fmt.Sprintf("%s %s %s %q", att.ID, att.Description, att.AttachmentMimeCode, att.Attachment),
		"seller":              inv.Seller.Name + " / " + inv.Seller.TradingName,
		"seller VAT":          inv.Seller.VATID,
		"seller registration": *inv.Seller.Spain.Registration,
		"seller contact":      *inv.Seller.Contact,
		"seller web":          inv.Seller.Spain.Web,
		"seller address":      inv.Seller.Address,
		"buyer":               inv.Buyer.Name,
		"buyer VAT":           inv.Buyer.VATID,
		"centre 0":            fmt.Sprintf("%s %s %s %s %s", c0.Code, c0.Role, c0.Name, c0.Description, c0.Address.PostCode),
		"centre 2":            fmt.Sprintf("%s %s %s %s", c2.Code, c2.Role, c2.Name, c2.Description),
		"line 0":              fmt.Sprintf("%s %s %s %s %s %s%s %s", l0.ID, l0.Quantity, l0.UnitCode, l0.NetPrice, l0.NetAmount, l0.VATCategoryCode, l0.VATRate, l0.Item.SellerID),
		"line 0 discount":     fmt.Sprintf("%t %s %s %s %q", a0.Charge, a0.Amount, a0.BaseAmount, a0.Percentage, a0.Reason),
		"line 1":              fmt.Sprintf("%s %s %s %s %s %s %s %q", l1.Quantity, l1.UnitCode, l1.NetAmount, l1.OrderReference, l1.Period.Start, l1.Period.End, l1.VATRate, l1.Note),
		"line 1 charge":       fmt.Sprintf("%t %s %q", a1.Charge, a1.Amount, a1.Reason),
		"line 2":              fmt.Sprintf("%s%s %s %v", l2.VATCategoryCode, l2.VATRate, l2.NetAmount, l2.Spain),
		"vat 0":               fmt.Sprintf("%s%s %s %s", v0.CategoryCode, v0.Rate, v0.TaxableAmount, v0.TaxAmount),
		"vat 2":               fmt.Sprintf("%s%s %s %s %q", v2.CategoryCode, v2.Rate, v2.TaxableAmount, v2.TaxAmount, v2.ExemptionReason),
		"totals":              fmt.Sprintf("%s %s %s %s %s", tot.LineTotal, tot.TaxBasisTotal, tot.TaxTotal, tot.GrandTotal, tot.DuePayable),
		"means":               fmt.Sprintf("%s %s %s %v", pm.TypeCode, pm.PayeeIBAN, pm.PayeeBIC, pm.Spain),
		"due date":            inv.PaymentTerms[0].DueDate,
		"remittance (BT-83)":  inv.RemittanceInformation,
		"unmodeled":           inv.Unmodeled(),
	}, map[string]string{
		"number":              "A0042",
		"series":              "A",
		"type":                "380",
		"issue date":          "2026-11-02",
		"tax point (BT-7)":    "2026-10-31",
		"place of issue":      "{28009 Madrid}",
		"period":              "2026-10-01 2026-10-31",
		"currency":            "EUR//",
		"notes":               ":Servicios de octubre de 2026 | ABL:Inscrita en el Registro Mercantil de Madrid, tomo 23456, folio 120, hoja M-123456 | AAI:Factura presentada en FACe",
		"order":               "PED-2026-118",
		"contract":            "CON-2025-031",
		"tender (BT-17)":      "EXP-2026-007",
		"attachment":          `1 Parte de horas application/pdf "%PDF-1.4\n% Parte de horas\n%%EOF\n"`,
		"seller":              "Soluciones Informáticas del Centro S.L. / SoluCentro",
		"seller VAT":          "ESB12345674",
		"seller registration": "{0 Madrid M-123456 120 8 23456 }",
		"seller contact":      "{Ana García  910000000 facturacion@solucentro.example}",
		"seller web":          "www.solucentro.example",
		"seller address":      "{Calle de Alcalá 100   Madrid 28009 Madrid ES}",
		"buyer":               "Ayuntamiento de Villa Ejemplo",
		"buyer VAT":           "ESP2899999C",
		"centre 0":            "L01999991 01 Intervención Oficina contable 28999",
		"centre 2":            "L01999993 03 Servicios Informáticos Unidad tramitadora",
		"line 0":              "1 2 C62 500.00 900.00 S21.00 LIC-01",
		"line 0 discount":     `false 100.00 1000.00 10.00 "Descuento comercial"`,
		"line 1":              `10 HUR 650.00 PED-2026-118 2026-10-01 2026-10-31 10.00 "Incluye la retirada de restos vegetales"`,
		"line 1 charge":       `true 50.00 "Desplazamiento"`,
		"line 2":              "E0.00 300.00 <nil>",
		"vat 0":               "S21.00 900.00 189.00",
		"vat 2":               `E0.00 300.00 0.00 "Exenta por el artículo 20.Uno.9º de la Ley 37/1992"`,
		"totals":              "1850.00 1850.00 254.00 2104.00 2104.00",
		"means":               "30 ES9121000418450200051332 CAIXESBBXXX <nil>",
		"due date":            "2026-12-02",
		"remittance (BT-83)":  "A0042",
		"unmodeled":           "[]",
	})
}

func TestParseProfessional(t *testing.T) {
	inv := parse(t, professionalInvoice)
	sp := inv.Spain
	if sp == nil || inv.Seller.Spain == nil || inv.Seller.Contact == nil || len(inv.Lines) != 1 || inv.Lines[0].Spain == nil ||
		len(inv.PaymentMeans) != 1 || inv.PaymentMeans[0].Spain == nil {
		t.Fatalf("unexpected structure: %+v", inv)
	}
	s, l, tot := inv.Seller.Spain, &inv.Lines[0], &inv.Totals
	check(t, professionalInvoice, map[string]any{
		"number":             inv.Number,
		"seller":             inv.Seller.Name,
		"seller names":       s.GivenName + "|" + s.FirstSurname + "|" + s.SecondSurname,
		"seller VAT":         inv.Seller.VATID,
		"seller contact":     *inv.Seller.Contact,
		"buyer":              inv.Buyer.Name + " " + inv.Buyer.VATID,
		"withholdings":       sp.Withholdings,
		"line withholdings":  l.Spain.Withholdings,
		"payments":           sp.PaymentsOnAccount,
		"facturae totals":    fmt.Sprintf("%q %q %q", sp.InvoiceTotal, sp.OutstandingAmount, sp.ExecutableAmount),
		"totals":             fmt.Sprintf("%s %s %s %s %s %s", tot.LineTotal, tot.TaxBasisTotal, tot.TaxTotal, tot.GrandTotal, tot.PaidAmount, tot.DuePayable),
		"installment amount": inv.PaymentMeans[0].Spain.Amount,
		"due date":           inv.PaymentTerms[0].DueDate,
		"line":               fmt.Sprintf("%s %s %s %s%s", l.Quantity, l.UnitCode, l.NetAmount, l.VATCategoryCode, l.VATRate),
	}, map[string]string{
		"number":             "2026-118",
		"seller":             "Lucía Martínez Ruiz",
		"seller names":       "Lucía|Martínez|Ruiz",
		"seller VAT":         "ES12345678Z",
		"seller contact":     "{  600000000 lucia@asesoria.example}",
		"buyer":              "Distribuciones Mediterráneo S.A. ESA28000008",
		"withholdings":       "[{04 15.00 1000.00  150.00  <nil>}]",
		"line withholdings":  "[{04 15.00 1000.00  150.00  <nil>}]",
		"payments":           "[{2026-10-15 100.00}]",
		"facturae totals":    `"" "" ""`,
		"totals":             "1000.00 1000.00 210.00 1210.00 100.00 1110.00",
		"installment amount": "960.00",
		"due date":           "2026-12-05",
		"line":               "1 C62 1000.00 S21.00",
	})
}

var (
	sixDecimals  = regexp.MustCompile(`<(UnitPriceWithoutTax|TotalCost|DiscountAmount|ChargeAmount|GrossAmount)>(-?[0-9]+\.[0-9]{2})<`)
	fourDecimals = regexp.MustCompile(`<(DiscountRate|ChargeRate)>([0-9]+\.[0-9]{2})<`)
)

// TestOlderVersions reads the B2G invoice in versions 3.2.1 and 3.2, without
// the elements that version 3.2.2 added: the document is valid against the
// schema of its version, and Marshal writes it in version 3.2.2.
func TestOlderVersions(t *testing.T) {
	src := string(read(t, b2gInvoice))
	for _, e := range []string{"InvoiceDescription", "ReceiverTransactionReference", "FileReference", "ReceiverContractReference"} {
		i := strings.Index(src, "\n        <"+e+">")
		j := i + strings.Index(src[i:], "</"+e+">") + len("</"+e+">")
		src = src[:i] + src[j:]
	}
	for _, v := range []struct{ version, ns string }{{"3.2.1", facturae.Namespace321}, {"3.2", facturae.Namespace32}} {
		data := strings.Replace(src, facturae.Namespace322, v.ns, 1)
		data = strings.Replace(data, "<SchemaVersion>3.2.2<", "<SchemaVersion>"+v.version+"<", 1)
		if v.version == "3.2" {
			// The amounts of the lines have six decimals, the rates four.
			data = sixDecimals.ReplaceAllString(data, "<$1>${2}0000<")
			data = fourDecimals.ReplaceAllString(data, "<$1>${2}00<")
		}
		if r := validate.XML([]byte(data), nil); !onlyScope(r) || !strings.Contains(r.Findings[0].Message, "Facturae "+v.version+" ") {
			t.Errorf("%s:\n%s", v.version, r)
		}
		inv, err := facturae.Parse([]byte(data))
		if err != nil {
			t.Fatalf("%s: %v", v.version, err)
		}
		if inv.Number != "A0042" || inv.PurchaseOrderReference != "" || inv.ContractReference != "" || inv.TenderReference() != "" ||
			len(inv.Notes) != 2 || inv.Totals.DuePayable.String() != "2104.00" || len(inv.Lines) != 3 || inv.Lines[1].OrderReference != "PED-2026-118" {
			t.Errorf("%s: %+v", v.version, inv)
		}
		out, err := facturae.Marshal(inv, &facturae.MarshalOptions{Strict: true})
		if err != nil {
			t.Fatalf("%s: %v", v.version, err)
		}
		if !bytes.Contains(out, []byte("<SchemaVersion>3.2.2</SchemaVersion>")) {
			t.Errorf("%s: written in another version:\n%s", v.version, out)
		}
		if back, err := facturae.Parse(out); err != nil || !reflect.DeepEqual(back, inv) {
			t.Errorf("%s: the invoice read back differs (%v)", v.version, err)
		}
	}
}

// TestBatch reads a document with two invoices: ParseAll reads two invoices
// that share no value, Parse the first one with an issue.
func TestBatch(t *testing.T) {
	data := string(read(t, b2gInvoice))
	i, j := strings.Index(data, "    <Invoice>\n"), strings.Index(data, "  </Invoices>")
	second := strings.Replace(data[i:j], "<InvoiceNumber>0042</InvoiceNumber>", "<InvoiceNumber>0043</InvoiceNumber>", 1)
	batch := []byte(data[:j] + second + data[j:])
	invs, err := facturae.ParseAll(batch)
	if err != nil || len(invs) != 2 {
		t.Fatalf("ParseAll: %d invoices, %v", len(invs), err)
	}
	if invs[0].Number != "A0042" || invs[1].Number != "A0043" || invs[1].Seller.Name != invs[0].Seller.Name {
		t.Errorf("invoices: %s %s %s", invs[0].Number, invs[1].Number, invs[1].Seller.Name)
	}
	if invs[0].Seller.Contact == invs[1].Seller.Contact || invs[0].Spain == invs[1].Spain || &invs[0].Buyer.Spain.Centres[0] == &invs[1].Buyer.Spain.Centres[0] {
		t.Error("the invoices of a batch share values")
	}
	inv, err := facturae.Parse(batch)
	var ie *facturae.IssuesError
	if inv == nil || inv.Number != "A0042" || !errors.As(err, &ie) || len(ie.Issues) != 1 || !strings.Contains(ie.Issues[0].Message, "2 invoices") {
		t.Errorf("Parse: %v", err)
	}
}

// TestTolerant reads a document with another prefix, a signature, an
// element that the format does not define and a malformed amount.
func TestTolerant(t *testing.T) {
	data := string(read(t, b2gInvoice))
	data = strings.ReplaceAll(data, "fe:Facturae", "ns2:Facturae")
	data = strings.Replace(data, "xmlns:fe=", `xmlns:ds="http://www.w3.org/2000/09/xmldsig#" xmlns:ns2=`, 1)
	data = strings.Replace(data, "  </Invoices>\n", "  </Invoices>\n  <ds:Signature><ds:SignedInfo/></ds:Signature>\n", 1)
	data = strings.Replace(data, "<TradeName>SoluCentro</TradeName>", `<TradeName>SoluCentro</TradeName><Extra a="1">x</Extra>`, 1)
	data = strings.Replace(data, "<TotalGrossAmount>1850.00", "<TotalGrossAmount>1850,00", 1)
	inv, err := facturae.Parse([]byte(data))
	var ie *facturae.IssuesError
	if inv == nil || !errors.As(err, &ie) || len(ie.Issues) != 1 {
		t.Fatalf("Parse: %v", err)
	}
	if is := ie.Issues[0]; is.Path != "/ns2:Facturae/Invoices/Invoice/InvoiceTotals/TotalGrossAmount" || is.Line != 174 {
		t.Errorf("issue %v", is)
	}
	if got := inv.Unmodeled(); !reflect.DeepEqual(got, []string{"Seller: LegalEntity/Extra"}) {
		t.Errorf("Unmodeled = %q", got)
	}
	if inv.Totals.LineTotal.IsSet() || inv.Totals.GrandTotal.String() != "2104.00" {
		t.Errorf("totals %+v", inv.Totals)
	}
	_, err = facturae.Marshal(inv, &facturae.MarshalOptions{Strict: true})
	var le *facturae.LossError
	if !errors.As(err, &le) || !reflect.DeepEqual(le.Terms, []string{"EXT Seller: LegalEntity/Extra"}) {
		t.Errorf("strict Marshal: %v", err)
	}
}

func TestNotFacturae(t *testing.T) {
	for _, data := range []string{
		string(read(t, germanInvoice)),
		strings.Replace(string(read(t, b2gInvoice)), facturae.Namespace322, "urn:example", 1),
	} {
		inv, err := facturae.Parse([]byte(data))
		if inv != nil || !errors.Is(err, facturae.ErrNotFacturae) {
			t.Errorf("Parse = %v, %v", inv, err)
		}
	}
	if _, err := facturae.Parse([]byte("<fe:Facturae")); err == nil || errors.Is(err, facturae.ErrNotFacturae) {
		t.Errorf("Parse of malformed XML: %v", err)
	}
}

// TestCreditNote writes a UBL credit note: a corrective invoice (OR) with
// negative amounts, whose Corrective block is derived, valid against the
// schema; it reads back as a corrected invoice (384) with the same totals,
// negative.
func TestCreditNote(t *testing.T) {
	src, err := ubl.Parse(read(t, creditNote))
	if err != nil {
		t.Fatal(err)
	}
	out, err := facturae.Marshal(src, &facturae.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"<InvoiceClass>OR</InvoiceClass>",
		"<ReasonCode>16</ReasonCode>\n          <ReasonDescription>Base imponible</ReasonDescription>",
		"<TaxPeriod>\n            <StartDate>2019-02-01</StartDate>\n            <EndDate>2019-02-28</EndDate>",
		"<CorrectionMethod>02</CorrectionMethod>\n          <CorrectionMethodDescription>Rectificación por diferencias</CorrectionMethodDescription>",
		"<InvoiceTotal>-100.11</InvoiceTotal>",
		"<Quantity>-1.00</Quantity>",
		"<UnitPriceWithoutTax>100.11</UnitPriceWithoutTax>",
		"<InstallmentDueDate>2019-09-23</InstallmentDueDate>",
		"<BIC>BPOTBEB1XXX</BIC>",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("%q not written:\n%s", s, out)
		}
	}
	if r := validate.XML(out, nil); !onlyScope(r) {
		t.Errorf("schema:\n%s", r)
	}
	inv, err := facturae.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if inv.TypeCode != "384" || inv.Number != src.Number || inv.Totals.GrandTotal.String() != "-100.11" ||
		inv.Lines[0].NetAmount.String() != "-100.11" || inv.Spain != nil {
		t.Errorf("invoice read back: %+v", inv)
	}
	if r := validate.XML(mustUBL(t, inv), nil); !r.Valid() {
		t.Errorf("UBL written back:\n%s", r)
	}
}

func mustUBL(t *testing.T, inv *invoice.Invoice) []byte {
	t.Helper()
	out, err := ubl.Marshal(inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestFromCII writes a German CII invoice: the Facturae values are derived
// from the EN 16931 fields, strict mode lists the terms that Facturae cannot
// carry, and the invoice reads back with the same totals.
func TestFromCII(t *testing.T) {
	src, err := cii.Parse(read(t, germanInvoice))
	if err != nil {
		t.Fatal(err)
	}
	// Facturae requires the tax identifier of the buyer, which the example
	// lacks.
	src.Buyer.VATID = "DE987654320"
	out, err := facturae.Marshal(src, &facturae.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"<BatchIdentifier>DE123456789471102</BatchIdentifier>",
		"<ResidenceTypeCode>U</ResidenceTypeCode>\n        <TaxIdentificationNumber>DE123456789</TaxIdentificationNumber>",
		"<PostCodeAndTown>80333 München</PostCodeAndTown>",
		"<CountryCode>DEU</CountryCode>",
		"<InvoiceDescription>Rechnung gemäß Bestellung vom 01.03.2018.</InvoiceDescription>",
		"<TaxRate>7.00</TaxRate>",
		"<ArticleCode>TB100A4</ArticleCode>",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("%q not written:\n%s", s, out)
		}
	}
	if r := validate.XML(out, nil); !onlyScope(r) {
		t.Errorf("schema:\n%s", r)
	}
	_, err = facturae.Marshal(src, &facturae.MarshalOptions{Strict: true})
	var le *facturae.LossError
	want := []string{"BT-29", "BT-32", "BT-46", "BG-1", "BT-130", "BT-148", "BT-157", "BT-20", "BT-72"}
	if !errors.As(err, &le) || !reflect.DeepEqual(le.Terms, want) {
		t.Errorf("strict Marshal: %v, want the loss of %q", err, want)
	}
	inv, err := facturae.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Number != src.Number || inv.Seller.VATID != src.Seller.VATID || len(inv.Lines) != len(src.Lines) ||
		!inv.Totals.TaxBasisTotal.Equal(src.Totals.TaxBasisTotal) || !inv.Totals.GrandTotal.Equal(src.Totals.GrandTotal) ||
		!inv.Totals.DuePayable.Equal(src.Totals.DuePayable) || inv.Spain != nil {
		t.Errorf("invoice read back: %+v", inv)
	}
}

// TestNationalData writes the test invoices in UBL, CII and FatturaPA: the
// documents are valid, and strict mode reports the national data as lost.
func TestNationalData(t *testing.T) {
	for _, tc := range []struct {
		name string
		want []string
		lost []string // by UBL and CII, before the national data
	}{
		{b2gInvoice, []string{"EXT Spain.SeriesCode", "EXT Spain.PlaceOfIssue", "EXT Seller.Spain.Registration", "EXT Seller.Spain.Web", "EXT Buyer.Spain.Centres"}, []string{"EXT Line.OrderReference"}},
		{professionalInvoice, []string{
			"EXT Spain.Withholdings", "EXT Spain.PaymentsOnAccount", "EXT Seller.Spain.GivenName", "EXT Seller.Spain.FirstSurname",
			"EXT Seller.Spain.SecondSurname", "EXT Lines[0].Spain.Withholdings", "EXT PaymentMeans[0].Spain.Amount",
		}, nil},
	} {
		inv := parse(t, tc.name)
		if got := inv.NationalData(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: NationalData = %q, want %q", tc.name, got, tc.want)
		}
		if r := validate.XML(mustUBL(t, inv), nil); len(r.Findings) > 0 {
			t.Errorf("%s: UBL:\n%s", tc.name, r)
		}
		out, err := cii.Marshal(inv, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r := validate.XML(out, nil); len(r.Findings) > 0 {
			t.Errorf("%s: CII:\n%s", tc.name, r)
		}
		lost := append(tc.lost, tc.want...)
		_, err = ubl.Marshal(inv, &ubl.MarshalOptions{Strict: true})
		var ue *ubl.LossError
		if !errors.As(err, &ue) || !reflect.DeepEqual(ue.Terms, lost) {
			t.Errorf("%s: ubl.Marshal: %v", tc.name, err)
		}
		_, err = cii.Marshal(inv, &cii.MarshalOptions{Strict: true})
		var ce *cii.LossError
		if !errors.As(err, &ce) || !reflect.DeepEqual(ce.Terms, lost) {
			t.Errorf("%s: cii.Marshal: %v", tc.name, err)
		}
		_, err = fatturapa.Marshal(inv, &fatturapa.MarshalOptions{Strict: true})
		var fe *fatturapa.LossError
		if !errors.As(err, &fe) {
			t.Fatalf("%s: fatturapa.Marshal: %v", tc.name, err)
		}
		for _, term := range tc.want {
			if !slices.Contains(fe.Terms, term) {
				t.Errorf("%s: fatturapa.Marshal does not report %s as lost: %q", tc.name, term, fe.Terms)
			}
		}
	}
}

// TestItalianData writes a FatturaPA invoice: strict mode reports the
// Italian national data as lost.
func TestItalianData(t *testing.T) {
	inv, err := fatturapa.Parse(read(t, italianInvoice))
	if err != nil {
		t.Fatal(err)
	}
	_, err = facturae.Marshal(inv, &facturae.MarshalOptions{Strict: true})
	var le *facturae.LossError
	if !errors.As(err, &le) {
		t.Fatalf("strict Marshal: %v", err)
	}
	for _, term := range inv.NationalData() {
		if !slices.Contains(le.Terms, term) {
			t.Errorf("%s not reported as lost: %q", term, le.Terms)
		}
	}
}

// TestForeignCurrency writes an invoice in dollars whose VAT is accounted in
// euros: the VAT total in euros (BT-111) needs the amounts in euros of the
// VAT breakdown, which the national block gives.
func TestForeignCurrency(t *testing.T) {
	src, err := cii.Parse(read(t, germanInvoice))
	if err != nil {
		t.Fatal(err)
	}
	src.Buyer.VATID = "DE987654320"
	src.Currency, src.TaxCurrency = "USD", "EUR"
	src.Totals.TaxTotalInTaxCurrency = invoice.MustDecimal("48.50")
	_, err = facturae.Marshal(src, &facturae.MarshalOptions{Strict: true})
	var le *facturae.LossError
	if !errors.As(err, &le) || !slices.Contains(le.Terms, "BT-111") {
		t.Errorf("strict Marshal without the VAT in euros: %v", err)
	}
	src.VATBreakdown[0].Spain = &invoice.SpainVAT{TaxableEuros: invoice.MustDecimal("237.00"), TaxEuros: invoice.MustDecimal("16.59")}
	src.VATBreakdown[1].Spain = &invoice.SpainVAT{TaxableEuros: invoice.MustDecimal("170.70"), TaxEuros: invoice.MustDecimal("31.91")}
	_, err = facturae.Marshal(src, &facturae.MarshalOptions{Strict: true})
	if !errors.As(err, &le) || slices.Contains(le.Terms, "BT-111") {
		t.Errorf("strict Marshal with the VAT in euros: %v", err)
	}
	out, err := facturae.Marshal(src, &facturae.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"<InvoiceCurrencyCode>USD</InvoiceCurrencyCode>\n        <TaxCurrencyCode>EUR</TaxCurrencyCode>",
		"<TotalAmount>275.00</TotalAmount>\n            <EquivalentInEuros>237.00</EquivalentInEuros>",
		"<TotalAmount>37.62</TotalAmount>\n            <EquivalentInEuros>31.91</EquivalentInEuros>",
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("%q not written:\n%s", s, out)
		}
	}
	if r := validate.XML(out, nil); !onlyScope(r) {
		t.Errorf("schema:\n%s", r)
	}
	inv, err := facturae.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Currency != "USD" || inv.TaxCurrency != "EUR" || inv.Totals.TaxTotalInTaxCurrency.String() != "48.50" ||
		!reflect.DeepEqual(inv.VATBreakdown[1].Spain, src.VATBreakdown[1].Spain) {
		t.Errorf("invoice read back: %s %s %s %+v", inv.Currency, inv.TaxCurrency, inv.Totals.TaxTotalInTaxCurrency, inv.VATBreakdown[1].Spain)
	}
}
