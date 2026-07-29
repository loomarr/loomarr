package suggest

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// kidsIntent is an intent that signals a kids/teen audience, so a proposed audience ceiling
// is KEPT (§4/§8). Ceiling tests use it; a no-signal Intent{} would drop the ceiling.
func kidsIntent() Intent { return Intent{Description: "cartoons for kids"} }

// groundPolicy is the extraction grounding gate (programming-design §8): the model
// proposes rule VALUES; we validate + clamp them. A valid policy survives; a
// hallucinated ceiling is DROPPED (not passed through to enforcement).
func TestGroundPolicy_ValidPolicySurvives(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-Y7"
	raw.Era.From, raw.Era.To = 1990, 1999
	raw.Genres.Include = []string{"Animation"}
	raw.Ordering = "syndication"
	raw.Seasonal.Mode = "auto"

	p := groundPolicy(raw, nil, nil, kidsIntent())
	if p.Audience.Ceiling != "TV-Y7" {
		t.Errorf("ceiling = %q, want TV-Y7", p.Audience.Ceiling)
	}
	if p.Scope.Era == nil || p.Scope.Era.From != 1990 || p.Scope.Era.To != 1999 {
		t.Errorf("era not extracted: %+v", p.Scope.Era)
	}
	if p.Ordering != schedule.OrderSyndication {
		t.Errorf("ordering = %q, want syndication", p.Ordering)
	}
	if p.Seasonal.Mode != schedule.SeasonalAuto {
		t.Errorf("seasonal mode = %q, want auto", p.Seasonal.Mode)
	}
	// The grounded policy must itself validate (closed enums, on-ladder ceiling).
	if err := p.Validate(); err != nil {
		t.Errorf("grounded policy should validate: %v", err)
	}
}

// A hallucinated / off-ladder ceiling is DROPPED, never passed through — the model
// can't invent a rating the enforcer doesn't understand.
func TestGroundPolicy_OffLadderCeilingDropped(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-SUPERSAFE" // not on the ladder
	p := groundPolicy(raw, nil, nil, Intent{})
	if p.Audience.Ceiling != "" {
		t.Errorf("off-ladder ceiling should be dropped, got %q", p.Audience.Ceiling)
	}
}

// Unknown enum values (ordering / seasonal mode) are dropped, not persisted.
func TestGroundPolicy_UnknownEnumsDropped(t *testing.T) {
	raw := &pickPolicy{}
	raw.Ordering = "sideways"
	raw.Seasonal.Mode = "eventually"
	p := groundPolicy(raw, nil, nil, Intent{})
	if p.Ordering != schedule.OrderInherit {
		t.Errorf("unknown ordering should be dropped (inherit), got %q", p.Ordering)
	}
	if p.Seasonal.Mode != schedule.SeasonalDefault {
		t.Errorf("unknown seasonal mode should be dropped, got %q", p.Seasonal.Mode)
	}
}

// A nil policy (the model omitted it) yields an empty policy → built-in defaults.
func TestGroundPolicy_NilIsEmpty(t *testing.T) {
	p := groundPolicy(nil, nil, nil, Intent{})
	if err := p.Validate(); err != nil {
		t.Errorf("empty policy must validate: %v", err)
	}
	if p.Audience.Ceiling != "" || p.Ordering != schedule.OrderInherit {
		t.Errorf("nil policy should be fully empty, got %+v", p)
	}
}

// THE empty-channel bug (live smoke): a small model set a TV-G ceiling but picked The
// Simpsons (TV-PG). The fail-closed audience gate would drop every episode → an empty
// channel. groundPolicy must RAISE the ceiling to admit the grounded picks.
func TestGroundPolicy_CeilingRaisedToAdmitPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-G" // rank 2 — stricter than the pick
	lineup := []ProposalItem{
		{Name: "The Simpsons", OfficialRating: "TV-PG"}, // rank 3
	}
	p := groundPolicy(raw, lineup, nil, kidsIntent())
	if p.Audience.Ceiling != "TV-PG" {
		t.Errorf("ceiling = %q, want TV-PG (raised to admit the TV-PG pick)", p.Audience.Ceiling)
	}
}

