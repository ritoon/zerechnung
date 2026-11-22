package facturae_test

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/internal/xmldsig"
)

var signingTime = sigtest.Epoch.Add(-time.Hour)

// signer returns a qualified signing certificate of a test PKI, with the
// intermediate authority, and its key.
func signer(t *testing.T, o sigtest.LeafOptions) (*sigtest.PKI, []*x509.Certificate, crypto.Signer) {
	t.Helper()
	p := sigtest.New(t)
	if o.QC == 0 {
		o.QC = sigtest.QcSignature
	}
	cert, key := p.Leaf(t, o)
	return p, []*x509.Certificate{cert, p.Intermediate}, key
}

func sign(t *testing.T, doc []byte, o *facturae.SignOptions) []byte {
	t.Helper()
	out, err := facturae.Sign(doc, o)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return out
}

// versions returns the B2G invoice in the namespace of each version.
func versions(t *testing.T) map[string][]byte {
	src := string(read(t, b2gInvoice))
	out := map[string][]byte{"3.2.2": []byte(src)}
	for version, ns := range map[string]string{"3.2.1": facturae.Namespace321, "3.2": facturae.Namespace32} {
		d := strings.Replace(src, facturae.Namespace322, ns, 1)
		out[version] = []byte(strings.Replace(d, "<SchemaVersion>3.2.2<", "<SchemaVersion>"+version+"<", 1))
	}
	return out
}

func TestSignAndRead(t *testing.T) {
	for _, keyType := range []string{"RSA", "RSA-PSS", "ECDSA"} {
		o := sigtest.LeafOptions{}
		if keyType == "ECDSA" {
			o.Key = sigtest.ECKey(t)
		}
		p, chain, key := signer(t, o)
		for version, doc := range versions(t) {
			name := keyType + ", " + version
			signed := sign(t, doc, &facturae.SignOptions{Signer: key, Chain: chain, SigningTime: signingTime, PSS: keyType == "RSA-PSS"})
			if !strings.HasSuffix(string(signed), "</ds:Signature></fe:Facturae>\n") {
				t.Errorf("%s: the signature is not the last child of the root", name)
			}
			for _, want := range []string{
				`<xades:Identifier>` + facturae.PolicyIdentifier + `</xades:Identifier>`,
				`<ds:DigestValue>` + facturae.PolicyDigest + `</ds:DigestValue>`,
				`<xades:ClaimedRole>emisor</xades:ClaimedRole>`,
				`<xades:SigningCertificate>`,
			} {
				if !strings.Contains(string(signed), want) {
					t.Errorf("%s: no %s", name, want)
				}
			}
			sig, err := facturae.ReadSignature(signed)
			if err != nil {
				t.Fatalf("%s: ReadSignature: %v", name, err)
			}
			if !sig.Certificate.Equal(chain[0]) || len(sig.Certificates) != 1 || !sig.Certificates[0].Equal(chain[1]) ||
				!sig.SigningTime.Equal(signingTime) || !sig.Policy.IsFacturae() || sig.Policy.Description != facturae.PolicyDescription ||
				sig.Role != facturae.RoleSupplier || !sig.KeyInfoSigned || sig.SigningCertificate != "SigningCertificate" ||
				sig.Kind != facturae.KindSignature || !sig.Qualified || !sig.QSCD {
				t.Errorf("%s: signature %+v", name, sig)
			}
			// The signature verifies; only the revocation is left unchecked.
			r := facturae.VerifySignature(signed, &facturae.VerifyOptions{Roots: p.Roots, At: sigtest.Epoch})
			if got := findings(r); got != "ES-SIG-12:info" {
				t.Errorf("%s: %s\n%s", name, got, r)
			}
			// Parse reads the signed invoice as the unsigned one.
			want, err1 := facturae.Parse(doc)
			got, err2 := facturae.Parse(signed)
			if err1 != nil || err2 != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("%s: the signed invoice reads differently (%v, %v)", name, err1, err2)
			}
		}
	}
}

