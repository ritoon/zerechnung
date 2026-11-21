package ubl

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The Italian extensions of the domestic invoice (EXT-IT).
//
// A CIUS-IT invoice of a seller in Italy declares the extension in its
// specification identifier (see invoice.ItalianExtension). Appendix 4.2 of
// the Italian technical rules (Regole tecniche, art. 3 d.lgs. 148/2018) then
// gives the UBL elements that carry the FatturaPA data outside EN 16931:
// seven UBL extensions whose URI begins with itURN, and elements of UBL that
// the syntax binding of EN 16931 does not use. Parse reads them into the
// national blocks of the model (invoice.Italy, Party.Italy, VATBreakdown.Italy
// and PaymentMeans.Italy), in the form package fatturapa gives them, and
// Marshal writes them back, so that an invoice converted from FatturaPA to
// UBL and back keeps that data.

// itURN begins the URI (ext:ExtensionURI) of each extension of EXT-IT; the
// name of the extension follows it, then possibly "::" and an index.
const itURN = "urn:fdc:agid.gov.it:fatturapa:"

// Names of the extensions of EXT-IT, after itURN, with the FatturaPA element
// each one carries.
const (
	itIssuingSystem = "SistemaEmittente"            // attribute SistemaEmittente
	itIssuer        = "SoggettoEmittente"           // 1.6 SoggettoEmittente
	itDocumentType  = "TipoDocumento"               // 2.1.1.1 TipoDocumento
	itWithholding   = "TipoRitenuta"                // 2.1.1.5.1 TipoRitenuta, one per withholding
	itArt73         = "Art73"                       // 2.1.1.12 Art73
	itRounding      = "RiepilogoIVA:Arrotondamento" // 2.2.2.4 Arrotondamento, one per VAT breakdown
	itDiscount      = "ScontoMaggiorazione"         // 2.1.1.8 ScontoMaggiorazione, one per discount
)

// na is the value that the technical rules give to an identifier that UBL
// requires and FatturaPA lacks (the line of a transport document); Marshal
// writes it for the shipment identifier too, and Parse does not keep it.
const na = "NA"

// schemeCF is the identification scheme (ISO 6523 ICD) of the Italian tax
// code, the Codice Fiscale.
const schemeCF = "0210"

// ---- Reading ----

// it returns the national block of the invoice, created when needed.
func (r *reader) it() *invoice.Italy {
	if r.inv.Italy == nil {
		r.inv.Italy = new(invoice.Italy)
	}
	return r.inv.Italy
}

// transport returns the transport data of the national block, created when
// needed.
func (r *reader) transport() *invoice.Transport {
	it := r.it()
	if it.Transport == nil {
		it.Transport = new(invoice.Transport)
	}
	return it.Transport
}

// partyItaly returns the national block of p, created when needed.
func partyItaly(p *invoice.Party) *invoice.ItalyParty {
	if p.Italy == nil {
		p.Italy = new(invoice.ItalyParty)
	}
	return p.Italy
}

// paymentItaly returns the national block of pm, created when needed.
func paymentItaly(pm *invoice.PaymentMeans) *invoice.ItalyPayment {
	if pm.Italy == nil {
		pm.Italy = new(invoice.ItalyPayment)
	}
	return pm.Italy
}

// plainLeaf returns the trimmed text of n when n is the cbc element name
// without child elements and without attributes, but for a currencyID equal
// to the invoice currency, and false otherwise.
func (r *reader) plainLeaf(n *xmltree.Node, name string) (string, bool) {
	if n == nil || n.Space != NamespaceCBC || n.Name != name || len(n.Children) > 0 {
		return "", false
	}
	for _, a := range n.Attrs {
		if a.Prefix == "" && (a.Name != "currencyID" || strings.TrimSpace(a.Value) != r.cur) {
			return "", false
		}
	}
	return strings.TrimSpace(n.Text), true
}

// plainDecimal is plainLeaf for a decimal, unset when the element is empty.
func (r *reader) plainDecimal(n *xmltree.Node, name string) (invoice.Decimal, bool) {
	s, ok := r.plainLeaf(n, name)
	if !ok || s == "" {
		return invoice.Decimal{}, ok
	}
	d, err := invoice.ParseDecimal(s)
	return d, err == nil
}

// single reports whether n has no attribute and a single child element, of
// the given local name.
func single(n *xmltree.Node, name string) bool {
	return len(n.Attrs) == 0 && len(n.Children) == 1 && n.Children[0].Name == name
}

// italianExtensions reads the extensions of EXT-IT of n, the
// ext:UBLExtensions of the document, once the rest of the document is read:
// an extension may complete a withholding or a VAT breakdown. The other
// extensions are kept, at position at of Invoice.Extensions, where
// ext:UBLExtensions was.
func (r *reader) italianExtensions(n *xmltree.Node, at int) {
	inv := r.inv
	from := len(inv.Extensions)
	r.enter(n)
	for _, c := range n.Children {
		if !r.italianExtension(c) {
			r.unread(c)
		}
	}
	r.leave()
	if kept := inv.Extensions[from:]; len(kept) > 0 && at < from {
		kept = slices.Clone(kept)
		inv.Extensions = slices.Insert(inv.Extensions[:from], at, kept...)
	}
}

