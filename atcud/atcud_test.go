package atcud

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/qrcode"
	"github.com/ritoon/zerechnung/ubl"
)

// specExamples are the seven examples of section 5 of the specification of
// the AT (version 1.1, October 2020), as their symbols hold them: decoded
// from the figures of the PDF, which the text of the PDF breaks across
// lines. version is the version of the figure; fields are the fields that
// Parse reports.
var specExamples = []struct {
	name    string
	content string
	version int
	fields  []string
}{
	{"1 Fatura", "A:123456789*B:999999990*C:PT*D:FT*E:N*F:20191231*G:FT AB2019/0035*H:CSDF7T5H-0035*" +
		"I1:PT*I2:12000.00*I3:15000.00*I4:900.00*I5:50000.00*I6:6500.00*I7:80000.00*I8:18400.00*" +
		"J1:PT-AC*J2:10000.00*J3:25000.56*J4:1000.02*J5:75000.00*J6:6750.00*J7:100000.00*J8:18000.00*" +
		"K1:PT-MA*K2:5000.00*K3:12500.00*K4:625.00*K5:25000.00*K6:3000.00*K7:40000.00*K8:8800.00*" +
		"L:100.00*M:25.00*N:64000.02*O:513600.58*P:100.00*Q:kLp0*R:9999*S:TB;PT00000000000000000000000;513500.58", 17, nil},
	{"2 Fatura simplificada", "A:123456789*B:999999990*C:PT*D:FS*E:N*F:20190812*G:FS CDVF/12345*H:CDF7T5HD-12345*" +
		"I1:PT*I7:0.65*I8:0.15*N:0.15*O:0.80*Q:YhGV*R:9999*S:NU;0.80", 9, nil},
	{"3 Fatura pró-forma", "A:500000000*B:123456789*C:PT*D:PF*E:N*F:20190123*G:PF G2019CB/145789*H:HB6FT7RV-145789*" +
		"I1:PT*I2:12345.34*I3:12532.65*I4:751.96*I5:52789.00*I6:6862.57*I7:32425.69*I8:7457.91*N:15072.44*O:125165.12*Q:r/fY*R:9999", 10, nil},
	{"4 Documento de transporte", "A:500000000*B:123456789*C:PT*D:GT*E:N*F:20190720*G:GT G234CB/50987*H:GTVX4Y8B-50987*" +
		"I1:0*N:0.00*O:0.00*Q:5uIg*R:9999", 9, nil},
	{"5 Taxa de imposto estrangeiro", "A:123456789*B:4443332215*C:FR*D:FT*E:N*F:20190526*G:ABC BNH/4561*H:DK5ZJ2HN-4561*" +
		"I1:FR*I7:100.00*I8:20.00*N:20.00*O:120.00*Q:YJRE*R:9999", 9, nil},
	{"6 Nota de débito", "A:123456789*B:500000000*C:PT*D:ND*E:N*F:20190216*G: M1F KLG/6145*H: RQD8L6DG-6145*" +
		"I1:PT-MA*I6:26.50*N:26.50*O:26.50*Q:h1rB*R:9999", 9, []string{"G", "H"}},
	{"7 Regime da margem", "A:500000000*B:123456789*C:PT*D:FT*E:N*F:20191124*G:NF 19A/789145*H: JL9DS4TT-789145*" +
		"I1:PT-AC*I7:50.00*I8:9.00*L:1000.00*N:9.00*O:1059.00*Q:d8/K*R:9999", 9, []string{"H"}},
}

// fields returns the fields of the *FieldError that err joins.
func fields(err error) []string {
	var out []string
	var walk func(error)
	walk = func(err error) {
		var fe *FieldError
		switch e := err.(type) {
		case nil:
		case interface{ Unwrap() []error }:
			for _, x := range e.Unwrap() {
				walk(x)
			}
		default:
			if errors.As(err, &fe) {
				out = append(out, fe.Field)
			} else {
				out = append(out, "?"+err.Error())
			}
		}
	}
	walk(err)
	return out
}

