package playoutresolve

import (
	"context"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/playout"
)

// sourceMeasurer is the part of measure.Measurer the resolver uses.
type sourceMeasurer interface {
	MeasureFacts(context.Context, measure.SourceRef) (inventory.SourceFacts, error)
	Submit(measure.SourceRef)
}

// measureSink is where measurements land: stream facts through the inventory service, everything
// else through the analysis store.
type measureSink struct {
	inventory.Service
	inventory.AnalysisStore
}

// NewMeasurer builds Loomarr's measurement job for this resolver: stream facts come from the same
// prober playout already uses, revisions from the same stat rule local sources are registered
// with, and the analysis tools from the given (background) runner.
func (r *Resolver) NewMeasurer(tools measure.Tools, analysis inventory.AnalysisStore) *measure.Measurer {
	return measure.New(measure.Deps{
		Sink:  measureSink{Service: r.inventory, AnalysisStore: analysis},
		Tools: tools,
		Facts: func(ctx context.Context, path string) (inventory.SourceFacts, error) {
			observed, err := r.probeSource(ctx, path)
			if err != nil {
				return inventory.SourceFacts{}, err
			}
			return InventoryFactsOf(observed), nil
		},
		Revision: localRevisionOfPath,
		Now:      func() time.Time { return r.inventoryNow() },
		Log:      r.log,
	})
}

// measureFirstPlay is the synchronous fast path for a source Loomarr has never measured: probe
// stream facts once, record them against the source revision, and queue the slower analysis. A
// probe failure returns false and playout keeps its existing behaviour.
func (r *Resolver) measureFirstPlay(ctx context.Context, origin inventory.OriginKey, input string) (formatOut playout.MediaFormat, ok bool) {
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
	ref := measure.SourceRef{ID: source.ID, Revision: source.Revision, Path: input}
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

// activePicture is the source's measured picture area for the channel watermark's anchor, from
// the stored analysis of its current revision; the zero Rect (unknown) before it is measured.
func (r *Resolver) activePicture(ctx context.Context, id inventory.SourceID) playout.Rect {
	if r.analyses == nil {
		return playout.Rect{}
	}
	a, ok, err := r.analyses.InventoryAnalysis(ctx, id)
	if err != nil || !ok || a.ActivePicture == nil {
		return playout.Rect{}
	}
	p := a.ActivePicture
	return playout.Rect{X: p.X, Y: p.Y, W: p.W, H: p.H}
}

// submitAnalysis queues background analysis for a source whose stream facts are already stored.
func (r *Resolver) submitAnalysis(id inventory.SourceID, revision, input string, facts inventory.SourceFacts) {
	if r.measurer != nil {
		r.measurer.Submit(measure.SourceRef{ID: id, Revision: revision, Path: input, Facts: facts})
	}
}

// canAnalyseItems reports whether this install can find, and read the analysis of, a library
// item's local file: inventory, an analysis store, and the Library to resolve the item's file.
func (r *Resolver) canAnalyseItems() bool {
	return r != nil && r.inventory != nil && r.analyses != nil && r.libraryFile != nil
}

// NaturalBreakSource binds mid-roll candidate lookups (§10) to one reconcile or preview pass.
// Nil when this install cannot answer.
func (r *Resolver) NaturalBreakSource(ctx context.Context) schedule.NaturalBreakSource {
	if !r.canAnalyseItems() {
		return nil
	}
	return naturalBreaks{ctx: ctx, r: r}
}

type naturalBreaks struct {
	ctx context.Context
	r   *Resolver
}

// NaturalBreaks reads the scene fades Loomarr measured for a library item's local file: of a
// movie in parts, each part's, at where the part airs in the programme. Where one part ends and
// the next begins is no break of itself: a movie split to fit two discs can be split mid-scene.
// An item with a file unmeasured airs whole until a later pass finds it measured (fileAnalysis
// queues it).
func (n naturalBreaks) NaturalBreaks(libraryItemID string) []schedule.BreakCandidate {
	var out []schedule.BreakCandidate
	measured := true
	for _, file := range n.r.programmeFiles(n.ctx, libraryItemID) {
		analysis, ok := n.r.fileAnalysis(n.ctx, file.Path)
		if !ok {
			measured = false
			continue // the rest are still queued
		}
		for _, c := range fadeCandidates(analysis.Breaks) {
			c.AtMs += file.Start.Milliseconds()
			out = append(out, c)
		}
	}
	if !measured {
		return nil
	}
	return out
}

// Premeasure queues each file of each scheduled library item that is not yet measured at its
// current revision (#1518): the scheduler's measure-ahead for the next PremeasureHorizon. The
// worker is the same single low-priority one first play uses, and Submit never blocks.
func (r *Resolver) Premeasure(ctx context.Context, libraryItemIDs []string) {
	if !r.canAnalyseItems() || r.measurer == nil {
		return
	}
	for _, id := range libraryItemIDs {
		for _, file := range r.programmeFiles(ctx, id) {
			if ctx.Err() != nil {
				return
			}
			r.fileAnalysis(ctx, file.Path)
		}
	}
}

// fileAnalysis is the stored analysis of a local file at its current revision. When there is
// none, the file is queued for measurement (one low-priority worker, G7) and measured is false.
func (r *Resolver) fileAnalysis(ctx context.Context, input string) (inventory.Analysis, bool) {
	origin, ok := r.ensureLocalInventorySource(ctx, input)
	if !ok {
		return inventory.Analysis{}, false
	}
	source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
		Item: inventory.ItemRef{Origin: &origin}, Now: r.inventoryNow(),
		Kinds: []inventory.SourceKind{inventory.SourceLocalFile},
	})
	if err != nil || !found {
		return inventory.Analysis{}, false
	}
	analysis, measured, err := r.analyses.InventoryAnalysis(ctx, source.ID)
	if err != nil {
		return inventory.Analysis{}, false
	}
	if !measured || analysis.Revision != source.Revision {
		r.submitAnalysis(source.ID, source.Revision, input, source.Observation.Facts)
		return inventory.Analysis{}, false
	}
	return analysis, true
}

// fadeCandidates keeps only breaks with a measured fade (OverlapMs > 0): a black-and-silence
// coincidence, or a chapter mark the measurement verified as sitting in one. A chapter mark alone
// is not a fade: on a remux the chapter at 904.862 s sat in a bright scene (YAVG 88.8 before,
// 93.4 after, where black reads 16), and cutting there would break mid-picture, which the
// maintainer's rule forbids. measure.ChapterBreaks verifies marks (#1529); this is the
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
