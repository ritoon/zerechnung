package invoice

import (
	"reflect"
	"strconv"
)

// Croatia holds the data of the Croatian CIUS and of its extension (CIUS-HR
// and EXT-HR 2025, Fiskalizacija 2.0: Specifikacija osnovne uporabe eRačuna
// s proširenjima, version 1.8 of the Croatian tax administration) that the
// EN 16931 fields of the model do not carry. It is the national block of the
// invoice; AllowanceCharge.Croatia, Line.Croatia and
// DocumentReference.Croatia hold the Croatian data of these values. Package
// ubl reads and writes these blocks in an invoice whose specification
// identifier (BT-24) is the Croatian one (see ProfileCIUSHR and URNCIUSHR);
// package cii does not carry them (see NationalData).
//
// The Croatian data that EN 16931 carries goes to the other fields of the
// model: the copy indicator (HR-BT-1) to CopyIndicator, the purchase order
// referenced by a line (HR-BT-9) to Line.OrderReference.
type Croatia struct {
	// IssueTime is the invoice issue time (HR-BT-2, cbc:IssueTime) that the
	// law on fiscalisation requires, as written in the XML (hh:mm:ss).
	IssueTime string
	// OperatorName and OperatorOIB are the operator label (HR-BT-4) and the
	// OIB of the operator (HR-BT-5) who issued the invoice, as the law on
	// fiscalisation defines them (cac:AccountingSupplierParty/
	// cac:SellerContact/cbc:Name and cbc:ID).
	OperatorName string
	OperatorOIB  string

	// CashAccounting is the text of the indicator "Obračun PDV-a po
	// naplaćenoj naknadi" (HR-BT-15, hrextac:HRObracunPDVPoNaplati): the
	// seller pays the VAT when the price is paid (cash accounting).
	CashAccounting string

	// The Croatian VAT breakdown and totals (extension HRFISK20Data): they
	// add to the EN 16931 ones the amounts out of the scope of VAT, such as
	// the consumption tax or a refundable deposit, which the HR categories
	// of the items and charges tell apart (see CroatiaTax).
	TaxTotal         Decimal      // HR-BT-25 HR invoice total VAT amount (hrextac:HRTaxTotal/cbc:TaxAmount)
	VATBreakdown     []CroatiaVAT // HR-BG-2 HR VAT breakdown (hrextac:HRTaxSubtotal)
	TaxBasisTotal    Decimal      // HR-BT-23 HR total amount subject to VAT (hrextac:HRLegalMonetaryTotal/cbc:TaxExclusiveAmount)
	OutOfScopeAmount Decimal      // HR-BT-24 HR amount out of the scope of VAT (hrextac:OutOfScopeOfVATAmount)
}

// CroatiaVAT is an entry of the Croatian VAT breakdown (HR-BG-2,
// hrextac:HRTaxSubtotal).
type CroatiaVAT struct {
	TaxableAmount       Decimal // HR-BT-16 HR VAT category taxable amount
	TaxAmount           Decimal // HR-BT-17 HR VAT category tax amount
	CategoryCode        string  // HR-BT-18 HR VAT category code (UNTDID 5305: S, Z, E, AE, K, G, O)
	Category            string  // HR-BT-22 HR VAT category label (table HR-TB-2: HR:PDV25, HR:PP…)
	Rate                Decimal // HR-BT-19 HR VAT category rate
	ExemptionReasonCode string  // HR-BT-21 HR VAT exemption reason code (table HR-TB-1)
	ExemptionReason     string  // HR-BT-20 HR VAT exemption reason text
	// TaxScheme is the identifier of the tax scheme (hrextac:HRTaxScheme):
	// VAT, or the scheme of a tax out of the scope of VAT (LOC for the
	// consumption tax of the official examples).
	TaxScheme string
}

// CroatiaTax is the Croatian data of the VAT category of a document level
// allowance or charge (HR-BT-6 to HR-BT-8) or of an invoice line (HR-BT-12
// to HR-BT-14, see CroatiaLine).
type CroatiaTax struct {
	// Category is the HR VAT category label (HR-BT-6, HR-BT-12, cbc:Name of
	// the category, table HR-TB-2): HR:PDV25 for the standard rate of 25 %,
	// HR:PP for the consumption tax, HR:POVNAK for a refundable deposit…
	Category            string
	ExemptionReason     string // HR-BT-7, HR-BT-13 VAT exemption reason text
	ExemptionReasonCode string // HR-BT-8, HR-BT-14 VAT exemption reason code (table HR-TB-1)
}

// CroatiaLine is the Croatian data of an invoice line.
type CroatiaLine struct {
	// Category, ExemptionReason and ExemptionReasonCode are the Croatian
	// data of the VAT category of the item (HR-BT-12 to HR-BT-14, in its
	// cac:ClassifiedTaxCategory), as in CroatiaTax.
	Category            string
	ExemptionReason     string
	ExemptionReasonCode string

	// DespatchLine and ReceiptLine are the despatch advice (HR-BT-10) and
	// the receiving advice (HR-BT-11) referenced by the line, with the line
	// they refer to (cac:DespatchLineReference, cac:ReceiptLineReference:
	// ID and LineID).
	DespatchLine *DocumentReference
	ReceiptLine  *DocumentReference
}

// CroatiaReference is the Croatian data of a preceding invoice reference
// (BG-3).
type CroatiaReference struct {
	// Description is the description of the preceding invoice (HR-BT-3,
	// cbc:DocumentDescription), which may give its VAT.
	Description string
}

// nationalCroatia appends to out the data of the Croatian blocks, as
// NationalData lists it.
func (inv *Invoice) nationalCroatia(out []string) []string {
	add := func(prefix string, v any) {
		out = appendSet(out, "EXT "+prefix+".", reflect.ValueOf(v).Elem())
	}
	if inv.Croatia != nil {
		add("Croatia", inv.Croatia)
	}
	for i := range inv.PrecedingInvoices {
		if c := inv.PrecedingInvoices[i].Croatia; c != nil {
			add("PrecedingInvoices["+strconv.Itoa(i)+"].Croatia", c)
		}
	}
	for i := range inv.AllowancesCharges {
		if c := inv.AllowancesCharges[i].Croatia; c != nil {
			add("AllowancesCharges["+strconv.Itoa(i)+"].Croatia", c)
		}
	}
	for i := range inv.Lines {
		if c := inv.Lines[i].Croatia; c != nil {
			add("Lines["+strconv.Itoa(i)+"].Croatia", c)
		}
	}
	return out
}
