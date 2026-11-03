package filler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/sourceprovider"
	archiveprovider "github.com/loomarr/loomarr/internal/sourceprovider/archive"
	youtubeprovider "github.com/loomarr/loomarr/internal/sourceprovider/youtube"
)

type fetchStub struct {
	sources     []filler.FetchSource
	paths       []string
	offers      []sourceprovider.Item
	queued      []string
	queuedIDs   []string
	queuedGaps  []string
	sourceID    string
	sourceKind  string
	calls       int
	listed      []string
	listedKinds []string
	listLimits  []int
	enumErr     error
	ingestErr   error
	// stamped records which sources were marked fetched, and when.
	stamped      map[string]time.Time
	checked      map[string]time.Time
	activeChecks map[string]bool
	failedChecks map[string]time.Time
	completions  map[string]filler.SourceCheckCompletion
	claimCalls   int
	catalogCalls int
}

func (f *fetchStub) ListFetchSources(context.Context) ([]filler.FetchSource, error) {
	return f.sources, nil
}
func (f *fetchStub) CatalogPaths(context.Context) ([]string, error) {
	f.catalogCalls++
	return f.paths, nil
}

// list is the raw listing the stub's providers read: what the source offers, however its provider
// then walks it.
func (f *fetchStub) list(source filler.FetchSource, limit int) ([]sourceprovider.Item, error) {
	f.calls++
	f.listed = append(f.listed, source.URI)
	f.listedKinds = append(f.listedKinds, source.Kind)
	f.listLimits = append(f.listLimits, limit)
	return f.offers, f.enumErr
}
func (f *fetchStub) IngestSource(_ context.Context, sourceID, sourceKind string, urls []string) (string, error) {
	f.sourceID = sourceID
	f.sourceKind = sourceKind
	if f.ingestErr != nil {
		return "", f.ingestErr
	}
	f.queued = append(f.queued, urls...)
	return "job-1", nil
}
func (f *fetchStub) IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []sourceprovider.Item) (string, error) {
	urls := make([]string, 0, len(items))
	for _, item := range items {
		f.queuedIDs = append(f.queuedIDs, item.ID)
		f.queuedGaps = append(f.queuedGaps, item.Gap)
		urls = append(urls, item.URL)
	}
	return f.IngestSource(ctx, sourceID, sourceKind, urls)
}
func (f *fetchStub) MarkFetched(_ context.Context, id string, at time.Time) error {
	if f.stamped == nil {
		f.stamped = map[string]time.Time{}
	}
	f.stamped[id] = at
	return nil
}

func (f *fetchStub) ClaimCheck(_ context.Context, id string, _ time.Time, _, _ time.Time) (bool, error) {
	f.claimCalls++
	if f.activeChecks == nil {
		f.activeChecks = map[string]bool{}
	}
	if f.activeChecks[id] {
		return false, nil
	}
	f.activeChecks[id] = true
	return true, nil
}

func (f *fetchStub) CompleteCheck(_ context.Context, id string, _ time.Time, completion filler.SourceCheckCompletion) error {
	if f.checked == nil {
		f.checked = map[string]time.Time{}
	}
	f.checked[id] = completion.CheckedAt
	if f.completions == nil {
		f.completions = map[string]filler.SourceCheckCompletion{}
	}
	f.completions[id] = completion
	delete(f.activeChecks, id)
	return nil
}

func (f *fetchStub) FailCheck(_ context.Context, id string, _ time.Time, retryAt time.Time) error {
	if f.failedChecks == nil {
		f.failedChecks = map[string]time.Time{}
	}
	f.failedChecks[id] = retryAt
	delete(f.activeChecks, id)
	return nil
}

func refs(ids ...string) []sourceprovider.Item {
	out := make([]sourceprovider.Item, len(ids))
	for i, id := range ids {
		out[i] = sourceprovider.Item{ID: id, URL: "https://archive.org/details/" + id}
	}
	return out
}

func limits(perRun, catalog int) filler.FetchLimits {
	return filler.FetchLimits{
		MaxPerRun:         func() int { return perRun },
		MaxProviderPerRun: func() int { return 50 },
		MaxCatalogClips:   func() int { return catalog },
		MinDuration:       func() time.Duration { return 10 * time.Second },
		MaxDuration:       func() time.Duration { return 2 * time.Minute },
	}
}

func newFetcher(t *testing.T, stub *fetchStub, l filler.FetchLimits) *filler.Fetcher {
	t.Helper()
	return newFetcherWithRemoteStates(t, stub, l, nil)
}

// fetchStoreWithRemoteStates adds the fetch port's typed-state call without turning the shared
// fetch fixture into a second stateful catalog implementation.
type fetchStoreWithRemoteStates struct {
	*fetchStub
	listRemoteStates func(context.Context) (map[string]filler.ExistingRemoteState, error)
}

func (s fetchStoreWithRemoteStates) ListAcquisitionRemoteStates(ctx context.Context) (map[string]filler.ExistingRemoteState, error) {
	if s.listRemoteStates == nil {
		return nil, nil
	}
	return s.listRemoteStates(ctx)
}

func newFetcherWithRemoteStates(t *testing.T, stub *fetchStub, l filler.FetchLimits, states map[string]filler.ExistingRemoteState) *filler.Fetcher {
	t.Helper()
	return filler.NewFetcher(fetchStoreWithRemoteStates{
		fetchStub: stub,
		listRemoteStates: func(context.Context) (map[string]filler.ExistingRemoteState, error) {
			return states, nil
		},
	}, providersOver(t, stub.list, nil), stub, l, discardLog())
}

