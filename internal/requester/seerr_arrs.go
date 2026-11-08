package requester

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ArrServer is a Sonarr or Radarr as Seerr knows it, so the wizard can fill in its connection
// instead of asking for it. URLs are the addresses to try in order: the one Seerr itself uses,
// then the one it shows people, when set.
type ArrServer struct {
	App    string
	URLs   []string
	APIKey string
}

type seerrArrSettings struct {
	Hostname    string `json:"hostname"`
	Port        int    `json:"port"`
	UseSSL      bool   `json:"useSsl"`
	BaseURL     string `json:"baseUrl"`
	ExternalURL string `json:"externalUrl"`
	APIKey      string `json:"apiKey"`
	Is4K        bool   `json:"is4k"`
	IsDefault   bool   `json:"isDefault"`
}

// ArrServers reads Seerr's default Sonarr and Radarr. An app Seerr has none of is left out.
func (s *Seerr) ArrServers(ctx context.Context) ([]ArrServer, error) {
	var out []ArrServer
	for _, app := range []string{"sonarr", "radarr"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+"/api/v1/settings/"+app, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", s.apiKey())
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err
		}
		var servers []seerrArrSettings
		err = json.NewDecoder(resp.Body).Decode(&servers)
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("seerr %s settings: status %d", app, resp.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("decode seerr %s settings: %w", app, err)
		}
		if server, ok := defaultArr(servers); ok {
			out = append(out, ArrServer{App: app, URLs: server.urls(), APIKey: server.APIKey})
		}
	}
	return out, nil
}

// defaultArr is the server Seerr sends ordinary requests to: its default non-4K one, else its
// first non-4K one, else its first.
func defaultArr(servers []seerrArrSettings) (seerrArrSettings, bool) {
	for _, s := range servers {
		if s.IsDefault && !s.Is4K {
			return s, true
		}
	}
	for _, s := range servers {
		if !s.Is4K {
			return s, true
		}
	}
	if len(servers) > 0 {
		return servers[0], true
	}
	return seerrArrSettings{}, false
}

func (s seerrArrSettings) urls() []string {
	var out []string
	if host := strings.TrimSpace(s.Hostname); host != "" {
		scheme := "http"
		if s.UseSSL {
			scheme = "https"
		}
		u := scheme + "://" + host
		if s.Port > 0 {
			u += ":" + strconv.Itoa(s.Port)
		}
		if base := strings.Trim(strings.TrimSpace(s.BaseURL), "/"); base != "" {
			u += "/" + base
		}
		out = append(out, u)
	}
	if external := strings.TrimRight(strings.TrimSpace(s.ExternalURL), "/"); external != "" && (len(out) == 0 || external != out[0]) {
		out = append(out, external)
	}
	return out
}

// ArrReachable probes one arr at url with key, as the Sonarr and Radarr checks do.
func ArrReachable(ctx context.Context, app, url, key string) error {
	conn := func() (string, string) { return url, key }
	none := func() (string, string) { return "", "" }
	conns := ArrConns{Sonarr: conn, Radarr: none}
	if app == "radarr" {
		conns = ArrConns{Sonarr: none, Radarr: conn}
	}
	return NewArr(conns).Reachable(ctx)
}
