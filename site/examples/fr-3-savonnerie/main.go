// Title: Une savonnerie marseillaise émet un avoir
// Tags: Factur-X, UBL, avoir 381, facture d'origine, TVA 20 %
//
// La Savonnerie du Vieux-Port, à Marseille, a livré fin octobre à L'Herbier
// de Montmartre, une boutique parisienne, des cubes de savon de Marseille
// moulés au chaudron et du savon noir à l'huile d'olive. Un carton est
// arrivé écrasé : la savonnerie accorde un avoir pour les pièces abîmées.
//
// Un avoir n'annule pas la facture, il s'y ajoute. Il porte le type 381 et
// cite la facture d'origine avec sa date, ce que les règles françaises
// exigent. L'exemple l'écrit en Factur-X, puis en UBL pour un client qui
// préfère cette syntaxe : la réforme admet les deux.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Retrouver la facture d'origine
	// L'avoir référence la facture qu'il corrige (BG-3) : son numéro (BT-25)
	// et sa date (BT-26). Sans cette référence datée, la règle BR-FR-CO-05
	// refuse l'avoir.
	pdf, err := os.ReadFile("facture.pdf")
	if err != nil {
		log.Fatal(err)
	}
	origine := invoice.DocumentReference{ID: "SV-2026-0712", IssueDate: invoice.NewDate(2026, time.October, 28)}

	// ## Décrire la savonnerie et la boutique
	// Les parties sont les mêmes que sur la facture : SIREN, SIRET, numéro de
	// TVA et adresse électronique de l'annuaire.
	savonnerie := invoice.Party{
		Name:              "Savonnerie du Vieux-Port SAS",
		IDs:               []invoice.Identifier{{ID: "38450127600016", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "384501276", Scheme: invoice.SchemeSIREN},
		VATID:             "FR16384501276",
		Address: invoice.Address{
			Line1: "14 rue Caisserie", PostCode: "13002", City: "Marseille", CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "384501276", Scheme: invoice.SchemeFRDirectory},
	}
	herbier := invoice.Party{
		Name:              "L'Herbier de Montmartre SARL",
		IDs:               []invoice.Identifier{{ID: "50672913600016", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "506729136", Scheme: invoice.SchemeSIREN},
		VATID:             "FR25506729136",
		Address: invoice.Address{
			Line1: "37 rue des Abbesses", PostCode: "75018", City: "Paris", CountryCode: "FR",
		},
		ElectronicAddress: invoice.Identifier{ID: "506729136", Scheme: invoice.SchemeFRDirectory},
	}

	// ## Rédiger l'avoir
	// Le type 381 fait de ce document un avoir : les quantités et les prix
	// restent positifs, c'est le type qui dit qu'il s'agit d'un crédit. Le
	// montant sera déduit de la prochaine facture, ce que disent les
	// conditions de paiement.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:           invoice.ProfileEN16931,
		BusinessProcess:   "B1",
		Number:            "SV-AV-2026-0031",
		TypeCode:          invoice.TypeCreditNote,
		IssueDate:         invoice.NewDate(2026, time.November, 19),
		Currency:          "EUR",
		BuyerReference:    "BOUTIQUE",
		PrecedingInvoices: []invoice.DocumentReference{origine},
		Seller:            savonnerie,
		Buyer:             herbier,
		Notes: []invoice.Note{
			{SubjectCode: "AAI", Content: "Avoir pour marchandises endommagées pendant le transport."},
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Pénalités de retard : trois fois le taux d'intérêt légal."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		PaymentTerms: []invoice.PaymentTerms{{Description: "Montant déduit de la prochaine facture."}},
	}

	// ## Lister les pièces abîmées
	// Seules les pièces abîmées sont créditées, au prix de la facture
	// d'origine. Le savon est à 20 % de TVA.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Cube de savon de Marseille, 72 % d'huile d'olive, 600 g"},
		Quantity: d("24"), UnitCode: "H87", NetPrice: d("4.20"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("20"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Savon noir à l'huile d'olive, pot de 1 kg"},
		Quantity: d("6"), UnitCode: "H87", NetPrice: d("7.90"),
		VATCategoryCode: invoice.VATStandard, VATRate: d("20"),
	}}

	// ## Créer l'avoir Factur-X
	// Create traite un avoir comme une facture : calcul, validation avec les
	// règles françaises (Auto), puis pièce jointe dans le PDF.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{Auto: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("SV-AV-2026-0031.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}
	back, _, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	ref := back.PrecedingInvoices[0]
	fmt.Println("Avoir", back.TypeCode, "·", back.Number, "sur", ref.ID, "du", ref.IssueDate)
	t := back.Totals
	fmt.Println("Total HT :", t.TaxBasisTotal, "€ · TVA :", t.TaxTotal, "€ · TTC :", t.GrandTotal, "€")

	// ## Écrire le même avoir en UBL
	// ubl.Marshal écrit un avoir UBL avec l'élément racine CreditNote, comme
	// le veut UBL 2.1. Le XML se valide avec les mêmes règles, ici choisies
	// explicitement : schéma, EN 16931 et règles françaises.
	xml, err := ubl.Marshal(inv, &ubl.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.France})
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	if err := os.WriteFile("SV-AV-2026-0031.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}
	root := bytes.Contains(xml, []byte("<CreditNote "))
	fmt.Println("UBL CreditNote :", root, "· erreurs :", len(report.Errors()))
}

// Output:
// Avoir 381 · SV-AV-2026-0031 sur SV-2026-0712 du 2026-10-28
// Total HT : 148.20 € · TVA : 29.64 € · TTC : 177.84 €
// UBL CreditNote : true · erreurs : 0
