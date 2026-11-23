// Command zerechnung reads, validates and creates Factur-X / ZUGFeRD /
// XRechnung invoices. validate and json also accept a CII or UBL 2.1 XML
// invoice, a FatturaPA document (Italy) and a Facturae document (Spain),
// whose syntax is detected from the root element; validate checks a
// FatturaPA or Facturae document against its XML schema, and a FatturaPA
// document against the checks of the SDI on its content too, describes the
// XAdES signature of a Facturae document, which -face requires and checks
// as FACe does, and json also opens a FatturaPA file signed in CAdES
// (.xml.p7m).
//
//	zerechnung extract [-o out.xml] invoice.pdf
//	zerechnung info invoice.pdf
//	zerechnung validate [-fr] [-de] [-peppol] [-nl] [-it] [-pt] [-ro] [-sk] [-hr] [-face] [-auto] [-reject-basicwl] [-choruspro [-service-required] [-engagement-required]] [-profile p] [-hybrid] [-pdfa full|basic] [-roots roots.pem] [-json] invoice.pdf|invoice.xml
//	zerechnung embed -xml invoice.xml [-profile p] [-attach file]... [-require-pdfa] [-o out.pdf] input.pdf
//	zerechnung sign -key key.pem -cert cert.pem [-reason r] [-location l] [-certify 1|2|3] [-o out.pdf] input.pdf
//	zerechnung json invoice.pdf|invoice.xml|invoice.xml.p7m
//	zerechnung rules [-fr] [-de] [-choruspro] [-peppol] [-nl] [-it] [-pt] [-ro] [-sk] [-hr] [-face]
//
// validate exits with status 1 when the invoice has errors. It checks the
// signatures of a signed PDF, and with -face the signature of a Facturae
// invoice, against the trusted roots of -roots when given. sign signs a PDF in PAdES Baseline B-B with a key and its
// certificate chain in PEM files.
package main

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

const usage = `zerechnung reads, validates and creates Factur-X / ZUGFeRD / XRechnung invoices.
validate and json also accept an XML invoice in CII or UBL 2.1 syntax, a
FatturaPA document and a Facturae document: validate checks them against
their XML schema, and FatturaPA against the checks of the SDI on its content
too, and describe the XAdES signature of a Facturae document (-face requires
it and checks it as FACe does), json gives an array when one holds several
invoices and also opens a FatturaPA file signed in CAdES (.xml.p7m).

Usage:
  zerechnung extract [-o out.xml] invoice.pdf
  zerechnung info invoice.pdf
  zerechnung validate [-fr] [-de] [-peppol] [-nl] [-it] [-pt] [-ro] [-sk] [-hr] [-face] [-auto] [-reject-basicwl] [-choruspro [-service-required] [-engagement-required]] [-profile p] [-hybrid] [-pdfa full|basic] [-roots roots.pem] [-json] invoice.pdf|invoice.xml
  zerechnung embed -xml invoice.xml [-profile p] [-attach file]... [-require-pdfa] [-convert-pdfa] [-o out.pdf] input.pdf
  zerechnung sign -key key.pem -cert cert.pem [-reason r] [-location l] [-contact c] [-name n] [-field f] [-certify 1|2|3] [-hash sha256|sha384|sha512] [-pss] [-o out.pdf] input.pdf
  zerechnung pdfa [-o out.pdf] input.pdf
  zerechnung json invoice.pdf|invoice.xml|invoice.xml.p7m
  zerechnung rules [-fr] [-de] [-choruspro] [-peppol] [-nl] [-it] [-pt] [-ro] [-sk] [-hr] [-face]

Profiles: minimum, basicwl, basic, en16931, extended, xrechnung, extended-ctc-fr, peppol, nlcius, cius-it, cius-pt, cius-ro, cius-hr.
`

