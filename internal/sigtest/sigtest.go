// Package sigtest builds the certificates, keys, CRLs and OCSP responses
// that the signature tests need: a root, an intermediate authority and
// signing certificates with the QCStatements of qualified certificates,
// all generated at run time. No real certificate is ever used.
package sigtest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"sync"
	"testing"
	"time"
)

// Epoch is the reference time of the test certificates: they are valid from
// a year before to a year after it.
var Epoch = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// PKI is a test public key infrastructure: a root and an intermediate
// authority.
type PKI struct {
	Root            *x509.Certificate
	RootKey         crypto.Signer
	Intermediate    *x509.Certificate
	IntermediateKey crypto.Signer
	// Roots holds the root only.
	Roots *x509.CertPool
}

var (
	rsaOnce sync.Once
	rsaKey  *rsa.PrivateKey
)

// RSAKey returns an RSA 2048 key, the same for every call of a test binary
// (generating one takes a while).
func RSAKey(tb testing.TB) *rsa.PrivateKey {
	tb.Helper()
	rsaOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		rsaKey = k
	})
	return rsaKey
}

// ECKey returns a new ECDSA P-256 key.
func ECKey(tb testing.TB) *ecdsa.PrivateKey {
	tb.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	return k
}

var serial int64 = 1000

func nextSerial() *big.Int {
	serial++
	return big.NewInt(serial)
}

