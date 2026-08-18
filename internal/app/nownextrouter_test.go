package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// The shipped bug, as a test: a channel Loomarr streams internally must NOT be answered from
// Tunarr's guide. Both readers return a programme; only one of them is about the schedule this
// channel is actually playing.
func TestNowNextRouter_InternalChannelDoesNotReadTunarrsGuide(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	ch := store.Channel{Channel: schedule.Channel{ID: "ch1", TunarrID: "tun1"}}
	// ⚠ A SECOND, TUNARR-BACKED CHANNEL IS LOAD-BEARING. With only internal channels present,
	// anyOnTunarr short-circuits the guide read entirely, so `fromTunarr` is nil and the
	// assertion below would pass even with the routing removed — the test would be vacuous.
	// Verified by sabotage: without this channel, reverting the fix leaves this test green.
	onTunarr := store.Channel{Channel: schedule.Channel{ID: "ch2", TunarrID: "tun-tun"}}

	r := nowNextRouter{
		tunarr:   stubGuide{now: "Tunarr Says This"},
		internal: stubBroadcasts{title: "Internal Says This", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels: routerChannels{list: []store.Channel{ch, onTunarr}},
		// Only ch1 is internal; ch2 keeps the Tunarr guide populated.
		internalFor: func(c store.Channel) bool { return c.ID == "ch1" },
		window:      2 * time.Hour,
	}

	got, err := r.NowNext(context.Background(), now)
	if err != nil {
		t.Fatalf("NowNext: %v", err)
	}
	entry, ok := got["ch1"]
	if !ok {
		t.Fatal("no entry for the internal channel")
	}
	if entry.Now == nil {
		t.Fatal("no 'now' programme")
	}
	if entry.Now.Title != "Internal Says This" {
		t.Errorf("now = %q, want the INTERNAL resolver's answer — reading Tunarr's guide for an "+
			"internally-played channel is the bug this routes around", entry.Now.Title)
	}
}

// The other half: a Tunarr-backed channel must keep reading Tunarr's guide, unchanged.
func TestNowNextRouter_TunarrChannelStillReadsTunarrsGuide(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	ch := store.Channel{Channel: schedule.Channel{ID: "ch1", TunarrID: "tun1"}}

	r := nowNextRouter{
		tunarr:      stubGuide{now: "Tunarr Says This"},
		internal:    stubBroadcasts{title: "Internal Says This", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels:    routerChannels{list: []store.Channel{ch}},
		internalFor: func(store.Channel) bool { return false },
		window:      2 * time.Hour,
	}

	got, _ := r.NowNext(context.Background(), now)
	if got["ch1"].Now == nil || got["ch1"].Now.Title != "Tunarr Says This" {
		t.Errorf("now = %+v, want Tunarr's answer for a Tunarr-backed channel", got["ch1"].Now)
	}
}

func TestNowNextRouterReadsAppliedCheckpointOncePerOperation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	reads := 0
	r := nowNextRouter{
		internal: stubBroadcasts{title: "Internal", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels: routerChannels{list: []store.Channel{
			{Channel: schedule.Channel{ID: "one", Status: schedule.StatusLive}},
			{Channel: schedule.Channel{ID: "two", Status: schedule.StatusLive}},
		}},
		appliedBackend: func(context.Context) (string, error) {
			reads++
			return schedule.PlayoutBackendInternal, nil
		},
		window: 2 * time.Hour,
	}
	got, err := r.NowNext(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 1 || len(got) != 2 {
		t.Fatalf("now/next = %d channels with %d checkpoint reads, want 2/1", len(got), reads)
	}
}

func TestNowNextRouterFailsClosedOnCheckpointRead(t *testing.T) {
	t.Parallel()
	want := errors.New("checkpoint unavailable")
	r := nowNextRouter{
		channels:       routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "one"}}}},
		appliedBackend: func(context.Context) (string, error) { return "", want },
	}
	if _, err := r.NowNext(context.Background(), time.Now()); !errors.Is(err, want) {
		t.Fatalf("NowNext error = %v, want checkpoint failure", err)
	}
}

