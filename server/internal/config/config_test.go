package config

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testPEM = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"

func load(t *testing.T, vars map[string]string, files map[string]string) (*Config, error) {
	t.Helper()
	return LoadFrom(func(k string) (string, bool) {
		v, ok := vars[k]
		return v, ok
	}, func(name string) ([]byte, error) {
		if v, ok := files[name]; ok {
			return []byte(v), nil
		}
		return nil, fs.ErrNotExist
	})
}

func TestDefaults(t *testing.T) {
	c, err := load(t, map[string]string{"ZR_AUTH": "none"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.MaxBodyBytes != 32<<20 || c.ShutdownTimeout != 20*time.Second || !c.Metrics || c.ServiceName != "zerechnung" {
		t.Errorf("%+v", c)
	}
	if c.Log.Level != slog.LevelInfo || c.Log.Format != "json" || len(c.Log.Outputs) != 1 || c.Log.Outputs[0] != "stdout" {
		t.Errorf("%+v", c.Log)
	}
	if c.Sign.Hash != "sha256" || !c.Sign.Default || c.Sign.Configured() {
		t.Errorf("%+v", c.Sign)
	}
}

func TestPortAndAddress(t *testing.T) {
	for vars, want := range map[[2]string]string{
		{"PORT", "9000"}:          ":9000",
		{"ZR_PORT", "8443"}:       ":8443",
		{"ZR_ADDR", "[::1]:7000"}: "[::1]:7000",
	} {
		c, err := load(t, map[string]string{"ZR_AUTH": "none", vars[0]: vars[1]}, nil)
		if err != nil || c.Addr != want {
			t.Errorf("%s=%s: %v %v", vars[0], vars[1], c, err)
		}
	}
	c, err := load(t, map[string]string{"ZR_AUTH": "none", "PORT": "9000", "ZR_PORT": "8081", "ZR_HOST": "127.0.0.1"}, nil)
	if err != nil || c.Addr != "127.0.0.1:8081" {
		t.Errorf("ZR_PORT wins over PORT: %v %v", c, err)
	}
}

func TestAuthInference(t *testing.T) {
	secret := strings.Repeat("k", 32)
	c, err := load(t, map[string]string{"ZR_JWT_SECRET": secret, "ZR_BASIC_USERS_FILE": "/run/secrets/users", "ZR_JWT_AUDIENCE": "billing, archive"},
		map[string]string{"/run/secrets/users": "svc:sha256:abcd\n"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.Auth.Methods, ",") != "jwt,basic" || c.Auth.BasicUsers != "svc:sha256:abcd" || len(c.Auth.JWT.Audiences) != 2 || c.Auth.JWT.Audiences[1] != "archive" {
		t.Errorf("%+v", c.Auth)
	}
	// No authentication at all is refused: anonymous access must be asked
	// for.
	if _, err := load(t, nil, nil); err == nil || !strings.Contains(err.Error(), "ZR_AUTH=none") {
		t.Errorf("no auth: %v", err)
	}
}

func TestSigningKeys(t *testing.T) {
	// PEM given as is, or in base64 to fit a single-line variable.
	c, err := load(t, map[string]string{
		"ZR_AUTH": "none", "ZR_SIGN_KEY_FILE": "/k.pem", "ZR_SIGN_CERT": base64.StdEncoding.EncodeToString([]byte(testPEM)),
		"ZR_SIGN_HASH": "SHA384", "ZR_SIGN_DEFAULT": "false",
	}, map[string]string{"/k.pem": testPEM})
	if err != nil {
		t.Fatal(err)
	}
	if string(c.Sign.Key) != strings.TrimRight(testPEM, "\n") || string(c.Sign.Cert) != testPEM || c.Sign.Hash != "sha384" || c.Sign.Default {
		t.Errorf("%+v", c.Sign)
	}
	// A PKCS#12 file is read as is; in a variable, it is base64.
	c, err = load(t, map[string]string{"ZR_AUTH": "none", "ZR_SIGN_PKCS12": base64.StdEncoding.EncodeToString([]byte{0x30, 0x82, 0, 1})}, nil)
	if err != nil || len(c.Sign.PKCS12) != 4 || !c.Sign.Configured() {
		t.Errorf("pkcs12: %v %v", c, err)
	}
	c, err = load(t, map[string]string{"ZR_AUTH": "none", "ZR_SIGN_PKCS12_FILE": "/c.p12"}, map[string]string{"/c.p12": "\x30\x82\n"})
	if err != nil || string(c.Sign.PKCS12) != "\x30\x82\n" {
		t.Errorf("pkcs12 file: %v %v", c, err)
	}
}

func TestLogOutputs(t *testing.T) {
	c, err := load(t, map[string]string{
		"ZR_AUTH": "none", "ZR_LOG_OUTPUTS": "stdout, Loki, otlp", "ZR_LOG_LEVEL": "debug",
		"ZR_LOG_LOKI_URL": "http://loki:3100", "ZR_LOG_LOKI_LABELS": "env=prod,team=billing",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318/", "OTEL_EXPORTER_OTLP_HEADERS": "authorization=Bearer%20abc",
		"OTEL_RESOURCE_ATTRIBUTES": "deployment.environment=prod",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := c.Log
	if strings.Join(l.Outputs, ",") != "stdout,loki,otlp" || l.Level != slog.LevelDebug || l.OTLPEndpoint != "http://collector:4318/v1/logs" ||
		l.OTLPHeaders["authorization"] != "Bearer abc" || l.LokiLabels["team"] != "billing" || l.Resource["deployment.environment"] != "prod" {
		t.Errorf("%+v", l)
	}
}

func TestErrorsAreCollected(t *testing.T) {
	_, err := load(t, map[string]string{
		"ZR_AUTH":             "jwt,ldap",
		"ZR_JWT_SECRET":       "short",
		"ZR_PORT":             "http",
		"ZR_SIGN_KEY":         testPEM,
		"ZR_SIGN_HASH":        "md5",
		"ZR_LOG_OUTPUTS":      "syslog,kafka",
		"ZR_LOG_LEVEL":        "loud",
		"ZR_MAX_BODY_BYTES":   "-1",
		"ZR_READ_TIMEOUT":     "soon",
		"ZR_BASIC_USERS":      "a:b",
		"ZR_BASIC_USERS_FILE": "/missing",
	}, nil)
	if err == nil {
		t.Fatal("no error")
	}
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Fatalf("not joined: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"ZR_JWT_SECRET holds 5 bytes", "unknown method \"ldap\"", "ZR_PORT", "ZR_SIGN_KEY needs ZR_SIGN_CERT", "ZR_SIGN_HASH",
		"the syslog log output needs ZR_LOG_SYSLOG_ADDRESS", "unknown output \"kafka\"", "ZR_LOG_LEVEL", "ZR_MAX_BODY_BYTES",
		"ZR_READ_TIMEOUT", "either ZR_BASIC_USERS or ZR_BASIC_USERS_FILE",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("lacks %q:\n%s", want, msg)
		}
	}
}

func TestSizes(t *testing.T) {
	for in, want := range map[string]int64{"1024": 1024, "64MiB": 64 << 20, "10MB": 10e6, "2g": 2 << 30, "512 KiB": 512 << 10} {
		if n, err := parseBytes(in); err != nil || n != want {
			t.Errorf("%s: %d %v", in, n, err)
		}
	}
	for _, in := range []string{"", "0", "-5", "ten", "5TB"} {
		if _, err := parseBytes(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}
