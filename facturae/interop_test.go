package facturae_test

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

	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

// The interoperability of the signatures is checked by
// conformance/signature.sh, in the Conformité workflow, as for package
// fatturapa:
//
//  1. TestWriteInteropFiles, with FX_SIG_OUT, writes a test PKI and the
//     files that Sign produces;
//  2. xmlsec1 (xmlsec1 --verify) and EU DSS verify them, DSS as XAdES-EPES
//     under the policy of Facturae, and both sign the same document with
//     the same keys;
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
// (b2g.xml), and the document signed by Sign with each key
// (facturae-<key>.xml).
func TestWriteInteropFiles(t *testing.T) {
	out := os.Getenv("FX_SIG_OUT")
	doc := read(t, b2gInvoice)
	p := sigtest.New(t)
	files := map[string][]byte{
		"b2g.xml":          doc,
		"root.pem":         pemCert(p.Root),
		"intermediate.pem": pemCert(p.Intermediate),
	}
	for _, s := range interopSigners {
		// The tools check the certificates against the current time.
		now := time.Now()
		o := sigtest.LeafOptions{QC: sigtest.QcSignature, CommonName: "María García", NotBefore: now.AddDate(-1, 0, 0), NotAfter: now.AddDate(1, 0, 0)}
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
		chain := []*x509.Certificate{cert, p.Intermediate}
		files["facturae-"+s.name+".xml"] = sign(t, doc, &facturae.SignOptions{Signer: key, Chain: chain, PSS: s.pss})
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
// besides ES-SIG-12 (no revocation data). Every file of FX_SIG_FOREIGN must
// match a prefix.
var foreignExpect = []struct {
	prefix string
	rules  []string
}{
	// xmlsec1 --sign on a template with the XAdES-EPES properties of the
	// policy: xmlsec1 fills ds:KeyInfo after it computes the digests, so
	// no reference covers it; xades:SigningCertificateV2 binds the
	// certificate.
	{"xmlsec1-epes", []string{facturae.RuleKeyInfoNotSigned}},
	// xmlsec1 --sign: a plain XML signature, without XAdES properties.
	{"xmlsec1-", []string{facturae.RulePolicy, facturae.RuleSigningTime, facturae.RuleKeyInfoNotSigned}},
	// EU DSS, XAdES with the policy, the role emisor and ds:KeyInfo
	// signed, in EN 319 132-1 (SigningCertificateV2, SignerRoleV2) and in
	// XAdES 1.3.2 (SigningCertificate, SignerRole).
	{"dss-", nil},
}

// TestForeignSignatures verifies the files that other tools signed, in the
// directory named by FX_SIG_FOREIGN (see conformance/signature.sh): each
// one must hold the invoice of b2g.xml of the same directory, and
// VerifySignature must raise the expected rules, with root.pem as the
// trusted root.
func TestForeignSignatures(t *testing.T) {
	dir := os.Getenv("FX_SIG_FOREIGN")
	if dir == "" {
		return
	}
	doc, err := os.ReadFile(filepath.Join(dir, "b2g.xml"))
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
		if name == "b2g.xml" || strings.HasSuffix(name, ".pem") {
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
			sig, err := facturae.ReadSignature(file)
			if err != nil {
				t.Fatalf("ReadSignature: %v", err)
			}
			// The tools write the document again: compare the invoices.
			if !sameInvoices(t, file, doc) {
				t.Errorf("the signed file does not hold the invoice of b2g.xml")
			}
			r := facturae.VerifySignature(file, &facturae.VerifyOptions{Roots: roots, Intermediates: intermediates})
			var rules []string
			for _, f := range r.Findings {
				if f.Rule != facturae.RuleRevocationUnknown {
					rules = append(rules, f.Rule)
				}
			}
			sort.Strings(rules)
			if strings.Join(rules, " ") != strings.Join(want, " ") {
				t.Errorf("rules %v, want %v:\n%s", rules, want, r)
			}
			t.Logf("%s, role %q, policy %v, %s", sig.Certificate.Subject, sig.Role, sig.Policy.IsFacturae(), sig.SigningCertificate)
		})
	}
	if n == 0 {
		t.Fatal("no signed file in FX_SIG_FOREIGN")
	}
	t.Logf("verified %d files signed by other tools", n)
}

// sameInvoices reports whether two Facturae documents hold the same
// invoices.
func sameInvoices(t *testing.T, a, b []byte) bool {
	t.Helper()
	ia, err := facturae.ParseAll(a)
	if err != nil {
		t.Errorf("signed file: %v", err)
	}
	ib, err := facturae.ParseAll(b)
	if err != nil {
		t.Errorf("b2g.xml: %v", err)
	}
	ja, _ := json.Marshal(ia)
	jb, _ := json.Marshal(ib)
	return len(ia) > 0 && bytes.Equal(ja, jb)
}
