package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// Home's New this week (#1663): titles that arrived since a time, with the channels they now play
// on, and each channel's creation time and requester.
func TestNewThisWeekOverHTTP(t *testing.T) {
	t.Parallel()
	h := newAPIHarness(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }

	title := func(tmdb int, state provision.State, arrived time.Time) provision.Record {
		t := provision.Title{MediaType: provision.Movie, TMDBID: tmdb, Name: fmt.Sprintf("Film %d", tmdb)}
		key, _ := t.Key()
		return provision.Record{Key: key, Title: t, State: state, AvailableAt: arrived}
	}
	arrived, older := title(1, provision.Available, days(1)), title(2, provision.Available, days(10))
	picked, inFlight := title(3, provision.Available, time.Time{}), title(4, provision.Downloading, time.Time{})
	for _, r := range []provision.Record{arrived, older, picked, inFlight} {
		if err := h.Store.UpsertTitle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Store.UpsertUser(ctx, store.User{ID: "u-member", Name: "Member One", Role: store.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.CreateJob(ctx, store.Job{ID: "job-1", Kind: "suggest", Status: "done", CreatedBy: "u-member"}); err != nil {
		t.Fatal(err)
	}
	channel := func(id string, number int, intentRef string, created time.Time, status schedule.ChannelStatus, keys ...provision.Key) {
		ch := store.Channel{Channel: schedule.Channel{ID: id, Name: "Channel " + id, Number: number, IntentRef: intentRef,
			Strategy: schedule.Shuffle, Status: status}, CreatedAt: created}
		for _, k := range keys {
			ch.Lineup = append(ch.Lineup, schedule.LineupEntry{Key: k})
		}
		if _, err := h.Store.SaveChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	channel("ch-old", 4, "", days(30), schedule.StatusLive, arrived.Key)
	channel("ch-new", 2, "job-1", time.Time{}, schedule.StatusLive, arrived.Key, picked.Key)
	channel("ch-gone", 6, "", days(30), schedule.StatusDetached, arrived.Key)

	var titles struct {
		Titles []struct {
			Key           string `json:"key"`
			State         string `json:"state"`
			AvailableAtMs int64  `json:"availableAtMs"`
			Channels      []struct {
				ID     string `json:"id"`
				Number int    `json:"number"`
			} `json:"channels"`
		} `json:"titles"`
	}
	path := fmt.Sprintf("/v1/titles?since=%d", days(7).UnixMilli())
	decodeOK(t, h.Do(http.MethodGet, path, memberToken, ""), &titles)
	var got []string
	for _, tt := range titles.Titles {
		var chans []string
		for _, c := range tt.Channels {
			chans = append(chans, fmt.Sprintf("%s#%d", c.ID, c.Number))
		}
		got = append(got, fmt.Sprintf("%s %s %s on [%s]", tt.Key, tt.State,
			now.Sub(time.UnixMilli(tt.AvailableAtMs)), strings.Join(chans, " ")))
	}
	// Only the arrival in the window; the in-library pick never arrived. A detached channel no longer
	// plays it.
	if want := "movie:tmdb:1 available 24h0m0s on [ch-new#2 ch-old#4]"; strings.Join(got, " | ") != want {
		t.Fatalf("new this week =\n %s\nwant\n %s", strings.Join(got, " | "), want)
	}

	// `since` asks about arrivals: only an available title arrives.
	if res := h.Do(http.MethodGet, path+"&state=downloading", memberToken, ""); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("since with state=downloading = %d, want 400", res.StatusCode)
	}
	if res := h.Do(http.MethodGet, path, "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", res.StatusCode)
	}

	var channels struct {
		Channels []struct {
			ID          string `json:"id"`
			CreatedAtMs int64  `json:"createdAtMs"`
			RequestedBy string `json:"requestedBy"`
		} `json:"channels"`
	}
	decodeOK(t, h.Do(http.MethodGet, "/v1/channels", memberToken, ""), &channels)
	got = nil
	for _, c := range channels.Channels {
		age := "unknown"
		if c.CreatedAtMs > 0 {
			age = now.Sub(time.UnixMilli(c.CreatedAtMs)).Round(time.Hour).String()
		}
		got = append(got, fmt.Sprintf("%s created %s ago by %q", c.ID, age, c.RequestedBy))
	}
	want := `ch-new created 0s ago by "Member One" | ch-old created 720h0m0s ago by "" | ch-gone created 720h0m0s ago by ""`
	if strings.Join(got, " | ") != want {
		t.Fatalf("channels =\n %s\nwant\n %s", strings.Join(got, " | "), want)
	}
}

func decodeOK(t *testing.T, res *http.Response, into any) {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}
