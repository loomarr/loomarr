package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// recordingSources captures what got registered, and can fail on demand.
type recordingSources struct {
	upserted []store.FillerSource
	err      error
}

type successfulClipIngestor struct{}

func (successfulClipIngestor) Run(context.Context, []clipfetch.Source) clipfetch.Result {
	return clipfetch.Result{Fetched: 1}
}

type recordingClipIngestor struct {
	sources chan []clipfetch.Source
	result  clipfetch.Result
}

type blockingClipIngestor struct {
	started   chan struct{}
	cancelled chan struct{}
}

func testInteractiveOperationLauncher(
	timeout time.Duration,
	run func(context.Context) error,
	complete func(context.Context, error),
) error {
	go func() {
		ctx := context.Background()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		err := run(ctx)
		if complete != nil {
			complete(context.Background(), err)
		}
	}()
	return nil
}

func (b blockingClipIngestor) Run(ctx context.Context, _ []clipfetch.Source) clipfetch.Result {
	close(b.started)
	<-ctx.Done()
	close(b.cancelled)
	return clipfetch.Result{}
}

func (r recordingClipIngestor) Run(_ context.Context, sources []clipfetch.Source) clipfetch.Result {
	r.sources <- append([]clipfetch.Source(nil), sources...)
	return r.result
}

type recordingAcquisitions struct {
	runs chan filler.AcquisitionRun
	err  error
}

func (r recordingAcquisitions) UpsertAcquisitionRun(_ context.Context, run filler.AcquisitionRun) error {
	if r.err != nil {
		return r.err
	}
	r.runs <- run
	return nil
}

func (r *recordingSources) ListFillerSources(context.Context) ([]store.FillerSource, error) {
	return r.upserted, nil
}
func (r *recordingSources) UpsertFillerSource(_ context.Context, s store.FillerSource) error {
	if r.err != nil {
		return r.err
	}
	r.upserted = append(r.upserted, s)
	return nil
}
func (r *recordingSources) DeleteFillerSource(context.Context, string) error { return nil }
func (r *recordingSources) MarkFillerSourceFetched(context.Context, string, time.Time) error {
	return nil
}
func (r *recordingSources) SetFillerSourceEnabled(context.Context, string, bool) error { return nil }
func (r *recordingSources) SetFillerSourceFetchPolicy(context.Context, string, *int, *int) error {
	return nil
}

func TestRememberSources(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	newAdapter := func(rs *recordingSources) fillerServiceAdapter {
		return fillerServiceAdapter{sources: rs, now: func() time.Time { return fixed }}
	}

	t.Run("registers an archive item by its identifier", func(t *testing.T) {
		rs := &recordingSources{}
		newAdapter(rs).rememberSources(context.Background(),
			[]string{"https://archive.org/details/classic_tv_commercials"})

		if len(rs.upserted) != 1 {
			t.Fatalf("registered %d sources, want 1", len(rs.upserted))
		}
		got := rs.upserted[0]
		// Keyed by IDENTIFIER, not the URL: re-adding the same collection spelled a
		// different way must update one row rather than growing a second.
		if got.ID != "classic_tv_commercials" {
			t.Errorf("id = %q, want the archive identifier", got.ID)
		}
		if got.Kind != "archive" {
			t.Errorf("kind = %q, want archive", got.Kind)
		}
		if !got.CreatedAt.Equal(fixed) {
			t.Errorf("CreatedAt = %v, want the injected clock", got.CreatedAt)
		}
	})

	// ⚠ A YouTube URL is a VIDEO, not a source with state to remember. Nothing about it needs
	// a licence, a fetch cursor or a name someone chose, and a row per video would turn the
	// registry into a second, worse clip list.
	t.Run("ignores non-archive urls", func(t *testing.T) {
		rs := &recordingSources{}
		newAdapter(rs).rememberSources(context.Background(), []string{
			"https://www.youtube.com/watch?v=abc123",
			"https://www.youtube.com/playlist?list=PL123",
		})
		if len(rs.upserted) != 0 {
			t.Errorf("registered %v, want nothing for YouTube urls", rs.upserted)
		}
	})

	// ⚠ THE property this is best-effort for: a registry failure must not be able to stop a
	// download. The method returns nothing, so this asserts it does not panic and leaves the
	// caller free to proceed.
	t.Run("survives a store failure", func(t *testing.T) {
		rs := &recordingSources{err: errors.New("disk full")}
		newAdapter(rs).rememberSources(context.Background(),
			[]string{"https://archive.org/details/x"})
		// Reaching here without a panic IS the assertion.
	})

	t.Run("does nothing when no store is wired", func(t *testing.T) {
		fillerServiceAdapter{}.rememberSources(context.Background(),
			[]string{"https://archive.org/details/x"})
	})
}

