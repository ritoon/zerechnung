package zerechnung_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/validate"
)

// negativeCase is an invoice that the ZUGFeRD corpus gives as invalid.
type negativeCase struct {
	rules []string // rules that the package must raise on it
	ref   string   // what the reference validators raise
	valid string   // when set, the package raises no error: why
}

// fnfeInfo is why five FNFE-MPE examples raise no error. The corpus moved
// them from ZUGFeRDv2/correct to ZUGFeRDv2/fail without saying what is wrong
// (commit cfa47d6, "updated validation status fnfe examples"). Besides
// PDF-35, which the examples of the series left in ZUGFeRDv2/correct raise
// too, the four EN 16931 ones raise the warning PDF-14 (/AFRelationship
// Data).
const fnfeInfo = "Mustang raises no error either; the /Info entries differ from the XMP (PDF-35), a warning since PDF/A-2 and PDF/A-3 do not require them to match"

// negativeCases are the invoices of ZUGFeRDv2/fail and the not_validating
// samples of XML-Rechnung, with the verdicts of KoSIT 1.6.3 and Mustang 2.26.0
// measured by the parity job (conformance/README.md). FX-SCH-A-* are the
// assertions of the Factur-X 1.09.2 schematrons that Mustang applies, and
// that the package applies to the XML of a PDF too; SCHEMA stands for an
// error of the XML schema.
var negativeCases = map[string]negativeCase{
	"XML-Rechnung/CII/not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.cii.xml": {
		ref: "KoSIT et Mustang : aucune erreur", valid: "no reference validator raises an error either",
	},
	"XML-Rechnung/UBL/not_validating_full_invoice_based_onTest_EeISI_300_CENfullmodel.ubl.xml": {
		rules: []string{"BR-CO-10", "BR-CO-15"}, ref: "Mustang : BR-CO-10, BR-CO-15",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type380_BASIC.pdf": {
		rules: []string{"FX-PROFILE-03"}, ref: "Mustang : FX-SCH-A-000493 (BT-24)",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type380_EN16931.pdf": {
		rules: []string{"PDF-35"}, ref: "Mustang : aucune erreur", valid: fnfeInfo,
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type380_MINIMUM.pdf": {
		rules: []string{"XSD-UNEXPECTED"}, ref: "Mustang : SCHEMA",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type381_BASICWL.pdf": {
		rules: []string{"PDF-35"}, ref: "Mustang : aucune erreur", valid: fnfeInfo,
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type381_EN16931.pdf": {
		rules: []string{"PDF-35"}, ref: "Mustang : aucune erreur", valid: fnfeInfo,
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Avoir_FR_type381_MINIMUM.pdf": {
		rules: []string{"XSD-UNEXPECTED"}, ref: "Mustang : SCHEMA",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_DOM_BASIC.pdf": {
		rules: []string{"FX-PROFILE-03"}, ref: "Mustang : FX-SCH-A-000493 (BT-24)",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_DOM_EN16931.pdf": {
		rules: []string{"PDF-35"}, ref: "Mustang : aucune erreur", valid: fnfeInfo,
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_FR_BASIC.pdf": {
		rules: []string{"FX-PROFILE-03"}, ref: "Mustang : FX-SCH-A-000493 (BT-24)",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_FR_EN16931.pdf": {
		rules: []string{"PDF-35"}, ref: "Mustang : aucune erreur", valid: fnfeInfo,
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_UE_BASIC.pdf": {
		rules: []string{"BR-IC-11", "BR-IC-12", "FX-PROFILE-03"}, ref: "Mustang : BR-IC-11, BR-IC-12, FX-SCH-A-000493 (BT-24)",
	},
	"ZUGFeRDv2/fail/FNFE-factur-x-examples/Facture_UE_EN16931.pdf": {
		rules: []string{"BR-IC-11"}, ref: "Mustang : BR-IC-11",
	},
	"ZUGFeRDv2/fail/FX-With-UBL-REC50304330.pdf": {
		rules: []string{"CII"}, ref: "Mustang : UBL interdit dans un PDF Factur-X (message de type 17)",
	},
	"ZUGFeRDv2/fail/MustangRE-20171118_506_ZUGFeRD1and2.pdf": {
		rules: []string{"FX-PROFILE-02", "PDF-28", "XSD-UNEXPECTED"}, ref: "Mustang : FX-SCH-A-000636 (BT-24), SCHEMA",
	},
	"ZUGFeRDv2/fail/Mustangproject/MustangGnuaccountingBeispielRE-20171118_506.pdf": {
		rules: []string{"FX-PROFILE-02", "XSD-UNEXPECTED"}, ref: "Mustang : FX-SCH-A-000636 (BT-24), SCHEMA",
	},
	"ZUGFeRDv2/fail/Mustangproject/MustangGnuaccountingBeispielRE-20190610_507a.pdf": {
		rules: []string{"XSD-NAMESPACE", "XSD-UNEXPECTED"}, ref: "Mustang : SCHEMA",
	},
	"ZUGFeRDv2/fail/Mustangproject/MustangGnuaccountingBeispielRE-20190610_507b.pdf": {
		rules: []string{"FX-SCH-A-000581", "XSD-NAMESPACE", "XSD-UNEXPECTED"}, ref: "Mustang : FX-SCH-A-000581 (schéma de BT-30), SCHEMA",
	},
	"ZUGFeRDv2/fail/Mustangproject/factur-x-invalid-xml-encoding-attribute.pdf": {
		rules: []string{"XML"}, ref: "Mustang : SCHEMA",
	},
	"ZUGFeRDv2/fail/Mustangproject/noNetPriceValidation.xml": {
		ref:   "Mustang : avertissement arithmétique (type 10), facture valide",
		valid: "the net prices do not match the line totals, which no EN 16931 rule checks; only Mustang's own arithmetic check (a warning) sees it",
	},
	"ZUGFeRDv2/fail/Mustangproject/wrongFilename.pdf": {
		rules: []string{"PDF-10"}, ref: "Mustang : DocumentFileName des métadonnées XMP, XML non extrait",
	},
	"ZUGFeRDv2/fail/XRechnung 722825203.XML": {
		rules: []string{"BR-DE-18"}, ref: "KoSIT : BR-DE-18 ; Mustang : BR-DE-18, SCHEMA (XSD Factur-X EN 16931, quand KoSIT et le package appliquent le XSD CII D16B)",
	},
	"ZUGFeRDv2/fail/ZUGFeRD_2.0_fully_compliant_complete.pdf": {
		rules: []string{"BR-S-02", "FX-SCH-A-000612", "FX-SCH-A-000613"}, ref: "Mustang : BR-S-02, FX-SCH-A-000612 et 000613 (unités)",
	},
	"ZUGFeRDv2/fail/ZUGFeRD_2_fully_compliant_complete.pdf": {
		rules: []string{"PDF-72"}, ref: "Mustang : PDF/A (veraPDF, ISO 19005-3 6.2.11.4.2, CIDSet incomplet), XML valide",
	},
	"ZUGFeRDv2/fail/python-factur-x/python-factur-x.pdf": {
		rules: []string{"FX-PROFILE-03", "PDF-28", "XSD-UNEXPECTED"}, ref: "Mustang : FX-SCH-A-000493 (BT-24), SCHEMA",
	},
}

