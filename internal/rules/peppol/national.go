package peppol

import (
	"regexp"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// National rules of the schematron: Norway, Denmark, Italy, Sweden and the
// Netherlands. Each applies when the seller is established in the country.

var (
	reNorwegianVAT = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[0-9]{9}MVA$`) })
	reItalianFC    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Z0-9]{11,16}$`) })
)

// norway checks the rules of the Norwegian pattern on every seller, when
// $supplierCountry is NO.
func (k *checker) norway() {
	if k.supplierCountry != "NO" {
		return
	}
	for _, s := range k.sellers {
		ok := false
		for id := range all(s, "SpecifiedTaxRegistration/ID") {
			if attr(id, "schemeID") == "FC" && norm(id.Text) == "Foretaksregisteret" {
				ok = true
				break
			}
		}
		k.check(ok, "NO-R-002", s, "")

		var vatIDs []*xmltree.Node // SpecifiedTaxRegistration[ram:ID/@schemeID = 'VAT']/ram:ID
		norwegian := false
		for _, r := range s.Elems("SpecifiedTaxRegistration") {
			if !isVATRegistration(r) {
				continue
			}
			ids := r.Elems("ID")
			vatIDs = append(vatIDs, ids...)
			if len(ids) > 0 && substring(sv(ids[0]), 1, 2) == "NO" {
				norwegian = true
			}
		}
		if norwegian {
			v := sv(vatIDs[0])
			k.check(reNorwegianVAT().MatchString(substringFrom(v, 3)) && mod11(substring(v, 3, 9)), "NO-R-001", vatIDs[0], "value: %q", v)
		}
	}
}

// denmark checks the Danish pattern: document rules for a seller in
// Denmark, payment means and line rules when the buyer is in Denmark too.
func (k *checker) denmark() {
	if !k.cii || !anyEq(k.root, pSeller+"/PostalTradeAddress/CountryID", "DK") {
		return
	}
	r := k.root
	customer := anyEq(r, pBuyer+"/PostalTradeAddress/CountryID", "DK")

	legal := k.absFirst(pSeller + "/SpecifiedLegalOrganization/ID")
	k.check(norm(text(legal)) != "", "DK-R-002", first(r, pSeller), "")

	if customer && anyEq(r, pSettlement+"/SpecifiedTradeAllowanceCharge/ReasonCode", "ZZZ") {
		reasons := k.abs(pSettlement + "/SpecifiedTradeAllowanceCharge/Reason")
		var reason, reasonText string
		if len(reasons) > 0 {
			reason, reasonText = sv(reasons[0]), text(reasons[0])
		}
		// number(Reason <= 9999) is the general comparison Reason <= 9999.
		taxCategory := length(norm(reasonText)) == 4 && number(reason) >= 0 &&
			anyNumber(reasons, func(f float64) bool { return f <= 9999 })
		hashed := len(reasons) > 0 && strings.Contains(reason, "#") && !strings.HasPrefix(reason, "#") && !strings.HasSuffix(reason, "#")
		k.check(taxCategory || hashed, "DK-R-004", first(r, pSettlement+"/SpecifiedTradeAllowanceCharge"), "reason %q", reason)
	}

	if customer {
		noScheme := func(party string) bool {
			return exists(r, party+"/GlobalID") && firstAttr(r, party+"/GlobalID", "schemeID") == ""
		}
		k.check(!noScheme(pSeller) && !noScheme(pBuyer), "DK-R-013", first(r, pAgreement), "")
	}
	if legal != nil {
		k.check(firstAttr(r, pSeller+"/SpecifiedLegalOrganization/ID", "schemeID") == "0184", "DK-R-014", legal,
			"scheme %q", firstAttr(r, pSeller+"/SpecifiedLegalOrganization/ID", "schemeID"))
	}
	if customer && norm(text(k.absFirst(pTypeCode))) == "381" {
		due := k.absFirst(pSummation + "/DuePayableAmount")
		k.check(!(number(text(due)) < 0), "DK-R-016", due, "value: %q", text(due))
	}

	if !customer {
		return
	}
	for _, pm := range k.abs(pPaymentMeans) {
		k.danishPaymentMeans(pm)
	}
	for _, line := range k.abs(pLines) {
		const class = "SpecifiedTradeProduct/DesignatedProductClassification/ClassCode"
		if anyAttrIn(line, class, "listID", "TST") {
			k.check(anyAttrIn(line, class, "listVersionID", "19.05.01", "19.0501", "26.08.01", "26.0801"), "DK-R-003", line,
				"version %q", attr(first(line, class), "listVersionID"))
		}
	}
}

