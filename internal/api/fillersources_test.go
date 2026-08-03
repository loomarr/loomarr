package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

// serverWithClips builds a server over a real SQLite store seeded with clips, so the
// per-source counts are counted rather than stubbed — the read-model's whole job.
func serverWithClips(t *testing.T, cfg map[string]string, clips []store.Clip) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, c := range clips {
		if err := st.UpsertClip(context.Background(), c); err != nil {
			t.Fatalf("seed clip %s: %v", c.Path, err)
		}
	}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:      st,
		Auth:       api.NewTokenAuthorizer(adminToken),
		Log:        slog.New(slog.DiscardHandler),
		LiveConfig: func(k string) string { return cfg[k] },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func clip(path, source string) store.Clip {
	var c store.Clip
	// Hash AND Path — identity is the hash since V38c; a Path-only clip has an empty id, and the
	// store keys on it, so every row would collide on "".
	c.Hash = path
	c.Path = path
	c.Name = path
	c.Kind = filler.Commercial
	c.Source = source
	c.DurationMs = 30000
	c.UpdatedAt = time.Unix(1_700_000_000, 0).UTC()
	return c
}

type sourcesBody struct {
	Sources []api.FillerSourceDTO `json:"sources"`
	Total   int                   `json:"total"`
}

func getSources(t *testing.T, srv *httptest.Server) sourcesBody {
	t.Helper()
	resp := do(t, srv, http.MethodGet, "/v1/filler/sources", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET sources → %d, want 200", resp.StatusCode)
	}
	var body sourcesBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// The read-model's reason for existing: counts come from the CATALOG, not from a table.
func TestFillerSources_CountsClipsByProvenance(t *testing.T) {
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []store.Clip{
		clip("a.mp4", "filler-dir"),
		clip("b.mp4", "filler-dir"),
		clip("c.mp4", "library"),
	})
	body := getSources(t, srv)

	byKind := map[string]api.FillerSourceDTO{}
	for _, s := range body.Sources {
		byKind[s.Kind] = s
	}
	if byKind["folder"].Count != 2 {
		t.Errorf("folder count = %d, want 2", byKind["folder"].Count)
	}
	if byKind["library"].Count != 1 {
		t.Errorf("library count = %d, want 1", byKind["library"].Count)
	}
	if body.Total != 3 {
		t.Errorf("total = %d, want 3", body.Total)
	}
}

// ⚠ Total is sent rather than summed client-side: a clip whose `source` matches no known row
// still belongs to the catalog, and a client adding up the rows would under-report it.
func TestFillerSources_TotalIncludesUnrecognizedProvenance(t *testing.T) {
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []store.Clip{
		clip("a.mp4", "filler-dir"),
		clip("weird.mp4", "hand-copied-by-an-operator"),
	})
	body := getSources(t, srv)

	var summed int
	for _, s := range body.Sources {
		summed += s.Count
	}
	if summed != 1 {
		t.Fatalf("rows summed to %d, want 1 (the unrecognized clip matches no row)", summed)
	}
	if body.Total != 2 {
		t.Errorf("total = %d, want 2 — the catalog includes clips no row claims", body.Total)
	}
}

// An unconfigured source is RETURNED with configured:false, not omitted. "No drop-folder
// configured" is the answer to "why is my catalog empty"; hiding the row leaves that unanswered.
func TestFillerSources_UnconfiguredSourceIsShownNotHidden(t *testing.T) {
	srv := serverWithClips(t, map[string]string{}, nil) // no filler.dir
	body := getSources(t, srv)

	var folder *api.FillerSourceDTO
	for i := range body.Sources {
		if body.Sources[i].Kind == "folder" {
			folder = &body.Sources[i]
		}
	}
	if folder == nil {
		t.Fatal("the folder row must be present even when unconfigured")
	}
	if folder.Configured {
		t.Error("configured must be false with no filler.dir")
	}
	if folder.Target == "" {
		t.Error("an unconfigured row still needs a target to render")
	}
	if folder.Fetchable {
		t.Error("an unconfigured folder cannot be fetched")
	}
}

// filler.dir hot-applies, so the row must read it live — a value captured at construction
// would report the old folder on the very screen an operator checks after changing it.
func TestFillerSources_ReadsTheDirLive(t *testing.T) {
	cfg := map[string]string{"filler.dir": "/data/filler"}
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:      st,
		Auth:       api.NewTokenAuthorizer(adminToken),
		Log:        slog.New(slog.DiscardHandler),
		LiveConfig: func(k string) string { return cfg[k] },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	if got := getSources(t, srv).Sources[0].Target; got != "/data/filler" {
		t.Fatalf("target = %q, want the configured dir", got)
	}
	cfg["filler.dir"] = "/srv/clips" // the operator changes it; no restart
	if got := getSources(t, srv).Sources[0].Target; got != "/srv/clips" {
		t.Errorf("target = %q after a settings change, want /srv/clips — the dir must be read live", got)
	}
}

// The drop-folder switch is read through the BOOL seam, driven by a REAL settings service.
//
// ⚠ A map[string]string stub is what hid this: the route read the bool key through
// LiveConfig (settings.String), which PANICS on a non-string Kind, so GET /v1/filler/sources
// died with an empty reply on every real install while every stubbed test passed. Wiring the
// real service is the point of this test — a fake that cannot panic would only prove the fake
// does not panic. Sabotage it by pointing folderEnabled back at s.liveConfig.
func TestFillerSources_FolderSwitchReadsTheRealSettingsService(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := st.ListSettings(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]settings.SettingRow, len(rows))
		for i, r := range rows {
			out[i] = settings.SettingRow{Key: r.Key, Value: r.Value, UpdatedBy: r.UpdatedBy}
		}
		return out, nil
	}}
	svc, err := settings.New(ctx, settings.NewRegistry(), loader, nil)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		// Wired exactly as the composition root wires them, typed per Kind.
		LiveConfig: func(k string) string { return svc.String(k) },
		LiveConfigBoolOn: func(k string) bool {
			if b, ok := svc.Resolve(k).Value.(bool); ok {
				return b
			}
			return true
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Declared default is true, so the folder row reports itself enabled.
	folder := sourceOfKind(t, getSources(t, srv), "folder")
	if !folder.Enabled {
		t.Error("folder source reads disabled on a fresh install, want enabled (declared default is true)")
	}

	// And the switch is actually read: turning it off must reach the row. SetDB is the
	// same hot-apply path a settings save takes, so no restart is involved here either.
	svc.SetDB(map[string]string{"filler.source.folder.enabled": "false"})
	folder = sourceOfKind(t, getSources(t, srv), "folder")
	if folder.Enabled {
		t.Error("folder source still reads enabled after the switch was turned off")
	}
}

func sourceOfKind(t *testing.T, body sourcesBody, kind string) api.FillerSourceDTO {
	t.Helper()
	for _, s := range body.Sources {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no %q source in %+v", kind, body.Sources)
	return api.FillerSourceDTO{}
}

// Admin-only: the rows name filesystem paths and library targets, which is infrastructure
// detail a member has no business reading.
func TestFillerSources_RequiresAdmin(t *testing.T) {
	srv := serverWithClips(t, nil, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/filler/sources"},
		{http.MethodPost, "/v1/filler/sources/fetch"},
	} {
		resp := do(t, srv, tc.method, tc.path, "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// With no filler service wired, Fetch now reports 501 rather than pretending to work.
func TestFillerSources_FetchWithoutAServiceIs501(t *testing.T) {
	srv := serverWithClips(t, nil, nil)
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("fetch with no filler service → %d, want 501", resp.StatusCode)
	}
}
