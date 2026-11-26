package ereporting_test

import (
	"fmt"
	"time"

	"github.com/ritoon/zerechnung/ereporting"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

// This example writes the transactions report of a month with the daily
// totals of the sales to consumers, and checks it.
func ExampleMarshal() {
	rep := &ereporting.Report{
		Document: ereporting.Document{ID: "TR-2026-09-0001", IssueDateTime: time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC), TypeCode: "IN",
			Sender: ereporting.Party{ID: "0042", SchemeID: "0238", Name: "Plateforme agréée", RoleCode: "WK"},
			Issuer: ereporting.Party{ID: "123456782", SchemeID: "0002", Name: "Fromagerie du Val", RoleCode: "SE"}},
		Transactions: &ereporting.TransactionsReport{
			Period: ereporting.Period{Start: invoice.NewDate(2026, 9, 1), End: invoice.NewDate(2026, 9, 30)},
			Transactions: []ereporting.TransactionTotals{{
				Date: invoice.NewDate(2026, 9, 30), Currency: "EUR", TaxDueDateTypeCode: "72", Category: "TLB1",
				TaxExclusiveAmount: invoice.MustDecimal("100.00"), TaxTotal: invoice.MustDecimal("20.00"),
				TaxSubtotals: []ereporting.RateTotal{{Rate: invoice.MustDecimal("20"),
					TaxableAmount: invoice.MustDecimal("100.00"), TaxTotal: invoice.MustDecimal("20.00")}},
			}},
		},
	}
	xml, err := ereporting.Marshal(rep, &ereporting.MarshalOptions{Indent: true})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", xml)
	// The schemas of the flux 10 and the rules of the PPF (schematron
	// Flux10).
	r := validate.XML(xml, nil)
	fmt.Println("valid:", r.Valid(), "findings:", len(r.Findings))
	// Output:
	// <?xml version="1.0" encoding="UTF-8"?>
	// <Report>
	//   <ReportDocument>
	//     <Id>TR-2026-09-0001</Id>
	//     <IssueDateTime>
	//       <DateTimeString>20261011093000</DateTimeString>
	//     </IssueDateTime>
	//     <TypeCode>IN</TypeCode>
	//     <Sender>
	//       <Id schemeId="0238">0042</Id>
	//       <Name>Plateforme agréée</Name>
	//       <RoleCode>WK</RoleCode>
	//     </Sender>
	//     <Issuer>
	//       <Id schemeId="0002">123456782</Id>
	//       <Name>Fromagerie du Val</Name>
	//       <RoleCode>SE</RoleCode>
	//     </Issuer>
	//   </ReportDocument>
	//   <TransactionsReport>
	//     <ReportPeriod>
	//       <StartDate>20260901</StartDate>
	//       <EndDate>20260930</EndDate>
	//     </ReportPeriod>
	//     <Transactions>
	//       <Date>20260930</Date>
	//       <TransactionsCurrency>EUR</TransactionsCurrency>
	//       <TaxDueDateTypeCode>72</TaxDueDateTypeCode>
	//       <CategoryCode>TLB1</CategoryCode>
	//       <TaxExclusiveAmount>100.00</TaxExclusiveAmount>
	//       <TaxTotal>20.00</TaxTotal>
	//       <TaxSubtotal>
	//         <TaxPercent>20</TaxPercent>
	//         <TaxableAmount>100.00</TaxableAmount>
	//         <TaxTotal>20.00</TaxTotal>
	//       </TaxSubtotal>
	//     </Transactions>
	//   </TransactionsReport>
	// </Report>
	// valid: true findings: 0
}
