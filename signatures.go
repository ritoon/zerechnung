package zerechnung

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/internal/cms"
	"github.com/ritoon/zerechnung/internal/esig"
	"github.com/ritoon/zerechnung/internal/pdf"
	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/tsp"
	"github.com/ritoon/zerechnung/validate"
)

// SignerKind tells whether a certificate is issued for electronic
// signatures or for electronic seals, as its QCStatements declare (ETSI
// EN 319 412-5).
type SignerKind int

// Signer kinds.
const (
	KindUnknown   SignerKind = iota // the certificate declares neither
	KindSignature                   // electronic signature of a natural person (QcType esign)
	KindSeal                        // electronic seal of a legal person, such as a company (QcType eseal)
)

func (k SignerKind) String() string {
	switch k {
	case KindSignature:
		return "signature"
	case KindSeal:
		return "seal"
	}
	return "unknown"
}

// PAdES baseline levels (ETSI EN 319 142-1), the values of Signature.Level.
const (
	LevelBB   = "B-B"   // signing certificate referenced, time in /M
	LevelBT   = "B-T"   // a time-stamp proves the signature existed at its time
	LevelBLT  = "B-LT"  // the document security store holds the revocation data of the signing certificate
	LevelBLTA = "B-LTA" // a document time-stamp covers that data
)

// Signature describes a signature of a PDF, as VerifySignatures reads it.
type Signature struct {
	// Field is the fully qualified name of the signature field.
	Field string
	// DocumentTimestamp tells that the field holds a document time-stamp
	// (/Type /DocTimeStamp, /SubFilter ETSI.RFC3161) rather than a
	// signature: Certificate is then the certificate of the
	// time-stamping authority, and Time the time it gives.
	DocumentTimestamp bool
	// SubFilter is the encoding of the signature: ETSI.CAdES.detached
	// (PAdES), adbe.pkcs7.detached, adbe.pkcs7.sha1 or ETSI.RFC3161.
	SubFilter string
	// Certificate is the signing certificate, and Certificates the other
	// certificates of the signature.
	Certificate  *x509.Certificate
	Certificates []*x509.Certificate
	// Kind tells whether a signature or a seal was applied, Qualified
	// whether the certificate declares itself an EU qualified certificate,
	// and QSCD whether it declares its key held in a qualified device.
	Kind      SignerKind
	Qualified bool
	QSCD      bool
	// Time is the signing time the signer claims (/M, or the
	// signing-time attribute of a signature that has no /M).
	Time time.Time
	// TimestampTime is the time of the signature time-stamp (B-T), zero
	// when the signature has none or it does not verify.
	TimestampTime time.Time
	// Name, Reason, Location and ContactInfo are the entries of the
	// signature dictionary.
	Name, Reason, Location, ContactInfo string
	// Certification is the DocMDP level of a certification signature,
	// NoCertification for an approval signature.
	Certification Certification
	// Level is the PAdES baseline level the signature reaches: LevelBB,
	// LevelBT, LevelBLT or LevelBLTA; empty when it is not a PAdES
	// baseline signature, or does not verify.
	Level string
	// Valid tells that the signature verifies on the bytes it covers:
	// byte range, digest, signature value and signing certificate
	// reference. Trust in the certificate is reported apart.
	Valid bool
	// SignedLength is the number of bytes from the start of the file to
	// the end of the revision the signature covers. WholeDocument tells
	// that the signature covers the whole file: nothing was appended
	// after it.
	SignedLength  int64
	WholeDocument bool
	// Changes is the nature of the updates appended after the signature,
	// such as "embedded files" or "XMP metadata", "signature fields" and
	// "validation data (DSS)" included; nil when it covers the whole file.
	Changes []string
}

