package logging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

var info = Info{Service: "zerechnung", Version: "1.2.3", Hostname: "node-1", Resource: map[string]string{"deployment.environment": "test"}}

func baseConfig(outputs ...string) *config.Log {
	return &config.Log{Level: slog.LevelDebug, Format: "json", Outputs: outputs, Buffer: 100, BatchSize: 10, FlushInterval: 20 * time.Millisecond, FileMaxBytes: 1 << 20, FileMaxBackups: 2, SyslogFacility: "local0", HTTPFormat: "ndjson"}
}

func logSome(t *testing.T, c *config.Log) *Logging {
	t.Helper()
	l, err := New(c, info, http.DefaultClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	lg := l.Logger.With("request_id", "r-1").WithGroup("http")
	lg.Info("request", "status", 201, "duration", 15*time.Millisecond, slog.Group("auth", "method", "jwt"))
	l.Logger.Error("signature failed", "error", io.ErrUnexpectedEOF, "trace_id", "4bf92f3577b34da6a3ce929d0e0e4736", "span_id", "00f067aa0ba902b7")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.Close(ctx); err != nil {
		t.Fatal(err)
	}
	return l
}

// collector is an HTTP server that keeps the bodies it receives.
type collector struct {
	mu     sync.Mutex
	bodies []string
	header http.Header
	status int
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, r.URL.Path+" "+string(b))
	c.header = r.Header.Clone()
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
}

func (c *collector) all() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.bodies, "\n")
}

