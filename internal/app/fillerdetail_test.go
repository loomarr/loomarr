package app

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/capability"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// #1412 end to end through the scheduler driver: production has no text model configured and a
// preparation backlog that does not drain. The deterministic pass must still be recorded for an
// existing clip that has never had one.
func TestFillerPipelineDriverRecordsPassForExistingClipUnderProductionShape(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := clipstore.Clip{Clip: clips.Clip{Hash: "existing", Path: "existing.mp4", Name: "Brambly Sauce Advert",
		Role: clips.Commercial}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	signals := capability.NewEnrichmentSignals(st, fstest.MapFS{}, nil)
	runner := enrichment.NewRunner(storeadapter.NewEnrichment(st), signals.Load,
		func() int { return 25 }, func() time.Time { return at.Add(time.Minute) })
	coordinator := enrichment.NewCoordinator(runner, storeadapter.NewEnrichment(st),
		func() enrichment.TextSelection { return enrichment.TextSelection{} },
		signals.Load, func() int { return 25 }, func() time.Time { return at.Add(time.Minute) })
	driver := newFillerPipelineDriver(nil, coordinator, nil)
	driver.prepare = func(context.Context) (filler.PipelineResult, error) {
		return filler.PipelineResult{Overview: pipelinestate.PipelineOverview{Runnable: 168, InProgress: 2}}, nil
	}
	if err := driver.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	candidates, err := st.ListFillerEnrichmentCandidates(t.Context(), enrichment.DeterministicProducer,
		enrichment.DeterministicProducerVersion, enrichment.ControlledTaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("clip still lacks a deterministic pass: %+v, err %v", candidates, err)
	}
}
