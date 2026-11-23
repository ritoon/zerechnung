package ubl

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Namespaces of the UBL 2.1 syntax.
const (
	NamespaceInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	NamespaceCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	NamespaceCAC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	NamespaceCBC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	NamespaceEXT        = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
)

// ErrNotUBL is returned (wrapped) by Parse when the document is well-formed
// XML but neither a UBL Invoice nor a UBL CreditNote, for example a CII
// invoice.
var ErrNotUBL = errors.New("ubl: not a UBL Invoice or CreditNote document")

// Issue is a value that could not be read: a malformed amount, quantity,
// date, indicator or base64 attachment. The corresponding field of the
// invoice is left empty.
type Issue struct {
	Path    string // location of the element, as given by xmltree.Node.Path
	Line    int    // line of the element in the document, 1-based
	Message string
}

// String formats the issue as "line <Line>: <Path>: <Message>".
func (i Issue) String() string {
	return "line " + strconv.Itoa(i.Line) + ": " + i.Path + ": " + i.Message
}

// IssuesError is the error returned by Parse, together with a non-nil
// invoice, when some values could not be read. Everything else was read: a
// caller that tolerates incomplete data can use the invoice and inspect
// Issues with errors.As.
type IssuesError struct {
	Issues []Issue
}

// Error gives the number of issues and the first one.
func (e *IssuesError) Error() string {
	if len(e.Issues) == 1 {
		return "ubl: unreadable value at " + e.Issues[0].String()
	}
	return "ubl: " + strconv.Itoa(len(e.Issues)) + " unreadable values, first at " + e.Issues[0].String()
}

// IsUBL reports whether root is the root element of a UBL Invoice or
// CreditNote.
func IsUBL(root *xmltree.Node) bool { return checkRoot(root) == nil }

// Parse reads a UBL Invoice or CreditNote.
//
// It fails, with a nil invoice, only when data is not well-formed XML or its
// root is neither an Invoice nor a CreditNote in the UBL 2.1 namespaces (the
// error then wraps ErrNotUBL). When some values cannot be read, Parse returns
// the invoice with the corresponding fields empty and an *IssuesError listing
// them.
//
// The business terms are read from the elements that CEN/TS 16931-3-2 binds
// them to. Two conventions of the binding are understood: a note (BT-22)
// written "#AAI#text" carries its subject code (BT-21), and the VAT point
// date codes of UBL (BT-8: 3, 35 and 432) are read as the CII codes of the
// model (5, 29 and 72). In a credit note, the payment due date (BT-9) is read
// from cac:PaymentMeans/cbc:PaymentDueDate and the project reference (BT-11)
// from the additional document reference of type 50. When the specification
// identifier declares the Dutch G-account extension (see
// invoice.GAccountExtension), the elements that it adds to the binding are
// read too: cac:PaymentMeans/cbc:ID into PaymentMeans.ID, and the
// cbc:PaymentMeansID and cbc:Amount of each cac:PaymentTerms into
// PaymentTerms.PaymentMeansID and PartialAmount. In other invoices they are
// kept as extensions.
//
// When the specification identifier declares the Italian extension of the
// domestic invoice (see invoice.ItalianExtension), the UBL extensions and
// elements that appendix 4.2 of the Italian technical rules adds to the
// binding are read into the national blocks, as package fatturapa would read
// the FatturaPA data they carry (see invoice.Italy): the seven extensions of
// EXT-IT (urn:fdc:agid.gov.it:fatturapa:SistemaEmittente, SoggettoEmittente,
// TipoDocumento, TipoRitenuta, Art73, RiepilogoIVA:Arrotondamento and
// ScontoMaggiorazione, possibly followed by "::" and the ordinal of the
// withholding, VAT breakdown or discount), the dates of the purchase order
// and of the dispatch advice, the main invoice
// (cac:BillingReference/cac:AdditionalDocumentReference), the tax regime
// (cbc:TaxLevelCode), permanent establishment (cac:AgentParty) and
// third-party intermediary (cac:ServiceProviderParty) of the seller, the tax
// code of the tax representative, the carrier, pickup and shipment of the
// delivery, the delivery terms, the withholdings (cac:WithholdingTaxTotal),
// the discount for early payment and the penalty for late payment, and the
// order and dispatch advice references of each line. A document type that
// BT-3 implies, the tax regime RF01 and the identifiers NA that the
// technical rules give to identifiers FatturaPA lacks are not kept. In
// other invoices these elements are kept as extensions.
//
// When the specification identifier is the one of the Croatian CIUS, with or
// without its extension (see invoice.URNCIUSHR and invoice.ProfileCIUSHR),
// the Croatian data goes to the national blocks too: the UBL extension
// hrextac:HRFISK20Data (indicator of cash accounting HR-BT-15, Croatian VAT
// breakdown HR-BG-2 and totals HR-BT-23 to HR-BT-25) to invoice.Croatia,
// with the issue time (cbc:IssueTime, HR-BT-2) and the operator of the
// seller (cac:SellerContact, HR-BT-4 and HR-BT-5); the description of a
// preceding invoice (HR-BT-3) to DocumentReference.Croatia; the HR VAT
// category label and the exemption reason of a document level allowance or
// charge (HR-BT-6 to HR-BT-8) to AllowanceCharge.Croatia, those of an item
// (HR-BT-12 to HR-BT-14) and the despatch and receiving advices of a line
// (HR-BT-10, HR-BT-11) to Line.Croatia. The copy indicator (HR-BT-1) goes to
// Invoice.CopyIndicator and the purchase order of a line (HR-BT-9) to
// Line.OrderReference. A line identifier NA in a line reference stands for
// no line. In other invoices these elements are kept as extensions.
//
// Nothing is dropped: the elements and attributes outside the binding are kept
// as extensions (see invoice.Extension) in the Extensions field of the value
// that encloses them, with paths made of the conventional cac, cbc and ext
// prefixes ("cac:PartyIdentification/cbc:ID"); Invoice.Unmodeled lists them.
// A currencyID equal to the invoice or VAT accounting currency is implied by
// BT-5 and BT-6, a UBLVersionID of 2.1 by the syntax, and attributes in other
// namespaces (xsi:schemaLocation) are not kept.
func Parse(data []byte) (*invoice.Invoice, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("ubl: %w", err)
	}
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	inv, issues := ParseTree(root)
	if len(issues) > 0 {
		return inv, &IssuesError{Issues: issues}
	}
	return inv, nil
}

// ParseTree reads a UBL invoice from an already parsed document, so that a
// caller that also validates the document parses the XML only once. It
// returns the invoice and the values that could not be read (see Parse). If
// root is neither a UBL Invoice nor a CreditNote, the invoice is nil and the
// single issue says why.
func ParseTree(root *xmltree.Node) (*invoice.Invoice, []Issue) {
	if err := checkRoot(root); err != nil {
		is := Issue{Message: err.Error()}
		if root != nil {
			is.Path, is.Line = root.Path(), root.Line
		}
		return nil, []Issue{is}
	}
	r := reader{inv: new(invoice.Invoice), credit: root.Name == "CreditNote"}
	r.document(root)
	return r.inv, r.issues
}

