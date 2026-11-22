package it

import (
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// SDICatalog lists the rules of CheckSDI: the controls of the SdI on the
// content of a FatturaPA invoice that need none of its data, named after
// their error code, in the order of appendix 1 of the technical
// specifications (1.9.1 and 1.10). The descriptions number the elements as
// the specifications do (1.2.1.1 IdFiscaleIVA of the seller…).
//
// Some controls are only checked in part. The SdI looks the tax codes up in
// the Anagrafe tributaria (00300 to 00306): only their format and check
// character are checked here, which catches a code the SdI cannot find. The
// control 00419, whose condition is that of 00443, is reported as 00443.
//
// Not checked, since they need the data of the SdI: duplicate file name
// (00002), recipient code not valid or not active (00311, 00312), tax codes
// not consistent or VAT groups (00320 to 00327), duplicate invoice (00404)
// and invalidated declaration of intent (00477). The file name (00001), its
// size (00003) and the signature (00100 to 00107) are checked by
// fatturapa.VerifySignature, the format (00200, 00201) by the schema.
var SDICatalog = []rules.RuleInfo{
	{ID: "00300", Severity: rules.Error, Description: "If 1.1.1.1 IdPaese (transmitter) is IT, 1.1.1.2 IdCodice must be a codice fiscale: 16 characters, or 11 digits, with a valid check character. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00301", Severity: rules.Error, Description: "If 1.2.1.1.1 IdPaese (seller) is IT, 1.2.1.1.2 IdCodice must be a partita IVA: 11 digits with a valid check digit. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00302", Severity: rules.Error, Description: "1.2.1.2 CodiceFiscale (seller) must be a codice fiscale: 16 characters, or 11 digits, with a valid check character. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00303", Severity: rules.Error, Description: "If the IdPaese of the tax representative of the seller (1.3.1.1.1) or of the buyer (1.4.4.1.1) is IT, its IdCodice must be a partita IVA: 11 digits with a valid check digit. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00304", Severity: rules.Error, Description: "1.3.1.2 CodiceFiscale (tax representative of the seller) must be a codice fiscale: 16 characters, or 11 digits, with a valid check character. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00305", Severity: rules.Error, Description: "If 1.4.1.1.1 IdPaese (buyer) is IT, 1.4.1.1.2 IdCodice must be a partita IVA: 11 digits with a valid check digit. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00306", Severity: rules.Error, Description: "When the buyer has no 1.4.1.1 IdFiscaleIVA, 1.4.1.2 CodiceFiscale must be a codice fiscale: 16 characters, or 11 digits, with a valid check character. Its presence in the Anagrafe tributaria is not checked."},
	{ID: "00313", Severity: rules.Error, Description: "1.1.4 CodiceDestinatario XXXXXXX is reserved to buyers not established in Italy: 1.4.1.1 IdFiscaleIVA (buyer) must be present, with an IdPaese other than IT."},
	{ID: "00330", Severity: rules.Error, Description: "1.1.6 PECDestinatario must not be a certified e-mail address of the SdI (sdiNN@pec.fatturapa.it)."},
	{ID: "00400", Severity: rules.Error, Description: "2.2.1.14 Natura must be present when 2.2.1.12 AliquotaIVA (line) is zero."},
	{ID: "00401", Severity: rules.Error, Description: "2.2.1.14 Natura must not be present when 2.2.1.12 AliquotaIVA (line) is not zero, except in a document of type TD16."},
	{ID: "00403", Severity: rules.Error, Description: "2.1.1.3 Data must not be later than the reception of the invoice by the SdI (validate.Options.ReceivedAt, the current time by default)."},
	{ID: "00409", Severity: rules.Error, Description: "Two invoices of a lot must not have the same 2.1.1.4 Numero in the same year of 2.1.1.3 Data (the same date when 2.1.1.11 Art73 is SI), unless one of them is a credit note (TD04)."},
	{ID: "00411", Severity: rules.Error, Description: "2.1.1.5 DatiRitenuta must be present when a line has 2.2.1.13 Ritenuta SI."},
	{ID: "00413", Severity: rules.Error, Description: "2.1.1.7.7 Natura must be present when 2.1.1.7.5 AliquotaIVA (pension fund contribution) is zero."},
	{ID: "00414", Severity: rules.Error, Description: "2.1.1.7.7 Natura must not be present when 2.1.1.7.5 AliquotaIVA (pension fund contribution) is not zero."},
	{ID: "00415", Severity: rules.Error, Description: "2.1.1.5 DatiRitenuta must be present when a pension fund contribution has 2.1.1.7.6 Ritenuta SI."},
	{ID: "00417", Severity: rules.Error, Description: "The buyer must have 1.4.1.1 IdFiscaleIVA or 1.4.1.2 CodiceFiscale."},
	{ID: "00418", Severity: rules.Error, Description: "2.1.1.3 Data must not be earlier than 2.1.6.3 Data, the date of a linked invoice."},
	{ID: "00420", Severity: rules.Error, Description: "2.2.2.2 Natura must not be a reverse charge code (N6, N6.1 to N6.9) when 2.2.2.7 EsigibilitaIVA is S (split payment)."},
	{ID: "00421", Severity: rules.Error, Description: "2.2.2.6 Imposta must be 2.2.2.1 AliquotaIVA × 2.2.2.5 ImponibileImporto / 100, rounded to 2 decimals, within 1 cent."},
	{ID: "00422", Severity: rules.Error, Description: "For each VAT rate, the sum of 2.2.2.5 ImponibileImporto must be the sum of 2.2.1.11 PrezzoTotale of the lines, 2.1.1.7.4 ImportoContributoCassa of the pension fund contributions and 2.2.2.4 Arrotondamento of the summaries at that rate, within 1 euro (the tolerance of the specifications for this control, whose appendix says 1 cent)."},
	{ID: "00423", Severity: rules.Error, Description: "2.2.1.11 PrezzoTotale must be 2.2.1.9 PrezzoUnitario, increased or reduced by each 2.2.1.10 ScontoMaggiorazione in turn (its Importo, or its Percentuale of the price so far), times 2.2.1.5 Quantita (1 when absent), within 1 cent."},
	{ID: "00424", Severity: rules.Error, Description: "The VAT rates (2.2.1.12, 2.2.2.1, 2.1.1.7.5 AliquotaIVA) must be percentages: 10% is 10.00, not 0.10. A rate between 0 and 1 is rejected."},
	{ID: "00425", Severity: rules.Error, Description: "2.1.1.4 Numero must contain at least one digit."},
	{ID: "00427", Severity: rules.Error, Description: "1.1.4 CodiceDestinatario must have 6 characters when 1.1.3 FormatoTrasmissione is FPA12, and 7 when it is FPR12."},
	{ID: "00428", Severity: rules.Error, Description: "1.1.3 FormatoTrasmissione must be the versione attribute of the root element."},
	{ID: "00429", Severity: rules.Error, Description: "2.2.2.2 Natura must be present when 2.2.2.1 AliquotaIVA (summary) is zero."},
	{ID: "00430", Severity: rules.Error, Description: "2.2.2.2 Natura must not be present when 2.2.2.1 AliquotaIVA (summary) is not zero, except in a document of type TD16."},
	{ID: "00437", Severity: rules.Error, Description: "A 2.1.1.8 ScontoMaggiorazione of the document must have 2.1.1.8.2 Percentuale or 2.1.1.8.3 Importo."},
	{ID: "00438", Severity: rules.Error, Description: "A 2.2.1.10 ScontoMaggiorazione of a line must have 2.2.1.10.2 Percentuale or 2.2.1.10.3 Importo."},
	{ID: "00443", Severity: rules.Error, Description: "Each VAT rate of the lines (2.2.1.12) and of the pension fund contributions (2.1.1.7.5) must have a summary 2.2.2 DatiRiepilogo at that rate (2.2.2.1); this is also the condition of 00419."},
	{ID: "00444", Severity: rules.Error, Description: "Each Natura of the lines (2.2.1.14) and of the pension fund contributions (2.1.1.7.7) must be the Natura of a summary (2.2.2.2)."},
	{ID: "00445", Severity: rules.Error, Description: "The generic codes N2, N3 and N6 are no longer admitted as Natura since 1 January 2021: the detailed codes (N2.1, N2.2, N3.1…) must be used."},
	{ID: "00471", Severity: rules.Error, Description: "In a document of type TD01, TD02, TD03, TD06, TD16, TD17, TD18, TD19, TD20, TD24, TD25, TD28 or TD29, the seller and the buyer must not be the same subject (same IdFiscaleIVA or same CodiceFiscale)."},
	{ID: "00472", Severity: rules.Error, Description: "In a document of type TD21 or TD27, the seller and the buyer must be the same subject (same IdFiscaleIVA or same CodiceFiscale)."},
	{ID: "00473", Severity: rules.Error, Description: "1.2.1.1.1 IdPaese (seller) must not be IT in a document of type TD17, TD18, TD19 or TD28, and must be IT in a document of type TD29."},
	{ID: "00474", Severity: rules.Error, Description: "In a document of type TD21 (self-invoice for exceeding the ceiling, splafonamento), 2.2.1.12 AliquotaIVA must not be zero on any line."},
	{ID: "00475", Severity: rules.Error, Description: "In a document of type TD16, TD17, TD18, TD19, TD20, TD22, TD23, TD28 or TD29, 1.4.1.1 IdFiscaleIVA (buyer) must be present."},
	{ID: "00476", Severity: rules.Error, Description: "1.2.1.1.1 IdPaese (seller) and 1.4.1.1.1 IdPaese (buyer) must not both be other than IT."},
}

// sdiParty is the tax identity of a party of a FatturaPA invoice: its
// IdFiscaleIVA (country and code) and its CodiceFiscale.
type sdiParty struct {
	country, code, cf string
}

func partyOf(dati *xmltree.Node) sdiParty {
	id := dati.Elem("IdFiscaleIVA")
	return sdiParty{country: id.Value("IdPaese"), code: id.Value("IdCodice"), cf: dati.Value("CodiceFiscale")}
}

func (p sdiParty) hasVAT() bool { return p.code != "" }

// same reports whether p and q are the same subject: the same IdFiscaleIVA,
// the same CodiceFiscale, or the Italian partita IVA of one as the
// CodiceFiscale of the other (that of a company is its partita IVA).
func (p sdiParty) same(q sdiParty) bool {
	switch {
	case p.hasVAT() && p.country == q.country && p.code == q.code:
		return true
	case p.cf != "" && p.cf == q.cf:
		return true
	case p.country == "IT" && p.code != "" && p.code == q.cf:
		return true
	case q.country == "IT" && q.code != "" && q.code == p.cf:
		return true
	}
	return false
}

// comparable reports whether the identities of p and q tell whether they
// are the same subject: both have an IdFiscaleIVA or both a CodiceFiscale.
func (p sdiParty) comparable(q sdiParty) bool {
	return p.hasVAT() && q.hasVAT() || p.cf != "" && q.cf != ""
}

// Document types of the controls on the parties.
var (
	tdNotSelf    = typeSet("TD01 TD02 TD03 TD06 TD16 TD17 TD18 TD19 TD20 TD24 TD25 TD28 TD29") // 00471
	tdSelf       = typeSet("TD21 TD27")                                                        // 00472
	tdForeign    = typeSet("TD17 TD18 TD19 TD28")                                              // 00473
	tdBuyerVAT   = typeSet("TD16 TD17 TD18 TD19 TD20 TD22 TD23 TD28 TD29")                     // 00475
	genericNatur = typeSet("N2 N3 N6")                                                         // 00445
)

func typeSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, t := range strings.Fields(s) {
		m[t] = true
	}
	return m
}