// stderr receives the usage, the errors and the warnings of the commands.
var stderr io.Writer = os.Stderr

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	code := 0
	switch cmd {
	case "extract":
		err = extract(args)
	case "info":
		err = info(args)
	case "validate":
		code, err = validateCmd(args, os.Stdout)
	case "embed":
		err = embed(args)
	case "sign":
		err = signCmd(args)
	case "pdfa":
		err = pdfaCmd(args)
	case "json":
		err = jsonCmd(args, os.Stdout)
	case "rules":
		err = rulesCmd(args)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(stderr, "zerechnung: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(stderr, "zerechnung:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func oneFile(fs *flag.FlagSet, args []string) (string, []byte, error) {
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if fs.NArg() != 1 {
		return "", nil, fmt.Errorf("%s: expected one file, got %d", fs.Name(), fs.NArg())
	}
	name := fs.Arg(0)
	var data []byte
	var err error
	if name == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(name)
	}
	return name, data, err
}

func isPDF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\t\n\r "), []byte("%PDF-"))
}

func writeOut(path string, data []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func extract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: standard output)")
	_, data, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	att, err := zerechnung.Extract(data)
	if err != nil {
		return err
	}
	return writeOut(*out, att.XML)
}

func info(args []string) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	_, data, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	att, err := zerechnung.Extract(data)
	if err != nil {
		return err
	}
	w := os.Stdout
	fmt.Fprintf(w, "PDF version:        %s\n", att.PDFVersion)
	fmt.Fprintf(w, "Attachment:         %s (%d bytes, %s)\n", att.Filename, len(att.XML), orDash(att.MimeType))
	fmt.Fprintf(w, "AFRelationship:     %s (in /AF: %v)\n", orDash(att.Relationship), att.InAF)
	if !att.ModDate.IsZero() {
		fmt.Fprintf(w, "Modification date:  %s\n", att.ModDate.Format(time.RFC3339))
	}
	if m := att.Metadata; m != nil {
		if m.PDFARev != "" {
			fmt.Fprintf(w, "PDF/A:              %d%s (rev %s)\n", m.PDFAPart, m.PDFAConformance, m.PDFARev)
		} else {
			fmt.Fprintf(w, "PDF/A:              %d%s\n", m.PDFAPart, m.PDFAConformance)
		}
		fmt.Fprintf(w, "XMP namespace:      %s\n", m.NamespaceURI)
		fmt.Fprintf(w, "XMP profile:        %s (document %s, file %s, version %s)\n", m.ConformanceLevel, m.DocumentType, m.DocumentFileName, m.Version)
	} else {
		fmt.Fprintln(w, "XMP:                none")
	}
	inv, err := cii.Parse(att.XML)
	if inv == nil {
		return err
	}
	if e := invoice.EditionOf(inv.SpecificationID); !e.Supported() {
		fmt.Fprintf(w, "Profile (BT-24):    %s, %s (edition not supported)\n", inv.Profile, e)
	} else {
		fmt.Fprintf(w, "Profile (BT-24):    %s\n", inv.Profile)
	}
	fmt.Fprintf(w, "Invoice:            %s, type %s, issued %s\n", inv.Number, inv.TypeCode, inv.IssueDate)
	fmt.Fprintf(w, "Seller:             %s (%s)\n", inv.Seller.Name, inv.Seller.Address.CountryCode)
	fmt.Fprintf(w, "Buyer:              %s (%s)\n", inv.Buyer.Name, inv.Buyer.Address.CountryCode)
	fmt.Fprintf(w, "Total:              %s %s without VAT, %s with VAT, %s due\n", inv.Totals.TaxBasisTotal, inv.Currency, inv.Totals.GrandTotal, inv.Totals.DuePayable)
	fmt.Fprintf(w, "Lines:              %d\n", len(inv.Lines))
	return nil
}

