// Command gen compiles XML schemas into the compact Go tables used by package
// schema: the schemas of the Factur-X profiles (CII), the complete UN/CEFACT
// CII schemas used by the reference validators of XRechnung (D16B) and of
// the French EXTENDED-CTC-FR profile (D22B), the OASIS UBL 2.1 schemas of
// the Invoice and CreditNote documents, the FatturaPA 1.2 schema of the
// Agenzia delle Entrate and the Facturae 3.2, 3.2.1 and 3.2.2 schemas of
// facturae.gob.es.
//
// Usage:
//
//	go run ./gen [-o tables_gen.go] <dir>
//	go run ./gen -cii D16B|D22B [-src path] [-o cii_d16b_tables_gen.go]
//	go run ./gen -ubl [-zip UBL-2.1.zip] [-hr "UBL2.1 eRačun.zip"] [-o ubl_tables_gen.go]
//	go run ./gen -fatturapa [-src Schema_VFPA12_v1.2.4.xsd] [-o fatturapa_tables_gen.go]
//	go run ./gen -facturae [-src dir] [-o facturae_tables_gen.go]
//
// In the first form, dir is searched recursively for the main schema of each
// Factur-X profile: a file whose name ends in _MINIMUM.xsd, _BASICWL.xsd (or
// _BASIC-WL.xsd), _BASIC.xsd, _EN16931.xsd or _EXTENDED.xsd, such as
// "Factur-X_1.09.2_EN16931.xsd" in the official Factur-X package. The schemas
// it imports are read from the paths given by their schemaLocation. Complex
// types that are structurally identical in several profiles are emitted once
// and shared.
//
// With -cii, gen compiles one complete UN/CEFACT Cross Industry Invoice
// schema (uncoupled: without the code list schemas), as a single schema set
// like the UBL one below:
//
//   - D16B: the schema of the KoSIT validator configuration for XRechnung
//     3.0.2 (resources/cii/16b/xsd), against which KoSIT validates XRechnung
//     CII invoices;
//   - D22B: the schema of the FNFE-MPE France_RFE package 1.4.0.04
//     (FNFE_RFE_INVOICE/CII/1xsd-CII_D22B_uncoupled), against which the
//     package validates CII invoices of the EN 16931 and EXTENDED-CTC-FR
//     profiles.
//
// The schema files are downloaded from their publisher (the KoSIT release
// archive, or the files of the France_RFE repository at the commit of the
// release tag) or read from -src: the KoSIT archive or a directory holding
// the files. Every file read must have the SHA-256 recorded in ciiSets.
//
// With -ubl, gen reads the OASIS UBL 2.1 package UBL-2.1.zip, from a local
// file (-zip) or downloaded from docs.oasis-open.org, checks its SHA-256 and
// compiles the runtime schemas (xsdrt, the schemas without annotations) of
// the two documents an EN 16931 invoice may use: xsdrt/maindoc/UBL-Invoice-2.1.xsd
// and xsdrt/maindoc/UBL-CreditNote-2.1.xsd, with the common schemas they
// import. The two documents share one schema set:
//
//   - named complex types are compiled once, which allows the recursive types
//     of UBL (a cac:InvoiceLine holds cac:SubInvoiceLine elements of the same
//     type);
//   - a simple content extension that adds nothing to the complex type it
//     extends is that type: the cbc types are the udt types they extend;
//   - an imported schema is read only when one of its components is
//     referenced;
//   - the wildcard of ext:ExtensionContent (xsd:any namespace="##other"
//     processContents="lax") is compiled as processContents="skip": its
//     element must be in a namespace other than the extension one, but its
//     content is not checked. The signature schemas that
//     UBL-ExtensionContentDataType-2.1.xsd imports for lax validation
//     (xmldsig, XAdES) are therefore never read.
//
// The schemas of the UBL extensions that package schema validates where the
// wildcard admits them (lax validation) are compiled into the same set, as
// extension roots: the schema of the Croatian extension HRFISK20Data
// (CIUS-HR and EXT-HR 2025), HRExtensionAggregateComponents-1.xsd of the
// package "UBL2.1 eRačun" of the Croatian tax administration, read from a
// local file (-hr) or downloaded from porezna.gov.hr, with its SHA-256 and
// that of the schema checked. It refers to the cbc elements and udt types of
// UBL, which are the OASIS ones: the other files of the package are the
// OASIS UBL 2.1 schemas, with their annotations, but for
// ExtensionContentType, a choice of HRFISK20Data and the UBL signature
// instead of the wildcard.
//
// With -fatturapa, gen reads the FatturaPA schema 1.2.4, from a local file
// (-src) or downloaded from www.fatturapa.gov.it, checks its SHA-256 and
// compiles it. The schema accepts both FPA12 and FPR12 documents (the files
// published for each are identical). Three of its constructs need more than
// the UN/CEFACT and UBL schemas:
//
//   - the choice between Denominazione, and Nome and Cognome, is a choice of
//     sequences inside a sequence: its elements are flattened into the
//     sequence, optional, and the alternatives are recorded so that package
//     schema checks that exactly one of them is present, complete;
//   - its patterns use the XML Schema block escapes \p{IsBasicLatin} and
//     \p{IsLatin-1Supplement}, which package regexp does not know: they are
//     rewritten as the character ranges of the blocks;
//   - the reference to ds:Signature, of the XML signature schema the
//     FatturaPA schema imports from www.w3.org, is compiled as an element
//     whose attributes and content are not checked: the signature schema is
//     never read.
//
// With -facturae, gen reads the Facturae schemas 3.2, 3.2.1 and 3.2.2, from
// a local directory (-src) or downloaded from facturae.gob.es, checks their
// SHA-256 and compiles them into one set, with one root per version. Beyond
// the constructs of FatturaPA (nested choices, here of single elements and
// possibly optional, and ds:Signature), the schemas use xs:double, complex
// content extensions that add nothing (compiled as their base type), and
// xs:any processContents="strict" in the Extensions elements, compiled as
// skip: the schemas that would define their content are those of the
// sender.
//
// Only the subset of XML Schema used by these schemas is supported: named
// and anonymous complex types holding one sequence or one choice of element
// particles (local elements or references to global elements), simple
// content extensions and restrictions with attributes, simple types
// restricting a built-in type with enumeration, pattern, length and
// inclusive bound facets,
// attributes of a string type with a fixed value, and a sequence made of a
// single xsd:any of namespace ##other. Anything else stops the generator
// with an error, so that a new version of the schemas never compiles
// silently into wrong tables.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"
	"time"
)

const nsXS = "http://www.w3.org/2001/XMLSchema"

// profiles, in the order of the generated tables (and of the profile bit
// masks). The token is the file name suffix that identifies the main schema.
var profiles = []struct {
	name   string // display name
	tokens []string
}{
	{"MINIMUM", []string{"MINIMUM"}},
	{"BASIC WL", []string{"BASICWL", "BASIC-WL", "BASIC_WL"}},
	{"BASIC", []string{"BASIC"}},
	{"EN 16931", []string{"EN16931"}},
	{"EXTENDED", []string{"EXTENDED"}},
}

// The OASIS UBL 2.1 package.
const (
	ublArchive = "UBL-2.1.zip"
	ublURL     = "https://docs.oasis-open.org/ubl/os-UBL-2.1/" + ublArchive
	ublSum     = "60b80d76394a8a2add90723ecb8e0e2e9d826775de9749df37a72d60703f86ed"
	ublVersion = "2.1"
)

// The schema of the Croatian extension of UBL in the package "UBL2.1
// eRačun" of the Porezna uprava (15/04/2026), compiled with the UBL schemas
// at the path of ublExtensionDocs.
const (
	hrArchive    = "UBL2.1 eRačun.zip"
	hrURL        = "https://porezna.gov.hr/fiskalizacija/api/dokumenti/198"
	hrArchiveSum = "549ced211d2b463db0e5821ed9e604580140aa1870ee8636d207206d872c8388"
	hrFile       = "UBL2.1 eRačun/XsdUbl/common/HRExtensionAggregateComponents-1.xsd"
	hrFileSum    = "c5aae7d62509029a987156ddc563972bdc4326658b43c6c424002efecfff275f"
	nsHR         = "urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"
)

// The FatturaPA 1.2.4 schema. Schema_VFPR12_v1.2.4.xsd, published for the
// invoices between private parties, is the same file.
const (
	fpaURL     = "https://www.fatturapa.gov.it/export/documenti/fatturapa/v1.5/" + fpaFile
	fpaFile    = "Schema_VFPA12_v1.2.4.xsd"
	fpaSum     = "29728f77e2739e150131258279218936287630e95566cab360cf66d53fc8f338"
	fpaVersion = "1.2.4"
	nsFPA      = "http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"
	nsDS       = "http://www.w3.org/2000/09/xmldsig#"
)

// The Facturae schemas compiled with -facturae, from the oldest version, and
// the URL they are published at.
var facturaeFiles = []struct{ version, ns, name, sum string }{
	{"3.2", "http://www.facturae.es/Facturae/2009/v3.2/Facturae", "Facturaev3_2.xml", "05f390ac7e2fd36da6567185cd0e420764e304b6ba491af51a8a100b07283396"},
	{"3.2.1", "http://www.facturae.es/Facturae/2014/v3.2.1/Facturae", "Facturaev3_2_1.xml", "f0f24f27729db91fdd90b715a5dc16f2d779612b4e9e9f193c2903f0e67dc6a6"},
	{"3.2.2", "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml", "Facturaev3_2_2.xml", "b4bbcd587f5fb0a8a906336cca09b0a40d06ffaa78c6a62f6e438c4e6ea86e07"},
}

const (
	facturaeURL  = "https://www.facturae.gob.es/content/dam/facturae/formato/versiones/"
	facturaeRoot = "Facturae"
)

// rootDoc is a document of a single schema set: its main schema and its
// root element.
type rootDoc struct{ file, ns, root string }

// ublDocs lists the UBL documents compiled, in the order of the generated
// roots: their main schema in the package and their root element.
var ublDocs = []rootDoc{
	{"xsdrt/maindoc/UBL-Invoice-2.1.xsd", "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2", "Invoice"},
	{"xsdrt/maindoc/UBL-CreditNote-2.1.xsd", "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2", "CreditNote"},
}

