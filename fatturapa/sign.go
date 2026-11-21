package fatturapa

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/esig"
	"github.com/ritoon/zerechnung/internal/xmldsig"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// SignOptions configure SignCAdES and SignXAdES.
type SignOptions struct {
	// Signer is the signing or sealing key. It may stay in the qualified
	// device that holds it (smart card, HSM, remote signing service): only
	// a digest is signed. The SdI requires a qualified certificate for an
	// invoice to a public administration.
	Signer crypto.Signer
	// Chain is the signing certificate first, followed by the certificates
	// that lead to the root of its certification authority. They are all
	// written in the signature.
	Chain []*x509.Certificate
	// SigningTime is the riferimento temporale of the SdI, the signing-time
	// attribute (CAdES) or xades:SigningTime (XAdES); time.Now() when zero.
	SigningTime time.Time
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
}

// MaxFileSize bounds the length of the files that Unwrap and
// VerifySignature read.
const MaxFileSize = 64 << 20

func (o *SignOptions) signingTime() time.Time {
	if o.SigningTime.IsZero() {
		return time.Now()
	}
	return o.SigningTime
}

// SignCAdES signs a FatturaPA document in CAdES Baseline B (ETSI
// EN 319 122-1), the format of the .xml.p7m files the SdI accepts (SdI
// technical specifications 1.8.4, §2.1): a CMS SignedData in DER that
// encapsulates the document, with the signed attributes content-type,
// message-digest, signing-time, signing-certificate-v2 and mime-type, and
// the certificate chain. SHA-256 is the digest algorithm. Name the result
// with FileName and cades set.
func SignCAdES(doc []byte, opts *SignOptions) ([]byte, error) {
	if opts == nil {
		return nil, errors.New("fatturapa: no signing options")
	}
	if err := checkSignable(doc); err != nil {
		return nil, err
	}
	p7m, err := cms.Sign(doc, &cms.SignOptions{
		Signer:      opts.Signer,
		Chain:       opts.Chain,
		PSS:         opts.PSS,
		SigningTime: opts.signingTime(),
		MimeType:    "text/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("fatturapa: %w", err)
	}
	return p7m, nil
}

// SignXAdES signs a FatturaPA document in XAdES Baseline B (ETSI
// EN 319 132-1), enveloped, the only XAdES mode the SdI accepts (§2.1): the
// ds:Signature becomes the last child of FatturaElettronica, as the
// FatturaPA schema places it, and signs the whole document (reference
// URI="" with the enveloped-signature transform) and the xades:SignedProperties
// (signing time, digest of the signing certificate, format of the
// document). The document must be UTF-8 and not signed yet. Name the
// result with FileName and cades unset.
func SignXAdES(doc []byte, opts *SignOptions) ([]byte, error) {
	if opts == nil {
		return nil, errors.New("fatturapa: no signing options")
	}
	if err := checkSignable(doc); err != nil {
		return nil, err
	}
	out, err := xmldsig.SignEnveloped(doc, &xmldsig.SignOptions{
		Signer:      opts.Signer,
		Chain:       opts.Chain,
		PSS:         opts.PSS,
		SigningTime: opts.signingTime(),
	})
	if err != nil {
		return nil, fmt.Errorf("fatturapa: %w", err)
	}
	return out, nil
}

// checkSignable checks that doc is a FatturaPA document.
func checkSignable(doc []byte) error {
	root, err := xmltree.Parse(doc)
	if err != nil {
		return fmt.Errorf("fatturapa: %w", err)
	}
	return checkRoot(root)
}

// Format is the format of a signature.
type Format int

// Signature formats.
const (
	CAdES Format = iota + 1 // CMS SignedData, .xml.p7m
	XAdES                   // enveloped XML signature, .xml
)

func (f Format) String() string {
	switch f {
	case CAdES:
		return "CAdES"
	case XAdES:
		return "XAdES"
	}
	return "unknown"
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

// Signature describes the signature of a file.
type Signature struct {
	// Format is CAdES or XAdES.
	Format Format
	// Signers are the signatures that verify: the SignerInfos of a CAdES
	// envelope (several for parallel signatures, and those of the inner
	// envelopes, outermost first, for nested ones), or the XAdES
	// signature.
	Signers []Signer
}

// Signer is one signature of a file.
type Signer struct {
	// Certificate is the signing certificate.
	Certificate *x509.Certificate
	// Certificates are the other certificates the signature carries, to
	// build the chain.
	Certificates []*x509.Certificate
	// SigningTime is the riferimento temporale, zero when the signature
	// has none.
	SigningTime time.Time
	// Kind tells whether a signature or a seal was applied.
	Kind Kind
	// Qualified tells that the certificate declares itself an EU qualified
	// certificate (QcCompliance, or a QCP-n or QCP-l policy).
	Qualified bool
	// QSCD tells that the certificate declares its key held in a qualified
	// signature or seal creation device.
	QSCD bool
	// Baseline lists the requirements of CAdES or XAdES Baseline B the
	// signature does not meet, other than the signing time; empty for a
	// Baseline B signature.
	Baseline []string
}

// maxNesting bounds the envelopes Unwrap opens in a nested CAdES signature.
const maxNesting = 4

// ErrNotSigned is the error of Unwrap for a document without signature.
var ErrNotSigned = errors.New("fatturapa: the file is not signed")

// Unwrap opens a signed FatturaPA file and returns the document it signs,
// after checking every signature against it: the digests, the signature
// values and the signed reference to the signing certificate. It does not
// check trust in the certificates (VerifySignature does).
//
// A .xml.p7m file may be in DER or BER, or encoded in base64, with or
// without PEM header lines, as such files are often received; nested
// envelopes (a .p7m signed again) are opened. For a file signed in XAdES,
// the document returned is the file itself, signature included, once the
// signature is known to cover all of it. A document without signature
// gives ErrNotSigned.
func Unwrap(file []byte) ([]byte, *Signature, error) {
	if len(file) > MaxFileSize {
		return nil, nil, fmt.Errorf("fatturapa: file larger than %d bytes", MaxFileSize)
	}
	if isXML(file) {
		res, err := xmldsig.Verify(file)
		if errors.Is(err, xmldsig.ErrNotSigned) {
			return nil, nil, ErrNotSigned
		}
		if err != nil {
			return nil, nil, fmt.Errorf("fatturapa: %w", err)
		}
		return file, &Signature{Format: XAdES, Signers: []Signer{xadesSigner(res)}}, nil
	}
	der, err := decodeP7M(file)
	if err != nil {
		return nil, nil, err
	}
	sig := &Signature{Format: CAdES}
	sd, err := cms.Parse(der)
	if err != nil {
		return nil, nil, fmt.Errorf("fatturapa: %w", err)
	}
	for depth := 1; ; depth++ {
		if sd.Detached {
			return nil, nil, errors.New("fatturapa: the signature does not hold the document (detached CMS)")
		}
		if err := sd.VerifyAll(nil); err != nil {
			return nil, nil, fmt.Errorf("fatturapa: %w", err)
		}
		for _, s := range sd.Signers {
			sig.Signers = append(sig.Signers, cadesSigner(sd, s))
		}
		content := sd.Content
		if depth >= maxNesting || isXML(content) {
			return content, sig, nil
		}
		// A nested envelope: the content is itself a signed file.
		inner, err := decodeP7M(content)
		if err != nil {
			return content, sig, nil
		}
		if sd, err = cms.Parse(inner); err != nil {
			return content, sig, nil
		}
	}
}

// isXML reports whether data starts like an XML document.
func isXML(data []byte) bool {
	d := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	d = bytes.TrimLeft(d, " \t\r\n")
	return len(d) > 0 && d[0] == '<'
}

// decodeP7M returns the DER (or BER) bytes of a .p7m file, decoding base64
// and dropping PEM header lines when the file is not binary.
func decodeP7M(data []byte) ([]byte, error) {
	if len(data) > 0 && data[0] == 0x30 {
		return data, nil
	}
	var b64 []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("-----")) {
			continue
		}
		b64 = append(b64, line...)
	}
	b64 = bytes.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' {
			return -1
		}
		return r
	}, b64)
	decode := func(enc *base64.Encoding, s []byte) ([]byte, error) {
		out := make([]byte, enc.DecodedLen(len(s)))
		n, err := enc.Decode(out, s)
		return out[:n], err
	}
	out, err := decode(base64.StdEncoding, b64)
	if err != nil {
		out, err = decode(base64.RawStdEncoding, bytes.TrimRight(b64, "="))
	}
	if err != nil || len(out) == 0 || out[0] != 0x30 {
		return nil, errors.New("fatturapa: the file is neither XML nor a CMS signature in DER or base64")
	}
	return out, nil
}

