package peppol

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

// Anchors of the Self-Billing invoice of base (the UBL invoice of base after
// selfBilling).
const (
	sbCustomization = "<cbc:CustomizationID>" + SelfBillingSpecificationID + "</cbc:CustomizationID>"
	sbProfileID     = "<cbc:ProfileID>" + ProcessSelfBilling + "</cbc:ProfileID>"
	sbTypeCode      = "<cbc:InvoiceTypeCode>389</cbc:InvoiceTypeCode>"
)

// selfBillingBase turns the UBL invoice of base into a self-billed invoice
// of Peppol BIS Self-Billing: its specification identifier, its business
// process and the type code 389.
func selfBillingBase() []edit {
	return one(
		rep(uCustomization, sbCustomization),
		rep(uProfileID, sbProfileID),
		rep(uTypeCode, sbTypeCode),
	)
}

// sbCreditNote turns the self-billed invoice into a credit note of type
// code.
func sbCreditNote(code string) []edit {
	return one(
		rep(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`, `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"`),
		rep("</Invoice>", "</CreditNote>"),
		rep(sbTypeCode, "<cbc:CreditNoteTypeCode>"+code+"</cbc:CreditNoteTypeCode>"),
		remove(uDueDate),
		repAll("cac:InvoiceLine>", "cac:CreditNoteLine>"),
		repAll("cbc:InvoicedQuantity", "cbc:CreditedQuantity"),
	)
}

// checkSelfBilling runs the Peppol rules on a UBL document whose profile is
// Peppol Self-Billing.
func checkSelfBilling(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: &rules.Doc{UBL: root, Profile: invoice.ProfilePeppolSelfBilling}, Report: r, Source: rules.SourcePeppol})
	return r
}

// checkSBFindings verifies that every finding is in SelfBillingCatalog, with
// its severity and its message, and located.
func checkSBFindings(t *testing.T, r *rules.Report) {
	t.Helper()
	for _, f := range r.Findings {
		info, ok := sbByID()[f.Rule]
		if !ok {
			t.Errorf("rule %s is not in SelfBillingCatalog", f.Rule)
			continue
		}
		if f.Severity != info.Severity || f.Source != rules.SourcePeppol || f.Path == "" ||
			!strings.HasPrefix(f.Message, "["+f.Rule+"]-"+info.Description) {
			t.Errorf("bad finding %+v", f)
		}
	}
}

