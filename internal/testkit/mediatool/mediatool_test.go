package mediatool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocatePrecedence(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "internal", "pkg")
	local := filepath.Join(root, ".tools", "ffmpeg", "bin", "ffmpeg")
	for _, dir := range []string{pkg, filepath.Dir(local)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	onPath := func(string) (string, error) { return "/path/ffmpeg", nil }
	check := func(name, override, want string, wantPinned bool) {
		t.Helper()
		got, pinned, err := locate(pkg, override, onPath)
		if err != nil || got != want || pinned != wantPinned {
			t.Errorf("%s: got %q pinned=%v err=%v, want %q pinned=%v", name, got, pinned, err, want, wantPinned)
		}
	}

	check("no repo-local build", "", "/path/ffmpeg", false)
	if err := os.WriteFile(local, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	check("repo-local beats PATH", "", local, true)
	check("FFMPEG_PATH beats repo-local", "/chosen/ffmpeg", "/chosen/ffmpeg", true)
}
