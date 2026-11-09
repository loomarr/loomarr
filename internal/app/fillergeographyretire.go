package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// fillerGeographyRetirement removes clips no current place can air (#2059 decision 14): a local
// clip whose station is out of every place's radius, or a national clip from another country. It
// runs after enrichment and research, and considers only clips whose geography is settled; unknown
// or inferred geography is never a reason to remove one. Removal is the ordinary catalog tombstone,
// so the clip's hash stays known and fetching never pulls it again.
type fillerGeographyRetirement struct {
	clips interface {
		ListSettledGeographyClips(ctx context.Context) ([]records.SettledGeographyClip, error)
	}
	store interface {
		ListChannels(ctx context.Context) ([]channelstore.Channel, error)
		SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
	}
	policy func() pod.Policy
	log    *slog.Logger
	now    func() time.Time
}

func (r fillerGeographyRetirement) Run(ctx context.Context) (int, error) {
	areas, err := currentAreas(ctx, r.store, r.policy())
	if err != nil {
		return 0, fmt.Errorf("retire unusable filler: %w", err)
	}
	if len(areas) == 0 {
		return 0, nil
	}
	settled, err := r.clips.ListSettledGeographyClips(ctx)
	if err != nil {
		return 0, fmt.Errorf("retire unusable filler: %w", err)
	}
	var paths []string
	for _, c := range settled {
		clip := clips.Clip{GeographicScope: pod.GeographicScope(c.Scope), Country: c.Country, Station: c.Station}
		if !pod.UsableIn(clip, areas) {
			paths = append(paths, c.Path)
		}
	}
	if len(paths) == 0 {
		return 0, nil
	}
	removed, err := r.store.SetClipsRemoved(ctx, paths, r.now().UTC())
	if err != nil {
		return 0, fmt.Errorf("retire unusable filler: %w", err)
	}
	r.log.Info("filler: removed clips no current place can air", "removed", removed)
	return removed, nil
}
