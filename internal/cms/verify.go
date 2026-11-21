package cms

import (
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/ritoon/zerechnung/internal/sigalg"
)

// Errors of Verify. Each error returned by Verify wraps one of them.
var (
	// ErrNoCertificate: the signing certificate is not in the SignedData.
	ErrNoCertificate = errors.New("cms: the signing certificate is not in the signature")
	// ErrDigest: the content does not match the message-digest attribute.
	ErrDigest = errors.New("cms: the content does not match the signed digest")
	// ErrSignature: the signature value does not verify, or cannot be
	// checked (malformed attributes, unsupported or weak algorithm).
	ErrSignature = errors.New("cms: the signature value is not valid")
	// ErrSigningCertificate: the signing-certificate attribute designates
	// another certificate.
	ErrSigningCertificate = errors.New("cms: the signing-certificate attribute does not match the signing certificate")
)

// Verify checks the SignerInfo against content, the encapsulated content
// or, for a detached signature, the signed data: the content-type and
// message-digest attributes, the signature value with the public key of the
// signing certificate, and the signing-certificate attribute when present.
// A SignerInfo without signed attributes is refused: CAdES requires them.
// SHA-1 is refused.
func (s *SignerInfo) Verify(sd *SignedData, content []byte) error {
	if s.AlgorithmErr != nil {
		return fmt.Errorf("%w: %v", ErrSignature, s.AlgorithmErr)
	}
	if s.Algorithm.Weak() {
		return fmt.Errorf("%w: %v is not admitted", ErrSignature, s.Algorithm)
	}
	if s.Certificate == nil {
		return ErrNoCertificate
	}
	if !s.HasSignedAttrs {
		return fmt.Errorf("%w: no signed attributes", ErrSignature)
	}
	if s.AttrErr != nil {
		return fmt.Errorf("%w: %v", ErrSignature, s.AttrErr)
	}
	if s.ContentType == nil || !s.ContentType.Equal(sd.ContentType) {
		return fmt.Errorf("%w: the content-type attribute is missing or differs from the content type", ErrSignature)
	}
	if s.MessageDigest == nil {
		return fmt.Errorf("%w: no message-digest attribute", ErrSignature)
	}
	if string(sigalg.Digest(s.Algorithm.Hash, content)) != string(s.MessageDigest) {
		return ErrDigest
	}
	if err := sigalg.Verify(s.Certificate.PublicKey, s.Algorithm, sigalg.Digest(s.Algorithm.Hash, s.attrsDER), s.signature); err != nil {
		return fmt.Errorf("%w: %v", ErrSignature, err)
	}
	if s.SigningCert != nil && !s.SigningCert.Matches(s.Certificate) {
		return ErrSigningCertificate
	}
	return nil
}

// VerifyAll verifies every SignerInfo against the encapsulated content, or
// against content when the signature is detached. It returns the first
// error.
func (sd *SignedData) VerifyAll(content []byte) error {
	if !sd.Detached {
		content = sd.Content
	} else if content == nil {
		return errors.New("cms: detached signature without its content")
	}
	for _, s := range sd.Signers {
		if err := s.Verify(sd, content); err != nil {
			return err
		}
	}
	return nil
}

// Chain returns the certificates of the SignedData other than cert, as
// intermediates for path building.
func (sd *SignedData) Chain(cert *x509.Certificate) []*x509.Certificate {
	var out []*x509.Certificate
	for _, c := range sd.Certificates {
		if c != cert {
			out = append(out, c)
		}
	}
	return out
}
