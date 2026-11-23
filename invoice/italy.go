package invoice

import (
	"reflect"
	"strconv"
	"strings"
)

// Italy holds the data of the Italian FatturaPA format (FPA12 for public
// administrations, FPR12 between private parties) that the EN 16931 fields
// of the model do not carry. It is the national block of the invoice;
// Party.Italy, Line.Italy, VATBreakdown.Italy, PaymentMeans.Italy and
// AllowanceCharge.Italy hold the FatturaPA data of these values. Package
// fatturapa reads and writes these blocks. Package ubl carries the part that
// the Italian extension of the domestic invoice binds to UBL (EXT-IT, see
// ItalianExtension and ubl.Marshal); package cii does not carry them (see
// NationalData).
//
// The FatturaPA data that EN 16931 carries goes to the other fields of the
// model, as appendix 4 of the Italian technical rules (Regole tecniche, art.
// 3 d.lgs. 148/2018) maps it, with the conventions of the CIUS-IT: the name
// of a natural person written "Nome#Cognome:given#family" (see ItalianName),
// a Codice Fiscale as the legal registration identifier of scheme 0210
// (BT-30, BT-47), the EORI code, the professional register and the REA
// registration as party identifiers "EORI:code", "ALBO:register#number#date"
// and "REA:office#number" (BT-29, BT-46), the nature (Natura) and the kind
// (TipoCessionePrestazione) of a line as item attributes NATURA and
// TipoCessionePrestazione (BT-160, BT-161). The national blocks only hold
// the rest: a document type more precise than BT-3 (TD24 is an invoice
// 380), a payment method more precise than BT-81, the withholding taxes, the
// transport data… An invoice built from EN 16931 data alone needs no
// national block: package fatturapa derives each FatturaPA value from the
// EN 16931 fields.
type Italy struct {
	// Transmission data (DatiTrasmissione, 1.1) and issuer (1.5, 1.6).
	// TransmitterCountry and TransmitterCode identify the transmitter
	// (1.1.1 IdTrasmittente) when it is not the seller, identified by its
	// Codice Fiscale or its VAT number.
	TransmitterCountry string
	TransmitterCode    string
	// SequenceNumber is the ProgressivoInvio (1.1.2) when it is not the
	// last ten letters and digits of the invoice number (BT-1).
	SequenceNumber string
	// Format is the FormatoTrasmissione (1.1.3) when it is not the one of
	// the buyer: FPA12 for a buyer addressed by an IPA code (scheme 0201),
	// FPR12 otherwise.
	Format string
	// RecipientCode is the CodiceDestinatario (1.1.4) when the buyer's
	// electronic address (BT-49) does not carry it: 0000000 (no code) or
	// XXXXXXX (a buyer outside Italy).
	RecipientCode    string
	TransmitterPhone string // 1.1.5.1 ContattiTrasmittente/Telefono
	TransmitterEmail string // 1.1.5.2 ContattiTrasmittente/Email
	// RecipientPEC is the PECDestinatario (1.1.6) when the buyer's
	// electronic address (BT-49) is not that PEC address.
	RecipientPEC  string
	IssuingSystem string // attribute SistemaEmittente of the root element
	Intermediary  *Party // 1.5 TerzoIntermediarioOSoggettoEmittente
	Issuer        string // 1.6 SoggettoEmittente: CC (the buyer) or TZ (a third party)

	// General data of the document (DatiGeneraliDocumento, 2.1.1).
	DocumentType string        // 2.1.1.1 TipoDocumento, when BT-3 does not imply it (see DocumentTypeOfTypeCode)
	Withholdings []Withholding // 2.1.1.5 DatiRitenuta
	StampDuty    *StampDuty    // 2.1.1.6 DatiBollo
	Discounts    []Discount    // 2.1.1.8 ScontoMaggiorazione, on the total of the document
	Art73        bool          // 2.1.1.12 Art73: issued under article 73 of DPR 633/72

	// Related documents (2.1.2 to 2.1.6) that the EN 16931 references do
	// not carry: those with line references, a date or an item, and those
	// after the first. A contract is the contract reference (BT-12), with
	// its CUP and CIG codes as the project (BT-11) and tender (BT-17)
	// references; an order with its identifier alone is the purchase order
	// reference (BT-13) and its CodiceCommessaConvenzione the buyer
	// reference (BT-10), its CUP and CIG codes the project and tender
	// references when there is no such contract; a receipt is the receiving
	// advice reference (BT-15); a linked invoice is a preceding invoice
	// (BG-3).
	PurchaseOrders []RelatedDocument // 2.1.2 DatiOrdineAcquisto
	Contracts      []RelatedDocument // 2.1.3 DatiContratto
	Agreements     []RelatedDocument // 2.1.4 DatiConvenzione
	Receipts       []RelatedDocument // 2.1.5 DatiRicezione
	LinkedInvoices []RelatedDocument // 2.1.6 DatiFattureCollegate

	WorkPhases []string // 2.1.7 DatiSAL/RiferimentoFase
	// TransportDocuments are the transport documents (2.1.8 DatiDDT). The
	// number of the first one is also the despatch advice reference (BT-16).
	TransportDocuments []TransportDocument
	Transport          *Transport         // 2.1.9 DatiTrasporto, except the delivery address and date (BG-15, BT-72)
	MainInvoice        *DocumentReference // 2.1.10 FatturaPrincipale
	Vehicle            *Vehicle           // 2.3 DatiVeicoli
}

