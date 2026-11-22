// Package render draws an invoice.Invoice as a PDF/A-3b document, ready for
// zerechnung.Create.
//
// The layout is a single A4 template: the seller and the document
// references at the top, the buyer and the delivery party, the lines, the
// VAT breakdown and the totals, then the payment instructions and the notes.
// The labels exist in English, French, German, Italian, Spanish, Portuguese
// and Dutch. The text is set in the Go fonts, embedded as subsets, so the
// document meets the font requirements of PDF/A; a character the fonts lack
// is shown as a question mark.
package render

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/invoice"
)

// Options tune Invoice.
type Options struct {
	// Language is the language of the labels: "en", "fr", "de", "it", "es",
	// "pt" or "nl" (a region subtag such as "fr-BE" is ignored). When it is
	// empty, the language of the invoice is used if it is one of these,
	// then the language of the seller's country (FR→fr, DE/AT/CH→de,
	// IT→it, ES→es, PT→pt, NL/BE→nl…), else English.
	Language string
	// Time is the creation date written in the XMP metadata and the
	// document information dictionary; time.Now() when zero. Set it to get
	// the same bytes for the same invoice.
	Time time.Time
}

// Languages returns the supported languages, sorted.
func Languages() []string {
	out := make([]string, 0, len(translations))
	for l := range translations {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// baseLanguage returns the language subtag of a language tag, lower case.
func baseLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

// language picks the language of the document, see Options.Language.
func language(inv *invoice.Invoice, requested string) (string, error) {
	if requested != "" {
		l := baseLanguage(requested)
		if translations[l] == nil {
			return "", fmt.Errorf("render: unsupported language %q (supported: %s)", requested, strings.Join(Languages(), ", "))
		}
		return l, nil
	}
	if l := baseLanguage(inv.Language); translations[l] != nil {
		return l, nil
	}
	if l, ok := countryLanguages[strings.ToUpper(inv.Seller.Address.CountryCode)]; ok {
		return l, nil
	}
	return "en", nil
}

// Invoice draws inv as a PDF/A-3b document. When the invoice has no totals
// (BT-112 and BT-115 unset), they are computed on a copy, as
// invoice.Invoice.Calculate does: inv is not modified.
func Invoice(inv *invoice.Invoice, o *Options) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("render: no invoice")
	}
	if o == nil {
		o = &Options{}
	}
	lang, err := language(inv, o.Language)
	if err != nil {
		return nil, err
	}
	fs, err := loadFaces()
	if err != nil {
		return nil, err
	}
	t := o.Time
	if t.IsZero() {
		t = time.Now()
	}
	l := translations[lang]
	d := &drawer{c: newCanvas(l, fs), l: l, inv: withTotals(inv)}
	d.draw()
	title := strings.TrimSpace(l.title(inv.TypeCode) + " " + inv.Number)
	return d.c.document(title, lang, t), nil
}

// withTotals returns inv, or a copy with its totals computed when it has
// none. The copy does not share what Calculate modifies.
func withTotals(inv *invoice.Invoice) *invoice.Invoice {
	if inv.Totals.DuePayable.IsSet() || inv.Totals.GrandTotal.IsSet() {
		return inv
	}
	cp := *inv
	cp.Lines = make([]invoice.Line, len(inv.Lines))
	for i, l := range inv.Lines {
		l.AllowancesCharges = append([]invoice.AllowanceCharge(nil), l.AllowancesCharges...)
		cp.Lines[i] = l
	}
	cp.AllowancesCharges = append([]invoice.AllowanceCharge(nil), inv.AllowancesCharges...)
	cp.VATBreakdown = append([]invoice.VATBreakdown(nil), inv.VATBreakdown...)
	cp.Calculate()
	return &cp
}

// drawer lays out an invoice on a canvas.
type drawer struct {
	c   *canvas
	l   *labels
	inv *invoice.Invoice
}

// Text sizes and line heights.
const (
	sizeBody  = 8.5
	lhBody    = 11.5
	sizeSmall = 7.5
	lhSmall   = 9.8
	sizeHead  = 7.5
	gap       = 16.0 // between sections
)

