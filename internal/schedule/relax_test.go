package schedule_test

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// A tiny pool that can't fill a long no-repeat window forces the relaxation ladder
// to descend (§7): the ladder records each applied step (surfaced in the UI). Two
// programs of the same series with a big episodeNoRepeat can't honor the window, so
// the ladder halves it and records the step.
func TestRelax_LadderDescendsAndRecords(t *testing.T) {
	// A single-series channel with a long no-repeat window and few episodes forces
	// the ladder. Single-series relaxes seriesMinGap/blockMax, so the shortfall is
	// the no-repeat window → ladder step 1 (halve episodeNoRepeat).
	eps := []schedule.ResolvedProgram{
		{LibraryItemID: "e1", Title: "E1", DurationMs: 60000, Season: 1},
		{LibraryItemID: "e2", Title: "E2", DurationMs: 60000, Season: 1},
	}
	entries := []schedule.LineupEntry{{Key: "series:tvdb:1", Title: "Show"}}
	sav := newSeriesAvail(map[string][]schedule.ResolvedProgram{"series:tvdb:1": eps})

	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSyndication, Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(720 * time.Hour)}}}
	ch := schedule.Channel{ID: "r", Name: "R", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 3}}
	d := schedule.ComputeDesiredAt(ch, entries, sav, schedule.PodFill, p, time.Time{})

	// The two episodes still air (never dropped); the ladder just records the relax.
	if len(programKeys(d)) < 2 {
		t.Errorf("episodes should still air after relaxation: got %d", len(programKeys(d)))
	}
	if len(d.Applied) == 0 {
		t.Fatal("a shortfall should have recorded at least one relaxation step")
	}
	// The FIRST ladder step is the no-repeat window (§7 order).
	if d.Applied[0].Kind != "episodeNoRepeat" {
		t.Errorf("first relaxation should be episodeNoRepeat, got %q", d.Applied[0].Kind)
	}
	// The recorded From/To are human-readable durations.
	if !strings.Contains(d.Applied[0].From, "h") || d.Applied[0].To == "" {
		t.Errorf("relaxation should record readable from/to: %+v", d.Applied[0])
	}
}

// Pool growth un-relaxes automatically (§7): a channel that needed relaxation with a
// tiny pool records NO relaxation once the pool is large enough to honor the policy.
func TestRelax_UnRelaxesWhenPoolRecovers(t *testing.T) {
	// A 2h no-repeat window; the pool below is 4 series × 1h = 4h of cycle, which
	// comfortably exceeds the window AND has enough distinct series to honor blockMax
	// — so NO relaxation is needed.
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSyndication, Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(2 * time.Hour)}}}
	ch := schedule.Channel{ID: "u", Name: "U", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 1}}

	entries := []schedule.LineupEntry{
		{Key: "series:tvdb:1", Title: "S1"}, {Key: "series:tvdb:2", Title: "S2"},
		{Key: "series:tvdb:3", Title: "S3"}, {Key: "series:tvdb:4", Title: "S4"},
	}
	eps := func(id string) []schedule.ResolvedProgram {
		return []schedule.ResolvedProgram{{LibraryItemID: id + "a", Title: id, DurationMs: 3600000, Season: 1}}
	}
	sav := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tvdb:1": eps("1"), "series:tvdb:2": eps("2"),
		"series:tvdb:3": eps("3"), "series:tvdb:4": eps("4"),
	})
	d := schedule.ComputeDesiredAt(ch, entries, sav, schedule.PodFill, p, time.Time{})
	if len(d.Applied) != 0 {
		t.Errorf("a healthy pool should apply NO relaxation, got %+v", d.Applied)
	}
}

// The ladder NEVER relaxes audience/scope (§7): even after full descent on a tiny
// pool, a TV-MA item stays off a kids channel and an out-of-era item stays out.
// (Complements TestEnforce_TVMA_NeverUnderKidsCeiling_EvenAfterRelaxation with the
// scope side.)
func TestRelax_NeverRelaxesScope(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "In Era", "", 1994),
		ratedEntry("movie:tmdb:2", "Out Of Era", "", 1980),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{Era: &schedule.Range{From: 1990, To: 1999}}, Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(720 * time.Hour)}}}
	d := schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill, p, time.Time{})
	if hasKey(programKeys(d), "movie:tmdb:2") {
		t.Error("scope (era) must NEVER be relaxed to admit an out-of-era title")
	}
}
