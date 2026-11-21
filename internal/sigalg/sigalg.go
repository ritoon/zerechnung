// Package sigalg holds the digest and signature algorithms shared by the CMS
// signatures (CAdES), the XML signatures (XAdES) and the revocation data
// (CRL, OCSP): their ASN.1 identifiers, and signing with a crypto.Signer or
// verifying with a public key.
//
// The schemes are RSA PKCS#1 v1.5, RSA-PSS (MGF1 with the same hash; the
// salt as long as the hash when signing, of the declared length when
// verifying) and ECDSA, with SHA-256, SHA-384 or SHA-512. SHA-1 is
// known, for revocation data and certificate identifiers, but the callers
// refuse it for the signature of a document (see Algorithm.Weak).
package sigalg

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"

	// The hash functions must be linked for crypto.Hash.New.
	_ "crypto/sha1"
	_ "crypto/sha256"
	_ "crypto/sha512"
)

// Object identifiers of the digest algorithms.
var (
	OIDSHA1   = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	OIDSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	OIDSHA384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	OIDSHA512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
)

// Object identifiers of the signature algorithms.
var (
	OIDRSAEncryption   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	OIDSHA1WithRSA     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 5}
	OIDMGF1            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 8}
	OIDRSAPSS          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 10}
	OIDSHA256WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	OIDSHA384WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	OIDSHA512WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	OIDECPublicKey     = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	OIDECDSAWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 1}
	OIDECDSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	OIDECDSAWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	OIDECDSAWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
)

var hashOIDs = []struct {
	oid  asn1.ObjectIdentifier
	hash crypto.Hash
}{
	{OIDSHA1, crypto.SHA1},
	{OIDSHA256, crypto.SHA256},
	{OIDSHA384, crypto.SHA384},
	{OIDSHA512, crypto.SHA512},
}

// HashFromOID returns the hash function a digest algorithm identifier names.
func HashFromOID(oid asn1.ObjectIdentifier) (crypto.Hash, bool) {
	for _, h := range hashOIDs {
		if h.oid.Equal(oid) {
			return h.hash, true
		}
	}
	return 0, false
}

// OIDFromHash returns the identifier of a hash function.
func OIDFromHash(h crypto.Hash) (asn1.ObjectIdentifier, bool) {
	for _, e := range hashOIDs {
		if e.hash == h {
			return e.oid, true
		}
	}
	return nil, false
}

// Digest returns the hash of data.
func Digest(h crypto.Hash, data []byte) []byte {
	w := h.New()
	w.Write(data)
	return w.Sum(nil)
}

// Scheme is a signature scheme.
type Scheme int

// Signature schemes.
const (
	PKCS1v15 Scheme = iota + 1 // RSASSA-PKCS1-v1_5
	PSS                        // RSASSA-PSS, MGF1 with the same hash
	ECDSA                      // ECDSA
)

func (s Scheme) String() string {
	switch s {
	case PKCS1v15:
		return "RSA PKCS#1 v1.5"
	case PSS:
		return "RSA-PSS"
	case ECDSA:
		return "ECDSA"
	}
	return "unknown"
}

// Algorithm is a signature scheme with its hash function.
type Algorithm struct {
	Scheme Scheme
	Hash   crypto.Hash
	// SaltLength is, for RSA-PSS, the length of the salt in bytes when it
	// is not the length of the hash (zero), as the RSASSA-PSS parameters of
	// a CMS signature may declare it: OpenSSL 3.0 signs with the longest
	// salt the key allows.
	SaltLength int
}

func (a Algorithm) String() string {
	s := a.Scheme.String() + " with " + a.Hash.String()
	if a.SaltLength != 0 {
		s += fmt.Sprintf(", salt of %d bytes", a.SaltLength)
	}
	return s
}

