package fatturapa

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/internal/xmldsig"
)

// findings returns the rules of a report with their severity, sorted, as
// "00102:error".
func findings(r *Report) string {
	var out []string
	for _, f := range r.Findings {
		if f.Source != rules.SourceSDI {
			out = append(out, f.Rule+":wrong source "+f.Source)
		}
		out = append(out, f.Rule+":"+f.Severity.String())
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func TestVerifySignatureRules(t *testing.T) {
	fpa := readTestdata(t, "fpa12.xml")
	fpr := readTestdata(t, "fpr12.xml")
	p := sigtest.New(t)
	at := sigtest.Epoch
	leafOf := func(o sigtest.LeafOptions) ([]*x509.Certificate, *SignOptions) {
		cert, key := p.Leaf(t, o)
		chain := []*x509.Certificate{cert, p.Intermediate}
		return chain, &SignOptions{Signer: key, Chain: chain, SigningTime: at.Add(-time.Hour)}
	}
	chain, base := leafOf(sigtest.LeafOptions{QC: sigtest.QcSignature})
	leaf := chain[0]
	good := func(c *x509.Certificate, when time.Time) [][]byte {
		return [][]byte{p.OCSP(t, c, sigtest.OCSPGood, time.Time{}, when, false)}
	}
	opts := func(edit func(*VerifyOptions)) *VerifyOptions {
		o := &VerifyOptions{Roots: p.Roots, At: at, OCSP: good(leaf, at)}
		if edit != nil {
			edit(o)
		}
		return o
	}
	with := func(o *SignOptions, edit func(*SignOptions)) *SignOptions {
		c := *o
		edit(&c)
		return &c
	}

	type tc struct {
		name string
		doc  []byte
		sign *SignOptions
		opts *VerifyOptions
		want string
	}
	cases := []tc{
		{"valid", fpa, base, opts(nil), ""},
		{"valid invoice between companies", fpr, base, opts(nil), ""},
		{"00100 expired", fpa, base, opts(func(o *VerifyOptions) {
			o.At = leaf.NotAfter.Add(time.Hour)
			o.OCSP = good(leaf, o.At)
		}), "00100:error"},
		{"00101 revoked", fpa, base, opts(func(o *VerifyOptions) {
			o.OCSP = nil
			o.CRLs = []*x509.RevocationList{p.CRL(t, at, at.Add(-time.Minute), false, leaf)}
		}), "00101:error"},
		{"00101 suspended", fpa, base, opts(func(o *VerifyOptions) {
			o.OCSP = nil
			o.CRLs = []*x509.RevocationList{p.CRL(t, at, at.Add(-time.Minute), true, leaf)}
		}), "00101:error"},
		{"00101 revoked, OCSP", fpa, base, opts(func(o *VerifyOptions) {
			o.OCSP = [][]byte{p.OCSP(t, leaf, sigtest.OCSPRevoked, at.Add(-time.Minute), at, false)}
		}), "00101:error"},
		{"00104 untrusted", fpa, base, opts(func(o *VerifyOptions) { o.Roots = sigtest.New(t).Roots }), "00104:error"},
		{"00105 signing time after the reception", fpa, with(base, func(s *SignOptions) { s.SigningTime = at.Add(time.Hour) }), opts(nil), "00105:error"},
		{"00107 not yet valid", fpa, with(base, func(s *SignOptions) { s.SigningTime = leaf.NotBefore.Add(-2 * time.Hour) }), opts(func(o *VerifyOptions) {
			o.At = leaf.NotBefore.Add(-time.Hour)
			o.OCSP = good(leaf, o.At)
		}), "00107:error"},
		{"IT-SIG-04 no roots", fpa, base, opts(func(o *VerifyOptions) { o.Roots = nil }), "IT-SIG-04:warning"},
		{"IT-SIG-05 no revocation data", fpa, base, opts(func(o *VerifyOptions) { o.OCSP = nil }), "IT-SIG-05:info"},
		{"00001 file name", fpa, base, opts(func(o *VerifyOptions) { o.Name = "fattura.xml" }), "00001:error"},
	}
	encipher, encOpts := leafOf(sigtest.LeafOptions{QC: sigtest.QcSignature, KeyUsage: x509.KeyUsageKeyEncipherment})
	cases = append(cases, tc{"00107 key usage", fpa, encOpts, opts(func(o *VerifyOptions) { o.OCSP = good(encipher[0], at) }), "00107:error"})
	plain, plainOpts := leafOf(sigtest.LeafOptions{QC: sigtest.QcNone})
	cases = append(cases,
		tc{"IT-SIG-02 not qualified, FPA12", fpa, plainOpts, opts(func(o *VerifyOptions) { o.OCSP = good(plain[0], at) }), "IT-SIG-02:warning"},
		tc{"not qualified, FPR12", fpr, plainOpts, opts(func(o *VerifyOptions) { o.OCSP = good(plain[0], at) }), ""})
	seal, sealOpts := leafOf(sigtest.LeafOptions{QC: sigtest.QcSeal, CommonName: "ACME S.p.A."})
	cases = append(cases,
		tc{"IT-SIG-03 seal, FPA12", fpa, sealOpts, opts(func(o *VerifyOptions) { o.OCSP = good(seal[0], at) }), "IT-SIG-03:warning"},
		tc{"IT-SIG-03 seal, FPR12", fpr, sealOpts, opts(func(o *VerifyOptions) { o.OCSP = good(seal[0], at) }), "IT-SIG-03:info"})

	for _, c := range cases {
		files := signBoth(t, c.doc, c.sign)
		for format, file := range files {
			o := *c.opts
			if o.Name == "" {
				ext := ExtXML
				if format == CAdES {
					ext = ExtP7M
				}
				o.Name = "IT01234567890_FPA01" + ext
			}
			if got := findings(VerifySignature(file, &o)); got != c.want {
				t.Errorf("%s, %v: %q, want %q\n%s", c.name, format, got, c.want, VerifySignature(file, &o))
			}
		}
	}

	// 00102: the file does not match the signature; nothing else is checked.
	files := signBoth(t, fpa, base)
	for format, file := range files {
		tampered := bytes.Replace(file, []byte("<Numero>"), []byte("<Numerx>"), 1)
		if format == XAdES {
			tampered = bytes.Replace(file, []byte("<Numero>"), []byte("<Numero> "), 1)
		}
		if got := findings(VerifySignature(tampered, opts(nil))); got != "00102:error" {
			t.Errorf("00102, %v: %q", format, got)
		}
	}
	// 00001: the extension does not match the format.
	if got := findings(VerifySignature(files[CAdES], opts(func(o *VerifyOptions) { o.Name = "IT01234567890_FPA01.xml" }))); got != "00001:error" {
		t.Errorf("00001, .xml for CAdES: %q", got)
	}
	if got := findings(VerifySignature(files[XAdES], opts(func(o *VerifyOptions) { o.Name = "IT01234567890_FPA01.xml.p7m" }))); got != "00001:error" {
		t.Errorf("00001, .xml.p7m for XAdES: %q", got)
	}
	if got := findings(VerifySignature(files[CAdES], opts(func(o *VerifyOptions) { o.Name = "IT01234567890_FPA01.zip" }))); got != "00001:error" {
		t.Errorf("00001, .zip: %q", got)
	}
	// 00103: a CAdES signature without signing time.
	noTime, err := cms.Sign(fpa, &cms.SignOptions{Signer: base.Signer, Chain: chain})
	if err != nil {
		t.Fatal(err)
	}
	if got := findings(VerifySignature(noTime, opts(nil))); got != "00103:error" {
		t.Errorf("00103: %q", got)
	}
	// IT-SIG-01: unsigned files.
	if got := findings(VerifySignature(fpa, opts(nil))); got != "IT-SIG-01:error" {
		t.Errorf("IT-SIG-01, FPA12: %q", got)
	}
	if got := findings(VerifySignature(fpr, opts(func(o *VerifyOptions) { o.Name = "IT01234567890_FPR01.xml.p7m" }))); got != "00001:error IT-SIG-01:info" {
		t.Errorf("IT-SIG-01, FPR12 named .xml.p7m: %q", got)
	}
	// IT-SIG-07: the signed content is not FatturaPA.
	other, err := signAny([]byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`), base.Signer, chain)
	if err != nil {
		t.Fatal(err)
	}
	if got := findings(VerifySignature(other, opts(nil))); got != "IT-SIG-07:error" {
		t.Errorf("IT-SIG-07: %q", got)
	}
	// Garbage is a signature that cannot be read.
	if got := findings(VerifySignature([]byte("garbage"), opts(nil))); got != "00102:error" {
		t.Errorf("garbage: %q", got)
	}
}

// TestVerifyNested checks a .p7m signed again: each signature is checked
// and named in the messages.
func TestVerifyNested(t *testing.T) {
	fpa := readTestdata(t, "fpa12.xml")
	p := sigtest.New(t)
	at := sigtest.Epoch
	signer, key := p.Leaf(t, sigtest.LeafOptions{QC: sigtest.QcSignature})
	seal, sealKey := p.Leaf(t, sigtest.LeafOptions{QC: sigtest.QcSeal, Key: sigtest.ECKey(t), CommonName: "ACME S.p.A."})
	inner, err := SignCAdES(fpa, &SignOptions{Signer: key, Chain: []*x509.Certificate{signer, p.Intermediate}, SigningTime: at.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := signAny(inner, sealKey, []*x509.Certificate{seal, p.Intermediate})
	if err != nil {
		t.Fatal(err)
	}
	r := VerifySignature(outer, &VerifyOptions{Roots: p.Roots, At: at, OCSP: [][]byte{
		p.OCSP(t, signer, sigtest.OCSPGood, time.Time{}, at, false),
		p.OCSP(t, seal, sigtest.OCSPGood, time.Time{}, at, true),
	}})
	if got := findings(r); got != "IT-SIG-03:warning" {
		t.Fatalf("nested: %q\n%s", got, r)
	}
	if msg := r.Findings[0].Message; !strings.HasPrefix(msg, "signature 1 of 2: ") {
		t.Errorf("message %q", msg)
	}
}

func TestBaselineDescriptions(t *testing.T) {
	sd := &cms.SignedData{}
	if s := cadesSigner(sd, &cms.SignerInfo{}); len(s.Baseline) != 1 {
		t.Errorf("CMS without signing-certificate-v2: %v", s.Baseline)
	}
	if s := cadesSigner(sd, &cms.SignerInfo{SigningCert: &cms.CertRef{}}); len(s.Baseline) != 1 {
		t.Errorf("CMS with signing-certificate (v1): %v", s.Baseline)
	}
	for _, c := range []struct {
		res  xmldsig.Result
		want int
	}{
		{xmldsig.Result{}, 1},
		{xmldsig.Result{XAdES: true}, 2},
		{xmldsig.Result{XAdES: true, SigningCertificate: "SigningCertificate", DataObjectFormat: true}, 1},
		{xmldsig.Result{XAdES: true, SigningCertificate: "SigningCertificateV2", DataObjectFormat: true}, 0},
	} {
		if s := xadesSigner(&c.res); len(s.Baseline) != c.want {
			t.Errorf("%+v: %v", c.res, s.Baseline)
		}
	}
	for _, k := range []Kind{KindUnknown, KindSignature, KindSeal} {
		if k.String() == "" {
			t.Errorf("Kind(%d)", k)
		}
	}
	if Format(0).String() != "unknown" {
		t.Error("Format(0)")
	}
}

func TestSignatureRulesCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range SignatureRules {
		if seen[r.ID] || r.Description == "" || !strings.HasSuffix(r.Description, ".") {
			t.Errorf("rule %s", r.ID)
		}
		seen[r.ID] = true
	}
	for _, id := range []string{RuleFileName, RuleCertExpired, RuleCertRevoked, RuleSignatureInvalid, RuleNoSigningTime,
		RuleUntrustedCA, RuleSigningTimeAfter, RuleCertInvalid, RuleNotSigned, RuleNotQualified, RuleSeal,
		RuleTrustNotChecked, RuleRevocationUnknown, RuleNotBaselineB, RuleNotFatturaPA} {
		if !seen[id] {
			t.Errorf("rule %s not in SignatureRules", id)
		}
	}
}

func FuzzUnwrap(f *testing.F) {
	doc, err := readFile("fpr12.xml")
	if err != nil {
		f.Fatal(err)
	}
	p := sigtest.New(f)
	cert, key := p.Leaf(f, sigtest.LeafOptions{QC: sigtest.QcSignature})
	opts := &SignOptions{Signer: key, Chain: []*x509.Certificate{cert}, SigningTime: sigtest.Epoch}
	p7m, err := SignCAdES(doc, opts)
	if err != nil {
		f.Fatal(err)
	}
	signed, err := SignXAdES(doc, opts)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(p7m)
	f.Add(signed)
	f.Add([]byte(base64.StdEncoding.EncodeToString(p7m)))
	f.Add([]byte("-----BEGIN PKCS7-----\nMAA=\n-----END PKCS7-----\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, sig, err := Unwrap(data)
		if err == nil && (got == nil || sig == nil || len(sig.Signers) == 0) {
			t.Fatalf("Unwrap succeeded with content %v, signature %v", got, sig)
		}
		_ = VerifySignature(data, &VerifyOptions{At: sigtest.Epoch, Name: "IT01234567890_1.xml"})
	})
}

func FuzzCheckFileName(f *testing.F) {
	for _, s := range []string{"IT01234567890_FPA01.xml", "ITAAABBB99T99X999W_00001.xml.p7m", "DE123_a.zip", "X_1.xml", "_.xml", "IT_.xml.p7m"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if err := CheckFileName(name); err != nil {
			return
		}
		if !strings.HasSuffix(name, ExtXML) && !strings.HasSuffix(name, ExtP7M) && !strings.HasSuffix(name, ExtZIP) {
			t.Fatalf("%q accepted without a valid extension", name)
		}
	})
}

func readFile(name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		return nil, fmt.Errorf("testdata: %w", err)
	}
	return data, nil
}