// providersOver is the registry the fetch tests run on: the real YouTube and archive.org
// providers reading what list returns for each source, and the scanned folder and library beside
// them. list receives only what the provider's own call carries, so a source is identified by its
// Kind and URI. search is nil for a catalog that cannot search.
func providersOver(
	t *testing.T,
	list func(source filler.FetchSource, limit int) ([]sourceprovider.Item, error),
	search func(source filler.FetchSource, query string, limit int) ([]sourceprovider.Item, error),
) *sourceprovider.Registry {
	t.Helper()
	providers, err := sourceprovider.NewRegistry(
		youtubeprovider.New(func(_ context.Context, target string, limit int) ([]sourceprovider.Item, int, error) {
			items, err := list(filler.FetchSource{Kind: "youtube", URI: target}, limit)
			return items, len(items), err
		}),
		archiveprovider.New(archiveprovider.Config{
			Catalog: archiveCatalogOver{list: list, search: search}, Log: discardLog(),
		}),
		sourceprovider.Scanned("folder", "Folder", ""),
		sourceprovider.Scanned("library", "Media library", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	return providers
}

// archiveCatalogOver is an archive.org catalog over listings of provider items. archive.org lists
// an item by its id, so an item's URL is its details page.
type archiveCatalogOver struct {
	list   func(source filler.FetchSource, limit int) ([]sourceprovider.Item, error)
	search func(source filler.FetchSource, query string, limit int) ([]sourceprovider.Item, error)
}

func discovered(items []sourceprovider.Item) clipfetch.DiscoveryResult {
	res := clipfetch.DiscoveryResult{Total: len(items)}
	for _, item := range items {
		res.Items = append(res.Items, clipfetch.DiscoveredItem{
			ID: item.ID, Title: item.Title, License: item.License, Year: item.ObservedYear,
			Date: item.PublishedAt, DurationMS: item.DurationMS, Height: item.Height,
		})
	}
	return res
}

func (c archiveCatalogOver) EnumerateCollection(_ context.Context, ref string, limit int) (clipfetch.DiscoveryResult, error) {
	items, err := c.list(filler.FetchSource{Kind: "archive", URI: ref}, limit)
	return discovered(items), err
}

func (c archiveCatalogOver) SearchCollection(_ context.Context, ref, query string, limit int) (clipfetch.DiscoveryResult, error) {
	if c.search == nil {
		return clipfetch.DiscoveryResult{}, nil
	}
	items, err := c.search(filler.FetchSource{Kind: "archive", URI: ref}, query, limit)
	return discovered(items), err
}

func (archiveCatalogOver) Enrich(context.Context, []clipfetch.DiscoveredItem) {}

type sourceEnum func(filler.FetchSource) []sourceprovider.Item

func (e sourceEnum) list(source filler.FetchSource, _ int) ([]sourceprovider.Item, error) {
	return e(source), nil
}

func youtubeRef(id string, duration time.Duration) sourceprovider.Item {
	return sourceprovider.Item{
		ID: id, URL: "https://youtube.com/watch?v=" + id,
		DurationMS: int(duration.Milliseconds()), DurationKnown: true,
	}
}

func TestFetch_YouTubeRejectsUnusableMediaBeforeQueueing(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "youtube:bounded", Kind: "youtube", URI: "https://youtube.com/@bounded/videos", Enabled: true,
		}},
		offers: []sourceprovider.Item{
			youtubeRef("eligible", 30*time.Second),
			youtubeRef("short", 9*time.Second),
			youtubeRef("long", 121*time.Second),
			{ID: "unknown", URL: "https://youtube.com/watch?v=unknown"},
			{ID: "live", URL: "https://youtube.com/watch?v=live", DurationKnown: true, DurationMS: 30_000, LiveStatus: "is_live"},
			{ID: "upcoming", URL: "https://youtube.com/watch?v=upcoming", DurationKnown: true, DurationMS: 30_000, LiveStatus: "is_upcoming"},
			{ID: "private", URL: "https://youtube.com/watch?v=private", DurationKnown: true, DurationMS: 30_000, Availability: "private"},
			{ID: "unavailable", URL: "https://youtube.com/watch?v=unavailable", DurationKnown: true, DurationMS: 30_000, Availability: "needs_auth"},
			{ID: "incomplete", DurationKnown: true, DurationMS: 30_000},
		},
	}

	res, err := newFetcher(t, stub, limits(10, 2000)).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 1 || !reflect.DeepEqual(stub.queuedIDs, []string{"eligible"}) {
		t.Fatalf("queued = %d/%v, want only the eligible short video", res.Queued, stub.queuedIDs)
	}
	want := sourceprovider.Outcomes{
		sourceprovider.OutcomeQueued: 1, sourceprovider.OutcomeTooShort: 1,
		sourceprovider.OutcomeTooLong: 1, sourceprovider.OutcomeMetadataIncomplete: 2,
		sourceprovider.OutcomeLive: 1, sourceprovider.OutcomeUpcoming: 1,
		sourceprovider.OutcomePrivate: 1, sourceprovider.OutcomeUnavailable: 1,
	}
	if got := stub.completions["youtube:bounded"].Outcomes; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %#v, want %#v", got, want)
	}
}

func TestFetch_YouTubeResumesPastTheLastExaminedItemAcrossChecks(t *testing.T) {
	const sourceID = "youtube:resume"
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: sourceID, Kind: "youtube", URI: "https://youtube.com/@resume/videos", Enabled: true,
		}},
		offers: []sourceprovider.Item{
			youtubeRef("video-1", 30*time.Second), youtubeRef("video-2", 30*time.Second),
			youtubeRef("video-3", 30*time.Second), youtubeRef("video-4", 30*time.Second),
		},
	}
	states := map[string]filler.ExistingRemoteState{}
	fetcher := newFetcherWithRemoteStates(t, stub, limits(2, 2000), states)

	first, err := fetcher.RunSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	firstCompletion := stub.completions[sourceID]
	if first.Queued != 2 || !reflect.DeepEqual(stub.queuedIDs, []string{"video-1", "video-2"}) ||
		firstCompletion.Checkpoint != (sourceprovider.Checkpoint{Cursor: "video-2", PendingWatermark: "video-1"}) ||
		!firstCompletion.ReplaceOutcomes || len(stub.listLimits) != 1 || stub.listLimits[0] != youtubeprovider.InitialLookback {
		t.Fatalf("first pass = %+v, ids=%v, completion=%+v", first, stub.queuedIDs, firstCompletion)
	}
	for _, id := range stub.queuedIDs {
		identity := filler.RemoteIdentity{Provider: "youtube", SourceID: sourceID, RemoteID: id}
		states[identity.Key()] = filler.RemoteQueued
	}
	stub.sources[0].ScanCheckpoint = firstCompletion.Checkpoint
	stub.queuedIDs = nil

	second, err := fetcher.RunSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	secondCompletion := stub.completions[sourceID]
	if second.Queued != 2 || !reflect.DeepEqual(stub.queuedIDs, []string{"video-3", "video-4"}) ||
		secondCompletion.Checkpoint != (sourceprovider.Checkpoint{Watermark: "video-1"}) ||
		secondCompletion.ReplaceOutcomes {
		t.Fatalf("second pass = %+v, ids=%v, completion=%+v", second, stub.queuedIDs, secondCompletion)
	}
	for _, id := range stub.queuedIDs {
		identity := filler.RemoteIdentity{Provider: "youtube", SourceID: sourceID, RemoteID: id}
		states[identity.Key()] = filler.RemoteQueued
	}
	stub.sources[0].ScanCheckpoint = secondCompletion.Checkpoint
	stub.offers = append([]sourceprovider.Item{youtubeRef("new-video", 30*time.Second)}, stub.offers...)
	stub.queuedIDs = nil

	third, err := fetcher.RunSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	thirdCompletion := stub.completions[sourceID]
	if third.Queued != 1 || !reflect.DeepEqual(stub.queuedIDs, []string{"new-video"}) ||
		thirdCompletion.Checkpoint != (sourceprovider.Checkpoint{Watermark: "new-video"}) ||
		!thirdCompletion.ReplaceOutcomes {
		t.Fatalf("refresh pass = %+v, ids=%v, completion=%+v", third, stub.queuedIDs, thirdCompletion)
	}
}

