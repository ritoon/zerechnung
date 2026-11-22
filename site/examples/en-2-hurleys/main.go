// Title: A Kilkenny hurley maker invoices a public body over Peppol
// Tags: Peppol BIS Billing 3.0, UBL, EN 16931, Irish VAT 23%
//
// Nore Valley Hurleys turns ash planks into hurleys in a workshop outside
// Kilkenny. This autumn it equipped the schools of the Nore and Barrow
// Education and Training Board for the hurling and camogie season: senior
// and junior hurleys, a few hundred sliotars, and the repair of sticks
// cracked during the first weeks of training.
//
// An education and training board is a contracting authority, and like
// other Irish public bodies it receives EN 16931 e-invoices, many of them
// through the Peppol network. ZeRechnung writes the invoice in UBL following
// Peppol BIS Billing 3.0, and checks it against the Peppol rules before it
// is handed to an access point.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Describe the supplier and the board
	// On Peppol, each party has an electronic address (BT-34, BT-49): its
	// participant identifier on the network. Here both use the scheme 9935,
	// an Irish VAT number; the board has one too.
	hurleys := invoice.Party{
		Name:  "Nore Valley Hurleys Ltd",
		VATID: "IE7046192G",
		Address: invoice.Address{Line1: "Unit 4, Bennettsbridge Road", City: "Kilkenny",
			PostCode: "R95 FX62", CountryCode: "IE"},
		ElectronicAddress: invoice.Identifier{ID: "IE7046192G", Scheme: invoice.SchemeIEVAT},
	}
	board := invoice.Party{
		Name:  "Nore and Barrow Education and Training Board",
		VATID: "IE4180736U",
		Address: invoice.Address{Line1: "Barrow House, Ormonde Road", City: "Kilkenny",
			PostCode: "R95 A39T", CountryCode: "IE"},
		ElectronicAddress: invoice.Identifier{ID: "IE4180736U", Scheme: invoice.SchemeIEVAT},
	}

	// ## Add the Peppol references
	// The Peppol profile sets the specification identifier (BT-24), and
	// BT-23 names the Peppol billing process. Peppol requires a buyer
	// reference or an order reference: the board's purchase order number
	// goes in BT-13, and the reference of the ordering department in BT-10.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfilePeppol,
		BusinessProcess:        invoice.ProcessPeppolBilling,
		Number:                 "NVH-2026-0214",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 16),
		Currency:               "EUR",
		BuyerReference:         "SCHOOLS-SPORT",
		PurchaseOrderReference: "PO-2026-04417",
		Seller:                 hurleys,
		Buyer:                  board,
		DeliveryDate:           invoice.NewDate(2026, time.November, 9),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IE29AIBK93115212345678",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Payment within 30 days of receipt of invoice.",
			DueDate:     invoice.NewDate(2026, time.December, 16),
		}},
	}

	// ## List the hurleys, the sliotars and the repairs
	// Sports equipment carries the standard rate of 23%, and so does the
	// repair service. H87 is the UN/ECE code for a piece, C62 for one unit.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Ash hurley, handcrafted, sizes 30 to 36"},
		Quantity: d("40"), UnitCode: "H87", NetPrice: d("38.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Junior ash hurley, sizes 18 to 28"},
		Quantity: d("60"), UnitCode: "H87", NetPrice: d("27.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Sliotar, leather, match grade"},
		Quantity: d("200"), UnitCode: "H87", NetPrice: d("6.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Hurley repair: splice and metal band"},
		Quantity: d("25"), UnitCode: "H87", NetPrice: d("8.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Delivery to the board's schools"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("60.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}}

	// ## Write the UBL invoice
	// Calculate works out the line amounts, the totals and the VAT breakdown
	// with exact decimals. ubl.Marshal writes a UBL 2.1 Invoice.
	inv.Calculate()
	xml, err := ubl.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}

	// ## Validate with the Peppol rules
	// The Peppol rules apply on their own to an invoice that declares Peppol
	// in BT-24; naming them makes the check explicit. They come on top of
	// the UBL 2.1 schema and the EN 16931 rules, and check, among others,
	// the electronic addresses, the references and the code lists.
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.Peppol})
	if !report.Valid() {
		log.Fatal(report)
	}

	// ## Read the invoice back
	// ubl.Parse reads the UBL document into the same model, as the board's
	// finance system would.
	back, err := ubl.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(back.Profile, "·", back.Number)
	to := back.Buyer.ElectronicAddress
	fmt.Println("To:", back.Buyer.Name, "·", to.Scheme+":"+to.ID)
	fmt.Println("Order:", back.PurchaseOrderReference)
	vat := back.VATBreakdown[0]
	fmt.Printf("VAT %s%% on €%s: €%s\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	fmt.Printf("Amount due: €%s\n", back.Totals.DuePayable)
	fmt.Println("Findings:", len(report.Findings))
}

// Output:
// PEPPOL · NVH-2026-0214
// To: Nore and Barrow Education and Training Board · 9935:IE4180736U
// Order: PO-2026-04417
// VAT 23% on €4730.00: €1087.90
// Amount due: €5817.90
// Findings: 0
