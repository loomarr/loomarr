package playoutcert

import (
	"bufio"
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
	"time"
)

type endpoint struct {
	base    *url.URL
	client  *http.Client
	bearer  string
	device  string
	timeout time.Duration
	audit   *auditCapsule
}

func newEndpoint(config Config, capsules ...*auditCapsule) (*endpoint, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, err
	}
	base.Path = strings.TrimRight(base.Path, "/")
	var audit *auditCapsule
	if len(capsules) > 0 {
		audit = capsules[0]
	}
	client := *config.Client
	priorRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !sameOrigin(base, req.URL) {
			return errors.New("cross-origin redirect refused")
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if priorRedirect != nil {
			if err := priorRedirect(req, via); err != nil {
				return err
			}
		}
		if !sameOrigin(base, req.URL) {
			return errors.New("cross-origin redirect refused")
		}
		if audit != nil && !audit.registerRequestURL(req.URL, requestProvenancePrivate) {
			return errors.New("publication audit unavailable")
		}
		return nil
	}
	return &endpoint{base: base, client: &client, bearer: config.AdminBearer, device: config.DeviceToken, timeout: config.RequestTimeout, audit: audit}, nil
}

func (e *endpoint) resolve(path string) (*url.URL, error) {
	rel, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	resolved := e.base.ResolveReference(rel)
	if !sameOrigin(e.base, resolved) {
		return nil, errors.New("cross-origin URL refused")
	}
	return resolved, nil
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (e *endpoint) request(ctx context.Context, method, path string, body io.Reader, admin bool) (*http.Response, error) {
	return e.requestForProvenance(ctx, method, path, body, admin, e.timeout, requestProvenancePrivate)
}

// requestFor permits a caller-owned deadline for a long-lived admitted stream.
// A zero timeout does not make the request unbounded: its supplied context must
// already carry the phase deadline.
func (e *endpoint) requestFor(ctx context.Context, method, path string, body io.Reader, admin bool, timeout time.Duration) (*http.Response, error) {
	return e.requestForProvenance(ctx, method, path, body, admin, timeout, requestProvenancePrivate)
}

func (e *endpoint) requestForProvenance(ctx context.Context, method, path string, body io.Reader, admin bool, timeout time.Duration, provenance requestProvenance) (*http.Response, error) {
	requestURL, err := e.resolve(path)
	if err != nil {
		return nil, err
	}
	if e.audit != nil && !e.audit.registerRequestURL(requestURL, provenance) {
		return nil, errors.New("publication audit unavailable")
	}
	requestCtx, cancel := context.WithCancel(ctx)
	if timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	req, err := http.NewRequestWithContext(requestCtx, method, requestURL.String(), body)
	if err != nil {
		cancel()
		return nil, err
	}
	if admin {
		req.Header.Set("Authorization", "Bearer "+e.bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }

var errResponseTooLarge = errors.New("response body exceeds size limit")

// boundedReader permits at most limit bytes from body.  A read after those bytes
// probes one additional byte so an exact-limit EOF is distinguishable from a
// truncated prefix.  The probe byte is never returned to the caller.
type boundedReader struct {
	body      io.Reader
	remaining int64
}

func newBoundedReader(body io.Reader, limit int64) *boundedReader {
	return &boundedReader{body: body, remaining: limit}
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.body.Read(probe[:])
		if n > 0 {
			return 0, errResponseTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func readBoundedBody(body io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(newBoundedReader(body, limit))
}

func decodeBoundedJSON(body io.Reader, limit int64, output any) error {
	decoder := json.NewDecoder(newBoundedReader(body, limit))
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("response has trailing JSON value")
		}
		return err
	}
	return nil
}

func (e *endpoint) target(ctx context.Context, channelCount int, digest string) (Target, sessionSnapshot, error) {
	var version struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Ready   bool   `json:"ready"`
	}
	if err := e.getJSON(ctx, "/v1/system/version", true, &version); err != nil {
		return Target{}, sessionSnapshot{}, err
	}
	if !version.Ready {
		return Target{}, sessionSnapshot{}, errors.New("target is not ready")
	}
	if !safeIdentity(version.Version, 64, false) || !safeIdentity(version.Commit, 40, true) {
		return Target{}, sessionSnapshot{}, errors.New("target build identity is invalid")
	}
	sessions, err := e.sessions(ctx)
	if err != nil {
		return Target{}, sessionSnapshot{}, err
	}
	if !sessions.Running || sessions.Capacity <= 0 {
		return Target{}, sessionSnapshot{}, errors.New("internal playout capacity is unavailable")
	}
	return Target{Version: version.Version, Revision: version.Commit, ManifestSHA256: digest, ConfiguredChannels: channelCount, Capacity: sessions.Capacity}, sessions, nil
}

func (e *endpoint) getJSON(ctx context.Context, path string, admin bool, output any) error {
	return e.getJSONProvenance(ctx, path, admin, output, requestProvenancePrivate)
}

func (e *endpoint) getJSONProvenance(ctx context.Context, path string, admin bool, output any, provenance requestProvenance) error {
	resp, err := e.requestForProvenance(ctx, http.MethodGet, path, nil, admin, e.timeout, provenance)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	if err := decodeBoundedJSON(resp.Body, 2<<20, output); err != nil {
		return err
	}
	return nil
}

type sessionSnapshot struct {
	Running       bool           `json:"running"`
	Capacity      int            `json:"capacity"`
	Active        int            `json:"active"`
	ViewerActive  int            `json:"viewerActiveSessions"`
	GraceIdle     int            `json:"graceIdleSessions"`
	TranscodeCost int            `json:"transcodeCost"`
	Sessions      []sessionState `json:"sessions"`
}

// sessionState is the existing per-(Channel, plan) telemetry projection.  Lifecycle
// evidence must name the raw tuner audience it exercised; aggregate totals can be
// affected by an unrelated Channel.
type sessionState struct {
	ChannelID string `json:"channelId"`
	Target    string `json:"target"`
	Viewers   int    `json:"viewers"`
}

func (s sessionSnapshot) session(channelID, target string) (sessionState, bool) {
	for _, item := range s.Sessions {
		if item.ChannelID == channelID && item.Target == target {
			return item, true
		}
	}
	return sessionState{}, false
}

type playoutStatusSnapshot struct {
	Running bool `json:"running"`
	GPU     struct {
		VRAMGiB    float64 `json:"vramGiB"`
		LLMVRAMGiB float64 `json:"llmVramGiB"`
		Contended  bool    `json:"contended"`
	} `json:"gpu"`
	Channels []struct {
		Health string `json:"health"`
	} `json:"channels"`
	Prepared struct {
		Channels      int `json:"channels"`
		ReadyChannels int `json:"readyChannels"`
	} `json:"prepared"`
}

func (e *endpoint) sessions(ctx context.Context) (sessionSnapshot, error) {
	var snapshot sessionSnapshot
	err := e.getJSON(ctx, "/v1/playout/sessions", true, &snapshot)
	return snapshot, err
}

func (e *endpoint) playoutStatus(ctx context.Context) (playoutStatusSnapshot, error) {
	var snapshot playoutStatusSnapshot
	err := e.getJSON(ctx, "/v1/playout/status", true, &snapshot)
	return snapshot, err
}

func (e *endpoint) mint(ctx context.Context, channelID string) (*url.URL, time.Duration, string) {
	started := time.Now()
	path := "/v1/channels/" + url.PathEscape(channelID) + "/play-url"
	resp, err := e.request(ctx, http.MethodPost, path, bytes.NewBufferString("{}"), true)
	if err != nil {
		return nil, 0, "request_failed"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, httpClass(resp.StatusCode)
	}
	var output struct {
		RelativeURL string `json:"relativeUrl"`
	}
	if err := decodeBoundedJSON(resp.Body, 1<<20, &output); err != nil {
		return nil, 0, "invalid_response"
	}
	signed, err := e.resolve(output.RelativeURL)
	if err != nil || signed.RawQuery == "" {
		return nil, 0, "invalid_signed_url"
	}
	if e.audit != nil && !e.audit.registerSignedURL(signed) {
		return nil, 0, "audit_unavailable"
	}
	return signed, time.Since(started), "ok"
}

func (e *endpoint) prepared(ctx context.Context, signed *url.URL) (time.Duration, bool, string) {
	probe := *signed
	query := probe.Query()
	query.Set("mode", "prepared")
	probe.RawQuery = query.Encode()
	started := time.Now()
	resp, err := e.request(ctx, http.MethodGet, probe.String(), nil, false)
	if err != nil {
		return 0, false, "request_failed"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return time.Since(started), false, "prepared_miss"
	}
	if resp.StatusCode != http.StatusOK {
		return 0, false, httpClass(resp.StatusCode)
	}
	manifest, err := readBoundedBody(resp.Body, 1<<20)
	if err != nil {
		return 0, false, "body_failed"
	}
	assetRef, err := firstMediaReference(manifest)
	if err != nil {
		return 0, false, "invalid_hls"
	}
	asset := probe.ResolveReference(assetRef)
	if !sameOrigin(e.base, asset) {
		return 0, false, "invalid_hls"
	}
	assetResp, err := e.request(ctx, http.MethodGet, asset.String(), nil, false)
	if err != nil {
		return 0, false, "request_failed"
	}
	defer func() { _ = assetResp.Body.Close() }()
	if assetResp.StatusCode != http.StatusOK {
		return 0, false, httpClass(assetResp.StatusCode)
	}
	var first [1]byte
	if _, err := io.ReadFull(assetResp.Body, first[:]); err != nil {
		return 0, false, "empty_asset"
	}
	return time.Since(started), true, "ok"
}

func firstMediaReference(manifest []byte) (*url.URL, error) {
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		reference, err := url.Parse(line)
		if err != nil || reference.IsAbs() || strings.Contains(reference.Path, "..") {
			return nil, errors.New("unsafe HLS reference")
		}
		return reference, nil
	}
	return nil, errors.New("HLS manifest has no media reference")
}

func httpClass(code int) string { return "http_" + strconv.Itoa(code) }

func (e *endpoint) metrics(ctx context.Context) (map[string]float64, error) {
	resp, err := e.request(ctx, http.MethodGet, "/metrics", nil, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics HTTP status %d", resp.StatusCode)
	}
	wanted := map[string]struct{}{
		"process_resident_memory_bytes": {}, "process_cpu_seconds_total": {}, "process_open_fds": {},
		"go_goroutines": {}, "loomarr_http_requests_in_flight": {}, "loomarr_playout_sessions_active": {},
	}
	values := make(map[string]float64, len(wanted))
	scanner := bufio.NewScanner(newBoundedReader(resp.Body, 4<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		if _, ok := wanted[fields[0]]; !ok {
			continue
		}
		value, parseErr := strconv.ParseFloat(fields[1], 64)
		if parseErr == nil {
			values[fields[0]] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for name := range wanted {
		if _, ok := values[name]; !ok {
			return nil, fmt.Errorf("missing metric %s", name)
		}
	}
	return values, nil
}

func (e *endpoint) metricTotal(ctx context.Context, name string) (float64, error) {
	resp, err := e.request(ctx, http.MethodGet, "/metrics", nil, false)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("metrics HTTP status %d", resp.StatusCode)
	}
	total, found := 0.0, false
	scanner := bufio.NewScanner(newBoundedReader(resp.Body, 4<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || (fields[0] != name && !strings.HasPrefix(fields[0], name+"{")) {
			continue
		}
		value, parseErr := strconv.ParseFloat(fields[1], 64)
		if parseErr != nil {
			return 0, errors.New("metric value is invalid")
		}
		total += value
		found = true
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if !found {
		return 0, errors.New("metric family is missing")
	}
	return total, nil
}

func (e *endpoint) ffmpegRunning(ctx context.Context) (int, error) {
	var page struct {
		Items []struct {
			Executable string `json:"executable"`
			Status     string `json:"status"`
		} `json:"items"`
	}
	if err := e.getJSONProvenance(ctx, "/v1/diagnostics/processes?status=running&limit=100", true, &page, requestProvenanceDiagnosticsFixedControls); err != nil {
		return 0, err
	}
	count := 0
	for _, item := range page.Items {
		if item.Status == "running" && (item.Executable == "ffmpeg" || strings.HasSuffix(item.Executable, "/ffmpeg")) {
			count++
		}
	}
	return count, nil
}

func safeIdentity(value string, limit int, hexadecimal bool) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for _, r := range value {
		if hexadecimal {
			if r < '0' || r > '9' && r < 'a' || r > 'f' {
				return false
			}
			continue
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("._+-", r) {
			return false
		}
	}
	return !hexadecimal || len(value) == limit
}

func (e *endpoint) sample(ctx context.Context, point string) (ResourceSample, error) {
	metrics, err := e.metrics(ctx)
	if err != nil {
		return ResourceSample{}, err
	}
	sessions, err := e.sessions(ctx)
	if err != nil {
		return ResourceSample{}, err
	}
	ffmpeg, err := e.ffmpegRunning(ctx)
	if err != nil {
		return ResourceSample{}, err
	}
	status, err := e.playoutStatus(ctx)
	if err != nil {
		return ResourceSample{}, err
	}
	stalled := 0
	for _, channel := range status.Channels {
		if channel.Health == "stalled" {
			stalled++
		}
	}
	return ResourceSample{
		Point: point, RSSBytes: metrics["process_resident_memory_bytes"], CPUSeconds: metrics["process_cpu_seconds_total"],
		OpenFDs: metrics["process_open_fds"], Goroutines: metrics["go_goroutines"], HTTPInFlight: metrics["loomarr_http_requests_in_flight"],
		SessionsActive: sessions.Active, ViewerActive: sessions.ViewerActive, GraceIdle: sessions.GraceIdle,
		TranscodeCost: sessions.TranscodeCost, Capacity: sessions.Capacity, FFmpegRunning: ffmpeg,
		PreparedChannels: status.Prepared.Channels, ReadyChannels: status.Prepared.ReadyChannels,
		ChannelHealth: len(status.Channels), StalledChannels: stalled,
		GPUVRAMGiB: status.GPU.VRAMGiB, LLMVRAMGiB: status.GPU.LLMVRAMGiB, GPUContended: status.GPU.Contended,
	}, nil
}
