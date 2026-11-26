package ubl

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
)

func readFile(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustParse(t testing.TB, data []byte) *invoice.Invoice {
	t.Helper()
	inv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return inv
}

func mustMarshal(t testing.TB, inv *invoice.Invoice, opts *MarshalOptions) []byte {
	t.Helper()
	out, err := Marshal(inv, opts)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return out
}

// stripExtensions clears every Extensions field of v (a pointer), recursively:
// Marshal does not write extensions back.
func stripExtensions(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			stripExtensions(v.Elem())
		}
	case reflect.Slice:
		if v.Type().Elem() == reflect.TypeOf(invoice.Extension{}) {
			v.SetZero()
			return
		}
		for i := 0; i < v.Len(); i++ {
			stripExtensions(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				stripExtensions(v.Field(i))
			}
		}
	}
}

func jsonOf(t testing.TB, v any) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sameInvoice fails when a and b differ, extensions excluded.
func sameInvoice(t testing.TB, name string, a, b *invoice.Invoice) {
	t.Helper()
	stripExtensions(reflect.ValueOf(a))
	stripExtensions(reflect.ValueOf(b))
	if ja, jb := jsonOf(t, a), jsonOf(t, b); ja != jb {
		la, lb := strings.Split(ja, "\n"), strings.Split(jb, "\n")
		for i := 0; i < len(la) && i < len(lb); i++ {
			if la[i] != lb[i] {
				t.Errorf("%s: round trip differs at line %d: %s != %s", name, i+1, strings.TrimSpace(la[i]), strings.TrimSpace(lb[i]))
				return
			}
		}
		t.Errorf("%s: round trip differs in length", name)
	}
}

// roundTrip writes inv as UBL, reads it back and compares, extensions
// excluded.
func roundTrip(t testing.TB, name string, inv *invoice.Invoice) {
	t.Helper()
	out := mustMarshal(t, inv, &MarshalOptions{Indent: true})
	back, err := Parse(out)
	if err != nil {
		t.Errorf("%s: reading back: %v", name, err)
		return
	}
	sameInvoice(t, name, inv, back)
}

func TestParseEinfach(t *testing.T) {
	inv := mustParse(t, readFile(t, "../testdata/EN16931_Einfach.ubl.xml"))
	if inv.Profile != invoice.ProfilePeppol {
		t.Errorf("profile %v, want PEPPOL", inv.Profile)
	}
	if inv.BusinessProcess != "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0" {
		t.Errorf("BT-23 %q", inv.BusinessProcess)
	}
	if inv.Number != "471102" || inv.TypeCode != "380" || inv.IssueDate.String() != "2018-03-05" || inv.Currency != "EUR" {
		t.Errorf("header %q %q %s %q", inv.Number, inv.TypeCode, inv.IssueDate, inv.Currency)
	}
	if inv.Seller.Name != "Lieferant GmbH" || inv.Seller.VATID != "DE123456789" || inv.Seller.TaxRegistrationID != "201/113/40209" {
		t.Errorf("seller %q %q %q", inv.Seller.Name, inv.Seller.VATID, inv.Seller.TaxRegistrationID)
	}
	if len(inv.Lines) != 2 || len(inv.VATBreakdown) != 2 {
		t.Fatalf("%d lines, %d VAT breakdowns", len(inv.Lines), len(inv.VATBreakdown))
	}
	if got := inv.Totals.DuePayable.String(); got != "529.87" {
		t.Errorf("BT-115 %s", got)
	}
	if l := inv.Lines[0]; l.Item.Name != "Trennblätter A4" || l.Quantity.String() != "20" || l.UnitCode != "H87" || l.NetAmount.String() != "198" {
		t.Errorf("line 1 %q %s %q %s", l.Item.Name, l.Quantity, l.UnitCode, l.NetAmount)
	}
	if u := inv.Unmodeled(); len(u) != 0 {
		t.Errorf("unmodeled data: %q", u)
	}
}

