package fatturapa

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

// Namespaces of the FatturaPA format.
const (
	// Namespace is the namespace of the root element FatturaElettronica,
	// whose children are not qualified.
	Namespace = "http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"
	// NamespaceDS is the namespace of the XML signature (ds:Signature).
	NamespaceDS = "http://www.w3.org/2000/09/xmldsig#"
)

// Transmission formats (FormatoTrasmissione, attribute versione).
const (
	FormatPA      = "FPA12" // invoice to a public administration
	FormatPrivate = "FPR12" // invoice between private parties
)

// ErrNotFatturaPA is returned (wrapped) by Parse when the document is
// well-formed XML but not a FatturaPA document, for example a CII invoice.
var ErrNotFatturaPA = errors.New("fatturapa: not a FatturaPA document")

// Issue is a value that could not be read: a malformed amount, date or
// base64 attachment, a document without body. The corresponding field of
// the invoice is left empty.
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
		return "fatturapa: unreadable value at " + e.Issues[0].String()
	}
	return "fatturapa: " + strconv.Itoa(len(e.Issues)) + " unreadable values, first at " + e.Issues[0].String()
}

// IsFatturaPA reports whether root is the root element of a FatturaPA
// document: FatturaElettronica in the FatturaPA namespace.
func IsFatturaPA(root *xmltree.Node) bool { return checkRoot(root) == nil }

// Parse reads a FatturaPA document that holds a single invoice. For a
// document with several bodies (a lotto), it returns the first invoice and
// an *IssuesError that says how many there are: ParseAll reads them all.
//
// It fails, with a nil invoice, only when data is not well-formed XML or its
// root is not FatturaElettronica in the FatturaPA namespace (the error then
// wraps ErrNotFatturaPA). When some values cannot be read, Parse returns the
// invoice with the corresponding fields empty and an *IssuesError listing
// them.
func Parse(data []byte) (*invoice.Invoice, error) {
	invs, err := ParseAll(data)
	if len(invs) == 0 {
		return nil, err
	}
	if len(invs) > 1 {
		is := Issue{Message: "the document holds " + strconv.Itoa(len(invs)) + " invoices (lotto); only the first is read, ParseAll reads them all"}
		var ie *IssuesError
		if errors.As(err, &ie) {
			ie.Issues = append(ie.Issues, is)
		} else {
			err = &IssuesError{Issues: []Issue{is}}
		}
	}
	return invs[0], err
}

