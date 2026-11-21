package cii

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// rootName is the local name of the root element.
const rootName = "rsm:CrossIndustryInvoice"

// Namespaces of the CII D16B syntax.
const (
	NamespaceRSM = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	NamespaceRAM = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	NamespaceUDT = "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
	NamespaceQDT = "urn:un:unece:uncefact:data:standard:QualifiedDataType:100"
)

const (
	xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>`
	rootStart      = `<rsm:CrossIndustryInvoice xmlns:rsm="` + NamespaceRSM +
		`" xmlns:qdt="` + NamespaceQDT + `" xmlns:ram="` + NamespaceRAM +
		`" xmlns:udt="` + NamespaceUDT + `">`
)

// MarshalOptions tune Marshal. A nil *MarshalOptions is the zero value.
type MarshalOptions struct {
	// Profile is the target profile. ProfileUnknown means inv.Profile, or
	// EN 16931 when that is unknown too.
	Profile invoice.Profile
	// Indent writes one element per line, indented with two spaces.
	Indent bool
	// Strict makes Marshal fail with a *LossError when the invoice holds data
	// that the target profile cannot carry (invoice lines in BASIC WL,
	// EXTENDED fields or extensions in EN 16931…). Without Strict such data
	// is omitted.
	Strict bool
}

// LossError is returned by Marshal in strict mode when the invoice holds data
// that the target profile cannot carry.
type LossError struct {
	Profile invoice.Profile
	// Terms lists the business terms that would be lost, in document order:
	// "BT-33", "BG-25"…, or "EXT <Field>" for data that only the EXTENDED
	// profile carries and "EXT <Role>.<Field>" for party data. An extension
	// (see invoice.Extension) is reported as "EXT " followed by its location
	// from the root element:
	// "EXT SupplyChainTradeTransaction/ApplicableHeaderTradeAgreement/QuotationReferencedDocument".
	// The data of the national blocks that CII does not carry (the
	// FatturaPA data of invoice.Italy…) follows, as listed by
	// invoice.Invoice.NationalData: "EXT Italy.Withholdings".
	Terms []string
}

// Error gives the target profile and lists the business terms that would be
// lost.
func (e *LossError) Error() string {
	return "cii: profile " + e.Profile.String() + " cannot carry " +
		strconv.Itoa(len(e.Terms)) + " business terms: " + strings.Join(e.Terms, ", ")
}

// Marshal writes inv as a CII document for the profile chosen in opts.
//
// Only the elements that the target profile allows are written, in the order
// of the profile's XML schema (Factur-X 1.09.2; XRECHNUNG and PEPPOL use the
// EN 16931 schema and EXTENDED-CTC-FR the EXTENDED one). An XRECHNUNG invoice
// keeps a single preceding invoice reference (BG-3), the CII D16B schema it is
// validated against allowing only one; the others are lost. The specification
// identifier BT-24 is inv.SpecificationID when it identifies the target
// profile (this keeps, for example, the XRechnung version), the target's URN
// otherwise: a ZUGFeRD 1.0 identifier, which only a CrossIndustryDocument
// carries, is replaced.
//
// The Dutch G-account extension (see invoice.GAccountExtension) is defined
// for UBL only, the Dutch Peppol Authority publishing no such extension of
// NLCIUS-CII: it is removed from BT-24, and the payment means identifiers
// (PaymentMeans.ID) and payment means references (PaymentTerms.PaymentMeansID)
// that it adds are reported as lost, in every profile. The amounts of the
// payment terms (PaymentTerms.PartialAmount) are written in EXTENDED, like
// the payment terms after the first.
//
// Mandatory elements of the target schema are always written, empty when the
// invoice has no value for them, so that a validator reports the missing
// value. Amounts are written with the number of decimals they hold.
//
// The extensions of the invoice (see invoice.Extension) are put back at the
// position that the target schema gives them, the containers they need being
// created. They are written only when the target profile is EXTENDED or
// carries at least the data set of inv.Profile (an EXTENDED invoice written as
// EN 16931 loses them), and only where the target schema allows them, without
// repeating an element beyond its maximum number of occurrences; the others are
// reported as lost.
func Marshal(inv *invoice.Invoice, opts *MarshalOptions) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("cii: nil invoice")
	}
	var o MarshalOptions
	if opts != nil {
		o = *opts
	}
	target := o.Profile
	if target == invoice.ProfileUnknown {
		target = inv.Profile
	}
	if target == invoice.ProfileUnknown {
		target = invoice.ProfileEN16931
	}
	w := writer{
		b:      make([]byte, 0, sizeHint(inv, o.Indent)),
		indent: o.Indent,
		lvl:    levelOf(target),
		target: target,
	}
	if inv.HasExtensions() {
		src := lvlExtended
		if inv.Profile != invoice.ProfileUnknown {
			src = levelOf(inv.Profile)
		}
		w.track = true
		w.extOK = w.lvl >= lvlExtended || w.lvl >= src
	}
	w.b = append(w.b, xmlDeclaration...)
	w.document(inv, target)
	if o.Indent {
		w.b = append(w.b, '\n')
	}
	if o.Strict {
		w.losses = append(w.losses, inv.NationalData()...)
	}
	if o.Strict && len(w.losses) > 0 {
		return nil, &LossError{Profile: target, Terms: w.losses}
	}
	return w.b, nil
}

// sizeHint estimates the size of the output so that it is built in a single
// allocation.
func sizeHint(inv *invoice.Invoice, indent bool) int {
	n := 2500 + 1500*len(inv.Lines)
	for _, p := range []*invoice.Party{inv.SellerTaxRepresentative, inv.Payee, inv.SalesAgent, inv.BuyerAgent, inv.Invoicer, inv.Invoicee, inv.Payer, inv.ShipTo} {
		if p != nil {
			n += 1100
		}
	}
	for i := range inv.Notes {
		n += len(inv.Notes[i].Content) + 100
	}
	for i := range inv.Lines {
		l := &inv.Lines[i]
		n += len(l.Note) + len(l.Item.Description) + 400*len(l.AllowancesCharges) +
			200*(len(l.Item.Attributes)+len(l.Item.Classifications))
		if l.ShipTo != nil {
			n += 1100
		}
		if l.InvoiceReference != nil {
			n += 300
		}
	}
	for i := range inv.AdditionalDocuments {
		n += len(inv.AdditionalDocuments[i].Attachment)*4/3 + 350
	}
	n += 450*len(inv.AllowancesCharges) + 400*len(inv.VATBreakdown) +
		350*len(inv.PaymentMeans) + 300*len(inv.PaymentTerms) + 300*len(inv.PrecedingInvoices) +
		450*len(inv.LogisticsCharges)
	for _, p := range []*invoice.Party{inv.ProductEndUser, inv.UltimateShipTo, inv.ShipFrom} {
		if p != nil {
			n += 1100
		}
	}
	if indent {
		n += n / 3
	}
	return n
}

