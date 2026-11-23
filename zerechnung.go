// Package zerechnung reads, validates and creates Factur-X, ZUGFeRD and
// XRechnung hybrid invoices: a PDF that carries the invoice as CII XML.
//
// The package does not draw invoices. It takes a PDF produced by your own
// rendering engine and adds what the standard requires (the XML attachment,
// its file specification, the XMP metadata), with an incremental update that
// leaves the original bytes untouched. It reads and checks any hybrid
// invoice, whatever its profile.
//
// The main entry points are:
//
//   - Extract and Read get the XML, or the parsed invoice, out of a PDF,
//     ZUGFeRD 1.0 PDFs included.
//   - Embed adds an XML invoice to a PDF; Create does the same from an
//     invoice.Invoice.
//   - Check validates a hybrid PDF: container, XML schema, EN 16931 rules,
//     profile rules, signatures and, on request, French and German rules.
//   - Sign signs a PDF in PAdES, as a signature or a seal; AddValidationData
//     and AddDocumentTimestamp take the signature to B-LT and B-LTA, and
//     VerifySignatures checks the signatures of a PDF.
//
// Sub-packages give direct access to each layer: invoice (the EN 16931
// model), cii (CII XML), validate (validation of XML invoices).
//
// The functions of this package and of packages cii, ubl, facturae,
// fatturapa and validate can be called from several goroutines at once, on
// the same arguments: they only read the byte slices and the invoices they
// are given. The exception is Create with CreateOptions.Calculate, which
// fills in the amounts of the invoice: that invoice must not be used by
// another goroutine meanwhile.
package zerechnung

