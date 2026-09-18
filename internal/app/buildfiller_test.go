package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFillerSourceAdapter_HotEnablesTunarrAnnotation(t *testing.T) {
	client := testkit.NewTunarr()
	enabled := false
	adapter := fillerSourceAdapter{
		prog:       client,
		configured: func() bool { return enabled },
	}
	if got, err := adapter.LocalClipIDsByName(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("disabled annotation = %v, %v; want empty success", got, err)
	}
	if client.FillerClipReads != 0 {
		t.Fatalf("disabled adapter made %d Tunarr calls", client.FillerClipReads)
	}

	enabled = true
	if _, err := adapter.LocalClipIDsByName(context.Background()); err != nil {
		t.Fatalf("enabled annotation: %v", err)
	}
	if client.FillerClipReads != 1 {
		t.Fatalf("enabled adapter made %d calls, want 1", client.FillerClipReads)
	}
}

func TestFetchStoreAdapter_InheritsInstallationLocationAndDisablesOutOfMarketSources(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	for _, tc := range []struct {
		id, country, market string
	}{
		{"us-wide", "US", ""},
		{"ny-local", "US", "New York"},
		{"california", "US", "California"},
		{"canadian", "CA", ""},
		{"unknown", "", ""},
	} {
		src := store.NewFillerSource(tc.id, "archive", tc.id, tc.id, time.Now().UTC())
		src.Geography = filler.Geography{Country: tc.country, Market: tc.market}
		if err := st.UpsertFillerSource(t.Context(), src); err != nil {
			t.Fatal(err)
		}
	}
	adapter := fetchStoreAdapter{
		st: st, fetchEvery: func() time.Duration { return time.Hour },
		home: func() filler.Geography { return filler.Geography{Country: "US", Market: "New York"} },
	}
	sources, err := adapter.ListFetchSources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, src := range sources {
		got[src.ID] = src.Enabled
	}
	californiaEnabled, californiaPresent := got["california"]
	canadianEnabled, canadianPresent := got["canadian"]
	// Sources without their own geography—including the built-in starters and the
	// explicit "unknown" row above—inherit the Installation location. Explicitly conflicting
	// sources stay visible to the fetcher as disabled so a manual check returns a refusal rather
	// than pretending that an absent source was checked successfully.
	if !got["us-wide"] || !got["ny-local"] || !got["unknown"] || !californiaPresent || californiaEnabled ||
		!canadianPresent || canadianEnabled {
		t.Fatalf("fetch sources = %v, want inherited/matching sources enabled and out-of-market sources disabled", got)
	}
}

func TestFetchStoreAdapter_DisablesRemoteSourcesUntilInstallationHasALocation(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	src := store.NewFillerSource("archive:local", "archive", "local", "Local", time.Now().UTC())
	if err := st.UpsertFillerSource(t.Context(), src); err != nil {
		t.Fatal(err)
	}
	adapter := fetchStoreAdapter{
		st: st, fetchEvery: func() time.Duration { return time.Hour },
		home: func() filler.Geography { return filler.Geography{} },
	}
	sources, err := adapter.ListFetchSources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.ID == src.ID {
			if source.Enabled {
				t.Fatal("source is enabled for downloading before the installation has a location")
			}
			return
		}
	}
	t.Fatalf("source %q disappeared instead of remaining visible as disabled", src.ID)
}

type fetchIngestorFunc func(context.Context, string, string, []string) (string, error)

func (f fetchIngestorFunc) IngestSource(
	ctx context.Context, sourceID, sourceKind string, urls []string,
) (string, error) {
	return f(ctx, sourceID, sourceKind, urls)
}

func TestFillerFetchJobSelectsDueGlobalAndPerSourcePoliciesThroughTheApplicationAdapter(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	st := testkit.MigratedSQLiteStore(t)

	// Keep the production starter rows out of this exact due-set assertion. The application
	// adapter still reads the real registry, policy columns, geography, and check-state columns.
	seeded, err := st.ListFillerSources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range seeded {
		if err := st.SetFillerSourceEnabled(t.Context(), source.ID, false); err != nil {
			t.Fatal(err)
		}
	}

	addSource := func(id string, every *int, lastChecked time.Time) {
		t.Helper()
		source := store.NewFillerSource(id, "archive", id, id, now.Add(-24*time.Hour))
		if err := st.UpsertFillerSource(t.Context(), source); err != nil {
			t.Fatal(err)
		}
		if every != nil {
			if err := st.SetFillerSourceFetchPolicy(t.Context(), id, every, nil); err != nil {
				t.Fatal(err)
			}
		}
		leaseUntil := lastChecked.Add(-time.Minute)
		claimed, err := st.ClaimFillerSourceCheck(
			t.Context(), id, time.Time{}, lastChecked.Add(-2*time.Minute), leaseUntil,
		)
		if err != nil || !claimed {
			t.Fatalf("seed check claim for %s = %v, %v", id, claimed, err)
		}
		if err := st.CompleteFillerSourceCheck(t.Context(), id, leaseUntil, lastChecked); err != nil {
			t.Fatal(err)
		}
	}

	everyHour, everyTwelveHours := 3600, 12*3600
	addSource("due-default", nil, now.Add(-6*time.Hour))
	addSource("not-due-custom", &everyTwelveHours, now.Add(-6*time.Hour))
	addSource("due-custom", &everyHour, now.Add(-2*time.Hour))

	var enumerated []string
	fetcher := filler.NewFetcher(
		fetchStoreAdapter{
			st:         st,
			fetchEvery: func() time.Duration { return 6 * time.Hour },
			home:       func() filler.Geography { return filler.Geography{Country: "US"} },
		},
		enumeratorFunc(func(_ context.Context, source filler.FetchSource, _ int) ([]filler.DiscoveredRef, int, error) {
			enumerated = append(enumerated, source.ID)
			return nil, 0, nil
		}),
		fetchIngestorFunc(func(context.Context, string, string, []string) (string, error) {
			t.Fatal("an empty provider listing must not enqueue an ingest")
			return "", nil
		}),
		filler.FetchLimits{
			MaxPerRun:       func() int { return 10 },
			MaxCatalogClips: func() int { return 2000 },
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).WithClock(func() time.Time { return now })

	result, err := fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range enumerated {
		got[id] = true
	}
	if result.SourcesPolled != 2 || !got["due-default"] || !got["due-custom"] || got["not-due-custom"] {
		t.Fatalf("scheduled pass = %+v, enumerated %v; want due global/default and due custom only", result, enumerated)
	}

	byID := map[string]store.FillerSource{}
	sources, err := st.ListFillerSources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		byID[source.ID] = source
	}
	if !byID["due-default"].LastCheckedAt.Equal(now) || !byID["due-custom"].LastCheckedAt.Equal(now) {
		t.Fatalf("due check state was not committed at the controlled clock: %+v", byID)
	}
	if !byID["not-due-custom"].LastCheckedAt.Equal(now.Add(-6 * time.Hour)) {
		t.Fatalf("not-due source was advanced: %+v", byID["not-due-custom"])
	}
}

