package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// GET /v1/guide — the time-grid endpoint (V13b). Distinct from /playout/guide.xml, which the
// tests in playoutguide_test.go cover: same arithmetic underneath, different projection and a
// different auth model.

type guideBody struct {
	FromMs   int64  `json:"fromMs"`
	ToMs     int64  `json:"toMs"`
	Timezone string `json:"timezone"`
	Channels []struct {
		ChannelID string `json:"channelId"`
		Name      string `json:"name"`
		Number    int    `json:"number"`
		Airings   []struct {
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Series     string `json:"series"`
			StartMs    int64  `json:"startMs"`
			StopMs     int64  `json:"stopMs"`
			Nominal    bool   `json:"nominal"`
			RuntimeMs  int64  `json:"runtimeMs"`
			Provenance string `json:"provenance"`
			Pod        *struct {
				MatchLevel string `json:"matchLevel"`
				TotalMs    int64  `json:"totalMs"`
				Entries    []struct {
					Name        string `json:"name"`
					Kind        string `json:"kind"`
					Era         int    `json:"era"`
					Quality     string `json:"quality"`
					Brand       string `json:"brand"`
					Audience    string `json:"audience"`
					Category    string `json:"category"`
					VisibleText string `json:"visibleText"`
				} `json:"entries"`
			} `json:"pod"`
		} `json:"airings"`
	} `json:"channels"`
}

func newGridServer(t *testing.T, g api.PlayoutGuide) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/grid.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	log := slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(api.Router(log, api.Options{
		Store:        st,
		Auth:         testAuthorizer{},
		Log:          log,
		PlayoutGuide: g,
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

// newGridServerWithPods wires the pod assembler so filler blocks carry their composition.
func newGridServerWithPods(t *testing.T, g api.PlayoutGuide, p api.PodPreviewer) (*httptest.Server, store.Store) {
	t.Helper()
	return newGridServerWithConfig(t, g, p, nil, nil)
}

// newGridServerWithConfig is the full knob set: pods plus live string/int settings, so the
// timezone and retention tests can drive `guide.*` without a settings subsystem.
func newGridServerWithConfig(
	t *testing.T, g api.PlayoutGuide, p api.PodPreviewer, cfg map[string]string, cfgInt map[string]int,
) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/grid.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	log := slog.New(slog.DiscardHandler)
	opts := api.Options{
		Store:        st,
		Auth:         testAuthorizer{},
		Log:          log,
		PlayoutGuide: g,
		Pods:         p,
	}
	if cfg != nil {
		opts.LiveConfig = func(k string) string { return cfg[k] }
	}
	if cfgInt != nil {
		opts.LiveConfigInt = func(k string) int { return cfgInt[k] }
	}
	srv := httptest.NewServer(api.Router(log, opts))
	t.Cleanup(srv.Close)
	return srv, st
}

func seedGridChannel(t *testing.T, st store.Store, id string, number int) {
	t.Helper()
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: id, Name: id, Number: number, Strategy: "sequential"},
	}); err != nil {
		t.Fatal(err)
	}
}

func getGuide(t *testing.T, srv *httptest.Server, query string) guideBody {
	t.Helper()
	resp := do(t, srv, http.MethodGet, "/v1/guide"+query, adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/guide%s → %d, want 200", query, resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()
	var body guideBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// ms renders an epoch-millisecond value for a query string.
func ms(v int64) string { return strconv.FormatInt(v, 10) }

// errGridBoom stands in for any per-channel failure (a store hiccup, a scheduler error).
var errGridBoom = errors.New("timeline unavailable")

func gridBlock(kind schedule.SlotKind, title string, offset time.Duration, mins int) playout.Broadcast {
	start := time.Now().Add(offset)
	return playout.Broadcast{
		Kind: kind, Title: title,
		Start: start, Stop: start.Add(time.Duration(mins) * time.Minute),
	}
}

// THE GATE'S CENTRAL CLAUSE: a pending slot and a filler pod must be DISTINGUISHABLE. The
// endpoint this replaces collapsed both to `gap: true`, so the grid had no way to draw
// "commercials are playing" differently from "we are still acquiring this" — two completely
// different situations with one rendering.
func TestGuide_PendingAndFillerAreDistinguishable(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {
			gridBlock(schedule.SlotProgram, "Heat", 0, 60),
			gridBlock(schedule.SlotFiller, "Break", time.Hour, 2),
			{
				Kind: schedule.SlotPending, Title: "Dune 2", Nominal: true,
				Start: time.Now().Add(62 * time.Minute), Stop: time.Now().Add(92 * time.Minute),
			},
		},
	}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	body := getGuide(t, srv, "")
	if len(body.Channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(body.Channels))
	}
	kinds := map[string]bool{}
	for _, a := range body.Channels[0].Airings {
		kinds[a.Kind] = true
	}
	for _, want := range []string{"program", "filler", "pending"} {
		if !kinds[want] {
			t.Errorf("no %q block in the timeline; got kinds %v — the grid cannot draw what "+
				"the API will not distinguish", want, kinds)
		}
	}
}

