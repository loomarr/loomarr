package filler_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestNewLayout_NormalizesAndDerivesWatchFolder(t *testing.T) {
	base := t.TempDir()
	layout, err := filler.NewLayout("  "+filepath.Join(base, "nested", "..", "clips")+"  ", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(resolvedTestDir(t, base), "clips")
	if err := os.MkdirAll(wantRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if layout.ClipDir() != wantRoot {
		t.Errorf("clip dir = %q, want %q", layout.ClipDir(), wantRoot)
	}
	if want := filepath.Join(wantRoot, filler.WatchDirName); layout.WatchDir() != want {
		t.Errorf("watch dir = %q, want %q", layout.WatchDir(), want)
	}
	if _, err := fs.Stat(layout.FS(), "."); err != nil {
		t.Errorf("layout FS does not expose clip root: %v", err)
	}
}

func TestNewLayout_NormalizesExplicitWatchFolder(t *testing.T) {
	base := t.TempDir()
	layout, err := filler.NewLayout(filepath.Join(base, "clips"), filepath.Join(base, "in", "..", "watch"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolvedTestDir(t, base), "watch"); layout.WatchDir() != want {
		t.Errorf("watch dir = %q, want %q", layout.WatchDir(), want)
	}
}

func TestNewLayout_EmptyPairIsTheOnlyZeroLayout(t *testing.T) {
	layout, err := filler.NewLayout("", "")
	if err != nil {
		t.Fatal(err)
	}
	if layout.ClipDir() != "" || layout.WatchDir() != "" || layout.FS() != nil {
		t.Fatalf("empty pair = (%q, %q, %v), want zero layout", layout.ClipDir(), layout.WatchDir(), layout.FS())
	}
	if _, err := filler.NewLayout("", filepath.Join(t.TempDir(), "watch")); err == nil {
		t.Fatal("watch folder without a clip folder was accepted")
	}
}

func TestNewLayout_RejectsFilesystemRoots(t *testing.T) {
	root := string(filepath.Separator)
	if _, err := filler.NewLayout(root, ""); err == nil {
		t.Fatal("filesystem root was accepted as clip folder")
	}
	if _, err := filler.NewLayout(t.TempDir(), root); err == nil {
		t.Fatal("filesystem root was accepted as watch folder")
	}
}

func TestNewLayout_RejectsRelativePaths(t *testing.T) {
	if _, err := filler.NewLayout("relative/clips", ""); err == nil {
		t.Fatal("relative clip folder was accepted")
	}
	if _, err := filler.NewLayout(t.TempDir(), "relative/watch"); err == nil {
		t.Fatal("relative watch folder was accepted")
	}
}

func TestNewLayout_RejectsWatchFolderContainingClipLibrary(t *testing.T) {
	base := t.TempDir()
	clipDir := filepath.Join(base, "clips")
	for name, watchDir := range map[string]string{
		"same folder": clipDir,
		"parent":      base,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := filler.NewLayout(clipDir, watchDir); err == nil {
				t.Fatalf("NewLayout(%q, %q) accepted a watch folder containing the clip library", clipDir, watchDir)
			}
		})
	}
}

func TestNewLayout_RejectsExistingSymlinkAliasOfClipLibrary(t *testing.T) {
	base := t.TempDir()
	clipDir := filepath.Join(base, "clips")
	if err := os.MkdirAll(clipDir, 0o750); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "watch-alias")
	if err := os.Symlink(clipDir, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := filler.NewLayout(clipDir, alias); err == nil {
		t.Fatal("NewLayout accepted a watch symlink resolving to the clip library")
	}
}

func TestNewLayout_RejectsSymlinkedParentBeforeClipFolderExists(t *testing.T) {
	realRoot := t.TempDir()
	aliasRoot := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	clipDir := filepath.Join(aliasRoot, "clips") // deliberately absent
	if _, err := filler.NewLayout(clipDir, realRoot); err == nil {
		t.Fatalf("NewLayout(%q, %q) accepted a symlinked parent containing the future clip library", clipDir, realRoot)
	}
}

func TestNewLayout_RejectsAliasedPrefixesWhenBothFinalPathsAreMissing(t *testing.T) {
	realRoot := t.TempDir()
	aliases := t.TempDir()
	aliasA := filepath.Join(aliases, "mount-a")
	aliasB := filepath.Join(aliases, "mount-b")
	if err := os.Symlink(realRoot, aliasA); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(realRoot, aliasB); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	watchDir := filepath.Join(aliasA, "inbox")
	clipDir := filepath.Join(aliasB, "inbox", "clips")
	if _, err := filler.NewLayout(clipDir, watchDir); err == nil {
		t.Fatalf("NewLayout(%q, %q) accepted missing paths beneath aliased prefixes", clipDir, watchDir)
	}
}

func TestNewLayout_RejectsCaseFoldedMissingAncestor(t *testing.T) {
	base := t.TempDir()
	watchDir := filepath.Join(base, "Inbox")
	clipDir := filepath.Join(base, "inbox", "clips")
	if _, err := filler.NewLayout(clipDir, watchDir); err == nil {
		t.Fatalf("NewLayout(%q, %q) accepted a layout unsafe on a case-insensitive filesystem", clipDir, watchDir)
	}
}

func TestNewLayout_RejectsExistingFilesAsFolders(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := filler.NewLayout(file, filepath.Join(base, "watch")); err == nil {
		t.Fatal("NewLayout accepted a regular file as the clip folder")
	}
	if _, err := filler.NewLayout(filepath.Join(base, "clips"), file); err == nil {
		t.Fatal("NewLayout accepted a regular file as the watch folder")
	}
}

func TestNewLayout_NormalizesSafeSymlinkRootsForTraversal(t *testing.T) {
	realClip := t.TempDir()
	realWatch := t.TempDir()
	aliases := t.TempDir()
	clipAlias := filepath.Join(aliases, "clips")
	watchAlias := filepath.Join(aliases, "watch")
	if err := os.Symlink(realClip, clipAlias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(realWatch, watchAlias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	layout, err := filler.NewLayout(clipAlias, watchAlias)
	if err != nil {
		t.Fatal(err)
	}
	wantClip := resolvedTestDir(t, realClip)
	wantWatch := resolvedTestDir(t, realWatch)
	if layout.ClipDir() != wantClip || layout.WatchDir() != wantWatch {
		t.Fatalf("symlink traversal layout = (%q, %q), want (%q, %q)", layout.ClipDir(), layout.WatchDir(), wantClip, wantWatch)
	}
}

func TestNewLayout_NormalizesAliasedNestedWatchToClipTraversal(t *testing.T) {
	realRoot := t.TempDir()
	aliases := t.TempDir()
	aliasA := filepath.Join(aliases, "mount-a")
	aliasB := filepath.Join(aliases, "mount-b")
	if err := os.Symlink(realRoot, aliasA); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(realRoot, aliasB); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	layout, err := filler.NewLayout(filepath.Join(aliasA, "clips"), filepath.Join(aliasB, "clips", "inbox"))
	if err != nil {
		t.Fatal(err)
	}
	wantClip := filepath.Join(resolvedTestDir(t, realRoot), "clips")
	if layout.ClipDir() != wantClip || layout.WatchDir() != filepath.Join(wantClip, "inbox") {
		t.Fatalf("aliased nested layout = (%q, %q), want clip spelling %q/inbox", layout.ClipDir(), layout.WatchDir(), wantClip)
	}
}

func resolvedTestDir(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
