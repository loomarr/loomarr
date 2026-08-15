package setup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/testkit"
)

func newConnector(lib *testkit.LiveTV) *setup.LiveTVConnector {
	urls := setup.TunarrURLsFrom("http://tunarr:8000")
	return setup.NewLiveTVConnectorFixed(lib, urls)
}

func TestTunarrURLsFrom(t *testing.T) {
	urls := setup.TunarrURLsFrom("http://tunarr:8000/")
	if urls.M3U != "http://tunarr:8000/api/channels.m3u" {
		t.Errorf("M3U = %q", urls.M3U)
	}
	if urls.XMLTV != "http://tunarr:8000/api/xmltv.xml" {
		t.Errorf("XMLTV = %q", urls.XMLTV)
	}
}

// The first connect registers both tuner + guide; the SECOND connect is a no-op
// (§6 idempotent enumerate-first — the Phase-10 gate). No duplicate tuners.
func TestConnect_IdempotentSecondCallNoOp(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)
	ctx := context.Background()

	first, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !first.TunerAdded || !first.ListingAdded {
		t.Fatalf("first connect should add both, got %+v", first)
	}
	if first.AlreadyWired() {
		t.Fatal("first connect should not report already-wired")
	}
	if lib.TunerCount() != 1 || lib.ListingCount() != 1 {
		t.Fatalf("after first connect: %d tuners, %d listings (want 1,1)", lib.TunerCount(), lib.ListingCount())
	}

	// Second connect: nothing new registered — the no-op the gate asserts.
	second, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.TunerAdded || second.ListingAdded {
		t.Errorf("second connect registered again: %+v", second)
	}
	if !second.AlreadyWired() {
		t.Error("second connect should report already-wired")
	}
	if lib.TunerCount() != 1 || lib.ListingCount() != 1 {
		t.Errorf("second connect created a duplicate: %d tuners, %d listings", lib.TunerCount(), lib.ListingCount())
	}
}

func TestReconnectTargetUsesExplicitAppliedBackendURLs(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://tunarr:8000")
	target := setup.InternalPlayoutURLs("http://loomarr:8080", "current-token")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	connector := setup.NewLiveTVConnectorFixed(lib, oldURLs)

	result, err := connector.ReconnectTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.TunerRemoved != 1 || !lib.HasTuner(target.M3U) || lib.HasTuner(oldURLs.M3U) {
		t.Fatalf("reconnect result = %+v, target=%v old=%v", result,
			lib.HasTuner(target.M3U), lib.HasTuner(oldURLs.M3U))
	}
	if lib.Rescans != 1 || lib.Refreshes != 1 {
		t.Fatalf("repair pokes = rescans %d refreshes %d, want 1 each", lib.Rescans, lib.Refreshes)
	}
}

// If the tuner exists but the guide is missing (a half-wired state), connect adds
// only the missing half.
func TestConnect_AddsOnlyMissingHalf(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)
	ctx := context.Background()

	// Pre-register the tuner only.
	_ = lib.AddTuner(ctx, setup.TunarrURLsFrom("http://tunarr:8000").M3U)

	res, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.TunerAdded {
		t.Error("tuner already present; should not re-add")
	}
	if !res.ListingAdded {
		t.Error("missing listing provider should be added")
	}
}

func TestPrepare_TunerAddFailureLeavesOldPair(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://old:8000")
	target := setup.TunarrURLsFrom("http://new:8000")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	lib.AddTunerErr = errors.New("media server unavailable")

	c := setup.NewLiveTVConnectorFixed(lib, target)
	if _, err := c.Prepare(ctx, target); err == nil {
		t.Fatal("Prepare should surface the target tuner add failure")
	}
	if !lib.HasTuner(oldURLs.M3U) || lib.TunerCount() != 1 || lib.ListingCount() != 1 {
		t.Fatalf("old pair changed after target tuner failure: tuners=%d listings=%d old=%v",
			lib.TunerCount(), lib.ListingCount(), lib.HasTuner(oldURLs.M3U))
	}
}

