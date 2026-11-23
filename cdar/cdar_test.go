package cdar

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

func readFile(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// examples returns the examples of statuses of testdata and, with FX_FNFE
// (a checkout of github.com/fnfempe/France_RFE), those of France_RFE.
func examples(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("testdata/*.xml")
	if err != nil || len(paths) < 12 {
		t.Fatalf("%d examples in testdata (%v)", len(paths), err)
	}
	if dir := os.Getenv("FX_FNFE"); dir != "" {
		more, err := filepath.Glob(filepath.Join(dir, "FNFE_RFE_INVOICE", "Z.example", "TEST", "CDAR", "*.xml"))
		if err != nil || len(more) < 50 {
			t.Fatalf("%d examples in %s (%v)", len(more), dir, err)
		}
		paths = append(paths, more...)
	}
	return paths
}

// TestRoundTrip checks that Parse reads every example without an issue,
// that Marshal writes a status valid against the CDAR D22B schema and the
// rules BR-FR-CDV, and that Parse reads back the same status.
func TestRoundTrip(t *testing.T) {
	for _, path := range examples(t) {
		name := filepath.Base(path)
		m, err := Parse(readFile(t, path))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(m.Acknowledgements) != 1 || len(m.Acknowledgements[0].Invoices) != 1 || m.Acknowledgements[0].Invoices[0].Code == "" {
			t.Errorf("%s: %+v", name, m.Acknowledgements)
		}
		for _, indent := range []bool{false, true} {
			out, err := Marshal(m, &MarshalOptions{Indent: indent})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if r := validate.XML(out, nil); len(r.Findings) > 0 {
				t.Errorf("%s (indent %v): findings on the message written:\n%s\n%s", name, indent, r, out)
			}
			back, err := Parse(out)
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			if !reflect.DeepEqual(back, m) {
				t.Errorf("%s (indent %v): read back\n%+v\nwant\n%+v", name, indent, back, m)
			}
		}
	}
}

// TestParse checks the values that Parse reads from a status 212 Encaissée
// and a status 207 En litige.
func TestParse(t *testing.T) {
	m, err := Parse(readFile(t, "testdata/UC1_F202500003_07-CDV-212_Encaissee.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if m.BusinessProcess != ProcessRegulated || m.Guideline != GuidelineInvoice || m.ID != "F202500003_212_20250701151000#380_20250701" ||
		!m.IssueTime.Equal(time.Date(2025, 8, 2, 10, 5, 0, 0, time.UTC)) {
		t.Errorf("context and header: %+v", m)
	}
	if m.Sender == nil || m.Sender.Role != RolePlatform || m.Issuer == nil || m.Issuer.Name != "VENDEUR" || m.Issuer.Role != RoleSeller ||
		len(m.Issuer.IDs) != 1 || m.Issuer.IDs[0] != (invoice.Identifier{ID: "100000009", Scheme: invoice.SchemeSIREN}) {
		t.Errorf("sender %+v, issuer %+v", m.Sender, m.Issuer)
	}
	if len(m.Recipients) != 2 || m.Recipients[0].Address != (invoice.Identifier{ID: "200000008", Scheme: invoice.SchemeFRDirectory}) ||
		m.Recipients[1].Role != RolePPF || m.Recipients[1].IDs[0] != (invoice.Identifier{ID: PPFID, Scheme: SchemePlatform}) {
		t.Errorf("recipients: %+v", m.Recipients)
	}
	a := m.Acknowledgements[0]
	st := a.Invoices[0]
	if a.TypeCode != PhaseProcessing || a.MultipleReferences || st.InvoiceID != "F202500003" || st.StatusCode != "47" || st.InvoiceType != "380" ||
		st.InvoiceDate != invoice.NewDate(2025, 7, 1) || st.Code != StatusCollected || st.Label != Label(StatusCollected) ||
		!st.ReceiptTime.Equal(time.Date(2025, 7, 1, 15, 10, 0, 0, time.UTC)) || st.Seller == nil || st.Seller.IDs[0].ID != "100000009" {
		t.Errorf("status: %+v", st)
	}
	if len(st.Details) != 1 || st.Details[0].Sequence != 1 || len(st.Details[0].Characteristics) != 1 {
		t.Fatalf("details: %+v", st.Details)
	}
	c := st.Details[0].Characteristics[0]
	if c.TypeCode != CharacteristicCollected || c.Changed || c.Amount.String() != "12000" || c.Currency != "EUR" || c.Percent.String() != "20.00" {
		t.Errorf("characteristic: %+v", c)
	}

	m, err = Parse(readFile(t, "testdata/UC4_F202500006_04-CDV-207_En_litige.xml"))
	if err != nil {
		t.Fatal(err)
	}
	d := m.Acknowledgements[0].Invoices[0].Details[0]
	if d.ReasonCode != "TX_TVA_ERR" || d.Reason != "Taux de TVA erroné" || d.ActionCode != "NIN" || d.Action != "Créer une Facture Rectificative" ||
		len(d.Characteristics) != 2 || d.Characteristics[0].ID != "BT-152" || !d.Characteristics[0].Changed || d.Characteristics[1].TypeCode != "DVA" ||
		!strings.HasSuffix(d.Characteristics[0].Location, "ram:RateApplicablePercent") || d.Characteristics[0].Name != "Taux TVA" {
		t.Errorf("detail: %+v", d)
	}
	if Label("999") != "" {
		t.Error("Label of an unknown code")
	}
}

// statusDoc is a CDAR message, whose elements tests put in place of BODY.
const statusDoc = `<rsm:CrossDomainAcknowledgementAndResponse xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossDomainAcknowledgementAndResponse:100"
 xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
 xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
 xmlns:qdt="urn:un:unece:uncefact:data:standard:QualifiedDataType:100">BODY</rsm:CrossDomainAcknowledgementAndResponse>`

// TestParseIssues checks the values that Parse cannot read and the elements
// it does not hold: each is an issue, and the rest of the message is read.
func TestParseIssues(t *testing.T) {
	for _, c := range []struct {
		name, body, issue string
	}{
		{"unknown element", `<rsm:Foo/>`, "element Foo is not read"},
		{"unknown child", `<rsm:ExchangedDocument><ram:TypeCode>1</ram:TypeCode></rsm:ExchangedDocument>`, "element TypeCode is not read"},
		{"attribute", `<rsm:ExchangedDocument><ram:ID schemeID="x">A</ram:ID></rsm:ExchangedDocument>`, "attribute schemeID is not read"},
		{"repeated", `<rsm:ExchangedDocument><ram:ID>A</ram:ID><ram:ID>B</ram:ID></rsm:ExchangedDocument>`, "repeated element ID"},
		{"date and time format", `<rsm:ExchangedDocument><ram:IssueDateTime><udt:DateTimeString format="102">20250701</udt:DateTimeString></ram:IssueDateTime></rsm:ExchangedDocument>`, `format "102" not read`},
		{"bad date and time", `<rsm:ExchangedDocument><ram:IssueDateTime><udt:DateTimeString format="204">20250701</udt:DateTimeString></ram:IssueDateTime></rsm:ExchangedDocument>`, "expects CCYYMMDDHHMMSS"},
		{"bad xs:dateTime", `<rsm:ExchangedDocument><ram:IssueDateTime><udt:DateTime>hier</udt:DateTime></ram:IssueDateTime></rsm:ExchangedDocument>`, `invalid date and time "hier"`},
		{"date format", `<rsm:AcknowledgementDocument><ram:ReferenceReferencedDocument><ram:FormattedIssueDateTime><qdt:DateTimeString format="204">20250701000000</qdt:DateTimeString></ram:FormattedIssueDateTime></ram:ReferenceReferencedDocument></rsm:AcknowledgementDocument>`, `date format "204" not read`},
		{"bad date", `<rsm:AcknowledgementDocument><ram:ReferenceReferencedDocument><ram:FormattedIssueDateTime><qdt:DateTimeString format="102">20250231</qdt:DateTimeString></ram:FormattedIssueDateTime></ram:ReferenceReferencedDocument></rsm:AcknowledgementDocument>`, "no such day"},
		{"indicator", `<rsm:AcknowledgementDocument><ram:MultipleReferencesIndicator><udt:Indicator>non</udt:Indicator></ram:MultipleReferencesIndicator></rsm:AcknowledgementDocument>`, `invalid indicator "non"`},
		{"sequence", `<rsm:AcknowledgementDocument><ram:ReferenceReferencedDocument><ram:SpecifiedDocumentStatus><ram:SequenceNumeric>0</ram:SequenceNumeric></ram:SpecifiedDocumentStatus></ram:ReferenceReferencedDocument></rsm:AcknowledgementDocument>`, `invalid sequence number "0"`},
		{"amount", `<rsm:AcknowledgementDocument><ram:ReferenceReferencedDocument><ram:SpecifiedDocumentStatus><ram:SpecifiedDocumentCharacteristic><ram:ValueAmount currencyID="EUR">12 000</ram:ValueAmount></ram:SpecifiedDocumentCharacteristic></ram:SpecifiedDocumentStatus></ram:ReferenceReferencedDocument></rsm:AcknowledgementDocument>`, `invalid number "12 000"`},
		{"percent attribute", `<rsm:AcknowledgementDocument><ram:ReferenceReferencedDocument><ram:SpecifiedDocumentStatus><ram:SpecifiedDocumentCharacteristic><ram:ValuePercent currencyID="EUR">20</ram:ValuePercent></ram:SpecifiedDocumentCharacteristic></ram:SpecifiedDocumentStatus></ram:ReferenceReferencedDocument></rsm:AcknowledgementDocument>`, "attribute currencyID is not read"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse([]byte(strings.Replace(statusDoc, "BODY", c.body, 1)))
			var ie *IssuesError
			if m == nil || !errors.As(err, &ie) || !strings.Contains(err.Error(), c.issue) || ie.Issues[0].Line == 0 || !strings.HasPrefix(ie.Issues[0].Path, "/rsm:CrossDomainAcknowledgementAndResponse/") {
				t.Errorf("Parse: %v, %v", m, err)
			}
		})
	}
	m, err := Parse([]byte(strings.Replace(statusDoc, "BODY", `<rsm:Foo/><rsm:Bar/><rsm:ExchangedDocument><ram:ID>A</ram:ID></rsm:ExchangedDocument>`, 1)))
	if err == nil || !strings.Contains(err.Error(), "2 unreadable values") || m.ID != "A" {
		t.Errorf("two issues: %v, %v", m, err)
	}
	if _, err := Parse([]byte(`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`)); !errors.Is(err, ErrNotCDAR) {
		t.Errorf("CII invoice: %v", err)
	}
	if _, err := Parse([]byte(`<rsm:CrossDomain`)); err == nil || errors.Is(err, ErrNotCDAR) {
		t.Errorf("malformed XML: %v", err)
	}
	if IsCDAR(nil) {
		t.Error("IsCDAR(nil)")
	}
	if _, err := Marshal(nil, nil); err == nil {
		t.Error("Marshal(nil)")
	}
}

// TestMarshal checks what Marshal writes of the fields that the examples of
// France_RFE leave empty: the test indicator, an xs:dateTime read as format
// 204, and the date of a characteristic.
func TestMarshal(t *testing.T) {
	m, err := Parse([]byte(strings.Replace(statusDoc, "BODY", `<rsm:ExchangedDocumentContext><ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>`+
		`<ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>`+GuidelineInvoice+`</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter></rsm:ExchangedDocumentContext>`+
		`<rsm:ExchangedDocument><ram:IssueDateTime><udt:DateTime>2026-09-01T10:00:00Z</udt:DateTime></ram:IssueDateTime></rsm:ExchangedDocument>`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	m.Acknowledgements = []Acknowledgement{{Invoices: []Status{{Details: []Detail{{Characteristics: []Characteristic{{Date: invoice.NewDate(2026, 9, 30)}}}}}}}}
	out, err := Marshal(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<ram:TestIndicator><udt:Indicator>true</udt:Indicator></ram:TestIndicator>`,
		`<udt:DateTimeString format="204">20260901100000</udt:DateTimeString>`,
		`<ram:ValueDateTime><udt:DateTimeString format="102">20260930</udt:DateTimeString></ram:ValueDateTime>`,
		`<ram:ValueChangedIndicator><udt:IndicatorString>false</udt:IndicatorString></ram:ValueChangedIndicator>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	back, err := Parse(out)
	if err != nil || !back.Test || !back.IssueTime.Equal(m.IssueTime) || back.Acknowledgements[0].Invoices[0].Details[0].Characteristics[0].Date != invoice.NewDate(2026, 9, 30) {
		t.Errorf("read back: %+v, %v", back, err)
	}
}
