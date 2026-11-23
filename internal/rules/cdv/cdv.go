// Package cdv implements the rules BR-FR-CDV of the statuses of the life
// cycle of the French invoices (cycle de vie, CDV), as the FNFE-MPE
// publishes them in the schematron BR-FR-CDV-Schematron-CDAR version
// 1.4.0.04 of the France_RFE package (fatal on reception by 1 October 2026
// at the latest, and on emission from that date). A status is a UN/CEFACT
// Cross Domain Acknowledgement and Response (CDAR) D22B message, which the
// approved platforms exchange and send to the public invoicing portal
// (PPF): the note of the FNFE-MPE checks it in two steps, the CDAR D22B
// schema (package internal/schema) then this schematron.
//
// # Rules
//
// The schematron has 28 patterns and 43 assertions, which the package
// applies in full to a CDAR message:
//
//   - BR-FR-04_MDT-91: the type of the referenced document (MDT-91) is one
//     of the French invoice types, as BR-FR-04 requires of BT-3;
//   - BR-FR-CDV-01 to BR-FR-CDV-16: the business process and the profile of
//     the message (MDT-2, MDT-3: urn.cpro.gouv.fr:1p0:CDV:invoice, or
//     urn.cpro.gouv.fr:1p0:CDV:einvoicingF2 for a status sent to the PPF
//     alone), its identifier and dates, its sender, issuer and recipients
//     and their electronic addresses, the type of acknowledgement (MDT-77:
//     23 for the processing phase, 305 for the transmission phase), the
//     referenced invoice, its date and its seller, the status code
//     (MDT-105), the collected amounts and their VAT rates that the status
//     212 Encaissée requires (characteristics MEN), the reason that the
//     statuses 206, 207, 208, 210, 213 and 501 require, and the sequence
//     number of each status detail;
//   - BR-FR-CDV-CL-01 to BR-FR-CDV-CL-11: the closed lists of the business
//     processes, of the roles of the parties by phase, of the status codes
//     (200 to 228, 250, 251, 300, 301, 400, 401, 500, 501, 601), of the
//     reasons, in general and by status (200, 206, 207, 208, 210 and its B2G
//     variant, 213, 221, 501), of the requested actions and of the types of
//     the characteristics.
//
// BR-FR-CDV-13_MDT-129-2 (no SIREN of the seller) and the two assertions of
// BR-FR-CDV-CL-06 (status codes outside the list, which a bilateral
// agreement may add) are warnings; the other assertions are errors.
//
// The schematron is reproduced as published, with its slips: several
// presence rules (BR-FR-CDV-01, 04, 05, 06 and 10) test the element they
// take as context, which exists, and cannot fail (the schema requires some
// of these elements, not all); BR-FR-CDV-07_MDT-38_no repeats the context
// of BR-FR-CDV-07_MDT-38_yes in the same pattern and is never evaluated, as
// an element is checked by the first rule of a pattern that matches it. The
// messages are the French ones of the schematron.
//
// The warning version of the schematron (BR-FR-CDV-Schematron-CDAR_WARNING,
// every assertion a warning) applied on reception only, until 30 September
// 2026: the package implements the fatal version alone.
//
// # Evaluation
//
// The rules are not rewritten one by one: rules_gen.go reproduces the
// schematron as the FNFE-MPE publishes it (see gen), and package
// internal/schematron evaluates its contexts, variables, functions and
// tests with an interpreter of the subset of XPath 2.0 they use, with the
// semantics of an XSLT 2.0 processor.
//
// The transmission of the statuses between the platforms and to the PPF,
// and the processing that depends on the business process of the invoice,
// are outside the schematron and this package.
package cdv

//go:generate go run ./gen -o rules_gen.go

import (
	"slices"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schematron"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

type (
	let       = schematron.Let
	assertion = schematron.Assertion
	rule      = schematron.Rule
	pattern   = schematron.Pattern
)

// compiled returns the compiled schematron, or the first expression that
// does not compile.
var compiled = sync.OnceValues(func() (*schematron.Schema, error) {
	return schematron.Compile(namespaces, patterns, functions...)
})

func all(string) bool { return true }

// Check applies the rules of the schematron BR-FR-CDV to root, the root
// element of a CDAR message, and records the findings with c. c.Doc may be
// nil.
func Check(c *rules.Checker, root *xmltree.Node) {
	if root == nil {
		return
	}
	s, err := compiled()
	if err != nil {
		panic("cdv: " + err.Error())
	}
	s.Run(c, root, all)
}

// Catalog lists the rules of the schematron BR-FR-CDV, in schematron order,
// with their messages. The list is built on first use; the caller must not
// modify it.
func Catalog() []rules.RuleInfo { return catalog() }

var catalog = sync.OnceValue(func() []rules.RuleInfo {
	return slices.Clip(schematronCatalog)
})
