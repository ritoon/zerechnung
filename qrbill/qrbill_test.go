package qrbill

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/qrcode"
)

// none is the seven empty lines of the ultimate creditor, or of an absent
// ultimate debtor.
var none = []string{"", "", "", "", "", "", ""}

func content(parts ...[]string) string { return strings.Join(slices.Concat(parts...), "\n") }

var (
	muster      = []string{"S", "Max Muster & Söhne (sample company)", "Musterstrasse", "123", "8000", "Seldwyla", "CH"}
	simon       = []string{"S", "Simon Muster", "Musterstrasse", "1", "8000", "Seldwyla", "CH"}
	sarah       = []string{"S", "Sarah Beispiel", "Musterstrasse", "1", "8000", "Seldwyla", "CH"}
	musterAddr  = Address{"Max Muster & Söhne (sample company)", "Musterstrasse", "123", "8000", "Seldwyla", "CH"}
	simonAddr   = Address{"Simon Muster", "Musterstrasse", "1", "8000", "Seldwyla", "CH"}
	sarahAddr   = Address{"Sarah Beispiel", "Musterstrasse", "1", "8000", "Seldwyla", "CH"}
	konstanzDeb = Address{"Sarah Beispiel", "Musterstrasse", "1", "78462", "Konstanz", "DE"}
)

// annexA are the six examples of Annex A of version 2.4 of the guidelines
// (tables 17 to 22), as String writes them: lines separated by LF, the
// empty lines of status A left out.
var annexA = []struct {
	content string
	bill    Bill
}{
	{ // Example 1, table 17: QR-IBAN and QR reference.
		content(
			[]string{"SPC", "0200", "1", "CH6431961000004421557"}, muster, none, []string{"50.00", "CHF"}, simon,
			[]string{"QRR", "000008207791225857421286694", "Payment of travel", "EPD"}),
		Bill{Account: "CH6431961000004421557", Creditor: musterAddr, Amount: invoice.MustDecimal("50.00"), Currency: "CHF",
			Debtor: &simonAddr, ReferenceType: QRReference, Reference: "000008207791225857421286694", Message: "Payment of travel"},
	},
	{ // Example 2, table 18: with billing information and an alternative procedure.
		content(
			[]string{"SPC", "0200", "1", "CH4431999123000889012"}, muster, none, []string{"1949.75", "CHF"}, simon,
			[]string{"QRR", "210000000003139471430009017", "Order from 15.10.2020", "EPD",
				"//S1/10/1234/11/201021/30/102673386/32/7.7/40/0:30", "eBill/B/simon.muster@example.com"}),
		Bill{Account: "CH4431999123000889012", Creditor: musterAddr, Amount: invoice.MustDecimal("1949.75"), Currency: "CHF",
			Debtor: &simonAddr, ReferenceType: QRReference, Reference: "210000000003139471430009017", Message: "Order from 15.10.2020",
			BillInformation: "//S1/10/1234/11/201021/30/102673386/32/7.7/40/0:30", AlternativeSchemes: []string{"eBill/B/simon.muster@example.com"}},
	},
	{ // Example 3, table 19: without amount (a donation) and without debtor.
		content(
			[]string{"SPC", "0200", "1", "CH5204835012345671000"},
			[]string{"S", "Muster Stiftung (sample foundation)", "P.O. box", "", "3001", "Bern", "CH"}, none,
			[]string{"", "CHF"}, none, []string{"NON", "", "", "EPD"}),
		Bill{Account: "CH5204835012345671000", Creditor: Address{Name: "Muster Stiftung (sample foundation)", Street: "P.O. box",
			PostCode: "3001", Town: "Bern", Country: "CH"}, Currency: "CHF", ReferenceType: NoReference},
	},
	{ // Example 4, table 20: IBAN and Creditor Reference. The table prints
		// RF720191230100405JSH0438, whose check digits are wrong: those of
		// ISO 11649 (modulo 97-10, chapter 2.12.2) are 24 (TestAnnexA4).
		content(
			[]string{"SPC", "0200", "1", "CH5800791123000889012"},
			[]string{"S", "Muster Krankenkasse (sample health insurer)", "Musterstrasse", "12", "8000", "Seldwyla", "CH"}, none,
			[]string{"211.00", "CHF"}, sarah, []string{"SCOR", "RF240191230100405JSH0438", "", "EPD"}),
		Bill{Account: "CH5800791123000889012", Creditor: Address{"Muster Krankenkasse (sample health insurer)", "Musterstrasse", "12",
			"8000", "Seldwyla", "CH"}, Amount: invoice.MustDecimal("211.00"), Currency: "CHF", Debtor: &sarahAddr,
			ReferenceType: CreditorReference, Reference: "RF240191230100405JSH0438"},
	},
	{ // Example 5, table 21: creditor in Liechtenstein.
		content(
			[]string{"SPC", "0200", "1", "CH5800791123000889012"},
			[]string{"S", "Max Muster & Söhne (sample company)", "Musterstrasse", "123", "9490", "Vaduz", "LI"}, none,
			[]string{"199.95", "CHF"}, sarah, []string{"SCOR", "RF18539007547034", "", "EPD"}),
		Bill{Account: "CH5800791123000889012", Creditor: Address{"Max Muster & Söhne (sample company)", "Musterstrasse", "123",
			"9490", "Vaduz", "LI"}, Amount: invoice.MustDecimal("199.95"), Currency: "CHF", Debtor: &sarahAddr,
			ReferenceType: CreditorReference, Reference: "RF18539007547034"},
	},
	{ // Example 6, table 22: debtor in Germany.
		content(
			[]string{"SPC", "0200", "1", "CH5800791123000889012"}, muster, none, []string{"199.95", "CHF"},
			[]string{"S", "Sarah Beispiel", "Musterstrasse", "1", "78462", "Konstanz", "DE"},
			[]string{"SCOR", "RF18539007547034", "", "EPD"}),
		Bill{Account: "CH5800791123000889012", Creditor: musterAddr, Amount: invoice.MustDecimal("199.95"), Currency: "CHF",
			Debtor: &konstanzDeb, ReferenceType: CreditorReference, Reference: "RF18539007547034"},
	},
}

