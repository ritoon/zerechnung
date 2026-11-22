package fatturapa_test

import (
	"fmt"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// The tests of package fatturapa validate the documents they write with
// validate.XML, which they cannot import: package validate imports
// fatturapa.
func init() {
	fatturapa.ValidateXML = func(xml []byte) (invoice.Profile, []string, string) {
		r := validate.XML(xml, nil)
		rules := make([]string, len(r.Findings))
		for i, f := range r.Findings {
			rules[i] = f.Rule
		}
		return r.Profile, rules, fmt.Sprint(r)
	}
}