// italianExtension reads the ext:UBLExtension n when it is an extension of
// EXT-IT of the form the technical rules give, whose value has a place in
// the model, and reports whether it did.
func (r *reader) italianExtension(n *xmltree.Node) bool {
	if n.Space != NamespaceEXT || n.Name != "UBLExtension" || len(n.Attrs) > 0 || len(n.Children) != 2 {
		return false
	}
	uri, content := n.Children[0], n.Children[1]
	if uri.Space != NamespaceEXT || uri.Name != "ExtensionURI" || len(uri.Attrs) > 0 || len(uri.Children) > 0 ||
		content.Space != NamespaceEXT || content.Name != "ExtensionContent" || len(content.Attrs) > 0 || len(content.Children) == 0 {
		return false
	}
	name, ok := strings.CutPrefix(strings.TrimSpace(uri.Text), itURN)
	if !ok {
		return false
	}
	index := 1
	if s, i, found := strings.Cut(name, "::"); found {
		k, err := strconv.Atoi(i)
		if err != nil || k < 1 {
			return false
		}
		name, index = s, k
	}
	if name == itDiscount {
		return r.italianDiscounts(content.Children)
	}
	if len(content.Children) != 1 {
		return false
	}
	v := content.Children[0]
	it := r.inv.Italy // nil until a value is read into it
	set := func(field func(*invoice.Italy) *string, s string, ok bool) bool {
		if !ok || s == "" || index != 1 || it != nil && *field(it) != "" {
			return false
		}
		*field(r.it()) = s
		return true
	}
	switch name {
	case itIssuingSystem:
		s, ok := r.plainLeaf(v, "Description")
		return set(func(it *invoice.Italy) *string { return &it.IssuingSystem }, s, ok)
	case itIssuer:
		s, ok := r.plainLeaf(v, "TypeCode")
		return set(func(it *invoice.Italy) *string { return &it.Issuer }, s, ok)
	case itDocumentType:
		s, ok := r.plainLeaf(v, "TypeCode")
		return set(func(it *invoice.Italy) *string { return &it.DocumentType }, s, ok)
	case itWithholding:
		s, ok := r.plainLeaf(v, "TypeCode")
		if !ok || s == "" || it == nil || index > len(it.Withholdings) || it.Withholdings[index-1].Type != "" {
			return false
		}
		it.Withholdings[index-1].Type = s
	case itArt73:
		if s, ok := r.plainLeaf(v, "TypeCode"); !ok || s != "SI" || index != 1 || it != nil && it.Art73 {
			return false
		}
		r.it().Art73 = true
	case itRounding:
		d, ok := r.plainDecimal(v, "Amount")
		if !ok || !d.IsSet() || index > len(r.inv.VATBreakdown) {
			return false
		}
		vb := &r.inv.VATBreakdown[index-1]
		if vb.Italy != nil && vb.Italy.Rounding.IsSet() {
			return false
		}
		if vb.Italy == nil {
			vb.Italy = new(invoice.ItalyVAT)
		}
		vb.Italy.Rounding = d
	default:
		return false
	}
	return true
}

// italianDiscounts reads the content of an extension ScontoMaggiorazione:
// a discount (SC) or a surcharge (MG) on the document total for each
// cac:AllowanceCharge, with its percentage and its amount.
func (r *reader) italianDiscounts(charges []*xmltree.Node) bool {
	var out []invoice.Discount
	for _, c := range charges {
		if c.Space != NamespaceCAC || c.Name != "AllowanceCharge" || len(c.Attrs) > 0 {
			return false
		}
		var d invoice.Discount
		var pctDone, amtDone bool
		for _, k := range c.Children {
			var ok bool
			switch {
			case k.Name == "ChargeIndicator" && d.Type == "":
				var s string
				s, ok = r.plainLeaf(k, "ChargeIndicator")
				switch s {
				case "true", "1":
					d.Type = "MG"
				case "false", "0":
					d.Type = "SC"
				default:
					ok = false
				}
			case k.Name == "MultiplierFactorNumeric" && !pctDone:
				pctDone = true
				d.Percentage, ok = r.plainDecimal(k, "MultiplierFactorNumeric")
			case k.Name == "Amount" && !amtDone:
				amtDone = true
				d.Amount, ok = r.plainDecimal(k, "Amount")
			}
			if !ok {
				return false
			}
		}
		if d.Type == "" {
			return false
		}
		out = append(out, d)
	}
	it := r.it()
	it.Discounts = append(it.Discounts, out...)
	return true
}

// mainInvoice reads the cac:AdditionalDocumentReference n of a
// cac:BillingReference, the reference to the main invoice (FatturaPrincipale),
// when it has the form the technical rules give: an identifier and a date.
func (r *reader) mainInvoice(n *xmltree.Node) bool {
	id, date := child(n, "ID"), child(n, "IssueDate")
	if len(n.Attrs) > 0 || len(n.Children) != 2 || id == nil || date == nil || r.inv.Italy != nil && r.inv.Italy.MainInvoice != nil {
		return false
	}
	ref := new(invoice.DocumentReference)
	r.enter(n)
	ref.ID, ref.IssueDate = r.text(id), r.date(date)
	r.leave()
	r.it().MainInvoice = ref
	return true
}

