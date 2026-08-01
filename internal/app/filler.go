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
func (a fillerTagStoreAdapter) UpdateClipTags(ctx context.Context, id string, era int, audience, category string, suggestedEra int, aiTagged bool, updatedAt time.Time) error {
	return a.st.UpdateClipTags(ctx, id, era, audience, category, suggestedEra, aiTagged, updatedAt)
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

// fillerSplitStoreAdapter bridges the store → filler.SplitStore (V34). The
// proposal methods pass filler.SplitProposal straight through — the store
// persists exactly that type, so there is nothing to translate.
type fillerSplitStoreAdapter struct{ st store.Store }

func (a fillerSplitStoreAdapter) GetClip(ctx context.Context, id string) (filler.StoreClip, bool, error) {
	return fillerStoreAdapter(a).GetClip(ctx, id)
}
func (a fillerSplitStoreAdapter) ListClips(ctx context.Context) ([]filler.StoreClip, error) {
	clips, err := a.st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]filler.StoreClip, len(clips))
	for i, c := range clips {
		out[i] = filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt}
	}
	return out, nil
}
func (a fillerSplitStoreAdapter) UpsertClip(ctx context.Context, c filler.StoreClip) error {
	return a.st.UpsertClip(ctx, store.Clip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
}
func (a fillerSplitStoreAdapter) DeleteClip(ctx context.Context, id string) error {
	return a.st.DeleteClip(ctx, id)
}
func (a fillerSplitStoreAdapter) UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	return a.st.UpsertSplitProposal(ctx, p)
}
func (a fillerSplitStoreAdapter) GetSplitProposal(ctx context.Context, id string) (filler.SplitProposal, error) {
	return a.st.GetSplitProposal(ctx, id)
}
func (a fillerSplitStoreAdapter) DeleteSplitProposal(ctx context.Context, id string) error {
	return a.st.DeleteSplitProposal(ctx, id)
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
	// sources registers what an operator added, so the Sources tab can show where clips
	// came from. Narrow interface, not the whole store: this adapter has no other reason
	// to reach persistence.
	sources store.FillerSourceStore
	now     func() time.Time
	// splitter / splitClips back compilation splitting (§10, V34). nil splitter ⇒
	// no drop-folder configured ⇒ Split/ConfirmSplit answer ErrSplitUnavailable.
	splitter   *filler.Splitter
	splitClips fillerSplitStoreAdapter
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
func (a fillerServiceAdapter) Ingest(ctx context.Context, urls []string) (string, error) {
	if a.fetcher == nil {
		return "", api.ErrIngestUnavailable
	}
	a.rememberSources(ctx, urls)
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

// Split starts compilation detection on one clip (§10, V34). It returns a job id
// immediately — a full-decode detection pass runs minutes per file — and reports
// over the SSE bus as `filler_split` frames, the same shape as Ingest above. The
// terminal frame carries the proposal id the review UI then reads back.
//
// The clip's existence is checked SYNCHRONOUSLY: "that clip is gone" is an
// answer the caller should get as a 404, not as an SSE error frame seconds later.
func (a fillerServiceAdapter) Split(ctx context.Context, clipID string) (string, error) {
	if a.splitter == nil {
		return "", api.ErrSplitUnavailable
	}
	if _, found, err := a.splitClips.GetClip(ctx, clipID); err != nil {
		return "", err
	} else if !found {
		return "", store.ErrNotFound
	}
	jobID := a.newID()
	go func() {
		// Deliberately NOT the request context, for the same reason as Ingest: the
		// response is already written, and detection must not die with the tab.
		bg := context.Background()
		if a.timeout > 0 {
			var cancel context.CancelFunc
			bg, cancel = context.WithTimeout(bg, a.timeout)
			defer cancel()
		}
		a.publishSplit(jobID, clipID, "running", "", 0, "")
		p, err := a.splitter.Propose(bg, clipID)
		if err != nil {
			a.publishSplit(jobID, clipID, "error", "", 0, err.Error())
			return
		}
		a.publishSplit(jobID, clipID, "success", p.ID, len(p.Segments), "")
	}()
	return jobID, nil
}

// ConfirmSplit commits the operator's reviewed cut list (§10, V34) — straight
// through to the splitter, whose validation errors (filler.ErrSplitValidation)
// and missing-proposal (store.ErrNotFound) the API maps to 422/404.
func (a fillerServiceAdapter) ConfirmSplit(ctx context.Context, proposalID string, segments []filler.SplitSegment) error {
	if a.splitter == nil {
		return api.ErrSplitUnavailable
	}
	return a.splitter.Confirm(ctx, proposalID, segments)
}

// publishSplit emits one split-detection frame (§7, type=filler_split). The
// terminal "success" frame is what hands the review UI its proposal id.
func (a fillerServiceAdapter) publishSplit(jobID, clipPath, status, proposalID string, segments int, errMsg string) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(events.Event{
		Type: "filler_split",
		Payload: map[string]any{
			"jobId": jobID, "clipPath": clipPath, "status": status,
			"proposalId": proposalID, "segments": segments, "error": errMsg,
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

// PreviewAt assembles the pod for the break starting at a specific instant — what internal
// playout airs there, and what the guide's hover card promises.
//
// The SAME call both consumers make, for the §10 one-assembler reason: if the guide computed
// a break independently, the hover card would eventually list clips that are not the ones
// playing, and nobody would be able to reproduce the discrepancy on demand.
func (a podPreviewAdapter) PreviewAt(ctx context.Context, channelID string, breakStartMs int64) (filler.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return filler.Pod{}, err
	}
	return a.pods.Preview(ctx, ch.ID, channels.PodSeedAt(ch.ID, breakStartMs), channels.SelectionForChannel(ch))
}

// Coverage reports which ladder rung this channel's breaks would draw from (V29b-api).
//
// Resolved through `SelectionForChannel`, exactly like the previews above — coverage that
// derived a channel's selection its own way could report on a different window than the pods
// it claims to describe, which is the "lying meter" the whole phase exists to prevent.
func (a podPreviewAdapter) Coverage(ctx context.Context, channelID string) (filler.CoverageReport, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return filler.CoverageReport{}, err
	}
	return a.pods.CoverageFor(ctx, ch.ID, channels.SelectionForChannel(ch))
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

// Discover searches archive.org for clips the operator could add (§10, V33).
//
// ⚠ SYNCHRONOUS, unlike Ingest above — and the asymmetry is the point. Ingest starts a
// download that runs for minutes to hours, so it returns a job id and reports on the SSE bus.
// This is one HTTP GET to a public JSON API that answers in well under a second, so a job id
// would be ceremony the caller has to poll for a result it could already have had.
//
// ⚠ Available WITHOUT the ingest tooling. Searching is plain net/http (§10 chose it precisely
// because Archive needs no key or binary), so an operator on the default image can browse and
// see what exists — they simply cannot fetch it. Refusing the search too would hide the reason
// the fetch is unavailable behind a second, unrelated-looking wall.
func (a fillerServiceAdapter) Discover(ctx context.Context, query string, limit int) ([]api.DiscoveredClip, int, error) {
	res, err := clipfetch.NewArchiveDownloader(false).Search(ctx, query, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]api.DiscoveredClip, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, api.DiscoveredClip{
			ID:    it.ID,
			Title: it.Title,
			Year:  it.Year,
			// The item's own page, so an operator can look before adding. Built here
			// rather than in clipfetch because it is a presentation concern.
			URL: "https://archive.org/details/" + it.ID,
		})
	}
	return out, res.Total, nil
}

// rememberSources records the archive.org items an operator asked for, so the Sources tab can
// show where the catalog came from (§10, V33).
//
// ⚠ BEST-EFFORT by contract, and the ordering says so: it runs before the download starts and
// a failure only logs. Registering where a clip came from must never be able to stop the clip
// being fetched — that would trade the feature for its own bookkeeping. The same reasoning
// RecordActivity carries.
//
// ⚠ Only archive.org items get a row. A YouTube URL is a video, not a source with state to
// remember: nothing about it needs a licence, a fetch cursor or a name an operator chose, and a
// row per video would turn the registry into a second, worse clip list.
func (a fillerServiceAdapter) rememberSources(ctx context.Context, urls []string) {
	if a.sources == nil {
		return
	}
	for _, u := range urls {
		if clipfetch.KindForURL(u) != clipfetch.Archive {
			continue
		}
		id := archiveIDFrom(u)
		if id == "" {
			continue
		}
		now := time.Now
		if a.now != nil {
			now = a.now
		}
		// Upsert by id: re-adding a source an operator already has is not an error, and
		// UpsertFillerSource deliberately leaves last_fetched_at alone (see the store).
		// ⚠ The error is deliberately DROPPED, not logged: this adapter has no logger (it
		// reports through the event bus, which is for the download itself), and inventing a
		// second channel for bookkeeping noise would be worse than the silence. The download
		// proceeds either way, which is the property that matters.
		_ = a.sources.UpsertFillerSource(ctx, store.FillerSource{
			ID: id, Kind: "archive", URI: u, Label: id, CreatedAt: now(),
		})
	}
}

// archiveIDFrom pulls the identifier out of an archive.org URL. Returns "" for anything that
// is not one, which the caller skips.
func archiveIDFrom(raw string) string {
	const marker = "archive.org/details/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return ""
	}
	id := raw[i+len(marker):]
	if j := strings.IndexAny(id, "/?#"); j >= 0 {
		id = id[:j]
	}
	return id
}
