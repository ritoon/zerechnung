package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 6710: "6,710", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBadge(t *testing.T) {
	cs := []comparison{
		{Reference: "kosit", Invoice: "kosit/a.xml"},
		{Reference: "mustang", Invoice: "kosit/a.xml"},
		{Reference: "mustang", Invoice: "corpus/b.pdf", Known: []string{"+BR-01: reason"}},
		{Reference: "pt", Invoice: "pt/espap/c.xml"},
	}
	b := newBadge(cs)
	if b.Color != "brightgreen" || b.Message != "3 validators · 3 invoices · 0 unexplained" {
		t.Errorf("all justified: got %q, %q", b.Color, b.Message)
	}

	cs = append(cs, comparison{Reference: "npa", Invoice: "nl/d.xml", Unexpected: []string{"+BR-NL-1"}})
	b = newBadge(cs)
	if b.Color != "red" || b.Message != "1 unexplained difference of 4 invoices" {
		t.Errorf("one unexpected: got %q, %q", b.Color, b.Message)
	}

	dir := t.TempDir()
	in, out := filepath.Join(dir, "report.json"), filepath.Join(dir, "badge.json")
	data, _ := json.Marshal(report{Comparisons: cs[:4]})
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := badge(in, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var e endpoint
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	if e.SchemaVersion != 1 || e.Label != "reference parity" || e.Color != "brightgreen" {
		t.Errorf("written badge: %+v", e)
	}

	if err := os.WriteFile(in, []byte(`{"comparisons":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := badge(in, out); err == nil {
		t.Error("empty report: no error")
	}
}