func (w *writer) document(inv *invoice.Invoice, target invoice.Profile) {
	at := len(w.b)
	w.newline()
	if w.track {
		w.trackStart(rootName, at, len(w.b)+1+len(rootName), true)
	}
	w.b = append(w.b, rootStart...)
	if w.track {
		w.frames[0].body = len(w.b)
		w.hold(inv.Extensions)
	}
	w.depth++
	w.context(inv, target)
	w.exchangedDocument(inv)
	w.start("rsm:SupplyChainTradeTransaction")
	if len(inv.Lines) > 0 {
		if w.lvl >= lvlBasic {
			for i := range inv.Lines {
				w.line(&inv.Lines[i])
			}
		} else {
			w.lose("BG-25")
		}
	}
	w.agreement(inv)
	w.delivery(inv)
	w.settlement(inv)
	w.end("rsm:SupplyChainTradeTransaction")
	w.end(rootName)
}

func (w *writer) context(inv *invoice.Invoice, target invoice.Profile) {
	w.start("rsm:ExchangedDocumentContext")
	if inv.TestIndicator {
		if w.lvl >= lvlExtended {
			w.indicator("ram:TestIndicator", true)
		} else {
			w.lose("EXT TestIndicator")
		}
	}
	if inv.BusinessProcess != "" {
		w.start("ram:BusinessProcessSpecifiedDocumentContextParameter")
		w.leafReq("ram:ID", inv.BusinessProcess)
		w.end("ram:BusinessProcessSpecifiedDocumentContextParameter")
	}
	spec := target.URNFor(inv.Seller.Address.CountryCode)
	if inv.SpecificationID != "" && !zugferd1URN(inv.SpecificationID) && detectProfile(inv.SpecificationID) == target {
		spec = strings.TrimSpace(inv.SpecificationID)
		// The G-account extension exists in UBL only.
		spec = strings.Replace(spec, invoice.GAccountExtension, "", 1)
	}
	w.start("ram:GuidelineSpecifiedDocumentContextParameter")
	w.leafReq("ram:ID", spec)
	w.end("ram:GuidelineSpecifiedDocumentContextParameter")
	w.end("rsm:ExchangedDocumentContext")
}

func (w *writer) exchangedDocument(inv *invoice.Invoice) {
	ext := w.lvl >= lvlExtended
	w.start("rsm:ExchangedDocument")
	w.leafReq("ram:ID", inv.Number)
	w.leafReq("ram:TypeCode", inv.TypeCode)
	w.dateIn("ram:IssueDateTime", "udt:DateTimeString", inv.IssueDate)
	if inv.CopyIndicator {
		if ext {
			w.indicator("ram:CopyIndicator", true)
		} else {
			w.lose("EXT CopyIndicator")
		}
	}
	w.text(ext, "ram:LanguageID", inv.Language, "EXT Language")
	for i := range inv.Notes {
		n := &inv.Notes[i]
		if w.lvl < lvlBasicWL {
			w.lose("BG-1")
			break
		}
		w.start("ram:IncludedNote")
		w.hold(n.Extensions)
		w.text(ext, "ram:ContentCode", n.ContentCode, "EXT Note.ContentCode")
		if ext {
			w.leaf("ram:Content", n.Content)
		} else {
			w.leafReq("ram:Content", n.Content)
		}
		w.leaf("ram:SubjectCode", n.SubjectCode)
		w.end("ram:IncludedNote")
	}
	w.end("rsm:ExchangedDocument")
}

