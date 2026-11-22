// Package metrics keeps the counters of the server and writes them in the
// Prometheus text format, without a client library.
package metrics

import (
	"fmt"
	"io"
	"maps"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// buckets are the upper bounds, in seconds, of the duration histograms.
var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// Registry holds the metrics.
type Registry struct {
	mu        sync.Mutex
	start     time.Time
	info      map[string]string
	counters  map[string]map[string]float64 // name → labels → value
	help      map[string]string
	hists     map[string]map[string]*histogram
	inFlight  int64
	collector []func(w io.Writer)
}

type histogram struct {
	counts []uint64
	sum    float64
	count  uint64
}

// New returns a registry; info labels the build_info gauge.
func New(info map[string]string) *Registry {
	return &Registry{
		start:    time.Now(),
		info:     info,
		counters: map[string]map[string]float64{},
		help:     map[string]string{},
		hists:    map[string]map[string]*histogram{},
	}
}

// Labels formats label pairs: Labels("method", "GET", "status", "200").
func Labels(kv ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(kv[i])
		b.WriteString(`="`)
		b.WriteString(escape(kv[i+1]))
		b.WriteByte('"')
	}
	return b.String()
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// Add adds v to a counter.
func (r *Registry) Add(name, help, labels string, v float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.counters[name]
	if m == nil {
		m = map[string]float64{}
		r.counters[name] = m
		r.help[name] = help
	}
	m[labels] += v
}

// Observe records a duration in a histogram.
func (r *Registry) Observe(name, help, labels string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.hists[name]
	if m == nil {
		m = map[string]*histogram{}
		r.hists[name] = m
		r.help[name] = help
	}
	h := m[labels]
	if h == nil {
		h = &histogram{counts: make([]uint64, len(buckets))}
		m[labels] = h
	}
	s := d.Seconds()
	for i, b := range buckets {
		if s <= b {
			h.counts[i]++
		}
	}
	h.sum += s
	h.count++
}

// InFlight changes the number of requests in progress.
func (r *Registry) InFlight(delta int64) {
	r.mu.Lock()
	r.inFlight += delta
	r.mu.Unlock()
}

// Collect registers a function that writes more metrics, read at each
// scrape.
func (r *Registry) Collect(f func(w io.Writer)) {
	r.mu.Lock()
	r.collector = append(r.collector, f)
	r.mu.Unlock()
}

// Write writes the metrics in the Prometheus text format.
func (r *Registry) Write(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(w, "# HELP zerechnung_build_info Build information.\n# TYPE zerechnung_build_info gauge\nzerechnung_build_info{%s} 1\n",
		Labels(flatten(r.info)...))
	fmt.Fprintf(w, "# HELP process_start_time_seconds Start time of the process since the Unix epoch.\n# TYPE process_start_time_seconds gauge\nprocess_start_time_seconds %d\n", r.start.Unix())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines.\n# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "# HELP go_memstats_heap_alloc_bytes Bytes of allocated heap objects.\n# TYPE go_memstats_heap_alloc_bytes gauge\ngo_memstats_heap_alloc_bytes %d\n", ms.HeapAlloc)
	fmt.Fprintf(w, "# HELP zerechnung_http_requests_in_flight Requests in progress.\n# TYPE zerechnung_http_requests_in_flight gauge\nzerechnung_http_requests_in_flight %d\n", r.inFlight)
	for _, name := range slices.Sorted(maps.Keys(r.counters)) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", name, r.help[name], name)
		m := r.counters[name]
		for _, l := range slices.Sorted(maps.Keys(m)) {
			fmt.Fprintf(w, "%s%s %s\n", name, braces(l), num(m[l]))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(r.hists)) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", name, r.help[name], name)
		m := r.hists[name]
		for _, l := range slices.Sorted(maps.Keys(m)) {
			h := m[l]
			sep := ""
			if l != "" {
				sep = ","
			}
			for i, b := range buckets {
				fmt.Fprintf(w, "%s_bucket{%s%sle=\"%s\"} %d\n", name, l, sep, num(b), h.counts[i])
			}
			fmt.Fprintf(w, "%s_bucket{%s%sle=\"+Inf\"} %d\n", name, l, sep, h.count)
			fmt.Fprintf(w, "%s_sum%s %s\n%s_count%s %d\n", name, braces(l), num(h.sum), name, braces(l), h.count)
		}
	}
	for _, f := range r.collector {
		f(w)
	}
}

func braces(l string) string {
	if l == "" {
		return ""
	}
	return "{" + l + "}"
}

func num(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func flatten(m map[string]string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out = append(out, k, m[k])
	}
	return out
}
