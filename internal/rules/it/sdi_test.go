package it

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

const (
	fprBase = "../../../fatturapa/testdata/fpr12.xml" // invoice between private parties (TD24), without finding
	fpaBase = "../../../fatturapa/testdata/fpa12.xml" // invoice to a public administration (TD01), without finding
)

// sdiReceived is the reception of the test invoices, dated 18 and 20
// November 2026.
var sdiReceived = time.Date(2026, time.December, 1, 10, 0, 0, 0, time.UTC)

// runSDI parses a FatturaPA document and runs CheckSDI on it.
func runSDI(t testing.TB, data string, at time.Time) *rules.Report {
	t.Helper()
	root, err := xmltree.Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := &rules.Report{}
	CheckSDI(&rules.Checker{Report: r, Source: rules.SourceSDI}, root, at)
	return r
}

// twice adds to doc a copy of its invoice (FatturaElettronicaBody), with the
// edits applied to the copy alone: a lot of two invoices.
func twice(t testing.TB, doc string, edits ...edit) string {
	t.Helper()
	const open, end = "  <FatturaElettronicaBody>", "</FatturaElettronicaBody>\n"
	i, j := strings.Index(doc, open), strings.Index(doc, end)
	if i < 0 || j < 0 {
		t.Fatal("no FatturaElettronicaBody")
	}
	j += len(end)
	return doc[:j] + mutate(t, doc[i:j], edits...) + doc[j:]
}

// Elements of the test invoices that the cases insert.
const (
	sdiRepresentative = "\n    <RappresentanteFiscale>\n      <DatiAnagrafici>\n        <IdFiscaleIVA>\n          <IdPaese>IT</IdPaese>\n          <IdCodice>12345678901</IdCodice>\n        </IdFiscaleIVA>\n        <CodiceFiscale>RSSMRA85T50A562X</CodiceFiscale>\n        <Anagrafica>\n          <Denominazione>Rappresentanza Esempio S.r.l.</Denominazione>\n        </Anagrafica>\n      </DatiAnagrafici>\n    </RappresentanteFiscale>"
	sdiBuyerRep       = "\n      <RappresentanteFiscale>\n        <IdFiscaleIVA>\n          <IdPaese>IT</IdPaese>\n          <IdCodice>12345678901</IdCodice>\n        </IdFiscaleIVA>\n        <Denominazione>Rappresentanza Esempio S.r.l.</Denominazione>\n      </RappresentanteFiscale>"
	sdiLinked         = "<DatiFattureCollegate>\n        <IdDocumento>FT-2026/0041</IdDocumento>\n        <Data>%s</Data>\n      </DatiFattureCollegate>\n      "
	sdiSellerEnd      = "</CedentePrestatore>"
	sdiWithholding    = "<DatiRitenuta>\n          <TipoRitenuta>RT01</TipoRitenuta>\n          <ImportoRitenuta>200.00</ImportoRitenuta>\n          <AliquotaRitenuta>20.00</AliquotaRitenuta>\n          <CausalePagamento>A</CausalePagamento>\n        </DatiRitenuta>\n        "
	sdiLineNature     = "<AliquotaIVA>0.00</AliquotaIVA>\n        <Natura>N1</Natura>\n      </DettaglioLinee>"
)