func TestSignRole(t *testing.T) {
	_, chain, key := signer(t, sigtest.LeafOptions{})
	doc := string(read(t, b2gInvoice))
	for _, c := range []struct {
		issuerType string
		role       facturae.SignerRole
		want       facturae.SignerRole
	}{
		{"EM", "", facturae.RoleSupplier},
		{"RE", "", facturae.RoleCustomer},
		{"TE", "", facturae.RoleThirdParty},
		{"XX", "", facturae.RoleSupplier},
		{"EM", facturae.RoleThirdParty, facturae.RoleThirdParty},
	} {
		d := strings.Replace(doc, "<InvoiceIssuerType>EM<", "<InvoiceIssuerType>"+c.issuerType+"<", 1)
		signed := sign(t, []byte(d), &facturae.SignOptions{Signer: key, Chain: chain, SigningTime: signingTime, Role: c.role})
		sig, err := facturae.ReadSignature(signed)
		if err != nil || sig.Role != c.want || !reflect.DeepEqual(sig.Roles, []string{string(c.want)}) {
			t.Errorf("%s, %q: %+v, %v", c.issuerType, c.role, sig, err)
		}
	}
}

func TestSignErrors(t *testing.T) {
	_, chain, key := signer(t, sigtest.LeafOptions{})
	doc := read(t, b2gInvoice)
	o := &facturae.SignOptions{Signer: key, Chain: chain}
	signed := sign(t, doc, o)
	for name, c := range map[string]struct {
		doc []byte
		o   *facturae.SignOptions
	}{
		"no options":       {doc, nil},
		"no signer":        {doc, &facturae.SignOptions{Chain: chain}},
		"no certificate":   {doc, &facturae.SignOptions{Signer: key}},
		"unknown role":     {doc, &facturae.SignOptions{Signer: key, Chain: chain, Role: "supplier"}},
		"already signed":   {signed, o},
		"not Facturae":     {read(t, italianInvoice), o},
		"not XML":          {[]byte("factura"), o},
		"Facturae 3.1":     {[]byte(strings.Replace(string(doc), facturae.Namespace322, "http://www.facturae.es/Facturae/2007/v3.1/Facturae", 1)), o},
		"signed elsewhere": {[]byte(strings.Replace(string(signed), "</ds:Signature></fe:Facturae>", "</ds:Signature><x/></fe:Facturae>", 1)), o},
		"empty root":       {[]byte(`<fe:Facturae xmlns:fe="` + facturae.Namespace322 + `"/>`), o},
	} {
		if _, err := facturae.Sign(c.doc, c.o); err == nil {
			t.Errorf("%s: signed", name)
		}
	}
	if _, err := facturae.ReadSignature(doc); !errors.Is(err, facturae.ErrNotSigned) {
		t.Errorf("unsigned document: %v", err)
	}
}

