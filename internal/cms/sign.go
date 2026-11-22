package cms

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ritoon/zerechnung/internal/sigalg"
)

// SignOptions configure Sign.
type SignOptions struct {
	// Signer holds the private key; it may stay in a smart card, an HSM or
	// a remote signing service, since only a digest is signed.
	Signer crypto.Signer
	// Chain is the signing certificate followed by the certificates that
	// lead to a trusted root, which are written in the certificates field.
	Chain []*x509.Certificate
	// Hash is the digest algorithm; SHA-256 when zero.
	Hash crypto.Hash
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
	// SigningTime is written in the signing-time attribute, in UTC to the
	// second; the attribute is left out when it is zero (PAdES puts the
	// time in the signature dictionary instead).
	SigningTime time.Time
	// MimeType, when set, is written in the mime-type attribute (ETSI
	// EN 319 122-1 §5.2.4.2).
	MimeType string
	// Detached leaves the content out of the SignedData.
	Detached bool
	// ContentType is the type of the content, written in the
	// encapsulated content info and the content-type attribute; id-data
	// when nil (id-ct-TSTInfo for a time-stamp token).
	ContentType asn1.ObjectIdentifier
	// SignatureTimestamp, when set, is called with the signature value
	// and returns a time-stamp token over it (RFC 3161), which is written
	// in the signature-time-stamp unsigned attribute (CAdES and PAdES
	// Baseline T, ETSI EN 319 122-1 §5.3).
	SignatureTimestamp func(signature []byte) ([]byte, error)
}

// Sign returns a ContentInfo holding a SignedData that signs content, DER
// encoded: a CAdES Baseline B signature (ETSI EN 319 122-1 §6.3) when
// SigningTime is set.
func Sign(content []byte, o *SignOptions) ([]byte, error) {
	if o == nil || o.Signer == nil {
		return nil, errors.New("cms: no signer")
	}
	if len(o.Chain) == 0 {
		return nil, errors.New("cms: no signing certificate")
	}
	cert := o.Chain[0]
	if !samePublicKey(cert.PublicKey, o.Signer.Public()) {
		return nil, errors.New("cms: the signing certificate does not match the key of the signer")
	}
	alg, err := sigalg.For(cert.PublicKey, o.Hash, o.PSS)
	if err != nil {
		return nil, fmt.Errorf("cms: %w", err)
	}
	hashOID, _ := sigalg.OIDFromHash(alg.Hash)
	digestAI := mustMarshal(pkix.AlgorithmIdentifier{Algorithm: hashOID})
	contentType := o.ContentType
	if contentType == nil {
		contentType = OIDData
	}

	// Signed attributes, sorted by their encoding as DER sorts a SET OF.
	attrs := [][]byte{
		attr(OIDAttrContentType, mustMarshal(contentType)),
		attr(OIDAttrMessageDigest, mustMarshal(sigalg.Digest(alg.Hash, content))),
		attr(OIDAttrSigningCertificateV2, signingCertV2(cert, alg.Hash)),
	}
	if !o.SigningTime.IsZero() {
		attrs = append(attrs, attr(OIDAttrSigningTime, mustMarshal(o.SigningTime.UTC().Truncate(time.Second))))
	}
	if o.MimeType != "" {
		v, err := asn1.MarshalWithParams(o.MimeType, "utf8")
		if err != nil {
			return nil, fmt.Errorf("cms: mime type: %w", err)
		}
		attrs = append(attrs, attr(OIDAttrMimeType, v))
	}
	signedAttrs := tlv(0x31, sortedSet(attrs)...)
	sig, err := sigalg.Sign(o.Signer, alg, sigalg.Digest(alg.Hash, signedAttrs))
	if err != nil {
		return nil, fmt.Errorf("cms: signing: %w", err)
	}
	sigAI, err := alg.AlgorithmIdentifier()
	if err != nil {
		return nil, fmt.Errorf("cms: %w", err)
	}
	implicitAttrs := append([]byte{0xa0}, signedAttrs[1:]...)

	fields := [][]byte{
		mustMarshal(1),
		tlv(0x30, cert.RawIssuer, mustMarshal(cert.SerialNumber)),
		digestAI,
		implicitAttrs,
		mustMarshal(sigAI),
		mustMarshal(sig),
	}
	if o.SignatureTimestamp != nil {
		token, err := o.SignatureTimestamp(sig)
		if err != nil {
			return nil, fmt.Errorf("cms: signature time-stamp: %w", err)
		}
		var v asn1.RawValue
		if rest, err := asn1.Unmarshal(token, &v); err != nil || len(rest) != 0 || v.Tag != asn1.TagSequence {
			return nil, errors.New("cms: the signature time-stamp token is not a DER SEQUENCE")
		}
		fields = append(fields, tlv(0xa1, attr(OIDAttrTimestamp, token)))
	}
	signerInfo := tlv(0x30, fields...)

	var certs [][]byte
	seen := map[string]bool{}
	for _, c := range o.Chain {
		if !seen[string(c.Raw)] {
			seen[string(c.Raw)] = true
			certs = append(certs, c.Raw)
		}
	}
	encap := [][]byte{mustMarshal(contentType)}
	if !o.Detached {
		encap = append(encap, tlv(0xa0, mustMarshal(content)))
	}
	signedData := tlv(0x30,
		mustMarshal(1),
		tlv(0x31, digestAI),
		tlv(0x30, encap...),
		tlv(0xa0, sortedSet(certs)...),
		tlv(0x31, signerInfo),
	)
	return tlv(0x30, mustMarshal(OIDSignedData), tlv(0xa0, signedData)), nil
}

// signingCertV2 returns a SigningCertificateV2 that holds the hash of the
// signing certificate, without issuerSerial (ETSI EN 319 122-1 §6.3 g).
func signingCertV2(cert *x509.Certificate, h crypto.Hash) []byte {
	var id [][]byte
	if h != crypto.SHA256 { // the default value is left out in DER
		oid, _ := sigalg.OIDFromHash(h)
		id = append(id, mustMarshal(pkix.AlgorithmIdentifier{Algorithm: oid}))
	}
	id = append(id, mustMarshal(sigalg.Digest(h, cert.Raw)))
	return tlv(0x30, tlv(0x30, tlv(0x30, id...)))
}

func attr(oid asn1.ObjectIdentifier, value []byte) []byte {
	return tlv(0x30, mustMarshal(oid), tlv(0x31, value))
}

// sortedSet sorts the encodings of the elements of a SET OF, as DER
// requires.
func sortedSet(elems [][]byte) [][]byte {
	out := append([][]byte(nil), elems...)
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i], out[j]) < 0 })
	return out
}

func tlv(tag byte, content ...[]byte) []byte {
	n := 0
	for _, c := range content {
		n += len(c)
	}
	out := appendLength([]byte{tag}, n)
	for _, c := range content {
		out = append(out, c...)
	}
	return out
}

func mustMarshal(v any) []byte {
	b, err := asn1.Marshal(v)
	if err != nil {
		panic("cms: " + err.Error())
	}
	return b
}

func samePublicKey(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	if e, ok := a.(equaler); ok {
		return e.Equal(b)
	}
	return false
}
