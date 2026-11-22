package logging

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// recordJSON encodes an entry as a flat JSON object, the format of the
// syslog, Loki and HTTP outputs: time, level, msg, the service fields,
// then the attributes.
func recordJSON(e Entry, info Info) []byte {
	m := make(map[string]any, len(e.Attrs)+5)
	for k, v := range info.Resource {
		m[k] = v
	}
	m["service"] = info.Service
	if info.Hostname != "" {
		m["host"] = info.Hostname
	}
	if info.Version != "" {
		m["version"] = info.Version
	}
	for _, a := range e.Attrs {
		m[a.Key] = jsonValue(a.Value)
	}
	m["time"] = e.Time.UTC().Format(time.RFC3339Nano)
	m["level"] = e.Level.String()
	m["msg"] = e.Message
	b, err := json.Marshal(m)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"time": m["time"], "level": m["level"], "msg": e.Message, "log_error": err.Error()})
	}
	return b
}

func jsonValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		if f := v.Float64(); !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f
		}
		return v.String()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	}
	a := v.Any()
	if err, ok := a.(error); ok {
		return err.Error()
	}
	if _, err := json.Marshal(a); err != nil {
		return fmt.Sprint(a)
	}
	return a
}

// --- File ---

// rotating is a log file renamed to name.1, name.2… when it exceeds its
// maximum size.
type rotating struct {
	mu      sync.Mutex
	path    string
	max     int64
	backups int
	f       *os.File
	size    int64
}

func openRotating(path string, max int64, backups int) (*rotating, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &rotating{path: path, max: max, backups: backups}
	return r, r.open()
}

func (r *rotating) open() error {
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > r.max && r.size > 0 {
		if err := r.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "zerechnung-server: log file rotation: %v\n", err)
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotating) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	if r.backups > 0 {
		for i := r.backups - 1; i >= 1; i-- {
			_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
		}
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return err
		}
	} else if err := os.Truncate(r.path, 0); err != nil {
		return err
	}
	return r.open()
}

func (r *rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

// --- Stream connections (syslog, GELF) ---

// conn is a connection to a syslog or GELF server, dialed again after a
// failure.
type conn struct {
	network, addr string
	tlsConf       *tls.Config
	c             net.Conn
}

// parseAddr reads udp://host:port, tcp://host:port or tls://host:port.
func parseAddr(s string, schemes ...string) (scheme, hostport string, err error) {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("%q is not scheme://host:port", s)
	}
	for _, sc := range schemes {
		if u.Scheme == sc {
			if _, _, err := net.SplitHostPort(u.Host); err != nil {
				return "", "", fmt.Errorf("%q: %w", s, err)
			}
			return sc, u.Host, nil
		}
	}
	return "", "", fmt.Errorf("%q: the scheme must be one of %s", s, strings.Join(schemes, ", "))
}

