package app

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// River is the only job executor. When it cannot start, no job would ever run on its schedule,
// so the process must refuse to boot instead of serving a healthy-looking app with a dead
// background.
func TestBuild_RiverStartFailureFailsTheBuild(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)

	// A River bookkeeping table with the wrong shape makes River's own migrator fail, the same
	// way a corrupt or foreign schema would on a real install.
	if _, err := store.PoolOf(st).ExecContext(t.Context(),
		`CREATE TABLE river_migration (unrelated TEXT)`); err != nil {
		t.Fatalf("poison river schema: %v", err)
	}

	application, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{DataDir: t.TempDir()})
	if err == nil {
		_ = application.Shutdown(t.Context())
		t.Fatal("Build succeeded although River could not start")
	}
	if application != nil {
		t.Errorf("Build returned an application alongside error %v", err)
	}
	if !strings.Contains(err.Error(), "river") {
		t.Errorf("Build error %q does not name River as the cause", err)
	}
}
