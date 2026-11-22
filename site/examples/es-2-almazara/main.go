// Title: Una almazara de Jaén factura el aceite nuevo a Róterdam por Peppol
// Tags: Peppol BIS Billing 3.0, UBL, Entrega intracomunitaria, IVA 0 %
//
// La almazara cooperativa Cerro del Lobo, en Mancha Real (Jaén), moltura
// las aceitunas de variedad picual que le traen sus socios. En noviembre
// sale el primer aceite de la campaña, el de cosecha temprana, y una parte
// viaja a Róterdam: el mayorista Groothandel Bakker & El Amrani lo reparte
// entre tiendas y restaurantes de los Países Bajos.
//
// El mayorista recibe sus facturas por la red Peppol, como muchas empresas
// neerlandesas. La factura se escribe en Peppol BIS Billing 3.0, la
// especificación de esa red, en sintaxis UBL, y se valida con sus reglas.
// Como entrega intracomunitaria, va sin IVA español.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Identificar a las partes en la red Peppol
	// En Peppol, cada parte tiene una dirección electrónica (BT-34, BT-49)
	// con su esquema. La almazara usa su NIF-IVA (esquema 9920, el de la
	// Agencia Tributaria); el mayorista, su número de la Cámara de Comercio
	// neerlandesa, el KvK-nummer (esquema 0106). Las reglas Peppol
	// comprueban el formato de estos identificadores según su esquema.
	almazara := invoice.Party{
		Name:  "Almazara Cerro del Lobo, S. Coop. And.",
		VATID: "ESF23619471",
		Address: invoice.Address{
			Line1: "Camino de la Estación, s/n", PostCode: "23100", City: "Mancha Real",
			CountrySubdivision: "Jaén", CountryCode: "ES",
		},
		Contact: &invoice.Contact{
			PersonName: "Antonio Expósito", Email: "ventas@cerrodellobo.example",
		},
		ElectronicAddress: invoice.Identifier{ID: "ESF23619471", Scheme: "9920"},
	}
	mayorista := invoice.Party{
		Name:              "Groothandel Bakker & El Amrani B.V.",
		VATID:             "NL853164721B01",
		LegalRegistration: invoice.Identifier{ID: "24381567", Scheme: "0106"},
		Address: invoice.Address{
			Line1: "Maashaven Oostzijde 12", PostCode: "3072 HE", City: "Rotterdam",
			CountryCode: "NL",
		},
		ElectronicAddress: invoice.Identifier{ID: "24381567", Scheme: "0106"},
	}

	// ## Preparar la cabecera Peppol
	// Peppol pide el proceso de negocio (BT-23) y una referencia del
	// comprador o de su pedido (BT-10 o BT-13), y solo admite una nota en la
	// cabecera. El perfil ProfilePeppol escribe el identificador de la
	// especificación (BT-24). La exención del artículo 25 de la Ley del IVA
	// va en el desglose, con el almacén de destino y la fecha de entrega.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfilePeppol,
		BusinessProcess:        "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0",
		Number:                 "CL-2026-0871",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 19),
		Currency:               "EUR",
		BuyerReference:         "INKOOP-OLIE",
		PurchaseOrderReference: "PO-77310",
		Seller:                 almazara,
		Buyer:                  mayorista,
		DeliveryDate:           invoice.NewDate(2026, time.November, 17),
		ShipTo: &invoice.Party{
			Name: "Bakker & El Amrani, almacén de Waalhaven",
			Address: invoice.Address{
				Line1: "Waalhaven Zuidzijde 40", PostCode: "3089 JH", City: "Rotterdam",
				CountryCode: "NL",
			},
		},
		Notes: []invoice.Note{
			{Content: "Aceite de la campaña 2026/2027, extraído en frío el día de la recogida."},
		},
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReason:     "Entrega intracomunitaria exenta, art. 25 Ley 37/1992.",
			ExemptionReasonCode: "VATEX-EU-IC",
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:         invoice.PaymentSEPACreditTransfer,
			PayeeIBAN:        "ES9121000418450200051332",
			PayeeAccountName: "Almazara Cerro del Lobo",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Transferencia a 30 días.",
			DueDate:     invoice.NewDate(2026, time.December, 19),
		}},
	}

	// ## Listar los aceites
	// AOVE es el aceite de oliva virgen extra. El de los bag-in-box se
	// factura por litros (código UN/ECE LTR), las botellas y las latas por
	// piezas (H87). Cada línea lleva la categoría K y un tipo del 0 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "AOVE picual de cosecha temprana, bag-in-box de 5 l"},
		Quantity: d("600"), UnitCode: "LTR", NetPrice: d("9.30"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "AOVE DOP Sierra Mágina, botella de 500 ml"},
		Quantity: d("960"), UnitCode: "H87", NetPrice: d("6.20"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "AOVE picual, lata de 2,5 l"},
		Quantity: d("200"), UnitCode: "H87", NetPrice: d("24.90"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Transporte en camión, 4 palés, Mancha Real-Róterdam"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("640.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Calcular y escribir el UBL
	// Calculate calcula los importes de las líneas, los totales y el desglose
	// del IVA. ubl.Marshal escribe la factura en UBL 2.1, la sintaxis que
	// circula por Peppol.
	inv.Calculate()
	xml, err := ubl.Marshal(inv, &ubl.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("CL-2026-0871.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Validar con las reglas Peppol
	// Las reglas Peppol se aplicarían solas, porque la factura se declara
	// Peppol en BT-24; aquí se piden de forma explícita. Se suman al esquema
	// UBL y a las reglas EN 16931. Una factura con errores no debe salir: el
	// programa se detiene y los muestra.
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.Peppol})
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}

	// ## Releer la factura
	// El mayorista leerá el UBL con ubl.Parse, o con su propio programa. Se
	// recupera el mismo modelo de factura, con sus direcciones Peppol y sus
	// totales.
	back, err := ubl.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	from, to := back.Seller.ElectronicAddress, back.Buyer.ElectronicAddress
	fmt.Println(report.Profile, "·", from.Scheme+":"+from.ID, "→", to.Scheme+":"+to.ID)
	fmt.Println("Base:", back.Totals.TaxBasisTotal, "€ · IVA:", back.Totals.TaxTotal, "€ ·",
		back.VATBreakdown[0].ExemptionReasonCode)
	fmt.Println("Errores:", len(report.Errors()), "· avisos:", len(report.Warnings()))
}

// Output:
// PEPPOL · 9920:ESF23619471 → 0106:24381567
// Base: 17152.00 € · IVA: 0.00 € · VATEX-EU-IC
// Errores: 0 · avisos: 0