// sbMutationCases returns the mutations of the self-billed invoice of base
// that the rules of Self-Billing tell from those of Billing.
func sbMutationCases() []mutationCase {
	return []mutationCase{
		{"base", nil, nil},
		{"self-billed debit note", one(rep(sbTypeCode, "<cbc:InvoiceTypeCode>527</cbc:InvoiceTypeCode>")), nil},
		{"self-billed credit note", sbCreditNote("261"), nil},

		// Specification identifier, business process and type codes.
		{"Billing specification identifier", one(rep(sbCustomization, uCustomization)), ids("PEPPOL-EN16931-R004")},
		{"specification identifier with more text", one(rep(sbCustomization, "<cbc:CustomizationID>"+SelfBillingSpecificationID+"#conformant#urn:example</cbc:CustomizationID>")), nil},
		{"specification identifier with ::", one(rep(sbCustomization, "<cbc:CustomizationID>"+SelfBillingSpecificationID+"::1</cbc:CustomizationID>")), ids("PEPPOL-EN16931-R004")},
		// The Billing process is not a process of Self-Billing: the profile
		// is unknown, and the type code is not checked.
		{"Billing process", one(rep(sbProfileID, uProfileID)), ids("PEPPOL-EN16931-R007")},
		{"no business process", one(remove(sbProfileID)), ids("PEPPOL-EN16931-R001", "PEPPOL-EN16931-R007", "PEPPOL-EN16931-R276")},
		{"French regulated process", one(rep(sbProfileID, "<cbc:ProfileID>urn:peppol:france:billing:regulated</cbc:ProfileID>")), nil},
		{"French process, commercial invoice", one(rep(sbProfileID, "<cbc:ProfileID>urn:peppol:france:billing:non-regulated</cbc:ProfileID>"),
			rep(sbTypeCode, "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>")), ids("PEPPOL-EN16931-P0100")},
		{"commercial invoice", one(rep(sbTypeCode, "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>")), ids("PEPPOL-EN16931-P0100")},
		{"type code with spaces", one(rep(sbTypeCode, "<cbc:InvoiceTypeCode> 389 </cbc:InvoiceTypeCode>")), nil},
		{"commercial credit note", sbCreditNote("381"), ids("PEPPOL-EN16931-P0101")},
		// In profile 02 (billing with response), Self-Billing checks no type
		// code, where Billing checks those of its profiles 01 and 02.
		{"billing with response, any type code", one(rep(sbProfileID, "<cbc:ProfileID>urn:peppol:bis:billing_with_response</cbc:ProfileID>"),
			rep(sbTypeCode, "<cbc:InvoiceTypeCode>999</cbc:InvoiceTypeCode>")), nil},
		{"billing with response, credit note", edits(sbCreditNote("999"),
			one(rep(sbProfileID, "<cbc:ProfileID>urn:peppol:bis:billing_with_response</cbc:ProfileID>"))), nil},

		// No national rule, and no PEPPOL-EN16931-P0112: a self-billed
		// invoice between two German parties, or between two Danish, Greek,
		// Icelandic or Dutch ones, gets no rule of their country.
		{"Germany", edits(germany, one(rep(uMeansCode, "<cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>"))), nil},
		{"Denmark giro", edits(denmark, one(rep(uMeansCode, "<cbc:PaymentMeansCode>50</cbc:PaymentMeansCode>"))), nil},
		{"Greece", greece, nil},
		{"Iceland transfer account", edits(iceland, one(rep(uMeansCode, "<cbc:PaymentMeansCode>42</cbc:PaymentMeansCode>"))), nil},
		{"Netherlands", one(sellerIn("NL"), after(uSellerLegal, `<cbc:CompanyID schemeID="0088">5012345000015</cbc:CompanyID>`)), nil},

		// The rules that Self-Billing shares with Billing.
		{"no buyer reference", one(remove(uBuyerRef)), ids("PEPPOL-EN16931-R003")},
		{"two notes", one(after(uNote, uNote)), ids("PEPPOL-EN16931-R002")},
		{"empty element", one(rep(uNote, "<cbc:Note> </cbc:Note>")), ids("PEPPOL-EN16931-R008")},
		{"Dutch VAT identifier", one(rep(uSellerVAT, "<cbc:CompanyID>NL123456789</cbc:CompanyID>")), ids("PEPPOL-COMMON-R056-2")},
		{"GLN check digit", one(rep(uSellerEndpoint, `<cbc:EndpointID schemeID="0088">5012345000016</cbc:EndpointID>`)), ids("PEPPOL-COMMON-R040")},
		{"Luxembourgish VAT identifier", one(rep(uSellerVAT, "<cbc:CompanyID>LU12345678</cbc:CompanyID>")), ids("PEPPOL-COMMON-R058")},
		{"no seller electronic address", one(remove(uSellerEndpoint)), ids("PEPPOL-EN16931-R020", "PEPPOL-EN16931-R229")},
		{"Slovakia", edits(slovakia, one(remove(uSellerStreet))), nil},
		// Self-Billing checks the schemes 0096 and 0198 after the Belgian VAT
		// identifier, where Billing checks them before (PEPPOL-COMMON-R052).
		{"Belgian VAT identifier with scheme 0096", one(rep(uSellerVAT, `<cbc:CompanyID schemeID="0096">BE0123456749</cbc:CompanyID>`)), nil},
		{"invalid Belgian VAT identifier with scheme 0198", one(rep(uSellerVAT, `<cbc:CompanyID schemeID="0198">BE0123456748</cbc:CompanyID>`)), ids("PEPPOL-COMMON-R062")},
	}
}

func TestSelfBillingMutations(t *testing.T) {
	doc := apply(t, load(t, ublBase), selfBillingBase())
	for _, c := range sbMutationCases() {
		t.Run(c.name, func(t *testing.T) {
			r := checkSelfBilling(t, []byte(apply(t, doc, c.edits)))
			got := r.Rules()
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("rules = %q, want %q\n%s", got, want, r)
			}
			checkSBFindings(t, r)
		})
	}
}

// TestSelfBillingProfile checks that the profile of the document chooses
// the rules: a self-billed invoice checked as a Billing one breaks
// PEPPOL-EN16931-R004, R007 and P0112, and a Billing invoice checked as a
// self-billed one breaks R004 and R007, with the messages of each
// schematron.
func TestSelfBillingProfile(t *testing.T) {
	sb := apply(t, load(t, ublBase), selfBillingBase())
	r := runUBL(t, []byte(sb))
	if got := r.Rules(); !slices.Equal(got, ids("PEPPOL-EN16931-P0112", "PEPPOL-EN16931-R004", "PEPPOL-EN16931-R007")) {
		t.Errorf("self-billed invoice under the Billing rules: %q\n%s", got, r)
	}
	checkUBLFindings(t, r)
	r = checkSelfBilling(t, []byte(load(t, ublBase)))
	if got := r.Rules(); !slices.Equal(got, ids("PEPPOL-EN16931-R004", "PEPPOL-EN16931-R007")) {
		t.Errorf("Billing invoice under the Self-Billing rules: %q\n%s", got, r)
	}
	checkSBFindings(t, r)
	for _, f := range r.Findings {
		if f.Rule == "PEPPOL-EN16931-R004" && !strings.Contains(f.Message, "MUST have the value '"+SelfBillingSpecificationID+"'") {
			t.Errorf("message of R004: %q", f.Message)
		}
	}
	// A CII invoice has no Self-Billing syntax: it gets the CII rules of
	// Billing whatever its profile.
	root, err := xmltree.Parse([]byte(load(t, base)))
	if err != nil {
		t.Fatal(err)
	}
	r = &rules.Report{}
	Check(&rules.Checker{Doc: rules.NewDoc(root, invoice.ProfilePeppolSelfBilling), Report: r, Source: rules.SourcePeppol})
	if len(r.Findings) > 0 {
		t.Errorf("CII invoice of profile PEPPOL-SELF-BILLING:\n%s", r)
	}
}