// TestAnnexA reproduces the examples of Annex A: String writes them, Parse
// reads them back without error, also as the annex prints them, CR + LF
// and the empty lines of status A included, and their symbol is of level M
// and version 25 at most.
func TestAnnexA(t *testing.T) {
	for i, ex := range annexA {
		if got := ex.bill.String(); got != ex.content {
			t.Errorf("example %d: String writes\n%s\nwant\n%s", i+1, got, ex.content)
		}
		if err := ex.bill.Check(); err != nil {
			t.Errorf("example %d: Check: %v", i+1, err)
		}
		annex := strings.ReplaceAll(ex.content, "\n", "\r\n")
		for n := strings.Count(annex, "\r\n") + 1; n < maxLines; n++ {
			annex += "\r\n"
		}
		for _, s := range []string{ex.content, annex, ex.content + "\n"} {
			b, err := Parse(s, Guidelines24)
			if err != nil {
				t.Errorf("example %d: Parse(%q): %v", i+1, s, err)
				continue
			}
			if b.AlternativeSchemes == nil && ex.bill.AlternativeSchemes == nil {
				b.AlternativeSchemes = nil
			}
			if !reflect.DeepEqual(b, &ex.bill) {
				t.Errorf("example %d: Parse reads\n%+v\nwant\n%+v", i+1, b, ex.bill)
			}
		}
		sym, err := ex.bill.Symbol()
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		if sym.Level() != qrcode.Medium || sym.Version() > 25 {
			t.Errorf("example %d: v%d-%s", i+1, sym.Version(), sym.Level())
		}
	}
}

// TestAnnexA4 reads example 4 of Annex A as table 20 prints it: Parse
// refuses its Creditor Reference, whose check digits are wrong, and only
// it.
func TestAnnexA4(t *testing.T) {
	_, err := Parse(set(3, 29, "RF720191230100405JSH0438"), Guidelines24)
	if got := errLines(err); !slices.Equal(got, []int{29}) {
		t.Errorf("errors on lines %v: %v", got, err)
	}
}

