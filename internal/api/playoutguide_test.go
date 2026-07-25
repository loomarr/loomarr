package api_test

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeXMLTVGuide answers programme timelines without a store or a scheduler.
type fakeXMLTVGuide struct {
	byChannel map[string][]playout.Broadcast
	// withPending is what BroadcastsWithPending returns; nil ⇒ it echoes byChannel. Kept
	// separate so a test can prove WHICH projection a route asked for — the XMLTV document and
	// the time grid must not be able to swap them silently.
	withPending map[string][]playout.Broadcast
	errFor      map[string]error
	windows     []time.Duration // the (to-from) span of each call, for the window assertions
}

func (f *fakeXMLTVGuide) BroadcastsBetween(
	_ context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	f.windows = append(f.windows, to.Sub(from))
	if err := f.errFor[channelID]; err != nil {
		return nil, err
	}
	return f.byChannel[channelID], nil
}

func (f *fakeXMLTVGuide) BroadcastsWithPending(
	_ context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	f.windows = append(f.windows, to.Sub(from))
	if err := f.errFor[channelID]; err != nil {
		return nil, err
	}
	if f.withPending != nil {
		return f.withPending[channelID], nil
	}
	return f.byChannel[channelID], nil
}

func newGuideServer(t *testing.T, g api.PlayoutGuide) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/guide.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := map[string]string{
		"server.public_url": "http://loomarr.local:8080",
		"playout.backend":   "internal",
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:         st,
		Auth:          api.NewTokenAuthorizer(adminToken),
		Log:           slog.New(slog.DiscardHandler),
		PlayoutSecret: func() string { return playoutToken },
		PlayoutGuide:  g,
		LiveConfig:    func(k string) string { return cfg[k] },
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

func nowProgramme(title string, offset time.Duration, mins int) playout.Broadcast {
	start := time.Now().Add(offset)
	return playout.Broadcast{
		Kind: schedule.SlotProgram, Title: title,
		Start: start, Stop: start.Add(time.Duration(mins) * time.Minute),
	}
}

// The guide is a playout route, so it is gated by the device token like every other one.
func TestPlayoutGuide_RequiresTheDeviceToken(t *testing.T) {
	srv, st := newGuideServer(t, &fakeXMLTVGuide{})
	seedChannel(t, st, "ch1", "Classic Sci-Fi", 50, "internal")

	for _, q := range []string{"", "?token=wrong", "?token=" + playoutToken[:8]} {
		if resp := getPlayout(t, srv, "/playout/guide.xml"+q); resp.StatusCode != http.StatusNotFound {
			t.Errorf("token %q: status %d, want 404", q, resp.StatusCode)
		}
	}
}

// THE POINT OF THE PHASE: a channel in the tuner gets real listings, not "no information".
func TestPlayoutGuide_ServesRealListings(t *testing.T) {
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"ch1": {
			nowProgramme("Last Action Hero", -40*time.Minute, 130),
			nowProgramme("The Transformers: The Movie", 90*time.Minute, 84),
		},
	}}
	srv, st := newGuideServer(t, g)
	seedChannel(t, st, "ch1", "Classic Sci-Fi", 50, "internal")

	resp := getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("Content-Type = %q, want an XML type", ct)
	}
	body, _ := io.ReadAll(resp.Body)

	var doc struct {
		Channels []struct {
			ID string `xml:"id,attr"`
		} `xml:"channel"`
		Programmes []struct {
			Title   string `xml:"title"`
			Channel string `xml:"channel,attr"`
			Start   string `xml:"start,attr"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("guide does not parse — a media server would show nothing: %v\n%s", err, body)
	}
	if len(doc.Channels) != 1 || doc.Channels[0].ID != "ch1" {
		t.Errorf("channels = %+v, want just ch1", doc.Channels)
	}
	if len(doc.Programmes) != 2 {
		t.Fatalf("%d programmes, want 2:\n%s", len(doc.Programmes), body)
	}
	if doc.Programmes[0].Title != "Last Action Hero" {
		t.Errorf("first programme = %q", doc.Programmes[0].Title)
	}
	if doc.Programmes[0].Channel != "ch1" {
		t.Errorf("programme not attributed to its channel: %q", doc.Programmes[0].Channel)
	}
}

// ⚠ The guide's channel ids must match the TUNER's tvg-id exactly — the most common Live TV
// wiring failure, and silent (channels play, guide is empty). Asserted by fetching BOTH and
// comparing, rather than by trusting each in isolation.
func TestPlayoutGuide_ChannelIDsMatchTheTuner(t *testing.T) {
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"classic-simpsons": {nowProgramme("Bart the Genius", 0, 22)},
	}}
	srv, st := newGuideServer(t, g)
	seedChannel(t, st, "classic-simpsons", "Classic Simpsons", 52, "internal")

	tuner, _ := io.ReadAll(getPlayout(t, srv, "/playout/tuner.m3u?token="+playoutToken).Body)
	guide, _ := io.ReadAll(getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken).Body)

	if !strings.Contains(string(tuner), `tvg-id="classic-simpsons"`) {
		t.Fatalf("tuner is missing the channel:\n%s", tuner)
	}
	if !strings.Contains(string(guide), `<channel id="classic-simpsons">`) {
		t.Errorf("guide id does not match the tuner's tvg-id — every channel would play with "+
			"an empty guide:\n%s", guide)
	}
}

// Only channels internal playout actually serves. A Tunarr-backed channel in Loomarr's guide
// would advertise listings for a channel Loomarr does not stream.
func TestPlayoutGuide_ExcludesTunarrBackedChannels(t *testing.T) {
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"mine":   {nowProgramme("Heat", 0, 60)},
		"theirs": {nowProgramme("Predator", 0, 60)},
	}}
	srv, st := newGuideServer(t, g)
	seedChannel(t, st, "mine", "Internal", 1, "internal")
	seedChannel(t, st, "theirs", "Tunarr", 2, "tunarr")

	body, _ := io.ReadAll(getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken).Body)
	if !strings.Contains(string(body), `id="mine"`) {
		t.Errorf("internal channel missing:\n%s", body)
	}
	if strings.Contains(string(body), `id="theirs"`) {
		t.Errorf("advertised a Tunarr-backed channel Loomarr does not stream:\n%s", body)
	}
}

// ONE channel failing must not blank the whole guide. A media server re-fetches the document
// wholesale, so an error return would empty every channel's listings because one had a problem.
func TestPlayoutGuide_OneChannelFailingDoesNotEmptyTheGuide(t *testing.T) {
	g := &fakeXMLTVGuide{
		byChannel: map[string][]playout.Broadcast{"good": {nowProgramme("Heat", 0, 60)}},
		errFor:    map[string]error{"bad": errors.New("scheduler exploded")},
	}
	srv, st := newGuideServer(t, g)
	seedChannel(t, st, "good", "Good", 1, "internal")
	seedChannel(t, st, "bad", "Bad", 2, "internal")

	resp := getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d — one bad channel failed the whole guide", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Heat") {
		t.Errorf("the healthy channel lost its listings:\n%s", body)
	}
	// The failing channel still APPEARS, so the media server keeps it and shows
	// "no information" rather than dropping it from Live TV.
	if !strings.Contains(string(body), `id="bad"`) {
		t.Errorf("the failing channel vanished from Live TV entirely:\n%s", body)
	}
}

// The window spans past AND future: a media server needs the currently-airing programme's real
// start time to draw it, which requires asking from before now.
func TestPlayoutGuide_WindowCoversPastAndFuture(t *testing.T) {
	g := &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{"ch1": {}}}
	srv, st := newGuideServer(t, g)
	seedChannel(t, st, "ch1", "Ch", 1, "internal")

	_ = getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken)
	if len(g.windows) == 0 {
		t.Fatal("the guide never asked for a timeline")
	}
	// Lookback + lookahead. A window of only the future would leave the in-progress
	// programme without a start time.
	if g.windows[0] < 20*time.Hour {
		t.Errorf("window is %v — too short to be a useful day of listings", g.windows[0])
	}
}

// A guide must never be cached: it changes as the wall clock advances, and a copy served an
// hour later lists programmes that already finished.
func TestPlayoutGuide_IsNotCacheable(t *testing.T) {
	srv, st := newGuideServer(t, &fakeXMLTVGuide{})
	seedChannel(t, st, "ch1", "Ch", 1, "internal")

	resp := getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken)
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

// Playout not running is a 501 that explains itself, not a 404 that reads as a wiring mistake.
func TestPlayoutGuide_NotRunningExplainsItself(t *testing.T) {
	srv, st := newGuideServer(t, nil)
	seedChannel(t, st, "ch1", "Ch", 1, "internal")

	if resp := getPlayout(t, srv, "/playout/guide.xml?token="+playoutToken); resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status %d, want 501", resp.StatusCode)
	}
}
