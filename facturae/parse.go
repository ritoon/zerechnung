package facturae

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

// Namespaces of the Facturae versions: the namespace of the root element
// Facturae, whose children are not qualified.
const (
	Namespace32  = "http://www.facturae.es/Facturae/2009/v3.2/Facturae"
	Namespace321 = "http://www.facturae.es/Facturae/2014/v3.2.1/Facturae"
	Namespace322 = "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"
	// NamespaceDS is the namespace of the XML signature (ds:Signature).
	NamespaceDS = "http://www.w3.org/2000/09/xmldsig#"
)

// versions gives the version of each Facturae namespace.
var versions = map[string]string{Namespace32: "3.2", Namespace321: "3.2.1", Namespace322: "3.2.2"}

// ErrNotFacturae is returned (wrapped) by Parse when the document is
// well-formed XML but not a Facturae 3.2, 3.2.1 or 3.2.2 document, for
// example a CII invoice or a Facturae 3.1 document.
var ErrNotFacturae = errors.New("facturae: not a Facturae 3.2, 3.2.1 or 3.2.2 document")

// Issue is a value that could not be read: a malformed amount or date, an
// unknown country code, a document without invoice. The corresponding field
// of the invoice is left empty.
type Issue struct {
	Path    string // location of the element, as given by xmltree.Node.Path
	Line    int    // line of the element in the document, 1-based
	Message string
}

// String formats the issue as "line <Line>: <Path>: <Message>".
func (i Issue) String() string {
	return "line " + strconv.Itoa(i.Line) + ": " + i.Path + ": " + i.Message
}

// IssuesError is the error returned by Parse and ParseAll, together with the
// invoices, when some values could not be read. Everything else was read: a
// caller that tolerates incomplete data can use the invoices and inspect
// Issues with errors.As.
type IssuesError struct {
	Issues []Issue
}

// Error gives the number of issues and the first one.
func (e *IssuesError) Error() string {
	if len(e.Issues) == 1 {
		return "facturae: unreadable value at " + e.Issues[0].String()
	}
	return "facturae: " + strconv.Itoa(len(e.Issues)) + " unreadable values, first at " + e.Issues[0].String()
}

// Version returns the Facturae version of the document whose root element
// is root: "3.2", "3.2.1" or "3.2.2", or "" when root is not the element
// Facturae in the namespace of one of these versions.
func Version(root *xmltree.Node) string {
	if root == nil || root.Name != "Facturae" {
		return ""
	}
	return versions[root.Space]
}

// IsFacturae reports whether root is the root element of a Facturae 3.2,
// 3.2.1 or 3.2.2 document.
func IsFacturae(root *xmltree.Node) bool { return Version(root) != "" }

// Parse reads a Facturae document that holds a single invoice. For a
// document with several invoices (a batch), it returns the first invoice
// and an *IssuesError that says how many there are: ParseAll reads them
// all.
//
// It fails, with a nil invoice, only when data is not well-formed XML or its
// root is not Facturae in the namespace of version 3.2, 3.2.1 or 3.2.2 (the
// error then wraps ErrNotFacturae). When some values cannot be read, Parse
// returns the invoice with the corresponding fields empty and an
// *IssuesError listing them.
func Parse(data []byte) (*invoice.Invoice, error) {
	invs, err := ParseAll(data)
	if len(invs) == 0 {
		return nil, err
	}
	if len(invs) > 1 {
		is := Issue{Message: "the document holds " + strconv.Itoa(len(invs)) + " invoices (batch); only the first is read, ParseAll reads them all"}
		var ie *IssuesError
		if errors.As(err, &ie) {
			ie.Issues = append(ie.Issues, is)
		} else {
			err = &IssuesError{Issues: []Issue{is}}
		}
	}
	return invs[0], err
}

// ParseAll reads a Facturae document: one invoice per Invoice element, each
// with the data of the file header and of the parties.
//
// The data that EN 16931 carries goes to the EN 16931 fields, as the
// documentation of the package describes it, and the rest to the national
// blocks of the model (see invoice.Spain). The invoices have the EN 16931
// profile (invoice.ProfileEN16931). The totals that Facturae gives are read
// as they are (BT-106 to BT-109, BT-113); the VAT total (BT-110) is the sum
// of the VAT breakdown, the total with VAT (BT-112) the total without VAT
// plus the VAT total, and the amount due (BT-115) the total with VAT minus
// the paid amount, as EN 16931 defines them: the totals of Facturae that
// these do not give (InvoiceTotal, TotalOutstandingAmount,
// TotalExecutableAmount) are kept in the national block when they differ
// from the ones derived from the data read.
//
// Nothing is dropped, except the XAdES signature and the values that
// Marshal derives (the number of invoices of the batch, the totals of
// Facturae): the elements and attributes that the format does not define,
// the content of the Extensions elements and the factoring assignment
// (FactoringAssignmentData) are kept as extensions (see invoice.Extension),
// in the Extensions field of the value that encloses them (the invoice, a
// party, a line), with paths made of the local names of the elements
// ("Invoices/Invoice/AdditionalData"). Invoice.Unmodeled lists them.
func ParseAll(data []byte) ([]*invoice.Invoice, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("facturae: %w", err)
	}
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	invs, issues := ParseTree(root)
	if len(issues) > 0 {
		return invs, &IssuesError{Issues: issues}
	}
	return invs, nil
}

// ParseTree reads a Facturae document from an already parsed tree (see
// ParseAll). If root is not a Facturae document, it returns no invoice and
// a single issue that says why.
func ParseTree(root *xmltree.Node) ([]*invoice.Invoice, []Issue) {
	if err := checkRoot(root); err != nil {
		is := Issue{Message: err.Error()}
		if root != nil {
			is.Path, is.Line = root.Path(), root.Line
		}
		return nil, []Issue{is}
	}
	r := &reader{ns: root.Space, version: versions[root.Space]}
	var header, parties, invoices *xmltree.Node
	var other []*xmltree.Node
	for _, c := range root.Children {
		switch {
		case c.Space == NamespaceDS && c.Name == "Signature":
		case !r.format(c):
			other = append(other, c)
		case c.Name == "FileHeader" && header == nil:
			header = c
		case c.Name == "Parties" && parties == nil:
			parties = c
		case c.Name == "Invoices" && invoices == nil:
			invoices = c
		default:
			other = append(other, c)
		}
	}
	for _, n := range []struct {
		node *xmltree.Node
		name string
	}{{header, "FileHeader"}, {parties, "Parties"}, {invoices, "Invoices"}} {
		if n.node == nil {
			r.issue(root, "no "+n.name)
		}
	}
	var elems, rest []*xmltree.Node
	if invoices != nil {
		for _, c := range invoices.Children {
			if r.format(c) && c.Name == "Invoice" {
				elems = append(elems, c)
			} else {
				rest = append(rest, c)
			}
		}
		if len(elems) == 0 {
			r.issue(invoices, "no Invoice")
		}
	}
	if len(elems) == 0 {
		elems = append(elems, nil)
	}
	invs := make([]*invoice.Invoice, 0, len(elems))
	for i, n := range elems {
		inv := &invoice.Invoice{Profile: invoice.ProfileEN16931, Spain: new(invoice.Spain)}
		r.inv, r.sp = inv, inv.Spain
		r.lines, r.vatTypes, r.amounts, r.dues, r.reconciliations = nil, nil, nil, nil, nil
		r.batchTotal, r.invoiceTotal, r.outstanding, r.executable = invoice.Decimal{}, invoice.Decimal{}, invoice.Decimal{}, invoice.Decimal{}
		r.reimbursableTotal = invoice.Decimal{}
		// The header and the parties are read again for each invoice, so
		// that the invoices share no value; their issues are reported once.
		r.quiet = i > 0
		h := r.hold(root, &inv.Extensions)
		if i == 0 {
			for _, c := range other {
				r.unread(c)
			}
		}
		if header != nil {
			r.fileHeader(header)
		}
		if parties != nil {
			r.parties(parties)
		}
		r.quiet = false
		if invoices != nil {
			r.enter(invoices)
			if i == 0 {
				for _, c := range rest {
					r.unread(c)
				}
			}
			if n != nil {
				r.invoice(n)
			}
			r.leave()
		}
		r.release(h)
		r.finish(inv)
		invs = append(invs, inv)
	}
	return invs, r.issues
}

