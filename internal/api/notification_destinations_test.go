package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/secretprotection"
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
	notifications.DestinationMetadata,
	string,
) (notifications.DestinationTestResult, error) {
	return notifications.DestinationTestResult{IntentID: "intent-test-1", Created: true}, nil
}

type notificationDestinationsHarness struct {
	*apiHarness
	repository *notifications.ProtectedDestinationRepository
}

type notificationDestinationsHarnessConfig struct {
	means            notifications.Means
	nextID           func() string
	now              func() time.Time
	withTester       bool
	webPushPublicKey string
}

func newAdminNotificationDestinationsHarness(t *testing.T, now time.Time) *notificationDestinationsHarness {
	t.Helper()
	return startNotificationDestinationsHarness(t, notificationDestinationsHarnessConfig{
		means: notifications.MeansSlack, nextID: func() string { return "destination-1" },
		now: func() time.Time { return now }, withTester: true,
	})
}

func newMemberWebPushDestinationsHarness(t *testing.T) *notificationDestinationsHarness {
	t.Helper()
	nextID := 0
	return startNotificationDestinationsHarness(t, notificationDestinationsHarnessConfig{
		means: notifications.MeansWebPush,
		nextID: func() string {
			nextID++
			return fmt.Sprintf("browser-%d", nextID)
		},
		now: time.Now, webPushPublicKey: "public-vapid-key",
	})
}

func startNotificationDestinationsHarness(
	t *testing.T,
	config notificationDestinationsHarnessConfig,
) *notificationDestinationsHarness {
	t.Helper()
	var repository *notifications.ProtectedDestinationRepository
	base := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		protection, err := secretprotection.NewManager(t.Context(), defaults.Store, secretprotection.ManagerOptions{
			InstallationKey: secretprotection.InstallationKey{0x51},
		})
		if err != nil {
			t.Fatal(err)
		}
		repository = notifications.NewProtectedDestinationRepository(defaults.Store, protection)
		manager := notifications.NewDestinationManager(repository, []notifications.DestinationValidator{
			acceptingDestinationValidator{means: config.means},
		}, config.nextID, config.now)
		if config.withTester {
			manager = manager.WithTester(acceptingDestinationTester{})
		}
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: notificationAuthorizer{}, NotificationDestinations: manager,
			WebPushPublicKey: config.webPushPublicKey,
		})
	})
	return &notificationDestinationsHarness{apiHarness: base, repository: repository}
}

func newNotificationProviderTypesHarness(t *testing.T) *apiHarness {
	t.Helper()
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{Auth: notificationAuthorizer{}})
	})
}

func TestNotificationProvidersUseOneRedactedAdminWorkflow(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	harness := newAdminNotificationDestinationsHarness(t, now)

	body := `{"type":"slack","label":"Operations Slack","events":["channel_degraded"],"enabled":true,"settings":{"webhookUrl":"https://hooks.slack.com/services/never-return-this"}}`
	member := harness.Do(http.MethodPost, "/v1/notifications/providers", memberToken, body)
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member create provider = %d", member.StatusCode)
	}
	created := harness.Do(http.MethodPost, "/v1/notifications/providers", adminToken, body)
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
	stored, err := harness.repository.ResolveNotificationDestination(t.Context(), "destination-1")
	if err != nil || stored.Credentials["webhookUrl"] != "https://hooks.slack.com/services/never-return-this" {
		t.Fatalf("stored destination = %+v, %v", stored.Summary(), err)
	}
	updateBody := `{"label":"On-call Slack","events":["channel_degraded"],"enabled":true}`
	updated := harness.Do(http.MethodPut, "/v1/notifications/providers/destination-1", adminToken, updateBody)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("admin update destination = %d: %s", updated.StatusCode, readBody(t, updated))
	}
	updatedBody := readBody(t, updated)
	if strings.Contains(updatedBody, "never-return-this") || !strings.Contains(updatedBody, `"label":"On-call Slack"`) {
		t.Fatalf("update response was not redacted: %s", updatedBody)
	}
	stored, err = harness.repository.ResolveNotificationDestination(t.Context(), "destination-1")
	if err != nil || stored.Credentials["webhookUrl"] != "https://hooks.slack.com/services/never-return-this" {
		t.Fatalf("updated stored destination = %+v, %v", stored.Summary(), err)
	}
	testDelivery := harness.Do(http.MethodPost, "/v1/notifications/providers/destination-1/test", adminToken, `{"requestId":"request-1"}`)
	if testDelivery.StatusCode != http.StatusAccepted {
		t.Fatalf("admin test destination = %d: %s", testDelivery.StatusCode, readBody(t, testDelivery))
	}
	testBody := readBody(t, testDelivery)
	if !strings.Contains(testBody, `"queued":true`) || !strings.Contains(testBody, `"intentId":"intent-test-1"`) ||
		strings.Contains(strings.ToLower(testBody), "delivered") {
		t.Fatalf("destination test did not distinguish queued handoff: %s", testBody)
	}
	adminList := harness.Do(http.MethodGet, "/v1/notifications/providers", adminToken, "")
	adminListBody := readBody(t, adminList)
	if adminList.StatusCode != http.StatusOK || !strings.Contains(adminListBody, `"health":{"queuedCount":0,"terminalFailureCount":0}`) {
		t.Fatalf("admin destination health = %d: %s", adminList.StatusCode, adminListBody)
	}

	memberList := harness.Do(http.MethodGet, "/v1/notifications/providers", memberToken, "")
	memberListBody := readBody(t, memberList)
	if memberList.StatusCode != http.StatusOK || !strings.Contains(memberListBody, `"providers":[]`) {
		t.Fatalf("member list = %d: %s", memberList.StatusCode, memberListBody)
	}
	deleted := harness.Do(http.MethodDelete, "/v1/notifications/providers/destination-1", adminToken, "")
	if deleted.StatusCode != http.StatusOK || !strings.Contains(readBody(t, deleted), `"unsubscribeCurrentBrowser":false`) {
		t.Fatalf("admin delete destination = %d", deleted.StatusCode)
	}
}