// ublExtensionDocs lists the schemas of the UBL extensions compiled, with
// the global element that ext:ExtensionContent may hold. The Croatian schema
// is placed beside the common UBL schemas, whose files it imports by name.
var ublExtensionDocs = []rootDoc{
	{"xsdrt/common/HRExtensionAggregateComponents-1.xsd", nsHR, "HRFISK20Data"},
}

// ciiSet describes a complete UN/CEFACT CII schema compiled with -cii.
type ciiSet struct {
	prefix string // prefix of the generated identifiers
	origin string // publisher and release, for the generated comments
	// The files are in the zip archive at archive (SHA-256 archiveSum),
	// under dir; or, when archive is empty, at the URL dir + name.
	archive, archiveSum string
	dir                 string
	files               []pinnedFile // every schema file, the main one first
}

// pinnedFile is a schema file and its SHA-256.
type pinnedFile struct{ name, sum string }

// ciiSets lists the schemas -cii compiles, by version.
var ciiSets = map[string]ciiSet{
	"D16B": {
		prefix:     "d16b",
		origin:     "KoSIT validator configuration for XRechnung 3.0.2, release 2026-08-31",
		archive:    "https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v2026-08-31/xrechnung-3.0.2-validator-configuration-2026-08-31.zip",
		archiveSum: "2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8",
		dir:        "resources/cii/16b/xsd/",
		files: []pinnedFile{
			{"CrossIndustryInvoice_100pD16B.xsd", "3baf143b90f289d62c5dfb086aa51fa183490a2d0b38a40c6b31dcae0760f2ba"},
			{"CrossIndustryInvoice_QualifiedDataType_100pD16B.xsd", "6649215914728652e883662bec1d3eab008c215e86a59f3bb417db9723ee8c5c"},
			{"CrossIndustryInvoice_ReusableAggregateBusinessInformationEntity_100pD16B.xsd", "1c42244298a863bcc0b5196983ff0a5d084fcabe58aea77ef82b242e7cd0ddfd"},
			{"CrossIndustryInvoice_UnqualifiedDataType_100pD16B.xsd", "04fce6f6f28521886731294be18a4e6bb5cb3003456457d99ad5c4d3e652f497"},
		},
	},
	"D22B": {
		prefix: "d22b",
		origin: "FNFE-MPE France_RFE 1.4.0.04 (tag v1.4.0.04, commit 97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d)",
		dir:    "https://raw.githubusercontent.com/fnfempe/France_RFE/97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d/FNFE_RFE_INVOICE/CII/1xsd-CII_D22B_uncoupled/",
		files: []pinnedFile{
			{"CrossIndustryInvoice_100pD22B.xsd", "7be4b747f29e12634d269e5b456e1d58d7aece2817e23a2ea656bbf78afed80f"},
			{"CrossIndustryInvoice_100pD22B_urn_un_unece_uncefact_data_standard_QualifiedDataType_100.xsd", "bc85893ae92561a9a9a760d1d624f340694858ea9b760c9167598da8cc2abe39"},
			{"CrossIndustryInvoice_100pD22B_urn_un_unece_uncefact_data_standard_ReusableAggregateBusinessInformationEntity_100.xsd", "da59380415e1efe1032e0f787986961efd69c6689906747ed56fd4d17c4dfe61"},
			{"CrossIndustryInvoice_100pD22B_urn_un_unece_uncefact_data_standard_UnqualifiedDataType_100.xsd", "58acb786f7cab1d70bb179ec6ff3add2e57fdb55f1c3995d3e05887bd2d717a9"},
		},
	},
}

// Namespace and root element of a CII invoice.
const (
	nsRSM   = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	ciiRoot = "CrossIndustryInvoice"
)

// Conventional prefixes of the namespaces, used in display names.
var prefixes = map[string]string{
	"urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100":                       "rsm",
	"urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100": "ram",
	"urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100":                        "udt",
	"urn:un:unece:uncefact:data:standard:QualifiedDataType:100":                          "qdt",

	"urn:oasis:names:specification:ubl:schema:xsd:Invoice-2":                   "ubl",
	"urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2":                "cn",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2": "cac",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2":     "cbc",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2": "ext",
	"urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2":      "udt",
	"urn:oasis:names:specification:ubl:schema:xsd:QualifiedDataTypes-2":        "qdt",
	"urn:un:unece:uncefact:data:specification:CoreComponentTypeSchemaModule:2": "ccts-cct",

	nsHR: "hrextac",

	nsFPA: "p",
	nsDS:  "ds",

	"http://www.facturae.es/Facturae/2009/v3.2/Facturae":              "fe32",
	"http://www.facturae.es/Facturae/2014/v3.2.1/Facturae":            "fe321",
	"http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml": "fe322",
}

func main() {
	out := flag.String("o", "", "output file (default tables_gen.go, cii_d16b_tables_gen.go with -cii D16B, ubl_tables_gen.go with -ubl)")
	ubl := flag.Bool("ubl", false, "compile the OASIS UBL 2.1 schemas of Invoice and CreditNote")
	zipPath := flag.String("zip", "", "with -ubl: path of "+ublArchive+" (downloaded from "+ublURL+" when empty)")
	hrPath := flag.String("hr", "", "with -ubl: path of the Croatian package "+hrArchive+" (downloaded from "+hrURL+" when empty)")
	cii := flag.String("cii", "", "compile the complete UN/CEFACT CII schema of this version (D16B or D22B)")
	srcPath := flag.String("src", "", "with -cii: the KoSIT archive (D16B) or a directory holding the schema files; with -fatturapa: the schema file; with -facturae: a directory holding the schema files (downloaded when empty)")
	fpa := flag.Bool("fatturapa", false, "compile the FatturaPA "+fpaVersion+" schema")
	facturae := flag.Bool("facturae", false, "compile the Facturae 3.2, 3.2.1 and 3.2.2 schemas")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gen [-o tables_gen.go] <directory of the Factur-X XSD files>")
		fmt.Fprintln(os.Stderr, "       gen -cii D16B|D22B [-src path] [-o cii_d16b_tables_gen.go]")
		fmt.Fprintln(os.Stderr, "       gen -ubl [-zip "+ublArchive+"] [-hr \""+hrArchive+"\"] [-o ubl_tables_gen.go]")
		fmt.Fprintln(os.Stderr, "       gen -fatturapa [-src "+fpaFile+"] [-o fatturapa_tables_gen.go]")
		fmt.Fprintln(os.Stderr, "       gen -facturae [-src dir] [-o facturae_tables_gen.go]")
		flag.PrintDefaults()
	}
	flag.Parse()
	var (
		src []byte
		err error
	)
	switch {
	case *facturae && !*fpa && *cii == "" && !*ubl && *zipPath == "" && *hrPath == "" && flag.NArg() == 0:
		if *out == "" {
			*out = "facturae_tables_gen.go"
		}
		src, err = generateFacturae(*srcPath)
	case *fpa && !*facturae && *cii == "" && !*ubl && *zipPath == "" && *hrPath == "" && flag.NArg() == 0:
		if *out == "" {
			*out = "fatturapa_tables_gen.go"
		}
		src, err = generateFatturaPA(*srcPath)
	case *cii != "" && !*ubl && !*fpa && !*facturae && *zipPath == "" && *hrPath == "" && flag.NArg() == 0:
		if *out == "" {
			*out = "cii_" + strings.ToLower(*cii) + "_tables_gen.go"
		}
		src, err = generateCII(*cii, *srcPath)
	case *ubl && *cii == "" && !*fpa && !*facturae && *srcPath == "" && flag.NArg() == 0:
		if *out == "" {
			*out = "ubl_tables_gen.go"
		}
		src, err = generateUBLArchive(*zipPath, *hrPath)
	case !*ubl && !*fpa && !*facturae && *cii == "" && *srcPath == "" && flag.NArg() == 1 && flag.Arg(0) != "" && *zipPath == "" && *hrPath == "":
		if *out == "" {
			*out = "tables_gen.go"
		}
		src, err = generate(flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

// generate compiles the Factur-X schemas found in dir and returns the Go
// source.
func generate(dir string) ([]byte, error) {
	fsys := os.DirFS(dir)
	mains, version, err := findMains(fsys, dir)
	if err != nil {
		return nil, err
	}
	c := newCompiler(false)
	for i, name := range mains {
		if err := c.compileProfile(fsys, i, name); err != nil {
			return nil, fmt.Errorf("%s: %w", profiles[i].name, err)
		}
	}
	return c.emit(version, nil)
}

// generateUBLArchive reads the UBL 2.1 package from path and the Croatian
// package from hrPath, or downloads them when the path is empty, checks
// their SHA-256 and compiles the UBL schemas with the schema of the
// Croatian extension.
func generateUBLArchive(path, hrPath string) ([]byte, error) {
	zr, err := openArchive(path, ublURL, ublSum)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ublArchive, err)
	}
	hr, err := openArchive(hrPath, hrURL, hrArchiveSum)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", hrArchive, err)
	}
	data, err := fs.ReadFile(hr, hrFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", hrArchive, err)
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != hrFileSum {
		return nil, fmt.Errorf("%s: %s: SHA-256 %s, want %s", hrArchive, hrFile, got, hrFileSum)
	}
	return generateUBL(overlay{zr, fstest.MapFS{ublExtensionDocs[0].file: &fstest.MapFile{Data: data}}})
}

// overlay is a file system that adds the files of add to FS.
type overlay struct {
	fs.FS
	add fstest.MapFS
}

func (o overlay) Open(name string) (fs.File, error) {
	if _, ok := o.add[name]; ok {
		return o.add.Open(name)
	}
	return o.FS.Open(name)
}