func TestFetch_ScheduledPassCapsAllSourcesFromOneProvider(t *testing.T) {
	stub := &fetchStub{sources: []filler.FetchSource{
		{ID: "youtube:one", Kind: "youtube", URI: "one", Enabled: true, MaxPerRun: 3},
		{ID: "youtube:two", Kind: "youtube", URI: "two", Enabled: true, MaxPerRun: 3},
		{ID: "youtube:three", Kind: "youtube", URI: "three", Enabled: true, MaxPerRun: 3},
	}}
	enum := sourceEnum(func(source filler.FetchSource) []sourceprovider.Item {
		return []sourceprovider.Item{
			youtubeRef(source.URI+":1", 30*time.Second),
			youtubeRef(source.URI+":2", 30*time.Second),
			youtubeRef(source.URI+":3", 30*time.Second),
		}
	})
	l := limits(3, 2000)
	l.MaxProviderPerRun = func() int { return 5 }
	fetcher := filler.NewFetcher(fetchStoreWithRemoteStates{fetchStub: stub}, providersOver(t, enum.list, nil), stub, l, discardLog())

	res, err := fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 5 || res.SourcesPolled != 2 || res.StoppedBy != "provider" {
		t.Fatalf("result = %+v, want five queued from two sources and a reported provider stop", res)
	}
	if _, checked := stub.checked["youtube:three"]; checked {
		t.Fatal("third YouTube source was checked after the provider pass budget was exhausted")
	}
}

func TestFetch_YouTubePrefersDifferentCampaignsOverShortAndLongCuts(t *testing.T) {
	short := youtubeRef("qb-15", 15*time.Second)
	short.Title = "HH X GFL QB Rory :15 sec. spot"
	long := youtubeRef("qb-30", 30*time.Second)
	long.Title = "HH X GFL QB Rory :30 sec. spot"
	other := youtubeRef("hot-takes", 30*time.Second)
	other.Title = "Hot Takes - 30 Sec"
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "youtube:hoagie-hut", Kind: "youtube", URI: "https://youtube.com/@hoagiehut/videos", Enabled: true,
		}},
		offers: []sourceprovider.Item{short, long, other},
	}

	res, err := newFetcher(t, stub, limits(2, 2000)).RunSource(t.Context(), "youtube:hoagie-hut")
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 || !reflect.DeepEqual(stub.queuedIDs, []string{"hot-takes", "qb-30"}) {
		t.Fatalf("queued = %v, want the fuller QB cut and a different campaign", stub.queuedIDs)
	}
}

func TestFetch_YouTubeDurationVariantsDoNotReturnOnTheNextCheck(t *testing.T) {
	const sourceID = "youtube:hoagie-hut"
	short := youtubeRef("qb-15", 15*time.Second)
	short.Title = "HH X GFL QB Rory :15 sec. spot"
	long := youtubeRef("qb-30", 30*time.Second)
	long.Title = "HH X GFL QB Rory :30 sec. spot"
	firstOther := youtubeRef("hot-takes", 30*time.Second)
	firstOther.Title = "Hot Takes - 30 Sec"
	secondOther := youtubeRef("remember", 30*time.Second)
	secondOther.Title = "Remember Get It Before Its Gone - 30 Sec"
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: sourceID, Kind: "youtube", URI: "https://youtube.com/@hoagiehut/videos", Enabled: true,
		}},
		offers: []sourceprovider.Item{short, firstOther, secondOther, long},
	}
	states := map[string]filler.ExistingRemoteState{}
	fetcher := newFetcherWithRemoteStates(t, stub, limits(2, 2000), states)

	first, err := fetcher.RunSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	completion := stub.completions[sourceID]
	for _, id := range stub.queuedIDs {
		states[(filler.RemoteIdentity{Provider: "youtube", SourceID: sourceID, RemoteID: id}).Key()] = filler.RemoteQueued
	}
	stub.sources[0].ScanCheckpoint = completion.Checkpoint
	stub.queuedIDs = nil

	second, err := fetcher.RunSource(t.Context(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Queued != 2 || second.Queued != 1 || !reflect.DeepEqual(stub.queuedIDs, []string{"qb-30"}) {
		t.Fatalf("checks queued %d then %d/%v, want two different campaigns then only the fuller QB cut",
			first.Queued, second.Queued, stub.queuedIDs)
	}
}

func TestFetch_ManualCheckRetainsTheProviderPassCap(t *testing.T) {
	items := make([]sourceprovider.Item, 60)
	for i := range items {
		items[i] = youtubeRef(fmt.Sprintf("video-%02d", i), 30*time.Second)
	}
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "youtube:manual-cap", Kind: "youtube", URI: "manual-cap", Enabled: true, MaxPerRun: 100,
		}},
		offers: items,
	}

	res, err := newFetcher(t, stub, limits(100, 2000)).RunSource(t.Context(), "youtube:manual-cap")
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 50 || res.MaxPerCheck != 50 || res.StoppedBy != "provider" {
		t.Fatalf("manual result = %+v, want effective provider-bounded cap of 50", res)
	}
}

func TestFetch_QueueFailureDoesNotAdvanceTheYouTubeCheckpoint(t *testing.T) {
	now := time.Date(2026, 9, 19, 15, 0, 0, 0, time.UTC)
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "youtube:retry", Kind: "youtube", URI: "retry", Enabled: true,
			ScanCheckpoint: sourceprovider.Checkpoint{Watermark: "prior"},
		}},
		offers:    []sourceprovider.Item{youtubeRef("new-video", 30*time.Second)},
		ingestErr: errors.New("temporary queue failure"),
	}

	_, err := newFetcher(t, stub, limits(2, 2000)).WithClock(func() time.Time { return now }).RunSource(t.Context(), "youtube:retry")
	if err == nil {
		t.Fatal("manual check hid the queue failure")
	}
	if _, completed := stub.completions["youtube:retry"]; completed {
		t.Fatal("failed queue advanced the source checkpoint")
	}
	if got := stub.failedChecks["youtube:retry"]; !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("retry = %v, want %v", got, now.Add(time.Minute))
	}
}

