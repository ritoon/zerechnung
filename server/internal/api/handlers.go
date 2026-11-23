package api

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/server/internal/metrics"
	"github.com/ritoon/zerechnung/server/internal/pades"
	"github.com/ritoon/zerechnung/server/internal/render"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

// InvoiceRequest is the body of POST /v1/invoices. Exactly one of
// Invoice and XML is set.
type InvoiceRequest struct {
	// Invoice is the invoice, in the JSON form of invoice.Invoice (the
	// output of zerechnung json).
	Invoice json.RawMessage `json:"invoice,omitempty"`
	// XML is a CII invoice produced elsewhere, embedded as is.
	XML []byte `json:"xml,omitempty"`
	// PDF is the PDF to turn into a Factur-X invoice; when empty, the
	// invoice is rendered as a PDF/A-3 document.
	PDF []byte `json:"pdf,omitempty"`
	// Profile overrides the profile of the invoice.
	Profile string `json:"profile,omitempty"`
	// Language of the rendered PDF: en, fr, de, it, es, pt, nl.
	Language string `json:"language,omitempty"`
	// Calculate computes the line amounts, the totals and the VAT
	// breakdown; true when omitted.
	Calculate *bool `json:"calculate,omitempty"`
	// Validation selects the rules; the invoice is refused when the
	// report has errors.
	Validation *Validation `json:"validation,omitempty"`
	// ConvertPDFA converts a PDF that does not declare PDF/A to PDF/A-3.
	ConvertPDFA bool `json:"convert_pdfa,omitempty"`
	// RequirePDFA refuses a PDF that does not declare PDF/A.
	RequirePDFA bool `json:"require_pdfa,omitempty"`
	// Sign signs the PDF (PAdES); ZR_SIGN_DEFAULT when omitted.
	Sign *bool `json:"sign,omitempty"`
	// Reason and Location of the signature, instead of ZR_SIGN_REASON and
	// ZR_SIGN_LOCATION.
	Reason   string `json:"reason,omitempty"`
	Location string `json:"location,omitempty"`
	// Attachments are supporting documents embedded besides the invoice.
	Attachments []Attachment `json:"attachments,omitempty"`
	// Filename of the PDF returned; from the invoice number when empty.
	Filename string `json:"filename,omitempty"`
}

// Attachment is a supporting document.
type Attachment struct {
	Name        string `json:"name"`
	MimeType    string `json:"mime_type,omitempty"`
	Description string `json:"description,omitempty"`
	Data        []byte `json:"data"`
}

// InvoiceResponse is the JSON response of POST /v1/invoices, sent when the
// client accepts application/json; the PDF itself is sent otherwise.
type InvoiceResponse struct {
	Filename string  `json:"filename"`
	Number   string  `json:"number,omitempty"`
	Profile  string  `json:"profile"`
	Rendered bool    `json:"rendered"`
	Signed   bool    `json:"signed"`
	Size     int     `json:"size"`
	SHA256   string  `json:"sha256"`
	Report   *Report `json:"report,omitempty"`
	PDF      []byte  `json:"pdf"`
}

// readInvoiceRequest reads a JSON body or a multipart form whose parts are
// request (the JSON options), invoice, xml, pdf and attachment.
func readInvoiceRequest(r *http.Request) (*InvoiceRequest, error) {
	req := &InvoiceRequest{}
	if !isMultipart(r) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if err := decodeJSON(data, req); err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
		return req, nil
	}
	parts, err := readMultipart(r)
	if err != nil {
		return nil, err
	}
	one := func(name string) ([]byte, error) {
		switch p := parts[name]; len(p) {
		case 0:
			return nil, nil
		case 1:
			return p[0].data, nil
		}
		return nil, fmt.Errorf("several parts named %q", name)
	}
	if b, err := one("request"); err != nil {
		return nil, err
	} else if b != nil {
		if err := decodeJSON(b, req); err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
	}
	for name, dst := range map[string]*[]byte{"xml": &req.XML, "pdf": &req.PDF} {
		b, err := one(name)
		if err != nil {
			return nil, err
		}
		if b != nil {
			*dst = b
		}
	}
	b, err := one("invoice")
	if err != nil {
		return nil, err
	}
	if b != nil {
		req.Invoice = b
	}
	for _, p := range parts["attachment"] {
		if p.filename == "" {
			return nil, errors.New(`an "attachment" part has no file name`)
		}
		mt := p.contentType
		if mt == "application/octet-stream" {
			mt = "" // guessed from the extension
		}
		req.Attachments = append(req.Attachments, Attachment{Name: p.filename, MimeType: mt, Data: p.data})
	}
	return req, nil
}

