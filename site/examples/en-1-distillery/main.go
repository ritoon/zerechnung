// Title: A Cork distillery invoices a Dublin pub
// Tags: Factur-X, EN 16931, Irish VAT 23%
//
// Inishcarra Distillers, on the banks of the Lee west of Cork, sends an
// order up to the Brazen Lantern, a pub on Camden Street in Dublin: single
// pot still whiskey, a sherry-finished single malt, a cask strength bottling
// and a few bottles of poitín. The distillery's accounts package already
// prints the invoice as a PDF; ZeRechnung adds the XML and the Factur-X
// metadata, and checks the invoice against EN 16931 before writing
// anything.
//
// Spirits carry the standard rate of Irish VAT, 23%, and the prices already
// include the Alcohol Products Tax paid when the bottles left the bonded
// warehouse.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Read the invoice PDF
	// The starting point is the PDF your software already produces. Ideally a
	// PDF/A-3, but any PDF will do: ZeRechnung does not change its bytes, it
	// adds the attachment with an incremental update.
	pdf, err := os.ReadFile("invoice.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Describe the seller and the buyer
	// Both parties are VAT-registered, so their Irish VAT numbers go on the
	// invoice (BT-31, BT-48). The distillery adds its company registration
	// number, as Irish companies print it on their business documents
	// (BT-33). An Irish address ends with its Eircode, the postcode.
	distillery := invoice.Party{
		Name:        "Inishcarra Distillers Ltd",
		Description: "Registered in Ireland No. 612345",
		VATID:       "IE3254871E",
		Address: invoice.Address{Line1: "Mill Road", City: "Inishcarra", PostCode: "P31 X2F7",
			CountrySubdivision: "Co. Cork", CountryCode: "IE"},
	}
	pub := invoice.Party{
		Name:  "Brazen Lantern Taverns Ltd",
		VATID: "IE9471263R",
		Address: invoice.Address{Line1: "14 Camden Street Lower", City: "Dublin 2",
			PostCode: "D02 YX48", CountryCode: "IE"},
	}

	// ## Add the order details and payment terms
	// The pub's order number goes in BT-13, the note tells the buyer that the
	// excise duty is paid, and the payment instructions give the IBAN of the
	// distillery for a SEPA credit transfer.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		Number:                 "INV-26-0571",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		PurchaseOrderReference: "BL-2026-118",
		Seller:                 distillery,
		Buyer:                  pub,
		DeliveryDate:           invoice.NewDate(2026, time.November, 18),
		Notes: []invoice.Note{
			{Content: "Alcohol Products Tax paid and included in the prices."},
		},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IE29AIBK93115212345678",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Payment within 30 days by bank transfer.",
			DueDate:     invoice.NewDate(2026, time.December, 20),
		}},
	}

	// ## List the bottles
	// Each line has a quantity, a unit (UN/ECE code: H87 for a piece, C62
	// for one unit), a net price and its VAT rate. The delivery is charged
	// at the same 23% as the spirits.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Single Pot Still Irish Whiskey, 12 Year Old, 70 cl"},
		Quantity: d("24"), UnitCode: "H87", NetPrice: d("46.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Single Malt, Oloroso Sherry Cask Finish, 70 cl"},
		Quantity: d("12"), UnitCode: "H87", NetPrice: d("58.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Single Pot Still, Cask Strength, 70 cl"},
		Quantity: d("6"), UnitCode: "H87", NetPrice: d("79.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Irish Poitín, 70 cl"},
		Quantity: d("12"), UnitCode: "H87", NetPrice: d("32.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Delivery to Dublin"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("45.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
	}}

	// ## Create the Factur-X invoice
	// Calculate works out the line amounts, the totals and the VAT breakdown
	// with exact decimals. With the default options, the invoice is then
	// checked against the XML schema, the EN 16931 business rules and the
	// Factur-X schematron of its profile. An invalid invoice is refused:
	// Create returns the findings and writes nothing.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("INV-26-0571.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Read back and check the result
	// The PDF reads like any invoice you receive. Check validates the PDF
	// container, the XMP metadata and the XML, with the same rules.
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Metadata.ConformanceLevel)
	for _, vat := range back.VATBreakdown {
		fmt.Printf("VAT %s%% on €%s: €%s\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	}
	t := back.Totals
	fmt.Printf("Total excl. VAT: €%s · incl. VAT: €%s\n", t.TaxBasisTotal, t.GrandTotal)

	report, _, err := zerechnung.Check(out, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Errors:", len(report.Errors()))
}

// Output:
// factur-x.xml · EN 16931
// VAT 23% on €2715.00: €624.45
// Total excl. VAT: €2715.00 · incl. VAT: €3339.45
// Errors: 0