// despatchDocument reads a cac:DespatchDocumentReference: its identifier
// is the despatch advice reference (BT-16) and, with its date, a transport
// document (DatiDDT). A reference without date, or without EXT-IT, is read
// as the binding of EN 16931 reads it.
func (r *reader) despatchDocument(n *xmltree.Node) {
	inv := r.inv
	if !r.extIT || child(n, "IssueDate") == nil {
		r.docID(n, &inv.DespatchAdviceReference)
		return
	}
	var number string
	var date invoice.Date
	var idDone, dateDone bool
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			number = r.text(c)
		case c.Name == "IssueDate" && !dateDone:
			dateDone = true
			date = r.date(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if inv.DespatchAdviceReference == "" {
		inv.DespatchAdviceReference = number
	}
	r.transportDocument(number, date, "")
}

// transportDocument records the transport document number of date, for
// the invoice line line when it is not empty: the lines of a document
// already recorded are gathered.
func (r *reader) transportDocument(number string, date invoice.Date, line string) {
	it := r.it()
	for i := range it.TransportDocuments {
		if d := &it.TransportDocuments[i]; d.Number == number && d.Date == date {
			if line != "" {
				d.Lines = append(d.Lines, line)
			}
			return
		}
	}
	d := invoice.TransportDocument{Number: number, Date: date}
	if line != "" {
		d.Lines = []string{line}
	}
	it.TransportDocuments = append(it.TransportDocuments, d)
}

// establishment reads the cac:AgentParty n of the seller, the address of its
// permanent establishment in Italy (StabileOrganizzazione).
func (r *reader) establishment(n *xmltree.Node, p *invoice.Party) bool {
	if !single(n, "PostalAddress") || p.Italy != nil && p.Italy.Establishment != nil {
		return false
	}
	r.enter(n)
	a := r.address(n.Children[0])
	r.leave()
	partyItaly(p).Establishment = &a
	return true
}

// intermediary reads the cac:ServiceProviderParty n of the seller, the
// third party that issues the invoice on its behalf
// (TerzoIntermediarioOSoggettoEmittente).
func (r *reader) intermediary(n *xmltree.Node) bool {
	party := child(n, "Party")
	if party == nil || r.inv.Italy != nil && r.inv.Italy.Intermediary != nil {
		return false
	}
	p := new(invoice.Party)
	r.enter(n)
	for _, c := range n.Children {
		if c == party {
			r.italianParty(c, p, nil)
			continue
		}
		r.unread(c)
	}
	r.leave()
	r.it().Intermediary = p
	return true
}

// italianParty reads n, the party of the third-party intermediary or of the
// carrier, as package fatturapa reads them: its tax code (Codice Fiscale,
// with the scheme 0210 or the prefix "CF:") is its legal registration
// identifier with the scheme 0210, its EORI code ("EORI:" followed by the
// code) a party identifier, and the name of a natural person, given in
// cac:Person when the party has no cac:PartyName, is written
// "Nome#Cognome:given#family" (see invoice.ItalianName). licence, when not
// nil, receives the driving licence number of the carrier. The extensions
// are kept on the value being read, the party having no place for them in
// Invoice.Unmodeled.
func (r *reader) italianParty(n *xmltree.Node, p *invoice.Party, licence *string) {
	r.enter(n)
	named := child(n, "PartyName") != nil
	var nameDone, personDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "PartyIdentification":
			r.italianID(c, p)
		case c.Name == "PartyName" && !nameDone:
			nameDone = true
			r.enter(c)
			for _, k := range c.Children {
				if k.Name == "Name" && p.Name == "" {
					p.Name = r.text(k)
					continue
				}
				r.unread(k)
			}
			r.leave()
		case c.Name == "PartyTaxScheme":
			r.partyTaxScheme(c, p, false)
		case c.Name == "Person" && !personDone:
			personDone = true
			r.person(c, p, named, licence)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// italianID reads a cac:PartyIdentification of an Italian party (see
// italianParty).
func (r *reader) italianID(n *xmltree.Node, p *invoice.Party) {
	r.enter(n)
	done := false
	for _, c := range n.Children {
		if c.Name != "ID" || done {
			r.unread(c)
			continue
		}
		done = true
		id := r.ident(c)
		code, cf := strings.CutPrefix(id.ID, "CF:")
		switch {
		case id.Scheme == schemeCF && p.LegalRegistration.IsZero():
			p.LegalRegistration = id
		case id.Scheme == "" && cf && p.LegalRegistration.IsZero():
			p.LegalRegistration = invoice.Identifier{ID: code, Scheme: schemeCF}
		default:
			p.IDs = append(p.IDs, id)
		}
	}
	r.leave()
}

// person reads the cac:Person of an Italian party: its names are the name
// of the party when it has none (named is false), and its identity document
// of type 40 the driving licence number when licence is not nil.
func (r *reader) person(n *xmltree.Node, p *invoice.Party, named bool, licence *string) {
	var given, family *xmltree.Node
	docDone := false
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "FirstName" && given == nil && !named:
			given = c
		case c.Name == "FamilyName" && family == nil && !named:
			family = c
		case c.Name == "IdentityDocumentReference" && licence != nil && !docDone && r.licence(c, licence):
			docDone = true
		default:
			r.unread(c)
		}
	}
	if given != nil || family != nil {
		p.Name = invoice.ItalianName(r.text(given), r.text(family))
	}
	r.leave()
}

// licence reads the driving licence number of the carrier (NumeroLicenzaGuida)
// from n, a cac:IdentityDocumentReference of type 40.
func (r *reader) licence(n *xmltree.Node, dst *string) bool {
	id, code := child(n, "ID"), child(n, "DocumentTypeCode")
	if *dst != "" || len(n.Attrs) > 0 || len(n.Children) != 2 || id == nil || code == nil {
		return false
	}
	if s, ok := r.plainLeaf(code, "DocumentTypeCode"); !ok || s != "40" {
		return false
	}
	r.enter(n)
	*dst = r.text(id)
	r.text(code)
	r.leave()
	return true
}

// carrier reads the cac:CarrierParty of the delivery.
func (r *reader) carrier(n *xmltree.Node) {
	t := r.transport()
	t.Carrier = new(invoice.Party)
	r.italianParty(n, t.Carrier, &t.CarrierLicence)
}

