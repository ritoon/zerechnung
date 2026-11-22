package zerechnung

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/md5"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/sigalg"
	"github.com/ritoon/zerechnung/internal/tsp"
)

// Errors of Sign, Embed and Create on signed documents.
var (
	// ErrCertified is wrapped by the error of Sign, Embed and Create when
	// a certification signature (DocMDP) forbids the change: Embed and
	// Create on any certified document, since no level permits embedding
	// a file; Sign on a document certified at level 1 (no change), and
	// a certification of a document already signed.
	ErrCertified = errors.New("zerechnung: the document is certified (DocMDP) and its certification forbids this change")
)

// Certification is the level of a certification signature (DocMDP, ISO
// 32000-1 12.8.2.2): the changes the signer permits after it. A document
// has at most one certification signature, its first signature.
type Certification int

// Certification levels. Validation data (DSS) and document time-stamps
// may be added at every level (ISO 32000-2, 12.8.2.2.2).
const (
	// NoCertification makes an approval signature.
	NoCertification Certification = iota
	// CertifyNoChanges permits no change (/P 1): any later change, an
	// embedded invoice or another signature included, invalidates the
	// certification.
	CertifyNoChanges
	// CertifyFormFilling permits filling in forms, instantiating page
	// templates and signing (/P 2).
	CertifyFormFilling
	// CertifyAnnotations permits annotations as well (/P 3).
	CertifyAnnotations
)

func (c Certification) String() string {
	switch c {
	case NoCertification:
		return "approval"
	case CertifyNoChanges:
		return "certified, no changes"
	case CertifyFormFilling:
		return "certified, form filling and signing"
	case CertifyAnnotations:
		return "certified, form filling, signing and annotations"
	}
	return "unknown"
}

// Timestamper obtains an RFC 3161 time-stamp token over digest, the hash
// computed with h, from the caller's time-stamping authority. It returns
// the TimeStampToken in DER, or the TimeStampResp that holds it. The
// package never calls a time-stamping authority itself.
type Timestamper func(digest []byte, h crypto.Hash) ([]byte, error)

// SignOptions configure Sign.
type SignOptions struct {
	// Signer is the signing or sealing key, RSA or ECDSA. Only a digest
	// is signed, so the key may stay in the device that holds it (smart
	// card, HSM, remote signing service). A qualified electronic
	// signature or seal needs a qualified certificate and a qualified
	// device: the package checks neither.
	Signer crypto.Signer
	// Chain is the signing certificate first, followed by the
	// certificates that lead to the root of its authority, all written in
	// the signature.
	Chain []*x509.Certificate
	// Hash is the digest algorithm: SHA-256 when zero, or SHA-384 or
	// SHA-512. SHA-1 is refused.
	Hash crypto.Hash
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
	// Time is the signing time written in /M, as PAdES places it;
	// time.Now() when zero.
	Time time.Time
	// Name, Reason, Location and ContactInfo are written in the /Name,
	// /Reason, /Location and /ContactInfo entries of the signature
	// dictionary when not empty.
	Name, Reason, Location, ContactInfo string
	// FieldName is the name of the signature field: the first free
	// "SignatureN" when empty. A name a field already has is refused.
	FieldName string
	// Certify makes a certification signature of that level instead of
	// an approval signature. It is refused on a document already signed.
	Certify Certification
	// Timestamp, when set, time-stamps the signature value (PAdES
	// Baseline B-T): the token is written in the signature. The space
	// reserved for the signature then grows by TimestampSize.
	Timestamp Timestamper
	// TimestampSize is the space reserved for the token of Timestamp, in
	// bytes; DefaultTimestampSize when zero.
	TimestampSize int
}

// DefaultTimestampSize is the space Sign reserves for a time-stamp token
// when SignOptions.TimestampSize is zero: enough for a token that carries
// the certificate chain of its authority.
const DefaultTimestampSize = 12 << 10

// byteRangePlaceholder is written in /ByteRange before the offsets are
// known: four integers wide enough for a file of up to 10 GB, replaced in
// place by the real values, padded with spaces.
const byteRangePlaceholder = "[0 9999999999 9999999999 9999999999]"

// contentsMargin is added to the size of a trial signature to reserve the
// /Contents string.
const contentsMargin = 64

// maxContents bounds the /Contents string: PDF/A limits a string to 32767
// bytes (ISO 19005-2, 6.1.13).
const maxContents = 32767

