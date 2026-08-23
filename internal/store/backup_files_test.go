package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

// write creates a file in dir and returns its path. Content length is the only thing
// that varies, so a size assertion can tell two backups apart.
func write(t *testing.T, dir, name string, size int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(files []store.BackupFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.Base(f.Path))
	}
	return out
}

// ListBackups returns our files newest-first and ignores everything else in the
// directory. `backup.dir` is operator-set and may hold other things.
func TestListBackups_NewestFirstAndOursOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "loomarr-2026-07-27-033000.db", 10)
	write(t, dir, "loomarr-2026-07-29-033000.db", 30)
	write(t, dir, "loomarr-2026-07-28-033000.db", 20)
	// None of these are ours, and none may appear.
	write(t, dir, "notes.txt", 1)
	write(t, dir, "loomarr.db", 1)            // the live database, if dir==data dir
	write(t, dir, "loomarr-2026-07-28.db", 1) // right prefix, wrong shape
	write(t, dir, "loomarr-2026-07-28-033000.db.gz", 1)
	if err := os.Mkdir(filepath.Join(dir, "loomarr-2026-07-26-033000.db"), 0o750); err != nil {
		t.Fatal(err) // a DIRECTORY named like a backup must not be listed
	}

	got, err := store.ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"loomarr-2026-07-29-033000.db",
		"loomarr-2026-07-28-033000.db",
		"loomarr-2026-07-27-033000.db",
	}
	if diff := names(got); !equal(diff, want) {
		t.Errorf("ListBackups = %v, want %v", diff, want)
	}
	if got[0].Bytes != 30 {
		t.Errorf("newest Bytes = %d, want 30", got[0].Bytes)
	}
}

// A directory that does not exist yet is the normal state of a fresh install, and must
// read as "no backups", never as an error the Backup page renders as breakage.
func TestListBackups_MissingDirIsEmptyNotError(t *testing.T) {
	got, err := store.ListBackups(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("missing dir → error %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("missing dir → %d backups, want 0", len(got))
	}
	// An unset backup.dir is the same story.
	if got, err := store.ListBackups(""); err != nil || len(got) != 0 {
		t.Errorf(`ListBackups("") = %v, %v; want empty, nil`, got, err)
	}
}

// ⚠ THE RETENTION GATE. Pruning keeps the newest `retain` and deletes the rest.
func TestPruneBackups_KeepsNewestRetain(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{
		"loomarr-2026-07-25-033000.db",
		"loomarr-2026-07-26-033000.db",
		"loomarr-2026-07-27-033000.db",
		"loomarr-2026-07-28-033000.db",
		"loomarr-2026-07-29-033000.db",
	} {
		write(t, dir, n, 1)
	}

	removed, err := store.PruneBackups(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want 3", removed)
	}
	got, err := store.ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"loomarr-2026-07-29-033000.db", "loomarr-2026-07-28-033000.db"}
	if diff := names(got); !equal(diff, want) {
		t.Errorf("after prune = %v, want %v", diff, want)
	}
}

// ⚠ Retention must never delete a file it did not write. `backup.dir` can legitimately
// point at a directory holding an operator's other files; deleting those is data loss,
// not a retention policy. Verified with retain=1 against a directory that is mostly
// foreign files — a prune that ignored the pattern would take them all.
func TestPruneBackups_NeverTouchesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "loomarr-2026-07-28-033000.db", 1)
	write(t, dir, "loomarr-2026-07-29-033000.db", 1)
	foreign := []string{"family-photos.tar", "loomarr.db", "loomarr.db-wal", "README.md"}
	for _, n := range foreign {
		write(t, dir, n, 1)
	}

	if _, err := store.PruneBackups(dir, 1); err != nil {
		t.Fatal(err)
	}
	for _, n := range foreign {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("prune deleted a file it did not write: %s (%v)", n, err)
		}
	}
	// And it did do its actual job.
	got, _ := store.ListBackups(dir)
	if len(got) != 1 || filepath.Base(got[0].Path) != "loomarr-2026-07-29-033000.db" {
		t.Errorf("after prune = %v, want just the newest backup", names(got))
	}
}

// retain <= 0 keeps everything. An operator who wants unbounded history needs a way to
// say so, and "delete every backup" is never what a retention setting should mean.
func TestPruneBackups_ZeroRetainKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "loomarr-2026-07-28-033000.db", 1)
	write(t, dir, "loomarr-2026-07-29-033000.db", 1)

	for _, retain := range []int{0, -1} {
		removed, err := store.PruneBackups(dir, retain)
		if err != nil {
			t.Fatal(err)
		}
		if removed != 0 {
			t.Errorf("retain=%d removed %d, want 0", retain, removed)
		}
		if got, _ := store.ListBackups(dir); len(got) != 2 {
			t.Errorf("retain=%d left %d backups, want 2", retain, len(got))
		}
	}
}

// Fewer backups than the policy keeps is a no-op, not an error.
func TestPruneBackups_UnderRetainIsNoOp(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "loomarr-2026-07-29-033000.db", 1)
	removed, err := store.PruneBackups(dir, 7)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if got, _ := store.ListBackups(dir); len(got) != 1 {
		t.Errorf("left %d backups, want 1", len(got))
	}
}

// IsBackupName is the boundary the download handler validates a CLIENT-SUPPLIED path
// segment against, so traversal attempts belong in its table explicitly.
func TestIsBackupName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"loomarr-2026-07-29-033000.db", true},
		{"loomarr-1999-01-01-000000.db", true},
		{"", false},
		{"loomarr.db", false},
		{"loomarr-2026-07-29-033000.db.gz", false},
		{"loomarr-2026-07-29-0330.db", false}, // short time field
		{"loomarr-2026-7-29-033000.db", false},
		{"../loomarr-2026-07-29-033000.db", false},
		{"../../etc/passwd", false},
		{"/etc/passwd", false},
		{"subdir/loomarr-2026-07-29-033000.db", false},
		{"loomarr-2026-07-29-033000.db/../../etc/passwd", false},
	} {
		if got := store.IsBackupName(tc.name); got != tc.want {
			t.Errorf("IsBackupName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