func (s *Server) count(operation, result string) {
	s.Metrics.Add("zerechnung_operations_total", "Operations by type and result.", metrics.Labels("operation", operation, "result", result), 1)
}

// createInvoice is POST /v1/invoices: invoice → PDF (rendered or given) →
// CII XML embedded → validation → PAdES signature.
func (s *Server) createInvoice(w http.ResponseWriter, r *http.Request) {
	req, err := readInvoiceRequest(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	bad := func(code, title, detail string) {
		s.count("invoice", "refused")
		s.problem(w, r, http.StatusUnprocessableEntity, code, title, detail)
	}
	if (len(req.Invoice) == 0) == (len(req.XML) == 0) {
		bad("bad_request", "invalid request", "send either invoice (the invoice as JSON) or xml (a CII invoice)")
		return
	}
	vopts, err := req.Validation.options()
	if err != nil {
		bad("bad_request", "invalid validation options", err.Error())
		return
	}
	signIt := s.Sign.Default && s.Signer != nil
	if req.Sign != nil {
		signIt = *req.Sign
	}
	if signIt {
		if err := s.signerReady(); err != nil {
			s.signingProblem(w, r, err)
			return
		}
	}
	var profile invoice.Profile
	if req.Profile != "" {
		if profile = invoice.ParseProfile(req.Profile); profile == invoice.ProfileUnknown {
			bad("bad_request", "invalid request", fmt.Sprintf("unknown profile %q", req.Profile))
			return
		}
	}
	now := s.Now()

	// The invoice, as the model and as XML.
	var inv *invoice.Invoice
	xml := req.XML
	if len(req.Invoice) > 0 {
		if inv, err = decodeInvoice(req.Invoice); err != nil {
			bad("bad_request", "invalid invoice", err.Error())
			return
		}
		if profile != invoice.ProfileUnknown {
			inv.Profile = profile
		}
		if inv.Profile == invoice.ProfileUnknown {
			inv.Profile = invoice.ProfileEN16931
		}
		if req.Calculate == nil || *req.Calculate {
			inv.Calculate()
		}
		if xml, err = cii.Marshal(inv, &cii.MarshalOptions{Profile: inv.Profile}); err != nil {
			bad("invalid_invoice", "the invoice cannot be written as CII", err.Error())
			return
		}
	} else if len(req.PDF) == 0 {
		// The XML alone: its invoice is rendered.
		if inv, err = cii.Parse(xml); inv == nil {
			bad("invalid_xml", "the XML cannot be read as a CII invoice", err.Error())
			return
		}
	}

	// The PDF: the one given, or the rendering of the invoice.
	host := req.PDF
	rendered := len(host) == 0
	if rendered {
		if host, err = render.Invoice(inv, &render.Options{Language: req.Language, Time: now}); err != nil {
			bad("render_failed", "the invoice cannot be rendered", err.Error())
			return
		}
	} else if !isPDF(host) {
		bad("bad_request", "invalid PDF", "pdf does not start with %PDF-")
		return
	}

	eo := &zerechnung.EmbedOptions{Profile: profile, ModDate: now, ConvertPDFA: req.ConvertPDFA, RequirePDFA: req.RequirePDFA}
	if inv != nil && len(req.Invoice) > 0 {
		eo.Profile = inv.Profile
	}
	for _, a := range req.Attachments {
		eo.Attachments = append(eo.Attachments, zerechnung.File{Name: a.Name, MimeType: a.MimeType, Description: a.Description, Data: a.Data})
	}
	pdf, err := zerechnung.Embed(host, xml, eo)
	if err != nil {
		var ce *zerechnung.ConversionError
		if errors.As(err, &ce) {
			s.count("invoice", "refused")
			s.writeProblem(w, r, &Problem{Status: http.StatusUnprocessableEntity, Code: "not_convertible",
				Title: "the PDF cannot be converted to PDF/A-3", Reasons: ce.Reasons})
			return
		}
		bad("embed_failed", "the invoice cannot be embedded in the PDF", err.Error())
		return
	}

	// The whole document is checked: PDF/A container and XML.
	var report *Report
	if vopts != nil {
		vr, _, err := s.checker.Check(pdf, vopts)
		if err != nil {
			bad("check_failed", "the PDF cannot be checked", err.Error())
			return
		}
		report = newReport(vr, "Factur-X")
		if !report.Valid {
			s.count("invoice", "invalid")
			s.writeProblem(w, r, &Problem{Status: http.StatusUnprocessableEntity, Code: "invalid_invoice",
				Title: "the invoice is not valid", Detail: fmt.Sprintf("%d error(s), first: %s", report.Errors, vr.Errors()[0]), Report: report})
			return
		}
	}

	if signIt {
		if pdf, err = s.signPDF(pdf, req.Reason, req.Location, now); err != nil {
			s.signingProblem(w, r, err)
			return
		}
	}
	s.count("invoice", "created")

	number := ""
	if inv != nil {
		number = inv.Number
	} else if parsed, _ := cii.Parse(xml); parsed != nil {
		number = parsed.Number
	}
	name := req.Filename
	if name == "" {
		name = number
	}
	name = safeFilename(name, ".pdf")
	p := eo.Profile
	if p == invoice.ProfileUnknown {
		p, _ = zerechnung.DetectProfile(xml)
	}
	sum := sha256.Sum256(pdf)
	if wantsJSON(r) {
		writeJSON(w, http.StatusCreated, &InvoiceResponse{
			Filename: name, Number: number, Profile: p.String(), Rendered: rendered, Signed: signIt,
			Size: len(pdf), SHA256: hex.EncodeToString(sum[:]), Report: report, PDF: pdf,
		})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	h.Set("X-Invoice-Number", strings.Map(printableASCII, number))
	h.Set("X-Invoice-Profile", p.String())
	h.Set("X-Invoice-Signed", strconv.FormatBool(signIt))
	h.Set("Repr-Digest", "sha-256=:"+base64.StdEncoding.EncodeToString(sum[:])+":")
	if report != nil {
		h.Set("X-Validation-Warnings", strconv.Itoa(report.Warnings))
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(pdf)
}

func printableASCII(c rune) rune {
	if c < 0x20 || c > 0x7e {
		return '_'
	}
	return c
}

// XMLRequest is the body of POST /v1/xml.
type XMLRequest struct {
	Invoice json.RawMessage `json:"invoice"`
	// Format is cii (default), ubl, fatturapa or facturae.
	Format string `json:"format,omitempty"`
	// Profile overrides the profile of the invoice (CII and UBL).
	Profile    string      `json:"profile,omitempty"`
	Calculate  *bool       `json:"calculate,omitempty"`
	Validation *Validation `json:"validation,omitempty"`
}

// writeXML is POST /v1/xml: invoice → CII, UBL, FatturaPA or Facturae.
func (s *Server) writeXML(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	var req XMLRequest
	if err := decodeJSON(data, &req); err != nil {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "malformed request", err.Error())
		return
	}
	bad := func(code, title, detail string) {
		s.count("xml", "refused")
		s.problem(w, r, http.StatusUnprocessableEntity, code, title, detail)
	}
	if len(req.Invoice) == 0 {
		bad("bad_request", "invalid request", "invoice is required")
		return
	}
	inv, err := decodeInvoice(req.Invoice)
	if err != nil {
		bad("bad_request", "invalid invoice", err.Error())
		return
	}
	vopts, err := req.Validation.options()
	if err != nil {
		bad("bad_request", "invalid validation options", err.Error())
		return
	}
	if req.Profile != "" {
		if inv.Profile = invoice.ParseProfile(req.Profile); inv.Profile == invoice.ProfileUnknown {
			bad("bad_request", "invalid request", fmt.Sprintf("unknown profile %q", req.Profile))
			return
		}
	}
	if req.Calculate == nil || *req.Calculate {
		inv.Calculate()
	}
	var xml []byte
	format := strings.ToLower(req.Format)
	switch format {
	case "", "cii":
		format = "cii"
		p := inv.Profile
		if p == invoice.ProfileUnknown {
			p = invoice.ProfileEN16931
		}
		xml, err = cii.Marshal(inv, &cii.MarshalOptions{Profile: p})
	case "ubl":
		xml, err = ubl.Marshal(inv, nil)
	case "fatturapa":
		xml, err = fatturapa.Marshal(inv, nil)
	case "facturae":
		xml, err = facturae.Marshal(inv, nil)
	default:
		bad("bad_request", "invalid request", fmt.Sprintf("unknown format %q (cii, ubl, fatturapa, facturae)", req.Format))
		return
	}
	if err != nil {
		bad("invalid_invoice", "the invoice cannot be written as "+format, err.Error())
		return
	}
	var report *Report
	if vopts != nil {
		vr := validate.XML(xml, vopts)
		report = newReport(vr, format)
		if !report.Valid {
			s.count("xml", "invalid")
			s.writeProblem(w, r, &Problem{Status: http.StatusUnprocessableEntity, Code: "invalid_invoice",
				Title: "the invoice is not valid", Detail: fmt.Sprintf("%d error(s), first: %s", report.Errors, vr.Errors()[0]), Report: report})
			return
		}
	}
	s.count("xml", "created")
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{"format": format, "xml": string(xml), "report": report})
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeFilename(inv.Number, ".xml")))
	_, _ = w.Write(xml)
}

