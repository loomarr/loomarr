package programmer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

	c := programmer.New(srv.URL, "", "cfg-uuid")
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

func TestEnsureChannel_Update_PutsToID(t *testing.T) {
	var got capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "", "cfg")
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

	c := programmer.New(srv.URL, "", "cfg")
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

	c := programmer.New(srv.URL, "", "cfg")
	ch, ok, err := c.GetChannel(context.Background(), "2540b613-10b1-4d78-ab35-ee48b313e359")
	if err != nil || !ok {
		t.Fatalf("GetChannel = ok=%v err=%v", ok, err)
	}
	if ch.Number != 9901 || ch.Name != "loomarr-phase0-spike" || ch.Group != "loomarr-test" {
		t.Errorf("parsed channel wrong: %+v", ch)
	}
}

func TestSetLineup_TranslatesSlots(t *testing.T) {
	var got capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "", "cfg")
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, LibraryItemID: "lib-1", DurationMs: 3600000},
		{Kind: schedule.SlotPending, DurationMs: 0},                             // → flex
		{Kind: schedule.SlotFiller, DurationMs: 30000},                          // no lib id → flex
		{Kind: schedule.SlotFiller, LibraryItemID: "clip-9", DurationMs: 30000}, // resolved → content
		{Kind: schedule.SlotFlex, DurationMs: 60000},                            // → flex
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
	if len(body.Lineup) != 5 {
		t.Fatalf("want 5 lineup items, got %d", len(body.Lineup))
	}
	want := []struct {
		typ string
		id  string
	}{
		{"content", "lib-1"},
		{"flex", ""},
		{"flex", ""},
		{"content", "clip-9"},
		{"flex", ""},
	}
	for i, w := range want {
		if body.Lineup[i].Type != w.typ || body.Lineup[i].ID != w.id {
			t.Errorf("item %d = {%s,%s}, want {%s,%s}", i, body.Lineup[i].Type, body.Lineup[i].ID, w.typ, w.id)
		}
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

	c := programmer.New(srv.URL, "", "cfg")
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

	c := programmer.New(srv.URL, "", "cfg")
	if err := c.DeleteChannel(context.Background(), "gone"); err != nil {
		t.Fatalf("deleting an absent channel must be a no-op, got %v", err)
	}
}

func TestAPIKey_SentWhenSet(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "secret-key", "cfg")
	_, _, _ = c.GetChannel(context.Background(), "x")
	if authHeader != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want Bearer secret-key", authHeader)
	}
}
