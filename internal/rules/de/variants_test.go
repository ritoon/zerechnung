package de

import (
	"slices"
	"testing"

	"github.com/ritoon/zerechnung/internal/variants"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// kositScenarios are the specification identifiers (BT-24) by which the
// scenarios of the KoSIT validator select the XRechnung rules, by root
// element: the CIUS, the extension (for invoices only) and the CVD, compared
// as they are written.
var kositScenarios = map[string][]string{
	"CrossIndustryInvoice": {CIUSID, ExtensionID, CVDID},
	"Invoice":              {CIUSID, ExtensionID, CVDID},
	"CreditNote":           {CIUSID, CVDID},
}

// kositCovers reports whether the KoSIT validator checks doc with the
// XRechnung rules.
func kositCovers(t *testing.T, doc string) bool {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	spec := root.Elem("CustomizationID")
	if root.Name == "CrossIndustryInvoice" {
		spec = root.Elem("ExchangedDocumentContext").Elem("GuidelineSpecifiedDocumentContextParameter").Elem("ID")
	}
	return spec != nil && slices.Contains(kositScenarios[root.Name], spec.Text)
}

// TestVariants builds the document of every mutation case of the CII and UBL
// XRechnung invoices, base invoices included, and writes them to -variants
// when it is set:
//
//	go test ./internal/rules/de -run '^TestVariants$' -variants DIR
//
// cii-001-<case>.xml and ubl-001-<case>.xml, numbered in the order of the
// cases. The parity job (conformance/parity.sh) compares them with the KoSIT
// validator, which applies the EN 16931 rules and those of XRechnung. The
// cases whose specification identifier no scenario of the validator selects
// (BR-DE-21, the extension of a credit note) are left out: a validator then
// does not apply the rules of this package, which only test them when the
// caller asks for them.
func TestVariants(t *testing.T) {
	w := variants.New(t)
	for _, set := range []struct {
		prefix, base string
		cases        []mutationCase
	}{
		{"cii", base, mutationCases()},
		{"ubl", ublBase, ublCases()},
	} {
		doc := load(t, set.base)
		for i, c := range set.cases {
			v := apply(t, doc, c.edits)
			if !kositCovers(t, v) {
				continue
			}
			w.Add(t, set.prefix, i, c.name, []byte(v), c.want)
		}
	}
	w.Close(t)
}
