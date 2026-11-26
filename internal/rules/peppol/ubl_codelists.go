package peppol

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rules of the pattern "Restricted code lists and formatting" of the UBL
// schematron. The MIME codes, currencies, reason codes and electronic
// address schemes are the lists of the CII schematron.

var (
	uncl2005 = set("3 35 432")

	// ublInvoiceTypes and ublCreditNoteTypes are the type codes of profiles
	// 01 and 02 (PEPPOL-EN16931-P0100, PEPPOL-EN16931-P0101).
	ublInvoiceTypes = sync.OnceValue(func() map[string]bool {
		return set("71 80 82 84 102 218 219 326 331 380 382 383 384 386 388 389 393 395 553 575 623 780 817 870 875 876 877")
	})
	ublCreditNoteTypes = sync.OnceValue(func() map[string]bool { return set("381 396 81 83 532") })

	// sbInvoiceTypes and sbCreditNoteTypes are the type codes of profile 01
	// of Peppol BIS Self-Billing (PEPPOL-EN16931-P0100, P0101 of
	// PEPPOL-EN16931-UBL-SB.sch): self-billed invoice and debit note, and
	// self-billed credit note.
	sbInvoiceTypes    = set("389 527")
	sbCreditNoteTypes = set("261")

	// exemptionCategories maps a VAT exemption reason code (upper-cased) to
	// the rule that checks the VAT category, and the category it requires.
	exemptionCategories = map[string]struct{ id, category string }{
		"VATEX-EU-G":  {"PEPPOL-EN16931-P0104", "G"},
		"VATEX-EU-O":  {"PEPPOL-EN16931-P0105", "O"},
		"VATEX-EU-IC": {"PEPPOL-EN16931-P0106", "K"},
		"VATEX-EU-AE": {"PEPPOL-EN16931-P0107", "AE"},
		"VATEX-EU-D":  {"PEPPOL-EN16931-P0108", "E"},
		"VATEX-EU-F":  {"PEPPOL-EN16931-P0109", "E"},
		"VATEX-EU-I":  {"PEPPOL-EN16931-P0110", "E"},
		"VATEX-EU-J":  {"PEPPOL-EN16931-P0111", "E"},
	}
)

// isAmount reports whether n is one of the amounts whose currency
// PEPPOL-EN16931-CL007 checks.
func isAmount(n *xmltree.Node) bool {
	if n.Space != nsCbc {
		return false
	}
	switch n.Name {
	case "Amount", "BaseAmount", "PriceAmount", "TaxAmount", "TaxableAmount", "LineExtensionAmount", "TaxExclusiveAmount",
		"TaxInclusiveAmount", "AllowanceTotalAmount", "ChargeTotalAmount", "PrepaidAmount", "PayableRoundingAmount", "PayableAmount":
		return true
	}
	return false
}

// isDate reports whether n is one of the dates PEPPOL-EN16931-F001 checks.
func isDate(n *xmltree.Node) bool {
	if n.Space != nsCbc {
		return false
	}
	switch n.Name {
	case "IssueDate", "DueDate", "TaxPointDate", "StartDate", "EndDate", "ActualDeliveryDate":
		return true
	}
	return false
}

