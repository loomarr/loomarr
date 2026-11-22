package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDeviceFlow is the server half of /v1/auth/device/{start,poll} as internal/api/people
// implements it: start returns codes; poll answers 428 while pending, 404 once dead, 200 with the
// token once approved. Its status codes are the contract `loomarr pair` maps, so they are spelled
// out here rather than imported.
type fakeDeviceFlow struct {
	prefix      string
	startStatus int
	// failFirst are answers the poll gives before anything else, in order: an HTTP status, or 0
	// to drop the connection with no response, the way a restart or a Wi-Fi drop looks.
	failFirst []int
	// pendingPolls is how many 428s the poll answers (after failFirst) before its final answer.
	pendingPolls int32
	finalStatus  int
	finalBody    string
	expiresIn    time.Duration

	polls     atomic.Int32
	startName string
}

func (f *fakeDeviceFlow) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+f.prefix+"/v1/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			DeviceName string `json:"deviceName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("start body is not JSON: %v", err)
		}
		f.startName = in.DeviceName
		if f.startStatus != 0 && f.startStatus != http.StatusOK {
			w.WriteHeader(f.startStatus)
			return
		}
		expires := f.expiresIn
		if expires == 0 {
			expires = 10 * time.Minute
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deviceCode": "device-secret",
			"userCode":   "BCDF-GHJK",
			"expiresAt":  time.Now().Add(expires).UTC().Format(time.RFC3339),
			"interval":   5,
		})
	})
	mux.HandleFunc("POST "+f.prefix+"/v1/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			DeviceCode string `json:"deviceCode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.DeviceCode != "device-secret" {
			t.Errorf("poll sent device code %q, want the one /start returned", in.DeviceCode)
		}
		n := f.polls.Add(1)
		if i := int(n) - 1; i < len(f.failFirst) {
			if f.failFirst[i] == 0 {
				conn, _, err := http.NewResponseController(w).Hijack()
				if err != nil {
					t.Fatalf("hijack: %v", err)
				}
				_ = conn.Close()
				return
			}
			w.WriteHeader(f.failFirst[i])
			return
		}
		if n-int32(len(f.failFirst)) <= f.pendingPolls {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusPreconditionRequired)
			_, _ = w.Write([]byte(`{"title":"Precondition Required","status":428,"detail":"Waiting for approval"}`))
			return
		}
		status := f.finalStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		body := f.finalBody
		if body == "" && status == http.StatusOK {
			body = `{"token":"tok-123","deviceName":"Agent CLI"}`
		}
		_, _ = w.Write([]byte(body))
	})
	return mux
}

// testPairing points a pairing at srv with a sleep that returns at once (counting the waits) and
// the real clock, unless the test swaps now.
func testPairing(t *testing.T, srv *httptest.Server, prefix string) (*pairing, *atomic.Int32) {
	t.Helper()
	p, err := newPairing(srv.URL + prefix)
	if err != nil {
		t.Fatal(err)
	}
	var sleeps atomic.Int32
	p.sleep = func(ctx context.Context, d time.Duration) error {
		if d != 5*time.Second {
			t.Errorf("waited %v between polls, want the server's interval of 5s", d)
		}
		sleeps.Add(1)
		return ctx.Err()
	}
	return p, &sleeps
}

