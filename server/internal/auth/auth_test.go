package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

var (
	rsaKey, _       = rsa.GenerateKey(rand.Reader, 2048)
	ecKey, _        = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	edPub, edKey, _ = ed25519.GenerateKey(rand.Reader)
	secret          = []byte("0123456789abcdef0123456789abcdef")
	discard         = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// sign builds a token; key is a []byte secret or a crypto.Signer.
func sign(t *testing.T, alg string, key any, kid string, claims map[string]any) string {
	t.Helper()
	h := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		h["kid"] = kid
	}
	hj, _ := json.Marshal(h)
	cj, _ := json.Marshal(claims)
	signed := b64(hj) + "." + b64(cj)
	a := algorithms[alg]
	var sig []byte
	var digest []byte
	if a.hash != 0 {
		hh := a.hash.New()
		hh.Write([]byte(signed))
		digest = hh.Sum(nil)
	}
	var err error
	switch a.kind {
	case "hmac":
		m := hmac.New(a.hash.New, key.([]byte))
		m.Write([]byte(signed))
		sig = m.Sum(nil)
	case "rsa":
		sig, err = rsa.SignPKCS1v15(rand.Reader, key.(*rsa.PrivateKey), a.hash, digest)
	case "pss":
		sig, err = rsa.SignPSS(rand.Reader, key.(*rsa.PrivateKey), a.hash, digest, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	case "ecdsa":
		var r, s *big.Int
		r, s, err = ecdsa.Sign(rand.Reader, key.(*ecdsa.PrivateKey), digest)
		if err == nil {
			sig = make([]byte, 2*a.size)
			r.FillBytes(sig[:a.size])
			s.FillBytes(sig[a.size:])
		}
	case "eddsa":
		sig = ed25519.Sign(key.(ed25519.PrivateKey), []byte(signed))
	default:
		sig = nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + b64(sig)
}

func valid() map[string]any {
	now := time.Now().Unix()
	return map[string]any{"sub": "billing-service", "iss": "https://idp.example", "aud": []string{"zerechnung"}, "exp": now + 300, "iat": now, "scope": "invoices:write invoices:sign"}
}

func pemOf(t *testing.T, pub crypto.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func newAuth(t *testing.T, c config.Auth) *Authenticator {
	t.Helper()
	if c.Realm == "" {
		c.Realm = "zerechnung"
	}
	a, err := New(&c, http.DefaultClient, discard)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func call(a *Authenticator, header string) (int, string) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/info", nil)
	if header != "" {
		r.Header.Set("Authorization", header)
	}
	var sub string
	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub = FromContext(r.Context()).Subject
	})).ServeHTTP(rec, r)
	return rec.Code, sub
}

func TestJWTAlgorithms(t *testing.T) {
	keys := append(append(pemOf(t, rsaKey.Public()), pemOf(t, ecKey.Public())...), pemOf(t, edPub)...)
	a := newAuth(t, config.Auth{Methods: []string{"jwt"}, JWT: config.JWT{
		Secret: secret, PublicKeys: keys, Issuers: []string{"https://idp.example"}, Audiences: []string{"zerechnung"}, Leeway: time.Minute,
	}})
	for _, tc := range []struct {
		alg string
		key any
	}{
		{"HS256", secret}, {"HS384", secret}, {"HS512", secret},
		{"RS256", rsaKey}, {"RS512", rsaKey}, {"PS256", rsaKey}, {"PS384", rsaKey},
		{"ES256", ecKey}, {"EdDSA", edKey},
	} {
		code, sub := call(a, "Bearer "+sign(t, tc.alg, tc.key, "", valid()))
		if code != http.StatusOK || sub != "billing-service" {
			t.Errorf("%s: got %d %q, want 200", tc.alg, code, sub)
		}
	}
}

