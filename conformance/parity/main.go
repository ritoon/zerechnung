// Command parity compares the rules that this package raises with the ones
// raised by the reference validators, invoice by invoice: the KoSIT validator
// with the XRechnung configuration, and Mustang; for NLCIUS, the XSLT of the
// schematrons of the Dutch Peppol Authority (NPa); for the CIUS-PT, the
// schematron of eSPap compiled by SchXslt. See ../README.md.
//
//	parity stage -corpus DIR -kosit DIR -work DIR [-repo DIR] [-nl DIR -nl-tests DIR -nl-variants DIR] [-pt ZIP -pt-variants DIR]
//	parity compare -work DIR [-known known.txt] [-md report.md] [-json report.json]
//
// stage copies the invoices of the ZUGFeRD corpus and of the KoSIT test suite
// into DIR/in under short unique names (0001.xml, 0002.pdf…), lists them in
// DIR/index.tsv with their origin, and writes the lists of files to give to
// each validator (DIR/kosit.list: the XML files, DIR/mustang.list: all).
// With -nl, it adds the NLCIUS invoices: the SimplerInvoicing test files
// (-nl-tests), the NLCIUS invoices of the repository (-repo) and the
// variants of the mutation tests (-nl-variants), which only the NPa XSLT
// receive, each with the stylesheet a validator picks for it
// (DIR/npa.list). With -pt, it adds the CIUS-PT invoices: the examples of
// eSPap (the XML files of the archive -pt), the CIUS-PT invoices of the
// repository and the variants of the mutation tests (-pt-variants), which
// only the CIUS-PT schematron receives (DIR/pt.list).
//
// compare reads the reports that the validators wrote in DIR/kosit
// (0001-report.xml), DIR/mustang (0001.xml.xml), DIR/npa and DIR/pt
// (0001.xml.svrl), validates each invoice with this package, and compares
// the rules raised as errors, and also as warnings for the NPa XSLT and the
// CIUS-PT schematron, on the same element for the latter (whose XSLT,
// compiled by SchXslt, it reads from DIR/cius-pt.xsl). It lists the
// assertions of the NLCIUS patterns that no invoice makes fail. A
// difference, or an assertion that never fails, that known.txt does not
// justify makes the command exit with status 1.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "stage":
		fs := flag.NewFlagSet("stage", flag.ExitOnError)
		corpus := fs.String("corpus", "", "clone of ZUGFeRD/corpus")
		kosit := fs.String("kosit", "", "clone of itplr-kosit/xrechnung-testsuite")
		work := fs.String("work", "", "work directory")
		repo := fs.String("repo", "", "root of this repository")
		var (
			nl nlInputs
			pt ptInputs
		)
		fs.StringVar(&nl.npa, "nl", "", "clone of peppolautoriteit-nl/validation (FX_NL)")
		fs.StringVar(&nl.tests, "nl-tests", "", "SimplerInvoicing test files of phive-rules (FX_NL_TESTS)")
		fs.StringVar(&nl.variants, "nl-variants", "", "documents of the mutation cases of internal/rules/nl")
		fs.StringVar(&pt.examples, "pt", "", "archive of the CIUS-PT examples of eSPap (FX_PT)")
		fs.StringVar(&pt.variants, "pt-variants", "", "documents of the mutation cases of internal/rules/pt")
		fs.Parse(os.Args[2:])
		nl.repo, pt.repo = *repo, *repo
		if *corpus == "" || *kosit == "" || *work == "" ||
			nl.npa != "" && (nl.tests == "" || *repo == "" || nl.variants == "") ||
			pt.examples != "" && (*repo == "" || pt.variants == "") {
			usage()
		}
		if nl.npa != "" {
			if nl.npa, err = filepath.Abs(nl.npa); err != nil {
				break
			}
		}
		err = stage(*corpus, *kosit, *work, nl, pt)
	case "compare":
		fs := flag.NewFlagSet("compare", flag.ExitOnError)
		work := fs.String("work", "", "work directory")
		known := fs.String("known", "", "file of the known differences")
		md := fs.String("md", "", "Markdown report")
		js := fs.String("json", "", "JSON report")
		fs.Parse(os.Args[2:])
		if *work == "" {
			usage()
		}
		var ok bool
		ok, err = compare(*work, *known, *md, *js)
		if err == nil && !ok {
			os.Exit(1)
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "parity:", err)
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  parity stage -corpus DIR -kosit DIR -work DIR [-repo DIR] [-nl DIR -nl-tests DIR -nl-variants DIR] [-pt ZIP -pt-variants DIR]
  parity compare -work DIR [-known known.txt] [-md report.md] [-json report.json]`)
	os.Exit(2)
}
