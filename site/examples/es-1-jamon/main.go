// Title: Un secadero de Guijuelo vende jamón ibérico a una charcutería de Hamburgo
// Tags: ZUGFeRD, Factur-X, EN 16931, Entrega intracomunitaria, IVA 0 %
//
// Jamones Hermanas Prieto cura jamones y paletas de bellota en sus bodegas
// de Guijuelo (Salamanca), al aire frío de la sierra. Desde hace unos años
// sirve a Feinkost Möller & Kaya, una charcutería fina de Hamburgo que corta
// el jamón a cuchillo delante de sus clientes. El programa de facturación
// del secadero ya genera la factura en PDF; ZeRechnung le añade el XML y los
// metadatos de ZUGFeRD, el nombre alemán de Factur-X, para que la
// contabilidad de Hamburgo la lea sin teclear nada.
//
// Es una entrega intracomunitaria de bienes: la factura no lleva IVA
// español (categoría K) y el comprador declara el IVA de la adquisición en
// Alemania. ZeRechnung no tiene reglas nacionales para España: la factura
// se valida con la norma europea EN 16931 y con lo que Alemania exige a una
// factura electrónica.
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
	// ## Leer el PDF de la factura
	// Se parte del PDF que el programa de facturación ya produce. Lo ideal es
	// un PDF/A-3, pero sirve cualquier PDF: ZeRechnung no toca sus bytes y
	// añade el XML como adjunto mediante una actualización incremental.
	pdf, err := os.ReadFile("factura.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Describir al vendedor y al comprador
	// En una entrega intracomunitaria, la factura debe llevar el NIF-IVA de
	// las dos partes (regla BR-IC-02): el del secadero, con el prefijo ES, y
	// el número alemán de IVA de la charcutería (USt-IdNr.). ZeRechnung
	// comprueba que estén, pero no consulta el sistema VIES: esa
	// verificación sigue siendo cosa del vendedor.
	secadero := invoice.Party{
		Name:  "Jamones Hermanas Prieto, S.L.",
		VATID: "ESB37412855",
		Address: invoice.Address{
			Line1: "Calle de la Dehesa, 14", PostCode: "37770", City: "Guijuelo",
			CountrySubdivision: "Salamanca", CountryCode: "ES",
		},
		Contact: &invoice.Contact{
			PersonName: "Rocío Prieto",
			Phone:      "+34 923 000 000",
			Email:      "pedidos@hermanasprieto.example",
		},
	}
	charcuteria := invoice.Party{
		Name:  "Feinkost Möller & Kaya GmbH",
		VATID: "DE318472657",
		Address: invoice.Address{
			Line1: "Isestraße 41", PostCode: "20144", City: "Hamburg", CountryCode: "DE",
		},
	}

	// ## Añadir las menciones obligatorias
	// Una factura exenta debe citar la norma que la exime: el artículo 25 de
	// la Ley del IVA va en el motivo de exención (BT-120), con su código
	// VATEX-EU-IC (BT-121). La fecha de entrega (BT-72) y el país de destino
	// de la mercancía (BT-80) también son obligatorios con la categoría K. El
	// plazo de pago respeta los 60 días de la ley de morosidad.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		Number:                 "HP-2026-0412",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		PurchaseOrderReference: "B-2026-118",
		Seller:                 secadero,
		Buyer:                  charcuteria,
		DeliveryDate:           invoice.NewDate(2026, time.November, 17),
		ShipTo: &invoice.Party{
			Name: "Feinkost Möller & Kaya, tienda de Eppendorf",
			Address: invoice.Address{
				Line1: "Isestraße 41", PostCode: "20144", City: "Hamburg", CountryCode: "DE",
			},
		},
		Notes: []invoice.Note{
			{SubjectCode: "PMD", Content: "Intereses de demora según la Ley 3/2004 de morosidad."},
		},
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReason:     "Entrega intracomunitaria exenta, art. 25 Ley 37/1992.",
			ExemptionReasonCode: "VATEX-EU-IC",
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:  invoice.PaymentSEPACreditTransfer,
			PayeeIBAN: "ES9121000418450200051332",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Transferencia a 30 días de la fecha de entrega.",
			DueDate:     invoice.NewDate(2026, time.December, 17),
		}},
	}

	// ## Detallar los productos
	// El jamón se vende al peso: la cantidad va en kilos (código UN/ECE KGM),
	// los soportes por piezas (H87) y el transporte como una unidad (C62).
	// Todas las líneas llevan la categoría K y un tipo del 0 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Jamón de bellota 100 % ibérico, DOP Guijuelo, 5 piezas"},
		Quantity: d("41.6"), UnitCode: "KGM", NetPrice: d("62.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Paleta de bellota 100 % ibérica, DOP Guijuelo, 6 piezas"},
		Quantity: d("27.3"), UnitCode: "KGM", NetPrice: d("38.50"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Lomo de bellota ibérico, pieza entera"},
		Quantity: d("9.8"), UnitCode: "KGM", NetPrice: d("54.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Chorizo ibérico de bellota, vela"},
		Quantity: d("10"), UnitCode: "KGM", NetPrice: d("21.50"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Soporte jamonero de madera de haya"},
		Quantity: d("2"), UnitCode: "H87", NetPrice: d("45.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "6", Item: invoice.Item{Name: "Transporte a temperatura controlada, Guijuelo-Hamburgo"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("180.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Crear la factura ZUGFeRD
	// Calculate calcula los importes de las líneas, los totales y el desglose
	// del IVA, con decimales exactos. Con Auto, la validación añadiría las
	// reglas del país del vendedor, pero para España no hay ninguna. Por eso
	// se piden, además de las reglas por defecto, las de GermanyProfile: la
	// Hacienda alemana solo admite como factura electrónica los perfiles
	// completos, a partir de BASIC. Si la factura no es válida, Create
	// devuelve los errores y no escribe nada.
	opts := &validate.Options{Rules: validate.Default | validate.GermanyProfile}
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  opts,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("HP-2026-0412.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Releer y comprobar el resultado
	// El PDF generado se lee como cualquier factura recibida. Check revisa el
	// contenedor PDF, los metadatos XMP y el XML con las mismas reglas. Con
	// las reglas alemanas, el XML debe ir adjunto como Alternative: en
	// Alemania, lo que cuenta legalmente es el XML, no el PDF.
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Relationship, "·", att.Metadata.ConformanceLevel)
	for _, vat := range back.VATBreakdown {
		fmt.Printf("IVA %s %s %% sobre %s € (%s)\n",
			vat.CategoryCode, vat.Rate, vat.TaxableAmount, vat.ExemptionReasonCode)
	}
	due := back.PaymentTerms[0].DueDate
	fmt.Println("Total:", back.Totals.GrandTotal, "€ · a pagar antes del", due)

	report, _, err := zerechnung.Check(out, opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Errores:", len(report.Errors()))
}

// Output:
// factur-x.xml · Alternative · EN 16931
// IVA K 0 % sobre 4644.45 € (VATEX-EU-IC)
// Total: 4644.45 € · a pagar antes del 2026-12-17
// Errores: 0
