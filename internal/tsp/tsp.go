// Package tsp reads and verifies RFC 3161 time-stamp tokens: the token of a
// signature time-stamp (CAdES and PAdES Baseline T), which covers a
// signature value, and the token of a PDF document time-stamp (PAdES
// Baseline LTA), which covers the bytes of a revision. The token is a CMS
// SignedData whose content is a TSTInfo: the digest it time-stamps and the
// time the authority gives it. Nothing is fetched: the caller obtains the
// token from its time-stamping authority.
package tsp

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/sigalg"
)

// Token is a parsed time-stamp token.
type Token struct {
	// DER is the TimeStampToken, a CMS ContentInfo.
	DER []byte
	// SignedData is the CMS structure of the token.
	SignedData *cms.SignedData
	// Hash and Digest are the message imprint: the hash function and the
	// digest of the time-stamped data.
	Hash   crypto.Hash
	Digest []byte
	// Time is the genTime of the TSTInfo, the time the authority gives.
	Time time.Time
	// Policy is the policy of the authority, and Serial the serial number
	// of the token.
	Policy asn1.ObjectIdentifier
	Serial *big.Int
}

// Certificate returns the certificate of the authority that signed the
// token, nil when the token does not carry it.
func (t *Token) Certificate() *x509.Certificate {
	if t == nil || t.SignedData == nil || len(t.SignedData.Signers) == 0 {
		return nil
	}
	return t.SignedData.Signers[0].Certificate
}

// Parse reads a TimeStampToken, or the TimeStampResp of RFC 3161 §2.4.2
// that carries one (with the status granted or grantedWithMods), in DER.
func Parse(der []byte) (*Token, error) {
	der, err := unwrapResponse(der)
	if err != nil {
		return nil, err
	}
	sd, err := cms.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("tsp: %w", err)
	}
	if !sd.ContentType.Equal(cms.OIDTSTInfo) {
		return nil, fmt.Errorf("tsp: the content type %v is not id-ct-TSTInfo", sd.ContentType)
	}
	if sd.Detached {
		return nil, errors.New("tsp: the token does not hold its TSTInfo")
	}
	if len(sd.Signers) != 1 {
		return nil, fmt.Errorf("tsp: the token has %d SignerInfos instead of one", len(sd.Signers))
	}
	t := &Token{DER: der, SignedData: sd}
	if err := t.readInfo(sd.Content); err != nil {
		return nil, err
	}
	return t, nil
}

// unwrapResponse returns the token of a TimeStampResp, or der itself when
// it is a token: a ContentInfo starts with an object identifier, a
// TimeStampResp with the PKIStatusInfo SEQUENCE.
func unwrapResponse(der []byte) ([]byte, error) {
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(der, &outer); err != nil || outer.Tag != asn1.TagSequence {
		return nil, errors.New("tsp: the token is not a DER SEQUENCE")
	}
	var first asn1.RawValue
	rest, err := asn1.Unmarshal(outer.Bytes, &first)
	if err != nil {
		return nil, fmt.Errorf("tsp: %w", err)
	}
	if first.Tag != asn1.TagSequence {
		return der, nil
	}
	var status struct {
		Status int
		Rest   asn1.RawContent `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(first.FullBytes, &status); err != nil {
		return nil, errors.New("tsp: the PKIStatusInfo of the response cannot be read")
	}
	if status.Status != 0 && status.Status != 1 {
		return nil, fmt.Errorf("tsp: the time-stamp request was not granted (status %d)", status.Status)
	}
	var token asn1.RawValue
	if _, err := asn1.Unmarshal(rest, &token); err != nil {
		return nil, errors.New("tsp: the response holds no time-stamp token")
	}
	return token.FullBytes, nil
}

// readInfo reads the TSTInfo (RFC 3161 §2.4.2): version, policy,
// messageImprint, serialNumber and genTime, which come first; the optional
// fields after them are not needed.
func (t *Token) readInfo(der []byte) error {
	bad := func(what string) error { return fmt.Errorf("tsp: TSTInfo: %s cannot be read", what) }
	var seq asn1.RawValue
	if rest, err := asn1.Unmarshal(der, &seq); err != nil || len(rest) != 0 || seq.Tag != asn1.TagSequence {
		return bad("the structure")
	}
	b := seq.Bytes
	var version int
	var err error
	if b, err = asn1.Unmarshal(b, &version); err != nil || version != 1 {
		return bad("the version")
	}
	if b, err = asn1.Unmarshal(b, &t.Policy); err != nil {
		return bad("the policy")
	}
	var imprint struct {
		Algorithm pkix.AlgorithmIdentifier
		Digest    []byte
	}
	if b, err = asn1.Unmarshal(b, &imprint); err != nil {
		return bad("the message imprint")
	}
	h, ok := sigalg.HashFromOID(imprint.Algorithm.Algorithm)
	if !ok {
		return fmt.Errorf("tsp: TSTInfo: hash algorithm %v: %w", imprint.Algorithm.Algorithm, sigalg.ErrUnsupported)
	}
	t.Hash, t.Digest = h, imprint.Digest
	if b, err = asn1.Unmarshal(b, &t.Serial); err != nil {
		return bad("the serial number")
	}
	if _, err = asn1.UnmarshalWithParams(b, &t.Time, "generalized"); err != nil {
		return bad("the time")
	}
	return nil
}

// Errors of Verify.
var (
	// ErrImprint: the token time-stamps other data.
	ErrImprint = errors.New("tsp: the time-stamp token does not cover the data")
	// ErrSignature: the CMS signature of the token does not verify.
	ErrSignature = errors.New("tsp: the signature of the time-stamp token is not valid")
	// ErrNotTSA: the certificate of the token is not issued for
	// time-stamping.
	ErrNotTSA = errors.New("tsp: the certificate of the time-stamp token is not issued for time-stamping")
)

// Verify checks that the token time-stamps data (its message imprint is
// the digest of data), that its CMS signature verifies, and that the
// certificate of the authority has the time-stamping extended key usage
// (RFC 3161 §2.3). Trust in that certificate is left to the caller. SHA-1
// is refused as the hash of the imprint.
func (t *Token) Verify(data []byte) error {
	if t.Hash == crypto.SHA1 || !t.Hash.Available() {
		return fmt.Errorf("%w: hash %v of the imprint not admitted", ErrImprint, t.Hash)
	}
	if !bytes.Equal(sigalg.Digest(t.Hash, data), t.Digest) {
		return ErrImprint
	}
	if err := t.SignedData.VerifyAll(nil); err != nil {
		return fmt.Errorf("%w: %v", ErrSignature, err)
	}
	cert := t.Certificate()
	tsa := false
	for _, u := range cert.ExtKeyUsage {
		tsa = tsa || u == x509.ExtKeyUsageTimeStamping
	}
	if !tsa {
		return ErrNotTSA
	}
	return nil
}