// The guard NEVER lowers a ceiling: a kids channel (TV-Y7) whose picks are all at/below
// stays capped, so a stray adult title can't sneak in via a low-rated pick set.
func TestGroundPolicy_CeilingNotLoweredBelowPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-PG" // rank 3
	lineup := []ProposalItem{
		{Name: "Kids Show", OfficialRating: "TV-Y7"}, // rank 1 — below the ceiling
	}
	p := groundPolicy(raw, lineup, nil, kidsIntent())
	if p.Audience.Ceiling != "TV-PG" {
		t.Errorf("ceiling = %q, want TV-PG (unchanged — picks are below it)", p.Audience.Ceiling)
	}
}

// An unrated pick doesn't raise the ceiling (it's governed by UnratedPolicy, not the
// ladder) — so an unrated title can't blow the roof off a kids ceiling.
func TestGroundPolicy_UnratedPickDoesNotRaiseCeiling(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-Y7"
	lineup := []ProposalItem{
		{Name: "Mystery", OfficialRating: ""},        // unrated → ignored by the guard
		{Name: "Also Mystery", OfficialRating: "??"}, // off-ladder → ignored
	}
	p := groundPolicy(raw, lineup, nil, kidsIntent())
	if p.Audience.Ceiling != "TV-Y7" {
		t.Errorf("ceiling = %q, want TV-Y7 (unrated picks never raise it)", p.Audience.Ceiling)
	}
}

// THE CEILING-IS-KIDS-ONLY RULE (§4/§8): with NO kids/teen signal in the intent, a
// model-proposed ceiling is DROPPED — an unqualified channel is adult-default. This is the
// "1980s Action Heroes shouldn't be capped at TV-14 and lose Die Hard" fix.
func TestGroundPolicy_NoKidsSignalDropsProposedCeiling(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-14" // a small model reflexively caps "action"
	// An adult-themed intent with no kids/family/teen words.
	intent := Intent{Description: "1980s action heroes", Era: "1980s", Tone: "high-energy"}
	p := groundPolicy(raw, nil, nil, intent)
	if p.Audience.Ceiling != "" {
		t.Errorf("ceiling = %q, want \"\" (no kids signal → no ceiling, adult-default)", p.Audience.Ceiling)
	}
}

// With a kids/teen signal present, the proposed ceiling is KEPT and enforced — the guardrail
// still works for the channels it's for. Tested across several signal phrasings.
func TestGroundPolicy_KidsSignalKeepsCeiling(t *testing.T) {
	for _, desc := range []string{
		"cartoons for kids", "family-friendly adventures", "a Bluey channel",
		"Saturday morning cartoons", "wholesome shows for children", "teen dramas",
	} {
		raw := &pickPolicy{}
		raw.Audience.Ceiling = "TV-Y7"
		p := groundPolicy(raw, nil, nil, Intent{Description: desc})
		if p.Audience.Ceiling != "TV-Y7" {
			t.Errorf("intent %q: ceiling = %q, want TV-Y7 (kids signal keeps it)", desc, p.Audience.Ceiling)
		}
	}
}

// The kids signal can come from must-include terms or refine text, not just description.
func TestGroundPolicy_KidsSignalFromOtherIntentFields(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-G"
	// No kids word in the description; the signal is in MustInclude.
	p := groundPolicy(raw, nil, nil, Intent{Description: "animated films", MustInclude: []string{"kids classics"}})
	if p.Audience.Ceiling != "TV-G" {
		t.Errorf("ceiling = %q, want TV-G (kids signal from mustInclude keeps it)", p.Audience.Ceiling)
	}
}