// ParseAll reads a FatturaPA document: one invoice per
// FatturaElettronicaBody, each with the data of the header (the transmission
// data, the seller, the buyer…).
//
// The business terms are read from the elements that appendix 4 of the
// Italian technical rules maps them to, with the conventions of the CIUS-IT
// (see invoice.Italy); the other data goes to the national blocks of the
// model. The invoices have the CIUS-IT profile (invoice.ProfileCIUSIT). Their
// type code (BT-3) is read from the document type (TipoDocumento), the VAT
// category of a line or a summary from its rate and its nature (Natura), the
// payment means type code (BT-81) from the payment method
// (ModalitaPagamento), as appendix 5 of the technical rules maps them. A
// line with a negative unit price and a quantity is read with a positive
// price and a negative quantity, as EN 16931 requires (BR-27); the net price
// (BT-146) of a line with discounts or surcharges on its unit price
// (ScontoMaggiorazione) is the unit price after them, the unit price is its
// gross price (BT-148). The totals that FatturaPA does not give are
// computed: the sum of the line net amounts (BT-106), the charges (BT-108),
// the total without VAT (BT-109) and the VAT total (BT-110); the amount due
// (BT-115) is the total with VAT (ImportoTotaleDocumento, BT-112, or the
// computed one) plus the rounding (Arrotondamento, BT-114).
//
// Nothing is dropped, except the XAdES signature: the elements and
// attributes that the format does not define are kept as extensions (see
// invoice.Extension), in the Extensions field of the value that encloses
// them (the invoice, a party, a line, a payment means…), with paths made of
// the local names of the elements ("FatturaElettronicaBody/DatiGenerali").
// Invoice.Unmodeled lists them.
func ParseAll(data []byte) ([]*invoice.Invoice, error) {
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("fatturapa: %w", err)
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

// ParseTree reads a FatturaPA document from an already parsed tree (see
// ParseAll). If root is not a FatturaPA document, it returns no invoice and
// a single issue that says why.
func ParseTree(root *xmltree.Node) ([]*invoice.Invoice, []Issue) {
	if err := checkRoot(root); err != nil {
		is := Issue{Message: err.Error()}
		if root != nil {
			is.Path, is.Line = root.Path(), root.Line
		}
		return nil, []Issue{is}
	}
	var header *xmltree.Node
	var bodies, other []*xmltree.Node
	for _, c := range root.Children {
		switch {
		case c.Space == NamespaceDS && c.Name == "Signature":
		case c.Space != "":
			other = append(other, c)
		case c.Name == "FatturaElettronicaHeader" && header == nil:
			header = c
		case c.Name == "FatturaElettronicaBody":
			bodies = append(bodies, c)
		default:
			other = append(other, c)
		}
	}
	r := new(reader)
	if header == nil {
		r.issue(root, "no FatturaElettronicaHeader")
	}
	if len(bodies) == 0 {
		r.issue(root, "no FatturaElettronicaBody")
		bodies = append(bodies, nil)
	}
	invs := make([]*invoice.Invoice, 0, len(bodies))
	for i, body := range bodies {
		inv := &invoice.Invoice{Profile: invoice.ProfileCIUSIT, Italy: new(invoice.Italy)}
		// The header is read again for each body, so that the invoices
		// share no value; its issues are reported once.
		r.quiet = i > 0
		h := r.hold(root, &inv.Extensions, "versione", "SistemaEmittente")
		inv.Italy.Format = attr(root, "versione")
		inv.Italy.IssuingSystem = attr(root, "SistemaEmittente")
		if i == 0 {
			for _, c := range other {
				r.unread(c)
			}
		}
		if header != nil {
			r.header(header, inv)
		}
		r.quiet = false
		if body != nil {
			r.body(body, inv)
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
		return ErrNotFatturaPA
	case root.Name == "FatturaElettronica" && root.Space == Namespace:
		return nil
	case root.Name == "FatturaElettronica":
		return fmt.Errorf("%w: root element <FatturaElettronica> in namespace %q", ErrNotFatturaPA, root.Space)
	}
	return fmt.Errorf("%w: root element is <%s>", ErrNotFatturaPA, root.Name)
}

// reader maps a FatturaPA tree to the model.
//
// Every element and attribute of the document is either read into a field of
// the model or kept as an extension: each container function handles the
// children it knows and passes the others to unread. Extensions go to the
// value being read whose Extensions field is ext, with a path relative to the
// element of that value (path holds the names of the elements).
type reader struct {
	issues []Issue
	quiet  bool // the header is read again: its issues are already reported

	ext  *[]invoice.Extension
	path []string

	// Data of the body that is mapped once the body is read.
	chargeability []string          // EsigibilitaIVA of each VAT summary
	amounts       []invoice.Decimal // ImportoPagamento of each payment means

	buf []byte
}

func (r *reader) issue(n *xmltree.Node, msg string) {
	if !r.quiet {
		r.issues = append(r.issues, Issue{Path: n.Path(), Line: n.Line, Message: msg})
	}
}

// holder is the state saved by hold.
type holder struct {
	ext  *[]invoice.Extension
	path []string
}

// hold makes the value whose Extensions field is ext the receiver of the
// extensions found until release, with paths relative to n, its element.
// The attributes of n itself, except a and b, are kept with an empty path.
func (r *reader) hold(n *xmltree.Node, ext *[]invoice.Extension, a, b string) holder {
	h := holder{r.ext, r.path}
	r.ext, r.path = ext, r.path[len(r.path):]
	r.keepAttrs(n, "", a, b)
	return h
}

func (r *reader) release(h holder) { r.ext, r.path = h.ext, h.path }

// enter starts reading the children of the container n: its attributes are
// kept as extensions and its name extends the path.
func (r *reader) enter(n *xmltree.Node) {
	r.keepAttrs(n, r.attrPath(n), "", "")
	r.path = append(r.path, qname(n))
}

func (r *reader) leave() { r.path = r.path[:len(r.path)-1] }

// unread keeps n, a child of the current container, as an extension.
func (r *reader) unread(n *xmltree.Node) {
	r.buf = appendElement(r.buf[:0], n)
	*r.ext = append(*r.ext, invoice.Extension{Path: strings.Join(r.path, "/"), XML: string(r.buf)})
}

// keepAttrs keeps the attributes of n, whose location is path (see
// attrPath), except a and b, which the caller reads. Attributes in another
// namespace (xsi:schemaLocation…) are not part of the data.
func (r *reader) keepAttrs(n *xmltree.Node, path, a, b string) {
	for _, at := range n.Attrs {
		if at.Prefix != "" || at.Name == a || at.Name == b {
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
	if n.Prefix != "" && n.Space != Namespace {
		return n.Prefix + ":" + n.Name
	}
	return n.Name
}

// text returns the trimmed text of the leaf n ("" for nil) and keeps its
// attributes and child elements.
func (r *reader) text(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	r.keepAttrs(n, r.attrPath(n), "", "")
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

// decimal reads an amount, a rate, a quantity or a weight.
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
	return r.parseDate(n, r.text(n))
}

func (r *reader) parseDate(n *xmltree.Node, s string) invoice.Date {
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

// yes reads an indicator whose only value is SI.
func (r *reader) yes(n *xmltree.Node) bool {
	s := r.text(n)
	if s != "SI" {
		r.issue(n, "invalid value "+strconv.Quote(s)+", want SI")
	}
	return s == "SI"
}

// binary decodes a base64 binary object, ignoring white space.
func (r *reader) binary(n *xmltree.Node) []byte {
	r.text(n)
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

// once reports whether c, a child of the current container, is the first
// child of its name; a repeated element is kept as an extension.
func (r *reader) once(seen map[string]bool, c *xmltree.Node) bool {
	if seen[c.Name] {
		r.unread(c)
		return false
	}
	seen[c.Name] = true
	return true
}

// unqualified reports whether c is an element of the format: its children
// are not qualified. Other elements are kept as extensions.
func (r *reader) unqualified(c *xmltree.Node) bool {
	if c.Space != "" {
		r.unread(c)
		return false
	}
	return true
}

// header reads FatturaElettronicaHeader.
func (r *reader) header(n *xmltree.Node, inv *invoice.Invoice) {
	it := inv.Italy
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "DatiTrasmissione":
			r.transmission(c, inv)
		case "CedentePrestatore":
			r.seller(c, inv)
		case "RappresentanteFiscale":
			inv.SellerTaxRepresentative = new(invoice.Party)
			r.registryOf(c, inv.SellerTaxRepresentative)
		case "CessionarioCommittente":
			r.buyer(c, inv)
		case "TerzoIntermediarioOSoggettoEmittente":
			it.Intermediary = new(invoice.Party)
			r.registryOf(c, it.Intermediary)
		case "SoggettoEmittente":
			it.Issuer = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// transmission reads DatiTrasmissione. The CodiceDestinatario is the
// electronic address of the buyer (BT-49): an IPA code (scheme 0201) for a
// public administration, a recipient code (0205) otherwise, or the PEC
// address (0202) when the code is 0000000 and a PECDestinatario is given.
func (r *reader) transmission(n *xmltree.Node, inv *invoice.Invoice) {
	it := inv.Italy
	var code, pec string
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "IdTrasmittente":
			it.TransmitterCountry, it.TransmitterCode = r.taxID(c)
		case "ProgressivoInvio":
			it.SequenceNumber = r.text(c)
		case "FormatoTrasmissione":
			it.Format = r.text(c)
		case "CodiceDestinatario":
			code = r.text(c)
		case "ContattiTrasmittente":
			r.enter(c)
			seen := map[string]bool{}
			for _, k := range c.Children {
				if !r.unqualified(k) || !r.once(seen, k) {
					continue
				}
				switch k.Name {
				case "Telefono":
					it.TransmitterPhone = r.text(k)
				case "Email":
					it.TransmitterEmail = r.text(k)
				default:
					r.unread(k)
				}
			}
			r.leave()
		case "PECDestinatario":
			pec = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	addr := &inv.Buyer.ElectronicAddress
	switch {
	case code == "0000000" && pec != "":
		*addr = invoice.Identifier{ID: pec, Scheme: "0202"}
		pec = ""
	case len(code) == 6:
		*addr = invoice.Identifier{ID: code, Scheme: "0201"}
	case len(code) == 7 && code != "0000000" && code != "XXXXXXX":
		*addr = invoice.Identifier{ID: code, Scheme: "0205"}
	default:
		it.RecipientCode = code
	}
	it.RecipientPEC = pec
}

// taxID reads an IdFiscaleType: IdPaese and IdCodice.
func (r *reader) taxID(n *xmltree.Node) (country, code string) {
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "IdPaese":
			country = r.text(c)
		case "IdCodice":
			code = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	return country, code
}

// seller reads CedentePrestatore.
func (r *reader) seller(n *xmltree.Node, inv *invoice.Invoice) {
	p := &inv.Seller
	pit := new(invoice.ItalyParty)
	h := r.hold(n, &p.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "DatiAnagrafici":
			r.registry(c, p, pit)
		case "Sede":
			p.Address = r.address(c)
		case "StabileOrganizzazione":
			a := r.address(c)
			pit.Establishment = &a
		case "IscrizioneREA":
			r.rea(c, p, pit)
		case "Contatti":
			r.contacts(c, p, pit)
		case "RiferimentoAmministrazione":
			inv.BuyerAccountingReference = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	if pit.TaxRegime == "RF01" {
		pit.TaxRegime = "" // written by default
	}
	setItaly(p, pit)
}

// setItaly gives p its national block when it holds data.
func setItaly(p *invoice.Party, pit *invoice.ItalyParty) {
	if *pit != (invoice.ItalyParty{}) {
		p.Italy = pit
	}
}

// registry reads the DatiAnagrafici of a party, or the
// DatiAnagraficiVettore of a carrier. pit is nil for the parties whose
// professional register and tax regime are not defined.
func (r *reader) registry(n *xmltree.Node, p *invoice.Party, pit *invoice.ItalyParty) {
	own := pit == nil
	if own {
		pit = new(invoice.ItalyParty)
	}
	var albo, number, date string
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "IdFiscaleIVA":
			country, code := r.taxID(c)
			p.VATID = country + code
		case "CodiceFiscale":
			p.LegalRegistration = invoice.Identifier{ID: r.text(c), Scheme: "0210"}
		case "Anagrafica":
			r.anagrafica(c, p, pit)
		case "AlboProfessionale":
			albo = r.text(c)
		case "ProvinciaAlbo":
			pit.RegisterProvince = r.text(c)
		case "NumeroIscrizioneAlbo":
			number = r.text(c)
		case "DataIscrizioneAlbo":
			date = r.text(c)
			r.parseDate(c, date)
		case "RegimeFiscale":
			pit.TaxRegime = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if albo != "" || number != "" || date != "" {
		p.IDs = append(p.IDs, invoice.Identifier{ID: "ALBO:" + albo + "#" + number + "#" + date})
	}
	if own {
		setItaly(p, pit)
	}
}

// anagrafica reads the name of a party: Denominazione, or Nome and Cognome
// written "Nome#Cognome:Nome#Cognome" (see invoice.ItalianName), with its
// title and its EORI code (party identifier "EORI:code").
func (r *reader) anagrafica(n *xmltree.Node, p *invoice.Party, pit *invoice.ItalyParty) {
	var given, family *xmltree.Node
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "Denominazione":
			p.Name = r.text(c)
		case "Nome":
			given = c
		case "Cognome":
			family = c
		case "Titolo":
			pit.Title = r.text(c)
		case "CodEORI":
			p.IDs = append(p.IDs, invoice.Identifier{ID: "EORI:" + r.text(c)})
		default:
			r.unread(c)
		}
	}
	switch {
	case given == nil && family == nil:
	case p.Name != "":
		// Denominazione excludes Nome and Cognome.
		for _, c := range []*xmltree.Node{given, family} {
			if c != nil {
				r.unread(c)
			}
		}
	default:
		p.Name = invoice.ItalianName(r.text(given), r.text(family))
	}
	r.leave()
}

// address reads an IndirizzoType: the street (Indirizzo) is the first
// address line, the street number (NumeroCivico) the second.
func (r *reader) address(n *xmltree.Node) invoice.Address {
	var a invoice.Address
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "Indirizzo":
			a.Line1 = r.text(c)
		case "NumeroCivico":
			a.Line2 = r.text(c)
		case "CAP":
			a.PostCode = r.text(c)
		case "Comune":
			a.City = r.text(c)
		case "Provincia":
			a.CountrySubdivision = r.text(c)
		case "Nazione":
			a.CountryCode = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	return a
}

// rea reads IscrizioneREA: the office and the number are the party
// identifier "REA:office#number".
func (r *reader) rea(n *xmltree.Node, p *invoice.Party, pit *invoice.ItalyParty) {
	var office, number string
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "Ufficio":
			office = r.text(c)
		case "NumeroREA":
			number = r.text(c)
		case "CapitaleSociale":
			pit.ShareCapital = r.decimal(c)
		case "SocioUnico":
			pit.SoleShareholder = r.text(c)
		case "StatoLiquidazione":
			if pit.Liquidation = r.text(c); pit.Liquidation == "LN" {
				pit.Liquidation = "" // written by default
			}
		default:
			r.unread(c)
		}
	}
	r.leave()
	p.IDs = append(p.IDs, invoice.Identifier{ID: "REA:" + office + "#" + number})
}

// contacts reads Contatti: the phone number (BT-42) and the e-mail address
// (BT-43) of the seller's contact, and its fax number.
func (r *reader) contacts(n *xmltree.Node, p *invoice.Party, pit *invoice.ItalyParty) {
	var ct invoice.Contact
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "Telefono":
			ct.Phone = r.text(c)
		case "Fax":
			pit.Fax = r.text(c)
		case "Email":
			ct.Email = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if ct != (invoice.Contact{}) {
		p.Contact = &ct
	}
}

// registryOf reads a party that only has DatiAnagrafici: the
// RappresentanteFiscale of the seller (BG-11) or the
// TerzoIntermediarioOSoggettoEmittente.
func (r *reader) registryOf(n *xmltree.Node, p *invoice.Party) {
	h := r.hold(n, &p.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		if c.Name == "DatiAnagrafici" {
			r.registry(c, p, nil)
			continue
		}
		r.unread(c)
	}
	r.release(h)
}

// buyer reads CessionarioCommittente.
func (r *reader) buyer(n *xmltree.Node, inv *invoice.Invoice) {
	p := &inv.Buyer
	pit := new(invoice.ItalyParty)
	h := r.hold(n, &p.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "DatiAnagrafici":
			r.registry(c, p, pit)
		case "Sede":
			p.Address = r.address(c)
		case "StabileOrganizzazione":
			a := r.address(c)
			pit.Establishment = &a
		case "RappresentanteFiscale":
			tr := new(invoice.Party)
			r.buyerRepresentative(c, tr)
			pit.TaxRepresentative = tr
		default:
			r.unread(c)
		}
	}
	r.release(h)
	setItaly(p, pit)
}

// buyerRepresentative reads the RappresentanteFiscale of the buyer.
func (r *reader) buyerRepresentative(n *xmltree.Node, p *invoice.Party) {
	var given, family string
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "IdFiscaleIVA":
			country, code := r.taxID(c)
			p.VATID = country + code
		case "Denominazione":
			p.Name = r.text(c)
		case "Nome":
			given = r.text(c)
		case "Cognome":
			family = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.leave()
	if p.Name == "" && (given != "" || family != "") {
		p.Name = invoice.ItalianName(given, family)
	}
}

// body reads a FatturaElettronicaBody.
func (r *reader) body(n *xmltree.Node, inv *invoice.Invoice) {
	r.chargeability, r.amounts = r.chargeability[:0], r.amounts[:0]
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "DatiGenerali":
			if r.once(seen, c) {
				r.general(c, inv)
			}
		case "DatiBeniServizi":
			if r.once(seen, c) {
				r.goods(c, inv)
			}
		case "DatiVeicoli":
			if r.once(seen, c) {
				r.vehicle(c, inv)
			}
		case "DatiPagamento":
			r.payment(c, inv)
		case "Allegati":
			r.attachment(c, inv)
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// general reads DatiGenerali.
func (r *reader) general(n *xmltree.Node, inv *invoice.Invoice) {
	it := inv.Italy
	var orders, contracts, receipts, linked []invoice.RelatedDocument
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "DatiGeneraliDocumento":
			if r.once(seen, c) {
				r.document(c, inv)
			}
		case "DatiOrdineAcquisto":
			orders = append(orders, r.related(c))
		case "DatiContratto":
			contracts = append(contracts, r.related(c))
		case "DatiConvenzione":
			it.Agreements = append(it.Agreements, r.related(c))
		case "DatiRicezione":
			receipts = append(receipts, r.related(c))
		case "DatiFattureCollegate":
			linked = append(linked, r.related(c))
		case "DatiSAL":
			r.enter(c)
			for _, k := range c.Children {
				if r.unqualified(k) && k.Name == "RiferimentoFase" {
					it.WorkPhases = append(it.WorkPhases, r.text(k))
					continue
				}
				r.unread(k)
			}
			r.leave()
		case "DatiDDT":
			it.TransportDocuments = append(it.TransportDocuments, r.transportDocument(c))
		case "DatiTrasporto":
			if r.once(seen, c) {
				r.transport(c, inv)
			}
		case "FatturaPrincipale":
			if r.once(seen, c) {
				r.mainInvoice(c, inv)
			}
		default:
			r.unread(c)
		}
	}
	r.leave()

	// The references that EN 16931 carries go to its fields, the others to
	// the national block.
	plain := func(d invoice.RelatedDocument) bool {
		return len(d.Lines) == 0 && d.Date.IsZero() && d.Item == ""
	}
	// The tender reference (BT-17) is the CIG of the contract, or of the
	// order when there is no contract.
	contract := len(contracts) > 0 && plain(contracts[0]) && contracts[0].OrderCode == ""
	tender := ""
	for i, d := range orders {
		if i == 0 && plain(d) && d.CUP == "" && (d.CIG == "" || !contract) {
			inv.PurchaseOrderReference, inv.BuyerReference, tender = d.ID, d.OrderCode, d.CIG
			continue
		}
		it.PurchaseOrders = append(it.PurchaseOrders, d)
	}
	for i, d := range contracts {
		if i == 0 && contract {
			inv.ContractReference, inv.ProjectReference, tender = d.ID, d.CUP, d.CIG
			continue
		}
		it.Contracts = append(it.Contracts, d)
	}
	if tender != "" {
		inv.AdditionalDocuments = append(inv.AdditionalDocuments, invoice.AdditionalDocument{ID: tender, TypeCode: "50"})
	}
	for i, d := range receipts {
		if i == 0 && plain(d) && d.OrderCode == "" && d.CUP == "" && d.CIG == "" {
			inv.ReceivingAdviceReference = d.ID
			continue
		}
		it.Receipts = append(it.Receipts, d)
	}
	for _, d := range linked {
		if len(d.Lines) == 0 && d.Item == "" && d.OrderCode == "" && d.CUP == "" && d.CIG == "" {
			inv.PrecedingInvoices = append(inv.PrecedingInvoices, invoice.DocumentReference{ID: d.ID, IssueDate: d.Date})
			continue
		}
		it.LinkedInvoices = append(it.LinkedInvoices, d)
	}
	if len(it.TransportDocuments) > 0 {
		inv.DespatchAdviceReference = it.TransportDocuments[0].Number
	}
}

// document reads DatiGeneraliDocumento.
func (r *reader) document(n *xmltree.Node, inv *invoice.Invoice) {
	it := inv.Italy
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "TipoDocumento":
			if !r.once(seen, c) {
				continue
			}
			td := r.text(c)
			inv.TypeCode = invoice.TypeCodeOfDocumentType(td)
			if inv.TypeCode == "" {
				r.issue(c, "unknown document type "+strconv.Quote(td))
			}
			if invoice.DocumentTypeOfTypeCode(inv.TypeCode) != td {
				it.DocumentType = td
			}
		case "Divisa":
			if r.once(seen, c) {
				inv.Currency = r.text(c)
			}
		case "Data":
			if r.once(seen, c) {
				inv.IssueDate = r.date(c)
			}
		case "Numero":
			if r.once(seen, c) {
				inv.Number = r.text(c)
			}
		case "DatiRitenuta":
			it.Withholdings = append(it.Withholdings, r.withholding(c))
		case "DatiBollo":
			if r.once(seen, c) {
				r.stampDuty(c, inv)
			}
		case "DatiCassaPrevidenziale":
			inv.AllowancesCharges = append(inv.AllowancesCharges, r.fund(c))
		case "ScontoMaggiorazione":
			it.Discounts = append(it.Discounts, r.discount(c))
		case "ImportoTotaleDocumento":
			if r.once(seen, c) {
				inv.Totals.GrandTotal = r.decimal(c)
			}
		case "Arrotondamento":
			if r.once(seen, c) {
				inv.Totals.RoundingAmount = r.decimal(c)
			}
		case "Causale":
			inv.Notes = append(inv.Notes, invoice.Note{Content: r.text(c)})
		case "Art73":
			if r.once(seen, c) {
				it.Art73 = r.yes(c)
			}
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// fields reads the children of n whose names are keys of dst, once each,
// and keeps the others as extensions.
func (r *reader) fields(n *xmltree.Node, dst map[string]*string) {
	r.enter(n)
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		p, ok := dst[c.Name]
		if !ok || *p != "" {
			r.unread(c)
			continue
		}
		if *p = r.text(c); *p == "" {
			*p = " " // read, but empty: a second occurrence is an extension
		}
	}
	r.leave()
	for _, p := range dst {
		if *p == " " {
			*p = ""
		}
	}
}

// withholding reads DatiRitenuta.
func (r *reader) withholding(n *xmltree.Node) invoice.Withholding {
	var w invoice.Withholding
	var amount, rate string
	r.fields(n, map[string]*string{
		"TipoRitenuta": &w.Type, "ImportoRitenuta": &amount, "AliquotaRitenuta": &rate, "CausalePagamento": &w.Reason,
	})
	w.Amount, w.Rate = r.parse(n, "ImportoRitenuta", amount), r.parse(n, "AliquotaRitenuta", rate)
	return w
}

// parse parses the decimal s read from the child name of n.
func (r *reader) parse(n *xmltree.Node, name, s string) invoice.Decimal {
	if s == "" {
		return invoice.Decimal{}
	}
	d, err := invoice.ParseDecimal(s)
	if err != nil {
		c := n.Elem(name)
		if c == nil {
			c = n
		}
		r.issue(c, err.Error())
		return invoice.Decimal{}
	}
	return d
}

// stampDuty reads DatiBollo.
func (r *reader) stampDuty(n *xmltree.Node, inv *invoice.Invoice) {
	var virtual, amount string
	r.fields(n, map[string]*string{"BolloVirtuale": &virtual, "ImportoBollo": &amount})
	if virtual != "SI" {
		r.issue(n, "invalid BolloVirtuale "+strconv.Quote(virtual)+", want SI")
	}
	inv.Italy.StampDuty = &invoice.StampDuty{Amount: r.parse(n, "ImportoBollo", amount)}
}

// fund reads DatiCassaPrevidenziale as a document level charge, whose
// reason (BT-104) is the fund type followed by the withholding indicator
// and the nature, as appendix 4 writes it ("TC22", "TC04 SI N4").
func (r *reader) fund(n *xmltree.Node) invoice.AllowanceCharge {
	ac := invoice.AllowanceCharge{Charge: true}
	var fund, rate, amount, base, vat, withholding, nature, ref string
	h := r.hold(n, &ac.Extensions, "", "")
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		dst := map[string]*string{
			"TipoCassa": &fund, "AlCassa": &rate, "ImportoContributoCassa": &amount, "ImponibileCassa": &base,
			"AliquotaIVA": &vat, "Ritenuta": &withholding, "Natura": &nature, "RiferimentoAmministrazione": &ref,
		}[c.Name]
		if dst == nil || *dst != "" {
			r.unread(c)
			continue
		}
		*dst = r.text(c)
	}
	r.release(h)
	if withholding != "" && withholding != "SI" {
		r.issue(n, "invalid Ritenuta "+strconv.Quote(withholding)+", want SI")
		withholding = ""
	}
	ac.Percentage = r.parse(n, "AlCassa", rate)
	ac.Amount = r.parse(n, "ImportoContributoCassa", amount)
	ac.BaseAmount = r.parse(n, "ImponibileCassa", base)
	ac.VATRate = r.parse(n, "AliquotaIVA", vat)
	ac.VATCategoryCode = category(ac.VATRate, nature)
	ac.Reason = strings.Join(nonEmpty(fund, withholding, nature), " ")
	if ref != "" {
		ac.Italy = &invoice.ItalyCharge{AdministrationRef: ref}
	}
	return ac
}

// nonEmpty returns the values that are not empty.
func nonEmpty(values ...string) []string {
	out := values[:0]
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// category returns the VAT category of a rate and a nature: S for a rate
// that is not zero, the category of the nature otherwise.
func category(rate invoice.Decimal, nature string) string {
	if rate.IsSet() && !rate.IsZero() {
		return "S"
	}
	c, _ := invoice.CategoryOfNature(nature)
	return c
}

// discount reads a ScontoMaggiorazione.
func (r *reader) discount(n *xmltree.Node) invoice.Discount {
	var d invoice.Discount
	var pct, amount string
	r.fields(n, map[string]*string{"Tipo": &d.Type, "Percentuale": &pct, "Importo": &amount})
	d.Percentage, d.Amount = r.parse(n, "Percentuale", pct), r.parse(n, "Importo", amount)
	return d
}

// related reads a DatiDocumentiCorrelatiType.
func (r *reader) related(n *xmltree.Node) invoice.RelatedDocument {
	var d invoice.RelatedDocument
	var date string
	r.enter(n)
	dst := map[string]*string{
		"IdDocumento": &d.ID, "Data": &date, "NumItem": &d.Item, "CodiceCommessaConvenzione": &d.OrderCode,
		"CodiceCUP": &d.CUP, "CodiceCIG": &d.CIG,
	}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		if c.Name == "RiferimentoNumeroLinea" {
			d.Lines = append(d.Lines, r.text(c))
			continue
		}
		p, ok := dst[c.Name]
		if !ok || *p != "" {
			r.unread(c)
			continue
		}
		*p = r.text(c)
		if c.Name == "Data" {
			d.Date = r.parseDate(c, date)
		}
	}
	r.leave()
	return d
}

// transportDocument reads DatiDDT.
func (r *reader) transportDocument(n *xmltree.Node) invoice.TransportDocument {
	var d invoice.TransportDocument
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "NumeroDDT":
			if r.once(seen, c) {
				d.Number = r.text(c)
			}
		case "DataDDT":
			if r.once(seen, c) {
				d.Date = r.date(c)
			}
		case "RiferimentoNumeroLinea":
			d.Lines = append(d.Lines, r.text(c))
		default:
			r.unread(c)
		}
	}
	r.leave()
	return d
}

