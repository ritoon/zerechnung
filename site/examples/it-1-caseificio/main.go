// Title: Un caseificio reggiano fattura il Parmigiano a una trattoria milanese
// Tags: FatturaPA, FPR12, fattura differita, IVA 4%
//
// Il Caseificio Sociale Il Crinale, sulle colline di Vezzano sul Crostolo,
// consegna ogni settimana alla Trattoria dei Navigli, a Milano, Parmigiano
// Reggiano di diverse stagionature, ricotta e burro di affioramento. Ogni
// consegna viaggia con il suo documento di trasporto; a fine mese il
// caseificio emette una sola fattura differita che li riepiloga tutti.
//
// La fattura va al Sistema di Interscambio in formato FatturaPA. ZeRechnung
// la scrive a partire dal modello EN 16931, ricava il formato FPR12 dal
// codice destinatario del cliente e la controlla rispetto allo schema XSD
// dell'Agenzia delle Entrate.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Descrivere il cedente e il cessionario
	// Il caseificio è identificato dalla partita IVA (BT-31) e dal codice
	// fiscale, con lo schema 0210 (BT-30); l'iscrizione al REA segue la
	// convenzione della CIUS-IT, «REA:ufficio#numero». Il numero civico va
	// nella seconda riga dell'indirizzo e la provincia nella suddivisione.
	// La trattoria riceve le fatture con il suo codice destinatario, un
	// indirizzo elettronico con lo schema 0205.
	caseificio := invoice.Party{
		Name:              "Caseificio Sociale Il Crinale Soc. Agr. Coop.",
		IDs:               []invoice.Identifier{{ID: "REA:RE#245871"}},
		LegalRegistration: invoice.Identifier{ID: "02846170351", Scheme: "0210"},
		VATID:             "IT02846170351",
		Address: invoice.Address{Line1: "Via del Crinale", Line2: "14", PostCode: "42030",
			City: "Vezzano sul Crostolo", CountrySubdivision: "RE", CountryCode: "IT"},
	}
	trattoria := invoice.Party{
		Name:              "Trattoria dei Navigli S.r.l.",
		LegalRegistration: invoice.Identifier{ID: "05172830159", Scheme: "0210"},
		VATID:             "IT05172830159",
		Address: invoice.Address{Line1: "Ripa di Porta Ticinese", Line2: "55", PostCode: "20143",
			City: "Milano", CountrySubdivision: "MI", CountryCode: "IT"},
		ElectronicAddress: invoice.Identifier{ID: "KRRH6B9", Scheme: "0205"},
	}

	// ## Indicare la fattura differita e i DDT
	// La fattura differita, emessa entro il 15 del mese dopo le consegne,
	// ha il tipo documento TD24, che il blocco nazionale Italy aggiunge al
	// codice 380 di EN 16931. Riporta i documenti di trasporto del mese
	// (DatiDDT): il primo è anche il riferimento all'avviso di spedizione,
	// BT-16.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Number:    "312/2026",
		TypeCode:  invoice.TypeCommercialInvoice,
		IssueDate: invoice.NewDate(2026, time.November, 30),
		Currency:  "EUR",
		Seller:    caseificio,
		Buyer:     trattoria,
		Italy: &invoice.Italy{
			DocumentType: "TD24", // fattura differita
			TransportDocuments: []invoice.TransportDocument{
				{Number: "412", Date: invoice.NewDate(2026, time.November, 6)},
				{Number: "437", Date: invoice.NewDate(2026, time.November, 13)},
				{Number: "465", Date: invoice.NewDate(2026, time.November, 20)},
				{Number: "489", Date: invoice.NewDate(2026, time.November, 27)},
			},
		},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IT60X0542811101000000123456",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Bonifico bancario a 30 giorni fine mese.",
			DueDate:     invoice.NewDate(2026, time.December, 31),
		}},
	}

	// ## Elencare i prodotti
	// Formaggi, latticini e burro hanno l'aliquota IVA del 4%. Il trasporto
	// refrigerato è accessorio alla cessione: segue l'aliquota dei beni che
	// accompagna (art. 12 DPR 633/1972). Le unità sono codici UN/ECE: KGM per
	// il chilogrammo, C62 per l'unità.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Parmigiano Reggiano DOP 24 mesi, quarti di forma"},
		Quantity: d("38.40"), UnitCode: "KGM", NetPrice: d("16.90"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("4"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Parmigiano Reggiano DOP 36 mesi, spicchi sottovuoto"},
		Quantity: d("12"), UnitCode: "KGM", NetPrice: d("21.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("4"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Ricotta vaccina fresca"},
		Quantity: d("16"), UnitCode: "KGM", NetPrice: d("6.80"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("4"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Burro di affioramento"},
		Quantity: d("8"), UnitCode: "KGM", NetPrice: d("9.40"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("4"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Trasporto refrigerato, quattro consegne"},
		Quantity: d("4"), UnitCode: "C62", NetPrice: d("18.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("4"),
	}}

	// ## Scrivere la FatturaPA
	// Calculate calcola gli importi delle righe, i totali e il riepilogo IVA
	// con decimali esatti. Marshal scrive il documento nell'ordine dello
	// schema: il codice destinatario della trattoria dà il formato FPR12, e il
	// cedente è anche il trasmittente.
	inv.Calculate()
	xml, err := fatturapa.Marshal(inv, &fatturapa.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}

	// ## Controllare lo schema
	// validate.XML riconosce una FatturaPA e la controlla rispetto allo
	// schema XSD 1.2.4. I controlli dello SdI non sono fatti: lo schema
	// segnala però le date, gli importi e i codici scritti male.
	report := validate.XML(xml, nil)
	if !report.Valid() {
		log.Fatal(report)
	}

	// ## Rileggere la fattura
	// fatturapa.Parse riporta il documento nel modello: i dati che EN 16931
	// prevede tornano nei campi comuni, il resto nel blocco nazionale.
	back, err := fatturapa.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	it := back.Italy
	fmt.Println("Fattura", back.Number, "·", it.DocumentType, "·", len(it.TransportDocuments), "DDT")
	for _, vat := range back.VATBreakdown {
		fmt.Printf("IVA %s%% su %s €: %s €\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	}
	fmt.Println("Totale documento:", back.Totals.GrandTotal, "€")
	fmt.Println("Errori:", len(report.Errors()))
}

// Output:
// Fattura 312/2026 · TD24 · 4 DDT
// IVA 4.00% su 1162.96 €: 46.52 €
// Totale documento: 1209.48 €
// Errori: 0
