// Package pt implements the rules of the CIUS-PT, the CIUS of EN 16931 for
// Portugal, as eSPap (Entidade de Serviços Partilhados da Administração
// Pública) publishes them in its schematron, version 2.1.1 of 26/02/2021
// (urn_feap.gov.pt_CIUS-PT_2.1.1.sch, page "Normas" of www.espap.gov.pt).
// The CIUS-PT is defined for the UBL 2.1 syntax only; its specification
// identifier (BT-24) is
// urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1.
//
// # Rules
//
// The schematron has four patterns, which the package applies in full:
//
//   - UBL-syntax: the UBL syntax rules (UBL-CR, UBL-SR, UBL-DT), most of
//     them warnings, in the versions of the CIUS-PT;
//   - UBL-model and UBL-condition: the rules of EN 16931 the CIUS-PT keeps
//     (BR-*, BR-CO-*, BR-S-*, BR-E-*, BR-AA-*), in the versions of its base
//     artefact, and its own rules (BR-CIUS-PT-*);
//   - UBL-datatype: the formats and code lists of the CIUS-PT
//     (DT-CIUS-PT-*), which accept the Portuguese document types (FT, FS,
//     FR, ND, NC…) and VAT categories (NOR, INT, RED, ISE, OUT…) that the
//     code lists of EN 16931 refuse, and the tolerance of ±1.00 the CIUS-PT
//     grants to the totals (DT-CIUS-PT-157 to DT-CIUS-PT-177).
//
// Check reports the failed assertions of the CIUS-PT itself, listed in
// Catalog, in the Portugal rule set (source CIUS-PT). CheckCEN reports the
// others, listed in CENCatalog, in the EN 16931 rule set: for a CIUS-PT
// invoice, they replace the rules of packages en16931 and ubl, as the
// validator of eSPap applies this schematron and not the artefacts of
// CEN/TC 434. The flags map to the severities of package rules: fatal to
// Error, warning to Warning.
//
// # Evaluation
//
// The rules are not rewritten one by one: rules_gen.go reproduces the
// schematron as eSPap publishes it (the abstract patterns instantiated with
// the parameters of their UBL patterns, see gen), and package
// internal/schematron evaluates its contexts, variables and tests with an
// interpreter of the subset of XPath 2.0 they use, with the semantics of an
// XSLT 2.0 processor.
//
// # Beyond the schematron
//
// The package adds the check of the Portuguese VAT identifier (NIF) of the
// seller (BT-31), of the buyer (BT-48) and of the seller's tax
// representative (BT-63) when it starts with PT: nine digits (PT-NIF-01),
// the last one the check digit modulo 11 (PT-NIF-02). The CIUS-PT does not
// require it: both are warnings. An invoice in the CII syntax that declares
// the CIUS-PT gets PT-SYNTAX-01, as the CIUS-PT has no CII binding.
//
// The CIUS-PT carries no ATCUD, no hash and no certificate of the invoicing
// software in the XML: the hash and the number of the software certified by
// the Autoridade Tributária appear only as optional structured notes
// (#HASHCODE@ATCERTIFIEDPROGRAM#, #NUMBER@ATCERTIFIEDPROGRAM#), whose format
// the schematron checks. The ATCUD and the QR code of the printed invoice,
// the SAF-T (PT) file and the communication to the e-fatura portal are
// outside the CIUS-PT and this package.
package pt

//go:generate go run ./gen -o rules_gen.go

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/invoice"
)

// Applies reports whether the rules of the CIUS-PT replace those of EN
// 16931 for doc: a UBL invoice whose profile is the CIUS-PT.
func Applies(doc *rules.Doc) bool {
	return doc != nil && doc.UBL != nil && doc.Profile == invoice.ProfileCIUSPT
}

// Check applies the rules of the CIUS-PT itself (BR-CIUS-PT, DT-CIUS-PT)
// and the check of the NIF to the UBL invoice of c. A CII invoice gets
// PT-SYNTAX-01.
func Check(c *rules.Checker) {
	if c.Doc.UBL == nil {
		c.Fail("PT-SYNTAX-01", rules.Error, c.Doc.Root,
			"the CIUS-PT is defined for the UBL 2.1 syntax only: a CIUS-PT invoice cannot be a CII invoice")
		return
	}
	run(c, c.Doc.UBL, false)
	checkNIF(c, c.Doc.UBL)
}

// CheckCEN applies the other rules of the CIUS-PT schematron (UBL syntax
// rules and rules of EN 16931, see CENCatalog) to the UBL invoice of c.
func CheckCEN(c *rules.Checker) {
	if c.Doc.UBL == nil {
		return
	}
	run(c, c.Doc.UBL, true)
}

// Catalog lists the rules of the Portugal rule set: those of the CIUS-PT
// schematron (BR-CIUS-PT, DT-CIUS-PT), in schematron order with their
// official messages, then the checks of the package.
var Catalog = append(schematronCatalog,
	rules.RuleInfo{ID: "PT-SYNTAX-01", Severity: rules.Error, Description: "A CIUS-PT invoice shall use the UBL 2.1 syntax: the CIUS-PT has no CII binding"},
	rules.RuleInfo{ID: "PT-NIF-01", Severity: rules.Warning, Description: "A Portuguese VAT identifier (BT-31, BT-48, BT-63) shall be PT followed by the nine digits of a NIF"},
	rules.RuleInfo{ID: "PT-NIF-02", Severity: rules.Warning, Description: "The last digit of a NIF shall be its check digit modulo 11"},
)
