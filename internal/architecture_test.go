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

// `testing` DOES reach the shipped binary, through exactly one package, and this pins it there.
//
// `internal/store`'s conformance suite (7 files, ~4,450 lines — 42% of the non-test package)
// lives in ordinary .go files rather than _test.go ones, and that is deliberate: BOTH backend
// drivers must import `RunConformance` — SQLite in-package, Postgres behind a build tag — so the
// assertions have to be importable package code. The consequence is that `go list -deps
// ./cmd/loomarr` contains `testing`, and `flag` behind it.
//
// That sits awkwardly beside the rule above, whose stated principle is that test code in the
// binary "is a seam that only ever gets wider". The principle is right; the answer is not to
// delete a working two-backend suite, it is to stop the seam widening. So: one package may do
// this, it is named here, and a second one fails.
//
// ⚠ The exit is known and NOT blocked on design — verified 2026-08-10 that the conformance files
// reference ZERO unexported store identifiers, so they can move to a sibling package that only
// the two drivers import, and `testing` leaves the binary entirely. That move is a ~4,450-line
// mechanical rename across the tree's highest-churn files, so it is sequenced rather than done
// opportunistically. Until then, this test is the thing standing between "one documented
// exemption" and "test code is normal in production packages now".
func TestOnlyStoreLinksTestingIntoTheBinary(t *testing.T) {
	const allowed = "github.com/mantonx/loomarr/internal/store"

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
			if imp != "testing" || pkg == allowed {
				continue
			}
			t.Errorf("%s imports `testing` and is linked into cmd/loomarr — only %s may do that, "+
				"and only because both store conformance drivers must import the suite (§14.1). "+
				"Put the assertions in a _test.go file, or in a package the binary does not reach.",
				pkg, allowed)
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
