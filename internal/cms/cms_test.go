package cms

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"

	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

var content = []byte(`<?xml version="1.0" encoding="UTF-8"?><p:FatturaElettronica versione="FPR12"/>`)

func sign(t *testing.T, o *SignOptions) []byte {
	t.Helper()
	der, err := Sign(content, o)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return der
}

func parse1(t *testing.T, der []byte) (*SignedData, *SignerInfo) {
	t.Helper()
	sd, err := Parse(der)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sd.Signers) != 1 {
		t.Fatalf("%d signers", len(sd.Signers))
	}
	return sd, sd.Signers[0]
}

func TestSignParseVerify(t *testing.T) {
	p := sigtest.New(t)
	for _, c := range []struct {
		name   string
		key    crypto.Signer
		pss    bool
		scheme sigalg.Scheme
	}{
		{"RSA", nil, false, sigalg.PKCS1v15},
		{"PSS", nil, true, sigalg.PSS},
		{"ECDSA", sigtest.ECKey(t), false, sigalg.ECDSA},
	} {
		t.Run(c.name, func(t *testing.T) {
			leaf, key := p.Leaf(t, sigtest.LeafOptions{Key: c.key})
			der := sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf, p.Intermediate}, PSS: c.pss, SigningTime: sigtest.Epoch, MimeType: "text/xml"})
			if n, err := normalize(der); err != nil || !bytes.Equal(n, der) {
				t.Errorf("normalize changes a DER encoding: %v", err)
			}
			sd, s := parse1(t, der)
			if !sd.ContentType.Equal(OIDData) || sd.Detached || !bytes.Equal(sd.Content, content) {
				t.Errorf("content type %v, detached %v, content %q", sd.ContentType, sd.Detached, sd.Content)
			}
			if len(sd.Certificates) != 2 || !s.Certificate.Equal(leaf) {
				t.Fatalf("%d certificates, signer %v", len(sd.Certificates), s.Certificate)
			}
			if s.Algorithm.Scheme != c.scheme || s.Algorithm.Hash != crypto.SHA256 || s.AlgorithmErr != nil {
				t.Errorf("algorithm %v, %v", s.Algorithm, s.AlgorithmErr)
			}
			if !s.HasSignedAttrs || s.AttrErr != nil || !s.ContentType.Equal(OIDData) {
				t.Errorf("signed attributes %v, %v, content type %v", s.HasSignedAttrs, s.AttrErr, s.ContentType)
			}
			if !s.SigningTime.Equal(sigtest.Epoch) || s.MimeType != "text/xml" {
				t.Errorf("signing time %v, mime type %q", s.SigningTime, s.MimeType)
			}
			if s.SigningCert == nil || !s.SigningCert.V2 || !s.SigningCert.Matches(leaf) || s.SigningCert.Matches(p.Intermediate) {
				t.Errorf("signing certificate %+v", s.SigningCert)
			}
			if err := sd.VerifyAll(nil); err != nil {
				t.Errorf("VerifyAll: %v", err)
			}
			if chain := sd.Chain(s.Certificate); len(chain) != 1 || !chain[0].Equal(p.Intermediate) {
				t.Errorf("Chain = %v", chain)
			}
		})
	}
}

