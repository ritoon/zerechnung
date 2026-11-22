// Title: Un pâtissier dijonnais facture une commune par Chorus Pro
// Tags: Factur-X, Chorus Pro, B2G, code service, numéro d'engagement
//
// Pour le vin d'honneur du 11 Novembre, la commune de Vergy-les-Vignes a
// commandé à la Pâtisserie Lambert, à Dijon, des gougères, des petits fours,
// du pain d'épices au miel et du crémant de Bourgogne. Depuis 2020, toute
// facture adressée à une collectivité passe par Chorus Pro : la commune y
// est identifiée par son SIRET, et elle exige un code service et un numéro
// d'engagement.
//
// L'exemple montre ce que ZeRechnung vérifie avant le dépôt : une première
// facture, sans numéro d'engagement, est refusée ; la seconde est créée. Le
// dépôt lui-même se fait sur le portail Chorus Pro, par son API ou par une
// plateforme agréée.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Lire le PDF et décrire le pâtissier
	// Comme en B2B, le vendeur donne son SIREN, son SIRET, son numéro de TVA
	// et son adresse électronique de l'annuaire (schéma 0225).
	pdf, err := os.ReadFile("facture.pdf")
	if err != nil {
		log.Fatal(err)
	}
	patisserie := invoice.Party{
		Name:              "Pâtisserie Lambert SARL",
		IDs:               []invoice.Identifier{{ID: "79315284400015", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "793152844", Scheme: invoice.SchemeSIREN},
		VATID:             "FR44793152844",
		Address: invoice.Address{
			Line1: "21 rue de la Liberté", PostCode: "21000", City: "Dijon", CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "793152844", Scheme: invoice.SchemeFRDirectory},
	}

	// ## Identifier la commune par son SIRET
	// Chorus Pro identifie la structure publique par son SIRET (BT-46,
	// schéma 0009), dont ZeRechnung contrôle la clé de Luhn. Une commune n'a
	// pas toujours de numéro de TVA : il n'est pas exigé de l'acheteur.
	commune := invoice.Party{
		Name:              "Commune de Vergy-les-Vignes",
		IDs:               []invoice.Identifier{{ID: "21219990500010", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "212199905", Scheme: invoice.SchemeSIREN},
		Address: invoice.Address{
			Line1:       "Place de la Mairie",
			PostCode:    "21220",
			City:        "Vergy-les-Vignes",
			CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "212199905", Scheme: invoice.SchemeFRDirectory},
	}

	// ## Ajouter le code service et les mentions
	// Le code service du destinataire va dans la référence acheteur (BT-10).
	// Les notes PMT, PMD et AAB restent obligatoires ; pour un acheteur
	// public, le retard de paiement donne lieu à des intérêts moratoires.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:         invoice.ProfileEN16931,
		BusinessProcess: "B1",
		Number:          "PL-2026-1111",
		TypeCode:        invoice.TypeCommercialInvoice,
		IssueDate:       invoice.NewDate(2026, time.November, 13),
		Currency:        "EUR",
		BuyerReference:  "FETES-CEREMONIES",
		Seller:          patisserie,
		Buyer:           commune,
		DeliveryDate:    invoice.NewDate(2026, time.November, 11),
		Notes: []invoice.Note{
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Intérêts moratoires au taux de la BCE majoré de huit points."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:  invoice.PaymentSEPACreditTransfer,
			PayeeIBAN: "FR7610107001011234567890129",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Paiement à 30 jours, délai global de paiement de la commande publique.",
			DueDate:     invoice.NewDate(2026, time.December, 13),
		}},
	}

	// ## Lister la commande
	// Les gougères, les petits fours et le pain d'épices sont des denrées
	// alimentaires vendues à emporter : 5,5 %. Le crémant est à 20 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Gougères bourguignonnes"},
		Quantity: d("300"), UnitCode: "H87", NetPrice: d("0.45"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Petits fours sucrés assortis"},
		Quantity: d("400"), UnitCode: "H87", NetPrice: d("0.70"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Pain d'épices de Dijon au miel, 1 kg"},
		Quantity: d("6"), UnitCode: "H87", NetPrice: d("14.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Crémant de Bourgogne brut, 75 cl"},
		Quantity: d("24"), UnitCode: "H87", NetPrice: d("11.50"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("20"),
	}}

	// ## Valider avec les règles de Chorus Pro
	// Les règles ChorusPro s'ajoutent aux règles françaises. PublicBuyer
	// reprend le paramétrage de la commune dans l'annuaire de Chorus Pro :
	// elle exige un code service et un numéro d'engagement. Le numéro
	// d'engagement a été oublié : Create refuse la facture et renvoie les
	// constats dans une *ValidationError.
	opts := &zerechnung.CreateOptions{
		Calculate: true,
		Validate: &validate.Options{
			Rules:       validate.Default | validate.France | validate.ChorusPro,
			PublicBuyer: validate.PublicBuyer{ServiceCodeRequired: true, EngagementRequired: true},
		},
	}
	_, err = zerechnung.Create(pdf, inv, opts)
	var invalid *zerechnung.ValidationError
	if !errors.As(err, &invalid) {
		log.Fatal("la facture aurait dû être refusée")
	}
	for _, f := range invalid.Report.Errors() {
		fmt.Println("Refusée :", f.Rule)
	}

	// ## Ajouter le numéro d'engagement et créer la facture
	// Le numéro d'engagement, donné par la commune sur son bon de commande,
	// va dans la référence de commande (BT-13). La facture passe alors toutes
	// les règles, et le PDF est prêt à être déposé.
	inv.PurchaseOrderReference = "EJ-2026-004187"
	out, err := zerechnung.Create(pdf, inv, opts)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("PL-2026-1111.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Metadata.ConformanceLevel)
	fmt.Println("Service :", back.BuyerReference, "· engagement :", back.PurchaseOrderReference)
	for _, vat := range back.VATBreakdown {
		fmt.Printf("TVA %s %% sur %s € : %s €\n", vat.Rate, vat.TaxableAmount, vat.TaxAmount)
	}
	fmt.Println("Total TTC :", back.Totals.GrandTotal, "€")
}

// Output:
// Refusée : CPRO-05
// factur-x.xml · EN 16931
// Service : FETES-CEREMONIES · engagement : EJ-2026-004187
// TVA 5.5 % sur 502.00 € : 27.61 €
// TVA 20 % sur 276.00 € : 55.20 €
// Total TTC : 860.81 €