// validateCmd runs the validate command and writes the report to w.
func validateCmd(args []string, w io.Writer) (int, error) {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fr := fs.Bool("fr", false, "apply the French rules BR-FR and the profiles the French reform admits")
	de := fs.Bool("de", false, "apply the German rules BR-DE (XRechnung) and the profiles the BMF accepts (BR-DE applied anyway to XRECHNUNG invoices)")
	auto := fs.Bool("auto", false, "apply the national rules of the seller's country")
	rejectBasicWL := fs.Bool("reject-basicwl", false, "report the profile BASIC WL as an error under the French rules")
	cpro := fs.Bool("choruspro", false, "apply the Chorus Pro checks of French B2G invoices")
	serviceRequired := fs.Bool("service-required", false, "with -choruspro: the public buyer requires a service code (BT-10)")
	engagementRequired := fs.Bool("engagement-required", false, "with -choruspro: the public buyer requires an engagement number (BT-13)")
	pep := fs.Bool("peppol", false, "apply the Peppol BIS Billing 3.0 rules (applied anyway when BT-24 is the Peppol identifier)")
	nl := fs.Bool("nl", false, "apply the Dutch rules BR-NL of NLCIUS (applied anyway when BT-24 is the NLCIUS identifier)")
	ita := fs.Bool("it", false, "apply the Italian rules BR-IT of the CIUS-IT (applied anyway when BT-24 is a CIUS-IT identifier)")
	por := fs.Bool("pt", false, "apply the Portuguese rules of the CIUS-PT (applied anyway when BT-24 is a CIUS-PT identifier)")
	rou := fs.Bool("ro", false, "apply the Romanian rules BR-RO of the CIUS-RO and the check of the CIF (applied anyway when BT-24 is a CIUS-RO identifier)")
	svk := fs.Bool("sk", false, "apply the Slovak requirements of the eFaktura beyond Peppol BIS (SK-*; applied with -auto to a seller in Slovakia)")
	cro := fs.Bool("hr", false, "apply the Croatian rules HR-BR of the CIUS-HR and the check of the OIB (applied anyway when BT-24 is a CIUS-HR identifier)")
	face := fs.Bool("face", false, "apply the FACe checks of Spanish B2G invoices: a Facturae document must be signed in XAdES-EPES under the signature policy of Facturae, and its signature verify (trust in the certificate and its revocation are not checked)")
	profile := fs.String("profile", "", "validate against this profile instead of BT-24")
	hybrid := fs.Bool("hybrid", false, "the XML is the invoice of a Factur-X PDF: check an EN 16931 invoice against the Factur-X schematron (always the case for a PDF)")
	pdfa := fs.String("pdfa", "full", "PDF/A checks of a PDF: full, or basic to skip those that read the pages and the font programs (PDF-41 to PDF-69, PDF-71, PDF-72), about ten times faster")
	roots := fs.String("roots", "", "PEM file of the trusted root certificates the signatures of a PDF, and of a Facturae invoice with -face, are checked against")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	_, data, err := oneFile(fs, args)
	if err != nil {
		return 0, err
	}
	opts := &validate.Options{Auto: *auto, RejectBasicWL: *rejectBasicWL, Hybrid: *hybrid}
	if *roots != "" {
		b, err := os.ReadFile(*roots)
		if err != nil {
			return 0, err
		}
		certs, err := parseCertificates(b)
		if err != nil {
			return 0, fmt.Errorf("-roots: %w", err)
		}
		opts.Trust.Roots = x509.NewCertPool()
		for _, c := range certs {
			opts.Trust.Roots.AddCert(c)
		}
	}
	switch *pdfa {
	case "full":
		opts.PDFA = validate.PDFAFull
	case "basic":
		opts.PDFA = validate.PDFABasic
	default:
		return 0, fmt.Errorf("unknown PDF/A check level %q (full or basic)", *pdfa)
	}
	if *profile != "" {
		if opts.Profile = invoice.ParseProfile(*profile); opts.Profile == invoice.ProfileUnknown {
			return 0, fmt.Errorf("unknown profile %q", *profile)
		}
	}
	if *fr || *de || *cpro || *pep || *nl || *ita || *por || *rou || *svk || *cro || *face {
		opts.Rules = validate.Default
		if *fr {
			opts.Rules |= validate.France
		}
		if *de {
			opts.Rules |= validate.Germany | validate.GermanyProfile
		}
		if *cpro {
			opts.Rules |= validate.ChorusPro
		}
		if *pep {
			opts.Rules |= validate.Peppol
		}
		if *nl {
			opts.Rules |= validate.Netherlands
		}
		if *ita {
			opts.Rules |= validate.Italy
		}
		if *por {
			opts.Rules |= validate.Portugal
		}
		if *rou {
			opts.Rules |= validate.Romania
		}
		if *svk {
			opts.Rules |= validate.Slovakia
		}
		if *cro {
			opts.Rules |= validate.Croatia
		}
		if *face {
			opts.Rules |= validate.FACe
		}
	}
	opts.PublicBuyer = validate.PublicBuyer{ServiceCodeRequired: *serviceRequired, EngagementRequired: *engagementRequired}
	return report(data, opts, *asJSON, w)
}

