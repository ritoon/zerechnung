package xmldsig

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/sigalg"
)

// Errors of Verify.
var (
	// ErrNotSigned: the root element holds no ds:Signature.
	ErrNotSigned = errors.New("xmldsig: the document is not signed")
	// ErrInvalid: the signature does not verify, or cannot be checked.
	// Every other error of Verify wraps it.
	ErrInvalid = errors.New("xmldsig: the signature is not valid")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Result describes a signature that verifies.
type Result struct {
	// Certificate is the signing certificate: the one that
	// xades:SigningCertificateV2 designates, or else the certificate of
	// ds:KeyInfo whose key verifies the signature.
	Certificate *x509.Certificate
	// Certificates are the certificates of ds:KeyInfo, in their order.
	Certificates []*x509.Certificate
	// Algorithm is the signature algorithm.
	Algorithm sigalg.Algorithm
	// XAdES tells that the signature signs XAdES qualifying properties.
	XAdES bool
	// SigningTime is the signed xades:SigningTime, zero when absent.
	SigningTime time.Time
	// SigningCertificate tells which signed reference binds the signing
	// certificate: "SigningCertificateV2", "SigningCertificate", or "".
	SigningCertificate string
	// DataObjectFormat tells that a signed xades:DataObjectFormat with a
	// MimeType describes the document.
	DataObjectFormat bool
	// Policy is the signed signature policy, nil when there is none.
	Policy *Policy
}

// Verify checks the enveloped signature of doc: it must be a child of the
// root element and the only signature of the document (the signatures it
// holds, such as counter-signatures, aside), its reference with URI=""
// must cover the whole document but the signature, its other references
// must point inside the signature, every reference digest and the
// signature value must verify. SHA-1 is refused.
func Verify(doc []byte) (*Result, error) {
	d, err := parse(doc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var all []*node
	d.root.walk(func(n *node) {
		if n.is(NSDS, "Signature") {
			all = append(all, n)
		}
	})
	sigs := d.root.elems(NSDS, "Signature")
	switch {
	case len(sigs) == 0 && len(all) == 0:
		return nil, ErrNotSigned
	case len(sigs) == 0:
		return nil, invalidf("the signature is not a child of the root element")
	case len(sigs) > 1:
		return nil, invalidf("the root element holds %d signatures", len(sigs))
	}
	sig := sigs[0]
	for _, s := range all {
		if !within(s, sig) {
			return nil, invalidf("a second signature is outside the signature of the root element")
		}
	}
	v := &verifier{doc: d, sig: sig}
	return v.verify()
}

// within reports whether n is in the subtree of top.
func within(n, top *node) bool {
	for e := n; e != nil; e = e.parent {
		if e == top {
			return true
		}
	}
	return false
}

type verifier struct {
	doc *document
	sig *node
	res Result

	docRef   *node // the ds:Reference to the whole document
	propsRef *node // the ds:Reference to the signed properties
	props    *node // the xades:SignedProperties it designates

	ids map[string][]*node // the elements of the document by Id, ID or id
}

// MaxReferences bounds the ds:Reference elements of a signature: each one
// is digested before the signature value is checked, and a FatturaPA
// signature has two or three (the document, the signed properties, at times
// the key information).
const MaxReferences = 8

// MaxCertificates bounds the certificates of ds:KeyInfo, each of which may
// be tried as an issuer.
const MaxCertificates = 64

// byID returns the elements of the document that bear an Id, ID or id
// attribute equal to id.
func (v *verifier) byID(id string) []*node {
	if v.ids == nil {
		v.ids = map[string][]*node{}
		v.doc.root.walk(func(e *node) {
			for _, name := range [...]string{"Id", "ID", "id"} {
				if val, ok := e.attrValue(name); ok {
					v.ids[val] = append(v.ids[val], e)
					return
				}
			}
		})
	}
	return v.ids[id]
}

func (v *verifier) verify() (*Result, error) {
	kids := v.sig.childElements()
	if len(kids) < 2 || !kids[0].is(NSDS, "SignedInfo") || !kids[1].is(NSDS, "SignatureValue") {
		return nil, invalidf("ds:Signature does not start with ds:SignedInfo and ds:SignatureValue")
	}
	si, sv := kids[0], kids[1]
	var keyInfo *node
	for _, k := range kids[2:] {
		switch {
		case k.is(NSDS, "KeyInfo") && keyInfo == nil && k == kids[2]:
			keyInfo = k
		case k.is(NSDS, "Object"):
		default:
			return nil, invalidf("unexpected element %s in ds:Signature", k.local)
		}
	}

	// SignedInfo.
	siKids := si.childElements()
	if len(siKids) < 3 || !siKids[0].is(NSDS, "CanonicalizationMethod") || !siKids[1].is(NSDS, "SignatureMethod") {
		return nil, invalidf("malformed ds:SignedInfo")
	}
	cm, err := methodOf(siKids[0])
	if err != nil {
		return nil, err
	}
	algURI, _ := siKids[1].attrValue("Algorithm")
	alg, ok := algFromURI(algURI)
	if !ok {
		return nil, invalidf("unsupported signature method %q", algURI)
	}
	if alg.Weak() {
		return nil, invalidf("the signature method %q (SHA-1) is not admitted", algURI)
	}
	v.res.Algorithm = alg
	switch refs := len(siKids[2:]); {
	case refs == 0:
		return nil, invalidf("no ds:Reference")
	case refs > MaxReferences:
		return nil, invalidf("%d ds:Reference elements, more than %d", refs, MaxReferences)
	}
	uris := map[string]bool{}
	for _, ref := range siKids[2:] {
		if !ref.is(NSDS, "Reference") {
			return nil, invalidf("unexpected element %s in ds:SignedInfo", ref.local)
		}
		if uri, ok := ref.attrValue("URI"); ok {
			if uris[uri] {
				return nil, invalidf("several references to %q", uri)
			}
			uris[uri] = true
		}
		if err := v.reference(ref); err != nil {
			return nil, err
		}
	}
	if v.docRef == nil {
		return nil, invalidf("no reference covers the whole document (URI=\"\" with the enveloped-signature transform)")
	}

	// Certificates and qualifying properties.
	if keyInfo != nil {
		if v.res.Certificates, err = certificates(keyInfo); err != nil {
			return nil, err
		}
	}
	if v.props != nil {
		if err := v.properties(); err != nil {
			return nil, err
		}
	}

	// The signature value.
	siC14N, err := canonicalize(v.doc, si, nil, cm, true)
	if err != nil {
		return nil, invalidf("%v", err)
	}
	value, err := decodeBase64(sv.text())
	if err != nil {
		return nil, invalidf("ds:SignatureValue is not base64")
	}
	digest := sigalg.Digest(alg.Hash, siC14N)
	check := func(c *x509.Certificate) error {
		val := value
		if alg.Scheme == sigalg.ECDSA {
			pub, ok := c.PublicKey.(*ecdsa.PublicKey)
			if !ok {
				return errors.New("not an ECDSA key")
			}
			if len(val) != 2*sigalg.ECDSASize(pub) {
				return errors.New("ECDSA signature of the wrong length")
			}
			var err error
			if val, err = sigalg.ECDSAFromRaw(val); err != nil {
				return err
			}
		}
		return sigalg.Verify(c.PublicKey, alg, digest, val)
	}
	if v.res.Certificate != nil {
		if err := check(v.res.Certificate); err != nil {
			return nil, invalidf("the signature value does not verify: %v", err)
		}
	} else {
		for _, c := range v.res.Certificates {
			if check(c) == nil {
				v.res.Certificate = c
				break
			}
		}
		if v.res.Certificate == nil {
			if len(v.res.Certificates) == 0 {
				return nil, invalidf("ds:KeyInfo holds no certificate")
			}
			return nil, invalidf("the signature value does not verify with the certificates of ds:KeyInfo")
		}
	}
	return &v.res, nil
}

// methodOf reads the canonicalization method of a CanonicalizationMethod
// or Transform element, with its InclusiveNamespaces PrefixList.
func methodOf(e *node) (c14nMethod, error) {
	uri, _ := e.attrValue("Algorithm")
	m, ok := c14nFor(uri)
	if !ok {
		return m, invalidf("unsupported canonicalization %q", uri)
	}
	if m.exclusive {
		for _, k := range e.childElements() {
			if k.local == "InclusiveNamespaces" && k.space == ExcC14N {
				list, _ := k.attrValue("PrefixList")
				m.prefixes = map[string]bool{}
				for _, p := range strings.Fields(list) {
					m.prefixes[p] = true
				}
			}
		}
	}
	return m, nil
}

// reference checks the digest of a ds:Reference.
func (v *verifier) reference(ref *node) error {
	uri, hasURI := ref.attrValue("URI")
	if !hasURI {
		return invalidf("ds:Reference without URI")
	}
	refType, _ := ref.attrValue("Type")
	kids := ref.childElements()
	var transforms []*node
	if len(kids) > 0 && kids[0].is(NSDS, "Transforms") {
		transforms = kids[0].childElements()
		kids = kids[1:]
	}
	if len(kids) != 2 || !kids[0].is(NSDS, "DigestMethod") || !kids[1].is(NSDS, "DigestValue") {
		return invalidf("malformed ds:Reference")
	}
	dmURI, _ := kids[0].attrValue("Algorithm")
	h, ok := hashFromURI(dmURI)
	if !ok {
		return invalidf("unsupported digest method %q", dmURI)
	}
	if h == crypto.SHA1 {
		return invalidf("the digest method %q (SHA-1) is not admitted", dmURI)
	}
	want, err := decodeBase64(kids[1].text())
	if err != nil {
		return invalidf("ds:DigestValue is not base64")
	}

	var apex *node
	var excluded func(*node) bool
	switch {
	case uri == "":
		if v.docRef != nil {
			return invalidf("several references to the whole document")
		}
	case strings.HasPrefix(uri, "#") && !strings.HasPrefix(uri, "#xpointer("):
		targets := v.byID(uri[1:])
		if len(targets) != 1 {
			return invalidf("the reference %q designates %d elements", uri, len(targets))
		}
		apex = targets[0]
		if !within(apex, v.sig) {
			return invalidf("the reference %q points outside the signature", uri)
		}
	default:
		return invalidf("unsupported reference URI %q", uri)
	}

	// Transforms: the enveloped-signature transform (or one of its XPath
	// forms) for the reference to the document, then at most one
	// canonicalization, the last transform.
	m := c14nMethod{} // Canonical XML 1.0, applied when no other is given
	enveloped := false
	for i, t := range transforms {
		if !t.is(NSDS, "Transform") {
			return invalidf("unexpected element %s in ds:Transforms", t.local)
		}
		tURI, _ := t.attrValue("Algorithm")
		switch tURI {
		case TransformEnveloped:
			enveloped = true
			excluded = func(n *node) bool { return n == v.sig }
		case TransformXPath:
			if !isNotSignatureXPath(t) {
				return invalidf("unsupported XPath transform")
			}
			enveloped = true
			excluded = func(n *node) bool { return n.is(NSDS, "Signature") }
		case TransformXPath2:
			if !isSubtractSignatures(t) {
				return invalidf("unsupported XPath Filter 2.0 transform")
			}
			enveloped = true
			excluded = func(n *node) bool { return n.is(NSDS, "Signature") }
		default:
			cm, err := methodOf(t)
			if err != nil {
				return invalidf("unsupported transform %q", tURI)
			}
			if i != len(transforms)-1 {
				return invalidf("a canonicalization must be the last transform")
			}
			m = cm
		}
	}
	if uri == "" {
		if !enveloped {
			return invalidf("the reference to the document lacks the enveloped-signature transform")
		}
		v.docRef = ref
	} else if enveloped {
		excluded = nil // the signature is outside the subtree, or this is malformed anyway
		if within(v.sig, apex) {
			return invalidf("the reference %q holds the signature", uri)
		}
	}
	got, err := canonicalize(v.doc, apex, excluded, m, false)
	if err != nil {
		return invalidf("%v", err)
	}
	if string(sigalg.Digest(h, got)) != string(want) {
		if uri == "" {
			return invalidf("the document does not match the signed digest")
		}
		return invalidf("the element %q does not match the signed digest", uri)
	}
	if apex != nil && (refType == TypeSignedProperties || apex.is(NSXAdES, "SignedProperties")) {
		if v.propsRef != nil {
			return invalidf("several references to signed properties")
		}
		if !apex.is(NSXAdES, "SignedProperties") {
			return invalidf("the reference to the signed properties does not designate xades:SignedProperties")
		}
		v.propsRef, v.props = ref, apex
	}
	return nil
}

// isNotSignatureXPath reports whether an XPath transform holds the
// expression not(ancestor-or-self::ds:Signature), the prefix bound to the
// XML Signature namespace.
func isNotSignatureXPath(t *node) bool {
	x := t.elem(NSDS, "XPath")
	if x == nil {
		return false
	}
	expr := strings.Join(strings.Fields(x.text()), "")
	rest, ok := strings.CutPrefix(expr, "not(ancestor-or-self::")
	if !ok {
		return false
	}
	prefix, ok := strings.CutSuffix(rest, ":Signature)")
	if !ok || prefix == "" {
		return false
	}
	uri, ok := x.lookupNS(prefix)
	return ok && uri == NSDS
}

// isSubtractSignatures reports whether an XPath Filter 2.0 transform
// subtracts every ds:Signature element of the document, as EU DSS writes the
// enveloped-signature transform: a single dsig-filter2:XPath with
// Filter="subtract" and the expression /descendant::ds:Signature (or
// //ds:Signature), the prefix bound to the XML Signature namespace. Verify
// admits no ds:Signature outside the signature, so the subtraction removes
// the signature and the signatures it holds only.
func isSubtractSignatures(t *node) bool {
	kids := t.childElements()
	if len(kids) != 1 || !kids[0].is(TransformXPath2, "XPath") {
		return false
	}
	x := kids[0]
	if f, _ := x.attrValue("Filter"); f != "subtract" {
		return false
	}
	expr := strings.Join(strings.Fields(x.text()), "")
	rest, ok := strings.CutPrefix(expr, "/descendant::")
	if !ok {
		if rest, ok = strings.CutPrefix(expr, "//"); !ok {
			return false
		}
	}
	prefix, ok := strings.CutSuffix(rest, ":Signature")
	if !ok || prefix == "" {
		return false
	}
	uri, ok := x.lookupNS(prefix)
	return ok && uri == NSDS
}

// properties reads the signed qualifying properties.
func (v *verifier) properties() error {
	qp := v.props.parent
	if qp == nil || !qp.is(NSXAdES, "QualifyingProperties") || qp.parent == nil || !qp.parent.is(NSDS, "Object") || qp.parent.parent != v.sig {
		return invalidf("xades:SignedProperties is not in a xades:QualifyingProperties of the signature")
	}
	if id, ok := v.sig.attrValue("Id"); ok {
		if target, _ := qp.attrValue("Target"); target != "#"+id {
			return invalidf("xades:QualifyingProperties does not target the signature")
		}
	}
	v.res.XAdES = true
	ssp := v.props.elem(NSXAdES, "SignedSignatureProperties")
	if ssp != nil {
		if st := ssp.elem(NSXAdES, "SigningTime"); st != nil {
			t, err := parseDateTime(strings.TrimSpace(st.text()))
			if err != nil {
				return invalidf("xades:SigningTime cannot be read: %v", err)
			}
			v.res.SigningTime = t
		}
		if sc := ssp.elem(NSXAdES, "SigningCertificateV2"); sc != nil {
			v.res.SigningCertificate = "SigningCertificateV2"
			if err := v.signingCert(sc); err != nil {
				return err
			}
		} else if sc := ssp.elem(NSXAdES, "SigningCertificate"); sc != nil {
			v.res.SigningCertificate = "SigningCertificate"
			if err := v.signingCert(sc); err != nil {
				return err
			}
		}
		if spi := ssp.elem(NSXAdES, "SignaturePolicyIdentifier"); spi != nil {
			if id := spi.elem(NSXAdES, "SignaturePolicyId"); id != nil {
				p := &Policy{}
				if spid := id.elem(NSXAdES, "SigPolicyId"); spid != nil {
					p.Identifier = strings.TrimSpace(textOf(spid.elem(NSXAdES, "Identifier")))
					p.Description = strings.TrimSpace(textOf(spid.elem(NSXAdES, "Description")))
				}
				if ph := id.elem(NSXAdES, "SigPolicyHash"); ph != nil {
					if dm := ph.elem(NSDS, "DigestMethod"); dm != nil {
						u, _ := dm.attrValue("Algorithm")
						p.Hash, _ = hashFromURI(u)
					}
					p.Digest, _ = decodeBase64(textOf(ph.elem(NSDS, "DigestValue")))
				}
				v.res.Policy = p
			}
		}
	}
	if sdop := v.props.elem(NSXAdES, "SignedDataObjectProperties"); sdop != nil {
		docID, _ := v.docRef.attrValue("Id")
		for _, f := range sdop.elems(NSXAdES, "DataObjectFormat") {
			if r, _ := f.attrValue("ObjectReference"); docID != "" && r == "#"+docID && f.elem(NSXAdES, "MimeType") != nil {
				v.res.DataObjectFormat = true
			}
		}
	}
	return nil
}

// signingCert finds the certificate of ds:KeyInfo that the first xades:Cert
// of SigningCertificateV2 (or SigningCertificate) designates by its digest.
func (v *verifier) signingCert(sc *node) error {
	certs := sc.elems(NSXAdES, "Cert")
	if len(certs) == 0 {
		return invalidf("xades:%s holds no xades:Cert", sc.local)
	}
	cd := certs[0].elem(NSXAdES, "CertDigest")
	if cd == nil {
		return invalidf("xades:Cert without xades:CertDigest")
	}
	dm := cd.elem(NSDS, "DigestMethod")
	if dm == nil {
		return invalidf("xades:CertDigest without ds:DigestMethod")
	}
	u, _ := dm.attrValue("Algorithm")
	h, ok := hashFromURI(u)
	if !ok {
		return invalidf("unsupported digest method %q for the signing certificate", u)
	}
	want, err := decodeBase64(textOf(cd.elem(NSDS, "DigestValue")))
	if err != nil {
		return invalidf("the digest of the signing certificate is not base64")
	}
	for _, c := range v.res.Certificates {
		if string(sigalg.Digest(h, c.Raw)) == string(want) {
			v.res.Certificate = c
			return nil
		}
	}
	return invalidf("the signing certificate designated by xades:%s is not in ds:KeyInfo", sc.local)
}

func textOf(n *node) string {
	if n == nil {
		return ""
	}
	return n.text()
}

// certificates reads the certificates of ds:KeyInfo/ds:X509Data.
func certificates(keyInfo *node) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	for _, xd := range keyInfo.elems(NSDS, "X509Data") {
		for _, xc := range xd.elems(NSDS, "X509Certificate") {
			if len(out) == MaxCertificates {
				return nil, invalidf("more than %d certificates in ds:KeyInfo", MaxCertificates)
			}
			der, err := decodeBase64(xc.text())
			if err != nil {
				return nil, invalidf("ds:X509Certificate is not base64")
			}
			c, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, invalidf("ds:X509Certificate cannot be read: %v", err)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// decodeBase64 decodes base64 text, ignoring white space.
func decodeBase64(s string) ([]byte, error) {
	s = strings.Join(strings.Fields(s), "")
	return base64.StdEncoding.DecodeString(s)
}

// parseDateTime reads an xsd:dateTime; a time without zone is taken as UTC.
func parseDateTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not an xsd:dateTime", s)
}
