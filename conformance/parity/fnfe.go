package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// The EXTENDED-CTC-FR comparison: the XSLT that the FNFE-MPE publishes for
// its EXTENDED-CTC-FR schematrons (France_RFE 1.4.0.04, FX_FNFE), one for
// CII and one for UBL, run by Saxon-HE 12.10 (NPaBatch.java), against the
// rules of internal/rules/en16931 and internal/rules/ubl. The invoices are
// the EXTENDED-CTC-FR examples of France_RFE, the EXTENDED-CTC-FR invoices
// of the repository and the documents of the EXTENDED-CTC-FR mutation cases
// of both packages.

// The XSLT of the FNFE-MPE, relative to FX_FNFE.
const (
	fnfeXSLCII = "FNFE_RFE_INVOICE/CII/EXTENDED-CTC-FR/2xslt/EXTENDED-CTC-FR-CII.xslt"
	fnfeXSLUBL = "FNFE_RFE_INVOICE/UBL/EXTENDED-CTC-FR/2xslt/EXTENDED-CTC-FR-UBL.xslt"
)

// fnfeExampleDirs are the directories of the EXTENDED-CTC-FR examples of
// France_RFE, relative to FX_FNFE. The examples of the other profiles do not
// declare EXTENDED-CTC-FR, which the package then does not apply.
var fnfeExampleDirs = []string{
	"FNFE_RFE_INVOICE/Z.example/TEST/CII/EXTENDED-CTC-FR",
	"FNFE_RFE_INVOICE/Z.example/TEST/UBL/EXTENDED-CTC-FR",
}

// fnfeInputs are the inputs of the EXTENDED-CTC-FR comparison. The invoices
// are staged under the origins fr/fnfe/<path>, fr/zerechnung/<path> and
// fr/variants/<package>/<file>.
type fnfeInputs struct {
	rfe      string // FX_FNFE: checkout of fnfempe/France_RFE
	repo     string // root of this repository, whose internal/rules/fr/testdata holds invoices of every French profile
	variants string // documents of the EXTENDED-CTC-FR mutation cases (go test -variants), one directory per rule package (en16931, ubl)
}

// fnfeRepoDir holds the invoices of the repository among which the
// comparison takes those that declare EXTENDED-CTC-FR.
const fnfeRepoDir = "internal/rules/fr/testdata"

// declaresCTCFR reports whether a document holds the specification
// identifier of EXTENDED-CTC-FR.
func declaresCTCFR(data []byte) bool {
	return bytes.Contains(data, []byte(invoice.URNExtendedCTCFR))
}

// fnfeStylesheet returns the XSLT of the FNFE-MPE for an invoice, relative
// to FX_FNFE: the CII one for a CII invoice, the UBL one for a UBL Invoice
// or CreditNote, "" for another document.
func fnfeStylesheet(data []byte) string {
	switch npaStylesheet(data) {
	case xslCII:
		return fnfeXSLCII
	case xslUBL, xslGAccount:
		return fnfeXSLUBL
	}
	return ""
}

// oursFR validates an invoice of the EXTENDED-CTC-FR comparison with this
// package, with the default rule sets, and keeps the findings of the EN 16931
// rule set, which applies the EXTENDED-CTC-FR rules to an invoice that
// declares the profile. The findings of the XML schema and of the other rule
// sets are left out: the XSLT do not have them.
func oursFR(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, nil)
	for _, f := range rep.Findings {
		if f.Source == validate.SourceEN16931 {
			res.add(f.Rule, f.Severity.String())
		} else {
			res.other(f.Rule)
		}
	}
	return res
}

// fnfeSection writes the part of the report on the EXTENDED-CTC-FR XSLT: the
// invoices, and the transformations stopped on a dynamic error.
func fnfeSection(b *strings.Builder, all []comparison) {
	var examples, repo, variants int
	var stopped []string
	for _, c := range all {
		if c.Reference != "fnfe" {
			continue
		}
		switch {
		case strings.HasPrefix(c.Invoice, "fr/fnfe/"):
			examples++
		case strings.HasPrefix(c.Invoice, "fr/zerechnung/"):
			repo++
		default:
			variants++
		}
		if c.Ref.Stopped != "" {
			stopped = append(stopped, fmt.Sprintf("| `%s` | %s |", strings.TrimPrefix(c.Invoice, "fr/"), c.Ref.Stopped))
		}
	}
	if examples+repo+variants == 0 {
		return
	}
	fmt.Fprintf(b, "\n### EXTENDED-CTC-FR : XSLT de la FNFE-MPE\n\nSaxon applique à chaque facture la XSLT EXTENDED-CTC-FR de sa syntaxe (France_RFE 1.4.0.04, CII ou UBL). Factures : %d exemples EXTENDED-CTC-FR de France_RFE, %d factures EXTENDED-CTC-FR du dépôt et %d variantes, les documents des tests de mutation EXTENDED-CTC-FR de `internal/rules/en16931` et `internal/rules/ubl`.\n\n", examples, repo, variants)
	b.WriteString("Les règles levées en erreur et en avertissement sont comparées sous leur identifiant (pour une assertion sans identifiant, celui entre crochets qui ouvre son message). ZeRechnung valide avec ses règles par défaut ; seuls ses constats EN 16931, qui portent les règles EXTENDED-CTC-FR, entrent dans la comparaison.\n")
	if len(stopped) > 0 {
		b.WriteString("\nArrêts sur erreur dynamique : la transformation s'arrête, et seule l'assertion qui l'a levée est comparée.\n\n| Facture | Arrêt |\n|---|---|\n")
		b.WriteString(strings.Join(stopped, "\n") + "\n")
	}
}

// fnfeListLine returns the line of fnfe.list for a staged invoice.
func fnfeListLine(rfe, src, abs string, data []byte) (string, error) {
	xsl := fnfeStylesheet(data)
	if xsl == "" {
		return "", fmt.Errorf("%s: neither a CII nor a UBL invoice", src)
	}
	return filepath.Join(rfe, filepath.FromSlash(xsl)) + "\t" + abs + "\n", nil
}
