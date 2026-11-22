package it

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/internal/rules"
	ublrules "github.com/ritoon/zerechnung/internal/rules/ubl"
	"github.com/ritoon/zerechnung/internal/xmltree"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
)

const (
	ciiBase = "testdata/ciusit.xml"     // domestic CIUS-IT CII invoice without finding
	ublBase = "testdata/ciusit.ubl.xml" // the same invoice in UBL
	extBase = "testdata/extit.ubl.xml"  // ublBase with the Italian extensions (EXT-IT)
)

func load(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// run parses a CII or UBL document and runs Check on it.
func run(t testing.TB, data []byte) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	doc := rules.NewDoc(root, invoice.ProfileUnknown)
	if ublrules.IsUBL(root) {
		doc = ublrules.View(root, invoice.ProfileUnknown)
	}
	r := &rules.Report{}
	Check(&rules.Checker{Doc: doc, Report: r, Source: rules.SourceIT})
	return r
}

// toUBL converts a CII invoice to UBL with packages cii and ubl.
func toUBL(t testing.TB, data []byte) []byte {
	t.Helper()
	inv, err := cii.Parse(data)
	if err != nil {
		t.Fatalf("cii.Parse: %v", err)
	}
	out, err := ubl.Marshal(inv, nil)
	if err != nil {
		t.Fatalf("ubl.Marshal: %v", err)
	}
	return out
}

// edit replaces occurrence nth (from 1) of old by new, or every occurrence
// when nth is 0.
type edit struct {
	old, new string
	nth      int
}

func rep(old, new string) edit         { return edit{old, new, 1} }
func repN(n int, old, new string) edit { return edit{old, new, n} }
func repAll(old, new string) edit      { return edit{old, new, 0} }
func remove(old string) edit           { return edit{old, "", 1} }
func after(anchor, xml string) edit    { return edit{anchor, anchor + xml, 1} }
func before(anchor, xml string) edit   { return edit{anchor, xml + anchor, 1} }

func mutate(t testing.TB, doc string, edits ...edit) string {
	t.Helper()
	for _, e := range edits {
		if !strings.Contains(doc, e.old) {
			t.Fatalf("edit: %q not found", e.old)
		}
		switch e.nth {
		case 0:
			doc = strings.ReplaceAll(doc, e.old, e.new)
		default:
			i := -1
			for range e.nth {
				j := strings.Index(doc[i+1:], e.old)
				if j < 0 {
					t.Fatalf("edit: occurrence %d of %q not found", e.nth, e.old)
				}
				i += j + 1
			}
			doc = doc[:i] + e.new + doc[i+len(e.old):]
		}
	}
	return doc
}

func x(n int) string { return strings.Repeat("X", n) }

// Anchors of the base invoice.
const (
	sellerName  = "<ram:Name>Esempio Reti S.r.l.</ram:Name>"
	sellerLegal = `<ram:ID schemeID="0210">12345678903</ram:ID>`
	buyerName   = "<ram:Name>Comune di Esempio</ram:Name>"
	buyerLegal  = `<ram:ID schemeID="0210">80012345676</ram:ID>`
	buyerURI    = `<ram:URIID schemeID="0201">UFABCD</ram:URIID>`
	buyerEnd    = "</ram:BuyerTradeParty>"
	tender      = "<ram:TypeCode>50</ram:TypeCode>\n      </ram:AdditionalReferencedDocument>"
	order       = "<ram:IssuerAssignedID>ORD-2026-17</ram:IssuerAssignedID>\n      </ram:BuyerOrderReferencedDocument>"
	event       = "<ram:ActualDeliverySupplyChainEvent>"
	eventEnd    = "</ram:ActualDeliverySupplyChainEvent>"
	summation   = "</ram:SpecifiedTradeSettlementHeaderMonetarySummation>"
	line1Sum    = "<ram:LineTotalAmount>750.00</ram:LineTotalAmount>\n        </ram:SpecifiedTradeSettlementLineMonetarySummation>"
	terms       = "<ram:Description>Bonifico a 30 giorni data fattura</ram:Description>"
	spec        = "urn:cen.eu:en16931:2017#compliant#urn:fatturapa.gov.it:CIUS-IT:2.0.0#conformant#urn:fatturapa.gov.it:EXT-IT:1.0.0"
	sellerIT    = "<ram:CountryID>IT</ram:CountryID>"
	exemption   = "<ram:TypeCode>VAT</ram:TypeCode>\n        <ram:BasisAmount>998.00</ram:BasisAmount>"
	orderDoc    = "<ram:BuyerOrderReferencedDocument>\n        <ram:IssuerAssignedID>ORD-2026-17</ram:IssuerAssignedID>\n      </ram:BuyerOrderReferencedDocument>"
	orderLine   = "<ram:LineID>1</ram:LineID>\n        </ram:BuyerOrderReferencedDocument>"
)

