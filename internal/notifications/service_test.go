package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPublishIsTypedDurableAndIdempotent(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	service := notifications.NewService(repository, emailRouter(), nil, sequentialIDs(), func() time.Time { return now })
	command := recoveryCommand()

	first, created, err := service.Publish(context.Background(), command)
	if err != nil || !created {
		t.Fatalf("first publish = %+v, %t, %v", first, created, err)
	}
	second, created, err := service.Publish(context.Background(), command)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("idempotent publish = %+v, %t, %v", second, created, err)
	}
	if len(repository.Intents) != 1 || len(repository.Attempts) != 1 {
		t.Fatalf("durable rows = %d intents, %d attempts", len(repository.Intents), len(repository.Attempts))
	}
}

func TestPublishPersistsSuppressionWithoutCallingAnAdapter(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	adapter := &testkit.NotificationAdapter{
		DeliveryMeans: notifications.MeansEmail,
		Result:        notifications.Result{Status: notifications.StatusDelivered},
	}
	router := testkit.NotificationRouter{RoutesResult: []notifications.Route{{
		Means: notifications.MeansEmail, DestinationRef: "contact-1", DestinationRedacted: "unavailable",
		Suppressed: notifications.OutcomeDeliveryDisabled,
	}}}
	service := notifications.NewService(repository, router, []notifications.Adapter{adapter}, sequentialIDs(), func() time.Time { return now })
	_, created, err := service.Publish(context.Background(), recoveryCommand())
	if err != nil || !created {
		t.Fatalf("publish suppressed = %t, %v", created, err)
	}
	run, err := service.RunOne(context.Background(), "worker-1")
	if err != nil || run {
		t.Fatalf("run suppressed = %t, %v", run, err)
	}
	if len(adapter.Calls) != 0 {
		t.Fatal("suppressed delivery reached adapter")
	}
	for _, intent := range repository.Intents {
		if intent.TerminalAt.IsZero() {
			t.Fatal("all-suppressed intent was not terminal")
		}
	}
}

func TestConfigurableProductIntentRoutesToIndependentDeliveryMeans(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	discord := &testkit.NotificationAdapter{
		DeliveryMeans: notifications.MeansDiscord,
		Result:        notifications.Result{Status: notifications.StatusDelivered, ProviderMessageID: "discord-safe-1"},
	}
	router := testkit.NotificationRouter{RoutesResult: []notifications.Route{
		{Means: notifications.MeansDiscord, DestinationRef: "destination-discord", DestinationRedacted: "Family Discord"},
		{Means: notifications.MeansWebhook, DestinationRef: "destination-webhook", DestinationRedacted: "Automation webhook"},
	}}
	service := notifications.NewService(repository, router, []notifications.Adapter{discord}, sequentialIDs(), func() time.Time { return now })

	intent, created, err := service.Publish(t.Context(), productCommand())
	if err != nil || !created {
		t.Fatalf("publish product intent = %+v, %t, %v", intent, created, err)
	}
	if len(repository.Intents) != 1 || len(repository.Attempts) != 2 {
		t.Fatalf("durable fan-out = %d intents, %d attempts", len(repository.Intents), len(repository.Attempts))
	}
	for range 2 {
		if ran, runErr := service.RunOne(t.Context(), "worker-1"); runErr != nil || !ran {
			t.Fatalf("run product delivery = %t, %v", ran, runErr)
		}
	}
	if len(discord.Calls) != 1 {
		t.Fatalf("Discord adapter calls = %d, want one", len(discord.Calls))
	}
	attempts, err := repository.ListNotificationAttempts(t.Context(), intent.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("product attempts = %+v, %v", attempts, err)
	}
	statuses := map[notifications.Means]notifications.Attempt{}
	for _, attempt := range attempts {
		statuses[attempt.Means] = attempt
	}
	if statuses[notifications.MeansDiscord].Status != notifications.StatusDelivered ||
		statuses[notifications.MeansWebhook].Status != notifications.StatusFailed ||
		statuses[notifications.MeansWebhook].OutcomeCode != notifications.OutcomeMeansUnavailable {
		t.Fatalf("independent outcomes = %+v", statuses)
	}
}

