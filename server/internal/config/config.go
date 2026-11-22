// Package config reads the configuration of the server from environment
// variables, the usual way to configure a container.
//
// Every variable starts with ZR_. A secret can also be read from a file,
// which is how Docker and Kubernetes secrets are mounted: ZR_X_FILE names
// the file that holds the value of ZR_X. Setting both is an error.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Prefix starts the name of every variable.
const Prefix = "ZR_"

// Config is the configuration of the server.
type Config struct {
	// Addr is the address the server listens on, host:port.
	Addr string
	// TLSCertFile and TLSKeyFile, when set, make the server speak HTTPS.
	TLSCertFile, TLSKeyFile string
	// MaxBodyBytes bounds the size of a request body.
	MaxBodyBytes int64
	// ReadTimeout, WriteTimeout and IdleTimeout bound the connections.
	ReadTimeout, WriteTimeout, IdleTimeout time.Duration
	// ShutdownTimeout bounds the time left to the requests in flight when
	// the server stops.
	ShutdownTimeout time.Duration
	// ServiceName names the service in the logs and the metrics.
	ServiceName string
	// Metrics serves the Prometheus metrics on /metrics.
	Metrics bool
	// CAFile adds the certificates of a PEM file to the system roots, for
	// the outgoing TLS connections (JWKS, log outputs).
	CAFile string

	Auth Auth
	Sign Sign
	Log  Log
}

// Auth configures the authentication of the API.
type Auth struct {
	// Methods are the accepted methods: "jwt", "basic", or "none" alone.
	Methods []string
	// BasicUsers lists the users of the basic authentication, one
	// user:password or user:sha256:<hex digest> per line or comma.
	BasicUsers string
	// Realm is the realm sent in WWW-Authenticate.
	Realm string
	JWT   JWT
}

// JWT configures the verification of the bearer tokens.
type JWT struct {
	// Secret is the HMAC key of the HS256, HS384 and HS512 tokens.
	Secret []byte
	// PublicKeys holds PEM public keys or certificates for the RS*, PS*,
	// ES* and EdDSA tokens.
	PublicKeys []byte
	// JWKSURL is the URL of a JSON Web Key Set, such as the jwks_uri of an
	// OpenID Connect provider.
	JWKSURL string
	// JWKSRefresh is the time between two downloads of the key set.
	JWKSRefresh time.Duration
	// Issuers are the accepted values of the iss claim; any when empty.
	Issuers []string
	// Audiences are the accepted values of the aud claim; any when empty.
	Audiences []string
	// Algorithms restricts the accepted algorithms; those of the configured
	// keys when empty.
	Algorithms []string
	// Leeway is the clock skew admitted on exp, nbf and iat.
	Leeway time.Duration
	// RequiredScopes must all be in the scope (or scp) claim.
	RequiredScopes []string
}

// Configured reports whether a key is set to verify tokens.
func (j *JWT) Configured() bool {
	return len(j.Secret) > 0 || len(j.PublicKeys) > 0 || j.JWKSURL != ""
}

// Sign configures the signature of the documents.
type Sign struct {
	// Key is a PEM private key: PKCS#1, SEC 1, PKCS#8, or encrypted
	// PKCS#8 with KeyPassword.
	Key         []byte
	KeyPassword string
	// Cert holds the PEM certificates: the signing certificate and the
	// intermediate authorities.
	Cert []byte
	// PKCS12 is a PKCS#12 (.p12, .pfx) file holding the key and the
	// certificates, instead of Key and Cert.
	PKCS12         []byte
	PKCS12Password string
	// Hash is "sha256", "sha384" or "sha512".
	Hash string
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5.
	PSS bool
	// Reason, Location, ContactInfo and Name are written in the PDF
	// signatures.
	Reason, Location, ContactInfo, Name string
	// Default signs the invoices that a request does not say to sign or
	// not.
	Default bool
}

// Configured reports whether a signing key is set.
func (s *Sign) Configured() bool { return len(s.Key) > 0 || len(s.PKCS12) > 0 }

