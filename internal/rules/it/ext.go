package it

import (
	"slices"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// extURN begins the URI (ext:ExtensionURI) of each extension of EXT-IT, as
// the AgID documentation of the domestic invoice gives them; the name of
// the extension follows it, then possibly "::" and an index.
const extURN = "urn:fdc:agid.gov.it:fatturapa:"

// Names of the extensions of EXT-IT, after extURN.
const (
	extIssuer       = "SoggettoEmittente"           // 1.6 SoggettoEmittente
	extDocumentType = "TipoDocumento"               // 2.1.1.1 TipoDocumento
	extWithholding  = "TipoRitenuta"                // 2.1.1.5.1 TipoRitenuta
	extArt73        = "Art73"                       // 2.1.1.12 Art73
	extRounding     = "RiepilogoIVA:Arrotondamento" // 2.2.2.4 Arrotondamento
	extDiscounts    = "ScontoMaggiorazione"         // 2.1.1.8 ScontoMaggiorazione
)

// Values of the FatturaPA codes the extensions carry (BR-IT-DE-001 to 003,
// BR-IT-DE-017), from the FatturaPA schema.
var (
	issuers          = schema.FatturaPAEnumeration("SoggettoEmittenteType")
	documentTypes    = schema.FatturaPAEnumeration("TipoDocumentoType")
	withholdingTypes = schema.FatturaPAEnumeration("TipoRitenutaType")
	taxRegimes       = schema.FatturaPAEnumeration("RegimeFiscaleType")
)

// extension is an extension of EXT-IT in a UBL document.
type extension struct {
	node    *xmltree.Node // ext:UBLExtension
	content *xmltree.Node // ext:ExtensionContent, nil when missing
	name    string        // name after extURN, without the index
	index   int           // index after "::", 0 when there is none, -1 when it is not a number
}

// extensions returns the extensions of EXT-IT of the UBL document root.
func extensions(root *xmltree.Node) []extension {
	var out []extension
	for _, n := range root.FindAll("ext:UBLExtensions/ext:UBLExtension") {
		uri, ok := strings.CutPrefix(n.Value("ext:ExtensionURI"), extURN)
		if !ok {
			continue
		}
		e := extension{node: n, content: n.Elem("ext:ExtensionContent"), name: uri}
		if name, index, ok := strings.Cut(uri, "::"); ok {
			e.name = name
			if i, err := strconv.Atoi(index); err == nil && i > 0 {
				e.index = i
			} else {
				e.index = -1
			}
		}
		out = append(out, e)
	}
	return out
}

// ublLines returns the invoice or credit note lines of the UBL document
// root.
func ublLines(root *xmltree.Node) []*xmltree.Node {
	return append(root.Elems("cac:InvoiceLine"), root.Elems("cac:CreditNoteLine")...)
}

// italianExtensions runs the rules of appendix 3 (BR-IT-DE-*) on the UBL
// document of a seller in Italy.
func (k *checker) italianExtensions() {
	u := k.d.UBL
	withholdings := map[int]bool{} // indexes of the TipoRitenuta extensions
	var withholding *xmltree.Node  // the first one
	for _, e := range extensions(u) {
		switch e.name {
		case extIssuer:
			k.extCode("BR-IT-DE-001", e, issuers)
		case extDocumentType:
			k.extCode("BR-IT-DE-002", e, documentTypes)
		case extWithholding:
			k.extCode("BR-IT-DE-003", e, withholdingTypes)
			withholdings[e.index] = true
			if withholding == nil {
				withholding = e.node
			}
		case extArt73:
			k.extCode("BR-IT-DE-004", e, []string{"SI"})
		case extRounding:
			if n := e.content.Elem("cbc:Amount"); n == nil || n.Trim() == "" {
				k.fail("BR-IT-DE-005", or(e.content, e.node), "Missing: cbc:Amount.")
			} else {
				k.amounts("BR-IT-DE-005", n)
			}
		case extDiscounts:
			charges := e.content.Elems("cac:AllowanceCharge")
			if len(charges) == 0 {
				k.fail("BR-IT-DE-006", or(e.content, e.node), "")
			}
			for _, c := range charges {
				if n := c.Elem("cbc:Amount"); n == nil || n.Trim() == "" {
					k.fail("BR-IT-DE-007", c, "Missing: cbc:Amount.")
				} else {
					k.prices("BR-IT-DE-007", n)
				}
				k.rates("BR-IT-DE-008", c.Elem("cbc:MultiplierFactorNumeric"))
			}
		}
	}
	k.withholdingTax(u, withholdings, withholding)
	k.extReferences(u)
	k.extSeller(u.Find("cac:AccountingSupplierParty/cac:Party"))
	for _, n := range u.FindAll("cac:TaxRepresentativeParty/cac:PartyIdentification/cbc:ID") {
		if strings.TrimSpace(n.Attr("schemeID")) == schemeCF {
			k.taxCode("BR-IT-DE-024", n, n.Trim())
		} else {
			v := n.Trim()
			code, ok := strings.CutPrefix(v, "EORI:")
			if !ok || len(code) < 13 || len(code) > 17 || !alnum(code) {
				k.fail("BR-IT-DE-025", n, "Value: %q.", v)
			}
		}
	}
	for _, d := range u.Elems("cac:Delivery") {
		k.extDelivery(d)
	}
	for _, n := range u.FindAll("cac:DeliveryTerms/cbc:ID") {
		if v := n.Trim(); length(v) != 3 {
			k.fail("BR-IT-DE-036", n, "Length: %d, value: %q.", length(v), v)
		}
	}
	for _, t := range u.Elems("cac:PaymentTerms") {
		k.amounts("BR-IT-DE-037", t.Elems("cbc:SettlementDiscountAmount")...)
		k.amounts("BR-IT-DE-038", t.Elems("cbc:PenaltyAmount")...)
	}
	for _, l := range ublLines(u) {
		k.maxLen("BR-IT-DE-039", 20, l.FindAll("cac:OrderLineReference/cac:OrderReference/cbc:ID")...)
		for _, r := range l.Elems("cac:DespatchLineReference") {
			if n := r.Elem("cbc:LineID"); n.Trim() != "NA" {
				k.fail("BR-IT-DE-040", or(n, r), "Value: %q.", n.Trim())
			}
			doc := r.Elem("cac:DocumentReference")
			k.maxLen("BR-IT-DE-041", 20, doc.Elem("cbc:ID"))
			if doc.Value("cbc:IssueDate") == "" {
				k.fail("BR-IT-DE-042", or(doc, r), "")
			}
		}
	}
}

// extCode checks that the content of extension e holds a cbc:TypeCode whose
// value is one of codes.
func (k *checker) extCode(id string, e extension, codes []string) {
	n := e.content.Elem("cbc:TypeCode")
	if n == nil {
		k.fail(id, or(e.content, e.node), "Missing: cbc:TypeCode.")
		return
	}
	if v := n.Trim(); !slices.Contains(codes, v) {
		k.fail(id, n, "Value: %q.", v)
	}
}

// withholdingTax checks the withholding taxes (BR-IT-DE-009 to 013):
// withholdings holds the indexes of the TipoRitenuta extensions, the first
// of which is ext.
func (k *checker) withholdingTax(u *xmltree.Node, withholdings map[int]bool, ext *xmltree.Node) {
	totals := u.Elems("cac:WithholdingTaxTotal")
	if ext != nil && len(totals) == 0 {
		k.fail("BR-IT-DE-009", ext, "")
	}
	i := 0 // ordinal of the withholding in the document
	for _, t := range totals {
		k.amounts("BR-IT-DE-010", t.Elem("cbc:TaxAmount"))
		subtotals := t.Elems("cac:TaxSubtotal")
		if len(subtotals) < 1 || len(subtotals) > 2 {
			k.fail("BR-IT-DE-011", t, "Withholdings: %d.", len(subtotals))
		}
		for _, s := range subtotals {
			i++
			amount := s.Elem("cbc:TaxAmount")
			k.amounts("BR-IT-DE-012", amount)
			var faults []string
			if !withholdings[i] && !(i == 1 && withholdings[0]) {
				if i == 1 {
					faults = append(faults, "no extension "+extURN+extWithholding)
				} else {
					faults = append(faults, "no extension "+extURN+extWithholding+"::"+strconv.Itoa(i))
				}
			}
			if amount.Trim() == "" {
				faults = append(faults, "no cbc:TaxAmount")
			}
			cat := s.Elem("cac:TaxCategory")
			if v := cat.Value("cbc:ID"); v != "S" {
				faults = append(faults, "category "+strconv.Quote(v)+" instead of \"S\"")
			}
			if v := cat.Value("cbc:Percent"); v == "" {
				faults = append(faults, "no cbc:Percent")
			} else if !fits(v, 6, 2, 2) {
				faults = append(faults, "rate "+strconv.Quote(v))
			}
			if v := cat.Value("cac:TaxScheme/cbc:ID"); v != "SWT" {
				faults = append(faults, "scheme "+strconv.Quote(v)+" instead of \"SWT\"")
			}
			if cat.Value("cac:TaxScheme/cbc:TaxTypeCode") == "" {
				faults = append(faults, "no reason for payment (cac:TaxScheme/cbc:TaxTypeCode)")
			}
			if len(faults) > 0 {
				k.fail("BR-IT-DE-013", s, "Withholding %d: %s.", i, strings.Join(faults, ", "))
			}
		}
	}
}

// extReferences checks the references to the main invoice and to the
// transport documents (BR-IT-DE-014 to 016).
func (k *checker) extReferences(u *xmltree.Node) {
	for _, r := range u.FindAll("cac:BillingReference/cac:AdditionalDocumentReference") {
		if missing := absent(r, "cbc:ID", "cbc:IssueDate"); missing != "" {
			k.fail("BR-IT-DE-014", r, "Missing: %s.", missing)
		}
		k.maxLen("BR-IT-DE-015", 20, r.Elem("cbc:ID"))
	}
	for _, r := range u.Elems("cac:DespatchDocumentReference") {
		if r.Value("cbc:IssueDate") == "" {
			k.fail("BR-IT-DE-016", r, "")
		}
	}
}

// extSeller checks the tax regime, the permanent establishment and the
// third-party intermediary of the seller (BR-IT-DE-017 to 023, 050, 051).
func (k *checker) extSeller(party *xmltree.Node) {
	for _, n := range party.FindAll("cac:PartyTaxScheme/cbc:TaxLevelCode") {
		if v := n.Trim(); !slices.Contains(taxRegimes, v) {
			k.fail("BR-IT-DE-017", n, "Value: %q.", v)
		}
	}
	for _, agent := range party.Elems("cac:AgentParty") {
		a := agent.Elem("cac:PostalAddress")
		if a == nil {
			k.fail("BR-IT-DE-018", agent, "Missing: cac:PostalAddress.")
		} else if missing := absent(a, "cbc:StreetName", "cbc:CityName", "cbc:PostalZone", "cbc:CountrySubentity", "cac:Country/cbc:IdentificationCode"); missing != "" {
			k.fail("BR-IT-DE-018", a, "Missing: %s.", missing)
		}
		k.maxLen("BR-IT-DE-019", 60, a.Elem("cbc:StreetName"))
		k.maxLen("BR-IT-DE-020", 60, a.Elem("cbc:CityName"))
		if n := a.Elem("cbc:PostalZone"); n != nil {
			if v := n.Trim(); len(v) != 5 || !digits(v) {
				k.fail("BR-IT-DE-021", n, "Value: %q.", v)
			}
		}
		k.maxLen("BR-IT-DE-022", 2, a.Elem("cbc:CountrySubentity"))
	}
	for _, p := range party.FindAll("cac:ServiceProviderParty/cac:Party") {
		var missing []string
		if p.Value("cac:PartyTaxScheme/cbc:CompanyID") == "" {
			missing = append(missing, "cac:PartyTaxScheme/cbc:CompanyID")
		}
		if p.Value("cac:PartyTaxScheme/cac:TaxScheme/cbc:ID") != "VAT" {
			missing = append(missing, "cac:PartyTaxScheme/cac:TaxScheme/cbc:ID with the value VAT")
		}
		if p.Value("cac:PartyName/cbc:Name") == "" && (p.Value("cac:Person/cbc:FirstName") == "" || p.Value("cac:Person/cbc:FamilyName") == "") {
			missing = append(missing, "cac:PartyName/cbc:Name or cac:Person/cbc:FirstName and cbc:FamilyName")
		}
		if len(missing) > 0 {
			k.fail("BR-IT-DE-023", p, "Missing: %s.", strings.Join(missing, ", "))
		}
		if strings.HasPrefix(p.Value("cac:PartyTaxScheme/cbc:CompanyID"), "IT") {
			var cf *xmltree.Node
			for _, n := range p.FindAll("cac:PartyIdentification/cbc:ID") {
				if strings.TrimSpace(n.Attr("schemeID")) == schemeCF {
					cf = n
					break
				}
			}
			if cf == nil {
				k.fail("BR-IT-DE-050", p, "Missing: cac:PartyIdentification/cbc:ID with the schemeID 0210.")
			} else {
				k.taxCode("BR-IT-DE-050", cf, cf.Trim())
			}
		}
		k.maxLen("BR-IT-DE-051", 80, p.FindAll("cac:PartyName/cbc:Name")...)
	}
}

// extDelivery checks the carrier, the pickup and the shipment of a delivery
// (BR-IT-DE-026 to 035).
func (k *checker) extDelivery(d *xmltree.Node) {
	for _, c := range d.Elems("cac:CarrierParty") {
		for _, n := range c.FindAll("cac:PartyIdentification/cbc:ID") {
			v := n.Trim()
			if strings.TrimSpace(n.Attr("schemeID")) == schemeCF {
				k.taxCode("BR-IT-DE-026", n, v)
			} else if code, ok := strings.CutPrefix(v, "CF:"); ok {
				k.taxCode("BR-IT-DE-026", n, code)
			} else {
				k.fail("BR-IT-DE-026", n, "Value: %q, without the schemeID 0210.", v)
			}
		}
		k.maxLen("BR-IT-DE-027", 80, c.FindAll("cac:PartyName/cbc:Name")...)
		k.maxLen("BR-IT-DE-027", 60, c.FindAll("cac:Person/cbc:FirstName")...)
		k.maxLen("BR-IT-DE-027", 60, c.FindAll("cac:Person/cbc:FamilyName")...)
		k.maxLen("BR-IT-DE-028", 30, c.FindAll("cac:PartyTaxScheme/cbc:CompanyID")...)
		for _, s := range c.Elems("cac:PartyTaxScheme") {
			if n := s.Find("cac:TaxScheme/cbc:ID"); n.Trim() != "VAT" {
				k.fail("BR-IT-DE-029", or(n, s), "Value: %q.", n.Trim())
			}
		}
		for _, r := range c.FindAll("cac:Person/cac:IdentityDocumentReference") {
			k.maxLen("BR-IT-DE-030", 20, r.Elem("cbc:ID"))
			if n := r.Elem("cbc:DocumentTypeCode"); n.Trim() != "40" {
				k.fail("BR-IT-DE-030", or(n, r), "Document type: %q.", n.Trim())
			}
		}
	}
	k.maxLen("BR-IT-DE-035", 100, d.FindAll("cac:Despatch/cbc:Instructions")...)
	for _, s := range d.Elems("cac:Shipment") {
		for _, n := range append(s.Elems("cbc:GrossWeightMeasure"), s.Elems("cbc:NetWeightMeasure")...) {
			if v := n.Trim(); strings.HasPrefix(v, "-") || !fits(v, 7, 2, 2) {
				k.fail("BR-IT-DE-031", n, "Value: %q.", v)
			}
		}
		for _, n := range s.Elems("cbc:TotalTransportHandlingUnitQuantity") {
			if v := n.Trim(); !packages(v) {
				k.fail("BR-IT-DE-032", n, "Value: %q.", v)
			}
		}
		k.maxLen("BR-IT-DE-033", 100, s.FindAll("cac:GoodsItem/cbc:Description")...)
		k.maxLen("BR-IT-DE-034", 80, s.FindAll("cac:ShipmentStage/cbc:TransportMeansTypeCode")...)
	}
}

// orderLines checks that an invoice line that references a purchase order
// line (BT-132) has a purchase order reference, in the header (BT-13) or in
// the line (BR-IT-DE-052).
func (k *checker) orderLines() {
	if u := k.d.UBL; u != nil {
		header := u.Value("cac:OrderReference/cbc:ID") != ""
		for _, l := range ublLines(u) {
			for _, r := range l.Elems("cac:OrderLineReference") {
				if n := r.Elem("cbc:LineID"); n.Trim() != "" && !header && r.Value("cac:OrderReference/cbc:ID") == "" {
					k.fail("BR-IT-DE-052", n, "Line %q, order line %q.", l.Value("cbc:ID"), n.Trim())
				}
			}
		}
		return
	}
	header := k.d.Agreement.Value("BuyerOrderReferencedDocument/IssuerAssignedID") != ""
	for _, l := range k.d.Lines {
		for _, r := range l.FindAll("SpecifiedLineTradeAgreement/BuyerOrderReferencedDocument") {
			if n := r.Elem("LineID"); n.Trim() != "" && !header && r.Value("IssuerAssignedID") == "" {
				k.fail("BR-IT-DE-052", n, "Line %q, order line %q.", l.Value("AssociatedDocumentLineDocument/LineID"), n.Trim())
			}
		}
	}
}

// taxCode checks that the Codice Fiscale v, the text of n or a part of it,
// has from 11 to 16 letters and digits (BR-IT-DE-024, 026, 050).
func (k *checker) taxCode(id string, n *xmltree.Node, v string) {
	if len(v) < 11 || len(v) > 16 || !alnum(v) {
		k.fail(id, n, "Value: %q.", n.Trim())
	}
}

// rates checks that each percentage, written with 2 decimals, has at most 6
// characters (the FatturaPA type RateType).
func (k *checker) rates(id string, nodes ...*xmltree.Node) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if v := n.Trim(); !fits(v, 6, 2, 2) {
			k.fail(id, n, "Value: %q.", v)
		}
	}
}

// absent returns the paths under parent that have no value, separated by
// commas.
func absent(parent *xmltree.Node, paths ...string) string {
	var missing []string
	for _, p := range paths {
		if parent.Value(p) == "" {
			missing = append(missing, p)
		}
	}
	return strings.Join(missing, ", ")
}

// packages reports whether v, a number of packages, is a whole number from
// 1 to 9999 (the FatturaPA element NumeroColli), possibly written with zero
// decimals.
func packages(v string) bool {
	intPart, frac, _ := strings.Cut(v, ".")
	if frac != "" && strings.Trim(frac, "0") != "" || !digits(intPart) {
		return false
	}
	n, err := strconv.Atoi(intPart)
	return err == nil && n >= 1 && n <= 9999
}
