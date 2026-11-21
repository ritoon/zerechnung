package xmldsig

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/xmlenc"
)

// SignOptions configure SignEnveloped.
type SignOptions struct {
	// Signer holds the private key; only a digest is signed, so the key may
	// stay in a smart card, an HSM or a remote signing service.
	Signer crypto.Signer
	// Chain is the signing certificate followed by the certificates that
	// lead to a trusted root, all written in ds:KeyInfo.
	Chain []*x509.Certificate
	// Hash is the digest algorithm; SHA-256 when zero.
	Hash crypto.Hash
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
	// SigningTime is written in xades:SigningTime, in UTC to the second.
	SigningTime time.Time
	// MimeType is the media type of the signed document, written in
	// xades:DataObjectFormat; "text/xml" when empty.
	MimeType string
	// Policy, when set, is written in xades:SignaturePolicyIdentifier
	// (XAdES-EPES).
	Policy *Policy
}

// SignEnveloped returns doc with an enveloped XAdES Baseline B signature
// appended as the last child of its root element. doc must be UTF-8 and
// must not already hold a signature in its root.
func SignEnveloped(doc []byte, o *SignOptions) ([]byte, error) {
	if o == nil || o.Signer == nil {
		return nil, errors.New("xmldsig: no signer")
	}
	if len(o.Chain) == 0 {
		return nil, errors.New("xmldsig: no signing certificate")
	}
	if o.SigningTime.IsZero() {
		return nil, errors.New("xmldsig: no signing time")
	}
	cert := o.Chain[0]
	if pub, ok := cert.PublicKey.(interface{ Equal(crypto.PublicKey) bool }); !ok || !pub.Equal(o.Signer.Public()) {
		return nil, errors.New("xmldsig: the signing certificate does not match the key of the signer")
	}
	alg, err := sigalg.For(cert.PublicKey, o.Hash, o.PSS)
	if err != nil {
		return nil, fmt.Errorf("xmldsig: %w", err)
	}
	d, err := parse(doc)
	if err != nil {
		return nil, err
	}
	if len(d.root.elems(NSDS, "Signature")) > 0 {
		return nil, errors.New("xmldsig: the document is already signed")
	}
	if d.rootEnd+2 > len(doc) || doc[d.rootEnd] != '<' || doc[d.rootEnd+1] != '/' {
		return nil, errors.New("xmldsig: the root element has no end tag")
	}
	whole, err := canonicalize(d, nil, nil, c14nMethod{}, false)
	if err != nil {
		return nil, err
	}

	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return nil, err
	}
	t := &template{
		id:       "xmldsig-" + hex.EncodeToString(rnd[:]),
		alg:      alg,
		docHash:  sigalg.Digest(alg.Hash, whole),
		chain:    o.Chain,
		time:     o.SigningTime.UTC().Truncate(time.Second),
		mimeType: o.MimeType,
		policy:   o.Policy,
	}
	if t.mimeType == "" {
		t.mimeType = "text/xml"
	}
	insert := func(sig []byte) []byte {
		out := make([]byte, 0, len(doc)+len(sig))
		out = append(out, doc[:d.rootEnd]...)
		out = append(out, sig...)
		return append(out, doc[d.rootEnd:]...)
	}

	// The digest of the signed properties, canonicalized in their context.
	spDoc, err := parse(insert(t.signature()))
	if err != nil {
		return nil, err
	}
	sp := findByID(spDoc.root, t.id+"-signedprops")
	if len(sp) != 1 {
		return nil, errors.New("xmldsig: signed properties not found")
	}
	spC14N, err := canonicalize(spDoc, sp[0], nil, c14nMethod{}, false)
	if err != nil {
		return nil, err
	}
	t.propsHash = sigalg.Digest(alg.Hash, spC14N)

	// The signature of the canonicalized SignedInfo.
	siDoc, err := parse(insert(t.signature()))
	if err != nil {
		return nil, err
	}
	si := siDoc.root.elems(NSDS, "Signature")[0].elem(NSDS, "SignedInfo")
	siC14N, err := canonicalize(siDoc, si, nil, c14nMethod{}, true)
	if err != nil {
		return nil, err
	}
	sig, err := sigalg.Sign(o.Signer, alg, sigalg.Digest(alg.Hash, siC14N))
	if err != nil {
		return nil, fmt.Errorf("xmldsig: signing: %w", err)
	}
	if alg.Scheme == sigalg.ECDSA {
		if sig, err = sigalg.ECDSARaw(sig, sigalg.ECDSASize(cert.PublicKey.(*ecdsa.PublicKey))); err != nil {
			return nil, err
		}
	}
	t.value = sig
	return insert(t.signature()), nil
}

// template writes the ds:Signature element.
type template struct {
	id        string
	alg       sigalg.Algorithm
	docHash   []byte
	propsHash []byte
	value     []byte
	chain     []*x509.Certificate
	time      time.Time
	mimeType  string
	policy    *Policy
}

