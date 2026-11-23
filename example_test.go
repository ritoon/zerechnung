package zerechnung_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

// newInvoice builds a French B2B invoice for services at the EN 16931
// profile, with the mentions required by the 2026 reform.
func newInvoice() *invoice.Invoice {
	d := invoice.MustDecimal
	seller := invoice.Party{
		Name:              "Exemple Informatique SAS",
		IDs:               []invoice.Identifier{{ID: "12345678900017", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "123456789", Scheme: invoice.SchemeSIREN},
		VATID:             "FR32123456789",
		Address:           invoice.Address{Line1: "1 rue de l'Exemple", PostCode: "75001", City: "Paris", CountryCode: "FR"},
		ElectronicAddress: invoice.Identifier{ID: "123456789_FACTURES", Scheme: invoice.SchemeFRDirectory},
	}
	buyer := invoice.Party{
		Name:              "Client Exemple SARL",
		IDs:               []invoice.Identifier{{ID: "98765432100015", Scheme: invoice.SchemeSIRET}},
		LegalRegistration: invoice.Identifier{ID: "987654321", Scheme: invoice.SchemeSIREN},
		VATID:             "FR58987654321",
		Address:           invoice.Address{Line1: "2 place de l'Exemple", PostCode: "69002", City: "Lyon", CountryCode: "FR"},
		ElectronicAddress: invoice.Identifier{ID: "987654321_ACHATS", Scheme: invoice.SchemeFRDirectory},
	}
	return &invoice.Invoice{
		Profile:         invoice.ProfileEN16931,
		BusinessProcess: "S1", // services
		Number:          "FA-2026-00042",
		TypeCode:        invoice.TypeCommercialInvoice,
		IssueDate:       invoice.NewDate(2026, time.October, 15),
		Currency:        "EUR",
		Notes: []invoice.Note{
			{SubjectCode: "BAR", Content: "B2B"},
			{SubjectCode: "PMT", Content: "Indemnité forfaitaire pour frais de recouvrement : 40 euros."},
			{SubjectCode: "PMD", Content: "Pénalités de retard : trois fois le taux d'intérêt légal."},
			{SubjectCode: "AAB", Content: "Pas d'escompte pour paiement anticipé."},
		},
		BuyerReference: "SERVICE-ACHATS",
		Seller:         seller,
		Buyer:          buyer,
		DeliveryDate:   invoice.NewDate(2026, time.October, 12),
		PaymentMeans:   []invoice.PaymentMeans{{TypeCode: invoice.PaymentSEPACreditTransfer, PayeeIBAN: "FR7630006000011234567890189"}},
		PaymentTerms:   []invoice.PaymentTerms{{Description: "Paiement à 30 jours par virement.", DueDate: invoice.NewDate(2026, time.November, 14)}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Maintenance applicative, octobre 2026"},
			Quantity:        d("2"),
			UnitCode:        "DAY",
			NetPrice:        d("450.00"),
			VATCategoryCode: invoice.VATStandard,
			VATRate:         d("20.00"),
		}},
	}
}

// Example creates a Factur-X invoice from a PDF produced elsewhere, then reads
// it back and checks it with the French rules.
func Example() {
	host, err := os.ReadFile("testdata/host.pdf") // any PDF, ideally PDF/A-3
	if err != nil {
		log.Fatal(err)
	}

	inv := newInvoice()
	pdf, err := zerechnung.Create(host, inv, &zerechnung.CreateOptions{
		Calculate: true, // line amounts, totals and VAT breakdown
		Validate:  &validate.Options{Rules: validate.Default | validate.France},
	})
	if err != nil {
		log.Fatal(err)
	}

	back, att, err := zerechnung.Read(pdf)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(att.Filename, att.Relationship, att.Metadata.ConformanceLevel)
	fmt.Println(back.Number, back.Totals.TaxBasisTotal, back.Totals.TaxTotal, back.Totals.DuePayable, back.Currency)

	report, _, err := zerechnung.Check(pdf, &validate.Options{Rules: validate.Default | validate.France})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("errors:", len(report.Errors()))
	// Output:
	// factur-x.xml Alternative EN 16931
	// FA-2026-00042 900.00 180.00 1080.00 EUR
	// errors: 0
}

