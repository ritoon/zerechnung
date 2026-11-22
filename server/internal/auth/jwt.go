package auth

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	// The hash functions must be linked for crypto.Hash.New.
	_ "crypto/sha256"
	_ "crypto/sha512"
)

// maxTokenLen bounds the length of a bearer token.
const maxTokenLen = 16 << 10

// algorithm describes a JWS algorithm (RFC 7518, RFC 8037).
type algorithm struct {
	hash crypto.Hash
	kind string // "hmac", "rsa", "pss", "ecdsa", "eddsa"
	// curve and size are those of the ECDSA algorithms.
	curve elliptic.Curve
	size  int
}

var algorithms = map[string]algorithm{
	"HS256": {hash: crypto.SHA256, kind: "hmac"},
	"HS384": {hash: crypto.SHA384, kind: "hmac"},
	"HS512": {hash: crypto.SHA512, kind: "hmac"},
	"RS256": {hash: crypto.SHA256, kind: "rsa"},
	"RS384": {hash: crypto.SHA384, kind: "rsa"},
	"RS512": {hash: crypto.SHA512, kind: "rsa"},
	"PS256": {hash: crypto.SHA256, kind: "pss"},
	"PS384": {hash: crypto.SHA384, kind: "pss"},
	"PS512": {hash: crypto.SHA512, kind: "pss"},
	"ES256": {hash: crypto.SHA256, kind: "ecdsa", curve: elliptic.P256(), size: 32},
	"ES384": {hash: crypto.SHA384, kind: "ecdsa", curve: elliptic.P384(), size: 48},
	"ES512": {hash: crypto.SHA512, kind: "ecdsa", curve: elliptic.P521(), size: 66},
	"EdDSA": {kind: "eddsa"},
}

// Claims are the claims of a verified token.
type Claims struct {
	Subject  string
	Issuer   string
	Scopes   []string
	Expiry   time.Time
	ClientID string
	// All holds every claim, as decoded from JSON.
	All map[string]any
}

// jwtVerifier verifies the signature and the claims of a JWS compact
// token.
type jwtVerifier struct {
	secret    []byte
	static    []crypto.PublicKey
	jwks      *keySet
	algs      map[string]bool
	issuers   []string
	audiences []string
	scopes    []string
	leeway    time.Duration
	now       func() time.Time
}

// errInvalidToken wraps every reason to refuse a token; the reason is
// logged, not sent to the client.
var errInvalidToken = errors.New("invalid token")

// errScope is returned for a valid token that lacks a required scope.
var errScope = errors.New("insufficient scope")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errInvalidToken, fmt.Sprintf(format, args...))
}

// defaultAlgorithms returns the algorithms that the configured keys can
// verify.
func defaultAlgorithms(secret []byte, keys []crypto.PublicKey, jwks bool) map[string]bool {
	out := map[string]bool{}
	add := func(names ...string) {
		for _, n := range names {
			out[n] = true
		}
	}
	if len(secret) > 0 {
		add("HS256", "HS384", "HS512")
	}
	if jwks {
		add("RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA")
	}
	for _, k := range keys {
		switch k := k.(type) {
		case *rsa.PublicKey:
			add("RS256", "RS384", "RS512", "PS256", "PS384", "PS512")
		case *ecdsa.PublicKey:
			switch k.Curve {
			case elliptic.P256():
				add("ES256")
			case elliptic.P384():
				add("ES384")
			case elliptic.P521():
				add("ES512")
			}
		case ed25519.PublicKey:
			add("EdDSA")
		}
	}
	return out
}

// parsePublicKeys reads the public keys of PEM blocks: PUBLIC KEY,
// RSA PUBLIC KEY or CERTIFICATE.
func parsePublicKeys(data []byte) ([]crypto.PublicKey, error) {
	var keys []crypto.PublicKey
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			break
		}
		var k crypto.PublicKey
		var err error
		switch b.Type {
		case "PUBLIC KEY":
			k, err = x509.ParsePKIXPublicKey(b.Bytes)
		case "RSA PUBLIC KEY":
			k, err = x509.ParsePKCS1PublicKey(b.Bytes)
		case "CERTIFICATE":
			var c *x509.Certificate
			if c, err = x509.ParseCertificate(b.Bytes); err == nil {
				k = c.PublicKey
			}
		default:
			return nil, fmt.Errorf("PEM block %q: expected PUBLIC KEY, RSA PUBLIC KEY or CERTIFICATE", b.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("PEM block %q: %w", b.Type, err)
		}
		if err := checkKey(k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if len(bytes.TrimSpace(data)) > 0 || len(keys) == 0 {
		return nil, errors.New("expected PEM public keys or certificates")
	}
	return keys, nil
}

// checkKey refuses the keys that are too weak to verify tokens.
func checkKey(k crypto.PublicKey) error {
	switch k := k.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return fmt.Errorf("RSA key of %d bits: at least 2048 are required", k.N.BitLen())
		}
	case *ecdsa.PublicKey, ed25519.PublicKey:
	default:
		return fmt.Errorf("unsupported public key type %T", k)
	}
	return nil
}

type header struct {
	Alg  string          `json:"alg"`
	Kid  string          `json:"kid"`
	Typ  string          `json:"typ"`
	Crit json.RawMessage `json:"crit"`
}

