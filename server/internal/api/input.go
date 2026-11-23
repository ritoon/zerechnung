package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// part is a part of a multipart request, read in memory: the body limit
// bounds it, and no temporary file is written, so the container may run
// with a read-only file system.
type part struct {
	filename    string
	contentType string
	data        []byte
}

// readMultipart reads the parts of a multipart/form-data body, by name.
func readMultipart(r *http.Request) (map[string][]part, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	out := map[string][]part{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(p)
		if err != nil {
			return nil, err
		}
		out[p.FormName()] = append(out[p.FormName()], part{filename: p.FileName(), contentType: p.Header.Get("Content-Type"), data: data})
	}
}

func isMultipart(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt == "multipart/form-data"
}

// document reads the document of a request: the body itself, or the part
// named file of a multipart form.
func document(r *http.Request) ([]byte, error) {
	if isMultipart(r) {
		parts, err := readMultipart(r)
		if err != nil {
			return nil, err
		}
		f := parts["file"]
		if len(f) != 1 {
			return nil, errors.New(`the multipart form must hold one part named "file"`)
		}
		return f[0].data, nil
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty body: send the document as the body, or as the part \"file\" of a multipart form")
	}
	return data, nil
}

// decodeJSON decodes a JSON value strictly: an unknown field is an error,
// which catches the typos in field names.
func decodeJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after the JSON value")
	}
	return nil
}

func decodeInvoice(raw json.RawMessage) (*invoice.Invoice, error) {
	var inv invoice.Invoice
	if err := decodeJSON(raw, &inv); err != nil {
		return nil, fmt.Errorf("invoice: %w", err)
	}
	return &inv, nil
}

func isPDF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\t\n\r "), []byte("%PDF-"))
}

// looksXML reports whether data starts like an XML document.
func looksXML(data []byte) bool {
	d := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(d) > 0 && d[0] == '<'
}

// wantsJSON reports whether the client asks for a JSON response instead of
// the document itself.
func wantsJSON(r *http.Request) bool {
	for a := range strings.SplitSeq(r.Header.Get("Accept"), ",") {
		if mt, _, err := mime.ParseMediaType(strings.TrimSpace(a)); err == nil && mt == "application/json" {
			return true
		}
	}
	return false
}

// safeFilename keeps the characters of a name that need no escaping in a
// Content-Disposition header.
func safeFilename(name, ext string) string {
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			b.WriteRune(c)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "._")
	if s == "" {
		s = "invoice"
	}
	if len(s) > 100 {
		s = s[:100]
	}
	if !strings.HasSuffix(strings.ToLower(s), ext) {
		s += ext
	}
	return s
}

// Validation selects the rules applied to an invoice.
type Validation struct {
	// Skip disables the validation.
	Skip bool `json:"skip"`
	// Auto adds the national rules of the seller's country.
	Auto bool `json:"auto"`
	// Rules adds rule sets: fr, de, choruspro, peppol, nl, it, pt, ro, hr,
	// face.
	Rules []string `json:"rules"`
	// Profile validates against this profile instead of BT-24.
	Profile string `json:"profile"`
	// RejectBasicWL reports BASIC WL as an error under the French rules.
	RejectBasicWL bool `json:"reject_basicwl"`
	// ServiceCodeRequired and EngagementRequired describe the public
	// buyer for the Chorus Pro checks.
	ServiceCodeRequired bool `json:"service_code_required"`
	EngagementRequired  bool `json:"engagement_required"`
}

// validationFromQuery reads the validation options of the query string:
// rules=fr,de, auto, profile, reject_basicwl, service_code_required,
// engagement_required.
func validationFromQuery(q url.Values) (*Validation, error) {
	v := &Validation{Profile: q.Get("profile")}
	for _, r := range q["rules"] {
		for name := range strings.SplitSeq(r, ",") {
			if name = strings.TrimSpace(name); name != "" {
				v.Rules = append(v.Rules, name)
			}
		}
	}
	for name, dst := range map[string]*bool{
		"auto": &v.Auto, "reject_basicwl": &v.RejectBasicWL,
		"service_code_required": &v.ServiceCodeRequired, "engagement_required": &v.EngagementRequired,
	} {
		if s := q.Get(name); s != "" {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not a boolean", name, s)
			}
			*dst = b
		}
	}
	return v, nil
}

// options returns the options of package validate; nil when the
// validation is skipped.
func (v *Validation) options() (*validate.Options, error) {
	if v == nil {
		return &validate.Options{}, nil
	}
	if v.Skip {
		return nil, nil
	}
	o := &validate.Options{
		Auto:          v.Auto,
		RejectBasicWL: v.RejectBasicWL,
		PublicBuyer:   validate.PublicBuyer{ServiceCodeRequired: v.ServiceCodeRequired, EngagementRequired: v.EngagementRequired},
	}
	if v.Profile != "" {
		if o.Profile = invoice.ParseProfile(v.Profile); o.Profile == invoice.ProfileUnknown {
			return nil, fmt.Errorf("unknown profile %q", v.Profile)
		}
	}
	if len(v.Rules) > 0 {
		o.Rules = validate.Default
		for _, r := range v.Rules {
			switch strings.ToLower(r) {
			case "fr", "france":
				o.Rules |= validate.France
			case "de", "germany":
				o.Rules |= validate.Germany | validate.GermanyProfile
			case "choruspro":
				o.Rules |= validate.ChorusPro
			case "peppol":
				o.Rules |= validate.Peppol
			case "nl", "netherlands":
				o.Rules |= validate.Netherlands
			case "it", "italy":
				o.Rules |= validate.Italy
			case "pt", "portugal":
				o.Rules |= validate.Portugal
			case "ro", "romania":
				o.Rules |= validate.Romania
			case "sk", "slovakia":
				o.Rules |= validate.Slovakia
			case "hr", "croatia":
				o.Rules |= validate.Croatia
			case "face":
				o.Rules |= validate.FACe
			default:
				return nil, fmt.Errorf("unknown rule set %q (fr, de, choruspro, peppol, nl, it, pt, ro, sk, hr, face)", r)
			}
		}
	}
	return o, nil
}

// Report is a validation report.
type Report struct {
	Valid    bool               `json:"valid"`
	Profile  string             `json:"profile,omitempty"`
	Format   string             `json:"format,omitempty"`
	Errors   int                `json:"errors"`
	Warnings int                `json:"warnings"`
	Findings []validate.Finding `json:"findings"`
}

func newReport(r *validate.Report, format string) *Report {
	out := &Report{Valid: r.Valid(), Format: format, Errors: len(r.Errors()), Warnings: len(r.Warnings()), Findings: r.Findings}
	if r.Profile != invoice.ProfileUnknown {
		out.Profile = r.Profile.String()
	}
	if out.Findings == nil {
		out.Findings = []validate.Finding{}
	}
	return out
}