func checkRoot(root *xmltree.Node) error {
	switch {
	case root == nil:
		return ErrNotUBL
	case root.Name == "Invoice" && root.Space == NamespaceInvoice,
		root.Name == "CreditNote" && root.Space == NamespaceCreditNote:
		return nil
	case root.Name == "Invoice" || root.Name == "CreditNote":
		return fmt.Errorf("%w: root element <%s> in namespace %q", ErrNotUBL, root.Name, root.Space)
	}
	return fmt.Errorf("%w: root element is <%s>", ErrNotUBL, root.Name)
}

// prefixOf returns the conventional prefix of a UBL namespace, or "".
func prefixOf(space string) string {
	switch space {
	case NamespaceCAC:
		return "cac"
	case NamespaceCBC:
		return "cbc"
	case NamespaceEXT:
		return "ext"
	case NamespaceHR:
		return "hrextac"
	}
	return ""
}

// qname returns the name of n with the conventional prefix of its namespace,
// or with its own prefix outside the UBL namespaces.
func qname(n *xmltree.Node) string {
	if p := prefixOf(n.Space); p != "" {
		return p + ":" + n.Name
	}
	if n.Prefix != "" {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

// reader maps a UBL tree to the model.
//
// Every element and attribute of the document is either read into a field of
// the model or kept as an extension: each container function handles the
// children it knows and passes the others to unread. Extensions go to the
// value being read whose Extensions field is ext, with a path relative to the
// element of that value (path holds its qualified names).
type reader struct {
	inv    *invoice.Invoice
	issues []Issue
	credit bool // the document is a CreditNote
	// extIT is set when the specification identifier declares the Italian
	// extension (EXT-IT): the elements of the domestic invoice are read into
	// the national blocks (see italy.go).
	extIT bool
	// extHR is set when the specification identifier is the one of the
	// CIUS-HR: the Croatian data is read into the national blocks (see
	// croatia.go).
	extHR bool

	ext  *[]invoice.Extension
	path []string

	// cur and taxCur are the invoice and VAT accounting currencies: a
	// currencyID equal to one of them is implied by BT-5 and BT-6.
	cur, taxCur string

	// Payment terms: BT-9, BT-20 (one per cac:PaymentTerms) and BT-89 are
	// spread over the document in UBL and gathered in PaymentTerms.
	dueDate invoice.Date
	terms   []invoice.PaymentTerms
	mandate string

	orderDate invoice.Date // date of the purchase order (BT-13) under EXT-IT

	buf []byte
}

// gaccount reports whether the specification identifier read so far
// declares the Dutch G-account extension, whose elements are then read into
// the model rather than kept as extensions.
func (r *reader) gaccount() bool {
	return strings.Contains(r.inv.SpecificationID, invoice.GAccountExtension)
}

func (r *reader) issue(n *xmltree.Node, msg string) {
	r.issues = append(r.issues, Issue{Path: n.Path(), Line: n.Line, Message: msg})
}

// holder is the state saved by hold.
type holder struct {
	ext  *[]invoice.Extension
	path []string
}

// hold makes the value whose Extensions field is ext the receiver of the
// extensions found until release, with paths relative to n, its element.
// The attributes of n itself are kept with an empty path.
func (r *reader) hold(n *xmltree.Node, ext *[]invoice.Extension) holder {
	h := holder{r.ext, r.path}
	r.ext, r.path = ext, r.path[len(r.path):]
	if len(n.Attrs) > 0 {
		r.keepAttrs(n, "", "", "", "")
	}
	return h
}

func (r *reader) release(h holder) { r.ext, r.path = h.ext, h.path }

// enter starts reading the children of the container n: its attributes are
// kept as extensions and its name extends the path.
func (r *reader) enter(n *xmltree.Node) {
	if len(n.Attrs) > 0 {
		r.keepAttrs(n, r.attrPath(n), "", "", "")
	}
	r.path = append(r.path, qname(n))
}

func (r *reader) leave() { r.path = r.path[:len(r.path)-1] }

func (r *reader) pathString() string { return strings.Join(r.path, "/") }

// unread keeps n, a child of the current container, as an extension.
func (r *reader) unread(n *xmltree.Node) {
	r.buf = appendElement(r.buf[:0], n)
	*r.ext = append(*r.ext, invoice.Extension{Path: r.pathString(), XML: string(r.buf)})
}

// keepAttrs keeps the attributes of n, whose location is path (see
// attrPath), except those named a, b and c, which the caller reads.
// Attributes in another namespace (xsi:schemaLocation…) are not part of the
// data.
func (r *reader) keepAttrs(n *xmltree.Node, path, a, b, c string) {
	for _, at := range n.Attrs {
		if at.Prefix != "" || at.Name == a || at.Name == b || at.Name == c {
			continue
		}
		*r.ext = append(*r.ext, invoice.Extension{Path: path, Attr: at.Name, Value: at.Value})
	}
}

// attrPath returns the location of n, a child of the current container:
// the current path followed by the qualified name of n, with its occurrence
// number when it is not the first child of that name.
func (r *reader) attrPath(n *xmltree.Node) string {
	name := qname(n)
	if p := n.Parent; p != nil {
		k := 0
		for _, s := range p.Children {
			if s == n {
				break
			}
			if s.Name == n.Name && s.Space == n.Space {
				k++
			}
		}
		if k > 0 {
			name += "[" + strconv.Itoa(k+1) + "]"
		}
	}
	if len(r.path) == 0 {
		return name
	}
	return r.pathString() + "/" + name
}

// text returns the trimmed text of the leaf n ("" for nil) and keeps its
// attributes and child elements.
func (r *reader) text(n *xmltree.Node) string { return r.textAttrs(n, "", "", "") }

// textAttrs is text for a leaf whose attributes a, b and c are read by the
// caller.
func (r *reader) textAttrs(n *xmltree.Node, a, b, c string) string {
	if n == nil {
		return ""
	}
	if len(n.Attrs) > 0 {
		r.keepAttrs(n, r.attrPath(n), a, b, c)
	}
	if len(n.Children) > 0 {
		r.path = append(r.path, qname(n))
		for _, k := range n.Children {
			r.unread(k)
		}
		r.leave()
	}
	return strings.TrimSpace(n.Text)
}

// attr returns the trimmed value of an attribute of n.
func attr(n *xmltree.Node, name string) string {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name && n.Attrs[i].Prefix == "" {
			return strings.TrimSpace(n.Attrs[i].Value)
		}
	}
	return ""
}

// ident reads an identifier element with its schemeID.
func (r *reader) ident(n *xmltree.Node) invoice.Identifier {
	return invoice.Identifier{ID: r.textAttrs(n, "schemeID", "", ""), Scheme: attr(n, "schemeID")}
}

// decimal reads an amount, a rate or a percentage. A currencyID equal to
// the invoice or VAT accounting currency is implied; another one is kept.
func (r *reader) decimal(n *xmltree.Node) invoice.Decimal {
	var s string
	if c := attr(n, "currencyID"); c != "" && (c == r.cur || c == r.taxCur) {
		s = r.textAttrs(n, "currencyID", "", "")
	} else {
		s = r.text(n)
	}
	return r.parseDecimal(n, s)
}

// quantity reads a quantity and its unit code.
func (r *reader) quantity(n *xmltree.Node) (invoice.Decimal, string) {
	return r.parseDecimal(n, r.textAttrs(n, "unitCode", "", "")), attr(n, "unitCode")
}

func (r *reader) parseDecimal(n *xmltree.Node, s string) invoice.Decimal {
	if s == "" {
		return invoice.Decimal{}
	}
	d, err := invoice.ParseDecimal(s)
	if err != nil {
		r.issue(n, err.Error())
		return invoice.Decimal{}
	}
	return d
}

// date reads a cbc date (xs:date, 2006-01-02).
func (r *reader) date(n *xmltree.Node) invoice.Date {
	s := r.text(n)
	if s == "" {
		return invoice.Date{}
	}
	d, err := invoice.ParseISODate(s)
	if err != nil {
		r.issue(n, err.Error())
		return invoice.Date{}
	}
	return d
}

// binary decodes a base64 binary object, ignoring white space.
func (r *reader) binary(n *xmltree.Node) []byte {
	r.textAttrs(n, "mimeCode", "filename", "")
	s := unspace(n.Text)
	if len(s) == 0 {
		return nil
	}
	b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
	k, err := base64.StdEncoding.Decode(b, s)
	if err != nil {
		s = bytes.TrimRight(s, "=")
		b = make([]byte, base64.RawStdEncoding.DecodedLen(len(s)))
		var err2 error
		if k, err2 = base64.RawStdEncoding.Decode(b, s); err2 != nil {
			r.issue(n, "invalid base64 content: "+err.Error())
			return nil
		}
	}
	return b[:k]
}

// unspace returns s without XML white space.
func unspace(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			out = append(out, c)
		}
	}
	return out
}

