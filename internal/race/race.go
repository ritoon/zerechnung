//go:build race

// Package race tells the tests whether they run with the race detector,
// which allocates on its own: the allocation budgets of the tests are
// skipped under -race.
package race

// Enabled is true when the race detector is on.
const Enabled = true
