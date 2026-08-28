package internal_test

import (
	"os"
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
// the composition root and `api.Server` from metrics alone, and reading the
// code showed both were correct as they stood. A threshold test on either would fire forever on
// something nobody should change.
//
// ⚠ **These gates USED TO SERVE A STALE PASS (GH #282, fixed 2026-08-10) — do not undo it by
// reaching for `exec.Command("go", "list", ...)`.** They learned about the tree by shelling out,
// and subprocess output is not an input Go's test cache tracks: moving a file in `internal/store`
// did not change package `internal`'s test binary, so nothing invalidated the cached result and
// the gate reported `ok (cached)` over a tree it never examined. CI restores a warm cache, so a
// violating PR could go green on it.
//
// They now read the tree from SOURCE (importgraph_test.go), which the cache DOES track. The
// difference is directly observable, and worth repeating if you touch this:
//
//	git mv internal/store/conformance_titles_test.go internal/store/conformance_titles.go
//	go test ./internal/ -run TestNoLoomarrPackageLinksTestingIntoTheBinary   # NO -count=1
//
// Before: `ok (cached)`. After: FAIL, correctly, on a warm cache. It is also ~50x faster
// (1.0s → 0.02s), because nothing spawns a subprocess per package any more.

// The dependency direction is one-way: domain packages must not import the API layer. A domain
// package that needs an API type is telling you the type belongs in the domain.
func TestDomainPackagesDoNotImportAPI(t *testing.T) {
	pkgs := loomarrPackages(t)
	const api = modulePath + "/internal/api"

	for _, domain := range []string{
		"playout", "schedule", "store", "filler", "channels", "suggest", "library", "provision",
	} {
		root := modulePath + "/internal/" + domain
		if _, ok := pkgs[root]; !ok {
			// ⚠ A domain package that vanished from the graph must FAIL, not silently pass.
			// A renamed package would otherwise remove itself from its own gate.
			t.Errorf("%s is not in the import graph — renamed or moved? Update this list, "+
				"because a domain package missing from it is a domain package nobody checks", root)
			continue
		}
		if reachable := reachableFrom(pkgs, root); reachable[api] {
			t.Errorf("%s transitively imports internal/api — the dependency direction is one-way "+
				"(§14.1). A domain package that needs an API type is telling you the type belongs "+
				"in the domain.", root)
		}
	}
}

// The domain must not import the HTTP FRAMEWORK either, which is the same rule one level down.
//
// ⚠ **The gate above bans `internal/api` and that was not enough.** `internal/schedule` — 4,000-odd
// lines of I/O-free scheduling logic, the purest package in the tree — imported `huma/v2` to
// describe one field's JSON encoding (`func (Duration) Schema(huma.Registry) *huma.Schema`). It
// passed every gate: it does not import `internal/api`, so the dependency-direction test was
// satisfied while the pure domain depended on the transport's framework.
//
// The wire description now lives in `internal/api/durationwire.go` and is attached with
// `Registry.RegisterTypeAlias`. This test is what stops the next one — a rule about a specific
// package name could not have caught it, so the rule is about the LAYER.
func TestDomainPackagesDoNotImportTheHTTPFramework(t *testing.T) {
	pkgs := loomarrPackages(t)

	// Transport-shaped dependencies a pure domain package has no business holding. `net/http` is
	// deliberately NOT here: several domain packages are legitimate HTTP CLIENTS (library, tmdb,
	// programmer talk to real services). The rule is about SERVING, not about the protocol.
	frameworks := []string{
		"github.com/danielgtaylor/huma/v2",
		"github.com/danielgtaylor/huma/v2/adapters/humago",
	}

	for _, domain := range []string{
		"playout", "schedule", "store", "filler", "channels", "suggest", "library", "provision",
	} {
		root := modulePath + "/internal/" + domain
		if _, ok := pkgs[root]; !ok {
			t.Errorf("%s is not in the import graph — renamed or moved? Update this list", root)
			continue
		}
		reachable := reachableFrom(pkgs, root)
		for _, fw := range frameworks {
			if importers := importersOf(pkgs, reachable, fw); len(importers) > 0 {
				t.Errorf("%s reaches the HTTP framework %s through %v (§14.1). A domain type that "+
					"needs a wire format should get one in internal/api — see durationwire.go, "+
					"which attaches a schema with Registry.RegisterTypeAlias instead of putting a "+
					"huma method on the domain type.", root, fw, importers)
			}
		}
	}
}

// Test doubles must never reach the shipped binary. testkit exists so unit tests never touch
// the network; compiling it into production is a seam that only ever gets wider.
func TestProductionBinaryDoesNotLinkTestkit(t *testing.T) {
	pkgs := loomarrPackages(t)
	linked := reachableFrom(pkgs, modulePath+"/cmd/loomarr")

	if importers := importersOf(pkgs, linked, modulePath+"/internal/testkit"); len(importers) > 0 {
		t.Errorf("cmd/loomarr links internal/testkit through %v — test doubles must not ship (§14.1)",
			importers)
	}
}

// The Image service's pixel boundary is the required Rust worker (§22). Keeping Go's image
// codecs out of this package prevents a certification helper or a convenient local decode from
// quietly becoming a second production implementation. Other domains can own measured, bounded
// frame analysis independently; this gate is specifically the Image service boundary.
func TestImageServiceDoesNotImportGoPixelCode(t *testing.T) {
	pkgs := loomarrPackages(t)
	imageService := modulePath + "/internal/images"
	pkg, ok := pkgs[imageService]
	if !ok {
		t.Fatalf("%s is not in the import graph", imageService)
	}
	for _, imported := range pkg.Imports {
		if imported == "image" || strings.HasPrefix(imported, "image/") {
			t.Errorf("%s imports %s — §22 assigns Image-service pixel work to the required Rust worker", imageService, imported)
		}
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
// dependency. This gate deliberately scopes to `github.com/loomarr/loomarr/internal`: a rule we
// cannot act on is not a gate, it is noise. If River ever drops it, the raw `go list` becomes
// clean on its own; nothing here needs to change.
func TestNoLoomarrPackageLinksTestingIntoTheBinary(t *testing.T) {
	pkgs := loomarrPackages(t)
	linked := reachableFrom(pkgs, modulePath+"/cmd/loomarr")

	for _, pkg := range importersOf(pkgs, linked, "testing") {
		t.Errorf("%s imports `testing` and is linked into cmd/loomarr (§14.1). Put the "+
			"assertions in a _test.go file — a test file is visible to every other test file "+
			"in its own package, so shared helpers do NOT need to be ordinary package code.",
			pkg)
	}
}

// The §14.2 package map lists every package. A map that silently goes stale is worse than no
// map: it reads as authoritative while quietly omitting whatever was added last.
//
// This checks PRESENCE, not prose — the one-line description is generated from each package's
// doc comment, and asserting on its wording would make the gate fire on every honest edit.
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
		files, err := os.ReadDir(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read internal/%s: %v", e.Name(), err)
		}
		hasProductionGo := false
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") && !strings.HasSuffix(f.Name(), "_test.go") {
				hasProductionGo = true
				break
			}
		}
		if !hasProductionGo {
			continue
		}
		if !strings.Contains(string(doc), "**`"+e.Name()+"`**") {
			t.Errorf("internal/%s is missing from the §14.2 package map — add a row saying what it does",
				e.Name())
		}
	}
}

// Every package carries a package doc. They are the orientation for subsystems whose invariants
// are not visible from their types; internal/playout is the clearest case, and was the one
// package missing one when §14.1 was written.
func TestEveryPackageHasAPackageDoc(t *testing.T) {
	for path, pkg := range loomarrPackages(t) {
		// GoFiles counts NON-test files: a package that is only tests (this one, and
		// internal/integration) has no production code to document, and demanding a doc there
		// would be documenting the test harness rather than a subsystem.
		if len(pkg.GoFiles) == 0 || strings.TrimSpace(pkg.Doc) != "" {
			continue
		}
		t.Errorf("%s has no package doc (§14.1) — write one where the subsystem's invariants live", path)
	}
}