func TestSpecificationExamples(t *testing.T) {
	for _, ex := range specExamples {
		t.Run(ex.name, func(t *testing.T) {
			q, err := Parse(ex.content)
			if got := fields(err); !slices.Equal(got, ex.fields) {
				t.Fatalf("fields %v, want %v: %v", got, ex.fields, err)
			}
			content := ex.content
			if ex.fields != nil {
				// The spaces after the colons removed, as section 3 a) asks.
				content = strings.NewReplacer("G: ", "G:", "H: ", "H:").Replace(content)
				if q, err = Parse(content); err != nil {
					t.Fatal(err)
				}
			}
			if got := q.String(); got != content {
				t.Errorf("String:\n%s\nwant\n%s", got, content)
			}
			sym, err := q.Symbol()
			if err != nil {
				t.Fatal(err)
			}
			if ex.fields == nil && sym.Version() != ex.version {
				t.Errorf("version %d, the figure of the specification has version %d", sym.Version(), ex.version)
			}
			if sym.Level() != qrcode.Medium {
				t.Errorf("level %s", sym.Level())
			}
		})
	}
}

// TestSpecificationExample1 writes example 1, which fills every field, from
// the fields.
func TestSpecificationExample1(t *testing.T) {
	d := invoice.MustDecimal
	q := &QR{
		IssuerNIF: "123456789", BuyerTaxID: "999999990", BuyerCountry: "PT", DocumentType: "FT", DocumentStatus: "N",
		Date: invoice.NewDate(2019, time.December, 31), DocumentID: "FT AB2019/0035", ATCUD: "CSDF7T5H-0035",
		Spaces: [3]Space{
			{"PT", d("12000"), d("15000"), d("900"), d("50000"), d("6500"), d("80000"), d("18400")},
			{"PT-AC", d("10000"), d("25000.56"), d("1000.02"), d("75000"), d("6750"), d("100000"), d("18000")},
			{"PT-MA", d("5000"), d("12500"), d("625"), d("25000"), d("3000"), d("40000"), d("8800")},
		},
		NotSubject: d("100"), StampDuty: d("25"), TaxTotal: d("64000.02"), GrossTotal: d("513600.58"), Withholding: d("100"),
		Hash: "kLp0", Certificate: "9999", Other: "TB;PT00000000000000000000000;513500.58",
	}
	if err := q.Check(); err != nil {
		t.Fatal(err)
	}
	if got := q.String(); got != specExamples[0].content {
		t.Errorf("String:\n%s\nwant\n%s", got, specExamples[0].content)
	}
}

