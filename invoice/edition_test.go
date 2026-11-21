package invoice

import "testing"

func TestEditionOf(t *testing.T) {
	for urn, want := range map[string]Edition{
		"urn:cen.eu:en16931:2017":                                                    Edition2017,
		"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic":                Edition2017,
		"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended":            Edition2017,
		"urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic":                    Edition2017,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0": Edition2017,
		" URN:CEN.EU:EN16931:2017 ":                                                  Edition2017,
		"urn:cen.eu:en16931:2026":                                                    Edition2026,
		" urn:cen.eu:en16931:2026 ":                                                  Edition2026,
		"urn:cen.eu:en16931:2026#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0": Edition2026,
		"urn:cen.eu:en16931:2026#conformant#urn:factur-x.eu:1p0:extended":            Edition2026,
		"urn:cen.eu:en16931:2031":                                                    2031,
		"urn:factur-x.eu:1p0:minimum":                                                EditionNone,
		"urn:factur-x.eu:1p0:basicwl":                                                EditionNone,
		"urn:ferd:CrossIndustryDocument:invoice:1p0:comfort":                         EditionNone,
		"urn:fdc:peppol.eu:2017:poacc:billing:3.0":                                   EditionNone,
		"urn:cen.eu:en16931:20":                                                      EditionNone,
		"urn:cen.eu:en16931:2026x":                                                   EditionNone,
		"urn:cen.eu:en16931:0207":                                                    EditionNone,
		"urn:cen.eu:en16931:abcd":                                                    EditionNone,
		"urn:cen.eu:en16931:":                                                        EditionNone,
		"":                                                                           EditionNone,
		"urn:example:urn:cen.eu:en16931:2026":                                        EditionNone,
		"urn:cen.eu:en16931:2026#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0#conformant": Edition2026,
		// The Spanish B2B invoice (SPFE, draft of the AEAT).
		"urn:cen.eu:en16931:2026#conformant#urn:aeat.es:spfe:1.0.0:extended": Edition2026,
	} {
		if got := EditionOf(urn); got != want {
			t.Errorf("EditionOf(%q) = %d, want %d", urn, got, want)
		}
	}
}

func TestEdition(t *testing.T) {
	for _, c := range []struct {
		e         Edition
		s         string
		supported bool
	}{
		{EditionNone, "", true},
		{Edition2017, "EN 16931-1:2017", true},
		{Edition2026, "EN 16931-1:2026", false},
		{2031, "EN 16931-1:2031", false},
	} {
		if got := c.e.String(); got != c.s {
			t.Errorf("Edition(%d).String() = %q, want %q", c.e, got, c.s)
		}
		if got := c.e.Supported(); got != c.supported {
			t.Errorf("Edition(%d).Supported() = %v, want %v", c.e, got, c.supported)
		}
	}
	// Every identifier of the profiles of the package names the edition it
	// implements, or none.
	for _, p := range Profiles {
		if e := EditionOf(p.URN()); !e.Supported() {
			t.Errorf("EditionOf(%s.URN()) = %s, not supported", p, e)
		}
	}
	// The profile of a 2026 identifier is the one of its data set: the
	// edition is told apart by EditionOf, not by DetectProfile.
	if got := DetectProfile("urn:cen.eu:en16931:2026"); got != ProfileEN16931 {
		t.Errorf("DetectProfile(2026) = %s, want %s", got, ProfileEN16931)
	}
}
