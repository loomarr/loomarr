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
type Client struct {
	flavor   Flavor
	baseURL  string
	token    string // admin LIBRARY_TOKEN
	deviceID string // stable per install (§11)
	http     *http.Client
}

// New builds a Library client. deviceID should be the stable per-install id
// (§11, derived from the instance id at first migration); tests may pass a fixed
// value.
func New(flavor Flavor, baseURL, token, deviceID string) *Client {
	return &Client{
		flavor:   flavor,
		baseURL:  strings.TrimRight(baseURL, "/"),
		token:    token,
		deviceID: deviceID,
		http:     httpx.New(httpx.TimeoutLibrary),
	}
}

// item is the slice of the media server's /Items response we need (§6).
type item struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

type itemsResponse struct {
	Items []item `json:"Items"`
}

// Lookup implements the §6 presence check:
//
//	GET /Items?Recursive=true&IncludeItemTypes=<type>&Limit=1&AnyProviderIdEquals=<kind>.<id>
//
// Present iff Items is non-empty → Items[0].Id. Provider name is lowercase
// (tmdb./tvdb.) per §6; Phase 0 confirmed Emby matches case-insensitively but
// lowercase is the spec.
func (c *Client) Lookup(ctx context.Context, kind ProviderKind, providerID string, mt MediaType) (string, bool, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", string(mt))
	q.Set("Limit", "1")
	q.Set("AnyProviderIdEquals", fmt.Sprintf("%s.%s", kind, providerID))

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return "", false, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)

	var out itemsResponse
	if err := c.do(req, &out); err != nil {
		return "", false, err
	}
	if len(out.Items) == 0 {
		return "", false, nil
	}
	return out.Items[0].ID, true, nil
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
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)

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

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rdr *strings.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
		return http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	}
	return http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
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