func checkRoot(root *xmltree.Node) error {
	switch {
	case root == nil:
		return ErrNotFacturae
	case Version(root) != "":
		return nil
	case root.Name == "Facturae":
		return fmt.Errorf("%w: root element <Facturae> in namespace %q", ErrNotFacturae, root.Space)
	}
	return fmt.Errorf("%w: root element is <%s>", ErrNotFacturae, root.Name)
}

// reader maps a Facturae tree to the model.
//
// Every element and attribute of the document is either read into a field of
// the model or kept as an extension: each container function handles the
// children it knows and passes the others to unread. Extensions go to the
// value being read whose Extensions field is ext, with a path relative to the
// element of that value (path holds the names of the elements).
type reader struct {
	ns      string // namespace of the root element
	version string
	issues  []Issue
	quiet   bool // the header is read again: its issues are already reported

	ext  *[]invoice.Extension
	path []string

	inv *invoice.Invoice
	sp  *invoice.Spain

	// Data of the invoice that is mapped once the invoice is read.
	lines           []lineTax         // tax data of each line
	vatTypes        []string          // TaxTypeCode of each VAT breakdown
	amounts         []invoice.Decimal // InstallmentAmount of each payment means
	dues            []invoice.Date    // InstallmentDueDate of each payment means
	reconciliations []string          // PaymentReconciliationReference of each payment means
	batchTotal      invoice.Decimal   // TotalInvoicesAmount of the batch
	invoiceTotal    invoice.Decimal   // InvoiceTotal
	outstanding     invoice.Decimal   // TotalOutstandingAmount
	executable      invoice.Decimal   // TotalExecutableAmount

	reimbursableTotal invoice.Decimal // TotalReimbursableExpenses

	buf []byte
}

// lineTax is the tax data of a line that sets its VAT category.
type lineTax struct {
	taxType string // TaxTypeCode of its VAT
	event   string // SpecialTaxableEventCode
	reason  string // SpecialTaxableEventReason
}

func (r *reader) issue(n *xmltree.Node, msg string) {
	if !r.quiet {
		r.issues = append(r.issues, Issue{Path: n.Path(), Line: n.Line, Message: msg})
	}
}

// format reports whether c is an element of the format: its children are
// not qualified, or, tolerated, qualified with the namespace of the root.
func (r *reader) format(c *xmltree.Node) bool { return c.Space == "" || c.Space == r.ns }

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
	r.keepAttrs(n, "")
	return h
}

func (r *reader) release(h holder) { r.ext, r.path = h.ext, h.path }

// enter starts reading the children of the container n: its attributes are
// kept as extensions and its name extends the path.
func (r *reader) enter(n *xmltree.Node) {
	r.keepAttrs(n, r.attrPath(n))
	r.path = append(r.path, qname(n))
}

func (r *reader) leave() { r.path = r.path[:len(r.path)-1] }

// unread keeps n, a child of the current container, as an extension.
func (r *reader) unread(n *xmltree.Node) {
	r.buf = appendElement(r.buf[:0], n)
	*r.ext = append(*r.ext, invoice.Extension{Path: strings.Join(r.path, "/"), XML: string(r.buf)})
}

// keepAttrs keeps the attributes of n, whose location is path (see
// attrPath). Attributes in another namespace (xsi:schemaLocation…) are not
// part of the data.
func (r *reader) keepAttrs(n *xmltree.Node, path string) {
	for _, at := range n.Attrs {
		if at.Prefix != "" {
			continue
		}
		*r.ext = append(*r.ext, invoice.Extension{Path: path, Attr: at.Name, Value: at.Value})
	}
}

// attrPath returns the location of n, a child of the current container:
// the current path followed by the name of n, with its occurrence number
// when it is not the first child of that name.
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
	return strings.Join(r.path, "/") + "/" + name
}

// qname returns the name of n with its prefix, which only the root element
// and the elements of other namespaces have.
func qname(n *xmltree.Node) string {
	if n.Prefix != "" && !versionSpace(n.Space) {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

// versionSpace reports whether ns is the namespace of a Facturae version.
func versionSpace(ns string) bool { _, ok := versions[ns]; return ok }

// fields reads the children of the container n: f handles each element of
// the format the first time it appears and returns false for an element it
// does not know. Unknown and repeated elements are kept as extensions.
func (r *reader) fields(n *xmltree.Node, f func(c *xmltree.Node) bool) {
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.format(c) || seen[c.Name] || !f(c) {
			r.unread(c)
			continue
		}
		seen[c.Name] = true
	}
	r.leave()
}

// list reads the children named name of the container n, a list such as
// TaxesOutputs or Items, with f; the other children are kept as
// extensions.
func (r *reader) list(n *xmltree.Node, name string, f func(c *xmltree.Node)) {
	r.enter(n)
	for _, c := range n.Children {
		if r.format(c) && c.Name == name {
			f(c)
		} else {
			r.unread(c)
		}
	}
	r.leave()
}

// text returns the trimmed text of the leaf n ("" for nil) and keeps its
// attributes and child elements.
func (r *reader) text(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	r.keepAttrs(n, r.attrPath(n))
	if len(n.Children) > 0 {
		r.path = append(r.path, qname(n))
		for _, k := range n.Children {
			r.unread(k)
		}
		r.leave()
	}
	return strings.TrimSpace(n.Text)
}

// decimal reads an amount, a rate or a quantity (xs:double).
func (r *reader) decimal(n *xmltree.Node) invoice.Decimal {
	s := r.text(n)
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

// date reads a date (xs:date, 2006-01-02).
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

// amount reads an AmountType: TotalAmount and EquivalentInEuros.
func (r *reader) amount(n *xmltree.Node) (total, euros invoice.Decimal) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TotalAmount":
			total = r.decimal(c)
		case "EquivalentInEuros":
			euros = r.decimal(c)
		default:
			return false
		}
		return true
	})
	return total, euros
}

// period reads a PeriodDates: StartDate and EndDate.
func (r *reader) period(n *xmltree.Node) invoice.Period {
	var p invoice.Period
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "StartDate":
			p.Start = r.date(c)
		case "EndDate":
			p.End = r.date(c)
		default:
			return false
		}
		return true
	})
	return p
}

// country reads a country code (CountryType, ISO 3166-1 alpha-3) as its
// alpha-2 code; a code without alpha-2 code is kept as written.
func (r *reader) country(n *xmltree.Node) string {
	s := r.text(n)
	if s == "" {
		return ""
	}
	a2, ok := countryOf(s)
	if !ok {
		r.issue(n, "country code "+strconv.Quote(s)+" has no ISO 3166-1 alpha-2 code")
		return s
	}
	return a2
}

// fileHeader reads FileHeader.
func (r *reader) fileHeader(n *xmltree.Node) {
	sp := r.sp
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "SchemaVersion":
			if v := r.text(c); v != r.version {
				r.issue(c, "SchemaVersion "+strconv.Quote(v)+" in a document of version "+r.version)
			}
		case "Modality":
			sp.Modality = r.text(c)
		case "InvoiceIssuerType":
			sp.IssuerType = r.text(c)
		case "ThirdParty":
			p := new(invoice.Party)
			r.taxParty(c, p)
			sp.ThirdParty = p
		case "Batch":
			r.batch(c)
		default:
			// FactoringAssignmentData is kept as an extension.
			return false
		}
		return true
	})
}