// report validates data, a PDF or an XML invoice, writes the report to w,
// as JSON when asJSON is set, and returns the exit code of validate.
func report(data []byte, opts *validate.Options, asJSON bool, w io.Writer) (int, error) {
	var (
		r   *validate.Report
		err error
	)
	var format string            // the format of a FatturaPA (FPA12 or FPR12) or Facturae (3.2, 3.2.1 or 3.2.2) document
	var signature *signatureInfo // the signature of a Facturae document
	if isPDF(data) {
		r, _, err = zerechnung.Check(data, opts)
		if err != nil {
			return 0, err
		}
	} else {
		r = validate.XML(data, opts)
		if root, perr := xmltree.Parse(data); perr == nil && fatturapa.IsFatturaPA(root) {
			format = strings.TrimSpace("FatturaPA " + root.Attr("versione"))
		} else if perr == nil && facturae.IsFacturae(root) {
			format = "Facturae " + facturae.Version(root)
			signature = facturaeSignature(data)
		}
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		out := struct {
			Profile   string             `json:"profile"`
			Format    string             `json:"format,omitempty"`
			Signature *signatureInfo     `json:"signature,omitempty"`
			Valid     bool               `json:"valid"`
			Findings  []validate.Finding `json:"findings"`
		}{r.Profile.String(), format, signature, r.Valid(), r.Findings}
		if out.Findings == nil {
			out.Findings = []validate.Finding{}
		}
		if err := enc.Encode(out); err != nil {
			return 0, err
		}
	} else {
		if format != "" {
			fmt.Fprintf(w, "Format: %s\n", format)
		} else {
			fmt.Fprintf(w, "Profile: %s\n", r.Profile)
		}
		if signature != nil {
			fmt.Fprintf(w, "Signature: %s\n", signature)
		}
		for _, f := range r.Findings {
			fmt.Fprintln(w, f)
		}
		fmt.Fprintf(w, "%d error(s), %d warning(s)\n", len(r.Errors()), len(r.Warnings()))
	}
	if !r.Valid() {
		return 1, nil
	}
	return 0, nil
}

// signatureInfo describes the XAdES signature of a Facturae document, once
// its integrity is checked (facturae.ReadSignature).
type signatureInfo struct {
	Signer    string     `json:"signer,omitempty"`
	Time      *time.Time `json:"time,omitempty"`
	Role      string     `json:"role,omitempty"`
	Policy    string     `json:"policy,omitempty"`
	Qualified bool       `json:"qualified"`
	Error     string     `json:"error,omitempty"`
}

// facturaeSignature describes the signature of a Facturae document, nil
// when it is not signed.
func facturaeSignature(data []byte) *signatureInfo {
	sig, err := facturae.ReadSignature(data)
	switch {
	case errors.Is(err, facturae.ErrNotSigned):
		return nil
	case err != nil:
		return &signatureInfo{Error: strings.TrimPrefix(err.Error(), "facturae: ")}
	}
	info := &signatureInfo{Signer: sig.Certificate.Subject.String(), Role: string(sig.Role), Qualified: sig.Qualified}
	if t := sig.SigningTime; !t.IsZero() {
		info.Time = &t
	}
	if sig.Policy != nil {
		info.Policy = sig.Policy.Identifier
	}
	return info
}