// TestSameAsCII reads the same invoice from its CII and its UBL version: the
// business terms that both documents carry are equal.
func TestSameAsCII(t *testing.T) {
	c, err := cii.Parse(readFile(t, "../testdata/EN16931_Einfach.cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	u := mustParse(t, readFile(t, "../testdata/EN16931_Einfach.ubl.xml"))
	eq := func(what string, a, b any) {
		t.Helper()
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: CII %v, UBL %v", what, a, b)
		}
	}
	dec := func(what string, a, b invoice.Decimal) {
		t.Helper()
		if a.Cmp(b) != 0 {
			t.Errorf("%s: CII %s, UBL %s", what, a, b)
		}
	}
	eq("BT-1", c.Number, u.Number)
	eq("BT-2", c.IssueDate, u.IssueDate)
	eq("BT-3", c.TypeCode, u.TypeCode)
	eq("BG-1", c.Notes, u.Notes)
	eq("BT-27", c.Seller.Name, u.Seller.Name)
	eq("BG-5", c.Seller.Address, u.Seller.Address)
	eq("BG-6", c.Seller.Contact, u.Seller.Contact)
	eq("BG-8", c.Buyer.Address, u.Buyer.Address)
	eq("BT-49", c.Buyer.ElectronicAddress, u.Buyer.ElectronicAddress)
	eq("BT-72", c.DeliveryDate, u.DeliveryDate)
	eq("BG-16", c.PaymentMeans, u.PaymentMeans)
	eq("BT-20", c.PaymentTerms, u.PaymentTerms)
	dec("BT-112", c.Totals.GrandTotal, u.Totals.GrandTotal)
	dec("BT-115", c.Totals.DuePayable, u.Totals.DuePayable)
	for i := range c.VATBreakdown {
		dec("BT-117", c.VATBreakdown[i].TaxAmount, u.VATBreakdown[i].TaxAmount)
		eq("BT-118", c.VATBreakdown[i].CategoryCode, u.VATBreakdown[i].CategoryCode)
	}
	for i := range c.Lines {
		eq("BT-126", c.Lines[i].ID, u.Lines[i].ID)
		eq("BT-153", c.Lines[i].Item.Name, u.Lines[i].Item.Name)
		dec("BT-131", c.Lines[i].NetAmount, u.Lines[i].NetAmount)
		dec("BT-146", c.Lines[i].NetPrice, u.Lines[i].NetPrice)
	}
}

func TestParseCreditNote(t *testing.T) {
	data := readFile(t, "../testdata/ubl-tc434-creditnote1.xml")
	inv := mustParse(t, data)
	if inv.TypeCode != "381" || !IsCreditNote(inv.TypeCode) {
		t.Errorf("BT-3 %q", inv.TypeCode)
	}
	if len(inv.Lines) != 1 || inv.Lines[0].Quantity.String() != "1.00" {
		t.Errorf("lines %d", len(inv.Lines))
	}
	out := mustMarshal(t, inv, nil)
	if !bytes.Contains(out, []byte("<CreditNote xmlns=\""+NamespaceCreditNote+"\"")) || !bytes.Contains(out, []byte("<cac:CreditNoteLine>")) ||
		!bytes.Contains(out, []byte("<cbc:CreditedQuantity")) {
		t.Errorf("not written as a credit note:\n%s", out)
	}
	roundTrip(t, "ubl-tc434-creditnote1.xml", inv)
}

// TestRoundTripTestdata reads the UBL and CII invoices of testdata and checks
// that writing them as UBL and reading them back gives the same invoice.
func TestRoundTripTestdata(t *testing.T) {
	for _, name := range []string{"EN16931_Einfach.ubl.xml", "XRECHNUNG_Einfach.ubl.xml", "ubl-tc434-creditnote1.xml"} {
		roundTrip(t, name, mustParse(t, readFile(t, "../testdata/"+name)))
	}
	for _, name := range []string{"EN16931_Einfach.cii.xml", "EN16931_Rabatte.cii.xml", "XRECHNUNG_Einfach.cii.xml"} {
		inv, err := cii.Parse(readFile(t, "../testdata/"+name))
		if err != nil {
			t.Fatal(err)
		}
		// Normalizations of the UBL binding: a gross price (BT-148) comes
		// with its price discount (BT-147), and a contact has one point of
		// contact (BT-41), the person rather than the department.
		for i := range inv.Lines {
			if l := &inv.Lines[i]; l.GrossPrice.IsSet() && !l.PriceDiscount.IsSet() {
				l.PriceDiscount = l.GrossPrice.Sub(l.NetPrice)
			}
		}
		for _, p := range []*invoice.Party{&inv.Seller, &inv.Buyer} {
			if p.Contact != nil && p.Contact.PersonName != "" {
				p.Contact.DepartmentName = ""
			}
		}
		roundTrip(t, name, inv)
	}
}

// ublFiles returns the UBL documents of the XML-Rechnung corpus (FX_CORPUS)
// and of the KoSIT test suite (FX_KOSIT).
func ublFiles(t *testing.T) []string {
	var files []string
	if dir := os.Getenv("FX_CORPUS"); dir != "" {
		f, _ := filepath.Glob(filepath.Join(dir, "XML-Rechnung", "UBL", "*.xml"))
		files = append(files, f...)
	}
	if dir := os.Getenv("FX_KOSIT"); dir != "" {
		f, _ := filepath.Glob(filepath.Join(dir, "src", "test", "*", "*", "*_ubl.xml"))
		files = append(files, f...)
	}
	if len(files) == 0 {
		t.Skip("FX_CORPUS and FX_KOSIT not set, or without UBL documents")
	}
	return files
}

// TestCorpus reads the UBL documents of the corpus and of the KoSIT test
// suite without any issue, and writes them back without change.
func TestCorpus(t *testing.T) {
	for _, f := range ublFiles(t) {
		inv, err := Parse(readFile(t, f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		roundTrip(t, filepath.Base(f), inv)
	}
}

// TestCreditNoteBinding checks the terms that a credit note binds elsewhere
// than an invoice: the payment due date (BT-9) and the project reference
// (BT-11).
func TestCreditNoteBinding(t *testing.T) {
	due, _ := invoice.ParseISODate("2024-06-30")
	for _, tc := range []struct {
		typeCode   string
		want, lack []string
	}{
		{"380", []string{"<cbc:DueDate>2024-06-30</cbc:DueDate>", "<cac:ProjectReference><cbc:ID>PRJ-1</cbc:ID></cac:ProjectReference>"},
			[]string{"PaymentDueDate", "<cbc:DocumentTypeCode>50</cbc:DocumentTypeCode>"}},
		{"381", []string{"<cbc:PaymentDueDate>2024-06-30</cbc:PaymentDueDate>", "<cbc:ID>PRJ-1</cbc:ID><cbc:DocumentTypeCode>50</cbc:DocumentTypeCode>"},
			[]string{"<cbc:DueDate>", "ProjectReference"}},
	} {
		inv := &invoice.Invoice{
			Number: "F1", TypeCode: tc.typeCode, Currency: "EUR", ProjectReference: "PRJ-1",
			PaymentTerms: []invoice.PaymentTerms{{DueDate: due}},
			PaymentMeans: []invoice.PaymentMeans{{TypeCode: "30"}},
		}
		out := string(mustMarshal(t, inv, nil))
		for _, s := range tc.want {
			if !strings.Contains(out, s) {
				t.Errorf("%s: %s not written:\n%s", tc.typeCode, s, out)
			}
		}
		for _, s := range tc.lack {
			if strings.Contains(out, s) {
				t.Errorf("%s: unexpected %s:\n%s", tc.typeCode, s, out)
			}
		}
		back := mustParse(t, []byte(out))
		if back.ProjectReference != "PRJ-1" || len(back.PaymentTerms) != 1 || back.PaymentTerms[0].DueDate != due {
			t.Errorf("%s: read back BT-11 %q, BT-9 %v", tc.typeCode, back.ProjectReference, back.PaymentTerms)
		}
		if len(back.AdditionalDocuments) != 0 {
			t.Errorf("%s: BT-11 read as a supporting document: %v", tc.typeCode, back.AdditionalDocuments)
		}
	}
}

// TestNotes checks the "#code#text" convention of the binding for the note
// subject code (BT-21).
func TestNotes(t *testing.T) {
	inv := &invoice.Invoice{Number: "F1", TypeCode: "380", Currency: "EUR", Notes: []invoice.Note{
		{SubjectCode: "AAI", Content: "Facture & avoir"},
		{Content: "Sans code"},
		{Content: "Remise #2 incluse"},
	}}
	out := string(mustMarshal(t, inv, nil))
	for _, s := range []string{"<cbc:Note>#AAI#Facture &amp; avoir</cbc:Note>", "<cbc:Note>Sans code</cbc:Note>", "<cbc:Note>Remise #2 incluse</cbc:Note>"} {
		if !strings.Contains(out, s) {
			t.Errorf("%s not written:\n%s", s, out)
		}
	}
	back := mustParse(t, []byte(out))
	if !reflect.DeepEqual(back.Notes, inv.Notes) {
		t.Errorf("notes read back %+v", back.Notes)
	}
}

// TestTaxPointDateCode checks the translation of the VAT point date codes
// (BT-8) between the model (CII codes) and UBL.
func TestTaxPointDateCode(t *testing.T) {
	for model, ubl := range map[string]string{"5": "3", "29": "35", "72": "432"} {
		inv := &invoice.Invoice{Number: "F1", TypeCode: "380", Currency: "EUR", TaxPointDateCode: model}
		out := string(mustMarshal(t, inv, nil))
		if want := "<cbc:DescriptionCode>" + ubl + "</cbc:DescriptionCode>"; !strings.Contains(out, want) {
			t.Errorf("BT-8 %s: %s not written:\n%s", model, want, out)
		}
		if back := mustParse(t, []byte(out)); back.TaxPointDateCode != model {
			t.Errorf("BT-8 %s read back as %q", model, back.TaxPointDateCode)
		}
	}
}

func TestIsCreditNote(t *testing.T) {
	for code, want := range map[string]bool{"381": true, " 396 ": true, "81": true, "380": false, "389": false, "": false, "381 380": false} {
		if got := IsCreditNote(code); got != want {
			t.Errorf("IsCreditNote(%q) = %v", code, got)
		}
	}
}

func TestMarshalStrict(t *testing.T) {
	inv := mustParse(t, readFile(t, "../testdata/EN16931_Einfach.ubl.xml"))
	if _, err := Marshal(inv, &MarshalOptions{Strict: true}); err != nil {
		t.Errorf("EN 16931 invoice: %v", err)
	}
	inv.TestIndicator = true
	inv.ProjectName = "Projet"
	inv.SalesAgent = &invoice.Party{Name: "Agent"}
	inv.Lines[0].Extensions = []invoice.Extension{{Path: "cac:Item/cbc:Keyword", Value: "mot"}}
	_, err := Marshal(inv, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) {
		t.Fatalf("got %v, want a *LossError", err)
	}
	got := strings.Join(le.Terms, " | ")
	for _, term := range []string{"EXT TestIndicator", "EXT ProjectName", "EXT SalesAgent", "EXT Lines[0]: cac:Item/cbc:Keyword"} {
		if !strings.Contains(got, term) {
			t.Errorf("%q not reported in %q", term, got)
		}
	}
	if _, err := Marshal(inv, nil); err != nil {
		t.Errorf("without Strict: %v", err)
	}
}

// TestGAccount writes the payment means identifiers and the payment terms
// of the Dutch G-account extension when the specification identifier written
// declares it, and reports them as lost otherwise.
func TestGAccount(t *testing.T) {
	inv := mustParse(t, readFile(t, "../testdata/EN16931_Einfach.ubl.xml"))
	inv.SpecificationID, inv.Profile = invoice.URNNLCIUS, invoice.ProfileNLCIUS
	inv.PaymentMeans = []invoice.PaymentMeans{
		{ID: "BENEFICIARY", TypeCode: "30", PayeeIBAN: "NL91ABNA0417164300"},
		{ID: "GACCOUNT", TypeCode: "30", PayeeIBAN: "NL20INGB0001234567"},
	}
	due := invoice.NewDate(2026, 12, 2)
	inv.PaymentTerms = []invoice.PaymentTerms{
		{Description: "30 dagen", DueDate: due, PaymentMeansID: "BENEFICIARY", PartialAmount: invoice.MustDecimal("90.00")},
		{PaymentMeansID: "GACCOUNT", PartialAmount: invoice.MustDecimal("10.00")},
	}

	_, err := Marshal(inv, &MarshalOptions{Strict: true})
	var le *LossError
	if !errors.As(err, &le) || strings.Join(le.Terms, ", ") != "EXT PaymentMeans.ID, EXT PaymentTerms.PaymentMeansID, EXT PaymentTerms.PartialAmount, EXT PaymentTerms" {
		t.Errorf("without the extension: got %v", err)
	}
	out := mustMarshal(t, inv, nil)
	for _, s := range []string{"<cbc:ID>BENEFICIARY</cbc:ID>", "<cbc:PaymentMeansID>", "<cbc:Amount "} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("without the extension: %s written", s)
		}
	}

	spec := invoice.URNNLCIUS + invoice.GAccountExtension
	out = mustMarshal(t, inv, &MarshalOptions{CustomizationID: spec, Strict: true})
	want := `<cac:PaymentMeans><cbc:ID>BENEFICIARY</cbc:ID><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>`
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("payment means identifier: want %s in\n%s", want, out)
	}
	want = `<cac:PaymentTerms><cbc:PaymentMeansID>BENEFICIARY</cbc:PaymentMeansID><cbc:Note>30 dagen</cbc:Note><cbc:Amount currencyID="EUR">90.00</cbc:Amount></cac:PaymentTerms>` +
		`<cac:PaymentTerms><cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID><cbc:Amount currencyID="EUR">10.00</cbc:Amount></cac:PaymentTerms>`
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("payment terms: want %s in\n%s", want, out)
	}
	back := mustParse(t, out)
	if back.SpecificationID != spec || !reflect.DeepEqual(back.PaymentMeans, inv.PaymentMeans) || !reflect.DeepEqual(back.PaymentTerms, inv.PaymentTerms) {
		t.Errorf("read back: %q\n%+v\n%+v", back.SpecificationID, back.PaymentMeans, back.PaymentTerms)
	}

	// An invoice that does not declare the extension keeps its elements as
	// extensions.
	gaccount := string(readFile(t, "../internal/rules/nl/testdata/gaccount.ubl.xml"))
	if !strings.Contains(gaccount, spec+"<") {
		t.Fatal("G-account identifier not found")
	}
	plain := mustParse(t, []byte(strings.Replace(gaccount, spec+"<", invoice.URNNLCIUS+"<", 1)))
	want = "PaymentMeans[0]: ID PaymentMeans[1]: ID PaymentTerms[0]: PaymentMeansID PaymentTerms[0]: Amount PaymentTerms[1]: PaymentMeansID PaymentTerms[1]: Amount"
	if got := strings.Join(plain.Unmodeled(), " "); got != want || plain.PaymentMeans[0].ID != "" || plain.PaymentTerms[0].PaymentMeansID != "" || plain.PaymentTerms[0].PartialAmount.IsSet() {
		t.Errorf("without the extension: unmodeled %q, want %q", got, want)
	}
	if got := mustParse(t, []byte(gaccount)).Unmodeled(); len(got) > 0 {
		t.Errorf("G-account invoice: unmodeled %q", got)
	}

	// The due date and the mandate reference of the second entry have no
	// place in UBL.
	inv.PaymentTerms[1].DueDate = due
	inv.PaymentTerms[1].DirectDebitMandateID = "M-1"
	_, err = Marshal(inv, &MarshalOptions{CustomizationID: spec, Strict: true})
	if !errors.As(err, &le) || strings.Join(le.Terms, ", ") != "EXT PaymentTerms.DueDate, EXT PaymentTerms.DirectDebitMandateID" {
		t.Errorf("second due date and mandate: got %v", err)
	}
}

