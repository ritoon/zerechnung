package render

// labels are the texts of the document in one language.
type labels struct {
	// Number formatting: decimal and thousands separators, and whether the
	// currency comes before the amount.
	decimal, thousands string
	currencyFirst      bool
	percent            string // between the rate and the % sign
	colon              string // between a label and its value
	dateFormat         string // time layout; "" for "2 Jan 2006" with month names
	months             [12]string

	invoice, creditNote, correctedInvoice, prepaymentInvoice, selfBilledInvoice string
	partialInvoice, debitNote                                                   string
	copy, test                                                                  string

	number, issueDate, dueDate, deliveryDate, period, taxPointDate string
	buyerReference, orderReference, salesOrder, contractReference  string
	projectReference, precedingInvoice, despatchAdvice, tender     string
	billTo, shipTo, payee, seller                                  string
	vatID, taxNumber, registration, phone, email, contact          string

	colNo, colDescription, colQty, colUnit, colUnitPrice, colVAT, colAmount string
	itemNo, origin                                                          string

	allowance, charge, allowancesCharges                   string
	vatBreakdown, vatCategory, vatRate, taxable, vatAmount string
	lineTotal, allowanceTotal, chargeTotal, totalNet       string
	totalVAT, totalGross, paid, rounding, due              string

	payment, paymentMeans, iban, bic, accountName, accountID string
	paymentReference                                         string
	mandate, creditorID, debitedAccount, card, notes, page   string
	continued                                                string

	// categories are the VAT categories (UNTDID 5305). S is the category of
	// every positive rate, reduced ones included: its label does not say
	// "standard rate", the rate column gives the rate.
	categories map[string]string
	means      map[string]string // payment means (UNTDID 4461)
	units      map[string]string // units of measure (UN/ECE Rec 20)
}

