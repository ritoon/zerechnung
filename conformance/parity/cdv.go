package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ritoon/zerechnung/cdar"
	"github.com/ritoon/zerechnung/validate"
)

// The comparison of the statuses of the life cycle of an invoice: the XSLT
// that the FNFE-MPE publishes for its schematron BR-FR-CDV of the CDAR
// statuses (France_RFE 1.4.0.04, FX_FNFE), run by Saxon-HE 12.10
// (NPaBatch.java), against the rules of internal/rules/cdv. The statuses are
// the CDAR examples of France_RFE and the documents of the mutation tests of
// internal/rules/cdv. The CDAR files of the repository (cdar/testdata) are
// copies of examples of France_RFE, which the comparison already has.

// cdvXSL is the XSLT of the schematron BR-FR-CDV, relative to FX_FNFE.
const cdvXSL = "FNFE_RFE_INVOICE/CDAR/2xslt/BR-FR-CDV-Schematron-CDAR.xslt"

// cdvExampleDir is the directory of the CDAR examples of France_RFE,
// relative to FX_FNFE.
const cdvExampleDir = "FNFE_RFE_INVOICE/Z.example/TEST/CDAR"

// cdvInputs are the inputs of the comparison of the statuses. The statuses
// are staged under the origins cdv/fnfe/<path> and cdv/variants/<file>.
type cdvInputs struct {
	rfe      string // FX_FNFE: checkout of fnfempe/France_RFE
	variants string // documents of the mutation cases of internal/rules/cdv (go test -variants)
}

// oursCDV validates a status with this package, with the default rule sets,
// and keeps the findings of the rules BR-FR-CDV. The findings of the CDAR
// schema are left out: the XSLT does not have them.
func oursCDV(name string, data []byte) result {
	res := newResult()
	rep := validate.XML(data, nil)
	for _, f := range rep.Findings {
		if f.Source == validate.SourceCDV {
			res.add(f.Rule, f.Severity.String())
		} else {
			res.other(f.Rule)
		}
	}
	return res
}

// cdvListLine returns the line of cdv.list for a staged status.
func cdvListLine(rfe, src, abs string, data []byte) (string, error) {
	if m, _ := cdar.Parse(data); m == nil {
		return "", fmt.Errorf("%s: not a CDAR message", src)
	}
	return filepath.Join(rfe, filepath.FromSlash(cdvXSL)) + "\t" + abs + "\n", nil
}

// cdvSection writes the part of the report on the XSLT BR-FR-CDV: the
// statuses, and the transformations stopped on a dynamic error.
func cdvSection(b *strings.Builder, all []comparison) {
	var examples, variants int
	var stopped []string
	for _, c := range all {
		if c.Reference != "cdv" {
			continue
		}
		if strings.HasPrefix(c.Invoice, "cdv/fnfe/") {
			examples++
		} else {
			variants++
		}
		if c.Ref.Stopped != "" {
			stopped = append(stopped, fmt.Sprintf("| `%s` | %s |", strings.TrimPrefix(c.Invoice, "cdv/"), c.Ref.Stopped))
		}
	}
	if examples+variants == 0 {
		return
	}
	fmt.Fprintf(b, "\n### Statuts du cycle de vie (CDAR) : XSLT BR-FR-CDV de la FNFE-MPE\n\nSaxon applique à chaque statut la XSLT du schematron BR-FR-CDV (France_RFE 1.4.0.04). Statuts : %d exemples CDAR de France_RFE et %d variantes, les documents des tests de mutation de `internal/rules/cdv`.\n\n", examples, variants)
	b.WriteString("Les règles levées en erreur et en avertissement sont comparées sous leur identifiant. ZeRechnung valide avec ses règles par défaut ; seuls ses constats BR-FR-CDV entrent dans la comparaison, pas ceux du schéma CDAR D22B.\n")
	if len(stopped) > 0 {
		b.WriteString("\nArrêts sur erreur dynamique : la transformation s'arrête, et seule l'assertion qui l'a levée est comparée.\n\n| Statut | Arrêt |\n|---|---|\n")
		b.WriteString(strings.Join(stopped, "\n") + "\n")
	}
}