// danishPaymentMeans checks DK-R-005 to DK-R-011 on a payment means of a
// domestic Danish invoice.
func (k *checker) danishPaymentMeans(pm *xmltree.Node) {
	code := sv(first(pm, "TypeCode"))
	k.check(strings.Contains(" 1 10 31 42 48 49 50 58 59 93 97 ", " "+code+" "), "DK-R-005", pm, "value: %q", code)

	is := func(c string) bool { return anyEq(pm, "TypeCode", c) }
	iban := text(first(pm, "PayeePartyCreditorFinancialAccount/IBANID"))
	ref := sv(first(pm.Parent, "PaymentReference"))
	prefix := substring(ref, 0, 4) // the first three characters

	if is("31") || is("42") {
		k.check(norm(iban) != "" && norm(text(first(pm, "PayeeSpecifiedCreditorFinancialInstitution/BICID"))) != "", "DK-R-006", pm, "")
	}
	if is("49") {
		// The mandate reference is looked up below the payment means.
		k.check(norm(text(first(pm.Parent, "CreditorReferenceID"))) != "" && norm(text(first(pm, "SpecifiedTradePaymentTerms/DirectDebitMandateID"))) != "",
			"DK-R-007", pm, "")
	}
	if is("50") {
		k.check(in(prefix, "01#", "04#", "15#") && length(iban) == 7, "DK-R-008", pm, "payment reference %q, account %q", ref, iban)
		if in(prefix, "04#", "15#") {
			k.check(length(ref) == 19, "DK-R-009", pm, "payment reference %q", ref)
		}
	}
	if is("93") {
		k.check(in(prefix, "71#", "73#", "75#") && length(iban) == 8, "DK-R-010", pm, "payment reference %q, account %q", ref, iban)
		if in(prefix, "71#", "75#") {
			k.check(length(ref) == 18 || length(ref) == 19, "DK-R-011", pm, "payment reference %q", ref)
		}
	}
}

// italy checks the Italian pattern on every seller, when $supplierCountry is
// IT.
func (k *checker) italy() {
	if k.supplierCountry != "IT" {
		return
	}
	for _, s := range k.sellers {
		for _, r := range s.Elems("SpecifiedTaxRegistration") {
			ok := false
			var fc *xmltree.Node
			for _, id := range r.Elems("ID") {
				if norm(attr(id, "schemeID")) != "FC" {
					ok = true
				} else if fc == nil {
					fc = id
				}
			}
			k.check(ok || reItalianFC().MatchString(nsv(fc)), "IT-R-001", r, "value: %q", nsv(fc))
		}
		k.check(exists(s, "PostalTradeAddress/LineOne"), "IT-R-002", s, "")
		k.check(exists(s, "PostalTradeAddress/CityName"), "IT-R-003", s, "")
		k.check(exists(s, "PostalTradeAddress/PostcodeCode"), "IT-R-004", s, "")
	}
}

// sweden checks the Swedish pattern, for sellers whose address is in Sweden.
func (k *checker) sweden() {
	swedish := func(party *xmltree.Node) bool { return anyEq(party, "PostalTradeAddress/CountryID", "SE") }
	// swedishVAT reports whether a registration has a VAT identifier
	// (schemeID 'VAT') starting with SE.
	swedishVAT := func(r *xmltree.Node) bool {
		for _, id := range r.Elems("ID") {
			if attr(id, "schemeID") == "VAT" {
				return substring(sv(id), 1, 2) == "SE"
			}
		}
		return false
	}
	taxRates := false
	for _, s := range k.abs(pSeller) {
		if !swedish(s) {
			continue
		}
		var vat []*xmltree.Node // ram:SpecifiedTaxRegistration/ram:ID[@schemeID = 'VAT']
		hasSwedishVAT := false
		for _, r := range s.Elems("SpecifiedTaxRegistration") {
			if swedishVAT(r) {
				hasSwedishVAT = true
			}
			for _, id := range r.Elems("ID") {
				if attr(id, "schemeID") == "VAT" {
					vat = append(vat, id)
				}
			}
		}
		if hasSwedishVAT {
			taxRates = true
			v := sv(vat[0])
			k.check(length(norm(v)) == 14, "SE-R-001", vat[0], "value: %q", v)
			k.check(isNumber(substring(v, 3, 12)), "SE-R-002", vat[0], "value: %q", v)
		}
		for _, lo := range s.Elems("SpecifiedLegalOrganization") {
			id := lo.Elem("ID")
			if id == nil {
				continue
			}
			v := nsv(id)
			k.check(isNumber(sv(id)), "SE-R-003", id, "value: %q", v)
			k.check(length(v) == 10, "SE-R-004", id, "value: %q", v)
			k.check(swedishOrgNumber(v), "SE-R-013", id, "value: %q", v)
		}
		if exists(s, "SpecifiedLegalOrganization/ID") {
			for id := range all(s, "SpecifiedTaxRegistration/ID") {
				if attr(id, "schemeID") == "FC" {
					k.check(norm(strings.ToUpper(sv(id))) == "GODKÄND FÖR F-SKATT", "SE-R-005", id, "value: %q", sv(id))
				}
			}
		}
	}
	if taxRates {
		for _, t := range k.taxes {
			rate := number(sv(first(t, "RateApplicablePercent")))
			k.check(rate == 25 || rate == 12 || rate == 6, "SE-R-006", t, "rate %q", sv(first(t, "RateApplicablePercent")))
		}
	}

	for _, tx := range k.abs(pTransaction) {
		if !anyEqParty(tx, "ApplicableHeaderTradeAgreement/SellerTradeParty", swedish) {
			continue
		}
		buyer := anyEqParty(tx, "ApplicableHeaderTradeAgreement/BuyerTradeParty", swedish)
		for pm := range all(tx, "ApplicableHeaderTradeSettlement/SpecifiedTradeSettlementPaymentMeans") {
			code := nsv(first(pm, "TypeCode"))
			branch := nsv(first(pm, "PayeeSpecifiedCreditorFinancialInstitution/BICID"))
			for id := range all(pm, "PayeePartyCreditorFinancialAccount/ProprietaryID") {
				v := nsv(id)
				switch {
				case code == "30" && branch == "SE:PLUSGIRO":
					k.check(isNumber(v), "SE-R-007", id, "value: %q", v)
					k.check(length(v) >= 2 && length(v) <= 8, "SE-R-010", id, "value: %q", v)
				case code == "30" && branch == "SE:BANKGIRO":
					k.check(isNumber(v), "SE-R-008", id, "value: %q", v)
					k.check(length(v) == 7 || length(v) == 8, "SE-R-009", id, "value: %q", v)
				}
			}
			switch {
			case code == "50" || code == "56":
				k.fail("SE-R-011", pm, "payment means code %s", code)
			case code == "31" && buyer:
				k.fail("SE-R-012", pm, "")
			}
		}
	}
}

