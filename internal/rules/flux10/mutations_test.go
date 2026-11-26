package flux10

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// The mutation cases change one of the two fictitious reports of
// ereporting/testdata (France_RFE publishes no example of a report) so as to
// break one rule of the schematron, and give the rules that the package
// must report for the variant: each of the 150 rules is raised by at least
// one case. The expected rules were checked against the XSLT that the
// FNFE-MPE publishes for the schematron (France_RFE 1.4.0.04,
// FNFE_RFE_INVOICE/Flux10/Flux10.xslt), run by Saxon-HE 12.10, which
// TestReference repeats:
//
//	go test ./internal/rules/flux10 -run '^TestVariants$' -variants DIR
//	(run the XSLT on DIR/*.xml, one SVRL report DIR/<name>.xml.svrl each,
//	with conformance/parity/NPaBatch.java)
//	go test ./internal/rules/flux10 -run '^TestReference$' -variants DIR
var variantsDir = flag.String("variants", "", "directory of the variants of the mutation cases (TestVariants, TestReference)")

// testdata holds the fictitious reports of package ereporting.
const testdata = "../../../ereporting/testdata/"

// edit replaces occurrence nth (1-based) of old by new.
type edit struct {
	nth      int
	old, new string
}

func apply(t testing.TB, doc string, e edit) string {
	t.Helper()
	at, off := -1, 0
	for range e.nth {
		i := strings.Index(doc[off:], e.old)
		if i < 0 {
			t.Fatalf("edit: occurrence %d of %q not found", e.nth, e.old)
		}
		at, off = off+i, off+i+len(e.old)
	}
	return doc[:at] + e.new + doc[at+len(e.old):]
}

// load returns the report base of testdata: transactions or payments.
func load(t testing.TB, base string) string {
	t.Helper()
	b, err := os.ReadFile(testdata + base + ".xml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// check returns the report of Check for the report doc.
func check(t testing.TB, doc string) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Report: r, Source: rules.SourceFlux10}, root)
	return r
}

