package programmer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/testkit"
)

// newServer spins a mock Tunarr with a per-path handler map and records the last
// request body seen for assertions.
type capture struct {
	method string
	path   string
	body   []byte
}

func TestEnsureChannel_Create_ReadsServerAssignedID(t *testing.T) {
	// Phase-0 finding 1: the server assigns the id; the client-supplied id is
	// ignored. The fixture's id (2540b613-…) must come back, NOT anything we send.
	createResp := testkit.Fixture(t, "tunarr/channel_create_response.json")
	var got capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(createResp)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg-uuid")
	id, err := c.EnsureChannel(context.Background(), programmer.ChannelSpec{
		Number: 42, Name: "Cartoons", Group: "Kids",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "2540b613-10b1-4d78-ab35-ee48b313e359" {
		t.Fatalf("EnsureChannel must return the server-assigned id, got %q", id)
	}
	if got.method != http.MethodPost || got.path != "/api/channels" {
		t.Fatalf("create hit %s %s, want POST /api/channels", got.method, got.path)
	}
	// The body must be the {type:"new",channel:{…}} envelope (Phase-0 finding 2).
	var env struct {
		Type    string `json:"type"`
		Channel struct {
			Number      int    `json:"number"`
			Name        string `json:"name"`
			GroupTitle  string `json:"groupTitle"`
			StreamMode  string `json:"streamMode"`
			TranscodeID string `json:"transcodeConfigId"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(got.body, &env); err != nil {
		t.Fatalf("create body not JSON: %v", err)
	}
	if env.Type != "new" {
		t.Errorf("create envelope type = %q, want new", env.Type)
	}
	if env.Channel.Number != 42 || env.Channel.Name != "Cartoons" || env.Channel.GroupTitle != "Kids" {
		t.Errorf("channel fields not sent: %+v", env.Channel)
	}
	if env.Channel.TranscodeID != "cfg-uuid" {
		t.Errorf("transcodeConfigId = %q, want cfg-uuid (Phase-0 finding 3)", env.Channel.TranscodeID)
	}
	if env.Channel.StreamMode != "hls" {
		t.Errorf("streamMode = %q, want hls", env.Channel.StreamMode)
	}
}

// FINDING 5: an empty TUNARR_TRANSCODE_CONFIG_ID (the common case — it is Advanced,
// §15) must AUTO-RESOLVE the instance's Default, not send "" and let Tunarr 400 every
// create. The resolve is one GET, cached; the Default is picked by name.
func TestEnsureChannel_Create_AutoResolvesTranscodeConfig(t *testing.T) {
	createResp := testkit.Fixture(t, "tunarr/channel_create_response.json")
	var configHits int
	var sentTranscodeID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/transcode_configs":
			configHits++
			// A non-Default first entry proves the pick is by NAME, not position.
			_, _ = w.Write([]byte(`[{"id":"first-uuid","name":"HW"},{"id":"default-uuid","name":"Default"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/channels":
			body, _ := io.ReadAll(r.Body)
			var env struct {
				Channel struct {
					TranscodeID string `json:"transcodeConfigId"`
				} `json:"channel"`
			}
			_ = json.Unmarshal(body, &env)
			sentTranscodeID = env.Channel.TranscodeID
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(createResp)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "") // EMPTY config id → resolve
	for i := 0; i < 2; i++ {         // twice, to prove the resolve is cached
		if _, err := c.EnsureChannel(context.Background(), programmer.ChannelSpec{Number: i + 1, Name: "Ch"}); err != nil {
			t.Fatal(err)
		}
	}
	if sentTranscodeID != "default-uuid" {
		t.Errorf("auto-resolved transcodeConfigId = %q, want default-uuid (the Default, by name)", sentTranscodeID)
	}
	if configHits != 1 {
		t.Errorf("resolved the transcode config %d times, want 1 (it must be cached)", configHits)
	}
}

// A reachable Tunarr with NO transcode configs is a real dead-end: every create would
// 400. TranscodeConfigID surfaces it as an error so the setup check can go red instead
// of green (FINDING 5).
func TestTranscodeConfigID_ErrorsWhenNoneExist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "")
	if _, err := c.TranscodeConfigID(context.Background()); err == nil {
		t.Error("expected an error when the instance reports no transcode configs")
	}
}

// A non-2xx from Tunarr must carry Tunarr's own response body into the error, not
// just a bare status code. This is the difference between debugging a "status 400"
// mystery and reading Tunarr's reason (e.g. a version-drift field rejection) straight
// from the log — the live 1.3.9 diagnostic that motivated surfacing the body.
func TestEnsureChannel_SurfacesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Bad Request"}`))
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg-uuid")
	_, err := c.EnsureChannel(context.Background(), programmer.ChannelSpec{Number: 1, Name: "X"})
	if err == nil {
		t.Fatal("expected an error on a 400 create")
	}
	if !strings.Contains(err.Error(), "Bad Request") {
		t.Errorf("error must surface Tunarr's body, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error must still carry the status code, got %q", err.Error())
	}
}

func TestEnsureChannel_Update_PutsToID(t *testing.T) {
	var got capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	id, err := c.EnsureChannel(context.Background(), programmer.ChannelSpec{
		TunarrID: "existing-id", Number: 42, Name: "Renamed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "existing-id" {
		t.Fatalf("update must preserve the id, got %q", id)
	}
	if got.method != http.MethodPut || got.path != "/api/channels/existing-id" {
		t.Fatalf("update hit %s %s, want PUT /api/channels/existing-id", got.method, got.path)
	}
}

func TestGetChannel_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	_, ok, err := c.GetChannel(context.Background(), "gone")
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if ok {
		t.Fatal("GetChannel(404) should report ok=false")
	}
}

func TestGetChannel_ParsesFixture(t *testing.T) {
	getResp := testkit.Fixture(t, "tunarr/channel_get_response.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(getResp)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	ch, ok, err := c.GetChannel(context.Background(), "2540b613-10b1-4d78-ab35-ee48b313e359")
	if err != nil || !ok {
		t.Fatalf("GetChannel = ok=%v err=%v", ok, err)
	}
	if ch.Number != 9901 || ch.Name != "loomarr-phase0-spike" || ch.Group != "loomarr-test" {
		t.Errorf("parsed channel wrong: %+v", ch)
	}
}

// mockTunarrWithIndex is a Tunarr test double that serves the content-resolution
// endpoints (media-sources → libraries → persisted programs) so SetLineup can map
// media-server item ids → Tunarr program uuids, plus records the programming push.
// index maps external item id → program uuid.
func mockTunarrWithIndex(t *testing.T, got *capture, index map[string]string) *httptest.Server {
	t.Helper()
	// Build the persisted-programs payload from the index.
	type ident struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	type prog struct {
		ID      string `json:"id"`
		Program struct {
			Type        string  `json:"type"`
			Identifiers []ident `json:"identifiers"`
		} `json:"program"`
	}
	var programs []prog
	for extID, uuid := range index {
		var p prog
		p.ID = uuid
		p.Program.Type = "movie"
		p.Program.Identifiers = []ident{{Type: "emby", ID: extID}}
		programs = append(programs, p)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		// An emby source (src-1) AND a local filler source (src-local). The resolver
		// must SKIP the local one: its /libraries sub-path 400s (like real Tunarr),
		// so walking it would break the whole index build (live-smoke bug).
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "src-1", "type": "emby"},
			{"id": "src-local", "type": "local"},
		})
	})
	mux.HandleFunc("/api/media-sources/src-1/libraries", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "lib-A", "name": "Movies", "enabled": true}})
	})
	// Real Tunarr 400s on a local source's /libraries sub-path — if the resolver
	// doesn't skip local sources, it hits this and the whole resolution fails.
	mux.HandleFunc("/api/media-sources/src-local/libraries", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Bad Request"}`))
	})
	mux.HandleFunc("/api/media-libraries/lib-A/programs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(programs)
	})
	mux.HandleFunc("/api/channels/ch-id/programming", func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetLineup_ResolvesContentIdsAndTranslatesSlots(t *testing.T) {
	var got capture
	// Tunarr has indexed lib-1 → uuid-1 and clip-9 → uuid-9; "gone" is NOT indexed.
	srv := mockTunarrWithIndex(t, &got, map[string]string{
		"lib-1":  "uuid-1",
		"clip-9": "uuid-9",
	})

	c := programmer.New(srv.URL, "cfg")
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, LibraryItemID: "lib-1", DurationMs: 3600000}, // → content uuid-1
		{Kind: schedule.SlotPending, DurationMs: 0},                               // → flex
		{Kind: schedule.SlotFiller, DurationMs: 30000},                            // → flex (§10: filler is a Tunarr filler-list, never inline content)
		{Kind: schedule.SlotFiller, LibraryItemID: "clip-9", DurationMs: 30000},   // → flex (filler never inlines content post-redesign)
		{Kind: schedule.SlotFlex, DurationMs: 60000},                              // → flex
		{Kind: schedule.SlotProgram, LibraryItemID: "gone", DurationMs: 1000},     // unindexed → flex
	}
	if err := c.SetLineup(context.Background(), "ch-id", slots); err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/api/channels/ch-id/programming" {
		t.Fatalf("set lineup hit %s %s", got.method, got.path)
	}
	var body struct {
		Type   string `json:"type"`
		Lineup []struct {
			Type string  `json:"type"`
			ID   string  `json:"id"`
			Dur  float64 `json:"duration"`
		} `json:"lineup"`
	}
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("lineup body not JSON: %v", err)
	}
	if body.Type != "manual" {
		t.Errorf("lineup type = %q, want manual", body.Type)
	}
	if len(body.Lineup) != 6 {
		t.Fatalf("want 6 lineup items, got %d", len(body.Lineup))
	}
	// Content ids are the RESOLVED Tunarr program uuids, not the raw item ids.
	// An unindexed program degrades to flex (never dead air), not a failed push.
	want := []struct {
		typ string
		id  string
	}{
		{"content", "uuid-1"}, // lib-1 resolved
		{"flex", ""},
		{"flex", ""},
		{"flex", ""}, // filler slot → flex (§10 redesign: commercials live in a Tunarr filler-list)
		{"flex", ""},
		{"flex", ""}, // "gone" not indexed → flex
	}
	for i, w := range want {
		if body.Lineup[i].Type != w.typ || body.Lineup[i].ID != w.id {
			t.Errorf("item %d = {%s,%s}, want {%s,%s}", i, body.Lineup[i].Type, body.Lineup[i].ID, w.typ, w.id)
		}
	}
	// Every flex item must carry a POSITIVE duration: Tunarr rejects the whole
	// push with "duration Too small: expected number to be >0" if any is ≤ 0. A
	// pending/unresolved slot has DurationMs 0, so the adapter floors flex (live-
	// smoke bug). Slots 1-4 above are all flex, several with input duration 0.
	for i, item := range body.Lineup {
		if item.Type == "flex" && item.Dur <= 0 {
			t.Errorf("flex item %d has duration %v — Tunarr rejects ≤ 0 (must be floored)", i, item.Dur)
		}
	}
}

