package zerechnung

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/internal/tsp/tsptest"
	"github.com/ritoon/zerechnung/validate"
)

// TestWritePAdESInteropFiles writes, in the directory FX_PADES_OUT, a test
// PKI, an unsigned hybrid invoice (invoice.pdf) for the other tools to
// sign, and the PDF that Sign, AddValidationData and AddDocumentTimestamp
// produce, for conformance/signature.sh: EU DSS must find the signatures of
// pades-bb-* PAdES-BASELINE-B, of pades-bt-* PAdES-BASELINE-T and of
// pades-lta-* PAdES-BASELINE-LTA. The keys go to signer-*.key, for DSS to
// sign with them.
func TestWritePAdESInteropFiles(t *testing.T) {
	out := os.Getenv("FX_PADES_OUT")
	p := sigtest.New(t)
	ts := tsptest.New(t, p)
	invoice := signHost(t)
	files := map[string][]byte{
		"invoice.pdf":      invoice,
		"root.pem":         pemBlock("CERTIFICATE", p.Root.Raw),
		"intermediate.pem": pemBlock("CERTIFICATE", p.Intermediate.Raw),
	}
	// The tools check the certificates against the current time.
	now := time.Now()
	leaf := func(name string, key crypto.Signer) *SignOptions {
		o := sigtest.LeafOptions{QC: sigtest.QcSeal, Key: key, NotBefore: now.AddDate(-1, 0, 0), NotAfter: now.AddDate(1, 0, 0),
			CommonName: "Exemple Informatique SAS", CRLDistributionPoints: []string{"http://crl.example.invalid/intermediate.crl"}}
		cert, k := p.Leaf(t, o)
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		files["signer-"+name+".pem"] = pemBlock("CERTIFICATE", cert.Raw)
		files["signer-"+name+".key"] = pemBlock("PRIVATE KEY", der)
		return &SignOptions{Signer: k, Chain: []*x509.Certificate{cert, p.Intermediate}, Reason: "Facture", Location: "Paris"}
	}
	rsa := leaf("rsa", nil)
	ec := leaf("ec", sigtest.ECKey(t))
	sign := func(name string, data []byte, o *SignOptions) []byte {
		t.Helper()
		signed, err := Sign(data, o)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if name != "" {
			files[name] = signed
		}
		return signed
	}
	sign("pades-bb-rsa.pdf", invoice, rsa)
	pss := *rsa
	pss.PSS, pss.Hash = true, crypto.SHA384
	sign("pades-bb-pss.pdf", invoice, &pss)
	sign("pades-bb-ec.pdf", invoice, ec)
	certify := *rsa
	certify.Certify = CertifyFormFilling
	sign("pades-bb-certified-and-approved.pdf", sign("", invoice, &certify), ec)
	bt := *ec
	bt.Timestamp = ts.Timestamp
	signed := sign("pades-bt-ec.pdf", invoice, &bt)

	// B-LTA: the chain and the revocation data of the signing certificate
	// and of the intermediate authority, then a document time-stamp.
	sig := bt.Chain[0]
	lt, err := AddValidationData(signed, &ValidationData{
		Certificates: []*x509.Certificate{p.Intermediate, p.Root, ts.Cert},
		CRLs: [][]byte{
			p.CRL(t, now.Add(-time.Hour), time.Time{}, false).Raw,
			p.RootCRL(t, now.Add(-time.Hour)).Raw,
		},
		OCSP: [][]byte{p.OCSP(t, sig, sigtest.OCSPGood, time.Time{}, now.Add(-time.Minute), false)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if files["pades-lta-ec.pdf"], err = AddDocumentTimestamp(lt, ts.Timestamp, 0); err != nil {
		t.Fatal(err)
	}

	// Each file verifies here first.
	trust := &validate.TrustOptions{Roots: p.Roots}
	for name, data := range files {
		if !strings.HasPrefix(name, "pades-") {
			continue
		}
		r, sigs, err := VerifySignatures(data, trust)
		if err != nil {
			t.Fatal(err)
		}
		if got := problems(r); len(got) > 0 || len(sigs) == 0 {
			t.Errorf("%s: %q, %d signatures", name, got, len(sigs))
		}
	}
	if out == "" {
		return
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("wrote %d files in %s", len(files), out)
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

// TestForeignPAdES checks the PDF that other tools signed, in the
// directory FX_PADES_FOREIGN (conformance/signature.sh): EU DSS signs
// invoice.pdf in PAdES Baseline B (dss-pades-*.pdf). VerifySignatures must
// find a valid PAdES B-B signature over the whole file, trusted with
// root.pem, and the invoice must still be there.
func TestForeignPAdES(t *testing.T) {
	dir := os.Getenv("FX_PADES_FOREIGN")
	if dir == "" {
		return
	}
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cert := func(name string) *x509.Certificate {
		t.Helper()
		b, _ := pem.Decode(read(name))
		if b == nil {
			t.Fatalf("%s holds no certificate", name)
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert("root.pem"))
	trust := &validate.TrustOptions{Roots: roots, Intermediates: []*x509.Certificate{cert("intermediate.pem")}}
	invoice, err := Extract(read("invoice.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "dss-pades-*.pdf"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no dss-pades-*.pdf in %s: %v", dir, err)
	}
	for _, path := range matches {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data := read(name)
			r, sigs, err := VerifySignatures(data, trust)
			if err != nil {
				t.Fatal(err)
			}
			if got := problems(r); len(got) > 0 {
				t.Errorf("findings %q", got)
			}
			if len(sigs) != 1 {
				t.Fatalf("%d signatures, want 1", len(sigs))
			}
			s := sigs[0]
			if !s.Valid || s.Level != LevelBB || !s.WholeDocument || s.SubFilter != "ETSI.CAdES.detached" {
				t.Errorf("signature %+v", s)
			}
			t.Logf("%s, %s, %s", s.Field, s.Level, s.Certificate.Subject)
			att, err := Extract(data)
			if err != nil || !bytes.Equal(att.XML, invoice.XML) {
				t.Errorf("the invoice of the signed file: %v", err)
			}
		})
	}
}
