// Package auth authenticates the requests of the API: bearer tokens (JWT
// signed with an HMAC secret, a public key or a key of a JWKS endpoint)
// and HTTP basic authentication.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	// Method is "jwt", "basic" or "none".
	Method string
	// Subject is the sub claim of a token, or the basic user name.
	Subject string
	// Claims are the claims of a token; nil for the other methods.
	Claims *Claims
}

type ctxKey struct{}

// FromContext returns the principal of a request; nil before the
// authentication.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// Authenticator checks the credentials of the requests.
type Authenticator struct {
	methods []string
	realm   string
	users   map[string][]byte // user name → SHA-256 of the password
	jwt     *jwtVerifier
	logger  *slog.Logger
}

// New builds an authenticator. client downloads the JWKS, when one is
// configured.
func New(c *config.Auth, client *http.Client, logger *slog.Logger) (*Authenticator, error) {
	a := &Authenticator{methods: c.Methods, realm: c.Realm, logger: logger}
	if slices.Contains(c.Methods, "basic") {
		users, err := parseUsers(c.BasicUsers)
		if err != nil {
			return nil, fmt.Errorf("ZR_BASIC_USERS: %w", err)
		}
		a.users = users
	}
	if slices.Contains(c.Methods, "jwt") {
		j := c.JWT
		v := &jwtVerifier{
			secret:    j.Secret,
			issuers:   j.Issuers,
			audiences: j.Audiences,
			scopes:    j.RequiredScopes,
			leeway:    j.Leeway,
			now:       time.Now,
		}
		if len(j.PublicKeys) > 0 {
			keys, err := parsePublicKeys(j.PublicKeys)
			if err != nil {
				return nil, fmt.Errorf("ZR_JWT_PUBLIC_KEYS: %w", err)
			}
			v.static = keys
		}
		if j.JWKSURL != "" {
			v.jwks = &keySet{url: j.JWKSURL, client: client, refresh: j.JWKSRefresh, logger: logger}
		}
		v.algs = defaultAlgorithms(v.secret, v.static, v.jwks != nil)
		if len(j.Algorithms) > 0 {
			allowed := map[string]bool{}
			for _, name := range j.Algorithms {
				if _, ok := algorithms[name]; !ok {
					return nil, fmt.Errorf("ZR_JWT_ALGORITHMS: unknown algorithm %q", name)
				}
				if !v.algs[name] {
					return nil, fmt.Errorf("ZR_JWT_ALGORITHMS: no configured key can verify %s", name)
				}
				allowed[name] = true
			}
			v.algs = allowed
		}
		a.jwt = v
	}
	return a, nil
}

// Run keeps the JWKS up to date until ctx is done; it returns at once when
// no JWKS is configured.
func (a *Authenticator) Run(ctx context.Context) {
	if a.jwt != nil && a.jwt.jwks != nil {
		a.jwt.jwks.Run(ctx)
	}
}

// Ready reports whether the authenticator can verify tokens: false until
// the JWKS, when one is configured, is downloaded.
func (a *Authenticator) Ready() bool {
	return a.jwt == nil || a.jwt.jwks == nil || a.jwt.jwks.Ready() || len(a.jwt.static) > 0 || len(a.jwt.secret) > 0
}

// Methods returns the accepted methods.
func (a *Authenticator) Methods() []string { return a.methods }

