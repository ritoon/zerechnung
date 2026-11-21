package validate

import (
	"os"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
)

// The FatturaPA test documents of package fatturapa.
var fatturaPAFiles = []string{"../fatturapa/testdata/fpr12.xml", "../fatturapa/testdata/fpa12.xml"}

// TestFatturaPA checks that a FatturaPA document is checked against its
// schema, with the Schema rule set only, whatever the other rule sets.
func TestFatturaPA(t *testing.T) {
	for _, name := range fatturaPAFiles {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, opts := range []*Options{nil, {Rules: Default | France | Italy | Peppol}, {Auto: true}} {
			if r := XML(data, opts); len(r.Findings) > 0 || r.Profile != invoice.ProfileUnknown {
				t.Errorf("%s with %+v: profile %s, findings:\n%s", name, opts, r.Profile, r)
			}
		}

		bad := []byte(strings.Replace(string(data), "<Numero>", "<Nota>x</Nota><Numero>", 1))
		r := XML(bad, nil)
		if len(r.Findings) != 1 || r.Findings[0].Rule != "XSD-UNEXPECTED" || r.Findings[0].Source != SourceSyntax || r.Valid() {
			t.Errorf("%s with an unknown element: findings:\n%s", name, r)
		}
		if r := XML(bad, &Options{Rules: EN16931 | Italy}); len(r.Findings) > 0 {
			t.Errorf("%s without the Schema rule set: findings:\n%s", name, r)
		}
	}

	r := XML([]byte(`<FatturaElettronica versione="FPR12"/>`), nil)
	if len(r.Findings) != 1 || r.Findings[0].Rule != "CII" || !strings.Contains(r.Findings[0].Message, "FatturaPA FatturaElettronica") {
		t.Errorf("FatturaElettronica outside its namespace: findings:\n%s", r)
	}
}
