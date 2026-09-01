package requester

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/provision"
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
	return NewSeerrDynamicObserved(conn, nil)
}

// NewSeerrDynamicObserved binds operational requests to one application generation.
func NewSeerrDynamicObserved(conn func() (baseURL, apiKey string), recorder *metrics.Recorder) *Seerr {
	return &Seerr{conn: conn, http: httpx.NewNamedObserved("seerr", httpx.TimeoutSeerr, recorder)}
}

func (s *Seerr) baseURL() string { u, _ := s.conn(); return strings.TrimRight(u, "/") }
func (s *Seerr) apiKey() string  { _, k := s.conn(); return k }

// seerrSeasons serializes a series' requested seasons the way Jellyseerr's /request
// endpoint demands: the literal string "all" when no specific seasons are asked for,
// otherwise the [int] array. Omitting the field entirely (the old `omitempty` []int)
// makes Jellyseerr 500 ("Cannot read properties of undefined (reading 'filter')") — its
// TV handler assumes `seasons` is always present and filters over it. Verified live: an
// all-seasons TV request with `seasons:"all"` returns 201; the same request with the field
// omitted returns 500. Movies never send this field (Request() leaves it nil-and-omitted).
type seerrSeasons []int

func (s seerrSeasons) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte(`"all"`), nil
	}
	return json.Marshal([]int(s))
}

// seerrRequest is the POST body (§6): {mediaType, mediaId=TMDBID, seasons}. `Seasons` is a
// POINTER so presence tracks media type, not emptiness: a series ALWAYS sets it (non-nil,
// even when empty → marshals "all"); a movie leaves it nil → omitted. `omitempty` alone
// couldn't do this — it would drop the empty-but-series case that must send "all".
type seerrRequest struct {
	MediaType string        `json:"mediaType"`         // "movie" | "tv"
	MediaID   int           `json:"mediaId"`           // TMDB id (Seerr uses TMDB even for series)
	Seasons   *seerrSeasons `json:"seasons,omitempty"` // series: "all"|[ints]; movies: nil→omitted
}

// Request implements Requester against Seerr. Idempotent per §6.
func (s *Seerr) Request(ctx context.Context, t provision.Title) error {
	body := seerrRequest{MediaID: t.TMDBID}
	switch t.MediaType {
	case provision.Movie:
		body.MediaType = "movie"
	case provision.Series:
		body.MediaType = "tv"
		// Always non-nil for a series so `seasons` is present in the body (empty → "all");
		// nil would omit it and Jellyseerr 500s. Distinct from any airing-season window —
		// this is what to ACQUIRE (all seasons unless the pick named specific ones).
		seasons := seerrSeasons(t.Seasons)
		body.Seasons = &seasons
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

// Jellyseerr/Overseerr MediaStatus enum (their MediaStatus TS enum). Loomarr only cares about
// the in-flight middle: PROCESSING (a release is being fetched) and PARTIALLY_AVAILABLE (some
// but not all episodes have landed). PENDING is "requested, nothing grabbed yet"; AVAILABLE is
// the library scan's job, not ours.
const (
	seerrMediaPending    = 2
	seerrMediaProcessing = 3
	seerrMediaPartial    = 4
	seerrMediaAvailable  = 5
)

// seerrMediaItem is the slice of a /api/v1/media record we read: the TMDB id correlates back to
// a Loomarr title (Seerr keys on TMDB, §6), and status is the coarse lifecycle enum. Seerr does
// expose a `downloadStatus` array that *can* carry byte-level size/sizeleft, but it is empty on
// the deployments observed (Jellyseerr doesn't reliably proxy the arr queue back), so this path
// deliberately reads only the enum — a coarse, always-present signal — never a fake percentage.
type seerrMediaItem struct {
	TMDBID int `json:"tmdbId"`
	Status int `json:"status"`
}

type seerrMediaPage struct {
	Results []seerrMediaItem `json:"results"`
}

// QueueStatus implements reconcile.QueueStatuser for the Seerr provider (§18.1). Unlike the arr
// (one /queue fetch + one lookup per title), Seerr answers the whole in-flight set in ONE call:
// GET /api/v1/media?filter=processing returns every media item Seerr considers in progress. We
// index those by TMDB id and, for each requested title, map the enum to a coarse QueueItem.
//
// Because Seerr surfaces no byte percentage (see seerrMediaItem), Progress stays 0 for a
// downloading title and the meaning rides the Status LABEL ("Downloading" / "Partly available") —
// the FE renders an indeterminate bar for a grabbed title whose Progress is 0, rather than a
// misleading "0% of a known download". A title not in the processing set is returned Grabbed=false
// (still merely `requested`, or already available — the library scan owns that transition).
func (s *Seerr) QueueStatus(ctx context.Context, titles []provision.Title) ([]QueueItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+"/api/v1/media?filter=processing&take=200", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", s.apiKey())
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("seerr media: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("seerr media: unexpected status %d", resp.StatusCode)
	}
	var page seerrMediaPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("seerr media: decode: %w", err)
	}

	// Index the processing set by TMDB id → status.
	byTMDB := make(map[int]int, len(page.Results))
	for _, m := range page.Results {
		if m.TMDBID != 0 {
			byTMDB[m.TMDBID] = m.Status
		}
	}

	out := make([]QueueItem, 0, len(titles))
	for _, t := range titles {
		key, err := t.Key()
		if err != nil {
			continue // no usable key → can't correlate
		}
		status, present := byTMDB[t.TMDBID]
		grabbed, label := seerrCoarse(status, present)
		out = append(out, QueueItem{Key: key, Grabbed: grabbed, Status: label})
	}
	return out, nil
}

// seerrCoarse maps a media status (and whether the title was in the processing set at all) to the
// two fields Seerr can honestly fill: whether the title is in flight (Grabbed → promotes
// requested→downloading), and a human label. Progress is intentionally left 0 (indeterminate) —
// Seerr has no percentage. A title absent from the processing set is not grabbed.
func seerrCoarse(status int, present bool) (grabbed bool, label string) {
	if !present {
		return false, ""
	}
	switch status {
	case seerrMediaProcessing:
		return true, "Downloading"
	case seerrMediaPartial:
		return true, "Partly available"
	case seerrMediaPending:
		// Queued inside Seerr but nothing grabbed yet — still merely `requested`.
		return false, ""
	case seerrMediaAvailable:
		// Fully available is the library scan's transition to make, not the poller's.
		return false, ""
	default:
		return false, ""
	}
}

// Reachable is a cheap, side-effect-free connection probe for the "test my Seerr"
// button (§8 setup): a GET on an admin-authed endpoint validates BOTH reachability
// and the API key — a dead host is a transport error, a bad key is 401/403. Unlike
// Request it adds/changes nothing.
func (s *Seerr) Reachable(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+"/api/v1/settings/main", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", s.apiKey())
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("seerr rejected the API key (status %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("seerr unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Cancel is a best-effort no-op for Seerr (§4/§6): Loomarr doesn't persist the
// Seerr request id, and Seerr owns its own request queue. Giving up on a title
// simply stops Loomarr tracking it; any stale Seerr request ages out under
// Seerr's own policy. A direct-arr requester (§15) can implement a real cancel.
func (s *Seerr) Cancel(ctx context.Context, t provision.Title) error {
	return nil
}