// despatch reads the cac:Despatch of the delivery: the reason of the
// transport (CausaleTrasporto), the pickup date and time (DataOraRitiro,
// midnight without time) and the start date of the transport
// (DataInizioTrasporto).
func (r *reader) despatch(n *xmltree.Node) {
	t := r.transport()
	hasDay := child(n, "RequestedDespatchDate") != nil
	var day, hour string
	var dayDone, hourDone bool
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "Instructions" && t.Reason == "":
			t.Reason = r.text(c)
		case c.Name == "RequestedDespatchDate" && !dayDone:
			dayDone = true
			if d := r.date(c); !d.IsZero() {
				day = d.String()
			}
		case c.Name == "RequestedDespatchTime" && hasDay && !hourDone:
			hourDone = true
			hour = r.text(c)
		case c.Name == "ActualDespatchDate" && t.StartDate.IsZero():
			t.StartDate = r.date(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if day != "" {
		if hour == "" {
			hour = "00:00:00"
		}
		t.PickupTime = day + "T" + hour
	}
}

// shipment reads the cac:Shipment of the delivery: the gross and net
// weights with their unit, the number of packages, the description of the
// goods and the means of transport. Its identifier, which FatturaPA lacks,
// is kept as an extension unless it is na.
func (r *reader) shipment(n *xmltree.Node) {
	t := r.transport()
	var idDone, grossDone, netDone, goodsDone, stageDone bool
	sameUnit := func(c *xmltree.Node) bool { return t.WeightUnit == "" || attr(c, "unitCode") == t.WeightUnit }
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone && len(c.Attrs) == 0 && len(c.Children) == 0 && textOf(c) == na:
			idDone = true
		case c.Name == "GrossWeightMeasure" && !grossDone && sameUnit(c):
			grossDone = true
			var unit string
			t.GrossWeight, unit = r.quantity(c)
			if unit != "" {
				t.WeightUnit = unit
			}
		case c.Name == "NetWeightMeasure" && !netDone && sameUnit(c):
			netDone = true
			var unit string
			t.NetWeight, unit = r.quantity(c)
			if unit != "" {
				t.WeightUnit = unit
			}
		case c.Name == "TotalTransportHandlingUnitQuantity" && t.Packages == "":
			t.Packages = r.text(c)
		case c.Name == "GoodsItem" && !goodsDone && single(c, "Description"):
			goodsDone = true
			r.enter(c)
			t.Description = r.text(c.Children[0])
			r.leave()
		case c.Name == "ShipmentStage" && !stageDone && single(c, "TransportMeansTypeCode"):
			stageDone = true
			r.enter(c)
			t.Means = r.text(c.Children[0])
			r.leave()
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// deliveryTerms reads the cac:DeliveryTerms n of the document, whose
// identifier is the Incoterms code of the delivery (TipoResa).
func (r *reader) deliveryTerms(n *xmltree.Node) bool {
	if !single(n, "ID") || r.inv.Italy != nil && r.inv.Italy.Transport != nil && r.inv.Italy.Transport.DeliveryTerms != "" {
		return false
	}
	r.enter(n)
	r.transport().DeliveryTerms = r.text(n.Children[0])
	r.leave()
	return true
}

// withholdingTaxTotal reads a cac:WithholdingTaxTotal: each tax subtotal is
// a withholding (DatiRitenuta) whose type the extensions TipoRitenuta give.
// The total is implied when it is the sum of the withholdings.
func (r *reader) withholdingTaxTotal(n *xmltree.Node) {
	it := r.it()
	first := len(it.Withholdings)
	var total *xmltree.Node
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "TaxAmount" && total == nil:
			total = c
		case c.Name == "TaxSubtotal":
			it.Withholdings = append(it.Withholdings, r.withholding(c))
		default:
			r.unread(c)
		}
	}
	if total != nil {
		sum := invoice.NewDecimal(0, 2)
		for _, w := range it.Withholdings[first:] {
			sum = sum.Add(w.Amount)
		}
		if d, ok := r.plainDecimal(total, "TaxAmount"); ok && d.IsSet() && d.Equal(sum) {
			r.decimal(total)
		} else {
			r.unread(total)
		}
	}
	r.leave()
}

// withholding reads a cac:TaxSubtotal of a cac:WithholdingTaxTotal: the
// amount, and the rate and the reason of payment (CausalePagamento) of its
// category S in the scheme SWT.
func (r *reader) withholding(n *xmltree.Node) invoice.Withholding {
	var w invoice.Withholding
	var amtDone, catDone bool
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "TaxAmount" && !amtDone:
			amtDone = true
			w.Amount = r.decimal(c)
		case c.Name == "TaxCategory" && !catDone:
			catDone = true
			r.enter(c)
			var idDone, rateDone, schemeDone bool
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && !idDone && textOf(k) == "S":
					idDone = true
					r.text(k)
				case k.Name == "Percent" && !rateDone:
					rateDone = true
					w.Rate = r.decimal(k)
				case k.Name == "TaxScheme" && !schemeDone:
					schemeDone = true
					r.enter(k)
					var swtDone bool
					for _, s := range k.Children {
						switch {
						case s.Name == "ID" && !swtDone && textOf(s) == "SWT":
							swtDone = true
							r.text(s)
						case s.Name == "TaxTypeCode" && w.Reason == "":
							w.Reason = r.text(s)
						default:
							r.unread(s)
						}
					}
					r.leave()
				default:
					r.unread(k)
				}
			}
			r.leave()
		default:
			r.unread(c)
		}
	}
	r.leave()
	return w
}

// orderLine reads the cac:OrderLineReference of the invoice line l: its
// line identifier is the referenced purchase order line (BT-132) and, with
// the order that EXT-IT adds, the item of a purchase order (NumItem) whose
// lines include l. A line identifier na stands for no line.
func (r *reader) orderLine(n *xmltree.Node, l *invoice.Line) {
	var lineID, order *xmltree.Node
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "LineID" && lineID == nil:
			lineID = c
		case c.Name == "OrderReference" && order == nil && r.extIT:
			order = c
		default:
			r.unread(c)
		}
	}
	id := r.text(lineID)
	if order == nil {
		l.OrderLineReference = id
		r.leave()
		return
	}
	var ref invoice.RelatedDocument
	var idDone, dateDone bool
	r.enter(order)
	for _, c := range order.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			ref.ID = r.text(c)
		case c.Name == "IssueDate" && !dateDone:
			dateDone = true
			ref.Date = r.date(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	r.leave()
	if id != na {
		l.OrderLineReference, ref.Item = id, id
	}
	it := r.it()
	for i := range it.PurchaseOrders {
		if d := &it.PurchaseOrders[i]; len(d.Lines) > 0 && d.ID == ref.ID && d.Date == ref.Date && d.Item == ref.Item {
			d.Lines = append(d.Lines, l.ID)
			return
		}
	}
	ref.Lines = []string{l.ID}
	it.PurchaseOrders = append(it.PurchaseOrders, ref)
}

