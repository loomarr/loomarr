package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// The ids stored records name, and their rewrite: titles, airings (two ids for one item keep one
// row per airing), a channel's slots, membership stamps, scope and rules, and open proposals; a
// decided proposal is history, and the media server's caches go.
func testLibraryReferences(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.Unix(1_800_000_000, 0).UTC()

	film := sampleRecord("movie:tmdb:90000001", provision.Available, time.Time{})
	film.LibraryID = "emby-film"
	show := sampleRecord("series:tvdb:90000002", provision.Available, time.Time{})
	show.Title = provision.Title{MediaType: provision.Series, TVDBID: 90000002, Name: "Show"}
	show.LibraryID = "emby-show"
	home := sampleRecord("movie:tmdb:90000003", provision.Available, time.Time{})
	home.LibraryID = "lib-home" // already a library id
	for _, r := range []provision.Record{film, show, home} {
		if err := st.UpsertTitle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	ch := sampleChannel("ch-1", 7, time.Time{})
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: film.Key, LibraryItemID: "emby-film", DurationMs: 1},
		{Kind: schedule.SlotProgram, Key: show.Key, LibraryItemID: "emby-ep", DurationMs: 1},
	}
	ch.Lineup[0].BoxSetIDs, ch.Lineup[0].BoxSetsResolved = []string{"boxset-1"}, true
	ch.Policy.Scope.Collections = []string{"boxset-1", "own-1"}
	ch.Policy.Rules = []schedule.SchedulingRule{{What: &schedule.ScopePolicy{Collections: []string{"boxset-2"}}}}
	saved, err := st.SaveChannel(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct {
		item   string
		airedS int64
	}{{"emby-film", 100}, {"emby-film-alt", 100}, {"emby-film-alt", 200}, {"emby-ep", 300}} {
		airedAt := time.Unix(1_800_000_000+a.airedS, 0)
		if err := st.RecordAiring(ctx, "ch-1", film.Key, a.item, airedAt, airedAt); err != nil {
			t.Fatal(err)
		}
	}

	pick := func(id string) proposals.ProposalItem {
		return proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000001, Name: "Film", LibraryItemID: id}
	}
	for _, p := range []Proposal{
		{ID: "open", Status: "submitted", CreatedAt: at, Body: proposals.Proposal{
			Lineup: []proposals.ProposalItem{pick("emby-film")}, Alternates: []proposals.ProposalItem{pick("emby-show")},
			Refused: []proposals.RefusedPick{{Item: pick("emby-film-alt"), Reason: "over_ceiling"}},
			Policy:  schedule.ChannelPolicy{Scope: schedule.ScopePolicy{Collections: []string{"boxset-2"}}},
		}},
		{ID: "decided", Status: "approved", CreatedAt: at, Body: proposals.Proposal{
			Lineup: []proposals.ProposalItem{pick("emby-old")},
		}},
	} {
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	refs, err := st.LibraryReferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"emby-ep", "emby-film", "emby-film-alt", "emby-show", "lib-home"}; !slices.Equal(refs.Items, want) {
		t.Errorf("items = %v, want %v (a decided proposal's are history)", refs.Items, want)
	}
	if want := []string{"boxset-1", "boxset-2", "own-1"}; !slices.Equal(refs.Collections, want) {
		t.Errorf("collections = %v, want %v", refs.Collections, want)
	}

	items := map[string]string{
		"emby-film": "lib-film", "emby-film-alt": "lib-film", "emby-show": "lib-show", "emby-ep": "lib-ep",
		"emby-old": "lib-old",
	}
	collections := map[string]string{"boxset-1": "own-9", "boxset-2": "own-8"}
	if _, err := st.RewriteLibraryReferences(ctx, items, collections); err != nil {
		t.Fatal(err)
	}

	refs, err = st.LibraryReferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lib-ep", "lib-film", "lib-home", "lib-show"}; !slices.Equal(refs.Items, want) {
		t.Errorf("items after = %v, want %v", refs.Items, want)
	}
	if want := []string{"own-1", "own-8", "own-9"}; !slices.Equal(refs.Collections, want) {
		t.Errorf("collections after = %v, want %v", refs.Collections, want)
	}
	if got, _ := st.GetTitle(ctx, show.Key); got.LibraryID != "lib-show" {
		t.Errorf("show library id = %q", got.LibraryID)
	}
	got, err := st.GetChannel(ctx, "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != saved.Revision+1 || got.Desired[0].LibraryItemID != "lib-film" || got.Lineup[0].BoxSetIDs[0] != "own-9" ||
		!slices.Equal(got.Policy.Scope.Collections, []string{"own-9", "own-1"}) || got.Policy.Rules[0].What.Collections[0] != "own-8" {
		t.Errorf("channel after = rev %d %+v %+v %+v", got.Revision, got.Desired, got.Lineup[0].BoxSetIDs, got.Policy)
	}
	last, err := st.LastAiredByKey(ctx, "ch-1")
	if err != nil || len(last) != 1 {
		t.Errorf("airings after = %v, %v", last, err)
	}
	if decided, err := st.GetProposal(ctx, "decided"); err != nil || decided.Body.Lineup[0].LibraryItemID != "emby-old" {
		t.Errorf("a decided proposal was rewritten: %+v, %v", decided.Body.Lineup, err)
	}
	if open, err := st.GetProposal(ctx, "open"); err != nil || open.Body.Refused[0].Item.LibraryItemID != "lib-film" ||
		open.Body.Policy.Scope.Collections[0] != "own-8" {
		t.Errorf("open proposal after = %+v, %v", open.Body, err)
	}
	// Run again: nothing left to change.
	if n, err := st.RewriteLibraryReferences(ctx, items, collections); err != nil || n != 0 {
		t.Errorf("a second rewrite changed %d, %v; want 0", n, err)
	}
}
