package notifications_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

type capturedProviderRequest struct {
	method string
	url    string
	header http.Header
	body   string
}

type providerDoer struct {
	status   int
	response string
	request  capturedProviderRequest
	err      error
}

func (d *providerDoer) Do(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	d.request = capturedProviderRequest{
		method: request.Method, url: request.URL.String(), header: request.Header.Clone(), body: string(body),
	}
	if d.err != nil {
		return nil, d.err
	}
	return &http.Response{
		StatusCode: d.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(d.response)),
	}, nil
}

func TestHTTPProviderAdaptersBuildDocumentedRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		means         notifications.Means
		configuration map[string]string
		credentials   map[string]string
		response      string
		wantMethod    string
		wantURL       string
		wantHeader    string
		wantBody      string
	}{
		{notifications.MeansWebhook, nil, map[string]string{
			"url": "https://example.test/hooks/loomarr", "bearerToken": "bearer-value", "hmacSecret": "hmac-value",
		}, `{}`, http.MethodPost, "/hooks/loomarr", "X-Loomarr-Signature", `"eventId":"intent-provider"`},
		{notifications.MeansDiscord, nil, map[string]string{
			"webhookUrl": "https://discord.com/api/webhooks/123/secret",
		}, `{}`, http.MethodPost, "/api/webhooks/123/secret", "", `"allowed_mentions":{"parse":[]}`},
		{notifications.MeansNtfy, map[string]string{
			"baseUrl": "https://ntfy.example.test/prefix", "username": "loomarr",
		}, map[string]string{"topic": "private topic", "password": "token"}, `{}`, http.MethodPost, "/prefix/private%20topic", "Authorization", "Channel needs attention"},
		{notifications.MeansGotify, map[string]string{
			"serverUrl": "https://gotify.example.test/base",
		}, map[string]string{"applicationToken": "gotify-token"}, `{}`, http.MethodPost, "/base/message", "X-Gotify-Key", `"priority":5`},
		{notifications.MeansApprise, map[string]string{
			"baseUrl": "https://apprise.example.test/api",
		}, map[string]string{"configurationKey": "household", "token": "api-token"}, `{}`, http.MethodPost, "/api/notify/household", "Authorization", `"type":"warning"`},
		{notifications.MeansPushover, nil, map[string]string{
			"applicationToken": "app-token", "recipientKey": "user-key", "device": "phone",
		}, `{"status":1,"request":"push-request"}`, http.MethodPost, "/1/messages.json", "", "priority=1"},
		{notifications.MeansTelegram, nil, map[string]string{
			"botToken": "123:token", "chatId": "-100123", "threadId": "7",
		}, `{"ok":true,"result":{"message_id":42}}`, http.MethodPost, "/bot123:token/sendMessage", "", `"message_thread_id":7`},
		{notifications.MeansMattermost, nil, map[string]string{
			"webhookUrl": "https://mattermost.example.test/hooks/secret",
		}, `ok`, http.MethodPost, "/hooks/secret", "", `"text":"Loomarr:`},
		{notifications.MeansMatrix, map[string]string{
			"homeserverUrl": "https://matrix.example.test/prefix",
		}, map[string]string{"roomId": "!room:example.test", "accessToken": "matrix-token"},
			`{"event_id":"$event"}`, http.MethodPut, "/prefix/_matrix/client/v3/rooms/!room:example.test/send/m.room.message/intent-provider", "Authorization", `"msgtype":"m.text"`},
		{notifications.MeansSlack, nil, map[string]string{
			"webhookUrl": "https://hooks.slack.com/services/T/B/secret",
		}, `ok`, http.MethodPost, "/services/T/B/secret", "", `"type":"plain_text"`},
	}
	for _, tt := range tests {
		t.Run(string(tt.means), func(t *testing.T) {
			t.Parallel()
			doer := &providerDoer{status: http.StatusOK, response: tt.response}
			adapter := httpAdapter(t, tt.means, doer)
			destination := providerDestination(tt.means, tt.configuration, tt.credentials)
			result := adapter.Deliver(t.Context(), providerDelivery(&destination))
			if result.Status != notifications.StatusDelivered {
				t.Fatalf("result = %+v", result)
			}
			if doer.request.method != tt.wantMethod || !strings.Contains(doer.request.url, tt.wantURL) ||
				(tt.wantHeader != "" && doer.request.header.Get(tt.wantHeader) == "") ||
				!strings.Contains(doer.request.body, tt.wantBody) {
				t.Fatalf("request = %+v", doer.request)
			}
			if strings.Contains(doer.request.body, "@here") || strings.Contains(doer.request.body, "@everyone") {
				t.Fatalf("request allowed a broad mention: %s", doer.request.body)
			}
		})
	}
}

