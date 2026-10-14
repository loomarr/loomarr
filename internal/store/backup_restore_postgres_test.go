//go:build integration

package store

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

// TestPostgresBackupRestoreDrill exercises the documented pg_dump/pg_restore
// path in one dedicated ephemeral container. The only database it drops is the
// one that helper created for this test.
func TestPostgresBackupRestoreDrill(t *testing.T) {
	ctx := context.Background()
	dsn, ctr := startPostgresContainer(t)
	live, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("start source store: %v", err)
	}
	seedBackupRestoreDrill(t, live)

	const dumpPath = "/tmp/loomarr-backup-restore.dump"
	execBackupDrill(t, ctr, "sh", "-ceu", "umask 077; pg_dump --format=custom --file="+dumpPath+" --username=loomarr --dbname=loomarr")
	if got := strings.TrimSpace(execBackupDrill(t, ctr, "stat", "-c", "%a", dumpPath)); got != "600" {
		t.Fatalf("Postgres dump mode = %q, want 600", got)
	}
	if err := live.Close(); err != nil {
		t.Fatalf("stop source store: %v", err)
	}

	execBackupDrill(t, ctr, "dropdb", "--username=loomarr", "--force", "loomarr")
	execBackupDrill(t, ctr, "createdb", "--username=loomarr", "--owner=loomarr", "loomarr")
	execBackupDrill(t, ctr, "pg_restore", "--username=loomarr", "--dbname=loomarr", "--exit-on-error", dumpPath)
	owner := strings.TrimSpace(execBackupDrill(t, ctr,
		"psql", "--username=loomarr", "--dbname=postgres", "--tuples-only", "--no-align",
		"--command", "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'loomarr'"))
	if owner != "loomarr" {
		t.Fatalf("restored database owner = %q, want loomarr", owner)
	}

	restored, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatalf("start restored store: %v", err)
	}
	defer func() { _ = restored.Close() }()
	assertBackupRestoreDrill(t, restored)
}

func execBackupDrill(t *testing.T, ctr testcontainers.Container, command ...string) string {
	t.Helper()
	code, output, err := ctr.Exec(context.Background(), command, tcexec.Multiplexed())
	if err != nil {
		t.Fatalf("container exec %v: %v", command, err)
	}
	data, readErr := io.ReadAll(output)
	if readErr != nil {
		t.Fatalf("read container exec %v: %v", command, readErr)
	}
	if code != 0 {
		t.Fatalf("container exec %v exited %d: %s", command, code, strings.TrimSpace(string(data)))
	}
	return string(data)
}
