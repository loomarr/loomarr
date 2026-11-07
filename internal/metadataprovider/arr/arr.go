// Package arr provides Sonarr and Radarr as metadata providers (#2151 decisions 7, 15, 38). They
// are the download managers that placed the household's files, so they know which title each file
// is and carry the guide text, numbering, certification and a media summary for it. Sonarr's text
// comes from TVDB and Radarr's from TMDB.
//
// Each provider reads its connection once per operation and carries it through every request that
// operation makes, so a saved URL or key takes effect on the next operation and one app's key is
// never sent to another.
package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// Conn returns an app's base URL and API key. An empty URL means the app is not configured.
type Conn func() (baseURL, apiKey string)

// maxBody bounds one response. A full Radarr library is tens of megabytes; this is far above it.
const maxBody = 256 << 20

// defaultTimeout bounds one request when the caller supplies no client.
const defaultTimeout = 30 * time.Second

// endpoint is one operation's resolved connection.
type endpoint struct {
	base, key string
}

type client struct {
	app  string
	conn Conn
	http *http.Client
}

// newClient copies httpClient (or a plain one with a timeout) and refuses every redirect: Go
// forwards X-Api-Key across hosts, and an arr's read API never redirects.
func newClient(app string, conn Conn, httpClient *http.Client) client {
	c := http.Client{Timeout: defaultTimeout}
	if httpClient != nil {
		c = *httpClient
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client{app: app, conn: conn, http: &c}
}

// endpoint resolves the connection for one operation.
func (c client) endpoint() (endpoint, error) {
	if c.conn == nil {
		return endpoint{}, fmt.Errorf("%s: %w", c.app, mp.ErrNotConfigured)
	}
	base, key := c.conn()
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return endpoint{}, fmt.Errorf("%s: %w", c.app, mp.ErrNotConfigured)
	}
	return endpoint{base: base, key: key}, nil
}

// errStatus is a non-2xx answer.
type errStatus struct {
	app  string
	path string
	code int
}

func (e errStatus) Error() string {
	if e.code == http.StatusUnauthorized || e.code == http.StatusForbidden {
		return fmt.Sprintf("%s rejected the API key on %s (status %d)", e.app, e.path, e.code)
	}
	return fmt.Sprintf("%s %s: status %d", e.app, e.path, e.code)
}

// get decodes GET path?query into v.
func (c client) get(ctx context.Context, ep endpoint, path string, query url.Values, v any) error {
	target := ep.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("%s %s: %w", c.app, path, err)
	}
	req.Header.Set("X-Api-Key", ep.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", c.app, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return errStatus{app: c.app, path: path, code: resp.StatusCode}
	}
	body := &io.LimitedReader{R: resp.Body, N: maxBody + 1}
	if err := json.NewDecoder(body).Decode(v); err != nil {
		if body.N <= 0 {
			return fmt.Errorf("%s %s: response exceeds %d bytes", c.app, path, maxBody)
		}
		return fmt.Errorf("%s %s: decode: %w", c.app, path, err)
	}
	return nil
}

// isStatus reports whether err is a non-2xx answer with code.
func isStatus(err error, code int) bool {
	var s errStatus
	return errors.As(err, &s) && s.code == code
}

// numericRef returns ref's value as a positive integer id, or ErrUnsupported.
func numericRef(app string, caps mp.Capabilities, ref mp.Ref) (int, error) {
	if !caps.CanResolve(ref) {
		return 0, fmt.Errorf("%s: %s by %s: %w", app, ref.Kind, ref.ID.Namespace, mp.ErrUnsupported)
	}
	n, err := strconv.Atoi(strings.TrimSpace(ref.ID.Value))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: %s id %q is not a positive integer: %w", app, ref.ID.Namespace, ref.ID.Value, mp.ErrUnsupported)
	}
	return n, nil
}

// image is an arr image. Only the remote URL is kept: the local one needs the API key.
type image struct {
	CoverType string `json:"coverType"`
	RemoteURL string `json:"remoteUrl"`
}