// A pending block's times are INVENTED (a nominal display width), and the wire format must say
// so. A client that treated them as scheduled airtime would promise the user a programme at a
// moment nothing will actually air.
func TestGuide_PendingBlocksAreMarkedNominal(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {
			gridBlock(schedule.SlotProgram, "Heat", 0, 60),
			{
				Kind: schedule.SlotPending, Title: "Dune 2", Nominal: true,
				Start: time.Now().Add(30 * time.Minute), Stop: time.Now().Add(60 * time.Minute),
			},
		},
	}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	for _, a := range getGuide(t, srv, "").Channels[0].Airings {
		if a.Kind == "pending" && !a.Nominal {
			t.Error("a pending block was not marked nominal — a client would render invented " +
				"times as a real listing")
		}
		if a.Kind == "program" && a.Nominal {
			t.Error("a real programme was marked nominal")
		}
	}
}

// THE GATE'S THIRD CLAUSE: Upcoming()'s gap-filtering must not come back. Dropping a break is
// right for a "what's on next" strip and fatal for a grid — every later block silently slides
// into the hole, so the timeline stops matching the clock.
func TestGuide_GapsArePreservedNotFiltered(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {
			gridBlock(schedule.SlotProgram, "Heat", 0, 60),
			gridBlock(schedule.SlotFiller, "Break", time.Hour, 2),
			gridBlock(schedule.SlotProgram, "Predator", 62*time.Minute, 30),
		},
	}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	airings := getGuide(t, srv, "").Channels[0].Airings
	if len(airings) != 3 {
		t.Fatalf("got %d blocks, want 3 — a filtered gap leaves a hole later blocks slide into",
			len(airings))
	}
	// Contiguous: each block starts exactly where the previous one stopped.
	for i := 1; i < len(airings); i++ {
		if airings[i].StartMs != airings[i-1].StopMs {
			t.Errorf("block %d starts at %d but %d stopped at %d — the timeline has a hole",
				i, airings[i].StartMs, i-1, airings[i-1].StopMs)
		}
	}
}

// THE GATE'S FIRST CLAUSE: a window spanning past AND future returns per-channel timelines. The
// grid scrolls backwards to "what did I miss", so a window that begins before now is normal.
func TestGuide_WindowSpanningPastAndFutureReturnsTimelines(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotProgram, "Heat", -30*time.Minute, 60)},
		"ch2": {gridBlock(schedule.SlotProgram, "Alien", -10*time.Minute, 90)},
	}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)
	seedGridChannel(t, st, "ch2", 2)

	now := time.Now()
	body := getGuide(t, srv, "?from="+ms(now.Add(-time.Hour).UnixMilli())+
		"&to="+ms(now.Add(time.Hour).UnixMilli()))

	if len(body.Channels) != 2 {
		t.Fatalf("got %d channels, want both", len(body.Channels))
	}
	for _, ch := range body.Channels {
		if len(ch.Airings) == 0 {
			t.Errorf("channel %s has no timeline over a past+future window", ch.ChannelID)
		}
	}
	if body.FromMs >= body.ToMs {
		t.Errorf("echoed window is empty: %d..%d", body.FromMs, body.ToMs)
	}
}

// A window far larger than the cap is CLAMPED, not rejected: a fast scroll must not blank the
// grid with a 400. The response echoes what was actually served so the client can tell.
func TestGuide_OversizedWindowIsClampedNotRejected(t *testing.T) {
	g := &fakeXMLTVGuide{}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	now := time.Now()
	body := getGuide(t, srv, "?from="+ms(now.UnixMilli())+
		"&to="+ms(now.Add(365*24*time.Hour).UnixMilli()))

	if span := time.Duration(body.ToMs-body.FromMs) * time.Millisecond; span > 8*24*time.Hour {
		t.Errorf("served a %v window; an unbounded walk would produce a multi-megabyte document",
			span)
	}
}

