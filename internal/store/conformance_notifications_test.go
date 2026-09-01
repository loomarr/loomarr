package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

func testNotificationLifecycle(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	intent, attempts := notificationFixture("one", now, notifications.StatusQueued)
	createdIntent, created, err := s.CreateNotificationIntent(ctx, intent, attempts)
	if err != nil || !created || createdIntent.ID != intent.ID {
		t.Fatalf("create notification = %+v, %t, %v", createdIntent, created, err)
	}
	byReference, err := s.ListNotificationIntentsByReference(
		ctx, notifications.ReferenceInvitation, intent.ReferenceID,
	)
	if err != nil || len(byReference) != 1 || byReference[0].ID != intent.ID {
		t.Fatalf("list notification intents by reference = %+v, %v", byReference, err)
	}
	if unrelated, err := s.ListNotificationIntentsByReference(
		ctx, notifications.ReferenceInvitation, "another-invitation",
	); err != nil || len(unrelated) != 0 {
		t.Fatalf("unrelated notification intents = %+v, %v", unrelated, err)
	}

	duplicate := intent
	duplicate.ID = "intent-duplicate-call"
	duplicate.CreatedAt = now.Add(time.Minute)
	duplicateAttempts := []notifications.Attempt{attempts[0]}
	duplicateAttempts[0].ID = "attempt-duplicate-call"
	duplicateAttempts[0].IntentID = duplicate.ID
	duplicateAttempts[0].CreatedAt = duplicate.CreatedAt
	duplicateAttempts[0].AvailableAt = duplicate.CreatedAt
	got, created, err := s.CreateNotificationIntent(ctx, duplicate, duplicateAttempts)
	if err != nil || created || got.ID != intent.ID {
		t.Fatalf("idempotent create = %+v, %t, %v", got, created, err)
	}
	conflict := duplicate
	conflict.Template.RecipientName = "Different payload"
	if _, _, err := s.CreateNotificationIntent(ctx, conflict, duplicateAttempts); !errors.Is(err, notifications.ErrConflict) {
		t.Fatalf("conflicting idempotency key = %v, want ErrConflict", err)
	}

	claimed, err := s.ClaimDueNotificationAttempt(ctx, "worker-a", now, time.Minute)
	if err != nil || claimed.ID != attempts[0].ID || claimed.Status != notifications.StatusSending {
		t.Fatalf("claim notification = %+v, %v", claimed, err)
	}
	if _, err := s.ClaimDueNotificationAttempt(ctx, "worker-b", now, time.Minute); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("leased attempt reclaimed = %v", err)
	}
	wrongOwner := notifications.Completion{
		AttemptID: claimed.ID, LeaseOwner: "worker-b", Status: notifications.StatusDelivered, FinishedAt: now,
	}
	if err := s.CompleteNotificationAttempt(ctx, wrongOwner); !errors.Is(err, notifications.ErrConflict) {
		t.Fatalf("wrong lease owner completion = %v, want ErrConflict", err)
	}

	delay, _ := notifications.RetryDelay(intent.ID, 2)
	next := notifications.Attempt{
		ID: "attempt-one-2", IntentID: intent.ID, Means: notifications.MeansEmail,
		DestinationRef: "contact-one", DestinationRedacted: "a***@example.com",
		Status: notifications.StatusQueued, AttemptNumber: 2, AvailableAt: now.Add(delay), CreatedAt: now,
	}
	completion := notifications.Completion{
		AttemptID: claimed.ID, LeaseOwner: "worker-a", Status: notifications.StatusFailed,
		FailureClass: notifications.FailureTransientPreAcceptance,
		OutcomeCode:  notifications.OutcomeTransportUnavailable,
		FinishedAt:   now,
		Next:         &next,
	}
	if err := s.CompleteNotificationAttempt(ctx, completion); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ListNotificationAttempts(ctx, intent.ID)
	if err != nil || len(stored) != 2 || stored[0].Status != notifications.StatusFailed ||
		stored[1].Status != notifications.StatusQueued {
		t.Fatalf("retry transition = %+v, %v", stored, err)
	}
	if got, err := s.GetNotificationIntent(ctx, intent.ID); err != nil || !got.TerminalAt.IsZero() {
		t.Fatalf("intent with queued retry became terminal: %+v, %v", got, err)
	}
	if _, err := s.ClaimDueNotificationAttempt(ctx, "worker-b", next.AvailableAt.Add(-time.Second), time.Minute); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("retry claimed early = %v", err)
	}
	claimed, err = s.ClaimDueNotificationAttempt(ctx, "worker-b", next.AvailableAt, time.Minute)
	if err != nil || claimed.AttemptNumber != 2 {
		t.Fatalf("claim due retry = %+v, %v", claimed, err)
	}
	if err := s.CompleteNotificationAttempt(ctx, notifications.Completion{
		AttemptID: claimed.ID, LeaseOwner: "worker-b", Status: notifications.StatusFailed,
		FailureClass: notifications.FailureAmbiguous, OutcomeCode: notifications.OutcomeAcceptanceAmbiguous,
		FinishedAt: next.AvailableAt,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetNotificationIntent(ctx, intent.ID)
	if err != nil || !got.TerminalAt.Equal(next.AvailableAt) {
		t.Fatalf("terminal intent = %+v, %v", got, err)
	}

	deliveredIntent, deliveredAttempts := notificationFixture("delivered", now, notifications.StatusQueued)
	if _, _, err := s.CreateNotificationIntent(ctx, deliveredIntent, deliveredAttempts); err != nil {
		t.Fatal(err)
	}
	deliveredAttempt, err := s.ClaimDueNotificationAttempt(ctx, "worker-delivered", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteNotificationAttempt(ctx, notifications.Completion{
		AttemptID: deliveredAttempt.ID, LeaseOwner: "worker-delivered",
		Status: notifications.StatusDelivered, ProviderMessageID: "provider-safe-42", FinishedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	deliveredStored, err := s.ListNotificationAttempts(ctx, deliveredIntent.ID)
	if err != nil || len(deliveredStored) != 1 || deliveredStored[0].Status != notifications.StatusDelivered ||
		deliveredStored[0].ProviderMessageID != "provider-safe-42" {
		t.Fatalf("delivered attempt = %+v, %v", deliveredStored, err)
	}
}

func testNotificationProductVocabulary(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	intent := notifications.Intent{
		ID: "intent-product", Topic: notifications.TopicChannelLive,
		RecipientKind: notifications.RecipientPerson, RecipientID: "user-1",
		ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-1",
		Policy: notifications.PolicyConfigurable,
		Template: notifications.TemplateData{
			RecipientName: "Ada", SubjectName: "Saturday Cartoons", Summary: "Your Channel is live.",
		},
		IdempotencyKey: "channel-1:live:user-1", CreatedAt: now,
	}
	means := []notifications.Means{
		notifications.MeansEmail, notifications.MeansWebhook, notifications.MeansDiscord,
		notifications.MeansNtfy, notifications.MeansGotify, notifications.MeansApprise,
		notifications.MeansPushover, notifications.MeansTelegram, notifications.MeansMattermost,
		notifications.MeansMatrix, notifications.MeansWebPush, notifications.MeansMQTT,
		notifications.MeansSlack,
	}
	attempts := make([]notifications.Attempt, 0, len(means))
	for index, means := range means {
		attempts = append(attempts, notifications.Attempt{
			ID: fmt.Sprintf("attempt-product-%d", index+1), IntentID: intent.ID, Means: means,
			DestinationRef: "destination-" + string(means), DestinationRedacted: "Configured " + string(means),
			Status: notifications.StatusQueued, AttemptNumber: 1, AvailableAt: now, CreatedAt: now,
		})
	}
	if _, created, err := s.CreateNotificationIntent(ctx, intent, attempts); err != nil || !created {
		t.Fatalf("create product notification = %t, %v", created, err)
	}
	stored, err := s.GetNotificationIntent(ctx, intent.ID)
	if err != nil || stored.Topic != notifications.TopicChannelLive ||
		stored.RecipientKind != notifications.RecipientPerson || stored.ReferenceKind != notifications.ReferenceChannel ||
		stored.Policy != notifications.PolicyConfigurable || stored.Template != intent.Template {
		t.Fatalf("stored product intent = %+v, %v", stored, err)
	}
	attempts, err = s.ListNotificationAttempts(ctx, intent.ID)
	if err != nil || len(attempts) != len(means) {
		t.Fatalf("stored product attempts = %+v, %v", attempts, err)
	}
	storedMeans := make(map[notifications.Means]bool, len(attempts))
	for _, attempt := range attempts {
		storedMeans[attempt.Means] = true
	}
	for _, means := range means {
		if !storedMeans[means] {
			t.Errorf("delivery means %q did not round-trip", means)
		}
	}

	unrouted := notifications.Intent{
		ID: "intent-product-unrouted", Topic: notifications.TopicProposalSubmitted,
		RecipientKind: notifications.RecipientApprovers, RecipientID: "approvers",
		ReferenceKind: notifications.ReferenceProposal, ReferenceID: "proposal-1",
		Policy:         notifications.PolicyConfigurable,
		Template:       notifications.TemplateData{SubjectName: "Sunday Mysteries"},
		IdempotencyKey: "proposal-1:submitted:approvers", CreatedAt: now,
	}
	createdIntent, created, err := s.CreateNotificationIntent(ctx, unrouted, nil)
	if err != nil || !created || !createdIntent.TerminalAt.Equal(now) {
		t.Fatalf("create unrouted product fact = %+v, %t, %v", createdIntent, created, err)
	}
	if attempts, err := s.ListNotificationAttempts(ctx, unrouted.ID); err != nil || len(attempts) != 0 {
		t.Fatalf("unrouted product attempts = %+v, %v", attempts, err)
	}

	remaining := []notifications.Intent{
		{ID: "intent-approved", Topic: notifications.TopicProposalApproved, RecipientKind: notifications.RecipientPerson,
			RecipientID: "user-1", ReferenceKind: notifications.ReferenceProposal, ReferenceID: "proposal-2"},
		{ID: "intent-declined", Topic: notifications.TopicProposalDeclined, RecipientKind: notifications.RecipientPerson,
			RecipientID: "user-1", ReferenceKind: notifications.ReferenceProposal, ReferenceID: "proposal-3"},
		{ID: "intent-available", Topic: notifications.TopicAcquisitionAvailable, RecipientKind: notifications.RecipientPerson,
			RecipientID: "user-1", ReferenceKind: notifications.ReferenceTitle, ReferenceID: "movie:tmdb:603"},
		{ID: "intent-gave-up", Topic: notifications.TopicAcquisitionGaveUp, RecipientKind: notifications.RecipientOperators,
			RecipientID: "operators", ReferenceKind: notifications.ReferenceTitle, ReferenceID: "movie:tmdb:604"},
		{ID: "intent-degraded", Topic: notifications.TopicChannelDegraded, RecipientKind: notifications.RecipientOperators,
			RecipientID: "operators", ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-2"},
	}
	for _, candidate := range remaining {
		candidate.Policy = notifications.PolicyConfigurable
		candidate.Template = notifications.TemplateData{SubjectName: "Safe subject"}
		candidate.IdempotencyKey = candidate.ID + ":created"
		candidate.CreatedAt = now
		if _, created, err := s.CreateNotificationIntent(ctx, candidate, nil); err != nil || !created {
			t.Errorf("create %s product fact = %t, %v", candidate.Topic, created, err)
		}
	}
}

func testNotificationDestinations(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	destination := notifications.Destination{
		ID: "destination-slack", Means: notifications.MeansSlack, Label: "Operations Slack",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics:  []notifications.Topic{notifications.TopicChannelDegraded, notifications.TopicAcquisitionGaveUp},
		Enabled: true, Configuration: map[string]string{"channel": "alerts"},
		Credentials: map[string]string{"token": "secret-that-must-round-trip"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.SaveNotificationDestination(ctx, destination); err != nil {
		t.Fatalf("save destination: %v", err)
	}
	stored, err := s.GetNotificationDestination(ctx, destination.ID)
	if err != nil || stored.ID != destination.ID || stored.Means != destination.Means ||
		stored.Label != destination.Label || stored.Scope != destination.Scope || stored.Audience != destination.Audience ||
		!stored.Enabled || len(stored.Topics) != 2 || stored.Configuration["channel"] != "alerts" ||
		stored.Credentials["token"] != "secret-that-must-round-trip" {
		t.Fatalf("stored destination = %+v, %v", stored.Summary(), err)
	}
	listed, err := s.ListNotificationDestinations(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != destination.ID {
		t.Fatalf("listed destinations = %+v, %v", listed, err)
	}

	destination.Enabled = false
	destination.Credentials = map[string]string{"token": "rotated-secret"}
	destination.UpdatedAt = now.Add(time.Minute)
	if err := s.SaveNotificationDestination(ctx, destination); err != nil {
		t.Fatalf("update destination: %v", err)
	}
	stored, err = s.GetNotificationDestination(ctx, destination.ID)
	if err != nil || stored.Enabled || stored.Credentials["token"] != "rotated-secret" ||
		!stored.CreatedAt.Equal(now) || !stored.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("updated destination = %+v, %v", stored.Summary(), err)
	}
	testIntent := notifications.Intent{
		ID: "intent-destination-test", Topic: notifications.TopicDeliveryTest,
		RecipientKind: notifications.RecipientOperators, RecipientID: destination.ID,
		ReferenceKind: notifications.ReferenceDestination, ReferenceID: destination.ID,
		Policy:         notifications.PolicyConfigurable,
		Template:       notifications.TemplateData{SubjectName: "Test notification"},
		IdempotencyKey: "destination-test:request-1", CreatedAt: now.Add(2 * time.Minute),
	}
	testAttempt := notifications.Attempt{
		ID: "attempt-destination-test", IntentID: testIntent.ID, Means: destination.Means,
		DestinationRef: destination.ID, DestinationRedacted: destination.Label,
		Status: notifications.StatusQueued, AttemptNumber: 1,
		AvailableAt: testIntent.CreatedAt, CreatedAt: testIntent.CreatedAt,
	}
	if _, created, err := s.CreateNotificationIntent(ctx, testIntent, []notifications.Attempt{testAttempt}); err != nil || !created {
		t.Fatalf("create destination test intent = %t, %v", created, err)
	}
	health, err := s.ListNotificationDestinationHealth(ctx)
	if err != nil || health[destination.ID].QueuedCount != 1 {
		t.Fatalf("queued destination health = %+v, %v", health, err)
	}
	claimed, err := s.ClaimDueNotificationAttempt(ctx, "health-worker", testIntent.CreatedAt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	failedAt := testIntent.CreatedAt.Add(time.Second)
	if err := s.CompleteNotificationAttempt(ctx, notifications.Completion{
		AttemptID: claimed.ID, LeaseOwner: "health-worker", Status: notifications.StatusFailed,
		FailureClass: notifications.FailurePermanent, OutcomeCode: notifications.OutcomeConfigurationInvalid,
		FinishedAt: failedAt,
	}); err != nil {
		t.Fatal(err)
	}
	health, err = s.ListNotificationDestinationHealth(ctx)
	gotHealth := health[destination.ID]
	if err != nil || gotHealth.QueuedCount != 0 || gotHealth.TerminalFailureCount != 1 ||
		gotHealth.LastFailureOutcome != notifications.OutcomeConfigurationInvalid || !gotHealth.LastFailureAt.Equal(failedAt) {
		t.Fatalf("failed destination health = %+v, %v", gotHealth, err)
	}
	if err := s.DeleteNotificationDestination(ctx, destination.ID); err != nil {
		t.Fatalf("delete destination: %v", err)
	}
	if _, err := s.GetNotificationDestination(ctx, destination.ID); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("get deleted destination: %v", err)
	}
}

func testNotificationExpiredLease(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	intent, attempts := notificationFixture("expired", now, notifications.StatusQueued)
	if _, _, err := s.CreateNotificationIntent(ctx, intent, attempts); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimDueNotificationAttempt(ctx, "lost-worker", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimDueNotificationAttempt(ctx, "next-worker", now.Add(time.Minute), time.Minute); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("claim after expired sending lease = %v, want no automatic retry", err)
	}
	stored, err := s.ListNotificationAttempts(ctx, intent.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("expired attempts = %+v, %v", stored, err)
	}
	if stored[0].Status != notifications.StatusFailed || stored[0].FailureClass != notifications.FailureAmbiguous ||
		stored[0].OutcomeCode != notifications.OutcomeWorkerInterrupted {
		t.Fatalf("expired sending attempt = %+v", stored[0])
	}
	got, err := s.GetNotificationIntent(ctx, intent.ID)
	if err != nil || !got.TerminalAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("expired intent terminal state = %+v, %v", got, err)
	}
}

func testNotificationConcurrentClaim(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	const count = 8
	for i := 0; i < count; i++ {
		intent, attempts := notificationFixture(fmt.Sprintf("concurrent-%d", i), now, notifications.StatusQueued)
		if _, _, err := s.CreateNotificationIntent(ctx, intent, attempts); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				attempt, err := s.ClaimDueNotificationAttempt(ctx, fmt.Sprintf("worker-%d", worker), now, time.Minute)
				if errors.Is(err, notifications.ErrNotFound) {
					return
				}
				if err != nil {
					t.Errorf("concurrent claim: %v", err)
					return
				}
				mu.Lock()
				seen[attempt.ID]++
				mu.Unlock()
			}
		}(worker)
	}
	wg.Wait()
	if len(seen) != count {
		t.Fatalf("claimed %d distinct attempts, want %d", len(seen), count)
	}
	for id, claims := range seen {
		if claims != 1 {
			t.Fatalf("attempt %s claimed %d times", id, claims)
		}
	}
}

func testNotificationRetention(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)
	old := now.Add(-notifications.Retention - time.Hour)
	for _, fixture := range []struct {
		name   string
		at     time.Time
		status notifications.Status
	}{
		{"old-terminal", old, notifications.StatusSuppressed},
		{"old-active", old, notifications.StatusQueued},
		{"new-terminal", now, notifications.StatusSuppressed},
	} {
		intent, attempts := notificationFixture(fixture.name, fixture.at, fixture.status)
		if _, _, err := s.CreateNotificationIntent(ctx, intent, attempts); err != nil {
			t.Fatal(err)
		}
	}
	purged, err := s.PurgeTerminalNotifications(ctx, now.Add(-notifications.Retention))
	if err != nil || purged != 1 {
		t.Fatalf("purge notifications = %d, %v; want one old terminal intent", purged, err)
	}
	if _, err := s.GetNotificationIntent(ctx, "intent-old-terminal"); !errors.Is(err, notifications.ErrNotFound) {
		t.Fatalf("old terminal intent survived: %v", err)
	}
	if attempts, err := s.ListNotificationAttempts(ctx, "intent-old-terminal"); err != nil || len(attempts) != 0 {
		t.Fatalf("purged intent attempts = %+v, %v", attempts, err)
	}
	if _, err := s.GetNotificationIntent(ctx, "intent-old-active"); err != nil {
		t.Fatalf("active intent was purged: %v", err)
	}
	if _, err := s.GetNotificationIntent(ctx, "intent-new-terminal"); err != nil {
		t.Fatalf("new terminal intent was purged: %v", err)
	}
}

func notificationFixture(name string, at time.Time, status notifications.Status) (notifications.Intent, []notifications.Attempt) {
	intent := notifications.Intent{
		ID: "intent-" + name, Topic: notifications.TopicAccountInvitation,
		RecipientKind: notifications.RecipientInvitation, RecipientID: "invitation-" + name,
		ReferenceKind: notifications.ReferenceInvitation, ReferenceID: "invitation-" + name,
		Policy: notifications.PolicyMandatoryAccount, Template: notifications.TemplateData{RecipientName: "Ada"},
		IdempotencyKey: "invitation-" + name + ":created", CreatedAt: at,
	}
	attempt := notifications.Attempt{
		ID: "attempt-" + name + "-1", IntentID: intent.ID, Means: notifications.MeansEmail,
		DestinationRef: "contact-" + name, DestinationRedacted: "a***@example.com",
		Status: status, AttemptNumber: 1, AvailableAt: at, CreatedAt: at,
	}
	if status == notifications.StatusSuppressed {
		attempt.FinishedAt = at
		attempt.OutcomeCode = notifications.OutcomeDeliveryDisabled
	}
	return intent, []notifications.Attempt{attempt}
}
