package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/recovery"
)

type fakePasswordRecovery struct {
	requestErr error
	preview    recovery.Record
	previewErr error
	redeemErr  error
	requests   []string
}

func (f *fakePasswordRecovery) Request(_ context.Context, username, _ string) error {
	f.requests = append(f.requests, username)
	return f.requestErr
}

func (f *fakePasswordRecovery) Preview(context.Context, string, string) (recovery.Record, error) {
	return f.preview, f.previewErr
}

func (f *fakePasswordRecovery) Redeem(context.Context, string, string, string) error {
	return f.redeemErr
}

func passwordRecoveryServer(t *testing.T, service api.PasswordRecoveryService) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		PasswordRecovery: service,
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPasswordRecoveryRequest_PublicResponseIsIdentical(t *testing.T) {
	service := &fakePasswordRecovery{}
	server := passwordRecoveryServer(t, service)
	var expected string
	for _, username := range []string{"unknown", "disabled", "imported", "eligible"} {
		response := do(t, server, http.MethodPost, "/v1/auth/password-recovery/request", "",
			`{"username":"`+username+`"}`)
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("request %q = %d %s", username, response.StatusCode, body)
		}
		if expected == "" {
			expected = string(body)
		} else if string(body) != expected {
			t.Fatalf("request %q response differs: %q != %q", username, body, expected)
		}
	}
}

func TestPasswordRecoveryBearerEndpointsAreSafeAndBodyOnly(t *testing.T) {
	const bearer = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	service := &fakePasswordRecovery{previewErr: auth.ErrInvalidPasswordRecovery}
	server := passwordRecoveryServer(t, service)
	invalid := do(t, server, http.MethodPost, "/v1/auth/password-recovery/preview", "",
		`{"grant":"`+bearer+`"}`)
	invalidBody, _ := io.ReadAll(invalid.Body)
	if invalid.StatusCode != http.StatusGone || strings.Contains(string(invalidBody), bearer) {
		t.Fatalf("invalid preview = %d %s; want safe 410 without bearer", invalid.StatusCode, invalidBody)
	}

	service.previewErr = nil
	service.preview = recovery.Record{ExpiresAt: time.Unix(1_900_000_000, 0)}
	preview := do(t, server, http.MethodPost, "/v1/auth/password-recovery/preview", "",
		`{"grant":"`+bearer+`"}`)
	if preview.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(preview.Body)
		t.Fatalf("valid preview = %d %s", preview.StatusCode, body)
	}

	service.redeemErr = auth.ErrWeakPassword
	weak := do(t, server, http.MethodPost, "/v1/auth/password-recovery/redeem", "",
		`{"grant":"`+bearer+`","password":"short"}`)
	weakBody, _ := io.ReadAll(weak.Body)
	if weak.StatusCode != http.StatusUnprocessableEntity || strings.Contains(string(weakBody), bearer) {
		t.Fatalf("weak redeem = %d %s; want safe 422 without bearer", weak.StatusCode, weakBody)
	}
	service.redeemErr = nil
	redeemed := do(t, server, http.MethodPost, "/v1/auth/password-recovery/redeem", "",
		`{"grant":"`+bearer+`","password":"replacement-password"}`)
	if redeemed.StatusCode != http.StatusNoContent || len(redeemed.Cookies()) != 0 {
		t.Fatalf("redeem = %d cookies=%v; reset must issue no session", redeemed.StatusCode, redeemed.Cookies())
	}
}

func TestPasswordRecoveryRateLimitsHaveDedicatedPublicErrors(t *testing.T) {
	service := &fakePasswordRecovery{requestErr: auth.ErrRateLimited}
	server := passwordRecoveryServer(t, service)
	request := do(t, server, http.MethodPost, "/v1/auth/password-recovery/request", "", `{"username":"Ada"}`)
	if request.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("request limit = %d, want 429", request.StatusCode)
	}
	service.requestErr = nil
	service.previewErr = auth.ErrRateLimited
	preview := do(t, server, http.MethodPost, "/v1/auth/password-recovery/preview", "", `{"grant":"invalid"}`)
	if preview.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("preview limit = %d, want 429", preview.StatusCode)
	}
}