// despatchLine reads a cac:DespatchLineReference of the invoice line l: the
// transport document it refers to includes l.
func (r *reader) despatchLine(n *xmltree.Node, l *invoice.Line) bool {
	doc := child(n, "DocumentReference")
	if doc == nil || len(n.Attrs) > 0 {
		return false
	}
	var number string
	var date invoice.Date
	lineDone := false
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "LineID" && !lineDone && len(c.Attrs) == 0 && len(c.Children) == 0 && textOf(c) == na:
			lineDone = true
		case c == doc:
			var idDone, dateDone bool
			r.enter(c)
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && !idDone:
					idDone = true
					number = r.text(k)
				case k.Name == "IssueDate" && !dateDone:
					dateDone = true
					date = r.date(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		default:
			r.unread(c)
		}
	}
	r.leave()
	r.transportDocument(number, date, l.ID)
	return true
}

// earlyPayment reads c, a child of a cac:PaymentTerms, when it is the
// discount for early payment, the penalty for late payment or the date of
// either, which go to the first payment means, and reports whether it did.
func (r *reader) earlyPayment(c *xmltree.Node) bool {
	if !r.extIT || len(r.inv.PaymentMeans) == 0 {
		return false
	}
	pm := &r.inv.PaymentMeans[0]
	var cur invoice.ItalyPayment
	if pm.Italy != nil {
		cur = *pm.Italy
	}
	switch {
	case c.Name == "SettlementDiscountAmount" && !cur.EarlyDiscount.IsSet():
		paymentItaly(pm).EarlyDiscount = r.decimal(c)
	case c.Name == "PenaltyAmount" && !cur.LatePenalty.IsSet():
		paymentItaly(pm).LatePenalty = r.decimal(c)
	case c.Name == "SettlementPeriod" && cur.EarlyDiscountDate.IsZero() && single(c, "EndDate"):
		r.enter(c)
		paymentItaly(pm).EarlyDiscountDate = r.date(c.Children[0])
		r.leave()
	case c.Name == "PenaltyPeriod" && cur.LatePenaltyDate.IsZero() && single(c, "StartDate"):
		r.enter(c)
		paymentItaly(pm).LatePenaltyDate = r.date(c.Children[0])
		r.leave()
	default:
		return false
	}
	return true
}

// finishItaly completes the national block once the document is read: the
// date of the purchase order (BT-13) makes it an order of the national
// block, a document type implied by the type code (BT-3) is not kept, and
// an empty block is removed.
func (r *reader) finishItaly() {
	inv := r.inv
	if !r.orderDate.IsZero() {
		it := r.it()
		it.PurchaseOrders = slices.Insert(it.PurchaseOrders, 0, invoice.RelatedDocument{ID: inv.PurchaseOrderReference, Date: r.orderDate})
	}
	it := inv.Italy
	if it == nil {
		return
	}
	if it.DocumentType != "" && it.DocumentType == invoice.DocumentTypeOfTypeCode(inv.TypeCode) {
		it.DocumentType = ""
	}
	if reflect.ValueOf(it).Elem().IsZero() {
		inv.Italy = nil
	}
}

// ---- Writing ----

// carry records that the national data term ("Italy.Withholdings", see
// invoice.Invoice.NationalData) is written in full.
func (w *writer) carry(term string) {
	if w.carried == nil {
		w.carried = map[string]bool{}
	}
	w.carried["EXT "+term] = true
}

// prepareItaly finds, before writing, where the national data of EXT-IT
// goes: the purchase order and the transport document of the document, and
// those of each invoice line.
func (w *writer) prepareItaly(inv *invoice.Invoice) {
	it := inv.Italy
	w.it, w.orderAt, w.ddtAt = it, -1, -1
	if it == nil {
		return
	}
	lines := map[string]*invoice.Line{}
	for i := range inv.Lines {
		if id := inv.Lines[i].ID; lines[id] == nil {
			lines[id] = &inv.Lines[i]
		}
	}

	// The purchase order of the document is the order without line that has
	// the identifier BT-13, or the first one when BT-13 is empty.
	for i := range it.PurchaseOrders {
		if d := &it.PurchaseOrders[i]; len(d.Lines) == 0 && (d.ID == inv.PurchaseOrderReference || inv.PurchaseOrderReference == "") {
			w.orderAt = i
			break
		}
	}
	all := true
	w.lineOrder = map[string]int{}
	for i := range it.PurchaseOrders {
		d := &it.PurchaseOrders[i]
		plain := d.OrderCode == "" && d.CUP == "" && d.CIG == ""
		if i == w.orderAt {
			all = all && plain && d.Item == ""
			continue
		}
		if len(d.Lines) == 0 || !plain {
			all = false
		}
		for _, id := range d.Lines {
			l := lines[id]
			if _, dup := w.lineOrder[id]; l == nil || dup {
				all = false
				continue
			}
			w.lineOrder[id] = i
			if d.Item != "" && l.OrderLineReference != "" && l.OrderLineReference != d.Item {
				all = false
			}
		}
	}
	if all {
		w.carry("Italy.PurchaseOrders")
	}

	// The transport document of the document is the one numbered BT-16,
	// preferably without line, or the first one without line when BT-16 is
	// empty.
	for pass := 0; pass < 2 && w.ddtAt < 0; pass++ {
		for i := range it.TransportDocuments {
			d := &it.TransportDocuments[i]
			if pass == 0 && len(d.Lines) == 0 && (d.Number == inv.DespatchAdviceReference || inv.DespatchAdviceReference == "") ||
				pass == 1 && d.Number == inv.DespatchAdviceReference && d.Number != "" {
				w.ddtAt = i
				break
			}
		}
	}
	all = true
	w.lineDDT = map[string][]int{}
	for i := range it.TransportDocuments {
		d := &it.TransportDocuments[i]
		if len(d.Lines) == 0 && i != w.ddtAt {
			all = false
		}
		for _, id := range d.Lines {
			if lines[id] == nil {
				all = false
				continue
			}
			w.lineDDT[id] = append(w.lineDDT[id], i)
		}
	}
	if all {
		w.carry("Italy.TransportDocuments")
	}
}

