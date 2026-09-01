package internal_test

import (
	"go/build"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var inboundDependencyExceptions = map[string]string{
	modulePath + "/internal/api": "owns the inbound HTTP adapter and its Huma wire definitions",
	modulePath + "/internal/app": "is the composition root that wires the inbound adapter to domain modules",
}

func TestInboundDependencyRuleIncludesNewNestedProductionPackages(t *testing.T) {
	api := modulePath + "/internal/api"
	app := modulePath + "/internal/app"
	nested := modulePath + "/internal/newdomain/nested"
	safe := modulePath + "/internal/safe"
	testOnly := modulePath + "/internal/testonly"
	pkgs := map[string]*build.Package{
		api:      {GoFiles: []string{"api.go"}},
		app:      {GoFiles: []string{"app.go"}, Imports: []string{api}},
		nested:   {GoFiles: []string{"domain.go"}, Imports: []string{api}},
		safe:     {GoFiles: []string{"safe.go"}},
		testOnly: {TestGoFiles: []string{"only_test.go"}, Imports: []string{api}},
	}

	roots := inboundDependencyRulePackages(pkgs)
	if want := []string{nested, safe}; !reflect.DeepEqual(roots, want) {
		t.Fatalf("rule packages = %v, want %v", roots, want)
	}
	if got, want := packagesReaching(pkgs, roots, api), []string{nested}; !reflect.DeepEqual(got, want) {
		t.Fatalf("packages reaching api = %v, want %v", got, want)
	}
}

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

// The dependency direction is one-way: every production package enters this rule automatically
// except the named inbound adapter and composition root. A package that needs an API type is
// telling us the type belongs in its owning module.
func TestProductionPackagesDoNotImportAPI(t *testing.T) {
	pkgs := loomarrPackages(t)
	const api = modulePath + "/internal/api"

	for _, root := range packagesReaching(pkgs, inboundDependencyRulePackages(pkgs), api) {
		t.Errorf("%s transitively imports internal/api — the dependency direction is one-way "+
			"(§14.1). A domain type needed outside the inbound adapter belongs in its owning "+
			"module, not internal/api.", root)
	}
}

// Production packages must not import the HTTP FRAMEWORK either, which is the same rule one level
// down. The same exhaustive source-derived package set keeps the two gates aligned.
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
func TestProductionPackagesDoNotImportTheHTTPFramework(t *testing.T) {
	pkgs := loomarrPackages(t)

	// Transport-shaped dependencies a pure domain package has no business holding. `net/http` is
	// deliberately NOT here: several domain packages are legitimate HTTP CLIENTS (library, tmdb,
	// programmer talk to real services). The rule is about SERVING, not about the protocol.
	frameworks := []string{
		"github.com/danielgtaylor/huma/v2",
		"github.com/danielgtaylor/huma/v2/adapters/humago",
	}

	for _, root := range inboundDependencyRulePackages(pkgs) {
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

func TestInboundDependencyExceptionsAreCurrent(t *testing.T) {
	pkgs := loomarrPackages(t)
	for path, rationale := range inboundDependencyExceptions {
		if strings.TrimSpace(rationale) == "" {
			t.Errorf("%s has no exception rationale", path)
		}
		pkg, ok := pkgs[path]
		if !ok || len(pkg.GoFiles)+len(pkg.CgoFiles) == 0 {
			t.Errorf("%s is an obsolete inbound-dependency exception — remove or replace it", path)
		}
	}
}

// Test doubles must never reach a binary. testkit exists so unit tests never touch the
// network; compiling it into any command is a seam that only ever gets wider.
func TestProductionCommandsDoNotLinkTestkit(t *testing.T) {
	pkgs := loomarrPackages(t)
	for packagePath := range pkgs {
		if !strings.HasPrefix(packagePath, modulePath+"/cmd/") {
			continue
		}
		linked := reachableFrom(pkgs, packagePath)
		if importers := importersOf(pkgs, linked, modulePath+"/internal/testkit"); len(importers) > 0 {
			t.Errorf("%s links internal/testkit through %v — test doubles must not ship (§14.1)",
				packagePath, importers)
		}
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

func inboundDependencyRulePackages(pkgs map[string]*build.Package) []string {
	var roots []string
	for path, pkg := range pkgs {
		if !strings.HasPrefix(path, modulePath+"/internal/") || len(pkg.GoFiles)+len(pkg.CgoFiles) == 0 {
			continue
		}
		if _, excepted := inboundDependencyExceptions[path]; excepted {
			continue
		}
		roots = append(roots, path)
	}
	sort.Strings(roots)
	return roots
}

func packagesReaching(pkgs map[string]*build.Package, roots []string, target string) []string {
	var found []string
	for _, root := range roots {
		if reachableFrom(pkgs, root)[target] {
			found = append(found, root)
		}
	}
	return found
}
