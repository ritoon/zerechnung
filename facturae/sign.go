package facturae

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/esig"
	"github.com/ritoon/zerechnung/internal/xmldsig"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The signature policy of Facturae, version 3.1, under which Sign signs and
// which VerifySignature expects: the policy of Facturae 3.2, 3.2.1 and
// 3.2.2. PolicyDigest is the SHA-1 digest, in base64, of the document of
// the policy, as the signing tools write it and FACe accepts it.
const (
	PolicyIdentifier  = "http://www.facturae.es/politica_de_firma_formato_facturae/politica_de_firma_formato_facturae_v3_1.pdf"
	PolicyDescription = "Política de Firma FacturaE v3.1"
	PolicyDigest      = "Ohixl6upD6av8N7pEvDABhEL6hM="
)

// SignerRole is the role of the signer (xades:ClaimedRole) that the
// signature policy admits: the issuer, the recipient or a third party
// signs the invoice.
type SignerRole string

// Signer roles, as Sign writes them. The policy also admits their English
// names, "supplier", "customer" and "third party", which ReadSignature
// reads as these roles.
const (
	RoleSupplier   SignerRole = "emisor"   // the issuer of the invoice signs
	RoleCustomer   SignerRole = "receptor" // the recipient signs (self-billing)
	RoleThirdParty SignerRole = "tercero"  // a third party signs, neither issuer nor recipient
)

// roleNames maps the values of xades:ClaimedRole that the policy admits to
// the roles.
var roleNames = map[string]SignerRole{
	"emisor": RoleSupplier, "supplier": RoleSupplier,
	"receptor": RoleCustomer, "customer": RoleCustomer,
	"tercero": RoleThirdParty, "third party": RoleThirdParty,
}

// Valid reports whether r is one of the three roles of the policy.
func (r SignerRole) Valid() bool {
	return r == RoleSupplier || r == RoleCustomer || r == RoleThirdParty
}

// issuerTypes maps the InvoiceIssuerType of the file header to the role of
// the signer.
var issuerTypes = map[string]SignerRole{"EM": RoleSupplier, "RE": RoleCustomer, "TE": RoleThirdParty}

// SignOptions configure Sign.
type SignOptions struct {
	// Signer is the signing or sealing key. It may stay in the device that
	// holds it (smart card, HSM, remote signing service): only a digest is
	// signed. FACe requires a qualified certificate.
	Signer crypto.Signer
	// Chain is the signing certificate first, followed by the certificates
	// that lead to the root of its certification authority. They are all
	// written in ds:KeyInfo, which the signature covers.
	Chain []*x509.Certificate
	// SigningTime is written in xades:SigningTime; time.Now() when zero.
	SigningTime time.Time
	// Role is the role of the signer. When empty, it follows the
	// InvoiceIssuerType of the file header: RoleSupplier for EM (and when
	// the header gives none), RoleCustomer for RE, RoleThirdParty for TE.
	Role SignerRole
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
}

// MaxFileSize bounds the length of the documents that ReadSignature and
// VerifySignature read.
const MaxFileSize = 64 << 20

// Sign signs a Facturae document (version 3.2, 3.2.1 or 3.2.2) in
// XAdES-EPES, enveloped, as the signature policy of Facturae v3.1 defines
// the basic advanced electronic signature (§1): the ds:Signature becomes
// the last child of the root element Facturae, where the schema places it,
// and its references cover the whole document (URI="" with the
// enveloped-signature transform), the xades:SignedProperties and the
// ds:KeyInfo that holds the certificate chain. The signed properties are
// xades:SigningTime, xades:SigningCertificate (digest, issuer and serial
// number of the signing certificate), xades:SignaturePolicyIdentifier
// (PolicyIdentifier and its SHA-1 digest PolicyDigest), xades:SignerRole
// and xades:DataObjectFormat. The XAdES namespace is that of version 1.3.2,
// the digest algorithm SHA-256, the canonicalization Canonical XML 1.0.
//
// The document must be UTF-8 and not signed yet. Any later change to the
// signed document, including writing it again with Marshal, breaks the
// signature.
func Sign(doc []byte, opts *SignOptions) ([]byte, error) {
	if opts == nil {
		return nil, errors.New("facturae: no signing options")
	}
	root, err := xmltree.Parse(doc)
	if err != nil {
		return nil, fmt.Errorf("facturae: %w", err)
	}
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	role := opts.Role
	switch {
	case role == "":
		if role = issuerTypes[root.Value("FileHeader/InvoiceIssuerType")]; role == "" {
			role = RoleSupplier
		}
	case !role.Valid():
		return nil, fmt.Errorf("facturae: signer role %q is not emisor, receptor or tercero", string(role))
	}
	t := opts.SigningTime
	if t.IsZero() {
		t = time.Now()
	}
	digest, _ := base64.StdEncoding.DecodeString(PolicyDigest)
	out, err := xmldsig.SignEnveloped(doc, &xmldsig.SignOptions{
		Signer:      opts.Signer,
		Chain:       opts.Chain,
		PSS:         opts.PSS,
		SigningTime: t,
		Description: "Factura electrónica",
		Policy: &xmldsig.Policy{
			Identifier:  PolicyIdentifier,
			Description: PolicyDescription,
			Hash:        crypto.SHA1,
			Digest:      digest,
		},
		SigningCertificateV1: true,
		SignKeyInfo:          true,
		ClaimedRoles:         []string{string(role)},
	})
	if err != nil {
		return nil, fmt.Errorf("facturae: %w", err)
	}
	return out, nil
}

