package esig

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ritoon/zerechnung/internal/sigalg"
)

// OCSP response structures (RFC 6960 §4.2.1).

type ocspResponse struct {
	Status   asn1.Enumerated
	Response ocspResponseBytes `asn1:"explicit,optional,tag:0"`
}

type ocspResponseBytes struct {
	Type     asn1.ObjectIdentifier
	Response []byte
}

type basicOCSPResponse struct {
	TBS          asn1.RawValue
	SignatureAlg pkix.AlgorithmIdentifier
	Signature    asn1.BitString
	Certs        []asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type responseData struct {
	Raw        asn1.RawContent
	Version    int           `asn1:"optional,explicit,default:0,tag:0"`
	ByName     asn1.RawValue `asn1:"optional,explicit,tag:1"`
	ByKey      []byte        `asn1:"optional,explicit,tag:2"`
	ProducedAt time.Time     `asn1:"generalized"`
	Responses  []singleResponse
	Extensions asn1.RawValue `asn1:"optional,explicit,tag:1"`
}

type certID struct {
	HashAlg       pkix.AlgorithmIdentifier
	IssuerNameKey []byte
	IssuerKeyHash []byte
	Serial        *big.Int
}

type singleResponse struct {
	CertID     certID
	Good       asn1.Flag     `asn1:"optional,tag:0"`
	Revoked    revokedInfo   `asn1:"optional,tag:1"`
	Unknown    asn1.Flag     `asn1:"optional,tag:2"`
	ThisUpdate time.Time     `asn1:"generalized"`
	NextUpdate time.Time     `asn1:"generalized,explicit,tag:0,optional"`
	Extensions asn1.RawValue `asn1:"explicit,tag:1,optional"`
}

type revokedInfo struct {
	Time   time.Time       `asn1:"generalized"`
	Reason asn1.Enumerated `asn1:"explicit,tag:0,optional"`
}

var (
	oidOCSPBasic      = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1}
	errOCSPOtherCert  = errors.New("the response is about another certificate")
	errOCSPMalformed  = errors.New("malformed response")
	oidExtKeyOCSPSign = x509.ExtKeyUsageOCSPSigning
)

