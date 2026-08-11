package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ARCHITECTURE GATE (design §14.1).
//
// Two structural rules that are cheap to check and expensive to notice by review. Both held
// when this was written; the point is that they keep holding, because each describes a boundary
// that erodes silently — an import added in one file, in a PR about something else.
//
// Deliberately NOT here: line counts and field counts. The sweep that produced §14.1 flagged
// `BuildHandler` (630 lines) and `api.Server` (33 fields) from metrics alone, and reading the
// code showed both were correct as they stood. A threshold test on either would fire forever on
// something nobody should change.
//
// ⚠ **THESE GATES CAN SERVE A STALE PASS, AND THAT IS NOT FIXED — see GH #282.** Most of them
// learn about the tree by shelling out to `go list`, and subprocess output is not an input Go's
// test cache tracks. So moving a file in `internal/store` does not change package `internal`'s
// test binary, nothing invalidates the cached result, and the gate reports `ok (cached)` over a
// tree it never examined. Observed 2026-08-10: a deliberate sabotage that SHOULD have failed
// `TestNoLoomarrPackageLinksTestingIntoTheBinary` came back green, and only re-running with
// `-count=1` showed the real red.
//
// Two consequences, both live:
//   - **Verifying one of these by sabotage REQUIRES `-count=1`.** Without it you are reading a
//     result from before your change, which is the exact false-green this repo keeps being bitten
//     by (a gate that exits 0 having executed nothing).
//   - **CI restores a warm build cache**, so a PR that violates one of these rules can go green
//     on it. That is a real hole in a real guard, not a theoretical one.
//
// The durable fix is to have them read the tree with `os.ReadDir`/`os.ReadFile` — reads the cache
// DOES track — instead of parsing `go list` output. Two of the five already do.

// The dependency direction is one-way: domain packages must not import the API layer. A domain
// package that needs an API type is telling you the type belongs in the domain.
func TestDomainPackagesDoNotImportAPI(t *testing.T) {
	// `go list` over the domain packages, asking which of them depend on internal/api.
	out, err := exec.Command("go", "list", "-deps", "./playout/...", "./schedule/...", "./store/...",
		"./filler/...", "./channels/...", "./suggest/...", "./library/...", "./provision/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(line, "loomarr/internal/api") {
			t.Errorf("a domain package imports internal/api — the dependency direction is one-way "+
				"(§14.1); found %q in the transitive set", line)
		}
	}
}

// Test doubles must never reach the shipped binary. testkit exists so unit tests never touch
// the network; compiling it into production is a seam that only ever gets wider.
func TestProductionBinaryDoesNotLinkTestkit(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../cmd/loomarr").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	if strings.Contains(string(out), "loomarr/internal/testkit") {
		t.Error("cmd/loomarr links internal/testkit — test doubles must not ship (§14.1)")
	}
}

// NO Loomarr package linked into the binary may import `testing`. There is no exemption.
//
// ⚠ **There used to be one, for `internal/store`, and the reason it lasted was a mis-estimate
// rather than a constraint.** The conformance suite (7 files, ~4,450 lines — 42% of the non-test
// package) sat in ordinary .go files because BOTH backend drivers must reach `RunConformance` —
// SQLite in-package, Postgres behind the `integration` tag — so the assertions "had to be
// importable package code". The exit was written down as a ~4,450-line mechanical rename into a
// sibling package, and sequenced for later on that basis.
//
// The estimate assumed the wrong move. A sibling package would indeed have meant qualifying
// every `Store`, `Channel`, `ErrNotFound` … reference across 4,450 lines. But both drivers are
// ALREADY `_test.go` files in `package store`, and a test file is visible to every other test
// file in its package — so renaming `conformance*.go` to `conformance*_test.go` keeps them
// exactly where they are, keeps both drivers working, and takes them out of the production build
// with **zero content changes**. Done 2026-08-10; the diff is seven renames.
//
// The lesson is worth more than the fix: the blocker was never the code, it was a cost estimate
// nobody re-checked. It had been quoted twice, in this comment and in the plan that scheduled it.
//
// ⚠ `go list -deps ./cmd/loomarr` STILL reports `testing` and `flag`, and that is not ours —
// `riverqueue/river/rivershared/testsignal` imports `riversharedtest`, in the job-queue
// dependency. This gate deliberately scopes to `github.com/mantonx/loomarr/internal`: a rule we
// cannot act on is not a gate, it is noise. If River ever drops it, the raw `go list` becomes
// clean on its own; nothing here needs to change.
func TestNoLoomarrPackageLinksTestingIntoTheBinary(t *testing.T) {
	deps, err := exec.Command("go", "list", "-deps", "../cmd/loomarr").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Split(strings.TrimSpace(string(deps)), "\n") {
		if !strings.HasPrefix(pkg, "github.com/mantonx/loomarr/internal") {
			continue
		}
		imports, err := exec.Command("go", "list", "-f", "{{join .Imports \" \"}}", pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		for _, imp := range strings.Fields(string(imports)) {
			if imp != "testing" {
				continue
			}
			t.Errorf("%s imports `testing` and is linked into cmd/loomarr (§14.1). Put the "+
				"assertions in a _test.go file — a test file is visible to every other test file "+
				"in its own package, so shared helpers do NOT need to be ordinary package code.",
				pkg)
		}
	}
}

// The §14.2 package map lists every package. A map that silently goes stale is worse than no
// map: it reads as authoritative while quietly omitting whatever was added last.
//
// This checks PRESENCE, not prose — the one-line description is a human's job, and asserting on
// its wording would make the gate fire on every honest edit.
func TestPackageMapListsEveryPackage(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "docs", "design.md"))
	if err != nil {
		t.Fatalf("read design.md: %v", err)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.Contains(string(doc), "| `"+e.Name()+"` |") {
			t.Errorf("internal/%s is missing from the §14.2 package map — add a row saying what it does",
				e.Name())
		}
	}
}

// Every package carries a package doc. They are the orientation for subsystems whose invariants
// are not visible from their types; internal/playout is the clearest case, and was the one
// package missing one when §14.1 was written.
func TestEveryPackageHasAPackageDoc(t *testing.T) {
	// GoFiles counts NON-test files: a package that is only tests (this one, and
	// internal/integration) has no production code to document, and demanding a doc there
	// would be documenting the test harness rather than a subsystem.
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}\t{{len .GoFiles}}\t{{.Doc}}", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		goFiles, doc, _ := strings.Cut(rest, "\t")
		if goFiles == "0" || strings.TrimSpace(doc) != "" {
			continue
		}
		t.Errorf("%s has no package doc (§14.1) — write one where the subsystem's invariants live", path)
	}
}
