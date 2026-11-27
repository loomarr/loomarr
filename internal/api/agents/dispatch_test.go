package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A tool's nested call looks to the API exactly as the MCP request did: the same credential and
// the same client address and forwarded headers, never this process's own loopback address, which
// would be trusted wherever loopback is and would put every agent on one rate-limit key.
func TestDispatchCarriesTheCallersCredentialAndOrigin(t *testing.T) {
	var got *http.Request
	d := NewDispatcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	in := httptest.NewRequest(http.MethodPost, "/v1/mcp", nil)
	in.RemoteAddr = "192.0.2.10:51234"
	in.Header.Set("Authorization", "Bearer device-token")
	in.Header.Set("X-Forwarded-For", "198.51.100.7")
	in.Header.Set("X-Forwarded-Proto", "https")
	in.Header.Set("Cookie", "loomarr_session=browser")

	if _, err := d.Do(context.Background(), CallerFrom(in), http.MethodPut, "/v1/me/favourites/c1", nil, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"RemoteAddr":        got.RemoteAddr,
		"Authorization":     got.Header.Get("Authorization"),
		"X-Forwarded-For":   got.Header.Get("X-Forwarded-For"),
		"X-Forwarded-Proto": got.Header.Get("X-Forwarded-Proto"),
	} {
		var sent string
		if name == "RemoteAddr" {
			sent = in.RemoteAddr
		} else {
			sent = in.Header.Get(name)
		}
		if want != sent {
			t.Errorf("%s = %q on the nested call, want the caller's %q", name, want, sent)
		}
	}
	if c := got.Header.Get("Cookie"); c != "" {
		t.Errorf("the nested call carried the cookie %q; only the Bearer credential travels", c)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("a body was sent without a JSON content type")
	}
}

type outerKey struct{}

// The nested request carries none of the outer request's context values (the auth middleware's
// user, device and role live there), yet still ends when the outer request is cancelled.
func TestDispatchIsolatesTheNestedContext(t *testing.T) {
	var sawOuter, sawCancel bool
	outer, cancel := context.WithCancel(context.WithValue(context.Background(), outerKey{}, "outer user"))
	d := NewDispatcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawOuter = r.Context().Value(outerKey{}) != nil
		cancel() // the outer request ends while the nested one is being served
		select {
		case <-r.Context().Done():
			sawCancel = true
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if _, err := d.Do(outer, Caller{}, http.MethodGet, "/v1/household", nil, nil); err != nil {
		t.Fatal(err)
	}
	if sawOuter {
		t.Error("the nested request could read the outer request's context values")
	}
	if !sawCancel {
		t.Error("cancelling the outer request did not end the nested one")
	}
}

// A reply past the cap is an error, never a silently truncated document decoded as if whole.
func TestDispatchRefusesAnOversizedReply(t *testing.T) {
	d := NewDispatcher(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxReplyBytes+1)))
	}))
	if _, err := d.Do(context.Background(), Caller{}, http.MethodGet, "/v1/guide", nil, nil); err == nil {
		t.Fatal("an oversized reply was returned")
	}
}
