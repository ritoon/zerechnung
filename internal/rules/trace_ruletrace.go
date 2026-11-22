//go:build ruletrace

package rules

import (
	"fmt"
	"os"
	"sync"
)

// TraceEnv names the environment variable that gives the file to which a
// test binary built with the tag ruletrace appends the rules of the findings
// it reports, one per line, each rule once per binary:
//
//	ZERECHNUNG_RULE_TRACE=$PWD/rules.txt go test -tags ruletrace ./...
//
// The test binaries of the packages run at the same time: each line is
// written in one append, which keeps the lines whole.
const TraceEnv = "ZERECHNUNG_RULE_TRACE"

const tracing = true

var trace struct {
	sync.Mutex
	once sync.Once
	file *os.File
	seen map[string]bool
}

// Trace records that a finding of the rule was reported: it appends the rule
// to the file of ZERECHNUNG_RULE_TRACE the first time the binary reports
// it. Without the variable, it does nothing.
func Trace(rule string) {
	trace.once.Do(func() {
		name := os.Getenv(TraceEnv)
		if name == "" {
			return
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			panic(fmt.Sprintf("%s: %v", TraceEnv, err))
		}
		trace.file, trace.seen = f, map[string]bool{}
	})
	if trace.file == nil {
		return
	}
	trace.Lock()
	defer trace.Unlock()
	if trace.seen[rule] {
		return
	}
	trace.seen[rule] = true
	if _, err := trace.file.WriteString(rule + "\n"); err != nil {
		panic(fmt.Sprintf("%s: %v", TraceEnv, err))
	}
}
