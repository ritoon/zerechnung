package atcud

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ritoon/zerechnung/invoice"
)

// Data are the data of the QR code that an invoice does not carry, or that
// Build cannot derive from it.
type Data struct {
	// ValidationCode is the validation code of the series of the invoice,
	// which the AT returns when the series is communicated to it
	// (Portaria n.º 195/2020, article 2.º): the ATCUD is this code, a
	// hyphen and the sequential number of the invoice.
	ValidationCode string
	// Hash is the field Q: the four characters of the signature of the
	// invoice that HashChars extracts. Empty, it is read from the
	// structured note #HASHCODE@ATCERTIFIEDPROGRAM# of the CIUS-PT, which
	// holds the four characters or the whole signature.
	Hash string
	// Certificate is the field R: the number of the certificate that the
	// AT assigned to the invoicing software. Empty, it is read from the
	// structured note #NUMBER@ATCERTIFIEDPROGRAM# of the CIUS-PT (9999 for
	// 9999/AT).
	Certificate string
	// DocumentType is the field D, the type of the document in SAF-T (PT).
	// Empty, it is the type code of the invoice (BT-3) when that is a type
	// of SAF-T (PT), as the CIUS-PT admits (FT, FS, FR, ND, NC…), FT for
	// 380, NC for 381 and ND for 383.
	DocumentType string
	// DocumentStatus is the field E, the status of the document in SAF-T
	// (PT). Empty, it is N, a normal document.
	DocumentStatus string
	// Region is the fiscal space of the VAT of the invoice, the field I1:
	// PT-AC or PT-MA for the autonomous regions, the code of another
	// country for a foreign VAT. Empty, it is PT.
	Region string
	// Other is the field S, free information such as the payment data:
	// TB;PT50000201231234567890154;411.40 for a transfer, MB;entity;
	// reference;amount for a Multibanco reference.
	Other string
}

// Build returns the content of the QR code of inv, from the invoice and d:
//
//   - A, the NIF of the seller (BT-31, or BT-32 without BT-31) without
//     its prefix PT;
//   - B, the VAT identifier of the buyer (BT-48) without its country
//     prefix, 999999990 (final consumer) without BT-48; C, the country of
//     the buyer (BT-55), Desconhecido without it;
//   - D and E from d; F, the issue date (BT-2); G, the invoice number
//     (BT-1); H, the ATCUD of d.ValidationCode and of the sequential
//     number of BT-1;
//   - the first fiscal space, d.Region, with the VAT breakdown (BG-23)
//     summed by rate, in the categories of the CIUS-PT: NOR or S (normal
//     rate) to I7 and I8, INT to I5 and I6, RED or AA to I3 and I4, ISE,
//     E, Z, AE, K, G or IC (exempt) to I2; O, OUT or NA (not subject, or
//     a margin scheme) to L;
//   - N, the total VAT (BT-110, or the sum of the breakdown without it);
//     O, the total with VAT (BT-112); P, the sum of the withholding taxes
//     of the structured notes #WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-nnn# of
//     the CIUS-PT;
//   - Q, R and S from d, or from the structured notes of the CIUS-PT.
//
// The amounts of the QR code are in euros (section 3 c): Build refuses an
// invoice in another currency. The CIUS-PT reads category S as the normal
// rate and AA as the reduced one: a breakdown of category S at the reduced
// or intermediate rate goes to I7 and I8, give it category RED or INT.
// Stamp duty (M) and the other fiscal spaces are not in the invoice: set
// them in the result, then call its Check.
//
// Build returns the content with the error, which joins the categories it
// cannot place and the findings of Check.
func Build(inv *invoice.Invoice, d *Data) (*QR, error) {
	if inv == nil {
		return nil, errors.New("atcud: no invoice")
	}
	if d == nil {
		d = new(Data)
	}
	if inv.Currency != "" && inv.Currency != "EUR" {
		return nil, fmt.Errorf("atcud: the invoice is in %s: the amounts of the QR code are in euros (section 3 c)", inv.Currency)
	}
	var errs []error
	q := &QR{
		IssuerNIF:      sellerNIF(&inv.Seller),
		BuyerTaxID:     "999999990",
		BuyerCountry:   inv.Buyer.Address.CountryCode,
		DocumentType:   d.DocumentType,
		DocumentStatus: d.DocumentStatus,
		Date:           inv.IssueDate,
		DocumentID:     inv.Number,
		Hash:           d.Hash,
		Certificate:    d.Certificate,
		Other:          d.Other,
	}
	if id := inv.Buyer.VATID; id != "" {
		q.BuyerTaxID = stripCountry(id)
	}
	if q.BuyerCountry == "" {
		q.BuyerCountry = "Desconhecido"
	}
	if q.DocumentType == "" {
		q.DocumentType = documentType(inv.TypeCode)
	}
	if q.DocumentStatus == "" {
		q.DocumentStatus = "N"
	}
	if n, ok := sequentialNumber(inv.Number); ok && d.ValidationCode != "" {
		q.ATCUD = Code{ValidationCode: d.ValidationCode, Number: n}.String()
	}

	s := &q.Spaces[0]
	s.Region = d.Region
	if s.Region == "" {
		s.Region = "PT"
	}
	add := func(dst *invoice.Decimal, v invoice.Decimal) {
		if !dst.IsSet() {
			*dst = invoice.DecimalFromInt(0)
		}
		*dst = dst.Add(v)
	}
	var taxes invoice.Decimal
	for i, b := range inv.VATBreakdown {
		add(&taxes, b.TaxAmount)
		switch strings.ToUpper(strings.TrimSpace(b.CategoryCode)) {
		case "NOR", invoice.VATStandard:
			add(&s.NormalBase, b.TaxableAmount)
			add(&s.NormalVAT, b.TaxAmount)
		case "INT":
			add(&s.IntermediateBase, b.TaxableAmount)
			add(&s.IntermediateVAT, b.TaxAmount)
		case "RED", "AA":
			add(&s.ReducedBase, b.TaxableAmount)
			add(&s.ReducedVAT, b.TaxAmount)
		case "ISE", invoice.VATExempt, invoice.VATZeroRated, invoice.VATReverseCharge,
			invoice.VATIntraCommunity, invoice.VATExport, "IC":
			add(&s.ExemptBase, b.TaxableAmount)
		case "OUT", invoice.VATOutOfScope, "NA":
			add(&q.NotSubject, b.TaxableAmount)
		default:
			errs = append(errs, &FieldError{Field: "I1", Msg: fmt.Sprintf(
				"the VAT category %q of breakdown %d (BT-118) has no field in the QR code", b.CategoryCode, i+1)})
		}
	}
	q.TaxTotal = inv.Totals.TaxTotal
	if !q.TaxTotal.IsSet() {
		q.TaxTotal = taxes
	}
	q.GrossTotal = inv.Totals.GrandTotal

	for _, n := range inv.Notes {
		name, v, ok := structuredNote(n.Content)
		switch {
		case !ok:
		case name == "HASHCODE@ATCERTIFIEDPROGRAM" && q.Hash == "":
			if len(v) == 4 {
				q.Hash = v
			} else if h, err := HashChars(v); err == nil {
				q.Hash = h
			}
		case name == "NUMBER@ATCERTIFIEDPROGRAM" && q.Certificate == "":
			q.Certificate = strings.TrimSuffix(v, "/AT")
		case strings.HasPrefix(name, "WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-"):
			x, err := invoice.ParseDecimal(v)
			if err != nil {
				errs = append(errs, &FieldError{Field: "P", Msg: fmt.Sprintf("the withholding tax %q of note %s is not an amount", v, name)})
				continue
			}
			add(&q.Withholding, x)
		}
	}
	if err := q.Check(); err != nil {
		errs = append(errs, err)
	}
	return q, errors.Join(errs...)
}

