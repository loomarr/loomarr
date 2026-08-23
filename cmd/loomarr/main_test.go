package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/config"
)

func TestDatabaseMigrationRequesterIsNonBlocking(t *testing.T) {
	lifecycle := newLifecycleRequester()

	if err := lifecycle.RequestMigration("postgres://first"); err != nil {
		t.Fatalf("first request: %v", err)
	}
	if got := (<-lifecycle.requests).databaseMigrationDSN; got != "postgres://first" {
		t.Fatalf("queued DSN = %q, want first request", got)
	}
	// Receiving the request does not reopen admission during generation teardown.
	if err := lifecycle.RequestMigration("postgres://duplicate"); !errors.Is(err, errLifecycleTransitionAlreadyRequested) {
		t.Fatalf("duplicate request error = %v, want %v", err, errLifecycleTransitionAlreadyRequested)
	}
}

func TestDatabaseMigrationDoesNotQueueBehindRestart(t *testing.T) {
	lifecycle := newLifecycleRequester()
	lifecycle.RequestRestart()

	err := lifecycle.RequestMigration("postgres://target")
	if !errors.Is(err, errLifecycleTransitionAlreadyRequested) {
		t.Fatalf("migration behind restart = %v, want already requested", err)
	}
	if got := (<-lifecycle.requests).databaseMigrationDSN; got != "" {
		t.Fatalf("queued lifecycle request changed to migration %q", got)
	}
}

func TestDatabaseMigrationTimeoutCancelsOfflineCopy(t *testing.T) {
	err := runDatabaseMigrationWithTimeout(
		context.Background(),
		10*time.Millisecond,
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("migration error = %v, want context deadline exceeded", err)
	}
}

func TestDatabaseCutoverClosesLiveStoreBeforeCopy(t *testing.T) {
	var events []string
	err := performDatabaseCutover(
		context.Background(),
		func() error {
			events = append(events, "close")
			return nil
		},
		func(context.Context) error {
			events = append(events, "copy")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(events); got != 2 || events[0] != "close" || events[1] != "copy" {
		t.Fatalf("cutover events = %v, want [close copy]", events)
	}
}

func TestDatabaseCutoverDoesNotCopyWhenLiveStoreCannotClose(t *testing.T) {
	copied := false
	err := performDatabaseCutover(
		context.Background(),
		func() error { return errors.New("worker still owns store") },
		func(context.Context) error { copied = true; return nil },
	)
	if err == nil {
		t.Fatal("cutover succeeded after close failure")
	}
	if copied {
		t.Fatal("database copy started before the live store closed")
	}
}

func TestDatabaseMigrationRechecksEnvironmentPin(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///pinned.db")
	err := migrateDatabase(context.Background(), "invalid-source", "postgres://target")
	if !errors.Is(err, errDatabaseURLPinned) {
		t.Fatalf("migration error = %v, want environment pin refusal", err)
	}
}

func TestDatabaseURLIsPersistedOnlyAfterVerifiedMigration(t *testing.T) {
	t.Run("migration failure leaves bootstrap unchanged", func(t *testing.T) {
		persisted := false
		err := migrateThenPersistDatabaseURL(
			func() error { return errors.New("parity mismatch") },
			func() error {
				persisted = true
				return nil
			},
		)
		if err == nil {
			t.Fatal("expected migration failure")
		}
		if persisted {
			t.Fatal("DATABASE_URL was persisted after failed verification")
		}
	})

	t.Run("verified migration commits bootstrap", func(t *testing.T) {
		persisted := false
		err := migrateThenPersistDatabaseURL(
			func() error { return nil },
			func() error {
				persisted = true
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !persisted {
			t.Fatal("verified migration did not persist DATABASE_URL")
		}
	})
}

func TestFailedFirstPostgresGenerationRestoresSQLite(t *testing.T) {
	dir := t.TempDir()
	if err := config.WriteBootstrapFile(dir, map[string]string{
		"DATABASE_URL": "postgres://target",
		"LOG_LEVEL":    "debug",
	}); err != nil {
		t.Fatal(err)
	}
	state := &databaseMigrationState{
		fallbackSQLiteURL: "sqlite://" + filepath.Join(dir, "loomarr.db"),
		bootstrapDir:      dir,
	}
	if err := state.restoreSQLite(errors.New("target unavailable")); err != nil {
		t.Fatal(err)
	}
	values, err := config.LoadBootstrapFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if values["DATABASE_URL"] != "sqlite://"+filepath.Join(dir, "loomarr.db") {
		t.Fatalf("restored DATABASE_URL = %q", values["DATABASE_URL"])
	}
	if values["LOG_LEVEL"] != "debug" {
		t.Fatalf("restore discarded LOG_LEVEL: %#v", values)
	}
	if state.fallbackSQLiteURL != "" || state.lastError == "" {
		t.Fatalf("restored state = %+v", state)
	}
}
