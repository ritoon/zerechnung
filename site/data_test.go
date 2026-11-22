package main

import (
	"testing"
)

func TestNumbers(t *testing.T) {
	en := &page{Lang: &lang{Code: "en"}}
	fr := &page{Lang: &lang{Code: "fr"}}
	tests := []struct {
		p    *page
		us   float64
		want string
	}{
		{en, 3, "3.0\u00a0µs"},
		{fr, 30.2, "30,2\u00a0µs"},
		{fr, 124, "124\u00a0µs"},
		{fr, 1140, "1,14\u00a0ms"},
		{en, 14650, "14.65\u00a0ms"},
		{en, 0.496, "496\u00a0ns"},
		{en, 2.67e6, "2.67\u00a0s"},
	}
	for _, tt := range tests {
		if got := tt.p.Dur(tt.us); got != tt.want {
			t.Errorf("%s: Dur(%v) = %q, want %q", tt.p.Lang.Code, tt.us, got, tt.want)
		}
	}
	if got := fr.Tick(1000); got != "1\u00a0ms" {
		t.Errorf("Tick(1000) = %q", got)
	}
	if got := fr.Times(2.49); got != "×2,5" {
		t.Errorf("Times(2.49) = %q", got)
	}
	if got := en.Times(15.3); got != "×15" {
		t.Errorf("Times(15.3) = %q", got)
	}
}

func TestLoadBench(t *testing.T) {
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	d, err := loadBench(dataFS, langs)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range append(d.Go, d.Poly...) {
		if len(c.Ticks) < 2 || c.Ticks[0].At != 0 || c.Ticks[len(c.Ticks)-1].At != 100 {
			t.Errorf("%s %s: ticks %v do not run from 0 to 100", c.Op, c.Input, c.Ticks)
		}
		for _, r := range c.Rows {
			if r.Fail == "" && (r.Width <= 0 || r.Width > 100) {
				t.Errorf("%s %s: %s: width %v", c.Op, c.Input, r.Lib, r.Width)
			}
		}
	}
	if len(d.Go) == 0 || d.Go[0].Gain <= 1 {
		t.Errorf("no gain on the first Go chart")
	}
}

func TestCountriesSortByName(t *testing.T) {
	p := &page{
		Lang: &lang{Code: "de"},
		P:    map[string]string{"country_AT": "Österreich", "country_NL": "Niederlande", "country_PL": "Polen"},
		Comp: &compData{Countries: []*compCountry{{Code: "AT", EU: true}, {Code: "PL", EU: true}, {Code: "NL", EU: true}}},
	}
	var got []string
	for _, c := range p.Countries(true) {
		got = append(got, c.Code)
	}
	if want := "NL AT PL"; joinSpace(got) != want {
		t.Errorf("got %q, want %q", joinSpace(got), want)
	}
}

func joinSpace(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}

func TestLoadComp(t *testing.T) {
	langs, err := loadLangs(i18nFS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadComp(dataFS, langs); err != nil {
		t.Fatal(err)
	}
}
