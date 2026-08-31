package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The generated config docs must match the committed docs/configuration.md
// (config-design §2, the OpenAPI-style drift gate). A registry change without a
// `make config-docs` regen fails here — the same contract discipline as
// openapi-verify, enforced under comprehensive verification too (not only in the Makefile).
func TestConfigDocs_NoDrift(t *testing.T) {
	// Walk up to the repo root (where docs/ lives) from the package dir.
	root := repoRoot(t)
	committed, err := os.ReadFile(filepath.Join(root, "docs", "configuration.md"))
	if err != nil {
		t.Fatalf("read committed config docs (run `make config-docs`?): %v", err)
	}
	generated := NewRegistry().Markdown()
	if string(committed) != string(generated) {
		t.Errorf("docs/configuration.md is stale — run `make config-docs` and commit.\n"+
			"committed %d bytes, generated %d bytes", len(committed), len(generated))
	}
}

// The generated doc must mention every registry key (no key silently omitted).
func TestConfigDocs_CoversEveryKey(t *testing.T) {
	md := string(NewRegistry().Markdown())
	for _, key := range keysForDocs(NewRegistry()) {
		if !strings.Contains(md, "`"+key+"`") {
			t.Errorf("config docs omit key %q", key)
		}
	}
}

// repoRoot finds the module root by walking up until go.mod is found.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from " + dir)
		}
		dir = parent
	}
}