// TestReferences checks the references of the guidelines: the check digit
// of Annex B (figure 22), the Creditor References of examples 4 and 5, the
// QR-IBANs of examples 1 and 2.
func TestReferences(t *testing.T) {
	for in, want := range map[string]string{
		"21 00000 00003 13947 14300 0901": "210000000003139471430009017", // Annex B
		"00000820779122585742128669":      "000008207791225857421286694", // example 1
		"1234":                            "000000000000000000000012347",
	} {
		if got, err := NewQRReference(in); got != want || err != nil {
			t.Errorf("NewQRReference(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0000", "123456789012345678901234567", "12a4"} {
		if got, err := NewQRReference(in); err == nil {
			t.Errorf("NewQRReference(%q) = %q, want an error", in, got)
		}
	}
	for in, want := range map[string]string{
		"539007547034":         "RF18539007547034",         // examples 5 and 6
		"0191230100405jsh0438": "RF240191230100405JSH0438", // example 4, see TestAnnexA4
	} {
		if got, err := NewCreditorReference(in); got != want || err != nil {
			t.Errorf("NewCreditorReference(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1234567890123456789012", "AB-12"} {
		if got, err := NewCreditorReference(in); err == nil {
			t.Errorf("NewCreditorReference(%q) = %q, want an error", in, got)
		}
	}
	if !validCreditorReference("rf18539007547034") || validCreditorReference("RF19539007547034") {
		t.Error("validCreditorReference ignores the case and checks the check digits")
	}
	for iban, want := range map[string]bool{
		"CH6431961000004421557":  true,  // QR-IID 31961
		"CH4431999123000889012":  true,  // QR-IID 31999
		"CH5800791123000889012":  false, // IID 00791
		"CH4431999123000889013":  false, // wrong check digits
		"DE89370400440532013000": false,
	} {
		if got := IsQRIBAN(iban); got != want {
			t.Errorf("IsQRIBAN(%q) = %v", iban, got)
		}
	}
}

// lines returns the lines of example i of Annex A, padded to 34.
func lines(i int) []string {
	l := strings.Split(annexA[i].content, "\n")
	return append(l, make([]string, maxLines-len(l))...)
}

// set returns the content of example i with line n set to v.
func set(i int, kv ...any) string {
	l := lines(i)
	for j := 0; j < len(kv); j += 2 {
		l[kv[j].(int)-1] = kv[j+1].(string)
	}
	return strings.Join(l, "\n")
}

// errLines returns the lines of the *FieldError that err joins.
func errLines(err error) []int {
	var out []int
	var walk func(error)
	walk = func(e error) {
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, e := range j.Unwrap() {
				walk(e)
			}
			return
		}
		var fe *FieldError
		if errors.As(e, &fe) {
			out = append(out, fe.Line)
		}
	}
	if err != nil {
		walk(err)
	}
	return out
}

// TestInvalid has Parse report each content that breaks the guidelines on
// the line at fault, and on that line only.
func TestInvalid(t *testing.T) {
	long := strings.Repeat("x", 141)
	accents := strings.Repeat("é", 70)
	tests := []struct {
		name    string
		content string
		g       Guidelines
		line    int
	}{
		{"not SPC", set(1, 1, "SPX"), Guidelines24, 1},
		{"version", set(1, 2, "0201"), Guidelines24, 2},
		{"coding type", set(1, 3, "2"), Guidelines24, 3},
		{"IBAN check digits", set(3, 4, "CH5800791123000889013"), Guidelines24, 4},
		{"IBAN of Germany", set(3, 4, "DE89370400440532013000"), Guidelines24, 4},
		{"IBAN with spaces", set(3, 4, "CH58 0079 1123 0008 8901 2"), Guidelines24, 4},
		{"combined address", set(3, 5, "K"), Guidelines24, 5},
		{"no creditor name", set(3, 6, ""), Guidelines24, 6},
		{"creditor name too long", set(3, 6, strings.Repeat("n", 71)), Guidelines24, 6},
		{"building number too long", set(3, 8, strings.Repeat("1", 17)), Guidelines24, 8},
		{"no postal code", set(3, 9, ""), Guidelines24, 9},
		{"no town", set(3, 10, ""), Guidelines24, 10},
		{"country", set(3, 11, "XX"), Guidelines24, 11},
		{"ultimate creditor", set(3, 13, "Someone"), Guidelines24, 13},
		{"one decimal", set(3, 19, "211.0"), Guidelines24, 19},
		{"leading zero", set(3, 19, "0211.00"), Guidelines24, 19},
		{"decimal comma", set(3, 19, "211,00"), Guidelines24, 19},
		{"amount too large", set(3, 19, "1000000000.00"), Guidelines24, 19},
		{"zero amount", set(3, 19, "0.00"), Guidelines24, 19},
		{"notification without amount", set(3, 19, "", 30, "DO NOT USE FOR PAYMENT"), Guidelines24, 19},
		{"currency", set(3, 20, "USD"), Guidelines24, 20},
		{"debtor without town", set(3, 26, ""), Guidelines24, 26},
		{"debtor address type", set(3, 21, ""), Guidelines24, 21},
		{"QRR with IBAN", set(3, 28, "QRR", 29, "210000000003139471430009017"), Guidelines24, 28},
		{"SCOR with QR-IBAN", set(1, 28, "SCOR", 29, "RF18539007547034"), Guidelines24, 28},
		{"NON with QR-IBAN", set(1, 28, "NON", 29, ""), Guidelines24, 28},
		{"QR reference in EUR, version 2.4", set(1, 20, "EUR"), Guidelines24, 28},
		{"unknown reference type", set(3, 28, "ISO"), Guidelines24, 28},
		{"QR reference check digit", set(1, 29, "210000000003139471430009018"), Guidelines24, 29},
		{"QR reference of zeros", set(1, 29, "000000000000000000000000000"), Guidelines24, 29},
		{"Creditor Reference check digits", set(3, 29, "RF19539007547034"), Guidelines24, 29},
		{"reference with NON", set(2, 29, "RF18539007547034"), Guidelines24, 29},
		{"message too long", set(3, 30, long), Guidelines24, 30},
		{"character set", set(3, 30, "Merci 🙂"), Guidelines24, 30},
		{"trailer", set(3, 31, "EOD"), Guidelines24, 31},
		{"message and billing information", set(1, 30, strings.Repeat("m", 100), 32, "//S1/10/12345678901234567890123456789012345678901234"), Guidelines24, 32},
		{"billing information prefix", set(3, 32, "S1/10/1234"), Guidelines24, 32},
		{"S1 tags out of order", set(3, 32, "//S1/11/201021/10/1234"), Guidelines24, 32},
		{"S1 date", set(3, 32, "//S1/11/201321"), Guidelines24, 32},
		{"alternative procedure too long", set(3, 33, strings.Repeat("a", 101)), Guidelines24, 33},
		{"three alternative procedures", annexA[1].content + "\nA\nB", Guidelines24, 0},
		{"mixed line breaks", strings.Replace(annexA[3].content, "\n", "\r\n", 3), Guidelines24, 0},
		{"too few lines", strings.Join(lines(3)[:25], "\n"), Guidelines24, 0},
		{"too long", set(3, 6, accents, 7, accents, 22, accents, 23, accents, 10, accents[:70], 26, accents[:70],
			30, strings.Repeat("é", 140)), Guidelines24, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.content, tc.g)
			got := errLines(err)
			if len(got) == 0 || slices.ContainsFunc(got, func(l int) bool { return l != tc.line }) {
				t.Errorf("errors on lines %v, want line %d: %v", got, tc.line, err)
			}
		})
	}
}

// TestGuidelines23 checks the one rule that differs between the two
// versions: version 2.3 admits a QR-IBAN and a QR reference in EUR.
func TestGuidelines23(t *testing.T) {
	s := set(1, 20, "EUR")
	if _, err := Parse(s, Guidelines23); err != nil {
		t.Errorf("version 2.3: %v", err)
	}
	b, err := Parse(s, Guidelines24)
	if err == nil || !strings.Contains(err.Error(), "version 2.3") {
		t.Errorf("version 2.4: %v", err)
	}
	b.Guidelines = Guidelines23
	if err := b.Check(); err != nil {
		t.Errorf("Check with version 2.3: %v", err)
	}
	if Guidelines23.String() != "2.3" || Guidelines24.String() != "2.4" {
		t.Error("Guidelines.String")
	}
	// A Creditor Reference in EUR is admitted by both.
	if _, err := Parse(set(3, 20, "EUR"), Guidelines24); err != nil {
		t.Errorf("SCOR in EUR: %v", err)
	}
}

// TestNotification checks the QR-bill that only notifies (chapter 4.4).
func TestNotification(t *testing.T) {
	for _, lang := range []string{"de", "fr", "it", "en", "rm"} {
		msg, ok := Notification(lang)
		if !ok {
			t.Fatalf("no notification in %s", lang)
		}
		if _, err := Parse(set(3, 19, "0.00", 30, msg), Guidelines24); err != nil {
			t.Errorf("%s: %v", lang, err)
		}
		if _, err := Parse(set(3, 30, msg), Guidelines24); err == nil {
			t.Errorf("%s: a notification of 211.00 is accepted", lang)
		}
	}
	if _, ok := Notification("es"); ok {
		t.Error("Notification(es)")
	}
}

// TestAdditional checks that the errors of the lines of status A are told
// apart, as the guidelines forbid to reject a QR-bill for them.
func TestAdditional(t *testing.T) {
	_, err := Parse(set(3, 6, "", 32, "S1"), Guidelines24)
	var additional, other int
	for _, l := range errLines(err) {
		if (&FieldError{Line: l}).Additional() {
			additional++
		} else {
			other++
		}
	}
	if additional != 1 || other != 1 {
		t.Errorf("%d errors of status A, %d others: %v", additional, other, err)
	}
	if got := (&FieldError{Line: 6, Element: "Cdtr.Name", Msg: "m"}).Error(); got != "qrbill: line 6 (Cdtr.Name): m" {
		t.Errorf("Error() = %q", got)
	}
	if got := (&FieldError{Msg: "m"}).Error(); got != "qrbill: m" {
		t.Errorf("Error() = %q", got)
	}
}

// TestS1 reads the example of the syntax S1 in Annex D and writes it back.
func TestS1(t *testing.T) {
	const annexD = "//S1/10/10201409/11/190512/20/140.000-53/30/106017086/31/180508/32/7.7/40/2:10;0:30"
	s, err := ParseS1(annexD)
	if err != nil {
		t.Fatal(err)
	}
	want := &S1{InvoiceNumber: "10201409", InvoiceDate: invoice.MustDate("2019-05-12"), CustomerReference: "140.000-53",
		VATNumber: "106017086", VATDate: invoice.MustDate("2018-05-08"),
		VATRates:   []VATRate{{Rate: invoice.MustDecimal("7.7")}},
		Conditions: []Discount{{invoice.MustDecimal("2"), 10}, {invoice.MustDecimal("0"), 30}}}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("ParseS1 reads %+v", s)
	}
	if got := s.String(); got != annexD {
		t.Errorf("String writes %q", got)
	}
	for _, in := range []string{
		"//S1/31/181001190131/32/8:1000;2.5:51.80;7.7:250/33/7.7:48.37;2.5:12.4",
		`//S1/10/A\/B\\C`,
		"//S1",
	} {
		s, err := ParseS1(in)
		if err != nil {
			t.Errorf("ParseS1(%q): %v", in, err)
			continue
		}
		if got := s.String(); got != strings.ReplaceAll(in, "51.80", "51.8") {
			t.Errorf("ParseS1(%q).String() = %q", in, got)
		}
	}
	if s, _ := ParseS1(`//S1/10/A\/B\\C`); s.InvoiceNumber != `A/B\C` {
		t.Errorf("unescaped %q", s.InvoiceNumber)
	}
	for _, in := range []string{"//S2/10/1", "//S1/10", "//S1/10/1/10/2", "//S1/99/1", "//S1/1/1", "//S1/30/CHE106017086",
		"//S1/31/1805", "//S1/32/8:1000;7.7", "//S1/33/7.7", "//S1/40/2", "//S1/10/1\\", "//S1x"} {
		if _, err := ParseS1(in); err == nil {
			t.Errorf("ParseS1(%q) accepts it", in)
		}
	}
}