func TestHTTPProviderAdapterClassifiesBoundedOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		want   notifications.FailureClass
		code   notifications.OutcomeCode
	}{
		{http.StatusTooManyRequests, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
		{http.StatusServiceUnavailable, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
		{http.StatusUnauthorized, notifications.FailurePermanent, notifications.OutcomeRecipientRejected},
		{http.StatusGone, notifications.FailurePermanent, notifications.OutcomeDestinationUnavailable},
		{http.StatusBadRequest, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	}
	for _, tt := range tests {
		doer := &providerDoer{status: tt.status, response: "provider details must not escape"}
		adapter := httpAdapter(t, notifications.MeansSlack, doer)
		destination := providerDestination(notifications.MeansSlack, nil, map[string]string{
			"webhookUrl": "https://hooks.slack.com/services/T/B/secret",
		})
		result := adapter.Deliver(t.Context(), providerDelivery(&destination))
		if result.FailureClass != tt.want || result.OutcomeCode != tt.code {
			t.Errorf("status %d result = %+v", tt.status, result)
		}
	}
}

func TestHTTPProviderValidationRejectsLookalikeAndIncompleteDestinations(t *testing.T) {
	t.Parallel()
	adapter := httpAdapter(t, notifications.MeansSlack, &providerDoer{})
	validator := adapter.(notifications.DestinationValidator)
	if err := validator.ValidateDestination(nil, map[string]string{
		"webhookUrl": "https://hooks.slack.com.attacker.test/services/T/B/secret",
	}); err == nil {
		t.Fatal("Slack accepted a lookalike webhook host")
	}
	if err := validator.ValidateDestination(nil, nil); err == nil {
		t.Fatal("Slack accepted a missing webhook URL")
	}
}

func httpAdapter(t *testing.T, means notifications.Means, doer notifications.HTTPDoer) notifications.Adapter {
	t.Helper()
	adapters, _ := notifications.NewHTTPProviderAdapters(doer, func() string { return "https://loomarr.example.test" })
	for _, adapter := range adapters {
		if adapter.Means() == means {
			return adapter
		}
	}
	t.Fatalf("adapter %q missing", means)
	return nil
}

func providerDestination(
	means notifications.Means,
	configuration, credentials map[string]string,
) notifications.Destination {
	now := time.Unix(1_900_000_000, 0).UTC()
	return notifications.Destination{
		ID: "provider-1", Means: means, Label: "Provider",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
		Configuration: configuration, Credentials: credentials, CreatedAt: now, UpdatedAt: now,
	}
}

func providerDelivery(destination *notifications.Destination) notifications.Delivery {
	now := time.Unix(1_900_000_000, 0).UTC()
	return notifications.Delivery{
		Intent: notifications.Intent{
			ID: "intent-provider", Topic: notifications.TopicChannelDegraded,
			RecipientKind: notifications.RecipientOperators, RecipientID: "operators",
			ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-1",
			Policy: notifications.PolicyConfigurable,
			Template: notifications.TemplateData{
				SubjectName: "Channel @everyone", Summary: "Channel needs attention @here",
			},
			IdempotencyKey: "channel-1:degraded:operators", CreatedAt: now,
		},
		Attempt: notifications.Attempt{ID: "attempt-1"}, Destination: destination,
	}
}

var _ notifications.HTTPDoer = (*providerDoer)(nil)
var _ = context.Background