// cases lists, for each rule of Catalog, mutations of the base invoice that
// break it, and some that keep it valid (no rule expected).
var cases = []struct {
	name    string
	edits   []edit
	want    []string // rules expected, sorted
	ciiOnly bool     // the UBL conversion cannot carry the mutation
}{
	{"BR-IT-010 length", []edit{rep("<ram:ID>FT-2026/0042</ram:ID>", "<ram:ID>FT-2026/0042-ABCDEFGH</ram:ID>")}, []string{"BR-IT-010"}, false},
	{"BR-IT-010 no digit", []edit{rep("<ram:ID>FT-2026/0042</ram:ID>", "<ram:ID>FATTURA-SENZA</ram:ID>")}, []string{"BR-IT-010"}, false},
	{"BR-IT-020", []edit{after(tender, "<ram:SpecifiedProcuringProject><ram:ID>"+x(16)+"</ram:ID><ram:Name>Rete</ram:Name></ram:SpecifiedProcuringProject>")}, []string{"BR-IT-020"}, false},
	{"BR-IT-030", []edit{after(order, "<ram:ContractReferencedDocument><ram:IssuerAssignedID>"+x(21)+"</ram:IssuerAssignedID></ram:ContractReferencedDocument>")}, []string{"BR-IT-030"}, false},
	{"BR-IT-040", []edit{rep("ORD-2026-17", x(21))}, []string{"BR-IT-040"}, false},
	{"BR-IT-050", []edit{after(eventEnd, "<ram:ReceivingAdviceReferencedDocument><ram:IssuerAssignedID>"+x(21)+"</ram:IssuerAssignedID></ram:ReceivingAdviceReferencedDocument>")}, []string{"BR-IT-050"}, false},
	{"BR-IT-060", []edit{after(eventEnd, "<ram:DespatchAdviceReferencedDocument><ram:IssuerAssignedID>"+x(21)+"</ram:IssuerAssignedID></ram:DespatchAdviceReferencedDocument>")}, []string{"BR-IT-060"}, false},
	{"BR-IT-070", []edit{rep("B12C345678", x(16))}, []string{"BR-IT-070"}, false},
	{"BR-IT-071", []edit{after(tender, "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>"+x(21)+"</ram:IssuerAssignedID><ram:TypeCode>130</ram:TypeCode></ram:AdditionalReferencedDocument>")}, []string{"BR-IT-071"}, false},
	{"BR-IT-080", []edit{after(summation, "<ram:ReceivableSpecifiedTradeAccountingAccount><ram:ID>"+x(21)+"</ram:ID></ram:ReceivableSpecifiedTradeAccountingAccount>")}, []string{"BR-IT-080"}, false},
	{"BR-IT-081", []edit{rep("Manutenzione della rete, ottobre 2026.", x(201))}, []string{"BR-IT-081"}, false},
	{"BR-IT-081 200 characters", []edit{rep("Manutenzione della rete, ottobre 2026.", strings.Repeat("è", 200))}, nil, false},
	{"BR-IT-090", []edit{after(summation, "<ram:InvoiceReferencedDocument><ram:IssuerAssignedID>"+x(21)+"</ram:IssuerAssignedID></ram:InvoiceReferencedDocument>")}, []string{"BR-IT-090"}, false},
	{"BR-IT-091 legal person", []edit{rep(sellerName, "<ram:Name>"+x(81)+"</ram:Name>")}, []string{"BR-IT-091"}, false},
	{"BR-IT-091 natural person", []edit{rep(sellerName, "<ram:Name>Nome#Cognome:"+x(61)+"#Rossi</ram:Name>")}, []string{"BR-IT-091"}, false},
	{"BR-IT-091 natural person, valid", []edit{rep(sellerName, "<ram:Name>Nome#Cognome:"+x(60)+"#"+x(60)+"</ram:Name>")}, nil, false},
	{"BR-IT-DC-100 EORI", []edit{before(sellerName, "<ram:ID>EORI:IT12345</ram:ID>")}, []string{"BR-IT-DC-100"}, false},
	{"BR-IT-DC-100 REA", []edit{before(sellerName, "<ram:ID>REA:RM-123456</ram:ID>")}, []string{"BR-IT-DC-100"}, false},
	{"BR-IT-DC-100 ALBO", []edit{before(sellerName, "<ram:ID>ALBO:"+x(133)+"</ram:ID>")}, []string{"BR-IT-DC-100"}, false},
	{"BR-IT-DC-100 valid", []edit{before(sellerName, "<ram:ID>REA:RM#1234567</ram:ID><ram:ID>EORI:IT1234567890123</ram:ID><ram:ID>ALBO:Ingegneri#A123#2010-05-04</ram:ID>")}, nil, false},
	{"BR-IT-DC-110 scheme 0210", []edit{rep(sellerLegal, `<ram:ID schemeID="0210">1234567890</ram:ID>`)}, []string{"BR-IT-DC-110", "IT-CF-01"}, false},
	{"BR-IT-DC-110 CF:", []edit{rep(sellerLegal, `<ram:ID>CF:1234567890</ram:ID>`)}, []string{"BR-IT-DC-110", "IT-CF-01"}, false},
	{"BR-IT-DC-110 CF:, valid", []edit{rep(sellerLegal, `<ram:ID>CF:RSSMRA85T10A562S</ram:ID>`)}, nil, false},
	{"BR-IT-120", []edit{rep("IT12345678903</ram:ID>", "1234567890312</ram:ID>")}, []string{"BR-IT-120"}, false},
	{"BR-IT-122", []edit{after(sellerName, "<ram:Description>"+x(22)+"</ram:Description>")}, []string{"BR-IT-122"}, false},
	{"BR-IT-DC-140", []edit{rep("<ram:CityName>Roma</ram:CityName>", "")}, []string{"BR-IT-DC-140"}, false},
	{"BR-IT-DC-141 no house number", []edit{rep("<ram:LineTwo>12</ram:LineTwo>", "<ram:LineTwo>Interno B</ram:LineTwo>")}, []string{"BR-IT-DC-141"}, false},
	{"BR-IT-DC-141 missing", []edit{remove("<ram:LineTwo>12</ram:LineTwo>")}, []string{"BR-IT-DC-141"}, false},
	{"BR-IT-DC-141 s.n.c.", []edit{rep("<ram:LineTwo>12</ram:LineTwo>", "<ram:LineTwo>s.n.c.</ram:LineTwo>")}, nil, false},
	{"BR-IT-DC-150", []edit{rep("<ram:CountrySubDivisionName>RM<", "<ram:CountrySubDivisionName>Lazio<")}, []string{"BR-IT-DC-150"}, false},
	{"BR-IT-DC-161", []edit{rep("0612345678", "0612")}, []string{"BR-IT-DC-161"}, false},
	{"BR-IT-171", []edit{rep(buyerName, "<ram:Name>"+x(81)+"</ram:Name>")}, []string{"BR-IT-171"}, false},
	{"BR-IT-180", []edit{after("</ram:URIUniversalCommunication>\n      </ram:BuyerTradeParty>", "")}, nil, false},
	{"BR-IT-180 length", []edit{before(buyerEnd, `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">IT`+strings.Repeat("1", 29)+`</ram:ID></ram:SpecifiedTaxRegistration>`)}, []string{"BR-IT-180", "IT-PIVA-01"}, false},
	{"BR-IT-180 valid", []edit{before(buyerEnd, `<ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">IT80012345676</ram:ID></ram:SpecifiedTaxRegistration>`)}, nil, false},
	{"BR-IT-190 scheme", []edit{rep(buyerURI, `<ram:URIID schemeID="0202">ufficio@pec.example.it</ram:URIID>`)}, []string{"BR-IT-190"}, false},
	{"BR-IT-190 missing", []edit{rep("<ram:URIUniversalCommunication>\n          "+buyerURI+"\n        </ram:URIUniversalCommunication>", "")}, []string{"BR-IT-190"}, false},
	{"BR-IT-200 lower case", []edit{rep("UFABCD", "ufabcd")}, []string{"BR-IT-200"}, false},
	{"BR-IT-200 length", []edit{rep("UFABCD", "UFABCDE")}, []string{"BR-IT-200"}, false},
	{"BR-IT-210", []edit{remove("<ram:PostcodeCode>00100</ram:PostcodeCode>")}, []string{"BR-IT-210"}, false},
	{"BR-IT-220", []edit{repN(2, "<ram:CountrySubDivisionName>RM<", "<ram:CountrySubDivisionName>Lazio<")}, []string{"BR-IT-220"}, false},
	{"BR-IT-DC-221", []edit{after("<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>", "<ram:PayeeTradeParty><ram:Name>"+x(201)+"</ram:Name></ram:PayeeTradeParty>")}, []string{"BR-IT-DC-221"}, false},
	{"BR-IT-222", []edit{after(buyerEnd, "<ram:SellerTaxRepresentativeTradeParty><ram:Name>"+x(81)+`</ram:Name><ram:PostalTradeAddress><ram:CountryID>IT</ram:CountryID></ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">IT12345678903</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTaxRepresentativeTradeParty>`)}, []string{"BR-IT-222"}, false},
	{"BR-IT-230", []edit{after(buyerEnd, `<ram:SellerTaxRepresentativeTradeParty><ram:Name>Rappresentante</ram:Name><ram:PostalTradeAddress><ram:CountryID>IT</ram:CountryID></ram:PostalTradeAddress><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">9912345678903</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTaxRepresentativeTradeParty>`)}, []string{"BR-IT-230"}, false},
	{"BR-IT-240 and 250", []edit{before(event, "<ram:ShipToTradeParty><ram:PostalTradeAddress><ram:LineOne>Via Roma 1</ram:LineOne><ram:CountryID>IT</ram:CountryID><ram:CountrySubDivisionName>Lazio</ram:CountrySubDivisionName></ram:PostalTradeAddress></ram:ShipToTradeParty>")}, []string{"BR-IT-240", "BR-IT-250"}, false},
	{"BR-IT-240 outside Italy", []edit{before(event, "<ram:ShipToTradeParty><ram:PostalTradeAddress><ram:LineOne>1 rue de Rivoli</ram:LineOne><ram:CountryID>FR</ram:CountryID><ram:CountrySubDivisionName>Île-de-France</ram:CountrySubDivisionName></ram:PostalTradeAddress></ram:ShipToTradeParty>")}, nil, false},
	{"BR-IT-260", []edit{rep("<ram:SpecifiedTradeSettlementPaymentMeans>\n        <ram:TypeCode>58</ram:TypeCode>\n        <ram:PayeePartyCreditorFinancialAccount>\n          <ram:IBANID>IT60X0542811101000000123456</ram:IBANID>\n        </ram:PayeePartyCreditorFinancialAccount>\n        <ram:PayeeSpecifiedCreditorFinancialInstitution>\n          <ram:BICID>BPPIITRRXXX</ram:BICID>\n        </ram:PayeeSpecifiedCreditorFinancialInstitution>\n      </ram:SpecifiedTradeSettlementPaymentMeans>", "")}, []string{"BR-IT-260"}, false},
	{"BR-IT-261", []edit{remove(terms)}, []string{"BR-IT-261"}, false},
	{"BR-IT-DC-262", []edit{rep("<ram:PaymentReference>FT-2026/0042</ram:PaymentReference>", "<ram:PaymentReference>"+x(61)+"</ram:PaymentReference>")}, []string{"BR-IT-DC-262"}, false},
	{"BR-IT-270 check digits", []edit{rep("IT60X", "IT61X")}, []string{"BR-IT-270"}, false},
	{"BR-IT-270 proprietary", []edit{rep("<ram:IBANID>IT60X0542811101000000123456</ram:IBANID>", "<ram:ProprietaryID>123456</ram:ProprietaryID>")}, []string{"BR-IT-270"}, true},
	{"BR-IT-280", []edit{rep("BPPIITRRXXX", "BPPIIT")}, []string{"BR-IT-280"}, false},
	{"BR-IT-290", []edit{rep("<ram:ActualAmount>20.00</ram:ActualAmount>", "<ram:ActualAmount>1234567890123.00</ram:ActualAmount>")}, []string{"BR-IT-290"}, false},
	{"BR-IT-300", []edit{rep("<ram:GrandTotalAmount>1217.56", "<ram:GrandTotalAmount>1234567890123.5")}, []string{"BR-IT-300"}, false},
	{"BR-IT-300 15 characters", []edit{rep("<ram:GrandTotalAmount>1217.56", "<ram:GrandTotalAmount>000123456789012.00")}, nil, false},
	{"BR-IT-310", []edit{before("<ram:GrandTotalAmount>", "<ram:RoundingAmount>-123456789012.00</ram:RoundingAmount>")}, []string{"BR-IT-310"}, false},
	{"BR-IT-310 15 characters", []edit{before("<ram:GrandTotalAmount>", "<ram:RoundingAmount>-12345678901.00</ram:RoundingAmount>")}, nil, false},
	{"BR-IT-320", []edit{rep("<ram:DuePayableAmount>1217.56", "<ram:DuePayableAmount>1234567890123")}, []string{"BR-IT-320"}, false},
	{"BR-IT-330", []edit{rep("<ram:BasisAmount>998.00", "<ram:BasisAmount>1234567890123")}, []string{"BR-IT-330"}, false},
	{"BR-IT-340", []edit{rep("<ram:CalculatedAmount>219.56", "<ram:CalculatedAmount>1234567890123")}, []string{"BR-IT-340"}, false},
	{"BR-IT-350", []edit{rep("<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>L</ram:CategoryCode>")}, []string{"BR-IT-350"}, false},
	{"BR-IT-DC-351 characters", []edit{rep(exemption, "<ram:TypeCode>VAT</ram:TypeCode><ram:ExemptionReason>Esente ✓</ram:ExemptionReason><ram:BasisAmount>998.00</ram:BasisAmount>")}, []string{"BR-IT-DC-351"}, false},
	{"BR-IT-DC-351 length", []edit{rep(exemption, "<ram:TypeCode>VAT</ram:TypeCode><ram:ExemptionReason>"+x(106)+"</ram:ExemptionReason><ram:BasisAmount>998.00</ram:BasisAmount>")}, []string{"BR-IT-DC-351"}, false},
	{"BR-IT-360", []edit{after(tender, "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>ALL-1</ram:IssuerAssignedID><ram:TypeCode>916</ram:TypeCode></ram:AdditionalReferencedDocument>")}, []string{"BR-IT-360"}, false},
	{"BR-IT-360 valid", []edit{after(tender, "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>ALL-1</ram:IssuerAssignedID><ram:URIID>https://example.it/all-1.pdf</ram:URIID><ram:TypeCode>916</ram:TypeCode></ram:AdditionalReferencedDocument>")}, nil, false},
	{"BR-IT-361 too large", []edit{rep("<ram:LineID>2</ram:LineID>", "<ram:LineID>10000</ram:LineID>")}, []string{"BR-IT-361"}, false},
	{"BR-IT-361 not numeric", []edit{rep("<ram:LineID>2</ram:LineID>", "<ram:LineID>2a</ram:LineID>")}, []string{"BR-IT-361"}, false},
	{"BR-IT-361 valid", []edit{rep("<ram:LineID>2</ram:LineID>", "<ram:LineID>9999</ram:LineID>")}, nil, false},
	{"BR-IT-370", []edit{after(line1Sum, "<ram:AdditionalReferencedDocument><ram:IssuerAssignedID>"+x(101)+"</ram:IssuerAssignedID><ram:TypeCode>130</ram:TypeCode></ram:AdditionalReferencedDocument>")}, []string{"BR-IT-370"}, false},
	{"BR-IT-380 decimals", []edit{rep(`unitCode="HUR">10<`, `unitCode="HUR">1.123456789<`)}, []string{"BR-IT-380"}, false},
	{"BR-IT-380 length", []edit{rep(`unitCode="HUR">10<`, `unitCode="HUR">12345678901234567890<`)}, []string{"BR-IT-380"}, true},
	{"BR-IT-380 valid", []edit{rep(`unitCode="HUR">10<`, `unitCode="HUR">123456789012.12345678<`)}, nil, true},
	{"BR-IT-390", []edit{rep("<ram:LineTotalAmount>750.00</ram:LineTotalAmount>\n        </ram:SpecifiedTradeSettlementLineMonetarySummation>", "<ram:LineTotalAmount>1234567890123.00</ram:LineTotalAmount>\n        </ram:SpecifiedTradeSettlementLineMonetarySummation>")}, []string{"BR-IT-390"}, false},
	{"BR-IT-400", []edit{rep("<ram:BuyerOrderReferencedDocument>\n          <ram:LineID>1</ram:LineID>", "<ram:BuyerOrderReferencedDocument>\n          <ram:LineID>"+x(21)+"</ram:LineID>")}, []string{"BR-IT-400"}, false},
	{"BR-IT-410", []edit{after(line1Sum, "<ram:ReceivableSpecifiedTradeAccountingAccount><ram:ID>"+x(21)+"</ram:ID></ram:ReceivableSpecifiedTradeAccountingAccount>")}, []string{"BR-IT-410"}, false},
	{"BR-IT-420", []edit{rep("<ram:ActualAmount>12.00</ram:ActualAmount>", "<ram:ActualAmount>1234567890123.00</ram:ActualAmount>")}, []string{"BR-IT-420"}, false},
	{"BR-IT-430", []edit{rep("<ram:ChargeAmount>75.00</ram:ChargeAmount>", "<ram:ChargeAmount>75.123456789</ram:ChargeAmount>")}, []string{"BR-IT-430"}, false},
	{"BR-IT-440", []edit{rep("MAN-01", x(36))}, []string{"BR-IT-440"}, false},
	{"BR-IT-470", []edit{after("</ram:Description>\n      </ram:SpecifiedTradeProduct>", "")}, nil, false},
	{"BR-IT-470 length", []edit{rep("degli uffici comunali</ram:Description>", `degli uffici comunali</ram:Description><ram:DesignatedProductClassification><ram:ClassCode listID="STI">`+x(36)+`</ram:ClassCode></ram:DesignatedProductClassification>`)}, []string{"BR-IT-470"}, false},
	{"BR-IT-DC-510 terms", []edit{repAll("<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>B</ram:CategoryCode>")}, []string{"BR-IT-DC-510"}, false},
	{"BR-IT-DC-510 mixed", []edit{rep("<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>B</ram:CategoryCode>"), rep(terms, "<ram:Description>The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)</ram:Description>")}, []string{"BR-IT-DC-510"}, false},
	{"BR-IT-DC-510 valid", []edit{repAll("<ram:CategoryCode>S</ram:CategoryCode>", "<ram:CategoryCode>B</ram:CategoryCode>"), rep(terms, "<ram:Description>The total is without the VAT amount due to Split payment (ex art.17-ter del DPR 633/1972)</ram:Description>")}, nil, false},
	{"BR-IT-520 missing", []edit{remove("<ram:SpecifiedLegalOrganization>\n          " + buyerLegal + "\n        </ram:SpecifiedLegalOrganization>")}, []string{"BR-IT-520"}, false},
	{"BR-IT-520 scheme", []edit{rep(buyerLegal, "<ram:ID>80012345676</ram:ID>")}, []string{"BR-IT-520"}, false},
	{"BR-IT-520 CF:", []edit{rep(buyerLegal, "<ram:ID>CF:80012345676</ram:ID>")}, nil, false},
	{"BR-IT-521", []edit{before(buyerName, "<ram:ID>IT12345</ram:ID>")}, []string{"BR-IT-521"}, false},
	{"BR-IT-521 valid", []edit{before(buyerName, "<ram:ID>EORI:IT1234567890123</ram:ID>")}, nil, false},
	{"BR-IT-530 characters", []edit{rep("<ram:Name>Switch di rete</ram:Name>", "<ram:Name>Switch di rete €</ram:Name>")}, []string{"BR-IT-530"}, false},
	{"BR-IT-530 length", []edit{rep("Intervento mensile sugli apparati di rete degli uffici comunali", x(1000))}, []string{"BR-IT-530"}, false},
	{"IT-PROFILE-01 domestic", []edit{rep(spec, invoice.URNCIUSIT)}, []string{"IT-PROFILE-01"}, false},
	{"IT-PROFILE-01 cross-border", []edit{rep(sellerIT, "<ram:CountryID>FR</ram:CountryID>"), rep("IT12345678903", "FR32123456789"), rep("0612345678", "06")}, []string{"IT-PROFILE-01"}, false},
	{"cross-border", []edit{rep(spec, invoice.URNCIUSIT), rep(sellerIT, "<ram:CountryID>FR</ram:CountryID>"), rep("IT12345678903", "FR32123456789"), rep("0612345678", "06")}, nil, false},
	{"IT-PROFILE-01 no Italian party", []edit{rep(spec, invoice.URNCIUSIT), repAll(sellerIT, "<ram:CountryID>FR</ram:CountryID>")}, []string{"IT-PROFILE-01"}, false},
	{"IT-PIVA-02", []edit{rep("IT12345678903", "IT12345678904")}, []string{"IT-PIVA-02"}, false},
	{"IT-PIVA-01", []edit{rep(`<ram:URIID schemeID="0211">12345678903</ram:URIID>`, `<ram:URIID schemeID="0211">1234567890</ram:URIID>`)}, []string{"IT-PIVA-01"}, false},
	{"IT-CF-02 legal person", []edit{rep(buyerLegal, `<ram:ID schemeID="0210">80012345677</ram:ID>`)}, []string{"IT-CF-02"}, false},
	{"IT-CF-02 natural person", []edit{rep(buyerLegal, `<ram:ID schemeID="0210">RSSMRA85T10A562T</ram:ID>`)}, []string{"IT-CF-02"}, false},
	{"IT-CF-01", []edit{rep(buyerLegal, `<ram:ID schemeID="0210">RSSMRA85Z10A562S</ram:ID>`)}, []string{"IT-CF-01"}, false},
	{"IT-CF-01 identifier", []edit{before(sellerName, `<ram:GlobalID schemeID="0210">ABC</ram:GlobalID>`)}, []string{"IT-CF-01"}, false},
	{"IT-CF-02 tax registration", []edit{rep(`<ram:ID schemeID="VA">IT12345678903</ram:ID>`, `<ram:ID schemeID="VA">IT12345678903</ram:ID></ram:SpecifiedTaxRegistration><ram:SpecifiedTaxRegistration><ram:ID schemeID="FC">RSSMRA85T10A562T</ram:ID>`)}, []string{"IT-CF-02"}, false},
	{"BR-IT-DE-052", []edit{remove(orderDoc)}, []string{"BR-IT-DE-052"}, false},
	{"BR-IT-DE-052 order in the line", []edit{remove(orderDoc), rep(orderLine, "<ram:LineID>1</ram:LineID><ram:IssuerAssignedID>ORD-2026-17</ram:IssuerAssignedID></ram:BuyerOrderReferencedDocument>")}, nil, true},
	{"BR-IT-DE-052 cross-border", []edit{remove(orderDoc), rep(spec, invoice.URNCIUSIT), rep(sellerIT, "<ram:CountryID>FR</ram:CountryID>"), rep("IT12345678903", "FR32123456789"), rep("0612345678", "06")}, nil, false},
}

