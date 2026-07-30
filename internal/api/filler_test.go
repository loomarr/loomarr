package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeFiller records sync/tag calls.
type fakeFiller struct {
	syncs, tags int
	ingested    []string
	// unavailable simulates loomarr:latest — the image with no ingest tooling.
	unavailable bool
}

func (f *fakeFiller) Sync(context.Context) (int, int, int, int, error) {
	f.syncs++
	return 4, 2, 1, 0, nil
}
func (f *fakeFiller) Tag(context.Context) (int, int, int, int, error) {
	f.tags++
	return 3, 2, 1, 0, nil
}

func (f *fakeFiller) Ingest(_ context.Context, urls []string) (string, error) {
	if f.unavailable {
		return "", api.ErrIngestUnavailable
	}
	f.ingested = append(f.ingested, urls...)
	return "job-1", nil
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
	// Path is identity since §9.1; the Tunarr uuid rides alongside for filler-lists.
	c.Path = id
	c.TunarrProgramID = "tun-" + id
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
			Path, Kind string
			Tagged     bool
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Clips) != 2 {
		t.Errorf("kind=commercial = %d, want 2", len(body.Clips))
	}
	for _, c := range body.Clips {
		if !c.Tagged {
			t.Errorf("fully-tagged clip %s reported untagged", c.Path)
		}
	}
}

func TestPatchClip_RequiresAdmin(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "u1", filler.Commercial, 0, "", "")
	resp := do(t, srv, http.MethodPatch, "/v1/filler/u1", "", `{"era":1994,"audience":"kids","category":"cereal"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member patch → %d, want 401", resp.StatusCode)
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
	if resp := do(t, srv, http.MethodPost, "/v1/filler/sync", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member sync → %d, want 401", resp.StatusCode)
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
	if resp := do(t, srv, http.MethodPost, "/v1/filler/tag", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member tag → %d, want 401", resp.StatusCode)
	}
	resp := do(t, srv, http.MethodPost, "/v1/filler/tag", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin tag → %d", resp.StatusCode)
	}
	if ff.tags != 1 {
		t.Errorf("tag invoked %d times, want 1", ff.tags)
	}
}

// Clip search lives on /v1/filler, not /v1/search (§7.2). A clip is not a provisionable
// title, so it cannot be a federated Candidate without pushing a non-title through the
// LLM grounding path — the leak §10 exists to prevent.
func TestFiller_NameSearch(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "c1", filler.Commercial, 1992, filler.Kids, "cereal")
	seedClip(t, st, "c2", filler.Commercial, 1994, filler.Kids, "toys")

	resp := do(t, srv, http.MethodGet, "/v1/filler?q=C1", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Clips []struct {
			Path string `json:"path"`
		} `json:"clips"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive, and the result carries the Tunarr program id — the identity that
	// makes a search hit deep-linkable, which a title-shaped Candidate could not carry.
	if len(body.Clips) != 1 || body.Clips[0].Path != "c1" {
		t.Errorf("q=C1 → %+v, want exactly clip c1", body.Clips)
	}
}

// Kind is correctable by hand (§10): detection at sync mis-reads a trailer as a
// commercial often enough to matter, and kind drives pod ROLE, so a wrong kind produces
// structurally wrong pods rather than merely a mis-tagged clip.
func TestFiller_PatchCorrectsKind(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "t1", filler.Commercial, 1994, filler.Kids, "toys")

	resp := do(t, srv, http.MethodPatch, "/v1/filler/t1", adminToken,
		`{"era":1994,"audience":"kids","category":"toys","kind":"trailer"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch → %d, want 200", resp.StatusCode)
	}
	got, err := st.GetClip(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != filler.Trailer {
		t.Errorf("kind = %q, want trailer", got.Kind)
	}

	// Omitting kind must leave it alone, so a tag-only edit never rewrites it.
	resp = do(t, srv, http.MethodPatch, "/v1/filler/t1", adminToken, `{"era":1995,"audience":"kids","category":"toys"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tag-only patch → %d, want 200", resp.StatusCode)
	}
	if got, _ = st.GetClip(context.Background(), "t1"); got.Kind != filler.Trailer {
		t.Errorf("tag-only edit rewrote kind to %q, want it left as trailer", got.Kind)
	}
}

// Ingest is admin-only, returns a job id rather than blocking, and reports the
// image-variant gate as something a setting cannot fix.
func TestFiller_Ingest(t *testing.T) {
	srv, _, ff := newFillerServer(t)

	// §19 negative: downloading arbitrary URLs onto the host is admin-only.
	if resp := do(t, srv, http.MethodPost, "/v1/filler/ingest", "", `{"urls":["https://archive.org/details/x"]}`); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member ingest → %d, want 401", resp.StatusCode)
	}

	resp := do(t, srv, http.MethodPost, "/v1/filler/ingest", adminToken, `{"urls":["https://archive.org/details/x"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ingest → %d, want 200", resp.StatusCode)
	}
	var body struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// A job id, not a result: the download outlives the request (§10).
	if body.JobID == "" {
		t.Error("no jobId returned — progress is unwatchable without one")
	}
	if len(ff.ingested) != 1 {
		t.Errorf("ingested = %v, want the one URL passed through", ff.ingested)
	}
}

// On loomarr:latest the gate is NOT a configuration problem, and the error must not
// send the operator to a Settings page that cannot help them.
func TestFiller_IngestUnavailableOnDefaultImage(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	ff.unavailable = true

	resp := do(t, srv, http.MethodPost, "/v1/filler/ingest", adminToken, `{"urls":["https://youtube.com/playlist?list=x"]}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("ingest without tooling → %d, want 409", resp.StatusCode)
	}
	var problem struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	// The remedy is a different IMAGE. Naming it is the whole point of this branch.
	if !strings.Contains(problem.Detail, "loomarr:filler") {
		t.Errorf("detail = %q, want it to name the loomarr:filler image", problem.Detail)
	}
}
