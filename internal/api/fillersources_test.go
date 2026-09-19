package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

// serverWithClips builds a server over a real SQLite store seeded with clips, so the
// per-source counts are counted rather than stubbed — the read-model's whole job.
func serverWithClips(t *testing.T, cfg map[string]string, clips []store.Clip) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/api.db")
	t.Cleanup(func() { _ = st.Close() })
	for _, c := range clips {
		if err := st.UpsertClip(context.Background(), c); err != nil {
			t.Fatalf("seed clip %s: %v", c.Path, err)
		}
	}
	layout, err := filler.NewLayout(cfg["filler.dir"], cfg["filler.watch_dir"])
	if err != nil {
		t.Fatalf("filler.NewLayout: %v", err)
	}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         api.NewTokenAuthorizer(adminToken),
		Log:          slog.New(slog.DiscardHandler),
		FillerLayout: layout,
		LiveConfig:   func(k string) string { return cfg[k] },
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

type sourceSuggestionsBody struct {
	Suggestions []api.FillerSourceSuggestionDTO `json:"suggestions"`
}

func TestFillerSourceSuggestionsAreReadOnlyAndMarkRegisteredCollections(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	if err := st.UpsertFillerSource(context.Background(), store.NewFillerSource(
		"archive:classic_tv_commercials", "archive", "classic_tv_commercials", "Classic TV Commercials", time.Now(),
	)); err != nil {
		t.Fatal(err)
	}
	ff.sourceSuggestions = []filler.SourceSuggestion{{
		Provider: "archive", TargetType: "collection", CanonicalID: "classic_tv_commercials",
		CanonicalURL: "https://archive.org/details/classic_tv_commercials", Title: "Classic TV Commercials", ItemCount: 8362,
	}}

	resp := do(t, srv, http.MethodGet, "/v1/filler/providers/archive/suggestions?q=classic+tv&limit=8", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET suggestions → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var body sourceSuggestionsBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Suggestions) != 1 || !body.Suggestions[0].AlreadyAdded || body.Suggestions[0].ItemCount != 8362 {
		t.Fatalf("suggestions = %#v", body.Suggestions)
	}
	if len(ff.sourceSuggestionCalls) != 1 || ff.sourceSuggestionCalls[0].Provider != "archive" || ff.sourceSuggestionCalls[0].Limit != 8 {
		t.Fatalf("finder calls = %#v", ff.sourceSuggestionCalls)
	}
	sources, err := st.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("search changed the registry: %#v", sources)
	}
}