func TestHTTPOutputs(t *testing.T) {
	col := &collector{}
	srv := httptest.NewServer(col)
	defer srv.Close()

	c := baseConfig("otlp", "loki", "http")
	c.OTLPEndpoint = srv.URL + "/v1/logs"
	c.OTLPHeaders = map[string]string{"Authorization": "Bearer otlp"}
	c.LokiURL = srv.URL
	c.LokiTenant = "billing"
	c.LokiLabels = map[string]string{"env": "test"}
	c.HTTPURL = srv.URL + "/ingest"
	l := logSome(t, c)
	for _, s := range l.Stats() {
		if s.Sent != 2 || s.Dropped != 0 || s.Failed != 0 {
			t.Errorf("%+v", s)
		}
	}
	got := col.all()
	for _, want := range []string{
		// OTLP
		`/v1/logs {"resourceLogs"`, `"key":"service.name","value":{"stringValue":"zerechnung"}`,
		`"key":"deployment.environment"`, `"severityNumber":17`, `"traceId":"4bf92f3577b34da6a3ce929d0e0e4736"`,
		`"key":"http.status","value":{"intValue":"201"}`, `"key":"http.auth.method"`,
		// Loki
		`/loki/api/v1/push {"streams"`, `"level":"error"`, `"env":"test"`,
		// HTTP
		`/ingest {`, `\"msg\":\"request\"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	// The NDJSON body has one record per line.
	for _, b := range col.bodies {
		if p, body, _ := strings.Cut(b, " "); p == "/ingest" {
			lines := strings.Split(strings.TrimSpace(body), "\n")
			if len(lines) != 2 {
				t.Errorf("%d NDJSON lines", len(lines))
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
				t.Fatal(err)
			}
			if m["request_id"] != "r-1" || m["http.status"] != float64(201) || m["service"] != "zerechnung" || m["deployment.environment"] != "test" {
				t.Errorf("record %v", m)
			}
		}
	}
}

func TestHTTPRefused(t *testing.T) {
	col := &collector{status: http.StatusUnauthorized}
	srv := httptest.NewServer(col)
	defer srv.Close()
	c := baseConfig("http")
	c.HTTPURL = srv.URL
	l := logSome(t, c)
	if s := l.Stats()[0]; s.Failed != 2 || s.Sent != 0 {
		t.Errorf("%+v", s)
	}
	if n := len(col.bodies); n != 1 {
		t.Errorf("a refused batch was sent %d times", n)
	}
}

func TestDropWhenFull(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	c := baseConfig("http")
	c.HTTPURL = srv.URL
	c.Buffer, c.BatchSize = 5, 1
	l, err := New(c, info, http.DefaultClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for range 100 {
		l.Logger.Info("x")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("logging blocked for %s", d)
	}
	if s := l.Stats()[0]; s.Dropped < 90 {
		t.Errorf("%+v", s)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	l.Close(ctx)
}

func TestSyslogAndGELF(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	var tcpData bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := tcp.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		io.Copy(&tcpData, bufio.NewReader(c))
	}()

	c := baseConfig("syslog", "gelf")
	c.SyslogAddress = "tcp://" + tcp.Addr().String()
	c.GELFAddress = "udp://" + udp.LocalAddr().String()
	logSome(t, c)
	wg.Wait()

	// Syslog over TCP: octet counting framing, RFC 5424 header.
	s := tcpData.String()
	if !strings.Contains(s, "<134>1 ") || !strings.Contains(s, "<131>1 ") || !strings.Contains(s, " node-1 zerechnung ") {
		t.Errorf("syslog: %q", s)
	}
	for i := 0; i < 2; i++ {
		n, rest, ok := strings.Cut(s, " ")
		size, err := strconv.Atoi(n)
		if !ok || err != nil || size > len(rest) || !strings.HasPrefix(rest, "<13") || !strings.HasSuffix(rest[:size], "}") {
			t.Fatalf("syslog framing of message %d: %q", i, s)
		}
		s = rest[size:]
	}
	if s != "" {
		t.Errorf("syslog: trailing %q", s)
	}

	// GELF over UDP: one datagram per record.
	buf := make([]byte, 65536)
	_ = udp.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := range 2 {
		n, _, err := udp.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(buf[:n], &m); err != nil {
			t.Fatal(err)
		}
		if m["version"] != "1.1" || m["host"] != "node-1" || m["_service"] != "zerechnung" {
			t.Errorf("GELF %d: %v", i, m)
		}
		if i == 0 && (m["short_message"] != "request" || m["_http.status"] != float64(201) || m["level"] != float64(6)) {
			t.Errorf("GELF record: %v", m)
		}
	}
}

func TestGELFChunks(t *testing.T) {
	m := bytes.Repeat([]byte("a"), 3*gelfChunk)
	chunks, err := gelfChunks(m)
	if err != nil {
		t.Fatal(err)
	}
	var joined []byte
	for i, c := range chunks {
		if c[0] != 0x1e || c[1] != 0x0f || int(c[10]) != i || int(c[11]) != len(chunks) || len(c) > gelfChunk {
			t.Errorf("chunk %d header %x", i, c[:12])
		}
		joined = append(joined, c[12:]...)
	}
	if !bytes.Equal(joined, m) {
		t.Error("the chunks do not rebuild the message")
	}
	if _, err := gelfChunks(bytes.Repeat([]byte("a"), 129*gelfChunk)); err == nil {
		t.Error("no error for more than 128 chunks")
	}
}

func TestFileRotation(t *testing.T) {
	dir := t.TempDir()
	c := baseConfig("file")
	c.File = filepath.Join(dir, "logs", "server.log")
	c.FileMaxBytes = 300
	l, err := New(c, info, http.DefaultClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		l.Logger.Info("a line long enough to rotate the file after a few records")
	}
	l.Close(context.Background())
	for _, name := range []string{"server.log", "server.log.1", "server.log.2"} {
		if _, err := os.Stat(filepath.Join(dir, "logs", name)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "server.log.3")); err == nil {
		t.Error("more backups than configured")
	}
}

func TestLevelAndNone(t *testing.T) {
	col := &collector{}
	srv := httptest.NewServer(col)
	defer srv.Close()
	c := baseConfig("http", "none")
	c.HTTPURL = srv.URL
	c.Level = slog.LevelWarn
	l := logSome(t, c)
	if s := l.Stats()[0]; s.Sent != 1 {
		t.Errorf("%+v", s)
	}
	l2, err := New(baseConfig("none"), info, http.DefaultClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l2.Logger.Enabled(context.Background(), slog.LevelError) {
		t.Error("none logs")
	}
}

func TestBadAddresses(t *testing.T) {
	for _, tc := range []struct{ output, addr string }{
		{"syslog", "localhost:514"}, {"syslog", "http://host:514"}, {"gelf", "tls://host:12201"}, {"loki", "ftp://loki"},
	} {
		c := baseConfig(tc.output)
		c.SyslogAddress, c.GELFAddress, c.LokiURL = tc.addr, tc.addr, tc.addr
		if _, err := New(c, info, http.DefaultClient, nil); err == nil {
			t.Errorf("%s %s: no error", tc.output, tc.addr)
		}
	}
}
