package schema

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The FatturaPA test documents of package fatturapa.
const (
	fpr12 = "../../fatturapa/testdata/fpr12.xml"
	fpa12 = "../../fatturapa/testdata/fpa12.xml"
)

// checkFatturaPA parses data and runs CheckFatturaPA.
func checkFatturaPA(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	CheckFatturaPA(&rules.Checker{Report: r, Source: rules.SourceSyntax}, root)
	return r
}

func TestFatturaPATestdata(t *testing.T) {
	for _, name := range []string{fpr12, fpa12} {
		if r := checkFatturaPA(t, readFile(t, name)); len(r.Findings) > 0 {
			t.Errorf("%s: unexpected findings:\n%s", name, r)
		}
	}
	if FatturaPAVersion() != "1.2.4" {
		t.Errorf("FatturaPAVersion() = %q", FatturaPAVersion())
	}
}

// fpaSeller is the name of the seller in fpr12.
const fpaSeller = "<Denominazione>Esempio Reti S.r.l.</Denominazione>"

// fpaMutations are edits of fpr12 that check each kind of finding, and the
// constructs that the FatturaPA schema adds: the choice of Denominazione, or
// Nome and Cognome; the bounds; the Latin patterns; the default value of
// Nazione; the signature, not checked.
var fpaMutations = []struct {
	name, old, new string
	rule, msg      string // rule "" = valid
	signature      bool   // the content of ds:Signature is invalid, which CheckFatturaPA does not check
}{
	{name: "name and company", old: fpaSeller, new: fpaSeller + "<Nome>Mario</Nome>", rule: RuleUnexpected, msg: "Nome is not allowed in Anagrafica with Denominazione: they are alternatives"},
	{name: "given name only", old: fpaSeller, new: "<Nome>Mario</Nome>", rule: RuleMissing, msg: "Cognome is missing in Anagrafica (with Nome)"},
	{name: "natural person", old: fpaSeller, new: "<Nome>Mario</Nome><Cognome>Rossi</Cognome>"},
	{name: "no name", old: fpaSeller, rule: RuleMissing, msg: "Denominazione or Nome and Cognome is missing in Anagrafica"},
	{name: "name order", old: fpaSeller, new: "<Cognome>Rossi</Cognome><Nome>Mario</Nome>", rule: RuleOrder, msg: "Nome is not allowed at this position"},
	{name: "line number 0", old: "<NumeroLinea>1</NumeroLinea>", new: "<NumeroLinea>0</NumeroLinea>", rule: RuleType, msg: `NumeroLinea: value "0" is less than 1, the minimum of p:NumeroLineaType`},
	{name: "rate above 100", old: "<AliquotaIVA>0.00</AliquotaIVA>\n        <Natura>", new: "<AliquotaIVA>100.01</AliquotaIVA>\n        <Natura>", rule: RuleType, msg: "is greater than 100.00, the maximum of p:RateType"},
	{name: "date before 1970", old: "<Data>2026-11-20</Data>", new: "<Data>1969-12-31</Data>", rule: RuleType, msg: "is less than 1970-01-01"},
	{name: "tax regime RF21 (1.2.4)", old: "<RegimeFiscale>RF01</RegimeFiscale>", new: "<RegimeFiscale>RF21</RegimeFiscale>"},
	{name: "unknown tax regime", old: "<RegimeFiscale>RF01</RegimeFiscale>", new: "<RegimeFiscale>RF99</RegimeFiscale>", rule: RuleType, msg: "is not an allowed value"},
	{name: "non-Latin name", old: fpaSeller, new: "<Denominazione>Esempio Δ S.r.l.</Denominazione>", rule: RuleType, msg: "does not match the pattern of"},
	{name: "Latin-1 description", old: "<Descrizione>Spese di trasporto</Descrizione>", new: "<Descrizione>Spese di trasporto (è)</Descrizione>"},
	{name: "description too long", old: "<Descrizione>Spese di trasporto</Descrizione>", new: "<Descrizione>" + strings.Repeat("x", 1001) + "</Descrizione>", rule: RuleType, msg: "does not match the pattern of"},
	{name: "bad e-mail", old: "<Email>fatture@esempio-reti.it</Email>", new: "<Email>fatture@</Email>", rule: RuleType, msg: "does not match the pattern of"},
	{name: "empty Nazione (default IT)", old: "<Nazione>IT</Nazione>", new: "<Nazione></Nazione>"},
	{name: "lower case Nazione", old: "<Nazione>IT</Nazione>", new: "<Nazione>it</Nazione>", rule: RuleType, msg: "does not match the pattern of"},
	{name: "amount with one decimal", old: "<ImportoBollo>2.00</ImportoBollo>", new: "<ImportoBollo>2.0</ImportoBollo>", rule: RuleType, msg: "does not match the pattern of"},
	{name: "spaces around an amount", old: "<ImportoBollo>2.00</ImportoBollo>", new: "<ImportoBollo> 2.00 </ImportoBollo>"},
	{name: "signature", old: "</FatturaElettronicaBody>\n</p:FatturaElettronica>",
		new: `</FatturaElettronicaBody><ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="s1"><ds:SignedInfo/><ds:Unknown>x</ds:Unknown></ds:Signature></p:FatturaElettronica>`, signature: true},
	{name: "signature with a valid content", old: "</FatturaElettronicaBody>\n</p:FatturaElettronica>", new: `</FatturaElettronicaBody>` + fpaSignature + `</p:FatturaElettronica>`},
	{name: "element after the signature", old: "</FatturaElettronicaBody>\n</p:FatturaElettronica>",
		new: `</FatturaElettronicaBody><ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"/><Extra/></p:FatturaElettronica>`, rule: RuleUnexpected, msg: "Extra is not allowed in p:FatturaElettronica", signature: true},
	{name: "bad version", old: `versione="FPR12"`, new: `versione="FPX12"`, rule: RuleType, msg: "attribute versione of p:FatturaElettronica"},
	{name: "no version", old: ` versione="FPR12"`, rule: RuleAttributeMissing, msg: "attribute versione is required"},
	{name: "qualified children", old: `<p:FatturaElettronica xmlns:p="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"`,
		new: `<p:FatturaElettronica xmlns="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2" xmlns:p="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2"`, rule: RuleNamespace, msg: "FatturaElettronicaHeader is in namespace"},
	{name: "missing number", old: "<Numero>FT-2026/0042</Numero>", rule: RuleMissing, msg: "Numero is missing in DatiGeneraliDocumento"},
	{name: "unknown element", old: "<Causale>", new: "<Nota>x</Nota><Causale>", rule: RuleUnexpected, msg: "Nota is not allowed in DatiGeneraliDocumento"},
}

