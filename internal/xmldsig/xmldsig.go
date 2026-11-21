// Package xmldsig writes and reads enveloped XML signatures (XML Signature
// 1.1) with the qualifying properties of XAdES Baseline B (ETSI
// EN 319 132-1 §6.3), as the SdI admits them for FatturaPA files and as the
// Spanish Facturae requires them with a signature policy (XAdES-EPES).
//
// SignEnveloped appends a ds:Signature to the root element: a reference to
// the whole document (URI="" with the enveloped-signature transform), a
// reference to the xades:SignedProperties (signing time, digest of the
// signing certificate, format of the signed document, optional signature
// policy), and the certificate chain in ds:KeyInfo. Canonicalization is
// Canonical XML 1.0 without comments.
//
// Verify accepts what other signing tools write: Canonical XML 1.0 and 1.1
// and Exclusive XML Canonicalization, with or without comments, the
// enveloped-signature transform or its XPath equivalent
// not(ancestor-or-self::ds:Signature). Against signature wrapping, it only
// accepts a signature that is a child of the root element, that is the only
// one of the document (counter-signatures inside it aside), and whose
// reference to the document covers all of it; the other references must
// point inside the signature, by an identifier that only one element bears.
// Trust in the certificate is left to the caller.
package xmldsig

import (
	"crypto"

	"github.com/ritoon/zerechnung/internal/sigalg"
)

// Namespaces.
const (
	NSDS       = "http://www.w3.org/2000/09/xmldsig#"
	NSXAdES    = "http://uri.etsi.org/01903/v1.3.2#"
	NSXAdES141 = "http://uri.etsi.org/01903/v1.4.1#"
)

// Algorithm identifiers.
const (
	TransformEnveloped   = NSDS + "enveloped-signature"
	TransformXPath       = "http://www.w3.org/TR/1999/REC-xpath-19991116"
	TransformXPath2      = "http://www.w3.org/2002/06/xmldsig-filter2"
	TypeSignedProperties = "http://uri.etsi.org/01903#SignedProperties"
)

var digestURIs = []struct {
	uri  string
	hash crypto.Hash
}{
	{"http://www.w3.org/2001/04/xmlenc#sha256", crypto.SHA256},
	{"http://www.w3.org/2001/04/xmldsig-more#sha384", crypto.SHA384},
	{"http://www.w3.org/2001/04/xmlenc#sha512", crypto.SHA512},
	{NSDS + "sha1", crypto.SHA1},
}

func digestURI(h crypto.Hash) string {
	for _, d := range digestURIs {
		if d.hash == h {
			return d.uri
		}
	}
	return ""
}

func hashFromURI(uri string) (crypto.Hash, bool) {
	for _, d := range digestURIs {
		if d.uri == uri {
			return d.hash, true
		}
	}
	return 0, false
}

var signatureURIs = []struct {
	uri string
	alg sigalg.Algorithm
}{
	{"http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", sigalg.Algorithm{Scheme: sigalg.PKCS1v15, Hash: crypto.SHA256}},
	{"http://www.w3.org/2001/04/xmldsig-more#rsa-sha384", sigalg.Algorithm{Scheme: sigalg.PKCS1v15, Hash: crypto.SHA384}},
	{"http://www.w3.org/2001/04/xmldsig-more#rsa-sha512", sigalg.Algorithm{Scheme: sigalg.PKCS1v15, Hash: crypto.SHA512}},
	{NSDS + "rsa-sha1", sigalg.Algorithm{Scheme: sigalg.PKCS1v15, Hash: crypto.SHA1}},
	{"http://www.w3.org/2007/05/xmldsig-more#sha256-rsa-MGF1", sigalg.Algorithm{Scheme: sigalg.PSS, Hash: crypto.SHA256}},
	{"http://www.w3.org/2007/05/xmldsig-more#sha384-rsa-MGF1", sigalg.Algorithm{Scheme: sigalg.PSS, Hash: crypto.SHA384}},
	{"http://www.w3.org/2007/05/xmldsig-more#sha512-rsa-MGF1", sigalg.Algorithm{Scheme: sigalg.PSS, Hash: crypto.SHA512}},
	{"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256", sigalg.Algorithm{Scheme: sigalg.ECDSA, Hash: crypto.SHA256}},
	{"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384", sigalg.Algorithm{Scheme: sigalg.ECDSA, Hash: crypto.SHA384}},
	{"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512", sigalg.Algorithm{Scheme: sigalg.ECDSA, Hash: crypto.SHA512}},
	{"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1", sigalg.Algorithm{Scheme: sigalg.ECDSA, Hash: crypto.SHA1}},
}

func signatureURI(a sigalg.Algorithm) string {
	for _, s := range signatureURIs {
		if s.alg == a {
			return s.uri
		}
	}
	return ""
}

func algFromURI(uri string) (sigalg.Algorithm, bool) {
	for _, s := range signatureURIs {
		if s.uri == uri {
			return s.alg, true
		}
	}
	return sigalg.Algorithm{}, false
}

// Policy identifies a signature policy (xades:SignaturePolicyIdentifier),
// which makes the signature an XAdES-EPES one.
type Policy struct {
	// Identifier is the URI (or the urn:oid: URN) of the policy.
	Identifier string
	// Description is an optional description of the policy.
	Description string
	// Hash and Digest are the digest algorithm and the digest of the policy
	// document. SHA-1 is admitted here: the Facturae policies are
	// identified by their SHA-1 digest.
	Hash   crypto.Hash
	Digest []byte
}
