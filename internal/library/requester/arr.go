package requester

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/metrics"
)

// Arr is the direct Sonarr/Radarr Requester (§6) — the alternative to Seerr, selected by
// requester.provider=arr. It routes movies to Radarr and series to Sonarr; the two arr apps
// share an identical /api/v3 shape (lookup → add, queue, system/status), so one adapter drives
// both, parameterized per media type. Connections resolve per call (config-design §3 hot-apply)
// exactly like Seerr, so saving a new URL/key takes effect on the next request without a restart.
//
// Overrides: quality profile + root folder are auto-picked (the first the arr reports) unless a
// setting pins them — zero-config by default, tunable when it matters.
type Arr struct {
	// sonarr/radarr each resolve (baseURL, apiKey); an empty URL means that arr isn't
	// configured, so a request for that media type fails (stays wanted for the reconciler).
	sonarr func() (baseURL, apiKey string)
	radarr func() (baseURL, apiKey string)
	// overrides resolve the optional pinned profile/root per app; "" ⇒ auto-pick.
	sonarrQualityProfile func() string
	sonarrRootFolder     func() string
	radarrQualityProfile func() string
	radarrRootFolder     func() string
	http                 *http.Client
}

// ArrConns bundles the per-app connection + override resolvers the composition root supplies.
type ArrConns struct {
	Sonarr               func() (baseURL, apiKey string)
	Radarr               func() (baseURL, apiKey string)
	SonarrQualityProfile func() string
	SonarrRootFolder     func() string
	RadarrQualityProfile func() string
	RadarrRootFolder     func() string
}

// NewArr builds the direct-arr requester from per-app connection resolvers.
func NewArr(c ArrConns) *Arr {
	return NewArrObserved(c, nil)
}

// NewArrObserved binds operational Sonarr/Radarr requests to one application generation.
func NewArrObserved(c ArrConns, recorder *metrics.Recorder) *Arr {
	return &Arr{
		sonarr: c.Sonarr, radarr: c.Radarr,
		sonarrQualityProfile: c.SonarrQualityProfile, sonarrRootFolder: c.SonarrRootFolder,
		radarrQualityProfile: c.RadarrQualityProfile, radarrRootFolder: c.RadarrRootFolder,
		http: httpx.NewNamedObserved(httpx.TargetArr, httpx.TimeoutArr, recorder),
	}
}

// arrEndpoint is the per-media-type routing: which arr, and its /api/v3 path segment + lookup
// id namespace. Radarr keys on tmdb; Sonarr keys on tvdb (falling back to tmdb when no tvdb id).
type arrEndpoint struct {
	baseURL, apiKey string
	kind            string // "movie" | "series" — the /api/v3/<kind> path segment
	term            string // lookup term, e.g. "tmdb:603" or "tvdb:81189"
	qualityOverride string
	rootOverride    string
}

// endpointFor resolves the arr + lookup term for a title, or an error when the needed arr isn't
// configured or the title lacks a usable id.
func (a *Arr) endpointFor(t provision.Title) (arrEndpoint, error) {
	switch t.MediaType {
	case provision.Movie:
		base, key := a.radarr()
		if strings.TrimSpace(base) == "" {
			return arrEndpoint{}, fmt.Errorf("arr: Radarr not configured (needed for movie %q)", t.Name)
		}
		if t.TMDBID <= 0 {
			return arrEndpoint{}, fmt.Errorf("arr: movie %q has no TMDB id", t.Name)
		}
		return arrEndpoint{
			baseURL: strings.TrimRight(base, "/"), apiKey: key, kind: "movie",
			term:            "tmdb:" + strconv.Itoa(t.TMDBID),
			qualityOverride: resolve(a.radarrQualityProfile), rootOverride: resolve(a.radarrRootFolder),
		}, nil
	case provision.Series:
		base, key := a.sonarr()
		if strings.TrimSpace(base) == "" {
			return arrEndpoint{}, fmt.Errorf("arr: Sonarr not configured (needed for series %q)", t.Name)
		}
		// Sonarr looks up by tvdb; fall back to tmdb when the title carries no tvdb id.
		term := ""
		switch {
		case t.TVDBID > 0:
			term = "tvdb:" + strconv.Itoa(t.TVDBID)
		case t.TMDBID > 0:
			term = "tmdb:" + strconv.Itoa(t.TMDBID)
		default:
			return arrEndpoint{}, fmt.Errorf("arr: series %q has no tvdb/tmdb id", t.Name)
		}
		return arrEndpoint{
			baseURL: strings.TrimRight(base, "/"), apiKey: key, kind: "series",
			term:            term,
			qualityOverride: resolve(a.sonarrQualityProfile), rootOverride: resolve(a.sonarrRootFolder),
		}, nil
	default:
		return arrEndpoint{}, fmt.Errorf("arr: unsupported media type %q", t.MediaType)
	}
}

