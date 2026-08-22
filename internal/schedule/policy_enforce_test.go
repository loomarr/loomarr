package schedule_test

import (
	"strings"
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

func TestEnforce_EraBindsEveryExpandedEpisode(t *testing.T) {
	series := ratedEntry("series:tmdb:456", "The Simpsons", "TV-PG", 1989, "Animation", "Comedy")
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tmdb:456": {
			{LibraryItemID: "classic", Title: "Marge vs. the Monorail", DurationMs: 1_320_000, Season: 4, Episode: 12, Year: 1993},
			{LibraryItemID: "modern", Title: "Top Goon", DurationMs: 1_320_000, Season: 34, Episode: 11, Year: 2022},
		},
	})
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Scope: schedule.ScopePolicy{Era: &schedule.Range{From: 1989, To: 1999}},
	}}

	desired := computeWithPolicy([]schedule.LineupEntry{series}, avail, policy)
	if desired.ProgramCount() != 1 || desired.Slots[0].LibraryItemID != "classic" {
		t.Fatalf("1989-1999 series scope scheduled %+v, want only the 1993 episode", desired.Slots)
	}
	if len(desired.Excluded.Items) != 1 || desired.Excluded.Items[0].Reason != "out_of_scope" {
		t.Fatalf("episode exclusion report = %+v, want one out_of_scope episode", desired.Excluded)
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

// --- §4 PER-EPISODE AUDIENCE ----------------------------------------------------
//
// A series entry's rating is a SUMMARY of its episodes, and enforcing against it alone let
// above-ceiling episodes air. King of the Hill, from the maintainer's live library: a TV-PG
// series whose 275 episodes are 253 × TV-PG, 20 × unrated, and 2 × TV-14. The entry gate
// admitted the show — correctly, it IS a TV-PG show — and both TV-14 episodes then aired on a
// TV-PG channel because nothing below the entry was ever asked. These pin the episode gate.

// programItemIDs returns the library item ids of the program slots, so a per-EPISODE assertion
// is possible: every episode of a series shares the series Key, so programKeys cannot tell them
// apart — a test written against it would pass while the wrong episodes aired.
func programItemIDs(d schedule.DesiredLineup) []string {
	var out []string
	for _, s := range d.Slots {
		if s.IsProgram() {
			out = append(out, s.LibraryItemID)
		}
	}
	return out
}

func hasID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// The three cases must be distinguishable in ONE fixture, or a green result proves nothing:
// "rated and under the ceiling", "rated ABOVE it", and "not rated at all" have to reach three
// different outcomes, and the unrated one has to be shown airing rather than merely not-crashing.
func TestEnforce_EpisodeRatingBeatsTheSeriesSummary(t *testing.T) {
	// A TV-PG series — it clears the entry gate, exactly as King of the Hill does.
	koth := ratedEntry("series:tmdb:2122", "King of the Hill", "TV-PG", 1997)
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tmdb:2122": {
			{LibraryItemID: "ep-pg", Title: "Pilot", DurationMs: 1_320_000, Season: 1, Episode: 1,
				OfficialRating: "TV-PG"},
			{LibraryItemID: "ep-14", Title: "Wings of the Dope", DurationMs: 1_320_000, Season: 4, Episode: 6,
				OfficialRating: "TV-14"},
			{LibraryItemID: "ep-none", Title: "Unrated Episode", DurationMs: 1_320_000, Season: 5, Episode: 2},
		},
	})
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Audience: schedule.AudiencePolicy{Ceiling: "TV-PG"},
	}}

	d := computeWithPolicy([]schedule.LineupEntry{koth}, avail, p)
	ids := programItemIDs(d)

	if !hasID(ids, "ep-pg") {
		t.Error("a TV-PG episode must air under a TV-PG ceiling")
	}
	if hasID(ids, "ep-14") {
		t.Error("a TV-14 EPISODE must not air under a TV-PG ceiling, even from a TV-PG series")
	}
	// The decision recorded on #260: an episode the media server left unrated INHERITS its
	// parent's rating rather than failing closed. The parent already cleared the ceiling, and
	// 20 of King of the Hill's 275 episodes carry no rating — dropping them would silently
	// remove 7% of an approved show over a metadata gap.
	if !hasID(ids, "ep-none") {
		t.Error("an unrated episode of a below-ceiling series must inherit its parent's rating and air")
	}

	// The drop is COUNTED, not silent — the exclusion report is the only structured record of it.
	if d.Excluded.OverCeiling != 1 || d.Excluded.Unrated != 0 {
		t.Errorf("exclusion report = over:%d unrated:%d, want over:1 unrated:0",
			d.Excluded.OverCeiling, d.Excluded.Unrated)
	}
	// …and names the EPISODE. Keyed by the series key, N drops would otherwise be N identical
	// rows saying only that something in the show was refused.
	if len(d.Excluded.Items) != 1 || !strings.Contains(d.Excluded.Items[0].Title, "S04E06") {
		t.Errorf("excluded item = %+v, want one naming the episode (S04E06)", d.Excluded.Items)
	}
}