// anyEqParty reports whether one of the elements matching path below n
// satisfies f.
func anyEqParty(n *xmltree.Node, path string, f func(*xmltree.Node) bool) bool {
	for p := range all(n, path) {
		if f(p) {
			return true
		}
	}
	return false
}

// netherlands checks the Dutch pattern, for a seller whose address is in
// the Netherlands.
func (k *checker) netherlands() {
	country := func(path string) bool {
		return strings.ToUpper(nsv(k.absFirst(path+"/PostalTradeAddress/CountryID"))) == "NL"
	}
	if !country(pSeller) {
		return
	}
	customer, taxRep := country(pBuyer), country(pTaxRep)

	for _, d := range k.abs("ExchangedDocument") {
		if in(nsv(first(d, "TypeCode")), "81", "83", "381", "396", "532") {
			ok := false
			for _, s := range k.settlements {
				if exists(s, "InvoiceReferencedDocument/IssuerAssignedID") {
					ok = true
					break
				}
			}
			k.check(ok, "NL-R-001", d, "")
		}
	}
	address := func(id, path string) {
		for _, a := range k.abs(path + "/PostalTradeAddress") {
			k.check(exists(a, "LineOne") && exists(a, "CityName") && exists(a, "PostcodeCode"), id, a, "")
		}
	}
	legal := func(id, path string) {
		for _, n := range k.abs(path + "/SpecifiedLegalOrganization/ID") {
			s := " " + attr(n, "schemeID") + " "
			k.check((strings.Contains(s, " 0106 ") || strings.Contains(s, " 0190 ")) && nsv(n) != "", id, n, "scheme %q", attr(n, "schemeID"))
		}
	}
	address("NL-R-002", pSeller)
	legal("NL-R-003", pSeller)
	if customer {
		address("NL-R-004", pBuyer)
		legal("NL-R-005", pBuyer)
	}
	if taxRep {
		address("NL-R-006", pTaxRep)
	}

	creditNote := norm(text(k.absFirst(pTypeCode))) == "381"
	paymentMeans := len(k.abs(pPaymentMeans)) > 0
	for _, sum := range k.abs(pSummation) {
		dn := first(sum, "DuePayableAmount")
		due, ok := dec(dn)
		if dn != nil && !ok {
			continue // xs:decimal() fails in the reference
		}
		paid := ok && (!creditNote && due.Sign() <= 0 || creditNote && due.Sign() >= 0)
		k.check(paid || paymentMeans, "NL-R-007", sum, "amount due %q", sv(first(sum, "DuePayableAmount")))
	}
	if customer {
		for _, pm := range k.abs(pPaymentMeans) {
			code := nsv(first(pm, "TypeCode"))
			k.check(in(code, "30", "48", "49", "57", "58", "59"), "NL-R-008", pm, "value: %q", code)
		}
	}
	order := len(k.abs(pAgreement+"/BuyerOrderReferencedDocument/IssuerAssignedID")) > 0
	for _, l := range k.abs(pLines + "/SpecifiedLineTradeAgreement/BuyerOrderReferencedDocument/LineID") {
		k.check(order, "NL-R-009", l, "")
	}
}