// Request implements Requester against Sonarr/Radarr (§6). It looks the title up on the arr
// (to obtain the arr's own payload for the add), then POSTs it monitored + searching, with the
// resolved quality profile + root folder. Idempotent: an already-added title (409, or the arr's
// already-exists validation 400) is success, matching Seerr's 2xx-or-conflict rule.
func (a *Arr) Request(ctx context.Context, t provision.Title) error {
	ep, err := a.endpointFor(t)
	if err != nil {
		return err
	}

	// 1. Look the title up on the arr — the add body is the arr's own lookup object plus the
	//    add fields, so we don't hand-build the (large, version-drifting) payload.
	payload, err := a.lookup(ctx, ep)
	if err != nil {
		return err
	}

	// 2. Resolve quality profile + root folder (override or first auto-pick).
	qp, err := a.qualityProfileID(ctx, ep)
	if err != nil {
		return err
	}
	root, err := a.rootFolderPath(ctx, ep)
	if err != nil {
		return err
	}
	payload["qualityProfileId"] = qp
	payload["rootFolderPath"] = root
	payload["monitored"] = true
	// addOptions triggers an immediate search on add — the whole point of a request.
	payload["addOptions"] = map[string]any{
		"searchForMovie":           ep.kind == "movie",
		"searchForMissingEpisodes": ep.kind == "series",
		"monitor":                  "all",
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := a.do(ctx, ep, http.MethodPost, "/api/v3/"+ep.kind, buf)
	if err != nil {
		return fmt.Errorf("arr add: %w", err) // not accepted; reconciler retries
	}
	defer func() { _ = resp.Body.Close() }()

	// 2xx accepted; 409, or the arr's already-exists validation failure, means the title is
	// already added — success per §6. Every other status, 400 included, is an error.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == http.StatusConflict ||
		(resp.StatusCode == http.StatusBadRequest && isAlreadyAdded(body)) {
		return nil
	}
	return fmt.Errorf("arr add: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// isAlreadyAdded reports whether a 400 body is the arr's duplicate-title validation failure
// (errorCode MovieExistsValidator on Radarr, SeriesExistsValidator on Sonarr).
func isAlreadyAdded(body []byte) bool {
	var failures []struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(body, &failures); err != nil {
		return false
	}
	for _, f := range failures {
		if f.ErrorCode == "MovieExistsValidator" || f.ErrorCode == "SeriesExistsValidator" {
			return true
		}
	}
	return false
}

// lookup fetches the arr's object for a title (GET /api/v3/<kind>/lookup?term=<ns>:<id>),
// returning the first match as a generic map to POST back as the add body.
func (a *Arr) lookup(ctx context.Context, ep arrEndpoint) (map[string]any, error) {
	q := url.Values{}
	q.Set("term", ep.term)
	resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/"+ep.kind+"/lookup?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return nil, fmt.Errorf("arr lookup: %w", err)
	}
	var results []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("arr lookup decode: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("arr lookup: no match for %s", ep.term)
	}
	return results[0], nil
}

// qualityProfileID returns the override (parsed as an id) or the first profile the arr reports.
func (a *Arr) qualityProfileID(ctx context.Context, ep arrEndpoint) (int, error) {
	if ep.qualityOverride != "" {
		if id, err := strconv.Atoi(ep.qualityOverride); err == nil {
			return id, nil
		}
		// A name override: match against the list below.
	}
	resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/qualityprofile", nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return 0, fmt.Errorf("arr qualityprofile: %w", err)
	}
	var profiles []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profiles); err != nil {
		return 0, fmt.Errorf("arr qualityprofile decode: %w", err)
	}
	if len(profiles) == 0 {
		return 0, fmt.Errorf("arr: no quality profiles configured")
	}
	if ep.qualityOverride != "" {
		for _, p := range profiles {
			if strings.EqualFold(p.Name, ep.qualityOverride) {
				return p.ID, nil
			}
		}
		return 0, fmt.Errorf("arr: quality profile %q not found", ep.qualityOverride)
	}
	return profiles[0].ID, nil
}

// ErrArrNotConfigured reports an arr with no address set.
var ErrArrNotConfigured = errors.New("arr: not configured")

// RootFolders lists every root folder one arr reports, as the arr sees it ("sonarr" or
// "radarr"), for discovering scan roots (#2151 decision 6). It ignores the root-folder override,
// which only picks where new titles go. ErrArrNotConfigured when that arr has no address.
func (a *Arr) RootFolders(ctx context.Context, app string) ([]string, error) {
	var conn func() (string, string)
	switch app {
	case "sonarr":
		conn = a.sonarr
	case "radarr":
		conn = a.radarr
	default:
		return nil, fmt.Errorf("arr: unknown app %q", app)
	}
	baseURL, apiKey := conn()
	if baseURL == "" {
		return nil, ErrArrNotConfigured
	}
	resp, err := a.do(ctx, arrEndpoint{baseURL: baseURL, apiKey: apiKey}, http.MethodGet, "/api/v3/rootfolder", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return nil, fmt.Errorf("arr rootfolder: %w", err)
	}
	var roots []struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&roots); err != nil {
		return nil, fmt.Errorf("arr rootfolder decode: %w", err)
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if r.Path != "" {
			out = append(out, r.Path)
		}
	}
	return out, nil
}