// Withholding is a withholding tax (DatiRitenuta).
type Withholding struct {
	Type   string  // TipoRitenuta: RT01 to RT06
	Amount Decimal // ImportoRitenuta
	Rate   Decimal // AliquotaRitenuta
	Reason string  // CausalePagamento: A, B… (codes of the form 770)
}

// StampDuty is the virtual stamp duty (DatiBollo, BolloVirtuale SI).
type StampDuty struct {
	Amount Decimal // ImportoBollo
}

// Discount is a discount (SC) or a surcharge (MG): ScontoMaggiorazione.
type Discount struct {
	Type       string  // Tipo: SC (discount) or MG (surcharge)
	Percentage Decimal // Percentuale
	Amount     Decimal // Importo
}

// RelatedDocument is a document that the invoice refers to
// (DatiDocumentiCorrelati).
type RelatedDocument struct {
	Lines     []string // RiferimentoNumeroLinea: the invoice lines it concerns, none for the whole invoice
	ID        string   // IdDocumento
	Date      Date     // Data
	Item      string   // NumItem: the line of the referenced document
	OrderCode string   // CodiceCommessaConvenzione
	CUP       string   // CodiceCUP: code of the public investment project
	CIG       string   // CodiceCIG: code of the public tender
}

// TransportDocument is a transport document (DatiDDT).
type TransportDocument struct {
	Number string   // NumeroDDT
	Date   Date     // DataDDT
	Lines  []string // RiferimentoNumeroLinea
}

// Transport holds the transport data (DatiTrasporto). The delivery address
// (IndirizzoResa) is the deliver-to address of the invoice (BG-15), and the
// day of DataOraConsegna its actual delivery date (BT-72).
type Transport struct {
	Carrier        *Party  // DatiAnagraficiVettore
	CarrierLicence string  // DatiAnagraficiVettore/NumeroLicenzaGuida
	Means          string  // MezzoTrasporto
	Reason         string  // CausaleTrasporto
	Packages       string  // NumeroColli
	Description    string  // Descrizione
	WeightUnit     string  // UnitaMisuraPeso
	GrossWeight    Decimal // PesoLordo
	NetWeight      Decimal // PesoNetto
	PickupTime     string  // DataOraRitiro, as written (xs:dateTime)
	StartDate      Date    // DataInizioTrasporto
	DeliveryTerms  string  // TipoResa: Incoterms code
	// DeliveryTime is DataOraConsegna as written (xs:dateTime), when it is
	// not the actual delivery date (BT-72) at midnight.
	DeliveryTime string
}

// Vehicle holds the data of a new means of transport sold to another member
// state (DatiVeicoli).
type Vehicle struct {
	Date     Date   // Data: first registration
	Distance string // TotalePercorso: kilometres or hours
}

