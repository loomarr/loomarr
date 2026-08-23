package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

const (
	backupDrillUserID      = "backup-drill-admin"
	backupDrillSessionHash = "backup-drill-session-hash"
	backupDrillSecret      = "backup-drill-provider-secret"
	backupDrillTitleID     = 987654
	backupDrillChannelID   = "backup-drill-channel"
	backupDrillJobID       = "backup-drill-job"
	backupDrillProposalID  = "backup-drill-proposal"
)

func backupDrillTimestamp() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

// TestSQLiteBackupRestoreDrill proves the documented database-only recovery path
// against an isolated data directory. The backup lives beside that directory so
// destroying the simulated volume cannot accidentally destroy its restore point.
func TestSQLiteBackupRestoreDrill(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	liveDir := filepath.Join(root, "live-data")
	backupDir := filepath.Join(root, "off-volume-backups")
	if err := os.MkdirAll(liveDir, 0o750); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(liveDir, "loomarr.db")
	dsn := "sqlite://" + databasePath

	live, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("start source store: %v", err)
	}
	seedBackupRestoreDrill(t, live)
	writer := BackupWriter(live)
	if writer == nil {
		t.Fatal("SQLite store did not expose its backup writer")
	}
	backup, err := writer.WriteBackup(ctx, backupDir)
	if err != nil {
		t.Fatalf("write consistent snapshot: %v", err)
	}
	assertFileMode(t, backup.Path, 0o600)
	if err := live.Close(); err != nil {
		t.Fatalf("stop source store: %v", err)
	}

	// This is the drill's destructive boundary: a directory created beneath this
	// test's TempDir. The off-volume snapshot is intentionally outside it.
	if err := os.RemoveAll(liveDir); err != nil {
		t.Fatalf("destroy isolated live data: %v", err)
	}
	if err := os.MkdirAll(liveDir, 0o750); err != nil {
		t.Fatalf("recreate isolated live data: %v", err)
	}
	snapshot, err := os.ReadFile(backup.Path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if err := os.WriteFile(databasePath, snapshot, 0o600); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		t.Fatalf("restore database permissions: %v", err)
	}
	assertFileMode(t, databasePath, 0o600)

	restored, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatalf("start restored store: %v", err)
	}
	defer func() { _ = restored.Close() }()
	assertBackupRestoreDrill(t, restored)
}

func seedBackupRestoreDrill(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	now := backupDrillTimestamp()
	user := User{
		ID: backupDrillUserID, Name: "Backup Drill Admin", Role: RoleAdmin,
		Quota: 4, AutoApprove: true, PasswordHash: "bcrypt-backup-drill-hash",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if err := s.CreateSession(ctx, Session{
		TokenHash: backupDrillSessionHash, UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := s.SetSetting(ctx, "llm.api_key.openai", backupDrillSecret); err != nil {
		t.Fatalf("seed secret setting: %v", err)
	}

	title := provision.Title{MediaType: provision.Movie, TMDBID: backupDrillTitleID, Name: "Backup Drill Movie", Year: 1999}
	key, err := title.Key()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertTitle(ctx, provision.Record{
		Key: key, Title: title, State: provision.Wanted,
		Deadline: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed title: %v", err)
	}

	if _, err := s.SaveChannel(ctx, sampleChannel(backupDrillChannelID, 77, now)); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := s.CreateJob(ctx, Job{
		ID: backupDrillJobID, Kind: "suggest", Status: "done", IntentJSON: `{}`, IntentHash: "backup-drill-intent",
		CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := s.CreateProposal(ctx, Proposal{
		ID: backupDrillProposalID, JobID: backupDrillJobID, Status: "approved",
		CreatedBy: user.ID, ApprovedBy: user.ID, ProposalJSON: `{}`,
		ApprovedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed proposal audit: %v", err)
	}
}

func assertBackupRestoreDrill(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	now := backupDrillTimestamp()
	user, err := s.GetUser(ctx, backupDrillUserID)
	if err != nil {
		t.Fatalf("restored account: %v", err)
	}
	if user.Role != RoleAdmin || user.PasswordHash != "bcrypt-backup-drill-hash" || !user.AutoApprove || user.Quota != 4 {
		t.Fatalf("restored account lost authorization/credential state: %+v", user)
	}
	if _, err := s.GetSession(ctx, backupDrillSessionHash, now); err != nil {
		t.Fatalf("restored live session: %v", err)
	}
	if got, err := s.GetSetting(ctx, "llm.api_key.openai"); err != nil || got != backupDrillSecret {
		t.Fatalf("restored secret setting = %q, %v", got, err)
	}
	key, err := (provision.Title{MediaType: provision.Movie, TMDBID: backupDrillTitleID}).Key()
	if err != nil {
		t.Fatal(err)
	}
	if title, err := s.GetTitle(ctx, key); err != nil || title.State != provision.Wanted || title.Title.Name != "Backup Drill Movie" {
		t.Fatalf("restored title = %+v, %v", title, err)
	}
	if channel, err := s.GetChannel(ctx, backupDrillChannelID); err != nil || channel.Number != 77 || channel.Policy.Audience.Ceiling != "TV-Y7" {
		t.Fatalf("restored channel = %+v, %v", channel, err)
	}
	if proposal, err := s.GetProposal(ctx, backupDrillProposalID); err != nil || proposal.Status != "approved" || proposal.ApprovedBy != backupDrillUserID {
		t.Fatalf("restored proposal audit = %+v, %v", proposal, err)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}