func TestFillerSourceResolutionIsReadOnly(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	ff.resolvedSource = filler.SourceSuggestion{
		Provider: "archive", TargetType: "collection", CanonicalID: "classic_tv_commercials",
		CanonicalURL: "https://archive.org/details/classic_tv_commercials", Title: "Classic TV Commercials",
		PreviewItems: []filler.SourcePreviewItem{
			{Title: "First ad", URL: "https://archive.org/details/ad_one", DurationMS: 31500},
			{Title: "Second ad", URL: "https://archive.org/details/ad_two"},
		},
	}
	resp := do(t, srv, http.MethodPost, "/v1/filler/providers/archive/resolve", adminToken,
		`{"input":"https://archive.org/details/classic_tv_commercials"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST resolve → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var got api.FillerSourceSuggestionDTO
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.CanonicalID != "classic_tv_commercials" || got.AlreadyAdded || len(got.PreviewItems) != 2 ||
		got.PreviewItems[0].Title != "First ad" || got.PreviewItems[0].DurationMS != 31500 {
		t.Fatalf("resolved = %#v", got)
	}
	if sources, err := st.ListFillerSources(context.Background()); err != nil || len(sources) != 0 {
		t.Fatalf("resolution changed the registry: %#v, %v", sources, err)
	}
}

func TestAddArchiveSourceRevalidatesItsCanonicalTarget(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	ff.resolvedSource = filler.SourceSuggestion{
		Provider: "archive", TargetType: "collection", CanonicalID: "classic_tv_commercials",
		CanonicalURL: "https://archive.org/details/classic_tv_commercials", Title: "Classic TV Commercials",
	}
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources", adminToken,
		`{"kind":"archive","uri":"https://archive.org/details/classic_tv_commercials"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST source → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	if len(ff.sourceResolutionCalls) != 1 || ff.sourceResolutionCalls[0].Input != "classic_tv_commercials" {
		t.Fatalf("resolution calls = %#v", ff.sourceResolutionCalls)
	}
	sources, err := st.ListFillerSources(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %#v, %v", sources, err)
	}
	if sources[0].URI != "classic_tv_commercials" || sources[0].Label != "Classic TV Commercials" {
		t.Fatalf("registered source = %#v", sources[0])
	}
}

func TestAddArchiveSourcePersistsNothingWhenRevalidationFails(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	ff.sourceResolutionErr = filler.ErrInvalidSourceReference
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources", adminToken,
		`{"kind":"archive","uri":"one_video"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("POST invalid source → %d, want 422", resp.StatusCode)
	}
	if sources, err := st.ListFillerSources(context.Background()); err != nil || len(sources) != 0 {
		t.Fatalf("failed verification changed the registry: %#v, %v", sources, err)
	}
}

func TestAddYouTubeSourceRevalidatesAndStoresItsCanonicalTarget(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	ff.resolvedSource = filler.SourceSuggestion{
		Provider: "youtube", TargetType: "channel", CanonicalID: "UC-vault",
		CanonicalURL: "https://www.youtube.com/channel/UC-vault/videos", Title: "Broadcast Vault",
	}
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources", adminToken,
		`{"kind":"youtube","uri":"https://youtube.com/@broadcastvault"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST source → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	if len(ff.sourceResolutionCalls) != 1 || ff.sourceResolutionCalls[0].Provider != "youtube" {
		t.Fatalf("resolution calls = %#v", ff.sourceResolutionCalls)
	}
	sources, err := st.ListFillerSources(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %#v, %v", sources, err)
	}
	if sources[0].URI != "https://www.youtube.com/channel/UC-vault/videos" || sources[0].Label != "Broadcast Vault" {
		t.Fatalf("registered source = %#v", sources[0])
	}
}

func TestFillerSourceSuggestionsStopAtDisabledProvider(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st, ff := harness.Server, harness.Store, harness.Filler
	if err := st.SetFillerProviderEnabled(context.Background(), "archive", false); err != nil {
		t.Fatal(err)
	}
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers/archive/suggestions?q=classic", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("GET suggestions while off → %d, want 409", resp.StatusCode)
	}
	if len(ff.sourceSuggestionCalls) != 0 {
		t.Fatalf("disabled provider reached finder: %#v", ff.sourceSuggestionCalls)
	}
	add := do(t, srv, http.MethodPost, "/v1/filler/sources", adminToken,
		`{"kind":"archive","uri":"classic_tv_commercials"}`)
	if add.StatusCode != http.StatusConflict {
		t.Fatalf("POST source while off → %d, want 409", add.StatusCode)
	}
	if len(ff.sourceResolutionCalls) != 0 {
		t.Fatalf("disabled provider reached resolver: %#v", ff.sourceResolutionCalls)
	}
}

func TestYouTubeSourceSearchFailureKeepsManualInputActionable(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.sourceSuggestionErr = filler.ErrSourceProvider
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers/youtube/suggestions?q=retro+ads", adminToken, "")
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("GET YouTube suggestions → %d, want 502", resp.StatusCode)
	}
	var problem struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if problem.Detail != "YouTube search isn’t available right now. Paste a channel or playlist URL instead." {
		t.Fatalf("detail = %q", problem.Detail)
	}
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

func TestFillerSources_MissingInstallationLocationDoesNotPromiseAnAutomaticCheck(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	src := store.NewFillerSource("archive:local", "archive", "local", "Local collection", time.Now().UTC())
	if err := st.UpsertFillerSource(t.Context(), src); err != nil {
		t.Fatal(err)
	}

	var projected api.FillerSourceDTO
	for _, source := range getSources(t, srv).Sources {
		if source.ID == src.ID {
			projected = source
			break
		}
	}
	if projected.ID == "" {
		t.Fatal("registered source was not projected")
	}
	if projected.Readiness != api.FillerSourceNeedsLocation {
		t.Fatalf("readiness = %q, want needs_location", projected.Readiness)
	}
	if projected.AutomaticDownloads == nil || projected.AutomaticDownloads.NextCheckAt != "" {
		t.Fatalf("automatic downloads = %+v, want policy summary without a promised check", projected.AutomaticDownloads)
	}
	if strings.Contains(strings.ToLower(projected.Detail), "automatically") {
		t.Fatalf("detail = %q, must not promise automatic work while the source needs a location", projected.Detail)
	}
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

// A successful fetch deliberately holds every new clip for Incoming. The source row must report
// that work separately from playable clips or a full review queue looks exactly like a broken
// source that found nothing.
func TestFillerSources_CountsHeldClipsByProvenance(t *testing.T) {
	held := clip("incoming.mp4", "filler-dir")
	held.Held = true
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []store.Clip{held})
	body := getSources(t, srv)

	folder := sourceOfKind(t, body, "folder")
	if folder.Count != 0 {
		t.Errorf("folder playable count = %d, want 0", folder.Count)
	}
	if folder.Incoming != 1 {
		t.Errorf("folder Incoming count = %d, want 1 — fetched work must not disappear from its source row", folder.Incoming)
	}
}

func TestFillerSources_RollsHeldClipsIntoTheirRegisteredProvider(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	ctx := context.Background()
	registered := store.NewFillerSource(
		"archive:tv_ads", "archive", "tv_ads", "TV Ads", time.Unix(1_700_000_000, 0).UTC(),
	)
	if err := st.UpsertFillerSource(ctx, registered); err != nil {
		t.Fatal(err)
	}
	held := clip("tv-ad.mp4", registered.ID)
	held.Held = true
	if err := st.UpsertClip(ctx, held); err != nil {
		t.Fatal(err)
	}
	reel := clip("tv-ad-reel.mp4", registered.ID)
	reel.Held = true
	reel.IsComposite = true
	if err := st.UpsertClip(ctx, reel); err != nil {
		t.Fatal(err)
	}

	body := getSources(t, srv)
	byID := make(map[string]api.FillerSourceDTO, len(body.Sources))
	for _, source := range body.Sources {
		byID[source.ID] = source
	}
	if byID[registered.ID].Incoming != 1 {
		t.Errorf("TV Ads Incoming count = %d, want 1; the terminal compilation is retained but not pending", byID[registered.ID].Incoming)
	}
	if byID["provider:archive"].Incoming != 1 {
		t.Errorf("Archive.org Incoming roll-up = %d, want 1", byID["provider:archive"].Incoming)
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

// A saved filesystem-root change is desired state until restart. The operational Sources view
// must keep describing the applied generation, or its Fetch action would claim to target a path
// that scan and intake do not yet use.
func TestFillerSources_ReportsAppliedDirUntilRestart(t *testing.T) {
	cfg := map[string]string{"filler.dir": "/data/filler"}
	st := openTestStore(t, t.TempDir()+"/api.db")
	t.Cleanup(func() { _ = st.Close() })
	layout, err := filler.NewLayout(cfg["filler.dir"], "")
	if err != nil {
		t.Fatal(err)
	}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         api.NewTokenAuthorizer(adminToken),
		Log:          slog.New(slog.DiscardHandler),
		FillerLayout: layout,
		LiveConfig:   func(k string) string { return cfg[k] },
	})
	readSources := func() sourcesBody {
		req := httptest.NewRequest(http.MethodGet, "/v1/filler/sources", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET sources → %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body sourcesBody
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	if got := readSources().Sources[0].Target; got != "/data/filler" {
		t.Fatalf("target = %q, want the configured dir", got)
	}
	cfg["filler.dir"] = "/srv/clips" // saved desired state; this generation remains on /data/filler
	if got := readSources().Sources[0].Target; got != "/data/filler" {
		t.Errorf("target = %q after a settings change, want applied /data/filler until restart", got)
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
	st := openTestStore(t, t.TempDir()+"/api.db")
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
		FillerLayout: func() filler.Layout {
			layout, layoutErr := filler.NewLayout(svc.String("filler.dir"), svc.String("filler.watch_dir"))
			if layoutErr != nil {
				t.Fatalf("filler.NewLayout: %v", layoutErr)
			}
			return layout
		}(),
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
	harness := newFillerHarness(t)
	srv := harness.Server
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/filler/sources"},
		{http.MethodPost, "/v1/filler/sources/fetch"},
	} {
		resp := do(t, srv, tc.method, tc.path, memberToken, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as member → %d, want 403", tc.method, tc.path, resp.StatusCode)
		}
		resp = do(t, srv, tc.method, tc.path, "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// With no filler service wired, Fetch now reports 501 rather than pretending to work.
func TestFillerSources_FetchWithoutAServiceIs501(t *testing.T) {
	srv := serverWithClips(t, nil, nil)
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("fetch with no filler service → %d, want 501", resp.StatusCode)
	}
}

func TestFillerSources_FetchRequiresOneSelectedSource(t *testing.T) {
	harness := newFillerHarness(t)
	srv := harness.Server
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch", adminToken, "")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("fetch without a source → %d, want 422", resp.StatusCode)
	}
}

// The beta's per-source "Fetch now" only ran the local catalog scan. For remote Archive/YouTube
// rows that meant a successful 200 with no download ever queued — exactly the reported symptom.
func TestFillerSources_FetchNowRunsAcquisitionBeforeCatalogSync(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetch → %d, want 200", resp.StatusCode)
	}
	if ff.fetches != 1 || ff.syncs != 1 {
		t.Fatalf("fetches/syncs = %d/%d, want 1/1 so remote acquisition and local catalog refresh both run", ff.fetches, ff.syncs)
	}
	if len(ff.fetchedSourceIDs) != 1 || ff.fetchedSourceIDs[0] != "archive:classic" {
		t.Fatalf("fetched source ids = %v, want only the selected row", ff.fetchedSourceIDs)
	}
	var body struct {
		SourceID      string `json:"sourceId"`
		SourcesPolled int    `json:"sourcesPolled"`
		Queued        int    `json:"queued"`
		MaxPerCheck   int    `json:"maxPerCheck"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.SourceID != "archive:classic" || body.SourcesPolled != 1 || body.Queued != 2 || body.MaxPerCheck != 7 {
		t.Fatalf("fetch result = %+v, want selected source identity and its acquisition outcome", body)
	}
}

func TestFillerSources_FetchNowReportsAnActiveCheck(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.fetchErr = filler.ErrSourceCheckInProgress

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("fetch during an active check = %d, want 409", resp.StatusCode)
	}
	if ff.syncs != 0 {
		t.Fatalf("catalog synced %d times after the source was already claimed, want 0", ff.syncs)
	}
}

func TestFillerSources_FetchNowReportsTheEffectiveCapAndCatalogStop(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.fetchResult = filler.FetchResult{MaxPerCheck: 3, StoppedBy: "catalog"}

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("capacity-stopped fetch = %d, want 200", resp.StatusCode)
	}
	var body struct {
		MaxPerCheck int    `json:"maxPerCheck"`
		StoppedBy   string `json:"stoppedBy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.MaxPerCheck != 3 || body.StoppedBy != "catalog" {
		t.Fatalf("capacity-stopped result = %+v, want effective cap 3 and catalog stop", body)
	}
}

func TestFillerSources_FetchNowRefusesADisabledSourceBeforeSync(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.fetchErr = filler.ErrSourceDisabled

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("fetch for a disabled source = %d, want 409", resp.StatusCode)
	}
	if ff.syncs != 0 {
		t.Fatalf("catalog synced %d times after the source was refused, want 0", ff.syncs)
	}
}

func TestFillerSources_FetchNowReturnsNotFoundForRemovedSource(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.fetchErr = filler.ErrFetchSourceNotFound

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=removed", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("fetch for a removed source = %d, want 404", resp.StatusCode)
	}
	if ff.syncs != 0 {
		t.Fatalf("catalog synced %d times after missing-source refusal, want 0", ff.syncs)
	}
}

func TestFillerSources_FetchNowReportsUnavailableIngestTooling(t *testing.T) {
	harness := newFillerHarness(t)
	srv, ff := harness.Server, harness.Filler
	ff.fetchErr = api.ErrIngestUnavailable

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("fetch without ingest tooling = %d, want 409", resp.StatusCode)
	}
	if ff.syncs != 0 {
		t.Fatalf("catalog synced %d times after acquisition failed, want 0", ff.syncs)
	}
}
