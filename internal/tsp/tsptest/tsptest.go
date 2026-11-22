// Package tsptest is a time-stamping authority for tests: it issues RFC
// 3161 time-stamp tokens with a certificate of a sigtest PKI.
package tsptest

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

// Authority issues time-stamp tokens.
type Authority struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	// Chain is written in the tokens after Cert.
	Chain []*x509.Certificate
	// Time is the genTime of the tokens; time.Now() when zero.
	Time time.Time
	// Response wraps the tokens in a TimeStampResp.
	Response bool
	serial   atomic.Int64
}

// New returns an authority whose certificate the intermediate authority of
// p issues.
func New(tb testing.TB, p *sigtest.PKI) *Authority {
	tb.Helper()
	cert, key := p.TSA(tb)
	return &Authority{Cert: cert, Key: key, Chain: []*x509.Certificate{p.Intermediate}}
}

// policy is the policy of the test authority.
var policy = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}

// Timestamp returns a token over digest, computed with h: the signature of
// a zerechnung.Timestamper.
func (a *Authority) Timestamp(digest []byte, h crypto.Hash) ([]byte, error) {
	oid, ok := sigalg.OIDFromHash(h)
	if !ok {
		return nil, errors.New("tsptest: unsupported hash")
	}
	at := a.Time
	if at.IsZero() {
		at = time.Now()
	}
	info, err := asn1.Marshal(struct {
		Version int
		Policy  asn1.ObjectIdentifier
		Imprint struct {
			Algorithm pkix.AlgorithmIdentifier
			Digest    []byte
		}
		Serial *big.Int
		Time   time.Time `asn1:"generalized"`
	}{
		Version: 1,
		Policy:  policy,
		Imprint: struct {
			Algorithm pkix.AlgorithmIdentifier
			Digest    []byte
		}{pkix.AlgorithmIdentifier{Algorithm: oid}, digest},
		Serial: big.NewInt(a.serial.Add(1)),
		Time:   at.UTC().Truncate(time.Second),
	})
	if err != nil {
		return nil, err
	}
	token, err := cms.Sign(info, &cms.SignOptions{
		Signer:      a.Key,
		Chain:       append([]*x509.Certificate{a.Cert}, a.Chain...),
		ContentType: cms.OIDTSTInfo,
	})
	if err != nil {
		return nil, err
	}
	if !a.Response {
		return token, nil
	}
	status, _ := asn1.Marshal(struct{ Status int }{0})
	return asn1.Marshal(struct {
		Status asn1.RawValue
		Token  asn1.RawValue
	}{asn1.RawValue{FullBytes: status}, asn1.RawValue{FullBytes: token}})
}