func TestSignOptions(t *testing.T) {
	p := sigtest.New(t)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{})
	_, s := parse1(t, sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, Hash: crypto.SHA512}))
	if !s.SigningTime.IsZero() || s.MimeType != "" || s.Algorithm.Hash != crypto.SHA512 {
		t.Errorf("signing time %v, mime type %q, hash %v", s.SigningTime, s.MimeType, s.Algorithm.Hash)
	}
	other, _ := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	for name, o := range map[string]*SignOptions{
		"no options":     nil,
		"no signer":      {Chain: []*x509.Certificate{leaf}},
		"no certificate": {Signer: key},
		"another key":    {Signer: key, Chain: []*x509.Certificate{other}},
		"SHA-1":          {Signer: key, Chain: []*x509.Certificate{leaf}, Hash: crypto.SHA1},
	} {
		if _, err := Sign(content, o); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestDetached(t *testing.T) {
	p := sigtest.New(t)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{})
	sd, _ := parse1(t, sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, SigningTime: sigtest.Epoch, Detached: true}))
	if !sd.Detached || sd.Content != nil {
		t.Fatalf("detached %v, content %q", sd.Detached, sd.Content)
	}
	if err := sd.VerifyAll(nil); err == nil {
		t.Error("a detached signature verifies without its content")
	}
	if err := sd.VerifyAll(content); err != nil {
		t.Errorf("VerifyAll: %v", err)
	}
	if err := sd.VerifyAll([]byte("other")); !errors.Is(err, ErrDigest) {
		t.Errorf("VerifyAll with other content: %v", err)
	}
}

func TestTampering(t *testing.T) {
	p := sigtest.New(t)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{})
	der := sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, SigningTime: sigtest.Epoch})

	changed := bytes.Replace(der, []byte("FPR12"), []byte("FPA12"), 1)
	if sd, _ := parse1(t, changed); !errors.Is(sd.VerifyAll(nil), ErrDigest) {
		t.Errorf("modified content: %v", sd.VerifyAll(nil))
	}
	// The signature value ends the SignerInfo, and the file.
	changed = bytes.Clone(der)
	changed[len(changed)-1] ^= 1
	if sd, _ := parse1(t, changed); !errors.Is(sd.VerifyAll(nil), ErrSignature) {
		t.Errorf("modified signature value: %v", sd.VerifyAll(nil))
	}
	// The signing time is a signed attribute.
	tm, _ := asn1.Marshal(sigtest.Epoch)
	tm2, _ := asn1.Marshal(sigtest.Epoch.Add(1e9))
	changed = bytes.Replace(der, tm, tm2, 1)
	if bytes.Equal(changed, der) {
		t.Fatal("signing time not found")
	}
	if sd, _ := parse1(t, changed); !errors.Is(sd.VerifyAll(nil), ErrSignature) {
		t.Errorf("modified signing time: %v", sd.VerifyAll(nil))
	}
}

// replaceSID makes the SignerInfo designate the certificate of serial to:
// the SignerInfo, after the certificates, holds the last occurrence of the
// serial number.
func replaceSID(t *testing.T, der []byte, from, to *big.Int) []byte {
	t.Helper()
	a, _ := asn1.Marshal(from)
	b, _ := asn1.Marshal(to)
	i := bytes.LastIndex(der, a)
	if i < 0 || len(a) != len(b) {
		t.Fatalf("cannot replace serial %v by %v", from, to)
	}
	out := bytes.Clone(der)
	copy(out[i:], b)
	return out
}

func TestSigningCertificate(t *testing.T) {
	p := sigtest.New(t)
	key := sigtest.RSAKey(t)
	leaf1, _ := p.Leaf(t, sigtest.LeafOptions{Key: key})
	leaf2, _ := p.Leaf(t, sigtest.LeafOptions{Key: key, CommonName: "Luigi Bianchi"})
	der := sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf1, leaf2}, SigningTime: sigtest.Epoch})

	// The SignerInfo designates the other certificate of the same key: the
	// signature verifies, but signing-certificate-v2 does not match.
	sd, s := parse1(t, replaceSID(t, der, leaf1.SerialNumber, leaf2.SerialNumber))
	if !s.Certificate.Equal(leaf2) {
		t.Fatalf("signer %v", s.Certificate.Subject)
	}
	if err := sd.VerifyAll(nil); !errors.Is(err, ErrSigningCertificate) {
		t.Errorf("VerifyAll: %v, want ErrSigningCertificate", err)
	}

	// The SignerInfo designates no certificate of the SignedData.
	unknown := new(big.Int).Add(leaf2.SerialNumber, big.NewInt(1))
	sd, s = parse1(t, replaceSID(t, der, leaf1.SerialNumber, unknown))
	if s.Certificate != nil || !errors.Is(sd.VerifyAll(nil), ErrNoCertificate) {
		t.Errorf("unknown signer: %v, %v", s.Certificate, sd.VerifyAll(nil))
	}
}