// transport reads DatiTrasporto: the delivery address is the deliver-to
// address (BG-15), the day of the delivery its actual delivery date
// (BT-72), the rest goes to the national block.
func (r *reader) transport(n *xmltree.Node, inv *invoice.Invoice) {
	var t invoice.Transport
	r.enter(n)
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "DatiAnagraficiVettore":
			t.Carrier = new(invoice.Party)
			r.carrier(c, &t)
		case "MezzoTrasporto":
			t.Means = r.text(c)
		case "CausaleTrasporto":
			t.Reason = r.text(c)
		case "NumeroColli":
			t.Packages = r.text(c)
		case "Descrizione":
			t.Description = r.text(c)
		case "UnitaMisuraPeso":
			t.WeightUnit = r.text(c)
		case "PesoLordo":
			t.GrossWeight = r.decimal(c)
		case "PesoNetto":
			t.NetWeight = r.decimal(c)
		case "DataOraRitiro":
			t.PickupTime = r.text(c)
		case "DataInizioTrasporto":
			t.StartDate = r.date(c)
		case "TipoResa":
			t.DeliveryTerms = r.text(c)
		case "IndirizzoResa":
			if inv.ShipTo == nil {
				inv.ShipTo = new(invoice.Party)
			}
			inv.ShipTo.Address = r.address(c)
		case "DataOraConsegna":
			s := r.text(c)
			day, rest := s, ""
			if len(s) > 10 {
				day, rest = s[:10], s[10:]
			}
			inv.DeliveryDate = r.parseDate(c, day)
			if rest != "" && rest != "T00:00:00" {
				t.DeliveryTime = s
			}
		default:
			r.unread(c)
		}
	}
	r.leave()
	if t != (invoice.Transport{}) {
		inv.Italy.Transport = &t
	}
}