// Example_vatOnDebits declares the option for paying the VAT on debits
// ("option pour le paiement de la taxe d'après les débits"), a mandatory
// mention in France for a supplier of services who has opted for it. The
// DGFiP carries it in BT-8 with the code of the invoice date (rule G1.43 of
// its external specifications v3.2): 5 in CII, written in every VAT
// breakdown. BT-8 excludes the VAT point date BT-7 (BR-CO-03). Without the
// option, leave BT-8 empty: the VAT on services is then due on payment.
func Example_vatOnDebits() {
	inv := newInvoice() // services, billing frame S1
	inv.TaxPointDateCode = invoice.TaxPointDebits
	inv.Calculate()

	xml, err := cii.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	const elem = "<ram:DueDateTypeCode>5</ram:DueDateTypeCode>"
	fmt.Println(elem, bytes.Count(xml, []byte(elem)) == 1)

	report := validate.XML(xml, &validate.Options{Rules: validate.Default | validate.France})
	fmt.Println("errors:", len(report.Errors()))
	// Output:
	// <ram:DueDateTypeCode>5</ram:DueDateTypeCode> true
	// errors: 0
}

// Example_ciusRO writes a Romanian invoice of the CIUS-RO, for RO
// e-Factura, in UBL and in CII. An address in Romania carries its county
// code of ISO 3166-2:RO in the country subdivision (BT-39), and an address
// in Bucharest (RO-B) its sector as city (BT-37); an invoice in another
// currency than the leu gives the VAT accounting currency RON (BT-6) and
// the VAT total in lei (BT-111). The Romanian rules apply to the invoice as
// it declares the CIUS-RO in BT-24.
func Example_ciusRO() {
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:     invoice.ProfileCIUSRO,
		Number:      "RO-2026-0001",
		TypeCode:    invoice.TypeCommercialInvoice,
		IssueDate:   invoice.NewDate(2026, time.November, 20),
		Currency:    "EUR",
		TaxCurrency: "RON",
		Seller: invoice.Party{
			Name:    "Exemplu Software SRL",
			VATID:   "RO14399840",
			Address: invoice.Address{Line1: "Strada Exemplu 10", City: "SECTOR2", PostCode: "021234", CountrySubdivision: "RO-B", CountryCode: "RO"},
		},
		Buyer: invoice.Party{
			Name:    "Client Cluj SA",
			VATID:   "RO18547290",
			Address: invoice.Address{Line1: "Bulevardul Eroilor 5", City: "Cluj-Napoca", PostCode: "400129", CountrySubdivision: "RO-CJ", CountryCode: "RO"},
		},
		PaymentTerms: []invoice.PaymentTerms{{DueDate: invoice.NewDate(2026, time.December, 20)}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Mentenanță"},
			Quantity:        d("2"),
			UnitCode:        "HUR",
			NetPrice:        d("150.00"),
			VATCategoryCode: invoice.VATStandard,
			VATRate:         d("19"),
		}},
	}
	inv.Calculate()
	inv.Totals.TaxTotalInTaxCurrency = d("284.10") // 57.00 EUR at 4.9842 lei

	ublXML, err := ubl.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	ciiXML, err := cii.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, x := range [][]byte{ublXML, ciiXML} {
		report := validate.XML(x, nil)
		fmt.Println(report.Profile, "errors:", len(report.Errors()), "warnings:", len(report.Findings)-len(report.Errors()))
	}
	// Output:
	// CIUS-RO errors: 0 warnings: 0
	// CIUS-RO errors: 0 warnings: 0
}

