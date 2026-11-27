package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pairing is the client half of the device flow (/v1/auth/device/start, then /poll), used by
// `loomarr pair` so an agent's CLI signs in the same way a TV app does (#4484).
//
// WHY THE DEVICE FLOW AND NOT API_TOKEN: API_TOKEN is admin-only break-glass and has no user. A
// paired device acts as the person who approved it, with their role, and picks up a demotion or a
// disabled account on its next request (ADR 0043). It is revoked in Settings → Devices like any
// other device, which is the revocation story agents need.
type pairing struct {
	client *http.Client
	base   *url.URL
	// sleep waits between polls. Tests replace it so a five-second interval costs nothing; it
	// must return ctx.Err() when the context ends so Ctrl-C stops a waiting pair at once.
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
}

// pairRequestTimeout bounds each start/poll request, not the whole pairing: a person may take
// minutes to approve, but a single request that hangs is a server or network problem.
const pairRequestTimeout = 15 * time.Second

var (
	// errPairingExpired is the poll's 404: the code expired or was already redeemed. The person
	// has to start again; polling further cannot succeed.
	errPairingExpired = errors.New("pairing expired or was already used")
	// errPairingPending is the poll's 428, the normal "keep waiting" answer.
	errPairingPending = errors.New("waiting for approval")
)

