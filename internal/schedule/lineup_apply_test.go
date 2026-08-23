package schedule_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

func lineupKeys(entries []schedule.LineupEntry) []provision.Key {
	out := make([]provision.Key, len(entries))
	for i, e := range entries {
		out[i] = e.Key
	}
	return out
}

func keysEqual(got []provision.Key, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if string(got[i]) != want[i] {
			return false
		}
	}
	return true
}

// A plain Replace returns the incoming lineup as-is (human/refine approval — a person decided).
func TestApplyLineup_ReplacePlain(t *testing.T) {
	current := []schedule.LineupEntry{entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B")}
	incoming := []schedule.LineupEntry{entry("movie:tmdb:2", "B"), entry("movie:tmdb:3", "C")}

	got := schedule.ApplyLineup(current, incoming, schedule.LineupReplace, schedule.ApplyOpts{})
	if !keysEqual(lineupKeys(got), "movie:tmdb:2", "movie:tmdb:3") {
		t.Fatalf("plain replace should equal incoming, got %v", lineupKeys(got))
	}
}

// Replace + PreserveByKey carries rich metadata (rating/runtime/duration/collection) and an
// OMITTED season window forward from the matching current entry, while display fields and a
// PROVIDED season window come from incoming — the lossy-DTO guard (§7).
func TestApplyLineup_ReplacePreservesByKey(t *testing.T) {
	current := []schedule.LineupEntry{{
		Key: "series:tvdb:1", Title: "Old Title", OfficialRating: "TV-PG",
		RuntimeSec: 1320, DurationMs: 1_320_000, CollectionID: -1, SeasonMin: 1, SeasonMax: 10,
	}}
	// Incoming is DTO-derived: display fields set, rich fields zero, no season window.
	incoming := []schedule.LineupEntry{{Key: "series:tvdb:1", Title: "New Title", Year: 1989}}

	e := schedule.ApplyLineup(current, incoming, schedule.LineupReplace, schedule.ApplyOpts{PreserveByKey: true})[0]
	if e.Title != "New Title" || e.Year != 1989 {
		t.Errorf("display fields must come from incoming, got title=%q year=%d", e.Title, e.Year)
	}
	if e.OfficialRating != "TV-PG" || e.RuntimeSec != 1320 || e.DurationMs != 1_320_000 || e.CollectionID != -1 {
		t.Errorf("rich metadata must be preserved, got %+v", e)
	}
	if e.SeasonMin != 1 || e.SeasonMax != 10 {
		t.Errorf("omitted season window must be preserved, got %d–%d", e.SeasonMin, e.SeasonMax)
	}

	// A PROVIDED season window overrides the preserved one.
	incoming2 := []schedule.LineupEntry{{Key: "series:tvdb:1", Title: "X", SeasonMin: 3, SeasonMax: 5}}
	e2 := schedule.ApplyLineup(current, incoming2, schedule.LineupReplace, schedule.ApplyOpts{PreserveByKey: true})[0]
	if e2.SeasonMin != 3 || e2.SeasonMax != 5 {
		t.Errorf("provided season window must win, got %d–%d", e2.SeasonMin, e2.SeasonMax)
	}
}

// Additive unions fresh onto existing, keeps a not-re-picked title unless Drop reports it
// off-intent, drops it when Drop says so, and appends genuinely-new picks — existing first.
func TestApplyLineup_Additive(t *testing.T) {
	existing := []schedule.LineupEntry{
		entry("movie:tmdb:1", "Keep-available"), // not re-picked, not droppable → kept
		entry("movie:tmdb:2", "Drop-gone"),      // not re-picked, droppable → dropped
		entry("movie:tmdb:3", "Repicked"),       // re-picked → kept regardless
	}
	fresh := []schedule.LineupEntry{
		entry("movie:tmdb:3", "Repicked"),
		entry("movie:tmdb:4", "New"),
	}
	drop := func(e schedule.LineupEntry) bool { return e.Key == "movie:tmdb:2" }

	got := schedule.ApplyLineup(existing, fresh, schedule.LineupAdditive, schedule.ApplyOpts{Drop: drop})
	// existing-kept first (1, 3), then genuinely-new (4); 2 dropped.
	if !keysEqual(lineupKeys(got), "movie:tmdb:1", "movie:tmdb:3", "movie:tmdb:4") {
		t.Fatalf("additive union wrong: got %v", lineupKeys(got))
	}
}

// A nil Drop makes Additive a pure union (nothing dropped) — the fail-safe default.
func TestApplyLineup_AdditiveNilDropKeepsAll(t *testing.T) {
	existing := []schedule.LineupEntry{entry("movie:tmdb:1", "A")}
	fresh := []schedule.LineupEntry{entry("movie:tmdb:2", "B")}
	got := schedule.ApplyLineup(existing, fresh, schedule.LineupAdditive, schedule.ApplyOpts{})
	if len(got) != 2 {
		t.Fatalf("nil Drop must keep everything (pure union), got %v", lineupKeys(got))
	}
}

// Heal enriches in place without changing membership or order; nil Enrich is a no-op.
func TestApplyLineup_Heal(t *testing.T) {
	current := []schedule.LineupEntry{
		{Key: "movie:tmdb:1", Title: "A"},
		{Key: "movie:tmdb:2", Title: "B", OfficialRating: "R"}, // already rated → untouched
	}
	enrich := func(e *schedule.LineupEntry) {
		if e.OfficialRating == "" {
			e.OfficialRating = "PG-13"
		}
	}
	got := schedule.ApplyLineup(current, nil, schedule.LineupHeal, schedule.ApplyOpts{Enrich: enrich})
	if !keysEqual(lineupKeys(got), "movie:tmdb:1", "movie:tmdb:2") {
		t.Fatalf("heal must not change membership/order, got %v", lineupKeys(got))
	}
	if got[0].OfficialRating != "PG-13" {
		t.Errorf("heal should fill the empty rating, got %q", got[0].OfficialRating)
	}
	if got[1].OfficialRating != "R" {
		t.Errorf("heal must not overwrite an already-set rating, got %q", got[1].OfficialRating)
	}
	if len(schedule.ApplyLineup(current, nil, schedule.LineupHeal, schedule.ApplyOpts{})) != 2 {
		t.Fatalf("nil Enrich must be a no-op")
	}
}