// `playout.backend` is per-channel overridable (§15), so a MIXED install must be right — this is
// the case a global on/off switch would get wrong.
func TestNowNextRouter_MixedInstallRoutesEachChannelSeparately(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	internalCh := store.Channel{
		Channel: schedule.Channel{ID: "ch-int", TunarrID: "tun-int"},
		Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{Backend: "internal"}}},
	}
	tunarrCh := store.Channel{
		Channel: schedule.Channel{ID: "ch-tun", TunarrID: "tun-tun"},
		Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{Backend: "tunarr"}}},
	}

	r := nowNextRouter{
		tunarr:   stubGuide{now: "From Tunarr"},
		internal: stubBroadcasts{title: "From Internal", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels: routerChannels{list: []store.Channel{internalCh, tunarrCh}},
		// The real precedence rule, not a stub — a mixed install is exactly where a
		// hand-rolled second copy of it would drift.
		internalFor: func(ch store.Channel) bool {
			return schedule.PlaysInternally(ch.Policy, "internal")
		},
		window: 2 * time.Hour,
	}

	got, _ := r.NowNext(context.Background(), now)
	if got["ch-int"].Now == nil || got["ch-int"].Now.Title != "From Internal" {
		t.Errorf("internal channel: now = %+v, want the internal resolver", got["ch-int"].Now)
	}
	if got["ch-tun"].Now == nil || got["ch-tun"].Now.Title != "From Tunarr" {
		t.Errorf("tunarr channel: now = %+v, want Tunarr's guide", got["ch-tun"].Now)
	}
}

// An internal channel with no internal reader must yield NO entry — never a silent fall back to
// Tunarr's guide, which is the defect itself.
func TestNowNextRouter_InternalChannelWithNoResolverHasNoEntry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	r := nowNextRouter{
		tunarr:      stubGuide{now: "Tunarr Says This"},
		internal:    nil,
		channels:    routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "ch1", TunarrID: "tun1"}}}},
		internalFor: func(store.Channel) bool { return true },
		window:      2 * time.Hour,
	}

	got, _ := r.NowNext(context.Background(), now)
	if entry, ok := got["ch1"]; ok {
		t.Errorf("got entry %+v for an internal channel with no resolver; want none — "+
			"falling back to Tunarr's guide is the bug", entry)
	}
}

// An install with every channel on internal playout must not pay a Tunarr round trip to render
// its channel list, and must not be degraded when Tunarr is slow or down.
func TestNowNextRouter_AllInternalDoesNotCallTunarr(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	guide := &countingGuide{}
	r := nowNextRouter{
		tunarr:      guide,
		internal:    stubBroadcasts{title: "Internal", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels:    routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "ch1", TunarrID: "tun1"}}}},
		internalFor: func(store.Channel) bool { return true },
		window:      2 * time.Hour,
	}

	if _, err := r.NowNext(context.Background(), now); err != nil {
		t.Fatalf("NowNext: %v", err)
	}
	if guide.calls != 0 {
		t.Errorf("called Tunarr %d times for an all-internal install; want 0", guide.calls)
	}
}

// A Tunarr outage must not blank an internal channel's card — the internal half does not depend
// on it and must still answer.
func TestNowNextRouter_TunarrFailureDoesNotBlankInternalChannels(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	internalCh := store.Channel{
		Channel: schedule.Channel{ID: "ch-int", TunarrID: "tun-int"},
		Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{Backend: "internal"}}},
	}
	tunarrCh := store.Channel{Channel: schedule.Channel{ID: "ch-tun", TunarrID: "tun-tun"}}

	r := nowNextRouter{
		tunarr:   failingGuide{},
		internal: stubBroadcasts{title: "From Internal", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels: routerChannels{list: []store.Channel{internalCh, tunarrCh}},
		internalFor: func(ch store.Channel) bool {
			return schedule.PlaysInternally(ch.Policy, "tunarr")
		},
		window: 2 * time.Hour,
	}

	got, err := r.NowNext(context.Background(), now)
	if err != nil {
		t.Fatalf("NowNext returned an error on a Tunarr outage: %v", err)
	}
	if got["ch-int"].Now == nil {
		t.Error("internal channel lost its card because Tunarr was down")
	}
	if _, ok := got["ch-tun"]; ok {
		t.Error("tunarr channel got an entry despite the guide failing")
	}
}

