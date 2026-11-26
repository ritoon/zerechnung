package qrbill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// Data are the data of the QR-bill that an invoice does not carry, or
// that replace what Build derives from it.
type Data struct {
	// Account is the IBAN or QR-IBAN of the creditor. Empty, it is an IBAN
	// of Switzerland or Liechtenstein of the payment instructions of the
	// invoice (BT-84), spaces removed: the first QR-IBAN when the reference
	// is a QR reference, the first other IBAN otherwise, the first of
	// either when there is no other.
	Account string
	// Reference is the payment reference: a QR reference with a QR-IBAN
	// (see NewQRReference), a Creditor Reference with an IBAN (see
	// NewCreditorReference). Empty, it is the remittance information of the
	// invoice (BT-83) when that is a QR reference or a Creditor Reference,
	// spaces removed. An IBAN without reference makes a QR-bill of type NON.
	Reference string
	// Message is the unstructured message, the payment purpose. Empty, it
	// is the remittance information (BT-83) when that is not the reference.
	Message string
	// NoBillInformation leaves out the billing information in the syntax
	// S1 that Build writes from the invoice.
	NoBillInformation bool
	// AlternativeSchemes are the parameters of at most two alternative
	// procedures, such as eBill/B/simon.muster@example.com.
	AlternativeSchemes []string
}

// Build returns the content of the Swiss QR Code of inv, from the invoice
// and d, under version 2.4 of the guidelines, whose QR-bills version 2.3
// also admits:
//
//   - the account, see Data.Account; the creditor, its name the name of the
//     account (BT-85), or of the payee (BT-59), or of the seller (BT-27),
//     its address that of the payee when the invoice gives one (EXT), of
//     the seller (BG-5) otherwise;
//   - the amount due (BT-115, or BT-112 without it) and the currency of the
//     invoice (BT-5), CHF or EUR; an invoice with nothing to pay makes a
//     notification, of amount 0.00 and of message "DO NOT USE FOR PAYMENT"
//     in the language of the invoice, German, French, Italian or Romansh,
//     English otherwise (chapter 4.4);
//   - the buyer (BG-7) as ultimate debtor, when its address has a postal
//     code, a town and a country: the debtor fills in an incomplete one by
//     hand on the payment part;
//   - the reference and the message, see Data;
//   - the billing information in the syntax S1 (see S1): the number (BT-1)
//     and date (BT-2) of the invoice, the UID of the seller (BT-31 or BT-32
//     of the form CHE-123.456.789), the period of the service (BG-14), or
//     the delivery date (BT-72), or the VAT point date (BT-7), the rates of
//     the VAT breakdown (BG-23) with their taxable amounts when there are
//     several, category O left out, and the days between the issue date
//     and the due date (BT-9) as a condition without discount.
//
// An address line of EN 16931 holds the street and the building number
// together: Build puts it in the street and leaves the building number
// empty, which the guidelines tolerate (chapter 4.3.1, note 2); set
// BuildingNumber in the result to split them.
//
// Build returns the content with the error, which joins what it cannot
// derive from the invoice and the findings of Check.
func Build(inv *invoice.Invoice, d *Data) (*Bill, error) {
	if inv == nil {
		return nil, errors.New("qrbill: no invoice")
	}
	if d == nil {
		d = new(Data)
	}
	var errs []error
	fail := func(line int, format string, args ...any) {
		errs = append(errs, &FieldError{Line: line, Element: elements[line], Msg: fmt.Sprintf(format, args...)})
	}
	b := &Bill{Currency: inv.Currency, Message: d.Message, AlternativeSchemes: d.AlternativeSchemes}

	remittance := strings.ToUpper(strings.ReplaceAll(inv.RemittanceInformation, " ", ""))
	isRef := validQRReference(remittance) || validCreditorReference(remittance)
	b.Reference = d.Reference
	if b.Reference == "" && isRef {
		b.Reference = remittance
	}
	if b.Message == "" && !isRef {
		b.Message = inv.RemittanceInformation
	}

	accountName := ""
	b.Account = d.Account
	if b.Account == "" {
		b.Account, accountName = account(inv, validQRReference(b.Reference))
		if b.Account == "" {
			fail(lineIBAN, "the invoice has no IBAN of Switzerland or Liechtenstein (BT-84): set Data.Account")
		}
	}
	switch {
	case IsQRIBAN(b.Account), b.Account == "" && validQRReference(b.Reference):
		b.ReferenceType = QRReference
		if b.Reference == "" {
			fail(lineRef, "a QR-IBAN needs a QR reference: set Data.Reference, see NewQRReference")
		}
	case b.Reference != "":
		b.ReferenceType = CreditorReference
	default:
		b.ReferenceType = NoReference
	}

	b.Creditor = creditor(inv, accountName)
	b.Debtor = debtor(&inv.Buyer)

	amount := inv.Totals.DuePayable
	if !amount.IsSet() {
		amount = inv.Totals.GrandTotal
	}
	switch {
	case amount.Sign() < 0:
		fail(lineAmount, "the amount due is %s (BT-115): a QR-bill asks for a payment, which a credit note does not", amount)
	case amount.IsSet() && amount.IsZero():
		b.Amount = invoice.NewDecimal(0, 2)
		if d.Message == "" {
			msg, ok := Notification(inv.Language)
			if !ok {
				msg, _ = Notification("en")
			}
			b.Message = msg
		}
	case amount.IsSet():
		b.Amount = amount.Round(2)
	}

	if !d.NoBillInformation {
		if s := billInformation(inv).String(); s != "//S1" {
			b.BillInformation = s
		}
	}
	if err := b.Check(); err != nil {
		reported := make(map[int]bool, len(errs))
		for _, e := range errs {
			reported[e.(*FieldError).Line] = true
		}
		for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
			if fe, ok := e.(*FieldError); !ok || !reported[fe.Line] {
				errs = append(errs, e)
			}
		}
	}
	return b, errors.Join(errs...)
}

