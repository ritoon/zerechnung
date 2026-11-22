package nl

import (
	"testing"

	"github.com/ritoon/zerechnung/internal/variants"
)

// TestVariants builds the document of every mutation case of the CII, UBL
// and G-account invoices, and writes them to -variants when it is set:
//
//	go test ./internal/rules/nl -run '^TestVariants$' -variants DIR
//
// cii-001-<case>.xml, ubl-001-<case>.xml, gaccount-001-<case>.xml, numbered
// in the order of the cases. The base invoices, which are in testdata, are
// left out. The parity job (conformance/parity.sh) compares them with the
// NPa XSLT.
func TestVariants(t *testing.T) {
	w := variants.New(t)
	for _, set := range []struct {
		prefix, base string
		cases        []mutationCase
	}{
		{"cii", base, mutationCases()},
		{"ubl", ublBase, ublCases()},
		{"gaccount", gaBase, gaccountCases()},
	} {
		doc := load(t, set.base)
		for i, c := range set.cases {
			if len(c.edits) == 0 {
				continue
			}
			w.Add(t, set.prefix, i, c.name, []byte(apply(t, doc, c.edits)), c.want)
		}
	}
	w.Close(t)
}