// ⚠ THE bound that makes auto-fetch safe to enable by default: an archive.org collection is
// thousands of items, and without this "add a source" means "download all of it tonight".
func TestFetch_StopsAtMaxPerRun(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers:  refs("a", "b", "c", "d", "e", "f", "g", "h"),
	}
	res, err := newFetcher(t, stub, limits(3, 2000)).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 3 || len(stub.queued) != 3 {
		t.Fatalf("queued %d (%v), want 3 — max_per_run is what stops a collection arriving at once",
			res.Queued, stub.queued)
	}
	if stub.sourceID != "s1" {
		t.Errorf("queued source id = %q, want s1 — admission provenance was dropped", stub.sourceID)
	}
	if len(stub.queuedIDs) != 3 {
		t.Fatalf("queued remote ids = %v, want exact identities retained", stub.queuedIDs)
	}
}

// #1773: until automatic splitting is certified, a scheduled pass leaves compilation reels (known
// to run longer than the longest single clip) where they are, and takes them once the setting is
// on. An item of unknown length can't be judged before download, so it is still taken; probe
// marks it a compilation if it is one.
func TestFetch_CompilationsWaitUntilTheSettingTakesThem(t *testing.T) {
	offers := []sourceprovider.Item{
		{ID: "reel", URL: "https://archive.org/details/reel", DurationMS: 30 * 60 * 1000},
		{ID: "spot", URL: "https://archive.org/details/spot", DurationMS: 45 * 1000},
		{ID: "unknown", URL: "https://archive.org/details/unknown"},
	}
	for _, tc := range []struct {
		take bool
		want []string
	}{
		{take: false, want: []string{"spot", "unknown"}},
		{take: true, want: []string{"reel", "spot", "unknown"}},
	} {
		stub := &fetchStub{
			sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
			offers:  offers,
		}
		l := limits(10, 2000)
		take := tc.take
		l.Compilations = filler.CompilationGate{
			Take: func() bool { return take },
			Over: func() time.Duration { return 2 * time.Minute },
		}
		if _, err := newFetcher(t, stub, l).Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		got := append([]string(nil), stub.queuedIDs...)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("compilations taken=%v: queued %v, want %v", tc.take, got, tc.want)
		}
		if !tc.take {
			if n := stub.completions["s1"].Outcomes[sourceprovider.OutcomeTooLong]; n != 1 {
				t.Errorf("compilations off: too_long outcomes = %d, want the reel counted once", n)
			}
		}
	}
}

// The pass result explains what every source it checked saw, summed: it is what a manual run
// reports, so a source's outcomes stored but not added to the pass would leave the operator with
// "nothing happened".
func TestFetch_ResultSumsEachSourcesOutcomes(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "s1", Kind: "archive", URI: "coll-1", Enabled: true},
			{ID: "s2", Kind: "archive", URI: "coll-2", Enabled: true},
		},
		offers: []sourceprovider.Item{
			{ID: "reel", URL: "https://archive.org/details/reel", DurationMS: 30 * 60 * 1000},
			{ID: "spot", URL: "https://archive.org/details/spot", DurationMS: 45 * 1000},
		},
	}
	l := limits(10, 2000)
	l.Compilations = filler.CompilationGate{
		Take: func() bool { return false },
		Over: func() time.Duration { return 2 * time.Minute },
	}
	res, err := newFetcher(t, stub, l).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := sourceprovider.Outcomes{sourceprovider.OutcomeTooLong: 2, sourceprovider.OutcomeQueued: 2}
	if !maps.Equal(res.Outcomes, want) {
		t.Fatalf("pass outcomes = %v, want %v — one reel held back and one spot queued per source", res.Outcomes, want)
	}
}

// Choosing one duration cut per campaign is YouTube's: its cursor can split one listing across
// checks. Another provider has no preferred cut, so a title that names its length ("30 second
// spot") is an ordinary item, not one already known. Until this was scoped, every such archive.org
// item was skipped as already_known, and never fetched (#1375 to here).
func TestFetch_ANonYouTubeTitleThatNamesItsLengthIsStillTaken(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers: []sourceprovider.Item{
			{ID: "cola", URL: "https://archive.org/details/cola", Title: "Acme Cola 30 second spot", DurationMS: 30 * 1000},
			{ID: "plain", URL: "https://archive.org/details/plain", Title: "Acme Cola spot", DurationMS: 30 * 1000},
		},
	}
	res, err := newFetcher(t, stub, limits(10, 2000)).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), stub.queuedIDs...)
	sort.Strings(got)
	if strings.Join(got, ",") != "cola,plain" || res.Skipped != 0 {
		t.Fatalf("queued %v with %d skipped, want cola and plain and nothing skipped", got, res.Skipped)
	}
}

func TestFetch_RanksMetadataInsteadOfTakingProviderOrder(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers: []sourceprovider.Item{
			{ID: "first-low", URL: "https://archive.org/details/first-low", Height: 240},
			{ID: "second-hd", URL: "https://archive.org/details/second-hd", Height: 1080, License: "cc-by"},
		},
	}
	res, err := newFetcher(t, stub, limits(1, 2000)).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 1 || len(stub.queuedIDs) != 1 || stub.queuedIDs[0] != "second-hd" {
		t.Fatalf("scheduled selection queued %v, want the declared-rights HD item", stub.queuedIDs)
	}
}

func TestFetch_EnumeratesTheRegisteredProviderKind(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "youtube:kids", Kind: "youtube", URI: "https://youtube.com/@kids/videos", Enabled: true}},
		offers:  []sourceprovider.Item{youtubeRef("video-1", 30*time.Second)},
	}
	if _, err := newFetcher(t, stub, limits(2, 2000)).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(stub.listedKinds) != 1 || stub.listedKinds[0] != "youtube" {
		t.Fatalf("enumerated kinds = %v, want the registered YouTube lane", stub.listedKinds)
	}
	if stub.sourceKind != "youtube" {
		t.Fatalf("queued source kind = %q, want the registered YouTube kind", stub.sourceKind)
	}
}

func TestFetch_ScheduledSelectionKeepsProviderNamespacesDistinct(t *testing.T) {
	stub := &fetchStub{sources: []filler.FetchSource{
		{ID: "archive:classic", Kind: "archive", URI: "https://archive.org/details/classic", Enabled: true},
		{ID: "youtube:classic", Kind: "youtube", URI: "https://youtube.com/@classic/videos", Enabled: true},
	}}
	enum := sourceEnum(func(source filler.FetchSource) []sourceprovider.Item {
		if source.Kind == "youtube" {
			item := youtubeRef("abcdef12345", 30*time.Second)
			item.Height = 1080
			return []sourceprovider.Item{item}
		}
		return []sourceprovider.Item{{ID: "abcdef12345", URL: "https://archive.org/details/abcdef12345", Height: 1080}}
	})
	f := filler.NewFetcher(fetchStoreWithRemoteStates{fetchStub: stub}, providersOver(t, enum.list, nil), stub, limits(1, 2000), discardLog())
	res, err := f.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 || len(stub.queued) != 2 {
		t.Fatalf("queued=%d urls=%v, want both provider-specific candidates", res.Queued, stub.queued)
	}
}

