// Package agents is Loomarr's surface for AI agents (#4483): an MCP endpoint whose tools are thin
// wrappers over the existing HTTP API.
//
// WHY EVERY TOOL GOES THROUGH THE ROUTER. A tool never calls a store or a domain service. It sends
// an in-process request through the same http.Handler the apps use, carrying the caller's own
// credential. So a tool can do exactly what its caller's role allows — the route's
// httpcore.WithRole check, validation and error shapes all apply unchanged — and this package
// holds no permission model of its own to drift from the routes. It is in-process, not a
// loopback socket: nothing leaves the process and no port is involved.
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxReplyBytes bounds what one in-process call may return to a tool. The largest replies a tool
// reads are the full channel list (every channel's lineup and policy, which the channel lookup
// needs) and a 24-hour guide across the lineup with its breaks; both stay in the low megabytes for
// a large household. A reply past the cap is reported as an error rather than held whole.
const maxReplyBytes = 16 << 20

// Dispatcher sends requests through the API as the caller.
type Dispatcher struct {
	api http.Handler
}

// NewDispatcher wraps the API's full handler: the one with authentication and every route on it.
func NewDispatcher(api http.Handler) *Dispatcher { return &Dispatcher{api: api} }

// Reply is one in-process response.
type Reply struct {
	Status int
	Body   []byte
}

// OK reports a 2xx status.
func (r Reply) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Decode unmarshals a successful reply into out.
func (r Reply) Decode(out any) error {
	if err := json.Unmarshal(r.Body, out); err != nil {
		return fmt.Errorf("unexpected reply from the API: %w", err)
	}
	return nil
}

// Err turns a failed reply into an error a model can act on: the route's own problem detail when
// it sent one (huma answers RFC 9457 problem+json), else the status.
func (r Reply) Err() error {
	var problem struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if json.Unmarshal(r.Body, &problem) == nil && (problem.Detail != "" || problem.Title != "") {
		msg := problem.Detail
		if msg == "" {
			msg = problem.Title
		}
		return &APIError{Status: r.Status, Message: msg}
	}
	return &APIError{Status: r.Status, Message: http.StatusText(r.Status)}
}

// APIError is a route's refusal. Its message leads with the status ("403: …"), which is what the
// model sees as the tool's error and what tells it "not allowed" apart from "not configured here".
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("%d: %s", e.Status, e.Message) }

// Caller is who an MCP request came from, carried onto every nested call a tool makes.
//
// Only the credential and the client's address travel. Cookies do not: an MCP client
// authenticates with a Bearer token, and a Bearer request is exempt from the CSRF guard (hasBearer
// in internal/api/sessionauth.go), so a tool's POST needs no CSRF header.
//
// WHY THE ADDRESS IS THE CLIENT'S, NOT THIS PROCESS'S. A nested call stamped 127.0.0.1 would be
// trusted wherever loopback is, and every agent would share one rate-limit key. Carrying the real
// socket peer and its forwarded headers makes a nested call look exactly as the MCP request did:
// the same trusted-proxy decision (httpcore.Origin), the same client address, the same limits.
type Caller struct {
	authorization string
	remoteAddr    string
	forwarded     http.Header
}

// forwardedHeaders are the request headers that decide where a request came from (httpcore.Origin).
var forwardedHeaders = []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"}

// CallerFrom captures r's credential and origin.
func CallerFrom(r *http.Request) Caller {
	c := Caller{authorization: r.Header.Get("Authorization"), remoteAddr: r.RemoteAddr, forwarded: http.Header{}}
	for _, h := range forwardedHeaders {
		if v := r.Header.Values(h); len(v) > 0 {
			c.forwarded[h] = v
		}
	}
	return c
}

// Do sends one request through the API as c. query may be nil; body, when not nil, is sent as JSON.
func (d *Dispatcher) Do(ctx context.Context, c Caller, method, path string, query url.Values, body any) (Reply, error) {
	var rdr io.Reader = http.NoBody
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return Reply{}, err
		}
		rdr = bytes.NewReader(buf)
	}
	target := path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	// The nested request gets a context with NONE of the outer request's values: the outer
	// identity (user, device, role) must never be what a nested route sees, only the credential
	// below. It still ends when the outer request does.
	nested, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(nested, method, target, rdr)
	if err != nil {
		return Reply{}, err
	}
	req.RemoteAddr = c.remoteAddr
	for h, v := range c.forwarded {
		req.Header[h] = v
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "loomarr-mcp")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.authorization != "" {
		req.Header.Set("Authorization", c.authorization)
	}
	rec := &recorder{header: http.Header{}}
	d.api.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	if rec.truncated {
		return Reply{}, fmt.Errorf("the API's reply to %s %s was larger than %d bytes", method, path, maxReplyBytes)
	}
	return Reply{Status: rec.status, Body: rec.body.Bytes()}, nil
}

// recorder is a minimal in-memory ResponseWriter with a size cap.
type recorder struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	truncated bool
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if room := maxReplyBytes - r.body.Len(); len(p) > room {
		r.body.Write(p[:max(room, 0)])
		r.truncated = true
		return len(p), nil
	}
	return r.body.Write(p)
}

// bearer returns the request's Authorization header when it is a Bearer credential, else "".
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > len("Bearer ") && strings.EqualFold(h[:len("Bearer ")], "Bearer ") {
		return h
	}
	return ""
}