// Sign returns pdf with a new PAdES signature, appended as an incremental
// update (ISO 32000-1, 7.5.6): the original bytes are kept, so the invoice
// of a Factur-X PDF and any earlier signature stay valid. Sign a hybrid
// invoice after embedding its XML, so that the signature covers both the
// PDF and the XML: Embed and Create do it in one call with
// EmbedOptions.Sign.
//
// The signature is a PAdES Baseline B-B signature (ETSI EN 319 142-1): a
// detached CMS SignedData (/SubFilter ETSI.CAdES.detached) over the whole
// file but the signature value, with the content-type, message-digest and
// signing-certificate-v2 attributes and without signing-time, the signing
// time going in /M. With SignOptions.Timestamp, it is a B-T signature;
// AddValidationData and AddDocumentTimestamp take it to B-LT and B-LTA.
// Whether it is an electronic signature or an electronic seal depends on
// the certificate alone.
//
// The signature field is invisible: a widget annotation of zero size on
// the first page, printable and locked, with an empty appearance stream,
// which keeps the document PDF/A-2, PDF/A-3 or PDF/A-4 when it was. Sign
// refuses an encrypted document, a document certified at level 1
// (ErrCertified), a key that does not match the signing certificate, and
// SHA-1.
func Sign(pdfData []byte, opts *SignOptions) ([]byte, error) {
	o := opts
	if o == nil || o.Signer == nil {
		return nil, errors.New("zerechnung: no signer")
	}
	if len(o.Chain) == 0 || o.Chain[0] == nil {
		return nil, errors.New("zerechnung: no signing certificate")
	}
	cert := o.Chain[0]
	if !samePublicKey(cert.PublicKey, o.Signer.Public()) {
		return nil, errors.New("zerechnung: the signing certificate does not match the key of the signer")
	}
	alg, err := sigalg.For(cert.PublicKey, o.Hash, o.PSS)
	if err != nil {
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	if o.Certify < NoCertification || o.Certify > CertifyAnnotations {
		return nil, fmt.Errorf("zerechnung: unknown certification level %d", o.Certify)
	}
	size, err := reserve(o, alg)
	if err != nil {
		return nil, err
	}
	tm := o.Time
	if tm.IsZero() {
		tm = time.Now()
	}
	tm = tm.Truncate(time.Second)
	sig := pdf.Dict{
		{Key: "Type", Val: pdf.Name("Sig")},
		{Key: "Filter", Val: pdf.Name("Adobe.PPKLite")},
		{Key: "SubFilter", Val: pdf.Name("ETSI.CAdES.detached")},
	}
	tail := pdf.Dict{{Key: "M", Val: pdf.String(pdf.FormatDate(tm))}}
	for _, kv := range []struct {
		key pdf.Name
		val string
	}{{"Name", o.Name}, {"Reason", o.Reason}, {"Location", o.Location}, {"ContactInfo", o.ContactInfo}} {
		if kv.val != "" {
			tail.Set(kv.key, pdf.EncodeText(kv.val))
		}
	}
	if o.Certify != NoCertification {
		tail.Set("Reference", pdf.Array{pdf.Dict{
			{Key: "Type", Val: pdf.Name("SigRef")},
			{Key: "TransformMethod", Val: pdf.Name("DocMDP")},
			{Key: "TransformParams", Val: pdf.Dict{
				{Key: "Type", Val: pdf.Name("TransformParams")},
				{Key: "P", Val: pdf.Int(o.Certify)},
				{Key: "V", Val: pdf.Name("1.2")},
			}},
		}})
	}
	cmsOpts := &cms.SignOptions{Signer: o.Signer, Chain: o.Chain, Hash: alg.Hash, PSS: o.PSS, Detached: true}
	if o.Timestamp != nil {
		cmsOpts.SignatureTimestamp = func(value []byte) ([]byte, error) {
			return timestampToken(o.Timestamp, alg.Hash, value)
		}
	}
	return appendSignature(pdfData, signatureParams{
		field:   o.FieldName,
		prefix:  "Signature",
		head:    sig,
		tail:    tail,
		size:    size,
		certify: o.Certify,
		time:    tm,
		contents: func(signed []byte) ([]byte, error) {
			return cms.Sign(signed, cmsOpts)
		},
	})
}

// AddDocumentTimestamp returns pdf with a document time-stamp appended as
// an incremental update (PAdES, ETSI EN 319 142-1 §5.4): a signature field
// whose dictionary (/Type /DocTimeStamp, /SubFilter ETSI.RFC3161) holds an
// RFC 3161 token over the whole file but the token itself, obtained with
// ts. Added after AddValidationData, it takes the signatures of the
// document to PAdES Baseline B-LTA; later document time-stamps renew it.
// size is the space reserved for the token; DefaultTimestampSize when
// zero. A document time-stamp is admitted at every certification level.
func AddDocumentTimestamp(pdfData []byte, ts Timestamper, size int) ([]byte, error) {
	if ts == nil {
		return nil, errors.New("zerechnung: no time-stamper")
	}
	if size <= 0 {
		size = DefaultTimestampSize
	}
	if size > maxContents {
		return nil, fmt.Errorf("zerechnung: %d bytes reserved for the time-stamp token, more than the %d a PDF/A string admits", size, maxContents)
	}
	return appendSignature(pdfData, signatureParams{
		prefix: "Timestamp",
		head: pdf.Dict{
			{Key: "Type", Val: pdf.Name("DocTimeStamp")},
			{Key: "Filter", Val: pdf.Name("Adobe.PPKLite")},
			{Key: "SubFilter", Val: pdf.Name("ETSI.RFC3161")},
		},
		size:      size,
		timestamp: true,
		time:      time.Now(),
		contents: func(signed []byte) ([]byte, error) {
			return timestampToken(ts, crypto.SHA256, signed)
		},
	})
}

// timestampToken obtains a token over data from ts and checks that it
// covers data, so that a wrong token is reported at once rather than by a
// later verification.
func timestampToken(ts Timestamper, h crypto.Hash, data []byte) ([]byte, error) {
	der, err := ts(sigalg.Digest(h, data), h)
	if err != nil {
		return nil, fmt.Errorf("zerechnung: time-stamp: %w", err)
	}
	tok, err := tsp.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("zerechnung: time-stamp: %w", err)
	}
	if err := tok.Verify(data); err != nil {
		return nil, fmt.Errorf("zerechnung: time-stamp: %w", err)
	}
	return tok.DER, nil
}