func TestFetch_PreservesCaseSensitiveYouTubeItemIdentity(t *testing.T) {
	upperRef, lowerRef := youtubeRef("AbCd123", 30*time.Second), youtubeRef("abcd123", 30*time.Second)
	upperRef.Title, lowerRef.Title = "Upper title", "Lower title"
	stub := &fetchStub{sources: []filler.FetchSource{{ID: "youtube:case", Kind: "youtube", URI: "https://youtube.com/@case/videos", Enabled: true}}, offers: []sourceprovider.Item{upperRef, lowerRef}}
	res, err := newFetcher(t, stub, limits(2, 2000)).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 || !reflect.DeepEqual(stub.queuedIDs, []string{"AbCd123", "abcd123"}) {
		t.Fatalf("queued=%d ids=%v, want both case-sensitive IDs", res.Queued, stub.queuedIDs)
	}
}

func TestFetch_TypedRemoteStatesExcludeOnlyExactIdentity(t *testing.T) {
	upper := filler.RemoteIdentity{Provider: "youtube", SourceID: "youtube:case", RemoteID: "AbCd123"}
	stub := &fetchStub{sources: []filler.FetchSource{{ID: upper.SourceID, Kind: upper.Provider, URI: "https://youtube.com/@case/videos", Enabled: true}}, offers: []sourceprovider.Item{youtubeRef(upper.RemoteID, 30*time.Second), youtubeRef("abcd123", 30*time.Second)}}
	res, err := newFetcherWithRemoteStates(t, stub, limits(2, 2000), map[string]filler.ExistingRemoteState{upper.Key(): filler.RemoteCatalogued}).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || len(stub.queued) != 1 || stub.queued[0] != "https://youtube.com/watch?v=abcd123" {
		t.Fatalf("result=%+v queued=%v, want only unrelated identity URL", res, stub.queued)
	}
}

func TestFetch_FailedQueueLeavesIdentityEligibleForHealthyRetry(t *testing.T) {
	states := map[string]filler.ExistingRemoteState{}
	stub := &fetchStub{sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}}, offers: refs("retry"), ingestErr: errors.New("temporary")}
	f := newFetcherWithRemoteStates(t, stub, limits(2, 2000), states)
	if _, err := f.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Fatalf("states mutated after failed queue: %v", states)
	}
	stub.ingestErr = nil
	res, err := f.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 1 || len(stub.queued) != 1 {
		t.Fatalf("retry result=%+v queued=%v, want healthy retry", res, stub.queued)
	}
}

// A disabled source is not polled. The Sources switch claims Loomarr "stops scanning, searching
// and downloading" from it; auto-fetch honouring anything less makes that copy false.
func TestFetch_SkipsDisabledSources(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: false}},
		offers:  refs("a", "b"),
	}
	res, _ := newFetcher(t, stub, limits(10, 2000)).Run(context.Background())
	if res.SourcesPolled != 0 || len(stub.queued) != 0 {
		t.Errorf("a switched-off source was polled (%d) and queued %v", res.SourcesPolled, stub.queued)
	}
	if stub.calls != 0 {
		t.Error("a switched-off source was still listed upstream — the switch must stop the request")
	}
}

func TestFetch_ManualCheckRefusesADisabledSource(t *testing.T) {
	stub := &fetchStub{sources: []filler.FetchSource{{
		ID: "off", Kind: "archive", URI: "collection", Enabled: false,
	}}}

	_, err := newFetcher(t, stub, limits(10, 2000)).RunSource(t.Context(), "off")
	if !errors.Is(err, filler.ErrSourceDisabled) || stub.catalogCalls != 0 || stub.calls != 0 {
		t.Fatalf("disabled manual check = %v, catalog/provider calls = %d/%d; want refusal before work",
			err, stub.catalogCalls, stub.calls)
	}
}

// The config-backed rows are SCANNED, not fetched. They have no URI, and polling them would be a
// request to nowhere.
func TestFetch_SkipsFolderAndLibraryRows(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "folder", Kind: "folder", URI: "", Enabled: true},
			{ID: "library", Kind: "library", URI: "", Enabled: true},
		},
		offers: refs("a"),
	}
	res, _ := newFetcher(t, stub, limits(10, 2000)).Run(context.Background())
	if res.SourcesPolled != 0 || len(stub.queued) != 0 {
		t.Error("a config-backed row was polled — those are scanned, not downloaded from")
	}
}

// A source whose provider is not registered is inert: a scheduled pass skips it, says so once, and
// leaves the row to be fetched again when the provider is back. A scanned provider's source, even
// one with an address, is never fetched either, and a manual check of either polls nothing.
func TestFetch_ASourceWhoseProviderIsNotFetchedIsInert(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "vimeo-src", Kind: "vimeo", URI: "https://vimeo.com/channels/x", Enabled: true},
			{ID: "scanned-src", Kind: "folder", URI: "/media/clips", Enabled: true},
			{ID: "s1", Kind: "archive", URI: "coll", Enabled: true},
		},
		offers: refs("a"),
	}
	var logged bytes.Buffer
	fetcher := filler.NewFetcher(fetchStoreWithRemoteStates{fetchStub: stub}, providersOver(t, stub.list, nil), stub, limits(10, 2000),
		slog.New(slog.NewTextHandler(&logged, nil)))

	for range 2 {
		res, err := fetcher.Run(t.Context())
		if err != nil {
			t.Fatalf("a scheduled pass failed on an inert source: %v", err)
		}
		if res.SourcesPolled != 1 || res.Queued != 1 {
			t.Fatalf("pass = %+v, want only the archive source polled", res)
		}
		delete(stub.checked, "s1")
	}
	if stub.claimCalls != 2 || !reflect.DeepEqual(stub.listed, []string{"coll", "coll"}) {
		t.Fatalf("claims=%d listed=%v, want only the archive source claimed and listed, once per pass", stub.claimCalls, stub.listed)
	}
	if n := strings.Count(logged.String(), "provider is not registered or not fetched"); n != 1 {
		t.Fatalf("logged %d inert reports over two passes, want one:\n%s", n, logged.String())
	}

	for _, id := range []string{"vimeo-src", "scanned-src"} {
		if res, err := fetcher.RunSource(t.Context(), id); err != nil || res.SourcesPolled != 0 {
			t.Errorf("manual check of %s = %+v, %v, want nothing polled and no error", id, res, err)
		}
	}
	if stub.claimCalls != 2 {
		t.Fatalf("a manual check of an inert source claimed it (%d claims)", stub.claimCalls)
	}
	if status, err := fetcher.Status(t.Context()); err != nil || !status.Enabled {
		t.Fatalf("status = %+v, %v, want fetching on: the archive source is fetched", status, err)
	}
}

