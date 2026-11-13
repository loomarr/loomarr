package tmdb

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library/provision"
)

// imageBase is TMDB's image CDN at the ORIGINAL size. TMDB's /configuration endpoint returns
// the canonical base, but it's stable and documented, so we hardcode it (one fewer round-trip)
// — same pragmatism as the other pinned TMDB shapes.
//
// ⚠ **`original`, not `w500`.** A URL handed straight to a browser would want a mid-size
// rendition (crisp on a guide tile without a full-res download), but no client fetches these.
// Every caller hands the URL to `images.Adopt`,
// which downloads it once server-side and generates the whole width ladder locally (§22) — so
// asking TMDB for a pre-shrunk copy would throw away the resolution the ladder's larger rungs
// need, and would make our 780px poster an upscale of their 500px one.
//
// It also means these URLs are an INTERNAL detail now: they are what we store as `source_url`,
// never what a page loads. Removing third-party origins from the operator's browser is the whole
// point of §22, so a caller that cannot adopt returns no image rather than falling back to a
// hot-link.
const imageBase = "https://image.tmdb.org/t/p/original"

// PosterURL returns a full, directly-fetchable poster image URL for a title (§icon), or ""
// (no error) when TMDB has no poster — sparse coverage is normal, so an empty answer is a
// legitimate result the caller falls back on (no icon), not a failure. Used to give a
// channel a themed icon from its primary series/movie. A poster (portrait) reads better as a
// channel tile than a backdrop.
//
// ⚠ The caller ADOPTS this URL (§22); nothing fetches it from a browser. The image service is
// what Tunarr and the operator's browser both fetch from.
func (c *Client) PosterURL(ctx context.Context, mt provision.MediaType, tmdbID int) (string, error) {
	ctx, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if tmdbID <= 0 {
		return "", nil
	}
	path := "/movie/" + strconv.Itoa(tmdbID)
	if mt == provision.Series {
		path = "/tv/" + strconv.Itoa(tmdbID)
	}
	var body struct {
		PosterPath string `json:"poster_path"`
	}
	if err := c.get(ctx, path, &body); err != nil {
		return "", err
	}
	if p := strings.TrimSpace(body.PosterPath); p != "" {
		return imageBase + p, nil // poster_path already has a leading "/"
	}
	return "", nil
}

// BackdropURL returns a title's landscape artwork, or "" when TMDB has none. Backdrops are the
// movie-level counterpart to episode stills: both are 16:9 and therefore share one preview shape
// in the Guide and Watch timeline. PosterURL remains separate because portrait artwork is still
// the right source for channel icons and title tiles.
//
// As with PosterURL, the caller adopts this original-size URL into Loomarr's image service; it is
// never handed to an operator's browser.
func (c *Client) BackdropURL(ctx context.Context, mt provision.MediaType, tmdbID int) (string, error) {
	ctx, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if tmdbID <= 0 {
		return "", nil
	}
	path := "/movie/" + strconv.Itoa(tmdbID)
	if mt == provision.Series {
		path = "/tv/" + strconv.Itoa(tmdbID)
	}
	var body struct {
		BackdropPath string `json:"backdrop_path"`
	}
	if err := c.get(ctx, path, &body); err != nil {
		return "", err
	}
	if p := strings.TrimSpace(body.BackdropPath); p != "" {
		return imageBase + p, nil // backdrop_path already has a leading "/"
	}
	return "", nil
}

// PosterURLByTVDB resolves a TVDB series id to a TMDB poster via the /find bridge
// (/find/{tvdb_id}?external_source=tvdb_id → the matching tv result's poster_path). Our
// series are often TVDB-keyed (Seerr's canonical id for series), but posters live on TMDB;
// this is the one-hop bridge. Returns "" (no error) when no match / no poster.
func (c *Client) PosterURLByTVDB(ctx context.Context, tvdbID int) (string, error) {
	ctx, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if tvdbID <= 0 {
		return "", nil
	}
	var body struct {
		TVResults []struct {
			PosterPath string `json:"poster_path"`
		} `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+strconv.Itoa(tvdbID)+"?external_source=tvdb_id", &body); err != nil {
		return "", err
	}
	for _, r := range body.TVResults {
		if p := strings.TrimSpace(r.PosterPath); p != "" {
			return imageBase + p, nil
		}
	}
	return "", nil
}

// EpisodeStillURLByTVDB resolves a TVDB series id to a per-episode still, via the same /find bridge
// PosterURLByTVDB uses (/find/{tvdb_id}?external_source=tvdb_id → the tv result's TMDB id) and then
// EpisodeStillURL for that id + season/episode. Series are usually TVDB-keyed (§3 series key), so the
// live-TV timeline needs this to show an episode thumbnail for them. Best-effort throughout: no TVDB
// match, no TMDB id, or no still all return "" + nil, never a hard failure.
func (c *Client) EpisodeStillURLByTVDB(ctx context.Context, tvdbID, season, episode int) (string, error) {
	ctx, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if tvdbID <= 0 {
		return "", nil
	}
	var body struct {
		TVResults []struct {
			ID int `json:"id"` // the TMDB series id — the bridge from tvdb to tmdb
		} `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+strconv.Itoa(tvdbID)+"?external_source=tvdb_id", &body); err != nil {
		return "", err
	}
	for _, r := range body.TVResults {
		if r.ID > 0 {
			return c.EpisodeStillURL(ctx, r.ID, season, episode)
		}
	}
	return "", nil
}

// EpisodeStillURL returns the absolute image URL of a series episode's still frame (the little
// preview image TMDB has per episode), or "" when TMDB has none for that episode. Best-effort like
// PosterURL: a not-found episode or an image-less one is "" + nil error, never a hard failure — the
// caller renders a fallback. Used by the live-TV timeline to show a per-episode thumbnail on hover.
func (c *Client) EpisodeStillURL(ctx context.Context, tmdbID, season, episode int) (string, error) {
	ctx, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if tmdbID <= 0 {
		return "", nil
	}
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", tmdbID, season, episode)
	var body struct {
		StillPath string `json:"still_path"`
	}
	if _, err := c.getOptional(ctx, path, &body); err != nil {
		return "", err
	}
	if p := strings.TrimSpace(body.StillPath); p != "" {
		return imageBase + p, nil // still_path already has a leading "/"
	}
	return "", nil
}
