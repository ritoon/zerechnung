package esig

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/sigtest"
)

// issue returns a certificate of the intermediate authority of p built from
// tmpl, for what sigtest.Leaf does not write.
func issue(t *testing.T, p *sigtest.PKI, tmpl *x509.Certificate) *x509.Certificate {
	t.Helper()
	if tmpl.SerialNumber == nil {
		leaf, _ := p.Leaf(t, sigtest.LeafOptions{})
		tmpl.SerialNumber = leaf.SerialNumber.Add(leaf.SerialNumber, leaf.SerialNumber)
	}
	if tmpl.NotBefore.IsZero() {
		tmpl.NotBefore, tmpl.NotAfter = sigtest.Epoch.AddDate(-1, 0, 0), sigtest.Epoch.AddDate(1, 0, 0)
	}
	tmpl.Subject = pkix.Name{CommonName: "Test"}
	key := sigtest.ECKey(t)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.Intermediate, key.Public(), p.IntermediateKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func policy(t *testing.T, id asn1.ObjectIdentifier) x509.OID {
	t.Helper()
	ints := make([]uint64, len(id))
	for i, v := range id {
		ints[i] = uint64(v)
	}
	o, err := x509.OIDFromInts(ints)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func qcExt(t *testing.T, ids ...asn1.ObjectIdentifier) pkix.Extension {
	t.Helper()
	var stmts []qcStatement
	for _, id := range ids {
		s := qcStatement{ID: id}
		if len(id) == 7 { // a QcType value
			info, _ := asn1.Marshal([]asn1.ObjectIdentifier{id})
			s = qcStatement{ID: oidQcType, Info: asn1.RawValue{FullBytes: info}}
		}
		stmts = append(stmts, s)
	}
	v, err := asn1.Marshal(stmts)
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: oidQCStatements, Value: v}
}

func TestParseQC(t *testing.T) {
	p := sigtest.New(t)
	for _, c := range []struct {
		qc   int
		want QC
	}{
		{sigtest.QcSignature, QC{Qualified: true, QSCD: true, Type: TypeSignature}},
		{sigtest.QcSeal, QC{Qualified: true, QSCD: true, Type: TypeSeal}},
		{sigtest.QcNoType, QC{Qualified: true, Type: TypeSignature}},
		{sigtest.QcNone, QC{}},
	} {
		leaf, _ := p.Leaf(t, sigtest.LeafOptions{QC: c.qc})
		if got := ParseQC(leaf); got != c.want {
			t.Errorf("QC %d: %+v, want %+v", c.qc, got, c.want)
		}
	}
	for name, c := range map[string]struct {
		tmpl *x509.Certificate
		want QC
	}{
		"web":               {&x509.Certificate{ExtraExtensions: []pkix.Extension{qcExt(t, oidQcCompliance, oidQcTypeWeb)}}, QC{Qualified: true, Type: TypeWeb}},
		"other legislation": {&x509.Certificate{ExtraExtensions: []pkix.Extension{qcExt(t, oidQcCompliance, oidQcCCLegislate)}}, QC{Type: TypeSignature}},
		"malformed":         {&x509.Certificate{ExtraExtensions: []pkix.Extension{{Id: oidQCStatements, Value: []byte{0x30, 0x03}}}}, QC{}},
		"QCP-n":             {&x509.Certificate{Policies: []x509.OID{policy(t, oidPolicyQCPn)}}, QC{Qualified: true, Type: TypeSignature}},
		"QCP-n-qscd":        {&x509.Certificate{Policies: []x509.OID{policy(t, oidPolicyQCPnQSCD)}}, QC{Qualified: true, QSCD: true, Type: TypeSignature}},
		"QCP-l":             {&x509.Certificate{Policies: []x509.OID{policy(t, oidPolicyQCPl)}}, QC{Qualified: true, Type: TypeSeal}},
		"QCP-l-qscd":        {&x509.Certificate{Policies: []x509.OID{policy(t, oidPolicyQCPlQSCD)}}, QC{Qualified: true, QSCD: true, Type: TypeSeal}},
	} {
		if got := ParseQC(issue(t, p, c.tmpl)); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
	for _, ty := range []Type{TypeUnknown, TypeSignature, TypeSeal, TypeWeb} {
		if ty.String() == "" {
			t.Errorf("Type(%d) has no name", ty)
		}
	}
}

func TestCheckChain(t *testing.T) {
	p := sigtest.New(t)
	leaf, _ := p.Leaf(t, sigtest.LeafOptions{})
	inter := []*x509.Certificate{p.Intermediate}

	r := Check(leaf, inter, &Options{Roots: p.Roots, At: sigtest.Epoch})
	if r.Expired || r.NotYetValid || r.KeyUsage != nil || !r.TrustChecked || r.Untrusted || r.ChainErr != nil || len(r.Chain) != 3 {
		t.Errorf("valid certificate: %+v", r)
	}
	if r.Revocation.Status != StatusUnknown {
		t.Errorf("revocation without data: %v", r.Revocation.Status)
	}
	// The intermediate authority given apart.
	if r := Check(leaf, nil, &Options{Roots: p.Roots, Intermediates: inter, At: sigtest.Epoch}); len(r.Chain) != 3 {
		t.Errorf("intermediate in the options: %+v", r)
	}
	if r := Check(leaf, nil, &Options{Roots: p.Roots, At: sigtest.Epoch}); !r.Untrusted {
		t.Errorf("no intermediate: %+v", r)
	}
	if r := Check(leaf, inter, &Options{At: sigtest.Epoch}); r.TrustChecked || r.Chain != nil {
		t.Errorf("no roots: %+v", r)
	}
	if r := Check(leaf, inter, &Options{Roots: sigtest.New(t).Roots, At: sigtest.Epoch}); !r.Untrusted {
		t.Errorf("roots of another PKI: %+v", r)
	}
	// An expired certificate still tells whether its authority is trusted.
	if r := Check(leaf, inter, &Options{Roots: p.Roots, At: leaf.NotAfter.Add(time.Hour)}); !r.Expired || r.Untrusted || len(r.Chain) != 3 {
		t.Errorf("expired: %+v", r)
	}
	if r := Check(leaf, inter, &Options{Roots: p.Roots, At: leaf.NotBefore.Add(-time.Hour)}); !r.NotYetValid || r.Untrusted {
		t.Errorf("not yet valid: %+v", r)
	}
	encipher, _ := p.Leaf(t, sigtest.LeafOptions{KeyUsage: x509.KeyUsageKeyEncipherment})
	if r := Check(encipher, inter, &Options{Roots: p.Roots, At: sigtest.Epoch}); r.KeyUsage == nil {
		t.Errorf("key usage keyEncipherment: %+v", r)
	}
	// A certificate valid when its authority was not.
	old := issue(t, p, &x509.Certificate{
		NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if r := Check(old, inter, &Options{Roots: p.Roots, At: sigtest.Epoch}); !r.Expired || r.ChainErr == nil {
		t.Errorf("authority not valid: %+v", r)
	}
}

func TestRevocationCRL(t *testing.T) {
	p := sigtest.New(t)
	leaf, _ := p.Leaf(t, sigtest.LeafOptions{})
	inter := []*x509.Certificate{p.Intermediate}
	at := sigtest.Epoch
	check := func(crls ...*x509.RevocationList) Revocation {
		return Check(leaf, inter, &Options{Roots: p.Roots, At: at, CRLs: crls}).Revocation
	}
	day := 24 * time.Hour

	if r := check(p.CRL(t, at, at.Add(-day), false, leaf)); r.Status != StatusRevoked || !r.Time.Equal(at.Add(-day)) || r.Source != "CRL" {
		t.Errorf("revoked: %+v", r)
	}
	if r := check(p.CRL(t, at, at.Add(-day), true, leaf)); r.Status != StatusSuspended {
		t.Errorf("on hold: %+v", r)
	}
	// Revoked after the time checked, by a later CRL: good at that time.
	if r := check(p.CRL(t, at.Add(2*day), at.Add(day), false, leaf)); r.Status != StatusGood {
		t.Errorf("revoked later: %+v", r)
	}
	// A CRL that does not list the certificate, current or later.
	if r := check(p.CRL(t, at.Add(-day), time.Time{}, false)); r.Status != StatusGood {
		t.Errorf("current CRL: %+v", r)
	}
	// A CRL whose next update is before the time checked tells nothing.
	if r := check(p.CRL(t, at.Add(-30*day), time.Time{}, false)); r.Status != StatusUnknown {
		t.Errorf("outdated CRL: %+v", r)
	}
	// Revoked beats good.
	if r := check(p.CRL(t, at.Add(-day), time.Time{}, false), p.CRL(t, at, at.Add(-day), false, leaf)); r.Status != StatusRevoked {
		t.Errorf("good and revoked: %+v", r)
	}
	// A CRL of another authority of the same name does not verify.
	other := sigtest.New(t)
	if r := check(other.CRL(t, at, at.Add(-day), false, leaf)); r.Status != StatusUnknown || len(r.Errors) != 1 {
		t.Errorf("CRL of another authority: %+v", r)
	}
	// Without the issuer, the CRL cannot be checked.
	r := Check(leaf, nil, &Options{At: at, CRLs: []*x509.RevocationList{p.CRL(t, at, at.Add(-day), false, leaf)}}).Revocation
	if r.Status != StatusUnknown || len(r.Errors) != 1 {
		t.Errorf("no issuer: %+v", r)
	}
}

// TestRevocationCRLScope checks the CRLs whose silence does not prove that
// a certificate is not revoked.
func TestRevocationCRLScope(t *testing.T) {
	p := sigtest.New(t)
	const dp = "http://crl.example.it/ca.crl"
	leaf, _ := p.Leaf(t, sigtest.LeafOptions{CRLDistributionPoints: []string{dp}})
	inter := []*x509.Certificate{p.Intermediate}
	at := sigtest.Epoch
	day := 24 * time.Hour
	check := func(c *x509.Certificate, crl *x509.RevocationList) Status {
		return Check(c, inter, &Options{Roots: p.Roots, At: at, CRLs: []*x509.RevocationList{crl}}).Revocation.Status
	}
	ext := func(id asn1.ObjectIdentifier, v any) pkix.Extension {
		b, err := asn1.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return pkix.Extension{Id: id, Value: b}
	}
	crl := func(thisUpdate time.Time, exts ...pkix.Extension) *x509.RevocationList {
		return p.SignCRL(t, &x509.RevocationList{ThisUpdate: thisUpdate, NextUpdate: thisUpdate.Add(7 * day), ExtraExtensions: exts})
	}
	// The DistributionPointName [0] holding fullName [0] with one URI.
	idpDP := func(uri string) asn1.RawValue {
		gn, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte(uri)})
		full, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: gn})
		return asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: full}
	}
	type idp = issuingDistributionPoint

	for _, c := range []struct {
		name string
		crl  *x509.RevocationList
		want Status
	}{
		{"complete CRL", crl(at), StatusGood},
		{"delta CRL", crl(at, ext(oidDeltaCRLIndicator, 1)), StatusUnknown},
		{"only user certificates", crl(at, ext(oidIssuingDistributionPoint, idp{OnlyUserCerts: true})), StatusGood},
		{"only CA certificates", crl(at, ext(oidIssuingDistributionPoint, idp{OnlyCACerts: true})), StatusUnknown},
		{"some reasons only", crl(at, ext(oidIssuingDistributionPoint, idp{OnlySomeReasons: asn1.BitString{Bytes: []byte{0x40}, BitLength: 2}})), StatusUnknown},
		{"indirect CRL", crl(at, ext(oidIssuingDistributionPoint, idp{IndirectCRL: true})), StatusUnknown},
		{"distribution point of the certificate", crl(at, ext(oidIssuingDistributionPoint, idp{DistributionPoint: idpDP(dp)})), StatusGood},
		{"another distribution point", crl(at, ext(oidIssuingDistributionPoint, idp{DistributionPoint: idpDP("http://crl.example.it/2.crl")})), StatusUnknown},
		{"malformed issuing distribution point", crl(at, pkix.Extension{Id: oidIssuingDistributionPoint, Value: []byte{0x30, 0x03, 0x01}}), StatusUnknown},
		// Issued after the certificate expired, the CRL may have dropped it.
		{"issued after the expiry", crl(leaf.NotAfter.Add(day)), StatusUnknown},
		{"issued after the expiry, expired certificates kept", crl(leaf.NotAfter.Add(day), ext(oidExpiredCertsOnCRL, time.Unix(0, 0).UTC())), StatusGood},
	} {
		if got := check(leaf, c.crl); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// A certificate without distribution point is outside a scoped CRL.
	plain, _ := p.Leaf(t, sigtest.LeafOptions{})
	if got := check(plain, crl(at, ext(oidIssuingDistributionPoint, idp{DistributionPoint: idpDP(dp)}))); got != StatusUnknown {
		t.Errorf("no distribution point: %v", got)
	}
	// removeFromCRL takes a certificate off hold.
	removed := p.SignCRL(t, &x509.RevocationList{ThisUpdate: at, NextUpdate: at.Add(7 * day),
		RevokedCertificateEntries: []x509.RevocationListEntry{{SerialNumber: leaf.SerialNumber, RevocationTime: at.Add(-day), ReasonCode: 8}}})
	if got := check(leaf, removed); got != StatusGood {
		t.Errorf("removeFromCRL: %v", got)
	}
}

