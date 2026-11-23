package cdar

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// frenchInvoice is a French invoice of the EN 16931 profile, its seller and
// its buyer with a SIREN and an address in the directory.
const frenchInvoice = "../internal/rules/fr/testdata/en16931.xml"

func loadInvoice(t *testing.T) *invoice.Invoice {
	t.Helper()
	inv, err := cii.Parse(readFile(t, frenchInvoice))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

// checkBuilt writes m and checks that the message is valid against the
// CDAR D22B schema and the rules BR-FR-CDV, and that Parse reads it back.
func checkBuilt(t *testing.T, m *Message) {
	t.Helper()
	out, err := Marshal(m, &MarshalOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := validate.XML(out, nil); len(r.Findings) > 0 {
		t.Errorf("findings:\n%s\n%s", r, out)
	}
	back, err := Parse(out)
	if err != nil || !reflect.DeepEqual(back, m) {
		t.Errorf("read back %+v (%v), want %+v", back, err, m)
	}
}

var (
	created  = time.Date(2026, 10, 15, 9, 35, 0, 0, time.UTC)
	received = time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)
)

// TestDeposited checks the status 200 Déposée of the French invoice, sent to
// the seller and the PPF, and its version for the PPF alone, against the
// examples of France_RFE (UC1, 01-CDV-200).
func TestDeposited(t *testing.T) {
	inv := loadInvoice(t)
	m, err := Deposited(inv, &BuildOptions{Time: created, Received: received})
	if err != nil {
		t.Fatal(err)
	}
	checkBuilt(t, m)
	if m.ID != "FA-2026-00042_200_20261015093000#380_20261015" || m.Guideline != GuidelineInvoice || m.BusinessProcess != ProcessRegulated ||
		!m.IssueTime.Equal(created) || m.Sender.Role != RolePlatform || len(m.Sender.IDs) != 0 || m.Issuer.Role != RolePlatform {
		t.Errorf("header: %+v", m)
	}
	want := []Party{
		{IDs: []invoice.Identifier{{ID: "123456789", Scheme: "0002"}}, Name: "Exemple Informatique SAS", Role: RoleSeller, Address: invoice.Identifier{ID: "123456789_FACTURES", Scheme: "0225"}},
		{IDs: []invoice.Identifier{{ID: PPFID, Scheme: SchemePlatform}}, Name: "PPF", Role: RolePPF},
	}
	if !reflect.DeepEqual(m.Recipients, want) {
		t.Errorf("recipients: %+v", m.Recipients)
	}
	a := m.Acknowledgements[0]
	st := a.Invoices[0]
	if a.TypeCode != PhaseTransmission || !a.IssueTime.Equal(received) || st.InvoiceID != "FA-2026-00042" || st.StatusCode != "10" ||
		st.InvoiceType != "380" || st.InvoiceDate != inv.IssueDate || st.Code != StatusDeposited || st.Label != "Déposée" ||
		!st.ReceiptTime.Equal(received) || st.ReferenceType != "" || len(st.Details) != 0 ||
		!reflect.DeepEqual(st.Seller, &Party{IDs: []invoice.Identifier{{ID: "123456789", Scheme: "0002"}}}) {
		t.Errorf("status: %+v", st)
	}

	platform := Party{IDs: []invoice.Identifier{{ID: "0003", Scheme: SchemePlatform}}, Name: "PA-E Vendeur"}
	m, err = Deposited(inv, &BuildOptions{Time: created, Received: received, Platform: platform, PPF: true, ID: "FFE0614A_CC0238_1"})
	if err != nil {
		t.Fatal(err)
	}
	checkBuilt(t, m)
	if m.ID != "FFE0614A_CC0238_1" || m.Guideline != GuidelinePPF || m.BusinessProcess != "" || len(m.Recipients) != 1 || m.Recipients[0].Role != RolePPF ||
		m.Acknowledgements[0].Invoices[0].ReferenceType != GuidelinePPF ||
		!reflect.DeepEqual(m.Sender, &Party{IDs: platform.IDs, Name: platform.Name, Role: RolePlatform}) || !reflect.DeepEqual(m.Issuer, m.Sender) {
		t.Errorf("status for the PPF: %+v", m)
	}
	m.Sender.IDs[0].ID = "0009"
	if platform.IDs[0].ID != "0003" || m.Issuer.IDs[0].ID != "0003" {
		t.Error("the sender shares its identifiers with the options or the issuer")
	}

	now := time.Now()
	m, err = Deposited(inv, nil)
	if err != nil || m.IssueTime.Before(now.Truncate(time.Second)) || m.IssueTime.Nanosecond() != 0 || !m.Acknowledgements[0].IssueTime.Equal(m.IssueTime) {
		t.Errorf("default times: %+v, %v", m, err)
	}
	if _, err := Deposited(nil, nil); err == nil {
		t.Error("nil invoice")
	}
}

// TestRejected checks the status 213 Rejetée of the French invoice from its
// validation report: one detail per error, the warnings left out.
func TestRejected(t *testing.T) {
	inv := loadInvoice(t)
	rep := &validate.Report{}
	rep.Add(validate.Finding{Rule: "BR-CO-10", Severity: validate.Error, Message: "Sum of Invoice line net amount (BT-106) = Σ Invoice line net amount (BT-131)."})
	rep.Add(validate.Finding{Rule: "BR-FR-12_BT-49", Severity: validate.Warning, Message: "un avertissement"})
	rep.Add(validate.Finding{Rule: "BR-FR-05_BT-22_PMT", Severity: validate.Error, Message: "message <&>"})
	m, err := Rejected(inv, rep, &BuildOptions{Time: created, Received: received, ToBuyer: true, BusinessProcess: ProcessB2BInt})
	if err != nil {
		t.Fatal(err)
	}
	checkBuilt(t, m)
	st := m.Acknowledgements[0].Invoices[0]
	if st.Code != StatusRejected || st.Label != "Rejetée" || st.StatusCode != "8" || m.BusinessProcess != ProcessB2BInt ||
		m.ID != "FA-2026-00042_213_20261015093000#380_20261015" {
		t.Errorf("status: %+v", st)
	}
	wantDetails := []Detail{
		{ReasonCode: ReasonSemantic, Reason: "BR-CO-10 : Sum of Invoice line net amount (BT-106) = Σ Invoice line net amount (BT-131).", Sequence: 1},
		{ReasonCode: ReasonSemantic, Reason: "BR-FR-05_BT-22_PMT : message <&>", Sequence: 2},
	}
	if !reflect.DeepEqual(st.Details, wantDetails) {
		t.Errorf("details: %+v", st.Details)
	}
	if len(m.Recipients) != 3 || m.Recipients[1].Role != RoleBuyer || m.Recipients[1].Address.ID != "987654321_ACHATS" || m.Recipients[1].IDs[0].ID != "987654321" {
		t.Errorf("recipients: %+v", m.Recipients)
	}

	m, err = Rejected(inv, rep, &BuildOptions{PPF: true, Time: created})
	if err != nil || len(m.Recipients) != 1 || len(m.Acknowledgements[0].Invoices[0].Details) != 2 {
		t.Errorf("for the PPF: %+v, %v", m, err)
	}
	checkBuilt(t, m)

	warnings := &validate.Report{}
	warnings.Add(validate.Finding{Rule: "BR-FR-12_BT-49", Severity: validate.Warning})
	for _, r := range []*validate.Report{nil, {}, warnings} {
		if _, err := Rejected(inv, r, nil); err == nil || !strings.Contains(err.Error(), "no error") {
			t.Errorf("report %v: %v", r, err)
		}
	}
	if _, err := Rejected(nil, rep, nil); err == nil {
		t.Error("nil invoice")
	}
}

// TestLegalIDs checks the identifiers that a status gives of a party: its
// legal registration identifier, else its identifiers of scheme 0002, and
// none for a seller without SIREN, which BR-FR-CDV-13 then reports.
func TestLegalIDs(t *testing.T) {
	p := &invoice.Party{IDs: []invoice.Identifier{{ID: "12345678900017", Scheme: "0009"}, {ID: "123456789", Scheme: "0002"}}}
	if got := legalIDs(p); len(got) != 1 || got[0].ID != "123456789" {
		t.Errorf("legalIDs: %v", got)
	}
	inv := loadInvoice(t)
	inv.Seller.LegalRegistration = invoice.Identifier{}
	inv.Seller.IDs = nil
	m, err := Deposited(inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := validate.XML(out, nil)
	if !r.Has("BR-FR-CDV-13_MDT-129") || !r.Has("BR-FR-CDV-13_MDT-129-2") {
		t.Errorf("seller without SIREN:\n%s", r)
	}
}
