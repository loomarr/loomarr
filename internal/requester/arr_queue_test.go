package requester

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// QueueStatus correlates queue records to titles by the arr's internal id (resolved via lookup),
// reporting Grabbed + progress for those with a live download record.
func TestArr_QueueStatus_ReportsProgress(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.lookupID = 42 // the movie's arr id
	// A queue record for that movie: 25% done (size 100, sizeleft 75), status "downloading".
	stub.queueForID = 42
	stub.queueRecID = 7
	stub.queueSize, stub.queueLeft = 100, 75
	stub.queueStatus, stub.queueTime = "downloading", "00:10:00"
	a := arrFor("movie", stub.server.URL, "", "")

	items, err := a.QueueStatus(context.Background(), []provision.Title{
		{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if !it.Grabbed {
		t.Error("title with a queue record should be Grabbed")
	}
	if it.Progress != 0.25 {
		t.Errorf("progress = %v, want 0.25", it.Progress)
	}
	if it.Status != "downloading" || it.ETAText != "00:10:00" {
		t.Errorf("status/eta passthrough wrong: %+v", it)
	}
}

// A title with no queue record is reported not-grabbed (still merely requested, or imported).
func TestArr_QueueStatus_NotGrabbedWhenAbsent(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.lookupID = 42
	stub.queueForID = 1 // a DIFFERENT movie is queued
	stub.queueRecID = 9
	a := arrFor("movie", stub.server.URL, "", "")

	items, err := a.QueueStatus(context.Background(), []provision.Title{
		{MediaType: provision.Movie, TMDBID: 603, Name: "M"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Grabbed {
		t.Errorf("absent from queue should be not-grabbed: %+v", items)
	}
}

// A warning/stalled status is still Grabbed (a record exists) but its status is surfaced.
func TestArr_QueueStatus_SurfacesWarning(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.lookupID = 42
	stub.queueForID = 42
	stub.queueRecID = 3
	stub.queueSize, stub.queueLeft = 100, 100 // 0% — stuck
	stub.queueStatus = "warning"
	a := arrFor("movie", stub.server.URL, "", "")

	items, err := a.QueueStatus(context.Background(), []provision.Title{
		{MediaType: provision.Movie, TMDBID: 603, Name: "M"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].Grabbed || items[0].Status != "warning" || items[0].Progress != 0 {
		t.Errorf("stalled download should be grabbed with warning status + 0 progress: %+v", items[0])
	}
}

// A downloading series reports "8 of 36 episodes" (Home's On the way, #1667) from Sonarr's own
// series statistics: files on disk of the episodes it wants (monitored and aired). The bytes
// progress is one release's; the counts are the series'.
func TestArr_QueueStatus_SeriesEpisodeCounts(t *testing.T) {
	stub := newArrStub(t, "series")
	stub.lookupID, stub.queueForID, stub.queueRecID = 42, 42, 5
	stub.queueSize, stub.queueLeft = 100, 50
	stub.seriesStats = map[int]map[string]any{
		42: {"episodeFileCount": 8, "episodeCount": 36, "totalEpisodeCount": 40},
	}
	a := arrFor("series", stub.server.URL, "", "")

	items, err := a.QueueStatus(context.Background(), []provision.Title{
		{MediaType: provision.Series, TVDBID: 90001, Name: "A sitcom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := items[0]; !got.Grabbed || got.EpisodesHave != 8 || got.EpisodesWanted != 36 {
		t.Errorf("series item = %+v, want grabbed with 8 of 36 episodes", got)
	}
}

// Counts decorate the progress, so a statistics read that fails leaves them out and keeps the
// progress: the poll must not lose a download's percentage over a missing count.
func TestArr_QueueStatus_SeriesCountsMissingKeepProgress(t *testing.T) {
	stub := newArrStub(t, "series")
	stub.lookupID, stub.queueForID, stub.queueRecID = 42, 42, 5
	stub.queueSize, stub.queueLeft = 100, 50 // no seriesStats → the series read 404s
	a := arrFor("series", stub.server.URL, "", "")

	items, err := a.QueueStatus(context.Background(), []provision.Title{
		{MediaType: provision.Series, TVDBID: 90001, Name: "A sitcom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := items[0]; !got.Grabbed || got.Progress != 0.5 || got.EpisodesHave != 0 || got.EpisodesWanted != 0 {
		t.Errorf("series item = %+v, want grabbed at 0.5 with no counts", got)
	}
}
