package internal_test

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A test that sleeps for real is slow when it passes and flaky when the machine is loaded: the
// sleep is either longer than needed or, one day, shorter. Wait on the event instead:
//
//   - a channel or a hook the code under test offers;
//   - testkit.Eventually for a state with neither;
//   - testing/synctest when the test is only about time (no subprocess or network waits);
//   - testkit.Clock for code that takes a `now func() time.Time`.
//
// The ledger testdata/test-sleeps.txt holds one "path count" line per test file that calls
// time.Sleep. A file may not gain a call, and a file that loses one lowers its line with
//
//	go test ./internal/ -run TestTestsDoNotSleep -lower-sleep-ledger
//
// which only ever lowers or deletes lines. A sleep inside a synctest bubble costs no real time but
// counts all the same: the scan does not follow calls to find the bubble.

const sleepLedger = "testdata/test-sleeps.txt"

var lowerSleepLedger = flag.Bool("lower-sleep-ledger", false,
	"rewrite "+sleepLedger+" to the current counts where they are lower")

// countSleeps is how many times a file calls time.Sleep, under whatever name it imports "time".
func countSleeps(t *testing.T, filename string, src any) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	name := ""
	for _, imp := range file.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path == "time" {
			name = "time"
			if imp.Name != nil {
				name = imp.Name.Name
			}
		}
	}
	if name == "" {
		return 0
	}
	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sleep" {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == name {
				count++
			}
		}
		return true
	})
	return count
}

// lowerCounts is the ledger lowered to the current counts. It never adds a file or raises a count.
func lowerCounts(ledger, counts map[string]int) map[string]int {
	lowered := map[string]int{}
	for file, allowed := range ledger {
		if n := min(allowed, counts[file]); n > 0 {
			lowered[file] = n
		}
	}
	return lowered
}

func parseSleepLedger(t *testing.T, content string) (header string, ledger map[string]int) {
	t.Helper()
	ledger = map[string]int{}
	for _, line := range strings.SplitAfter(content, "\n") {
		if strings.HasPrefix(line, "#") {
			header += line
			continue
		}
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		path, count, ok := strings.Cut(line, " ")
		n, err := strconv.Atoi(count)
		if !ok || err != nil {
			t.Fatalf("%s: %q is not a \"path count\" line", sleepLedger, line)
		}
		ledger[path] = n
	}
	return header, ledger
}

func TestTestsDoNotSleep(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	files, err := moduleTestFiles(root)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("found no _test.go files: the guard would pass over a tree it never examined")
	}
	counts := map[string]int{}
	for _, rel := range files {
		if n := countSleeps(t, filepath.Join(root, rel), nil); n > 0 {
			counts[filepath.ToSlash(rel)] = n
		}
	}

	content, err := os.ReadFile(sleepLedger)
	if err != nil {
		t.Fatal(err)
	}
	header, ledger := parseSleepLedger(t, string(content))
	if *lowerSleepLedger {
		ledger = lowerCounts(ledger, counts)
		var paths []string
		for path := range ledger {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		out := header
		for _, path := range paths {
			out += path + " " + strconv.Itoa(ledger[path]) + "\n"
		}
		if err := os.WriteFile(sleepLedger, []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for file, n := range counts {
		if allowed := ledger[file]; n > allowed {
			t.Errorf("%s calls time.Sleep %d time(s), the ledger allows %d. Wait on a channel or hook, "+
				"testkit.Eventually, testing/synctest or testkit.Clock instead.", file, n, allowed)
		}
	}
	for file, allowed := range ledger {
		if n := counts[file]; n < allowed {
			t.Errorf("%s calls time.Sleep %d time(s) against a ledger of %d: run "+
				"go test ./internal/ -run TestTestsDoNotSleep -lower-sleep-ledger", file, n, allowed)
		}
	}
}

func TestSleepScan(t *testing.T) {
	src := `package p
import (
	clock "time"
)
func f() {
	clock.Sleep(1)
	wait := clock.Sleep
	wait(1)
	_ = "time.Sleep(1)" // time.Sleep(1)
}`
	if got := countSleeps(t, "p_test.go", src); got != 2 {
		t.Fatalf("counted %d sleeps, want 2", got)
	}
	lowered := lowerCounts(map[string]int{"a": 3, "b": 1}, map[string]int{"a": 5, "c": 2})
	if len(lowered) != 1 || lowered["a"] != 3 {
		t.Fatalf("lowered to %v, want only a:3", lowered)
	}
}
