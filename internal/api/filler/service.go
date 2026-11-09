package filler

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Service backs filler acquisition and readiness routes (§10). Clip edits read/write the
// store directly; descriptive enrichment is automatic background work rather than an API command.
type Service interface {
	// Readiness is the one server-owned operational summary used by the Filler overview.
	Readiness(ctx context.Context) (filler.Readiness, error)
	// PreviewStorageCleanup and CleanupStorage expose only terminal private staging proven safe
	// to remove. They never select catalog media or unresolved recovery evidence.
	PreviewStorageCleanup(ctx context.Context) (acquisition.StorageCleanupPreview, error)
	CleanupStorage(ctx context.Context) (acquisition.StorageCleanupResult, error)
	// Sync reconciles the clip catalog from the media server's filler library.
	Sync(ctx context.Context) (total, added, updated, pruned int, err error)
	// Fetch runs one bounded acquisition pass over enabled remote sources. It is the same
	// operation the scheduled filler-fetch job runs; the Sources page uses it to run that policy
	// now rather than merely scanning local files and claiming a remote fetch succeeded.
	Fetch(ctx context.Context, sourceID string) (acquisition.FetchResult, error)
	// SuggestSources and ResolveSource are read-only provider discovery. They create no registry
	// row and grant no acquisition authority; POST /v1/filler/sources remains that boundary.
	SuggestSources(ctx context.Context, provider, query string, limit int) ([]source.Suggestion, error)
	ResolveSource(ctx context.Context, provider, input string) (source.Suggestion, error)
	// Ingest downloads clips from the given source URLs into the drop-folder, returning
	// a job id immediately (§10). Downloads take minutes to hours, so this is
	// fire-and-report: progress arrives on the SSE bus as `filler_ingest` frames, the
	// same shape the model pull uses (§8.1). ErrIngestUnavailable when the running image
	// lacks the tooling.
	//
	// ⚠ Downloads and nothing else — it does not register a source.
	Ingest(ctx context.Context, urls []string) (jobID string, err error)
	// IngestSourceItems queues exact candidates discovered inside one registered source. The
	// source identity and provider kind are server-owned policy; RemoteID preserves provider
	// deduplication and this path never registers an item as another source.
	IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []source.Item) (jobID string, err error)
	// IngestPull validates readiness, constructs the queued run, and calls commit exactly once
	// before launching the ordinary downloader. A failed commit must not start any work.
	IngestPull(ctx context.Context, pullID string, targets []acquisition.AcquisitionTarget, commit func(context.Context, acquisition.AcquisitionRun) error) (jobID string, err error)
	// IngestAsked is Ingest plus "remember this source", for `POST /v1/filler/ingest` only.
	//
	// ⚠ The split is a correctness fix: registration used to live inside Ingest, and auto-fetch
	// passes the URL of every ITEM inside an already-registered collection — so one fetch
	// registered 30 clips as 30 sources. Only the caller knows which it holds.
	IngestAsked(ctx context.Context, urls []string) (jobID string, err error)
	// Discover searches archive.org for candidate clips WITHOUT downloading anything (§10,
	// V33). One search request whatever the result count — the operator is deciding what to
	// fetch, and making them download gigabytes to find out would defeat the point.
	//
	// ⚠ Unlike Ingest this is SYNCHRONOUS: it is one HTTP call to a public JSON API, fast
	// enough to answer in the request, so there is no job to report on. That asymmetry is
	// deliberate — a job id for a sub-second read would be ceremony the caller has to poll.
	Discover(ctx context.Context, query string, limit int) ([]DiscoveredClip, int, error)
	// DiscoverCollection lists ONE named collection, downloading nothing (§10, V17d). Same
	// contract as Discover — synchronous, listing-only — with the collection as the argument
	// instead of a keyword. This is the starter pack: a curated collection an operator keeps
	// or excludes from before anything is fetched, so a suggested pack and a search result
	// travel the same path rather than the pack growing its own acquisition route.
	//
	// `ref` accepts a full URL, a /details/<id> path, or a bare identifier — the spellings
	// Ingest already takes, so an operator pasting a URL need not know which form is wanted.
	// When query is non-empty, results are filtered within the named collection. An empty query
	// lists the collection, preserving the starter-pack browse path.
	DiscoverCollection(ctx context.Context, ref, query string, limit int) ([]DiscoveredClip, int, error)
	// EnrichDiscovered fills in duration + quality for specific results, ON DEMAND.
	//
	// ⚠ Separate from Discover because it is a DIFFERENT cost, measured: a listing is one
	// request, while these fields are one `/metadata/<id>` call per row — 22.6s for a page of
	// 25 against the live API, and worse if the fan-out is widened, because archive.org
	// throttles. So a client asks only for the rows a person is actually looking at.
	//
	// Returns what it learned, keyed by id; an id it could not answer for is simply absent
	// rather than zero, because 0 means "unknown" to every caller and a map entry saying 0
	// would be indistinguishable from a genuinely empty clip.
	EnrichDiscovered(ctx context.Context, ids []string) (map[string]DiscoveredClipStats, error)
	// Split proposes cuts for a compilation clip (§10, V34). Fire-and-report like
	// Ingest — detection runs minutes per file, so progress arrives on the SSE bus
	// as `filler_split` frames and the finished proposal is read back via
	// GET /v1/filler/splits/{id}. The clip must exist (ErrNotFound otherwise).
	Split(ctx context.Context, clipID string) (jobID string, err error)
	// ConfirmSplit commits the operator's reviewed cut list (§10, V34): the ONLY
	// path by which proposal segments become catalog clips. Synchronous — stream
	// copy cuts seek rather than decode, so this is seconds, not the detection's
	// minutes. filler.ErrSplitValidation ⇒ 422; a missing proposal ⇒ ErrNotFound.
	ConfirmSplit(ctx context.Context, proposalID string, segments []split.Segment) error
}

