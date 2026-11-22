package validate

import (
	"slices"

	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schema"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Facturae documents, the Spanish format (versions 3.2, 3.2.1 and 3.2.2),
// are checked against the XML schema of their version, compiled into the
// package (the XSD-* rules of the Schema rule set; the XML signature and
// the content of the Extensions elements are not checked by it), and for
// values that can be read (FE-01). Facturae is not a syntax of EN 16931:
// the EN 16931 rules and the national rules, written for CII and UBL
// documents, do not apply, which the warning FE-02 says whatever the rule
// sets.
//
// The FACe rule set requires the XAdES signature of the document, and
// checks it as facturae.VerifySignature does, at Options.ReceivedAt: its
// integrity, the signature policy of Facturae, the signing time, the role
// of the signer and the signing certificate. Trust in the certificate and
// its revocation need the trusted roots and the revocation data that
// facturae.VerifySignature takes: the rule set does not check them, and
// reports neither ES-SIG-11 nor ES-SIG-12.

// Rules of the Facturae check.
const (
	ruleFEValue = "FE-01"
	ruleFEScope = "FE-02"
)

// facturaeCatalog lists the rules of the Facturae check, part of the Schema
// rule set, except FE-02, reported whatever the rule sets.
var facturaeCatalog = []RuleInfo{
	{ID: ruleFEValue, Severity: Error, Description: "A Facturae document holds a file header, the parties and at least one invoice, and every amount, rate, quantity, date, country code and payment means of it can be read."},
	{ID: ruleFEScope, Severity: Warning, Description: "Facturae is not a syntax of EN 16931: its documents only get the check of the XML schema of their version (3.2, 3.2.1 or 3.2.2) and FE-01, not the EN 16931 or national rules."},
}

// notValidated are the rules of facturae.VerifySignature that the FACe rule
// set does not report: it is given neither trusted roots nor revocation
// data.
var notValidated = []string{facturae.RuleTrustNotChecked, facturae.RuleRevocationUnknown}

// faceCatalog lists the rules of the FACe rule set.
func faceCatalog() []RuleInfo {
	var out []RuleInfo
	for _, r := range facturae.SignatureRules {
		if !slices.Contains(notValidated, r.ID) {
			out = append(out, r)
		}
	}
	return out
}

// signedTree reports whether root, the root element of a Facturae document,
// holds a ds:Signature.
func signedTree(root *xmltree.Node) bool {
	for _, c := range root.Children {
		if c.Space == facturae.NamespaceDS && c.Name == "Signature" {
			return true
		}
	}
	return false
}

// checkFacturaeSignature checks the signature of data, a signed Facturae
// document, under the FACe rule set: at opts.ReceivedAt, else at
// opts.Trust.At, against the roots and the revocation data of opts.Trust.
func checkFacturaeSignature(data []byte, opts *Options, r *Report) {
	at := opts.ReceivedAt
	if at.IsZero() {
		at = opts.Trust.At
	}
	vo := &facturae.VerifyOptions{Roots: opts.Trust.Roots, Intermediates: opts.Trust.Intermediates, CRLs: opts.Trust.CRLs, OCSP: opts.Trust.OCSP, At: at}
	for _, f := range facturae.VerifySignature(data, vo).Findings {
		if !slices.Contains(notValidated, f.Rule) {
			r.Add(f)
		}
	}
}

// checkFacturae checks root, the root element of a Facturae document (see
// facturae.IsFacturae). The profile of the report is opts.Profile.
func checkFacturae(root *xmltree.Node, opts *Options, r *Report) {
	r.Profile = opts.Profile
	set := opts.Rules
	if set == 0 {
		set = Default
	}
	if set&FACe != 0 && !signedTree(root) {
		r.Add(Finding{Rule: facturae.RuleNotSigned, Severity: Error, Source: SourceFacturae, Path: root.Path(), Line: root.Line,
			Message: "the document is not signed: FACe only receives invoices signed in XAdES-EPES under the signature policy of Facturae"})
	}
	if set&Schema != 0 {
		schema.CheckFacturae(&rules.Checker{Report: r, Source: SourceSyntax}, root)
		_, issues := facturae.ParseTree(root)
		for _, is := range issues {
			r.Add(Finding{Rule: ruleFEValue, Severity: Error, Source: SourceSyntax, Path: is.Path, Line: is.Line, Message: is.Message})
		}
	}
	r.Add(Finding{Rule: ruleFEScope, Severity: Warning, Source: SourceSyntax, Path: root.Path(), Line: root.Line,
		Message: "Facturae " + facturae.Version(root) + " is not a syntax of EN 16931: only the XML schema of its version and the readability of its values are checked (FE-01), " +
			"not the EN 16931 or national rules. Convert it to UBL or CII (package facturae) to check the EN 16931 rules on its data."})
}
