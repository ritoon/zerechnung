package zerechnung

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/sigtest"
	"github.com/ritoon/zerechnung/internal/tsp/tsptest"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// signHost returns a Factur-X PDF/A-3 file that the tests sign: a host
// converted to PDF/A-3 with an EN 16931 invoice.
func signHost(t *testing.T) []byte {
	t.Helper()
	host := convHost{content: "0.2 0.4 0.6 rg 10 10 100 100 re f"}.build()
	out, err := Embed(host, ciiXML(invoice.URNEN16931), &EmbedOptions{ModDate: testDate, ConvertPDFA: true})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// signer returns the options of a signature by a qualified certificate of
// the test PKI.
func signer(t *testing.T, p *sigtest.PKI, lo sigtest.LeafOptions) *SignOptions {
	t.Helper()
	if lo.QC == 0 {
		lo.QC = sigtest.QcSignature
	}
	cert, key := p.Leaf(t, lo)
	return &SignOptions{Signer: key, Chain: []*x509.Certificate{cert, p.Intermediate}, Time: sigtest.Epoch, Reason: "Facture", Location: "Paris"}
}

// verified returns the report and the signatures of a signed PDF, checked
// at sigtest.Epoch with the roots of p.
func verified(t *testing.T, data []byte, p *sigtest.PKI) (*validate.Report, []*Signature) {
	t.Helper()
	trust := &validate.TrustOptions{At: sigtest.Epoch}
	if p != nil {
		trust.Roots = p.Roots
	}
	r, sigs, err := VerifySignatures(data, trust)
	if err != nil {
		t.Fatal(err)
	}
	return r, sigs
}

// problems returns the errors and warnings of a report.
func problems(r *validate.Report) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Severity != validate.Info {
			out = append(out, f.String())
		}
	}
	return out
}

func TestSignVerify(t *testing.T) {
	p := sigtest.New(t)
	host := signHost(t)
	before, _, err := Check(host, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		key  crypto.Signer
		pss  bool
		hash crypto.Hash
	}{
		{name: "RSA"},
		{name: "RSA-PSS", pss: true},
		{name: "RSA SHA-512", hash: crypto.SHA512},
		{name: "ECDSA", key: sigtest.ECKey(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := signer(t, p, sigtest.LeafOptions{Key: tc.key})
			o.PSS, o.Hash = tc.pss, tc.hash
			out, err := Sign(host, o)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(out, host) {
				t.Fatal("not an incremental update")
			}
			r, sigs := verified(t, out, p)
			if got := problems(r); len(got) > 0 {
				t.Errorf("findings: %q", got)
			}
			if len(sigs) != 1 {
				t.Fatalf("%d signatures", len(sigs))
			}
			s := sigs[0]
			if !s.Valid || !s.WholeDocument || s.Level != LevelBB || s.Field != "Signature1" ||
				s.Kind != KindSignature || !s.Qualified || !s.QSCD || s.SubFilter != "ETSI.CAdES.detached" ||
				!s.Time.Equal(sigtest.Epoch) || s.Reason != "Facture" || s.Location != "Paris" || s.Certification != NoCertification {
				t.Errorf("signature: %+v", s)
			}
			if !r.Has("SIG-14") || !r.Has("SIG-12") {
				t.Errorf("rules %v, want SIG-12 and SIG-14", r.Rules())
			}

			// Signing adds no problem to the container: the PDF/A checks
			// still pass.
			after, att, err := Check(out, &validate.Options{Trust: validate.TrustOptions{Roots: p.Roots, At: sigtest.Epoch}})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := problems(after), problems(before); !slices.Equal(got, want) {
				t.Errorf("Check after signing:\n%q\nbefore:\n%q", got, want)
			}
			if att == nil || !bytes.Equal(att.XML, ciiXML(invoice.URNEN16931)) {
				t.Error("the invoice is not read back")
			}
		})
	}
}

func TestSignUnsigned(t *testing.T) {
	r, sigs, err := VerifySignatures(signHost(t), nil)
	if err != nil || len(sigs) != 0 || len(r.Findings) != 0 {
		t.Errorf("unsigned PDF: %v, %v, %v", r.Findings, sigs, err)
	}
	if _, _, err := VerifySignatures([]byte("not a PDF"), nil); err == nil {
		t.Error("no error for a file that is not a PDF")
	}
}