// batch reads Batch. The number of invoices and the totals are those of the
// invoices; only the identifier and the equivalents in euros are kept.
func (r *reader) batch(n *xmltree.Node) {
	sp := r.sp
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "BatchIdentifier":
			sp.BatchID = r.text(c)
		case "InvoicesCount":
			r.text(c)
		case "TotalInvoicesAmount":
			r.batchTotal, sp.BatchTotalEuros = r.amount(c)
		case "TotalOutstandingAmount":
			_, sp.BatchOutstandingEuros = r.amount(c)
		case "TotalExecutableAmount":
			_, sp.BatchExecutableEuros = r.amount(c)
		case "InvoiceCurrencyCode":
			if r.inv.Currency == "" {
				r.inv.Currency = r.text(c)
			} else {
				r.text(c)
			}
		default:
			return false
		}
		return true
	})
}

// parties reads Parties.
func (r *reader) parties(n *xmltree.Node) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "SellerParty":
			r.business(c, &r.inv.Seller)
		case "BuyerParty":
			r.business(c, &r.inv.Buyer)
		default:
			return false
		}
		return true
	})
}

// business reads a BusinessType: the seller or the buyer.
func (r *reader) business(n *xmltree.Node, p *invoice.Party) {
	sp := new(invoice.SpainParty)
	p.Spain = sp
	h := r.hold(n, &p.Extensions)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.format(c) || seen[c.Name] {
			r.unread(c)
			continue
		}
		seen[c.Name] = true
		switch c.Name {
		case "TaxIdentification":
			r.taxIdentification(c, p, sp)
		case "PartyIdentification":
			p.IDs = append(p.IDs, invoice.Identifier{ID: r.text(c)})
		case "AdministrativeCentres":
			r.list(c, "AdministrativeCentre", func(k *xmltree.Node) {
				sp.Centres = append(sp.Centres, r.centre(k))
			})
		case "LegalEntity":
			r.legalEntity(c, p, sp)
		case "Individual":
			r.individual(c, p, sp)
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

// taxParty reads a party made of a tax identification and a legal entity
// or an individual (ThirdParty), whose extensions go to the invoice.
func (r *reader) taxParty(n *xmltree.Node, p *invoice.Party) {
	sp := new(invoice.SpainParty)
	p.Spain = sp
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TaxIdentification":
			r.taxIdentification(c, p, sp)
		case "LegalEntity":
			r.legalEntity(c, p, sp)
		case "Individual":
			r.individual(c, p, sp)
		default:
			return false
		}
		return true
	})
}

// taxIdentification reads the TaxIdentification of a party: the person and
// residence types are kept in the national block until finish compares
// them with the derived ones, the tax identification number is the VAT
// identifier (with the prefix ES for a resident in Spain written without
// it) or, for a party outside Spain whose number has no country prefix, the
// legal registration identifier.
func (r *reader) taxIdentification(n *xmltree.Node, p *invoice.Party, sp *invoice.SpainParty) {
	t := r.taxID(n)
	sp.PersonType, sp.ResidenceType, sp.TaxID = t.PersonType, t.ResidenceType, t.Number
	switch tin := t.Number; {
	case tin == "":
	case vatPrefix(tin) != "":
		p.VATID = tin
	case t.ResidenceType == "R" || t.ResidenceType == "":
		p.VATID = "ES" + tin
	default:
		p.LegalRegistration = invoice.Identifier{ID: tin}
	}
}

// taxID reads a TaxIdentificationType as written.
func (r *reader) taxID(n *xmltree.Node) invoice.TaxIdentification {
	var t invoice.TaxIdentification
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "PersonTypeCode":
			t.PersonType = r.text(c)
		case "ResidenceTypeCode":
			t.ResidenceType = r.text(c)
		case "TaxIdentificationNumber":
			t.Number = r.text(c)
		default:
			return false
		}
		return true
	})
	return t
}

// legalEntity reads a LegalEntity.
func (r *reader) legalEntity(n *xmltree.Node, p *invoice.Party, sp *invoice.SpainParty) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "CorporateName":
			p.Name = r.text(c)
		case "TradeName":
			p.TradingName = r.text(c)
		case "RegistrationData":
			sp.Registration = r.registration(c)
		case "AddressInSpain", "OverseasAddress":
			p.Address = r.address(c)
		case "ContactDetails":
			r.contactDetails(c, &p.Contact, &sp.Fax, &sp.Web, &sp.CNAE, &sp.INETownCode, &sp.OtherContact)
		default:
			return false
		}
		return true
	})
}

// individual reads an Individual: the name of the party is the given name
// followed by the surnames.
func (r *reader) individual(n *xmltree.Node, p *invoice.Party, sp *invoice.SpainParty) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Name":
			sp.GivenName = r.text(c)
		case "FirstSurname":
			sp.FirstSurname = r.text(c)
		case "SecondSurname":
			sp.SecondSurname = r.text(c)
		case "AddressInSpain", "OverseasAddress":
			p.Address = r.address(c)
		case "ContactDetails":
			r.contactDetails(c, &p.Contact, &sp.Fax, &sp.Web, &sp.CNAE, &sp.INETownCode, &sp.OtherContact)
		default:
			return false
		}
		return true
	})
	p.Name = invoice.SpanishName(sp.GivenName, sp.FirstSurname, sp.SecondSurname)
}

// registration reads RegistrationData.
func (r *reader) registration(n *xmltree.Node) *invoice.Registration {
	reg := new(invoice.Registration)
	dst := map[string]*string{
		"Book": &reg.Book, "RegisterOfCompaniesLocation": &reg.Location, "Sheet": &reg.Sheet, "Folio": &reg.Folio,
		"Section": &reg.Section, "Volume": &reg.Volume, "AdditionalRegistrationData": &reg.Additional,
	}
	r.strings(n, dst)
	return reg
}

// strings reads the leaves of the container n into dst, by name.
func (r *reader) strings(n *xmltree.Node, dst map[string]*string) {
	r.fields(n, func(c *xmltree.Node) bool {
		p, ok := dst[c.Name]
		if ok {
			*p = r.text(c)
		}
		return ok
	})
}

// address reads an AddressInSpain (Address, PostCode, Town, Province,
// CountryCode) or an OverseasAddress (Address, PostCodeAndTown, Province,
// CountryCode). The PostCodeAndTown of an address outside Spain is its city
// (BT-37…).
func (r *reader) address(n *xmltree.Node) invoice.Address {
	var a invoice.Address
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "Address":
			a.Line1 = r.text(c)
		case "PostCode":
			a.PostCode = r.text(c)
		case "Town", "PostCodeAndTown":
			a.City = r.text(c)
		case "Province":
			a.CountrySubdivision = r.text(c)
		case "CountryCode":
			a.CountryCode = r.country(c)
		default:
			return false
		}
		return true
	})
	return a
}

// contactDetails reads ContactDetails: the telephone, the e-mail and the
// contact person are the contact of the party (BG-6, BG-9), the other
// details go to the national block.
func (r *reader) contactDetails(n *xmltree.Node, contact **invoice.Contact, fax, web, cnae, ine, other *string) {
	ct := new(invoice.Contact)
	r.strings(n, map[string]*string{
		"Telephone": &ct.Phone, "TeleFax": fax, "WebAddress": web, "ElectronicMail": &ct.Email,
		"ContactPersons": &ct.PersonName, "CnoCnae": cnae, "INETownCode": ine, "AdditionalContactDetails": other,
	})
	if *ct != (invoice.Contact{}) {
		*contact = ct
	}
}

