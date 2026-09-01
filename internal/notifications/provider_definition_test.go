package notifications_test

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/notifications"
)

func TestProviderDefinitionsCoverEveryDeliveryMeans(t *testing.T) {
	want := []notifications.Means{
		notifications.MeansEmail,
		notifications.MeansWebhook,
		notifications.MeansDiscord,
		notifications.MeansNtfy,
		notifications.MeansGotify,
		notifications.MeansApprise,
		notifications.MeansPushover,
		notifications.MeansTelegram,
		notifications.MeansMattermost,
		notifications.MeansMatrix,
		notifications.MeansWebPush,
		notifications.MeansMQTT,
		notifications.MeansSlack,
	}
	definitions := notifications.ProviderDefinitions()
	got := make([]notifications.Means, 0, len(definitions))
	for _, definition := range definitions {
		got = append(got, definition.Means)
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %q is invalid: %v", definition.Means, err)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider means = %v, want %v", got, want)
	}
}

func TestProviderDefinitionClassifiesSecretsOnTheServer(t *testing.T) {
	tests := []struct {
		means       notifications.Means
		settings    map[string]string
		wantConfig  map[string]string
		wantSecrets map[string]string
	}{
		{
			means: notifications.MeansEmail,
			settings: map[string]string{
				"host": "mail.example.test", "port": "587", "password": "smtp-password",
			},
			wantConfig:  map[string]string{"host": "mail.example.test", "port": "587"},
			wantSecrets: map[string]string{"password": "smtp-password"},
		},
		{
			means:       notifications.MeansWebhook,
			settings:    map[string]string{"url": "https://hooks.example.test/path/token", "hmacSecret": "signing-key"},
			wantConfig:  map[string]string{},
			wantSecrets: map[string]string{"url": "https://hooks.example.test/path/token", "hmacSecret": "signing-key"},
		},
		{
			means:       notifications.MeansSlack,
			settings:    map[string]string{"webhookUrl": "https://hooks.slack.com/services/secret"},
			wantConfig:  map[string]string{},
			wantSecrets: map[string]string{"webhookUrl": "https://hooks.slack.com/services/secret"},
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.means), func(t *testing.T) {
			definition, ok := notifications.ProviderDefinitionFor(tt.means)
			if !ok {
				t.Fatalf("provider definition %q is missing", tt.means)
			}
			configuration, credentials, err := definition.Classify(tt.settings)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(configuration, tt.wantConfig) {
				t.Fatalf("configuration = %#v, want %#v", configuration, tt.wantConfig)
			}
			if !reflect.DeepEqual(credentials, tt.wantSecrets) {
				t.Fatalf("credentials = %#v, want %#v", credentials, tt.wantSecrets)
			}
		})
	}
}

func TestProviderDefinitionRejectsUnknownFields(t *testing.T) {
	definition, ok := notifications.ProviderDefinitionFor(notifications.MeansDiscord)
	if !ok {
		t.Fatal("Discord provider definition is missing")
	}
	if _, _, err := definition.Classify(map[string]string{"configuration": "client-chosen-bucket"}); err == nil {
		t.Fatal("expected an unknown provider field to be rejected")
	}
}

func TestProviderDefinitionReturnsOnlySafeFieldState(t *testing.T) {
	definition, ok := notifications.ProviderDefinitionFor(notifications.MeansEmail)
	if !ok {
		t.Fatal("SMTP provider definition is missing")
	}
	states := definition.Redact(
		map[string]string{"host": "mail.example.test", "username": "mailer"},
		map[string]string{"password": "never-return-this"},
	)
	byKey := make(map[string]notifications.ProviderFieldState, len(states))
	for _, state := range states {
		byKey[state.Key] = state
	}
	if got := byKey["host"]; got.Value != "mail.example.test" || got.SecretConfigured {
		t.Fatalf("host state = %+v", got)
	}
	if got := byKey["password"]; got.Value != "" || !got.SecretConfigured {
		t.Fatalf("password state = %+v", got)
	}
	if got := byKey["username"]; got.Value != "mailer" || got.SecretConfigured {
		t.Fatalf("username state = %+v", got)
	}
}

func TestCredentialBearingProviderLocationsAreAlwaysSensitive(t *testing.T) {
	fields := map[notifications.Means]string{
		notifications.MeansWebhook:    "url",
		notifications.MeansDiscord:    "webhookUrl",
		notifications.MeansNtfy:       "topic",
		notifications.MeansGotify:     "applicationToken",
		notifications.MeansApprise:    "configurationKey",
		notifications.MeansPushover:   "recipientKey",
		notifications.MeansTelegram:   "chatId",
		notifications.MeansMattermost: "webhookUrl",
		notifications.MeansMatrix:     "roomId",
		notifications.MeansWebPush:    "endpoint",
		notifications.MeansMQTT:       "brokerUrl",
		notifications.MeansSlack:      "webhookUrl",
	}
	for means, key := range fields {
		definition, ok := notifications.ProviderDefinitionFor(means)
		if !ok {
			t.Fatalf("provider definition %q is missing", means)
		}
		field, ok := definition.Field(key)
		if !ok || !field.Sensitive {
			t.Errorf("%s.%s must be server-classified as sensitive", means, key)
		}
	}
}

func TestProviderDefinitionsOfferOnlyEventsTheirRecipientModelCanDeliver(t *testing.T) {
	smtp, ok := notifications.ProviderDefinitionFor(notifications.MeansEmail)
	if !ok {
		t.Fatal("SMTP provider definition is missing")
	}
	slack, ok := notifications.ProviderDefinitionFor(notifications.MeansSlack)
	if !ok {
		t.Fatal("Slack provider definition is missing")
	}
	if !hasProviderTopic(smtp, notifications.TopicProposalApproved) {
		t.Fatal("SMTP must offer requester-addressed events")
	}
	if hasProviderTopic(slack, notifications.TopicProposalApproved) ||
		hasProviderTopic(slack, notifications.TopicProposalDeclined) {
		t.Fatal("a shared Slack destination must not offer requester-only events")
	}
	if !hasProviderTopic(slack, notifications.TopicProposalSubmitted) ||
		!hasProviderTopic(slack, notifications.TopicChannelDegraded) {
		t.Fatal("Slack must offer approver and operator events")
	}
	pushover, ok := notifications.ProviderDefinitionFor(notifications.MeansPushover)
	if !ok {
		t.Fatal("Pushover provider definition is missing")
	}
	if pushover.MemberOwned || hasProviderTopic(pushover, notifications.TopicProposalApproved) ||
		hasProviderTopic(pushover, notifications.TopicProposalDeclined) {
		t.Fatal("installation Pushover must not offer requester-only events")
	}
}

func hasProviderTopic(definition notifications.ProviderDefinition, want notifications.Topic) bool {
	for _, topic := range definition.Topics {
		if topic == want {
			return true
		}
	}
	return false
}
