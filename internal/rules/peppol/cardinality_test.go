package peppol

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ritoon/zerechnung/internal/rules"
	"github.com/ritoon/zerechnung/internal/xmltree"
)

// cardinalityDoc returns a document reduced to a context of the pattern
// inferred-mandatory-cardinality: the document element root
// ("ubl-invoice:Invoice" or "ubl-creditnote:CreditNote"), the elements of
// path below it, and n elements child in the last one.
func cardinalityDoc(root, path, child string, n int) string {
	name, space := "Invoice", nsInvoice
	if root == "ubl-creditnote:CreditNote" {
		name, space = "CreditNote", nsCreditNote
	}
	var open, end string
	if path != "" {
		for _, e := range strings.Split(path, "/") {
			open += "<" + e + ">"
			end = "</" + e + ">" + end
		}
	}
	return "<" + name + ` xmlns="` + space + `" xmlns:cac="` + nsCac + `" xmlns:cbc="` + nsCbc + `">` +
		open + strings.Repeat("<"+child+">1</"+child+">", n) + end + "</" + name + ">"
}

// checkCardinality checks the rule id of the pattern, of context ctx
// ("ubl-invoice:Invoice/cac:PaymentTerms") and child, on documents reduced
// to the context, with check running the rules: the rule fires once when
// the child is missing or doubled, never when it appears once, nor when the
// path is below the other document element.
func checkCardinality(t *testing.T, check func([]byte) *rules.Report, id, ctx, child string) {
	t.Helper()
	root, path, _ := strings.Cut(ctx, "/")
	other := "ubl-creditnote:CreditNote"
	if root == other {
		other = "ubl-invoice:Invoice"
	}
	for _, c := range []struct {
		root    string
		n, want int
	}{{root, 0, 1}, {root, 1, 0}, {root, 2, 1}, {other, 0, 0}} {
		r := check([]byte(cardinalityDoc(c.root, path, child, c.n)))
		got := 0
		for _, f := range r.Findings {
			if f.Rule == id {
				got++
			}
		}
		if got != c.want {
			t.Errorf("%s: %s below %s with %d %s fired %d times, want %d\n%s", id, path, c.root, c.n, child, got, c.want, r)
		}
	}
}

// TestCardinality checks each rule of cardinalityContexts, with the rules of
// Billing and with those of Self-Billing, which has the same: its message,
// taken from the schematron, names its context and its child, and it fires
// when the child is missing or doubled.
func TestCardinality(t *testing.T) {
	billing := func(b []byte) *rules.Report {
		r := runUBL(t, b)
		checkUBLFindings(t, r)
		return r
	}
	sb := func(b []byte) *rules.Report {
		r := checkSelfBilling(t, b)
		checkSBFindings(t, r)
		return r
	}
	n := 0
	for _, c := range cardinalityContexts {
		ctx := "ubl-invoice:Invoice"
		if c.space == nsCreditNote {
			ctx = "ubl-creditnote:CreditNote"
		}
		if c.path != "" {
			ctx += "/" + c.path
		}
		for _, r := range c.rules {
			n++
			want := ctx + " MUST contain exactly one " + r.child + "."
			for _, info := range []*rules.RuleInfo{ublByID()[r.id], sbByID()[r.id]} {
				if info == nil || info.Description != want || info.Severity != rules.Error {
					t.Errorf("%s: catalog entry %+v, want an error %q", r.id, info, want)
				}
			}
			checkCardinality(t, billing, r.id, ctx, r.child)
			checkCardinality(t, sb, r.id, ctx, r.child)
		}
	}
	if n != 120 {
		t.Errorf("%d rules of inferred cardinality, want 120", n)
	}
}

// officialCardinality checks the rules of the pattern
// inferred-mandatory-cardinality of the schematron file on documents reduced
// to their context, with check running the rules, and returns their
// identifiers.
func officialCardinality(t *testing.T, file string, check func([]byte) *rules.Report) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := xmltree.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	test := regexp.MustCompile(`^count\(([a-z]+:[A-Za-z]+)\) = 1$`)
	var ids []string
	sch.Walk(func(p *xmltree.Node) bool {
		if p.Name != "pattern" || p.Attr("id") != "inferred-mandatory-cardinality" {
			return true
		}
		for _, rule := range p.Elems("rule") {
			for _, a := range rule.Elems("assert") {
				id := a.Attr("id")
				m := test.FindStringSubmatch(a.Attr("test"))
				if m == nil {
					t.Errorf("%s: unexpected test %q", id, a.Attr("test"))
					continue
				}
				checkCardinality(t, check, id, rule.Attr("context"), m[1])
				ids = append(ids, id)
			}
		}
		return false
	})
	if len(ids) == 0 {
		t.Errorf("%s: no rule of inferred cardinality", file)
	}
	return ids
}