// ItalyParty holds the FatturaPA data of a party that EN 16931 does not
// carry.
type ItalyParty struct {
	Title            string   // Anagrafica/Titolo
	TaxRegime        string   // RegimeFiscale of the seller: RF02 to RF20 (RF21 from FatturaPA 1.2.4), empty for RF01 (ordinary regime)
	RegisterProvince string   // ProvinciaAlbo of the seller's professional register
	ShareCapital     Decimal  // IscrizioneREA/CapitaleSociale
	SoleShareholder  string   // IscrizioneREA/SocioUnico: SU (sole) or SM (several)
	Liquidation      string   // IscrizioneREA/StatoLiquidazione: LS (in liquidation), empty for LN (not in liquidation)
	Fax              string   // Contatti/Fax
	Establishment    *Address // StabileOrganizzazione: permanent establishment in Italy
	// TaxRepresentative is the buyer's tax representative
	// (CessionarioCommittente/RappresentanteFiscale): its VAT identifier and
	// name.
	TaxRepresentative *Party
}

// ItalyLine holds the FatturaPA data of an invoice line that EN 16931 does
// not carry.
type ItalyLine struct {
	// Codes are the article codes (CodiceArticolo) that the item
	// identifiers do not carry: the seller's identifier (BT-155) carries a
	// code CARB or AICFARMACO with its type as prefix ("CARB:27101249") or
	// a code of type "Identificativo del prodotto", the standard identifier
	// (BT-157) a code whose type is its scheme ("0160" for a GTIN).
	Codes       []ItemCode
	Discounts   []Discount  // ScontoMaggiorazione on the unit price
	Withholding bool        // Ritenuta SI: the line is subject to withholding tax
	OtherData   []OtherData // AltriDatiGestionali
}

// ItemCode is an article code (CodiceArticolo).
type ItemCode struct {
	Type  string // CodiceTipo: EAN, TARIC, AICFARMACO, CARB…
	Value string // CodiceValore
}

// OtherData is a piece of management data of a line (AltriDatiGestionali).
type OtherData struct {
	Type   string  // TipoDato
	Text   string  // RiferimentoTesto
	Number Decimal // RiferimentoNumero
	Date   Date    // RiferimentoData
}

// ItalyVAT holds the FatturaPA data of a VAT summary (DatiRiepilogo) that
// EN 16931 does not carry. The nature (Natura) and the legal reference
// (RiferimentoNormativo) of a summary without VAT are its exemption reason
// (BT-120, "N2.2 Art. 7-ter"); a summary of split payment (EsigibilitaIVA S)
// has the VAT category B, and deferred VAT (D) is the VAT point date code
// 72 (on payment, BT-8).
type ItalyVAT struct {
	AncillaryCharges Decimal // SpeseAccessorie
	Rounding         Decimal // Arrotondamento
	// Chargeability is the EsigibilitaIVA (I immediate, D deferred, S split
	// payment) when the category and the VAT point date code do not imply
	// it.
	Chargeability string
	// LegalReference is the RiferimentoNormativo of a summary with VAT,
	// which cannot have an exemption reason.
	LegalReference string
}

// ItalyPayment holds the FatturaPA data of a payment detail
// (DettaglioPagamento) that EN 16931 does not carry. Each payment detail is
// a payment means of the model, whose type code (BT-81) is read from the
// payment method (see TypeCodeOfPaymentMethod).
type ItalyPayment struct {
	// Conditions are the CondizioniPagamento of the DatiPagamento that
	// holds the payment detail (TP01 instalments, TP02 full payment, TP03
	// advance), when they are not those of the payment terms (BT-20).
	Conditions string
	// Method is the ModalitaPagamento when BT-81 does not imply it.
	Method             string
	Beneficiary        string  // Beneficiario, when it is not the payee name (BT-59)
	TermsReferenceDate Date    // DataRiferimentoTerminiPagamento
	TermsDays          string  // GiorniTerminiPagamento, when the payment terms (BT-20) do not carry it
	DueDate            Date    // DataScadenzaPagamento, when it is not the payment due date (BT-9)
	Amount             Decimal // ImportoPagamento, when it is not the amount due for payment (BT-115)
	PostOffice         string  // CodUfficioPostale
	ReceiverSurname    string  // CognomeQuietanzante
	ReceiverName       string  // NomeQuietanzante
	ReceiverCF         string  // CFQuietanzante
	ReceiverTitle      string  // TitoloQuietanzante
	Institution        string  // IstitutoFinanziario
	ABI                string  // ABI
	CAB                string  // CAB
	EarlyDiscount      Decimal // ScontoPagamentoAnticipato
	EarlyDiscountDate  Date    // DataLimitePagamentoAnticipato
	LatePenalty        Decimal // PenalitaPagamentiRitardati
	LatePenaltyDate    Date    // DataDecorrenzaPenale
	PaymentCode        string  // CodicePagamento, when it is not the remittance information (BT-83)
}