// pssOptions returns the options of an RSA-PSS signature.
func (a Algorithm) pssOptions() *rsa.PSSOptions {
	if a.SaltLength > 0 {
		return &rsa.PSSOptions{SaltLength: a.SaltLength, Hash: a.Hash}
	}
	return &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: a.Hash}
}

// saltLength returns the length of the salt of an RSA-PSS signature.
func (a Algorithm) saltLength() int {
	if a.SaltLength > 0 {
		return a.SaltLength
	}
	return a.Hash.Size()
}

// Weak reports whether the algorithm hashes with SHA-1, which the callers
// refuse for the signature of a document.
func (a Algorithm) Weak() bool { return a.Hash == crypto.SHA1 }

// ErrUnsupported is returned for an algorithm or a key this package does not
// handle.
var ErrUnsupported = errors.New("unsupported algorithm")

// For returns the algorithm used to sign with key: RSA PKCS#1 v1.5, or
// RSA-PSS when pss is set, for an RSA key, ECDSA for an ECDSA key; h is the
// hash function, SHA-256 when zero.
func For(key crypto.PublicKey, h crypto.Hash, pss bool) (Algorithm, error) {
	if h == 0 {
		h = crypto.SHA256
	}
	if _, ok := OIDFromHash(h); !ok || h == crypto.SHA1 {
		return Algorithm{}, fmt.Errorf("%w: hash %v", ErrUnsupported, h)
	}
	switch key.(type) {
	case *rsa.PublicKey:
		if pss {
			return Algorithm{Scheme: PSS, Hash: h}, nil
		}
		return Algorithm{Scheme: PKCS1v15, Hash: h}, nil
	case *ecdsa.PublicKey:
		if pss {
			return Algorithm{}, fmt.Errorf("%w: RSA-PSS with an ECDSA key", ErrUnsupported)
		}
		return Algorithm{Scheme: ECDSA, Hash: h}, nil
	}
	return Algorithm{}, fmt.Errorf("%w: key of type %T", ErrUnsupported, key)
}

// Sign signs digest, the hash of the message, with signer. An ECDSA
// signature is returned in its ASN.1 form, as crypto.Signer gives it.
func Sign(signer crypto.Signer, a Algorithm, digest []byte) ([]byte, error) {
	var opts crypto.SignerOpts = a.Hash
	if a.Scheme == PSS {
		opts = a.pssOptions()
	}
	return signer.Sign(rand.Reader, digest, opts)
}