func TestBuildFetcher_DownloadsIntoTheAppliedWatchFolder(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("LOOMARR_YTDLP_ARGS", argsFile)
	ytdlp := testkit.Executable(t, "yt-dlp", "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LOOMARR_YTDLP_ARGS\"\n")
	ffmpeg := testkit.Executable(t, "ffmpeg", "#!/bin/sh\nexit 0\n")
	clipDir := filepath.Join(t.TempDir(), "clips")
	watchDir := filepath.Join(t.TempDir(), "incoming")
	layout, err := filler.NewLayout(clipDir, watchDir)
	if err != nil {
		t.Fatal(err)
	}
	set := visionSet(t, map[string]string{
		"ingest.ytdlp_path":  ytdlp,
		"ingest.ffmpeg_path": ffmpeg,
	})
	governor, err := storagegovernor.NewFilesystem([]storagegovernor.ManagedRoot{
		{Path: layout.ClipDir(), Domain: storagegovernor.DomainFiller},
		{Path: layout.WatchDir(), Domain: storagegovernor.DomainFiller},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := buildFetcher(set, layout, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, governor)
	if fetcher == nil {
		t.Fatal("buildFetcher returned nil with both tools configured")
	}
	fetcher.Run(context.Background(), []clipfetch.Source{{Kind: clipfetch.YouTube, URL: "https://example.invalid/video"}})

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if !strings.Contains(args, watchDir+"/%(title)s [%(id)s].%(ext)s") {
		t.Errorf("yt-dlp args = %q, want output under applied watch %q", args, watchDir)
	}
	if strings.Contains(args, clipDir+"/%(title)s") {
		t.Errorf("yt-dlp args = %q, unexpectedly write raw arrivals into clip library %q", args, clipDir)
	}
}

// The hosted picker stores credentials under the branded provider, not the flattened `openai`
// wire kind. The filler language path must resolve that same active selection or it sends an
// unauthenticated request even though Settings says the provider is configured.
func TestHostedLanguageAsker_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"en"}}]}`))
	}))
	t.Cleanup(server.Close)

	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                server.URL,
		"llm.model":              "audio-model",
		"llm.api_key.openrouter": "provider-secret",
	})
	asker := hostedLanguageAsker(set, nil)
	if asker == nil {
		t.Fatal("hosted language asker is nil for a configured provider")
	}
	if _, err := asker.AskAboutAudio(context.Background(), filler.AudioAsk{
		Audio: []byte("audio"), Format: "wav", Prompt: "language?",
	}); err != nil {
		t.Fatalf("ask about audio: %v", err)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}

func TestHostedTranscriber_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Buy now.","duration":1,"segments":[{"start":0,"end":1,"text":"Buy now."}]}`))
	}))
	t.Cleanup(server.Close)

	// The seam under test is provider selection, not ffmpeg. This stand-in writes the requested
	// output path so the production HostedTranscriber reaches its HTTP client without requiring a
	// media fixture or weakening its extraction contract.
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nfor last; do :; done\nprintf wav > \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	set := visionSet(t, map[string]string{
		"playout.ffmpeg_path":        ffmpeg,
		"llm.provider":               "openai",
		"llm.hosted_provider":        "openrouter",
		"llm.url":                    server.URL,
		"llm.model":                  "openai/gpt-4o-mini",
		"llm.api_key.openrouter":     "provider-secret",
		"filler.transcribe.provider": "hosted",
		"filler.transcribe.model":    "openai/whisper-large-v3",
	})

	segments, err := buildFillerMediaTools(set, nil).Transcribe(context.Background(), "clip.mp4", 0, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Text != "Buy now." {
		t.Fatalf("segments = %+v", segments)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}

func TestFillerEnrichment_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(server.Close)

	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                server.URL,
		"llm.model":              "openai/gpt-4o-mini",
		"llm.api_key.openrouter": "provider-secret",
	})
	selection := activeFillerTextSelection(set, nil)
	if selection.Provider == nil {
		t.Fatal("enrichment provider is nil for configured OpenRouter")
	}
	if _, err := selection.Provider.Chat(context.Background(), nil, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}
