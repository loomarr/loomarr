package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
)

// testClipAirWindow pins the air window's storage (#3384): a new clip has none, the sweep lists
// only airable clips without one, the setter is its writer, and a re-scan does not erase it.
func testClipAirWindow(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	for _, hash := range []string{"spot", "held", "reel"} {
		c := sampleClip(hash, hash, clips.Commercial, 0, "", "")
		c.DurationMs, c.CreatedAt, c.UpdatedAt = 30_000, at, at
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	held, _ := s.GetClip(ctx, "held")
	if _, err := s.HoldClips(ctx, []string{held.Path}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipComposite(ctx, "reel", true, at); err != nil {
		t.Fatal(err)
	}

	pending, err := s.ClipsWithoutAirWindow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Hash != "spot" || pending[0].AirEndMs != 0 {
		t.Fatalf("clips without a window = %+v, want only the airable spot", pending)
	}

	if err := s.SetClipAirWindow(ctx, "spot", 1_300, 29_200, at); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipAirWindow(ctx, "spot", 5, 5, at); err == nil {
		t.Error("an empty window was stored")
	}
	got, err := s.GetClip(ctx, "spot")
	if err != nil {
		t.Fatal(err)
	}
	if got.AirStartMs != 1_300 || got.AirEndMs != 29_200 || got.AirLengthMs() != 27_900 {
		t.Fatalf("stored window = [%d, %d), want [1300, 29200)", got.AirStartMs, got.AirEndMs)
	}
	if pending, err = s.ClipsWithoutAirWindow(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("after measuring, clips without a window = %+v (%v)", pending, err)
	}

	// A re-scan of the same file keeps its window.
	again := sampleClip("spot", "spot", clips.Commercial, 0, "", "")
	again.DurationMs, again.CreatedAt, again.UpdatedAt = 30_000, at, at.Add(time.Hour)
	if err := s.UpsertClip(ctx, again); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetClip(ctx, "spot"); got.AirStartMs != 1_300 || got.AirEndMs != 29_200 {
		t.Errorf("a re-scan changed the window to [%d, %d)", got.AirStartMs, got.AirEndMs)
	}
	if err := s.SetClipAirWindow(ctx, "missing", 0, 1_000, at); err == nil {
		t.Error("a window was set on a clip that does not exist")
	}

	// A new file under the clip's row (a re-encode) is new bytes: its window is measured again.
	replaced := sampleClip("spot-reencoded", "spot", clips.Commercial, 0, "", "")
	replaced.DurationMs, replaced.UpdatedAt = 29_960, at.Add(2*time.Hour)
	if err := s.ReplaceClipIdentity(ctx, "spot", replaced); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetClip(ctx, "spot-reencoded"); err != nil || got.AirStartMs != 0 || got.AirEndMs != 0 {
		t.Errorf("after a re-encode the window = [%d, %d) (%v), want it unmeasured", got.AirStartMs, got.AirEndMs, err)
	}
}