// TaxonomyEditor is the deep graph-edit module used by taxonomy writes. The store still owns the
// atomic graph transaction; this seam adds an authoritative preview and post-commit channel
// convergence without making HTTP handlers understand either implementation.
type TaxonomyEditor interface {
	Preview(ctx context.Context, edit clipstore.TaxonomyEdit) (TaxonomyImpact, error)
	Apply(ctx context.Context, edit clipstore.TaxonomyEdit) (TaxonomyImpact, error)
}

type TaxonomyImpact struct {
	Store    clipstore.TaxonomyEditImpact
	Channels []TaxonomyChannelImpact
}

type TaxonomyChannelImpact struct {
	ID     string
	Name   string
	Number int
}

// Rewinder is the optional recovery seam behind the Incoming UI. Separate from Service so a
// runtime without a pipeline reports 501 and lightweight callers need not fake a recovery
// operation they cannot perform.
type Rewinder interface {
	Rewind(ctx context.Context, hash string, from pipelinestate.StageID, force bool) error
	RetryFailure(ctx context.Context, hash string) error
}

// DiscoveredClip is one candidate the operator could add (§10, V33).
//
// ⚠ NOT a ClipDTO: nothing has been downloaded, so it has no path and no tags — only what the
// source knows about it. Reusing ClipDTO would advertise fields that are structurally
// unavailable at this stage.
//
// ⚠ DurationMS and Height are absent from a SEARCH and filled by a follow-up enrich call (V35),
// so a client must treat them as arriving late rather than as missing. They are on this struct
// rather than a separate type because a row gains them in place.
type DiscoveredClip struct {
	// ID is the source's identifier — what Ingest would be given to fetch it.
	ID string `json:"id"`
	// Title as the source has it; may be empty, in which case a client shows the id.
	Title string `json:"title,omitempty"`
	// Year the source catalogued it under; 0 when it declares none, which is common.
	//
	// ⚠ A WEAK hint, never a clip's era. It is when the ITEM was catalogued — for a
	// compilation upload that is often the upload year, not the broadcast year — which is the
	// same trap §10 records for yt-dlp's `upload_date`.
	Year int `json:"year,omitempty"`
	// URL is the item's page, so an operator can look at it before adding it.
	URL string `json:"url"`
	// Date is the source's catalogued date (RFC3339), absent when it declares none.
	//
	// ⚠ Carries Year's weak-hint caveat, and the live data shows it: one pinned item is dated
	// 1996 while archive.org's `publicdate` for it is 2023. Rendered for a human to read;
	// never used to set a clip's era.
	Date string `json:"date,omitempty"`
	// DurationMS is the item's runtime. ⚠ ABSENT means unknown, never zero-length — archive.org
	// has not probed every item's files, and a client must render "—" rather than "0:00", which
	// would claim the clip is empty. Costs a per-item metadata call (see clipfetch.enrich).
	DurationMS int `json:"durationMs,omitempty"`
	// Height is the best available derivative's vertical resolution, rendered as a quality hint
	// ("480p"). Absent means unknown, on the same terms as DurationMS.
	Height int `json:"height,omitempty"`
	// ThumbnailURL is archive.org's own item thumbnail. Free — a stable URL pattern needing no
	// API call — which is why it is built here rather than fetched.
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	// EmbedURL plays the item in its provider's own player; absent when it cannot be embedded.
	EmbedURL string `json:"embedUrl,omitempty"`
}

// DiscoveredClipStats is what a per-item metadata call learns about one result (§10, V35) —
// the two fields a search cannot afford to include.
//
// ⚠ A separate type rather than reusing DiscoveredClip, because the enrich response must not be
// able to CONTRADICT the search: returning whole clips would let a second call disagree about a
// title or a URL the client already rendered, and there is no rule for which one wins. This
// carries only what the metadata call is authoritative for.
type DiscoveredClipStats struct {
	// DurationMS is the runtime. ⚠ Only ever present when known: an id archive.org has not
	// probed is OMITTED from the map, never returned as 0, since 0 and "unknown" render
	// differently and only one of them is true.
	DurationMS int `json:"durationMs,omitempty"`
	// Height is the best derivative's vertical resolution — the "480p" quality hint.
	Height int `json:"height,omitempty"`
}

// ErrSplitUnavailable reports that compilation splitting cannot run because the
// filler drop-folder (filler.dir) is unset — clip paths are relative to it, so
// without it there is nothing to cut. Unlike ErrIngestUnavailable this IS a
// configuration remedy: Settings → Filler opens it (§10, V34).
var ErrSplitUnavailable = errors.New("splitting needs the filler drop-folder (filler.dir)")