// Log configures the logs.
type Log struct {
	Level slog.Level
	// Format is "json" or "text", for the stdout, stderr and file outputs.
	Format string
	// Outputs lists where the logs go: stdout, stderr, file, syslog, gelf,
	// otlp, loki, http.
	Outputs []string

	File           string
	FileMaxBytes   int64
	FileMaxBackups int

	// SyslogAddress is udp://host:port, tcp://host:port or tls://host:port.
	SyslogAddress  string
	SyslogFacility string
	// GELFAddress is udp://host:port or tcp://host:port (Graylog).
	GELFAddress string
	// OTLPEndpoint is the URL of the OTLP/HTTP logs endpoint, such as
	// http://otel-collector:4318/v1/logs.
	OTLPEndpoint string
	OTLPHeaders  map[string]string
	// LokiURL is the base URL of Grafana Loki.
	LokiURL    string
	LokiLabels map[string]string
	LokiTenant string
	// HTTPURL receives batches of JSON records.
	HTTPURL     string
	HTTPHeaders map[string]string
	// HTTPFormat is "ndjson" (one record per line) or "json" (an array).
	HTTPFormat string

	// Buffer is the number of records kept per remote output while it is
	// slow or down; the records beyond are dropped and counted.
	Buffer int
	// BatchSize and FlushInterval decide when a batch is sent.
	BatchSize     int
	FlushInterval time.Duration
	// Resource holds attributes added to every record sent to a remote
	// output (OTEL_RESOURCE_ATTRIBUTES).
	Resource map[string]string
}

// Load reads the configuration from the environment.
func Load() (*Config, error) {
	return LoadFrom(os.LookupEnv, os.ReadFile)
}

