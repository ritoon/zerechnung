package validate

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	ublxml "github.com/ritoon/zerechnung/ubl"
)

// ErrNotInvoice is returned by Parse for an XML document whose root element
// is not that of an invoice of a syntax it reads.
var ErrNotInvoice = errors.New("validate: not a CII, UBL, FatturaPA or Facturae invoice")

// Parse reads and validates an invoice, parsing the XML once instead of
// twice for a Parse function followed by XML. The report is the one XML
// returns. The invoice and the error are those of the Parse function of
// the syntax of the document: cii.Parse for a CII invoice (ZUGFeRD 1.0
// included), ubl.Parse, fatturapa.Parse or facturae.Parse; as with them,
// an invoice with values that cannot be read comes with an *IssuesError of
// its package, which lists them. An XML that cannot be parsed, or whose
// root element is none of these, gives no invoice, an error (ErrNotInvoice
// for the latter) and the report that says why.
func Parse(data []byte, opts *Options) (*invoice.Invoice, *Report, error) {
	root, err := xmltree.Parse(data)
	r := xmlReport(data, root, err, opts)
	if err != nil {
		return nil, r, fmt.Errorf("validate: %w", err)
	}
	inv, err := readTree(root)
	return inv, r, err
}

// readTree reads the invoice of root as the Parse function of its syntax
// does.
func readTree(root *xmltree.Node) (*invoice.Invoice, error) {
	switch {
	case root.Name == "CrossIndustryInvoice" && root.Space == cii.NamespaceRSM, cii.IsZUGFeRD1(root):
		inv, issues := cii.ParseTree(root)
		if len(issues) > 0 {
			return inv, &cii.IssuesError{Issues: issues}
		}
		return inv, nil
	case ublxml.IsUBL(root):
		inv, issues := ublxml.ParseTree(root)
		if len(issues) > 0 {
			return inv, &ublxml.IssuesError{Issues: issues}
		}
		return inv, nil
	case fatturapa.IsFatturaPA(root):
		// As fatturapa.Parse: the first invoice of a batch (lotto).
		invs, issues := fatturapa.ParseTree(root)
		if len(invs) > 1 {
			issues = append(issues, fatturapa.Issue{Message: "the document holds " + strconv.Itoa(len(invs)) +
				" invoices (lotto); only the first is read, ParseAll reads them all"})
		}
		var err error
		if len(issues) > 0 {
			err = &fatturapa.IssuesError{Issues: issues}
		}
		if len(invs) == 0 {
			return nil, err
		}
		return invs[0], err
	case facturae.IsFacturae(root):
		// As facturae.Parse: the first invoice of a batch.
		invs, issues := facturae.ParseTree(root)
		if len(invs) > 1 {
			issues = append(issues, facturae.Issue{Message: "the document holds " + strconv.Itoa(len(invs)) +
				" invoices (batch); only the first is read, ParseAll reads them all"})
		}
		var err error
		if len(issues) > 0 {
			err = &facturae.IssuesError{Issues: issues}
		}
		if len(invs) == 0 {
			return nil, err
		}
		return invs[0], err
	}
	return nil, fmt.Errorf("%w: root element <%s> in namespace %q", ErrNotInvoice, root.Name, root.Space)
}
