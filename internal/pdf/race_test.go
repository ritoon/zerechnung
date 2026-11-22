//go:build race

package pdf

// raceEnabled is true when the tests run with the race detector, which
// makes sync.Pool drop items at random.
const raceEnabled = true
