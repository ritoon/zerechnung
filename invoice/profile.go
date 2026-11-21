package invoice

import (
	"fmt"
	"strings"
)

// Profile is a Factur-X / ZUGFeRD profile, identified in the XML by the
// specification identifier BT-24.
type Profile int

// Profiles, from the smallest to the largest data set. XRECHNUNG, PEPPOL
// (Peppol BIS Billing 3.0), NLCIUS (the Dutch CIUS, SI-UBL 2.0 in UBL) and
// CIUS-IT (the Italian CIUS, for invoices to Italian public administrations
// through the SDI) are CIUS of EN 16931 and sit at its level;
// EXTENDED-CTC-FR is the French subset of EXTENDED.
const (
	ProfileUnknown Profile = iota
	ProfileMinimum
	ProfileBasicWL
	ProfileBasic
	ProfileEN16931
	ProfileXRechnung
	ProfilePeppol
	ProfileNLCIUS
	ProfileCIUSIT
	ProfileExtendedCTCFR
	ProfileExtended
)

// Profiles lists the known profiles.
var Profiles = []Profile{
	ProfileMinimum, ProfileBasicWL, ProfileBasic, ProfileEN16931,
	ProfileXRechnung, ProfilePeppol, ProfileNLCIUS, ProfileCIUSIT, ProfileExtendedCTCFR, ProfileExtended,
}

// Specification identifiers (BT-24) written for each profile.
const (
	URNMinimum        = "urn:factur-x.eu:1p0:minimum"
	URNBasicWL        = "urn:factur-x.eu:1p0:basicwl"
	URNBasic          = "urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic"
	URNEN16931        = "urn:cen.eu:en16931:2017"
	URNExtended       = "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended"
	URNXRechnung      = "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0"
	URNExtendedCTCFR  = "urn:cen.eu:en16931:2017#conformant#urn.cpro.gouv.fr:1p0:extended-ctc-fr"
	URNPeppol         = "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	URNNLCIUS         = "urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0"
	URNCIUSIT         = "urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:2.0.0"
	urnCIUSIT         = "urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:"
	urnXRechnungOld   = "urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_"
	urnXRechnungNew   = "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_"
	urnExtendedCTCFR2 = "urn:cen.eu:en16931:2017#conformant#urn:cpro.gouv.fr:1p0:extended-ctc-fr"
)

// GAccountExtension ends the specification identifier (BT-24) of an SI-UBL
// 2.0 invoice that uses the Dutch G-account extension: URNNLCIUS followed by
// it. Such an invoice is paid in part to the G-account of the supplier (a
// blocked account for wage taxes and VAT): it has two means of payment
// identified by PaymentMeans.ID and two payment terms that refer to them
// (PaymentTerms.PaymentMeansID, PartialAmount). The Dutch Peppol Authority
// defines the extension for UBL only.
const GAccountExtension = "#conformant#urn:fdc:nen.nl:gaccount:v1.0"

// ItalianExtension ends the specification identifier (BT-24) of a domestic
// CIUS-IT invoice, whose seller is in Italy (BT-40 is IT): URNCIUSIT
// followed by it. The Italian technical rules (Regole tecniche, art. 3
// d.lgs. 148/2018, 4.1.3.1) give the identifier from the countries of the
// seller and of the buyer: URNCIUSIT for a cross-border invoice (the
// seller is not in Italy, the buyer is), URNCIUSIT followed by this
// extension for a domestic one, which may then carry the Italian
// extensions (EXT-IT) and gets the domestic rules of the CIUS (BR-IT-DC).
const ItalianExtension = "#conformant#urn:fatturapa.gov.it:EXT-IT:1.0.0"

// Specification identifiers of ZUGFeRD 1.0, in the
// rsm:CrossIndustryDocument syntax that predates CII D16B. They are read,
// never written (see ProfileFromZUGFeRD1URN).
const (
	URNZUGFeRD1Basic    = "urn:ferd:CrossIndustryDocument:invoice:1p0:basic"
	URNZUGFeRD1Comfort  = "urn:ferd:CrossIndustryDocument:invoice:1p0:comfort"
	URNZUGFeRD1Extended = "urn:ferd:CrossIndustryDocument:invoice:1p0:extended"
	urnZUGFeRD1         = "urn:ferd:CrossIndustryDocument:invoice:1p0:"
	urnZUGFeRD1RC       = "urn:ferd:invoice:rc:" // release candidate (2014)
)

