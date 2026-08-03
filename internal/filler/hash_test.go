package filler_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// writeBytes writes a file with specific CONTENT — distinct from scan_test.go's `writeFile`,
// which only needs a file to exist. Identity here is the bytes, so the bytes are the point.
func writeBytes(t *testing.T, dir, name string, body []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// filler body: big enough that head and tail windows do not overlap.
func body(fill byte, n int) []byte { return bytes.Repeat([]byte{fill}, n) }

// ⚠ THE property the whole identity change exists for: the same advert in two folders is ONE
// clip. Under path identity these were two clips with the same relative path, one silently
// overwriting the other; the hash is what lets the scan recognise and skip the duplicate.
func TestClipID_SameContentInTwoFoldersIsOneIdentity(t *testing.T) {
	dir := t.TempDir()
	content := body('a', 200_000)
	a := writeBytes(t, dir, "folder-one/ads/coke.mp4", content)
	b := writeBytes(t, dir, "folder-two/ads/coke.mp4", content)

	idA, err := filler.ClipID(a)
	if err != nil {
		t.Fatal(err)
	}
	idB, err := filler.ClipID(b)
	if err != nil {
		t.Fatal(err)
	}
	if idA != idB {
		t.Errorf("same bytes in two folders produced different ids:\n  %s\n  %s", idA, idB)
	}
}

// Different adverts must not collide, including when they share a head — which real video files
// do, since containers put similar codec metadata up front.
func TestClipID_DistinguishesFilesSharingAHead(t *testing.T) {
	dir := t.TempDir()
	head := body('h', 100_000)

	one := append(append([]byte{}, head...), body('1', 100_000)...)
	two := append(append([]byte{}, head...), body('2', 100_000)...)

	idOne, _ := filler.ClipID(writeBytes(t, dir, "one.mp4", one))
	idTwo, _ := filler.ClipID(writeBytes(t, dir, "two.mp4", two))
	if idOne == idTwo {
		t.Error("two files sharing a head collided — the TAIL window is not contributing")
	}
}

// ⚠ The size is mixed in precisely for this case: a truncated copy shares its original's head,
// and (for a long enough file) nothing else is read from the middle. Size is what separates them,
// and a truncated duplicate is the single most likely real-world near-collision.
func TestClipID_DistinguishesATruncatedCopy(t *testing.T) {
	dir := t.TempDir()
	full := body('x', 300_000)

	idFull, _ := filler.ClipID(writeBytes(t, dir, "full.mp4", full))
	idCut, _ := filler.ClipID(writeBytes(t, dir, "cut.mp4", full[:250_000]))
	if idFull == idCut {
		t.Error("a truncated copy collided with its original — the SIZE is not contributing")
	}
}

// A file smaller than one window is hashed once, not twice. ⚠ Without the guard the head and tail
// windows overlap, so a small file's identity would depend on how much of it happened to be read
// twice — stable, but arbitrary, and it would differ from the same bytes stored in a larger file.
func TestClipID_HandlesFilesSmallerThanTheWindow(t *testing.T) {
	dir := t.TempDir()
	small := body('s', 512)

	id, err := filler.ClipID(writeBytes(t, dir, "small.mp4", small))
	if err != nil {
		t.Fatalf("a small file failed to hash: %v", err)
	}
	if id == "" {
		t.Error("empty id for a small but valid file")
	}
	// Same content, different name → same id. Identity is the bytes, not the filename.
	again, _ := filler.ClipID(writeBytes(t, dir, "renamed.mp4", small))
	if id != again {
		t.Error("renaming a file changed its identity — identity must be the CONTENT")
	}
}

// An empty file has no identity worth computing: every empty file would share one, so a whole
// folder of them would collapse into a single clip.
func TestClipID_RefusesAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := filler.ClipID(writeBytes(t, dir, "empty.mp4", nil)); err == nil {
		t.Error("an empty file was given an identity — every empty file would share it")
	}
}