var (
	cent    = invoice.NewDecimal(1, 2)
	one     = invoice.DecimalFromInt(1) // 1 euro, or a rate of 1
	hundred = invoice.DecimalFromInt(100)
)

// sdi holds the state of CheckSDI.
type sdi struct {
	c      *rules.Checker
	latest time.Time // the latest date of the document that the reception admits (00403)
	seller sdiParty
	buyer  sdiParty
}

func (s *sdi) fail(rule string, n *xmltree.Node, format string, args ...any) {
	s.c.Fail(rule, rules.Error, n, format, args...)
}

// CheckSDI applies to the FatturaPA document root the controls of the SdI
// on its content that need none of its data (see SDICatalog). receivedAt is
// the time the SdI receives the document, for the control of its date
// (00403); the current time when zero.
func CheckSDI(c *rules.Checker, root *xmltree.Node, receivedAt time.Time) {
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	// The date of the document is a date in Italy: the latest one is that
	// of the reception in the time zone of the Italian summer (UTC+2), which
	// is never earlier than the Italian date.
	y, m, d := receivedAt.In(time.FixedZone("CEST", 2*3600)).Date()
	s := &sdi{c: c, latest: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
	h := root.Elem("FatturaElettronicaHeader")
	s.seller = partyOf(h.Find("CedentePrestatore/DatiAnagrafici"))
	s.buyer = partyOf(h.Find("CessionarioCommittente/DatiAnagrafici"))
	s.header(root, h)
	bodies := root.Elems("FatturaElettronicaBody")
	for _, b := range bodies {
		s.body(b)
	}
	s.lot(bodies)
}

// header checks the transmission data and the parties.
func (s *sdi) header(root, h *xmltree.Node) {
	dt := h.Elem("DatiTrasmissione")
	format := dt.Elem("FormatoTrasmissione")
	if v := root.Attr("versione"); format != nil && v != "" && format.Trim() != v {
		s.fail("00428", format, "FormatoTrasmissione %s does not match the versione attribute %s of the root element", format.Trim(), v)
	}
	if cd := dt.Elem("CodiceDestinatario"); cd != nil {
		code := cd.Trim()
		switch f := format.Trim(); {
		case f == "FPA12" && len(code) == 7:
			s.fail("00427", cd, "CodiceDestinatario %s has 7 characters while FormatoTrasmissione is FPA12: an invoice to a public administration is addressed to the 6 characters of its office code (codice IPA)", code)
		case f == "FPR12" && len(code) == 6:
			s.fail("00427", cd, "CodiceDestinatario %s has 6 characters while FormatoTrasmissione is FPR12: a code of 6 characters is the office code of a public administration, whose invoices are FPA12", code)
		}
		if code == "XXXXXXX" && (!s.buyer.hasVAT() || s.buyer.country == "IT") {
			s.fail("00313", cd, "CodiceDestinatario XXXXXXX is reserved to buyers not established in Italy, but the buyer has no IdFiscaleIVA of another country")
		}
	}
	if pec := dt.Elem("PECDestinatario"); pec != nil {
		if local, domain, ok := strings.Cut(strings.ToLower(pec.Trim()), "@"); ok && domain == "pec.fatturapa.it" && strings.HasPrefix(local, "sdi") {
			s.fail("00330", pec, "PECDestinatario %s is a certified e-mail address of the SdI, not of the buyer", pec.Trim())
		}
	}

	if id := dt.Elem("IdTrasmittente"); id.Value("IdPaese") == "IT" {
		s.codiceFiscale("00300", id.Elem("IdCodice"), "IdCodice of the transmitter")
	}
	seller := h.Find("CedentePrestatore/DatiAnagrafici")
	s.partitaIVA("00301", seller.Elem("IdFiscaleIVA"), "IdCodice of the seller")
	s.codiceFiscale("00302", seller.Elem("CodiceFiscale"), "CodiceFiscale of the seller")
	rep := h.Find("RappresentanteFiscale/DatiAnagrafici")
	s.partitaIVA("00303", rep.Elem("IdFiscaleIVA"), "IdCodice of the tax representative of the seller")
	s.codiceFiscale("00304", rep.Elem("CodiceFiscale"), "CodiceFiscale of the tax representative of the seller")
	buyer := h.Find("CessionarioCommittente/DatiAnagrafici")
	s.partitaIVA("00303", h.Find("CessionarioCommittente/RappresentanteFiscale/IdFiscaleIVA"), "IdCodice of the tax representative of the buyer")
	s.partitaIVA("00305", buyer.Elem("IdFiscaleIVA"), "IdCodice of the buyer")
	if !s.buyer.hasVAT() {
		s.codiceFiscale("00306", buyer.Elem("CodiceFiscale"), "CodiceFiscale of the buyer")
	}
	if buyer != nil && !s.buyer.hasVAT() && s.buyer.cf == "" {
		s.fail("00417", buyer, "the buyer has neither IdFiscaleIVA nor CodiceFiscale")
	}
	if s.seller.hasVAT() && s.seller.country != "IT" && s.buyer.hasVAT() && s.buyer.country != "IT" {
		s.fail("00476", buyer.Find("IdFiscaleIVA/IdPaese"), "the seller (%s) and the buyer (%s) are both identified outside Italy: the SdI only takes invoices with an Italian party", s.seller.country, s.buyer.country)
	}
}

// partitaIVA checks the IdCodice of an IdFiscaleIVA whose IdPaese is IT.
func (s *sdi) partitaIVA(rule string, id *xmltree.Node, what string) {
	code := id.Elem("IdCodice")
	if code == nil || id.Value("IdPaese") != "IT" {
		return
	}
	v := code.Trim()
	if why := pivaFormat(v); why != "" {
		s.fail(rule, code, "%s %s is not a partita IVA: %s", what, v, why)
	} else if c := pivaCheckDigit(v[:10]); c != v[10] {
		s.fail(rule, code, "%s %s is not a partita IVA: its check digit should be %c", what, v, c)
	}
}

// codiceFiscale checks a codice fiscale, of a natural or a legal person.
func (s *sdi) codiceFiscale(rule string, n *xmltree.Node, what string) {
	if n == nil {
		return
	}
	v := n.Trim()
	if why := cfFormat(v); why != "" {
		s.fail(rule, n, "%s %s is not a codice fiscale: %s", what, v, why)
	} else if c := cfCheck(v); c != v[len(v)-1] {
		s.fail(rule, n, "%s %s is not a codice fiscale: its check character should be %c", what, v, c)
	}
}

// rateUse is a VAT rate of a line or a pension fund contribution, with its
// nature and the amount it adds to the taxable base of the rate.
type rateUse struct {
	rate   *xmltree.Node
	nature *xmltree.Node
	amount *xmltree.Node
}

// body checks an invoice of the document.
func (s *sdi) body(b *xmltree.Node) {
	dg := b.Elem("DatiGenerali")
	doc := dg.Elem("DatiGeneraliDocumento")
	td := doc.Value("TipoDocumento")
	if n := doc.Elem("Numero"); n != nil && !strings.ContainsAny(n.Trim(), "0123456789") {
		s.fail("00425", n, "Numero %s has no digit", n.Trim())
	}
	dateNode := doc.Elem("Data")
	date, dated := sdiDate(dateNode)
	if dated && date.After(s.latest) {
		s.fail("00403", dateNode, "the date of the invoice, %s, is later than its reception by the SdI (%s)", dateNode.Trim(), s.latest.Format(time.DateOnly))
	}
	for _, linked := range dg.Elems("DatiFattureCollegate") {
		if d, ok := sdiDate(linked.Elem("Data")); ok && dated && date.Before(d) {
			s.fail("00418", dateNode, "the date of the invoice, %s, is earlier than that of the linked invoice %s, %s", dateNode.Trim(), linked.Value("IdDocumento"), linked.Value("Data"))
		}
	}
	for _, sm := range doc.Elems("ScontoMaggiorazione") {
		s.discount("00437", sm)
	}

	withholding := doc.Elem("DatiRitenuta") != nil
	var uses []rateUse
	for _, k := range doc.Elems("DatiCassaPrevidenziale") {
		u := rateUse{rate: k.Elem("AliquotaIVA"), nature: k.Elem("Natura"), amount: k.Elem("ImportoContributoCassa")}
		s.rateNature(u.rate, u.nature, "00413", "00414", false)
		if r := k.Elem("Ritenuta"); r.Trim() == "SI" && !withholding {
			s.fail("00415", r, "the pension fund contribution is subject to withholding (Ritenuta SI) but the document has no DatiRitenuta")
		}
		uses = append(uses, u)
	}
	goods := b.Elem("DatiBeniServizi")
	for _, l := range goods.Elems("DettaglioLinee") {
		u := rateUse{rate: l.Elem("AliquotaIVA"), nature: l.Elem("Natura"), amount: l.Elem("PrezzoTotale")}
		s.rateNature(u.rate, u.nature, "00400", "00401", td == "TD16")
		if r := l.Elem("Ritenuta"); r.Trim() == "SI" && !withholding {
			s.fail("00411", r, "line %s is subject to withholding (Ritenuta SI) but the document has no DatiRitenuta", l.Value("NumeroLinea"))
		}
		for _, sm := range l.Elems("ScontoMaggiorazione") {
			s.discount("00438", sm)
		}
		s.lineTotal(l)
		if rate, ok := rules.Dec(u.rate); ok && rate.IsZero() && td == "TD21" {
			s.fail("00474", u.rate, "line %s has a VAT rate of zero in a self-invoice for exceeding the ceiling (TD21)", l.Value("NumeroLinea"))
		}
		uses = append(uses, u)
	}
	// Without lines or summaries, which the schema requires, there is
	// nothing to compare.
	if lines, sums := goods.Elems("DettaglioLinee"), goods.Elems("DatiRiepilogo"); len(lines) > 0 && len(sums) > 0 {
		s.summaries(sums, uses, td)
	} else {
		for _, r := range sums {
			s.summary(r, td)
		}
	}
	s.parties(doc.Elem("TipoDocumento"), td)
}

// rateNature checks a VAT rate and the nature beside it: the rate as a
// percentage (00424), a nature when the rate is zero (zero), none
// otherwise, except in TD16 when exempt (nonZero), and no generic nature
// (00445).
func (s *sdi) rateNature(rateNode, nature *xmltree.Node, zero, nonZero string, exempt bool) {
	if nature != nil && genericNatur[nature.Trim()] {
		s.fail("00445", nature, "Natura %s is a generic code, no longer admitted since 1 January 2021: use one of its detailed codes (%s.1…)", nature.Trim(), nature.Trim())
	}
	rate, ok := rules.Dec(rateNode)
	if !ok {
		return
	}
	switch {
	case rate.IsZero() && nature == nil:
		s.fail(zero, rateNode, "the VAT rate is zero but no Natura says why the operation is not taxed")
	case !rate.IsZero() && nature != nil && !exempt:
		s.fail(nonZero, nature, "Natura %s is given with a VAT rate of %s: a taxed operation has no nature", nature.Trim(), rateNode.Trim())
	}
	if rate.Sign() > 0 && rate.Cmp(one) < 0 {
		s.fail("00424", rateNode, "the VAT rate %s is not a percentage: a rate of %s%% is written %s", rateNode.Trim(), rate.Mul(hundred).Round(2), rate.Mul(hundred).Round(2))
	}
}

// discount checks that a ScontoMaggiorazione has a percentage or an amount.
func (s *sdi) discount(rule string, sm *xmltree.Node) {
	if sm.Elem("Percentuale") == nil && sm.Elem("Importo") == nil {
		s.fail(rule, sm, "ScontoMaggiorazione %s has neither Percentuale nor Importo", sm.Value("Tipo"))
	}
}

// maxPrices bounds the prices lineTotal tries: a ScontoMaggiorazione with
// both a percentage and an amount gives two.
const maxPrices = 16

// lineTotal checks the total price of a line (00423). The specifications
// do not say which of the percentage and the amount of a discount applies
// when both are given: the total is accepted with either.
func (s *sdi) lineTotal(l *xmltree.Node) {
	totalNode := l.Elem("PrezzoTotale")
	total, ok := rules.Dec(totalNode)
	price, okPrice := rules.Dec(l.Elem("PrezzoUnitario"))
	if !ok || !okPrice {
		return
	}
	quantity := invoice.DecimalFromInt(1)
	if q := l.Elem("Quantita"); q != nil {
		if quantity, ok = rules.Dec(q); !ok {
			return
		}
	}
	prices := []invoice.Decimal{price}
	for _, sm := range l.Elems("ScontoMaggiorazione") {
		var sign int64
		switch sm.Value("Tipo") {
		case "SC":
			sign = -1
		case "MG":
			sign = 1
		default:
			return
		}
		pct, okPct := rules.Dec(sm.Elem("Percentuale"))
		amount, okAmount := rules.Dec(sm.Elem("Importo"))
		if !okPct && !okAmount {
			continue
		}
		var next []invoice.Decimal
		for _, p := range prices {
			if okAmount {
				next = append(next, p.Add(amount.Mul(invoice.DecimalFromInt(sign))))
			}
			if okPct && (!okAmount || len(next) < maxPrices) {
				next = append(next, p.Add(p.Mul(pct).Div(hundred, 12).Mul(invoice.DecimalFromInt(sign))))
			}
		}
		prices = next
	}
	for _, p := range prices {
		if p.Mul(quantity).Sub(total).Abs().Cmp(cent) <= 0 {
			return
		}
	}
	s.fail("00423", totalNode, "PrezzoTotale %s of line %s is not PrezzoUnitario %s, with its discounts and supplements, times Quantita %s: %s",
		totalNode.Trim(), l.Value("NumeroLinea"), l.Value("PrezzoUnitario"), quantity, prices[0].Mul(quantity).Round(2))
}

// rateKey is the key of a VAT rate in the summaries: its value with two
// decimals.
func rateKey(n *xmltree.Node) (string, bool) {
	d, ok := rules.Dec(n)
	if !ok {
		return "", false
	}
	return d.Round(2).String(), true
}

// summaries checks the summaries of an invoice (DatiRiepilogo) and their
// agreement with the lines and the pension fund contributions.
func (s *sdi) summaries(sums []*xmltree.Node, uses []rateUse, td string) {
	type total struct {
		first *xmltree.Node
		base  invoice.Decimal // sum of ImponibileImporto
		want  invoice.Decimal // sum of the amounts and of Arrotondamento
		ok    bool
	}
	totals := make(map[string]*total)
	var order []string
	natures := make(map[string]bool)
	for _, r := range sums {
		s.summary(r, td)
		rateNode := r.Elem("AliquotaIVA")
		if nature := r.Elem("Natura"); nature != nil {
			natures[nature.Trim()] = true
		}
		base, okBase := rules.Dec(r.Elem("ImponibileImporto"))
		key, ok := rateKey(rateNode)
		if !ok {
			continue
		}
		t := totals[key]
		if t == nil {
			t = &total{first: r, ok: true}
			totals[key] = t
			order = append(order, key)
		}
		rounding, okRounding := rules.Dec(r.Elem("Arrotondamento"))
		t.ok = t.ok && okBase && (okRounding || r.Elem("Arrotondamento") == nil)
		t.base = t.base.Add(base)
		t.want = t.want.Add(rounding)
	}

	missingRate, missingNature := make(map[string]bool), make(map[string]bool)
	for _, u := range uses {
		key, ok := rateKey(u.rate)
		if !ok {
			continue
		}
		if t := totals[key]; t == nil {
			if !missingRate[key] {
				missingRate[key] = true
				s.fail("00443", u.rate, "no summary (DatiRiepilogo) has the VAT rate %s", u.rate.Trim())
			}
		} else {
			amount, ok := rules.Dec(u.amount)
			t.ok = t.ok && ok
			t.want = t.want.Add(amount)
		}
		if n := u.nature.Trim(); u.nature != nil && !natures[n] && !missingNature[n] {
			missingNature[n] = true
			s.fail("00444", u.nature, "no summary (DatiRiepilogo) has the Natura %s", n)
		}
	}
	for _, key := range order {
		t := totals[key]
		if t.ok && t.base.Sub(t.want).Abs().Cmp(one) > 0 {
			s.fail("00422", t.first.Elem("ImponibileImporto"), "the taxable amount at the VAT rate %s is %s, but the lines, the pension fund contributions and the roundings at that rate add up to %s", key, t.base, t.want)
		}
	}
}

// summary checks a summary on its own: its rate and its nature, and its tax.
func (s *sdi) summary(r *xmltree.Node, td string) {
	rateNode, nature := r.Elem("AliquotaIVA"), r.Elem("Natura")
	s.rateNature(rateNode, nature, "00429", "00430", td == "TD16")
	if nature != nil && strings.HasPrefix(nature.Trim(), "N6") && r.Value("EsigibilitaIVA") == "S" {
		s.fail("00420", nature, "Natura %s (reverse charge) is not compatible with EsigibilitaIVA S (split payment)", nature.Trim())
	}
	rate, okRate := rules.Dec(rateNode)
	base, okBase := rules.Dec(r.Elem("ImponibileImporto"))
	taxNode := r.Elem("Imposta")
	if tax, ok := rules.Dec(taxNode); ok && okRate && okBase {
		if want := rate.Mul(base).Div(hundred, 2); tax.Sub(want).Abs().Cmp(cent) > 0 {
			s.fail("00421", taxNode, "Imposta %s is not AliquotaIVA %s × ImponibileImporto %s / 100 = %s", taxNode.Trim(), rateNode.Trim(), r.Value("ImponibileImporto"), want)
		}
	}
}

// parties checks the seller and the buyer against the document type.
func (s *sdi) parties(n *xmltree.Node, td string) {
	switch {
	case tdNotSelf[td] && s.seller.same(s.buyer):
		s.fail("00471", n, "in a document of type %s, the seller and the buyer must not be the same subject", td)
	case tdSelf[td] && s.seller.comparable(s.buyer) && !s.seller.same(s.buyer):
		s.fail("00472", n, "in a document of type %s, the seller and the buyer must be the same subject", td)
	}
	switch {
	case tdForeign[td] && s.seller.country == "IT":
		s.fail("00473", n, "in a document of type %s, the seller is established outside Italy: its IdPaese cannot be IT", td)
	case td == "TD29" && s.seller.hasVAT() && s.seller.country != "IT":
		s.fail("00473", n, "in a document of type TD29, the seller is Italian: its IdPaese must be IT, not %s", s.seller.country)
	}
	if tdBuyerVAT[td] && !s.buyer.hasVAT() {
		s.fail("00475", n, "a document of type %s requires the IdFiscaleIVA of the buyer", td)
	}
}

// lot checks that the invoices of a lot have different numbers (00409).
func (s *sdi) lot(bodies []*xmltree.Node) {
	type key struct{ number, date string }
	var seen [2]map[key]bool // invoices, then credit notes (TD04)
	seen[0], seen[1] = make(map[key]bool), make(map[key]bool)
	for _, b := range bodies {
		doc := b.Find("DatiGenerali/DatiGeneraliDocumento")
		number, date := doc.Elem("Numero"), doc.Value("Data")
		if number == nil || len(date) < 4 {
			continue
		}
		k := key{number: number.Trim(), date: date[:4]}
		if doc.Value("Art73") == "SI" {
			k.date = date
		}
		i := 0
		if doc.Value("TipoDocumento") == "TD04" {
			i = 1
		}
		if seen[i][k] {
			s.fail("00409", number, "another invoice of the lot has the number %s in %s", k.number, k.date)
		}
		seen[i][k] = true
	}
}

// sdiDate parses a date of a FatturaPA document.
func sdiDate(n *xmltree.Node) (time.Time, bool) {
	if n == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.DateOnly, n.Trim())
	return t, err == nil
}