// String returns the profile name as written in the XMP ConformanceLevel
// ("BASIC WL", "EN 16931"), or "PEPPOL", "NLCIUS", "CIUS-IT",
// "EXTENDED-CTC-FR" and "UNKNOWN".
func (p Profile) String() string {
	switch p {
	case ProfileMinimum:
		return "MINIMUM"
	case ProfileBasicWL:
		return "BASIC WL"
	case ProfileBasic:
		return "BASIC"
	case ProfileEN16931:
		return "EN 16931"
	case ProfileXRechnung:
		return "XRECHNUNG"
	case ProfilePeppol:
		return "PEPPOL"
	case ProfileNLCIUS:
		return "NLCIUS"
	case ProfileCIUSIT:
		return "CIUS-IT"
	case ProfileExtendedCTCFR:
		return "EXTENDED-CTC-FR"
	case ProfileExtended:
		return "EXTENDED"
	}
	return "UNKNOWN"
}

// URN returns the specification identifier (BT-24) of the profile. For
// CIUS-IT it is the cross-border identifier; see URNFor.
func (p Profile) URN() string {
	switch p {
	case ProfileMinimum:
		return URNMinimum
	case ProfileBasicWL:
		return URNBasicWL
	case ProfileBasic:
		return URNBasic
	case ProfileEN16931:
		return URNEN16931
	case ProfileXRechnung:
		return URNXRechnung
	case ProfilePeppol:
		return URNPeppol
	case ProfileNLCIUS:
		return URNNLCIUS
	case ProfileCIUSIT:
		return URNCIUSIT
	case ProfileExtendedCTCFR:
		return URNExtendedCTCFR
	case ProfileExtended:
		return URNExtended
	}
	return ""
}

// URNFor returns the specification identifier (BT-24) of the profile for
// an invoice whose seller is in country sellerCountry (BT-40): URN, except
// for CIUS-IT and a seller in Italy, where it is the domestic identifier,
// URNCIUSIT followed by ItalianExtension.
func (p Profile) URNFor(sellerCountry string) string {
	if p == ProfileCIUSIT && strings.EqualFold(strings.TrimSpace(sellerCountry), "IT") {
		return URNCIUSIT + ItalianExtension
	}
	return p.URN()
}

// ConformanceLevel returns the value of fx:ConformanceLevel in the XMP
// metadata of a Factur-X PDF. The Factur-X specification defines no value for
// EXTENDED-CTC-FR, Peppol BIS Billing 3.0, NLCIUS nor CIUS-IT: such invoices
// are declared EXTENDED and EN 16931.
func (p Profile) ConformanceLevel() string {
	switch p {
	case ProfileExtendedCTCFR:
		return "EXTENDED"
	case ProfilePeppol, ProfileNLCIUS, ProfileCIUSIT:
		return "EN 16931"
	case ProfileUnknown:
		return ""
	}
	return p.String()
}

// Filename returns the name of the XML attachment in a Factur-X PDF:
// "xrechnung.xml" for XRECHNUNG, "factur-x.xml" otherwise.
func (p Profile) Filename() string {
	if p == ProfileXRechnung {
		return "xrechnung.xml"
	}
	return "factur-x.xml"
}

// AFRelationship returns the default /AFRelationship of the XML attachment:
// "Data" for MINIMUM and BASIC WL, whose XML is not a full invoice,
// "Alternative" otherwise. Both values suit France and Germany. For BASIC
// and above, France also admits "Source" and "Data", while Germany admits
// only "Alternative" (Factur-X 1.09, 6.2.2).
func (p Profile) AFRelationship() string {
	if p == ProfileMinimum || p == ProfileBasicWL {
		return "Data"
	}
	return "Alternative"
}