// child returns the first child element of n with the given local name
// (nil when n is nil).
func child(n *xmltree.Node, name string) *xmltree.Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// textOf returns the trimmed text of n without keeping anything ("" for
// nil); it is used to look ahead.
func textOf(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Text)
}

// isVAT reports whether a cac:TaxScheme element identifies the VAT.
func isVAT(scheme *xmltree.Node) bool {
	return strings.EqualFold(textOf(child(scheme, "ID")), "VAT")
}

// VAT point date codes (BT-8): UBL uses UNTDID 2005, CII and the model
// UNTDID 2475.
func taxPointFromUBL(code string) string {
	switch code {
	case "3":
		return invoice.TaxPointInvoiceDate
	case "35":
		return invoice.TaxPointDeliveryDate
	case "432":
		return invoice.TaxPointPaymentDate
	}
	return code
}

func taxPointToUBL(code string) string {
	switch code {
	case invoice.TaxPointInvoiceDate:
		return "3"
	case invoice.TaxPointDeliveryDate:
		return "35"
	case invoice.TaxPointPaymentDate:
		return "432"
	}
	return code
}

// splitNote splits a note written "#CODE#text" into its subject code
// (BT-21) and its content (BT-22).
func splitNote(s string) (code, content string) {
	if len(s) < 3 || s[0] != '#' {
		return "", s
	}
	end := strings.IndexByte(s[1:], '#')
	if end <= 0 || end > 10 {
		return "", s
	}
	code = s[1 : 1+end]
	for i := 0; i < len(code); i++ {
		c := code[i]
		if !('A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9') {
			return "", s
		}
	}
	return code, s[2+end:]
}

func (r *reader) document(root *xmltree.Node) {
	inv := r.inv
	r.cur, r.taxCur = textOf(child(root, "DocumentCurrencyCode")), textOf(child(root, "TaxCurrencyCode"))
	r.extIT = strings.Contains(textOf(child(root, "CustomizationID")), invoice.ItalianExtension)
	r.extHR = invoice.ProfileFromURN(textOf(child(root, "CustomizationID"))) == invoice.ProfileCIUSHR
	h := r.hold(root, &inv.Extensions)
	var period, delivery, extHRDone, copyDone bool
	var extensions *xmltree.Node // ext:UBLExtensions, read last under EXT-IT
	extAt := 0
	for _, c := range root.Children {
		if r.extIT && extensions == nil && c.Space == NamespaceEXT && c.Name == "UBLExtensions" {
			extensions, extAt = c, len(inv.Extensions)
			continue
		}
		if r.extHR && !extHRDone && c.Space == NamespaceEXT && c.Name == "UBLExtensions" {
			extHRDone = true
			r.croatianExtensions(c)
			continue
		}
		if c.Space != NamespaceCBC && c.Space != NamespaceCAC {
			r.unread(c)
			continue
		}
		switch c.Name {
		case "UBLVersionID":
			if textOf(c) != "2.1" || len(c.Attrs) > 0 {
				r.unread(c)
			}
		case "CustomizationID":
			setOnce(r, c, &inv.SpecificationID)
		case "ProfileID":
			setOnce(r, c, &inv.BusinessProcess)
		case "ID":
			setOnce(r, c, &inv.Number)
		case "CopyIndicator":
			if !r.extHR || copyDone {
				r.unread(c)
				continue
			}
			copyDone = true
			inv.CopyIndicator = r.indicator(c)
		case "IssueTime":
			if !r.extHR || inv.Croatia != nil && inv.Croatia.IssueTime != "" {
				r.unread(c)
				continue
			}
			r.hr().IssueTime = r.text(c)
		case "IssueDate":
			if !inv.IssueDate.IsZero() {
				r.unread(c)
				continue
			}
			inv.IssueDate = r.date(c)
		case "DueDate":
			if !r.dueDate.IsZero() || r.credit {
				r.unread(c)
				continue
			}
			r.dueDate = r.date(c)
		case "InvoiceTypeCode", "CreditNoteTypeCode":
			setOnce(r, c, &inv.TypeCode)
		case "Note":
			code, content := splitNote(r.text(c))
			inv.Notes = append(inv.Notes, invoice.Note{SubjectCode: code, Content: content})
		case "TaxPointDate":
			if !inv.TaxPointDate.IsZero() {
				r.unread(c)
				continue
			}
			inv.TaxPointDate = r.date(c)
		case "DocumentCurrencyCode":
			setOnce(r, c, &inv.Currency)
		case "TaxCurrencyCode":
			setOnce(r, c, &inv.TaxCurrency)
		case "AccountingCost":
			setOnce(r, c, &inv.BuyerAccountingReference)
		case "BuyerReference":
			setOnce(r, c, &inv.BuyerReference)
		case "InvoicePeriod":
			if period {
				r.unread(c)
				continue
			}
			period = true
			r.invoicePeriod(c)
		case "OrderReference":
			if inv.PurchaseOrderReference != "" || inv.SalesOrderReference != "" {
				r.unread(c)
				continue
			}
			r.orderReference(c)
		case "BillingReference":
			r.billingReference(c)
		case "DespatchDocumentReference":
			r.despatchDocument(c)
		case "ReceiptDocumentReference":
			r.docID(c, &inv.ReceivingAdviceReference)
		case "ContractDocumentReference":
			r.docID(c, &inv.ContractReference)
		case "ProjectReference":
			r.docID(c, &inv.ProjectReference)
		case "OriginatorDocumentReference":
			if inv.TenderReference() != "" {
				r.unread(c)
				continue
			}
			var id string
			r.docID(c, &id)
			if id != "" {
				inv.AdditionalDocuments = append(inv.AdditionalDocuments, invoice.AdditionalDocument{ID: id, TypeCode: "50"})
			}
		case "AdditionalDocumentReference":
			r.additionalDocument(c)
		case "AccountingSupplierParty":
			r.roleParty(c, &inv.Seller, roleSeller)
		case "AccountingCustomerParty":
			r.roleParty(c, &inv.Buyer, roleBuyer)
		case "PayeeParty":
			if inv.Payee != nil {
				r.unread(c)
				continue
			}
			inv.Payee = new(invoice.Party)
			r.party(c, inv.Payee, rolePayee)
		case "TaxRepresentativeParty":
			if inv.SellerTaxRepresentative != nil {
				r.unread(c)
				continue
			}
			inv.SellerTaxRepresentative = new(invoice.Party)
			r.party(c, inv.SellerTaxRepresentative, roleTaxRep)
		case "Delivery":
			if delivery {
				r.unread(c)
				continue
			}
			delivery = true
			r.delivery(c)
		case "DeliveryTerms":
			if !r.extIT || !r.deliveryTerms(c) {
				r.unread(c)
			}
		case "PaymentMeans":
			r.paymentMeans(c)
		case "PaymentTerms":
			r.paymentTerms(c)
		case "AllowanceCharge":
			inv.AllowancesCharges = append(inv.AllowancesCharges, r.allowanceCharge(c, true))
		case "TaxTotal":
			r.taxTotal(c)
		case "WithholdingTaxTotal":
			if !r.extIT {
				r.unread(c)
				continue
			}
			r.withholdingTaxTotal(c)
		case "LegalMonetaryTotal":
			r.totals(c)
		case "InvoiceLine", "CreditNoteLine":
			if (c.Name == "CreditNoteLine") != r.credit {
				r.unread(c)
				continue
			}
			inv.Lines = append(inv.Lines, invoice.Line{})
			r.line(c, &inv.Lines[len(inv.Lines)-1])
		default:
			r.unread(c)
		}
	}
	if extensions != nil {
		r.italianExtensions(extensions, extAt)
	}
	r.release(h)
	r.finishTerms()
	if r.extIT {
		r.finishItaly()
	}
	inv.Profile = invoice.DetectProfile(inv.SpecificationID)
}