// ONE channel failing must not empty the grid — the row survives with no blocks, which reads as
// "nothing scheduled here" rather than as a deleted channel.
func TestGuide_OneChannelFailingKeepsTheRest(t *testing.T) {
	g := &fakeXMLTVGuide{
		withPending: map[string][]playout.Broadcast{
			"ok": {gridBlock(schedule.SlotProgram, "Heat", 0, 60)},
		},
		errFor: map[string]error{"broken": errGridBoom},
	}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ok", 1)
	seedGridChannel(t, st, "broken", 2)

	body := getGuide(t, srv, "")
	if len(body.Channels) != 2 {
		t.Fatalf("got %d channels, want both (a failing channel still has a row)", len(body.Channels))
	}
	for _, ch := range body.Channels {
		if ch.ChannelID == "ok" && len(ch.Airings) == 0 {
			t.Error("the healthy channel lost its timeline because another channel failed")
		}
		if ch.ChannelID == "broken" && ch.Airings == nil {
			t.Error("airings must be an empty array, not null — clients iterate it")
		}
	}
}

// With no timeline resolver wired the grid is EMPTY, not a 501: the page renders its "nothing
// scheduled" state rather than an error.
func TestGuide_NoResolverIsEmptyNotAnError(t *testing.T) {
	srv, st := newGridServer(t, nil)
	seedGridChannel(t, st, "ch1", 1)

	body := getGuide(t, srv, "")
	if len(body.Channels) != 0 {
		t.Errorf("got %d channels with no resolver wired", len(body.Channels))
	}
}

// The grid shows EVERY channel, including Tunarr-backed ones. The tuner M3U filters to the
// internal backend so a media server does not see two tuners offering one channel — but this is
// Loomarr's own UI, where hiding a channel would look like it had been deleted.
func TestGuide_IncludesTunarrBackedChannels(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"tunarr-ch": {gridBlock(schedule.SlotProgram, "Heat", 0, 60)},
	}}
	srv, st := newGridServer(t, g)
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "tunarr-ch", Name: "Tunarr", Number: 7, Strategy: "sequential"},
		Policy: schedule.ChannelPolicy{
			OperatorPolicy: schedule.OperatorPolicy{
				Playout: &schedule.PlayoutPolicy{Backend: "tunarr"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if body := getGuide(t, srv, ""); len(body.Channels) != 1 {
		t.Errorf("got %d channels; a Tunarr-backed channel is still one of the user's channels",
			len(body.Channels))
	}
}

// The guide is VIEWER-FACING (§8.1): a member browsing what's on must not be met with an admin
// wall. This mirrors GET /v1/channels, which is likewise not admin-gated.
//
// Deliberately asserts "not 403" rather than "401 when anonymous": authorization here is
// two-state (a route either calls requireAdmin or it does not), so there is no authenticated-
// but-not-admin gate to assert against. The regression worth catching is someone later adding
// requireAdmin to this route and silently removing the grid from every non-admin account.
func TestGuide_IsNotAdminGated(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotProgram, "Heat", 0, 60)},
	}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	// A real MEMBER, not an anonymous caller. This previously passed "" and asserted the
	// guide was reachable — which proved an ANONYMOUS caller could read it, not a member.
	// The distinction is the whole point of §8.1 being viewer-facing rather than public.
	resp := do(t, srv, http.MethodGet, "/v1/guide", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Errorf("GET /v1/guide → %d for a MEMBER; the guide is viewer-facing (§8.1), so "+
			"members must be able to see what is on", resp.StatusCode)
	}
}

// --- V13b gaps 5–8: pod composition, metadata, timezone, retention ---

