package fatturapa

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ritoon/zerechnung/internal/xmlenc"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// MarshalOptions are the options of Marshal.
type MarshalOptions struct {
	// Format is the transmission format, FormatPA or FormatPrivate. Empty
	// means the format of the national block (invoice.Italy.Format), or
	// FPA12 when the buyer's electronic address (BT-49) is an IPA code
	// (scheme 0201) and FPR12 otherwise.
	Format string
	// Indent writes one element per line, indented with two spaces.
	Indent bool
	// Strict makes Marshal fail with a *LossError when the invoice holds data
	// that FatturaPA cannot carry. Without Strict such data is omitted.
	Strict bool
}

// LossError is returned by Marshal in strict mode when the invoice holds data
// that FatturaPA cannot carry.
type LossError struct {
	// Terms lists the business terms that would be lost: "BT-14",
	// "BG-14"…, "EXT <Field>" for the data of the model outside EN 16931
	// and "EXT <Role>.<Field>" for party data. An extension (see
	// invoice.Extension) is reported as "EXT " followed by its entry in
	// Invoice.Unmodeled, the data of the national blocks of other countries
	// as listed by invoice.Invoice.NationalData: "EXT Spain.Withholdings".
	Terms []string
}

// Error lists the business terms that would be lost.
func (e *LossError) Error() string {
	return "fatturapa: FatturaPA cannot carry " + strconv.Itoa(len(e.Terms)) + " business terms: " + strings.Join(e.Terms, ", ")
}

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// Marshal writes inv as a FatturaPA document holding a single invoice.
//
// The elements are written in the order of the FatturaPA schema. Each value
// comes from the national blocks of the invoice (see invoice.Italy) when
// they hold it, and is otherwise derived from the EN 16931 fields as
// appendix 4 of the Italian technical rules maps them: the document type
// from the invoice type code (BT-3), the nature of a line or a summary
// without VAT from its exemption reason (BT-120) or its VAT category, the
// payment method from the payment means type code (BT-81), the payment
// amount from the amount due (BT-115), the CodiceDestinatario and the
// PECDestinatario from the buyer's electronic address (BT-49): an IPA code
// (scheme 0201) or a recipient code (0205) is the CodiceDestinatario, a PEC
// address (0202) the PECDestinatario with the code 0000000; without them the
// code is 0000000, or XXXXXXX for a buyer outside Italy. The transmitter is
// the seller, and the ProgressivoInvio the last ten letters and digits of
// the invoice number, unless the national block gives them.
//
// The document level allowances and charges (BG-20, BG-21) are written as
// lines of kind SC (discount) and AC (ancillary charge), except the pension
// fund contributions: a charge whose reason (BT-104) is a fund type, the
// withholding indicator and the nature ("TC22", "TC04 SI N4") is a
// DatiCassaPrevidenziale. A charge with the reason code SAE also gives the
// virtual stamp duty (DatiBollo). A line with a negative quantity is
// written with the opposite quantity and unit price, as FatturaPA wants a
// positive quantity; the unit price of a line with allowances or charges
// (BG-27, BG-28) is its net amount divided by its quantity, so that the
// line stays consistent.
//
// Mandatory elements are always written, empty when the invoice has no value
// for them, so that a validator reports the missing value. Amounts are
// written with two decimals, unit prices and quantities with two to eight,
// rates with two.
//
// The data that FatturaPA cannot carry is omitted, or reported by a
// *LossError with MarshalOptions.Strict: among others the VAT accounting
// currency (BT-6), the invoicing period (BG-14), the item identifiers and
// attributes outside the conventions above, the line notes (BT-127), the
// payment card and direct debit details (BG-18, BG-19), the paid amount
// (BT-113), the parties of the EXTENDED profile, and every extension,
// whatever the syntax it was read from.
func Marshal(inv *invoice.Invoice, opts *MarshalOptions) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("fatturapa: nil invoice")
	}
	var o MarshalOptions
	if opts != nil {
		o = *opts
	}
	it := inv.Italy
	if it == nil {
		it = new(invoice.Italy)
	}
	w := writer{
		b:      make([]byte, 0, 4000+1200*len(inv.Lines)),
		indent: o.Indent,
		inv:    inv,
		it:     it,
	}
	format := o.Format
	if format == "" {
		format = it.Format
	}
	if format == "" {
		format = formatFor(inv)
	}
	w.b = append(w.b, xmlDeclaration...)
	w.b = append(w.b, "<p:FatturaElettronica"...)
	w.attr("xmlns:p", Namespace)
	w.attr("versione", format)
	if it.IssuingSystem != "" {
		w.attr("SistemaEmittente", it.IssuingSystem)
	}
	w.b = append(w.b, '>')
	w.depth++
	w.header(format)
	w.body()
	w.end("p:FatturaElettronica")
	if o.Indent {
		w.b = append(w.b, '\n')
	}
	w.unwritten()
	for _, e := range inv.Unmodeled() {
		w.lose("EXT " + e)
	}
	// The national blocks of other countries (the Facturae data of
	// invoice.Spain…).
	for _, e := range inv.NationalData() {
		if !strings.Contains(e, "Italy.") {
			w.lose(e)
		}
	}
	if o.Strict && len(w.losses) > 0 {
		return nil, &LossError{Terms: w.losses}
	}
	return w.b, nil
}