func TestPairPrintsOnlyTheTokenOnStdout(t *testing.T) {
	// The load-bearing property: `export LOOMARR_TOKEN=$(loomarr pair ...)` must capture the token
	// and nothing else, while the person still sees where to approve on stderr.
	f := &fakeDeviceFlow{pendingPolls: 2}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, sleeps := testPairing(t, srv, "")

	var stdout, stderr bytes.Buffer
	if err := pairAndPrint(context.Background(), p, "Agent CLI", &stdout, &stderr); err != nil {
		t.Fatalf("pair: %v\nstderr:\n%s", err, stderr.String())
	}
	if got := stdout.String(); got != "tok-123\n" {
		t.Errorf("stdout = %q, want exactly the token and a newline", got)
	}
	if !strings.Contains(stderr.String(), srv.URL+"/pair?code=BCDF-GHJK") {
		t.Errorf("stderr does not show the approval link; got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "restish api connect loomarr '"+srv.URL+"' --spec '"+srv.URL+"/v1/openapi.json'") {
		t.Errorf("stderr does not show the Restish setup; got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "tok-123") {
		t.Error("the token leaked onto stderr, which terminals and logs keep")
	}
	if f.startName != "Agent CLI" {
		t.Errorf("start sent device name %q, want %q", f.startName, "Agent CLI")
	}
	if got := f.polls.Load(); got != 3 {
		t.Errorf("polled %d times, want 3 (two pending, then approved)", got)
	}
	if got := sleeps.Load(); got != 3 {
		t.Errorf("waited %d times, want one wait before each poll", got)
	}
}

func TestPairKeepsAReverseProxyPrefix(t *testing.T) {
	f := &fakeDeviceFlow{prefix: "/loomarr"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, _ := testPairing(t, srv, "/loomarr/")

	var stdout, stderr bytes.Buffer
	if err := pairAndPrint(context.Background(), p, "Agent CLI", &stdout, &stderr); err != nil {
		t.Fatalf("pair behind a prefix: %v", err)
	}
	for _, want := range []string{srv.URL + "/loomarr/pair?code=BCDF-GHJK", srv.URL + "/loomarr/v1/openapi.json"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr is missing %q; got:\n%s", want, stderr.String())
		}
	}
}

func TestPairFailures(t *testing.T) {
	tests := []struct {
		name    string
		flow    *fakeDeviceFlow
		wantErr error
		wantMsg string
	}{
		{"expired code", &fakeDeviceFlow{pendingPolls: 1, finalStatus: http.StatusNotFound}, errPairingExpired, "run `loomarr pair` again"},
		{"not a Loomarr server", &fakeDeviceFlow{startStatus: http.StatusNotFound}, nil, "does not offer device pairing"},
		{"pairing throttled", &fakeDeviceFlow{startStatus: http.StatusTooManyRequests}, nil, "too many pairings"},
		{"approved without a token", &fakeDeviceFlow{finalBody: `{"deviceName":"x"}`}, nil, "sent no token"},
		{"a reply that is not JSON", &fakeDeviceFlow{finalBody: `<html>`}, nil, "not Loomarr's"},
		{"a refusal ends it", &fakeDeviceFlow{failFirst: []int{http.StatusBadRequest}}, nil, "returned 400"},
		{"a redirect mid-wait ends it", &fakeDeviceFlow{failFirst: []int{http.StatusFound}}, nil, "redirected"},
		{"a lost approval says a device may exist", &fakeDeviceFlow{failFirst: []int{0}, finalStatus: http.StatusNotFound}, errPairingExpired, "may have been approved"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.flow.handler(t))
			defer srv.Close()
			p, _ := testPairing(t, srv, "")

			var stdout, stderr bytes.Buffer
			err := pairAndPrint(context.Background(), p, "Agent CLI", &stdout, &stderr)
			if err == nil {
				t.Fatal("pairing succeeded, want an error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("err = %q, want it to say %q", err, tc.wantMsg)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q on failure; a failed pair must print no token", stdout.String())
			}
		})
	}
}

func TestPairStopsWhenTheCodeExpiresWhileWaiting(t *testing.T) {
	// The server never answers 404 here (it stays pending); the client's own reading of
	// expiresAt is what ends the wait instead of polling forever.
	f := &fakeDeviceFlow{pendingPolls: 1 << 30, expiresIn: time.Minute}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, _ := testPairing(t, srv, "")
	clock := time.Now()
	p.now = func() time.Time { clock = clock.Add(30 * time.Second); return clock }

	_, err := p.run(context.Background(), "Agent CLI", &bytes.Buffer{})
	if !errors.Is(err, errPairingExpired) {
		t.Fatalf("err = %v, want errPairingExpired once the expiry passes", err)
	}
	if got := f.polls.Load(); got > 3 {
		t.Errorf("polled %d times after a one-minute expiry at 30s per step", got)
	}
}

func TestPairStopsOnInterrupt(t *testing.T) {
	f := &fakeDeviceFlow{pendingPolls: 1 << 30}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, _ := testPairing(t, srv, "")
	ctx, cancel := context.WithCancel(context.Background())
	p.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }

	_, err := p.run(ctx, "Agent CLI", &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestPairRidesOutTransientFailures(t *testing.T) {
	// The person may be halfway through approving on their phone when the server restarts or the
	// Wi-Fi drops. Those failures must not throw the code away; the next poll that gets through
	// carries on.
	f := &fakeDeviceFlow{failFirst: []int{0, http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusBadGateway}, pendingPolls: 1}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, _ := testPairing(t, srv, "")

	var stdout, stderr bytes.Buffer
	if err := pairAndPrint(context.Background(), p, "Agent CLI", &stdout, &stderr); err != nil {
		t.Fatalf("pair: %v\nstderr:\n%s", err, stderr.String())
	}
	if stdout.String() != "tok-123\n" {
		t.Errorf("stdout = %q, want the token", stdout.String())
	}
	if got := strings.Count(stderr.String(), "trying again"); got != 4 {
		t.Errorf("stderr reports %d retries, want 4; got:\n%s", got, stderr.String())
	}
}

func TestPairStopsRetryingWhenTheCodeExpires(t *testing.T) {
	// A server that stays down must still end the wait: the code's own expiry is the bound.
	f := &fakeDeviceFlow{failFirst: slices.Repeat([]int{http.StatusServiceUnavailable}, 1000), expiresIn: time.Minute}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p, _ := testPairing(t, srv, "")
	clock := time.Now()
	p.now = func() time.Time { clock = clock.Add(30 * time.Second); return clock }

	_, err := p.run(context.Background(), "Agent CLI", &bytes.Buffer{})
	if !errors.Is(err, errPairingExpired) {
		t.Fatalf("err = %v, want errPairingExpired", err)
	}
	if got := f.polls.Load(); got > 3 {
		t.Errorf("polled %d times against a one-minute code at 30s per step", got)
	}
}

func TestPairReportsARedirectInsteadOfFollowingIt(t *testing.T) {
	// http:// behind a proxy that redirects to https:// is the common case. Following a 302 would
	// turn the POST into a bodiless GET and blame the address; following a 307 would send the
	// device code to wherever the redirect points.
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var followed atomic.Bool
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed.Store(true) }))
			defer target.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+r.URL.Path, status)
			}))
			defer srv.Close()
			p, _ := testPairing(t, srv, "")

			_, err := p.run(context.Background(), "Agent CLI", &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "redirected to "+target.URL) {
				t.Fatalf("err = %v, want it to name the redirect target", err)
			}
			if followed.Load() {
				t.Error("the redirect was followed, so the request reached the other host")
			}
		})
	}
}

