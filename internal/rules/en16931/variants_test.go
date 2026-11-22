package en16931

import (
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/variants"
	"github.com/ritoon/zerechnung/invoice"
)

// TestVariants builds the document of every case of TestRules that a
// validator checks with the EN 16931 rules, and writes them to -variants when
// it is set:
//
//	go test ./internal/rules/en16931 -run '^TestVariants$' -variants DIR
//
// cii-001-<case>.xml, numbered in the order of the cases. The cases of the
// Factur-X profiles, whose specification identifier selects other rules,
// are left out; an XRechnung case is kept, the validator adding the German
// rules. The parity job (conformance/parity.sh) compares the documents with
// the KoSIT validator.
func TestVariants(t *testing.T) {
	w := variants.New(t)
	for i, c := range ruleCases() {
		doc := c.doc(t)
		if c.profile != invoice.ProfileUnknown ||
			!strings.Contains(doc, "<ram:ID>urn:cen.eu:en16931:2017</ram:ID>") && !strings.Contains(doc, "urn:xeinkauf.de:kosit:xrechnung_3.0</ram:ID>") {
			continue
		}
		w.Add(t, "cii", i, c.name, []byte(doc), strings.Fields(c.want))
	}
	w.Close(t)
}

// TestCTCFRVariants builds the document of every EXTENDED-CTC-FR mutation
// case, and writes them to -variants when it is set:
//
//	go test ./internal/rules/en16931 -run '^TestCTCFRVariants$' -variants DIR
//
// cii-000-<case>.xml, numbered in the order of the cases. The parity job
// (conformance/parity.sh) compares them with the EXTENDED-CTC-FR XSLT of the
// FNFE-MPE, which must raise the rules each case expects.
func TestCTCFRVariants(t *testing.T) {
	w := variants.New(t)
	for i, c := range append(ctcfrCases(), ctcfrModelCases()...) {
		w.Add(t, "cii", i, c.name, []byte(c.doc(t)), strings.Fields(c.want))
	}
	w.Close(t)
}