// findings returns the rules of a report with their severity, sorted, as
// "ES-SIG-02:error".
func findings(r *facturae.Report) string {
	var out []string
	for _, f := range r.Findings {
		if f.Source != rules.SourceFacturae {
			out = append(out, f.Rule+":wrong source "+f.Source)
		}
		out = append(out, f.Rule+":"+f.Severity.String())
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// signXML signs doc with internal/xmldsig, to write the signatures that
// Sign does not: other policies, roles, references.
func signXML(t *testing.T, doc []byte, o *xmldsig.SignOptions) []byte {
	t.Helper()
	out, err := xmldsig.SignEnveloped(doc, o)
	if err != nil {
		t.Fatalf("SignEnveloped: %v", err)
	}
	return out
}

func TestVerifySignatureRules(t *testing.T) {
	doc := read(t, b2gInvoice)
	p := sigtest.New(t)
	at := sigtest.Epoch
	leafOf := func(o sigtest.LeafOptions) ([]*x509.Certificate, crypto.Signer) {
		cert, key := p.Leaf(t, o)
		return []*x509.Certificate{cert, p.Intermediate}, key
	}
	chain, key := leafOf(sigtest.LeafOptions{QC: sigtest.QcSignature})
	leaf := chain[0]
	good := func(c *x509.Certificate, when time.Time) [][]byte {
		return [][]byte{p.OCSP(t, c, sigtest.OCSPGood, time.Time{}, when, false)}
	}
	opts := func(edit func(*facturae.VerifyOptions)) *facturae.VerifyOptions {
		o := &facturae.VerifyOptions{Roots: p.Roots, At: at, OCSP: good(leaf, at)}
		if edit != nil {
			edit(o)
		}
		return o
	}
	base := &facturae.SignOptions{Signer: key, Chain: chain, SigningTime: signingTime}
	with := func(edit func(*facturae.SignOptions)) []byte {
		c := *base
		edit(&c)
		return sign(t, doc, &c)
	}
	signed := sign(t, doc, base)
	policyDigest, _ := base64.StdEncoding.DecodeString(facturae.PolicyDigest)
	policy := &xmldsig.Policy{Identifier: facturae.PolicyIdentifier, Hash: crypto.SHA1, Digest: policyDigest}
	// xml writes the signature of Sign with one change.
	xml := func(d []byte, edit func(*xmldsig.SignOptions)) []byte {
		o := &xmldsig.SignOptions{
			Signer: key, Chain: chain, SigningTime: signingTime, Policy: policy,
			SigningCertificateV1: true, SignKeyInfo: true, ClaimedRoles: []string{"emisor"},
		}
		edit(o)
		return signXML(t, d, o)
	}

	type tc struct {
		name string
		doc  []byte
		opts *facturae.VerifyOptions
		want string
	}
	cases := []tc{
		{"valid", signed, opts(nil), ""},
		{"ES-SIG-01 not signed", doc, opts(nil), "ES-SIG-01:error"},
		{"ES-SIG-02 tampered", []byte(strings.Replace(string(signed), "A0042", "A0043", 1)), opts(nil), "ES-SIG-02:error"},
		{"ES-SIG-02 not XML", []byte("factura"), opts(nil), "ES-SIG-02:error"},
		{"ES-SIG-03 no policy", xml(doc, func(o *xmldsig.SignOptions) { o.Policy = nil }), opts(nil), "ES-SIG-03:error"},
		{"ES-SIG-03 other policy", xml(doc, func(o *xmldsig.SignOptions) {
			o.Policy = &xmldsig.Policy{Identifier: "urn:oid:2.16.724.1.3.1.1.2.1.9", Hash: crypto.SHA256, Digest: make([]byte, 32)}
		}), opts(nil), "ES-SIG-03:error"},
		{"ES-SIG-03 policy digest", xml(doc, func(o *xmldsig.SignOptions) {
			o.Policy = &xmldsig.Policy{Identifier: facturae.PolicyIdentifier, Hash: crypto.SHA1, Digest: make([]byte, 20)}
		}), opts(nil), "ES-SIG-03:error"},
		{"ES-SIG-04 signing time after the reception", with(func(o *facturae.SignOptions) { o.SigningTime = at.Add(time.Hour) }), opts(nil), "ES-SIG-04:error"},
		{"ES-SIG-05 unknown role", xml(doc, func(o *xmldsig.SignOptions) { o.ClaimedRoles = []string{"proveedor"} }), opts(nil), "ES-SIG-05:error"},
		{"ES-SIG-05 two roles", xml(doc, func(o *xmldsig.SignOptions) { o.ClaimedRoles = []string{"emisor", "receptor"} }), opts(nil), "ES-SIG-05:error"},
		{"English role", xml(doc, func(o *xmldsig.SignOptions) { o.ClaimedRoles = []string{"Third  Party"} }), opts(nil), "ES-SIG-06:warning"},
		{"no role", xml(doc, func(o *xmldsig.SignOptions) { o.ClaimedRoles = nil }), opts(nil), ""},
		{"ES-SIG-06 role", with(func(o *facturae.SignOptions) { o.Role = facturae.RoleCustomer }), opts(nil), "ES-SIG-06:warning"},
		{"ES-SIG-07 KeyInfo not signed", xml(doc, func(o *xmldsig.SignOptions) { o.SignKeyInfo = false }), opts(nil), "ES-SIG-07:warning"},
		{"SigningCertificateV2", xml(doc, func(o *xmldsig.SignOptions) { o.SigningCertificateV1 = false }), opts(nil), ""},
		{"ES-SIG-08 expired", signed, opts(func(o *facturae.VerifyOptions) {
			o.At = leaf.NotAfter.Add(time.Hour)
			o.OCSP = good(leaf, o.At)
		}), "ES-SIG-08:error ES-SIG-12:info"},
		{"ES-SIG-09 untrusted", signed, opts(func(o *facturae.VerifyOptions) { o.Roots = sigtest.New(t).Roots }), "ES-SIG-09:error"},
		{"ES-SIG-10 revoked", signed, opts(func(o *facturae.VerifyOptions) {
			o.OCSP = nil
			o.CRLs = []*x509.RevocationList{p.CRL(t, at, at.Add(-time.Minute), false, leaf)}
		}), "ES-SIG-10:error"},
		{"ES-SIG-10 revoked, OCSP", signed, opts(func(o *facturae.VerifyOptions) {
			o.OCSP = [][]byte{p.OCSP(t, leaf, sigtest.OCSPRevoked, at.Add(-time.Minute), at, false)}
		}), "ES-SIG-10:error"},
		{"ES-SIG-11 no roots", signed, opts(func(o *facturae.VerifyOptions) { o.Roots = nil }), "ES-SIG-11:warning"},
		{"ES-SIG-12 no revocation data", signed, opts(func(o *facturae.VerifyOptions) { o.OCSP = nil }), "ES-SIG-12:info"},
		{"ES-SIG-15 not Facturae", xml(read(t, italianInvoice), func(*xmldsig.SignOptions) {}), opts(nil), "ES-SIG-15:error"},
	}
	// A change to the signed properties breaks the signature, and nothing
	// else is reported.
	props := []byte(strings.Replace(string(signed), "<xades:ClaimedRole>emisor<", "<xades:ClaimedRole>receptor<", 1))
	cases = append(cases, tc{"ES-SIG-02 signed properties changed", props, opts(nil), "ES-SIG-02:error"})
	enc, encKey := leafOf(sigtest.LeafOptions{QC: sigtest.QcSignature, KeyUsage: x509.KeyUsageKeyEncipherment})
	cases = append(cases, tc{"ES-SIG-08 key usage", sign(t, doc, &facturae.SignOptions{Signer: encKey, Chain: enc, SigningTime: signingTime}),
		opts(func(o *facturae.VerifyOptions) { o.OCSP = good(enc[0], at) }), "ES-SIG-08:error"})
	plain, plainKey := leafOf(sigtest.LeafOptions{QC: sigtest.QcNone})
	cases = append(cases, tc{"ES-SIG-13 not qualified", sign(t, doc, &facturae.SignOptions{Signer: plainKey, Chain: plain, SigningTime: signingTime}),
		opts(func(o *facturae.VerifyOptions) { o.OCSP = good(plain[0], at) }), "ES-SIG-13:warning"})
	seal, sealKey := leafOf(sigtest.LeafOptions{QC: sigtest.QcSeal, CommonName: "ACME S.L."})
	cases = append(cases, tc{"ES-SIG-14 seal", sign(t, doc, &facturae.SignOptions{Signer: sealKey, Chain: seal, SigningTime: signingTime}),
		opts(func(o *facturae.VerifyOptions) { o.OCSP = good(seal[0], at) }), "ES-SIG-14:info"})

	for _, c := range cases {
		r := facturae.VerifySignature(c.doc, c.opts)
		if got := findings(r); got != c.want {
			t.Errorf("%s: %q, want %q\n%s", c.name, got, c.want, r)
		}
	}
}

func TestSignatureRulesCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range facturae.SignatureRules {
		if seen[r.ID] || r.Description == "" || !strings.HasPrefix(r.ID, "ES-SIG-") {
			t.Errorf("rule %+v", r)
		}
		seen[r.ID] = true
	}
	for i := 1; i <= 15; i++ {
		if id := fmt.Sprintf("ES-SIG-%02d", i); !seen[id] {
			t.Errorf("rule %s not in SignatureRules", id)
		}
	}
}
