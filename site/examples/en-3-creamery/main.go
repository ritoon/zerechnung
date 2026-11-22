// Title: A Kerry creamery ships butter and cheddar to Germany
// Tags: CII, EN 16931, intra-Community supply, VAT 0%
//
// Slieve Mish Co-operative Creamery churns butter and makes cheddar from the
// milk of grass-fed herds on the Dingle Peninsula. A refrigerated trailer
// leaves Castlemaine for the central warehouse of Frischmarkt Hanse, a
// supermarket chain in northern Germany, with salted and unsalted butter and
// two cheddars.
//
// A supply of goods to a VAT-registered business in another member state is
// an intra-Community supply: no Irish VAT is charged, and the invoice says
// why. The chain's purchase ledger reads EN 16931 invoices in CII, the XML
// syntax of ZUGFeRD that XRechnung also accepts, so ZeRechnung writes the
// invoice as CII XML and checks the rules that apply to such a supply.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Describe the creamery, the chain and the warehouse
	// An intra-Community supply needs the VAT numbers of both parties
	// (BT-31, BT-48): the German one shows that the buyer is a business
	// registered for VAT in another member state. The goods go to the
	// chain's warehouse in Bremen, the deliver-to address (BG-15), whose
	// country is mandatory for such a supply.
	creamery := invoice.Party{
		Name:  "Slieve Mish Co-operative Creamery Ltd",
		VATID: "IE5218437A",
		Address: invoice.Address{Line1: "Creamery Road", City: "Castlemaine", PostCode: "V93 K2P6",
			CountrySubdivision: "Co. Kerry", CountryCode: "IE"},
	}
	chain := invoice.Party{
		Name:  "Frischmarkt Hanse GmbH & Co. KG",
		VATID: "DE136902056",
		Address: invoice.Address{Line1: "Am Sandtorkai 40", City: "Hamburg", PostCode: "20457",
			CountryCode: "DE"},
	}
	warehouse := &invoice.Party{
		Name: "Frischmarkt Hanse, Zentrallager Bremen",
		Address: invoice.Address{Line1: "Industriestraße 12", City: "Bremen", PostCode: "28199",
			CountryCode: "DE"},
	}

	// ## State the VAT exemption
	// The VAT category is K, intra-Community supply, at 0%. The breakdown
	// carries the reason for the exemption as a VATEX code and as text
	// (BT-121, BT-120); Calculate keeps them when it fills in the amounts.
	// The delivery date (BT-72) tells in which period the supply took place.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		Number:                 "SMC-EX-26-0388",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 19),
		Currency:               "EUR",
		PurchaseOrderReference: "4500987123",
		Seller:                 creamery,
		Buyer:                  chain,
		ShipTo:                 warehouse,
		DeliveryDate:           invoice.NewDate(2026, time.November, 17),
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReasonCode: invoice.VATEXIntraCommunity,
			ExemptionReason:     "Intra-Community supply, Article 138 of the VAT Directive",
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IE29AIBK93115212345678",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Payment within 45 days by SEPA credit transfer.",
			DueDate:     invoice.NewDate(2027, time.January, 3),
		}},
	}

	// ## List the butter and the cheese
	// Every line carries the category K and the rate 0%. Butter is invoiced
	// by the 250 g pack (H87, a piece), the block cheddar by the kilogram
	// (KGM).
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Salted creamery butter, 250 g"},
		Quantity: d("9600"), UnitCode: "H87", NetPrice: d("1.95"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Unsalted creamery butter, 250 g"},
		Quantity: d("2400"), UnitCode: "H87", NetPrice: d("1.95"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Mature white cheddar, 2.5 kg block"},
		Quantity: d("500"), UnitCode: "KGM", NetPrice: d("7.40"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Vintage red cheddar, 200 g"},
		Quantity: d("3000"), UnitCode: "H87", NetPrice: d("2.10"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Add the haulage as a charge
	// The refrigerated transport to Bremen is a document level charge
	// (BG-21) with the reason code FC, freight. It is part of the same
	// supply, with the same category K.
	inv.AllowancesCharges = []invoice.AllowanceCharge{{
		Charge: true, Reason: "Refrigerated haulage to Bremen", ReasonCode: "FC",
		Amount: d("1450.00"), VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Write and validate the CII invoice
	// Calculate works out the line amounts, the totals and the VAT breakdown.
	// cii.Marshal writes the invoice at the EN 16931 profile; validate.XML
	// checks the CII schema and the EN 16931 rules, among them the BR-IC
	// rules of the intra-Community supply: both VAT numbers, the deliver-to
	// country, the delivery date, the exemption reason, no VAT amount.
	inv.Calculate()
	xml, err := cii.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	report := validate.XML(xml, nil)
	if !report.Valid() {
		log.Fatal(report)
	}

	// ## Read the invoice back
	// cii.Parse reads the XML into the same model, as the chain's system
	// would.
	back, err := cii.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	vat := back.VATBreakdown[0]
	fmt.Printf("VAT %s %s%% on €%s: €%s\n", vat.CategoryCode, vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	fmt.Println("Exemption:", vat.ExemptionReasonCode, "·", vat.ExemptionReason)
	fmt.Println("Deliver to:", back.ShipTo.Address.City+",", back.ShipTo.Address.CountryCode)
	t := back.Totals
	fmt.Printf("Lines: €%s · charges: €%s\n", t.LineTotal, t.ChargeTotal)
	fmt.Printf("Amount due: €%s\n", t.DuePayable)
	fmt.Println("Errors:", len(report.Errors()))
}

// Output:
// VAT K 0% on €34850.00: €0.00
// Exemption: VATEX-EU-IC · Intra-Community supply, Article 138 of the VAT Directive
// Deliver to: Bremen, DE
// Lines: €33400.00 · charges: €1450.00
// Amount due: €34850.00
// Errors: 0