// validate is POST /v1/validate: a hybrid PDF or an XML invoice → report.
func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	v, err := validationFromQuery(r.URL.Query())
	if err != nil {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", err.Error())
		return
	}
	opts, err := v.options()
	if err != nil {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", err.Error())
		return
	}
	data, err := document(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	var vr *validate.Report
	format := ""
	if isPDF(data) {
		format = "PDF"
		var att *zerechnung.Attachment
		if vr, att, err = s.checker.Check(data, opts); err != nil {
			s.problem(w, r, http.StatusUnprocessableEntity, "unreadable_pdf", "the PDF cannot be read", err.Error())
			return
		}
		if att != nil {
			format = "Factur-X"
		}
	} else {
		vr = validate.XML(data, opts)
		format = xmlFormat(data)
	}
	report := newReport(vr, format)
	result := "valid"
	if !report.Valid {
		result = "invalid"
	}
	s.count("validate", result)
	writeJSON(w, http.StatusOK, report)
}

// xmlFormat names the syntax of an XML invoice.
func xmlFormat(data []byte) string {
	root, err := xmltree.Parse(data)
	switch {
	case err != nil:
		return ""
	case fatturapa.IsFatturaPA(root):
		return strings.TrimSpace("FatturaPA " + root.Attr("versione"))
	case facturae.IsFacturae(root):
		return "Facturae " + facturae.Version(root)
	case ubl.IsUBL(root):
		return "UBL"
	case cii.IsZUGFeRD1(root):
		return "ZUGFeRD 1.0"
	}
	return "CII"
}