// signatureParams describe the signature appendSignature writes.
type signatureParams struct {
	field     string   // name of the field; prefix + first free number when empty
	prefix    string   // prefix of the default field name
	head      pdf.Dict // entries of the signature dictionary before /ByteRange
	tail      pdf.Dict // entries after /Contents
	size      int      // bytes reserved for /Contents
	certify   Certification
	timestamp bool      // a document time-stamp
	time      time.Time // for the file identifier
	// contents returns the value of /Contents for the signed bytes.
	contents func(signed []byte) ([]byte, error)
}

// appendSignature appends to data an incremental update with a new
// signature field, its widget on the first page and its signature
// dictionary, computes the byte range, and fills /Contents with what
// p.contents returns for the signed bytes.
func appendSignature(data []byte, p signatureParams) ([]byte, error) {
	if p.field != "" && strings.Contains(p.field, ".") {
		return nil, fmt.Errorf("zerechnung: the field name %q contains a period, which separates the partial names of fields", p.field)
	}
	r, err := pdf.Open(data)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	cat := r.Catalog()
	level, signed := docMDP(r, cat), hasSignatures(r, cat)
	switch {
	case p.certify != NoCertification && (signed || level != NoCertification):
		return nil, fmt.Errorf("%w: a certification must be the first signature of the document", ErrCertified)
	case !p.timestamp && level == CertifyNoChanges:
		return nil, fmt.Errorf("%w: it permits no change, another signature included (DocMDP /P 1)", ErrCertified)
	}
	rootRef, ok := r.Trailer().Get("Root").(pdf.Ref)
	if !ok {
		return nil, errors.New("zerechnung: the catalog is not an indirect object")
	}
	pageRef, page, err := signaturePage(r, cat)
	if err != nil {
		return nil, err
	}

	u := pdf.NewUpdate(r)
	sigRef, fieldRef, apRef := u.Alloc(), u.Alloc(), u.Alloc()
	newCat := cat.Clone()
	catChanged := false

	// The interactive form: the new field is added to /Fields, and
	// /SigFlags declares that the document has signatures (1) and is to be
	// changed by incremental updates only (2).
	formObj := cat.Get("AcroForm")
	form, _ := r.Dict(formObj)
	newForm := form.Clone()
	fieldsObj := form.Get("Fields")
	fields, _ := r.Resolve(fieldsObj).(pdf.Array)
	name, err := fieldName(r, fields, p.field, p.prefix)
	if err != nil {
		return nil, err
	}
	newFields := append(append(make(pdf.Array, 0, len(fields)+1), fields...), fieldRef)
	if ref, ok := fieldsObj.(pdf.Ref); ok {
		u.Put(ref, newFields)
	} else {
		newForm.Set("Fields", newFields)
	}
	flags, _ := pdf.AsInt(r.Resolve(form.Get("SigFlags")))
	newForm.Set("SigFlags", pdf.Int(flags|3))
	if ref, ok := formObj.(pdf.Ref); ok {
		u.Put(ref, newForm)
	} else {
		newCat.Set("AcroForm", newForm)
		catChanged = true
	}
	if p.certify != NoCertification {
		perms, _ := r.Dict(cat.Get("Perms"))
		perms = perms.Clone()
		perms.Set("DocMDP", sigRef)
		newCat.Set("Perms", perms)
		catChanged = true
	}
	if catChanged {
		u.Put(rootRef, newCat)
	}

	// The widget annotation of the field, on the first page.
	annotsObj := page.Get("Annots")
	annots, _ := r.Resolve(annotsObj).(pdf.Array)
	newAnnots := append(append(make(pdf.Array, 0, len(annots)+1), annots...), fieldRef)
	if ref, ok := annotsObj.(pdf.Ref); ok {
		u.Put(ref, newAnnots)
	} else {
		np := page.Clone()
		np.Set("Annots", newAnnots)
		u.Put(pageRef, np)
	}

	// The field and its widget, merged in one dictionary: a rectangle of
	// zero size, flags Print and Locked (ISO 19005-2, 6.3.2), and an empty
	// appearance stream, which PDF/A does not require for a widget of zero
	// size (6.3.3) but some readers expect.
	zero := pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(0), pdf.Int(0)}
	u.Put(fieldRef, pdf.Dict{
		{Key: "Type", Val: pdf.Name("Annot")},
		{Key: "Subtype", Val: pdf.Name("Widget")},
		{Key: "FT", Val: pdf.Name("Sig")},
		{Key: "T", Val: pdf.EncodeText(name)},
		{Key: "V", Val: sigRef},
		{Key: "F", Val: pdf.Int(132)},
		{Key: "Rect", Val: zero},
		{Key: "P", Val: pageRef},
		{Key: "AP", Val: pdf.Dict{{Key: "N", Val: apRef}}},
	})
	u.PutStream(apRef, pdf.Dict{
		{Key: "Type", Val: pdf.Name("XObject")},
		{Key: "Subtype", Val: pdf.Name("Form")},
		{Key: "BBox", Val: zero},
		{Key: "Resources", Val: pdf.Dict{}},
	}, nil)

	// The signature dictionary, with placeholders for /ByteRange and
	// /Contents: zero bytes, which are written as a hexadecimal string.
	sig := p.head.Clone()
	sig = append(sig,
		pdf.KV{Key: "ByteRange", Val: pdf.Array{pdf.Int(0), pdf.Int(9999999999), pdf.Int(9999999999), pdf.Int(9999999999)}},
		pdf.KV{Key: "Contents", Val: pdf.String(make([]byte, p.size))},
	)
	sig = append(sig, p.tail...)
	u.Put(sigRef, sig)

	trailer := pdf.Dict{{Key: "Root", Val: rootRef}}
	if info := r.Trailer().Get("Info"); info != nil {
		trailer.Set("Info", info)
	}
	trailer.Set("ID", updateFileID(r, data, p.time))
	out := u.Bytes(trailer)

	// Offsets of the /Contents string, delimiters included: the signature
	// covers the whole file but that string (ISO 32000-1, 12.8.1).
	marker := []byte("/ByteRange " + byteRangePlaceholder + "/Contents <")
	i := bytes.Index(out[len(data):], marker)
	if i < 0 {
		return nil, errors.New("zerechnung: internal error: the signature dictionary was not found in the update")
	}
	brAt := len(data) + i + len("/ByteRange ")
	a := len(data) + i + len(marker) - 1
	b := a + 2*p.size + 2
	if b > len(out) || out[b-1] != '>' {
		return nil, errors.New("zerechnung: internal error: the /Contents string was not found in the update")
	}
	c := len(out) - b
	br := "[0 " + strconv.Itoa(a) + " " + strconv.Itoa(b) + " " + strconv.Itoa(c)
	if len(br) >= len(byteRangePlaceholder) {
		return nil, errors.New("zerechnung: the document is too large to be signed")
	}
	br += strings.Repeat(" ", len(byteRangePlaceholder)-len(br)-1) + "]"
	copy(out[brAt:], br)

	signedBytes := make([]byte, 0, a+c)
	signedBytes = append(signedBytes, out[:a]...)
	signedBytes = append(signedBytes, out[b:]...)
	der, err := p.contents(signedBytes)
	if err != nil {
		if strings.HasPrefix(err.Error(), "zerechnung: ") {
			return nil, err
		}
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	if len(der) > p.size {
		return nil, fmt.Errorf("zerechnung: the signature takes %d bytes, more than the %d reserved", len(der), p.size)
	}
	hex.Encode(out[a+1:], der)
	return out, nil
}