// Even on a kept (kids) ceiling, the raise-to-admit-picks NEVER crosses the kids→adult line:
// a stray R pick on a kids channel does NOT lift the ceiling to R — it's left for the §4
// enforcer to DROP. This is the safety bound on the auto-raise.
func TestGroundPolicy_RaiseNeverCrossesKidsLine(t *testing.T) {
	raw := &pickPolicy{}
	raw.Audience.Ceiling = "TV-Y7" // rank 1
	lineup := []ProposalItem{
		{Name: "Cartoon", OfficialRating: "TV-Y7"}, // fine
		{Name: "A Mistake", OfficialRating: "R"},   // rank 5 — must NOT pull the ceiling up
	}
	p := groundPolicy(raw, lineup, nil, kidsIntent())
	if r, _ := p.Audience.Ceiling.Rank(); r > schedule.KidsCeilingRank {
		t.Errorf("ceiling = %q raised above the kids line — a kids channel must never admit adult content via a stray pick", p.Audience.Ceiling)
	}
}

func series(tmdbID int, name string) ProposalItem {
	return ProposalItem{MediaType: provision.Series, TMDBID: tmdbID, Name: name}
}
func movie(tmdbID int, name string) ProposalItem {
	return ProposalItem{MediaType: provision.Movie, TMDBID: tmdbID, Name: name}
}

// THE STAR TREK FIX: a lineup with ≥2 distinct series and NO ordering picked by the model
// defaults to syndication, so the shows INTERMIX instead of playing one to completion then
// the next (programming-design §5). This is what makes a multi-show channel feel like a
// rerun network rather than a chronological box set.
func TestGroundPolicy_MultiSeriesDefaultsToSyndication(t *testing.T) {
	raw := &pickPolicy{} // model omitted ordering
	lineup := []ProposalItem{series(1, "TNG"), series(2, "DS9"), series(3, "Voyager")}
	p := groundPolicy(raw, lineup, nil, Intent{})
	if p.Ordering != schedule.OrderSyndication {
		t.Errorf("ordering = %q, want syndication (multi-series default)", p.Ordering)
	}
}

// A SINGLE-series channel is NOT force-defaulted: ordering stays OrderInherit ("") so the
// channel's Strategy decides (episodes in order — correct for one show). Defaulting a single
// show to syndication would intermix it with nothing and pointlessly scramble episode order.
func TestGroundPolicy_SingleSeriesStaysInherit(t *testing.T) {
	raw := &pickPolicy{}
	lineup := []ProposalItem{series(1, "The Simpsons"), series(1, "The Simpsons")} // same show twice → 1 distinct
	p := groundPolicy(raw, lineup, nil, Intent{})
	if p.Ordering != schedule.OrderInherit {
		t.Errorf("ordering = %q, want inherit (single distinct series is not multi-series)", p.Ordering)
	}
}

// One series + several movies is NOT "multi-series" (only ≥2 distinct SERIES qualifies), so
// it stays inherit. (Conservative reading of §5 — a one-show-plus-movies channel keeps order.)
func TestGroundPolicy_OneSeriesPlusMoviesStaysInherit(t *testing.T) {
	raw := &pickPolicy{}
	lineup := []ProposalItem{series(1, "MST3K"), movie(10, "B-Movie A"), movie(11, "B-Movie B")}
	p := groundPolicy(raw, lineup, nil, Intent{})
	if p.Ordering != schedule.OrderInherit {
		t.Errorf("ordering = %q, want inherit (one series + movies is not multi-series)", p.Ordering)
	}
}

// The model's EXPLICIT ordering choice always wins over the multi-series default: a user who
// asks for a chronological multi-show marathon gets sequential, not an override to syndication.
func TestGroundPolicy_ExplicitOrderingWinsOverMultiSeriesDefault(t *testing.T) {
	raw := &pickPolicy{Ordering: "sequential"}
	lineup := []ProposalItem{series(1, "TNG"), series(2, "DS9")}
	p := groundPolicy(raw, lineup, nil, Intent{})
	if p.Ordering != schedule.OrderSequential {
		t.Errorf("ordering = %q, want sequential (explicit model choice wins)", p.Ordering)
	}
}

