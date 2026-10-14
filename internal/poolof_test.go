package internal_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `store.PoolOf` hands out the raw *sql.DB, and only the composition root may ask for it.
//
// `store.Store` is the port every consumer depends on. Handing the pool to any of them would let
// them write SQL straight at the catalog and bypass the store's own methods — the boundary §2
// exists to keep. One caller genuinely needs it: `internal/app` wires River, which takes a
// database/sql pool directly, and shares this pool rather than opening a second one (on SQLite
// that is load-bearing — the store sets MaxOpenConns(1) because modernc serializes writes, and a
// second pool would reintroduce the WAL contention that setting avoids).
//
// ⚠ **All of that was already written down on PoolOf, and prose is not a gate.** The function was
// flagged in an architecture review as "six downcast escape hatches, 15 external call sites — the
// one place the Store abstraction is genuinely pierced". Measured, it has TWO callers: the
// composition root and one test. The doc comment was accurate and the concern was not — but
// nothing was stopping the third caller, and a rule that only exists in a comment is one PR away
// from being false. This is that rule, executable.
//
// The `*sqlStore` type assertions inside package store are NOT what this guards. A package
// asserting on its own concrete type is ordinary implementation, not a pierced boundary; those
// are how backup, migrate and migrate_data reach the pool they own.
func TestOnlyTheCompositionRootTakesTheRawPool(t *testing.T) {
	// `internal/store` defines it; `internal/app` is the sanctioned caller.
	allowed := map[string]bool{"store": true, "app": true}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}

	var offenders []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			// ⚠ Production files only. A test reaching for the pool to set up a fixture is a
			// different act from a subsystem depending on it, and gating tests here would be
			// the kind of over-broad rule people delete rather than obey.
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			src, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if !strings.Contains(string(src), "PoolOf(") {
				return nil
			}
			pkg := filepath.Base(filepath.Dir(path))
			if !allowed[pkg] {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	for _, f := range offenders {
		t.Errorf("%s calls store.PoolOf — only the composition root (internal/app) may take the raw "+
			"*sql.DB (§2, §14.1). A subsystem that needs the database should get a store method, or "+
			"an interface it declares; reaching past Store lets it write SQL at the catalog and the "+
			"store stops being the one writer.", f)
	}

	// ⚠ Non-vacuity. If PoolOf is renamed or deleted, the substring above stops matching and this
	// test passes over a tree it is no longer checking — silently, forever. Assert the sanctioned
	// caller is still there, so the gate fails loudly when its own premise moves.
	appDir := filepath.Join(root, "internal", "app")
	if _, err := build.ImportDir(appDir, 0); err != nil {
		t.Fatalf("internal/app is not a package: %v", err)
	}
	found := false
	entries, err := os.ReadDir(appDir)
	if err != nil {
		t.Fatalf("read internal/app: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_test.go") || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, rerr := os.ReadFile(filepath.Join(appDir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		if strings.Contains(string(src), "store.PoolOf(") {
			found = true
			break
		}
	}
	if !found {
		t.Error("internal/app no longer calls store.PoolOf — either the River wiring moved (update " +
			"`allowed` here) or PoolOf was renamed, in which case this gate is now matching nothing " +
			"and would never fail again")
	}
}
