package mediatool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocatePrecedence(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "internal", "pkg")
	bin := filepath.Join(root, ".tools", "ffmpeg", "bin")
	local := filepath.Join(bin, "ffmpeg")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(root, "go.mod"), "")
	write(filepath.Join(root, "scripts", "macos-ffmpeg.pins"), "# comment\nbase new\n")
	onPath := func(string) (string, error) { return "/path/ffmpeg", nil }
	check := func(name, override, want string, wantPinned bool) {
		t.Helper()
		got, pinned, err := locate(pkg, override, onPath)
		if err != nil || got != want || pinned != wantPinned {
			t.Errorf("%s: got %q pinned=%v err=%v, want %q pinned=%v", name, got, pinned, err, want, wantPinned)
		}
	}
	unusable := func(name string) {
		t.Helper()
		if got, _, err := locate(pkg, "", onPath); !errors.Is(err, errUnusable) {
			t.Errorf("%s: got %q err=%v, want errUnusable", name, got, err)
		}
	}

	check("no repo-local build", "", "/path/ffmpeg", false)
	write(local, "")
	unusable("build with no record")
	write(filepath.Join(bin, ".pins"), "base old\n")
	unusable("build from other pins")
	check("FFMPEG_PATH beats a stale build", "/chosen/ffmpeg", "/chosen/ffmpeg", true)
	write(filepath.Join(bin, ".pins"), "base new\n")
	check("repo-local beats PATH", "", local, true)
	check("FFMPEG_PATH beats repo-local", "/chosen/ffmpeg", "/chosen/ffmpeg", true)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	unusable("record with no build")
}