func TestPairRejectsAStartWithoutUsableCodes(t *testing.T) {
	tests := []struct{ name, body, wantMsg string }{
		{"no codes", `{"deviceCode":"","userCode":"","expiresAt":"2030-01-01T00:00:00Z","interval":5}`, "without a pairing code"},
		{"unreadable expiry", `{"deviceCode":"d","userCode":"BCDF-GHJK","expiresAt":"soon","interval":5}`, "unreadable expiry"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			p, _ := testPairing(t, srv, "")
			_, err := p.run(context.Background(), "Agent CLI", &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("err = %v, want it to say %q", err, tc.wantMsg)
			}
		})
	}
}

func TestPollIntervalIsClamped(t *testing.T) {
	for in, want := range map[int]time.Duration{-1: 5 * time.Second, 0: 5 * time.Second, 1: time.Second, 5: 5 * time.Second, 3600: 30 * time.Second} {
		if got := pollInterval(in); got != want {
			t.Errorf("pollInterval(%d) = %v, want %v", in, got, want)
		}
	}
}

func TestMinutesLeftRoundsUp(t *testing.T) {
	for in, want := range map[time.Duration]string{
		10 * time.Minute:               "10 minutes",
		9*time.Minute + 59*time.Second: "10 minutes",
		90 * time.Second:               "2 minutes",
		5 * time.Second:                "1 minute",
		-time.Second:                   "1 minute",
	} {
		if got := minutesLeft(in); got != want {
			t.Errorf("minutesLeft(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestRunPairRejectsALongName(t *testing.T) {
	err := runPair([]string{"--server", "http://nas.local", "--name", strings.Repeat("x", 65)}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "at most 64") {
		t.Fatalf("err = %v, want the 64-character limit", err)
	}
}

func TestParseServerURL(t *testing.T) {
	tests := []struct {
		in, want, wantErr string
	}{
		{in: "http://nas.local:8080", want: "http://nas.local:8080"},
		{in: "  https://tv.example/  ", want: "https://tv.example"},
		{in: "https://tv.example/loomarr/", want: "https://tv.example/loomarr"},
		{in: "", wantErr: "--server is required"},
		{in: "nas.local:8080", wantErr: "needs http:// or https://"},
		{in: "ftp://nas.local", wantErr: "needs http:// or https://"},
		{in: "http://", wantErr: "has no host"},
		{in: "http://nas.local/?x=1", wantErr: "without ? or #"},
		{in: "http://me:secret@nas.local", wantErr: "should not contain a username or password"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			u, err := parseServerURL(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to say %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if u.String() != tc.want {
				t.Errorf("got %q, want %q", u.String(), tc.want)
			}
		})
	}
}

func TestRunPairRejectsStrayArguments(t *testing.T) {
	// `loomarr pair http://nas` (forgetting --server) must fail clearly rather than try to pair
	// with LOOMARR_SERVER or nothing.
	t.Setenv("LOOMARR_SERVER", "")
	err := runPair([]string{"http://nas.local"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "use --server") {
		t.Fatalf("err = %v, want a pointer to --server", err)
	}
}

func TestDispatchRoutesPair(t *testing.T) {
	// Without the dispatch case `pair` would be an unknown subcommand. With it, a missing
	// --server is the error, which proves the route reached runPair without starting a server.
	t.Setenv("LOOMARR_SERVER", "")
	err := dispatch([]string{"pair"})
	if err == nil || errors.Is(err, errUnknownSubcommand) || !strings.Contains(err.Error(), "--server is required") {
		t.Fatalf("dispatch(pair) = %v, want runPair's missing --server error", err)
	}
}
