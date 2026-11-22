// Command parity compares the rules that this package raises with the ones
// raised by the reference validators, invoice by invoice: the KoSIT validator
// with the XRechnung configuration, and Mustang; and, for NLCIUS, the XSLT of
// the schematrons of the Dutch Peppol Authority (NPa). See ../README.md.
//
//	parity stage -corpus DIR -kosit DIR -work DIR [-variants DIR] [-nl DIR -nl-tests DIR -repo DIR -nl-variants DIR]
//	parity compare -work DIR [-kosit-config DIR] [-known known.txt] [-md report.md] [-json report.json]
//
// stage copies the invoices of the ZUGFeRD corpus and of the KoSIT test suite
// into DIR/in under short unique names (0001.xml, 0002.pdf…), lists them in
// DIR/index.tsv with their origin, and writes the lists of files to give to
// each validator (DIR/kosit.list: the XML files, DIR/mustang.list: all).
// With -variants, it adds the documents of the mutation cases of the EN 16931
// and XRechnung rules (go test -variants, one directory per rule package),
// which only the KoSIT validator receives, and writes the rules that each
// case expects to DIR/expected.tsv. With -nl, it adds the NLCIUS invoices: the SimplerInvoicing test files
// (-nl-tests), the NLCIUS invoices of the repository (-repo) and the
// variants of the mutation tests (-nl-variants), which only the NPa XSLT
// receive, each with the stylesheet a validator picks for it
// (DIR/npa.list).
//
// compare reads the reports that the validators wrote in DIR/kosit
// (0001-report.xml), DIR/mustang (0001.xml.xml) and DIR/npa (0001.xml.svrl),
// and the table at the end of DIR/kosit.log, which alone tells the documents
// that KoSIT could not assess; it validates each invoice with this package, and compares the rules raised as
// errors, and also as warnings for the NPa XSLT and the variants. Each
// variant must raise, at the reference, the rules that its mutation case
// expects. It lists the assertions of the NLCIUS patterns that no invoice
// makes fail and, with -kosit-config (the unzipped configuration of the
// KoSIT validator), counts the assertions of the KoSIT schematrons that at
// least one invoice raises. A difference, a rule expected and not raised, or
// an NLCIUS assertion that never fails, that known.txt does not justify
// makes the command exit with status 1.
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
		var nl nlInputs
		fs.StringVar(&nl.npa, "nl", "", "clone of peppolautoriteit-nl/validation (FX_NL)")
		fs.StringVar(&nl.tests, "nl-tests", "", "SimplerInvoicing test files of phive-rules (FX_NL_TESTS)")
		fs.StringVar(&nl.repo, "repo", "", "root of this repository")
		fs.StringVar(&nl.variants, "nl-variants", "", "documents of the mutation cases of internal/rules/nl")
		variants := fs.String("variants", "", "documents of the mutation cases of the EN 16931 and XRechnung rules, one directory per package")
		fs.Parse(os.Args[2:])
		if *corpus == "" || *kosit == "" || *work == "" ||
			nl.npa != "" && (nl.tests == "" || nl.repo == "" || nl.variants == "") {
			usage()
		}
		if nl.npa != "" {
			if nl.npa, err = filepath.Abs(nl.npa); err != nil {
				break
			}
		}
		err = stage(*corpus, *kosit, *work, nl, *variants)
	case "compare":
		fs := flag.NewFlagSet("compare", flag.ExitOnError)
		work := fs.String("work", "", "work directory")
		kositConfig := fs.String("kosit-config", "", "unzipped configuration of the KoSIT validator")
		known := fs.String("known", "", "file of the known differences")
		md := fs.String("md", "", "Markdown report")
		js := fs.String("json", "", "JSON report")
		fs.Parse(os.Args[2:])
		if *work == "" {
			usage()
		}
		var ok bool
		ok, err = compare(*work, *kositConfig, *known, *md, *js)
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
  parity stage -corpus DIR -kosit DIR -work DIR [-variants DIR] [-nl DIR -nl-tests DIR -repo DIR -nl-variants DIR]
  parity compare -work DIR [-kosit-config DIR] [-known known.txt] [-md report.md] [-json report.json]`)
	os.Exit(2)
}
