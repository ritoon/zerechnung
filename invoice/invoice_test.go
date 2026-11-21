package invoice

import (
	"testing"
	"time"
)

func TestParseDecimal(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{"12.50", "12.50"}, {"-3", "-3"}, {"+7", "7"}, {".5", "0.5"}, {"7.", "7"},
		{" 0.075 ", "0.075"}, {"-0.50", "-0.50"}, {"000123.4500", "123.4500"}, {"0", "0"}, {"0.00", "0.00"},
	} {
		d, err := ParseDecimal(c.in)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", c.in, err)
			continue
		}
		if got := d.String(); got != c.out {
			t.Errorf("ParseDecimal(%q) = %q, want %q", c.in, got, c.out)
		}
	}
	for _, in := range []string{"", "-", ".", "1,5", "1e3", "12.3.4", "abc", "1234567890123456789"} {
		if _, err := ParseDecimal(in); err == nil {
			t.Errorf("ParseDecimal(%q) succeeded", in)
		}
	}
}

func TestDecimalArithmetic(t *testing.T) {
	d := MustDecimal
	for _, c := range []struct {
		name string
		got  Decimal
		want string
	}{
		{"add", d("1.5").Add(d("2.25")), "3.75"},
		{"sub", d("1").Sub(d("2.50")), "-1.50"},
		{"mul", d("3").Mul(d("19.99")), "59.97"},
		{"mul scale", d("1.5").Mul(d("0.25")), "0.375"},
		{"div", d("10").Div(d("3"), 4), "3.3333"},
		{"div round", d("2").Div(d("3"), 2), "0.67"},
		{"div neg", d("-2").Div(d("3"), 2), "-0.67"},
		{"round half up", d("2.345").Round(2), "2.35"},
		{"round half away neg", d("-2.345").Round(2), "-2.35"},
		{"round half ceil neg", d("-2.345").RoundHalfCeil(2), "-2.34"},
		{"round pad", d("3").Round(2), "3.00"},
		{"absent add", Decimal{}.Add(d("1.1")), "1.1"},
		{"big mul", d("123456789.1234").Mul(d("98765.4321")), "12193263123451.18122"},
		{"overflow mul", d("12345678901.2345").Mul(d("1234567.8912")), "15241578766529409.74"},
		{"saturate", d("123456789012.345678").Mul(d("123456789.123456")), "9223372036854775807"},
	} {
		if s := c.got.String(); s != c.want {
			t.Errorf("%s: got %s, want %s", c.name, s, c.want)
		}
	}
	if !d("1.50").Equal(d("1.5")) || d("1").Cmp(d("1.01")) != -1 {
		t.Error("numeric comparison")
	}
	if (Decimal{}).IsSet() || (Decimal{}).String() != "" {
		t.Error("zero value must be absent")
	}
	if v := d("1").Div(d("0"), 2); v.IsSet() {
		t.Error("division by zero must give an absent value")
	}
}

func TestDate(t *testing.T) {
	dt, err := ParseDate("20261116", "102")
	if err != nil || dt != NewDate(2026, time.November, 16) {
		t.Fatalf("ParseDate 102: %v %v", dt, err)
	}
	if dt.Format102() != "20261116" || dt.String() != "2026-11-16" {
		t.Errorf("formats: %s %s", dt.Format102(), dt)
	}
	if _, err := ParseDate("20260230", "102"); err == nil {
		t.Error("February 30 accepted")
	}
	if _, err := ParseDate("2026-11-16", "102"); err != nil {
		t.Error("ISO date refused")
	}
	if m, _ := ParseDate("202602", "610"); m != NewDate(2026, 2, 1) {
		t.Errorf("format 610: %v", m)
	}
	// Format 616 (ISO 8601 week) gives the Monday of the week.
	for s, want := range map[string]Date{
		"201338": NewDate(2013, time.September, 16),
		"202601": NewDate(2025, time.December, 29), // week 1 of 2026 starts in 2025
		"202053": NewDate(2020, time.December, 28),
	} {
		if w, err := ParseDate(s, "616"); err != nil || w != want {
			t.Errorf("format 616 %s: %v %v, want %v", s, w, err, want)
		}
	}
	for _, s := range []string{"202153", "202600", "2026W1", "2026011"} {
		if w, err := ParseDate(s, "616"); err == nil {
			t.Errorf("format 616 %s accepted: %v", s, w)
		}
	}
	if !NewDate(2026, 1, 31).Before(NewDate(2026, 2, 1)) || NewDate(2026, 1, 31).AddDays(1) != NewDate(2026, 2, 1) {
		t.Error("date arithmetic")
	}
}

