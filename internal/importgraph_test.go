package internal_test

import (
	"go/build"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The Loomarr package graph, read from SOURCE, for the architecture gates (§14.1).
//
// ⚠ **This exists because the gates were passing over trees they never examined** (GH #282).
// They learned about the tree by shelling out to `go list`, and subprocess output is NOT an
// input Go's test cache tracks — so moving a file in `internal/store` did not change package
// `internal`'s test binary, nothing invalidated the cached result, and the gate reported
// `ok (cached)` with the violation present. Only `-count=1` showed the red. CI restores a warm
// cache, so a violating PR could go green on it: a real hole in a real guard.
//
// Reading the tree with `filepath.WalkDir` + `go/build` (which opens and reads the files) makes
// those reads part of what the cache tracks, so a moved, added or edited file invalidates the
// result on its own.
//
// ⚠ `go/build` rather than a hand-rolled `go/parser` walk, deliberately: it applies build
// constraints and excludes `_test.go` files exactly as the compiler does. A parser walk would
// see `//go:build integration` files that are NOT in the shipped binary and report imports that
// do not exist there — inventing violations, which is the failure mode that discredits a gate
// fastest.

const modulePath = "github.com/mantonx/loomarr"

// loomarrPackages maps each Loomarr package import path to its parsed *build.Package, for a
// DEFAULT build (no build tags), which is what `cmd/loomarr` compiles as. `.Imports` drives the
// dependency gates; `.GoFiles` and `.Doc` drive the package-doc gate.
func loomarrPackages(t *testing.T) map[string]*build.Package {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}

	pkgs := map[string]*build.Package{}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// Skip what `go list ./...` skips, plus the frontend tree and VCS metadata. A
		// `testdata` directory is not a package even when it holds .go files.
		switch d.Name() {
		case ".git", "node_modules", "testdata", "web", ".agents", ".claude":
			return filepath.SkipDir
		}

		// A directory with no buildable Go files (a fixtures dir, a parent of packages, or one
		// whose files are all behind build tags) yields an error rather than a package. That is
		// ordinary here: this walk maps what EXISTS in a default build, and a directory that
		// does not participate has nothing to contribute. Genuine build breakage is `make
		// check`'s job, not a gate's.
		pkg, ierr := build.ImportDir(path, 0)
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		importPath := modulePath
		if rel != "." {
			importPath += "/" + filepath.ToSlash(rel)
		}
		pkgs[importPath] = pkg
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(pkgs) == 0 {
		t.Fatal("package graph is empty — the walk found no packages, so every gate built on it " +
			"would vacuously pass (the exact shape #282 was about)")
	}
	return pkgs
}

// reachableFrom returns every Loomarr package transitively imported by root, INCLUDING root —
// i.e. exactly the Loomarr packages linked into that binary.
func reachableFrom(pkgs map[string]*build.Package, root string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(p string) {
		if seen[p] {
			return
		}
		seen[p] = true
		pkg, ok := pkgs[p]
		if !ok {
			return
		}
		for _, imp := range pkg.Imports {
			// Only Loomarr packages are traversed; third-party imports are recorded on the
			// importer but never followed, which is what scopes these gates to code we own.
			if strings.HasPrefix(imp, modulePath+"/") {
				walk(imp)
			}
		}
	}
	walk(root)
	return seen
}

// importersOf names every package in `set` that imports `target`. It returns the importers
// rather than a bool so a failure says WHERE the edge is, not merely that one exists.
func importersOf(pkgs map[string]*build.Package, set map[string]bool, target string) []string {
	var found []string
	for p := range set {
		pkg, ok := pkgs[p]
		if !ok {
			continue
		}
		for _, imp := range pkg.Imports {
			if imp == target {
				found = append(found, p)
			}
		}
	}
	return found
}

// TestPackageGraph_IsReadFromSource pins the property this file exists for.
//
// ⚠ It cannot assert on the cache directly — a test cannot observe whether it was itself served
// from cache. What it CAN do is fail loudly if the walk stops seeing the tree, which is the
// change that would silently reintroduce #282: swap this back to `exec.Command("go", "list")`
// and the file reads disappear, taking cache invalidation with them and leaving nothing to
// notice it.
//
// The direct verification is manual, and recorded here so it can be repeated:
//
//	git mv internal/store/conformance_titles_test.go internal/store/conformance_titles.go
//	go test ./internal/ -run TestNoLoomarrPackageLinksTestingIntoTheBinary   # NO -count=1
//
// Before #282's fix that returned `ok (cached)`. After it, it correctly FAILS on a warm cache.
func TestPackageGraph_IsReadFromSource(t *testing.T) {
	pkgs := loomarrPackages(t)

	// The composition root must import the store. If this fails, the walk is reading the wrong
	// thing and every gate on top of it is passing vacuously.
	app, ok := pkgs[modulePath+"/internal/app"]
	if !ok {
		t.Fatal("internal/app is not in the graph — the source walk is broken")
	}
	var importsStore bool
	for _, imp := range app.Imports {
		if imp == modulePath+"/internal/store" {
			importsStore = true
		}
	}
	if !importsStore {
		t.Error("internal/app does not import internal/store — the composition root wires the " +
			"store into every subsystem, so this means the walk is reading the wrong thing")
	}

	// And the reachable set from the binary must be substantial. A one-element result would mean
	// the transitive walk silently stopped.
	if linked := reachableFrom(pkgs, modulePath+"/cmd/loomarr"); len(linked) < 10 {
		t.Errorf("only %d Loomarr packages reachable from cmd/loomarr — the transitive walk is "+
			"not walking; gates built on it would pass vacuously", len(linked))
	}
}