// carrier reads DatiAnagraficiVettore.
func (r *reader) carrier(n *xmltree.Node, t *invoice.Transport) {
	p := t.Carrier
	pit := new(invoice.ItalyParty)
	h := r.hold(n, &p.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "IdFiscaleIVA":
			country, code := r.taxID(c)
			p.VATID = country + code
		case "CodiceFiscale":
			p.LegalRegistration = invoice.Identifier{ID: r.text(c), Scheme: "0210"}
		case "Anagrafica":
			r.anagrafica(c, p, pit)
		case "NumeroLicenzaGuida":
			t.CarrierLicence = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	setItaly(p, pit)
}

// mainInvoice reads FatturaPrincipale.
func (r *reader) mainInvoice(n *xmltree.Node, inv *invoice.Invoice) {
	var ref invoice.DocumentReference
	var date string
	r.fields(n, map[string]*string{"NumeroFatturaPrincipale": &ref.ID, "DataFatturaPrincipale": &date})
	if date != "" {
		c := n.Elem("DataFatturaPrincipale")
		ref.IssueDate = r.parseDate(c, date)
	}
	inv.Italy.MainInvoice = &ref
}

// vehicle reads DatiVeicoli.
func (r *reader) vehicle(n *xmltree.Node, inv *invoice.Invoice) {
	var v invoice.Vehicle
	var date string
	r.fields(n, map[string]*string{"Data": &date, "TotalePercorso": &v.Distance})
	if date != "" {
		v.Date = r.parseDate(n.Elem("Data"), date)
	}
	inv.Italy.Vehicle = &v
}

// goods reads DatiBeniServizi.
func (r *reader) goods(n *xmltree.Node, inv *invoice.Invoice) {
	r.enter(n)
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "DettaglioLinee":
			inv.Lines = append(inv.Lines, invoice.Line{})
			r.line(c, &inv.Lines[len(inv.Lines)-1])
		case "DatiRiepilogo":
			inv.VATBreakdown = append(inv.VATBreakdown, invoice.VATBreakdown{})
			r.summary(c, &inv.VATBreakdown[len(inv.VATBreakdown)-1])
		default:
			r.unread(c)
		}
	}
	r.leave()
}