// fpaSignature is an XML signature valid against the schema of xmldsig.
const fpaSignature = `<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:SignedInfo>` +
	`<ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"/>` +
	`<ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/>` +
	`<ds:Reference URI=""><ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue>AAAA</ds:DigestValue></ds:Reference>` +
	`</ds:SignedInfo><ds:SignatureValue>AAAA</ds:SignatureValue></ds:Signature>`

func TestFatturaPAMutations(t *testing.T) {
	src := readFile(t, fpr12)
	for _, c := range fpaMutations {
		t.Run(c.name, func(t *testing.T) {
			r := checkFatturaPA(t, replace(t, src, c.old, c.new))
			if c.rule == "" {
				if len(r.Findings) > 0 {
					t.Errorf("unexpected findings:\n%s", r)
				}
				return
			}
			for _, f := range r.Findings {
				if f.Rule == c.rule && strings.Contains(f.Message, c.msg) {
					return
				}
			}
			t.Errorf("no %s finding with %q:\n%s", c.rule, c.msg, r)
		})
	}
}

func TestFatturaPARoot(t *testing.T) {
	r := checkFatturaPA(t, []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`))
	if len(r.Findings) != 1 || r.Findings[0].Rule != RuleRoot {
		t.Errorf("findings:\n%s", r)
	}
}

// TestFatturaPATables checks the invariants of the generated tables.
func TestFatturaPATables(t *testing.T) {
	for i := range fpaTypes {
		ty := &fpaTypes[i]
		if int(ty.first)+int(ty.n) > len(fpaParticles) || int(ty.afirst)+int(ty.an) > len(fpaAttributes) ||
			int(ty.altFirst)+int(ty.altN) > len(fpaAlternatives) || int(ty.facets) >= len(fpaFacetTable) {
			t.Errorf("type %s: index out of range", ty.name)
		}
		for _, a := range fpaAlternatives[ty.altFirst : ty.altFirst+ty.altN] {
			if int(a.first)+int(a.n) > int(ty.n) || a.need == 0 {
				t.Errorf("type %s: alternative %+v", ty.name, a)
			}
		}
	}
	var choices []string
	for i := range fpaTypes {
		if fpaTypes[i].altN > 0 {
			choices = append(choices, fpaTypes[i].name)
		}
	}
	sort.Strings(choices)
	if strings.Join(choices, " ") != "p:AnagraficaType p:RappresentanteFiscaleCessionarioType" {
		t.Errorf("types with a choice: %q", choices)
	}
}

func TestFatturaPAEnumeration(t *testing.T) {
	for name, want := range map[string]string{
		"SoggettoEmittenteType":        "CC TZ",
		"TipoRitenutaType":             "RT01 RT02 RT03 RT04 RT05 RT06",
		"Art73Type":                    "SI",
		"FatturaElettronicaHeaderType": "",
		"NoSuchType":                   "",
	} {
		if got := strings.Join(FatturaPAEnumeration(name), " "); got != want {
			t.Errorf("FatturaPAEnumeration(%q) = %q, want %q", name, got, want)
		}
	}
	td := FatturaPAEnumeration("TipoDocumentoType")
	if len(td) != 20 || td[0] != "TD01" || td[19] != "TD29" {
		t.Errorf("TipoDocumentoType: %q", td)
	}
	rf := FatturaPAEnumeration("RegimeFiscaleType")
	if len(rf) != 20 || rf[0] != "RF01" || rf[19] != "RF21" {
		t.Errorf("RegimeFiscaleType: %q", rf)
	}
	td[0] = "XX"
	if FatturaPAEnumeration("TipoDocumentoType")[0] != "TD01" {
		t.Error("FatturaPAEnumeration shares the tables")
	}
}

// TestFatturaPAExamples checks the official examples of FatturaPA 1.2
// (FX_FATTURAPA, see package fatturapa) against the schema. The example
// FPR02 is not valid: its ContattiTrasmittente follows PECDestinatario
// instead of preceding CodiceDestinatario, which xmllint reports too.
func TestFatturaPAExamples(t *testing.T) {
	dir := os.Getenv("FX_FATTURAPA")
	if dir == "" {
		t.Skip("FX_FATTURAPA is not set")
	}
	names, err := filepath.Glob(filepath.Join(dir, "IT01234567890_FP*.xml"))
	if err != nil || len(names) != 6 {
		t.Fatalf("%d examples in %s: %v", len(names), dir, err)
	}
	for _, name := range names {
		r := checkFatturaPA(t, readFile(t, name))
		if filepath.Base(name) == fpaInvalidExample {
			if len(r.Findings) != 1 || r.Findings[0].Rule != RuleOrder || !strings.Contains(r.Findings[0].Message, "ContattiTrasmittente") {
				t.Errorf("%s: want one %s finding on ContattiTrasmittente:\n%s", filepath.Base(name), RuleOrder, r)
			}
			continue
		}
		if len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", filepath.Base(name), r)
		}
	}
}

// fpaInvalidExample is the official example that the schema rejects.
const fpaInvalidExample = "IT01234567890_FPR02.xml"

// TestFatturaPAXmllint checks that xmllint (libxml2), given the FatturaPA
// schema 1.2.4 and the schema of xmldsig of the directory named by
// FX_FATTURAPA_XSD, accepts and rejects the same documents as CheckFatturaPA:
// the test documents, the mutations, and the official examples if
// FX_FATTURAPA is set. The mutations that make the content of ds:Signature
// invalid are left out: CheckFatturaPA does not check it.
func TestFatturaPAXmllint(t *testing.T) {
	dir := os.Getenv("FX_FATTURAPA_XSD")
	if dir == "" {
		t.Skip("FX_FATTURAPA_XSD is not set")
	}
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Fatal("FX_FATTURAPA_XSD is set but xmllint is missing")
	}
	tmp := t.TempDir()
	const dsig = "xmldsig-core-schema.xsd"
	xsd := string(readFile(t, filepath.Join(dir, "Schema_VFPA12_v1.2.4.xsd")))
	const remote = `schemaLocation="http://www.w3.org/TR/2002/REC-xmldsig-core-20020212/xmldsig-core-schema.xsd"`
	if !strings.Contains(xsd, remote) {
		t.Fatal("the FatturaPA schema does not import xmldsig from " + remote)
	}
	xsd = strings.Replace(xsd, remote, `schemaLocation="`+dsig+`"`, 1)
	if err := os.WriteFile(filepath.Join(tmp, "fatturapa.xsd"), []byte(xsd), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, dsig), readFile(t, filepath.Join(dir, dsig)), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := map[string][]byte{"fpr12": readFile(t, fpr12), "fpa12": readFile(t, fpa12)}
	for _, c := range fpaMutations {
		if !c.signature {
			docs[c.name] = replace(t, docs["fpr12"], c.old, c.new)
		}
	}
	if ex := os.Getenv("FX_FATTURAPA"); ex != "" {
		names, _ := filepath.Glob(filepath.Join(ex, "IT01234567890_FP*.xml"))
		for _, name := range names {
			docs[filepath.Base(name)] = readFile(t, name)
		}
	}
	var invalid int
	for name, data := range docs {
		path := filepath.Join(tmp, "doc.xml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(xmllint, "--noout", "--nonet", "--schema", filepath.Join(tmp, "fatturapa.xsd"), path)
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatal(err)
		}
		// xmllint exits with 3 when the document is not valid, and with
		// another code on any other error.
		if err != nil && exit.ExitCode() != 3 {
			t.Fatalf("%s: xmllint: %v\n%s", name, err, out)
		}
		libxml := err == nil
		r := checkFatturaPA(t, data)
		if ours := len(r.Findings) == 0; ours != libxml {
			t.Errorf("%s: valid for xmllint %t, for CheckFatturaPA %t\nxmllint: %s\nCheckFatturaPA:\n%s", name, libxml, ours, out, r)
		}
		if !libxml {
			invalid++
		}
	}
	if invalid == 0 || invalid == len(docs) {
		t.Errorf("%d documents of %d invalid", invalid, len(docs))
	}
}
