// Title: Eine Münchner Bäckerei beliefert ein Wirtshaus
// Tags: ZUGFeRD, EN 16931, Leistungszeitraum, USt 7 % und 19 %
//
// Die Backstube Brandl in Giesing liefert jeden Morgen frische Brezn,
// Kaisersemmeln, Bauernbrot und Auszogne an das Gasthaus Zur Isarlust. Am
// Monatsende stellt sie eine Sammelrechnung über alle Lieferungen des
// Monats. Ihr Warenwirtschaftssystem erzeugt die Rechnung schon als PDF;
// ZeRechnung bettet das XML ein und macht daraus eine ZUGFeRD-Rechnung, die
// das Gasthaus seit 2025 als E-Rechnung empfangen muss.
//
// Backwaren tragen den ermäßigten Steuersatz von 7 %, die zwei
// Brezenständer aus Edelstahl, die das Gasthaus dazugekauft hat, den
// Regelsteuersatz von 19 %.
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
	// ## Das PDF der Rechnung lesen
	// Ausgangspunkt ist das PDF, das die Software ohnehin erzeugt, am besten
	// ein PDF/A-3. ZeRechnung ändert seine Bytes nicht: Es hängt das XML mit
	// einer inkrementellen Aktualisierung an.
	pdf, err := os.ReadFile("rechnung.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Verkäufer und Käufer beschreiben
	// Nach § 14 UStG nennt die Rechnung Namen und Anschrift beider Parteien
	// und die Steuernummer oder die Umsatzsteuer-Identifikationsnummer des
	// Verkäufers. Die Backstube gibt beide an: die Steuernummer des
	// Finanzamts München (BT-32) und die USt-IdNr (BT-31).
	backstube := invoice.Party{
		Name:              "Backstube Brandl GmbH",
		VATID:             "DE812746359",
		TaxRegistrationID: "143/158/40273",
		Address: invoice.Address{
			Line1:       "Tegernseer Landstraße 61",
			PostCode:    "81541",
			City:        "München",
			CountryCode: "DE",
		},
	}
	isarlust := invoice.Party{
		Name:  "Gasthaus Zur Isarlust GmbH",
		VATID: "DE293618476",
		Address: invoice.Address{
			Line1: "Ohlmüllerstraße 12", PostCode: "81541", City: "München", CountryCode: "DE",
		},
	}

	// ## Leistungszeitraum und Pflichtangaben
	// Eine Sammelrechnung gibt statt eines Lieferdatums den
	// Leistungszeitraum an (BG-14): hier den ganzen Oktober. Die Angaben
	// zum Handelsregister stehen in einer Notiz mit dem Code REG, die
	// Zahlungsbedingungen in BT-20.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:        invoice.ProfileEN16931,
		Number:         "RE-2026-1031",
		TypeCode:       invoice.TypeCommercialInvoice,
		IssueDate:      invoice.NewDate(2026, time.November, 2),
		Currency:       "EUR",
		BuyerReference: "KÜCHE",
		Seller:         backstube,
		Buyer:          isarlust,
		InvoicingPeriod: invoice.Period{
			Start: invoice.NewDate(2026, time.October, 1),
			End:   invoice.NewDate(2026, time.October, 31),
		},
		Notes: []invoice.Note{
			{SubjectCode: "REG", Content: "Amtsgericht München, HRB 214587. Geschäftsführerin: Theresa Brandl."},
		},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "DE89370400440532013000",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Zahlbar innerhalb von 14 Tagen ohne Abzug.",
			DueDate:     invoice.NewDate(2026, time.November, 16),
		}},
	}

	// ## Die Positionen erfassen
	// Jede Position hat eine Menge, eine Einheit (UN/ECE-Code: H87 für
	// Stück), einen Nettopreis und ihren Steuersatz. Die Backwaren eines
	// Monats stehen in einer Position je Sorte.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Laugenbrezn, frisch gebacken"},
		Quantity: d("1200"), UnitCode: "H87", NetPrice: d("0.62"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Kaisersemmeln"},
		Quantity: d("800"), UnitCode: "H87", NetPrice: d("0.48"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Bauernbrot, Roggenmischbrot 2 kg"},
		Quantity: d("60"), UnitCode: "H87", NetPrice: d("5.90"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Auszogne (Schmalznudeln)"},
		Quantity: d("150"), UnitCode: "H87", NetPrice: d("1.80"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Brezenständer aus Edelstahl, 12 Haken"},
		Quantity: d("2"), UnitCode: "H87", NetPrice: d("89.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("19"),
	}}

	// ## Die ZUGFeRD-Rechnung erzeugen
	// Calculate berechnet Positionsbeträge, Summen und die Aufschlüsselung
	// nach Steuersätzen, mit exakten Dezimalzahlen. Auto ergänzt für einen
	// Verkäufer in Deutschland die Prüfung, ob das Profil eine E-Rechnung im
	// Sinne des BMF ist. Eine ungültige Rechnung wird abgelehnt: Create gibt
	// dann die Befunde zurück und schreibt nichts.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{Auto: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("RE-2026-1031.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Das Ergebnis lesen und prüfen
	// Das erzeugte PDF lässt sich wie jede empfangene Rechnung lesen. Check
	// prüft den PDF-Container, die XMP-Metadaten und das XML mit denselben
	// Regeln.
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Metadata.ConformanceLevel, "·", att.Relationship)
	fmt.Println("Leistungszeitraum:", back.InvoicingPeriod.Start, "bis", back.InvoicingPeriod.End)
	for _, vat := range back.VATBreakdown {
		fmt.Printf("USt %s %% auf %s €: %s €\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	}
	fmt.Println("Netto:", back.Totals.TaxBasisTotal, "€ · Brutto:", back.Totals.GrandTotal, "€")

	report, _, err := zerechnung.Check(out, &validate.Options{Auto: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Fehler:", len(report.Errors()))
}

// Output:
// factur-x.xml · EN 16931 · Alternative
// Leistungszeitraum: 2026-10-01 bis 2026-10-31
// USt 7 % auf 1752.00 €: 122.64 €
// USt 19 % auf 178.00 €: 33.82 €
// Netto: 1930.00 € · Brutto: 2086.46 €
// Fehler: 0