// centre reads an AdministrativeCentre.
func (r *reader) centre(n *xmltree.Node) invoice.AdministrativeCentre {
	var ac invoice.AdministrativeCentre
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "CentreCode":
			ac.Code = r.text(c)
		case "RoleTypeCode":
			ac.Role = r.text(c)
		case "Name":
			ac.Name = r.text(c)
		case "FirstSurname":
			ac.FirstSurname = r.text(c)
		case "SecondSurname":
			ac.SecondSurname = r.text(c)
		case "AddressInSpain", "OverseasAddress":
			ac.Address = r.address(c)
		case "ContactDetails":
			r.contactDetails(c, &ac.Contact, &ac.Fax, &ac.Web, &ac.CNAE, &ac.INETownCode, &ac.OtherContact)
		case "PhysicalGLN":
			ac.PhysicalGLN = r.text(c)
		case "LogicalOperationalPoint":
			ac.LogicalPoint = r.text(c)
		case "CentreDescription":
			ac.Description = r.text(c)
		default:
			return false
		}
		return true
	})
	return ac
}

// invoice reads an Invoice.
func (r *reader) invoice(n *xmltree.Node) {
	inv := r.inv
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "InvoiceHeader":
			r.invoiceHeader(c)
		case "InvoiceIssueData":
			r.issueData(c)
		case "TaxesOutputs":
			r.list(c, "Tax", r.invoiceTax)
		case "TaxesWithheld":
			r.list(c, "Tax", func(k *xmltree.Node) { r.sp.Withholdings = append(r.sp.Withholdings, r.tax(k)) })
		case "InvoiceTotals":
			r.totals(c)
		case "Items":
			r.list(c, "InvoiceLine", func(k *xmltree.Node) {
				inv.Lines = append(inv.Lines, invoice.Line{})
				r.line(k, &inv.Lines[len(inv.Lines)-1])
			})
		case "PaymentDetails":
			r.list(c, "Installment", r.installment)
		case "LegalLiterals":
			r.list(c, "LegalReference", func(k *xmltree.Node) {
				inv.Notes = append(inv.Notes, invoice.Note{SubjectCode: "ABL", Content: r.text(k)})
			})
		case "AdditionalData":
			r.additionalData(c)
		default:
			return false
		}
		return true
	})
}

// invoiceHeader reads InvoiceHeader: the invoice number (BT-1) is the
// series followed by the number.
func (r *reader) invoiceHeader(n *xmltree.Node) {
	inv, sp := r.inv, r.sp
	var number string
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "InvoiceNumber":
			number = r.text(c)
		case "InvoiceSeriesCode":
			sp.SeriesCode = r.text(c)
		case "InvoiceDocumentType":
			sp.DocumentType = r.text(c)
		case "InvoiceClass":
			sp.Class = r.text(c)
		case "Corrective":
			r.corrective(c)
		default:
			return false
		}
		return true
	})
	inv.Number = sp.SeriesCode + number
	inv.TypeCode = typeCodeOf(sp.DocumentType, sp.Class)
}

// typeCodeOf returns the invoice type code (BT-3) of a Facturae document
// type and class: 471 (corrective self-billed invoice) for a corrective
// self-billing (OR and AF), 384 (corrected invoice) for another corrective
// invoice, 389 (self-billed invoice) for a self-billing (AF), 380 otherwise.
func typeCodeOf(docType, class string) string {
	switch {
	case class == invoice.SpanishCorrective && docType == "AF":
		return "471"
	case class == invoice.SpanishCorrective:
		return "384"
	case docType == "AF":
		return "389"
	}
	return "380"
}

// corrective reads Corrective: the corrected invoice is the preceding
// invoice reference (BG-3).
func (r *reader) corrective(n *xmltree.Node) {
	co := new(invoice.Corrective)
	var number string
	var date invoice.Date
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "InvoiceNumber":
			number = r.text(c)
		case "InvoiceSeriesCode":
			co.SeriesCode = r.text(c)
		case "ReasonCode":
			co.ReasonCode = r.text(c)
		case "ReasonDescription":
			co.ReasonDescription = r.text(c)
		case "TaxPeriod":
			co.TaxPeriod = r.period(c)
		case "CorrectionMethod":
			co.Method = r.text(c)
		case "CorrectionMethodDescription":
			co.MethodDescription = r.text(c)
		case "AdditionalReasonDescription":
			co.AdditionalReason = r.text(c)
		case "InvoiceIssueDate":
			date = r.date(c)
		default:
			return false
		}
		return true
	})
	if co.ReasonDescription == invoice.CorrectiveReason(co.ReasonCode) {
		co.ReasonDescription = ""
	}
	if co.MethodDescription == invoice.CorrectionMethodName(co.Method) {
		co.MethodDescription = ""
	}
	if number != "" || co.SeriesCode != "" || !date.IsZero() {
		r.inv.PrecedingInvoices = append(r.inv.PrecedingInvoices, invoice.DocumentReference{ID: co.SeriesCode + number, IssueDate: date})
	}
	r.sp.Corrective = co
}

// issueData reads InvoiceIssueData.
func (r *reader) issueData(n *xmltree.Node) {
	inv, sp := r.inv, r.sp
	var rate invoice.Decimal
	var rateDate invoice.Date
	exchange := false
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "IssueDate":
			inv.IssueDate = r.date(c)
		case "OperationDate":
			inv.TaxPointDate = r.date(c)
		case "PlaceOfIssue":
			pl := new(invoice.PlaceOfIssue)
			r.strings(c, map[string]*string{"PostCode": &pl.PostCode, "PlaceOfIssueDescription": &pl.Description})
			sp.PlaceOfIssue = pl
		case "InvoicingPeriod":
			inv.InvoicingPeriod = r.period(c)
		case "InvoiceCurrencyCode":
			inv.Currency = r.text(c)
		case "ExchangeRateDetails":
			exchange = true
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "ExchangeRate":
					rate = r.decimal(k)
				case "ExchangeRateDate":
					rateDate = r.date(k)
				default:
					return false
				}
				return true
			})
		case "TaxCurrencyCode":
			inv.TaxCurrency = r.text(c)
		case "LanguageName":
			inv.Language = r.text(c)
		case "InvoiceDescription":
			inv.Notes = append(inv.Notes, invoice.Note{Content: r.text(c)})
		case "ReceiverTransactionReference":
			inv.PurchaseOrderReference = r.text(c)
		case "FileReference":
			inv.AdditionalDocuments = append(inv.AdditionalDocuments, invoice.AdditionalDocument{ID: r.text(c), TypeCode: "50"})
		case "ReceiverContractReference":
			inv.ContractReference = r.text(c)
		default:
			return false
		}
		return true
	})
	if inv.TaxCurrency == inv.Currency {
		inv.TaxCurrency = ""
	}
	if inv.Language == "es" {
		inv.Language = ""
	}
	if exchange {
		target := inv.TaxCurrency
		if target == "" {
			target = "EUR"
		}
		inv.CurrencyExchange = &invoice.CurrencyExchange{SourceCurrency: inv.Currency, TargetCurrency: target, Rate: rate, Date: rateDate}
	}
}

// taxOutput is a tax of TaxesOutputs as written.
type taxOutput struct {
	invoice.SpainTax
	vat invoice.SpainVAT // TaxableEuros, TaxEuros and the data of Output
}

// output reads a TaxOutputType.
func (r *reader) output(n *xmltree.Node) taxOutput {
	var t taxOutput
	v := &t.vat
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TaxTypeCode":
			t.Type = r.text(c)
		case "TaxRate":
			t.Rate = r.decimal(c)
		case "TaxableBase":
			t.TaxableBase, v.TaxableEuros = r.amount(c)
		case "TaxAmount":
			t.Amount, v.TaxEuros = r.amount(c)
		case "SpecialTaxableBase":
			v.SpecialBase, v.SpecialBaseEuros = r.amount(c)
		case "SpecialTaxAmount":
			v.SpecialAmount, v.SpecialAmountEuros = r.amount(c)
		case "EquivalenceSurcharge":
			v.SurchargeRate = r.decimal(c)
		case "EquivalenceSurchargeAmount":
			v.SurchargeAmount, v.SurchargeEuros = r.amount(c)
		default:
			return false
		}
		return true
	})
	return t
}