import (
	"errors"
	"fmt"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/rules"
	ublrules "github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// Read extracts the invoice XML from a hybrid PDF and parses it. The XML of
// a ZUGFeRD 1.0 PDF (rsm:CrossIndustryDocument) is read too, see cii.Parse.
func Read(pdf []byte) (*invoice.Invoice, *Attachment, error) {
	att, err := Extract(pdf)
	if err != nil {
		return nil, nil, err
	}
	inv, err := cii.Parse(att.XML)
	if err != nil {
		return inv, att, err
	}
	return inv, att, nil
}

// ReadAndCheck reads and checks a hybrid PDF, reading the PDF and parsing
// its XML once instead of twice for Read followed by Check. The report and
// the attachment are those of Check, the invoice and the error those of
// Read: a PDF without an invoice (ErrNoInvoice), an encrypted PDF
// (ErrEncrypted) or an XML that cannot be read comes with an error and the
// report that says why. Only a PDF that cannot be read at all gives no
// report, with the error of Check.
func ReadAndCheck(pdf []byte, opts *validate.Options) (*invoice.Invoice, *validate.Report, *Attachment, error) {
	return readAndCheck(pdf, opts, nil)
}

// readAndCheck implements ReadAndCheck, with the font cache of a Checker
// when fonts is not nil.
func readAndCheck(pdf []byte, opts *validate.Options, fonts *fontCache) (*invoice.Invoice, *validate.Report, *Attachment, error) {
	c, err := checkTree(pdf, opts, fonts)
	if err != nil {
		return nil, nil, nil, err
	}
	if c.att == nil {
		return nil, c.report, nil, c.noInvoice
	}
	inv, err := readTree(c.root, c.att.XML)
	return inv, c.report, c.att, err
}

// readTree reads the invoice of an attachment as Read does, from root, the
// XML parsed, when it is a CII invoice.
func readTree(root *xmltree.Node, xml []byte) (*invoice.Invoice, error) {
	if root == nil || !(root.Name == "CrossIndustryInvoice" && root.Space == cii.NamespaceRSM || cii.IsZUGFeRD1(root)) {
		// The XML cannot be parsed, or is not a CII invoice: cii.Parse
		// says why, as for Read.
		return cii.Parse(xml)
	}
	inv, issues := cii.ParseTree(root)
	if len(issues) > 0 {
		return inv, &cii.IssuesError{Issues: issues}
	}
	return inv, nil
}

// CreateOptions tune Create.
type CreateOptions struct {
	// Profile is the profile to write; zero means inv.Profile, or EN 16931
	// when inv.Profile is not set.
	Profile invoice.Profile
	// Calculate runs inv.Calculate before writing the XML, so that Create
	// modifies inv.
	Calculate bool
	// Validate, when not nil, validates the XML before embedding it and makes
	// Create fail with a *ValidationError when the report has errors.
	Validate *validate.Options
	// Embed tunes the PDF part. Its Profile field is ignored. Set
	// Embed.RequirePDFA to refuse a pdf that does not declare PDF/A-2,
	// PDF/A-3 or PDF/A-4.
	Embed EmbedOptions
}

// ValidationError is returned by Create when the invoice is not valid.
type ValidationError struct {
	Report *validate.Report
}

// Error gives the number of errors of the report and the first one.
func (e *ValidationError) Error() string {
	errs := e.Report.Errors()
	if len(errs) == 0 {
		return "zerechnung: invalid invoice"
	}
	return fmt.Sprintf("zerechnung: invalid invoice: %d error(s), first: %s", len(errs), errs[0])
}

// Create writes inv as CII XML for its profile and embeds it in pdf. With
// opts.Embed.RequirePDFA, it fails with an error wrapping ErrNotPDFA when
// pdf does not declare PDF/A-2, PDF/A-3 or PDF/A-4.
func Create(pdf []byte, inv *invoice.Invoice, opts *CreateOptions) ([]byte, error) {
	if opts == nil {
		opts = &CreateOptions{}
	}
	p := opts.Profile
	if p == invoice.ProfileUnknown {
		p = inv.Profile
	}
	if p == invoice.ProfileUnknown {
		p = invoice.ProfileEN16931
	}
	if opts.Calculate {
		inv.Calculate()
	}
	xml, err := cii.Marshal(inv, &cii.MarshalOptions{Profile: p})
	if err != nil {
		return nil, err
	}
	if opts.Validate != nil {
		vo := *opts.Validate
		vo.Profile = p
		vo.Hybrid = true // the XML goes into a Factur-X PDF
		if r := validate.XML(xml, &vo); !r.Valid() {
			return nil, &ValidationError{Report: r}
		}
	}
	eo := opts.Embed
	eo.Profile = p
	return Embed(pdf, xml, &eo)
}

// Check validates a hybrid PDF: the PDF container (PDF/A-3, attachment, XMP)
// and the embedded XML. The returned attachment is nil when no invoice was
// found. The error is reserved for PDFs that cannot be read at all.
//
// The national rule sets that apply to the XML (France, Germany,
// GermanyProfile, selected explicitly or through Auto, and Germany for an
// XRECHNUNG invoice, see validate.RulesFor) also select the national usage
// of the container: with the German rules, the /AFRelationship of a
// BASIC invoice or above must be Alternative (PDF-40); with the French
// rules, Data is admitted besides Alternative and Source (no PDF-14).
//
// The XML is validated with opts.Hybrid set: an EN 16931 invoice in a
// Factur-X PDF is checked against the Factur-X schematron of the EN 16931
// profile, like BASIC and EXTENDED invoices (see validate.Options.Hybrid).
//
// The signatures of a signed PDF are checked by VerifySignatures, with
// opts.Trust: the findings of SignatureCatalog follow those of the
// container.
//
// opts.PDFA selects the PDF/A checks of the container. The default,
// validate.PDFAFull, reads the content streams of the pages and the font
// programs they use, which takes most of the time and memory of a check;
// validate.PDFABasic skips them (see validate.PDFALevel). A Checker keeps
// the font programs it reads from one check to the next.
func Check(pdf []byte, opts *validate.Options) (*validate.Report, *Attachment, error) {
	return check(pdf, opts, nil)
}

// check implements Check, with the font cache of a Checker when fonts is
// not nil.
func check(pdf []byte, opts *validate.Options, fonts *fontCache) (*validate.Report, *Attachment, error) {
	c, err := checkTree(pdf, opts, fonts)
	return c.report, c.att, err
}

// checked is what checkTree finds: the report and the attachment of Check,
// and what ReadAndCheck reads besides.
type checked struct {
	report    *validate.Report
	att       *Attachment
	root      *xmltree.Node // the invoice XML, nil when it cannot be parsed
	noInvoice error         // why the PDF has no invoice attachment, as Extract returns it
}

// checkTree implements Check and ReadAndCheck.
func checkTree(pdf []byte, opts *validate.Options, fonts *fontCache) (checked, error) {
	o := validate.Options{}
	if opts != nil {
		o = *opts
	}
	o.Hybrid = true
	opts = &o
	// The XML is parsed once: to find the national rules that apply to the
	// container, then to validate it.
	var root *xmltree.Node
	k := &containerCheck{level: opts.PDFA, fonts: fonts}
	att, err := k.check(pdf, func(xml []byte) countries {
		t, err := xmltree.Parse(xml)
		if err != nil {
			return 0
		}
		root = t
		if t.Name != "CrossIndustryInvoice" || t.Space != rules.NSRsm {
			return 0
		}
		return countriesOf(validate.RulesFor(rules.NewDoc(t, opts.Profile), opts))
	})
	findings := k.findings
	if err != nil && att == nil && len(findings) == 0 {
		return checked{}, err
	}
	r := &validate.Report{}
	switch {
	case att == nil || len(att.XML) == 0:
	case root != nil:
		r = validate.Tree(root, opts)
		// A Factur-X or ZUGFeRD PDF embeds a CII invoice: a UBL invoice is
		// validated as such, but the PDF is not a valid hybrid invoice.
		if ublrules.IsUBL(root) {
			rules.Trace("CII")
			findings = append(findings, validate.Finding{Rule: "CII", Severity: validate.Error, Source: validate.SourceSyntax,
				Path: "/" + root.Name, Line: root.Line,
				Message: "the embedded XML is a UBL invoice: a Factur-X or ZUGFeRD PDF embeds a CII invoice (rsm:CrossIndustryInvoice)"})
		}
	default:
		r = validate.XML(att.XML, opts) // reports why the XML cannot be parsed
	}
	// The signatures of the PDF, when it has some.
	if sr, sigs, err := VerifySignatures(pdf, &opts.Trust); err == nil && len(sigs) > 0 {
		findings = append(findings, sr.Findings...)
	}
	r.Findings = append(findings, r.Findings...)
	return checked{report: r, att: att, root: root, noInvoice: k.noInvoice}, nil
}

// DetectProfile returns the profile declared by the BT-24 of a CII invoice,
// or for a ZUGFeRD 1.0 invoice, the profile matched to its specification
// identifier (see invoice.ProfileFromZUGFeRD1URN).
func DetectProfile(xml []byte) (invoice.Profile, error) {
	root, err := xmltree.Parse(xml)
	if err != nil {
		return invoice.ProfileUnknown, err
	}
	zf1 := cii.IsZUGFeRD1(root)
	context := "ExchangedDocumentContext"
	if zf1 {
		context = "SpecifiedExchangedDocumentContext"
	}
	id := root.Value(context + "/GuidelineSpecifiedDocumentContextParameter/ID")
	if id == "" {
		return invoice.ProfileUnknown, errors.New("zerechnung: no specification identifier (BT-24)")
	}
	if zf1 {
		return invoice.ProfileFromZUGFeRD1URN(id), nil
	}
	return invoice.ProfileFromURN(id), nil
}
