package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/requester"
)

// arrDiscovery is api.ArrDiscovery over the settings: Seerr's Sonarr and Radarr, read per call
// so a just-saved Seerr connection is the one asked.
type arrDiscovery struct{ set resolved }

func (d arrDiscovery) ArrServers(ctx context.Context) ([]api.DiscoveredArr, error) {
	// Seerr is the requester unless the direct arrs are chosen, and only Seerr knows the arrs.
	if settings.RequesterProvider.Get(d.set) == "arr" || settings.SeerrURL.Get(d.set) == "" {
		return nil, nil
	}
	servers, err := requester.NewSeerrDynamic(d.set.seerrConn()).ArrServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.DiscoveredArr, 0, len(servers))
	for _, s := range servers {
		out = append(out, api.DiscoveredArr{App: s.App, URLs: s.URLs, APIKey: s.APIKey})
	}
	return out, nil
}

func (d arrDiscovery) Reachable(ctx context.Context, app, url, apiKey string) error {
	return requester.ArrReachable(ctx, app, url, apiKey)
}