// ublWant lists the rules expected on the UBL conversion of a case when
// they differ from those on the CII invoice: the UBL syntax carries the
// Italian extensions, whose rules (BR-IT-DE-*) apply to it only.
var ublWant = map[string][]string{
	// BT-16 has no date: BR-IT-DE-016 requires one in UBL.
	"BR-IT-060": {"BR-IT-060", "BR-IT-DE-016"},
}

func TestValid(t *testing.T) {
	for _, name := range []string{ciiBase, ublBase, extBase} {
		if r := run(t, []byte(load(t, name))); len(r.Findings) > 0 {
			t.Errorf("%s:\n%s", name, r)
		}
	}
}

func TestRules(t *testing.T) {
	base := load(t, ciiBase)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := []byte(mutate(t, base, tc.edits...))
			r := run(t, doc)
			if got := r.Rules(); !slices.Equal(got, tc.want) {
				t.Errorf("CII: rules %v, want %v\n%s", got, tc.want, r)
			}
			for _, f := range r.Findings {
				if f.Source != rules.SourceIT || f.Severity != severities()[f.Rule] || !strings.HasPrefix(f.Message, "["+f.Rule+"] ") || f.Line == 0 {
					t.Errorf("CII: finding %+v", f)
				}
			}
			if tc.ciiOnly {
				return
			}
			want, ok := ublWant[tc.name]
			if !ok {
				want = tc.want
			}
			r = run(t, toUBL(t, doc))
			if got := r.Rules(); !slices.Equal(got, want) {
				t.Errorf("UBL: rules %v, want %v\n%s", got, want, r)
			}
			for _, f := range r.Findings {
				if !strings.HasPrefix(f.Path, "/Invoice") {
					t.Errorf("UBL: finding not located in the UBL document: %+v", f)
				}
			}
		})
	}
}