// reserve returns the size to reserve for the /Contents string: the size
// of a trial signature made with a signer that returns a signature value of
// the largest length the key can produce, plus a margin and the space of
// the time-stamp token. The key itself is not used.
func reserve(o *SignOptions, alg sigalg.Algorithm) (int, error) {
	der, err := cms.Sign([]byte("trial"), &cms.SignOptions{
		Signer: maxSigner{o.Signer.Public()}, Chain: o.Chain, Hash: alg.Hash, PSS: o.PSS, Detached: true,
	})
	if err != nil {
		return 0, fmt.Errorf("zerechnung: %w", err)
	}
	size := len(der) + contentsMargin
	if o.Timestamp != nil {
		ts := o.TimestampSize
		if ts <= 0 {
			ts = DefaultTimestampSize
		}
		size += ts + 32 // the attribute around the token
	}
	if size > maxContents {
		return 0, fmt.Errorf("zerechnung: the signature needs %d bytes, more than the %d a PDF/A string admits: shorten the certificate chain or TimestampSize", size, maxContents)
	}
	return size, nil
}

// maxSigner is a crypto.Signer that returns a signature value of the
// largest length its public key allows, for the trial signature of
// reserve.
type maxSigner struct{ pub crypto.PublicKey }

func (s maxSigner) Public() crypto.PublicKey { return s.pub }