// TestOfficialNegativeCases checks the invoices of negativeCases: each must
// raise its rules and hold an error, unless the defect is outside what the
// package checks. Every invoice of ZUGFeRDv2/fail must be listed.
func TestOfficialNegativeCases(t *testing.T) {
	root := os.Getenv("FX_CORPUS")
	if root == "" {
		t.Skip("FX_CORPUS not set")
	}
	invoice := func(p string) bool {
		ext := strings.ToLower(filepath.Ext(p))
		return ext == ".pdf" || ext == ".xml"
	}
	err := filepath.WalkDir(filepath.Join(root, "ZUGFeRDv2", "fail"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !invoice(p) {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if _, ok := negativeCases[filepath.ToSlash(rel)]; !ok {
			t.Errorf("%s: invalid invoice of the corpus missing from negativeCases", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(negativeCases))
	for name := range negativeCases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := negativeCases[name]
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Error(err)
			continue
		}
		var rep *validate.Report
		if strings.EqualFold(filepath.Ext(name), ".pdf") {
			if rep, _, err = zerechnung.Check(data, nil); err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
		} else {
			rep = validate.XML(data, nil)
		}
		raised := map[string]bool{}
		var errs []string
		for _, f := range rep.Findings {
			raised[f.Rule] = true
			if f.Severity.String() == "error" {
				errs = append(errs, f.Rule)
			}
		}
		for _, r := range c.rules {
			if !raised[r] {
				t.Errorf("%s: %s not raised (reference: %s)", name, r, c.ref)
			}
		}
		switch {
		case c.valid == "" && len(errs) == 0:
			t.Errorf("%s: no error (reference: %s)", name, c.ref)
		case c.valid != "" && len(errs) > 0:
			t.Errorf("%s: errors %v, expected none: %s", name, errs, c.valid)
		case c.valid != "":
			t.Logf("%s: no error: %s", name, c.valid)
		}
	}
}