// headerOrder returns the purchase order reference (BT-13) and the date of
// the order of the document.
func (w *writer) headerOrder(inv *invoice.Invoice) (string, invoice.Date) {
	if w.orderAt < 0 {
		return inv.PurchaseOrderReference, invoice.Date{}
	}
	d := &w.it.PurchaseOrders[w.orderAt]
	return d.ID, d.Date
}

// headerDespatch returns the despatch advice reference (BT-16) and the date
// of the transport document of the document.
func (w *writer) headerDespatch(inv *invoice.Invoice) (string, invoice.Date) {
	if w.ddtAt < 0 {
		return inv.DespatchAdviceReference, invoice.Date{}
	}
	d := &w.it.TransportDocuments[w.ddtAt]
	return d.Number, d.Date
}

// hasItalianExtensions reports whether inv holds data that the extensions
// of EXT-IT carry.
func (w *writer) hasItalianExtensions(inv *invoice.Invoice) bool {
	for i := range inv.VATBreakdown {
		if it := inv.VATBreakdown[i].Italy; it != nil && it.Rounding.IsSet() {
			return true
		}
	}
	it := inv.Italy
	if it == nil {
		return false
	}
	for _, wh := range it.Withholdings {
		if wh.Type != "" && !w.credit {
			return true
		}
	}
	return it.IssuingSystem != "" || it.Issuer != "" || it.DocumentType != "" || it.Art73 || len(it.Discounts) > 0
}

// italianExtensions writes the ext:UBLExtensions of EXT-IT, in the order of
// appendix 4.2. A credit note has no withholdings (UBL 2.1 has no
// cac:WithholdingTaxTotal in a CreditNote), hence no type of withholding.
func (w *writer) italianExtensions(inv *invoice.Invoice) {
	ext := func(name string, index int, content func()) {
		uri := itURN + name
		if index > 0 {
			uri += "::" + strconv.Itoa(index)
		}
		w.start("ext:UBLExtension")
		w.leafReq("ext:ExtensionURI", uri)
		w.start("ext:ExtensionContent")
		content()
		w.end("ext:ExtensionContent")
		w.end("ext:UBLExtension")
	}
	w.start("ext:UBLExtensions")
	if it := inv.Italy; it != nil {
		code := func(name, value, term string) {
			if value != "" {
				ext(name, 0, func() { w.leafReq("cbc:TypeCode", value) })
				w.carry(term)
			}
		}
		if it.IssuingSystem != "" {
			ext(itIssuingSystem, 0, func() { w.leafReq("cbc:Description", it.IssuingSystem) })
			w.carry("Italy.IssuingSystem")
		}
		code(itIssuer, it.Issuer, "Italy.Issuer")
		code(itDocumentType, it.DocumentType, "Italy.DocumentType")
		for i, wh := range it.Withholdings {
			if wh.Type != "" && !w.credit {
				ext(itWithholding, ordinal(i), func() { w.leafReq("cbc:TypeCode", wh.Type) })
			}
		}
		if it.Art73 {
			ext(itArt73, 0, func() { w.leafReq("cbc:TypeCode", "SI") })
			w.carry("Italy.Art73")
		}
	}
	for i := range inv.VATBreakdown {
		if vi := inv.VATBreakdown[i].Italy; vi != nil && vi.Rounding.IsSet() {
			ext(itRounding, i+1, func() { w.amountReq("cbc:Amount", vi.Rounding, w.cur) })
			w.carry("VATBreakdown[" + strconv.Itoa(i) + "].Italy.Rounding")
		}
	}
	if it := inv.Italy; it != nil && len(it.Discounts) > 0 {
		all, k := true, 0
		for _, d := range it.Discounts {
			if d.Type != "SC" && d.Type != "MG" {
				all = false
				continue
			}
			ext(itDiscount, ordinal(k), func() {
				w.start("cac:AllowanceCharge")
				w.leafReq("cbc:ChargeIndicator", strconv.FormatBool(d.Type == "MG"))
				w.dec("cbc:MultiplierFactorNumeric", d.Percentage)
				w.amountReq("cbc:Amount", d.Amount, w.cur)
				w.end("cac:AllowanceCharge")
			})
			k++
		}
		if all {
			w.carry("Italy.Discounts")
		}
	}
	w.end("ext:UBLExtensions")
}

// ordinal returns the index written after the URI of the extension that
// carries the (i+1)th value of a list: none for the first one.
func ordinal(i int) int {
	if i == 0 {
		return 0
	}
	return i + 1
}

// mainInvoice writes the reference to the main invoice (FatturaPrincipale)
// in the first cac:BillingReference: BR-55 wants a preceding invoice
// reference (BT-25) in each, so that an invoice without one cannot carry it.
func (w *writer) mainInvoice() {
	m := w.it.MainInvoice
	w.start("cac:AdditionalDocumentReference")
	w.leafReq("cbc:ID", m.ID)
	w.date("cbc:IssueDate", m.IssueDate)
	w.end("cac:AdditionalDocumentReference")
	if m.TypeCode == "" && m.LineID == "" {
		w.carry("Italy.MainInvoice")
	}
}