// Internal playout has no remote identity requirement: a fresh internal-only channel must get
// now/next from its local timeline even when it has never had a Tunarr id.
func TestNowNextRouter_InternalChannelWithNoTunarrIDIsIncluded(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	r := nowNextRouter{
		internal:    stubBroadcasts{title: "Internal", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels:    routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "ch1"}}}},
		internalFor: func(store.Channel) bool { return true },
		window:      2 * time.Hour,
	}
	got, err := r.NowNext(context.Background(), now)
	if err != nil {
		t.Fatalf("NowNext: %v", err)
	}
	if got["ch1"].Now == nil || got["ch1"].Now.Title != "Internal" {
		t.Errorf("now = %+v, want the internal timeline without a Tunarr id", got["ch1"].Now)
	}
}

func TestNowNextRouter_OmitsPausedDetachedAndEmptyChannels(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	guide := &countingGuide{}
	r := nowNextRouter{
		tunarr: guide,
		internal: stubBroadcasts{
			title: "Must not air", start: now.Add(-time.Hour), stop: now.Add(time.Hour),
		},
		channels: routerChannels{list: []store.Channel{
			{Channel: schedule.Channel{ID: "paused", Status: schedule.StatusPaused}},
			{Channel: schedule.Channel{ID: "detached", TunarrID: "tun-detached", Status: schedule.StatusDetached}},
			{Channel: schedule.Channel{ID: "empty", TunarrID: "tun-empty", Status: schedule.StatusEmpty}},
		}},
		internalFor: func(ch store.Channel) bool { return ch.ID == "paused" },
		window:      2 * time.Hour,
	}

	got, err := r.NowNext(context.Background(), now)
	if err != nil {
		t.Fatalf("NowNext: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("NowNext = %+v, want no off-air channels", got)
	}
	if guide.calls != 0 {
		t.Fatalf("Tunarr guide calls = %d, want none for detached channels", guide.calls)
	}
	for _, id := range []string{"paused", "detached", "empty"} {
		upcoming, uerr := r.Upcoming(context.Background(), id, now, 6)
		if uerr != nil {
			t.Fatalf("Upcoming(%s): %v", id, uerr)
		}
		if len(upcoming) != 0 {
			t.Fatalf("Upcoming(%s) = %+v, want no off-air entries", id, upcoming)
		}
	}
	if guide.calls != 0 {
		t.Fatalf("Tunarr guide calls after Upcoming = %d, want none", guide.calls)
	}
}

// Upcoming routes on the backend too — the strip and the card must not disagree with each other
// any more than either may disagree with the grid.
func TestNowNextRouter_UpcomingRoutesToTheStreamingBackend(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	r := nowNextRouter{
		tunarr:      stubGuide{now: "Tunarr Says This"},
		internal:    stubBroadcasts{title: "Internal Says This", start: now.Add(-time.Hour), stop: now.Add(time.Hour)},
		channels:    routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "ch1"}}}},
		internalFor: func(store.Channel) bool { return true },
		window:      2 * time.Hour,
	}

	got, err := r.Upcoming(context.Background(), "ch1", now, 6)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no upcoming entries")
	}
	if got[0].Title != "Internal Says This" {
		t.Errorf("upcoming[0] = %q, want the internal resolver's answer", got[0].Title)
	}
}

