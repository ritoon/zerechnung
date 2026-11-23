package codes

import (
	"slices"
	"strings"
)

// The CIUS-HR (Croatia) gives the business process type (BT-23) from table
// HR-TB-1 of its specification (Specifikacija osnovne uporabe eRačuna s
// proširenjima, version 1.8), and an HR VAT category label (HR-BT-6,
// HR-BT-12, HR-BT-22) from table HR-TB-2, which binds each label to a VAT
// category code of EN 16931 (UNTDID 5305, BT-118), to an HR VAT category code
// of the extension (HR-BT-18) and to a tax scheme (UNTDID 5153). The
// schematron of the CIUS-HR 1.0.0 lists the labels in HR-BR-CL-1 and the
// processes in HR-BR-34; package internal/rules/hr checks them against
// these tables.

// CroatianProcesses are the business process types P1 to P12 of table
// HR-TB-1, sorted. P99: followed by a label of the buyer is a process
// defined by the buyer.
var CroatianProcesses = []string{"P1", "P10", "P11", "P12", "P2", "P3", "P4", "P5", "P6", "P7", "P8", "P9"}

// CroatianProcess reports whether s is a business process type of table
// HR-TB-1: P1 to P12, or P99: followed by a label of the buyer (HR-BR-34).
func CroatianProcess(s string) bool {
	return in(CroatianProcesses, s) || strings.HasPrefix(s, "P99:")
}

// CroatianVATLabel is a row of table HR-TB-2.
type CroatianVATLabel struct {
	Label      string // HR VAT category label (HR-BT-6, HR-BT-12, HR-BT-22)
	Category   string // VAT category code of EN 16931 (UNTDID 5305, BT-118)
	HRCategory string // HR VAT category code of the extension (HR-BT-18)
	Scheme     string // tax scheme (UNTDID 5153)
	Name       string // meaning of the label, in English
}

// CroatianVATLabels are the rows of table HR-TB-2, sorted by label. The
// three pass-through items (HR:POVNAK, HR:PP, HR:PPMV) and HR:N are exempt
// in EN 16931 and out of the scope of VAT in the HR VAT breakdown.
var CroatianVATLabels = []CroatianVATLabel{
	{"HR:AE", "AE", "AE", "VAT", "Reverse charge: the buyer pays the VAT"},
	{"HR:E", "E", "E", "VAT", "Exempt from VAT under the article given in the exemption reason"},
	{"HR:G", "G", "G", "VAT", "Export, exempt from VAT"},
	{"HR:K", "K", "K", "VAT", "Intra-community supply, exempt from VAT"},
	{"HR:N", "E", "O", "VAT", "Item not subject to VAT"},
	{"HR:O", "O", "O", "VAT", "Not subject to VAT: the whole invoice is outside VAT (article 5 of the VAT law)"},
	{"HR:PDV13", "S", "S", "VAT", "Reduced VAT rate of 13 %"},
	{"HR:PDV25", "S", "S", "VAT", "Standard VAT rate of 25 %"},
	{"HR:PDV5", "S", "S", "VAT", "Reduced VAT rate of 5 %"},
	{"HR:POVNAK", "E", "O", "OTH", "Deposit on single-use packaging, a pass-through item outside the taxable amount (article 33(3) of the VAT law)"},
	{"HR:PP", "E", "O", "LOC", "Special tax on consumption in hospitality, a pass-through item (article 33(3) of the VAT law)"},
	{"HR:PPMV", "E", "O", "CAR", "Special tax on motor vehicles, a pass-through item (article 33(3) of the VAT law)"},
	{"HR:Z", "Z", "Z", "VAT", "Zero rate of VAT"},
}

// CroatianVAT returns the row of table HR-TB-2 of the label s (HR-BR-CL-1).
func CroatianVAT(s string) (CroatianVATLabel, bool) {
	i, ok := slices.BinarySearchFunc(CroatianVATLabels, s, func(r CroatianVATLabel, s string) int {
		return strings.Compare(r.Label, s)
	})
	if !ok {
		return CroatianVATLabel{}, false
	}
	return CroatianVATLabels[i], true
}
