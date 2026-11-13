package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// A timezone stored before guide.timezone was validated must not be served on: every client reads
// it, and a browser's Intl throws on a name Go happily loads ("Local") or never heard of. The
// settings layer heals it to unset on read, so both routes that carry the household's zone, the
// small /v1/household read and the Guide, tell the same thing: none.
func TestStoredBadTimezoneIsNotServedToClients(t *testing.T) {
	for _, tc := range []struct{ stored, want string }{
		{"Mars/Olympus", ""},
		{"not a zone", ""},
		{"Local", ""},
		{" Local ", ""},
		{"", ""},
		{"Pacific/Auckland", "Pacific/Auckland"},
		{" America/New_York ", "America/New_York"},
		{"UTC", "UTC"},
	} {
		t.Run(tc.stored, func(t *testing.T) {
			svc, err := settings.New(context.Background(), settings.NewRegistry(),
				settings.StoreLoader{List: func(context.Context) ([]settings.SettingRow, error) {
					return []settings.SettingRow{{Key: "guide.timezone", Value: tc.stored, UpdatedBy: "test"}}, nil
				}}, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			srv := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
				return api.Router(defaults.Log, api.Options{
					Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, LiveSettings: svc,
				})
			}).Server

			resp := do(t, srv, http.MethodGet, "/v1/household", memberToken, "")
			defer func() { _ = resp.Body.Close() }()
			var household struct {
				TimeZone string `json:"timeZone"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&household); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || household.TimeZone != tc.want {
				t.Errorf("/v1/household with %q stored = %d %q, want %q", tc.stored, resp.StatusCode, household.TimeZone, tc.want)
			}
			if guide := getGuide(t, srv, ""); guide.Timezone != tc.want {
				t.Errorf("/v1/guide with %q stored = %q, want %q", tc.stored, guide.Timezone, tc.want)
			}
		})
	}
}
