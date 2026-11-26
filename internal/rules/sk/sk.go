// Package sk checks the requirements of the Slovak electronic invoice
// (eFaktúra) that go beyond Peppol BIS Billing 3.0.
//
// From 1 January 2027, law 385/2025 Z.z., which amends the VAT law 222/2004
// Z.z., makes the electronic invoice mandatory between VAT payers established
// in Slovakia, in B2B and B2G, with the reporting of its data to the
// Financial Administration (Finančná správa) in a five-corner model. The
// format is Peppol BIS Billing 3.0 in UBL 2.1, with the Peppol specification
// identifier in BT-24: the Peppol rule set already applies to these
// invoices, PEPPOL-EN16931-P0112 included, which admits the type code 384
// between two Slovak parties. Slovakia has no national rules in the Peppol
// schematron and publishes no schematron of its own.
//
// The Financial Administration publishes instead a "transposition of Peppol
// BIS to the Slovak legislation" (version 1.13 of 29 October 2026), a
// workbook that ties each business term to the Slovak laws (VAT law
// 222/2004, accounting law 431/2002, Commercial Code 513/1991, law 215/2019
// on electronic invoicing) and marks it "required" or "conditionally
// required", without rule identifiers, and answers the questions of the
// implementers in an FAQ (20 November 2026). The rules of this package
// implement what these documents require beyond Peppol BIS; their
// identifiers (SK-*) are specific to the package:
//
//   - SK-01, SK-02: the street, city and post code of the seller (BT-35,
//     BT-37, BT-38) and of the buyer (BT-50, BT-52, BT-53), which § 74(1)(a)
//     and (b) of the VAT law require although EN 16931 makes them optional;
//   - SK-03: the same data of the seller's tax representative (BT-64, BT-66,
//     BT-67) when the invoice has one (BG-11);
//   - SK-04, SK-05: the electronic address of a Slovak seller (BT-34) and of
//     a Slovak buyer (BT-49) has the scheme 0245, the Slovak tax
//     identification number (DIČ), which the Peppol Authority Specific
//     Requirements make the identifier of a Slovak participant (version 1.2
//     of the workbook replaced 9950 by 0245);
//   - SK-06: an identifier of scheme 0245 is a DIČ, ten digits;
//   - SK-07, SK-08: the VAT identifier (IČ DPH) of the seller (BT-31) and of
//     the buyer (BT-48), required when one was assigned (§ 74(1)(a) and
//     (b));
//   - SK-09: a Slovak VAT identifier is SK followed by ten digits;
//   - SK-10, SK-11: the identification number of the organisation (IČO) of a
//     Slovak seller (BT-30) and of a Slovak buyer (BT-47), required when one
//     was assigned (§ 3a of the Commercial Code);
//   - SK-12: an IČO without scheme has eight digits;
//   - SK-13: the legal form and the entry in the commercial or trade register
//     of a Slovak seller (BT-33, § 3a of the Commercial Code);
//   - SK-14: the date of the taxable supply (BT-7), required when it is known
//     and differs from the issue date (§ 74(1)(d));
//   - SK-15: a corrective invoice (type 384) refers to the invoice it
//     corrects (BT-25).
//
// A requirement without condition is an error. A conditional requirement is
// an error when the invoice tells that its condition holds, a warning
// otherwise:
//
//   - a VAT category other than E and O (S, Z, AE, K, G, L, M) tells that the
//     seller is a VAT payer, hence has a VAT identifier; reverse charge (AE)
//     tells that the buyer is one too. Exempt supplies (E) may come from a
//     taxable person that is not registered for VAT, and BR-O-02 forbids the
//     VAT identifiers on an invoice of category O;
//   - the name of a party ending in the legal form of a company that the
//     Commercial Code registers in the commercial register (s.r.o., a.s.,
//     k.s., v.o.s., j.s.a., družstvo: §§ 2, 57, 78, 106, 154, 220a and 221 of
//     the Commercial Code require it in the business name) tells that it has
//     an IČO and a register entry;
//   - an actual delivery date (BT-72) other than the issue date tells that
//     the date of the supply is known and differs from it, unless the VAT
//     point date code (BT-8) gives it.
//
// The workbook has no check digit for the DIČ, the IČ DPH or the IČO, and
// the Financial Administration publishes none: only their format is checked.
//
// The rules run on the CII view of a UBL invoice and locate their findings in
// the UBL document. The tax data document that the access points report to
// the Financial Administration (TDD SK, urn:peppol:taxdata:sk-1), its own
// rules and Peppol transmission are outside the package. The eFaktúra also
// carries the self-billed invoices (389) and credit notes (261) of Peppol BIS
// Self-Billing 3.0, with the requirements of the invoice 380 (FAQ of the
// Financial Administration of 20/11/2026): these rules apply to them as to
// any invoice, and the Peppol rule set gives them the rules of Self-Billing.
// If Peppol BIS gains national rules for Slovakia, they belong to
// the Peppol rule set, and the rules of this package they restate are to be
// removed.
package sk

