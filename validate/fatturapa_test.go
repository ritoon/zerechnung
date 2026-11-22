package validate

import (
	"os"
	"strings"
	"testing"
	"time"

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

// TestFatturaPASDI checks that the controls of the SdI on the content of a
// FatturaPA document apply whatever the rule sets, and that the date of the
// invoice is compared with Options.ReceivedAt.
func TestFatturaPASDI(t *testing.T) {
	data, err := os.ReadFile(fatturaPAFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte(strings.Replace(string(data), "<Imposta>225.50</Imposta>", "<Imposta>225.60</Imposta>", 1))
	for _, opts := range []*Options{nil, {Rules: EN16931}, {Rules: Italy}} {
		r := XML(bad, opts)
		if len(r.Findings) != 1 || r.Findings[0].Rule != "00421" || r.Findings[0].Source != SourceSDI || r.Valid() {
			t.Errorf("Imposta not consistent with %+v: findings:\n%s", opts, r)
		}
	}

	// The invoice is dated 20 November 2026, in Italy.
	for _, tc := range []struct {
		at   time.Time
		want int
	}{
		{time.Date(2026, time.November, 19, 21, 59, 0, 0, time.UTC), 1},
		{time.Date(2026, time.November, 19, 22, 0, 0, 0, time.UTC), 0},
	} {
		r := XML(data, &Options{ReceivedAt: tc.at})
		if len(r.Findings) != tc.want || tc.want == 1 && r.Findings[0].Rule != "00403" {
			t.Errorf("received at %s: findings:\n%s", tc.at, r)
		}
	}
}
