package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung"
)

// mutated writes to a temporary file the invoice name with old replaced by
// new, and returns its path.
func mutated(t *testing.T, name, old, new string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(old)) {
		t.Fatalf("%s: %q not found", name, old)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(name))
	if err := os.WriteFile(path, bytes.ReplaceAll(b, []byte(old), []byte(new)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runValidate runs zerechnung validate -json with args and returns the exit
// code and the rules of the findings.
func runValidate(t *testing.T, args ...string) (int, map[string]bool) {
	t.Helper()
	var out bytes.Buffer
	code, err := validateCmd(append([]string{"-json"}, args...), &out)
	if err != nil {
		t.Fatalf("validate %s: %v", strings.Join(args, " "), err)
	}
	var report struct {
		Findings []struct{ Rule string }
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("validate %s: %v\n%s", strings.Join(args, " "), err, out.Bytes())
	}
	rules := map[string]bool{}
	for _, f := range report.Findings {
		rules[f.Rule] = true
	}
	return code, rules
}

// TestValidateDeclaredRules checks that the rule options of validate do not
// drop the rules an invoice declares in BT-24: BR-DE for XRECHNUNG, Peppol
// for a Peppol invoice, BR-NL for an NLCIUS invoice, BR-IT for a CIUS-IT
// invoice.
func TestValidateDeclaredRules(t *testing.T) {
	xrechnung := mutated(t, "../../testdata/XRECHNUNG_Einfach.cii.xml",
		"<ram:BuyerReference>04011000-12345-34</ram:BuyerReference>", "")
	peppol := mutated(t, "../../internal/rules/peppol/testdata/peppol.xml",
		"URIUniversalCommunication>", "X>")
	nlcius := mutated(t, "../../internal/rules/nl/testdata/nlcius.ubl.xml",
		"<cbc:BuyerReference>PO-4512</cbc:BuyerReference>", "")
	ciusit := mutated(t, "../../internal/rules/it/testdata/ciusit.xml",
		`<ram:URIID schemeID="0201">UFABCD</ram:URIID>`, `<ram:URIID schemeID="0201">ufabcd</ram:URIID>`)
	for _, flags := range [][]string{
		nil,
		{"-fr"},
		{"-de"},
		{"-choruspro"},
		{"-peppol"},
		{"-nl"},
		{"-it"},
		{"-fr", "-auto"},
	} {
		code, got := runValidate(t, append(flags, xrechnung)...)
		if code != 1 || !got["BR-DE-15"] {
			t.Errorf("validate %v on XRECHNUNG without BT-10: code %d, BR-DE-15 reported %v", flags, code, got["BR-DE-15"])
		}
		code, got = runValidate(t, append(flags, peppol)...)
		if code != 1 || !got["PEPPOL-EN16931-R010"] {
			t.Errorf("validate %v on Peppol invoice without electronic address: code %d, PEPPOL-EN16931-R010 reported %v", flags, code, got["PEPPOL-EN16931-R010"])
		}
		code, got = runValidate(t, append(flags, nlcius)...)
		if code != 1 || !got["BR-NL-2"] {
			t.Errorf("validate %v on NLCIUS invoice without BT-10: code %d, BR-NL-2 reported %v", flags, code, got["BR-NL-2"])
		}
		code, got = runValidate(t, append(flags, ciusit)...)
		if code != 1 || !got["BR-IT-200"] {
			t.Errorf("validate %v on CIUS-IT invoice with an IPA code in lower case: code %d, BR-IT-200 reported %v", flags, code, got["BR-IT-200"])
		}
	}
}

// TestValidateNLOption checks that -nl applies the Dutch rules to an invoice
// that does not declare NLCIUS (on a UBL invoice, the SI-UBL 2.0 version
// check reports it), and that they are not applied otherwise.
func TestValidateNLOption(t *testing.T) {
	const peppol = "../../internal/rules/peppol/testdata/peppol.ubl.xml"
	for _, c := range []struct {
		flags []string
		want  bool
	}{
		{nil, false},
		{[]string{"-auto"}, false},
		{[]string{"-peppol"}, false},
		{[]string{"-nl"}, true},
	} {
		_, got := runValidate(t, append(c.flags, peppol)...)
		if got["SI-V20-INV-R000"] != c.want {
			t.Errorf("validate %v on a Peppol invoice: SI-V20-INV-R000 reported %v, want %v", c.flags, got["SI-V20-INV-R000"], c.want)
		}
	}
}

// TestValidateITOption checks that -it applies the Italian rules to an
// invoice that does not declare the CIUS-IT, and that they are not applied
// otherwise.
func TestValidateITOption(t *testing.T) {
	const french = "../../internal/rules/fr/testdata/en16931.xml"
	for _, c := range []struct {
		flags []string
		want  bool
	}{
		{nil, false},
		{[]string{"-auto"}, false},
		{[]string{"-peppol"}, false},
		{[]string{"-it"}, true},
	} {
		_, got := runValidate(t, append(c.flags, french)...)
		if got["BR-IT-190"] != c.want {
			t.Errorf("validate %v on a Factur-X invoice: BR-IT-190 reported %v, want %v", c.flags, got["BR-IT-190"], c.want)
		}
	}
}

// TestValidatePeppolOption checks that -peppol applies the Peppol rules to an
// invoice that does not declare itself Peppol, and that they are not applied
// otherwise.
func TestValidatePeppolOption(t *testing.T) {
	const french = "../../internal/rules/fr/testdata/en16931.xml"
	for _, c := range []struct {
		flags []string
		want  bool
	}{
		{nil, false},
		{[]string{"-fr"}, false},
		{[]string{"-auto"}, false},
		{[]string{"-peppol"}, true},
		{[]string{"-fr", "-peppol"}, true},
	} {
		_, got := runValidate(t, append(c.flags, french)...)
		if got["PEPPOL-EN16931-R004"] != c.want {
			t.Errorf("validate %v on a Factur-X invoice: PEPPOL-EN16931-R004 reported %v, want %v", c.flags, got["PEPPOL-EN16931-R004"], c.want)
		}
	}
}

// TestEmbedRequirePDFA checks that embed -require-pdfa refuses an input PDF
// that does not declare PDF/A and writes nothing.
func TestEmbedRequirePDFA(t *testing.T) {
	const host, xml = "../../testdata/host.pdf", "../../testdata/EN16931_Einfach.cii.xml"
	out := filepath.Join(t.TempDir(), "out.pdf")
	err := embed([]string{"-xml", xml, "-require-pdfa", "-o", out, host})
	if !errors.Is(err, zerechnung.ErrNotPDFA) {
		t.Fatalf("embed -require-pdfa on a PDF without PDF/A declaration: %v, want ErrNotPDFA", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output written: %v", err)
	}
	if err := embed([]string{"-xml", xml, "-o", out, host}); err != nil {
		t.Fatalf("embed without -require-pdfa: %v", err)
	}
	if err := embed([]string{"-xml", xml, "-require-pdfa", "-o", out, out}); err != nil {
		t.Errorf("embed -require-pdfa on a PDF declared PDF/A-3: %v", err)
	}
}

// vectorPDF is a PDF of one page that draws an RGB rectangle, without
// PDF/A declaration nor output intent.
const vectorPDF = "%PDF-1.7\n" +
	"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
	"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R >>\nendobj\n" +
	"4 0 obj\n<< /Length 33 >>\nstream\n0.2 0.4 0.6 rg 10 10 100 100 re f\nendstream\nendobj\n" +
	"trailer\n<< /Root 1 0 R >>\n%%EOF\n"

func TestPDFA(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, []byte(vectorPDF), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pdf")
	if err := pdfaCmd([]string{"-o", out, in}); err != nil {
		t.Fatal(err)
	}
	invoice := filepath.Join(dir, "invoice.pdf")
	if err := embed([]string{"-xml", "../../testdata/EN16931_Einfach.cii.xml", "-require-pdfa", "-o", invoice, out}); err != nil {
		t.Fatalf("embed in the converted PDF: %v", err)
	}
	if code, rules := runValidate(t, invoice); code != 0 || len(rules) > 0 {
		t.Errorf("validate: exit %d, %v", code, rules)
	}

	// A refusal lists its reasons, one per line.
	err := pdfaCmd([]string{"-o", filepath.Join(dir, "host.pdf"), "../../testdata/host.pdf"})
	var ce *zerechnung.ConversionError
	if !errors.Is(err, zerechnung.ErrNotConvertible) || !errors.As(err, &ce) ||
		!strings.HasPrefix(err.Error(), "the PDF cannot be converted to PDF/A-3:\n  page 1 /Contents: the font \"Helvetica\"") {
		t.Errorf("pdfa on a PDF with a font not embedded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "host.pdf")); !os.IsNotExist(err) {
		t.Errorf("output written: %v", err)
	}
}

func TestEmbedConvertPDFA(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, []byte(vectorPDF), 0o644); err != nil {
		t.Fatal(err)
	}
	const xml = "../../testdata/EN16931_Einfach.cii.xml"
	out := filepath.Join(dir, "out.pdf")
	if err := embed([]string{"-xml", xml, "-convert-pdfa", "-require-pdfa", "-o", out, in}); err != nil {
		t.Fatal(err)
	}
	if code, rules := runValidate(t, out); code != 0 || len(rules) > 0 {
		t.Errorf("validate: exit %d, %v", code, rules)
	}
	err := embed([]string{"-xml", xml, "-convert-pdfa", "-o", out, "../../testdata/host.pdf"})
	if !errors.Is(err, zerechnung.ErrNotConvertible) || !strings.Contains(err.Error(), "\n  page 1 /Contents: the font") {
		t.Errorf("embed -convert-pdfa on a PDF with a font not embedded: %v", err)
	}
}

// TestJSONFatturaPA checks that json reads a FatturaPA document: one
// invoice as an object, a lotto of several invoices as an array.
func TestJSONFatturaPA(t *testing.T) {
	const name = "../../fatturapa/testdata/fpr12.xml"
	var out bytes.Buffer
	if err := jsonCmd([]string{name}, &out); err != nil {
		t.Fatal(err)
	}
	var inv struct {
		Number  string
		Profile string
		Italy   struct{ DocumentType string }
	}
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil || inv.Number != "FT-2026/0042" || inv.Profile != "CIUS-IT" ||
		inv.Italy.DocumentType != "TD24" {
		t.Errorf("json %s: %+v, %v", name, inv, err)
	}

	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	data := string(b)
	i, j := strings.Index(data, "  <FatturaElettronicaBody>"), strings.Index(data, "</p:FatturaElettronica>")
	lotto := filepath.Join(t.TempDir(), "lotto.xml")
	if err := os.WriteFile(lotto, []byte(data[:j]+data[i:j]+data[j:]), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := jsonCmd([]string{lotto}, &out); err != nil {
		t.Fatal(err)
	}
	var invs []struct{ Number string }
	if err := json.Unmarshal(out.Bytes(), &invs); err != nil || len(invs) != 2 || invs[1].Number != "FT-2026/0042" {
		t.Errorf("json of a lotto: %+v, %v", invs, err)
	}
}

// TestValidateFatturaPA checks that validate names the format of a FatturaPA
// document and reports the findings of its schema.
func TestValidateFatturaPA(t *testing.T) {
	const name = "../../fatturapa/testdata/fpr12.xml"
	var out bytes.Buffer
	code, err := validateCmd([]string{name}, &out)
	if err != nil || code != 0 || !strings.HasPrefix(out.String(), "Format: FatturaPA FPR12\n") {
		t.Errorf("validate %s: code %d, %v\n%s", name, code, err, out.String())
	}
	out.Reset()
	if code, err = validateCmd([]string{"-json", "-it", name}, &out); err != nil || code != 0 {
		t.Fatalf("validate -json -it %s: code %d, %v", name, code, err)
	}
	var report struct {
		Format string
		Valid  bool
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Format != "FatturaPA FPR12" || !report.Valid {
		t.Errorf("validate -json %s: %+v, %v\n%s", name, report, err, out.Bytes())
	}

	bad := mutated(t, name, "<RegimeFiscale>RF01</RegimeFiscale>", "<RegimeFiscale>RF99</RegimeFiscale>")
	code, got := runValidate(t, bad)
	if code != 1 || !got["XSD-TYPE"] || len(got) != 1 {
		t.Errorf("validate of a FatturaPA document with an unknown tax regime: code %d, rules %v", code, got)
	}
}

// TestFacturae reads a Facturae invoice with json, and checks it with
// validate: the schema of its version passes, FE-02 says that the EN 16931
// rules are not checked, and an invalid value is reported.
func TestFacturae(t *testing.T) {
	const name = "../../facturae/testdata/b2g.xml"
	var out bytes.Buffer
	if err := jsonCmd([]string{name}, &out); err != nil {
		t.Fatal(err)
	}
	var inv struct {
		Number string
		Spain  struct{ SeriesCode string }
		Buyer  struct {
			Spain struct{ Centres []struct{ Code string } }
		}
	}
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil || inv.Number != "A0042" || inv.Spain.SeriesCode != "A" ||
		len(inv.Buyer.Spain.Centres) != 3 {
		t.Errorf("json %s: %+v, %v", name, inv, err)
	}

	code, got := runValidate(t, name)
	if code != 0 || len(got) != 1 || !got["FE-02"] {
		t.Errorf("validate %s: code %d, findings %v", name, code, got)
	}
	out.Reset()
	if code, err := validateCmd([]string{name}, &out); err != nil || code != 0 || !strings.HasPrefix(out.String(), "Format: Facturae 3.2.2\n") {
		t.Errorf("validate %s: code %d, %v\n%s", name, code, err, out.String())
	}
	bad := mutated(t, name, "<InvoiceClass>OO</InvoiceClass>", "<InvoiceClass>XX</InvoiceClass>")
	if code, got := runValidate(t, bad); code != 1 || !got["XSD-TYPE"] || !got["FE-02"] {
		t.Errorf("validate of an invalid class: code %d, findings %v", code, got)
	}
	bad = mutated(t, name, "<IssueDate>2026-11-02</IssueDate>", "<IssueDate>02/11/2026</IssueDate>")
	if code, got := runValidate(t, bad); code != 1 || !got["FE-01"] {
		t.Errorf("validate of an unreadable date: code %d, findings %v", code, got)
	}
}