func TestParseRejects(t *testing.T) {
	if _, err := Parse(readFile(t, "../testdata/EN16931_Einfach.cii.xml")); !errors.Is(err, ErrNotUBL) {
		t.Errorf("CII invoice: %v, want ErrNotUBL", err)
	}
	if _, err := Parse([]byte(`<Invoice xmlns="urn:example"/>`)); !errors.Is(err, ErrNotUBL) {
		t.Errorf("wrong namespace: %v, want ErrNotUBL", err)
	}
	if _, err := Parse([]byte(`<Invoice`)); err == nil || errors.Is(err, ErrNotUBL) {
		t.Errorf("malformed XML: %v", err)
	}
	root, err := xmltree.Parse(readFile(t, "../testdata/ubl-tc434-creditnote1.xml"))
	if err != nil || !IsUBL(root) {
		t.Errorf("IsUBL(credit note) = false, %v", err)
	}
	if inv, issues := ParseTree(&xmltree.Node{Name: "CrossIndustryInvoice"}); inv != nil || len(issues) != 1 {
		t.Errorf("ParseTree(CII) = %v, %v", inv, issues)
	}
}

func TestParseIssues(t *testing.T) {
	data := []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ID>F1</cbc:ID>
 <cbc:IssueDate>05/03/2024</cbc:IssueDate>
 <cac:LegalMonetaryTotal>
  <cbc:PayableAmount currencyID="EUR">12,50</cbc:PayableAmount>
 </cac:LegalMonetaryTotal>
</Invoice>`)
	inv, err := Parse(data)
	var ie *IssuesError
	if !errors.As(err, &ie) || inv == nil {
		t.Fatalf("got %v, %v", inv, err)
	}
	if len(ie.Issues) != 2 || ie.Issues[0].Line != 5 || ie.Issues[1].Path != "/Invoice/cac:LegalMonetaryTotal/cbc:PayableAmount" {
		t.Errorf("issues %v", ie.Issues)
	}
	if inv.Number != "F1" {
		t.Errorf("BT-1 %q", inv.Number)
	}
}

func TestMarshalNil(t *testing.T) {
	if _, err := Marshal(nil, nil); err == nil {
		t.Error("no error for a nil invoice")
	}
}

// TestCustomizationID checks the specification identifier (BT-24) written by
// default: that of the invoice for the profiles UBL carries, the EN 16931
// identifier for the Factur-X profiles that only exist in CII.
func TestCustomizationID(t *testing.T) {
	for _, c := range []struct {
		spec    string
		profile invoice.Profile
		want    string
	}{
		{invoice.URNPeppol, invoice.ProfilePeppol, invoice.URNPeppol},
		{"", invoice.ProfilePeppol, invoice.URNPeppol},
		{"", invoice.ProfileXRechnung, invoice.URNXRechnung},
		{"", invoice.ProfilePeppolSelfBilling, invoice.URNPeppolSB},
		{invoice.URNPeppolSB, invoice.ProfilePeppolSelfBilling, invoice.URNPeppolSB},
		{invoice.URNBasic, invoice.ProfileBasic, invoice.URNEN16931},
		{"", invoice.ProfileExtended, invoice.URNEN16931},
	} {
		inv := mustParse(t, readFile(t, "../testdata/EN16931_Einfach.ubl.xml"))
		inv.SpecificationID, inv.Profile, inv.BusinessProcess = c.spec, c.profile, ""
		out := mustMarshal(t, inv, nil)
		if want := "<cbc:CustomizationID>" + c.want + "</cbc:CustomizationID>"; !bytes.Contains(out, []byte(want)) {
			t.Errorf("%q %s: want %s", c.spec, c.profile, want)
		}
		// The business process (BT-23) defaults to the one of the
		// specification written: Peppol BIS Billing or Self-Billing.
		process := ""
		switch invoice.ProfileFromURN(c.want) {
		case invoice.ProfilePeppol, invoice.ProfileXRechnung:
			process = invoice.ProcessPeppolBilling
		case invoice.ProfilePeppolSelfBilling:
			process = invoice.ProcessPeppolSelfBilling
		}
		if process != "" && !bytes.Contains(out, []byte("<cbc:ProfileID>"+process+"</cbc:ProfileID>")) {
			t.Errorf("%q %s: want business process %s", c.spec, c.profile, process)
		}
	}
}
