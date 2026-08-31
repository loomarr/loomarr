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
	}{
		"accepted": {
			transmission: notifications.EmailTransmission{State: notifications.EmailAccepted, ProviderMessageID: "provider-42"},
			want:         notifications.Result{Status: notifications.StatusDelivered, ProviderMessageID: "provider-42"},
		},
		"temporary before acceptance": {
			transmission: notifications.EmailTransmission{State: notifications.EmailTransientPreAcceptance},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailureTransientPreAcceptance, OutcomeCode: notifications.OutcomeTransportUnavailable},
		},
		"recipient rejected": {
			transmission: notifications.EmailTransmission{State: notifications.EmailRecipientRejected},
			want:         notifications.Result{Status: notifications.StatusFailed, FailureClass: notifications.FailurePermanent, OutcomeCode: notifications.OutcomeRecipientRejected},
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
			adapter := notifications.NewEmailAdapter(func() notifications.EmailConfig { return config },
				fakeEmailMaterializer{message: message}, sender)
			got := adapter.Deliver(context.Background(), delivery)
			if got != tc.want {
				t.Fatalf("result = %+v, want %+v", got, tc.want)
			}
			if len(sender.messages) != 1 || sender.messages[0] != message {
				t.Fatalf("sender messages = %+v", sender.messages)
			}
		})
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
	message notifications.EmailMessage
	err     error
}

func (f fakeEmailMaterializer) Materialize(context.Context, notifications.Delivery) (notifications.EmailMessage, error) {
	return f.message, f.err
}

type fakeEmailSender struct {
	result   notifications.EmailTransmission
	messages []notifications.EmailMessage
}

func (f *fakeEmailSender) Send(_ context.Context, _ notifications.EmailConfig, message notifications.EmailMessage) notifications.EmailTransmission {
	f.messages = append(f.messages, message)
	return f.result
}
