package fatturapa

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// signer returns a qualified signing certificate of a test PKI and its key.
func signer(t *testing.T, o sigtest.LeafOptions) (*sigtest.PKI, []*x509.Certificate, crypto.Signer) {
	t.Helper()
	p := sigtest.New(t)
	if o.QC == 0 {
		o.QC = sigtest.QcSignature
	}
	cert, key := p.Leaf(t, o)
	return p, []*x509.Certificate{cert, p.Intermediate}, key
}

var signingTime = sigtest.Epoch.Add(-time.Hour)

func signBoth(t *testing.T, doc []byte, opts *SignOptions) map[Format][]byte {
	t.Helper()
	p7m, err := SignCAdES(doc, opts)
	if err != nil {
		t.Fatalf("SignCAdES: %v", err)
	}
	signed, err := SignXAdES(doc, opts)
	if err != nil {
		t.Fatalf("SignXAdES: %v", err)
	}
	return map[Format][]byte{CAdES: p7m, XAdES: signed}
}

func TestSignAndUnwrap(t *testing.T) {
	doc := readTestdata(t, "fpa12.xml")
	for _, keyType := range []string{"RSA", "RSA-PSS", "ECDSA"} {
		t.Run(keyType, func(t *testing.T) {
			o := sigtest.LeafOptions{}
			if keyType == "ECDSA" {
				o.Key = sigtest.ECKey(t)
			}
			_, chain, key := signer(t, o)
			files := signBoth(t, doc, &SignOptions{Signer: key, Chain: chain, SigningTime: signingTime, PSS: keyType == "RSA-PSS"})
			for format, file := range files {
				got, sig, err := Unwrap(file)
				if err != nil {
					t.Fatalf("%v: Unwrap: %v", format, err)
				}
				if sig.Format != format || len(sig.Signers) != 1 {
					t.Fatalf("%v: signature %+v", format, sig)
				}
				s := sig.Signers[0]
				if !s.Certificate.Equal(chain[0]) || !s.SigningTime.Equal(signingTime.Truncate(time.Second)) {
					t.Errorf("%v: signer %v at %v", format, s.Certificate.Subject, s.SigningTime)
				}
				if s.Kind != KindSignature || !s.Qualified || !s.QSCD || len(s.Baseline) != 0 {
					t.Errorf("%v: kind %v, qualified %v, QSCD %v, baseline %v", format, s.Kind, s.Qualified, s.QSCD, s.Baseline)
				}
				if len(s.Certificates) != 1 || !s.Certificates[0].Equal(chain[1]) {
					t.Errorf("%v: chain %v", format, s.Certificates)
				}
				switch format {
				case CAdES:
					if !bytes.Equal(got, doc) {
						t.Errorf("CAdES: the unwrapped document differs from the signed one")
					}
				case XAdES:
					if !bytes.Equal(got, file) {
						t.Errorf("XAdES: Unwrap must return the file")
					}
					if _, err := Parse(got); err != nil {
						t.Errorf("XAdES: the signed file cannot be read: %v", err)
					}
				}
			}
		})
	}
}

func TestSignXAdESPlacement(t *testing.T) {
	doc := readTestdata(t, "fpr12.xml")
	_, chain, key := signer(t, sigtest.LeafOptions{})
	signed, err := SignXAdES(doc, &SignOptions{Signer: key, Chain: chain, SigningTime: signingTime})
	if err != nil {
		t.Fatal(err)
	}
	end := bytes.LastIndex(signed, []byte("</"))
	sigEnd := bytes.LastIndex(signed, []byte("</ds:Signature>"))
	if sigEnd < 0 || sigEnd+len("</ds:Signature>") != end {
		t.Fatalf("ds:Signature is not the last child of the root:\n%s", signed[max(0, end-200):])
	}
	if _, err := SignXAdES(signed, &SignOptions{Signer: key, Chain: chain}); err == nil {
		t.Error("SignXAdES signed a document twice")
	}
	if _, err := SignXAdES([]byte(`<Invoice/>`), &SignOptions{Signer: key, Chain: chain}); err == nil {
		t.Error("SignXAdES signed a document that is not FatturaPA")
	}
	if _, err := SignCAdES([]byte(`not xml`), &SignOptions{Signer: key, Chain: chain}); err == nil {
		t.Error("SignCAdES signed a document that is not FatturaPA")
	}
	other := sigtest.ECKey(t)
	if _, err := SignCAdES(doc, &SignOptions{Signer: other, Chain: chain}); err == nil {
		t.Error("SignCAdES accepted a key that does not match the certificate")
	}
}

func TestSigningTimeDefaultsToNow(t *testing.T) {
	doc := readTestdata(t, "fpr12.xml")
	_, chain, key := signer(t, sigtest.LeafOptions{NotAfter: time.Now().AddDate(1, 0, 0)})
	before := time.Now().Add(-time.Second)
	files := signBoth(t, doc, &SignOptions{Signer: key, Chain: chain})
	for format, file := range files {
		_, sig, err := Unwrap(file)
		if err != nil {
			t.Fatal(err)
		}
		if st := sig.Signers[0].SigningTime; st.Before(before) || st.After(time.Now().Add(time.Second)) {
			t.Errorf("%v: signing time %v, want about now", format, st)
		}
	}
}