func (s *signatureInfo) String() string {
	if s.Error != "" {
		return "not valid: " + s.Error
	}
	out := "XAdES by " + s.Signer
	if s.Time != nil {
		out += ", signed " + s.Time.UTC().Format(time.RFC3339)
	}
	if s.Role != "" {
		out += ", role " + s.Role
	}
	switch s.Policy {
	case "":
		out += ", no signature policy"
	case facturae.PolicyIdentifier:
		out += ", policy of Facturae v3.1"
	default:
		out += ", policy " + s.Policy
	}
	if s.Qualified {
		out += ", qualified certificate"
	}
	return out + " (integrity checked; -face checks it as FACe does)"
}

// fileList is a repeatable flag.
type fileList []string

func (l *fileList) String() string     { return strings.Join(*l, ",") }
func (l *fileList) Set(s string) error { *l = append(*l, s); return nil }

func embed(args []string) error {
	fs := flag.NewFlagSet("embed", flag.ContinueOnError)
	xmlPath := fs.String("xml", "", "invoice XML to embed (required)")
	profile := fs.String("profile", "", "profile (default: read from BT-24)")
	rel := fs.String("relationship", "", "AFRelationship (default: Data for MINIMUM and BASIC WL, Alternative otherwise; Germany and XRECHNUNG admit only Alternative)")
	var attach fileList
	fs.Var(&attach, "attach", "supplementary file to embed besides the invoice, with AFRelationship Supplement and the MIME type of its extension (repeatable)")
	requirePDFA := fs.Bool("require-pdfa", false, "refuse an input PDF whose XMP metadata does not declare PDF/A-2, PDF/A-3 or PDF/A-4")
	convertPDFA := fs.Bool("convert-pdfa", false, "convert to PDF/A-3 an input PDF whose XMP metadata does not declare PDF/A-2, PDF/A-3 or PDF/A-4 (see the pdfa command)")
	out := fs.String("o", "", "output PDF (default: standard output)")
	_, pdf, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	if *xmlPath == "" {
		return errors.New("embed: -xml is required")
	}
	xml, err := os.ReadFile(*xmlPath)
	if err != nil {
		return err
	}
	opts := &zerechnung.EmbedOptions{Relationship: *rel, RequirePDFA: *requirePDFA, ConvertPDFA: *convertPDFA}
	for _, p := range attach {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		opts.Attachments = append(opts.Attachments, zerechnung.File{Name: filepath.Base(p), Data: data})
	}
	if *profile != "" {
		if opts.Profile = invoice.ParseProfile(*profile); opts.Profile == invoice.ProfileUnknown {
			return fmt.Errorf("unknown profile %q", *profile)
		}
	}
	res, err := zerechnung.Embed(pdf, xml, opts)
	if err != nil {
		return reasons(err)
	}
	if _, sigs, err := zerechnung.VerifySignatures(pdf, nil); err == nil && len(sigs) > 0 {
		fmt.Fprintf(stderr, "zerechnung: warning: the input PDF holds %d signature(s); they do not cover the invoice embedded after them (SIG-05): sign the hybrid PDF instead\n", len(sigs))
	}
	return writeOut(*out, res)
}

