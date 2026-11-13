package quarantine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestPutMovesFilesAsideAndEmptyDeletesThemADayLater(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_900_000_000, 0).UTC()
	bin := New(root, func() time.Time { return now })
	media := filepath.Join(root, "ab", "cd", "clip.mp4")
	sidecar := filepath.Join(root, "ab", "cd", "clip.info.json")
	write(t, media)
	write(t, sidecar)

	if failed := bin.Put([]string{media, sidecar, filepath.Join(root, "gone.mp4")}); len(failed) != 0 {
		t.Fatalf("Put failed: %v", failed)
	}
	held := filepath.Join(root, DirName, "1900000000", "ab", "cd", "clip.mp4")
	if exists(media) || !exists(held) {
		t.Fatalf("after Put: original present=%v, held present=%v; want moved aside", exists(media), exists(held))
	}

	now = now.Add(Hold - time.Second)
	if n, err := bin.Empty(); err != nil || n != 0 || !exists(held) {
		t.Fatalf("Empty before a day = %d, %v, held=%v; want nothing removed", n, err, exists(held))
	}
	now = now.Add(time.Second)
	if n, err := bin.Empty(); err != nil || n != 1 || exists(held) {
		t.Fatalf("Empty after a day = %d, %v, held=%v; want the batch removed", n, err, exists(held))
	}
}

func TestPutRefusesAFileOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.mp4")
	write(t, outside)
	failed := New(root, time.Now).Put([]string{outside, filepath.Join(root, DirName, "1", "x.mp4")})
	if len(failed) != 2 || !exists(outside) {
		t.Fatalf("Put = %v, outside present=%v; want both refused and nothing moved", failed, exists(outside))
	}
	for _, err := range failed {
		if !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("Put error = %v, want ErrOutsideRoot", err)
		}
	}
}

func TestEmptyLeavesWhatItDidNotPutThere(t *testing.T) {
	root := t.TempDir()
	stranger := filepath.Join(root, DirName, "not-a-batch", "file")
	write(t, stranger)
	now := time.Unix(1_900_000_000, 0).UTC()
	if n, err := New(root, func() time.Time { return now }).Empty(); err != nil || n != 0 || !exists(stranger) {
		t.Fatalf("Empty = %d, %v, stranger=%v; want an unknown entry kept", n, err, exists(stranger))
	}
}
