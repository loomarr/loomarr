package playoutcert

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestAuditRequestCapturesRawRequestAndMintedURLs(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(
		step(http.StatusOK, ""),
		step(http.StatusOK, `{"relativeUrl":"/hls/a%2Fmaster.m3u8?sig=first%2Btoken"}`),
		step(http.StatusOK, `{"relativeUrl":"/hls/b%2Fmaster.m3u8?sig=second%2Btoken"}`),
		step(http.StatusOK, "#EXTM3U\nasset%2Fone.ts?part=a%2Bb\n"),
		step(http.StatusOK, "x"),
	)
	endpoint, capsule := auditEndpoint(t, transport)

	initial, err := endpoint.request(context.Background(), http.MethodGet, "/raw/%2Fpath?query=a%2Bb", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = initial.Body.Close()
	first, _, class := endpoint.mint(context.Background(), "one")
	if class != "ok" {
		t.Fatalf("first mint = %q", class)
	}
	second, _, class := endpoint.mint(context.Background(), "two")
	if class != "ok" {
		t.Fatalf("second mint = %q", class)
	}
	if _, ok, class := endpoint.prepared(context.Background(), first); !ok || class != "ok" {
		t.Fatalf("prepared = ok:%t class:%q", ok, class)
	}

	requests := transport.Requests()
	if len(requests) != 5 || requests[0].URL != "https://fixture.invalid/raw/%2Fpath?query=a%2Bb" || requests[3].URL != "https://fixture.invalid/hls/a%2Fmaster.m3u8?mode=prepared&sig=first%2Btoken" || requests[4].URL != "https://fixture.invalid/hls/asset%2Fone.ts?part=a%2Bb" {
		t.Fatalf("actual requests = %#v", requests)
	}
	for _, value := range []string{first.String(), second.String(), requests[0].URL, requests[3].URL, requests[4].URL} {
		if !auditHasProbe(capsule, value) {
			t.Fatalf("audit did not capture %q", value)
		}
	}
}

func TestAuditRequestCapturesSameOriginRedirectDestination(t *testing.T) {
	redirect := response(http.StatusFound, "")
	redirect.Header.Set("Location", "/redirect/%2Fnext?sig=dest%2Btoken")
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: redirect}, step(http.StatusOK, ""))
	endpoint, capsule := auditEndpoint(t, transport)

	resp, err := endpoint.request(context.Background(), http.MethodGet, "/start", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	const destination = "https://fixture.invalid/redirect/%2Fnext?sig=dest%2Btoken"
	if calls := transport.Calls(); calls != 2 || !auditHasProbe(capsule, destination) {
		t.Fatalf("redirect calls = %d, captured destination = %t", calls, auditHasProbe(capsule, destination))
	}
}

func TestAuditRequestDiagnosticsFixedControlsHaveProducerProvenance(t *testing.T) {
	t.Run("ffmpeg producer excludes only its fixed controls", func(t *testing.T) {
		transport := httpfixture.NewScriptedTransport(step(http.StatusOK, `{"items":[{"executable":"ffmpeg","status":"running"}]}`))
		config := Config{BaseURL: "https://fixture.invalid", AdminBearer: "100", DeviceToken: "device", Client: &http.Client{Transport: transport}, RequestTimeout: time.Second}
		capsule := newAuditCapsule(config)
		endpoint, err := newEndpoint(config, capsule)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := endpoint.ffmpegRunning(context.Background()); err != nil || got != 1 {
			t.Fatalf("ffmpeg running = %d, %v; want 1, nil", got, err)
		}
		const diagnosticsURL = "https://fixture.invalid/v1/diagnostics/processes?status=running&limit=100"
		if !auditHasProbe(capsule, diagnosticsURL) || !auditHasProbe(capsule, "status=running&limit=100") {
			t.Fatal("diagnostics URL and whole query were not retained")
		}
		if auditHasProbe(capsule, "running") {
			t.Fatal("fixed status filter was registered as private")
		}
		if document, reason, ok := parseJSONProvenance([]byte(`{"target":{"capacity":100}}`), false, newAuditWork(time.Now().Add(time.Second))); !ok {
			t.Fatalf("parse report: %q", reason)
		} else if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
			t.Fatalf("private bearer collision = %q, %q; want unavailable dynamic collision", status, reason)
		}
	})

	t.Run("ordinary diagnostic-shaped and signed requests remain private", func(t *testing.T) {
		transport := httpfixture.NewScriptedTransport(step(http.StatusOK, ""), step(http.StatusOK, ""))
		endpoint, capsule := auditEndpoint(t, transport)
		for _, path := range []string{
			"/v1/diagnostics/processes?status=running&limit=100",
			"/v1/diagnostics/processes?limit=100&sig=capability",
		} {
			resp, err := endpoint.request(context.Background(), http.MethodGet, path, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
		}
		for _, value := range []string{"running", "100", "capability"} {
			if !auditHasProbe(capsule, value) {
				t.Fatalf("ordinary request did not retain %q as private", value)
			}
		}
		document, reason, ok := parseJSONProvenance([]byte(`{"target":{"capacity":100}}`), false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatalf("parse report: %q", reason)
		}
		if status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document); status != AuditUnavailable || reason != AuditReasonDynamicCollision {
			t.Fatalf("ordinary limit collision = %q, %q; want unavailable dynamic collision", status, reason)
		}
	})

	t.Run("redirect does not inherit producer controls", func(t *testing.T) {
		redirect := response(http.StatusFound, "")
		redirect.Header.Set("Location", "/redirect?limit=100")
		endpoint, capsule := auditEndpoint(t, httpfixture.NewScriptedTransport(httpfixture.Step{Response: redirect}, step(http.StatusOK, "")))
		resp, err := endpoint.requestForProvenance(context.Background(), http.MethodGet, "/start?status=running&limit=100", nil, false, time.Second, requestProvenanceDiagnosticsFixedControls)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if !auditHasProbe(capsule, "100") {
			t.Fatal("redirect query control inherited fixed-public provenance")
		}
	})
}

