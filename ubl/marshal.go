package ubl

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>`

// MarshalOptions tune Marshal. A nil *MarshalOptions is the zero value.
type MarshalOptions struct {
	// CustomizationID is the specification identifier (BT-24) to write.
	// When empty, Marshal writes inv.SpecificationID if it identifies an
	// EN 16931 invoice that UBL can carry (EN 16931 itself, a CIUS such as
	// XRechnung or Peppol BIS, EXTENDED-CTC-FR, or an identifier it does not
	// know), and the EN 16931 identifier otherwise (the Factur-X profiles
	// MINIMUM, BASIC WL, BASIC and EXTENDED only exist in CII).
	CustomizationID string
	// Indent writes one element per line, indented with two spaces.
	Indent bool
	// Strict makes Marshal fail with a *LossError when the invoice holds data
	// that the UBL binding cannot carry (EXTENDED fields of the model,
	// extensions…). Without Strict such data is omitted.
	Strict bool
}

// LossError is returned by Marshal in strict mode when the invoice holds data
// that the UBL binding cannot carry.
type LossError struct {
	// Terms lists the business terms that would be lost, in document order:
	// "BT-33", "BG-25"…, or "EXT <Field>" for data of the model outside
	// the EN 16931 data set and "EXT <Role>.<Field>" for party data. An
	// extension (see invoice.Extension) is reported as "EXT " followed by its
	// entry in Invoice.Unmodeled. The data of the national blocks that UBL
	// does not carry (the FatturaPA data of invoice.Italy, the Facturae data
	// of invoice.Spain, the Croatian data of invoice.Croatia when the
	// identifier written is not the one of the CIUS-HR) follows, as listed
	// by invoice.Invoice.NationalData: "EXT Italy.Withholdings". A domestic
	// Italian invoice carries part of the FatturaPA data (see Marshal); a
	// field carried in part only is reported.
	Terms []string
}

// Error lists the business terms that would be lost.
func (e *LossError) Error() string {
	return "ubl: UBL cannot carry " + strconv.Itoa(len(e.Terms)) + " business terms: " + strings.Join(e.Terms, ", ")
}

// creditNoteCodes are the document type codes (BT-3) of a CreditNote, as
// listed by rule BR-CL-01 of the EN 16931 UBL validation artefacts.
const creditNoteCodes = " 81 83 261 262 296 308 381 396 420 458 502 503 532 "

// IsCreditNote reports whether a document type code (BT-3) is written as a UBL
// CreditNote rather than an Invoice.
func IsCreditNote(typeCode string) bool {
	t := strings.TrimSpace(typeCode)
	return t != "" && !strings.Contains(t, " ") && strings.Contains(creditNoteCodes, " "+t+" ")
}

// Marshal writes inv as a UBL 2.1 Invoice, or as a CreditNote when its type
// code (BT-3) is a credit note code (see IsCreditNote).
//
// The business terms are written in the elements that CEN/TS 16931-3-2 binds
// them to, in the order of the UBL 2.1 schema. A note with a subject code
// (BT-21) is written "#AAI#text"; the VAT point date code (BT-8) is written
// with the UBL codes (3, 35 and 432 for the model's 5, 29 and 72); in a credit
// note, the payment due date (BT-9) goes into the first cac:PaymentMeans and
// the project reference (BT-11) into an additional document reference of type
// 50. The logistics charges of the model are written as document level
// charges, which is how EN 16931 counts them. A Peppol or XRechnung invoice
// (whose specification identifier is the Peppol or an XRechnung one, see
// invoice.URNPeppol and invoice.URNXRechnung) without business process type (BT-23) gets the one of
// Peppol billing, invoice.ProcessPeppolBilling, as rule PEPPOL-EN16931-R001
// requires one; a Peppol Self-Billing invoice (invoice.URNPeppolSB) gets the
// one of Peppol self-billing, invoice.ProcessPeppolSelfBilling. A
// self-billed credit note (type code 261) is written as a CreditNote, a
// self-billed invoice (389) as an Invoice, with the seller and the buyer in
// their usual elements.
//
// When the specification identifier written declares the Dutch G-account
// extension (see invoice.GAccountExtension), the elements that the extension
// adds to the binding are written too: PaymentMeans.ID in
// cac:PaymentMeans/cbc:ID, and each payment terms entry that has a
// description, a PaymentMeansID or a PartialAmount in a cac:PaymentTerms of
// its own, with cbc:PaymentMeansID, cbc:Note and cbc:Amount. The due date
// (BT-9) and the mandate reference (BT-89) are still read from the first
// entry; those of the other entries cannot be carried. Without the extension,
// these fields of the model are not part of the binding, nor are the payment
// terms after the first.
//
// When the specification identifier written declares the Italian extension
// of the domestic invoice (see invoice.ItalianExtension), the national data
// that appendix 4.2 of the Italian technical rules binds to UBL is written
// too, as Parse reads it: the UBL extensions of EXT-IT (issuing system,
// issuer, document type, types of the withholdings, Art. 73, rounding of each
// VAT breakdown, discounts and surcharges on the total), the date of the
// purchase order, the main invoice (in the first cac:BillingReference, which
// a preceding invoice must open), the dated transport documents, the tax
// regime, the permanent establishment and the third-party intermediary of the
// seller, the tax code and the EORI code of the tax representative, the
// carrier, the pickup and the shipment of the delivery, the delivery terms,
// the withholdings (cac:WithholdingTaxTotal), the discount for early payment
// and the penalty for late payment of the first payment means, and the
// purchase orders and transport documents of each line. The rest of the
// national blocks has no place in UBL.
//
// When the specification identifier written is the one of the Croatian
// CIUS (see invoice.URNCIUSHR), the Croatian data is written too, as Parse
// reads it: the UBL extension hrextac:HRFISK20Data, the copy indicator when
// true, the issue time, the operator of the seller, the description of each
// preceding invoice, the HR VAT category label and the exemption reason of
// the document level allowances and charges and of the items, and the
// purchase order, despatch advice and receiving advice of each line, with
// the line identifier NA when the referenced line is not given.
//
// Mandatory elements of the binding are always written, empty when the
// invoice has no value for them, so that a validator reports the missing
// value. Amounts are written with the number of decimals they hold.
//
// The data that the binding cannot carry is omitted, or reported by a
// *LossError with MarshalOptions.Strict: the fields of the model that only
// EXTENDED has, the fields a party cannot have in its role, the BG-25 of an
// invoice without lines, and every extension, whatever the syntax it was read
// from.
func Marshal(inv *invoice.Invoice, opts *MarshalOptions) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("ubl: nil invoice")
	}
	var o MarshalOptions
	if opts != nil {
		o = *opts
	}
	w := writer{
		b:      make([]byte, 0, 3000+1500*len(inv.Lines)),
		indent: o.Indent,
		credit: IsCreditNote(inv.TypeCode),
		cur:    inv.Currency,
	}
	w.b = append(w.b, xmlDeclaration...)
	spec := o.CustomizationID
	if spec == "" {
		spec = customizationID(inv)
	}
	w.gaccount = strings.Contains(spec, invoice.GAccountExtension)
	w.italy = strings.Contains(spec, invoice.ItalianExtension)
	w.croatia = invoice.ProfileFromURN(spec) == invoice.ProfileCIUSHR
	w.document(inv, spec)
	if o.Indent {
		w.b = append(w.b, '\n')
	}
	for _, u := range inv.Unmodeled() {
		w.lose("EXT " + u)
	}
	if o.Strict {
		for _, t := range inv.NationalData() {
			if !w.carried[t] {
				w.losses = append(w.losses, t)
			}
		}
	}
	if o.Strict && len(w.losses) > 0 {
		return nil, &LossError{Terms: w.losses}
	}
	return w.b, nil
}

// defaultProcess returns the business process type (BT-23) written for an
// invoice of profile p that has none: the one of Peppol billing for Peppol
// and XRechnung, which require one (PEPPOL-EN16931-R001), the one of Peppol
// self-billing for Peppol Self-Billing, "" otherwise.
func defaultProcess(p invoice.Profile) string {
	switch p {
	case invoice.ProfilePeppol, invoice.ProfileXRechnung:
		return invoice.ProcessPeppolBilling
	case invoice.ProfilePeppolSelfBilling:
		return invoice.ProcessPeppolSelfBilling
	}
	return ""
}

// customizationID returns the specification identifier written by default.
func customizationID(inv *invoice.Invoice) string {
	if inv.SpecificationID != "" {
		switch invoice.DetectProfile(inv.SpecificationID) {
		case invoice.ProfileEN16931, invoice.ProfileXRechnung, invoice.ProfilePeppol, invoice.ProfilePeppolSelfBilling, invoice.ProfileNLCIUS, invoice.ProfileCIUSIT, invoice.ProfileCIUSPT, invoice.ProfileCIUSRO, invoice.ProfileCIUSHR, invoice.ProfileExtendedCTCFR, invoice.ProfileUnknown:
			return strings.TrimSpace(inv.SpecificationID)
		}
	}
	switch inv.Profile {
	case invoice.ProfileXRechnung, invoice.ProfilePeppol, invoice.ProfilePeppolSelfBilling, invoice.ProfileNLCIUS, invoice.ProfileCIUSIT, invoice.ProfileCIUSPT, invoice.ProfileCIUSRO, invoice.ProfileCIUSHR, invoice.ProfileExtendedCTCFR:
		return inv.Profile.URNFor(inv.Seller.Address.CountryCode)
	}
	return invoice.URNEN16931
}

func (w *writer) document(inv *invoice.Invoice, spec string) {
	root, ns, typeName, lineName := "Invoice", NamespaceInvoice, "cbc:InvoiceTypeCode", "cac:InvoiceLine"
	if w.credit {
		root, ns, typeName, lineName = "CreditNote", NamespaceCreditNote, "cbc:CreditNoteTypeCode", "cac:CreditNoteLine"
	}
	ext := false
	if w.italy {
		w.prepareItaly(inv)
		ext = w.hasItalianExtensions(inv)
	}
	hrExt := w.croatia && hasCroatianExtension(inv.Croatia)
	w.newline()
	w.b = append(w.b, `<`+root+` xmlns="`+ns+`" xmlns:cac="`+NamespaceCAC+`" xmlns:cbc="`+NamespaceCBC+`"`...)
	if ext || hrExt {
		w.b = append(w.b, ` xmlns:ext="`+NamespaceEXT+`"`...)
	}
	if hrExt {
		w.b = append(w.b, ` xmlns:hrextac="`+NamespaceHR+`"`...)
	}
	w.b = append(w.b, '>')
	w.depth++
	if ext {
		w.italianExtensions(inv)
	}
	if hrExt {
		w.croatianExtension(inv.Croatia)
		for _, f := range []string{"CashAccounting", "TaxTotal", "VATBreakdown", "TaxBasisTotal", "OutOfScopeAmount"} {
			w.carry("Croatia." + f)
		}
	}

	w.loseIf(inv.TestIndicator, "EXT TestIndicator")
	w.loseIf(inv.CopyIndicator && !w.croatia, "EXT CopyIndicator")
	w.loseText(inv.Language, "EXT Language")

	var terms *invoice.PaymentTerms
	if len(inv.PaymentTerms) > 0 {
		terms = &inv.PaymentTerms[0]
	}

	w.leafReq("cbc:CustomizationID", spec)
	process := inv.BusinessProcess
	if process == "" {
		process = defaultProcess(invoice.ProfileFromURN(spec))
	}
	w.leaf("cbc:ProfileID", process)
	w.leafReq("cbc:ID", inv.Number)
	if w.croatia && inv.CopyIndicator {
		w.leafReq("cbc:CopyIndicator", "true")
	}
	if inv.IssueDate.IsZero() {
		w.leafReq("cbc:IssueDate", "")
	} else {
		w.date("cbc:IssueDate", inv.IssueDate)
	}
	if w.croatia && inv.Croatia != nil {
		w.leaf("cbc:IssueTime", inv.Croatia.IssueTime)
		w.carry("Croatia.IssueTime")
	}
	if !w.credit && terms != nil {
		w.date("cbc:DueDate", terms.DueDate)
	}
	if w.credit {
		w.date("cbc:TaxPointDate", inv.TaxPointDate)
	}
	w.leafReq(typeName, inv.TypeCode)
	for i := range inv.Notes {
		n := &inv.Notes[i]
		s := n.Content
		if n.SubjectCode != "" {
			s = "#" + n.SubjectCode + "#" + s
		}
		w.leafReq("cbc:Note", s)
		w.loseText(n.ContentCode, "EXT Notes.ContentCode")
	}
	if !w.credit {
		w.date("cbc:TaxPointDate", inv.TaxPointDate)
	}
	w.leafReq("cbc:DocumentCurrencyCode", inv.Currency)
	w.leaf("cbc:TaxCurrencyCode", inv.TaxCurrency)
	w.leaf("cbc:AccountingCost", inv.BuyerAccountingReference)
	w.leaf("cbc:BuyerReference", inv.BuyerReference)

	if p := &inv.InvoicingPeriod; !p.Start.IsZero() || !p.End.IsZero() || inv.TaxPointDateCode != "" {
		w.start("cac:InvoicePeriod")
		w.date("cbc:StartDate", p.Start)
		w.date("cbc:EndDate", p.End)
		w.leaf("cbc:DescriptionCode", taxPointToUBL(inv.TaxPointDateCode))
		w.end("cac:InvoicePeriod")
	}
	w.loseText(inv.InvoicingPeriod.Description, "EXT InvoicingPeriod.Description")

	orderID, orderDate := inv.PurchaseOrderReference, invoice.Date{}
	if w.italy {
		orderID, orderDate = w.headerOrder(inv)
	}
	if orderID != "" || inv.SalesOrderReference != "" {
		w.start("cac:OrderReference")
		w.leafReq("cbc:ID", orderID)
		w.leaf("cbc:SalesOrderID", inv.SalesOrderReference)
		w.date("cbc:IssueDate", orderDate)
		w.end("cac:OrderReference")
	}
	for i := range inv.PrecedingInvoices {
		d := &inv.PrecedingInvoices[i]
		w.start("cac:BillingReference")
		w.start("cac:InvoiceDocumentReference")
		w.leafReq("cbc:ID", d.ID)
		w.date("cbc:IssueDate", d.IssueDate)
		if w.croatia && d.Croatia != nil {
			w.leaf("cbc:DocumentDescription", d.Croatia.Description)
			w.carryAll("PrecedingInvoices["+strconv.Itoa(i)+"].Croatia", d.Croatia)
		}
		w.end("cac:InvoiceDocumentReference")
		if i == 0 && w.italy && w.it != nil && w.it.MainInvoice != nil {
			w.mainInvoice()
		}
		w.end("cac:BillingReference")
		w.loseText(d.TypeCode, "EXT PrecedingInvoices.TypeCode")
		w.loseText(d.LineID, "EXT PrecedingInvoices.LineID")
	}
	despatch, despatchDate := inv.DespatchAdviceReference, invoice.Date{}
	if w.italy {
		despatch, despatchDate = w.headerDespatch(inv)
	}
	if despatch != "" {
		w.start("cac:DespatchDocumentReference")
		w.leafReq("cbc:ID", despatch)
		w.date("cbc:IssueDate", despatchDate)
		w.end("cac:DespatchDocumentReference")
	}
	w.docRef("cac:ReceiptDocumentReference", inv.ReceivingAdviceReference)
	tender := inv.TenderReference()
	if !w.credit {
		w.docRef("cac:OriginatorDocumentReference", tender)
	}
	w.docRef("cac:ContractDocumentReference", inv.ContractReference)
	tenderSeen := false
	for i := range inv.AdditionalDocuments {
		d := &inv.AdditionalDocuments[i]
		if d.TypeCode == "50" && !tenderSeen {
			tenderSeen = true // written as cac:OriginatorDocumentReference
			w.loseText(d.Description, "BT-123")
			continue
		}
		w.additionalDocument(d)
	}
	if w.credit {
		if inv.ProjectReference != "" {
			w.start("cac:AdditionalDocumentReference")
			w.leafReq("cbc:ID", inv.ProjectReference)
			w.leafReq("cbc:DocumentTypeCode", "50")
			w.end("cac:AdditionalDocumentReference")
		}
		w.docRef("cac:OriginatorDocumentReference", tender)
	} else {
		w.docRef("cac:ProjectReference", inv.ProjectReference)
	}
	w.loseText(inv.ProjectName, "EXT ProjectName")

	w.start("cac:AccountingSupplierParty")
	w.party("cac:Party", &inv.Seller, roleSeller, inv.CreditorReferenceID)
	if w.croatia {
		w.sellerContact(inv.Croatia)
		w.carry("Croatia.OperatorOIB")
		w.carry("Croatia.OperatorName")
	}
	w.end("cac:AccountingSupplierParty")
	w.start("cac:AccountingCustomerParty")
	w.party("cac:Party", &inv.Buyer, roleBuyer, "")
	w.end("cac:AccountingCustomerParty")
	if !inv.Payee.IsZero() {
		w.party("cac:PayeeParty", inv.Payee, rolePayee, "")
	}
	if !inv.SellerTaxRepresentative.IsZero() {
		w.party("cac:TaxRepresentativeParty", inv.SellerTaxRepresentative, roleTaxRep, "")
	}
	for _, x := range []struct {
		p    *invoice.Party
		name string
	}{
		{inv.SalesAgent, "SalesAgent"}, {inv.BuyerAgent, "BuyerAgent"}, {inv.Invoicer, "Invoicer"},
		{inv.Invoicee, "Invoicee"}, {inv.Payer, "Payer"}, {inv.ProductEndUser, "ProductEndUser"},
		{inv.UltimateShipTo, "UltimateShipTo"}, {inv.ShipFrom, "ShipFrom"},
	} {
		w.loseIf(!x.p.IsZero(), "EXT "+x.name)
	}
	w.loseIf(inv.DeliveryTerms != nil, "EXT DeliveryTerms")
	w.loseIf(len(inv.UltimateCustomerOrders) > 0, "EXT UltimateCustomerOrders")
	w.loseIf(!inv.DeliveryNote.IsZero(), "EXT DeliveryNote")
	w.loseIf(inv.CurrencyExchange != nil, "EXT CurrencyExchange")
	w.loseIf(len(inv.ThirdPartyCharges) > 0, "BG-34")

	var transport *invoice.Transport
	if w.italy {
		transport = w.deliveryTransport()
	}
	if !inv.DeliveryDate.IsZero() || !inv.ShipTo.IsZero() || transport != nil {
		w.delivery(inv.DeliveryDate, inv.ShipTo, transport)
	}
	if w.italy {
		w.deliveryTermsIT()
	}
	w.paymentMeans(inv, terms)
	w.paymentTerms(inv, terms)
	for i := range inv.AllowancesCharges {
		ac := &inv.AllowancesCharges[i]
		w.allowanceCharge(ac, true)
		if w.croatia && ac.Croatia != nil {
			w.carryAll("AllowancesCharges["+strconv.Itoa(i)+"].Croatia", ac.Croatia)
		}
	}
	for i := range inv.LogisticsCharges {
		c := &inv.LogisticsCharges[i]
		w.allowanceCharge(&invoice.AllowanceCharge{
			Charge: true, Amount: c.Amount, Reason: c.Description,
			VATCategoryCode: c.VATCategoryCode, VATRate: c.VATRate,
		}, true)
	}
	w.taxTotals(inv)
	if w.italy && w.it != nil && len(w.it.Withholdings) > 0 && !w.credit {
		w.withholdings(w.it.Withholdings)
	}
	w.totals(&inv.Totals)

	if len(inv.Lines) == 0 {
		w.lose("BG-25")
	}
	for i := range inv.Lines {
		l := &inv.Lines[i]
		w.line(lineName, l)
		if w.croatia && l.Croatia != nil {
			w.carryAll("Lines["+strconv.Itoa(i)+"].Croatia", l.Croatia)
		}
	}
	w.depth--
	w.newline()
	w.b = append(w.b, `</`+root+`>`...)
}

// docRef writes a document reference holding only an identifier.
func (w *writer) docRef(name, id string) {
	if id == "" {
		return
	}
	w.start(name)
	w.leafReq("cbc:ID", id)
	w.end(name)
}

func (w *writer) additionalDocument(d *invoice.AdditionalDocument) {
	w.start("cac:AdditionalDocumentReference")
	object := d.TypeCode == "130"
	if object {
		w.ident("cbc:ID", invoice.Identifier{ID: d.ID, Scheme: d.ReferenceTypeCode})
		if d.ID == "" {
			w.leafReq("cbc:ID", "")
		}
		w.leafReq("cbc:DocumentTypeCode", "130")
	} else {
		w.leafReq("cbc:ID", d.ID)
		w.loseText(d.ReferenceTypeCode, "AdditionalDocument.ReferenceTypeCode (TypeCode 130 only)")
	}
	w.leaf("cbc:DocumentDescription", d.Description)
	binary := len(d.Attachment) > 0 || d.AttachmentMimeCode != "" || d.AttachmentFilename != ""
	if binary || d.URI != "" {
		w.start("cac:Attachment")
		if binary {
			w.binary("cbc:EmbeddedDocumentBinaryObject", d.Attachment, d.AttachmentMimeCode, d.AttachmentFilename)
		}
		if d.URI != "" {
			w.start("cac:ExternalReference")
			w.leafReq("cbc:URI", d.URI)
			w.end("cac:ExternalReference")
		}
		w.end("cac:Attachment")
	}
	w.end("cac:AdditionalDocumentReference")
	w.loseIf(!d.IssueDate.IsZero(), "EXT AdditionalDocument.IssueDate")
	if t := d.TypeCode; t != "" && t != "916" && !object {
		w.lose("EXT AdditionalDocument.TypeCode")
	}
}

// Party fields, used to name the losses.
const (
	fIDs = iota
	fName
	fDescription
	fLegal
	fTrading
	fContact
	fAddress
	fURI
	fVAT
	fFC
	numPartyFields
)

var roleNames = [...]string{roleSeller: "Seller", roleBuyer: "Buyer", rolePayee: "Payee", roleTaxRep: "SellerTaxRepresentative", roleShipTo: "ShipTo"}

var partyFieldNames = [numPartyFields]string{
	"IDs", "Name", "Description", "LegalRegistration", "TradingName",
	"Contact", "Address", "ElectronicAddress", "VATID", "TaxRegistrationID",
}

// partyTerms gives, for each role and field, the business term reported when
// UBL cannot carry the field for the role. Fields without a business term for
// a role are reported as "EXT <Role>.<Field>".
var partyTerms = func() (t [len(roleNames)][numPartyFields]string) {
	for r := range roleNames {
		for f := range partyFieldNames {
			t[r][f] = "EXT " + roleNames[r] + "." + partyFieldNames[f]
		}
	}
	set := func(r role, terms ...string) {
		for i, s := range terms {
			if s != "" {
				t[r][i] = s
			}
		}
	}
	set(rolePayee, "BT-60", "BT-59", "", "BT-61")
	set(roleTaxRep, "", "BT-62", "", "", "", "", "BG-12", "", "BT-63")
	set(roleShipTo, "BT-71", "BT-70", "", "", "", "", "BG-15")
	return t
}()

// party writes a cac:PartyType element with the fields that the role
// carries in the binding. sepa is the bank assigned creditor identifier
// (BT-90), written as a party identifier of the seller.
func (w *writer) party(name string, p *invoice.Party, ro role, sepa string) {
	terms := &partyTerms[ro]
	main := ro == roleSeller || ro == roleBuyer
	w.start(name)
	if !p.ElectronicAddress.IsZero() {
		if main {
			w.leafAttr("cbc:EndpointID", "schemeID", p.ElectronicAddress.Scheme, p.ElectronicAddress.ID)
		} else {
			w.lose(terms[fURI])
		}
	}
	ids := p.IDs
	italianRep := ro == roleTaxRep && w.italy // carries its tax code and its EORI code
	switch ro {
	case roleBuyer, rolePayee:
		if len(ids) > 1 {
			w.lose(terms[fIDs])
			ids = ids[:1]
		}
	case roleTaxRep:
		var eori []invoice.Identifier
		for _, id := range ids {
			if italianRep && id.Scheme == "" && strings.HasPrefix(id.ID, "EORI:") {
				eori = append(eori, id)
			} else {
				w.lose(terms[fIDs])
			}
		}
		ids = eori
		if italianRep && p.LegalRegistration.Scheme == schemeCF && p.LegalRegistration.ID != "" {
			ids = append([]invoice.Identifier{p.LegalRegistration}, ids...)
		}
	}
	for _, id := range ids {
		w.start("cac:PartyIdentification")
		w.leafAttr("cbc:ID", "schemeID", id.Scheme, id.ID)
		w.end("cac:PartyIdentification")
	}
	if sepa != "" {
		w.start("cac:PartyIdentification")
		w.leafAttr("cbc:ID", "schemeID", "SEPA", sepa)
		w.end("cac:PartyIdentification")
	}
	partyName := p.TradingName
	if !main {
		partyName = p.Name
		w.loseText(p.TradingName, terms[fTrading])
	}
	if partyName != "" {
		w.start("cac:PartyName")
		w.leafReq("cbc:Name", partyName)
		w.end("cac:PartyName")
	}
	if ro == rolePayee {
		w.loseIf(!p.Address.IsZero(), terms[fAddress])
	} else if main || !p.Address.IsZero() {
		w.address("cac:PostalAddress", &p.Address)
	}
	if p.VATID != "" {
		if ro != rolePayee {
			level := ""
			if ro == roleSeller && w.italy && p.Italy != nil && p.Italy.TaxRegime != "" {
				level = p.Italy.TaxRegime
				w.carry("Seller.Italy.TaxRegime")
			}
			w.taxScheme(p.VATID, "VAT", level)
		} else {
			w.lose(terms[fVAT])
		}
	}
	if p.TaxRegistrationID != "" {
		if ro == roleSeller {
			w.taxScheme(p.TaxRegistrationID, "FC", "")
		} else {
			w.lose(terms[fFC])
		}
	}
	legal := !p.LegalRegistration.IsZero()
	if legal && ro == roleTaxRep {
		if !italianRep || p.LegalRegistration.Scheme != schemeCF || p.LegalRegistration.ID == "" {
			w.lose(terms[fLegal])
		}
		legal = false
	}
	desc := p.Description != ""
	if desc && ro != roleSeller {
		w.lose(terms[fDescription])
		desc = false
	}
	if main || legal || desc {
		w.start("cac:PartyLegalEntity")
		if main {
			w.leafReq("cbc:RegistrationName", p.Name)
		}
		if legal {
			w.leafAttr("cbc:CompanyID", "schemeID", p.LegalRegistration.Scheme, p.LegalRegistration.ID)
		}
		if desc {
			w.leafReq("cbc:CompanyLegalForm", p.Description)
		}
		w.end("cac:PartyLegalEntity")
	}
	if c := p.Contact; c != nil && (c.PersonName != "" || c.DepartmentName != "" || c.Phone != "" || c.Email != "") {
		if main {
			w.start("cac:Contact")
			name := c.PersonName
			if name == "" {
				name = c.DepartmentName
			} else {
				w.loseText(c.DepartmentName, "EXT "+roleNames[ro]+".Contact.DepartmentName")
			}
			w.leaf("cbc:Name", name)
			w.leaf("cbc:Telephone", c.Phone)
			w.leaf("cbc:ElectronicMail", c.Email)
			w.end("cac:Contact")
		} else {
			w.lose(terms[fContact])
		}
	}
	if ro == roleSeller && w.italy {
		w.sellerItaly(p)
	}
	w.end(name)
}

// taxScheme writes a cac:PartyTaxScheme with its tax level code, the tax
// regime of an Italian seller.
func (w *writer) taxScheme(id, scheme, level string) {
	w.start("cac:PartyTaxScheme")
	w.leafReq("cbc:CompanyID", id)
	w.leaf("cbc:TaxLevelCode", level)
	w.start("cac:TaxScheme")
	w.leafReq("cbc:ID", scheme)
	w.end("cac:TaxScheme")
	w.end("cac:PartyTaxScheme")
}

func (w *writer) address(name string, a *invoice.Address) {
	w.start(name)
	w.leaf("cbc:StreetName", a.Line1)
	w.leaf("cbc:AdditionalStreetName", a.Line2)
	w.leaf("cbc:CityName", a.City)
	w.leaf("cbc:PostalZone", a.PostCode)
	w.leaf("cbc:CountrySubentity", a.CountrySubdivision)
	if a.Line3 != "" {
		w.start("cac:AddressLine")
		w.leafReq("cbc:Line", a.Line3)
		w.end("cac:AddressLine")
	}
	if a.CountryCode != "" {
		w.start("cac:Country")
		w.leafReq("cbc:IdentificationCode", a.CountryCode)
		w.end("cac:Country")
	}
	w.end(name)
}

// delivery writes cac:Delivery: the actual delivery date (BT-72) and the
// deliver-to party (BG-13), and under EXT-IT the carrier, the pickup and the
// shipment of t when it is not nil.
func (w *writer) delivery(date invoice.Date, ship *invoice.Party, t *invoice.Transport) {
	w.start("cac:Delivery")
	w.date("cbc:ActualDeliveryDate", date)
	terms := &partyTerms[roleShipTo]
	if !ship.IsZero() {
		ids := ship.IDs
		if len(ids) > 1 {
			w.lose(terms[fIDs])
			ids = ids[:1]
		}
		if len(ids) > 0 || !ship.Address.IsZero() {
			w.start("cac:DeliveryLocation")
			if len(ids) > 0 {
				w.leafAttr("cbc:ID", "schemeID", ids[0].Scheme, ids[0].ID)
			}
			if !ship.Address.IsZero() {
				w.address("cac:Address", &ship.Address)
			}
			w.end("cac:DeliveryLocation")
		}
	}
	if t != nil {
		w.carrier(t)
	}
	if !ship.IsZero() {
		if ship.Name != "" {
			w.start("cac:DeliveryParty")
			w.start("cac:PartyName")
			w.leafReq("cbc:Name", ship.Name)
			w.end("cac:PartyName")
			w.end("cac:DeliveryParty")
		}
		w.loseText(ship.Description, terms[fDescription])
		w.loseIf(!ship.LegalRegistration.IsZero(), terms[fLegal])
		w.loseText(ship.TradingName, terms[fTrading])
		w.loseIf(ship.Contact != nil && *ship.Contact != (invoice.Contact{}), terms[fContact])
		w.loseIf(!ship.ElectronicAddress.IsZero(), terms[fURI])
		w.loseText(ship.VATID, terms[fVAT])
		w.loseText(ship.TaxRegistrationID, terms[fFC])
	}
	if t != nil {
		w.despatch(t)
	}
	w.end("cac:Delivery")
}

// paymentMeans writes the payment instructions (BG-16). The remittance
// information (BT-83) is repeated in each of them, the mandate reference
// (BT-89) goes to the direct debit, and in a credit note the payment due date
// (BT-9) goes to the first one. With the G-account extension, each one
// carries its identifier.
func (w *writer) paymentMeans(inv *invoice.Invoice, terms *invoice.PaymentTerms) {
	var due invoice.Date
	var mandate string
	if terms != nil {
		due, mandate = terms.DueDate, terms.DirectDebitMandateID
	}
	if !w.credit {
		due = invoice.Date{}
	}
	mandateAt := -1
	if mandate != "" {
		for i := range inv.PaymentMeans {
			if c := inv.PaymentMeans[i].TypeCode; c == invoice.PaymentDirectDebit || c == invoice.PaymentSEPADirectDebit {
				mandateAt = i
				break
			}
		}
		if mandateAt < 0 {
			for i := range inv.PaymentMeans {
				if inv.PaymentMeans[i].PayerIBAN != "" {
					mandateAt = i
					break
				}
			}
		}
		if mandateAt < 0 && len(inv.PaymentMeans) > 0 {
			mandateAt = 0
		}
	}
	if len(inv.PaymentMeans) == 0 {
		w.loseIf(!due.IsZero(), "BT-9")
		w.loseText(mandate, "BT-89")
		w.loseText(inv.RemittanceInformation, "BT-83")
		return
	}
	for i := range inv.PaymentMeans {
		pm := &inv.PaymentMeans[i]
		w.start("cac:PaymentMeans")
		if w.gaccount {
			w.leaf("cbc:ID", pm.ID)
		} else {
			w.loseText(pm.ID, "EXT PaymentMeans.ID")
		}
		w.leafAttr("cbc:PaymentMeansCode", "name", pm.Information, pm.TypeCode)
		if pm.TypeCode == "" {
			w.leafReq("cbc:PaymentMeansCode", "")
		}
		if i == 0 {
			w.date("cbc:PaymentDueDate", due)
		}
		w.leaf("cbc:PaymentID", inv.RemittanceInformation)
		if pm.CardID != "" || pm.CardholderName != "" {
			w.start("cac:CardAccount")
			w.leafReq("cbc:PrimaryAccountNumberID", pm.CardID)
			w.leafReq("cbc:NetworkID", "NA")
			w.leaf("cbc:HolderName", pm.CardholderName)
			w.end("cac:CardAccount")
		}
		account := pm.PayeeIBAN
		if account == "" {
			account = pm.PayeeProprietaryID
		} else {
			w.loseText(pm.PayeeProprietaryID, "BT-84")
		}
		if account != "" || pm.PayeeAccountName != "" || pm.PayeeBIC != "" {
			w.start("cac:PayeeFinancialAccount")
			w.leafReq("cbc:ID", account)
			w.leaf("cbc:Name", pm.PayeeAccountName)
			if pm.PayeeBIC != "" {
				w.start("cac:FinancialInstitutionBranch")
				w.leafReq("cbc:ID", pm.PayeeBIC)
				w.end("cac:FinancialInstitutionBranch")
			}
			w.end("cac:PayeeFinancialAccount")
		}
		if i == mandateAt || pm.PayerIBAN != "" {
			w.start("cac:PaymentMandate")
			if i == mandateAt {
				w.leafReq("cbc:ID", mandate)
			}
			if pm.PayerIBAN != "" {
				w.start("cac:PayerFinancialAccount")
				w.leafReq("cbc:ID", pm.PayerIBAN)
				w.end("cac:PayerFinancialAccount")
			}
			w.end("cac:PaymentMandate")
		}
		w.end("cac:PaymentMeans")
		w.loseText(pm.PayerAccountName, "BT-216")
		w.loseText(pm.PayerBIC, "BT-215")
	}
}

// paymentTerms writes the payment terms (BT-20), first is the first entry of
// inv.PaymentTerms. With the G-account extension, each entry that holds
// more than a due date or a mandate reference is written, with its payment
// means reference and its amount.
func (w *writer) paymentTerms(inv *invoice.Invoice, first *invoice.PaymentTerms) {
	if !w.gaccount {
		pit := w.earlyPaymentOf(inv)
		if first != nil && first.Description != "" || pit != nil {
			w.start("cac:PaymentTerms")
			if first != nil {
				w.leaf("cbc:Note", first.Description)
			}
			if pit != nil {
				w.earlyPayment(pit)
			}
			w.end("cac:PaymentTerms")
		}
		if first != nil {
			w.loseText(first.PaymentMeansID, "EXT PaymentTerms.PaymentMeansID")
			w.loseIf(first.PartialAmount.IsSet(), "EXT PaymentTerms.PartialAmount")
		}
		if len(inv.PaymentTerms) > 1 {
			w.lose("EXT PaymentTerms")
		}
		return
	}
	for i := range inv.PaymentTerms {
		t := &inv.PaymentTerms[i]
		if t.Description != "" || t.PaymentMeansID != "" || t.PartialAmount.IsSet() {
			w.start("cac:PaymentTerms")
			w.leaf("cbc:PaymentMeansID", t.PaymentMeansID)
			w.leaf("cbc:Note", t.Description)
			w.amount("cbc:Amount", t.PartialAmount)
			w.end("cac:PaymentTerms")
		}
		if i > 0 {
			w.loseIf(!t.DueDate.IsZero(), "EXT PaymentTerms.DueDate")
			w.loseText(t.DirectDebitMandateID, "EXT PaymentTerms.DirectDebitMandateID")
		}
	}
}

// allowanceCharge writes a cac:AllowanceCharge; vat tells whether it is at
// document level, where it carries a tax category.
func (w *writer) allowanceCharge(ac *invoice.AllowanceCharge, vat bool) {
	w.start("cac:AllowanceCharge")
	w.leafReq("cbc:ChargeIndicator", strconv.FormatBool(ac.Charge))
	w.leaf("cbc:AllowanceChargeReasonCode", ac.ReasonCode)
	w.leaf("cbc:AllowanceChargeReason", ac.Reason)
	w.dec("cbc:MultiplierFactorNumeric", ac.Percentage)
	w.amountReq("cbc:Amount", ac.Amount, w.cur)
	w.amount("cbc:BaseAmount", ac.BaseAmount)
	if vat {
		var hr *invoice.CroatiaTax
		if w.croatia {
			hr = ac.Croatia
		}
		if ac.VATCategoryCode != "" || ac.VATRate.IsSet() || hr != nil {
			w.taxCategory("cac:TaxCategory", ac.VATCategoryCode, ac.VATRate, nil, hr)
		}
	} else if ac.VATCategoryCode != "" || ac.VATRate.IsSet() {
		w.lose("EXT Line.AllowanceCharge.VAT")
	}
	w.end("cac:AllowanceCharge")
}

// taxCategory writes a cac:TaxCategory or cac:ClassifiedTaxCategory in the
// VAT scheme, with the exemption reason of vb when vb is not nil.
func (w *writer) taxCategory(name, code string, rate invoice.Decimal, vb *invoice.VATBreakdown, hr *invoice.CroatiaTax) {
	w.start(name)
	w.leafReq("cbc:ID", code)
	if hr != nil {
		w.leaf("cbc:Name", hr.Category)
	}
	w.dec("cbc:Percent", rate)
	switch {
	case vb != nil:
		w.leaf("cbc:TaxExemptionReasonCode", vb.ExemptionReasonCode)
		w.leaf("cbc:TaxExemptionReason", vb.ExemptionReason)
	case hr != nil:
		w.leaf("cbc:TaxExemptionReasonCode", hr.ExemptionReasonCode)
		w.leaf("cbc:TaxExemptionReason", hr.ExemptionReason)
	}
	w.start("cac:TaxScheme")
	w.leafReq("cbc:ID", "VAT")
	w.end("cac:TaxScheme")
	w.end(name)
}

// taxTotals writes the invoice total VAT amount (BT-110) with the VAT
// breakdown (BG-23), and the total VAT amount in accounting currency
// (BT-111) in a second cac:TaxTotal. The schema requires an amount in each
// cac:TaxTotal: when BT-110 is not set, the sum of the VAT category tax
// amounts (BT-117) is written, as BR-CO-14 requires.
func (w *writer) taxTotals(inv *invoice.Invoice) {
	t := &inv.Totals
	if t.TaxTotal.IsSet() || len(inv.VATBreakdown) > 0 {
		total := t.TaxTotal
		if !total.IsSet() {
			total = invoice.NewDecimal(0, 2)
			for i := range inv.VATBreakdown {
				total = total.Add(inv.VATBreakdown[i].TaxAmount)
			}
		}
		w.start("cac:TaxTotal")
		w.amountReq("cbc:TaxAmount", total, w.cur)
		for i := range inv.VATBreakdown {
			vb := &inv.VATBreakdown[i]
			w.start("cac:TaxSubtotal")
			w.amountReq("cbc:TaxableAmount", vb.TaxableAmount, w.cur)
			w.amountReq("cbc:TaxAmount", vb.TaxAmount, w.cur)
			w.taxCategory("cac:TaxCategory", vb.CategoryCode, vb.Rate, vb, nil)
			w.end("cac:TaxSubtotal")
		}
		w.end("cac:TaxTotal")
	}
	if t.TaxTotalInTaxCurrency.IsSet() {
		cur := inv.TaxCurrency
		w.start("cac:TaxTotal")
		w.amountReq("cbc:TaxAmount", t.TaxTotalInTaxCurrency, cur)
		w.end("cac:TaxTotal")
	}
}

func (w *writer) totals(t *invoice.Totals) {
	w.start("cac:LegalMonetaryTotal")
	w.amount("cbc:LineExtensionAmount", t.LineTotal)
	w.amount("cbc:TaxExclusiveAmount", t.TaxBasisTotal)
	w.amount("cbc:TaxInclusiveAmount", t.GrandTotal)
	w.amount("cbc:AllowanceTotalAmount", t.AllowanceTotal)
	w.amount("cbc:ChargeTotalAmount", t.ChargeTotal)
	w.amount("cbc:PrepaidAmount", t.PaidAmount)
	w.amount("cbc:PayableRoundingAmount", t.RoundingAmount)
	w.amountReq("cbc:PayableAmount", t.DuePayable, w.cur)
	w.end("cac:LegalMonetaryTotal")
}

func (w *writer) line(name string, l *invoice.Line) {
	w.start(name)
	w.leafReq("cbc:ID", l.ID)
	w.leaf("cbc:Note", l.Note)
	qty := "cbc:InvoicedQuantity"
	if w.credit {
		qty = "cbc:CreditedQuantity"
	}
	w.quantity(qty, l.Quantity, l.UnitCode)
	w.amountReq("cbc:LineExtensionAmount", l.NetAmount, w.cur)
	w.leaf("cbc:AccountingCost", l.AccountingReference)
	if !l.Period.Start.IsZero() || !l.Period.End.IsZero() {
		w.start("cac:InvoicePeriod")
		w.date("cbc:StartDate", l.Period.Start)
		w.date("cbc:EndDate", l.Period.End)
		w.end("cac:InvoicePeriod")
	}
	w.loseText(l.Period.Description, "EXT Line.Period.Description")
	if w.croatia {
		w.lineReferencesHR(l)
	} else {
		w.lineReferences(l)
	}
	if !l.ObjectIdentifier.IsZero() {
		w.start("cac:DocumentReference")
		w.leafAttr("cbc:ID", "schemeID", l.ObjectIdentifier.Scheme, l.ObjectIdentifier.ID)
		if l.ObjectIdentifier.ID == "" {
			w.leafReq("cbc:ID", "")
		}
		w.leafReq("cbc:DocumentTypeCode", "130")
		w.end("cac:DocumentReference")
	}
	for i := range l.AllowancesCharges {
		w.allowanceCharge(&l.AllowancesCharges[i], false)
	}
	w.item(l)
	w.price(l)
	w.end(name)

	w.loseText(l.ParentID, "EXT Line.ParentID")
	w.loseText(l.StatusCode, "EXT Line.StatusCode")
	w.loseText(l.StatusReasonCode, "EXT Line.StatusReasonCode")
	w.loseIf(l.DeliveryTerms != nil, "EXT Line.DeliveryTerms")
	if !w.croatia {
		w.loseText(l.OrderReference, "EXT Line.OrderReference")
	}
	w.loseIf(len(l.UltimateCustomerOrders) > 0, "EXT Line.UltimateCustomerOrders")
	w.loseIf(!l.ShipTo.IsZero(), "EXT Line.ShipTo")
	w.loseIf(!l.UltimateShipTo.IsZero(), "EXT Line.UltimateShipTo")
	w.loseIf(!l.DeliveryDate.IsZero(), "EXT Line.DeliveryDate")
	w.loseIf(!l.DeliveryNote.IsZero(), "EXT Line.DeliveryNote")
	w.loseIf(!l.InvoiceReference.IsZero(), "EXT Line.InvoiceReference")
}

func (w *writer) item(l *invoice.Line) {
	it := &l.Item
	w.start("cac:Item")
	w.leaf("cbc:Description", it.Description)
	w.leafReq("cbc:Name", it.Name)
	if it.BuyerID != "" {
		w.start("cac:BuyersItemIdentification")
		w.leafReq("cbc:ID", it.BuyerID)
		w.end("cac:BuyersItemIdentification")
	}
	if it.SellerID != "" {
		w.start("cac:SellersItemIdentification")
		w.leafReq("cbc:ID", it.SellerID)
		w.end("cac:SellersItemIdentification")
	}
	if !it.GlobalID.IsZero() {
		w.start("cac:StandardItemIdentification")
		w.leafAttr("cbc:ID", "schemeID", it.GlobalID.Scheme, it.GlobalID.ID)
		if it.GlobalID.ID == "" {
			w.leafReq("cbc:ID", "")
		}
		w.end("cac:StandardItemIdentification")
	}
	if it.OriginCountry != "" {
		w.start("cac:OriginCountry")
		w.leafReq("cbc:IdentificationCode", it.OriginCountry)
		w.end("cac:OriginCountry")
	}
	for i := range it.Classifications {
		c := &it.Classifications[i]
		w.start("cac:CommodityClassification")
		w.open("cbc:ItemClassificationCode")
		if c.ListID != "" {
			w.attr("listID", c.ListID)
		}
		if c.ListVersionID != "" {
			w.attr("listVersionID", c.ListVersionID)
		}
		w.close("cbc:ItemClassificationCode", c.Code)
		w.end("cac:CommodityClassification")
		w.loseText(c.Name, "EXT Item.Classifications.Name")
	}
	var hr *invoice.CroatiaTax
	if c := l.Croatia; w.croatia && c != nil && (c.Category != "" || c.ExemptionReason != "" || c.ExemptionReasonCode != "") {
		hr = &invoice.CroatiaTax{Category: c.Category, ExemptionReason: c.ExemptionReason, ExemptionReasonCode: c.ExemptionReasonCode}
	}
	w.taxCategory("cac:ClassifiedTaxCategory", l.VATCategoryCode, l.VATRate, nil, hr)
	for i := range it.Attributes {
		a := &it.Attributes[i]
		w.start("cac:AdditionalItemProperty")
		w.leafReq("cbc:Name", a.Name)
		w.leafReq("cbc:Value", a.Value)
		w.end("cac:AdditionalItemProperty")
		w.loseText(a.TypeCode, "EXT Item.Attributes.TypeCode")
	}
	w.end("cac:Item")
	w.loseIf(len(it.ReferencedProducts) > 0, "EXT Item.ReferencedProducts")
}

// price writes cac:Price: the net price (BT-146), the base quantity
// (BT-149, BT-150) and, when the gross price (BT-148) or the price discount
// (BT-147) is known, an allowance carrying them. The schema requires the
// amount of that allowance: without BT-147 it is the gross price minus the
// net price.
func (w *writer) price(l *invoice.Line) {
	w.start("cac:Price")
	w.amountReq("cbc:PriceAmount", l.NetPrice, w.cur)
	w.quantity("cbc:BaseQuantity", l.PriceBaseQuantity, l.PriceBaseQuantityUnit)
	if l.GrossPrice.IsSet() || l.PriceDiscount.IsSet() {
		discount := l.PriceDiscount
		if !discount.IsSet() && l.NetPrice.IsSet() {
			discount = l.GrossPrice.Sub(l.NetPrice)
		}
		w.start("cac:AllowanceCharge")
		w.leafReq("cbc:ChargeIndicator", "false")
		w.amountReq("cbc:Amount", discount, w.cur)
		w.amount("cbc:BaseAmount", l.GrossPrice)
		w.end("cac:AllowanceCharge")
	}
	w.end("cac:Price")
	w.loseIf(len(l.PriceAllowancesCharges) > 0, "EXT Line.PriceAllowancesCharges")
}
