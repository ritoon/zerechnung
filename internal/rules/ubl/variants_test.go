package ubl

import (
	"testing"

	"github.com/ritoon/zerechnung/internal/variants"
)

// TestVariants builds the document of every case of TestCheck that a
// validator checks with the EN 16931 rules alone, and writes them to
// -variants when it is set:
//
//	go test ./internal/rules/ubl -run '^TestVariants$' -variants DIR
//
// ubl-001-<case>.xml, numbered in the order of the cases. The cases of the
// Dutch G-account extension, whose specification identifier selects other
// rules, are left out. The parity job (conformance/parity.sh) compares the
// documents with the KoSIT validator.
func TestVariants(t *testing.T) {
	w := variants.New(t)
	for i, c := range checkCases() {
		if c.new2 != "" && c.old2 == cenSpecification {
			continue
		}
		w.Add(t, "ubl", i, c.name, []byte(c.doc(t)), c.want)
	}
	w.Close(t)
}

// TestCTCFRVariants builds the document of every EXTENDED-CTC-FR mutation
// case, and writes them to -variants when it is set:
//
//	go test ./internal/rules/ubl -run '^TestCTCFRVariants$' -variants DIR
//
// ubl-000-<case>.xml, numbered in the order of the cases. The parity job
// (conformance/parity.sh) compares them with the EXTENDED-CTC-FR XSLT of the
// FNFE-MPE, which must raise the rules each case expects.
func TestCTCFRVariants(t *testing.T) {
	w := variants.New(t)
	for i, c := range ctcfrCases() {
		w.Add(t, "ubl", i, c.name, []byte(c.doc(t)), c.want)
	}
	w.Close(t)
}