// A wake whose only enabled sources are inert reads source state and nothing else: it neither
// claims them nor walks the catalog, so an unregistered provider costs no more than an idle wake.
func TestFetch_AWakeWithOnlyInertSourcesStaysCheap(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "vimeo-src", Kind: "vimeo", URI: "https://vimeo.com/channels/x", Enabled: true},
			{ID: "scanned-src", Kind: "folder", URI: "/media/clips", Enabled: true},
		},
	}
	fetcher := filler.NewFetcher(fetchStoreWithRemoteStates{fetchStub: stub}, providersOver(t, stub.list, nil), stub,
		limits(10, 2000), nil)
	if _, err := fetcher.Run(t.Context()); err != nil {
		t.Fatalf("pass: %v", err)
	}
	if stub.catalogCalls != 0 || stub.claimCalls != 0 {
		t.Fatalf("catalog reads=%d claims=%d, want neither for a wake with only inert sources",
			stub.catalogCalls, stub.claimCalls)
	}
}

// ⚠ Without dedupe the job re-downloads its own output on every pass, forever. The typed remote
// state is the high-water mark; paths still count toward the catalog ceiling but cannot prove
// provider/source identity.
func TestFetch_SkipsWhatIsAlreadyInTheCatalog(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		paths:   []string{"a.mp4", "nested/b.mp4"},
		offers:  refs("a", "b", "c"),
	}
	res, _ := newFetcherWithRemoteStates(t, stub, limits(10, 2000), map[string]filler.ExistingRemoteState{
		(filler.RemoteIdentity{Provider: "archive", SourceID: "s1", RemoteID: "a"}).Key(): filler.RemoteCatalogued,
		(filler.RemoteIdentity{Provider: "archive", SourceID: "s1", RemoteID: "b"}).Key(): filler.RemoteCatalogued,
	}).Run(context.Background())
	if res.Skipped != 2 {
		t.Errorf("skipped %d, want 2 — a catalogued clip must be recognised as the item it came from", res.Skipped)
	}
	if len(stub.queued) != 1 || stub.queued[0] != "https://archive.org/details/c" {
		t.Errorf("queued %v, want only the new item", stub.queued)
	}
}

// A catalogued artifact remains excluded even when its stored path uses Archive's output template.
func TestFetch_SkipsCataloguedArchiveOutputTemplatePath(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		paths:   []string{"HearthpotsSoupAdvert - Hearthpot's Soup Advert 1993.mp4"},
		offers:  refs("HearthpotsSoupAdvert", "new-ad"),
	}
	res, err := newFetcherWithRemoteStates(t, stub, limits(10, 2000), map[string]filler.ExistingRemoteState{
		(filler.RemoteIdentity{Provider: "archive", SourceID: "s1", RemoteID: "HearthpotsSoupAdvert"}).Key(): filler.RemoteCatalogued,
	}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 {
		t.Fatalf("skipped %d, want catalogued Archive item skipped", res.Skipped)
	}
	if len(stub.queued) != 1 || stub.queued[0] != "https://archive.org/details/new-ad" {
		t.Fatalf("queued = %v, want only the new Archive item", stub.queued)
	}
}

func TestFetch_DoesNotTreatAnOrdinaryNameAsArchiveOutput(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		paths:   []string{"An ordinary catalog name - not an Archive item ID.mp4"},
		offers:  refs("An ordinary catalog name"),
	}
	res, err := newFetcher(t, stub, limits(10, 2000)).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 0 || len(stub.queued) != 1 {
		t.Fatalf("result = %+v; queued = %v, want ordinary name left unmatched", res, stub.queued)
	}
}

// A catalogued artifact remains excluded even when its stored path uses yt-dlp's output template.
func TestFetch_SkipsCataloguedYouTubeOutputTemplatePath(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "youtube:retro", Kind: "youtube", URI: "https://youtube.com/@retro/videos", Enabled: true}},
		paths:   []string{"Title for a catalogued clip [video-id].mp4"},
		offers:  []sourceprovider.Item{youtubeRef("video-id", 30*time.Second)},
	}
	res, err := newFetcherWithRemoteStates(t, stub, limits(10, 2000), map[string]filler.ExistingRemoteState{
		(filler.RemoteIdentity{Provider: "youtube", SourceID: "youtube:retro", RemoteID: "video-id"}).Key(): filler.RemoteCatalogued,
	}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || len(stub.queued) != 0 {
		t.Fatalf("result = %+v; queued = %v, want catalogued YouTube video skipped", res, stub.queued)
	}
	if _, stamped := stub.stamped["youtube:retro"]; stamped {
		t.Fatal("stamped a source whose catalogued YouTube video was not queued")
	}
}

// The catalog ceiling stops the pass and SAYS SO. An operator whose catalog stopped growing must
// be able to see which limit stopped it (§10) — a crawler that quietly does nothing is
// indistinguishable from one that is broken.
func TestFetch_StopsAndReportsAtTheCatalogCeiling(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		paths:   []string{"x.mp4", "y.mp4", "z.mp4"},
		offers:  refs("a"),
	}
	res, _ := newFetcher(t, stub, limits(10, 3)).Run(context.Background())
	if res.StoppedBy != "catalog" {
		t.Errorf("StoppedBy = %q, want catalog", res.StoppedBy)
	}
	if len(stub.queued) != 0 {
		t.Errorf("queued %v at the ceiling", stub.queued)
	}
}

func TestFetch_ManualCheckReportsItsCapWhenCapacityStopsIt(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "selected", Kind: "archive", URI: "collection", Enabled: true, MaxPerRun: 3,
		}},
		paths: []string{"a", "b"},
	}

	res, err := newFetcher(t, stub, limits(10, 2)).RunSource(t.Context(), "selected")
	if err != nil {
		t.Fatal(err)
	}
	if res.StoppedBy != "catalog" || res.MaxPerCheck != 3 || stub.calls != 0 {
		t.Fatalf("manual capacity result = %+v, provider calls = %d; want catalog stop with source cap 3",
			res, stub.calls)
	}
}

