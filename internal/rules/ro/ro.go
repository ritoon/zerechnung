// Package ro implements the rules of the CIUS-RO, the CIUS of EN 16931 for
// Romania, version 1.0.1 (order OMF 4092/2022 of the ministry of finance),
// as ANAF publishes them in the schematron of its validation artefacts,
// version 1.0.9 of 05/06/2024 (ro16931-ubl-1.0.9.zip, page "Informații
// tehnice" of mfinante.gov.ro). Every domestic invoice goes through the
// platform RO e-Factura of ANAF: between businesses and with public
// administrations, and to consumers since 2025. Its specification
// identifier (BT-24) is
// urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.1.
//
// # Rules
//
// The pattern ROmodel of the schematron holds the rules of the CIUS-RO,
// which the package applies in full to a UBL invoice or credit note:
//
//   - BR-RO-001: the specification identifier of the CIUS-RO 1.0.1;
//   - BR-RO-010 to BR-RO-212: the document types (380, 384, 389, 751, and
//     381 for a credit note), an invoice number with a digit, the VAT
//     accounting currency RON when the invoice is in another currency, the
//     identifiers of the seller and of the buyer, the address line and the
//     city of each party, the county code of ISO 3166-2:RO of an address in
//     Romania and the sector of an address in Bucharest (SECTOR1 to
//     SECTOR6), the value added tax point date codes 3, 35 and 432;
//   - BR-RO-DT001 to BR-RO-DT006: the dates written YYYY-MM-DD, without a
//     time zone;
//   - BR-RO-Lnnn and BR-RO-Annn: the maximum length of about seventy data
//     and the maximum number of notes, supporting documents, item
//     attributes and preceding invoices;
//   - BR-DEC-RO-*: two decimals at most in the amounts.
//
// The pattern also restates BR-27 (the item net price is not negative),
// which the artefacts of EN 16931 that ANAF bundles failed to check in some
// cases before version 1.0.9: the EN 16931 rule set checks it, and the
// package leaves it out. All the rules are errors (flag fatal).
//
// The schematron is reproduced as published, with its slips: the
// identifier that a message writes sometimes differs from the attribute id
// of its assertion (BR-RO-L155 writes [BR-RO-L200]); the package reports
// the attribute id, as the SVRL report of the validator does. Some
// assertions never fail on a document that the UBL schema accepts:
// BR-DEC-RO-13 and BR-DEC-RO-15 (their context, /ubl:Invoice |
// cac:CreditNote, names cac:CreditNote where the root cn:CreditNote was
// meant, and the root of an invoice is taken by an earlier rule),
// BR-DEC-RO-23 and BR-RO-L1019 (an earlier rule takes the same elements),
// BR-RO-A051 and BR-RO-A052 (they count the context element itself) and
// BR-RO-A500 (cac:BillingReference holds one cac:InvoiceDocumentReference
// at most); and the rules whose context starts with /ubl:CreditNote, a
// root that does not exist (the prefix ubl is the namespace of the
// invoice), do not apply to a credit note: BR-RO-140 to BR-RO-212 on its
// tax representative and delivery addresses, BR-DEC-RO-01 to BR-DEC-RO-06
// on its document level allowances and charges. The package does as the
// schematron does: ANAF validates with it.
//
// # CII
//
// The order OMF 4092/2022 admits CII D16B, but ANAF publishes no
// schematron for it: Check applies to a CII invoice the transposition of
// the rules that ciiPatterns describes, with the same identifiers.
//
// # Evaluation
//
// The rules are not rewritten one by one: rules_gen.go reproduces the
// pattern as ANAF publishes it (see gen), and package internal/schematron
// evaluates its contexts, variables and tests with an interpreter of the
// subset of XPath 2.0 they use, with the semantics of an XSLT 2.0
// processor. The transposition to CII is written in the same form.
//
// # Beyond the schematron
//
// The package adds the check of the Romanian fiscal identification code
// (CIF, or CUI) in the VAT identifiers of the seller (BT-31), of the buyer
// (BT-48) and of the seller's tax representative (BT-63) when they start
// with RO, and in the tax registration identifier of a seller in Romania
// (BT-32): two to ten digits (RO-CIF-01), the last one the check digit of
// the others (RO-CIF-02). The CIUS-RO does not require it: both are
// warnings.
//
// The transmission to ANAF (the API of the SPV), the signature of the
// ministry on the invoices it delivers, the conversion of the XML into PDF,
// RO e-Transport and the SAF-T file (D406) are outside the CIUS-RO and this
// package.
package ro

//go:generate go run ./gen -o rules_gen.go

import (
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schematron"
)

type (
	let       = schematron.Let
	assertion = schematron.Assertion
	rule      = schematron.Rule
	pattern   = schematron.Pattern
)

// compiled returns the compiled UBL schematron and its transposition to
// CII, or the first expression that does not compile.
var compiled = sync.OnceValues(func() ([2]*schematron.Schema, error) {
	ubl, err := schematron.Compile(namespaces, patterns)
	if err != nil {
		return [2]*schematron.Schema{}, err
	}
	cii, err := schematron.Compile(ciiNamespaces, ciiPatterns)
	return [2]*schematron.Schema{ubl, cii}, err
})

// national reports whether an assertion is a rule of the CIUS-RO itself
// (BR-RO-*, BR-DEC-RO-*): the pattern also restates BR-27 of EN 16931.
func national(id string) bool { return strings.Contains(id, "-RO-") }

// Check applies the rules of the CIUS-RO to the invoice of c: the
// schematron of ANAF to a UBL invoice, its transposition to a CII one,
// then the check of the CIF.
func Check(c *rules.Checker) {
	s, err := compiled()
	if err != nil {
		panic("ro: " + err.Error())
	}
	if c.Doc.UBL != nil {
		s[0].Run(c, c.Doc.UBL, national)
	} else {
		s[1].Run(c, c.Doc.Root, national)
	}
	checkCIF(c)
}

// Catalog lists the rules of the Romania rule set: those of the CIUS-RO
// schematron (BR-RO-*, BR-DEC-RO-*), in schematron order with their
// English messages, then the checks of the package.
var Catalog = append(schematronCatalog,
	rules.RuleInfo{ID: RuleCIFFormat, Severity: rules.Warning, Description: "A Romanian VAT identifier (BT-31, BT-48, BT-63) shall be RO followed by the 2 to 10 digits of a CIF, and the tax registration identifier of a seller in Romania (BT-32) such a CIF, with or without RO"},
	rules.RuleInfo{ID: RuleCIFKey, Severity: rules.Warning, Description: "The last digit of a CIF shall be its check digit (key 753217532, modulo 11)"},
)
