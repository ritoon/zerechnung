// Package pades signs PDF documents with PAdES Baseline B-B signatures
// (ETSI EN 319 142-1) and checks their integrity.
//
// Sign appends the signature to the document with an incremental update
// (ISO 32000-1, 7.5.6): the original bytes are kept unchanged, so the XML
// invoice of a Factur-X PDF and any earlier signature stay valid. The
// signature is a detached CMS SignedData (SubFilter ETSI.CAdES.detached)
// with the content-type, message-digest and signing-certificate-v2
// attributes; the signing time goes in the /M entry of the signature
// dictionary, as PAdES requires. The signature field is invisible: a widget
// annotation of zero size on the first page, which PDF/A admits without an
// appearance stream.
//
// Verify checks the integrity of every signature of a document: the byte
// range, the digest and the signature value. It neither builds a trust
// chain nor checks revocation.
package pades

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
)

// Errors of Sign.
var (
	// ErrEncrypted: the document is encrypted, which this package does not
	// handle (and PDF/A forbids).
	ErrEncrypted = errors.New("pades: the document is encrypted")
	// ErrLocked: the document is certified with a DocMDP signature that
	// allows no change (/P 1), so a new signature would invalidate it.
	ErrLocked = errors.New("pades: the document is certified and allows no change (DocMDP /P 1)")
)

// Options configure Sign.
type Options struct {
	// Signer holds the private key, RSA or ECDSA. Only a digest is signed,
	// so the key may stay in a smart card, an HSM or a remote service.
	Signer crypto.Signer
	// Chain is the signing certificate followed by the intermediate
	// authorities, all written in the signature.
	Chain []*x509.Certificate
	// Hash is the digest algorithm: SHA-256 when zero, or SHA-384 or
	// SHA-512. SHA-1 is refused.
	Hash crypto.Hash
	// PSS signs with RSA-PSS instead of RSA PKCS#1 v1.5 (RSA keys only).
	PSS bool
	// Time is the signing time written in /M; time.Now() when zero.
	Time time.Time
	// Name, Reason, Location and ContactInfo are written in the /Name,
	// /Reason, /Location and /ContactInfo entries when not empty.
	Name, Reason, Location, ContactInfo string
	// FieldName is the name of the signature field: "Signature1", or the
	// next free "SignatureN", when empty. A name already taken by a field
	// is refused.
	FieldName string
}

// byteRangePlaceholder is written in /ByteRange before the offsets are
// known: four integers wide enough for a file of up to 10 GB, replaced in
// place by the real values, padded with spaces.
const byteRangePlaceholder = "[0 9999999999 9999999999 9999999999]"

// contentsMargin is added to the size of a trial signature to reserve the
// /Contents string.
const contentsMargin = 64