// swissInvoice is an invoice of a Swiss seller with two VAT rates.
func swissInvoice() *invoice.Invoice {
	return &invoice.Invoice{
		Number:    "2026-123",
		TypeCode:  "380",
		IssueDate: invoice.MustDate("2026-11-26"),
		Currency:  "CHF",
		Language:  "fr",
		Seller: invoice.Party{Name: "Robert Schneider AG", VATID: "CHE-123.456.788 MWST",
			Address: invoice.Address{Line1: "Rue du Lac 1268", PostCode: "2501", City: "Biel", CountryCode: "CH"}},
		Buyer: invoice.Party{Name: "Pia-Maria Rutschmann-Schnyder",
			Address: invoice.Address{Line1: "Grosse Marktgasse 28", PostCode: "9400", City: "Rorschach", CountryCode: "CH"}},
		InvoicingPeriod: invoice.Period{Start: invoice.MustDate("2026-11-01"), End: invoice.MustDate("2026-11-30")},
		PaymentMeans: []invoice.PaymentMeans{
			{TypeCode: "58", PayeeIBAN: "CH58 0079 1123 0008 8901 2"},
			{TypeCode: "58", PayeeIBAN: "CH44 3199 9123 0008 8901 2", PayeeAccountName: "Robert Schneider SA"},
		},
		PaymentTerms:          []invoice.PaymentTerms{{DueDate: invoice.MustDate("2026-12-26")}},
		RemittanceInformation: "21 00000 00003 13947 14300 09017",
		VATBreakdown: []invoice.VATBreakdown{
			{CategoryCode: "S", Rate: invoice.MustDecimal("8.1"), TaxableAmount: invoice.MustDecimal("1500.00"), TaxAmount: invoice.MustDecimal("121.50")},
			{CategoryCode: "AA", Rate: invoice.MustDecimal("2.60"), TaxableAmount: invoice.MustDecimal("300.00"), TaxAmount: invoice.MustDecimal("7.80")},
		},
		Totals: invoice.Totals{GrandTotal: invoice.MustDecimal("1929.30"), DuePayable: invoice.MustDecimal("1929.30")},
	}
}

