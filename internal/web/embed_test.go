package web

import (
	"io/fs"
	"os/exec"
	"strings"
	"testing"
)

// `//go:embed all:dist` does not compile unless the directory exists, so .gitkeep is the
// one committed file in there (.gitignore un-ignores exactly that path). Vite's
// `emptyOutDir` deletes it on every build, and a routine `git add -A` then stages the
// deletion — which has broken the clean-clone build TWICE.
//
// The cause is fixed in vite.config.ts (a plugin rewrites the file after each bundle);
// this is the guard that catches it if that plugin is ever removed. It asserts the file
// is TRACKED BY GIT, not merely present on disk: present-but-untracked is exactly the
// state that compiles here and fails on a fresh checkout.
func TestGitkeepIsTracked(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "dist/.gitkeep").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err) // a source tarball has no git; not a failure
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("internal/web/dist/.gitkeep is not tracked — `go:embed all:dist` will not " +
			"compile on a clean clone. Vite's emptyOutDir removed it; `git add -f " +
			"internal/web/dist/.gitkeep` and check vite.config.ts's keepEmbedDir plugin.")
	}
}

// The embedded FS must always carry at least one entry, which is what makes the embed
// directive legal.
func TestEmbedHasContent(t *testing.T) {
	entries, err := fs.ReadDir(distFS, "dist")
	if err != nil {
		t.Fatalf("dist is not embedded: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("dist embedded empty")
	}
}
