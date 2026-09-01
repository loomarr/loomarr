package notifications_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

func TestEmailConfigRequiresCompleteCoherentSender(t *testing.T) {
	valid := notifications.EmailConfig{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Security: notifications.EmailSecuritySTARTTLS,
		Username: "loomarr", Password: "secret-value",
		FromAddress: "loomarr@example.com", FromName: "Loomarr",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	for name, mutate := range map[string]func(*notifications.EmailConfig){
		"host":              func(c *notifications.EmailConfig) { c.Host = "" },
		"port":              func(c *notifications.EmailConfig) { c.Port = 0 },
		"security":          func(c *notifications.EmailConfig) { c.Security = "opportunistic" },
		"sender":            func(c *notifications.EmailConfig) { c.FromAddress = "" },
		"password username": func(c *notifications.EmailConfig) { c.Username = "" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected invalid configuration")
			}
		})
	}

	unauthenticated := valid
	unauthenticated.Username = ""
	unauthenticated.Password = ""
	if err := unauthenticated.Validate(); err != nil {
		t.Fatalf("unauthenticated relay: %v", err)
	}

	disabled := notifications.EmailConfig{}
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled config should not require SMTP: %v", err)
	}
}

func TestRenderAccountEmailUsesTypedIntentAndAccessibleAlternatives(t *testing.T) {
	expires := time.Date(2030, time.January, 2, 15, 4, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		topic       notifications.Topic
		subjectPart string
		actionText  string
	}{
		"invitation": {notifications.TopicAccountInvitation, "invited", "Open your invitation"},
		"recovery":   {notifications.TopicLocalPasswordRecovery, "Reset", "Reset your password"},
	} {
		t.Run(name, func(t *testing.T) {
			intent := notifications.Intent{Topic: tc.topic, Template: notifications.TemplateData{RecipientName: "Ada & Co"}}
			content, err := notifications.RenderAccountEmail(intent,
				"https://loomarr.example/join#grant=secret-token", expires)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(content.Subject, tc.subjectPart) ||
				!strings.Contains(content.TextBody, tc.actionText) ||
				!strings.Contains(content.TextBody, "January 2, 2030") {
				t.Errorf("plain alternative = %+v", content)
			}
			for _, want := range []string{`<html lang="en">`, `<main>`, tc.actionText,
				"Ada &amp; Co", "January 2, 2030"} {
				if !strings.Contains(content.HTMLBody, want) {
					t.Errorf("HTML omitted %q: %s", want, content.HTMLBody)
				}
			}
			if strings.Contains(content.HTMLBody, "Ada & Co") {
				t.Fatal("recipient name was not HTML escaped")
			}
		})
	}
}

func TestRenderAccountEmailRejectsUnknownTopicAndUnsafeActionURL(t *testing.T) {
	for _, tc := range []struct {
		intent notifications.Intent
		url    string
	}{
		{notifications.Intent{Topic: "channel_live"}, "https://loomarr.example/join#grant=token"},
		{notifications.Intent{Topic: notifications.TopicAccountInvitation}, "javascript:alert(1)"},
	} {
		if _, err := notifications.RenderAccountEmail(tc.intent, tc.url, time.Now()); err == nil {
			t.Fatal("expected rendering refusal")
		}
	}
}

