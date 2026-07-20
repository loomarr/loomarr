package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Client is the shared Emby/Jellyfin adapter. One code path; the flavor only
// changes header construction (auth.go).
//
// Connection (base URL + token) is resolved through a provider at CALL time, not
// captured at construction (config-design §3 hot-apply): saving a new Emby token
// in Settings means the next Lookup uses it, with no restart. New() supplies a
// static provider (fixed config / tests); NewDynamic() reads the live settings
// snapshot per call.
type Client struct {
	flavor   Flavor
	conn     func() (baseURL, token string) // resolved per request (hot-apply)
	deviceID string                         // stable per install (§11)
	http     *http.Client
}

// New builds a Library client with a FIXED connection (tests + static config).
// deviceID should be the stable per-install id (§11, derived from the instance
// id at first migration); tests may pass a fixed value.
func New(flavor Flavor, baseURL, token, deviceID string) *Client {
	base := strings.TrimRight(baseURL, "/")
	return NewDynamic(flavor, func() (string, string) { return base, token }, deviceID)
}

// NewDynamic builds a Library client whose connection is resolved per call via
// conn (config-design §3 hot-apply). The composition root passes a closure over
// the settings snapshot; the base URL is normalized (trailing slash stripped) on
// each read so a saved "http://emby:8096/" and "http://emby:8096" are one value.
func NewDynamic(flavor Flavor, conn func() (baseURL, token string), deviceID string) *Client {
	return &Client{
		flavor:   flavor,
		conn:     conn,
		deviceID: deviceID,
		http:     httpx.NewNamed("library", httpx.TimeoutLibrary),
	}
}

// baseURL resolves the current media-server base URL (trailing slash stripped).
func (c *Client) baseURL() string {
	u, _ := c.conn()
	return strings.TrimRight(u, "/")
}

// token resolves the current media-server token.
func (c *Client) token() string {
	_, t := c.conn()
	return t
}

// item is the slice of the media server's /Items response we need (§6). The
// enforcement metadata (OfficialRating/Genres) is populated only when the request
// asks for it via Fields — LookupDetail does, Lookup's other callers don't need it.
type item struct {
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	OfficialRating string   `json:"OfficialRating"`
	Genres         []string `json:"Genres"`
}

type itemsResponse struct {
	Items []item `json:"Items"`
}

// LibraryItem is a presence-check hit enriched with the audience-enforcement
// metadata (§4). It exists because the *discovery* path finds titles via TMDB and
// only confirms library ownership afterwards, so — unlike keyword search — it must
// fetch the rating explicitly. Without it a discovered-then-owned title reaches the
// scheduler unrated, and a channel with an audience ceiling drops it, going live
// with no programs (dead air, §9).
type LibraryItem struct {
	ID             string
	OfficialRating string
	Genres         []string
}

// LookupDetail is the §6 presence check that also returns the enforcement metadata:
//
//	GET /Items?Recursive=true&IncludeItemTypes=<type>&Limit=1
//	    &Fields=Genres,OfficialRating&AnyProviderIdEquals=<kind>.<id>
//
// Present iff Items is non-empty. Provider name is lowercase (tmdb./tvdb.) per §6.
func (c *Client) LookupDetail(ctx context.Context, kind ProviderKind, providerID string, mt MediaType) (LibraryItem, bool, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", string(mt))
	q.Set("Limit", "1")
	q.Set("Fields", "Genres,OfficialRating")
	q.Set("AnyProviderIdEquals", fmt.Sprintf("%s.%s", kind, providerID))

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return LibraryItem{}, false, err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)

	var out itemsResponse
	if err := c.do(req, &out); err != nil {
		return LibraryItem{}, false, err
	}
	if len(out.Items) == 0 {
		return LibraryItem{}, false, nil
	}
	it := out.Items[0]
	return LibraryItem{ID: it.ID, OfficialRating: it.OfficialRating, Genres: it.Genres}, true, nil
}

// Lookup is the id-only presence check (reconcile, ingest, eval): whether the title
// is present and its library item id. It delegates to LookupDetail and discards the
// metadata, so there is ONE presence query, not two shapes that could drift.
func (c *Client) Lookup(ctx context.Context, kind ProviderKind, providerID string, mt MediaType) (string, bool, error) {
	d, present, err := c.LookupDetail(ctx, kind, providerID, mt)
	return d.ID, present, err
}

// authResponse is AuthenticateByName's success body (§11).
type authResponse struct {
	AccessToken string `json:"AccessToken"`
	User        struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
			IsDisabled      bool `json:"IsDisabled"`
		} `json:"Policy"`
	} `json:"User"`
}

// AuthenticateByName verifies credentials (§11). On success it best-effort logs
// the media-server session out (we only needed to confirm the password) and
// returns the User; callers issue their own session (Phase 9).
func (c *Client) AuthenticateByName(ctx context.Context, username, password string) (User, error) {
	body, _ := json.Marshal(map[string]string{"Username": username, "Pw": password})
	req, err := c.newRequest(ctx, http.MethodPost, "/Users/AuthenticateByName", body)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.flavor.applyLoginAuth(req, c.deviceID) // client-id header, no token yet (§11)

	var out authResponse
	if err := c.do(req, &out); err != nil {
		return User{}, err // 401 (bad creds) surfaces here as an error (§11 negative path)
	}
	c.bestEffortLogout(ctx, out.AccessToken)

	return User{
		ID:       out.User.ID,
		Name:     out.User.Name,
		IsAdmin:  out.User.Policy.IsAdministrator,
		Disabled: out.User.Policy.IsDisabled,
	}, nil
}

// userDTO mirrors a /Users list entry (§11).
type userDTO struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Policy struct {
		IsAdministrator bool `json:"IsAdministrator"`
		IsDisabled      bool `json:"IsDisabled"`
	} `json:"Policy"`
}

// ListUsers returns all server users for import/sync (§11), via the admin token.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/Users", nil)
	if err != nil {
		return nil, err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)

	var dtos []userDTO
	if err := c.do(req, &dtos); err != nil {
		return nil, err
	}
	users := make([]User, len(dtos))
	for i, d := range dtos {
		users[i] = User{ID: d.ID, Name: d.Name, IsAdmin: d.Policy.IsAdministrator, Disabled: d.Policy.IsDisabled}
	}
	return users, nil
}

// bestEffortLogout discards the media-server token (§11). Failures are ignored.
func (c *Client) bestEffortLogout(ctx context.Context, token string) {
	if token == "" {
		return
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/Sessions/Logout", nil)
	if err != nil {
		return
	}
	c.flavor.applyTokenAuth(req, token, c.deviceID)
	resp, err := c.http.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// newJSONRequest builds a request with a JSON-marshaled body and the
// Content-Type header set. Used by the Live TV wiring POSTs (§6).
func (c *Client) newJSONRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	blob, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s body: %w", path, err)
	}
	req, err := c.newRequest(ctx, method, path, blob)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rdr *strings.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
		return http.NewRequestWithContext(ctx, method, c.baseURL()+path, rdr)
	}
	return http.NewRequestWithContext(ctx, method, c.baseURL()+path, nil)
}

// do executes a request and decodes a JSON body into out. Non-2xx is an error
// (auth failures surface as errors — §11 negative path).
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d", req.Method, req.URL.Path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}
	return nil
}
