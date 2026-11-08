package api

import (
	"context"
	"errors"
	"testing"
)

// patchOnlySettings records Patch; nothing else is called.
type patchOnlySettings struct {
	SettingsService
	patched map[string]string
	pinned  string
}

func (p *patchOnlySettings) Patch(_ context.Context, edits map[string]string, _ string) []SettingResult {
	var out []SettingResult
	for k, v := range edits {
		if k == p.pinned {
			out = append(out, SettingResult{Key: k, Status: "pinned"})
			continue
		}
		p.patched[k] = v
		out = append(out, SettingResult{Key: k, Status: "saved"})
	}
	return out
}

type fakeArrDiscovery struct {
	servers   []DiscoveredArr
	reachable map[string]bool
}

func (f fakeArrDiscovery) ArrServers(context.Context) ([]DiscoveredArr, error) { return f.servers, nil }

func (f fakeArrDiscovery) Reachable(_ context.Context, _, url, _ string) error {
	if f.reachable[url] {
		return nil
	}
	return errors.New("unreachable")
}

// Each arr Loomarr has no address for takes Seerr's: the first address Loomarr reaches, or the
// first one when it reaches none, with Seerr's key. A connection already set is left alone.
func TestArrsFromRequester(t *testing.T) {
	settings := &patchOnlySettings{patched: map[string]string{}}
	s := &Server{
		settings:     settings,
		liveSettings: mapSettings{"radarr.url": "http://radarr.invalid:7878"},
		arrDiscovery: fakeArrDiscovery{
			servers: []DiscoveredArr{
				{App: "sonarr", URLs: []string{"http://sonarr:8989", "https://sonarr.example.invalid"}, APIKey: "sonarr-key"},
				{App: "radarr", URLs: []string{"http://radarr:7878"}, APIKey: "radarr-key"},
			},
			reachable: map[string]bool{"https://sonarr.example.invalid": true},
		},
	}
	out, err := s.arrsFromRequester(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []arrFromRequesterDTO{
		{App: "sonarr", URL: "https://sonarr.example.invalid", Filled: true, Reachable: true},
		{App: "radarr", URL: "http://radarr.invalid:7878"},
	}
	if len(out.Body.Arrs) != len(want) {
		t.Fatalf("arrs = %+v, want %+v", out.Body.Arrs, want)
	}
	for i := range want {
		if out.Body.Arrs[i] != want[i] {
			t.Errorf("arr %d = %+v, want %+v", i, out.Body.Arrs[i], want[i])
		}
	}
	if settings.patched["sonarr.url"] != "https://sonarr.example.invalid" || settings.patched["sonarr.api_key"] != "sonarr-key" {
		t.Errorf("sonarr saved as %v", settings.patched)
	}
	if _, touched := settings.patched["radarr.url"]; touched {
		t.Error("radarr was already set and must not change")
	}

	// Unreachable everywhere: the first address is saved, so the operator only corrects it.
	settings.patched = map[string]string{}
	s.liveSettings = mapSettings{}
	s.arrDiscovery = fakeArrDiscovery{servers: []DiscoveredArr{{App: "sonarr", URLs: []string{"http://sonarr:8989"}, APIKey: "k"}}}
	out, err = s.arrsFromRequester(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Body.Arrs[0]; got.URL != "http://sonarr:8989" || !got.Filled || got.Reachable {
		t.Errorf("unreachable sonarr = %+v, want filled with its first address, not reachable", got)
	}

	// A connection the environment pins is not filled.
	settings.pinned = "sonarr.url"
	out, _ = s.arrsFromRequester(context.Background(), nil)
	if out.Body.Arrs[0].Filled {
		t.Error("a pinned sonarr.url must not read as filled")
	}
}
