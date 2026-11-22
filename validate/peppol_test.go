package validate

import (
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	ublsyntax "github.com/ritoon/zerechnung/ubl"
)

// TestPeppolBusinessProcess checks that an invoice written as Peppol without
// business process type (BT-23) gets the one of Peppol billing, which rule
// PEPPOL-EN16931-R001 requires, in CII and in UBL: the invoice written passes
// the Peppol rules and reads back with invoice.ProcessPeppolBilling.
func TestPeppolBusinessProcess(t *testing.T) {
	for _, c := range []struct {
		name    string
		file    string
		parse   func([]byte) (*invoice.Invoice, error)
		marshal func(*invoice.Invoice) ([]byte, error)
	}{
		{"CII", peppolInvoice, cii.Parse, func(inv *invoice.Invoice) ([]byte, error) { return cii.Marshal(inv, nil) }},
		{"UBL", peppolUBLInvoice, ublsyntax.Parse, func(inv *invoice.Invoice) ([]byte, error) { return ublsyntax.Marshal(inv, nil) }},
	} {
		inv, err := c.parse([]byte(readXML(t, c.file)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if inv.Profile != invoice.ProfilePeppol {
			t.Fatalf("%s: profile %s", c.name, inv.Profile)
		}
		inv.BusinessProcess = ""
		out, err := c.marshal(inv)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r := XML(out, &Options{Rules: Default | Peppol}); !r.Valid() || len(r.Findings) > 0 {
			t.Errorf("%s: findings on the invoice written:\n%s", c.name, r)
		}
		back, err := c.parse(out)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if back.BusinessProcess != invoice.ProcessPeppolBilling {
			t.Errorf("%s: BT-23 %q, want %q", c.name, back.BusinessProcess, invoice.ProcessPeppolBilling)
		}

		// An explicit BT-23 is written as is: an unknown process breaks rule
		// R007.
		inv.BusinessProcess = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0#unknown"
		if out, err = c.marshal(inv); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r := XML(out, &Options{Rules: Default | Peppol}); findings(r, "PEPPOL-EN16931-R007") == nil {
			t.Errorf("%s: an explicit BT-23 is not kept:\n%s", c.name, r)
		}
	}
}

// TestXRechnungBusinessProcess checks that an invoice written as XRechnung
// without business process type (BT-23) gets the one of Peppol billing too:
// the Peppol rules of the XRechnung schematron require one
// (PEPPOL-EN16931-R001). The invoice written does not break that rule under
// the default rules, which add the German rules for XRechnung, and reads back
// with invoice.ProcessPeppolBilling; an explicit BT-23 is kept.
func TestXRechnungBusinessProcess(t *testing.T) {
	for _, c := range []struct {
		name    string
		file    string
		parse   func([]byte) (*invoice.Invoice, error)
		marshal func(*invoice.Invoice) ([]byte, error)
	}{
		{"CII", xrechnungInvoice, cii.Parse, func(inv *invoice.Invoice) ([]byte, error) { return cii.Marshal(inv, nil) }},
		{"UBL", "../testdata/XRECHNUNG_Einfach.ubl.xml", ublsyntax.Parse, func(inv *invoice.Invoice) ([]byte, error) { return ublsyntax.Marshal(inv, nil) }},
	} {
		inv, err := c.parse([]byte(readXML(t, c.file)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if inv.Profile != invoice.ProfileXRechnung {
			t.Fatalf("%s: profile %s", c.name, inv.Profile)
		}
		inv.BusinessProcess = ""
		out, err := c.marshal(inv)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r := XML(out, nil); findings(r, "PEPPOL-EN16931-R001") != nil {
			t.Errorf("%s: PEPPOL-EN16931-R001 on the invoice written:\n%s", c.name, r)
		}
		back, err := c.parse(out)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if back.BusinessProcess != invoice.ProcessPeppolBilling {
			t.Errorf("%s: BT-23 %q, want %q", c.name, back.BusinessProcess, invoice.ProcessPeppolBilling)
		}

		inv.BusinessProcess = "urn:example:process"
		if out, err = c.marshal(inv); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if back, err = c.parse(out); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if back.BusinessProcess != "urn:example:process" {
			t.Errorf("%s: explicit BT-23 written as %q", c.name, back.BusinessProcess)
		}
	}
}