func (w *writer) line(l *invoice.Line) {
	ext := w.lvl >= lvlExtended
	en := w.lvl >= lvlEN16931
	w.start("ram:IncludedSupplyChainTradeLineItem")
	w.hold(l.Extensions)

	w.start("ram:AssociatedDocumentLineDocument")
	w.leafReq("ram:LineID", l.ID)
	w.text(ext, "ram:ParentLineID", l.ParentID, "EXT Line.ParentID")
	w.text(ext, "ram:LineStatusCode", l.StatusCode, "EXT Line.StatusCode")
	w.text(ext, "ram:LineStatusReasonCode", l.StatusReasonCode, "EXT Line.StatusReasonCode")
	if l.Note != "" {
		w.start("ram:IncludedNote")
		w.leafReq("ram:Content", l.Note)
		w.end("ram:IncludedNote")
	}
	w.end("ram:AssociatedDocumentLineDocument")

	w.item(&l.Item)

	// Trade agreement: order reference and prices.
	m := w.start("ram:SpecifiedLineTradeAgreement")
	w.deliveryTerms(l.DeliveryTerms, "EXT Line.DeliveryTerms")
	if l.OrderReference != "" || l.OrderLineReference != "" {
		ref, lineRef := l.OrderReference, l.OrderLineReference
		if ref != "" && !ext {
			w.lose("EXT Line.OrderReference")
			ref = ""
		}
		if lineRef != "" && !en {
			w.lose("BT-132")
			lineRef = ""
		}
		if ref != "" || lineRef != "" {
			w.start("ram:BuyerOrderReferencedDocument")
			w.leaf("ram:IssuerAssignedID", ref)
			w.leaf("ram:LineID", lineRef)
			w.end("ram:BuyerOrderReferencedDocument")
		}
	}
	detailed := len(l.PriceAllowancesCharges) > 0
	if detailed && !ext {
		w.lose("EXT Line.PriceAllowancesCharges")
		detailed = false
	}
	if l.GrossPrice.IsSet() || l.PriceDiscount.IsSet() || detailed {
		w.start("ram:GrossPriceProductTradePrice")
		w.decReq("ram:ChargeAmount", l.GrossPrice)
		w.decAttr("ram:BasisQuantity", "unitCode", l.PriceBaseQuantityUnit, l.PriceBaseQuantity)
		switch {
		case detailed:
			for i := range l.PriceAllowancesCharges {
				w.priceAllowanceCharge(&l.PriceAllowancesCharges[i])
			}
		case l.PriceDiscount.IsSet():
			w.start("ram:AppliedTradeAllowanceCharge")
			w.indicator("ram:ChargeIndicator", false)
			w.decReq("ram:ActualAmount", l.PriceDiscount)
			w.end("ram:AppliedTradeAllowanceCharge")
		}
		w.end("ram:GrossPriceProductTradePrice")
	}
	if !ext || l.NetPrice.IsSet() || l.PriceBaseQuantity.IsSet() {
		w.start("ram:NetPriceProductTradePrice")
		w.decReq("ram:ChargeAmount", l.NetPrice)
		w.decAttr("ram:BasisQuantity", "unitCode", l.PriceBaseQuantityUnit, l.PriceBaseQuantity)
		w.end("ram:NetPriceProductTradePrice")
	}
	for i := range l.UltimateCustomerOrders {
		if !ext {
			w.lose("EXT Line.UltimateCustomerOrders")
			break
		}
		w.reference("ram:UltimateCustomerOrderReferencedDocument", &l.UltimateCustomerOrders[i])
	}
	if ext {
		w.endOpt("ram:SpecifiedLineTradeAgreement", m)
	} else {
		w.end("ram:SpecifiedLineTradeAgreement")
	}

	// Trade delivery: quantity, and in EXTENDED the line delivery.
	m = w.start("ram:SpecifiedLineTradeDelivery")
	if !ext || l.Quantity.IsSet() || l.UnitCode != "" {
		w.openLeaf("ram:BilledQuantity")
		if l.UnitCode != "" {
			w.attr("unitCode", l.UnitCode)
		}
		w.b = append(w.b, '>')
		w.b = l.Quantity.Append(w.b)
		w.b = append(w.b, "</ram:BilledQuantity>"...)
	}
	if !l.ShipTo.IsZero() {
		if ext {
			w.party("ram:ShipToTradeParty", l.ShipTo, roleLineShipTo, false)
		} else {
			w.lose(partyTerms[roleLineShipTo][fGroup])
		}
	}
	w.optParty("ram:UltimateShipToTradeParty", l.UltimateShipTo, roleLineUltimateShipTo, ext)
	if !l.DeliveryDate.IsZero() {
		if ext {
			w.start("ram:ActualDeliverySupplyChainEvent")
			w.dateTime("ram:OccurrenceDateTime", l.DeliveryDate)
			w.end("ram:ActualDeliverySupplyChainEvent")
		} else {
			w.lose("EXT Line.DeliveryDate")
		}
	}
	if !l.DeliveryNote.IsZero() {
		if ext {
			w.reference("ram:DeliveryNoteReferencedDocument", l.DeliveryNote)
		} else {
			w.lose("EXT Line.DeliveryNote")
		}
	}
	if ext {
		w.endOpt("ram:SpecifiedLineTradeDelivery", m)
	} else {
		w.end("ram:SpecifiedLineTradeDelivery")
	}

	// Trade settlement.
	w.start("ram:SpecifiedLineTradeSettlement")
	if !ext || l.VATCategoryCode != "" || l.VATRate.IsSet() {
		w.start("ram:ApplicableTradeTax")
		w.leafReq("ram:TypeCode", "VAT")
		w.leafReq("ram:CategoryCode", l.VATCategoryCode)
		w.dec("ram:RateApplicablePercent", l.VATRate)
		w.end("ram:ApplicableTradeTax")
	}
	w.period(&l.Period, "BG-26")
	for i := range l.AllowancesCharges {
		w.allowanceCharge(&l.AllowancesCharges[i], true)
	}
	w.start("ram:SpecifiedTradeSettlementLineMonetarySummation")
	w.decReq("ram:LineTotalAmount", l.NetAmount)
	w.end("ram:SpecifiedTradeSettlementLineMonetarySummation")
	if r := l.InvoiceReference; !r.IsZero() {
		if ext {
			w.reference("ram:InvoiceReferencedDocument", r)
		} else {
			w.lose("EXT Line.InvoiceReference")
		}
	}
	if !l.ObjectIdentifier.IsZero() {
		if en {
			w.start("ram:AdditionalReferencedDocument")
			w.leafReq("ram:IssuerAssignedID", l.ObjectIdentifier.ID)
			w.leafReq("ram:TypeCode", "130")
			w.leaf("ram:ReferenceTypeCode", l.ObjectIdentifier.Scheme)
			w.end("ram:AdditionalReferencedDocument")
		} else {
			w.lose("BT-128")
		}
	}
	if l.AccountingReference != "" {
		if en {
			w.start("ram:ReceivableSpecifiedTradeAccountingAccount")
			w.leafReq("ram:ID", l.AccountingReference)
			w.end("ram:ReceivableSpecifiedTradeAccountingAccount")
		} else {
			w.lose("BT-133")
		}
	}
	w.end("ram:SpecifiedLineTradeSettlement")
	w.end("ram:IncludedSupplyChainTradeLineItem")
}

