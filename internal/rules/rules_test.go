package rules

import (
	"encoding/json"
	"testing"
)

func TestSeverityJSON(t *testing.T) {
	in := []Finding{{Rule: "BR-01", Severity: Error}, {Rule: "BR-02", Severity: Warning}, {Rule: "BR-03", Severity: Info}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out []Finding
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("%d: %+v, want %+v", i, out[i], in[i])
		}
	}
	var s Severity
	if err := s.UnmarshalText([]byte("fatal")); err == nil {
		t.Error("unknown severity accepted")
	}
}
