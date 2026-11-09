package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func viewingIDs(vs []Viewing) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.ID)
	}
	return out
}

func sameIDs(got []string, want ...string) bool { return slices.Equal(got, want) }

// testViewings: the watch history (#3010) pages newest first, filters by person and by the span a
// viewing overlaps, extends a viewing's end without ever moving it back, and clears per person or
// for everyone.
func testViewings(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustUpsertPreferenceUser(t, s, "u-sam")
	mustUpsertPreferenceUser(t, s, "u-alex")
	base := time.Unix(1_800_000_000, 0).UTC()
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }

	for _, v := range []Viewing{
		{ID: "v1", UserID: "u-sam", Device: "Living room TV", ChannelID: "ch-cartoons", ChannelName: "Cartoons",
			Programme: "Bluey", StartedAt: at(0), EndedAt: at(10)},
		{ID: "v2", UserID: "u-alex", Device: "Firefox on macOS", ChannelID: "ch-movies", ChannelName: "Movies",
			StartedAt: at(20), EndedAt: at(30)},
		{ID: "v3", UserID: "u-sam", Device: "Living room TV", ChannelID: "ch-movies", ChannelName: "Movies",
			Programme: "Up", StartedAt: at(40), EndedAt: at(40)},
	} {
		if err := s.InsertViewing(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.InsertViewing(ctx, Viewing{ID: "bad", UserID: "u-sam"}); err == nil {
		t.Error("a viewing with no channel or start was accepted")
	}

	all, err := s.ListViewings(ctx, ViewingQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := viewingIDs(all); !sameIDs(got, "v3", "v2", "v1") {
		t.Fatalf("history = %v, want newest first v3 v2 v1", got)
	}
	if v := all[2]; v.Device != "Living room TV" || v.ChannelName != "Cartoons" || v.Programme != "Bluey" ||
		!v.StartedAt.Equal(at(0)) || !v.EndedAt.Equal(at(10)) {
		t.Errorf("v1 read back as %+v", v)
	}

	page, err := s.ListViewings(ctx, ViewingQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	last := page[len(page)-1]
	next, err := s.ListViewings(ctx, ViewingQuery{Limit: 2, AfterStartedAt: last.StartedAt, AfterID: last.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := append(viewingIDs(page), viewingIDs(next)...); !sameIDs(got, "v3", "v2", "v1") {
		t.Errorf("two pages = %v, want v3 v2 | v1", got)
	}

	sam, _ := s.ListViewings(ctx, ViewingQuery{Limit: 10, UserID: "u-sam"})
	if got := viewingIDs(sam); !sameIDs(got, "v3", "v1") {
		t.Errorf("u-sam's history = %v, want v3 v1", got)
	}
	// v1 ends at 10 and v2 starts at 20: a window of [5, 25) overlaps both, not v3.
	window, _ := s.ListViewings(ctx, ViewingQuery{Limit: 10, From: at(5), To: at(25)})
	if got := viewingIDs(window); !sameIDs(got, "v2", "v1") {
		t.Errorf("window [5,25) = %v, want v2 v1", got)
	}

	for _, step := range []struct {
		id, programme string
		end           int
	}{{"v3", "", 55}, {"v3", "", 45}, {"v2", "Paddington", 31}} {
		if found, err := s.ExtendViewing(ctx, step.id, step.programme, at(step.end)); err != nil || !found {
			t.Fatalf("extend %s = %v, %v; want found", step.id, found, err)
		}
	}
	if found, err := s.ExtendViewing(ctx, "missing", "", at(60)); err != nil || found {
		t.Errorf("extending a cleared viewing = %v, %v; want not found and no error", found, err)
	}
	all, _ = s.ListViewings(ctx, ViewingQuery{Limit: 10})
	if v := all[0]; !v.EndedAt.Equal(at(55)) || v.Programme != "Up" {
		t.Errorf("v3 after extends = end %v programme %q, want end at 55 and Up kept", v.EndedAt, v.Programme)
	}
	if v := all[1]; v.Programme != "Paddington" || !v.EndedAt.Equal(at(31)) {
		t.Errorf("v2 after extend = %+v, want programme Paddington ending at 31", v)
	}

	n, err := s.DeleteViewings(ctx, "u-sam")
	if err != nil || n != 2 {
		t.Fatalf("clear u-sam = %d, %v; want 2 rows", n, err)
	}
	all, _ = s.ListViewings(ctx, ViewingQuery{Limit: 10})
	if got := viewingIDs(all); !sameIDs(got, "v2") {
		t.Errorf("after clearing u-sam = %v, want only u-alex's v2", got)
	}
	if n, err := s.DeleteViewings(ctx, ""); err != nil || n != 1 {
		t.Errorf("clear all = %d, %v; want 1 row", n, err)
	}
	if all, _ = s.ListViewings(ctx, ViewingQuery{Limit: 10}); len(all) != 0 {
		t.Errorf("after clearing everyone %d rows remain", len(all))
	}
	if _, err := s.ListViewings(ctx, ViewingQuery{}); err == nil {
		t.Error("a zero limit was accepted")
	}
}