// GAP 5: a filler block carries the ACTUAL clips that will air in THAT break — the hover
// card's whole purpose. Without it, "why are my commercials wrong" has no answer on the
// surface where the question occurs.
func TestGuide_FillerBlocksCarryTheirPodComposition(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotFiller, "Break", 0, 4)},
	}}
	fp := &fakePods{pod: filler.Pod{
		MatchLevel: filler.MatchExact,
		TotalMs:    65000,
		Entries: []filler.PodEntry{
			{Name: "Channel bumper", Kind: filler.Bumper, DurationMs: 5000, Era: 1994, Quality: "480p"},
			{Name: "Sunny D", Kind: filler.Commercial, DurationMs: 30000, Era: 1994, Quality: "480p"},
		},
	}}
	srv, st := newGridServerWithPods(t, g, fp)
	seedGridChannel(t, st, "ch1", 1)

	airings := getGuide(t, srv, "").Channels[0].Airings
	if len(airings) != 1 || airings[0].Pod == nil {
		t.Fatalf("filler block has no pod composition: %+v", airings)
	}
	pod := airings[0].Pod
	if len(pod.Entries) != 2 {
		t.Fatalf("pod has %d entries, want the clips that actually play", len(pod.Entries))
	}
	if pod.MatchLevel == "" {
		t.Error("no match level — the answer to 'why are my commercials wrong' is missing")
	}
	if pod.Entries[0].Quality == "" || pod.Entries[0].Era == 0 {
		t.Errorf("entry lacks era/quality context: %+v", pod.Entries[0])
	}
}

// V44: the richer clip metadata (brand, audience, category, visibleText) rides the hover card
// alongside era/quality, so the break can explain what it is FOR ("Kellogg's · cereal · kids"),
// not just when it is from. These are display-only passengers carried straight from the catalog
// row — the mapper must not drop them, or the whole V44 surfacing is invisible to the guide.
func TestGuide_FillerPodCarriesRicherClipMetadata(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotFiller, "Break", 0, 4)},
	}}
	fp := &fakePods{pod: filler.Pod{
		MatchLevel: filler.MatchExact,
		TotalMs:    30000,
		Entries: []filler.PodEntry{{
			Name: "Frosted Flakes", Kind: filler.Commercial, DurationMs: 30000,
			Era: 1994, Quality: "480p",
			// Grounded at the source (§8): each of these persisted on the clip only because a
			// text/visual signal literally contained it. Here the mapper merely carries them.
			Brand: "Kellogg's", Audience: filler.Kids, Category: "cereal",
			VisibleText: "KELLOGG'S FROSTED FLAKES",
		}},
	}}
	srv, st := newGridServerWithPods(t, g, fp)
	seedGridChannel(t, st, "ch1", 1)

	airings := getGuide(t, srv, "").Channels[0].Airings
	if len(airings) != 1 || airings[0].Pod == nil || len(airings[0].Pod.Entries) != 1 {
		t.Fatalf("filler block has no pod entry: %+v", airings)
	}
	e := airings[0].Pod.Entries[0]
	// Each field must survive the Clip → PodEntry → PodEntryDTO trip. Sabotage check: drop any
	// one of these in podToPoolDTO and exactly its assertion goes red.
	if e.Brand != "Kellogg's" {
		t.Errorf("brand = %q, want it carried to the hover card", e.Brand)
	}
	if e.Audience != "kids" {
		t.Errorf("audience = %q, want it carried through as the Audience string", e.Audience)
	}
	if e.Category != "cereal" {
		t.Errorf("category = %q, want it carried to the hover card", e.Category)
	}
	if e.VisibleText != "KELLOGG'S FROSTED FLAKES" {
		t.Errorf("visibleText = %q, want the on-screen text that grounds the brand (auditable, §8)",
			e.VisibleText)
	}
}