// Sign returns pdf with a new PAdES Baseline B-B signature, appended as an
// incremental update. It refuses an encrypted document (ErrEncrypted), a
// document certified without changes allowed (ErrLocked), a key that does
// not match the signing certificate and SHA-1.
func Sign(data []byte, o *Options) ([]byte, error) {
	if o == nil || o.Signer == nil {
		return nil, errors.New("pades: no signer")
	}
	if len(o.Chain) == 0 || o.Chain[0] == nil {
		return nil, errors.New("pades: no signing certificate")
	}
	cert := o.Chain[0]
	if !samePublicKey(cert.PublicKey, o.Signer.Public()) {
		return nil, errors.New("pades: the signing certificate does not match the key of the signer")
	}
	alg, err := sigalg.For(cert.PublicKey, o.Hash, o.PSS)
	if err != nil {
		return nil, fmt.Errorf("pades: %w", err)
	}
	if o.FieldName != "" && strings.Contains(o.FieldName, ".") {
		return nil, fmt.Errorf("pades: the field name %q contains a period, which separates the partial names of fields", o.FieldName)
	}

	r, err := pdf.Open(data)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("pades: %w", err)
	}
	cat := r.Catalog()
	if locked(r, cat) {
		return nil, ErrLocked
	}
	rootRef, ok := r.Trailer().Get("Root").(pdf.Ref)
	if !ok {
		return nil, errors.New("pades: the catalog is not an indirect object")
	}
	pageRef, page, err := firstPage(r, cat)
	if err != nil {
		return nil, err
	}
	size, err := reserve(o, alg)
	if err != nil {
		return nil, err
	}

	u := pdf.NewUpdate(r)
	sigRef, fieldRef := u.Alloc(), u.Alloc()
	tm := o.Time
	if tm.IsZero() {
		tm = time.Now()
	}
	tm = tm.Truncate(time.Second)

	// The interactive form: the new field is added to /Fields, and
	// /SigFlags declares that the document has signatures (1) and is to be
	// changed by incremental updates only (2).
	formObj := cat.Get("AcroForm")
	form, _ := r.Dict(formObj)
	newForm := form.Clone()
	fieldsObj := form.Get("Fields")
	fields, _ := r.Resolve(fieldsObj).(pdf.Array)
	name, err := fieldName(r, fields, o.FieldName)
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
		cat = cat.Clone()
		cat.Set("AcroForm", newForm)
		u.Put(rootRef, cat)
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

	// The field and its widget, merged in one dictionary. A zero-size
	// rectangle needs no appearance stream in PDF/A (ISO 19005-2, 6.3.3);
	// the flags are Print and Locked (6.3.2).
	u.Put(fieldRef, pdf.Dict{
		{Key: "Type", Val: pdf.Name("Annot")},
		{Key: "Subtype", Val: pdf.Name("Widget")},
		{Key: "FT", Val: pdf.Name("Sig")},
		{Key: "T", Val: pdf.EncodeText(name)},
		{Key: "V", Val: sigRef},
		{Key: "F", Val: pdf.Int(132)},
		{Key: "Rect", Val: pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(0), pdf.Int(0)}},
		{Key: "P", Val: pageRef},
	})

	// The signature dictionary, with placeholders for /ByteRange and
	// /Contents: zero bytes, which are written as a hexadecimal string.
	sig := pdf.Dict{
		{Key: "Type", Val: pdf.Name("Sig")},
		{Key: "Filter", Val: pdf.Name("Adobe.PPKLite")},
		{Key: "SubFilter", Val: pdf.Name("ETSI.CAdES.detached")},
		{Key: "ByteRange", Val: pdf.Array{pdf.Int(0), pdf.Int(9999999999), pdf.Int(9999999999), pdf.Int(9999999999)}},
		{Key: "Contents", Val: pdf.String(make([]byte, size))},
		{Key: "M", Val: pdf.String(pdf.FormatDate(tm))},
	}
	for _, kv := range []struct {
		key pdf.Name
		val string
	}{{"Name", o.Name}, {"Reason", o.Reason}, {"Location", o.Location}, {"ContactInfo", o.ContactInfo}} {
		if kv.val != "" {
			sig.Set(kv.key, pdf.EncodeText(kv.val))
		}
	}
	u.Put(sigRef, sig)

	trailer := pdf.Dict{{Key: "Root", Val: rootRef}}
	if info := r.Trailer().Get("Info"); info != nil {
		trailer.Set("Info", info)
	}
	trailer.Set("ID", fileID(r, data, tm))
	out := u.Bytes(trailer)

	// Offsets of the /Contents string, delimiters included: the signature
	// covers the whole file but that string (ISO 32000-1, 12.8.1).
	marker := []byte("/ByteRange " + byteRangePlaceholder + "/Contents <")
	i := bytes.Index(out[len(data):], marker)
	if i < 0 {
		return nil, errors.New("pades: internal error: the signature dictionary was not found in the update")
	}
	brAt := len(data) + i + len("/ByteRange ")
	a := len(data) + i + len(marker) - 1
	b := a + 2*size + 2
	if b > len(out) || out[b-1] != '>' {
		return nil, errors.New("pades: internal error: the /Contents string was not found in the update")
	}
	c := len(out) - b
	br := "[0 " + strconv.Itoa(a) + " " + strconv.Itoa(b) + " " + strconv.Itoa(c)
	if len(br) >= len(byteRangePlaceholder) {
		return nil, errors.New("pades: the document is too large to be signed")
	}
	br += strings.Repeat(" ", len(byteRangePlaceholder)-len(br)-1) + "]"
	copy(out[brAt:], br)

	signed := make([]byte, 0, a+c)
	signed = append(signed, out[:a]...)
	signed = append(signed, out[b:]...)
	der, err := cms.Sign(signed, &cms.SignOptions{Signer: o.Signer, Chain: o.Chain, Hash: alg.Hash, PSS: o.PSS, Detached: true})
	if err != nil {
		return nil, fmt.Errorf("pades: %w", err)
	}
	if len(der) > size {
		return nil, fmt.Errorf("pades: the signature takes %d bytes, more than the %d reserved", len(der), size)
	}
	hex.Encode(out[a+1:], der)
	return out, nil
}

