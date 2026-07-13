package testkit

import (
	"encoding/json"
	"testing"
)

// TestFixturesLoadable is a guard: the Phase-0 pinned captures must stay present
// and valid JSON. If a fixture is renamed or corrupted, this fails loudly (the
// parsers written in later phases depend on these exact files).
func TestFixturesLoadable(t *testing.T) {
	for _, rel := range []string{
		"radarr/test_webhook.json",
		"radarr/grab_webhook.json",
		"radarr/import_webhook.json",
		"sonarr/test_webhook.json",
		"emby/lookup_present.json",
		"emby/lookup_absent.json",
		"seerr/request_available_201.json",
	} {
		b := Fixture(t, rel)
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("fixture %q is not valid JSON: %v", rel, err)
		}
	}
}

// TestArrNamingQuirk pins the §6 contract fact discovered in Phase 0: the Radarr
// import webhook's eventType is the string "Download", not "Import". A parser
// regression that keys on the wrong string will fail here.
func TestArrNamingQuirk(t *testing.T) {
	var imp struct {
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(Fixture(t, "radarr/import_webhook.json"), &imp); err != nil {
		t.Fatal(err)
	}
	if imp.EventType != "Download" {
		t.Errorf("radarr import eventType = %q, want \"Download\" (§6 naming quirk)", imp.EventType)
	}
}