// Inheritance must be one-directional: a permissive PARENT can never lift an episode that
// carries its own above-ceiling rating. This is the half that keeps §4's asymmetry intact —
// without it, "inherit when unrated" would decay into "trust the series summary".
func TestEnforce_EpisodeRating_ParentCannotLiftARatedEpisode(t *testing.T) {
	// An unrated series on an ADULT channel (no kids ceiling ⇒ the entry gate admits it).
	show := ratedEntry("series:tmdb:1", "Mixed Bag", "", 1997)
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tmdb:1": {
			{LibraryItemID: "ep-ok", Title: "Fine", DurationMs: 1_320_000, Season: 1, Episode: 1,
				OfficialRating: "TV-14"},
			{LibraryItemID: "ep-ma", Title: "Not Fine", DurationMs: 1_320_000, Season: 1, Episode: 2,
				OfficialRating: "TV-MA"},
		},
	})
	// TV-14 ceiling: NOT a kids ceiling, so unrated resolves to allow — the parent is admitted
	// and its unrated-ness is permissive. The TV-MA episode must still be refused on its own.
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Audience: schedule.AudiencePolicy{Ceiling: "TV-14"},
	}}

	ids := programItemIDs(computeWithPolicy([]schedule.LineupEntry{show}, avail, p))
	if !hasID(ids, "ep-ok") {
		t.Error("a TV-14 episode must air under a TV-14 ceiling")
	}
	if hasID(ids, "ep-ma") {
		t.Fatal("a TV-MA episode aired because its PARENT was permissive — inheritance must be one-way")
	}
}

// A series the audience gate empties resolves to NOTHING — never a pending slot. A pending slot
// means "approved, still acquiring": it advertises the title as coming and, under PodFill, holds
// airtime for it. A show refused by the ceiling is not late, and promising it is the same leak.
func TestEnforce_EpisodeRating_FullyRefusedSeriesIsNotPending(t *testing.T) {
	show := ratedEntry("series:tmdb:2", "All Too Adult", "TV-PG", 1997)
	avail := newSeriesAvail(map[string][]schedule.ResolvedProgram{
		"series:tmdb:2": {
			{LibraryItemID: "e1", Title: "One", DurationMs: 1_320_000, Season: 1, Episode: 1, OfficialRating: "TV-MA"},
			{LibraryItemID: "e2", Title: "Two", DurationMs: 1_320_000, Season: 1, Episode: 2, OfficialRating: "TV-MA"},
		},
	})
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Audience: schedule.AudiencePolicy{Ceiling: "TV-PG"},
	}}

	d := computeWithPolicy([]schedule.LineupEntry{show}, avail, p)
	for _, s := range d.Slots {
		if s.Kind == schedule.SlotPending {
			t.Fatal("a ceiling-refused series became a PENDING slot — it is excluded, not acquiring")
		}
	}
	if d.Excluded.OverCeiling != 2 {
		t.Errorf("exclusion report over-ceiling = %d, want 2 (both episodes)", d.Excluded.OverCeiling)
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

// boxSetEntry builds a movie entry with media-server collection membership STAMPED, as the
// reconcile heal leaves it (programming-design §2.2).
func boxSetEntry(key, title string, boxSets ...string) schedule.LineupEntry {
	e := ratedEntry(key, title, "", 1994)
	e.BoxSetIDs = boxSets
	e.BoxSetsResolved = true
	return e
}

// A collections scope keeps stamped members and drops stamped non-members
// (programming-design §2.2). This replaces the earlier inert-field test, exactly as that
// test's own failure message instructed when the field started binding.
func TestEnforce_CollectionsBind(t *testing.T) {
	entries := []schedule.LineupEntry{
		boxSetEntry("movie:tmdb:1", "In The Collection", "star-trek"),
		boxSetEntry("movie:tmdb:2", "Outside It", "halloween"),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
		Collections: []string{"star-trek"},
	}}}

	keys := programKeys(computeWithPolicy(entries, avail, p))
	if !hasKey(keys, "movie:tmdb:1") {
		t.Errorf("a stamped member of the scoped collection was excluded; got %v", keys)
	}
	if hasKey(keys, "movie:tmdb:2") {
		t.Errorf("a stamped NON-member scheduled anyway — the collections filter is not binding; got %v", keys)
	}
}

// ⚠ An UNRESOLVED entry airs. Membership is stamped by the reconcile heal, so before it has
// answered (media server down, or a channel reconciled once before the resolver was wired)
// BoxSetsResolved is false — and dropping those would let a dependency outage silently empty a
// channel to dead air. Scope is a taste filter; the fail-CLOSED rule belongs to audience (§4),
// and it points the other way. This is the assertion that fails if someone "tidies" boxSetOK
// into treating unresolved as a non-member.
func TestEnforce_CollectionsFailOpenWhenUnresolved(t *testing.T) {
	unresolved := ratedEntry("movie:tmdb:3", "Never Stamped", "", 1994) // BoxSetsResolved stays false
	entries := []schedule.LineupEntry{
		boxSetEntry("movie:tmdb:1", "In The Collection", "star-trek"),
		unresolved,
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:3": "l3"}
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
		Collections: []string{"star-trek"},
	}}}

	keys := programKeys(computeWithPolicy(entries, avail, p))
	if !hasKey(keys, "movie:tmdb:3") {
		t.Errorf("an UNRESOLVED entry was filtered out — a down media server would empty the "+
			"lineup. scope.collections must fail open (§2.2); got %v", keys)
	}
}

// An empty collections scope admits everything, including entries that were never stamped —
// the additive default every other scope field follows. Without this, wiring the resolver
// would change behaviour on channels that use no collection scope at all.
func TestEnforce_CollectionsEmptyScopeAdmitsAll(t *testing.T) {
	entries := []schedule.LineupEntry{
		boxSetEntry("movie:tmdb:1", "In Some Collection", "star-trek"),
		ratedEntry("movie:tmdb:2", "Never Stamped", "", 1994),
	}
	avail := mapAvail{"movie:tmdb:1": "l1", "movie:tmdb:2": "l2"}
	p := schedule.ChannelPolicy{} // no scope at all

	if keys := programKeys(computeWithPolicy(entries, avail, p)); len(keys) != 2 {
		t.Errorf("an empty collections scope filtered something (%d of 2 scheduled); got %v", len(keys), keys)
	}
}
