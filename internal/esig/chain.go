package esig

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"slices"
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
			if e.ReasonCode == 8 { // removeFromCRL, in a delta CRL: no longer on hold
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
		// to its date, and until its next update, when it covers every
		// certificate of its issuer and still lists cert if revoked.
		if !revoked && crlCovers(crl, cert) &&
			(!crl.ThisUpdate.Before(at) || !crl.NextUpdate.IsZero() && !at.After(crl.NextUpdate)) {
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

var (
	oidDeltaCRLIndicator        = asn1.ObjectIdentifier{2, 5, 29, 27}
	oidIssuingDistributionPoint = asn1.ObjectIdentifier{2, 5, 29, 28}
	oidExpiredCertsOnCRL        = asn1.ObjectIdentifier{2, 5, 29, 60}
)

// issuingDistributionPoint is the extension of RFC 5280 §5.2.5.
type issuingDistributionPoint struct {
	DistributionPoint asn1.RawValue  `asn1:"optional,tag:0"`
	OnlyUserCerts     bool           `asn1:"optional,tag:1"`
	OnlyCACerts       bool           `asn1:"optional,tag:2"`
	OnlySomeReasons   asn1.BitString `asn1:"optional,tag:3"`
	IndirectCRL       bool           `asn1:"optional,tag:4"`
	OnlyAttrCerts     bool           `asn1:"optional,tag:5"`
}

// crlCovers reports whether the absence of cert from crl means that cert is
// not revoked: crl is a complete CRL, not a delta CRL, its scope (issuing
// distribution point) includes cert, and it was issued before cert expired
// or keeps listing expired certificates (RFC 5280 §3.3: a CA may drop an
// expired certificate from its CRLs).
func crlCovers(crl *x509.RevocationList, cert *x509.Certificate) bool {
	if crl.ThisUpdate.After(cert.NotAfter) && !hasExtension(crl, oidExpiredCertsOnCRL) {
		return false
	}
	for _, e := range crl.Extensions {
		switch {
		case e.Id.Equal(oidDeltaCRLIndicator):
			return false
		case e.Id.Equal(oidIssuingDistributionPoint):
			var idp issuingDistributionPoint
			if rest, err := asn1.Unmarshal(e.Value, &idp); err != nil || len(rest) != 0 {
				return false
			}
			if idp.OnlyCACerts || idp.OnlyAttrCerts || idp.IndirectCRL || idp.OnlySomeReasons.BitLength > 0 {
				return false
			}
			if len(idp.DistributionPoint.FullBytes) > 0 && !inDistributionPoint(idp.DistributionPoint.Bytes, cert) {
				return false
			}
		}
	}
	return true
}

// inDistributionPoint reports whether the DistributionPointName dp (the
// content of the [0] field of the extension) names one of the CRL
// distribution points of cert by URI.
func inDistributionPoint(dp []byte, cert *x509.Certificate) bool {
	var name asn1.RawValue
	if _, err := asn1.Unmarshal(dp, &name); err != nil || name.Class != asn1.ClassContextSpecific || name.Tag != 0 {
		return false // nameRelativeToCRLIssuer, or malformed
	}
	for rest := name.Bytes; len(rest) > 0; {
		var gn asn1.RawValue
		var err error
		if rest, err = asn1.Unmarshal(rest, &gn); err != nil {
			return false
		}
		if gn.Class == asn1.ClassContextSpecific && gn.Tag == 6 && slices.Contains(cert.CRLDistributionPoints, string(gn.Bytes)) {
			return true
		}
	}
	return false
}

func hasExtension(crl *x509.RevocationList, id asn1.ObjectIdentifier) bool {
	for _, e := range crl.Extensions {
		if e.Id.Equal(id) {
			return true
		}
	}
	return false
}