func TestProfile(t *testing.T) {
	for _, p := range Profiles {
		if got := ProfileFromURN(p.URN()); got != p {
			t.Errorf("ProfileFromURN(%s.URN()) = %s", p, got)
		}
		if got := ParseProfile(p.String()); got != p {
			t.Errorf("ParseProfile(%q) = %s", p.String(), got)
		}
		if p != ProfileExtendedCTCFR && p != ProfilePeppol && p != ProfileNLCIUS && p != ProfileCIUSIT {
			if got := ProfileFromConformanceLevel(p.ConformanceLevel()); got != p {
				t.Errorf("ProfileFromConformanceLevel(%q) = %s", p.ConformanceLevel(), got)
			}
		}
	}
	for urn, want := range map[string]Profile{
		"urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_1.2":                                                     ProfileXRechnung,
		"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0": ProfileXRechnung,
		"urn:zugferd.de:2p0:basicwl": ProfileBasicWL,
		" urn:cen.eu:en16931:2017 ":  ProfileEN16931,
		"urn:cen.eu:en16931:2017#conformant#urn:cpro.gouv.fr:1p0:extended-ctc-fr":                              ProfileExtendedCTCFR,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0":                           ProfilePeppol,
		" urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0#conformant#urn:example ":  ProfilePeppol,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:2.0":                           ProfileUnknown,
		"urn:fdc:peppol.eu:2017:poacc:billing:3.0":                                                             ProfileUnknown,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0":                                         ProfileNLCIUS,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0": ProfileNLCIUS,
		"urn:fdc:nen.nl:nlcius:v1.0": ProfileUnknown,
		"urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:2.0.0":                                              ProfileCIUSIT,
		"urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:2.0.0#conformant#urn:fatturapa.gov.it:EXT-IT:1.0.0": ProfileCIUSIT,
		" urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:1.0.0.20170801 ":                                   ProfileCIUSIT,
		"urn:fatturapa.gov.it:CIUS-IT:2.0.0":                                                                                ProfileUnknown,
		"urn:example":                                                                                                       ProfileUnknown,
	} {
		if got := ProfileFromURN(urn); got != want {
			t.Errorf("ProfileFromURN(%q) = %s, want %s", urn, got, want)
		}
	}
	// ZUGFeRD 1.0 identifiers: matched by ProfileFromZUGFeRD1URN and
	// DetectProfile, not by ProfileFromURN, which validators use for CII D16B.
	for urn, want := range map[string]Profile{
		URNZUGFeRD1Basic:                                     ProfileBasic,
		" " + URNZUGFeRD1Comfort + "\n":                      ProfileEN16931,
		URNZUGFeRD1Extended:                                  ProfileExtended,
		"urn:ferd:invoice:rc:comfort":                        ProfileEN16931,
		"urn:ferd:CrossIndustryDocument:invoice:1p0:komfort": ProfileUnknown,
		"urn:ferd:CrossIndustryDocument:order:1p0:extended":  ProfileUnknown,
		URNEN16931: ProfileUnknown,
	} {
		if got := ProfileFromZUGFeRD1URN(urn); got != want {
			t.Errorf("ProfileFromZUGFeRD1URN(%q) = %s, want %s", urn, got, want)
		}
		if want != ProfileUnknown {
			if got := DetectProfile(urn); got != want {
				t.Errorf("DetectProfile(%q) = %s, want %s", urn, got, want)
			}
			if got := ProfileFromURN(urn); got != ProfileUnknown {
				t.Errorf("ProfileFromURN(%q) = %s, want UNKNOWN", urn, got)
			}
		}
	}
	if ParseProfile("basic-wl") != ProfileBasicWL || ParseProfile("en16931") != ProfileEN16931 || ParseProfile("extended-ctc-fr") != ProfileExtendedCTCFR ||
		ParseProfile(" peppol ") != ProfilePeppol || ProfileFromConformanceLevel("PEPPOL") != ProfileUnknown ||
		ParseProfile("NLCIUS") != ProfileNLCIUS || ProfileFromConformanceLevel("NLCIUS") != ProfileUnknown {
		t.Error("ParseProfile")
	}
	if ProfileNLCIUS.ConformanceLevel() != "EN 16931" || ProfileNLCIUS.Filename() != "factur-x.xml" ||
		!ProfileNLCIUS.AtLeast(ProfileEN16931) || ProfileNLCIUS.AtLeast(ProfileExtendedCTCFR) || !ProfileNLCIUS.EN16931() ||
		DetectProfile("urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius") != ProfileNLCIUS {
		t.Error("NLCIUS profile")
	}
	if ProfileCIUSIT.ConformanceLevel() != "EN 16931" || ProfileCIUSIT.Filename() != "factur-x.xml" || ProfileCIUSIT.String() != "CIUS-IT" ||
		!ProfileCIUSIT.AtLeast(ProfileEN16931) || ProfileCIUSIT.AtLeast(ProfileExtendedCTCFR) || !ProfileCIUSIT.EN16931() ||
		ParseProfile("cius-it") != ProfileCIUSIT || ParseProfile("CIUSIT") != ProfileCIUSIT || ProfileFromConformanceLevel("CIUS-IT") != ProfileUnknown ||
		DetectProfile("urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:cius-it") != ProfileCIUSIT {
		t.Error("CIUS-IT profile")
	}
	if ProfileCIUSIT.URNFor(" it ") != URNCIUSIT+ItalianExtension || ProfileCIUSIT.URNFor("FR") != URNCIUSIT ||
		ProfileCIUSIT.URNFor("") != URNCIUSIT || ProfilePeppol.URNFor("IT") != URNPeppol {
		t.Error("URNFor")
	}
	if ProfilePeppol.ConformanceLevel() != "EN 16931" || ProfilePeppol.Filename() != "factur-x.xml" || ProfilePeppol.AFRelationship() != "Alternative" {
		t.Error("Peppol XMP values")
	}
	if ProfileXRechnung.Filename() != "xrechnung.xml" || ProfileBasic.Filename() != "factur-x.xml" {
		t.Error("Filename")
	}
	if ProfileMinimum.AFRelationship() != "Data" || ProfileEN16931.AFRelationship() != "Alternative" {
		t.Error("AFRelationship")
	}
	if !ProfileXRechnung.AtLeast(ProfileEN16931) || ProfileXRechnung.AtLeast(ProfileExtendedCTCFR) || ProfileBasicWL.HasLines() ||
		!ProfilePeppol.AtLeast(ProfileEN16931) || !ProfileEN16931.AtLeast(ProfilePeppol) || ProfilePeppol.AtLeast(ProfileExtendedCTCFR) ||
		!ProfilePeppol.EN16931() || !ProfilePeppol.HasLines() {
		t.Error("profile levels")
	}
}