// Verify checks a token and returns its claims.
func (v *jwtVerifier) Verify(token string) (*Claims, error) {
	if len(token) > maxTokenLen {
		return nil, invalid("token longer than %d bytes", maxTokenLen)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, invalid("not a JWS compact token (%d parts)", len(parts))
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, invalid("header: %v", err)
	}
	var h header
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, invalid("header: %v", err)
	}
	if len(h.Crit) > 0 {
		return nil, invalid("unsupported critical header parameters %s", h.Crit)
	}
	alg, known := algorithms[h.Alg]
	if !known || !v.algs[h.Alg] {
		return nil, invalid("algorithm %q not accepted", h.Alg)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, invalid("signature: %v", err)
	}
	signed := []byte(parts[0] + "." + parts[1])
	if !v.verifySignature(h, alg, signed, sig) {
		return nil, invalid("bad signature or unknown key (alg %s, kid %q)", h.Alg, h.Kid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, invalid("payload: %v", err)
	}
	return v.checkClaims(payload)
}

func (v *jwtVerifier) verifySignature(h header, alg algorithm, signed, sig []byte) bool {
	if alg.kind == "hmac" {
		return len(v.secret) > 0 && verifyWith(alg, v.secret, signed, sig)
	}
	if v.jwks != nil {
		for _, k := range v.jwks.Lookup(h.Kid, h.Alg) {
			if verifyWith(alg, k, signed, sig) {
				return true
			}
		}
	}
	for _, k := range v.static {
		if verifyWith(alg, k, signed, sig) {
			return true
		}
	}
	return false
}

// verifyWith checks a signature with a key whose type must match the
// algorithm, which rules out the algorithm confusion attacks.
func verifyWith(alg algorithm, key any, signed, sig []byte) bool {
	var digest []byte
	if alg.hash != 0 {
		hh := alg.hash.New()
		hh.Write(signed)
		digest = hh.Sum(nil)
	}
	switch alg.kind {
	case "hmac":
		k, ok := key.([]byte)
		if !ok {
			return false
		}
		m := hmac.New(alg.hash.New, k)
		m.Write(signed)
		return hmac.Equal(m.Sum(nil), sig)
	case "rsa":
		k, ok := key.(*rsa.PublicKey)
		return ok && rsa.VerifyPKCS1v15(k, alg.hash, digest, sig) == nil
	case "pss":
		k, ok := key.(*rsa.PublicKey)
		return ok && rsa.VerifyPSS(k, alg.hash, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto}) == nil
	case "ecdsa":
		k, ok := key.(*ecdsa.PublicKey)
		if !ok || k.Curve != alg.curve || len(sig) != 2*alg.size {
			return false
		}
		r := new(big.Int).SetBytes(sig[:alg.size])
		s := new(big.Int).SetBytes(sig[alg.size:])
		return ecdsa.Verify(k, digest, r, s)
	case "eddsa":
		k, ok := key.(ed25519.PublicKey)
		return ok && len(k) == ed25519.PublicKeySize && ed25519.Verify(k, signed, sig)
	}
	return false
}

func (v *jwtVerifier) checkClaims(payload []byte) (*Claims, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var all map[string]any
	if err := dec.Decode(&all); err != nil {
		return nil, invalid("claims: %v", err)
	}
	now := v.now()
	c := &Claims{All: all}

	exp, ok, err := numericDate(all, "exp")
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return nil, invalid("no exp claim")
	case now.After(exp.Add(v.leeway)):
		return nil, invalid("expired at %s", exp.UTC().Format(time.RFC3339))
	}
	c.Expiry = exp
	if nbf, ok, err := numericDate(all, "nbf"); err != nil {
		return nil, err
	} else if ok && now.Add(v.leeway).Before(nbf) {
		return nil, invalid("not valid before %s", nbf.UTC().Format(time.RFC3339))
	}
	if iat, ok, err := numericDate(all, "iat"); err != nil {
		return nil, err
	} else if ok && now.Add(v.leeway).Before(iat) {
		return nil, invalid("issued in the future (%s)", iat.UTC().Format(time.RFC3339))
	}

	c.Issuer, _ = all["iss"].(string)
	if len(v.issuers) > 0 && !slices.Contains(v.issuers, c.Issuer) {
		return nil, invalid("issuer %q not accepted", c.Issuer)
	}
	if len(v.audiences) > 0 {
		auds := stringList(all["aud"])
		if !slices.ContainsFunc(auds, func(a string) bool { return slices.Contains(v.audiences, a) }) {
			return nil, invalid("audience %q not accepted", auds)
		}
	}
	c.Subject, _ = all["sub"].(string)
	c.ClientID, _ = all["client_id"].(string)
	if c.ClientID == "" {
		c.ClientID, _ = all["azp"].(string)
	}
	// scope is a space-separated string (RFC 8693); scp is a list or a
	// string in some providers (Azure AD, Okta).
	c.Scopes = append(strings.Fields(stringOr(all["scope"])), stringList(all["scp"])...)
	for _, s := range v.scopes {
		if !slices.Contains(c.Scopes, s) {
			return c, fmt.Errorf("%w: %q is required", errScope, s)
		}
	}
	return c, nil
}

// numericDate reads a NumericDate claim (RFC 7519, seconds since the epoch,
// possibly with a fraction).
func numericDate(all map[string]any, name string) (time.Time, bool, error) {
	v, ok := all[name]
	if !ok {
		return time.Time{}, false, nil
	}
	n, ok := v.(json.Number)
	if !ok {
		return time.Time{}, false, invalid("%s is not a number", name)
	}
	f, err := n.Float64()
	if err != nil {
		return time.Time{}, false, invalid("%s: %v", name, err)
	}
	sec := int64(f)
	return time.Unix(sec, int64((f-float64(sec))*1e9)), true, nil
}

func stringOr(v any) string {
	s, _ := v.(string)
	return s
}

// stringList reads a claim that holds a string or a list of strings.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return strings.Fields(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
