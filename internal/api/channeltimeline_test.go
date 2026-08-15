package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeTimelineThumbs is a canned TimelineThumbResolver — it records the keys it was asked about and
// returns a stub URL for programme keys, so a test can assert the handler resolves images for
// programmes (and skips breaks).
// ⚠ It returns an IMAGE-SERVICE URL, not a TMDB one — the resolver stopped emitting third-party
// URLs in V52 phase 7 (§22), and a fake still handing one back would let a regression through.
type fakeTimelineThumbs struct {
	mu    sync.Mutex
	asked []string
	hash  string
}

func (f *fakeTimelineThumbs) ThumbFor(_ context.Context, key string, _, _ int) (string, string) {
	f.mu.Lock()
	f.asked = append(f.asked, key)
	f.mu.Unlock()
	if key == "" {
		return "", ""
	}
	return "/v1/images/" + key + "/w300.jpg", f.hash
}

func (f *fakeTimelineThumbs) askedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

func newTimelineServer(t *testing.T, g api.PlayoutGuide, thumbs api.TimelineThumbResolver) (*httptest.Server, store.Store) {
	return newTimelineServerWithImages(t, g, thumbs, nil)
}

func newTimelineServerWithImages(t *testing.T, g api.PlayoutGuide, thumbs api.TimelineThumbResolver, imageService api.ImageService) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/timeline.db")
	t.Cleanup(func() { _ = st.Close() })
	cfg := map[string]string{"server.public_url": "http://loomarr.local:8080", "playout.backend": "internal"}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:          st,
		Auth:           api.NewTokenAuthorizer(adminToken),
		Log:            slog.New(slog.DiscardHandler),
		PlayoutGuide:   g,
		TimelineThumbs: thumbs,
		Images:         imageService,
		LiveConfig:     func(k string) string { return cfg[k] },
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

// Watch consumes the same image-service record as Guide and Filler. Pinning the public timeline
// response here prevents the player from quietly regressing to the legacy URL-only projection.
func TestChannelTimeline_ProgrammeCarriesImageServiceRecord(t *testing.T) {
	now := time.Now()
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"ch1": {{
			Kind: schedule.SlotProgram, Key: "movie:tmdb:89", Title: "Indiana Jones",
			Start: now, Stop: now.Add(2 * time.Hour),
		}},
	}}
	imageService := newFakeImageService()
	imageService.records["watch-art"] = images.Image{
		Hash: "watch-art", Role: images.RoleBackdrop, Width: 640, Height: 360,
		Visibility: images.VisibilityMember,
	}
	srv, st := newTimelineServerWithImages(t, g, &fakeTimelineThumbs{hash: "watch-art"}, imageService)
	seedChannel(t, st, "ch1", "Springfield Classics", 1, "internal")

	airings := getTimeline(t, srv, "ch1")
	if len(airings) != 1 {
		t.Fatalf("got %d airings, want 1", len(airings))
	}
	got := airings[0].ThumbImage
	if got == nil || got.Hash != "watch-art" || got.Src != "/v1/images/watch-art/w780.jpg" || got.SrcSetWebP == "" {
		t.Fatalf("watch image = %+v, want the complete image-service record", got)
	}
}

// getTimeline fetches + decodes the timeline airings for a channel.
func getTimeline(t *testing.T, srv *httptest.Server, channelID string) []api.GuideAiring {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/channels/"+channelID+"/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timeline status %d, want 200", resp.StatusCode)
	}
	var body struct {
		Airings []api.GuideAiring `json:"airings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Airings
}

// A programme block on a series carries its episode identity + a break sits between programmes, and
// the strip resolves a preview image for the PROGRAMMES only — a break has no title image.
func TestChannelTimeline_EpisodesAndBreaks(t *testing.T) {
	now := time.Now()
	prog := func(series, ep string, s, e int, key provision.Key, start time.Time, mins int) playout.Broadcast {
		return playout.Broadcast{
			Kind: schedule.SlotProgram, SeriesTitle: series, Title: ep, Season: s, Episode: e,
			Key: key, Start: start, Stop: start.Add(time.Duration(mins) * time.Minute),
		}
	}
	brk := func(start time.Time, mins int) playout.Broadcast {
		return playout.Broadcast{Kind: schedule.SlotFiller, Start: start, Stop: start.Add(time.Duration(mins) * time.Minute)}
	}
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"ch1": {
			prog("The Simpsons", "Moaning Lisa", 1, 6, "series:tmdb:456", now, 22),
			brk(now.Add(22*time.Minute), 2),
			prog("The Simpsons", "Bart the Genius", 1, 2, "series:tmdb:456", now.Add(24*time.Minute), 22),
		},
	}}
	thumbs := &fakeTimelineThumbs{}
	srv, st := newTimelineServer(t, g, thumbs)
	seedChannel(t, st, "ch1", "Springfield Classics", 1, "internal")

	airings := getTimeline(t, srv, "ch1")
	if len(airings) != 3 {
		t.Fatalf("got %d airings, want 3 (program, break, program)", len(airings))
	}

	// The first block carries the full episode identity the strip's hover card shows.
	first := airings[0]
	if first.Kind != "program" || first.Series != "The Simpsons" || first.Title != "Moaning Lisa" ||
		first.Season != 1 || first.Episode != 6 {
		t.Errorf("first block episode identity wrong: %+v", first)
	}
	// The middle block is the commercial break (filler), with no title image.
	if airings[1].Kind != "filler" {
		t.Errorf("middle block kind = %q, want filler (the commercial break)", airings[1].Kind)
	}
	if airings[1].ThumbURL != "" {
		t.Errorf("a break must have no preview image, got %q", airings[1].ThumbURL)
	}
	// Programmes got a resolved image; the break did not (the resolver was asked only for programmes).
	if first.ThumbURL == "" {
		t.Error("a programme block should carry a resolved preview image")
	}
	if got := thumbs.askedCount(); got != 2 {
		t.Errorf("thumb resolver asked %d times, want 2 (one per programme, not the break)", got)
	}
}

// No internal-playout guide wired ⇒ an empty strip, not an error — the player falls back to its
// status line rather than the page failing.
func TestChannelTimeline_NoGuideIsEmptyNotError(t *testing.T) {
	srv, st := newTimelineServer(t, nil, nil)
	seedChannel(t, st, "ch1", "Springfield Classics", 1, "internal")
	if airings := getTimeline(t, srv, "ch1"); len(airings) != 0 {
		t.Errorf("no guide should yield an empty strip, got %d airings", len(airings))
	}
}