// HashChars returns the four characters of signature, the signature of a
// document in Base64 (field Hash of SAF-T (PT)), that the document prints
// and the QR code holds in field Q: those at the 1st, 11th, 21st and 31st
// positions (Portaria n.º 363/2010, article 6.º, n.º 3, a)).
func HashChars(signature string) (string, error) {
	if len(signature) < 31 || !isBase64(strings.TrimRight(signature, "=")) {
		return "", &FieldError{Field: "Q", Msg: "the signature is not a Base64 string of 31 characters at least"}
	}
	return string([]byte{signature[0], signature[10], signature[20], signature[30]}), nil
}

// sellerNIF returns the VAT identifier of the seller (BT-31), or its tax
// registration identifier (BT-32) without one, without the prefix PT.
func sellerNIF(p *invoice.Party) string {
	id := p.VATID
	if id == "" {
		id = p.TaxRegistrationID
	}
	return strings.TrimPrefix(strings.TrimSpace(id), "PT")
}

// stripCountry returns a VAT identifier without its country prefix, the
// two letters it starts with.
func stripCountry(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 2 && isLetter(id[0]) && isLetter(id[1]) {
		return id[2:]
	}
	return id
}

func isLetter(c byte) bool { return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' }

// documentType returns the type of SAF-T (PT) of an invoice type code
// (BT-3): the code itself when it is one, the type of the UNTDID 1001 codes
// that the CIUS-PT admits beside them, or "" for the others.
func documentType(code string) string {
	switch code {
	case invoice.TypeCommercialInvoice:
		return "FT"
	case invoice.TypeCreditNote:
		return "NC"
	case "383":
		return "ND"
	}
	if isCapitals(code, 2) {
		return code
	}
	return ""
}

// structuredNote splits a structured note of the CIUS-PT, #NAME#value#,
// into its name and its value.
func structuredNote(s string) (name, value string, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) < 3 || s[0] != '#' || s[len(s)-1] != '#' {
		return "", "", false
	}
	name, value, ok = strings.Cut(s[1:len(s)-1], "#")
	return name, value, ok
}
