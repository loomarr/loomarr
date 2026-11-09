package records

import (
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// One PATCH of a registered source is one command: the switch, the coverage and the automatic
// download tuning land together or not at all, and an edit that leaves a part out leaves it alone.
func testChangeFillerSource(t *testing.T, newStore NewStoreFunc) {
	ctx := t.Context()
	s := newStore(t)
	at := time.Unix(1_800_000_000, 0).UTC()
	if err := s.UpsertFillerSource(ctx, NewFillerSource("chg-1", "archive", "chg", "Change me", at)); err != nil {
		t.Fatal(err)
	}
	canada, every, perRun := pod.Geography{Country: "ca"}, 900, 5

	if err := s.ChangeFillerSource(ctx, "chg-1", FillerSourceChange{
		Enabled: false, Geography: &canada, FetchPolicy: &FillerFetchPolicy{EverySeconds: &every, MaxPerRun: &perRun},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := findSource(t, s, "chg-1")
	if got.Enabled || got.Geography != (pod.Geography{Country: "CA"}) ||
		got.FetchEverySeconds == nil || *got.FetchEverySeconds != 900 ||
		got.FetchMaxPerRun == nil || *got.FetchMaxPerRun != 5 {
		t.Fatalf("after the full edit = %+v, want off, CA, every 900s, 5 per run", got)
	}

	// A switch-only edit leaves the coverage and the tuning exactly as they were.
	if err := s.ChangeFillerSource(ctx, "chg-1", FillerSourceChange{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = findSource(t, s, "chg-1")
	if !got.Enabled || got.Geography != (pod.Geography{Country: "CA"}) ||
		got.FetchEverySeconds == nil || *got.FetchEverySeconds != 900 || got.FetchMaxPerRun == nil {
		t.Fatalf("after the switch-only edit = %+v, want the coverage and tuning kept", got)
	}

	// An empty policy is "defaults": both overrides go back to inherit.
	if err := s.ChangeFillerSource(ctx, "chg-1", FillerSourceChange{Enabled: true, FetchPolicy: &FillerFetchPolicy{}}); err != nil {
		t.Fatal(err)
	}
	got, _ = findSource(t, s, "chg-1")
	if got.FetchEverySeconds != nil || got.FetchMaxPerRun != nil {
		t.Fatalf("after clearing the policy = %+v, want both overrides back to inherit", got)
	}

	// An unknown id is refused rather than believed recorded.
	if err := s.ChangeFillerSource(ctx, "nope", FillerSourceChange{Enabled: false}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown source error = %v, want ErrNotFound", err)
	}
}
