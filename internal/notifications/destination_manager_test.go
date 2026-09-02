package notifications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/testkit"
)

type destinationValidator struct {
	means notifications.Means
	err   error
}

func (v destinationValidator) Means() notifications.Means { return v.means }

func (v destinationValidator) ValidateDestination(map[string]string, map[string]string) error {
	return v.err
}

func TestDestinationManagerKeepsCredentialsWriteOnlyAndScopesMemberReads(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(
		repository, []notifications.DestinationValidator{destinationValidator{means: notifications.MeansWebPush}},
		sequentialDestinationIDs(), func() time.Time { return now },
	)
	credentials := map[string]string{"endpoint": "https://push.example.test/subscription-private"}
	summary, err := manager.Create(t.Context(), notifications.Principal{PersonID: "user-1"}, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "This browser", Scope: notifications.ScopePerson,
		OwnerID: "user-1", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
		Configuration: map[string]string{"endpointHost": "push.example"}, Credentials: &credentials,
	})
	if err != nil || !summary.CredentialsConfigured {
		t.Fatalf("create personal destination = %+v, %v", summary, err)
	}
	stored, err := repository.ResolveNotificationDestination(t.Context(), summary.ID)
	if err != nil || stored.Credentials["endpoint"] != "https://push.example.test/subscription-private" {
		t.Fatalf("stored personal destination = %+v, %v", stored.Summary(), err)
	}
	if summaries, err := manager.List(t.Context(), notifications.Principal{PersonID: "user-2"}); err != nil || len(summaries) != 0 {
		t.Fatalf("other member summaries = %+v, %v", summaries, err)
	}
	if summaries, err := manager.List(t.Context(), notifications.Principal{PersonID: "user-1"}); err != nil ||
		len(summaries) != 1 || !summaries[0].CredentialsConfigured {
		t.Fatalf("owner summaries = %+v, %v", summaries, err)
	}
}

func TestDestinationManagerClassifiesAndRedactsTheUnifiedProviderSettings(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(repository, nil, sequentialDestinationIDs(), func() time.Time { return now })
	admin := notifications.Principal{PersonID: "admin-1", Administrator: true}
	summary, err := manager.Create(t.Context(), admin, notifications.DestinationCommand{
		Means: notifications.MeansEmail, Label: "Household SMTP", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicProposalApproved},
		Settings: map[string]string{
			"host": "mail.example.test", "port": "587", "password": "never-return-this",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repository.ResolveNotificationDestination(t.Context(), summary.ID)
	if err != nil || stored.Configuration["host"] != "mail.example.test" ||
		stored.Credentials["password"] != "never-return-this" {
		t.Fatalf("stored provider settings = %+v, %v", stored, err)
	}
	var password notifications.ProviderFieldState
	for _, state := range summary.Settings {
		if state.Key == "password" {
			password = state
		}
	}
	if password.Value != "" || !password.SecretConfigured {
		t.Fatalf("password state = %+v", password)
	}
	changes := map[string]string{"host": "relay.example.test"}
	updated, err := manager.Update(t.Context(), admin, summary.ID, notifications.DestinationUpdateCommand{
		Label: "Household SMTP", Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Settings: &changes,
	})
	if err != nil || !updated.CredentialsConfigured {
		t.Fatalf("updated provider = %+v, %v", updated, err)
	}
	stored, err = repository.ResolveNotificationDestination(t.Context(), summary.ID)
	if err != nil || stored.Configuration["host"] != "relay.example.test" ||
		stored.Credentials["password"] != "never-return-this" {
		t.Fatalf("updated provider settings = %+v, %v", stored, err)
	}
}

func TestDestinationManagerRejectsQueryStringsWhenUpdatingOrdinaryProviderURLs(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(repository, nil, sequentialDestinationIDs(), func() time.Time { return now })
	admin := notifications.Principal{PersonID: "admin-1", Administrator: true}
	created, err := manager.Create(t.Context(), admin, notifications.DestinationCommand{
		Means: notifications.MeansNtfy, Label: "Household ntfy", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelDegraded},
		Settings: map[string]string{"baseUrl": "https://notify.example.test", "topic": "private-topic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]string{"baseUrl": "https://notify.example.test?token=must-not-be-plaintext"}
	if _, err := manager.Update(t.Context(), admin, created.ID, notifications.DestinationUpdateCommand{
		Label: "Household ntfy", Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Settings: &changes,
	}); err == nil {
		t.Fatal("provider update accepted a credential-bearing ordinary URL")
	}
}

func TestDestinationManagerRejectsMemberInstallationAndCrossPersonWrites(t *testing.T) {
	manager := notifications.NewDestinationManager(
		testkit.NewNotificationRepository(), nil, sequentialDestinationIDs(), time.Now,
	)
	member := notifications.Principal{PersonID: "user-1"}
	if _, err := manager.Create(t.Context(), member, notifications.DestinationCommand{
		Means: notifications.MeansSlack, Label: "Operations", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelDegraded},
	}); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("member installation create = %v", err)
	}
	if _, err := manager.Create(t.Context(), member, notifications.DestinationCommand{
		Means: notifications.MeansEmail, Label: "Someone else", Scope: notifications.ScopePerson,
		OwnerID: "user-2", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicProposalApproved},
	}); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("cross-person create = %v", err)
	}
	if _, err := manager.Create(t.Context(), member, notifications.DestinationCommand{
		Means: notifications.MeansEmail, Label: "Approver mail", Scope: notifications.ScopePerson,
		OwnerID: "user-1", Audience: notifications.RecipientApprovers,
		Topics: []notifications.Topic{notifications.TopicProposalSubmitted},
	}); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("member group-audience create = %v", err)
	}
}