func artwork(images []image) []mp.ArtworkRef {
	var out []mp.ArtworkRef
	for _, img := range images {
		ref := strings.TrimSpace(img.RemoteURL)
		if img.CoverType == "" || !strings.HasPrefix(ref, "https://") {
			continue
		}
		out = append(out, mp.ArtworkRef{Kind: img.CoverType, Ref: ref})
	}
	return out
}

// mediaInfo is the arr's summary of a file, read from the file by the arr.
type mediaInfo struct {
	AudioChannels     float64 `json:"audioChannels"`
	AudioCodec        string  `json:"audioCodec"`
	AudioLanguages    string  `json:"audioLanguages"`
	VideoCodec        string  `json:"videoCodec"`
	VideoDynamicRange string  `json:"videoDynamicRange"`
	Resolution        string  `json:"resolution"`
	RunTime           string  `json:"runTime"`
	ScanType          string  `json:"scanType"`
	Subtitles         string  `json:"subtitles"`
}

// technical maps a file's summary. It returns nil when the arr has no summary for the file.
func technical(info *mediaInfo, size int64) *mp.Technical {
	if info == nil {
		return nil
	}
	t := &mp.Technical{
		Duration:          parseRunTime(info.RunTime),
		SizeBytes:         size,
		VideoCodec:        strings.TrimSpace(info.VideoCodec),
		HDR:               isHDR(info.VideoDynamicRange),
		Interlaced:        strings.EqualFold(strings.TrimSpace(info.ScanType), "interlaced"),
		AudioCodec:        strings.TrimSpace(info.AudioCodec),
		AudioChannels:     info.AudioChannels,
		AudioLanguages:    splitLanguages(info.AudioLanguages),
		SubtitleLanguages: splitLanguages(info.Subtitles),
	}
	t.Width, t.Height = parseResolution(info.Resolution)
	return t
}

// isHDR reads the arr's dynamic range, which is empty or "SDR" for standard range.
func isHDR(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.EqualFold(s, "SDR")
}

// parseRunTime reads "44:12", "1:02:03" or "01:02:03.5". An unreadable value is zero, a gap.
func parseRunTime(s string) time.Duration {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	var total time.Duration
	units := []time.Duration{time.Second, time.Minute, time.Hour}
	for i := range parts {
		field := parts[len(parts)-1-i]
		if i == 0 {
			secs, err := strconv.ParseFloat(field, 64)
			if err != nil || secs < 0 || secs >= 60 {
				return 0
			}
			total += time.Duration(secs * float64(time.Second))
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 || (i == 1 && len(parts) == 3 && n >= 60) {
			return 0
		}
		total += time.Duration(n) * units[i]
	}
	return total
}

// parseResolution reads "1920x1080".
func parseResolution(s string) (int, int) {
	w, h, ok := strings.Cut(strings.ToLower(strings.TrimSpace(s)), "x")
	if !ok {
		return 0, 0
	}
	width, errW := strconv.Atoi(w)
	height, errH := strconv.Atoi(h)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

// splitLanguages reads the arr's "eng/jpn" list.
func splitLanguages(s string) []string {
	var out []string
	for _, lang := range strings.Split(s, "/") {
		if lang = strings.TrimSpace(lang); lang != "" {
			out = append(out, lang)
		}
	}
	return out
}

// date returns the YYYY-MM-DD part of an arr date or timestamp, or empty.
func date(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return ""
	}
	if _, err := time.Parse(time.DateOnly, s[:10]); err != nil {
		return ""
	}
	return s[:10]
}

// page is a title's page in the app's own web UI: /series/<slug> in Sonarr, /movie/<slug> in
// Radarr. None without a slug.
func page(base, kind, slug string) string {
	slug = strings.TrimSpace(slug)
	if base == "" || slug == "" {
		return ""
	}
	return base + "/" + kind + "/" + url.PathEscape(slug)
}

func ids(pairs ...mp.ExternalID) []mp.ExternalID {
	var out []mp.ExternalID
	for _, id := range pairs {
		if id.Value != "" && id.Value != "0" {
			out = append(out, id)
		}
	}
	return out
}

func intID(ns mp.Namespace, n int) mp.ExternalID {
	if n <= 0 {
		return mp.ExternalID{Namespace: ns}
	}
	return mp.ExternalID{Namespace: ns, Value: strconv.Itoa(n)}
}

func minutes(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Minute
}
