package validate

import (
	"github.com/ritoon/zerechnung/ereporting"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Reports of the French e-reporting (flux 10, see package ereporting) are
// checked against the schemas of the flux 10 of France_RFE 1.4.0.04,
// compiled into the package (the XSD-* rules of the Schema rule set), and
// for the parts that ereporting.Parse cannot read (ER-01): a date that is
// not AAAAMMJJ, a time of transmission that is not AAAAMMJJHHMMSS, which
// the schemas type as strings. A report is not an invoice: the EN 16931
// rules and the national rules do not apply, nor are the rules of the PPF
// on its content checked (the schematron Flux10 of France_RFE), which the
// warning ER-02 says whatever the rule sets.

// Rules of the e-reporting check.
const (
	ruleERValue = "ER-01"
	ruleERScope = "ER-02"
)

// ereportingCatalog lists the rules of the e-reporting check, part of the
// Schema rule set, except ER-02, reported whatever the rule sets.
var ereportingCatalog = []RuleInfo{
	{ID: ruleERValue, Severity: Error, Description: "Every value of an e-reporting report can be read: the dates in the format AAAAMMJJ, the time of the transmission (TT-3) in the format AAAAMMJJHHMMSS, the amounts, rates and quantities as decimals, the number of transactions as an integer."},
	{ID: ruleERScope, Severity: Warning, Description: "An e-reporting report (flux 10) is only checked against the XML schemas of the flux 10 of France_RFE and ER-01, not against the rules of the PPF on its content (schematron Flux10 of France_RFE)."},
}

// checkEReporting checks root, the root element of an e-reporting report
// (see ereporting.IsReport). The profile of the report is opts.Profile.
func checkEReporting(root *xmltree.Node, opts *Options, r *Report) {
	r.Profile = opts.Profile
	if set := opts.Rules; set == 0 || set&Schema != 0 {
		n := len(r.Findings)
		schema.CheckEReporting(&rules.Checker{Report: r, Source: SourceSyntax}, root)
		// The schema check reports the parts that the schema does not
		// define, and the values of the elements it types as decimals,
		// integers or booleans: ER-01 reports the other values that cannot
		// be read, those of the elements typed as strings (the dates).
		var typed map[string]bool
		for _, f := range r.Findings[n:] {
			if f.Rule == schema.RuleType {
				if typed == nil {
					typed = map[string]bool{}
				}
				typed[f.Path] = true
			}
		}
		_, issues := ereporting.ParseTree(root)
		for _, is := range issues {
			if !is.Undefined && !typed[is.Path] {
				r.Add(Finding{Rule: ruleERValue, Severity: Error, Source: SourceSyntax, Path: is.Path, Line: is.Line, Message: is.Message})
			}
		}
	}
	r.Add(Finding{Rule: ruleERScope, Severity: Warning, Source: SourceSyntax, Path: root.Path(), Line: root.Line,
		Message: "an e-reporting report (flux 10) is only checked against the XML schemas of the flux 10 of France_RFE " + schema.EReportingVersion() +
			" and for the values that can be read (ER-01), not against the rules of the PPF on its content (schematron Flux10 of France_RFE)"})
}
