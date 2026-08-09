package images

import (
	"slices"
	"testing"
)

func TestLaddersAreAscendingAndNonEmpty(t *testing.T) {
	for _, r := range []Role{RolePoster, RoleBackdrop, RoleIcon, RoleThumb} {
		w := r.Widths()
		if len(w) == 0 {
			t.Fatalf("role %q has an empty ladder", r)
		}
		if !slices.IsSorted(w) {
			t.Errorf("role %q ladder is not ascending: %v — ResizeLadder sorts internally, but "+
				"NearestWidth's round-up walk assumes order", r, w)
		}
	}
}

// ⚠ **The amplification guard, and the reason NearestWidth exists at all.** The serve route takes
// a width from the URL. Honouring it literally would let an unauthenticated caller request ten
// thousand distinct widths and make the box encode ten thousand renditions — CPU and disk
// amplification with no login required. Snapping to the ladder bounds the set of files one image
// can ever produce.
func TestNearestWidthSnapsToTheLadder(t *testing.T) {
	ladder := RolePoster.Widths()

	for _, requested := range []int{1, 3, 99, 155, 300, 499, 501, 779, 781, 100000, -5} {
		got := RolePoster.NearestWidth(requested)
		if !slices.Contains(ladder, got) {
			t.Errorf("NearestWidth(%d) = %d, which is not a ladder rung %v", requested, got, ladder)
		}
	}
}

func TestNearestWidthRoundsUpThenClamps(t *testing.T) {
	// Rounding up rather than to the nearest: a layout that asked for 500 and got 342 renders
	// visibly soft, which is worse than a few extra kilobytes.
	if got := RolePoster.NearestWidth(343); got != 500 {
		t.Errorf("NearestWidth(343) = %d, want 500 (round up, never down)", got)
	}
	if got := RolePoster.NearestWidth(154); got != 154 {
		t.Errorf("an exact rung must be returned unchanged, got %d", got)
	}
	// Past the top rung it clamps rather than inventing a bigger one.
	if got := RolePoster.NearestWidth(99999); got != 780 {
		t.Errorf("NearestWidth(99999) = %d, want the 780 ceiling", got)
	}
}

// An invented role must land somewhere visibly small rather than somewhere plausible: Role is a
// closed set, so reaching the fallback means a caller is wrong and should be able to see it.
func TestUnknownRoleFallsBackSmall(t *testing.T) {
	got := Role("banner").Widths()
	if !slices.Equal(got, RoleIcon.Widths()) {
		t.Errorf("unknown role ladder = %v, want the icon ladder %v", got, RoleIcon.Widths())
	}
}

func TestPosterLadderMirrorsTMDBTokens(t *testing.T) {
	// These numbers are TMDB's own poster size tokens. If they ever drift apart, a future
	// fetcher can no longer take TMDB's rendition unmodified when the rungs match — so pin them.
	want := []int{154, 185, 342, 500, 780}
	if !slices.Equal(RolePoster.Widths(), want) {
		t.Errorf("poster ladder = %v, want TMDB's tokens %v", RolePoster.Widths(), want)
	}
}

func TestAspectRatios(t *testing.T) {
	if got := RolePoster.AspectRatio(); got < 0.66 || got > 0.67 {
		t.Errorf("poster aspect = %v, want 2:3", got)
	}
	if got := RoleBackdrop.AspectRatio(); got < 1.77 || got > 1.78 {
		t.Errorf("backdrop aspect = %v, want 16:9", got)
	}
	// An icon is whatever shape the operator uploaded; claiming a fixed aspect would letterbox
	// their logo, so the role reports none and the stored dimensions win.
	if got := RoleIcon.AspectRatio(); got != 0 {
		t.Errorf("icon aspect = %v, want 0 (no fixed shape)", got)
	}
}