// sdiCases lists, for each control of SDICatalog, mutations of the test
// invoices that break it, and some that keep them valid (no rule expected).
var sdiCases = []struct {
	name   string
	pa     bool      // on fpaBase instead of fprBase
	at     time.Time // reception, sdiReceived when zero
	edits  []edit
	second []edit // when set, a lot with a copy of the invoice so edited
	want   []string
}{
	{"00300", false, time.Time{}, []edit{repN(1, "<IdCodice>12345678903</IdCodice>", "<IdCodice>12345678901</IdCodice>")}, nil, []string{"00300"}},
	{"00300 format", false, time.Time{}, []edit{repN(1, "<IdCodice>12345678903</IdCodice>", "<IdCodice>RSSMRA85T50</IdCodice>")}, nil, []string{"00300"}},
	{"00300 natural person", false, time.Time{}, []edit{repN(1, "<IdCodice>12345678903</IdCodice>", "<IdCodice>RSSMRA85T50A562W</IdCodice>")}, nil, nil},
	{"00300 abroad", false, time.Time{}, []edit{repN(1, "<IdPaese>IT</IdPaese>", "<IdPaese>DE</IdPaese>"), repN(1, "<IdCodice>12345678903</IdCodice>", "<IdCodice>DE123456789</IdCodice>")}, nil, nil},
	{"00301", false, time.Time{}, []edit{repN(2, "<IdCodice>12345678903</IdCodice>", "<IdCodice>12345678901</IdCodice>")}, nil, []string{"00301"}},
	{"00301 format", false, time.Time{}, []edit{repN(2, "<IdCodice>12345678903</IdCodice>", "<IdCodice>1234567890</IdCodice>")}, nil, []string{"00301"}},
	{"00301 abroad", false, time.Time{}, []edit{repN(2, "<IdPaese>IT</IdPaese>", "<IdPaese>AT</IdPaese>"), repN(2, "<IdCodice>12345678903</IdCodice>", "<IdCodice>U12345678</IdCodice>")}, nil, nil},
	{"00302", false, time.Time{}, []edit{rep("<CodiceFiscale>12345678903</CodiceFiscale>", "<CodiceFiscale>12345678901</CodiceFiscale>")}, nil, []string{"00302"}},
	{"00302 natural person", true, time.Time{}, []edit{rep("<CodiceFiscale>RSSMRA85T50A562W</CodiceFiscale>", "<CodiceFiscale>RSSMRA85T50A562X</CodiceFiscale>")}, nil, []string{"00302"}},
	{"00303 00304", false, time.Time{}, []edit{after(sdiSellerEnd, sdiRepresentative)}, nil, []string{"00303", "00304"}},
	{"00303 buyer", false, time.Time{}, []edit{before("\n    </CessionarioCommittente>", sdiBuyerRep)}, nil, []string{"00303"}},
	{"00305", false, time.Time{}, []edit{rep("<IdCodice>09876543217</IdCodice>", "<IdCodice>09876543210</IdCodice>")}, nil, []string{"00305"}},
	{"00306", true, time.Time{}, []edit{rep("<CodiceFiscale>80012345676</CodiceFiscale>", "<CodiceFiscale>80012345670</CodiceFiscale>")}, nil, []string{"00306"}},
	{"00306 with IdFiscaleIVA", false, time.Time{}, []edit{after("<IdCodice>09876543217</IdCodice>\n        </IdFiscaleIVA>", "\n        <CodiceFiscale>0987654321</CodiceFiscale>")}, nil, nil},
	{"00313", false, time.Time{}, []edit{rep("<CodiceDestinatario>ABC1234</CodiceDestinatario>", "<CodiceDestinatario>XXXXXXX</CodiceDestinatario>")}, nil, []string{"00313"}},
	{"00313 abroad", false, time.Time{}, []edit{rep("<CodiceDestinatario>ABC1234</CodiceDestinatario>", "<CodiceDestinatario>XXXXXXX</CodiceDestinatario>"), repN(3, "<IdPaese>IT</IdPaese>", "<IdPaese>DE</IdPaese>")}, nil, nil},
	{"00330", false, time.Time{}, []edit{after("<CodiceDestinatario>ABC1234</CodiceDestinatario>", "\n      <PECDestinatario>SDI01@PEC.FatturaPA.it</PECDestinatario>")}, nil, []string{"00330"}},
	{"00330 buyer", false, time.Time{}, []edit{after("<CodiceDestinatario>ABC1234</CodiceDestinatario>", "\n      <PECDestinatario>fatture@pec.officine-esempio.it</PECDestinatario>")}, nil, nil},
	{"00400", false, time.Time{}, []edit{rep(sdiLineNature, "<AliquotaIVA>0.00</AliquotaIVA>\n      </DettaglioLinee>")}, nil, []string{"00400"}},
	{"00401", false, time.Time{}, []edit{after("<PrezzoTotale>900.00</PrezzoTotale>\n        <AliquotaIVA>22.00</AliquotaIVA>", "\n        <Natura>N1</Natura>")}, nil, []string{"00401"}},
	{"00401 TD16", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD16</TipoDocumento>"), after("<PrezzoTotale>900.00</PrezzoTotale>\n        <AliquotaIVA>22.00</AliquotaIVA>", "\n        <Natura>N1</Natura>")}, nil, nil},
	{"00403", false, time.Date(2026, time.November, 19, 21, 59, 0, 0, time.UTC), nil, nil, []string{"00403"}},
	{"00403 Italian date", false, time.Date(2026, time.November, 19, 22, 0, 0, 0, time.UTC), nil, nil, nil},
	{"00409", false, time.Time{}, nil, []edit{}, []string{"00409"}},
	{"00409 same year", false, time.Time{}, nil, []edit{rep("<Data>2026-11-20</Data>", "<Data>2026-01-05</Data>")}, []string{"00409"}},
	{"00409 Art73", false, time.Time{}, []edit{after("<ImportoTotaleDocumento>1252.50</ImportoTotaleDocumento>\n        <Causale>Fattura differita per le consegne di ottobre 2026.</Causale>", "\n        <Art73>SI</Art73>")}, []edit{rep("<Data>2026-11-20</Data>", "<Data>2026-01-05</Data>")}, nil},
	{"00409 other number", false, time.Time{}, nil, []edit{rep("<Numero>FT-2026/0042</Numero>", "<Numero>FT-2026/0043</Numero>")}, nil},
	{"00409 credit note", false, time.Time{}, nil, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD04</TipoDocumento>")}, nil},
	{"00411", true, time.Time{}, []edit{remove(sdiWithholding)}, nil, []string{"00411"}},
	{"00413", true, time.Time{}, []edit{repN(1, "<AliquotaIVA>22.00</AliquotaIVA>", "<AliquotaIVA>0.00</AliquotaIVA>")}, nil, []string{"00413", "00422", "00443"}},
	{"00414", true, time.Time{}, []edit{after("<ImponibileCassa>1000.00</ImponibileCassa>\n          <AliquotaIVA>22.00</AliquotaIVA>", "\n          <Natura>N4</Natura>")}, nil, []string{"00414", "00444"}},
	{"00415", true, time.Time{}, []edit{remove(sdiWithholding), after("<ImponibileCassa>1000.00</ImponibileCassa>\n          <AliquotaIVA>22.00</AliquotaIVA>", "\n          <Ritenuta>SI</Ritenuta>")}, nil, []string{"00411", "00415"}},
	{"00417", true, time.Time{}, []edit{remove("<CodiceFiscale>80012345676</CodiceFiscale>\n        ")}, nil, []string{"00417"}},
	{"00418", false, time.Time{}, []edit{before("<DatiDDT>", strings.Replace(sdiLinked, "%s", "2026-11-21", 1))}, nil, []string{"00418"}},
	{"00418 earlier", false, time.Time{}, []edit{before("<DatiDDT>", strings.Replace(sdiLinked, "%s", "2026-11-20", 1))}, nil, nil},
	{"00420", false, time.Time{}, []edit{repAll("<Natura>N1</Natura>", "<Natura>N6.1</Natura>"), after("<Imposta>0.00</Imposta>", "\n        <EsigibilitaIVA>S</EsigibilitaIVA>")}, nil, []string{"00420"}},
	{"00420 deferred", false, time.Time{}, []edit{repAll("<Natura>N1</Natura>", "<Natura>N6.1</Natura>"), after("<Imposta>0.00</Imposta>", "\n        <EsigibilitaIVA>D</EsigibilitaIVA>")}, nil, nil},
	{"00421", false, time.Time{}, []edit{rep("<Imposta>225.50</Imposta>", "<Imposta>225.52</Imposta>")}, nil, []string{"00421"}},
	{"00421 cent", false, time.Time{}, []edit{rep("<Imposta>225.50</Imposta>", "<Imposta>225.51</Imposta>")}, nil, nil},
	{"00422", false, time.Time{}, []edit{rep("<ImponibileImporto>1025.00</ImponibileImporto>\n        <Imposta>225.50</Imposta>", "<ImponibileImporto>1027.00</ImponibileImporto>\n        <Imposta>225.94</Imposta>")}, nil, []string{"00422"}},
	{"00422 euro", false, time.Time{}, []edit{rep("<ImponibileImporto>1025.00</ImponibileImporto>\n        <Imposta>225.50</Imposta>", "<ImponibileImporto>1026.00</ImponibileImporto>\n        <Imposta>225.72</Imposta>")}, nil, nil},
	{"00422 rounding", false, time.Time{}, []edit{rep("<ImponibileImporto>1025.00</ImponibileImporto>\n        <Imposta>225.50</Imposta>", "<Arrotondamento>2.00</Arrotondamento>\n        <ImponibileImporto>1027.00</ImponibileImporto>\n        <Imposta>225.94</Imposta>")}, nil, nil},
	{"00422 pension fund", true, time.Time{}, []edit{rep("<ImportoContributoCassa>40.00</ImportoContributoCassa>", "<ImportoContributoCassa>42.00</ImportoContributoCassa>")}, nil, []string{"00422"}},
	{"00423", false, time.Time{}, []edit{rep("<PrezzoTotale>900.00</PrezzoTotale>", "<PrezzoTotale>900.02</PrezzoTotale>")}, nil, []string{"00423"}},
	{"00423 amount", false, time.Time{}, []edit{rep("<PrezzoTotale>105.00</PrezzoTotale>", "<PrezzoTotale>105.50</PrezzoTotale>")}, nil, []string{"00423"}},
	{"00423 percentage and amount", false, time.Time{}, []edit{rep("<Percentuale>10.00</Percentuale>", "<Percentuale>10.00</Percentuale>\n          <Importo>60.00</Importo>")}, nil, nil},
	{"00423 no quantity", false, time.Time{}, []edit{rep("<Quantita>1.00</Quantita>\n        <PrezzoUnitario>20.00</PrezzoUnitario>", "<PrezzoUnitario>20.00</PrezzoUnitario>")}, nil, nil},
	{"00423 supplement", false, time.Time{}, []edit{
		rep("<PrezzoUnitario>20.00</PrezzoUnitario>\n        <PrezzoTotale>20.00</PrezzoTotale>", "<PrezzoUnitario>20.00</PrezzoUnitario>\n        <ScontoMaggiorazione>\n          <Tipo>MG</Tipo>\n          <Importo>5.00</Importo>\n        </ScontoMaggiorazione>\n        <PrezzoTotale>25.00</PrezzoTotale>"),
		rep("<ImponibileImporto>1025.00</ImponibileImporto>\n        <Imposta>225.50</Imposta>", "<ImponibileImporto>1030.00</ImponibileImporto>\n        <Imposta>226.60</Imposta>"),
	}, nil, nil},
	{"00424", false, time.Time{}, []edit{repAll("<AliquotaIVA>22.00</AliquotaIVA>", "<AliquotaIVA>0.22</AliquotaIVA>"), rep("<Imposta>225.50</Imposta>", "<Imposta>2.26</Imposta>")}, nil, []string{"00424"}},
	{"00425", false, time.Time{}, []edit{rep("<Numero>FT-2026/0042</Numero>", "<Numero>FT-ABC</Numero>")}, nil, []string{"00425"}},
	{"00427 FPR12", false, time.Time{}, []edit{rep("<CodiceDestinatario>ABC1234</CodiceDestinatario>", "<CodiceDestinatario>ABC123</CodiceDestinatario>")}, nil, []string{"00427"}},
	{"00427 FPA12", true, time.Time{}, []edit{rep("<CodiceDestinatario>UFABCD</CodiceDestinatario>", "<CodiceDestinatario>UFABCDE</CodiceDestinatario>")}, nil, []string{"00427"}},
	{"00428", false, time.Time{}, []edit{rep(`versione="FPR12"`, `versione="FPA12"`)}, nil, []string{"00428"}},
	{"00429", false, time.Time{}, []edit{rep("<Natura>N1</Natura>\n        <ImponibileImporto>", "<ImponibileImporto>")}, nil, []string{"00429", "00444"}},
	{"00430", false, time.Time{}, []edit{rep("<AliquotaIVA>22.00</AliquotaIVA>\n        <SpeseAccessorie>", "<AliquotaIVA>22.00</AliquotaIVA>\n        <Natura>N1</Natura>\n        <SpeseAccessorie>")}, nil, []string{"00430"}},
	{"00430 TD16", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD16</TipoDocumento>"), rep("<AliquotaIVA>22.00</AliquotaIVA>\n        <SpeseAccessorie>", "<AliquotaIVA>22.00</AliquotaIVA>\n        <Natura>N1</Natura>\n        <SpeseAccessorie>")}, nil, nil},
	{"00437", false, time.Time{}, []edit{before("<ImportoTotaleDocumento>", "<ScontoMaggiorazione>\n          <Tipo>SC</Tipo>\n        </ScontoMaggiorazione>\n        ")}, nil, []string{"00437"}},
	{"00438", false, time.Time{}, []edit{remove("\n          <Percentuale>10.00</Percentuale>")}, nil, []string{"00423", "00438"}},
	{"00443", false, time.Time{}, []edit{repN(3, "<AliquotaIVA>22.00</AliquotaIVA>", "<AliquotaIVA>10.00</AliquotaIVA>")}, nil, []string{"00422", "00443"}},
	{"00443 pension fund", true, time.Time{}, []edit{repN(1, "<AliquotaIVA>22.00</AliquotaIVA>", "<AliquotaIVA>10.00</AliquotaIVA>")}, nil, []string{"00422", "00443"}},
	{"00444", false, time.Time{}, []edit{rep(sdiLineNature, "<AliquotaIVA>0.00</AliquotaIVA>\n        <Natura>N2.1</Natura>\n      </DettaglioLinee>")}, nil, []string{"00444"}},
	{"00445", false, time.Time{}, []edit{repAll("<Natura>N1</Natura>", "<Natura>N2</Natura>")}, nil, []string{"00445"}},
	{"00471", false, time.Time{}, []edit{rep("<IdCodice>09876543217</IdCodice>", "<IdCodice>12345678903</IdCodice>")}, nil, []string{"00471"}},
	{"00471 codice fiscale", true, time.Time{}, []edit{rep("<CodiceFiscale>80012345676</CodiceFiscale>", "<CodiceFiscale>13579246805</CodiceFiscale>")}, nil, []string{"00471"}},
	{"00472", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD27</TipoDocumento>")}, nil, []string{"00472"}},
	{"00472 same subject", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD27</TipoDocumento>"), rep("<IdCodice>09876543217</IdCodice>", "<IdCodice>12345678903</IdCodice>")}, nil, nil},
	{"00473", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD17</TipoDocumento>")}, nil, []string{"00473"}},
	{"00473 TD17 abroad", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD17</TipoDocumento>"), repN(2, "<IdPaese>IT</IdPaese>", "<IdPaese>FR</IdPaese>")}, nil, nil},
	{"00473 TD29", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD29</TipoDocumento>"), repN(2, "<IdPaese>IT</IdPaese>", "<IdPaese>FR</IdPaese>")}, nil, []string{"00473"}},
	{"00474", false, time.Time{}, []edit{rep("<TipoDocumento>TD24</TipoDocumento>", "<TipoDocumento>TD21</TipoDocumento>"), rep("<IdCodice>09876543217</IdCodice>", "<IdCodice>12345678903</IdCodice>")}, nil, []string{"00474"}},
	{"00475", true, time.Time{}, []edit{rep("<TipoDocumento>TD01</TipoDocumento>", "<TipoDocumento>TD16</TipoDocumento>")}, nil, []string{"00475"}},
	{"00476", false, time.Time{}, []edit{repN(2, "<IdPaese>IT</IdPaese>", "<IdPaese>FR</IdPaese>"), repN(2, "<IdPaese>IT</IdPaese>", "<IdPaese>DE</IdPaese>")}, nil, []string{"00476"}},
}

func TestCheckSDI(t *testing.T) {
	bases := map[bool]string{false: load(t, fprBase), true: load(t, fpaBase)}
	for pa, base := range bases {
		if r := runSDI(t, base, sdiReceived); len(r.Findings) > 0 {
			t.Errorf("base (pa %v): %s", pa, r)
		}
	}
	inCatalog := map[string]bool{}
	for _, r := range SDICatalog {
		inCatalog[r.ID] = true
	}
	tested := map[string]bool{}
	for _, tc := range sdiCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mutate(t, bases[tc.pa], tc.edits...)
			if tc.second != nil {
				doc = twice(t, doc, tc.second...)
			}
			at := tc.at
			if at.IsZero() {
				at = sdiReceived
			}
			r := runSDI(t, doc, at)
			if got := r.Rules(); !slices.Equal(got, tc.want) {
				t.Errorf("rules %v, want %v\n%s", got, tc.want, r)
			}
			for _, f := range r.Findings {
				tested[f.Rule] = true
				if !inCatalog[f.Rule] || f.Source != rules.SourceSDI || f.Severity != rules.Error || f.Message == "" ||
					f.Line == 0 || !strings.HasPrefix(f.Path, "/p:FatturaElettronica/") {
					t.Errorf("finding %+v", f)
				}
			}
		})
	}
	for _, r := range SDICatalog {
		if !tested[r.ID] {
			t.Errorf("no case breaks %s", r.ID)
		}
	}
}

func TestSDICatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range SDICatalog {
		if seen[r.ID] || len(r.ID) != 5 || r.Severity != rules.Error || !strings.HasSuffix(r.Description, ".") {
			t.Errorf("rule %+v", r)
		}
		seen[r.ID] = true
	}
	if !slices.IsSortedFunc(SDICatalog, func(a, b rules.RuleInfo) int { return strings.Compare(a.ID, b.ID) }) {
		t.Error("SDICatalog is not in the order of the codes")
	}
}

// TestCheckSDINow checks that a zero reception is the current time: an
// invoice dated tomorrow is rejected.
func TestCheckSDINow(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 2).Format(time.DateOnly)
	doc := mutate(t, load(t, fprBase), rep("<Data>2026-11-20</Data>", "<Data>"+tomorrow+"</Data>"))
	if got := runSDI(t, doc, time.Time{}).Rules(); !slices.Equal(got, []string{"00403"}) {
		t.Errorf("rules %v, want [00403]", got)
	}
}