// --- §6.6 curation-rule grounding ---

// A weekend marathon of a grounded series lowers into a real SchedulingRule: weekend When,
// marathon How (sequential + unbounded + no breaks + WindowFull), the series scope
// intersected with the grounded picks.
func TestGroundRules_WeekendMarathon(t *testing.T) {
	tng := series(1, "TNG")
	key, _ := tng.Key()
	raw := &pickPolicy{Rules: []pickRule{
		{When: "weekend", What: "series:" + string(key), How: "marathon"},
	}}
	p := groundPolicy(raw, []ProposalItem{tng, series(2, "DS9")}, nil, Intent{})
	if len(p.Rules) != 1 {
		t.Fatalf("want 1 grounded rule, got %d", len(p.Rules))
	}
	r := p.Rules[0]
	if !r.When.Matches(time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)) { // Saturday
		t.Error("weekend rule should match Saturday")
	}
	if r.How.Ordering != schedule.OrderSequential || !r.How.NoBreaks || r.Window != schedule.WindowFull {
		t.Errorf("marathon How not applied: %+v window=%v", r.How, r.Window)
	}
	if r.What == nil || len(r.What.Series) != 1 || r.What.Series[0] != key {
		t.Errorf("series scope not intersected onto the rule: %+v", r.What)
	}
}

// A rule whose WHEN token is unknown is DROPPED entirely (no time predicate = meaningless).
func TestGroundRules_UnknownWhenDropped(t *testing.T) {
	raw := &pickPolicy{Rules: []pickRule{
		{When: "whenever-i-feel-like-it", How: "marathon"},
		{When: "primetime", How: "syndication"},
	}}
	p := groundPolicy(raw, nil, nil, Intent{})
	if len(p.Rules) != 1 || p.Rules[0].When.HourFrom != 20 {
		t.Errorf("only the valid primetime rule should survive: %+v", p.Rules)
	}
}

// A series WHAT that names a series NOT in the grounded picks drops the series scope (the
// rule keeps its timing but inherits the channel scope) — a rule can't scope to a phantom.
func TestGroundRules_SeriesScopeIntersectedWithGroundedPicks(t *testing.T) {
	tng := series(1, "TNG")
	raw := &pickPolicy{Rules: []pickRule{
		{When: "weekend", What: "series:series:tvdb:99999", How: "marathon"}, // not a grounded pick
	}}
	p := groundPolicy(raw, []ProposalItem{tng}, nil, Intent{})
	if len(p.Rules) != 1 {
		t.Fatalf("rule should survive (timing valid), got %d", len(p.Rules))
	}
	if p.Rules[0].What != nil {
		t.Errorf("ungrounded series scope should drop to nil (inherit channel scope): %+v", p.Rules[0].What)
	}
}

// No rules proposed → nil Rules (base whole-policy behavior, backward compatible).
func TestGroundRules_NoneProposed(t *testing.T) {
	p := groundPolicy(&pickPolicy{}, nil, nil, Intent{})
	if p.Rules != nil {
		t.Errorf("no rules proposed should yield nil Rules, got %+v", p.Rules)
	}
}