func TestSelfBillingCatalog(t *testing.T) {
	sev := map[rules.Severity]int{}
	for i := range SelfBillingCatalog {
		r := &SelfBillingCatalog[i]
		sev[r.Severity]++
		if sbByID()[r.ID] != r {
			t.Errorf("sbByID()[%s] is not the catalog entry", r.ID)
		}
		// Every rule of Self-Billing is a rule of Billing, with the same
		// severity and, but for R004, the same message.
		u := ublByID()[r.ID]
		switch {
		case u == nil:
			t.Errorf("%s is not in UBLCatalog", r.ID)
		case u.Severity != r.Severity:
			t.Errorf("%s: severity %v, %v in UBLCatalog", r.ID, r.Severity, u.Severity)
		case (u.Description == r.Description) == (r.ID == "PEPPOL-EN16931-R004"):
			t.Errorf("%s: description %q, %q in UBLCatalog", r.ID, r.Description, u.Description)
		}
	}
	if len(SelfBillingCatalog) != 185 || len(sbByID()) != 185 || sev[rules.Error] != 178 || sev[rules.Warning] != 7 {
		t.Errorf("%d assertions (%d errors, %d warnings), want 185 (178, 7)", len(SelfBillingCatalog), sev[rules.Error], sev[rules.Warning])
	}
	// The rules of Billing that Self-Billing does not have: the national
	// rules and PEPPOL-EN16931-P0112.
	for _, r := range UBLCatalog {
		national := !strings.HasPrefix(r.ID, "PEPPOL-")
		if (sbByID()[r.ID] == nil) != (national || r.ID == "PEPPOL-EN16931-P0112") {
			t.Errorf("%s: in SelfBillingCatalog = %v", r.ID, sbByID()[r.ID] != nil)
		}
	}
}

// TestSelfBillingEmpty checks that incomplete or foreign documents do not
// make the rules of Self-Billing panic, and give only findings of their
// catalog.
func TestSelfBillingEmpty(t *testing.T) {
	const ns = `xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"`
	for _, doc := range []string{
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" ` + ns + `/>`,
		`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2" ` + ns + `><cbc:CreditNoteTypeCode/></CreditNote>`,
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" ` + ns + `><cbc:ProfileID>` + ProcessSelfBilling + `</cbc:ProfileID><cbc:InvoiceTypeCode/>` +
			`<cac:AccountingSupplierParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>DK</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:Party></cac:AccountingSupplierParty></Invoice>`,
		`<x/>`,
	} {
		checkSBFindings(t, checkSelfBilling(t, []byte(doc)))
	}
}

// TestOfficialSelfBilling reads the artefacts of Peppol BIS Self-Billing
// 3.1.0 in the directory named by FX_PEPPOL_SB (conformance/fetch.sh): the
// assertions of PEPPOL-EN16931-UBL-SB.sch are those of SelfBillingCatalog,
// in the same order, with the same flags and messages, its rules of
// inferred cardinality fire on documents reduced to their context, and the
// official examples (BIS-SelfBilling-Examples.zip) pass the rules of
// Self-Billing.
func TestOfficialSelfBilling(t *testing.T) {
	dir := os.Getenv("FX_PEPPOL_SB")
	if dir == "" {
		t.Skip("FX_PEPPOL_SB is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "PEPPOL-EN16931-UBL-SB.sch"))
	if err != nil {
		t.Fatal(err)
	}
	sch, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var got []rules.RuleInfo
	sch.Walk(func(n *xmltree.Node) bool {
		if n.Name != "assert" || n.Space != "http://purl.oclc.org/dsdl/schematron" {
			return true
		}
		sev := rules.Error
		if n.Attr("flag") == "warning" {
			sev = rules.Warning
		}
		id := n.Attr("id")
		msg := strings.Join(strings.Fields(n.Text), " ")
		got = append(got, rules.RuleInfo{ID: id, Severity: sev, Description: strings.TrimPrefix(msg, "["+id+"]-")})
		return true
	})
	if !slices.Equal(got, SelfBillingCatalog) {
		for i := range max(len(got), len(SelfBillingCatalog)) {
			var a, b rules.RuleInfo
			if i < len(got) {
				a = got[i]
			}
			if i < len(SelfBillingCatalog) {
				b = SelfBillingCatalog[i]
			}
			if a != b {
				t.Errorf("assertion %d: schematron %+v, catalog %+v", i+1, a, b)
			}
		}
	}

	cardinality := officialCardinality(t, filepath.Join(dir, "PEPPOL-EN16931-UBL-SB.sch"), func(b []byte) *rules.Report { return checkSelfBilling(t, b) })
	if len(cardinality) != 120 {
		t.Errorf("%d rules of inferred cardinality, want 120", len(cardinality))
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
		if r := checkSelfBilling(t, b); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", f.Name, r)
		}
	}
	if n == 0 {
		t.Error("no example in BIS-SelfBilling-Examples.zip")
	}
	t.Logf("%d assertions, %d examples", len(got), n)
}
