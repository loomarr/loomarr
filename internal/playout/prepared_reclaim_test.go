package playout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A beta.7 prepared library: <root>/<sha256 hex>/ publications, each with .publication.json, and
// .staging-<hex>-* workspaces an interrupted build left behind.
func writePublication(t *testing.T, root, name string, segmentBytes int) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{
		".publication.json": `{"key":"x"}`,
		"seg-00001.m4s":     strings.Repeat("v", segmentBytes),
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReclaimRetiredPrepared_RemovesPublicationsAndStagingThenTheEmptyDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "prepared")
	writePublication(t, root, strings.Repeat("a", 64), 1000)
	writePublication(t, root, strings.Repeat("b", 64), 500)
	staging := filepath.Join(root, ".staging-"+strings.Repeat("c", 64)+"-123")
	if err := os.MkdirAll(staging, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "seg-00001.m4s"), make([]byte, 250), 0o640); err != nil {
		t.Fatal(err)
	}

	got, err := ReclaimRetiredPrepared(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries != 3 || got.Bytes != int64(1000+500+250+2*len(`{"key":"x"}`)) || !got.DirRemoved {
		t.Fatalf("reclaim = %+v; want 3 entries, every byte counted, the empty dir removed", got)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("prepared dir still present: %v", err)
	}
	// A second boot finds nothing to do.
	if again, err := ReclaimRetiredPrepared(root); err != nil || again != (PreparedReclaim{}) {
		t.Fatalf("second reclaim = %+v, %v; want a no-op", again, err)
	}
}

func TestReclaimRetiredPrepared_LeavesWhatItDoesNotRecognise(t *testing.T) {
	root := filepath.Join(t.TempDir(), "prepared")
	writePublication(t, root, strings.Repeat("d", 64), 100)
	// Not prepared media: a hex-named dir with no publication record, an operator's own dir and
	// file, and evidence the move to the state directory could not take.
	keep := []string{
		filepath.Join(root, strings.Repeat("e", 64), "notes.txt"),
		filepath.Join(root, "my-recordings", "show.mkv"),
		filepath.Join(root, "README"),
		filepath.Join(root, capabilityEvidenceName),
	}
	for _, p := range keep {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("keep"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	// A hex-named symlink to a real publication elsewhere: never followed, never removed.
	elsewhere := t.TempDir()
	writePublication(t, elsewhere, strings.Repeat("f", 64), 100)
	link := filepath.Join(root, strings.Repeat("f", 64))
	if err := os.Symlink(filepath.Join(elsewhere, strings.Repeat("f", 64)), link); err != nil {
		t.Fatal(err)
	}

	got, err := ReclaimRetiredPrepared(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries != 1 || got.DirRemoved {
		t.Fatalf("reclaim = %+v; want only the one publication, and the dir kept", got)
	}
	for _, p := range append(keep, filepath.Join(elsewhere, strings.Repeat("f", 64), ".publication.json")) {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was removed: %v", p, err)
		}
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("symlink removed: %v", err)
	}
}

func TestReclaimRetiredPrepared_NoDirIsANoOp(t *testing.T) {
	got, err := ReclaimRetiredPrepared(filepath.Join(t.TempDir(), "prepared"))
	if err != nil || got != (PreparedReclaim{}) {
		t.Fatalf("reclaim = %+v, %v; want a no-op", got, err)
	}
	if got, err := ReclaimRetiredPrepared(""); err != nil || got != (PreparedReclaim{}) {
		t.Fatalf("empty path: reclaim = %+v, %v; want a no-op", got, err)
	}
}