// account returns an IBAN of Switzerland or Liechtenstein of the payment
// instructions of inv, a QR-IBAN first if qr, another first otherwise,
// and the name of its account (BT-85).
func account(inv *invoice.Invoice, qr bool) (iban, name string) {
	found := false
	for _, pm := range inv.PaymentMeans {
		v := strings.ToUpper(strings.ReplaceAll(pm.PayeeIBAN, " ", ""))
		if !strings.HasPrefix(v, "CH") && !strings.HasPrefix(v, "LI") {
			continue
		}
		if IsQRIBAN(v) == qr {
			return v, pm.PayeeAccountName
		}
		if !found {
			iban, name, found = v, pm.PayeeAccountName, true
		}
	}
	return iban, name
}

// creditor returns the creditor of the invoice: see Build.
func creditor(inv *invoice.Invoice, accountName string) Address {
	name, addr := inv.Seller.Name, inv.Seller.Address
	if p := inv.Payee; p != nil {
		if p.Name != "" {
			name = p.Name
		}
		if !p.Address.IsZero() {
			addr = p.Address
		}
	}
	if accountName != "" {
		name = accountName
	}
	return address(name, &addr)
}

// debtor returns the buyer as ultimate debtor, nil when its address lacks
// a postal code, a town or a country.
func debtor(p *invoice.Party) *Address {
	a := &p.Address
	if p.Name == "" || a.PostCode == "" || a.City == "" || a.CountryCode == "" {
		return nil
	}
	addr := address(p.Name, a)
	return &addr
}

func address(name string, a *invoice.Address) Address {
	return Address{Name: name, Street: a.Line1, PostCode: a.PostCode, Town: a.City, Country: a.CountryCode}
}

// billInformation returns the billing information S1 of inv: see Build.
func billInformation(inv *invoice.Invoice) *S1 {
	s := &S1{InvoiceNumber: inv.Number, InvoiceDate: inv.IssueDate}
	for _, id := range []string{inv.Seller.VATID, inv.Seller.TaxRegistrationID} {
		if uid := swissUID(id); uid != "" {
			s.VATNumber = uid
			break
		}
	}
	switch p := inv.InvoicingPeriod; {
	case !p.Start.IsZero() && !p.End.IsZero():
		s.VATDate, s.VATEndDate = p.Start, p.End
	case !inv.DeliveryDate.IsZero():
		s.VATDate = inv.DeliveryDate
	default:
		s.VATDate = inv.TaxPointDate
	}
	for _, v := range inv.VATBreakdown {
		if strings.EqualFold(strings.TrimSpace(v.CategoryCode), "O") {
			continue
		}
		rate := v.Rate
		if !rate.IsSet() {
			rate = invoice.DecimalFromInt(0)
		}
		s.VATRates = append(s.VATRates, VATRate{Rate: rate, Amount: v.TaxableAmount})
	}
	if len(s.VATRates) == 1 {
		s.VATRates[0].Amount = invoice.Decimal{} // the rate of the whole amount
	}
	for _, t := range inv.PaymentTerms {
		if t.DueDate.IsZero() || inv.IssueDate.IsZero() || t.DueDate.Before(inv.IssueDate) {
			continue
		}
		days := int(t.DueDate.Time(nil).Sub(inv.IssueDate.Time(nil)).Hours() / 24)
		s.Conditions = []Discount{{Percent: invoice.DecimalFromInt(0), Days: days}}
		break
	}
	return s
}

// swissUID returns the nine digits of a Swiss UID, CHE-123.456.789 with or
// without separators and suffix (MWST, TVA, IVA, VAT), or "".
func swissUID(id string) string {
	var digits strings.Builder
	v := strings.ToUpper(id)
	rest, ok := strings.CutPrefix(v, "CHE")
	if !ok {
		return ""
	}
	for _, suffix := range []string{"MWST", "TVA", "IVA", "VAT"} {
		rest = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), suffix))
	}
	for _, r := range rest {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '-' || r == '.' || r == ' ':
		default:
			return ""
		}
	}
	if digits.Len() != 9 {
		return ""
	}
	return digits.String()
}