func describe(s *Signer, cert *x509.Certificate) {
	s.Certificate = cert
	if cert == nil {
		return
	}
	qc := esig.ParseQC(cert)
	s.Qualified, s.QSCD = qc.Qualified, qc.QSCD
	switch qc.Type {
	case esig.TypeSignature:
		s.Kind = KindSignature
	case esig.TypeSeal:
		s.Kind = KindSeal
	}
}

func cadesSigner(sd *cms.SignedData, si *cms.SignerInfo) Signer {
	s := Signer{SigningTime: si.SigningTime}
	describe(&s, si.Certificate)
	s.Certificates = sd.Chain(si.Certificate)
	switch {
	case si.SigningCert == nil:
		s.Baseline = append(s.Baseline, "no signing-certificate-v2 attribute")
	case !si.SigningCert.V2:
		s.Baseline = append(s.Baseline, "signing-certificate attribute (SHA-1) instead of signing-certificate-v2")
	}
	return s
}

func xadesSigner(res *xmldsig.Result) Signer {
	s := Signer{SigningTime: res.SigningTime}
	describe(&s, res.Certificate)
	for _, c := range res.Certificates {
		if c != res.Certificate {
			s.Certificates = append(s.Certificates, c)
		}
	}
	switch {
	case !res.XAdES:
		s.Baseline = append(s.Baseline, "no signed XAdES properties (plain XML signature)")
	default:
		switch res.SigningCertificate {
		case "":
			s.Baseline = append(s.Baseline, "no xades:SigningCertificateV2")
		case "SigningCertificate":
			s.Baseline = append(s.Baseline, "xades:SigningCertificate instead of xades:SigningCertificateV2")
		}
		if !res.DataObjectFormat {
			s.Baseline = append(s.Baseline, "no xades:DataObjectFormat with a MimeType for the document")
		}
	}
	return s
}
