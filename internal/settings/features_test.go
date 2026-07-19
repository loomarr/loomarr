package settings

import (
	"context"
	"testing"
)

// featureService builds a Service over the REAL registry with the given db
// overrides, so gating is tested against the actual RequiredFor declarations.
func featureService(t *testing.T, db map[string]string) *Service {
	t.Helper()
	s, err := New(context.Background(), NewRegistry(), fakeLoader{m: db}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(string) (string, bool) { return "", false } // no env pins in gating tests
	return s
}

// Nothing configured → no gated feature is available (config-design §7).
func TestFeatures_EmptyIsAllOff(t *testing.T) {
	f := featureService(t, nil).Features()
	if f.Acquisition || f.Suggestions || f.Filler {
		t.Errorf("empty config should gate everything off, got %+v", f)
	}
}

// Suggestions need BOTH an LLM provider AND TMDB grounding (config-design §7).
// (llm.provider has a default of "ollama", so the missing piece is tmdb.api_key.)
func TestFeatures_SuggestionsNeedTMDB(t *testing.T) {
	// llm.provider defaults to ollama (set), but tmdb.api_key is empty → gated off.
	if featureService(t, nil).Features().Suggestions {
		t.Error("suggestions should be off without TMDB grounding")
	}
	// Add TMDB → on.
	if !featureService(t, map[string]string{"tmdb.api_key": "tmdbkey"}).Features().Suggestions {
		t.Error("suggestions should be on with a provider + TMDB")
	}
}

// Acquisition is the OR gate: Seerr alone works; Sonarr alone does NOT (needs the
// Radarr pair); Sonarr+Radarr works (config-design §7).
func TestFeatures_AcquisitionOrGate(t *testing.T) {
	cases := []struct {
		name string
		db   map[string]string
		want bool
	}{
		{"nothing", nil, false},
		{"seerr only", map[string]string{"seerr.url": "http://seerr:5055"}, true},
		{"sonarr only", map[string]string{"sonarr.url": "http://sonarr:8989"}, false},
		{"sonarr+radarr", map[string]string{"sonarr.url": "http://sonarr:8989", "radarr.url": "http://radarr:7878"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := featureService(t, c.db).Features().Acquisition; got != c.want {
				t.Errorf("Acquisition = %v, want %v", got, c.want)
			}
		})
	}
}

// Filler needs the drop-folder (config-design §7).
func TestFeatures_Filler(t *testing.T) {
	if featureService(t, nil).Features().Filler {
		t.Error("filler off without a dir")
	}
	if !featureService(t, map[string]string{"filler.dir": "/data/filler"}).Features().Filler {
		t.Error("filler on with a dir")
	}
}

// Available() reflects the set and treats ungated features as always-on.
func TestFeatureSet_Available(t *testing.T) {
	f := FeatureSet{Suggestions: true}
	if !f.Available(FeatureSuggestions) || f.Available(FeatureFiller) {
		t.Error("Available should mirror the set")
	}
	if !f.Available(FeatureNone) {
		t.Error("ungated feature should be available")
	}
}

// UserSync gates a CONDITIONALLY REGISTERED route: with no media server,
// POST /v1/users/sync does not exist at runtime, yet it IS in the committed spec
// (generated in schema-only mode, where every route registers). The generated client
// therefore offers a call that 404s unless the UI checks this flag first — which is the
// bug this feature exists to close.
//
// The condition must mirror app.go's wiring EXACTLY (`library.flavor` set). If someone
// rewires app.go to a different key, this test still passes while the flag silently lies,
// so the assertion is pinned to the key name deliberately: change one, this comment
// sends you to the other.
func TestFeatures_UserSyncTracksMediaServerWiring(t *testing.T) {
	if featureService(t, nil).Features().UserSync {
		t.Error("no media server configured → user sync must be off (the route is not registered)")
	}
	if !featureService(t, map[string]string{"library.flavor": "emby"}).Features().UserSync {
		t.Error("library.flavor set → user sync must be on; app.go registers the route on this exact key")
	}
}

// Ingest is the one gate NOT derived from settings completeness (config-design §7): it
// depends on whether the running IMAGE carries yt-dlp + ffmpeg. The probe is injected so
// this never touches the filesystem (CLAUDE.md: unit tests stay off the disk).
func TestFeatures_IngestNeedsBothToolsPresent(t *testing.T) {
	svc := func(db map[string]string, present ...string) *Service {
		s := featureService(t, db)
		set := map[string]bool{}
		for _, p := range present {
			set[p] = true
		}
		s.execProbe = func(path string) bool { return set[path] }
		return s
	}
	paths := map[string]string{"ingest.ytdlp_path": "/bin/yt-dlp", "ingest.ffmpeg_path": "/bin/ffmpeg"}

	// loomarr:latest — no tool paths configured at all.
	if svc(nil).Features().Ingest {
		t.Error("no tool paths → ingest must be off (this is the default image)")
	}
	// Configured but missing on disk: off, and NOT a boot failure — the image is
	// otherwise perfectly usable without ingest.
	if svc(paths).Features().Ingest {
		t.Error("paths configured but absent on disk → ingest must be off")
	}
	// Only one tool present. yt-dlp cannot merge video+audio without ffmpeg, so a
	// half-present image would accept the request and fail mid-download on exactly the
	// high-resolution sources most worth fetching.
	if svc(paths, "/bin/yt-dlp").Features().Ingest {
		t.Error("yt-dlp without ffmpeg → ingest must be off")
	}
	if svc(paths, "/bin/ffmpeg").Features().Ingest {
		t.Error("ffmpeg without yt-dlp → ingest must be off")
	}
	// Both present — the loomarr:filler image.
	if !svc(paths, "/bin/yt-dlp", "/bin/ffmpeg").Features().Ingest {
		t.Error("both tools present → ingest must be on (this is loomarr:filler)")
	}
}
