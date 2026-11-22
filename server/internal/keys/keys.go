// Package keys loads the signing key and its certificates: PEM files
// (PKCS#1, SEC 1, PKCS#8, encrypted PKCS#8) or a PKCS#12 file (.p12,
// .pfx), as certification authorities deliver them and as Windows exports
// them.
package keys

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

// Signer is a signing key with its certificate chain.
type Signer struct {
	Key crypto.Signer
	// Chain is the signing certificate first, then the authorities that
	// issued it, as far as they were given.
	Chain []*x509.Certificate
}

// Certificate returns the signing certificate.
func (s *Signer) Certificate() *x509.Certificate { return s.Chain[0] }

// Algorithm describes the key: "RSA 3072", "ECDSA P-256".
func (s *Signer) Algorithm() string {
	switch k := s.Key.Public().(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d", k.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + k.Curve.Params().Name
	}
	return fmt.Sprintf("%T", s.Key.Public())
}

// CheckValidity returns an error when the signing certificate is not valid
// at t.
func (s *Signer) CheckValidity(t time.Time) error {
	c := s.Certificate()
	switch {
	case t.Before(c.NotBefore):
		return fmt.Errorf("the signing certificate is valid from %s", c.NotBefore.UTC().Format(time.RFC3339))
	case t.After(c.NotAfter):
		return fmt.Errorf("the signing certificate expired on %s", c.NotAfter.UTC().Format(time.RFC3339))
	}
	return nil
}

// Load reads the signing key of the configuration; nil when none is set.
func Load(c *config.Sign) (*Signer, error) {
	var key crypto.Signer
	var certs []*x509.Certificate
	var err error
	switch {
	case len(c.PKCS12) > 0:
		key, certs, err = ParsePKCS12(c.PKCS12, c.PKCS12Password)
		if err != nil {
			return nil, fmt.Errorf("ZR_SIGN_PKCS12: %w", err)
		}
		if len(c.Cert) > 0 {
			extra, err := ParseCertificates(c.Cert)
			if err != nil {
				return nil, fmt.Errorf("ZR_SIGN_CERT: %w", err)
			}
			certs = append(certs, extra...)
		}
	case len(c.Key) > 0:
		key, err = ParsePrivateKey(c.Key, c.KeyPassword)
		if err != nil {
			return nil, fmt.Errorf("ZR_SIGN_KEY: %w", err)
		}
		certs, err = ParseCertificates(c.Cert)
		if err != nil {
			return nil, fmt.Errorf("ZR_SIGN_CERT: %w", err)
		}
	default:
		return nil, nil
	}
	return New(key, certs)
}

// New checks the key and puts the certificates in chain order: the
// certificate of the key, then its issuer, and so on. The certificates
// outside the chain are dropped.
func New(key crypto.Signer, certs []*x509.Certificate) (*Signer, error) {
	switch k := key.Public().(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return nil, fmt.Errorf("RSA key of %d bits: at least 2048 are required", k.N.BitLen())
		}
	case *ecdsa.PublicKey:
	case ed25519.PublicKey:
		return nil, errors.New("Ed25519 keys cannot sign PAdES nor CAdES signatures yet: use an RSA or ECDSA key")
	default:
		return nil, fmt.Errorf("unsupported key type %T", k)
	}
	var leaf *x509.Certificate
	for _, c := range certs {
		if samePublicKey(c.PublicKey, key.Public()) {
			leaf = c
			break
		}
	}
	if leaf == nil {
		return nil, errors.New("no certificate matches the private key")
	}
	chain := []*x509.Certificate{leaf}
	for cur := leaf; !bytes.Equal(cur.RawIssuer, cur.RawSubject); {
		var next *x509.Certificate
		for _, c := range certs {
			if bytes.Equal(c.RawSubject, cur.RawIssuer) && !contains(chain, c) && cur.CheckSignatureFrom(c) == nil {
				next = c
				break
			}
		}
		if next == nil {
			break
		}
		chain = append(chain, next)
		cur = next
	}
	return &Signer{Key: key, Chain: chain}, nil
}

func contains(chain []*x509.Certificate, c *x509.Certificate) bool {
	for _, e := range chain {
		if e.Equal(c) {
			return true
		}
	}
	return false
}

func samePublicKey(a, b crypto.PublicKey) bool {
	k, ok := a.(interface{ Equal(crypto.PublicKey) bool })
	return ok && k.Equal(b)
}

// ParsePrivateKey reads the first private key of PEM data: RSA PRIVATE KEY
// (PKCS#1), EC PRIVATE KEY (SEC 1), PRIVATE KEY (PKCS#8) or ENCRYPTED
// PRIVATE KEY (PKCS#8 encrypted with PBES2, as OpenSSL writes it).
func ParsePrivateKey(data []byte, password string) (crypto.Signer, error) {
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			return nil, errors.New("no PEM private key")
		}
		if _, legacy := b.Headers["DEK-Info"]; legacy {
			return nil, errors.New("legacy PEM encryption (DEK-Info) is not supported: convert the key with openssl pkcs8 -topk8")
		}
		var k any
		var err error
		switch b.Type {
		case "RSA PRIVATE KEY":
			k, err = x509.ParsePKCS1PrivateKey(b.Bytes)
		case "EC PRIVATE KEY":
			k, err = x509.ParseECPrivateKey(b.Bytes)
		case "PRIVATE KEY":
			k, err = x509.ParsePKCS8PrivateKey(b.Bytes)
		case "ENCRYPTED PRIVATE KEY":
			if password == "" {
				return nil, errors.New("the key is encrypted: set ZR_SIGN_KEY_PASSWORD")
			}
			var der []byte
			if der, err = decryptPKCS8(b.Bytes, []byte(password)); err == nil {
				k, err = x509.ParsePKCS8PrivateKey(der)
			}
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Type, err)
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%s: %T cannot sign", b.Type, k)
		}
		return s, nil
	}
}

// ParseCertificates reads the CERTIFICATE blocks of PEM data.
func ParseCertificates(data []byte) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.New("no PEM certificate")
	}
	return out, nil
}