// ERA AUTO-WIDEN (programming-design §4): a model-proposed era that would exclude the
// channel's OWN grounded picks is widened to admit them. Caught live — a "Midnight Sci-Fi
// Horror" proposal carried era.from 1982 AND Alien (1979) on its approved lineup, so the
// enforcer filtered out a title the operator had explicitly approved: six on the lineup,
// four in the guide, and nothing naming the missing two.
func TestGroundPolicy_EraWidenedToAdmitPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Era.From, raw.Era.To = 1982, 2026
	lineup := []ProposalItem{
		{Name: "Alien", Year: 1979},          // older than from → must widen
		{Name: "The Thing", Year: 1982},      // exactly on the bound
		{Name: "Alien: Romulus", Year: 2024}, // inside
	}
	p := groundPolicy(raw, lineup, nil, Intent{Description: "sci-fi horror"})
	if p.Scope.Era == nil {
		t.Fatal("era should survive, widened")
	}
	if p.Scope.Era.From != 1979 {
		t.Errorf("era.from = %d, want 1979 (widened to admit Alien, a title the operator approved)", p.Scope.Era.From)
	}
	if p.Scope.Era.To != 2026 {
		t.Errorf("era.to = %d, want 2026 (unchanged — no pick is later)", p.Scope.Era.To)
	}
}

// An acquisition counts too: it becomes a real airing the moment it lands, so an era that
// excluded it would quietly drop the title AFTER the download finished — the most
// confusing possible timing for the operator.
func TestGroundPolicy_EraWidenedForAcquisitionsToo(t *testing.T) {
	raw := &pickPolicy{}
	raw.Era.From, raw.Era.To = 2000, 2010
	acquisitions := []ProposalItem{{Name: "Nosferatu", Year: 2024}}
	p := groundPolicy(raw, nil, acquisitions, Intent{Description: "horror"})
	if p.Scope.Era == nil || p.Scope.Era.To != 2024 {
		t.Errorf("era = %+v, want to=2024 (widened for a pending acquisition)", p.Scope.Era)
	}
}

// The widen only ever LOOSENS. An era that already admits every pick is left exactly as the
// model proposed it — the guard must not become a way to silently rewrite a good scope.
func TestGroundPolicy_EraUntouchedWhenItAlreadyAdmitsPicks(t *testing.T) {
	raw := &pickPolicy{}
	raw.Era.From, raw.Era.To = 1990, 1999
	lineup := []ProposalItem{{Name: "The Matrix", Year: 1999}, {Name: "Speed", Year: 1994}}
	p := groundPolicy(raw, lineup, nil, Intent{Description: "90s action"})
	if p.Scope.Era == nil || p.Scope.Era.From != 1990 || p.Scope.Era.To != 1999 {
		t.Errorf("era = %+v, want 1990-1999 unchanged", p.Scope.Era)
	}
}

// A 0 bound means UNBOUNDED to the enforcer, so it must never be "widened" — stretching it
// to a pick's year would turn an open end into a closed one and NARROW the era, excluding
// content the open bound admitted.
func TestGroundPolicy_EraOpenBoundStaysOpen(t *testing.T) {
	raw := &pickPolicy{}
	raw.Era.From = 1990 // to is 0 = open-ended
	lineup := []ProposalItem{{Name: "Old Thing", Year: 1975}, {Name: "New Thing", Year: 2024}}
	p := groundPolicy(raw, lineup, nil, Intent{Description: "stuff"})
	if p.Scope.Era == nil {
		t.Fatal("era should survive")
	}
	if p.Scope.Era.From != 1975 {
		t.Errorf("era.from = %d, want 1975 (widened for the 1975 pick)", p.Scope.Era.From)
	}
	if p.Scope.Era.To != 0 {
		t.Errorf("era.to = %d, want 0 (already unbounded — closing it would EXCLUDE content)", p.Scope.Era.To)
	}
}

// A pick with no year can't argue for widening anything.
func TestGroundPolicy_UnknownYearDoesNotWidenEra(t *testing.T) {
	raw := &pickPolicy{}
	raw.Era.From, raw.Era.To = 1990, 1999
	lineup := []ProposalItem{{Name: "Mystery", Year: 0}}
	p := groundPolicy(raw, lineup, nil, Intent{Description: "x"})
	if p.Scope.Era == nil || p.Scope.Era.From != 1990 || p.Scope.Era.To != 1999 {
		t.Errorf("era = %+v, want 1990-1999 unchanged by a yearless pick", p.Scope.Era)
	}
}
