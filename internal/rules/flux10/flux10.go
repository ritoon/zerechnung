// Package flux10 implements the rules of the public invoicing portal (PPF)
// on the reports of the French e-reporting (flux 10, see package
// ereporting), as the FNFE-MPE publishes them in the schematron Flux10 of
// the France_RFE package, version 1.4.0.04 (FNFE_RFE_INVOICE/Flux10/
// Flux10.sch, "PPF — Flux 10 e-reporting — v1.8"): the management rules of
// annex 7 v1.8 of the external specifications of the DGFiP that a report
// can be checked against by itself. A report is checked in two steps, the
// XML schemas of the flux 10 (package internal/schema) then this
// schematron.
//
// # Rules
//
// The schematron has 96 patterns and 150 assertions, all fatal, which the
// package applies in full to a report, as errors:
//
//   - the transmission (ReportDocument): its identifier (G1.104), its date
//     and time (G7.53, G1.36), its type IN or RE (G8.01), a single report of
//     transactions or of payments (G6.29), the sender, an approved platform
//     (G6.22, G7.51), and the declarant, by its SIREN (G6.26, G7.52);
//   - the transactions report (flux 10.1 and 10.3): the reporting periods
//     (G1.09, G1.36, G6.25), and for each invoice its type and business
//     process (G1.01, G1.02, G1.60, S1.12), its identifiers and dates
//     (G1.05, G1.09, G1.36), the date of exigibility of the VAT (P1.11),
//     the preceding invoices that a corrective invoice or a credit note
//     references (G1.32), the identifiers of the seller and of the buyer by
//     scheme (G2.19, G2.33, G6.28, G1.102), their countries (G2.01), the
//     VAT categories, rates and exemptions (G2.31, G1.24, G1.40, G6.21), the
//     amounts and their totals (G1.14, G1.15, G1.16, G1.53, G1.55, G6.23)
//     and the currencies (G1.10); for the aggregated transactions, their
//     date, category (G1.68), exigibility, rates, amounts and totals;
//   - the payments report (flux 10.2 and 10.4): the period, and for each
//     invoice or aggregated transaction the dates of the payments, the VAT
//     rates, the amounts and their currency, the euro (G1.09, G1.36, G1.24,
//     G1.16, G6.27, G1.10).
//
// # Identifiers
//
// The assertions of the schematron have no identifier: each message opens
// with the code of annex 7 that it applies ("[G1.104] "), and a code recurs
// within a pattern and across patterns. A rule takes the identifier of its
// pattern ("F10-TRANSMISSION-G1.104"), the second and following assertions
// of a pattern with a suffix -2, -3… in schematron order
// ("F10-TRANSMISSION-G1.104-2" to "-4"): a search of the schematron for the
// identifier, without its suffix, finds the pattern. The messages are the
// French ones of the schematron, with their code and their source in annex
// 7.
//
// The schematron is reproduced as published. Two of its choices stand out:
// G6.28 requires the identifier of the buyer on every invoice of a
// transactions report, and G1.53 compares the totals with the sum of the
// VAT breakdowns in xs:double arithmetic, as an XSLT processor does, with a
// tolerance of 0.01 per amount added; the totals of an invoice or of
// aggregated transactions in another currency than the euro are not
// compared.
//
// # Evaluation
//
// The rules are not rewritten one by one: rules_gen.go reproduces the
// schematron as the FNFE-MPE publishes it (see gen), and package
// internal/schematron evaluates its contexts, variables and tests with an
// interpreter of the subset of XPath 2.0 they use, with the semantics of an
// XSLT 2.0 processor.
//
// The rules that need the directories of the PPF (the existence of a SIREN,
// of a country or currency code, of the matricule of the platform) and the
// correspondence of a report with the invoices it declares (the schematrons
// BR-FR-MAP, which the FNFE-MPE has not published) are outside the
// schematron and this package.
package flux10

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
	return schematron.Compile(nil, patterns)
})

func all(string) bool { return true }

// Check applies the rules of the schematron Flux10 to root, the root element
// of an e-reporting report, and records the findings with c. c.Doc may be
// nil.
func Check(c *rules.Checker, root *xmltree.Node) {
	if root == nil {
		return
	}
	s, err := compiled()
	if err != nil {
		panic("flux10: " + err.Error())
	}
	s.Run(c, root, all)
}

// Catalog lists the rules of the schematron Flux10, in schematron order,
// with their messages. The list is built on first use; the caller must not
// modify it.
func Catalog() []rules.RuleInfo { return catalog() }

var catalog = sync.OnceValue(func() []rules.RuleInfo {
	return slices.Clip(schematronCatalog)
})
