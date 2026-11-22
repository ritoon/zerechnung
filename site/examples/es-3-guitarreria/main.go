// Title: Una guitarrería de Granada factura tres guitarras a una escuela de Madrid
// Tags: UBL, EN 16931, Factura nacional, IVA 21 %, Descuento
//
// En la Cuesta de Gomérez, la calle que sube a la Alhambra, Guitarras
// Fajardo Morente construye guitarras a mano desde hace tres generaciones;
// hoy dirige el taller Inmaculada Fajardo. La escuela de guitarra flamenca
// La Bulería, en Lavapiés, le ha encargado tres guitarras para sus
// profesores: dos blancas de ciprés y una negra de palosanto.
//
// Con la Ley Crea y Crece, la factura electrónica será obligatoria entre
// empresas: desde octubre de 2027 para las que facturan más de 8 millones
// de euros, un año después para las demás. Valdrá el modelo de la norma
// europea EN 16931 en cuatro sintaxis, entre ellas UBL, la de la solución
// gratuita de la Agencia Tributaria. ZeRechnung escribe y valida esa
// factura; no genera los registros de VERI*FACTU ni envía nada a la AEAT.
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
	// ## Describir el taller y la escuela
	// Las dos partes se identifican por su NIF, con el prefijo ES (BT-31,
	// BT-48). Los datos de inscripción en el Registro Mercantil, que una
	// sociedad debe hacer constar en sus facturas, van en la información
	// legal del vendedor (BT-33). La dirección electrónica (BT-34, BT-49)
	// repite el NIF con el esquema 9920, el de la Agencia Tributaria.
	taller := invoice.Party{
		Name:        "Guitarras Fajardo Morente, S.L.",
		VATID:       "ESB18726042",
		Description: "Inscrita en el R. M. de Granada, tomo 1532, folio 45, hoja GR-48213.",
		Address: invoice.Address{
			Line1: "Cuesta de Gomérez, 18", PostCode: "18009", City: "Granada", CountryCode: "ES",
		},
		ElectronicAddress: invoice.Identifier{ID: "ESB18726042", Scheme: "9920"},
	}
	escuela := invoice.Party{
		Name:  "Escuela de Guitarra Flamenca La Bulería, S.L.",
		VATID: "ESB87531299",
		Address: invoice.Address{
			Line1: "Calle del Ave María, 30", PostCode: "28012", City: "Madrid", CountryCode: "ES",
		},
		ElectronicAddress: invoice.Identifier{ID: "ESB87531299", Scheme: "9920"},
	}

	// ## Añadir las menciones de la factura
	// La factura lleva su serie y su número, la fecha de expedición y la
	// fecha de la operación, aquí la de entrega (BT-72), y remite al encargo
	// de la escuela (BT-13). El plazo de pago queda dentro de los 60 días de
	// la ley de morosidad, con transferencia a la cuenta del taller.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		Number:                 "A-2026-0147",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		BuyerReference:         "PROFESORADO",
		PurchaseOrderReference: "ENC-2026-031",
		Seller:                 taller,
		Buyer:                  escuela,
		DeliveryDate:           invoice.NewDate(2026, time.November, 18),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:  invoice.PaymentSEPACreditTransfer,
			PayeeIBAN: "ES9121000418450200051332",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Transferencia a 30 días.",
			DueDate:     invoice.NewDate(2026, time.December, 20),
		}},
	}

	// ## Describir cada guitarra
	// Una guitarra hecha a mano se define por sus maderas. Los atributos del
	// artículo (BG-32) las recogen como pares de nombre y valor, que el
	// programa del comprador lee sin tener que interpretar el nombre del
	// artículo. Todo lleva el tipo general del IVA, el 21 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{
			Name: "Guitarra flamenca blanca, modelo Albaicín",
			Attributes: []invoice.ItemAttribute{
				{Name: "Tapa", Value: "abeto alemán"},
				{Name: "Aros y fondo", Value: "ciprés"},
				{Name: "Diapasón", Value: "ébano"},
			},
		},
		Quantity: d("2"), UnitCode: "H87", NetPrice: d("3200.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("21"),
	}, {
		ID: "2", Item: invoice.Item{
			Name: "Guitarra flamenca negra, modelo Sacromonte",
			Attributes: []invoice.ItemAttribute{
				{Name: "Tapa", Value: "cedro rojo"},
				{Name: "Aros y fondo", Value: "palosanto de la India"},
				{Name: "Diapasón", Value: "ébano"},
			},
		},
		Quantity: d("1"), UnitCode: "H87", NetPrice: d("3900.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("21"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Estuche rígido de fibra"},
		Quantity: d("3"), UnitCode: "H87", NetPrice: d("180.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("21"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Ajuste de altura de cuerdas y cejuela, por profesor"},
		Quantity: d("3"), UnitCode: "HUR", NetPrice: d("45.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("21"),
	}}

	// ## Aplicar el descuento
	// Por encargar tres guitarras, la escuela tiene un 5 % de descuento sobre
	// su precio. Es un descuento a nivel de documento (BG-20), con su base,
	// su porcentaje, su motivo y su código (95, descuento), y lleva el IVA de
	// lo que descuenta. Calculate deduce el importe.
	inv.AllowancesCharges = []invoice.AllowanceCharge{{
		BaseAmount: d("10300.00"), Percentage: d("5"),
		Reason: "Descuento por encargo de tres guitarras", ReasonCode: "95",
		VATCategoryCode: invoice.VATStandard, VATRate: d("21"),
	}}

	// ## Calcular y escribir el UBL
	// Calculate calcula los importes de las líneas, el descuento, los totales
	// y el desglose del IVA, con decimales exactos. ubl.Marshal escribe la
	// factura en UBL 2.1.
	inv.Calculate()
	xml, err := ubl.Marshal(inv, &ubl.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("A-2026-0147.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Validar y releer la factura
	// ZeRechnung no tiene una CIUS española: se aplican el esquema UBL 2.1 y
	// las reglas de la norma EN 16931, las opciones por defecto. Si hay un
	// error, el programa lo muestra y se detiene. La escuela leerá la factura
	// con ubl.Parse, o con su programa de contabilidad.
	report := validate.XML(xml, &validate.Options{Rules: validate.Default})
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	back, err := ubl.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	negra, t := back.Lines[1].Item, back.Totals
	fmt.Println(negra.Name+":", negra.Attributes[1].Name, "de", negra.Attributes[1].Value)
	fmt.Println("Descuento:", t.AllowanceTotal, "€ · base imponible:", t.TaxBasisTotal, "€")
	fmt.Println("IVA", back.VATBreakdown[0].Rate, "%:", t.TaxTotal, "€ · total:", t.GrandTotal, "€")
	fmt.Println("Errores:", len(report.Errors()))
}

// Output:
// Guitarra flamenca negra, modelo Sacromonte: Aros y fondo de palosanto de la India
// Descuento: 515.00 € · base imponible: 10460.00 €
// IVA 21 %: 2196.60 € · total: 12656.60 €
// Errores: 0