func TestEmailAdapterMapsOnlyClosedDeliveryOutcomes(t *testing.T) {
	delivery := notifications.Delivery{Intent: notifications.Intent{Topic: notifications.TopicAccountInvitation}}
	message := notifications.EmailMessage{
		ToAddress: "person@example.com", Subject: "Invitation", TextBody: "Plain text",
		HTMLBody: "<p>HTML</p>",
	}
	config := notifications.EmailConfig{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Security:    notifications.EmailSecuritySTARTTLS,
		FromAddress: "loomarr@example.com", FromName: "Loomarr",
	}

	for name, tc := range map[string]struct {
		transmission notifications.EmailTransmission
		want         notifications.Result
		invalidates  bool
	}{
		"accepted": {
			transmission: notifications.EmailTransmission{State: notifications.EmailAccepted, ProviderMessageID: "provider-42"},
			want:         notifications.Result{Status: notifications.StatusDelivered, ProviderMessageID: "provider-42"},
		},
		"temporary before acceptance": {
			transmission: notifications.EmailTransmission{State: notifications.EmailTransientPreAcceptance},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailureTransientPreAcceptance, OutcomeCode: notifications.OutcomeTransportUnavailable},
			invalidates:  true,
		},
		"recipient rejected": {
			transmission: notifications.EmailTransmission{State: notifications.EmailRecipientRejected},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailurePermanent, OutcomeCode: notifications.OutcomeRecipientRejected},
			invalidates:  true,
		},
		"ambiguous acceptance": {
			transmission: notifications.EmailTransmission{State: notifications.EmailAcceptanceAmbiguous},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailureAmbiguous, OutcomeCode: notifications.OutcomeAcceptanceAmbiguous},
		},
		"cancelled": {
			transmission: notifications.EmailTransmission{State: notifications.EmailCancelled},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailureCancelled, OutcomeCode: notifications.OutcomeCancelled},
		},
	} {
		t.Run(name, func(t *testing.T) {
			sender := &fakeEmailSender{result: tc.transmission}
			invalidations := 0
			adapter := notifications.NewEmailAdapter(func() notifications.EmailConfig { return config },
				fakeEmailMaterializer{materialized: notifications.MaterializedEmail{
					Message: message,
					Invalidate: func(context.Context) error {
						invalidations++
						return nil
					},
				}}, sender)
			got := adapter.Deliver(context.Background(), delivery)
			if got != tc.want {
				t.Fatalf("result = %+v, want %+v", got, tc.want)
			}
			if len(sender.messages) != 1 || sender.messages[0] != message {
				t.Fatalf("sender messages = %+v", sender.messages)
			}
			wantInvalidations := 0
			if tc.invalidates {
				wantInvalidations = 1
			}
			if invalidations != wantInvalidations {
				t.Fatalf("grant invalidations = %d, want %d", invalidations, wantInvalidations)
			}
		})
	}
}

func TestEmailAdapterStopsRetryWhenPreAcceptanceGrantCannotBeInvalidated(t *testing.T) {
	config := notifications.EmailConfig{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Security: notifications.EmailSecuritySTARTTLS, FromAddress: "loomarr@example.com",
	}
	materializer := fakeEmailMaterializer{materialized: notifications.MaterializedEmail{
		Message: notifications.EmailMessage{
			ToAddress: "person@example.com", Subject: "Invitation",
			TextBody: "Plain text", HTMLBody: "<p>HTML</p>",
		},
		Invalidate: func(context.Context) error { return context.DeadlineExceeded },
	}}
	sender := &fakeEmailSender{result: notifications.EmailTransmission{State: notifications.EmailTransientPreAcceptance}}
	adapter := notifications.NewEmailAdapter(func() notifications.EmailConfig { return config }, materializer, sender)

	result := adapter.Deliver(t.Context(), notifications.Delivery{})
	if result.Status != notifications.StatusFailed || result.FailureClass != notifications.FailureAmbiguous ||
		result.OutcomeCode != notifications.OutcomeAcceptanceAmbiguous {
		t.Fatalf("result = %+v", result)
	}
}

func TestEmailAdapterInvalidatesGrantWhenMaterializedMessageIsInvalid(t *testing.T) {
	config := notifications.EmailConfig{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Security: notifications.EmailSecuritySTARTTLS, FromAddress: "loomarr@example.com",
	}
	invalidations := 0
	materializer := fakeEmailMaterializer{materialized: notifications.MaterializedEmail{
		Message: notifications.EmailMessage{ToAddress: "not a mailbox"},
		Invalidate: func(context.Context) error {
			invalidations++
			return nil
		},
	}}
	sender := &fakeEmailSender{}
	adapter := notifications.NewEmailAdapter(func() notifications.EmailConfig { return config }, materializer, sender)

	result := adapter.Deliver(t.Context(), notifications.Delivery{})
	if result.Status != notifications.StatusFailed || result.FailureClass != notifications.FailurePermanent ||
		result.OutcomeCode != notifications.OutcomeConfigurationInvalid {
		t.Fatalf("result = %+v", result)
	}
	if invalidations != 1 {
		t.Fatalf("grant invalidations = %d, want 1", invalidations)
	}
	if len(sender.messages) != 0 {
		t.Fatal("invalid materialized message reached SMTP sender")
	}
}

