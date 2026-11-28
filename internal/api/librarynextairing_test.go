package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

type nextAiringsBody struct {
	NextAirings []struct {
		Channel struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Number int    `json:"number"`
		} `json:"channel"`
		Start    time.Time `json:"start"`
		End      time.Time `json:"end"`
		Forecast bool      `json:"forecast"`
		Episode  *struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Season int    `json:"season"`
			Number int    `json:"number"`
		} `json:"episode"`
	} `json:"nextAirings"`
}

const (
	nextMovieKey  = provision.Key("movie:tmdb:90000001")
	nextSeriesKey = provision.Key("series:tvdb:90000002")
)

// nextAiringHarness is the member library with its movie and series named by title keys, so a
// series is found through its key.
func nextAiringHarness(t *testing.T) (*apiHarness, time.Time) {
	t.Helper()
	h := newLibraryHarness(t)
	seedMemberLibrary(t, h.Store)
	ctx := context.Background()
	for _, r := range []provision.Record{
		{Key: nextMovieKey, State: provision.Available, LibraryID: "90000001",
			Title: provision.Title{MediaType: provision.Movie, TMDBID: 90000001, Name: "Alpha Movie"}},
		{Key: nextSeriesKey, State: provision.Available, LibraryID: "90000002",
			Title: provision.Title{MediaType: provision.Series, TVDBID: 90000002, Name: "Bravo Show"}},
	} {
		if err := h.Store.UpsertTitle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	return h, time.Now().Truncate(time.Second)
}

func airOn(t *testing.T, h *apiHarness, id string, number int, status schedule.ChannelStatus, airings ...channeldatastore.UpcomingAiring) {
	t.Helper()
	ctx := context.Background()
	ch, err := h.Store.SaveChannel(ctx, channelstore.Channel{Channel: schedule.Channel{
		ID: id, Name: "Channel " + id, Number: number, Strategy: schedule.Shuffle, Status: status,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if wrote, err := h.Store.ReplaceUpcomingAirings(ctx, ch.ID, ch.Revision, airings); err != nil || !wrote {
		t.Fatalf("store airings on %s: wrote %v, %v", id, wrote, err)
	}
}

func nextAirings(t *testing.T, h *apiHarness, id, token string) (nextAiringsBody, string) {
	t.Helper()
	resp := h.Do(http.MethodGet, "/v1/library/items/"+id, token, "")
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("page %s = %d %s %v", id, resp.StatusCode, raw, err)
	}
	var out nextAiringsBody
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, string(raw)
}

func TestLibraryPageSaysWhenAMovieNextAirsOnEachChannel(t *testing.T) {
	h, now := nextAiringHarness(t)
	at := func(min int) time.Time { return now.Add(time.Duration(min) * time.Minute) }
	movie := func(from, to int, forecast bool) channeldatastore.UpcomingAiring {
		return channeldatastore.UpcomingAiring{Start: at(from), End: at(to), LibraryItemID: "90000001",
			TitleKey: nextMovieKey, Forecast: forecast}
	}
	// The lower-numbered channel airs it later; the other has it on air now.
	airOn(t, h, "later", 1, schedule.StatusLive, movie(120, 210, true), movie(600, 690, true))
	airOn(t, h, "now", 2, schedule.StatusLive, movie(-200, -110, false), movie(-10, 80, false), movie(300, 390, true))
	airOn(t, h, "paused", 3, schedule.StatusPaused, movie(10, 100, false))

	for _, token := range []string{adminToken, memberToken} {
		got, _ := nextAirings(t, h, "90000001", token)
		if len(got.NextAirings) != 2 {
			t.Fatalf("%s next airings = %+v, want one each on now and later", token, got.NextAirings)
		}
		first, second := got.NextAirings[0], got.NextAirings[1]
		if first.Channel.ID != "now" || first.Channel.Name != "Channel now" || first.Channel.Number != 2 ||
			!first.Start.Equal(at(-10)) || !first.End.Equal(at(80)) || first.Forecast || first.Episode != nil {
			t.Errorf("first = %+v, want the programme on air on channel 2", first)
		}
		if second.Channel.ID != "later" || !second.Start.Equal(at(120)) || !second.Forecast || second.Episode != nil {
			t.Errorf("second = %+v, want channel 1's forecast", second)
		}
	}

	// A title on no channel reads an empty list, never null.
	if _, raw := nextAirings(t, h, answeredMovie, memberToken); !strings.Contains(raw, `"nextAirings":[]`) {
		t.Errorf("page of a title on no channel = %s, want an empty list", raw)
	}
}

// §19: a series' page names the episode that airs, and never one a member doesn't read.
func TestLibraryPageNamesNoHiddenEpisodeToAMember(t *testing.T) {
	h, now := nextAiringHarness(t)
	episode := func(id string, from int) channeldatastore.UpcomingAiring {
		start := now.Add(time.Duration(from) * time.Minute)
		return channeldatastore.UpcomingAiring{Start: start, End: start.Add(30 * time.Minute), LibraryItemID: id,
			TitleKey: nextSeriesKey}
	}
	airOn(t, h, "named", 1, schedule.StatusLive, episode("90000011", 30))
	airOn(t, h, "from-file", 2, schedule.StatusLive, episode(fileNamedEpisode, 60), episode("90000011", 90))
	airOn(t, h, "unmarked", 3, schedule.StatusLive, episode(unmarkedEpisode, 120))

	admin, _ := nextAirings(t, h, "90000002", adminToken)
	if len(admin.NextAirings) != 3 {
		t.Fatalf("admin next airings = %+v", admin.NextAirings)
	}
	for i, want := range []string{"90000011", fileNamedEpisode, unmarkedEpisode} {
		if ep := admin.NextAirings[i].Episode; ep == nil || ep.ID != want || ep.Season != 1 || ep.Number != i+1 {
			t.Errorf("admin entry %d episode = %+v, want %s", i, ep, want)
		}
	}

	member, raw := nextAirings(t, h, "90000002", memberToken)
	if len(member.NextAirings) != 3 {
		t.Fatalf("member next airings = %+v, want every channel still listed", member.NextAirings)
	}
	if ep := member.NextAirings[0].Episode; ep == nil || ep.ID != "90000011" || ep.Name != "Bravo E1" {
		t.Errorf("member's named episode = %+v", ep)
	}
	for _, a := range member.NextAirings[1:] {
		if a.Episode != nil {
			t.Errorf("member reads hidden episode %+v on %s", a.Episode, a.Channel.ID)
		}
	}
	for _, hidden := range []string{fileNamedEpisode, unmarkedEpisode, "bravo.e2", "bravo.e3"} {
		if strings.Contains(raw, hidden) {
			t.Errorf("member page holds %q: %s", hidden, raw)
		}
	}
}