// toBER re-encodes DER as a streaming encoder writes BER: every constructed
// element with an indefinite length, and OCTET STRINGs cut into chunks.
func toBER(t *testing.T, der []byte) []byte {
	t.Helper()
	var out []byte
	for len(der) > 0 {
		var v asn1.RawValue
		rest, err := asn1.Unmarshal(der, &v)
		if err != nil {
			t.Fatal(err)
		}
		hdr := v.FullBytes[:len(v.FullBytes)-len(v.Bytes)]
		switch {
		case v.IsCompound:
			out = append(out, hdr[0], 0x80)
			out = append(out, toBER(t, v.Bytes)...)
			out = append(out, 0, 0)
		case v.Class == asn1.ClassUniversal && v.Tag == asn1.TagOctetString && len(v.Bytes) > 16:
			out = append(out, 0x24, 0x80)
			for b := v.Bytes; len(b) > 0; {
				n := min(16, len(b))
				out = append(out, 0x04, byte(n))
				out = append(out, b[:n]...)
				b = b[n:]
			}
			out = append(out, 0, 0)
		default:
			out = append(out, v.FullBytes...)
		}
		der = rest
	}
	return out
}

func TestBER(t *testing.T) {
	p := sigtest.New(t)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	der := sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf, p.Intermediate}, SigningTime: sigtest.Epoch})
	ber := toBER(t, der)
	if bytes.Equal(ber, der) {
		t.Fatal("toBER changed nothing")
	}
	n, err := normalize(ber)
	if err != nil || !bytes.Equal(n, der) {
		t.Fatalf("normalize does not restore the DER encoding: %v", err)
	}
	sd, s := parse1(t, ber)
	if !bytes.Equal(sd.Content, content) || !s.Certificate.Equal(leaf) {
		t.Errorf("content %q, signer %v", sd.Content, s.Certificate)
	}
	if err := sd.VerifyAll(nil); err != nil {
		t.Errorf("VerifyAll: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	p := sigtest.New(t)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{})
	der := sign(t, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, SigningTime: sigtest.Epoch})
	data, _ := asn1.Marshal(contentInfo{ContentType: OIDData, Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: []byte{0x04, 0x00}}})
	for name, b := range map[string][]byte{
		"empty":            nil,
		"garbage":          []byte("not a signature"),
		"truncated":        der[:len(der)/2],
		"trailing data":    append(bytes.Clone(der), 1, 2, 3),
		"not signed data":  data,
		"deep nesting":     bytes.Repeat([]byte{0x30, 0x80}, 200),
		"huge length":      {0x30, 0x84, 0x7f, 0xff, 0xff, 0xff, 0x00},
		"unterminated BER": {0x30, 0x80, 0x02, 0x01, 0x00},
		"bad tag":          {0x1f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00},
	} {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// Zero padding after the ContentInfo is tolerated.
	if _, err := Parse(append(bytes.Clone(der), 0, 0, 0)); err != nil {
		t.Errorf("zero padding: %v", err)
	}
}

func FuzzParse(f *testing.F) {
	p := sigtest.New(f)
	leaf, key := p.Leaf(f, sigtest.LeafOptions{})
	der, err := Sign(content, &SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, SigningTime: sigtest.Epoch})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(der)
	f.Add([]byte{0x30, 0x80, 0x24, 0x80, 0x04, 0x01, 0x41, 0x00, 0x00, 0x00, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		sd, err := Parse(data)
		if err != nil {
			return
		}
		_ = sd.VerifyAll(content)
		for _, s := range sd.Signers {
			_ = s.SigningCert.Matches(s.Certificate)
		}
	})
}
