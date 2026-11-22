package zerechnung_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ritoon/zerechnung"
	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/facturae"
	"github.com/ritoon/zerechnung/fatturapa"
	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/ubl"
	"github.com/ritoon/zerechnung/validate"
)

// TestConcurrentAPI calls the public API from many goroutines on shared
// inputs (the same byte slices, the same parsed invoice). Each result must
// equal the result of a sequential run, and no input may change. Under -race,
// as the CI runs it, it also catches unsynchronized accesses.
func TestConcurrentAPI(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	host := read("host.pdf")
	hybrid := read("EN16931_Einfach.pdf")
	xmlNames := []string{
		"EN16931_Einfach.cii.xml", "EN16931_Einfach.ubl.xml", "EN16931_Rabatte.cii.xml",
		"XRECHNUNG_Einfach.cii.xml", "XRECHNUNG_Einfach.ubl.xml",
		"ZUGFeRD_1p0_COMFORT_SEPA_Prenotification.xml", "ubl-tc434-creditnote1.xml",
	}
	xmls := map[string][]byte{}
	for _, n := range xmlNames {
		xmls[n] = read(n)
	}
	inputs := map[string][]byte{"host.pdf": host, "EN16931_Einfach.pdf": hybrid}
	maps.Copy(inputs, xmls)
	pristine := map[string][]byte{}
	for n, b := range inputs {
		pristine[n] = bytes.Clone(b)
	}

	date := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	all := &validate.Options{Rules: validate.Default | validate.France | validate.Germany |
		validate.GermanyProfile | validate.Peppol | validate.Netherlands | validate.Italy}

	// One parsed invoice, read by every Marshal call at the same time.
	shared, err := cii.Parse(xmls["EN16931_Einfach.cii.xml"])
	if err != nil {
		t.Fatal(err)
	}

	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:8]) }
	errText := func(err error) string {
		if err == nil {
			return "<nil>"
		}
		return err.Error()
	}
	att := func(a *zerechnung.Attachment) string {
		if a == nil {
			return "<nil>"
		}
		m := "<nil>"
		if a.Metadata != nil {
			m = fmt.Sprintf("%+v", *a.Metadata)
		}
		return fmt.Sprintf("%s %s %s %s %q %s %t %s %s", a.Filename, sum(a.XML), a.Relationship,
			a.MimeType, a.Description, a.ModDate, a.InAF, m, a.PDFVersion)
	}
	rep := func(r *validate.Report) string {
		if r == nil {
			return "<nil>"
		}
		return r.String()
	}
	roundTrip := func(parse func([]byte) (*invoice.Invoice, error), data []byte) string {
		inv, err := parse(data)
		if err != nil {
			return errText(err)
		}
		out, err := cii.Marshal(inv, nil)
		return sum(out) + " " + errText(err)
	}

	type op struct {
		name string
		run  func() string
	}
	marshalCII := op{"cii.Marshal", func() string { out, err := cii.Marshal(shared, nil); return sum(out) + errText(err) }}
	ops := []op{
		{"Read", func() string {
			inv, a, err := zerechnung.Read(hybrid)
			n := ""
			if inv != nil {
				n = inv.Number + " " + inv.Totals.DuePayable.String()
			}
			return n + " " + att(a) + " " + errText(err)
		}},
		{"Check", func() string {
			r, a, err := zerechnung.Check(hybrid, all)
			return rep(r) + att(a) + errText(err)
		}},
		{"Check/no invoice", func() string {
			r, a, err := zerechnung.Check(host, nil)
			return rep(r) + att(a) + errText(err)
		}},
		{"CheckContainer", func() string {
			a, fs, err := zerechnung.CheckContainer(hybrid)
			return att(a) + fmt.Sprint(fs) + errText(err)
		}},
		{"ExtractAll", func() string {
			as, err := zerechnung.ExtractAll(hybrid)
			var b strings.Builder
			for _, a := range as {
				b.WriteString(att(a))
			}
			return b.String() + errText(err)
		}},
		{"Embed", func() string {
			out, err := zerechnung.Embed(host, xmls["EN16931_Einfach.ubl.xml"],
				&zerechnung.EmbedOptions{Profile: invoice.ProfileEN16931, ModDate: date})
			return sum(out) + " " + errText(err)
		}},
		{"Embed/ConvertPDFA", func() string {
			out, err := zerechnung.Embed(hybrid, xmls["EN16931_Einfach.cii.xml"],
				&zerechnung.EmbedOptions{ModDate: date, ConvertPDFA: true, RequirePDFA: true})
			return sum(out) + " " + errText(err)
		}},
		{"Create", func() string {
			// Create with Calculate fills in the amounts of the invoice:
			// each call has its own.
			out, err := zerechnung.Create(host, newInvoice(), &zerechnung.CreateOptions{
				Calculate: true,
				Validate:  &validate.Options{Rules: validate.Default | validate.France},
				Embed:     zerechnung.EmbedOptions{ModDate: date},
			})
			return sum(out) + " " + errText(err)
		}},
		{"ConvertPDFA", func() string {
			out, err := zerechnung.ConvertPDFA(hybrid, &zerechnung.ConvertOptions{ModDate: date})
			return sum(out) + " " + errText(err)
		}},
		{"ConvertPDFA/refused", func() string {
			out, err := zerechnung.ConvertPDFA(host, &zerechnung.ConvertOptions{ModDate: date})
			return sum(out) + " " + errText(err)
		}},
		marshalCII,
		{"ubl.Marshal", func() string { out, err := ubl.Marshal(shared, nil); return sum(out) + errText(err) }},
		{"facturae.Marshal", func() string {
			out, err := facturae.Marshal(shared, nil)
			if err != nil {
				return errText(err)
			}
			return sum(out) + roundTrip(facturae.Parse, out) + rep(validate.XML(out, all))
		}},
		{"fatturapa.Marshal", func() string {
			out, err := fatturapa.Marshal(shared, nil)
			if err != nil {
				return errText(err)
			}
			return sum(out) + roundTrip(fatturapa.Parse, out) + rep(validate.XML(out, all))
		}},
	}
	for _, n := range xmlNames {
		data := xmls[n]
		ops = append(ops,
			op{"DetectProfile " + n, func() string { p, err := zerechnung.DetectProfile(data); return fmt.Sprint(p) + errText(err) }},
			op{"validate.XML " + n, func() string { return rep(validate.XML(data, all)) }},
		)
		switch {
		case strings.Contains(n, ".ubl.") || strings.HasPrefix(n, "ubl-"):
			ops = append(ops, op{"ubl.Parse " + n, func() string { return roundTrip(ubl.Parse, data) }})
		case strings.Contains(n, ".cii."):
			ops = append(ops, op{"cii.Parse " + n, func() string { return roundTrip(cii.Parse, data) }})
		}
	}

	sharedBefore := marshalCII.run()
	want := make([]string, len(ops))
	for i, o := range ops {
		want[i] = o.run()
	}

	workers := 4 * runtime.GOMAXPROCS(0)
	const rounds = 3
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		mismatch = map[string]int{}
		start    = make(chan struct{})
	)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for r := range rounds {
				for k := range ops {
					// Each worker runs the operations from another start,
					// so that different operations overlap.
					i := (k + w*7 + r*3) % len(ops)
					if got := ops[i].run(); got != want[i] {
						mu.Lock()
						mismatch[ops[i].name]++
						mu.Unlock()
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	for name, n := range mismatch {
		t.Errorf("%s: %d concurrent results differ from the sequential one", name, n)
	}
	for n, b := range inputs {
		if !bytes.Equal(b, pristine[n]) {
			t.Errorf("input %s was modified", n)
		}
	}
	if marshalCII.run() != sharedBefore {
		t.Error("the shared invoice was modified")
	}
}