// Middleware rejects the requests without valid credentials with 401
// (403 for a token that lacks a required scope), and puts the principal
// of the others in their context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Authenticate(r)
		if err != nil {
			a.reject(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

var (
	errNoCredentials = errors.New("no credentials")
	errBadBasic      = errors.New("bad user name or password")
	errMethod        = errors.New("authentication method not accepted")
)

// Authenticate checks the credentials of a request.
func (a *Authenticator) Authenticate(r *http.Request) (*Principal, error) {
	if slices.Contains(a.methods, "none") {
		return &Principal{Method: "none"}, nil
	}
	h := r.Header.Get("Authorization")
	scheme, cred, _ := strings.Cut(h, " ")
	cred = strings.TrimSpace(cred)
	switch {
	case h == "":
		return nil, errNoCredentials
	case strings.EqualFold(scheme, "Bearer"):
		if a.jwt == nil {
			return nil, errMethod
		}
		c, err := a.jwt.Verify(cred)
		if err != nil {
			return nil, err
		}
		sub := c.Subject
		if sub == "" {
			sub = c.ClientID
		}
		return &Principal{Method: "jwt", Subject: sub, Claims: c}, nil
	case strings.EqualFold(scheme, "Basic"):
		if a.users == nil {
			return nil, errMethod
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !a.checkPassword(user, pass) {
			return nil, errBadBasic
		}
		return &Principal{Method: "basic", Subject: user}, nil
	}
	return nil, errMethod
}

// checkPassword compares the digests in constant time, and spends the same
// time on an unknown user.
func (a *Authenticator) checkPassword(user, pass string) bool {
	got := sha256.Sum256([]byte(pass))
	want, ok := a.users[user]
	if !ok {
		want = make([]byte, sha256.Size)
	}
	return subtle.ConstantTimeCompare(got[:], want) == 1 && ok
}

// reject answers 401 with the challenges of the accepted methods, or 403
// for a missing scope. The reason is logged, not sent: it would help an
// attacker more than a client.
func (a *Authenticator) reject(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusUnauthorized
	code, msg := "unauthorized", "missing or invalid credentials"
	if errors.Is(err, errScope) {
		status, code, msg = http.StatusForbidden, "forbidden", "the token lacks a required scope"
	}
	if slices.Contains(a.methods, "jwt") {
		ch := fmt.Sprintf("Bearer realm=%q", a.realm)
		switch {
		case errors.Is(err, errScope):
			ch += `, error="insufficient_scope"`
		case errors.Is(err, errInvalidToken):
			ch += `, error="invalid_token"`
		}
		w.Header().Add("WWW-Authenticate", ch)
	}
	if slices.Contains(a.methods, "basic") {
		w.Header().Add("WWW-Authenticate", fmt.Sprintf("Basic realm=%q, charset=\"UTF-8\"", a.realm))
	}
	a.logger.LogAttrs(r.Context(), slog.LevelWarn, "authentication refused",
		slog.String("reason", err.Error()),
		slog.String("remote_addr", r.RemoteAddr),
		slog.String("path", r.URL.Path))
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"type":"about:blank","title":%q,"status":%d,"code":%q}`+"\n", msg, status, code)
}

// parseUsers reads user:password entries, one per line, or separated by
// commas when the value holds a single line. A password of the form
// sha256:<64 hexadecimal digits> is the SHA-256 digest of the password.
func parseUsers(s string) (map[string][]byte, error) {
	sep := ","
	if strings.Contains(s, "\n") {
		sep = "\n"
	}
	users := map[string][]byte{}
	for e := range strings.SplitSeq(s, sep) {
		e = strings.TrimSpace(e)
		if e == "" || strings.HasPrefix(e, "#") {
			continue
		}
		user, pass, ok := strings.Cut(e, ":")
		if !ok || user == "" || pass == "" {
			return nil, fmt.Errorf("entry %q is not user:password", redact(e))
		}
		if _, dup := users[user]; dup {
			return nil, fmt.Errorf("user %q listed twice", user)
		}
		if hexDigest, ok := strings.CutPrefix(pass, "sha256:"); ok && len(hexDigest) == 2*sha256.Size {
			d, err := hex.DecodeString(hexDigest)
			if err != nil {
				return nil, fmt.Errorf("user %q: the sha256 digest is not hexadecimal", user)
			}
			users[user] = d
			continue
		}
		d := sha256.Sum256([]byte(pass))
		users[user] = d[:]
	}
	if len(users) == 0 {
		return nil, errors.New("no user")
	}
	return users, nil
}

// redact keeps the user name of an entry and hides the rest.
func redact(e string) string {
	user, _, _ := strings.Cut(e, ":")
	return user + ":***"
}