func TestEmailAdapterUsesTheEncryptedProviderSettingsForProductDelivery(t *testing.T) {
	sender := &fakeEmailSender{result: notifications.EmailTransmission{State: notifications.EmailAccepted}}
	materializer := fakeEmailMaterializer{materialized: notifications.MaterializedEmail{Message: notifications.EmailMessage{
		ToAddress: "person@example.com", Subject: "Loomarr: Channel live",
		TextBody: "Channel is live", HTMLBody: "<p>Channel is live</p>",
	}}}
	adapter := notifications.NewEmailAdapter(nil, materializer, sender)
	destination := notifications.Destination{
		Means: notifications.MeansEmail,
		Configuration: map[string]string{
			"host": "smtp.provider.test", "port": "465", "security": "tls",
			"fromAddress": "loomarr@example.com", "fromName": "Loomarr", "username": "mailer",
		},
		Credentials: map[string]string{"password": "provider-password"},
	}
	result := adapter.Deliver(t.Context(), notifications.Delivery{Destination: &destination})
	if result.Status != notifications.StatusDelivered || len(sender.configs) != 1 {
		t.Fatalf("result/configs = %+v/%+v", result, sender.configs)
	}
	config := sender.configs[0]
	if config.Host != "smtp.provider.test" || config.Port != 465 ||
		config.Security != notifications.EmailSecurityTLS || config.Password != "provider-password" {
		t.Fatalf("SMTP provider config = %+v", config)
	}
	if err := adapter.ValidateDestination(destination.Configuration, destination.Credentials); err != nil {
		t.Fatalf("valid SMTP provider rejected: %v", err)
	}
}

func TestEmailAdapterSendsEveryMaterializedGroupRecipient(t *testing.T) {
	sender := &fakeEmailSender{result: notifications.EmailTransmission{State: notifications.EmailAccepted}}
	adapter := notifications.NewEmailAdapter(nil, fakeEmailMaterializer{materialized: notifications.MaterializedEmail{
		Messages: []notifications.EmailMessage{
			{ToAddress: "ada@example.com", Subject: "Loomarr event", TextBody: "Event", HTMLBody: "<p>Event</p>"},
			{ToAddress: "grace@example.com", Subject: "Loomarr event", TextBody: "Event", HTMLBody: "<p>Event</p>"},
		},
	}}, sender)
	destination := notifications.Destination{Means: notifications.MeansEmail, Configuration: map[string]string{
		"host": "smtp.example.test", "port": "587", "security": "starttls", "fromAddress": "loomarr@example.com",
	}}
	result := adapter.Deliver(t.Context(), notifications.Delivery{Destination: &destination})
	if result.Status != notifications.StatusDelivered || len(sender.messages) != 2 ||
		sender.messages[0].ToAddress != "ada@example.com" || sender.messages[1].ToAddress != "grace@example.com" {
		t.Fatalf("result/messages = %+v/%+v", result, sender.messages)
	}
}