func TestRevocationOCSP(t *testing.T) {
	p := sigtest.New(t)
	leaf, _ := p.Leaf(t, sigtest.LeafOptions{})
	another, _ := p.Leaf(t, sigtest.LeafOptions{})
	inter := []*x509.Certificate{p.Intermediate}
	at := sigtest.Epoch
	day := 24 * time.Hour
	check := func(resp ...[]byte) Revocation {
		return Check(leaf, inter, &Options{Roots: p.Roots, At: at, OCSP: resp}).Revocation
	}

	if r := check(p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at, false)); r.Status != StatusGood || r.Source != "OCSP" {
		t.Errorf("good: %+v", r)
	}
	if r := check(p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at, true)); r.Status != StatusGood {
		t.Errorf("good, responder by key: %+v", r)
	}
	if r := check(p.OCSP(t, leaf, sigtest.OCSPRevoked, at.Add(-day), at, false)); r.Status != StatusRevoked || !r.Time.Equal(at.Add(-day)) {
		t.Errorf("revoked: %+v", r)
	}
	if r := check(p.OCSP(t, leaf, sigtest.OCSPRevoked, at.Add(day), at.Add(2*day), false)); r.Status != StatusGood {
		t.Errorf("revoked later: %+v", r)
	}
	if r := check(p.OCSP(t, leaf, sigtest.OCSPUnknown, time.Time{}, at, false)); r.Status != StatusUnknown || len(r.Errors) != 0 {
		t.Errorf("unknown: %+v", r)
	}
	// A response produced before the time checked, without next update.
	if r := check(p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at.Add(-day), false)); r.Status != StatusUnknown {
		t.Errorf("old response: %+v", r)
	}
	// A response about another certificate is ignored.
	if r := check(p.OCSP(t, another, sigtest.OCSPRevoked, at.Add(-day), at, false)); r.Status != StatusUnknown || len(r.Errors) != 0 {
		t.Errorf("another certificate: %+v", r)
	}
	// A response signed by another authority of the same name.
	other := sigtest.New(t)
	if r := check(other.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at, false)); r.Status != StatusUnknown {
		t.Errorf("response of another authority: %+v", r)
	}
	for name, resp := range map[string][]byte{
		"garbage":           []byte("garbage"),
		"unsuccessful":      {0x30, 0x03, 0x0a, 0x01, 0x01},
		"not basic":         append([]byte{0x30, 0x0f, 0x0a, 0x01, 0x00, 0xa0, 0x0a, 0x30, 0x08, 0x06, 0x02, 0x2a, 0x03}, 0x04, 0x02, 0x30, 0x00),
		"malformed basic":   mustOCSPWithBasic(t, []byte{0x30, 0x00}),
		"truncated element": p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at, false)[:40],
	} {
		if r := check(resp); r.Status != StatusUnknown || len(r.Errors) != 1 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// Without the issuer, the response cannot be checked.
	r := Check(leaf, nil, &Options{At: at, OCSP: [][]byte{p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, at, false)}}).Revocation
	if r.Status != StatusUnknown || len(r.Errors) != 1 {
		t.Errorf("no issuer: %+v", r)
	}
	// A good status produced after the certificate expired proves
	// nothing: the responder need not keep expired certificates.
	late := leaf.NotAfter.Add(day)
	if r := Check(leaf, inter, &Options{Roots: p.Roots, At: leaf.NotAfter.Add(-day), OCSP: [][]byte{p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, late, false)}}).Revocation; r.Status != StatusUnknown {
		t.Errorf("good after the expiry: %+v", r)
	}

	// Delegated responders.
	ocspSigning := []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning}
	delegated := func(o sigtest.LeafOptions) []byte {
		o.Key = sigtest.ECKey(t)
		o.CommonName = "OCSP responder"
		o.KeyUsage = x509.KeyUsageDigitalSignature
		c, k := p.Leaf(t, o)
		return p.OCSPFrom(t, leaf, sigtest.OCSPGood, time.Time{}, at, c, k)
	}
	if r := check(delegated(sigtest.LeafOptions{ExtKeyUsage: ocspSigning})); r.Status != StatusGood || len(r.Errors) != 0 {
		t.Errorf("delegated responder: %+v", r)
	}
	for name, resp := range map[string][]byte{
		"responder without OCSP signing": delegated(sigtest.LeafOptions{}),
		"responder expired":              delegated(sigtest.LeafOptions{ExtKeyUsage: ocspSigning, NotBefore: at.Add(-2 * day), NotAfter: at.Add(-day)}),
		"responder not yet valid":        delegated(sigtest.LeafOptions{ExtKeyUsage: ocspSigning, NotBefore: at.Add(day), NotAfter: at.Add(2 * day)}),
	} {
		if r := check(resp); r.Status != StatusUnknown || len(r.Errors) != 1 {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func mustOCSPWithBasic(t *testing.T, basic []byte) []byte {
	t.Helper()
	v, err := asn1.Marshal(ocspResponse{Status: 0, Response: ocspResponseBytes{Type: oidOCSPBasic, Response: basic}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
