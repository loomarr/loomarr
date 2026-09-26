package app

import (
	"context"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/mediameasure"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
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

// naturalBreakSource binds mid-roll candidate lookups (§10) to one reconcile or preview pass.
// Nil when this install cannot answer (no inventory, analysis store or library client).
func (r *playoutResolver) naturalBreakSource(ctx context.Context) schedule.NaturalBreakSource {
	if r == nil || r.inventory == nil || r.analyses == nil || r.lib == nil {
		return nil
	}
	return naturalBreaks{ctx: ctx, r: r}
}

type naturalBreaks struct {
	ctx context.Context
	r   *playoutResolver
}

// NaturalBreaks reads the scene fades Loomarr measured for a library item's local file. An item
// with no stored analysis for its current revision is queued for measurement (one low-priority
// worker, G7) and airs whole until a later pass finds it measured. A streamed (non-file) item
// has none: measurement reads files directly, never through the media server.
func (n naturalBreaks) NaturalBreaks(libraryItemID string) []schedule.BreakCandidate {
	ctx, r := n.ctx, n.r
	var pm library.PathMap
	if r.pathMap != nil {
		pm = r.pathMap()
	}
	input := r.lib.ResolveInput(ctx, libraryItemID, pm, library.StatReadableFile)
	if input.Kind != library.InputFile {
		return nil
	}
	origin, ok := r.ensureLocalInventorySource(ctx, input.URL)
	if !ok {
		return nil
	}
	source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
		Item: inventory.ItemRef{Origin: &origin}, Now: r.inventoryNow(),
		Kinds: []inventory.SourceKind{inventory.SourceLocalFile},
	})
	if err != nil || !found {
		return nil
	}
	analysis, measured, err := r.analyses.InventoryAnalysis(ctx, source.ID)
	if err != nil {
		return nil
	}
	if !measured {
		r.submitAnalysis(source.ID, source.Revision, input.URL, source.Observation.Facts)
		return nil
	}
	return fadeCandidates(analysis.Breaks)
}

// fadeCandidates keeps only breaks with a measured fade (OverlapMs > 0): a black-and-silence
// coincidence, or a chapter mark the measurement verified as sitting in one. A chapter mark alone
// is not a fade: on a remux the chapter at 904.862 s sat in a bright scene (YAVG 88.8 before,
// 93.4 after, where black reads 16), and cutting there would break mid-picture, which the
// maintainer's rule forbids. mediameasure.ChapterBreaks verifies marks (#1529); this is the
// scheduler-side guard that nothing unverified is ever cut at.
func fadeCandidates(breaks []inventory.Break) []schedule.BreakCandidate {
	var out []schedule.BreakCandidate
	for _, b := range breaks {
		if b.OverlapMs <= 0 {
			continue
		}
		out = append(out, schedule.BreakCandidate{AtMs: b.AtMs, Confidence: b.Confidence})
	}
	return out
}

func localRevisionOfPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return localInventoryRevision(info), nil
}