func TestEmailAdapterTreatsPartialGroupAcceptanceAsAmbiguous(t *testing.T) {
	sender := &sequenceEmailSenderForAdapter{results: []notifications.EmailTransmission{
		{State: notifications.EmailAccepted}, {State: notifications.EmailRecipientRejected},
	}}
	adapter := notifications.NewEmailAdapter(nil, fakeEmailMaterializer{materialized: notifications.MaterializedEmail{
		Messages: []notifications.EmailMessage{
			{ToAddress: "ada@example.com", Subject: "Event", TextBody: "Event", HTMLBody: "<p>Event</p>"},
			{ToAddress: "invalid@example.com", Subject: "Event", TextBody: "Event", HTMLBody: "<p>Event</p>"},
		},
	}}, sender)
	destination := notifications.Destination{Means: notifications.MeansEmail, Configuration: map[string]string{
		"host": "smtp.example.test", "port": "587", "security": "starttls", "fromAddress": "loomarr@example.com",
	}}
	result := adapter.Deliver(t.Context(), notifications.Delivery{Destination: &destination})
	if result.FailureClass != notifications.FailureAmbiguous || result.OutcomeCode != notifications.OutcomeAcceptanceAmbiguous {
		t.Fatalf("result = %+v", result)
	}
}

func TestRenderProductEmailEscapesContentAndIncludesStableLink(t *testing.T) {
	content, err := notifications.RenderProductEmail(notifications.Intent{
		Topic: notifications.TopicChannelDegraded, Policy: notifications.PolicyConfigurable,
		Template: notifications.TemplateData{SubjectName: `<Channel & Friends>`, Summary: `Needs "attention"`},
	}, "https://loomarr.example/channels/channel-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content.TextBody, "https://loomarr.example/channels/channel-1") ||
		strings.Contains(content.HTMLBody, "<Channel & Friends>") ||
		!strings.Contains(content.HTMLBody, "&lt;Channel &amp; Friends&gt;") {
		t.Fatalf("product email = %+v", content)
	}
}

func TestEmailAdapterSendTestUsesAppliedConfigurationWithoutMaterializingADomainGrant(t *testing.T) {
	sender := &fakeEmailSender{result: notifications.EmailTransmission{State: notifications.EmailAccepted, ProviderMessageID: "provider-42"}}
	config := notifications.EmailConfig{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Security:    notifications.EmailSecuritySTARTTLS,
		FromAddress: "loomarr@example.com", FromName: "Loomarr",
	}
	adapter := notifications.NewEmailAdapter(func() notifications.EmailConfig { return config }, nil, sender)

	result := adapter.SendTest(t.Context(), "admin@example.com")
	if result.Status != notifications.StatusDelivered || result.ProviderMessageID != "provider-42" {
		t.Fatalf("test result = %+v", result)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("messages = %d", len(sender.messages))
	}
	message := sender.messages[0]
	if message.ToAddress != "admin@example.com" || !strings.Contains(message.Subject, "test") ||
		!strings.Contains(message.TextBody, "configured correctly") ||
		!strings.Contains(message.HTMLBody, "configured correctly") {
		t.Fatalf("test message = %+v", message)
	}

	config.Enabled = false
	result = adapter.SendTest(t.Context(), "admin@example.com")
	if result.Status != notifications.StatusSuppressed || result.OutcomeCode != notifications.OutcomeDeliveryDisabled {
		t.Fatalf("disabled result = %+v", result)
	}
	if len(sender.messages) != 1 {
		t.Fatal("disabled delivery reached SMTP sender")
	}
}

type fakeEmailMaterializer struct {
	materialized notifications.MaterializedEmail
	err          error
}

func (f fakeEmailMaterializer) Materialize(context.Context, notifications.Delivery) (notifications.MaterializedEmail, error) {
	return f.materialized, f.err
}

type fakeEmailSender struct {
	result   notifications.EmailTransmission
	messages []notifications.EmailMessage
	configs  []notifications.EmailConfig
}

type sequenceEmailSenderForAdapter struct {
	results []notifications.EmailTransmission
}

func (s *sequenceEmailSenderForAdapter) Send(
	context.Context, notifications.EmailConfig, notifications.EmailMessage,
) notifications.EmailTransmission {
	result := s.results[0]
	s.results = s.results[1:]
	return result
}

func (f *fakeEmailSender) Send(_ context.Context, config notifications.EmailConfig, message notifications.EmailMessage) notifications.EmailTransmission {
	f.configs = append(f.configs, config)
	f.messages = append(f.messages, message)
	return f.result
}