func (s maxSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	switch k := s.pub.(type) {
	case *rsa.PublicKey:
		return make([]byte, k.Size()), nil
	case *ecdsa.PublicKey:
		// SEQUENCE { INTEGER r, INTEGER s }, each integer as long as the
		// order of the curve plus a leading zero byte.
		n := (k.Params().N.BitLen()+7)/8 + 1
		inner := 2 * (2 + n)
		head := 2
		if inner >= 128 {
			head = 3
		}
		return make([]byte, head+inner), nil
	}
	return nil, fmt.Errorf("zerechnung: key of type %T", s.pub)
}

// samePublicKey reports whether two public keys are equal.
func samePublicKey(a, b crypto.PublicKey) bool {
	k, ok := a.(interface{ Equal(crypto.PublicKey) bool })
	return ok && k.Equal(b)
}

// docMDP returns the level of the certification signature of the document,
// NoCertification when it has none.
func docMDP(r *pdf.Reader, cat pdf.Dict) Certification {
	perms, ok := r.Dict(cat.Get("Perms"))
	if !ok {
		return NoCertification
	}
	sig, ok := r.Dict(perms.Get("DocMDP"))
	if !ok {
		return NoCertification
	}
	return certificationOf(r, sig)
}

// certificationOf returns the DocMDP level of a signature dictionary: that
// of its DocMDP signature reference, NoCertification without one. A
// missing or unknown /P means level 2 (ISO 32000-1, table 254).
func certificationOf(r *pdf.Reader, sig pdf.Dict) Certification {
	refs, _ := r.Resolve(sig.Get("Reference")).(pdf.Array)
	for _, x := range refs {
		ref, ok := r.Dict(x)
		if !ok {
			continue
		}
		if m, _ := ref.Name("TransformMethod"); m != "DocMDP" {
			continue
		}
		params, _ := r.Dict(ref.Get("TransformParams"))
		switch p, _ := pdf.AsInt(r.Resolve(params.Get("P"))); p {
		case 1:
			return CertifyNoChanges
		case 3:
			return CertifyAnnotations
		default:
			return CertifyFormFilling
		}
	}
	return NoCertification
}