// TestCatalog checks that every rule of Catalog is tested, and that Catalog
// and the rules left out cover appendices 2 and 3 of the technical rules.
func TestCatalog(t *testing.T) {
	tested := map[string]bool{}
	for _, tc := range cases {
		for _, r := range tc.want {
			tested[r] = true
		}
	}
	for _, tc := range extCases {
		for _, r := range tc.want {
			tested[r] = true
		}
	}
	var ids, ext []string
	for _, r := range Catalog {
		if !tested[r.ID] {
			t.Errorf("%s: no test case", r.ID)
		}
		if r.Description == "" || r.Severity != rules.Error && r.Severity != rules.Warning {
			t.Errorf("%s: %+v", r.ID, r)
		}
		switch {
		case strings.HasPrefix(r.ID, "BR-IT-DE-"):
			ext = append(ext, r.ID)
		case strings.HasPrefix(r.ID, "BR-IT-"):
			ids = append(ids, r.ID)
		}
	}
	// The 61 rules of appendix 2, technical rules 2.7.
	appendix2 := strings.Fields(`BR-IT-010 BR-IT-020 BR-IT-030 BR-IT-040 BR-IT-050 BR-IT-060 BR-IT-070
		BR-IT-071 BR-IT-080 BR-IT-081 BR-IT-090 BR-IT-091 BR-IT-DC-100 BR-IT-DC-110 BR-IT-120 BR-IT-122
		BR-IT-DC-140 BR-IT-DC-141 BR-IT-DC-150 BR-IT-DC-161 BR-IT-171 BR-IT-180 BR-IT-190 BR-IT-200
		BR-IT-210 BR-IT-220 BR-IT-DC-221 BR-IT-222 BR-IT-230 BR-IT-240 BR-IT-250 BR-IT-260 BR-IT-261
		BR-IT-DC-262 BR-IT-270 BR-IT-280 BR-IT-290 BR-IT-DC-291 BR-IT-300 BR-IT-310 BR-IT-320 BR-IT-330
		BR-IT-340 BR-IT-350 BR-IT-DC-351 BR-IT-360 BR-IT-361 BR-IT-370 BR-IT-380 BR-IT-390 BR-IT-400
		BR-IT-410 BR-IT-420 BR-IT-430 BR-IT-440 BR-IT-470 BR-IT-DC-480 BR-IT-DC-510 BR-IT-520 BR-IT-521
		BR-IT-530`)
	notImplemented := []string{"BR-IT-DC-291", "BR-IT-DC-480"}
	ids = append(ids, notImplemented...)
	slices.Sort(ids)
	slices.Sort(appendix2)
	if len(appendix2) != 61 || !slices.Equal(ids, appendix2) {
		t.Errorf("Catalog and the rules left out: %v\nappendix 2: %v", ids, appendix2)
	}
	// The 45 rules of appendix 3, technical rules 2.7, in their order.
	var appendix3 []string
	for i := 1; i <= 42; i++ {
		appendix3 = append(appendix3, fmt.Sprintf("BR-IT-DE-%03d", i))
	}
	appendix3 = append(appendix3, "BR-IT-DE-050", "BR-IT-DE-051", "BR-IT-DE-052")
	if !slices.Equal(ext, appendix3) {
		t.Errorf("Catalog: %v\nappendix 3: %v", ext, appendix3)
	}
}