func TestDestinationManagerPreventsEnablingIncompleteOrUnavailableProvider(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(
		repository,
		[]notifications.DestinationValidator{destinationValidator{means: notifications.MeansSlack, err: errors.New("token required")}},
		sequentialDestinationIDs(), func() time.Time { return now },
	)
	admin := notifications.Principal{PersonID: "admin-1", Administrator: true}
	command := notifications.DestinationCommand{
		Means: notifications.MeansSlack, Label: "Operations", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelDegraded},
		Enabled: true,
	}
	if _, err := manager.Create(t.Context(), admin, command); err == nil {
		t.Fatal("enabled incomplete provider was accepted")
	}
	command.Enabled = false
	if _, err := manager.Create(t.Context(), admin, command); err != nil {
		t.Fatalf("disabled draft = %v", err)
	}
	command.Means = notifications.MeansDiscord
	command.Enabled = true
	if _, err := manager.Create(t.Context(), admin, command); !errors.Is(err, notifications.ErrMeansUnavailable) {
		t.Fatalf("enabled unavailable provider = %v", err)
	}
}

func TestDestinationManagerUpdatePreservesWriteOnlyCredentialsAndOwnership(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(
		repository, []notifications.DestinationValidator{destinationValidator{means: notifications.MeansWebPush}},
		sequentialDestinationIDs(), func() time.Time { return now },
	)
	credentials := map[string]string{"endpoint": "https://push.example.test/subscription-preserved"}
	created, err := manager.Create(t.Context(), notifications.Principal{PersonID: "user-1"}, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "Browser", Scope: notifications.ScopePerson,
		OwnerID: "user-1", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
		Credentials: &credentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	updated, err := manager.Update(t.Context(), notifications.Principal{PersonID: "user-1"}, created.ID, notifications.DestinationUpdateCommand{
		Label: "Living-room browser", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
		Credentials: nil,
	})
	if err != nil || updated.Label != "Living-room browser" || !updated.CredentialsConfigured {
		t.Fatalf("update destination = %+v, %v", updated, err)
	}
	stored, err := repository.ResolveNotificationDestination(t.Context(), created.ID)
	if err != nil || stored.Credentials["endpoint"] != "https://push.example.test/subscription-preserved" || !stored.UpdatedAt.Equal(now) {
		t.Fatalf("updated stored destination = %+v, %v", stored.Summary(), err)
	}
	if _, err := manager.Update(t.Context(), notifications.Principal{PersonID: "user-2"}, created.ID, notifications.DestinationUpdateCommand{
		Label: "Stolen", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive},
	}); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("cross-person update = %v", err)
	}
	if _, err := manager.Delete(t.Context(), notifications.Principal{PersonID: "user-2"}, created.ID, ""); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("cross-person delete = %v", err)
	}
	if _, err := manager.Delete(t.Context(), notifications.Principal{PersonID: "user-1"}, created.ID, ""); err != nil {
		t.Fatalf("owner delete = %v", err)
	}
}

