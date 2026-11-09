package operator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Impact is what a taxonomy edit would change: the store's accounting of the graph and clips, and
// the channels whose saved filler selection names an affected category.
type Impact struct {
	Store    clipstore.TaxonomyEditImpact
	Channels []ChannelImpact
}

// ChannelImpact names a channel a taxonomy edit affects.
type ChannelImpact struct {
	ID     string
	Name   string
	Number int
}

// TaxonomyStore is the slice of the store a taxonomy edit reads and commits through.
type TaxonomyStore interface {
	PreviewTaxonomyEdit(ctx context.Context, edit clipstore.TaxonomyEdit) (clipstore.TaxonomyEditImpact, error)
	ApplyTaxonomyEdit(ctx context.Context, edit clipstore.TaxonomyEdit, at time.Time) error
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	ListClips(ctx context.Context, filter clipstore.ClipFilter) ([]clipstore.Clip, error)
}

// TaxonomyEditor is the deep module around a graph edit. Its interface is only preview/apply;
// behind that seam it combines the store's prospective graph accounting, saved channel references,
// the atomic commit, and best-effort post-commit convergence.
type TaxonomyEditor struct {
	store TaxonomyStore
	wake  *ChannelWake
	now   func() time.Time
	// holidaysChanged rereads the holiday snapshot: a seasonal taxon's edit can add or retire the
	// undated holiday it owns (#2030).
	holidaysChanged func(context.Context)
}

// NewTaxonomyEditor returns the editor over store. wake and holidaysChanged may be nil.
func NewTaxonomyEditor(store TaxonomyStore, wake *ChannelWake, holidaysChanged func(context.Context)) TaxonomyEditor {
	return TaxonomyEditor{store: store, wake: wake, holidaysChanged: holidaysChanged}
}

func (e TaxonomyEditor) Preview(ctx context.Context, edit clipstore.TaxonomyEdit) (Impact, error) {
	return e.preview(ctx, edit)
}

func (e TaxonomyEditor) Apply(ctx context.Context, edit clipstore.TaxonomyEdit) (Impact, error) {
	impact, err := e.preview(ctx, edit)
	if err != nil {
		return Impact{}, err
	}
	before, err := e.playableSnapshots(ctx, impact.Store.PlayableClipHashes)
	if err != nil {
		return Impact{}, err
	}
	now := time.Now
	if e.now != nil {
		now = e.now
	}
	if err := e.store.ApplyTaxonomyEdit(ctx, edit, now().UTC()); err != nil {
		return Impact{}, err
	}
	if e.holidaysChanged != nil {
		// The edit is committed: a client that hangs up now must not leave the snapshot stale.
		e.holidaysChanged(context.WithoutCancel(ctx))
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
	snapshots := append(before, after...)
	if e.wake != nil && len(snapshots) > 0 {
		e.wake.Run(ctx, snapshots)
	}
	return impact, nil
}

func (e TaxonomyEditor) preview(ctx context.Context, edit clipstore.TaxonomyEdit) (Impact, error) {
	if e.store == nil {
		return Impact{}, fmt.Errorf("preview taxonomy edit: no store configured")
	}
	storeImpact, err := e.store.PreviewTaxonomyEdit(ctx, edit)
	if err != nil {
		return Impact{}, err
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

	out := Impact{Store: storeImpact}
	channels, err := e.store.ListChannels(ctx)
	if err != nil {
		return Impact{}, fmt.Errorf("preview taxonomy channel selections: %w", err)
	}
	for _, channel := range channels {
		if channel.Policy.Filler == nil {
			continue
		}
		for _, slug := range channel.Policy.Filler.Categories {
			if targets[slug] {
				out.Channels = append(out.Channels, ChannelImpact{
					ID: channel.ID, Name: channel.Name, Number: channel.Number,
				})
				break
			}
		}
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].Number < out.Channels[j].Number })
	return out, nil
}

func (e TaxonomyEditor) playableSnapshots(ctx context.Context, hashes []string) ([]clips.Clip, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	rows, err := e.store.ListClips(ctx, clipstore.ClipFilter{Hashes: hashes})
	if err != nil {
		return nil, err
	}
	out := make([]clips.Clip, len(rows))
	for i := range rows {
		out[i] = rows[i].Clip
	}
	return out, nil
}
