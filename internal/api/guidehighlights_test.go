package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

func episodeAt(series string, season, episode int, start time.Time) playout.Broadcast {
	return playout.Broadcast{Kind: schedule.SlotProgram, SeriesTitle: series, Title: fmt.Sprintf("Episode %d", episode),
		Key: provision.Key("series:tvdb:" + series), Season: season, Episode: episode, Start: start, Stop: start.Add(30 * time.Minute)}
}

type highlightsWire struct {
	Highlights []struct {
		Reason        string          `json:"reason"`
		ChannelID     string          `json:"channelId"`
		ChannelName   string          `json:"channelName"`
		ChannelNumber int             `json:"channelNumber"`
		Episodes      int             `json:"episodes"`
		UntilMs       int64           `json:"untilMs"`
		Airing        api.GuideAiring `json:"airing"`
	} `json:"highlights"`
}

func TestGuideHighlightsOverHTTP(t *testing.T) {
	t.Parallel()
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	guide := &fakeXMLTVGuide{
		byChannel: map[string][]playout.Broadcast{
			"ch-comedy": {
				episodeAt("Office Show", 4, 1, start),
				episodeAt("Office Show", 4, 2, start.Add(30*time.Minute)),
				episodeAt("Office Show", 4, 3, start.Add(time.Hour)),
			},
			"ch-scifi": {
				episodeAt("Space Show", 2, 7, start),
				episodeAt("Space Show", 2, 8, start.Add(30*time.Minute)),
				episodeAt("Space Show", 2, 9, start.Add(time.Hour)),
				episodeAt("Space Show", 2, 10, start.Add(90*time.Minute)),
			},
			// A paused channel airs nothing, whatever its schedule says.
			"ch-paused": {episodeAt("Paused Show", 1, 1, start)},
		},
		// One channel's timeline failing leaves it out rather than failing Home.
		errFor: map[string]error{"ch-broken": errors.New("library unreachable")},
	}
	h := newPlayoutGuideHarness(t, guide)
	seedChannel(t, h.Store, "ch-comedy", "Comedy", 3, "internal")
	seedChannel(t, h.Store, "ch-scifi", "Sci-Fi", 7, "internal")
	seedChannel(t, h.Store, "ch-paused", "Paused", 9, "internal")
	seedChannel(t, h.Store, "ch-broken", "Broken", 11, "internal")
	setChannelStatus(t, h.Store, "ch-paused", schedule.StatusPaused)

	res := do(t, h.Server, http.MethodGet, "/v1/guide/highlights", adminToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("highlights = %d, want 200", res.StatusCode)
	}
	var body highlightsWire
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	var got []string
	for _, hl := range body.Highlights {
		got = append(got, fmt.Sprintf("%s#%d %s S%dE%d x%d until+%s", hl.ChannelName, hl.ChannelNumber, hl.Reason,
			hl.Airing.Season, hl.Airing.Episode, hl.Episodes, time.UnixMilli(hl.UntilMs).Sub(start)))
	}
	want := "Comedy#3 season_premiere S4E1 x3 until+1h30m0s | Sci-Fi#7 marathon S2E7 x4 until+2h0m0s"
	if strings.Join(got, " | ") != want {
		t.Fatalf("highlights =\n %s\nwant\n %s", strings.Join(got, " | "), want)
	}

	// A signed-out caller gets nothing.
	if res := do(t, h.Server, http.MethodGet, "/v1/guide/highlights", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous highlights = %d, want 401", res.StatusCode)
	}
}
