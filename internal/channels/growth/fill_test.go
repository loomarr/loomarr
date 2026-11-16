package growth_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
	"github.com/loomarr/loomarr/internal/testkit"
)

var fillNow = time.Date(2026, 11, 16, 0, 5, 0, 0, time.UTC)

// libraryShow saves a series of n playable episodes under a TVDB id.
func libraryShow(t *testing.T, st store.Store, tvdb, year int, rating string, n int, genres ...string) {
	t.Helper()
	ctx := context.Background()
	id := fmt.Sprintf("show-%d", tvdb)
	items := []librarystore.LibraryItem{{ID: id, Kind: "series", Name: fmt.Sprintf("Show %d", tvdb), Year: year, Rating: rating,
		Genres: genres, IDs: []librarystore.LibraryID{{Namespace: "tvdb", Value: fmt.Sprint(tvdb)}}}}
	var files []librarystore.LibraryFile
	for e := range n {
		ep := fmt.Sprintf("%s-e%d", id, e)
		items = append(items, librarystore.LibraryItem{ID: ep, Kind: "episode", Name: ep, RuntimeMs: 22 * 60_000})
		files = append(files, librarystore.LibraryFile{Path: "/t/" + ep + ".mkv", Root: "/t", Size: 1, ModifiedNs: 1, Kind: "episode",
			Header: librarystore.LibraryHeaderRead, SeenAt: fillNow, ItemID: ep, SeriesID: id})
	}
	if err := st.SaveLibraryItems(ctx, items, fillNow); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
}

func seed(tvdb, year int) schedule.LineupEntry {
	return schedule.LineupEntry{Key: provision.Key(fmt.Sprintf("series:tvdb:%d", tvdb)), Title: fmt.Sprintf("Show %d", tvdb),
		Year: year, OfficialRating: "TV-PG", Genres: []string{"Comedy", "Family"}}
}

// fillHarness is the real store, workflow, approver and binder: the fill writes nothing itself.
func fillHarness(t *testing.T, policy schedule.ChannelPolicy) (store.Store, *growth.Fill) {
	t.Helper()
	st := testkit.MigratedSQLiteStore(t)
	libraryShow(t, st, 90000701, 1989, "TV-PG", 30, "Comedy", "Family")
	libraryShow(t, st, 90000702, 1992, "TV-PG", 30, "Comedy", "Family")
	libraryShow(t, st, 90000703, 1990, "TV-PG", 20, "Comedy", "Family") // a clear fit
	libraryShow(t, st, 90000704, 1991, "TV-MA", 20, "Comedy", "Family") // above the seeds' rating
	libraryShow(t, st, 90000705, 2016, "TV-PG", 20, "Comedy", "Family") // outside the years
	libraryShow(t, st, 90000706, 1990, "TV-PG", 20, "Drama")            // outside the genres
	ch := channelstore.Channel{ID: "ch-grow", Name: "Grow", Number: 7, Strategy: schedule.Shuffle, Status: schedule.StatusLive, FromRequest: true,
		Lineup: []schedule.LineupEntry{seed(90000701, 1989), seed(90000702, 1992)}, Policy: policy}
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	log := testkit.Logger()
	ids := 0
	newID := func() string { ids++; return fmt.Sprintf("id-%d", ids) }
	clock := func() time.Time { return fillNow }
	wf := workflow.New(st, newID, clock, suggest.IntentHash)
	approver := accept.NewApprover(st, approval.New(st, nil, nil, log), clock)
	return st, growth.NewFill(st, wf, approver, clock, log)
}

func lineupOf(t *testing.T, st store.Store) channelstore.Channel {
	t.Helper()
	ch, err := st.GetChannel(context.Background(), "ch-grow")
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

// A short channel takes the library title inside its line as one batch, through the binder: the
// seeds stay seeds and in place, the channel's request is unchanged, and nothing the line or the
// ceiling refuses joins. A second night with nothing new to add changes nothing.
func TestTheNightlyFillAddsWhatTheLineAdmitsAsOneBatch(t *testing.T) {
	st, fill := fillHarness(t, schedule.ChannelPolicy{})
	if grown, err := fill.Run(context.Background()); err != nil || grown != 1 {
		t.Fatalf("grown %d, err %v; want one channel", grown, err)
	}
	ch := lineupOf(t, st)
	var keys []provision.Key
	for _, e := range ch.Lineup {
		keys = append(keys, e.Key)
	}
	want := []provision.Key{"series:tvdb:90000701", "series:tvdb:90000702", "series:tvdb:90000703"}
	if !slices.Equal(keys, want) {
		t.Fatalf("lineup %v, want the seeds then the clear fit", keys)
	}
	if all, _ := st.ListChannels(context.Background()); len(all) != 1 {
		t.Fatalf("%d channels, want the growth batch bound to the one channel", len(all))
	}
	if ch.Lineup[0].Grown != "" || ch.Lineup[1].Grown != "" || ch.Lineup[2].Grown != "growth-2026-11-16" {
		t.Errorf("batches %q %q %q, want two seeds and one grown title", ch.Lineup[0].Grown, ch.Lineup[1].Grown, ch.Lineup[2].Grown)
	}
	if ch.IntentJSON != "" {
		t.Errorf("a channel with no stored request gained one from a growth batch: %q", ch.IntentJSON)
	}

	if grown, err := fill.Run(context.Background()); err != nil || grown != 0 {
		t.Fatalf("second night grew %d channels (err %v), want none", grown, err)
	}
	if again := lineupOf(t, st); len(again.Lineup) != 3 {
		t.Errorf("second night lineup has %d titles, want 3", len(again.Lineup))
	}
}

// Taking a grown title off by hand is the undo: the next night leaves it off.
func TestAGrownTitleTakenOffByHandStaysOff(t *testing.T) {
	st, fill := fillHarness(t, schedule.ChannelPolicy{})
	ctx := context.Background()
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	editor := &edit.Service{Store: st, Log: testkit.Logger()}
	if _, err := editor.Edit(ctx, edit.Backend{}, "ch-grow", 0, func(ch *channelstore.Channel) error {
		ch.Lineup = ch.Lineup[:2]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !lineupOf(t, st).Policy.Declined("series:tvdb:90000703") {
		t.Fatal("taking the grown title off did not record it as declined")
	}
	if grown, err := fill.Run(ctx); err != nil || grown != 0 {
		t.Fatalf("the next night grew %d channels (err %v), want none", grown, err)
	}
	if n := len(lineupOf(t, st).Lineup); n != 2 {
		t.Errorf("the title taken off rejoined: %d titles, want 2", n)
	}
}

// A channel kept as it is takes nothing, and a title a person took back off never rejoins.
func TestAKeptChannelAndADeclinedTitleTakeNothing(t *testing.T) {
	st, fill := fillHarness(t, schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Growth: &schedule.GrowthPolicy{Keep: true}}})
	if _, err := fill.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(lineupOf(t, st).Lineup); n != 2 {
		t.Errorf("a kept channel has %d titles, want its 2 seeds", n)
	}

	st, fill = fillHarness(t, schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{
		Growth: &schedule.GrowthPolicy{Declined: []provision.Key{"series:tvdb:90000703"}}}})
	if _, err := fill.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(lineupOf(t, st).Lineup); n != 2 {
		t.Errorf("the declined title rejoined: %d titles, want 2", n)
	}
}