// HasLines reports whether the profile carries invoice lines (BG-25).
func (p Profile) HasLines() bool { return p >= ProfileBasic }

// EN16931 reports whether invoices of this profile are complete EN 16931
// invoices (BASIC and above). MINIMUM and BASIC WL are not.
func (p Profile) EN16931() bool { return p >= ProfileBasic }

// level orders profiles by data set: XRECHNUNG, PEPPOL, NLCIUS and CIUS-IT
// are at the EN 16931 level, EXTENDED-CTC-FR below EXTENDED.
func (p Profile) level() int {
	switch p {
	case ProfileXRechnung, ProfilePeppol, ProfileNLCIUS, ProfileCIUSIT:
		return int(ProfileEN16931)
	default:
		return int(p)
	}
}

// AtLeast reports whether p carries at least the data set of q.
func (p Profile) AtLeast(q Profile) bool { return p.level() >= q.level() }

// MarshalText implements encoding.TextMarshaler: the profile name, as String.
func (p Profile) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler. It accepts what
// ParseProfile accepts; "UNKNOWN" and empty text give ProfileUnknown.
func (p *Profile) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || strings.EqualFold(s, "UNKNOWN") {
		*p = ProfileUnknown
		return nil
	}
	q := ParseProfile(s)
	if q == ProfileUnknown {
		return fmt.Errorf("invoice: unknown profile %q", s)
	}
	*p = q
	return nil
}

// ProfileFromURN returns the profile identified by a specification identifier
// (BT-24). It recognizes the Factur-X identifiers, the ZUGFeRD 2.x ones
// (urn:zugferd.de:2p0:…), all XRechnung versions, both spellings of the
// EXTENDED-CTC-FR identifier, the Peppol BIS Billing 3.0 identifier, alone
// or followed by more text as rule PEPPOL-EN16931-R004 admits, the
// NLCIUS identifier, alone or followed by an extension such as the G-account
// one (…#conformant#urn:fdc:nen.nl:gaccount:v1.0), and the CIUS-IT
// identifiers of any version, cross-border or domestic (followed by
// ItalianExtension). Surrounding white space is ignored.
func ProfileFromURN(urn string) Profile {
	u := strings.TrimSpace(urn)
	switch u {
	case URNMinimum, "urn:zugferd.de:2p0:minimum":
		return ProfileMinimum
	case URNBasicWL, "urn:zugferd.de:2p0:basicwl":
		return ProfileBasicWL
	case URNBasic, "urn:cen.eu:en16931:2017#compliant#urn:zugferd.de:2p0:basic":
		return ProfileBasic
	case URNEN16931:
		return ProfileEN16931
	case URNExtended, "urn:cen.eu:en16931:2017#conformant#urn:zugferd.de:2p0:extended":
		return ProfileExtended
	case URNExtendedCTCFR, urnExtendedCTCFR2:
		return ProfileExtendedCTCFR
	}
	if strings.HasPrefix(u, urnXRechnungNew) || strings.HasPrefix(u, urnXRechnungOld) {
		// "…xrechnung_3.0" and the extension variant "…xrechnung_3.0#conformant#…".
		return ProfileXRechnung
	}
	if strings.HasPrefix(u, URNPeppol) {
		return ProfilePeppol
	}
	if strings.HasPrefix(u, URNNLCIUS) {
		return ProfileNLCIUS
	}
	if strings.HasPrefix(u, urnCIUSIT) {
		return ProfileCIUSIT
	}
	return ProfileUnknown
}

// ProfileFromZUGFeRD1URN returns the profile matched to a ZUGFeRD 1.0
// specification identifier: BASIC for basic, EN 16931 for comfort, EXTENDED
// for extended. ZUGFeRD 1.0 predates EN 16931: the match is the profile with
// the same data set, not a conformance claim. The identifiers of the
// ZUGFeRD 1.0 release candidate (urn:ferd:invoice:rc:basic, comfort,
// extended) are recognized too. Surrounding white space is ignored; any
// other identifier gives ProfileUnknown.
func ProfileFromZUGFeRD1URN(urn string) Profile {
	u := strings.TrimSpace(urn)
	level, ok := strings.CutPrefix(u, urnZUGFeRD1)
	if !ok {
		if level, ok = strings.CutPrefix(u, urnZUGFeRD1RC); !ok {
			return ProfileUnknown
		}
	}
	switch level {
	case "basic":
		return ProfileBasic
	case "comfort":
		return ProfileEN16931
	case "extended":
		return ProfileExtended
	}
	return ProfileUnknown
}

