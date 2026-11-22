// Title: Eine Schwarzwälder Schreinerei stellt einer Gemeinde eine XRechnung
// Tags: XRechnung, CII, Leitweg-ID, Skonto, USt 19 %
//
// Die Schreinerei Wälder in Triberg hat für die Gemeinde Tannweiler eine
// handgeschnitzte Kuckucksuhr für das Foyer des Rathauses gebaut und sechs
// Sitzbänke aus Weißtanne für den Kurpark geliefert und aufgestellt.
// Öffentliche Auftraggeber in Deutschland nehmen Rechnungen nur noch als
// XRechnung an: eine reine XML-Datei, ohne PDF, adressiert über die
// Leitweg-ID der Behörde.
//
// Die Schreinerei gewährt 2 % Skonto bei Zahlung innerhalb von zehn Tagen.
// XRechnung schreibt dafür eine maschinenlesbare Zeile in den
// Zahlungsbedingungen vor, die ZeRechnung prüft.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Den Verkäufer mit Ansprechpartner beschreiben
	// XRechnung verlangt mehr als die EN 16931: einen Ansprechpartner mit
	// Telefonnummer und E-Mail-Adresse (BR-DE-2 bis BR-DE-7) und eine
	// elektronische Adresse (BT-34), hier eine E-Mail-Adresse (Schema EM).
	schreinerei := invoice.Party{
		Name:              "Schreinerei Wälder GmbH",
		VATID:             "DE147258365",
		LegalRegistration: invoice.Identifier{ID: "HRB 708124"},
		Address: invoice.Address{
			Line1:       "Hauptstraße 27",
			PostCode:    "78098",
			City:        "Triberg im Schwarzwald",
			CountryCode: "DE",
		},
		Contact: &invoice.Contact{
			PersonName: "Lena Wälder",
			Phone:      "+49 7722 91 84 0",
			Email:      "lena.waelder@schreinerei-waelder.example",
		},
		ElectronicAddress: invoice.Identifier{
			ID: "rechnung@schreinerei-waelder.example", Scheme: invoice.SchemeEmail,
		},
	}

	// ## Den öffentlichen Auftraggeber über die Leitweg-ID adressieren
	// Die Leitweg-ID kennzeichnet die rechnungsempfangende Stelle. Sie steht
	// in der Käuferreferenz (BT-10, Pflicht nach BR-DE-15) und als
	// elektronische Adresse mit dem Schema 0204. Ihre zwei Prüfziffern
	// (ISO 7064 MOD 97-10) prüft ZeRechnung mit.
	leitwegID := "08315123-RATHAUS-91"
	gemeinde := invoice.Party{
		Name: "Gemeinde Tannweiler, Hauptamt",
		Address: invoice.Address{
			Line1: "Rathausplatz 1", PostCode: "79874", City: "Tannweiler", CountryCode: "DE",
		},
		ElectronicAddress: invoice.Identifier{ID: leitwegID, Scheme: invoice.SchemeLeitwegID},
	}

	// ## Rechnungskopf und Positionen
	// Die Rechnung nennt die Bestellung der Gemeinde (BT-13), das Datum der
	// Aufstellung und drei Positionen: die Uhr, die Bänke und die Montage
	// nach Stunden (UN/ECE-Code HUR). Alle tragen den Regelsteuersatz.
	// XRechnung übernimmt außerdem Peppol-Regeln: der Geschäftsprozess
	// (BT-23) ist Pflicht (PEPPOL-EN16931-R001), hier der von Peppol BIS
	// Billing 3.0.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileXRechnung,
		BusinessProcess:        invoice.ProcessPeppolBilling,
		Number:                 "2026-0417",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		BuyerReference:         leitwegID,
		PurchaseOrderReference: "HA-2026-118",
		Seller:                 schreinerei,
		Buyer:                  gemeinde,
		DeliveryDate:           invoice.NewDate(2026, time.November, 13),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:         invoice.PaymentSEPACreditTransfer,
			PayeeIBAN:        "DE02120300000000202051",
			PayeeAccountName: "Schreinerei Wälder GmbH",
		}},
	}
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{
			Name:        "Kuckucksuhr, handgeschnitzt, Achttagewerk",
			Description: "Lindenholz, Höhe 85 cm, mit Musikwerk",
		},
		Quantity: d("1"), UnitCode: "H87", NetPrice: d("1890.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("19"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Sitzbank aus Weißtanne, 1,80 m, wetterfest geölt"},
		Quantity: d("6"), UnitCode: "H87", NetPrice: d("640.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("19"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Montage im Kurpark und im Rathaus"},
		Quantity: d("12"), UnitCode: "HUR", NetPrice: d("58.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("19"),
	}}

	// ## Skonto in den Zahlungsbedingungen angeben
	// Das Skonto steht als eigene Zeile in BT-20, in der Form
	// #SKONTO#TAGE=10#PROZENT=2.00#, gefolgt von einem Zeilenumbruch
	// (BR-DE-18). Ein Buchhaltungssystem kann es so automatisch abziehen.
	inv.PaymentTerms = []invoice.PaymentTerms{{
		Description: "Zahlbar innerhalb von 30 Tagen netto.\n#SKONTO#TAGE=10#PROZENT=2.00#\n",
		DueDate:     invoice.NewDate(2026, time.December, 20),
	}}

	// ## Die XRechnung schreiben
	// Calculate berechnet die Beträge. cii.Marshal schreibt die Rechnung in
	// der CII-Syntax mit der Spezifikationskennung von XRechnung 3.0
	// (BT-24); die Behörde nimmt sie über ihr Rechnungseingangsportal an.
	inv.Calculate()
	xml, err := cii.Marshal(inv, &cii.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("xrechnung.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Mit den Regeln von XRechnung prüfen
	// Eine XRechnung erkennt ZeRechnung an ihrer Spezifikationskennung: Die
	// Regeln BR-DE und die Prüfung der Leitweg-ID kommen von selbst zu
	// Schema, EN 16931 und Profilregeln hinzu, ohne Option.
	report := validate.XML(xml, nil)
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	fmt.Println("Leitweg-ID:", inv.BuyerReference)
	t := inv.Totals
	fmt.Println("Netto:", t.TaxBasisTotal, "€ · USt:", t.TaxTotal, "€ · Brutto:", t.GrandTotal, "€")
	skonto := t.DuePayable.Mul(d("0.02")).Round(2)
	fmt.Println("Bei Zahlung bis", inv.IssueDate.AddDays(10), "zu zahlen:", t.DuePayable.Sub(skonto), "€")
	fmt.Println("Befunde:", len(report.Findings))
}

// Output:
// Leitweg-ID: 08315123-RATHAUS-91
// Netto: 6426.00 € · USt: 1220.94 € · Brutto: 7646.94 €
// Bei Zahlung bis 2026-11-30 zu zahlen: 7494.00 €
// Befunde: 0
