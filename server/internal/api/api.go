// Package api is the HTTP API of the server: it creates Factur-X invoices
// end to end (rendering, XML, embedding, signature), validates, signs,
// verifies and converts documents.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ritoon/zerechnung/server/internal/auth"
	"github.com/ritoon/zerechnung/server/internal/config"
	"github.com/ritoon/zerechnung/server/internal/keys"
	"github.com/ritoon/zerechnung/server/internal/metrics"
)

// Deps are what the API needs.
type Deps struct {
	Logger  *slog.Logger
	Auth    *auth.Authenticator
	Metrics *metrics.Registry
	// Signer signs the documents; nil when no key is configured.
	Signer *keys.Signer
	Sign   config.Sign
	// MaxBodyBytes bounds the request bodies.
	MaxBodyBytes int64
	// MetricsEnabled serves /metrics.
	MetricsEnabled bool
	Version        string
	// Now gives the current time; time.Now when nil.
	Now func() time.Time
}

// Server serves the API.
type Server struct {
	Deps
	mux      *http.ServeMux
	stopping atomic.Bool
	openapi  []byte
}

// New builds the server and its routes.
func New(d Deps) (*Server, error) {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &Server{Deps: d, mux: http.NewServeMux()}
	doc, err := openAPI(d.Version)
	if err != nil {
		return nil, fmt.Errorf("OpenAPI document: %w", err)
	}
	s.openapi = doc

	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.mux.HandleFunc("GET /readyz", s.readyz)
	s.mux.HandleFunc("GET /openapi.json", s.openAPIHandler)
	if d.MetricsEnabled {
		s.mux.HandleFunc("GET /metrics", s.metricsHandler)
	}
	protected := func(pattern string, h http.HandlerFunc) {
		s.mux.Handle(pattern, d.Auth.Middleware(s.limit(h)))
	}

	protected("GET /v1/info", s.info)
	protected("POST /v1/invoices", s.createInvoice)
	protected("POST /v1/xml", s.writeXML)
	protected("POST /v1/validate", s.validate)
	protected("POST /v1/sign", s.sign)
	protected("POST /v1/verify", s.verify)
	protected("POST /v1/extract", s.extract)
	protected("POST /v1/pdfa", s.convertPDFA)
	return s, nil
}

// Handler returns the handler of the server, with its middlewares.
func (s *Server) Handler() http.Handler {
	return s.recover(s.observe(s.mux))
}

// Stopping marks the server as stopping: /readyz answers 503 so that the
// load balancers stop sending requests while those in flight end.
func (s *Server) Stopping() { s.stopping.Store(true) }

// limit bounds the request body, and records the authenticated caller
// for the access log.
func (s *Server) limit(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ri := reqInfoOf(r); ri != nil {
			ri.principal = auth.FromContext(r.Context())
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.MaxBodyBytes)
		h.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

// reqInfo is what the access log needs to know of a request.
type reqInfo struct {
	id        string
	principal *auth.Principal
}

func reqInfoOf(r *http.Request) *reqInfo {
	ri, _ := r.Context().Value(ctxKey{}).(*reqInfo)
	return ri
}

// requestID returns the identifier of a request.
func requestID(r *http.Request) string {
	if ri := reqInfoOf(r); ri != nil {
		return ri.id
	}
	return ""
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:\-]{1,128}$`)

// traceparent reads a W3C trace context header (version 00): the trace
// and parent span identifiers, so that the logs join the traces.
func traceparent(h string) (traceID, spanID string) {
	p := strings.Split(strings.TrimSpace(h), "-")
	if len(p) != 4 || len(p[0]) != 2 || len(p[1]) != 32 || len(p[2]) != 16 || len(p[3]) != 2 || p[0] == "ff" {
		return "", ""
	}
	for _, s := range p[:3] {
		if _, err := hex.DecodeString(s); err != nil {
			return "", ""
		}
	}
	if strings.Trim(p[1], "0") == "" || strings.Trim(p[2], "0") == "" {
		return "", ""
	}
	return p[1], p[2]
}

// recorder keeps the status and the size of a response.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// observe gives each request an identifier (X-Request-ID, kept when the
// caller sends a valid one), logs it and counts it.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		ri := &reqInfo{id: id}
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, ri))
		rec := &recorder{ResponseWriter: w}
		s.Metrics.InFlight(1)
		next.ServeHTTP(rec, r)
		s.Metrics.InFlight(-1)

		d := time.Since(start)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		} else if _, path, ok := strings.Cut(route, " "); ok {
			route = path
		}
		s.Metrics.Add("zerechnung_http_requests_total", "HTTP requests by route, method and status.",
			metrics.Labels("route", route, "method", r.Method, "status", strconv.Itoa(rec.status)), 1)
		s.Metrics.Observe("zerechnung_http_request_duration_seconds", "Duration of the HTTP requests.",
			metrics.Labels("route", route, "method", r.Method), d)

		// The probes and the scrapes would drown the other requests.
		level := slog.LevelInfo
		switch {
		case route == "/healthz" || route == "/readyz" || route == "/metrics":
			level = slog.LevelDebug
		case rec.status >= 500:
			level = slog.LevelError
		case rec.status >= 400:
			level = slog.LevelWarn
		}
		attrs := []slog.Attr{
			slog.String("request_id", id),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("route", route),
			slog.Int("status", rec.status),
			slog.Int64("bytes_in", max(r.ContentLength, 0)),
			slog.Int64("bytes_out", rec.bytes),
			slog.Float64("duration_ms", float64(d.Microseconds())/1000),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}
		if p := ri.principal; p != nil {
			attrs = append(attrs, slog.String("auth_method", p.Method), slog.String("subject", p.Subject))
		}
		if tid, sid := traceparent(r.Header.Get("traceparent")); tid != "" {
			attrs = append(attrs, slog.String("trace_id", tid), slog.String("span_id", sid))
		}
		s.Logger.LogAttrs(r.Context(), level, "request", attrs...)
	})
}

// recover turns a panic into a 500 response and an error log.
func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Logger.Error("panic", "request_id", requestID(r), "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				s.problem(w, r, http.StatusInternalServerError, "internal_error", "internal error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Problem is an error response (RFC 9457).
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
	// Report is the validation report of a refused invoice.
	Report *Report `json:"report,omitempty"`
	// Reasons are why a PDF cannot be converted to PDF/A-3.
	Reasons []string `json:"reasons,omitempty"`
}

func (s *Server) problem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	s.writeProblem(w, r, &Problem{Status: status, Code: code, Title: title, Detail: detail})
}

func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, p *Problem) {
	p.Type = "about:blank"
	p.RequestID = requestID(r)
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Del("Content-Disposition")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// bodyError answers the errors of reading a request body: 413 when it is
// too large, 400 otherwise.
func (s *Server) bodyError(w http.ResponseWriter, r *http.Request, err error) {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		s.problem(w, r, http.StatusRequestEntityTooLarge, "too_large", "request body too large",
			fmt.Sprintf("the limit is %d bytes (ZR_MAX_BODY_BYTES)", mbe.Limit))
		return
	}
	s.problem(w, r, http.StatusBadRequest, "bad_request", "malformed request", err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	var reasons []string
	if s.stopping.Load() {
		reasons = append(reasons, "shutting down")
	}
	if !s.Auth.Ready() {
		reasons = append(reasons, "the JWKS is not downloaded yet")
	}
	if len(reasons) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not ready", "reasons": reasons})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.Metrics.Write(w)
}

func (s *Server) openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.openapi)
}