func TestDestinationManagerReusesWebPushSubscriptionAndMatchesOnlyTheSelectedBrowser(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	manager := notifications.NewDestinationManager(
		repository, []notifications.DestinationValidator{destinationValidator{means: notifications.MeansWebPush}},
		sequentialDestinationIDs(), func() time.Time { return now },
	)
	principal := notifications.Principal{PersonID: "user-1"}
	settings := map[string]string{
		"endpoint": "https://push.example.test/subscription-one", "p256dh": "public-key", "auth": "auth-secret",
	}
	first, err := manager.Create(t.Context(), principal, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "Laptop", Scope: notifications.ScopePerson,
		OwnerID: principal.PersonID, Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true, Settings: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	second, err := manager.Create(t.Context(), principal, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "This browser", Scope: notifications.ScopePerson,
		OwnerID: principal.PersonID, Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true, Settings: settings,
	})
	if err != nil || second.ID != first.ID || second.Label != "This browser" {
		t.Fatalf("repeat create = %+v, %v; first = %+v", second, err, first)
	}
	if len(repository.Destinations) != 1 {
		t.Fatalf("destination count = %d, want one", len(repository.Destinations))
	}

	otherSettings := map[string]string{
		"endpoint": "https://push.example.test/subscription-two", "p256dh": "other-key", "auth": "other-secret",
	}
	other, err := manager.Create(t.Context(), principal, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "Tablet", Scope: notifications.ScopePerson,
		OwnerID: principal.PersonID, Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true, Settings: otherSettings,
	})
	if err != nil || other.ID == first.ID {
		t.Fatalf("other subscription = %+v, %v", other, err)
	}

	deleted, err := manager.Delete(t.Context(), principal, other.ID, settings["endpoint"])
	if err != nil || deleted.UnsubscribeCurrentBrowser {
		t.Fatalf("delete other browser = %+v, %v", deleted, err)
	}
	deleted, err = manager.Delete(t.Context(), principal, first.ID, settings["endpoint"])
	if err != nil || !deleted.UnsubscribeCurrentBrowser {
		t.Fatalf("delete current browser = %+v, %v", deleted, err)
	}
}

