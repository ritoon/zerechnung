package keys

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPKCS12(t *testing.T) {
	for _, tc := range []struct {
		file, password, leaf, alg string
		chain                     int
	}{
		{"modern.p12", "test1234", "Exemple Informatique SAS", "RSA 2048", 2},
		{"legacy.p12", "test1234", "Exemple Informatique SAS", "RSA 2048", 2},
		{"ec.p12", "pässwörd€", "Exemple EC", "ECDSA P-256", 2},
		{"empty.p12", "", "Exemple Informatique SAS", "RSA 2048", 1},
	} {
		s, err := Load(&config.Sign{PKCS12: read(t, tc.file), PKCS12Password: tc.password})
		if err != nil {
			t.Errorf("%s: %v", tc.file, err)
			continue
		}
		if cn := s.Certificate().Subject.CommonName; cn != tc.leaf || len(s.Chain) != tc.chain || s.Algorithm() != tc.alg {
			t.Errorf("%s: leaf %q, chain %d, %s", tc.file, cn, len(s.Chain), s.Algorithm())
		}
		if len(s.Chain) == 2 && s.Chain[1].Subject.CommonName != "ZeRechnung Test CA" {
			t.Errorf("%s: issuer %q", tc.file, s.Chain[1].Subject.CommonName)
		}
		if err := s.CheckValidity(time.Now()); err != nil {
			t.Errorf("%s: %v", tc.file, err)
		}
	}
}

func TestPKCS12WrongPassword(t *testing.T) {
	for _, f := range []string{"modern.p12", "legacy.p12", "empty.p12"} {
		_, _, err := ParsePKCS12(read(t, f), "wrong")
		if !errors.Is(err, errBadPassword) {
			t.Errorf("%s: %v", f, err)
		}
	}
	if _, _, err := ParsePKCS12([]byte("not a pkcs12"), "x"); err == nil {
		t.Error("garbage: no error")
	}
}

func TestEncryptedPEM(t *testing.T) {
	for _, tc := range []struct{ key, cert string }{{"leaf-enc.pem", "leaf.crt"}, {"ec-enc.pem", "ec.crt"}} {
		cert := append(read(t, tc.cert), read(t, "ca.crt")...)
		s, err := Load(&config.Sign{Key: read(t, tc.key), KeyPassword: "test1234", Cert: cert})
		if err != nil {
			t.Errorf("%s: %v", tc.key, err)
			continue
		}
		if len(s.Chain) != 2 {
			t.Errorf("%s: chain of %d", tc.key, len(s.Chain))
		}
		if _, err := ParsePrivateKey(read(t, tc.key), "wrong"); err == nil {
			t.Errorf("%s: wrong password accepted", tc.key)
		}
		if _, err := ParsePrivateKey(read(t, tc.key), ""); err == nil || !strings.Contains(err.Error(), "ZR_SIGN_KEY_PASSWORD") {
			t.Errorf("%s: no password: %v", tc.key, err)
		}
	}
}

// selfSigned returns a key and its certificate, valid from notBefore for a
// day.
func selfSigned(t *testing.T, key any, notBefore time.Time) *x509.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: notBefore, NotAfter: notBefore.Add(24 * time.Hour)}
	pub := key.(crypto.Signer).Public()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return c
}

func TestPlainPEM(t *testing.T) {
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	ek, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p8, _ := x509.MarshalPKCS8PrivateKey(ek)
	sec1, _ := x509.MarshalECPrivateKey(ek)
	for name, block := range map[string]*pem.Block{
		"PKCS#1": {Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rk)},
		"PKCS#8": {Type: "PRIVATE KEY", Bytes: p8},
		"SEC 1":  {Type: "EC PRIVATE KEY", Bytes: sec1},
	} {
		k, err := ParsePrivateKey(append([]byte("junk before\n"), pem.EncodeToMemory(block)...), "")
		if err != nil || k == nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	legacy := &pem.Block{Type: "RSA PRIVATE KEY", Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-256-CBC,00"}, Bytes: []byte{1}}
	if _, err := ParsePrivateKey(pem.EncodeToMemory(legacy), "x"); err == nil || !strings.Contains(err.Error(), "pkcs8") {
		t.Errorf("legacy encryption: %v", err)
	}
}

func TestNewChecks(t *testing.T) {
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now()
	c := selfSigned(t, rk, now.Add(-time.Hour))
	if _, err := New(other, []*x509.Certificate{c}); err == nil {
		t.Error("certificate of another key accepted")
	}
	s, err := New(rk, []*x509.Certificate{c})
	if err != nil {
		t.Fatal(err)
	}
	if s.CheckValidity(now) != nil || s.CheckValidity(now.Add(48*time.Hour)) == nil || s.CheckValidity(now.Add(-48*time.Hour)) == nil {
		t.Error("CheckValidity")
	}
	// Unrelated certificates are dropped from the chain.
	modern, certs, err := ParsePKCS12(read(t, "modern.p12"), "test1234")
	if err != nil {
		t.Fatal(err)
	}
	s, err = New(modern, append([]*x509.Certificate{c}, certs...))
	if err != nil || len(s.Chain) != 2 {
		t.Errorf("chain %v %v", len(s.Chain), err)
	}
}

// RFC 2268, section 5.
func TestRC2Vectors(t *testing.T) {
	for _, tc := range []struct {
		key        string
		bits       int
		plain, out string
	}{
		{"0000000000000000", 63, "0000000000000000", "ebb773f993278eff"},
		{"ffffffffffffffff", 64, "ffffffffffffffff", "278b27e42e2f0d49"},
		{"3000000000000000", 64, "1000000000000001", "30649edf9be7d2c2"},
		{"88", 64, "0000000000000000", "61a8a244adacccf0"},
		{"88bca90e90875a", 64, "0000000000000000", "6ccf4308974c267f"},
		{"88bca90e90875a7f0f79c384627bafb2", 64, "0000000000000000", "1a807d272bbe5db1"},
		{"88bca90e90875a7f0f79c384627bafb2", 128, "0000000000000000", "2269552ab0f85ca6"},
	} {
		key, _ := hex.DecodeString(tc.key)
		plain, _ := hex.DecodeString(tc.plain)
		want, _ := hex.DecodeString(tc.out)
		c, err := newRC2(key, tc.bits)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 8)
		c.Encrypt(got, plain)
		if !bytes.Equal(got, want) {
			t.Errorf("key %s/%d: encrypt %x, want %x", tc.key, tc.bits, got, want)
		}
		c.Decrypt(got, want)
		if !bytes.Equal(got, plain) {
			t.Errorf("key %s/%d: decrypt %x, want %x", tc.key, tc.bits, got, plain)
		}
	}
}
