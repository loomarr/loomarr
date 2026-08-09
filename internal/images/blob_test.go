package images

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	hashA = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	hashB = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestHashBytesIsStableAndFullLength(t *testing.T) {
	h := HashBytes([]byte("loomarr"))
	if len(h) != 64 {
		t.Errorf("hash is %d chars; truncating buys a collision domain that silently serves one operator's icon as another's", len(h))
	}
	if h != HashBytes([]byte("loomarr")) {
		t.Error("hash is not stable")
	}
	if h == HashBytes([]byte("loomarr ")) {
		t.Error("distinct inputs hashed the same")
	}
}

// ⚠ The containment assertion. The serve route takes a hash from the URL, so anything that is not
// exactly 64 lowercase hex characters must be refused before it is ever joined to a path — a shape
// in which no separator, dot, or NUL can occur cannot escape the directory at all.
func TestPathsRefuseAnythingThatIsNotAHash(t *testing.T) {
	b := newBlobStore(t.TempDir())

	for _, bad := range []string{
		"../../../../etc/passwd",
		"abcdef/../../../etc/passwd",
		strings.Repeat("a", 63), // too short
		strings.Repeat("a", 65), // too long
		strings.Repeat("A", 64), // uppercase — not our encoding
		strings.Repeat("g", 64), // not hex
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef012345678\x00",
		"",
	} {
		if _, err := b.OriginalPath(bad, ".png"); !errors.Is(err, ErrBadHash) {
			t.Errorf("OriginalPath(%q) did not reject it", bad)
		}
		if _, err := b.DerivativePath(bad, 320, FormatWebP); !errors.Is(err, ErrBadHash) {
			t.Errorf("DerivativePath(%q) did not reject it", bad)
		}
	}
}

func TestPathsAreShardedTwoLevels(t *testing.T) {
	dir := t.TempDir()
	b := newBlobStore(dir)

	orig, err := b.OriginalPath(hashA, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "orig", "ab", "cd", hashA+".jpg")
	if orig != want {
		t.Errorf("OriginalPath = %q, want %q", orig, want)
	}

	drv, err := b.DerivativePath(hashA, 320, FormatWebP)
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Join(dir, "drv", "ab", "cd", hashA+"_w320.webp")
	if drv != want {
		t.Errorf("DerivativePath = %q, want %q", drv, want)
	}

	// Originals and derivatives must not share a subtree: "clear the cache" has to be able to
	// delete drv/ wholesale without a filter that could be got wrong.
	if strings.HasPrefix(drv, filepath.Join(dir, "orig")) {
		t.Error("derivative path sits under orig/")
	}
}

// JPEG's format name and its extension differ, and a derivative written to one path but looked up
// at another is a rendition that regenerates forever.
func TestDerivativeExtensionMatchesTheFormat(t *testing.T) {
	b := newBlobStore(t.TempDir())
	p, err := b.DerivativePath(hashA, 500, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, "_w500.jpg") {
		t.Errorf("JPEG derivative = %q, want a .jpg suffix", p)
	}
}

func TestWriteIsAtomicAndLeavesNoScratchFiles(t *testing.T) {
	dir := t.TempDir()
	b := newBlobStore(dir)
	path, _ := b.OriginalPath(hashA, ".png")

	if err := b.Write(path, []byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read back = %q, %v", got, err)
	}

	// A leftover ".tmp-*" would eventually be adopted by a directory scan or fill the disk.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("scratch file left behind: %s", e.Name())
		}
	}
}

// ⚠ "Exists" is the wrong question: a zero-length file is what a killed encoder leaves, and
// recording it as a present derivative means serving a BROKEN image — worse than an absent one,
// because absence has a designed fallback and corruption does not.
func TestStatTreatsAnEmptyFileAsAbsent(t *testing.T) {
	dir := t.TempDir()
	b := newBlobStore(dir)
	path, _ := b.DerivativePath(hashA, 320, FormatWebP)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, ok := b.Stat(path); ok {
		t.Fatal("Stat reported an empty file as present")
	}
}

func TestWriteFromCapsOnTheReadNotTheClaim(t *testing.T) {
	dir := t.TempDir()
	b := newBlobStore(dir)
	path, _ := b.OriginalPath(hashA, ".png")

	// A reader that would happily supply far more than the cap — standing in for a remote server
	// that lies in Content-Length, or a multipart part that under-declares its Size.
	big := strings.NewReader(strings.Repeat("x", 5000))
	if _, err := b.WriteFrom(path, big, 1000); err == nil {
		t.Fatal("WriteFrom accepted a body past its cap")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("an over-cap write left a partial file behind")
	}

	ok := strings.NewReader("small")
	if _, err := b.WriteFrom(path, ok, 1000); err != nil {
		t.Fatalf("WriteFrom rejected a body under its cap: %v", err)
	}
}

// The shard directory is shared between images, so clearing one image's renditions must not take
// its neighbours' with it.
func TestRemoveAllForLeavesOtherImagesAlone(t *testing.T) {
	dir := t.TempDir()
	b := newBlobStore(dir)

	mine, _ := b.DerivativePath(hashA, 320, FormatWebP)
	mineToo, _ := b.DerivativePath(hashA, 500, FormatAVIF)
	orig, _ := b.OriginalPath(hashA, ".png")
	for _, p := range []string{mine, mineToo, orig} {
		if err := b.Write(p, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	// A neighbour that lands in the same shard would be a coincidence; force one by writing
	// directly into the same directory under a different hash prefix-sharing name.
	neighbour := filepath.Join(filepath.Dir(mine), hashB+"_w320.webp")
	if err := b.Write(neighbour, []byte("keep me")); err != nil {
		t.Fatal(err)
	}

	if err := b.RemoveAllFor(hashA); err != nil {
		t.Fatalf("RemoveAllFor: %v", err)
	}

	if _, ok := b.Stat(mine); ok {
		t.Error("own derivative survived")
	}
	if _, ok := b.Stat(mineToo); ok {
		t.Error("own second derivative survived")
	}
	if _, ok := b.Stat(neighbour); !ok {
		t.Error("a DIFFERENT image's derivative was deleted from the shared shard")
	}
	// Derivative eviction must never touch the original — for an upload that is the only copy.
	if _, ok := b.Stat(orig); !ok {
		t.Error("RemoveAllFor deleted the original; for an upload that is unrecoverable data loss")
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	b := newBlobStore(t.TempDir())
	path, _ := b.OriginalPath(hashA, ".png")
	if err := b.Remove(path); err != nil {
		t.Errorf("removing an absent file errored: %v", err)
	}
}
