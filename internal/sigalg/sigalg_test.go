package sigalg

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"testing"

	"github.com/ritoon/zerechnung/internal/sigtest"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	rsaKey := sigtest.RSAKey(t)
	ecKey := sigtest.ECKey(t)
	for _, h := range []crypto.Hash{crypto.SHA256, crypto.SHA384, crypto.SHA512} {
		for _, c := range []struct {
			name string
			key  crypto.Signer
			pss  bool
		}{{"RSA", rsaKey, false}, {"PSS", rsaKey, true}, {"ECDSA", ecKey, false}} {
			t.Run(c.name+"/"+h.String(), func(t *testing.T) {
				a, err := For(c.key.Public(), h, c.pss)
				if err != nil {
					t.Fatal(err)
				}
				digest := Digest(h, []byte("fattura"))
				sig, err := Sign(c.key, a, digest)
				if err != nil {
					t.Fatal(err)
				}
				if err := Verify(c.key.Public(), a, digest, sig); err != nil {
					t.Fatalf("Verify: %v", err)
				}
				if err := Verify(c.key.Public(), a, Digest(h, []byte("other")), sig); err == nil {
					t.Error("a signature of another digest verifies")
				}
				ai, err := a.AlgorithmIdentifier()
				if err != nil {
					t.Fatal(err)
				}
				back, err := FromAlgorithmIdentifier(ai, h)
				if err != nil || back != a {
					t.Errorf("FromAlgorithmIdentifier = %v, %v, want %v", back, err, a)
				}
			})
		}
	}
}

func TestFor(t *testing.T) {
	if a, err := For(sigtest.RSAKey(t).Public(), 0, false); err != nil || a.Hash != crypto.SHA256 || a.Scheme != PKCS1v15 {
		t.Errorf("For with no hash = %v, %v, want RSA with SHA-256", a, err)
	}
	if _, err := For(sigtest.RSAKey(t).Public(), crypto.SHA1, false); !errors.Is(err, ErrUnsupported) {
		t.Errorf("For with SHA-1: %v", err)
	}
	if _, err := For(sigtest.ECKey(t).Public(), crypto.SHA256, true); !errors.Is(err, ErrUnsupported) {
		t.Errorf("For with PSS and an ECDSA key: %v", err)
	}
	if _, err := For("key", crypto.SHA256, false); !errors.Is(err, ErrUnsupported) {
		t.Errorf("For with a key of another type: %v", err)
	}
	if !(Algorithm{Scheme: ECDSA, Hash: crypto.SHA1}).Weak() || (Algorithm{Scheme: ECDSA, Hash: crypto.SHA256}).Weak() {
		t.Error("Weak")
	}
}