func TestNotificationProviderTypesDriveTheAdminFormWithoutSecretValues(t *testing.T) {
	harness := newNotificationProviderTypesHarness(t)

	member := harness.Do(http.MethodGet, "/v1/notifications/provider-types", memberToken, "")
	memberBody := readBody(t, member)
	if member.StatusCode != http.StatusOK || !strings.Contains(memberBody, `"type":"web_push"`) ||
		strings.Contains(memberBody, `"type":"slack"`) {
		t.Fatalf("member provider types = %d: %s", member.StatusCode, memberBody)
	}
	response := harness.Do(http.MethodGet, "/v1/notifications/provider-types", adminToken, "")
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

func TestNotificationProvidersBindBrowserPushToTheAuthenticatedMember(t *testing.T) {
	harness := newMemberWebPushDestinationsHarness(t)

	body := `{"type":"web_push","label":"Living room browser","events":["proposal_approved"],"enabled":true,"settings":{"endpoint":"https://push.example.test/secret","p256dh":"browser-public","auth":"browser-auth"}}`
	created := harness.Do(http.MethodPost, "/v1/notifications/providers", memberToken, body)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("member Web Push create = %d: %s", created.StatusCode, readBody(t, created))
	}
	stored, err := harness.repository.ResolveNotificationDestination(t.Context(), "browser-1")
	if err != nil || stored.Scope != notifications.ScopePerson || stored.OwnerID != "member-1" ||
		stored.Audience != notifications.RecipientPerson || stored.Credentials["endpoint"] == "" {
		t.Fatalf("stored Web Push destination = %+v, %v", stored.Summary(), err)
	}
	updated := harness.Do(http.MethodPut, "/v1/notifications/providers/browser-1", memberToken,
		`{"label":"Laptop browser","events":["proposal_approved"],"enabled":true}`)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("member Web Push update = %d: %s", updated.StatusCode, readBody(t, updated))
	}
	adminList := harness.Do(http.MethodGet, "/v1/notifications/providers", adminToken, "")
	if body := readBody(t, adminList); strings.Contains(body, "Living room browser") {
		t.Fatalf("another administrator saw a member subscription: %s", body)
	}
	memberList := harness.Do(http.MethodGet, "/v1/notifications/providers", memberToken, "")
	if body := readBody(t, memberList); !strings.Contains(body, "Laptop browser") ||
		strings.Contains(body, "push.example.test") || strings.Contains(body, "browser-auth") ||
		strings.Contains(body, "subscriptionFingerprint") {
		t.Fatalf("member Web Push list was not scoped/redacted: %s", body)
	}

	repeated := harness.Do(http.MethodPost, "/v1/notifications/providers", memberToken, body)
	repeatedBody := readBody(t, repeated)
	if repeated.StatusCode != http.StatusCreated || !strings.Contains(repeatedBody, `"id":"browser-1"`) {
		t.Fatalf("repeat Web Push create = %d: %s", repeated.StatusCode, repeatedBody)
	}
	records, err := harness.Store.ListNotificationDestinationRecords(t.Context())
	if err != nil || len(records) != 1 {
		t.Fatalf("repeat Web Push destination count = %d, %v", len(records), err)
	}

	otherBody := `{"type":"web_push","label":"Tablet","events":["proposal_approved"],"enabled":true,"settings":{"endpoint":"https://push.example.test/other-secret","p256dh":"browser-public","auth":"browser-auth"}}`
	other := harness.Do(http.MethodPost, "/v1/notifications/providers", memberToken, otherBody)
	if other.StatusCode != http.StatusCreated {
		t.Fatalf("other Web Push create = %d: %s", other.StatusCode, readBody(t, other))
	}
	deleteOther := harness.Do(http.MethodDelete, "/v1/notifications/providers/browser-3", memberToken,
		`{"currentBrowserEndpoint":"https://push.example.test/secret"}`)
	if deleteOther.StatusCode != http.StatusOK ||
		!strings.Contains(readBody(t, deleteOther), `"unsubscribeCurrentBrowser":false`) {
		t.Fatalf("delete other browser = %d", deleteOther.StatusCode)
	}
	deleteCurrent := harness.Do(http.MethodDelete, "/v1/notifications/providers/browser-1", memberToken,
		`{"currentBrowserEndpoint":"https://push.example.test/secret"}`)
	if deleteCurrent.StatusCode != http.StatusOK ||
		!strings.Contains(readBody(t, deleteCurrent), `"unsubscribeCurrentBrowser":true`) {
		t.Fatalf("delete current browser = %d", deleteCurrent.StatusCode)
	}
}