func TestFetchStatus_ReportsTheLiveLimitWithoutRunningAFetch(t *testing.T) {
	stub := &fetchStub{
		paths:   []string{"x.mp4", "y.mp4", "z.mp4"},
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
	}
	f := newFetcher(t, stub, limits(10, 3))
	status, err := f.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.StoppedBy != "catalog" || status.CatalogClips != 3 || status.MaxCatalog != 3 {
		t.Errorf("status = %+v, want enabled catalog ceiling 3/3", status)
	}
	if stub.calls != 0 || len(stub.queued) != 0 {
		t.Error("status check performed fetch work")
	}

	stub.sources[0].NeverFetch = true
	status, err = f.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.StoppedBy != "" {
		t.Errorf("disabled status = %+v, want no active stop reason", status)
	}
}

// ⚠ A polled source must be STAMPED, or the Sources tab reads "never fetched" forever while
// auto-fetch downloads from it every six hours — a row describing a source nobody has touched,
// on an install actively using it. `MarkFillerSourceFetched` shipped in V33 with no production
// caller, which is how it stayed easy to forget.
func TestFetch_StampsASourceItQueuedFrom(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers:  refs("a"),
	}
	at := time.Unix(1_800_000_000, 0).UTC()
	f := newFetcher(t, stub, limits(10, 2000)).WithClock(func() time.Time { return at })
	if _, err := f.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, ok := stub.stamped["s1"]; !ok || !got.Equal(at) {
		t.Errorf("stamped = %v (present=%v), want %v", got, ok, at)
	}
}

// ⚠ ...but only when something was ACTUALLY queued. "Last fetched" must mean "last brought
// something in": a source polled fruitlessly for a week would otherwise read as freshly
// productive, which is the opposite of what the timestamp is for.
func TestFetch_DoesNotStampASourceThatBroughtNothingIn(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		paths:   []string{"a.mp4"},
		offers:  refs("a"),
	}
	states := map[string]filler.ExistingRemoteState{
		(filler.RemoteIdentity{Provider: "archive", SourceID: "s1", RemoteID: "a"}).Key(): filler.RemoteCatalogued,
	}
	if _, err := newFetcherWithRemoteStates(t, stub, limits(10, 2000), states).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := stub.stamped["s1"]; ok {
		t.Error("stamped a source that queued nothing — the row would claim a productive fetch")
	}
}

// ⚠ A source may opt OUT of unattended fetching while staying ON (§10 V38c). The two are
// deliberately different: an enabled source is still searched and its clips still count, and
// collapsing them would make "stop auto-downloading from this one" require switching it off
// entirely — which also stops search.
func TestFetch_SkipsASourceThatOptedOutButStaysEnabled(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "opted-out", Kind: "archive", URI: "a", Enabled: true, NeverFetch: true},
			{ID: "normal", Kind: "archive", URI: "b", Enabled: true},
		},
		offers: refs("x"),
	}
	res, _ := newFetcher(t, stub, limits(10, 2000)).Run(context.Background())
	if res.SourcesPolled != 1 {
		t.Errorf("polled %d sources, want 1 — the opted-out source must be skipped", res.SourcesPolled)
	}
	if _, ok := stub.stamped["opted-out"]; ok {
		t.Error("an opted-out source was fetched from")
	}
}

// A source's own per-run cap beats the global. A busy collection and a small playlist want
// different numbers, which one figure served badly.
func TestFetch_PrefersASourcesOwnPerRunCap(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true, MaxPerRun: 2}},
		offers:  refs("a", "b", "c", "d", "e"),
	}
	// The global says 10; this source says 2.
	res, _ := newFetcher(t, stub, limits(10, 2000)).Run(context.Background())
	if res.Queued != 2 {
		t.Errorf("queued %d, want 2 — the source's own cap must beat the global 10", res.Queued)
	}
}

// ...and an unset override falls back to the global rather than to zero. ⚠ The failure this
// guards is a source with no override silently fetching NOTHING, which looks identical to a
// working install whose sources have all run dry.
func TestFetch_FallsBackToTheGlobalCapWhenUnset(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers:  refs("a", "b", "c", "d", "e"),
	}
	res, _ := newFetcher(t, stub, limits(3, 2000)).Run(context.Background())
	if res.Queued != 3 {
		t.Errorf("queued %d, want the global 3 — an unset override must inherit, not zero out", res.Queued)
	}
}

// A globally inherited `filler.fetch.every = 0` resolves each inheriting source to NeverFetch.
// Nothing is polled and nothing is queued, while a source with an explicit interval may still run.
func TestFetch_DisabledDoesNothing(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true, NeverFetch: true}},
		offers:  refs("a", "b"),
	}
	f := newFetcher(t, stub, limits(10, 2000))
	res, err := f.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesPolled != 0 || len(stub.queued) != 0 || stub.calls != 0 {
		t.Errorf("disabled auto-fetch still ran: polled=%d queued=%v calls=%d",
			res.SourcesPolled, stub.queued, stub.calls)
	}
}

// A row-level Fetch now is a deliberate action, not the global crawler. It must fetch exactly the
// selected enabled source, even when unattended timing is off, while retaining the ordinary cap.
func TestFetch_RunSourceFetchesOnlyTheSelectedSource(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "first", Kind: "archive", URI: "one", Enabled: true},
			{ID: "selected", Kind: "archive", URI: "two", Enabled: true, NeverFetch: true},
		},
		offers: refs("a", "b", "c"),
	}
	f := newFetcher(t, stub, limits(2, 2000))
	res, err := f.RunSource(context.Background(), "selected")
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesPolled != 1 || res.Queued != 2 {
		t.Fatalf("result = %+v, want one selected source and its bounded two items", res)
	}
	if len(stub.listed) != 1 || stub.listed[0] != "two" {
		t.Fatalf("listed = %v, want only the selected source", stub.listed)
	}
	if _, ok := stub.stamped["first"]; ok {
		t.Fatal("the unselected source was stamped as fetched")
	}
}

func TestFetch_ScheduledRunChecksOnlySourcesWhoseEffectiveIntervalIsDue(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "due", Kind: "archive", URI: "due", Enabled: true, Every: 6 * time.Hour, LastCheckedAt: now.Add(-6 * time.Hour)},
			{ID: "not-due", Kind: "archive", URI: "not-due", Enabled: true, Every: 12 * time.Hour, LastCheckedAt: now.Add(-6 * time.Hour)},
			{ID: "custom-while-global-off", Kind: "archive", URI: "custom", Enabled: true, Every: time.Hour, LastCheckedAt: now.Add(-2 * time.Hour)},
		},
		offers: refs("a"),
	}
	f := newFetcher(t, stub, limits(1, 2000)).WithClock(func() time.Time { return now })
	res, err := f.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesPolled != 2 || !reflect.DeepEqual(stub.listed, []string{"due", "custom"}) {
		t.Fatalf("result/listed = %+v / %v, want only the two due sources", res, stub.listed)
	}
}

