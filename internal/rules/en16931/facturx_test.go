package en16931

import (
	"sort"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// TestFacturXSchematron checks the CEN rules applied where the Factur-X
// schematron of the profile applies (rules.Doc.FacturX): BASIC, EXTENDED,
// and EN 16931 in a Factur-X PDF. The code lists (BR-CL) are left to
// package facturx, the CII syntax rules that the Factur-X schematrons leave
// out are not reported, and CII-SR-465 and CII-SR-466 are errors.
func TestFacturXSchematron(t *testing.T) {
	// A buyer contact with a person and a department (CII-SR-466), a unit
	// out of the CEN list (BR-CL-23), a currencyID on the sum of the line
	// net amounts (CII-DT-031).
	doc := edit(t, tBase,
		buyerAddr, buyerAddr+"<ram:DefinedTradeContact><ram:PersonName>A</ram:PersonName><ram:DepartmentName>B</ram:DepartmentName></ram:DefinedTradeContact>",
		`unitCode="C62"`, `unitCode="QQQ"`,
		"<ram:LineTotalAmount>100.00</ram:LineTotalAmount>\n<ram:TaxBasis", "<ram:LineTotalAmount currencyID=\"EUR\">100.00</ram:LineTotalAmount>\n<ram:TaxBasis")
	levels := func(urn string, hybrid bool) string {
		t.Helper()
		root, err := xmltree.Parse([]byte(strings.Replace(doc, invoice.URNEN16931+"<", urn+"<", 1)))
		if err != nil {
			t.Fatal(err)
		}
		d := rules.NewDoc(root, invoice.ProfileUnknown)
		d.Hybrid = hybrid
		r := &rules.Report{}
		Check(&rules.Checker{Doc: d, Report: r, Source: rules.SourceEN16931})
		var out []string
		for _, f := range r.Findings {
			out = append(out, f.Rule+":"+f.Severity.String())
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for _, tc := range []struct {
		name   string
		urn    string
		hybrid bool
		want   string
	}{
		{"EN 16931", invoice.URNEN16931, false, "BR-CL-23:error CII-DT-031:error CII-SR-466:warning"},
		{"EN 16931 in a PDF", invoice.URNEN16931, true, "CII-SR-466:error"},
		{"BASIC", invoice.URNBasic, false, "CII-SR-466:error"},
		{"EXTENDED", invoice.URNExtended, false, "CII-SR-466:error"},
		{"XRECHNUNG in a PDF", invoice.URNXRechnung, true, "BR-CL-23:warning CII-DT-031:error CII-SR-466:error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := levels(tc.urn, tc.hybrid); got != tc.want {
				t.Errorf("findings %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFacturXSyntax checks the CII syntax rules kept for BASIC and EN 16931
// in a PDF: those of the Factur-X schematrons, plus CII-DT-097.
func TestFacturXSyntax(t *testing.T) {
	for _, id := range []string{"CII-SR-463", "CII-SR-470", "CII-SR-473", "CII-DT-097"} {
		if !facturXSyntax(modeBasic, id) || !facturXSyntax(modeEN16931FX, id) {
			t.Errorf("%s left out", id)
		}
	}
	for _, id := range []string{"CII-SR-474", "CII-SR-476"} {
		if facturXSyntax(modeBasic, id) || !facturXSyntax(modeEN16931FX, id) {
			t.Errorf("%s: BASIC %t, EN 16931 %t", id, facturXSyntax(modeBasic, id), facturXSyntax(modeEN16931FX, id))
		}
	}
	for _, id := range []string{"CII-DT-031", "CII-SR-119", "CII-SR-471"} {
		if facturXSyntax(modeEN16931FX, id) {
			t.Errorf("%s kept", id)
		}
	}
	for id := range en16931Syntax {
		if _, ok := severity()[id]; !ok {
			t.Errorf("%s is not in Catalog", id)
		}
	}
}
