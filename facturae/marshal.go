package facturae

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmlenc"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// SchemaVersion is the version of the Facturae documents that Marshal
// writes.
const SchemaVersion = "3.2.2"

// MarshalOptions are the options of Marshal.
type MarshalOptions struct {
	// Indent writes one element per line, indented with two spaces.
	Indent bool
	// Strict makes Marshal fail with a *LossError when the invoice holds data
	// that Facturae cannot carry. Without Strict such data is omitted.
	Strict bool
}

// LossError is returned by Marshal in strict mode when the invoice holds data
// that Facturae cannot carry.
type LossError struct {
	// Terms lists the business terms that would be lost: "BT-10",
	// "BG-13"…, "EXT <Field>" for the data of the model outside EN 16931
	// and "EXT <Role>.<Field>" for party data. An extension (see
	// invoice.Extension) is reported as "EXT " followed by its entry in
	// Invoice.Unmodeled, the data of the national blocks of another country
	// as NationalData lists it.
	Terms []string
}

// Error lists the business terms that would be lost.
func (e *LossError) Error() string {
	return "facturae: Facturae cannot carry " + strconv.Itoa(len(e.Terms)) + " business terms: " + strings.Join(e.Terms, ", ")
}

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// Marshal writes inv as a Facturae 3.2.2 document holding a single invoice
// (modality I), without signature.
//
// The elements are written in the order of the Facturae schema. Each value
// comes from the national blocks of the invoice (see invoice.Spain) when
// they hold it, and is otherwise derived from the EN 16931 fields, as the
// documentation of the package describes it: the document type and the
// class from the invoice type code (BT-3), the tax type of a tax from its
// VAT category, the special taxable event of a line from its VAT category
// and the exemption reason of the category, the unit of measure from the
// unit code (BT-130), the payment means from the payment means type code
// (BT-81), the tax identification of a party from its VAT identifier, the
// totals of Facturae from the EN 16931 totals and the national data. A
// credit note (BT-3 381 and the other credit note codes) is written as a
// corrective invoice (class OR) with the opposite amounts and quantities,
// as Facturae has no credit note: it is read back as a corrected invoice
// (384) with negative amounts.
//
// Mandatory elements are always written, empty when the invoice has no value
// for them, so that a validator reports the missing value. Amounts, prices
// and rates are written with two to eight decimals, the amounts of type
// DoubleTwoDecimalType (installments, equivalents in euros) with two.
//
// The data that Facturae cannot carry is omitted, or reported by a
// *LossError with MarshalOptions.Strict: among others the buyer reference
// (BT-10), the project reference (BT-11), the delivery information (BG-13),
// the payee (BG-10) and the tax representative (BG-11), the electronic
// addresses (BT-34, BT-49), the item identifiers and attributes other than
// the seller's identifier, the payment terms (BT-20), the data of the
// EXTENDED profile, the national blocks of other countries, and every
// extension, whatever the syntax it was read from.
func Marshal(inv *invoice.Invoice, opts *MarshalOptions) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("facturae: nil invoice")
	}
	var o MarshalOptions
	if opts != nil {
		o = *opts
	}
	if invoice.IsCreditNoteType(inv.TypeCode) {
		inv = negated(inv)
	}
	sp := inv.Spain
	if sp == nil {
		sp = new(invoice.Spain)
	}
	w := writer{
		b:      make([]byte, 0, 4000+1500*len(inv.Lines)),
		indent: o.Indent,
		inv:    inv,
		sp:     sp,
		totals: facturaeTotals(inv),
	}
	w.b = append(w.b, xmlDeclaration...)
	w.b = append(w.b, "<fe:Facturae"...)
	w.attr("xmlns:fe", Namespace322)
	w.b = append(w.b, '>')
	w.depth++
	w.fileHeader()
	w.start("Parties")
	w.business("SellerParty", &inv.Seller, &inv.Buyer, "Seller")
	w.business("BuyerParty", &inv.Buyer, &inv.Seller, "Buyer")
	w.end("Parties")
	w.start("Invoices")
	w.invoice()
	w.end("Invoices")
	w.end("fe:Facturae")
	if o.Indent {
		w.b = append(w.b, '\n')
	}
	w.unwritten()
	for _, e := range inv.Unmodeled() {
		w.lose("EXT " + e)
	}
	if o.Strict && len(w.losses) > 0 {
		return nil, &LossError{Terms: w.losses}
	}
	return w.b, nil
}

// negated returns a copy of the credit note inv with the opposite amounts
// and quantities of its EN 16931 fields.
func negated(inv *invoice.Invoice) *invoice.Invoice {
	c := *inv
	neg := func(d invoice.Decimal) invoice.Decimal {
		if d.IsSet() {
			return d.Neg()
		}
		return d
	}
	charges := func(acs []invoice.AllowanceCharge) []invoice.AllowanceCharge {
		out := append([]invoice.AllowanceCharge(nil), acs...)
		for i := range out {
			out[i].Amount, out[i].BaseAmount = neg(out[i].Amount), neg(out[i].BaseAmount)
		}
		return out
	}
	c.AllowancesCharges = charges(inv.AllowancesCharges)
	c.Lines = append([]invoice.Line(nil), inv.Lines...)
	for i := range c.Lines {
		l := &c.Lines[i]
		l.Quantity, l.NetAmount = neg(l.Quantity), neg(l.NetAmount)
		l.AllowancesCharges = charges(l.AllowancesCharges)
	}
	c.VATBreakdown = append([]invoice.VATBreakdown(nil), inv.VATBreakdown...)
	for i := range c.VATBreakdown {
		v := &c.VATBreakdown[i]
		v.TaxableAmount, v.TaxAmount = neg(v.TaxableAmount), neg(v.TaxAmount)
	}
	t := &c.Totals
	for _, d := range []*invoice.Decimal{
		&t.LineTotal, &t.AllowanceTotal, &t.ChargeTotal, &t.TaxBasisTotal, &t.TaxTotal,
		&t.TaxTotalInTaxCurrency, &t.GrandTotal, &t.PaidAmount, &t.RoundingAmount, &t.DuePayable,
	} {
		*d = neg(*d)
	}
	return &c
}

// totals are the totals of Facturae (InvoiceTotals).
type totals struct {
	taxOutputs  invoice.Decimal // TotalTaxOutputs
	withheld    invoice.Decimal // TotalTaxesWithheld
	invoice     invoice.Decimal // InvoiceTotal
	outstanding invoice.Decimal // TotalOutstandingAmount
	executable  invoice.Decimal // TotalExecutableAmount
}

