// Command zerechnung reads, validates and creates Factur-X / ZUGFeRD /
// XRechnung invoices. validate and json also accept a CII or UBL 2.1 XML
// invoice, and json a FatturaPA document, whose syntax is detected from the
// root element.
//
//	zerechnung extract [-o out.xml] invoice.pdf
//	zerechnung info invoice.pdf
//	zerechnung validate [-fr] [-de] [-peppol] [-nl] [-it] [-auto] [-reject-basicwl] [-choruspro [-service-required] [-engagement-required]] [-profile p] [-hybrid] [-json] invoice.pdf|invoice.xml
//	zerechnung embed -xml invoice.xml [-profile p] [-attach file]... [-require-pdfa] [-o out.pdf] input.pdf
//	zerechnung json invoice.pdf|invoice.xml
//	zerechnung rules [-fr] [-de] [-choruspro] [-peppol] [-nl] [-it]
//
// validate exits with status 1 when the invoice has errors.
package main

import (
	"bytes"
	"encoding/json"
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
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

const usage = `zerechnung reads, validates and creates Factur-X / ZUGFeRD / XRechnung invoices.
validate and json also accept an XML invoice in CII or UBL 2.1 syntax,
json also a FatturaPA document (an array when it holds several invoices).

Usage:
  zerechnung extract [-o out.xml] invoice.pdf
  zerechnung info invoice.pdf
  zerechnung validate [-fr] [-de] [-peppol] [-nl] [-it] [-auto] [-reject-basicwl] [-choruspro [-service-required] [-engagement-required]] [-profile p] [-hybrid] [-json] invoice.pdf|invoice.xml
  zerechnung embed -xml invoice.xml [-profile p] [-attach file]... [-require-pdfa] [-convert-pdfa] [-o out.pdf] input.pdf
  zerechnung pdfa [-o out.pdf] input.pdf
  zerechnung json invoice.pdf|invoice.xml
  zerechnung rules [-fr] [-de] [-choruspro] [-peppol] [-nl] [-it]

Profiles: minimum, basicwl, basic, en16931, extended, xrechnung, extended-ctc-fr, peppol, nlcius, cius-it.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
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
		fmt.Fprintf(os.Stderr, "zerechnung: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "zerechnung:", err)
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
	profile := fs.String("profile", "", "validate against this profile instead of BT-24")
	hybrid := fs.Bool("hybrid", false, "the XML is the invoice of a Factur-X PDF: check an EN 16931 invoice against the Factur-X schematron (always the case for a PDF)")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	_, data, err := oneFile(fs, args)
	if err != nil {
		return 0, err
	}
	opts := &validate.Options{Auto: *auto, RejectBasicWL: *rejectBasicWL, Hybrid: *hybrid}
	if *profile != "" {
		if opts.Profile = invoice.ParseProfile(*profile); opts.Profile == invoice.ProfileUnknown {
			return 0, fmt.Errorf("unknown profile %q", *profile)
		}
	}
	if *fr || *de || *cpro || *pep || *nl || *ita {
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
	}
	opts.PublicBuyer = validate.PublicBuyer{ServiceCodeRequired: *serviceRequired, EngagementRequired: *engagementRequired}
	var r *validate.Report
	if isPDF(data) {
		r, _, err = zerechnung.Check(data, opts)
		if err != nil {
			return 0, err
		}
	} else {
		r = validate.XML(data, opts)
	}
	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		out := struct {
			Profile  string             `json:"profile"`
			Valid    bool               `json:"valid"`
			Findings []validate.Finding `json:"findings"`
		}{r.Profile.String(), r.Valid(), r.Findings}
		if out.Findings == nil {
			out.Findings = []validate.Finding{}
		}
		if err := enc.Encode(out); err != nil {
			return 0, err
		}
	} else {
		fmt.Fprintf(w, "Profile: %s\n", r.Profile)
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
	return writeOut(*out, res)
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
	if isPDF(data) {
		att, err := zerechnung.Extract(data)
		if err != nil {
			return err
		}
		data = att.XML
	}
	invs, err := parseInvoices(data)
	if len(invs) == 0 {
		return err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "zerechnung: warning:", err)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if len(invs) == 1 {
		return enc.Encode(invs[0])
	}
	return enc.Encode(invs)
}

// parseInvoices reads a CII, UBL or FatturaPA document, detected from the
// root element. A FatturaPA document may hold several invoices.
func parseInvoices(data []byte) ([]*invoice.Invoice, error) {
	var inv *invoice.Invoice
	var err error
	if root, perr := xmltree.Parse(data); perr == nil && fatturapa.IsFatturaPA(root) {
		return fatturapa.ParseAll(data)
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