// ItalyCharge holds the FatturaPA data of a pension fund contribution
// (DatiCassaPrevidenziale) that EN 16931 does not carry. A document level
// charge whose reason (BT-104) is a fund type followed by the withholding
// indicator and the nature, as appendix 4 of the technical rules writes it
// ("TC22", "TC04 SI", "TC07 N4"), is a pension fund contribution: the charge
// holds the contribution (ImportoContributoCassa, BT-99), its base
// (ImponibileCassa, BT-100), its rate (AlCassa, BT-101) and its VAT category
// and rate (AliquotaIVA, BT-102, BT-103).
type ItalyCharge struct {
	AdministrationRef string // RiferimentoAmministrazione
}

// Document types (TipoDocumento) and the invoice type codes (BT-3) of
// appendix 5 of the technical rules.
var italianDocumentTypes = map[string]string{
	"TD01": "380", "TD02": "386", "TD03": "386", "TD04": "381", "TD05": "383", "TD06": "380",
	"TD16": "380", "TD17": "380", "TD18": "380", "TD19": "380", "TD20": "380", "TD21": "380",
	"TD22": "380", "TD23": "380", "TD24": "380", "TD25": "380", "TD26": "380", "TD27": "380",
	"TD28": "380", "TD29": "380",
}

// TypeCodeOfDocumentType returns the invoice type code (BT-3) of a FatturaPA
// document type (TD01…), as appendix 5 of the technical rules maps it, or ""
// for an unknown type.
func TypeCodeOfDocumentType(td string) string { return italianDocumentTypes[td] }

// DocumentTypeOfTypeCode returns the FatturaPA document type of an invoice
// type code (BT-3): TD04 for a credit note (381 and the other credit note
// codes), TD05 for a debit note (383), TD02 for a prepayment invoice (386),
// TD01 otherwise.
func DocumentTypeOfTypeCode(code string) string {
	switch strings.TrimSpace(code) {
	case "381", "81", "83", "261", "262", "296", "308", "396", "420", "458", "532":
		return "TD04"
	case "383":
		return "TD05"
	case "386":
		return "TD02"
	}
	return "TD01"
}

// natureCategories gives, for each nature (Natura), the VAT category code
// (BT-118, BT-151) and the VAT exemption reason code (BT-121) of appendix 5
// of the technical rules.
var natureCategories = map[string][2]string{
	"N1":   {"Z", ""},
	"N2":   {"E", "VATEX-EU-132"},
	"N2.1": {"E", "VATEX-EU-132"},
	"N2.2": {"E", "VATEX-EU-132"},
	"N3":   {"G", "VATEX-EU-G"},
	"N3.1": {"G", "VATEX-EU-G"},
	"N3.2": {"K", "VATEX-EU-IC"},
	"N3.3": {"G", "VATEX-EU-G"},
	"N3.4": {"G", "VATEX-EU-G"},
	"N3.5": {"G", "VATEX-EU-G"},
	"N3.6": {"K", "VATEX-EU-IC"},
	"N4":   {"E", "VATEX-EU-132"},
	"N5":   {"E", "VATEX-EU-132"},
	"N6":   {"AE", "VATEX-EU-AE"},
	"N6.1": {"AE", "VATEX-EU-AE"},
	"N6.2": {"AE", "VATEX-EU-AE"},
	"N6.3": {"AE", "VATEX-EU-AE"},
	"N6.4": {"AE", "VATEX-EU-AE"},
	"N6.5": {"AE", "VATEX-EU-AE"},
	"N6.6": {"AE", "VATEX-EU-AE"},
	"N6.7": {"AE", "VATEX-EU-AE"},
	"N6.8": {"AE", "VATEX-EU-AE"},
	"N6.9": {"AE", "VATEX-EU-AE"},
	"N7":   {"K", "VATEX-EU-151"},
}

