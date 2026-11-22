package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"testing"
)

// TestConcurrentRequests sends requests to every route from many goroutines,
// with one signing key shared by all of them. Each response must have the
// status and content type of a sequential run, and the same body when that
// body does not change from one run to the next (it does for signed PDFs and
// for the metrics).
func TestConcurrentRequests(t *testing.T) {
	e := newEnv(t, testSigner(t))
	hybrid := testdata(t, "EN16931_Einfach.pdf")
	_, signed := e.do("POST", "/v1/sign", "application/pdf", hybrid)

	type request struct {
		method, path, contentType, accept string
		body                              []byte
	}
	jsonBody := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	inv := invoiceJSON(t, frenchInvoice())
	requests := []request{
		{"POST", "/v1/invoices", "application/json", "", jsonBody(map[string]any{
			"invoice": inv, "validation": map[string]any{"rules": []string{"fr"}}, "language": "fr",
		})},
		{"POST", "/v1/invoices", "application/json", "application/json", jsonBody(map[string]any{"invoice": inv, "sign": false, "language": "de"})},
		{"POST", "/v1/invoices", "application/json", "", jsonBody(map[string]any{"xml": testdata(t, "EN16931_Einfach.cii.xml"), "sign": false})},
		{"POST", "/v1/validate?rules=de", "application/pdf", "", hybrid},
		{"POST", "/v1/validate", "application/xml", "", testdata(t, "XRECHNUNG_Einfach.ubl.xml")},
		{"POST", "/v1/sign?reason=Archivage", "application/pdf", "", hybrid},
		{"POST", "/v1/verify", "application/pdf", "", signed},
		{"POST", "/v1/extract", "application/pdf", "", hybrid},
		{"POST", "/v1/extract?format=json", "application/xml", "", testdata(t, "XRECHNUNG_Einfach.ubl.xml")},
		{"POST", "/v1/pdfa", "application/pdf", "", blankPDF()},
		{"POST", "/v1/pdfa", "application/pdf", "", testdata(t, "host.pdf")},
		{"GET", "/v1/info", "", "", nil},
		{"GET", "/metrics", "", "", nil},
	}
	for _, format := range []string{"cii", "ubl", "fatturapa", "facturae"} {
		requests = append(requests, request{"POST", "/v1/xml", "application/json", "", jsonBody(map[string]any{
			"invoice": inv, "format": format, "validation": map[string]any{"skip": format == "fatturapa" || format == "facturae"},
		})})
	}

	// send must not call t.Fatal: it runs on other goroutines than the test.
	send := func(r request) (string, []byte, error) {
		req, err := http.NewRequest(r.method, e.srv.URL+r.path, bytes.NewReader(r.body))
		if err != nil {
			return "", nil, err
		}
		req.SetBasicAuth("billing", "s3cret-password")
		if r.contentType != "" {
			req.Header.Set("Content-Type", r.contentType)
		}
		if r.accept != "" {
			req.Header.Set("Accept", r.accept)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return fmt.Sprintf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type")), body, err
	}

	wantHead := make([]string, len(requests))
	wantBody := make([][]byte, len(requests))
	stable := make([]bool, len(requests))
	for i, r := range requests {
		head, body, err := send(r)
		if err != nil {
			t.Fatal(err)
		}
		_, again, err := send(r)
		if err != nil {
			t.Fatal(err)
		}
		if head[0] != '2' && r.path != "/v1/pdfa" {
			t.Fatalf("%s %s: %s: %.300s", r.method, r.path, head, body)
		}
		wantHead[i], wantBody[i], stable[i] = head, body, bytes.Equal(body, again)
	}

	workers := 2 * runtime.GOMAXPROCS(0)
	const rounds = 2
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		failures = map[string]int{}
		start    = make(chan struct{})
	)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for r := range rounds {
				for k := range requests {
					i := (k + w*3 + r) % len(requests)
					head, body, err := send(requests[i])
					if err != nil || head != wantHead[i] || stable[i] && !bytes.Equal(body, wantBody[i]) {
						mu.Lock()
						failures[fmt.Sprintf("%s %s (%d)", requests[i].method, requests[i].path, i)]++
						mu.Unlock()
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	for name, n := range failures {
		t.Errorf("%s: %d concurrent responses differ from the sequential one", name, n)
	}
}