// Names of the item attributes (BT-160) that carry the kind and the nature
// of a line, as appendix 4 of the technical rules writes them.
const (
	attrKind   = "TipoCessionePrestazione"
	attrNature = "NATURA"
)

// line reads a DettaglioLinee.
func (r *reader) line(n *xmltree.Node, l *invoice.Line) {
	li := new(invoice.ItalyLine)
	var price invoice.Decimal
	var nature string
	h := r.hold(n, &l.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "CodiceArticolo":
			r.itemCode(c, l, li)
			continue
		case "ScontoMaggiorazione":
			li.Discounts = append(li.Discounts, r.discount(c))
			continue
		case "AltriDatiGestionali":
			li.OtherData = append(li.OtherData, r.otherData(c))
			continue
		}
		if !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "NumeroLinea":
			l.ID = r.text(c)
		case "TipoCessionePrestazione":
			l.Item.Attributes = append(l.Item.Attributes, invoice.ItemAttribute{Name: attrKind, Value: r.text(c)})
		case "Descrizione":
			l.Item.Name = r.text(c)
		case "Quantita":
			l.Quantity = r.decimal(c)
		case "UnitaMisura":
			l.UnitCode = r.text(c)
		case "DataInizioPeriodo":
			l.Period.Start = r.date(c)
		case "DataFinePeriodo":
			l.Period.End = r.date(c)
		case "PrezzoUnitario":
			price = r.decimal(c)
		case "PrezzoTotale":
			l.NetAmount = r.decimal(c)
		case "AliquotaIVA":
			l.VATRate = r.decimal(c)
		case "Ritenuta":
			li.Withholding = r.yes(c)
		case "Natura":
			nature = r.text(c)
			l.Item.Attributes = append(l.Item.Attributes, invoice.ItemAttribute{Name: attrNature, Value: nature})
		case "RiferimentoAmministrazione":
			l.AccountingReference = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	l.VATCategoryCode = category(l.VATRate, nature)
	r.price(l, li, price)
	if li.Withholding || len(li.Codes) > 0 || len(li.Discounts) > 0 || len(li.OtherData) > 0 {
		l.Italy = li
	}
}

