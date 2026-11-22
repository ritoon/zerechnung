// Title: Une fromagerie du Jura facture un bouchon lyonnais
// Tags: Factur-X, EN 16931, BR-FR, TVA 5,5 % et 20 %
//
// La Fromagerie des Monts du Jura, à Poligny, livre chaque semaine le
// Bouchon des Canuts, à Lyon : meules de Comté, Morbier, Mont d'Or en boîte
// d'épicéa et quelques bouteilles de vin jaune. Son logiciel de caisse
// produit déjà la facture en PDF ; ZeRechnung y ajoute le XML et les
// métadonnées Factur-X, et vérifie les règles françaises de la réforme avant
// d'écrire quoi que ce soit.
//
// Les fromages sont à 5,5 % de TVA, le vin jaune et la livraison à 20 % :
// la facture porte deux taux, et ZeRechnung calcule la ventilation.
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
	// ## Lire le PDF de la facture
	// Le point de départ est le PDF que votre logiciel produit déjà. Idéalement
	// un PDF/A-3, mais tout PDF convient : ZeRechnung n'en modifie pas les
	// octets, il ajoute la pièce jointe par une mise à jour incrémentale.
	pdf, err := os.ReadFile("facture.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Décrire le vendeur et l'acheteur
	// Depuis la réforme, chaque partie est identifiée par son SIREN (BT-30) et
	// son SIRET, et reçoit ses factures à une adresse électronique de
	// l'annuaire (schéma 0225). Le numéro de TVA est obligatoire pour les deux.
	fromagerie := invoice.Party{
		Name:              "Fromagerie des Monts du Jura SARL",
		IDs:               []invoice.Identifier{{ID: "84215763800011", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "842157638", Scheme: invoice.SchemeSIREN},
		VATID:             "FR62842157638",
		Address: invoice.Address{
			Line1: "12 rue du Comté", PostCode: "39800", City: "Poligny", CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "842157638", Scheme: invoice.SchemeFRDirectory},
	}
	bouchon := invoice.Party{
		Name:              "Le Bouchon des Canuts SAS",
		IDs:               []invoice.Identifier{{ID: "53917842600018", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "539178426", Scheme: invoice.SchemeSIREN},
		VATID:             "FR53539178426",
		Address: invoice.Address{
			Line1: "8 rue Mercière", PostCode: "69002", City: "Lyon", CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "539178426_ACHATS", Scheme: invoice.SchemeFRDirectory},
	}

	// ## Ajouter les mentions obligatoires
	// Le cadre de facturation B1 dit qu'il s'agit d'une livraison de biens. Les
	// notes PMT, PMD et AAB portent l'indemnité forfaitaire de recouvrement,
	// les pénalités de retard et les conditions d'escompte, que le Code de
	// commerce exige sur toute facture entre professionnels.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		BusinessProcess:        "B1",
		Number:                 "FJ-2026-0318",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		BuyerReference:         "CUISINE",
		PurchaseOrderReference: "BC-1147",
		Seller:                 fromagerie,
		Buyer:                  bouchon,
		DeliveryDate:           invoice.NewDate(2026, time.November, 18),
		Notes: []invoice.Note{
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Pénalités de retard : trois fois le taux d'intérêt légal."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:  invoice.PaymentSEPACreditTransfer,
			PayeeIBAN: "FR7630006000011234567890189",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Paiement à 30 jours fin de livraison, par virement.",
			DueDate:     invoice.NewDate(2026, time.December, 18),
		}},
	}

	// ## Lister les produits
	// Chaque ligne a sa quantité, son unité (code UN/ECE : KGM pour le
	// kilogramme, H87 pour la pièce, C62 pour une unité), son prix net et son
	// taux de TVA. Les fromages relèvent du taux réduit de 5,5 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Comté AOP 18 mois, à la coupe"},
		Quantity: d("12.5"), UnitCode: "KGM", NetPrice: d("24.80"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Morbier AOP"},
		Quantity: d("6"), UnitCode: "KGM", NetPrice: d("17.40"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Mont d'Or AOP, boîte d'épicéa de 500 g"},
		Quantity: d("10"), UnitCode: "H87", NetPrice: d("11.90"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Vin jaune du Jura AOC, clavelin 62 cl"},
		Quantity: d("6"), UnitCode: "H87", NetPrice: d("32.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("20"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Livraison en camion frigorifique"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("35.00"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("20"),
	}}

	// ## Créer la facture Factur-X
	// Calculate calcule les montants des lignes, les totaux et la ventilation
	// de TVA par taux, en décimales exactes. Avec Auto, la validation ajoute
	// les règles BR-FR, puisque le vendeur est en France. Une facture
	// invalide est refusée : Create renvoie alors les constats, rien n'est
	// écrit.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{Auto: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("FJ-2026-0318.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Relire et vérifier le résultat
	// Le PDF produit se relit comme n'importe quelle facture reçue. Check
	// contrôle le conteneur PDF, les métadonnées XMP et le XML, avec les mêmes
	// règles.
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Metadata.ConformanceLevel)
	for _, vat := range back.VATBreakdown {
		fmt.Printf("TVA %s %% sur %s € : %s €\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	}
	fmt.Println("Total HT :", back.Totals.TaxBasisTotal, "€ · TTC :", back.Totals.GrandTotal, "€")

	report, _, err := zerechnung.Check(out, &validate.Options{Auto: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Erreurs :", len(report.Errors()))
}

// Output:
// factur-x.xml · EN 16931
// TVA 5.5 % sur 533.40 € : 29.34 €
// TVA 20 % sur 227.00 € : 45.40 €
// Total HT : 760.40 € · TTC : 835.14 €
// Erreurs : 0
