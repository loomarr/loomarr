package clipfetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestYouTubeSourceFinderCancellationStopsCompleteProcessTree(t *testing.T) {
	dir := t.TempDir()
	executable, parentPIDFile, childPIDFile := blockingYtDlp(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := NewYouTubeSourceFinder(executable).Suggest(ctx, "retro commercials", 6)
		result <- err
	}()

	parentPID := waitForHelperPID(t, parentPIDFile)
	childPID := waitForHelperPID(t, childPIDFile)
	t.Cleanup(func() { killTestProcess(childPID) })
	cancel()
	err := waitForDownload(t, result)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("search cancellation = %v, want context.Canceled", err)
	}
	assertProcessGone(t, parentPID)
	assertProcessGone(t, childPID)
}

func TestArchiveSourceFinderSuggestsCollectionsWithoutResolvingOrDownloading(t *testing.T) {
	var searchCalls, metadataCalls int
	var query url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		searchCalls++
		query = r.URL.Query()
		_, _ = fmt.Fprint(w, `{"response":{"numFound":2,"docs":[`+
			`{"identifier":"classic_tv_commercials","title":"Classic TV Commercials","description":"A large collection","item_count":8362},`+
			`{"identifier":"retro_ads","title":"Retro Ads"},`+
			`{"identifier":"station_breaks","title":"Station Breaks","description":["Bumpers","and IDs"]}]}}`)
	})
	mux.HandleFunc("/metadata/", func(http.ResponseWriter, *http.Request) { metadataCalls++ })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	finder := newArchiveSourceFinder(srv.URL, srv.Client())
	got, err := finder.Suggest(context.Background(), "classic tv", 50)
	if err != nil {
		t.Fatal(err)
	}
	if searchCalls != 1 || metadataCalls != 0 {
		t.Fatalf("search/metadata calls = %d/%d, want 1/0", searchCalls, metadataCalls)
	}
	if gotQuery := query.Get("q"); gotQuery != `mediatype:collection AND (title:("classic" "tv") OR identifier:("classic" "tv"))` {
		t.Errorf("q = %q", gotQuery)
	}
	if rows := query.Get("rows"); rows != "8" {
		t.Errorf("rows = %q, want hard cap 8", rows)
	}
	if len(got) != 3 || got[0].CanonicalID != "classic_tv_commercials" || got[0].Title != "Classic TV Commercials" {
		t.Fatalf("suggestions = %#v", got)
	}
	if got[0].CanonicalURL != "https://archive.org/details/classic_tv_commercials" || got[0].ItemCount != 8362 {
		t.Errorf("first suggestion = %#v", got[0])
	}
	if got[2].Description != "Bumpers and IDs" {
		t.Errorf("array description = %q", got[2].Description)
	}
}

func TestArchiveSourceFinderCollapsesConcurrentIdenticalSearchesAndCachesTheResult(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		if calls == 1 {
			close(started)
		}
		mu.Unlock()
		<-release
		_, _ = fmt.Fprint(w, `{"response":{"docs":[{"identifier":"classic","title":"Classic"}]}}`)
	}))
	t.Cleanup(srv.Close)
	finder := newArchiveSourceFinder(srv.URL, srv.Client())

	results := make(chan error, 2)
	go func() { _, err := finder.Suggest(context.Background(), "classic tv", 8); results <- err }()
	<-started
	go func() { _, err := finder.Suggest(context.Background(), "classic tv", 8); results <- err }()
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := finder.Suggest(context.Background(), "  classic   tv ", 8); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want one shared call and a cache hit", calls)
	}
}

func TestArchiveSourceFinderBoundsJSONAndIdentifiesItsRequest(t *testing.T) {
	var userAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, strings.Repeat("x", maxSourceFinderJSONBytes+1))
	}))
	t.Cleanup(srv.Close)

	_, err := newArchiveSourceFinder(srv.URL, srv.Client()).Suggest(context.Background(), "classic", 8)
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized response error = %v", err)
	}
	if userAgent != "Loomarr filler-source-finder" {
		t.Fatalf("User-Agent = %q", userAgent)
	}
}

func TestArchiveSourceFinderKeepsUserSyntaxInsideQuotedSearchTerms(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Get("q")
		_, _ = fmt.Fprint(w, `{"response":{"docs":[]}}`)
	}))
	t.Cleanup(srv.Close)

	if _, err := newArchiveSourceFinder(srv.URL, srv.Client()).Suggest(
		context.Background(), `classic) OR mediatype:movies`, 8,
	); err != nil {
		t.Fatal(err)
	}
	if query != `mediatype:collection AND (title:("classic" "OR" "mediatype" "movies") OR identifier:("classic" "OR" "mediatype" "movies"))` {
		t.Fatalf("query = %q", query)
	}
}

