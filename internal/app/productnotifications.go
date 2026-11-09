package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/provision"
)

type productNotificationCoordinator struct {
	publisher *notifications.ProductPublisher
	source    productNotificationSource
	log       *slog.Logger
}

type productNotificationSource interface {
	ListNotificationReferenceRecipients(context.Context, notifications.ReferenceKind, string) ([]string, error)
	GetChannel(context.Context, string) (channelstore.Channel, error)
}

func (c *productNotificationCoordinator) ProposalSubmitted(ctx context.Context, proposal store.Proposal) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalSubmitted, "")
}

func (c *productNotificationCoordinator) ProposalApproved(ctx context.Context, proposal store.Proposal, _ string) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalApproved, proposal.ModSummary)
}

func (c *productNotificationCoordinator) ProposalDenied(ctx context.Context, proposal store.Proposal) {
	c.publishProposal(ctx, proposal, notifications.TopicProposalDenied, proposal.DenyReason)
}

func (c *productNotificationCoordinator) Provisioned(ctx context.Context, event provision.DomainEvent) {
	if c == nil || c.publisher == nil || c.source == nil {
		return
	}
	topic := notifications.TopicAcquisitionGaveUp
	if event.State == provision.Available {
		topic = notifications.TopicAcquisitionAvailable
	} else if event.State != provision.Unavailable {
		return
	}
	people, err := c.source.ListNotificationReferenceRecipients(ctx, notifications.ReferenceTitle, string(event.Key))
	if err == nil {
		_, err = c.publisher.Publish(ctx, notifications.ProductEvent{
			Identity: "title:" + string(event.Key) + ":" + string(event.State), Topic: topic,
			ReferenceID: string(event.Key), SubjectName: event.Title.Name, PersonIDs: people,
		})
	}
	c.logPublicationFailure(err, "title", string(event.Key), topic)
}

func (c *productNotificationCoordinator) ChannelChanged(channelID, previousStatus, status string) {
	if c == nil || c.publisher == nil || c.source == nil {
		return
	}
	previousTopic, previousNotifiable := channelNotificationTopic(previousStatus)
	topic, notifiable := channelNotificationTopic(status)
	if !notifiable || (previousNotifiable && previousTopic == topic) {
		return
	}
	ctx := context.Background()
	channel, err := c.source.GetChannel(ctx, channelID)
	if err != nil {
		c.logPublicationFailure(err, "channel", channelID, "")
		return
	}
	people, err := c.source.ListNotificationReferenceRecipients(ctx, notifications.ReferenceChannel, channelID)
	if err == nil {
		_, err = c.publisher.Publish(ctx, notifications.ProductEvent{
			Identity: "channel:" + channelID + ":revision:" + fmt.Sprint(channel.Revision) + ":" + status,
			Topic:    topic, ReferenceID: channelID, SubjectName: channel.Name, PersonIDs: people,
		})
	}
	c.logPublicationFailure(err, "channel", channelID, topic)
}

func channelNotificationTopic(status string) (notifications.Topic, bool) {
	switch schedule.ChannelStatus(status) {
	case schedule.StatusLive:
		return notifications.TopicChannelLive, true
	case schedule.StatusDrifted, schedule.StatusEmpty:
		return notifications.TopicChannelDegraded, true
	default:
		return "", false
	}
}

func (c *productNotificationCoordinator) publishProposal(
	ctx context.Context,
	proposal store.Proposal,
	topic notifications.Topic,
	summary string,
) {
	if c == nil || c.publisher == nil {
		return
	}
	people := []string(nil)
	if topic != notifications.TopicProposalSubmitted {
		people = []string{proposal.CreatedBy}
	}
	_, err := c.publisher.Publish(ctx, notifications.ProductEvent{
		Identity: "proposal:" + proposal.ID + ":" + proposalEventIdentity(topic),
		Topic:    topic, ReferenceID: proposal.ID, SubjectName: proposalSubject(proposal),
		Summary: summary, PersonIDs: people,
	})
	if err != nil && c.log != nil {
		c.log.Warn("product notification could not be published", "topic", topic, "proposal", proposal.ID, "err", err)
	}
}