func TestSignRefused(t *testing.T) {
	p := sigtest.New(t)
	host := signHost(t)
	o := signer(t, p, sigtest.LeafOptions{})
	other, _ := p.Leaf(t, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	for _, tc := range []struct {
		name string
		opts *SignOptions
	}{
		{"nil options", nil},
		{"no chain", &SignOptions{Signer: o.Signer}},
		{"key of another certificate", &SignOptions{Signer: o.Signer, Chain: []*x509.Certificate{other}}},
		{"SHA-1", &SignOptions{Signer: o.Signer, Chain: o.Chain, Hash: crypto.SHA1}},
		{"field name with a period", &SignOptions{Signer: o.Signer, Chain: o.Chain, FieldName: "a.b"}},
		{"unknown certification", &SignOptions{Signer: o.Signer, Chain: o.Chain, Certify: 7}},
	} {
		if _, err := Sign(host, tc.opts); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
	signed, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(signed, &SignOptions{Signer: o.Signer, Chain: o.Chain, FieldName: "Signature1"}); err == nil {
		t.Error("a field name taken: no error")
	}
	// A second signature takes the next field name.
	twice, err := Sign(signed, o)
	if err != nil {
		t.Fatal(err)
	}
	r, sigs := verified(t, twice, p)
	if len(sigs) != 2 || sigs[1].Field != "Signature2" || sigs[0].WholeDocument || !sigs[1].WholeDocument {
		t.Fatalf("two signatures: %+v", sigs)
	}
	if got := problems(r); len(got) > 0 {
		t.Errorf("a second signature is not a modification: %q", got)
	}
	if !slices.Equal(sigs[0].Changes, []string{"signature fields"}) {
		t.Errorf("changes after the first signature: %q", sigs[0].Changes)
	}
}

func TestSignTampered(t *testing.T) {
	p := sigtest.New(t)
	out, err := Sign(signHost(t), signer(t, p, sigtest.LeafOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	// A byte of the binary comment of the header: the file still reads,
	// but no longer matches what was signed.
	bad := bytes.Clone(out)
	bad[10] ^= 0x01
	r, sigs := verified(t, bad, p)
	if !r.Has("SIG-03") || len(sigs) != 1 || sigs[0].Valid || sigs[0].Level != "" {
		t.Errorf("tampered file: %q, %+v", problems(r), sigs)
	}

	// A byte range that also excludes the bytes before /Contents.
	i := bytes.LastIndex(out, []byte("/ByteRange ["))
	j := bytes.IndexByte(out[i:], ']')
	var a, b, c int64
	if _, err := fmtSscan(string(out[i+len("/ByteRange ["):i+j]), &a, &b, &c); err != nil {
		t.Fatal(err)
	}
	moved := bytes.Clone(out)
	br := []byte("[0 " + itoa(a-1) + " " + itoa(b) + " " + itoa(c))
	br = append(br, bytes.Repeat([]byte(" "), j-len("/ByteRange ")-len(br))...)
	copy(moved[i+len("/ByteRange "):], br)
	if r, _ := verified(t, moved, p); !r.Has("SIG-02") {
		t.Errorf("byte range off by one: %q", problems(r))
	}

	// A byte range that stops before the end of the revision.
	short := bytes.Clone(out)
	br = []byte("[0 " + itoa(a) + " " + itoa(b) + " " + itoa(c-10))
	br = append(br, bytes.Repeat([]byte(" "), j-len("/ByteRange ")-len(br))...)
	copy(short[i+len("/ByteRange "):], br)
	if r, _ := verified(t, short, p); !r.Has("SIG-02") {
		t.Errorf("byte range short of the end: %q", problems(r))
	}

	// A /Contents that does not hold a CMS signature.
	k := bytes.LastIndex(out, []byte("/Contents <")) + len("/Contents <")
	garbage := bytes.Clone(out)
	copy(garbage[k:], "4142")
	if r, _ := verified(t, garbage, p); !r.Has("SIG-01") {
		t.Errorf("garbage /Contents: %q", problems(r))
	}
}

func TestSignThenEmbed(t *testing.T) {
	p := sigtest.New(t)
	signed, err := Sign(signHost(t), signer(t, p, sigtest.LeafOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Embed(signed, ciiXML(invoice.URNBasic), &EmbedOptions{ModDate: testDate})
	if err != nil {
		t.Fatalf("Embed on a signed PDF: %v", err)
	}
	r, sigs := verified(t, out, p)
	if len(sigs) != 1 || !sigs[0].Valid || sigs[0].WholeDocument {
		t.Fatalf("signatures: %+v", sigs)
	}
	if !r.Has("SIG-05") {
		t.Errorf("no SIG-05: %q", problems(r))
	}
	for _, want := range []string{"embedded files", "XMP metadata"} {
		if !slices.Contains(sigs[0].Changes, want) {
			t.Errorf("changes %q lack %q", sigs[0].Changes, want)
		}
	}
	// Check reports it too.
	cr, _, err := Check(out, nil)
	if err != nil || !cr.Has("SIG-05") {
		t.Errorf("Check: %v, %v", cr.Rules(), err)
	}
}

func TestEmbedSign(t *testing.T) {
	p := sigtest.New(t)
	host := convHost{content: "0 0 1 rg 0 0 50 50 re f"}.build()
	x := ciiXML(invoice.URNEN16931)
	o := signer(t, p, sigtest.LeafOptions{})
	inv, err := cii.Parse(x)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Create(host, inv, &CreateOptions{Embed: EmbedOptions{ModDate: testDate, ConvertPDFA: true, Sign: o}})
	if err != nil {
		t.Fatal(err)
	}
	r, sigs := verified(t, out, p)
	if got := problems(r); len(got) > 0 || len(sigs) != 1 || !sigs[0].WholeDocument {
		t.Errorf("Create with Sign: %q, %+v", got, sigs)
	}
	out, err = Embed(host, x, &EmbedOptions{ModDate: testDate, ConvertPDFA: true, Sign: o})
	if err != nil {
		t.Fatal(err)
	}
	if att, err := Extract(out); err != nil || !bytes.Equal(att.XML, x) {
		t.Errorf("Extract after Embed with Sign: %v", err)
	}
	if _, sigs := verified(t, out, p); len(sigs) != 1 || !sigs[0].Valid {
		t.Errorf("Embed with Sign: %+v", sigs)
	}
}

func TestCertify(t *testing.T) {
	p := sigtest.New(t)
	host := signHost(t)
	ts := tsptest.New(t, p)
	ts.Time = sigtest.Epoch.Add(time.Hour)

	o := signer(t, p, sigtest.LeafOptions{QC: sigtest.QcSeal, CommonName: "ACME SA"})
	o.Certify = CertifyNoChanges
	locked, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	r, sigs := verified(t, locked, p)
	if got := problems(r); len(got) > 0 || sigs[0].Certification != CertifyNoChanges || sigs[0].Kind != KindSeal {
		t.Fatalf("certified: %q, %+v", got, sigs[0])
	}
	if _, err := Embed(locked, ciiXML(invoice.URNBasic), nil); !errors.Is(err, ErrCertified) {
		t.Errorf("Embed on a certified PDF: %v", err)
	}
	approval := signer(t, p, sigtest.LeafOptions{})
	if _, err := Sign(locked, approval); !errors.Is(err, ErrCertified) {
		t.Errorf("Sign on a PDF certified without changes: %v", err)
	}
	if _, err := Sign(locked, o); !errors.Is(err, ErrCertified) {
		t.Errorf("a second certification: %v", err)
	}
	// Validation data and document time-stamps are permitted.
	more, err := AddValidationData(locked, &ValidationData{Certificates: []*x509.Certificate{p.Intermediate, p.Root}})
	if err != nil {
		t.Fatal(err)
	}
	more, err = AddDocumentTimestamp(more, ts.Timestamp, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, sigs = verified(t, more, p)
	if got := problems(r); len(got) > 0 || len(sigs) != 2 || !sigs[1].DocumentTimestamp {
		t.Errorf("DSS and document time-stamp after a certification: %q, %+v", got, sigs)
	}

	// An update the certification forbids, here an annotation: written
	// by hand, since the package refuses it.
	annotated := addAnnotation(t, locked)
	if r, _ := verified(t, annotated, p); !r.Has("SIG-06") {
		t.Errorf("annotation after a certification without changes: %q", problems(r))
	}

	// Level 2 admits further signatures, not annotations; level 3 both.
	o.Certify = CertifyFormFilling
	forms, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Sign(forms, approval)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := verified(t, twice, p); r.Has("SIG-06") || r.Has("SIG-05") {
		t.Errorf("signature after a certification at level 2: %q", problems(r))
	}
	if r, _ := verified(t, addAnnotation(t, forms), p); !r.Has("SIG-06") {
		t.Errorf("annotation after a certification at level 2: %q", problems(r))
	}
	o.Certify = CertifyAnnotations
	annots, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := verified(t, addAnnotation(t, annots), p); r.Has("SIG-06") || r.Has("SIG-05") {
		t.Errorf("annotation after a certification at level 3: %q", problems(r))
	}
	if _, err := Sign(twice, o); !errors.Is(err, ErrCertified) {
		t.Errorf("certification of a signed PDF: %v", err)
	}
}

// addAnnotation appends an incremental update with a text annotation on
// the first page.
func addAnnotation(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := pdf.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	pageRef, page, err := signaturePage(r, r.Catalog())
	if err != nil {
		t.Fatal(err)
	}
	u := pdf.NewUpdate(r)
	annot := u.Alloc()
	u.Put(annot, pdf.Dict{
		{Key: "Type", Val: pdf.Name("Annot")}, {Key: "Subtype", Val: pdf.Name("Text")},
		{Key: "Rect", Val: pdf.Array{pdf.Int(10), pdf.Int(10), pdf.Int(30), pdf.Int(30)}},
		{Key: "Contents", Val: pdf.String("note")}, {Key: "F", Val: pdf.Int(4)},
	})
	annots, _ := r.Resolve(page.Get("Annots")).(pdf.Array)
	np := page.Clone()
	np.Set("Annots", append(append(pdf.Array(nil), annots...), annot))
	if ref, ok := page.Get("Annots").(pdf.Ref); ok {
		u.Put(ref, np.Get("Annots"))
	} else {
		u.Put(pageRef, np)
	}
	return u.Bytes(pdf.Dict{{Key: "Root", Val: r.Trailer().Get("Root")}, {Key: "ID", Val: r.Trailer().Get("ID")}})
}

func TestSignLevels(t *testing.T) {
	p := sigtest.New(t)
	ts := tsptest.New(t, p)
	ts.Time = sigtest.Epoch.Add(time.Minute)
	o := signer(t, p, sigtest.LeafOptions{})
	o.Timestamp = ts.Timestamp
	bt, err := Sign(signHost(t), o)
	if err != nil {
		t.Fatal(err)
	}
	r, sigs := verified(t, bt, p)
	if got := problems(r); len(got) > 0 || sigs[0].Level != LevelBT || !sigs[0].TimestampTime.Equal(ts.Time) {
		t.Fatalf("B-T: %q, %+v", got, sigs[0])
	}

	// The token of a TimeStampResp is accepted as well.
	ts.Response = true
	if _, err := Sign(signHost(t), o); err != nil {
		t.Errorf("TimeStampResp: %v", err)
	}
	ts.Response = false

	// A token over other data is refused when signing.
	o2 := *o
	o2.Timestamp = func(digest []byte, h crypto.Hash) ([]byte, error) {
		return ts.Timestamp(append([]byte{1}, digest[1:]...), h)
	}
	if _, err := Sign(signHost(t), &o2); err == nil || !strings.Contains(err.Error(), "time-stamp") {
		t.Errorf("token over other data: %v", err)
	}

	// B-LT: the revocation data of the signing certificate in the DSS.
	leaf := o.Chain[0]
	crl := p.CRL(t, sigtest.Epoch.Add(time.Hour), time.Time{}, false)
	lt, err := AddValidationData(bt, &ValidationData{
		Certificates: []*x509.Certificate{p.Intermediate, p.Root},
		CRLs:         [][]byte{crl.Raw},
		OCSP:         [][]byte{p.OCSP(t, leaf, sigtest.OCSPGood, time.Time{}, sigtest.Epoch.Add(time.Hour), false)},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, sigs = verified(t, lt, p)
	if got := problems(r); len(got) > 0 || sigs[0].Level != LevelBLT || r.Has("SIG-12") {
		t.Fatalf("B-LT: %q, %+v", got, sigs[0])
	}
	// The same data again adds nothing to the store.
	again, err := AddValidationData(lt, &ValidationData{CRLs: [][]byte{crl.Raw}})
	if err != nil {
		t.Fatal(err)
	}
	rr, _ := pdf.Open(again)
	dss, _ := rr.Dict(rr.Catalog().Get("DSS"))
	if crls, _ := rr.Resolve(dss.Get("CRLs")).(pdf.Array); len(crls) != 1 {
		t.Errorf("%d CRLs in the DSS, want 1", len(crls))
	}

	// B-LTA: a document time-stamp over the DSS.
	lta, err := AddDocumentTimestamp(lt, ts.Timestamp, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, sigs = verified(t, lta, p)
	if len(sigs) != 2 || sigs[0].Level != LevelBLTA || !sigs[1].DocumentTimestamp || !sigs[1].Valid || sigs[1].Field != "Timestamp1" {
		t.Fatalf("B-LTA: %q, %+v %+v", problems(r), sigs[0], sigs[1])
	}
	if got := problems(r); len(got) > 0 {
		t.Errorf("B-LTA: %q", got)
	}
	if _, err := AddDocumentTimestamp(lt, nil, 0); err == nil {
		t.Error("no time-stamper: no error")
	}

	// A B-B signature covered by a document time-stamp is B-T.
	bb, err := Sign(signHost(t), signer(t, p, sigtest.LeafOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	stamped, err := AddDocumentTimestamp(bb, ts.Timestamp, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, sigs := verified(t, stamped, p); sigs[0].Level != LevelBT {
		t.Errorf("B-B with a document time-stamp: %s", sigs[0].Level)
	}
}

func TestSignTrust(t *testing.T) {
	p := sigtest.New(t)
	host := signHost(t)
	o := signer(t, p, sigtest.LeafOptions{})
	out, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	leaf := o.Chain[0]

	// Roots of another PKI.
	if r, _ := verified(t, out, sigtest.New(t)); !r.Has("SIG-09") {
		t.Errorf("other roots: %q", problems(r))
	}
	// No roots: not checked, and said so.
	if r, _ := verified(t, out, nil); r.Has("SIG-09") || !r.Has("SIG-10") {
		t.Errorf("no roots: %v", r.Rules())
	}
	// Revoked before the signing time.
	crl := p.CRL(t, sigtest.Epoch, sigtest.Epoch.Add(-time.Hour), false, leaf)
	r, _, err := VerifySignatures(out, &validate.TrustOptions{Roots: p.Roots, At: sigtest.Epoch, CRLs: []*x509.RevocationList{crl}})
	if err != nil || !r.Has("SIG-11") {
		t.Errorf("revoked: %q, %v", problems(r), err)
	}
	// Expired at the time of the verification, without a time-stamp.
	later := sigtest.Epoch.AddDate(3, 0, 0)
	r, _, _ = VerifySignatures(out, &validate.TrustOptions{Roots: p.Roots, At: later})
	if !r.Has("SIG-08") || r.Has("SIG-07") {
		t.Errorf("expired since, no time-stamp: %v", r.Rules())
	}
	// With a time-stamp, the signature stays verifiable.
	ts := tsptest.New(t, p)
	ts.Time = sigtest.Epoch
	o.Timestamp = ts.Timestamp
	bt, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	r, _, _ = VerifySignatures(bt, &validate.TrustOptions{Roots: p.Roots, At: later})
	if got := problems(r); len(got) > 0 {
		t.Errorf("expired since, time-stamped: %q", got)
	}
	// Signed with an expired certificate.
	o.Timestamp = nil
	o.Time = sigtest.Epoch.AddDate(2, 0, 0)
	expired, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := verified(t, expired, p); !r.Has("SIG-07") {
		t.Errorf("signed after the expiry: %v", r.Rules())
	}
	// A key usage that does not allow signing.
	bad := signer(t, p, sigtest.LeafOptions{KeyUsage: x509.KeyUsageKeyEncipherment, Key: sigtest.ECKey(t)})
	enc, err := Sign(host, bad)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := verified(t, enc, p); !r.Has("SIG-07") {
		t.Errorf("key encipherment only: %v", r.Rules())
	}
}

// fmtSscan and itoa keep the imports of the tests short.
func fmtSscan(s string, a, b, c *int64) (int, error) {
	f := strings.Fields(s)
	if len(f) != 4 {
		return 0, errors.New("not four integers")
	}
	for i, p := range []*int64{a, b, c} {
		n, ok := pdf.AsInt(parseInt(f[i+1]))
		if !ok {
			return i, errors.New("not an integer")
		}
		*p = n
	}
	return 3, nil
}

func parseInt(s string) pdf.Object {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return nil
		}
		n = n*10 + int64(c-'0')
	}
	return pdf.Int(n)
}

func itoa(n int64) string {
	return string(pdf.AppendObject(nil, pdf.Int(n)))
}

// FuzzVerifySignatures checks that VerifySignatures never panics on a
// damaged signed PDF: B-T signature, validation data and document
// time-stamp, then a certification and a second signature. The seeds are
// large: run it with a short -fuzzminimizetime.
func FuzzVerifySignatures(f *testing.F) {
	p := sigtest.New(f)
	ts := tsptest.New(f, p)
	ts.Time = sigtest.Epoch
	// An ECDSA key: generating the RSA key of sigtest takes too long in
	// the instrumented binary of -fuzz.
	cert, key := p.Leaf(f, sigtest.LeafOptions{QC: sigtest.QcSeal, Key: sigtest.ECKey(f)})
	o := &SignOptions{Signer: key, Chain: []*x509.Certificate{cert, p.Intermediate}, Time: sigtest.Epoch, Timestamp: ts.Timestamp, TimestampSize: 4096}
	host := convHost{content: "0 g 10 10 50 50 re f"}.build()
	must := func(b []byte, err error) []byte {
		f.Helper()
		if err != nil {
			f.Fatal(err)
		}
		return b
	}
	signed := must(Sign(host, o))
	lt := must(AddValidationData(signed, &ValidationData{Certificates: []*x509.Certificate{p.Root}, CRLs: [][]byte{p.CRL(f, sigtest.Epoch, time.Time{}, false).Raw}}))
	lta := must(AddDocumentTimestamp(lt, ts.Timestamp, 4096))
	certified := must(Sign(host, &SignOptions{Signer: key, Chain: o.Chain, Time: sigtest.Epoch, Certify: CertifyFormFilling}))
	twice := must(Sign(certified, &SignOptions{Signer: key, Chain: o.Chain, Time: sigtest.Epoch}))
	for _, b := range [][]byte{signed, lta, twice} {
		f.Add(b)
	}
	trust := &validate.TrustOptions{Roots: p.Roots, At: sigtest.Epoch}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, sigs, err := VerifySignatures(data, trust)
		if err != nil {
			return
		}
		for _, s := range sigs {
			if s.Valid && s.SignedLength > int64(len(data)) {
				t.Errorf("signature %q covers %d bytes of %d", s.Field, s.SignedLength, len(data))
			}
		}
		_ = r.Valid()
	})
}

// TestSignPDFA checks that the signatures keep a PDF/A-3 hybrid invoice
// conforming: B-B, B-T, B-LTA (validation data and document time-stamp),
// certification followed by an approval signature. With FX_PDFA_OUT, it
// writes the files for veraPDF (conformance/verapdf.sh), which checks the
// rules of ISO 19005-3, 6.4.3 on signatures.
func TestSignPDFA(t *testing.T) {
	p := sigtest.New(t)
	ts := tsptest.New(t, p)
	ts.Time = sigtest.Epoch.Add(time.Minute)
	host := signHost(t)
	o := signer(t, p, sigtest.LeafOptions{})
	bb, err := Sign(host, o)
	if err != nil {
		t.Fatal(err)
	}
	ot := *o
	ot.Timestamp = ts.Timestamp
	bt, err := Sign(host, &ot)
	if err != nil {
		t.Fatal(err)
	}
	lt, err := AddValidationData(bt, &ValidationData{
		Certificates: []*x509.Certificate{p.Intermediate, p.Root},
		CRLs:         [][]byte{p.CRL(t, sigtest.Epoch.Add(time.Hour), time.Time{}, false).Raw},
	})
	if err != nil {
		t.Fatal(err)
	}
	lta, err := AddDocumentTimestamp(lt, ts.Timestamp, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc := *o
	oc.Certify = CertifyFormFilling
	certified, err := Sign(host, &oc)
	if err != nil {
		t.Fatal(err)
	}
	seal := signer(t, p, sigtest.LeafOptions{QC: sigtest.QcSeal, Key: sigtest.ECKey(t)})
	twice, err := Sign(certified, seal)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"signed-B-B", bb}, {"signed-B-T", bt}, {"signed-B-LTA", lta}, {"signed-certified-and-approved", twice},
	} {
		for _, e := range pdfaErrors(f.data) {
			t.Errorf("%s: PDF/A: %s", f.name, e)
		}
		if r, _ := verified(t, f.data, p); len(problems(r)) > 0 {
			t.Errorf("%s: %q", f.name, problems(r))
		}
		writeConverted(t, f.name, f.data)
	}
}

// TestSignForeignShapes checks signatures that Sign does not write but
// other tools do: an adbe.pkcs7.detached signature with a signing-time
// attribute verifies but is not PAdES Baseline B (SIG-04), and a signature
// time-stamp over other data is reported (SIG-13) without making the
// signature invalid.
func TestSignForeignShapes(t *testing.T) {
	p := sigtest.New(t)
	ts := tsptest.New(t, p)
	o := signer(t, p, sigtest.LeafOptions{Key: sigtest.ECKey(t)})
	params := func(subFilter string, cmsOpts *cms.SignOptions) signatureParams {
		return signatureParams{
			prefix: "Signature",
			head: pdf.Dict{
				{Key: "Type", Val: pdf.Name("Sig")},
				{Key: "Filter", Val: pdf.Name("Adobe.PPKLite")},
				{Key: "SubFilter", Val: pdf.Name(subFilter)},
			},
			tail:     pdf.Dict{{Key: "M", Val: pdf.String(pdf.FormatDate(sigtest.Epoch))}},
			size:     8 << 10,
			time:     sigtest.Epoch,
			contents: func(signed []byte) ([]byte, error) { return cms.Sign(signed, cmsOpts) },
		}
	}

	pkcs7, err := appendSignature(signHost(t), params("adbe.pkcs7.detached", &cms.SignOptions{
		Signer: o.Signer, Chain: o.Chain, Detached: true, SigningTime: sigtest.Epoch,
	}))
	if err != nil {
		t.Fatal(err)
	}
	r, sigs := verified(t, pkcs7, p)
	if !r.Has("SIG-04") || r.Has("SIG-03") || !sigs[0].Valid || sigs[0].Level != "" || !sigs[0].Time.Equal(sigtest.Epoch) {
		t.Errorf("adbe.pkcs7.detached: %q, %+v", problems(r), sigs[0])
	}

	stamped, err := appendSignature(signHost(t), params("ETSI.CAdES.detached", &cms.SignOptions{
		Signer: o.Signer, Chain: o.Chain, Detached: true,
		SignatureTimestamp: func([]byte) ([]byte, error) {
			return ts.Timestamp(make([]byte, 32), crypto.SHA256)
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	r, sigs = verified(t, stamped, p)
	if !r.Has("SIG-13") || !sigs[0].Valid || sigs[0].Level != LevelBB || !sigs[0].TimestampTime.IsZero() {
		t.Errorf("time-stamp over other data: %q, %+v", problems(r), sigs[0])
	}
}
