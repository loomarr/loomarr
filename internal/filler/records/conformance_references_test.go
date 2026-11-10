package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// saveFillerSources registers an archive source for each id not yet registered, so a download run
// that names it satisfies its foreign key to filler_sources (#2793). The package's twin of
// testkit.SaveFillerSources, which it can't import.
func saveFillerSources(t *testing.T, s Store, ids ...string) {
	t.Helper()
	ctx := context.Background()
	existing, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatalf("list filler sources: %v", err)
	}
	have := map[string]bool{}
	for _, src := range existing {
		have[src.ID] = true
	}
	for _, id := range ids {
		if id == "" || have[id] {
			continue
		}
		have[id] = true
		if err := s.UpsertFillerSource(ctx, FillerSource{
			ID: id, Kind: "archive", URI: "fixture-" + id, CreatedAt: time.Unix(1, 0), Enabled: true,
		}); err != nil {
			t.Fatalf("save filler source %s: %v", id, err)
		}
	}
}

// saveAcquisitionRun stores a finished download run with no source, so a clip pipeline row or
// ready event that names it satisfies its foreign key to filler_acquisition_runs.
func saveAcquisitionRun(t *testing.T, s Store, id string) {
	t.Helper()
	at := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertAcquisitionRun(context.Background(), acquisition.AcquisitionRun{
		ID: id, Trigger: acquisition.TriggerSource, Status: acquisition.RunSuccess,
		StartedAt: at, CompletedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatalf("save acquisition run %s: %v", id, err)
	}
}