func TestPIVA(t *testing.T) {
	for s, want := range map[string]bool{
		"12345678903": true, "80012345676": true, "00000000000": true,
		"12345678904": false, "1234567890": false, "IT12345678903": false, "1234567890A": false, "": false,
	} {
		if got := ValidPIVA(s); got != want {
			t.Errorf("ValidPIVA(%q) = %v", s, got)
		}
	}
}

func TestCF(t *testing.T) {
	for s, want := range map[string]bool{
		"RSSMRA85T10A562S": true,
		"RSSMRA85T10A562T": false, // check letter
		"RSSMRA85T50A562W": true,  // a woman born on the 10th (day 50)
		"RSSMRA85T50A562R": false, // check letter
		"RSSMRA85T32A562S": false, // day 32
		"RSSMRA85T10A56NA": false, // omocodia: 2 written N, check letter not recomputed
		"80012345676":      true,  // legal person
		"80012345677":      false,
		"rssmra85t10a562s": false, // lower case
		"RSSMRA85T10A562":  false,
		"":                 false,
	} {
		if got := ValidCF(s); got != want {
			t.Errorf("ValidCF(%q) = %v", s, got)
		}
	}
	// Omocodia: each digit replaced by its letter keeps a valid format; the
	// check letter is recomputed on the code as written.
	cf := []byte("RSSMRA85T10A562S")
	for _, i := range cfDigitPositions {
		cf[i] = omocodia[cf[i]-'0']
	}
	cf[15] = cfCheckChar(string(cf[:15]))
	if s := string(cf); !ValidCF(s) || cfFormat(s) != "" {
		t.Errorf("ValidCF(%q) = false: %s", s, cfFormat(s))
	}
	if why := cfFormat("RSSMRA85T72A562S"); why == "" {
		t.Error("day 72 accepted")
	}
}

