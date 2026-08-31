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