// signCmd signs a PDF in PAdES Baseline B-B, or certifies it with
// -certify.
func signCmd(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	keyPath := fs.String("key", "", "PEM file of the private key, RSA or ECDSA, unencrypted (PKCS#8, PKCS#1 or SEC 1) (required)")
	certPath := fs.String("cert", "", "PEM file of the signing certificate, followed by the certificates of its chain (required)")
	reason := fs.String("reason", "", "reason of the signature (/Reason)")
	location := fs.String("location", "", "place of signing (/Location)")
	contact := fs.String("contact", "", "contact information of the signer (/ContactInfo)")
	name := fs.String("name", "", "name of the signer (/Name)")
	field := fs.String("field", "", "name of the signature field (default: the first free SignatureN)")
	certify := fs.Int("certify", 0, "certify the document (DocMDP): 1 permits no change, 2 form filling and signing, 3 annotations as well (default: an approval signature)")
	hash := fs.String("hash", "sha256", "digest algorithm: sha256, sha384 or sha512")
	pss := fs.Bool("pss", false, "sign with RSA-PSS instead of RSA PKCS#1 v1.5")
	out := fs.String("o", "", "output PDF (default: standard output)")
	_, data, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	if *keyPath == "" || *certPath == "" {
		return errors.New("sign: -key and -cert are required")
	}
	kb, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	key, err := parsePrivateKey(kb)
	if err != nil {
		return fmt.Errorf("-key: %w", err)
	}
	cb, err := os.ReadFile(*certPath)
	if err != nil {
		return err
	}
	chain, err := parseCertificates(cb)
	if err != nil {
		return fmt.Errorf("-cert: %w", err)
	}
	opts := &zerechnung.SignOptions{
		Signer: key, Chain: chain, PSS: *pss, Certify: zerechnung.Certification(*certify),
		Reason: *reason, Location: *location, ContactInfo: *contact, Name: *name, FieldName: *field,
	}
	switch strings.ToLower(*hash) {
	case "sha256":
		opts.Hash = crypto.SHA256
	case "sha384":
		opts.Hash = crypto.SHA384
	case "sha512":
		opts.Hash = crypto.SHA512
	default:
		return fmt.Errorf("unknown digest algorithm %q (sha256, sha384 or sha512)", *hash)
	}
	res, err := zerechnung.Sign(data, opts)
	if err != nil {
		return err
	}
	return writeOut(*out, res)
}

// parsePrivateKey reads the first private key of a PEM file: PKCS#8,
// PKCS#1 (RSA) or SEC 1 (EC), unencrypted.
func parsePrivateKey(data []byte) (crypto.Signer, error) {
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return nil, errors.New("no private key in the PEM file")
		}
		var key any
		var err error
		switch block.Type {
		case "PRIVATE KEY":
			key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		case "RSA PRIVATE KEY":
			if _, encrypted := block.Headers["DEK-Info"]; encrypted {
				return nil, errors.New("the key is encrypted: decrypt it first (openssl pkey -in key.pem -out plain.pem)")
			}
			key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		case "EC PRIVATE KEY":
			if _, encrypted := block.Headers["DEK-Info"]; encrypted {
				return nil, errors.New("the key is encrypted: decrypt it first (openssl pkey -in key.pem -out plain.pem)")
			}
			key, err = x509.ParseECPrivateKey(block.Bytes)
		case "ENCRYPTED PRIVATE KEY":
			return nil, errors.New("the key is encrypted: decrypt it first (openssl pkey -in key.pem -out plain.pem)")
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("unsupported key type %T", key)
		}
		return signer, nil
	}
}

// parseCertificates reads the certificates of a PEM file, in their order.
func parseCertificates(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, errors.New("no certificate in the PEM file")
	}
	return certs, nil
}

// pdfaCmd converts a PDF to PDF/A-3. When the conversion is refused, every
// reason is printed, one per line.
func pdfaCmd(args []string) error {
	fs := flag.NewFlagSet("pdfa", flag.ContinueOnError)
	out := fs.String("o", "", "output PDF (default: standard output)")
	_, pdf, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	res, err := zerechnung.ConvertPDFA(pdf, nil)
	if err != nil {
		return reasons(err)
	}
	return writeOut(*out, res)
}

// reasonsError lists the reasons of a ConversionError one per line.
type reasonsError struct{ *zerechnung.ConversionError }

func (e reasonsError) Error() string {
	return "the PDF cannot be converted to PDF/A-3:\n  " + strings.Join(e.Reasons, "\n  ")
}

func (e reasonsError) Unwrap() error { return e.ConversionError }

// reasons returns err, with the reasons one per line when it is a
// ConversionError.
func reasons(err error) error {
	var ce *zerechnung.ConversionError
	if errors.As(err, &ce) {
		return reasonsError{ce}
	}
	return err
}

