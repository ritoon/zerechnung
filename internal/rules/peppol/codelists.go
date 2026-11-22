package peppol

import (
	"regexp"
	"strings"
	"sync"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// Rules of the pattern "Restricted code lists and formatting".

// Code lists of the schematron (variables of the pattern). The pattern also
// declares ISO 3166, UNCL 2005 and UNCL 5305 lists, which no CII rule uses.
var (
	mimeCodes = sync.OnceValue(func() map[string]bool {
		return set("application/pdf image/png image/jpeg text/csv application/vnd.openxmlformats-officedocument.spreadsheetml.sheet application/vnd.oasis.opendocument.spreadsheet")
	})

	iso4217 = sync.OnceValue(func() map[string]bool {
		return set("AED AFN ALL AMD AOA ARS AUD AWG AZN BAM BBD BDT BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XAG XAU XBA XBB XBC XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA YER ZAR ZMW ZWG XXX CNH XCG")
	})

	uncl5189 = sync.OnceValue(func() map[string]bool { return set("41 42 60 62 63 64 65 66 67 68 70 71 88 95 100 102 103 104 105") })

	uncl7161 = sync.OnceValue(func() map[string]bool {
		return set("AA AAA AAC AAD AAE AAF AAH AAI AAS AAT AAV AAY AAZ ABA ABB ABC ABD ABF ABK ABL ABN ABR ABS ABT ABU ACF ACG ACH ACI ACJ ACK ACL ACM ACS ADC ADE ADJ ADK ADL ADM ADN ADO ADP ADQ ADR ADT ADW ADY ADZ AEA AEB AEC AED AEF AEH AEI AEJ AEK AEL AEM AEN AEO AEP AES AET AEU AEV AEW AEX AEY AEZ AJ AU CA CAB CAD CAE CAF CAI CAJ CAK CAL CAM CAN CAO CAP CAQ CAR CAS CAT CAU CAV CAW CAX CAY CAZ CD CG CS CT DAB DAC DAD DAF DAG DAH DAI DAJ DAK DAL DAM DAN DAO DAP DAQ DL EG EP ER FAA FAB FAC FC FH FI GAA HAA HD HH IAA IAB ID IF IR IS KO L1 LA LAA LAB LF MAE MI ML NAA OA PA PAA PC PL PRV RAB RAC RAD RAF RE RF RH RV SA SAA SAD SAE SAI SG SH SM SU TAB TAC TT TV V1 V2 WH XAA YY ZZZ")
	})

	// eas is the Electronic Address Identifier Scheme list ($eaid).
	eas = sync.OnceValue(func() map[string]bool {
		return set("0002 0007 0009 0060 0088 0096 0097 0106 0130 0135 0142 0151 0158 0183 0184 0188 0190 0191 0192 0195 0196 0198 0199 0200 0201 0204 0208 0209 0210 0211 0216 0218 0221 0225 0230 0235 0240 0242 0244 0245 0246 0248 9910 9913 9914 9915 9918 9919 9920 9922 9923 9924 9925 9926 9927 9928 9929 9930 9931 9932 9933 9934 9935 9936 9937 9938 9939 9940 9941 9942 9943 9944 9945 9946 9947 9948 9949 9950 9951 9952 9953 9957 9959")
	})

	// invoiceTypes are the document type codes of profiles 01 and 02
	// (PEPPOL-EN16931-P0100).
	invoiceTypes = sync.OnceValue(func() map[string]bool {
		return set("71 102 218 219 326 331 382 553 817 870 875 876 877 380 383 384 386 388 393 82 80 84 395 575 623 780 381 396 81 83 532")
	})
)

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, v := range strings.Fields(s) {
		m[v] = true
	}
	return m
}

var reDate = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`20[0-9]{6}`) })

// codeList checks n against the rules of the code list pattern.
func (k *checker) codeList(n *xmltree.Node) {
	switch n.Name {
	case "AttachmentBinaryObject":
		if m, ok := n.LookupAttr("mimeCode"); ok {
			k.check(mimeCodes()[m], "PEPPOL-EN16931-CL001", n, "value: %q", m)
		}
	case "ReasonCode":
		if parentIs(n, "SpecifiedTradeAllowanceCharge") {
			v := norm(n.Text)
			switch nsv(first(n.Parent, "ChargeIndicator/Indicator")) {
			case "false":
				k.check(uncl5189()[v], "PEPPOL-EN16931-CL002", n, "value: %q", v)
			case "true":
				k.check(uncl7161()[v], "PEPPOL-EN16931-CL003", n, "value: %q", v)
			}
		}
	case "TaxTotalAmount":
		if c, ok := n.LookupAttr("currencyID"); ok {
			k.check(iso4217()[c], "PEPPOL-EN16931-CL007", n, "value: %q", c)
		}
	case "TypeCode":
		// The context is ram:ExchangedDocument/ram:TypeCode.
		if p := n.Parent; p != nil && p.Name == "ExchangedDocument" && p.Space == rules.NSRam && (k.profile == "01" || k.profile == "02") {
			v := norm(n.Text)
			k.check(invoiceTypes()[v], "PEPPOL-EN16931-P0100", n, "value: %q", v)
		}
	case "DateTimeString":
		if n.Space == rules.NSUdt {
			k.check(norm(attr(n, "format")) == "102" && length(n.Text) == 8 && reDate().MatchString(norm(n.Text)), "PEPPOL-EN16931-F001", n,
				"value: %q, format %q", n.Text, attr(n, "format"))
		}
	case "URIID":
		if p := n.Parent; p != nil && p.Name == "URIUniversalCommunication" && (parentIs(p, "BuyerTradeParty") || parentIs(p, "SellerTradeParty")) {
			s, _ := n.LookupAttr("schemeID")
			k.check(eas()[s], "PEPPOL-EN16931-CL008", n, "scheme %q", s)
		}
	}
}