// openArchive reads the zip archive at path, or downloads it from url when
// path is empty, and checks its SHA-256.
func openArchive(path, url, sum string) (*zip.Reader, error) {
	var (
		data []byte
		err  error
	)
	if path != "" {
		data, err = os.ReadFile(path)
	} else {
		data, err = download(url)
	}
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != sum {
		return nil, fmt.Errorf("SHA-256 %s, want %s", got, sum)
	}
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// generateCII compiles the complete CII schema of version v, read from src
// (see ciiFiles), and returns the Go source.
func generateCII(v, src string) ([]byte, error) {
	set, ok := ciiSets[v]
	if !ok {
		return nil, fmt.Errorf("unknown CII schema version %q (known: D16B, D22B)", v)
	}
	fsys, err := ciiFiles(set, src)
	if err != nil {
		return nil, fmt.Errorf("CII %s: %w", v, err)
	}
	return compileCII(v, set, fsys)
}

// compileCII compiles the CII schema set whose files are in fsys.
func compileCII(v string, set ciiSet, fsys fs.FS) ([]byte, error) {
	c := newCompiler(true)
	if err := c.compileSingle(fsys, []rootDoc{{set.files[0].name, nsRSM, ciiRoot}}, nil); err != nil {
		return nil, fmt.Errorf("CII %s: %w", v, err)
	}
	source := "Source: " + set.origin + ","
	if set.archive != "" {
		source += "\n// " + set.archive + " (SHA-256 " + set.archiveSum + "),\n// directory " + set.dir
	} else {
		source += "\n// " + set.dir
	}
	return c.emit(v, &setInfo{
		prefix:  set.prefix,
		title:   "UN/CEFACT CII " + v,
		source:  source + " (SHA-256 of each file):",
		schemas: "CII schema",
		roots:   "the root element of the CII invoice",
	})
}

// ciiFiles returns the schema files of set, each checked against its
// SHA-256. src is the zip archive of set (when set.archive is not empty) or
// a directory holding the files; when it is empty, the files are
// downloaded.
func ciiFiles(set ciiSet, src string) (fs.FS, error) {
	var read func(name string) ([]byte, error)
	switch {
	case set.archive != "" && (src == "" || strings.EqualFold(path.Ext(src), ".zip")):
		zr, err := openArchive(src, set.archive, set.archiveSum)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path.Base(set.archive), err)
		}
		read = func(name string) ([]byte, error) { return fs.ReadFile(zr, set.dir+name) }
	case src != "":
		read = func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(src, name)) }
	default:
		read = func(name string) ([]byte, error) { return download(set.dir + name) }
	}
	fsys := fstest.MapFS{}
	for _, f := range set.files {
		data, err := read(f.name)
		if err != nil {
			return nil, err
		}
		h := sha256.Sum256(data)
		if got := hex.EncodeToString(h[:]); got != f.sum {
			return nil, fmt.Errorf("%s: SHA-256 %s, want %s", f.name, got, f.sum)
		}
		fsys[f.name] = &fstest.MapFile{Data: data}
	}
	return fsys, nil
}

// generateUBL compiles the UBL schemas of fsys, laid out as in the UBL 2.1
// package, with the schemas of the extensions of ublExtensionDocs, and
// returns the Go source.
func generateUBL(fsys fs.FS) ([]byte, error) {
	c := newCompiler(true)
	if err := c.compileSingle(fsys, ublDocs, ublExtensionDocs); err != nil {
		return nil, fmt.Errorf("UBL: %w", err)
	}
	return c.emit(ublVersion, &setInfo{
		prefix: "ubl",
		title:  "OASIS UBL " + ublVersion,
		source: fmt.Sprintf("Source: %s (SHA-256 %s),\n// the runtime schemas of Invoice and CreditNote, and the schema of the Croatian\n// extension of the package %q of the Porezna uprava,\n// %s (SHA-256 %s),\n// placed as %s (SHA-256 of each file):",
			ublURL, ublSum, "UBL2.1 eRačun", hrURL, hrArchiveSum, ublExtensionDocs[0].file),
		schemas:    "UBL schemas",
		roots:      "the root element of each UBL document",
		extensions: "the global element of each extension schema, which ext:ExtensionContent may hold",
	})
}

// generateFatturaPA reads the FatturaPA schema from path, or downloads it
// when path is empty, checks its SHA-256 and compiles it.
func generateFatturaPA(path string) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if path != "" {
		data, err = os.ReadFile(path)
	} else {
		data, err = download(fpaURL)
	}
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(data)
	if got := hex.EncodeToString(h[:]); got != fpaSum {
		return nil, fmt.Errorf("%s: SHA-256 %s, want %s", fpaFile, got, fpaSum)
	}
	return compileFatturaPA(fstest.MapFS{fpaFile: &fstest.MapFile{Data: data}}, fpaFile, fpaVersion)
}

// compileFatturaPA compiles the FatturaPA schema file name of fsys, of the
// given version.
func compileFatturaPA(fsys fs.FS, name, version string) ([]byte, error) {
	c := newCompiler(true)
	c.skip = map[string]bool{nsDS: true}
	if err := c.compileSingle(fsys, []rootDoc{{name, nsFPA, "FatturaElettronica"}}, nil); err != nil {
		return nil, fmt.Errorf("FatturaPA: %w", err)
	}
	return c.emit(version, &setInfo{
		prefix:  "fpa",
		title:   "FatturaPA " + version,
		source:  fmt.Sprintf("Source: %s (SHA-256 %s),\n// the FatturaPA schema of the Agenzia delle Entrate, also published as\n// Schema_VFPR12_v1.2.4.xsd (SHA-256 of the file):", fpaURL, fpaSum),
		schemas: "FatturaPA schema",
		roots:   "the root element of a FatturaPA document",
	})
}

// generateFacturae compiles the Facturae schemas, read from the directory
// src or downloaded from facturae.gob.es when src is empty; every file must
// have the SHA-256 recorded in facturaeFiles.
func generateFacturae(src string) ([]byte, error) {
	files := fstest.MapFS{}
	for _, f := range facturaeFiles {
		var (
			data []byte
			err  error
		)
		if src != "" {
			data, err = os.ReadFile(filepath.Join(src, f.name))
		} else {
			data, err = download(facturaeURL + f.name)
		}
		if err != nil {
			return nil, fmt.Errorf("facturae %s: %w", f.version, err)
		}
		h := sha256.Sum256(data)
		if got := hex.EncodeToString(h[:]); got != f.sum {
			return nil, fmt.Errorf("facturae %s: %s: SHA-256 %s, want %s", f.version, f.name, got, f.sum)
		}
		files[f.name] = &fstest.MapFile{Data: data}
	}
	return compileFacturae(files)
}

// compileFacturae compiles the Facturae schemas of facturaeFiles, read from
// fsys, into one schema set with one root per version. The children of the
// root element are not qualified (the schemas have no elementFormDefault).
// The XML signature (ds:Signature) is an element whose content is not
// checked, the xmldsig schema being never read, and the content of the
// Extensions elements (xs:any namespace="##other" processContents="strict")
// is not checked either: the schemas that would define it are those of the
// sender.
func compileFacturae(fsys fs.FS) ([]byte, error) {
	c := newCompiler(true)
	c.skip = map[string]bool{nsDS: true}
	c.strictAsSkip = true
	docs := make([]rootDoc, len(facturaeFiles))
	versions := make([]string, len(facturaeFiles))
	for i, f := range facturaeFiles {
		docs[i] = rootDoc{f.name, f.ns, facturaeRoot}
		versions[i] = f.version
	}
	if err := c.compileSingle(fsys, docs, nil); err != nil {
		return nil, fmt.Errorf("facturae: %w", err)
	}
	return c.emit(facturaeFiles[len(facturaeFiles)-1].version, &setInfo{
		prefix: "facturae",
		title:  "Facturae " + strings.Join(versions, ", "),
		source: "Source: facturae.gob.es,\n// " + facturaeURL + "<file> (SHA-256 of the first 8 bytes below).\n" +
			"// Origen de los datos: Ministerio de Hacienda y Administraciones Públicas - Ministerio de Industria, Energía y Turismo.\n" +
			"// Reused under Ley 37/2007 and Real Decreto 1495/2011; the schemas give no date of update.",
		schemas: "latest Facturae schema",
		roots:   "the root element of each version: " + strings.Join(versions, ", "),
	})
}