func TestJWTRefused(t *testing.T) {
	a := newAuth(t, config.Auth{Methods: []string{"jwt"}, JWT: config.JWT{
		PublicKeys: pemOf(t, rsaKey.Public()), Issuers: []string{"https://idp.example"}, Audiences: []string{"zerechnung"},
		RequiredScopes: []string{"invoices:write"}, Leeway: time.Minute,
	}})
	mod := func(f func(map[string]any)) map[string]any { c := valid(); f(c); return c }
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := sign(t, "RS256", rsaKey, "", valid())
	for name, tc := range map[string]struct {
		header string
		want   int
	}{
		"no header":        {"", 401},
		"garbage":          {"Bearer abc.def", 401},
		"expired":          {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { c["exp"] = time.Now().Add(-2 * time.Minute).Unix() })), 401},
		"no exp":           {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { delete(c, "exp") })), 401},
		"not yet valid":    {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() })), 401},
		"wrong issuer":     {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { c["iss"] = "https://evil.example" })), 401},
		"wrong audience":   {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { c["aud"] = "other" })), 401},
		"missing scope":    {"Bearer " + sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { c["scope"] = "invoices:read" })), 403},
		"other key":        {"Bearer " + sign(t, "RS256", otherKey, "", valid()), 401},
		"alg none":         {"Bearer " + strings.Join(strings.Split(tok, ".")[:2], ".") + ".", 401},
		"HS256 confusion":  {"Bearer " + sign(t, "HS256", pemOf(t, rsaKey.Public()), "", valid()), 401},
		"tampered payload": {"Bearer " + tamper(tok), 401},
		"basic not on":     {"Basic " + base64.StdEncoding.EncodeToString([]byte("a:b")), 401},
	} {
		if code, _ := call(a, tc.header); code != tc.want {
			t.Errorf("%s: got %d, want %d", name, code, tc.want)
		}
	}
	// The scope may also come as an scp list.
	if code, _ := call(a, "Bearer "+sign(t, "RS256", rsaKey, "", mod(func(c map[string]any) { delete(c, "scope"); c["scp"] = []string{"invoices:write"} }))); code != 200 {
		t.Errorf("scp list: got %d", code)
	}
}

func tamper(tok string) string {
	p := strings.Split(tok, ".")
	c := valid()
	c["sub"] = "admin"
	cj, _ := json.Marshal(c)
	return p[0] + "." + b64(cj) + "." + p[2]
}

func TestJWTChallenge(t *testing.T) {
	a := newAuth(t, config.Auth{Methods: []string{"jwt", "basic"}, BasicUsers: "u:p", JWT: config.JWT{Secret: secret}})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer x.y.z")
	a.Middleware(http.NotFoundHandler()).ServeHTTP(rec, r)
	ch := rec.Header().Values("WWW-Authenticate")
	if rec.Code != 401 || len(ch) != 2 || !strings.Contains(ch[0], `error="invalid_token"`) || !strings.HasPrefix(ch[1], "Basic ") {
		t.Errorf("got %d %q", rec.Code, ch)
	}
}

