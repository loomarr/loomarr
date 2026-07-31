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
	"strings"
	"testing"
	"time"

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
	// discovered records the queries Discover was asked for; discoverErr forces the
	// upstream-failure path.
	discovered    []string
	discoverLimit int
	discoverErr   error
}

func (f *fakeFiller) Sync(context.Context) (int, int, int, int, error) {
	f.syncs++
	return 4, 2, 1, 0, nil
}
func (f *fakeFiller) Tag(context.Context) (int, int, int, int, error) {
	f.tags++
	return 3, 2, 1, 0, nil
}

// Discover records the query so a test can prove the handler passes it through, and returns a
// total LARGER than the item count — the real API pages, and a fake that returned len(items)
// would let a handler reporting the page length pass.
func (f *fakeFiller) Discover(_ context.Context, query string, limit int) ([]api.DiscoveredClip, int, error) {
	f.discovered = append(f.discovered, query)
	f.discoverLimit = limit
	if f.discoverErr != nil {
		return nil, 0, f.discoverErr
	}
	return []api.DiscoveredClip{
		{ID: "cm-1993-4", Title: "Commercials 1993", Year: 1993, URL: "https://archive.org/details/cm-1993-4"},
		{ID: "no-year-item", Title: "Untitled reel", URL: "https://archive.org/details/no-year-item"},
	}, 54, nil
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

// Era suggestions (§10, V34): the list surfaces an unconfirmed suggestion, and
// PATCHing era CONFIRMS it — the suggestion clears in the same write.
func TestPatchClip_ConfirmsEraSuggestion(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "u1", filler.Commercial, 0, "", "")
	if err := st.UpdateClipTags(context.Background(), "u1", 0, "kids", "cereal", 1985, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	// The suggestion rides the DTO so the UI can ask the question.
	resp := do(t, srv, http.MethodGet, "/v1/filler", adminToken, "")
	var list struct {
		Clips []struct {
			Path         string
			Era          int
			SuggestedEra int `json:"suggestedEra"`
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Clips) != 1 || list.Clips[0].SuggestedEra != 1985 || list.Clips[0].Era != 0 {
		t.Fatalf("suggestion not surfaced: %+v", list.Clips)
	}
	// Confirm: era lands, suggestion clears.
	resp = do(t, srv, http.MethodPatch, "/v1/filler/u1", adminToken, `{"era":1985}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm patch → %d", resp.StatusCode)
	}
	var body struct {
		Era          int
		SuggestedEra int `json:"suggestedEra"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Era != 1985 || body.SuggestedEra != 0 {
		t.Errorf("confirm did not clear the suggestion: %+v", body)
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

// --- discovery (§10, V33) ---

func decodeDiscover(t *testing.T, resp *http.Response) struct {
	Items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Year  int    `json:"year"`
		URL   string `json:"url"`
	} `json:"items"`
	Total       int    `json:"total"`
	LicenceNote string `json:"licenceNote"`
} {
	t.Helper()
	var body struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Year  int    `json:"year"`
			URL   string `json:"url"`
		} `json:"items"`
		Total       int    `json:"total"`
		LicenceNote string `json:"licenceNote"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestDiscoverFiller_ReturnsCandidatesWithTheSourcesTotal(t *testing.T) {
	srv, _, ff := newFillerServer(t)

	resp := do(t, srv, http.MethodGet, "/v1/filler/discover?q=1980s+cereal+commercial", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeDiscover(t, resp)

	if len(body.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(body.Items))
	}
	// ⚠ Total is the SOURCE's match count, not the page length. An operator judging "is this
	// search any good" needs the real number — 54 hits shown 2 at a time is a different
	// situation from 2 hits total.
	if body.Total != 54 {
		t.Errorf("total = %d, want 54 (the source's count, not len(items))", body.Total)
	}
	if body.Total == len(body.Items) {
		t.Error("total equals the page length — it is reporting the page, not the search")
	}
	// The query reaches the service rather than being dropped.
	if len(ff.discovered) != 1 || ff.discovered[0] != "1980s cereal commercial" {
		t.Errorf("service saw %v, want the typed query", ff.discovered)
	}
}

// ⚠ The licence note is sent ONCE, about the search, not per row. archive.org declares a
// licence on ~8% of items and yt-dlp on none, so a per-result chip would read "unknown" on
// nearly every row — implying a per-item check that never happened (build plan §6.3).
func TestDiscoverFiller_StatesTheLicenceCaveatOnceNotPerItem(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	body := decodeDiscover(t, do(t, srv, http.MethodGet, "/v1/filler/discover?q=cereal", adminToken, ""))
	if body.LicenceNote == "" {
		t.Error("no licence note — an operator has no signal that licences are unknown")
	}
	// If a per-item licence field is ever added, this test should be revisited deliberately
	// rather than silently: assert the DTO has no such field today.
	raw, _ := json.Marshal(body.Items)
	if bytes.Contains(raw, []byte("licence")) || bytes.Contains(raw, []byte("license")) {
		t.Error("an item carries a licence field — §6.3 says it would read 'unknown' on nearly every row")
	}
}

// An item with no year is the common case (Solr omits the field), and it must round-trip as
// absent rather than as the year 0.
func TestDiscoverFiller_OmitsAnUnknownYear(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	resp := do(t, srv, http.MethodGet, "/v1/filler/discover?q=cereal", adminToken, "")
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"year":0`)) {
		t.Error(`an unknown year serialised as "year":0 — it should be omitted`)
	}
}

// Upstream failures are archive.org's, not the caller's: a 502-shaped problem that names which
// side broke rather than blaming the query.
func TestDiscoverFiller_UpstreamFailureIsABadGateway(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	ff.discoverErr = errors.New("dial tcp: connection refused")

	resp := do(t, srv, http.MethodGet, "/v1/filler/discover?q=cereal", adminToken, "")
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

// Admin-only: it names an outbound integration and feeds the ingest path.
func TestDiscoverFiller_IsAdminOnly(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	if resp := do(t, srv, http.MethodGet, "/v1/filler/discover?q=cereal", memberToken, ""); resp.StatusCode == http.StatusOK {
		t.Error("a member could search for clips to add")
	}
}

// An empty query would return archive.org's whole movies corpus ranked by nothing — refused at
// the schema, so the request never reaches the service.
func TestDiscoverFiller_RequiresAQuery(t *testing.T) {
	srv, _, ff := newFillerServer(t)

	resp := do(t, srv, http.MethodGet, "/v1/filler/discover", adminToken, "")
	if resp.StatusCode == http.StatusOK {
		t.Error("an empty query was accepted")
	}
	if len(ff.discovered) != 0 {
		t.Errorf("the service was called with %v despite an invalid request", ff.discovered)
	}
}
