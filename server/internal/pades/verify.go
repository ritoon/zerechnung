package pades

import (
	"bytes"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/pdf"
)

// Signature describes a signature of a document that Verify has checked.
type Signature struct {
	// Field is the fully qualified name of the signature field.
	Field string
	// SubFilter is the encoding of the signature, such as
	// ETSI.CAdES.detached (PAdES) or adbe.pkcs7.detached.
	SubFilter string
	// Signer is the signing certificate.
	Signer *x509.Certificate
	// Time is the signing time of /M, or of the signing-time attribute
	// when /M is absent; zero when neither is there.
	Time time.Time
	// Name, Reason and Location are the /Name, /Reason and /Location
	// entries of the signature dictionary.
	Name, Reason, Location string
	// WholeDocument tells that the byte range covers the whole file but
	// the signature value: the signature is the last change of the
	// document.
	WholeDocument bool
}

// Verify finds every signature field of the document and checks, for each
// one, that its /ByteRange is well formed and excludes exactly its
// /Contents string, that the digest of the signed bytes matches and that
// the CMS signature verifies with its signing certificate. It neither
// builds a trust chain nor checks revocation. It returns the signatures in
// the order they were added, or an error naming the first one that fails.
// A document without signatures gives no signature and no error.
func Verify(data []byte) ([]Signature, error) {
	r, err := pdf.Open(data)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("pades: %w", err)
	}
	form, ok := r.Dict(r.Catalog().Get("AcroForm"))
	if !ok {
		return nil, nil
	}
	type found struct {
		name string
		dict pdf.Dict
	}
	var sigs []found
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
		if ft == "Sig" {
			if v, ok := r.Dict(d.Get("V")); ok {
				sigs = append(sigs, found{name, v})
			}
		}
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

	out := make([]Signature, 0, len(sigs))
	ends := make([]int, 0, len(sigs))
	for _, f := range sigs {
		s, end, err := check(r, data, f.dict)
		if err != nil {
			return nil, fmt.Errorf("pades: signature %q: %w", f.name, err)
		}
		s.Field = f.name
		out = append(out, s)
		ends = append(ends, end)
	}
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return ends[idx[i]] < ends[idx[j]] })
	sorted := make([]Signature, len(out))
	for i, k := range idx {
		sorted[i] = out[k]
	}
	return sorted, nil
}

// check verifies one signature dictionary and returns it with the end of
// the bytes it covers.
func check(r *pdf.Reader, data []byte, v pdf.Dict) (Signature, int, error) {
	var s Signature
	sub, _ := v.Name("SubFilter")
	s.SubFilter = string(sub)
	switch s.SubFilter {
	case "ETSI.CAdES.detached", "adbe.pkcs7.detached":
	default:
		return s, 0, fmt.Errorf("unsupported /SubFilter %q", s.SubFilter)
	}
	br, _ := r.Resolve(v.Get("ByteRange")).(pdf.Array)
	if len(br) != 4 {
		return s, 0, errors.New("the /ByteRange is not an array of four integers")
	}
	var n [4]int64
	for i, x := range br {
		var ok bool
		if n[i], ok = pdf.AsInt(r.Resolve(x)); !ok {
			return s, 0, errors.New("the /ByteRange is not an array of four integers")
		}
	}
	size := int64(len(data))
	if n[0] != 0 || n[1] <= 0 || n[2] < n[1]+2 || n[3] < 0 || n[2] > size || n[3] > size-n[2] {
		return s, 0, fmt.Errorf("the /ByteRange %v does not fit the file of %d bytes", n, size)
	}
	a, b, c := int(n[1]), int(n[2]), int(n[3])

	// The excluded bytes must be the /Contents hexadecimal string, and
	// nothing else.
	gap := data[a:b]
	if gap[0] != '<' || gap[len(gap)-1] != '>' {
		return s, 0, errors.New("the bytes the /ByteRange excludes are not a hexadecimal string")
	}
	der, err := hex.DecodeString(string(gap[1 : len(gap)-1]))
	if err != nil {
		return s, 0, errors.New("the bytes the /ByteRange excludes are not a hexadecimal string")
	}
	if contents, _ := r.Resolve(v.Get("Contents")).(pdf.String); !bytes.Equal(contents, der) {
		return s, 0, errors.New("the /ByteRange does not exclude the /Contents of the signature dictionary")
	}

	sd, err := cms.Parse(der)
	if err != nil {
		return s, 0, err
	}
	if !sd.Detached {
		return s, 0, errors.New("the CMS signature is not detached")
	}
	if len(sd.Signers) != 1 {
		return s, 0, fmt.Errorf("the CMS signature has %d SignerInfos instead of one", len(sd.Signers))
	}
	signed := make([]byte, 0, a+c)
	signed = append(signed, data[:a]...)
	signed = append(signed, data[b:b+c]...)
	if err := sd.VerifyAll(signed); err != nil {
		return s, 0, err
	}

	si := sd.Signers[0]
	s.Signer = si.Certificate
	text := func(k pdf.Name) string {
		if t, ok := r.Resolve(v.Get(k)).(pdf.String); ok {
			return pdf.Text(t)
		}
		return ""
	}
	s.Name, s.Reason, s.Location = text("Name"), text("Reason"), text("Location")
	if t, ok := pdf.ParseDate(text("M")); ok {
		s.Time = t
	} else {
		s.Time = si.SigningTime
	}
	s.WholeDocument = b+c == len(data)
	return s, b + c, nil
}