// setOnce reads the leaf c into *dst when it is still empty, and keeps c as
// an extension otherwise.
func setOnce(r *reader, c *xmltree.Node, dst *string) {
	if *dst != "" {
		r.unread(c)
		return
	}
	*dst = r.text(c)
}

// finishTerms gathers the payment terms: the first entry carries the due
// date (BT-9) and the mandate reference (BT-89) with the first note (BT-20).
func (r *reader) finishTerms() {
	if r.dueDate.IsZero() && r.mandate == "" && len(r.terms) == 0 {
		return
	}
	if len(r.terms) == 0 {
		r.terms = append(r.terms, invoice.PaymentTerms{})
	}
	r.terms[0].DueDate = r.dueDate
	r.terms[0].DirectDebitMandateID = r.mandate
	r.inv.PaymentTerms = r.terms
}

func (r *reader) invoicePeriod(n *xmltree.Node) {
	inv := r.inv
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "StartDate" && inv.InvoicingPeriod.Start.IsZero():
			inv.InvoicingPeriod.Start = r.date(c)
		case c.Name == "EndDate" && inv.InvoicingPeriod.End.IsZero():
			inv.InvoicingPeriod.End = r.date(c)
		case c.Name == "DescriptionCode" && inv.TaxPointDateCode == "":
			inv.TaxPointDateCode = taxPointFromUBL(r.text(c))
		default:
			r.unread(c)
		}
	}
	r.leave()
}

