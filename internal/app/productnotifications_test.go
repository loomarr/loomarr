package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type journeyNotificationSource struct {
	*testkit.NotificationRepository
	channel store.Channel
}

func (s *journeyNotificationSource) ListNotificationReferenceRecipients(
	_ context.Context,
	kind notifications.ReferenceKind,
	id string,
) ([]string, error) {
	if kind == notifications.ReferenceChannel && id == s.channel.ID {
		return []string{"requester-1"}, nil
	}
	return nil, nil
}

func (s *journeyNotificationSource) GetChannel(context.Context, string) (store.Channel, error) {
	return s.channel, nil
}

func TestProductNotificationJourneyRoutesOnlyConfiguredAudiencesWithStableDeduplication(t *testing.T) {
	now := time.Date(2026, 8, 31, 21, 0, 0, 0, time.UTC)
	repository := testkit.NewNotificationRepository()
	destinations := []notifications.Destination{
		{ID: "approver-slack", Means: notifications.MeansSlack, Label: "Approvals", Scope: notifications.ScopeInstallation,
			Audience: notifications.RecipientApprovers, Topics: []notifications.Topic{notifications.TopicProposalSubmitted}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "requester-browser", Means: notifications.MeansWebPush, Label: "Requester browser", Scope: notifications.ScopePerson,
			OwnerID: "requester-1", Audience: notifications.RecipientPerson,
			Topics: []notifications.Topic{notifications.TopicProposalApproved, notifications.TopicChannelLive}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "operator-mqtt", Means: notifications.MeansMQTT, Label: "Home Assistant", Scope: notifications.ScopeInstallation,
			Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelLive}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
		{ID: "unconfigured-discord", Means: notifications.MeansDiscord, Label: "Other event", Scope: notifications.ScopeInstallation,
			Audience: notifications.RecipientOperators, Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
			CreatedAt: now, UpdatedAt: now},
	}
	for _, destination := range destinations {
		if err := repository.SaveNotificationDestination(t.Context(), destination); err != nil {
			t.Fatal(err)
		}
	}
	source := &journeyNotificationSource{
		NotificationRepository: repository,
		channel:                store.Channel{Channel: schedule.Channel{ID: "channel-1", Name: "Saturday Cartoons"}, Revision: 3},
	}
	nextID := 0
	service := notifications.NewService(repository, notifications.NewDestinationRouter(source, nil), nil, func() string {
		nextID++
		return fmt.Sprintf("notification-%d", nextID)
	}, func() time.Time { return now })
	coordinator := &productNotificationCoordinator{
		publisher: notifications.NewProductPublisher(service), source: source,
	}
	proposal := store.Proposal{
		ID: "proposal-1", CreatedBy: "requester-1",
		ProposalJSON: `{"channelName":"Saturday Cartoons"}`,
	}

	coordinator.ProposalSubmitted(t.Context(), proposal)
	coordinator.ProposalApproved(t.Context(), proposal, "admin-1")
	coordinator.ChannelChanged("channel-1", string(schedule.StatusLive))
	coordinator.ProposalSubmitted(t.Context(), proposal)
	coordinator.ProposalApproved(t.Context(), proposal, "admin-1")
	coordinator.ChannelChanged("channel-1", string(schedule.StatusLive))

	if len(repository.Intents) != 4 || len(repository.Attempts) != 4 {
		t.Fatalf("journey created %d intents and %d attempts, want four configured routes", len(repository.Intents), len(repository.Attempts))
	}
	wantRoutes := map[string]string{
		string(notifications.TopicProposalSubmitted) + ":" + string(notifications.RecipientApprovers): "approver-slack",
		string(notifications.TopicProposalApproved) + ":" + string(notifications.RecipientPerson):     "requester-browser",
		string(notifications.TopicChannelLive) + ":" + string(notifications.RecipientPerson):          "requester-browser",
		string(notifications.TopicChannelLive) + ":" + string(notifications.RecipientOperators):       "operator-mqtt",
	}
	for _, intent := range repository.Intents {
		key := string(intent.Topic) + ":" + string(intent.RecipientKind)
		wantDestination, ok := wantRoutes[key]
		if !ok {
			t.Fatalf("unexpected journey intent %+v", intent)
		}
		matched := false
		for _, attempt := range repository.Attempts {
			if attempt.IntentID == intent.ID {
				matched = true
				if attempt.DestinationRef != wantDestination {
					t.Fatalf("route %s destination = %q, want %q", key, attempt.DestinationRef, wantDestination)
				}
			}
		}
		if !matched {
			t.Fatalf("intent %s has no configured attempt", key)
		}
	}
}

func TestProposalSubjectTruncatesLongUnicodeWithoutBreakingUTF8(t *testing.T) {
	proposal := store.Proposal{ProposalJSON: `{"intent":{"description":"` + strings.Repeat("é", 150) + `"}}`}
	subject := proposalSubject(proposal)
	if len(subject) > 200 || subject == "Channel proposal" {
		t.Fatalf("unicode subject = %q (%d bytes)", subject, len(subject))
	}
}