// errNoSigner is returned when a signature is asked without a key.
var errNoSigner = errors.New("no signing key is configured (ZR_SIGN_KEY or ZR_SIGN_PKCS12)")

type certError struct{ error }

func (s *Server) signerReady() error {
	if s.Signer == nil {
		return errNoSigner
	}
	if err := s.Signer.CheckValidity(s.Now()); err != nil {
		return certError{err}
	}
	return nil
}

func (s *Server) hash() crypto.Hash {
	switch s.Sign.Hash {
	case "sha384":
		return crypto.SHA384
	case "sha512":
		return crypto.SHA512
	}
	return crypto.SHA256
}

func (s *Server) signPDF(pdf []byte, reason, location string, now time.Time) ([]byte, error) {
	if reason == "" {
		reason = s.Sign.Reason
	}
	if location == "" {
		location = s.Sign.Location
	}
	out, err := pades.Sign(pdf, &pades.Options{
		Signer: s.Signer.Key, Chain: s.Signer.Chain, Hash: s.hash(), PSS: s.Sign.PSS, Time: now,
		Name: s.Sign.Name, Reason: reason, Location: location, ContactInfo: s.Sign.ContactInfo,
	})
	if err == nil {
		s.Metrics.Add("zerechnung_signatures_total", "Signatures by format.", metrics.Labels("format", "pades"), 1)
	}
	return out, err
}