// TestBuild builds examples 2 to 5 and 7 of the specification from
// invoices that carry their data.
func TestBuild(t *testing.T) {
	d := invoice.MustDecimal
	party := func(vatID, country string) invoice.Party {
		return invoice.Party{VATID: vatID, Address: invoice.Address{CountryCode: country}}
	}
	vat := func(cat, base, tax string) invoice.VATBreakdown {
		return invoice.VATBreakdown{CategoryCode: cat, TaxableAmount: d(base), TaxAmount: d(tax)}
	}
	tests := []struct {
		name string
		inv  invoice.Invoice
		data Data
		want string
	}{
		{"2", invoice.Invoice{
			Number: "FS CDVF/12345", TypeCode: "FS", IssueDate: invoice.NewDate(2019, 8, 12), Currency: "EUR",
			Seller: party("PT123456789", "PT"), Buyer: party("", "PT"),
			VATBreakdown: []invoice.VATBreakdown{vat("S", "0.65", "0.15")},
			Totals:       invoice.Totals{TaxTotal: d("0.15"), GrandTotal: d("0.80")},
		}, Data{ValidationCode: "CDF7T5HD", Hash: "YhGV", Certificate: "9999", Other: "NU;0.80"}, specExamples[1].content},
		{"3", invoice.Invoice{
			Number: "PF G2019CB/145789", IssueDate: invoice.NewDate(2019, 1, 23),
			Seller: party("PT500000000", "PT"), Buyer: party("PT123456789", "PT"),
			VATBreakdown: []invoice.VATBreakdown{vat("ISE", "12345.34", "0"), vat("RED", "12532.65", "751.96"),
				vat("INT", "52789.00", "6862.57"), vat("NOR", "32425.69", "7457.91")},
			Totals: invoice.Totals{TaxTotal: d("15072.44"), GrandTotal: d("125165.12")},
		}, Data{ValidationCode: "HB6FT7RV", Hash: "r/fY", Certificate: "9999", DocumentType: "PF"}, specExamples[2].content},
		{"4", invoice.Invoice{
			Number: "GT G234CB/50987", IssueDate: invoice.NewDate(2019, 7, 20),
			Seller: party("PT500000000", "PT"), Buyer: party("PT123456789", "PT"),
			Totals: invoice.Totals{TaxTotal: d("0"), GrandTotal: d("0")},
		}, Data{ValidationCode: "GTVX4Y8B", Hash: "5uIg", Certificate: "9999", DocumentType: "GT", Region: "0"}, specExamples[3].content},
		{"5", invoice.Invoice{
			Number: "ABC BNH/4561", TypeCode: invoice.TypeCommercialInvoice, IssueDate: invoice.NewDate(2019, 5, 26),
			Seller: party("PT123456789", "PT"), Buyer: party("FR4443332215", "FR"),
			VATBreakdown: []invoice.VATBreakdown{vat("S", "100.00", "20.00")},
			Totals:       invoice.Totals{TaxTotal: d("20.00"), GrandTotal: d("120.00")},
		}, Data{ValidationCode: "DK5ZJ2HN", Hash: "YJRE", Certificate: "9999", Region: "FR"}, specExamples[4].content},
		{"7", invoice.Invoice{
			Number: "NF 19A/789145", TypeCode: "FT", IssueDate: invoice.NewDate(2019, 11, 24),
			Seller: party("PT500000000", "PT"), Buyer: party("PT123456789", "PT"),
			VATBreakdown: []invoice.VATBreakdown{vat("S", "50.00", "9.00"), vat("OUT", "1000.00", "0.00")},
			Totals:       invoice.Totals{TaxTotal: d("9.00"), GrandTotal: d("1059.00")},
		}, Data{ValidationCode: "JL9DS4TT", Hash: "d8/K", Certificate: "9999", Region: "PT-AC"},
			strings.Replace(specExamples[6].content, "H: ", "H:", 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Build(&tc.inv, &tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if got := q.String(); got != tc.want {
				t.Errorf("String:\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestBuildCIUSPT builds the QR codes of the CIUS-PT invoices of the tests
// of package internal/rules/pt: the four characters of the signature, the
// number of the certificate and the withholding taxes come from their
// structured notes.
func TestBuildCIUSPT(t *testing.T) {
	tests := []struct {
		file string
		data Data
		want string
	}{
		{"cius-pt.ubl.xml", Data{ValidationCode: "ZRSERIE7"}, "A:501234560*B:600012344*C:PT*D:FT*E:N*F:20261120*G:FT ZR2026/41*H:ZRSERIE7-41*" +
			"I1:PT*I3:40.00*I4:2.40*I7:300.00*I8:69.00*N:71.40*O:411.40*Q:Zr4q*R:9999"},
		// Without the structured notes of the certified program.
		{"cius-pt-nc.ubl.xml", Data{ValidationCode: "ZRSERIE7", Hash: "Zr4q", Certificate: "9999"}, "A:501234560*B:600012344*C:PT*D:NC*E:N*F:20261121*G:NC ZR2026/7*H:ZRSERIE7-7*" +
			"I1:PT*I2:115.00*N:0.00*O:115.00*Q:Zr4q*R:9999"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			xml, err := os.ReadFile("../internal/rules/pt/testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			inv, err := ubl.Parse(xml)
			if err != nil {
				t.Fatal(err)
			}
			q, err := Build(inv, &tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if got := q.String(); got != tc.want {
				t.Errorf("String:\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestBuildWithholding(t *testing.T) {
	inv := &invoice.Invoice{
		Number: "FT ZR2026/77", TypeCode: "FT", IssueDate: invoice.NewDate(2026, 11, 20), Currency: "EUR",
		Seller: invoice.Party{VATID: "PT501234560"}, Buyer: invoice.Party{VATID: "PT600012344", Address: invoice.Address{CountryCode: "PT"}},
		Notes: []invoice.Note{
			{Content: "#HASHCODE@ATCERTIFIEDPROGRAM#" + strings.Repeat("Qx7mAbCdEf", 4) + "#"},
			{Content: "#NUMBER@ATCERTIFIEDPROGRAM#1234/AT#"},
			{Content: "#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-001#12.50#"},
			{Content: "#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-002#7.50#"},
		},
		VATBreakdown: []invoice.VATBreakdown{{CategoryCode: "AA", TaxableAmount: invoice.MustDecimal("100"), TaxAmount: invoice.MustDecimal("6")}},
		Totals:       invoice.Totals{TaxTotal: invoice.MustDecimal("6"), GrandTotal: invoice.MustDecimal("106")},
	}
	q, err := Build(inv, &Data{ValidationCode: "ZRSERIE7", Other: "MB;21312;123456789;86.00"})
	if err != nil {
		t.Fatal(err)
	}
	want := "A:501234560*B:600012344*C:PT*D:FT*E:N*F:20261120*G:FT ZR2026/77*H:ZRSERIE7-77*" +
		"I1:PT*I3:100.00*I4:6.00*N:6.00*O:106.00*P:20.00*Q:QQQQ*R:1234*S:MB;21312;123456789;86.00"
	if got := q.String(); got != want {
		t.Errorf("String:\n%s\nwant\n%s", got, want)
	}
}

func TestBuildErrors(t *testing.T) {
	valid := func() *invoice.Invoice {
		return &invoice.Invoice{
			Number: "FT A/1", TypeCode: "380", IssueDate: invoice.NewDate(2026, 11, 26), Currency: "EUR",
			Seller:       invoice.Party{VATID: "PT501234560"},
			VATBreakdown: []invoice.VATBreakdown{{CategoryCode: "S", TaxableAmount: invoice.MustDecimal("10"), TaxAmount: invoice.MustDecimal("2.30")}},
			Totals:       invoice.Totals{TaxTotal: invoice.MustDecimal("2.30"), GrandTotal: invoice.MustDecimal("12.30")},
		}
	}
	data := Data{ValidationCode: "ABCD1234", Hash: "AbC/", Certificate: "1"}
	q, err := Build(valid(), &data)
	if err != nil {
		t.Fatal(err)
	}
	if want := "A:501234560*B:999999990*C:Desconhecido*D:FT*E:N*F:20261126*G:FT A/1*H:ABCD1234-1*I1:PT*I7:10.00*I8:2.30*N:2.30*O:12.30*Q:AbC/*R:1"; q.String() != want {
		t.Errorf("String:\n%s\nwant\n%s", q.String(), want)
	}

	tests := []struct {
		name   string
		edit   func(*invoice.Invoice, *Data)
		fields []string
	}{
		{"no validation code", func(_ *invoice.Invoice, d *Data) { d.ValidationCode = "" }, []string{"H"}},
		{"short validation code", func(_ *invoice.Invoice, d *Data) { d.ValidationCode = "ABC" }, []string{"H"}},
		{"number without slash", func(inv *invoice.Invoice, _ *Data) { inv.Number = "FT-1" }, []string{"G", "H"}},
		{"foreign seller", func(inv *invoice.Invoice, _ *Data) { inv.Seller.VATID = "FR12345678901" }, []string{"A"}},
		{"category of the Canary Islands", func(inv *invoice.Invoice, _ *Data) { inv.VATBreakdown[0].CategoryCode = "L" }, []string{"I1"}},
		{"type code 384", func(inv *invoice.Invoice, _ *Data) { inv.TypeCode = "384" }, []string{"D"}},
		{"no hash", func(_ *invoice.Invoice, d *Data) { d.Hash = "" }, []string{"Q"}},
		{"no total", func(inv *invoice.Invoice, _ *Data) { inv.Totals.GrandTotal = invoice.Decimal{} }, []string{"O"}},
		{"negative total", func(inv *invoice.Invoice, _ *Data) { inv.Totals.GrandTotal = invoice.MustDecimal("-1") }, []string{"O"}},
		{"bad withholding", func(inv *invoice.Invoice, _ *Data) {
			inv.Notes = []invoice.Note{{Content: "#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-001#douze#"}}
		}, []string{"P"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv, d := valid(), data
			tc.edit(inv, &d)
			q, err := Build(inv, &d)
			if q == nil {
				t.Fatal("no content")
			}
			if got := fields(err); !slices.Equal(got, tc.fields) {
				t.Errorf("fields %v, want %v: %v", got, tc.fields, err)
			}
		})
	}

	inv := valid()
	inv.Currency = "USD"
	if _, err := Build(inv, &data); err == nil || !strings.Contains(err.Error(), "euros") {
		t.Errorf("an invoice in USD: %v", err)
	}
	if _, err := Build(nil, &data); err == nil {
		t.Error("no error without invoice")
	}
}

// TestParseRejects changes example 2 of the specification, one field at a
// time, and checks the field that Parse reports.
func TestParseRejects(t *testing.T) {
	base := specExamples[1].content
	tests := []struct {
		name, old, new string
		fields         []string
	}{
		{"NIF check digit", "A:123456789", "A:123456780", []string{"A"}},
		{"NIF of eight digits", "A:123456789", "A:12345678", []string{"A"}},
		{"NIF with prefix", "A:123456789", "A:PT123456789", []string{"A"}},
		{"buyer missing", "B:999999990*", "", []string{"B"}},
		{"buyer too long", "B:999999990", "B:" + strings.Repeat("9", 31), []string{"B"}},
		{"country name", "C:PT", "C:Portugal", []string{"C"}},
		{"type of one letter", "D:FS", "D:F", []string{"D"}},
		{"status in lower case", "E:N", "E:n", []string{"E"}},
		{"30 February", "F:20190812", "F:20190230", []string{"F"}},
		{"ISO date", "F:20190812", "F:2019-08-12", []string{"F"}},
		{"identification without slash", "G:FS CDVF/12345", "G:FS CDVF-12345", []string{"G"}},
		{"identification with a trailing space", "G:FS CDVF/12345", "G:FS CDVF/12345 ", []string{"G"}},
		{"validation code of seven characters", "H:CDF7T5HD-12345", "H:CDF7T5H-12345", []string{"H"}},
		{"validation code with a symbol", "H:CDF7T5HD-12345", "H:CDF7T5H$-12345", []string{"H"}},
		{"ATCUD without hyphen", "H:CDF7T5HD-12345", "H:CDF7T5HD12345", []string{"H"}},
		{"ATCUD of another number", "H:CDF7T5HD-12345", "H:CDF7T5HD-12346", []string{"H"}},
		{"ATCUD with prefix", "H:CDF7T5HD-12345", "H:ATCUD:CDF7T5HD-12345", []string{"H"}},
		{"fiscal space", "I1:PT", "I1:PT-LX", []string{"I1"}},
		{"fiscal space missing", "I1:PT*", "", []string{"I1"}},
		{"0 with amounts", "I1:PT", "I1:0", []string{"I7", "I8"}},
		{"decimal comma", "I7:0.65", "I7:0,65", []string{"I7"}},
		{"one decimal", "I7:0.65", "I7:0.6", []string{"I7"}},
		{"leading zero", "I7:0.65", "I7:00.65", []string{"I7"}},
		{"negative amount", "I8:0.15", "I8:-0.15", []string{"I8"}},
		{"empty amount", "I8:0.15", "I8:", []string{"I8"}},
		{"fiscal space twice", "I8:0.15*", "I8:0.15*J1:PT*", []string{"J1"}},
		{"amounts without fiscal space", "I8:0.15*", "I8:0.15*J7:1.00*", []string{"J1"}},
		{"0 in J1", "I8:0.15*", "I8:0.15*J1:0*", []string{"J1"}},
		{"amount too long", "O:0.80", "O:12345678901234.00", []string{"O"}},
		{"total missing", "N:0.15*", "", []string{"N"}},
		{"total out of order", "N:0.15*O:0.80", "O:0.80*N:0.15", []string{"N"}},
		{"field twice", "R:9999", "R:9999*R:9999", []string{"R"}},
		{"three characters of the signature", "Q:YhGV", "Q:YhG", []string{"Q"}},
		{"hyphen in the signature", "Q:YhGV", "Q:Yh-V", []string{"Q"}},
		{"certificate of five digits", "R:9999", "R:99999", []string{"R"}},
		{"certificate with a letter", "R:9999", "R:99a9", []string{"R"}},
		{"other information too long", "S:NU;0.80", "S:" + strings.Repeat("x", 66), []string{"S"}},
		{"empty other information", "S:NU;0.80", "S:", []string{"S"}},
		{"unknown code", "S:NU;0.80", "T:NU;0.80", []string{""}},
		{"part without colon", "*S:NU;0.80", "*S:NU;0.80*NU", []string{""}},
		{"not UTF-8", "S:NU;0.80", "S:NU;\xff", []string{"S"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := strings.Replace(base, tc.old, tc.new, 1)
			if s == base {
				t.Fatalf("%q not found", tc.old)
			}
			_, err := Parse(s)
			if got := fields(err); !slices.Equal(got, tc.fields) {
				t.Errorf("fields %v, want %v: %v", got, tc.fields, err)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	q, err := Parse(specExamples[1].content)
	if err != nil {
		t.Fatal(err)
	}
	q.Other = "a*b"
	q.Spaces[2].NormalBase = invoice.MustDecimal("1")
	err = q.Check()
	if got, want := fields(err), []string{"K1", "S"}; !slices.Equal(got, want) {
		t.Errorf("fields %v, want %v: %v", got, want, err)
	}
	if _, err := q.Symbol(); err == nil {
		t.Error("Symbol encodes an invalid content")
	}
	var fe *FieldError
	if !errors.As(err, &fe) || !strings.HasPrefix(fe.Error(), "atcud: field K1: ") {
		t.Errorf("error %v", err)
	}
}

func TestCode(t *testing.T) {
	c, err := New("CSDF7T5H", "FT AB2019/0035")
	if err != nil || c.String() != "CSDF7T5H-0035" {
		t.Errorf("New: %v, %v", c, err)
	}
	for _, s := range []string{"CSDF7T5H-0035", "ATCUD:CSDF7T5H-0035"} {
		if c, err := ParseCode(s); err != nil || c != (Code{"CSDF7T5H", "0035"}) {
			t.Errorf("ParseCode(%q): %v, %v", s, c, err)
		}
	}
	for _, tc := range []struct{ code, id, field string }{
		{"CSDF7T5", "FT AB2019/0035", "H"},
		{"CSDF7T5H", "FT AB2019-0035", "G"},
		{"CSDF7T5H", "FT AB2019/", "G"},
		{"CSDF-T5H", "FT AB2019/35", "H"},
		{"CSDF7T5H" + strings.Repeat("X", 60), "FT A/35", "H"},
	} {
		if _, err := New(tc.code, tc.id); !slices.Equal(fields(err), []string{tc.field}) {
			t.Errorf("New(%q, %q): %v", tc.code, tc.id, err)
		}
	}
	for _, s := range []string{"CSDF7T5H0035", "CSDF7T5H-", "CSDF7T5H-00a5", "CSDF7T5H-0035-1", "CSDF7T5-0035"} {
		if _, err := ParseCode(s); !slices.Equal(fields(err), []string{"H"}) {
			t.Errorf("ParseCode(%q): %v", s, err)
		}
	}
}

func TestHashChars(t *testing.T) {
	sig := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=="
	if got, err := HashChars(sig); err != nil || got != "AKUe" {
		t.Errorf("HashChars: %q, %v", got, err)
	}
	for _, s := range []string{sig[:30], "ABCDEFGHIJ-LMNOPQRSTUVWXYZabcdefghij"} {
		if _, err := HashChars(s); err == nil {
			t.Errorf("HashChars(%q): no error", s)
		}
	}
}

// FuzzParse checks that Parse does not panic, and that a content it
// accepts is written back as is and encoded.
func FuzzParse(f *testing.F) {
	for _, ex := range specExamples {
		f.Add(ex.content)
	}
	f.Add("A:1*B:2")
	f.Fuzz(func(t *testing.T, s string) {
		q, err := Parse(s)
		if err != nil {
			return
		}
		if got := q.String(); got != s {
			t.Fatalf("String:\n%q\nParse read\n%q", got, s)
		}
		if err := q.Check(); err != nil {
			t.Fatalf("Check: %v", err)
		}
		if _, err := q.Symbol(); err != nil {
			t.Fatalf("Symbol: %v", err)
		}
	})
}
