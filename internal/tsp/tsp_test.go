package tsp_test

import (
	"bytes"
	"crypto"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"testing"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/internal/tsp"
	"github.com/ritoon/zerechnung/internal/tsp/tsptest"
)

var data = []byte("the signature value")

func stamp(t *testing.T, a *tsptest.Authority, digest []byte, h crypto.Hash) []byte {
	t.Helper()
	der, err := a.Timestamp(digest, h)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestParseVerify(t *testing.T) {
	p := sigtest.New(t)
	a := tsptest.New(t, p)
	a.Time = sigtest.Epoch
	digest := sha256.Sum256(data)
	for _, response := range []bool{false, true} {
		a.Response = response
		der := stamp(t, a, digest[:], crypto.SHA256)
		tok, err := tsp.Parse(der)
		if err != nil {
			t.Fatalf("response %v: %v", response, err)
		}
		if response == bytes.Equal(tok.DER, der) {
			t.Errorf("response %v: the token is the whole input %v", response, bytes.Equal(tok.DER, der))
		}
		if tok.Hash != crypto.SHA256 || !bytes.Equal(tok.Digest, digest[:]) || !tok.Time.Equal(sigtest.Epoch) ||
			!tok.Policy.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}) || tok.Serial.Sign() <= 0 {
			t.Errorf("token %+v", tok)
		}
		if !tok.Certificate().Equal(a.Cert) {
			t.Errorf("certificate %v", tok.Certificate().Subject)
		}
		if err := tok.Verify(data); err != nil {
			t.Errorf("Verify: %v", err)
		}
		if err := tok.Verify([]byte("other data")); !errors.Is(err, tsp.ErrImprint) {
			t.Errorf("Verify of other data: %v", err)
		}
	}
	var nilToken *tsp.Token
	if nilToken.Certificate() != nil {
		t.Error("a nil token has a certificate")
	}
}

func TestVerifyErrors(t *testing.T) {
	p := sigtest.New(t)
	a := tsptest.New(t, p)
	digest := sha256.Sum256(data)

	// SHA-1 is refused as the hash of the imprint.
	d1 := sha1.Sum(data)
	tok, err := tsp.Parse(stamp(t, a, d1[:], crypto.SHA1))
	if err != nil {
		t.Fatal(err)
	}
	if err := tok.Verify(data); !errors.Is(err, tsp.ErrImprint) {
		t.Errorf("SHA-1 imprint: %v", err)
	}

	// A certificate without the time-stamping extended key usage.
	leaf, key := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	other := &tsptest.Authority{Cert: leaf, Key: key, Chain: []*x509.Certificate{p.Intermediate}}
	if tok, err = tsp.Parse(stamp(t, other, digest[:], crypto.SHA256)); err != nil {
		t.Fatal(err)
	}
	if err := tok.Verify(data); !errors.Is(err, tsp.ErrNotTSA) {
		t.Errorf("certificate not issued for time-stamping: %v", err)
	}

	// A TSTInfo changed after signing: the policy 1.3.6.1.4.1.99999.1
	// becomes .2.
	der := stamp(t, a, digest[:], crypto.SHA256)
	policy, _ := asn1.Marshal(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1})
	i := bytes.Index(der, policy)
	if i < 0 {
		t.Fatal("policy not found in the token")
	}
	bad := bytes.Clone(der)
	bad[i+len(policy)-1] = 2
	if tok, err = tsp.Parse(bad); err != nil {
		t.Fatal(err)
	}
	if err := tok.Verify(data); !errors.Is(err, tsp.ErrSignature) {
		t.Errorf("changed TSTInfo: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	p := sigtest.New(t)
	a := tsptest.New(t, p)
	digest := sha256.Sum256(data)
	token := stamp(t, a, digest[:], crypto.SHA256)
	leaf, key := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	notInfo, err := cms.Sign(data, &cms.SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}})
	if err != nil {
		t.Fatal(err)
	}
	detached, err := cms.Sign(data, &cms.SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, ContentType: cms.OIDTSTInfo, Detached: true})
	if err != nil {
		t.Fatal(err)
	}
	badInfo, err := cms.Sign([]byte{0x30, 0x03, 0x02, 0x01, 0x02}, &cms.SignOptions{Signer: key, Chain: []*x509.Certificate{leaf}, ContentType: cms.OIDTSTInfo})
	if err != nil {
		t.Fatal(err)
	}
	response := func(status int, tok []byte) []byte {
		st, _ := asn1.Marshal(struct{ Status int }{status})
		v := struct {
			Status asn1.RawValue
			Token  asn1.RawValue `asn1:"optional"`
		}{Status: asn1.RawValue{FullBytes: st}}
		if tok != nil {
			v.Token = asn1.RawValue{FullBytes: tok}
		}
		der, err := asn1.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
	if _, err := tsp.Parse(response(1, token)); err != nil {
		t.Errorf("response granted with modifications: %v", err)
	}
	for name, der := range map[string][]byte{
		"empty":               nil,
		"not DER":             []byte("token"),
		"not a SEQUENCE":      {0x02, 0x01, 0x01},
		"rejected":            response(2, nil),
		"no token":            response(0, nil),
		"not a TSTInfo":       notInfo,
		"detached":            detached,
		"version 2 TSTInfo":   badInfo,
		"truncated":           token[:len(token)/2],
		"response, truncated": response(0, token)[:40],
	} {
		if _, err := tsp.Parse(der); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// FuzzParse checks that Parse and Verify never panic on a damaged token.
func FuzzParse(f *testing.F) {
	p := sigtest.New(f)
	a := tsptest.New(f, p)
	digest := sha256.Sum256(data)
	for _, response := range []bool{false, true} {
		a.Response = response
		der, err := a.Timestamp(digest[:], crypto.SHA256)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(der)
	}
	f.Fuzz(func(t *testing.T, der []byte) {
		if tok, err := tsp.Parse(der); err == nil {
			_ = tok.Verify(data)
			_ = tok.Certificate()
		}
	})
}
