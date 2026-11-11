package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/proposals"
)

// A channel's planned items and the collections its scope and rules name are rows, written with the
// channel (#2793): a name the library has no row for refuses the save, the rows follow every save,
// a collection a channel names can't be deleted, and the catalog purge keeps a planned item. Open
// proposals' picks count; a decided proposal's are history.
func testLibraryReferences(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.Unix(1_800_000_000, 0).UTC()
	saveLibraryItems(t, st, "90000801", "90000802", "90000803", "90000804")
	night, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "Night", Members: []string{"90000801"}}, at)
	if err != nil {
		t.Fatal(err)
	}
	home, err := st.CreateLibraryCollection(ctx, LibraryCollection{Name: "Home", Members: []string{"90000802"}}, at)
	if err != nil {
		t.Fatal(err)
	}

	ch := sampleChannel("ch-1", 7, time.Time{})
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "90000801", DurationMs: 1},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "90000801", DurationMs: 1},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000002", LibraryItemID: "90000802", DurationMs: 1},
		{Kind: schedule.SlotFiller, DurationMs: 1},
	}
	ch.Policy.Scope.Collections = []string{night}
	ch.Policy.Rules = []schedule.SchedulingRule{{What: &schedule.ScopePolicy{Collections: []string{home, night}}}}
	saved, err := st.SaveChannel(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}

	sqlSt := st.(*sqlStore)
	strings := func(query string) []string {
		t.Helper()
		got, err := sqlSt.distinctStrings(ctx, sqlSt.db, query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		slices.Sort(got)
		return got
	}
	if got, want := strings(`SELECT library_item_id FROM channel_planned_items WHERE channel_id = 'ch-1'`),
		[]string{"90000801", "90000802"}; !slices.Equal(got, want) {
		t.Errorf("planned items = %v, want %v", got, want)
	}
	if got, err := st.GetChannel(ctx, "ch-1"); err != nil || len(got.Desired) != 4 || got.Desired[2].LibraryItemID != "90000802" {
		t.Errorf("schedule read back = %+v, %v; want each slot's library item kept", got.Desired, err)
	}
	naming, err := st.ChannelsNamingCollections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(naming[night], []string{"ch-1"}) || !slices.Equal(naming[home], []string{"ch-1"}) || len(naming) != 2 {
		t.Errorf("channels naming collections = %v, want ch-1 for both", naming)
	}

	// A collection a channel names can't be deleted.
	if err := st.DeleteLibraryCollection(ctx, home); err == nil {
		t.Error("deleted a collection a channel's rule names")
	}

	// The rows follow the next save.
	saved.Desired = saved.Desired[2:]
	saved.Policy.Rules = nil
	if saved, err = st.SaveChannel(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if got := strings(`SELECT library_item_id FROM channel_planned_items WHERE channel_id = 'ch-1'`); !slices.Equal(got, []string{"90000802"}) {
		t.Errorf("planned items after the save = %v, want [90000802]", got)
	}
	if naming, err = st.ChannelsNamingCollections(ctx); err != nil || len(naming) != 1 || naming[home] != nil {
		t.Errorf("channels naming collections after the save = %v, %v; want only %s", naming, err, night)
	}
	if err := st.DeleteLibraryCollection(ctx, home); err != nil {
		t.Errorf("delete a collection no channel names any more: %v", err)
	}

	// A name the library has no row for refuses the save, which writes nothing.
	for _, c := range []struct {
		what   string
		change func(*Channel)
	}{
		{"planned item", func(c *Channel) {
			c.Desired = append(c.Desired, schedule.Slot{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000003",
				LibraryItemID: "90000899", DurationMs: 1})
		}},
		{"scope collection", func(c *Channel) { c.Policy.Scope.Collections = []string{"90000898"} }},
		{"rule collection", func(c *Channel) {
			c.Policy.Rules = []schedule.SchedulingRule{{What: &schedule.ScopePolicy{Collections: []string{"90000897"}}}}
		}},
	} {
		bad := saved
		bad.Desired = slices.Clone(saved.Desired)
		c.change(&bad)
		if _, err := st.SaveChannel(ctx, bad); !errors.Is(err, ErrUnknownLibraryReference) {
			t.Errorf("save naming an unknown %s = %v, want ErrUnknownLibraryReference", c.what, err)
		}
	}
	if got, err := st.GetChannel(ctx, "ch-1"); err != nil || got.Revision != saved.Revision {
		t.Errorf("channel after refused saves = revision %d, %v; want %d", got.Revision, err, saved.Revision)
	}

	// Deleting the channel takes its rows.
	if err := st.DeleteChannel(ctx, "ch-1", saved.Revision); err != nil {
		t.Fatal(err)
	}
	if got := strings(`SELECT library_item_id FROM channel_planned_items`); len(got) != 0 {
		t.Errorf("planned items of a deleted channel = %v", got)
	}
	if naming, err = st.ChannelsNamingCollections(ctx); err != nil || len(naming) != 0 {
		t.Errorf("channels naming collections after delete = %v, %v", naming, err)
	}

	// Open proposals' picks; a decided proposal's are history.
	pick := func(id string) proposals.ProposalItem {
		return proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000001, Name: "Film", LibraryItemID: id}
	}
	for _, p := range []Proposal{
		{ID: "open", Status: "submitted", CreatedAt: at, Body: proposals.Proposal{
			Lineup: []proposals.ProposalItem{pick("90000803")}, Alternates: []proposals.ProposalItem{pick("90000801")},
			Refused: []proposals.RefusedPick{{Item: pick("90000804"), Reason: "over_ceiling"}},
		}},
		{ID: "decided", Status: "approved", CreatedAt: at, Body: proposals.Proposal{
			Lineup: []proposals.ProposalItem{pick("90000802")},
		}},
	} {
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	picks, err := sqlSt.openProposalItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"90000801", "90000803", "90000804"}; !slices.Equal(picks, want) {
		t.Errorf("open proposal picks = %v, want %v (a decided proposal's are history)", picks, want)
	}
}
