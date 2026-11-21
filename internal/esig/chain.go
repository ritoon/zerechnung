package esig

import (
	"crypto/x509"
	"errors"
	"fmt"
	"time"
)

// Options configure Check.
type Options struct {
	// Roots are the trusted certification authorities. When nil, the chain
	// is not checked (Result.TrustChecked is false).
	Roots *x509.CertPool
	// Intermediates are certificates that may complete the chain, besides
	// those the signature carries.
	Intermediates []*x509.Certificate
	// At is the time at which the certificate must be valid and not
	// revoked.
	At time.Time
	// CRLs and OCSP are revocation data the caller obtained: CRLs parsed
	// by x509.ParseRevocationList, OCSP responses in DER.
	CRLs []*x509.RevocationList
	OCSP [][]byte
}

// Result is the outcome of Check.
type Result struct {
	// Expired: the certificate has expired at Options.At.
	Expired bool
	// NotYetValid: the certificate is not valid yet at Options.At.
	NotYetValid bool
	// KeyUsage tells why the key usage of the certificate does not allow
	// signing, when it does not.
	KeyUsage error
	// TrustChecked: Options.Roots was given and the chain was built.
	TrustChecked bool
	// Untrusted: no chain leads to Options.Roots.
	Untrusted bool
	// ChainErr is another reason why the chain is not valid (an expired or
	// invalid authority, a forbidden name...).
	ChainErr error
	// Chain is the verified chain, from the certificate to a root, when
	// one was found.
	Chain []*x509.Certificate
	// Revocation is the revocation status found in the CRLs and OCSP
	// responses.
	Revocation Revocation
}

// Check checks cert, the signing certificate, at o.At: its validity period,
// its key usage, its chain to o.Roots (using intermediates, the other
// certificates of the signature, and o.Intermediates) and its revocation
// status.
func Check(cert *x509.Certificate, intermediates []*x509.Certificate, o *Options) Result {
	var r Result
	at := o.At
	switch {
	case at.After(cert.NotAfter):
		r.Expired = true
	case at.Before(cert.NotBefore):
		r.NotYetValid = true
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0 {
		r.KeyUsage = errors.New("the key usage of the certificate allows neither digitalSignature nor nonRepudiation")
	}
	pool := x509.NewCertPool()
	for _, c := range intermediates {
		pool.AddCert(c)
	}
	for _, c := range o.Intermediates {
		pool.AddCert(c)
	}
	if o.Roots != nil {
		r.TrustChecked = true
		// The chain is checked at a time when the certificate itself is
		// valid, so that an expired certificate still tells whether its
		// authority is trusted: its expiry is reported apart.
		vat := at
		if vat.After(cert.NotAfter) {
			vat = cert.NotAfter
		}
		if vat.Before(cert.NotBefore) {
			vat = cert.NotBefore
		}
		chains, err := cert.Verify(x509.VerifyOptions{
			Roots:         o.Roots,
			Intermediates: pool,
			CurrentTime:   vat,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		var ua x509.UnknownAuthorityError
		switch {
		case err == nil:
			r.Chain = chains[0]
		case errors.As(err, &ua):
			r.Untrusted = true
		default:
			r.ChainErr = err
		}
	}
	issuer := findIssuer(cert, r.Chain, intermediates, o.Intermediates)
	r.Revocation = revocation(cert, issuer, at, o)
	return r
}

// findIssuer returns the certificate that issued cert: the next one in the
// verified chain, or a certificate whose subject is the issuer of cert and
// whose key verifies its signature.
func findIssuer(cert *x509.Certificate, chain []*x509.Certificate, lists ...[]*x509.Certificate) *x509.Certificate {
	if len(chain) > 1 {
		return chain[1]
	}
	for _, l := range lists {
		for _, c := range l {
			if string(c.RawSubject) == string(cert.RawIssuer) && cert.CheckSignatureFrom(c) == nil {
				return c
			}
		}
	}
	return nil
}

// Status is a revocation status.
type Status int

// Revocation statuses.
const (
	StatusUnknown   Status = iota // no CRL or OCSP response covers the certificate
	StatusGood                    // a CRL or an OCSP response says the certificate is not revoked
	StatusSuspended               // on hold (CRL reason certificateHold)
	StatusRevoked                 // revoked
)

// Revocation is the revocation status of a certificate.
type Revocation struct {
	Status Status
	// Time is the revocation time, for a revoked or suspended certificate.
	Time time.Time
	// Source says which data gave the status ("CRL", "OCSP").
	Source string
	// Errors are the revocation data that could not be used, and why.
	Errors []error
}

func revocation(cert, issuer *x509.Certificate, at time.Time, o *Options) Revocation {
	var rev Revocation
	set := func(s Status, t time.Time, src string) {
		if s > rev.Status { // revoked beats suspended, which beats good, which beats unknown
			rev.Status, rev.Time, rev.Source = s, t, src
		}
	}
	for _, crl := range o.CRLs {
		if crl == nil || string(crl.RawIssuer) != string(cert.RawIssuer) {
			continue
		}
		if issuer == nil {
			rev.Errors = append(rev.Errors, errors.New("CRL: the issuer of the certificate is not known"))
			continue
		}
		if err := crl.CheckSignatureFrom(issuer); err != nil {
			rev.Errors = append(rev.Errors, fmt.Errorf("CRL: %w", err))
			continue
		}
		revoked := false
		for _, e := range crl.RevokedCertificateEntries {
			if e.SerialNumber == nil || e.SerialNumber.Cmp(cert.SerialNumber) != 0 || e.RevocationTime.After(at) {
				continue
			}
			revoked = true
			if e.ReasonCode == 6 { // certificateHold
				set(StatusSuspended, e.RevocationTime, "CRL")
			} else {
				set(StatusRevoked, e.RevocationTime, "CRL")
			}
		}
		// A CRL tells the status of an unrevoked certificate at any time up
		// to its date, and until its next update.
		if !revoked && (!crl.ThisUpdate.Before(at) || crl.NextUpdate.IsZero() || !at.After(crl.NextUpdate)) {
			set(StatusGood, time.Time{}, "CRL")
		}
	}
	for _, der := range o.OCSP {
		s, t, err := checkOCSP(der, cert, issuer, at)
		if err != nil {
			if !errors.Is(err, errOCSPOtherCert) {
				rev.Errors = append(rev.Errors, fmt.Errorf("OCSP: %w", err))
			}
			continue
		}
		set(s, t, "OCSP")
	}
	return rev
}
