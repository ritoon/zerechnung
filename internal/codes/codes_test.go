package codes

import (
	"slices"
	"strings"
	"testing"
)

func TestLookups(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) bool
		ok   []string
		ko   []string
	}{
		{"DocumentType", DocumentType, []string{"380", "381", "384", "389", "751", "71"}, []string{"", "999", " 380", "380 "}},
		{"Currency", Currency, []string{"EUR", "USD", "CHF", "XXX"}, []string{"eur", "EURO", "", "ZZZ"}},
		{"TaxPointDateCode", TaxPointDateCode, []string{"5", "29", "72"}, []string{"3", "35", ""}},
		{"ReferenceType", ReferenceType, []string{"AAA", "ON", "ZZZ"}, []string{"aaa", "A"}},
		{"TextSubject", TextSubject, []string{"AAI", "REG", "ABL"}, []string{"XYZ1"}},
		{"ICD", ICD, []string{"0002", "0088", "0060", "0204"}, []string{"88", "9958", "EM"}},
		{"ItemClassification", ItemClassification, []string{"AA", "STI", "TST"}, []string{"ZZZZ"}},
		{"Country", Country, []string{"DE", "FR", "1A", "XI"}, []string{"de", "EL", "UK", "D"}},
		{"PaymentMeans", PaymentMeans, []string{"1", "30", "58", "59", "ZZZ"}, []string{"0", "999"}},
		{"VATCategory", VATCategory, []string{"S", "Z", "E", "AE", "K", "G", "O", "L", "M", "B"}, []string{"s", "X", "VAT"}},
		{"AllowanceReason", AllowanceReason, []string{"41", "95", "100"}, []string{"1", "ZZZ"}},
		{"ChargeReason", ChargeReason, []string{"AA", "ABL", "ZZZ"}, []string{"aa", "12345"}},
		{"VATEX", VATEX, []string{"VATEX-EU-AE", "vatex-eu-ae", "VATEX-EU-132-1A", "VATEX-ES-IVA23/24"}, []string{"VATEX-EU", "AE", ""}},
		{"Unit", Unit, []string{"C62", "H87", "XPP", "KGM", "LTR", "HUR", "DAY"}, []string{"c62", "PCS", ""}},
		{"MIME", MIME, []string{"application/pdf", "image/png", "image/jpeg", "text/csv"}, []string{"application/xml", "image/gif", "APPLICATION/PDF"}},
		{"EAS", EAS, []string{"0088", "9930", "EM", "0204"}, []string{"9958", "88", "em"}},
		{"VATPrefix", VATPrefix, []string{"DE", "FR", "XI", "1A"}, []string{"EL", "de", "D"}},
		{"RomanianCounty", RomanianCounty, []string{"RO-AB", "RO-B", "RO-BC", "RO-CJ", "RO-IF", "RO-VS"}, []string{"RO-XX", "ro-cj", "CJ", "RO-CJ ", ""}},
		{"BucharestSector", BucharestSector, []string{"SECTOR1", "SECTOR6"}, []string{"SECTOR7", "Sector1", "SECTOR 1", ""}},
	}
	for _, tt := range tests {
		for _, s := range tt.ok {
			if !tt.fn(s) {
				t.Errorf("%s(%q) = false, want true", tt.name, s)
			}
		}
		for _, s := range tt.ko {
			if tt.fn(s) {
				t.Errorf("%s(%q) = true, want false", tt.name, s)
			}
		}
	}
}

func TestListsSorted(t *testing.T) {
	all := map[string][]string{
		"documentTypes": documentTypes, "currencies": currencies, "taxPointDateCodes": taxPointDateCodes,
		"referenceTypes": referenceTypes, "textSubjects": textSubjects, "icd": icd,
		"itemClassifications": itemClassifications, "countries": countries, "paymentMeans": paymentMeans,
		"vatCategories": vatCategories, "allowanceReasons": allowanceReasons, "chargeReasons": chargeReasons,
		"vatex": vatex, "units": units, "mimeCodes": mimeCodes, "eas": eas, "vatPrefixes": vatPrefixes,
		"RomanianCounties": RomanianCounties, "BucharestSectors": BucharestSectors,
	}
	for name, l := range all {
		if len(l) == 0 {
			t.Errorf("%s is empty", name)
		}
		if !slices.IsSorted(l) {
			t.Errorf("%s is not sorted", name)
		}
		for i := 1; i < len(l); i++ {
			if l[i] == l[i-1] {
				t.Errorf("%s: duplicate %q", name, l[i])
			}
		}
		for _, c := range l {
			if !in(l, c) {
				t.Errorf("%s: %q not found", name, c)
			}
		}
	}
	for _, c := range vatex {
		if strings.ToUpper(c) != c {
			t.Errorf("vatex code %q is not upper case", c)
		}
		if !VATEX(strings.ToLower(c)) {
			t.Errorf("VATEX(%q) = false", strings.ToLower(c))
		}
	}
	if got := len(units); got < 2000 {
		t.Errorf("%d unit codes, want more than 2000", got)
	}
}

func TestNoAlloc(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		Unit("C62")
		VATEX("vatex-eu-ae")
		Country("DE")
	})
	if n != 0 {
		t.Errorf("lookups allocate %v times", n)
	}
}

func BenchmarkUnit(b *testing.B) {
	for b.Loop() {
		Unit("XPP")
	}
}
