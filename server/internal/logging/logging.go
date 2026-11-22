// Package logging builds the logger of the server: a log/slog logger that
// writes to the standard output, a file, or sends the records outside the
// container to syslog, Graylog (GELF), an OpenTelemetry collector (OTLP),
// Grafana Loki or any HTTP endpoint, several at once.
//
// The remote outputs never slow down a request: each one has a buffer that
// a goroutine empties in batches. When an output is down or too slow, the
// records beyond its buffer are dropped and counted, and the failures are
// reported on the standard error, at most every 30 seconds per output.
package logging

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ritoon/zerechnung/server/internal/config"
)

// Info describes the service in the records sent to the remote outputs.
type Info struct {
	Service  string
	Version  string
	Hostname string
	// Resource holds extra attributes, such as deployment.environment.
	Resource map[string]string
}

// Stats are the counters of a remote output.
type Stats struct {
	Output  string
	Sent    uint64 // records delivered
	Dropped uint64 // records dropped because the buffer was full
	Failed  uint64 // records lost because the output refused them or was unreachable
}

// Logging holds the logger and the outputs to close.
type Logging struct {
	Logger   *slog.Logger
	batchers []*batcher
	closers  []io.Closer
}

// New builds the logger. client sends the records of the HTTP outputs and
// tlsConf configures the TLS connections of syslog.
func New(c *config.Log, info Info, client *http.Client, tlsConf *tls.Config) (*Logging, error) {
	l := &Logging{}
	opts := &slog.HandlerOptions{Level: c.Level}
	local := func(w io.Writer) slog.Handler {
		if c.Format == "text" {
			return slog.NewTextHandler(w, opts)
		}
		return slog.NewJSONHandler(w, opts)
	}
	var handlers []slog.Handler
	remote := func(name string, e exporter) {
		b := newBatcher(name, e, c)
		l.batchers = append(l.batchers, b)
		handlers = append(handlers, &entryHandler{level: c.Level, out: b})
	}
	for _, o := range slices.Compact(slices.Clone(c.Outputs)) {
		switch o {
		case "none":
		case "stdout":
			handlers = append(handlers, local(os.Stdout))
		case "stderr":
			handlers = append(handlers, local(os.Stderr))
		case "file":
			f, err := openRotating(c.File, c.FileMaxBytes, c.FileMaxBackups)
			if err != nil {
				return nil, fmt.Errorf("log file: %w", err)
			}
			l.closers = append(l.closers, f)
			handlers = append(handlers, local(f))
		case "syslog":
			e, err := newSyslog(c.SyslogAddress, c.SyslogFacility, info, tlsConf)
			if err != nil {
				return nil, fmt.Errorf("syslog output: %w", err)
			}
			remote("syslog", e)
		case "gelf":
			e, err := newGELF(c.GELFAddress, info)
			if err != nil {
				return nil, fmt.Errorf("gelf output: %w", err)
			}
			remote("gelf", e)
		case "otlp":
			remote("otlp", &otlpExporter{url: c.OTLPEndpoint, headers: c.OTLPHeaders, info: info, client: client})
		case "loki":
			e, err := newLoki(c.LokiURL, c.LokiLabels, c.LokiTenant, info, client)
			if err != nil {
				return nil, fmt.Errorf("loki output: %w", err)
			}
			remote("loki", e)
		case "http":
			remote("http", &httpExporter{url: c.HTTPURL, headers: c.HTTPHeaders, array: c.HTTPFormat == "json", info: info, client: client})
		default:
			return nil, fmt.Errorf("unknown log output %q", o)
		}
	}
	for _, b := range l.batchers {
		go b.run()
	}
	var h slog.Handler
	switch len(handlers) {
	case 0:
		h = slog.DiscardHandler
	case 1:
		h = handlers[0]
	default:
		h = &fanout{handlers: handlers}
	}
	l.Logger = slog.New(h)
	return l, nil
}

// Stats returns the counters of the remote outputs.
func (l *Logging) Stats() []Stats {
	out := make([]Stats, 0, len(l.batchers))
	for _, b := range l.batchers {
		out = append(out, Stats{Output: b.name, Sent: b.sent.Load(), Dropped: b.dropped.Load(), Failed: b.failed.Load()})
	}
	return out
}

// Close sends the buffered records, until ctx is done, and closes the
// outputs.
func (l *Logging) Close(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, b := range l.batchers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.close(ctx)
		}()
	}
	wg.Wait()
	var errs []error
	for _, b := range l.batchers {
		if c, ok := b.exp.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}
	for _, c := range l.closers {
		errs = append(errs, c.Close())
	}
	return errors.Join(errs...)
}

// fanout sends each record to several handlers.
type fanout struct{ handlers []slog.Handler }