// spainTax returns t as a tax of the national block.
func (t *taxOutput) spainTax() invoice.SpainTax {
	st := t.SpainTax
	st.TaxableEuros, st.AmountEuros = t.vat.TaxableEuros, t.vat.TaxEuros
	o := t.vat
	o.TaxableEuros, o.TaxEuros = invoice.Decimal{}, invoice.Decimal{}
	if o != (invoice.SpainVAT{}) {
		st.Output = &o
	}
	return st
}

// tax reads a TaxType, a tax withheld.
func (r *reader) tax(n *xmltree.Node) invoice.SpainTax {
	var t invoice.SpainTax
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TaxTypeCode":
			t.Type = r.text(c)
		case "TaxRate":
			t.Rate = r.decimal(c)
		case "TaxableBase":
			t.TaxableBase, t.TaxableEuros = r.amount(c)
		case "TaxAmount":
			t.Amount, t.AmountEuros = r.amount(c)
		default:
			return false
		}
		return true
	})
	return t
}

// invoiceTax reads a Tax of the TaxesOutputs of the invoice: a tax of type
// 01, 02 or 03 is a VAT breakdown (BG-23), whose category is set by finish,
// the others go to the national block.
func (r *reader) invoiceTax(n *xmltree.Node) {
	t := r.output(n)
	if !invoice.IsSpanishVAT(t.Type) {
		r.sp.Taxes = append(r.sp.Taxes, t.spainTax())
		return
	}
	v := invoice.VATBreakdown{Rate: t.Rate, TaxableAmount: t.TaxableBase, TaxAmount: t.Amount}
	if t.vat != (invoice.SpainVAT{}) {
		sv := t.vat
		v.Spain = &sv
	}
	r.inv.VATBreakdown = append(r.inv.VATBreakdown, v)
	r.vatTypes = append(r.vatTypes, t.Type)
}

// totals reads InvoiceTotals.
func (r *reader) totals(n *xmltree.Node) {
	inv, sp, t := r.inv, r.sp, &r.inv.Totals
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "TotalGrossAmount":
			t.LineTotal = r.decimal(c)
		case "GeneralDiscounts":
			r.list(c, "Discount", func(k *xmltree.Node) { inv.AllowancesCharges = append(inv.AllowancesCharges, r.discount(k, false)) })
		case "GeneralSurcharges":
			r.list(c, "Charge", func(k *xmltree.Node) { inv.AllowancesCharges = append(inv.AllowancesCharges, r.discount(k, true)) })
		case "TotalGeneralDiscounts":
			t.AllowanceTotal = r.decimal(c)
		case "TotalGeneralSurcharges":
			t.ChargeTotal = r.decimal(c)
		case "TotalGrossAmountBeforeTaxes":
			t.TaxBasisTotal = r.decimal(c)
		case "TotalTaxOutputs", "TotalTaxesWithheld":
			r.decimal(c) // derived from the taxes
		case "InvoiceTotal":
			r.invoiceTotal = r.decimal(c)
		case "Subsidies":
			r.list(c, "Subsidy", func(k *xmltree.Node) {
				var s invoice.Subsidy
				r.fields(k, func(e *xmltree.Node) bool {
					switch e.Name {
					case "SubsidyDescription":
						s.Description = r.text(e)
					case "SubsidyRate":
						s.Rate = r.decimal(e)
					case "SubsidyAmount":
						s.Amount = r.decimal(e)
					default:
						return false
					}
					return true
				})
				sp.Subsidies = append(sp.Subsidies, s)
			})
		case "PaymentsOnAccount":
			r.list(c, "PaymentOnAccount", func(k *xmltree.Node) {
				var p invoice.PaymentOnAccount
				r.fields(k, func(e *xmltree.Node) bool {
					switch e.Name {
					case "PaymentOnAccountDate":
						p.Date = r.date(e)
					case "PaymentOnAccountAmount":
						p.Amount = r.decimal(e)
					default:
						return false
					}
					return true
				})
				sp.PaymentsOnAccount = append(sp.PaymentsOnAccount, p)
			})
		case "ReimbursableExpenses":
			r.list(c, "ReimbursableExpenses", func(k *xmltree.Node) {
				sp.ReimbursableExpenses = append(sp.ReimbursableExpenses, r.reimbursable(k))
			})
		case "TotalFinancialExpenses":
			sp.FinancialExpenses = r.decimal(c)
		case "TotalOutstandingAmount":
			r.outstanding = r.decimal(c)
		case "TotalPaymentsOnAccount":
			t.PaidAmount = r.decimal(c)
		case "AmountsWithheld":
			a := new(invoice.AmountWithheld)
			r.fields(c, func(e *xmltree.Node) bool {
				switch e.Name {
				case "WithholdingReason":
					a.Reason = r.text(e)
				case "WithholdingRate":
					a.Rate = r.decimal(e)
				case "WithholdingAmount":
					a.Amount = r.decimal(e)
				default:
					return false
				}
				return true
			})
			sp.AmountWithheld = a
		case "TotalExecutableAmount":
			r.executable = r.decimal(c)
		case "TotalReimbursableExpenses":
			r.reimbursableTotal = r.decimal(c)
		case "PaymentInKind":
			p := new(invoice.PaymentInKind)
			r.fields(c, func(e *xmltree.Node) bool {
				switch e.Name {
				case "PaymentInKindReason":
					p.Reason = r.text(e)
				case "PaymentInKindAmount":
					p.Amount = r.decimal(e)
				default:
					return false
				}
				return true
			})
			sp.PaymentInKind = p
		default:
			return false
		}
		return true
	})
}

// discount reads a Discount (DiscountReason, DiscountRate, DiscountAmount)
// or a Charge (ChargeReason, ChargeRate, ChargeAmount) as an allowance or a
// charge.
func (r *reader) discount(n *xmltree.Node, charge bool) invoice.AllowanceCharge {
	ac := invoice.AllowanceCharge{Charge: charge}
	prefix := "Discount"
	if charge {
		prefix = "Charge"
	}
	r.fields(n, func(c *xmltree.Node) bool {
		switch strings.TrimPrefix(c.Name, prefix) {
		case "Reason":
			ac.Reason = r.text(c)
		case "Rate":
			ac.Percentage = r.decimal(c)
		case "Amount":
			ac.Amount = r.decimal(c)
		default:
			return false
		}
		return true
	})
	return ac
}

// reimbursable reads a ReimbursableExpensesType.
func (r *reader) reimbursable(n *xmltree.Node) invoice.ReimbursableExpense {
	var e invoice.ReimbursableExpense
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "ReimbursableExpensesSellerParty":
			t := r.taxID(c)
			e.Seller = &t
		case "ReimbursableExpensesBuyerParty":
			t := r.taxID(c)
			e.Buyer = &t
		case "IssueDate":
			e.IssueDate = r.date(c)
		case "InvoiceNumber":
			e.Number = r.text(c)
		case "InvoiceSeriesCode":
			e.SeriesCode = r.text(c)
		case "ReimbursableExpensesAmount":
			e.Amount = r.decimal(c)
		default:
			return false
		}
		return true
	})
	return e
}

