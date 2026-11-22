// Title: Uma oficina de azulejos de Lisboa fatura a uma loja de Bruxelas por Peppol
// Tags: Peppol BIS Billing 3.0, UBL, Bélgica, Transmissão intracomunitária, IVA 0 %
//
// Na oficina de Leonor Bastos, em Alfama, os azulejos ainda se pintam à
// mão, um a um, na tradição dos padrões pombalinos e das figuras avulsas
// do século XVIII. A Maison Lemaire & Benali, uma loja de decoração de Ixelles, em
// Bruxelas, encomendou-lhe azulejos para revenda e um painel com uma vista
// do Tejo, pintado por encomenda.
//
// Desde 1 de janeiro de 2026, a fatura eletrónica em Peppol é obrigatória
// entre empresas belgas, e a loja recebe as faturas por essa rede. A
// oficina, que não tem essa obrigação, segue o mesmo caminho: uma fatura
// Peppol BIS Billing 3.0 em UBL, validada com as regras Peppol, sem IVA
// português por se tratar de uma transmissão intracomunitária.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

func main() {
	// ## Identificar as partes na rede Peppol
	// Na rede Peppol, cada parte tem um endereço eletrónico (BT-34, BT-49)
	// com o seu esquema. A oficina usa o seu número de IVA português
	// (esquema 9946). A loja usa o número de empresa belga (esquema 0208),
	// cujos dois últimos algarismos são 97 menos o resto da divisão por 97
	// dos oito primeiros: a regra PEPPOL-COMMON-R043 verifica-o.
	oficina := invoice.Party{
		Name:  "Oficina de Azulejos Leonor Bastos, Lda.",
		VATID: "PT514729384",
		Address: invoice.Address{
			Line1: "Rua dos Remédios, 87", PostCode: "1100-443", City: "Lisboa", CountryCode: "PT",
		},
		Contact: &invoice.Contact{
			PersonName: "Leonor Bastos", Email: "encomendas@leonorbastos.example",
		},
		ElectronicAddress: invoice.Identifier{ID: "PT514729384", Scheme: invoice.SchemePTVAT},
	}
	loja := invoice.Party{
		Name:              "Maison Lemaire & Benali SRL",
		VATID:             "BE0728391608",
		LegalRegistration: invoice.Identifier{ID: "0728391608", Scheme: invoice.SchemeBEEnterprise},
		Address: invoice.Address{
			Line1: "Rue du Bailli 54", PostCode: "1050", City: "Ixelles", CountryCode: "BE",
		},
		ElectronicAddress: invoice.Identifier{ID: "0728391608", Scheme: invoice.SchemeBEEnterprise},
	}

	// ## Preencher o cabeçalho Peppol
	// O Peppol exige o processo de negócio (BT-23) e uma referência do
	// comprador ou da sua encomenda (BT-10 ou BT-13), e só admite uma nota
	// no cabeçalho: aqui o ATCUD, o código único do documento. O perfil
	// ProfilePeppol escreve o identificador da especificação (BT-24). A
	// isenção cita o artigo 14.º do RITI; a data e o local de entrega são
	// obrigatórios numa transmissão intracomunitária.
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:                invoice.ProfilePeppol,
		BusinessProcess:        invoice.ProcessPeppolBilling,
		Number:                 "FT 2026/1209",
		TypeCode:               invoice.TypeCommercialInvoice,
		IssueDate:              invoice.NewDate(2026, time.November, 20),
		Currency:               "EUR",
		BuyerReference:         "ACHATS-DECO",
		PurchaseOrderReference: "BC-2026-0093",
		Seller:                 oficina,
		Buyer:                  loja,
		DeliveryDate:           invoice.NewDate(2026, time.November, 16),
		ShipTo: &invoice.Party{
			Name: "Maison Lemaire & Benali",
			Address: invoice.Address{
				Line1: "Rue du Bailli 54", PostCode: "1050", City: "Ixelles", CountryCode: "BE",
			},
		},
		Notes: []invoice.Note{{SubjectCode: "REG", Content: "ATCUD: KD7Q2MPA-1209"}},
		VATBreakdown: []invoice.VATBreakdown{{
			CategoryCode:        invoice.VATIntraCommunity,
			Rate:                d("0"),
			ExemptionReason:     "Isento nos termos do artigo 14.º do RITI.",
			ExemptionReasonCode: invoice.VATEXIntraCommunity,
		}},
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode:         invoice.PaymentSEPACreditTransfer,
			PayeeIBAN:        "PT50000201231234567890154",
			PayeeAccountName: "Oficina de Azulejos Leonor Bastos",
		}},
		PaymentTerms: []invoice.PaymentTerms{{
			Description: "Pagamento a 30 dias, por transferência bancária.",
			DueDate:     invoice.NewDate(2026, time.December, 20),
		}},
	}

	// ## Listar os azulejos
	// Os azulejos contam-se à peça (código UN/ECE H87) e o painel como uma
	// unidade (C62). Todas as linhas levam a categoria K e a taxa de 0 %.
	inv.Lines = []invoice.Line{{
		ID: "1", Item: invoice.Item{Name: "Azulejo pintado à mão, padrão pombalino, 14 × 14 cm"},
		Quantity: d("800"), UnitCode: "H87", NetPrice: d("6.80"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "2", Item: invoice.Item{Name: "Azulejo de figura avulsa, aves e flores, 14 × 14 cm"},
		Quantity: d("240"), UnitCode: "H87", NetPrice: d("9.50"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		ID: "3", Item: invoice.Item{Name: "Painel «Vista do Tejo», 6 × 8 azulejos, por encomenda"},
		Quantity: d("1"), UnitCode: "C62", NetPrice: d("1450.00"),
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Acrescentar a embalagem e o transporte
	// A embalagem em grades de madeira e o transporte até Bruxelas são
	// encargos ao nível do documento (BG-21), com o motivo em texto e o seu
	// código UNTDID 7161: PC para a embalagem, FC para o frete. Levam a mesma
	// categoria de IVA que os azulejos e entram no total dos encargos
	// (BT-108).
	inv.AllowancesCharges = []invoice.AllowanceCharge{{
		Charge: true, Amount: d("185.00"),
		Reason: "Embalagem em grades de madeira", ReasonCode: "PC",
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}, {
		Charge: true, Amount: d("320.00"),
		Reason: "Transporte Lisboa-Bruxelas", ReasonCode: "FC",
		VATCategoryCode: invoice.VATIntraCommunity, VATRate: d("0"),
	}}

	// ## Calcular e escrever o UBL
	// Calculate calcula os montantes das linhas, os totais e o resumo do IVA,
	// com decimais exatos. ubl.Marshal escreve a fatura em UBL 2.1, a sintaxe
	// que circula na rede Peppol.
	inv.Calculate()
	xml, err := ubl.Marshal(inv, &ubl.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("FT-2026-1209.xml", xml, 0o644); err != nil {
		log.Fatal(err)
	}

	// ## Validar com as regras Peppol e reler
	// As regras Peppol juntam-se ao esquema UBL e às regras EN 16931. Uma
	// fatura com erros não deve sair: o programa mostra-os e para. A loja
	// lerá o UBL com ubl.Parse, ou com o seu próprio programa.
	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.Peppol})
	if !report.Valid() {
		for _, f := range report.Errors() {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	back, err := ubl.Parse(xml)
	if err != nil {
		log.Fatal(err)
	}
	from, to := back.Seller.ElectronicAddress, back.Buyer.ElectronicAddress
	fmt.Println(report.Profile, "·", from.Scheme+":"+from.ID, "→", to.Scheme+":"+to.ID)
	t := back.Totals
	fmt.Println("Linhas:", t.LineTotal, "€ · embalagem e transporte:", t.ChargeTotal, "€")
	fmt.Println("Total:", t.GrandTotal, "€ ·", back.VATBreakdown[0].ExemptionReason)
	fmt.Println("Erros:", len(report.Errors()), "· avisos:", len(report.Warnings()))
}

// Output:
// PEPPOL · 9946:PT514729384 → 0208:0728391608
// Linhas: 9170.00 € · embalagem e transporte: 505.00 €
// Total: 9675.00 € · Isento nos termos do artigo 14.º do RITI.
// Erros: 0 · avisos: 0
