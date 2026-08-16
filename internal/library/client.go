package library

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Connection is one immutable media-server connection snapshot. Flavor, URL,
// and token belong together: mixing any member from a later settings value can
// send a credential to the wrong server or select the wrong auth header.
type Connection struct {
	Flavor  Flavor
	BaseURL string
	Token   string
}

// ConnectionGeneration is a safe, comparable identity for one validated
// connection generation. It deliberately keeps the credential digest opaque:
// callers can key caches without retaining or exposing the raw token.
type ConnectionGeneration struct {
	flavor    Flavor
	baseURL   string
	tokenHash [sha256.Size]byte
}

// ConnectionSource resolves the current media-server connection. Dynamic
// clients call it exactly once at the start of each public operation.
type ConnectionSource func() Connection

// ErrConnectionRequired reports an incomplete or invalid media-server
// connection. No HTTP request is attempted when this error is returned. The
// field-specific errors let presentation adapters explain the repair without
// reimplementing this validation judgment.
var (
	ErrConnectionRequired       = errors.New("library: complete media-server flavor, URL, and token are required")
	ErrConnectionFlavorRequired = fmt.Errorf("%w: flavor must be emby or jellyfin", ErrConnectionRequired)
	ErrConnectionURLRequired    = fmt.Errorf("%w: URL must be absolute HTTP or HTTPS", ErrConnectionRequired)
	ErrConnectionTokenRequired  = fmt.Errorf("%w: API token is required", ErrConnectionRequired)
)

// Client is the shared Emby/Jellyfin adapter. One code path; the flavor only
// changes header construction (auth.go).
//
// Connection is resolved through a provider at OPERATION time, not captured at
// construction (config-design §3 hot-apply): saving a new token or changing
// flavor means the next Lookup uses it, with no restart. One operation keeps one
// immutable flavor + URL + token snapshot through every nested/batched request.
type Client struct {
	current  ConnectionSource
	fixed    *Connection
	deviceID string // stable per install (§11)
	http     *http.Client
}

// New builds a Library client with a FIXED connection (tests + static config).
// deviceID should be the stable per-install id (§11, derived from the instance
// id at first migration); tests may pass a fixed value.
func New(flavor Flavor, baseURL, token, deviceID string) *Client {
	connection := Connection{Flavor: flavor, BaseURL: baseURL, Token: token}
	return NewDynamic(func() Connection { return connection }, deviceID)
}

// NewDynamic builds a Library client whose complete connection is resolved once
// per public operation (config-design §3 hot-apply).
func NewDynamic(current ConnectionSource, deviceID string) *Client {
	return newDynamicWithHTTP(current, deviceID, httpx.NewNamed("library", httpx.TimeoutLibrary))
}

func newDynamicWithHTTP(current ConnectionSource, deviceID string, httpClient *http.Client) *Client {
	if current == nil {
		current = func() Connection { return Connection{} }
	}
	if httpClient == nil {
		httpClient = httpx.NewNamed("library", httpx.TimeoutLibrary)
	}
	return &Client{
		current:  current,
		deviceID: deviceID,
		http:     httpClient,
	}
}

// Snapshot binds the current complete connection without validating or making
// an HTTP request. Compound adapters can use the returned client for several
// calls that form one domain operation, ensuring every request addresses the
// same server with the same credential.
func (c *Client) Snapshot() *Client {
	if c == nil {
		connection := Connection{}
		return &Client{fixed: &connection, http: &http.Client{}}
	}
	if c.fixed != nil {
		return c
	}
	connection := normalizeConnection(c.current())
	return &Client{fixed: &connection, deviceID: c.deviceID, http: c.http}
}

// Connection returns a copy of the immutable connection carried by this
// snapshot. Call Snapshot first when this value and subsequent client calls
// must belong to the same operation.
func (c *Client) Connection() Connection { return *c.Snapshot().fixed }

// Validate returns the normalized complete connection or
// ErrConnectionRequired. It is the shared fail-closed seam for compound
// adapters that need to validate before consulting cached state or performing
// any I/O.
func (c Connection) Validate() (Connection, error) {
	c = normalizeConnection(c)
	if c.Flavor != Emby && c.Flavor != Jellyfin {
		return Connection{}, ErrConnectionFlavorRequired
	}
	if c.BaseURL == "" {
		return Connection{}, ErrConnectionURLRequired
	}
	if strings.TrimSpace(c.Token) == "" {
		return Connection{}, ErrConnectionTokenRequired
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Connection{}, ErrConnectionURLRequired
	}
	return c, nil
}

// Generation returns a safe cache key for the full validated connection. URL
// spelling differences that normalize to the same endpoint share a generation;
// flavor, endpoint, and every token rotation remain distinct.
func (c Connection) Generation() (ConnectionGeneration, error) {
	c, err := c.Validate()
	if err != nil {
		return ConnectionGeneration{}, err
	}
	return ConnectionGeneration{
		flavor: c.Flavor, baseURL: c.BaseURL, tokenHash: sha256.Sum256([]byte(c.Token)),
	}, nil
}

func normalizeConnection(connection Connection) Connection {
	connection.BaseURL = strings.TrimRight(strings.TrimSpace(connection.BaseURL), "/")
	return connection
}

func (c *Client) operation() (*Client, error) {
	op := c.Snapshot()
	if _, err := op.Connection().Validate(); err != nil {
		return nil, err
	}
	return op, nil
}

func (c *Client) baseURL() string { return c.fixed.BaseURL }
func (c *Client) token() string   { return c.fixed.Token }
func (c *Client) flavor() Flavor  { return c.fixed.Flavor }

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
	c, err := c.operation()
	if err != nil {
		return LibraryItem{}, false, err
	}
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
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

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
	c = c.Snapshot()
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
	c, err := c.operation()
	if err != nil {
		return User{}, err
	}
	body, _ := json.Marshal(map[string]string{"Username": username, "Pw": password})
	req, err := c.newRequest(ctx, http.MethodPost, "/Users/AuthenticateByName", body)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.flavor().applyLoginAuth(req, c.deviceID) // client-id header, no token yet (§11)

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
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/Users", nil)
	if err != nil {
		return nil, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

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
	c.flavor().applyTokenAuth(req, token, c.deviceID)
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
		// Include a snippet of the response body — a bare "status 500" hides the
		// media server's own explanation (e.g. Emby's reason for rejecting a tuner
		// add), which is exactly what an operator needs to fix a wiring failure.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(snippet))
		if msg == "" {
			return fmt.Errorf("%s %s: status %d", req.Method, req.URL.Path, resp.StatusCode)
		}
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}
	return nil
}

// doTolerate404 runs a request whose target may already be gone, treating a 404 as
// success so a delete stays idempotent (re-running the URL-change reconcile, or a
// race, must not error on an already-removed tuner). All other non-2xx surface as
// with do.
func (c *Client) doTolerate404(req *http.Request) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(snippet))
		if msg == "" {
			return fmt.Errorf("%s %s: status %d", req.Method, req.URL.Path, resp.StatusCode)
		}
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, msg)
	}
	return nil
}
