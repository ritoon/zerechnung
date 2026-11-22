// Package cms writes and reads CMS SignedData structures (RFC 5652) in the
// form CAdES Baseline B gives them (ETSI EN 319 122-1 §6.3): the signed
// attributes content-type, message-digest, signing-time, signing-certificate-v2
// and, optionally, mime-type; the signing certificate and its chain in the
// certificates field; a SignerInfo identified by issuer and serial number.
//
// The content is attached (the .p7m envelope of a FatturaPA file) or detached
// (the signature of a PDF, PAdES). Parse accepts DER and BER encodings,
// indefinite lengths and chunked contents included, as signing tools produce
// them, and Verify checks each SignerInfo against the content: message
// digest, content type, signature value and signing certificate attribute.
// Trust in the certificate (chain, validity, revocation) is left to the
// caller.
package cms

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

// MaxSize bounds the length of the structures Parse reads, MaxSigners their
// SignerInfos and MaxCertificates the entries of their certificates field:
// each SignerInfo digests the whole content again and each certificate may
// be tried as an issuer, so an unbounded count would let a small file keep
// a verifier busy for hours.
const (
	MaxSize         = 64 << 20
	MaxSigners      = 16
	MaxCertificates = 64
)

// Object identifiers of the content types and attributes.
var (
	OIDData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	OIDSignedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}

	OIDAttrContentType          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	OIDAttrMessageDigest        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	OIDAttrSigningTime          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	OIDAttrSigningCertificate   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 12}
	OIDAttrSigningCertificateV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	OIDAttrMimeType             = asn1.ObjectIdentifier{0, 4, 0, 1733, 2, 1}
	// OIDAttrTimestamp is the signature-time-stamp unsigned attribute
	// (id-aa-signatureTimeStampToken, RFC 3161 appendix A).
	OIDAttrTimestamp = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
	// OIDTSTInfo is the content type of a time-stamp token (id-ct-TSTInfo).
	OIDTSTInfo = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}
)

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type signedData struct {
	Version          int
	DigestAlgorithms asn1.RawValue `asn1:"set"`
	EncapContentInfo encapContentInfo
	Certificates     asn1.RawValue `asn1:"optional,tag:0"`
	CRLs             asn1.RawValue `asn1:"optional,tag:1"`
	SignerInfos      asn1.RawValue `asn1:"set"`
}

