package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

type destinationSource struct {
	destinations []notifications.Destination
}

type audienceEligibility map[string]bool

func (e audienceEligibility) Eligible(_ context.Context, audience notifications.RecipientKind, personID string) (bool, error) {
	return e[string(audience)+":"+personID], nil
}

func (s destinationSource) ListNotificationDestinations(context.Context) ([]notifications.Destination, error) {
	return append([]notifications.Destination(nil), s.destinations...), nil
}

func TestDestinationRouterMatchesOnlyEnabledAuthorizedDestinations(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{
		{ID: "matching", Means: notifications.MeansSlack, Label: "Operations Slack",
			Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
			Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "disabled", Means: notifications.MeansDiscord, Label: "Disabled Discord",
			Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
			Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: false,
			CreatedAt: now, UpdatedAt: now},
		{ID: "wrong-topic", Means: notifications.MeansNtfy, Label: "Acquisition only",
			Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
			Topics: []notifications.Topic{notifications.TopicAcquisitionGaveUp}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "other-person", Means: notifications.MeansEmail, Label: "Another person's email",
			Scope: notifications.ScopePerson, OwnerID: "user-2", Audience: notifications.RecipientPerson,
			Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
	}}
	router := notifications.NewDestinationRouter(source, nil)
	routes, err := router.Routes(t.Context(), notifications.Intent{
		ID: "intent-1", Topic: notifications.TopicChannelDegraded,
		RecipientKind: notifications.RecipientOperators, RecipientID: "operators",
		ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Cartoons"},
		IdempotencyKey: "channel-1:degraded:operators", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].Means != notifications.MeansSlack ||
		routes[0].DestinationRef != "matching" || routes[0].DestinationRedacted != "Operations Slack" {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestDestinationRouterMatchesOnlyTheIntentPerson(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{
		{ID: "mine", Means: notifications.MeansWebPush, Label: "This browser",
			Scope: notifications.ScopePerson, OwnerID: "user-1", Audience: notifications.RecipientPerson,
			Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "theirs", Means: notifications.MeansEmail, Label: "Other email",
			Scope: notifications.ScopePerson, OwnerID: "user-2", Audience: notifications.RecipientPerson,
			Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
	}}
	routes, err := notifications.NewDestinationRouter(source, nil).Routes(t.Context(), notifications.Intent{
		ID: "intent-1", Topic: notifications.TopicProposalApproved,
		RecipientKind: notifications.RecipientPerson, RecipientID: "user-1",
		ReferenceKind: notifications.ReferenceProposal, ReferenceID: "proposal-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Cartoons"},
		IdempotencyKey: "proposal-1:approved:user-1", CreatedAt: now,
	})
	if err != nil || len(routes) != 1 || routes[0].DestinationRef != "mine" {
		t.Fatalf("routes = %+v, %v", routes, err)
	}
}

func TestDestinationRouterRechecksPersonalGroupEligibilityAtEventTime(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{
		{ID: "eligible", Means: notifications.MeansEmail, Label: "Current approver",
			Scope: notifications.ScopePerson, OwnerID: "admin-1", Audience: notifications.RecipientApprovers,
			Topics: []notifications.Topic{notifications.TopicProposalSubmitted}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "demoted", Means: notifications.MeansWebPush, Label: "Former approver",
			Scope: notifications.ScopePerson, OwnerID: "user-2", Audience: notifications.RecipientApprovers,
			Topics: []notifications.Topic{notifications.TopicProposalSubmitted}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
	}}
	routes, err := notifications.NewDestinationRouter(source, audienceEligibility{
		"approvers:admin-1": true,
	}).Routes(t.Context(), notifications.Intent{
		ID: "intent-1", Topic: notifications.TopicProposalSubmitted,
		RecipientKind: notifications.RecipientApprovers, RecipientID: "approvers",
		ReferenceKind: notifications.ReferenceProposal, ReferenceID: "proposal-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Cartoons"},
		IdempotencyKey: "proposal-1:submitted:approvers", CreatedAt: now,
	})
	if err != nil || len(routes) != 1 || routes[0].DestinationRef != "eligible" {
		t.Fatalf("personal approver routes = %+v, %v", routes, err)
	}
}

func TestDestinationRouterDerivesSharedAudienceFromTheEvent(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{{
		ID: "shared", Means: notifications.MeansSlack, Label: "Household Slack",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientApprovers,
		Topics: []notifications.Topic{notifications.TopicAcquisitionAvailable}, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}}}
	router := notifications.NewDestinationRouter(source, nil)
	person := productIntent(now, notifications.TopicAcquisitionAvailable, notifications.RecipientPerson, "member-1")
	if routes, err := router.Routes(t.Context(), person); err != nil || len(routes) != 0 {
		t.Fatalf("shared routes for requester intent = %+v, %v", routes, err)
	}
	operators := productIntent(now, notifications.TopicAcquisitionAvailable, notifications.RecipientOperators, "operators")
	if routes, err := router.Routes(t.Context(), operators); err != nil || len(routes) != 1 || routes[0].DestinationRef != "shared" {
		t.Fatalf("shared routes for operator intent = %+v, %v", routes, err)
	}
}

func TestDestinationRouterUsesSMTPAsOneRecipientAwareProvider(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{{
		ID: "smtp", Means: notifications.MeansEmail, Label: "Household email",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}}}
	routes, err := notifications.NewDestinationRouter(source, nil).Routes(t.Context(),
		productIntent(now, notifications.TopicProposalApproved, notifications.RecipientPerson, "member-1"))
	if err != nil || len(routes) != 1 || routes[0].DestinationRef != "smtp" {
		t.Fatalf("SMTP requester route = %+v, %v", routes, err)
	}
}

func TestDestinationRouterUsesPushoverForRequesterEvents(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	source := destinationSource{destinations: []notifications.Destination{{
		ID: "pushover", Means: notifications.MeansPushover, Label: "Household Pushover",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicProposalApproved}, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}}}
	routes, err := notifications.NewDestinationRouter(source, nil).Routes(t.Context(),
		productIntent(now, notifications.TopicProposalApproved, notifications.RecipientPerson, "member-1"))
	if err != nil || len(routes) != 1 || routes[0].DestinationRef != "pushover" {
		t.Fatalf("Pushover requester route = %+v, %v", routes, err)
	}
}

func productIntent(now time.Time, topic notifications.Topic, kind notifications.RecipientKind, id string) notifications.Intent {
	return notifications.Intent{
		ID: "intent-" + string(kind), Topic: topic, RecipientKind: kind, RecipientID: id,
		ReferenceKind: notifications.ReferenceTitle, ReferenceID: "reference-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Cartoons"},
		IdempotencyKey: "event:" + string(topic) + ":" + string(kind) + ":" + id, CreatedAt: now,
	}
}
