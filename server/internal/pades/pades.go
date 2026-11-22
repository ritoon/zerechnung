// Package pades signs PDF documents with PAdES Baseline B-B signatures
// (ETSI EN 319 142-1) and checks their integrity, for the server.
//
// It is a thin layer over zerechnung.Sign and zerechnung.VerifySignatures,
// which hold the implementation: Sign appends the signature to the
// document with an incremental update (ISO 32000-1, 7.5.6), so the XML
// invoice of a Factur-X PDF and any earlier signature stay valid; the
// signature is a detached CMS SignedData (SubFilter ETSI.CAdES.detached)
// with the signing time in /M, in an invisible signature field that PDF/A
// admits.
//
// Verify checks the integrity of every signature of a document: the byte
// range, the digest and the signature value. It neither builds a trust
// chain nor checks revocation, which zerechnung.VerifySignatures does when
// given trusted roots.
package pades

import (
	"crypto"
	"crypto/x509"
	"errors"
	"time"

	"github.com/ritoon/zerechnung"
)

// Errors of Sign.
var (
	// ErrEncrypted: the document is encrypted, which this package does not
	// handle (and PDF/A forbids).
	ErrEncrypted = zerechnung.ErrEncrypted
	// ErrLocked: the document is certified with a DocMDP signature that
	// allows no change (/P 1), so a new signature would invalidate it.
	ErrLocked = zerechnung.ErrCertified
)

// Options configure Sign.
type Options struct {
	// Signer holds the private key, RSA or ECDSA. Only a digest is signed,
	// so the key may stay in a smart card, an HSM or a remote service.
	Signer crypto.Signer
	// Chain is the signing certificate followed by the intermediate
	// authorities, all written in the signature.
	Chain []*x509.Certificate
	// Hash is the digest algorithm: SHA-256 when zero, or SHA-384 or
	// SHA-512. SHA-1 is refused.
	Hash crypto.Hash
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
	// Time is the signing time written in /M; time.Now() when zero.
	Time time.Time
	// Name, Reason, Location and ContactInfo are written in the /Name,
	// /Reason, /Location and /ContactInfo entries when not empty.
	Name, Reason, Location, ContactInfo string
	// FieldName is the name of the signature field: "Signature1", or the
	// next free "SignatureN", when empty. A name already taken by a field
	// is refused.
	FieldName string
}

// Sign returns pdf with a new PAdES Baseline B-B signature, appended as an
// incremental update. It refuses an encrypted document (ErrEncrypted), a
// document certified without changes allowed (ErrLocked), a key that does
// not match the signing certificate and SHA-1.
func Sign(data []byte, o *Options) ([]byte, error) {
	if o == nil {
		return nil, errors.New("pades: no signer")
	}
	return zerechnung.Sign(data, &zerechnung.SignOptions{
		Signer: o.Signer, Chain: o.Chain, Hash: o.Hash, PSS: o.PSS, Time: o.Time,
		Name: o.Name, Reason: o.Reason, Location: o.Location, ContactInfo: o.ContactInfo,
		FieldName: o.FieldName,
	})
}
