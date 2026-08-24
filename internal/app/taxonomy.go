package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// taxonomyEditor is the deep app module around a graph edit. Its interface is only preview/apply;
// behind that seam it combines the store's prospective graph accounting, saved channel references,
// the atomic commit, and best-effort post-commit convergence.
type taxonomyEditor struct {
	store store.Store
	wake  *fillerChannelWake
	now   func() time.Time
}

func (e taxonomyEditor) Preview(ctx context.Context, edit store.TaxonomyEdit) (api.TaxonomyImpact, error) {
	return e.preview(ctx, edit)
}

func (e taxonomyEditor) Apply(ctx context.Context, edit store.TaxonomyEdit) (api.TaxonomyImpact, error) {
	impact, err := e.preview(ctx, edit)
	if err != nil {
		return api.TaxonomyImpact{}, err
	}
	before, err := e.playableSnapshots(ctx, impact.Store.PlayableClipHashes)
	if err != nil {
		return api.TaxonomyImpact{}, err
	}
	now := time.Now
	if e.now != nil {
		now = e.now
	}
	if err := e.store.ApplyTaxonomyEdit(ctx, edit, now().UTC()); err != nil {
		return api.TaxonomyImpact{}, err
	}
	after, err := e.playableSnapshots(ctx, impact.Store.PlayableClipHashes)
	if err != nil {
		// The semantic graph edit is already durable. The ordinary channel sweep is the crash-safe
		// convergence path, so a post-commit snapshot failure must not turn success into a retryable
		// HTTP error that applies the same edit twice.
		if e.wake != nil && e.wake.log != nil {
			e.wake.log.Warn("taxonomy changed but updated filler snapshots could not be loaded; channel sweep will converge", "err", err)
		}
		return impact, nil
	}
	if e.wake != nil {
		e.wake.Run(ctx, append(before, after...))
	}
	return impact, nil
}

func (e taxonomyEditor) preview(ctx context.Context, edit store.TaxonomyEdit) (api.TaxonomyImpact, error) {
	if e.store == nil {
		return api.TaxonomyImpact{}, fmt.Errorf("preview taxonomy edit: no store configured")
	}
	storeImpact, err := e.store.PreviewTaxonomyEdit(ctx, edit)
	if err != nil {
		return api.TaxonomyImpact{}, err
	}
	targets := map[string]bool{}
	if edit.Delete {
		targets[edit.Slug] = true
	} else if !edit.Create {
		targets[edit.Taxon.Slug] = true
	}
	for _, descendant := range storeImpact.Descendants {
		targets[descendant.Slug] = true
	}

	out := api.TaxonomyImpact{Store: storeImpact}
	channels, err := e.store.ListChannels(ctx)
	if err != nil {
		return api.TaxonomyImpact{}, fmt.Errorf("preview taxonomy channel selections: %w", err)
	}
	for _, channel := range channels {
		if channel.Policy.Filler == nil {
			continue
		}
		for _, slug := range channel.Policy.Filler.Categories {
			if targets[slug] {
				out.Channels = append(out.Channels, api.TaxonomyChannelImpact{
					ID: channel.ID, Name: channel.Name, Number: channel.Number,
				})
				break
			}
		}
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].Number < out.Channels[j].Number })
	return out, nil
}

func (e taxonomyEditor) playableSnapshots(ctx context.Context, hashes []string) ([]filler.Clip, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	clips, err := e.store.ListClips(ctx, store.ClipFilter{Hashes: hashes})
	if err != nil {
		return nil, err
	}
	out := make([]filler.Clip, len(clips))
	for i := range clips {
		out[i] = clips[i].Clip
	}
	return out, nil
}