func TestAuditRequestRejectsPriorRedirectMutationBeforeDispatch(t *testing.T) {
	redirect := response(http.StatusFound, "")
	redirect.Header.Set("Location", "/redirect/next")
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: redirect})
	config := Config{
		BaseURL:     "https://fixture.invalid",
		AdminBearer: "admin",
		DeviceToken: "device",
		Client: &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			req.URL, _ = req.URL.Parse("https://elsewhere.invalid/next")
			return nil
		}},
		RequestTimeout: time.Second,
	}
	capsule := newAuditCapsule(config)
	endpoint, err := newEndpoint(config, capsule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.request(context.Background(), http.MethodGet, "/start", nil, false); err == nil || !strings.Contains(err.Error(), "cross-origin redirect refused") || transport.Calls() != 1 {
		t.Fatalf("err = %v, calls = %d", err, transport.Calls())
	}
}

func TestAuditRequestUnavailableOrCrossOriginPreventsDispatch(t *testing.T) {
	t.Run("initial", func(t *testing.T) {
		transport := httpfixture.NewScriptedTransport(step(http.StatusOK, ""))
		endpoint, capsule := auditEndpoint(t, transport)
		capsule.markUnavailable(AuditReasonMatcherLimit)
		if _, err := endpoint.request(context.Background(), http.MethodGet, "/start", nil, false); err == nil || err.Error() != "publication audit unavailable" || transport.Calls() != 0 {
			t.Fatalf("err = %v, calls = %d", err, transport.Calls())
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var calls int
		var capsule *auditCapsule
		var ep *endpoint
		transport := httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
			calls++
			capsule.markUnavailable(AuditReasonMatcherLimit)
			redirect := response(http.StatusFound, "")
			redirect.Header.Set("Location", "/next")
			return redirect, nil
		})
		ep, capsule = auditEndpoint(t, transport)
		if _, err := ep.request(context.Background(), http.MethodGet, "/start", nil, false); err == nil || !strings.Contains(err.Error(), "publication audit unavailable") || calls != 1 {
			t.Fatalf("err = %v, calls = %d", err, calls)
		}
	})
	t.Run("cross-origin", func(t *testing.T) {
		redirect := response(http.StatusFound, "")
		redirect.Header.Set("Location", "https://elsewhere.invalid/next")
		transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: redirect})
		endpoint, _ := auditEndpoint(t, transport)
		if _, err := endpoint.request(context.Background(), http.MethodGet, "/start", nil, false); err == nil || !strings.Contains(err.Error(), "cross-origin redirect refused") || transport.Calls() != 1 {
			t.Fatalf("err = %v, calls = %d", err, transport.Calls())
		}
	})
}

func auditEndpoint(t *testing.T, transport http.RoundTripper) (*endpoint, *auditCapsule) {
	t.Helper()
	config := Config{BaseURL: "https://fixture.invalid", AdminBearer: "admin", DeviceToken: "device", Client: &http.Client{Transport: transport}, RequestTimeout: time.Second}
	capsule := newAuditCapsule(config)
	endpoint, err := newEndpoint(config, capsule)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint, capsule
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func step(status int, body string) httpfixture.Step {
	return httpfixture.Step{Response: response(status, body)}
}

func auditHasProbe(capsule *auditCapsule, want string) bool {
	probes, _, ok := capsule.snapshot()
	if !ok {
		return false
	}
	for _, probe := range probes {
		if probe.value == want {
			return true
		}
	}
	return false
}