// line reads an InvoiceLine.
func (r *reader) line(n *xmltree.Node, l *invoice.Line) {
	sl := new(invoice.SpainLine)
	var lt lineTax
	var vat *taxOutput
	h := r.hold(n, &l.Extensions)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.format(c) || seen[c.Name] {
			r.unread(c)
			continue
		}
		seen[c.Name] = true
		switch c.Name {
		case "IssuerContractReference":
			sl.IssuerContract = r.text(c)
		case "IssuerContractDate":
			sl.IssuerContractDate = r.date(c)
		case "IssuerTransactionReference":
			sl.IssuerTransaction = r.text(c)
		case "IssuerTransactionDate":
			sl.IssuerTransactionDate = r.date(c)
		case "ReceiverContractReference":
			sl.ReceiverContract = r.text(c)
		case "ReceiverContractDate":
			sl.ReceiverContractDate = r.date(c)
		case "ReceiverTransactionReference":
			l.OrderReference = r.text(c)
		case "ReceiverTransactionDate":
			sl.ReceiverTransactionDate = r.date(c)
		case "FileReference":
			sl.File = r.text(c)
		case "FileDate":
			sl.FileDate = r.date(c)
		case "SequenceNumber":
			l.ID = lineID(r.text(c))
		case "DeliveryNotesReferences":
			r.list(c, "DeliveryNote", func(k *xmltree.Node) {
				var d invoice.DocumentReference
				r.fields(k, func(e *xmltree.Node) bool {
					switch e.Name {
					case "DeliveryNoteNumber":
						d.ID = r.text(e)
					case "DeliveryNoteDate":
						d.IssueDate = r.date(e)
					default:
						return false
					}
					return true
				})
				if l.DeliveryNote == nil {
					l.DeliveryNote = &d
				} else {
					sl.DeliveryNotes = append(sl.DeliveryNotes, d)
				}
			})
		case "ItemDescription":
			l.Item.Name = r.text(c)
		case "Quantity":
			l.Quantity = r.decimal(c)
		case "UnitOfMeasure":
			u := r.text(c)
			if l.UnitCode = invoice.UnitCodeOfUnitOfMeasure(u); l.UnitCode == "" {
				sl.UnitOfMeasure = u
			}
		case "UnitPriceWithoutTax":
			l.NetPrice = r.decimal(c)
		case "TotalCost":
			sl.TotalCost = r.decimal(c)
		case "DiscountsAndRebates":
			r.list(c, "Discount", func(k *xmltree.Node) { l.AllowancesCharges = append(l.AllowancesCharges, r.discount(k, false)) })
		case "Charges":
			r.list(c, "Charge", func(k *xmltree.Node) { l.AllowancesCharges = append(l.AllowancesCharges, r.discount(k, true)) })
		case "GrossAmount":
			l.NetAmount = r.decimal(c)
		case "TaxesWithheld":
			r.list(c, "Tax", func(k *xmltree.Node) { sl.Withholdings = append(sl.Withholdings, r.tax(k)) })
		case "TaxesOutputs":
			r.list(c, "Tax", func(k *xmltree.Node) {
				t := r.output(k)
				if vat == nil && invoice.IsSpanishVAT(t.Type) {
					vat = &t
					return
				}
				sl.Taxes = append(sl.Taxes, t.spainTax())
			})
		case "LineItemPeriod":
			l.Period = r.period(c)
		case "TransactionDate":
			sl.TransactionDate = r.date(c)
		case "AdditionalLineItemInformation":
			l.Note = r.text(c)
		case "SpecialTaxableEvent":
			r.fields(c, func(k *xmltree.Node) bool {
				switch k.Name {
				case "SpecialTaxableEventCode":
					lt.event = r.text(k)
				case "SpecialTaxableEventReason":
					lt.reason = r.text(k)
				default:
					return false
				}
				return true
			})
		case "ArticleCode":
			l.Item.SellerID = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	if l.ID == "" {
		l.ID = strconv.Itoa(len(r.lines) + 1)
	}
	if vat != nil {
		lt.taxType = vat.Type
		l.VATRate = vat.Rate
		v := vat.vat
		if vat.TaxableBase.IsSet() && !vat.TaxableBase.Equal(l.NetAmount) {
			v.TaxableBase = vat.TaxableBase
		}
		if vat.Amount.IsSet() && !vat.Amount.Equal(lineTaxAmount(vat.TaxableBase, vat.Rate)) {
			v.TaxAmount = vat.Amount
		}
		if v != (invoice.SpainVAT{}) {
			sl.VAT = &v
		}
	}
	if sl.TotalCost.IsSet() && sl.TotalCost.Equal(totalCost(l)) {
		sl.TotalCost = invoice.Decimal{}
	}
	for i := range l.AllowancesCharges {
		if ac := &l.AllowancesCharges[i]; ac.Percentage.IsSet() {
			ac.BaseAmount = totalCost(l)
			if sl.TotalCost.IsSet() {
				ac.BaseAmount = sl.TotalCost
			}
		}
	}
	r.lines = append(r.lines, lt)
	l.Spain = sl
}

// lineID returns the line identifier (BT-126) of a SequenceNumber: the
// number as written, without a zero fractional part ("1.0" is "1").
func lineID(s string) string {
	if i := strings.IndexByte(s, '.'); i > 0 && strings.Trim(s[i+1:], "0") == "" {
		return s[:i]
	}
	return s
}

// totalCost returns the quantity of l times its unit price, with the scale
// of the unit price and at least two decimals.
func totalCost(l *invoice.Line) invoice.Decimal {
	if !l.Quantity.IsSet() || !l.NetPrice.IsSet() {
		return invoice.Decimal{}
	}
	return l.Quantity.Mul(l.NetPrice).Round(min(max(l.NetPrice.Scale(), 2), 8))
}

// lineTaxAmount returns the tax amount of a taxable base at a rate,
// rounded to two decimals.
func lineTaxAmount(base, rate invoice.Decimal) invoice.Decimal {
	if !base.IsSet() || !rate.IsSet() {
		return invoice.Decimal{}
	}
	return base.Mul(rate).Div(invoice.DecimalFromInt(100), 2)
}

// installment reads an Installment: a payment means (BG-16).
func (r *reader) installment(n *xmltree.Node) {
	inv := r.inv
	var pm invoice.PaymentMeans
	ps := new(invoice.SpainPayment)
	var due invoice.Date
	var amount invoice.Decimal
	var reconciliation string
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "InstallmentDueDate":
			due = r.date(c)
		case "InstallmentAmount":
			amount = r.decimal(c)
		case "PaymentMeans":
			ps.Means = r.text(c)
			pm.TypeCode = invoice.TypeCodeOfSpanishPaymentMeans(ps.Means)
			if pm.TypeCode == "" {
				r.issue(c, "unknown PaymentMeans "+strconv.Quote(ps.Means))
			}
		case "AccountToBeCredited":
			r.account(c, &pm.PayeeIBAN, &pm.PayeeProprietaryID, &ps.CreditBank, &ps.CreditBranch, &ps.CreditBranchAddress, &pm.PayeeBIC)
		case "PaymentReconciliationReference":
			reconciliation = r.text(c)
		case "AccountToBeDebited":
			r.account(c, &pm.PayerIBAN, &ps.DebitAccount, &ps.DebitBank, &ps.DebitBranch, &ps.DebitBranchAddress, &pm.PayerBIC)
		case "CollectionAdditionalInformation":
			ps.CollectionInfo = r.text(c)
		case "RegulatoryReportingData":
			ps.RegulatoryReporting = r.text(c)
		case "DebitReconciliationReference":
			ps.DebitReconciliation = r.text(c)
		default:
			return false
		}
		return true
	})
	if invoice.SpanishPaymentMeansOfTypeCode(pm.TypeCode) == ps.Means {
		ps.Means = ""
	}
	pm.Spain = ps
	inv.PaymentMeans = append(inv.PaymentMeans, pm)
	r.dues = append(r.dues, due)
	r.amounts = append(r.amounts, amount)
	r.reconciliations = append(r.reconciliations, reconciliation)
}

