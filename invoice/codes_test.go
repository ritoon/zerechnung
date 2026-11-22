package invoice

import (
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/codes"
)

// TestTaxPointCodes checks the BT-8 constants against the code list of
// BR-CL-06, and the code of the option for VAT on debits (DGFiP external
// specifications v3.2, rule G1.43: 5 in CII).
func TestTaxPointCodes(t *testing.T) {
	for _, c := range []string{TaxPointInvoiceDate, TaxPointDeliveryDate, TaxPointPaymentDate, TaxPointDebits} {
		if !codes.TaxPointDateCode(c) {
			t.Errorf("%q is not a VAT point date code", c)
		}
	}
	if TaxPointDebits != "5" {
		t.Errorf("TaxPointDebits = %q, want 5", TaxPointDebits)
	}
}

// TestSchemeCodes checks the scheme constants against the code lists of
// BR-CL-25 (EAS, electronic addresses) and BR-CL-10/11/21/26 (ISO 6523 ICD,
// party identifiers): every scheme is an EAS, and only the VAT number
// schemes (99xx) and EM are not ICD schemes.
func TestSchemeCodes(t *testing.T) {
	schemes := []string{
		SchemeSIREN, SchemeSIRET, SchemeGLN, SchemeFRDirectory, SchemeLeitwegID, SchemeGEBA, SchemeEmail,
		SchemeSEOrgNumber, SchemeKvK, SchemeCVR, SchemeOIN, SchemeNOOrgNumber, SchemeIPA, SchemePEC,
		SchemeCodiceDestinatario, SchemeBEEnterprise, SchemeCodiceFiscale, SchemePartitaIVA,
		SchemeATVAT, SchemeESVAT, SchemeBEVAT, SchemeDEVAT, SchemeHRVAT, SchemeIEVAT, SchemeLUVAT,
		SchemeNLVAT, SchemePLVAT, SchemePTVAT, SchemeFRVAT,
	}
	seen := map[string]bool{}
	for _, s := range schemes {
		if seen[s] {
			t.Errorf("scheme %q defined twice", s)
		}
		seen[s] = true
		if !codes.EAS(s) {
			t.Errorf("%q is not an electronic address scheme", s)
		}
		if vat := s == SchemeEmail || s[:2] == "99"; codes.ICD(s) == vat {
			t.Errorf("codes.ICD(%q) = %v", s, !vat)
		}
	}
}

// TestVATEXCodes checks the VAT exemption reason codes against the code list
// of BR-CL-22, as written: upper case.
func TestVATEXCodes(t *testing.T) {
	for _, c := range []string{VATEXIntraCommunity, VATEXReverseCharge, VATEXExport, VATEXOutOfScope, VATEXFranchise} {
		if !codes.VATEX(c) || strings.ToUpper(c) != c {
			t.Errorf("%q is not a VATEX code", c)
		}
	}
}