// signingProblem answers the signature errors: the document cannot be
// signed (422), no key is configured (501), the certificate is not valid
// (503), or the key failed (500).
func (s *Server) signingProblem(w http.ResponseWriter, r *http.Request, err error) {
	s.count("sign", "failed")
	var ce certError
	switch {
	case errors.Is(err, errNoSigner):
		s.problem(w, r, http.StatusNotImplemented, "signing_not_configured", "signing is not configured", err.Error())
	case errors.As(err, &ce):
		s.problem(w, r, http.StatusServiceUnavailable, "certificate_not_valid", "the signing certificate is not valid", err.Error())
	case errors.Is(err, pades.ErrEncrypted), errors.Is(err, pades.ErrLocked), errors.Is(err, errNotSignable):
		s.problem(w, r, http.StatusUnprocessableEntity, "not_signable", "the document cannot be signed", err.Error())
	default:
		s.Logger.Error("signature failed", "request_id", requestID(r), "error", err)
		s.problem(w, r, http.StatusInternalServerError, "signing_failed", "the signature failed", err.Error())
	}
}

var errNotSignable = errors.New("not signable")

// signFacturae answers POST /v1/sign for a Facturae document: XAdES-EPES
// under the signature policy of Facturae (?format=xades or no format),
// with the role of the signer of ?role (emisor, receptor or tercero; by
// default the one that InvoiceIssuerType gives).
func (s *Server) signFacturae(w http.ResponseWriter, r *http.Request, data []byte, now time.Time) {
	q := r.URL.Query()
	if f := strings.ToLower(q.Get("format")); f != "" && f != "xades" {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", fmt.Sprintf("a Facturae document is signed in XAdES only, not %q", f))
		return
	}
	role := facturae.SignerRole(strings.ToLower(q.Get("role")))
	if role != "" && !role.Valid() {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", fmt.Sprintf("unknown role %q (emisor, receptor, tercero)", q.Get("role")))
		return
	}
	out, err := facturae.Sign(data, &facturae.SignOptions{Signer: s.Signer.Key, Chain: s.Signer.Chain, SigningTime: now, Role: role, PSS: s.Sign.PSS})
	if err != nil {
		s.signingProblem(w, r, fmt.Errorf("%w: %w", errNotSignable, err))
		return
	}
	s.Metrics.Add("zerechnung_signatures_total", "Signatures by format.", metrics.Labels("format", "xades-epes"), 1)
	s.count("sign", "xades-epes")
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(out)
}

