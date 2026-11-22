// Package initcost measures what importing the packages of the module costs
// at startup: the heap that the initialization of their package-level
// variables allocates, paid by every program that imports them, before any
// call.
package initcost

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// budgets are the bytes that the initialization of the packages of the
// module may allocate, for a program that imports the package alone.
var budgets = []struct {
	pkg    string
	budget int
}{
	{"cii", 100 << 10},
	{"ubl", 100 << 10},
	{"validate", 300 << 10},
	{"zerechnung", 300 << 10},
}

// TestImportCost builds a program that imports each package and nothing
// else (testdata/<package>), runs it with GODEBUG=inittrace=1 and checks
// that the initialization of the packages of the module stays within the
// budget. The packages of the standard library are reported, not counted:
// their cost depends on the version of Go.
func TestImportCost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	gotool := filepath.Join(os.Getenv("GOROOT"), "bin", "go")
	if _, err := os.Stat(gotool); err != nil {
		gotool = "go"
	}
	dir := t.TempDir()
	for _, b := range budgets {
		t.Run(b.pkg, func(t *testing.T) {
			bin := filepath.Join(dir, b.pkg)
			if out, err := exec.Command(gotool, "build", "-o", bin, "./testdata/"+b.pkg).CombinedOutput(); err != nil {
				t.Fatalf("go build: %v\n%s", err, out)
			}
			cmd := exec.Command(bin)
			cmd.Env = append(os.Environ(), "GODEBUG=inittrace=1")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", b.pkg, err, out)
			}
			own, std := parseInitTrace(t, out)
			total := 0
			for _, c := range own {
				total += c.bytes
			}
			stdTotal := 0
			for _, c := range std {
				stdTotal += c.bytes
			}
			t.Logf("%s: %d bytes in %d packages of the module, %d bytes in the standard library", b.pkg, total, len(own), stdTotal)
			if total > b.budget {
				for _, c := range own {
					t.Logf("  %-50s %8d bytes %6d allocs", c.pkg, c.bytes, c.allocs)
				}
				t.Errorf("importing %s allocates %d bytes at initialization, over the budget of %d", b.pkg, total, b.budget)
			}
		})
	}
}

type initCost struct {
	pkg           string
	bytes, allocs int
}

// initLine matches a line of GODEBUG=inittrace=1:
// "init internal/bytealg @0.002 ms, 0 ms clock, 0 bytes, 0 allocs".
var initLine = regexp.MustCompile(`^init (\S+) @\S+ ms, \S+ ms clock, (\d+) bytes, (\d+) allocs`)

// parseInitTrace returns the cost of the packages of the module, and of the
// others.
func parseInitTrace(t *testing.T, out []byte) (own, std []initCost) {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		m := initLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		a, _ := strconv.Atoi(m[3])
		c := initCost{pkg: m[1], bytes: n, allocs: a}
		if strings.HasPrefix(c.pkg, "github.com/ritoon/zerechnung") {
			own = append(own, c)
		} else {
			std = append(std, c)
		}
	}
	return own, std
}
