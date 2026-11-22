package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/server/internal/auth"
	"github.com/ritoon/zerechnung/server/internal/config"
	"github.com/ritoon/zerechnung/server/internal/keys"
	"github.com/ritoon/zerechnung/server/internal/metrics"
	"github.com/ritoon/zerechnung/server/internal/pades"
	"github.com/ritoon/zerechnung/validate"
)

// now is within the validity of the test certificate.
var now = time.Date(2027, 1, 15, 9, 30, 0, 0, time.UTC)

// blankPDF is a one page PDF without text, which ConvertPDFA can convert.
func blankPDF() []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	content := "0.9 g 72 72 451 698 re f\n"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
	}
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// report is a validation report as a client reads it.
type report struct {
	Valid    bool   `json:"valid"`
	Profile  string `json:"profile"`
	Format   string `json:"format"`
	Errors   int    `json:"errors"`
	Findings []struct {
		Rule     string `json:"rule"`
		Severity string `json:"severity"`
	} `json:"findings"`
}

// frenchInvoice is the invoice of the examples of package zerechnung, which
// meets the French rules.
func frenchInvoice() *invoice.Invoice {
	d := invoice.MustDecimal
	seller := invoice.Party{
		Name:              "Exemple Informatique SAS",
		IDs:               []invoice.Identifier{{ID: "12345678900017", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "123456789", Scheme: invoice.SchemeSIREN},
		VATID:             "FR32123456789",
		Address:           invoice.Address{Line1: "1 rue de l'Exemple", PostCode: "75001", City: "Paris", CountryCode: "FR"},
		ElectronicAddress: invoice.Identifier{ID: "123456789_FACTURES", Scheme: invoice.SchemeFRDirectory},
	}
	buyer := invoice.Party{
		Name:              "Client Exemple SARL",
		IDs:               []invoice.Identifier{{ID: "98765432100015", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "987654321", Scheme: invoice.SchemeSIREN},
		VATID:             "FR58987654321",
		Address:           invoice.Address{Line1: "2 place de l'Exemple", PostCode: "69002", City: "Lyon", CountryCode: "FR"},
		ElectronicAddress: invoice.Identifier{ID: "987654321_ACHATS", Scheme: invoice.SchemeFRDirectory},
	}
	return &invoice.Invoice{
		Profile:         invoice.ProfileEN16931,
		BusinessProcess: "S1",
		Number:          "FA-2026-00042",
		TypeCode:        invoice.TypeCommercialInvoice,
		IssueDate:       invoice.NewDate(2026, time.October, 15),
		Currency:        "EUR",
		Notes: []invoice.Note{
			{SubjectCode: "BAR", Content: "B2B"},
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Pénalités de retard : trois fois le taux d'intérêt légal."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		BuyerReference: "SERVICE-ACHATS",
		Seller:         seller,
		Buyer:          buyer,
		DeliveryDate:   invoice.NewDate(2026, time.October, 12),
		PaymentMeans:   []invoice.PaymentMeans{{TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "FR7630006000011234567890189"}},
		PaymentTerms:   []invoice.PaymentTerms{{Description: "Paiement à 30 jours par virement.", DueDate: invoice.NewDate(2026, time.November, 14)}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Maintenance applicative, octobre 2026"},
			Quantity:        d("2"),
			UnitCode:        "DAY",
			NetPrice:        d("450.00"),
			VATCategoryCode: invoice.VATStandard,
			VATRate:         d("20.00"),
		}},
	}
}

func invoiceJSON(t *testing.T, inv *invoice.Invoice) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testSigner(t *testing.T) *keys.Signer {
	t.Helper()
	p12, err := os.ReadFile("../keys/testdata/modern.p12")
	if err != nil {
		t.Fatal(err)
	}
	k, certs, err := keys.ParsePKCS12(p12, "test1234")
	if err != nil {
		t.Fatal(err)
	}
	s, err := keys.New(k, certs)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type env struct {
	t   *testing.T
	srv *httptest.Server
	log *bytes.Buffer
}

func newEnv(t *testing.T, signer *keys.Signer) *env {
	t.Helper()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	a, err := auth.New(&config.Auth{Methods: []string{"basic"}, BasicUsers: "billing:s3cret-password", Realm: "zerechnung"}, http.DefaultClient, logger)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Deps{
		Logger: logger, Auth: a, Metrics: metrics.New(map[string]string{"version": "test"}), Signer: signer,
		Sign:         config.Sign{Hash: "sha256", Default: true, Reason: "Facture émise", Location: "Paris"},
		MaxBodyBytes: 4 << 20, MetricsEnabled: true, Version: "test",
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &env{t: t, srv: ts, log: &logBuf}
}

func (e *env) do(method, path, contentType string, body []byte, header ...string) (*http.Response, []byte) {
	e.t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	req.SetBasicAuth("billing", "s3cret-password")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for i := 0; i+1 < len(header); i += 2 {
		if header[i] == "Authorization" && header[i+1] == "" {
			req.Header.Del("Authorization")
			continue
		}
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp, out
}

func (e *env) postJSON(path string, v any, header ...string) (*http.Response, []byte) {
	e.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.do("POST", path, "application/json", b, header...)
}

func problemOf(t *testing.T, body []byte) Problem {
	t.Helper()
	var p Problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("not a problem: %v: %s", err, body)
	}
	return p
}

func TestCreateRenderedSignedInvoice(t *testing.T) {
	e := newEnv(t, testSigner(t))
	resp, body := e.postJSON("/v1/invoices", map[string]any{
		"invoice":    invoiceJSON(t, frenchInvoice()),
		"validation": map[string]any{"rules": []string{"fr"}},
		"language":   "fr",
	}, "X-Request-ID", "req-42", "traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	h := resp.Header
	if h.Get("Content-Type") != "application/pdf" || h.Get("X-Invoice-Number") != "FA-2026-00042" || h.Get("X-Invoice-Signed") != "true" ||
		h.Get("X-Request-ID") != "req-42" || !strings.Contains(h.Get("Content-Disposition"), `filename="FA-2026-00042.pdf"`) {
		t.Errorf("headers %v", h)
	}
	sigs, err := pades.Verify(body)
	if err != nil || len(sigs) != 1 || !sigs[0].WholeDocument || sigs[0].Reason != "Facture émise" || !sigs[0].Time.Equal(now) {
		t.Fatalf("signatures %+v, %v", sigs, err)
	}
	report, att, err := zerechnung.Check(body, &validate.Options{Rules: validate.Default | validate.France})
	if err != nil || !report.Valid() {
		t.Errorf("check: %v %v", err, report)
	}
	// Check describes the signature (SIG-14), and tells that neither its
	// chain nor its revocation was checked; nothing else.
	described := false
	for _, f := range report.Findings {
		described = described || f.Rule == "SIG-14"
		if f.Source != validate.SourceSignature || f.Severity != validate.Info {
			t.Errorf("check: %v", f)
		}
	}
	if !described {
		t.Errorf("check does not describe the signature: %v", report)
	}
	inv, _, err := zerechnung.Read(body)
	if err != nil || inv.Number != "FA-2026-00042" || inv.Totals.DuePayable.String() != "1080.00" || att.Filename != "factur-x.xml" {
		t.Errorf("read back: %v %+v", err, inv)
	}
	// The access log carries the request identifier, the caller and the
	// trace.
	logs := e.log.String()
	for _, want := range []string{`"request_id":"req-42"`, `"subject":"billing"`, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"status":201`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %s:\n%s", want, logs)
		}
	}
}

func TestCreateJSONResponse(t *testing.T) {
	e := newEnv(t, testSigner(t))
	resp, body := e.postJSON("/v1/invoices", map[string]any{"invoice": invoiceJSON(t, frenchInvoice()), "sign": false}, "Accept", "application/json")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var out InvoiceResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Signed || !out.Rendered || out.Profile != "EN 16931" || out.Report == nil || !out.Report.Valid || !bytes.HasPrefix(out.PDF, []byte("%PDF-")) || out.Size != len(out.PDF) {
		t.Errorf("%+v", out)
	}
	if sigs, _ := pades.Verify(out.PDF); len(sigs) != 0 {
		t.Error("signed although sign is false")
	}
}

func TestCreateInvalidInvoice(t *testing.T) {
	e := newEnv(t, testSigner(t))
	inv := frenchInvoice()
	inv.Seller.ElectronicAddress = invoice.Identifier{}
	resp, body := e.postJSON("/v1/invoices", map[string]any{"invoice": invoiceJSON(t, inv), "validation": map[string]any{"auto": true}})
	p := problemOf(t, body)
	if resp.StatusCode != http.StatusUnprocessableEntity || p.Code != "invalid_invoice" || p.Report == nil || p.Report.Valid || !strings.Contains(string(body), "BR-FR-13") {
		t.Errorf("status %d: %s", resp.StatusCode, body)
	}
	// The same invoice passes when the validation is skipped.
	resp, body = e.postJSON("/v1/invoices", map[string]any{"invoice": invoiceJSON(t, inv), "validation": map[string]any{"skip": true}})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("skip: status %d: %s", resp.StatusCode, body)
	}
}

func TestCreateBadRequests(t *testing.T) {
	e := newEnv(t, nil)
	for name, tc := range map[string]struct {
		body string
		want int
		code string
	}{
		"not JSON":         {`{`, 400, "bad_request"},
		"unknown field":    {`{"invoce": {}}`, 400, "bad_request"},
		"nothing":          {`{}`, 422, "bad_request"},
		"typo in invoice":  {`{"invoice": {"Numbr": "1"}}`, 422, "bad_request"},
		"unknown profile":  {`{"invoice": {"Number": "1"}, "profile": "gold"}`, 422, "bad_request"},
		"unknown rule set": {`{"invoice": {"Number": "1"}, "validation": {"rules": ["xx"]}}`, 422, "bad_request"},
		"sign without key": {`{"invoice": {"Number": "1"}, "sign": true}`, 501, "signing_not_configured"},
		"pdf is not a PDF": {`{"invoice": {"Number": "1"}, "pdf": "aGVsbG8="}`, 422, "bad_request"},
		"invoice and xml":  {`{"invoice": {"Number": "1"}, "xml": "PHg+"}`, 422, "bad_request"},
	} {
		resp, body := e.do("POST", "/v1/invoices", "application/json", []byte(tc.body))
		if resp.StatusCode != tc.want || problemOf(t, body).Code != tc.code || resp.Header.Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: status %d: %s", name, resp.StatusCode, body)
		}
	}
}

func multipartBody(t *testing.T, parts map[string][]byte, files [][3]string) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for name, data := range parts {
		w, err := mw.CreateFormFile(name, name+".bin")
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	for _, f := range files {
		w, err := mw.CreateFormFile(f[0], f[1])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f[2]))
	}
	mw.Close()
	return b.Bytes(), mw.FormDataContentType()
}

func TestCreateMultipartWithHostPDF(t *testing.T) {
	e := newEnv(t, testSigner(t))
	opts, _ := json.Marshal(map[string]any{"convert_pdfa": true, "filename": "facture-42"})
	body, ct := multipartBody(t, map[string][]byte{
		"request": opts,
		"invoice": invoiceJSON(t, frenchInvoice()),
		"pdf":     blankPDF(),
	}, [][3]string{{"attachment", "timesheet.csv", "day;hours\n1;8\n"}})
	resp, out := e.do("POST", "/v1/invoices", ct, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "facture-42.pdf") {
		t.Errorf("filename %q", resp.Header.Get("Content-Disposition"))
	}
	all, err := zerechnung.ExtractAll(out)
	if err != nil || len(all) != 2 || all[1].Filename != "timesheet.csv" {
		t.Errorf("attachments: %v %d", err, len(all))
	}
	if _, err := pades.Verify(out); err != nil {
		t.Error(err)
	}
}

func TestCreateFromXML(t *testing.T) {
	e := newEnv(t, nil)
	resp, body := e.postJSON("/v1/invoices", map[string]any{"xml": testdata(t, "EN16931_Einfach.cii.xml")})
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("X-Invoice-Number") != "471102" || resp.Header.Get("X-Invoice-Signed") != "false" {
		t.Fatalf("status %d %v: %.300s", resp.StatusCode, resp.Header, body)
	}
	att, err := zerechnung.Extract(body)
	if err != nil || !bytes.Equal(att.XML, testdata(t, "EN16931_Einfach.cii.xml")) {
		t.Errorf("the XML is not embedded as is: %v", err)
	}
}

func TestValidate(t *testing.T) {
	e := newEnv(t, nil)
	resp, body := e.do("POST", "/v1/validate?rules=de", "application/pdf", testdata(t, "EN16931_Einfach.pdf"))
	var r report
	if err := json.Unmarshal(body, &r); err != nil || resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if r.Format != "Factur-X" || r.Profile != "EN 16931" || r.Valid || r.Errors == 0 || !strings.Contains(string(body), `"rule":"BR-DE-15"`) && !strings.Contains(string(body), `"rule": "BR-DE-15"`) {
		t.Errorf("%+v", r)
	}
	resp, body = e.do("POST", "/v1/validate", "application/xml", testdata(t, "XRECHNUNG_Einfach.ubl.xml"))
	if err := json.Unmarshal(body, &r); err != nil || resp.StatusCode != 200 || r.Format != "UBL" || !r.Valid {
		t.Errorf("UBL: status %d: %s", resp.StatusCode, body)
	}
	resp, body = e.do("POST", "/v1/validate?auto=maybe", "application/xml", []byte("<x/>"))
	if resp.StatusCode != 400 {
		t.Errorf("bad query: %d %s", resp.StatusCode, body)
	}
	body, ct := multipartBody(t, map[string][]byte{"file": testdata(t, "EN16931_Einfach.cii.xml")}, nil)
	resp, out := e.do("POST", "/v1/validate", ct, body)
	if err := json.Unmarshal(out, &r); err != nil || resp.StatusCode != 200 || r.Format != "CII" {
		t.Errorf("multipart: status %d: %s", resp.StatusCode, out)
	}
}

func TestSignAndVerify(t *testing.T) {
	e := newEnv(t, testSigner(t))
	resp, signed := e.do("POST", "/v1/sign?reason=Archivage", "application/pdf", testdata(t, "EN16931_Einfach.pdf"))
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("status %d: %.300s", resp.StatusCode, signed)
	}
	resp, body := e.do("POST", "/v1/verify", "application/pdf", signed)
	var v VerifyResponse
	if err := json.Unmarshal(body, &v); err != nil || resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !v.Valid || len(v.Signatures) != 1 || v.Signatures[0].Reason != "Archivage" || !strings.Contains(v.Signatures[0].Signer.Subject, "Exemple Informatique SAS") {
		t.Errorf("%+v", v)
	}
	// A modified byte breaks the signature.
	tampered := bytes.Clone(signed)
	i := bytes.Index(tampered, []byte("Lieferant GmbH"))
	tampered[i] = 'X'
	_, body = e.do("POST", "/v1/verify", "application/pdf", tampered)
	if err := json.Unmarshal(body, &v); err != nil || v.Valid || v.Error == "" {
		t.Errorf("tampered: %s", body)
	}
	// An unsigned PDF is not valid.
	_, body = e.do("POST", "/v1/verify", "application/pdf", testdata(t, "EN16931_Einfach.pdf"))
	if err := json.Unmarshal(body, &v); err != nil || v.Valid {
		t.Errorf("unsigned: %s", body)
	}
}

func TestSignWithoutKey(t *testing.T) {
	e := newEnv(t, nil)
	resp, body := e.do("POST", "/v1/sign", "application/pdf", testdata(t, "EN16931_Einfach.pdf"))
	if resp.StatusCode != http.StatusNotImplemented || problemOf(t, body).Code != "signing_not_configured" {
		t.Errorf("status %d: %s", resp.StatusCode, body)
	}
}

func TestExtract(t *testing.T) {
	e := newEnv(t, nil)
	pdf := testdata(t, "EN16931_Einfach.pdf")
	resp, body := e.do("POST", "/v1/extract", "application/pdf", pdf)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/xml" || !bytes.Contains(body, []byte("CrossIndustryInvoice")) {
		t.Errorf("xml: status %d", resp.StatusCode)
	}
	resp, body = e.do("POST", "/v1/extract?format=json", "application/pdf", pdf)
	var inv invoice.Invoice
	if err := json.Unmarshal(body, &inv); err != nil || resp.StatusCode != 200 || inv.Number != "471102" {
		t.Errorf("json: status %d: %.200s", resp.StatusCode, body)
	}
	resp, body = e.do("POST", "/v1/extract?format=json", "application/xml", testdata(t, "XRECHNUNG_Einfach.ubl.xml"))
	if err := json.Unmarshal(body, &inv); err != nil || resp.StatusCode != 200 {
		t.Errorf("UBL json: status %d: %.200s", resp.StatusCode, body)
	}
	// The JSON of the invoice goes back into /v1/invoices as is.
	resp, out := e.postJSON("/v1/invoices", map[string]any{"invoice": json.RawMessage(body), "validation": map[string]any{"skip": true}})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("round trip: status %d: %s", resp.StatusCode, out)
	}
}

func TestWriteXML(t *testing.T) {
	e := newEnv(t, nil)
	for _, format := range []string{"cii", "ubl", "fatturapa", "facturae"} {
		resp, body := e.postJSON("/v1/xml", map[string]any{"invoice": invoiceJSON(t, frenchInvoice()), "format": format, "validation": map[string]any{"skip": format == "fatturapa" || format == "facturae"}})
		if resp.StatusCode != 200 || !bytes.HasPrefix(bytes.TrimSpace(body), []byte("<?xml")) {
			t.Errorf("%s: status %d: %.300s", format, resp.StatusCode, body)
		}
	}
}

func TestPDFA(t *testing.T) {
	e := newEnv(t, nil)
	resp, body := e.do("POST", "/v1/pdfa", "application/pdf", blankPDF())
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("pdfaid:part")) {
		t.Fatalf("status %d: %.300s", resp.StatusCode, body)
	}
	// A PDF whose fonts are not embedded cannot be converted: the reasons
	// say why.
	resp, body = e.do("POST", "/v1/pdfa", "application/pdf", testdata(t, "host.pdf"))
	if p := problemOf(t, body); resp.StatusCode != 422 || p.Code != "not_convertible" || len(p.Reasons) == 0 {
		t.Errorf("status %d: %s", resp.StatusCode, body)
	}
}

func TestAuthAndLimits(t *testing.T) {
	e := newEnv(t, nil)
	resp, _ := e.do("GET", "/v1/info", "", nil, "Authorization", "")
	if resp.StatusCode != 401 || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Basic ") {
		t.Errorf("no credentials: %d", resp.StatusCode)
	}
	resp, _ = e.do("GET", "/v1/info", "", nil, "Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("billing:wrong")))
	if resp.StatusCode != 401 {
		t.Errorf("wrong password: %d", resp.StatusCode)
	}
	resp, body := e.do("POST", "/v1/validate", "application/xml", bytes.Repeat([]byte("x"), 5<<20))
	if resp.StatusCode != http.StatusRequestEntityTooLarge || problemOf(t, body).Code != "too_large" {
		t.Errorf("too large: %d %s", resp.StatusCode, body)
	}
	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/openapi.json"} {
		if resp, _ := e.do("GET", path, "", nil, "Authorization", ""); resp.StatusCode != 200 {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
}

func TestInfoMetricsOpenAPI(t *testing.T) {
	e := newEnv(t, testSigner(t))
	resp, body := e.do("GET", "/v1/info", "", nil)
	var in Info
	if err := json.Unmarshal(body, &in); err != nil || resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !in.Signing.Enabled || in.Signing.Algorithm != "RSA 2048" || in.Signing.Chain != 2 || in.Subject != "billing" || len(in.Languages) != 7 {
		t.Errorf("%+v", in)
	}
	_, body = e.do("GET", "/metrics", "", nil)
	for _, want := range []string{
		`zerechnung_http_requests_total{route="/v1/info",method="GET",status="200"} 1`,
		`zerechnung_http_request_duration_seconds_bucket{route="/v1/info",method="GET",le="+Inf"} 1`,
		"zerechnung_build_info{version=\"test\"} 1",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("metrics lack %s:\n%s", want, body)
		}
	}
	_, body = e.do("GET", "/openapi.json", "", nil)
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	inv := schemas["Invoice"].(map[string]any)["properties"].(map[string]any)
	if inv["Number"].(map[string]any)["type"] != "string" || inv["Seller"].(map[string]any)["$ref"] != "#/components/schemas/Party" || inv["IssueDate"].(map[string]any)["format"] != "date" {
		t.Errorf("Invoice schema: %v", inv)
	}
	if _, ok := schemas["Party"]; !ok {
		t.Error("no Party schema")
	}
}

// TestExampleRequest checks the request of the examples of the README: the
// invoice of package zerechnung's examples, which meets the French rules.
func TestExampleRequest(t *testing.T) {
	body, err := os.ReadFile("../../examples/invoice-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var req struct{ Invoice *invoice.Invoice }
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if got, want := invoiceJSON(t, req.Invoice), invoiceJSON(t, frenchInvoice()); !bytes.Equal(got, want) {
		t.Errorf("the example differs from frenchInvoice:\n%s\n%s", got, want)
	}
	e := newEnv(t, testSigner(t))
	resp, out := e.do("POST", "/v1/invoices", "application/json", body)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("X-Invoice-Signed") != "true" {
		t.Errorf("status %d: %.300s", resp.StatusCode, out)
	}
}