func TestPrepare_ListingAddFailureRetainsOldPairAndRetriesMissingHalf(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://old:8000")
	target := setup.TunarrURLsFrom("http://new:8000")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	lib.AddListingErr = errors.New("listing endpoint unavailable")
	c := setup.NewLiveTVConnectorFixed(lib, target)

	first, err := c.Prepare(ctx, target)
	if err == nil {
		t.Fatal("Prepare should surface the target listing add failure")
	}
	if !first.TunerAdded || first.ListingAdded {
		t.Fatalf("first prepare = %+v, want only target tuner added", first)
	}
	if !lib.HasTuner(oldURLs.M3U) || !lib.HasTuner(target.M3U) || lib.ListingCount() != 1 {
		t.Fatalf("partial prepare did not preserve old pair: tuners=%d listings=%d", lib.TunerCount(), lib.ListingCount())
	}

	lib.AddListingErr = nil
	second, err := c.Prepare(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if second.TunerAdded || !second.ListingAdded {
		t.Fatalf("retry = %+v, want only missing target listing added", second)
	}
	if lib.TunerCount() != 2 || lib.ListingCount() != 2 {
		t.Fatalf("retry duplicated or retired early: tuners=%d listings=%d", lib.TunerCount(), lib.ListingCount())
	}
	if lib.Rescans != 0 || lib.Refreshes != 0 {
		t.Fatalf("Prepare must not fetch an unpublished feed: rescans=%d refreshes=%d", lib.Rescans, lib.Refreshes)
	}
}

func TestRetireStale_FailureLeavesTargetAndRetryCompletes(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://old:8000")
	target := setup.TunarrURLsFrom("http://new:8000")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	c := setup.NewLiveTVConnectorFixed(lib, target)
	if _, err := c.Prepare(ctx, target); err != nil {
		t.Fatal(err)
	}
	lib.RemoveTunerErr = errors.New("delete rejected")

	if _, err := c.RetireStale(ctx, target); err == nil {
		t.Fatal("RetireStale should surface stale tuner removal failure")
	}
	if !lib.HasTuner(target.M3U) || !lib.HasTuner(oldURLs.M3U) || lib.ListingCount() != 2 {
		t.Fatalf("removal failure lost a registration: tuners=%d listings=%d", lib.TunerCount(), lib.ListingCount())
	}

	lib.RemoveTunerErr = nil
	res, err := c.RetireStale(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if res.TunerRemoved != 1 || res.ListingRemoved != 1 {
		t.Fatalf("retry retirement = %+v, want one stale pair removed", res)
	}
	if lib.HasTuner(oldURLs.M3U) || !lib.HasTuner(target.M3U) || lib.TunerCount() != 1 || lib.ListingCount() != 1 {
		t.Fatalf("retry did not converge on target pair: tuners=%d listings=%d", lib.TunerCount(), lib.ListingCount())
	}
}

func TestRetireStale_RefusesBeforeBothTargetHalvesExist(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://old:8000")
	target := setup.TunarrURLsFrom("http://new:8000")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	if err := lib.AddTuner(ctx, target.M3U); err != nil {
		t.Fatal(err)
	}
	c := setup.NewLiveTVConnectorFixed(lib, target)

	if _, err := c.RetireStale(ctx, target); err == nil {
		t.Fatal("RetireStale should refuse while the target listing is absent")
	}
	if !lib.HasTuner(oldURLs.M3U) || lib.ListingCount() != 1 {
		t.Fatal("RetireStale removed the old pair before both target halves existed")
	}
}

func TestConnect_PreparesBothTargetHalvesBeforeAnyRetirement(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	oldURLs := setup.TunarrURLsFrom("http://old:8000")
	target := setup.TunarrURLsFrom("http://new:8000")
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	if err := lib.AddListingProvider(ctx, oldURLs.XMLTV); err != nil {
		t.Fatal(err)
	}
	c := setup.NewLiveTVConnectorFixed(lib, target)

	if _, err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	calls := lib.Calls()
	addTuner := callIndex(calls, "add-tuner:"+target.M3U)
	addListing := callIndex(calls, "add-listing:"+target.XMLTV)
	removeTuner := callPrefixIndex(calls, "remove-tuner:")
	removeListing := callPrefixIndex(calls, "remove-listing:")
	if addTuner < 0 || addListing < 0 || removeTuner < 0 || removeListing < 0 {
		t.Fatalf("missing expected transition calls: %v", calls)
	}
	if removeTuner < addTuner || removeTuner < addListing || removeListing < addTuner || removeListing < addListing {
		t.Fatalf("stale retirement ran before target pair was prepared: %v", calls)
	}
}

func callIndex(calls []string, want string) int {
	for i, call := range calls {
		if call == want {
			return i
		}
	}
	return -1
}

func callPrefixIndex(calls []string, prefix string) int {
	for i, call := range calls {
		if len(call) >= len(prefix) && call[:len(prefix)] == prefix {
			return i
		}
	}
	return -1
}

// When the Tunarr URL changes, Connect must RETIRE the Loomarr-owned tuner left at
// the old URL (not orphan it beside the new one) while NEVER touching a tuner the
// household added by hand — the ownership boundary from §6/§9. This is the fix for
// the live-smoke finding: repointing tunarr.url used to leave a dead tuner in Emby.
func TestConnect_URLChange_RetiresStaleLoomarrTuner_KeepsHandAdded(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()

	oldURLs := setup.TunarrURLsFrom("http://100.123.114.40:8000") // the old Tunarr
	newURLs := setup.TunarrURLsFrom("http://tunarr:8000")         // newConnector's URL

	// Pre-state: a Loomarr-owned tuner at the OLD url, plus a hand-added HDHomeRun.
	lib.SeedTuner(oldURLs.M3U, "loomarr")
	lib.SeedTuner("http://192.168.1.50:5004", "My HDHomeRun")
	// And the old Loomarr guide provider.
	_ = lib.AddListingProvider(ctx, oldURLs.XMLTV)

	c := newConnector(lib) // targets http://tunarr:8000
	res, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if res.TunerRemoved != 1 {
		t.Errorf("expected 1 stale Loomarr tuner retired, got %d", res.TunerRemoved)
	}
	if !res.TunerAdded {
		t.Error("the new-URL tuner should have been added")
	}
	if res.AlreadyWired() {
		t.Error("a URL-change reconcile is not a no-op")
	}

	// The stale Loomarr tuner is gone; the new one is present; the hand-added one survives.
	if lib.HasTuner(oldURLs.M3U) {
		t.Error("stale Loomarr tuner at the old URL should have been removed")
	}
	if !lib.HasTuner(newURLs.M3U) {
		t.Error("the new Tunarr tuner should be registered")
	}
	if !lib.HasTuner("http://192.168.1.50:5004") {
		t.Error("a hand-added tuner must never be touched (§9 ownership)")
	}
	// Two tuners remain: the new Loomarr one + the household's HDHomeRun.
	if lib.TunerCount() != 2 {
		t.Errorf("expected 2 tuners (new Loomarr + hand-added), got %d", lib.TunerCount())
	}
	// The stale Loomarr guide was retired and the new one added.
	if res.ListingRemoved != 1 {
		t.Errorf("expected 1 stale Loomarr listing retired, got %d", res.ListingRemoved)
	}
	if lib.ListingCount() != 1 {
		t.Errorf("expected exactly 1 listing (the new guide), got %d", lib.ListingCount())
	}
}

// Re-running Connect at the SAME URL after a URL-change reconcile is still a no-op
// (the stale one is already gone) — the idempotency gate holds across the new path.
func TestConnect_AfterURLChange_IsStableNoOp(t *testing.T) {
	lib := testkit.NewLiveTV()
	ctx := context.Background()
	lib.SeedTuner(setup.TunarrURLsFrom("http://old:8000").M3U, "loomarr")

	c := newConnector(lib)
	if _, err := c.Connect(ctx); err != nil { // moves the tuner to the new URL
		t.Fatal(err)
	}
	again, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyWired() {
		t.Errorf("second connect at the same URL should be a no-op, got %+v", again)
	}
}

// A connect that WIRES something must poke the media server — a tuner re-scan
// (discover the new tuner's channels) AND a guide refresh (fill their EPG) — so
// channels appear in the guide in minutes, not after the nightly scan (§9).
func TestConnect_PokesRescanAndRefreshWhenWired(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)

	res, err := c.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Poked {
		t.Error("a wiring connect should report Poked")
	}
	if lib.Rescans != 1 {
		t.Errorf("tuner re-scans = %d, want 1 (discover the new tuner's channels)", lib.Rescans)
	}
	if lib.Refreshes != 1 {
		t.Errorf("guide refreshes = %d, want 1 (fill the new channels' EPG)", lib.Refreshes)
	}
}

// A no-op connect (Tunarr already wired, nothing changed) pokes NOTHING — there is
// nothing new for the media server to discover, so we don't waste the calls.
func TestConnect_NoOpDoesNotPoke(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)
	ctx := context.Background()

	if _, err := c.Connect(ctx); err != nil { // first: wires + pokes
		t.Fatal(err)
	}
	rescansAfterFirst, refreshesAfterFirst := lib.Rescans, lib.Refreshes

	res, err := c.Connect(ctx) // second: no-op
	if err != nil {
		t.Fatal(err)
	}
	if res.Poked {
		t.Error("a no-op connect must not poke")
	}
	if lib.Rescans != rescansAfterFirst || lib.Refreshes != refreshesAfterFirst {
		t.Errorf("no-op connect poked: rescans %d→%d, refreshes %d→%d",
			rescansAfterFirst, lib.Rescans, refreshesAfterFirst, lib.Refreshes)
	}
}

