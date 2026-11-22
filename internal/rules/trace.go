//go:build !ruletrace

package rules

// tracing tells whether the package is built with the tag ruletrace, which
// records the rules of the findings (see trace_ruletrace.go).
const tracing = false

// Trace records that a finding of the rule was reported, for the measure of
// the rules that the test suite raises (TestRuleCoverage of the package
// zerechnung). Report.Add calls it for each finding; code that builds
// findings outside a Report calls it too. Without the tag ruletrace, it does
// nothing.
func Trace(rule string) {}
