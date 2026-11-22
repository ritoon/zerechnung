// Title: Uma fábrica de rolhas de Santa Maria da Feira fatura a um château de Bordéus
// Tags: Factur-X, EN 16931, Transmissão intracomunitária, IVA 0 %, Preço ao milheiro
//
// A Rolhas Teixeira & Filhas, em Santa Maria da Feira, transforma em rolhas
// a cortiça dos montados alentejanos. Portugal produz perto de metade da
// cortiça do mundo, e é no concelho da Feira que se concentra a indústria.
// Para engarrafar a colheita de 2024, o Château Mauvesin-Lartigue, em
// Saint-Émilion, encomendou rolhas naturais com o brasão da casa marcado a
// fogo.
//
// O programa de faturação certificado da fábrica já emite a fatura em PDF,
// com o ATCUD e o código QR que a lei portuguesa exige. O ZeRechnung
// junta-lhe o XML e os metadados Factur-X, o formato franco-alemão que o
// château lê sem esforço. A comunicação das faturas à AT continua a cargo
// do programa certificado.
//
// Trata-se de uma transmissão intracomunitária de bens: a fatura não leva
// IVA português (categoria K).
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
	// ## Ler o PDF da fatura
	// O ponto de partida é o PDF que o programa de faturação já produz. O
	// ideal é um PDF/A-3, mas qualquer PDF serve: o ZeRechnung não altera os
	// seus bytes e junta o XML numa atualização incremental.
	pdf, err := os.ReadFile("fatura.pdf")
	if err != nil {
		log.Fatal(err)
	}

	// ## Descrever o vendedor e o comprador
	// Numa transmissão intracomunitária, a fatura indica o número de IVA das
	// duas partes (regra BR-IC-02): o NIF da fábrica, com o prefixo PT, e o
	// número de IVA intracomunitário do château. O capital social e a
	// conservatória do registo comercial, que as sociedades portuguesas
	// indicam nos seus documentos, vão na informação jurídica do vendedor
	// (BT-33).
	fabrica := invoice.Party{
		Name:        "Rolhas Teixeira & Filhas, Lda.",
		VATID:       "PT508361729",
		Description: "Capital social: 250 000 €. Matriculada na C.R.C. de Santa Maria da Feira.",
		Address: invoice.Address{
			Line1: "Rua do Montado, 212", PostCode: "4520-211", City: "Santa Maria da Feira",
			CountryCode: "PT",
		},
		Contact: &invoice.Contact{
			PersonName: "Marta Teixeira",
			Phone:      "+351 256 000 000",
			Email:      "comercial@rolhasteixeira.example",
		},
	}
	chateau := invoice.Party{
		Name:              "SCEA Château Mauvesin-Lartigue",
		VATID:             "FR43438162752",
		LegalRegistration: invoice.Identifier{ID: "438162752", Scheme: invoice.SchemeSIREN},
		Address: invoice.Address{
			Line1: "Lieu-dit Mauvesin", PostCode: "33330", City: "Saint-Émilion", CountryCode: "FR",
		},
	}

	// ## Acrescentar as menções obrigatórias
	// O número segue a série do programa certificado (FT 2026/318), e o
	// ATCUD, o código único do documento, vai numa nota com o código REG
	// (informação regulamentar). A isenção cita o artigo 14.º do RITI, o
	// regime do IVA nas transações intracomunitárias, com o código
	// VATEX-EU-IC. A data de entrega (BT-72) e o país de destino (BT-80) são
	// obrigatórios na categoria K.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileEN16931,
		Number:                 "FT 2026/318",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		PurchaseOrderReference: "CMD-2026-044",
		Seller:                 fabrica,
		Buyer:                  chateau,
		DeliveryDate:           invoice.NewDate(2026, time.November, 18),
		ShipTo: &invoice.Party{
			Name: "Château Mauvesin-Lartigue, chai d'embouteillage",
			Address: invoice.Address{
				Line1: "Lieu-dit Mauvesin", PostCode: "33330", City: "Saint-Émilion", CountryCode: "FR",
			},
		},
		Notes: []invoice.Note{{SubjectCode: "REG", Content: "ATCUD: JJMRT4WK-318"}},
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReason:     "Isento nos termos do artigo 14.º do RITI.",
			ExemptionReasonCode: invoice.VATEXIntraCommunity,
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:  invoice.PaymentSEPACreditTransfer,
			PayeeIBAN: "PT50000201231234567890154",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Pagamento a 60 dias da data da fatura, por transferência bancária.",
			DueDate:     invoice.NewDate(2027, time.January, 19),
		}},
	}

	// ## Listar as rolhas
	// As rolhas vendem-se ao milheiro: a quantidade conta-se em peças (código
	// UN/ECE H87) e o preço refere-se a mil rolhas, a quantidade de base do
	// preço (BT-149). A marcação a fogo também se cobra ao milheiro, e o
	// transporte como uma unidade (C62).
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Rolha natural 49 × 24 mm, classe Flor"},
		Quantity: d("24000"), UnitCode: "H87", NetPrice: d("980.00"),
		PriceBaseQuantity: d("1000"), PriceBaseQuantityUnit: "H87",
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Rolha natural 45 × 24 mm, classe Extra"},
		Quantity: d("12000"), UnitCode: "H87", NetPrice: d("620.00"),
		PriceBaseQuantity: d("1000"), PriceBaseQuantityUnit: "H87",
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Marcação a fogo do brasão e do ano de colheita"},
		Quantity: d("36000"), UnitCode: "H87", NetPrice: d("18.00"),
		PriceBaseQuantity: d("1000"), PriceBaseQuantityUnit: "H87",
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Transporte rodoviário até Saint-Émilion"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("420.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Criar a fatura Factur-X
	// Calculate calcula os montantes das linhas (dividindo pela quantidade de
	// base), os totais e o resumo do IVA, com decimais exatos. Com Auto, a
	// validação acrescentaria as regras nacionais do país do vendedor. Para
	// Portugal não há nenhuma: a CIUS-PT, a especificação portuguesa das
	// faturas ao Estado, não está implementada. Aplicam-se o esquema, as
	// regras EN 16931 e as do perfil Factur-X. Uma fatura inválida é
	// recusada: Create devolve os erros e não escreve nada.
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{
		Calculate: true,
		Validate:  &validate.Options{Auto: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("FT-2026-318.pdf", out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Reler e verificar o resultado
	// O PDF produzido lê-se como qualquer fatura recebida. Check verifica o
	// contentor PDF, os metadados XMP e o XML, com as mesmas regras.
	back, att, err := zerechnung.Read(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, "·", att.Metadata.ConformanceLevel)
	l := back.Lines[0]
	fmt.Printf("%s: %s × %s €/%s = %s €\n",
		l.Item.Name, l.Quantity, l.NetPrice, l.PriceBaseQuantity, l.NetAmount)
	vat := back.VATBreakdown[0]
	fmt.Println("IVA", vat.CategoryCode, vat.Rate, "% ·", vat.ExemptionReason)
	fmt.Println("Total:", back.Totals.GrandTotal, "€ · vencimento:", back.PaymentTerms[0].DueDate)

	report, _, err := zerechnung.Check(out, &validate.Options{Auto: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Erros:", len(report.Errors()))
}

// Output:
// factur-x.xml · EN 16931
// Rolha natural 49 × 24 mm, classe Flor: 24000 × 980.00 €/1000 = 23520.00 €
// IVA K 0 % · Isento nos termos do artigo 14.º do RITI.
// Total: 32028.00 € · vencimento: 2027-01-19
// Erros: 0
