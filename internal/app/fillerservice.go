package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/operator"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

// fillerServiceAdapter is the filler operator as the HTTP surface sees it: it holds what the
// composition root wires, hands the operations to operator.Operator, and keeps the one thing that
// speaks the API's own types, the archive.org discovery listings.
type fillerServiceAdapter struct {
	syncer  *filler.Syncer
	fetcher operator.Fetcher
	// afterIngest closes the watch-folder → catalog → pipeline loop before a successful ingest
	// event is published (§10 V56). Optional only in narrow unit tests.
	afterIngest  func(context.Context) error
	bus          *events.Bus
	log          *slog.Logger
	newID        func() string
	timeout      time.Duration
	start        interactiveOperationLauncher
	operations   operator.OperationWriter
	sources      operator.SourceRegistry
	providers    *source.Registry
	home         func() pod.Geography
	acquisitions operator.AcquisitionWriter
	readiness    operator.ReadinessStore
	pool         func(context.Context) (pod.PoolReport, error)
	now          func() time.Time
	splitter     *filler.Splitter
	splitClips   storeadapter.Split
	pipeline     *filler.Pipeline
	autoFetch    *acquisition.Fetcher
	storage      *storagegovernor.Governor

	storagePath      string
	storageAutomatic func() bool
	storageCleanup   *fetch.AcquisitionCleaner
}

// fillerEvents turns the operator's progress into the wire frames the SSE bus carries.
type fillerEvents struct{ bus *events.Bus }

func (e fillerEvents) IngestProgress(jobID, status string, res fetch.Result, errMsg string) {
	e.bus.Publish(events.Event{
		Type: "filler_ingest",
		Payload: opsapi.FillerIngestEvent{
			JobID: jobID, Status: status,
			Fetched: res.Fetched, Skipped: res.Skipped,
			Failed: res.Failed, Empty: res.Empty, Error: errMsg,
		},
	})
}

func (e fillerEvents) SplitProgress(jobID, clipHash, status, proposalID string, segments int, errMsg string) {
	e.bus.Publish(events.Event{
		Type: "filler_split",
		Payload: opsapi.FillerSplitEvent{
			JobID: jobID, ClipHash: clipHash, Status: status,
			ProposalID: proposalID, Segments: segments, Error: errMsg,
		},
	})
}

// op is the operator over the adapter's current wiring.
func (a fillerServiceAdapter) op() *operator.Operator {
	var sink operator.Events
	if a.bus != nil {
		sink = fillerEvents{bus: a.bus}
	}
	return operator.New(operator.Config{
		Syncer: a.syncer, Fetcher: a.fetcher, AfterIngest: a.afterIngest, Events: sink, Log: a.log,
		NewID: a.newID, Timeout: a.timeout, Start: operator.Launcher(a.start), Operations: a.operations,
		Sources: a.sources, Providers: a.providers, Acquisitions: a.acquisitions, Readiness: a.readiness,
		Pool: a.pool, Now: a.now, Splitter: a.splitter, SplitClips: a.splitClips, Pipeline: a.pipeline,
		AutoFetch: a.autoFetch, Storage: a.storage, StoragePath: a.storagePath,
		StorageAutomatic: a.storageAutomatic, StorageCleanup: a.storageCleanup,
	})
}

var _ fillerapi.Rewinder = fillerServiceAdapter{}

func (a fillerServiceAdapter) SuggestSources(ctx context.Context, provider, query string, limit int) ([]source.Suggestion, error) {
	return a.op().SuggestSources(ctx, provider, query, limit)
}

func (a fillerServiceAdapter) ResolveSource(ctx context.Context, provider, input string) (source.Suggestion, error) {
	return a.op().ResolveSource(ctx, provider, input)
}

func (a fillerServiceAdapter) Readiness(ctx context.Context) (filler.Readiness, error) {
	return a.op().Readiness(ctx)
}

func (a fillerServiceAdapter) PreviewStorageCleanup(ctx context.Context) (acquisition.StorageCleanupPreview, error) {
	return a.op().PreviewStorageCleanup(ctx)
}

func (a fillerServiceAdapter) CleanupStorage(ctx context.Context) (acquisition.StorageCleanupResult, error) {
	return a.op().CleanupStorage(ctx)
}

func (a fillerServiceAdapter) FetchStatus(ctx context.Context) (acquisition.FetchStatus, error) {
	return a.op().FetchStatus(ctx)
}

func (a fillerServiceAdapter) Fetch(ctx context.Context, sourceID string) (acquisition.FetchResult, error) {
	return a.op().Fetch(ctx, sourceID)
}

func (a fillerServiceAdapter) Rewind(ctx context.Context, hash string, from pipelinestate.StageID, force bool) error {
	return a.op().Rewind(ctx, hash, from, force)
}

func (a fillerServiceAdapter) RetryFailure(ctx context.Context, hash string) error {
	return a.op().RetryFailure(ctx, hash)
}

func (a fillerServiceAdapter) Sync(ctx context.Context) (int, int, int, int, error) {
	return a.op().Sync(ctx)
}

func (a fillerServiceAdapter) Ingest(ctx context.Context, urls []string) (string, error) {
	return a.op().Ingest(ctx, urls)
}

func (a fillerServiceAdapter) IngestSource(ctx context.Context, sourceID, sourceKind string, urls []string) (string, error) {
	return a.op().IngestSource(ctx, sourceID, sourceKind, urls)
}