// checkOCSP reads an OCSP response and returns the status it gives for
// cert at the time at. The response must be signed by the issuer of cert,
// or by a responder that the issuer certified for OCSP signing.
func checkOCSP(der []byte, cert, issuer *x509.Certificate, at time.Time) (Status, time.Time, error) {
	var resp ocspResponse
	if rest, err := asn1.Unmarshal(der, &resp); err != nil || len(rest) != 0 {
		return 0, time.Time{}, errOCSPMalformed
	}
	if resp.Status != 0 {
		return 0, time.Time{}, fmt.Errorf("response status %d", resp.Status)
	}
	if !resp.Response.Type.Equal(oidOCSPBasic) {
		return 0, time.Time{}, fmt.Errorf("response type %v", resp.Response.Type)
	}
	var basic basicOCSPResponse
	if rest, err := asn1.Unmarshal(resp.Response.Response, &basic); err != nil || len(rest) != 0 {
		return 0, time.Time{}, errOCSPMalformed
	}
	var data responseData
	if rest, err := asn1.Unmarshal(basic.TBS.FullBytes, &data); err != nil || len(rest) != 0 {
		return 0, time.Time{}, errOCSPMalformed
	}
	if issuer == nil {
		return 0, time.Time{}, errors.New("the issuer of the certificate is not known")
	}
	// The single response about cert.
	var single *singleResponse
	for i := range data.Responses {
		if matchCertID(&data.Responses[i].CertID, cert, issuer) {
			single = &data.Responses[i]
			break
		}
	}
	if single == nil {
		return 0, time.Time{}, errOCSPOtherCert
	}
	// The responder: the issuer itself, or a certificate it issued for
	// OCSP signing.
	responder := issuer
	if !responderIs(&data, issuer) {
		responder = nil
		for _, raw := range basic.Certs {
			c, err := x509.ParseCertificate(raw.FullBytes)
			if err != nil || !responderIs(&data, c) {
				continue
			}
			if c.CheckSignatureFrom(issuer) != nil || !hasEKU(c, oidExtKeyOCSPSign) {
				return 0, time.Time{}, errors.New("the responder is not authorized by the issuer of the certificate")
			}
			if data.ProducedAt.Before(c.NotBefore) || data.ProducedAt.After(c.NotAfter) {
				return 0, time.Time{}, errors.New("the responder certificate is not valid when the response was produced")
			}
			responder = c
			break
		}
		if responder == nil {
			return 0, time.Time{}, errors.New("the responder certificate is missing")
		}
	}
	alg, err := sigalg.FromAlgorithmIdentifier(basic.SignatureAlg, 0)
	if err != nil {
		return 0, time.Time{}, err
	}
	if err := sigalg.Verify(responder.PublicKey, alg, sigalg.Digest(alg.Hash, basic.TBS.FullBytes), basic.Signature.RightAlign()); err != nil {
		return 0, time.Time{}, fmt.Errorf("signature: %w", err)
	}
	switch {
	case single.Revoked.Time != (time.Time{}) && !single.Revoked.Time.After(at):
		if single.Revoked.Reason == 6 { // certificateHold
			return StatusSuspended, single.Revoked.Time, nil
		}
		return StatusRevoked, single.Revoked.Time, nil
	case bool(single.Unknown):
		return StatusUnknown, time.Time{}, nil
	}
	// A good status, or a revocation after at, holds at any time up to
	// thisUpdate, and until nextUpdate. A responder need not keep the
	// status of expired certificates (RFC 6960 §4.4.4): a response produced
	// after cert expired proves nothing. The archive cutoff extension,
	// which would vouch for it, is not read.
	if single.ThisUpdate.After(cert.NotAfter) {
		return StatusUnknown, time.Time{}, nil
	}
	if !single.ThisUpdate.Before(at) || !single.NextUpdate.IsZero() && !at.After(single.NextUpdate) {
		return StatusGood, time.Time{}, nil
	}
	return StatusUnknown, time.Time{}, nil
}

func matchCertID(id *certID, cert, issuer *x509.Certificate) bool {
	if id.Serial == nil || id.Serial.Cmp(cert.SerialNumber) != 0 {
		return false
	}
	h, ok := sigalg.HashFromOID(id.HashAlg.Algorithm)
	if !ok {
		return false
	}
	key, err := publicKeyBits(issuer)
	if err != nil {
		return false
	}
	return string(sigalg.Digest(h, issuer.RawSubject)) == string(id.IssuerNameKey) &&
		string(sigalg.Digest(h, key)) == string(id.IssuerKeyHash)
}

// responderIs reports whether the ResponderID of the response designates c.
func responderIs(data *responseData, c *x509.Certificate) bool {
	if len(data.ByName.Bytes) > 0 {
		return string(data.ByName.Bytes) == string(c.RawSubject)
	}
	if len(data.ByKey) > 0 {
		key, err := publicKeyBits(c)
		if err != nil {
			return false
		}
		return string(sigalg.Digest(crypto.SHA1, key)) == string(data.ByKey) // RFC 6960 §4.2.1
	}
	return false
}

// publicKeyBits returns the subjectPublicKey bits of a certificate.
func publicKeyBits(c *x509.Certificate) ([]byte, error) {
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(c.RawSubjectPublicKeyInfo, &spki); err != nil {
		return nil, err
	}
	return spki.PublicKey.RightAlign(), nil
}

func hasEKU(c *x509.Certificate, eku x509.ExtKeyUsage) bool {
	for _, e := range c.ExtKeyUsage {
		if e == eku {
			return true
		}
	}
	return false
}
