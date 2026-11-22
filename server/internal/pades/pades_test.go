package pades

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/pdf/pdftest"
	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/validate"
)

// testdata reads a file of the testdata directory of the main module.
func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// facturX returns testdata/host.pdf with the CII invoice of
// EN16931_Einfach embedded.
func facturX(t *testing.T) []byte {
	t.Helper()
	out, err := zerechnung.Embed(testdata(t, "host.pdf"), testdata(t, "EN16931_Einfach.cii.xml"), &zerechnung.EmbedOptions{
		ModDate: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// created returns a one-page PDF with the invoice of EN16931_Einfach
// written by Create, which validates it and converts the PDF to PDF/A-3B
// first (testdata/host.pdf cannot be converted: its font is not embedded).
func created(t *testing.T) []byte {
	t.Helper()
	inv, err := cii.Parse(testdata(t, "EN16931_Einfach.cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := zerechnung.Create(minimal(pdftest.Options{}, "", "", nil), inv, &zerechnung.CreateOptions{
		Validate: &validate.Options{},
		Embed: zerechnung.EmbedOptions{
			ModDate:     time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
			ConvertPDFA: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type signer struct {
	name  string
	key   crypto.Signer
	chain []*x509.Certificate
	hash  crypto.Hash
	pss   bool
}

func signers(t *testing.T) []signer {
	t.Helper()
	pki := sigtest.New(t)
	rsaCert, rsaKey := pki.Leaf(t, sigtest.LeafOptions{CommonName: "RSA signer"})
	ecCert, ecKey := pki.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t), CommonName: "ECDSA signer"})
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384Cert, _ := pki.Leaf(t, sigtest.LeafOptions{Key: p384, CommonName: "ECDSA P-384 signer"})
	chain := func(c *x509.Certificate) []*x509.Certificate { return []*x509.Certificate{c, pki.Intermediate} }
	return []signer{
		{"RSA PKCS#1 v1.5", rsaKey, chain(rsaCert), 0, false},
		{"RSA-PSS SHA-384", rsaKey, chain(rsaCert), crypto.SHA384, true},
		{"ECDSA P-256", ecKey, chain(ecCert), 0, false},
		{"ECDSA P-384 SHA-512", p384, chain(p384Cert), crypto.SHA512, false},
	}
}

func (s signer) options() *Options {
	return &Options{Signer: s.key, Chain: s.chain, Hash: s.hash, PSS: s.pss}
}

// pdfErrors returns the sorted rules of the errors Check finds in the PDF
// container.
func pdfErrors(t *testing.T, data []byte) []string {
	t.Helper()
	r, _, err := zerechnung.Check(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range r.Findings {
		if f.Source == validate.SourcePDF && f.Severity == validate.Error {
			out = append(out, f.Rule+" "+f.Path)
		}
	}
	sort.Strings(out)
	return out
}

func TestSignVerify(t *testing.T) {
	docs := []struct {
		name string
		data []byte
	}{
		{"EN16931_Einfach.pdf", testdata(t, "EN16931_Einfach.pdf")},
		{"host.pdf with Embed", facturX(t)},
		{"Create", created(t)},
	}
	for _, doc := range docs {
		before := pdfErrors(t, doc.data)
		if doc.name == "Create" && len(before) > 0 {
			t.Fatalf("%s: PDF errors before signing:\n%s", doc.name, strings.Join(before, "\n"))
		}
		att, err := zerechnung.Extract(doc.data)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range signers(t) {
			t.Run(doc.name+"/"+s.name, func(t *testing.T) {
				o := s.options()
				o.Time = time.Date(2026, 11, 21, 9, 30, 15, 0, time.FixedZone("CET", 3600))
				o.Reason, o.Location, o.Name = "Facture émise", "Paris", "Acme"
				out, err := Sign(doc.data, o)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(out, doc.data) {
					t.Fatal("the original bytes changed")
				}
				sigs, err := Verify(out)
				if err != nil {
					t.Fatal(err)
				}
				if len(sigs) != 1 {
					t.Fatalf("Verify found %d signatures, want 1", len(sigs))
				}
				g := sigs[0]
				if g.Field != "Signature1" || g.SubFilter != "ETSI.CAdES.detached" || !g.WholeDocument {
					t.Errorf("signature %+v", g)
				}
				if !g.Signer.Equal(s.chain[0]) {
					t.Errorf("signer %v, want %v", g.Signer.Subject, s.chain[0].Subject)
				}
				if !g.Time.Equal(o.Time) || g.Reason != o.Reason || g.Location != o.Location || g.Name != o.Name {
					t.Errorf("time %v reason %q location %q name %q", g.Time, g.Reason, g.Location, g.Name)
				}
				// The invoice is still there, unchanged.
				got, err := zerechnung.Extract(out)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.XML, att.XML) {
					t.Error("the XML invoice changed")
				}
				if _, _, err := zerechnung.Read(out); err != nil {
					t.Error(err)
				}
				// No new PDF/A error.
				if after := pdfErrors(t, out); strings.Join(after, "\n") != strings.Join(before, "\n") {
					t.Errorf("PDF errors before signing:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
				}
			})
		}
	}
}

// TestCMS checks the attributes of the signature: PAdES Baseline B-B
// keeps the signing time out of the CMS (ETSI EN 319 142-1, 6.3).
func TestCMS(t *testing.T) {
	s := signers(t)[0]
	out, err := Sign(facturX(t), s.options())
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdf.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	form, _ := r.Dict(r.Catalog().Get("AcroForm"))
	if f, _ := pdf.AsInt(r.Resolve(form.Get("SigFlags"))); f != 3 {
		t.Errorf("SigFlags %d, want 3", f)
	}
	fields, _ := r.Resolve(form.Get("Fields")).(pdf.Array)
	field, _ := r.Dict(fields[len(fields)-1])
	if f, _ := pdf.AsInt(field.Get("F")); f != 132 {
		t.Errorf("widget flags %d, want 132", f)
	}
	v, _ := r.Dict(field.Get("V"))
	if f, _ := v.Name("Filter"); f != "Adobe.PPKLite" {
		t.Errorf("Filter %q", f)
	}
	sd, err := cms.Parse(v.Get("Contents").(pdf.String))
	if err != nil {
		t.Fatal(err)
	}
	si := sd.Signers[0]
	if !sd.Detached || !si.SigningTime.IsZero() || si.SigningCert == nil || !si.SigningCert.V2 {
		t.Errorf("detached %v, signing time %v, signing certificate %+v", sd.Detached, si.SigningTime, si.SigningCert)
	}
	if len(sd.Certificates) != 2 {
		t.Errorf("%d certificates, want the chain of 2", len(sd.Certificates))
	}
}

func TestTwoSignatures(t *testing.T) {
	ss := signers(t)
	once, err := Sign(facturX(t), ss[0].options())
	if err != nil {
		t.Fatal(err)
	}
	o := ss[2].options()
	o.Reason = "Approved"
	twice, err := Sign(once, o)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(twice, once) {
		t.Fatal("the second signature changed the bytes of the first")
	}
	sigs, err := Verify(twice)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 2 {
		t.Fatalf("%d signatures, want 2", len(sigs))
	}
	if sigs[0].Field != "Signature1" || sigs[0].WholeDocument || !sigs[0].Signer.Equal(ss[0].chain[0]) {
		t.Errorf("first signature %+v", sigs[0])
	}
	if sigs[1].Field != "Signature2" || !sigs[1].WholeDocument || sigs[1].Reason != "Approved" || !sigs[1].Signer.Equal(ss[2].chain[0]) {
		t.Errorf("second signature %+v", sigs[1])
	}
	if _, err := Sign(once, &Options{Signer: ss[0].key, Chain: ss[0].chain, FieldName: "Signature1"}); err == nil {
		t.Error("a field name already taken was accepted")
	}
}

func TestTampering(t *testing.T) {
	s := signers(t)[0]
	doc := facturX(t)
	out, err := Sign(doc, s.options())
	if err != nil {
		t.Fatal(err)
	}
	contents := bytes.LastIndex(out, []byte("/Contents <")) + len("/Contents <")
	for _, at := range []int{100, len(doc) / 2, len(doc) + 10, contents + 40, len(out) - 10} {
		bad := bytes.Clone(out)
		switch {
		case bad[at] >= '0' && bad[at] <= '8':
			bad[at]++
		case bad[at] == '9':
			bad[at] = '8'
		default:
			bad[at] ^= 0x20
		}
		if _, err := Verify(bad); err == nil {
			t.Errorf("a change at byte %d (of %d) was not detected", at, len(out))
		}
	}
	// Bytes appended after the signature are not covered by it.
	sigs, err := Verify(append(bytes.Clone(out), "\n% comment\n"...))
	if err != nil {
		t.Fatal(err)
	}
	if sigs[0].WholeDocument {
		t.Error("a signature that does not cover the end of the file is reported as covering the whole document")
	}
}

// minimal builds a one-page PDF with pdftest.
func minimal(o pdftest.Options, catalog, page string, extra func(b *pdftest.Builder) (catalog, page string)) []byte {
	b := &pdftest.Builder{Trailer: "/Root 1 0 R /ID [<0102> <0102>]"}
	b.Reserve() // 1: catalog
	b.Reserve() // 2: pages
	b.Reserve() // 3: page
	content := b.AddStream("", []byte("q Q"))
	if extra != nil {
		c, p := extra(b)
		catalog += c
		page += p
	}
	b.Set(1, "<< /Type /Catalog /Pages 2 0 R "+catalog+">>")
	b.Set(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	b.Set(3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R %s>>", content, page))
	return b.Bytes(o)
}

func TestLayouts(t *testing.T) {
	s := signers(t)[2]
	for _, tc := range []struct {
		name   string
		o      pdftest.Options
		stream bool
	}{
		{"classic", pdftest.Options{}, false},
		{"xref stream", pdftest.Options{Layout: pdftest.XrefStream}, true},
		{"xref stream and object stream", pdftest.Options{Layout: pdftest.XrefStream, ObjStm: true, Predictor: true}, true},
		{"hybrid", pdftest.Options{Layout: pdftest.Hybrid, ObjStm: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := minimal(tc.o, "", "", nil)
			out, err := Sign(doc, s.options())
			if err != nil {
				t.Fatal(err)
			}
			r, err := pdf.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			if r.Repaired() {
				t.Error("the signed file needs a repair")
			}
			if r.XrefStream() != tc.stream {
				t.Errorf("cross-reference stream %v, want %v", r.XrefStream(), tc.stream)
			}
			sigs, err := Verify(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(sigs) != 1 || !sigs[0].WholeDocument {
				t.Errorf("signatures %+v", sigs)
			}
		})
	}
}

func TestExistingForm(t *testing.T) {
	s := signers(t)[0]
	text := "<< /FT /Tx /T (Signature1) /Rect [0 0 0 0] /F 4 >>"
	for _, tc := range []struct {
		name  string
		extra func(b *pdftest.Builder) (string, string)
	}{
		{"direct form", func(b *pdftest.Builder) (string, string) {
			f := b.Add(text)
			return fmt.Sprintf("/AcroForm << /Fields [%d 0 R] /SigFlags 0 >> ", f), fmt.Sprintf("/Annots [%d 0 R] ", f)
		}},
		{"indirect form, fields and annotations", func(b *pdftest.Builder) (string, string) {
			f := b.Add(text)
			fields := b.Add(fmt.Sprintf("[%d 0 R]", f))
			form := b.Add(fmt.Sprintf("<< /Fields %d 0 R >>", fields))
			annots := b.Add(fmt.Sprintf("[%d 0 R]", f))
			return fmt.Sprintf("/AcroForm %d 0 R ", form), fmt.Sprintf("/Annots %d 0 R ", annots)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Sign(minimal(pdftest.Options{}, "", "", tc.extra), s.options())
			if err != nil {
				t.Fatal(err)
			}
			sigs, err := Verify(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(sigs) != 1 || sigs[0].Field != "Signature2" {
				t.Fatalf("signatures %+v", sigs)
			}
			r, _ := pdf.Open(out)
			form, _ := r.Dict(r.Catalog().Get("AcroForm"))
			fields, _ := r.Resolve(form.Get("Fields")).(pdf.Array)
			if len(fields) != 2 {
				t.Errorf("%d fields, want 2", len(fields))
			}
			if f, _ := pdf.AsInt(r.Resolve(form.Get("SigFlags"))); f != 3 {
				t.Errorf("SigFlags %d, want 3", f)
			}
			page, _ := r.Dict(r.Object(3))
			annots, _ := r.Resolve(page.Get("Annots")).(pdf.Array)
			if len(annots) != 2 {
				t.Errorf("%d annotations, want 2", len(annots))
			}
		})
	}
}

func TestNoSignature(t *testing.T) {
	sigs, err := Verify(testdata(t, "EN16931_Einfach.pdf"))
	if err != nil || len(sigs) != 0 {
		t.Errorf("Verify of an unsigned document: %v, %v", sigs, err)
	}
	if _, err := Verify([]byte("not a pdf")); err == nil {
		t.Error("Verify accepted a file that is not a PDF")
	}
}

func TestErrors(t *testing.T) {
	ss := signers(t)
	doc := minimal(pdftest.Options{}, "", "", nil)
	locked := minimal(pdftest.Options{}, "", "", func(b *pdftest.Builder) (string, string) {
		sig := b.Add("<< /Type /Sig /Reference [<< /TransformMethod /DocMDP /TransformParams << /P 1 >> >>] >>")
		return fmt.Sprintf("/Perms << /DocMDP %d 0 R >> ", sig), ""
	})
	allowed := minimal(pdftest.Options{}, "", "", func(b *pdftest.Builder) (string, string) {
		sig := b.Add("<< /Type /Sig /Reference [<< /TransformMethod /DocMDP /TransformParams << /P 2 >> >>] >>")
		return fmt.Sprintf("/Perms << /DocMDP %d 0 R >> ", sig), ""
	})
	encrypted := &pdftest.Builder{Trailer: "/Root 1 0 R /Encrypt 2 0 R"}
	encrypted.Add("<< /Type /Catalog >>")
	encrypted.Add("<< /Filter /Standard >>")

	for _, tc := range []struct {
		name string
		data []byte
		o    *Options
		want error
	}{
		{"no options", doc, nil, nil},
		{"no signer", doc, &Options{Chain: ss[0].chain}, nil},
		{"no certificate", doc, &Options{Signer: ss[0].key}, nil},
		{"key and certificate differ", doc, &Options{Signer: ss[0].key, Chain: ss[2].chain}, nil},
		{"SHA-1", doc, &Options{Signer: ss[0].key, Chain: ss[0].chain, Hash: crypto.SHA1}, nil},
		{"PSS with ECDSA", doc, &Options{Signer: ss[2].key, Chain: ss[2].chain, PSS: true}, nil},
		{"period in the field name", doc, &Options{Signer: ss[0].key, Chain: ss[0].chain, FieldName: "a.b"}, nil},
		{"not a PDF", []byte("hello"), ss[0].options(), nil},
		{"encrypted", encrypted.Bytes(pdftest.Options{}), ss[0].options(), ErrEncrypted},
		{"certified without changes", locked, ss[0].options(), ErrLocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Sign(tc.data, tc.o)
			if err == nil {
				t.Fatal("no error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("error %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := Sign(allowed, ss[0].options()); err != nil {
		t.Errorf("a document certified with form filling and signing allowed: %v", err)
	}
}

// TestReserve checks that the reserved size bounds the real signature for
// each key, ECDSA signatures varying in length.
func TestReserve(t *testing.T) {
	for _, s := range signers(t) {
		o := s.options()
		size, err := reserve(o, mustAlg(t, o))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			der, err := cms.Sign([]byte{byte(i)}, &cms.SignOptions{Signer: s.key, Chain: s.chain, Hash: s.hash, PSS: s.pss, Detached: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(der) > size-contentsMargin {
				t.Fatalf("%s: signature of %d bytes, more than the trial %d", s.name, len(der), size-contentsMargin)
			}
		}
	}
}

func mustAlg(t *testing.T, o *Options) sigalg.Algorithm {
	t.Helper()
	a, err := sigalg.For(o.Chain[0].PublicKey, o.Hash, o.PSS)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestWriteSamples writes signed files to the directory named by
// PADES_OUT, for checks with other tools (pdfsig, openssl, veraPDF).
func TestWriteSamples(t *testing.T) {
	dir := os.Getenv("PADES_OUT")
	if dir == "" {
		t.Skip("PADES_OUT not set")
	}
	ss := signers(t)
	doc := facturX(t)
	var once []byte
	for i, s := range ss {
		o := s.options()
		o.Reason = "Facture émise"
		out, err := Sign(doc, o)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			once = out
		}
		name := strings.NewReplacer(" ", "-", "#", "", ".", "").Replace(s.name)
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	twice, err := Sign(once, ss[2].options())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two-signatures.pdf"), twice, 0o644); err != nil {
		t.Fatal(err)
	}
	einfach, err := Sign(testdata(t, "EN16931_Einfach.pdf"), ss[0].options())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EN16931_Einfach-signed.pdf"), einfach, 0o644); err != nil {
		t.Fatal(err)
	}
}
