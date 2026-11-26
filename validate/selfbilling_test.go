package validate

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
)

// selfBilled writes the Slovak invoice of internal/rules/sk as a document
// of Peppol BIS Self-Billing of type code: a self-billed invoice (389), a
// self-billed debit note (527) or a self-billed credit note (261). The
// seller and the buyer keep their roles; ubl.Marshal writes the
// Self-Billing identifier (BT-24) and business process (BT-23).
func selfBilled(t *testing.T, typeCode string, edit func(*invoice.Invoice)) []byte {
	t.Helper()
	inv, err := ubl.Parse([]byte(readXML(t, slovakInvoices[0])))
	if err != nil {
		t.Fatal(err)
	}
	inv.Profile = invoice.ProfilePeppolSelfBilling
	inv.SpecificationID, inv.BusinessProcess = "", ""
	inv.TypeCode = typeCode
	if edit != nil {
		edit(inv)
	}
	out, err := ubl.Marshal(inv, &ubl.MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPeppolSelfBilling checks the self-billed documents that ubl.Marshal
// writes: they declare Peppol BIS Self-Billing, get its rules whatever the
// rule sets, and pass them, as they pass the rules of EN 16931 and the
// Slovak rules, which Auto adds for a seller in Slovakia.
func TestPeppolSelfBilling(t *testing.T) {
	for _, code := range []string{"389", "527", "261"} {
		doc := selfBilled(t, code, nil)
		root := "<Invoice "
		if code == "261" {
			root = "<CreditNote "
		}
		for _, want := range []string{root, "<cbc:CustomizationID>" + invoice.URNPeppolSB + "</cbc:CustomizationID>",
			"<cbc:ProfileID>" + invoice.ProcessPeppolSelfBilling + "</cbc:ProfileID>", "TypeCode>" + code + "</cbc:"} {
			if !strings.Contains(string(doc), want) {
				t.Errorf("%s: %q not written\n%s", code, want, doc)
			}
		}
		for _, opts := range []*Options{nil, {Auto: true}, {Rules: Default | Slovakia | Peppol}} {
			r := XML(doc, opts)
			if r.Profile != invoice.ProfilePeppolSelfBilling || len(r.Findings) > 0 {
				t.Errorf("%s (%+v): profile %s\n%s", code, opts, r.Profile, r)
			}
		}
	}

	// The Peppol rules of Self-Billing apply whatever the rule sets, and
	// the Slovak ones with Auto.
	for _, c := range []struct {
		name string
		doc  []byte
		opts *Options
		want []string
	}{
		// A commercial invoice under Self-Billing.
		{"type code 380", selfBilled(t, "380", nil), &Options{Rules: Schema}, []string{"PEPPOL-EN16931-P0100"}},
		{"credit note 381", selfBilled(t, "381", nil), nil, []string{"PEPPOL-EN16931-P0101"}},
		// The business process of Billing is not one of Self-Billing.
		{"Billing process", selfBilled(t, "389", func(inv *invoice.Invoice) { inv.BusinessProcess = invoice.ProcessPeppolBilling }), nil, []string{"PEPPOL-EN16931-R007"}},
		// The Slovak rules apply to a self-billed invoice of a Slovak seller
		// with Auto, as to any of its invoices, and not without.
		{"Slovak seller without legal form, Auto", selfBilled(t, "389", func(inv *invoice.Invoice) { inv.Seller.Description = "" }), &Options{Auto: true}, []string{"SK-13"}},
		{"Slovak seller without legal form", selfBilled(t, "389", func(inv *invoice.Invoice) { inv.Seller.Description = "" }), nil, nil},
	} {
		r := XML(c.doc, c.opts)
		if !slices.Equal(r.Rules(), c.want) {
			t.Errorf("%s: rules %q, want %q\n%s", c.name, r.Rules(), c.want, r)
		}
	}

	// The profile can be forced: a Billing invoice checked as a self-billed
	// one breaks the identifier and process rules of Self-Billing, a
	// self-billed invoice checked as a Billing one those of Billing, and
	// its type code 389 between two Slovak parties (PEPPOL-EN16931-P0112).
	billing := []byte(readXML(t, slovakInvoices[0]))
	if r := XML(billing, &Options{Profile: invoice.ProfilePeppolSelfBilling}); !slices.Equal(r.Rules(), []string{"PEPPOL-EN16931-R004", "PEPPOL-EN16931-R007"}) {
		t.Errorf("Billing invoice as PEPPOL-SELF-BILLING:\n%s", r)
	}
	if r := XML(selfBilled(t, "389", nil), &Options{Profile: invoice.ProfilePeppol}); !slices.Equal(r.Rules(), []string{"PEPPOL-EN16931-P0112", "PEPPOL-EN16931-R004", "PEPPOL-EN16931-R007"}) {
		t.Errorf("self-billed invoice as PEPPOL:\n%s", r)
	}
}

// TestPeppolSelfBillingExamples validates the official examples of Peppol
// BIS Self-Billing 3.0.2 (BIS-SelfBilling-Examples.zip in the directory
// named by FX_PEPPOL_SB): with the default rule sets, which add the Peppol
// rules of Self-Billing to these UBL invoices, none has an error.
func TestPeppolSelfBillingExamples(t *testing.T) {
	dir := os.Getenv("FX_PEPPOL_SB")
	if dir == "" {
		t.Skip("FX_PEPPOL_SB not set")
	}
	z, err := zip.OpenReader(filepath.Join(dir, "BIS-SelfBilling-Examples.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	n := 0
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		n++
		if r := XML(b, nil); r.Profile != invoice.ProfilePeppolSelfBilling || !r.Valid() {
			t.Errorf("%s: profile %s\n%s", f.Name, r.Profile, r)
		}
	}
	if n == 0 {
		t.Fatal("no example in BIS-SelfBilling-Examples.zip")
	}
	t.Logf("%d examples", n)
}
