package app

import (
	"context"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/mediameasure"
	"github.com/loomarr/loomarr/internal/playout"
)

// sourceMeasurer is the part of mediameasure.Measurer the resolver uses.
type sourceMeasurer interface {
	MeasureFacts(context.Context, mediameasure.SourceRef) (inventory.SourceFacts, error)
	Submit(mediameasure.SourceRef)
}

// measureSink is where measurements land: stream facts through the inventory service, everything
// else through the analysis store.
type measureSink struct {
	inventory.Service
	inventory.AnalysisStore
}

// newMeasurer builds Loomarr's measurement job for this resolver: stream facts come from the same
// prober playout already uses, revisions from the same stat rule local sources are registered
// with, and the analysis tools from the given (background) runner.
func (r *playoutResolver) newMeasurer(tools mediameasure.Tools, analysis inventory.AnalysisStore) *mediameasure.Measurer {
	return mediameasure.New(mediameasure.Deps{
		Sink:  measureSink{Service: r.inventory, AnalysisStore: analysis},
		Tools: tools,
		Facts: func(ctx context.Context, path string) (inventory.SourceFacts, error) {
			observed, err := r.probeSource(ctx, path)
			if err != nil {
				return inventory.SourceFacts{}, err
			}
			return inventoryFactsOf(observed), nil
		},
		Revision: localRevisionOfPath,
		Now:      func() time.Time { return r.inventoryNow() },
		Log:      r.log,
	})
}

// measureFirstPlay is the synchronous fast path for a source Loomarr has never measured: probe
// stream facts once, record them against the source revision, and queue the slower analysis. A
// probe failure returns false and playout keeps its existing behaviour.
func (r *playoutResolver) measureFirstPlay(ctx context.Context, origin inventory.OriginKey, input string) (formatOut playout.MediaFormat, ok bool) {
	if r.measurer == nil || r.probeSource == nil {
		return playout.MediaFormat{}, false
	}
	source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
		Item: inventory.ItemRef{Origin: &origin}, Now: r.inventoryNow(),
		Kinds: []inventory.SourceKind{inventory.SourceLocalFile},
	})
	if err != nil || !found {
		return playout.MediaFormat{}, false
	}
	ref := mediameasure.SourceRef{ID: source.ID, Revision: source.Revision, Path: input}
	facts, err := r.measurer.MeasureFacts(ctx, ref)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: first-play measurement failed, falling back to probe", "err", err)
		}
		return playout.MediaFormat{}, false
	}
	ref.Facts = facts
	r.measurer.Submit(ref)
	return playoutFormatOf(facts), true
}

// submitAnalysis queues background analysis for a source whose stream facts are already stored.
func (r *playoutResolver) submitAnalysis(id inventory.SourceID, revision, input string, facts inventory.SourceFacts) {
	if r.measurer != nil {
		r.measurer.Submit(mediameasure.SourceRef{ID: id, Revision: revision, Path: input, Facts: facts})
	}
}

func localRevisionOfPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return localInventoryRevision(info), nil
}