// SignatureCatalog lists the checks of VerifySignatures, which Check
// applies to a signed PDF. Every finding has Source
// validate.SourceSignature.
var SignatureCatalog = []rules.RuleInfo{
	{ID: "SIG-01", Severity: rules.Error, Description: "The signature can be read: its dictionary has a /Contents string holding a CMS SignedData in DER with a single SignerInfo (ISO 19005-2, 6.4.3), a /ByteRange of four integers within the file, and a /SubFilter the package verifies: ETSI.CAdES.detached, adbe.pkcs7.detached, adbe.pkcs7.sha1, or ETSI.RFC3161 for a document time-stamp."},
	{ID: "SIG-02", Severity: rules.Error, Description: "The /ByteRange covers the file from its start to the end of a revision (a %%EOF marker), except the /Contents value of the signature dictionary with its < and > delimiters, and nothing else (ISO 32000-1, 12.8.1; ISO 19005-2 and 19005-3, 6.4.3)."},
	{ID: "SIG-03", Severity: rules.Error, Description: "The signature verifies: the signed bytes match its message digest (or, for a document time-stamp, the message imprint of the token), its signature value verifies with the key of the signing certificate, which the signature carries (ISO 19005-2, 6.4.3), and the signing-certificate attribute, when present, designates that certificate. SHA-1 signatures are refused."},
	{ID: "SIG-04", Severity: rules.Warning, Description: "The signature is a PAdES Baseline B signature (ETSI EN 319 142-1, 6.3): /SubFilter ETSI.CAdES.detached, the content type id-data, the signing-certificate-v2 attribute, no signing-time attribute, the claimed signing time in /M, no /Cert entry."},
	{ID: "SIG-05", Severity: rules.Warning, Description: "The document was not modified after an approval signature by other updates than validation data (DSS), document time-stamps and further signatures. The signature still verifies, but on an earlier revision: the updates are listed by nature (embedded files, such as an invoice embedded after the signature, XMP metadata, form fields, annotations, pages, other objects)."},
	{ID: "SIG-06", Severity: rules.Error, Description: "A certification signature (DocMDP) is the first signature of the document, and the updates appended after it are among those its level permits: none at level 1, filling in forms and signing at level 2, annotations as well at level 3; validation data (DSS) and document time-stamps at every level (ISO 32000-2, 12.8.2.2). Embedding a file is permitted at no level."},
	{ID: "SIG-07", Severity: rules.Error, Description: "The signing certificate is valid at the signing time — the time of a time-stamp that covers the signature, or else the claimed time of /M — and its key usage allows signing (digitalSignature or nonRepudiation)."},
	{ID: "SIG-08", Severity: rules.Warning, Description: "No time-stamp covers the signature (PAdES B-B), so nothing proves it was made before its certificate expired: the certificate is not valid at the time of the verification (validate.TrustOptions.At, now by default). A signature time-stamp (B-T) or a document time-stamp keeps such a signature verifiable."},
	{ID: "SIG-09", Severity: rules.Error, Description: "The signing certificate leads to one of the trusted roots given (validate.TrustOptions.Roots) by a valid chain, built with the certificates of the signature, of the document security store and of validate.TrustOptions.Intermediates."},
	{ID: "SIG-10", Severity: rules.Info, Description: "No trusted root was given (validate.TrustOptions.Roots): the chains of the signing certificates are not checked."},
	{ID: "SIG-11", Severity: rules.Error, Description: "The signing certificate was not revoked or suspended before the signing time, according to the CRLs and OCSP responses given (validate.TrustOptions) or found in the document security store."},
	{ID: "SIG-12", Severity: rules.Info, Description: "No CRL or OCSP response given or found in the document security store covers the signing certificate: its revocation is not checked."},
	{ID: "SIG-13", Severity: rules.Error, Description: "The time-stamp tokens verify (RFC 3161): the signature time-stamp of a signature covers its signature value, the CMS signature of each token verifies, and the certificate of the authority is issued for time-stamping."},
	{ID: "SIG-14", Severity: rules.Info, Description: "A signature of the document: its field, the signer and the kind of certificate (signature of a person or seal of a legal person, qualified or not, key in a qualified device, according to the QCStatements of ETSI EN 319 412-5), the signing time, the PAdES level, the certification level and whether it covers the whole document."},
}

// VerifySignatures checks the signatures of a PDF and returns them in the
// order they were applied, with a report of the problems found (rules of
// SignatureCatalog). For each signature, it checks that its /ByteRange
// covers its revision whole but its value, that the CMS signature
// verifies on the signed bytes, its PAdES baseline requirements, and the
// time-stamp tokens it carries; it lists the updates appended after it and
// their nature, and reports those a certification signature (DocMDP)
// forbids; it checks the signing certificate at the signing time — its
// validity, its chain to opts.Roots, its revocation against the CRLs and
// OCSP responses of opts and of the document security store — and
// whether the signature remains verifiable once the certificate expires.
// Nothing is fetched from the network.
//
// A PDF without signatures gives an empty report and no signature. The
// error is reserved for files that cannot be read as a PDF; an encrypted
// PDF gives ErrEncrypted.
func VerifySignatures(pdfData []byte, opts *validate.TrustOptions) (*validate.Report, []*Signature, error) {
	r, err := pdf.Open(pdfData)
	if err != nil {
		if errors.Is(err, pdf.ErrEncrypted) {
			return nil, nil, ErrEncrypted
		}
		return nil, nil, fmt.Errorf("zerechnung: %w", err)
	}
	k := &sigCheck{data: pdfData, r: r, report: &validate.Report{}}
	if opts != nil {
		k.opts = *opts
	}
	if k.opts.At.IsZero() {
		k.opts.At = time.Now()
	}
	sigs := k.run()
	return k.report, sigs, nil
}

// sigCheck holds the state of VerifySignatures.
type sigCheck struct {
	data   []byte
	r      *pdf.Reader
	opts   validate.TrustOptions
	report *validate.Report

	dss      dssData
	ctx      *changeContext
	dssAt    map[int64]bool // whether the revision ending at an offset has a DSS
	trustMsg bool           // SIG-10 reported
}

// dssData is the content of the document security store.
type dssData struct {
	present bool
	certs   []*x509.Certificate
	crls    []*x509.RevocationList
	ocsp    [][]byte
}

// found is a signature field being checked.
type found struct {
	sig    *Signature
	dict   pdf.Dict
	ref    pdf.Object
	path   string
	end    int64 // end of the byte range, -1 when unknown
	signer *cms.SignerInfo
	sd     *cms.SignedData
	token  *tsp.Token // the token of a document time-stamp
	pades  []string   // PAdES baseline requirements not met
}