func TestMandatoryAccountIntentRejectsANonEmailRoute(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	router := testkit.NotificationRouter{RoutesResult: []notifications.Route{{
		Means: notifications.MeansDiscord, DestinationRef: "destination-discord", DestinationRedacted: "Family Discord",
	}}}
	service := notifications.NewService(repository, router, nil, sequentialIDs(), func() time.Time {
		return time.Unix(1_900_000_000, 0)
	})
	if _, _, err := service.Publish(t.Context(), recoveryCommand()); err == nil {
		t.Fatal("mandatory account intent accepted a non-email route")
	}
	if len(repository.Intents) != 0 || len(repository.Attempts) != 0 {
		t.Fatal("invalid account route reached persistence")
	}
}

func TestConfigurableProductIntentPersistsWhenNoDestinationMatches(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	service := notifications.NewService(
		repository, testkit.NotificationRouter{}, nil, sequentialIDs(),
		func() time.Time { return time.Unix(1_900_000_000, 0) },
	)
	intent, created, err := service.Publish(t.Context(), productCommand())
	if err != nil || !created {
		t.Fatalf("publish unrouted product intent = %+v, %t, %v", intent, created, err)
	}
	if intent.TerminalAt.IsZero() || len(repository.Intents) != 1 || len(repository.Attempts) != 0 {
		t.Fatalf("unrouted durable fact = %+v, %d intents, %d attempts", intent, len(repository.Intents), len(repository.Attempts))
	}
	if ran, runErr := service.RunOne(t.Context(), "worker-1"); runErr != nil || ran {
		t.Fatalf("run unrouted product intent = %t, %v", ran, runErr)
	}
}

func TestMandatoryAccountIntentStillRequiresADeliveryDecision(t *testing.T) {
	service := notifications.NewService(
		testkit.NewNotificationRepository(), testkit.NotificationRouter{}, nil, sequentialIDs(),
		func() time.Time { return time.Unix(1_900_000_000, 0) },
	)
	if _, _, err := service.Publish(t.Context(), recoveryCommand()); err == nil {
		t.Fatal("mandatory account intent accepted no delivery decision")
	}
}

func TestRunOneRetriesOnlyDefinitelyPreAcceptanceFailure(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	adapter := &testkit.NotificationAdapter{
		DeliveryMeans: notifications.MeansEmail,
		Result: notifications.Result{
			Status: notifications.StatusFailed, FailureClass: notifications.FailureTransientPreAcceptance,
			OutcomeCode: notifications.OutcomeTransportUnavailable,
		},
	}
	service := notifications.NewService(repository, emailRouter(), []notifications.Adapter{adapter}, sequentialIDs(), func() time.Time { return now })
	intent, _, err := service.Publish(context.Background(), recoveryCommand())
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.RunOne(context.Background(), "worker-1")
	if err != nil || !run {
		t.Fatalf("run transient = %t, %v", run, err)
	}
	if len(repository.Completions) != 1 || repository.Completions[0].Next == nil {
		t.Fatal("transient pre-acceptance failure did not schedule a retry")
	}
	next := repository.Completions[0].Next
	wantDelay, _ := notifications.RetryDelay(intent.ID, 2)
	if next.AttemptNumber != 2 || !next.AvailableAt.Equal(now.Add(wantDelay)) {
		t.Fatalf("retry = %+v, want attempt 2 at %v", next, now.Add(wantDelay))
	}

	adapter.Result = notifications.Result{
		Status: notifications.StatusFailed, FailureClass: notifications.FailureAmbiguous,
		OutcomeCode: notifications.OutcomeAcceptanceAmbiguous,
	}
	now = next.AvailableAt
	run, err = service.RunOne(context.Background(), "worker-1")
	if err != nil || !run {
		t.Fatalf("run ambiguous = %t, %v", run, err)
	}
	if repository.Completions[1].Next != nil {
		t.Fatal("ambiguous acceptance automatically retried")
	}
}

