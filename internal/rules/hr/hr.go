// Package hr implements the rules of the Croatian CIUS and of its extension
// (CIUS-HR and EXT-HR 2025), which the law on fiscalisation (Zakon o
// fiskalizaciji, Fiskalizacija 2.0) requires of the invoices between
// Croatian taxable persons from 1 January 2026, as the Croatian tax
// administration (Porezna uprava) publishes them in the schematron of the
// specification of the core use of the e-invoice with extensions
// (Specifikacija osnovne uporabe eRačuna s proširenjima, version 1.8):
// schematron 1.0.0 of 13/03/2026 (HRUBLSchematron_13032026-2.zip, page
// eRačun of porezna.gov.hr). The CIUS-HR is defined for the UBL 2.1 syntax
// only; its specification identifier (BT-24) is
// urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0.
//
// # Rules
//
// The schematron has three patterns, which the package applies in full to a
// UBL invoice or credit note:
//
//   - HR-BR-33: no empty element, but in the signature of the invoice;
//   - HR-BR-1 to HR-BR-56: the issue date from 2026, the issue time
//     (HR-BT-2), the business process type (P1 to P12 or P99:, table
//     HR-TB-1), the specification identifier, the electronic addresses of
//     the seller and of the buyer, the operator who issued the invoice and
//     his OIB (HR-BT-4, HR-BT-5), the OIB in the VAT identifiers of the
//     parties in Croatia, the references at line level (HR-BT-9 to
//     HR-BT-11), the HR VAT category label and the exemption reason of the
//     items and charges not subject to VAT or exempt (HR-BT-6 to HR-BT-8,
//     HR-BT-12 to HR-BT-14), the KPD classification of the items, the
//     Croatian VAT breakdown and totals of the extension (HR-BG-2,
//     HR-BT-23 to HR-BT-25), at most 10 decimals in the prices, rates and
//     quantities, the date ranges and the lengths of some texts;
//   - HR-BR-S-*, HR-BR-Z-*, HR-BR-E-*, HR-BR-AE-*: the buyer VAT identifier
//     that each VAT category requires, and the rates of the Croatian VAT
//     breakdown;
//   - HR-BR-CL-1 and HR-BR-CL-2: the HR VAT category labels of table
//     HR-TB-2 and the codes of the KPD 2025 classification (list CG), as
//     the schematron lists them.
//
// The OIB, the personal identification number of a Croatian person or
// company (osobni identifikacijski broj), has eleven digits, the last one a
// check digit of ISO 7064 (MOD 11,10): the schematron checks it with its
// function u:ctrlOIB, which the package reproduces along with the rules (see
// ValidOIB). All the rules are errors (flag fatal).
//
// The schematron is reproduced as published, with its slips: the messages
// of HR-BR-13 and HR-BR-55 name the wrong business terms (HR-BT-6 and
// HR-BT-7 for HR-BT-7 and HR-BT-8, HR-BT-110 for BT-110), and HR-BR-CL-2
// writes the identifier of HR-BR-25 in its message; the package reports
// the attribute id, as the SVRL report of the validator does, with English
// messages that name the right terms. The test of HR-BR-28 accepts a zero
// tax amount for any HR VAT category, as its first alternative holds as
// soon as the category has an identifier: the rule still asks a zero amount
// of the categories E and O. The rules on the date of a credit note line
// period use the same context as those of an invoice line.
//
// # Evaluation
//
// The rules are not rewritten one by one: rules_gen.go reproduces the
// schematron as the tax administration publishes it (see gen), and package
// internal/schematron evaluates its contexts, variables, functions and
// tests with an interpreter of the subset of XPath 2.0 they use, with the
// semantics of an XSLT 2.0 processor.
//
// # Beyond the schematron
//
// The package adds the check of the OIB in the electronic addresses of the
// seller (BT-34) and of the buyer (BT-49) of scheme 9934, the Croatian VAT
// number in the code list EAS, as the official examples write them
// (HR-OIB-01); the CIUS-HR does not require it: it is a warning. An invoice
// in the CII syntax that declares the CIUS-HR gets HR-SYNTAX-01, as the
// CIUS-HR has no CII binding.
//
// The fiscalisation of the invoice and the reports to the tax
// administration (eIzvještavanje), the access points (AMS and MPS) and the
// exchange of invoices, the signature, MIKROeRAČUN and the fiscalisation of
// the invoices to consumers are outside the CIUS-HR and this package.
package hr

//go:generate go run ./gen -o rules_gen.go

import (
	"slices"
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

// Rules of the package beyond the schematron.
const (
	RuleSyntax     = "HR-SYNTAX-01"
	RuleAddressOIB = "HR-OIB-01"
)

// compiled returns the compiled schematron, or the first expression that
// does not compile.
var compiled = sync.OnceValues(func() (*schematron.Schema, error) {
	return schematron.Compile(namespaces, patterns, functions...)
})

func all(string) bool { return true }

// Check applies the rules of the CIUS-HR schematron and the check of the
// OIB of the electronic addresses to the UBL invoice of c. A CII invoice
// gets HR-SYNTAX-01.
func Check(c *rules.Checker) {
	if c.Doc.UBL == nil {
		c.Fail(RuleSyntax, rules.Error, c.Doc.Root,
			"the CIUS-HR is defined for the UBL 2.1 syntax only: a CIUS-HR invoice cannot be a CII invoice")
		return
	}
	s, err := compiled()
	if err != nil {
		panic("hr: " + err.Error())
	}
	s.Run(c, c.Doc.UBL, all)
	checkAddresses(c, c.Doc.UBL)
}

// Catalog lists the rules of the Croatia rule set: those of the CIUS-HR
// schematron, in schematron order with their English messages, then the
// checks of the package. The list is built on first use; the caller must
// not modify it.
func Catalog() []rules.RuleInfo { return catalog() }

var catalog = sync.OnceValue(func() []rules.RuleInfo {
	return append(slices.Clip(schematronCatalog),
		rules.RuleInfo{ID: RuleSyntax, Severity: rules.Error, Description: "A CIUS-HR invoice shall use the UBL 2.1 syntax: the CIUS-HR has no CII binding"},
		rules.RuleInfo{ID: RuleAddressOIB, Severity: rules.Warning, Description: "An electronic address of scheme 9934 (BT-34, BT-49) shall be a valid OIB: eleven digits, the last one the check digit of ISO 7064 (MOD 11,10)"},
	)
})
