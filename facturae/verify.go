package facturae

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/esig"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Report is the result of VerifySignature: the same type as
// validate.Report.
type Report = rules.Report

// VerifyOptions configure VerifySignature. The zero value checks the
// signature itself and the validity of the certificate now, but neither its
// chain nor its revocation.
type VerifyOptions struct {
	// Roots are the trusted certification authorities: for FACe, those of
	// the Spanish trusted list of qualified trust service providers (or the
	// EU trusted lists). Nothing is downloaded: without Roots, the chain is
	// not checked (ES-SIG-11).
	Roots *x509.CertPool
	// Intermediates are certificates that may complete the chain, besides
	// those the signature carries.
	Intermediates []*x509.Certificate
	// At is the time the invoice was received: the certificate must be
	// valid then, and the signing time must not be later. time.Now() when
	// zero.
	At time.Time
	// CRLs and OCSP are revocation data obtained by the caller (CRLs
	// parsed by x509.ParseRevocationList, OCSP responses in DER). The
	// revocation is only checked against them.
	CRLs []*x509.RevocationList
	OCSP [][]byte
}

// Rules of VerifySignature: the requirements of the signature policy of
// Facturae v3.1 on the basic advanced electronic signature (§1 and §4), and
// the checks of the certificate that its validation needs.
const (
	RuleNotSigned         = "ES-SIG-01" // the document is not signed
	RuleSignatureInvalid  = "ES-SIG-02" // the signature does not verify
	RulePolicy            = "ES-SIG-03" // not the signature policy of Facturae v3.1
	RuleSigningTime       = "ES-SIG-04" // no signing time, or later than the reception
	RuleRole              = "ES-SIG-05" // signer role not admitted by the policy
	RuleRoleIssuerType    = "ES-SIG-06" // signer role other than InvoiceIssuerType
	RuleKeyInfoNotSigned  = "ES-SIG-07" // ds:KeyInfo not signed
	RuleCertInvalid       = "ES-SIG-08" // signing certificate expired, not valid yet, or not for signing
	RuleUntrustedCA       = "ES-SIG-09" // chain not valid or not to a trusted authority
	RuleCertRevoked       = "ES-SIG-10" // signing certificate revoked or suspended
	RuleTrustNotChecked   = "ES-SIG-11" // no trusted roots given
	RuleRevocationUnknown = "ES-SIG-12" // no revocation data covers the certificate
	RuleNotQualified      = "ES-SIG-13" // the certificate does not declare itself qualified
	RuleSeal              = "ES-SIG-14" // an electronic seal, not a signature
	RuleNotFacturae       = "ES-SIG-15" // the signed document is not a Facturae document
)