func (w *writer) item(it *invoice.Item) {
	en := w.lvl >= lvlEN16931
	ext := w.lvl >= lvlExtended
	w.start("ram:SpecifiedTradeProduct")
	w.leafAttr("ram:GlobalID", "schemeID", it.GlobalID.Scheme, it.GlobalID.ID)
	w.text(en, "ram:SellerAssignedID", it.SellerID, "BT-155")
	w.text(en, "ram:BuyerAssignedID", it.BuyerID, "BT-156")
	w.leafReq("ram:Name", it.Name)
	w.text(en, "ram:Description", it.Description, "BT-154")
	for i := range it.Attributes {
		a := &it.Attributes[i]
		if !en {
			w.lose("BG-32")
			break
		}
		w.start("ram:ApplicableProductCharacteristic")
		w.hold(a.Extensions)
		w.text(ext, "ram:TypeCode", a.TypeCode, "EXT ItemAttribute.TypeCode")
		w.leafReq("ram:Description", a.Name)
		w.leafReq("ram:Value", a.Value)
		w.end("ram:ApplicableProductCharacteristic")
	}
	for i := range it.Classifications {
		c := &it.Classifications[i]
		if !en {
			w.lose("BT-158")
			break
		}
		m := w.start("ram:DesignatedProductClassification")
		w.hold(c.Extensions)
		if c.Code != "" || c.ListID != "" || c.ListVersionID != "" {
			w.openLeaf("ram:ClassCode")
			if c.ListID != "" {
				w.attr("listID", c.ListID)
			}
			if c.ListVersionID != "" {
				w.attr("listVersionID", c.ListVersionID)
			}
			w.closeLeaf("ram:ClassCode", c.Code)
		}
		w.text(ext, "ram:ClassName", c.Name, "EXT Classification.Name")
		w.endOpt("ram:DesignatedProductClassification", m)
	}
	if it.OriginCountry != "" {
		if en {
			w.start("ram:OriginTradeCountry")
			w.leafReq("ram:ID", it.OriginCountry)
			w.end("ram:OriginTradeCountry")
		} else {
			w.lose("BT-159")
		}
	}
	for i := range it.ReferencedProducts {
		if !ext {
			w.lose("EXT Item.ReferencedProducts")
			break
		}
		w.referencedProduct(&it.ReferencedProducts[i])
	}
	w.end("ram:SpecifiedTradeProduct")
}

// referencedProduct writes a product included in the item of a line (EXT).
func (w *writer) referencedProduct(p *invoice.ReferencedProduct) {
	w.start("ram:IncludedReferencedProduct")
	w.hold(p.Extensions)
	w.leaf("ram:ID", p.ID)
	for _, id := range p.GlobalIDs {
		w.leafAttr("ram:GlobalID", "schemeID", id.Scheme, id.ID)
	}
	w.leaf("ram:SellerAssignedID", p.SellerID)
	w.leaf("ram:BuyerAssignedID", p.BuyerID)
	w.leaf("ram:IndustryAssignedID", p.IndustryID)
	w.leafReq("ram:Name", p.Name)
	w.leaf("ram:Description", p.Description)
	w.decAttr("ram:UnitQuantity", "unitCode", p.UnitQuantityUnit, p.UnitQuantity)
	w.end("ram:IncludedReferencedProduct")
}

// priceAllowanceCharge writes an allowance or a charge applied to the gross
// price of a line (EXT).
func (w *writer) priceAllowanceCharge(ac *invoice.AllowanceCharge) {
	w.start("ram:AppliedTradeAllowanceCharge")
	w.hold(ac.Extensions)
	w.indicator("ram:ChargeIndicator", ac.Charge)
	w.dec("ram:CalculationPercent", ac.Percentage)
	w.dec("ram:BasisAmount", ac.BaseAmount)
	w.decReq("ram:ActualAmount", ac.Amount)
	w.leaf("ram:ReasonCode", ac.ReasonCode)
	w.leaf("ram:Reason", ac.Reason)
	w.end("ram:AppliedTradeAllowanceCharge")
}

// reference writes a ram:ReferencedDocumentType from a document reference
// (EXT, or BG-3 for a preceding invoice).
func (w *writer) reference(name string, d *invoice.DocumentReference) {
	w.start(name)
	w.hold(d.Extensions)
	w.leaf("ram:IssuerAssignedID", d.ID)
	w.leaf("ram:LineID", d.LineID)
	w.leaf("ram:TypeCode", d.TypeCode)
	if !d.IssueDate.IsZero() {
		w.dateIn("ram:FormattedIssueDateTime", "qdt:DateTimeString", d.IssueDate)
	}
	w.end(name)
}

// deliveryTerms writes delivery terms (EXT), or records term as lost.
func (w *writer) deliveryTerms(t *invoice.DeliveryTerms, term string) {
	if t == nil || *t == (invoice.DeliveryTerms{}) {
		return
	}
	if w.lvl < lvlExtended {
		w.lose(term)
		return
	}
	w.start("ram:ApplicableTradeDeliveryTerms")
	w.leafReq("ram:DeliveryTypeCode", t.Code)
	if t.LocationCountryCode != "" || t.LocationName != "" {
		w.start("ram:RelevantTradeLocation")
		w.leaf("ram:CountryID", t.LocationCountryCode)
		w.leaf("ram:Name", t.LocationName)
		w.end("ram:RelevantTradeLocation")
	}
	w.end("ram:ApplicableTradeDeliveryTerms")
}

// allowanceCharge writes a document level (BG-20, BG-21) or line level
// (BG-27, BG-28) allowance or charge.
func (w *writer) allowanceCharge(ac *invoice.AllowanceCharge, line bool) {
	full := true // percentage and base amount
	pct, base := "BT-94", "BT-93"
	if ac.Charge {
		pct, base = "BT-101", "BT-100"
	}
	if line {
		full = w.lvl >= lvlEN16931
		pct, base = "BT-138", "BT-137"
		if ac.Charge {
			pct, base = "BT-143", "BT-142"
		}
	}
	w.start("ram:SpecifiedTradeAllowanceCharge")
	w.hold(ac.Extensions)
	w.indicator("ram:ChargeIndicator", ac.Charge)
	w.amount(full, "ram:CalculationPercent", ac.Percentage, pct)
	w.amount(full, "ram:BasisAmount", ac.BaseAmount, base)
	w.decReq("ram:ActualAmount", ac.Amount)
	w.leaf("ram:ReasonCode", ac.ReasonCode)
	w.leaf("ram:Reason", ac.Reason)
	if ac.VATCategoryCode != "" || ac.VATRate.IsSet() || (!line && w.lvl == lvlBasicWL) {
		if !line || w.lvl >= lvlExtended {
			w.start("ram:CategoryTradeTax")
			w.leafReq("ram:TypeCode", "VAT")
			w.leafReq("ram:CategoryCode", ac.VATCategoryCode)
			w.dec("ram:RateApplicablePercent", ac.VATRate)
			w.end("ram:CategoryTradeTax")
		} else {
			w.lose("EXT Line.AllowanceCharge.VAT")
		}
	}
	w.end("ram:SpecifiedTradeAllowanceCharge")
}