func (t *template) signature() []byte {
	b64 := base64.StdEncoding.EncodeToString
	dm := `<ds:DigestMethod Algorithm="` + digestURI(t.alg.Hash) + `"/>`
	var b []byte
	b = append(b, `<ds:Signature xmlns:ds="`+NSDS+`" Id="`+t.id+`">`...)
	b = append(b, `<ds:SignedInfo>`...)
	b = append(b, `<ds:CanonicalizationMethod Algorithm="`+C14N10+`"/>`...)
	b = append(b, `<ds:SignatureMethod Algorithm="`+signatureURI(t.alg)+`"/>`...)
	b = append(b, `<ds:Reference Id="`+t.id+`-ref0" URI="">`...)
	b = append(b, `<ds:Transforms><ds:Transform Algorithm="`+TransformEnveloped+`"/><ds:Transform Algorithm="`+C14N10+`"/></ds:Transforms>`...)
	b = append(b, dm+`<ds:DigestValue>`+b64(t.docHash)+`</ds:DigestValue></ds:Reference>`...)
	b = append(b, `<ds:Reference Type="`+TypeSignedProperties+`" URI="#`+t.id+`-signedprops">`...)
	b = append(b, `<ds:Transforms><ds:Transform Algorithm="`+C14N10+`"/></ds:Transforms>`...)
	b = append(b, dm+`<ds:DigestValue>`+b64(t.propsHash)+`</ds:DigestValue></ds:Reference>`...)
	b = append(b, `</ds:SignedInfo>`...)
	b = append(b, `<ds:SignatureValue Id="`+t.id+`-sigvalue">`+b64(t.value)+`</ds:SignatureValue>`...)
	b = append(b, `<ds:KeyInfo><ds:X509Data>`...)
	for _, c := range t.chain {
		b = append(b, `<ds:X509Certificate>`+b64(c.Raw)+`</ds:X509Certificate>`...)
	}
	b = append(b, `</ds:X509Data></ds:KeyInfo>`...)
	b = append(b, `<ds:Object><xades:QualifyingProperties xmlns:xades="`+NSXAdES+`" Target="#`+t.id+`">`...)
	b = append(b, `<xades:SignedProperties Id="`+t.id+`-signedprops"><xades:SignedSignatureProperties>`...)
	b = append(b, `<xades:SigningTime>`+t.time.Format("2006-01-02T15:04:05Z")+`</xades:SigningTime>`...)
	b = append(b, `<xades:SigningCertificateV2><xades:Cert><xades:CertDigest>`...)
	b = append(b, dm+`<ds:DigestValue>`+b64(sigalg.Digest(t.alg.Hash, t.chain[0].Raw))+`</ds:DigestValue>`...)
	b = append(b, `</xades:CertDigest></xades:Cert></xades:SigningCertificateV2>`...)
	if p := t.policy; p != nil {
		b = append(b, `<xades:SignaturePolicyIdentifier><xades:SignaturePolicyId><xades:SigPolicyId><xades:Identifier>`...)
		b = xmlenc.AppendEscaped(b, p.Identifier, false)
		b = append(b, `</xades:Identifier>`...)
		if p.Description != "" {
			b = append(b, `<xades:Description>`...)
			b = xmlenc.AppendEscaped(b, p.Description, false)
			b = append(b, `</xades:Description>`...)
		}
		b = append(b, `</xades:SigPolicyId><xades:SigPolicyHash><ds:DigestMethod Algorithm="`+digestURI(p.Hash)+`"/>`...)
		b = append(b, `<ds:DigestValue>`+b64(p.Digest)+`</ds:DigestValue></xades:SigPolicyHash>`...)
		b = append(b, `</xades:SignaturePolicyId></xades:SignaturePolicyIdentifier>`...)
	}
	b = append(b, `</xades:SignedSignatureProperties><xades:SignedDataObjectProperties>`...)
	b = append(b, `<xades:DataObjectFormat ObjectReference="#`+t.id+`-ref0"><xades:MimeType>`...)
	b = xmlenc.AppendEscaped(b, t.mimeType, false)
	b = append(b, `</xades:MimeType></xades:DataObjectFormat>`...)
	b = append(b, `</xades:SignedDataObjectProperties></xades:SignedProperties>`...)
	b = append(b, `</xades:QualifyingProperties></ds:Object></ds:Signature>`...)
	return b
}

// findByID returns the elements of the subtree of n that bear an Id, ID or
// id attribute equal to id.
func findByID(n *node, id string) []*node {
	var out []*node
	n.walk(func(e *node) {
		for _, name := range [...]string{"Id", "ID", "id"} {
			if v, ok := e.attrValue(name); ok && v == id {
				out = append(out, e)
				return
			}
		}
	})
	return out
}
