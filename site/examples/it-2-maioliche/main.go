// Title: Una bottega di Deruta fattura a un Comune le targhe in maiolica
// Tags: FatturaPA, FPA12, CIUS-IT, scissione dei pagamenti, IVA 22%
//
// Le Fornaci del Tevere, bottega di ceramica artistica di Deruta, hanno
// dipinto a mano le nuove targhe toponomastiche di Torre dei Colli, un borgo
// delle colline umbre: nomi delle vie, numeri civici e un pannello con lo
// stemma comunale, tutti in maiolica, posati sulle facciate del centro
// storico dai loro artigiani. Il cliente è il Comune, che riceve le fatture
// dal Sistema di Interscambio dal 2015.
//
// Il Comune applica la scissione dei pagamenti (art. 17-ter DPR 633/1972):
// paga alla bottega il solo imponibile e versa l'IVA direttamente
// all'Erario. ZeRechnung scrive la fattura in FatturaPA, ne controlla lo
// schema, poi applica alla sua versione europea le regole della CIUS-IT.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Descrivere la bottega e il Comune
	// Il Comune, che acquista per la sua attività istituzionale, è
	// identificato dal solo codice fiscale (schema 0210). Il suo indirizzo
	// elettronico è il codice univoco ufficio dell'Indice delle Pubbliche
	// Amministrazioni (IPA, schema 0201): per questo ZeRechnung scrive la
	// fattura nel formato FPA12.
	bottega := invoice.Party{
		Name:              "Fornaci del Tevere S.r.l.",
		LegalRegistration: invoice.Identifier{ID: "03918460548", Scheme: invoice.SchemeCodiceFiscale},
		VATID:             "IT03918460548",
		Address: invoice.Address{Line1: "Via Tiberina Nord", Line2: "112", PostCode: "06053",
			City: "Deruta", CountrySubdivision: "PG", CountryCode: "IT"},
	}
	comune := invoice.Party{
		Name:              "Comune di Torre dei Colli",
		LegalRegistration: invoice.Identifier{ID: "80002460543", Scheme: invoice.SchemeCodiceFiscale},
		Address: invoice.Address{Line1: "Piazza del Municipio", Line2: "1", PostCode: "06050",
			City: "Torre dei Colli", CountrySubdivision: "PG", CountryCode: "IT"},
		ElectronicAddress: invoice.Identifier{ID: "UF4TQZ", Scheme: invoice.SchemeIPA},
	}

	// ## Riportare contratto, CIG e CUP
	// Per la tracciabilità dei flussi finanziari (legge 136/2010), la fattura
	// alla PA riporta il CIG della procedura (BT-17) e il CUP del progetto
	// (BT-11): in FatturaPA vanno con il contratto (BT-12), mentre la
	// determina che ha affidato il lavoro è il riferimento dell'ordine.
	// Il profilo CIUS-IT darà alla versione europea della fattura il suo
	// identificativo (BT-24). La CIUS-IT chiede le condizioni di pagamento in
	// chiaro (BT-20): con la scissione dei pagamenti, la frase prevista dalla
	// regola BR-IT-DC-510.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileCIUSIT,
		Number:                 "FT-2026/0187",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 24),
		Currency:               "EUR",
		Seller:                 bottega,
		Buyer:                  comune,
		ProjectReference:       "D29J26000110004", // CUP
		ContractReference:      "REP-1187/2026",
		AdditionalDocuments:    []invoice.AdditionalDocument{{ID: "B4F7A21C93", TypeCode: "50"}}, // CIG
		PurchaseOrderReference: "DET-214/2026",
		DeliveryDate:           invoice.NewDate(2026, time.November, 19),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "IT60X0542811101000000123456",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)",
			DueDate:     invoice.NewDate(2026, time.December, 24),
		}},
	}

	// ## Elencare le maioliche e la posa
	// Ogni riga ha la categoria IVA B, la scissione dei pagamenti, con
	// l'aliquota ordinaria del 22%. La CIUS-IT non ammette altre categorie
	// nella stessa fattura. H87 è il codice UN/ECE del pezzo, HUR quello
	// dell'ora.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Targa toponomastica in maiolica, 60 x 30 cm"},
		Quantity: d("28"), UnitCode: "H87", NetPrice: d("165.00"),
		VATCategoryCode: invoice.VATSplitPayment, VATRate: d("22"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Numero civico in maiolica, 15 x 15 cm"},
		Quantity: d("140"), UnitCode: "H87", NetPrice: d("22.00"),
		VATCategoryCode: invoice.VATSplitPayment, VATRate: d("22"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Stemma comunale in maiolica, pannello 80 x 80 cm"},
		Quantity: d("1"), UnitCode: "H87", NetPrice: d("1350.00"),
		VATCategoryCode: invoice.VATSplitPayment, VATRate: d("22"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Posa in opera sulle facciate"},
		Quantity: d("24"), UnitCode: "HUR", NetPrice: d("35.00"),
		VATCategoryCode: invoice.VATSplitPayment, VATRate: d("22"),
	}}

	// ## Calcolare il totale e l'importo del bonifico
	// Calculate calcola l'IVA come per l'aliquota ordinaria, e il totale
	// documento la comprende, come vuole EN 16931 (BR-CO-15): la scissione
	// cambia chi versa l'imposta, non il totale della fattura. Il Comune
	// paga alla bottega l'imponibile, che è l'importo del bonifico
	// (ImportoPagamento), nel blocco nazionale del pagamento.
	inv.Calculate()
	inv.PaymentMeans[0].Italy = &invoice.ItalyPayment{Amount: inv.Totals.TaxBasisTotal}

	// ## Scrivere la FatturaPA
	// Il formato FPA12 viene dal codice IPA del Comune. La categoria B
	// diventa l'esigibilità IVA S nel riepilogo; la frase delle condizioni di
	// pagamento non serve in FatturaPA, e Marshal la toglie. Con Strict,
	// Marshal si ferma se un dato non trova posto in FatturaPA. Lo schema
	// XSD 1.2.4 e i controlli dello SdI verificano poi il documento prima
	// che parta; ReceivedAt fissa la ricezione al giorno dell'emissione,
	// perché lo SdI scarta una fattura datata dopo (codice 00403).
	fpa, err := fatturapa.Marshal(inv, &fatturapa.MarshalOptions{Indent: true, Strict: true})
	if err != nil {
		log.Fatal(err)
	}
	if r := validate.XML(fpa, &validate.Options{ReceivedAt: inv.IssueDate.Time(nil)}); !r.Valid() {
		log.Fatal(r)
	}

	// ## Controllare le regole della CIUS-IT
	// Le regole della CIUS-IT valgono per le fatture europee destinate alla
	// PA: si applicano alla stessa fattura scritta in UBL. L'importo del
	// bonifico non ha un termine EN 16931 e resta solo nella FatturaPA.
	// Il controllo comprende anche la cifra di controllo della partita IVA e
	// del codice fiscale.
	xml, err := ubl.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.Italy})
	if !report.Valid() {
		log.Fatal(report)
	}

	// ## Rileggere la FatturaPA
	// fatturapa.Parse ritrova la categoria B dall'esigibilità S e rimette la
	// frase della scissione nelle condizioni di pagamento.
	back, err := fatturapa.Parse(fpa)
	if err != nil {
		log.Fatal(err)
	}
	vat := back.VATBreakdown[0]
	fmt.Printf("Categoria IVA %s, aliquota %s%%\n", vat.CategoryCode, vat.Rate)
	t, paid := back.Totals, back.PaymentMeans[0].Italy.Amount
	fmt.Println("Imponibile:", t.TaxBasisTotal, "€ · totale documento:", t.GrandTotal, "€")
	fmt.Println("Bonifico alla bottega:", paid, "€ · IVA all'Erario:", t.TaxTotal, "€")
	fmt.Println("Segnalazioni CIUS-IT:", len(report.Findings))
}

// Output:
// Categoria IVA B, aliquota 22.00%
// Imponibile: 9890.00 € · totale documento: 12065.80 €
// Bonifico alla bottega: 9890.00 € · IVA all'Erario: 2175.80 €
// Segnalazioni CIUS-IT: 0