// TestBuild builds the content of a Swiss invoice: the QR-IBAN for its QR
// reference, the name of the account, the buyer as debtor, the billing
// information S1.
func TestBuild(t *testing.T) {
	b, err := Build(swissInvoice(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := content(
		[]string{"SPC", "0200", "1", "CH4431999123000889012",
			"S", "Robert Schneider SA", "Rue du Lac 1268", "", "2501", "Biel", "CH"}, none,
		[]string{"1929.30", "CHF", "S", "Pia-Maria Rutschmann-Schnyder", "Grosse Marktgasse 28", "", "9400", "Rorschach", "CH",
			"QRR", "210000000003139471430009017", "", "EPD",
			"//S1/10/2026-123/11/261126/30/123456788/31/261101261130/32/8.1:1500;2.6:300/40/0:30"})
	if got := b.String(); got != want {
		t.Errorf("Build writes\n%s\nwant\n%s", got, want)
	}
	if _, err := Parse(b.String(), Guidelines24); err != nil {
		t.Errorf("Parse: %v", err)
	}
}

// TestBuildVariants covers the other paths of Build.
func TestBuildVariants(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*invoice.Invoice, *Data)
		check func(*testing.T, *Bill)
		line  int // line of the error, -1 for none
	}{
		{"creditor reference", func(inv *invoice.Invoice, _ *Data) {
			inv.RemittanceInformation = "RF18 5390 0754 7034"
		}, func(t *testing.T, b *Bill) {
			if b.Account != "CH5800791123000889012" || b.ReferenceType != CreditorReference || b.Reference != "RF18539007547034" {
				t.Errorf("%s %s %s", b.Account, b.ReferenceType, b.Reference)
			}
		}, -1},
		{"message", func(inv *invoice.Invoice, _ *Data) {
			inv.RemittanceInformation = "Facture 2026-123"
			inv.Seller.VATID = ""
			inv.VATBreakdown = inv.VATBreakdown[:1]
			inv.InvoicingPeriod = invoice.Period{}
			inv.DeliveryDate = invoice.MustDate("2026-11-20")
		}, func(t *testing.T, b *Bill) {
			if b.ReferenceType != NoReference || b.Message != "Facture 2026-123" ||
				b.BillInformation != "//S1/10/2026-123/11/261126/31/261120/32/8.1/40/0:30" {
				t.Errorf("%s %q %q", b.ReferenceType, b.Message, b.BillInformation)
			}
		}, -1},
		{"data", func(_ *invoice.Invoice, d *Data) {
			*d = Data{Account: "CH5204835012345671000", Reference: "RF240191230100405JSH0438", Message: "Merci",
				NoBillInformation: true, AlternativeSchemes: []string{"eBill/B/pia@example.com"}}
		}, func(t *testing.T, b *Bill) {
			if b.Account != "CH5204835012345671000" || b.Reference != "RF240191230100405JSH0438" || b.Message != "Merci" ||
				b.BillInformation != "" || len(b.AlternativeSchemes) != 1 {
				t.Errorf("%+v", b)
			}
		}, -1},
		{"payee, no debtor", func(inv *invoice.Invoice, _ *Data) {
			inv.PaymentMeans[1].PayeeAccountName = ""
			inv.Payee = &invoice.Party{Name: "Schneider Factoring AG",
				Address: invoice.Address{Line1: "Bahnhofstrasse 1", PostCode: "8001", City: "Zürich", CountryCode: "CH"}}
			inv.Buyer.Address.City = ""
		}, func(t *testing.T, b *Bill) {
			if b.Creditor != (Address{Name: "Schneider Factoring AG", Street: "Bahnhofstrasse 1", PostCode: "8001", Town: "Zürich", Country: "CH"}) ||
				b.Debtor != nil {
				t.Errorf("%+v %+v", b.Creditor, b.Debtor)
			}
		}, -1},
		{"notification", func(inv *invoice.Invoice, _ *Data) {
			inv.Totals.DuePayable = invoice.MustDecimal("0.00")
		}, func(t *testing.T, b *Bill) {
			if b.Amount.String() != "0.00" || b.Message != "NE PAS UTILISER POUR LE PAIEMENT" {
				t.Errorf("%s %q", b.Amount, b.Message)
			}
		}, -1},
		{"notification in English", func(inv *invoice.Invoice, _ *Data) {
			inv.Totals.DuePayable = invoice.MustDecimal("0")
			inv.Language = ""
		}, func(t *testing.T, b *Bill) {
			if b.Message != "DO NOT USE FOR PAYMENT" {
				t.Errorf("%q", b.Message)
			}
		}, -1},
		{"grand total", func(inv *invoice.Invoice, _ *Data) {
			inv.Totals.DuePayable = invoice.Decimal{}
			inv.Totals.GrandTotal = invoice.MustDecimal("1929.304")
		}, func(t *testing.T, b *Bill) {
			if b.Amount.String() != "1929.30" {
				t.Errorf("%s", b.Amount)
			}
		}, -1},
		{"negative amount", func(inv *invoice.Invoice, _ *Data) {
			inv.Totals.DuePayable = invoice.MustDecimal("-10.00")
		}, nil, 19},
		{"currency", func(inv *invoice.Invoice, _ *Data) { inv.Currency = "USD" }, nil, 20},
		{"QR reference in EUR", func(inv *invoice.Invoice, _ *Data) { inv.Currency = "EUR" }, nil, 28},
		{"no IBAN", func(inv *invoice.Invoice, _ *Data) { inv.PaymentMeans = nil }, nil, 4},
		{"QR-IBAN without reference", func(inv *invoice.Invoice, _ *Data) {
			inv.PaymentMeans = inv.PaymentMeans[1:]
			inv.RemittanceInformation = ""
		}, nil, 29},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv, d := swissInvoice(), new(Data)
			tc.edit(inv, d)
			b, err := Build(inv, d)
			got := errLines(err)
			switch {
			case tc.line < 0 && err != nil:
				t.Fatalf("Build: %v", err)
			case tc.line >= 0 && (len(got) == 0 || slices.ContainsFunc(got, func(l int) bool { return l != tc.line })):
				t.Errorf("errors on lines %v, want line %d: %v", got, tc.line, err)
			}
			if tc.check != nil {
				tc.check(t, b)
			}
		})
	}
	if _, err := Build(nil, nil); err == nil {
		t.Error("Build(nil) accepts it")
	}
}