func TestFetch_ScheduledRunWithNothingDueDoesNotReadCapacity(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stub := &fetchStub{sources: []filler.FetchSource{{
		ID: "later", Kind: "archive", URI: "collection", Enabled: true,
		Every: 6 * time.Hour, LastCheckedAt: now.Add(-time.Hour),
	}}}

	res, err := newFetcher(t, stub, limits(10, 2000)).WithClock(func() time.Time { return now }).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesPolled != 0 || stub.catalogCalls != 0 || stub.calls != 0 {
		t.Fatalf("idle wake = %+v, catalog/provider calls = %d/%d; want a source-state-only no-op",
			res, stub.catalogCalls, stub.calls)
	}
}

func TestFetch_SuccessfulEmptyCheckAdvancesDueTimeWithoutClaimingAFetch(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "empty", Kind: "archive", URI: "empty", Enabled: true, Every: 6 * time.Hour}},
	}
	f := newFetcher(t, stub, limits(10, 2000)).WithClock(func() time.Time { return now })
	if _, err := f.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stub.checked["empty"]; !got.Equal(now) {
		t.Fatalf("last check = %v, want %v", got, now)
	}
	if _, ok := stub.stamped["empty"]; ok {
		t.Fatal("an empty check claimed that it fetched an item")
	}
}

func TestFetch_NextAutomaticCheckCombinesCadenceRetryAndLease(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	base := filler.FetchSource{
		ID: "source", Kind: "archive", URI: "collection", Enabled: true,
		Every: 6 * time.Hour, LastCheckedAt: now.Add(-4 * time.Hour),
	}
	if got, ok := base.NextAutomaticCheck(now); !ok || !got.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("cadence next check = %v/%v, want %v/true", got, ok, now.Add(2*time.Hour))
	}
	base.CheckRetryAt = now.Add(5 * time.Hour)
	if got, _ := base.NextAutomaticCheck(now); !got.Equal(base.CheckRetryAt) {
		t.Fatalf("retry next check = %v, want %v", got, base.CheckRetryAt)
	}
	base.CheckLeaseUntil = now.Add(10 * time.Hour)
	if got, _ := base.NextAutomaticCheck(now); !got.Equal(base.CheckLeaseUntil) {
		t.Fatalf("lease next check = %v, want %v", got, base.CheckLeaseUntil)
	}
	base.NeverFetch = true
	if _, ok := base.NextAutomaticCheck(now); ok {
		t.Fatal("opted-out source projected an automatic check")
	}
}

func TestFetch_ProviderFailureUsesDurableBoundedBackoff(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "failing", Kind: "archive", URI: "collection", Enabled: true,
			CheckFailureCount: 2,
		}},
		enumErr: errors.New("provider unavailable"),
	}
	res, err := newFetcher(t, stub, limits(10, 2000)).WithClock(func() time.Time { return now }).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.SourcesPolled != 1 || !stub.failedChecks["failing"].Equal(now.Add(15*time.Minute)) {
		t.Fatalf("failure result/retry = %+v / %v, want third-failure 15m backoff", res, stub.failedChecks)
	}
	if got := filler.SourceCheckRetryDelay(20); got != time.Hour {
		t.Fatalf("retry cap = %v, want 1h", got)
	}
}

func TestFetch_ScheduledBackoffIsSkippedButManualCheckBypassesIt(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	stub := &fetchStub{
		sources: []filler.FetchSource{{
			ID: "retrying", Kind: "archive", URI: "collection", Enabled: true,
			CheckRetryAt: now.Add(time.Hour), MaxPerRun: 3,
		}},
		offers: refs("a"),
	}
	fetcher := newFetcher(t, stub, limits(10, 2000)).WithClock(func() time.Time { return now })
	if res, err := fetcher.Run(t.Context()); err != nil || res.SourcesPolled != 0 || stub.calls != 0 {
		t.Fatalf("scheduled backoff result = %+v calls=%d err=%v", res, stub.calls, err)
	}
	res, err := fetcher.RunSource(t.Context(), "retrying")
	if err != nil || res.SourcesPolled != 1 || res.MaxPerCheck != 3 || stub.calls != 1 {
		t.Fatalf("manual retry result = %+v calls=%d err=%v", res, stub.calls, err)
	}
}

func TestFetch_ManualCheckRefusesAnActiveSourceClaim(t *testing.T) {
	stub := &fetchStub{
		sources:      []filler.FetchSource{{ID: "active", Kind: "archive", URI: "collection", Enabled: true}},
		activeChecks: map[string]bool{"active": true},
	}
	_, err := newFetcher(t, stub, limits(10, 2000)).RunSource(t.Context(), "active")
	if !errors.Is(err, filler.ErrSourceCheckInProgress) || stub.calls != 0 {
		t.Fatalf("active check error/calls = %v/%d, want in-progress and no provider call", err, stub.calls)
	}
}

func TestFetch_ManualCheckRejectsAnUnknownSource(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "known", Kind: "archive", URI: "collection", Enabled: true}},
	}
	_, err := newFetcher(t, stub, limits(10, 2000)).RunSource(t.Context(), "missing")
	if !errors.Is(err, filler.ErrFetchSourceNotFound) || stub.catalogCalls != 0 || stub.calls != 0 {
		t.Fatalf("unknown source error/catalog/provider calls = %v/%d/%d, want not-found and no work", err, stub.catalogCalls, stub.calls)
	}
}

// Scheduled fetching is resilient across independently failing sources, but Fetch now is a direct
// admin request and must not return success when its selected source could not queue anything.
func TestFetch_RunSourceReportsQueueFailure(t *testing.T) {
	want := errors.New("ingest tooling unavailable")
	stub := &fetchStub{
		sources:   []filler.FetchSource{{ID: "selected", Kind: "archive", URI: "two", Enabled: true}},
		offers:    refs("a"),
		ingestErr: want,
	}

	_, err := newFetcher(t, stub, limits(2, 2000)).RunSource(context.Background(), "selected")
	if !errors.Is(err, want) {
		t.Fatalf("RunSource error = %v, want wrapped queue failure", err)
	}
}

func TestFetch_ScheduledRunKeepsQueueFailureBestEffort(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "first", Kind: "archive", URI: "one", Enabled: true},
			{ID: "second", Kind: "archive", URI: "two", Enabled: true},
		},
		offers:    refs("a"),
		ingestErr: errors.New("one source cannot queue"),
	}

	res, err := newFetcher(t, stub, limits(2, 2000)).Run(context.Background())
	if err != nil {
		t.Fatalf("scheduled Run error = %v, want per-source failure isolated", err)
	}
	if res.SourcesPolled != 2 || res.Queued != 0 {
		t.Fatalf("scheduled result = %+v, want both sources attempted and none queued", res)
	}
}