// The content-index build reads a library's ENTIRE program list with no paging,
// which on a large library is a slow, large response (a live homelab: 52 MB / ~17 s
// for 15,788 programs). It goes through the long-timeout bulk client, so a slow
// /programs response still resolves rather than being cancelled mid-pull. This test
// delays the /programs response and asserts the resolution succeeds.
func TestSetLineup_SlowProgramsIndexStillResolves(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "src-1", "type": "emby"}})
	})
	mux.HandleFunc("/api/media-sources/src-1/libraries", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "lib-A", "name": "Movies", "enabled": true}})
	})
	mux.HandleFunc("/api/media-libraries/lib-A/programs", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond) // stand in for a slow, large bulk pull
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "uuid-1", "program": map[string]any{"identifiers": []map[string]string{{"type": "emby", "id": "lib-1"}}}},
		})
	})
	var got capture
	mux.HandleFunc("/api/channels/ch-id/programming", func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	slots := []schedule.Slot{{Kind: schedule.SlotProgram, LibraryItemID: "lib-1", DurationMs: 3600000}}
	if err := c.SetLineup(context.Background(), "ch-id", slots); err != nil {
		t.Fatalf("a slow /programs pull must still resolve via the bulk client, got %v", err)
	}
	var body struct {
		Lineup []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"lineup"`
	}
	_ = json.Unmarshal(got.body, &body)
	if len(body.Lineup) != 1 || body.Lineup[0].Type != "content" || body.Lineup[0].ID != "uuid-1" {
		t.Errorf("slow index did not resolve lib-1 → uuid-1, got %+v", body.Lineup)
	}
}

func TestGetLineup_EmptyChannel400IsEmpty(t *testing.T) {
	// Phase-0 finding 4: Tunarr 400s on an unprogrammed channel's lineup. The
	// adapter must absorb that as an empty lineup, not an error.
	empty := testkit.Fixture(t, "tunarr/channel_lineup_empty.json") // {"error":"Bad Request"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(empty)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	slots, err := c.GetLineup(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("400-on-empty must be absorbed, got err: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("unprogrammed channel should yield 0 slots, got %d", len(slots))
	}
}

func TestDeleteChannel_404Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "cfg")
	if err := c.DeleteChannel(context.Background(), "gone"); err != nil {
		t.Fatalf("deleting an absent channel must be a no-op, got %v", err)
	}
}
