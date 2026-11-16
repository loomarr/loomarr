package fetch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/testkit"
)

// openLedgerStore is the real, migrated store, so the ledger's behaviour is the shipped one.
func openLedgerStore(t *testing.T) records.Store {
	t.Helper()
	db, err := records.Open(t.Context(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func ledgerItem(src fetch.Source) acquisition.DownloadItem {
	return acquisition.DownloadItem{Kind: string(src.Kind), SourceID: src.ID, RemoteID: src.RemoteID, URL: src.URL,
		DurationMS: src.DurationMS, Height: src.Height, Gap: src.Gap}
}

// resting reads what the ledger holds for an item, and when it is next tried.
func resting(t *testing.T, ledger acquisition.DownloadLedger, item acquisition.DownloadItem) (time.Time, bool) {
	t.Helper()
	all, err := ledger.RestingDownloads(t.Context(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	at, ok := all[item.Key()]
	return at, ok
}

// A download that fails is remembered with its next attempt, and the pass counts it as a failure.
func TestRun_FailedItemIsRememberedForARetry(t *testing.T) {
	ledger := openLedgerStore(t)
	src := fetch.Source{ID: "9000a1", Kind: fetch.YouTube, URL: "https://example.invalid/watch/1", RemoteID: "item-1", DurationMS: 30_000, Height: 480}
	ing := fetch.New(fetch.Downloaders{fetch.YouTube: downloaderFunc(func(context.Context, fetch.Source, string) (fetch.DownloadResult, error) {
		return fetch.DownloadResult{}, errors.New("video unavailable")
	})}, t.TempDir(), &testkit.ArtifactWriter{}, discardLog()).WithLedger(ledger)

	res := ing.Run(t.Context(), []fetch.Source{src})
	if res.Failed != 1 || res.Deferred != 0 {
		t.Fatalf("result = %+v, want one failure", res)
	}
	retryAt, ok := resting(t, ledger, ledgerItem(src))
	if !ok {
		t.Fatal("the failed item was not remembered")
	}
	if wait := time.Until(retryAt); wait < 5*time.Hour || wait > 7*time.Hour {
		t.Fatalf("first retry in %v, want the first delay (about 6h)", wait)
	}

	// The retry carries what the item was; a second failure counts and waits longer.
	due, err := ledger.ClaimDueDownloads(t.Context(), time.Now().Add(7*time.Hour), time.Now().Add(8*time.Hour), 5)
	if err != nil || len(due) != 1 || due[0].DownloadItem != ledgerItem(src) || due[0].Failures != 1 {
		t.Fatalf("due = %+v %v, want the item with one failure", due, err)
	}
	if res = ing.Run(t.Context(), []fetch.Source{src}); res.Failed != 1 {
		t.Fatalf("second result = %+v", res)
	}
	all, _ := ledger.ClaimDueDownloads(t.Context(), time.Now().Add(100*24*time.Hour), time.Now().Add(101*24*time.Hour), 5)
	if len(all) != 1 || all[0].Failures != 2 {
		t.Fatalf("after the second failure = %+v, want two counted", all)
	}
}

// A download that works clears whatever the item's failures had built up.
func TestRun_SuccessForgetsEarlierFailures(t *testing.T) {
	ledger := openLedgerStore(t)
	src := fetch.Source{ID: "9000a1", Kind: fetch.YouTube, URL: "https://example.invalid/watch/2", RemoteID: "item-2"}
	if _, err := ledger.RecordDownloadFailure(t.Context(), ledgerItem(src), "earlier", time.Now()); err != nil {
		t.Fatal(err)
	}
	ing := fetch.New(fetch.Downloaders{fetch.YouTube: downloaderFunc(func(context.Context, fetch.Source, string) (fetch.DownloadResult, error) {
		return fetch.DownloadResult{Fetched: 1}, nil
	})}, t.TempDir(), &testkit.ArtifactWriter{}, discardLog()).WithLedger(ledger)

	if res := ing.Run(t.Context(), []fetch.Source{src}); res.Fetched != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v", res)
	}
	if _, ok := resting(t, ledger, ledgerItem(src)); ok {
		t.Fatal("a downloaded item is still remembered as failed")
	}
}

// No room is a temporary cause: the item waits, the pass reports it deferred and not failed, and no
// failure is counted against the item however often the room is missing.
func TestRun_NoRoomDefersWithoutCountingAFailure(t *testing.T) {
	ledger := openLedgerStore(t)
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 20 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 10 * storagegovernor.GiB}
	})
	downloader := estimatingDownloader{
		budget: storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: 4 * storagegovernor.GiB},
		download: func(context.Context, fetch.Source, string) (fetch.DownloadResult, error) {
			t.Fatal("downloaded with no room")
			return fetch.DownloadResult{}, nil
		},
	}
	src := fetch.Source{ID: "9000a1", Kind: fetch.YouTube, URL: "https://example.invalid/watch/3", RemoteID: "item-3"}
	ing := fetch.New(fetch.Downloaders{fetch.YouTube: downloader}, t.TempDir(), &testkit.ArtifactWriter{}, discardLog()).
		WithStorageGovernor(governor).WithLedger(ledger)

	for range 3 {
		meter.measurement.FreeBytes = 20 * storagegovernor.GiB
		plan, err := ing.Prepare(t.Context(), []fetch.Source{src}, storagegovernor.Automatic)
		if err != nil {
			t.Fatal(err)
		}
		meter.measurement.FreeBytes = 7 * storagegovernor.GiB // the room goes after the run was queued
		res := plan.Run(t.Context())
		if res.Failed != 0 || res.Deferred != 1 {
			t.Fatalf("result = %+v, want the item deferred and no failure", res)
		}
	}
	retryAt, ok := resting(t, ledger, ledgerItem(src))
	if !ok {
		t.Fatal("the deferred item was not remembered, so nothing would try it again")
	}
	if wait := time.Until(retryAt); wait <= 0 || wait > 2*acquisition.DownloadWait {
		t.Fatalf("deferred item is tried again in %v, want about %v", wait, acquisition.DownloadWait)
	}
	due, err := ledger.ClaimDueDownloads(t.Context(), time.Now().Add(3*acquisition.DownloadWait), time.Now().Add(4*acquisition.DownloadWait), 5)
	if err != nil || len(due) != 1 || due[0].Failures != 0 {
		t.Fatalf("due = %+v %v, want the item with no failures counted", due, err)
	}
}

// A downloader that reports the storage governor refused it mid-run is the same temporary cause.
func TestRun_CapacityRefusalFromTheDownloaderDefers(t *testing.T) {
	ledger := openLedgerStore(t)
	src := fetch.Source{ID: "9000a1", Kind: fetch.YouTube, URL: "https://example.invalid/watch/4", RemoteID: "item-4"}
	ing := fetch.New(fetch.Downloaders{fetch.YouTube: downloaderFunc(func(context.Context, fetch.Source, string) (fetch.DownloadResult, error) {
		return fetch.DownloadResult{}, &fetch.CapacityError{Decision: storagegovernor.Decision{
			Snapshot: storagegovernor.Snapshot{Reason: storagegovernor.ReasonLibraryLimit},
		}}
	})}, t.TempDir(), &testkit.ArtifactWriter{}, discardLog()).WithLedger(ledger)

	res := ing.Run(t.Context(), []fetch.Source{src})
	if res.Failed != 0 || res.Deferred != 1 {
		t.Fatalf("result = %+v, want deferred and no failure", res)
	}
}