func (d *drawer) draw() {
	d.c.onNewPage = []func(){d.pageHeader}
	d.header()
	d.parties()
	d.precedingInvoices()
	d.lines()
	d.documentCharges()
	d.summary()
	d.payment()
	d.notes()
	d.footers()
}

// pair joins a label and its value.
func (d *drawer) pair(label, value string) string { return label + d.l.colon + value }

// docTitle is the title of the document followed by its number.
func (d *drawer) docTitle() string {
	return strings.TrimSpace(d.l.title(d.inv.TypeCode) + " " + d.l.number + " " + d.inv.Number)
}

// pageHeader draws the header of the pages after the first one.
func (d *drawer) pageHeader() {
	c := d.c
	c.text(c.bold, 9, left, top-8, fit(c.bold.face, 9, d.inv.Seller.Name, width/2-10), ink)
	c.textRight(c.reg, 9, right, top-8, fit(c.reg.face, 9, d.docTitle(), width/2-10), muted)
	c.hline(left, right, top-15, 0.5, rule)
	c.y = top - 26
}

// schemeLabel names the identifier schemes that are printed with a name.
func (d *drawer) schemeLabel(scheme string) string {
	switch scheme {
	case invoice.SchemeSIREN:
		return "SIREN"
	case invoice.SchemeSIRET:
		return "SIRET"
	case invoice.SchemeGLN:
		return "GLN"
	case "0060":
		return "DUNS"
	case invoice.SchemeKvK:
		return "KvK"
	case invoice.SchemeBEEnterprise:
		return "KBO-BCE"
	case invoice.SchemeCVR:
		return "CVR"
	case invoice.SchemeCodiceFiscale:
		return "Codice fiscale"
	case invoice.SchemePartitaIVA:
		return "Partita IVA"
	case invoice.SchemeLeitwegID:
		return "Leitweg-ID"
	}
	return d.l.registration
}

// styled is a line of text with its style.
type styled struct {
	text  string
	muted bool
}

