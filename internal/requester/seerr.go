package requester

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
	"github.com/mantonx/loomarr/internal/provision"
)

// Seerr is the default Requester (§6). POST {url}/api/v1/request with the
// X-Api-Key header (never a query param — §6). Phase-0 finding: a request for an
// already-available/duplicate title returns 201 with the existing media (not
// 409). We treat any 2xx AND 409 as success (§6 idempotency).
type Seerr struct {
	conn func() (baseURL, apiKey string) // resolved per request (config-design §3 hot-apply)
	http *http.Client
}

// NewSeerr builds the Seerr requester with a FIXED connection (tests / static
// config). The base URL is normalized (trailing slash stripped).
func NewSeerr(baseURL, apiKey string) *Seerr {
	base := strings.TrimRight(baseURL, "/")
	return NewSeerrDynamic(func() (string, string) { return base, apiKey })
}

// NewSeerrDynamic builds a Seerr requester whose connection is resolved per call
// via conn (config-design §3 hot-apply). The composition root passes a closure
// over the settings snapshot.
func NewSeerrDynamic(conn func() (baseURL, apiKey string)) *Seerr {
	return &Seerr{conn: conn, http: httpx.New(httpx.TimeoutSeerr)}
}

func (s *Seerr) baseURL() string { u, _ := s.conn(); return strings.TrimRight(u, "/") }
func (s *Seerr) apiKey() string  { _, k := s.conn(); return k }

// seerrRequest is the POST body (§6): {mediaType, mediaId=TMDBID, seasons?}.
type seerrRequest struct {
	MediaType string `json:"mediaType"`         // "movie" | "tv"
	MediaID   int    `json:"mediaId"`           // TMDB id (Seerr uses TMDB even for series)
	Seasons   []int  `json:"seasons,omitempty"` // series only; omit = all
}

// Request implements Requester against Seerr. Idempotent per §6.
func (s *Seerr) Request(ctx context.Context, t provision.Title) error {
	body := seerrRequest{MediaID: t.TMDBID}
	switch t.MediaType {
	case provision.Movie:
		body.MediaType = "movie"
	case provision.Series:
		body.MediaType = "tv"
		body.Seasons = t.Seasons
	default:
		return fmt.Errorf("seerr: unsupported media type %q", t.MediaType)
	}
	if t.TMDBID == 0 {
		// Seerr keys on TMDB even for series (§6); without it we cannot request.
		return fmt.Errorf("seerr: title %q has no TMDB id", t.Name)
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+"/api/v1/request", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.apiKey())

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("seerr request: %w", err) // not accepted; reconciler retries
	}
	defer func() { _ = resp.Body.Close() }()

	// 2xx (incl. the observed 201-with-existing-media) and 409 are success (§6).
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("seerr request: unexpected status %d", resp.StatusCode)
}

// Cancel is a best-effort no-op for Seerr (§4/§6): Loomarr doesn't persist the
// Seerr request id, and Seerr owns its own request queue. Giving up on a title
// simply stops Loomarr tracking it; any stale Seerr request ages out under
// Seerr's own policy. A direct-arr requester (§15) can implement a real cancel.
func (s *Seerr) Cancel(ctx context.Context, t provision.Title) error {
	return nil
}