type encapContentInfo struct {
	EContentType asn1.ObjectIdentifier
	EContent     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type signerInfo struct {
	Version            int
	SID                asn1.RawValue
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttrs        asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
	UnsignedAttrs      asn1.RawValue `asn1:"optional,tag:1"`
}

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

// SignedData is a parsed CMS SignedData.
type SignedData struct {
	// ContentType is the type of the encapsulated content (id-data for a
	// signed file).
	ContentType asn1.ObjectIdentifier
	// Content is the encapsulated content, nil when the signature is
	// detached.
	Content []byte
	// Detached tells that the content is not encapsulated.
	Detached bool
	// Certificates are the certificates of the certificates field that can
	// be read, in their order.
	Certificates []*x509.Certificate
	// Signers are the SignerInfos, in their order.
	Signers []*SignerInfo
}

// SignerInfo is a parsed SignerInfo.
type SignerInfo struct {
	// Certificate is the certificate the SignerInfo identifies, among the
	// certificates of the SignedData; nil when it is not there.
	Certificate *x509.Certificate
	// Algorithm is the signature algorithm, with the digest algorithm.
	Algorithm sigalg.Algorithm
	// AlgorithmErr tells why the algorithm cannot be used, when it cannot.
	AlgorithmErr error

	// HasSignedAttrs tells that the SignerInfo has signed attributes.
	HasSignedAttrs bool
	// ContentType and MessageDigest are the values of the content-type and
	// message-digest attributes.
	ContentType   asn1.ObjectIdentifier
	MessageDigest []byte
	// SigningTime is the value of the signing-time attribute, zero when it
	// is absent.
	SigningTime time.Time
	// MimeType is the value of the mime-type attribute.
	MimeType string
	// SigningCert is the reference of the signing-certificate-v2 attribute,
	// or of the older signing-certificate one, to the signing certificate;
	// nil when neither is present.
	SigningCert *CertRef
	// AttrErr tells why the signed attributes cannot be read, when they
	// cannot.
	AttrErr error
	// Timestamps are the values of the signature-time-stamp unsigned
	// attributes: time-stamp tokens over the signature value (RFC 3161).
	Timestamps [][]byte

	sid       asn1.RawValue
	attrsDER  []byte // the signed attributes, with the SET tag they are signed with
	signature []byte
}

// CertRef is the reference to the signing certificate in a signed attribute.
type CertRef struct {
	V2     bool        // signing-certificate-v2 (or else signing-certificate)
	Hash   crypto.Hash // hash function of Digest
	Digest []byte      // hash of the DER certificate
}

// Matches reports whether the reference designates cert.
func (r *CertRef) Matches(cert *x509.Certificate) bool {
	if r == nil || cert == nil || !r.Hash.Available() {
		return false
	}
	d := sigalg.Digest(r.Hash, cert.Raw)
	return string(d) == string(r.Digest)
}

// Parse reads a CMS ContentInfo that holds a SignedData, in DER or BER.
func Parse(data []byte) (*SignedData, error) {
	if len(data) > MaxSize {
		return nil, fmt.Errorf("cms: structure larger than %d bytes", MaxSize)
	}
	sd, err := parse(data)
	if err == nil {
		return sd, nil
	}
	der, nerr := normalize(data)
	if nerr != nil {
		return nil, err
	}
	return parse(der)
}

func parse(data []byte) (*SignedData, error) {
	var ci contentInfo
	rest, err := asn1.Unmarshal(data, &ci)
	if err != nil {
		return nil, fmt.Errorf("cms: %w", err)
	}
	if len(trimZeros(rest)) != 0 {
		return nil, errors.New("cms: trailing data after the ContentInfo")
	}
	if !ci.ContentType.Equal(OIDSignedData) {
		return nil, fmt.Errorf("cms: content type %v is not signed data", ci.ContentType)
	}
	var raw signedData
	if rest, err := asn1.Unmarshal(ci.Content.Bytes, &raw); err != nil || len(rest) != 0 {
		if err == nil {
			err = errors.New("trailing data")
		}
		return nil, fmt.Errorf("cms: SignedData: %w", err)
	}
	sd := &SignedData{ContentType: raw.EncapContentInfo.EContentType}
	if len(raw.EncapContentInfo.EContent.FullBytes) == 0 {
		sd.Detached = true
	} else {
		var content []byte
		if rest, err := asn1.Unmarshal(raw.EncapContentInfo.EContent.Bytes, &content); err != nil || len(rest) != 0 {
			return nil, errors.New("cms: the encapsulated content is not an OCTET STRING")
		}
		sd.Content = content
		if sd.Content == nil {
			sd.Content = []byte{}
		}
	}
	certs := raw.Certificates.Bytes
	for n := 0; len(certs) > 0; n++ {
		if n == MaxCertificates {
			return nil, fmt.Errorf("cms: more than %d certificates", MaxCertificates)
		}
		var c asn1.RawValue
		var err error
		if certs, err = asn1.Unmarshal(certs, &c); err != nil {
			return nil, fmt.Errorf("cms: certificates: %w", err)
		}
		if c.Class != asn1.ClassUniversal || c.Tag != asn1.TagSequence {
			continue // attribute certificates and other formats
		}
		if cert, err := x509.ParseCertificate(c.FullBytes); err == nil {
			sd.Certificates = append(sd.Certificates, cert)
		}
	}
	infos := raw.SignerInfos.Bytes
	for len(infos) > 0 {
		if len(sd.Signers) == MaxSigners {
			return nil, fmt.Errorf("cms: more than %d SignerInfos", MaxSigners)
		}
		var si signerInfo
		var err error
		if infos, err = asn1.Unmarshal(infos, &si); err != nil {
			return nil, fmt.Errorf("cms: SignerInfo: %w", err)
		}
		sd.Signers = append(sd.Signers, sd.newSigner(&si))
	}
	if len(sd.Signers) == 0 {
		return nil, errors.New("cms: no SignerInfo")
	}
	return sd, nil
}

// trimZeros drops the zero bytes some tools pad a file with.
func trimZeros(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return b
}

func (sd *SignedData) newSigner(si *signerInfo) *SignerInfo {
	s := &SignerInfo{sid: si.SID, signature: si.Signature}
	digest, ok := sigalg.HashFromOID(si.DigestAlgorithm.Algorithm)
	if !ok {
		s.AlgorithmErr = fmt.Errorf("%w: digest algorithm %v", sigalg.ErrUnsupported, si.DigestAlgorithm.Algorithm)
	} else {
		s.Algorithm, s.AlgorithmErr = sigalg.FromAlgorithmIdentifier(si.SignatureAlgorithm, digest)
	}
	s.Certificate = sd.findCert(si.SID)
	if len(si.SignedAttrs.FullBytes) > 0 {
		s.HasSignedAttrs = true
		s.attrsDER = append([]byte(nil), si.SignedAttrs.FullBytes...)
		s.attrsDER[0] = 0x31 // [0] IMPLICIT becomes the SET OF tag that is signed
		s.AttrErr = s.readAttrs(si.SignedAttrs.Bytes)
	}
	s.Timestamps = unsignedTimestamps(si.UnsignedAttrs.Bytes)
	return s
}

// SignatureValue returns the signature value of the SignerInfo, which a
// signature time-stamp covers.
func (s *SignerInfo) SignatureValue() []byte { return s.signature }

// unsignedTimestamps returns the values of the signature-time-stamp
// attributes among the unsigned attributes b; those that cannot be read
// are skipped.
func unsignedTimestamps(b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 && len(out) < MaxSigners {
		var a attribute
		var err error
		if b, err = asn1.Unmarshal(b, &a); err != nil {
			return out
		}
		if !a.Type.Equal(OIDAttrTimestamp) {
			continue
		}
		for vals := a.Values.Bytes; len(vals) > 0; {
			var v asn1.RawValue
			if vals, err = asn1.Unmarshal(vals, &v); err != nil {
				break
			}
			out = append(out, v.FullBytes)
		}
	}
	return out
}

// findCert returns the certificate a SignerIdentifier designates.
func (sd *SignedData) findCert(sid asn1.RawValue) *x509.Certificate {
	switch {
	case sid.Class == asn1.ClassUniversal && sid.Tag == asn1.TagSequence:
		var ias issuerAndSerial
		if _, err := asn1.Unmarshal(sid.FullBytes, &ias); err != nil || ias.Serial == nil {
			return nil
		}
		for _, c := range sd.Certificates {
			if c.SerialNumber.Cmp(ias.Serial) == 0 && string(c.RawIssuer) == string(ias.Issuer.FullBytes) {
				return c
			}
		}
	case sid.Class == asn1.ClassContextSpecific && sid.Tag == 0:
		for _, c := range sd.Certificates {
			if len(c.SubjectKeyId) > 0 && string(c.SubjectKeyId) == string(sid.Bytes) {
				return c
			}
		}
	}
	return nil
}

func (s *SignerInfo) readAttrs(b []byte) error {
	seen := map[string]bool{}
	for len(b) > 0 {
		var a attribute
		var err error
		if b, err = asn1.Unmarshal(b, &a); err != nil {
			return fmt.Errorf("cms: signed attributes: %w", err)
		}
		key := a.Type.String()
		if seen[key] {
			return fmt.Errorf("cms: signed attribute %v repeated", a.Type)
		}
		seen[key] = true
		var v asn1.RawValue
		rest, err := asn1.Unmarshal(a.Values.Bytes, &v)
		if err != nil || len(rest) != 0 {
			return fmt.Errorf("cms: signed attribute %v must have exactly one value", a.Type)
		}
		switch {
		case a.Type.Equal(OIDAttrContentType):
			if _, err := asn1.Unmarshal(v.FullBytes, &s.ContentType); err != nil {
				return fmt.Errorf("cms: content-type attribute: %w", err)
			}
		case a.Type.Equal(OIDAttrMessageDigest):
			if _, err := asn1.Unmarshal(v.FullBytes, &s.MessageDigest); err != nil {
				return fmt.Errorf("cms: message-digest attribute: %w", err)
			}
		case a.Type.Equal(OIDAttrSigningTime):
			if _, err := asn1.Unmarshal(v.FullBytes, &s.SigningTime); err != nil {
				return fmt.Errorf("cms: signing-time attribute: %w", err)
			}
		case a.Type.Equal(OIDAttrMimeType):
			if _, err := asn1.UnmarshalWithParams(v.FullBytes, &s.MimeType, "utf8"); err != nil {
				return fmt.Errorf("cms: mime-type attribute: %w", err)
			}
		case a.Type.Equal(OIDAttrSigningCertificateV2):
			ref, err := parseSigningCertV2(v.FullBytes)
			if err != nil {
				return err
			}
			s.SigningCert = ref
		case a.Type.Equal(OIDAttrSigningCertificate):
			if s.SigningCert != nil && s.SigningCert.V2 {
				continue
			}
			ref, err := parseSigningCertV1(v.FullBytes)
			if err != nil {
				return err
			}
			s.SigningCert = ref
		}
	}
	return nil
}

// parseSigningCertV2 reads the first ESSCertIDv2 of a SigningCertificateV2
// (RFC 5035), the one of the signing certificate.
func parseSigningCertV2(b []byte) (*CertRef, error) {
	bad := errors.New("cms: signing-certificate-v2 attribute cannot be read")
	var outer struct {
		Certs    []asn1.RawValue
		Policies asn1.RawValue `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(b, &outer); err != nil || len(outer.Certs) == 0 {
		return nil, bad
	}
	// ESSCertIDv2 ::= SEQUENCE { hashAlgorithm DEFAULT sha256, certHash, issuerSerial OPTIONAL }
	inner := outer.Certs[0].Bytes
	ref := &CertRef{V2: true, Hash: crypto.SHA256}
	var first asn1.RawValue
	rest, err := asn1.Unmarshal(inner, &first)
	if err != nil {
		return nil, bad
	}
	if first.Class == asn1.ClassUniversal && first.Tag == asn1.TagSequence {
		var ai pkix.AlgorithmIdentifier
		if _, err := asn1.Unmarshal(first.FullBytes, &ai); err != nil {
			return nil, bad
		}
		h, ok := sigalg.HashFromOID(ai.Algorithm)
		if !ok {
			return nil, fmt.Errorf("cms: signing-certificate-v2 with hash %v: %w", ai.Algorithm, sigalg.ErrUnsupported)
		}
		ref.Hash = h
		if _, err := asn1.Unmarshal(rest, &ref.Digest); err != nil {
			return nil, bad
		}
	} else if _, err := asn1.Unmarshal(first.FullBytes, &ref.Digest); err != nil {
		return nil, bad
	}
	return ref, nil
}

// parseSigningCertV1 reads the first ESSCertID of a SigningCertificate
// (RFC 2634), a SHA-1 hash.
func parseSigningCertV1(b []byte) (*CertRef, error) {
	var outer struct {
		Certs    []asn1.RawValue
		Policies asn1.RawValue `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(b, &outer); err != nil || len(outer.Certs) == 0 {
		return nil, errors.New("cms: signing-certificate attribute cannot be read")
	}
	ref := &CertRef{Hash: crypto.SHA1}
	if _, err := asn1.Unmarshal(outer.Certs[0].Bytes, &ref.Digest); err != nil {
		return nil, errors.New("cms: signing-certificate attribute cannot be read")
	}
	return ref, nil
}