// pssIdentifier returns RSASSA-PSS parameters with the given hash for the
// message and for MGF1, salt length and trailer field.
func pssIdentifier(t *testing.T, h, mgf asn1.ObjectIdentifier, salt, trailer int) pkix.AlgorithmIdentifier {
	t.Helper()
	hashAI := pkix.AlgorithmIdentifier{Algorithm: h, Parameters: asn1.NullRawValue}
	mgfParams, err := asn1.Marshal(pkix.AlgorithmIdentifier{Algorithm: mgf, Parameters: asn1.NullRawValue})
	if err != nil {
		t.Fatal(err)
	}
	params, err := asn1.Marshal(pssParams{
		Hash:         hashAI,
		MGF:          pkix.AlgorithmIdentifier{Algorithm: OIDMGF1, Parameters: asn1.RawValue{FullBytes: mgfParams}},
		SaltLength:   salt,
		TrailerField: trailer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.AlgorithmIdentifier{Algorithm: OIDRSAPSS, Parameters: asn1.RawValue{FullBytes: params}}
}

// TestPSSSaltLength checks that an RSA-PSS signature verifies with the salt
// length its parameters declare: OpenSSL 3.0 signs with the longest salt
// the key allows.
func TestPSSSaltLength(t *testing.T) {
	key := sigtest.RSAKey(t)
	digest := Digest(crypto.SHA256, []byte("fattura"))
	maxSalt := key.Size() - crypto.SHA256.Size() - 2
	sig, err := rsa.SignPSS(rand.Reader, key, crypto.SHA256, digest, &rsa.PSSOptions{SaltLength: maxSalt})
	if err != nil {
		t.Fatal(err)
	}
	a, err := FromAlgorithmIdentifier(pssIdentifier(t, OIDSHA256, OIDSHA256, maxSalt, 1), crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if a.SaltLength != maxSalt {
		t.Fatalf("salt length %d, want %d", a.SaltLength, maxSalt)
	}
	if err := Verify(key.Public(), a, digest, sig); err != nil {
		t.Errorf("Verify with the declared salt: %v", err)
	}
	if err := Verify(key.Public(), Algorithm{Scheme: PSS, Hash: crypto.SHA256}, digest, sig); err == nil {
		t.Error("a signature with a longer salt verifies with a salt as long as the hash")
	}
	if a.String() != "RSA-PSS with SHA-256, salt of 222 bytes" {
		t.Errorf("String = %q", a.String())
	}
	// Signing with the declared salt writes it back.
	sig, err = Sign(key, a, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPSS(&key.PublicKey, crypto.SHA256, digest, sig, &rsa.PSSOptions{SaltLength: maxSalt}); err != nil {
		t.Error(err)
	}
	ai, err := a.AlgorithmIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	if back, err := FromAlgorithmIdentifier(ai, 0); err != nil || back != a {
		t.Errorf("round trip = %v, %v", back, err)
	}

	for name, ai := range map[string]pkix.AlgorithmIdentifier{
		"MGF1 with another hash": pssIdentifier(t, OIDSHA256, OIDSHA512, 32, 1),
		"no salt":                pssIdentifier(t, OIDSHA256, OIDSHA256, 0, 1),
		"trailer field 2":        pssIdentifier(t, OIDSHA256, OIDSHA256, 32, 2),
		"unknown hash":           pssIdentifier(t, asn1.ObjectIdentifier{1, 2, 3}, OIDSHA256, 32, 1),
		"no parameters":          {Algorithm: OIDRSAPSS},
		"bad parameters":         {Algorithm: OIDRSAPSS, Parameters: asn1.RawValue{FullBytes: []byte{0x30, 0x03, 0x01}}},
	} {
		if _, err := FromAlgorithmIdentifier(ai, 0); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := FromAlgorithmIdentifier(pssIdentifier(t, OIDSHA256, OIDSHA256, 32, 1), crypto.SHA512); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PSS with SHA-256 and digest SHA-512: %v", err)
	}
}

func TestFromAlgorithmIdentifier(t *testing.T) {
	for _, c := range []struct {
		oid    asn1.ObjectIdentifier
		digest crypto.Hash
		want   Algorithm
	}{
		{OIDRSAEncryption, crypto.SHA256, Algorithm{Scheme: PKCS1v15, Hash: crypto.SHA256}},
		{OIDECPublicKey, crypto.SHA384, Algorithm{Scheme: ECDSA, Hash: crypto.SHA384}},
		{OIDSHA1WithRSA, 0, Algorithm{Scheme: PKCS1v15, Hash: crypto.SHA1}},
		{OIDSHA384WithRSA, crypto.SHA384, Algorithm{Scheme: PKCS1v15, Hash: crypto.SHA384}},
		{OIDSHA512WithRSA, 0, Algorithm{Scheme: PKCS1v15, Hash: crypto.SHA512}},
		{OIDECDSAWithSHA1, 0, Algorithm{Scheme: ECDSA, Hash: crypto.SHA1}},
		{OIDECDSAWithSHA512, 0, Algorithm{Scheme: ECDSA, Hash: crypto.SHA512}},
	} {
		got, err := FromAlgorithmIdentifier(pkix.AlgorithmIdentifier{Algorithm: c.oid}, c.digest)
		if err != nil || got != c.want {
			t.Errorf("%v: %v, %v, want %v", c.oid, got, err, c.want)
		}
	}
	for name, c := range map[string]struct {
		oid    asn1.ObjectIdentifier
		digest crypto.Hash
	}{
		"rsaEncryption without digest":      {OIDRSAEncryption, 0},
		"sha256WithRSA with digest SHA-384": {OIDSHA256WithRSA, crypto.SHA384},
		"unknown":                           {asn1.ObjectIdentifier{1, 2, 3, 4}, crypto.SHA256},
	} {
		if _, err := FromAlgorithmIdentifier(pkix.AlgorithmIdentifier{Algorithm: c.oid}, c.digest); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := (Algorithm{Scheme: ECDSA, Hash: crypto.SHA1}).AlgorithmIdentifier(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ECDSA with SHA-1 identifier: %v", err)
	}
	if err := Verify(sigtest.ECKey(t).Public(), Algorithm{Scheme: PKCS1v15, Hash: crypto.SHA256}, nil, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("RSA algorithm with an ECDSA key: %v", err)
	}
}

func TestECDSARaw(t *testing.T) {
	key := sigtest.ECKey(t)
	digest := Digest(crypto.SHA256, []byte("fattura"))
	a := Algorithm{Scheme: ECDSA, Hash: crypto.SHA256}
	sig, err := Sign(key, a, digest)
	if err != nil {
		t.Fatal(err)
	}
	size := ECDSASize(&key.PublicKey)
	if size != 32 {
		t.Fatalf("ECDSASize = %d", size)
	}
	raw, err := ECDSARaw(sig, size)
	if err != nil || len(raw) != 2*size {
		t.Fatalf("ECDSARaw: %d bytes, %v", len(raw), err)
	}
	back, err := ECDSAFromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(key.Public(), a, digest, back); err != nil {
		t.Errorf("the converted signature does not verify: %v", err)
	}
	if _, err := ECDSARaw([]byte{1, 2}, size); err == nil {
		t.Error("ECDSARaw accepts garbage")
	}
	if _, err := ECDSAFromRaw([]byte{1, 2, 3}); err == nil {
		t.Error("ECDSAFromRaw accepts an odd length")
	}
}
