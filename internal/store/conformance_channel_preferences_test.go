package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func mustUpsertPreferenceUser(t *testing.T, s Store, id string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.UpsertUser(context.Background(), User{ID: id, Name: id, Role: RoleMember,
		CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func favouriteIDs(lists UserChannelLists) []string {
	out := make([]string, 0, len(lists.Favourites))
	for _, f := range lists.Favourites {
		out = append(out, f.ChannelID)
	}
	return out
}

func recentIDs(lists UserChannelLists) []string {
	out := make([]string, 0, len(lists.Recent))
	for _, r := range lists.Recent {
		out = append(out, r.ChannelID)
	}
	return out
}

func testFavouriteChannels(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustUpsertPreferenceUser(t, s, "u-ada")
	mustUpsertPreferenceUser(t, s, "u-bo")
	for i, id := range []string{"fav-a", "fav-b", "fav-c"} {
		mustSaveChannel(t, s, sampleChannel(id, 40+i, time.Now().Add(time.Hour)))
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	for i, id := range []string{"fav-b", "fav-a"} {
		if err := s.AddFavouriteChannel(ctx, "u-ada", id, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// Adding again is idempotent and keeps the original added time.
	if err := s.AddFavouriteChannel(ctx, "u-ada", "fav-b", base.Add(time.Hour)); err != nil {
		t.Fatalf("re-add = %v, want idempotent success", err)
	}
	if err := s.AddFavouriteChannel(ctx, "u-bo", "fav-c", base); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFavouriteChannel(ctx, "u-ada", "missing-channel", base); !errors.Is(err, ErrNotFound) {
		t.Fatalf("favourite a missing channel = %v, want ErrNotFound", err)
	}

	ada, err := s.UserChannelLists(ctx, "u-ada")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(favouriteIDs(ada)); got != "[fav-b fav-a]" {
		t.Fatalf("u-ada favourites = %s, want [fav-b fav-a] in the order they were added", got)
	}
	if !ada.Favourites[0].AddedAt.Equal(base) {
		t.Fatalf("re-add moved addedAt to %v, want %v", ada.Favourites[0].AddedAt, base)
	}
	bo, err := s.UserChannelLists(ctx, "u-bo")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(favouriteIDs(bo)); got != "[fav-c]" {
		t.Fatalf("u-bo favourites = %s, want only their own [fav-c]", got)
	}

	if err := s.RemoveFavouriteChannel(ctx, "u-ada", "fav-b"); err != nil {
		t.Fatal(err)
	}
	// Removing what isn't there is not an error: the caller's intent already holds.
	if err := s.RemoveFavouriteChannel(ctx, "u-ada", "fav-b"); err != nil {
		t.Fatalf("second remove = %v, want nil", err)
	}
	saved, err := s.GetChannel(ctx, "fav-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteChannel(ctx, "fav-a", saved.Revision); err != nil {
		t.Fatal(err)
	}
	ada, err = s.UserChannelLists(ctx, "u-ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(ada.Favourites) != 0 {
		t.Fatalf("u-ada favourites after remove + channel delete = %v, want none", favouriteIDs(ada))
	}
}

func testRecentChannels(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustUpsertPreferenceUser(t, s, "u-ada")
	mustUpsertPreferenceUser(t, s, "u-bo")
	total := RecentChannelsKept + 3
	for i := range total {
		mustSaveChannel(t, s, sampleChannel(fmt.Sprintf("recent-%02d", i), 100+i, time.Now().Add(time.Hour)))
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	for i := range total {
		if err := s.RecordChannelTune(ctx, "u-ada", fmt.Sprintf("recent-%02d", i), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// Re-tuning an old channel moves it to the front.
	retuned := base.Add(time.Duration(total) * time.Minute)
	if err := s.RecordChannelTune(ctx, "u-ada", "recent-05", retuned); err != nil {
		t.Fatal(err)
	}
	// A late, older tune (a slow device) never moves a channel backwards.
	if err := s.RecordChannelTune(ctx, "u-ada", "recent-05", base); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordChannelTune(ctx, "u-bo", "recent-00", base); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordChannelTune(ctx, "u-ada", "missing-channel", base); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tune a missing channel = %v, want ErrNotFound", err)
	}

	ada, err := s.UserChannelLists(ctx, "u-ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(ada.Recent) != RecentChannelsKept {
		t.Fatalf("u-ada keeps %d recents, want the newest %d", len(ada.Recent), RecentChannelsKept)
	}
	if ada.Recent[0].ChannelID != "recent-05" || !ada.Recent[0].TunedAt.Equal(retuned) {
		t.Fatalf("newest recent = %+v, want recent-05 at %v", ada.Recent[0], retuned)
	}
	if ada.Recent[1].ChannelID != fmt.Sprintf("recent-%02d", total-1) {
		t.Fatalf("second recent = %s, want the last tuned before the re-tune", ada.Recent[1].ChannelID)
	}
	for i := 1; i < len(ada.Recent); i++ {
		if ada.Recent[i].TunedAt.After(ada.Recent[i-1].TunedAt) {
			t.Fatalf("recents out of order at %d: %v", i, recentIDs(ada))
		}
	}
	for _, r := range ada.Recent {
		if r.ChannelID == "recent-00" || r.ChannelID == "recent-01" || r.ChannelID == "recent-02" {
			t.Fatalf("the oldest tunes survived pruning: %v", recentIDs(ada))
		}
	}
	bo, err := s.UserChannelLists(ctx, "u-bo")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(recentIDs(bo)); got != "[recent-00]" {
		t.Fatalf("u-bo recents = %s, want only their own [recent-00]; pruning another user's list must not touch it", got)
	}

	saved, err := s.GetChannel(ctx, "recent-05")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteChannel(ctx, "recent-05", saved.Revision); err != nil {
		t.Fatal(err)
	}
	ada, err = s.UserChannelLists(ctx, "u-ada")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range ada.Recent {
		if r.ChannelID == "recent-05" {
			t.Fatalf("a deleted channel stayed in recents: %v", recentIDs(ada))
		}
	}
}