// reserve returns the size to reserve for the /Contents string: the size
// of a trial signature made with a signer that returns a signature value of
// the largest length the key can produce, plus a margin. The key itself is
// not used.
func reserve(o *Options, alg sigalg.Algorithm) (int, error) {
	der, err := cms.Sign([]byte("trial"), &cms.SignOptions{
		Signer: maxSigner{o.Signer.Public()}, Chain: o.Chain, Hash: alg.Hash, PSS: o.PSS, Detached: true,
	})
	if err != nil {
		return 0, fmt.Errorf("pades: %w", err)
	}
	return len(der) + contentsMargin, nil
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
	return nil, fmt.Errorf("pades: key of type %T", s.pub)
}

// samePublicKey reports whether two public keys are equal.
func samePublicKey(a, b crypto.PublicKey) bool {
	k, ok := a.(interface{ Equal(crypto.PublicKey) bool })
	return ok && k.Equal(b)
}

// locked reports whether the document is certified by a DocMDP signature
// whose transform parameters allow no change.
func locked(r *pdf.Reader, cat pdf.Dict) bool {
	perms, ok := r.Dict(cat.Get("Perms"))
	if !ok {
		return false
	}
	sig, ok := r.Dict(perms.Get("DocMDP"))
	if !ok {
		return false
	}
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
		if p, ok := pdf.AsInt(r.Resolve(params.Get("P"))); ok && p == 1 {
			return true
		}
	}
	return false
}

// firstPage returns the first page of the document, which must be an
// indirect object to be updated.
func firstPage(r *pdf.Reader, cat pdf.Dict) (pdf.Ref, pdf.Dict, error) {
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
		return pdf.Ref{}, nil, errors.New("pades: the document has no page")
	}
	return ref, d, nil
}

// fieldName returns the name of the new signature field: want, unless a
// top-level field already has it, or the first free "SignatureN".
func fieldName(r *pdf.Reader, fields pdf.Array, want string) (string, error) {
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
			return "", fmt.Errorf("pades: the document already has a field named %q", want)
		}
		return want, nil
	}
	for i := 1; ; i++ {
		if n := "Signature" + strconv.Itoa(i); !taken[n] {
			return n, nil
		}
	}
}

// fileID returns the file identifier of the new trailer: the permanent
// part of the original one, and a new changing part.
func fileID(r *pdf.Reader, data []byte, tm time.Time) pdf.Array {
	var first pdf.String
	if id, ok := r.Resolve(r.Trailer().Get("ID")).(pdf.Array); ok && len(id) > 0 {
		first, _ = r.Resolve(id[0]).(pdf.String)
	}
	h := md5.New()
	h.Write(first)
	h.Write([]byte(pdf.FormatDate(tm)))
	fmt.Fprintf(h, "pades %d", len(data))
	second := pdf.String(h.Sum(nil))
	if len(first) == 0 {
		first = second
	}
	return pdf.Array{first, second}
}
