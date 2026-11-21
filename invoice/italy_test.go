package invoice

import (
	"reflect"
	"testing"
)

// TestItalianDocumentTypes checks that the document types read back from
// their invoice type code are those that the code names alone.
func TestItalianDocumentTypes(t *testing.T) {
	for td, code := range italianDocumentTypes {
		back := DocumentTypeOfTypeCode(code)
		if TypeCodeOfDocumentType(back) != code {
			t.Errorf("%s: type code %s gives %s, of type code %s", td, code, back, TypeCodeOfDocumentType(back))
		}
	}
	for code, want := range map[string]string{"380": "TD01", "381": "TD04", "261": "TD04", "383": "TD05", "386": "TD02", "389": "TD01"} {
		if got := DocumentTypeOfTypeCode(code); got != want {
			t.Errorf("DocumentTypeOfTypeCode(%s) = %s, want %s", code, got, want)
		}
	}
	if got := TypeCodeOfDocumentType("TD99"); got != "" {
		t.Errorf("TypeCodeOfDocumentType(TD99) = %q", got)
	}
}

func TestNatures(t *testing.T) {
	for _, c := range []string{"Z", "E", "G", "K", "AE"} {
		n := NatureOfCategory(c)
		if back, _ := CategoryOfNature(n); back != c {
			t.Errorf("category %s: nature %s gives category %s", c, n, back)
		}
	}
	for _, c := range []string{"S", "B", "L", "M", ""} {
		if n := NatureOfCategory(c); n != "" {
			t.Errorf("NatureOfCategory(%q) = %s", c, n)
		}
	}
	for n, c := range natureCategories {
		if c[0] == "" || (c[0] != "Z") == (c[1] == "") {
			t.Errorf("nature %s: category %q, reason code %q", n, c[0], c[1])
		}
	}
	if c, r := CategoryOfNature("N8"); c != "" || r != "" {
		t.Errorf("CategoryOfNature(N8) = %q, %q", c, r)
	}
}

// TestPaymentMethods checks that each payment method read back from its
// type code is the same method, except for the methods that appendix 5 does
// not map from a type code.
func TestPaymentMethods(t *testing.T) {
	lossy := map[string]bool{"MP07": true, "MP09": true, "MP10": true, "MP11": true, "MP14": true, "MP16": true, "MP20": true, "MP21": true}
	for m, code := range typeCodesOfMethod {
		if back := PaymentMethodOfTypeCode(code); (back == m) == lossy[m] {
			t.Errorf("%s: type code %s gives %s", m, code, back)
		}
	}
	if len(typeCodesOfMethod) != 23 {
		t.Errorf("%d payment methods, want MP01 to MP23", len(typeCodesOfMethod))
	}
	for code, want := range map[string]string{"58": "MP05", " 30 ": "MP05", "59": "MP19", "10": "MP01", "1": "MP01", "999": "MP01"} {
		if got := PaymentMethodOfTypeCode(code); got != want {
			t.Errorf("PaymentMethodOfTypeCode(%q) = %s, want %s", code, got, want)
		}
	}
}

func TestItalianName(t *testing.T) {
	name := ItalianName("Maria", "Rossi")
	if name != "Nome#Cognome:Maria#Rossi" {
		t.Errorf("ItalianName = %q", name)
	}
	for _, c := range []struct {
		name, given, family string
		ok                  bool
	}{
		{name, "Maria", "Rossi", true},
		{"Nome&Cognome:Mario#De Luca", "Mario", "De Luca", true},
		{"Esempio S.r.l.", "", "", false},
	} {
		given, family, ok := SplitItalianName(c.name)
		if given != c.given || family != c.family || ok != c.ok {
			t.Errorf("SplitItalianName(%q) = %q, %q, %t", c.name, given, family, ok)
		}
	}
}

func TestNationalData(t *testing.T) {
	var inv Invoice
	if got := inv.NationalData(); got != nil {
		t.Errorf("NationalData of an invoice without national data = %q", got)
	}
	inv.Italy = &Italy{Art73: true, StampDuty: &StampDuty{Amount: MustDecimal("2.00")}}
	inv.Buyer.Italy = &ItalyParty{}
	inv.ShipTo = &Party{Italy: &ItalyParty{Fax: "0612345679"}}
	inv.Lines = []Line{{}, {Italy: &ItalyLine{Discounts: []Discount{{Type: "SC"}}}}}
	inv.PaymentMeans = []PaymentMeans{{Italy: &ItalyPayment{DueDate: MustDate("2026-12-20")}}}
	want := []string{"EXT Italy.StampDuty", "EXT Italy.Art73", "EXT ShipTo.Italy.Fax", "EXT Lines[1].Italy.Discounts", "EXT PaymentMeans[0].Italy.DueDate"}
	if got := inv.NationalData(); !reflect.DeepEqual(got, want) {
		t.Errorf("NationalData = %q, want %q", got, want)
	}
}
