package acquisition_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/testkit"
)

// retryFixture is the fetch fixture with the real, migrated store as the download ledger and a
// clock the test moves.
type retryFixture struct {
	stub    *fetchStub
	ledger  records.Combined
	fetcher *acquisition.Fetcher
	now     time.Time
	states  map[string]acquisition.ExistingRemoteState
}

func newRetryFixture(t *testing.T, offers ...string) *retryFixture {
	t.Helper()
	db, err := records.Open(t.Context(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fx := &retryFixture{
		stub:   &fetchStub{sources: []acquisition.FetchSource{{ID: "9000a1", Kind: "archive", URI: "coll", Enabled: true}}, offers: refs(offers...)},
		ledger: db, now: time.Unix(1_800_000_000, 0).UTC(), states: map[string]acquisition.ExistingRemoteState{},
	}
	fx.fetcher = acquisition.NewFetcher(fetchStoreWithRemoteStates{
		fetchStub: fx.stub,
		listRemoteStates: func(context.Context) (map[string]acquisition.ExistingRemoteState, error) {
			return fx.states, nil
		},
	}, providersOver(t, fx.stub.list, nil), fx.stub, limits(5, 2000), discardLog()).
		WithClock(func() time.Time { return fx.now }).WithDownloads(db)
	return fx
}

func (fx *retryFixture) item(id string) acquisition.DownloadItem {
	return acquisition.DownloadItem{Kind: "archive", SourceID: "9000a1", RemoteID: id, URL: "https://archive.org/details/" + id}
}

func (fx *retryFixture) fail(t *testing.T, id string) {
	t.Helper()
	if _, err := fx.ledger.RecordDownloadFailure(t.Context(), fx.item(id), "provider said no", fx.now); err != nil {
		t.Fatal(err)
	}
}

func (fx *retryFixture) run(t *testing.T) acquisition.FetchResult {
	t.Helper()
	res, err := fx.fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// An item that failed rests: the source's check does not offer it again until its time, though the
// listing still shows it, and then it is offered like any other.
func TestFetch_ARestingItemIsNotOfferedUntilItsRetryIsDue(t *testing.T) {
	fx := newRetryFixture(t, "item-a", "item-b")
	fx.fail(t, "item-a")

	res := fx.run(t)
	if res.Queued != 1 || len(fx.stub.queuedIDs) != 1 || fx.stub.queuedIDs[0] != "item-b" {
		t.Fatalf("result = %+v queued = %v, want only the item that has not failed", res, fx.stub.queuedIDs)
	}

	// A later check, once the first delay has passed, takes it again.
	fx.now = fx.now.Add(7 * time.Hour)
	fx.stub.queuedIDs = nil
	fx.states[acquisition.RemoteIdentity{Provider: "archive", SourceID: "9000a1", RemoteID: "item-b"}.Key()] = acquisition.RemoteQueued
	fx.run(t)
	if len(fx.stub.queuedIDs) != 1 || fx.stub.queuedIDs[0] != "item-a" {
		t.Fatalf("after the delay queued = %v, want the failed item offered again", fx.stub.queuedIDs)
	}
}

// An item no source listing offers any more (a pulled or typed one) is retried from the record: it
// goes through the ordinary ingest path, with the identity it had, once its time comes.
func TestFetch_ADueItemIsRetriedFromItsRecord(t *testing.T) {
	fx := newRetryFixture(t) // the listing offers nothing
	fx.fail(t, "item-a")

	if res := fx.run(t); res.Queued != 0 || len(fx.stub.queuedIDs) != 0 {
		t.Fatalf("a resting item was retried early: %+v %v", res, fx.stub.queuedIDs)
	}

	fx.now = fx.now.Add(7 * time.Hour)
	res := fx.run(t)
	if res.Queued != 1 || len(fx.stub.queuedIDs) != 1 || fx.stub.queuedIDs[0] != "item-a" ||
		fx.stub.sourceID != "9000a1" || fx.stub.sourceKind != "archive" || fx.stub.queued[0] != "https://archive.org/details/item-a" {
		t.Fatalf("result = %+v ingest = %s/%s %v %v, want the item queued under its own source", res, fx.stub.sourceID, fx.stub.sourceKind, fx.stub.queuedIDs, fx.stub.queued)
	}

	// The download is in flight, and a slow one outlasts the wait for room: no pass starts another
	// copy until the claim has run out.
	fx.stub.queuedIDs, fx.stub.queued = nil, nil
	fx.now = fx.now.Add(2 * acquisition.DownloadWait)
	if res := fx.run(t); res.Queued != 0 || len(fx.stub.queuedIDs) != 0 {
		t.Fatalf("an item whose download is still running was queued again: %+v %v", res, fx.stub.queuedIDs)
	}
}

// An item of a source the operator has switched off is not retried; one of a deleted source is
// forgotten.
func TestFetch_ADueItemOfADisabledOrDeletedSourceIsNotRetried(t *testing.T) {
	fx := newRetryFixture(t)
	fx.fail(t, "item-a")
	gone := acquisition.DownloadItem{Kind: "archive", SourceID: "9000a9", RemoteID: "item-g", URL: "https://archive.org/details/item-g"}
	if _, err := fx.ledger.RecordDownloadFailure(t.Context(), gone, "x", fx.now); err != nil {
		t.Fatal(err)
	}
	fx.stub.sources[0].Enabled = false
	fx.now = fx.now.Add(7 * time.Hour)

	fx.run(t)
	if len(fx.stub.queuedIDs) != 0 {
		t.Fatalf("retried %v for a source that is off or gone", fx.stub.queuedIDs)
	}
	resting, err := fx.ledger.RestingDownloads(t.Context(), fx.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resting[gone.Key()]; ok {
		t.Fatal("an item of a deleted source is still remembered")
	}
}

// A retry that cannot even be queued (a provider paused, no room) is not a failure of the item:
// nothing is counted, and it is tried again after the wait.
func TestFetch_ARetryThatCannotBeQueuedCountsNothing(t *testing.T) {
	fx := newRetryFixture(t)
	fx.fail(t, "item-a")
	fx.now = fx.now.Add(7 * time.Hour)
	fx.stub.ingestErr = errors.New("filler storage paused (library_limit)")

	fx.run(t)
	fx.now = fx.now.Add(acquisition.DownloadWait + time.Minute)
	due, err := fx.ledger.ClaimDueDownloads(t.Context(), fx.now, fx.now.Add(time.Hour), 5)
	if err != nil || len(due) != 1 || due[0].Failures != 1 {
		t.Fatalf("due = %+v %v, want the item back after the wait with still one failure", due, err)
	}
}

// An item that has since been downloaded (the catalog already has it) is forgotten, not fetched
// twice.
func TestFetch_ADueItemTheCatalogAlreadyHasIsForgotten(t *testing.T) {
	fx := newRetryFixture(t)
	fx.fail(t, "item-a")
	fx.states[acquisition.RemoteIdentity{Provider: "archive", SourceID: "9000a1", RemoteID: "item-a"}.Key()] = acquisition.RemoteCatalogued
	fx.now = fx.now.Add(7 * time.Hour)

	fx.run(t)
	if len(fx.stub.queuedIDs) != 0 {
		t.Fatalf("a catalogued item was fetched again: %v", fx.stub.queuedIDs)
	}
	resting, err := fx.ledger.RestingDownloads(t.Context(), fx.now.Add(-100*24*time.Hour))
	if err != nil || len(resting) != 0 {
		t.Fatalf("resting = %v %v, want the record gone", resting, err)
	}
}