// simpleParty reports whether p holds no data but its name, its VAT
// identifier, its tax code (legal registration identifier with the scheme
// 0210) and, when eori is set, EORI codes: what an Italian party of EXT-IT
// carries (see italianParty).
func simpleParty(p *invoice.Party, eori bool) bool {
	for _, id := range p.IDs {
		if !eori || id.Scheme != "" || !strings.HasPrefix(id.ID, "EORI:") {
			return false
		}
	}
	return (p.LegalRegistration.IsZero() || p.LegalRegistration.Scheme == schemeCF) &&
		p.Description == "" && p.TradingName == "" && p.Contact == nil && p.Address.IsZero() &&
		p.ElectronicAddress.IsZero() && p.TaxRegistrationID == "" && p.Italy == nil && len(p.Extensions) == 0
}

// italianParty writes p, the third-party intermediary or the carrier, as an
// Italian party of EXT-IT (see the reader's italianParty); licence is the
// driving licence number of the carrier. The VAT identifier is written even
// when empty: BR-IT-DE-023 and appendix 4.2 require it.
func (w *writer) italianParty(name string, p *invoice.Party, eori bool, licence string) {
	w.start(name)
	if id := p.LegalRegistration; id.ID != "" && id.Scheme == schemeCF {
		w.start("cac:PartyIdentification")
		w.ident("cbc:ID", id)
		w.end("cac:PartyIdentification")
	}
	if eori {
		for _, id := range p.IDs {
			if id.Scheme == "" && strings.HasPrefix(id.ID, "EORI:") {
				w.start("cac:PartyIdentification")
				w.leafReq("cbc:ID", id.ID)
				w.end("cac:PartyIdentification")
			}
		}
	}
	given, family, person := invoice.SplitItalianName(p.Name)
	if !person && p.Name != "" {
		w.start("cac:PartyName")
		w.leafReq("cbc:Name", p.Name)
		w.end("cac:PartyName")
	}
	w.taxScheme(p.VATID, "VAT", "")
	if person || licence != "" {
		w.start("cac:Person")
		if person {
			w.leafReq("cbc:FirstName", given)
			w.leafReq("cbc:FamilyName", family)
		}
		if licence != "" {
			w.start("cac:IdentityDocumentReference")
			w.leafReq("cbc:ID", licence)
			w.leafReq("cbc:DocumentTypeCode", "40")
			w.end("cac:IdentityDocumentReference")
		}
		w.end("cac:Person")
	}
	w.end(name)
}

// sellerItaly writes the permanent establishment of the seller and the
// third-party intermediary, the last children of the seller party.
func (w *writer) sellerItaly(p *invoice.Party) {
	if pit := p.Italy; pit != nil && pit.Establishment != nil {
		w.start("cac:AgentParty")
		w.address("cac:PostalAddress", pit.Establishment)
		w.end("cac:AgentParty")
		w.carry("Seller.Italy.Establishment")
	}
	if it := w.it; it != nil && it.Intermediary != nil {
		w.start("cac:ServiceProviderParty")
		w.italianParty("cac:Party", it.Intermediary, true, "")
		w.end("cac:ServiceProviderParty")
		if simpleParty(it.Intermediary, true) {
			w.carry("Italy.Intermediary")
		}
	}
}

// deliveryTransport returns the transport data that cac:Delivery carries,
// or nil.
func (w *writer) deliveryTransport() *invoice.Transport {
	if w.it == nil || w.it.Transport == nil {
		return nil
	}
	t := w.it.Transport
	if t.Carrier == nil && t.CarrierLicence == "" && t.Reason == "" && t.PickupTime == "" && t.StartDate.IsZero() &&
		t.Description == "" && t.Means == "" && t.Packages == "" && !t.GrossWeight.IsSet() && !t.NetWeight.IsSet() {
		return nil
	}
	return t
}

// carrier writes the carrier of the delivery.
func (w *writer) carrier(t *invoice.Transport) {
	if t.Carrier == nil && t.CarrierLicence == "" {
		return
	}
	c := t.Carrier
	if c == nil {
		c = new(invoice.Party)
	}
	w.italianParty("cac:CarrierParty", c, false, t.CarrierLicence)
}

// despatch writes the cac:Despatch and the cac:Shipment of the delivery.
func (w *writer) despatch(t *invoice.Transport) {
	day, hour, _ := strings.Cut(t.PickupTime, "T")
	if t.Reason != "" || day != "" || !t.StartDate.IsZero() {
		w.start("cac:Despatch")
		w.leaf("cbc:RequestedDespatchDate", day)
		if day != "" && hour != "00:00:00" {
			w.leaf("cbc:RequestedDespatchTime", hour)
		}
		w.date("cbc:ActualDespatchDate", t.StartDate)
		w.leaf("cbc:Instructions", t.Reason)
		w.end("cac:Despatch")
	}
	if t.GrossWeight.IsSet() || t.NetWeight.IsSet() || t.Packages != "" || t.Description != "" || t.Means != "" {
		w.start("cac:Shipment")
		w.leafReq("cbc:ID", na)
		w.weight("cbc:GrossWeightMeasure", t.GrossWeight, t.WeightUnit)
		w.weight("cbc:NetWeightMeasure", t.NetWeight, t.WeightUnit)
		w.leaf("cbc:TotalTransportHandlingUnitQuantity", t.Packages)
		if t.Description != "" {
			w.start("cac:GoodsItem")
			w.leafReq("cbc:Description", t.Description)
			w.end("cac:GoodsItem")
		}
		if t.Means != "" {
			w.start("cac:ShipmentStage")
			w.leafReq("cbc:TransportMeansTypeCode", t.Means)
			w.end("cac:ShipmentStage")
		}
		w.end("cac:Shipment")
	}
}

