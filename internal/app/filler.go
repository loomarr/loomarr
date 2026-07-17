package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/store"
)

// fillerSourceAdapter bridges the Tunarr client → filler.FillerSource (§10): it
// ensures the `local` filler source over FILLER_DIR and reads the scanned clips.
// Clip identity = the Tunarr program uuid; duration comes from Tunarr's scan.
type fillerSourceAdapter struct{ prog *programmer.Tunarr }

func (a fillerSourceAdapter) EnsureLocalSource(ctx context.Context, dir string) error {
	_, err := a.prog.EnsureLocalFillerSource(ctx, dir)
	return err
}

func (a fillerSourceAdapter) ListLocalClips(ctx context.Context) ([]filler.RawClip, error) {
	// Find Loomarr's local source, then read its clips. EnsureLocalFillerSource ran
	// first (Sync ensures before listing), so a local source exists.
	clips, err := a.prog.ListLocalFillerClipsAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]filler.RawClip, len(clips))
	for i, c := range clips {
		// Infer kind + era from the clip name (§10 cheapest tagging tier): Tunarr's
		// scan gives id/name/duration only, so kind defaults to Commercial (pod-
		// eligible) unless the name says bumper/station/psa/trailer. Without this a
		// clip lands as a generic interstitial the pod assembler can never place, so
		// filler would silently never build unless AI tagging is on. AI tagging (§10)
		// still refines era/audience/category afterward.
		out[i] = filler.RawClip{
			TunarrProgramID: c.ProgramID, Name: c.Name, DurationMs: c.DurationMs,
			Kind: filler.KindFromName(c.Name), Era: filler.EraFromName(c.Name),
		}
	}
	return out, nil
}

// fillerStoreAdapter bridges the store's clip methods → filler.Store (the sync).
type fillerStoreAdapter struct{ st store.Store }

func (a fillerStoreAdapter) UpsertClip(ctx context.Context, c filler.StoreClip) error {
	return a.st.UpsertClip(ctx, store.Clip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
}
func (a fillerStoreAdapter) GetClip(ctx context.Context, id string) (filler.StoreClip, bool, error) {
	c, err := a.st.GetClip(ctx, id)
	if err == store.ErrNotFound {
		return filler.StoreClip{}, false, nil
	}
	if err != nil {
		return filler.StoreClip{}, false, err
	}
	return filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt}, true, nil
}
func (a fillerStoreAdapter) DeleteClipsNotIn(ctx context.Context, keep []string) (int, error) {
	return a.st.DeleteClipsNotIn(ctx, keep)
}

// fillerTagStoreAdapter bridges the store → filler.TagStore (the AI-tagging job).
type fillerTagStoreAdapter struct{ st store.Store }

func (a fillerTagStoreAdapter) ListUntaggedCommercials(ctx context.Context) ([]filler.StoreClip, error) {
	clips, err := a.st.ListUntaggedCommercials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]filler.StoreClip, len(clips))
	for i, c := range clips {
		out[i] = filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt}
	}
	return out, nil
}
func (a fillerTagStoreAdapter) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error {
	return a.st.UpdateClipTags(ctx, id, era, audience, category, aiTagged, updatedAt)
}

// clipCatalogAdapter bridges the store → filler.CatalogReader (pod assembly).
type clipCatalogAdapter struct{ st store.Store }

func (a clipCatalogAdapter) AllClips(ctx context.Context) ([]filler.Clip, error) {
	clips, err := a.st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]filler.Clip, len(clips))
	for i, c := range clips {
		out[i] = c.Clip
	}
	return out, nil
}

// fillerServiceAdapter bridges filler.Syncer/Tagger → api.FillerService.
type fillerServiceAdapter struct {
	syncer *filler.Syncer
	tagger *filler.Tagger
}

func (a fillerServiceAdapter) Sync(ctx context.Context) (int, int, int, int, error) {
	res, err := a.syncer.Sync(ctx)
	return res.Total, res.Added, res.Updated, res.Pruned, err
}
func (a fillerServiceAdapter) Tag(ctx context.Context) (int, int, int, int, error) {
	if a.tagger == nil {
		return 0, 0, 0, 0, nil // AI tagging disabled (FILLER_AI_TAGGING=false)
	}
	res, err := a.tagger.Run(ctx)
	return res.Considered, res.Tagged, res.Partial, res.Skipped, err
}

// runFillerSync runs the periodic filler catalog sync until ctx is done (§10).
func runFillerSync(ctx context.Context, syncer *filler.Syncer, every time.Duration, log *slog.Logger) {
	if every <= 0 {
		every = 15 * time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	if _, err := syncer.Sync(ctx); err != nil {
		log.Warn("initial filler sync", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := syncer.Sync(ctx); err != nil {
				log.Warn("filler sync", "err", err)
			}
		}
	}
}