func TestUnwrapEncodings(t *testing.T) {
	doc := readTestdata(t, "fpa12.xml")
	_, chain, key := signer(t, sigtest.LeafOptions{})
	p7m, err := SignCAdES(doc, &SignOptions{Signer: key, Chain: chain, SigningTime: signingTime})
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(p7m)
	var wrapped strings.Builder
	for i := 0; i < len(b64); i += 64 {
		wrapped.WriteString(b64[i:min(i+64, len(b64))])
		wrapped.WriteString("\r\n")
	}
	encodings := map[string][]byte{
		"DER":                p7m,
		"base64":             []byte(b64),
		"base64, CRLF lines": []byte(wrapped.String()),
		"PEM":                pem.EncodeToMemory(&pem.Block{Type: "PKCS7", Bytes: p7m}),
		"base64, no padding": []byte(strings.TrimRight(b64, "=")),
	}
	for name, file := range encodings {
		got, _, err := Unwrap(file)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, doc) {
			t.Errorf("%s: wrong content", name)
		}
	}

	// Nested envelopes: a .p7m signed again by a second signer.
	_, chain2, key2 := signer(t, sigtest.LeafOptions{Key: sigtest.ECKey(t), QC: sigtest.QcSeal, CommonName: "ACME S.p.A."})
	outer, err := signAny(p7m, key2, chain2)
	if err != nil {
		t.Fatal(err)
	}
	got, sig, err := Unwrap(outer)
	if err != nil {
		t.Fatalf("nested: %v", err)
	}
	if !bytes.Equal(got, doc) || len(sig.Signers) != 2 {
		t.Fatalf("nested: %d signers", len(sig.Signers))
	}
	if sig.Signers[0].Kind != KindSeal || sig.Signers[1].Kind != KindSignature {
		t.Errorf("nested: kinds %v, %v", sig.Signers[0].Kind, sig.Signers[1].Kind)
	}
}

func TestUnwrapRefusesTampering(t *testing.T) {
	doc := readTestdata(t, "fpa12.xml")
	_, chain, key := signer(t, sigtest.LeafOptions{})
	files := signBoth(t, doc, &SignOptions{Signer: key, Chain: chain, SigningTime: signingTime})

	// The document changed after the signature.
	tampered := bytes.Replace(files[CAdES], []byte("<Numero>"), []byte("<Numerx>"), 1)
	if bytes.Equal(tampered, files[CAdES]) {
		t.Fatal("test document without <Numero>")
	}
	if _, _, err := Unwrap(tampered); err == nil {
		t.Error("CAdES: a modified document is accepted")
	}
	tampered = bytes.Replace(files[XAdES], []byte("<Numero>"), []byte("<Numero> "), 1)
	if _, _, err := Unwrap(tampered); err == nil {
		t.Error("XAdES: a modified document is accepted")
	}

	if _, _, err := Unwrap(doc); !errors.Is(err, ErrNotSigned) {
		t.Errorf("unsigned document: %v, want ErrNotSigned", err)
	}
	if _, _, err := Unwrap([]byte("garbage")); err == nil {
		t.Error("garbage accepted")
	}
}

func TestFileName(t *testing.T) {
	good := map[string]bool{
		"IT01234567890_FPA01.xml":         true,
		"ITAAABBB99T99X999W_00001.xml":    true,
		"IT99999999999_00002.xml.p7m":     true,
		"ITAAABBB99T99X999W_00001.zip":    true,
		"DE123456789_a1.xml":              true,
		"FR12_1.xml":                      true,
		"dir/IT01234567890_FPA01.xml.p7m": true,
	}
	bad := []string{
		"IT0123456789_00001.xml",                // 10 characters for IT
		"IT01234567890123456_1.xml",             // 17 characters
		"IT01234567890_000001.xml",              // progressive of 6 characters
		"IT01234567890_0000-.xml",               // forbidden character
		"IT01234567890-00001.xml",               // no underscore
		"IT01234567890_00_01.xml",               // two underscores
		"it01234567890_00001.xml",               // lower case country
		"IT01234567890_00001.XML",               // upper case extension
		"IT01234567890_00001.p7m",               // .p7m without .xml
		"IT01234567890_.xml",                    // empty progressive
		"X_1.xml",                               // no country
		"DE12345678901234567890123456789_1.xml", // 29 characters
	}
	for name := range good {
		if err := CheckFileName(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range bad {
		if err := CheckFileName(name); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	name, err := FileName("IT", "99999999999", "00002", true)
	if err != nil || name != "IT99999999999_00002.xml.p7m" {
		t.Errorf("FileName = %q, %v", name, err)
	}
	if _, err := FileName("IT", "123", "1", false); err == nil {
		t.Error("FileName accepted a short Italian identifier")
	}
}

// signAny signs any content in CAdES, as a second signer of a nested
// envelope does.
func signAny(content []byte, key crypto.Signer, chain []*x509.Certificate) ([]byte, error) {
	return cms.Sign(content, &cms.SignOptions{Signer: key, Chain: chain, SigningTime: signingTime})
}
