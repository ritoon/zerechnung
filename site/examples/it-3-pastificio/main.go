// Title: Un pastificio di Gragnano spedisce la pasta a Lione
// Tags: FatturaPA, Factur-X, EN 16931, cessione intracomunitaria, IVA 0%
//
// Il Pastificio Fratelli Amitrano, a Gragnano, trafila al bronzo ed essicca
// lentamente la sua pasta di Gragnano IGP. Un bancale di spaghetti, paccheri,
// fusilli lunghi bucati e candele parte per il Comptoir Italien de la
// Croix-Rousse, una gastronomia italiana di Lione.
//
// La fattura parte due volte. Dal luglio 2022 anche le fatture verso i
// clienti esteri passano dal Sistema di Interscambio, in FatturaPA; il
// cliente francese la riceve invece come Factur-X, un PDF che porta con sé
// i dati strutturati. ZeRechnung scrive i due formati dallo stesso modello e
// li controlla entrambi.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Leggere il PDF della fattura
	// Il gestionale del pastificio stampa già la fattura in PDF. Idealmente
	// un PDF/A-3, ma va bene qualsiasi PDF: ZeRechnung non ne modifica i
	// byte, aggiunge l'allegato XML con un aggiornamento incrementale.
	pdf, err := os.ReadFile("fattura.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Descrivere il pastificio e il cliente francese
	// Il cliente è identificato dalla sua partita IVA francese (BT-48) e dal
	// SIREN, con lo schema 0002. Per una cessione intracomunitaria EN 16931
	// chiede anche il paese di consegna (BT-80): la Francia, dove arriva la
	// merce.
	pastificio := invoice.Party{
		Name:              "Pastificio Fratelli Amitrano S.r.l.",
		LegalRegistration: invoice.Identifier{ID: "07429510634", Scheme: invoice.SchemeCodiceFiscale},
		VATID:             "IT07429510634",
		Address: invoice.Address{Line1: "Via Roma", Line2: "48", PostCode: "80054",
			City: "Gragnano", CountrySubdivision: "NA", CountryCode: "IT"},
	}
	comptoir := invoice.Party{
		Name:              "Comptoir Italien de la Croix-Rousse SARL",
		LegalRegistration: invoice.Identifier{ID: "817456320", Scheme: invoice.SchemeSIREN},
		VATID:             "FR34817456320",
		Address: invoice.Address{Line1: "12 boulevard de la Croix-Rousse", PostCode: "69004",
			City: "Lyon", CountryCode: "FR"},
	}

	// ## Indicare la cessione intracomunitaria
	// Una cessione di beni a un'impresa di un altro Stato membro è non
	// imponibile: categoria IVA K, aliquota 0%, con il motivo e il codice
	// VATEX dell'esenzione (BT-120, BT-121) nel riepilogo. In FatturaPA la
	// categoria K diventa la natura N3.2, e il motivo il riferimento
	// normativo.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:      invoice.ProfileEN16931,
		Number:       "EX-2026/0093",
		TypeCode:     invoice.TypeCommercialInvoice,
		IssueDate:    invoice.NewDate(2026, time.November, 23),
		Currency:     "EUR",
		Seller:       pastificio,
		Buyer:        comptoir,
		ShipTo:       &invoice.Party{Name: comptoir.Name, Address: comptoir.Address},
		DeliveryDate: invoice.NewDate(2026, time.November, 20),
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReasonCode: invoice.VATEXIntraCommunity,
			ExemptionReason:     "Cessione intracomunitaria non imponibile, art. 41 D.L. 331/1993",
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IT60X0542811101000000123456",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Bonifico SEPA a 30 giorni data fattura.",
			DueDate:     invoice.NewDate(2026, time.December, 23),
		}},
	}

	// ## Elencare le paste
	// La pasta viaggia in cartoni (XCT, il codice UN/ECE del cartone). Ogni
	// riga porta la stessa categoria K: Calculate ne raccoglie gli importi in
	// un solo riepilogo, conservando il motivo dell'esenzione.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Spaghetti di Gragnano IGP, 24 x 500 g"},
		Quantity: d("60"), UnitCode: "XCT", NetPrice: d("26.40"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Paccheri di Gragnano IGP, 24 x 500 g"},
		Quantity: d("40"), UnitCode: "XCT", NetPrice: d("28.80"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Fusilli lunghi bucati di Gragnano IGP, 12 x 500 g"},
		Quantity: d("25"), UnitCode: "XCT", NetPrice: d("16.20"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Candele spezzate di Gragnano IGP, 24 x 500 g"},
		Quantity: d("20"), UnitCode: "XCT", NetPrice: d("27.60"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Scrivere la FatturaPA per lo SdI
	// Il cliente non ha codice destinatario: per un cliente estero Marshal
	// scrive XXXXXXX, e lo SdI non recapita la fattura, ma ne acquisisce i
	// dati. Lo schema XSD e i controlli dello SdI verificano il documento;
	// ReceivedAt fissa la ricezione al giorno dell'emissione, perché lo SdI
	// scarta una fattura datata dopo (codice 00403).
	inv.Calculate()
	fpa, err := fatturapa.Marshal(inv, &fatturapa.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if r := validate.XML(fpa, &validate.Options{ReceivedAt: inv.IssueDate.Time(nil)}); !r.Valid() {
		log.Fatal(r)
	}

	// ## Creare il Factur-X per il cliente
	// Create scrive la stessa fattura in CII, al profilo EN 16931, la
	// controlla con lo schema, le regole EN 16931 e lo schematron Factur-X
	// (comprese le regole BR-IC delle cessioni intracomunitarie) e la allega
	// al PDF. Una fattura non valida viene rifiutata: Create restituisce le
	// segnalazioni e non scrive nulla.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Validate: &validate.Options{},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("EX-2026-0093.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Rileggere e controllare i due documenti
	// La FatturaPA riletta e il Factur-X riletto danno lo stesso totale.
	// Check controlla anche il contenitore PDF e i metadati XMP.
	sdi, err := fatturapa.Parse(fpa)
	if err != nil {
		log.Fatal(err)
	}
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("FatturaPA · codice destinatario", sdi.Italy.RecipientCode)
	fmt.Println("Factur-X ·", att.Filename, "·", att.Metadata.ConformanceLevel)
	vat := back.VATBreakdown[0]
	fmt.Printf("IVA %s %s%% su %s €: %s €\n", vat.CategoryCode, vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	fmt.Println("Totale FatturaPA:", sdi.Totals.GrandTotal, "€ · Factur-X:", back.Totals.GrandTotal, "€")

	report, _, err := zerechnung.Check(out, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Errori:", len(report.Errors()))
}

// Output:
// FatturaPA · codice destinatario XXXXXXX
// Factur-X · factur-x.xml · EN 16931
// IVA K 0% su 3693.00 €: 0.00 €
// Totale FatturaPA: 3693.00 € · Factur-X: 3693.00 €
// Errori: 0
