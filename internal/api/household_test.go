package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Times told outside the Guide read the household's timezone from this small read, so a page like
// the Library does not load the whole Guide to learn one name. Any signed-in person may read it.
func TestHouseholdTellsItsTimezone(t *testing.T) {
	srv := newGuideHarness(t, guideHarnessConfig{Timezone: "Pacific/Auckland"}).Server
	resp := do(t, srv, http.MethodGet, "/v1/household", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/household as a member → %d, want 200", resp.StatusCode)
	}
	var body struct {
		TimeZone string `json:"timeZone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.TimeZone != "Pacific/Auckland" {
		t.Fatalf("timeZone = %q, want Pacific/Auckland", body.TimeZone)
	}
}

// With no timezone set the household has none to tell, and the field is empty rather than absent.
func TestHouseholdTimezoneIsEmptyWhenUnset(t *testing.T) {
	srv := newGuideHarness(t, guideHarnessConfig{}).Server
	resp := do(t, srv, http.MethodGet, "/v1/household", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["timeZone"]; !ok || v != "" {
		t.Fatalf("timeZone = %v (present %v), want an empty string", v, ok)
	}
}