// TestSwissUID reads the UID of the seller in its usual forms.
func TestSwissUID(t *testing.T) {
	for in, want := range map[string]string{
		"CHE-123.456.788 MWST": "123456788",
		"CHE123456788TVA":      "123456788",
		"che-123.456.788 iva":  "123456788",
		"CHE-123.456.78":       "",
		"DE123456789":          "",
		"CHE-123.456.788 X":    "",
	} {
		if got := swissUID(in); got != want {
			t.Errorf("swissUID(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSymbol checks the symbol of the longest content, the cross and its
// drawings.
func TestSymbol(t *testing.T) {
	ex := annexA[1].bill
	ex.Creditor.Name = strings.Repeat("N", 70)
	ex.Creditor.Street = strings.Repeat("S", 70)
	ex.Debtor = &Address{strings.Repeat("N", 70), strings.Repeat("S", 70), strings.Repeat("1", 16),
		strings.Repeat("9", 16), strings.Repeat("T", 35), "CH"}
	ex.Message = strings.Repeat("M", 140-len(ex.BillInformation))
	ex.AlternativeSchemes = []string{strings.Repeat("A", 100), strings.Repeat("B", 100)}
	sym, err := ex.Symbol()
	if err != nil {
		t.Fatal(err)
	}
	if sym.Version() > 25 || sym.Level() != qrcode.Medium {
		t.Errorf("v%d-%s for %d bytes", sym.Version(), sym.Level(), len(ex.String()))
	}
	bad := annexA[0].bill
	bad.Currency = "USD"
	if _, err := bad.Symbol(); err == nil {
		t.Error("Symbol encodes a content in USD")
	}

	sym, _ = annexA[1].bill.Symbol()
	n := float64(sym.Size())
	cross := Cross(sym.Size())
	if len(cross) != 4 || !cross[0].Light || cross[1].Light || !cross[2].Light || !cross[3].Light {
		t.Fatalf("cross %+v", cross)
	}
	side := n * 7 / 46
	if c := cross[0]; abs(c.W-side) > 1e-9 || abs(c.X+c.W/2-n/2) > 1e-9 || abs(c.Y+c.H/2-n/2) > 1e-9 {
		t.Errorf("white square %+v, want a side of %.3f modules at the center of %v", c, side, n)
	}
	if c := cross[1]; abs(c.W/cross[0].W-(19.8-1.4357-1.4)/19.8) > 1e-9 {
		t.Errorf("black square %+v", c)
	}
	r := Render(sym, 3)
	if r.Scale != 3 || r.QuietZone != 4 || len(r.Overlay) != 4 {
		t.Errorf("Render %+v", r)
	}
	var svg bytes.Buffer
	if err := sym.SVG(&svg, r); err != nil || !strings.Contains(svg.String(), "<rect") {
		t.Errorf("SVG: %v", err)
	}
	img := sym.Image(r)
	if img.Bounds().Dx() != (sym.Size()+8)*3 {
		t.Errorf("image of %d pixels", img.Bounds().Dx())
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// FuzzParse checks that Parse does not panic, and that a content that it
// accepts is written back by String, LF for the line breaks and without
// the empty lines of status A.
func FuzzParse(f *testing.F) {
	for _, ex := range annexA {
		f.Add(ex.content)
	}
	f.Add(set(3, 32, "//S1/10/1"))
	f.Fuzz(func(t *testing.T, s string) {
		b, err := Parse(s, Guidelines23)
		if err != nil {
			return
		}
		norm := strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
		if got := b.String(); got != norm {
			t.Errorf("Parse(%q).String() = %q", s, got)
		}
		if err := b.Check(); err != nil {
			t.Errorf("Check after Parse: %v", err)
		}
	})
}
