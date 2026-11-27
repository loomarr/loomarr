package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// A failed download is remembered per item: each failure counts and schedules the next attempt, a
// wait for a temporary cause counts nothing, a claim moves the next attempt out of reach of the
// next pass, and a download that succeeds forgets the item (#3855, decision 12 of #3838).
func testFillerDownloadFailures(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0).UTC()
	item := acquisition.DownloadItem{Kind: "youtube", SourceID: "9000a1", RemoteID: "item-1", URL: "https://example.invalid/1",
		DurationMS: 30_000, Height: 480, Gap: "era:1990"}
	other := acquisition.DownloadItem{Kind: "archive", SourceID: "9000a2", RemoteID: "item-2", URL: "https://example.invalid/2"}

	// Three failures walk the schedule: the delays, then the rest.
	var got acquisition.DownloadFailure
	for n := 1; n <= acquisition.DownloadAttempts; n++ {
		var err error
		if got, err = s.RecordDownloadFailure(ctx, item, "provider said no", at); err != nil {
			t.Fatal(err)
		}
		if got.Failures != n || !got.RetryAt.Equal(acquisition.DownloadRetryAt(n, at)) {
			t.Fatalf("failure %d: recorded %+v, want failures=%d retry_at=%v", n, got, n, acquisition.DownloadRetryAt(n, at))
		}
	}
	if got.DownloadItem != item || got.LastError != "provider said no" || !got.FirstFailedAt.Equal(at) || !got.LastFailedAt.Equal(at) {
		t.Fatalf("record lost what a retry needs: %+v", got)
	}

	// The item is resting; a different item is unaffected.
	resting, err := s.RestingDownloads(ctx, at.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if retryAt, ok := resting[item.Key()]; !ok || !retryAt.Equal(at.Add(acquisition.DownloadRest)) {
		t.Fatalf("resting = %v, want the item resting until %v", resting, at.Add(acquisition.DownloadRest))
	}
	if _, ok := resting[other.Key()]; ok {
		t.Fatal("an item that never failed is resting")
	}
	if due, err := s.ClaimDueDownloads(ctx, at.Add(48*time.Hour), at.Add(49*time.Hour), 10); err != nil || len(due) != 0 {
		t.Fatalf("a resting item was claimed: %v %v", due, err)
	}

	// A wait for a temporary cause counts nothing and does not reset what has counted.
	if err := s.DeferDownload(ctx, other, "waiting for room", at.Add(time.Hour), at); err != nil {
		t.Fatal(err)
	}
	if err := s.DeferDownload(ctx, other, "waiting for room", at.Add(2*time.Hour), at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	due, err := s.ClaimDueDownloads(ctx, at.Add(150*time.Minute), at.Add(210*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].Key() != other.Key() || due[0].Failures != 0 || due[0].LastError != "waiting for room" {
		t.Fatalf("due = %+v, want the waiting item with no failure counted", due)
	}

	// Claiming moved it out of reach: the next pass does not pick it up, and a failure after the
	// claim is the first that counts.
	if again, err := s.ClaimDueDownloads(ctx, at.Add(151*time.Minute), at.Add(211*time.Minute), 10); err != nil || len(again) != 0 {
		t.Fatalf("a claimed item was claimed twice: %v %v", again, err)
	}
	failed, err := s.RecordDownloadFailure(ctx, other, "gone", at.Add(3*time.Hour))
	if err != nil || failed.Failures != 1 {
		t.Fatalf("first counted failure = %+v, %v", failed, err)
	}

	// Oldest first, and the limit holds.
	third := acquisition.DownloadItem{Kind: "youtube", SourceID: "9000a1", RemoteID: "item-3", URL: "https://example.invalid/3"}
	if err := s.DeferDownload(ctx, third, "waiting", at, at); err != nil {
		t.Fatal(err)
	}
	if err := s.DeferDownload(ctx, item, "waiting", at, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	batch, err := s.ClaimDueDownloads(ctx, at.Add(time.Hour), at.Add(2*time.Hour), 1)
	if err != nil || len(batch) != 1 {
		t.Fatalf("claim with limit 1 = %v %v", batch, err)
	}

	// A download that succeeds forgets the item, and forgetting an unknown one is not an error.
	if err := s.ClearDownload(ctx, item.Key()); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearDownload(ctx, "no such item"); err != nil {
		t.Fatal(err)
	}
	resting, err = s.RestingDownloads(ctx, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resting[item.Key()]; ok {
		t.Fatal("a cleared item is still resting")
	}

	// The nightly purge drops what has nowhere to go: an item of a source that was deleted. An item
	// of a source that exists, a typed URL and an open-search find all stay.
	if err := s.UpsertFillerSource(ctx, NewFillerSource("9000a3", "archive", "coll", "A source", at)); err != nil {
		t.Fatal(err)
	}
	keep := []acquisition.DownloadItem{
		{Kind: "archive", SourceID: "9000a3", RemoteID: "kept-1", URL: "https://example.invalid/k1"},
		{Kind: "youtube", URL: "https://example.invalid/typed"},
		{Kind: "archive", SourceID: acquisition.OpenSearchSourceID, RemoteID: "kept-2", URL: "https://example.invalid/k2"},
	}
	gone := acquisition.DownloadItem{Kind: "archive", SourceID: "9000a9", RemoteID: "gone-1", URL: "https://example.invalid/g1"}
	for _, it := range append(keep, gone) {
		if _, err := s.RecordDownloadFailure(ctx, it, "x", at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.PurgeSpentFillerRecords(ctx, at); err != nil {
		t.Fatal(err)
	}
	resting, err = s.RestingDownloads(ctx, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resting[gone.Key()]; ok {
		t.Fatal("an item of a deleted source survived the purge")
	}
	for _, it := range keep {
		if _, ok := resting[it.Key()]; !ok {
			t.Fatalf("the purge dropped %+v", it)
		}
	}
}
