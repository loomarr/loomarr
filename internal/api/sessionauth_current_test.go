package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionAuthorizerCurrentObservesAPITokenRotation(t *testing.T) {
	token := "first-token"
	a := NewSessionAuthorizerCurrent(nil, func(context.Context) (string, error) {
		return token, nil
	})
	request := func(value string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
		r.Header.Set("Authorization", "Bearer "+value)
		return r
	}
	if got := a.Authorize(request("first-token")); got != RoleAdmin {
		t.Fatalf("initial bearer role = %q, want admin", got)
	}
	token = "rotated-token"
	if got := a.Authorize(request("first-token")); got != RoleAnonymous {
		t.Fatalf("old bearer after rotation role = %q, want anonymous", got)
	}
	if got := a.Authorize(request("rotated-token")); got != RoleAdmin {
		t.Fatalf("rotated bearer role = %q, want admin", got)
	}
}

func TestSessionAuthorizerCurrentFailsBearerClosedOnReadError(t *testing.T) {
	a := NewSessionAuthorizerCurrent(nil, func(context.Context) (string, error) {
		return "would-match", errors.New("database unavailable")
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	r.Header.Set("Authorization", "Bearer would-match")
	if got := a.Authorize(r); got != RoleAnonymous {
		t.Fatalf("bearer role with unavailable durable token = %q, want anonymous", got)
	}
}