func TestArchiveSourceFinderResolvesOnlyCollections(t *testing.T) {
	var previewQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		previewQuery = r.URL.Query()
		_, _ = fmt.Fprint(w, `{"response":{"numFound":24,"docs":[`+
			`{"identifier":"ad_one","title":"First ad"},`+
			`{"identifier":"ad_two","title":"Second ad"},`+
			`{"identifier":"ad_three","title":"Third ad"}]}}`)
	})
	mux.HandleFunc("/metadata/classic_tv_commercials", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"metadata":{"mediatype":"collection","title":"Classic TV Commercials","description":"Broadcast advertising"}}`)
	})
	mux.HandleFunc("/metadata/one_video", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"metadata":{"mediatype":"movies","title":"One video"}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	finder := newArchiveSourceFinder(srv.URL, srv.Client())

	for _, input := range []string{
		"classic_tv_commercials",
		"/details/classic_tv_commercials",
		"https://archive.org/details/classic_tv_commercials?tab=about#reviews",
	} {
		got, err := finder.Resolve(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if got.CanonicalID != "classic_tv_commercials" || got.Title != "Classic TV Commercials" || got.TargetType != "collection" {
			t.Fatalf("%q resolved = %#v", input, got)
		}
		if got.ItemCount != 24 || len(got.PreviewItems) != 3 || got.PreviewItems[0].Title != "First ad" ||
			got.PreviewItems[0].URL != "https://archive.org/details/ad_one" {
			t.Fatalf("%q preview = %#v", input, got)
		}
		if gotQuery := previewQuery.Get("q"); gotQuery != `collection:"classic_tv_commercials" AND mediatype:movies AND (format:"MPEG4" OR format:"h.264 IA" OR format:"512Kb MPEG4")` {
			t.Fatalf("%q preview query = %q", input, gotQuery)
		}
		if sort := previewQuery.Get("sort[]"); sort != "downloads desc" {
			t.Fatalf("%q preview sort = %q", input, sort)
		}
		if rows := previewQuery.Get("rows"); rows != "3" {
			t.Fatalf("%q preview rows = %q", input, rows)
		}
	}
	if _, err := finder.Resolve(context.Background(), "one_video"); err == nil {
		t.Fatal("resolved a movie as a collection source")
	}
}

func TestArchiveSourceFinderTreatsAPastedDetailsURLAsOneExactSuggestion(t *testing.T) {
	var listingCalls, metadataCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, _ *http.Request) {
		listingCalls++
		_, _ = fmt.Fprint(w, `{"response":{"docs":[]}}`)
	})
	mux.HandleFunc("/metadata/classic_tv_commercials", func(w http.ResponseWriter, _ *http.Request) {
		metadataCalls++
		_, _ = fmt.Fprint(w, `{"metadata":{"mediatype":"collection","title":"Classic TV Commercials"}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := newArchiveSourceFinder(srv.URL, srv.Client()).Suggest(
		context.Background(), "https://archive.org/details/classic_tv_commercials", 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if listingCalls != 1 || metadataCalls != 1 || len(got) != 1 || got[0].CanonicalID != "classic_tv_commercials" {
		t.Fatalf("listing/metadata/results = %d/%d/%#v", listingCalls, metadataCalls, got)
	}
}

func TestArchiveSourceFinderKeepsAValidSourceWhenExamplesAreUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/classic_tv_commercials", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"metadata":{"mediatype":"collection","title":"Classic TV Commercials"}}`)
	})
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := newArchiveSourceFinder(srv.URL, srv.Client()).Resolve(context.Background(), "classic_tv_commercials")
	if err != nil {
		t.Fatal(err)
	}
	if got.CanonicalID != "classic_tv_commercials" || len(got.PreviewItems) != 0 {
		t.Fatalf("resolved source = %#v", got)
	}
}

func TestArchiveSourceFinderRejectsUnusableInputBeforeHTTP(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	t.Cleanup(srv.Close)
	finder := newArchiveSourceFinder(srv.URL, srv.Client())

	if _, err := finder.Suggest(context.Background(), "x", 8); err == nil {
		t.Fatal("one-character query was accepted")
	}
	if _, err := finder.Resolve(context.Background(), "https://example.com/details/nope"); err == nil {
		t.Fatal("foreign URL was accepted")
	}
	if calls != 0 {
		t.Fatalf("made %d HTTP calls for invalid input", calls)
	}
}
