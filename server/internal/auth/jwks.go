package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// maxJWKSBytes bounds the size of a key set document.
const maxJWKSBytes = 1 << 20

// minRefetch is the shortest time between two downloads triggered by an
// unknown key identifier, which keeps tokens with made-up kid values from
// hammering the identity provider.
const minRefetch = 30 * time.Second

// keySet is a JSON Web Key Set (RFC 7517) downloaded from a URL and
// refreshed periodically.
type keySet struct {
	url     string
	client  *http.Client
	refresh time.Duration
	logger  *slog.Logger

	mu      sync.RWMutex
	keys    []jwk
	fetched time.Time // last successful download
	tried   time.Time // last attempt
	fetchMu sync.Mutex
}

type jwk struct {
	kid string
	alg string
	key crypto.PublicKey
}

// Lookup returns the keys that may verify a token with this key identifier
// and algorithm. An unknown identifier triggers a download, at most every
// minRefetch.
func (s *keySet) Lookup(kid, alg string) []crypto.PublicKey {
	keys, found := s.match(kid, alg)
	if !found && kid != "" {
		s.mu.RLock()
		stale := time.Since(s.tried) > minRefetch
		s.mu.RUnlock()
		if stale {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.Fetch(ctx); err != nil {
				s.logger.Warn("JWKS download failed", "url", s.url, "error", err)
			}
			keys, _ = s.match(kid, alg)
		}
	}
	return keys
}

// match returns the matching keys, and whether a key has this identifier.
func (s *keySet) match(kid, alg string) ([]crypto.PublicKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []crypto.PublicKey
	found := false
	for _, k := range s.keys {
		if kid != "" && k.kid != kid {
			continue
		}
		found = found || k.kid == kid
		if k.alg != "" && k.alg != alg {
			continue
		}
		out = append(out, k.key)
	}
	return out, found
}

// Ready reports whether the key set was downloaded once.
func (s *keySet) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.fetched.IsZero()
}

// Run downloads the key set, then again every refresh period, until ctx
// is done. A failed download is retried sooner, with a backoff.
func (s *keySet) Run(ctx context.Context) {
	retry := 2 * time.Second
	for {
		wait := s.refresh
		if err := s.Fetch(ctx); err != nil {
			s.logger.Warn("JWKS download failed", "url", s.url, "error", err, "retry_in", retry.String())
			wait = retry
			retry = min(2*retry, s.refresh)
		} else {
			retry = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Fetch downloads the key set and replaces the keys.
func (s *keySet) Fetch(ctx context.Context) error {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	s.mu.Lock()
	s.tried = time.Now()
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/jwk-set+json, application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxJWKSBytes {
		return fmt.Errorf("key set larger than %d bytes", maxJWKSBytes)
	}
	keys, skipped, err := parseJWKS(body)
	if err != nil {
		return err
	}
	for _, msg := range skipped {
		s.logger.Debug("JWKS key skipped", "url", s.url, "reason", msg)
	}
	s.mu.Lock()
	s.keys = keys
	s.fetched = time.Now()
	s.mu.Unlock()
	s.logger.Info("JWKS loaded", "url", s.url, "keys", len(keys))
	return nil
}

type rawJWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// parseJWKS reads the signature keys of a key set. The keys it cannot use
// (encryption keys, symmetric keys, unknown types) are skipped, with the
// reason.
func parseJWKS(data []byte) ([]jwk, []string, error) {
	var doc struct {
		Keys []rawJWK `json:"keys"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("key set: %w", err)
	}
	var keys []jwk
	var skipped []string
	for _, r := range doc.Keys {
		if r.Use != "" && r.Use != "sig" {
			skipped = append(skipped, fmt.Sprintf("kid %q: use %q", r.Kid, r.Use))
			continue
		}
		k, err := r.publicKey()
		if err == nil {
			err = checkKey(k)
		}
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("kid %q: %v", r.Kid, err))
			continue
		}
		keys = append(keys, jwk{kid: r.Kid, alg: r.Alg, key: k})
	}
	if len(keys) == 0 {
		return nil, skipped, errors.New("the key set holds no usable signature key")
	}
	return keys, skipped, nil
}

func (r *rawJWK) publicKey() (crypto.PublicKey, error) {
	b64 := func(name, v string) ([]byte, error) {
		b, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil || len(b) == 0 {
			return nil, fmt.Errorf("parameter %s is not base64url", name)
		}
		return b, nil
	}
	switch r.Kty {
	case "RSA":
		n, err := b64("n", r.N)
		if err != nil {
			return nil, err
		}
		e, err := b64("e", r.E)
		if err != nil {
			return nil, err
		}
		if len(e) > 4 {
			return nil, errors.New("RSA exponent too large")
		}
		exp := 0
		for _, c := range e {
			exp = exp<<8 | int(c)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}, nil
	case "EC":
		var curve elliptic.Curve
		switch r.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported curve %q", r.Crv)
		}
		x, err := b64("x", r.X)
		if err != nil {
			return nil, err
		}
		y, err := b64("y", r.Y)
		if err != nil {
			return nil, err
		}
		size := (curve.Params().BitSize + 7) / 8
		if len(x) != size || len(y) != size {
			return nil, errors.New("EC coordinates of the wrong length")
		}
		return ecdsaFromPoint(curve, append([]byte{4}, append(x, y...)...))
	case "OKP":
		if r.Crv != "Ed25519" {
			return nil, fmt.Errorf("unsupported OKP curve %q", r.Crv)
		}
		x, err := b64("x", r.X)
		if err != nil {
			return nil, err
		}
		if len(x) != ed25519.PublicKeySize {
			return nil, errors.New("Ed25519 key of the wrong length")
		}
		return ed25519.PublicKey(x), nil
	}
	return nil, fmt.Errorf("unsupported key type %q", r.Kty)
}

// ecdsaFromPoint builds an ECDSA public key from an uncompressed point.
// The point goes through a SubjectPublicKeyInfo, whose parser refuses a
// point that is not on the curve.
func ecdsaFromPoint(curve elliptic.Curve, point []byte) (*ecdsa.PublicKey, error) {
	var oid asn1.ObjectIdentifier
	switch curve {
	case elliptic.P256():
		oid = asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}
	case elliptic.P384():
		oid = asn1.ObjectIdentifier{1, 3, 132, 0, 34}
	case elliptic.P521():
		oid = asn1.ObjectIdentifier{1, 3, 132, 0, 35}
	default:
		return nil, errors.New("unsupported curve")
	}
	type algorithmIdentifier struct {
		Algorithm  asn1.ObjectIdentifier
		Parameters asn1.ObjectIdentifier
	}
	spki, err := asn1.Marshal(struct {
		Algorithm algorithmIdentifier
		PublicKey asn1.BitString
	}{
		algorithmIdentifier{asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}, oid},
		asn1.BitString{Bytes: point, BitLength: 8 * len(point)},
	})
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return nil, fmt.Errorf("EC point: %w", err)
	}
	ek, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("EC point: not an ECDSA key")
	}
	return ek, nil
}
