package scan

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf16"
)

// notifyRecord is one FILE_NOTIFY_INFORMATION record as a server sends it.
func notifyRecord(action uint32, name string, last bool) []byte {
	units := utf16.Encode([]rune(name))
	rec := make([]byte, 12+2*len(units))
	binary.LittleEndian.PutUint32(rec[4:], action)
	binary.LittleEndian.PutUint32(rec[8:], uint32(2*len(units)))
	for i, u := range units {
		binary.LittleEndian.PutUint16(rec[12+2*i:], u)
	}
	for len(rec)%4 != 0 {
		rec = append(rec, 0)
	}
	if !last {
		binary.LittleEndian.PutUint32(rec[0:], uint32(len(rec)))
	}
	return rec
}

func TestParseNotifyReadsEveryRecord(t *testing.T) {
	t.Parallel()
	data := slices.Concat(
		notifyRecord(1, `Show (2001)\Season 1\Show - S01E01.mkv`, false),
		notifyRecord(5, `Café Film (1999)`, false),
		notifyRecord(2, `Show (2001)\Season 1\old.nfo`, true),
	)
	got := parseNotify(data)
	want := []string{"Show (2001)/Season 1/Show - S01E01.mkv", "Café Film (1999)", "Show (2001)/Season 1/old.nfo"}
	if !slices.Equal(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
	// A record cut short by the buffer is dropped, not misread.
	if got := parseNotify(data[:20]); len(got) != 0 {
		t.Fatalf("truncated names = %q", got)
	}
}

func TestChangedDirsPassesParentsOnce(t *testing.T) {
	t.Parallel()
	root := "/media/tv"
	got := changedDirs(root, []string{
		"Show (2001)/Season 1/a.mkv", "Show (2001)/Season 1/a.nfo", "New Show (2024)", "../escape.mkv",
	})
	want := []string{"/media/tv/Show (2001)/Season 1", "/media/tv"}
	if !slices.Equal(got, want) {
		t.Fatalf("dirs = %q, want %q", got, want)
	}
	if got := changedDirs(root, nil); !slices.Equal(got, []string{root}) {
		t.Fatalf("an unnamed change = %q, want the root", got)
	}
}

// Against a real SMB mount: LOOMARR_SMB_DIR is a folder on one. With LOOMARR_SMB_WRITABLE=1 the
// test writes and removes one file there and expects the server to report it.
func TestWatchShareOnARealMount(t *testing.T) {
	dir := os.Getenv("LOOMARR_SMB_DIR")
	if dir == "" {
		t.Skip("LOOMARR_SMB_DIR is not set")
	}
	if !cifsFilesystem(dir) {
		t.Fatalf("%s is not on an SMB mount", dir)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := make(chan shareChange, 8)
	failures := make(chan error, 1)
	go watchShare(ctx, dir, time.Hour, out, func(err error, _ bool) { failures <- err })

	if os.Getenv("LOOMARR_SMB_WRITABLE") != "1" {
		select {
		case err := <-failures:
			t.Fatalf("the share refused a watch: %v", err)
		case <-time.After(2 * time.Second):
		}
		return
	}
	// A folder two levels down, as an episode sits under a show and a season.
	nested := filepath.Join(dir, "Probe Show", "Season 1")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(filepath.Join(dir, "Probe Show")) }()
	// Let the folders' own changes arrive, then watch for the file alone.
	time.Sleep(time.Second)
	for drained := false; !drained; {
		select {
		case <-out:
		default:
			drained = true
		}
	}
	name := filepath.Join(nested, "loomarr-notify-probe.txt")
	if err := os.WriteFile(name, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case err := <-failures:
			t.Fatalf("the watch failed: %v", err)
		case change := <-out:
			if change.root != dir {
				t.Fatalf("change = %+v, want root %s", change, dir)
			}
			if slices.Contains(change.dirs, nested) {
				return
			}
		case <-deadline:
			t.Fatalf("the server did not report %s within 10 s", name)
		}
	}
}
