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
	return setup.NewLiveTVConnector(lib, urls)
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
