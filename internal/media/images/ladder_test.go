package images

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"
)

// roles is every Role constant the package declares: a Role missing from it would never be on the
// AVIF job's work list (AVIFRungs). Structural, so it reads the declarations from the syntax tree.
func TestRolesListsEveryRoleConstant(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "image.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var declared []Role
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := spec.Type.(*ast.Ident); !ok || ident.Name != "Role" {
			return true
		}
		for _, v := range spec.Values {
			if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				declared = append(declared, Role(s))
			}
		}
		return true
	})
	if len(declared) == 0 {
		t.Fatal("found no Role constants in image.go")
	}
	for _, r := range declared {
		if !slices.Contains(roles, r) {
			t.Errorf("Role %q is declared but missing from roles, so the AVIF job never encodes it", r)
		}
	}
	if len(roles) != len(declared) {
		t.Errorf("roles = %v, want exactly the declared %v", roles, declared)
	}
}

func TestLaddersAreAscendingAndNonEmpty(t *testing.T) {
	for _, r := range []Role{RolePoster, RoleBackdrop, RoleIcon, RoleThumb} {
		w := r.Widths()
		if len(w) == 0 {
			t.Fatalf("role %q has an empty ladder", r)
		}
		if !slices.IsSorted(w) {
			t.Errorf("role %q ladder is not ascending: %v — the renderer can normalize order, but "+
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

// Fanart is drawn full screen, so its ladder reaches 1920 and the TV rung 3840 (#2774); episode
// stills keep the short ladder.
func TestBackdropLadderReachesFullScreen(t *testing.T) {
	if want := []int{300, 780, 1280, 1920, 3840}; !slices.Equal(RoleBackdrop.Widths(), want) {
		t.Errorf("backdrop ladder = %v, want %v", RoleBackdrop.Widths(), want)
	}
	if want := []int{300, 780, 1280}; !slices.Equal(RoleThumb.Widths(), want) {
		t.Errorf("thumb ladder = %v, want %v", RoleThumb.Widths(), want)
	}
}

// ⚠ The server never makes a rung wider than the original: a 1280 original asked for 1920 is
// served at 1280, and a 1920 one has no 3840 rung.
func TestWidthsForNeverUpscale(t *testing.T) {
	cases := []struct {
		source int
		want   []int
	}{
		{0, []int{300, 780, 1280, 1920, 3840}}, // unknown: the whole ladder
		{100, []int{300, 780, 1280}},           // card-size rungs are kept for a small original
		{1000, []int{300, 780, 1280}},
		{1280, []int{300, 780, 1280}},
		{1920, []int{300, 780, 1280, 1920}},
		{3840, []int{300, 780, 1280, 1920, 3840}},
	}
	for _, c := range cases {
		if got := RoleBackdrop.WidthsFor(c.source); !slices.Equal(got, c.want) {
			t.Errorf("WidthsFor(%d) = %v, want %v", c.source, got, c.want)
		}
	}
	if got := RoleBackdrop.NearestWidthFor(3840, 1280); got != 1280 {
		t.Errorf("NearestWidthFor(3840, 1280) = %d, want 1280", got)
	}
	if got := RoleBackdrop.NearestWidthFor(100000, 1920); got != 1920 {
		t.Errorf("NearestWidthFor(100000, 1920) = %d, want 1920", got)
	}
	if got := RoleBackdrop.NearestWidthFor(400, 3840); got != 780 {
		t.Errorf("NearestWidthFor(400, 3840) = %d, want 780 (a phone gets a small rung)", got)
	}
}

// A still offers only the rungs its original fills at least 85% of, and always the smallest
// (#4476): a 300-wide catalogue still and a 320-wide filler clip still are each the one 300 rung,
// asked for at any width, while a split proposal's full-size segment still keeps every rung.
func TestStillWidthsNeverUpscale(t *testing.T) {
	cases := []struct {
		source int
		want   []int
	}{
		{0, []int{300, 780, 1280}},    // unknown: the whole ladder
		{185, []int{300}},             // narrower than the smallest rung: that rung, upscaled
		{300, []int{300}},             // a catalogue still
		{320, []int{300}},             // a filler clip's still (tools.PreviewWidth)
		{1000, []int{300, 780}},       // a 1000-wide arr screenshot: 78% of 1280, too far
		{1087, []int{300, 780}},       // just under 85% of 1280
		{1088, []int{300, 780, 1280}}, // exactly 85% of 1280 (1088 = 1280 × 0.85)
		{1100, []int{300, 780, 1280}}, // 86% of 1280: near enough to stretch
		{1920, []int{300, 780, 1280}}, // a split proposal's 1080p segment still
	}
	for _, c := range cases {
		if got := RoleThumb.WidthsFor(c.source); !slices.Equal(got, c.want) {
			t.Errorf("thumb WidthsFor(%d) = %v, want %v", c.source, got, c.want)
		}
		if got := RoleThumb.AVIFWidthsFor(c.source); !slices.Equal(got, c.want) {
			t.Errorf("thumb AVIFWidthsFor(%d) = %v, want %v", c.source, got, c.want)
		}
	}
	if got := RoleThumb.NearestWidthFor(1280, 300); got != 300 {
		t.Errorf("NearestWidthFor(1280, 300) = %d, want 300, never an upscale", got)
	}
	if got := RoleThumb.NearestWidthFor(1280, 1920); got != 1280 {
		t.Errorf("NearestWidthFor(1280, 1920) = %d, want 1280 for a full-size segment still", got)
	}
}

// A poster follows the stills rule (#4476): only the rungs its original fills at least 85% of,
// and always the smallest, so a 185-wide poster is never offered at 342, 500 or 780, while a
// 680-wide one keeps 780.
func TestPosterWidthsNeverUpscale(t *testing.T) {
	cases := []struct {
		source int
		want   []int
	}{
		{0, []int{154, 185, 342, 500, 780}},    // unknown: the whole ladder
		{100, []int{154}},                      // narrower than the smallest rung: that rung
		{185, []int{154, 185}},                 // a TMDB w185 poster
		{290, []int{154, 185}},                 // 84.8% of 342: too far
		{300, []int{154, 185, 342}},            // 87.7% of 342: near enough
		{499, []int{154, 185, 342, 500}},       // 64% of 780: too far
		{500, []int{154, 185, 342, 500}},       // a TMDB w500 poster
		{662, []int{154, 185, 342, 500}},       // just under 85% of 780
		{663, []int{154, 185, 342, 500, 780}},  // exactly 85% of 780 (663 = 780 × 0.85)
		{680, []int{154, 185, 342, 500, 780}},  // a TVDB poster, 87% of 780
		{2000, []int{154, 185, 342, 500, 780}}, // an original: every rung
	}
	for _, c := range cases {
		if got := RolePoster.WidthsFor(c.source); !slices.Equal(got, c.want) {
			t.Errorf("poster WidthsFor(%d) = %v, want %v", c.source, got, c.want)
		}
		if got := RolePoster.AVIFWidthsFor(c.source); !slices.Equal(got, c.want) {
			t.Errorf("poster AVIFWidthsFor(%d) = %v, want %v", c.source, got, c.want)
		}
	}
	if got := RolePoster.NearestWidthFor(780, 185); got != 185 {
		t.Errorf("NearestWidthFor(780, 185) = %d, want 185, never an upscale", got)
	}
	if got := RolePoster.NearestWidthFor(500, 300); got != 342 {
		t.Errorf("NearestWidthFor(500, 300) = %d, want 342, the widest rung a 300 original nearly fills", got)
	}
	if got := RolePoster.NearestWidthFor(780, 680); got != 780 {
		t.Errorf("NearestWidthFor(780, 680) = %d, want 780 for a near fit", got)
	}
}

// The serve snap never leaves an image's own set: whatever width is asked for, the rung served is
// one WidthsFor offers for that original.
func TestNearestWidthForStaysInTheSet(t *testing.T) {
	for _, role := range []Role{RolePoster, RoleThumb, RoleBackdrop} {
		for _, source := range []int{0, 100, 185, 290, 300, 499, 680, 1000, 1100, 2000} {
			set := role.WidthsFor(source)
			for _, requested := range []int{1, 154, 186, 343, 501, 780, 1281, 99999} {
				if got := role.NearestWidthFor(requested, source); !slices.Contains(set, got) {
					t.Errorf("%s NearestWidthFor(%d, %d) = %d, outside its set %v", role, requested, source, got, set)
				}
			}
		}
	}
}

// AVIF stops at 1920: the 3840 encode is the dearest rendition and a 4K client takes WebP.
func TestAVIFLadderStopsAt1920(t *testing.T) {
	if got, want := RoleBackdrop.AVIFWidthsFor(3840), []int{300, 780, 1280, 1920}; !slices.Equal(got, want) {
		t.Errorf("AVIFWidthsFor(3840) = %v, want %v", got, want)
	}
	if got, want := RolePoster.AVIFWidthsFor(0), RolePoster.Widths(); !slices.Equal(got, want) {
		t.Errorf("poster AVIF ladder = %v, want the whole ladder %v", got, want)
	}
}
