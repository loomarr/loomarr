package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/proposals"
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
		Body: proposals.Proposal{ChannelName: "Saturday Cartoons"},
	}

	coordinator.ProposalSubmitted(t.Context(), proposal)
	coordinator.ProposalApproved(t.Context(), proposal, "admin-1")
	coordinator.ChannelChanged("channel-1", string(schedule.StatusBuilding), string(schedule.StatusLive))
	coordinator.ProposalSubmitted(t.Context(), proposal)
	coordinator.ProposalApproved(t.Context(), proposal, "admin-1")
	source.channel.Revision++
	coordinator.ChannelChanged("channel-1", string(schedule.StatusLive), string(schedule.StatusLive))
	source.channel.Revision++
	coordinator.ChannelChanged("channel-1", string(schedule.StatusLive), string(schedule.StatusDrifted))
	source.channel.Revision++
	coordinator.ChannelChanged("channel-1", string(schedule.StatusDrifted), string(schedule.StatusEmpty))
	source.channel.Revision++
	coordinator.ChannelChanged("channel-1", string(schedule.StatusEmpty), string(schedule.StatusLive))

	if len(repository.Intents) != 8 || len(repository.Attempts) != 7 {
		t.Fatalf("journey created %d intents and %d attempts, want eight transition intents and seven configured routes",
			len(repository.Intents), len(repository.Attempts))
	}
	wantRoutes := map[string]string{
		string(notifications.TopicProposalSubmitted) + ":" + string(notifications.RecipientApprovers): "approver-slack",
		string(notifications.TopicProposalApproved) + ":" + string(notifications.RecipientPerson):     "requester-browser",
		string(notifications.TopicChannelLive) + ":" + string(notifications.RecipientPerson):          "requester-browser",
		string(notifications.TopicChannelLive) + ":" + string(notifications.RecipientOperators):       "operator-mqtt",
		string(notifications.TopicChannelDegraded) + ":" + string(notifications.RecipientOperators):   "unconfigured-discord",
	}
	for _, intent := range repository.Intents {
		key := string(intent.Topic) + ":" + string(intent.RecipientKind)
		wantDestination, ok := wantRoutes[key]
		if !ok {
			if intent.Topic == notifications.TopicChannelDegraded && intent.RecipientKind == notifications.RecipientPerson {
				continue
			}
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

// The scanner's notifications reach the operators' destination for each topic, never a person, and a
// failing arr is reported at most once a day.
func TestLibraryNotificationsReachOperators(t *testing.T) {
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	repository := testkit.NewNotificationRepository()
	if err := repository.SaveNotificationDestination(t.Context(), notifications.Destination{
		ID: "operator-ntfy", Means: notifications.MeansNtfy, Label: "Household", Scope: notifications.ScopeInstallation,
		Audience: notifications.RecipientOperators, Enabled: true, CreatedAt: now, UpdatedAt: now,
		Topics: []notifications.Topic{
			notifications.TopicLibraryFolderOffline, notifications.TopicLibraryFolderBack, notifications.TopicLibraryChangesDegraded,
			notifications.TopicLibraryNeedsYou,
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := &journeyNotificationSource{NotificationRepository: repository}
	nextID := 0
	service := notifications.NewService(repository, notifications.NewDestinationRouter(source, nil), nil, func() string {
		nextID++
		return fmt.Sprintf("notification-%d", nextID)
	}, func() time.Time { return now })
	var notify libraryNotifier = &productNotificationCoordinator{publisher: notifications.NewProductPublisher(service), source: source}

	long := "/mnt/" + strings.Repeat("ü", 150) + "\nmovies"
	notify.LibraryFolder(t.Context(), long, false)
	notify.LibraryFolder(t.Context(), long, true)
	notify.LibraryChangesDegraded(t.Context(), "sonarr", fmt.Errorf("refused"))
	notify.LibraryChangesDegraded(t.Context(), "sonarr", fmt.Errorf("refused again"))
	notify.LibraryNeedsYou(t.Context(), 3)
	notify.LibraryNeedsYou(t.Context(), 4)
	notify.LibraryNeedsYou(t.Context(), 0)

	if len(repository.Intents) != 4 || len(repository.Attempts) != 4 {
		t.Fatalf("intents = %d, attempts = %d; want four, each routed", len(repository.Intents), len(repository.Attempts))
	}
	topics := map[notifications.Topic]bool{}
	for _, intent := range repository.Intents {
		topics[intent.Topic] = true
		if intent.RecipientKind != notifications.RecipientOperators || intent.ReferenceKind != notifications.ReferenceLibrary {
			t.Errorf("intent %s went to %s about %s", intent.Topic, intent.RecipientKind, intent.ReferenceKind)
		}
		if strings.ContainsAny(intent.Template.SubjectName, "\n") || len(intent.Template.SubjectName) > 200 {
			t.Errorf("subject %q is not fit to send", intent.Template.SubjectName)
		}
		if intent.Topic == notifications.TopicLibraryNeedsYou &&
			intent.Template.Summary != "Loomarr can't tell what 3 titles are. Answer them in Library → Needs you." {
			t.Errorf("needs you summary = %q; want the day's first count", intent.Template.Summary)
		}
	}
	if len(topics) != 4 {
		t.Errorf("topics = %v", topics)
	}
}

func TestProposalSubjectTruncatesLongUnicodeWithoutBreakingUTF8(t *testing.T) {
	row := store.Proposal{Body: proposals.Proposal{Intent: proposals.Intent{Description: strings.Repeat("é", 150)}}}
	subject := proposalSubject(row)
	if len(subject) > 200 || subject == "Channel proposal" {
		t.Fatalf("unicode subject = %q (%d bytes)", subject, len(subject))
	}
}

func TestChannelNotificationTopicCollapsesDegradedStatuses(t *testing.T) {
	drifted, driftedOK := channelNotificationTopic(string(schedule.StatusDrifted))
	empty, emptyOK := channelNotificationTopic(string(schedule.StatusEmpty))
	if !driftedOK || !emptyOK || drifted != notifications.TopicChannelDegraded || empty != drifted {
		t.Fatalf("degraded topics = (%q, %t), (%q, %t)", drifted, driftedOK, empty, emptyOK)
	}
	if _, ok := channelNotificationTopic(string(schedule.StatusBuilding)); ok {
		t.Fatal("building must not be a product notification state")
	}
}