// SignatureRules lists the rules of VerifySignature.
var SignatureRules = []rules.RuleInfo{
	{ID: RuleNotSigned, Severity: rules.Error, Description: "The Facturae document is not signed: FACe only receives signed invoices, and between businesses the invoices carry an advanced electronic signature (Real Decreto 238/2026, article 7.3). The validate package reports it under its FACe rule set only."},
	{ID: RuleSignatureInvalid, Severity: rules.Error, Description: "The signature is not valid: the document does not match what was signed, the signature is not a ds:Signature child of the root element that covers the whole document, or it cannot be read or uses an algorithm that is not admitted (SHA-1)."},
	{ID: RulePolicy, Severity: rules.Error, Description: "The signature is not XAdES-EPES under the signature policy of Facturae v3.1: xades:SignaturePolicyIdentifier is missing, or does not give the identifier of the policy (facturae.PolicyIdentifier) with its SHA-1 digest (facturae.PolicyDigest)."},
	{ID: RuleSigningTime, Severity: rules.Error, Description: "The signature has no xades:SigningTime, which the policy requires, or its signing time is later than the reception of the invoice."},
	{ID: RuleRole, Severity: rules.Error, Description: "xades:SignerRole does not hold one and only one xades:ClaimedRole among those the policy admits: supplier or emisor, customer or receptor, third party or tercero."},
	{ID: RuleRoleIssuerType, Severity: rules.Warning, Description: "The role of the signer is not the one that InvoiceIssuerType of the file header gives: emisor for EM, receptor for RE, tercero for TE."},
	{ID: RuleKeyInfoNotSigned, Severity: rules.Error, Description: "No reference of the signature covers ds:KeyInfo, which the policy requires so that the certificate cannot be replaced. A warning when xades:SigningCertificate (or SigningCertificateV2) binds the signing certificate."},
	{ID: RuleCertInvalid, Severity: rules.Error, Description: "The signing certificate is not valid at the reception of the invoice: expired, not valid yet, or with a key usage that does not allow signing."},
	{ID: RuleUntrustedCA, Severity: rules.Error, Description: "The chain of the signing certificate is not valid, or does not lead to one of the trusted certification authorities."},
	{ID: RuleCertRevoked, Severity: rules.Error, Description: "The signing certificate is revoked or suspended, according to the CRLs and OCSP responses given."},
	{ID: RuleTrustNotChecked, Severity: rules.Warning, Description: "No trusted certification authority was given: the chain of the signing certificate is not checked (ES-SIG-09)."},
	{ID: RuleRevocationUnknown, Severity: rules.Info, Description: "No CRL or OCSP response given covers the signing certificate: its revocation is not checked (ES-SIG-10)."},
	{ID: RuleNotQualified, Severity: rules.Warning, Description: "The signing certificate does not declare itself an EU qualified certificate (QCStatements of ETSI EN 319 412-5), as the recognized certificates that the policy (§4) and FACe require are."},
	{ID: RuleSeal, Severity: rules.Info, Description: "The certificate is issued for electronic seals (QcType eseal): a seal of a legal person was applied, not the signature of a natural person."},
	{ID: RuleNotFacturae, Severity: rules.Error, Description: "The signed document is not a Facturae 3.2, 3.2.1 or 3.2.2 document."},
}