func (a fillerServiceAdapter) IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []source.Item) (string, error) {
	return a.op().IngestSourceItems(ctx, sourceID, sourceKind, items)
}

func (a fillerServiceAdapter) IngestAsked(ctx context.Context, urls []string) (string, error) {
	return a.op().IngestAsked(ctx, urls)
}

func (a fillerServiceAdapter) IngestPull(
	ctx context.Context,
	pullID string,
	targets []acquisition.AcquisitionTarget,
	commit func(context.Context, acquisition.AcquisitionRun) error,
) (string, error) {
	return a.op().IngestPull(ctx, pullID, targets, commit)
}

func (a fillerServiceAdapter) Split(ctx context.Context, clipID string) (string, error) {
	return a.op().Split(ctx, clipID)
}

func (a fillerServiceAdapter) ConfirmSplit(ctx context.Context, proposalID string, segments []split.Segment) error {
	return a.op().ConfirmSplit(ctx, proposalID, segments)
}

// Discover searches archive.org for clips the operator could add (§10, V33).
//
// ⚠ SYNCHRONOUS, unlike Ingest — and the asymmetry is the point. Ingest starts a
// download that runs for minutes to hours, so it returns a job id and reports on the SSE bus.
// This is one HTTP GET to a public JSON API that answers in well under a second, so a job id
// would be ceremony the caller has to poll for a result it could already have had.
//
// ⚠ Available WITHOUT the ingest tooling. Searching is plain net/http (§10 chose it precisely
// because Archive needs no key or binary), so an operator on the default image can browse and
// see what exists — they simply cannot fetch it. Refusing the search too would hide the reason
// the fetch is unavailable behind a second, unrelated-looking wall.
func (a fillerServiceAdapter) Discover(ctx context.Context, query string, limit int) ([]fillerapi.DiscoveredClip, int, error) {
	res, err := fetch.NewArchiveDownloader().Search(ctx, query, limit)
	if err != nil {
		return nil, 0, err
	}
	items, total := discoveredClips(res)
	return items, total, nil
}

// DiscoverCollection lists one collection (§10, V17d — the starter pack). Same shape as
// Discover: listing only, no ingest tooling needed, results mapped identically. The ONLY
// difference is which archive.org query gets asked, which is why both live here rather than
// the pack growing an acquisition path of its own.
//
// ⚠ This is the only production caller of fetch.ArchiveDownloader.DiscoverCollection: the
// starter pack reaches archive.org through here, not through an acquisition path of its own.
func (a fillerServiceAdapter) DiscoverCollection(ctx context.Context, ref, query string, limit int) ([]fillerapi.DiscoveredClip, int, error) {
	discoverer := fetch.NewArchiveDownloader()
	var (
		res fetch.DiscoveryResult
		err error
	)
	if strings.TrimSpace(query) == "" {
		res, err = discoverer.DiscoverCollection(ctx, ref, limit)
	} else {
		res, err = discoverer.SearchCollection(ctx, ref, query, limit)
	}
	if err != nil {
		return nil, 0, err
	}
	items, total := discoveredClips(res)
	return items, total, nil
}

// EnrichDiscovered fills in duration + quality for specific results (§10, V35).
//
// ⚠ Only ids archive.org actually answered for appear in the map. An item it never probed is
// OMITTED rather than returned as zero — 0 renders as "0:00", which claims a clip is empty,
// and absence is what lets a client render "—" instead.
func (a fillerServiceAdapter) EnrichDiscovered(ctx context.Context, ids []string) (map[string]fillerapi.DiscoveredClipStats, error) {
	items := make([]fetch.DiscoveredItem, len(ids))
	for i, id := range ids {
		items[i].ID = id
	}
	fetch.NewArchiveDownloader().Enrich(ctx, items)
	return discoveredStats(items), nil
}

// discoveredStats maps enriched items to the DTO map.
//
// ⚠ Split out from EnrichDiscovered so the omit-vs-zero rule is TESTABLE: the caller builds its
// own ArchiveDownloader (there is no seam to stub), so a test through the adapter would have to
// reach archive.org.
func discoveredStats(items []fetch.DiscoveredItem) map[string]fillerapi.DiscoveredClipStats {
	out := make(map[string]fillerapi.DiscoveredClipStats, len(items))
	for _, it := range items {
		if it.DurationMS == 0 && it.Height == 0 {
			continue // learned nothing — absent, never zeroed (see DiscoveredClipStats)
		}
		out[it.ID] = fillerapi.DiscoveredClipStats{DurationMS: it.DurationMS, Height: it.Height}
	}
	return out
}

// discoveredClips maps a fetch.DiscoveryResult to the DTO. Shared by both discovery modes so a
// field added for one cannot silently go missing from the other.
func discoveredClips(res fetch.DiscoveryResult) ([]fillerapi.DiscoveredClip, int) {
	out := make([]fillerapi.DiscoveredClip, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, fillerapi.DiscoveredClip{
			ID:         it.ID,
			Title:      it.Title,
			Year:       it.Year,
			Date:       it.Date,
			DurationMS: it.DurationMS,
			Height:     it.Height,
			// The item's own page, so an operator can look before adding. Built here
			// rather than in the fetch package because it is a presentation concern.
			URL: "https://archive.org/details/" + it.ID,
			// Archive's own thumbnail service: a stable URL pattern, no API call and no
			// per-item cost, which is why the row gets an image for free while duration and
			// quality had to be fetched.
			ThumbnailURL: "https://archive.org/services/img/" + it.ID,
		})
	}
	return out, res.Total
}
