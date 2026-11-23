package cdar_test

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ritoon/zerechnung/cdar"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/validate"
)

// The platform of the seller emits the status 200 Déposée of an invoice it
// accepted, writes it and checks it against the CDAR schema and the rules
// BR-FR-CDV.
func ExampleDeposited() {
	data, err := os.ReadFile("../internal/rules/fr/testdata/en16931.xml")
	if err != nil {
		log.Fatal(err)
	}
	inv, err := cii.Parse(data)
	if err != nil {
		log.Fatal(err)
	}
	received := time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)
	m, err := cdar.Deposited(inv, &cdar.BuildOptions{Time: received.Add(5 * time.Minute), Received: received})
	if err != nil {
		log.Fatal(err)
	}
	out, err := cdar.Marshal(m, &cdar.MarshalOptions{Indent: true})
	if err != nil {
		log.Fatal(err)
	}
	st := m.Acknowledgements[0].Invoices[0]
	fmt.Println(m.ID)
	fmt.Println(st.Code, st.Label, "to", len(m.Recipients), "recipients")
	fmt.Println("valid:", validate.XML(out, nil).Valid())
	// Output:
	// FA-2026-00042_200_20261015093000#380_20261015
	// 200 Déposée to 2 recipients
	// valid: true
}

// A platform reads a status it received.
func ExampleParse() {
	data, err := os.ReadFile("testdata/UC3_F202500005_04-CDV-210_Refusee.xml")
	if err != nil {
		log.Fatal(err)
	}
	m, err := cdar.Parse(data)
	if err != nil {
		log.Fatal(err)
	}
	st := m.Acknowledgements[0].Invoices[0]
	fmt.Println(st.InvoiceID, st.Code, st.Label)
	for _, d := range st.Details {
		fmt.Println(d.Sequence, d.ReasonCode)
	}
	// Output:
	// F202500005 210 Refusée
	// 1 TX_TVA_ERR
}
