package fatturapa_test

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/sigtest"
)

// TestConcurrentSign signs and verifies documents from many goroutines that
// share the same keys, options and inputs. The verification report of each
// result must equal the sequential one, and so must the signed file when
// signing is deterministic: an RSA CAdES signature is, whereas ECDSA draws a
// random nonce and XAdES a random identifier for ds:Signature.
func TestConcurrentSign(t *testing.T) {
	p := sigtest.New(t)
	rsaCert, rsaKey := p.Leaf(t, sigtest.LeafOptions{QC: sigtest.QcSignature})
	ecCert, ecKey := p.Leaf(t, sigtest.LeafOptions{QC: sigtest.QcSignature, Key: sigtest.ECKey(t)})
	verify := &fatturapa.VerifyOptions{Roots: p.Roots, At: sigtest.Epoch}
	signers := []struct {
		name string
		opts *fatturapa.SignOptions
	}{
		{"RSA", &fatturapa.SignOptions{Signer: rsaKey, Chain: []*x509.Certificate{rsaCert, p.Intermediate}, SigningTime: sigtest.Epoch}},
		{"ECDSA", &fatturapa.SignOptions{Signer: ecKey, Chain: []*x509.Certificate{ecCert, p.Intermediate}, SigningTime: sigtest.Epoch}},
	}
	formats := []struct {
		name string
		sign func([]byte, *fatturapa.SignOptions) ([]byte, error)
	}{{"XAdES", fatturapa.SignXAdES}, {"CAdES", fatturapa.SignCAdES}}

	var docs, pristine [][]byte
	for _, n := range []string{"fpa12.xml", "fpr12.xml"} {
		b, err := os.ReadFile(filepath.Join("testdata", n))
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, b)
		pristine = append(pristine, bytes.Clone(b))
	}

	type op struct {
		name          string
		run           func() (signed []byte, report string)
		deterministic bool
	}
	var ops []op
	for _, s := range signers {
		for _, f := range formats {
			for i, doc := range docs {
				run := func() ([]byte, string) {
					out, err := f.sign(doc, s.opts)
					if err != nil {
						return nil, err.Error()
					}
					return out, fatturapa.VerifySignature(out, verify).String()
				}
				a, _ := run()
				b, report := run()
				if a == nil || b == nil {
					t.Fatalf("%s %s, document %d: %s", s.name, f.name, i, report)
				}
				ops = append(ops, op{fmt.Sprintf("%s %s %d", s.name, f.name, i), run, bytes.Equal(a, b)})
			}
		}
	}

	wantSigned := make([][]byte, len(ops))
	wantReport := make([]string, len(ops))
	for i, o := range ops {
		wantSigned[i], wantReport[i] = o.run()
	}

	workers := 4 * runtime.GOMAXPROCS(0)
	const rounds = 3
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		mismatch = map[string]int{}
		start    = make(chan struct{})
	)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for r := range rounds {
				for k := range ops {
					i := (k + w*5 + r) % len(ops)
					signed, report := ops[i].run()
					if report != wantReport[i] || ops[i].deterministic && !bytes.Equal(signed, wantSigned[i]) {
						mu.Lock()
						mismatch[ops[i].name]++
						mu.Unlock()
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	for name, n := range mismatch {
		t.Errorf("%s: %d concurrent results differ from the sequential one", name, n)
	}
	for i := range docs {
		if !bytes.Equal(docs[i], pristine[i]) {
			t.Errorf("document %d was modified", i)
		}
	}
}
