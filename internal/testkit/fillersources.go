package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
)

// FillerSourceSaver is the store a fixture registers filler sources through.
type FillerSourceSaver interface {
	ListFillerSources(ctx context.Context) ([]records.FillerSource, error)
	UpsertFillerSource(ctx context.Context, src records.FillerSource) error
}

// SaveFillerSources registers an archive source for each id not yet registered, so a download
// run that names it satisfies its foreign key to filler_sources (#2793). A source the migrations
// seed (youtube, folder) is left as it is; "" is skipped, since a run with no source names none.
func SaveFillerSources(t testing.TB, s FillerSourceSaver, ids ...string) {
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
		src := records.FillerSource{ID: id, Kind: "archive", URI: "fixture-" + id, CreatedAt: time.Unix(1, 0), Enabled: true}
		if err := s.UpsertFillerSource(ctx, src); err != nil {
			t.Fatalf("save filler source %s: %v", id, err)
		}
	}
}