func jsonCmd(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("json", flag.ContinueOnError)
	_, data, err := oneFile(fs, args)
	if err != nil {
		return err
	}
	return writeJSON(data, w)
}

// writeJSON writes to w the invoice of data, a PDF or an XML invoice, in
// JSON: the invoices of a FatturaPA or Facturae batch as an array.
func writeJSON(data []byte, w io.Writer) error {
	switch {
	case isPDF(data):
		att, err := zerechnung.Extract(data)
		if err != nil {
			return err
		}
		data = att.XML
	case !looksXML(data):
		// A FatturaPA file signed in CAdES: the document it signs, once
		// the signature verifies.
		doc, _, err := fatturapa.Unwrap(data)
		if err != nil {
			return err
		}
		data = doc
	}
	invs, err := parseInvoices(data)
	if len(invs) == 0 {
		return err
	}
	if err != nil {
		fmt.Fprintln(stderr, "zerechnung: warning:", err)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if len(invs) == 1 {
		return enc.Encode(invs[0])
	}
	return enc.Encode(invs)
}

// looksXML reports whether data starts like an XML document.
func looksXML(data []byte) bool {
	d := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(d) > 0 && d[0] == '<'
}

// parseInvoices reads a CII, UBL, FatturaPA or Facturae document, detected
// from the root element. A FatturaPA or Facturae document may hold several
// invoices.
func parseInvoices(data []byte) ([]*invoice.Invoice, error) {
	var inv *invoice.Invoice
	var err error
	if root, perr := xmltree.Parse(data); perr == nil && fatturapa.IsFatturaPA(root) {
		return fatturapa.ParseAll(data)
	} else if perr == nil && facturae.IsFacturae(root) {
		return facturae.ParseAll(data)
	} else if perr == nil && ubl.IsUBL(root) {
		inv, err = ubl.Parse(data)
	} else {
		inv, err = cii.Parse(data)
	}
	if inv == nil {
		return nil, err
	}
	return []*invoice.Invoice{inv}, err
}

func rulesCmd(args []string) error {
	fs := flag.NewFlagSet("rules", flag.ContinueOnError)
	fr := fs.Bool("fr", false, "include the French rules")
	de := fs.Bool("de", false, "include the German rules")
	cpro := fs.Bool("choruspro", false, "include the Chorus Pro checks")
	pep := fs.Bool("peppol", false, "include the Peppol BIS Billing 3.0 rules")
	nl := fs.Bool("nl", false, "include the Dutch rules of NLCIUS")
	ita := fs.Bool("it", false, "include the Italian rules of the CIUS-IT")
	por := fs.Bool("pt", false, "include the Portuguese rules of the CIUS-PT")
	rou := fs.Bool("ro", false, "include the Romanian rules of the CIUS-RO")
	svk := fs.Bool("sk", false, "include the Slovak requirements of the eFaktura")
	cro := fs.Bool("hr", false, "include the Croatian rules of the CIUS-HR")
	face := fs.Bool("face", false, "include the FACe checks of the signature of a Facturae document")
	if err := fs.Parse(args); err != nil {
		return err
	}
	set := validate.Default
	if *fr {
		set |= validate.France
	}
	if *de {
		set |= validate.Germany | validate.GermanyProfile
	}
	if *cpro {
		set |= validate.ChorusPro
	}
	if *pep {
		set |= validate.Peppol
	}
	if *nl {
		set |= validate.Netherlands
	}
	if *ita {
		set |= validate.Italy
	}
	if *por {
		set |= validate.Portugal
	}
	if *rou {
		set |= validate.Romania
	}
	if *svk {
		set |= validate.Slovakia
	}
	if *cro {
		set |= validate.Croatia
	}
	if *face {
		set |= validate.FACe
	}
	for _, r := range validate.Catalog(set) {
		fmt.Printf("%-28s %-8s %s\n", r.ID, r.Severity, strings.TrimSpace(r.Description))
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