// LoadFrom reads the configuration with lookup, which returns the value of
// a variable, and readFile, which reads the files of the _FILE variables.
func LoadFrom(lookup func(string) (string, bool), readFile func(string) ([]byte, error)) (*Config, error) {
	e := &env{lookup: lookup, readFile: readFile}
	c := &Config{
		MaxBodyBytes:    e.bytes("MAX_BODY_BYTES", 32<<20),
		ReadTimeout:     e.duration("READ_TIMEOUT", time.Minute),
		WriteTimeout:    e.duration("WRITE_TIMEOUT", 2*time.Minute),
		IdleTimeout:     e.duration("IDLE_TIMEOUT", 2*time.Minute),
		ShutdownTimeout: e.duration("SHUTDOWN_TIMEOUT", 20*time.Second),
		TLSCertFile:     e.str("TLS_CERT_FILE", ""),
		TLSKeyFile:      e.str("TLS_KEY_FILE", ""),
		ServiceName:     e.str("SERVICE_NAME", e.raw("OTEL_SERVICE_NAME", "zerechnung")),
		Metrics:         e.bool("METRICS", true),
		CAFile:          e.str("CA_FILE", ""),
	}
	c.Addr = e.addr()
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		e.fail("ZR_TLS_CERT_FILE and ZR_TLS_KEY_FILE go together")
	}

	// Authentication.
	a := &c.Auth
	a.Realm = e.str("AUTH_REALM", "zerechnung")
	a.BasicUsers = string(e.secret("BASIC_USERS"))
	a.JWT = JWT{
		Secret:         e.secret("JWT_SECRET"),
		PublicKeys:     e.secret("JWT_PUBLIC_KEYS"),
		JWKSURL:        e.str("JWT_JWKS_URL", ""),
		JWKSRefresh:    e.duration("JWT_JWKS_REFRESH", 15*time.Minute),
		Issuers:        e.list("JWT_ISSUER"),
		Audiences:      e.list("JWT_AUDIENCE"),
		Algorithms:     e.list("JWT_ALGORITHMS"),
		Leeway:         e.duration("JWT_LEEWAY", time.Minute),
		RequiredScopes: e.list("JWT_REQUIRED_SCOPES"),
	}
	if n := len(a.JWT.Secret); n > 0 && n < 32 {
		e.fail(fmt.Sprintf("ZR_JWT_SECRET holds %d bytes: an HMAC key needs at least 32", n))
	}
	a.Methods = e.list("AUTH")
	if len(a.Methods) == 0 {
		if a.JWT.Configured() {
			a.Methods = append(a.Methods, "jwt")
		}
		if a.BasicUsers != "" {
			a.Methods = append(a.Methods, "basic")
		}
	}
	e.checkAuth(a)

	// Signature.
	s := &c.Sign
	s.Key = e.pem("SIGN_KEY")
	s.KeyPassword = string(e.secret("SIGN_KEY_PASSWORD"))
	s.Cert = e.pem("SIGN_CERT")
	s.PKCS12 = e.binary("SIGN_PKCS12")
	s.PKCS12Password = string(e.secret("SIGN_PKCS12_PASSWORD"))
	s.Hash = strings.ToLower(e.str("SIGN_HASH", "sha256"))
	s.PSS = e.bool("SIGN_RSA_PSS", false)
	s.Reason = e.str("SIGN_REASON", "")
	s.Location = e.str("SIGN_LOCATION", "")
	s.ContactInfo = e.str("SIGN_CONTACT", "")
	s.Name = e.str("SIGN_NAME", "")
	s.Default = e.bool("SIGN_DEFAULT", true)
	switch {
	case len(s.Key) > 0 && len(s.PKCS12) > 0:
		e.fail("set either ZR_SIGN_KEY or ZR_SIGN_PKCS12, not both")
	case len(s.Key) > 0 && len(s.Cert) == 0:
		e.fail("ZR_SIGN_KEY needs ZR_SIGN_CERT, the signing certificate")
	case len(s.Cert) > 0 && !s.Configured():
		e.fail("ZR_SIGN_CERT needs ZR_SIGN_KEY, the private key")
	}
	switch s.Hash {
	case "sha256", "sha384", "sha512":
	default:
		e.fail(fmt.Sprintf("ZR_SIGN_HASH: %q is not sha256, sha384 nor sha512", s.Hash))
	}

	// Logs.
	l := &c.Log
	l.Level = e.level("LOG_LEVEL", slog.LevelInfo)
	l.Format = strings.ToLower(e.str("LOG_FORMAT", "json"))
	if l.Format != "json" && l.Format != "text" {
		e.fail(fmt.Sprintf("ZR_LOG_FORMAT: %q is not json nor text", l.Format))
	}
	l.Outputs = e.list("LOG_OUTPUTS")
	if len(l.Outputs) == 0 {
		l.Outputs = []string{"stdout"}
	}
	l.File = e.str("LOG_FILE", "")
	l.FileMaxBytes = e.bytes("LOG_FILE_MAX_BYTES", 100<<20)
	l.FileMaxBackups = e.int("LOG_FILE_MAX_BACKUPS", 5)
	l.SyslogAddress = e.str("LOG_SYSLOG_ADDRESS", "")
	l.SyslogFacility = strings.ToLower(e.str("LOG_SYSLOG_FACILITY", "local0"))
	l.GELFAddress = e.str("LOG_GELF_ADDRESS", "")
	l.OTLPEndpoint = e.str("LOG_OTLP_ENDPOINT", otlpEndpoint(e))
	l.OTLPHeaders = e.pairs("LOG_OTLP_HEADERS", e.raw("OTEL_EXPORTER_OTLP_LOGS_HEADERS", e.raw("OTEL_EXPORTER_OTLP_HEADERS", "")))
	l.LokiURL = e.str("LOG_LOKI_URL", "")
	l.LokiLabels = e.pairs("LOG_LOKI_LABELS", "")
	l.LokiTenant = e.str("LOG_LOKI_TENANT", "")
	l.HTTPURL = e.str("LOG_HTTP_URL", "")
	l.HTTPHeaders = e.pairs("LOG_HTTP_HEADERS", "")
	l.HTTPFormat = strings.ToLower(e.str("LOG_HTTP_FORMAT", "ndjson"))
	l.Buffer = e.int("LOG_BUFFER", 10000)
	l.BatchSize = e.int("LOG_BATCH_SIZE", 200)
	l.FlushInterval = e.duration("LOG_FLUSH_INTERVAL", 2*time.Second)
	l.Resource = e.pairs("", e.raw("OTEL_RESOURCE_ATTRIBUTES", ""))
	e.checkLog(l)

	if len(e.errs) > 0 {
		return nil, errors.Join(e.errs...)
	}
	return c, nil
}

// otlpEndpoint follows the OpenTelemetry variables: the logs endpoint as
// is, or the base endpoint followed by /v1/logs.
func otlpEndpoint(e *env) string {
	if v := e.raw("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", ""); v != "" {
		return v
	}
	if v := e.raw("OTEL_EXPORTER_OTLP_ENDPOINT", ""); v != "" {
		return strings.TrimSuffix(v, "/") + "/v1/logs"
	}
	return ""
}