// findMains returns the main schema of each profile.
func findMains(fsys fs.FS, dir string) ([]string, string, error) {
	mains := make([]string, len(profiles))
	var version string
	verRE := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)*)`)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(path.Ext(name), ".xsd") {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), path.Ext(d.Name()))
		upper := strings.ToUpper(base)
		for i, p := range profiles {
			for _, tok := range p.tokens {
				if !strings.HasSuffix(upper, "_"+tok) {
					continue
				}
				if mains[i] != "" && mains[i] != name {
					return fmt.Errorf("two main schemas for %s: %s and %s", p.name, mains[i], name)
				}
				mains[i] = name
				if v := verRE.FindString(base); v != "" && newerVersion(v, version) {
					version = v
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", dir, err)
	}
	for i, m := range mains {
		if m == "" {
			return nil, "", fmt.Errorf("no main schema for profile %s in %s", profiles[i].name, dir)
		}
	}
	return mains, version, nil
}

// newerVersion reports whether the dotted version v is greater than cur
// ("" being lower than any version). The tables take the highest version
// of the main schemas: a corrigendum such as Factur-X 1.09.2 only changes
// some profiles, and the schemas of the others may keep the file names of
// the release they come from.
func newerVersion(v, cur string) bool {
	if cur == "" {
		return true
	}
	a, b := strings.Split(v, "."), strings.Split(cur, ".")
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// xnode is a generic XML element of a schema document.
type xnode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []*xnode   `xml:",any"`
}

func (n *xnode) attr(name string) string {
	v, _ := n.lookupAttr(name)
	return v
}

// lookupAttr returns the value of the unqualified attribute name and whether
// it is present.
func (n *xnode) lookupAttr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// is reports whether n is the XML Schema element local.
func (n *xnode) is(local string) bool {
	return n.XMLName.Space == nsXS && n.XMLName.Local == local
}

// checkAttrs reports an error when n has an unqualified attribute that is
// not in allowed: an attribute such as fixed, default or nillable would
// change the meaning of the declaration.
func (n *xnode) checkAttrs(allowed ...string) error {
	for _, a := range n.Attrs {
		if a.Name.Space != "" {
			continue
		}
		ok := false
		for _, s := range allowed {
			if a.Name.Local == s {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("unsupported attribute %s on xs:%s", a.Name.Local, n.XMLName.Local)
		}
	}
	return nil
}

// schemaDoc is one parsed schema file.
type schemaDoc struct {
	path              string // in the file system of the schemas
	data              []byte
	tns               string
	ns                map[string]string // prefixes declared on xs:schema
	imports           map[string]string // namespace: path of the imported schema (lazy imports)
	qualifiedElements bool
}

type qname struct{ space, local string }

func (q qname) String() string {
	if p, ok := prefixes[q.space]; ok {
		return p + ":" + q.local
	}
	if q.space == nsXS {
		return "xs:" + q.local
	}
	return "{" + q.space + "}" + q.local
}

// decl is a named schema component with the document that defines it.
type decl struct {
	doc  *schemaDoc
	node *xnode
}

// profileSchemas holds the components of one schema set (a Factur-X
// profile, or the UBL documents).
type profileSchemas struct {
	fsys     fs.FS
	lazy     bool // imports are read when one of their components is referenced
	docs     map[string]*schemaDoc
	complex  map[qname]decl
	simple   map[qname]decl
	elements map[qname]decl
	files    []*schemaDoc // in reading order
}

func newSchemas(fsys fs.FS, lazy bool) *profileSchemas {
	return &profileSchemas{fsys: fsys, lazy: lazy, docs: map[string]*schemaDoc{},
		complex: map[qname]decl{}, simple: map[qname]decl{}, elements: map[qname]decl{}}
}

func (ps *profileSchemas) load(name string) (*schemaDoc, error) {
	name = path.Clean(name)
	if doc, ok := ps.docs[name]; ok {
		return doc, nil
	}
	data, err := fs.ReadFile(ps.fsys, name)
	if err != nil {
		return nil, err
	}
	var root xnode
	if err := xml.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &root); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if !root.is("schema") {
		return nil, fmt.Errorf("%s: root element is not xs:schema", name)
	}
	doc := &schemaDoc{path: name, data: data, tns: root.attr("targetNamespace"), ns: map[string]string{}, imports: map[string]string{}}
	doc.qualifiedElements = root.attr("elementFormDefault") == "qualified"
	if f := root.attr("attributeFormDefault"); f == "qualified" {
		return nil, fmt.Errorf("%s: attributeFormDefault=qualified is not supported", name)
	}
	for _, a := range root.Attrs {
		switch {
		case a.Name.Space == "xmlns":
			doc.ns[a.Name.Local] = a.Value
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			doc.ns[""] = a.Value
		}
	}
	ps.docs[name] = doc
	ps.files = append(ps.files, doc)
	for _, c := range root.Children {
		qn := qname{doc.tns, c.attr("name")}
		switch {
		case c.is("import"), c.is("include"):
			loc := c.attr("schemaLocation")
			if loc == "" {
				return nil, fmt.Errorf("%s: import of %s without schemaLocation", name, c.attr("namespace"))
			}
			target := path.Join(path.Dir(name), loc)
			if c.is("import") && ps.lazy {
				ns := c.attr("namespace")
				if ns == doc.tns {
					return nil, fmt.Errorf("%s: import of the target namespace %s", name, ns)
				}
				doc.imports[ns] = target
				continue
			}
			sub, err := ps.load(target)
			if err != nil {
				return nil, err
			}
			if c.is("include") && sub.tns != doc.tns {
				return nil, fmt.Errorf("%s: included schema %s has target namespace %q, not %q", name, sub.path, sub.tns, doc.tns)
			}
		case c.is("complexType"):
			if err := define(ps.complex, qn, decl{doc, c}); err != nil {
				return nil, err
			}
		case c.is("simpleType"):
			if err := define(ps.simple, qn, decl{doc, c}); err != nil {
				return nil, err
			}
		case c.is("element"):
			if err := define(ps.elements, qn, decl{doc, c}); err != nil {
				return nil, err
			}
		case c.is("annotation"):
		default:
			return nil, fmt.Errorf("%s: unsupported top-level xs:%s", name, c.XMLName.Local)
		}
	}
	return doc, nil
}

// define registers a top-level component, which must be new.
func define(m map[qname]decl, q qname, d decl) error {
	if prev, ok := m[q]; ok {
		return fmt.Errorf("%s: %s already defined in %s", d.doc.path, q, prev.doc.path)
	}
	m[q] = d
	return nil
}

// need reads, for lazy imports, the schema that doc imports for namespace
// ns.
func (ps *profileSchemas) need(doc *schemaDoc, ns string) error {
	if !ps.lazy || ns == nsXS || ns == doc.tns {
		return nil
	}
	loc, ok := doc.imports[ns]
	if !ok {
		return fmt.Errorf("%s: namespace %s is used but not imported", doc.path, ns)
	}
	_, err := ps.load(loc)
	return err
}

// resolve turns a QName written in doc into a namespace and local name.
func (doc *schemaDoc) resolve(s string) (qname, error) {
	prefix, local, ok := strings.Cut(s, ":")
	if !ok {
		prefix, local = "", s
	}
	uri, found := doc.ns[prefix]
	if !found {
		if prefix == "" {
			return qname{"", local}, nil
		}
		return qname{}, fmt.Errorf("%s: undeclared prefix in %q", doc.path, s)
	}
	return qname{uri, local}, nil
}

// Simple type kinds, as named in package schema.
const (
	kString   = "kString"
	kDecimal  = "kDecimal"
	kInteger  = "kInteger"
	kBoolean  = "kBoolean"
	kBase64   = "kBase64"
	kDate     = "kDate"
	kTime     = "kTime"
	kDateTime = "kDateTime"
	kDouble   = "kDouble"
)

// builtins maps the supported built-in types to their kind. The string-like
// types accept any value (their white space facet does not restrict the
// lexical space), except those of builtinPatterns.
var builtins = map[string]string{
	"string": kString, "normalizedString": kString, "token": kString, "anyURI": kString,
	"NMTOKEN": kString, "Name": kString, "NCName": kString, "language": kString, "ID": kString,
	"decimal": kDecimal,
	"integer": kInteger, "long": kInteger, "int": kInteger, "short": kInteger,
	"nonNegativeInteger": kInteger, "positiveInteger": kInteger,
	"boolean":      kBoolean,
	"base64Binary": kBase64,
	"date":         kDate,
	"time":         kTime,
	"dateTime":     kDateTime,
	"double":       kDouble, "float": kDouble,
}

// builtinPatterns gives the pattern facet of the built-in string types whose
// pattern can be checked with package regexp.
var builtinPatterns = map[string]string{
	"language": `[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*`,
}

// collapsing reports whether white space is collapsed for the built-in type
// (it matters for enumeration and length facets).
var collapsing = map[string]bool{"token": true, "NMTOKEN": true, "Name": true, "NCName": true, "language": true, "ID": true, "anyURI": true,
	"double": true, "float": true}

// facets of a simple type.
type gFacets struct {
	enum             []string
	pattern          []string
	min, max         int // lengths; -1 = none
	collapse         bool
	minIncl, maxIncl string // inclusive bounds; "" = none
}

func (f *gFacets) empty() bool {
	return f == nil || (len(f.enum) == 0 && len(f.pattern) == 0 && f.min < 0 && f.max < 0 && f.minIncl == "" && f.maxIncl == "")
}

func (f *gFacets) key() string {
	if f.empty() {
		return ""
	}
	k := fmt.Sprintf("%q/%q/%d/%d/%t", f.enum, f.pattern, f.min, f.max, f.collapse)
	if f.minIncl != "" || f.maxIncl != "" {
		k += fmt.Sprintf("/%q/%q", f.minIncl, f.maxIncl)
	}
	return k
}

// gSimple is a resolved simple type.
type gSimple struct {
	name   string // display name of the type, for messages
	kind   string
	facets *gFacets
}

// Content kinds, as named in package schema.
const (
	contentEmpty    = "contentEmpty"
	contentElements = "contentElements"
	contentSimple   = "contentSimple"
	contentAny      = "contentAny"
	contentSkip     = "contentSkip"
)

// gAlt is one sequence of a choice flattened into the sequence of a type
// (see alternative in package schema).
type gAlt struct {
	group, first, n int
	need            uint8
}

type gPart struct {
	name     string
	ns       string
	typ      *gType
	min, max int // max -1 = unbounded
}

type gAttr struct {
	name     string
	simple   gSimple
	required bool
}

type gType struct {
	name     string // display name
	content  string
	choice   bool
	min, max int // occurrences of the choice, or of the wildcard
	other    string
	simple   gSimple
	parts    []gPart
	attrs    []gAttr
	alts     []gAlt
	def      string // default value of the element (simple content)
	profiles uint8
	id       int
	key      string
}

type compiler struct {
	// single is set for one schema set shared by several roots (UBL): named
	// complex types are compiled once and may be recursive, imports are lazy,
	// and a simple content extension that adds nothing is the type it
	// extends.
	single     bool
	byKey      map[string]*gType
	types      []*gType
	roots      []gPart
	extensions []gPart        // global elements of the extension schemas (UBL)
	files      [][]*schemaDoc // per profile
	namespaces []string
	inProgress map[string]bool
	// skip lists the namespaces whose schemas are not read: a reference to
	// one of their elements compiles to an element that is not checked.
	skip map[string]bool
	// strictAsSkip compiles a wildcard of processContents strict as skip:
	// the elements of the other namespaces are accepted unchecked, the
	// schemas that would define them not being part of the set (the
	// Extensions of Facturae).
	strictAsSkip bool
}

func newCompiler(single bool) *compiler {
	return &compiler{single: single, byKey: map[string]*gType{}, inProgress: map[string]bool{}}
}

func (c *compiler) compileProfile(fsys fs.FS, p int, name string) error {
	ps := newSchemas(fsys, false)
	if _, err := ps.load(name); err != nil {
		return err
	}
	c.files = append(c.files, ps.files)
	pc := &profileCompiler{c: c, ps: ps, p: p, memo: map[qname]*gType{}}
	root, ok := ps.elements[qname{nsRSM, ciiRoot}]
	if !ok {
		return errors.New("no global element rsm:CrossIndustryInvoice")
	}
	part, err := pc.element(root.doc, root.node, true)
	if err != nil {
		return err
	}
	c.roots = append(c.roots, part)
	return nil
}

// compileSingle compiles the documents docs into one schema set (the UBL
// documents, or a complete CII schema), with the global elements exts of
// the extension schemas.
func (c *compiler) compileSingle(fsys fs.FS, docs, exts []rootDoc) error {
	ps := newSchemas(fsys, true)
	pc := &profileCompiler{c: c, ps: ps, memo: map[qname]*gType{}}
	compile := func(d rootDoc) (gPart, error) {
		if _, err := ps.load(d.file); err != nil {
			return gPart{}, err
		}
		root, ok := ps.elements[qname{d.ns, d.root}]
		if !ok {
			return gPart{}, fmt.Errorf("%s: no global element %s", d.file, qname{d.ns, d.root})
		}
		return pc.element(root.doc, root.node, true)
	}
	for _, d := range docs {
		part, err := compile(d)
		if err != nil {
			return err
		}
		c.roots = append(c.roots, part)
	}
	for _, d := range exts {
		part, err := compile(d)
		if err != nil {
			return err
		}
		c.extensions = append(c.extensions, part)
	}
	c.files = append(c.files, ps.files)
	return nil
}

type profileCompiler struct {
	c    *compiler
	ps   *profileSchemas
	p    int
	memo map[qname]*gType
}

// ref resolves a QName written in doc, reading the schema of its namespace
// when imports are lazy.
func (pc *profileCompiler) ref(doc *schemaDoc, s string) (qname, error) {
	q, err := doc.resolve(s)
	if err != nil {
		return q, err
	}
	return q, pc.ps.need(doc, q.space)
}

// intern returns the shared copy of t, registering it when it is new.
func (pc *profileCompiler) intern(t *gType) *gType {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%t|%d|%d|%s|%s|%s", t.name, t.content, t.choice, t.min, t.max, t.simple.kind, t.simple.name, t.simple.facets.key())
	if t.def != "" {
		fmt.Fprintf(&b, "|default:%q", t.def)
	}
	if t.content == contentAny {
		fmt.Fprintf(&b, "|any:%s", t.other)
	}
	for _, a := range t.attrs {
		fmt.Fprintf(&b, "|@%s:%s:%s:%s:%t", a.name, a.simple.kind, a.simple.name, a.simple.facets.key(), a.required)
	}
	for _, p := range t.parts {
		fmt.Fprintf(&b, "|%s{%s}#%d:%d:%d", p.name, p.ns, p.typ.id, p.min, p.max)
	}
	for _, a := range t.alts {
		fmt.Fprintf(&b, "|alt%d:%d:%d:%d", a.group, a.first, a.n, a.need)
	}
	t.key = b.String()
	if s, ok := pc.c.byKey[t.key]; ok {
		s.profiles |= 1 << pc.p
		return s
	}
	pc.register(t)
	pc.c.byKey[t.key] = t
	return t
}

// register adds t to the types, without sharing.
func (pc *profileCompiler) register(t *gType) {
	t.profiles = 1 << pc.p
	t.id = len(pc.c.types)
	pc.c.types = append(pc.c.types, t)
}

func (pc *profileCompiler) addNamespace(ns string) {
	for _, n := range pc.c.namespaces {
		if n == ns {
			return
		}
	}
	pc.c.namespaces = append(pc.c.namespaces, ns)
}

// occurs reads minOccurs and maxOccurs.
func occurs(n *xnode) (min, max int, err error) {
	min, max = 1, 1
	if s := n.attr("minOccurs"); s != "" {
		if min, err = strconv.Atoi(s); err != nil || min < 0 || min > 254 {
			return 0, 0, fmt.Errorf("unsupported minOccurs %q", s)
		}
	}
	if s := n.attr("maxOccurs"); s != "" {
		if s == "unbounded" {
			max = -1
		} else if max, err = strconv.Atoi(s); err != nil || max < 1 || max > 254 {
			return 0, 0, fmt.Errorf("unsupported maxOccurs %q", s)
		}
	}
	if max >= 0 && max < min {
		return 0, 0, fmt.Errorf("maxOccurs %d < minOccurs %d", max, min)
	}
	return min, max, nil
}

// element compiles an element declaration (global or local) into a particle.
func (pc *profileCompiler) element(doc *schemaDoc, n *xnode, global bool) (gPart, error) {
	if ref := n.attr("ref"); ref != "" {
		if err := n.checkAttrs("ref", "minOccurs", "maxOccurs"); err != nil {
			return gPart{}, fmt.Errorf("%s: element %s: %w", doc.path, ref, err)
		}
		if q, err := doc.resolve(ref); err == nil && pc.c.skip[q.space] {
			pc.addNamespace(q.space)
			part := gPart{name: q.local, ns: q.space}
			part.typ = pc.intern(&gType{name: q.String() + " (not checked)", content: contentSkip})
			part.min, part.max, err = occurs(n)
			return part, err
		}
		q, err := pc.ref(doc, ref)
		if err != nil {
			return gPart{}, err
		}
		d, ok := pc.ps.elements[q]
		if !ok {
			return gPart{}, fmt.Errorf("unknown element %s", q)
		}
		part, err := pc.element(d.doc, d.node, true)
		if err != nil {
			return gPart{}, err
		}
		part.min, part.max, err = occurs(n)
		return part, err
	}
	name := n.attr("name")
	if name == "" {
		return gPart{}, fmt.Errorf("%s: element without name", doc.path)
	}
	allowed := []string{"name", "type", "minOccurs", "maxOccurs", "form", "default"}
	if global {
		allowed = allowed[:2]
	}
	if err := n.checkAttrs(allowed...); err != nil {
		return gPart{}, fmt.Errorf("%s: element %s: %w", doc.path, name, err)
	}
	ns := ""
	if global || doc.qualifiedElements || n.attr("form") == "qualified" {
		ns = doc.tns
	}
	if n.attr("form") == "unqualified" {
		ns = ""
	}
	if ns != "" {
		pc.addNamespace(ns)
	}
	part := gPart{name: name, ns: ns}
	var err error
	part.min, part.max, err = occurs(n)
	if err != nil {
		return gPart{}, fmt.Errorf("element %s: %w", name, err)
	}
	display := qname{ns, name}.String()
	if typ := n.attr("type"); typ != "" {
		q, err := pc.ref(doc, typ)
		if err != nil {
			return gPart{}, err
		}
		part.typ, err = pc.typeByName(q)
		if err != nil {
			return gPart{}, fmt.Errorf("element %s: %w", display, err)
		}
		if def, ok := n.lookupAttr("default"); ok {
			// The default value is the value of the element when it is
			// empty.
			if part.typ.content != contentSimple || len(part.typ.attrs) > 0 {
				return gPart{}, fmt.Errorf("element %s: a default value is only supported for a simple type", display)
			}
			d := *part.typ
			d.def = def
			part.typ = pc.intern(&d)
		}
		return part, nil
	}
	if _, ok := n.lookupAttr("default"); ok {
		return gPart{}, fmt.Errorf("element %s: a default value is only supported with a named type", display)
	}
	for _, c := range n.Children {
		switch {
		case c.is("complexType"):
			part.typ, err = pc.complexType(doc, c, "type of "+display)
		case c.is("simpleType"):
			var s gSimple
			s, err = pc.simpleType(doc, c, "type of "+display)
			if err == nil {
				part.typ = pc.intern(&gType{name: "type of " + display, content: contentSimple, simple: s})
			}
		case c.is("annotation"):
			continue
		default:
			err = fmt.Errorf("unsupported xs:%s", c.XMLName.Local)
		}
		if err != nil {
			return gPart{}, fmt.Errorf("element %s: %w", display, err)
		}
	}
	if part.typ == nil {
		// xs:anyType is not supported: every element of the schemas has a type.
		return gPart{}, fmt.Errorf("element %s has no type", display)
	}
	return part, nil
}

// typeByName compiles the named type q (complex, simple or built-in).
func (pc *profileCompiler) typeByName(q qname) (*gType, error) {
	if t, ok := pc.memo[q]; ok {
		return t, nil
	}
	key := fmt.Sprint(pc.p, q)
	if pc.c.inProgress[key] {
		return nil, fmt.Errorf("recursive type %s is not supported", q)
	}
	pc.c.inProgress[key] = true
	defer delete(pc.c.inProgress, key)
	var (
		t   *gType
		err error
	)
	if d, ok := pc.ps.complex[q]; ok {
		if pc.c.single {
			t, err = pc.namedComplex(q, d)
		} else {
			t, err = pc.complexType(d.doc, d.node, q.String())
		}
	} else {
		var s gSimple
		s, err = pc.simpleByName(q)
		if err == nil {
			t = pc.intern(&gType{name: q.String(), content: contentSimple, simple: s})
		}
	}
	if err != nil {
		return nil, err
	}
	pc.memo[q] = t
	return t, nil
}

// namedComplex compiles the named complex type q of a single schema set.
// The type is registered before its content, so that it may be recursive.
func (pc *profileCompiler) namedComplex(q qname, d decl) (*gType, error) {
	base, err := pc.plainExtension(d)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", q, err)
	}
	if base == (qname{}) {
		if base, err = pc.plainComplexExtension(d.doc, d.node); err != nil {
			return nil, fmt.Errorf("%s: %w", q, err)
		}
	}
	if base != (qname{}) {
		return pc.typeByName(base)
	}
	t := &gType{name: q.String(), content: contentEmpty, min: 1, max: 1}
	pc.register(t)
	pc.memo[q] = t
	if err := pc.complexBody(d.doc, d.node, t); err != nil {
		return nil, err
	}
	return t, nil
}

// plainExtension returns the base of the complex type d when d is a simple
// content extension of a complex type that adds nothing (no attribute), the
// zero qname otherwise.
func (pc *profileCompiler) plainExtension(d decl) (qname, error) {
	var ext *xnode
	for _, c := range d.node.Children {
		switch {
		case c.is("annotation"):
		case c.is("simpleContent") && ext == nil:
			for _, e := range c.Children {
				switch {
				case e.is("annotation"):
				case e.is("extension") && ext == nil:
					ext = e
				default:
					return qname{}, nil
				}
			}
		default:
			return qname{}, nil
		}
	}
	if ext == nil || d.node.attr("mixed") == "true" {
		return qname{}, nil
	}
	for _, a := range ext.Children {
		if !a.is("annotation") {
			return qname{}, nil
		}
	}
	base, err := pc.ref(d.doc, ext.attr("base"))
	if err != nil {
		return qname{}, err
	}
	if _, ok := pc.ps.complex[base]; !ok {
		return qname{}, nil
	}
	return base, nil
}

// plainComplexExtension returns the base of the complex type n when n is a
// complex content extension of a named complex type that adds nothing (no
// particle, no attribute), the zero qname otherwise: such a type is its
// base, as the anonymous type of the Tax of a Facturae invoice line is
// TaxOutputType.
func (pc *profileCompiler) plainComplexExtension(doc *schemaDoc, n *xnode) (qname, error) {
	var ext *xnode
	for _, c := range n.Children {
		switch {
		case c.is("annotation"):
		case c.is("complexContent") && ext == nil:
			if c.attr("mixed") == "true" {
				return qname{}, nil
			}
			for _, e := range c.Children {
				switch {
				case e.is("annotation"):
				case e.is("extension") && ext == nil:
					ext = e
				default:
					return qname{}, nil
				}
			}
		default:
			return qname{}, nil
		}
	}
	if ext == nil || n.attr("mixed") == "true" {
		return qname{}, nil
	}
	for _, a := range ext.Children {
		if !a.is("annotation") {
			return qname{}, fmt.Errorf("complex content extension of %s that adds content is not supported", ext.attr("base"))
		}
	}
	base, err := pc.ref(doc, ext.attr("base"))
	if err != nil {
		return qname{}, err
	}
	if _, ok := pc.ps.complex[base]; !ok {
		return qname{}, fmt.Errorf("complex content extension of %s, which is not a complex type", base)
	}
	return base, nil
}

// complexType compiles a complex type definition, shared with the
// structurally identical ones.
func (pc *profileCompiler) complexType(doc *schemaDoc, n *xnode, name string) (*gType, error) {
	base, err := pc.plainComplexExtension(doc, n)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if base != (qname{}) {
		return pc.typeByName(base)
	}
	t := &gType{name: name, content: contentEmpty, min: 1, max: 1}
	if err := pc.complexBody(doc, n, t); err != nil {
		return nil, err
	}
	return pc.intern(t), nil
}

// complexBody compiles the content of a complex type definition into t.
func (pc *profileCompiler) complexBody(doc *schemaDoc, n *xnode, t *gType) error {
	name := t.name
	if err := n.checkAttrs("name", "mixed"); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if n.attr("mixed") == "true" {
		return fmt.Errorf("%s: mixed content is not supported", name)
	}
	for _, c := range n.Children {
		switch {
		case c.is("annotation"):
		case c.is("sequence"), c.is("choice"):
			if t.content != contentEmpty || len(t.parts) > 0 {
				return fmt.Errorf("%s: several model groups", name)
			}
			if err := pc.group(doc, c, t); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		case c.is("simpleContent"):
			if t.content != contentEmpty || len(t.attrs) > 0 {
				return fmt.Errorf("%s: simple content after other content", name)
			}
			if err := pc.simpleContent(doc, c, t); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		case c.is("attribute"):
			a, err := pc.attribute(doc, c)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if a != nil {
				if err := addAttr(t, *a); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
		default:
			return fmt.Errorf("%s: unsupported xs:%s", name, c.XMLName.Local)
		}
	}
	return nil
}

// addAttr appends a to the attributes of t; its name must be new.
func addAttr(t *gType, a gAttr) error {
	for _, b := range t.attrs {
		if b.name == a.name {
			return fmt.Errorf("attribute %s declared twice", a.name)
		}
	}
	t.attrs = append(t.attrs, a)
	return nil
}

// group compiles the top-level sequence or choice of a complex type.
func (pc *profileCompiler) group(doc *schemaDoc, g *xnode, t *gType) error {
	min, max, err := occurs(g)
	if err != nil {
		return err
	}
	t.choice = g.is("choice")
	if !t.choice && (min != 1 || max != 1) {
		return errors.New("repeated or optional sequences are not supported")
	}
	t.min, t.max = min, max
	seen := map[string]bool{}
	add := func(part gPart) error {
		if seen[part.name] {
			// Children are matched by local name at run time.
			return fmt.Errorf("element name %s used twice in one content model", part.name)
		}
		seen[part.name] = true
		t.parts = append(t.parts, part)
		return nil
	}
	nParticles := 0
	for _, c := range g.Children {
		if !c.is("annotation") {
			nParticles++
		}
	}
	groups := 0
	for _, c := range g.Children {
		switch {
		case c.is("annotation"):
		case c.is("element"):
			part, err := pc.element(doc, c, false)
			if err != nil {
				return err
			}
			if err := add(part); err != nil {
				return err
			}
		case c.is("choice") && !t.choice && pc.c.single:
			groups++
			if err := pc.nestedChoice(doc, c, t, groups, add); err != nil {
				return err
			}
		case c.is("any"):
			if t.choice || nParticles != 1 {
				return errors.New("xs:any is only supported alone in a sequence")
			}
			return pc.wildcard(doc, c, t)
		default:
			return fmt.Errorf("nested xs:%s is not supported", c.XMLName.Local)
		}
	}
	if len(t.parts) > 0 {
		t.content = contentElements
	}
	return nil
}

// nestedChoice flattens into the sequence of t a choice it holds, whose
// alternatives are sequences of elements or single elements (the choice
// between Denominazione, and Nome and Cognome, of FatturaPA): the elements
// are added optional, and each alternative is recorded in t.alts with the
// elements it requires. A choice that may be absent (minOccurs 0, as the
// branch address of a Facturae account) gets an empty alternative; a
// repeated choice is not supported.
func (pc *profileCompiler) nestedChoice(doc *schemaDoc, ch *xnode, t *gType, group int, add func(gPart) error) error {
	if err := ch.checkAttrs("minOccurs", "maxOccurs"); err != nil {
		return fmt.Errorf("nested xs:choice: %w", err)
	}
	minOcc, maxOcc, err := occurs(ch)
	if err != nil {
		return fmt.Errorf("nested xs:choice: %w", err)
	}
	if maxOcc != 1 {
		return errors.New("repeated nested xs:choice is not supported")
	}
	alts := 0
	for _, c := range ch.Children {
		var elems []*xnode
		switch {
		case c.is("annotation"):
			continue
		case c.is("element"):
			elems = []*xnode{c}
		case c.is("sequence"):
			if err := c.checkAttrs(); err != nil {
				return fmt.Errorf("sequence in a nested xs:choice: %w", err)
			}
			for _, e := range c.Children {
				switch {
				case e.is("annotation"):
				case e.is("element"):
					elems = append(elems, e)
				default:
					return fmt.Errorf("xs:%s in a sequence of a nested xs:choice is not supported", e.XMLName.Local)
				}
			}
		default:
			return fmt.Errorf("xs:%s in a nested xs:choice is not supported", c.XMLName.Local)
		}
		if len(elems) == 0 || len(elems) > 8 {
			return fmt.Errorf("an alternative of a nested xs:choice has %d elements (1 to 8 supported)", len(elems))
		}
		a := gAlt{group: group, first: len(t.parts), n: len(elems)}
		for i, e := range elems {
			part, err := pc.element(doc, e, false)
			if err != nil {
				return err
			}
			if part.min > 1 {
				return fmt.Errorf("element %s of a nested xs:choice with minOccurs %d is not supported", part.name, part.min)
			}
			if part.min > 0 {
				a.need |= 1 << i
			}
			part.min = 0
			if err := add(part); err != nil {
				return err
			}
		}
		t.alts = append(t.alts, a)
		alts++
	}
	if alts < 2 {
		return errors.New("a nested xs:choice needs two alternatives")
	}
	if minOcc == 0 {
		t.alts = append(t.alts, gAlt{group: group, first: len(t.parts)})
	}
	return nil
}

// wildcard compiles a sequence made of a single xs:any. Only the namespace
// constraint ##other is supported, and the content of the element is not
// checked: processContents lax is compiled as skip, and so is strict when
// the compiler has strictAsSkip.
func (pc *profileCompiler) wildcard(doc *schemaDoc, n *xnode, t *gType) error {
	if err := n.checkAttrs("namespace", "processContents", "minOccurs", "maxOccurs"); err != nil {
		return err
	}
	if ns := n.attr("namespace"); ns != "##other" {
		return fmt.Errorf("xs:any namespace=%q is not supported (only ##other)", ns)
	}
	if doc.tns == "" {
		return errors.New("xs:any namespace=\"##other\" in a schema without target namespace is not supported")
	}
	switch p := n.attr("processContents"); {
	case p == "lax", p == "skip":
	case pc.c.strictAsSkip && (p == "" || p == "strict"):
	default:
		return fmt.Errorf("xs:any processContents=%q is not supported (only lax and skip)", p)
	}
	for _, c := range n.Children {
		if !c.is("annotation") {
			return fmt.Errorf("unsupported xs:%s in xs:any", c.XMLName.Local)
		}
	}
	min, max, err := occurs(n)
	if err != nil {
		return err
	}
	t.content = contentAny
	t.min, t.max = min, max
	t.other = doc.tns
	pc.addNamespace(doc.tns)
	return nil
}

// simpleContent compiles an xs:simpleContent extension or restriction.
func (pc *profileCompiler) simpleContent(doc *schemaDoc, n *xnode, t *gType) error {
	t.content = contentSimple
	for _, c := range n.Children {
		switch {
		case c.is("annotation"):
		case c.is("extension"):
			base, err := pc.ref(doc, c.attr("base"))
			if err != nil {
				return err
			}
			if _, ok := pc.ps.complex[base]; ok {
				bt, err := pc.typeByName(base)
				if err != nil {
					return err
				}
				if bt.content != contentSimple {
					return fmt.Errorf("simple content extension of %s, which has no simple content", base)
				}
				t.simple = bt.simple
				t.attrs = append(t.attrs, bt.attrs...)
			} else {
				s, err := pc.simpleByName(base)
				if err != nil {
					return err
				}
				t.simple = s
			}
			for _, a := range c.Children {
				switch {
				case a.is("annotation"):
				case a.is("attribute"):
					ga, err := pc.attribute(doc, a)
					if err != nil {
						return err
					}
					if ga != nil {
						if err := addAttr(t, *ga); err != nil {
							return err
						}
					}
				default:
					return fmt.Errorf("unsupported xs:%s in extension", a.XMLName.Local)
				}
			}
		case c.is("restriction"):
			if err := pc.simpleRestriction(doc, c, t); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported simple content xs:%s", c.XMLName.Local)
		}
	}
	return nil
}

// simpleRestriction compiles an xs:simpleContent restriction of a complex
// type with simple content. Only the attributes may be restricted: an
// attribute of the base type is redeclared (with another type or use) or
// prohibited; the others are inherited.
func (pc *profileCompiler) simpleRestriction(doc *schemaDoc, n *xnode, t *gType) error {
	base, err := pc.ref(doc, n.attr("base"))
	if err != nil {
		return err
	}
	if _, ok := pc.ps.complex[base]; !ok {
		return fmt.Errorf("simple content restriction of %s, which is not a complex type", base)
	}
	bt, err := pc.typeByName(base)
	if err != nil {
		return err
	}
	if bt.content != contentSimple {
		return fmt.Errorf("simple content restriction of %s, which has no simple content", base)
	}
	t.simple = bt.simple
	t.attrs = append([]gAttr(nil), bt.attrs...)
	for _, a := range n.Children {
		switch {
		case a.is("annotation"):
		case a.is("attribute"):
			name := a.attr("name")
			i := -1
			for j := range t.attrs {
				if t.attrs[j].name == name {
					i = j
					break
				}
			}
			if i < 0 {
				return fmt.Errorf("restriction of %s declares attribute %q, which %s does not have", base, name, base)
			}
			ga, err := pc.attribute(doc, a)
			if err != nil {
				return err
			}
			if ga == nil {
				t.attrs = append(t.attrs[:i], t.attrs[i+1:]...)
			} else {
				t.attrs[i] = *ga
			}
		default:
			return fmt.Errorf("unsupported xs:%s in simple content restriction", a.XMLName.Local)
		}
	}
	return nil
}

// attribute compiles an attribute declaration; prohibited attributes give nil.
func (pc *profileCompiler) attribute(doc *schemaDoc, n *xnode) (*gAttr, error) {
	if n.attr("ref") != "" {
		return nil, errors.New("attribute references are not supported")
	}
	name := n.attr("name")
	if name == "" {
		return nil, errors.New("attribute without name")
	}
	// default does not change validation; fixed restricts the value.
	if err := n.checkAttrs("name", "type", "use", "default", "fixed"); err != nil {
		return nil, fmt.Errorf("attribute %s: %w", name, err)
	}
	a := &gAttr{name: name, simple: gSimple{name: "xs:anySimpleType", kind: kString}}
	switch n.attr("use") {
	case "required":
		a.required = true
	case "prohibited":
		return nil, nil
	case "", "optional":
	default:
		return nil, fmt.Errorf("attribute %s: unsupported use %q", name, n.attr("use"))
	}
	if typ := n.attr("type"); typ != "" {
		q, err := pc.ref(doc, typ)
		if err != nil {
			return nil, err
		}
		if a.simple, err = pc.simpleByName(q); err != nil {
			return nil, fmt.Errorf("attribute %s: %w", name, err)
		}
	}
	for _, c := range n.Children {
		switch {
		case c.is("annotation"):
		case c.is("simpleType"):
			s, err := pc.simpleType(doc, c, "type of @"+name)
			if err != nil {
				return nil, fmt.Errorf("attribute %s: %w", name, err)
			}
			a.simple = s
		default:
			return nil, fmt.Errorf("attribute %s: unsupported xs:%s", name, c.XMLName.Local)
		}
	}
	if fixed, ok := n.lookupAttr("fixed"); ok {
		if err := fix(&a.simple, fixed); err != nil {
			return nil, fmt.Errorf("attribute %s: %w", name, err)
		}
	}
	return a, nil
}

// fix restricts s to the value fixed by a fixed attribute declaration: an
// enumeration of that single value. Only string types are supported: they
// compare values by their lexical form, after white space collapsing for
// the collapsing types, which is what an enumeration checks.
func fix(s *gSimple, fixed string) error {
	if s.kind != kString {
		return fmt.Errorf("fixed value %q on %s, which is not a string type, is not supported", fixed, s.name)
	}
	f := &gFacets{min: -1, max: -1}
	if s.facets != nil {
		g := *s.facets
		g.pattern = append([]string(nil), g.pattern...)
		f = &g
	}
	if f.collapse {
		fixed = strings.Join(strings.Fields(fixed), " ")
	}
	if len(f.enum) > 0 && !slices.Contains(f.enum, fixed) {
		return fmt.Errorf("fixed value %q is not in the enumeration of %s", fixed, s.name)
	}
	f.enum = []string{fixed}
	s.facets = f
	return nil
}

// simpleByName resolves a named simple type or a built-in type.
func (pc *profileCompiler) simpleByName(q qname) (gSimple, error) {
	if q.space == nsXS {
		kind, ok := builtins[q.local]
		if !ok {
			return gSimple{}, fmt.Errorf("unsupported built-in type xs:%s", q.local)
		}
		f := &gFacets{min: -1, max: -1, collapse: collapsing[q.local]}
		if p, ok := builtinPatterns[q.local]; ok {
			f.pattern = []string{p}
		}
		return gSimple{name: "xs:" + q.local, kind: kind, facets: f}, nil
	}
	d, ok := pc.ps.simple[q]
	if !ok {
		if _, isComplex := pc.ps.complex[q]; isComplex {
			return gSimple{}, fmt.Errorf("%s is a complex type, a simple type is expected", q)
		}
		return gSimple{}, fmt.Errorf("unknown type %s", q)
	}
	return pc.simpleType(d.doc, d.node, q.String())
}

// simpleType compiles a simple type definition (a restriction of a built-in
// or named simple type).
func (pc *profileCompiler) simpleType(doc *schemaDoc, n *xnode, name string) (gSimple, error) {
	for _, c := range n.Children {
		switch {
		case c.is("annotation"):
		case c.is("restriction"):
			base, err := pc.ref(doc, c.attr("base"))
			if err != nil {
				return gSimple{}, err
			}
			bs, err := pc.simpleByName(base)
			if err != nil {
				return gSimple{}, err
			}
			f := &gFacets{min: -1, max: -1}
			if bs.facets != nil {
				g := *bs.facets
				g.enum = append([]string(nil), g.enum...)
				g.pattern = append([]string(nil), g.pattern...)
				f = &g
			}
			var enum []string
			for _, fc := range c.Children {
				v := fc.attr("value")
				switch {
				case fc.is("annotation"):
				case fc.is("enumeration"):
					enum = append(enum, v)
				case fc.is("pattern"):
					if strings.Contains(v, `\i`) || strings.Contains(v, `\c`) || strings.Contains(v, `\I`) || strings.Contains(v, `\C`) {
						return gSimple{}, fmt.Errorf("%s: pattern %q uses XML Schema name escapes", name, v)
					}
					re, err := goPattern(v)
					if err != nil {
						return gSimple{}, fmt.Errorf("%s: pattern %q: %w", name, v, err)
					}
					if _, err := regexp.Compile("^(?:" + re + ")$"); err != nil {
						return gSimple{}, fmt.Errorf("%s: pattern %q: %w", name, v, err)
					}
					f.pattern = append(f.pattern, re)
				case fc.is("minInclusive"), fc.is("maxInclusive"):
					if err := checkBound(bs.kind, v); err != nil {
						return gSimple{}, fmt.Errorf("%s: %s %q: %w", name, fc.XMLName.Local, v, err)
					}
					if fc.is("minInclusive") {
						f.minIncl = v
					} else {
						f.maxIncl = v
					}
				case fc.is("length"), fc.is("minLength"), fc.is("maxLength"):
					l, err := strconv.Atoi(v)
					if err != nil || l < 0 {
						return gSimple{}, fmt.Errorf("%s: bad %s %q", name, fc.XMLName.Local, v)
					}
					if !fc.is("maxLength") {
						f.min = l
					}
					if !fc.is("minLength") {
						f.max = l
					}
				case fc.is("whiteSpace"):
					f.collapse = v == "collapse"
				default:
					return gSimple{}, fmt.Errorf("%s: unsupported facet xs:%s", name, fc.XMLName.Local)
				}
			}
			if len(enum) > 0 {
				f.enum = enum
			}
			return gSimple{name: name, kind: bs.kind, facets: f}, nil
		default:
			return gSimple{}, fmt.Errorf("%s: unsupported xs:%s (only restrictions are supported)", name, c.XMLName.Local)
		}
	}
	return gSimple{}, fmt.Errorf("%s: simple type without restriction", name)
}

// blocks gives the character ranges of the Unicode blocks that the
// patterns name with \p{IsBlock}.
var blocks = map[string]string{
	"BasicLatin":        `\x{0}-\x{7F}`,
	"Latin-1Supplement": `\x{80}-\x{FF}`,
}

// goPattern rewrites an XML Schema pattern for package regexp: the block
// escapes \p{IsBlock} of the blocks listed in blocks become their ranges,
// inside a character class or as a class of their own.
func goPattern(p string) (string, error) {
	var b strings.Builder
	class := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\\' && strings.HasPrefix(p[i:], `\p{Is`):
			end := strings.IndexByte(p[i:], '}')
			if end < 0 {
				return "", errors.New("unterminated \\p{")
			}
			r, ok := blocks[p[i+len(`\p{Is`):i+end]]
			if !ok {
				return "", fmt.Errorf("unsupported block escape %s", p[i:i+end+1])
			}
			if class {
				b.WriteString(r)
			} else {
				b.WriteString("[" + r + "]")
			}
			i += end
		case c == '\\' && i+1 < len(p):
			b.WriteString(p[i : i+2])
			i++
		case c == '[' && !class:
			class = true
			b.WriteByte(c)
		case c == ']' && class:
			class = false
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// checkBound checks that an inclusive bound is a value of the kind of its
// type: decimal, integer or date.
func checkBound(kind, v string) error {
	var re string
	switch kind {
	case kDecimal:
		re = `[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)`
	case kInteger:
		re = `[+-]?[0-9]+`
	case kDate:
		re = `[0-9]{4}-[0-9]{2}-[0-9]{2}`
	default:
		return fmt.Errorf("bounds of a %s type are not supported", kind)
	}
	if !regexp.MustCompile("^" + re + "$").MatchString(v) {
		return fmt.Errorf("not a %s value", kind)
	}
	return nil
}

// setInfo describes a single schema set for emit.
type setInfo struct {
	prefix  string // prefix of the generated identifiers ("ubl": ublTypes)
	title   string // name of the schemas, in the first line
	source  string // comment lines on the source of the files, without the first "// "
	schemas string // what the version is the version of ("UBL schemas")
	roots   string // what the roots are
	// extensions says what the extension roots are, emitted when there are
	// some.
	extensions string
}

// emit writes the Go source of the tables: the Factur-X tables when info is
// nil, the tables of the single schema set info otherwise.
func (c *compiler) emit(version string, info *setInfo) ([]byte, error) {
	// Stable order: by name, then by the lowest profile using the type.
	order := append([]*gType(nil), c.types...)
	if info != nil {
		order = c.reachable()
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].name != order[j].name {
			return order[i].name < order[j].name
		}
		return lowBit(order[i].profiles) < lowBit(order[j].profiles)
	})
	newID := make(map[*gType]int, len(order))
	for i, t := range order {
		newID[t] = i
	}
	sort.Strings(c.namespaces)
	nsIndex := map[string]int{"": 0}
	for i, ns := range c.namespaces {
		nsIndex[ns] = i + 1
	}

	// Facets, deduplicated; index 0 means none.
	var facetList []*gFacets
	facetIndex := map[string]int{"": 0}
	facetOf := func(f *gFacets) int {
		k := f.key()
		if i, ok := facetIndex[k]; ok {
			return i
		}
		facetList = append(facetList, f)
		facetIndex[k] = len(facetList)
		return len(facetList)
	}

	// Names of the generated declarations.
	v := func(s string) string { return s }
	if info != nil {
		v = func(s string) string { return info.prefix + strings.ToUpper(s[:1]) + s[1:] }
	}

	var (
		b         bytes.Buffer
		parts     bytes.Buffer
		attrs     bytes.Buffer
		alts      bytes.Buffer
		nParts    int
		nAttrs    int
		nAlts     int
		maxOccurs = func(m int) string {
			if m < 0 {
				return "unbounded"
			}
			return strconv.Itoa(m)
		}
	)
	if info != nil {
		fmt.Fprintf(&b, "// Code generated by internal/schema/gen from the %s XML schemas; DO NOT EDIT.\n\n", info.title)
		b.WriteString("package schema\n\n")
		fmt.Fprintf(&b, "// %s\n//\n", info.source)
	} else {
		fmt.Fprintf(&b, "// Code generated by internal/schema/gen from the Factur-X %s XML schemas; DO NOT EDIT.\n\n", version)
		b.WriteString("package schema\n\n")
		b.WriteString("// Source schemas (SHA-256):\n//\n")
	}
	for p, files := range c.files {
		if info == nil {
			fmt.Fprintf(&b, "//\t%s:\n", profiles[p].name)
		}
		for _, f := range files {
			sum := sha256.Sum256(f.data)
			if info != nil {
				fmt.Fprintf(&b, "//\t%s %s\n", hex.EncodeToString(sum[:8]), f.path)
			} else {
				fmt.Fprintf(&b, "//\t\t%s %s\n", hex.EncodeToString(sum[:8]), path.Base(f.path))
			}
		}
	}
	if info != nil {
		fmt.Fprintf(&b, "\n// %s is the version of the %s compiled into the tables.\nconst %[1]s = %[3]q\n\n", v("schemaVersion"), info.schemas, version)
		fmt.Fprintf(&b, "// %s lists the element namespaces; particle.ns indexes it (0 = no namespace).\n", v("namespaces"))
	} else {
		fmt.Fprintf(&b, "\n// schemaVersion is the version of the Factur-X schemas compiled into the tables.\nconst schemaVersion = %q\n\n", version)
		b.WriteString("// namespaces lists the element namespaces; particle.ns indexes it (0 = no namespace).\n")
	}
	fmt.Fprintf(&b, "var %s = [...]string{\"\",\n", v("namespaces"))
	for _, ns := range c.namespaces {
		fmt.Fprintf(&b, "%q,\n", ns)
	}
	b.WriteString("}\n\n")

	if info != nil {
		fmt.Fprintf(&b, "// %s holds the content model of every element type.\n", v("types"))
	} else {
		b.WriteString("// types holds the content model of every element type, shared between\n// profiles when identical; profiles is a bit mask of the profiles using it.\n")
	}
	fmt.Fprintf(&b, "var %s = [...]ctype{\n", v("types"))
	for i, t := range order {
		firstPart, firstAttr := nParts, nAttrs
		for _, p := range t.parts {
			fmt.Fprintf(&parts, "{%q, %d, %d, %d, %s}, // %d\n", p.name, nsIndex[p.ns], newID[p.typ], p.min, maxOccurs(p.max), nParts)
			nParts++
		}
		for _, a := range t.attrs {
			fmt.Fprintf(&attrs, "{%q, %s, %d, %t},\n", a.name, a.simple.kind, facetOf(a.simple.facets), a.required)
			nAttrs++
		}
		fmt.Fprintf(&b, "{name: %q, content: %s", t.name, t.content)
		if t.choice {
			fmt.Fprintf(&b, ", choice: true, min: %d, max: %s", t.min, maxOccurs(t.max))
		}
		if t.content == contentAny {
			fmt.Fprintf(&b, ", min: %d, max: %s, other: %d", t.min, maxOccurs(t.max), nsIndex[t.other])
		}
		if t.content == contentSimple {
			fmt.Fprintf(&b, ", simple: %s", t.simple.kind)
			if fi := facetOf(t.simple.facets); fi != 0 {
				fmt.Fprintf(&b, ", facets: %d", fi)
			}
			if t.simple.kind != kString {
				fmt.Fprintf(&b, ", simpleName: %q", t.simple.name)
			}
			if t.def != "" {
				fmt.Fprintf(&b, ", def: %q", t.def)
			}
		}
		if len(t.parts) > 0 {
			fmt.Fprintf(&b, ", first: %d, n: %d", firstPart, len(t.parts))
		}
		if len(t.attrs) > 0 {
			fmt.Fprintf(&b, ", afirst: %d, an: %d", firstAttr, len(t.attrs))
		}
		if len(t.alts) > 0 {
			fmt.Fprintf(&b, ", altFirst: %d, altN: %d", nAlts, len(t.alts))
			for _, a := range t.alts {
				fmt.Fprintf(&alts, "{%d, %d, %d, %#02x}, // %s\n", a.group, a.first, a.n, a.need, t.name)
				nAlts++
			}
		}
		if info != nil {
			fmt.Fprintf(&b, "}, // %d\n", i)
		} else {
			fmt.Fprintf(&b, ", profiles: %#02x}, // %d\n", t.profiles, i)
		}
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// %s lists the child elements of the types: name, namespace, type, minOccurs, maxOccurs.\n", v("particles"))
	fmt.Fprintf(&b, "var %s = [...]particle{\n", v("particles"))
	b.Write(parts.Bytes())
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// %s lists the attributes of the types: name, kind, facets, required.\n", v("attributes"))
	fmt.Fprintf(&b, "var %s = [...]attribute{\n", v("attributes"))
	b.Write(attrs.Bytes())
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "// %s lists the facets of the restricted simple types; index 0 is none.\n", v("facetTable"))
	fmt.Fprintf(&b, "var %s = [...]facets{{},\n", v("facetTable"))
	for _, f := range facetList {
		b.WriteString("{")
		if len(f.enum) > 0 {
			fmt.Fprintf(&b, "enum: %#v, ", f.enum)
		}
		if len(f.pattern) > 0 {
			fmt.Fprintf(&b, "patterns: %#v, ", f.pattern)
		}
		fmt.Fprintf(&b, "minLen: %d, maxLen: %d, collapse: %t", f.min, f.max, f.collapse)
		if f.minIncl != "" {
			fmt.Fprintf(&b, ", minIncl: %q", f.minIncl)
		}
		if f.maxIncl != "" {
			fmt.Fprintf(&b, ", maxIncl: %q", f.maxIncl)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	if nAlts > 0 {
		fmt.Fprintf(&b, "// %s lists the alternatives of the choices flattened into sequences: group, first particle, number of particles, required particles.\n", v("alternatives"))
		fmt.Fprintf(&b, "var %s = [...]alternative{\n", v("alternatives"))
		b.Write(alts.Bytes())
		b.WriteString("}\n\n")
	}
	if info != nil {
		fmt.Fprintf(&b, "// %s holds %s.\n", v("roots"), info.roots)
		fmt.Fprintf(&b, "var %s = [...]particle{\n", v("roots"))
		for _, r := range c.roots {
			fmt.Fprintf(&b, "{%q, %d, %d, 1, 1},\n", r.name, nsIndex[r.ns], newID[r.typ])
		}
	} else {
		b.WriteString("// roots holds the root element of each profile, in profile order.\n")
		b.WriteString("var roots = [numProfiles]particle{\n")
		for p, r := range c.roots {
			fmt.Fprintf(&b, "{%q, %d, %d, 1, 1}, // %s\n", r.name, nsIndex[r.ns], newID[r.typ], profiles[p].name)
		}
	}
	b.WriteString("}\n")
	if info != nil && len(c.extensions) > 0 {
		fmt.Fprintf(&b, "\n// %s holds %s.\n", v("extensions"), info.extensions)
		fmt.Fprintf(&b, "var %s = [...]particle{\n", v("extensions"))
		for _, r := range c.extensions {
			fmt.Fprintf(&b, "{%q, %d, %d, 1, 1},\n", r.name, nsIndex[r.ns], newID[r.typ])
		}
		b.WriteString("}\n")
	}
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated code: %w\n%s", err, b.Bytes())
	}
	return src, nil
}

// reachable returns the types reachable from the roots: the base types of
// simple content derivations are compiled but not emitted.
func (c *compiler) reachable() []*gType {
	seen := map[*gType]bool{}
	var walk func(t *gType)
	walk = func(t *gType) {
		if seen[t] {
			return
		}
		seen[t] = true
		for _, p := range t.parts {
			walk(p.typ)
		}
	}
	for _, r := range c.roots {
		walk(r.typ)
	}
	for _, r := range c.extensions {
		walk(r.typ)
	}
	var out []*gType
	for _, t := range c.types {
		if seen[t] {
			out = append(out, t)
		}
	}
	return out
}

func lowBit(m uint8) int {
	for i := 0; i < 8; i++ {
		if m&(1<<i) != 0 {
			return i
		}
	}
	return 8
}