// facturaeTotals returns the totals of Facturae of inv, as the Facturae
// schemas define them: the tax outputs are the VAT breakdown, its
// equivalence surcharges and the other taxes of the national block; the
// invoice total is the total without VAT (BT-109) plus the tax outputs minus
// the taxes withheld; the outstanding amount is the invoice total minus the
// subsidies and the paid amount (BT-113); the executable amount is the
// outstanding amount minus the amounts withheld and the payment in kind,
// plus the reimbursable and the financial expenses. The totals that the
// national block gives replace the derived ones.
func facturaeTotals(inv *invoice.Invoice) totals {
	sp := inv.Spain
	if sp == nil {
		sp = new(invoice.Spain)
	}
	zero := invoice.NewDecimal(0, 2)
	var t totals
	outputs := []invoice.Decimal{zero}
	for i := range inv.VATBreakdown {
		v := &inv.VATBreakdown[i]
		outputs = append(outputs, v.TaxAmount)
		if v.Spain != nil {
			outputs = append(outputs, v.Spain.SurchargeAmount)
		}
	}
	for i := range sp.Taxes {
		outputs = append(outputs, sp.Taxes[i].Amount)
		if o := sp.Taxes[i].Output; o != nil {
			outputs = append(outputs, o.SurchargeAmount)
		}
	}
	t.taxOutputs = invoice.Sum(outputs...)
	withheld := []invoice.Decimal{zero}
	for i := range sp.Withholdings {
		withheld = append(withheld, sp.Withholdings[i].Amount)
	}
	t.withheld = invoice.Sum(withheld...)
	t.invoice = sp.InvoiceTotal
	if !t.invoice.IsSet() {
		t.invoice = inv.Totals.TaxBasisTotal.Add(t.taxOutputs).Sub(t.withheld)
	}
	t.outstanding = sp.OutstandingAmount
	if !t.outstanding.IsSet() {
		t.outstanding = t.invoice
		for i := range sp.Subsidies {
			t.outstanding = t.outstanding.Sub(sp.Subsidies[i].Amount)
		}
		if inv.Totals.PaidAmount.IsSet() {
			t.outstanding = t.outstanding.Sub(inv.Totals.PaidAmount)
		}
	}
	t.executable = sp.ExecutableAmount
	if !t.executable.IsSet() {
		t.executable = t.outstanding
		if a := sp.AmountWithheld; a != nil {
			t.executable = t.executable.Sub(a.Amount)
		}
		if p := sp.PaymentInKind; p != nil {
			t.executable = t.executable.Sub(p.Amount)
		}
		if r := reimbursableTotal(sp); r.IsSet() {
			t.executable = t.executable.Add(r)
		}
		if sp.FinancialExpenses.IsSet() {
			t.executable = t.executable.Add(sp.FinancialExpenses)
		}
	}
	return t
}

// reimbursableTotal returns the TotalReimbursableExpenses of the national
// block: ReimbursableTotal, or the sum of the reimbursable expenses (unset
// when there are none).
func reimbursableTotal(sp *invoice.Spain) invoice.Decimal {
	if sp.ReimbursableTotal.IsSet() {
		return sp.ReimbursableTotal
	}
	var d []invoice.Decimal
	for i := range sp.ReimbursableExpenses {
		d = append(d, sp.ReimbursableExpenses[i].Amount)
	}
	if len(d) == 0 {
		return invoice.Decimal{}
	}
	return invoice.Sum(d...)
}

// issuerTypeOf returns the InvoiceIssuerType derived from inv: TE (a third
// party) when the national block has a third party, RE (the buyer) for a
// self-billed invoice (389, 471), EM (the seller) otherwise.
func issuerTypeOf(inv *invoice.Invoice) string {
	switch {
	case inv.Spain != nil && inv.Spain.ThirdParty != nil:
		return "TE"
	case inv.TypeCode == "389" || inv.TypeCode == "471":
		return "RE"
	}
	return "EM"
}

// docTypeOf returns the InvoiceDocumentType of an invoice type code (BT-3):
// AF (self-billing) for 389 and 471, FC (complete invoice) otherwise.
func docTypeOf(code string) string {
	if code == "389" || code == "471" {
		return "AF"
	}
	return "FC"
}

// splitNumber splits an invoice number (BT-1, BT-25) into the series, when
// the number begins with it and goes beyond it, and the rest.
func splitNumber(number, series string) (string, string) {
	if series != "" && len(number) > len(series) && strings.HasPrefix(number, series) {
		return series, number[len(series):]
	}
	return "", number
}

// batchIDOf returns the BatchIdentifier derived from inv: the tax
// identification number of the seller, the invoice number and the series.
func batchIDOf(inv *invoice.Invoice) string {
	series := ""
	if inv.Spain != nil {
		series = inv.Spain.SeriesCode
	}
	series, number := splitNumber(inv.Number, series)
	return taxIDOf(&inv.Seller, &inv.Buyer) + number + series
}

// euMembers are the member states of the European Union, by ISO 3166-1
// alpha-2 code.
var euMembers = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true, "DK": true, "EE": true, "ES": true,
	"FI": true, "FR": true, "GR": true, "HR": true, "HU": true, "IE": true, "IT": true, "LT": true, "LU": true,
	"LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true, "SI": true, "SK": true,
}

// vatPrefix returns the country prefix of a VAT identifier of a member state
// (EL for Greece, XI for Northern Ireland), or "" when s does not begin with
// one.
func vatPrefix(s string) string {
	if len(s) < 3 {
		return ""
	}
	switch p := s[:2]; {
	case p == "EL" || p == "XI" || euMembers[p] && p != "GR":
		return p
	}
	return ""
}

// partyCountry returns the country of p: the country of its address, or
// the country of the prefix of its VAT identifier.
func partyCountry(p *invoice.Party) string {
	if c := p.Address.CountryCode; c != "" {
		return c
	}
	switch c := vatPrefix(p.VATID); c {
	case "EL":
		return "GR"
	case "XI":
		return "GB"
	default:
		return c
	}
}

// intraCommunity reports whether p and its counterpart other are in two
// different member states.
func intraCommunity(p, other *invoice.Party) bool {
	if other == nil {
		return false
	}
	a, b := partyCountry(p), partyCountry(other)
	return euMembers[a] && euMembers[b] && a != b
}

// derivedTaxID returns the TaxIdentificationNumber derived from the party
// p, whose counterpart is other (nil for a third party): its VAT identifier
// (BT-31, BT-48), without the prefix ES unless the invoice is
// intra-community, as the Facturae schemas want it; without VAT identifier,
// its legal registration identifier (BT-30, BT-47), or the seller's tax
// registration identifier (BT-32).
func derivedTaxID(p, other *invoice.Party) string {
	if v := p.VATID; v != "" {
		if rest, ok := strings.CutPrefix(v, "ES"); ok && rest != "" && !intraCommunity(p, other) {
			return rest
		}
		return v
	}
	if id := p.LegalRegistration.ID; id != "" {
		return id
	}
	return p.TaxRegistrationID
}

// taxIDOf returns the TaxIdentificationNumber of p: the one of its national
// block, or the derived one.
func taxIDOf(p, other *invoice.Party) string {
	if p.Spain != nil && p.Spain.TaxID != "" {
		return p.Spain.TaxID
	}
	return derivedTaxID(p, other)
}

// derivedResidence returns the ResidenceTypeCode derived from the country
// of p: R for Spain (and a party without country), U for another member
// state, E otherwise.
func derivedResidence(p *invoice.Party) string {
	switch c := partyCountry(p); {
	case c == "ES" || c == "":
		return "R"
	case euMembers[c]:
		return "U"
	}
	return "E"
}

// residenceOf returns the ResidenceTypeCode of p.
func residenceOf(p *invoice.Party) string {
	if p.Spain != nil && p.Spain.ResidenceType != "" {
		return p.Spain.ResidenceType
	}
	return derivedResidence(p)
}

// derivedPerson returns the PersonTypeCode derived from p: F when its
// national block gives a given name or a surname, J otherwise.
func derivedPerson(p *invoice.Party) string {
	if s := p.Spain; s != nil && (s.GivenName != "" || s.FirstSurname != "" || s.SecondSurname != "") {
		return "F"
	}
	return "J"
}

// personOf returns the PersonTypeCode of p.
func personOf(p *invoice.Party) string {
	if p.Spain != nil && p.Spain.PersonType != "" {
		return p.Spain.PersonType
	}
	return derivedPerson(p)
}