func (k *sigCheck) add(rule, path, format string, args ...any) {
	sev := rules.Error
	for _, ri := range SignatureCatalog {
		if ri.ID == rule {
			sev = ri.Severity
		}
	}
	k.report.Add(rules.Finding{Rule: rule, Severity: sev, Source: rules.SourceSignature, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (k *sigCheck) run() []*Signature {
	cat := k.r.Catalog()
	var fs []*found
	seen := map[pdf.Ref]bool{}
	walkFields(k.r, cat, func(name string, _ pdf.Object, d pdf.Dict, ft pdf.Name) {
		if ft != "Sig" {
			return
		}
		v := d.Get("V")
		if ref, ok := v.(pdf.Ref); ok {
			if seen[ref] {
				return
			}
			seen[ref] = true
		}
		sd, ok := k.r.Dict(v)
		if !ok {
			return
		}
		fs = append(fs, &found{sig: &Signature{Field: name}, dict: sd, ref: v, path: fmt.Sprintf("signature field %q", name), end: -1})
	})
	if len(fs) == 0 {
		return nil
	}
	k.loadDSS(cat)
	for _, f := range fs {
		k.integrity(f)
	}
	sort.SliceStable(fs, func(i, j int) bool {
		ei, ej := fs[i].end, fs[j].end
		if ei < 0 {
			ei = int64(len(k.data)) + 1
		}
		if ej < 0 {
			ej = int64(len(k.data)) + 1
		}
		return ei < ej
	})
	k.changes(fs, cat)
	for _, f := range fs {
		k.trust(f, fs)
	}
	k.levels(fs)
	out := make([]*Signature, len(fs))
	for i, f := range fs {
		out[i] = f.sig
		k.describe(f)
	}
	return out
}

// loadDSS reads the certificates, CRLs and OCSP responses of the document
// security store.
func (k *sigCheck) loadDSS(cat pdf.Dict) {
	dss, ok := k.r.Dict(cat.Get("DSS"))
	if !ok {
		return
	}
	k.dss.present = true
	each := func(key pdf.Name, fn func([]byte)) {
		arr, _ := k.r.Resolve(dss.Get(key)).(pdf.Array)
		for i, x := range arr {
			if i >= 1024 {
				return
			}
			if st, ok := k.r.Resolve(x).(*pdf.Stream); ok {
				if b, err := k.r.Decode(st); err == nil {
					fn(b)
				}
			}
		}
	}
	each("Certs", func(b []byte) {
		if c, err := x509.ParseCertificate(b); err == nil {
			k.dss.certs = append(k.dss.certs, c)
		}
	})
	each("CRLs", func(b []byte) {
		if c, err := x509.ParseRevocationList(b); err == nil {
			k.dss.crls = append(k.dss.crls, c)
		}
	})
	each("OCSPs", func(b []byte) { k.dss.ocsp = append(k.dss.ocsp, b) })
}

// text returns a text string entry of the signature dictionary.
func (k *sigCheck) text(d pdf.Dict, key pdf.Name) string {
	if t, ok := k.r.Resolve(d.Get(key)).(pdf.String); ok {
		return pdf.Text(t)
	}
	return ""
}

// integrity checks the byte range and the signature value of f (SIG-01 to
// SIG-04, SIG-13), and fills in what the signature says.
func (k *sigCheck) integrity(f *found) {
	s, d := f.sig, f.dict
	sub, _ := d.Name("SubFilter")
	s.SubFilter = string(sub)
	typ, _ := d.Name("Type")
	s.DocumentTimestamp = typ == "DocTimeStamp" || sub == "ETSI.RFC3161"
	s.Name, s.Reason, s.Location, s.ContactInfo = k.text(d, "Name"), k.text(d, "Reason"), k.text(d, "Location"), k.text(d, "ContactInfo")
	if t, ok := pdf.ParseDate(k.text(d, "M")); ok {
		s.Time = t
	}
	s.Certification = certificationOf(k.r, d)
	switch sub {
	case "ETSI.CAdES.detached", "adbe.pkcs7.detached", "adbe.pkcs7.sha1", "ETSI.RFC3161":
	default:
		k.add("SIG-01", f.path, "the /SubFilter %q is not one the package verifies (ETSI.CAdES.detached, adbe.pkcs7.detached, adbe.pkcs7.sha1, ETSI.RFC3161)", s.SubFilter)
		return
	}

	// The byte range.
	br, _ := k.r.Resolve(d.Get("ByteRange")).(pdf.Array)
	var n [4]int64
	ok := len(br) == 4
	for i := 0; ok && i < 4; i++ {
		n[i], ok = pdf.AsInt(k.r.Resolve(br[i]))
	}
	if !ok {
		k.add("SIG-01", f.path+" /ByteRange", "the /ByteRange is not an array of four integers")
		return
	}
	size := int64(len(k.data))
	if n[0] != 0 || n[1] <= 0 || n[2] < n[1]+2 || n[3] < 0 || n[2] > size || n[3] > size-n[2] {
		k.add("SIG-02", f.path+" /ByteRange", "the /ByteRange %v does not fit the file of %d bytes as [0 a b c] with a < b", n, size)
		return
	}
	a, b, c := n[1], n[2], n[3]
	contents, isString := k.r.Resolve(d.Get("Contents")).(pdf.String)
	if !isString || len(contents) == 0 {
		k.add("SIG-01", f.path+" /Contents", "the signature dictionary has no /Contents string")
		return
	}
	gap := k.data[a:b]
	if gap[0] != '<' || gap[len(gap)-1] != '>' {
		k.add("SIG-02", f.path+" /ByteRange", "the bytes the /ByteRange excludes, %d to %d, are not a hexadecimal string", a, b)
		return
	}
	if raw, ok := decodeHex(gap[1 : len(gap)-1]); !ok || !bytes.Equal(raw, contents) {
		k.add("SIG-02", f.path+" /ByteRange", "the bytes the /ByteRange excludes, %d to %d, are not the /Contents of the signature dictionary", a, b)
		return
	}
	end := b + c
	if !revisionEnd(k.data, end) {
		k.add("SIG-02", f.path+" /ByteRange", "the /ByteRange ends at byte %d, which is not the end of a revision (a %%%%EOF marker)", end)
	}
	f.end = end
	s.SignedLength = end
	s.WholeDocument = len(bytes.Trim(k.data[end:], "\x00\t\n\r\f ")) == 0
	signed := make([]byte, 0, a+c)
	signed = append(signed, k.data[:a]...)
	signed = append(signed, k.data[b:end]...)

	if s.DocumentTimestamp {
		k.documentTimestamp(f, contents, signed)
		return
	}
	sd, err := cms.Parse(contents)
	if err != nil {
		k.add("SIG-01", f.path+" /Contents", "%v", err)
		return
	}
	if len(sd.Signers) != 1 {
		k.add("SIG-01", f.path+" /Contents", "the CMS signature has %d SignerInfos instead of one", len(sd.Signers))
		return
	}
	si := sd.Signers[0]
	f.sd, f.signer = sd, si
	s.Certificate = si.Certificate
	if si.Certificate != nil {
		s.Certificates = sd.Chain(si.Certificate)
	}
	if s.Time.IsZero() {
		s.Time = si.SigningTime
	}
	content := signed
	switch {
	case sub == "adbe.pkcs7.sha1":
		if sd.Detached {
			k.add("SIG-01", f.path+" /Contents", "a signature with /SubFilter adbe.pkcs7.sha1 must hold the SHA-1 digest of the signed bytes, but the CMS signature is detached")
			return
		}
		sum := sha1.Sum(signed)
		if !bytes.Equal(sd.Content, sum[:]) {
			k.add("SIG-03", f.path, "the document does not match what was signed: the SHA-1 digest of the signed bytes differs from the signed content")
			return
		}
		content = sd.Content
	case !sd.Detached:
		k.add("SIG-01", f.path+" /Contents", "the CMS signature is not detached, as /SubFilter %s requires", s.SubFilter)
		return
	}
	if err := si.Verify(sd, content); err != nil {
		msg := strings.TrimPrefix(err.Error(), "cms: ")
		if errors.Is(err, cms.ErrDigest) {
			msg = "the document does not match what was signed: " + msg
		}
		k.add("SIG-03", f.path, "%s", msg)
		return
	}
	s.Valid = true
	k.describeCert(s, si.Certificate)

	// PAdES Baseline B (ETSI EN 319 142-1, 6.3).
	if sub != "ETSI.CAdES.detached" {
		f.pades = append(f.pades, "/SubFilter "+s.SubFilter+" instead of ETSI.CAdES.detached")
	}
	if !sd.ContentType.Equal(cms.OIDData) {
		f.pades = append(f.pades, fmt.Sprintf("content type %v instead of id-data", sd.ContentType))
	}
	switch {
	case si.SigningCert == nil:
		f.pades = append(f.pades, "no signing-certificate-v2 attribute")
	case !si.SigningCert.V2:
		f.pades = append(f.pades, "a signing-certificate attribute (SHA-1) instead of signing-certificate-v2")
	}
	if !si.SigningTime.IsZero() {
		f.pades = append(f.pades, "a signing-time attribute, which PAdES forbids (the time goes in /M)")
	}
	if !d.Has("M") {
		f.pades = append(f.pades, "no /M entry (claimed signing time)")
	}
	if d.Has("Cert") {
		f.pades = append(f.pades, "a /Cert entry")
	}
	if len(f.pades) > 0 {
		k.add("SIG-04", f.path, "not a PAdES Baseline B signature: %s", strings.Join(f.pades, "; "))
	}

	// Signature time-stamps (B-T).
	for _, der := range si.Timestamps {
		tok, err := tsp.Parse(der)
		if err == nil {
			err = tok.Verify(si.SignatureValue())
		}
		if err != nil {
			k.add("SIG-13", f.path, "the signature time-stamp does not verify: %s", strings.TrimPrefix(err.Error(), "tsp: "))
			continue
		}
		if s.TimestampTime.IsZero() || tok.Time.Before(s.TimestampTime) {
			s.TimestampTime = tok.Time
		}
	}
}

// documentTimestamp checks the token of a document time-stamp.
func (k *sigCheck) documentTimestamp(f *found, contents, signed []byte) {
	s := f.sig
	if sub := s.SubFilter; sub != "ETSI.RFC3161" {
		k.add("SIG-01", f.path, "a document time-stamp has /SubFilter %s instead of ETSI.RFC3161", sub)
		return
	}
	tok, err := tsp.Parse(contents)
	if err != nil {
		k.add("SIG-01", f.path+" /Contents", "%s", strings.TrimPrefix(err.Error(), "tsp: "))
		return
	}
	f.token, f.sd = tok, tok.SignedData
	s.Certificate = tok.Certificate()
	if s.Certificate != nil {
		s.Certificates = tok.SignedData.Chain(s.Certificate)
	}
	s.Time = tok.Time
	switch err := tok.Verify(signed); {
	case errors.Is(err, tsp.ErrImprint):
		k.add("SIG-03", f.path, "the document does not match what was time-stamped: %s", strings.TrimPrefix(err.Error(), "tsp: "))
		return
	case errors.Is(err, tsp.ErrNotTSA):
		k.add("SIG-13", f.path, "%s", strings.TrimPrefix(err.Error(), "tsp: "))
	case err != nil:
		k.add("SIG-03", f.path, "%s", strings.TrimPrefix(err.Error(), "tsp: "))
		return
	}
	s.Valid = true
	k.describeCert(s, s.Certificate)
}

func (k *sigCheck) describeCert(s *Signature, cert *x509.Certificate) {
	if cert == nil {
		return
	}
	qc := esig.ParseQC(cert)
	s.Qualified, s.QSCD = qc.Qualified, qc.QSCD
	switch qc.Type {
	case esig.TypeSignature:
		s.Kind = KindSignature
	case esig.TypeSeal:
		s.Kind = KindSeal
	}
}

// decodeHex decodes the digits of a hexadecimal string, white space
// skipped; an odd final digit is followed by 0 (ISO 32000-1, 7.3.4.3).
func decodeHex(b []byte) ([]byte, bool) {
	out := make([]byte, 0, len(b)/2)
	var hi byte
	half := false
	for _, c := range b {
		var v byte
		switch {
		case '0' <= c && c <= '9':
			v = c - '0'
		case 'a' <= c && c <= 'f':
			v = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			v = c - 'A' + 10
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0:
			continue
		default:
			return nil, false
		}
		if half {
			out = append(out, hi<<4|v)
		} else {
			hi = v
		}
		half = !half
	}
	if half {
		out = append(out, hi<<4)
	}
	return out, true
}

// revisionEnd reports whether the file data ends a revision at offset end:
// end follows a %%EOF marker, possibly with its end-of-line marker.
func revisionEnd(data []byte, end int64) bool {
	for k := int64(0); k <= 2; k++ {
		e := end - k
		if e < 5 || e > int64(len(data)) {
			continue
		}
		if string(data[e-5:e]) != "%%EOF" {
			continue
		}
		eol := string(data[e:end])
		if eol == "" || eol == "\n" || eol == "\r" || eol == "\r\n" {
			return true
		}
	}
	return false
}

// changes lists the updates appended after each signature, and reports
// those that follow an approval signature (SIG-05) and those that a
// certification forbids (SIG-06).
func (k *sigCheck) changes(fs []*found, cat pdf.Dict) {
	mdp := docMDP(k.r, cat)
	var certRef pdf.Object
	if perms, ok := k.r.Dict(cat.Get("Perms")); ok {
		certRef = perms.Get("DocMDP")
	}
	for i, f := range fs {
		s := f.sig
		if s.Certification != NoCertification {
			if i > 0 {
				k.add("SIG-06", f.path, "a certification signature (DocMDP) must be the first signature of the document, but it is signature %d", i+1)
			} else if certRef == nil || !sameRef(certRef, f.ref) {
				k.add("SIG-06", f.path, "the certification signature is not the one /Perms /DocMDP of the catalog designates")
			}
		}
		if f.end < 0 || s.WholeDocument {
			continue
		}
		ch := k.changesAfter(f.end)
		s.Changes = ch.list(^change(0))
		if s.DocumentTimestamp {
			continue
		}
		level := s.Certification
		if i == 0 && level == NoCertification && mdp != NoCertification && certRef != nil && sameRef(certRef, f.ref) {
			level = mdp
		}
		if level != NoCertification && i == 0 {
			allowed := changeDSS | changeTimestamp
			switch level {
			case CertifyFormFilling:
				allowed |= changeSignature | changeForm
			case CertifyAnnotations:
				allowed |= changeSignature | changeForm | changeAnnotation
			}
			if bad := ch.list(^allowed); len(bad) > 0 {
				k.add("SIG-06", f.path, "the certification (DocMDP, %s) forbids the updates appended after it: %s", level, strings.Join(bad, ", "))
			}
			continue
		}
		if notable := ch.list(^(changeDSS | changeTimestamp | changeSignature)); len(notable) > 0 {
			k.add("SIG-05", f.path, "the document was modified after the signature, which only covers its first %d bytes: %s", f.end, strings.Join(notable, ", "))
		}
	}
}

func sameRef(a, b pdf.Object) bool {
	ra, ok1 := a.(pdf.Ref)
	rb, ok2 := b.(pdf.Ref)
	return ok1 && ok2 && ra == rb
}

// trust checks the signing certificate of f at the signing time (SIG-07 to
// SIG-12).
func (k *sigCheck) trust(f *found, fs []*found) {
	s := f.sig
	cert := s.Certificate
	if !s.Valid || cert == nil {
		return
	}
	// The time a time-stamp proves: that of the signature time-stamp, or
	// of the first document time-stamp that covers the signature.
	proof := s.TimestampTime
	if s.DocumentTimestamp {
		proof = s.Time
	}
	if proof.IsZero() {
		for _, g := range fs {
			if g.sig.DocumentTimestamp && g.sig.Valid && g.end > f.end {
				proof = g.sig.Time
				break
			}
		}
	}
	at := proof
	if at.IsZero() {
		at = s.Time
	}
	if at.IsZero() {
		at = k.opts.At
	}
	intermediates := append(append([]*x509.Certificate(nil), k.opts.Intermediates...), k.dss.certs...)
	res := esig.Check(cert, s.Certificates, &esig.Options{
		Roots:         k.opts.Roots,
		Intermediates: intermediates,
		At:            at,
		CRLs:          append(append([]*x509.RevocationList(nil), k.opts.CRLs...), k.dss.crls...),
		OCSP:          append(append([][]byte(nil), k.opts.OCSP...), k.dss.ocsp...),
	})
	subject := cert.Subject.String()
	when := "at the signing time"
	if !proof.IsZero() {
		when = "at the time of the time-stamp"
	}
	switch {
	case res.Expired:
		k.add("SIG-07", f.path, "the signing certificate (%s) expired on %s, before %s, %s", subject, cert.NotAfter.UTC().Format(time.RFC3339), when, at.UTC().Format(time.RFC3339))
	case res.NotYetValid:
		k.add("SIG-07", f.path, "the signing certificate (%s) is not valid before %s, after %s, %s", subject, cert.NotBefore.UTC().Format(time.RFC3339), when, at.UTC().Format(time.RFC3339))
	case proof.IsZero() && (k.opts.At.After(cert.NotAfter) || k.opts.At.Before(cert.NotBefore)):
		k.add("SIG-08", f.path, "the signing certificate (%s) is not valid at %s, valid from %s to %s, and no time-stamp proves that the signature was made before it expired", subject,
			k.opts.At.UTC().Format(time.RFC3339), cert.NotBefore.UTC().Format(time.RFC3339), cert.NotAfter.UTC().Format(time.RFC3339))
	}
	if res.KeyUsage != nil && !s.DocumentTimestamp {
		k.add("SIG-07", f.path, "%v", res.KeyUsage)
	}
	switch {
	case res.Untrusted:
		k.add("SIG-09", f.path, "the signing certificate (%s, issued by %s) does not lead to a trusted root", subject, cert.Issuer.String())
	case res.ChainErr != nil:
		k.add("SIG-09", f.path, "the chain of the signing certificate (%s) is not valid: %v", subject, res.ChainErr)
	case !res.TrustChecked && !k.trustMsg:
		k.trustMsg = true
		k.add("SIG-10", "", "no trusted root given: the chains of the signing certificates are not checked")
	}
	switch res.Revocation.Status {
	case esig.StatusRevoked:
		k.add("SIG-11", f.path, "the signing certificate (%s) was revoked on %s (%s)", subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
	case esig.StatusSuspended:
		k.add("SIG-11", f.path, "the signing certificate (%s) is suspended since %s (%s)", subject, res.Revocation.Time.UTC().Format(time.RFC3339), res.Revocation.Source)
	case esig.StatusUnknown:
		msg := "no CRL or OCSP response covers the signing certificate (" + subject + "): its revocation is not checked"
		for _, e := range res.Revocation.Errors {
			msg += "; " + e.Error()
		}
		k.add("SIG-12", f.path, "%s", msg)
	}
}

// levels sets the PAdES baseline level of each signature.
func (k *sigCheck) levels(fs []*found) {
	for _, f := range fs {
		s := f.sig
		if !s.Valid || s.DocumentTimestamp || len(f.pades) > 0 {
			continue
		}
		s.Level = LevelBB
		var covering []*found // valid document time-stamps after the signature
		for _, g := range fs {
			if g.sig.DocumentTimestamp && g.sig.Valid && g.end > f.end {
				covering = append(covering, g)
			}
		}
		if s.TimestampTime.IsZero() && len(covering) == 0 {
			continue
		}
		s.Level = LevelBT
		if !k.dss.present || !k.revocationInDSS(s, s.TimestampTime) {
			continue
		}
		s.Level = LevelBLT
		for _, g := range covering {
			if k.hasDSSAt(g.end) {
				s.Level = LevelBLTA
				break
			}
		}
	}
}

// revocationInDSS reports whether the document security store alone gives
// the revocation status of the signing certificate.
func (k *sigCheck) revocationInDSS(s *Signature, at time.Time) bool {
	if at.IsZero() {
		at = s.Time
	}
	if at.IsZero() {
		at = k.opts.At
	}
	res := esig.Check(s.Certificate, append(append([]*x509.Certificate(nil), s.Certificates...), k.dss.certs...), &esig.Options{
		At: at, CRLs: k.dss.crls, OCSP: k.dss.ocsp,
	})
	return res.Revocation.Status == esig.StatusGood
}

// hasDSSAt reports whether the revision ending at end has a document
// security store.
func (k *sigCheck) hasDSSAt(end int64) bool {
	if v, ok := k.dssAt[end]; ok {
		return v
	}
	if k.dssAt == nil {
		k.dssAt = map[int64]bool{}
	}
	v := false
	if old, err := pdf.Open(k.data[:end]); err == nil {
		_, v = old.Dict(old.Catalog().Get("DSS"))
	}
	k.dssAt[end] = v
	return v
}

// describe reports the signature (SIG-14).
func (k *sigCheck) describe(f *found) {
	s := f.sig
	var parts []string
	what := "signature"
	if s.DocumentTimestamp {
		what = "document time-stamp"
	}
	who := "unknown signer"
	if s.Certificate != nil {
		who = s.Certificate.Subject.String()
		var tags []string
		if s.Kind != KindUnknown {
			tags = append(tags, s.Kind.String())
		}
		if s.Qualified {
			tags = append(tags, "qualified")
		}
		if s.QSCD {
			tags = append(tags, "QSCD")
		}
		if len(tags) > 0 {
			who += " (" + strings.Join(tags, ", ") + ")"
		}
	}
	parts = append(parts, who)
	if !s.Time.IsZero() {
		parts = append(parts, "at "+s.Time.UTC().Format(time.RFC3339))
	}
	if !s.TimestampTime.IsZero() {
		parts = append(parts, "time-stamped at "+s.TimestampTime.UTC().Format(time.RFC3339))
	}
	if s.Level != "" {
		parts = append(parts, "PAdES "+s.Level)
	}
	if s.Certification != NoCertification {
		parts = append(parts, s.Certification.String())
	}
	switch {
	case !s.Valid:
		parts = append(parts, "does not verify")
	case s.WholeDocument:
		parts = append(parts, "covers the whole document")
	default:
		c := "covers the first " + fmt.Sprint(s.SignedLength) + " bytes"
		if len(s.Changes) > 0 {
			c += "; updates after it: " + strings.Join(s.Changes, ", ")
		}
		parts = append(parts, c)
	}
	k.add("SIG-14", f.path, "%s: %s", what, strings.Join(parts, ", "))
}

// change is a kind of update appended after a signature.
type change uint8

const (
	changeDSS        change = 1 << iota // validation data
	changeTimestamp                     // document time-stamps
	changeSignature                     // signature fields, their values and widgets
	changeForm                          // other form fields
	changeAnnotation                    // annotations
	changeOther                         // anything else
)

// changeSet is the set of updates after a signature, by description.
type changeSet map[string]change

func (c changeSet) add(k change, desc string) { c[desc] |= k }

// list returns the descriptions of the changes of the kinds of mask.
func (c changeSet) list(mask change) []string {
	var out []string
	for desc, k := range c {
		if k&mask != 0 {
			out = append(out, desc)
		}
	}
	slices.Sort(out)
	return out
}

// changeContext classifies the objects of the document: what a reference
// designates, for the objects appended after a signature.
type changeContext struct {
	root, info, metadata, form, fieldsArr uint32
	dss                                   map[uint32]bool
	sig                                   map[uint32]change // signature and time-stamp fields, widgets, values, appearances
	fields                                map[uint32]bool   // other fields and widgets
	pages                                 map[uint32]bool
	annotsArr                             map[uint32]bool
	names                                 map[uint32]bool // objects of the name trees and of /AF
}

func (k *sigCheck) context() *changeContext {
	if k.ctx != nil {
		return k.ctx
	}
	r := k.r
	cx := &changeContext{
		dss: map[uint32]bool{}, sig: map[uint32]change{}, fields: map[uint32]bool{},
		pages: map[uint32]bool{}, annotsArr: map[uint32]bool{}, names: map[uint32]bool{},
	}
	num := func(o pdf.Object) uint32 {
		if ref, ok := o.(pdf.Ref); ok {
			return ref.Num
		}
		return 0
	}
	cx.root = num(r.Trailer().Get("Root"))
	cx.info = num(r.Trailer().Get("Info"))
	cat := r.Catalog()
	cx.metadata = num(cat.Get("Metadata"))
	cx.form = num(cat.Get("AcroForm"))
	if form, ok := r.Dict(cat.Get("AcroForm")); ok {
		cx.fieldsArr = num(form.Get("Fields"))
	}
	reach(r, cat.Get("DSS"), 5, cx.dss)
	reach(r, cat.Get("Names"), 8, cx.names)
	reach(r, cat.Get("AF"), 4, cx.names)
	walkFields(r, cat, func(_ string, o pdf.Object, d pdf.Dict, ft pdf.Name) {
		n := num(o)
		if ft != "Sig" {
			if n != 0 {
				cx.fields[n] = true
			}
			return
		}
		kind := changeSignature
		if v, ok := r.Dict(d.Get("V")); ok {
			if t, _ := v.Name("Type"); t == "DocTimeStamp" {
				kind = changeTimestamp
			}
		}
		if n != 0 {
			cx.sig[n] = kind
		}
		marked := map[uint32]bool{}
		reach(r, d.Get("V"), 2, marked)
		reach(r, d.Get("AP"), 3, marked)
		for m := range marked {
			cx.sig[m] = kind
		}
	})
	// The pages and their /Annots arrays.
	seen := map[uint32]bool{}
	var walk func(o pdf.Object, depth int)
	walk = func(o pdf.Object, depth int) {
		n := num(o)
		if n == 0 || seen[n] || depth > 64 || len(seen) > 1<<20 {
			return
		}
		seen[n] = true
		d, ok := r.Dict(o)
		if !ok {
			return
		}
		if kids, ok := r.Resolve(d.Get("Kids")).(pdf.Array); ok {
			for _, kid := range kids {
				walk(kid, depth+1)
			}
			return
		}
		cx.pages[n] = true
		if a := num(d.Get("Annots")); a != 0 {
			cx.annotsArr[a] = true
		}
	}
	walk(cat.Get("Pages"), 0)
	k.ctx = cx
	return cx
}

// reach marks the objects reachable from o by references, to a depth.
func reach(r *pdf.Reader, o pdf.Object, depth int, marked map[uint32]bool) {
	if depth < 0 || len(marked) > 1<<16 {
		return
	}
	if ref, ok := o.(pdf.Ref); ok {
		if marked[ref.Num] {
			return
		}
		marked[ref.Num] = true
		o = r.Resolve(ref)
	}
	switch v := o.(type) {
	case pdf.Dict:
		for _, kv := range v {
			reach(r, kv.Val, depth-1, marked)
		}
	case *pdf.Stream:
		for _, kv := range v.Dict {
			reach(r, kv.Val, depth-1, marked)
		}
	case pdf.Array:
		for _, x := range v {
			reach(r, x, depth-1, marked)
		}
	}
}

// changesAfter classifies the objects that the updates after the revision
// ending at end add or replace.
func (k *sigCheck) changesAfter(end int64) changeSet {
	ch := changeSet{}
	old, err := pdf.Open(k.data[:end])
	if err != nil {
		ch.add(changeOther, "the signed revision cannot be read")
		return ch
	}
	nums, ok := k.r.UpdatedAfter(end)
	if !ok {
		ch.add(changeOther, "unknown updates (damaged cross-reference data)")
		return ch
	}
	cx := k.context()
	for i, n := range nums {
		if i >= 1<<16 {
			ch.add(changeOther, "other objects")
			break
		}
		no, oo := k.r.Object(n), old.Object(n)
		if st, ok := no.(*pdf.Stream); ok {
			if t, _ := st.Dict.Name("Type"); t == "XRef" || t == "ObjStm" {
				continue
			}
		}
		if oo != nil && sameObject(old, k.r, oo, no, 0) {
			continue
		}
		k.classify(ch, cx, old, n, oo, no)
	}
	return ch
}

// classify records the nature of the update of object n, from oo (nil
// when the object is new) to no.
func (k *sigCheck) classify(ch changeSet, cx *changeContext, old *pdf.Reader, n uint32, oo, no pdf.Object) {
	r := k.r
	nd, _ := pdf.AsDict(no)
	od, _ := pdf.AsDict(oo)
	switch {
	case cx.dss[n]:
		ch.add(changeDSS, "validation data (DSS)")
	case cx.sig[n] != 0:
		ch.add(cx.sig[n], kindName(cx.sig[n]))
	case n == cx.root:
		for _, key := range unionKeys(od, nd) {
			if sameObject(old, r, od.Get(key), nd.Get(key), 0) {
				continue
			}
			switch key {
			case "DSS":
				ch.add(changeDSS, "validation data (DSS)")
			case "AcroForm":
				ofd, _ := old.Dict(od.Get(key))
				nfd, _ := r.Dict(nd.Get(key))
				k.formChanges(ch, cx, old, ofd, nfd)
			case "AF", "Names":
				ch.add(changeOther, "embedded files")
			case "Metadata":
				ch.add(changeOther, "XMP metadata")
			default:
				ch.add(changeOther, "catalog (/"+string(key)+")")
			}
		}
	case cx.pages[n]:
		for _, key := range unionKeys(od, nd) {
			if sameObject(old, r, od.Get(key), nd.Get(key), 0) {
				continue
			}
			if key == "Annots" {
				oa, _ := old.Resolve(od.Get(key)).(pdf.Array)
				na, _ := r.Resolve(nd.Get(key)).(pdf.Array)
				k.arrayChanges(ch, cx, oa, na, changeAnnotation)
				continue
			}
			ch.add(changeOther, "pages (content or resources)")
		}
	case cx.annotsArr[n]:
		oa, _ := oo.(pdf.Array)
		na, _ := no.(pdf.Array)
		k.arrayChanges(ch, cx, oa, na, changeAnnotation)
	case n == cx.form:
		k.formChanges(ch, cx, old, od, nd)
	case n == cx.fieldsArr:
		oa, _ := oo.(pdf.Array)
		na, _ := no.(pdf.Array)
		k.arrayChanges(ch, cx, oa, na, changeForm)
	case cx.fields[n]:
		ch.add(changeForm, "form fields")
	case n == cx.info:
		ch.add(changeOther, "document information (/Info)")
	case n == cx.metadata:
		ch.add(changeOther, "XMP metadata")
	case cx.names[n] || hasType(nd, "Filespec", "EmbeddedFile"):
		ch.add(changeOther, "embedded files")
	case hasType(nd, "Metadata"):
		ch.add(changeOther, "XMP metadata")
	case hasType(nd, "Annot") || nd.Has("Subtype") && nd.Has("Rect"):
		ch.add(changeAnnotation, "annotations")
	default:
		ch.add(changeOther, "other objects")
	}
}

func kindName(c change) string {
	switch c {
	case changeTimestamp:
		return "document time-stamps"
	case changeSignature:
		return "signature fields"
	case changeDSS:
		return "validation data (DSS)"
	case changeForm:
		return "form fields"
	case changeAnnotation:
		return "annotations"
	}
	return "other objects"
}

// formChanges classifies the changes of the interactive form dictionary.
func (k *sigCheck) formChanges(ch changeSet, cx *changeContext, old *pdf.Reader, od, nd pdf.Dict) {
	for _, key := range unionKeys(od, nd) {
		if sameObject(old, k.r, od.Get(key), nd.Get(key), 0) {
			continue
		}
		switch key {
		case "Fields":
			oa, _ := old.Resolve(od.Get(key)).(pdf.Array)
			na, _ := k.r.Resolve(nd.Get(key)).(pdf.Array)
			k.arrayChanges(ch, cx, oa, na, changeForm)
		case "SigFlags":
			ch.add(changeSignature, "signature fields")
		default:
			ch.add(changeForm, "form fields")
		}
	}
}

// arrayChanges classifies the elements added to or removed from an array
// of annotations or fields: signature and time-stamp fields are recorded
// as such, other fields as form fields, the rest as kind.
func (k *sigCheck) arrayChanges(ch changeSet, cx *changeContext, oa, na pdf.Array, kind change) {
	had := map[pdf.Ref]bool{}
	for _, x := range oa {
		if ref, ok := x.(pdf.Ref); ok {
			had[ref] = true
		}
	}
	has := map[pdf.Ref]bool{}
	for _, x := range na {
		ref, ok := x.(pdf.Ref)
		if !ok {
			ch.add(kind, kindName(kind))
			continue
		}
		has[ref] = true
		if had[ref] {
			continue
		}
		switch {
		case cx.sig[ref.Num] != 0:
			ch.add(cx.sig[ref.Num], kindName(cx.sig[ref.Num]))
		case cx.fields[ref.Num]:
			ch.add(changeForm, "form fields")
		default:
			ch.add(kind, kindName(kind))
		}
	}
	for ref := range had {
		if !has[ref] {
			ch.add(kind, kindName(kind)+" (removed)")
		}
	}
}

func hasType(d pdf.Dict, types ...pdf.Name) bool {
	t, ok := d.Name("Type")
	return ok && slices.Contains(types, t)
}

func unionKeys(a, b pdf.Dict) []pdf.Name {
	var keys []pdf.Name
	seen := map[pdf.Name]bool{}
	for _, d := range []pdf.Dict{a, b} {
		for _, kv := range d {
			if !seen[kv.Key] {
				seen[kv.Key] = true
				keys = append(keys, kv.Key)
			}
		}
	}
	return keys
}

// sameObject reports whether two objects, read from ra and rb, are equal:
// references by number, dictionaries whatever the order of their entries,
// streams by dictionary and encoded data.
func sameObject(ra, rb *pdf.Reader, a, b pdf.Object, depth int) bool {
	if depth > 32 {
		return false
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case pdf.Ref:
		y, ok := b.(pdf.Ref)
		return ok && x == y
	case pdf.String:
		y, ok := b.(pdf.String)
		return ok && bytes.Equal(x, y)
	case pdf.Array:
		y, ok := b.(pdf.Array)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !sameObject(ra, rb, x[i], y[i], depth+1) {
				return false
			}
		}
		return true
	case pdf.Dict:
		y, ok := b.(pdf.Dict)
		return ok && sameDictionary(ra, rb, x, y, depth)
	case *pdf.Stream:
		y, ok := b.(*pdf.Stream)
		return ok && bytes.Equal(x.Raw, y.Raw) && sameDictionary(ra, rb, x.Dict, y.Dict, depth)
	default:
		return a == b
	}
}

func sameDictionary(ra, rb *pdf.Reader, x, y pdf.Dict, depth int) bool {
	count := func(d pdf.Dict) int {
		n := 0
		for _, kv := range d {
			if kv.Val != nil {
				n++
			}
		}
		return n
	}
	if count(x) != count(y) {
		return false
	}
	for _, kv := range x {
		if kv.Val != nil && !sameObject(ra, rb, kv.Val, y.Get(kv.Key), depth+1) {
			return false
		}
	}
	return true
}