// appendElement appends n and its content as XML, for an extension. Elements
// without namespace are written with their name alone; other elements keep
// their prefix and get a namespace declaration. Attributes in other
// namespaces than the element's (xsi:…) are not kept.
func appendElement(b []byte, n *xmltree.Node) []byte {
	name, decl := qname(n), ""
	if n.Space != "" {
		if n.Prefix != "" {
			decl = "xmlns:" + n.Prefix + `="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		} else {
			decl = `xmlns="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		}
	} else if n.Parent != nil && n.Parent.Space != "" && n.Parent.Prefix == "" {
		decl = `xmlns=""`
	}
	b = append(b, '<')
	b = append(b, name...)
	if decl != "" {
		b = append(b, ' ')
		b = append(b, decl...)
	}
	for _, a := range n.Attrs {
		if a.Prefix != "" && a.Prefix != "xml" {
			continue
		}
		b = append(b, ' ')
		if a.Prefix != "" {
			b = append(b, a.Prefix...)
			b = append(b, ':')
		}
		b = append(b, a.Name...)
		b = append(b, '=', '"')
		b = xmlenc.AppendEscaped(b, a.Value, true)
		b = append(b, '"')
	}
	b = append(b, '>')
	if len(n.Children) == 0 {
		b = xmlenc.AppendEscaped(b, n.Text, false)
	} else {
		for _, c := range n.Children {
			b = appendElement(b, c)
		}
	}
	b = append(b, '<', '/')
	b = append(b, name...)
	return append(b, '>')
}

// writer appends XML to a byte slice.
type writer struct {
	b      []byte
	depth  int
	indent bool

	inv *invoice.Invoice
	it  *invoice.Italy // national block, never nil

	losses []string
}

// indentation is sliced to write a newline followed by the indentation of
// the current depth.
const indentation = "\n                                                                                "

func (w *writer) newline() {
	if !w.indent {
		return
	}
	w.b = append(w.b, indentation[:min(1+2*w.depth, len(indentation))]...)
}

// start writes the start tag of an aggregate.
func (w *writer) start(name string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
	w.depth++
}

// end writes the end tag of an aggregate.
func (w *writer) end(name string) {
	w.depth--
	w.newline()
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// attr writes an attribute in an open start tag.
func (w *writer) attr(name, value string) {
	w.b = append(w.b, ' ')
	w.b = append(w.b, name...)
	w.b = append(w.b, '=', '"')
	w.b = xmlenc.AppendEscaped(w.b, value, true)
	w.b = append(w.b, '"')
}

// leafReq writes a leaf element, even when value is empty.
func (w *writer) leafReq(name, value string) {
	w.newline()
	w.b = append(w.b, '<')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
	w.b = xmlenc.AppendEscaped(w.b, value, false)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// leaf writes a leaf element when value is not empty.
func (w *writer) leaf(name, value string) {
	if value != "" {
		w.leafReq(name, value)
	}
}

// yes writes an indicator SI when set.
func (w *writer) yes(name string, set bool) {
	if set {
		w.leafReq(name, "SI")
	}
}

// date writes a date (xs:date) when d is set.
func (w *writer) date(name string, d invoice.Date) {
	if !d.IsZero() {
		w.leafReq(name, d.String())
	}
}

// amount writes an amount with two decimals (Amount2DecimalType) when d is
// set.
func (w *writer) amount(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.leafReq(name, d.StringFixed(2))
	}
}

// amountReq writes an amount with two decimals, empty when d is not set.
func (w *writer) amountReq(name string, d invoice.Decimal) {
	w.leafReq(name, d.StringFixed(2))
}

// price writes a unit price, a total price or a quantity with two to eight
// decimals (Amount8DecimalType, QuantitaType), empty when d is not set.
func (w *writer) price(name string, d invoice.Decimal) {
	if d.IsSet() {
		d = d.Round(min(max(d.Scale(), 2), 8))
	}
	w.leafReq(name, d.String())
}

// rate writes a rate with two decimals (RateType), 0.00 when d is not set.
func (w *writer) rate(name string, d invoice.Decimal) {
	if !d.IsSet() {
		d = invoice.DecimalFromInt(0)
	}
	w.leafReq(name, d.StringFixed(2))
}

// lose records a business term that FatturaPA cannot carry.
func (w *writer) lose(term string) {
	for _, t := range w.losses {
		if t == term {
			return
		}
	}
	w.losses = append(w.losses, term)
}

// loseIf records term as lost when cond holds.
func (w *writer) loseIf(cond bool, term string) {
	if cond {
		w.lose(term)
	}
}

// header writes FatturaElettronicaHeader.
func (w *writer) header(format string) {
	inv, it := w.inv, w.it
	w.start("FatturaElettronicaHeader")
	w.transmission(format)
	w.seller()
	if p := inv.SellerTaxRepresentative; p != nil {
		w.start("RappresentanteFiscale")
		w.registry(p, true, "BG-11")
		w.end("RappresentanteFiscale")
		w.loseIf(!p.Address.IsZero(), "BG-12")
		w.otherParty(p, "SellerTaxRepresentative")
	}
	w.buyer()
	if p := it.Intermediary; p != nil {
		w.start("TerzoIntermediarioOSoggettoEmittente")
		w.registry(p, false, "EXT Italy.Intermediary")
		w.end("TerzoIntermediarioOSoggettoEmittente")
	}
	w.leaf("SoggettoEmittente", it.Issuer)
	w.end("FatturaElettronicaHeader")
}

// transmission writes DatiTrasmissione.
func (w *writer) transmission(format string) {
	inv, it := w.inv, w.it
	country, code := it.TransmitterCountry, it.TransmitterCode
	if country == "" && code == "" {
		country, code = transmitterOf(inv)
	}
	seq := it.SequenceNumber
	if seq == "" {
		seq = sequenceOf(inv.Number)
	}
	recipient, pec := it.RecipientCode, it.RecipientPEC
	switch ea := inv.Buyer.ElectronicAddress; ea.Scheme {
	case "0201", "0205":
		if recipient == "" {
			recipient = ea.ID
		}
	case "0202":
		if recipient == "" {
			recipient = "0000000"
		}
		if pec == "" {
			pec = ea.ID
		}
	default:
		w.loseIf(!ea.IsZero(), "BT-49")
	}
	if recipient == "" {
		recipient = "0000000"
		if c := countryOf(&inv.Buyer); c != "" && c != "IT" {
			recipient = "XXXXXXX"
		}
	}
	w.start("DatiTrasmissione")
	w.start("IdTrasmittente")
	w.leafReq("IdPaese", country)
	w.leafReq("IdCodice", code)
	w.end("IdTrasmittente")
	w.leafReq("ProgressivoInvio", seq)
	w.leafReq("FormatoTrasmissione", format)
	w.leafReq("CodiceDestinatario", recipient)
	if it.TransmitterPhone != "" || it.TransmitterEmail != "" {
		w.start("ContattiTrasmittente")
		w.leaf("Telefono", it.TransmitterPhone)
		w.leaf("Email", it.TransmitterEmail)
		w.end("ContattiTrasmittente")
	}
	w.leaf("PECDestinatario", pec)
	w.end("DatiTrasmissione")
}

// transmitterOf returns the transmitter of an invoice whose national block
// does not give it: the seller, identified by its Codice Fiscale or its VAT
// number.
func transmitterOf(inv *invoice.Invoice) (country, code string) {
	country, code = splitVAT(inv.Seller.VATID, inv.Seller.Address.CountryCode)
	if cf := codiceFiscale(&inv.Seller); cf != "" {
		code = cf
	}
	return country, code
}

// formatFor returns the transmission format of an invoice whose national
// block does not give it: FPA12 when the buyer's electronic address is an
// IPA code, FPR12 otherwise.
func formatFor(inv *invoice.Invoice) string {
	if inv.Buyer.ElectronicAddress.Scheme == "0201" {
		return FormatPA
	}
	return FormatPrivate
}

// sequenceOf derives a ProgressivoInvio from an invoice number: its last ten
// letters and digits, or 1.
func sequenceOf(number string) string {
	var b []byte
	for i := 0; i < len(number); i++ {
		if c := number[i]; c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			b = append(b, c)
		}
	}
	if len(b) == 0 {
		return "1"
	}
	return string(b[max(len(b)-10, 0):])
}

// splitVAT splits a VAT identifier into its country prefix and its number;
// without prefix, the country is the one of the address.
func splitVAT(vat, country string) (string, string) {
	if len(vat) > 2 && isUpper(vat[0]) && isUpper(vat[1]) {
		return vat[:2], vat[2:]
	}
	return country, vat
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }

// countryOf returns the country of a party: the one of its address, or the
// prefix of its VAT identifier.
func countryOf(p *invoice.Party) string {
	if p.Address.CountryCode != "" {
		return p.Address.CountryCode
	}
	c, _ := splitVAT(p.VATID, "")
	return c
}

// codiceFiscale returns the Codice Fiscale of a party: its legal
// registration identifier of scheme 0210, or written "CF:code" as the
// CIUS-IT allows.
func codiceFiscale(p *invoice.Party) string {
	if cf, ok := strings.CutPrefix(p.LegalRegistration.ID, "CF:"); ok {
		return cf
	}
	if p.LegalRegistration.Scheme == "0210" {
		return p.LegalRegistration.ID
	}
	return ""
}

// partyIDs returns the party identifiers of the CIUS-IT conventions: the
// EORI code, the professional register (register, number, date) and the
// REA registration (office, number). other reports whether the party has
// other identifiers.
func partyIDs(p *invoice.Party) (eori string, albo, rea []string, other bool) {
	for _, id := range p.IDs {
		switch {
		case strings.HasPrefix(id.ID, "EORI:") && eori == "":
			eori = id.ID[len("EORI:"):]
		case strings.HasPrefix(id.ID, "ALBO:") && albo == nil:
			albo = strings.SplitN(id.ID[len("ALBO:"):]+"##", "#", 3)
			albo[2] = strings.TrimRight(albo[2], "#")
		case strings.HasPrefix(id.ID, "REA:") && rea == nil:
			rea = strings.SplitN(id.ID[len("REA:"):]+"#", "#", 2)
			rea[1] = strings.TrimRight(rea[1], "#")
		default:
			other = true
		}
	}
	return eori, albo, rea, other
}

// italy returns the national block of a party, never nil.
func italy(p *invoice.Party) *invoice.ItalyParty {
	if p.Italy != nil {
		return p.Italy
	}
	return new(invoice.ItalyParty)
}

// registry writes the DatiAnagrafici of a party that has neither a
// professional register nor a tax regime: the seller's tax representative
// (vat true: its VAT identifier is mandatory) or the intermediary.
func (w *writer) registry(p *invoice.Party, vat bool, role string) {
	w.start("DatiAnagrafici")
	w.vatID(p, vat)
	w.leaf("CodiceFiscale", codiceFiscale(p))
	eori, _, _, _ := partyIDs(p)
	w.anagrafica(p, italy(p).Title, eori)
	w.end("DatiAnagrafici")
	w.loseIf(len(p.IDs) > 0 && eori == "" || len(p.IDs) > 1, role+" identifiers")
}

// vatID writes IdFiscaleIVA, when the party has a VAT identifier or req is
// set.
func (w *writer) vatID(p *invoice.Party, req bool) {
	if p.VATID == "" && !req {
		return
	}
	country, code := splitVAT(p.VATID, p.Address.CountryCode)
	w.start("IdFiscaleIVA")
	w.leafReq("IdPaese", country)
	w.leafReq("IdCodice", code)
	w.end("IdFiscaleIVA")
}

// anagrafica writes the name of a party: Nome and Cognome for a name
// written "Nome#Cognome:given#family", Denominazione otherwise.
func (w *writer) anagrafica(p *invoice.Party, title, eori string) {
	w.start("Anagrafica")
	w.name(p.Name)
	w.leaf("Titolo", title)
	w.leaf("CodEORI", eori)
	w.end("Anagrafica")
}

func (w *writer) name(name string) {
	if given, family, ok := invoice.SplitItalianName(name); ok {
		w.leafReq("Nome", given)
		w.leafReq("Cognome", family)
		return
	}
	w.leafReq("Denominazione", name)
}

// address writes an IndirizzoType: the first address line is the street,
// the second the street number when it fits (eight characters), otherwise
// the end of the street with the third line. The post code of an address
// outside Italy that is not five digits is written 00000, as the SDI
// wants.
func (w *writer) address(name string, a invoice.Address) {
	street, number := a.Line1, a.Line2
	if utf8.RuneCountInString(number) > 8 {
		street, number = join(street, number), ""
	}
	street = join(street, a.Line3)
	cap := a.PostCode
	if a.CountryCode != "IT" && !digits5(cap) {
		cap = "00000"
	}
	w.start(name)
	w.leafReq("Indirizzo", street)
	w.leaf("NumeroCivico", number)
	w.leafReq("CAP", cap)
	w.leafReq("Comune", a.City)
	if a.CountryCode == "IT" || len(a.CountrySubdivision) == 2 && isUpper(a.CountrySubdivision[0]) && isUpper(a.CountrySubdivision[1]) {
		w.leaf("Provincia", a.CountrySubdivision)
	} else {
		w.loseIf(a.CountrySubdivision != "", "BT-39")
	}
	w.leafReq("Nazione", a.CountryCode)
	w.end(name)
}

func join(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

func digits5(s string) bool {
	if len(s) != 5 {
		return false
	}
	for i := range 5 {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// seller writes CedentePrestatore.
func (w *writer) seller() {
	inv := w.inv
	p := &inv.Seller
	pit := italy(p)
	eori, albo, rea, other := partyIDs(p)
	w.start("CedentePrestatore")
	w.start("DatiAnagrafici")
	w.vatID(p, true)
	w.leaf("CodiceFiscale", codiceFiscale(p))
	w.anagrafica(p, pit.Title, eori)
	if albo != nil {
		w.leaf("AlboProfessionale", albo[0])
		w.leaf("ProvinciaAlbo", pit.RegisterProvince)
		w.leaf("NumeroIscrizioneAlbo", albo[1])
		w.leaf("DataIscrizioneAlbo", albo[2])
	} else {
		w.loseIf(pit.RegisterProvince != "", "EXT Seller.Italy.RegisterProvince")
	}
	regime := pit.TaxRegime
	if regime == "" {
		regime = "RF01"
	}
	w.leafReq("RegimeFiscale", regime)
	w.end("DatiAnagrafici")
	w.address("Sede", p.Address)
	if pit.Establishment != nil {
		w.address("StabileOrganizzazione", *pit.Establishment)
	}
	if rea != nil {
		w.start("IscrizioneREA")
		w.leafReq("Ufficio", rea[0])
		w.leafReq("NumeroREA", rea[1])
		w.amount("CapitaleSociale", pit.ShareCapital)
		w.leaf("SocioUnico", pit.SoleShareholder)
		liquidation := pit.Liquidation
		if liquidation == "" {
			liquidation = "LN"
		}
		w.leafReq("StatoLiquidazione", liquidation)
		w.end("IscrizioneREA")
	} else {
		w.loseIf(pit.ShareCapital.IsSet() || pit.SoleShareholder != "" || pit.Liquidation != "", "EXT Seller.Italy.REA")
	}
	var phone, email string
	if c := p.Contact; c != nil {
		phone, email = c.Phone, c.Email
		w.loseIf(c.PersonName != "" || c.DepartmentName != "", "BT-41")
	}
	if phone != "" || email != "" || pit.Fax != "" {
		w.start("Contatti")
		w.leaf("Telefono", phone)
		w.leaf("Fax", pit.Fax)
		w.leaf("Email", email)
		w.end("Contatti")
	}
	w.leaf("RiferimentoAmministrazione", inv.BuyerAccountingReference)
	w.end("CedentePrestatore")
	w.loseIf(other, "BT-29")
	w.loseIf(!p.LegalRegistration.IsZero() && codiceFiscale(p) == "", "BT-30")
	w.loseIf(p.TradingName != "", "BT-28")
	w.loseIf(p.TaxRegistrationID != "", "BT-32")
	w.loseIf(p.Description != "", "BT-33")
	w.loseIf(!p.ElectronicAddress.IsZero(), "BT-34")
	w.loseIf(pit.TaxRepresentative != nil, "EXT Seller.Italy.TaxRepresentative")
}

// otherParty records the data of a party that FatturaPA does not carry in
// its role.
func (w *writer) otherParty(p *invoice.Party, role string) {
	w.loseIf(p.TradingName != "" || p.Contact != nil || !p.ElectronicAddress.IsZero() || p.TaxRegistrationID != "" ||
		p.Description != "", "EXT "+role)
	if pit := p.Italy; pit != nil {
		w.loseIf(pit.TaxRegime != "" || pit.RegisterProvince != "" || pit.ShareCapital.IsSet() || pit.SoleShareholder != "" ||
			pit.Liquidation != "" || pit.Fax != "" || pit.Establishment != nil || pit.TaxRepresentative != nil, "EXT "+role+".Italy")
	}
}

// buyer writes CessionarioCommittente.
func (w *writer) buyer() {
	p := &w.inv.Buyer
	pit := italy(p)
	eori, _, _, _ := partyIDs(p)
	w.start("CessionarioCommittente")
	w.start("DatiAnagrafici")
	w.vatID(p, false)
	w.leaf("CodiceFiscale", codiceFiscale(p))
	w.anagrafica(p, pit.Title, eori)
	w.end("DatiAnagrafici")
	w.address("Sede", p.Address)
	if pit.Establishment != nil {
		w.address("StabileOrganizzazione", *pit.Establishment)
	}
	if tr := pit.TaxRepresentative; tr != nil {
		w.start("RappresentanteFiscale")
		w.vatID(tr, true)
		w.name(tr.Name)
		w.end("RappresentanteFiscale")
	}
	w.end("CessionarioCommittente")
	w.loseIf(len(p.IDs) > 0 && eori == "" || len(p.IDs) > 1, "BT-46")
	w.loseIf(!p.LegalRegistration.IsZero() && codiceFiscale(p) == "", "BT-47")
	w.loseIf(p.TradingName != "", "BT-45")
	w.loseIf(p.Contact != nil, "BG-9")
	w.loseIf(pit.TaxRegime != "" || pit.RegisterProvince != "" || pit.ShareCapital.IsSet() || pit.SoleShareholder != "" ||
		pit.Liquidation != "" || pit.Fax != "", "EXT Buyer.Italy")
}

// body writes FatturaElettronicaBody.
func (w *writer) body() {
	w.start("FatturaElettronicaBody")
	w.general()
	w.goods()
	if v := w.it.Vehicle; v != nil {
		w.start("DatiVeicoli")
		w.leafReq("Data", dateString(v.Date))
		w.leafReq("TotalePercorso", v.Distance)
		w.end("DatiVeicoli")
	}
	w.payments()
	w.attachments()
	w.end("FatturaElettronicaBody")
}

func dateString(d invoice.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.String()
}

// general writes DatiGenerali.
func (w *writer) general() {
	inv, it := w.inv, w.it
	w.start("DatiGenerali")
	w.document()

	// The tender reference (BT-17) goes with the contract, or with the
	// order when there is no contract.
	orders, contracts := it.PurchaseOrders, it.Contracts
	tender := inv.TenderReference()
	if id := inv.PurchaseOrderReference; id != "" && !hasID(orders, id) {
		order := invoice.RelatedDocument{ID: id, OrderCode: inv.BuyerReference}
		if inv.ContractReference == "" {
			order.CIG, tender = tender, ""
		}
		orders = append([]invoice.RelatedDocument{order}, orders...)
	} else {
		w.loseIf(inv.BuyerReference != "" && !hasOrderCode(orders, inv.BuyerReference), "BT-10")
	}
	w.related("DatiOrdineAcquisto", orders)
	if id := inv.ContractReference; id != "" && !hasID(contracts, id) {
		contracts = append([]invoice.RelatedDocument{{ID: id, CUP: inv.ProjectReference, CIG: tender}}, contracts...)
	} else {
		w.loseIf(inv.ProjectReference != "", "BT-11")
		w.loseIf(tender != "", "BT-17")
	}
	w.related("DatiContratto", contracts)
	w.related("DatiConvenzione", it.Agreements)
	receipts := it.Receipts
	if id := inv.ReceivingAdviceReference; id != "" && !hasID(receipts, id) {
		receipts = append([]invoice.RelatedDocument{{ID: id}}, receipts...)
	}
	w.related("DatiRicezione", receipts)
	var linked []invoice.RelatedDocument
	for _, p := range inv.PrecedingInvoices {
		if !hasID(it.LinkedInvoices, p.ID) {
			linked = append(linked, invoice.RelatedDocument{ID: p.ID, Date: p.IssueDate})
		}
		w.loseIf(p.TypeCode != "" || p.LineID != "", "EXT PrecedingInvoices")
	}
	linked = append(linked, it.LinkedInvoices...)
	w.related("DatiFattureCollegate", linked)
	for _, phase := range it.WorkPhases {
		w.start("DatiSAL")
		w.leafReq("RiferimentoFase", phase)
		w.end("DatiSAL")
	}
	ddts := it.TransportDocuments
	if n := inv.DespatchAdviceReference; n != "" && !hasNumber(ddts, n) {
		if inv.DeliveryDate.IsZero() {
			w.lose("BT-16")
		} else {
			ddts = append(ddts[:len(ddts):len(ddts)], invoice.TransportDocument{Number: n, Date: inv.DeliveryDate})
		}
	}
	for _, d := range ddts {
		w.start("DatiDDT")
		w.leafReq("NumeroDDT", d.Number)
		w.leafReq("DataDDT", dateString(d.Date))
		for _, l := range d.Lines {
			w.leafReq("RiferimentoNumeroLinea", l)
		}
		w.end("DatiDDT")
	}
	w.transport()
	if m := it.MainInvoice; m != nil {
		w.start("FatturaPrincipale")
		w.leafReq("NumeroFatturaPrincipale", m.ID)
		w.leafReq("DataFatturaPrincipale", dateString(m.IssueDate))
		w.end("FatturaPrincipale")
	}
	w.end("DatiGenerali")
}

func hasID(docs []invoice.RelatedDocument, id string) bool {
	for _, d := range docs {
		if d.ID == id {
			return true
		}
	}
	return false
}

func hasOrderCode(docs []invoice.RelatedDocument, code string) bool {
	for _, d := range docs {
		if d.OrderCode == code {
			return true
		}
	}
	return false
}

func hasNumber(docs []invoice.TransportDocument, number string) bool {
	for _, d := range docs {
		if d.Number == number {
			return true
		}
	}
	return false
}

// related writes DatiDocumentiCorrelatiType elements.
func (w *writer) related(name string, docs []invoice.RelatedDocument) {
	for _, d := range docs {
		w.start(name)
		for _, l := range d.Lines {
			w.leafReq("RiferimentoNumeroLinea", l)
		}
		w.leafReq("IdDocumento", d.ID)
		w.date("Data", d.Date)
		w.leaf("NumItem", d.Item)
		w.leaf("CodiceCommessaConvenzione", d.OrderCode)
		w.leaf("CodiceCUP", d.CUP)
		w.leaf("CodiceCIG", d.CIG)
		w.end(name)
	}
}

// fund is a pension fund contribution read from the reason of a charge.
type fund struct {
	kind        string // TipoCassa
	withholding bool
	nature      string
}

// fundOf returns the pension fund contribution of a document level charge
// whose reason is "TCnn[ SI][ Nx]", and false for another charge.
func fundOf(ac *invoice.AllowanceCharge) (fund, bool) {
	if !ac.Charge {
		return fund{}, false
	}
	f := strings.Fields(ac.Reason)
	if len(f) == 0 || len(f[0]) != 4 || !strings.HasPrefix(f[0], "TC") || !isDigit(f[0][2]) || !isDigit(f[0][3]) {
		return fund{}, false
	}
	k := fund{kind: f[0]}
	f = f[1:]
	if len(f) > 0 && f[0] == "SI" {
		k.withholding = true
		f = f[1:]
	}
	if len(f) > 0 && strings.HasPrefix(f[0], "N") {
		k.nature = f[0]
		f = f[1:]
	}
	return k, len(f) == 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// document writes DatiGeneraliDocumento.
func (w *writer) document() {
	inv, it := w.inv, w.it
	td := it.DocumentType
	if td == "" {
		td = invoice.DocumentTypeOfTypeCode(inv.TypeCode)
	}
	w.start("DatiGeneraliDocumento")
	w.leafReq("TipoDocumento", td)
	w.leafReq("Divisa", inv.Currency)
	w.leafReq("Data", dateString(inv.IssueDate))
	w.leafReq("Numero", inv.Number)
	for _, r := range it.Withholdings {
		w.start("DatiRitenuta")
		w.leafReq("TipoRitenuta", r.Type)
		w.amountReq("ImportoRitenuta", r.Amount)
		w.rate("AliquotaRitenuta", r.Rate)
		w.leafReq("CausalePagamento", r.Reason)
		w.end("DatiRitenuta")
	}
	stamp := it.StampDuty
	if stamp == nil {
		for i := range inv.AllowancesCharges {
			if ac := &inv.AllowancesCharges[i]; ac.Charge && ac.ReasonCode == "SAE" {
				stamp = &invoice.StampDuty{Amount: ac.Amount}
				break
			}
		}
	}
	if stamp != nil {
		w.start("DatiBollo")
		w.leafReq("BolloVirtuale", "SI")
		w.amount("ImportoBollo", stamp.Amount)
		w.end("DatiBollo")
	}
	for i := range inv.AllowancesCharges {
		ac := &inv.AllowancesCharges[i]
		f, ok := fundOf(ac)
		if !ok {
			w.loseIf(ac.Italy != nil, "EXT AllowancesCharges["+strconv.Itoa(i)+"].Italy")
			continue
		}
		nature := f.nature
		if nature == "" && (!ac.VATRate.IsSet() || ac.VATRate.IsZero()) {
			nature = invoice.NatureOfCategory(ac.VATCategoryCode)
		}
		w.start("DatiCassaPrevidenziale")
		w.leafReq("TipoCassa", f.kind)
		w.rate("AlCassa", ac.Percentage)
		w.amountReq("ImportoContributoCassa", ac.Amount)
		w.amount("ImponibileCassa", ac.BaseAmount)
		w.rate("AliquotaIVA", ac.VATRate)
		w.yes("Ritenuta", f.withholding)
		w.leaf("Natura", nature)
		if ac.Italy != nil {
			w.leaf("RiferimentoAmministrazione", ac.Italy.AdministrationRef)
		}
		w.end("DatiCassaPrevidenziale")
	}
	for _, d := range it.Discounts {
		w.discount(d, false)
	}
	w.amount("ImportoTotaleDocumento", inv.Totals.GrandTotal)
	w.amount("Arrotondamento", inv.Totals.RoundingAmount)
	for _, n := range inv.Notes {
		for _, s := range split(n.Content, 200) {
			w.leafReq("Causale", s)
		}
		w.loseIf(n.SubjectCode != "", "BT-21")
	}
	w.yes("Art73", it.Art73)
	w.end("DatiGeneraliDocumento")
}

// split cuts s into pieces of at most n characters.
func split(s string, n int) []string {
	var out []string
	for utf8.RuneCountInString(s) > n {
		i, k := 0, 0
		for k < n {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
			k++
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return append(out, s)
}

// discount writes a ScontoMaggiorazione; neg negates its amount.
func (w *writer) discount(d invoice.Discount, neg bool) {
	amount := d.Amount
	if neg && amount.IsSet() {
		amount = amount.Neg()
	}
	w.start("ScontoMaggiorazione")
	w.leafReq("Tipo", d.Type)
	if d.Percentage.IsSet() {
		w.rate("Percentuale", d.Percentage)
	}
	if amount.IsSet() {
		w.price("Importo", amount)
	}
	w.end("ScontoMaggiorazione")
}

// transport writes DatiTrasporto, with the deliver-to address (BG-15) and
// the actual delivery date (BT-72).
func (w *writer) transport() {
	inv := w.inv
	t := w.it.Transport
	var addr invoice.Address
	if inv.ShipTo != nil {
		addr = inv.ShipTo.Address
		w.loseIf(inv.ShipTo.Name != "", "BT-70")
		w.loseIf(len(inv.ShipTo.IDs) > 0, "BT-71")
	}
	if t == nil && addr.IsZero() && inv.DeliveryDate.IsZero() {
		return
	}
	if t == nil {
		t = new(invoice.Transport)
	}
	w.start("DatiTrasporto")
	if c := t.Carrier; c != nil {
		w.start("DatiAnagraficiVettore")
		w.vatID(c, true)
		w.leaf("CodiceFiscale", codiceFiscale(c))
		eori, _, _, _ := partyIDs(c)
		w.anagrafica(c, italy(c).Title, eori)
		w.leaf("NumeroLicenzaGuida", t.CarrierLicence)
		w.end("DatiAnagraficiVettore")
	}
	w.leaf("MezzoTrasporto", t.Means)
	w.leaf("CausaleTrasporto", t.Reason)
	w.leaf("NumeroColli", t.Packages)
	w.leaf("Descrizione", t.Description)
	w.leaf("UnitaMisuraPeso", t.WeightUnit)
	w.weight("PesoLordo", t.GrossWeight)
	w.weight("PesoNetto", t.NetWeight)
	w.leaf("DataOraRitiro", t.PickupTime)
	w.date("DataInizioTrasporto", t.StartDate)
	w.leaf("TipoResa", t.DeliveryTerms)
	if !addr.IsZero() {
		w.address("IndirizzoResa", addr)
	}
	switch {
	case t.DeliveryTime != "":
		w.leafReq("DataOraConsegna", t.DeliveryTime)
	case !inv.DeliveryDate.IsZero():
		w.leafReq("DataOraConsegna", inv.DeliveryDate.String()+"T00:00:00")
	}
	w.end("DatiTrasporto")
}

// weight writes a weight (PesoType) with one or two decimals.
func (w *writer) weight(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.leafReq(name, d.Round(min(max(d.Scale(), 1), 2)).String())
	}
}

// goods writes DatiBeniServizi: the lines, the document level allowances
// and charges as lines, and the VAT summaries.
func (w *writer) goods() {
	inv := w.inv
	w.start("DatiBeniServizi")
	next := 1
	used := map[string]bool{}
	for i := range inv.Lines {
		if n, err := strconv.Atoi(inv.Lines[i].ID); err == nil && n >= 1 && n <= 9999 {
			used[inv.Lines[i].ID] = true
			next = max(next, n+1)
		}
	}
	for i := range inv.Lines {
		l := &inv.Lines[i]
		number := l.ID
		if n, err := strconv.Atoi(number); err != nil || n < 1 || n > 9999 || strconv.Itoa(n) != number {
			number = strconv.Itoa(next)
			next++
			w.lose("BT-126")
		}
		w.line(l, number, i)
	}
	for i := range inv.AllowancesCharges {
		ac := &inv.AllowancesCharges[i]
		if _, ok := fundOf(ac); ok {
			continue
		}
		w.chargeLine(ac, strconv.Itoa(next))
		next++
	}
	for i := range inv.VATBreakdown {
		w.summary(&inv.VATBreakdown[i])
	}
	w.end("DatiBeniServizi")
}

// attribute returns the value of the item attribute name (BT-160), whose
// name may end with a colon.
func attribute(it *invoice.Item, name string) string {
	for _, a := range it.Attributes {
		if strings.TrimSuffix(strings.TrimSpace(a.Name), ":") == name {
			return a.Value
		}
	}
	return ""
}

// line writes a DettaglioLinee.
func (w *writer) line(l *invoice.Line, number string, i int) {
	li := l.Italy
	if li == nil {
		li = new(invoice.ItalyLine)
	}
	quantity, neg := l.Quantity, l.Quantity.IsSet() && l.Quantity.Sign() < 0
	if neg {
		quantity = quantity.Neg()
	}
	price, discounts := w.unitPrice(l, li)
	if neg && price.IsSet() {
		price = price.Neg()
	}
	nature := attribute(&l.Item, attrNature)
	if nature == "" && (!l.VATRate.IsSet() || l.VATRate.IsZero()) {
		nature = invoice.NatureOfCategory(l.VATCategoryCode)
	}
	w.start("DettaglioLinee")
	w.leafReq("NumeroLinea", number)
	w.leaf("TipoCessionePrestazione", attribute(&l.Item, attrKind))
	for _, c := range w.itemCodes(l, li) {
		w.start("CodiceArticolo")
		w.leafReq("CodiceTipo", c.Type)
		w.leafReq("CodiceValore", c.Value)
		w.end("CodiceArticolo")
	}
	w.leafReq("Descrizione", join(l.Item.Name, l.Item.Description))
	if quantity.IsSet() {
		w.price("Quantita", quantity)
	}
	w.leaf("UnitaMisura", l.UnitCode)
	w.date("DataInizioPeriodo", l.Period.Start)
	w.date("DataFinePeriodo", l.Period.End)
	w.price("PrezzoUnitario", price)
	for _, d := range discounts {
		w.discount(d, neg)
	}
	w.price("PrezzoTotale", l.NetAmount)
	w.rate("AliquotaIVA", l.VATRate)
	w.yes("Ritenuta", li.Withholding)
	w.leaf("Natura", nature)
	w.leaf("RiferimentoAmministrazione", l.AccountingReference)
	for _, d := range li.OtherData {
		w.start("AltriDatiGestionali")
		w.leafReq("TipoDato", d.Type)
		w.leaf("RiferimentoTesto", d.Text)
		if d.Number.IsSet() {
			w.price("RiferimentoNumero", d.Number)
		}
		w.date("RiferimentoData", d.Date)
		w.end("AltriDatiGestionali")
	}
	w.end("DettaglioLinee")

	w.loseIf(l.Note != "", "BT-127")
	w.loseIf(!l.ObjectIdentifier.IsZero(), "BT-128")
	w.loseIf(l.OrderLineReference != "", "BT-132")
	w.loseIf(l.Item.BuyerID != "", "BT-156")
	w.loseIf(l.Item.OriginCountry != "", "BT-159")
	w.loseIf(!l.Period.IsZero() && l.Period.Description != "", "EXT Lines["+strconv.Itoa(i)+"].Period")
	for _, a := range l.Item.Attributes {
		if n := strings.TrimSuffix(strings.TrimSpace(a.Name), ":"); n != attrKind && n != attrNature {
			w.lose("BG-32")
		}
	}
}

// unitPrice returns the unit price of a line and the discounts and
// surcharges on it: the gross price (BT-148) with the native discounts, or
// with the price discount (BT-147) as a discount by an amount when the two
// give the net price of a unit, or the net price (BT-146) per unit of the
// price base quantity (BT-149). The unit
// price of a line with allowances or charges is its net amount divided by
// its quantity.
func (w *writer) unitPrice(l *invoice.Line, li *invoice.ItalyLine) (invoice.Decimal, []invoice.Discount) {
	if len(li.Discounts) > 0 {
		if l.GrossPrice.IsSet() {
			return l.GrossPrice, li.Discounts
		}
		return l.NetPrice, li.Discounts
	}
	if len(l.AllowancesCharges) > 0 {
		w.lose("BG-27")
		if l.Quantity.IsSet() && !l.Quantity.IsZero() && l.NetAmount.IsSet() {
			return trim(l.NetAmount.Div(l.Quantity.Abs(), 8).Abs()), nil
		}
		return l.NetPrice, nil
	}
	// The gross price and its discount (BT-148, BT-147), when they give the
	// net price of a unit: PrezzoTotale must be their difference times the
	// quantity (SdI control 00423). A price for another base quantity, or
	// with charges beside the discount (EXTENDED), is written net.
	one := invoice.DecimalFromInt(1)
	if b := l.PriceBaseQuantity; l.GrossPrice.IsSet() && l.PriceDiscount.IsSet() && !l.PriceDiscount.IsZero() &&
		(!b.IsSet() || b.Equal(one)) && (!l.NetPrice.IsSet() || l.GrossPrice.Sub(l.PriceDiscount).Equal(l.NetPrice)) {
		return l.GrossPrice, []invoice.Discount{{Type: "SC", Amount: l.PriceDiscount}}
	}
	price := l.NetPrice
	if b := l.PriceBaseQuantity; b.IsSet() && !b.IsZero() && !b.Equal(one) && price.IsSet() {
		price = trim(price.Div(b, 8))
	}
	if !price.IsSet() {
		price = l.GrossPrice
	}
	return price, nil
}

// itemCodes returns the article codes of a line: the seller's identifier
// (BT-155) and the standard identifier (BT-157) as appendix 4 maps them,
// the classifications (BT-158) with their scheme as type, then the codes of
// the national block.
func (w *writer) itemCodes(l *invoice.Line, li *invoice.ItalyLine) []invoice.ItemCode {
	var codes []invoice.ItemCode
	if id := l.Item.SellerID; id != "" {
		c := invoice.ItemCode{Type: productID, Value: id}
		for _, t := range []string{"CARB", "AICFARMACO"} {
			if v, ok := strings.CutPrefix(id, t+":"); ok {
				c = invoice.ItemCode{Type: t, Value: v}
			}
		}
		codes = append(codes, c)
	}
	if g := l.Item.GlobalID; !g.IsZero() {
		scheme := g.Scheme
		if scheme == "" {
			scheme = "0160"
		}
		codes = append(codes, invoice.ItemCode{Type: scheme, Value: g.ID})
	}
	for _, c := range l.Item.Classifications {
		codes = append(codes, invoice.ItemCode{Type: c.ListID, Value: c.Code})
	}
	return append(codes, li.Codes...)
}

// chargeLine writes a document level allowance (BG-20) or charge (BG-21) as
// a line of kind SC or AC.
func (w *writer) chargeLine(ac *invoice.AllowanceCharge, number string) {
	kind, amount := "AC", ac.Amount
	if !ac.Charge {
		kind = "SC"
		if amount.IsSet() {
			amount = amount.Neg()
		}
	}
	desc := ac.Reason
	if desc == "" {
		desc = ac.ReasonCode
	}
	nature := ""
	if !ac.VATRate.IsSet() || ac.VATRate.IsZero() {
		nature = invoice.NatureOfCategory(ac.VATCategoryCode)
	}
	w.start("DettaglioLinee")
	w.leafReq("NumeroLinea", number)
	w.leafReq("TipoCessionePrestazione", kind)
	w.leafReq("Descrizione", desc)
	w.leafReq("Quantita", "1.00")
	w.leafReq("UnitaMisura", "C62")
	w.price("PrezzoUnitario", amount)
	w.price("PrezzoTotale", amount)
	w.rate("AliquotaIVA", ac.VATRate)
	w.leaf("Natura", nature)
	w.end("DettaglioLinee")
	w.loseIf(ac.BaseAmount.IsSet() || ac.Percentage.IsSet(), "BT-93")
}

// splitNature splits an exemption reason (BT-120) into the nature that starts
// it and the legal reference that follows.
func splitNature(reason string) (nature, ref string) {
	first, rest, _ := strings.Cut(reason, " ")
	if c, _ := invoice.CategoryOfNature(first); c != "" {
		return first, strings.TrimSpace(rest)
	}
	return "", reason
}

// summary writes a DatiRiepilogo.
func (w *writer) summary(v *invoice.VATBreakdown) {
	vi := v.Italy
	if vi == nil {
		vi = new(invoice.ItalyVAT)
	}
	nature, ref := splitNature(v.ExemptionReason)
	if nature == "" && (!v.Rate.IsSet() || v.Rate.IsZero()) {
		nature = invoice.NatureOfCategory(v.CategoryCode)
	}
	if ref == "" {
		ref = vi.LegalReference
	}
	chargeability := vi.Chargeability
	switch {
	case chargeability != "":
	case v.CategoryCode == "B":
		chargeability = "S"
	case w.inv.TaxPointDateCode == "72":
		chargeability = "D"
	case v.Rate.IsSet() && !v.Rate.IsZero():
		chargeability = "I"
	}
	w.start("DatiRiepilogo")
	w.rate("AliquotaIVA", v.Rate)
	w.leaf("Natura", nature)
	w.amount("SpeseAccessorie", vi.AncillaryCharges)
	if vi.Rounding.IsSet() {
		w.price("Arrotondamento", vi.Rounding)
	}
	w.amountReq("ImponibileImporto", v.TaxableAmount)
	w.amountReq("Imposta", v.TaxAmount)
	w.leaf("EsigibilitaIVA", chargeability)
	w.leaf("RiferimentoNormativo", ref)
	w.end("DatiRiepilogo")
}

// payments writes the DatiPagamento: one per run of payment means with the
// same payment conditions, each payment means a DettaglioPagamento.
func (w *writer) payments() {
	inv := w.inv
	var terms invoice.PaymentTerms
	if len(inv.PaymentTerms) > 0 {
		terms = inv.PaymentTerms[0]
	}
	desc := strings.TrimSpace(strings.TrimSuffix(terms.Description, splitPaymentTerms))
	conditions, days, _ := strings.Cut(desc, " ")
	switch conditions {
	case "TP01", "TP02", "TP03":
		if days != "" && !allDigits(days) {
			w.lose("BT-20")
			days = ""
		}
	default:
		w.loseIf(desc != "", "BT-20")
		conditions, days = "TP02", ""
	}
	if len(inv.PaymentMeans) == 0 {
		w.loseIf(!terms.DueDate.IsZero(), "BT-9")
	}
	open, explicit := "", false
	for i := range inv.PaymentMeans {
		pm := &inv.PaymentMeans[i]
		pit := pm.Italy
		if pit == nil {
			pit = new(invoice.ItalyPayment)
		}
		c := pit.Conditions
		if c == "" {
			c = conditions
		}
		if i == 0 || c != open || pit.Conditions != "" && !explicit {
			if i > 0 {
				w.end("DatiPagamento")
			}
			w.start("DatiPagamento")
			w.leafReq("CondizioniPagamento", c)
			open, explicit = c, pit.Conditions != ""
		}
		w.detail(pm, pit, i == 0, terms, days)
	}
	if len(inv.PaymentMeans) > 0 {
		w.end("DatiPagamento")
	}
	w.loseIf(len(inv.PaymentTerms) > 1, "EXT PaymentTerms")
	w.loseIf(terms.DirectDebitMandateID != "", "BT-89")
	w.loseIf(inv.CreditorReferenceID != "", "BT-90")
	if p := inv.Payee; p != nil {
		w.loseIf(len(p.IDs) > 0, "BT-60")
		w.loseIf(!p.LegalRegistration.IsZero(), "BT-61")
	}
}

// splitPaymentTerms is the mention of split payment that BR-IT-DC-510 of the
// CIUS-IT requires in the payment terms (BT-20) of an invoice in split
// payment (VAT category B).
const splitPaymentTerms = "The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)"

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return s != ""
}

// detail writes a DettaglioPagamento.
func (w *writer) detail(pm *invoice.PaymentMeans, pit *invoice.ItalyPayment, first bool, terms invoice.PaymentTerms, days string) {
	inv := w.inv
	beneficiary := pit.Beneficiary
	if beneficiary == "" && inv.Payee != nil {
		beneficiary = inv.Payee.Name
	}
	method := pit.Method
	if method == "" {
		method = invoice.PaymentMethodOfTypeCode(pm.TypeCode)
	}
	if pit.TermsDays != "" || !first {
		days = pit.TermsDays
	}
	due := pit.DueDate
	if due.IsZero() {
		due = terms.DueDate
	}
	amount := pit.Amount
	if !amount.IsSet() {
		amount = inv.Totals.DuePayable
	}
	code := pit.PaymentCode
	if code == "" {
		code = inv.RemittanceInformation
	}
	w.start("DettaglioPagamento")
	w.leaf("Beneficiario", beneficiary)
	w.leafReq("ModalitaPagamento", method)
	w.date("DataRiferimentoTerminiPagamento", pit.TermsReferenceDate)
	w.leaf("GiorniTerminiPagamento", days)
	w.date("DataScadenzaPagamento", due)
	w.amountReq("ImportoPagamento", amount)
	w.leaf("CodUfficioPostale", pit.PostOffice)
	w.leaf("CognomeQuietanzante", pit.ReceiverSurname)
	w.leaf("NomeQuietanzante", pit.ReceiverName)
	w.leaf("CFQuietanzante", pit.ReceiverCF)
	w.leaf("TitoloQuietanzante", pit.ReceiverTitle)
	w.leaf("IstitutoFinanziario", pit.Institution)
	w.leaf("IBAN", pm.PayeeIBAN)
	w.leaf("ABI", pit.ABI)
	w.leaf("CAB", pit.CAB)
	w.leaf("BIC", pm.PayeeBIC)
	w.amount("ScontoPagamentoAnticipato", pit.EarlyDiscount)
	w.date("DataLimitePagamentoAnticipato", pit.EarlyDiscountDate)
	w.amount("PenalitaPagamentiRitardati", pit.LatePenalty)
	w.date("DataDecorrenzaPenale", pit.LatePenaltyDate)
	w.leaf("CodicePagamento", code)
	w.end("DettaglioPagamento")
	w.loseIf(pm.Information != "", "BT-82")
	w.loseIf(pm.PayeeProprietaryID != "", "BT-84")
	w.loseIf(pm.PayeeAccountName != "", "BT-85")
	w.loseIf(pm.CardID != "" || pm.CardholderName != "", "BG-18")
	w.loseIf(pm.PayerIBAN != "", "BT-91")
	w.loseIf(pm.ID != "" || pm.PayerAccountName != "" || pm.PayerBIC != "", "EXT PaymentMeans")
}

// attachments writes the Allegati: the additional supporting documents
// (BG-24) with an attached document.
func (w *writer) attachments() {
	for i := range w.inv.AdditionalDocuments {
		d := &w.inv.AdditionalDocuments[i]
		switch d.TypeCode {
		case "50":
			continue // the tender reference (BT-17), with the contract
		case "130":
			w.lose("BT-18")
			continue
		}
		if len(d.Attachment) == 0 {
			w.lose("BG-24")
			continue
		}
		name := d.AttachmentFilename
		if name == "" {
			name = d.ID
		}
		format := formatOf(d.AttachmentMimeCode)
		w.start("Allegati")
		w.leafReq("NomeAttachment", name)
		w.leaf("FormatoAttachment", format)
		w.leaf("DescrizioneAttachment", d.Description)
		w.newline()
		w.b = append(w.b, "<Attachment>"...)
		w.b = base64.StdEncoding.AppendEncode(w.b, d.Attachment)
		w.b = append(w.b, "</Attachment>"...)
		w.end("Allegati")
		w.loseIf(d.ID != "" && d.AttachmentFilename != "" && d.ID != d.AttachmentFilename, "BT-122")
		w.loseIf(format == "" && d.AttachmentMimeCode != "", "BT-125-1")
		w.loseIf(d.URI != "", "BT-124")
	}
}

// unwritten records the EN 16931 data of the invoice that FatturaPA does
// not carry at all.
func (w *writer) unwritten() {
	inv := w.inv
	w.loseIf(inv.TaxCurrency != "" && inv.TaxCurrency != inv.Currency, "BT-6")
	w.loseIf(!inv.TaxPointDate.IsZero(), "BT-7")
	switch inv.TaxPointDateCode {
	case "", "5", "29", "72":
	default:
		w.lose("BT-8")
	}
	w.loseIf(inv.SalesOrderReference != "", "BT-14")
	w.loseIf(inv.BusinessProcess != "", "BT-23")
	w.loseIf(!inv.InvoicingPeriod.IsZero(), "BG-14")
	w.loseIf(inv.Totals.PaidAmount.IsSet() && !inv.Totals.PaidAmount.IsZero(), "BT-113")
	w.loseIf(inv.Totals.TaxTotalInTaxCurrency.IsSet() && inv.TaxCurrency != "" && inv.TaxCurrency != inv.Currency, "BT-111")
	for _, p := range []struct {
		role  string
		party *invoice.Party
	}{
		{"SalesAgent", inv.SalesAgent}, {"BuyerAgent", inv.BuyerAgent}, {"Invoicer", inv.Invoicer},
		{"Invoicee", inv.Invoicee}, {"Payer", inv.Payer}, {"ProductEndUser", inv.ProductEndUser},
		{"UltimateShipTo", inv.UltimateShipTo}, {"ShipFrom", inv.ShipFrom},
	} {
		w.loseIf(!p.party.IsZero(), "EXT "+p.role)
	}
	if p := inv.Payee; p != nil {
		w.otherParty(p, "Payee")
	}
	w.loseIf(inv.DeliveryTerms != nil, "EXT DeliveryTerms")
	w.loseIf(inv.DeliveryNote != nil, "EXT DeliveryNote")
	w.loseIf(len(inv.UltimateCustomerOrders) > 0, "EXT UltimateCustomerOrders")
	w.loseIf(len(inv.LogisticsCharges) > 0, "EXT LogisticsCharges")
	w.loseIf(len(inv.ThirdPartyCharges) > 0, "BG-34")
	w.loseIf(inv.CurrencyExchange != nil, "EXT CurrencyExchange")
	w.loseIf(inv.Language != "", "EXT Language")
	w.loseIf(inv.TestIndicator || inv.CopyIndicator, "EXT TestIndicator")
}