func (e *env) checkAuth(a *Auth) {
	seen := map[string]bool{}
	for _, m := range a.Methods {
		m = strings.ToLower(m)
		switch m {
		case "jwt":
			if !a.JWT.Configured() {
				e.fail("ZR_AUTH=jwt needs ZR_JWT_SECRET, ZR_JWT_PUBLIC_KEYS or ZR_JWT_JWKS_URL")
			}
		case "basic":
			if a.BasicUsers == "" {
				e.fail("ZR_AUTH=basic needs ZR_BASIC_USERS")
			}
		case "none":
			if len(a.Methods) > 1 {
				e.fail("ZR_AUTH=none cannot be combined with another method")
			}
		default:
			e.fail(fmt.Sprintf("ZR_AUTH: unknown method %q (jwt, basic, none)", m))
		}
		seen[m] = true
	}
	if len(a.Methods) == 0 {
		e.fail("no authentication is configured: set ZR_JWT_SECRET, ZR_JWT_PUBLIC_KEYS, ZR_JWT_JWKS_URL or ZR_BASIC_USERS, or ZR_AUTH=none to accept anonymous requests")
	}
	a.Methods = a.Methods[:0]
	for _, m := range []string{"jwt", "basic", "none"} {
		if seen[m] {
			a.Methods = append(a.Methods, m)
		}
	}
}

func (e *env) checkLog(l *Log) {
	need := map[string]struct {
		value, name string
	}{
		"file":   {l.File, "ZR_LOG_FILE"},
		"syslog": {l.SyslogAddress, "ZR_LOG_SYSLOG_ADDRESS"},
		"gelf":   {l.GELFAddress, "ZR_LOG_GELF_ADDRESS"},
		"otlp":   {l.OTLPEndpoint, "ZR_LOG_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT"},
		"loki":   {l.LokiURL, "ZR_LOG_LOKI_URL"},
		"http":   {l.HTTPURL, "ZR_LOG_HTTP_URL"},
	}
	for i, o := range l.Outputs {
		o = strings.ToLower(o)
		l.Outputs[i] = o
		switch o {
		case "stdout", "stderr", "none":
		default:
			n, ok := need[o]
			if !ok {
				e.fail(fmt.Sprintf("ZR_LOG_OUTPUTS: unknown output %q (stdout, stderr, file, syslog, gelf, otlp, loki, http, none)", o))
			} else if n.value == "" {
				e.fail(fmt.Sprintf("the %s log output needs %s", o, n.name))
			}
		}
	}
	if l.HTTPFormat != "ndjson" && l.HTTPFormat != "json" {
		e.fail(fmt.Sprintf("ZR_LOG_HTTP_FORMAT: %q is not ndjson nor json", l.HTTPFormat))
	}
	if l.Buffer < 1 || l.BatchSize < 1 || l.FlushInterval <= 0 {
		e.fail("ZR_LOG_BUFFER, ZR_LOG_BATCH_SIZE and ZR_LOG_FLUSH_INTERVAL must be positive")
	}
	switch l.SyslogFacility {
	case "kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news", "uucp", "cron", "authpriv", "ftp",
		"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7":
	default:
		e.fail(fmt.Sprintf("ZR_LOG_SYSLOG_FACILITY: unknown facility %q", l.SyslogFacility))
	}
}

// env reads the variables and collects the errors, so that a single run
// reports every mistake.
type env struct {
	lookup   func(string) (string, bool)
	readFile func(string) ([]byte, error)
	errs     []error
}

func (e *env) fail(msg string) { e.errs = append(e.errs, errors.New(msg)) }

// raw returns the value of a variable without prefix.
func (e *env) raw(name, def string) string {
	if v, ok := e.lookup(name); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (e *env) str(key, def string) string { return e.raw(Prefix+key, def) }

// secret returns the value of ZR_key, or the content of the file named by
// ZR_key_FILE without its final line break.
func (e *env) secret(key string) []byte {
	v, hasValue := e.lookup(Prefix + key)
	f, hasFile := e.lookup(Prefix + key + "_FILE")
	hasValue = hasValue && v != ""
	hasFile = hasFile && f != ""
	switch {
	case hasValue && hasFile:
		e.fail(fmt.Sprintf("set either %s%s or %s%s_FILE, not both", Prefix, key, Prefix, key))
		return nil
	case hasFile:
		b, err := e.readFile(f)
		if err != nil {
			e.fail(fmt.Sprintf("%s%s_FILE: %v", Prefix, key, err))
			return nil
		}
		return []byte(strings.TrimRight(string(b), "\r\n"))
	case hasValue:
		return []byte(v)
	}
	return nil
}

// pem returns a secret that holds PEM blocks, given as is or encoded in
// base64, which fits in a single-line variable.
func (e *env) pem(key string) []byte {
	b := e.secret(key)
	if len(b) == 0 || strings.Contains(string(b), "-----BEGIN") {
		return b
	}
	d, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(b)), ""))
	if err != nil || !strings.Contains(string(d), "-----BEGIN") {
		e.fail(fmt.Sprintf("%s%s holds neither PEM blocks nor PEM encoded in base64", Prefix, key))
		return nil
	}
	return d
}