// Example_slovakia writes a Slovak invoice of the eFaktúra: Peppol BIS
// Billing 3.0 in UBL, between two VAT payers established in Slovakia. Beyond
// Peppol BIS, the Finančná správa requires the electronic addresses of
// scheme 0245 (the DIČ), complete postal addresses, the IČ DPH of the VAT
// payers, the IČO of the companies, the legal form and the register entry of
// the seller (BT-33) and, when the supply is not dated on the issue date,
// the VAT point date (BT-7). Auto applies the Slovak rules to a seller in
// Slovakia.
func Example_slovakia() {
	d := invoice.MustDecimal
	inv := &invoice.Invoice{
		Profile:        invoice.ProfilePeppol,
		Number:         "FA-2027-0001",
		TypeCode:       invoice.TypeCommercialInvoice,
		IssueDate:      invoice.NewDate(2027, time.January, 15),
		Currency:       "EUR",
		TaxPointDate:   invoice.NewDate(2027, time.January, 12),
		BuyerReference: "OBJ-2026-118",
		Seller: invoice.Party{
			Name:              "Príklad Softvér s.r.o.",
			Description:       "Zapísaná v Obchodnom registri Mestského súdu Bratislava III, oddiel Sro, vložka č. 12345/B",
			LegalRegistration: invoice.Identifier{ID: "12345678"},
			ElectronicAddress: invoice.Identifier{ID: "2020123456", Scheme: "0245"},
			VATID:             "SK2020123456",
			Address:           invoice.Address{Line1: "Hlavná 1", City: "Bratislava", PostCode: "811 01", CountryCode: "SK"},
		},
		Buyer: invoice.Party{
			Name:              "Odberateľ Košice a.s.",
			LegalRegistration: invoice.Identifier{ID: "87654321"},
			ElectronicAddress: invoice.Identifier{ID: "2021987654", Scheme: "0245"},
			VATID:             "SK2021987654",
			Address:           invoice.Address{Line1: "Hlavná 50", City: "Košice", PostCode: "040 01", CountryCode: "SK"},
		},
		PaymentTerms: []invoice.PaymentTerms{{DueDate: invoice.NewDate(2027, time.January, 29)}},
		Lines: []invoice.Line{{
			ID:              "1",
			Item:            invoice.Item{Name: "Údržba siete"},
			Quantity:        d("10"),
			UnitCode:        "HUR",
			NetPrice:        d("80.00"),
			VATCategoryCode: invoice.VATStandard,
			VATRate:         d("23"),
		}},
	}
	inv.Calculate()

	xml, err := ubl.Marshal(inv, nil)
	if err != nil {
		log.Fatal(err)
	}
	report := validate.XML(xml, &validate.Options{Auto: true})
	fmt.Println(report.Profile, "errors:", len(report.Errors()), "warnings:", len(report.Findings)-len(report.Errors()))
	// Output:
	// PEPPOL errors: 0 warnings: 0
}

// ConvertPDFA lists why a PDF cannot be converted to PDF/A-3: here, the
// font of the host of the tests is not embedded.
func ExampleConvertPDFA() {
	host, err := os.ReadFile("testdata/host.pdf")
	if err != nil {
		log.Fatal(err)
	}
	pdfa, err := zerechnung.ConvertPDFA(host, nil)
	var ce *zerechnung.ConversionError
	if errors.As(err, &ce) {
		for _, r := range ce.Reasons {
			fmt.Println(r)
		}
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(pdfa) > 0)
	// Output:
	// page 1 /Contents: the font "Helvetica" (Type1) shows text but its font program is not embedded, which PDF/A requires; embed the font when producing the PDF
}

// A Checker keeps the font programs it reads: a server that checks the
// invoices of the same issuers creates one and shares it between its
// requests. The second check of the PDF finds its three fonts in the
// cache, and gives the same report.
func ExampleChecker() {
	pdf, err := os.ReadFile("testdata/EN16931_Einfach.pdf")
	if err != nil {
		log.Fatal(err)
	}
	checker := zerechnung.NewChecker(&zerechnung.CheckerOptions{FontCache: 32 << 20})
	for range 2 {
		report, att, err := checker.Check(pdf, nil)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(att.Filename, len(report.Errors()), "errors")
	}
	// Output:
	// factur-x.xml 0 errors
	// factur-x.xml 0 errors
}

// ExampleSign signs a hybrid invoice in PAdES Baseline B-B and verifies the
// signature. The self-signed certificate stands in for the qualified
// certificate of the company and its chain.
func ExampleSign() {
	host, err := os.ReadFile("testdata/host.pdf")
	if err != nil {
		log.Fatal(err)
	}
	hybrid, err := zerechnung.Create(host, newInvoice(), &zerechnung.CreateOptions{Calculate: true})
	if err != nil {
		log.Fatal(err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Exemple Informatique SAS", Country: []string{"FR"}},
		NotBefore:    now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		log.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		log.Fatal(err)
	}

	signed, err := zerechnung.Sign(hybrid, &zerechnung.SignOptions{
		Signer: key, Chain: []*x509.Certificate{cert}, Reason: "Facture", Location: "Paris",
	})
	if err != nil {
		log.Fatal(err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(cert)
	report, sigs, err := zerechnung.VerifySignatures(signed, &validate.TrustOptions{Roots: roots})
	if err != nil {
		log.Fatal(err)
	}
	s := sigs[0]
	fmt.Println(s.Field, s.Level, s.Valid, s.WholeDocument, s.Reason)
	fmt.Println("errors:", len(report.Errors()))
	// Output:
	// Signature1 B-B true true Facture
	// errors: 0
}
