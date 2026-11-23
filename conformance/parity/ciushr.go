package main

import (
	"strings"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// The CIUS-HR comparison: the schematron of the Croatian CIUS and its
// extension (CIUS-HR and EXT-HR 2025) that the Croatian tax administration
// (Porezna uprava) publishes for Fiskalizacija 2.0 (FX_HR_SCH), compiled into
// an XSLT by SchXslt 1.10.1 (parity.sh writes it to the work directory as
// hrXSL) and run by Saxon-HE 12.10 (CiusPtBatch.java), against package
// internal/rules/hr. As for the CIUS-PT and the CIUS-RO, the rules are
// compared at every level and on the element where they are raised
// (RULE@PATH, see ptPath). The schematron holds the Croatian rules only
// (HR-BR-*), every one compared, and binds them to UBL only.

// hrXSL is the name of the XSLT compiled from the CIUS-HR schematron, in the
// work directory.
const hrXSL = "cius-hr.xsl"

// hrSchematron reports whether a rule of the Croatia rule set is one of the
// schematron (HR-BR-*), and not a check of the package: HR-OIB-01, the OIB of
// an electronic address of scheme 9934, and HR-SYNTAX-01, a CII invoice.
func hrSchematron(id string) bool { return strings.HasPrefix(id, "HR-BR-") }

// oursHR validates an invoice of the CIUS-HR comparison with this package as
// an invoice of the CIUS-HR, whatever its BT-24 (a variant may change or
// remove it, which HR-BR-5 reports), and keeps the findings of the rules of
// the schematron: those of the Croatia rule set but the checks of the
// package (see hrSchematron). The findings are keyed RULE@PATH, see ptPath.
func oursHR(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, &validate.Options{Profile: invoice.ProfileCIUSHR, Rules: validate.Croatia})
	for _, f := range rep.Findings {
		if f.Source == validate.SourceHR && hrSchematron(f.Rule) {
			res.add(f.Rule+"@"+ptPath(f.Path), f.Severity.String())
		} else {
			res.other(f.Rule)
		}
	}
	return res
}
