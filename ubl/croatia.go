package ubl

import (
	"reflect"
	"strconv"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// The Croatian CIUS and its extension (CIUS-HR and EXT-HR 2025).
//
// An invoice whose specification identifier (BT-24) is the Croatian one
// (see invoice.URNCIUSHR) carries data that EN 16931 does not have, which
// the Croatian specification (Specifikacija osnovne uporabe eRačuna s
// proširenjima, version 1.8) binds to elements of UBL 2.1 outside the syntax
// binding of EN 16931 and to the UBL extension HRFISK20Data. Parse reads
// them into the national blocks of the model (invoice.Croatia,
// DocumentReference.Croatia, AllowanceCharge.Croatia and Line.Croatia), the
// copy indicator (HR-BT-1) into CopyIndicator and the purchase order of an
// invoice line (HR-BT-9) into Line.OrderReference; Marshal writes them back.

// NamespaceHR is the namespace of the aggregates of the Croatian extension
// (hrextac).
const NamespaceHR = "urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"

// ---- Reading ----

// hr returns the national block of the invoice, created when needed.
func (r *reader) hr() *invoice.Croatia {
	if r.inv.Croatia == nil {
		r.inv.Croatia = new(invoice.Croatia)
	}
	return r.inv.Croatia
}

// lineCroatia returns the national block of l, created when needed.
func lineCroatia(l *invoice.Line) *invoice.CroatiaLine {
	if l.Croatia == nil {
		l.Croatia = new(invoice.CroatiaLine)
	}
	return l.Croatia
}

// indicator reads the indicator c ("true" or "false", "1" or "0").
func (r *reader) indicator(c *xmltree.Node) bool {
	switch s := r.text(c); s {
	case "true", "1":
		return true
	case "false", "0":
	default:
		r.issue(c, "invalid indicator "+strconv.Quote(s))
	}
	return false
}

// croatianExtensions reads the ext:UBLExtensions n of a CIUS-HR invoice: the
// extension that holds hrextac:HRFISK20Data is read into the national block,
// the other ones are kept as extensions.
func (r *reader) croatianExtensions(n *xmltree.Node) {
	r.enter(n)
	done := false
	for _, c := range n.Children {
		if done || c.Space != NamespaceEXT || c.Name != "UBLExtension" || len(c.Attrs) > 0 {
			r.unread(c)
			continue
		}
		content := child(c, "ExtensionContent")
		if len(c.Children) != 1 || content == nil || len(content.Attrs) > 0 || len(content.Children) != 1 {
			r.unread(c)
			continue
		}
		data := content.Children[0]
		if data.Space != NamespaceHR || data.Name != "HRFISK20Data" {
			r.unread(c)
			continue
		}
		done = true
		r.enter(c)
		r.enter(content)
		r.fisk20(data)
		r.leave()
		r.leave()
	}
	r.leave()
}

// fisk20 reads hrextac:HRFISK20Data: the indicator of cash accounting
// (HR-BT-15), the Croatian VAT breakdown (HR-BG-2) and totals (HR-BT-23 to
// HR-BT-25).
func (r *reader) fisk20(n *xmltree.Node) {
	hr := r.hr()
	r.enter(n)
	var cashDone, taxDone, totalDone bool
	for _, c := range n.Children {
		switch {
		case c.Space == NamespaceHR && c.Name == "HRObracunPDVPoNaplati" && !cashDone:
			cashDone = true
			hr.CashAccounting = r.text(c)
		case c.Space == NamespaceHR && c.Name == "HRTaxTotal" && !taxDone:
			taxDone = true
			r.enter(c)
			amtDone := false
			for _, k := range c.Children {
				switch {
				case k.Space == NamespaceCBC && k.Name == "TaxAmount" && !amtDone:
					amtDone = true
					hr.TaxTotal = r.decimal(k)
				case k.Space == NamespaceHR && k.Name == "HRTaxSubtotal":
					hr.VATBreakdown = append(hr.VATBreakdown, r.hrTaxSubtotal(k))
				default:
					r.unread(k)
				}
			}
			r.leave()
		case c.Space == NamespaceHR && c.Name == "HRLegalMonetaryTotal" && !totalDone:
			totalDone = true
			r.enter(c)
			var basisDone, outDone bool
			for _, k := range c.Children {
				switch {
				case k.Space == NamespaceCBC && k.Name == "TaxExclusiveAmount" && !basisDone:
					basisDone = true
					hr.TaxBasisTotal = r.decimal(k)
				case k.Space == NamespaceHR && k.Name == "OutOfScopeOfVATAmount" && !outDone:
					outDone = true
					hr.OutOfScopeAmount = r.decimal(k)
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
}

// hrTaxSubtotal reads an entry of the Croatian VAT breakdown
// (hrextac:HRTaxSubtotal).
func (r *reader) hrTaxSubtotal(n *xmltree.Node) invoice.CroatiaVAT {
	var v invoice.CroatiaVAT
	r.enter(n)
	var baseDone, amtDone, catDone bool
	for _, c := range n.Children {
		switch {
		case c.Space == NamespaceCBC && c.Name == "TaxableAmount" && !baseDone:
			baseDone = true
			v.TaxableAmount = r.decimal(c)
		case c.Space == NamespaceCBC && c.Name == "TaxAmount" && !amtDone:
			amtDone = true
			v.TaxAmount = r.decimal(c)
		case c.Space == NamespaceHR && c.Name == "HRTaxCategory" && !catDone:
			catDone = true
			r.hrTaxCategory(c, &v)
		default:
			r.unread(c)
		}
	}
	r.leave()
	return v
}

func (r *reader) hrTaxCategory(n *xmltree.Node, v *invoice.CroatiaVAT) {
	r.enter(n)
	var idDone, nameDone, rateDone, codeDone, reasonDone, schemeDone bool
	for _, c := range n.Children {
		cbc := c.Space == NamespaceCBC
		switch {
		case cbc && c.Name == "ID" && !idDone:
			idDone = true
			v.CategoryCode = r.text(c)
		case cbc && c.Name == "Name" && !nameDone:
			nameDone = true
			v.Category = r.text(c)
		case cbc && c.Name == "Percent" && !rateDone:
			rateDone = true
			v.Rate = r.decimal(c)
		case cbc && c.Name == "TaxExemptionReasonCode" && !codeDone:
			codeDone = true
			v.ExemptionReasonCode = r.text(c)
		case cbc && c.Name == "TaxExemptionReason" && !reasonDone:
			reasonDone = true
			v.ExemptionReason = r.text(c)
		case c.Space == NamespaceHR && c.Name == "HRTaxScheme" && !schemeDone:
			schemeDone = true
			r.enter(c)
			done := false
			for _, k := range c.Children {
				if k.Space == NamespaceCBC && k.Name == "ID" && !done {
					done = true
					v.TaxScheme = r.text(k)
					continue
				}
				r.unread(k)
			}
			r.leave()
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// sellerContact reads the cac:SellerContact of the seller: the OIB (HR-BT-5)
// and the label (HR-BT-4) of the operator who issued the invoice.
func (r *reader) sellerContact(n *xmltree.Node) {
	hr := r.hr()
	r.enter(n)
	var idDone, nameDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			hr.OperatorOIB = r.text(c)
		case c.Name == "Name" && !nameDone:
			nameDone = true
			hr.OperatorName = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// lineReference reads a cac:DespatchLineReference (HR-BT-10) or a
// cac:ReceiptLineReference (HR-BT-11) of an invoice line: the referenced
// line and the identifier and date of the document. A line identifier na
// stands for no line.
func (r *reader) lineReference(n *xmltree.Node) *invoice.DocumentReference {
	d := new(invoice.DocumentReference)
	r.enter(n)
	var lineDone, docDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "LineID" && !lineDone:
			lineDone = true
			if id := r.text(c); id != na {
				d.LineID = id
			}
		case c.Name == "DocumentReference" && !docDone:
			docDone = true
			r.enter(c)
			var idDone, dateDone bool
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && !idDone:
					idDone = true
					d.ID = r.text(k)
				case k.Name == "IssueDate" && !dateDone:
					dateDone = true
					d.IssueDate = r.date(k)
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
	return d
}

// orderLineHR reads the cac:OrderReference of the cac:OrderLineReference of
// an invoice line: the purchase order it references (HR-BT-9).
func (r *reader) orderLineHR(n *xmltree.Node, l *invoice.Line) {
	r.enter(n)
	done := false
	for _, c := range n.Children {
		if c.Name == "ID" && !done {
			done = true
			l.OrderReference = r.text(c)
			continue
		}
		r.unread(c)
	}
	r.leave()
}

// ---- Writing ----

// hasCroatianExtension reports whether c holds data of the extension
// HRFISK20Data.
func hasCroatianExtension(c *invoice.Croatia) bool {
	return c != nil && (c.CashAccounting != "" || c.TaxTotal.IsSet() || len(c.VATBreakdown) > 0 || c.TaxBasisTotal.IsSet() || c.OutOfScopeAmount.IsSet())
}

// carryAll records that the fields of the national block v, a pointer to a
// struct listed as prefix by invoice.Invoice.NationalData, are written.
func (w *writer) carryAll(prefix string, v any) {
	t := reflect.TypeOf(v).Elem()
	for i := range t.NumField() {
		w.carry(prefix + "." + t.Field(i).Name)
	}
}

// croatianExtension writes the ext:UBLExtensions of the Croatian extension:
// hrextac:HRFISK20Data with the indicator of cash accounting, the Croatian
// VAT breakdown and totals, in the order of its schema.
func (w *writer) croatianExtension(c *invoice.Croatia) {
	w.start("ext:UBLExtensions")
	w.start("ext:UBLExtension")
	w.start("ext:ExtensionContent")
	w.start("hrextac:HRFISK20Data")
	w.leaf("hrextac:HRObracunPDVPoNaplati", c.CashAccounting)
	if c.TaxTotal.IsSet() || len(c.VATBreakdown) > 0 {
		w.start("hrextac:HRTaxTotal")
		w.amountReq("cbc:TaxAmount", c.TaxTotal, w.cur)
		for i := range c.VATBreakdown {
			v := &c.VATBreakdown[i]
			w.start("hrextac:HRTaxSubtotal")
			w.amountReq("cbc:TaxableAmount", v.TaxableAmount, w.cur)
			w.amountReq("cbc:TaxAmount", v.TaxAmount, w.cur)
			w.start("hrextac:HRTaxCategory")
			w.leafReq("cbc:ID", v.CategoryCode)
			w.leaf("cbc:Name", v.Category)
			w.dec("cbc:Percent", v.Rate)
			w.leaf("cbc:TaxExemptionReasonCode", v.ExemptionReasonCode)
			w.leaf("cbc:TaxExemptionReason", v.ExemptionReason)
			w.start("hrextac:HRTaxScheme")
			w.leafReq("cbc:ID", v.TaxScheme)
			w.end("hrextac:HRTaxScheme")
			w.end("hrextac:HRTaxCategory")
			w.end("hrextac:HRTaxSubtotal")
		}
		w.end("hrextac:HRTaxTotal")
	}
	if c.TaxBasisTotal.IsSet() || c.OutOfScopeAmount.IsSet() {
		w.start("hrextac:HRLegalMonetaryTotal")
		w.amountReq("cbc:TaxExclusiveAmount", c.TaxBasisTotal, w.cur)
		w.amountReq("hrextac:OutOfScopeOfVATAmount", c.OutOfScopeAmount, w.cur)
		w.end("hrextac:HRLegalMonetaryTotal")
	}
	w.end("hrextac:HRFISK20Data")
	w.end("ext:ExtensionContent")
	w.end("ext:UBLExtension")
	w.end("ext:UBLExtensions")
}

// sellerContact writes the cac:SellerContact of the seller: the OIB and the
// label of the operator.
func (w *writer) sellerContact(c *invoice.Croatia) {
	if c == nil || c.OperatorOIB == "" && c.OperatorName == "" {
		return
	}
	w.start("cac:SellerContact")
	w.leaf("cbc:ID", c.OperatorOIB)
	w.leaf("cbc:Name", c.OperatorName)
	w.end("cac:SellerContact")
}

// lineReferencesHR writes the references of the invoice line l that the
// CIUS-HR adds: the purchase order (HR-BT-9) in its cac:OrderLineReference,
// the despatch advice (HR-BT-10) and the receiving advice (HR-BT-11), with
// the line they refer to (na for none).
func (w *writer) lineReferencesHR(l *invoice.Line) {
	if l.OrderLineReference != "" || l.OrderReference != "" {
		id := l.OrderLineReference
		if id == "" {
			id = na
		}
		w.start("cac:OrderLineReference")
		w.leafReq("cbc:LineID", id)
		if l.OrderReference != "" {
			w.start("cac:OrderReference")
			w.leafReq("cbc:ID", l.OrderReference)
			w.end("cac:OrderReference")
		}
		w.end("cac:OrderLineReference")
	}
	if c := l.Croatia; c != nil {
		w.lineReference("cac:DespatchLineReference", c.DespatchLine)
		w.lineReference("cac:ReceiptLineReference", c.ReceiptLine)
	}
}

func (w *writer) lineReference(name string, d *invoice.DocumentReference) {
	if d.IsZero() {
		return
	}
	id := d.LineID
	if id == "" {
		id = na
	}
	w.start(name)
	w.leafReq("cbc:LineID", id)
	if d.ID != "" || !d.IssueDate.IsZero() {
		w.start("cac:DocumentReference")
		w.leafReq("cbc:ID", d.ID)
		w.date("cbc:IssueDate", d.IssueDate)
		w.end("cac:DocumentReference")
	}
	w.end(name)
	w.loseText(d.TypeCode, "EXT Line.Croatia.LineReference.TypeCode")
	w.loseIf(d.Croatia != nil, "EXT Line.Croatia.LineReference.Croatia")
}