// price sets the prices of a line from its unit price and the discounts and
// surcharges on it, which apply one after the other. A negative unit price
// with a quantity is read as a positive price and a negative quantity.
func (r *reader) price(l *invoice.Line, li *invoice.ItalyLine, price invoice.Decimal) {
	if !price.IsSet() {
		return
	}
	if price.Sign() < 0 && l.Quantity.IsSet() && !l.Quantity.IsZero() {
		price, l.Quantity = price.Neg(), l.Quantity.Neg()
		for i := range li.Discounts {
			if d := &li.Discounts[i]; d.Amount.IsSet() {
				d.Amount = d.Amount.Neg()
			}
		}
		defer func() {
			for i := range li.Discounts {
				if d := &li.Discounts[i]; d.Amount.IsSet() {
					d.Amount = d.Amount.Neg()
				}
			}
		}()
	}
	if len(li.Discounts) == 0 {
		l.NetPrice = price
		return
	}
	net := price
	for _, d := range li.Discounts {
		amount := d.Amount
		if d.Percentage.IsSet() {
			amount = net.Mul(d.Percentage).Div(invoice.DecimalFromInt(100), 8)
		}
		switch d.Type {
		case "SC":
			net = net.Sub(amount)
		case "MG":
			net = net.Add(amount)
		}
	}
	net = trim(net.Round(8))
	if net.Cmp(price) > 0 {
		l.NetPrice = net
		return
	}
	l.GrossPrice, l.NetPrice = price, net
	if !net.Equal(price) {
		l.PriceDiscount = trim(price.Sub(net))
	}
	// A single discount by an amount is the price discount (BT-147).
	if d := li.Discounts; len(d) == 1 && d[0].Type == "SC" && !d[0].Percentage.IsSet() && l.PriceDiscount.Equal(d[0].Amount) &&
		l.PriceDiscount.IsSet() {
		li.Discounts = nil
	}
}

// trim removes the trailing zeros of the decimals of d, keeping two.
func trim(d invoice.Decimal) invoice.Decimal {
	c, s := d.Coef(), d.Scale()
	for s > 2 && c%10 == 0 {
		c /= 10
		s--
	}
	return invoice.NewDecimal(c, s)
}

// itemCode reads a CodiceArticolo, as appendix 4 of the technical rules
// maps it: a code CARB or AICFARMACO is the seller's identifier of the item
// (BT-155) with its type as prefix ("CARB:27101249"), a code of type
// "Identificativo del prodotto" the seller's identifier itself, a code whose
// type is an ISO 6523 scheme (four digits, 0160 for a GTIN) the standard
// identifier (BT-157). The other codes go to the national block.
func (r *reader) itemCode(n *xmltree.Node, l *invoice.Line, li *invoice.ItalyLine) {
	var code invoice.ItemCode
	r.fields(n, map[string]*string{"CodiceTipo": &code.Type, "CodiceValore": &code.Value})
	switch {
	case l.Item.SellerID == "" && (code.Type == "CARB" || code.Type == "AICFARMACO"):
		l.Item.SellerID = code.Type + ":" + code.Value
	case l.Item.SellerID == "" && code.Type == productID:
		l.Item.SellerID = code.Value
	case l.Item.GlobalID.IsZero() && isScheme(code.Type):
		l.Item.GlobalID = invoice.Identifier{ID: code.Value, Scheme: code.Type}
	default:
		li.Codes = append(li.Codes, code)
	}
}

// productID is the article code type of the seller's identifier of an item
// (BT-155) without prefix.
const productID = "Identificativo del prodotto"

