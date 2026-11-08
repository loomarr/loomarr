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
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	archiveprovider "github.com/loomarr/loomarr/internal/filler/source/archive"
	youtubeprovider "github.com/loomarr/loomarr/internal/filler/source/youtube"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// serverWithClips builds a server over a real SQLite store seeded with clips, so the
// per-source counts are counted rather than stubbed — the read-model's whole job.
func serverWithClips(t *testing.T, cfg map[string]string, clips []clipstore.Clip) *httptest.Server {
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
		LiveSettings: stringSettings(func(k string) string { return cfg[k] }),

		SourceProviders: testSourceProviders(t),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func clip(path, source string) clipstore.Clip {
	var c clipstore.Clip
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
	srv, st, ff := newFillerServer(t)
	if err := st.UpsertFillerSource(context.Background(), records.NewFillerSource(
		"archive:classic_tv_commercials", "archive", "classic_tv_commercials", "Classic TV Commercials", time.Now(),
	)); err != nil {
		t.Fatal(err)
	}
	ff.sourceSuggestions = []source.Suggestion{{
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
	srv, st, ff := newFillerServer(t)
	ff.resolvedSource = source.Suggestion{
		Provider: "archive", TargetType: "collection", CanonicalID: "classic_tv_commercials",
		CanonicalURL: "https://archive.org/details/classic_tv_commercials", Title: "Classic TV Commercials",
		PreviewItems: []source.PreviewItem{
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
	srv, st, ff := newFillerServer(t)
	ff.resolvedSource = source.Suggestion{
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
	srv, st, ff := newFillerServer(t)
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
	srv, st, ff := newFillerServer(t)
	ff.resolvedSource = source.Suggestion{
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
	srv, st, ff := newFillerServer(t)
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
	srv, _, ff := newFillerServer(t)
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
	srv, st, _ := newFillerServer(t)
	src := records.NewFillerSource("archive:local", "archive", "local", "Local collection", time.Now().UTC())
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

func TestFillerSources_ProjectsTypedLastCheckOutcomesWithoutCursorDetails(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := t.Context()
	src := records.NewFillerSource(
		"youtube:outcomes", "youtube", "https://www.youtube.com/@outcomes/videos", "Outcome channel", time.Now().UTC(),
	)
	if err := st.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	checkedAt := time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)
	leaseUntil := checkedAt.Add(30 * time.Minute)
	claimed, err := st.ClaimFillerSourceCheck(ctx, src.ID, time.Time{}, checkedAt, leaseUntil)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if err := st.CompleteFillerSourceCheck(ctx, src.ID, leaseUntil, filler.SourceCheckCompletion{
		CheckedAt: checkedAt,
		Checkpoint: source.Checkpoint{
			Cursor: "internal-cursor", PendingWatermark: "internal-newest",
		},
		ReplaceOutcomes: true,
		Outcomes: source.Outcomes{
			source.OutcomeQueued: 3, source.OutcomeTooLong: 12,
			source.OutcomeUnavailable: 2,
		},
	}); err != nil {
		t.Fatal(err)
	}

	var projected api.FillerSourceDTO
	for _, source := range getSources(t, srv).Sources {
		if source.ID == src.ID {
			projected = source
			break
		}
	}
	if projected.LastCheck == nil || projected.LastCheck.Queued != 3 ||
		projected.LastCheck.TooLong != 12 || projected.LastCheck.Unavailable != 2 {
		t.Fatalf("last check = %+v, want typed outcome counts", projected.LastCheck)
	}
}

// The read-model's reason for existing: counts come from the CATALOG, not from a table.
func TestFillerSources_CountsClipsByProvenance(t *testing.T) {
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []clipstore.Clip{
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
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []clipstore.Clip{held})
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
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	registered := records.NewFillerSource(
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
	srv := serverWithClips(t, map[string]string{"filler.dir": "/data/filler"}, []clipstore.Clip{
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
		LiveSettings: stringSettings(func(k string) string { return cfg[k] }),

		SourceProviders: testSourceProviders(t),
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

// The drop-folder switch is read through its typed handle, driven by a REAL settings service.
//
// ⚠ A map[string]string stub is what once hid a kind-mismatch panic: the route read the bool key
// through a string accessor, so GET /v1/filler/sources died with an empty reply on every real
// install while every stubbed test passed. Wiring the real service keeps the read honest — a
// fake that cannot answer the wrong kind would only prove the fake does not panic. Sabotage it by
// hardcoding folderEnabled in the route.
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
		LiveSettings: svc,
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
	srv, _, _ := newFillerServer(t)
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
	srv, _, _ := newFillerServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch", adminToken, "")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("fetch without a source → %d, want 422", resp.StatusCode)
	}
}

// The beta's per-source "Fetch now" only ran the local catalog scan. For remote Archive/YouTube
// rows that meant a successful 200 with no download ever queued — exactly the reported symptom.
func TestFillerSources_FetchNowRunsAcquisitionBeforeCatalogSync(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	ff.fetchResult.Outcomes = source.Outcomes{
		source.OutcomeQueued: 2, source.OutcomeTooLong: 4,
	}
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
		SourceID      string                           `json:"sourceId"`
		SourcesPolled int                              `json:"sourcesPolled"`
		Queued        int                              `json:"queued"`
		MaxPerCheck   int                              `json:"maxPerCheck"`
		Outcomes      *api.FillerSourceCheckSummaryDTO `json:"outcomes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.SourceID != "archive:classic" || body.SourcesPolled != 1 || body.Queued != 2 || body.MaxPerCheck != 7 ||
		body.Outcomes == nil || body.Outcomes.Queued != 2 || body.Outcomes.TooLong != 4 {
		t.Fatalf("fetch result = %+v, want selected source identity and its acquisition outcome", body)
	}
}

func TestFillerSources_FetchNowReportsAnActiveCheck(t *testing.T) {
	srv, _, ff := newFillerServer(t)
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
	srv, _, ff := newFillerServer(t)
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
	srv, _, ff := newFillerServer(t)
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
	srv, _, ff := newFillerServer(t)
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
	srv, _, ff := newFillerServer(t)
	ff.fetchErr = api.ErrIngestUnavailable

	resp := do(t, srv, http.MethodPost, "/v1/filler/sources/fetch?id=archive%3Aclassic", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("fetch without ingest tooling = %d, want 409", resp.StatusCode)
	}
	if ff.syncs != 0 {
		t.Fatalf("catalog synced %d times after acquisition failed, want 0", ff.syncs)
	}
}

// testSourceProviders is the registry a server under test publishes: the production providers'
// shapes, with no network behind them.
func testSourceProviders(t *testing.T) *source.Registry {
	t.Helper()
	registry, err := source.NewRegistry(
		archiveprovider.New(archiveprovider.Config{}),
		youtubeprovider.New(nil),
		source.Folder(),
		source.Library(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// The web asks the server which providers exist and what each can do, instead of carrying its own
// list: a provider added to the registry shows up with no client change.
func TestListFillerProviders_PublishesTheRegistry(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET providers → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var got struct {
		Providers []api.FillerProviderDescriptionDTO `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(got.Providers))
	byID := map[string]api.FillerProviderDescriptionDTO{}
	for i, p := range got.Providers {
		ids[i] = p.ID
		byID[p.ID] = p
	}
	if strings.Join(ids, ",") != "archive,youtube,folder,library" {
		t.Fatalf("provider ids = %v, want the registry's order", ids)
	}
	archive := byID["archive"]
	if archive.Label != "Archive.org" || !archive.Fetched || !archive.Listable || !archive.Searchable || !archive.Discoverable || archive.OpenSearchable {
		t.Errorf("archive = %+v, want fetched, listable, searchable and discoverable", archive)
	}
	if youtube := byID["youtube"]; !youtube.Fetched || !youtube.Listable || youtube.Searchable || !youtube.Discoverable {
		t.Errorf("youtube = %+v, want fetched and discoverable, not searchable in place", youtube)
	}
	folder := byID["folder"]
	if folder.Fetched || folder.Listable || folder.Searchable || folder.Discoverable || !folder.Addable {
		t.Errorf("folder = %+v, want scanned in place, never fetched, and addable by hand", folder)
	}
}

func TestListFillerProviders_IsAdminOnly(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/filler/providers", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member GET providers → %d, want 403", resp.StatusCode)
	}
}

// Everything the source page used to carry per provider arrives on the descriptor.
func TestListFillerProviders_CarriesTheWordsTheSourcePageShows(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers", adminToken, "")
	var got struct {
		Providers []api.FillerProviderDescriptionDTO `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	byID := map[string]api.FillerProviderDescriptionDTO{}
	for _, p := range got.Providers {
		byID[p.ID] = p
	}
	youtube := byID["youtube"]
	if youtube.SourceNoun != "YouTube channel or playlist" || youtube.FindMinQuery != 3 ||
		youtube.FindLabel != "Find a YouTube channel or playlist" || youtube.Blurb == "" || youtube.PreviewHeading == "" {
		t.Errorf("youtube = %+v", youtube)
	}
	if folder := byID["folder"]; folder.AddressHint == "" || folder.AddressLabel != "Folder path" || folder.FindLabel != "" {
		t.Errorf("folder = %+v, want an address field and no finder", folder)
	}
}

// An address is checked, not searched for: the finder answers with the one source it names.
func TestFillerSourceSuggestions_CheckAnAddressInsteadOfSearchingForIt(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	ff.resolvedSource = source.Suggestion{
		Provider: "archive", TargetType: "collection", CanonicalID: "classic_tv_commercials",
		CanonicalURL: "https://archive.org/details/classic_tv_commercials", Title: "Classic TV Commercials",
		PreviewItems: []source.PreviewItem{{Title: "First ad", URL: "https://archive.org/details/ad_one"}},
	}
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers/archive/suggestions?q=classic_tv_commercials", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET suggestions → %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var body struct {
		Exact       bool                            `json:"exact"`
		Suggestions []api.FillerSourceSuggestionDTO `json:"suggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Exact || len(body.Suggestions) != 1 || len(body.Suggestions[0].PreviewItems) != 1 {
		t.Fatalf("body = %+v", body)
	}
	if got := body.Suggestions[0].PreviewItems[0].EmbedURL; got != "https://archive.org/embed/ad_one?autoplay=1" {
		t.Errorf("embed = %q, want the provider's player", got)
	}
	if len(ff.sourceSuggestionCalls) != 0 {
		t.Fatalf("an address was searched for: %#v", ff.sourceSuggestionCalls)
	}

	words := do(t, srv, http.MethodGet, "/v1/filler/providers/archive/suggestions?q=cartoons", adminToken, "")
	if words.StatusCode != http.StatusOK || len(ff.sourceSuggestionCalls) != 1 {
		t.Fatalf("a word was not searched for: %d %#v", words.StatusCode, ff.sourceSuggestionCalls)
	}
}

// The provider decides how short a search may be, so the route no longer needs one number for all.
func TestFillerSourceSuggestions_HoldAProviderToItsOwnMinimum(t *testing.T) {
	srv, _, ff := newFillerServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/filler/providers/youtube/suggestions?q=tv", adminToken, "")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("2-character YouTube search → %d, want 422", resp.StatusCode)
	}
	if len(ff.sourceSuggestionCalls) != 0 {
		t.Fatalf("a too-short search reached the finder: %#v", ff.sourceSuggestionCalls)
	}
}

// A provider the registry does not know is refused by the registry, not by a list in the route.
func TestFillerProviderRoutes_RefuseAnUnknownProviderByRegistry(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/filler/providers/nonesuch/suggestions?q=classic", adminToken, ""); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("suggestions for an unknown provider → %d, want 422", resp.StatusCode)
	}
	if resp := do(t, srv, http.MethodPatch, "/v1/filler/providers/nonesuch", adminToken, `{"enabled":false}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PATCH an unknown provider → %d, want 404", resp.StatusCode)
	}
}

func TestFillerSources_LinkARegisteredSourceToItsOwnPage(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertFillerSource(t.Context(), records.NewFillerSource(
		"archive:tv_ads", "archive", "tv_ads", "TV Ads", time.Now().UTC(),
	)); err != nil {
		t.Fatal(err)
	}
	for _, source := range getSources(t, srv).Sources {
		if source.ID == "archive:tv_ads" {
			if source.PageURL != "https://archive.org/details/tv_ads" {
				t.Fatalf("pageUrl = %q", source.PageURL)
			}
			return
		}
	}
	t.Fatal("registered source missing from the list")
}