// period writes a ram:BillingSpecifiedPeriod (BG-14 or BG-26).
func (w *writer) period(p *invoice.Period, term string) {
	if p.IsZero() {
		return
	}
	if w.lvl < lvlBasicWL {
		w.lose(term)
		return
	}
	m := w.start("ram:BillingSpecifiedPeriod")
	if p.Description != "" {
		if w.lvl >= lvlExtended {
			w.leafReq("ram:Description", p.Description)
		} else {
			w.lose("EXT " + term + ".Description")
		}
	}
	w.dateTime("ram:StartDateTime", p.Start)
	w.dateTime("ram:EndDateTime", p.End)
	w.endOpt("ram:BillingSpecifiedPeriod", m)
}

// docRef writes a referenced document that carries only its identifier.
func (w *writer) docRef(allowed bool, name, id, term string) {
	if id == "" {
		return
	}
	if !allowed {
		w.lose(term)
		return
	}
	w.start(name)
	w.leafReq("ram:IssuerAssignedID", id)
	w.end(name)
}

func (w *writer) agreement(inv *invoice.Invoice) {
	l := w.lvl
	w.start("ram:ApplicableHeaderTradeAgreement")
	w.leaf("ram:BuyerReference", inv.BuyerReference)
	w.party("ram:SellerTradeParty", &inv.Seller, roleSeller, true)
	w.party("ram:BuyerTradeParty", &inv.Buyer, roleBuyer, true)
	w.optParty("ram:SalesAgentTradeParty", inv.SalesAgent, roleSalesAgent, l >= lvlExtended)
	w.optParty("ram:SellerTaxRepresentativeTradeParty", inv.SellerTaxRepresentative, roleSellerTaxRep, l >= lvlBasicWL)
	w.optParty("ram:ProductEndUserTradeParty", inv.ProductEndUser, roleProductEndUser, l >= lvlExtended)
	w.deliveryTerms(inv.DeliveryTerms, "EXT DeliveryTerms")
	w.docRef(l >= lvlEN16931, "ram:SellerOrderReferencedDocument", inv.SalesOrderReference, "BT-14")
	w.docRef(true, "ram:BuyerOrderReferencedDocument", inv.PurchaseOrderReference, "BT-13")
	w.docRef(l >= lvlBasicWL, "ram:ContractReferencedDocument", inv.ContractReference, "BT-12")
	for i := range inv.AdditionalDocuments {
		w.additionalDocument(&inv.AdditionalDocuments[i])
	}
	w.optParty("ram:BuyerAgentTradeParty", inv.BuyerAgent, roleBuyerAgent, l >= lvlExtended)
	if inv.ProjectReference != "" || inv.ProjectName != "" {
		if l >= lvlEN16931 {
			w.start("ram:SpecifiedProcuringProject")
			w.leafReq("ram:ID", inv.ProjectReference)
			w.leafReq("ram:Name", inv.ProjectName)
			w.end("ram:SpecifiedProcuringProject")
		} else {
			w.lose("BT-11")
		}
	}
	for i := range inv.UltimateCustomerOrders {
		if l < lvlExtended {
			w.lose("EXT UltimateCustomerOrders")
			break
		}
		w.reference("ram:UltimateCustomerOrderReferencedDocument", &inv.UltimateCustomerOrders[i])
	}
	w.end("ram:ApplicableHeaderTradeAgreement")
}

// additionalDocument writes a BG-24 supporting document, or the BT-17 tender
// reference (type code 50) or the BT-18 invoiced object (type code 130).
func (w *writer) additionalDocument(d *invoice.AdditionalDocument) {
	code := d.TypeCode
	if code == "" {
		code = "916"
	}
	if w.lvl < lvlEN16931 {
		switch code {
		case "50":
			w.lose("BT-17")
		case "130":
			w.lose("BT-18")
		default:
			w.lose("BG-24")
		}
		return
	}
	doc := code != "50" && code != "130"
	w.start("ram:AdditionalReferencedDocument")
	w.hold(d.Extensions)
	w.leafReq("ram:IssuerAssignedID", d.ID)
	w.text(doc, "ram:URIID", d.URI, "BT-124")
	w.leafReq("ram:TypeCode", code)
	w.text(doc, "ram:Name", d.Description, "BT-123")
	if len(d.Attachment) > 0 || d.AttachmentMimeCode != "" || d.AttachmentFilename != "" {
		if doc {
			w.binary("ram:AttachmentBinaryObject", d.Attachment, d.AttachmentMimeCode, d.AttachmentFilename)
		} else {
			w.lose("BT-125")
		}
	}
	if code == "130" {
		w.leaf("ram:ReferenceTypeCode", d.ReferenceTypeCode)
	} else if d.ReferenceTypeCode != "" {
		// Factur-X only allows a reference type on object identifiers.
		w.lose("AdditionalDocument.ReferenceTypeCode (TypeCode 130 only)")
	}
	if !d.IssueDate.IsZero() {
		if w.lvl >= lvlExtended {
			w.dateIn("ram:FormattedIssueDateTime", "qdt:DateTimeString", d.IssueDate)
		} else {
			w.lose("EXT AdditionalDocument.IssueDate")
		}
	}
	w.end("ram:AdditionalReferencedDocument")
}