// account reads an AccountType: an IBAN or an account number, the bank and
// branch codes, the branch address and the BIC.
func (r *reader) account(n *xmltree.Node, iban, number, bank, branch *string, branchAddress **invoice.Address, bic *string) {
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "IBAN":
			*iban = r.text(c)
		case "AccountNumber":
			*number = r.text(c)
		case "BankCode":
			*bank = r.text(c)
		case "BranchCode":
			*branch = r.text(c)
		case "BranchInSpainAddress", "OverseasBranchAddress":
			a := r.address(c)
			*branchAddress = &a
		case "BIC":
			*bic = r.text(c)
		default:
			return false
		}
		return true
	})
}

// additionalData reads AdditionalData. The Extensions element is kept as
// an extension.
func (r *reader) additionalData(n *xmltree.Node) {
	inv := r.inv
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "RelatedInvoice":
			r.sp.RelatedInvoice = r.text(c)
		case "RelatedDocuments":
			r.list(c, "Attachment", r.attachment)
		case "InvoiceAdditionalInformation":
			inv.Notes = append(inv.Notes, invoice.Note{SubjectCode: "AAI", Content: r.text(c)})
		default:
			return false
		}
		return true
	})
}

// attachment reads an Attachment: a supporting document (BG-24) whose
// identifier (BT-122) is its position among the attachments.
func (r *reader) attachment(n *xmltree.Node) {
	inv := r.inv
	k := 0
	for _, d := range inv.AdditionalDocuments {
		if d.TypeCode != "50" {
			k++
		}
	}
	doc := invoice.AdditionalDocument{ID: strconv.Itoa(k + 1)}
	sa := new(invoice.SpainAttachment)
	var data *xmltree.Node
	r.fields(n, func(c *xmltree.Node) bool {
		switch c.Name {
		case "AttachmentCompressionAlgorithm":
			sa.Compression = r.text(c)
		case "AttachmentFormat":
			sa.Format = r.text(c)
		case "AttachmentEncoding":
			sa.Encoding = r.text(c)
		case "AttachmentDescription":
			doc.Description = r.text(c)
		case "AttachmentData":
			data = c
			r.text(c)
		default:
			return false
		}
		return true
	})
	doc.AttachmentMimeCode = mimeOf(sa.Format)
	if doc.AttachmentMimeCode != "" {
		sa.Format = ""
	}
	if data != nil {
		if sa.Encoding == "" || sa.Encoding == "BASE64" {
			doc.Attachment = r.binary(data)
		} else {
			doc.Attachment = []byte(data.Text)
		}
	}
	if *sa != (invoice.SpainAttachment{}) {
		doc.Spain = sa
	}
	inv.AdditionalDocuments = append(inv.AdditionalDocuments, doc)
}