// Verify checks sig, the signature of digest (the hash of the message) with
// the public key. An ECDSA signature is expected in its ASN.1 form.
func Verify(pub crypto.PublicKey, a Algorithm, digest, sig []byte) error {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		switch a.Scheme {
		case PKCS1v15:
			return rsa.VerifyPKCS1v15(k, a.Hash, digest, sig)
		case PSS:
			return rsa.VerifyPSS(k, a.Hash, digest, sig, a.pssOptions())
		}
	case *ecdsa.PublicKey:
		if a.Scheme == ECDSA {
			if !ecdsa.VerifyASN1(k, digest, sig) {
				return errors.New("ecdsa: verification error")
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %v with a key of type %T", ErrUnsupported, a, pub)
}

// pssParams is RSASSA-PSS-params (RFC 4055 §3.1).
type pssParams struct {
	Hash         pkix.AlgorithmIdentifier `asn1:"optional,explicit,tag:0"`
	MGF          pkix.AlgorithmIdentifier `asn1:"optional,explicit,tag:1"`
	SaltLength   int                      `asn1:"optional,explicit,tag:2,default:20"`
	TrailerField int                      `asn1:"optional,explicit,tag:3,default:1"`
}

// AlgorithmIdentifier returns the identifier of a for the signatureAlgorithm
// of a CMS SignerInfo: rsaEncryption for RSA PKCS#1 v1.5, as RFC 3370 §3.2
// and OpenSSL write it, RSASSA-PSS with its parameters, ecdsa-with-SHA*.
func (a Algorithm) AlgorithmIdentifier() (pkix.AlgorithmIdentifier, error) {
	hashOID, ok := OIDFromHash(a.Hash)
	if !ok {
		return pkix.AlgorithmIdentifier{}, ErrUnsupported
	}
	switch a.Scheme {
	case PKCS1v15:
		return pkix.AlgorithmIdentifier{Algorithm: OIDRSAEncryption, Parameters: asn1.NullRawValue}, nil
	case PSS:
		hashAI := pkix.AlgorithmIdentifier{Algorithm: hashOID, Parameters: asn1.NullRawValue}
		mgfParams, err := asn1.Marshal(hashAI)
		if err != nil {
			return pkix.AlgorithmIdentifier{}, err
		}
		params, err := asn1.Marshal(pssParams{
			Hash:         hashAI,
			MGF:          pkix.AlgorithmIdentifier{Algorithm: OIDMGF1, Parameters: asn1.RawValue{FullBytes: mgfParams}},
			SaltLength:   a.saltLength(),
			TrailerField: 1,
		})
		if err != nil {
			return pkix.AlgorithmIdentifier{}, err
		}
		return pkix.AlgorithmIdentifier{Algorithm: OIDRSAPSS, Parameters: asn1.RawValue{FullBytes: params}}, nil
	case ECDSA:
		switch a.Hash {
		case crypto.SHA256:
			return pkix.AlgorithmIdentifier{Algorithm: OIDECDSAWithSHA256}, nil
		case crypto.SHA384:
			return pkix.AlgorithmIdentifier{Algorithm: OIDECDSAWithSHA384}, nil
		case crypto.SHA512:
			return pkix.AlgorithmIdentifier{Algorithm: OIDECDSAWithSHA512}, nil
		}
	}
	return pkix.AlgorithmIdentifier{}, ErrUnsupported
}

// FromAlgorithmIdentifier reads a signature algorithm identifier, of a CMS
// SignerInfo or of an X.509 structure. digest is the hash function given
// apart (the digestAlgorithm of a CMS SignerInfo), used when the identifier
// names none (rsaEncryption, id-ecPublicKey); zero when there is none.
func FromAlgorithmIdentifier(ai pkix.AlgorithmIdentifier, digest crypto.Hash) (Algorithm, error) {
	oid := ai.Algorithm
	pick := func(s Scheme, h crypto.Hash) (Algorithm, error) {
		if digest != 0 && digest != h {
			return Algorithm{}, fmt.Errorf("%w: signature algorithm %v with digest %v", ErrUnsupported, oid, digest)
		}
		return Algorithm{Scheme: s, Hash: h}, nil
	}
	switch {
	case oid.Equal(OIDRSAEncryption), oid.Equal(OIDECPublicKey):
		if digest == 0 {
			return Algorithm{}, fmt.Errorf("%w: %v without a digest algorithm", ErrUnsupported, oid)
		}
		if oid.Equal(OIDECPublicKey) {
			return Algorithm{Scheme: ECDSA, Hash: digest}, nil
		}
		return Algorithm{Scheme: PKCS1v15, Hash: digest}, nil
	case oid.Equal(OIDSHA1WithRSA):
		return pick(PKCS1v15, crypto.SHA1)
	case oid.Equal(OIDSHA256WithRSA):
		return pick(PKCS1v15, crypto.SHA256)
	case oid.Equal(OIDSHA384WithRSA):
		return pick(PKCS1v15, crypto.SHA384)
	case oid.Equal(OIDSHA512WithRSA):
		return pick(PKCS1v15, crypto.SHA512)
	case oid.Equal(OIDECDSAWithSHA1):
		return pick(ECDSA, crypto.SHA1)
	case oid.Equal(OIDECDSAWithSHA256):
		return pick(ECDSA, crypto.SHA256)
	case oid.Equal(OIDECDSAWithSHA384):
		return pick(ECDSA, crypto.SHA384)
	case oid.Equal(OIDECDSAWithSHA512):
		return pick(ECDSA, crypto.SHA512)
	case oid.Equal(OIDRSAPSS):
		return pssFrom(ai.Parameters, digest)
	}
	return Algorithm{}, fmt.Errorf("%w: %v", ErrUnsupported, oid)
}

func pssFrom(raw asn1.RawValue, digest crypto.Hash) (Algorithm, error) {
	if len(raw.FullBytes) == 0 {
		return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS without parameters", ErrUnsupported)
	}
	var p pssParams
	if rest, err := asn1.Unmarshal(raw.FullBytes, &p); err != nil || len(rest) != 0 {
		return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS parameters cannot be read", ErrUnsupported)
	}
	h := crypto.SHA1 // the default of RFC 4055
	if len(p.Hash.Algorithm) != 0 {
		var ok bool
		if h, ok = HashFromOID(p.Hash.Algorithm); !ok {
			return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS with hash %v", ErrUnsupported, p.Hash.Algorithm)
		}
	}
	mgfHash := crypto.SHA1
	if len(p.MGF.Algorithm) != 0 {
		if !p.MGF.Algorithm.Equal(OIDMGF1) {
			return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS with mask generation %v", ErrUnsupported, p.MGF.Algorithm)
		}
		var mh pkix.AlgorithmIdentifier
		if _, err := asn1.Unmarshal(p.MGF.Parameters.FullBytes, &mh); err != nil {
			return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS MGF1 parameters cannot be read", ErrUnsupported)
		}
		var ok bool
		if mgfHash, ok = HashFromOID(mh.Algorithm); !ok {
			return Algorithm{}, fmt.Errorf("%w: MGF1 with hash %v", ErrUnsupported, mh.Algorithm)
		}
	}
	if mgfHash != h || p.TrailerField != 1 {
		return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS parameters other than MGF1 with the same hash", ErrUnsupported)
	}
	if p.SaltLength < 1 {
		// crypto/rsa cannot require an empty salt.
		return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS with a salt of %d bytes", ErrUnsupported, p.SaltLength)
	}
	if digest != 0 && digest != h {
		return Algorithm{}, fmt.Errorf("%w: RSASSA-PSS with %v and digest %v", ErrUnsupported, h, digest)
	}
	a := Algorithm{Scheme: PSS, Hash: h}
	if p.SaltLength != h.Size() {
		a.SaltLength = p.SaltLength
	}
	return a, nil
}

// ECDSARaw converts an ASN.1 ECDSA signature into the concatenation of r and
// s, each on size bytes, as XML signatures write it (RFC 4050, RFC 6931).
func ECDSARaw(der []byte, size int) ([]byte, error) {
	var sig struct{ R, S *big.Int }
	if rest, err := asn1.Unmarshal(der, &sig); err != nil || len(rest) != 0 {
		return nil, errors.New("ecdsa: malformed signature")
	}
	if sig.R.Sign() <= 0 || sig.S.Sign() <= 0 || len(sig.R.Bytes()) > size || len(sig.S.Bytes()) > size {
		return nil, errors.New("ecdsa: malformed signature")
	}
	out := make([]byte, 2*size)
	sig.R.FillBytes(out[:size])
	sig.S.FillBytes(out[size:])
	return out, nil
}

// ECDSAFromRaw converts the concatenation of r and s into an ASN.1 ECDSA
// signature.
func ECDSAFromRaw(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return nil, errors.New("ecdsa: malformed signature")
	}
	half := len(raw) / 2
	return asn1.Marshal(struct{ R, S *big.Int }{
		new(big.Int).SetBytes(raw[:half]),
		new(big.Int).SetBytes(raw[half:]),
	})
}

// ECDSASize returns the length in bytes of r and s for an ECDSA key.
func ECDSASize(pub *ecdsa.PublicKey) int { return (pub.Curve.Params().BitSize + 7) / 8 }
