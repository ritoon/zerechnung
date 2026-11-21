package fatturapa

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/sigtest"
)

// The interoperability of the signatures is checked by
// conformance/signature.sh, in the Conformité workflow:
//
//  1. TestWriteInteropFiles, with FX_SIG_OUT, writes a test PKI and the
//     files that SignCAdES and SignXAdES produce;
//  2. OpenSSL (openssl cms -verify), xmlsec1 (xmlsec1 --verify) and EU DSS
//     verify them, and sign the same document with the same keys;
//  3. TestForeignSignatures, with FX_SIG_FOREIGN, verifies the files those
//     tools signed.
//
// Without the variables, both tests check their own setup and return.

// interopSigners are the keys of the interoperability files.
var interopSigners = []struct {
	name string
	pss  bool
	ec   bool
}{
	{name: "rsa"},
	{name: "pss", pss: true},
	{name: "ec", ec: true},
}

// TestWriteInteropFiles writes in the directory named by FX_SIG_OUT the
// root and intermediate certificates (root.pem, intermediate.pem), the
// signing certificates and their PKCS#8 keys (signer-rsa.pem,
// signer-rsa.key, signer-ec.pem, signer-ec.key), the document signed
// (fpa12.xml), and the document signed in CAdES (cades-<key>.xml.p7m) and in
// XAdES (xades-<key>.xml) with each key.
func TestWriteInteropFiles(t *testing.T) {
	out := os.Getenv("FX_SIG_OUT")
	doc := readTestdata(t, "fpa12.xml")
	p := sigtest.New(t)
	files := map[string][]byte{
		"fpa12.xml":        doc,
		"root.pem":         pemCert(p.Root),
		"intermediate.pem": pemCert(p.Intermediate),
	}
	for _, s := range interopSigners {
		// The tools check the certificates against the current time.
		now := time.Now()
		o := sigtest.LeafOptions{QC: sigtest.QcSignature, NotBefore: now.AddDate(-1, 0, 0), NotAfter: now.AddDate(1, 0, 0)}
		if s.ec {
			o.Key = sigtest.ECKey(t)
		}
		cert, key := p.Leaf(t, o)
		if !s.pss {
			keyDER, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			files["signer-"+s.name+".pem"] = pemCert(cert)
			files["signer-"+s.name+".key"] = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		}
		opts := &SignOptions{Signer: key, Chain: []*x509.Certificate{cert, p.Intermediate}, PSS: s.pss}
		signed := signBoth(t, doc, opts)
		files["cades-"+s.name+".xml.p7m"] = signed[CAdES]
		files["xades-"+s.name+".xml"] = signed[XAdES]
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

func pemCert(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

// foreignExpect lists, by prefix of the file name, the rules of
// VerifySignature that a file signed by another tool is expected to raise,
// besides IT-SIG-05 (no revocation data). Every file of FX_SIG_FOREIGN must
// match a prefix.
var foreignExpect = []struct {
	prefix string
	rules  []string
}{
	// openssl cms -sign -cades: CAdES Baseline B but for the mime-type
	// attribute, which is only recommended.
	{"openssl-cades", nil},
	// openssl cms -sign without -cades: no signing-certificate-v2.
	{"openssl-", []string{RuleNotBaselineB}},
	// xmlsec1 --sign: a plain XML signature, without XAdES properties.
	{"xmlsec1-", []string{RuleNoSigningTime, RuleNotBaselineB}},
	// EU DSS, XAdES and CAdES Baseline B.
	{"dss-", nil},
}

// TestForeignSignatures verifies the files that other tools signed, in the
// directory named by FX_SIG_FOREIGN (see conformance/signature.sh): each
// one must unwrap to fpa12.xml of the same directory, and VerifySignature
// must raise the expected rules, with root.pem as the trusted root.
func TestForeignSignatures(t *testing.T) {
	dir := os.Getenv("FX_SIG_FOREIGN")
	if dir == "" {
		return
	}
	doc, err := os.ReadFile(filepath.Join(dir, "fpa12.xml"))
	if err != nil {
		t.Fatal(err)
	}
	rootPEM, err := os.ReadFile(filepath.Join(dir, "root.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		t.Fatal("root.pem holds no certificate")
	}
	// Some tools write the signing certificate only: the intermediate
	// authority is given apart, as a verifier would get it.
	var intermediates []*x509.Certificate
	if interPEM, err := os.ReadFile(filepath.Join(dir, "intermediate.pem")); err == nil {
		if b, _ := pem.Decode(interPEM); b != nil {
			c, err := x509.ParseCertificate(b.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			intermediates = append(intermediates, c)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if name == "fpa12.xml" || strings.HasSuffix(name, ".pem") {
			continue
		}
		n++
		t.Run(name, func(t *testing.T) {
			var want []string
			found := false
			for _, f := range foreignExpect {
				if strings.HasPrefix(name, f.prefix) {
					want, found = f.rules, true
					break
				}
			}
			if !found {
				t.Fatalf("no expectation for %s", name)
			}
			file, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			got, sig, err := Unwrap(file)
			if err != nil {
				t.Fatalf("Unwrap: %v", err)
			}
			if sig.Format == XAdES {
				// The tools write the document again: compare the invoices.
				if !sameInvoices(t, got, doc) {
					t.Errorf("the signed file does not hold the invoice of fpa12.xml")
				}
			} else if !bytes.Equal(got, doc) {
				t.Errorf("the unwrapped content differs from fpa12.xml")
			}
			r := VerifySignature(file, &VerifyOptions{Roots: roots, Intermediates: intermediates})
			var rules []string
			for _, f := range r.Findings {
				if f.Rule != RuleRevocationUnknown {
					rules = append(rules, f.Rule)
				}
			}
			sort.Strings(rules)
			if strings.Join(rules, " ") != strings.Join(want, " ") {
				t.Errorf("rules %v, want %v:\n%s", rules, want, r)
			}
			t.Logf("%s: %d signer(s), %s", sig.Format, len(sig.Signers), sig.Signers[0].Certificate.Subject)
		})
	}
	if n == 0 {
		t.Fatal("no signed file in FX_SIG_FOREIGN")
	}
	t.Logf("verified %d files signed by other tools", n)
}

// sameInvoices reports whether two FatturaPA documents hold the same
// invoices.
func sameInvoices(t *testing.T, a, b []byte) bool {
	t.Helper()
	ia, err := ParseAll(a)
	if err != nil {
		t.Errorf("signed file: %v", err)
	}
	ib, err := ParseAll(b)
	if err != nil {
		t.Errorf("fpa12.xml: %v", err)
	}
	ja, _ := json.Marshal(ia)
	jb, _ := json.Marshal(ib)
	return len(ia) > 0 && bytes.Equal(ja, jb)
}