func (w *writer) delivery(inv *invoice.Invoice) {
	l := w.lvl
	w.start("ram:ApplicableHeaderTradeDelivery")
	w.optParty("ram:ShipToTradeParty", inv.ShipTo, roleShipTo, l >= lvlBasicWL)
	w.optParty("ram:UltimateShipToTradeParty", inv.UltimateShipTo, roleUltimateShipTo, l >= lvlExtended)
	w.optParty("ram:ShipFromTradeParty", inv.ShipFrom, roleShipFrom, l >= lvlExtended)
	if !inv.DeliveryDate.IsZero() {
		if l >= lvlBasicWL {
			w.start("ram:ActualDeliverySupplyChainEvent")
			w.dateTime("ram:OccurrenceDateTime", inv.DeliveryDate)
			w.end("ram:ActualDeliverySupplyChainEvent")
		} else {
			w.lose("BT-72")
		}
	}
	w.docRef(l >= lvlBasicWL, "ram:DespatchAdviceReferencedDocument", inv.DespatchAdviceReference, "BT-16")
	w.docRef(l >= lvlEN16931, "ram:ReceivingAdviceReferencedDocument", inv.ReceivingAdviceReference, "BT-15")
	if !inv.DeliveryNote.IsZero() {
		if l >= lvlExtended {
			w.reference("ram:DeliveryNoteReferencedDocument", inv.DeliveryNote)
		} else {
			w.lose("EXT DeliveryNote")
		}
	}
	w.end("ram:ApplicableHeaderTradeDelivery")
}

func (w *writer) settlement(inv *invoice.Invoice) {
	l := w.lvl
	wl := l >= lvlBasicWL
	ext := l >= lvlExtended
	w.start("ram:ApplicableHeaderTradeSettlement")
	w.text(wl, "ram:CreditorReferenceID", inv.CreditorReferenceID, "BT-90")
	w.text(wl, "ram:PaymentReference", inv.RemittanceInformation, "BT-83")
	w.text(wl, "ram:TaxCurrencyCode", inv.TaxCurrency, "BT-6")
	w.leafReq("ram:InvoiceCurrencyCode", inv.Currency)
	w.optParty("ram:InvoicerTradeParty", inv.Invoicer, roleInvoicer, ext)
	w.optParty("ram:InvoiceeTradeParty", inv.Invoicee, roleInvoicee, ext)
	w.optParty("ram:PayeeTradeParty", inv.Payee, rolePayee, wl)
	w.optParty("ram:PayerTradeParty", inv.Payer, rolePayer, ext)
	if x := inv.CurrencyExchange; x != nil && (x.SourceCurrency != "" || x.TargetCurrency != "" || x.Rate.IsSet() || !x.Date.IsZero()) {
		if ext {
			w.start("ram:TaxApplicableTradeCurrencyExchange")
			w.leafReq("ram:SourceCurrencyCode", x.SourceCurrency)
			w.leafReq("ram:TargetCurrencyCode", x.TargetCurrency)
			w.decReq("ram:ConversionRate", x.Rate)
			w.dateTime("ram:ConversionRateDateTime", x.Date)
			w.end("ram:TaxApplicableTradeCurrencyExchange")
		} else {
			w.lose("EXT CurrencyExchange")
		}
	}
	for i := range inv.PaymentMeans {
		w.paymentMeans(&inv.PaymentMeans[i])
	}
	w.vatBreakdown(inv)
	w.period(&inv.InvoicingPeriod, "BG-14")
	for i := range inv.AllowancesCharges {
		ac := &inv.AllowancesCharges[i]
		if !wl {
			if ac.Charge {
				w.lose("BG-21")
			} else {
				w.lose("BG-20")
			}
			continue
		}
		w.allowanceCharge(ac, false)
	}
	for i := range inv.LogisticsCharges {
		if !ext {
			w.lose("EXT LogisticsCharges")
			break
		}
		w.logisticsCharge(&inv.LogisticsCharges[i])
	}
	w.paymentTerms(inv)
	w.totals(inv)
	for i := range inv.ThirdPartyCharges {
		c := &inv.ThirdPartyCharges[i]
		if !ext {
			w.lose("BG-34")
			break
		}
		w.start("ram:SpecifiedFinancialAdjustment")
		w.hold(c.Extensions)
		w.leafReq("ram:Reason", c.Reason)
		w.decReq("ram:ActualAmount", c.Amount)
		w.end("ram:SpecifiedFinancialAdjustment")
	}
	for i := range inv.PrecedingInvoices {
		r := &inv.PrecedingInvoices[i]
		// The CII D16B schema of XRechnung allows a single
		// ram:InvoiceReferencedDocument, the Factur-X schemas several.
		if !wl || i > 0 && w.target == invoice.ProfileXRechnung {
			w.lose("BG-3")
			break
		}
		w.start("ram:InvoiceReferencedDocument")
		w.hold(r.Extensions)
		w.leafReq("ram:IssuerAssignedID", r.ID)
		w.text(ext, "ram:LineID", r.LineID, "EXT PrecedingInvoice.LineID")
		w.text(ext, "ram:TypeCode", r.TypeCode, "EXT PrecedingInvoice.TypeCode")
		if !r.IssueDate.IsZero() {
			w.dateIn("ram:FormattedIssueDateTime", "qdt:DateTimeString", r.IssueDate)
		}
		w.end("ram:InvoiceReferencedDocument")
	}
	if inv.BuyerAccountingReference != "" {
		if wl {
			w.start("ram:ReceivableSpecifiedTradeAccountingAccount")
			w.leafReq("ram:ID", inv.BuyerAccountingReference)
			w.end("ram:ReceivableSpecifiedTradeAccountingAccount")
		} else {
			w.lose("BT-19")
		}
	}
	w.end("ram:ApplicableHeaderTradeSettlement")
}

// logisticsCharge writes a logistics service charge (EXT).
func (w *writer) logisticsCharge(c *invoice.LogisticsCharge) {
	w.start("ram:SpecifiedLogisticsServiceCharge")
	w.hold(c.Extensions)
	w.leafReq("ram:Description", c.Description)
	w.decReq("ram:AppliedAmount", c.Amount)
	if c.VATCategoryCode != "" || c.VATRate.IsSet() {
		w.start("ram:AppliedTradeTax")
		w.leafReq("ram:TypeCode", "VAT")
		w.leafReq("ram:CategoryCode", c.VATCategoryCode)
		w.dec("ram:RateApplicablePercent", c.VATRate)
		w.end("ram:AppliedTradeTax")
	}
	w.end("ram:SpecifiedLogisticsServiceCharge")
}

