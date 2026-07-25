package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/events"
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

// LocalClipIDsByName maps a clip's file name to the Tunarr program uuid Tunarr assigned it.
//
// Name is the only join key Tunarr's scan reports back, so it is what we have. Imperfect: two
// clips sharing a basename in different subfolders collide and one may take the other's uuid.
// That is tolerable ONLY because the uuid stopped being identity (§9.1) — a wrong uuid degrades
// a Tunarr filler-list, it cannot corrupt the catalog or misdirect internal playout, both of
// which key on the path.
func (a fillerSourceAdapter) LocalClipIDsByName(ctx context.Context) (map[string]string, error) {
	clips, err := a.prog.ListLocalFillerClipsAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(clips))
	for _, c := range clips {
		// Tunarr reports the display name; the scanner derives its Name the same way (base
		// filename without extension), so strip an extension here if Tunarr kept one.
		name := strings.TrimSuffix(c.Name, filepath.Ext(c.Name))
		out[name] = c.ProgramID
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
	// fetcher is nil unless the running image carries the ingest tooling (loomarr:filler
	// — §16). nil is the normal state on loomarr:latest, not a misconfiguration.
	fetcher *clipfetch.Ingestor
	bus     *events.Bus
	newID   func() string
	timeout time.Duration
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

// Ingest downloads clips into the drop-folder (§10). It returns a job id immediately and
// does the work in the background: a playlist can take minutes to hours, so holding the
// HTTP request open would guarantee a gateway timeout on exactly the useful cases.
// Progress rides the SSE bus as `filler_ingest` frames — the same shape the §8.1 model
// pull uses, because it is the same problem (long external process, no request to hold).
func (a fillerServiceAdapter) Ingest(_ context.Context, urls []string) (string, error) {
	if a.fetcher == nil {
		return "", api.ErrIngestUnavailable
	}
	sources := make([]clipfetch.Source, 0, len(urls))
	for _, u := range urls {
		sources = append(sources, clipfetch.Source{Kind: clipfetch.KindForURL(u), URL: u})
	}

	jobID := a.newID()
	go func() {
		// Deliberately NOT the request context: the HTTP response has already been
		// written, so tying the download to it would cancel every ingest the moment the
		// client disconnected. The timeout below is what bounds the work instead.
		bg := context.Background()
		if a.timeout > 0 {
			var cancel context.CancelFunc
			bg, cancel = context.WithTimeout(bg, a.timeout*time.Duration(len(sources)))
			defer cancel()
		}
		a.publishIngest(jobID, "starting", clipfetch.Result{}, "")
		res := a.fetcher.Run(bg, sources)
		if res.Failed > 0 && res.Fetched == 0 {
			// Every source failed — report it as an error rather than a success with
			// zeroes, which reads as "nothing to download" and hides a bad URL or an
			// expired yt-dlp.
			a.publishIngest(jobID, "error", res, "every source failed; check the URLs and the yt-dlp version")
			return
		}
		a.publishIngest(jobID, "success", res, "")
	}()
	return jobID, nil
}

// publishIngest emits one ingest-progress frame (§7, type=filler_ingest). Empty counts
// sources that yielded nothing without erroring — a typo'd Archive id returns 200 with no
// items, so without surfacing it the operator sees "fetched:0 failed:0" and no reason.
func (a fillerServiceAdapter) publishIngest(jobID, status string, res clipfetch.Result, errMsg string) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(events.Event{
		Type: "filler_ingest",
		Payload: map[string]any{
			"jobId": jobID, "status": status,
			"fetched": res.Fetched, "skipped": res.Skipped,
			"failed": res.Failed, "empty": res.Empty, "error": errMsg,
		},
	})
}

// podPreviewAdapter bridges filler.PodAdapter → api.PodPreviewer (§12). It exists to
// derive the selection + seed from the channel using the SAME exported helpers the
// reconciler calls, so a preview and the next reconcile of that channel produce the
// identical pool. The API stays out of that derivation entirely.
type podPreviewAdapter struct {
	store store.Store
	pods  *filler.PodAdapter
}

// Preview assembles the pool for the channel's SAVED filler selection (the GET …/pods
// path).
func (a podPreviewAdapter) Preview(ctx context.Context, channelID string) (filler.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return filler.Pod{}, err
	}
	return a.pods.Preview(ctx, ch.ID, channels.PodSeed(ch.ID), channels.SelectionForChannel(ch))
}

// PreviewDraft assembles the pool for a DRAFT selection (the POST …/pods/preview
// sandbox) — the same seed as the saved preview (so only the selection differs), but the
// caller's unsaved selection in place of the persisted one. The era still defaults from
// the channel's program scope when the draft leaves it unset, matching SelectionForChannel.
func (a podPreviewAdapter) PreviewDraft(ctx context.Context, channelID string, sel filler.Selection) (filler.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return filler.Pod{}, err
	}
	if sel.Era == 0 && ch.Policy.Scope.Era != nil {
		sel.Era = ch.Policy.Scope.Era.From
	}
	return a.pods.Preview(ctx, ch.ID, channels.PodSeed(ch.ID), sel)
}
