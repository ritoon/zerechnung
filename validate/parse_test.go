package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	ublxml "github.com/ritoon/zerechnung/ubl"
)

// parseDocuments returns the XML documents of the testdata of the module,
// and variants: a batch of FatturaPA and of Facturae invoices, a CII
// invoice with an amount that cannot be read, an XML that cannot be parsed
// and documents that are not invoices.
func parseDocuments(t *testing.T) map[string][]byte {
	docs := map[string][]byte{}
	for _, pattern := range []string{"../testdata/*.xml", "../facturae/testdata/*.xml", "../fatturapa/testdata/*.xml", "../internal/rules/*/testdata/*.xml"} {
		names, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			docs[name] = data
		}
	}
	if len(docs) < 30 {
		t.Fatalf("%d documents in the testdata", len(docs))
	}
	// twice repeats the element open … close of a document.
	twice := func(name, open, close string) []byte {
		data := docs[name]
		i, j := bytes.Index(data, []byte(open)), bytes.Index(data, []byte(close))
		if i < 0 || j < 0 {
			t.Fatalf("%s has no %s … %s", name, open, close)
		}
		j += len(close)
		return slices.Concat(data[:j], data[i:j], data[j:])
	}
	docs["lotto"] = twice("../fatturapa/testdata/fpa12.xml", "<FatturaElettronicaBody>", "</FatturaElettronicaBody>")
	docs["batch"] = twice("../facturae/testdata/b2g.xml", "<Invoice>", "</Invoice>")
	docs["unreadable amount"] = bytes.Replace(docs["../testdata/EN16931_Einfach.cii.xml"],
		[]byte("<ram:DuePayableAmount>"), []byte("<ram:DuePayableAmount>x"), 1)
	docs["not XML"] = []byte("<rsm:CrossIndustryInvoice")
	docs["unknown root"] = []byte(`<Order xmlns="urn:oasis:names:specification:ubl:schema:xsd:Order-2"/>`)
	docs["UBL root in another namespace"] = []byte(`<Invoice xmlns="urn:example"/>`)
	return docs
}

// parseSeparately reads data with the Parse function of its syntax, as a
// caller that does not use Parse does. ok is false when data cannot be
// parsed or is not an invoice of these syntaxes.
func parseSeparately(data []byte) (inv *invoice.Invoice, err error, ok bool) {
	root, err := xmltree.Parse(data)
	switch {
	case err != nil:
		return nil, err, false
	case root.Name == "CrossIndustryInvoice" && root.Space == cii.NamespaceRSM || cii.IsZUGFeRD1(root):
		inv, err = cii.Parse(data)
	case ublxml.IsUBL(root):
		inv, err = ublxml.Parse(data)
	case fatturapa.IsFatturaPA(root):
		inv, err = fatturapa.Parse(data)
	case facturae.IsFacturae(root):
		inv, err = facturae.Parse(data)
	default:
		return nil, nil, false
	}
	return inv, err, true
}

// TestParse checks that Parse gives the invoice and the error of the Parse
// function of the syntax of each document, and the report of XML.
func TestParse(t *testing.T) {
	for name, data := range parseDocuments(t) {
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Default | France | Germany | Peppol | FACe}} {
			t.Run(fmt.Sprintf("%s/%+v", name, opts), func(t *testing.T) {
				inv, r, err := Parse(data, opts)
				if want := XML(data, opts); !reflect.DeepEqual(r, want) {
					t.Errorf("report:\n%v\nXML reports:\n%v", r.Findings, want.Findings)
				}
				wantInv, wantErr, ok := parseSeparately(data)
				switch {
				case !ok:
					if inv != nil || err == nil {
						t.Errorf("invoice %v, error %v: want no invoice and an error", inv != nil, err)
					}
				case !reflect.DeepEqual(inv, wantInv):
					t.Errorf("invoice %+v, want %+v", inv, wantInv)
				case fmt.Sprint(err) != fmt.Sprint(wantErr) || reflect.TypeOf(err) != reflect.TypeOf(wantErr):
					t.Errorf("error %v (%T), want %v (%T)", err, err, wantErr, wantErr)
				}
			})
		}
	}
}

// TestParseVariants checks that the variants of parseDocuments test what
// they are meant to.
func TestParseVariants(t *testing.T) {
	docs := parseDocuments(t)
	for name, want := range map[string]any{
		"lotto":             new(fatturapa.IssuesError),
		"batch":             new(facturae.IssuesError),
		"unreadable amount": new(cii.IssuesError),
	} {
		inv, _, err := Parse(docs[name], nil)
		if inv == nil || reflect.TypeOf(err) != reflect.TypeOf(want) {
			t.Errorf("%s: invoice %v, error %v (%T), want an invoice and a %T", name, inv != nil, err, err, want)
		}
	}
	if inv, r, err := Parse(docs["not XML"], nil); inv != nil || err == nil || r.Valid() {
		t.Errorf("not XML: invoice %v, error %v, report valid %v", inv, err, r.Valid())
	}
	for _, name := range []string{"unknown root", "UBL root in another namespace"} {
		if inv, r, err := Parse(docs[name], nil); inv != nil || !errors.Is(err, ErrNotInvoice) || r.Valid() {
			t.Errorf("%s: invoice %v, error %v, report valid %v", name, inv, err, r.Valid())
		}
	}
}

// BenchmarkReadAndValidate measures cii.Parse followed by XML, and Parse,
// which parses the XML once.
func BenchmarkReadAndValidate(b *testing.B) {
	data, err := os.ReadFile("../testdata/EN16931_Einfach.cii.xml")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("separately", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		for b.Loop() {
			if _, err := cii.Parse(data); err != nil {
				b.Fatal(err)
			}
			XML(data, nil)
		}
	})
	b.Run("Parse", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := Parse(data, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
}
