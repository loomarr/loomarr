package backup

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// lockingWriter writes the new backup, then makes the directory read-only so the prune that
// follows cannot delete anything.
type lockingWriter struct {
	stubWriter
	dir string
}

func (l *lockingWriter) WriteBackup(ctx context.Context, dir string) (store.BackupFile, error) {
	bf, err := l.stubWriter.WriteBackup(ctx, dir)
	if err != nil {
		return bf, err
	}
	return bf, os.Chmod(dir, 0o500)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A prune that cannot delete does not fail the backup, and it is logged so the operator can see the
// directory growing.
func TestRunLogsAFailedPrune(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so a prune cannot be made to fail")
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	seed(t, dir, "loomarr-2026-07-27-033000.db", "loomarr-2026-07-28-033000.db")

	var logs lockedBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	w := &lockingWriter{stubWriter: stubWriter{name: "loomarr-2026-07-29-033000.db"}, dir: dir}
	entry, err := newBackupsService(dir, 1, w).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed because the prune did: %v", err)
	}
	if entry.Name != filepath.Base(w.name) {
		t.Errorf("Entry.Name = %q, want the backup just written", entry.Name)
	}
	if !strings.Contains(logs.String(), "backup.prune_failed") {
		t.Errorf("a failed prune left no warning in the log:\n%s", logs.String())
	}
}
