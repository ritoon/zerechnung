package main

import (
	"strings"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// The CIUS-RO comparison: the ROmodel phase of the schematron of ANAF
// (FX_RO_SCH), the pattern of the Romanian rules, compiled into an XSLT by
// SchXslt 1.10.1 (parity.sh writes it to the work directory as roXSL) and run
// by Saxon-HE 12.10 (CiusPtBatch.java), against package internal/rules/ro.
// As for the CIUS-PT, the rules are compared at every level and on the
// element where they are raised (RULE@PATH, see ptPath). The schematron
// binds the rules to UBL only: the transposition to CII of the package has no
// reference.

// roXSL is the name of the XSLT compiled from the ROmodel phase of the
// CIUS-RO schematron, in the work directory.
const roXSL = "cius-ro.xsl"

// roNational reports whether an assertion of the ROmodel pattern is a rule of
// the CIUS-RO (BR-RO-*, BR-DEC-RO-*): the pattern also restates BR-27 of
// EN 16931, which the rule set EN16931 of the package checks. A
// transformation stopped by the test of BR-27 compares nothing.
func roNational(id string) bool { return strings.Contains(id, "-RO-") }

// oursRO validates an invoice of the CIUS-RO comparison with this package as
// an invoice of the CIUS-RO, whatever its BT-24 (a variant may change or
// remove it, which BR-RO-001 reports), and keeps the findings of the rules of
// the schematron: those of the Romania rule set but the checks of the CIF of
// the package (RO-CIF), which the schematron does not have. The findings are
// keyed RULE@PATH, see ptPath.
func oursRO(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, &validate.Options{Profile: invoice.ProfileCIUSRO, Rules: validate.Romania})
	for _, f := range rep.Findings {
		if f.Source == validate.SourceRO && roNational(f.Rule) {
			res.add(f.Rule+"@"+ptPath(f.Path), f.Severity.String())
		} else {
			res.other(f.Rule)
		}
	}
	return res
}
