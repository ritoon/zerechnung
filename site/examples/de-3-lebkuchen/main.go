// Title: Ein Stand am Christkindlesmarkt prüft seine Eingangsrechnung
// Tags: ZUGFeRD, Empfang, Prüfung, BMF-Profile, USt 7 % und 19 %
//
// Monika Hollerbach verkauft am Nürnberger Christkindlesmarkt Lebkuchen,
// Zwetschgenmännla und Geschenkdosen. Ihre Ware bezieht sie von der
// Lebküchnerei Merkl in der Altstadt. Seit 2025 muss jedes Unternehmen in
// Deutschland E-Rechnungen empfangen können, auch ein Marktstand: Die
// Rechnung kommt als ZUGFeRD-PDF, und die Buchhaltung liest die Daten aus
// dem eingebetteten XML statt vom Papier.
//
// Dieses Beispiel spielt beide Seiten durch: Die Lebküchnerei erzeugt die
// Rechnung, der Stand liest und prüft sie. Am Ende zeigt es, was die
// Prüfung bei einem Profil meldet, das keine E-Rechnung im Sinne des BMF ist.
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
	// ## Die Lebküchnerei beschreibt die Rechnung
	// Die Lieferung ging am 24. November an den Stand, rechtzeitig zur
	// Eröffnung des Markts. Verkäufer und Käufer nennen Anschrift und
	// USt-IdNr; die Bankverbindung steht in den Zahlungsanweisungen (BG-16).
	pdf, err := os.ReadFile("rechnung.pdf")
	if err != nil {
		log.Fatal(err)
	}
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:   invoice.ProfileEN16931,
		Number:    "LM-26-0907",
		TypeCode:  invoice.TypeCommercialInvoice,
		IssueDate: invoice.NewDate(2026, time.November, 25),
		Currency:  "EUR",
		Seller: invoice.Party{
			Name:  "Lebküchnerei Merkl e. K.",
			VATID: "DE369147258",
			Address: invoice.Address{
				Line1: "Bergstraße 9", PostCode: "90403", City: "Nürnberg", CountryCode: "DE",
			},
		},
		Buyer: invoice.Party{
			Name:  "Monika Hollerbach, Lebkuchen und Geschenke",
			VATID: "DE528471633",
			Address: invoice.Address{
				Line1:       "Glockengießergasse 4",
				PostCode:    "90403",
				City:        "Nürnberg",
				CountryCode: "DE",
			},
		},
		DeliveryDate: invoice.NewDate(2026, time.November, 24),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "DE75512108001245126199",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Zahlbar bis 9. Dezember 2026 ohne Abzug.",
			DueDate:     invoice.NewDate(2026, time.December, 9),
		}},
	}

	// ## Lebkuchen zu 7 %, Blechdosen zu 19 %
	// Lebkuchen und Zwetschgenmännla sind Lebensmittel und tragen 7 %
	// Umsatzsteuer. Die bedruckten Blechdosen verkauft die Lebküchnerei
	// einzeln: Sie sind keine Verpackung der Lebkuchen und tragen 19 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Elisenlebkuchen, Karton mit 50 Stück"},
		Quantity: d("12"), UnitCode: "H87", NetPrice: d("42.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Lebkuchenherz „Frohe Weihnachten“, 18 cm"},
		Quantity: d("300"), UnitCode: "H87", NetPrice: d("2.40"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Nürnberger Zwetschgenmännla"},
		Quantity: d("80"), UnitCode: "H87", NetPrice: d("6.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("7"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Geschenkdose aus Blech, Motiv Altstadt"},
		Quantity: d("200"), UnitCode: "H87", NetPrice: d("3.20"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("19"),
	}}

	// ## Das ZUGFeRD-PDF erzeugen
	// Wie in jedem Beispiel berechnet Create die Beträge, prüft die Rechnung
	// und bettet das XML in das PDF ein. Dieses PDF schickt die
	// Lebküchnerei per E-Mail an den Stand.
	received, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{Auto: true},
	})
	if err != nil {
		log.Fatal(err)
	}

	// ## Die Eingangsrechnung lesen
	// Auf der Seite des Stands beginnt alles mit dem empfangenen PDF. Read
	// findet das eingebettete XML und liest es in dieselbe Struktur
	// invoice.Invoice, mit der die Lebküchnerei es geschrieben hat.
	in, _, err := zerechnung.Read(received)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(in.Seller.Name, "·", in.Number, "vom", in.IssueDate)
	for _, l := range in.Lines {
		fmt.Printf("%8s €  %4s × %s\n", l.NetAmount, l.Quantity, l.Item.Name)
	}
	fmt.Println("Zu zahlen:", in.Totals.DuePayable, "€ bis", in.PaymentTerms[0].DueDate)
	fmt.Println("Auf das Konto:", in.PaymentMeans[0].PayeeIBAN)

	// ## Die Rechnung vor dem Buchen prüfen
	// Check prüft das PDF und das XML. Auto ergänzt für einen Verkäufer in
	// Deutschland die Prüfung des Profils: Seit 2025 sind nur BASIC,
	// EN 16931, EXTENDED und XRECHNUNG E-Rechnungen im Sinne des BMF.
	report, _, err := zerechnung.Check(received, &validate.Options{Auto: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Fehler:", len(report.Errors()))

	// ## Was die Prüfung bei einem MINIMUM-PDF meldet
	// Das Profil MINIMUM trägt nur die Summen, keine Positionen und nicht
	// alle Pflichtangaben nach § 14 UStG: In Deutschland ist es keine
	// E-Rechnung. Die Prüfung meldet es mit der Regel DE-PROFILE-01, und der
	// Stand darf eine solche Rechnung zurückweisen.
	minimum, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{Profile: invoice.ProfileMinimum})
	if err != nil {
		log.Fatal(err)
	}
	report, _, err = zerechnung.Check(minimum, &validate.Options{Auto: true})
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range report.Errors() {
		fmt.Println("MINIMUM:", f.Rule)
	}
}

// Output:
// Lebküchnerei Merkl e. K. · LM-26-0907 vom 2026-11-25
//   510.00 €    12 × Elisenlebkuchen, Karton mit 50 Stück
//   720.00 €   300 × Lebkuchenherz „Frohe Weihnachten“, 18 cm
//   520.00 €    80 × Nürnberger Zwetschgenmännla
//   640.00 €   200 × Geschenkdose aus Blech, Motiv Altstadt
// Zu zahlen: 2634.10 € bis 2026-12-09
// Auf das Konto: DE75512108001245126199
// Fehler: 0
// MINIMUM: DE-PROFILE-01