// hasSignatures reports whether a signature field of the document has a
// value.
func hasSignatures(r *pdf.Reader, cat pdf.Dict) bool {
	found := false
	walkFields(r, cat, func(_ string, _ pdf.Object, d pdf.Dict, ft pdf.Name) {
		if ft == "Sig" && r.Resolve(d.Get("V")) != nil {
			found = true
		}
	})
	return found
}

// walkFields calls fn for every field and widget of the interactive form,
// with its fully qualified name and its inherited field type.
func walkFields(r *pdf.Reader, cat pdf.Dict, fn func(name string, o pdf.Object, d pdf.Dict, ft pdf.Name)) {
	form, ok := r.Dict(cat.Get("AcroForm"))
	if !ok {
		return
	}
	seen := map[pdf.Ref]bool{}
	var walk func(o pdf.Object, prefix string, ft pdf.Name, depth int)
	walk = func(o pdf.Object, prefix string, ft pdf.Name, depth int) {
		if ref, ok := o.(pdf.Ref); ok {
			if seen[ref] {
				return
			}
			seen[ref] = true
		}
		d, ok := r.Dict(o)
		if !ok || depth > 32 || len(seen) > 100000 {
			return
		}
		name := prefix
		if t, ok := r.Resolve(d.Get("T")).(pdf.String); ok {
			if name != "" {
				name += "."
			}
			name += pdf.Text(t)
		}
		if f, ok := d.Name("FT"); ok {
			ft = f
		}
		fn(name, o, d, ft)
		if kids, ok := r.Resolve(d.Get("Kids")).(pdf.Array); ok {
			for _, k := range kids {
				walk(k, name, ft, depth+1)
			}
		}
	}
	if fields, ok := r.Resolve(form.Get("Fields")).(pdf.Array); ok {
		for _, f := range fields {
			walk(f, "", "", 0)
		}
	}
}

// signaturePage returns the first page of the document, which must be an
// indirect object to be updated.
func signaturePage(r *pdf.Reader, cat pdf.Dict) (pdf.Ref, pdf.Dict, error) {
	seen := map[pdf.Ref]bool{}
	var find func(o pdf.Object, depth int) (pdf.Ref, pdf.Dict, bool)
	find = func(o pdf.Object, depth int) (pdf.Ref, pdf.Dict, bool) {
		ref, isRef := o.(pdf.Ref)
		if !isRef || seen[ref] || depth > 64 {
			return pdf.Ref{}, nil, false
		}
		seen[ref] = true
		d, ok := r.Dict(ref)
		if !ok {
			return pdf.Ref{}, nil, false
		}
		kids, hasKids := r.Resolve(d.Get("Kids")).(pdf.Array)
		if typ, _ := d.Name("Type"); typ == "Page" || !hasKids {
			return ref, d, typ == "Page" || d.Has("Contents") || d.Has("MediaBox")
		}
		for _, k := range kids {
			if pr, pd, ok := find(k, depth+1); ok {
				return pr, pd, true
			}
		}
		return pdf.Ref{}, nil, false
	}
	ref, d, ok := find(cat.Get("Pages"), 0)
	if !ok {
		return pdf.Ref{}, nil, errors.New("zerechnung: the document has no page")
	}
	return ref, d, nil
}

// fieldName returns the name of the new signature field: want, unless a
// top-level field already has it, or the first free prefix + number.
func fieldName(r *pdf.Reader, fields pdf.Array, want, prefix string) (string, error) {
	taken := map[string]bool{}
	for _, f := range fields {
		if d, ok := r.Dict(f); ok {
			if t, ok := r.Resolve(d.Get("T")).(pdf.String); ok {
				taken[pdf.Text(t)] = true
			}
		}
	}
	if want != "" {
		if taken[want] {
			return "", fmt.Errorf("zerechnung: the document already has a field named %q", want)
		}
		return want, nil
	}
	for i := 1; ; i++ {
		if n := prefix + strconv.Itoa(i); !taken[n] {
			return n, nil
		}
	}
}

