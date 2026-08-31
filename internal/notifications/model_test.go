package notifications

import (
	"testing"
	"time"
)

func TestIntentValidationPinsTopicIdentityAndPolicy(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	invitation := Intent{
		ID: "intent-1", Topic: TopicAccountInvitation,
		RecipientKind: RecipientInvitation, RecipientID: "invite-1",
		ReferenceKind: ReferenceInvitation, ReferenceID: "invite-1",
		Policy: PolicyMandatoryAccount, IdempotencyKey: "invite-1:created", CreatedAt: now,
	}
	if err := invitation.Validate(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*Intent)
	}{
		{"email is not a recipient identity", func(i *Intent) { i.RecipientKind = "email" }},
		{"invitation ids must agree", func(i *Intent) { i.ReferenceID = "another" }},
		{"account delivery is mandatory", func(i *Intent) { i.Policy = PolicyConfigurable }},
		{"template data rejects header injection", func(i *Intent) { i.Template.RecipientName = "Ada\r\nBcc: attacker@example.com" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := invitation
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestRetryDelayIsFixedDeterministicAndBounded(t *testing.T) {
	bases := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	for next, base := range bases {
		attempt := next + 2
		got, ok := RetryDelay("intent-1", attempt)
		if !ok {
			t.Fatalf("attempt %d was not retryable", attempt)
		}
		again, _ := RetryDelay("intent-1", attempt)
		if got != again {
			t.Fatalf("attempt %d delay changed: %v then %v", attempt, got, again)
		}
		if got < base*9/10 || got > base*11/10 {
			t.Fatalf("attempt %d delay %v outside ±10%% of %v", attempt, got, base)
		}
	}
	if _, ok := RetryDelay("intent-1", 1); ok {
		t.Fatal("initial attempt is not a retry")
	}
	if _, ok := RetryDelay("intent-1", MaxAttempts+1); ok {
		t.Fatal("retry policy exceeded five attempts")
	}
}

func TestAttemptOutcomeValidationIsClosed(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	base := Attempt{
		ID: "attempt-1", IntentID: "intent-1", Means: MeansEmail,
		DestinationRef: "contact-1", DestinationRedacted: "a***@example.com",
		Status: StatusQueued, AttemptNumber: 1, AvailableAt: now, CreatedAt: now,
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}

	failed := base
	failed.Status = StatusFailed
	failed.FailureClass = FailureTransientPreAcceptance
	failed.OutcomeCode = OutcomeTransportUnavailable
	failed.FinishedAt = now
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}
	failed.OutcomeCode = "smtp said password=hunter2"
	if err := failed.Validate(); err == nil {
		t.Fatal("arbitrary provider error was accepted for persistence")
	}
}

func TestAttemptStateRequiresMatchingLeaseAndTimes(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	base := Attempt{
		ID: "attempt-1", IntentID: "intent-1", Means: MeansEmail,
		DestinationRef: "contact-1", DestinationRedacted: "a***@example.com",
		Status: StatusQueued, AttemptNumber: 1, AvailableAt: now, CreatedAt: now,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid queued attempt: %v", err)
	}

	queuedWithLease := base
	queuedWithLease.LeaseOwner = "worker-1"
	if err := queuedWithLease.Validate(); err == nil {
		t.Fatal("queued attempt accepted a lease owner")
	}

	sendingWithoutLease := base
	sendingWithoutLease.Status = StatusSending
	if err := sendingWithoutLease.Validate(); err == nil {
		t.Fatal("sending attempt accepted without lease metadata")
	}

	terminalWithoutFinish := base
	terminalWithoutFinish.Status = StatusDelivered
	if err := terminalWithoutFinish.Validate(); err == nil {
		t.Fatal("terminal attempt accepted without finish time")
	}
}
