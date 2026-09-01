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

func TestConfigurableProductIntentValidationPinsAudienceAndReference(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	cases := []Intent{
		{ID: "intent-submitted", Topic: TopicProposalSubmitted, RecipientKind: RecipientApprovers,
			RecipientID: "approvers", ReferenceKind: ReferenceProposal, ReferenceID: "proposal-1",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Saturday Cartoons"},
			IdempotencyKey: "proposal-1:submitted:approvers", CreatedAt: now},
		{ID: "intent-approved", Topic: TopicProposalApproved, RecipientKind: RecipientPerson,
			RecipientID: "user-1", ReferenceKind: ReferenceProposal, ReferenceID: "proposal-1",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Saturday Cartoons"},
			IdempotencyKey: "proposal-1:approved:user-1", CreatedAt: now},
		{ID: "intent-declined", Topic: TopicProposalDeclined, RecipientKind: RecipientPerson,
			RecipientID: "user-1", ReferenceKind: ReferenceProposal, ReferenceID: "proposal-1",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Saturday Cartoons", Summary: "Needs a narrower audience"},
			IdempotencyKey: "proposal-1:declined:user-1", CreatedAt: now},
		{ID: "intent-available", Topic: TopicAcquisitionAvailable, RecipientKind: RecipientPerson,
			RecipientID: "user-1", ReferenceKind: ReferenceTitle, ReferenceID: "movie:tmdb:603",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "The Matrix"},
			IdempotencyKey: "movie:tmdb:603:available:user-1", CreatedAt: now},
		{ID: "intent-gave-up", Topic: TopicAcquisitionGaveUp, RecipientKind: RecipientOperators,
			RecipientID: "operators", ReferenceKind: ReferenceTitle, ReferenceID: "movie:tmdb:603",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "The Matrix"},
			IdempotencyKey: "movie:tmdb:603:gave-up:operators", CreatedAt: now},
		{ID: "intent-live", Topic: TopicChannelLive, RecipientKind: RecipientPerson,
			RecipientID: "user-1", ReferenceKind: ReferenceChannel, ReferenceID: "channel-1",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Saturday Cartoons"},
			IdempotencyKey: "channel-1:live:user-1", CreatedAt: now},
		{ID: "intent-degraded", Topic: TopicChannelDegraded, RecipientKind: RecipientOperators,
			RecipientID: "operators", ReferenceKind: ReferenceChannel, ReferenceID: "channel-1",
			Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Saturday Cartoons"},
			IdempotencyKey: "channel-1:degraded:operators", CreatedAt: now},
	}
	for _, intent := range cases {
		if err := intent.Validate(); err != nil {
			t.Errorf("%s: %v", intent.Topic, err)
		}
	}

	invalid := cases[0]
	invalid.Policy = PolicyMandatoryAccount
	if err := invalid.Validate(); err == nil {
		t.Fatal("product intent accepted mandatory account policy")
	}
	invalid = cases[0]
	invalid.RecipientID = "some-other-group"
	if err := invalid.Validate(); err == nil {
		t.Fatal("approver audience accepted an arbitrary recipient id")
	}
	invalid = cases[0]
	invalid.ReferenceKind = ReferenceChannel
	if err := invalid.Validate(); err == nil {
		t.Fatal("Proposal topic accepted a Channel reference")
	}
	invalid = cases[0]
	invalid.Template.SubjectName = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("product intent accepted an empty subject name")
	}
	invalid = cases[0]
	invalid.Template.Summary = "safe line\nBcc: attacker@example.com"
	if err := invalid.Validate(); err == nil {
		t.Fatal("product summary accepted header injection")
	}
}

func TestDeliveryMeansVocabularyIsClosed(t *testing.T) {
	means := []Means{
		MeansEmail, MeansWebhook, MeansDiscord, MeansNtfy, MeansGotify, MeansApprise,
		MeansPushover, MeansTelegram, MeansMattermost, MeansMatrix, MeansWebPush,
		MeansMQTT, MeansSlack,
	}
	for _, means := range means {
		route := Route{Means: means, DestinationRef: "destination-1", DestinationRedacted: "configured"}
		if err := route.Validate(); err != nil {
			t.Errorf("means %q: %v", means, err)
		}
	}
	if err := (Route{Means: "invented", DestinationRef: "destination-1", DestinationRedacted: "configured"}).Validate(); err == nil {
		t.Fatal("unknown delivery means was accepted")
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
