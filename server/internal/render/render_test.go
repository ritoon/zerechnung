package render

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

var fixed = time.Date(2026, 11, 22, 10, 30, 0, 0, time.UTC)

// sample is a French invoice for services with two VAT rates, a line
// allowance, a document charge and the usual payment details.
func sample() *invoice.Invoice {
	d := invoice.MustDecimal
	return &invoice.Invoice{
		Profile:   invoice.ProfileEN16931,
		Number:    "FA-2026-00042",
		TypeCode:  invoice.TypeCommercialInvoice,
		IssueDate: invoice.NewDate(2026, time.October, 15),
		Currency:  "EUR",
		Notes: []invoice.Note{
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Pénalités de retard : trois fois le taux d'intérêt légal."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		BuyerReference:         "SERVICE-ACHATS",
		PurchaseOrderReference: "PO-4512",
		Seller: invoice.Party{
			Name:              "Exemple Informatique SAS",
			Description:       "SAS au capital de 10 000 €",
			IDs:               []invoice.Identifier{{ID: "12345678900017", Scheme: invoice.SchemeSIRET}},
			LegalRegistration: invoice.Identifier{ID: "123456789", Scheme: invoice.SchemeSIREN},
			VATID:             "FR32123456789",
			Address:           invoice.Address{Line1: "1 rue de l'Exemple", PostCode: "75001", City: "Paris", CountryCode: "FR"},
			Contact:           &invoice.Contact{PersonName: "Camille Martin", Phone: "+33 1 23 45 67 89", Email: "factures@exemple.fr"},
		},
		Buyer: invoice.Party{
			Name:              "Client Exemple SARL",
			LegalRegistration: invoice.Identifier{ID: "987654321", Scheme: invoice.SchemeSIREN},
			VATID:             "FR58987654321",
			Address:           invoice.Address{Line1: "2 place de l'Exemple", Line2: "Bâtiment B", PostCode: "69002", City: "Lyon", CountryCode: "FR"},
		},
		ShipTo: &invoice.Party{
			Name:    "Entrepôt Client Exemple",
			Address: invoice.Address{Line1: "ZI des Exemples", PostCode: "69800", City: "Saint-Priest", CountryCode: "FR"},
		},
		DeliveryDate: invoice.NewDate(2026, time.October, 12),
		PaymentMeans: []invoice.PaymentMeans{{
			TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "FR7630006000011234567890189",
			PayeeBIC: "AGRIFRPP", PayeeAccountName: "Exemple Informatique SAS",
		}},
		RemittanceInformation: "FA-2026-00042",
		PaymentTerms:          []invoice.PaymentTerms{{Description: "Paiement à 30 jours par virement.", DueDate: invoice.NewDate(2026, time.November, 14)}},
		AllowancesCharges: []invoice.AllowanceCharge{{
			Charge: true, Amount: d("25.00"), Reason: "Frais de déplacement",
			VATCategoryCode: invoice.VATStandard, VATRate: d("20.00"),
		}},
		Lines: []invoice.Line{
			{
				ID:       "1",
				Item:     invoice.Item{Name: "Maintenance applicative, octobre 2026", Description: "Correctifs et évolutions mineures de l'application de facturation", SellerID: "MAINT-01"},
				Quantity: d("2"), UnitCode: "DAY", NetPrice: d("450.00"),
				VATCategoryCode: invoice.VATStandard, VATRate: d("20.00"),
				Period: invoice.Period{Start: invoice.NewDate(2026, time.October, 1), End: invoice.NewDate(2026, time.October, 31)},
			},
			{
				ID:       "2",
				Item:     invoice.Item{Name: "Formation des utilisateurs (support imprimé)"},
				Quantity: d("12"), UnitCode: "C62", NetPrice: d("35.50"),
				VATCategoryCode: invoice.VATStandard, VATRate: d("5.5"),
				AllowancesCharges: []invoice.AllowanceCharge{{Amount: d("21.30"), Percentage: d("5"), BaseAmount: d("426.00"), Reason: "Remise fidélité"}},
			},
		},
	}
}

// multipage is a long invoice whose texts mix accents, ß, the euro sign,
// Polish letters and characters the fonts lack.
func multipage() *invoice.Invoice {
	inv := sample()
	inv.Number = "RE-2026-ÄÖÜ-ß"
	inv.Seller.Name = "Łódzka Spółka Żółć Gęślą sp. z o.o. & Straße GmbH"
	inv.Seller.Address = invoice.Address{Line1: "ul. Świętokrzyska 12", PostCode: "00-916", City: "Warszawa", CountryCode: "PL"}
	inv.Buyer.Name = "Müller & Söhne Großhandel KG — 漢字 ✓"
	d := invoice.MustDecimal
	inv.Lines = nil
	for i := 1; i <= 64; i++ {
		ln := invoice.Line{
			ID:       fmt.Sprint(i),
			Item:     invoice.Item{Name: fmt.Sprintf("Artikel %d: Größe XL, Preis in €, zażółć gęślą jaźń ✓", i)},
			Quantity: d("3"), UnitCode: "H87", NetPrice: d("19.99"),
			VATCategoryCode: invoice.VATStandard, VATRate: d("23"),
		}
		if i%5 == 0 {
			ln.Item.Description = strings.Repeat("Długi opis pozycji z polskimi znakami: ąćęłńóśźż ĄĆĘŁŃÓŚŹŻ. ", 4)
			ln.Note = "Hinweis: Lieferung erfolgt in zwei Teilen; Rückfragen an die Buchhaltung ☎."
		}
		inv.Lines = append(inv.Lines, ln)
	}
	inv.AllowancesCharges = nil
	return inv
}

// pdfFindings returns the findings of Check whose source is the PDF.
func pdfFindings(t *testing.T, pdf []byte) []validate.Finding {
	t.Helper()
	r, att, err := zerechnung.Check(pdf, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if att == nil {
		t.Fatal("Check found no invoice")
	}
	var out []validate.Finding
	for _, f := range r.Findings {
		if f.Source == validate.SourcePDF {
			out = append(out, f)
		}
	}
	return out
}

// create renders inv and makes it a Factur-X invoice.
func create(t *testing.T, inv *invoice.Invoice, o *Options) (rendered, hybrid []byte) {
	t.Helper()
	pdf, err := Invoice(inv, o)
	if err != nil {
		t.Fatalf("Invoice: %v", err)
	}
	out, err := zerechnung.Create(pdf, inv, &zerechnung.CreateOptions{Calculate: true, Embed: zerechnung.EmbedOptions{RequirePDFA: true}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeOut(t, inv.Number+"-"+o.Language, pdf, out)
	return pdf, out
}

// writeOut saves the files in $RENDER_OUT, for veraPDF or a visual check.
func writeOut(t *testing.T, name string, rendered, hybrid []byte) {
	dir := os.Getenv("RENDER_OUT")
	if dir == "" {
		return
	}
	name = regexp.MustCompile(`[^A-Za-z0-9-]+`).ReplaceAllString(name, "_")
	if err := os.WriteFile(filepath.Join(dir, name+".pdf"), rendered, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+"-facturx.pdf"), hybrid, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLanguages(t *testing.T) {
	want := []string{"de", "en", "es", "fr", "it", "nl", "pt"}
	if got := Languages(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Languages() = %v, want %v", got, want)
	}
	for _, lang := range want {
		t.Run(lang, func(t *testing.T) {
			_, out := create(t, sample(), &Options{Language: lang, Time: fixed})
			for _, f := range pdfFindings(t, out) {
				t.Errorf("%s", f)
			}
		})
	}
}

func TestMultipage(t *testing.T) {
	inv := multipage()
	rendered, out := create(t, inv, &Options{Language: "de", Time: fixed})
	pages := bytes.Count(rendered, []byte("/Type /Page "))
	if pages < 3 {
		t.Errorf("%d pages, want at least 3", pages)
	}
	for _, f := range pdfFindings(t, out) {
		t.Errorf("%s", f)
	}
	text := contentText(t, rendered)
	if !strings.Contains(text, fmt.Sprintf("Seite %d/%d", pages, pages)) {
		t.Errorf("no page number %d/%d in the content", pages, pages)
	}
}

// contentText returns the text of the content streams, decoded through the
// ToUnicode CMaps: the renderer writes one glyph index per character.
func contentText(t *testing.T, pdf []byte) string {
	t.Helper()
	// Glyph to character maps of the two fonts, from the ToUnicode CMaps.
	cmaps := map[string]map[string]rune{}
	streamRe := regexp.MustCompile(`(?s)(\d+) 0 obj\n<< ([^\n]*?)>>\nstream\n(.*?)\nendstream`)
	objs := map[string][]byte{}
	for _, m := range streamRe.FindAllSubmatch(pdf, -1) {
		data := m[3]
		if bytes.Contains(m[2], []byte("/FlateDecode")) {
			zr, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			data, err = io.ReadAll(zr)
			if err != nil {
				t.Fatal(err)
			}
		}
		objs[string(m[1])] = data
	}
	fontRe := regexp.MustCompile(`/BaseFont /[A-Z]{6}\+(Go-\w+) /Encoding /Identity-H /DescendantFonts \[\d+ 0 R\] /ToUnicode (\d+) 0 R`)
	res := map[string]string{"Go-Regular": "F1", "Go-Bold": "F2"}
	bf := regexp.MustCompile(`<([0-9A-F]{4})> <([0-9A-F]{4})>`)
	for _, m := range fontRe.FindAllSubmatch(pdf, -1) {
		cm := map[string]rune{}
		for _, e := range bf.FindAllSubmatch(objs[string(m[2])], -1) {
			var r rune
			fmt.Sscanf(string(e[2]), "%X", &r)
			cm[string(e[1])] = r
		}
		cmaps[res[string(m[1])]] = cm
	}
	var b strings.Builder
	showRe := regexp.MustCompile(`BT /(F\d) [^<]*<([0-9A-F]*)> Tj ET`)
	for _, data := range objs {
		for _, m := range showRe.FindAllSubmatch(data, -1) {
			cm := cmaps[string(m[1])]
			for i := 0; i+4 <= len(m[2]); i += 4 {
				b.WriteRune(cm[string(m[2][i:i+4])])
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestText(t *testing.T) {
	rendered, _ := create(t, multipage(), &Options{Language: "fr", Time: fixed})
	text := contentText(t, rendered)
	for _, s := range []string{
		"Łódzka Spółka Żółć Gęślą", "Müller & Söhne Großhandel KG — ?? ?", "Größe XL, Preis in €",
		"ąćęłńóśźż", "Facture", "N° RE-2026-ÄÖÜ-ß", "Net à payer",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the text has no %q", s)
		}
	}
}

func TestNoNotdef(t *testing.T) {
	fs, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	u := newUsedFont(fs[0], "F1")
	for _, g := range u.encode("A漢✓☃é") {
		if g == 0 {
			t.Fatal("a character is shown with .notdef")
		}
	}
	if q := u.glyph('?'); u.encode("漢")[0] != q {
		t.Error("a missing character is not shown as '?'")
	}
}

func TestDeterministic(t *testing.T) {
	a, err := Invoice(sample(), &Options{Time: fixed})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Invoice(sample(), &Options{Time: fixed})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two renderings of the same invoice differ")
	}
	c, _ := Invoice(sample(), &Options{Time: fixed.Add(time.Second)})
	if bytes.Equal(a, c) {
		t.Error("the creation date is not written")
	}
}

func TestLanguageChoice(t *testing.T) {
	inv := sample()
	for _, c := range []struct {
		requested, docLang, country, want string
	}{
		{"", "", "FR", "fr"},
		{"", "", "AT", "de"},
		{"", "", "BE", "nl"},
		{"", "", "US", "en"},
		{"", "it", "FR", "it"},
		{"", "xx", "ES", "es"},
		{"pt-BR", "", "FR", "pt"},
		{"EN", "de", "DE", "en"},
	} {
		inv.Language, inv.Seller.Address.CountryCode = c.docLang, c.country
		got, err := language(inv, c.requested)
		if err != nil || got != c.want {
			t.Errorf("language(%q, doc %q, country %q) = %q, %v; want %q", c.requested, c.docLang, c.country, got, err, c.want)
		}
	}
	if _, err := Invoice(inv, &Options{Language: "xx"}); err == nil {
		t.Error("an unsupported language is accepted")
	}
	if _, err := Invoice(nil, nil); err == nil {
		t.Error("a nil invoice is accepted")
	}
}

func TestInvoiceNotModified(t *testing.T) {
	inv := sample()
	if _, err := Invoice(inv, &Options{Time: fixed}); err != nil {
		t.Fatal(err)
	}
	if inv.Totals.DuePayable.IsSet() || inv.Lines[0].NetAmount.IsSet() || inv.Lines[1].AllowancesCharges[0].Amount.String() != "21.30" || len(inv.VATBreakdown) != 0 {
		t.Error("Invoice modified the invoice")
	}
}

func TestCreditNoteAndEmpty(t *testing.T) {
	inv := sample()
	inv.TypeCode = invoice.TypeCreditNote
	inv.PrecedingInvoices = []invoice.DocumentReference{{ID: "FA-2026-00041", IssueDate: invoice.NewDate(2026, time.October, 1)}}
	inv.CopyIndicator = true
	rendered, out := create(t, inv, &Options{Language: "en", Time: fixed})
	if text := contentText(t, rendered); !strings.Contains(text, "Credit note") || !strings.Contains(text, "Preceding invoice: FA-2026-00041") {
		t.Errorf("no credit note title or preceding invoice in:\n%s", text)
	}
	for _, f := range pdfFindings(t, out) {
		t.Errorf("%s", f)
	}

	// A minimal invoice without lines still gives a valid PDF.
	min := &invoice.Invoice{Profile: invoice.ProfileMinimum, Number: "M-1", TypeCode: "380", IssueDate: invoice.NewDate(2026, 1, 2), Currency: "CHF",
		Seller: invoice.Party{Name: "S", Address: invoice.Address{CountryCode: "CH"}}, Buyer: invoice.Party{Name: "B"},
		Totals: invoice.Totals{TaxBasisTotal: invoice.MustDecimal("10.00"), GrandTotal: invoice.MustDecimal("10.81"), DuePayable: invoice.MustDecimal("10.81")}}
	_, out = create(t, min, &Options{Time: fixed})
	for _, f := range pdfFindings(t, out) {
		t.Errorf("minimum: %s", f)
	}
}

func TestFormat(t *testing.T) {
	d := invoice.MustDecimal
	for _, c := range []struct {
		lang, got, want string
	}{
		{"fr", translations["fr"].money(d("1234567.5"), "EUR"), "1 234 567,50 €"},
		{"de", translations["de"].money(d("-1234.56"), "EUR"), "-1.234,56 €"},
		{"en", translations["en"].money(d("-1234.56"), "EUR"), "-€1,234.56"},
		{"en", translations["en"].money(d("99"), "CHF"), "CHF 99.00"},
		{"nl", translations["nl"].rate(d("21.00")), "21%"},
		{"en", translations["en"].rate(d("0.50")), "0.5%"},
		{"fr", translations["fr"].rate(d("5.5")), "5,5 %"},
		{"de", translations["de"].date(invoice.NewDate(2026, 3, 9)), "09.03.2026"},
		{"en", translations["en"].date(invoice.NewDate(2026, 3, 9)), "9 Mar 2026"},
		{"it", translations["it"].num(d("12.125"), 2), "12,125"},
		{"en", iban("FR7630006000011234567890189"), "FR76 3000 6000 0112 3456 7890 189"},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.lang, c.got, c.want)
		}
	}
}

// TestLabelsFit checks that no column header is shortened, in any language.
func TestLabelsFit(t *testing.T) {
	fs, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	bold := fs[1]
	vc := vatColumns(left)
	for lang, l := range translations {
		for _, c := range []struct {
			label string
			w     float64
		}{
			{l.colNo, colNo.w}, {l.colDescription, colDesc.w}, {l.colQty, colQty.w}, {l.colUnit, colUnit.w},
			{l.colUnitPrice, colPrice.w}, {l.colVAT, colVAT.w}, {l.colAmount, colAmount.w},
			{l.vatCategory, vc[0] - left}, {l.vatRate, vc[1] - vc[0] + cellPad}, {l.taxable, vc[2] - vc[1] + cellPad}, {l.vatAmount, vc[3] - vc[2] + cellPad},
		} {
			if got := bold.measure(c.label, sizeHead); got > c.w-2*cellPad {
				t.Errorf("%s: %q is %.1f wide, the column has %.1f", lang, c.label, got, c.w-2*cellPad)
			}
		}
	}
}

func BenchmarkInvoice(b *testing.B) {
	inv := multipage()
	for b.Loop() {
		if _, err := Invoice(inv, &Options{Time: fixed}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestConcurrent(t *testing.T) {
	want, err := Invoice(sample(), &Options{Time: fixed})
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			got, err := Invoice(sample(), &Options{Time: fixed})
			if err == nil && !bytes.Equal(got, want) {
				err = fmt.Errorf("concurrent renderings differ")
			}
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}
