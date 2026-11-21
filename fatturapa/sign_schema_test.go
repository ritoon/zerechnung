package fatturapa_test

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/validate"
)

// TestSignedXAdESSchema checks that a document signed by SignXAdES stays
// valid against the FatturaPA schema, which places ds:Signature as the last
// child of FatturaElettronica.
func TestSignedXAdESSchema(t *testing.T) {
	p := sigtest.New(t)
	cert, key := p.Leaf(t, sigtest.LeafOptions{QC: sigtest.QcSignature})
	opts := &fatturapa.SignOptions{Signer: key, Chain: []*x509.Certificate{cert, p.Intermediate}, SigningTime: sigtest.Epoch}
	for _, name := range []string{"fpa12.xml", "fpr12.xml"} {
		doc, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		signed, err := fatturapa.SignXAdES(doc, opts)
		if err != nil {
			t.Fatal(err)
		}
		if r := validate.XML(signed, nil); len(r.Findings) != 0 {
			t.Errorf("%s, signed: %v", name, r.Findings)
		}
	}
}