func TestJWKS(t *testing.T) {
	var hits atomic.Int32
	kid := "key-2026"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		point, err := ecKey.ECDH()
		if err != nil {
			t.Error(err)
			return
		}
		xy := point.PublicKey().Bytes()[1:]
		x, y := xy[:32], xy[32:]
		e := big.NewInt(int64(rsaKey.E)).Bytes()
		fmt.Fprintf(w, `{"keys":[
			{"kty":"RSA","kid":%q,"use":"sig","alg":"RS256","n":%q,"e":%q},
			{"kty":"EC","kid":"ec-1","crv":"P-256","x":%q,"y":%q},
			{"kty":"OKP","kid":"ed-1","crv":"Ed25519","x":%q},
			{"kty":"RSA","kid":"enc","use":"enc","n":"AQAB","e":"AQAB"},
			{"kty":"oct","kid":"sym","k":"c2VjcmV0"}
		]}`, kid, b64(rsaKey.N.Bytes()), b64(e), b64(x), b64(y), b64(edPub))
	}))
	defer srv.Close()
	a := newAuth(t, config.Auth{Methods: []string{"jwt"}, JWT: config.JWT{JWKSURL: srv.URL, JWKSRefresh: time.Hour, Leeway: time.Minute}})
	if a.Ready() {
		t.Error("ready before the first download")
	}
	// The unknown kid triggers the first download.
	if code, _ := call(a, "Bearer "+sign(t, "RS256", rsaKey, kid, valid())); code != 200 {
		t.Errorf("RS256 with kid: got %d", code)
	}
	if !a.Ready() {
		t.Error("not ready after the download")
	}
	for _, tc := range []struct {
		alg, kid string
		key      any
		want     int
	}{
		{"ES256", "ec-1", ecKey, 200},
		{"EdDSA", "ed-1", edKey, 200},
		{"PS256", kid, rsaKey, 401},   // the key is restricted to RS256
		{"ES256", kid, ecKey, 401},    // wrong key type for the kid
		{"HS256", "sym", secret, 401}, // symmetric keys of a JWKS are ignored
	} {
		if code, _ := call(a, "Bearer "+sign(t, tc.alg, tc.key, tc.kid, valid())); code != tc.want {
			t.Errorf("%s kid %s: got %d, want %d", tc.alg, tc.kid, code, tc.want)
		}
	}
	// Unknown kids do not download the set again within minRefetch.
	before := hits.Load()
	for i := range 5 {
		call(a, "Bearer "+sign(t, "RS256", rsaKey, fmt.Sprint("unknown-", i), valid()))
	}
	if hits.Load() != before {
		t.Errorf("%d downloads for unknown kids", hits.Load()-before)
	}
}

func TestBasic(t *testing.T) {
	d := sha256.Sum256([]byte("s3cret:with:colons"))
	a := newAuth(t, config.Auth{Methods: []string{"basic"}, BasicUsers: "alice:wonderland\n# comment\nbob:sha256:" + hex.EncodeToString(d[:]) + "\n"})
	basic := func(u, p string) string { return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p)) }
	for _, tc := range []struct {
		user, pass string
		want       int
	}{
		{"alice", "wonderland", 200},
		{"bob", "s3cret:with:colons", 200},
		{"alice", "wrong", 401},
		{"carol", "wonderland", 401},
		{"bob", "sha256:" + hex.EncodeToString(d[:]), 401},
	} {
		if code, sub := call(a, basic(tc.user, tc.pass)); code != tc.want || (code == 200 && sub != tc.user) {
			t.Errorf("%s:%s: got %d %q, want %d", tc.user, tc.pass, code, sub, tc.want)
		}
	}
	if code, _ := call(a, "Bearer x.y.z"); code != 401 {
		t.Errorf("bearer with basic only: got %d", code)
	}
}

func TestParseUsersErrors(t *testing.T) {
	for _, s := range []string{"", "alice", "alice:", ":pw", "a:1,a:2"} {
		if _, err := parseUsers(s); err == nil {
			t.Errorf("%q: no error", s)
		} else if strings.Contains(err.Error(), "pw") {
			t.Errorf("%q: the error shows the password: %v", s, err)
		}
	}
}

func TestNone(t *testing.T) {
	a := newAuth(t, config.Auth{Methods: []string{"none"}})
	if code, _ := call(a, ""); code != 200 {
		t.Errorf("got %d", code)
	}
}

func TestConfigErrors(t *testing.T) {
	for name, c := range map[string]config.Auth{
		"weak RSA key":      {Methods: []string{"jwt"}, JWT: config.JWT{PublicKeys: pemOf(t, mustRSA(t, 1024).Public())}},
		"not PEM":           {Methods: []string{"jwt"}, JWT: config.JWT{PublicKeys: []byte("hello")}},
		"unknown algorithm": {Methods: []string{"jwt"}, JWT: config.JWT{Secret: secret, Algorithms: []string{"none"}}},
		"algorithm, no key": {Methods: []string{"jwt"}, JWT: config.JWT{Secret: secret, Algorithms: []string{"RS256"}}},
	} {
		if _, err := New(&c, http.DefaultClient, discard); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func mustRSA(t *testing.T, bits int) *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
