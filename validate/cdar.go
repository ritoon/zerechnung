package validate

import (
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/rules/cdv"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The statuses of the life cycle of the French invoices (CDV), CDAR D22B
// messages (see package cdar), are checked against the CDAR D22B schema of
// the France_RFE package of the FNFE-MPE (the XSD-* rules of the Schema rule
// set), then against the rules BR-FR-CDV of its schematron (package
// internal/rules/cdv), whatever the rule sets: a status is French, as the
// controls of the SDI apply to any FatturaPA document. The France rule set
// lists the rules BR-FR-CDV. The rules of the invoices do not apply to a
// status.

// checkCDAR checks root, the root element of a CDAR message.
func checkCDAR(root *xmltree.Node, opts *Options, r *Report) {
	if set := opts.Rules; set == 0 || set&Schema != 0 {
		schema.CheckCDAR(&rules.Checker{Report: r, Source: SourceSyntax}, root)
	}
	cdv.Check(&rules.Checker{Report: r, Source: SourceCDV}, root)
}