// binary decodes a base64 binary object, ignoring white space.
func (r *reader) binary(n *xmltree.Node) []byte {
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

// formats gives the MIME code (BT-125-1) of each AttachmentFormat.
var formats = map[string]string{
	"pdf": "application/pdf", "jpg": "image/jpeg", "xml": "text/xml", "html": "text/html",
	"doc": "application/msword", "rtf": "application/rtf", "xls": "application/vnd.ms-excel",
	"gif": "image/gif", "bmp": "image/bmp", "tiff": "image/tiff",
}

// mimeOf returns the MIME code of an AttachmentFormat, or "".
func mimeOf(format string) string { return formats[format] }

// formatOf returns the AttachmentFormat of a MIME code, or "".
func formatOf(mime string) string {
	for f, m := range formats {
		if m == mime {
			return f
		}
	}
	return ""
}

// vatex gives the VAT category of the exemption reason codes (BT-121) that
// set one; the other codes VATEX-EU- and VATEX-ES- are exemptions (E).
var vatex = map[string]string{
	"VATEX-EU-IC": invoice.VATIntraCommunity, "VATEX-EU-G": invoice.VATExport,
	"VATEX-EU-AE": invoice.VATReverseCharge, "VATEX-EU-O": invoice.VATOutOfScope,
}

// splitReason splits a SpecialTaxableEventReason into an exemption reason
// code (BT-121), when it begins with a VATEX code, and a text (BT-120).
func splitReason(s string) (code, text string) {
	if !strings.HasPrefix(s, "VATEX-") {
		return "", s
	}
	code, text, _ = strings.Cut(s, " ")
	return code, strings.TrimSpace(text)
}

// lineCategory returns the VAT category of a line from its tax data.
func lineCategory(lt lineTax, rate invoice.Decimal) string {
	cat := invoice.CategoryOfSpanishTax(lt.taxType, rate, lt.event)
	if cat != invoice.VATExempt && cat != invoice.VATOutOfScope {
		return cat
	}
	if code, _ := splitReason(lt.reason); code != "" {
		if c, ok := vatex[code]; ok {
			return c
		}
		return invoice.VATExempt
	}
	return cat
}

// finish maps the data that needs the whole invoice: the VAT categories,
// the exemption reasons, the totals, the payment due dates, the tax
// identification of the parties, and drops the values of the national
// blocks that Marshal derives.
func (r *reader) finish(inv *invoice.Invoice) {
	sp := inv.Spain
	// VAT categories of the lines, and exemption reasons of the VAT
	// breakdown.
	for i := range inv.Lines {
		l := &inv.Lines[i]
		lt := r.lines[i]
		if lt.taxType == "" {
			continue
		}
		l.VATCategoryCode = lineCategory(lt, l.VATRate)
	}
	for i := range inv.VATBreakdown {
		v := &inv.VATBreakdown[i]
		v.CategoryCode = invoice.CategoryOfSpanishTax(r.vatTypes[i], v.Rate, "")
		if v.CategoryCode != invoice.VATZeroRated {
			continue
		}
		// A VAT at the rate zero takes the category of its lines.
		for j := range inv.Lines {
			l := &inv.Lines[j]
			if r.lines[j].taxType != r.vatTypes[i] || !l.VATRate.IsZero() {
				continue
			}
			v.CategoryCode = l.VATCategoryCode
			if reason := r.lines[j].reason; reason != "" {
				v.ExemptionReasonCode, v.ExemptionReason = splitReason(reason)
			}
			break
		}
	}
	for i := range inv.Lines {
		l, lt := &inv.Lines[i], r.lines[i]
		sl := l.Spain
		if lt.event != "" && lt.event != invoice.SpecialEventOfCategory(l.VATCategoryCode) {
			sl.SpecialEvent = lt.event
		}
		if lt.reason != "" && lt.reason != reasonOf(inv, l) {
			sl.SpecialEventReason = lt.reason
		}
		if isZeroLine(sl) {
			l.Spain = nil
		}
	}
	// The document level allowances and charges take the category of the
	// VAT breakdown when it has a single category.
	if len(inv.VATBreakdown) == 1 {
		for i := range inv.AllowancesCharges {
			ac := &inv.AllowancesCharges[i]
			ac.VATCategoryCode, ac.VATRate = inv.VATBreakdown[0].CategoryCode, inv.VATBreakdown[0].Rate
		}
	}
	for i := range inv.AllowancesCharges {
		if ac := &inv.AllowancesCharges[i]; ac.Percentage.IsSet() {
			ac.BaseAmount = inv.Totals.LineTotal
		}
	}
	for i := range inv.VATBreakdown {
		if v := &inv.VATBreakdown[i]; v.Spain != nil && *v.Spain == (invoice.SpainVAT{}) {
			v.Spain = nil
		}
	}

	// Totals.
	t := &inv.Totals
	var tax []invoice.Decimal
	for i := range inv.VATBreakdown {
		tax = append(tax, inv.VATBreakdown[i].TaxAmount)
	}
	if len(tax) > 0 {
		t.TaxTotal = invoice.Sum(tax...)
	}
	if inv.TaxCurrency == "EUR" && inv.Currency != "EUR" {
		var euros []invoice.Decimal
		for i := range inv.VATBreakdown {
			if v := inv.VATBreakdown[i].Spain; v != nil && v.TaxEuros.IsSet() {
				euros = append(euros, v.TaxEuros)
			}
		}
		if len(euros) > 0 {
			t.TaxTotalInTaxCurrency = invoice.Sum(euros...)
		}
	}
	if t.TaxBasisTotal.IsSet() {
		t.GrandTotal = t.TaxBasisTotal.Add(t.TaxTotal)
		t.DuePayable = t.GrandTotal
		if t.PaidAmount.IsSet() {
			t.DuePayable = t.DuePayable.Sub(t.PaidAmount)
		}
	}
	if t.PaidAmount.IsSet() && t.PaidAmount.IsZero() && len(sp.PaymentsOnAccount) == 0 {
		t.PaidAmount = invoice.Decimal{}
		t.DuePayable = t.GrandTotal
	}
	if r.reimbursableTotal.IsSet() && !r.reimbursableTotal.Equal(reimbursableTotal(sp)) {
		sp.ReimbursableTotal = r.reimbursableTotal
	}
	ft := facturaeTotals(inv)
	if r.invoiceTotal.IsSet() && !r.invoiceTotal.Equal(ft.invoice) {
		sp.InvoiceTotal = r.invoiceTotal
		ft = facturaeTotals(inv)
	}
	if r.outstanding.IsSet() && !r.outstanding.Equal(ft.outstanding) {
		sp.OutstandingAmount = r.outstanding
		ft = facturaeTotals(inv)
	}
	if r.executable.IsSet() && !r.executable.Equal(ft.executable) {
		sp.ExecutableAmount = r.executable
	}

	// Payment: the due date of the first installment is the payment due
	// date (BT-9), its reconciliation reference the remittance information
	// (BT-83).
	if len(r.dues) > 0 {
		if !r.dues[0].IsZero() {
			inv.PaymentTerms = []invoice.PaymentTerms{{DueDate: r.dues[0]}}
		}
		inv.RemittanceInformation = r.reconciliations[0]
	}
	for i := range inv.PaymentMeans {
		pm := &inv.PaymentMeans[i]
		ps := pm.Spain
		if !r.dues[i].IsZero() && r.dues[i].Compare(r.dues[0]) != 0 {
			ps.DueDate = r.dues[i]
		}
		if r.amounts[i].IsSet() && !r.amounts[i].Equal(t.DuePayable) {
			ps.Amount = r.amounts[i]
		}
		if r.reconciliations[i] != inv.RemittanceInformation {
			ps.Reconciliation = r.reconciliations[i]
		}
		if *ps == (invoice.SpainPayment{}) {
			pm.Spain = nil
		}
	}

	// Parties: the values that Marshal derives are not kept.
	finishParty(&inv.Seller, &inv.Buyer)
	finishParty(&inv.Buyer, &inv.Seller)
	if sp.ThirdParty != nil {
		finishParty(sp.ThirdParty, nil)
	}

	// The values of the national block that Marshal derives are not kept.
	if sp.Modality == "I" {
		sp.Modality = ""
	}
	if sp.IssuerType == issuerTypeOf(inv) {
		sp.IssuerType = ""
	}
	if sp.DocumentType == docTypeOf(inv.TypeCode) {
		sp.DocumentType = ""
	}
	if sp.Class == invoice.SpanishClassOfTypeCode(inv.TypeCode) {
		sp.Class = ""
	}
	if sp.BatchID == batchIDOf(inv) {
		sp.BatchID = ""
	}
	if co := sp.Corrective; co != nil {
		if co.ReasonCode == defaultReason {
			co.ReasonCode = ""
		}
		if co.Method == correctionMethodOf(inv) {
			co.Method = ""
		}
		if co.TaxPeriod == taxPeriodOf(inv) {
			co.TaxPeriod = invoice.Period{}
		}
		if *co == (invoice.Corrective{}) {
			sp.Corrective = nil
		}
	}
	if isZeroSpain(sp) {
		inv.Spain = nil
	}
}

// reasonOf returns the SpecialTaxableEventReason that Marshal derives for
// the line l: the exemption reason of the VAT breakdown of its category and
// rate, with its code first.
func reasonOf(inv *invoice.Invoice, l *invoice.Line) string {
	for i := range inv.VATBreakdown {
		v := &inv.VATBreakdown[i]
		if v.CategoryCode == l.VATCategoryCode && v.Rate.Equal(l.VATRate) {
			return join(v.ExemptionReasonCode, v.ExemptionReason)
		}
	}
	return ""
}

// join joins two strings with a space, dropping the empty ones.
func join(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

// finishParty drops the data of the national block of p that Marshal
// derives; other is the counterpart of p, nil for a third party.
func finishParty(p, other *invoice.Party) {
	sp := p.Spain
	if sp == nil {
		return
	}
	if sp.TaxID == derivedTaxID(p, other) {
		sp.TaxID = ""
	}
	if sp.ResidenceType == derivedResidence(p) {
		sp.ResidenceType = ""
	}
	if sp.PersonType == derivedPerson(p) {
		sp.PersonType = ""
	}
	if isZeroParty(sp) {
		p.Spain = nil
	}
}

// isZeroParty reports whether the national block of a party holds no data.
func isZeroParty(sp *invoice.SpainParty) bool {
	return sp.PersonType == "" && sp.ResidenceType == "" && sp.TaxID == "" && sp.GivenName == "" &&
		sp.FirstSurname == "" && sp.SecondSurname == "" && sp.Registration == nil && sp.Fax == "" &&
		sp.Web == "" && sp.CNAE == "" && sp.INETownCode == "" && sp.OtherContact == "" && len(sp.Centres) == 0
}

// isZeroLine reports whether the national block of a line holds no data.
func isZeroLine(sl *invoice.SpainLine) bool {
	return sl.IssuerContract == "" && sl.IssuerContractDate.IsZero() && sl.IssuerTransaction == "" &&
		sl.IssuerTransactionDate.IsZero() && sl.ReceiverContract == "" && sl.ReceiverContractDate.IsZero() &&
		sl.ReceiverTransactionDate.IsZero() && sl.File == "" && sl.FileDate.IsZero() && len(sl.DeliveryNotes) == 0 &&
		sl.UnitOfMeasure == "" && !sl.TotalCost.IsSet() && sl.VAT == nil && len(sl.Taxes) == 0 &&
		len(sl.Withholdings) == 0 && sl.SpecialEvent == "" && sl.SpecialEventReason == "" && sl.TransactionDate.IsZero()
}

// isZeroSpain reports whether the national block of the invoice holds no
// data.
func isZeroSpain(sp *invoice.Spain) bool {
	return sp.Modality == "" && sp.IssuerType == "" && sp.ThirdParty == nil && sp.BatchID == "" &&
		!sp.BatchTotalEuros.IsSet() && !sp.BatchOutstandingEuros.IsSet() && !sp.BatchExecutableEuros.IsSet() &&
		sp.SeriesCode == "" && sp.DocumentType == "" && sp.Class == "" && sp.Corrective == nil && sp.PlaceOfIssue == nil &&
		len(sp.Taxes) == 0 && len(sp.Withholdings) == 0 && len(sp.Subsidies) == 0 && len(sp.PaymentsOnAccount) == 0 &&
		len(sp.ReimbursableExpenses) == 0 && !sp.ReimbursableTotal.IsSet() && !sp.FinancialExpenses.IsSet() &&
		sp.AmountWithheld == nil && sp.PaymentInKind == nil && !sp.InvoiceTotal.IsSet() && !sp.OutstandingAmount.IsSet() &&
		!sp.ExecutableAmount.IsSet() && sp.RelatedInvoice == ""
}