func TestArchiveIDFrom(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"https://archive.org/details/classic_tv_commercials", "classic_tv_commercials"},
		{"http://archive.org/details/item-1/", "item-1"},
		{"https://archive.org/details/item-2?start=3", "item-2"},
		{"https://archive.org/details/item-3#play", "item-3"},
		{"https://www.youtube.com/watch?v=abc", ""},
		{"not a url", ""},
		{"", ""},
	} {
		if got := archiveIDFrom(tc.in); got != tc.want {
			t.Errorf("archiveIDFrom(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ⚠ The omit-vs-zero rule, tested against the REAL mapping rather than through the API's fake
// — which implements the rule itself, so asserting there proves the fake works, not the code.
// Found by sabotage: deleting the rule from the adapter broke nothing.
//
// 0 means UNKNOWN. archive.org has not probed every item, and a client renders a present 0 as
// "0:00" — claiming the clip is empty, which is a different and false statement from "we don't
// know how long this is".
func TestDiscoveredStats_OmitsAnItemItLearnedNothingAbout(t *testing.T) {
	t.Parallel()
	stats := discoveredStats([]clipfetch.DiscoveredItem{
		{ID: "probed", DurationMS: 91_090, Height: 960},
		{ID: "unprobed"},
		// Partial knowledge is still knowledge: a height with no runtime must survive, or a
		// "known nothing" rule written as AND-vs-OR silently drops half the answers.
		{ID: "height-only", Height: 480},
		{ID: "duration-only", DurationMS: 30_000},
	})

	if _, present := stats["unprobed"]; present {
		t.Errorf("unprobed item present as %+v, want absent", stats["unprobed"])
	}
	for _, id := range []string{"probed", "height-only", "duration-only"} {
		if _, present := stats[id]; !present {
			t.Errorf("%s is missing — anything learned must survive", id)
		}
	}
	if stats["probed"].DurationMS != 91_090 || stats["probed"].Height != 960 {
		t.Errorf("probed = %+v, want the values it carried", stats["probed"])
	}
}

// ⚠ **AUTO-FETCH MUST NOT REGISTER A SOURCE PER CLIP.** `rememberSources` was written for V33,
// when an operator pasted one collection URL. V38b changed what the ingest path receives:
// auto-fetch enumerates a REGISTERED collection and passes the URL of every item inside it — so
// every downloaded clip registered itself, and one fetch turned 5 source rows into 35. The
// Sources tab became a second, worse clip list.
//
// The two entry points are what keep them apart: `Ingest` downloads, `IngestAsked` downloads and
// remembers. Only the caller knows which it holds.
func TestIngest_AutoFetchDoesNotRegisterEveryClipAsASource(t *testing.T) {
	t.Parallel()
	rec := &recordingSources{}
	a := fillerServiceAdapter{
		sources: rec,
		newID:   func() string { return "job-1" },
		now:     func() time.Time { return time.Unix(1_800_000_000, 0).UTC() },
	}

	// The shape auto-fetch sends: individual ITEMS inside a collection already registered.
	items := []string{
		"https://archive.org/details/CLE-B01_161770-162673",
		"https://archive.org/details/CalpolAdvert",
		"https://archive.org/details/CampbellsSoupAdvert",
	}
	// ⚠ A nil fetcher makes this return ErrIngestUnavailable BEFORE downloading, which is what
	// keeps the test hermetic — registration happens (or must not) regardless of the download.
	_, _ = a.Ingest(context.Background(), items)

	if len(rec.upserted) != 0 {
		t.Errorf("auto-fetch registered %d sources (%v) — one row per downloaded clip is the bug",
			len(rec.upserted), rec.upserted)
	}
}

// The mirror: an OPERATOR pasting a collection URL still gets it remembered, or the Sources tab
// stops answering "where did this come from".
func TestIngestAsked_RemembersWhatTheOperatorNamed(t *testing.T) {
	t.Parallel()
	rec := &recordingSources{}
	a := fillerServiceAdapter{
		sources: rec,
		newID:   func() string { return "job-1" },
		now:     func() time.Time { return time.Unix(1_800_000_000, 0).UTC() },
	}

	_, _ = a.IngestAsked(context.Background(), []string{"https://archive.org/details/classic_tv_commercials"})

	if len(rec.upserted) != 1 {
		t.Fatalf("registered %d sources, want the 1 the operator named", len(rec.upserted))
	}
	if rec.upserted[0].ID != "classic_tv_commercials" {
		t.Errorf("registered %q, want the collection identifier", rec.upserted[0].ID)
	}
}

// A successful downloader result was formerly the end of the job: bytes stayed in _watch until
// the next 15-minute scan, so Catalog and Incoming both looked unchanged. The completion hook is
// now part of success and closes that loop before the terminal event is emitted.
func TestIngest_SuccessCataloguesTheDownloadedFilesImmediately(t *testing.T) {
	prepared := make(chan struct{}, 1)
	a := fillerServiceAdapter{
		fetcher: successfulClipIngestor{},
		newID:   func() string { return "job-1" },
		start:   testInteractiveOperationLauncher,
		afterIngest: func(context.Context) error {
			prepared <- struct{}{}
			return nil
		},
	}

	if _, err := a.Ingest(t.Context(), []string{"https://archive.org/details/clip"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-prepared:
	case <-time.After(time.Second):
		t.Fatal("download completed but catalog sync/pipeline nudge never ran")
	}
}

func TestIngest_PersistsLifecycleAndCarriesAcquisitionToDownloader(t *testing.T) {
	fixed := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	fetched := recordingClipIngestor{
		sources: make(chan []clipfetch.Source, 1),
		result:  clipfetch.Result{Fetched: 2, Skipped: 1},
	}
	runs := make(chan filler.AcquisitionRun, 3)
	a := fillerServiceAdapter{
		fetcher: fetched, acquisitions: recordingAcquisitions{runs: runs},
		newID: func() string { return "acq-17" }, now: func() time.Time { return fixed },
		start: testInteractiveOperationLauncher,
	}

	jobID, err := a.IngestSource(t.Context(), "archive:classic", "archive", []string{
		"https://archive.org/details/one", "https://archive.org/details/two",
	})
	if err != nil {
		t.Fatal(err)
	}
	if jobID != "acq-17" {
		t.Fatalf("job id = %q, want acq-17", jobID)
	}
	for i, want := range []filler.AcquisitionStatus{
		filler.AcquisitionQueued, filler.AcquisitionRunning, filler.AcquisitionSuccess,
	} {
		select {
		case run := <-runs:
			if run.Status != want {
				t.Fatalf("snapshot %d status = %q, want %q", i, run.Status, want)
			}
			if run.ID != "acq-17" || run.SourceID != "archive:classic" || run.Requested != 2 {
				t.Fatalf("snapshot %d = %+v, want stable run identity and request count", i, run)
			}
			if want == filler.AcquisitionSuccess && (run.Fetched != 2 || run.Skipped != 1 || run.CompletedAt.IsZero()) {
				t.Fatalf("terminal snapshot = %+v, want downloader counts and completion", run)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for acquisition snapshot %d", i)
		}
	}
	select {
	case sources := <-fetched.sources:
		for _, source := range sources {
			if source.AcquisitionID != "acq-17" || source.ID != "archive:classic" || source.Kind != clipfetch.Archive {
				t.Fatalf("download source = %+v, want acquisition and source provenance", source)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("downloader was not called")
	}
}

func TestIngestPull_RejectsUnknownRegisteredKindBeforeDurableWork(t *testing.T) {
	runs := make(chan filler.AcquisitionRun, 1)
	a := fillerServiceAdapter{
		fetcher: successfulClipIngestor{}, acquisitions: recordingAcquisitions{runs: runs},
		newID: func() string { return "acq-17" },
	}
	if _, err := a.IngestPull(t.Context(), "pull-7", []filler.AcquisitionTarget{{
		SourceID: "unknown", Kind: "other", URL: "https://archive.org/details/one",
	}}); err == nil {
		t.Fatal("unknown registered kind started acquisition")
	}
	select {
	case run := <-runs:
		t.Fatalf("unknown registered kind persisted durable work: %+v", run)
	default:
	}
}

func TestRegisteredSourceEnumerator_UsesStoredYouTubeKind(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
case "$*" in
  *--no-config*--flat-playlist*--skip-download*--dump-single-json*--playlist-end\ 4*https://www.youtube.com/@retroads/videos) ;;
  *) exit 9 ;;
esac
printf '%s\n' '{"entries":[{"id":"one","webpage_url":"https://www.youtube.com/watch?v=one"}]}'
`)
	items, _, err := (registeredSourceEnumerator{youtube: clipfetch.NewYouTubeEnumerator(ytdlp)}).Enumerate(
		t.Context(), filler.FetchSource{Kind: "youtube", URI: "https://www.youtube.com/@retroads/videos"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "one" || items[0].URL != "https://www.youtube.com/watch?v=one" {
		t.Fatalf("items = %+v, want bounded YouTube result", items)
	}
}

func TestIngest_RefusesAJobThatCannotBePersisted(t *testing.T) {
	a := fillerServiceAdapter{
		fetcher:      successfulClipIngestor{},
		acquisitions: recordingAcquisitions{err: errors.New("database unavailable")},
		newID:        func() string { return "acq-17" },
	}
	if _, err := a.Ingest(t.Context(), []string{"https://archive.org/details/one"}); err == nil {
		t.Fatal("ingest started without durable run state")
	}
}

func TestIngest_IsOwnedByApplicationGenerationNotRequest(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	runs := make(chan filler.AcquisitionRun, 3)
	a := fillerServiceAdapter{
		fetcher:      blockingClipIngestor{started: started, cancelled: cancelled},
		acquisitions: recordingAcquisitions{runs: runs},
		newID:        func() string { return "acq-owned" },
		start:        lifecycle.startInteractiveOperation,
	}

	requestCtx, disconnect := context.WithCancel(t.Context())
	if _, err := a.Ingest(requestCtx, []string{"https://archive.org/details/one"}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if run := <-runs; run.Status != filler.AcquisitionQueued {
		t.Fatalf("first run = %q, want queued", run.Status)
	}
	<-started
	disconnect()
	select {
	case <-cancelled:
		t.Fatal("request disconnect cancelled the accepted acquisition")
	default:
	}

	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	<-cancelled
	if run := <-runs; run.Status != filler.AcquisitionRunning {
		t.Fatalf("second run = %q, want running", run.Status)
	}
	terminal := <-runs
	if terminal.Status != filler.AcquisitionError || terminal.Error == "" || terminal.CompletedAt.IsZero() {
		t.Fatalf("terminal run = %+v, want durable cancellation error", terminal)
	}
}

func TestIngest_RefusesAnEmptyAcquisition(t *testing.T) {
	a := fillerServiceAdapter{fetcher: successfulClipIngestor{}, newID: func() string { return "acq-17" }}
	if _, err := a.Ingest(t.Context(), nil); err == nil {
		t.Fatal("empty ingest created a successful-looking job")
	}
}

func TestIngestPull_PreservesPerTargetSourcesUnderOneRun(t *testing.T) {
	fetched := recordingClipIngestor{sources: make(chan []clipfetch.Source, 1)}
	runs := make(chan filler.AcquisitionRun, 3)
	a := fillerServiceAdapter{
		fetcher: fetched, acquisitions: recordingAcquisitions{runs: runs},
		newID: func() string { return "acq-17" },
		start: testInteractiveOperationLauncher,
	}

	_, err := a.IngestPull(t.Context(), "pull-7", []filler.AcquisitionTarget{
		{SourceID: "classic", Kind: "archive", URL: "https://archive.org/details/one"},
		{SourceID: "holiday", Kind: "youtube", URL: "https://youtube.com/watch?v=two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued := <-runs
	if queued.PullID != "pull-7" || queued.SourceID != "" {
		t.Fatalf("run = %+v, want pull attribution and no false single-source claim", queued)
	}
	select {
	case sources := <-fetched.sources:
		if sources[0].ID != "classic" || sources[1].ID != "holiday" {
			t.Fatalf("sources = %+v, want per-target registered source ids", sources)
		}
		if sources[0].Kind != clipfetch.Archive || sources[1].Kind != clipfetch.YouTube {
			t.Fatalf("sources = %+v, want registered provider kinds preserved", sources)
		}
		if sources[0].AcquisitionID != "acq-17" || sources[1].AcquisitionID != "acq-17" {
			t.Fatalf("sources = %+v, want one acquisition id", sources)
		}
	case <-time.After(time.Second):
		t.Fatal("downloader was not called")
	}
}