func TestIBAN(t *testing.T) {
	for s, want := range map[string]bool{
		"IT60X0542811101000000123456":       true,
		"NL91ABNA0417164300":                true,
		"DE89370400440532013000":            true,
		"IT61X0542811101000000123456":       false,
		"IT60 X054 2811 1010 0000 0123 456": false,
		"it60x0542811101000000123456":       false,
		"IT60X05428":                        false,
	} {
		if got := validIBAN(s); got != want {
			t.Errorf("validIBAN(%q) = %v", s, got)
		}
	}
}

func TestFits(t *testing.T) {
	for _, tc := range []struct {
		v                      string
		size, minFrac, maxFrac int
		want                   bool
	}{
		{"123456789012.00", 15, 2, 2, true},
		{"123456789012", 15, 2, 2, true},
		{"1234567890123", 15, 2, 2, false},
		{"-12345678901.00", 15, 2, 2, true},
		{"-123456789012", 15, 2, 2, false},
		{"0000000000001.00", 15, 2, 2, true},
		{"12.5", 15, 2, 2, true},
		{"12.555", 15, 2, 2, false},
		{"1.12345678", 21, 2, 8, true},
		{"1.123456789", 21, 2, 8, false},
		{"123456789012.12345678", 21, 2, 8, true},
		{"1234567890123.1234567", 21, 2, 8, true},
		{"12345678901234567890", 21, 2, 8, false},
		{"abc", 15, 2, 2, true},
		{"", 15, 2, 2, true},
	} {
		if got := fits(tc.v, tc.size, tc.minFrac, tc.maxFrac); got != tc.want {
			t.Errorf("fits(%q, %d, %d, %d) = %v", tc.v, tc.size, tc.minFrac, tc.maxFrac, got)
		}
	}
}

func TestProvinces(t *testing.T) {
	if len(provinces()) != 111 || !provinces()["RM"] || !provinces()["SU"] || provinces()["EE"] || provinces()["rm"] {
		t.Errorf("%d provinces", len(provinces()))
	}
}

func TestDomesticOnly(t *testing.T) {
	// A seller outside Italy: the domestic rules (BR-IT-DC-*) do not apply.
	doc := mutate(t, load(t, ciiBase),
		rep(spec, invoice.URNCIUSIT),
		rep(sellerIT, "<ram:CountryID>FR</ram:CountryID>"),
		rep("IT12345678903", "FR32123456789"),
		rep("0612345678", "06"),
		rep("<ram:LineTwo>12</ram:LineTwo>", ""),
		rep("<ram:PaymentReference>FT-2026/0042</ram:PaymentReference>", "<ram:PaymentReference>"+x(61)+"</ram:PaymentReference>"),
	)
	if r := run(t, []byte(doc)); len(r.Findings) > 0 {
		t.Errorf("cross-border invoice:\n%s", r)
	}
}
