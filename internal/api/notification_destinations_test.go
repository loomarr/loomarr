package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/store"
)

type notificationAuthorizer struct{}

func (notificationAuthorizer) Authorize(r *http.Request) api.Role {
	role, _ := notificationAuthorizer{}.AuthorizeUser(r)
	return role
}

func (notificationAuthorizer) AuthorizeUser(r *http.Request) (api.Role, *store.User) {
	switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
	case adminToken:
		user := store.User{ID: "admin-1", Name: "Admin", Role: store.RoleAdmin}
		return api.RoleAdmin, &user
	case memberToken:
		user := store.User{ID: "member-1", Name: "Member", Role: store.RoleMember}
		return api.RoleMember, &user
	default:
		return api.RoleAnonymous, nil
	}
}

type acceptingDestinationValidator struct{ means notifications.Means }

func (v acceptingDestinationValidator) Means() notifications.Means { return v.means }
func (acceptingDestinationValidator) ValidateDestination(map[string]string, map[string]string) error {
	return nil
}

type acceptingDestinationTester struct{}

func (acceptingDestinationTester) PublishDestinationTest(
	context.Context,
	notifications.Destination,
	string,
) (notifications.DestinationTestResult, error) {
	return notifications.DestinationTestResult{IntentID: "intent-test-1", Created: true}, nil
}

func TestNotificationProvidersUseOneRedactedAdminWorkflow(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/notification-destinations.db")
	t.Cleanup(func() { _ = st.Close() })
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(
		st, []notifications.DestinationValidator{acceptingDestinationValidator{means: notifications.MeansSlack}},
		func() string { return "destination-1" }, func() time.Time { return now },
	).WithTester(acceptingDestinationTester{})
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: notificationAuthorizer{}, NotificationDestinations: manager,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	body := `{"type":"slack","label":"Operations Slack","events":["channel_degraded"],"enabled":true,"settings":{"webhookUrl":"https://hooks.slack.com/services/never-return-this"}}`
	member := do(t, srv, http.MethodPost, "/v1/notifications/providers", memberToken, body)
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member create provider = %d", member.StatusCode)
	}
	created := do(t, srv, http.MethodPost, "/v1/notifications/providers", adminToken, body)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("admin create provider = %d: %s", created.StatusCode, readBody(t, created))
	}
	createdBody := readBody(t, created)
	if strings.Contains(createdBody, "never-return-this") ||
		!strings.Contains(createdBody, `"key":"webhookUrl"`) ||
		!strings.Contains(createdBody, `"secretConfigured":true`) ||
		strings.Contains(createdBody, `"scope"`) || strings.Contains(createdBody, `"audience"`) ||
		strings.Contains(createdBody, `"configuration"`) || strings.Contains(createdBody, `"credentials"`) {
		t.Fatalf("create response was not redacted: %s", createdBody)
	}
	stored, err := st.GetNotificationDestination(t.Context(), "destination-1")
	if err != nil || stored.Credentials["webhookUrl"] != "https://hooks.slack.com/services/never-return-this" {
		t.Fatalf("stored destination = %+v, %v", stored.Summary(), err)
	}
	updateBody := `{"label":"On-call Slack","events":["channel_degraded"],"enabled":true}`
	updated := do(t, srv, http.MethodPut, "/v1/notifications/providers/destination-1", adminToken, updateBody)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("admin update destination = %d: %s", updated.StatusCode, readBody(t, updated))
	}
	updatedBody := readBody(t, updated)
	if strings.Contains(updatedBody, "never-return-this") || !strings.Contains(updatedBody, `"label":"On-call Slack"`) {
		t.Fatalf("update response was not redacted: %s", updatedBody)
	}
	stored, err = st.GetNotificationDestination(t.Context(), "destination-1")
	if err != nil || stored.Credentials["webhookUrl"] != "https://hooks.slack.com/services/never-return-this" {
		t.Fatalf("updated stored destination = %+v, %v", stored.Summary(), err)
	}
	testDelivery := do(t, srv, http.MethodPost, "/v1/notifications/providers/destination-1/test", adminToken, `{"requestId":"request-1"}`)
	if testDelivery.StatusCode != http.StatusAccepted {
		t.Fatalf("admin test destination = %d: %s", testDelivery.StatusCode, readBody(t, testDelivery))
	}
	testBody := readBody(t, testDelivery)
	if !strings.Contains(testBody, `"queued":true`) || !strings.Contains(testBody, `"intentId":"intent-test-1"`) ||
		strings.Contains(strings.ToLower(testBody), "delivered") {
		t.Fatalf("destination test did not distinguish queued handoff: %s", testBody)
	}
	adminList := do(t, srv, http.MethodGet, "/v1/notifications/providers", adminToken, "")
	adminListBody := readBody(t, adminList)
	if adminList.StatusCode != http.StatusOK || !strings.Contains(adminListBody, `"health":{"queuedCount":0,"terminalFailureCount":0}`) {
		t.Fatalf("admin destination health = %d: %s", adminList.StatusCode, adminListBody)
	}

	memberList := do(t, srv, http.MethodGet, "/v1/notifications/providers", memberToken, "")
	if memberList.StatusCode != http.StatusForbidden {
		t.Fatalf("member list = %d, want 403", memberList.StatusCode)
	}
	deleted := do(t, srv, http.MethodDelete, "/v1/notifications/providers/destination-1", adminToken, "")
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("admin delete destination = %d", deleted.StatusCode)
	}
}

func TestNotificationProviderTypesDriveTheAdminFormWithoutSecretValues(t *testing.T) {
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{Auth: notificationAuthorizer{}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	member := do(t, srv, http.MethodGet, "/v1/notifications/provider-types", memberToken, "")
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member provider types = %d, want 403", member.StatusCode)
	}
	response := do(t, srv, http.MethodGet, "/v1/notifications/provider-types", adminToken, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin provider types = %d: %s", response.StatusCode, readBody(t, response))
	}
	var body struct {
		Providers []struct {
			Type   notifications.Means `json:"type"`
			Name   string              `json:"name"`
			Fields []struct {
				Key       string `json:"key"`
				Sensitive bool   `json:"sensitive"`
				Value     string `json:"value"`
			} `json:"fields"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Providers) != 13 || body.Providers[0].Type != notifications.MeansEmail || body.Providers[0].Name != "SMTP" {
		t.Fatalf("provider definitions = %+v", body.Providers)
	}
	var slackWebhookSensitive bool
	for _, provider := range body.Providers {
		for _, field := range provider.Fields {
			if field.Value != "" {
				t.Fatalf("provider metadata leaked a field value: %s.%s", provider.Type, field.Key)
			}
			if provider.Type == notifications.MeansSlack && field.Key == "webhookUrl" {
				slackWebhookSensitive = field.Sensitive
			}
		}
	}
	if !slackWebhookSensitive {
		t.Fatal("Slack webhook URL is not identified as a sensitive server-owned field")
	}
}