// findings returns the sorted identifiers of the rules that the package
// reports for the report doc, each once.
func findings(t testing.TB, doc string) []string {
	t.Helper()
	var ids []string
	for _, f := range check(t, doc).Findings {
		ids = append(ids, f.Rule)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// Values of the mutations.
const (
	long36 = "F2026-0915-ABCDEFGHIJKLMNOPQRSTUVWXY" // 36 characters, one more than G1.05 allows
	long19 = "ABCDEFGHIJ123456789"                  // 19 characters, one more than G2.19 allows for 0223 and 0227
	digits = "12345678901234567890"                 // 20 digits, one more than G1.14, G1.15 and G1.16 allow
)

// Elements of the reports that the cases change.
const (
	tID        = "<ID>F2026-0915</ID>"                                                     // 1 invoice, 2 its reference in the credit note, 3 a line of the credit note
	tStart     = "<StartDate>20260901</StartDate>"                                         // 1 report period, 2 invoice period, 3 line period
	tEnd       = "<EndDate>20260930</EndDate>"                                             // same
	tDate      = "<IssueDate>20260915</IssueDate>"                                         // 1 invoice, 2 reference of the credit note, 3 reference of its line
	tSeller    = `<CompanyId schemeId="0002">123456782</CompanyId>`                        // seller of the invoice
	tBuyer     = `<CompanyId schemeId="0223">DE123456789</CompanyId>`                      // buyer of the invoice
	tSellerVAT = `<TaxRegistrationId qualifyingId="VAT">FR32123456782</TaxRegistrationId>` // VAT identifier of the seller of the invoice
	tBuyerVAT  = `<TaxRegistrationId qualifyingId="VAT">DE123456789</TaxRegistrationId>`   // VAT identifier of the buyer of the invoice
	tCountry   = "<CountryId>DE</CountryId>"                                               // 1 buyer, 2 delivery, 3 delivery of a line of the credit note
	pStart     = "<StartDate>20260901</StartDate>"
	pEnd       = "<EndDate>20260930</EndDate>"
)

// cases are the mutation cases: a report base, its edits, and the rules
// that the variant raises.
var cases = []struct {
	base  string
	edits []edit
	want  []string
}{
	// Transmission.
	{"payments", []edit{{1, "<Id>PA-2026-09-0001</Id>", "<Id>" + strings.Repeat("A", 51) + "</Id>"}}, []string{"F10-TRANSMISSION-G1.104"}},
	{"payments", []edit{{1, "<Id>PA-2026-09-0001</Id>", "<Id> PA-1</Id>"}}, []string{"F10-TRANSMISSION-G1.104-2"}},
	{"payments", []edit{{1, "<Id>PA-2026-09-0001</Id>", "<Id>PA  1</Id>"}}, []string{"F10-TRANSMISSION-G1.104-3"}},
	{"payments", []edit{{1, "<Id>PA-2026-09-0001</Id>", "<Id>PA.1</Id>"}}, []string{"F10-TRANSMISSION-G1.104-4"}},
	{"payments", []edit{{1, "20261011094500", "2026101109450"}}, []string{"F10-TRANSMISSION-G7.53"}},
	{"payments", []edit{{1, "20261011094500", "19991011094500"}}, []string{"F10-TRANSMISSION-G7.53-2"}},
	{"payments", []edit{{1, "<TypeCode>IN</TypeCode>", "<TypeCode>XX</TypeCode>"}}, []string{"F10-TRANSMISSION-G8.01"}},
	{"payments", []edit{{1, "</Report>", "<TransactionsReport/></Report>"}}, []string{"F10-TRANSMISSION-G6.29"}},
	{"payments", []edit{{1, `<Id schemeId="0238">0042</Id>`, `<Id schemeId="0239">0042</Id>`}}, []string{"F10-EMETTEUR-G6.22"}},
	{"payments", []edit{{1, `<Id schemeId="0238">0042</Id>`, `<Id schemeId="0238">00420</Id>`}}, []string{"F10-EMETTEUR-G6.22-2"}},
	{"payments", []edit{{1, "<RoleCode>WK</RoleCode>", "<RoleCode>BY</RoleCode>"}}, []string{"F10-EMETTEUR-G7.51"}},
	{"payments", []edit{{1, `<Id schemeId="0002">123456782</Id>`, `<Id schemeId="0009">123456782</Id>`}}, []string{"F10-DECLARANT-G6.26"}},
	{"payments", []edit{{1, `<Id schemeId="0002">123456782</Id>`, `<Id schemeId="0002">12345678</Id>`}}, []string{"F10-DECLARANT-G6.26-2"}},
	{"payments", []edit{{1, "<RoleCode>SE</RoleCode>", "<RoleCode>WK</RoleCode>"}}, []string{"F10-DECLARANT-G7.52"}},

	// Period of the transactions report.
	{"transactions", []edit{{1, tStart, "<StartDate>2026090</StartDate>"}}, []string{"F10-PERIODE-TX-G1.09a"}},
	{"transactions", []edit{{1, tStart, "<StartDate>19990901</StartDate>"}}, []string{"F10-PERIODE-TX-G1.09a-2"}},
	{"transactions", []edit{{1, tEnd, "<EndDate>2026093</EndDate>"}}, []string{"F10-PERIODE-TX-G1.09b"}},
	{"transactions", []edit{{1, tEnd, "<EndDate>21000930</EndDate>"}}, []string{"F10-PERIODE-TX-G1.09b-2"}},
	{"transactions", []edit{{1, tEnd, "<EndDate>20260901</EndDate>"}}, []string{"F10-PERIODE-TX-G6.25"}},

	// Invoice.
	{"transactions", []edit{{1, "<TypeCode>380</TypeCode>", "<TypeCode>999</TypeCode>"}}, []string{"F10-FACTURE-G1.01"}},
	{"transactions", []edit{{1, "<ID>S1</ID>", "<ID>X1</ID>"}}, []string{"F10-FACTURE-G1.02"}},
	{"transactions", []edit{{1, "<ID>S1</ID>", "<ID>S4</ID>"}, {1, "<TypeCode>380</TypeCode>", "<TypeCode>386</TypeCode>"}}, []string{"F10-FACTURE-G1.60"}},
	{"transactions", []edit{{1, "<TypeID>urn.cpro.gouv.fr:1p0:ereporting</TypeID>", "<TypeID>urn.cpro.gouv.fr:1p0:extended-ctc-fr</TypeID>"}}, []string{"F10-FACTURE-S1.12"}},
	{"transactions", []edit{{1, tID, "<ID>" + long36 + "</ID>"}}, []string{"F10-FACTURE-G1.05a"}},
	{"transactions", []edit{{1, tID, "<ID> F2026-0915</ID>"}}, []string{"F10-FACTURE-G1.05a-2"}},
	{"transactions", []edit{{1, tID, "<ID>F2026  0915</ID>"}}, []string{"F10-FACTURE-G1.05a-3"}},
	{"transactions", []edit{{1, tID, "<ID>F2026.0915</ID>"}}, []string{"F10-FACTURE-G1.05a-4"}},
	{"transactions", []edit{{1, tDate, "<IssueDate>2026091</IssueDate>"}}, []string{"F10-FACTURE-G1.09a"}},
	{"transactions", []edit{{1, tDate, "<IssueDate>19990915</IssueDate>"}}, []string{"F10-FACTURE-G1.09a-2"}},
	{"transactions", []edit{{1, "<DueDate>20261015</DueDate>", "<DueDate>2026101</DueDate>"}}, []string{"F10-FACTURE-G1.09b"}},
	{"transactions", []edit{{1, "<DueDate>20261015</DueDate>", "<DueDate>19991015</DueDate>"}}, []string{"F10-FACTURE-G1.09b-2"}},
	{"transactions", []edit{{1, "<TaxDueDateTypeCode>5</TaxDueDateTypeCode>", "<TaxDueDateTypeCode>4</TaxDueDateTypeCode>"}}, []string{"F10-FACTURE-P1.11"}},
	{"transactions", []edit{{1, "VATEX-EU-AE", "VATEX-FR-CNWVAT"}}, []string{"F10-FACTURE-G6.21"}},
	{"transactions", []edit{{1, "<TypeCode>380</TypeCode>", "<TypeCode>384</TypeCode>"}}, []string{"F10-FACTURE-G1.32a"}},
	{"transactions", []edit{{1, "<TypeCode>380</TypeCode>", "<TypeCode>381</TypeCode>"}}, []string{"F10-FACTURE-G1.32b"}},
	{"transactions", []edit{{1, tBuyer, ""}}, []string{"F10-FACTURE-G6.28"}},
	{"transactions", []edit{{2, tDate, "<IssueDate>2026091</IssueDate>"}}, []string{"F10-FACTURE-G1.09c"}},
	{"transactions", []edit{{2, tDate, "<IssueDate>19990915</IssueDate>"}}, []string{"F10-FACTURE-G1.09c-2"}},
	{"transactions", []edit{{2, tID, "<ID>" + long36 + "</ID>"}}, []string{"F10-FACTURE-G1.05b"}},
	{"transactions", []edit{{2, tID, "<ID>F2026-0915 </ID>"}}, []string{"F10-FACTURE-G1.05b-2"}},
	{"transactions", []edit{{2, tID, "<ID>F2026  0915</ID>"}}, []string{"F10-FACTURE-G1.05b-3"}},
	{"transactions", []edit{{2, tID, "<ID>F2026#0915</ID>"}}, []string{"F10-FACTURE-G1.05b-4"}},

	// Seller.
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0009">123456782</CompanyId>`}}, []string{"F10-VENDEUR-G2.19a"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0002">12345678</CompanyId>`}}, []string{"F10-VENDEUR-G2.19b"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0223">` + long19 + `</CompanyId>`}}, []string{"F10-VENDEUR-G2.19c"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0227">` + long19 + `</CompanyId>`}}, []string{"F10-VENDEUR-G2.19d"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0228">12345678</CompanyId>`}}, []string{"F10-VENDEUR-G2.19e"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0229">1234567890</CompanyId>`}}, []string{"F10-VENDEUR-G2.19f"}},
	{"transactions", []edit{{1, tSellerVAT, ""}}, []string{"F10-VENDEUR-G2.33a"}},
	{"transactions", []edit{{1, tSellerVAT, `<TaxRegistrationId qualifyingId="TVA">FR32123456782</TaxRegistrationId>`}}, []string{"F10-VENDEUR-G2.33b"}},
	{"transactions", []edit{{1, tSeller, `<CompanyId schemeId="0227">123456782</CompanyId>`}, {1, tSellerVAT, ""}, {1, "<Code>AE</Code>", "<Code>E</Code>"}}, []string{"F10-VENDEUR-G1.102"}},
	{"transactions", []edit{{1, "<CountryId>FR</CountryId>", "<CountryId>fr</CountryId>"}}, []string{"F10-VENDEUR-G2.01"}},

	// Buyer.
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0009">DE123456789</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19a"}},
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0002">12345678</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19b"}},
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0223">` + long19 + `</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19c"}},
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0227">` + long19 + `</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19d"}},
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0228">12345678</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19e"}},
	{"transactions", []edit{{1, tBuyer, `<CompanyId schemeId="0229">1234567890</CompanyId>`}}, []string{"F10-ACHETEUR-G2.19f"}},
	{"transactions", []edit{{1, tBuyerVAT, ""}}, []string{"F10-ACHETEUR-G2.33a"}},
	{"transactions", []edit{{1, tBuyerVAT, `<TaxRegistrationId qualifyingId="TVA">DE123456789</TaxRegistrationId>`}}, []string{"F10-ACHETEUR-G2.33b"}},
	{"transactions", []edit{{1, tCountry, "<CountryId>DEU</CountryId>"}}, []string{"F10-ACHETEUR-G2.01"}},

	// Delivery and invoicing period.
	{"transactions", []edit{{1, "<Date>20260930</Date>", "<Date>2026093</Date>"}}, []string{"F10-LIVRAISON-G1.09"}},
	{"transactions", []edit{{1, "<Date>20260930</Date>", "<Date>19990930</Date>"}}, []string{"F10-LIVRAISON-G1.09-2"}},
	{"transactions", []edit{{2, tCountry, "<CountryId>D</CountryId>"}}, []string{"F10-LIVRAISON-G2.01"}},
	{"transactions", []edit{{2, tStart, "<StartDate>2026090</StartDate>"}}, []string{"F10-PERIODE-FAC-G1.09a"}},
	{"transactions", []edit{{2, tStart, "<StartDate>19990901</StartDate>"}}, []string{"F10-PERIODE-FAC-G1.09a-2"}},
	{"transactions", []edit{{2, tEnd, "<EndDate>2026093</EndDate>"}}, []string{"F10-PERIODE-FAC-G1.09b"}},
	{"transactions", []edit{{2, tEnd, "<EndDate>21000930</EndDate>"}}, []string{"F10-PERIODE-FAC-G1.09b-2"}},
	{"transactions", []edit{{2, tEnd, "<EndDate>20260801</EndDate>"}}, []string{"F10-PERIODE-FAC-G6.25"}},

	// Allowances and charges, totals, VAT breakdown of the invoice.
	{"transactions", []edit{{1, "<TaxCategoryCode>AE</TaxCategoryCode>", "<TaxCategoryCode>L</TaxCategoryCode>"}}, []string{"F10-REMISE-G2.31"}},
	{"transactions", []edit{{1, "<TaxPercent>0</TaxPercent>", "<TaxPercent>3</TaxPercent>"}}, []string{"F10-REMISE-G1.24"}},
	{"transactions", []edit{{1, "<Amount>50.00</Amount>", "<Amount>50.001</Amount>"}}, []string{"F10-REMISE-G1.14"}},
	{"transactions", []edit{{1, "<Amount>50.00</Amount>", "<Amount>" + digits + "</Amount>"}}, []string{"F10-REMISE-G1.14-2"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>950.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>950.001</TaxExclusiveAmount>"}}, []string{"F10-TOTAL-G1.14a"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>950.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>" + digits + "</TaxExclusiveAmount>"}}, []string{"F10-TOTAL-G1.14a-2"}},
	{"transactions", []edit{{1, `<TaxAmount CurrencyCode="EUR">0.00</TaxAmount>`, `<TaxAmount CurrencyCode="USD">0.00</TaxAmount>`}}, []string{"F10-TOTAL-TAXAMOUNT"}},
	{"transactions", []edit{{1, `<TaxAmount CurrencyCode="EUR">0.00</TaxAmount>`, `<TaxAmount CurrencyCode="EUR">0.001</TaxAmount>`}}, []string{"F10-TOTAL-TAXAMOUNT-2"}},
	{"transactions", []edit{{1, `<TaxAmount CurrencyCode="EUR">0.00</TaxAmount>`, `<TaxAmount CurrencyCode="EUR">` + digits + `</TaxAmount>`}}, []string{"F10-TOTAL-TAXAMOUNT-3"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>950.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>950.02</TaxExclusiveAmount>"}}, []string{"F10-TOTAL-G1.53"}},
	{"transactions", []edit{{1, `<TaxAmount CurrencyCode="EUR">0.00</TaxAmount>`, `<TaxAmount CurrencyCode="EUR">0.02</TaxAmount>`}}, []string{"F10-TOTAL-G1.53-2"}},
	{"transactions", []edit{{1, "<Code>AE</Code>", "<Code>L</Code>"}}, []string{"F10-TVA-G2.31"}},
	{"transactions", []edit{{1, "<Percent>0</Percent>", "<Percent>3</Percent>"}}, []string{"F10-TVA-G1.24"}},
	{"transactions", []edit{{1, "<Code>AE</Code>", "<Code>E</Code>"}, {1, "<TaxExemptionReason>Autoliquidation</TaxExemptionReason>", ""}}, []string{"F10-TVA-G1.40"}},
	{"transactions", []edit{{1, "<TaxableAmount>950.00</TaxableAmount>", "<TaxableAmount>950.001</TaxableAmount>"}}, []string{"F10-TVA-G1.14a"}},
	{"transactions", []edit{{1, "<TaxableAmount>950.00</TaxableAmount>", "<TaxableAmount>" + digits + "</TaxableAmount>"}}, []string{"F10-TVA-G1.14a-2"}},
	{"transactions", []edit{{1, "<TaxAmount>0.00</TaxAmount>", "<TaxAmount>0.001</TaxAmount>"}}, []string{"F10-TVA-G1.14b"}},
	{"transactions", []edit{{1, "<TaxAmount>0.00</TaxAmount>", "<TaxAmount>" + digits + "</TaxAmount>"}}, []string{"F10-TVA-G1.14b-2"}},

	// Lines.
	{"transactions", []edit{{3, tStart, "<StartDate>2026090</StartDate>"}}, []string{"F10-LIGNE-G1.09a"}},
	{"transactions", []edit{{3, tStart, "<StartDate>19990901</StartDate>"}}, []string{"F10-LIGNE-G1.09a-2"}},
	{"transactions", []edit{{3, tEnd, "<EndDate>2026093</EndDate>"}}, []string{"F10-LIGNE-G1.09b"}},
	{"transactions", []edit{{3, tEnd, "<EndDate>21000930</EndDate>"}}, []string{"F10-LIGNE-G1.09b-2"}},
	{"transactions", []edit{{3, tEnd, "<EndDate>20260801</EndDate>"}}, []string{"F10-LIGNE-G6.25"}},
	{"transactions", []edit{{3, tCountry, "<CountryId>de</CountryId>"}}, []string{"F10-LIGNE-G2.01"}},
	{"transactions", []edit{{1, "<Amount>0.00</Amount>", "<Amount>0.001</Amount>"}}, []string{"F10-LIGNE-G1.14"}},
	{"transactions", []edit{{1, "<Amount>0.00</Amount>", "<Amount>" + digits + "</Amount>"}}, []string{"F10-LIGNE-G1.14-2"}},
	{"transactions", []edit{{1, `<BilledQuantity UnitCode="DAY">10</BilledQuantity>`, `<BilledQuantity UnitCode="DAY">10.12345</BilledQuantity>`}}, []string{"F10-LIGNE-G1.15"}},
	{"transactions", []edit{{1, `<BilledQuantity UnitCode="DAY">10</BilledQuantity>`, `<BilledQuantity UnitCode="DAY">` + digits + `</BilledQuantity>`}}, []string{"F10-LIGNE-G1.15-2"}},
	{"transactions", []edit{{1, "<PriceAmount>100.00</PriceAmount>", "<PriceAmount>-100.00</PriceAmount>"}}, []string{"F10-LIGNE-G1.16a"}},
	{"transactions", []edit{{1, "<PriceAmount>100.00</PriceAmount>", "<PriceAmount>" + digits + "</PriceAmount>"}}, []string{"F10-LIGNE-G1.16a-2"}},
	{"transactions", []edit{{1, "<AllowanceChargeAmount>0</AllowanceChargeAmount>", "<AllowanceChargeAmount>-1</AllowanceChargeAmount>"}}, []string{"F10-LIGNE-G1.16b"}},
	{"transactions", []edit{{1, "<AllowanceChargeAmount>0</AllowanceChargeAmount>", "<AllowanceChargeAmount>" + digits + "</AllowanceChargeAmount>"}}, []string{"F10-LIGNE-G1.16b-2"}},
	{"transactions", []edit{{1, "<AllowanceChargeBaseAmount>100.00</AllowanceChargeBaseAmount>", "<AllowanceChargeBaseAmount>100.1234567</AllowanceChargeBaseAmount>"}}, []string{"F10-LIGNE-G1.16c"}},
	{"transactions", []edit{{1, "<AllowanceChargeBaseAmount>100.00</AllowanceChargeBaseAmount>", "<AllowanceChargeBaseAmount>" + digits + "</AllowanceChargeBaseAmount>"}}, []string{"F10-LIGNE-G1.16c-2"}},
	{"transactions", []edit{{1, "<PriceAmount>100.00</PriceAmount>", "<PriceAmount>99.00</PriceAmount>"}}, []string{"F10-LIGNE-G1.55"}},
	{"transactions", []edit{{3, tDate, "<IssueDate>2026091</IssueDate>"}}, []string{"F10-LIGNE-G1.09c"}},
	{"transactions", []edit{{3, tDate, "<IssueDate>19990915</IssueDate>"}}, []string{"F10-LIGNE-G1.09c-2"}},
	{"transactions", []edit{{3, tID, "<ID>" + long36 + "</ID>"}}, []string{"F10-LIGNE-G1.05"}},
	{"transactions", []edit{{3, tID, "<ID> F2026-0915</ID>"}}, []string{"F10-LIGNE-G1.05-2"}},
	{"transactions", []edit{{3, tID, "<ID>F2026  0915</ID>"}}, []string{"F10-LIGNE-G1.05-3"}},
	{"transactions", []edit{{3, tID, "<ID>F2026_0915*</ID>"}}, []string{"F10-LIGNE-G1.05-4"}},

	// Aggregated transactions.
	{"transactions", []edit{{1, "<Date>20260915</Date>", "<Date>2026091</Date>"}}, []string{"F10-TRANSAC-G1.09"}},
	{"transactions", []edit{{1, "<Date>20260915</Date>", "<Date>19990915</Date>"}}, []string{"F10-TRANSAC-G1.09-2"}},
	{"transactions", []edit{{1, "<CategoryCode>TLB1</CategoryCode>", "<CategoryCode>TLB2</CategoryCode>"}}, []string{"F10-TRANSAC-G1.68"}},
	{"transactions", []edit{{2, "<TaxDueDateTypeCode>5</TaxDueDateTypeCode>", "<TaxDueDateTypeCode>4</TaxDueDateTypeCode>"}}, []string{"F10-TRANSAC-P1.11"}},
	{"transactions", []edit{{1, "<TaxPercent>20</TaxPercent>", "<TaxPercent>19</TaxPercent>"}}, []string{"F10-TRANSAC-G1.24b"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>1500.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>1500.001</TaxExclusiveAmount>"}}, []string{"F10-TRANSAC-G1.14a"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>1500.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>" + digits + "</TaxExclusiveAmount>"}}, []string{"F10-TRANSAC-G1.14a-2"}},
	{"transactions", []edit{{1, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>227.501</TaxTotal>"}}, []string{"F10-TRANSAC-G1.14b"}},
	{"transactions", []edit{{1, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>" + digits + "</TaxTotal>"}}, []string{"F10-TRANSAC-G1.14b-2"}},
	{"transactions", []edit{{1, "<TaxableAmount>1000.00</TaxableAmount>", "<TaxableAmount>1000.001</TaxableAmount>"}}, []string{"F10-TRANSAC-G1.14c"}},
	{"transactions", []edit{{1, "<TaxableAmount>1000.00</TaxableAmount>", "<TaxableAmount>" + digits + "</TaxableAmount>"}}, []string{"F10-TRANSAC-G1.14c-2"}},
	{"transactions", []edit{{1, "<TaxTotal>200.00</TaxTotal>", "<TaxTotal>200.001</TaxTotal>"}}, []string{"F10-TRANSAC-G1.14d"}},
	{"transactions", []edit{{1, "<TaxTotal>200.00</TaxTotal>", "<TaxTotal>" + digits + "</TaxTotal>"}}, []string{"F10-TRANSAC-G1.14d-2"}},
	{"transactions", []edit{{1, "<TaxExclusiveAmount>1500.00</TaxExclusiveAmount>", "<TaxExclusiveAmount>1500.03</TaxExclusiveAmount>"}}, []string{"F10-TRANSAC-G1.53"}},
	{"transactions", []edit{{1, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>227.53</TaxTotal>"}}, []string{"F10-TRANSAC-G1.53-2"}},

	// Payments report.
	{"payments", []edit{{1, pStart, "<StartDate>2026090</StartDate>"}}, []string{"F10-PERIODE-PMT-G1.09a"}},
	{"payments", []edit{{1, pStart, "<StartDate>19990901</StartDate>"}}, []string{"F10-PERIODE-PMT-G1.09a-2"}},
	{"payments", []edit{{1, pEnd, "<EndDate>2026093</EndDate>"}}, []string{"F10-PERIODE-PMT-G1.09b"}},
	{"payments", []edit{{1, pEnd, "<EndDate>21000930</EndDate>"}}, []string{"F10-PERIODE-PMT-G1.09b-2"}},
	{"payments", []edit{{1, pEnd, "<EndDate>20260801</EndDate>"}}, []string{"F10-PERIODE-PMT-G6.25"}},
	{"payments", []edit{{1, "<InvoiceID>F2026-0812</InvoiceID>", "<InvoiceID>" + long36 + "</InvoiceID>"}}, []string{"F10-PMT-FACTURE-G1.05"}},
	{"payments", []edit{{1, "<InvoiceID>F2026-0812</InvoiceID>", "<InvoiceID>F2026-0812 </InvoiceID>"}}, []string{"F10-PMT-FACTURE-G1.05-2"}},
	{"payments", []edit{{1, "<InvoiceID>F2026-0812</InvoiceID>", "<InvoiceID>F2026  0812</InvoiceID>"}}, []string{"F10-PMT-FACTURE-G1.05-3"}},
	{"payments", []edit{{1, "<InvoiceID>F2026-0812</InvoiceID>", "<InvoiceID>F2026é0812</InvoiceID>"}}, []string{"F10-PMT-FACTURE-G1.05-4"}},
	{"payments", []edit{{1, "<IssueDate>20260812</IssueDate>", "<IssueDate>2026081</IssueDate>"}}, []string{"F10-PMT-FACTURE-G1.09"}},
	{"payments", []edit{{1, "<IssueDate>20260812</IssueDate>", "<IssueDate>19990812</IssueDate>"}}, []string{"F10-PMT-FACTURE-G1.09-2"}},
	{"payments", []edit{{1, "<Date>20260910</Date>", "<Date>2026091</Date>"}}, []string{"F10-PMT-ENCAISSEMENT-G1.09"}},
	{"payments", []edit{{1, "<Date>20260910</Date>", "<Date>19990910</Date>"}}, []string{"F10-PMT-ENCAISSEMENT-G1.09-2"}},
	{"payments", []edit{{1, "<TaxPercent>20</TaxPercent>", "<TaxPercent>19</TaxPercent>"}}, []string{"F10-PMT-ENCAISSEMENT-G1.24"}},
	{"payments", []edit{{1, "<CurrencyCode>EUR</CurrencyCode>", "<CurrencyCode>USD</CurrencyCode>"}}, []string{"F10-PMT-ENCAISSEMENT-G6.27a"}},
	{"payments", []edit{{1, "<Amount>1200.00</Amount>", "<Amount>-1200.00</Amount>"}}, []string{"F10-PMT-ENCAISSEMENT-G1.16"}},
	{"payments", []edit{{1, "<Amount>1200.00</Amount>", "<Amount>" + digits + "</Amount>"}}, []string{"F10-PMT-ENCAISSEMENT-G1.16-2"}},
	{"payments", []edit{{1, "<Date>20260915</Date>", "<Date>2026091</Date>"}}, []string{"F10-PMT-AGREGE-G1.09"}},
	{"payments", []edit{{1, "<Date>20260915</Date>", "<Date>19990915</Date>"}}, []string{"F10-PMT-AGREGE-G1.09-2"}},
	{"payments", []edit{{2, "<TaxPercent>20</TaxPercent>", "<TaxPercent>19</TaxPercent>"}}, []string{"F10-PMT-AGREGE-G1.24"}},
	{"payments", []edit{{2, "<CurrencyCode>EUR</CurrencyCode>", "<CurrencyCode>USD</CurrencyCode>"}}, []string{"F10-PMT-AGREGE-G6.27b"}},
	{"payments", []edit{{1, "<Amount>360.00</Amount>", "<Amount>360.1234567</Amount>"}}, []string{"F10-PMT-AGREGE-G1.16"}},
	{"payments", []edit{{1, "<Amount>360.00</Amount>", "<Amount>" + digits + "</Amount>"}}, []string{"F10-PMT-AGREGE-G1.16-2"}},
	{"payments", []edit{{1, "<CurrencyCode>EUR</CurrencyCode>", "<CurrencyCode>eur</CurrencyCode>"}}, []string{"F10-PMT-DEVISE-G1.10"}},
	{"transactions", []edit{{1, "<CurrencyCode>EUR</CurrencyCode>", "<CurrencyCode>EU</CurrencyCode>"}}, []string{"F10-TX-DEVISE-G1.10"}},

	// Both totals of an invoice, both reports, aggregated transactions in
	// another currency, whose totals G1.53 does not check.
	{"transactions", []edit{{1, "<TaxableAmount>950.00</TaxableAmount>", "<TaxableAmount>900.00</TaxableAmount>"}, {1, "<TaxAmount>0.00</TaxAmount>", "<TaxAmount>1.00</TaxAmount>"}}, []string{"F10-TOTAL-G1.53", "F10-TOTAL-G1.53-2"}},
	{"transactions", []edit{{1, "</Report>", "<PaymentsReport/></Report>"}}, []string{"F10-TRANSMISSION-G6.29"}},
	{"transactions", []edit{{1, "<TransactionsCurrency>EUR</TransactionsCurrency>", "<TransactionsCurrency>USD</TransactionsCurrency>"}, {1, "<TaxTotal>227.50</TaxTotal>", "<TaxTotal>0</TaxTotal>"}}, nil},
}

// variant returns the document of case i.
func variant(t testing.TB, i int) string {
	t.Helper()
	c := cases[i]
	doc := load(t, c.base)
	for _, e := range c.edits {
		doc = apply(t, doc, e)
	}
	return doc
}

// TestMutations checks that each case raises exactly its rules, and that
// the cases raise every rule of the schematron.
func TestMutations(t *testing.T) {
	raised := map[string]bool{}
	for i, c := range cases {
		got := findings(t, variant(t, i))
		want := slices.Sorted(slices.Values(c.want))
		if !slices.Equal(got, want) {
			t.Errorf("case %d (%s %v): %v, want %v", i, c.base, c.edits, got, want)
		}
		for _, id := range got {
			raised[id] = true
		}
	}
	for _, r := range Catalog() {
		if !raised[r.ID] {
			t.Errorf("%s: no case raises it", r.ID)
		}
	}
}

// variantName is the file name of case i, or of a base report for i < 0.
func variantName(i int) string {
	if i < 0 {
		return []string{"base-transactions", "base-payments"}[-i-1] + ".xml"
	}
	return fmt.Sprintf("case-%03d.xml", i)
}

// TestVariants writes the two base reports and the variant of each case to
// the directory of -variants, for the XSLT of the schematron.
func TestVariants(t *testing.T) {
	if *variantsDir == "" {
		// Not a skip: the job of the official artefacts fails on any
		// skipped test.
		t.Logf("%d variants (-variants DIR writes them)", len(cases))
		return
	}
	if err := os.MkdirAll(*variantsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(i int, doc string) {
		if err := os.WriteFile(filepath.Join(*variantsDir, variantName(i)), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(-1, load(t, "transactions"))
	write(-2, load(t, "payments"))
	for i := range cases {
		write(i, variant(t, i))
	}
}

// TestReference compares the findings of the package with the SVRL reports
// that the XSLT of the schematron, run by Saxon-HE, wrote for the variants of
// TestVariants (-variants): for each document, the same assertions fail, as
// many times, with the same messages, values included. An assertion of the
// SVRL is known by its pattern, the last active-pattern before it, and its
// message.
func TestReference(t *testing.T) {
	if *variantsDir == "" {
		t.Log("-variants DIR not set: no SVRL report to compare")
		return
	}
	pattern := map[string]string{} // rule → pattern
	for _, p := range patterns {
		for _, a := range p.Rules[0].Asserts {
			pattern[a.ID] = p.ID
		}
	}
	for i := -2; i < len(cases); i++ {
		name := variantName(i)
		var doc string
		if i < 0 {
			doc = load(t, []string{"transactions", "payments"}[-i-1])
		} else {
			doc = variant(t, i)
		}
		var ours []string
		for _, f := range check(t, doc).Findings {
			ours = append(ours, pattern[f.Rule]+" "+f.Message)
		}
		data, err := os.ReadFile(filepath.Join(*variantsDir, name+".svrl"))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := svrlAssertions(data)
		if err != nil {
			t.Fatalf("%s.svrl: %v", name, err)
		}
		slices.Sort(ours)
		slices.Sort(ref)
		if !slices.Equal(ours, ref) {
			t.Errorf("%s:\nours %q\nSaxon %q", name, ours, ref)
		}
	}
}

// svrlAssertions returns the failed assertions of an SVRL report, each as
// its pattern and its message, white space normalized.
func svrlAssertions(data []byte) ([]string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var out []string
	var pat string
	var text strings.Builder
	in, root := false, ""
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root == "" {
				root = t.Name.Local
			}
			switch t.Name.Local {
			case "active-pattern":
				for _, a := range t.Attr {
					if a.Name.Local == "id" {
						pat = a.Value
					}
				}
			case "text":
				in = true
				text.Reset()
			}
		case xml.CharData:
			if in {
				text.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "text" && in {
				in = false
				out = append(out, pat+" "+strings.Join(strings.Fields(text.String()), " "))
			}
		}
	}
	if root != "schematron-output" {
		return nil, fmt.Errorf("not an SVRL report: %s", root)
	}
	return out, nil
}
