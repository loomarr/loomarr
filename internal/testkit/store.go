package testkit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

// InvalidationListenStep scripts one subscription attempt for InvalidationListener.
// Started is closed when subscribed; BeforeReady and AfterReady gate the corresponding
// phases. WaitForCancellation keeps a successful subscription alive until its context ends.
type InvalidationListenStep struct {
	Started             chan struct{}
	BeforeReady         <-chan struct{}
	AfterReady          <-chan struct{}
	Err                 error
	WaitForCancellation bool
}

// InvalidationListener is the shared scripted double for store invalidation subscriptions.
// It records calls by consuming one step per subscription attempt.
type InvalidationListener struct {
	mu    sync.Mutex
	steps []InvalidationListenStep
	calls int
}

// NewInvalidationListener constructs a listener with the supplied subscription attempts.
func NewInvalidationListener(steps ...InvalidationListenStep) *InvalidationListener {
	return &InvalidationListener{steps: append([]InvalidationListenStep(nil), steps...)}
}

// Calls reports the number of subscription attempts received.
func (l *InvalidationListener) Calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// Listen matches store.ListenInvalidations and executes the next scripted attempt.
func (l *InvalidationListener) Listen(
	ctx context.Context,
	_ store.Store,
	ready func(context.Context) error,
	_ func(context.Context, store.Invalidation) error,
) error {
	l.mu.Lock()
	if l.calls >= len(l.steps) {
		l.mu.Unlock()
		return fmt.Errorf("test invalidation listener: unexpected subscription attempt")
	}
	step := l.steps[l.calls]
	l.calls++
	l.mu.Unlock()

	if step.Started != nil {
		close(step.Started)
	}
	if err := waitInvalidationStep(ctx, step.BeforeReady); err != nil {
		return err
	}
	if ready != nil {
		if err := ready(ctx); err != nil {
			return err
		}
	}
	if err := waitInvalidationStep(ctx, step.AfterReady); err != nil {
		return err
	}
	if step.WaitForCancellation {
		<-ctx.Done()
		return ctx.Err()
	}
	return step.Err
}

func waitInvalidationStep(ctx context.Context, gate <-chan struct{}) error {
	if gate == nil {
		return nil
	}
	select {
	case <-gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FaultSettingStore wraps a real Store with deterministic SetSetting failures.
// Inspect runs before fault injection so package-specific tests can decode and
// record values without teaching testkit the owning package's wire format.
type FaultSettingStore struct {
	store.Store

	mu         sync.Mutex
	writes     int
	FailWrites map[int]error
	Inspect    func(key, value string) error
	AfterSave  func()
}

func (s *FaultSettingStore) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	s.writes++
	write := s.writes
	fail := s.FailWrites[write]
	s.mu.Unlock()

	if s.Inspect != nil {
		if err := s.Inspect(key, value); err != nil {
			return err
		}
	}
	if fail != nil {
		return fail
	}
	if err := s.Store.SetSetting(ctx, key, value); err != nil {
		return err
	}
	if s.AfterSave != nil {
		s.AfterSave()
	}
	return nil
}

// SQLiteStore opens a fresh migrated production store for a test. Domain tests use
// this shared adapter when behavior depends on real settings/channel persistence.
//
// Prefer MigratedSQLiteStore below: it gives the same fully-migrated schema without
// paying migration's parse cost on every call. This form still runs the real
// migrate path and stays for callers that specifically want that (e.g. asserting
// on migration behavior itself), or as a fallback if the template ever needs
// bypassing for a specific test.
func SQLiteStore(t testing.TB) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatalf("open SQLite test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// templateOnce and its outputs build the migrated-schema template exactly once per
// `go test` process (§ this package's job: shared, safe test doubles).
//
// Why this exists: a CPU profile of `go test -race ./internal/app/` showed
// store.Open at 43% of process CPU, with store.migrate itself at 40% — almost
// entirely modernc's pure-Go SQL parser (_sqlite3RunParser) re-parsing all of
// this repo's SQLite migration DDL on every single store.Open(..., true) call.
// internal/app has hundreds of tests that each open their own store, so the same
// ~59 migration files get parsed and executed hundreds of times over, and that
// re-parse — not I/O, not the schema work itself — is the dominant cost.
//
// The fix is to migrate ONE on-disk database to HEAD once, then hand every test
// its own byte-for-byte copy of that already-migrated file opened with
// migrate=false (store.Open's third argument), which skips store.migrate (and
// therefore the parser) entirely. Each test still gets a fully independent
// database — copies are never shared or reused — so nothing here weakens test
// isolation; it only removes redundant migration work that produced an identical
// schema every time anyway.
var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// buildTemplate creates and fully migrates a SQLite database in a directory that
// outlives any single test's t.TempDir() (a process-lifetime directory under
// os.MkdirTemp, cleaned up via TestMain-independent os semantics — it simply
// lives for the process and the OS reclaims /tmp on its own schedule, same as
// any other test temp file), then closes it.
//
// Closing matters: SQLite (including modernc's driver) checkpoints WAL back into
// the main file and removes the -wal/-shm sidecars on a clean close, leaving one
// quiescent file. store/backup.go's StreamBackup comment is the same warning in
// the other direction ("never cp a live SQLite file") — copying while WAL is
// active can hand a reader a torn, incomplete view. Building the template with
// a normal store.Open(..., true) and then Close()-ing it before any copy is
// taken is what makes copying safe here.
func buildTemplate() (string, error) {
	dir, err := os.MkdirTemp("", "loomarr-testkit-template-*")
	if err != nil {
		return "", fmt.Errorf("create template dir: %w", err)
	}
	path := filepath.Join(dir, "template.db")

	st, err := store.Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		return "", fmt.Errorf("build migrated template store: %w", err)
	}
	if err := st.Close(); err != nil {
		return "", fmt.Errorf("close migrated template store: %w", err)
	}
	return path, nil
}

// MigratedSQLiteStore opens a fully-migrated SQLite store for a test by copying the
// process-shared migrated template (see buildTemplate) into the test's own
// t.TempDir() and opening that copy with migrate=false, so the schema is already
// at HEAD and store.Open does zero migration work. This is the fast-path
// equivalent of SQLiteStore above and should be preferred by any test that just
// wants an empty, fully-migrated store — see this package's comment above
// templateOnce for why it exists.
//
// Every call still gets an independent on-disk database: the template is only
// ever read from, and each test copies it into a fresh file it alone owns.
func MigratedSQLiteStore(t testing.TB) store.Store {
	t.Helper()

	templateOnce.Do(func() {
		templatePath, templateErr = buildTemplate()
	})
	if templateErr != nil {
		t.Fatalf("build migrated SQLite template: %v", templateErr)
	}

	dest := filepath.Join(t.TempDir(), "loomarr.db")
	if err := copyFile(templatePath, dest); err != nil {
		t.Fatalf("copy migrated SQLite template: %v", err)
	}

	st, err := store.Open(context.Background(), "sqlite://"+dest, false)
	if err != nil {
		t.Fatalf("open copied SQLite test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// copyFile duplicates src's bytes into dst. The template file is closed and
// quiescent (see buildTemplate) by the time this ever runs, so a plain byte copy
// is safe — no WAL sidecars exist to miss.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is our own template path, not user input
	if err != nil {
		return fmt.Errorf("open template: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst) //nolint:gosec // dst is caller's own t.TempDir() path
	if err != nil {
		return fmt.Errorf("create test db: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy template bytes: %w", err)
	}
	return out.Close()
}