// translations holds the texts of the supported languages.
var translations = map[string]*labels{
	"en": {
		decimal: ".", thousands: ",", currencyFirst: true, percent: "", colon: ": ",
		months: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},

		invoice: "Invoice", creditNote: "Credit note", correctedInvoice: "Corrected invoice",
		prepaymentInvoice: "Prepayment invoice", selfBilledInvoice: "Self-billed invoice",
		partialInvoice: "Partial invoice", debitNote: "Debit note", copy: "Copy", test: "Test document",

		number: "No.", issueDate: "Issue date", dueDate: "Due date", deliveryDate: "Delivery date",
		period: "Period", taxPointDate: "VAT point date",
		buyerReference: "Buyer reference", orderReference: "Purchase order", salesOrder: "Sales order",
		contractReference: "Contract", projectReference: "Project", precedingInvoice: "Preceding invoice",
		despatchAdvice: "Despatch advice", tender: "Tender",
		billTo: "Bill to", shipTo: "Deliver to", payee: "Payee", seller: "Seller",
		vatID: "VAT ID", taxNumber: "Tax no.", registration: "Reg. no.", phone: "Phone", email: "Email", contact: "Contact",

		colNo: "#", colDescription: "Description", colQty: "Qty", colUnit: "Unit", colUnitPrice: "Unit price",
		colVAT: "VAT", colAmount: "Amount", itemNo: "Item no.", origin: "Origin",

		allowance: "Allowance", charge: "Charge", allowancesCharges: "Allowances and charges",
		vatBreakdown: "VAT breakdown", vatCategory: "Category", vatRate: "Rate", taxable: "Taxable amount", vatAmount: "VAT",
		lineTotal: "Sum of lines", allowanceTotal: "Allowances", chargeTotal: "Charges", totalNet: "Total excl. VAT",
		totalVAT: "VAT", totalGross: "Total incl. VAT", paid: "Paid", rounding: "Rounding", due: "Amount due",

		payment: "Payment", paymentMeans: "Means of payment", iban: "IBAN", bic: "BIC", accountName: "Account name", accountID: "Account",
		paymentReference: "Payment reference", mandate: "Mandate reference", creditorID: "Creditor ID",
		debitedAccount: "Debited account", card: "Card", notes: "Notes", page: "Page", continued: "continued",

		categories: map[string]string{
			"S": "Taxable", "Z": "Zero rated", "E": "Exempt", "AE": "Reverse charge",
			"K": "Intra-community supply", "G": "Export outside the EU", "O": "Not subject to VAT",
			"L": "Canary Islands (IGIC)", "M": "Ceuta and Melilla (IPSI)", "B": "Split payment",
		},
		means: map[string]string{
			"1": "Instrument not defined", "10": "Cash", "20": "Cheque", "30": "Credit transfer", "31": "Debit transfer",
			"42": "Payment to bank account", "48": "Bank card", "49": "Direct debit", "54": "Credit card",
			"55": "Debit card", "57": "Standing agreement", "58": "SEPA credit transfer", "59": "SEPA direct debit",
			"68": "Online payment service", "97": "Clearing between partners",
		},
		units: map[string]string{
			"C62": "pcs", "H87": "pcs", "EA": "pcs", "XPP": "pcs", "HUR": "h", "MIN": "min", "DAY": "day",
			"WEE": "week", "MON": "month", "ANN": "year", "LS": "lump sum", "SET": "set", "XBX": "box",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"fr": {
		decimal: ",", thousands: " ", percent: " ", colon: " : ", dateFormat: "02/01/2006",

		invoice: "Facture", creditNote: "Avoir", correctedInvoice: "Facture rectificative",
		prepaymentInvoice: "Facture d'acompte", selfBilledInvoice: "Autofacture",
		partialInvoice: "Facture partielle", debitNote: "Note de débit", copy: "Duplicata", test: "Document de test",

		number: "N°", issueDate: "Date d'émission", dueDate: "Échéance", deliveryDate: "Date de livraison",
		period: "Période", taxPointDate: "Date d'exigibilité de la TVA",
		buyerReference: "Référence acheteur", orderReference: "Bon de commande", salesOrder: "Commande client",
		contractReference: "Contrat", projectReference: "Projet", precedingInvoice: "Facture d'origine",
		despatchAdvice: "Avis d'expédition", tender: "Marché",
		billTo: "Facturé à", shipTo: "Livré à", payee: "Bénéficiaire", seller: "Vendeur",
		vatID: "N° TVA", taxNumber: "N° fiscal", registration: "N° d'immatriculation", phone: "Tél.", email: "E-mail", contact: "Contact",

		colNo: "N°", colDescription: "Désignation", colQty: "Qté", colUnit: "Unité", colUnitPrice: "Prix unit. HT",
		colVAT: "TVA", colAmount: "Montant HT", itemNo: "Réf.", origin: "Origine",

		allowance: "Remise", charge: "Frais", allowancesCharges: "Remises et frais",
		vatBreakdown: "Détail de la TVA", vatCategory: "Catégorie", vatRate: "Taux", taxable: "Base HT", vatAmount: "TVA",
		lineTotal: "Total des lignes", allowanceTotal: "Remises", chargeTotal: "Frais", totalNet: "Total HT",
		totalVAT: "TVA", totalGross: "Total TTC", paid: "Déjà payé", rounding: "Arrondi", due: "Net à payer",

		payment: "Paiement", paymentMeans: "Mode de paiement", iban: "IBAN", bic: "BIC", accountName: "Titulaire", accountID: "Compte",
		paymentReference: "Référence de paiement", mandate: "Référence du mandat", creditorID: "ICS",
		debitedAccount: "Compte débité", card: "Carte", notes: "Mentions", page: "Page", continued: "suite",

		categories: map[string]string{
			"S": "Soumis à TVA", "Z": "Taux zéro", "E": "Exonéré", "AE": "Autoliquidation",
			"K": "Livraison intracommunautaire", "G": "Exportation hors UE", "O": "Hors champ de la TVA",
			"L": "Îles Canaries (IGIC)", "M": "Ceuta et Melilla (IPSI)", "B": "Paiement scindé",
		},
		means: map[string]string{
			"1": "Moyen non défini", "10": "Espèces", "20": "Chèque", "30": "Virement", "31": "Prélèvement",
			"42": "Paiement sur compte bancaire", "48": "Carte bancaire", "49": "Prélèvement", "54": "Carte de crédit",
			"55": "Carte de débit", "57": "Accord permanent", "58": "Virement SEPA", "59": "Prélèvement SEPA",
			"68": "Service de paiement en ligne", "97": "Compensation",
		},
		units: map[string]string{
			"C62": "u", "H87": "pièce", "EA": "u", "XPP": "u", "HUR": "h", "MIN": "min", "DAY": "jour",
			"WEE": "semaine", "MON": "mois", "ANN": "an", "LS": "forfait", "SET": "lot", "XBX": "boîte",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"de": {
		decimal: ",", thousands: ".", percent: " ", colon: ": ", dateFormat: "02.01.2006",

		invoice: "Rechnung", creditNote: "Gutschrift", correctedInvoice: "Rechnungskorrektur",
		prepaymentInvoice: "Vorauszahlungsrechnung", selfBilledInvoice: "Gutschrift (Selbstfakturierung)",
		partialInvoice: "Teilrechnung", debitNote: "Belastungsanzeige", copy: "Kopie", test: "Testdokument",

		number: "Nr.", issueDate: "Rechnungsdatum", dueDate: "Fällig am", deliveryDate: "Lieferdatum",
		period: "Leistungszeitraum", taxPointDate: "Steuerzeitpunkt",
		buyerReference: "Käuferreferenz", orderReference: "Bestellung", salesOrder: "Auftrag",
		contractReference: "Vertrag", projectReference: "Projekt", precedingInvoice: "Ursprungsrechnung",
		despatchAdvice: "Lieferavis", tender: "Vergabe",
		billTo: "Rechnungsempfänger", shipTo: "Lieferanschrift", payee: "Zahlungsempfänger", seller: "Verkäufer",
		vatID: "USt-IdNr.", taxNumber: "Steuernummer", registration: "Register-Nr.", phone: "Tel.", email: "E-Mail", contact: "Kontakt",

		colNo: "Pos.", colDescription: "Bezeichnung", colQty: "Menge", colUnit: "Einheit", colUnitPrice: "Einzelpreis",
		colVAT: "USt.", colAmount: "Netto", itemNo: "Art.-Nr.", origin: "Herkunft",

		allowance: "Nachlass", charge: "Zuschlag", allowancesCharges: "Nachlässe und Zuschläge",
		vatBreakdown: "Umsatzsteuer", vatCategory: "Kategorie", vatRate: "Satz", taxable: "Netto", vatAmount: "USt.",
		lineTotal: "Summe Positionen", allowanceTotal: "Nachlässe", chargeTotal: "Zuschläge", totalNet: "Gesamt netto",
		totalVAT: "Umsatzsteuer", totalGross: "Gesamt brutto", paid: "Bereits gezahlt", rounding: "Rundung", due: "Zahlbetrag",

		payment: "Zahlung", paymentMeans: "Zahlungsart", iban: "IBAN", bic: "BIC", accountName: "Kontoinhaber", accountID: "Konto",
		paymentReference: "Verwendungszweck", mandate: "Mandatsreferenz", creditorID: "Gläubiger-ID",
		debitedAccount: "Belastetes Konto", card: "Karte", notes: "Hinweise", page: "Seite", continued: "Fortsetzung",

		categories: map[string]string{
			"S": "Steuerpflichtig", "Z": "Nullsatz", "E": "Steuerbefreit", "AE": "Steuerschuldnerschaft des Leistungsempfängers",
			"K": "Innergemeinschaftliche Lieferung", "G": "Ausfuhr", "O": "Nicht steuerbar",
			"L": "Kanarische Inseln (IGIC)", "M": "Ceuta und Melilla (IPSI)", "B": "Split Payment",
		},
		means: map[string]string{
			"1": "Nicht definiert", "10": "Bar", "20": "Scheck", "30": "Überweisung", "31": "Lastschrift",
			"42": "Zahlung auf Bankkonto", "48": "Bankkarte", "49": "Lastschrift", "54": "Kreditkarte",
			"55": "Debitkarte", "57": "Dauerauftrag", "58": "SEPA-Überweisung", "59": "SEPA-Lastschrift",
			"68": "Online-Zahlungsdienst", "97": "Verrechnung",
		},
		units: map[string]string{
			"C62": "Stk.", "H87": "Stk.", "EA": "Stk.", "XPP": "Stk.", "HUR": "Std.", "MIN": "Min.", "DAY": "Tag",
			"WEE": "Woche", "MON": "Monat", "ANN": "Jahr", "LS": "pauschal", "SET": "Satz", "XBX": "Karton",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"it": {
		decimal: ",", thousands: ".", percent: "", colon: ": ", dateFormat: "02/01/2006",

		invoice: "Fattura", creditNote: "Nota di credito", correctedInvoice: "Fattura rettificativa",
		prepaymentInvoice: "Fattura di acconto", selfBilledInvoice: "Autofattura",
		partialInvoice: "Fattura parziale", debitNote: "Nota di debito", copy: "Copia", test: "Documento di prova",

		number: "N.", issueDate: "Data", dueDate: "Scadenza", deliveryDate: "Data di consegna",
		period: "Periodo", taxPointDate: "Esigibilità IVA",
		buyerReference: "Riferimento acquirente", orderReference: "Ordine d'acquisto", salesOrder: "Ordine di vendita",
		contractReference: "Contratto", projectReference: "Progetto", precedingInvoice: "Fattura di riferimento",
		despatchAdvice: "DDT", tender: "Gara (CIG)",
		billTo: "Destinatario", shipTo: "Luogo di consegna", payee: "Beneficiario", seller: "Cedente",
		vatID: "Partita IVA", taxNumber: "Codice fiscale", registration: "N. REA", phone: "Tel.", email: "E-mail", contact: "Contatto",

		colNo: "N.", colDescription: "Descrizione", colQty: "Q.tà", colUnit: "U.M.", colUnitPrice: "Prezzo unit.",
		colVAT: "IVA", colAmount: "Importo", itemNo: "Cod.", origin: "Origine",

		allowance: "Sconto", charge: "Maggiorazione", allowancesCharges: "Sconti e maggiorazioni",
		vatBreakdown: "Riepilogo IVA", vatCategory: "Natura", vatRate: "Aliquota", taxable: "Imponibile", vatAmount: "Imposta",
		lineTotal: "Totale righe", allowanceTotal: "Sconti", chargeTotal: "Maggiorazioni", totalNet: "Totale imponibile",
		totalVAT: "IVA", totalGross: "Totale documento", paid: "Già pagato", rounding: "Arrotondamento", due: "Netto a pagare",

		payment: "Pagamento", paymentMeans: "Modalità", iban: "IBAN", bic: "BIC", accountName: "Intestatario", accountID: "Conto",
		paymentReference: "Causale", mandate: "Riferimento mandato", creditorID: "Codice creditore",
		debitedAccount: "Conto addebitato", card: "Carta", notes: "Note", page: "Pagina", continued: "segue",

		categories: map[string]string{
			"S": "Soggetto a IVA", "Z": "Aliquota zero", "E": "Esente", "AE": "Inversione contabile",
			"K": "Cessione intracomunitaria", "G": "Esportazione", "O": "Fuori campo IVA",
			"L": "Isole Canarie (IGIC)", "M": "Ceuta e Melilla (IPSI)", "B": "Scissione dei pagamenti",
		},
		means: map[string]string{
			"1": "Non definito", "10": "Contanti", "20": "Assegno", "30": "Bonifico", "31": "Addebito",
			"42": "Versamento su conto", "48": "Carta bancaria", "49": "Addebito diretto", "54": "Carta di credito",
			"55": "Carta di debito", "57": "Accordo permanente", "58": "Bonifico SEPA", "59": "Addebito diretto SEPA",
			"68": "Servizio di pagamento online", "97": "Compensazione",
		},
		units: map[string]string{
			"C62": "pz", "H87": "pz", "EA": "pz", "XPP": "pz", "HUR": "h", "MIN": "min", "DAY": "giorno",
			"WEE": "settimana", "MON": "mese", "ANN": "anno", "LS": "a corpo", "SET": "set", "XBX": "scatola",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"es": {
		decimal: ",", thousands: ".", percent: " ", colon: ": ", dateFormat: "02/01/2006",

		invoice: "Factura", creditNote: "Abono", correctedInvoice: "Factura rectificativa",
		prepaymentInvoice: "Factura de anticipo", selfBilledInvoice: "Autofactura",
		partialInvoice: "Factura parcial", debitNote: "Nota de cargo", copy: "Copia", test: "Documento de prueba",

		number: "N.º", issueDate: "Fecha de emisión", dueDate: "Vencimiento", deliveryDate: "Fecha de entrega",
		period: "Periodo", taxPointDate: "Devengo del IVA",
		buyerReference: "Referencia del comprador", orderReference: "Pedido", salesOrder: "Pedido de venta",
		contractReference: "Contrato", projectReference: "Proyecto", precedingInvoice: "Factura rectificada",
		despatchAdvice: "Albarán", tender: "Licitación",
		billTo: "Facturar a", shipTo: "Entregar en", payee: "Beneficiario", seller: "Vendedor",
		vatID: "NIF-IVA", taxNumber: "NIF", registration: "N.º de registro", phone: "Tel.", email: "Correo", contact: "Contacto",

		colNo: "N.º", colDescription: "Descripción", colQty: "Cant.", colUnit: "Unidad", colUnitPrice: "Precio unit.",
		colVAT: "IVA", colAmount: "Importe", itemNo: "Ref.", origin: "Origen",

		allowance: "Descuento", charge: "Cargo", allowancesCharges: "Descuentos y cargos",
		vatBreakdown: "Desglose del IVA", vatCategory: "Categoría", vatRate: "Tipo", taxable: "Base imponible", vatAmount: "Cuota",
		lineTotal: "Suma de líneas", allowanceTotal: "Descuentos", chargeTotal: "Cargos", totalNet: "Total sin IVA",
		totalVAT: "IVA", totalGross: "Total con IVA", paid: "Pagado", rounding: "Redondeo", due: "Total a pagar",

		payment: "Pago", paymentMeans: "Forma de pago", iban: "IBAN", bic: "BIC", accountName: "Titular", accountID: "Cuenta",
		paymentReference: "Concepto", mandate: "Referencia del mandato", creditorID: "Identificador del acreedor",
		debitedAccount: "Cuenta de cargo", card: "Tarjeta", notes: "Observaciones", page: "Página", continued: "continuación",

		categories: map[string]string{
			"S": "Sujeto a IVA", "Z": "Tipo cero", "E": "Exento", "AE": "Inversión del sujeto pasivo",
			"K": "Entrega intracomunitaria", "G": "Exportación", "O": "No sujeto a IVA",
			"L": "Canarias (IGIC)", "M": "Ceuta y Melilla (IPSI)", "B": "Escisión de pagos",
		},
		means: map[string]string{
			"1": "No definido", "10": "Efectivo", "20": "Cheque", "30": "Transferencia", "31": "Adeudo",
			"42": "Ingreso en cuenta", "48": "Tarjeta bancaria", "49": "Domiciliación", "54": "Tarjeta de crédito",
			"55": "Tarjeta de débito", "57": "Orden permanente", "58": "Transferencia SEPA", "59": "Adeudo directo SEPA",
			"68": "Servicio de pago en línea", "97": "Compensación",
		},
		units: map[string]string{
			"C62": "ud.", "H87": "ud.", "EA": "ud.", "XPP": "ud.", "HUR": "h", "MIN": "min", "DAY": "día",
			"WEE": "semana", "MON": "mes", "ANN": "año", "LS": "global", "SET": "juego", "XBX": "caja",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"pt": {
		decimal: ",", thousands: " ", percent: " ", colon: ": ", dateFormat: "02/01/2006",

		invoice: "Fatura", creditNote: "Nota de crédito", correctedInvoice: "Fatura retificativa",
		prepaymentInvoice: "Fatura de adiantamento", selfBilledInvoice: "Autofaturação",
		partialInvoice: "Fatura parcial", debitNote: "Nota de débito", copy: "Cópia", test: "Documento de teste",

		number: "N.º", issueDate: "Data de emissão", dueDate: "Vencimento", deliveryDate: "Data de entrega",
		period: "Período", taxPointDate: "Exigibilidade do IVA",
		buyerReference: "Referência do comprador", orderReference: "Nota de encomenda", salesOrder: "Encomenda",
		contractReference: "Contrato", projectReference: "Projeto", precedingInvoice: "Fatura de origem",
		despatchAdvice: "Guia de remessa", tender: "Concurso",
		billTo: "Faturar a", shipTo: "Local de entrega", payee: "Beneficiário", seller: "Vendedor",
		vatID: "NIF", taxNumber: "N.º fiscal", registration: "N.º de registo", phone: "Tel.", email: "E-mail", contact: "Contacto",

		colNo: "N.º", colDescription: "Descrição", colQty: "Qtd.", colUnit: "Unid.", colUnitPrice: "Preço unit.",
		colVAT: "IVA", colAmount: "Valor", itemNo: "Ref.", origin: "Origem",

		allowance: "Desconto", charge: "Encargo", allowancesCharges: "Descontos e encargos",
		vatBreakdown: "Resumo do IVA", vatCategory: "Categoria", vatRate: "Taxa", taxable: "Incidência", vatAmount: "IVA",
		lineTotal: "Total das linhas", allowanceTotal: "Descontos", chargeTotal: "Encargos", totalNet: "Total sem IVA",
		totalVAT: "IVA", totalGross: "Total com IVA", paid: "Pago", rounding: "Arredondamento", due: "Total a pagar",

		payment: "Pagamento", paymentMeans: "Meio de pagamento", iban: "IBAN", bic: "BIC", accountName: "Titular", accountID: "Conta",
		paymentReference: "Referência de pagamento", mandate: "Referência do mandato", creditorID: "ID do credor",
		debitedAccount: "Conta debitada", card: "Cartão", notes: "Observações", page: "Página", continued: "continuação",

		categories: map[string]string{
			"S": "Sujeito a IVA", "Z": "Taxa zero", "E": "Isento", "AE": "Autoliquidação",
			"K": "Fornecimento intracomunitário", "G": "Exportação", "O": "Não sujeito a IVA",
			"L": "Canárias (IGIC)", "M": "Ceuta e Melilha (IPSI)", "B": "Pagamento dividido",
		},
		means: map[string]string{
			"1": "Não definido", "10": "Numerário", "20": "Cheque", "30": "Transferência", "31": "Débito",
			"42": "Pagamento em conta bancária", "48": "Cartão bancário", "49": "Débito direto", "54": "Cartão de crédito",
			"55": "Cartão de débito", "57": "Ordem permanente", "58": "Transferência SEPA", "59": "Débito direto SEPA",
			"68": "Serviço de pagamento online", "97": "Compensação",
		},
		units: map[string]string{
			"C62": "un.", "H87": "un.", "EA": "un.", "XPP": "un.", "HUR": "h", "MIN": "min", "DAY": "dia",
			"WEE": "semana", "MON": "mês", "ANN": "ano", "LS": "global", "SET": "conjunto", "XBX": "caixa",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
	"nl": {
		decimal: ",", thousands: ".", percent: "", colon: ": ", dateFormat: "02-01-2006",

		invoice: "Factuur", creditNote: "Creditnota", correctedInvoice: "Gecorrigeerde factuur",
		prepaymentInvoice: "Voorschotfactuur", selfBilledInvoice: "Self-billing factuur",
		partialInvoice: "Deelfactuur", debitNote: "Debetnota", copy: "Kopie", test: "Testdocument",

		number: "Nr.", issueDate: "Factuurdatum", dueDate: "Vervaldatum", deliveryDate: "Leverdatum",
		period: "Periode", taxPointDate: "Btw-tijdstip",
		buyerReference: "Referentie koper", orderReference: "Inkooporder", salesOrder: "Verkooporder",
		contractReference: "Contract", projectReference: "Project", precedingInvoice: "Oorspronkelijke factuur",
		despatchAdvice: "Verzendbericht", tender: "Aanbesteding",
		billTo: "Factuur aan", shipTo: "Afleveradres", payee: "Begunstigde", seller: "Verkoper",
		vatID: "Btw-nr.", taxNumber: "Fiscaal nr.", registration: "KvK-nr.", phone: "Tel.", email: "E-mail", contact: "Contact",

		colNo: "Nr.", colDescription: "Omschrijving", colQty: "Aantal", colUnit: "Eenheid", colUnitPrice: "Prijs",
		colVAT: "Btw", colAmount: "Bedrag", itemNo: "Art.nr.", origin: "Herkomst",

		allowance: "Korting", charge: "Toeslag", allowancesCharges: "Kortingen en toeslagen",
		vatBreakdown: "Btw-specificatie", vatCategory: "Categorie", vatRate: "Tarief", taxable: "Grondslag", vatAmount: "Btw",
		lineTotal: "Totaal regels", allowanceTotal: "Kortingen", chargeTotal: "Toeslagen", totalNet: "Totaal excl. btw",
		totalVAT: "Btw", totalGross: "Totaal incl. btw", paid: "Reeds betaald", rounding: "Afronding", due: "Te betalen",

		payment: "Betaling", paymentMeans: "Betaalwijze", iban: "IBAN", bic: "BIC", accountName: "Rekeninghouder", accountID: "Rekening",
		paymentReference: "Betalingskenmerk", mandate: "Machtigingskenmerk", creditorID: "Incassant-ID",
		debitedAccount: "Te debiteren rekening", card: "Kaart", notes: "Opmerkingen", page: "Pagina", continued: "vervolg",

		categories: map[string]string{
			"S": "Belast met btw", "Z": "Nultarief", "E": "Vrijgesteld", "AE": "Btw verlegd",
			"K": "Intracommunautaire levering", "G": "Uitvoer buiten de EU", "O": "Niet onderworpen aan btw",
			"L": "Canarische Eilanden (IGIC)", "M": "Ceuta en Melilla (IPSI)", "B": "Gesplitste betaling",
		},
		means: map[string]string{
			"1": "Niet gedefinieerd", "10": "Contant", "20": "Cheque", "30": "Overboeking", "31": "Afschrijving",
			"42": "Betaling op bankrekening", "48": "Bankkaart", "49": "Automatische incasso", "54": "Creditcard",
			"55": "Debetkaart", "57": "Doorlopende opdracht", "58": "SEPA-overboeking", "59": "SEPA-incasso",
			"68": "Online betaaldienst", "97": "Verrekening",
		},
		units: map[string]string{
			"C62": "st.", "H87": "st.", "EA": "st.", "XPP": "st.", "HUR": "uur", "MIN": "min", "DAY": "dag",
			"WEE": "week", "MON": "maand", "ANN": "jaar", "LS": "forfait", "SET": "set", "XBX": "doos",
			"KGM": "kg", "GRM": "g", "TNE": "t", "MTR": "m", "KMT": "km", "CMT": "cm", "MMT": "mm",
			"MTK": "m²", "MTQ": "m³", "LTR": "l", "KWH": "kWh", "P1": "%",
		},
	},
}

// countryLanguages gives the language of a document whose seller is in a
// country, when neither the options nor the invoice set it.
var countryLanguages = map[string]string{
	"FR": "fr", "LU": "fr", "MC": "fr",
	"DE": "de", "AT": "de", "CH": "de", "LI": "de",
	"IT": "it", "SM": "it", "VA": "it",
	"ES": "es", "AD": "es",
	"PT": "pt", "BR": "pt",
	"NL": "nl", "BE": "nl", "SR": "nl",
}