func TestCalculate(t *testing.T) {
	d := MustDecimal
	inv := &Invoice{
		Lines: []Line{
			{ID: "1", Quantity: d("3"), NetPrice: d("19.99"), VATCategoryCode: VATStandard, VATRate: d("20")},
			{ID: "2", Quantity: d("2"), GrossPrice: d("10.00"), PriceDiscount: d("1.00"), VATCategoryCode: VATStandard, VATRate: d("20"),
				AllowancesCharges: []AllowanceCharge{{Percentage: d("10"), BaseAmount: d("18.00")}}},
			{ID: "3", Quantity: d("1"), NetPrice: d("100"), PriceBaseQuantity: d("4"), VATCategoryCode: VATStandard, VATRate: d("5.5")},
			{ID: "4", Quantity: d("1"), NetPrice: d("50"), VATCategoryCode: VATExempt, VATRate: d("0")},
			{ID: "G", StatusReasonCode: "GROUP", NetAmount: d("999.00"), VATCategoryCode: VATStandard, VATRate: d("20")},
		},
		AllowancesCharges: []AllowanceCharge{
			{Charge: true, Amount: d("5.00"), VATCategoryCode: VATStandard, VATRate: d("20"), Reason: "Port"},
			{Amount: d("10.00"), VATCategoryCode: VATStandard, VATRate: d("5.5")},
		},
		VATBreakdown:      []VATBreakdown{{CategoryCode: VATExempt, Rate: d("0"), ExemptionReason: "Exonération article 261"}},
		Totals:            Totals{PaidAmount: d("10.00")},
		ThirdPartyCharges: []ThirdPartyCharge{{Reason: "Stamp duty", Amount: d("2.50")}},
	}
	inv.Calculate()

	wantLines := []string{"59.97", "16.20", "25.00", "50.00", "999.00"}
	for i, w := range wantLines {
		if got := inv.Lines[i].NetAmount.String(); got != w {
			t.Errorf("line %d net amount = %s, want %s", i+1, got, w)
		}
	}
	tt := inv.Totals
	for name, c := range map[string][2]string{
		"BT-106": {tt.LineTotal.String(), "151.17"},
		"BT-107": {tt.AllowanceTotal.String(), "10.00"},
		"BT-108": {tt.ChargeTotal.String(), "5.00"},
		"BT-109": {tt.TaxBasisTotal.String(), "146.17"},
		"BT-110": {tt.TaxTotal.String(), "17.06"},
		"BT-112": {tt.GrandTotal.String(), "163.23"},
		"BT-115": {tt.DuePayable.String(), "155.73"}, // 163.23 − 10.00 + 2.50 (BG-34)
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
	if len(inv.VATBreakdown) != 3 {
		t.Fatalf("VAT breakdown has %d entries: %+v", len(inv.VATBreakdown), inv.VATBreakdown)
	}
	// S 20 %: 59.97 + 16.20 + 5.00 = 81.17 → 16.234 → 16.23; S 5.5 %: 25 − 10 = 15 → 0.825 → 0.83; E: 50.
	for i, w := range [][3]string{{"S", "81.17", "16.23"}, {"S", "15.00", "0.83"}, {"E", "50.00", "0.00"}} {
		b := inv.VATBreakdown[i]
		if b.CategoryCode != w[0] || b.TaxableAmount.String() != w[1] || b.TaxAmount.String() != w[2] {
			t.Errorf("breakdown %d = %s %s %s, want %v", i, b.CategoryCode, b.TaxableAmount, b.TaxAmount, w)
		}
	}
	if inv.VATBreakdown[2].ExemptionReason == "" {
		t.Error("exemption reason lost")
	}
}

func TestCalculateWithoutLines(t *testing.T) {
	d := MustDecimal
	inv := &Invoice{
		VATBreakdown: []VATBreakdown{
			{CategoryCode: VATStandard, Rate: d("20"), TaxableAmount: d("100.00")},
			{CategoryCode: VATStandard, Rate: d("10"), TaxableAmount: d("50.00")},
		},
	}
	inv.Calculate()
	if inv.Totals.TaxBasisTotal.String() != "150.00" || inv.Totals.TaxTotal.String() != "25.00" ||
		inv.Totals.GrandTotal.String() != "175.00" || inv.Totals.DuePayable.String() != "175.00" {
		t.Errorf("totals: %+v", inv.Totals)
	}
}