import (
	"slices"
	"strings"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rule identifiers.
const (
	RuleSellerAddress   = "SK-01" // street, city and post code of the seller
	RuleBuyerAddress    = "SK-02" // street, city and post code of the buyer
	RuleTaxRepAddress   = "SK-03" // street, city and post code of the tax representative
	RuleSellerEndpoint  = "SK-04" // electronic address of a Slovak seller of scheme 0245
	RuleBuyerEndpoint   = "SK-05" // electronic address of a Slovak buyer of scheme 0245
	RuleDIC             = "SK-06" // format of a DIČ
	RuleSellerVATID     = "SK-07" // VAT identifier of the seller
	RuleBuyerVATID      = "SK-08" // VAT identifier of the buyer
	RuleICDPH           = "SK-09" // format of an IČ DPH
	RuleSellerICO       = "SK-10" // IČO of a Slovak seller
	RuleBuyerICO        = "SK-11" // IČO of a Slovak buyer
	RuleICO             = "SK-12" // format of an IČO
	RuleSellerLegalInfo = "SK-13" // legal form and register entry of a Slovak seller
	RuleTaxPointDate    = "SK-14" // date of the taxable supply
	RuleCorrection      = "SK-15" // reference of a corrective invoice
)

// SchemeDIC is the ISO 6523 scheme of the Slovak tax identification number
// (DIČ), the identifier of a Slovak participant in Peppol.
const SchemeDIC = "0245"

// Catalog lists the rules of Check. The conditional requirements (SK-07,
// SK-08, SK-10, SK-11, SK-13) are errors when the invoice tells that their
// condition holds, warnings otherwise.
var Catalog = []rules.RuleInfo{
	{ID: RuleSellerAddress, Severity: rules.Error, Description: "The seller postal address (BG-5) shall contain the address line 1 (BT-35), the city (BT-37) and the post code (BT-38) (§ 74(1)(a) of the Slovak VAT law)."},
	{ID: RuleBuyerAddress, Severity: rules.Error, Description: "The buyer postal address (BG-8) shall contain the address line 1 (BT-50), the city (BT-52) and the post code (BT-53) (§ 74(1)(b) of the Slovak VAT law)."},
	{ID: RuleTaxRepAddress, Severity: rules.Error, Description: "When the invoice has a seller tax representative (BG-11), its postal address (BG-12) shall contain the address line 1 (BT-64), the city (BT-66) and the post code (BT-67)."},
	{ID: RuleSellerEndpoint, Severity: rules.Error, Description: "A seller in Slovakia (BT-40 = SK) shall have an electronic address (BT-34) of scheme 0245, its DIČ (Peppol Authority Specific Requirements of Slovakia)."},
	{ID: RuleBuyerEndpoint, Severity: rules.Error, Description: "A buyer in Slovakia (BT-55 = SK) shall have an electronic address (BT-49) of scheme 0245, its DIČ (Peppol Authority Specific Requirements of Slovakia)."},
	{ID: RuleDIC, Severity: rules.Error, Description: "An identifier of scheme 0245 shall be a Slovak tax identification number (DIČ): ten digits."},
	{ID: RuleSellerVATID, Severity: rules.Error, Description: "The invoice shall contain the seller VAT identifier (BT-31) or the seller tax representative VAT identifier (BT-63) when the seller has a VAT identifier (§ 74(1)(a) of the Slovak VAT law): an error when a VAT category other than E and O is used, a warning for an invoice of category E."},
	{ID: RuleBuyerVATID, Severity: rules.Error, Description: "The invoice shall contain the buyer VAT identifier (BT-48) when the buyer has a VAT identifier (§ 74(1)(b) of the Slovak VAT law): an error for reverse charge (AE), a warning for the other VAT categories but O."},
	{ID: RuleICDPH, Severity: rules.Error, Description: "A Slovak VAT identifier (IČ DPH) in BT-31, BT-48 or BT-63 shall be SK followed by ten digits."},
	{ID: RuleSellerICO, Severity: rules.Error, Description: "A seller in Slovakia shall have its identification number of organisation (IČO) as legal registration identifier (BT-30) when one was assigned (§ 3a of the Commercial Code): an error when its name gives the legal form of a company of the commercial register, a warning otherwise."},
	{ID: RuleBuyerICO, Severity: rules.Error, Description: "A buyer in Slovakia shall have its identification number of organisation (IČO) as legal registration identifier (BT-47) when one was assigned: an error when its name gives the legal form of a company of the commercial register, a warning otherwise."},
	{ID: RuleICO, Severity: rules.Error, Description: "The legal registration identifier without scheme of a seller (BT-30) or a buyer (BT-47) in Slovakia shall be an IČO: eight digits."},
	{ID: RuleSellerLegalInfo, Severity: rules.Error, Description: "A seller in Slovakia shall give its legal form and the register, section and entry number of its registration in the seller additional legal information (BT-33) (§ 3a of the Commercial Code): an error when its name gives the legal form of a company of the commercial register, a warning otherwise."},
	{ID: RuleTaxPointDate, Severity: rules.Error, Description: "The invoice shall contain the VAT point date (BT-7) when the date of the supply is known and differs from the issue date (§ 74(1)(d) of the Slovak VAT law): when the actual delivery date (BT-72) differs from the invoice issue date (BT-2) and the VAT point date code (BT-8) is not given."},
	{ID: RuleCorrection, Severity: rules.Error, Description: "A corrective invoice (BT-3 = 384) shall contain the reference of the invoice it corrects (BT-25)."},
}

// Paths from ram:ApplicableHeaderTradeAgreement and the parties.
const (
	pSeller    = "SellerTradeParty"
	pBuyer     = "BuyerTradeParty"
	pTaxRep    = "SellerTaxRepresentativeTradeParty"
	pAddress   = "PostalTradeAddress"
	pCountry   = "PostalTradeAddress/CountryID"
	pEndpoint  = "URIUniversalCommunication/URIID"
	pTaxReg    = "SpecifiedTaxRegistration/ID"
	pLegalID   = "SpecifiedLegalOrganization/ID"
	pLegalInfo = "Description"
)

// Check runs the Slovak rules on c.Doc and records the findings in
// c.Report.
func Check(c *rules.Checker) {
	if c == nil || c.Doc == nil || c.Doc.Root == nil {
		return
	}
	d := c.Doc
	agr := d.Agreement
	seller, buyer := agr.Elem(pSeller), agr.Elem(pBuyer)
	sellerSK := country(seller) == "SK"
	buyerSK := country(buyer) == "SK"
	cats := categories(d)

	// SK-01 to SK-03: postal addresses.
	checkAddress(c, RuleSellerAddress, seller, agr, "seller", "BT-35", "BT-37", "BT-38")
	checkAddress(c, RuleBuyerAddress, buyer, agr, "buyer", "BT-50", "BT-52", "BT-53")
	if rep := agr.Elem(pTaxRep); rep != nil {
		checkAddress(c, RuleTaxRepAddress, rep, agr, "seller tax representative", "BT-64", "BT-66", "BT-67")
	}

	// SK-04 to SK-06: electronic addresses and identifiers of scheme 0245.
	if sellerSK {
		checkEndpoint(c, RuleSellerEndpoint, seller, agr, "seller", "BT-34")
	}
	if buyerSK {
		checkEndpoint(c, RuleBuyerEndpoint, buyer, agr, "buyer", "BT-49")
	}
	checkDICs(c)

	// SK-07 to SK-09: VAT identifiers.
	checkSellerVATID(c, seller, agr.Elem(pTaxRep), agr, cats)
	checkBuyerVATID(c, buyer, agr, cats)
	for _, p := range []struct{ party, bt string }{{pSeller, "BT-31"}, {pBuyer, "BT-48"}, {pTaxRep, "BT-63"}} {
		for _, n := range vatIDs(agr.Elem(p.party)) {
			if v := n.Normalized(); strings.HasPrefix(v, "SK") && !ValidICDPH(v) {
				c.Fail(RuleICDPH, rules.Error, n, "[%s] The Slovak VAT identifier (IČ DPH) %s shall be SK followed by ten digits; value: %q.", RuleICDPH, p.bt, v)
			}
		}
	}

	// SK-10 to SK-13: IČO and legal information.
	if sellerSK {
		checkICO(c, RuleSellerICO, seller, agr, "seller", "BT-30")
		checkLegalInfo(c, seller, agr)
	}
	if buyerSK {
		checkICO(c, RuleBuyerICO, buyer, agr, "buyer", "BT-47")
	}

	// SK-14: date of the taxable supply.
	checkTaxPointDate(c)

	// SK-15: corrective invoice.
	if tc := d.Header.Elem("TypeCode"); tc.Trim() == "384" {
		refs := d.Settlement.FindAll("InvoiceReferencedDocument/IssuerAssignedID")
		if !slices.ContainsFunc(refs, func(n *xmltree.Node) bool { return n.Trim() != "" }) {
			c.Fail(RuleCorrection, rules.Error, tc, "[%s] A corrective invoice (BT-3 = 384) shall contain the reference of the invoice it corrects (BT-25).", RuleCorrection)
		}
	}
}

// country returns the country code of the postal address of a party.
func country(p *xmltree.Node) string {
	return strings.TrimSpace(p.Value(pCountry))
}

// or returns n, or alt when n is nil.
func or(n, alt *xmltree.Node) *xmltree.Node {
	if n != nil {
		return n
	}
	return alt
}

// checkAddress reports with rule the address line 1, city and post code
// that the postal address of party lacks; name and the business terms name
// the party and these data in the message.
func checkAddress(c *rules.Checker, rule string, party, agr *xmltree.Node, name, line, city, post string) {
	addr := party.Elem(pAddress)
	var missing []string
	for _, f := range []struct{ elem, bt, what string }{
		{"LineOne", line, "address line 1"},
		{"CityName", city, "city"},
		{"PostcodeCode", post, "post code"},
	} {
		if addr.Value(f.elem) == "" {
			missing = append(missing, f.what+" ("+f.bt+")")
		}
	}
	if len(missing) > 0 {
		c.Fail(rule, rules.Error, or(addr, or(party, agr)), "[%s] The %s postal address shall contain the %s: the Slovak VAT law (§ 74(1)) requires the address on the invoice.", rule, name, join(missing))
	}
}

// join joins the items of a list in English: "a", "a and b", "a, b and c".
func join(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// checkEndpoint checks that the electronic address of a Slovak party has
// the scheme 0245.
func checkEndpoint(c *rules.Checker, rule string, party, agr *xmltree.Node, name, bt string) {
	n := party.Find(pEndpoint)
	switch scheme := strings.TrimSpace(n.Attr("schemeID")); {
	case n == nil || n.Trim() == "":
		c.Fail(rule, rules.Error, or(n, or(party, agr)), "[%s] A %s in Slovakia shall have an electronic address (%s) of scheme 0245, its DIČ.", rule, name, bt)
	case scheme != SchemeDIC:
		c.Fail(rule, rules.Error, n, "[%s] The electronic address (%s) of a %s in Slovakia shall have the scheme 0245 (DIČ), not %q: the Peppol Authority of Slovakia identifies Slovak participants by their DIČ (9950 is no longer accepted).", rule, bt, name, scheme)
	}
}

// dicPaths are the identifiers that can carry the scheme 0245, from
// ram:ApplicableHeaderTradeAgreement, with their business terms.
var dicPaths = []struct{ path, bt string }{
	{pSeller + "/" + pEndpoint, "BT-34"},
	{pSeller + "/GlobalID", "BT-29"},
	{pSeller + "/" + pLegalID, "BT-30"},
	{pBuyer + "/" + pEndpoint, "BT-49"},
	{pBuyer + "/GlobalID", "BT-46"},
	{pBuyer + "/" + pLegalID, "BT-47"},
}

// checkDICs checks that the identifiers of scheme 0245 are DIČ.
func checkDICs(c *rules.Checker) {
	agr := c.Doc.Agreement
	for _, p := range dicPaths {
		for _, n := range agr.FindAll(p.path) {
			if strings.TrimSpace(n.Attr("schemeID")) != SchemeDIC {
				continue
			}
			if v := n.Normalized(); !ValidDIC(v) {
				c.Fail(RuleDIC, rules.Error, n, "[%s] An identifier of scheme 0245 (%s) shall be a Slovak tax identification number (DIČ), ten digits; value: %q.", RuleDIC, p.bt, v)
			}
		}
	}
}

// vatIDs returns the VAT identifiers of a party (scheme VA).
func vatIDs(p *xmltree.Node) []*xmltree.Node {
	var out []*xmltree.Node
	for _, n := range p.FindAll(pTaxReg) {
		if strings.TrimSpace(n.Attr("schemeID")) == "VA" && n.Trim() != "" {
			out = append(out, n)
		}
	}
	return out
}

// categories returns the VAT category codes of the invoice: of the VAT
// breakdown (BT-118), of the lines (BT-151) and of the document level
// allowances and charges (BT-95, BT-102).
func categories(d *rules.Doc) map[string]bool {
	out := map[string]bool{}
	add := func(nodes []*xmltree.Node) {
		for _, n := range nodes {
			if v := n.Trim(); v != "" {
				out[v] = true
			}
		}
	}
	add(d.Settlement.FindAll("ApplicableTradeTax/CategoryCode"))
	add(d.Settlement.FindAll("SpecifiedTradeAllowanceCharge/CategoryTradeTax/CategoryCode"))
	for _, l := range d.Lines {
		add(l.FindAll("SpecifiedLineTradeSettlement/ApplicableTradeTax/CategoryCode"))
	}
	return out
}

// payerCategories are the VAT categories that only a VAT payer invoices:
// VAT charged (S, Z, L, M), reverse charge (AE), intra-community supply (K)
// and export (G). A supplier that is not a VAT payer invoices O.
var payerCategories = []string{"S", "Z", "AE", "K", "G", "L", "M"}

// checkSellerVATID reports a missing VAT identifier of the seller (BT-31)
// when the invoice has no tax representative VAT identifier (BT-63).
func checkSellerVATID(c *rules.Checker, seller, rep, agr *xmltree.Node, cats map[string]bool) {
	if len(vatIDs(seller)) > 0 || len(vatIDs(rep)) > 0 {
		return
	}
	at := or(seller, agr)
	for _, cat := range payerCategories {
		if cats[cat] {
			c.Fail(RuleSellerVATID, rules.Error, at, "[%s] An invoice with the VAT category %s, which only a VAT payer invoices, shall contain the seller VAT identifier (BT-31, IČ DPH) or the seller tax representative VAT identifier (BT-63) (§ 74(1)(a) of the Slovak VAT law).", RuleSellerVATID, cat)
			return
		}
	}
	if cats["E"] {
		c.Fail(RuleSellerVATID, rules.Warning, at, "[%s] The invoice should contain the seller VAT identifier (BT-31, IČ DPH) if the seller has one (§ 74(1)(a) of the Slovak VAT law): exempt supplies (VAT category E) may come from a taxable person that is not registered for VAT.", RuleSellerVATID)
	}
}

// checkBuyerVATID reports a missing VAT identifier of the buyer (BT-48).
func checkBuyerVATID(c *rules.Checker, buyer, agr *xmltree.Node, cats map[string]bool) {
	if len(vatIDs(buyer)) > 0 {
		return
	}
	at := or(buyer, agr)
	switch {
	case cats["AE"]:
		c.Fail(RuleBuyerVATID, rules.Error, at, "[%s] An invoice with reverse charge (VAT category AE) shall contain the buyer VAT identifier (BT-48, IČ DPH), under which the buyer is liable for the VAT (§ 74(1)(b) of the Slovak VAT law).", RuleBuyerVATID)
	case len(cats) > 0 && !(len(cats) == 1 && cats["O"]):
		c.Fail(RuleBuyerVATID, rules.Warning, at, "[%s] The invoice should contain the buyer VAT identifier (BT-48, IČ DPH) if the buyer has one, under which the goods or services were supplied (§ 74(1)(b) of the Slovak VAT law).", RuleBuyerVATID)
	}
}

// checkICO checks the IČO of a Slovak party: the legal registration
// identifier (BT-30, BT-47) is required when one was assigned (rule), and
// has the format of an IČO when it has no scheme (SK-12).
func checkICO(c *rules.Checker, rule string, party, agr *xmltree.Node, name, bt string) {
	n := party.Find(pLegalID)
	if v := n.Normalized(); v != "" {
		if _, scheme := n.LookupAttr("schemeID"); !scheme && !ValidICO(v) {
			c.Fail(RuleICO, rules.Error, n, "[%s] The legal registration identifier (%s) of a %s in Slovakia shall be its identification number of organisation (IČO), eight digits; value: %q.", RuleICO, bt, name, v)
		}
		return
	}
	if form := CompanyForm(party.Value("Name")); form != "" {
		c.Fail(rule, rules.Error, or(party, agr), "[%s] The %s, a company (%s) of the commercial register, shall give its identification number of organisation (IČO) as legal registration identifier (%s) (§ 3a of the Commercial Code).", rule, name, form, bt)
		return
	}
	c.Fail(rule, rules.Warning, or(party, agr), "[%s] The %s in Slovakia should give its identification number of organisation (IČO) as legal registration identifier (%s) if one was assigned (§ 3a of the Commercial Code).", rule, name, bt)
}

// checkLegalInfo checks the additional legal information of a Slovak
// seller (BT-33).
func checkLegalInfo(c *rules.Checker, seller, agr *xmltree.Node) {
	if slices.ContainsFunc(seller.Elems(pLegalInfo), func(n *xmltree.Node) bool { return n.Trim() != "" }) {
		return
	}
	if form := CompanyForm(seller.Value("Name")); form != "" {
		c.Fail(RuleSellerLegalInfo, rules.Error, or(seller, agr), "[%s] The seller, a company (%s) of the commercial register, shall give its legal form and the register, section and entry number of its registration in the seller additional legal information (BT-33) (§ 3a of the Commercial Code).", RuleSellerLegalInfo, form)
		return
	}
	c.Fail(RuleSellerLegalInfo, rules.Warning, or(seller, agr), "[%s] A seller in Slovakia that is an entrepreneur should give its legal form and the register and entry number of its registration in the seller additional legal information (BT-33) (§ 3a of the Commercial Code).", RuleSellerLegalInfo)
}

// checkTaxPointDate reports a missing VAT point date (BT-7) when the actual
// delivery date (BT-72) differs from the issue date (BT-2) and no VAT point
// date code (BT-8) is given.
func checkTaxPointDate(c *rules.Checker) {
	d := c.Doc
	taxes := d.Settlement.Elems("ApplicableTradeTax")
	for _, t := range taxes {
		if t.Value("TaxPointDate/DateString") != "" || t.Value("DueDateTypeCode") != "" {
			return
		}
	}
	// BT-7 and BT-8 of a UBL invoice without VAT breakdown are not in the
	// CII view.
	if u := d.UBL; u != nil && (u.Value("TaxPointDate") != "" || u.Value("InvoicePeriod/DescriptionCode") != "") {
		return
	}
	// Both dates have the format 102 (YYYYMMDD), in the CII view of a UBL
	// invoice too; the message gives them as the document does.
	in := d.Header.Find("IssueDateTime/DateTimeString")
	n := d.Delivery.Find("ActualDeliverySupplyChainEvent/OccurrenceDateTime/DateTimeString")
	if issue, delivery := in.Trim(), n.Trim(); issue != "" && delivery != "" && delivery != issue {
		c.Fail(RuleTaxPointDate, rules.Error, n, "[%s] The actual delivery date (BT-72, %s) differs from the invoice issue date (BT-2, %s): the invoice shall give the date of the taxable supply in the VAT point date (BT-7) (§ 74(1)(d) of the Slovak VAT law).", RuleTaxPointDate, d.Origin(n).Trim(), d.Origin(in).Trim())
	}
}