func TestDestinationManagerConsolidatesLegacyWebPushDuplicates(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Unix(1_900_000_000, 0)
	endpoint := "https://push.example.test/legacy-subscription"
	for index, id := range []string{"legacy-original", "legacy-duplicate"} {
		createdAt := now.Add(time.Duration(index) * time.Second)
		if err := repository.SaveNotificationDestination(t.Context(), notifications.Destination{
			ID: id, Means: notifications.MeansWebPush, Label: id, Scope: notifications.ScopePerson,
			OwnerID: "user-1", Audience: notifications.RecipientPerson,
			Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
			Credentials: map[string]string{"endpoint": endpoint, "p256dh": "legacy-key", "auth": "legacy-auth"},
			CreatedAt:   createdAt, UpdatedAt: createdAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	manager := notifications.NewDestinationManager(
		repository, []notifications.DestinationValidator{destinationValidator{means: notifications.MeansWebPush}},
		sequentialDestinationIDs(), func() time.Time { return now.Add(time.Minute) },
	)
	created, err := manager.Create(t.Context(), notifications.Principal{PersonID: "user-1"}, notifications.DestinationCommand{
		Means: notifications.MeansWebPush, Label: "Current label", Scope: notifications.ScopePerson,
		OwnerID: "user-1", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true,
		Settings: map[string]string{"endpoint": endpoint, "p256dh": "current-key", "auth": "current-auth"},
	})
	if err != nil || created.ID != "legacy-original" {
		t.Fatalf("consolidated create = %+v, %v", created, err)
	}
	if len(repository.Destinations) != 1 || repository.Destinations[created.ID].SubscriptionFingerprint == "" {
		t.Fatalf("consolidated destinations = %+v", repository.Destinations)
	}
}

func TestDestinationManagerQueuesDistinctTestIntentWithoutExposingDestination(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)
	manager := notifications.NewDestinationManager(
		repository, []notifications.DestinationValidator{destinationValidator{means: notifications.MeansSlack}},
		sequentialDestinationIDs(), func() time.Time { return now },
	)
	created, err := manager.Create(t.Context(), notifications.Principal{PersonID: "admin-1", Administrator: true}, notifications.DestinationCommand{
		Means: notifications.MeansSlack, Label: "Operations", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tester := &recordingDestinationTester{}
	manager.WithTester(tester)

	result, err := manager.Test(t.Context(), notifications.Principal{PersonID: "admin-1", Administrator: true}, created.ID, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.IntentID != "test-intent-1" || !result.Created {
		t.Fatalf("test result = %+v", result)
	}
	if tester.destination.ID != created.ID || tester.requestID != "request-1" {
		t.Fatalf("tester call = destination %+v request %q", tester.destination, tester.requestID)
	}
	if _, err := manager.Test(t.Context(), notifications.Principal{PersonID: "member-1"}, created.ID, "request-2"); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("member test error = %v, want forbidden", err)
	}
}

func TestDestinationManagerExposesRedactedHealthOnlyToAdministrators(t *testing.T) {
	repository := testkit.NewNotificationRepository()
	now := time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	installation := notifications.Destination{
		ID: "installation-1", Means: notifications.MeansSlack, Label: "Operations",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	personal := notifications.Destination{
		ID: "personal-1", Means: notifications.MeansWebPush, Label: "Browser", Scope: notifications.ScopePerson,
		OwnerID: "member-1", Audience: notifications.RecipientPerson,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.SaveNotificationDestination(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveNotificationDestination(t.Context(), personal); err != nil {
		t.Fatal(err)
	}
	repository.Attempts["failed-1"] = notifications.Attempt{
		ID: "failed-1", IntentID: "intent-1", Means: notifications.MeansSlack,
		DestinationRef: installation.ID, DestinationRedacted: installation.Label,
		Status: notifications.StatusFailed, AttemptNumber: 1, AvailableAt: now, CreatedAt: now,
		FinishedAt: now.Add(time.Minute), FailureClass: notifications.FailurePermanent,
		OutcomeCode: notifications.OutcomeConfigurationInvalid,
	}
	manager := notifications.NewDestinationManager(repository, nil, sequentialDestinationIDs(), func() time.Time { return now })

	adminList, err := manager.List(t.Context(), notifications.Principal{PersonID: "admin-1", Administrator: true})
	if err != nil || len(adminList) != 1 || adminList[0].Health == nil ||
		adminList[0].Health.LastFailureOutcome != notifications.OutcomeConfigurationInvalid {
		t.Fatalf("admin health = %+v, %v", adminList, err)
	}
	memberList, err := manager.List(t.Context(), notifications.Principal{PersonID: "member-1"})
	if err != nil || len(memberList) != 1 || memberList[0].Health != nil {
		t.Fatalf("member health = %+v, %v", memberList, err)
	}
}

type recordingDestinationTester struct {
	destination notifications.DestinationMetadata
	requestID   string
}

func (t *recordingDestinationTester) PublishDestinationTest(
	_ context.Context,
	destination notifications.DestinationMetadata,
	requestID string,
) (notifications.DestinationTestResult, error) {
	t.destination = destination
	t.requestID = requestID
	return notifications.DestinationTestResult{IntentID: "test-intent-1", Created: true}, nil
}

func sequentialDestinationIDs() func() string {
	n := 0
	return func() string {
		n++
		return "destination-generated-" + string(rune('0'+n))
	}
}