// Kind tells whether a certificate is issued for electronic signatures or
// for electronic seals, as its QCStatements declare (ETSI EN 319 412-5).
type Kind int

// Certificate kinds.
const (
	KindUnknown   Kind = iota // the certificate declares neither
	KindSignature             // electronic signature of a natural person (QcType esign)
	KindSeal                  // electronic seal of a legal person, such as a company (QcType eseal)
)

func (k Kind) String() string {
	switch k {
	case KindSignature:
		return "signature"
	case KindSeal:
		return "seal"
	}
	return "unknown"
}

// Policy is a signature policy (xades:SignaturePolicyIdentifier).
type Policy struct {
	// Identifier is the URI of the policy.
	Identifier string
	// Description is the description of the policy, often empty.
	Description string
	// Hash and Digest are the digest algorithm and the digest of the
	// document of the policy; Hash is zero for an algorithm the package
	// does not know.
	Hash   crypto.Hash
	Digest []byte
}

// IsFacturae reports whether p is the signature policy of Facturae v3.1:
// its identifier PolicyIdentifier and its SHA-1 digest PolicyDigest.
func (p *Policy) IsFacturae() bool {
	return p != nil && p.Identifier == PolicyIdentifier && p.Hash == crypto.SHA1 &&
		base64.StdEncoding.EncodeToString(p.Digest) == PolicyDigest
}

// Signature describes the signature of a Facturae document.
type Signature struct {
	// Certificate is the signing certificate.
	Certificate *x509.Certificate
	// Certificates are the other certificates that ds:KeyInfo holds, to
	// build the chain.
	Certificates []*x509.Certificate
	// SigningTime is the signed xades:SigningTime, zero when the signature
	// has none.
	SigningTime time.Time
	// Policy is the signed signature policy, nil when there is none (the
	// signature is then not XAdES-EPES).
	Policy *Policy
	// Role is the signed role of the signer: one of the three roles when
	// xades:ClaimedRole holds a value that the policy admits (in Spanish
	// or in English), the value itself otherwise, "" when the signature
	// claims no role. Roles lists every claimed role, of which the policy
	// admits one only.
	Role  SignerRole
	Roles []string
	// KeyInfoSigned tells that a reference covers ds:KeyInfo, as the
	// policy requires (§1.3).
	KeyInfoSigned bool
	// SigningCertificate is the signed property that names the signing
	// certificate: "SigningCertificate" (the one the policy names),
	// "SigningCertificateV2" (EN 319 132-1), or "" when there is none.
	SigningCertificate string
	// Kind tells whether a signature or a seal was applied.
	Kind Kind
	// Qualified tells that the certificate declares itself an EU qualified
	// certificate (QcCompliance, or a QCP-n or QCP-l policy).
	Qualified bool
	// QSCD tells that the certificate declares its key held in a qualified
	// signature or seal creation device.
	QSCD bool
}

// ErrNotSigned is the error of ReadSignature for a document without
// signature.
var ErrNotSigned = errors.New("facturae: the document is not signed")

// ReadSignature checks the signature of a signed Facturae document and
// describes it: the signature must be a ds:Signature child of the root
// element and the only one of the document, cover the whole document, and
// its digests, its signature value and its signed reference to the signing
// certificate must verify. SHA-1 is refused, but for the digest of the
// policy and of the certificate. It does not check the policy, the signing
// time, the role nor trust in the certificate: VerifySignature does. A
// document without signature gives ErrNotSigned.
//
// The signature may come from another tool: the canonicalizations of XML
// Signature, the enveloped-signature transform or its XPath forms, XAdES
// 1.3.2 or 1.2.2 are accepted.
func ReadSignature(doc []byte) (*Signature, error) {
	if len(doc) > MaxFileSize {
		return nil, fmt.Errorf("facturae: document larger than %d bytes", MaxFileSize)
	}
	res, err := xmldsig.Verify(doc)
	if errors.Is(err, xmldsig.ErrNotSigned) {
		return nil, ErrNotSigned
	}
	if err != nil {
		return nil, fmt.Errorf("facturae: %w", err)
	}
	s := &Signature{
		Certificate:        res.Certificate,
		SigningTime:        res.SigningTime,
		Roles:              res.ClaimedRoles,
		KeyInfoSigned:      res.KeyInfoSigned,
		SigningCertificate: res.SigningCertificate,
	}
	for _, c := range res.Certificates {
		if c != res.Certificate {
			s.Certificates = append(s.Certificates, c)
		}
	}
	if p := res.Policy; p != nil {
		s.Policy = &Policy{Identifier: p.Identifier, Description: p.Description, Hash: p.Hash, Digest: p.Digest}
	}
	if len(s.Roles) > 0 {
		s.Role = SignerRole(s.Roles[0])
		if r, ok := roleNames[strings.ToLower(strings.Join(strings.Fields(s.Roles[0]), " "))]; ok {
			s.Role = r
		}
	}
	qc := esig.ParseQC(res.Certificate)
	s.Qualified, s.QSCD = qc.Qualified, qc.QSCD
	switch qc.Type {
	case esig.TypeSignature:
		s.Kind = KindSignature
	case esig.TypeSeal:
		s.Kind = KindSeal
	}
	return s, nil
}
