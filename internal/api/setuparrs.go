package api

import (
	"context"
	"net/http"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

// arrSettingKeys are one arr's connection settings.
var arrSettingKeys = map[string]struct{ url, apiKey settings.StringKey }{
	"sonarr": {settings.SonarrURL, settings.SonarrAPIKey},
	"radarr": {settings.RadarrURL, settings.RadarrAPIKey},
}

type arrFromRequesterDTO struct {
	App       string `json:"app" enum:"sonarr,radarr"`
	URL       string `json:"url" doc:"The address Loomarr now uses for it"`
	Filled    bool   `json:"filled" doc:"Loomarr saved this connection now; false when one was already set"`
	Reachable bool   `json:"reachable" doc:"Loomarr reached it at URL with the key"`
}

type arrsFromRequesterOutput struct {
	Body struct {
		Arrs []arrFromRequesterDTO `json:"arrs" doc:"One per arr the requester knows; empty when it knows none"`
	}
}

// arrsFromRequester fills in each arr Loomarr has no address for from the one Seerr uses (#2151).
// The first address Loomarr can reach wins; when it reaches none, the first is saved anyway, so
// the step shows it with its key and the operator only corrects the address.
func (s *Server) arrsFromRequester(ctx context.Context, _ *struct{}) (*arrsFromRequesterOutput, error) {
	out := &arrsFromRequesterOutput{}
	out.Body.Arrs = []arrFromRequesterDTO{}
	if s.arrDiscovery == nil || s.settings == nil {
		return out, nil
	}
	servers, err := s.arrDiscovery.ArrServers(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't ask Seerr",
			"Loomarr couldn't read Seerr's Sonarr and Radarr. Check the Seerr connection, or fill them in by hand.", err)
	}
	for _, server := range servers {
		keys, known := arrSettingKeys[server.App]
		if !known || len(server.URLs) == 0 {
			continue
		}
		if current := s.configValue(keys.url); current != "" {
			out.Body.Arrs = append(out.Body.Arrs, arrFromRequesterDTO{App: server.App, URL: current})
			continue
		}
		key := s.configValue(keys.apiKey)
		if key == "" {
			key = server.APIKey
		}
		pick := arrFromRequesterDTO{App: server.App, URL: server.URLs[0], Filled: true}
		for _, candidate := range server.URLs {
			if s.arrDiscovery.Reachable(ctx, server.App, candidate, key) == nil {
				pick.URL, pick.Reachable = candidate, true
				break
			}
		}
		edits := map[string]string{keys.url.Key(): pick.URL}
		if s.configValue(keys.apiKey) == "" {
			edits[keys.apiKey.Key()] = server.APIKey
		}
		for _, result := range s.settings.Patch(ctx, edits, auditActor(ctx)) {
			if result.Status != "saved" {
				// Pinned by the environment, or refused: Loomarr leaves that arr as it is.
				pick.Filled = false
			}
		}
		out.Body.Arrs = append(out.Body.Arrs, pick)
	}
	return out, nil
}
