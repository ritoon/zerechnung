package cii

import (
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// level is the data set of a profile: each level carries the elements of the
// levels below it. XRECHNUNG, PEPPOL, NLCIUS and CIUS-IT use the EN 16931 set,
// EXTENDED-CTC-FR the EXTENDED set.
type level uint8

const (
	lvlMinimum level = iota + 1
	lvlBasicWL
	lvlBasic
	lvlEN16931
	lvlExtended
)

func levelOf(p invoice.Profile) level {
	switch p {
	case invoice.ProfileMinimum:
		return lvlMinimum
	case invoice.ProfileBasicWL:
		return lvlBasicWL
	case invoice.ProfileBasic:
		return lvlBasic
	case invoice.ProfileExtended, invoice.ProfileExtendedCTCFR:
		return lvlExtended
	}
	return lvlEN16931
}

// detectProfile returns the profile identified by a specification identifier
// (BT-24), accepting the non-standard spellings found in the wild.
func detectProfile(urn string) invoice.Profile { return invoice.DetectProfile(urn) }

// zugferd1URN reports whether urn is a ZUGFeRD 1.0 specification identifier
// (urn:ferd:…), in any case.
func zugferd1URN(urn string) bool {
	u := strings.TrimSpace(urn)
	return len(u) >= 9 && strings.EqualFold(u[:9], "urn:ferd:")
}

// role is the role of a trade party; the elements a party may carry depend on
// it.
type role uint8

const (
	roleSeller role = iota
	roleBuyer
	roleSellerTaxRep
	rolePayee
	roleShipTo
	roleSalesAgent
	roleBuyerAgent
	roleInvoicer
	roleInvoicee
	rolePayer
	roleLineShipTo
	roleProductEndUser
	roleUltimateShipTo
	roleShipFrom
	roleLineUltimateShipTo
	numRoles
)

var roleNames = [numRoles]string{
	"Seller", "Buyer", "SellerTaxRepresentative", "Payee", "ShipTo",
	"SalesAgent", "BuyerAgent", "Invoicer", "Invoicee", "Payer", "Line.ShipTo",
	"ProductEndUser", "UltimateShipTo", "ShipFrom", "Line.UltimateShipTo",
}

// Party fields, used to index partyTerms.
const (
	fGroup = iota
	fIDs
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

var partyFieldNames = [numPartyFields]string{
	"", "IDs", "Name", "Description", "LegalRegistration", "TradingName",
	"Contact", "Address", "ElectronicAddress", "VATID", "TaxRegistrationID",
}

// partyTerms gives, for each role and field, the business term reported when
// the target profile cannot carry the field. Fields without a business term
// for a role are reported as "EXT <Role>.<Field>".
var partyTerms = func() (t [numRoles][numPartyFields]string) {
	set := func(r role, terms ...string) {
		for i, s := range terms {
			if s != "" {
				t[r][i] = s
			}
		}
	}
	for r := role(0); r < numRoles; r++ {
		t[r][fGroup] = "EXT " + roleNames[r]
		for f := fIDs; f < numPartyFields; f++ {
			t[r][f] = "EXT " + roleNames[r] + "." + partyFieldNames[f]
		}
	}
	set(roleSeller, "BG-4", "BT-29", "BT-27", "BT-33", "BT-30", "BT-28", "BG-6", "BG-5", "BT-34", "BT-31", "BT-32")
	set(roleBuyer, "BG-7", "BT-46", "BT-44", "", "BT-47", "BT-45", "BG-9", "BG-8", "BT-49", "BT-48", "")
	set(roleSellerTaxRep, "BG-11", "", "BT-62", "", "", "", "", "BG-12", "", "BT-63", "")
	set(rolePayee, "BG-10", "BT-60", "BT-59", "", "BT-61")
	set(roleShipTo, "BG-13", "BT-71", "BT-70", "", "", "", "", "BG-15")
	return t
}()

// partyRules tells which elements a party may carry.
type partyRules struct {
	ids, legal, trading, contact, country, address, uri, vat, fc, desc bool
}

// rulesFor returns the elements allowed for a party role in a profile, from
// the Factur-X 1.09.2 schemas and the "not used in the given context" rules of
// the profile schematrons.
func rulesFor(r role, l level) partyRules {
	if l >= lvlExtended {
		return partyRules{
			ids: true, legal: true, trading: true, contact: true, country: true,
			address: true, uri: true, vat: true, fc: true, desc: true,
		}
	}
	switch r {
	case roleSeller:
		return partyRules{
			ids: l >= lvlBasicWL, legal: true, trading: l >= lvlBasicWL,
			contact: l >= lvlEN16931, country: true, address: l >= lvlBasicWL,
			uri: l >= lvlBasicWL, vat: true, fc: true, desc: l >= lvlEN16931,
		}
	case roleBuyer:
		return partyRules{
			ids: l >= lvlBasicWL, legal: true, trading: l >= lvlEN16931,
			contact: l >= lvlEN16931, country: l >= lvlBasicWL, address: l >= lvlBasicWL,
			uri: l >= lvlBasicWL, vat: l >= lvlBasicWL,
		}
	case roleSellerTaxRep:
		return partyRules{country: true, address: true, vat: true}
	case rolePayee:
		return partyRules{ids: true, legal: true}
	case roleShipTo:
		return partyRules{ids: true, country: true, address: true}
	}
	return partyRules{}
}