func TestNowNextRouter_UpcomingTranslatesLoomarrIDForTunarr(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	var upcomingID string
	guide := stubGuide{
		now: "From Tunarr",
		onUpcoming: func(id string) {
			upcomingID = id
		},
	}
	r := nowNextRouter{
		tunarr:      guide,
		channels:    routerChannels{list: []store.Channel{{Channel: schedule.Channel{ID: "ch1", TunarrID: "tun1"}}}},
		internalFor: func(store.Channel) bool { return false },
		window:      2 * time.Hour,
	}

	got, err := r.Upcoming(context.Background(), "ch1", now, 6)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if upcomingID != "tun1" {
		t.Fatalf("Tunarr Upcoming id = %q, want translated remote id tun1", upcomingID)
	}
	if len(got) != 1 || got[0].Title != "From Tunarr" {
		t.Fatalf("upcoming = %+v", got)
	}
}

// A series card reads as the SHOW, not the episode name — the same choice XMLTV's <title> makes.
func TestBroadcastToNowNext_UsesTheSeriesTitleForAnEpisode(t *testing.T) {
	t.Parallel()
	b := playout.Broadcast{
		Kind:        schedule.SlotProgram,
		Title:       "Bart the Genius",
		SeriesTitle: "The Simpsons",
	}
	if got := broadcastToNowNext(b).Title; got != "The Simpsons" {
		t.Errorf("title = %q, want the series name", got)
	}
}

// The tmdb id joins a card back to its provisioning key, so a tvdb key must yield NOTHING rather
// than an id from a different space that would link to an unrelated title.
func TestTMDBIDFromKey(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"movie:tmdb:603":    "603",
		"series:tvdb:71663": "",
		"":                  "",
		"movie:tmdb":        "",
	}
	for key, want := range cases {
		if got := tmdbIDFromKey(provision.Key(key)); got != want {
			t.Errorf("tmdbIDFromKey(%q) = %q, want %q", key, got, want)
		}
	}
}

// --- stubs ---

type stubGuide struct {
	now        string
	onUpcoming func(string)
}

func (s stubGuide) NowNext(context.Context, time.Time) (map[string]api.ChannelNowNext, error) {
	return map[string]api.ChannelNowNext{
		"tun1":    {Now: &api.NowNextEntry{Title: s.now}},
		"tun-tun": {Now: &api.NowNextEntry{Title: s.now}},
		"tun-int": {Now: &api.NowNextEntry{Title: s.now}},
	}, nil
}

func (s stubGuide) Upcoming(_ context.Context, id string, _ time.Time, _ int) ([]api.NowNextEntry, error) {
	if s.onUpcoming != nil {
		s.onUpcoming(id)
	}
	return []api.NowNextEntry{{Title: s.now}}, nil
}

type countingGuide struct{ calls int }

func (c *countingGuide) NowNext(context.Context, time.Time) (map[string]api.ChannelNowNext, error) {
	c.calls++
	return map[string]api.ChannelNowNext{}, nil
}

func (c *countingGuide) Upcoming(context.Context, string, time.Time, int) ([]api.NowNextEntry, error) {
	c.calls++
	return nil, nil
}

type failingGuide struct{}

func (failingGuide) NowNext(context.Context, time.Time) (map[string]api.ChannelNowNext, error) {
	return nil, errors.New("tunarr is down")
}

func (failingGuide) Upcoming(context.Context, string, time.Time, int) ([]api.NowNextEntry, error) {
	return nil, errors.New("tunarr is down")
}

type stubBroadcasts struct {
	title       string
	start, stop time.Time
}

func (s stubBroadcasts) BroadcastsBetween(
	_ context.Context, _ string, _, _ time.Time,
) ([]playout.Broadcast, error) {
	return []playout.Broadcast{{
		Kind:  schedule.SlotProgram,
		Title: s.title,
		Start: s.start,
		Stop:  s.stop,
	}}, nil
}

type routerChannels struct{ list []store.Channel }

func (s routerChannels) ListChannels(context.Context) ([]store.Channel, error) { return s.list, nil }

func (s routerChannels) GetChannel(_ context.Context, id string) (store.Channel, error) {
	for _, ch := range s.list {
		if ch.ID == id {
			return ch, nil
		}
	}
	return store.Channel{}, store.ErrNotFound
}