func (c *productNotificationCoordinator) logPublicationFailure(err error, referenceKind, referenceID string, topic notifications.Topic) {
	if err != nil && c.log != nil {
		c.log.Warn("product notification could not be published", "topic", topic,
			"reference_kind", referenceKind, "reference_id", referenceID, "err", err)
	}
}

func proposalSubject(row store.Proposal) string {
	if name := strings.TrimSpace(row.Body.ChannelName); name != "" {
		return name
	}
	if description := strings.TrimSpace(row.Body.Intent.Description); description != "" {
		if len(description) > 200 {
			end := 200
			for end > 0 && !utf8.RuneStart(description[end]) {
				end--
			}
			return description[:end]
		}
		return description
	}
	return "Channel proposal"
}

// LibraryFolder tells the operators a media folder stopped opening, or opens again (#2151
// decision 32). The scanner calls it once per change, not per pass.
func (c *productNotificationCoordinator) LibraryFolder(ctx context.Context, root string, readable bool) {
	if c == nil || c.publisher == nil {
		return
	}
	topic, state := notifications.TopicLibraryFolderOffline, "offline"
	summary := "Loomarr can't open this folder. Nothing in it is marked missing until it opens again."
	if readable {
		topic, state, summary = notifications.TopicLibraryFolderBack, "back", "This folder opens again."
	}
	ref := string(scan.Authority(root))
	_, err := c.publisher.Publish(ctx, notifications.ProductEvent{
		Identity: "library:" + ref + ":" + state + ":" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		Topic:    topic, ReferenceID: ref, SubjectName: notificationText(root), Summary: summary,
	})
	c.logPublicationFailure(err, "library", ref, topic)
}

// LibraryChangesDegraded tells the operators an arr can no longer send Loomarr its new files, at
// most once a day per arr.
func (c *productNotificationCoordinator) LibraryChangesDegraded(ctx context.Context, app string, _ error) {
	if c == nil || c.publisher == nil {
		return
	}
	name := map[string]string{"sonarr": "Sonarr", "radarr": "Radarr"}[app]
	if name == "" {
		name = app
	}
	_, err := c.publisher.Publish(ctx, notifications.ProductEvent{
		Identity: "library:hooks:" + app + ":failed:" + time.Now().UTC().Format(time.DateOnly),
		Topic:    notifications.TopicLibraryChangesDegraded, ReferenceID: app, SubjectName: name,
		Summary: name + " can't tell Loomarr about new files, so they show up within the hour instead of " +
			"within seconds. Test it in Settings → Library.",
	})
	c.logPublicationFailure(err, "library", app, notifications.TopicLibraryChangesDegraded)
}

// LibraryNeedsYou tells the operators how many titles Loomarr can't tell what they are. The
// scanner calls it at most once a day, when any are new.
func (c *productNotificationCoordinator) LibraryNeedsYou(ctx context.Context, total int) {
	if c == nil || c.publisher == nil || total <= 0 {
		return
	}
	summary := "Loomarr can't tell what " + strconv.Itoa(total) + " titles are. Answer them in Library → Needs you."
	if total == 1 {
		summary = "Loomarr can't tell what 1 title is. Answer it in Library → Needs you."
	}
	_, err := c.publisher.Publish(ctx, notifications.ProductEvent{
		Identity: "library:needs:" + time.Now().UTC().Format(time.DateOnly),
		Topic:    notifications.TopicLibraryNeedsYou, ReferenceID: "needs", SubjectName: "Needs you",
		Summary: summary,
	})
	c.logPublicationFailure(err, "library", "needs", notifications.TopicLibraryNeedsYou)
}

// notificationText is s fit for a notification's subject: control characters become spaces, and
// it is cut to 200 bytes on a character boundary.
func notificationText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if len(s) <= 200 {
		return s
	}
	end := 200
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

func proposalEventIdentity(topic notifications.Topic) string {
	switch topic {
	case notifications.TopicProposalSubmitted:
		return "submitted"
	case notifications.TopicProposalApproved:
		return "approved"
	case notifications.TopicProposalDenied:
		return "denied"
	default:
		return "changed"
	}
}
