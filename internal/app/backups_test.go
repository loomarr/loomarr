package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

// stubWriter writes a file named like a real backup so the service under test does real
// filesystem work without needing a live SQLite store.
//
// The name is FIXED by the test rather than derived from time.Now(): the test decides
// which backup is "newest", and a same-second collision in CI would otherwise be flaky.
type stubWriter struct {
	name string
	err  error
}

func (s *stubWriter) WriteBackup(_ context.Context, dir string) (store.BackupFile, error) {
	if s.err != nil {
		return store.BackupFile{}, s.err
	}
	path := filepath.Join(dir, s.name)
	if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
		return store.BackupFile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return store.BackupFile{}, err
	}
	return store.BackupFile{Path: path, Bytes: info.Size(), WrittenAt: info.ModTime().Unix()}, nil
}

func newBackupsService(dir string, retain int, w backupWriter) *backupsService {
	return &backupsService{
		w:      w,
		dir:    func() string { return dir },
		retain: func() int { return retain },
		sched:  func() string { return "0 30 3 * * *" },
	}
}

func seed(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// ⚠ THE TRAVERSAL GATE, against the real service. `name` reaches Open straight from a
// URL path segment, so the difference between validating it and joining it is the
// difference between an admin-only backup download and an admin-only arbitrary file read.
func TestBackupsService_OpenRejectsTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, "loomarr-2026-07-29-033000.db")
	// The files an attacker would be reaching for: one beside the backups, one above.
	seed(t, dir, "secrets.env")
	parent := filepath.Dir(dir)
	if err := os.WriteFile(filepath.Join(parent, "loomarr.db"), []byte("live db"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := newBackupsService(dir, 7, &stubWriter{})

	// The happy path works, so the refusals below are about the NAME, not a broken service.
	got, err := svc.Open(context.Background(), "loomarr-2026-07-29-033000.db")
	if err != nil {
		t.Fatalf("Open of a real backup → %v", err)
	}
	if got.Path != filepath.Join(dir, "loomarr-2026-07-29-033000.db") {
		t.Errorf("Path = %q, want it inside the backup dir", got.Path)
	}

	for _, name := range []string{
		"../loomarr.db",
		"../../etc/passwd",
		"/etc/passwd",
		"secrets.env",
		"loomarr.db",
		"loomarr-2026-07-29-033000.db/../../loomarr.db",
		"./loomarr-2026-07-29-033000.db",
		"",
	} {
		got, err := svc.Open(context.Background(), name)
		if err == nil {
			t.Errorf("Open(%q) resolved to %q, want a refusal", name, got.Path)
		}
	}
}

// Open resolves only files the listing knows about, so "exists on disk" and "is a backup
// this instance will serve" stay one question with one answer.
func TestBackupsService_OpenUnknownName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := newBackupsService(dir, 7, &stubWriter{})
	if _, err := svc.Open(context.Background(), "loomarr-2026-07-29-033000.db"); err == nil {
		t.Error("Open of a name that is not on disk succeeded, want an error")
	}
}

// List reports the policy alongside the files — the page renders "nightly at 03:30 ·
// keeps 7", and neither number is derivable from the file list.
func TestBackupsService_ListReportsPolicy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, "loomarr-2026-07-28-033000.db", "loomarr-2026-07-29-033000.db", "unrelated.txt")

	list, err := newBackupsService(dir, 7, &stubWriter{}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !list.Supported {
		t.Error("Supported = false on a SQLite install, want true")
	}
	if list.Retain != 7 || list.Schedule != "0 30 3 * * *" || list.Dir != dir {
		t.Errorf("policy = retain %d, schedule %q, dir %q", list.Retain, list.Schedule, list.Dir)
	}
	if len(list.Backups) != 2 {
		t.Fatalf("Backups = %+v, want the 2 real backups only", list.Backups)
	}
	if list.Backups[0].Name != "loomarr-2026-07-29-033000.db" {
		t.Errorf("first entry = %q, want the newest", list.Backups[0].Name)
	}
	// Names, not paths: the UI shows a filename and the download endpoint takes one.
	if filepath.IsAbs(list.Backups[0].Name) {
		t.Errorf("Name = %q, want a bare filename", list.Backups[0].Name)
	}
}

// ⚠ Run prunes AFTER writing. Asserted by the OUTCOME with retain=1 against a full
// directory: the new backup survives and the old ones are gone. A prune-first
// implementation would delete an old backup, then add the new one — reaching the same
// count from a state where a failed write leaves the operator worse off.
func TestBackupsService_RunWritesThenPrunes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, "loomarr-2026-07-26-033000.db", "loomarr-2026-07-27-033000.db")

	svc := newBackupsService(dir, 1, &stubWriter{name: "loomarr-2026-07-29-033000.db"})
	entry, err := svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if entry.Name != "loomarr-2026-07-29-033000.db" {
		t.Errorf("entry.Name = %q", entry.Name)
	}

	left, err := store.ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 {
		t.Fatalf("after Run with retain=1: %d backups, want 1", len(left))
	}
	if filepath.Base(left[0].Path) != "loomarr-2026-07-29-033000.db" {
		t.Errorf("survivor = %q, want the backup just written", filepath.Base(left[0].Path))
	}
}

// ⚠ A FAILED write must not cost the operator a backup they already had. This is the
// whole reason prune runs second: an implementation that pruned first would satisfy
// retention by deleting the oldest and then fail, leaving fewer backups than it started
// with — at the exact moment the database is unhealthy.
func TestBackupsService_FailedWriteDeletesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, "loomarr-2026-07-26-033000.db", "loomarr-2026-07-27-033000.db")

	svc := newBackupsService(dir, 1, &stubWriter{err: os.ErrPermission})
	if _, err := svc.Run(context.Background()); err == nil {
		t.Fatal("Run with a failing writer returned nil, want the error")
	}

	left, err := store.ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Errorf("a failed backup left %d of 2 backups — retention ran before the write", len(left))
	}
}
