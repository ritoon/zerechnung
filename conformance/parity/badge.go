package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// endpoint is the JSON that the endpoint badges of shields.io read
// (https://shields.io/badges/endpoint-badge).
type endpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
	CacheSeconds  int    `json:"cacheSeconds,omitempty"`
}

// summarize counts, in the comparisons of a report, the reference
// validators, the invoices compared with at least one of them, and the
// comparisons with a difference that known.txt does not justify.
func summarize(cs []comparison) (refs, invoices, unexpected int) {
	seenRef := map[string]bool{}
	seenInv := map[string]bool{}
	for _, c := range cs {
		seenRef[c.Reference] = true
		seenInv[c.Invoice] = true
		if len(c.Unexpected) > 0 {
			unexpected++
		}
	}
	return len(seenRef), len(seenInv), unexpected
}

// newBadge describes the comparisons of a report in one badge: green when
// every difference is justified, red otherwise.
func newBadge(cs []comparison) endpoint {
	refs, invoices, unexpected := summarize(cs)
	b := endpoint{SchemaVersion: 1, Label: "reference parity", CacheSeconds: 3600}
	if unexpected == 0 {
		b.Color = "brightgreen"
		b.Message = fmt.Sprintf("%s · %s invoices · 0 unexplained",
			plural(refs, "validator", "validators"), thousands(invoices))
	} else {
		b.Color = "red"
		b.Message = fmt.Sprintf("%s of %s invoices",
			plural(unexpected, "unexplained difference", "unexplained differences"), thousands(invoices))
	}
	return b
}

// badge reads the JSON report of compare and writes the badge to out.
func badge(reportFile, out string) error {
	data, err := os.ReadFile(reportFile)
	if err != nil {
		return err
	}
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("%s: %w", reportFile, err)
	}
	if len(r.Comparisons) == 0 {
		return fmt.Errorf("%s: no comparison", reportFile)
	}
	b, err := json.Marshal(newBadge(r.Comparisons))
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(b, '\n'), 0o644)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

// thousands writes n with a comma between groups of three digits.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