func (w *writer) paymentMeans(pm *invoice.PaymentMeans) {
	if w.lvl < lvlBasicWL {
		w.lose("BG-16")
		return
	}
	en := w.lvl >= lvlEN16931
	w.start("ram:SpecifiedTradeSettlementPaymentMeans")
	w.hold(pm.Extensions)
	// The identifier of the G-account extension has no place in CII: the
	// Dutch Peppol Authority defines that extension for UBL only.
	w.text(false, "", pm.ID, "EXT PaymentMeans.ID")
	w.leafReq("ram:TypeCode", pm.TypeCode)
	w.text(en, "ram:Information", pm.Information, "BT-82")
	if pm.CardID != "" || pm.CardholderName != "" {
		if en {
			w.start("ram:ApplicableTradeSettlementFinancialCard")
			w.leafReq("ram:ID", pm.CardID)
			w.leaf("ram:CardholderName", pm.CardholderName)
			w.end("ram:ApplicableTradeSettlementFinancialCard")
		} else {
			w.lose("BG-18")
		}
	}
	ext := w.lvl >= lvlExtended
	if pm.PayerIBAN != "" || pm.PayerAccountName != "" && ext {
		w.start("ram:PayerPartyDebtorFinancialAccount")
		w.leafReq("ram:IBANID", pm.PayerIBAN)
		w.text(ext, "ram:AccountName", pm.PayerAccountName, "BT-216")
		w.end("ram:PayerPartyDebtorFinancialAccount")
	} else if pm.PayerAccountName != "" {
		w.lose("BT-216")
	}
	if pm.PayeeIBAN != "" || pm.PayeeAccountName != "" || pm.PayeeProprietaryID != "" {
		m := w.start("ram:PayeePartyCreditorFinancialAccount")
		w.leaf("ram:IBANID", pm.PayeeIBAN)
		w.text(en, "ram:AccountName", pm.PayeeAccountName, "BT-85")
		w.leaf("ram:ProprietaryID", pm.PayeeProprietaryID)
		w.endOpt("ram:PayeePartyCreditorFinancialAccount", m)
	}
	if pm.PayerBIC != "" {
		if ext {
			w.start("ram:PayerSpecifiedDebtorFinancialInstitution")
			w.leafReq("ram:BICID", pm.PayerBIC)
			w.end("ram:PayerSpecifiedDebtorFinancialInstitution")
		} else {
			w.lose("BT-215")
		}
	}
	if pm.PayeeBIC != "" {
		if en {
			w.start("ram:PayeeSpecifiedCreditorFinancialInstitution")
			w.leafReq("ram:BICID", pm.PayeeBIC)
			w.end("ram:PayeeSpecifiedCreditorFinancialInstitution")
		} else {
			w.lose("BT-86")
		}
	}
	w.end("ram:SpecifiedTradeSettlementPaymentMeans")
}

// vatBreakdown writes the BG-23 entries, each carrying BT-7 and BT-8.
func (w *writer) vatBreakdown(inv *invoice.Invoice) {
	if len(inv.VATBreakdown) == 0 || w.lvl < lvlBasicWL {
		if len(inv.VATBreakdown) > 0 {
			w.lose("BG-23")
		}
		if !inv.TaxPointDate.IsZero() {
			w.lose("BT-7")
		}
		if inv.TaxPointDateCode != "" {
			w.lose("BT-8")
		}
		return
	}
	taxPoint := !inv.TaxPointDate.IsZero()
	if taxPoint && w.lvl < lvlEN16931 {
		w.lose("BT-7")
		taxPoint = false
	}
	for i := range inv.VATBreakdown {
		v := &inv.VATBreakdown[i]
		w.start("ram:ApplicableTradeTax")
		w.hold(v.Extensions)
		w.dec("ram:CalculatedAmount", v.TaxAmount)
		w.leafReq("ram:TypeCode", "VAT")
		w.leaf("ram:ExemptionReason", v.ExemptionReason)
		w.dec("ram:BasisAmount", v.TaxableAmount)
		w.leafReq("ram:CategoryCode", v.CategoryCode)
		w.leaf("ram:ExemptionReasonCode", v.ExemptionReasonCode)
		if taxPoint {
			w.dateIn("ram:TaxPointDate", "udt:DateString", inv.TaxPointDate)
		}
		w.leaf("ram:DueDateTypeCode", inv.TaxPointDateCode)
		w.dec("ram:RateApplicablePercent", v.Rate)
		w.end("ram:ApplicableTradeTax")
	}
}

func (w *writer) paymentTerms(inv *invoice.Invoice) {
	ext := w.lvl >= lvlExtended
	for i := range inv.PaymentTerms {
		t := &inv.PaymentTerms[i]
		// The payment means reference of the G-account extension has no
		// place in CII (see paymentMeans).
		w.text(false, "", t.PaymentMeansID, "EXT PaymentTerms.PaymentMeansID")
		if w.lvl < lvlBasicWL {
			w.text(false, "", t.Description, "BT-20")
			w.date(false, "", t.DueDate, "BT-9")
			w.text(false, "", t.DirectDebitMandateID, "BT-89")
			w.amount(false, "", t.PartialAmount, "EXT PaymentTerms.PartialAmount")
			continue
		}
		if i > 0 && !ext {
			w.lose("EXT PaymentTerms[2:]")
			break
		}
		m := w.start("ram:SpecifiedTradePaymentTerms")
		w.hold(t.Extensions)
		w.leaf("ram:Description", t.Description)
		w.dateTime("ram:DueDateDateTime", t.DueDate)
		w.leaf("ram:DirectDebitMandateID", t.DirectDebitMandateID)
		w.amount(ext, "ram:PartialPaymentAmount", t.PartialAmount, "EXT PaymentTerms.PartialAmount")
		w.endOpt("ram:SpecifiedTradePaymentTerms", m)
	}
}