// rootFolderPath returns the override or the first root folder the arr reports.
func (a *Arr) rootFolderPath(ctx context.Context, ep arrEndpoint) (string, error) {
	if ep.rootOverride != "" {
		return ep.rootOverride, nil
	}
	resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/rootfolder", nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return "", fmt.Errorf("arr rootfolder: %w", err)
	}
	var roots []struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&roots); err != nil {
		return "", fmt.Errorf("arr rootfolder decode: %w", err)
	}
	if len(roots) == 0 {
		return "", fmt.Errorf("arr: no root folders configured")
	}
	return roots[0].Path, nil
}

// Cancel is a REAL withdrawal (unlike Seerr's no-op): it finds every queue record of the title
// and DELETEs each so a given-up title stops downloading. A series has one record per episode
// download, so all of them go. Every record is attempted even when one fails; the failures
// are joined. A missing queue entry is not an error.
func (a *Arr) Cancel(ctx context.Context, t provision.Title) error {
	ep, err := a.endpointFor(t)
	if err != nil {
		return err
	}
	ids, err := a.queueRecordIDs(ctx, ep)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := a.deleteQueueRecord(ctx, ep, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteQueueRecord removes one queue record; removeFromClient=true also tells the download
// client to abandon it.
func (a *Arr) deleteQueueRecord(ctx context.Context, ep arrEndpoint, id int) error {
	resp, err := a.do(ctx, ep, http.MethodDelete,
		fmt.Sprintf("/api/v3/queue/%d?removeFromClient=true&blocklist=false", id), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return fmt.Errorf("arr cancel: %w", err)
	}
	return nil
}

// queueRecordIDs lists the ids of every queue record for the title (matching the looked-up arr
// id). The queue lists in-flight downloads keyed by the arr's movieId/seriesId; a series has
// one record per episode.
func (a *Arr) queueRecordIDs(ctx context.Context, ep arrEndpoint) ([]int, error) {
	// Resolve the arr's internal id for this title via lookup, then match queue records to it.
	obj, err := a.lookup(ctx, ep)
	if err != nil {
		return nil, err
	}
	arrID, _ := toInt(obj["id"])
	if arrID == 0 {
		return nil, nil // not added yet → nothing queued
	}
	records, err := a.fetchQueue(ctx, ep)
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, rec := range records {
		if rec.arrID(ep.kind) == arrID && rec.ID != 0 {
			ids = append(ids, rec.ID)
		}
	}
	return ids, nil
}

// Reachable probes each CONFIGURED arr (GET /api/v3/system/status) for the Test button (§8): it
// validates URL + key without side effects. An unconfigured arr is skipped (a Seerr-only-then-
// switched install may have only one). At least one must be configured.
func (a *Arr) Reachable(ctx context.Context) error {
	checked := 0
	for _, app := range []struct {
		name string
		conn func() (string, string)
	}{
		{"Radarr", a.radarr},
		{"Sonarr", a.sonarr},
	} {
		base, key := app.conn()
		if strings.TrimSpace(base) == "" {
			continue
		}
		checked++
		ep := arrEndpoint{baseURL: strings.TrimRight(base, "/"), apiKey: key}
		resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/system/status", nil)
		if err != nil {
			return fmt.Errorf("%s unreachable: %w", app.name, err)
		}
		code := resp.StatusCode
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			return fmt.Errorf("%s rejected the API key (status %d)", app.name, code)
		}
		if code < 200 || code >= 300 {
			return fmt.Errorf("%s unexpected status %d", app.name, code)
		}
		// A login in front of the arr (an SSO proxy) redirects the request to its own page,
		// which answers 200 with HTML: reachable, but not the arr's API.
		if !json.Valid(body) {
			return fmt.Errorf("%s answered with a web page, not its API: is a login in front of it?", app.name)
		}
	}
	if checked == 0 {
		return fmt.Errorf("no Sonarr or Radarr URL configured")
	}
	return nil
}

// do issues an /api/v3 request with the arr's X-Api-Key header.
func (a *Arr) do(ctx context.Context, ep arrEndpoint, method, path string, body []byte) (*http.Response, error) {
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, ep.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Api-Key", ep.apiKey)
	return a.http.Do(req)
}

// resolve calls an optional resolver, returning "" when it's nil.
func resolve(f func() string) string {
	if f == nil {
		return ""
	}
	return f()
}

// toInt coerces a JSON number (float64) or string id to an int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		return 0, false
	}
}
