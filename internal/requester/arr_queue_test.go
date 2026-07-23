package requester

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
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
