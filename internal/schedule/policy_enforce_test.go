package schedule_test

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// ratedEntry builds a movie lineup entry with policy-enforcement metadata stamped.
func ratedEntry(key, title, rating string, year int, genres ...string) schedule.LineupEntry {
	return schedule.LineupEntry{
		Key:            provision.Key(key),
		Title:          title,
		OfficialRating: schedule.NormalizeRating(rating),
		Year:           year,
		Genres:         genres,
	}
}

// programKeys returns the ordered set of program-slot keys in a desired lineup.
func programKeys(d schedule.DesiredLineup) []provision.Key {
	var out []provision.Key
	for _, s := range d.Slots {
		if s.IsProgram() {
			out = append(out, s.Key)
		}
	}
	return out
}

func hasKey(keys []provision.Key, k provision.Key) bool {
	for _, x := range keys {
		if x == k {
			return true
		}
	}
	return false
}

// policyChannel is a channel with an explicit ordering (so tests don't depend on
// the inherited Strategy). Sequential keeps assertion order simple.
func policyChannel() schedule.Channel {
	return schedule.Channel{ID: "chp", Name: "Policy", Number: 9, Strategy: schedule.Sequential}
}

func computeWithPolicy(entries []schedule.LineupEntry, avail schedule.Availability, p schedule.ChannelPolicy) schedule.DesiredLineup {
	return schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill, p, time.Time{})
}

// --- §10 BINDING: scope filters bind (era / genre) ------------------------------

func TestEnforce_EraBinds(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "In Era", "", 1994),
		ratedEntry("movie:tmdb:2", "Too Old", "", 1985),
		ratedEntry("movie:tmdb:3", "Too New", "", 2005),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2", "movie:tmdb:3": "l3"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{Era: &schedule.Range{From: 1990, To: 1999}}}}

	keys := programKeys(computeWithPolicy(entries, avail, p))
	if !hasKey(keys, "movie:tmdb:1") {
		t.Error("in-era title should be scheduled")
	}
	if hasKey(keys, "movie:tmdb:2") || hasKey(keys, "movie:tmdb:3") {
		t.Errorf("out-of-era titles must be filtered: got %v", keys)
	}
}

func TestEnforce_GenreBinds(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Toon", "", 1994, "Animation"),
		ratedEntry("movie:tmdb:2", "Doc", "", 1994, "Documentary"),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
		Genres: schedule.GenreFilter{Include: []string{"Animation"}, Exclude: []string{"Documentary"}},
	}}}

	keys := programKeys(computeWithPolicy(entries, avail, p))
	if !hasKey(keys, "movie:tmdb:1") || hasKey(keys, "movie:tmdb:2") {
		t.Errorf("genre include/exclude didn't bind: got %v", keys)
	}
}

// --- §10 FAIL-CLOSED AUDIENCE ---------------------------------------------------

func TestEnforce_AudienceFailsClosed(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Kids OK", "TV-Y7", 1994),
		ratedEntry("movie:tmdb:2", "Too Adult", "TV-MA", 1994),
		ratedEntry("movie:tmdb:3", "No Rating", "", 1994), // unrated
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2", "movie:tmdb:3": "l3"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Audience: schedule.AudiencePolicy{Ceiling: "TV-Y7"}}}

	d := computeWithPolicy(entries, avail, p)
	keys := programKeys(d)
	if !hasKey(keys, "movie:tmdb:1") {
		t.Error("a below-ceiling title should air")
	}
	if hasKey(keys, "movie:tmdb:2") {
		t.Error("TV-MA must NEVER air under a TV-Y7 ceiling")
	}
	if hasKey(keys, "movie:tmdb:3") {
		t.Error("an UNRATED title must fail closed under a kids ceiling (never guessed)")
	}
	// The exclusion report explains the drops (for proposal review / UI).
	if d.Excluded.OverCeiling != 1 || d.Excluded.Unrated != 1 {
		t.Errorf("exclusion report = over:%d unrated:%d, want over:1 unrated:1", d.Excluded.OverCeiling, d.Excluded.Unrated)
	}
}

