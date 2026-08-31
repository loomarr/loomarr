package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

func TestDiscoveryFeedbackExistingAndDetachedChannelRemainsIndependentOfHousehold(t *testing.T) {
	srv, st := newServer(t)
	ctx := context.Background()
	channel := store.Channel{Channel: schedule.Channel{ID: "feedback-channel", Name: "Feedback Channel",
		Number: 43, Strategy: schedule.Sequential, Status: schedule.StatusLive},
		ReconcileDeadline: time.Now().Add(time.Hour)}
	_, err := st.SaveChannel(ctx, channel)
	if err != nil {
		t.Fatal(err)
	}
	channelEvent := `{"scope":"channel","scopeId":"feedback-channel","targetKey":"movie:tmdb:603","action":"never"}`
	resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", adminToken, channelEvent)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("existing-channel feedback = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp = do(t, srv, http.MethodDelete, "/v1/channels/feedback-channel", adminToken, "")
	if resp.StatusCode != http.StatusNoContent {
		_ = resp.Body.Close()
		t.Fatalf("soft detach = %d, want 204", resp.StatusCode)
	}
	_ = resp.Body.Close()
	detachedEvent := `{"scope":"channel","scopeId":"feedback-channel","targetKey":"movie:tmdb:603","action":"surprise"}`
	resp = do(t, srv, http.MethodPost, "/v1/discovery/feedback", adminToken, detachedEvent)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("detached-channel feedback = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()
	householdEvent := `{"scope":"household","targetKey":"movie:tmdb:603","action":"keep"}`
	resp = do(t, srv, http.MethodPost, "/v1/discovery/feedback", adminToken, householdEvent)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("household feedback = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	for _, query := range []struct {
		path   string
		action string
	}{
		{path: "/v1/discovery/feedback?scope=channel&scopeId=feedback-channel", action: "surprise"},
		{path: "/v1/discovery/feedback?scope=household", action: "keep"},
	} {
		resp = do(t, srv, http.MethodGet, query.path, memberToken, "")
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("list %s = %d, want 200", query.path, resp.StatusCode)
		}
		var got []struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			_ = resp.Body.Close()
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if len(got) != 1 || got[0].Action != query.action {
			t.Fatalf("list %s = %+v, want effective %s", query.path, got, query.action)
		}
	}
}

func TestDiscoveryFeedbackMissingChannelIsBounded404AfterAuthorization(t *testing.T) {
	srv, _ := newServer(t)
	requests := []struct {
		path string
		body string
	}{
		{path: "/v1/discovery/feedback", body: `{"scope":"channel","scopeId":"missing-channel","targetKey":"movie:tmdb:603","action":"never"}`},
		{path: "/v1/discovery/feedback/clear", body: `{"scope":"channel","scopeId":"missing-channel","targetKey":"movie:tmdb:603"}`},
	}
	for _, request := range requests {
		resp := do(t, srv, http.MethodPost, request.path, memberToken, request.body)
		if resp.StatusCode != http.StatusForbidden {
			_ = resp.Body.Close()
			t.Fatalf("member %s = %d, want 403 before channel existence disclosure", request.path, resp.StatusCode)
		}
		_ = resp.Body.Close()

		resp = do(t, srv, http.MethodPost, request.path, adminToken, request.body)
		if resp.StatusCode != http.StatusNotFound {
			_ = resp.Body.Close()
			t.Fatalf("admin %s = %d, want 404", request.path, resp.StatusCode)
		}
		problem := decodeProblem(t, resp.Body)
		_ = resp.Body.Close()
		if problem.Title != "Channel not found" || problem.Detail != "That channel doesn't exist — it may have been removed." {
			t.Fatalf("admin %s problem = %+v, want bounded Channel-not-found response", request.path, problem)
		}
	}

	resp := do(t, srv, http.MethodGet,
		"/v1/discovery/feedback?scope=channel&scopeId=missing-channel", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list missing-channel feedback = %d, want 200", resp.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing-channel feedback = %+v, want no event or clear tombstone", got)
	}
}

func TestDiscoveryFeedbackAuthorizationAndRoundTrip(t *testing.T) {
	srv, _ := newServer(t)
	body := `{"scope":"household","targetKey":"movie:tmdb:603","action":"keep","reason":"family favorite"}`

	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", "", body); resp.StatusCode != http.StatusUnauthorized {
		_ = resp.Body.Close()
		t.Fatalf("anonymous write = %d, want 401", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", memberToken, body); resp.StatusCode != http.StatusForbidden {
		_ = resp.Body.Close()
		t.Fatalf("member write = %d, want 403", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", adminToken, body); resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("admin write = %d, want 200", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodGet, "/v1/discovery/feedback?scope=household", memberToken, ""); resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("member read = %d, want 200", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
}
