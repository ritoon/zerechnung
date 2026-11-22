package pt

import (
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/schematron"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The tables of rules_gen.go hold the schematron as eSPap publishes it;
// package schematron runs it the way an ISO schematron processor does.
type (
	let       = schematron.Let
	assertion = schematron.Assertion
	rule      = schematron.Rule
	pattern   = schematron.Pattern
)

// compiled returns the compiled schematron, or the first expression that
// does not compile.
var compiled = sync.OnceValues(func() (*schematron.Schema, error) {
	return schematron.Compile(namespaces, patterns)
})

// national reports whether an assertion is a rule of the CIUS-PT itself
// (BR-CIUS-PT, DT-CIUS-PT), checked in the Portugal rule set.
func national(id string) bool { return strings.Contains(id, "CIUS-PT") }

// run applies the schematron to the UBL document of c and reports the
// failed assertions of the CIUS-PT (cen false) or the others (cen true).
func run(c *rules.Checker, root *xmltree.Node, cen bool) {
	s, err := compiled()
	if err != nil {
		panic("pt: " + err.Error())
	}
	s.Run(c, root, func(id string) bool { return national(id) != cen })
}