// VerifySignature checks the signature of a Facturae document against the
// signature policy of Facturae v3.1 and reports each problem under an
// ES-SIG rule (see SignatureRules): its integrity (ES-SIG-02, see
// ReadSignature), the policy (ES-SIG-03), the signing time (ES-SIG-04), the
// role of the signer (ES-SIG-05, ES-SIG-06), the signature of ds:KeyInfo
// (ES-SIG-07), and the signing certificate at the reception of the invoice
// (validity ES-SIG-08, chain ES-SIG-09, revocation ES-SIG-10, qualified
// certificate ES-SIG-13, seal ES-SIG-14). What needs data this package does
// not fetch is reported as not checked: trust without VerifyOptions.Roots
// (ES-SIG-11), revocation without CRL or OCSP response (ES-SIG-12). An
// unsigned document is an error (ES-SIG-01).
func VerifySignature(doc []byte, opts *VerifyOptions) *Report {
	if opts == nil {
		opts = &VerifyOptions{}
	}
	at := opts.At
	if at.IsZero() {
		at = time.Now()
	}
	r := &Report{}
	add := func(rule string, sev rules.Severity, format string, args ...any) {
		r.Add(rules.Finding{Rule: rule, Severity: sev, Source: rules.SourceFacturae, Message: fmt.Sprintf(format, args...)})
	}

	sig, err := ReadSignature(doc)
	switch {
	case errors.Is(err, ErrNotSigned):
		add(RuleNotSigned, rules.Error, "the document is not signed")
		return r
	case err != nil:
		add(RuleSignatureInvalid, rules.Error, "%s", strings.TrimPrefix(err.Error(), "facturae: "))
		return r
	}
	root, perr := xmltree.Parse(doc)
	if perr != nil || checkRoot(root) != nil {
		add(RuleNotFacturae, rules.Error, "the signed document is not a Facturae 3.2, 3.2.1 or 3.2.2 document")
		root = nil
	}

	// The requirements of the policy.
	switch p := sig.Policy; {
	case p == nil:
		add(RulePolicy, rules.Error, "the signature has no xades:SignaturePolicyIdentifier: it is not XAdES-EPES under the signature policy of Facturae v3.1")
	case p.Identifier != PolicyIdentifier:
		add(RulePolicy, rules.Error, "the signature policy is %q, not that of Facturae v3.1 (%s)", p.Identifier, PolicyIdentifier)
	case !p.IsFacturae():
		got := base64.StdEncoding.EncodeToString(p.Digest)
		if p.Hash != 0 {
			got = p.Hash.String() + " " + got
		}
		add(RulePolicy, rules.Error, "the digest of the signature policy is %s, not the SHA-1 digest of the policy of Facturae v3.1 (%s)", got, PolicyDigest)
	}
	if sig.SigningTime.IsZero() {
		add(RuleSigningTime, rules.Error, "the signature has no xades:SigningTime")
	} else if sig.SigningTime.After(at) {
		add(RuleSigningTime, rules.Error, "the signing time %s is later than the reception of the invoice, %s", sig.SigningTime.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339))
	}
	switch {
	case len(sig.Roles) > 1:
		add(RuleRole, rules.Error, "xades:SignerRole holds %d roles (%s), the policy admits one", len(sig.Roles), strings.Join(sig.Roles, ", "))
	case len(sig.Roles) == 1 && !sig.Role.Valid():
		add(RuleRole, rules.Error, "the role of the signer %q is not one that the policy admits (emisor, receptor, tercero, or supplier, customer, third party)", sig.Roles[0])
	case len(sig.Roles) == 1 && root != nil:
		it := root.Value("FileHeader/InvoiceIssuerType")
		if want, ok := issuerTypes[it]; ok && want != sig.Role {
			add(RuleRoleIssuerType, rules.Warning, "the signer claims the role %s, but the invoice issuer type is %s (%s)", sig.Role, it, want)
		}
	}
	if !sig.KeyInfoSigned {
		if sig.SigningCertificate != "" {
			add(RuleKeyInfoNotSigned, rules.Warning, "no reference covers ds:KeyInfo, which the policy requires; xades:%s binds the signing certificate", sig.SigningCertificate)
		} else {
			add(RuleKeyInfoNotSigned, rules.Error, "no reference covers ds:KeyInfo, and no xades:SigningCertificate binds the signing certificate: it could be replaced by another certificate of the same key")
		}
	}

	// The signing certificate.
	cert := sig.Certificate
	subject := cert.Subject.String()
	res := esig.Check(cert, sig.Certificates, &esig.Options{
		Roots:         opts.Roots,
		Intermediates: opts.Intermediates,
		At:            at,
		CRLs:          opts.CRLs,
		OCSP:          opts.OCSP,
	})
	switch {
	case res.Expired:
		add(RuleCertInvalid, rules.Error, "the signing certificate (%s) expired on %s", subject, cert.NotAfter.UTC().Format(time.RFC3339))
	case res.NotYetValid:
		add(RuleCertInvalid, rules.Error, "the signing certificate (%s) is not valid before %s", subject, cert.NotBefore.UTC().Format(time.RFC3339))
	}
	if res.KeyUsage != nil {
		add(RuleCertInvalid, rules.Error, "%v", res.KeyUsage)
	}
	switch {
	case opts.Roots == nil:
		add(RuleTrustNotChecked, rules.Warning, "no trusted certification authority given: the chain of the signing certificate is not checked")
	case res.Untrusted:
		add(RuleUntrustedCA, rules.Error, "the signing certificate (%s, issued by %s) does not lead to a trusted certification authority", subject, cert.Issuer.String())
	case res.ChainErr != nil:
		add(RuleUntrustedCA, rules.Error, "the chain of the signing certificate is not valid: %v", res.ChainErr)
	}
	switch res.Revocation.Status {
	case esig.StatusRevoked:
		add(RuleCertRevoked, rules.Error, "the signing certificate (%s) was revoked on %s (%s)", subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
	case esig.StatusSuspended:
		add(RuleCertRevoked, rules.Error, "the signing certificate (%s) is suspended since %s (%s)", subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
	case esig.StatusUnknown:
		msg := "no CRL or OCSP response given covers the signing certificate: its revocation is not checked"
		for _, e := range res.Revocation.Errors {
			msg += "; " + e.Error()
		}
		add(RuleRevocationUnknown, rules.Info, "%s", msg)
	}
	if !sig.Qualified {
		add(RuleNotQualified, rules.Warning, "the signing certificate (%s) does not declare itself an EU qualified certificate, as the recognized certificates that FACe requires are", subject)
	}
	if sig.Kind == KindSeal {
		add(RuleSeal, rules.Info, "the certificate (%s) is issued for electronic seals: a seal was applied, not a signature", subject)
	}
	return r
}