// DetectProfile is a lenient ProfileFromURN: when the identifier is not one
// of the official ones, it recognizes the ZUGFeRD 1.0 identifiers (see
// ProfileFromZUGFeRD1URN), then looks for a profile name in it, which
// recognizes spellings found in older or non-conforming invoices such as
// "urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic". Use it to read
// invoices; validators report such identifiers with ProfileFromURN.
//
// The profile is the one of the data set, whatever the edition of
// EN 16931-1 that the identifier names: urn:cen.eu:en16931:2026 gives
// EN 16931, as urn:cen.eu:en16931:2017 does. EditionOf tells the edition.
func DetectProfile(urn string) Profile {
	if p := ProfileFromURN(urn); p != ProfileUnknown {
		return p
	}
	if p := ProfileFromZUGFeRD1URN(urn); p != ProfileUnknown {
		return p
	}
	u := strings.ToLower(strings.TrimSpace(urn))
	switch {
	case u == "":
		return ProfileUnknown
	case strings.Contains(u, "extended-ctc-fr"):
		return ProfileExtendedCTCFR
	case strings.Contains(u, "extended"):
		return ProfileExtended
	case strings.Contains(u, "xrechnung"):
		return ProfileXRechnung
	case strings.Contains(u, "nlcius"):
		return ProfileNLCIUS
	case strings.Contains(u, "cius-it"):
		return ProfileCIUSIT
	case strings.Contains(u, "basicwl"), strings.Contains(u, "basic-wl"):
		return ProfileBasicWL
	case strings.Contains(u, "basic"):
		return ProfileBasic
	case strings.Contains(u, "minimum"):
		return ProfileMinimum
	case strings.HasPrefix(u, "urn:cen.eu:en16931"):
		return ProfileEN16931
	}
	return ProfileUnknown
}

// levelSeparators removes the separators ProfileFromConformanceLevel ignores.
// A Replacer is safe for concurrent use; building one per call cost more
// than the rest of the function.
var levelSeparators = strings.NewReplacer(" ", "", "-", "", "_", "")

// ProfileFromConformanceLevel returns the profile named by an XMP
// ConformanceLevel value. Case, spaces, dashes and underscores are ignored, so
// "BASIC WL", "basicwl" and "EN16931" are recognized, as is the legacy
// ZUGFeRD 1 value "COMFORT" (EN 16931).
func ProfileFromConformanceLevel(level string) Profile {
	switch strings.ToUpper(levelSeparators.Replace(strings.TrimSpace(level))) {
	case "MINIMUM":
		return ProfileMinimum
	case "BASICWL":
		return ProfileBasicWL
	case "BASIC":
		return ProfileBasic
	case "EN16931", "COMFORT":
		return ProfileEN16931
	case "XRECHNUNG":
		return ProfileXRechnung
	case "EXTENDED":
		return ProfileExtended
	case "EXTENDEDCTCFR":
		return ProfileExtendedCTCFR
	}
	return ProfileUnknown
}

// ParseProfile returns the profile named by s, which may be a profile name
// ("en16931", "basic-wl", "extended-ctc-fr", "peppol", "nlcius", "cius-it")
// or a specification identifier.
func ParseProfile(s string) Profile {
	if p := ProfileFromURN(s); p != ProfileUnknown {
		return p
	}
	// Not XMP ConformanceLevel values: Peppol, NLCIUS and CIUS-IT invoices
	// are declared EN 16931.
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PEPPOL":
		return ProfilePeppol
	case "NLCIUS":
		return ProfileNLCIUS
	case "CIUS-IT", "CIUSIT", "CIUS_IT":
		return ProfileCIUSIT
	}
	return ProfileFromConformanceLevel(s)
}