// isScheme reports whether s is an ISO 6523 scheme identifier: four digits.
func isScheme(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := range 4 {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// otherData reads AltriDatiGestionali.
func (r *reader) otherData(n *xmltree.Node) invoice.OtherData {
	var d invoice.OtherData
	var number, date string
	r.fields(n, map[string]*string{"TipoDato": &d.Type, "RiferimentoTesto": &d.Text, "RiferimentoNumero": &number, "RiferimentoData": &date})
	d.Number = r.parse(n, "RiferimentoNumero", number)
	if date != "" {
		d.Date = r.parseDate(n.Elem("RiferimentoData"), date)
	}
	return d
}

// summary reads a DatiRiepilogo. The nature and the legal reference of a
// summary without VAT are its exemption reason (BT-120), the code of the
// nature its exemption reason code (BT-121), except for the nature N1
// (category Z), which has neither.
func (r *reader) summary(n *xmltree.Node, v *invoice.VATBreakdown) {
	vi := new(invoice.ItalyVAT)
	var nature, chargeability, ref string
	h := r.hold(n, &v.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "AliquotaIVA":
			v.Rate = r.decimal(c)
		case "Natura":
			nature = r.text(c)
		case "SpeseAccessorie":
			vi.AncillaryCharges = r.decimal(c)
		case "Arrotondamento":
			vi.Rounding = r.decimal(c)
		case "ImponibileImporto":
			v.TaxableAmount = r.decimal(c)
		case "Imposta":
			v.TaxAmount = r.decimal(c)
		case "EsigibilitaIVA":
			chargeability = r.text(c)
		case "RiferimentoNormativo":
			ref = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	v.CategoryCode = category(v.Rate, nature)
	if v.CategoryCode == "S" && chargeability == "S" {
		v.CategoryCode, chargeability = "B", ""
	}
	switch v.CategoryCode {
	case "S", "B", "Z":
		// EN 16931 forbids an exemption reason (BR-S-10, BR-Z-10); the
		// nature N1 is the category Z.
		vi.LegalReference = ref
	default:
		_, v.ExemptionReasonCode = invoice.CategoryOfNature(nature)
		v.ExemptionReason = strings.Join(nonEmpty(nature, ref), " ")
	}
	r.chargeability = append(r.chargeability, chargeability)
	if *vi != (invoice.ItalyVAT{}) {
		v.Italy = vi
	}
}

// payment reads a DatiPagamento: each DettaglioPagamento is a payment means.
// The payment conditions of the first one, followed by its number of days
// and, for an invoice in split payment, by the mention that BR-IT-DC-510
// requires (see splitPaymentTerms), are the payment terms (BT-20); its due date is the payment due date
// (BT-9), its payment code the remittance information (BT-83) and its
// beneficiary the payee name (BT-59).
func (r *reader) payment(n *xmltree.Node, inv *invoice.Invoice) {
	var conditions string
	first := len(inv.PaymentMeans) == 0
	r.enter(n)
	for _, c := range n.Children {
		if !r.unqualified(c) {
			continue
		}
		switch c.Name {
		case "CondizioniPagamento":
			if conditions != "" {
				r.unread(c)
				continue
			}
			conditions = r.text(c)
		case "DettaglioPagamento":
			inv.PaymentMeans = append(inv.PaymentMeans, invoice.PaymentMeans{})
			r.detail(c, inv, &inv.PaymentMeans[len(inv.PaymentMeans)-1])
		default:
			r.unread(c)
		}
	}
	r.leave()
	means := inv.PaymentMeans[len(inv.PaymentMeans)-countDetails(n):]
	if first && len(inv.PaymentTerms) == 0 {
		t := invoice.PaymentTerms{Description: conditions}
		if len(means) > 0 {
			pit := means[0].Italy
			t.DueDate, pit.DueDate = pit.DueDate, invoice.Date{}
			if pit.TermsDays != "" {
				t.Description += " " + pit.TermsDays
				pit.TermsDays = ""
			}
		}
		inv.PaymentTerms = append(inv.PaymentTerms, t)
	}
	for i := range means {
		pit := means[i].Italy
		if !first || conditions != firstWord(inv.PaymentTerms[0].Description) {
			pit.Conditions = conditions
		}
		if !pit.DueDate.IsZero() && pit.DueDate == inv.PaymentTerms[0].DueDate {
			pit.DueDate = invoice.Date{}
		}
	}
}

// countDetails returns the number of DettaglioPagamento of n.
func countDetails(n *xmltree.Node) int {
	k := 0
	for _, c := range n.Children {
		if c.Space == "" && c.Name == "DettaglioPagamento" {
			k++
		}
	}
	return k
}

// firstWord returns s up to its first space.
func firstWord(s string) string {
	w, _, _ := strings.Cut(s, " ")
	return w
}

// detail reads a DettaglioPagamento into pm. Its national block is kept even
// when empty until the totals are known (see finish).
func (r *reader) detail(n *xmltree.Node, inv *invoice.Invoice, pm *invoice.PaymentMeans) {
	pit := new(invoice.ItalyPayment)
	pm.Italy = pit
	var amount invoice.Decimal
	h := r.hold(n, &pm.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "Beneficiario":
			pit.Beneficiary = r.text(c)
		case "ModalitaPagamento":
			m := r.text(c)
			pm.TypeCode = invoice.TypeCodeOfPaymentMethod(m)
			if pm.TypeCode == "" {
				r.issue(c, "unknown payment method "+strconv.Quote(m))
			}
			if pm.TypeCode == "" || invoice.PaymentMethodOfTypeCode(pm.TypeCode) != m {
				pit.Method = m
			}
		case "DataRiferimentoTerminiPagamento":
			pit.TermsReferenceDate = r.date(c)
		case "GiorniTerminiPagamento":
			pit.TermsDays = r.text(c)
		case "DataScadenzaPagamento":
			pit.DueDate = r.date(c)
		case "ImportoPagamento":
			amount = r.decimal(c)
		case "CodUfficioPostale":
			pit.PostOffice = r.text(c)
		case "CognomeQuietanzante":
			pit.ReceiverSurname = r.text(c)
		case "NomeQuietanzante":
			pit.ReceiverName = r.text(c)
		case "CFQuietanzante":
			pit.ReceiverCF = r.text(c)
		case "TitoloQuietanzante":
			pit.ReceiverTitle = r.text(c)
		case "IstitutoFinanziario":
			pit.Institution = r.text(c)
		case "IBAN":
			pm.PayeeIBAN = r.text(c)
		case "ABI":
			pit.ABI = r.text(c)
		case "CAB":
			pit.CAB = r.text(c)
		case "BIC":
			pm.PayeeBIC = r.text(c)
		case "ScontoPagamentoAnticipato":
			pit.EarlyDiscount = r.decimal(c)
		case "DataLimitePagamentoAnticipato":
			pit.EarlyDiscountDate = r.date(c)
		case "PenalitaPagamentiRitardati":
			pit.LatePenalty = r.decimal(c)
		case "DataDecorrenzaPenale":
			pit.LatePenaltyDate = r.date(c)
		case "CodicePagamento":
			pit.PaymentCode = r.text(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
	r.amounts = append(r.amounts, amount)
	if pit.Beneficiary != "" {
		if inv.Payee == nil {
			inv.Payee = &invoice.Party{Name: pit.Beneficiary}
		}
		if inv.Payee.Name == pit.Beneficiary {
			pit.Beneficiary = ""
		}
	}
	if pit.PaymentCode != "" {
		if inv.RemittanceInformation == "" {
			inv.RemittanceInformation = pit.PaymentCode
		}
		if inv.RemittanceInformation == pit.PaymentCode {
			pit.PaymentCode = ""
		}
	}
}

// attachment reads Allegati as an additional supporting document (BG-24):
// its name is the document reference (BT-122) and the file name (BT-125-2),
// its format gives the MIME code (BT-125-1).
func (r *reader) attachment(n *xmltree.Node, inv *invoice.Invoice) {
	inv.AdditionalDocuments = append(inv.AdditionalDocuments, invoice.AdditionalDocument{})
	d := &inv.AdditionalDocuments[len(inv.AdditionalDocuments)-1]
	h := r.hold(n, &d.Extensions, "", "")
	seen := map[string]bool{}
	for _, c := range n.Children {
		if !r.unqualified(c) || !r.once(seen, c) {
			continue
		}
		switch c.Name {
		case "NomeAttachment":
			d.ID = r.text(c)
			d.AttachmentFilename = d.ID
		case "FormatoAttachment":
			if m := mimeOf(textOf(c)); m != "" && len(c.Attrs) == 0 && len(c.Children) == 0 {
				d.AttachmentMimeCode = m
				continue
			}
			r.unread(c)
		case "DescrizioneAttachment":
			d.Description = r.text(c)
		case "Attachment":
			d.Attachment = r.binary(c)
		default:
			r.unread(c)
		}
	}
	r.release(h)
}

// textOf returns the trimmed text of n without keeping anything.
func textOf(n *xmltree.Node) string { return strings.TrimSpace(n.Text) }

// formats are the attachment formats (FormatoAttachment) whose MIME code
// (BT-125-1) EN 16931 allows.
var formats = []struct{ format, mime string }{
	{"PDF", "application/pdf"},
	{"PNG", "image/png"},
	{"JPEG", "image/jpeg"},
	{"JPG", "image/jpeg"},
	{"CSV", "text/csv"},
	{"XLSX", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	{"ODS", "application/vnd.oasis.opendocument.spreadsheet"},
}

// mimeOf returns the MIME code of an attachment format, or "".
func mimeOf(format string) string {
	for _, f := range formats {
		if strings.EqualFold(f.format, format) {
			return f.mime
		}
	}
	return ""
}

// formatOf returns the attachment format of a MIME code, or "".
func formatOf(mime string) string {
	for _, f := range formats {
		if strings.EqualFold(f.mime, mime) {
			return f.format
		}
	}
	return ""
}

// finish maps the data of the invoice that depends on several elements:
// the VAT chargeability, the category of the lines in split payment, the
// totals, the payment amounts.
func (r *reader) finish(inv *invoice.Invoice) {
	// Deferred VAT (D) on every summary with VAT is the VAT point date code
	// 72 (on payment); otherwise each summary keeps its chargeability.
	deferred := false
	for i, c := range r.chargeability {
		if inv.VATBreakdown[i].CategoryCode == "B" {
			continue
		}
		if c != "D" {
			deferred = false
			break
		}
		deferred = true
	}
	if deferred && inv.TaxPointDateCode == "" {
		inv.TaxPointDateCode = "72"
	}
	split := map[string]bool{}
	for i, c := range r.chargeability {
		v := &inv.VATBreakdown[i]
		if v.CategoryCode == "B" {
			split[v.Rate.String()] = true
			continue
		}
		if c == "" || c == "D" && inv.TaxPointDateCode == "72" || c == "I" && inv.TaxPointDateCode != "72" {
			continue
		}
		if v.Italy == nil {
			v.Italy = new(invoice.ItalyVAT)
		}
		v.Italy.Chargeability = c
	}
	for i := range inv.Lines {
		if l := &inv.Lines[i]; l.VATCategoryCode == "S" && split[l.VATRate.String()] {
			l.VATCategoryCode = "B"
		}
	}
	for i := range inv.AllowancesCharges {
		if ac := &inv.AllowancesCharges[i]; ac.VATCategoryCode == "S" && split[ac.VATRate.String()] {
			ac.VATCategoryCode = "B"
		}
	}

	if len(split) > 0 && len(inv.PaymentTerms) > 0 {
		t := &inv.PaymentTerms[0]
		t.Description = join(t.Description, splitPaymentTerms)
	}

	t := &inv.Totals
	var lines, charges, tax []invoice.Decimal
	for i := range inv.Lines {
		lines = append(lines, inv.Lines[i].NetAmount)
	}
	for i := range inv.AllowancesCharges {
		charges = append(charges, inv.AllowancesCharges[i].Amount)
	}
	for i := range inv.VATBreakdown {
		tax = append(tax, inv.VATBreakdown[i].TaxAmount)
	}
	if len(lines) > 0 {
		t.LineTotal = invoice.Sum(lines...).Round(2)
		t.TaxBasisTotal = t.LineTotal
		if len(charges) > 0 {
			t.ChargeTotal = invoice.Sum(charges...)
			t.TaxBasisTotal = t.TaxBasisTotal.Add(t.ChargeTotal)
		}
	}
	if len(tax) > 0 {
		t.TaxTotal = invoice.Sum(tax...)
	}
	if !t.GrandTotal.IsSet() && t.TaxBasisTotal.IsSet() {
		t.GrandTotal = t.TaxBasisTotal.Add(t.TaxTotal)
	}
	if t.GrandTotal.IsSet() {
		t.DuePayable = t.GrandTotal
		if t.RoundingAmount.IsSet() {
			t.DuePayable = t.DuePayable.Add(t.RoundingAmount)
		}
	}

	for i := range inv.PaymentMeans {
		pm := &inv.PaymentMeans[i]
		if pm.Italy == nil {
			continue
		}
		if i < len(r.amounts) && r.amounts[i].IsSet() && !r.amounts[i].Equal(t.DuePayable) {
			pm.Italy.Amount = r.amounts[i]
		}
		if *pm.Italy == (invoice.ItalyPayment{}) {
			pm.Italy = nil
		}
	}
	// The values that Marshal derives from the EN 16931 fields are not
	// kept.
	it := inv.Italy
	if country, code := transmitterOf(inv); it.TransmitterCountry == country && it.TransmitterCode == code {
		it.TransmitterCountry, it.TransmitterCode = "", ""
	}
	if it.SequenceNumber == sequenceOf(inv.Number) {
		it.SequenceNumber = ""
	}
	if it.Format == formatFor(inv) {
		it.Format = ""
	}
	if isZeroItaly(it) {
		inv.Italy = nil
	}
}

// isZeroItaly reports whether the national block holds no data.
func isZeroItaly(it *invoice.Italy) bool {
	return it.TransmitterCountry == "" && it.TransmitterCode == "" && it.SequenceNumber == "" && it.Format == "" &&
		it.RecipientCode == "" && it.TransmitterPhone == "" && it.TransmitterEmail == "" && it.RecipientPEC == "" &&
		it.IssuingSystem == "" && it.Intermediary == nil && it.Issuer == "" && it.DocumentType == "" &&
		len(it.Withholdings) == 0 && it.StampDuty == nil && len(it.Discounts) == 0 && !it.Art73 &&
		len(it.PurchaseOrders) == 0 && len(it.Contracts) == 0 && len(it.Agreements) == 0 && len(it.Receipts) == 0 &&
		len(it.LinkedInvoices) == 0 && len(it.WorkPhases) == 0 && len(it.TransportDocuments) == 0 &&
		it.Transport == nil && it.MainInvoice == nil && it.Vehicle == nil
}