func newPairing(server string) (*pairing, error) {
	base, err := parseServerURL(server)
	if err != nil {
		return nil, err
	}
	return &pairing{
		client: &http.Client{
			Timeout: pairRequestTimeout,
			// Redirects are reported, never followed. A 301/302 turns the POST into a bodiless GET
			// that fails as "not a Loomarr server", and a 307/308 would re-send the device code to
			// whatever host the redirect names. Telling the person the right address is both
			// clearer and safer.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		base:  base,
		sleep: sleepCtx,
		now:   time.Now,
	}, nil
}

// parseServerURL accepts what a person types for their server ("http://nas:8080",
// "https://loomarr.example.com/") and rejects what cannot be one. A path is kept so an instance
// behind a reverse-proxy prefix still works; a query or fragment is refused because it would be
// silently dropped.
func parseServerURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("--server is required, for example --server http://nas.local:8080")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("--server %q is not a URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--server %q needs http:// or https://", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("--server %q has no host", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("--server %q should be just the address, without ? or #", raw)
	}
	// A user:password in the URL would be echoed in every error and in the printed Restish
	// commands, which end up in shell history and pasted logs. Pairing needs no credential.
	if u.User != nil {
		return nil, errors.New("--server should not contain a username or password; pairing signs in through the approval page")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

// endpoint joins a path onto the server's base, keeping any reverse-proxy prefix.
func (p *pairing) endpoint(path string) string {
	u := *p.base
	u.Path = p.base.Path + path
	return u.String()
}

type pairStart struct {
	DeviceCode string `json:"deviceCode"`
	UserCode   string `json:"userCode"`
	ExpiresAt  string `json:"expiresAt"`
	Interval   int    `json:"interval"`
}

type pairResult struct {
	Token      string `json:"token"`
	DeviceName string `json:"deviceName"`
}

// approvalURL is the web page that approves this code. /pair reads ?code= and fills it in, so
// the person only has to sign in and press Approve.
func (p *pairing) approvalURL(userCode string) string {
	u := *p.base
	u.Path = p.base.Path + "/pair"
	u.RawQuery = url.Values{"code": {userCode}}.Encode()
	return u.String()
}

func (p *pairing) start(ctx context.Context, deviceName string) (pairStart, error) {
	var out pairStart
	status, location, err := p.post(ctx, "/v1/auth/device/start", map[string]string{"deviceName": deviceName}, &out)
	if err != nil {
		return out, err
	}
	switch {
	case status == http.StatusOK:
	case isRedirect(status):
		return out, errRedirected(p.base, location)
	case status == http.StatusNotFound:
		// The route is registered only when device pairing is enabled, and an older or
		// non-Loomarr server has no such path. Either way, say what was reached.
		return out, fmt.Errorf("%s does not offer device pairing; check the address is a Loomarr server", p.base)
	case status == http.StatusTooManyRequests:
		// The server allows a burst of five, then one every 20 seconds (buildoperations.go).
		return out, errors.New("too many pairings were started from this address; wait about 20 seconds and try again")
	default:
		return out, fmt.Errorf("starting the pairing failed: %s returned %d", p.base, status)
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return out, fmt.Errorf("%s answered without a pairing code; check the address is a Loomarr server", p.base)
	}
	return out, nil
}

// poll asks once whether the pairing was approved. It returns errPairingPending while the
// person has not approved yet, errPairingExpired when the code is dead, and the token once
// approved. Any other failure is returned as is; pollRetryable decides whether it ends pairing.
func (p *pairing) poll(ctx context.Context, deviceCode string) (pairResult, int, error) {
	var out pairResult
	status, location, err := p.post(ctx, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, &out)
	if err != nil {
		// Keep post's status: 0 means no response arrived and is retried, but a 200 whose body
		// is not Loomarr's is an answer. Reporting it as 0 would retry it until the code expired.
		return out, status, err
	}
	switch {
	case isRedirect(status):
		return out, status, errRedirected(p.base, location)
	case status == http.StatusOK:
		if out.Token == "" {
			return out, status, errors.New("the server approved the pairing but sent no token")
		}
		return out, status, nil
	case status == http.StatusPreconditionRequired:
		return out, status, errPairingPending
	case status == http.StatusNotFound:
		return out, status, errPairingExpired
	default:
		return out, status, fmt.Errorf("checking the pairing failed: %s returned %d", p.base, status)
	}
}

// pollRetryable decides whether a failed poll ends the pairing or is tried again at the next
// interval. status is the HTTP status, or 0 when no response arrived (connection dropped,
// timeout, DNS).
//
// Transient failures are retried until the code expires: the person may be halfway through
// approving on their phone, and a Wi-Fi drop or a server restart (Loomarr rebuilds itself in
// place) must not throw their code away. The code's own expiry bounds the retrying, so a
// server that stays down still ends the wait within PairingTTL. Everything else — another 4xx,
// a redirect, a 200 that is not Loomarr's — is an answer, and asking again cannot change it.
//
// errPairingPending and errPairingExpired never reach here: run handles them first.
func pollRetryable(status int) bool {
	return status == 0 || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// run starts a pairing, tells the person where to approve it, and polls until it is approved,
// expires, or ctx ends. prompt receives the human-facing lines; it is never the token's
// destination, so a caller can send it to stderr and keep stdout for the token alone.
func (p *pairing) run(ctx context.Context, deviceName string, prompt io.Writer) (pairResult, error) {
	st, err := p.start(ctx, deviceName)
	if err != nil {
		return pairResult{}, err
	}
	expires, err := time.Parse(time.RFC3339, st.ExpiresAt)
	if err != nil {
		return pairResult{}, fmt.Errorf("the server sent an unreadable expiry %q: %w", st.ExpiresAt, err)
	}
	interval := pollInterval(st.Interval)

	_, _ = fmt.Fprintf(prompt, "To approve %q, open:\n\n  %s\n\nand sign in, or enter the code %s on the Pair page.\n",
		deviceName, p.approvalURL(st.UserCode), st.UserCode)
	// A duration, not a clock time: the documented path runs in a container whose zone is UTC,
	// so "expires at 22:05" would be hours off the person's own clock.
	_, _ = fmt.Fprintf(prompt, "Waiting for approval (the code expires in %s)...\n",
		minutesLeft(expires.Sub(p.now())))

	// retried records that a poll failed in transit. The server redeems a code when it answers
	// the approving poll, so if that answer was the one lost, the next poll sees 404 for a code
	// that was in fact approved, and the person should know a device may already exist.
	retried := false
	for {
		if err := p.sleep(ctx, interval); err != nil {
			return pairResult{}, err
		}
		res, status, err := p.poll(ctx, st.DeviceCode)
		switch {
		case err == nil:
			return res, nil
		case errors.Is(err, errPairingExpired) && retried:
			return pairResult{}, fmt.Errorf("%w; it may have been approved while the connection was down, so check your paired devices, then run `loomarr pair` again", errPairingExpired)
		case errors.Is(err, errPairingExpired):
			return pairResult{}, fmt.Errorf("%w; run `loomarr pair` again for a new code", errPairingExpired)
		case errors.Is(err, errPairingPending):
		case ctx.Err() != nil:
			return pairResult{}, ctx.Err()
		case pollRetryable(status):
			retried = true
			_, _ = fmt.Fprintf(prompt, "Could not check the pairing (%v); trying again...\n", err)
		default:
			return pairResult{}, err
		}
		if !p.now().Before(expires) {
			return pairResult{}, fmt.Errorf("%w; run `loomarr pair` again for a new code", errPairingExpired)
		}
	}
}

// post sends a JSON body and decodes a 200 response into out. A non-200 status is returned
// without decoding: the device routes answer errors as problem+json, and the caller maps the
// status, which is the part that decides what happens next. location is the Location header,
// which only a redirect carries.
func (p *pairing) post(ctx context.Context, path string, body any, out any) (status int, location string, err error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(path), bytes.NewReader(buf))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("could not reach %s: %w", p.base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return resp.StatusCode, resp.Header.Get("Location"), nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return resp.StatusCode, "", fmt.Errorf("%s sent a reply that is not Loomarr's: %w", p.base, err)
	}
	return resp.StatusCode, "", nil
}

func isRedirect(status int) bool { return status >= 300 && status < 400 }

// errRedirected names where the server wanted to send us, so the person can rerun with that
// address — most often http:// behind a proxy that redirects to https://.
func errRedirected(base *url.URL, location string) error {
	if location == "" {
		return fmt.Errorf("%s redirected without saying where; check the address", base)
	}
	return fmt.Errorf("%s redirected to %s; run `loomarr pair` again with that address as --server", base, location)
}

// pollInterval turns the server's advisory interval into a wait, clamped so a bad value can
// neither hammer the server nor sleep past the code's expiry.
func pollInterval(seconds int) time.Duration {
	switch {
	case seconds <= 0:
		return 5 * time.Second
	case seconds > 30:
		return 30 * time.Second
	default:
		return time.Duration(seconds) * time.Second
	}
}

// minutesLeft says how long a code lasts in words, rounded up so "1 minute" never means 5 seconds
// have already gone.
func minutesLeft(d time.Duration) string {
	m := int((d + time.Minute - 1) / time.Minute)
	if m <= 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