// TV-MA must never reach a kids channel EVEN AFTER FULL LADDER RELAXATION (§7): the
// audience filter runs before the pool enters the ladder, so no amount of
// separation relaxation can admit it. Drive a tiny pool (forces the ladder) with a
// TV-MA item present and assert it never appears.
func TestEnforce_TVMA_NeverUnderKidsCeiling_EvenAfterRelaxation(t *testing.T) {
	// One below-ceiling item + one TV-MA item; the tiny below-ceiling pool forces the
	// relaxation ladder to descend (it can't fill a long no-repeat window from 1 item).
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Kids", "TV-Y", 1994),
		ratedEntry("movie:tmdb:2", "Adult", "TV-MA", 1994),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Audience: schedule.AudiencePolicy{Ceiling: "TV-Y"}, Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(720 * time.Hour)}}}
	d := computeWithPolicy(entries, avail, p)
	for _, k := range programKeys(d) {
		if k == "movie:tmdb:2" {
			t.Fatal("TV-MA reached a kids channel — fail-closed audience was violated")
		}
	}
}

// A general/adult channel (no ceiling) admits unrated + adult content.
func TestEnforce_NoCeiling_AdmitsEverything(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Adult", "TV-MA", 2001),
		ratedEntry("movie:tmdb:2", "Unrated", "", 2001),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	keys := programKeys(computeWithPolicy(entries, avail, schedule.ChannelPolicy{}))
	if len(keys) != 2 {
		t.Errorf("no ceiling should admit all: got %v", keys)
	}
}

// --- §10 DETERMINISM ------------------------------------------------------------

func TestEnforce_Deterministic(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "A", "", 1994),
		ratedEntry("movie:tmdb:2", "B", "", 1995),
		ratedEntry("movie:tmdb:3", "C", "", 1996),
		ratedEntry("movie:tmdb:4", "D", "", 1997),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2", "movie:tmdb:3": "l3", "movie:tmdb:4": "l4"}
	ch := schedule.Channel{ID: "d", Name: "D", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 42}}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Ordering: schedule.OrderSyndication}}

	a := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, p, time.Time{})
	b := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, p, time.Time{})
	ka, kb := programKeys(a), programKeys(b)
	if len(ka) != len(kb) {
		t.Fatalf("nondeterministic length: %d vs %d", len(ka), len(kb))
	}
	for i := range ka {
		if ka[i] != kb[i] {
			t.Fatalf("nondeterministic order at %d: %q vs %q", i, ka[i], kb[i])
		}
	}
}

// ⚠ **`scope.collections` does NOT bind, and this pins that it is honest about it.**
//
// The field exists on ScopePolicy and round-trips through PATCH → policy_json → the engine,
// but no filter reads it: the scope pass above checks Series, Era, Genres and RuntimeMax and
// skips Collections entirely. There is no library-adapter support for listing collections or
// resolving membership either (§12 records it as ORPHANED, pending `scripts/capture-collections.sh`).
//
// So this asserts the CURRENT truth rather than the desired one: setting a collection scope
// filters nothing. It is deliberately not skipped or commented out — a test that documents an
// inert field is what stops the next reader assuming it works, and it will fail the moment
// collections starts binding, which is exactly when someone should come back and rewrite it
// into the positive assertion.
func TestEnforce_CollectionsDoesNotBindYet(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Nominally In The Collection", "", 1994),
		ratedEntry("movie:tmdb:2", "Nominally Outside It", "", 1994),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
		Collections: []string{"star-trek"},
	}}}

	keys := programKeys(computeWithPolicy(entries, avail, p))
	if len(keys) != 2 {
		t.Fatalf("collections now filters (%d of 2 scheduled) — the field has started binding.\n"+
			"That is good news, and this test is now wrong: rewrite it as the positive assertion "+
			"(in-collection scheduled, out-of-collection filtered) and drop the Collections "+
			"exclusion from suggest.scopeNarrows.", len(keys))
	}
}