// updateFileID returns the file identifier of the new trailer: the permanent
// part of the original one, and a new changing part.
func updateFileID(r *pdf.Reader, data []byte, tm time.Time) pdf.Array {
	var first pdf.String
	if id, ok := r.Resolve(r.Trailer().Get("ID")).(pdf.Array); ok && len(id) > 0 {
		first, _ = r.Resolve(id[0]).(pdf.String)
	}
	h := md5.New()
	h.Write(first)
	h.Write([]byte(pdf.FormatDate(tm)))
	fmt.Fprintf(h, "signature %d", len(data))
	second := pdf.String(h.Sum(nil))
	if len(first) == 0 {
		first = second
	}
	return pdf.Array{first, second}
}

// ValidationData is the data that AddValidationData writes in the document
// security store: what a verifier needs to check the signatures of the
// document once their certificates have expired or the authorities have
// stopped answering (PAdES Baseline B-LT, ETSI EN 319 142-1 §5.4). The
// caller obtains it: the package fetches nothing.
type ValidationData struct {
	// Certificates are the certificates of the chains of the signing and
	// time-stamping certificates, and of the authorities that signed the
	// CRLs and OCSP responses.
	Certificates []*x509.Certificate
	// CRLs are CRLs in DER (x509.RevocationList.Raw).
	CRLs [][]byte
	// OCSP are OCSP responses in DER (OCSPResponse of RFC 6960).
	OCSP [][]byte
}

// AddValidationData returns pdf with the validation data merged into its
// document security store (/DSS of the catalog, ISO 32000-2 12.8.4.3), in
// an incremental update: each certificate, CRL and OCSP response becomes a
// stream listed in /Certs, /CRLs or /OCSPs, unless the store already holds
// the same bytes. After a B-T signature, it makes a B-LT signature; a
// document time-stamp added next (AddDocumentTimestamp) makes it B-LTA.
// The store may be updated at every certification level.
func AddValidationData(pdfData []byte, vd *ValidationData) ([]byte, error) {
	if vd == nil || len(vd.Certificates)+len(vd.CRLs)+len(vd.OCSP) == 0 {
		return nil, errors.New("zerechnung: no validation data")
	}
	r, err := pdf.Open(pdfData)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("zerechnung: %w", err)
	}
	rootRef, ok := r.Trailer().Get("Root").(pdf.Ref)
	if !ok {
		return nil, errors.New("zerechnung: the catalog is not an indirect object")
	}
	cat := r.Catalog().Clone()
	dssObj := cat.Get("DSS")
	dss, _ := r.Dict(dssObj)
	dss = dss.Clone()
	if !dss.Has("Type") {
		dss = append(pdf.Dict{{Key: "Type", Val: pdf.Name("DSS")}}, dss...)
	}
	u := pdf.NewUpdate(r)
	merge := func(key pdf.Name, items [][]byte) {
		old, _ := r.Resolve(dss.Get(key)).(pdf.Array)
		have := map[string]bool{}
		for _, x := range old {
			if st, ok := r.Resolve(x).(*pdf.Stream); ok {
				if b, err := r.Decode(st); err == nil {
					have[string(b)] = true
				}
			}
		}
		arr := append(pdf.Array(nil), old...)
		for _, it := range items {
			if len(it) == 0 || have[string(it)] {
				continue
			}
			have[string(it)] = true
			ref := u.Alloc()
			u.PutStream(ref, pdf.Dict{{Key: "Filter", Val: pdf.Name("FlateDecode")}}, pdf.Deflate(it))
			arr = append(arr, ref)
		}
		if len(arr) > 0 {
			dss.Set(key, arr)
		}
	}
	certs := make([][]byte, 0, len(vd.Certificates))
	for _, c := range vd.Certificates {
		if c != nil {
			certs = append(certs, c.Raw)
		}
	}
	merge("Certs", certs)
	merge("CRLs", vd.CRLs)
	merge("OCSPs", vd.OCSP)
	if ref, ok := dssObj.(pdf.Ref); ok {
		u.Put(ref, dss)
	} else {
		ref := u.Alloc()
		u.Put(ref, dss)
		cat.Set("DSS", ref)
		u.Put(rootRef, cat)
	}
	trailer := pdf.Dict{{Key: "Root", Val: rootRef}}
	if info := r.Trailer().Get("Info"); info != nil {
		trailer.Set("Info", info)
	}
	trailer.Set("ID", updateFileID(r, pdfData, time.Now()))
	return u.Bytes(trailer), nil
}