func (w *writer) totals(inv *invoice.Invoice) {
	t := &inv.Totals
	wl := w.lvl >= lvlBasicWL
	w.start("ram:SpecifiedTradeSettlementHeaderMonetarySummation")
	if wl {
		w.decReq("ram:LineTotalAmount", t.LineTotal)
	} else if t.LineTotal.IsSet() {
		w.lose("BT-106")
	}
	w.amount(wl, "ram:ChargeTotalAmount", t.ChargeTotal, "BT-108")
	w.amount(wl, "ram:AllowanceTotalAmount", t.AllowanceTotal, "BT-107")
	w.decReq("ram:TaxBasisTotalAmount", t.TaxBasisTotal)
	w.decAttr("ram:TaxTotalAmount", "currencyID", inv.Currency, t.TaxTotal)
	if t.TaxTotalInTaxCurrency.IsSet() {
		if wl {
			w.decAttr("ram:TaxTotalAmount", "currencyID", inv.TaxCurrency, t.TaxTotalInTaxCurrency)
		} else {
			w.lose("BT-111")
		}
	}
	w.amount(w.lvl >= lvlEN16931, "ram:RoundingAmount", t.RoundingAmount, "BT-114")
	w.decReq("ram:GrandTotalAmount", t.GrandTotal)
	w.amount(wl, "ram:TotalPrepaidAmount", t.PaidAmount, "BT-113")
	w.decReq("ram:DuePayableAmount", t.DuePayable)
	w.end("ram:SpecifiedTradeSettlementHeaderMonetarySummation")
}

// optParty writes an optional party, or records it as lost when the profile
// does not carry it.
func (w *writer) optParty(name string, p *invoice.Party, r role, allowed bool) {
	if p.IsZero() {
		return
	}
	if !allowed {
		w.lose(partyTerms[r][fGroup])
		return
	}
	w.party(name, p, r, false)
}

// party writes a ram:TradePartyType element with the fields that the role
// and the target profile allow. The seller and the buyer (required) always
// get a name element.
func (w *writer) party(name string, p *invoice.Party, r role, required bool) {
	pr := rulesFor(r, w.lvl)
	terms := &partyTerms[r]
	w.start(name)
	w.hold(p.Extensions)
	if len(p.IDs) > 0 {
		if pr.ids {
			for _, id := range p.IDs {
				if id.Scheme == "" {
					w.leafReq("ram:ID", id.ID)
				}
			}
			for _, id := range p.IDs {
				if id.Scheme != "" {
					w.leafAttr("ram:GlobalID", "schemeID", id.Scheme, id.ID)
				}
			}
		} else {
			w.lose(terms[fIDs])
		}
	}
	if required {
		w.leafReq("ram:Name", p.Name)
	} else {
		w.leaf("ram:Name", p.Name)
	}
	w.text(pr.desc, "ram:Description", p.Description, terms[fDescription])

	legal := !p.LegalRegistration.IsZero()
	if legal && !pr.legal {
		w.lose(terms[fLegal])
		legal = false
	}
	trading := p.TradingName != ""
	if trading && !pr.trading {
		w.lose(terms[fTrading])
		trading = false
	}
	if legal || trading {
		w.start("ram:SpecifiedLegalOrganization")
		if legal {
			w.leafAttr("ram:ID", "schemeID", p.LegalRegistration.Scheme, p.LegalRegistration.ID)
		}
		if trading {
			w.leafReq("ram:TradingBusinessName", p.TradingName)
		}
		w.end("ram:SpecifiedLegalOrganization")
	}

	if c := p.Contact; c != nil && (c.PersonName != "" || c.DepartmentName != "" || c.Phone != "" || c.Email != "") {
		if pr.contact {
			w.start("ram:DefinedTradeContact")
			w.leaf("ram:PersonName", c.PersonName)
			w.leaf("ram:DepartmentName", c.DepartmentName)
			if c.Phone != "" {
				w.start("ram:TelephoneUniversalCommunication")
				w.leafReq("ram:CompleteNumber", c.Phone)
				w.end("ram:TelephoneUniversalCommunication")
			}
			if c.Email != "" {
				w.start("ram:EmailURIUniversalCommunication")
				w.leafReq("ram:URIID", c.Email)
				w.end("ram:EmailURIUniversalCommunication")
			}
			w.end("ram:DefinedTradeContact")
		} else {
			w.lose(terms[fContact])
		}
	}

	if a := &p.Address; !a.IsZero() {
		switch {
		case pr.address:
			w.start("ram:PostalTradeAddress")
			w.leaf("ram:PostcodeCode", a.PostCode)
			w.leaf("ram:LineOne", a.Line1)
			w.leaf("ram:LineTwo", a.Line2)
			w.leaf("ram:LineThree", a.Line3)
			w.leaf("ram:CityName", a.City)
			w.leafReq("ram:CountryID", a.CountryCode)
			w.leaf("ram:CountrySubDivisionName", a.CountrySubdivision)
			w.end("ram:PostalTradeAddress")
		case pr.country:
			if a.PostCode != "" || a.Line1 != "" || a.Line2 != "" || a.Line3 != "" || a.City != "" || a.CountrySubdivision != "" {
				w.lose(terms[fAddress])
			}
			w.start("ram:PostalTradeAddress")
			w.leafReq("ram:CountryID", a.CountryCode)
			w.end("ram:PostalTradeAddress")
		default:
			w.lose(terms[fAddress])
		}
	}

	if !p.ElectronicAddress.IsZero() {
		if pr.uri {
			w.start("ram:URIUniversalCommunication")
			w.leafAttr("ram:URIID", "schemeID", p.ElectronicAddress.Scheme, p.ElectronicAddress.ID)
			w.end("ram:URIUniversalCommunication")
		} else {
			w.lose(terms[fURI])
		}
	}

	if p.VATID != "" {
		if pr.vat {
			w.start("ram:SpecifiedTaxRegistration")
			w.leafAttr("ram:ID", "schemeID", "VA", p.VATID)
			w.end("ram:SpecifiedTaxRegistration")
		} else {
			w.lose(terms[fVAT])
		}
	}
	if p.TaxRegistrationID != "" {
		if pr.fc {
			w.start("ram:SpecifiedTaxRegistration")
			w.leafAttr("ram:ID", "schemeID", "FC", p.TaxRegistrationID)
			w.end("ram:SpecifiedTaxRegistration")
		} else {
			w.lose(terms[fFC])
		}
	}
	w.end(name)
}
