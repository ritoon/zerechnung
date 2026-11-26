package atcud_test

import (
	"bytes"
	"fmt"

	"github.com/ritoon/zerechnung/atcud"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/qrcode"
)

// This example builds the ATCUD and the QR code of an invoice, then draws
// the symbol in SVG for the software that renders the document.
func ExampleBuild() {
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Number: "FT ZR2026/41", TypeCode: "FT", IssueDate: invoice.NewDate(2026, 11, 20), Currency: "EUR",
		Seller: invoice.Party{VATID: "PT501234560"},
		Buyer:  invoice.Party{VATID: "PT600012344", Address: invoice.Address{CountryCode: "PT"}},
		VATBreakdown: []invoice.VATBreakdown{
			{CategoryCode: "S", Rate: d("23"), TaxableAmount: d("300.00"), TaxAmount: d("69.00")},
			{CategoryCode: "RED", Rate: d("6"), TaxableAmount: d("40.00"), TaxAmount: d("2.40")},
		},
		Totals: invoice.Totals{TaxTotal: d("71.40"), GrandTotal: d("411.40")},
	}
	q, err := atcud.Build(inv, &atcud.Data{ValidationCode: "ZRSERIE7", Hash: "Zr4q", Certificate: "9999"})
	if err != nil {
		panic(err)
	}
	fmt.Println("ATCUD:" + q.ATCUD)
	fmt.Println(q)

	sym, err := q.Symbol()
	if err != nil {
		panic(err)
	}
	var svg bytes.Buffer
	if err := sym.SVG(&svg, &qrcode.Render{Scale: 2}); err != nil {
		panic(err)
	}
	fmt.Printf("version %d-%s, %d modules\n", sym.Version(), sym.Level(), sym.Size())
	// Output:
	// ATCUD:ZRSERIE7-41
	// A:501234560*B:600012344*C:PT*D:FT*E:N*F:20261120*G:FT ZR2026/41*H:ZRSERIE7-41*I1:PT*I3:40.00*I4:2.40*I7:300.00*I8:69.00*N:71.40*O:411.40*Q:Zr4q*R:9999
	// version 9-M, 53 modules
}

// This example checks the QR code of a received invoice: each error names
// the field.
func ExampleParse() {
	_, err := atcud.Parse("A:123456789*B:999999990*C:PT*D:FS*E:N*F:20190812*G:FS CDVF/12345*H:CDF7T5HD-12346*" +
		"I1:PT*I7:0,65*I8:0.15*N:0.15*O:0.80*Q:YhGV*R:9999")
	fmt.Println(err)
	// Output:
	// atcud: field I7: "0,65" is not an amount in digits with two decimals after a point, without thousands separator nor leading zero (section 3 c)
	// atcud: field H: the sequential number 12346 of the ATCUD is not 12345, the number of the document identification G (Portaria n.º 195/2020, article 3.º, n.º 3)
}

func ExampleNew() {
	c, err := atcud.New("CSDF7T5H", "FT AB2019/0035")
	if err != nil {
		panic(err)
	}
	fmt.Println(c)
	// Output: CSDF7T5H-0035
}