// CategoryOfNature returns the VAT category code (BT-118, BT-151) and the
// VAT exemption reason code (BT-121) of a FatturaPA nature (N1…), as appendix
// 5 of the technical rules maps them. The category is "" for an unknown
// nature.
func CategoryOfNature(nature string) (category, reasonCode string) {
	c := natureCategories[nature]
	return c[0], c[1]
}

// NatureOfCategory returns the FatturaPA nature of a VAT category code
// (BT-118, BT-151) without VAT, as the table of appendix 5 of the technical
// rules for cross-border invoices gives it: N1 for Z, N2.2 for E and O, N3.1
// for G, N3.2 for K, N6.9 for AE. It returns "" for the categories that bear
// VAT (S, B, L, M).
func NatureOfCategory(category string) string {
	switch strings.TrimSpace(category) {
	case "Z":
		return "N1"
	case "E", "O":
		return "N2.2"
	case "G":
		return "N3.1"
	case "K":
		return "N3.2"
	case "AE":
		return "N6.9"
	}
	return ""
}

// paymentMethods gives, for each payment means type code (BT-81, UNTDID
// 4461), the FatturaPA payment method (ModalitaPagamento) of appendix 5 of
// the technical rules. The codes not listed are MP01 (cash).
var paymentMethods = map[string]string{
	"2": "MP19", "3": "MP19", "4": "MP19", "5": "MP19", "6": "MP19", "7": "MP19", "8": "MP12",
	"9": "MP23", "11": "MP19", "12": "MP19", "13": "MP19", "14": "MP19", "15": "MP05", "16": "MP05",
	"20": "MP02", "21": "MP03", "22": "MP03", "23": "MP03", "24": "MP13", "25": "MP02", "26": "MP02",
	"30": "MP05", "31": "MP13", "42": "MP17", "44": "MP12", "45": "MP05", "46": "MP19", "47": "MP19",
	"48": "MP08", "49": "MP19", "50": "MP18", "51": "MP05", "54": "MP08", "55": "MP08", "56": "MP05",
	"57": "MP15", "58": "MP05", "59": "MP19", "60": "MP06", "61": "MP06", "62": "MP06", "63": "MP06",
	"64": "MP06", "65": "MP06", "66": "MP06", "67": "MP06", "68": "MP05", "70": "MP12", "74": "MP13",
	"75": "MP13", "76": "MP13", "77": "MP13", "78": "MP13", "91": "MP03", "92": "MP02", "93": "MP05",
	"94": "MP05", "95": "MP05", "97": "MP22", "ZZZ": "MP04",
}

// PaymentMethodOfTypeCode returns the FatturaPA payment method
// (ModalitaPagamento) of a payment means type code (BT-81), as appendix 5 of
// the technical rules maps it: MP05 (bank transfer) for 30 and 58, MP19
// (SEPA direct debit) for 59, MP08 (payment card) for 48… and MP01 (cash)
// for the codes the table maps to cash or does not list.
func PaymentMethodOfTypeCode(code string) string {
	if m, ok := paymentMethods[strings.TrimSpace(code)]; ok {
		return m
	}
	return "MP01"
}

// typeCodesOfMethod gives, for each FatturaPA payment method, the payment
// means type code (BT-81) read from it: the code of appendix 5 that EN 16931
// names most precisely (58 for a bank transfer, 59 for a SEPA direct debit),
// and an approaching code for the methods that the table does not use.
var typeCodesOfMethod = map[string]string{
	"MP01": "10", "MP02": "20", "MP03": "23", "MP04": "ZZZ", "MP05": "58", "MP06": "60",
	"MP07": "1", "MP08": "48", "MP09": "59", "MP10": "59", "MP11": "59", "MP12": "44",
	"MP13": "31", "MP14": "1", "MP15": "57", "MP16": "49", "MP17": "42", "MP18": "50",
	"MP19": "59", "MP20": "59", "MP21": "59", "MP22": "97", "MP23": "9",
}

