package notifications_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	header   http.Header
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
		StatusCode: d.status, Header: d.header.Clone(), Body: io.NopCloser(strings.NewReader(d.response)),
	}, nil
}

func TestWebhookSignatureCoversTheExactVersionedPayload(t *testing.T) {
	t.Parallel()
	doer := &providerDoer{status: http.StatusNoContent}
	adapter := httpAdapter(t, notifications.MeansWebhook, doer)
	destination := providerDestination(notifications.MeansWebhook, nil, map[string]string{
		"url": "https://example.test/hooks/loomarr", "hmacSecret": "hmac-value",
	})
	if result := adapter.Deliver(t.Context(), providerDelivery(&destination)); result.Status != notifications.StatusDelivered {
		t.Fatalf("result = %+v", result)
	}
	mac := hmac.New(sha256.New, []byte("hmac-value"))
	_, _ = mac.Write([]byte(doer.request.body))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := doer.request.header.Get("X-Loomarr-Signature"); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
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

func TestEveryHTTPProviderUsesCommonRateLimitAndTransportClassification(t *testing.T) {
	t.Parallel()
	providers := []struct {
		means         notifications.Means
		configuration map[string]string
		credentials   map[string]string
	}{
		{notifications.MeansWebhook, nil, map[string]string{"url": "https://example.test/hook"}},
		{notifications.MeansDiscord, nil, map[string]string{"webhookUrl": "https://discord.com/api/webhooks/1/secret"}},
		{notifications.MeansNtfy, map[string]string{"baseUrl": "https://ntfy.example.test"}, map[string]string{"topic": "private"}},
		{notifications.MeansGotify, map[string]string{"serverUrl": "https://gotify.example.test"}, map[string]string{"applicationToken": "token"}},
		{notifications.MeansApprise, map[string]string{"baseUrl": "https://apprise.example.test"}, map[string]string{"configurationKey": "home"}},
		{notifications.MeansPushover, nil, map[string]string{"applicationToken": "app", "recipientKey": "user"}},
		{notifications.MeansTelegram, nil, map[string]string{"botToken": "1:token", "chatId": "123"}},
		{notifications.MeansMattermost, nil, map[string]string{"webhookUrl": "https://mattermost.example.test/hooks/secret"}},
		{notifications.MeansMatrix, map[string]string{"homeserverUrl": "https://matrix.example.test"}, map[string]string{"roomId": "!room:example.test", "accessToken": "token"}},
		{notifications.MeansSlack, nil, map[string]string{"webhookUrl": "https://hooks.slack.com/services/T/B/secret"}},
	}
	for _, provider := range providers {
		provider := provider
		t.Run(string(provider.means), func(t *testing.T) {
			t.Parallel()
			destination := providerDestination(provider.means, provider.configuration, provider.credentials)
			doer := &providerDoer{status: http.StatusTooManyRequests, response: "rate limited"}
			result := httpAdapter(t, provider.means, doer).Deliver(t.Context(), providerDelivery(&destination))
			if result.FailureClass != notifications.FailureTransientPreAcceptance {
				t.Fatalf("rate limit result = %+v", result)
			}
			doer.err = errors.New("transport unavailable")
			result = httpAdapter(t, provider.means, doer).Deliver(t.Context(), providerDelivery(&destination))
			if result.FailureClass != notifications.FailureTransientPreAcceptance ||
				result.OutcomeCode != notifications.OutcomeTransportUnavailable {
				t.Fatalf("transport result = %+v", result)
			}
		})
	}
}

func TestTelegramDirectGroupAndTopicRequests(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, chatID, threadID, want string
	}{
		{"direct", "12345", "", `"chat_id":"12345"`},
		{"group", "-12345", "", `"chat_id":"-12345"`},
		{"topic", "-10012345", "77", `"message_thread_id":77`},
	} {
		t.Run(test.name, func(t *testing.T) {
			doer := &providerDoer{status: http.StatusOK, response: `{"ok":true,"result":{"message_id":1}}`}
			adapter := httpAdapter(t, notifications.MeansTelegram, doer)
			destination := providerDestination(notifications.MeansTelegram, nil, map[string]string{
				"botToken": "1:token", "chatId": test.chatID, "threadId": test.threadID,
			})
			if result := adapter.Deliver(t.Context(), providerDelivery(&destination)); result.Status != notifications.StatusDelivered {
				t.Fatalf("result = %+v", result)
			}
			if !strings.Contains(doer.request.body, test.want) {
				t.Fatalf("body = %s, want %s", doer.request.body, test.want)
			}
		})
	}
}

func TestHTTPProviderAdapterCarriesBoundedRateLimitHint(t *testing.T) {
	t.Parallel()
	doer := &providerDoer{
		status: http.StatusTooManyRequests, response: "slow down",
		header: http.Header{"Retry-After": []string{"999999"}},
	}
	adapter := httpAdapter(t, notifications.MeansDiscord, doer)
	destination := providerDestination(notifications.MeansDiscord, nil, map[string]string{
		"webhookUrl": "https://discord.com/api/webhooks/123/secret",
	})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailureTransientPreAcceptance || result.RetryAfter != 2*time.Hour {
		t.Fatalf("result = %+v", result)
	}
}

func TestApprisePartialFanoutFailureIsAmbiguous(t *testing.T) {
	t.Parallel()
	adapter := httpAdapter(t, notifications.MeansApprise, &providerDoer{
		status: http.StatusInternalServerError, response: `{"error":"One or more notification could not be sent"}`,
	})
	destination := providerDestination(notifications.MeansApprise, map[string]string{
		"baseUrl": "https://apprise.example.test",
	}, map[string]string{"configurationKey": "household"})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailureAmbiguous || result.OutcomeCode != notifications.OutcomeAcceptanceAmbiguous {
		t.Fatalf("result = %+v", result)
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
