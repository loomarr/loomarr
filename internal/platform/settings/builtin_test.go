package settings

import (
	"context"
	"testing"
)

// A secret nobody gave resolves to the value the build ships with (the bundled TMDB key,
// decision 51): readers get it, List calls it builtIn, and it is neither "set" nor previewed.
// The household's own key wins and is listed as theirs.
func TestList_BuiltInSecretIsUsedButNeverShown(t *testing.T) {
	full := NewRegistry()
	tmdb, _ := full.Get("tmdb.api_key")
	tmdb.Default = "shipped-key-90000001"
	provider, _ := full.Get("llm.provider")
	model, _ := full.Get("llm.model")
	reg := newRegistry([]Setting{tmdb, provider, model})
	p := newMemPersister()
	s, err := New(context.Background(), reg, fakeLoader{m: p.m}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(string) (string, bool) { return "", false }

	if got := TMDBAPIKey.Get(s); got != "shipped-key-90000001" {
		t.Fatalf("Get = %q, want the shipped key", got)
	}
	e := s.List(context.Background(), nil)[0]
	if !e.BuiltIn || e.Set || e.Preview != "" || e.Value != nil {
		t.Fatalf("shipped key listed as builtIn=%v set=%v preview=%q value=%v; want builtIn only",
			e.BuiltIn, e.Set, e.Preview, e.Value)
	}
	if missing := s.MissingRequired(FeatureSuggestions); containsKey(missing, "tmdb.api_key") {
		t.Fatalf("the shipped key should satisfy TMDB for suggestions, missing = %v", missing)
	}

	s.SetDB(map[string]string{"tmdb.api_key": "household-key-90000002"})
	if got := TMDBAPIKey.Get(s); got != "household-key-90000002" {
		t.Fatalf("Get = %q, want the household's key over the shipped one", got)
	}
	e = s.List(context.Background(), nil)[0]
	if e.BuiltIn || !e.Set || e.Preview != "…0002" {
		t.Fatalf("household key listed as builtIn=%v set=%v preview=%q; want set …0002", e.BuiltIn, e.Set, e.Preview)
	}
}

func containsKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}
