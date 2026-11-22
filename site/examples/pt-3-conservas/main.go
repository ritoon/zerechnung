// Title: Uma conserveira de Matosinhos envia uma XRechnung a cantinas de Berlim
// Tags: XRechnung, CII, Leitweg-ID, Desconto de pronto pagamento, Transmissão intracomunitária
//
// Em Matosinhos, as fábricas de conservas trabalham a sardinha desde o fim
// do século XIX. A Conserveira Marés de Leça mantém o enlatamento à mão: o
// peixe é limpo, cozido e arrumado em cada lata antes do azeite. O
// Studierendenwerk Spreebogen, o organismo público que gere cantinas
// universitárias em Berlim, encomendou-lhe conservas para uma semana de
// cozinha portuguesa.
//
// Os organismos públicos alemães recebem as faturas em XRechnung: um
// ficheiro XML, sem PDF, endereçado pela Leitweg-ID do serviço destinatário.
// O ZeRechnung escreve-a em CII e verifica-a com as regras alemãs (BR-DE),
// escritas a partir dos artefactos da KoSIT. A fatura não leva IVA
// português, por se tratar de uma transmissão intracomunitária.
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
	// ## Descrever a conserveira
	// A XRechnung pede mais do que a EN 16931: um contacto do vendedor com
	// nome, telefone e endereço de correio eletrónico (BR-DE-2 e BR-DE-5 a
	// BR-DE-7) e um endereço eletrónico (BT-34), aqui um e-mail (esquema
	// EM). O número de IVA, com o prefixo PT, é obrigatório na categoria K.
	conserveira := invoice.Party{
		Name:  "Conserveira Marés de Leça, Lda.",
		VATID: "PT502618477",
		Address: invoice.Address{
			Line1: "Rua das Traineiras, 45", PostCode: "4450-208", City: "Matosinhos",
			CountryCode: "PT",
		},
		Contact: &invoice.Contact{
			PersonName: "Beatriz Monteiro",
			Phone:      "+351 229 000 000",
			Email:      "beatriz.monteiro@maresdeleca.example",
		},
		ElectronicAddress: invoice.Identifier{
			ID: "faturas@maresdeleca.example", Scheme: invoice.SchemeEmail,
		},
	}

	// ## Endereçar o comprador público pela Leitweg-ID
	// A Leitweg-ID identifica o serviço que recebe a fatura; começa pelo
	// código do Land, 11 para Berlim. Vai na referência do comprador (BT-10,
	// obrigatória pela regra BR-DE-15) e no endereço eletrónico do
	// comprador, com o esquema 0204. O ZeRechnung verifica os seus dois
	// algarismos de controlo (ISO 7064 MOD 97-10). O número de IVA alemão
	// do comprador é obrigatório numa transmissão intracomunitária.
	leitwegID := "11001234-MENSEN-06"
	cantinas := invoice.Party{
		Name:  "Studierendenwerk Spreebogen, Anstalt des öffentlichen Rechts",
		VATID: "DE274193562",
		Address: invoice.Address{
			Line1: "Am Spreebogen 5", PostCode: "10557", City: "Berlin", CountryCode: "DE",
		},
		ElectronicAddress: invoice.Identifier{ID: leitwegID, Scheme: invoice.SchemeLeitwegID},
	}

	// ## Preencher as menções da fatura
	// A fatura cita a encomenda (BT-13), a data de entrega (BT-72) e o
	// armazém da cantina (BG-15, com localidade e código postal pelas regras
	// BR-DE-10 e BR-DE-11). O ATCUD vai numa nota; a isenção cita o artigo
	// 14.º do RITI. O desconto de pronto pagamento escreve-se numa linha
	// própria dos termos de pagamento (BT-20), na forma que a XRechnung
	// define e terminada por uma quebra de linha (BR-DE-18): a contabilidade
	// do comprador calcula-o sozinha.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfileXRechnung,
		Number:                 "FT 2026/2045",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		BuyerReference:         leitwegID,
		PurchaseOrderReference: "BEST-2026-3317",
		Seller:                 conserveira,
		Buyer:                  cantinas,
		DeliveryDate:           invoice.NewDate(2026, time.November, 16),
		ShipTo: &invoice.Party{
			Name: "Mensa Spreebogen, Wareneingang",
			Address: invoice.Address{
				Line1: "Am Spreebogen 5", PostCode: "10557", City: "Berlin", CountryCode: "DE",
			},
		},
		Notes: []invoice.Note{{SubjectCode: "REG", Content: "ATCUD: MR4TQ9XC-2045"}},
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReason:     "Isento nos termos do artigo 14.º do RITI.",
			ExemptionReasonCode: invoice.VATEXIntraCommunity,
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:         invoice.PaymentSEPACreditTransfer,
			PayeeIBAN:        "PT50000201231234567890154",
			PayeeAccountName: "Conserveira Marés de Leça, Lda.",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Pagamento a 30 dias, por transferência bancária.\n" +
				"#SKONTO#TAGE=10#PROZENT=2.00#\n",
			DueDate: invoice.NewDate(2026, time.December, 20),
		}},
	}

	// ## Listar as conservas
	// As latas contam-se à peça (código UN/ECE H87) e o transporte como uma
	// unidade (C62). Todas as linhas levam a categoria K e a taxa de 0 %; a
	// XRechnung exige que a taxa figure no resumo do IVA mesmo quando é zero
	// (BR-DE-14).
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Sardinhas em azeite virgem extra, lata de 120 g"},
		Quantity: d("1200"), UnitCode: "H87", NetPrice: d("1.85"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Sardinhas em azeite com piri-piri, lata de 120 g"},
		Quantity: d("600"), UnitCode: "H87", NetPrice: d("1.95"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Filetes de cavala em azeite, lata de 120 g"},
		Quantity: d("600"), UnitCode: "H87", NetPrice: d("2.10"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "4", Item: invoice.Item{Name: "Paté de sardinha, lata de 75 g"},
		Quantity: d("480"), UnitCode: "H87", NetPrice: d("1.20"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "5", Item: invoice.Item{Name: "Transporte rodoviário, Matosinhos-Berlim"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("380.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Calcular e escrever a XRechnung
	// Calculate calcula os montantes, os totais e o resumo do IVA.
	// cii.Marshal escreve a fatura na sintaxe CII, com o identificador da
	// XRechnung 3.0 (BT-24); ubl.Marshal escreveria a mesma fatura em UBL,
	// que os organismos públicos alemães também aceitam.
	inv.Calculate()
	xml, err := cii.Marshal(inv, &cii.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("FT-2026-2045.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Validar com as regras alemãs e reler
	// As regras alemãs (o conjunto Germany: as regras BR-DE e a verificação
	// da Leitweg-ID) aplicam-se sozinhas a uma XRechnung, que se declara em
	// BT-24; aqui pedem-se de forma explícita. Uma fatura com erros não deve
	// sair: o programa mostra-os e para. Depois, cii.Parse relê o XML como o
	// comprador o lerá.
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.Germany})
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	back, err := cii.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	t := back.Totals
	fmt.Println(report.Profile, "· Leitweg-ID", back.BuyerReference)
	fmt.Println("Total:", t.GrandTotal, "€ · IVA:", t.TaxTotal, "€ ·",
		back.VATBreakdown[0].ExemptionReasonCode)
	desconto := t.DuePayable.Mul(d("0.02")).Round(2)
	fmt.Printf("Com 2 %% de desconto até %s: %s €\n",
		back.IssueDate.AddDays(10), t.DuePayable.Sub(desconto))
	fmt.Println("Erros:", len(report.Errors()), "· avisos:", len(report.Warnings()))
}

// Output:
// XRECHNUNG · Leitweg-ID 11001234-MENSEN-06
// Total: 5606.00 € · IVA: 0.00 € · VATEX-EU-IC
// Com 2 % de desconto até 2026-11-30: 5493.88 €
// Erros: 0 · avisos: 0
