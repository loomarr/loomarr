package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/testkit"
)

func invitationServer(t *testing.T, publicURL string) (*httptest.Server, string) {
	return invitationServerWithDelivery(t, publicURL, nil)
}

func invitationServerWithDelivery(
	t *testing.T,
	publicURL string,
	delivery api.InvitationDeliveryService,
) (*httptest.Server, string) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/invitations.db")
	t.Cleanup(func() { _ = st.Close() })
	now := time.Unix(1_900_000_000, 0).UTC()
	bearer := strings.Repeat("a", 64)
	service := invitation.NewService(
		st,
		testkit.LibraryAccountResolver{Accounts: map[string]invitation.LibraryAccount{
			"library-42": {ID: "library-42", Name: "Grace Hopper"},
		}},
		func() string { return "invitation-1" },
		func() (string, error) { return bearer, nil },
		func() time.Time { return now },
	)
	handler := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler),
		Invitations: service, InvitationDelivery: delivery,
		AccessPublicURL: func() string { return publicURL },
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, bearer
}

type fakeInvitationDelivery struct {
	invitationID string
	requestID    string
	result       notifications.DeliverySummary
}

func (f *fakeInvitationDelivery) SendEmail(_ context.Context, invitationID, requestID string) (notifications.DeliverySummary, error) {
	f.invitationID = invitationID
	f.requestID = requestID
	return f.result, nil
}

func (f *fakeInvitationDelivery) LatestEmail(context.Context, string) (notifications.DeliverySummary, error) {
	return f.result, nil
}

func TestInvitations_AdminLifecycleAndBearerMinimization(t *testing.T) {
	srv, bearer := invitationServer(t, "https://loomarr.example.test/")

	member := do(t, srv, http.MethodPost, "/v1/invitations", memberToken,
		`{"kind":"local","username":"Ada"}`)
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member create = %d, want 403", member.StatusCode)
	}
	memberList := do(t, srv, http.MethodGet, "/v1/invitations", memberToken, "")
	if memberList.StatusCode != http.StatusForbidden {
		t.Fatalf("member list = %d, want 403", memberList.StatusCode)
	}
	anonymousGet := do(t, srv, http.MethodGet, "/v1/invitations/unknown", "", "")
	if anonymousGet.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous get = %d, want 401", anonymousGet.StatusCode)
	}
	created := do(t, srv, http.MethodPost, "/v1/invitations", adminToken,
		`{"kind":"local","username":"Ada","contactEmail":"ada@example.com"}`)
	if created.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(created.Body)
		t.Fatalf("create = %d %s, want 201", created.StatusCode, body)
	}
	var invitationBody struct {
		ID             string `json:"id"`
		Role           string `json:"role"`
		Status         string `json:"status"`
		ContactAddress *struct {
			Email string `json:"email"`
		} `json:"contactAddress"`
	}
	if err := json.NewDecoder(created.Body).Decode(&invitationBody); err != nil {
		t.Fatal(err)
	}
	if invitationBody.ID != "invitation-1" || invitationBody.Role != "member" ||
		invitationBody.Status != "pending" || invitationBody.ContactAddress == nil ||
		invitationBody.ContactAddress.Email != "ada@example.com" {
		t.Fatalf("created invitation = %+v", invitationBody)
	}

	shared := do(t, srv, http.MethodPost, "/v1/invitations/invitation-1/grant", adminToken,
		`{"conveyance":"qr"}`)
	if shared.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(shared.Body)
		t.Fatalf("share = %d %s", shared.StatusCode, body)
	}
	var grant struct {
		URL       string `json:"url"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if err := json.NewDecoder(shared.Body).Decode(&grant); err != nil {
		t.Fatal(err)
	}
	if grant.URL != "https://loomarr.example.test/join#grant="+bearer || grant.ExpiresAt == 0 {
		t.Fatalf("grant = %+v", grant)
	}

	listed := do(t, srv, http.MethodGet, "/v1/invitations", adminToken, "")
	contents, _ := io.ReadAll(listed.Body)
	if listed.StatusCode != http.StatusOK || strings.Contains(string(contents), bearer) ||
		strings.Contains(string(contents), "#grant=") {
		t.Fatalf("list = %d %s; bearer leaked", listed.StatusCode, contents)
	}
	missing := do(t, srv, http.MethodGet, "/v1/invitations/unknown", adminToken, "")
	missingBody, _ := io.ReadAll(missing.Body)
	if missing.StatusCode != http.StatusNotFound || strings.Contains(string(missingBody), bearer) {
		t.Fatalf("missing = %d %s", missing.StatusCode, missingBody)
	}

	revoked := do(t, srv, http.MethodDelete, "/v1/invitations/invitation-1", adminToken, "")
	if revoked.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", revoked.StatusCode)
	}
}

func TestInvitationGrantRequiresConfiguredRecipientURL(t *testing.T) {
	srv, bearer := invitationServer(t, "")
	created := do(t, srv, http.MethodPost, "/v1/invitations", adminToken,
		`{"kind":"local","username":"Ada"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create without access URL = %d, want 201", created.StatusCode)
	}
	shared := do(t, srv, http.MethodPost, "/v1/invitations/invitation-1/grant", adminToken,
		`{"conveyance":"copy"}`)
	contents, _ := io.ReadAll(shared.Body)
	if shared.StatusCode != http.StatusUnprocessableEntity || strings.Contains(string(contents), bearer) {
		t.Fatalf("share without access URL = %d %s", shared.StatusCode, contents)
	}
}

func TestInvitationEmailIsAnExplicitIdempotentAdminAction(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	delivery := &fakeInvitationDelivery{result: notifications.DeliverySummary{
		Status: notifications.StatusQueued, AttemptNumber: 1, UpdatedAt: now,
	}}
	srv, _ := invitationServerWithDelivery(t, "https://loomarr.example.test", delivery)
	created := do(t, srv, http.MethodPost, "/v1/invitations", adminToken,
		`{"kind":"local","username":"Ada","contactEmail":"ada@example.com"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", created.StatusCode)
	}

	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"anonymous": {token: "", want: http.StatusUnauthorized},
		"member":    {token: memberToken, want: http.StatusForbidden},
	} {
		response := do(t, srv, http.MethodPost, "/v1/invitations/invitation-1/email", tc.token,
			`{"requestId":"operator-action-42"}`)
		if response.StatusCode != tc.want {
			t.Fatalf("%s send = %d, want %d", name, response.StatusCode, tc.want)
		}
	}

	response := do(t, srv, http.MethodPost, "/v1/invitations/invitation-1/email", adminToken,
		`{"requestId":"operator-action-42"}`)
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("admin send = %d %s", response.StatusCode, body)
	}
	var body struct {
		Status        string `json:"status"`
		AttemptNumber int    `json:"attemptNumber"`
		UpdatedAt     int64  `json:"updatedAt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "queued" || body.AttemptNumber != 1 || body.UpdatedAt != now.UnixMilli() ||
		delivery.invitationID != "invitation-1" || delivery.requestID != "operator-action-42" {
		t.Fatalf("delivery response = %+v, call = %q %q", body, delivery.invitationID, delivery.requestID)
	}
}
