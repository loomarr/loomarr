package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeFiller records sync/tag calls.
type fakeFiller struct{ syncs, tags int }

func (f *fakeFiller) Sync(context.Context) (int, int, int, int, error) {
	f.syncs++
	return 4, 2, 1, 0, nil
}
func (f *fakeFiller) Tag(context.Context) (int, int, int, int, error) {
	f.tags++
	return 3, 2, 1, 0, nil
}

func newFillerServer(t *testing.T) (*httptest.Server, store.Store, *fakeFiller) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/f.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ff := &fakeFiller{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:  st,
		Auth:   api.NewTokenAuthorizer(adminToken),
		Log:    slog.New(slog.DiscardHandler),
		Filler: ff,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, ff
}

func seedClip(t *testing.T, st store.Store, id string, kind filler.Kind, era int, aud filler.Audience, cat string) {
	t.Helper()
	c := store.Clip{}
	c.TunarrProgramID = id
	c.Name = "clip " + id
	c.Kind = kind
	c.Era = era
	c.Audience = aud
	c.Category = cat
	c.DurationMs = 30000
	if err := st.UpsertClip(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestListFiller_FiltersAndVisibleToAll(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "c1", filler.Commercial, 1992, filler.Kids, "cereal")
	seedClip(t, st, "c2", filler.Commercial, 1994, filler.Kids, "toys")
	seedClip(t, st, "b1", filler.Bumper, 1992, filler.General, "")

	resp := do(t, srv, http.MethodGet, "/v1/filler?kind=commercial", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d", resp.StatusCode)
	}
	var body struct {
		Clips []struct {
			TunarrProgramID, Kind string
			Tagged                bool
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Clips) != 2 {
		t.Errorf("kind=commercial = %d, want 2", len(body.Clips))
	}
	for _, c := range body.Clips {
		if !c.Tagged {
			t.Errorf("fully-tagged clip %s reported untagged", c.TunarrProgramID)
		}
	}
}

func TestPatchClip_RequiresAdmin(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "u1", filler.Commercial, 0, "", "")
	resp := do(t, srv, http.MethodPatch, "/v1/filler/u1", "", `{"era":1994,"audience":"kids","category":"cereal"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member patch → %d, want 403", resp.StatusCode)
	}
}

func TestPatchClip_AdminEditsTags(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "u1", filler.Commercial, 0, "", "")
	resp := do(t, srv, http.MethodPatch, "/v1/filler/u1", adminToken, `{"era":1994,"audience":"kids","category":"cereal"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin patch → %d", resp.StatusCode)
	}
	var body struct {
		Era      int
		Audience string
		Tagged   bool
		AITagged bool
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Era != 1994 || body.Audience != "kids" || !body.Tagged {
		t.Errorf("patch didn't apply: %+v", body)
	}
	if body.AITagged {
		t.Error("a manual edit should clear the AI-tagged flag")
	}
	// Missing clip → 404.
	resp = do(t, srv, http.MethodPatch, "/v1/filler/nope", adminToken, `{"era":1990}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patch missing → %d, want 404", resp.StatusCode)
	}
}

func TestSyncFiller_AdminOnly(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	// Member → 403.
	if resp := do(t, srv, http.MethodPost, "/v1/filler/sync", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member sync → %d, want 403", resp.StatusCode)
	}
	// Admin → runs.
	resp := do(t, srv, http.MethodPost, "/v1/filler/sync", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin sync → %d", resp.StatusCode)
	}
	var body struct{ Total, Added, Pruned int }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Total != 4 || body.Added != 2 {
		t.Errorf("sync result = %+v", body)
	}
	if ff.syncs != 1 {
		t.Errorf("sync invoked %d times, want 1", ff.syncs)
	}
}

func TestTagFiller_AdminOnly(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	if resp := do(t, srv, http.MethodPost, "/v1/filler/tag", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member tag → %d, want 403", resp.StatusCode)
	}
	resp := do(t, srv, http.MethodPost, "/v1/filler/tag", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin tag → %d", resp.StatusCode)
	}
	if ff.tags != 1 {
		t.Errorf("tag invoked %d times, want 1", ff.tags)
	}
}
