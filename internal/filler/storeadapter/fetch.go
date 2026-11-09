package storeadapter

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Fetch bridges the store → acquisition.FetchStore (auto-fetch, §10 V38b).
type Fetch struct {
	st records.Store
	// fetchEvery is the GLOBAL poll interval a source's override resolves against (§10 V38c).
	// A closure so it hot-applies — an operator changing it expects the next pass to honour it.
	fetchEvery func() time.Duration
	home       func() pod.Geography
}

// NewFetch returns the auto-fetcher's store over st. fetchEvery and home are read on every pass.
func NewFetch(st records.Store, fetchEvery func() time.Duration, home func() pod.Geography) Fetch {
	return Fetch{st: st, fetchEvery: fetchEvery, home: home}
}

func (a Fetch) ListFetchSources(ctx context.Context) ([]acquisition.FetchSource, error) {
	srcs, err := a.st.ListFillerSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]acquisition.FetchSource, 0, len(srcs))
	for _, s := range srcs {
		eligible, region := true, 0
		if a.home != nil {
			home := a.home().Normalize()
			eligible = s.ServesHome(home)
			region = home.RegionRank(s.EffectiveGeography(home).Country)
		}
		out = append(out, s.FetchPlan(eligible, region, a.fetchEvery()))
	}
	return out, nil
}

// CatalogPaths returns every clip path.
//
// ⚠ `IncludeHeld` is REQUIRED. A held clip is not in the catalog by the default filter, but it is
// very much already on disk — omitting it would make auto-fetch re-download everything sitting in
// the review queue, on every pass, forever.
func (a Fetch) CatalogPaths(ctx context.Context) ([]string, error) {
	clips, err := a.st.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: true, IncludeRemoved: true})
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(clips))
	for i, c := range clips {
		paths[i] = c.Path
	}
	return paths, nil
}

func (a Fetch) ListAcquisitionRemoteStates(ctx context.Context) (map[string]acquisition.ExistingRemoteState, error) {
	return a.st.ListAcquisitionRemoteStates(ctx)
}

func (a Fetch) MarkFetched(ctx context.Context, id string, at time.Time) error {
	return a.st.MarkFillerSourceFetched(ctx, id, at)
}

func (a Fetch) ClaimCheck(
	ctx context.Context, id string, observedLastCheck, now, leaseUntil time.Time,
) (bool, error) {
	return a.st.ClaimFillerSourceCheck(ctx, id, observedLastCheck, now, leaseUntil)
}

func (a Fetch) CompleteCheck(
	ctx context.Context, id string, leaseUntil time.Time, completion acquisition.SourceCheckCompletion,
) error {
	return a.st.CompleteFillerSourceCheck(ctx, id, leaseUntil, completion)
}

func (a Fetch) FailCheck(ctx context.Context, id string, leaseUntil, retryAt time.Time) error {
	return a.st.FailFillerSourceCheck(ctx, id, leaseUntil, retryAt)
}

// Catalog bridges the store → pod.CatalogReader (pod assembly).
type Catalog struct{ st store.Store }

// NewCatalog returns the pod catalog reader over st.
func NewCatalog(st store.Store) Catalog { return Catalog{st: st} }

// ⚠ A ZERO filter, and that is what keeps HELD clips out of every pod (§10 V38). Pod assembly,
// coverage and the filler-list builder all read through here, so the exclusion living in
// ListClips means none of them can forget it. Adding IncludeHeld to this call would put every
// unreviewed download into live channels.
func (a Catalog) AllClips(ctx context.Context) ([]clips.Clip, error) {
	rows, err := a.st.ListClips(ctx, clipstore.ClipFilter{})
	if err != nil {
		return nil, err
	}
	variants, err := a.st.ListClipVariants(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]clips.Clip, len(rows))
	for i, c := range rows {
		out[i] = c.Clip
		out[i].Variants = variants[c.Hash]
	}
	return out, nil
}
