// Command zerechnung-server serves the HTTP API of ZeRechnung: it creates
// Factur-X invoices end to end (PDF/A-3 rendering, CII XML, PAdES
// signature), validates, signs, verifies and converts e-invoices.
//
//	zerechnung-server              serve the API (the default)
//	zerechnung-server serve        serve the API
//	zerechnung-server healthcheck  exit 0 when the local server answers /healthz
//	zerechnung-server version      print the version
//
// The configuration is read from environment variables (ZR_*), described
// in the README of the server and by the OpenAPI document on /openapi.json.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/ritoon/zerechnung/server/internal/api"
	"github.com/ritoon/zerechnung/server/internal/auth"
	"github.com/ritoon/zerechnung/server/internal/config"
	"github.com/ritoon/zerechnung/server/internal/keys"
	"github.com/ritoon/zerechnung/server/internal/logging"
	"github.com/ritoon/zerechnung/server/internal/metrics"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = ""

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "zerechnung-server:", err)
			os.Exit(1)
		}
	case "healthcheck":
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "zerechnung-server: healthcheck:", err)
			os.Exit(1)
		}
	case "version", "-version", "--version":
		fmt.Printf("zerechnung-server %s (%s, %s/%s)\n", getVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "-help", "--help":
		fmt.Print("usage: zerechnung-server [serve|healthcheck|version]\n\nThe configuration comes from the ZR_* environment variables.\n")
	default:
		fmt.Fprintf(os.Stderr, "zerechnung-server: unknown command %q (serve, healthcheck, version)\n", cmd)
		os.Exit(2)
	}
}

func getVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return "dev-" + s.Value[:12]
			}
		}
	}
	return "dev"
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}
	ver := getVersion()

	// Outgoing TLS (JWKS, log outputs): the system roots, plus ZR_CA_FILE.
	tlsConf := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return fmt.Errorf("ZR_CA_FILE: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("ZR_CA_FILE: no PEM certificate in %s", cfg.CAFile)
		}
		tlsConf.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConf
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	host, _ := os.Hostname()
	logs, err := logging.New(&cfg.Log, logging.Info{Service: cfg.ServiceName, Version: ver, Hostname: host, Resource: cfg.Log.Resource}, client, tlsConf)
	if err != nil {
		return err
	}
	logger := logs.Logger
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = logs.Close(ctx)
	}()
	fail := func(err error) error {
		logger.Error("startup failed", "error", err)
		return err
	}

	signer, err := keys.Load(&cfg.Sign)
	if err != nil {
		return fail(err)
	}
	authn, err := auth.New(&cfg.Auth, client, logger)
	if err != nil {
		return fail(err)
	}
	reg := metrics.New(map[string]string{"version": ver, "goversion": runtime.Version()})
	reg.Collect(func(w io.Writer) {
		fmt.Fprint(w, "# HELP zerechnung_log_records_total Log records of the remote outputs by result.\n# TYPE zerechnung_log_records_total counter\n")
		for _, s := range logs.Stats() {
			fmt.Fprintf(w, "zerechnung_log_records_total{output=%q,result=\"sent\"} %d\n", s.Output, s.Sent)
			fmt.Fprintf(w, "zerechnung_log_records_total{output=%q,result=\"dropped\"} %d\n", s.Output, s.Dropped)
			fmt.Fprintf(w, "zerechnung_log_records_total{output=%q,result=\"failed\"} %d\n", s.Output, s.Failed)
		}
	})
	srv, err := api.New(api.Deps{
		Logger: logger, Auth: authn, Metrics: reg, Signer: signer, Sign: cfg.Sign,
		MaxBodyBytes: cfg.MaxBodyBytes, MetricsEnabled: cfg.Metrics, Version: ver,
	})
	if err != nil {
		return fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go authn.Run(ctx)

	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fail(err)
	}
	startup := []any{
		"version", ver, "addr", ln.Addr().String(), "tls", cfg.TLSCertFile != "",
		"auth", strings.Join(authn.Methods(), ","), "log_outputs", strings.Join(cfg.Log.Outputs, ","),
		"metrics", cfg.Metrics, "go", runtime.Version(), "os", runtime.GOOS + "/" + runtime.GOARCH,
	}
	if signer != nil {
		c := signer.Certificate()
		startup = append(startup, "signing_key", signer.Algorithm(), "signing_cert", c.Subject.String(),
			"signing_cert_expiry", c.NotAfter.UTC().Format(time.RFC3339))
		switch err := signer.CheckValidity(time.Now()); {
		case err != nil:
			logger.Error("the signing certificate is not valid: the signature requests will fail", "error", err)
		case time.Until(c.NotAfter) < 30*24*time.Hour:
			logger.Warn("the signing certificate expires soon", "expiry", c.NotAfter.UTC().Format(time.RFC3339))
		}
	} else {
		startup = append(startup, "signing_key", "none")
	}
	if len(authn.Methods()) == 1 && authn.Methods()[0] == "none" {
		logger.Warn("authentication is disabled (ZR_AUTH=none): any client can use the API")
	}
	logger.Info("zerechnung-server started", startup...)

	errc := make(chan error, 1)
	go func() {
		if cfg.TLSCertFile != "" {
			errc <- hs.ServeTLS(ln, cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			errc <- hs.Serve(ln)
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return fail(err)
		}
	case <-ctx.Done():
	}
	logger.Info("stopping", "timeout", cfg.ShutdownTimeout.String())
	srv.Stopping()
	sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil {
		logger.Warn("requests interrupted by the shutdown", "error", err)
	}
	logger.Info("stopped")
	return nil
}

// healthcheck asks the local server for /healthz: the HEALTHCHECK of the
// container image, which has no shell nor curl.
func healthcheck() error {
	addr := os.Getenv("ZR_ADDR")
	if addr == "" {
		port := os.Getenv("ZR_PORT")
		if port == "" {
			port = os.Getenv("PORT")
		}
		if port == "" {
			port = "8080"
		}
		addr = net.JoinHostPort("", port)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	scheme := "http"
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("ZR_TLS_CERT_FILE") != "" {
		scheme = "https"
		// The certificate names the public host, not the loopback address.
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // local probe only
	}
	c := &http.Client{Timeout: 3 * time.Second, Transport: tr}
	resp, err := c.Get(scheme + "://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}
