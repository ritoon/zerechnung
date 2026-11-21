package fatturapa

import (
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/esig"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Report is the result of VerifySignature: the same type as
// validate.Report, whose findings carry the error codes of the SdI.
type Report = rules.Report

// VerifyOptions configure VerifySignature. The zero value checks the
// signature itself and the validity of the certificate now, but neither its
// chain nor its revocation.
type VerifyOptions struct {
	// Roots are the trusted certification authorities: for an invoice to a
	// public administration, those of the list of qualified trust service
	// providers (the AgID list, or the EU trusted lists). Nothing is
	// downloaded: without Roots, the chain is not checked (IT-SIG-04).
	Roots *x509.CertPool
	// Intermediates are certificates that may complete the chain, besides
	// those the signature carries.
	Intermediates []*x509.Certificate
	// At is the time the SdI received the file: the certificate must be
	// valid then (00100, 00107), and the signing time must not be later
	// (00105). time.Now() when zero.
	At time.Time
	// CRLs and OCSP are revocation data obtained by the caller (CRLs
	// parsed by x509.ParseRevocationList, OCSP responses in DER). The
	// revocation (00101) is only checked against them.
	CRLs []*x509.RevocationList
	OCSP [][]byte
	// Name, when set, is the file name to check against the naming rules
	// of the SdI (00001), and against the format of the signature.
	Name string
}

// Rules of VerifySignature. The rules named after a code of the SdI follow
// the checks of §5.1.1 of its technical specifications (1.8.4); the others
// are specific to this package.
const (
	RuleFileName          = "00001"     // file name not valid
	RuleCertExpired       = "00100"     // signing certificate expired
	RuleCertRevoked       = "00101"     // signing certificate revoked
	RuleSignatureInvalid  = "00102"     // signature not valid
	RuleNoSigningTime     = "00103"     // no riferimento temporale
	RuleUntrustedCA       = "00104"     // CA not in the list of trusted CAs
	RuleSigningTimeAfter  = "00105"     // riferimento temporale after the reception
	RuleCertInvalid       = "00107"     // signing certificate not valid
	RuleNotSigned         = "IT-SIG-01" // the file is not signed
	RuleNotQualified      = "IT-SIG-02" // the certificate does not declare itself qualified
	RuleSeal              = "IT-SIG-03" // an electronic seal, not a signature
	RuleTrustNotChecked   = "IT-SIG-04" // no trusted roots given
	RuleRevocationUnknown = "IT-SIG-05" // no revocation data covers the certificate
	RuleNotBaselineB      = "IT-SIG-06" // not a CAdES or XAdES Baseline B signature
	RuleNotFatturaPA      = "IT-SIG-07" // the signed document is not a FatturaPA document
)

// SignatureRules lists the rules of VerifySignature.
var SignatureRules = []rules.RuleInfo{
	{ID: RuleFileName, Severity: rules.Error, Description: "The file name does not follow the naming rules of the SdI (country code, identifier of the transmitter, \"_\", progressive of 1 to 5 alphanumeric characters, .xml or .xml.p7m), or its extension does not match the format of the signature (.xml for XAdES, .xml.p7m for CAdES)."},
	{ID: RuleCertExpired, Severity: rules.Error, Description: "The signing certificate has expired at the reception of the file."},
	{ID: RuleCertRevoked, Severity: rules.Error, Description: "The signing certificate is revoked or suspended, according to the CRLs and OCSP responses given."},
	{ID: RuleSignatureInvalid, Severity: rules.Error, Description: "The signature is not valid: the file does not match what was signed, or the signature cannot be read or uses an algorithm that is not admitted."},
	{ID: RuleNoSigningTime, Severity: rules.Error, Description: "The signature has no riferimento temporale (signing time)."},
	{ID: RuleUntrustedCA, Severity: rules.Error, Description: "The certification authority that issued the signing certificate is not among the trusted ones."},
	{ID: RuleSigningTimeAfter, Severity: rules.Error, Description: "The riferimento temporale of the signature is later than the reception of the file."},
	{ID: RuleCertInvalid, Severity: rules.Error, Description: "The signing certificate is not valid: not valid yet at the reception of the file, a key usage that does not allow signing, or a chain that is not valid."},
	{ID: RuleNotSigned, Severity: rules.Error, Description: "The file is not signed: an error for an invoice to a public administration (FPA12), for which the signature is mandatory; information otherwise."},
	{ID: RuleNotQualified, Severity: rules.Warning, Description: "The signing certificate of an invoice to a public administration (FPA12) does not declare itself an EU qualified certificate (QCStatements of ETSI EN 319 412-5)."},
	{ID: RuleSeal, Severity: rules.Info, Description: "The certificate is issued for electronic seals (QcType eseal): a seal of a legal person was applied, not the signature of a natural person. A warning for an invoice to a public administration, for which the SdI asks for a qualified electronic signature."},
	{ID: RuleTrustNotChecked, Severity: rules.Warning, Description: "No trusted certification authority was given: the chain of the signing certificate is not checked (00104)."},
	{ID: RuleRevocationUnknown, Severity: rules.Info, Description: "No CRL or OCSP response given covers the signing certificate: its revocation is not checked (00101)."},
	{ID: RuleNotBaselineB, Severity: rules.Warning, Description: "The signature does not meet CAdES or XAdES Baseline B, the formats the SdI admits (ETSI EN 319 122-1 and EN 319 132-1, §6.3)."},
	{ID: RuleNotFatturaPA, Severity: rules.Error, Description: "The signed document is not a FatturaPA document."},
}

// VerifySignature checks a signed FatturaPA file as the SdI does on
// reception (technical specifications 1.8.4, §5.1.1), and reports each
// problem under the SdI error code: the file name (00001), the integrity of
// the document (00102), the signing certificate (expired 00100, revoked
// 00101, untrusted authority 00104, not valid 00107) and the riferimento
// temporale (missing 00103, after the reception 00105). What the SdI checks
// with data this package does not fetch is reported as not checked: trust
// without VerifyOptions.Roots, revocation without CRL or OCSP response.
//
// The file is a .xml.p7m (CAdES, in DER or base64) or a .xml (XAdES, or
// unsigned). An unsigned invoice to a public administration (FPA12) is an
// error, since the signature is mandatory for it.
func VerifySignature(file []byte, opts *VerifyOptions) *Report {
	if opts == nil {
		opts = &VerifyOptions{}
	}
	at := opts.At
	if at.IsZero() {
		at = time.Now()
	}
	r := &Report{}
	add := func(rule string, sev rules.Severity, format string, args ...any) {
		r.Add(rules.Finding{Rule: rule, Severity: sev, Source: rules.SourceSDI, Message: fmt.Sprintf(format, args...)})
	}

	doc, sig, err := Unwrap(file)
	if opts.Name != "" {
		format := Format(0)
		if sig != nil {
			format = sig.Format
		}
		if err := checkNameFor(opts.Name, format, isXML(file)); err != nil {
			add(RuleFileName, rules.Error, "%v", err)
		}
	}
	switch {
	case errors.Is(err, ErrNotSigned):
		pa := isFPA12(file)
		if pa {
			add(RuleNotSigned, rules.Error, "the file is not signed, but the signature is mandatory for an invoice to a public administration (FPA12)")
		} else {
			add(RuleNotSigned, rules.Info, "the file is not signed")
		}
		return r
	case err != nil:
		add(RuleSignatureInvalid, rules.Error, "%s", strings.TrimPrefix(err.Error(), "fatturapa: "))
		return r
	}
	pa := isFPA12(doc)
	if root, perr := xmltree.Parse(doc); perr != nil || checkRoot(root) != nil {
		add(RuleNotFatturaPA, rules.Error, "the signed document is not a FatturaPA document")
	}

	if opts.Roots == nil {
		add(RuleTrustNotChecked, rules.Warning, "no trusted certification authority given: the chain of the signing certificate is not checked")
	}
	for i, s := range sig.Signers {
		who := ""
		if len(sig.Signers) > 1 {
			who = fmt.Sprintf("signature %d of %d: ", i+1, len(sig.Signers))
		}
		cert := s.Certificate
		subject := cert.Subject.String()
		if s.SigningTime.IsZero() {
			add(RuleNoSigningTime, rules.Error, "%sthe signature has no signing time (riferimento temporale)", who)
		} else if s.SigningTime.After(at) {
			add(RuleSigningTimeAfter, rules.Error, "%sthe signing time %s is later than the reception of the file, %s", who, s.SigningTime.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339))
		}
		res := esig.Check(cert, s.Certificates, &esig.Options{
			Roots:         opts.Roots,
			Intermediates: opts.Intermediates,
			At:            at,
			CRLs:          opts.CRLs,
			OCSP:          opts.OCSP,
		})
		switch {
		case res.Expired:
			add(RuleCertExpired, rules.Error, "%sthe signing certificate (%s) expired on %s", who, subject, cert.NotAfter.UTC().Format(time.RFC3339))
		case res.NotYetValid:
			add(RuleCertInvalid, rules.Error, "%sthe signing certificate (%s) is not valid before %s", who, subject, cert.NotBefore.UTC().Format(time.RFC3339))
		}
		if res.KeyUsage != nil {
			add(RuleCertInvalid, rules.Error, "%s%v", who, res.KeyUsage)
		}
		switch {
		case res.Untrusted:
			add(RuleUntrustedCA, rules.Error, "%sthe signing certificate (%s, issued by %s) does not lead to a trusted certification authority", who, subject, cert.Issuer.String())
		case res.ChainErr != nil:
			add(RuleCertInvalid, rules.Error, "%sthe chain of the signing certificate is not valid: %v", who, res.ChainErr)
		}
		switch res.Revocation.Status {
		case esig.StatusRevoked:
			add(RuleCertRevoked, rules.Error, "%sthe signing certificate (%s) was revoked on %s (%s)", who, subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
		case esig.StatusSuspended:
			add(RuleCertRevoked, rules.Error, "%sthe signing certificate (%s) is suspended since %s (%s)", who, subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
		case esig.StatusUnknown:
			msg := "no CRL or OCSP response given covers the signing certificate: its revocation is not checked"
			for _, e := range res.Revocation.Errors {
				msg += "; " + e.Error()
			}
			add(RuleRevocationUnknown, rules.Info, "%s%s", who, msg)
		}
		if pa && !s.Qualified {
			add(RuleNotQualified, rules.Warning, "%sthe signing certificate (%s) does not declare itself an EU qualified certificate, which the SdI requires for an invoice to a public administration", who, subject)
		}
		if s.Kind == KindSeal {
			sev := rules.Info
			if pa {
				sev = rules.Warning
			}
			add(RuleSeal, sev, "%sthe certificate (%s) is issued for electronic seals: a seal was applied, not a signature", who, subject)
		}
		if len(s.Baseline) > 0 {
			add(RuleNotBaselineB, rules.Warning, "%sthe signature is not %s Baseline B: %s", who, sig.Format, strings.Join(s.Baseline, "; "))
		}
	}
	return r
}

// isFPA12 reports whether doc is a FatturaPA document for a public
// administration (versione FPA12).
func isFPA12(doc []byte) bool {
	root, err := xmltree.Parse(doc)
	if err != nil || checkRoot(root) != nil {
		return false
	}
	return strings.TrimSpace(root.Attr("versione")) == "FPA12"
}
