package invoice

import "testing"

// TestSpanishUnits checks that each Facturae unit of measure with a unit
// code reads back from it.
func TestSpanishUnits(t *testing.T) {
	seen := map[string]string{}
	for u, code := range unitsOfMeasure {
		if other, ok := seen[code]; ok {
			t.Errorf("units %s and %s both give %s", u, other, code)
		}
		seen[code] = u
		if back := UnitOfMeasureOfCode(code); back != u {
			t.Errorf("unit %s: code %s gives %s", u, code, back)
		}
	}
	for _, u := range []string{"05", "14", "27", "29", "99"} {
		if code := UnitCodeOfUnitOfMeasure(u); code != "" {
			t.Errorf("UnitCodeOfUnitOfMeasure(%s) = %s", u, code)
		}
	}
	if u := UnitOfMeasureOfCode("H87"); u != "" {
		t.Errorf("UnitOfMeasureOfCode(H87) = %s", u)
	}
}

// TestSpanishPaymentMeans checks that each Facturae payment means read back
// from its type code is the same means, except for the means without a type
// code of their own (1).
func TestSpanishPaymentMeans(t *testing.T) {
	if len(spanishPaymentMeans) != 19 {
		t.Errorf("%d payment means, want 01 to 19", len(spanishPaymentMeans))
	}
	for m, code := range spanishPaymentMeans {
		if back := SpanishPaymentMeansOfTypeCode(code); (back == m) == (code == "1") {
			t.Errorf("%s: type code %s gives %s", m, code, back)
		}
		if TypeCodeOfSpanishPaymentMeans(m) != code {
			t.Errorf("TypeCodeOfSpanishPaymentMeans(%s) = %s", m, TypeCodeOfSpanishPaymentMeans(m))
		}
	}
	for code, want := range map[string]string{"58": "04", "42": "04", "59": "02", "54": "19", " 30 ": "04", "1": "13", "": "13"} {
		if got := SpanishPaymentMeansOfTypeCode(code); got != want {
			t.Errorf("SpanishPaymentMeansOfTypeCode(%q) = %s, want %s", code, got, want)
		}
	}
}

func TestSpanishTaxes(t *testing.T) {
	for _, c := range []struct{ category, taxType, rate, event string }{
		{"S", "01", "21.00", ""},
		{"Z", "01", "0.00", ""},
		{"E", "01", "0.00", "01"},
		{"O", "01", "0.00", "02"},
		{"L", "03", "7.00", ""},
		{"M", "02", "4.00", ""},
	} {
		if got := SpanishTaxTypeOfCategory(c.category); got != c.taxType {
			t.Errorf("SpanishTaxTypeOfCategory(%s) = %s, want %s", c.category, got, c.taxType)
		}
		if got := CategoryOfSpanishTax(c.taxType, MustDecimal(c.rate), c.event); got != c.category {
			t.Errorf("CategoryOfSpanishTax(%s, %s, %q) = %s, want %s", c.taxType, c.rate, c.event, got, c.category)
		}
		if !IsSpanishVAT(c.taxType) {
			t.Errorf("IsSpanishVAT(%s) = false", c.taxType)
		}
	}
	for category, event := range map[string]string{"E": "01", "K": "01", "G": "01", "O": "02", "AE": "02", "S": "", "Z": ""} {
		if got := SpecialEventOfCategory(category); got != event {
			t.Errorf("SpecialEventOfCategory(%s) = %q, want %q", category, got, event)
		}
	}
	if IsSpanishVAT(SpanishIRPF) || CategoryOfSpanishTax(SpanishIRPF, MustDecimal("15.00"), "") != "" {
		t.Error("IRPF taken for a VAT")
	}
}

func TestSpanishClasses(t *testing.T) {
	for code, want := range map[string]string{"380": "OO", "389": "OO", "386": "OO", "384": "OR", "471": "OR", "381": "OR", "396": "OR", " 261 ": "OR"} {
		if got := SpanishClassOfTypeCode(code); got != want {
			t.Errorf("SpanishClassOfTypeCode(%q) = %s, want %s", code, got, want)
		}
	}
	for code, want := range map[string]string{"16": "Base imponible", "02": "Serie de la factura", "99": ""} {
		if got := CorrectiveReason(code); got != want {
			t.Errorf("CorrectiveReason(%s) = %q, want %q", code, got, want)
		}
	}
	if len(correctiveReasons) != 22 || len(correctionMethods) != 4 {
		t.Errorf("%d reasons, %d methods", len(correctiveReasons), len(correctionMethods))
	}
	for _, c := range [][4]string{{"Lucía", "Martínez", "Ruiz", "Lucía Martínez Ruiz"}, {"Ana", "", "García", "Ana García"}, {"", "", "", ""}} {
		if got := SpanishName(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("SpanishName%q = %q", c[:3], got)
		}
	}
}