func (c *conn) get(ctx context.Context) (net.Conn, error) {
	if c.c != nil {
		return c.c, nil
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	var err error
	switch c.network {
	case "tls":
		td := &tls.Dialer{NetDialer: d, Config: c.tlsConf}
		c.c, err = td.DialContext(ctx, "tcp", c.addr)
	default:
		c.c, err = d.DialContext(ctx, c.network, c.addr)
	}
	return c.c, err
}

// write sends the messages, dialing again once if the connection broke.
func (c *conn) write(ctx context.Context, msgs [][]byte) error {
	for attempt := 0; ; attempt++ {
		nc, err := c.get(ctx)
		if err != nil {
			return err
		}
		if dl, ok := ctx.Deadline(); ok {
			_ = nc.SetWriteDeadline(dl)
		}
		for len(msgs) > 0 {
			if _, err = nc.Write(msgs[0]); err != nil {
				break
			}
			msgs = msgs[1:]
		}
		if err == nil {
			return nil
		}
		nc.Close()
		c.c = nil
		if attempt > 0 || c.network == "udp" {
			return err
		}
	}
}

func (c *conn) Close() error {
	if c.c == nil {
		return nil
	}
	return c.c.Close()
}

// --- Syslog (RFC 5424, RFC 6587 octet counting on TCP and TLS) ---

var facilities = map[string]int{
	"kern": 0, "user": 1, "mail": 2, "daemon": 3, "auth": 4, "syslog": 5, "lpr": 6, "news": 7,
	"uucp": 8, "cron": 9, "authpriv": 10, "ftp": 11,
	"local0": 16, "local1": 17, "local2": 18, "local3": 19, "local4": 20, "local5": 21, "local6": 22, "local7": 23,
}

type syslogExporter struct {
	conn
	facility int
	info     Info
	pid      string
}

func newSyslog(addr, facility string, info Info, tlsConf *tls.Config) (*syslogExporter, error) {
	scheme, hostport, err := parseAddr(addr, "udp", "tcp", "tls")
	if err != nil {
		return nil, err
	}
	return &syslogExporter{
		conn:     conn{network: scheme, addr: hostport, tlsConf: tlsConf},
		facility: facilities[facility],
		info:     info,
		pid:      strconv.Itoa(os.Getpid()),
	}, nil
}

// severity maps a slog level to a syslog severity.
func severity(l slog.Level) int {
	switch {
	case l >= slog.LevelError:
		return 3
	case l >= slog.LevelWarn:
		return 4
	case l >= slog.LevelInfo:
		return 6
	}
	return 7
}

func (s *syslogExporter) Export(ctx context.Context, entries []Entry) error {
	msgs := make([][]byte, len(entries))
	for i, e := range entries {
		host := s.info.Hostname
		if host == "" {
			host = "-"
		}
		m := fmt.Appendf(nil, "<%d>1 %s %s %s %s - - %s",
			s.facility*8+severity(e.Level), e.Time.UTC().Format(time.RFC3339Nano),
			sdName(host), sdName(s.info.Service), s.pid, recordJSON(e, s.info))
		if s.network != "udp" {
			m = append(strconv.AppendInt(nil, int64(len(m)), 10), append([]byte{' '}, m...)...)
		}
		msgs[i] = m
	}
	return s.write(ctx, msgs)
}

// sdName keeps the printable ASCII of a header field, as RFC 5424 wants.
func sdName(s string) string {
	b := []byte(s)
	out := b[:0]
	for _, c := range b {
		if c > 32 && c < 127 {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return string(out)
}

// --- GELF (Graylog) ---

// gelfChunk is the largest datagram of the UDP transport; longer messages
// are chunked.
const gelfChunk = 8192

type gelfExporter struct {
	conn
	info Info
}

func newGELF(addr string, info Info) (*gelfExporter, error) {
	scheme, hostport, err := parseAddr(addr, "udp", "tcp")
	if err != nil {
		return nil, err
	}
	return &gelfExporter{conn: conn{network: scheme, addr: hostport}, info: info}, nil
}

var gelfField = regexp.MustCompile(`[^\w.\-]`)

func gelfMessage(e Entry, info Info) []byte {
	m := map[string]any{
		"version":       "1.1",
		"host":          info.Hostname,
		"short_message": e.Message,
		"timestamp":     float64(e.Time.UnixMicro()) / 1e6,
		"level":         severity(e.Level),
		"_service":      info.Service,
	}
	if m["host"] == "" {
		m["host"] = info.Service
	}
	if info.Version != "" {
		m["_version"] = info.Version
	}
	for k, v := range info.Resource {
		m["_"+gelfField.ReplaceAllString(k, "_")] = v
	}
	for _, a := range e.Attrs {
		k := "_" + gelfField.ReplaceAllString(a.Key, "_")
		if k == "_id" {
			k = "_id_"
		}
		m[k] = jsonValue(a.Value)
	}
	b, _ := json.Marshal(m)
	return b
}

func (g *gelfExporter) Export(ctx context.Context, entries []Entry) error {
	var msgs [][]byte
	for _, e := range entries {
		m := gelfMessage(e, g.info)
		switch {
		case g.network == "tcp":
			msgs = append(msgs, append(m, 0))
		case len(m) <= gelfChunk:
			msgs = append(msgs, m)
		default:
			chunks, err := gelfChunks(m)
			if err != nil {
				return err
			}
			msgs = append(msgs, chunks...)
		}
	}
	return g.write(ctx, msgs)
}

// gelfChunks splits a message into GELF chunks: magic bytes, message
// identifier, sequence number and count, then the data.
func gelfChunks(m []byte) ([][]byte, error) {
	const header = 12
	size := gelfChunk - header
	n := (len(m) + size - 1) / size
	if n > 128 {
		return nil, fmt.Errorf("GELF message of %d bytes: more than 128 chunks", len(m))
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	out := make([][]byte, 0, n)
	for i := range n {
		c := append([]byte{0x1e, 0x0f}, id...)
		c = append(c, byte(i), byte(n))
		out = append(out, append(c, m[i*size:min(len(m), (i+1)*size)]...))
	}
	return out, nil
}

// --- HTTP outputs ---

// post sends a body; a 4xx response other than 408 and 429 is permanent.
func post(ctx context.Context, client *http.Client, url, contentType string, headers map[string]string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return permanentError{err}
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "zerechnung-server")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 300 {
		return nil
	}
	err = fmt.Errorf("HTTP status %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
		return permanentError{err}
	}
	return err
}

// httpExporter posts the records as NDJSON or as a JSON array: the input
// of Vector, Fluent Bit, Logstash, Datadog and many collectors.
type httpExporter struct {
	url     string
	headers map[string]string
	array   bool
	info    Info
	client  *http.Client
}

func (h *httpExporter) Export(ctx context.Context, entries []Entry) error {
	var b bytes.Buffer
	if h.array {
		b.WriteByte('[')
	}
	for i, e := range entries {
		if h.array && i > 0 {
			b.WriteByte(',')
		}
		b.Write(recordJSON(e, h.info))
		if !h.array {
			b.WriteByte('\n')
		}
	}
	ct := "application/x-ndjson"
	if h.array {
		b.WriteByte(']')
		ct = "application/json"
	}
	return post(ctx, h.client, h.url, ct, h.headers, b.Bytes())
}

// lokiExporter pushes the records to Grafana Loki, one stream per level.
type lokiExporter struct {
	url     string
	labels  map[string]string
	headers map[string]string
	info    Info
	client  *http.Client
}

func newLoki(base string, labels map[string]string, tenant string, info Info, client *http.Client) (*lokiExporter, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%q is not an http(s) URL", base)
	}
	if !strings.HasSuffix(u.Path, "/loki/api/v1/push") {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/loki/api/v1/push"
	}
	l := &lokiExporter{url: u.String(), labels: map[string]string{"service": info.Service}, headers: map[string]string{}, info: info, client: client}
	maps.Copy(l.labels, labels)
	if tenant != "" {
		l.headers["X-Scope-OrgID"] = tenant
	}
	return l, nil
}

func (l *lokiExporter) Export(ctx context.Context, entries []Entry) error {
	type stream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	byLevel := map[string]*stream{}
	var order []string
	for _, e := range entries {
		lv := strings.ToLower(e.Level.String())
		s := byLevel[lv]
		if s == nil {
			labels := map[string]string{"level": lv}
			maps.Copy(labels, l.labels)
			s = &stream{Stream: labels}
			byLevel[lv] = s
			order = append(order, lv)
		}
		s.Values = append(s.Values, [2]string{strconv.FormatInt(e.Time.UnixNano(), 10), string(recordJSON(e, l.info))})
	}
	sort.Strings(order)
	body := struct {
		Streams []*stream `json:"streams"`
	}{}
	for _, lv := range order {
		body.Streams = append(body.Streams, byLevel[lv])
	}
	b, err := json.Marshal(body)
	if err != nil {
		return permanentError{err}
	}
	return post(ctx, l.client, l.url, "application/json", l.headers, b)
}

// otlpExporter sends the records to an OpenTelemetry collector, or any
// OTLP/HTTP endpoint, in the JSON encoding of OTLP.
type otlpExporter struct {
	url     string
	headers map[string]string
	info    Info
	client  *http.Client
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

type otlpKV struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

func otlpString(s string) otlpValue { return otlpValue{StringValue: &s} }

func otlpAttr(key string, v slog.Value) otlpKV {
	var ov otlpValue
	switch v.Kind() {
	case slog.KindInt64:
		s := strconv.FormatInt(v.Int64(), 10)
		ov.IntValue = &s
	case slog.KindUint64:
		s := strconv.FormatUint(v.Uint64(), 10)
		ov.IntValue = &s
	case slog.KindFloat64:
		if f := v.Float64(); !math.IsNaN(f) && !math.IsInf(f, 0) {
			ov.DoubleValue = &f
		} else {
			ov = otlpString(v.String())
		}
	case slog.KindBool:
		b := v.Bool()
		ov.BoolValue = &b
	default:
		ov = otlpString(fmt.Sprint(jsonValue(v)))
	}
	return otlpKV{Key: key, Value: ov}
}

// severityNumber maps a slog level to an OpenTelemetry severity number.
func severityNumber(l slog.Level) int {
	switch {
	case l >= slog.LevelError:
		return 17
	case l >= slog.LevelWarn:
		return 13
	case l >= slog.LevelInfo:
		return 9
	}
	return 5
}

func (o *otlpExporter) Export(ctx context.Context, entries []Entry) error {
	type record struct {
		TimeUnixNano         string    `json:"timeUnixNano"`
		ObservedTimeUnixNano string    `json:"observedTimeUnixNano"`
		SeverityNumber       int       `json:"severityNumber"`
		SeverityText         string    `json:"severityText"`
		Body                 otlpValue `json:"body"`
		Attributes           []otlpKV  `json:"attributes,omitempty"`
		TraceID              string    `json:"traceId,omitempty"`
		SpanID               string    `json:"spanId,omitempty"`
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	records := make([]record, 0, len(entries))
	for _, e := range entries {
		r := record{
			TimeUnixNano:         strconv.FormatInt(e.Time.UnixNano(), 10),
			ObservedTimeUnixNano: now,
			SeverityNumber:       severityNumber(e.Level),
			SeverityText:         e.Level.String(),
			Body:                 otlpString(e.Message),
		}
		for _, a := range e.Attrs {
			switch a.Key {
			case "trace_id":
				r.TraceID = a.Value.String()
			case "span_id":
				r.SpanID = a.Value.String()
			default:
				r.Attributes = append(r.Attributes, otlpAttr(a.Key, a.Value))
			}
		}
		records = append(records, r)
	}
	res := []otlpKV{{Key: "service.name", Value: otlpString(o.info.Service)}}
	if o.info.Version != "" {
		res = append(res, otlpKV{Key: "service.version", Value: otlpString(o.info.Version)})
	}
	if o.info.Hostname != "" {
		res = append(res, otlpKV{Key: "host.name", Value: otlpString(o.info.Hostname)})
	}
	keys := make([]string, 0, len(o.info.Resource))
	for k := range o.info.Resource {
		if k != "service.name" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		res = append(res, otlpKV{Key: k, Value: otlpString(o.info.Resource[k])})
	}
	body := map[string]any{"resourceLogs": []any{map[string]any{
		"resource": map[string]any{"attributes": res},
		"scopeLogs": []any{map[string]any{
			"scope":      map[string]any{"name": "zerechnung-server", "version": o.info.Version},
			"logRecords": records,
		}},
	}}}
	b, err := json.Marshal(body)
	if err != nil {
		return permanentError{err}
	}
	return post(ctx, o.client, o.url, "application/json", o.headers, b)
}