// A URL change pokes too — the retire-and-re-add path counts as a change, so the
// freshly-moved tuner's channels get discovered immediately.
func TestConnect_URLChange_Pokes(t *testing.T) {
	lib := testkit.NewLiveTV()
	lib.SeedTuner(setup.TunarrURLsFrom("http://old:8000").M3U, "loomarr")
	c := newConnector(lib)

	res, err := c.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Poked || lib.Rescans != 1 || lib.Refreshes != 1 {
		t.Errorf("URL change should poke rescan+refresh, got Poked=%v rescans=%d refreshes=%d",
			res.Poked, lib.Rescans, lib.Refreshes)
	}
}

func TestRescanTargetUsesExplicitPreparedURL(t *testing.T) {
	lib := testkit.NewLiveTV()
	applied := setup.TunarrURLsFrom("http://tunarr:8000")
	prepared := setup.InternalPlayoutURLs("http://loomarr:8080", "device-token")
	c := setup.NewLiveTVConnectorFixed(lib, applied)

	if err := c.RescanTarget(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	calls := lib.Calls()
	want := "rescan-tuner:" + prepared.M3U
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("calls = %v, want [%s]", calls, want)
	}
}

// A poke failure is best-effort: it is recorded in PokeErr but never fails the
// connect — the wiring already succeeded, and the next channel reconcile pokes
// again anyway (§6 pokes never hard-fail wiring).
func TestConnect_PokeFailureDoesNotFailWiring(t *testing.T) {
	lib := testkit.NewLiveTV()
	lib.RescanErr = errors.New("emby scan busy")
	c := newConnector(lib)

	res, err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("a poke failure must not fail the connect, got %v", err)
	}
	if !res.TunerAdded {
		t.Error("wiring should still have succeeded")
	}
	if res.PokeErr == nil {
		t.Error("the poke failure should be surfaced in PokeErr for logging")
	}
}

func TestWired_TrueOnlyWhenBothPresent(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)
	ctx := context.Background()

	wired, _ := c.Wired(ctx)
	if wired {
		t.Fatal("nothing wired yet; Wired should be false")
	}
	_, _ = c.Connect(ctx)
	wired, _ = c.Wired(ctx)
	if !wired {
		t.Fatal("after connect, Wired should be true")
	}
}

func TestConnect_PropagatesRegisterError(t *testing.T) {
	lib := testkit.NewLiveTV()
	lib.AddTunerErr = errors.New("emby down")
	c := newConnector(lib)

	if _, err := c.Connect(context.Background()); err == nil {
		t.Fatal("expected connect to surface the register error")
	}
}

func TestPokeGuideRefresh(t *testing.T) {
	lib := testkit.NewLiveTV()
	c := newConnector(lib)
	if err := c.PokeGuideRefresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lib.Refreshes != 1 {
		t.Errorf("guide refresh count = %d, want 1", lib.Refreshes)
	}
}