func (r *reader) orderReference(n *xmltree.Node) {
	inv := r.inv
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && inv.PurchaseOrderReference == "":
			inv.PurchaseOrderReference = r.text(c)
		case c.Name == "SalesOrderID" && inv.SalesOrderReference == "":
			inv.SalesOrderReference = r.text(c)
		case c.Name == "IssueDate" && r.extIT && r.orderDate.IsZero():
			r.orderDate = r.date(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// docID reads a document reference of which the binding only uses the
// identifier (cbc:ID) into *dst.
func (r *reader) docID(n *xmltree.Node, dst *string) {
	if *dst != "" {
		r.unread(n)
		return
	}
	r.enter(n)
	for _, c := range n.Children {
		if c.Name == "ID" && *dst == "" {
			*dst = r.text(c)
			continue
		}
		r.unread(c)
	}
	r.leave()
}

func (r *reader) billingReference(n *xmltree.Node) {
	r.enter(n)
	for _, c := range n.Children {
		if c.Name == "AdditionalDocumentReference" && r.extIT && r.mainInvoice(c) {
			continue
		}
		if c.Name != "InvoiceDocumentReference" {
			r.unread(c)
			continue
		}
		r.inv.PrecedingInvoices = append(r.inv.PrecedingInvoices, invoice.DocumentReference{})
		d := &r.inv.PrecedingInvoices[len(r.inv.PrecedingInvoices)-1]
		h := r.hold(c, &d.Extensions)
		for _, k := range c.Children {
			switch {
			case k.Name == "ID" && d.ID == "":
				d.ID = r.text(k)
			case k.Name == "IssueDate" && d.IssueDate.IsZero():
				d.IssueDate = r.date(k)
			case k.Name == "DocumentDescription" && r.extHR && d.Croatia == nil:
				d.Croatia = &invoice.CroatiaReference{Description: r.text(k)}
			default:
				r.unread(k)
			}
		}
		r.release(h)
	}
	r.leave()
}

func (r *reader) additionalDocument(n *xmltree.Node) {
	inv := r.inv
	typ := textOf(child(n, "DocumentTypeCode"))
	if r.credit && typ == "50" && inv.ProjectReference == "" {
		r.enter(n)
		for _, c := range n.Children {
			switch {
			case c.Name == "ID" && inv.ProjectReference == "":
				inv.ProjectReference = r.text(c)
			case c.Name == "DocumentTypeCode":
				r.text(c)
			default:
				r.unread(c)
			}
		}
		r.leave()
		return
	}
	inv.AdditionalDocuments = append(inv.AdditionalDocuments, invoice.AdditionalDocument{})
	d := &inv.AdditionalDocuments[len(inv.AdditionalDocuments)-1]
	h := r.hold(n, &d.Extensions)
	var idDone, typeDone, descDone, attDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			if typ == "130" {
				id := r.ident(c)
				d.ID, d.ReferenceTypeCode = id.ID, id.Scheme
			} else {
				d.ID = r.text(c)
			}
		case c.Name == "DocumentTypeCode" && !typeDone:
			typeDone = true
			d.TypeCode = r.text(c)
		case c.Name == "DocumentDescription" && !descDone:
			descDone = true
			d.Description = r.text(c)
		case c.Name == "Attachment" && !attDone:
			attDone = true
			r.attachment(c, d)
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

func (r *reader) attachment(n *xmltree.Node, d *invoice.AdditionalDocument) {
	r.enter(n)
	binDone := false
	for _, c := range n.Children {
		switch {
		case c.Name == "EmbeddedDocumentBinaryObject" && !binDone:
			binDone = true
			d.Attachment = r.binary(c)
			d.AttachmentMimeCode, d.AttachmentFilename = attr(c, "mimeCode"), attr(c, "filename")
		case c.Name == "ExternalReference" && d.URI == "":
			r.enter(c)
			for _, k := range c.Children {
				if k.Name == "URI" && d.URI == "" {
					d.URI = r.text(k)
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

// role is the role of a party: it decides which name element carries the
// party name.
type role uint8

const (
	roleSeller role = iota
	roleBuyer
	rolePayee
	roleTaxRep
	roleShipTo
)

// roleParty reads cac:AccountingSupplierParty or cac:AccountingCustomerParty:
// the party is its cac:Party child, the other children are kept on the
// invoice.
func (r *reader) roleParty(n *xmltree.Node, p *invoice.Party, ro role) {
	r.enter(n)
	var done, contactDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "Party" && !done:
			done = true
			r.party(c, p, ro)
		case c.Name == "SellerContact" && ro == roleSeller && r.extHR && !contactDone:
			contactDone = true
			r.sellerContact(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// party reads a cac:Party (or cac:PayeeParty, cac:TaxRepresentativeParty,
// cac:DeliveryParty) element.
func (r *reader) party(n *xmltree.Node, p *invoice.Party, ro role) {
	h := r.hold(n, &p.Extensions)
	byName := ro != roleSeller && ro != roleBuyer // the party name is cac:PartyName
	var nameDone, addrDone, legalDone, contactDone, endpointDone bool
	for _, c := range n.Children {
		switch c.Name {
		case "EndpointID":
			if endpointDone {
				r.unread(c)
				continue
			}
			endpointDone = true
			p.ElectronicAddress = r.ident(c)
		case "PartyIdentification":
			r.partyIdentification(c, p, ro)
		case "PartyName":
			if nameDone {
				r.unread(c)
				continue
			}
			nameDone = true
			r.enter(c)
			for _, k := range c.Children {
				switch {
				case k.Name == "Name" && byName && p.Name == "":
					p.Name = r.text(k)
				case k.Name == "Name" && !byName && p.TradingName == "":
					p.TradingName = r.text(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case "PostalAddress":
			if addrDone {
				r.unread(c)
				continue
			}
			addrDone = true
			p.Address = r.address(c)
		case "PartyTaxScheme":
			r.partyTaxScheme(c, p, ro == roleSeller && r.extIT)
		case "PartyLegalEntity":
			if legalDone {
				r.unread(c)
				continue
			}
			legalDone = true
			r.legalEntity(c, p)
		case "Contact":
			if contactDone {
				r.unread(c)
				continue
			}
			contactDone = true
			p.Contact = r.contact(c)
		case "AgentParty":
			if ro != roleSeller || !r.extIT || !r.establishment(c, p) {
				r.unread(c)
			}
		case "ServiceProviderParty":
			if ro != roleSeller || !r.extIT || !r.intermediary(c) {
				r.unread(c)
			}
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

// partyIdentification reads a party identifier (BT-29, BT-46, BT-60). The
// SEPA identifier of the seller or the payee is the bank assigned creditor
// identifier (BT-90). Under EXT-IT, the tax code (scheme 0210) of the tax
// representative is its legal registration identifier, as package fatturapa
// reads it.
func (r *reader) partyIdentification(n *xmltree.Node, p *invoice.Party, ro role) {
	r.enter(n)
	done := false
	for _, c := range n.Children {
		if c.Name != "ID" || done {
			r.unread(c)
			continue
		}
		done = true
		id := r.ident(c)
		if (ro == roleSeller || ro == rolePayee) && strings.EqualFold(id.Scheme, "SEPA") && r.inv.CreditorReferenceID == "" {
			r.inv.CreditorReferenceID = id.ID
			continue
		}
		if ro == roleTaxRep && r.extIT && id.Scheme == schemeCF && p.LegalRegistration.IsZero() {
			p.LegalRegistration = id
			continue
		}
		p.IDs = append(p.IDs, id)
	}
	r.leave()
}

// partyTaxScheme reads a cac:PartyTaxScheme; with regime, the tax level code
// of the VAT scheme is the tax regime of the seller (RegimeFiscale), RF01,
// the ordinary regime, being implied as package fatturapa implies it.
func (r *reader) partyTaxScheme(n *xmltree.Node, p *invoice.Party, regime bool) {
	scheme := child(n, "TaxScheme")
	vat := isVAT(scheme)
	dst := &p.VATID
	if !vat {
		dst = &p.TaxRegistrationID
	}
	if *dst != "" {
		r.unread(n)
		return
	}
	r.enter(n)
	var idDone, schemeDone, levelDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "CompanyID" && !idDone:
			idDone = true
			*dst = r.text(c)
		case c.Name == "TaxLevelCode" && vat && regime && !levelDone:
			levelDone = true
			if s := r.text(c); s != "" && s != "RF01" {
				partyItaly(p).TaxRegime = s
			}
		case c == scheme && !schemeDone:
			schemeDone = true
			// FC, the scheme of a tax registration in CII, is the one
			// Marshal writes for BT-32: it is implied as VAT is.
			r.taxScheme(c, vat || strings.EqualFold(textOf(child(c, "ID")), "FC"))
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// taxScheme reads a cac:TaxScheme: an identifier that the binding implies
// (implied: VAT, or FC for BT-32) is not kept, anything else is.
func (r *reader) taxScheme(n *xmltree.Node, implied bool) {
	if !implied || len(n.Children) != 1 || len(n.Attrs) > 0 || len(n.Children[0].Attrs) > 0 {
		r.unread(n)
	}
}

func (r *reader) legalEntity(n *xmltree.Node, p *invoice.Party) {
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "RegistrationName" && p.Name == "":
			p.Name = r.text(c)
		case c.Name == "CompanyID" && p.LegalRegistration.IsZero():
			p.LegalRegistration = r.ident(c)
		case c.Name == "CompanyLegalForm" && p.Description == "":
			p.Description = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

func (r *reader) contact(n *xmltree.Node) *invoice.Contact {
	ct := new(invoice.Contact)
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "Name" && ct.PersonName == "":
			ct.PersonName = r.text(c)
		case c.Name == "Telephone" && ct.Phone == "":
			ct.Phone = r.text(c)
		case c.Name == "ElectronicMail" && ct.Email == "":
			ct.Email = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	return ct
}

func (r *reader) address(n *xmltree.Node) invoice.Address {
	var a invoice.Address
	r.enter(n)
	for _, c := range n.Children {
		switch {
		case c.Name == "StreetName" && a.Line1 == "":
			a.Line1 = r.text(c)
		case c.Name == "AdditionalStreetName" && a.Line2 == "":
			a.Line2 = r.text(c)
		case c.Name == "AddressLine" && a.Line3 == "" && child(c, "Line") != nil:
			r.enter(c)
			done := false
			for _, k := range c.Children {
				if k.Name == "Line" && !done {
					done = true
					a.Line3 = r.text(k)
					continue
				}
				r.unread(k)
			}
			r.leave()
		case c.Name == "CityName" && a.City == "":
			a.City = r.text(c)
		case c.Name == "PostalZone" && a.PostCode == "":
			a.PostCode = r.text(c)
		case c.Name == "CountrySubentity" && a.CountrySubdivision == "":
			a.CountrySubdivision = r.text(c)
		case c.Name == "Country" && a.CountryCode == "":
			r.enter(c)
			done := false
			for _, k := range c.Children {
				if k.Name == "IdentificationCode" && !done {
					done = true
					a.CountryCode = r.text(k)
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
	return a
}

// delivery reads cac:Delivery: the actual delivery date (BT-72) and the
// deliver-to party (BG-13), and under EXT-IT the carrier, the pickup and
// the shipment (DatiTrasporto).
func (r *reader) delivery(n *xmltree.Node) {
	inv := r.inv
	ship := new(invoice.Party)
	r.enter(n)
	var locDone, partyDone, carrierDone, despatchDone, shipmentDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "CarrierParty" && r.extIT && !carrierDone:
			carrierDone = true
			r.carrier(c)
		case c.Name == "Despatch" && r.extIT && !despatchDone:
			despatchDone = true
			r.despatch(c)
		case c.Name == "Shipment" && r.extIT && !shipmentDone:
			shipmentDone = true
			r.shipment(c)
		case c.Name == "ActualDeliveryDate" && inv.DeliveryDate.IsZero():
			inv.DeliveryDate = r.date(c)
		case c.Name == "DeliveryLocation" && !locDone:
			locDone = true
			r.enter(c)
			var idDone, addrDone bool
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && !idDone:
					idDone = true
					ship.IDs = append(ship.IDs, r.ident(k))
				case k.Name == "Address" && !addrDone:
					addrDone = true
					ship.Address = r.address(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case c.Name == "DeliveryParty" && !partyDone:
			partyDone = true
			r.party(c, ship, roleShipTo)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if !ship.IsZero() {
		inv.ShipTo = ship
	}
}

func (r *reader) paymentMeans(n *xmltree.Node) {
	inv := r.inv
	inv.PaymentMeans = append(inv.PaymentMeans, invoice.PaymentMeans{})
	pm := &inv.PaymentMeans[len(inv.PaymentMeans)-1]
	h := r.hold(n, &pm.Extensions)
	var idDone, codeDone, cardDone, payeeDone, mandateDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone && r.gaccount():
			idDone = true
			pm.ID = r.text(c)
		case c.Name == "PaymentMeansCode" && !codeDone:
			codeDone = true
			pm.TypeCode = r.textAttrs(c, "name", "", "")
			pm.Information = attr(c, "name")
		case c.Name == "PaymentDueDate" && r.credit && r.dueDate.IsZero():
			r.dueDate = r.date(c)
		case c.Name == "PaymentID" && inv.RemittanceInformation == "":
			inv.RemittanceInformation = r.text(c)
		case c.Name == "PaymentID" && textOf(c) == inv.RemittanceInformation && len(c.Attrs) == 0:
			// The same reference repeated in another payment means.
		case c.Name == "CardAccount" && !cardDone:
			cardDone = true
			r.enter(c)
			for _, k := range c.Children {
				switch {
				case k.Name == "PrimaryAccountNumberID" && pm.CardID == "":
					pm.CardID = r.text(k)
				case k.Name == "HolderName" && pm.CardholderName == "":
					pm.CardholderName = r.text(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case c.Name == "PayeeFinancialAccount" && !payeeDone:
			payeeDone = true
			r.payeeAccount(c, pm)
		case c.Name == "PaymentMandate" && !mandateDone:
			mandateDone = true
			r.enter(c)
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && r.mandate == "":
					r.mandate = r.text(k)
				case k.Name == "PayerFinancialAccount" && pm.PayerIBAN == "":
					r.enter(k)
					done := false
					for _, a := range k.Children {
						if a.Name == "ID" && !done {
							done = true
							pm.PayerIBAN = r.text(a)
							continue
						}
						r.unread(a)
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
	r.release(h)
}

func (r *reader) payeeAccount(n *xmltree.Node, pm *invoice.PaymentMeans) {
	r.enter(n)
	var idDone, branchDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			id := r.text(c)
			if looksLikeIBAN(id) {
				pm.PayeeIBAN = id
			} else {
				pm.PayeeProprietaryID = id
			}
		case c.Name == "Name" && pm.PayeeAccountName == "":
			pm.PayeeAccountName = r.text(c)
		case c.Name == "FinancialInstitutionBranch" && !branchDone:
			branchDone = true
			r.enter(c)
			done := false
			for _, k := range c.Children {
				if k.Name == "ID" && !done {
					done = true
					pm.PayeeBIC = r.text(k)
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

// looksLikeIBAN reports whether s has the shape of an IBAN: two letters, two
// digits and up to 30 letters or digits, spaces ignored.
func looksLikeIBAN(s string) bool {
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 5 || len(s) > 34 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
		digit := '0' <= c && c <= '9'
		switch {
		case i < 2 && !letter, i >= 2 && i < 4 && !digit, !letter && !digit:
			return false
		}
	}
	return true
}

func (r *reader) paymentTerms(n *xmltree.Node) {
	r.terms = append(r.terms, invoice.PaymentTerms{})
	t := &r.terms[len(r.terms)-1]
	h := r.hold(n, &t.Extensions)
	gaccount := r.gaccount()
	var refDone, amountDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "Note" && t.Description == "":
			t.Description = r.text(c)
		case c.Name == "PaymentMeansID" && !refDone && gaccount:
			refDone = true
			t.PaymentMeansID = r.text(c)
		case c.Name == "Amount" && !amountDone && gaccount:
			amountDone = true
			t.PartialAmount = r.decimal(c)
		case r.earlyPayment(c):
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

// allowanceCharge reads a cac:AllowanceCharge; vat tells whether its tax
// category is part of the binding (document level).
func (r *reader) allowanceCharge(n *xmltree.Node, vat bool) invoice.AllowanceCharge {
	var ac invoice.AllowanceCharge
	h := r.hold(n, &ac.Extensions)
	var indDone, codeDone, reasonDone, pctDone, amtDone, baseDone, catDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ChargeIndicator" && !indDone:
			indDone = true
			switch s := r.text(c); s {
			case "true", "1":
				ac.Charge = true
			case "false", "0":
			default:
				r.issue(c, "invalid indicator "+strconv.Quote(s))
			}
		case c.Name == "AllowanceChargeReasonCode" && !codeDone:
			codeDone = true
			ac.ReasonCode = r.text(c)
		case c.Name == "AllowanceChargeReason" && !reasonDone:
			reasonDone = true
			ac.Reason = r.text(c)
		case c.Name == "MultiplierFactorNumeric" && !pctDone:
			pctDone = true
			ac.Percentage = r.decimal(c)
		case c.Name == "Amount" && !amtDone:
			amtDone = true
			ac.Amount = r.decimal(c)
		case c.Name == "BaseAmount" && !baseDone:
			baseDone = true
			ac.BaseAmount = r.decimal(c)
		case c.Name == "TaxCategory" && vat && !catDone:
			catDone = true
			var hr *invoice.CroatiaTax
			if r.extHR {
				hr = new(invoice.CroatiaTax)
			}
			ac.VATCategoryCode, ac.VATRate = r.taxCategory(c, nil, hr)
			if hr != nil && *hr != (invoice.CroatiaTax{}) {
				ac.Croatia = hr
			}
		default:
			r.unread(c)
		}
	}
	r.release(h)
	return ac
}

// taxCategory reads a cac:TaxCategory or cac:ClassifiedTaxCategory: the
// category code and the rate, and, when vb is not nil, the exemption reason
// and its code. When hr is not nil, the Croatian data of the category goes
// to it: the HR VAT category label (cbc:Name) and, without vb, the
// exemption reason and its code.
func (r *reader) taxCategory(n *xmltree.Node, vb *invoice.VATBreakdown, hr *invoice.CroatiaTax) (code string, rate invoice.Decimal) {
	r.enter(n)
	var idDone, rateDone, schemeDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "ID" && !idDone:
			idDone = true
			code = r.text(c)
		case c.Name == "Name" && hr != nil && hr.Category == "":
			hr.Category = r.text(c)
		case c.Name == "Percent" && !rateDone:
			rateDone = true
			rate = r.decimal(c)
		case c.Name == "TaxExemptionReasonCode" && vb != nil && vb.ExemptionReasonCode == "":
			vb.ExemptionReasonCode = r.text(c)
		case c.Name == "TaxExemptionReason" && vb != nil && vb.ExemptionReason == "":
			vb.ExemptionReason = r.text(c)
		case c.Name == "TaxExemptionReasonCode" && vb == nil && hr != nil && hr.ExemptionReasonCode == "":
			hr.ExemptionReasonCode = r.text(c)
		case c.Name == "TaxExemptionReason" && vb == nil && hr != nil && hr.ExemptionReason == "":
			hr.ExemptionReason = r.text(c)
		case c.Name == "TaxScheme" && !schemeDone:
			schemeDone = true
			r.taxScheme(c, isVAT(c))
		default:
			r.unread(c)
		}
	}
	r.leave()
	return code, rate
}

// taxTotal reads a cac:TaxTotal: its amount is the invoice total VAT amount
// (BT-110), or the total VAT amount in accounting currency (BT-111) when its
// currency is the VAT accounting currency; its subtotals are the VAT
// breakdown (BG-23).
func (r *reader) taxTotal(n *xmltree.Node) {
	inv := r.inv
	r.enter(n)
	amtDone := false
	for _, c := range n.Children {
		switch {
		case c.Name == "TaxAmount" && !amtDone:
			amtDone = true
			cur := attr(c, "currencyID")
			switch {
			case r.taxCur != "" && r.taxCur != r.cur && cur == r.taxCur && !inv.Totals.TaxTotalInTaxCurrency.IsSet():
				inv.Totals.TaxTotalInTaxCurrency = r.decimal(c)
			case !inv.Totals.TaxTotal.IsSet():
				inv.Totals.TaxTotal = r.decimal(c)
			default:
				r.unread(c)
			}
		case c.Name == "TaxSubtotal":
			inv.VATBreakdown = append(inv.VATBreakdown, r.taxSubtotal(c))
		default:
			r.unread(c)
		}
	}
	r.leave()
}

func (r *reader) taxSubtotal(n *xmltree.Node) invoice.VATBreakdown {
	var vb invoice.VATBreakdown
	h := r.hold(n, &vb.Extensions)
	var baseDone, amtDone, catDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "TaxableAmount" && !baseDone:
			baseDone = true
			vb.TaxableAmount = r.decimal(c)
		case c.Name == "TaxAmount" && !amtDone:
			amtDone = true
			vb.TaxAmount = r.decimal(c)
		case c.Name == "TaxCategory" && !catDone:
			catDone = true
			vb.CategoryCode, vb.Rate = r.taxCategory(c, &vb, nil)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	return vb
}

func (r *reader) totals(n *xmltree.Node) {
	t := &r.inv.Totals
	r.enter(n)
	for _, c := range n.Children {
		var dst *invoice.Decimal
		switch c.Name {
		case "LineExtensionAmount":
			dst = &t.LineTotal
		case "TaxExclusiveAmount":
			dst = &t.TaxBasisTotal
		case "TaxInclusiveAmount":
			dst = &t.GrandTotal
		case "AllowanceTotalAmount":
			dst = &t.AllowanceTotal
		case "ChargeTotalAmount":
			dst = &t.ChargeTotal
		case "PrepaidAmount":
			dst = &t.PaidAmount
		case "PayableRoundingAmount":
			dst = &t.RoundingAmount
		case "PayableAmount":
			dst = &t.DuePayable
		}
		if dst == nil || dst.IsSet() {
			r.unread(c)
			continue
		}
		*dst = r.decimal(c)
	}
	r.leave()
}

func (r *reader) line(n *xmltree.Node, l *invoice.Line) {
	h := r.hold(n, &l.Extensions)
	var qtyDone, amtDone, periodDone, orderDone, docDone, itemDone, priceDone bool
	for _, c := range n.Children {
		switch c.Name {
		case "ID":
			if l.ID != "" {
				r.unread(c)
				continue
			}
			l.ID = r.text(c)
		case "Note":
			if l.Note != "" {
				r.unread(c)
				continue
			}
			l.Note = r.text(c)
		case "InvoicedQuantity", "CreditedQuantity":
			if qtyDone {
				r.unread(c)
				continue
			}
			qtyDone = true
			l.Quantity, l.UnitCode = r.quantity(c)
		case "LineExtensionAmount":
			if amtDone {
				r.unread(c)
				continue
			}
			amtDone = true
			l.NetAmount = r.decimal(c)
		case "AccountingCost":
			if l.AccountingReference != "" {
				r.unread(c)
				continue
			}
			l.AccountingReference = r.text(c)
		case "InvoicePeriod":
			if periodDone {
				r.unread(c)
				continue
			}
			periodDone = true
			r.enter(c)
			for _, k := range c.Children {
				switch {
				case k.Name == "StartDate" && l.Period.Start.IsZero():
					l.Period.Start = r.date(k)
				case k.Name == "EndDate" && l.Period.End.IsZero():
					l.Period.End = r.date(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case "OrderLineReference":
			if orderDone {
				r.unread(c)
				continue
			}
			orderDone = true
			r.orderLine(c, l)
		case "DespatchLineReference":
			if r.extHR && (l.Croatia == nil || l.Croatia.DespatchLine == nil) {
				lineCroatia(l).DespatchLine = r.lineReference(c)
				continue
			}
			if !r.extIT || !r.despatchLine(c, l) {
				r.unread(c)
			}
		case "ReceiptLineReference":
			if !r.extHR || l.Croatia != nil && l.Croatia.ReceiptLine != nil {
				r.unread(c)
				continue
			}
			lineCroatia(l).ReceiptLine = r.lineReference(c)
		case "DocumentReference":
			// The binding uses the line document reference for the invoice
			// line object identifier (BT-128) only, with the type code 130
			// that some producers leave out.
			if t := child(c, "DocumentTypeCode"); docDone || t != nil && textOf(t) != "130" {
				r.unread(c)
				continue
			}
			docDone = true
			r.enter(c)
			var idDone, typeDone bool
			for _, k := range c.Children {
				switch {
				case k.Name == "ID" && !idDone:
					idDone = true
					l.ObjectIdentifier = r.ident(k)
				case k.Name == "DocumentTypeCode" && !typeDone && len(k.Attrs) == 0:
					typeDone = true
					r.text(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case "AllowanceCharge":
			l.AllowancesCharges = append(l.AllowancesCharges, r.allowanceCharge(c, false))
		case "Item":
			if itemDone {
				r.unread(c)
				continue
			}
			itemDone = true
			r.item(c, l)
		case "Price":
			if priceDone {
				r.unread(c)
				continue
			}
			priceDone = true
			r.price(c, l)
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

func (r *reader) item(n *xmltree.Node, l *invoice.Line) {
	it := &l.Item
	r.enter(n)
	var buyerDone, sellerDone, stdDone, originDone, taxDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "Description" && it.Description == "":
			it.Description = r.text(c)
		case c.Name == "Name" && it.Name == "":
			it.Name = r.text(c)
		case c.Name == "BuyersItemIdentification" && !buyerDone:
			buyerDone = true
			it.BuyerID = r.itemID(c, nil)
		case c.Name == "SellersItemIdentification" && !sellerDone:
			sellerDone = true
			it.SellerID = r.itemID(c, nil)
		case c.Name == "StandardItemIdentification" && !stdDone:
			stdDone = true
			var id invoice.Identifier
			r.itemID(c, &id)
			it.GlobalID = id
		case c.Name == "OriginCountry" && !originDone:
			originDone = true
			r.enter(c)
			done := false
			for _, k := range c.Children {
				if k.Name == "IdentificationCode" && !done {
					done = true
					it.OriginCountry = r.text(k)
					continue
				}
				r.unread(k)
			}
			r.leave()
		case c.Name == "CommodityClassification" && child(c, "ItemClassificationCode") != nil:
			r.classification(c, it)
		case c.Name == "ClassifiedTaxCategory" && !taxDone:
			taxDone = true
			if !r.extHR {
				l.VATCategoryCode, l.VATRate = r.taxCategory(c, nil, nil)
				continue
			}
			var hr invoice.CroatiaTax
			l.VATCategoryCode, l.VATRate = r.taxCategory(c, nil, &hr)
			if hr != (invoice.CroatiaTax{}) {
				lc := lineCroatia(l)
				lc.Category, lc.ExemptionReason, lc.ExemptionReasonCode = hr.Category, hr.ExemptionReason, hr.ExemptionReasonCode
			}
		case c.Name == "AdditionalItemProperty":
			var a invoice.ItemAttribute
			h := r.hold(c, &a.Extensions)
			for _, k := range c.Children {
				switch {
				case k.Name == "Name" && a.Name == "":
					a.Name = r.text(k)
				case k.Name == "Value" && a.Value == "":
					a.Value = r.text(k)
				default:
					r.unread(k)
				}
			}
			r.release(h)
			it.Attributes = append(it.Attributes, a)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// itemID reads an item identification: its identifier, with its scheme
// when id is not nil.
func (r *reader) itemID(n *xmltree.Node, id *invoice.Identifier) string {
	r.enter(n)
	var s string
	done := false
	for _, c := range n.Children {
		if c.Name == "ID" && !done {
			done = true
			if id != nil {
				*id = r.ident(c)
				s = id.ID
			} else {
				s = r.text(c)
			}
			continue
		}
		r.unread(c)
	}
	r.leave()
	return s
}

func (r *reader) classification(n *xmltree.Node, it *invoice.Item) {
	var cl invoice.Classification
	h := r.hold(n, &cl.Extensions)
	done := false
	for _, c := range n.Children {
		if c.Name == "ItemClassificationCode" && !done {
			done = true
			cl.Code = r.textAttrs(c, "listID", "listVersionID", "")
			cl.ListID, cl.ListVersionID = attr(c, "listID"), attr(c, "listVersionID")
			continue
		}
		r.unread(c)
	}
	r.release(h)
	it.Classifications = append(it.Classifications, cl)
}

func (r *reader) price(n *xmltree.Node, l *invoice.Line) {
	r.enter(n)
	var amtDone, qtyDone, acDone bool
	for _, c := range n.Children {
		switch {
		case c.Name == "PriceAmount" && !amtDone:
			amtDone = true
			l.NetPrice = r.decimal(c)
		case c.Name == "BaseQuantity" && !qtyDone:
			qtyDone = true
			l.PriceBaseQuantity, l.PriceBaseQuantityUnit = r.quantity(c)
		case c.Name == "AllowanceCharge" && !acDone && textOf(child(c, "ChargeIndicator")) == "false":
			acDone = true
			r.enter(c)
			var indDone, amountDone, baseDone bool
			for _, k := range c.Children {
				switch {
				case k.Name == "ChargeIndicator" && !indDone && len(k.Attrs) == 0:
					indDone = true
					r.text(k)
				case k.Name == "Amount" && !amountDone:
					amountDone = true
					l.PriceDiscount = r.decimal(k)
				case k.Name == "BaseAmount" && !baseDone:
					baseDone = true
					l.GrossPrice = r.decimal(k)
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