// address returns the lines of a postal address.
func address(a invoice.Address) []string {
	var out []string
	for _, s := range []string{a.Line1, a.Line2, a.Line3} {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	city := strings.TrimSpace(strings.TrimSpace(a.PostCode) + " " + strings.TrimSpace(a.City))
	if a.CountrySubdivision != "" {
		if city != "" {
			city += ", "
		}
		city += a.CountrySubdivision
	}
	if a.CountryCode != "" {
		if city != "" {
			city += ", "
		}
		city += strings.ToUpper(a.CountryCode)
	}
	if city != "" {
		out = append(out, city)
	}
	return out
}

// partyLines returns the lines of a party below its name: trading name,
// address, identifiers and contact.
func (d *drawer) partyLines(p *invoice.Party) []styled {
	var out []styled
	if p.TradingName != "" && p.TradingName != p.Name {
		out = append(out, styled{p.TradingName, false})
	}
	for _, s := range address(p.Address) {
		out = append(out, styled{s, false})
	}
	if p.VATID != "" {
		out = append(out, styled{d.pair(d.l.vatID, p.VATID), true})
	}
	if p.TaxRegistrationID != "" {
		out = append(out, styled{d.pair(d.l.taxNumber, p.TaxRegistrationID), true})
	}
	if !p.LegalRegistration.IsZero() {
		out = append(out, styled{d.pair(d.schemeLabel(p.LegalRegistration.Scheme), p.LegalRegistration.ID), true})
	}
	for _, id := range p.IDs {
		if id.ID == "" || id.ID == p.LegalRegistration.ID {
			continue
		}
		label := "ID"
		if id.Scheme != "" {
			label = d.schemeLabel(id.Scheme)
		}
		out = append(out, styled{d.pair(label, id.ID), true})
	}
	if ct := p.Contact; ct != nil {
		name := strings.TrimSpace(strings.TrimSpace(ct.PersonName) + " " + strings.TrimSpace(ct.DepartmentName))
		if name != "" {
			out = append(out, styled{d.pair(d.l.contact, name), true})
		}
		if ct.Phone != "" {
			out = append(out, styled{d.pair(d.l.phone, ct.Phone), true})
		}
		if ct.Email != "" {
			out = append(out, styled{d.pair(d.l.email, ct.Email), true})
		}
	}
	if e := p.ElectronicAddress; e.Scheme == invoice.SchemeEmail && e.ID != "" && (p.Contact == nil || p.Contact.Email != e.ID) {
		out = append(out, styled{d.pair(d.l.email, e.ID), true})
	}
	return out
}

// header draws the seller on the left, the title and the references of the
// document on the right.
func (d *drawer) header() {
	c, l, inv := d.c, d.l, d.inv
	const rightX = 318.0 // left edge of the right column

	// Right column: title, number, dates and references.
	c.textRight(c.bold, 20, right, top-14, l.title(inv.TypeCode), accent)
	y := top - 32
	if inv.Number != "" {
		c.textRight(c.bold, 10.5, right, y, l.number+" "+inv.Number, ink)
	}
	for _, m := range []struct {
		on   bool
		text string
	}{{inv.CopyIndicator, l.copy}, {inv.TestIndicator, l.test}} {
		if m.on {
			y -= 12
			c.textRight(c.bold, 8, right, y, strings.ToUpper(m.text), accent)
		}
	}
	y -= 6
	var due invoice.Date
	for _, t := range inv.PaymentTerms {
		if !t.DueDate.IsZero() {
			due = t.DueDate
			break
		}
	}
	project := inv.ProjectReference
	if inv.ProjectName != "" && inv.ProjectName != project {
		project = strings.TrimSpace(project + " " + inv.ProjectName)
	}
	for _, p := range [][2]string{
		{l.issueDate, l.date(inv.IssueDate)},
		{l.dueDate, l.date(due)},
		{l.deliveryDate, l.date(inv.DeliveryDate)},
		{l.period, l.periodText(inv.InvoicingPeriod)},
		{l.taxPointDate, l.date(inv.TaxPointDate)},
		{l.buyerReference, inv.BuyerReference},
		{l.orderReference, inv.PurchaseOrderReference},
		{l.salesOrder, inv.SalesOrderReference},
		{l.contractReference, inv.ContractReference},
		{l.projectReference, project},
		{l.despatchAdvice, inv.DespatchAdviceReference},
		{l.tender, inv.TenderReference()},
	} {
		if p[1] == "" {
			continue
		}
		y -= 12
		label := fit(c.reg.face, 8, p[0], 118)
		c.text(c.reg, 8, rightX, y, label, muted)
		valueX := rightX + c.reg.measure(label, 8) + 10
		lines := wrap(c.reg.face, sizeBody, p[1], right-valueX)
		for i, s := range lines {
			if i > 0 {
				y -= 10.5
			}
			c.textRight(c.reg, sizeBody, right, y, s, ink)
		}
	}
	rightBottom := y

	// Left column: the seller.
	s := &inv.Seller
	sy := top - 12
	for i, line := range wrap(c.bold.face, 13, s.Name, rightX-left-16) {
		if i > 0 {
			sy -= 15
		}
		c.text(c.bold, 13, left, sy, line, ink)
	}
	sy -= 4
	lines := d.partyLines(s)
	if s.Description != "" {
		lines = append(lines, styled{s.Description, true})
	}
	for _, ln := range lines {
		for _, part := range wrap(c.reg.face, sizeBody, ln.text, rightX-left-16) {
			sy -= 10.5
			col := ink
			if ln.muted {
				col = muted
			}
			c.text(c.reg, sizeBody, left, sy, part, col)
		}
	}
	c.y = min(sy, rightBottom) - 24
}

// parties draws the buyer and, when there is one, the deliver-to party.
func (d *drawer) parties() {
	c, l, inv := d.c, d.l, d.inv
	type block struct {
		label string
		p     *invoice.Party
	}
	blocks := []block{{l.billTo, &inv.Buyer}}
	if !inv.ShipTo.IsZero() {
		blocks = append(blocks, block{l.shipTo, inv.ShipTo})
	}
	const sep = 18.0
	colW := (width - sep) / 2
	lowest := c.y
	for i, b := range blocks {
		x := left + float64(i)*(colW+sep)
		y := c.y
		c.text(c.bold, sizeHead, x, y-8, strings.ToUpper(b.label), accent)
		c.hline(x, x+colW, y-12, 0.6, accent)
		y -= 26
		for j, line := range wrap(c.bold.face, 10.5, b.p.Name, colW) {
			if j > 0 {
				y -= 13
			}
			c.text(c.bold, 10.5, x, y, line, ink)
		}
		for _, ln := range d.partyLines(b.p) {
			col := ink
			if ln.muted {
				col = muted
			}
			for _, part := range wrap(c.reg.face, sizeBody, ln.text, colW) {
				y -= 11
				c.text(c.reg, sizeBody, x, y, part, col)
			}
		}
		lowest = min(lowest, y)
	}
	c.y = lowest - gap - 4
}

// precedingInvoices draws the invoices that a credit note or a corrected
// invoice refers to.
func (d *drawer) precedingInvoices() {
	c, l := d.c, d.l
	for _, p := range d.inv.PrecedingInvoices {
		if p.ID == "" {
			continue
		}
		v := p.ID
		if !p.IssueDate.IsZero() {
			v += " (" + l.date(p.IssueDate) + ")"
		}
		c.need(lhBody)
		c.text(c.reg, sizeBody, left, c.y-lhBody*0.75, d.pair(l.precedingInvoice, v), ink)
		c.y -= lhBody
	}
	if len(d.inv.PrecedingInvoices) > 0 {
		c.y -= 8
	}
}

// Columns of the lines table: x of the left edge and width.
type column struct {
	x, w  float64
	right bool
}

var (
	colNo     = column{left, 24, false}
	colQty    = column{right - 260, 44, true}
	colUnit   = column{right - 216, 40, false}
	colPrice  = column{right - 176, 64, true}
	colVAT    = column{right - 112, 40, true}
	colAmount = column{right - 72, 72, true}
	colDesc   = column{left + 24, right - 260 - (left + 24), false}
)

const cellPad = 4.0

// cell draws s in a column, aligned and shortened to fit.
func (d *drawer) cell(f *usedFont, size float64, col column, y float64, s string, color rgb) {
	s = fit(f.face, size, s, col.w-2*cellPad)
	if col.right {
		d.c.textRight(f, size, col.x+col.w-cellPad, y, s, color)
	} else {
		d.c.text(f, size, col.x+cellPad, y, s, color)
	}
}

// tableHeader draws the header row of the lines table.
func (d *drawer) tableHeader() {
	c, l := d.c, d.l
	const h = 17.0
	c.rect(left, c.y-h, width, h, band)
	y := c.y - 11.5
	d.cell(c.bold, sizeHead, colNo, y, l.colNo, ink)
	d.cell(c.bold, sizeHead, colDesc, y, l.colDescription, ink)
	d.cell(c.bold, sizeHead, colQty, y, l.colQty, ink)
	d.cell(c.bold, sizeHead, colUnit, y, l.colUnit, ink)
	d.cell(c.bold, sizeHead, colPrice, y, l.colUnitPrice, ink)
	d.cell(c.bold, sizeHead, colVAT, y, l.colVAT, ink)
	d.cell(c.bold, sizeHead, colAmount, y, l.colAmount, ink)
	c.y -= h
}

// vatCell is the VAT column of a line: the rate, with the category when it
// is not the standard rate.
func (d *drawer) vatCell(category string, rate invoice.Decimal) string {
	s := d.l.rate(rate)
	if category != "" && category != invoice.VATStandard {
		if s != "" {
			s += " "
		}
		s += category
	}
	return s
}

// signed returns the amount of an allowance (negative) or a charge.
func signed(ac invoice.AllowanceCharge) invoice.Decimal {
	if !ac.Charge && ac.Amount.IsSet() {
		return ac.Amount.Neg()
	}
	return ac.Amount
}

// acLabel describes an allowance or a charge: its kind, percentage and
// reason.
func (d *drawer) acLabel(ac invoice.AllowanceCharge) string {
	s := d.l.allowance
	if ac.Charge {
		s = d.l.charge
	}
	if ac.Percentage.IsSet() {
		s += " " + d.l.rate(ac.Percentage)
		if ac.BaseAmount.IsSet() {
			s += " × " + d.l.amount(ac.BaseAmount)
		}
	}
	reason := ac.Reason
	if reason == "" {
		reason = ac.ReasonCode
	}
	if reason != "" {
		s += " (" + reason + ")"
	}
	return s
}

// subline is a line of text of a table row.
type subline struct {
	text  string
	f     *usedFont
	size  float64
	lh    float64
	color rgb
}

// lines draws the table of the invoice lines.
func (d *drawer) lines() {
	c, l, inv := d.c, d.l, d.inv
	if len(inv.Lines) == 0 {
		return
	}
	c.need(17 + 2*lhBody)
	d.tableHeader()
	c.onNewPage = append(c.onNewPage, d.tableHeader)
	defer func() { c.onNewPage = c.onNewPage[:len(c.onNewPage)-1] }()

	for _, ln := range inv.Lines {
		indent := 0.0
		if ln.ParentID != "" {
			indent = 8
		}
		descW := colDesc.w - 2*cellPad - indent
		var subs []subline
		name := ln.Item.Name
		if name == "" {
			name = ln.Item.Description
		}
		for _, s := range wrap(c.reg.face, sizeBody, name, descW) {
			subs = append(subs, subline{s, c.reg, sizeBody, lhBody, ink})
		}
		if len(subs) == 0 {
			subs = append(subs, subline{"", c.reg, sizeBody, lhBody, ink})
		}
		small := func(s string) {
			for _, part := range wrap(c.reg.face, sizeSmall, s, descW) {
				subs = append(subs, subline{part, c.reg, sizeSmall, lhSmall, muted})
			}
		}
		if ln.Item.Name != "" && ln.Item.Description != "" {
			small(ln.Item.Description)
		}
		var refs []string
		if ln.Item.SellerID != "" {
			refs = append(refs, d.pair(l.itemNo, ln.Item.SellerID))
		}
		if ln.Item.GlobalID.ID != "" {
			label := "ID"
			if ln.Item.GlobalID.Scheme == "0160" {
				label = "GTIN"
			}
			refs = append(refs, d.pair(label, ln.Item.GlobalID.ID))
		}
		if ln.Item.OriginCountry != "" {
			refs = append(refs, d.pair(l.origin, ln.Item.OriginCountry))
		}
		if len(refs) > 0 {
			small(strings.Join(refs, " · "))
		}
		for _, a := range ln.Item.Attributes {
			if a.Name != "" || a.Value != "" {
				small(d.pair(a.Name, a.Value))
			}
		}
		if p := l.periodText(ln.Period); p != "" {
			small(d.pair(l.period, p))
		}
		if !ln.DeliveryDate.IsZero() {
			small(d.pair(l.deliveryDate, l.date(ln.DeliveryDate)))
		}
		for _, ac := range ln.AllowancesCharges {
			small(d.acLabel(ac) + l.colon + l.amount(signed(ac)))
		}
		if ln.Note != "" {
			small(ln.Note)
		}

		price := l.num(ln.NetPrice, 2)
		if ln.PriceBaseQuantity.IsSet() && !ln.PriceBaseQuantity.Equal(invoice.DecimalFromInt(1)) {
			price += "/" + l.num(ln.PriceBaseQuantity, 0)
		}

		c.y -= cellPad
		c.need(subs[0].lh + cellPad)
		for k, s := range subs {
			c.need(s.lh)
			base := c.y - s.lh*0.75
			c.text(s.f, s.size, colDesc.x+cellPad+indent, base, s.text, s.color)
			if k == 0 {
				d.cell(c.reg, sizeBody, colNo, base, ln.ID, muted)
				d.cell(c.reg, sizeBody, colQty, base, l.num(ln.Quantity, 0), ink)
				d.cell(c.reg, sizeBody, colUnit, base, l.unit(ln.UnitCode), ink)
				d.cell(c.reg, sizeBody, colPrice, base, price, ink)
				d.cell(c.reg, sizeBody, colVAT, base, d.vatCell(ln.VATCategoryCode, ln.VATRate), ink)
				d.cell(c.reg, sizeBody, colAmount, base, l.amount(ln.NetAmount), ink)
			}
			c.y -= s.lh
		}
		c.y -= cellPad
		c.hline(left, right, c.y, 0.4, rule)
	}
	c.y -= gap
}

// row is a line of a block of label and amount rows.
type row struct {
	label, value string
	bold         bool
}

// documentCharges draws the allowances and charges of the document, the
// logistics charges and the charges collected for a third party.
func (d *drawer) documentCharges() {
	c, l, inv := d.c, d.l, d.inv
	type item struct{ label, vat, amount string }
	var items []item
	for _, ac := range inv.AllowancesCharges {
		items = append(items, item{d.acLabel(ac), d.vatCell(ac.VATCategoryCode, ac.VATRate), l.amount(signed(ac))})
	}
	for _, lc := range inv.LogisticsCharges {
		items = append(items, item{l.charge + " (" + lc.Description + ")", d.vatCell(lc.VATCategoryCode, lc.VATRate), l.amount(lc.Amount)})
	}
	if len(items) == 0 {
		return
	}
	c.need(14 + lhBody)
	c.text(c.bold, 9, left, c.y-9, l.allowancesCharges, accent)
	c.y -= 16
	for _, it := range items {
		lines := wrap(c.reg.face, sizeBody, it.label, colVAT.x-left-cellPad)
		c.need(float64(len(lines)) * lhBody)
		for k, s := range lines {
			base := c.y - lhBody*0.75
			c.text(c.reg, sizeBody, left, base, s, ink)
			if k == 0 {
				d.cell(c.reg, sizeBody, colVAT, base, it.vat, ink)
				d.cell(c.reg, sizeBody, colAmount, base, it.amount, ink)
			}
			c.y -= lhBody
		}
		c.y -= 2
		c.hline(left, right, c.y, 0.4, rule)
		c.y -= 2
	}
	c.y -= gap
}

// vatW is the width of the VAT breakdown table.
const vatW = 268.0

// vatColumns returns the right edges of the columns of the VAT breakdown
// table that starts at x: category, rate, taxable amount, VAT amount.
func vatColumns(x float64) [4]float64 {
	return [4]float64{x + 100, x + 140, x + 206, x + vatW}
}

// summary draws the VAT breakdown on the left and the totals on the right.
func (d *drawer) summary() {
	c, l, inv := d.c, d.l, d.inv
	t := inv.Totals
	cur := inv.Currency

	// Totals.
	var rows []row
	add := func(label string, v invoice.Decimal, bold bool) {
		if v.IsSet() {
			rows = append(rows, row{label, l.money(v, cur), bold})
		}
	}
	hasAC := (t.AllowanceTotal.IsSet() && !t.AllowanceTotal.IsZero()) || (t.ChargeTotal.IsSet() && !t.ChargeTotal.IsZero())
	if hasAC {
		add(l.lineTotal, t.LineTotal, false)
		if t.AllowanceTotal.IsSet() && !t.AllowanceTotal.IsZero() {
			add(l.allowanceTotal, t.AllowanceTotal.Neg(), false)
		}
		if t.ChargeTotal.IsSet() && !t.ChargeTotal.IsZero() {
			add(l.chargeTotal, t.ChargeTotal, false)
		}
	}
	add(l.totalNet, t.TaxBasisTotal, false)
	add(l.totalVAT, t.TaxTotal, false)
	if inv.TaxCurrency != "" && inv.TaxCurrency != cur && t.TaxTotalInTaxCurrency.IsSet() {
		rows = append(rows, row{l.totalVAT + " (" + inv.TaxCurrency + ")", l.money(t.TaxTotalInTaxCurrency, inv.TaxCurrency), false})
	}
	add(l.totalGross, t.GrandTotal, true)
	if t.PaidAmount.IsSet() && !t.PaidAmount.IsZero() {
		add(l.paid, t.PaidAmount.Neg(), false)
	}
	if t.RoundingAmount.IsSet() && !t.RoundingAmount.IsZero() {
		add(l.rounding, t.RoundingAmount, false)
	}
	for _, tp := range inv.ThirdPartyCharges {
		add(tp.Reason, tp.Amount, false)
	}

	// VAT breakdown.
	vatX := left
	cols := vatColumns(vatX)
	type vatRow struct {
		cat   []string
		cells [3]string
		note  []string
	}
	var vats []vatRow
	for _, b := range inv.VATBreakdown {
		r := vatRow{
			cat:   wrap(c.reg.face, 8, l.category(b.CategoryCode), cols[0]-vatX-cellPad),
			cells: [3]string{l.rate(b.Rate), l.amount(b.TaxableAmount), l.amount(b.TaxAmount)},
		}
		note := b.ExemptionReason
		if b.ExemptionReasonCode != "" {
			if note != "" {
				note += " "
			}
			note += "(" + b.ExemptionReasonCode + ")"
		}
		if note != "" {
			r.note = wrap(c.reg.face, sizeSmall, note, vatW-cellPad)
		}
		vats = append(vats, r)
	}

	// Heights, to keep the block on one page.
	vatH := 0.0
	if len(vats) > 0 {
		vatH = 16 + 15
		for _, r := range vats {
			vatH += float64(len(r.cat))*10.5 + float64(len(r.note))*lhSmall + 5
		}
	}
	totW := 214.0
	totX := right - totW
	totH := float64(len(rows))*15 + 6
	c.need(max(vatH, totH))
	y0 := c.y

	if len(vats) > 0 {
		y := y0
		c.text(c.bold, 9, vatX, y-9, l.vatBreakdown, accent)
		y -= 16
		c.rect(vatX, y-15, vatW, 15, band)
		hb := y - 10.5
		c.text(c.bold, sizeHead, vatX+cellPad, hb, fit(c.bold.face, sizeHead, l.vatCategory, cols[0]-vatX-2*cellPad), ink)
		for i, h := range []string{l.vatRate, l.taxable, l.vatAmount} {
			c.textRight(c.bold, sizeHead, cols[i+1]-cellPad, hb, fit(c.bold.face, sizeHead, h, cols[i+1]-cols[i]-cellPad), ink)
		}
		y -= 15
		for _, r := range vats {
			y -= 2
			for k, s := range r.cat {
				base := y - 8
				c.text(c.reg, 8, vatX+cellPad, base, s, ink)
				if k == 0 {
					for i, v := range r.cells {
						c.textRight(c.reg, 8, cols[i+1]-cellPad, base, v, ink)
					}
				}
				y -= 10.5
			}
			for _, s := range r.note {
				c.text(c.reg, sizeSmall, vatX+cellPad, y-7, s, muted)
				y -= lhSmall
			}
			y -= 3
			c.hline(vatX, vatX+vatW, y, 0.4, rule)
		}
		c.y = min(c.y, y)
	}

	y := y0
	for i, r := range rows {
		f, size := c.reg, 9.0
		if r.bold {
			f, size = c.bold, 9.5
		}
		if i > 0 && r.bold {
			c.hline(totX, right, y-1, 0.6, ink)
		}
		y -= 15
		c.text(f, size, totX, y+4, fit(f.face, size, r.label, totW-90), ink)
		c.textRight(f, size, right-cellPad, y+4, r.value, ink)
	}
	if t.DuePayable.IsSet() {
		y -= 6
		const h = 22.0
		c.rect(totX-6, y-h, totW+6, h, accent)
		white := rgb{1, 1, 1}
		c.text(c.bold, 10.5, totX, y-14.5, fit(c.bold.face, 10.5, l.due, totW-100), white)
		c.textRight(c.bold, 10.5, right-cellPad, y-14.5, l.money(t.DuePayable, cur), white)
		y -= h
	}
	c.y = min(c.y, y) - gap - 4
}

// section draws the title of a section, kept with its first line.
func (d *drawer) section(title string, firstLine float64) {
	c := d.c
	c.need(18 + firstLine)
	c.text(c.bold, 9, left, c.y-9, title, accent)
	c.hline(left, right, c.y-13, 0.6, rule)
	c.y -= 20
}

// labelled draws "label: value" lines, the value wrapped under itself.
func (d *drawer) labelled(items [][2]string) {
	c, l := d.c, d.l
	labelW := 0.0
	for _, it := range items {
		if it[0] != "" {
			labelW = max(labelW, c.reg.measure(it[0]+l.colon, sizeBody))
		}
	}
	labelW = min(labelW, 150)
	for _, it := range items {
		x := left
		if it[0] != "" {
			x = left + labelW + 10
		}
		lines := wrap(c.reg.face, sizeBody, it[1], right-x)
		for k, s := range lines {
			c.need(lhBody)
			base := c.y - lhBody*0.75
			if k == 0 && it[0] != "" {
				c.text(c.reg, sizeBody, left, base, fit(c.reg.face, sizeBody, it[0]+strings.TrimRight(l.colon, " "), labelW), muted)
			}
			c.text(c.reg, sizeBody, x, base, s, ink)
			c.y -= lhBody
		}
	}
}

// payment draws the payment instructions and terms.
func (d *drawer) payment() {
	l, inv := d.l, d.inv
	var items [][2]string
	for _, m := range inv.PaymentMeans {
		v := l.meansLabel(m.TypeCode)
		if m.Information != "" && m.Information != v {
			v = strings.TrimSpace(v + " – " + m.Information)
		}
		items = append(items, [2]string{l.paymentMeans, v})
		if m.PayeeIBAN != "" {
			items = append(items, [2]string{l.iban, iban(m.PayeeIBAN)})
		}
		if m.PayeeProprietaryID != "" {
			items = append(items, [2]string{l.accountID, m.PayeeProprietaryID})
		}
		if m.PayeeBIC != "" {
			items = append(items, [2]string{l.bic, m.PayeeBIC})
		}
		if m.PayeeAccountName != "" {
			items = append(items, [2]string{l.accountName, m.PayeeAccountName})
		}
		if m.CardID != "" {
			v := "•••• " + m.CardID
			if m.CardholderName != "" {
				v += " (" + m.CardholderName + ")"
			}
			items = append(items, [2]string{l.card, v})
		}
		if m.PayerIBAN != "" {
			items = append(items, [2]string{l.debitedAccount, iban(m.PayerIBAN)})
		}
	}
	if inv.RemittanceInformation != "" {
		items = append(items, [2]string{l.paymentReference, inv.RemittanceInformation})
	}
	if !inv.Payee.IsZero() && inv.Payee.Name != "" {
		items = append(items, [2]string{l.payee, inv.Payee.Name})
	}
	for _, t := range inv.PaymentTerms {
		if t.Description != "" {
			items = append(items, [2]string{"", t.Description})
		}
		if !t.DueDate.IsZero() && len(inv.PaymentTerms) > 1 {
			v := l.date(t.DueDate)
			if t.PartialAmount.IsSet() {
				v += " – " + l.money(t.PartialAmount, inv.Currency)
			}
			items = append(items, [2]string{l.dueDate, v})
		}
		if t.DirectDebitMandateID != "" {
			items = append(items, [2]string{l.mandate, t.DirectDebitMandateID})
		}
	}
	if inv.CreditorReferenceID != "" {
		items = append(items, [2]string{l.creditorID, inv.CreditorReferenceID})
	}
	if len(items) == 0 {
		return
	}
	d.section(l.payment, lhBody)
	d.labelled(items)
	d.c.y -= gap
}

// notes draws the notes of the invoice.
func (d *drawer) notes() {
	c := d.c
	var notes []string
	for _, n := range d.inv.Notes {
		if s := strings.TrimSpace(n.Content); s != "" {
			notes = append(notes, s)
		}
	}
	if len(notes) == 0 {
		return
	}
	d.section(d.l.notes, lhSmall)
	for _, n := range notes {
		for _, s := range wrap(c.reg.face, 8, n, width) {
			c.need(lhSmall + 0.5)
			c.text(c.reg, 8, left, c.y-7.5, s, ink)
			c.y -= lhSmall + 0.5
		}
		c.y -= 4
	}
}

// footers draws the footer of every page: the seller's legal identity and
// the page number.
func (d *drawer) footers() {
	c, l, s := d.c, d.l, &d.inv.Seller
	parts := []string{s.Name}
	if s.Description != "" {
		parts = append(parts, s.Description)
	}
	if !s.LegalRegistration.IsZero() {
		parts = append(parts, d.pair(d.schemeLabel(s.LegalRegistration.Scheme), s.LegalRegistration.ID))
	}
	if s.VATID != "" {
		parts = append(parts, d.pair(l.vatID, s.VATID))
	}
	var nonEmpty []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	legal := strings.Join(nonEmpty, " · ")
	n := len(c.pages)
	for i, p := range c.pages {
		c.cur = p
		c.hline(left, right, footerY+11, 0.5, rule)
		page := fmt.Sprintf("%s %d/%d", l.page, i+1, n)
		c.textRight(c.reg, 7, right, footerY, page, muted)
		c.text(c.reg, 7, left, footerY, fit(c.reg.face, 7, legal, width-c.reg.measure(page, 7)-20), muted)
	}
}