func TestRunOnePersistsNoArbitraryAdapterError(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	adapter := &testkit.NotificationAdapter{
		DeliveryMeans: notifications.MeansEmail,
		Result: notifications.Result{
			Status: notifications.StatusFailed, FailureClass: notifications.FailurePermanent,
			OutcomeCode: "password=not-safe",
		},
	}
	service := notifications.NewService(repository, emailRouter(), []notifications.Adapter{adapter}, sequentialIDs(), func() time.Time { return now })
	if _, _, err := service.Publish(context.Background(), recoveryCommand()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunOne(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	completion := repository.Completions[0]
	if completion.OutcomeCode != notifications.OutcomeConfigurationInvalid || completion.FailureClass != notifications.FailurePermanent {
		t.Fatalf("invalid adapter outcome persisted as %+v", completion)
	}
}

func TestLatestDeliveryComposesNewestIntentAndAttemptForAReference(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	adapter := &testkit.NotificationAdapter{
		DeliveryMeans: notifications.MeansEmail,
		Result:        notifications.Result{Status: notifications.StatusDelivered, ProviderMessageID: "safe-42"},
	}
	service := notifications.NewService(repository, emailRouter(), []notifications.Adapter{adapter}, sequentialIDs(), func() time.Time { return now })
	command := notifications.PublishCommand{
		Topic: notifications.TopicAccountInvitation, RecipientKind: notifications.RecipientInvitation,
		RecipientID: "invitation-1", ReferenceKind: notifications.ReferenceInvitation,
		ReferenceID: "invitation-1", Policy: notifications.PolicyMandatoryAccount,
		IdempotencyKey: "invitation-1:email:first",
	}
	if _, _, err := service.Publish(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunOne(t.Context(), "worker-1"); err != nil {
		t.Fatal(err)
	}

	delivered, err := service.LatestDelivery(t.Context(), notifications.ReferenceInvitation, "invitation-1")
	if err != nil || delivered.Status != notifications.StatusDelivered || delivered.AttemptNumber != 1 ||
		delivered.ProviderMessageID != "safe-42" {
		t.Fatalf("delivered summary = %+v, %v", delivered, err)
	}

	now = now.Add(time.Minute)
	command.IdempotencyKey = "invitation-1:email:resend"
	if _, _, err := service.Publish(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	queued, err := service.LatestDelivery(t.Context(), notifications.ReferenceInvitation, "invitation-1")
	if err != nil || queued.Status != notifications.StatusQueued || queued.AttemptNumber != 1 ||
		!queued.UpdatedAt.Equal(now) {
		t.Fatalf("queued summary = %+v, %v", queued, err)
	}
}

func emailRouter() testkit.NotificationRouter {
	return testkit.NotificationRouter{RoutesResult: []notifications.Route{{
		Means: notifications.MeansEmail, DestinationRef: "contact-1", DestinationRedacted: "a***@example.com",
	}}}
}

func recoveryCommand() notifications.PublishCommand {
	return notifications.PublishCommand{
		Topic: notifications.TopicLocalPasswordRecovery, RecipientKind: notifications.RecipientPerson,
		RecipientID: "user-1", ReferenceKind: notifications.ReferenceRecovery, ReferenceID: "recovery-1",
		Policy: notifications.PolicyMandatoryAccount, IdempotencyKey: "recovery-1:created",
	}
}

func productCommand() notifications.PublishCommand {
	return notifications.PublishCommand{
		Topic: notifications.TopicChannelLive, RecipientKind: notifications.RecipientPerson,
		RecipientID: "user-1", ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Saturday Cartoons"},
		IdempotencyKey: "channel-1:live:user-1",
	}
}

func sequentialIDs() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
}