// binary returns a secret holding binary data: a file read as is, or a
// value encoded in base64.
func (e *env) binary(key string) []byte {
	if f, ok := e.lookup(Prefix + key + "_FILE"); ok && f != "" {
		if v, ok := e.lookup(Prefix + key); ok && v != "" {
			e.fail(fmt.Sprintf("set either %s%s or %s%s_FILE, not both", Prefix, key, Prefix, key))
			return nil
		}
		b, err := e.readFile(f)
		if err != nil {
			e.fail(fmt.Sprintf("%s%s_FILE: %v", Prefix, key, err))
			return nil
		}
		return b
	}
	v, ok := e.lookup(Prefix + key)
	if !ok || v == "" {
		return nil
	}
	d, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(v), ""))
	if err != nil {
		e.fail(fmt.Sprintf("%s%s: not base64: %v", Prefix, key, err))
		return nil
	}
	return d
}

func (e *env) int(key string, def int) int {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.fail(fmt.Sprintf("%s%s: %q is not an integer", Prefix, key, v))
		return def
	}
	return n
}

// bytes reads a size in bytes, with an optional unit: 32MiB, 10MB, 512KiB.
func (e *env) bytes(key string, def int64) int64 {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	n, err := parseBytes(v)
	if err != nil {
		e.fail(fmt.Sprintf("%s%s: %v", Prefix, key, err))
		return def
	}
	return n
}

func parseBytes(v string) (int64, error) {
	units := []struct {
		suffix string
		factor int64
	}{
		{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30},
		{"KB", 1000}, {"MB", 1000 * 1000}, {"GB", 1000 * 1000 * 1000},
		{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"B", 1},
	}
	s := strings.ToUpper(strings.TrimSpace(v))
	factor := int64(1)
	for _, u := range units {
		if rest, ok := strings.CutSuffix(s, u.suffix); ok {
			s, factor = strings.TrimSpace(rest), u.factor
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a positive size", v)
	}
	return n * factor, nil
}

func (e *env) bool(key string, def bool) bool {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.fail(fmt.Sprintf("%s%s: %q is not a boolean", Prefix, key, v))
		return def
	}
	return b
}

func (e *env) duration(key string, def time.Duration) time.Duration {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		e.fail(fmt.Sprintf("%s%s: %q is not a positive duration (30s, 5m)", Prefix, key, v))
		return def
	}
	return d
}

func (e *env) level(key string, def slog.Level) slog.Level {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		e.fail(fmt.Sprintf("%s%s: %q is not debug, info, warn nor error", Prefix, key, v))
		return def
	}
	return l
}

// list splits a value on commas.
func (e *env) list(key string) []string {
	var out []string
	for s := range strings.SplitSeq(e.str(key, ""), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// pairs reads key=value pairs separated by commas, from ZR_key or, when it
// is not set, from def. The values may be percent-encoded, as in the
// OpenTelemetry variables.
func (e *env) pairs(key, def string) map[string]string {
	v := def
	if key != "" {
		v = e.str(key, def)
	}
	out := map[string]string{}
	for p := range strings.SplitSeq(v, ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		k, val, ok := strings.Cut(p, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			name := Prefix + key
			if key == "" {
				name = "OTEL_RESOURCE_ATTRIBUTES"
			}
			e.fail(fmt.Sprintf("%s: %q is not key=value", name, p))
			continue
		}
		out[k] = unescape(strings.TrimSpace(val))
	}
	return out
}

// unescape decodes the %XX sequences of a value, leaving it as is when it
// holds an invalid one.
func unescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// addr returns the listen address: ZR_ADDR, or ZR_HOST and ZR_PORT, or the
// PORT variable that many platforms set.
func (e *env) addr() string {
	if a := e.str("ADDR", ""); a != "" {
		if _, _, err := net.SplitHostPort(a); err != nil {
			e.fail(fmt.Sprintf("ZR_ADDR: %q is not host:port", a))
		}
		return a
	}
	port := e.str("PORT", e.raw("PORT", "8080"))
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		e.fail(fmt.Sprintf("ZR_PORT: %q is not a port number", port))
	}
	return net.JoinHostPort(e.str("HOST", ""), port)
}