// sign is POST /v1/sign: a PDF is signed in PAdES, a FatturaPA document
// in CAdES (.xml.p7m, ?format=cades, the default) or XAdES (?format=xades),
// a Facturae document in XAdES-EPES under the signature policy of Facturae
// (see signFacturae).
func (s *Server) sign(w http.ResponseWriter, r *http.Request) {
	if err := s.signerReady(); err != nil {
		s.signingProblem(w, r, err)
		return
	}
	data, err := document(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	q := r.URL.Query()
	now := s.Now()
	switch {
	case isPDF(data):
		out, err := s.signPDF(data, q.Get("reason"), q.Get("location"), now)
		if err != nil {
			if !errors.Is(err, pades.ErrEncrypted) && !errors.Is(err, pades.ErrLocked) {
				err = fmt.Errorf("%w: %w", errNotSignable, err)
			}
			s.signingProblem(w, r, err)
			return
		}
		s.count("sign", "pades")
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(out)
	case looksXML(data):
		if root, perr := xmltree.Parse(data); perr == nil && facturae.IsFacturae(root) {
			s.signFacturae(w, r, data, now)
			return
		}
		opts := &fatturapa.SignOptions{Signer: s.Signer.Key, Chain: s.Signer.Chain, SigningTime: now, PSS: s.Sign.PSS}
		format := strings.ToLower(q.Get("format"))
		var out []byte
		ct := "application/pkcs7-mime"
		switch format {
		case "", "cades":
			format = "cades"
			out, err = fatturapa.SignCAdES(data, opts)
		case "xades":
			ct = "application/xml"
			out, err = fatturapa.SignXAdES(data, opts)
		default:
			s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", fmt.Sprintf("unknown format %q (cades, xades)", format))
			return
		}
		if err != nil {
			s.signingProblem(w, r, fmt.Errorf("%w: %w", errNotSignable, err))
			return
		}
		s.Metrics.Add("zerechnung_signatures_total", "Signatures by format.", metrics.Labels("format", format), 1)
		s.count("sign", format)
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(out)
	default:
		s.problem(w, r, http.StatusUnprocessableEntity, "unsupported_document", "unsupported document", "send a PDF or a FatturaPA XML document")
	}
}

// Certificate describes a certificate.
type Certificate struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	Serial    string    `json:"serial"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
}

func describe(c *x509.Certificate) *Certificate {
	if c == nil {
		return nil
	}
	return &Certificate{Subject: c.Subject.String(), Issuer: c.Issuer.String(), Serial: c.SerialNumber.Text(16), NotBefore: c.NotBefore.UTC(), NotAfter: c.NotAfter.UTC()}
}

// SignatureInfo describes a signature found by POST /v1/verify.
type SignatureInfo struct {
	Field         string       `json:"field,omitempty"`
	Format        string       `json:"format"`
	Signer        *Certificate `json:"signer"`
	Time          *time.Time   `json:"time,omitempty"`
	Reason        string       `json:"reason,omitempty"`
	Location      string       `json:"location,omitempty"`
	WholeDocument *bool        `json:"whole_document,omitempty"`
	Qualified     *bool        `json:"qualified,omitempty"`
	// Policy and Role are the signature policy and the role of the signer
	// of a Facturae signature (XAdES-EPES).
	Policy string `json:"policy,omitempty"`
	Role   string `json:"role,omitempty"`
}

// VerifyResponse is the response of POST /v1/verify.
type VerifyResponse struct {
	Valid      bool            `json:"valid"`
	Error      string          `json:"error,omitempty"`
	Signatures []SignatureInfo `json:"signatures"`
}

// verify is POST /v1/verify: the integrity of the signatures of a PDF
// (PAdES), of a FatturaPA file (CAdES, XAdES) or of a Facturae document
// (XAdES-EPES). Trust in the certificates is not checked.
func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	data, err := document(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	resp := VerifyResponse{Signatures: []SignatureInfo{}}
	if isPDF(data) {
		sigs, err := pades.Verify(data)
		for _, sg := range sigs {
			t, whole := sg.Time, sg.WholeDocument
			info := SignatureInfo{Field: sg.Field, Format: sg.SubFilter, Signer: describe(sg.Signer), Reason: sg.Reason, Location: sg.Location, WholeDocument: &whole}
			if !t.IsZero() {
				info.Time = &t
			}
			resp.Signatures = append(resp.Signatures, info)
		}
		resp.Valid = err == nil && len(sigs) > 0
		if err != nil {
			resp.Error = err.Error()
		} else if len(sigs) == 0 {
			resp.Error = "the PDF is not signed"
		}
	} else if root, perr := xmltree.Parse(data); perr == nil && facturae.IsFacturae(root) {
		sig, err := facturae.ReadSignature(data)
		if sig != nil {
			q := sig.Qualified
			info := SignatureInfo{Format: "XAdES", Signer: describe(sig.Certificate), Qualified: &q, Role: string(sig.Role)}
			if sig.Policy != nil {
				info.Policy = sig.Policy.Identifier
			}
			if t := sig.SigningTime; !t.IsZero() {
				info.Time = &t
			}
			resp.Signatures = append(resp.Signatures, info)
		}
		resp.Valid = err == nil
		if err != nil {
			resp.Error = err.Error()
		}
	} else {
		_, sig, err := fatturapa.Unwrap(data)
		if sig != nil {
			for _, sg := range sig.Signers {
				q := sg.Qualified
				info := SignatureInfo{Format: sig.Format.String(), Signer: describe(sg.Certificate), Qualified: &q}
				if t := sg.SigningTime; !t.IsZero() {
					info.Time = &t
				}
				resp.Signatures = append(resp.Signatures, info)
			}
		}
		resp.Valid = err == nil
		if err != nil {
			resp.Error = err.Error()
		}
	}
	result := "valid"
	if !resp.Valid {
		result = "invalid"
	}
	s.count("verify", result)
	writeJSON(w, http.StatusOK, resp)
}

// extract is POST /v1/extract: the XML of a hybrid PDF (?format=xml, the
// default), or the invoice of a PDF, an XML invoice or a signed FatturaPA
// file as JSON (?format=json).
func (s *Server) extract(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != "" && format != "xml" && format != "json" {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "invalid query", fmt.Sprintf("unknown format %q (xml, json)", format))
		return
	}
	data, err := document(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	if isPDF(data) {
		att, err := zerechnung.Extract(data)
		if err != nil {
			s.count("extract", "failed")
			s.problem(w, r, http.StatusUnprocessableEntity, "no_invoice", "no invoice found in the PDF", err.Error())
			return
		}
		if format != "json" {
			s.count("extract", "xml")
			w.Header().Set("Content-Type", "application/xml")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeFilename(att.Filename, ".xml")))
			_, _ = w.Write(att.XML)
			return
		}
		data = att.XML
	} else if format != "json" {
		s.problem(w, r, http.StatusBadRequest, "bad_request", "not a PDF", "only a PDF has an XML to extract; use format=json to read an XML invoice")
		return
	} else if !looksXML(data) {
		doc, _, err := fatturapa.Unwrap(data)
		if err != nil {
			s.count("extract", "failed")
			s.problem(w, r, http.StatusUnprocessableEntity, "unreadable", "the document cannot be read", err.Error())
			return
		}
		data = doc
	}
	invs, err := parseInvoices(data)
	if len(invs) == 0 {
		s.count("extract", "failed")
		s.problem(w, r, http.StatusUnprocessableEntity, "unreadable", "the invoice cannot be read", fmt.Sprint(err))
		return
	}
	s.count("extract", "json")
	if err != nil {
		w.Header().Set("Warning", `199 zerechnung "`+strings.Map(printableASCII, err.Error())+`"`)
	}
	if len(invs) == 1 {
		writeJSON(w, http.StatusOK, invs[0])
		return
	}
	writeJSON(w, http.StatusOK, invs)
}

// parseInvoices reads the invoices of an XML document, whatever its
// syntax.
func parseInvoices(data []byte) ([]*invoice.Invoice, error) {
	var inv *invoice.Invoice
	var err error
	root, perr := xmltree.Parse(data)
	switch {
	case perr == nil && fatturapa.IsFatturaPA(root):
		return fatturapa.ParseAll(data)
	case perr == nil && facturae.IsFacturae(root):
		return facturae.ParseAll(data)
	case perr == nil && ubl.IsUBL(root):
		inv, err = ubl.Parse(data)
	default:
		inv, err = cii.Parse(data)
	}
	if inv == nil {
		return nil, err
	}
	return []*invoice.Invoice{inv}, err
}

// convertPDFA is POST /v1/pdfa: a PDF → PDF/A-3, or the reasons why it
// cannot be converted.
func (s *Server) convertPDFA(w http.ResponseWriter, r *http.Request) {
	data, err := document(r)
	if err != nil {
		s.bodyError(w, r, err)
		return
	}
	if !isPDF(data) {
		s.problem(w, r, http.StatusUnprocessableEntity, "bad_request", "not a PDF", "the body does not start with %PDF-")
		return
	}
	out, err := zerechnung.ConvertPDFA(data, &zerechnung.ConvertOptions{ModDate: s.Now()})
	if err != nil {
		s.count("pdfa", "refused")
		var ce *zerechnung.ConversionError
		if errors.As(err, &ce) {
			s.writeProblem(w, r, &Problem{Status: http.StatusUnprocessableEntity, Code: "not_convertible",
				Title: "the PDF cannot be converted to PDF/A-3", Reasons: ce.Reasons})
			return
		}
		s.problem(w, r, http.StatusUnprocessableEntity, "not_convertible", "the PDF cannot be converted to PDF/A-3", err.Error())
		return
	}
	s.count("pdfa", "converted")
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(out)
}

// Info is the response of GET /v1/info.
type Info struct {
	Version      string      `json:"version"`
	GoVersion    string      `json:"go_version"`
	AuthMethods  []string    `json:"auth_methods"`
	Signing      SigningInfo `json:"signing"`
	Languages    []string    `json:"languages"`
	Formats      []string    `json:"formats"`
	Profiles     []string    `json:"profiles"`
	RuleSets     []string    `json:"rule_sets"`
	MaxBodyBytes int64       `json:"max_body_bytes"`
	Subject      string      `json:"subject,omitempty"`
}

// SigningInfo describes the signing key.
type SigningInfo struct {
	Enabled     bool         `json:"enabled"`
	Default     bool         `json:"default"`
	Algorithm   string       `json:"algorithm,omitempty"`
	Hash        string       `json:"hash,omitempty"`
	PSS         bool         `json:"pss,omitempty"`
	Certificate *Certificate `json:"certificate,omitempty"`
	Chain       int          `json:"chain_length,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// info is GET /v1/info: what the server can do, and with which key.
func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	in := Info{
		Version:      s.Version,
		GoVersion:    runtime.Version(),
		AuthMethods:  s.Auth.Methods(),
		Languages:    render.Languages(),
		Formats:      []string{"cii", "ubl", "fatturapa", "facturae"},
		Profiles:     []string{"minimum", "basicwl", "basic", "en16931", "extended", "xrechnung", "extended-ctc-fr", "peppol", "nlcius", "cius-it", "cius-pt", "cius-ro"},
		RuleSets:     []string{"fr", "de", "choruspro", "peppol", "nl", "it", "pt", "ro", "sk", "face"},
		MaxBodyBytes: s.MaxBodyBytes,
	}
	if ri := reqInfoOf(r); ri != nil && ri.principal != nil {
		in.Subject = ri.principal.Subject
	}
	if s.Signer != nil {
		in.Signing = SigningInfo{
			Enabled: true, Default: s.Sign.Default, Algorithm: s.Signer.Algorithm(), Hash: s.Sign.Hash, PSS: s.Sign.PSS,
			Certificate: describe(s.Signer.Certificate()), Chain: len(s.Signer.Chain),
		}
		if err := s.Signer.CheckValidity(s.Now()); err != nil {
			in.Signing.Error = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, in)
}