// TypeCodeOfPaymentMethod returns the payment means type code (BT-81) read
// from a FatturaPA payment method (MP01…): 58 (SEPA credit transfer) for
// MP05, 59 (SEPA direct debit) for MP19, 10 (cash) for MP01… It returns ""
// for an unknown method. The methods that appendix 5 does not map from
// BT-81 (MP07 bollettino bancario, MP09 to MP11 RID, MP14 quietanza
// erario, MP16, MP20, MP21) give the nearest code; PaymentMethodOfTypeCode
// of that code is then another method.
func TypeCodeOfPaymentMethod(method string) string { return typeCodesOfMethod[method] }

// ItalianName returns the name of a natural person in the form of the
// CIUS-IT, "Nome#Cognome:given#family", which package fatturapa writes as
// Nome and Cognome.
func ItalianName(given, family string) string { return "Nome#Cognome:" + given + "#" + family }

// SplitItalianName returns the given name and the family name of a name in
// the form of the CIUS-IT ("Nome#Cognome:given#family", or with "&" for
// "#" in the prefix), and false for another name.
func SplitItalianName(name string) (given, family string, ok bool) {
	rest, ok := strings.CutPrefix(name, "Nome#Cognome:")
	if !ok {
		rest, ok = strings.CutPrefix(name, "Nome&Cognome:")
	}
	if !ok {
		return "", "", false
	}
	given, family, _ = strings.Cut(rest, "#")
	return given, family, true
}

// NationalData lists the data of the national blocks (Italy, Party.Italy,
// Line.Italy…, Spain, Party.Spain…, Croatia, Line.Croatia…) as
// "EXT <value>.<Field>" terms relative to the invoice ("EXT
// Italy.Withholdings", "EXT Lines[0].Italy.Discounts", "EXT
// Spain.Withholdings", "EXT Croatia.IssueTime"): EN 16931 cannot carry it,
// and packages cii and ubl report it as lost in strict mode, but for the
// part that package ubl writes in a domestic Italian invoice (EXT-IT) and
// in a Croatian one (CIUS-HR).
func (inv *Invoice) NationalData() []string {
	var out []string
	add := func(prefix string, v any) {
		out = appendSet(out, "EXT "+prefix+".", reflect.ValueOf(v).Elem())
	}
	if inv.Italy != nil {
		add("Italy", inv.Italy)
	}
	for _, p := range []struct {
		role  string
		party *Party
	}{
		{"Seller", &inv.Seller}, {"Buyer", &inv.Buyer}, {"SellerTaxRepresentative", inv.SellerTaxRepresentative},
		{"Payee", inv.Payee}, {"ShipTo", inv.ShipTo},
	} {
		if p.party != nil && p.party.Italy != nil {
			add(p.role+".Italy", p.party.Italy)
		}
	}
	for i := range inv.Lines {
		if it := inv.Lines[i].Italy; it != nil {
			add("Lines["+strconv.Itoa(i)+"].Italy", it)
		}
	}
	for i := range inv.VATBreakdown {
		if it := inv.VATBreakdown[i].Italy; it != nil {
			add("VATBreakdown["+strconv.Itoa(i)+"].Italy", it)
		}
	}
	for i := range inv.PaymentMeans {
		if it := inv.PaymentMeans[i].Italy; it != nil {
			add("PaymentMeans["+strconv.Itoa(i)+"].Italy", it)
		}
	}
	for i := range inv.AllowancesCharges {
		if it := inv.AllowancesCharges[i].Italy; it != nil {
			add("AllowancesCharges["+strconv.Itoa(i)+"].Italy", it)
		}
	}
	return inv.nationalCroatia(inv.nationalSpain(out))
}

// appendSet appends prefix followed by the name of each field of the struct
// v that is set.
func appendSet(out []string, prefix string, v reflect.Value) []string {
	t := v.Type()
	for i := range t.NumField() {
		if !isZeroValue(v.Field(i)) {
			out = append(out, prefix+t.Field(i).Name)
		}
	}
	return out
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice:
		return v.Len() == 0
	case reflect.Pointer:
		return v.IsNil()
	}
	switch x := v.Interface().(type) {
	case Decimal:
		return !x.IsSet()
	case Date:
		return x.IsZero()
	}
	return v.IsZero()
}