// weight writes a weight with its unit, required by the schema: written
// empty when unknown, so that a validator reports it.
func (w *writer) weight(name string, d invoice.Decimal, unit string) {
	if !d.IsSet() {
		return
	}
	w.open(name)
	w.attr("unitCode", unit)
	w.b = append(w.b, '>')
	w.b = d.Append(w.b)
	w.b = append(w.b, '<', '/')
	w.b = append(w.b, name...)
	w.b = append(w.b, '>')
}

// transportCarried reports whether UBL carries all the transport data t.
func transportCarried(t *invoice.Transport) bool {
	return t.DeliveryTime == "" && (t.Carrier == nil || simpleParty(t.Carrier, false)) &&
		(t.WeightUnit == "" || t.GrossWeight.IsSet() || t.NetWeight.IsSet()) &&
		(t.PickupTime == "" || strings.Contains(t.PickupTime, "T"))
}

// deliveryTermsIT writes the cac:DeliveryTerms of the document, the
// Incoterms code of the delivery (TipoResa), and records the transport data
// as carried.
func (w *writer) deliveryTermsIT() {
	if w.it == nil || w.it.Transport == nil {
		return
	}
	t := w.it.Transport
	if t.DeliveryTerms != "" {
		w.start("cac:DeliveryTerms")
		w.leafReq("cbc:ID", t.DeliveryTerms)
		w.end("cac:DeliveryTerms")
	}
	if transportCarried(t) {
		w.carry("Italy.Transport")
	}
}

// earlyPayment writes the discount for early payment and the penalty for
// late payment of the first payment means, with their dates, in the
// cac:PaymentTerms being written.
func (w *writer) earlyPayment(pit *invoice.ItalyPayment) {
	w.amount("cbc:SettlementDiscountAmount", pit.EarlyDiscount)
	w.amount("cbc:PenaltyAmount", pit.LatePenalty)
	if !pit.EarlyDiscountDate.IsZero() {
		w.start("cac:SettlementPeriod")
		w.date("cbc:EndDate", pit.EarlyDiscountDate)
		w.end("cac:SettlementPeriod")
	}
	if !pit.LatePenaltyDate.IsZero() {
		w.start("cac:PenaltyPeriod")
		w.date("cbc:StartDate", pit.LatePenaltyDate)
		w.end("cac:PenaltyPeriod")
	}
	for _, f := range []string{"EarlyDiscount", "EarlyDiscountDate", "LatePenalty", "LatePenaltyDate"} {
		w.carry("PaymentMeans[0].Italy." + f)
	}
}

// earlyPaymentOf returns the national block of the first payment means of
// inv when it holds a discount for early payment or a penalty for late
// payment, and nil otherwise.
func (w *writer) earlyPaymentOf(inv *invoice.Invoice) *invoice.ItalyPayment {
	if !w.italy || len(inv.PaymentMeans) == 0 {
		return nil
	}
	pit := inv.PaymentMeans[0].Italy
	if pit == nil || !pit.EarlyDiscount.IsSet() && !pit.LatePenalty.IsSet() && pit.EarlyDiscountDate.IsZero() && pit.LatePenaltyDate.IsZero() {
		return nil
	}
	return pit
}

// withholdings writes the withholdings (DatiRitenuta) of an invoice, two by
// cac:WithholdingTaxTotal as appendix 4.2 allows; their types are written
// in the extensions TipoRitenuta.
func (w *writer) withholdings(ws []invoice.Withholding) {
	for i := 0; i < len(ws); i += 2 {
		group := ws[i:min(i+2, len(ws))]
		total := invoice.NewDecimal(0, 2)
		for _, wh := range group {
			total = total.Add(wh.Amount)
		}
		w.start("cac:WithholdingTaxTotal")
		w.amountReq("cbc:TaxAmount", total, w.cur)
		for _, wh := range group {
			w.start("cac:TaxSubtotal")
			w.amountReq("cbc:TaxAmount", wh.Amount, w.cur)
			w.start("cac:TaxCategory")
			w.leafReq("cbc:ID", "S")
			w.dec("cbc:Percent", wh.Rate)
			w.start("cac:TaxScheme")
			w.leafReq("cbc:ID", "SWT")
			w.leafReq("cbc:TaxTypeCode", wh.Reason)
			w.end("cac:TaxScheme")
			w.end("cac:TaxCategory")
			w.end("cac:TaxSubtotal")
		}
		w.end("cac:WithholdingTaxTotal")
	}
	w.carry("Italy.Withholdings")
}

// lineReferences writes the references of the invoice line l: the purchase
// order line (BT-132) with, under EXT-IT, the order that includes the line,
// and the transport documents that include it.
func (w *writer) lineReferences(l *invoice.Line) {
	order := -1
	if w.italy {
		if i, ok := w.lineOrder[l.ID]; ok {
			order = i
		}
	}
	if order >= 0 {
		d := &w.it.PurchaseOrders[order]
		id := l.OrderLineReference
		if id == "" {
			id = d.Item
		}
		if id == "" {
			id = na
		}
		w.start("cac:OrderLineReference")
		w.leafReq("cbc:LineID", id)
		w.start("cac:OrderReference")
		w.leafReq("cbc:ID", d.ID)
		w.date("cbc:IssueDate", d.Date)
		w.end("cac:OrderReference")
		w.end("cac:OrderLineReference")
	} else if l.OrderLineReference != "" {
		w.start("cac:OrderLineReference")
		w.leafReq("cbc:LineID", l.OrderLineReference)
		w.end("cac:OrderLineReference")
	}
	if !w.italy {
		return
	}
	for _, i := range w.lineDDT[l.ID] {
		d := &w.it.TransportDocuments[i]
		w.start("cac:DespatchLineReference")
		w.leafReq("cbc:LineID", na)
		w.start("cac:DocumentReference")
		w.leafReq("cbc:ID", d.Number)
		w.date("cbc:IssueDate", d.Date)
		w.end("cac:DocumentReference")
		w.end("cac:DespatchLineReference")
	}
}
