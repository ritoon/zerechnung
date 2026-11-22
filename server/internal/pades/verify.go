package pades

import (
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/validate"
)

// Signature describes a signature of a document that Verify has checked.
type Signature struct {
	// Field is the fully qualified name of the signature field.
	Field string
	// SubFilter is the encoding of the signature, such as
	// ETSI.CAdES.detached (PAdES) or adbe.pkcs7.detached, or ETSI.RFC3161
	// for a document time-stamp.
	SubFilter string
	// Signer is the signing certificate (of the time-stamping authority
	// for a document time-stamp).
	Signer *x509.Certificate
	// Time is the signing time of /M, or of the signing-time attribute
	// when /M is absent (the time of the token for a document
	// time-stamp); zero when neither is there.
	Time time.Time
	// Name, Reason and Location are the /Name, /Reason and /Location
	// entries of the signature dictionary.
	Name, Reason, Location string
	// WholeDocument tells that the byte range covers the whole file but
	// the signature value: the signature is the last change of the
	// document.
	WholeDocument bool
}

// Verify finds every signature field of the document and checks, for each
// one, that its /ByteRange is well formed and excludes exactly its
// /Contents string, that the digest of the signed bytes matches and that
// the CMS signature verifies with its signing certificate. It neither
// builds a trust chain nor checks revocation. It returns the signatures in
// the order they were added, or an error naming the first one that fails.
// A document without signatures gives no signature and no error.
func Verify(data []byte) ([]Signature, error) {
	report, sigs, err := zerechnung.VerifySignatures(data, nil)
	if err != nil {
		if errors.Is(err, zerechnung.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("pades: %w", err)
	}
	out := make([]Signature, 0, len(sigs))
	for _, s := range sigs {
		if !s.Valid {
			return nil, fmt.Errorf("pades: signature %q: %s", s.Field, reason(report, s.Field))
		}
		out = append(out, Signature{
			Field: s.Field, SubFilter: s.SubFilter, Signer: s.Certificate, Time: s.Time,
			Name: s.Name, Reason: s.Reason, Location: s.Location, WholeDocument: s.WholeDocument,
		})
	}
	return out, nil
}

// reason returns the message of the first integrity error of the report
// for the signature field: SIG-01 (the signature cannot be read), SIG-02
// (its byte range) or SIG-03 (it does not verify).
func reason(r *validate.Report, field string) string {
	path := fmt.Sprintf("signature field %q", field)
	for _, f := range r.Findings {
		if f.Severity != validate.Error || !strings.HasPrefix(f.Path, path) {
			continue
		}
		switch f.Rule {
		case "SIG-01", "SIG-02", "SIG-03":
			return f.Message
		}
	}
	return "the signature does not verify"
}
