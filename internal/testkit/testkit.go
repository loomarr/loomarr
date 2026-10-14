// Package testkit holds the shared test doubles and pinned fixtures every test
// uses (AGENTS.md testing rules: unit tests never touch the network; phases
// extend the testkit rather than inventing private mocks). The Phase-0 captures
// live under fixtures/ with source-version comments and are the pinned truth
// parsers are written against.
//
// Phase 1 provides the fixtures loader. Service mocks (media server ×2 flavors,
// Tunarr, Seerr, TMDB, LLM, arr webhook sender) are added by the phase that
// first needs each, always here — never as a private mock in a _test.go file.
package testkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fixturesDir resolves to internal/testkit/fixtures regardless of the caller's
// working directory, by anchoring on this source file's location.
func fixturesDir() string {
	_, self, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(self), "fixtures")
}

// Fixture reads a pinned fixture file by its path relative to fixtures/, e.g.
// Fixture(t, "radarr/import_webhook.json"). It fails the test if the file is
// missing — a renamed or deleted fixture must surface loudly, not silently.
func Fixture(t testing.TB, rel string) []byte {
	t.Helper()
	p := filepath.Join(fixturesDir(), rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %q: %v", rel, err)
	}
	return b
}