// codeList checks n against the rules of the code list pattern.
func (k *ublChecker) codeList(n *xmltree.Node) {
	switch {
	case uis(n, "cbc:EmbeddedDocumentBinaryObject"):
		if m, ok := n.LookupAttr("mimeCode"); ok {
			k.check(mimeCodes()[m], "PEPPOL-EN16931-CL001", n, "value: %q", m)
		}
	case uis(n, "cbc:AllowanceChargeReasonCode") && uparent(n, "cac:AllowanceCharge"):
		v := norm(n.Text)
		switch {
		case uanyEq(n.Parent, "cbc:ChargeIndicator", "false"):
			k.check(uncl5189()[v], "PEPPOL-EN16931-CL002", n, "value: %q", v)
		case uanyEq(n.Parent, "cbc:ChargeIndicator", "true"):
			k.check(uncl7161()[v], "PEPPOL-EN16931-CL003", n, "value: %q", v)
		}
	case uis(n, "cbc:DescriptionCode") && uparent(n, "cac:InvoicePeriod"):
		v := norm(n.Text)
		k.check(uncl2005[v], "PEPPOL-EN16931-CL006", n, "value: %q", v)
	case isAmount(n):
		c, ok := n.LookupAttr("currencyID")
		k.check(ok && iso4217()[c], "PEPPOL-EN16931-CL007", n, "value: %q", c)
	case uis(n, "cbc:InvoiceTypeCode") && k.sb:
		v := norm(n.Text)
		k.check(k.profile != "01" || sbInvoiceTypes[v], "PEPPOL-EN16931-P0100", n, "value: %q", v)
	case uis(n, "cbc:InvoiceTypeCode"):
		v := norm(n.Text)
		k.check(k.profile != "01" && k.profile != "02" || ublInvoiceTypes()[v], "PEPPOL-EN16931-P0100", n, "value: %q", v)
		k.check(!in(v, "326", "384", "389") || k.supplierDE && k.customerDE ||
			v == "384" && k.supplierCountry == "SK" && k.customerCountry == "SK", "PEPPOL-EN16931-P0112", n,
			"value: %q, seller in %s, buyer in %s", v, k.supplierCountry, k.customerCountry)
	case uis(n, "cbc:CreditNoteTypeCode"):
		v := norm(n.Text)
		if k.sb {
			k.check(k.profile != "01" || sbCreditNoteTypes[v], "PEPPOL-EN16931-P0101", n, "value: %q", v)
		} else {
			k.check(k.profile != "01" && k.profile != "02" || ublCreditNoteTypes()[v], "PEPPOL-EN16931-P0101", n, "value: %q", v)
		}
	case isDate(n):
		_, ok := xsDate(sv(n))
		k.check(length(n.Text) == 10 && ok, "PEPPOL-EN16931-F001", n, "value: %q", n.Text)
	case uis(n, "cbc:EndpointID"):
		if s, ok := n.LookupAttr("schemeID"); ok {
			k.check(eas()[s], "PEPPOL-EN16931-CL008", n, "scheme %q", s)
		}
	case uis(n, "cac:TaxCategory"):
		if r, ok := exemptionCategories[strings.ToUpper(sv(ufirst(n, "cbc:TaxExemptionReasonCode")))]; ok {
			v := nsv(ufirst(n, "cbc:ID"))
			k.check(v == r.category, r.id, n, "category %q", v)
		}
	}
}

// xsDate implements the cast of a string to xs:date: an optional minus
// sign, a year of four digits or more (without leading zero beyond four), a
// month, a day and an optional time zone, surrounding white space ignored.
// The year 0000 is accepted (XSD 1.1). It returns the start of the day as
// minutes since the Unix epoch, in UTC; a date without time zone is taken
// in UTC.
func xsDate(s string) (int64, bool) {
	s = trim(s)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	i := strings.IndexByte(s, '-')
	if i < 4 || i > 4 && s[0] == '0' || !allDigits(s[:i]) || len(s) < i+6 || s[i+3] != '-' {
		return 0, false
	}
	year, err := strconv.ParseInt(s[:i], 10, 32)
	if err != nil {
		return 0, false
	}
	if neg {
		year = -year
	}
	mm, dd, tz := s[i+1:i+3], s[i+4:i+6], s[i+6:]
	if !digits(mm, 2) || !digits(dd, 2) {
		return 0, false
	}
	month, _ := strconv.Atoi(mm)
	day, _ := strconv.Atoi(dd)
	offset := 0
	switch {
	case tz == "" || tz == "Z":
	case len(tz) == 6 && (tz[0] == '+' || tz[0] == '-') && tz[3] == ':' && digits(tz[1:3], 2) && digits(tz[4:], 2):
		h, _ := strconv.Atoi(tz[1:3])
		m, _ := strconv.Atoi(tz[4:])
		if m > 59 || h > 14 || h == 14 && m > 0 {
			return 0, false
		}
		offset = h*60 + m
		if tz[0] == '-' {
			offset = -offset
		}
	default:
		return 0, false
	}
	if month < 1 || month > 12 || day < 1 {
		return 0, false
	}
	t := time.Date(int(year), time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day {
		return 0, false // past the end of the month
	}
	return t.Unix()/60 - int64(offset), true
}