// New returns a PKI whose authorities have ECDSA P-256 keys. They are valid
// from years before Epoch to years after it, and after the current time.
func New(tb testing.TB) *PKI {
	tb.Helper()
	p := &PKI{RootKey: ECKey(tb), IntermediateKey: ECKey(tb)}
	last := Epoch
	if now := time.Now(); now.After(last) {
		last = now
	}
	root := &x509.Certificate{
		SerialNumber:          nextSerial(),
		Subject:               pkix.Name{CommonName: "ZeRechnung Test Root CA", Organization: []string{"ZeRechnung tests"}, Country: []string{"IT"}},
		NotBefore:             Epoch.AddDate(-5, 0, 0),
		NotAfter:              last.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	p.Root = create(tb, root, root, p.RootKey.Public(), p.RootKey)
	inter := &x509.Certificate{
		SerialNumber:          nextSerial(),
		Subject:               pkix.Name{CommonName: "ZeRechnung Test Qualified CA", Organization: []string{"ZeRechnung tests"}, Country: []string{"IT"}},
		NotBefore:             Epoch.AddDate(-4, 0, 0),
		NotAfter:              last.AddDate(8, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	p.Intermediate = create(tb, inter, p.Root, p.IntermediateKey.Public(), p.RootKey)
	p.Roots = x509.NewCertPool()
	p.Roots.AddCert(p.Root)
	return p
}

func create(tb testing.TB, tmpl, parent *x509.Certificate, pub crypto.PublicKey, key crypto.Signer) *x509.Certificate {
	tb.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, key)
	if err != nil {
		tb.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// QcType values for Leaf.
const (
	QcNone      = iota // no QCStatements
	QcSignature        // QcCompliance, QcSSCD, QcType esign
	QcSeal             // QcCompliance, QcSSCD, QcType eseal
	QcNoType           // QcCompliance only
)

// LeafOptions describe a signing certificate.
type LeafOptions struct {
	// Key is the key of the certificate: the shared RSA key when nil.
	Key crypto.Signer
	// QC selects the QCStatements.
	QC int
	// NotBefore and NotAfter default to a year around Epoch.
	NotBefore, NotAfter time.Time
	// KeyUsage defaults to nonRepudiation, as Italian qualified signing
	// certificates have it.
	KeyUsage x509.KeyUsage
	// CommonName defaults to "Mario Rossi".
	CommonName string
	// SelfSigned issues the certificate by itself instead of the
	// intermediate authority.
	SelfSigned bool
	// ExtKeyUsage and CRLDistributionPoints are left out when empty.
	ExtKeyUsage           []x509.ExtKeyUsage
	CRLDistributionPoints []string
}

// Leaf returns a signing certificate issued by the intermediate authority,
// and its key.
func (p *PKI) Leaf(tb testing.TB, o LeafOptions) (*x509.Certificate, crypto.Signer) {
	tb.Helper()
	key := o.Key
	if key == nil {
		key = RSAKey(tb)
	}
	if o.NotBefore.IsZero() {
		o.NotBefore = Epoch.AddDate(-1, 0, 0)
	}
	if o.NotAfter.IsZero() {
		o.NotAfter = Epoch.AddDate(1, 0, 0)
	}
	if o.KeyUsage == 0 {
		o.KeyUsage = x509.KeyUsageContentCommitment
	}
	if o.CommonName == "" {
		o.CommonName = "Mario Rossi"
	}
	tmpl := &x509.Certificate{
		SerialNumber: nextSerial(),
		Subject:      pkix.Name{CommonName: o.CommonName, SerialNumber: "TINIT-RSSMRA80A01H501U", Country: []string{"IT"}},
		NotBefore:    o.NotBefore,
		NotAfter:     o.NotAfter,
		KeyUsage:     o.KeyUsage,

		ExtKeyUsage:           o.ExtKeyUsage,
		CRLDistributionPoints: o.CRLDistributionPoints,
	}
	if ext, ok := qcExtension(o.QC); ok {
		tmpl.ExtraExtensions = []pkix.Extension{ext}
	}
	if o.SelfSigned {
		return create(tb, tmpl, tmpl, key.Public(), key), key
	}
	return create(tb, tmpl, p.Intermediate, key.Public(), p.IntermediateKey), key
}

type qcStatement struct {
	ID   asn1.ObjectIdentifier
	Info asn1.RawValue `asn1:"optional"`
}

func qcExtension(qc int) (pkix.Extension, bool) {
	compliance := asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	sscd := asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
	qcType := asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}
	var stmts []qcStatement
	typeStmt := func(n int) qcStatement {
		info, _ := asn1.Marshal([]asn1.ObjectIdentifier{{0, 4, 0, 1862, 1, 6, n}})
		return qcStatement{ID: qcType, Info: asn1.RawValue{FullBytes: info}}
	}
	switch qc {
	case QcSignature:
		stmts = []qcStatement{{ID: compliance}, {ID: sscd}, typeStmt(1)}
	case QcSeal:
		stmts = []qcStatement{{ID: compliance}, {ID: sscd}, typeStmt(2)}
	case QcNoType:
		stmts = []qcStatement{{ID: compliance}}
	default:
		return pkix.Extension{}, false
	}
	v, err := asn1.Marshal(stmts)
	if err != nil {
		panic(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}, Value: v}, true
}

// CRL returns a CRL of the intermediate authority that revokes the given
// certificates at the given time (reason certificateHold when hold is set).
func (p *PKI) CRL(tb testing.TB, thisUpdate time.Time, revokedAt time.Time, hold bool, revoked ...*x509.Certificate) *x509.RevocationList {
	tb.Helper()
	tmpl := &x509.RevocationList{
		Number:     nextSerial(),
		ThisUpdate: thisUpdate,
		NextUpdate: thisUpdate.AddDate(0, 0, 7),
	}
	for _, c := range revoked {
		e := x509.RevocationListEntry{SerialNumber: c.SerialNumber, RevocationTime: revokedAt}
		if hold {
			e.ReasonCode = 6
		}
		tmpl.RevokedCertificateEntries = append(tmpl.RevokedCertificateEntries, e)
	}
	return p.SignCRL(tb, tmpl)
}

// SignCRL returns the CRL of the template, signed by the intermediate
// authority; a zero Number gets a fresh one.
func (p *PKI) SignCRL(tb testing.TB, tmpl *x509.RevocationList) *x509.RevocationList {
	tb.Helper()
	if tmpl.Number == nil {
		tmpl.Number = nextSerial()
	}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, p.Intermediate, p.IntermediateKey)
	if err != nil {
		tb.Fatal(err)
	}
	crl, err := x509.ParseRevocationList(der)
	if err != nil {
		tb.Fatal(err)
	}
	return crl
}

// RootCRL returns a CRL of the root authority that revokes nothing: the
// revocation data of the intermediate authority.
func (p *PKI) RootCRL(tb testing.TB, thisUpdate time.Time) *x509.RevocationList {
	tb.Helper()
	tmpl := &x509.RevocationList{Number: nextSerial(), ThisUpdate: thisUpdate, NextUpdate: thisUpdate.AddDate(0, 0, 7)}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, p.Root, p.RootKey)
	if err != nil {
		tb.Fatal(err)
	}
	crl, err := x509.ParseRevocationList(der)
	if err != nil {
		tb.Fatal(err)
	}
	return crl
}

// OCSP statuses for OCSP.
const (
	OCSPGood = iota
	OCSPRevoked
	OCSPUnknown
)

// OCSP returns an OCSP response about cert, signed by the intermediate
// authority (byKey: the responder is identified by the hash of its key).
func (p *PKI) OCSP(tb testing.TB, cert *x509.Certificate, status int, revokedAt, thisUpdate time.Time, byKey bool) []byte {
	tb.Helper()
	return p.ocsp(tb, cert, status, revokedAt, thisUpdate, byKey, nil, p.IntermediateKey)
}

// OCSPFrom returns an OCSP response about cert signed by a responder the
// intermediate authority delegated, whose certificate the response carries
// and whose key is an ECDSA one.
func (p *PKI) OCSPFrom(tb testing.TB, cert *x509.Certificate, status int, revokedAt, thisUpdate time.Time, responder *x509.Certificate, key crypto.Signer) []byte {
	tb.Helper()
	return p.ocsp(tb, cert, status, revokedAt, thisUpdate, false, responder, key)
}

func (p *PKI) ocsp(tb testing.TB, cert *x509.Certificate, status int, revokedAt, thisUpdate time.Time, byKey bool, responderCert *x509.Certificate, key crypto.Signer) []byte {
	tb.Helper()
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(p.Intermediate.RawSubjectPublicKeyInfo, &spki); err != nil {
		tb.Fatal(err)
	}
	nameHash := sha1.Sum(p.Intermediate.RawSubject)
	keyHash := sha1.Sum(spki.PublicKey.RightAlign())
	certID, _ := asn1.Marshal(struct {
		HashAlg pkix.AlgorithmIdentifier
		Name    []byte
		Key     []byte
		Serial  *big.Int
	}{pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}, Parameters: asn1.NullRawValue}, nameHash[:], keyHash[:], cert.SerialNumber})
	var certStatus []byte
	switch status {
	case OCSPGood:
		certStatus = []byte{0x80, 0x00}
	case OCSPUnknown:
		certStatus = []byte{0x82, 0x00}
	default:
		t, _ := asn1.MarshalWithParams(revokedAt.UTC(), "generalized")
		certStatus = append([]byte{0xa1, byte(len(t))}, t...)
	}
	thisUp, _ := asn1.MarshalWithParams(thisUpdate.UTC(), "generalized")
	single := der(0x30, certID, certStatus, thisUp)
	var responder []byte
	if byKey {
		kh, _ := asn1.Marshal(keyHash[:])
		responder = der(0xa2, kh)
	} else if responderCert != nil {
		responder = der(0xa1, responderCert.RawSubject)
	} else {
		responder = der(0xa1, p.Intermediate.RawSubject)
	}
	produced, _ := asn1.MarshalWithParams(thisUpdate.UTC(), "generalized")
	tbs := der(0x30, responder, produced, der(0x30, single))
	h := crypto.SHA256.New()
	h.Write(tbs)
	sig, err := key.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
	if err != nil {
		tb.Fatal(err)
	}
	sigAlg, _ := asn1.Marshal(pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}})
	sigBits, _ := asn1.Marshal(asn1.BitString{Bytes: sig, BitLength: 8 * len(sig)})
	basic := der(0x30, tbs, sigAlg, sigBits)
	if responderCert != nil {
		basic = der(0x30, tbs, sigAlg, sigBits, der(0xa0, der(0x30, responderCert.Raw)))
	}
	basicOctets, _ := asn1.Marshal(basic)
	oid, _ := asn1.Marshal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1})
	statusOK := []byte{0x0a, 0x01, 0x00}
	return der(0x30, statusOK, der(0xa0, der(0x30, oid, basicOctets)))
}

