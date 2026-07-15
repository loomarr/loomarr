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