// V44 omitempty: an untagged/ungrounded clip carries NONE of the richer fields, and the wire
// must OMIT them rather than ship empty strings. "" is the honest common case (§8 — no brand is
// not a failure), and a present-but-empty `brand` would read as "we looked and found none" when
// we simply never grounded one. Proven at the JSON level, since that is where omitempty acts.
func TestGuide_UntaggedClipOmitsRicherMetadata(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotFiller, "Break", 0, 4)},
	}}
	fp := &fakePods{pod: filler.Pod{
		Entries: []filler.PodEntry{{Name: "Mystery spot", Kind: filler.Commercial, DurationMs: 30000}},
	}}
	srv, st := newGridServerWithPods(t, g, fp)
	seedGridChannel(t, st, "ch1", 1)

	// Read the RAW JSON, not the decoded struct: omitempty is a wire fact, and decoding into a
	// struct with the fields present would hide a missing key behind its zero value.
	resp := do(t, srv, http.MethodGet, "/v1/guide", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"brand"`, `"audience"`, `"category"`, `"visibleText"`} {
		if bytes.Contains(raw, []byte(key)) {
			t.Errorf("untagged clip shipped %s — an empty value reads as 'we found none' rather "+
				"than 'never grounded' (§8)", key)
		}
	}
}

// GAP 5, the part that makes it per-AIRING: each break is resolved at its OWN start, so
// consecutive breaks can differ. Asking once per channel would show one representative pod
// and quietly misdescribe every other break.
func TestGuide_ResolvesEachBreakAtItsOwnStart(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {
			gridBlock(schedule.SlotFiller, "Break", 0, 4),
			gridBlock(schedule.SlotFiller, "Break", 30*time.Minute, 4),
		},
	}}
	fp := &fakePods{pod: filler.Pod{
		Entries: []filler.PodEntry{{Name: "Ad", Kind: filler.Commercial, DurationMs: 30000}},
	}}
	srv, st := newGridServerWithPods(t, g, fp)
	seedGridChannel(t, st, "ch1", 1)
	_ = getGuide(t, srv, "")

	if len(fp.atAsked) != 2 {
		t.Fatalf("asked for %d pods, want one per break", len(fp.atAsked))
	}
	if fp.atAsked[0] == fp.atAsked[1] {
		t.Error("both breaks were resolved at the same instant — every break would show the " +
			"same clips regardless of when it airs")
	}
}

// A non-filler block has no pod: a programme is not a break, and an empty pod object would
// make the client guard a case that cannot happen.
func TestGuide_ProgrammesCarryNoPod(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotProgram, "Heat", 0, 60)},
	}}
	fp := &fakePods{pod: filler.Pod{Entries: []filler.PodEntry{{Name: "Ad", DurationMs: 30000}}}}
	srv, st := newGridServerWithPods(t, g, fp)
	seedGridChannel(t, st, "ch1", 1)

	if pod := getGuide(t, srv, "").Channels[0].Airings[0].Pod; pod != nil {
		t.Errorf("a programme carried a pod: %+v", pod)
	}
}

// GAP 6: runtime and provenance ride on the block. Provenance is what turns a pending slot
// from a mystery into "requested · 41h left".
func TestGuide_CarriesRuntimeAndProvenance(t *testing.T) {
	b := gridBlock(schedule.SlotProgram, "Heat", 0, 60)
	b.RuntimeMs = 170 * 60 * 1000
	b.Provenance = "in library"
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{"ch1": {b}}}
	srv, st := newGridServer(t, g)
	seedGridChannel(t, st, "ch1", 1)

	a := getGuide(t, srv, "").Channels[0].Airings[0]
	if a.RuntimeMs == 0 {
		t.Error("no runtime — a 22m episode in a 30m slot would be indistinguishable from a full one")
	}
	if a.Provenance == "" {
		t.Error("no provenance — a pending slot stays a mystery without it")
	}
}

// GAP 7: the guide states which timezone its times should be RENDERED in. The times stay
// absolute epoch ms — a timezone is a formatting choice, not a reinterpretation.
func TestGuide_ReportsTheConfiguredTimezone(t *testing.T) {
	g := &fakeXMLTVGuide{}
	srv, st := newGridServerWithConfig(t, g, nil, map[string]string{
		"guide.timezone": "America/New_York",
	}, nil)
	seedGridChannel(t, st, "ch1", 1)

	if tz := getGuide(t, srv, "").Timezone; tz != "America/New_York" {
		t.Errorf("timezone = %q, want the configured value", tz)
	}
}

// GAP 8: retention bounds how far back the grid may scroll. The past is recomputed from the
// CURRENT lineup, so an unbounded lookback would render fiction as history.
func TestGuide_ClampsToTheConfiguredRetention(t *testing.T) {
	g := &fakeXMLTVGuide{}
	srv, st := newGridServerWithConfig(t, g, nil, nil, map[string]int{
		"guide.retention_hours": 2,
	})
	seedGridChannel(t, st, "ch1", 1)

	now := time.Now()
	// Ask for a week back; retention says two hours.
	body := getGuide(t, srv, "?from="+ms(now.Add(-7*24*time.Hour).UnixMilli())+
		"&to="+ms(now.UnixMilli()))

	back := now.Sub(time.UnixMilli(body.FromMs))
	if back > 3*time.Hour {
		t.Errorf("served %v of history against a 2h retention — the grid would show a "+
			"schedule that never aired", back)
	}
}