func (f *fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := &fanout{handlers: make([]slog.Handler, len(f.handlers))}
	for i, h := range f.handlers {
		out.handlers[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f *fanout) WithGroup(name string) slog.Handler {
	out := &fanout{handlers: make([]slog.Handler, len(f.handlers))}
	for i, h := range f.handlers {
		out.handlers[i] = h.WithGroup(name)
	}
	return out
}

// Entry is a record ready to export: its attributes are resolved and
// flattened, a group name prefixing the keys of its members ("req.id").
type Entry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []Attr
}

// Attr is an attribute of an entry.
type Attr struct {
	Key   string
	Value slog.Value
}

// entryHandler turns the records into entries for a remote output.
type entryHandler struct {
	level  slog.Leveler
	out    *batcher
	attrs  []Attr
	prefix string
}

func (h *entryHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *entryHandler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: slices.Clone(h.attrs)}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	r.Attrs(func(a slog.Attr) bool {
		e.Attrs = appendAttr(e.Attrs, h.prefix, a)
		return true
	})
	h.out.enqueue(e)
	return nil
}

func (h *entryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = slices.Clone(h.attrs)
	for _, a := range attrs {
		c.attrs = appendAttr(c.attrs, h.prefix, a)
	}
	return &c
}

func (h *entryHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = h.prefix + name + "."
	return &c
}

// appendAttr flattens an attribute, following the rules of slog: empty
// attributes are dropped, the members of a group without a key are
// inlined.
func appendAttr(out []Attr, prefix string, a slog.Attr) []Attr {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return out
	}
	if a.Value.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, m := range a.Value.Group() {
			out = appendAttr(out, p, m)
		}
		return out
	}
	return append(out, Attr{Key: prefix + a.Key, Value: a.Value})
}

// exporter sends a batch of entries to an output.
type exporter interface {
	Export(ctx context.Context, entries []Entry) error
}

// batcher buffers the entries of an output and sends them in batches.
type batcher struct {
	name     string
	exp      exporter
	ch       chan Entry
	size     int
	interval time.Duration

	stopping atomic.Bool
	// quit is closed by close, after stopCtx is set: the context of the
	// last sends.
	quit    chan struct{}
	stopCtx context.Context
	done    chan struct{}
	// base is the context of the regular sends, canceled when the
	// shutdown runs out of time.
	base   context.Context
	cancel context.CancelFunc

	sent, dropped, failed atomic.Uint64
	lastReport            atomic.Int64
}

func newBatcher(name string, e exporter, c *config.Log) *batcher {
	base, cancel := context.WithCancel(context.Background())
	return &batcher{
		base:     base,
		cancel:   cancel,
		name:     name,
		exp:      e,
		ch:       make(chan Entry, c.Buffer),
		size:     c.BatchSize,
		interval: c.FlushInterval,
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// enqueue never blocks: an entry that does not fit is dropped.
func (b *batcher) enqueue(e Entry) {
	if b.stopping.Load() {
		b.dropped.Add(1)
		return
	}
	select {
	case b.ch <- e:
	default:
		b.dropped.Add(1)
	}
}

func (b *batcher) run() {
	defer close(b.done)
	t := time.NewTicker(b.interval)
	defer t.Stop()
	batch := make([]Entry, 0, b.size)
	flush := func(ctx context.Context) {
		if len(batch) > 0 {
			b.send(ctx, batch)
			batch = batch[:0]
		}
	}
	for {
		select {
		case e := <-b.ch:
			batch = append(batch, e)
			if len(batch) >= b.size {
				flush(b.base)
			}
		case <-t.C:
			flush(b.base)
		case <-b.quit:
			// Drain what is buffered, then stop.
			ctx := b.stopCtx
			for {
				select {
				case e := <-b.ch:
					batch = append(batch, e)
					if len(batch) >= b.size {
						flush(ctx)
					}
					continue
				default:
				}
				break
			}
			flush(ctx)
			return
		}
	}
}

// send exports a batch, retrying twice with a backoff unless ctx says to
// stop.
func (b *batcher) send(ctx context.Context, batch []Entry) {
	var err error
	for attempt, wait := 0, 500*time.Millisecond; attempt < 3; attempt, wait = attempt+1, 2*wait {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = b.exp.Export(c, batch)
		cancel()
		if err == nil {
			b.sent.Add(uint64(len(batch)))
			return
		}
		var perm permanentError
		if errors.As(err, &perm) || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	b.failed.Add(uint64(len(batch)))
	b.report(err)
}

// report writes a failure on the standard error, at most every 30 seconds:
// logging it would loop.
func (b *batcher) report(err error) {
	now := time.Now().Unix()
	last := b.lastReport.Load()
	if now-last < 30 || !b.lastReport.CompareAndSwap(last, now) {
		return
	}
	fmt.Fprintf(os.Stderr, "zerechnung-server: log output %s: %v (sent %d, dropped %d, failed %d)\n",
		b.name, err, b.sent.Load(), b.dropped.Load(), b.failed.Load())
}

// close flushes the buffer and stops the goroutine; when ctx is done
// first, the send in progress is canceled and the rest is dropped.
func (b *batcher) close(ctx context.Context) {
	if !b.stopping.CompareAndSwap(false, true) {
		return
	}
	defer b.cancel()
	b.stopCtx = ctx
	close(b.quit)
	select {
	case <-b.done:
	case <-ctx.Done():
		b.cancel()
		<-b.done
	}
}

// permanentError marks a failure that a retry would not fix, such as a
// refused request.
type permanentError struct{ error }

func (e permanentError) Unwrap() error { return e.error }
