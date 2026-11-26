package qrbill_test

import (
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/qrbill"
)

// A Swiss invoice payable to a QR-IBAN with a QR reference: Build makes
// the content of its Swiss QR Code, and the symbol is drawn with the Swiss
// cross.
func ExampleBuild() {
	inv := &invoice.Invoice{
		Number:    "2026-123",
		IssueDate: invoice.MustDate("2026-11-26"),
		Currency:  "CHF",
		Seller: invoice.Party{Name: "Robert Schneider AG",
			Address: invoice.Address{Line1: "Rue du Lac 1268", PostCode: "2501", City: "Biel", CountryCode: "CH"}},
		Buyer: invoice.Party{Name: "Pia-Maria Rutschmann-Schnyder",
			Address: invoice.Address{Line1: "Grosse Marktgasse 28", PostCode: "9400", City: "Rorschach", CountryCode: "CH"}},
		PaymentMeans: []invoice.PaymentMeans{{TypeCode: "58", PayeeIBAN: "CH44 3199 9123 0008 8901 2"}},
		Totals:       invoice.Totals{DuePayable: invoice.MustDecimal("1949.75")},
	}
	ref, err := qrbill.NewQRReference("2026123") // the reference scheme is yours, or your bank's
	if err != nil {
		log.Fatal(err)
	}
	bill, err := qrbill.Build(inv, &qrbill.Data{Reference: ref})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(bill.ReferenceType, bill.Reference, bill.Amount, bill.Currency)
	fmt.Println(bill.BillInformation)

	sym, err := bill.Symbol() // level M, version 25 at most
	if err != nil {
		log.Fatal(err)
	}
	// Printed at 46 × 46 mm without the quiet zone, in the payment part.
	if err := sym.SVG(io.Discard, qrbill.Render(sym, 4)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("version %d, level %s\n", sym.Version(), sym.Level())
	// Output:
	// QRR 000000000000000000020261238 1949.75 CHF
	// //S1/10/2026-123/11/261126
	// version 11, level M
}

// Parse reads the content of a Swiss QR Code that a debtor scans, and
// names each line that breaks the guidelines.
func ExampleParse() {
	content := "SPC\n0200\n1\nCH5800791123000889012\n" +
		"S\nMax Muster & Söhne\nMusterstrasse\n123\n8000\nSeldwyla\nCH\n\n\n\n\n\n\n\n" +
		"199.95\nCHF\nS\nSarah Beispiel\nMusterstrasse\n1\n8000\nSeldwyla\nCH\n" +
		"QRR\n210000000003139471430009017\n\nEPD"
	_, err := qrbill.Parse(content, qrbill.Guidelines24)
	var fe *qrbill.FieldError
	if errors.As(err, &fe) {
		fmt.Println(fe.Line, fe.Element)
	}
	fmt.Println(err)
	// Output:
	// 28 Tp
	// qrbill: line 28 (Tp): a QR reference needs a QR-IBAN, whose institution identification is between 30000 and 31999 (chapter 4.3.2)
}