func der(tag byte, content ...[]byte) []byte {
	var body []byte
	for _, c := range content {
		body = append(body, c...)
	}
	n := len(body)
	out := []byte{tag}
	switch {
	case n < 0x80:
		out = append(out, byte(n))
	case n < 0x100:
		out = append(out, 0x81, byte(n))
	default:
		out = append(out, 0x82, byte(n>>8), byte(n))
	}
	return append(out, body...)
}

// TSA returns the certificate of a time-stamping authority issued by the
// intermediate authority, with an ECDSA key and the time-stamping extended
// key usage marked critical, as RFC 3161 §2.3 requires.
func (p *PKI) TSA(tb testing.TB) (*x509.Certificate, crypto.Signer) {
	tb.Helper()
	key := ECKey(tb)
	eku, err := asn1.Marshal([]asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 8}})
	if err != nil {
		tb.Fatal(err)
	}
	last := Epoch
	if now := time.Now(); now.After(last) {
		last = now
	}
	tmpl := &x509.Certificate{
		SerialNumber:    nextSerial(),
		Subject:         pkix.Name{CommonName: "ZeRechnung Test TSA", Organization: []string{"ZeRechnung tests"}, Country: []string{"IT"}},
		NotBefore:       Epoch.AddDate(-2, 0, 0),
		NotAfter:        last.AddDate(5, 0, 0),
		KeyUsage:        x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Critical: true, Value: eku}},
	}
	return create(tb, tmpl, p.Intermediate, key.Public(), p.IntermediateKey), key
}