// appendElement appends n and its content as XML, for an extension. Elements
// without namespace are written with their name alone; other elements keep
// their prefix and get a namespace declaration. Attributes in other
// namespaces than the element's (xsi:…) are not kept.
func appendElement(b []byte, n *xmltree.Node) []byte {
	name, decl := qname(n), ""
	if n.Space != "" && !versionSpace(n.Space) {
		if n.Prefix != "" {
			decl = "xmlns:" + n.Prefix + `="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		} else {
			decl = `xmlns="` + string(xmlenc.AppendEscaped(nil, n.Space, true)) + `"`
		}
	} else if n.Space == "" && n.Parent != nil && n.Parent.Space != "" && n.Parent.Prefix == "" {
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

	inv    *invoice.Invoice
	sp     *invoice.Spain // national block, never nil
	totals totals

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

// date writes a date (xs:date) when d is set.
func (w *writer) date(name string, d invoice.Date) {
	if !d.IsZero() {
		w.leafReq(name, d.String())
	}
}

// dateReq writes a date, empty when d is not set.
func (w *writer) dateReq(name string, d invoice.Date) {
	if d.IsZero() {
		w.leafReq(name, "")
		return
	}
	w.leafReq(name, d.String())
}

// number returns d with two to eight decimals (DoubleUpToEightDecimalType),
// or "" when d is not set.
func number(d invoice.Decimal) string {
	if !d.IsSet() {
		return ""
	}
	return d.Round(min(max(d.Scale(), 2), 8)).String()
}

// dec writes an amount, a price or a rate with two to eight decimals
// (DoubleUpToEightDecimalType) when d is set.
func (w *writer) dec(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.leafReq(name, number(d))
	}
}

// decReq writes an amount with two to eight decimals, empty when d is not
// set.
func (w *writer) decReq(name string, d invoice.Decimal) { w.leafReq(name, number(d)) }

// dec2 writes an amount with two decimals (DoubleTwoDecimalType) when d is
// set.
func (w *writer) dec2(name string, d invoice.Decimal) {
	if d.IsSet() {
		w.leafReq(name, d.StringFixed(2))
	}
}

// rate writes a tax rate with two to eight decimals, 0.00 when d is not
// set.
func (w *writer) rate(name string, d invoice.Decimal) {
	if !d.IsSet() {
		d = invoice.NewDecimal(0, 2)
	}
	w.leafReq(name, number(d))
}

// amountType writes an AmountType: TotalAmount, and EquivalentInEuros when
// euros is set.
func (w *writer) amountType(name string, total, euros invoice.Decimal) {
	w.start(name)
	w.decReq("TotalAmount", total)
	w.dec2("EquivalentInEuros", euros)
	w.end(name)
}

// period writes a PeriodDates.
func (w *writer) period(name string, p invoice.Period) {
	w.start(name)
	w.dateReq("StartDate", p.Start)
	w.dateReq("EndDate", p.End)
	w.end(name)
}

// lose records a business term that Facturae cannot carry.
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

// fileHeader writes FileHeader.
func (w *writer) fileHeader() {
	inv, sp := w.inv, w.sp
	w.start("FileHeader")
	w.leafReq("SchemaVersion", SchemaVersion)
	w.leafReq("Modality", or(sp.Modality, "I"))
	w.leafReq("InvoiceIssuerType", or(sp.IssuerType, issuerTypeOf(inv)))
	if p := sp.ThirdParty; p != nil {
		w.start("ThirdParty")
		w.taxIdentification(p, nil)
		w.entity(p, "Spain.ThirdParty")
		w.end("ThirdParty")
	}
	w.start("Batch")
	w.leafReq("BatchIdentifier", or(sp.BatchID, batchIDOf(inv)))
	w.leafReq("InvoicesCount", "1")
	w.amountType("TotalInvoicesAmount", w.totals.invoice, sp.BatchTotalEuros)
	w.amountType("TotalOutstandingAmount", w.totals.outstanding, sp.BatchOutstandingEuros)
	w.amountType("TotalExecutableAmount", w.totals.executable, sp.BatchExecutableEuros)
	w.leafReq("InvoiceCurrencyCode", inv.Currency)
	w.end("Batch")
	w.end("FileHeader")
}

// or returns a, or b when a is empty.
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// taxIdentification writes the TaxIdentification of p, whose counterpart is
// other.
func (w *writer) taxIdentification(p, other *invoice.Party) {
	w.start("TaxIdentification")
	w.leafReq("PersonTypeCode", personOf(p))
	w.leafReq("ResidenceTypeCode", residenceOf(p))
	w.leafReq("TaxIdentificationNumber", taxIDOf(p, other))
	w.end("TaxIdentification")
}

// business writes a BusinessType: the seller or the buyer.
func (w *writer) business(name string, p, other *invoice.Party, role string) {
	w.start(name)
	w.taxIdentification(p, other)
	id := ""
	for i, pid := range p.IDs {
		if i == 0 && pid.Scheme == "" && len(pid.ID) <= 10 && allDigits(pid.ID) {
			id = pid.ID
			continue
		}
		w.lose(partyTerm(role, "IDs"))
	}
	w.leaf("PartyIdentification", id)
	if s := p.Spain; s != nil && len(s.Centres) > 0 {
		w.start("AdministrativeCentres")
		for i := range s.Centres {
			w.centre(&s.Centres[i])
		}
		w.end("AdministrativeCentres")
	}
	w.entity(p, role)
	w.end(name)
	w.partyLosses(p, other, role)
}

// partyTerms gives the business terms of the party data that Facturae
// cannot carry, by role and field.
var partyTerms = map[string]map[string]string{
	"Seller": {"IDs": "BT-29", "LegalRegistration": "BT-30", "TaxRegistrationID": "BT-32", "Description": "BT-33", "ElectronicAddress": "BT-34", "Department": "BT-41"},
	"Buyer":  {"IDs": "BT-46", "LegalRegistration": "BT-47", "TaxRegistrationID": "EXT Buyer.TaxRegistrationID", "Description": "EXT Buyer.Description", "ElectronicAddress": "BT-49", "Department": "BT-56"},
}

// partyTerm returns the business term of a field of the party data of a
// role.
func partyTerm(role, field string) string {
	if t, ok := partyTerms[role][field]; ok {
		return t
	}
	return "EXT " + role + "." + field
}

// partyLosses records the data of p that Facturae cannot carry.
func (w *writer) partyLosses(p, other *invoice.Party, role string) {
	tin := taxIDOf(p, other)
	w.loseIf(p.LegalRegistration.ID != "" && p.LegalRegistration.ID != tin || p.LegalRegistration.Scheme != "", partyTerm(role, "LegalRegistration"))
	w.loseIf(p.TaxRegistrationID != "" && p.TaxRegistrationID != tin && "ES"+tin != p.TaxRegistrationID, partyTerm(role, "TaxRegistrationID"))
	w.loseIf(p.Description != "", partyTerm(role, "Description"))
	w.loseIf(!p.ElectronicAddress.IsZero(), partyTerm(role, "ElectronicAddress"))
	w.loseIf(p.Contact != nil && p.Contact.DepartmentName != "", partyTerm(role, "Department"))
	w.loseIf(p.VATID != "" && p.VATID != tin && p.VATID != "ES"+tin && p.Spain != nil && p.Spain.TaxID != "", partyTerm(role, "VATID"))
}

// allDigits reports whether s is made of ASCII digits only.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// entity writes the LegalEntity or the Individual of p, whose role names
// its data.
func (w *writer) entity(p *invoice.Party, role string) {
	s := p.Spain
	if s == nil {
		s = new(invoice.SpainParty)
	}
	if personOf(p) == "F" {
		given, first, second := s.GivenName, s.FirstSurname, s.SecondSurname
		if given == "" && first == "" && second == "" {
			given, first, _ = strings.Cut(p.Name, " ")
		}
		w.start("Individual")
		w.leafReq("Name", given)
		w.leafReq("FirstSurname", first)
		w.leaf("SecondSurname", second)
		w.address("AddressInSpain", "OverseasAddress", p.Address)
		w.contactDetails(p.Contact, s.Fax, s.Web, s.CNAE, s.INETownCode, s.OtherContact)
		w.end("Individual")
		w.loseIf(p.TradingName != "", partyTerm(role, "TradingName"))
		w.loseIf(s.Registration != nil, "EXT "+role+".Spain.Registration")
		return
	}
	w.start("LegalEntity")
	w.leafReq("CorporateName", p.Name)
	w.leaf("TradeName", p.TradingName)
	if r := s.Registration; r != nil {
		w.start("RegistrationData")
		w.leaf("Book", r.Book)
		w.leaf("RegisterOfCompaniesLocation", r.Location)
		w.leaf("Sheet", r.Sheet)
		w.leaf("Folio", r.Folio)
		w.leaf("Section", r.Section)
		w.leaf("Volume", r.Volume)
		w.leaf("AdditionalRegistrationData", r.Additional)
		w.end("RegistrationData")
	}
	w.address("AddressInSpain", "OverseasAddress", p.Address)
	w.contactDetails(p.Contact, s.Fax, s.Web, s.CNAE, s.INETownCode, s.OtherContact)
	w.end("LegalEntity")
}

// address writes an address: an address in Spain (inSpain) for the country
// ES with a postcode of five digits, an overseas address (overseas)
// otherwise, whose PostCodeAndTown is the postcode followed by the city.
// The address lines are joined by commas.
func (w *writer) address(inSpain, overseas string, a invoice.Address) {
	lines := a.Line1
	for _, l := range []string{a.Line2, a.Line3} {
		if l != "" {
			if lines != "" {
				lines += ", "
			}
			lines += l
		}
	}
	if a.CountryCode == "ES" && len(a.PostCode) == 5 && allDigits(a.PostCode) {
		w.start(inSpain)
		w.leafReq("Address", lines)
		w.leafReq("PostCode", a.PostCode)
		w.leafReq("Town", a.City)
		w.leafReq("Province", a.CountrySubdivision)
		w.leafReq("CountryCode", countryCode(a.CountryCode))
		w.end(inSpain)
		return
	}
	w.start(overseas)
	w.leafReq("Address", lines)
	w.leafReq("PostCodeAndTown", join(a.PostCode, a.City))
	w.leafReq("Province", a.CountrySubdivision)
	w.leafReq("CountryCode", countryCode(a.CountryCode))
	w.end(overseas)
}

// contactDetails writes ContactDetails when one of its values is set.
func (w *writer) contactDetails(c *invoice.Contact, fax, web, cnae, ine, other string) {
	var ct invoice.Contact
	if c != nil {
		ct = *c
	}
	if ct.Phone == "" && ct.Email == "" && ct.PersonName == "" && fax == "" && web == "" && cnae == "" && ine == "" && other == "" {
		return
	}
	w.start("ContactDetails")
	w.leaf("Telephone", ct.Phone)
	w.leaf("TeleFax", fax)
	w.leaf("WebAddress", web)
	w.leaf("ElectronicMail", ct.Email)
	w.leaf("ContactPersons", ct.PersonName)
	w.leaf("CnoCnae", cnae)
	w.leaf("INETownCode", ine)
	w.leaf("AdditionalContactDetails", other)
	w.end("ContactDetails")
}

// centre writes an AdministrativeCentre.
func (w *writer) centre(ac *invoice.AdministrativeCentre) {
	w.start("AdministrativeCentre")
	w.leaf("CentreCode", ac.Code)
	w.leaf("RoleTypeCode", ac.Role)
	w.leaf("Name", ac.Name)
	w.leaf("FirstSurname", ac.FirstSurname)
	w.leaf("SecondSurname", ac.SecondSurname)
	w.address("AddressInSpain", "OverseasAddress", ac.Address)
	w.contactDetails(ac.Contact, ac.Fax, ac.Web, ac.CNAE, ac.INETownCode, ac.OtherContact)
	w.leaf("PhysicalGLN", ac.PhysicalGLN)
	w.leaf("LogicalOperationalPoint", ac.LogicalPoint)
	w.leaf("CentreDescription", ac.Description)
	w.end("AdministrativeCentre")
}

// notes splits the invoice notes (BG-1): the first note without subject is
// the InvoiceDescription, the notes ABL are the legal literals, the first
// note AAI is the InvoiceAdditionalInformation. The others are lost.
func (w *writer) notes() (description string, legal []string, info string) {
	var hasDesc, hasInfo bool
	for _, n := range w.inv.Notes {
		switch {
		case n.SubjectCode == "" && !hasDesc:
			description, hasDesc = n.Content, true
		case n.SubjectCode == "ABL":
			legal = append(legal, n.Content)
		case n.SubjectCode == "AAI" && !hasInfo:
			info, hasInfo = n.Content, true
		default:
			w.lose("BG-1")
		}
		w.loseIf(n.ContentCode != "", "EXT Notes.ContentCode")
	}
	return description, legal, info
}

// invoice writes Invoice.
func (w *writer) invoice() {
	inv, sp := w.inv, w.sp
	description, legal, info := w.notes()
	w.start("Invoice")

	w.start("InvoiceHeader")
	series, num := splitNumber(inv.Number, sp.SeriesCode)
	w.leafReq("InvoiceNumber", num)
	w.leaf("InvoiceSeriesCode", series)
	w.leafReq("InvoiceDocumentType", or(sp.DocumentType, docTypeOf(inv.TypeCode)))
	class := or(sp.Class, invoice.SpanishClassOfTypeCode(inv.TypeCode))
	w.leafReq("InvoiceClass", class)
	if class == invoice.SpanishCorrective || sp.Corrective != nil {
		w.corrective()
	} else {
		w.loseIf(len(inv.PrecedingInvoices) > 0, "BG-3")
	}
	w.end("InvoiceHeader")

	w.start("InvoiceIssueData")
	w.dateReq("IssueDate", inv.IssueDate)
	w.date("OperationDate", inv.TaxPointDate)
	if pl := sp.PlaceOfIssue; pl != nil {
		w.start("PlaceOfIssue")
		w.leafReq("PostCode", pl.PostCode)
		w.leaf("PlaceOfIssueDescription", pl.Description)
		w.end("PlaceOfIssue")
	}
	if !inv.InvoicingPeriod.Start.IsZero() || !inv.InvoicingPeriod.End.IsZero() {
		w.period("InvoicingPeriod", inv.InvoicingPeriod)
	}
	w.loseIf(inv.InvoicingPeriod.Description != "", "EXT InvoicingPeriod.Description")
	w.leafReq("InvoiceCurrencyCode", inv.Currency)
	if x := inv.CurrencyExchange; x != nil {
		w.start("ExchangeRateDetails")
		w.decReq("ExchangeRate", x.Rate)
		w.dateReq("ExchangeRateDate", x.Date)
		w.end("ExchangeRateDetails")
		w.loseIf(x.SourceCurrency != "" && x.SourceCurrency != inv.Currency ||
			x.TargetCurrency != "" && x.TargetCurrency != or(inv.TaxCurrency, "EUR"), "EXT CurrencyExchange")
	}
	w.leafReq("TaxCurrencyCode", or(inv.TaxCurrency, inv.Currency))
	w.leafReq("LanguageName", or(inv.Language, "es"))
	w.leaf("InvoiceDescription", description)
	w.leaf("ReceiverTransactionReference", inv.PurchaseOrderReference)
	w.leaf("FileReference", inv.TenderReference())
	w.leaf("ReceiverContractReference", inv.ContractReference)
	w.end("InvoiceIssueData")

	w.start("TaxesOutputs")
	for i := range inv.VATBreakdown {
		v := &inv.VATBreakdown[i]
		s := v.Spain
		if s == nil {
			s = new(invoice.SpainVAT)
		}
		w.output(invoice.SpanishTaxTypeOfCategory(v.CategoryCode), v.Rate, v.TaxableAmount, s.TaxableEuros, v.TaxAmount, s.TaxEuros, s)
		w.loseIf(s.TaxableBase.IsSet() || s.TaxAmount.IsSet(), "EXT VATBreakdown.Spain")
		if (v.ExemptionReason != "" || v.ExemptionReasonCode != "") && !w.hasLine(v) {
			w.lose("BT-120")
		}
	}
	for i := range sp.Taxes {
		t := &sp.Taxes[i]
		o := t.Output
		if o == nil {
			o = new(invoice.SpainVAT)
		}
		w.output(t.Type, t.Rate, t.TaxableBase, t.TaxableEuros, t.Amount, t.AmountEuros, o)
	}
	w.end("TaxesOutputs")
	w.taxes("TaxesWithheld", sp.Withholdings)

	w.invoiceTotals()

	w.start("Items")
	for i := range inv.Lines {
		w.line(&inv.Lines[i], i)
	}
	w.end("Items")

	w.payments()

	if len(legal) > 0 {
		w.start("LegalLiterals")
		for _, l := range legal {
			w.leafReq("LegalReference", l)
		}
		w.end("LegalLiterals")
	}
	w.additionalData(info)
	w.end("Invoice")
}

// hasLine reports whether a line has the VAT category and rate of v, so
// that its exemption reason is written in the special taxable event of the
// line.
func (w *writer) hasLine(v *invoice.VATBreakdown) bool {
	for i := range w.inv.Lines {
		l := &w.inv.Lines[i]
		if l.VATCategoryCode == v.CategoryCode && l.VATRate.Equal(v.Rate) && invoice.SpecialEventOfCategory(l.VATCategoryCode) != "" {
			return true
		}
	}
	return false
}

// corrective writes Corrective: the corrected invoice is the first
// preceding invoice reference (BG-3).
func (w *writer) corrective() {
	inv := w.inv
	co := w.sp.Corrective
	if co == nil {
		co = new(invoice.Corrective)
	}
	var prec invoice.DocumentReference
	if len(inv.PrecedingInvoices) > 0 {
		prec = inv.PrecedingInvoices[0]
	}
	w.loseIf(len(inv.PrecedingInvoices) > 1, "BG-3")
	w.loseIf(prec.TypeCode != "" || prec.LineID != "", "EXT PrecedingInvoices")
	series, num := splitNumber(prec.ID, co.SeriesCode)
	reason, method, period := or(co.ReasonCode, defaultReason), or(co.Method, correctionMethodOf(inv)), co.TaxPeriod
	if period.IsZero() {
		period = taxPeriodOf(inv)
	}
	w.start("Corrective")
	w.leaf("InvoiceNumber", num)
	w.leaf("InvoiceSeriesCode", series)
	w.leafReq("ReasonCode", reason)
	w.leafReq("ReasonDescription", or(co.ReasonDescription, invoice.CorrectiveReason(reason)))
	w.period("TaxPeriod", period)
	w.leafReq("CorrectionMethod", method)
	w.leafReq("CorrectionMethodDescription", or(co.MethodDescription, invoice.CorrectionMethodName(method)))
	w.leaf("AdditionalReasonDescription", co.AdditionalReason)
	w.date("InvoiceIssueDate", prec.IssueDate)
	w.end("Corrective")
}

// defaultReason is the ReasonCode of a corrective invoice whose national
// block gives none: 16, the taxable base is corrected.
const defaultReason = "16"

// correctionMethodOf returns the CorrectionMethod of a corrective invoice
// whose national block gives none: 02 (rectificación por diferencias) when
// it states a negative total, as a credit note does once its amounts are
// negated, 01 (rectificación íntegra) otherwise.
func correctionMethodOf(inv *invoice.Invoice) string {
	if inv.Totals.GrandTotal.Sign() < 0 {
		return "02"
	}
	return "01"
}

// taxPeriodOf returns the TaxPeriod of a corrective invoice whose national
// block gives none: the invoicing period (BG-14) when it has both dates,
// else the calendar month of the issue date of the corrected invoice
// (BT-26), else that of the issue date (BT-2).
func taxPeriodOf(inv *invoice.Invoice) invoice.Period {
	if p := inv.InvoicingPeriod; !p.Start.IsZero() && !p.End.IsZero() {
		return invoice.Period{Start: p.Start, End: p.End}
	}
	d := inv.IssueDate
	if len(inv.PrecedingInvoices) > 0 && !inv.PrecedingInvoices[0].IssueDate.IsZero() {
		d = inv.PrecedingInvoices[0].IssueDate
	}
	if d.IsZero() {
		return invoice.Period{}
	}
	start := invoice.NewDate(d.Year(), d.Month(), 1)
	return invoice.Period{Start: start, End: invoice.DateOf(start.Time(nil).AddDate(0, 1, -1))}
}

// output writes a Tax of TaxesOutputs.
func (w *writer) output(taxType string, rate, base, baseEuros, amount, amountEuros invoice.Decimal, s *invoice.SpainVAT) {
	w.start("Tax")
	w.leafReq("TaxTypeCode", taxType)
	w.rate("TaxRate", rate)
	w.amountType("TaxableBase", base, baseEuros)
	w.amountType("TaxAmount", amount, amountEuros)
	if s.SpecialBase.IsSet() || s.SpecialBaseEuros.IsSet() {
		w.amountType("SpecialTaxableBase", s.SpecialBase, s.SpecialBaseEuros)
	}
	if s.SpecialAmount.IsSet() || s.SpecialAmountEuros.IsSet() {
		w.amountType("SpecialTaxAmount", s.SpecialAmount, s.SpecialAmountEuros)
	}
	w.dec2("EquivalenceSurcharge", s.SurchargeRate)
	if s.SurchargeAmount.IsSet() || s.SurchargeEuros.IsSet() {
		w.amountType("EquivalenceSurchargeAmount", s.SurchargeAmount, s.SurchargeEuros)
	}
	w.end("Tax")
}

// taxes writes taxes withheld (TaxesType) when there are some.
func (w *writer) taxes(name string, taxes []invoice.SpainTax) {
	if len(taxes) == 0 {
		return
	}
	w.start(name)
	for i := range taxes {
		t := &taxes[i]
		w.start("Tax")
		w.leafReq("TaxTypeCode", t.Type)
		w.rate("TaxRate", t.Rate)
		w.amountType("TaxableBase", t.TaxableBase, t.TaxableEuros)
		if t.Amount.IsSet() || t.AmountEuros.IsSet() {
			w.amountType("TaxAmount", t.Amount, t.AmountEuros)
		}
		w.loseIf(t.Output != nil, "EXT Spain.Withholdings.Output")
		w.end("Tax")
	}
	w.end(name)
}

// discounts writes the allowances (DiscountsAndRebates, GeneralDiscounts)
// or the charges (Charges, GeneralSurcharges) of acs. base is the amount on
// which a percentage applies in Facturae; prefix gives the business terms
// of the base and the reason code.
func (w *writer) discounts(name string, acs []invoice.AllowanceCharge, charge bool, base invoice.Decimal, doc bool) {
	item, prefix := "Discount", "Discount"
	if charge {
		item, prefix = "Charge", "Charge"
	}
	started := false
	for i := range acs {
		ac := &acs[i]
		if ac.Charge != charge {
			continue
		}
		if !started {
			w.start(name)
			started = true
		}
		w.start(item)
		w.leafReq(prefix+"Reason", ac.Reason)
		w.dec(prefix+"Rate", ac.Percentage)
		w.decReq(prefix+"Amount", ac.Amount)
		w.end(item)
		terms := [...]string{"BT-137", "BT-140", "BT-142", "BT-145"}
		if doc {
			terms = [...]string{"BT-93", "BT-98", "BT-100", "BT-105"}
		}
		k := 0
		if charge {
			k = 2
		}
		w.loseIf(ac.BaseAmount.IsSet() && (!ac.Percentage.IsSet() || !ac.BaseAmount.Equal(base)), terms[k])
		w.loseIf(ac.ReasonCode != "", terms[k+1])
		w.loseIf(ac.Italy != nil, "EXT AllowancesCharges.Italy")
	}
	if started {
		w.end(name)
	}
}

// invoiceTotals writes InvoiceTotals.
func (w *writer) invoiceTotals() {
	inv, sp, t, ft := w.inv, w.sp, &w.inv.Totals, w.totals
	w.start("InvoiceTotals")
	w.decReq("TotalGrossAmount", t.LineTotal)
	w.discounts("GeneralDiscounts", inv.AllowancesCharges, false, t.LineTotal, true)
	w.discounts("GeneralSurcharges", inv.AllowancesCharges, true, t.LineTotal, true)
	for i := range inv.AllowancesCharges {
		ac := &inv.AllowancesCharges[i]
		if ac.VATCategoryCode == "" {
			continue
		}
		if len(inv.VATBreakdown) != 1 || ac.VATCategoryCode != inv.VATBreakdown[0].CategoryCode || !ac.VATRate.Equal(inv.VATBreakdown[0].Rate) {
			w.lose(map[bool]string{false: "BT-95", true: "BT-102"}[ac.Charge])
		}
	}
	w.dec("TotalGeneralDiscounts", t.AllowanceTotal)
	w.dec("TotalGeneralSurcharges", t.ChargeTotal)
	w.decReq("TotalGrossAmountBeforeTaxes", t.TaxBasisTotal)
	w.decReq("TotalTaxOutputs", ft.taxOutputs)
	w.decReq("TotalTaxesWithheld", ft.withheld)
	w.decReq("InvoiceTotal", ft.invoice)
	if len(sp.Subsidies) > 0 {
		w.start("Subsidies")
		for _, s := range sp.Subsidies {
			w.start("Subsidy")
			w.leafReq("SubsidyDescription", s.Description)
			w.dec("SubsidyRate", s.Rate)
			w.decReq("SubsidyAmount", s.Amount)
			w.end("Subsidy")
		}
		w.end("Subsidies")
	}
	if len(sp.PaymentsOnAccount) > 0 {
		w.start("PaymentsOnAccount")
		for _, p := range sp.PaymentsOnAccount {
			w.start("PaymentOnAccount")
			w.dateReq("PaymentOnAccountDate", p.Date)
			w.decReq("PaymentOnAccountAmount", p.Amount)
			w.end("PaymentOnAccount")
		}
		w.end("PaymentsOnAccount")
	}
	if len(sp.ReimbursableExpenses) > 0 {
		w.start("ReimbursableExpenses")
		for _, e := range sp.ReimbursableExpenses {
			w.start("ReimbursableExpenses")
			w.taxID("ReimbursableExpensesSellerParty", e.Seller)
			w.taxID("ReimbursableExpensesBuyerParty", e.Buyer)
			w.date("IssueDate", e.IssueDate)
			w.leaf("InvoiceNumber", e.Number)
			w.leaf("InvoiceSeriesCode", e.SeriesCode)
			w.decReq("ReimbursableExpensesAmount", e.Amount)
			w.end("ReimbursableExpenses")
		}
		w.end("ReimbursableExpenses")
	}
	w.dec2("TotalFinancialExpenses", sp.FinancialExpenses)
	w.decReq("TotalOutstandingAmount", ft.outstanding)
	w.dec("TotalPaymentsOnAccount", t.PaidAmount)
	if a := sp.AmountWithheld; a != nil {
		w.start("AmountsWithheld")
		w.leafReq("WithholdingReason", a.Reason)
		w.dec("WithholdingRate", a.Rate)
		w.decReq("WithholdingAmount", a.Amount)
		w.end("AmountsWithheld")
	}
	w.decReq("TotalExecutableAmount", ft.executable)
	w.dec("TotalReimbursableExpenses", reimbursableTotal(sp))
	if p := sp.PaymentInKind; p != nil {
		w.start("PaymentInKind")
		w.leafReq("PaymentInKindReason", p.Reason)
		w.decReq("PaymentInKindAmount", p.Amount)
		w.end("PaymentInKind")
	}
	w.end("InvoiceTotals")
}

// taxID writes a TaxIdentificationType when t is set.
func (w *writer) taxID(name string, t *invoice.TaxIdentification) {
	if t == nil {
		return
	}
	w.start(name)
	w.leafReq("PersonTypeCode", t.PersonType)
	w.leafReq("ResidenceTypeCode", t.ResidenceType)
	w.leafReq("TaxIdentificationNumber", t.Number)
	w.end(name)
}

// defaultReasons gives the SpecialTaxableEventReason of the VAT categories
// that Facturae writes as exempt or not subject, when the VAT breakdown has
// no exemption reason: the exemption reason code that sets the category
// back when the document is read.
var defaultReasons = map[string]string{
	invoice.VATIntraCommunity: "VATEX-EU-IC", invoice.VATExport: "VATEX-EU-G", invoice.VATReverseCharge: "VATEX-EU-AE",
}

// line writes an InvoiceLine.
func (w *writer) line(l *invoice.Line, i int) {
	sl := l.Spain
	if sl == nil {
		sl = new(invoice.SpainLine)
	}
	w.start("InvoiceLine")
	w.leaf("IssuerContractReference", sl.IssuerContract)
	w.date("IssuerContractDate", sl.IssuerContractDate)
	w.leaf("IssuerTransactionReference", sl.IssuerTransaction)
	w.date("IssuerTransactionDate", sl.IssuerTransactionDate)
	w.leaf("ReceiverContractReference", sl.ReceiverContract)
	w.date("ReceiverContractDate", sl.ReceiverContractDate)
	w.leaf("ReceiverTransactionReference", l.OrderReference)
	w.date("ReceiverTransactionDate", sl.ReceiverTransactionDate)
	w.leaf("FileReference", sl.File)
	w.date("FileDate", sl.FileDate)
	if _, err := invoice.ParseDecimal(l.ID); err == nil {
		w.leafReq("SequenceNumber", l.ID)
	} else {
		w.loseIf(l.ID != "" && l.ID != strconv.Itoa(i+1), "BT-126")
	}
	notes := sl.DeliveryNotes
	if l.DeliveryNote != nil {
		notes = append([]invoice.DocumentReference{*l.DeliveryNote}, notes...)
	}
	if len(notes) > 0 {
		w.start("DeliveryNotesReferences")
		for _, d := range notes {
			w.start("DeliveryNote")
			w.leafReq("DeliveryNoteNumber", d.ID)
			w.date("DeliveryNoteDate", d.IssueDate)
			w.end("DeliveryNote")
			w.loseIf(d.TypeCode != "" || d.LineID != "", "EXT Lines.DeliveryNote")
		}
		w.end("DeliveryNotesReferences")
	}
	w.leafReq("ItemDescription", l.Item.Name)
	w.leafReq("Quantity", l.Quantity.String())
	unit := sl.UnitOfMeasure
	if unit == "" {
		unit = invoice.UnitOfMeasureOfCode(l.UnitCode)
		w.loseIf(unit == "" && l.UnitCode != "", "BT-130")
	}
	w.leaf("UnitOfMeasure", unit)
	w.decReq("UnitPriceWithoutTax", l.NetPrice)
	cost := sl.TotalCost
	if !cost.IsSet() {
		cost = totalCost(l)
	}
	w.decReq("TotalCost", cost)
	w.discounts("DiscountsAndRebates", l.AllowancesCharges, false, cost, false)
	w.discounts("Charges", l.AllowancesCharges, true, cost, false)
	w.decReq("GrossAmount", l.NetAmount)
	w.taxes("TaxesWithheld", sl.Withholdings)
	w.start("TaxesOutputs")
	v := sl.VAT
	if v == nil {
		v = new(invoice.SpainVAT)
	}
	base := v.TaxableBase
	if !base.IsSet() {
		base = l.NetAmount
	}
	amount := v.TaxAmount
	if !amount.IsSet() {
		amount = lineTaxAmount(base, l.VATRate)
		if !amount.IsSet() {
			amount = invoice.NewDecimal(0, 2)
		}
	}
	w.output(invoice.SpanishTaxTypeOfCategory(l.VATCategoryCode), l.VATRate, base, v.TaxableEuros, amount, v.TaxEuros, v)
	for k := range sl.Taxes {
		t := &sl.Taxes[k]
		o := t.Output
		if o == nil {
			o = new(invoice.SpainVAT)
		}
		w.output(t.Type, t.Rate, t.TaxableBase, t.TaxableEuros, t.Amount, t.AmountEuros, o)
	}
	w.end("TaxesOutputs")
	if !l.Period.Start.IsZero() || !l.Period.End.IsZero() {
		w.period("LineItemPeriod", l.Period)
	}
	w.date("TransactionDate", sl.TransactionDate)
	w.leaf("AdditionalLineItemInformation", l.Note)
	if event := or(sl.SpecialEvent, invoice.SpecialEventOfCategory(l.VATCategoryCode)); event != "" {
		reason := sl.SpecialEventReason
		if reason == "" {
			reason = reasonOf(w.inv, l)
		}
		if reason == "" {
			reason = defaultReasons[l.VATCategoryCode]
		}
		w.start("SpecialTaxableEvent")
		w.leafReq("SpecialTaxableEventCode", event)
		w.leafReq("SpecialTaxableEventReason", reason)
		w.end("SpecialTaxableEvent")
	}
	w.leaf("ArticleCode", l.Item.SellerID)
	w.end("InvoiceLine")
	w.lineLosses(l)
}

// lineLosses records the data of the line l that Facturae cannot carry.
func (w *writer) lineLosses(l *invoice.Line) {
	it := &l.Item
	w.loseIf(!l.ObjectIdentifier.IsZero(), "BT-128")
	w.loseIf(l.OrderLineReference != "", "BT-132")
	w.loseIf(l.AccountingReference != "", "BT-133")
	w.loseIf(l.PriceDiscount.IsSet() && !l.PriceDiscount.IsZero(), "BT-147")
	w.loseIf(l.GrossPrice.IsSet(), "BT-148")
	w.loseIf(l.PriceBaseQuantity.IsSet() && !l.PriceBaseQuantity.Equal(invoice.DecimalFromInt(1)), "BT-149")
	w.loseIf(l.PriceBaseQuantityUnit != "" && l.PriceBaseQuantityUnit != l.UnitCode, "BT-150")
	w.loseIf(it.Description != "", "BT-154")
	w.loseIf(it.BuyerID != "", "BT-156")
	w.loseIf(!it.GlobalID.IsZero(), "BT-157")
	w.loseIf(len(it.Classifications) > 0, "BT-158")
	w.loseIf(it.OriginCountry != "", "BT-159")
	w.loseIf(len(it.Attributes) > 0, "BG-32")
	w.loseIf(len(it.ReferencedProducts) > 0, "EXT Lines.Item.ReferencedProducts")
	w.loseIf(l.ParentID != "" || l.StatusCode != "" || l.StatusReasonCode != "", "EXT Lines.ParentID")
	w.loseIf(len(l.PriceAllowancesCharges) > 0, "EXT Lines.PriceAllowancesCharges")
	w.loseIf(l.DeliveryTerms != nil, "EXT Lines.DeliveryTerms")
	w.loseIf(len(l.UltimateCustomerOrders) > 0, "EXT Lines.UltimateCustomerOrders")
	w.loseIf(!l.ShipTo.IsZero() || !l.UltimateShipTo.IsZero(), "EXT Lines.ShipTo")
	w.loseIf(!l.DeliveryDate.IsZero(), "EXT Lines.DeliveryDate")
	w.loseIf(l.InvoiceReference != nil, "EXT Lines.InvoiceReference")
	w.loseIf(l.Period.Description != "", "EXT Lines.Period.Description")
	w.loseIf(l.Italy != nil, "EXT Lines.Italy")
}

// payments writes PaymentDetails: one installment per payment means.
func (w *writer) payments() {
	inv := w.inv
	var due invoice.Date
	for i, t := range inv.PaymentTerms {
		if i == 0 {
			due = t.DueDate
		}
		w.loseIf(t.Description != "", "BT-20")
		w.loseIf(t.DirectDebitMandateID != "", "BT-89")
		w.loseIf(i > 0 || t.PartialAmount.IsSet() || t.PaymentMeansID != "", "EXT PaymentTerms")
	}
	w.loseIf(inv.CreditorReferenceID != "", "BT-90")
	if len(inv.PaymentMeans) == 0 {
		w.loseIf(!due.IsZero(), "BT-9")
		w.loseIf(inv.RemittanceInformation != "", "BT-83")
		return
	}
	w.start("PaymentDetails")
	for i := range inv.PaymentMeans {
		pm := &inv.PaymentMeans[i]
		ps := pm.Spain
		if ps == nil {
			ps = new(invoice.SpainPayment)
		}
		w.start("Installment")
		d := ps.DueDate
		if d.IsZero() {
			d = due
		}
		if d.IsZero() {
			// The due date is required: without one, the payment is due
			// on the issue date.
			d = inv.IssueDate
		}
		w.dateReq("InstallmentDueDate", d)
		amount := ps.Amount
		if !amount.IsSet() {
			amount = inv.Totals.DuePayable
		}
		if amount.IsSet() {
			w.leafReq("InstallmentAmount", amount.StringFixed(2))
		} else {
			w.leafReq("InstallmentAmount", "")
		}
		w.leafReq("PaymentMeans", or(ps.Means, invoice.SpanishPaymentMeansOfTypeCode(pm.TypeCode)))
		if pm.PayeeIBAN != "" || pm.PayeeProprietaryID != "" || ps.CreditBank != "" || pm.PayeeBIC != "" {
			w.account("AccountToBeCredited", pm.PayeeIBAN, pm.PayeeProprietaryID, ps.CreditBank, ps.CreditBranch, ps.CreditBranchAddress, pm.PayeeBIC)
			w.loseIf(pm.PayeeIBAN != "" && pm.PayeeProprietaryID != "", "BT-84")
		}
		w.leaf("PaymentReconciliationReference", or(ps.Reconciliation, inv.RemittanceInformation))
		if pm.PayerIBAN != "" || ps.DebitAccount != "" || ps.DebitBank != "" || pm.PayerBIC != "" {
			w.account("AccountToBeDebited", pm.PayerIBAN, ps.DebitAccount, ps.DebitBank, ps.DebitBranch, ps.DebitBranchAddress, pm.PayerBIC)
		}
		w.leaf("CollectionAdditionalInformation", ps.CollectionInfo)
		w.leaf("RegulatoryReportingData", ps.RegulatoryReporting)
		w.leaf("DebitReconciliationReference", ps.DebitReconciliation)
		w.end("Installment")
		w.loseIf(pm.Information != "", "BT-82")
		w.loseIf(pm.PayeeAccountName != "", "BT-85")
		w.loseIf(pm.CardID != "", "BT-87")
		w.loseIf(pm.CardholderName != "", "BT-88")
		w.loseIf(pm.PayerAccountName != "", "BT-216")
		w.loseIf(pm.ID != "", "EXT PaymentMeans.ID")
		w.loseIf(pm.Italy != nil, "EXT PaymentMeans.Italy")
	}
	w.end("PaymentDetails")
}

// account writes an AccountType: the IBAN, or the account number, the bank
// and branch codes, the branch address and the BIC.
func (w *writer) account(name, iban, number, bank, branch string, address *invoice.Address, bic string) {
	w.start(name)
	if iban != "" {
		w.leafReq("IBAN", iban)
	} else {
		w.leafReq("AccountNumber", number)
	}
	w.leaf("BankCode", bank)
	w.leaf("BranchCode", branch)
	if address != nil {
		w.address("BranchInSpainAddress", "OverseasBranchAddress", *address)
	}
	if len(bic) == 8 {
		bic += "XXX" // the BIC of the head office has 11 characters
	}
	w.leaf("BIC", bic)
	w.end(name)
}

// additionalData writes AdditionalData: the related invoice, the attached
// documents (BG-24 with an attachment) and the additional information.
func (w *writer) additionalData(info string) {
	inv, sp := w.inv, w.sp
	var docs []*invoice.AdditionalDocument
	k := 0
	for i := range inv.AdditionalDocuments {
		d := &inv.AdditionalDocuments[i]
		switch {
		case d.TypeCode == "50":
			if d.ID != inv.TenderReference() || d.Description != "" || d.URI != "" || d.Attachment != nil {
				w.lose("BT-17")
			}
			continue
		case d.TypeCode == "130":
			w.lose("BT-18")
			continue
		case d.Attachment == nil:
			w.lose("BG-24")
			continue
		}
		k++
		docs = append(docs, d)
		w.loseIf(d.ID != strconv.Itoa(k), "BT-122")
		w.loseIf(d.URI != "", "BT-124")
		w.loseIf(d.AttachmentFilename != "", "BT-125-2")
		w.loseIf(!d.IssueDate.IsZero(), "EXT AdditionalDocuments.IssueDate")
	}
	if sp.RelatedInvoice == "" && len(docs) == 0 && info == "" {
		return
	}
	w.start("AdditionalData")
	w.leaf("RelatedInvoice", sp.RelatedInvoice)
	if len(docs) > 0 {
		w.start("RelatedDocuments")
		for _, d := range docs {
			sa := d.Spain
			if sa == nil {
				sa = new(invoice.SpainAttachment)
			}
			format := formatOf(d.AttachmentMimeCode)
			if format == "" {
				format = sa.Format
				w.loseIf(format == "" && d.AttachmentMimeCode != "", "BT-125-1")
			}
			w.start("Attachment")
			w.leaf("AttachmentCompressionAlgorithm", sa.Compression)
			w.leafReq("AttachmentFormat", format)
			w.leaf("AttachmentEncoding", sa.Encoding)
			w.leaf("AttachmentDescription", d.Description)
			if sa.Encoding == "" || sa.Encoding == "BASE64" {
				w.leafReq("AttachmentData", base64.StdEncoding.EncodeToString(d.Attachment))
			} else {
				w.leafReq("AttachmentData", string(d.Attachment))
			}
			w.end("Attachment")
		}
		w.end("RelatedDocuments")
	}
	w.leaf("InvoiceAdditionalInformation", info)
	w.end("AdditionalData")
}

// unwritten records the EN 16931 data of the invoice that Facturae cannot
// carry.
func (w *writer) unwritten() {
	inv := w.inv
	w.loseIf(inv.TaxPointDateCode != "", "BT-8")
	w.loseIf(inv.BuyerReference != "", "BT-10")
	w.loseIf(inv.ProjectReference != "" || inv.ProjectName != "", "BT-11")
	w.loseIf(inv.SalesOrderReference != "", "BT-14")
	w.loseIf(inv.ReceivingAdviceReference != "", "BT-15")
	w.loseIf(inv.DespatchAdviceReference != "", "BT-16")
	w.loseIf(inv.BuyerAccountingReference != "", "BT-19")
	w.loseIf(inv.BusinessProcess != "", "BT-23")
	w.loseIf(!inv.Payee.IsZero(), "BG-10")
	w.loseIf(!inv.SellerTaxRepresentative.IsZero(), "BG-11")
	w.loseIf(!inv.ShipTo.IsZero(), "BG-13")
	w.loseIf(!inv.DeliveryDate.IsZero(), "BT-72")
	w.loseIf(inv.Totals.RoundingAmount.IsSet() && !inv.Totals.RoundingAmount.IsZero(), "BT-114")
	if t := inv.Totals.TaxTotalInTaxCurrency; t.IsSet() && or(inv.TaxCurrency, inv.Currency) != inv.Currency {
		euros := false
		for i := range inv.VATBreakdown {
			if v := inv.VATBreakdown[i].Spain; v != nil && v.TaxEuros.IsSet() {
				euros = true
			}
		}
		w.loseIf(!euros || inv.TaxCurrency != "EUR", "BT-111")
	}
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
	w.loseIf(inv.DeliveryTerms != nil, "EXT DeliveryTerms")
	w.loseIf(inv.DeliveryNote != nil, "EXT DeliveryNote")
	w.loseIf(len(inv.UltimateCustomerOrders) > 0, "EXT UltimateCustomerOrders")
	w.loseIf(len(inv.LogisticsCharges) > 0, "EXT LogisticsCharges")
	w.loseIf(len(inv.ThirdPartyCharges) > 0, "BG-34")
	w.loseIf(inv.TestIndicator || inv.CopyIndicator, "EXT TestIndicator")
	// The national blocks of other countries.
	for _, e := range inv.NationalData() {
		if !strings.Contains(e, "Spain.") {
			w.lose(e)
		}
	}
}
