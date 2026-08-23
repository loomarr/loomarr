package programmer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/programmer"
	"github.com/loomarr/loomarr/internal/testkit"
)

// fillerMock is a Tunarr double for the §10 filler endpoints: `local` media-source
// create/get, per-library scan, program listing, and filler-list CRUD + channel
// attach. It records writes so tests can assert enumerate-first idempotency and the
// full-program-object echo.
type fillerMock struct {
	mu sync.Mutex

	sourceExists       bool     // whether the local source is already registered
	dir                string   // the registered local source's path
	programs           []any    // /programs payload (raw program objects)
	sourcePOSTs        int      // POST /api/media-sources count
	scans              int      // per-library scan count
	fillerLists        []any    // GET /api/filler-lists payload
	lastListBody       []byte   // last POST/PUT /api/filler-lists body
	fillerPOSTs        int      // create filler-list count
	fillerPUTs         int      // update filler-list count
	channelPUTs        int      // channel attach PUTs
	attachedList       string   // filler-list id attached to the channel
	attachedProgramIDs []string // program ids currently in the fl-1 list (idempotency check)
}

func newFillerMock(dir string) *fillerMock { return &fillerMock{dir: dir} }

func (m *fillerMock) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			out := []map[string]any{}
			if m.sourceExists {
				out = append(out, map[string]any{
					"id": "src-filler", "type": "local", "name": "Loomarr Filler",
					"paths": []string{m.dir},
				})
			}
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			m.sourcePOSTs++
			m.sourceExists = true
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "src-filler"})
		}
	})

	mux.HandleFunc("/api/media-sources/src-filler", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "src-filler", "type": "local",
			"libraries": []map[string]any{{"id": "lib-filler", "externalKey": m.dir}},
		})
	})
	mux.HandleFunc("/api/media-sources/src-filler/libraries/lib-filler/scan", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.scans++
		m.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/media-libraries/lib-filler/programs", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(m.programs)
	})

	mux.HandleFunc("/api/filler-lists", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(m.fillerLists)
		case http.MethodPost:
			m.fillerPOSTs++
			m.lastListBody, _ = io.ReadAll(r.Body)
			m.attachedProgramIDs = programIDsFromListBody(m.lastListBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "fl-1"})
		}
	})
	// GET /api/filler-lists/fl-1/programs — the attached list's current contents
	// (drives the content-based idempotency check).
	mux.HandleFunc("/api/filler-lists/fl-1/programs", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		out := make([]map[string]string, len(m.attachedProgramIDs))
		for i, id := range m.attachedProgramIDs {
			out[i] = map[string]string{"id": id}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	// PUT /api/filler-lists/fl-1 — update in place.
	mux.HandleFunc("/api/filler-lists/fl-1", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Method == http.MethodPut {
			m.fillerPUTs++
			m.lastListBody, _ = io.ReadAll(r.Body)
			m.attachedProgramIDs = programIDsFromListBody(m.lastListBody)
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/channels/ch-1", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			collections := []any{}
			if m.attachedList != "" {
				collections = append(collections, map[string]any{
					"id": m.attachedList, "weight": 1, "cooldownSeconds": 0,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ch-1", "name": "Test", "fillerCollections": collections,
			})
		case http.MethodPut:
			m.channelPUTs++
			var body struct {
				FillerColls []map[string]any `json:"fillerCollections"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			// Real Tunarr requires each fillerCollections entry to carry id + weight
			// + cooldownSeconds; a bare {id} → 400. Enforce it so the test catches a
			// missing-field attach (live-smoke bug).
			for _, fc := range body.FillerColls {
				_, hasW := fc["weight"]
				_, hasC := fc["cooldownSeconds"]
				if fc["id"] == nil || !hasW || !hasC {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"Bad Request"}`))
					return
				}
			}
			if len(body.FillerColls) > 0 {
				m.attachedList, _ = body.FillerColls[0]["id"].(string)
			}
			w.WriteHeader(http.StatusOK)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// programIDsFromListBody extracts the program ids from a POST/PUT /api/filler-lists
// body (so the mock can serve them back as the list's contents).
func programIDsFromListBody(b []byte) []string {
	var body struct {
		Programs []struct {
			ID string `json:"id"`
		} `json:"programs"`
	}
	_ = json.Unmarshal(b, &body)
	ids := make([]string, len(body.Programs))
	for i, p := range body.Programs {
		ids[i] = p.ID
	}
	return ids
}

// localProgramFixture is a scanned filler clip's /programs entry (§10).
func localProgramFixture(id, title string, durationMs int64) map[string]any {
	return map[string]any{
		"id": id,
		"program": map[string]any{
			"title": title, "duration": durationMs,
			"sourceType": "local", "type": "other_video",
		},
	}
}

// EnsureLocalFillerSource registers the source when absent, then scans per-library,
// and is a no-op create on a second call (enumerate-first idempotency).
func TestEnsureLocalFillerSource_Idempotent(t *testing.T) {
	m := newFillerMock("/drop")
	c := programmer.New(m.server(t).URL, "cfg")

	res, err := c.EnsureLocalFillerSource(context.Background(), "/drop")
	if err != nil {
		t.Fatal(err)
	}
	if !res.SourceAdded || res.SourceID != "src-filler" {
		t.Fatalf("first ensure: %+v, want SourceAdded + src-filler", res)
	}
	if m.sourcePOSTs != 1 || m.scans != 1 {
		t.Errorf("first ensure: posts=%d scans=%d, want 1/1", m.sourcePOSTs, m.scans)
	}

	// Second call: source already registered → no create (idempotent).
	res2, err := c.EnsureLocalFillerSource(context.Background(), "/drop")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.AlreadyWired() {
		t.Errorf("second ensure should be a no-op create, got %+v", res2)
	}
	if m.sourcePOSTs != 1 {
		t.Errorf("second ensure created a duplicate source: posts=%d", m.sourcePOSTs)
	}
}

// EnsureFillerList builds a list from program uuids, echoing the FULL program
// object (Tunarr rejects a minimal {id,duration}), and attaches it to the channel.
func TestEnsureFillerList_EchoesProgramAndAttaches(t *testing.T) {
	m := newFillerMock("/drop")
	m.sourceExists = true
	m.programs = []any{
		localProgramFixture("clip-a", "Frosted Flakes", 30000),
		localProgramFixture("clip-b", "TMNT figures", 30000),
	}
	c := programmer.New(m.server(t).URL, "cfg")

	if err := c.EnsureFillerList(context.Background(), "ch-1", []string{"clip-a", "clip-b"}); err != nil {
		t.Fatal(err)
	}
	if m.fillerPOSTs != 1 {
		t.Fatalf("filler-list not created: posts=%d", m.fillerPOSTs)
	}
	// The POST body must echo the full program object (not a minimal {id,duration}).
	var body struct {
		Name     string `json:"name"`
		Programs []struct {
			Type    string          `json:"type"`
			ID      string          `json:"id"`
			Program json.RawMessage `json:"program"`
		} `json:"programs"`
	}
	if err := json.Unmarshal(m.lastListBody, &body); err != nil {
		t.Fatalf("filler-list body not JSON: %v", err)
	}
	if body.Name != "loomarr:ch-1" {
		t.Errorf("filler-list name = %q, want loomarr:ch-1", body.Name)
	}
	if len(body.Programs) != 2 {
		t.Fatalf("filler-list programs = %d, want 2", len(body.Programs))
	}
	for _, p := range body.Programs {
		if p.Type != "content" || len(p.Program) == 0 || string(p.Program) == "null" {
			t.Errorf("filler-list entry missing full program object: %+v", p)
		}
	}
	// The list was attached to the channel.
	if m.channelPUTs == 0 || m.attachedList != "fl-1" {
		t.Errorf("filler-list not attached to channel: puts=%d attached=%q", m.channelPUTs, m.attachedList)
	}
}

func TestEnsureFillerList_UsesLiveFillerPolicyPerOperation(t *testing.T) {
	srv := testkit.NewTunarrHTTP(t, testkit.TunarrHTTPConfig{FillerProgramID: "clip"})

	cfg := programmer.Config{BaseURL: srv.URL, FillerWeight: 2, FillerCooldownSeconds: 15}
	client := programmer.NewDynamic(func() programmer.Config { return cfg })
	if err := client.EnsureFillerList(context.Background(), "channel", []string{"clip"}); err != nil {
		t.Fatal(err)
	}
	cfg.FillerWeight = 7
	cfg.FillerCooldownSeconds = 90
	if err := client.EnsureFillerList(context.Background(), "channel", []string{"clip"}); err != nil {
		t.Fatal(err)
	}

	want := []testkit.TunarrFillerPolicy{
		{Weight: 2, CooldownSeconds: 15},
		{Weight: 7, CooldownSeconds: 90},
	}
	if got := srv.FillerPolicies("channel"); !reflect.DeepEqual(got, want) {
		t.Errorf("same-channel filler policy history = %+v, want %+v", got, want)
	}
}

// Regression: idempotency must compare CONTENTS, not just count. A re-tagged
// catalog can yield a different but equal-sized pool; a count-only check would
// wrongly no-op and leave stale commercials attached forever.
func TestEnsureFillerList_EqualCountDifferentPoolUpdates(t *testing.T) {
	m := newFillerMock("/drop")
	m.sourceExists = true
	m.programs = []any{
		localProgramFixture("clip-a", "A", 30000),
		localProgramFixture("clip-b", "B", 30000),
		localProgramFixture("clip-c", "C", 30000),
	}
	// A list already exists with the same COUNT (2) but different ids.
	m.fillerLists = []any{map[string]any{"id": "fl-1", "name": "loomarr:ch-1", "contentCount": 2}}
	m.attachedProgramIDs = []string{"clip-a", "clip-b"}
	m.attachedList = "fl-1"
	c := programmer.New(m.server(t).URL, "cfg")

	// New desired pool is also size 2 but a different set → must UPDATE, not no-op.
	if err := c.EnsureFillerList(context.Background(), "ch-1", []string{"clip-a", "clip-c"}); err != nil {
		t.Fatal(err)
	}
	if m.fillerPUTs == 0 {
		t.Fatal("equal-count-but-different pool was skipped (count-only idempotency bug)")
	}
	if len(m.attachedProgramIDs) != 2 || m.attachedProgramIDs[1] != "clip-c" {
		t.Errorf("list not updated to the new pool: %v", m.attachedProgramIDs)
	}
}

// Idempotency: an UNCHANGED pool (same ids) makes no write.
func TestEnsureFillerList_UnchangedPoolNoWrite(t *testing.T) {
	m := newFillerMock("/drop")
	m.sourceExists = true
	m.programs = []any{
		localProgramFixture("clip-a", "A", 30000),
		localProgramFixture("clip-b", "B", 30000),
	}
	m.fillerLists = []any{map[string]any{"id": "fl-1", "name": "loomarr:ch-1", "contentCount": 2}}
	m.attachedProgramIDs = []string{"clip-a", "clip-b"}
	m.attachedList = "fl-1"
	c := programmer.New(m.server(t).URL, "cfg")

	if err := c.EnsureFillerList(context.Background(), "ch-1", []string{"clip-a", "clip-b"}); err != nil {
		t.Fatal(err)
	}
	if m.fillerPOSTs != 0 || m.fillerPUTs != 0 || m.channelPUTs != 0 {
		t.Errorf("unchanged pool made writes: posts=%d puts=%d attach=%d", m.fillerPOSTs, m.fillerPUTs, m.channelPUTs)
	}
}

// An empty program set with no existing list is a no-op (nothing to detach).
func TestEnsureFillerList_EmptyNoOp(t *testing.T) {
	m := newFillerMock("/drop")
	m.sourceExists = true
	c := programmer.New(m.server(t).URL, "cfg")

	if err := c.EnsureFillerList(context.Background(), "ch-1", nil); err != nil {
		t.Fatal(err)
	}
	if m.fillerPOSTs != 0 || m.channelPUTs != 0 {
		t.Errorf("empty pool should make no writes: posts=%d puts=%d", m.fillerPOSTs, m.channelPUTs)
	}
}
